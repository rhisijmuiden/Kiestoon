package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"

	// The adapters register themselves.
	_ "github.com/rhisijmuiden/kiestoon/server/internal/importer/asterisk"
	_ "github.com/rhisijmuiden/kiestoon/server/internal/unifitalk"
)

// Importing another phone system: the administrator uploads a backup or configuration files
// (one or more requests to /api/import/upload), Kiestoon reads them (/read) and shows the plan;
// the browser converts the recordings to prompts and /apply creates what was ticked.

// importSession is one upload being imported. It lives in memory for importTTL; the files are
// on disk below DataDir/import.
type importSession struct {
	mu      sync.Mutex
	id      string
	dir     string
	files   []string // names of the uploaded files, in upload order
	bundle  *importer.Bundle
	plan    *importer.Plan
	touched time.Time
}

const (
	importTTL      = 2 * time.Hour
	maxImportFiles = 50
)

type importSessions struct {
	mu sync.Mutex
	m  map[string]*importSession
}

func (s *Server) importDir() string { return filepath.Join(s.Cfg.DataDir, "import") }

func (s *Server) importRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/import/systems", admin(s.importSystems))
	mux.Handle("POST /api/import/upload", admin(s.importUpload))
	mux.Handle("POST /api/import/{id}/read", admin(s.importRead))
	mux.Handle("GET /api/import/{id}/audio", admin(s.importAudio))
	mux.Handle("POST /api/import/{id}/apply", admin(s.importApply))
	mux.Handle("DELETE /api/import/{id}", admin(s.importDelete))
}

func (s *Server) importSession(id string) *importSession {
	s.imports.mu.Lock()
	defer s.imports.mu.Unlock()
	s.expireImports()
	is := s.imports.m[id]
	if is != nil {
		is.touched = time.Now()
	}
	return is
}

// expireImports removes sessions that were not used for importTTL (imports.mu held).
func (s *Server) expireImports() {
	for id, is := range s.imports.m {
		if time.Since(is.touched) > importTTL {
			os.RemoveAll(is.dir)
			delete(s.imports.m, id)
		}
	}
}

func (s *Server) dropImport(id string) {
	s.imports.mu.Lock()
	defer s.imports.mu.Unlock()
	if is := s.imports.m[id]; is != nil {
		os.RemoveAll(is.dir)
		delete(s.imports.m, id)
	}
}

func (s *Server) importSystems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"systems": importer.Adapters()})
}

var reImportName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func cleanImportName(n string) string {
	n = path.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Trim(reImportName.ReplaceAllString(n, "_"), "._")
	if len(n) > 100 {
		n = n[len(n)-100:]
	}
	if n == "" {
		n = "upload"
	}
	return n
}

