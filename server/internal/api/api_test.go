package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

type client struct {
	t   *testing.T
	url string
	hc  *http.Client
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func newTestServer(t *testing.T) (*client, *Server) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, Repo: "o/r", DefaultChannel: "production"}
	db, err := store.Open(context.Background(), cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(SecurityHeaders(s.Routes()))
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, url: ts.URL, hc: &http.Client{Jar: jar}}, s
}

func TestSetupLoginAndMandatoryMFA(t *testing.T) {
	c, s := newTestServer(t)
	os.WriteFile(SetupTokenPath(s.Cfg.DataDir), []byte("ABCD-1234\n"), 0o600)

	if code, body := c.do("GET", "/api/setup/status", nil); code != 200 || body["setupRequired"] != true || body["tokenRequired"] != true {
		t.Fatalf("setup status: %d %v", code, body)
	}
	setup := map[string]string{"token": "wrong", "username": "admin", "password": "correct horse battery", "language": "nl"}
	if code, _ := c.do("POST", "/api/setup", setup); code != http.StatusForbidden {
		t.Fatalf("wrong token accepted: %d", code)
	}
	setup["token"] = "abcd-1234"
	setup["token"] = "ABCD-1234"
	if code, body := c.do("POST", "/api/setup", setup); code != http.StatusCreated || body["mfaEnrollmentRequired"] != true {
		t.Fatalf("setup: %d %v", code, body)
	}
	if code, _ := c.do("POST", "/api/setup", setup); code != http.StatusConflict {
		t.Fatalf("second setup allowed: %d", code)
	}

	// Admin without MFA is locked out of everything except enrollment.
	if code, body := c.do("PUT", "/api/system/channel", map[string]string{"channel": "testing"}); code != 403 || body["error"] != "mfa_enrollment_required" {
		t.Fatalf("expected enrollment block: %d %v", code, body)
	}
	_, enr := c.do("POST", "/api/account/mfa/enroll", map[string]any{})
	secret := enr["secret"].(string)
	if code, _ := c.do("POST", "/api/account/mfa/confirm", map[string]string{"code": "000000"}); code != 400 {
		t.Fatal("bad code accepted")
	}
	now, _ := totp.GenerateCode(secret, time.Now())
	code, conf := c.do("POST", "/api/account/mfa/confirm", map[string]string{"code": now})
	if code != 200 || len(conf["recoveryCodes"].([]any)) != 8 {
		t.Fatalf("confirm: %d %v", code, conf)
	}
	recovery := conf["recoveryCodes"].([]any)[0].(string)

	if code, _ := c.do("PUT", "/api/system/channel", map[string]string{"channel": "testing"}); code != 200 {
		t.Fatalf("channel switch after MFA: %d", code)
	}
	if _, info := c.do("GET", "/api/system/info", nil); info["channel"] != "testing" {
		t.Fatalf("channel not stored: %v", info)
	}

	// Log out and back in: the second factor is now required.
	c.do("POST", "/api/auth/logout", map[string]any{})
	if code, _ := c.do("GET", "/api/me", nil); code != 401 {
		t.Fatal("still signed in after logout")
	}
	if code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "ADMIN", "password": "correct horse battery"}); code != 200 || body["mfaRequired"] != true {
		t.Fatalf("login: %d %v", code, body)
	}
	if code, body := c.do("GET", "/api/me", nil); code != 401 || body["error"] != "mfa_required" {
		t.Fatalf("me before MFA: %d %v", code, body)
	}
	// The TOTP code used during enrollment cannot be replayed.
	if code, _ := c.do("POST", "/api/auth/mfa", map[string]string{"code": now}); code != 401 {
		t.Fatal("replayed TOTP code accepted")
	}
	if code, _ := c.do("POST", "/api/auth/mfa", map[string]string{"code": recovery}); code != 200 {
		t.Fatal("recovery code rejected")
	}
	if code, body := c.do("GET", "/api/me", nil); code != 200 || body["mfaEnrollmentRequired"] != false {
		t.Fatalf("me after MFA: %d %v", code, body)
	}
}

func TestLoginRateLimitAndJSONOnly(t *testing.T) {
	c, _ := newTestServer(t)
	for i := 0; i < 5; i++ {
		c.do("POST", "/api/auth/login", map[string]string{"username": "x", "password": "y"})
	}
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "x", "password": "y"}); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", code)
	}
	resp, _ := c.hc.Post(c.url+"/api/auth/login", "application/x-www-form-urlencoded", bytes.NewBufferString("a=b"))
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("form post accepted: %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing security headers")
	}
}

func jsonDecode(resp *http.Response, v any) error { return json.NewDecoder(resp.Body).Decode(v) }

func TestClientIPTrustedProxies(t *testing.T) {
	defer setTrustedProxies(nil)
	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	setTrustedProxies(nil)
	if got := clientIP(req("10.0.0.5:1234", "1.2.3.4")); got != "10.0.0.5" {
		t.Fatalf("untrusted proxy must not set the address, got %s", got)
	}
	if got := clientIP(req("127.0.0.1:1234", "1.2.3.4")); got != "1.2.3.4" {
		t.Fatalf("loopback proxy, got %s", got)
	}
	setTrustedProxies([]string{"10.0.0.0/24", "192.168.1.10"})
	if got := clientIP(req("10.0.0.5:1234", "6.6.6.6, 1.2.3.4")); got != "1.2.3.4" {
		t.Fatalf("trusted proxy, got %s", got)
	}
	if got := clientIP(req("10.0.0.5:1234", "6.6.6.6, 1.2.3.4, 192.168.1.10")); got != "1.2.3.4" {
		t.Fatalf("chained proxies, got %s", got)
	}
	if got := clientIP(req("[::ffff:10.0.0.5]:1234", "1.2.3.4")); got != "1.2.3.4" {
		t.Fatalf("mapped address, got %s", got)
	}
	if got := clientIP(req("10.0.1.5:1234", "1.2.3.4")); got != "10.0.1.5" {
		t.Fatalf("outside the range, got %s", got)
	}
	p := ProxySettings{Enabled: true, TrustedProxies: []string{"0.0.0.0/0"}}
	if p.Validate() == nil {
		t.Fatal("0.0.0.0/0 must be refused")
	}
	p = ProxySettings{Enabled: true, TrustedProxies: []string{" 10.0.0.7 ", "10.1.2.3/16"}, PublicURL: "https://pbx.example.com/"}
	if err := p.Validate(); err != nil || p.TrustedProxies[1] != "10.1.0.0/16" || p.PublicURL != "https://pbx.example.com" {
		t.Fatalf("validate: %v %+v", err, p)
	}
}
