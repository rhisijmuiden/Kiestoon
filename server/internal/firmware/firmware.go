// Package firmware keeps phone firmware files and decides which one a phone model gets. Files live in
// <data>/firmware/<token>/; phones download them from /prov/fw/<token>/<file> without credentials
// (firmware is public vendor software, and the long random token keeps the list private).
package firmware

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// MaxSize is the largest firmware upload (a Poly or Cisco bundle is a few hundred MB).
const MaxSize = 1 << 30

// Firmware is a stored firmware file (or the main file of an extracted bundle).
type Firmware struct {
	ID       int64    `json:"id"`
	Vendor   string   `json:"vendor"`
	Models   []string `json:"models"`
	Version  string   `json:"version"`
	Filename string   `json:"filename"`
	Token    string   `json:"-"`
	Size     int64    `json:"size"`
	SHA256   string   `json:"sha256"`
	Active   bool     `json:"active"`
	Created  string   `json:"createdAt"`
	Files    int      `json:"files"`
}

// Store manages the firmware table and directory.
type Store struct {
	DB  *sql.DB
	Dir string
}

// ErrNotFound is returned for an unknown firmware.
var ErrNotFound = errors.New("not found")

var (
	reName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,120}$`)
	reVersion = regexp.MustCompile(`^[A-Za-z0-9._ ()+-]{0,40}$`)
	reToken   = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// SafeName reports whether a file name can be stored and served as is.
func SafeName(n string) bool { return reName.MatchString(n) && !strings.Contains(n, "..") }

// CleanName turns a vendor's file name into a safe one: "T54W(T57W,T53W)-96.86.0.70.zip" becomes
// "T54W_T57W_T53W_-96.86.0.70.zip", "Release Notes.pdf" becomes "Release_Notes.pdf". It returns ""
// when nothing usable is left.
func CleanName(n string) string {
	n = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '+', r == '-':
			return r
		}
		return '_'
	}, filepath.Base(strings.ReplaceAll(n, "\\", "/")))
	for strings.Contains(n, "..") {
		n = strings.ReplaceAll(n, "..", ".")
	}
	n = strings.TrimLeft(n, "._+-")
	if len(n) > 121 {
		ext := filepath.Ext(n)
		if len(ext) > 10 {
			ext = ""
		}
		n = n[:121-len(ext)] + ext
	}
	if !SafeName(n) {
		return ""
	}
	return n
}

const cols = `id, vendor, models, version, filename, token, size, sha256, active, created_at`

func scan(row interface{ Scan(...any) error }) (*Firmware, error) {
	var f Firmware
	var models string
	var active int
	err := row.Scan(&f.ID, &f.Vendor, &models, &f.Version, &f.Filename, &f.Token, &f.Size, &f.SHA256, &active, &f.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.Active = active == 1
	_ = json.Unmarshal([]byte(models), &f.Models)
	if f.Models == nil {
		f.Models = []string{}
	}
	return &f, nil
}

// List returns all firmware, newest first.
func (s *Store) List(ctx context.Context) ([]Firmware, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM firmware ORDER BY vendor, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Firmware{}
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			return nil, err
		}
		entries, _ := os.ReadDir(filepath.Join(s.Dir, f.Token))
		f.Files = len(entries)
		out = append(out, *f)
	}
	return out, rows.Err()
}

// Get returns one firmware.
func (s *Store) Get(ctx context.Context, id int64) (*Firmware, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM firmware WHERE id = ?`, id))
}

