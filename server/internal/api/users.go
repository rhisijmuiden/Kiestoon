package api

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/crewjam/saml"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/sso"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) userRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/users", admin(s.listUsers))
	mux.Handle("POST /api/users", admin(s.saveUser))
	mux.Handle("PUT /api/users/{id}", admin(s.saveUser))
	mux.Handle("DELETE /api/users/{id}", admin(s.deleteUser))
	mux.Handle("POST /api/users/{id}/mfa-reset", admin(s.resetUserMFA))

	mux.Handle("GET /api/sso/providers", admin(s.listProviders))
	mux.Handle("POST /api/sso/providers", admin(s.saveProvider))
	mux.Handle("PUT /api/sso/providers/{id}", admin(s.saveProvider))
	mux.Handle("DELETE /api/sso/providers/{id}", admin(s.deleteProvider))

	// Public: the sign-in page lists providers and starts the flow.
	mux.HandleFunc("GET /api/auth/sso/providers", s.publicProviders)
	mux.HandleFunc("GET /api/auth/sso/{id}/start", s.ssoStart)
	mux.HandleFunc("GET /api/auth/sso/callback", s.ssoCallback)
	mux.HandleFunc("GET /api/auth/saml/metadata", s.samlMetadata)
	mux.HandleFunc("POST /api/auth/saml/acs", s.samlACS)
}

// mayManage reports whether actor may change an account with this role. Only a superadmin
// manages superadmins.
func mayManage(actor *auth.User, role string) bool {
	return actor.Role == "superadmin" || auth.RoleRank(role) < auth.RoleRank("superadmin")
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, us)
}

