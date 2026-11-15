// Package sso signs users in with OpenID Connect (Microsoft Entra ID, Google Workspace, Keycloak,
// Authentik or any other OIDC provider), using the authorization code flow with PKCE.
package sso

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// RoleMapping gives members of an identity provider group a Kiestoon role.
type RoleMapping struct {
	Group string `json:"group"`
	Role  string `json:"role"`
}

// Provider is a configured identity provider.
type Provider struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Kind         string        `json:"kind"` // entra | google | keycloak | authentik | generic
	Issuer       string        `json:"issuer"`
	ClientID     string        `json:"clientId"`
	ClientSecret string        `json:"clientSecret,omitempty"`
	SecretSet    bool          `json:"secretSet"`
	Scopes       string        `json:"scopes"`
	GroupsClaim  string        `json:"groupsClaim"`
	RoleMap      []RoleMapping `json:"roleMap"`
	// DefaultRole is given when no group matches. Empty: sign-in is refused.
	DefaultRole string `json:"defaultRole"`
	AutoCreate  bool   `json:"autoCreate"`
	// TrustMFA skips Kiestoon's own second factor, because the provider already asks for one.
	TrustMFA bool `json:"trustMfa"`
	Enabled  bool `json:"enabled"`
}

// Kinds are the provider templates offered in the UI.
var Kinds = []string{"entra", "google", "keycloak", "authentik", "generic"}

var roles = []string{"superadmin", "admin", "reception", "user"}

func rank(r string) int { return len(roles) - slices.Index(roles, r) }

// Validate checks a provider.
func (p *Provider) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Issuer = strings.TrimRight(strings.TrimSpace(p.Issuer), "/")
	p.ClientID = strings.TrimSpace(p.ClientID)
	if p.Name == "" || len(p.Name) > 60 {
		return fieldErr("name")
	}
	if !slices.Contains(Kinds, p.Kind) {
		return fieldErr("kind")
	}
	u, err := url.Parse(p.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fieldErr("issuer")
	}
	if p.ClientID == "" || len(p.ClientID) > 200 {
		return fieldErr("clientId")
	}
	if p.Scopes == "" {
		p.Scopes = "openid profile email"
	}
	if !strings.Contains(" "+p.Scopes+" ", " openid ") {
		p.Scopes = "openid " + p.Scopes
	}
	if p.GroupsClaim == "" {
		p.GroupsClaim = "groups"
	}
	if p.DefaultRole != "" && !slices.Contains(roles, p.DefaultRole) {
		return fieldErr("defaultRole")
	}
	if p.RoleMap == nil {
		p.RoleMap = []RoleMapping{}
	}
	for i := range p.RoleMap {
		p.RoleMap[i].Group = strings.TrimSpace(p.RoleMap[i].Group)
		if p.RoleMap[i].Group == "" || !slices.Contains(roles, p.RoleMap[i].Role) {
			return fieldErr("roleMap")
		}
	}
	return nil
}

// FieldError names the invalid field.
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "invalid " + e.Field }

func fieldErr(f string) error { return &FieldError{Field: f} }

// Store reads and writes providers.
type Store struct{ DB *sql.DB }

const cols = `id, name, kind, issuer, client_id, client_secret, scopes, groups_claim, role_map, default_role, auto_create, trust_mfa, enabled`

func scan(row interface{ Scan(...any) error }) (*Provider, error) {
	var p Provider
	var rm string
	var ac, tm, en int
	if err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.Issuer, &p.ClientID, &p.ClientSecret, &p.Scopes, &p.GroupsClaim, &rm,
		&p.DefaultRole, &ac, &tm, &en); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(rm), &p.RoleMap)
	if p.RoleMap == nil {
		p.RoleMap = []RoleMapping{}
	}
	p.AutoCreate, p.TrustMFA, p.Enabled, p.SecretSet = ac == 1, tm == 1, en == 1, p.ClientSecret != ""
	return &p, nil
}

// List returns all providers (with secrets; strip them before sending to a browser).
func (s *Store) List(ctx context.Context) ([]Provider, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM sso_providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Get returns one provider.
func (s *Store) Get(ctx context.Context, id int64) (*Provider, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM sso_providers WHERE id = ?`, id))
}

// Save validates and stores a provider. An empty secret on update keeps the stored one.
func (s *Store) Save(ctx context.Context, p *Provider) error {
	if p.ID != 0 && p.ClientSecret == "" {
		old, err := s.Get(ctx, p.ID)
		if err != nil {
			return err
		}
		p.ClientSecret = old.ClientSecret
	}
	if err := p.Validate(); err != nil {
		return err
	}
	rm, _ := json.Marshal(p.RoleMap)
	args := []any{p.Name, p.Kind, p.Issuer, p.ClientID, p.ClientSecret, p.Scopes, p.GroupsClaim, string(rm), p.DefaultRole,
		b2i(p.AutoCreate), b2i(p.TrustMFA), b2i(p.Enabled)}
	if p.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO sso_providers(name, kind, issuer, client_id, client_secret, scopes, groups_claim,
			role_map, default_role, auto_create, trust_mfa, enabled) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		p.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE sso_providers SET name=?, kind=?, issuer=?, client_id=?, client_secret=?, scopes=?,
		groups_claim=?, role_map=?, default_role=?, auto_create=?, trust_mfa=?, enabled=? WHERE id=?`, append(args, p.ID)...)
	return err
}

