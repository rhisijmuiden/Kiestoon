package api

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOnCallFlow(t *testing.T) {
	c, s := adminClient(t)
	user := func(name string) (int64, *client) {
		out := mustDo(t, c, "POST", "/api/users", map[string]any{"username": strings.ToLower(name), "displayName": name, "role": "user",
			"email": strings.ToLower(name) + "@example.com", "enabled": true, "password": name + " has a long password"}, 200)
		jar, _ := cookiejar.New(nil)
		u := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
		mustDo(t, u, "POST", "/api/auth/login", map[string]string{"username": strings.ToLower(name), "password": name + " has a long password"}, 200)
		return int64(out["id"].(float64)), u
	}
	jan, cj := user("Jan")
	pieter, cp := user("Pieter")
	sanne, cs := user("Sanne")
	mustDo(t, c, "PUT", fmt.Sprintf("/api/oncall/engineers/%d/mobile", jan), map[string]any{"mobile": "06 1234 5678"}, 200)
	mustDo(t, cp, "PUT", "/api/portal/oncall/mobile", map[string]any{"mobile": "0623456789"}, 200)
	mustDo(t, cp, "PUT", "/api/portal/oncall/mobile", map[string]any{"mobile": "not a number"}, 400)

	loc := s.loc(t.Context())
	now := time.Now().In(loc)
	wd := (int(now.Weekday()) + 6) % 7
	team := map[string]any{"number": "690", "name": "Storingsdienst ICT", "enabled": true, "mode": "weekly", "handoverDay": wd, "handoverTime": "00:00",
		"startDate": now.Format("2006-01-02"), "members": []int64{jan, pieter}, "backup": "next", "leadUser": sanne, "swapApproval": "lead",
		"confirm": true, "steps": []map[string]any{{"kind": "primary", "seconds": 25}, {"kind": "backup", "seconds": 25}, {"kind": "lead", "seconds": 30}},
		"final": map[string]any{"type": "hangup"}}
	created := mustDo(t, c, "POST", "/api/oncall/teams", team, 200)
	id := int64(created["id"].(float64))
	token := created["icalToken"].(string)
	if len(token) < 30 {
		t.Fatalf("token: %v", created)
	}
	// Ring groups cannot take the number of an on-call team.
	mustDo(t, c, "POST", "/api/oncall/teams", team, 400)

	dp := func() string {
		if err := s.Ast.Apply(t.Context()); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "extensions.conf"))
		return string(b)
	}
	if d := dp(); !strings.Contains(d, "[kiestoon-oc-1]") || !strings.Contains(d, "Local/0612345678@kiestoon-followme/n,25,tTU(kiestoon-ocok^1^1)") {
		t.Fatalf("dialplan:\n%s", d)
	}
	dests := mustDo(t, c, "GET", "/api/pbx/destinations", nil, 200)
	if !strings.Contains(fmt.Sprint(dests["options"]), "690 Storingsdienst ICT") {
		t.Fatalf("destinations: %v", dests)
	}

	// Jan asks Pieter to take over two hours tonight; Pieter accepts, Sanne approves.
	a, b := now.Add(time.Hour).Truncate(time.Minute), now.Add(3*time.Hour).Truncate(time.Minute)
	req := map[string]any{"teamId": id, "kind": "handover", "line": 1, "start": a, "end": b, "to": pieter, "note": "Verjaardag"}
	mustDo(t, cp, "POST", "/api/portal/oncall/requests", req, 400) // Pieter is not on duty then
	sh := mustDo(t, cj, "POST", "/api/portal/oncall/requests", req, 200)
	shID := int64(sh["id"].(float64))
	mustDo(t, cs, "POST", fmt.Sprintf("/api/oncall/shifts/%d/decide", shID), map[string]any{"approve": true}, 409) // not accepted yet
	mustDo(t, cj, "POST", fmt.Sprintf("/api/oncall/shifts/%d/respond", shID), map[string]any{"accept": true}, 403)
	mustDo(t, cp, "POST", fmt.Sprintf("/api/oncall/shifts/%d/respond", shID), map[string]any{"accept": true}, 200)
	mustDo(t, cj, "POST", fmt.Sprintf("/api/oncall/shifts/%d/decide", shID), map[string]any{"approve": true}, 403)
	got := mustDo(t, cs, "POST", fmt.Sprintf("/api/oncall/shifts/%d/decide", shID), map[string]any{"approve": true}, 200)
	if got["status"] != "approved" {
		t.Fatalf("decide: %v", got)
	}
	tl := mustDo(t, c, "GET", fmt.Sprintf("/api/oncall/teams/%d/timeline", id), nil, 200)
	view := tl["view"].(map[string]any)
	if int64(view["primary"].(float64)) != jan {
		t.Fatalf("now: %v", view)
	}
	found := false
	for _, sg := range view["segments"].([]any) {
		m := sg.(map[string]any)
		if m["kind"] == "handover" && int64(m["userId"].(float64)) == pieter {
			found = true
		}
	}
	if !found {
		t.Fatalf("handover not in timeline: %v", view["segments"])
	}

	// Sanne sees the team in the portal; Pieter takes over right now.
	p := mustDo(t, cs, "GET", "/api/portal/oncall", nil, 200)
	if len(p["teams"].([]any)) != 1 {
		t.Fatalf("portal: %v", p)
	}
	mustDo(t, cp, "POST", "/api/portal/oncall/takeover", map[string]any{"teamId": id, "line": 1}, 200)
	if d := dp(); !strings.Contains(d, "Dial(Local/0623456789@kiestoon-followme/n,25,") {
		t.Fatalf("after take-over:\n%s", d)
	}
	mustDo(t, cp, "POST", "/api/portal/oncall/takeover", map[string]any{"teamId": id, "line": 1}, 400)

	// The calendar link works without signing in; a wrong token does not.
	resp, err := http.Get(c.url + "/api/oncall/ical/" + token + ".ics")
	if err != nil || resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/calendar") {
		t.Fatalf("ical: %v %v", resp, err)
	}
	resp.Body.Close()
	if resp, _ := http.Get(c.url + "/api/oncall/ical/wrongwrongwrongwrongwrong"); resp.StatusCode != 404 {
		t.Fatalf("wrong token: %d", resp.StatusCode)
	}

	// Deleting the team removes its dialplan.
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/oncall/teams/%d", id), map[string]any{}, 200)
	if d := dp(); strings.Contains(d, "kiestoon-oc-1") {
		t.Fatal("team still in the dialplan")
	}
}
