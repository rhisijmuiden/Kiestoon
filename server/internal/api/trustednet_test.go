package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTrustedNetworksValidate(t *testing.T) {
	for _, bad := range []string{"0.0.0.0/0", "::/0", "10.0.0.0/7", "2000::/3", "nonsense", "10.0.0.300"} {
		tn := TrustedNetworks{Enabled: true, Networks: []TrustedNetwork{{CIDR: bad}}}
		if tn.Validate() == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
	tn := TrustedNetworks{Enabled: true, Roles: []string{"user", "user", "admin"}, Networks: []TrustedNetwork{
		{CIDR: " 192.168.1.77/24 ", Description: " Office "}, {CIDR: "192.168.1.0/24"}, {CIDR: "fd00:1::/64"}, {CIDR: "203.0.113.9"},
	}}
	if err := tn.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(tn.Networks) != 3 || tn.Networks[0].CIDR != "192.168.1.0/24" || tn.Networks[0].Description != "Office" || tn.Networks[2].CIDR != "203.0.113.9/32" || len(tn.Roles) != 2 {
		t.Fatalf("cleaned: %+v", tn)
	}
	if (&TrustedNetworks{Enabled: true}).Validate() == nil {
		t.Fatal("enabled without networks accepted")
	}
	if (&TrustedNetworks{Roles: []string{"owner"}}).Validate() == nil {
		t.Fatal("unknown role accepted")
	}
	for cidr, public := range map[string]bool{"10.0.0.0/8": false, "192.168.0.0/16": false, "100.64.0.0/10": false, "fd00::/8": false,
		"203.0.113.0/24": true, "172.16.0.0/11": true, "2001:db8::/32": true} {
		if got := isPublicRange(netip.MustParsePrefix(cidr)); got != public {
			t.Errorf("isPublicRange(%s) = %v", cidr, got)
		}
	}
}

func TestTrustedNetworkSkipsMFA(t *testing.T) {
	defer setActiveTrusted(TrustedNetworks{})
	c, s := newTestServer(t)
	os.WriteFile(SetupTokenPath(s.Cfg.DataDir), []byte("ABCD-1234\n"), 0o600)
	// The test server sees loopback, which is a trusted proxy, so X-Forwarded-For picks the address.
	c.hdr = map[string]string{"X-Forwarded-For": "192.168.10.5"}
	c.do("POST", "/api/setup", map[string]string{"token": "ABCD-1234", "username": "admin", "password": "correct horse battery", "language": "en"})
	_, enr := c.do("POST", "/api/account/mfa/enroll", map[string]any{})
	code, _ := totp.GenerateCode(enr["secret"].(string), time.Now())
	if st, _ := c.do("POST", "/api/account/mfa/confirm", map[string]string{"code": code}); st != 200 {
		t.Fatal("enroll")
	}
	// Off by default.
	if st, body := c.do("GET", "/api/firewall/trusted", nil); st != 200 || body["settings"].(map[string]any)["enabled"] != false || body["clientIp"] != "192.168.10.5" {
		t.Fatalf("get: %d %v", st, body)
	}
	if st, body := c.do("PUT", "/api/firewall/trusted", map[string]any{"enabled": true, "roles": []string{"user"}, "networks": []map[string]string{{"cidr": "0.0.0.0/0"}}}); st != 400 || body["error"] != "network_too_broad" {
		t.Fatalf("0.0.0.0/0: %d %v", st, body)
	}
	save := func(roles ...string) {
		t.Helper()
		if st, body := c.do("PUT", "/api/firewall/trusted", map[string]any{"enabled": true, "roles": roles,
			"networks": []map[string]string{{"cidr": "192.168.10.0/24", "description": "Office"}}}); st != 200 || body["clientNetwork"] != "192.168.10.0/24" {
			t.Fatalf("save: %d %v", st, body)
		}
	}
	login := func(ip string) bool {
		t.Helper()
		c.do("POST", "/api/auth/logout", map[string]any{})
		c.hdr["X-Forwarded-For"] = ip
		st, body := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "correct horse battery"})
		if st != 200 {
			t.Fatalf("login: %d %v", st, body)
		}
		return body["mfaRequired"].(bool)
	}

	save("superadmin")
	if login("192.168.10.5") {
		t.Fatal("MFA asked inside the trusted network")
	}
	if st, me := c.do("GET", "/api/me", nil); st != 200 || me["signInMethod"] != "trusted-network" {
		t.Fatalf("me: %d %v", st, me)
	}
	// Taking the super admin role off ends the skip for running sessions too, and the next sign-in
	// asks for the code again.
	save("user", "reception", "admin")
	if st, body := c.do("GET", "/api/me", nil); st != 401 || body["error"] != "mfa_required" {
		t.Fatalf("after role change: %d %v", st, body)
	}
	if !login("192.168.10.5") {
		t.Fatal("super admin skipped MFA without the role switched on")
	}

	// Outside the network the same session has to pass MFA first, also after coming back.
	setActiveTrusted(TrustedNetworks{Enabled: true, Roles: []string{"superadmin"}, Networks: []TrustedNetwork{{CIDR: "192.168.10.0/24", Description: "Office"}}})
	if login("192.168.10.5") {
		t.Fatal("MFA asked inside the trusted network")
	}
	c.hdr["X-Forwarded-For"] = "8.8.8.8"
	if st, body := c.do("GET", "/api/me", nil); st != 401 || body["error"] != "mfa_required" {
		t.Fatalf("outside: %d %v", st, body)
	}
	c.hdr["X-Forwarded-For"] = "192.168.10.5"
	if st, _ := c.do("GET", "/api/me", nil); st != 401 {
		t.Fatal("downgraded session regained access")
	}
	// From elsewhere: MFA as usual.
	if !login("8.8.8.8") {
		t.Fatal("MFA skipped outside the trusted network")
	}
	// A client that is not behind a trusted proxy cannot pretend to be inside.
	if _, ok := trustedNetworkFor(clientIP(reqFrom("8.8.8.8:1000", "192.168.10.5")), "superadmin"); ok {
		t.Fatal("X-Forwarded-For from an untrusted address was believed")
	}
	// The skip is in the audit log.
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'login.mfa_skipped' AND detail LIKE '%192.168.10.0/24 (Office)%'`).Scan(&n)
	if n != 2 {
		t.Fatalf("audit entries: %d", n)
	}
}

func reqFrom(remote, xff string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	r.Header.Set("X-Forwarded-For", xff)
	return r
}