// Delete removes a provider. Accounts that used it keep existing but can only sign in with a password.
func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sso_providers WHERE id = ?`, id)
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE users SET sso_provider = 0, sso_subject = '' WHERE sso_provider = ?`, id)
	}
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- sign-in flow -----------------------------------------------------------

type pending struct {
	provider    int64
	nonce       string
	verifier    string
	redirectURI string
	expires     time.Time
}

// Flow keeps the state of sign-ins in progress (in memory: they only last a few minutes).
type Flow struct {
	mu       sync.Mutex
	pending  map[string]pending
	discover map[string]*oidc.Provider
}

// NewFlow returns an empty Flow.
func NewFlow() *Flow {
	return &Flow{pending: map[string]pending{}, discover: map[string]*oidc.Provider{}}
}

func random() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (f *Flow) oidcProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	f.mu.Lock()
	p := f.discover[issuer]
	f.mu.Unlock()
	if p != nil {
		return p, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(cctx, issuer)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.discover[issuer] = p
	f.mu.Unlock()
	return p, nil
}

// Forget drops the cached discovery document (after a provider changed).
func (f *Flow) Forget(issuer string) {
	f.mu.Lock()
	delete(f.discover, issuer)
	f.mu.Unlock()
}

func config(op *oidc.Provider, p *Provider, redirectURI string) *oauth2.Config {
	return &oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, Endpoint: op.Endpoint(), RedirectURL: redirectURI,
		Scopes: strings.Fields(p.Scopes)}
}

// Start returns the state (to store in a cookie) and the URL to send the browser to.
func (f *Flow) Start(ctx context.Context, p *Provider, redirectURI string) (state, authURL string, err error) {
	op, err := f.oidcProvider(ctx, p.Issuer)
	if err != nil {
		return "", "", err
	}
	state = random()
	pd := pending{provider: p.ID, nonce: random(), verifier: oauth2.GenerateVerifier(), redirectURI: redirectURI,
		expires: time.Now().Add(10 * time.Minute)}
	f.mu.Lock()
	for k, v := range f.pending {
		if time.Now().After(v.expires) {
			delete(f.pending, k)
		}
	}
	if len(f.pending) > 1000 {
		f.mu.Unlock()
		return "", "", errors.New("too many sign-ins in progress")
	}
	f.pending[state] = pd
	f.mu.Unlock()
	opts := []oauth2.AuthCodeOption{oidc.Nonce(pd.nonce), oauth2.S256ChallengeOption(pd.verifier)}
	if p.Kind == "entra" || p.Kind == "google" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return state, config(op, p, redirectURI).AuthCodeURL(state, opts...), nil
}

// Identity is what Kiestoon learns about the person from the provider.
type Identity struct {
	ProviderID int64
	Subject    string
	Email      string
	Username   string
	Name       string
	Groups     []string
}

// PendingProvider returns the provider of a sign-in in progress.
func (f *Flow) PendingProvider(state string) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pd, ok := f.pending[state]
	if !ok || time.Now().After(pd.expires) {
		return 0, false
	}
	return pd.provider, true
}

// Finish exchanges the code and verifies the ID token.
func (f *Flow) Finish(ctx context.Context, p *Provider, state, code string) (*Identity, error) {
	f.mu.Lock()
	pd, ok := f.pending[state]
	delete(f.pending, state)
	f.mu.Unlock()
	if !ok || time.Now().After(pd.expires) || pd.provider != p.ID {
		return nil, errors.New("unknown or expired sign-in")
	}
	op, err := f.oidcProvider(ctx, p.Issuer)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := config(op, p, pd.redirectURI).Exchange(cctx, code, oauth2.VerifierOption(pd.verifier))
	if err != nil {
		return nil, fmt.Errorf("code exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return nil, errors.New("no id_token in the response")
	}
	idt, err := op.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(cctx, raw)
	if err != nil {
		return nil, fmt.Errorf("id token: %w", err)
	}
	if idt.Nonce != pd.nonce {
		return nil, errors.New("nonce mismatch")
	}
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		return nil, err
	}
	id := &Identity{ProviderID: p.ID, Subject: idt.Subject}
	str := func(k string) string { s, _ := claims[k].(string); return s }
	id.Email = str("email")
	id.Name = str("name")
	id.Username = str("preferred_username")
	if id.Username == "" {
		id.Username = str("upn")
	}
	if id.Email == "" && strings.Contains(id.Username, "@") {
		id.Email = id.Username
	}
	if id.Username == "" {
		id.Username = id.Email
	}
	id.Groups = groupsOf(claims[p.GroupsClaim])
	if p.GroupsClaim != "roles" {
		// Entra ID app roles come in "roles": accept them as groups too.
		id.Groups = append(id.Groups, groupsOf(claims["roles"])...)
	}
	return id, nil
}

func groupsOf(v any) []string {
	out := []string{}
	switch g := v.(type) {
	case []any:
		for _, x := range g {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case string:
		out = append(out, strings.Fields(strings.ReplaceAll(g, ",", " "))...)
	}
	return out
}

// RoleFor maps the groups of an identity to a role. ok is false when sign-in must be refused.
func RoleFor(p *Provider, groups []string) (role string, ok bool) {
	best := ""
	for _, m := range p.RoleMap {
		for _, g := range groups {
			if strings.EqualFold(g, m.Group) && (best == "" || rank(m.Role) > rank(best)) {
				best = m.Role
			}
		}
	}
	if best != "" {
		return best, true
	}
	if p.DefaultRole != "" {
		return p.DefaultRole, true
	}
	return "", false
}
