package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// External numbers as queue members: saved cleaned, written to queues.conf when active, bad numbers refused.
func TestQueueExternalMembers(t *testing.T) {
	c, s := adminClient(t)
	q := map[string]any{
		"number": "701", "name": "Support", "strategy": "ringall", "enabled": true, "members": []any{},
		"overflow": map[string]any{"type": "hangup", "target": ""},
		"options": map[string]any{"external": []any{
			map[string]any{"name": "Jan mobiel", "number": "06 1234 5678", "penalty": 1, "active": true},
			map[string]any{"name": "Kantoor Gent", "number": "+3291234567", "penalty": 0, "active": false},
		}},
	}
	got := mustDo(t, c, "POST", "/api/pbx/queues", q, 200)
	ext := got["options"].(map[string]any)["external"].([]any)
	if len(ext) != 2 || ext[0].(map[string]any)["number"] != "0612345678" {
		t.Fatalf("external = %v", ext)
	}
	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "queues.conf"))
	if !strings.Contains(string(b), "member => Local/0612345678@kiestoon-external/n,1,Jan mobiel") || strings.Contains(string(b), "3291234567") {
		t.Errorf("queues.conf:\n%s", b)
	}
	q["number"] = "702"
	q["options"] = map[string]any{"external": []any{map[string]any{"name": "x", "number": "abc", "active": true}}}
	mustDo(t, c, "POST", "/api/pbx/queues", q, 400)
}

// An on-call team as an IVR step (with who is on duty in the live view), and "back to" a chosen menu.
func TestIVROnCallAndBackTo(t *testing.T) {
	c, s := adminClient(t)
	u := mustDo(t, c, "POST", "/api/users", map[string]any{"username": "jan", "displayName": "Jan", "role": "user",
		"email": "jan@example.com", "enabled": true, "password": "Jan has a long password"}, 200)
	now := time.Now().In(s.loc(t.Context()))
	team := mustDo(t, c, "POST", "/api/oncall/teams", map[string]any{"number": "690", "name": "Storingsdienst", "enabled": true, "mode": "weekly",
		"handoverDay": (int(now.Weekday()) + 6) % 7, "handoverTime": "00:00", "startDate": now.Format("2006-01-02"), "members": []any{u["id"]},
		"backup": "none", "swapApproval": "none", "steps": []map[string]any{{"kind": "primary", "seconds": 25}}, "final": map[string]any{"type": "hangup"}}, 200)

	node := func(id, typ string, extra map[string]any) map[string]any {
		m := map[string]any{"id": id, "type": typ, "x": 0, "y": 0}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	edge := func(from, to string) map[string]string {
		return map[string]string{"from": from, "port": "out", "to": to}
	}
	flow := map[string]any{
		"nodes": []any{
			node("e", "entry", nil),
			node("k1", "key", map[string]any{"digit": "1"}),
			node("k2", "key", map[string]any{"digit": "2"}),
			node("closed", "menu", map[string]any{"label": "Closed"}),
			node("k9", "key", map[string]any{"digit": "9"}),
			node("oc", "destination", map[string]any{"destination": map[string]string{"type": "oncall", "target": fmt.Sprint(team["id"])}}),
			node("a", "audio", nil),
			node("back", "repeat", map[string]any{"menu": "closed"}),
			node("a2", "audio", nil),
			node("main", "repeat", map[string]any{"menu": "e"}),
		},
		"edges": []any{edge("e", "k1"), edge("e", "k2"), edge("k1", "closed"), edge("closed", "k9"), edge("k9", "oc"),
			edge("k2", "a"), edge("a", "back"), edge("a2", "main")},
	}
	ivr := mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Na sluitingstijd", "timeout": 5, "flow": flow}, 200)
	id := int(ivr["id"].(float64))
	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "extensions.conf"))
	dp := string(b)
	for _, want := range []string{
		fmt.Sprintf("Goto(kiestoon-oc-%v,s,1)", team["id"]),
		fmt.Sprintf("Goto(kiestoon-ivr-%d-m", id),
		fmt.Sprintf("Goto(kiestoon-ivr-%d,s,start)", id),
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}

	live := mustDo(t, c, "GET", "/api/pbx/ivrs/live", nil, 200)
	d, _ := live["dests"].(map[string]any)["oncall:"+fmt.Sprint(team["id"])].(map[string]any)
	if d == nil || d["onDuty"] != "Jan" {
		t.Errorf("live on-call = %v", d)
	}

	// "Back to" must point to a menu.
	flow["nodes"] = append(flow["nodes"].([]any), node("bad", "repeat", map[string]any{"menu": "a"}))
	mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Bad", "timeout": 5, "flow": flow}, 400)
}
