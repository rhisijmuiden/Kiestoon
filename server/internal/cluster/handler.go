package cluster

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// PrimaryHandler serves /api/cluster/v1/ on the primary: the standby fetches snapshots and files
// here, through the tunnel, with the shared secret.
type PrimaryHandler struct {
	DataDir string
	Snap    *Snapshotter
	// Secret returns the shared secret ("" when no standby is paired).
	Secret func() string
	// Allowed checks the source address (the standby's tunnel address).
	AllowedFrom func(r *http.Request) bool
	// OnHeartbeat records the standby's state and returns the answer.
	OnHeartbeat func(Heartbeat) PrimaryInfo
}

func (h *PrimaryHandler) authorized(r *http.Request) bool {
	secret := h.Secret()
	got := r.Header.Get("Authorization")
	if secret == "" || len(got) < 8 || subtle.ConstantTimeCompare([]byte(got[7:]), []byte(secret)) != 1 {
		return false
	}
	return h.AllowedFrom == nil || h.AllowedFrom(r)
}

func (h *PrimaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/api/cluster/v1/heartbeat":
		var hb Heartbeat
		if r.Method != http.MethodPost || json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&hb) != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.OnHeartbeat(hb))
	case "/api/cluster/v1/snapshot":
		path := SnapshotPath(h.DataDir)
		m, ok := LoadMeta(path)
		f, err := os.Open(path)
		if !ok || err != nil {
			http.Error(w, `{"error":"no_snapshot"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		// The snapshot is replaced by renaming, so the open file stays consistent with its meta only
		// when the sizes agree; otherwise the standby retries.
		if st, err := f.Stat(); err != nil || st.Size() != m.Size {
			http.Error(w, `{"error":"snapshot_changing"}`, http.StatusConflict)
			return
		}
		b, _ := json.Marshal(m)
		w.Header().Set("X-Kiestoon-Snapshot", string(b))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(m.Size, 10))
		_, _ = io.CopyN(w, f, m.Size)
	case "/api/cluster/v1/files":
		list, err := Manifest(h.DataDir)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	case "/api/cluster/v1/file":
		rel := r.URL.Query().Get("path")
		if !Allowed(rel) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		f, err := os.Open(filepath.Join(h.DataDir, filepath.FromSlash(rel)))
		if err != nil {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
		_, _ = io.CopyN(w, f, st.Size())
	default:
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
	}
}
