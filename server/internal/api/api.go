// Package api serves the JSON API used by the web portal.
package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
	"github.com/rhisijmuiden/kiestoon/server/internal/crm"
	"github.com/rhisijmuiden/kiestoon/server/internal/firmware"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/phonebook"
	"github.com/rhisijmuiden/kiestoon/server/internal/sso"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/updates"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

// CookieName is the session cookie.
const CookieName = "kiestoon_session"

// Server holds everything the handlers need.
type Server struct {
	Cfg     config.Config
	DB      *sql.DB
	Auth    *auth.Service
	Updates *updates.Checker
	Log     *slog.Logger
	PBX     *pbx.Repo
	Ast     *asterisk.Manager
	AMI     *asterisk.AMI
	Live    *asterisk.LiveCache
	CDR     *cdr.Store
	Mail    *mail.Service
	Helper  *helper.Client
	States  *asterisk.StateCache
	Spool   *media.Spool
	SSO     *sso.Store
	SSOFlow *sso.Flow
	// SAMLFlow keeps SAML sign-ins in progress.
	SAMLFlow *sso.SAMLFlow
	// loginLimit counts failed password and MFA attempts per IP.
	loginLimit *auth.Limiter
	// provLimit counts failed provisioning requests per IP.
	provLimit *auth.Limiter
	started   time.Time
	backupMu  sync.Mutex
	odMu      sync.Mutex
	odLogins  map[int64]oneDriveLogin
	ldapMu    sync.Mutex
	ldap      *phonebook.Directory
	ldapOn    bool
	hooks     *webhookQueue
	passkeys  passkeyStore
	// xfers are the attended transfers started from the portal, by transferer channel.
	xfers    transfers
	Firmware *firmware.Store
	speech   speechState
	faxMu    sync.Mutex
	siteMu   sync.Mutex
	fed      fedState
	ha       haState
	extBusy  sync.Mutex
	sttBusy  sync.Mutex
	// pairs are the one-time codes that link a Windows softphone.
	pairs desktopPairs
	// journeys records what happens to every call, for the call history.
	journeys journeyState
	// wall is the wallboard: agent timers, cached figures and active SLA alerts.
	wall wallState
	// mobile is the mobile app: voicemail counters for its notifications.
	mobile mobileState
	// crm links to CRM systems: look-ups for the call pop-up and call logging.
	crm     *crm.Service
	crmLive *crmState
	// quality follows the RTCP reports of the calls going on.
	quality qualityState
}

// New wires up a server.
func New(cfg config.Config, db *sql.DB, log *slog.Logger) *Server {
	s := &Server{
		Cfg:        cfg,
		DB:         db,
		Auth:       auth.New(db),
		Updates:    updates.NewChecker(cfg.Repo, cfg.DataDir),
		Log:        log,
		PBX:        &pbx.Repo{DB: db},
		CDR:        &cdr.Store{DB: db},
		Mail:       &mail.Service{DB: db, QueueDir: mail.QueueDir(cfg.DataDir)},
		Helper:     &helper.Client{Socket: cfg.HelperSocket},
		Spool:      &media.Spool{Dir: cfg.SpoolDir()},
		SSO:        &sso.Store{DB: db},
		SSOFlow:    sso.NewFlow(),
		SAMLFlow:   sso.NewSAMLFlow(),
		loginLimit: auth.NewLimiter(5, 10*time.Minute),
		provLimit:  auth.NewLimiter(20, 10*time.Minute),
		started:    time.Now(),
		hooks:      &webhookQueue{},
		Firmware:   &firmware.Store{DB: db, Dir: filepath.Join(cfg.DataDir, "firmware")},
	}
	secret, _ := store.GetSetting(context.Background(), db, "ami_secret", "")
	if secret == "" {
		secret = pbx.GenerateSecret(32)
		_ = store.SetSetting(context.Background(), db, "ami_secret", secret)
	}
	s.AMI = &asterisk.AMI{Addr: cfg.AMIAddr, User: "kiestoon", Secret: secret, Log: log, OnEvent: s.onAMIEvent}
	s.Mail.Prepare = s.prepareMail
	s.Live = &asterisk.LiveCache{AMI: s.AMI}
	s.States = &asterisk.StateCache{AMI: s.AMI}
	s.Ast = &asterisk.Manager{Dir: cfg.PBXDir(), Repo: s.PBX, AMI: s.AMI, Log: log, Options: s.asteriskOptions}
	s.loadHostnames(context.Background())
	s.startJourneys()
	s.startCRM()
	return s
}