// ForModel returns the active firmware of a phone model, or nil.
func (s *Store) ForModel(ctx context.Context, vendor, model string) *Firmware {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM firmware WHERE vendor = ? AND active = 1 ORDER BY id DESC`, vendor)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scan(rows)
		if err == nil && slices.Contains(f.Models, model) {
			return f
		}
	}
	return nil
}

// Path is the file on disk of a download request (token and file name from the URL), or "".
func (s *Store) Path(token, name string) string {
	if !reToken.MatchString(token) || !SafeName(name) {
		return ""
	}
	p := filepath.Join(s.Dir, token, name)
	if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return p
}

// Update changes the version label, the models and whether the firmware is active. Activating a
// firmware deactivates the other firmware of the same models.
func (s *Store) Update(ctx context.Context, f *Firmware, valid func(vendor, model string) bool) error {
	f.Version = strings.TrimSpace(f.Version)
	if !reVersion.MatchString(f.Version) {
		return errors.New("version")
	}
	models := []string{}
	for _, m := range f.Models {
		if !valid(f.Vendor, m) {
			return errors.New("models")
		}
		if !slices.Contains(models, m) {
			models = append(models, m)
		}
	}
	f.Models = models
	b, _ := json.Marshal(models)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if f.Active {
		rows, err := tx.QueryContext(ctx, `SELECT `+cols+` FROM firmware WHERE vendor = ? AND active = 1 AND id != ?`, f.Vendor, f.ID)
		if err != nil {
			return err
		}
		var others []Firmware
		for rows.Next() {
			if o, err := scan(rows); err == nil {
				others = append(others, *o)
			}
		}
		rows.Close()
		for _, o := range others {
			left := slices.DeleteFunc(slices.Clone(o.Models), func(m string) bool { return slices.Contains(models, m) })
			if len(left) == len(o.Models) {
				continue
			}
			ob, _ := json.Marshal(left)
			if _, err := tx.ExecContext(ctx, `UPDATE firmware SET models = ?, active = ? WHERE id = ?`, string(ob), b2i(len(left) > 0), o.ID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE firmware SET version = ?, models = ?, active = ? WHERE id = ?`, f.Version, string(b), b2i(f.Active), f.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Delete removes a firmware and its files.
func (s *Store) Delete(ctx context.Context, id int64) error {
	f, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM firmware WHERE id = ?`, id); err != nil {
		return err
	}
	if reToken.MatchString(f.Token) {
		return os.RemoveAll(filepath.Join(s.Dir, f.Token))
	}
	return nil
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Add stores an uploaded file. A .zip is unpacked (the firmware bundles of Poly and Cisco hold many
// files); its main file is the .loads file (Cisco), sip.ld (Poly) or else the largest file.
func (s *Store) Add(ctx context.Context, vendor, name string, r io.Reader) (*Firmware, error) {
	name = CleanName(name)
	if name == "" {
		return nil, errors.New("filename")
	}
	tok := newToken()
	dir := filepath.Join(s.Dir, tok)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	tmp := filepath.Join(dir, ".upload")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, MaxSize+1))
	out.Close()
	if err != nil {
		return nil, err
	}
	if n > MaxSize {
		return nil, errors.New("too_large")
	}
	if n == 0 {
		return nil, errors.New("empty")
	}
	f := &Firmware{Vendor: vendor, Models: []string{}, Filename: name, Token: tok, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		main, size, err := unzip(tmp, dir)
		if err != nil {
			return nil, err
		}
		_ = os.Remove(tmp)
		f.Filename, f.Size = main, size
		if b, err := os.ReadFile(filepath.Join(dir, main)); err == nil {
			sum := sha256.Sum256(b)
			f.SHA256 = hex.EncodeToString(sum[:])
		}
	} else if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO firmware(vendor, models, version, filename, token, size, sha256) VALUES (?, '[]', '', ?, ?, ?, ?)`,
		f.Vendor, f.Filename, f.Token, f.Size, f.SHA256)
	if err != nil {
		return nil, err
	}
	f.ID, _ = res.LastInsertId()
	ok = true
	return s.Get(ctx, f.ID)
}

// unzip extracts a bundle flat into dir (sub folders are dropped) and returns the main file.
func unzip(path, dir string) (string, int64, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", 0, errors.New("zip")
	}
	defer zr.Close()
	var total int64
	type entry struct {
		name string
		size int64
	}
	var files []entry
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(strings.ReplaceAll(zf.Name, "\\", "/"))
		if strings.HasPrefix(base, ".") || strings.HasPrefix(zf.Name, "__MACOSX") {
			continue
		}
		// Vendor bundles hold names like "Release Notes.pdf": store them under a safe name.
		name := CleanName(base)
		if name == "" {
			return "", 0, fmt.Errorf("zip: file name %q", base)
		}
		if len(files) >= 200 {
			return "", 0, errors.New("zip: too many files")
		}
		rc, err := zf.Open()
		if err != nil {
			return "", 0, errors.New("zip")
		}
		dst, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
		if err != nil {
			rc.Close()
			return "", 0, err
		}
		// Limit what is written, whatever the header claims (zip bombs).
		n, err := io.Copy(dst, io.LimitReader(rc, MaxSize-total+1))
		rc.Close()
		dst.Close()
		if err != nil {
			return "", 0, errors.New("zip")
		}
		total += n
		if total > MaxSize {
			return "", 0, errors.New("too_large")
		}
		files = append(files, entry{name, n})
	}
	if len(files) == 0 {
		return "", 0, errors.New("empty")
	}
	pick := func(match func(string) bool) (entry, bool) {
		for _, f := range files {
			if match(strings.ToLower(f.name)) {
				return f, true
			}
		}
		return entry{}, false
	}
	if f, ok := pick(func(n string) bool { return strings.HasSuffix(n, ".loads") }); ok {
		return f.name, f.size, nil
	}
	if f, ok := pick(func(n string) bool { return n == "sip.ld" }); ok {
		return f.name, f.size, nil
	}
	best := files[0]
	for _, f := range files {
		if f.size > best.size {
			best = f
		}
	}
	return best.name, best.size, nil
}
