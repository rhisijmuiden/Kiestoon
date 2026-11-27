package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
)

// OAuth 2.1 for MCP clients that sign in themselves (claude.ai and Claude Desktop custom connectors):
// discovery (RFC 9728 and RFC 8414), dynamic client registration (RFC 7591), the authorisation code
// flow with PKCE S256 only, rotating refresh tokens and revocation. Clients are public: they have no
// secret. The person signs in to the portal as usual (password + MFA, passkey or SSO) and chooses on
// the consent page what the assistant may do; that becomes one connection.

type oauthCode struct {
	clientID  string
	redirect  string
	challenge string
	userID    int64
	role      string
	scopes    []string
	days      int
	expires   time.Time
}

var oauthLimit = auth.NewLimiter(30, 10*time.Minute)

func scopeKeys() []string {
	out := []string{}
	for _, sc := range mcpScopes {
		out = append(out, sc.Key)
	}
	return out
}

func (s *Server) oauthResourceMeta(w http.ResponseWriter, r *http.Request) {
	base := s.requestBase(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 base + "/mcp",
		"authorization_servers":    []string{base},
		"scopes_supported":         scopeKeys(),
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Kiestoon",
	})
}

func (s *Server) oauthServerMeta(w http.ResponseWriter, r *http.Request) {
	base := s.requestBase(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"revocation_endpoint":                   base + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      scopeKeys(),
		"authorization_response_iss_parameter_supported": true,
	})
}

func oauthJSONErr(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func validRedirect(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Fragment != "" || p.Host == "" {
		return false
	}
	if p.Scheme == "https" {
		return true
	}
	h := p.Hostname()
	return p.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1")
}

func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if oauthLimit.Blocked("reg:" + ip) {
		oauthJSONErr(w, http.StatusTooManyRequests, "too_many_requests", "try again later")
		return
	}
	oauthLimit.Fail("reg:" + ip)
	if !s.mcpSettings(r.Context()).Enabled {
		oauthJSONErr(w, http.StatusForbidden, "access_denied", "the MCP server of this phone system is switched off")
		return
	}
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		oauthJSONErr(w, http.StatusBadRequest, "invalid_client_metadata", "invalid JSON")
		return
	}
	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > 5 {
		oauthJSONErr(w, http.StatusBadRequest, "invalid_redirect_uri", "one to five redirect_uris")
		return
	}
	for _, u := range req.RedirectURIs {
		if !validRedirect(u) || len(u) > 500 {
			oauthJSONErr(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must be https (or http on localhost)")
			return
		}
	}
	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60])
	}
	ctx := r.Context()
	// Registrations that never led to a connection are removed after a day.
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM mcp_oauth_clients WHERE created_at < ? AND id NOT IN (SELECT client_id FROM mcp_connections)`,
		time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_oauth_clients`).Scan(&n)
	if n >= 500 {
		oauthJSONErr(w, http.StatusServiceUnavailable, "temporarily_unavailable", "too many registered clients")
		return
	}
	id := newMCPSecret("kmc_")[:28]
	uris, _ := json.Marshal(req.RedirectURIs)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO mcp_oauth_clients(id, name, redirect_uris) VALUES (?,?,?)`, id, name, string(uris)); err != nil {
		oauthJSONErr(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": id, "client_id_issued_at": time.Now().Unix(), "client_name": name, "redirect_uris": req.RedirectURIs,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
	})
}

type oauthClient struct {
	ID       string
	Name     string
	Redirect []string
}

func (s *Server) oauthClient(r *http.Request, id string) (*oauthClient, bool) {
	var c oauthClient
	var uris string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id, name, redirect_uris FROM mcp_oauth_clients WHERE id = ?`, id).Scan(&c.ID, &c.Name, &uris)
	if err != nil {
		return nil, false
	}
	_ = json.Unmarshal([]byte(uris), &c.Redirect)
	return &c, true
}

// oauthParams checks the authorisation request. A bad client or redirect URI is never redirected to.
type oauthParams struct {
	client    *oauthClient
	redirect  string
	state     string
	challenge string
	scopes    []string
}

func (s *Server) parseAuthorize(r *http.Request, q url.Values) (*oauthParams, string, string) {
	c, ok := s.oauthClient(r, q.Get("client_id"))
	if !ok {
		return nil, "invalid_client", ""
	}
	ru := q.Get("redirect_uri")
	if ru == "" && len(c.Redirect) == 1 {
		ru = c.Redirect[0]
	}
	if !slices.Contains(c.Redirect, ru) {
		return nil, "invalid_redirect_uri", ""
	}
	p := &oauthParams{client: c, redirect: ru, state: q.Get("state"), challenge: q.Get("code_challenge")}
	if q.Get("response_type") != "code" {
		return p, "unsupported_response_type", ru
	}
	if p.challenge == "" || q.Get("code_challenge_method") != "S256" || len(p.challenge) < 43 || len(p.challenge) > 128 {
		return p, "invalid_request", ru
	}
	for _, sc := range strings.Fields(q.Get("scope")) {
		if _, ok := mcpScopeByKey(sc); ok {
			p.scopes = append(p.scopes, sc)
		}
	}
	return p, "", ru
}