type ctxKey struct{}

func contextWithSession(r *http.Request, s *auth.Session) context.Context {
	return context.WithValue(r.Context(), ctxKey{}, s)
}

func sessionFrom(r *http.Request) *auth.Session {
	s, _ := r.Context().Value(ctxKey{}).(*auth.Session)
	return s
}

// Routes returns the API handler, mounted under /api/.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/auth/login", s.login)

	// Signed in, second factor may still be pending
	mux.Handle("POST /api/auth/mfa", s.signedIn(http.HandlerFunc(s.verifyMFA), false))
	mux.Handle("POST /api/auth/logout", s.signedIn(http.HandlerFunc(s.logout), false))

	// Signed in and MFA passed (or not yet enrolled: then only the enrollment routes work)
	full := func(h http.HandlerFunc) http.Handler { return s.signedIn(s.mfaEnrolled(h), true) }
	enroll := func(h http.HandlerFunc) http.Handler { return s.signedIn(h, true) }
	admin := func(h http.HandlerFunc) http.Handler { return s.signedIn(s.mfaEnrolled(s.adminOnly(h)), true) }
	recep := func(h http.HandlerFunc) http.Handler { return s.signedIn(s.mfaEnrolled(s.receptionOnly(h)), true) }

	mux.Handle("GET /api/me", enroll(s.me))
	mux.Handle("POST /api/account/mfa/enroll", enroll(s.mfaEnroll))
	mux.Handle("POST /api/account/mfa/confirm", enroll(s.mfaConfirm))
	mux.Handle("PUT /api/account/language", enroll(s.setLanguage))
	mux.Handle("PUT /api/account/theme", enroll(s.setTheme))
	mux.Handle("PUT /api/account/password", full(s.changePassword))

	mux.Handle("GET /api/system/info", enroll(s.systemInfo))
	mux.Handle("PUT /api/system/channel", admin(s.setChannel))
	mux.Handle("POST /api/system/updates/check", admin(s.checkUpdates))
	mux.Handle("POST /api/system/updates/apply", admin(s.applyUpdate))
	mux.Handle("GET /api/system/updates/status", admin(s.updateStatus))
	mux.Handle("GET /api/system/audit", admin(s.auditLog))
	mux.Handle("GET /api/firewall/trusted", admin(s.trustedGet))
	mux.Handle("PUT /api/firewall/trusted", admin(s.trustedSave))
	mux.Handle("GET /api/system/proxy", admin(s.proxyGet))
	mux.Handle("PUT /api/system/proxy", admin(s.proxySave))
	mux.Handle("GET /api/system/hostnames", admin(s.hostnamesGet))
	mux.Handle("PUT /api/system/hostnames", admin(s.hostnamesSave))

	s.pbxRoutes(mux, admin, recep)
	s.systemRoutes(mux, admin)
	s.businessRoutes(mux, admin, recep)
	s.userRoutes(mux, admin)
	s.welcomeRoutes(mux, admin)
	s.portalRoutes(mux, full)
	s.transferRoutes(mux, full, recep)
	s.internetRoutes(mux, admin)
	s.faxRoutes(mux, admin, recep)
	s.phonebookRoutes(mux, admin, recep, full)
	s.integrationRoutes(mux, admin, recep)
	s.passkeyRoutes(mux, full)
	s.extensionRoutes(mux, admin, recep, full)
	s.siteRoutes(mux, admin)
	s.federationRoutes(mux, admin)
	s.clusterRoutes(mux, admin)
	s.desktopRoutes(mux, admin, full)
	s.callbackRoutes(mux, recep)
	s.aiRoutes(mux, admin, recep, full)
	s.wallboardRoutes(mux, admin, recep)
	s.reportRoutes(mux, admin)
	s.mobileRoutes(mux, admin, full)
	s.crmRoutes(mux, admin, recep, full)
	s.qualityRoutes(mux, admin)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusNotFound, "not_found") })
	return s.jsonOnly(mux)
}

