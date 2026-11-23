package api

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestLogViewerAndTrunkDiagnose(t *testing.T) {
	c, s := adminClient(t)
	s.Cfg.AsteriskLogDir = t.TempDir()
	if code, body := c.do("GET", "/api/logs?source=asterisk", nil); code != 404 || body["error"] != "log_missing" {
		t.Fatalf("missing log: %d %v", code, body)
	}
	log := "[2026-11-23 08:00:00] NOTICE[1] chan_pjsip.c: started\n" +
		"[2026-11-23 08:40:01] WARNING[2] res_pjsip_outbound_registration.c: Fatal response '403' received from 'sip:sip.cheap.example:5060' on registration attempt to 'sip:0501234@sip.cheap.example:5060', stopping outbound registration\n"
	os.WriteFile(filepath.Join(s.Cfg.AsteriskLogDir, "messages.log"), []byte(log), 0o600)

	code, body := c.do("GET", "/api/logs?source=asterisk&level=warning", nil)
	if code != 200 || len(body["entries"].([]any)) != 1 {
		t.Fatalf("read: %d %v", code, body)
	}
	if code, _ := c.do("GET", "/api/logs?source=../../etc/passwd", nil); code != 400 {
		t.Fatalf("unknown source: %d", code)
	}

	tr := &pbx.Trunk{Name: "Cheap", AuthMode: "register", Host: "sip.cheap.example", Port: 5060, Transport: "udp", Username: "0501234",
		Secret: "secret-long-enough-1", DIDSource: "to", Codecs: "alaw", Enabled: true}
	if err := s.PBX.SaveTrunk(t.Context(), tr); err != nil {
		t.Fatal(err)
	}
	code, d := c.do("GET", "/api/pbx/trunks/"+strconv.FormatInt(tr.ID, 10)+"/diagnose", nil)
	if code != 200 || d["reason"] != "forbidden" || d["code"] != "403" {
		t.Fatalf("diagnose: %d %v", code, d)
	}
}
