package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/internet"
)

// doHost sends a request as if it arrived on the given host name.
func (c *client) doHost(host, method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	req.Host = host
	// The jar matches cookies on the host name, which is the point of this test: send them by hand.
	for _, ck := range c.hc.Jar.Cookies(req.URL) {
		req.AddCookie(ck)
	}
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

func TestSeparateHostnames(t *testing.T) {
	c, _ := adminClient(t)
	t.Cleanup(func() { setTrustedProxies(nil) })

	// The portal address needs a PBX address.
	mustDo(t, c, "PUT", "/api/system/proxy", map[string]any{"enabled": true, "trustedProxies": []string{"10.0.0.5"},
		"portalUrl": "https://voipcentrale.example.com"}, 400)
	mustDo(t, c, "PUT", "/api/system/proxy", map[string]any{"enabled": true, "trustedProxies": []string{"10.0.0.5"},
		"publicUrl": "https://sip.example.com", "portalUrl": "https://voipcentrale.example.com/"}, 200)

	h := mustDo(t, c, "GET", "/api/system/hostnames", nil, 200)
	hn := h["hostnames"].(map[string]any)
	if hn["pbxHost"] != "sip.example.com" || hn["portalHost"] != "voipcentrale.example.com" || hn["separate"] != true {
		t.Fatalf("hostnames: %v", h)
	}
	if cb := h["ssoCallbacks"].([]any); len(cb) != 2 || cb[1] != "https://voipcentrale.example.com/api/auth/sso/callback" {
		t.Fatalf("callbacks: %v", cb)
	}

	// Employees get the portal address, phones the PBX host name.
	e := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1&lang=en", map[string]any{"number": "201", "name": "Anna",
		"email": "anna@example.com", "enabled": true}, 200)
	link := e["welcome"].(map[string]any)["link"].(string)
	if !strings.HasPrefix(link, "https://voipcentrale.example.com/set-password#") {
		t.Fatalf("welcome link: %s", link)
	}
	jar, _ := cookiejar.New(nil)
	anna := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	mustDo(t, anna, "POST", "/api/auth/password-link/set", map[string]string{"token": link[strings.Index(link, "#")+1:], "password": "a long enough password"}, 200)
	mustDo(t, anna, "POST", "/api/auth/login", map[string]string{"username": "anna@example.com", "password": "a long enough password"}, 200)
	if sip := mustDo(t, anna, "GET", "/api/portal/sip", nil, 200); sip["server"] != "sip.example.com" {
		t.Fatalf("sip server: %v", sip)
	}
	if p := mustDo(t, anna, "POST", "/api/desktop/pair", map[string]any{}, 200); p["server"] != "https://voipcentrale.example.com" {
		t.Fatalf("pair server: %v", p)
	}

	// Blocking administration on the portal host name.
	if code, out := c.doHost("voipcentrale.example.com", "PUT", "/api/system/hostnames", map[string]any{"blockAdmin": true}); code != 409 || out["error"] != "on_portal_host" {
		t.Fatalf("block from portal host: %d %v", code, out)
	}
	mustDo(t, c, "PUT", "/api/system/hostnames", map[string]any{"blockAdmin": true}, 200)
	code, me := c.doHost("voipcentrale.example.com", "GET", "/api/me", nil)
	if code != 200 || me["user"].(map[string]any)["role"] != "reception" || me["adminUrl"] != "https://sip.example.com" {
		t.Fatalf("me on portal host: %d %v", code, me)
	}
	if code, _ := c.doHost("voipcentrale.example.com", "GET", "/api/pbx/extensions", nil); code != 403 {
		t.Fatalf("admin api on portal host: %d", code)
	}
	if code, _ := c.doHost("voipcentrale.example.com:443", "GET", "/api/users", nil); code != 403 {
		t.Fatalf("admin api on portal host with port: %d", code)
	}
	if code, _ := c.doHost("sip.example.com", "GET", "/api/pbx/extensions", nil); code != 200 {
		t.Fatalf("admin api on pbx host: %d", code)
	}
	if code, me := c.doHost("sip.example.com", "GET", "/api/me", nil); code != 200 || me["user"].(map[string]any)["role"] != "superadmin" {
		t.Fatalf("me on pbx host: %v", me)
	}
	// Employees keep their portal there.
	if code, _ := anna.doHost("voipcentrale.example.com", "GET", "/api/portal/me", nil); code != 200 {
		t.Fatalf("portal on portal host: %d", code)
	}

	// X-Forwarded-Host only counts from a trusted proxy (the test client is loopback, so trusted).
	if code, _ := c.doHost("10.0.0.20:8080", "GET", "/api/pbx/extensions", nil); code != 200 {
		t.Fatalf("lan address: %d", code)
	}

	// Removing the portal address in the proxy settings lifts the block.
	mustDo(t, c, "PUT", "/api/system/proxy", map[string]any{"enabled": true, "trustedProxies": []string{"10.0.0.5"},
		"publicUrl": "https://sip.example.com"}, 200)
	if code, _ := c.doHost("voipcentrale.example.com", "GET", "/api/pbx/extensions", nil); code != 200 {
		t.Fatalf("after removing portal: %d", code)
	}
}

func TestForwardedHostFromTrustedProxyOnly(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://10.0.0.20:8080/", nil)
	r.Header.Set("X-Forwarded-Host", "voipcentrale.example.com")
	r.RemoteAddr = "203.0.113.9:4000"
	forwardedHost(r)
	if r.Host != "10.0.0.20:8080" {
		t.Fatalf("untrusted X-Forwarded-Host used: %s", r.Host)
	}
	r.RemoteAddr = "127.0.0.1:4000"
	forwardedHost(r)
	if r.Host != "voipcentrale.example.com" {
		t.Fatalf("trusted X-Forwarded-Host ignored: %s", r.Host)
	}
}

func TestCaddyfileTwoHostnames(t *testing.T) {
	s := internet.Settings{Enabled: true, Domain: "sip.example.com", PortalDomain: "VoIPcentrale.example.com."}
	cf, err := internet.Caddyfile(s, "127.0.0.1:8080")
	if err != nil || !strings.Contains(cf, "sip.example.com, voipcentrale.example.com {") {
		t.Fatalf("caddyfile: %v\n%s", err, cf)
	}
	s.PortalDomain = "sip.example.com"
	if cf, _ := internet.Caddyfile(s, "127.0.0.1:8080"); strings.Contains(cf, ", ") {
		t.Fatalf("same name twice:\n%s", cf)
	}
	s.PortalDomain = "not a host"
	if err := s.Validate(); err == nil || err.Error() != "portalDomain" {
		t.Fatalf("bad portal domain: %v", err)
	}
}