// ---- middleware -------------------------------------------------------------

// jsonOnly rejects state-changing requests that are not JSON. Browsers cannot send a cross-site
// application/json request without a CORS preflight, which we never allow, so this blocks CSRF
// together with the SameSite=Strict cookie.
func (s *Server) jsonOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// The SAML response arrives as a form POST from the identity provider; it is signed and
		// checked against a sign-in this portal started.
		if r.Method == http.MethodPost && r.URL.Path == "/api/auth/saml/acs" {
			next.ServeHTTP(w, r)
			return
		}
		// Firmware arrives as a binary upload. A cross-site page cannot send the custom header without a
		// CORS preflight, which is never allowed, so the header blocks CSRF like the JSON rule does.
		if r.Method == http.MethodPost && r.URL.Path == "/api/firmware/upload" {
			if r.Header.Get("X-Kiestoon-Upload") != "1" {
				writeErr(w, http.StatusForbidden, "upload_header_required")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if ct != "application/json" {
				writeErr(w, http.StatusUnsupportedMediaType, "json_required")
				return
			}
			limit := int64(1 << 20)
			if strings.HasPrefix(r.URL.Path, "/api/pbx/prompts") || strings.HasPrefix(r.URL.Path, "/api/portal/greetings/") {
				limit = 8 << 20 // base64 audio
			}
			if r.URL.Path == "/api/fax/send" {
				limit = 16 << 20 // base64 PDF of up to 10 MB
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) signedIn(next http.Handler, needMFA bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API tokens (Authorization: Bearer kt_…) instead of a session cookie.
		if tsess, bearer := s.tokenSession(r); bearer {
			if tsess == nil {
				writeErr(w, http.StatusUnauthorized, "invalid_token")
				return
			}
			if !tokenAllowed(r, tsess) {
				writeErr(w, http.StatusForbidden, "token_not_allowed")
				return
			}
			tsess = capSession(r, tsess)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, tsess)))
			return
		}
		c, err := r.Cookie(CookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not_signed_in")
			return
		}
		sess, err := s.Auth.SessionFromToken(r.Context(), c.Value)
		if err != nil {
			s.clearCookie(w, r)
			writeErr(w, http.StatusUnauthorized, "not_signed_in")
			return
		}
		// A session that skipped MFA on a trusted network keeps that only while it stays there (and
		// while the setting allows it): elsewhere it has to enter its code first.
		if sess.Method == auth.MethodTrusted && sess.MFAPassed {
			if _, ok := trustedNetworkFor(clientIP(r), sess.User.Role); !ok {
				if err := s.Auth.RequireMFA(r.Context(), sess); err != nil {
					writeErr(w, http.StatusInternalServerError, "internal")
					return
				}
				store.Audit(r.Context(), s.DB, sess.User.ID, "session.mfa_required", "left trusted network", clientIP(r))
			}
		}
		if needMFA && !sess.MFAPassed {
			writeErr(w, http.StatusUnauthorized, "mfa_required")
			return
		}
		if sess.Method == auth.MethodDesktop {
			if !desktopAllowed(r) {
				writeErr(w, http.StatusForbidden, "desktop_not_allowed")
				return
			}
			if sess.Renewed {
				s.setCookieTTL(w, r, c.Value, auth.DesktopTTL)
			}
		}
		sess = capSession(r, sess)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

