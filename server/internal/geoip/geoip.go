// Package geoip downloads and reads the free "IP to Country Lite" database of DB-IP (CC BY 4.0,
// https://db-ip.com) for geo-blocking. The portal downloads the file once a month; the root helper
// turns the selected countries into nftables sets.
package geoip

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// FileName is the database inside the geoip directory of the data directory.
const FileName = "dbip-country-lite.csv.gz"

// Path returns the database location.
func Path(dataDir string) string { return filepath.Join(dataDir, "geoip", FileName) }

// Attribution must be shown where the data is used (CC BY 4.0).
const Attribution = "IP Geolocation by DB-IP (https://db-ip.com), CC BY 4.0"

var reCountry = regexp.MustCompile(`^[A-Z]{2}$`)

// NormalizeCountries upper-cases, deduplicates and checks ISO 3166-1 alpha-2 codes.
func NormalizeCountries(in []string) ([]string, error) {
	out := []string{}
	for _, c := range in {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if !reCountry.MatchString(c) {
			return nil, fmt.Errorf("invalid country %q", c)
		}
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Sets are the networks of the selected countries.
type Sets struct {
	V4 []string
	V6 []string
}

// Empty reports whether no network was found.
func (s Sets) Empty() bool { return len(s.V4)+len(s.V6) == 0 }

// Load reads the database at path and returns the networks of the given countries as CIDR prefixes.
func Load(path string, countries []string) (Sets, error) {
	f, err := os.Open(path)
	if err != nil {
		return Sets{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Sets{}, err
	}
	return Parse(gz, countries)
}

// Parse reads "start,end,CC" lines.
func Parse(r io.Reader, countries []string) (Sets, error) {
	want := map[string]bool{}
	for _, c := range countries {
		want[strings.ToUpper(c)] = true
	}
	var s Sets
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	lines := 0
	for sc.Scan() {
		lines++
		parts := strings.Split(sc.Text(), ",")
		if len(parts) < 3 || !want[strings.Trim(parts[2], `" `)] {
			continue
		}
		from, err1 := netip.ParseAddr(strings.Trim(parts[0], `" `))
		to, err2 := netip.ParseAddr(strings.Trim(parts[1], `" `))
		if err1 != nil || err2 != nil || from.Is4() != to.Is4() || to.Less(from) {
			continue
		}
		for _, p := range RangePrefixes(from, to) {
			if p.Addr().Is4() {
				s.V4 = append(s.V4, p.String())
			} else {
				s.V6 = append(s.V6, p.String())
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Sets{}, err
	}
	if lines == 0 {
		return Sets{}, errors.New("empty geoip database")
	}
	return s, nil
}

// RangePrefixes splits the inclusive range from-to into the smallest list of CIDR prefixes.
func RangePrefixes(from, to netip.Addr) []netip.Prefix {
	var out []netip.Prefix
	bits := from.BitLen()
	for {
		// The largest block that starts at from and does not go past to.
		size := bits
		for size > 0 {
			p, err := from.Prefix(size - 1)
			if err != nil || p.Addr() != from || lastAddr(p).Compare(to) > 0 {
				break
			}
			size--
		}
		p := netip.PrefixFrom(from, size)
		out = append(out, p)
		last := lastAddr(p)
		if last.Compare(to) >= 0 {
			return out
		}
		from = last.Next()
		if !from.IsValid() {
			return out
		}
	}
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 0x80 >> (i % 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// URL is the download address of the database of a month.
func URL(t time.Time) string {
	return fmt.Sprintf("https://download.db-ip.com/free/dbip-country-lite-%s.csv.gz", t.UTC().Format("2006-01"))
}

// Age returns how old the stored database is, or a negative duration when there is none.
func Age(dataDir string) time.Duration {
	st, err := os.Stat(Path(dataDir))
	if err != nil {
		return -1
	}
	return time.Since(st.ModTime())
}

// Download fetches this month's database (or last month's, early in the month) into the data directory.
func Download(ctx context.Context, client *http.Client, dataDir string) error {
	now := time.Now()
	var lastErr error
	for _, t := range []time.Time{now, now.AddDate(0, -1, 0)} {
		if err := download(ctx, client, URL(t), Path(dataDir)); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func download(ctx context.Context, client *http.Client, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Kiestoon")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 200<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	// Check that it really is the gzip CSV before replacing the previous month.
	if _, err := Load(tmp, []string{"NL"}); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("downloaded file: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
