package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/welcome"
)

// Welcome e-mails: an extension with an e-mail address gets a portal account (role user) and a
// mail with a one-time link to set a password, the portal addresses and the phone details.

func (s *Server) welcomeRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("POST /api/pbx/extensions/{id}/welcome", admin(s.extensionWelcome))
	mux.Handle("POST /api/users/{id}/welcome", admin(s.userWelcome))
	// Public: the set-password page.
	mux.HandleFunc("POST /api/auth/password-link", s.passwordLinkInfo)
	mux.HandleFunc("POST /api/auth/password-link/set", s.passwordLinkSet)
}

// welcomeResult tells the admin what happened. Link is only returned when the mail could not be
// sent, so the admin can hand it over another way.
type welcomeResult struct {
	Sent     bool   `json:"sent"`
	Created  bool   `json:"created"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Error    string `json:"error,omitempty"`
	Link     string `json:"link,omitempty"`
}

// ensurePortalUser returns the account linked to the extension, links an unlinked account with the
// same e-mail address, or creates a new account with role user.
func (s *Server) ensurePortalUser(ctx context.Context, e *pbx.Extension, lang string) (*auth.User, bool, error) {
	if u, err := s.Auth.UserByExtension(ctx, e.ID); err == nil {
		return u, false, nil
	}
	if e.Email == "" {
		return nil, false, errors.New("no_email")
	}
	if u, err := s.Auth.UserByEmailOrName(ctx, e.Email); err == nil && u.ExtensionID == nil {
		if err := s.Auth.LinkExtension(ctx, u.ID, e.ID); err != nil {
			return nil, false, err
		}
		u.ExtensionID = &e.ID
		return u, false, nil
	}
	name := e.Name
	if name == "" {
		name = e.Number
	}
	local := e.Email
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	var lastErr error
	for _, username := range []string{e.Email, local + "." + e.Number, "ext" + e.Number} {
		if len(username) > 64 {
			continue
		}
		id := e.ID
		u, err := s.Auth.SaveUser(ctx, 0, auth.UserInput{Username: username, DisplayName: name, Email: e.Email, Role: "user",
			ExtensionID: &id, Language: lang, Enabled: true})
		if err == nil {
			return u, true, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "UNIQUE") {
			break
		}
	}
	return nil, false, lastErr
}

// portalLinks are the addresses the account reaches the portal on, the first one is for the link.
func (s *Server) portalLinks(r *http.Request, lang string) (primary string, links []welcome.Link, host string) {
	label := func(en, nl string) string {
		if lang == "nl" {
			return nl
		}
		return en
	}
	pub, _ := store.GetSetting(r.Context(), s.DB, portalURLKey, "")
	if pub == "" {
		pub, _ = store.GetSetting(r.Context(), s.DB, "public_url", "")
	}
	if pub = strings.TrimRight(pub, "/"); pub != "" {
		// Phones register with the PBX host name, employees open the portal host name.
		host = s.sipHost(r)
		if host == "" {
			if u, err := url.Parse(pub); err == nil {
				host = u.Hostname()
			}
		}
		return pub, []welcome.Link{{Label: label("Web portal", "Webportal"), URL: pub}}, host
	}
	host = r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	hp := host
	if strings.Contains(hp, ":") {
		hp = "[" + hp + "]"
	}
	if p := listenPort(s.Cfg.Listen, 8080); p != 0 {
		links = append(links, welcome.Link{Label: label("Web portal (http)", "Webportal (http)"), URL: fmt.Sprintf("http://%s:%d", hp, p)})
	}
	if p := listenPort(s.Cfg.TLSListen, 8443); p != 0 {
		links = append(links, welcome.Link{Label: label("Softphone and portal (https)", "Softphone en portal (https)"), URL: fmt.Sprintf("https://%s:%d", hp, p)})
	}
	// The set-password link uses the address the administrator is working on.
	return s.publicBase(r), links, host
}

// sendWelcome makes a set-password link for the account and mails it with the portal and phone details.
func (s *Server) sendWelcome(r *http.Request, u *auth.User, e *pbx.Extension) welcomeResult {
	ctx := r.Context()
	res := welcomeResult{Username: u.Username, Email: u.Email}
	if res.Email == "" && e != nil {
		res.Email = e.Email
	}
	if res.Email == "" {
		res.Error = "no_email"
		return res
	}
	token, exp, err := s.Auth.CreatePasswordToken(ctx, u.ID, auth.WelcomeTokenTTL)
	if err != nil {
		res.Error = "internal"
		return res
	}
	primary, links, host := s.portalLinks(r, u.Language)
	setURL := primary + "/set-password#" + token
	d := welcome.Data{
		Name: u.DisplayName, Username: u.Username, SetPasswordURL: setURL, Portal: links,
		ExpiresHours: int(time.Until(exp).Round(time.Hour) / time.Hour),
	}
	if ps, err := s.SSO.List(ctx); err == nil {
		for _, p := range ps {
			if p.Enabled {
				d.SSO = append(d.SSO, p.Name)
			}
		}
	}
	if e != nil {
		st, _ := pbx.LoadSettings(ctx, s.DB)
		d.Extension, d.SIPServer, d.Transport = e.Number, host, strings.ToUpper(e.Transport)
		d.SIPPort = st.SIPPort
		if e.Transport == "tls" {
			d.SIPPort = st.TLSPort
		}
		d.Voicemail, d.WebRTC = e.Voicemail, e.WebRTC
		devices := primary
		if len(links) > 0 {
			devices = links[len(links)-1].URL
		}
		d.DevicesURL = devices + "/me/devices"
	}
	subject, text, html, err := welcome.Render(u.Language, d)
	if err != nil {
		res.Error = "internal"
		return res
	}
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = s.Mail.SendHTML(sctx, "welcome", res.Email, subject, text, html)
	cancel()
	switch {
	case errors.Is(err, mail.ErrDisabled):
		res.Error, res.Link = "mail_disabled", setURL
	case err != nil:
		s.Log.Warn("welcome mail", "to", res.Email, "err", err)
		res.Error, res.Link = "mail_failed", setURL
	default:
		res.Sent = true
	}
	s.audit(r, "user.welcome", fmt.Sprintf("%s %s sent=%t", u.Username, res.Email, res.Sent))
	return res
}

// welcomeForExtension creates or links the portal account of a (new) extension and mails it.
func (s *Server) welcomeForExtension(r *http.Request, e *pbx.Extension, lang string) welcomeResult {
	if lang != "nl" && lang != "en" {
		lang = sessionFrom(r).User.Language
	}
	u, created, err := s.ensurePortalUser(r.Context(), e, lang)
	if err != nil {
		code := "internal"
		if err.Error() == "no_email" {
			code = "no_email"
		}
		return welcomeResult{Error: code, Email: e.Email}
	}
	if created {
		s.audit(r, "user.saved", u.Username+" user (extension "+e.Number+")")
	}
	res := s.sendWelcome(r, u, e)
	res.Created = created
	return res
}

func (s *Server) extensionWelcome(w http.ResponseWriter, r *http.Request) {
	e, err := s.PBX.Extension(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	_ = decode(r, &req)
	res := s.welcomeForExtension(r, e, req.Language)
	if res.Error == "no_email" || res.Error == "internal" {
		writeErr(w, http.StatusBadRequest, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) userWelcome(w http.ResponseWriter, r *http.Request) {
	u, err := s.Auth.UserByID(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !mayManage(sessionFrom(r).User, u.Role) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !u.Enabled {
		writeErr(w, http.StatusBadRequest, "user_disabled")
		return
	}
	var e *pbx.Extension
	if u.ExtensionID != nil {
		e, _ = s.PBX.Extension(r.Context(), *u.ExtensionID)
	}
	res := s.sendWelcome(r, u, e)
	if res.Error == "no_email" || res.Error == "internal" {
		writeErr(w, http.StatusBadRequest, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) passwordLinkInfo(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u, err := s.Auth.PasswordTokenUser(r.Context(), req.Token)
	if err != nil {
		s.loginLimit.Fail(ip)
		writeErr(w, http.StatusBadRequest, "link_invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "displayName": u.DisplayName, "language": u.Language})
}

func (s *Server) passwordLinkSet(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u, err := s.Auth.SetPasswordWithToken(r.Context(), req.Token, req.Password)
	switch {
	case errors.Is(err, auth.ErrWeakPassword):
		writeErr(w, http.StatusBadRequest, "weak_password")
		return
	case errors.Is(err, auth.ErrTokenInvalid):
		s.loginLimit.Fail(ip)
		writeErr(w, http.StatusBadRequest, "link_invalid")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(r.Context(), s.DB, u.ID, "user.password_set", "welcome link", ip)
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username})
}
