package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testWAV is half a second of silence in the prompt format (8 kHz, mono, 16 bit).
func testWAV() string {
	samples := 4000
	b := make([]byte, 44+samples*2)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+samples*2))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 8000)
	binary.LittleEndian.PutUint32(b[28:], 16000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(samples*2))
	return base64.StdEncoding.EncodeToString(b)
}

func TestIVRFlow(t *testing.T) {
	c, s := adminClient(t)
	ext := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true, "voicemail": true}, 200)
	tc := mustDo(t, c, "POST", "/api/pbx/timeconditions", map[string]any{"name": "Office hours",
		"schedule": []map[string]any{{"days": []int{1, 2, 3, 4, 5}, "from": "08:30", "to": "17:00"}},
		"open":     map[string]string{"type": "extension", "target": "201"}, "closed": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	var prompts []int64
	for _, n := range []string{"Welcome", "Press 1", "Hold on"} {
		p := mustDo(t, c, "POST", "/api/pbx/prompts", map[string]any{"name": n, "audio": testWAV()}, 200)
		prompts = append(prompts, int64(p["id"].(float64)))
	}

	// An old-style menu (options only) gets a flow.
	old := mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Old",
		"options":            []map[string]any{{"digit": "2", "destination": map[string]string{"type": "extension", "target": "201"}}},
		"timeoutDestination": map[string]string{"type": "voicemail", "target": "201"}}, 200)
	flow := old["flow"].(map[string]any)
	if n := len(flow["nodes"].([]any)); n != 5 {
		t.Fatalf("legacy flow nodes: %d %v", n, flow)
	}

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
	body := map[string]any{"name": "Main", "timeout": 5, "retries": 1, "flow": map[string]any{
		"nodes": []any{
			node("e", "entry", map[string]any{"promptId": prompts[0]}),
			node("k1", "key", map[string]any{"digit": "1", "label": "Sales", "promptId": prompts[1]}),
			node("k2", "key", map[string]any{"digit": "2", "label": "Hours"}),
			node("k9", "key", map[string]any{"digit": "9"}),
			node("to", "timeout", nil),
			node("a", "audio", map[string]any{"promptId": prompts[2]}),
			node("d", "destination", map[string]any{"destination": map[string]string{"type": "extension", "target": "201"}}),
			node("t", "time", map[string]any{"destination": map[string]string{"type": "timecondition", "target": fmt.Sprint(tc["id"])}}),
			node("r", "repeat", nil),
			node("loose", "key", map[string]any{"digit": "5"}),
		},
		"edges": []any{
			edge("e", "out", "k1"), edge("e", "out", "k2"), edge("e", "out", "k9"), edge("e", "out", "to"),
			edge("k1", "out", "a"), edge("a", "out", "d"), edge("k2", "out", "t"), edge("t", "open", "d"), edge("t", "closed", "r"),
			edge("k9", "out", "r"),
		},
	}}
	ivr := mustDo(t, c, "POST", "/api/pbx/ivrs", body, 200)
	if ivr["promptId"].(float64) != float64(prompts[0]) {
		t.Fatalf("derived prompt: %v", ivr["promptId"])
	}
	opts := ivr["options"].([]any)
	if len(opts) != 3 || opts[0].(map[string]any)["digit"] != "1" {
		t.Fatalf("derived options: %v", opts)
	}
	if d := opts[0].(map[string]any)["destination"].(map[string]any); d["type"] != "extension" {
		t.Fatalf("first destination: %v", d)
	}

	// The prompts are in use.
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/pbx/prompts/%d", prompts[2]), map[string]any{}, 409)

	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "extensions.conf"))
	dp := string(b)
	id := int(ivr["id"].(float64))
	for _, want := range []string{
		fmt.Sprintf("[kiestoon-ivr-%d]", id),
		fmt.Sprintf("Background(%s)", filepath.Join(s.promptDir(), fmt.Sprint(prompts[1]))),
		fmt.Sprintf("exten => 1,1,CELGenUserEvent(kt_ivrkey,id=%d&key=${EXTEN})\n same => n,Goto(kiestoon-ivr-%d-flow,1,1)", id, id),
		fmt.Sprintf("[kiestoon-ivr-%d-flow]", id),
		fmt.Sprintf("Playback(%s)", filepath.Join(s.promptDir(), fmt.Sprint(prompts[2]))),
		fmt.Sprintf("Gosub(kiestoon-tcstate-%v,s,1)", tc["id"]),
		fmt.Sprintf("Goto(kiestoon-ivr-%d,s,start)", id),
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	if strings.Contains(dp, "exten => 5,1") {
		t.Error("a key without a line from the entry is not part of the menu")
	}

	// Invalid flows.
	dup := map[string]any{"name": "Dup", "flow": map[string]any{"nodes": []any{node("e", "entry", nil), node("a", "key", map[string]any{"digit": "1"}), node("b", "key", map[string]any{"digit": "1"})}}}
	mustDo(t, c, "POST", "/api/pbx/ivrs", dup, 400)
	bad := map[string]any{"name": "Bad", "flow": map[string]any{"nodes": []any{node("e", "entry", nil), node("d", "destination", map[string]any{"destination": map[string]string{"type": "extension", "target": "201"}})},
		"edges": []any{edge("e", "out", "d")}}}
	mustDo(t, c, "POST", "/api/pbx/ivrs", bad, 400)

	// Deleting the extension clears it inside the flow too.
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/pbx/extensions/%v", ext["id"]), map[string]any{}, 200)
	getIVR := func() map[string]any {
		for _, v := range mustDoList(t, c, "/api/pbx/ivrs") {
			if int(v["id"].(float64)) == id {
				return v
			}
		}
		t.Fatal("ivr gone")
		return nil
	}
	got := getIVR()
	for _, n := range got["flow"].(map[string]any)["nodes"].([]any) {
		m := n.(map[string]any)
		if m["id"] == "d" && m["destination"].(map[string]any)["type"] != "hangup" {
			t.Fatalf("flow destination not cleared: %v", m)
		}
	}
	// Deleting the time condition turns the time step into a plain destination.
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/pbx/timeconditions/%v", tc["id"]), map[string]any{}, 200)
	got = getIVR()
	for _, n := range got["flow"].(map[string]any)["nodes"].([]any) {
		if m := n.(map[string]any); m["id"] == "t" && m["type"] != "destination" {
			t.Fatalf("time node after delete: %v", m)
		}
	}
}