// mfaEnrolled blocks admins that have not set up MFA yet from everything but enrollment.
func (s *Server) mfaEnrolled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		u := sess.User
		// The identity provider already asked for a second factor.
		// API tokens are created by a signed-in (and so enrolled) user.
		if u.IsAdmin() && !u.TOTPEnabled && sess.Method != "sso-mfa" && !strings.HasPrefix(sess.Method, "token") && sess.Method != "passkey" {
			writeErr(w, http.StatusForbidden, "mfa_enrollment_required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) receptionOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sessionFrom(r).User.IsReception() {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sessionFrom(r).User.IsAdmin() {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr sends a stable error code; the UI translates codes into English or Dutch text.
func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// fromProxy reports whether the request came through a trusted proxy: Caddy of internet mode on
// this machine, or an own reverse proxy listed in the proxy settings.
func fromProxy(r *http.Request) bool {
	a, ok := remoteAddr(r)
	return ok && isTrustedProxy(a) && r.Header.Get("X-Forwarded-For") != ""
}

// clientIP is the address of the browser or phone. Behind a trusted proxy it is the right-most
// address in X-Forwarded-For that is not a trusted proxy itself.
func clientIP(r *http.Request) string {
	if fromProxy(r) {
		if a, ok := forwardedClient(r); ok {
			return a.String()
		}
	}
	if a, ok := remoteAddr(r); ok {
		return a.String()
	}
	return r.RemoteAddr
}

func (s *Server) secure(r *http.Request) bool {
	return s.Cfg.SecureCookies || r.TLS != nil || (fromProxy(r) && r.Header.Get("X-Forwarded-Proto") == "https")
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	s.setCookieTTL(w, r, token, auth.SessionTTL)
}

func (s *Server) setCookieTTL(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure(r),
		SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

// ---- setup ------------------------------------------------------------------

// SetupTokenPath is where the one-time setup token lives. The installer prints it, so only someone
// with access to the console can create the first administrator.
func SetupTokenPath(dataDir string) string { return filepath.Join(dataDir, "setup-token") }

func (s *Server) setupNeeded(ctx context.Context) (bool, error) {
	n, err := s.Auth.UserCount(ctx)
	return n == 0, err
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := s.setupNeeded(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	_, statErr := os.Stat(SetupTokenPath(s.Cfg.DataDir))
	writeJSON(w, http.StatusOK, map[string]any{"setupRequired": needed, "tokenRequired": needed && statErr == nil})
}

type setupReq struct {
	Token        string `json:"token"`
	Organization string `json:"organization"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	Password     string `json:"password"`
	Language     string `json:"language"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req setupReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	needed, err := s.setupNeeded(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if !needed {
		writeErr(w, http.StatusConflict, "setup_done")
		return
	}
	if want, err := os.ReadFile(SetupTokenPath(s.Cfg.DataDir)); err == nil {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(want))), []byte(strings.TrimSpace(req.Token))) != 1 {
			s.loginLimit.Fail(ip)
			writeErr(w, http.StatusForbidden, "invalid_setup_token")
			return
		}
	}
	u, err := s.Auth.CreateUser(r.Context(), req.Username, req.DisplayName, req.Password, "superadmin", req.Language)
	if errors.Is(err, auth.ErrWeakPassword) {
		writeErr(w, http.StatusBadRequest, "weak_password")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_user")
		return
	}
	org := strings.TrimSpace(req.Organization)
	if org == "" {
		org = "Kiestoon"
	}
	_ = store.SetSetting(r.Context(), s.DB, "organization", org)
	_ = os.Remove(SetupTokenPath(s.Cfg.DataDir))
	store.Audit(r.Context(), s.DB, u.ID, "setup.completed", u.Username, ip)

	// No MFA yet, so the session is "MFA passed" but every route except enrollment stays blocked.
	token, err := s.Auth.CreateSession(r.Context(), u, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.setCookie(w, r, token)
	writeJSON(w, http.StatusCreated, s.meBody(r, u, true))
}

// ---- auth -------------------------------------------------------------------

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u, err := s.Auth.CheckPassword(r.Context(), req.Username, req.Password)
	if err != nil {
		s.loginLimit.Fail(ip)
		s.Log.Warn("login failed", "ip", ip)
		store.Audit(r.Context(), s.DB, 0, "login.failed", req.Username, ip)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	mfaRequired := u.TOTPEnabled
	method := "password"
	// From a trusted network (office LAN, VPN) the second factor may be skipped for some roles.
	tnet, trustedNet := TrustedNetwork{}, false
	if mfaRequired {
		if tnet, trustedNet = trustedNetworkFor(ip, u.Role); trustedNet {
			mfaRequired, method = false, auth.MethodTrusted
		}
	}
	token, err := s.Auth.CreateSessionMethod(r.Context(), u, !mfaRequired, method)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if !mfaRequired {
		s.loginLimit.Reset(ip)
		if trustedNet {
			detail := "mfa skipped: trusted network " + tnet.CIDR
			if tnet.Description != "" {
				detail += " (" + tnet.Description + ")"
			}
			store.Audit(r.Context(), s.DB, u.ID, "login.mfa_skipped", detail, ip)
		} else {
			store.Audit(r.Context(), s.DB, u.ID, "login.success", "", ip)
		}
	}
	s.setCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"mfaRequired": mfaRequired})
}

func (s *Server) verifyMFA(w http.ResponseWriter, r *http.Request) {
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
	sess := sessionFrom(r)
	if err := s.Auth.VerifySecondFactor(r.Context(), sess.User, req.Code); err != nil {
		s.loginLimit.Fail(ip)
		s.Log.Warn("mfa failed", "ip", ip)
		store.Audit(r.Context(), s.DB, sess.User.ID, "mfa.failed", "", ip)
		writeErr(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	if err := s.Auth.MarkMFAPassed(r.Context(), sess); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.loginLimit.Reset(ip)
	store.Audit(r.Context(), s.DB, sess.User.ID, "login.success", "mfa", ip)
	writeJSON(w, http.StatusOK, s.meBody(r, sess.User, true))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	_ = s.Auth.DeleteSession(r.Context(), sessionFrom(r))
	s.clearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) meBody(r *http.Request, u *auth.User, mfaPassed bool) map[string]any {
	return s.meBodyMethod(r, u, mfaPassed, "password")
}

func (s *Server) meBodyMethod(r *http.Request, u *auth.User, mfaPassed bool, method string) map[string]any {
	ctx := r.Context()
	// On the employee portal host name with administration blocked, administrators see the portal.
	adminElsewhere := ""
	if adminBlockedHere(r) {
		// The session may already carry the reduced role; the stored account tells whether this is an administrator.
		if real, err := s.Auth.UserByID(ctx, u.ID); err == nil && real.IsAdmin() {
			adminElsewhere = s.pbxBase(r)
		}
		u = hostUser(r, u)
	}
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	var ext map[string]string
	if u.ExtensionID != nil {
		if e, err := s.PBX.Extension(ctx, *u.ExtensionID); err == nil {
			ext = map[string]string{"number": e.Number, "name": e.Name}
		}
	}
	return map[string]any{
		"user":                  u,
		"mfaPassed":             mfaPassed,
		"mfaEnrollmentRequired": u.IsAdmin() && !u.TOTPEnabled && method != "sso-mfa" && method != "passkey",
		"organization":          org,
		"extension":             ext,
		"signInMethod":          method,
		"adminUrl":              adminElsewhere,
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	writeJSON(w, http.StatusOK, s.meBodyMethod(r, sess.User, sess.MFAPassed, sess.Method))
}

func (s *Server) mfaEnroll(w http.ResponseWriter, r *http.Request) {
	u := sessionFrom(r).User
	if u.TOTPEnabled {
		writeErr(w, http.StatusConflict, "mfa_already_enabled")
		return
	}
	secret, url, err := s.Auth.BeginTOTPEnrollment(r.Context(), u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauthUrl": url})
}

func (s *Server) mfaConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sess := sessionFrom(r)
	codes, err := s.Auth.ConfirmTOTPEnrollment(r.Context(), sess.User, req.Code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_code")
		return
	}
	_ = s.Auth.MarkMFAPassed(r.Context(), sess)
	store.Audit(r.Context(), s.DB, sess.User.ID, "mfa.enabled", "totp", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) setLanguage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language string `json:"language"`
	}
	if err := decode(r, &req); err != nil || (req.Language != "en" && req.Language != "nl") {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.Auth.SetLanguage(r.Context(), sessionFrom(r).User.ID, req.Language); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"language": req.Language})
}

func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Theme string `json:"theme"`
	}
	if err := decode(r, &req); err != nil || !auth.ValidTheme(req.Theme) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.Auth.SetTheme(r.Context(), sessionFrom(r).User.ID, req.Theme); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"theme": req.Theme})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u := sessionFrom(r).User
	switch err := s.Auth.ChangePassword(r.Context(), u, req.Current, req.New); {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeErr(w, http.StatusBadRequest, "invalid_current_password")
	case errors.Is(err, auth.ErrWeakPassword):
		writeErr(w, http.StatusBadRequest, "weak_password")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal")
	default:
		store.Audit(r.Context(), s.DB, u.ID, "password.changed", "", clientIP(r))
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---- system -----------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

func (s *Server) channel(ctx context.Context) string {
	c, _ := store.GetSetting(ctx, s.DB, "update_channel", s.Cfg.DefaultChannel)
	if !updates.ValidChannel(c) {
		return updates.ChannelProduction
	}
	return c
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildDate": version.BuildDate,
		"channel":   s.channel(r.Context()),
		"hostname":  host,
		"uptime":    int64(time.Since(s.started).Seconds()),
		"repo":      s.Cfg.Repo,
	})
}

