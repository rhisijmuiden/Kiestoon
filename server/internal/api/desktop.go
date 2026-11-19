package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

// The Windows softphone (Kiestoon Softphone) is a small desktop shell that shows the /desktop
// pages of this portal. It links itself to an account in one of two ways:
//
//   - pairing: a signed-in user clicks "Link softphone" in the portal (any sign-in method: password,
//     MFA, passkey or SSO). The portal makes a one-time code and opens kiestoon://pair?…; the
//     softphone trades the code for a desktop session.
//   - signing in inside the softphone with a password (and MFA), after which the softphone swaps the
//     normal session for a desktop session.
//
// A desktop session lasts 30 days and is extended every day it is used. It may only use the user
// portal API (its own phone, colleagues, queues, voicemail, history), never the admin API, so a
// long-lived session on a desk computer cannot change the phone system.

// pairTTL is how long a pairing code can be used.
const pairTTL = 10 * time.Minute

// pairAlphabet leaves out characters that are easy to mix up (0/O, 1/I/L).
const pairAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

type pairCode struct {
	userID  int64
	expires time.Time
}

type desktopPairs struct {
	mu sync.Mutex
	m  map[string]pairCode
}

func (p *desktopPairs) add(code string, c pairCode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]pairCode{}
	}
	now := time.Now()
	for k, v := range p.m {
		if now.After(v.expires) {
			delete(p.m, k)
		}
	}
	p.m[code] = c
}

// take returns and forgets a code: every code works once.
func (p *desktopPairs) take(code string) (pairCode, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.m[code]
	if ok {
		delete(p.m, code)
	}
	if !ok || time.Now().After(c.expires) {
		return pairCode{}, false
	}
	return c, true
}

func newPairCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 0, 9)
	for i, v := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, pairAlphabet[int(v)%len(pairAlphabet)])
	}
	return string(out)
}

// normalizePairCode accepts the code with or without dash, in any case and with spaces.
func normalizePairCode(c string) string {
	c = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(c))
	if len(c) != 8 {
		return ""
	}
	for _, r := range c {
		if !strings.ContainsRune(pairAlphabet, r) {
			return ""
		}
	}
	return c[:4] + "-" + c[4:]
}

// desktopAllowed lists what a desktop session may call: the user portal and nothing else.
func desktopAllowed(r *http.Request) bool {
	p := r.URL.Path
	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	switch {
	case strings.HasPrefix(p, "/api/portal/"), strings.HasPrefix(p, "/api/desktop/"), p == "/api/webrtc/ws":
		return true
	case p == "/api/me", p == "/api/auth/logout", p == "/api/account/language", p == "/api/account/theme":
		return true
	case get && (p == "/api/phonebook" || p == "/api/system/info"):
		return true
	}
	return false
}

func (s *Server) desktopRoutes(mux *http.ServeMux, admin, full func(http.HandlerFunc) http.Handler) {
	mux.HandleFunc("GET /api/desktop/info", s.desktopInfo)
	mux.Handle("GET /api/desktop/clients", admin(s.desktopClients))
	mux.Handle("DELETE /api/desktop/clients/{id}", admin(s.desktopClientDelete))
	mux.HandleFunc("POST /api/desktop/claim", s.desktopClaim)
	mux.Handle("POST /api/desktop/pair", full(s.desktopPair))
	mux.Handle("POST /api/desktop/session", full(s.desktopSession))
}

// installer is the newest MSI in the softphone directory.
func (s *Server) installer() (path string, info os.FileInfo) {
	files, _ := filepath.Glob(filepath.Join(s.Cfg.SoftphoneDir, "*.msi"))
	sort.Strings(files)
	for i := len(files) - 1; i >= 0; i-- {
		if st, err := os.Stat(files[i]); err == nil && st.Mode().IsRegular() {
			return files[i], st
		}
	}
	return "", nil
}

type installerInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

var shaCache sync.Map // path+modtime -> hex

func fileSHA256(path string, st os.FileInfo) string {
	key := path + "|" + st.ModTime().String()
	if v, ok := shaCache.Load(key); ok {
		return v.(string)
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	sum := hex.EncodeToString(h.Sum(nil))
	shaCache.Store(key, sum)
	return sum
}

// desktopInfo is public: the softphone asks it before anyone has signed in, and the portal shows
// the download.
func (s *Server) desktopInfo(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"version": version.Version, "installer": nil}
	if p, st := s.installer(); p != "" {
		out["installer"] = installerInfo{Name: filepath.Base(p), Size: st.Size(), SHA256: fileSHA256(p, st), URL: "/download/kiestoon-softphone.msi"}
	}
	if base, fp, err := s.desktopURL(r); err == nil && !strings.HasSuffix(base, ":0") {
		out["server"] = base
		out["fingerprint"] = fp
	}
	writeJSON(w, http.StatusOK, out)
}

// SoftphoneDownload serves the installer at /download/kiestoon-softphone.msi.
func (s *Server) SoftphoneDownload() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/kiestoon-softphone.msi" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		p, st := s.installer()
		if p == "" {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/x-msi")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(p)+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
	})
}