// importUpload takes one file as the request body: POST /api/import/upload?name=…[&session=…].
// Several loose files (pjsip.conf, extensions.conf ...) are uploaded one by one into the same
// session.
func (s *Server) importUpload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := cleanImportName(q.Get("name"))
	var is *importSession
	if id := q.Get("session"); id != "" {
		if is = s.importSession(id); is == nil {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
	} else {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		is = &importSession{id: hex.EncodeToString(b), touched: time.Now()}
		is.dir = filepath.Join(s.importDir(), is.id)
		if err := os.MkdirAll(is.dir, 0o700); err != nil {
			code, status := uploadErrCode(err)
			writeErr(w, status, code)
			return
		}
		s.imports.mu.Lock()
		if s.imports.m == nil {
			s.imports.m = map[string]*importSession{}
		}
		s.imports.m[is.id] = is
		s.imports.mu.Unlock()
	}
	is.mu.Lock()
	defer is.mu.Unlock()
	if len(is.files) >= maxImportFiles {
		writeErr(w, http.StatusBadRequest, "too_many_files")
		return
	}
	for containsName(is.files, name) {
		name = "_" + name
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(60 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, importer.MaxUpload)
	f, err := os.OpenFile(filepath.Join(is.dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = io.Copy(f, r.Body)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		os.Remove(filepath.Join(is.dir, name))
		code, status := uploadErrCode(err)
		writeErr(w, status, code)
		return
	}
	is.files = append(is.files, name)
	is.bundle, is.plan = nil, nil
	writeJSON(w, http.StatusOK, map[string]any{"id": is.id, "files": is.files})
}

func containsName(list []string, n string) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// upload describes the files of the session: one file, or the directory of loose files.
func (is *importSession) upload() (*importer.Upload, error) {
	if len(is.files) == 1 {
		return importer.NewUpload(is.files[0], filepath.Join(is.dir, is.files[0]))
	}
	return importer.NewUpload(is.files[0], is.dir)
}

// importRead reads the upload and returns the plan: POST /api/import/{id}/read {"system": "auto"}.
func (s *Server) importRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		System string `json:"system"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	is := s.importSession(r.PathValue("id"))
	if is == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	is.mu.Lock()
	defer is.mu.Unlock()
	if len(is.files) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing_to_import")
		return
	}
	if req.System != "" && req.System != "auto" && importer.Find(req.System) == nil {
		fieldErr(w, "system")
		return
	}
	u, err := is.upload()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(11 * time.Minute))
	b, err := importer.Read(ctx, u, req.System)
	if err != nil {
		s.importErr(w, err)
		return
	}
	p, err := importer.Prepare(ctx, s.PBX, s.plan(ctx), b)
	if err != nil {
		s.Log.Error("import prepare", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	is.bundle, is.plan = b, p
	s.audit(r, "import.read", b.Info.System+" "+strings.Join(is.files, ", "))
	writeJSON(w, http.StatusOK, map[string]any{"id": is.id, "system": b.Info.System, "plan": p})
}

func (s *Server) importErr(w http.ResponseWriter, err error) {
	var ce interface{ Code() string }
	switch {
	case errors.As(err, &ce):
		writeErr(w, http.StatusBadRequest, ce.Code())
	case errors.Is(err, importer.ErrUnknownFormat), errors.Is(err, importer.ErrEmpty), errors.Is(err, importer.ErrDamaged),
		errors.Is(err, importer.ErrTooLarge):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		// Adapters return errors with a stable code as their text (e.g. not_unifi_backup).
		msg := err.Error()
		if regexp.MustCompile(`^[a-z_]{3,40}$`).MatchString(msg) {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		s.Log.Warn("import read", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "damaged_file", "message": msg})
	}
}

// importAudio sends one recording of the upload, for the browser to convert: GET …/audio?file=….
func (s *Server) importAudio(w http.ResponseWriter, r *http.Request) {
	is := s.importSession(r.PathValue("id"))
	if is == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	is.mu.Lock()
	b := is.bundle
	is.mu.Unlock()
	if b == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	data, ok := b.Audio[r.URL.Query().Get("file")]
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	ct := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(string(data), "RIFF"):
		ct = "audio/wav"
	case strings.HasPrefix(string(data), "ID3"), len(data) > 1 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		ct = "audio/mpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// importApply creates what was ticked: POST /api/import/{id}/apply with an importer.Choice.
func (s *Server) importApply(w http.ResponseWriter, r *http.Request) {
	var c importer.Choice
	if err := decode(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	is := s.importSession(r.PathValue("id"))
	if is == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	is.mu.Lock()
	defer is.mu.Unlock()
	if is.plan == nil {
		writeErr(w, http.StatusBadRequest, "not_read")
		return
	}
	if c.Prompts == nil {
		c.Prompts = map[string]int64{}
	}
	// Prompt ids must be prompts that exist (the browser made them from the recordings).
	for f, id := range c.Prompts {
		if _, err := s.PBX.Prompt(r.Context(), id); err != nil {
			delete(c.Prompts, f)
		}
	}
	res, err := importer.Apply(r.Context(), s.PBX, s.plan(r.Context()), is.bundle, is.plan, c)
	if len(res.Created) > 0 {
		s.Ast.Changed()
	}
	var fe *importer.FieldError
	if errors.As(err, &fe) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "import_item", "key": fe.Key, "field": fe.Field, "code": fe.Code, "result": res})
		return
	}
	if err != nil {
		s.Log.Error("import apply", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal", "result": res})
		return
	}
	s.audit(r, "import.applied", is.bundle.Info.System)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) importDelete(w http.ResponseWriter, r *http.Request) {
	s.dropImport(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
