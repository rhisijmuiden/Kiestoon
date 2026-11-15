package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/rhisijmuiden/kiestoon/server/internal/sso"
)

// adminClient runs setup and MFA enrollment and returns a signed-in superadmin.
func adminClient(t *testing.T) (*client, *Server) {
	c, s := newTestServer(t)
	os.WriteFile(SetupTokenPath(s.Cfg.DataDir), []byte("T\n"), 0o600)
	if code, _ := c.do("POST", "/api/setup", map[string]string{"token": "T", "username": "admin", "password": "correct horse battery"}); code != 201 {
		t.Fatal("setup failed")
	}
	_, enr := c.do("POST", "/api/account/mfa/enroll", map[string]any{})
	code, _ := totp.GenerateCode(enr["secret"].(string), time.Now())
	if st, _ := c.do("POST", "/api/account/mfa/confirm", map[string]string{"code": code}); st != 200 {
		t.Fatal("mfa confirm failed")
	}
	return c, s
}

func mustDo(t *testing.T, c *client, method, path string, body any, want int) map[string]any {
	t.Helper()
	code, out := c.do(method, path, body)
	if code != want {
		t.Fatalf("%s %s: %d %v", method, path, code, out)
	}
	return out
}

func TestBusinessObjectsAndPortal(t *testing.T) {
	c, s := adminClient(t)
	e1 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "email": "anna@example.com", "enabled": true, "voicemail": true,
		"webrtc": true, "pickupGroup": "1"}, 200)
	e2 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "202", "name": "Bas", "enabled": true}, 200)
	id1 := int64(e1["id"].(float64))
	id2 := int64(e2["id"].(float64))
	if e1["outboundClass"] != "national" || e1["recordMode"] != "ondemand" {
		t.Fatalf("defaults: %v", e1)
	}

	rg := mustDo(t, c, "POST", "/api/pbx/ringgroups", map[string]any{"number": "601", "name": "Sales", "strategy": "rotating",
		"members": []string{"201", "202", "0612345678"}, "enabled": true, "failover": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	mustDo(t, c, "POST", "/api/pbx/ringgroups", map[string]any{"number": "201", "name": "Bad", "members": []string{"201"}}, 400)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true, "announcePosition": true,
		"announceFrequency": 30, "members": []map[string]any{{"extensionId": id1, "dynamic": true}, {"extensionId": id2}},
		"overflow": map[string]string{"type": "ringgroup", "target": fmt.Sprint(rg["id"])}}, 200)
	qid := int64(q["id"].(float64))
	tc := mustDo(t, c, "POST", "/api/pbx/timeconditions", map[string]any{"name": "Office hours", "useHolidays": true,
		"schedule": []map[string]any{{"days": []int{1, 2, 3, 4, 5}, "from": "08:30", "to": "17:00"}},
		"open":     map[string]string{"type": "queue", "target": fmt.Sprint(qid)}, "closed": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	ivr := mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Main menu", "directDial": true,
		"options":            []map[string]any{{"digit": "1", "destination": map[string]string{"type": "timecondition", "target": fmt.Sprint(tc["id"])}}},
		"timeoutDestination": map[string]string{"type": "extension", "target": "202"}}, 200)
	mustDo(t, c, "POST", "/api/pbx/conferences", map[string]any{"number": "801", "name": "Team", "pin": "1234", "enabled": true}, 200)
	mustDo(t, c, "POST", "/api/pbx/inbound", map[string]any{"name": "Main", "did": "0201234567", "destinationType": "ivr",
		"destination": fmt.Sprint(ivr["id"]), "enabled": true}, 200)

	// Generated configuration contains the new objects.
	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	read := func(n string) string { b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), n)); return string(b) }
	if out := os.Getenv("KT_GEN_OUT"); out != "" {
		for _, n := range []string{"extensions.conf", "pjsip.conf", "queues.conf", "confbridge.conf", "res_parking.conf"} {
			_ = os.WriteFile(filepath.Join(out, n), []byte(read(n)), 0o644)
		}
	}
	dp := read("extensions.conf")
	for _, want := range []string{"[kiestoon-rg-1]", "GLOBAL(KT_RG_1)", "(join),Queue(kiestoon-q-1,tT,,,,,,,${KT_QPOS})", "[kiestoon-ivr-1]",
		"include => kiestoon-direct", "GotoIfTime(08:30-17:00,mon-fri,*,*,Europe/Amsterdam?open)", "GotoIfTime(*,*,25,dec,Europe/Amsterdam?holiday)",
		"ConfBridge(kiestoon-1,kiestoon_bridge_1", "Goto(kiestoon-in-1,s,1)", "Goto(kiestoon-ivr-1,s,1)", "[kiestoon-allowed]", "Local/0612345678@kiestoon-followme/n"} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	if qc := read("queues.conf"); !strings.Contains(qc, "member => Local/201@kiestoon-ring/n,0,Anna,hint:201@kiestoon-extensions") {
		t.Errorf("queues.conf:\n%s", qc)
	}
	if pj := read("pjsip.conf"); !strings.Contains(pj, "[201-web]") || !strings.Contains(pj, "webrtc=yes") || !strings.Contains(pj, "named_pickup_group=1") {
		t.Error("pjsip.conf misses WebRTC endpoint or pickup group")
	}

	// Deleting the queue sends the time condition to "hang up" instead of a missing queue.
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/pbx/queues/%d", qid), map[string]any{}, 200)
	tcs := mustDoList(t, c, "/api/pbx/timeconditions")
	if open := tcs[0]["open"].(map[string]any); open["type"] != "hangup" {
		t.Fatalf("dangling destination: %v", open)
	}

	// A user linked to 201 uses the portal.
	mustDo(t, c, "POST", "/api/users", map[string]any{"username": "anna", "displayName": "Anna", "role": "user", "extensionId": id1,
		"enabled": true, "password": "anna her long password"}, 200)
	jar, _ := cookiejar.New(nil)
	u := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	mustDo(t, u, "POST", "/api/auth/login", map[string]string{"username": "anna", "password": "anna her long password"}, 200)
	me := mustDo(t, u, "GET", "/api/portal/me", nil, 200)
	if me["number"] != "201" {
		t.Fatalf("portal me: %v", me)
	}
	mustDo(t, u, "PUT", "/api/portal/features", map[string]any{"dnd": true, "forwardNoAnswer": "0612345678"}, 200)
	mustDo(t, u, "GET", "/api/pbx/extensions", nil, 403)
	mustDo(t, u, "GET", "/api/live", nil, 403)
	mustDo(t, u, "GET", "/api/portal/voicemail", nil, 200)
	rtc := mustDo(t, u, "GET", "/api/portal/webrtc", nil, 200)
	if rtc["username"] != "201-web" || rtc["password"] == "" {
		t.Fatalf("webrtc: %v", rtc)
	}

	// A feature code from the phone switches DND off again.
	s.featureCode("dnd", "201", "off", "", "")
	if me := mustDo(t, u, "GET", "/api/portal/me", nil, 200); me["features"].(map[string]any)["dnd"] != false {
		t.Fatal("feature code not applied")
	}

	// Renumbering to 4 digits follows ring group members and destinations.
	mustDo(t, c, "POST", "/api/pbx/numberplan/apply", map[string]any{"digits": 4}, 200)
	g := mustDoList(t, c, "/api/pbx/ringgroups")[0]
	if g["number"] != "6001" || g["members"].([]any)[0] != "2001" || g["failover"].(map[string]any)["target"] != "2001" {
		t.Fatalf("renumber: %v", g)
	}
}

func mustDoList(t *testing.T, c *client, path string) []map[string]any {
	t.Helper()
	resp, err := c.hc.Get(c.url + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []map[string]any
	if err := jsonDecode(resp, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRoleMapping(t *testing.T) {
	p := &sso.Provider{RoleMap: []sso.RoleMapping{{Group: "pbx-admins", Role: "admin"}, {Group: "reception", Role: "reception"}}}
	if r, ok := sso.RoleFor(p, []string{"reception", "PBX-admins"}); !ok || r != "admin" {
		t.Fatalf("got %s %v", r, ok)
	}
	if _, ok := sso.RoleFor(p, []string{"other"}); ok {
		t.Fatal("no default role: must be refused")
	}
	p.DefaultRole = "user"
	if r, _ := sso.RoleFor(p, nil); r != "user" {
		t.Fatal("default role not used")
	}
}
