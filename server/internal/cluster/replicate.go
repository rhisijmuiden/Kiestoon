package cluster

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the snapshot connection
)

// Replicated are the directories (inside the data directory) the standby keeps in sync. Generated
// configuration, the mail queue, update state and downloaded geo data are left out on purpose.
var Replicated = []string{"asterisk", "pbx/tls", "pbx/prompts", "fax", "speech", "firmware", "secrets"}

// Allowed reports whether a relative path may be served to and written by the standby.
func Allowed(rel string) bool {
	if rel == "" || strings.Contains(rel, "\\") || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean != rel || strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	for _, d := range Replicated {
		if strings.HasPrefix(clean, d+"/") {
			return !strings.HasSuffix(clean, ".tmp") && !strings.Contains(clean, "/.kt-")
		}
	}
	return false
}

// File is one entry of the file manifest.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Mod  int64  `json:"mod"` // unix nanoseconds
}

// Manifest lists the replicated files.
func Manifest(dataDir string) ([]File, error) {
	out := []File{}
	for _, d := range Replicated {
		root := filepath.Join(dataDir, d)
		err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return nil // unreadable directories are skipped, not fatal
			}
			if !e.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(dataDir, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !Allowed(rel) {
				return nil
			}
			info, err := e.Info()
			if err != nil {
				return nil
			}
			out = append(out, File{Path: rel, Size: info.Size(), Mod: info.ModTime().UnixNano()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ---- primary: snapshots ----------------------------------------------------------

// Snapshotter takes a consistent copy of the database whenever it changed (at most every few
// seconds). It uses its own read-only connection: VACUUM INTO reads a snapshot without blocking
// the portal's writes, and PRAGMA data_version on that connection shows commits of the others.
type Snapshotter struct {
	DataDir string
	DBPath  string
	Log     *slog.Logger

	mu      sync.Mutex
	db      *sql.DB
	version int64
	meta    Meta
	loaded  bool
}

func (s *Snapshotter) open() error {
	if s.db != nil {
		return nil
	}
	db, err := sql.Open("sqlite", "file:"+s.DBPath+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)
	s.db = db
	return nil
}

// Meta returns the last snapshot.
func (s *Snapshotter) Meta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMeta()
	return s.meta
}

func (s *Snapshotter) loadMeta() {
	if s.loaded {
		return
	}
	s.loaded = true
	if m, ok := LoadMeta(SnapshotPath(s.DataDir)); ok {
		s.meta = m
	}
}

// Take makes a snapshot if the database changed since the last one (or always with force).
func (s *Snapshotter) Take(ctx context.Context, force bool) (Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMeta()
	if err := s.open(); err != nil {
		return s.meta, err
	}
	var v int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&v); err != nil {
		return s.meta, err
	}
	role := LoadRole(s.DataDir)
	if !force && v == s.version && s.meta.Epoch == role.Epoch && s.meta.Seq > 0 {
		return s.meta, nil
	}
	path := SnapshotPath(s.DataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return s.meta, err
	}
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return s.meta, fmt.Errorf("snapshot: %w", err)
	}
	sum, size, err := fileSum(tmp)
	if err != nil {
		return s.meta, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return s.meta, err
	}
	m := Meta{Epoch: role.Epoch, Seq: 1, SHA256: sum, Size: size, At: time.Now().UTC()}
	if s.meta.Epoch == role.Epoch {
		m.Seq = s.meta.Seq + 1
	}
	if err := SaveMeta(path, m); err != nil {
		return s.meta, err
	}
	s.meta, s.version = m, v
	return m, nil
}

// Run takes snapshots until ctx ends.
func (s *Snapshotter) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if _, err := s.Take(ctx, false); err != nil && ctx.Err() == nil {
			s.Log.Warn("cluster snapshot", "err", err)
		}
		select {
		case <-ctx.Done():
			s.mu.Lock()
			if s.db != nil {
				s.db.Close()
				s.db = nil
			}
			s.mu.Unlock()
			return
		case <-t.C:
		}
	}
}

func fileSum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// ---- standby: replication ------------------------------------------------------

// Heartbeat is what the standby reports to the primary.
type Heartbeat struct {
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Replica      Meta      `json:"replica"`
	LastFileSync time.Time `json:"lastFileSync"`
	Files        int       `json:"files"`
}

// PrimaryInfo is the primary's answer to a heartbeat.
type PrimaryInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Snapshot Meta   `json:"snapshot"`
	// Takeover asks the standby to fetch everything now (planned switch-over).
	Takeover bool `json:"takeover"`
	// Promote tells the standby to take over now (switch-over without a virtual IP).
	Promote bool `json:"promote"`
	// Leave tells the standby that high availability was switched off.
	Leave bool `json:"leave"`
}

// ReplicaStatus is shown on the standby's status page and sent to the primary.
type ReplicaStatus struct {
	PrimaryReachable bool      `json:"primaryReachable"`
	PrimaryName      string    `json:"primaryName"`
	PrimaryVersion   string    `json:"primaryVersion"`
	LastContact      time.Time `json:"lastContact"`
	LastDBSync       time.Time `json:"lastDbSync"`
	LastFileSync     time.Time `json:"lastFileSync"`
	Files            int       `json:"files"`
	Replica          Meta      `json:"replica"`
	Primary          Meta      `json:"primary"`
	LastError        string    `json:"lastError"`
}