func (s *Server) issuer(r *http.Request) string { return s.requestBase(r) }

func redirectWith(base string, v url.Values) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	p, code, ru := s.parseAuthorize(r, r.URL.Query())
	if p == nil {
		http.Error(w, "Kiestoon: unknown client or redirect address ("+code+"). Remove the connector in your AI assistant and add it again.", http.StatusBadRequest)
		return
	}
	if code != "" {
		http.Redirect(w, r, redirectWith(ru, url.Values{"error": {code}, "state": {p.state}, "iss": {s.issuer(r)}}), http.StatusFound)
		return
	}
	if !s.mcpSettings(r.Context()).Enabled {
		http.Redirect(w, r, redirectWith(ru, url.Values{"error": {"access_denied"}, "error_description": {"the MCP server is switched off"}, "state": {p.state}, "iss": {s.issuer(r)}}), http.StatusFound)
		return
	}
	// The consent page of the portal signs the person in (if needed) and asks what to allow.
	http.Redirect(w, r, "/oauth/consent?"+r.URL.RawQuery, http.StatusFound)
}

// oauthRequestInfo tells the consent page who asks and what can be allowed.
func (s *Server) oauthRequestInfo(w http.ResponseWriter, r *http.Request) {
	q, err := url.ParseQuery(r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	p, code, _ := s.parseAuthorize(r, q)
	if p == nil || code != "" {
		writeErr(w, http.StatusBadRequest, "oauth_"+orDefault(code, "invalid_request"))
		return
	}
	set := s.mcpSettings(r.Context())
	u := sessionFrom(r).User
	if !set.Enabled {
		writeErr(w, http.StatusConflict, "mcp_disabled")
		return
	}
	if !s.mcpUserAllowed(r) {
		writeErr(w, http.StatusForbidden, "mcp_users_not_allowed")
		return
	}
	role := tokenRole(u)
	requested := cleanScopes(p.scopes, role, set)
	if len(requested) == 0 {
		for _, sc := range mcpScopes {
			if !set.DefaultReadOnly || sc.Kind == "read" {
				requested = append(requested, sc.Key)
			}
		}
		requested = cleanScopes(requested, role, set)
	}
	ru, _ := url.Parse(p.redirect)
	writeJSON(w, http.StatusOK, map[string]any{
		"client": p.client.Name, "redirectHost": ru.Host, "role": role, "username": u.Username,
		"scopes": mcpScopesFor(role, set), "requested": requested,
	})
}

func (s *Server) oauthDecide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query  string   `json:"query"`
		Allow  bool     `json:"allow"`
		Scopes []string `json:"scopes"`
		Days   int      `json:"days"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	q, err := url.ParseQuery(req.Query)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	p, code, _ := s.parseAuthorize(r, q)
	if p == nil || code != "" {
		writeErr(w, http.StatusBadRequest, "oauth_"+orDefault(code, "invalid_request"))
		return
	}
	set := s.mcpSettings(r.Context())
	u := sessionFrom(r).User
	iss := s.issuer(r)
	if !req.Allow {
		s.audit(r, "mcp.oauth.denied", p.client.Name)
		writeJSON(w, http.StatusOK, map[string]string{"redirect": redirectWith(p.redirect, url.Values{"error": {"access_denied"}, "state": {p.state}, "iss": {iss}})})
		return
	}
	if !set.Enabled || !s.mcpUserAllowed(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	role := tokenRole(u)
	scopes := cleanScopes(req.Scopes, role, set)
	if len(scopes) == 0 {
		fieldErr(w, "scopes")
		return
	}
	if req.Days < 1 || req.Days > 365 {
		fieldErr(w, "days")
		return
	}
	c := newMCPSecret("kac_")
	s.mcp.mu.Lock()
	if s.mcp.codes == nil {
		s.mcp.codes = map[string]oauthCode{}
	}
	now := time.Now()
	for k, v := range s.mcp.codes {
		if now.After(v.expires) {
			delete(s.mcp.codes, k)
		}
	}
	s.mcp.codes[hashMCP(c)] = oauthCode{clientID: p.client.ID, redirect: p.redirect, challenge: p.challenge, userID: u.ID, role: role,
		scopes: scopes, days: req.Days, expires: now.Add(5 * time.Minute)}
	s.mcp.mu.Unlock()
	s.audit(r, "mcp.oauth.allowed", p.client.Name+" "+strings.Join(scopes, ","))
	writeJSON(w, http.StatusOK, map[string]string{"redirect": redirectWith(p.redirect, url.Values{"code": {c}, "state": {p.state}, "iss": {iss}})})
}

func pkceOK(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if oauthLimit.Blocked("tok:" + ip) {
		oauthJSONErr(w, http.StatusTooManyRequests, "too_many_requests", "try again later")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthJSONErr(w, http.StatusBadRequest, "invalid_request", "form expected")
		return
	}
	fail := func(code, desc string) {
		oauthLimit.Fail("tok:" + ip)
		oauthJSONErr(w, http.StatusBadRequest, code, desc)
	}
	clientID := r.PostForm.Get("client_id")
	client, ok := s.oauthClient(r, clientID)
	if !ok {
		oauthLimit.Fail("tok:" + ip)
		oauthJSONErr(w, http.StatusUnauthorized, "invalid_client", "unknown client")
		return
	}
	set := s.mcpSettings(r.Context())
	if !set.Enabled {
		fail("access_denied", "the MCP server is switched off")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		key := hashMCP(r.PostForm.Get("code"))
		s.mcp.mu.Lock()
		ac, found := s.mcp.codes[key]
		delete(s.mcp.codes, key) // single use, also when the rest fails
		s.mcp.mu.Unlock()
		if !found || time.Now().After(ac.expires) || ac.clientID != clientID {
			fail("invalid_grant", "unknown or expired code")
			return
		}
		if ru := r.PostForm.Get("redirect_uri"); ru != "" && ru != ac.redirect {
			fail("invalid_grant", "redirect_uri does not match")
			return
		}
		if !pkceOK(r.PostForm.Get("code_verifier"), ac.challenge) {
			fail("invalid_grant", "PKCE verification failed")
			return
		}
		access, refresh := newMCPSecret(mcpAccessPrefix), newMCPSecret(mcpRefreshPrefix)
		sj, _ := json.Marshal(ac.scopes)
		exp := time.Now().AddDate(0, 0, ac.days).UTC().Format(time.RFC3339)
		_, err := s.DB.ExecContext(r.Context(), `INSERT INTO mcp_connections(name, kind, client_id, user_id, role, scopes, prefix, token_hash, access_expires, refresh_hash, expires_at, last_ip)
			VALUES (?, 'oauth', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, client.Name, client.ID, ac.userID, ac.role, string(sj), "OAuth",
			hashMCP(access), time.Now().Add(mcpAccessTTL).UTC().Format(time.RFC3339), hashMCP(refresh), exp, ip)
		if err != nil {
			oauthJSONErr(w, http.StatusInternalServerError, "server_error", "")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(mcpAccessTTL.Seconds()),
			"refresh_token": refresh, "scope": strings.Join(ac.scopes, " ")})
	case "refresh_token":
		var id int64
		var scopes, expires string
		err := s.DB.QueryRowContext(r.Context(), `SELECT id, scopes, expires_at FROM mcp_connections WHERE kind = 'oauth' AND refresh_hash = ? AND client_id = ?`,
			hashMCP(r.PostForm.Get("refresh_token")), clientID).Scan(&id, &scopes, &expires)
		if err == sql.ErrNoRows || (err == nil && expired(expires)) {
			fail("invalid_grant", "unknown or expired refresh token")
			return
		}
		if err != nil {
			oauthJSONErr(w, http.StatusInternalServerError, "server_error", "")
			return
		}
		access, refresh := newMCPSecret(mcpAccessPrefix), newMCPSecret(mcpRefreshPrefix)
		if _, err := s.DB.ExecContext(r.Context(), `UPDATE mcp_connections SET token_hash = ?, access_expires = ?, refresh_hash = ? WHERE id = ?`,
			hashMCP(access), time.Now().Add(mcpAccessTTL).UTC().Format(time.RFC3339), hashMCP(refresh), id); err != nil {
			oauthJSONErr(w, http.StatusInternalServerError, "server_error", "")
			return
		}
		var sc []string
		_ = json.Unmarshal([]byte(scopes), &sc)
		writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(mcpAccessTTL.Seconds()),
			"refresh_token": refresh, "scope": strings.Join(sc, " ")})
	default:
		fail("unsupported_grant_type", "authorization_code or refresh_token")
	}
}

func (s *Server) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err == nil {
		h := hashMCP(r.PostForm.Get("token"))
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM mcp_connections WHERE kind = 'oauth' AND (token_hash = ? OR refresh_hash = ?)`, h, h)
	}
	w.WriteHeader(http.StatusOK)
}
