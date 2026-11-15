package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestTIFFPages(t *testing.T) {
	// A little-endian TIFF with three empty image directories.
	var b bytes.Buffer
	b.WriteString("II")
	binary.Write(&b, binary.LittleEndian, uint16(42))
	binary.Write(&b, binary.LittleEndian, uint32(8))
	for i := 0; i < 3; i++ {
		binary.Write(&b, binary.LittleEndian, uint16(0))
		next := uint32(0)
		if i < 2 {
			next = uint32(8 + (i+1)*6)
		}
		binary.Write(&b, binary.LittleEndian, next)
	}
	p := filepath.Join(t.TempDir(), "f.tif")
	os.WriteFile(p, b.Bytes(), 0o600)
	if n := tiffPages(p); n != 3 {
		t.Fatalf("pages: %d", n)
	}
}

const faxMail = "From: Jan <Jan@Example.com>\r\nTo: fax+0201234567@example.com\r\nSubject: =?utf-8?q?Offerte?= 1234\r\n" +
	"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n--b1\r\nContent-Type: text/plain\r\n\r\nHallo\r\n" +
	"--b1\r\nContent-Type: application/pdf; name=\"offerte.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n--b1--\r\n"

func TestMailToFax(t *testing.T) {
	c, s := adminClient(t)
	mustDo(t, c, "POST", "/api/pbx/faxboxes", map[string]any{"name": "Fax", "emails": "fax@example.com", "enabled": true,
		"sendEnabled": true, "senders": "jan@example.com"}, 400) // number required
	fb := mustDo(t, c, "POST", "/api/pbx/faxboxes", map[string]any{"name": "Fax", "emails": "fax@example.com", "enabled": true,
		"sendEnabled": true, "number": "0201234500", "senders": "Jan@example.com", "sendPin": "1234"}, 200)
	if fb["sendPin"] != nil || fb["sendPinSet"] != true {
		t.Fatalf("PIN leaked: %v", fb)
	}
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 test"))
	ctx := context.Background()
	boxes, _ := s.PBX.FaxBoxes(ctx)
	s.mailToFax(ctx, boxes, []byte(strings.Replace(faxMail, "%s", pdf, 1)))
	jobs, _ := s.PBX.FaxJobs(ctx, nil, 10)
	if len(jobs) != 1 || jobs[0].Number != "0201234567" || jobs[0].Source != "email" || jobs[0].Document != "offerte.pdf" {
		t.Fatalf("jobs: %+v", jobs)
	}
	// Wrong PIN, unknown sender, failed DMARC: nothing is queued.
	s.mailToFax(ctx, boxes, []byte(strings.Replace(strings.Replace(faxMail, "1234\r\n", "9999\r\n", 1), "%s", pdf, 1)))
	s.mailToFax(ctx, boxes, []byte(strings.Replace(strings.Replace(faxMail, "Jan@Example.com", "eve@evil.test", 1), "%s", pdf, 1)))
	s.mailToFax(ctx, boxes, []byte("Authentication-Results: mx; dmarc=fail\r\n"+strings.Replace(faxMail, "%s", pdf, 1)))
	if jobs, _ := s.PBX.FaxJobs(ctx, nil, 10); len(jobs) != 1 {
		t.Fatalf("refused mails were queued: %d", len(jobs))
	}
	// Portal upload.
	mustDo(t, c, "POST", "/api/fax/send", map[string]any{"boxId": fb["id"], "number": "abc", "pdf": pdf}, 400)
	mustDo(t, c, "POST", "/api/fax/send", map[string]any{"boxId": fb["id"], "number": "020-123 45 68", "pdf": base64.StdEncoding.EncodeToString([]byte("not a pdf"))}, 400)
	j := mustDo(t, c, "POST", "/api/fax/send", map[string]any{"boxId": fb["id"], "number": "020-123 45 68", "name": "a.pdf", "pdf": pdf}, 200)
	if j["number"] != "0201234568" || j["status"] != "queued" {
		t.Fatalf("job: %v", j)
	}
	if got := mustDoList(t, c, "/api/fax/send/boxes"); len(got) != 1 {
		t.Fatalf("send boxes: %v", got)
	}
}

