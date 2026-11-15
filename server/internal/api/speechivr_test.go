package api

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpeechIVR(t *testing.T) {
	c, s := adminClient(t)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true, "voicemail": true}, 200)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "202", "name": "Bert", "enabled": true}, 200)
	node := func(id, typ string, extra map[string]any) map[string]any {
		m := map[string]any{"id": id, "type": typ, "x": 0, "y": 0}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	edge := func(from, port, to string) map[string]string { return map[string]string{"from": from, "port": port, "to": to} }
	dest := func(n string) map[string]any { return map[string]any{"destination": map[string]string{"type": "extension", "target": n}} }
	flow := map[string]any{
		"nodes": []any{
			node("e", "entry", nil), node("to", "timeout", nil),
			node("sp", "speech", map[string]any{"ai": true, "intents": []any{
				map[string]any{"label": "Sales & quotes", "keywords": []string{"offerte", "prijs", " offerte "}},
				map[string]any{"label": "Invoices", "keywords": []string{"factu"}},
			}}),
			node("d1", "destination", dest("201")), node("d2", "destination", dest("202")),
			node("vm", "destination", map[string]any{"destination": map[string]string{"type": "voicemail", "target": "201"}}),
		},
		"edges": []any{edge("e", "out", "to"), edge("to", "out", "sp"), edge("sp", "i1", "d1"), edge("sp", "i2", "d2"), edge("sp", "nomatch", "vm")},
	}
	ivr := mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Speech", "timeout": 3, "retries": 0, "flow": flow}, 200)
	id := int64(ivr["id"].(float64))
	// The timeout follows the "nomatch" line for the derived destination.
	if d := ivr["timeoutDestination"].(map[string]any); d["type"] != "voicemail" {
		t.Fatalf("derived timeout: %v", d)
	}
	for _, n := range ivr["flow"].(map[string]any)["nodes"].([]any) {
		if m := n.(map[string]any); m["id"] == "sp" {
			kws := m["intents"].([]any)[0].(map[string]any)["keywords"].([]any)
			if len(kws) != 2 {
				t.Fatalf("keywords not cleaned: %v", kws)
			}
		}
	}

	// A line from a port that is not an intent is refused.
	flow["edges"] = append(flow["edges"].([]any), edge("sp", "i3", "d1"))
	mustDo(t, c, "POST", "/api/pbx/ivrs", map[string]any{"name": "Bad", "flow": flow}, 400)

	if err := s.Ast.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Cfg.PBXDir(), "extensions.conf"))
	dp := string(b)
	for _, want := range []string{
		"Record(${KT_SPF},3,12,q)",
		fmt.Sprintf("AGI(agi://127.0.0.1:4573/speech,%d,sp,${KT_SPF})", id),
		`GotoIf($["${KT_INTENT}" = "2"]?i2)`,
		fmt.Sprintf("CELGenUserEvent(kt_ivrspeech,id=%d&intent=${KT_INTENTL})", id),
		" same => n(i1),Goto(",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}

	// The FastAGI side: a recording in the speech spool is recognised and removed.
	_ = os.MkdirAll(s.speechSpool(), 0o750)
	said := "Ik heb een vraag over mijn factuur"
	transcribeShort = func(*Server, context.Context, string, string, string) (string, error) { return said, nil }
	t.Cleanup(func() {
		transcribeShort = func(s *Server, ctx context.Context, model, language, file string) (string, error) {
			return s.speechEngine().TranscribeNow(ctx, model, language, file)
		}
	})
	agi := func(file string) []string {
		_ = os.WriteFile(file, []byte("x"), 0o640)
		a, b := net.Pipe()
		go s.serveAGI(context.Background(), b)
		fmt.Fprintf(a, "agi_network: yes\nagi_network_script: speech\nagi_arg_1: %d\nagi_arg_2: sp\nagi_arg_3: %s\n\n", id, file)
		r := bufio.NewReader(a)
		var cmds []string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				break
			}
			cmds = append(cmds, strings.TrimSpace(line))
			fmt.Fprint(a, "200 result=1\n")
		}
		return cmds
	}
	file := filepath.Join(s.speechSpool(), "1731644.1-1731644000.wav")
	cmds := agi(file)
	if len(cmds) != 2 || cmds[0] != `SET VARIABLE KT_INTENT "2"` || cmds[1] != `SET VARIABLE KT_INTENTL "Invoices"` {
		t.Fatalf("agi: %q", cmds)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("recording not removed")
	}
	said = "Wat kost een offerte?"
	if cmds := agi(file); len(cmds) != 2 || cmds[1] != `SET VARIABLE KT_INTENTL "Sales quotes"` {
		t.Fatalf("label: %q", cmds)
	}
	// Nothing matches and AI has no key: no variables (the dialplan takes "nomatch").
	said = "Goedemorgen"
	if cmds := agi(file); len(cmds) != 0 {
		t.Fatalf("no match: %q", cmds)
	}
	// Files outside the speech spool are never touched.
	outside := filepath.Join(t.TempDir(), "1-1.wav")
	if cmds := agi(outside); len(cmds) != 0 {
		t.Fatalf("outside: %q", cmds)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("file outside the spool was removed")
	}
}
