package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sample = []string{
	"[2026-11-23 08:30:00] NOTICE[100] res_pjsip_outbound_registration.c: Outbound registration to 'sip:other.nl' ok",
	"[2026-11-23 08:39:58] WARNING[200] res_pjsip_outbound_registration.c: No response received from 'sip:sip.cheap.nl:5060' on registration attempt to 'sip:0501234@sip.cheap.nl:5060', retrying in '60'",
	"[2026-11-23 08:40:01] WARNING[201] res_pjsip_outbound_registration.c: Fatal response '403' received from 'sip:sip.cheap.nl:5060' on registration attempt to 'sip:0501234@sip.cheap.nl:5060', stopping outbound registration",
	"[2026-11-23 08:40:02] ERROR[202][C-00000001] app_dial.c: Unable to create channel of type 'PJSIP' (cause 3 - No route to destination)",
	"  continued line",
	"[2026-11-23 08:41:00] WARNING[203] res_pjsip_outbound_registration.c: Fatal response '401' received from 'sip:trunk-12.example' on registration attempt for trunk-12",
}

func TestParseAndFilter(t *testing.T) {
	e := ParseAsterisk(sample[2])
	if e.At != "2026-11-23 08:40:01" || e.Level != "warning" || e.Module != "res_pjsip_outbound_registration" || !strings.HasPrefix(e.Text, "Fatal response") {
		t.Fatalf("parse: %+v", e)
	}
	c := ParseAsterisk(sample[3])
	if c.Level != "error" || c.Module != "app_dial" {
		t.Fatalf("call id: %+v", c)
	}
	all := Filter(sample, ParseAsterisk, Query{})
	if len(all) != 5 || all[0].At != "2026-11-23 08:41:00" || !strings.Contains(all[1].Text, "continued line") {
		t.Fatalf("filter all: %d %+v", len(all), all)
	}
	if got := Filter(sample, ParseAsterisk, Query{Level: "error"}); len(got) != 1 {
		t.Fatalf("level: %+v", got)
	}
	if got := Filter(sample, ParseAsterisk, Query{Q: "CHEAP fatal"}); len(got) != 1 || !strings.Contains(got[0].Text, "403") {
		t.Fatalf("search: %+v", got)
	}
	if got := Filter(sample, ParseAsterisk, Query{Limit: 2}); len(got) != 2 {
		t.Fatalf("limit: %d", len(got))
	}
}

func TestDiagnose(t *testing.T) {
	entries := Filter(sample, ParseAsterisk, Query{})
	d := Diagnose(entries, TrunkMatch{Endpoint: "trunk-1", Host: "sip.cheap.nl", Username: "0501234"})
	if d.Reason != "forbidden" || d.Code != "403" || len(d.Lines) != 2 {
		t.Fatalf("diagnose: %+v", d)
	}
	// trunk-1 must not match trunk-12.
	if d := Diagnose(entries, TrunkMatch{Endpoint: "trunk-1"}); d.Reason != "" || len(d.Lines) != 0 {
		t.Fatalf("trunk-1 matched trunk-12: %+v", d)
	}
	if d := Diagnose(entries, TrunkMatch{Endpoint: "trunk-12"}); d.Reason != "auth_failed" {
		t.Fatalf("401: %+v", d)
	}
	timeout := Filter(sample[:2], ParseAsterisk, Query{})
	if d := Diagnose(timeout, TrunkMatch{Host: "sip.cheap.nl"}); d.Reason != "timeout" {
		t.Fatalf("timeout: %+v", d)
	}
	dns := []Entry{{At: "x", Level: "warning", Module: "pjsip_resolver", Text: "Failed to resolve 'sip.nope.nl'"}}
	if d := Diagnose(dns, TrunkMatch{Host: "sip.nope.nl"}); d.Reason != "dns" {
		t.Fatalf("dns: %+v", d)
	}
}

func TestParseJournal(t *testing.T) {
	e := ParseJournal("2026-11-23T08:40:01+01:00 pbx kiestoon[812]: time=2026-11-23T08:40:01.000+01:00 level=WARN msg=\"mail failed\"")
	if e.At != "2026-11-23 08:40:01" || e.Level != "warning" || e.Module != "kiestoon" {
		t.Fatalf("journal: %+v", e)
	}
}

func TestTailRotated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "messages.log")
	os.WriteFile(p+".1", []byte("old 1\nold 2\n"), 0o600)
	os.WriteFile(p, []byte("new 1\n"), 0o600)
	lines, err := TailLines(p)
	if err != nil || strings.Join(lines, ",") != "old 1,old 2,new 1" {
		t.Fatalf("tail: %v %v", lines, err)
	}
}