// Replicator runs on the standby and pulls from the primary through the tunnel.
type Replicator struct {
	DataDir string
	// BaseURL is the primary's portal through the tunnel, for example http://10.130.4.1:8080.
	BaseURL string
	Secret  string
	Name    string
	Version string
	Log     *slog.Logger
	Client  *http.Client
	// OnPromote is called when the primary hands over (manual mode).
	OnPromote func()
	// OnLeave is called when the primary switched high availability off.
	OnLeave func()

	mu sync.Mutex
	st ReplicaStatus
}

// Status returns the current replication state.
func (r *Replicator) Status() ReplicaStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.st
	if m, ok := LoadMeta(ReplicaPath(r.DataDir)); ok {
		st.Replica = m
	}
	return st
}

func (r *Replicator) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (r *Replicator) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, r.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	return resp, nil
}

// SyncNow fetches the newest database and the changed files once.
func (r *Replicator) SyncNow(ctx context.Context, files bool) error {
	info, err := r.heartbeat(ctx)
	r.mu.Lock()
	r.st.PrimaryReachable = err == nil
	if err != nil {
		r.st.LastError = err.Error()
		r.mu.Unlock()
		return err
	}
	r.st.LastContact = time.Now().UTC()
	r.st.PrimaryName, r.st.PrimaryVersion, r.st.Primary = info.Name, info.Version, info.Snapshot
	r.st.LastError = ""
	r.mu.Unlock()
	local, _ := LoadMeta(ReplicaPath(r.DataDir))
	if info.Snapshot.Seq > 0 && info.Snapshot.Newer(local) {
		if err := r.fetchDB(ctx, info.Snapshot); err != nil {
			r.setErr(err)
			return err
		}
	}
	if files || info.Takeover {
		if err := r.syncFiles(ctx); err != nil {
			r.setErr(err)
			return err
		}
	}
	if info.Leave && r.OnLeave != nil {
		r.OnLeave()
		return nil
	}
	if info.Promote && r.OnPromote != nil {
		r.OnPromote()
	}
	return nil
}

func (r *Replicator) setErr(err error) {
	r.mu.Lock()
	r.st.LastError = err.Error()
	r.mu.Unlock()
}

func (r *Replicator) heartbeat(ctx context.Context) (PrimaryInfo, error) {
	st := r.Status()
	hb := Heartbeat{Name: r.Name, Version: r.Version, Replica: st.Replica, LastFileSync: st.LastFileSync, Files: st.Files}
	b, _ := json.Marshal(hb)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := r.request(cctx, http.MethodPost, "/api/cluster/v1/heartbeat", strings.NewReader(string(b)))
	if err != nil {
		return PrimaryInfo{}, err
	}
	defer resp.Body.Close()
	var info PrimaryInfo
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info)
	return info, err
}

func (r *Replicator) fetchDB(ctx context.Context, want Meta) error {
	resp, err := r.request(ctx, http.MethodGet, "/api/cluster/v1/snapshot", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var got Meta
	if err := json.Unmarshal([]byte(resp.Header.Get("X-Kiestoon-Snapshot")), &got); err != nil {
		return errors.New("snapshot without description")
	}
	path := ReplicaPath(r.DataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != got.SHA256 || n != got.Size {
		os.Remove(tmp)
		return errors.New("snapshot checksum mismatch")
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := SaveMeta(path, got); err != nil {
		return err
	}
	r.mu.Lock()
	r.st.LastDBSync = time.Now().UTC()
	r.mu.Unlock()
	return nil
}

func (r *Replicator) syncFiles(ctx context.Context) error {
	resp, err := r.request(ctx, http.MethodGet, "/api/cluster/v1/files", nil)
	if err != nil {
		return err
	}
	var remote []File
	err = json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&remote)
	resp.Body.Close()
	if err != nil {
		return err
	}
	local, err := Manifest(r.DataDir)
	if err != nil {
		return err
	}
	have := map[string]File{}
	for _, f := range local {
		have[f.Path] = f
	}
	want := map[string]bool{}
	for _, f := range remote {
		if !Allowed(f.Path) {
			continue
		}
		want[f.Path] = true
		if l, ok := have[f.Path]; ok && l.Size == f.Size && l.Mod == f.Mod {
			continue
		}
		if err := r.fetchFile(ctx, f); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	for _, f := range local {
		if !want[f.Path] {
			_ = os.Remove(filepath.Join(r.DataDir, filepath.FromSlash(f.Path)))
		}
	}
	r.mu.Lock()
	r.st.LastFileSync = time.Now().UTC()
	r.st.Files = len(want)
	r.mu.Unlock()
	return nil
}

func (r *Replicator) fetchFile(ctx context.Context, f File) error {
	resp, err := r.request(ctx, http.MethodGet, "/api/cluster/v1/file?path="+url.QueryEscape(f.Path), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dst := filepath.Join(r.DataDir, filepath.FromSlash(f.Path))
	if err := os.MkdirAll(filepath.Dir(dst), 0o2770); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(dst), ".kt-"+filepath.Base(dst)+".tmp")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, resp.Body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != f.Size {
		err = errors.New("short file")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	mod := time.Unix(0, f.Mod)
	_ = os.Chtimes(tmp, mod, mod)
	return os.Rename(tmp, dst)
}

// Run replicates until ctx ends: the database every 5 seconds, files every 30 seconds.
func (r *Replicator) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	last := time.Time{}
	for {
		files := time.Since(last) > 30*time.Second
		if err := r.SyncNow(ctx, files); err != nil {
			if ctx.Err() != nil {
				return
			}
			r.Log.Debug("cluster replication", "err", err)
		} else if files {
			last = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