func TestIVRLive(t *testing.T) {
	c, _ := adminClient(t)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	open := mustDo(t, c, "POST", "/api/pbx/timeconditions", map[string]any{"name": "Always open", "mode": "open",
		"open": map[string]string{"type": "extension", "target": "201"}, "closed": map[string]string{"type": "hangup"}}, 200)
	closed := mustDo(t, c, "POST", "/api/pbx/timeconditions", map[string]any{"name": "Always closed", "mode": "closed",
		"open": map[string]string{"type": "extension", "target": "201"}, "closed": map[string]string{"type": "hangup"}}, 200)
	rg := mustDo(t, c, "POST", "/api/pbx/ringgroups", map[string]any{"number": "601", "name": "Sales", "strategy": "simultaneous", "ringSeconds": 20, "members": []string{"201"}, "enabled": true}, 200)

	live := mustDo(t, c, "GET", "/api/pbx/ivrs/live", nil, 200)
	o := live["open"].(map[string]any)
	if o[fmt.Sprint(open["id"])] != true || o[fmt.Sprint(closed["id"])] != false {
		t.Fatalf("open = %v", o)
	}
	d := live["dests"].(map[string]any)
	if _, ok := d["extension:201"]; !ok {
		t.Fatalf("extension missing: %v", d)
	}
	if g := d["ringgroup:"+fmt.Sprint(rg["id"])].(map[string]any); g["members"] != float64(1) {
		t.Fatalf("ring group = %v", g)
	}
}