func (s *Server) saveUser(w http.ResponseWriter, r *http.Request) {
	var in auth.UserInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	actor := sessionFrom(r).User
	id := pathID(r)
	if !mayManage(actor, in.Role) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if id != 0 {
		old, err := s.Auth.UserByID(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		if !mayManage(actor, old.Role) {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		if old.ID == actor.ID && (in.Role != old.Role || !in.Enabled) {
			writeErr(w, http.StatusBadRequest, "cannot_change_self")
			return
		}
	}
	if in.ExtensionID != nil && *in.ExtensionID > 0 {
		if _, err := s.PBX.Extension(r.Context(), *in.ExtensionID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown_destination", "field": "extensionId"})
			return
		}
	}
	u, err := s.Auth.SaveUser(r.Context(), id, in)
	switch {
	case errors.Is(err, auth.ErrWeakPassword):
		writeErr(w, http.StatusBadRequest, "weak_password")
		return
	case errors.Is(err, auth.ErrLastSuperadmin):
		writeErr(w, http.StatusBadRequest, "last_superadmin")
		return
	case err != nil && strings.Contains(err.Error(), "UNIQUE"):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "duplicate", "field": "username"})
		return
	case err != nil:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "username"})
		return
	}
	s.audit(r, "user.saved", u.Username+" "+u.Role)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	actor := sessionFrom(r).User
	u, err := s.Auth.UserByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if u.ID == actor.ID {
		writeErr(w, http.StatusBadRequest, "cannot_change_self")
		return
	}
	if !mayManage(actor, u.Role) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.Auth.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, auth.ErrLastSuperadmin) {
			writeErr(w, http.StatusBadRequest, "last_superadmin")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "user.deleted", u.Username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) resetUserMFA(w http.ResponseWriter, r *http.Request) {
	u, err := s.Auth.UserByID(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !mayManage(sessionFrom(r).User, u.Role) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.Auth.ResetMFA(r.Context(), u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "user.mfa_reset", u.Username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- identity providers -----------------------------------------------------

// publicBase is the address people reach the portal on for this request: the employee portal address
// on the portal host name, otherwise the PBX address (internet mode or the proxy sets it), otherwise
// the address of this request. Single sign-on must come back to the host name it started on.
func (s *Server) publicBase(r *http.Request) string {
	if onPortalHost(r) {
		return s.portalBase(r)
	}
	return s.pbxBase(r)
}

// redirectURI is the callback address to register at the identity provider.
func (s *Server) redirectURI(r *http.Request) string {
	return s.publicBase(r) + "/api/auth/sso/callback"
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	ps, err := s.SSO.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	for i := range ps {
		ps[i].ClientSecret = ""
	}
	ep := sso.Endpoints(s.pbxBase(r))
	writeJSON(w, http.StatusOK, map[string]any{"providers": ps, "redirectUri": s.redirectURI(r), "kinds": sso.Kinds,
		"samlEntityId": ep.EntityID, "samlAcs": ep.ACS})
}

func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request) {
	var p sso.Provider
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	p.ID = pathID(r)
	if !mayManage(sessionFrom(r).User, "superadmin") {
		// Admins may not hand out the superadmin role through group mapping.
		for _, m := range p.RoleMap {
			if m.Role == "superadmin" {
				writeErr(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		if p.DefaultRole == "superadmin" {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	if p.IsSAML() && p.Issuer != "" && p.SAMLMetadata == "" {
		// A metadata URL: fetch the current metadata (signing certificates change over time).
		md, err := sso.FetchIDPMetadata(r.Context(), strings.TrimSpace(p.Issuer))
		if err != nil {
			s.Log.Warn("saml metadata", "url", p.Issuer, "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "issuer", "message": err.Error()})
			return
		}
		p.SAMLMetadata = md
	}
	if err := s.SSO.Save(r.Context(), &p); err != nil {
		var fe *sso.FieldError
		if errors.As(err, &fe) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": fe.Field})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.SSOFlow.Forget(p.Issuer)
	s.audit(r, "sso.saved", p.Name)
	p.ClientSecret = ""
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := s.SSO.Delete(r.Context(), pathID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "sso.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) publicProviders(w http.ResponseWriter, r *http.Request) {
	ps, _ := s.SSO.List(r.Context())
	type pub struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	out := []pub{}
	for _, p := range ps {
		if p.Enabled {
			out = append(out, pub{p.ID, p.Name, p.Kind})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

const ssoCookie = "kiestoon_sso"

func ssoFail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(code), http.StatusFound)
}

func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	p, err := s.SSO.Get(r.Context(), pathID(r))
	if err != nil || !p.Enabled {
		ssoFail(w, r, "sso_unknown")
		return
	}
	if p.IsSAML() {
		sp, err := sso.ServiceProvider(r.Context(), s.DB, sso.Endpoints(s.pbxBase(r)), p)
		if err != nil {
			s.Log.Warn("saml start", "provider", p.Name, "err", err)
			ssoFail(w, r, "sso_unreachable")
			return
		}
		u, err := s.SAMLFlow.Start(sp, p)
		if err != nil {
			s.Log.Warn("saml start", "provider", p.Name, "err", err)
			ssoFail(w, r, "sso_unreachable")
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	state, authURL, err := s.SSOFlow.Start(r.Context(), p, s.redirectURI(r))
	if err != nil {
		s.Log.Warn("sso start", "provider", p.Name, "err", err)
		ssoFail(w, r, "sso_unreachable")
		return
	}
	// Lax, not Strict: the browser comes back from the provider with a cross-site navigation.
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: state, Path: "/api/auth/sso/", HttpOnly: true, Secure: s.secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	q := r.URL.Query()
	c, err := r.Cookie(ssoCookie)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/api/auth/sso/", HttpOnly: true, Secure: s.secure(r), MaxAge: -1})
	if q.Get("error") != "" {
		ssoFail(w, r, "sso_denied")
		return
	}
	state := q.Get("state")
	if err != nil || state == "" || !auth.ConstantTimeEqual(c.Value, state) {
		ssoFail(w, r, "sso_state")
		return
	}
	pid, ok := s.SSOFlow.PendingProvider(state)
	if !ok {
		ssoFail(w, r, "sso_state")
		return
	}
	p, err := s.SSO.Get(r.Context(), pid)
	if err != nil || !p.Enabled {
		ssoFail(w, r, "sso_unknown")
		return
	}
	id, err := s.SSOFlow.Finish(r.Context(), p, state, q.Get("code"))
	if err != nil {
		s.Log.Warn("sso sign-in failed", "provider", p.Name, "err", err)
		store.Audit(r.Context(), s.DB, 0, "login.sso_failed", p.Name, ip)
		ssoFail(w, r, "sso_failed")
		return
	}
	s.ssoComplete(w, r, p, id)
}

// samlMetadata serves Kiestoon's service provider metadata, to import at the identity provider.
func (s *Server) samlMetadata(w http.ResponseWriter, r *http.Request) {
	sp, err := sso.ServiceProvider(r.Context(), s.DB, sso.Endpoints(s.pbxBase(r)), nil)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	b, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Content-Disposition", `inline; filename="kiestoon-saml-metadata.xml"`)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(b)
}

// samlACS receives the SAML response (HTTP-POST binding) from the identity provider.
func (s *Server) samlACS(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		ssoFail(w, r, "sso_state")
		return
	}
	pid, reqID, ok := s.SAMLFlow.Take(r.PostForm.Get("RelayState"))
	if !ok {
		ssoFail(w, r, "sso_state")
		return
	}
	p, err := s.SSO.Get(r.Context(), pid)
	if err != nil || !p.Enabled || !p.IsSAML() {
		ssoFail(w, r, "sso_unknown")
		return
	}
	sp, err := sso.ServiceProvider(r.Context(), s.DB, sso.Endpoints(s.pbxBase(r)), p)
	if err != nil {
		ssoFail(w, r, "sso_failed")
		return
	}
	a, err := sp.ParseResponse(r, []string{reqID})
	if err != nil {
		var ire *saml.InvalidResponseError
		detail := err.Error()
		if errors.As(err, &ire) && ire.PrivateErr != nil {
			detail = ire.PrivateErr.Error()
		}
		s.Log.Warn("saml sign-in failed", "provider", p.Name, "err", detail)
		store.Audit(r.Context(), s.DB, 0, "login.sso_failed", p.Name, ip)
		ssoFail(w, r, "sso_failed")
		return
	}
	id, err := sso.SAMLIdentity(p, a)
	if err != nil {
		s.Log.Warn("saml identity", "provider", p.Name, "err", err)
		ssoFail(w, r, "sso_failed")
		return
	}
	s.ssoComplete(w, r, p, id)
}

// ssoComplete signs in (and if allowed creates) the account of a verified identity.
func (s *Server) ssoComplete(w http.ResponseWriter, r *http.Request, p *sso.Provider, id *sso.Identity) {
	ip := clientIP(r)
	role, allowed := sso.RoleFor(p, id.Groups)
	if !allowed {
		store.Audit(r.Context(), s.DB, 0, "login.sso_denied", id.Username, ip)
		ssoFail(w, r, "sso_no_role")
		return
	}
	ctx := r.Context()
	u, err := s.Auth.UserBySSO(ctx, p.ID, id.Subject)
	if err != nil && id.Email != "" {
		// First SSO sign-in of an existing account: link it by e-mail address or username.
		if u, err = s.Auth.UserByEmailOrName(ctx, id.Email); err == nil {
			_ = s.Auth.LinkSSO(ctx, u.ID, p.ID, id.Subject)
		}
	}
	if err != nil {
		if !p.AutoCreate {
			store.Audit(ctx, s.DB, 0, "login.sso_unknown_user", id.Username, ip)
			ssoFail(w, r, "sso_no_account")
			return
		}
		username := id.Username
		if username == "" {
			username = id.Subject
		}
		u, err = s.Auth.SaveUser(ctx, 0, auth.UserInput{Username: username, DisplayName: id.Name, Email: id.Email, Role: role,
			Enabled: true, Language: "en"})
		if err != nil {
			s.Log.Warn("sso create user", "err", err)
			ssoFail(w, r, "sso_no_account")
			return
		}
		_ = s.Auth.LinkSSO(ctx, u.ID, p.ID, id.Subject)
		store.Audit(ctx, s.DB, u.ID, "user.created_sso", u.Username+" "+role, ip)
	}
	if !u.Enabled {
		ssoFail(w, r, "sso_disabled")
		return
	}
	// The provider's groups decide the role on every sign-in, except that the last superadmin stays one.
	if len(p.RoleMap) > 0 && role != u.Role {
		if u.Role != "superadmin" || s.superadmins(r) > 1 {
			_ = s.Auth.SetRole(ctx, u.ID, role)
			u.Role = role
		}
	}
	if u.ExtensionID == nil && u.Email != "" {
		var extID int64
		if s.DB.QueryRowContext(ctx, `SELECT id FROM extensions WHERE email = ? COLLATE NOCASE AND id NOT IN
			(SELECT extension_id FROM users WHERE extension_id IS NOT NULL) LIMIT 1`, u.Email).Scan(&extID) == nil {
			_, _ = s.DB.ExecContext(ctx, `UPDATE users SET extension_id = ? WHERE id = ?`, extID, u.ID)
		}
	}
	method, passed := "sso", true
	if p.TrustMFA {
		method = "sso-mfa"
	} else if u.TOTPEnabled {
		passed = false // ask Kiestoon's own second factor
	}
	token, err := s.Auth.CreateSessionMethod(ctx, u, passed, method)
	if err != nil {
		ssoFail(w, r, "internal")
		return
	}
	store.Audit(ctx, s.DB, u.ID, "login.success", "sso "+p.Name, ip)
	s.setCookie(w, r, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) superadmins(r *http.Request) int {
	var n int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'superadmin' AND enabled = 1`).Scan(&n)
	return n
}

func atoi64(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}
