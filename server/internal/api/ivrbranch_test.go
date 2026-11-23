package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A menu that checks the time first, with its own menu per branch, and a key that rings a queue and
// an external number at the same time.
func TestIVRTimeStartMenusAndParallel(t *testing.T) {
	c, s := adminClient(t)
	ext := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true, "voicemail": true}, 200)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true, "maxWait": 120,
		"members":  []map[string]any{{"extensionId": ext["id"], "dynamic": false}},
		"overflow": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	tc := mustDo(t, c, "POST", "/api/pbx/timeconditions", map[string]any{"name": "Office hours",
		"schedule": []map[string]any{{"days": []int{1, 2, 3, 4, 5}, "from": "08:30", "to": "17:00"}},
		"open":     map[string]string{"type": "extension", "target": "201"}, "closed": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	node := func(id, typ string, extra map[string]any) map[string]any {
		m := map[string]any{"id": id, "type": typ, "x": 0, "y": 0}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	edge := func(from, port, to string) map[string]string {
		return map[string]string{"from": from, "port": port, "to": to}
	}
	dest := func(id, typ, target string) map[string]any {
		return node(id, "destination", map[string]any{"destination": map[string]string{"type": typ, "target": target}})
	}
	flow := map[string]any{
		"nodes": []any{
			node("e", "entry", nil),
			node("t", "time", map[string]any{"destination": map[string]string{"type": "timecondition", "target": fmt.Sprint(tc["id"])}}),
			node("open", "menu", map[string]any{"label": "Open"}),
			node("k1", "key", map[string]any{"digit": "1"}),
			node("closed", "menu", map[string]any{"label": "Closed"}),
			node("k1b", "key", map[string]any{"digit": "1"}), // the same digit in another menu
			dest("q", "queue", fmt.Sprint(q["id"])),
			dest("x", "external", "0612345678"),
			dest("vm", "voicemail", "201"),
		},
		"edges": []any{
			edge("e", "out", "t"), edge("t", "open", "open"), edge("t", "closed", "closed"),
			edge("open", "out", "k1"), edge("k1", "out", "q"), edge("k1", "out", "x"),
			edge("closed", "out", "k1b"), edge("k1b", "out", "vm"),
		},
	}
	ivr := mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Branches", "timeout": 5, "flow": flow}, 200)
	id := int(ivr["id"].(float64))

	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "extensions.conf"))
	dp := string(b)
	for _, want := range []string{
		fmt.Sprintf("[kiestoon-ivr-%d]\nexten => s,1,NoOp(IVR Branches)", id),
		fmt.Sprintf("Goto(kiestoon-ivr-%d-flow,", id),
		fmt.Sprintf("[kiestoon-ivr-%d-m", id),
		fmt.Sprintf("Gosub(kiestoon-tcstate-%v,s,1)", tc["id"]),
		fmt.Sprintf("Goto(kiestoon-ivr-%d-flow,p1,1)", id),
		fmt.Sprintf("Dial(Local/par@kiestoon-queue-%v/n&Local/0612345678@kiestoon-external/n,120,tTm(default))", q["id"]),
		"exten => par,1,NoOp(Queue 701 rings together with other destinations)",
		fmt.Sprintf("Queue(kiestoon-q-%v,tTr,,,120,,)", q["id"]),
		"Goto(kiestoon-vm,201,1)",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	// The entry with a start step does not wait for a key itself.
	main := dp[strings.Index(dp, fmt.Sprintf("[kiestoon-ivr-%d]", id)):]
	main = main[:strings.Index(main, "\n\n")]
	if strings.Contains(main, "WaitExten") {
		t.Errorf("entry with a start step waits for a key:\n%s", main)
	}

	// A menu step only has lines to keys and a timeout; at most one start step.
	bad := map[string]any{"nodes": flow["nodes"], "edges": []any{edge("e", "out", "t"), edge("e", "out", "open")}}
	mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Two starts", "flow": bad}, 400)
	bad = map[string]any{"nodes": flow["nodes"], "edges": []any{edge("open", "out", "q")}}
	mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Menu to queue", "flow": bad}, 400)
}
