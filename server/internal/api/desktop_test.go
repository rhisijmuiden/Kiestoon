package api

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDesktopPairingAndScope(t *testing.T) {
	c, s := adminClient(t)
	s.Cfg.SoftphoneDir = t.TempDir()
	e := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1&lang=en", map[string]any{"number": "201", "name": "Anna",
		"email": "anna@example.com", "enabled": true}, 200)
	link := e["welcome"].(map[string]any)["link"].(string)
	token := link[strings.Index(link, "#")+1:]
	extID := int64(e["id"].(float64))

	newClient := func() *client {
		jar, _ := cookiejar.New(nil)
		return &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	}
	anna := newClient()
	mustDo(t, anna, "POST", "/api/auth/password-link/set", map[string]string{"token": token, "password": "a long enough password"}, 200)
	mustDo(t, anna, "POST", "/api/auth/login", map[string]string{"username": "anna@example.com", "password": "a long enough password"}, 200)

	// The admin has no phone, so there is nothing to link.
	mustDo(t, c, "POST", "/api/desktop/pair", map[string]any{}, 409)

	p := mustDo(t, anna, "POST", "/api/desktop/pair", map[string]any{}, 200)
	code := p["code"].(string)
	if len(code) != 9 || !strings.HasPrefix(p["link"].(string), "kiestoon://pair?") || !strings.Contains(p["link"].(string), "code="+code) {
		t.Fatalf("pair: %v", p)
	}

	desk := newClient()
	mustDo(t, desk, "POST", "/api/desktop/claim", map[string]string{"code": "AAAA-AAAA"}, 401)
	// Lower case and without the dash works too.
	mustDo(t, desk, "POST", "/api/desktop/claim", map[string]string{"code": strings.ToLower(strings.ReplaceAll(code, "-", ""))}, 200)
	mustDo(t, newClient(), "POST", "/api/desktop/claim", map[string]string{"code": code}, 401) // used once

	// The desktop session reaches the user portal, never the admin API.
	mustDo(t, desk, "GET", "/api/portal/me", nil, 200)
	mustDo(t, desk, "GET", "/api/me", nil, 200)
	if code, out := desk.do("GET", "/api/pbx/extensions", nil); code != 403 || out["error"] != "desktop_not_allowed" {
		t.Fatalf("admin api from desktop: %d %v", code, out)
	}
	mustDo(t, desk, "PUT", "/api/account/password", map[string]string{"current": "x", "next": "y"}, 403)

	// Linking switched the softphone line on.
	ext, err := s.PBX.Extension(context.Background(), extID)
	if err != nil || !ext.WebRTC {
		t.Fatalf("webrtc not enabled: %+v %v", ext, err)
	}
	mustDo(t, desk, "GET", "/api/portal/webrtc", nil, 200)

	// Signing in inside the softphone: the session is swapped, but only on the softphone's request.
	mustDo(t, anna, "POST", "/api/desktop/session", map[string]any{}, 403)
	req, _ := http.NewRequest("POST", c.url+"/api/desktop/session", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kiestoon-Desktop", "1")
	resp, err := anna.hc.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("session swap: %v %v", resp, err)
	}
	resp.Body.Close()
	var maxAge int
	for _, ck := range resp.Cookies() {
		if ck.Name == CookieName {
			maxAge = ck.MaxAge
		}
	}
	if maxAge < 29*24*3600 {
		t.Fatalf("desktop cookie lifetime: %d", maxAge)
	}
	mustDo(t, anna, "GET", "/api/pbx/extensions", nil, 403)

	// Installer download.
	info := mustDo(t, newClient(), "GET", "/api/desktop/info", nil, 200)
	if info["installer"] != nil {
		t.Fatalf("installer without file: %v", info)
	}
	os.WriteFile(filepath.Join(s.Cfg.SoftphoneDir, "Kiestoon-Softphone_0.7.0_x64.msi"), []byte("MSI"), 0o644)
	info = mustDo(t, newClient(), "GET", "/api/desktop/info", nil, 200)
	inst, _ := info["installer"].(map[string]any)
	if inst == nil || inst["name"] != "Kiestoon-Softphone_0.7.0_x64.msi" || inst["sha256"] == "" {
		t.Fatalf("installer: %v", info)
	}
	rr := httptestRecorder(s.SoftphoneDownload(), "/download/kiestoon-softphone.msi")
	if rr.StatusCode != 200 || rr.Header.Get("Content-Type") != "application/x-msi" {
		t.Fatalf("download: %d", rr.StatusCode)
	}
	if b, _ := io.ReadAll(rr.Body); string(b) != "MSI" {
		t.Fatalf("download body: %q", b)
	}
	if httptestRecorder(s.SoftphoneDownload(), "/download/../kiestoon.db").StatusCode != 404 {
		t.Fatal("download serves other files")
	}
}

func TestNormalizePairCode(t *testing.T) {
	for in, want := range map[string]string{"abcd-efgh": "ABCD-EFGH", " ab cd ef gh ": "ABCD-EFGH", "ABCDEFG": "", "ABCD-EFG0": "", "ABCD-EFGHJ": ""} {
		if got := normalizePairCode(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func httptestRecorder(h http.Handler, path string) *http.Response {
	req, _ := http.NewRequest("GET", "http://x"+path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestDesktopClientsAdmin(t *testing.T) {
	c, s := adminClient(t)
	s.Cfg.SoftphoneDir = t.TempDir()
	e := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1&lang=en", map[string]any{"number": "202", "name": "Bram",
		"email": "bram@example.com", "enabled": true}, 200)
	link := e["welcome"].(map[string]any)["link"].(string)
	token := link[strings.Index(link, "#")+1:]
	newClient := func() *client {
		jar, _ := cookiejar.New(nil)
		return &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	}
	bram := newClient()
	mustDo(t, bram, "POST", "/api/auth/password-link/set", map[string]string{"token": token, "password": "a long enough password"}, 200)
	mustDo(t, bram, "POST", "/api/auth/login", map[string]string{"username": "bram@example.com", "password": "a long enough password"}, 200)

	if l := mustDoList(t, c, "/api/desktop/clients"); len(l) != 0 {
		t.Fatalf("clients before linking: %v", l)
	}
	p := mustDo(t, bram, "POST", "/api/desktop/pair", map[string]any{}, 200)
	desk := newClient()
	mustDo(t, desk, "POST", "/api/desktop/claim", map[string]string{"code": p["code"].(string)}, 200)

	l := mustDoList(t, c, "/api/desktop/clients")
	if len(l) != 1 || l[0]["username"] != "bram@example.com" || l[0]["extension"] != "202" {
		t.Fatalf("clients: %v", l)
	}
	// Only admins see the list, and the softphone itself cannot reach it.
	mustDo(t, bram, "GET", "/api/desktop/clients", nil, 403)
	mustDo(t, desk, "GET", "/api/desktop/clients", nil, 403)

	id := strconv.FormatInt(int64(l[0]["id"].(float64)), 10)
	mustDo(t, c, "DELETE", "/api/desktop/clients/"+id, map[string]any{}, 200)
	mustDo(t, c, "DELETE", "/api/desktop/clients/"+id, map[string]any{}, 404)
	mustDo(t, desk, "GET", "/api/me", nil, 401)
	mustDo(t, bram, "GET", "/api/me", nil, 200) // the browser session stays
}
