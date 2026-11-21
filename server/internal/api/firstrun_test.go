package api

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"testing"
)

func TestInitialAdminFirstRun(t *testing.T) {
	c, s := newTestServer(t)
	ctx := context.Background()
	if err := s.EnsureInitialAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	a, ok := LoadInitialAdmin(s.Cfg.DataDir)
	if !ok || a.Username != "admin" || a.ID == 0 || !regexp.MustCompile(`^[A-Za-z2-9]{4}(-[A-Za-z2-9]{4}){3}$`).MatchString(a.Password) {
		t.Fatalf("initial admin = %+v", a)
	}
	// A second start keeps the same account and password.
	if err := s.EnsureInitialAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if b, _ := LoadInitialAdmin(s.Cfg.DataDir); b.Password != a.Password {
		t.Fatal("password changed on restart")
	}
	if n, _ := s.Auth.UserCount(ctx); n != 1 {
		t.Fatalf("users = %d", n)
	}
	if _, body := c.do("GET", "/api/setup/status", nil); body["setupRequired"] != false || body["initialAdmin"] != true {
		t.Fatalf("status = %v", body)
	}

	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": a.Password}); code != 200 {
		t.Fatalf("login = %d", code)
	}
	if _, me := c.do("GET", "/api/me", nil); me["firstRunRequired"] != true {
		t.Fatalf("me = %v", me)
	}
	// Nothing else works before the first run is done.
	if code, body := c.do("GET", "/api/ai", nil); code != http.StatusForbidden || body["error"] != "first_run_required" {
		t.Fatalf("extensions = %d %v", code, body)
	}
	if code, body := c.do("POST", "/api/setup/first-run", map[string]string{"organization": "Acme", "password": a.Password}); code != 400 || body["field"] != "password" {
		t.Fatalf("same password = %d %v", code, body)
	}
	if code, body := c.do("POST", "/api/setup/first-run", map[string]string{"organization": "Acme", "password": "short"}); code != 400 || body["error"] != "weak_password" {
		t.Fatalf("weak = %d %v", code, body)
	}
	code, me := c.do("POST", "/api/setup/first-run", map[string]string{"organization": "Acme", "displayName": "Dana", "password": "correct horse battery", "language": "nl"})
	if code != 200 || me["firstRunRequired"] != false || me["mfaEnrollmentRequired"] != true || me["organization"] != "Acme" {
		t.Fatalf("first run = %d %v", code, me)
	}
	if _, err := os.Stat(InitialAdminPath(s.Cfg.DataDir)); !os.IsNotExist(err) {
		t.Fatal("initial-admin file still there")
	}
	// Next the administrator sets up two-factor authentication, as before.
	if code, body := c.do("GET", "/api/ai", nil); code != http.StatusForbidden || body["error"] != "mfa_enrollment_required" {
		t.Fatalf("extensions after = %d %v", code, body)
	}
	if code, _ := c.do("POST", "/api/setup/first-run", map[string]string{"organization": "X", "password": "another long password"}); code != http.StatusConflict {
		t.Fatalf("second first run = %d", code)
	}
	if _, err := s.Auth.CheckPassword(ctx, "admin", "correct horse battery"); err != nil {
		t.Fatal("new password does not work")
	}
	if code, _ := c.do("POST", "/api/setup/first-run/cluster-join", map[string]string{"code": "x"}); code != http.StatusConflict {
		t.Fatalf("cluster join after first run = %d", code)
	}
}
