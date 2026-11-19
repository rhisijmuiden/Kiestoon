package firmware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Release is the newest firmware a source knows for a group of models.
type Release struct {
	Vendor string `json:"vendor"`
	// Models holds model ids or families with x as a wildcard digit ("GRP261x" covers GRP2612).
	Models  []string `json:"models"`
	Version string   `json:"version"`
	URL     string   `json:"url"`
	SHA256  string   `json:"sha256,omitempty"`
	Notes   string   `json:"notes,omitempty"`
	Source  string   `json:"source"`
}

// Catalog is the firmware list that the Kiestoon project keeps for vendors without a public list.
type Catalog struct {
	Firmware []Release `json:"firmware"`
}

// DefaultCatalogURL is the catalog in the Kiestoon repository.
const DefaultCatalogURL = "https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/packaging/firmware/catalog.json"

// GrandstreamURL is Grandstream's public firmware page.
const GrandstreamURL = "https://www.grandstream.com/support/firmware"

// Finder looks up the newest firmware at the vendors.
type Finder struct {
	Client      *http.Client
	CatalogURL  string // "" is the default catalog
	Grandstream string // "" is GrandstreamURL
}

// Releases returns what every source knows. A source that fails is reported and skipped.
func (f *Finder) Releases(ctx context.Context) ([]Release, []string) {
	var out []Release
	var errs []string
	cat := f.CatalogURL
	if cat == "" {
		cat = DefaultCatalogURL
	}
	if b, err := f.get(ctx, cat, 2<<20); err != nil {
		errs = append(errs, "catalog: "+err.Error())
	} else if rs, err := ParseCatalog(b); err != nil {
		errs = append(errs, "catalog: "+err.Error())
	} else {
		out = append(out, rs...)
	}
	gs := f.Grandstream
	if gs == "" {
		gs = GrandstreamURL
	}
	if b, err := f.get(ctx, gs, 8<<20); err != nil {
		errs = append(errs, "grandstream: "+err.Error())
	} else {
		out = append(out, ParseGrandstream(string(b))...)
	}
	return out, errs
}

func (f *Finder) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Kiestoon firmware check")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// ValidURL reports whether a firmware may be downloaded from u: http(s), no credentials.
func ValidURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != "" && p.User == nil
}

var reSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseCatalog reads a catalog and drops entries that are incomplete.
func ParseCatalog(b []byte) ([]Release, error) {
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	out := []Release{}
	for _, r := range c.Firmware {
		r.Vendor = strings.ToLower(strings.TrimSpace(r.Vendor))
		r.SHA256 = strings.ToLower(strings.TrimSpace(r.SHA256))
		if r.Vendor == "" || len(r.Models) == 0 || !reVersion.MatchString(r.Version) || r.Version == "" || !ValidURL(r.URL) || (r.SHA256 != "" && !reSHA.MatchString(r.SHA256)) {
			continue
		}
		r.Source = "catalog"
		out = append(out, r)
	}
	return out, nil
}

// Grandstream names its files Release_<models>_<version>.zip, for example Release_GRP261x_1.0.13.127.zip
// or Release_WP810_WP822_WP825_1.0.11.83.zip.
var reGrandstream = regexp.MustCompile(`https?://firmware\.grandstream\.com/Release_([A-Za-z0-9_-]+?)_([0-9]+(?:\.[0-9]+){2,})\.zip`)

// ParseGrandstream finds the firmware files on Grandstream's firmware page; for each model group
// the newest version wins.
func ParseGrandstream(page string) []Release {
	best := map[string]Release{}
	var order []string
	for _, m := range reGrandstream.FindAllStringSubmatch(page, -1) {
		var models []string
		for _, tok := range regexp.MustCompile(`[_-]`).Split(m[1], -1) {
			if len(tok) >= 3 && tok[0] >= 'A' && tok[0] <= 'Z' {
				models = append(models, tok)
			}
		}
		if len(models) == 0 {
			continue
		}
		key := strings.Join(models, ",")
		r := Release{Vendor: "grandstream", Models: models, Version: m[2], URL: strings.Replace(m[0], "http://", "https://", 1), Source: "grandstream"}
		if old, ok := best[key]; !ok {
			order = append(order, key)
			best[key] = r
		} else if CompareVersions(r.Version, old.Version) > 0 {
			best[key] = r
		}
	}
	out := make([]Release, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

// MatchModel tells how well pattern fits model: -1 not at all, otherwise the number of wildcards
// (0 is an exact match). Case is ignored.
func MatchModel(pattern, model string) int {
	p, m := strings.ToUpper(pattern), strings.ToUpper(model)
	if p == m {
		return 0
	}
	if len(p) != len(m) || !strings.Contains(p, "X") {
		return -1
	}
	n := 0
	for i := range len(p) {
		switch {
		case p[i] == m[i]:
		case p[i] == 'X' && m[i] >= '0' && m[i] <= '9':
			n++
		default:
			return -1
		}
	}
	return n
}

// Latest picks the release for a model: the most specific match, then the newest version.
func Latest(rs []Release, vendor, model string) *Release {
	var best *Release
	bestScore := -1
	for i := range rs {
		r := &rs[i]
		if r.Vendor != vendor {
			continue
		}
		score := -1
		for _, p := range r.Models {
			if s := MatchModel(p, model); s >= 0 && (score < 0 || s < score) {
				score = s
			}
		}
		if score < 0 {
			continue
		}
		if best == nil || score < bestScore || (score == bestScore && CompareVersions(r.Version, best.Version) > 0) {
			best, bestScore = r, score
		}
	}
	return best
}

var reUAVersion = regexp.MustCompile(`[0-9]+(?:\.[0-9]+){2,}`)

// VersionFromUserAgent finds the firmware version in a phone's User-Agent, for example
// "Yealink SIP-T42U 108.86.0.45 00:15:65:aa:bb:cc" or "Grandstream GRP2612 1.0.13.127".
func VersionFromUserAgent(ua string) string {
	all := reUAVersion.FindAllString(ua, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

// CompareVersions compares dotted versions number by number: -1, 0 or 1.
func CompareVersions(a, b string) int {
	split := func(s string) []int {
		var out []int
		for _, p := range regexp.MustCompile(`[^0-9]+`).Split(s, -1) {
			if p == "" {
				continue
			}
			n, _ := strconv.Atoi(p)
			out = append(out, n)
		}
		return out
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			if p < q {
				return -1
			}
			return 1
		}
	}
	return 0
}

// FindVersion returns the stored firmware of a vendor with this version, or nil.
func (s *Store) FindVersion(ctx context.Context, vendor, version string) *Firmware {
	row := s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM firmware WHERE vendor = ? AND version = ? ORDER BY id DESC LIMIT 1`, vendor, version)
	f, err := scan(row)
	if err != nil {
		return nil
	}
	return f
}

// ErrChecksum is returned when a download does not match the catalog.
var ErrChecksum = fmt.Errorf("checksum mismatch")