// desktopPair makes a one-time code for the signed-in user and the link that opens the softphone.
func (s *Server) desktopPair(w http.ResponseWriter, r *http.Request) {
	if s.myExtension(w, r) == nil {
		return
	}
	u := sessionFrom(r).User
	code := newPairCode()
	exp := time.Now().Add(pairTTL)
	s.pairs.add(code, pairCode{userID: u.ID, expires: exp})
	base, fp, err := s.desktopURL(r)
	if err != nil || strings.HasSuffix(base, ":0") {
		// No HTTPS port of its own (switched off): use the address this page was opened on.
		base, fp = s.portalBase(r), ""
	}
	q := url.Values{"server": {base}, "code": {code}}
	if fp != "" {
		q.Set("fp", fp)
	}
	s.audit(r, "desktop.pair_code", "")
	writeJSON(w, http.StatusOK, map[string]any{
		"code": code, "server": base, "fingerprint": fp, "expiresAt": exp.UTC().Format(time.RFC3339),
		"link": "kiestoon://pair?" + q.Encode(),
	})
}

// desktopClaim trades a pairing code for a desktop session.
func (s *Server) desktopClaim(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	pc, ok := s.pairs.take(normalizePairCode(req.Code))
	if !ok {
		s.loginLimit.Fail(ip)
		writeErr(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	u, err := s.Auth.UserByID(r.Context(), pc.userID)
	if err != nil || !u.Enabled {
		writeErr(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	s.startDesktop(w, r, u)
}

// desktopSession swaps the session the softphone signed in with for a desktop session.
func (s *Server) desktopSession(w http.ResponseWriter, r *http.Request) {
	// Only the softphone asks for this; a cross-site page cannot set the header without a preflight.
	if r.Header.Get("X-Kiestoon-Desktop") != "1" {
		writeErr(w, http.StatusForbidden, "desktop_only")
		return
	}
	sess := sessionFrom(r)
	if sess.Method == auth.MethodDesktop {
		s.ensureWebRTC(w, r, sess.User)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if strings.HasPrefix(sess.Method, "token") {
		writeErr(w, http.StatusForbidden, "desktop_only")
		return
	}
	_ = s.Auth.DeleteSession(r.Context(), sess)
	s.startDesktop(w, r, sess.User)
}

func (s *Server) startDesktop(w http.ResponseWriter, r *http.Request, u *auth.User) {
	if u.ExtensionID == nil {
		writeErr(w, http.StatusConflict, "no_extension")
		return
	}
	token, err := s.Auth.CreateSessionMethod(r.Context(), u, true, auth.MethodDesktop)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.setCookieTTL(w, r, token, auth.DesktopTTL)
	s.loginLimit.Reset(clientIP(r))
	r = r.WithContext(contextWithSession(r, &auth.Session{User: u, MFAPassed: true, Method: auth.MethodDesktop}))
	s.audit(r, "desktop.linked", "")
	if !s.ensureWebRTC(w, r, u) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ensureWebRTC switches the softphone line on for the user's extension: that is the account the
// Windows softphone registers with. It reports false after writing an error.
func (s *Server) ensureWebRTC(w http.ResponseWriter, r *http.Request, u *auth.User) bool {
	if u.ExtensionID == nil {
		writeErr(w, http.StatusConflict, "no_extension")
		return false
	}
	e, err := s.PBX.Extension(r.Context(), *u.ExtensionID)
	if err != nil {
		writeErr(w, http.StatusConflict, "no_extension")
		return false
	}
	if e.WebRTC {
		return true
	}
	e.WebRTC = true
	if err := s.PBX.SaveExtension(r.Context(), s.plan(r.Context()), e); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return false
	}
	s.Ast.Changed()
	s.audit(r, "desktop.webrtc_enabled", e.Number)
	return true
}

// desktopURL is the address the softphone links to: the employee portal address when it is HTTPS,
// otherwise the address other systems pair with.
func (s *Server) desktopURL(r *http.Request) (string, string, error) {
	if p, _ := store.GetSetting(r.Context(), s.DB, portalURLKey, ""); strings.HasPrefix(p, "https://") {
		return strings.TrimRight(p, "/"), "", nil
	}
	return s.portalURL(r, "")
}

// ---- admin console ----------------------------------------------------------------------------

type desktopClient struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Extension   string `json:"extension"`
	LinkedAt    string `json:"linkedAt"`
	ExpiresAt   string `json:"expiresAt"`
}

// desktopClients lists the linked softphones: the desktop sessions that have not expired.
func (s *Server) desktopClients(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `
		SELECT s.rowid, u.id, u.username, u.display_name, COALESCE(e.number, ''), s.created_at, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id LEFT JOIN extensions e ON e.id = u.extension_id
		WHERE s.method = ? AND s.expires_at > ? ORDER BY u.username, s.created_at`,
		auth.MethodDesktop, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	out := []desktopClient{}
	for rows.Next() {
		var c desktopClient
		if err := rows.Scan(&c.ID, &c.UserID, &c.Username, &c.DisplayName, &c.Extension, &c.LinkedAt, &c.ExpiresAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

// desktopClientDelete unlinks one softphone: its session ends and it shows the link screen again.
func (s *Server) desktopClientDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM sessions WHERE rowid = ? AND method = ?`, id, auth.MethodDesktop)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "desktop.client_removed", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
