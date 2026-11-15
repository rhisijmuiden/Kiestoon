package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Kiestoon can be reached on two host names: the PBX address (public_url: administration, phones,
// provisioning, SIP) and the employee portal address (portal_url: welcome mails, set-password links,
// softphone pairing). Both are written by internet mode (Caddy) or the own reverse proxy settings.
// When the portal address is set and "admin_on_portal" is "blocked", nobody gets administrator rights
// on a request that arrives on the portal host name.

const (
	portalURLKey     = "portal_url"
	adminOnPortalKey = "admin_on_portal"
)

// Hostnames is the effective address set, as the UI shows it.
type Hostnames struct {
	PBXURL      string `json:"pbxUrl"`
	PortalURL   string `json:"portalUrl"`
	BlockAdmin  bool   `json:"blockAdmin"`
	PBXHost     string `json:"pbxHost"`
	PortalHost  string `json:"portalHost"`
	Separate    bool   `json:"separate"`
	RequestHost string `json:"requestHost"`
	OnPortal    bool   `json:"onPortal"`
}

type hostState struct {
	pbx, portal string // host names, lower case, without port
	block       bool
}

// hosts caches the host names: the admin check runs on every request.
var hosts atomic.Pointer[hostState]

func urlHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

func requestHost(r *http.Request) string {
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
}

// loadHostnames refreshes the cache from the settings.
func (s *Server) loadHostnames(ctx context.Context) {
	pbxURL, _ := store.GetSetting(ctx, s.DB, "public_url", "")
	portalURL, _ := store.GetSetting(ctx, s.DB, portalURLKey, "")
	block, _ := store.GetSetting(ctx, s.DB, adminOnPortalKey, "")
	hosts.Store(&hostState{pbx: urlHost(pbxURL), portal: urlHost(portalURL), block: block == "blocked"})
}

func currentHosts() hostState {
	if h := hosts.Load(); h != nil {
		return *h
	}
	return hostState{}
}

// onPortalHost reports whether the request came in on the employee portal host name, and that host
// name differs from the PBX address.
func onPortalHost(r *http.Request) bool {
	h := currentHosts()
	return h.portal != "" && h.portal != h.pbx && requestHost(r) == h.portal
}

// adminBlockedHere reports whether administrator rights are switched off for this request.
func adminBlockedHere(r *http.Request) bool {
	return currentHosts().block && onPortalHost(r)
}

// hostUser is the user as this request may act: on the portal host name with administration blocked,
// administrators are employees (reception rights at most). The stored account is not changed.
func hostUser(r *http.Request, u *auth.User) *auth.User {
	if u == nil || !adminBlockedHere(r) || !u.IsAdmin() {
		return u
	}
	c := *u
	c.Role = "reception"
	return &c
}

// pbxBase is the PBX address for phones, provisioning and integrations.
func (s *Server) pbxBase(r *http.Request) string {
	if base, _ := store.GetSetting(r.Context(), s.DB, "public_url", ""); base != "" {
		return strings.TrimRight(base, "/")
	}
	return s.requestBase(r)
}

// portalBase is the employee portal address for links in mails and the softphone: the portal URL,
// then the PBX address, then the address of this request.
func (s *Server) portalBase(r *http.Request) string {
	if base, _ := store.GetSetting(r.Context(), s.DB, portalURLKey, ""); base != "" {
		return strings.TrimRight(base, "/")
	}
	return s.pbxBase(r)
}

func (s *Server) requestBase(r *http.Request) string {
	scheme := "http"
	if s.secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// sipHost is the host name phones and softphones register with: the PBX host name when one is set,
// otherwise the host of this request.
func (s *Server) sipHost(r *http.Request) string {
	if h := currentHosts().pbx; h != "" {
		return h
	}
	return requestHost(r)
}

func (s *Server) hostnames(r *http.Request) Hostnames {
	pbxURL, _ := store.GetSetting(r.Context(), s.DB, "public_url", "")
	portalURL, _ := store.GetSetting(r.Context(), s.DB, portalURLKey, "")
	h := currentHosts()
	return Hostnames{
		PBXURL: pbxURL, PortalURL: portalURL, BlockAdmin: h.block, PBXHost: h.pbx, PortalHost: h.portal,
		Separate: h.portal != "" && h.portal != h.pbx, RequestHost: requestHost(r), OnPortal: onPortalHost(r),
	}
}

func (s *Server) hostnamesGet(w http.ResponseWriter, r *http.Request) {
	out := s.hostnames(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"hostnames": out,
		// Identity providers need the callback of every host name people sign in on.
		"ssoCallbacks": ssoCallbacks(out),
	})
}

func ssoCallbacks(h Hostnames) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, b := range []string{h.PBXURL, h.PortalURL} {
		if b = strings.TrimRight(b, "/"); b != "" && !seen[b] {
			seen[b] = true
			out = append(out, b+"/api/auth/sso/callback")
		}
	}
	return out
}

func (s *Server) hostnamesSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BlockAdmin bool `json:"blockAdmin"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	h := currentHosts()
	// Blocking on the host the administrator is using right now would lock them out mid-session.
	if req.BlockAdmin && h.portal != "" && requestHost(r) == h.portal && h.portal != h.pbx {
		writeErr(w, http.StatusConflict, "on_portal_host")
		return
	}
	v := ""
	if req.BlockAdmin {
		v = "blocked"
	}
	if err := store.SetSetting(r.Context(), s.DB, adminOnPortalKey, v); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.loadHostnames(r.Context())
	s.audit(r, "hostnames.saved", "blockAdmin="+boolString(req.BlockAdmin))
	s.hostnamesGet(w, r)
}

// setPortalURL keeps portal_url in step with a source (internet mode or the proxy settings) the same
// way public_url is: a new value wins, clearing only removes the value that source wrote.
func (s *Server) setPortalURL(ctx context.Context, newURL, oldURL string) {
	cur, _ := store.GetSetting(ctx, s.DB, portalURLKey, "")
	switch {
	case newURL != "":
		_ = store.SetSetting(ctx, s.DB, portalURLKey, newURL)
	case oldURL != "" && cur == oldURL:
		_ = store.SetSetting(ctx, s.DB, portalURLKey, "")
	}
	s.loadHostnames(ctx)
}

// forwardedHost takes the host name from X-Forwarded-Host when a trusted proxy sent the request:
// some proxies rewrite Host to the upstream address.
func forwardedHost(r *http.Request) {
	fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	if fh == "" || strings.ContainsAny(fh, " /\\@") {
		return
	}
	a, ok := remoteAddr(r)
	if !ok || !isTrustedProxy(a) {
		return
	}
	r.Host = fh
}

// capSession applies hostUser to a session without touching the stored one.
func capSession(r *http.Request, sess *auth.Session) *auth.Session {
	if sess == nil {
		return sess
	}
	if u := hostUser(r, sess.User); u != sess.User {
		c := *sess
		c.User = u
		return &c
	}
	return sess
}
