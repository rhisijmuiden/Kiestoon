package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Passkeys (WebAuthn) sign a user in without a password. The authenticator checks the user
// (fingerprint, face or PIN), so a passkey sign-in also counts as the second factor.

func (s *Server) passkeyRoutes(mux *http.ServeMux, full func(http.HandlerFunc) http.Handler) {
	mux.HandleFunc("POST /api/auth/passkey/begin", s.passkeyLoginBegin)
	mux.HandleFunc("POST /api/auth/passkey/finish", s.passkeyLoginFinish)
	mux.Handle("GET /api/account/passkeys", full(s.passkeyList))
	mux.Handle("POST /api/account/passkeys/begin", full(s.passkeyRegisterBegin))
	mux.Handle("POST /api/account/passkeys/finish", full(s.passkeyRegisterFinish))
	mux.Handle("DELETE /api/account/passkeys/{id}", full(s.passkeyDelete))
}

type passkeyCeremony struct {
	data    webauthn.SessionData
	userID  int64
	expires time.Time
}

type passkeyStore struct {
	mu sync.Mutex
	m  map[string]passkeyCeremony
}

func (p *passkeyStore) put(key string, c passkeyCeremony) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]passkeyCeremony{}
	}
	now := time.Now()
	for k, v := range p.m {
		if now.After(v.expires) {
			delete(p.m, k)
		}
	}
	if len(p.m) > 1000 {
		return
	}
	p.m[key] = c
}

func (p *passkeyStore) take(key string) (passkeyCeremony, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.m[key]
	delete(p.m, key)
	if !ok || time.Now().After(c.expires) {
		return passkeyCeremony{}, false
	}
	return c, true
}

// waUser adapts a Kiestoon user to WebAuthn.
type waUser struct {
	u     *auth.User
	creds []webauthn.Credential
}

func userHandle(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

func (w waUser) WebAuthnID() []byte                         { return userHandle(w.u.ID) }
func (w waUser) WebAuthnName() string                       { return w.u.Username }
func (w waUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }
func (w waUser) WebAuthnDisplayName() string {
	if w.u.DisplayName != "" {
		return w.u.DisplayName
	}
	return w.u.Username
}

// relyingParty is built per request: the passkey belongs to the host name the browser uses.
func (s *Server) relyingParty(r *http.Request) (*webauthn.WebAuthn, error) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if net.ParseIP(host) != nil {
		return nil, errors.New("passkey_needs_hostname")
	}
	org, _ := store.GetSetting(r.Context(), s.DB, "organization", "Kiestoon")
	return webauthn.New(&webauthn.Config{RPID: host, RPDisplayName: org, RPOrigins: []string{s.scheme(r) + "://" + r.Host}})
}

func (s *Server) loadPasskeys(ctx context.Context, userID int64) ([]webauthn.Credential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []webauthn.Credential{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if json.Unmarshal([]byte(data), &c) == nil {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

type passkeyInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
}

func (s *Server) passkeyList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id, name, created_at, last_used_at FROM passkeys WHERE user_id = ? ORDER BY id`,
		sessionFrom(r).User.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	out := []passkeyInfo{}
	for rows.Next() {
		var p passkeyInfo
		if rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt) == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	wa, err := s.relyingParty(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_needs_hostname")
		return
	}
	u := sessionFrom(r).User
	creds, err := s.loadPasskeys(r.Context(), u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	exclude := []protocol.CredentialDescriptor{}
	for _, c := range creds {
		exclude = append(exclude, c.Descriptor())
	}
	opts, data, err := wa.BeginRegistration(waUser{u: u, creds: creds},
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}),
		webauthn.WithExclusions(exclude))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.passkeys.put("reg-"+hex.EncodeToString(userHandle(u.ID)), passkeyCeremony{data: *data, userID: u.ID, expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 60 {
		fieldErr(w, "name")
		return
	}
	u := sessionFrom(r).User
	c, ok := s.passkeys.take("reg-" + hex.EncodeToString(userHandle(u.ID)))
	if !ok {
		writeErr(w, http.StatusBadRequest, "passkey_expired")
		return
	}
	wa, err := s.relyingParty(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_needs_hostname")
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(req.Credential))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_invalid")
		return
	}
	creds, _ := s.loadPasskeys(r.Context(), u.ID)
	cred, err := wa.CreateCredential(waUser{u: u, creds: creds}, c.data, parsed)
	if err != nil {
		s.Log.Info("passkey registration failed", "err", err)
		writeErr(w, http.StatusBadRequest, "passkey_invalid")
		return
	}
	data, _ := json.Marshal(cred)
	res, err := s.DB.ExecContext(r.Context(), `INSERT INTO passkeys(user_id, name, credential_id, data) VALUES (?,?,?,?)`,
		u.ID, req.Name, hex.EncodeToString(cred.ID), string(data))
	if err != nil {
		writeErr(w, http.StatusConflict, "passkey_exists")
		return
	}
	id, _ := res.LastInsertId()
	s.audit(r, "passkey.added", req.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM passkeys WHERE id = ? AND user_id = ?`, pathID(r), sessionFrom(r).User.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "passkey.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if s.loginLimit.Blocked(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	wa, err := s.relyingParty(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_needs_hostname")
		return
	}
	opts, data, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	key := hex.EncodeToString(b)
	s.passkeys.put("login-"+key, passkeyCeremony{data: *data, expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, http.StatusOK, map[string]any{"id": key, "options": opts})
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		ID         string          `json:"id"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	c, ok := s.passkeys.take("login-" + req.ID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "passkey_expired")
		return
	}
	wa, err := s.relyingParty(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_needs_hostname")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(req.Credential))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "passkey_invalid")
		return
	}
	ctx := r.Context()
	var found *auth.User
	handler := func(_, handle []byte) (webauthn.User, error) {
		if len(handle) != 8 {
			return nil, errors.New("unknown user")
		}
		u, err := s.Auth.UserByID(ctx, int64(binary.BigEndian.Uint64(handle)))
		if err != nil || !u.Enabled {
			return nil, errors.New("unknown user")
		}
		creds, err := s.loadPasskeys(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		found = u
		return waUser{u: u, creds: creds}, nil
	}
	cred, err := wa.ValidateDiscoverableLogin(handler, c.data, parsed)
	if err != nil || found == nil {
		s.loginLimit.Fail(ip)
		s.Log.Warn("passkey login failed", "ip", ip)
		store.Audit(ctx, s.DB, 0, "login.failed", "passkey", ip)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	data, _ := json.Marshal(cred)
	_, _ = s.DB.ExecContext(ctx, `UPDATE passkeys SET data = ?, last_used_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE credential_id = ? AND user_id = ?`,
		string(data), hex.EncodeToString(cred.ID), found.ID)
	token, err := s.Auth.CreateSessionMethod(ctx, found, true, "passkey")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.loginLimit.Reset(ip)
	store.Audit(ctx, s.DB, found.ID, "login.success", "passkey", ip)
	s.setCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"mfaRequired": false})
}