func TestVoicemailTranscriptMail(t *testing.T) {
	_, s := adminClient(t)
	audio := []byte("RIFF....WAVEfmt fake audio")
	os.MkdirAll(s.transcriptDir(), 0o750)
	os.WriteFile(filepath.Join(s.transcriptDir(), sumHex(audio)+".txt"), []byte("Bel me even terug."), 0o640)
	s.DB.Exec(`INSERT INTO settings(key, value) VALUES('speech', '{"stt":{"enabled":true,"voicemail":true,"model":"base","language":"nl"},"tts":{"voiceEn":"en_GB-cori-medium","voiceNl":"nl_NL-pim-medium"}}')`)
	raw := "To: anna@example.com\r\nSubject: New voicemail\r\nX-Asterisk-VM-Message-Num: 1\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"----voicemail_1\"\r\n\r\nThis is a multi-part message in MIME format.\r\n\r\n" +
		"------voicemail_1\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\nHello Anna,\r\nA new message.\r\n\r\n" +
		"------voicemail_1\r\nContent-Type: audio/x-wav; name=\"msg0000.wav\"\r\nContent-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"msg0000.wav\"\r\n\r\n" + base64.StdEncoding.EncodeToString(audio) + "\r\n\r\n------voicemail_1--\r\n"
	out := s.prepareMail(context.Background(), []byte(raw))
	if out == nil || !bytes.Contains(out, []byte("Bel me even terug.")) || !bytes.Contains(out, []byte(base64.StdEncoding.EncodeToString(audio))) {
		t.Fatalf("transcript not added:\n%s", out)
	}
	if s.prepareMail(context.Background(), []byte("To: a@b.c\r\nSubject: x\r\n\r\nplain")) != nil {
		t.Fatal("non-voicemail mail changed")
	}
}

func TestMetricsAndFirmware(t *testing.T) {
	c, s := adminClient(t)
	h := s.MetricsHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 404 {
		t.Fatalf("metrics off: %d", rec.Code)
	}
	mustDo(t, c, "PUT", "/api/system/metrics", map[string]any{"enabled": true, "token": "short"}, 400)
	mustDo(t, c, "PUT", "/api/system/metrics", map[string]any{"enabled": true, "token": "kt_metrics_0123456789abcdef", "allowFrom": []string{"198.51.100.0/24"}}, 200)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer kt_metrics_0123456789abcdef")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "kiestoon_info{version=") || !strings.Contains(rec.Body.String(), `kiestoon_fax_jobs{status="queued"} 0`) {
		t.Fatalf("metrics: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "198.51.100.7:4000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("allowed address: %d", rec.Code)
	}

	// Firmware: upload without the CSRF header is refused, with it stored and served.
	up, _ := http.NewRequest("POST", c.url+"/api/firmware/upload?vendor=yealink&name=T54W-96.86.0.70.rom", strings.NewReader("ROMDATA"))
	up.Header.Set("Content-Type", "application/octet-stream")
	if resp, _ := c.hc.Do(up); resp.StatusCode != 403 {
		t.Fatalf("upload without header: %d", resp.StatusCode)
	}
	up, _ = http.NewRequest("POST", c.url+"/api/firmware/upload?vendor=yealink&name=T54W-96.86.0.70.rom", strings.NewReader("ROMDATA"))
	up.Header.Set("X-Kiestoon-Upload", "1")
	resp, err := c.hc.Do(up)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %d", err, resp.StatusCode)
	}
	var fw map[string]any
	jsonDecode(resp, &fw)
	mustDo(t, c, "PUT", "/api/firmware/"+jsonID(fw["id"]), map[string]any{"version": "96.86.0.70", "models": []string{"NOPE"}, "active": true}, 400)
	mustDo(t, c, "PUT", "/api/firmware/"+jsonID(fw["id"]), map[string]any{"version": "96.86.0.70", "models": []string{"T54W"}, "active": true}, 200)
	f := s.Firmware.ForModel(context.Background(), "yealink", "T54W")
	if f == nil {
		t.Fatal("firmware not active for T54W")
	}
	ph := &pbx.Phone{MAC: "001565abcdef", Vendor: "yealink", Model: "T54W"}
	in, _ := s.provInput(context.Background(), ph, "10.0.0.1:8080", "http")
	if in.Firmware != "http://10.0.0.1:8080/prov/fw/"+f.Token+"/T54W-96.86.0.70.rom" {
		t.Fatalf("firmware url: %s", in.Firmware)
	}
	rec = httptest.NewRecorder()
	s.ProvisioningHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/prov/fw/"+f.Token+"/T54W-96.86.0.70.rom", nil))
	if rec.Code != 200 || rec.Body.String() != "ROMDATA" {
		t.Fatalf("download: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.ProvisioningHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/prov/fw/"+f.Token+"/../kiestoon.db", nil))
	if rec.Code != 404 {
		t.Fatalf("traversal: %d", rec.Code)
	}
	// Proxy settings.
	mustDo(t, c, "PUT", "/api/system/proxy", map[string]any{"enabled": true, "trustedProxies": []string{}}, 400)
	mustDo(t, c, "PUT", "/api/system/proxy", map[string]any{"enabled": true, "trustedProxies": []string{"10.9.0.0/16"}, "publicUrl": "https://pbx.example.com"}, 200)
	defer setTrustedProxies(nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.9.1.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if clientIP(r) != "203.0.113.9" {
		t.Fatal("trusted proxy not applied")
	}
	if pub, _ := s.DB.Query(`SELECT value FROM settings WHERE key = 'public_url'`); pub != nil {
		var v string
		pub.Next()
		pub.Scan(&v)
		pub.Close()
		if v != "https://pbx.example.com" {
			t.Fatalf("public url: %q", v)
		}
	}
}
