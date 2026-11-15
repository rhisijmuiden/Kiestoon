package edge

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
)

//go:embed ui
var uiFiles embed.FS

// Handler serves the local status page, its small API and the provisioning proxy.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/prov/", a.ProvisioningProxy())
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.Status())
	})
	mux.HandleFunc("POST /api/pair", a.pairRequest)
	mux.HandleFunc("GET /rhis.svg", func(w http.ResponseWriter, r *http.Request) {
		b, _ := uiFiles.ReadFile("ui/rhis.svg")
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := uiFiles.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(b)
	})
	return mux
}

// pairRequest pairs from the local page. It only works while the edge is not paired, so somebody
// on the office network cannot move a working edge to another Kiestoon (resetting needs a shell).
func (a *Agent) pairRequest(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	err := a.Pair(r.Context(), req.Code)
	var apiErr *multisite.APIError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPaired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already_paired"})
	case errors.Is(err, multisite.ErrInvalidCode):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_code"})
	case errors.As(err, &apiErr):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": apiErr.Code})
	default:
		a.Log.Warn("edge: pairing", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unreachable", "detail": err.Error()})
	}
}

func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Server is the HTTP server of the edge (port 80: phones fetch their configuration here).
func (a *Agent) Server(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 5 * time.Minute, IdleTimeout: 120 * time.Second}
}
