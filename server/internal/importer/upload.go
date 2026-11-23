package importer

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Limits on what is read from an upload.
const (
	MaxUpload    = 2 << 30   // the uploaded file(s)
	MaxFile      = 256 << 20 // one file taken from an archive (database dumps)
	MaxAudioFile = 32 << 20
	MaxTotal     = 1 << 30 // everything taken from one upload
	maxEntries   = 200000
)

// Entry is a file in an upload.
type Entry struct {
	Name string // slash separated, without a leading "./" or "/"
	Size int64
}

// Upload is what the administrator uploaded: one file (an archive or a single configuration
// file) or several loose files. Archives (zip, tar, tar.gz, tar.bz2) are read without unpacking
// them on disk.
type Upload struct {
	// Name is the original file name (of the first file when there are several).
	Name string
	// Path is the file on disk, or a directory holding several loose files.
	Path string

	entries []Entry
	kind    string // dir | zip | tar | tgz | tbz | gz | file
}

// NewUpload describes a file or a directory of files on disk.
func NewUpload(name, p string) (*Upload, error) {
	u := &Upload{Name: name, Path: p}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		u.kind = "dir"
		return u, nil
	}
	head := u.Head(512)
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")):
		u.kind = "zip"
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		u.kind = "gz"
		if r, err := u.open(); err == nil {
			defer r.Close()
			if gz, err := gzip.NewReader(r); err == nil {
				var b [512]byte
				n, _ := io.ReadFull(gz, b[:])
				if isTar(b[:n]) {
					u.kind = "tgz"
				}
			}
		}
	case bytes.HasPrefix(head, []byte("BZh")):
		u.kind = "tbz"
	case isTar(head):
		u.kind = "tar"
	default:
		u.kind = "file"
	}
	return u, nil
}

func isTar(b []byte) bool { return len(b) >= 262 && string(b[257:262]) == "ustar" }

// IsArchive reports whether the upload holds several files.
func (u *Upload) IsArchive() bool { return u.kind != "file" && u.kind != "gz" }

func (u *Upload) open() (*os.File, error) { return os.Open(u.Path) }

// Open opens the uploaded file itself (for formats that are not an archive, such as an
// encrypted backup).
func (u *Upload) Open() (io.ReadCloser, error) {
	if u.kind == "dir" {
		return nil, errors.New("upload is a directory")
	}
	return u.open()
}

// Head returns the first n bytes of the uploaded file (nil for loose files).
func (u *Upload) Head(n int) []byte {
	if u.kind == "dir" {
		return nil
	}
	f, err := u.open()
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, n)
	m, _ := io.ReadFull(f, b)
	return b[:m]
}

func cleanName(n string) string {
	n = strings.ReplaceAll(n, "\\", "/")
	n = path.Clean("/" + n)
	return strings.TrimPrefix(n, "/")
}

// walk calls fn for every regular file; fn reads at most the size it needs from r.
func (u *Upload) walk(fn func(name string, size int64, r io.Reader) error) error {
	switch u.kind {
	case "dir":
		return filepath.WalkDir(u.Path, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			rel, _ := filepath.Rel(u.Path, p)
			st, err := d.Info()
			if err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			return fn(cleanName(filepath.ToSlash(rel)), st.Size(), f)
		})
	case "zip":
		zr, err := zip.OpenReader(u.Path)
		if err != nil {
			return ErrDamaged
		}
		defer zr.Close()
		for i, f := range zr.File {
			if i > maxEntries {
				return ErrTooLarge
			}
			if !f.Mode().IsRegular() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return ErrDamaged
			}
			err = fn(cleanName(f.Name), int64(f.UncompressedSize64), rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	case "file", "gz":
		f, err := u.open()
		if err != nil {
			return err
		}
		defer f.Close()
		st, _ := f.Stat()
		var r io.Reader = f
		name := u.Name
		if u.kind == "gz" {
			gz, err := gzip.NewReader(f)
			if err != nil {
				return ErrDamaged
			}
			r, name = gz, strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".GZ")
		}
		return fn(cleanName(path.Base(strings.ReplaceAll(name, "\\", "/"))), st.Size(), r)
	}
	f, err := u.open()
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = bufio.NewReader(f)
	switch u.kind {
	case "tgz":
		gz, err := gzip.NewReader(r)
		if err != nil {
			return ErrDamaged
		}
		r = gz
	case "tbz":
		r = bzip2.NewReader(r)
	}
	tr := tar.NewReader(r)
	for i := 0; ; i++ {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return ErrDamaged
		}
		if i > maxEntries {
			return ErrTooLarge
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			continue
		}
		if err := fn(cleanName(h.Name), h.Size, tr); err != nil {
			return err
		}
	}
}

// Entries lists the files in the upload.
func (u *Upload) Entries() []Entry {
	if u.entries != nil {
		return u.entries
	}
	u.entries = []Entry{}
	_ = u.walk(func(name string, size int64, _ io.Reader) error {
		u.entries = append(u.entries, Entry{Name: name, Size: size})
		return nil
	})
	return u.entries
}

// Has reports whether a file whose name ends in suffix (case-insensitive, matched at a path
// boundary) is in the upload.
func (u *Upload) Has(suffix string) bool { return u.Find(suffix) != "" }

// Find returns the name of the shortest file whose name ends in suffix at a path boundary, "" when none.
func (u *Upload) Find(suffix string) string {
	suffix = strings.ToLower(suffix)
	best := ""
	for _, e := range u.Entries() {
		n := strings.ToLower(e.Name)
		if n == suffix || strings.HasSuffix(n, "/"+suffix) {
			if best == "" || len(e.Name) < len(best) {
				best = e.Name
			}
		}
	}
	return best
}

// Files reads the files for which keep returns true, up to max bytes per file (larger files
// are skipped) and MaxTotal in all.
func (u *Upload) Files(keep func(name string, size int64) bool, max int64) (map[string][]byte, error) {
	out := map[string][]byte{}
	var total int64
	err := u.walk(func(name string, size int64, r io.Reader) error {
		if !keep(name, size) || size > max {
			return nil
		}
		b, err := io.ReadAll(io.LimitReader(r, max+1))
		if err != nil {
			return ErrDamaged
		}
		if int64(len(b)) > max {
			return nil
		}
		total += int64(len(b))
		if total > MaxTotal {
			return ErrTooLarge
		}
		out[name] = b
		return nil
	})
	return out, err
}

// ReadFile reads one file of the upload.
func (u *Upload) ReadFile(name string) ([]byte, error) {
	m, err := u.Files(func(n string, _ int64) bool { return n == name }, MaxFile)
	if err != nil {
		return nil, err
	}
	b, ok := m[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

// Nested returns an archive inside the upload as an upload of its own (for backups that hold
// another archive). The caller removes it with Close.
func (u *Upload) Nested(name string) (*Upload, error) {
	b, err := u.ReadFile(name)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "kt-import-nested-")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	f.Close()
	return NewUpload(path.Base(name), f.Name())
}

// Close removes a nested upload from disk.
func (u *Upload) Close() error { return os.Remove(u.Path) }

// AudioExts are the audio file types adapters take from an upload.
var AudioExts = []string{".wav", ".mp3", ".gsm", ".ulaw", ".ul", ".alaw", ".al", ".sln", ".sln16", ".g722", ".ogg", ".flac", ".WAV"}

// IsAudio reports whether the file name has an audio extension.
func IsAudio(name string) bool {
	return slices.Contains(AudioExts, strings.ToLower(path.Ext(name)))
}