func (s *Server) setChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := decode(r, &req); err != nil || !updates.ValidChannel(req.Channel) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := store.SetSetting(r.Context(), s.DB, "update_channel", req.Channel); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(r.Context(), s.DB, sessionFrom(r).User.ID, "updates.channel", req.Channel, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"channel": req.Channel})
}

func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request) {
	res, err := s.Updates.Check(r.Context(), version.Version, s.channel(r.Context()), true)
	if err != nil {
		s.Log.Warn("update check failed", "err", err)
		writeErr(w, http.StatusBadGateway, "update_check_failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if err := decode(r, &req); err != nil || req.Version == "" {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ch := s.channel(r.Context())
	rel, err := s.Updates.Find(r.Context(), req.Version, ch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "unknown_version")
		return
	}
	u := sessionFrom(r).User
	err = s.Updates.RequestUpdate(updates.Request{Version: rel.Version, Channel: ch, RequestedAt: time.Now().UTC(), RequestedBy: u.Username})
	if errors.Is(err, updates.ErrBusy) {
		writeErr(w, http.StatusConflict, "update_in_progress")
		return
	}
	if err != nil {
		s.Log.Error("queue update", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(r.Context(), s.DB, u.ID, "updates.requested", rel.Version, clientIP(r))
	writeJSON(w, http.StatusAccepted, s.Updates.Status())
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Updates.Status())
}

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id, a.at, COALESCE(u.username, ''), a.action, a.detail, a.ip
		FROM audit_log a LEFT JOIN users u ON u.id = a.user_id ORDER BY a.id DESC LIMIT 200`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	type entry struct {
		ID     int64  `json:"id"`
		At     string `json:"at"`
		User   string `json:"user"`
		Action string `json:"action"`
		Detail string `json:"detail"`
		IP     string `json:"ip"`
	}
	out := []entry{}
	for rows.Next() {
		var e entry
		if rows.Scan(&e.ID, &e.At, &e.User, &e.Action, &e.Detail, &e.IP) == nil {
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// SecurityHeaders adds strict browser security headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedHost(r)
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
		next.ServeHTTP(w, r)
	})
}
