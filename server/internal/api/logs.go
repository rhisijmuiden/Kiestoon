package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/logs"
)

// logSource is one log the viewer can show: an Asterisk log file, or a service's journal
// (read through the root helper, because the portal may not read the journal itself).
type logSource struct {
	ID   string
	File string // in the Asterisk log directory
	Unit string // systemd unit without .service
}

var logSources = []logSource{
	{ID: "asterisk", File: "messages.log"},
	{ID: "security", File: "security.log"},
	{ID: "portal", Unit: "kiestoon"},
	{ID: "helper", Unit: "kiestoon-helper"},
	{ID: "engine", Unit: "asterisk"},
}

func findLogSource(id string) (logSource, bool) {
	for _, s := range logSources {
		if s.ID == id {
			return s, true
		}
	}
	return logSource{}, false
}

func (s *Server) logRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/logs/sources", admin(s.logSourcesList))
	mux.Handle("GET /api/logs", admin(s.logRead))
	mux.Handle("GET /api/logs/download", admin(s.logDownload))
	mux.Handle("GET /api/pbx/trunks/{id}/diagnose", admin(s.trunkDiagnose))
}

// readLog returns the lines of a source, oldest first, and the parser for them.
func (s *Server) readLog(ctx context.Context, src logSource) ([]string, func(string) logs.Entry, error) {
	if src.File != "" {
		lines, err := logs.TailLines(filepath.Join(s.Cfg.AsteriskLogDir, src.File))
		return lines, logs.ParseAsterisk, err
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var lines []string
	err := s.Helper.Call(cctx, "logs.journal", helper.JournalArgs{Unit: src.Unit, Lines: 3000}, &lines)
	return lines, logs.ParseJournal, err
}

func (s *Server) logSourcesList(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Available bool   `json:"available"`
		Size      int64  `json:"size,omitempty"`
		Modified  string `json:"modified,omitempty"`
	}
	helperOK := s.Helper.Available(r.Context())
	out := []item{}
	for _, src := range logSources {
		it := item{ID: src.ID, Kind: "journal", Available: helperOK}
		if src.File != "" {
			it.Kind = "file"
			if st, err := os.Stat(filepath.Join(s.Cfg.AsteriskLogDir, src.File)); err == nil {
				it.Available, it.Size, it.Modified = true, st.Size(), st.ModTime().UTC().Format(time.RFC3339)
			} else {
				it.Available = false
			}
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) logRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	src, ok := findLogSource(q.Get("source"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	lines, parse, err := s.readLog(r.Context(), src)
	if err != nil {
		s.logReadErr(w, err)
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	entries := logs.Filter(lines, parse, logs.Query{Q: q.Get("q"), Level: q.Get("level"), Limit: limit})
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "scanned": len(lines)})
}

func (s *Server) logReadErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		writeErr(w, http.StatusNotFound, "log_missing")
	case errors.Is(err, os.ErrPermission):
		writeErr(w, http.StatusForbidden, "log_permission")
	case errors.Is(err, helper.ErrUnavailable):
		writeErr(w, http.StatusServiceUnavailable, "helper_unavailable")
	default:
		s.Log.Warn("log read failed", "err", err)
		writeErr(w, http.StatusBadGateway, "log_unavailable")
	}
}

func (s *Server) logDownload(w http.ResponseWriter, r *http.Request) {
	src, ok := findLogSource(r.URL.Query().Get("source"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	name := fmt.Sprintf("kiestoon-%s-%s.log", src.ID, time.Now().Format("20060102-1504"))
	if src.File != "" {
		f, err := os.Open(filepath.Join(s.Cfg.AsteriskLogDir, src.File))
		if err != nil {
			s.logReadErr(w, err)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeContent(w, r, name, st.ModTime(), f)
		return
	}
	lines, _, err := s.readLog(r.Context(), src)
	if err != nil {
		s.logReadErr(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

// trunkDiagnose explains from the Asterisk log why a trunk is not registered or not reachable.
func (s *Server) trunkDiagnose(w http.ResponseWriter, r *http.Request) {
	t, err := s.PBX.Trunk(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	src, _ := findLogSource("asterisk")
	lines, parse, err := s.readLog(r.Context(), src)
	if err != nil {
		s.logReadErr(w, err)
		return
	}
	entries := logs.Filter(lines, parse, logs.Query{Limit: logs.MaxLimit})
	d := logs.Diagnose(entries, logs.TrunkMatch{Endpoint: t.Endpoint(), Host: t.Host, Username: t.Username})
	writeJSON(w, http.StatusOK, d)
}
