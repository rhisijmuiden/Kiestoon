package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func bearer(t *testing.T, c *client, token, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req) // no cookies
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestAPITokens(t *testing.T) {
	c, _ := adminClient(t)
	mustDo(t, c, "POST", "/api/tokens", map[string]any{"name": "x", "role": "superadmin"}, 400)
	rw := mustDo(t, c, "POST", "/api/tokens", map[string]any{"name": "CRM", "role": "admin"}, 201)
	ro := mustDo(t, c, "POST", "/api/tokens", map[string]any{"name": "Report", "role": "admin", "readOnly": true}, 201)
	tok := rw["token"].(string)
	if !strings.HasPrefix(tok, TokenPrefix) {
		t.Fatal(tok)
	}
	if code, _ := bearer(t, c, tok, "GET", "/api/pbx/extensions", nil); code != 200 {
		t.Fatalf("token read: %d", code)
	}
	if code, _ := bearer(t, c, tok, "POST", "/api/pbx/extensions", map[string]any{"number": "301", "name": "Api", "enabled": true}); code != 200 {
		t.Fatalf("token write: %d", code)
	}
	if code, _ := bearer(t, c, ro["token"].(string), "POST", "/api/pbx/extensions", map[string]any{"number": "302", "name": "No"}); code != 403 {
		t.Fatalf("read-only token wrote: %d", code)
	}
	if code, _ := bearer(t, c, tok, "GET", "/api/tokens", nil); code != 403 {
		t.Fatalf("token reached token management: %d", code)
	}
	if code, _ := bearer(t, c, TokenPrefix+"0000", "GET", "/api/pbx/extensions", nil); code != 401 {
		t.Fatalf("bad token: %d", code)
	}
	mustDo(t, c, "DELETE", "/api/tokens/"+jsonID(rw["id"]), map[string]any{}, 200)
	if code, _ := bearer(t, c, tok, "GET", "/api/pbx/extensions", nil); code != 401 {
		t.Fatalf("deleted token still works: %d", code)
	}
}

func jsonID(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestPhonebookFaxAndWebhooks(t *testing.T) {
	c, _ := adminClient(t)
	out := mustDo(t, c, "POST", "/api/phonebook/import", map[string]any{"csv": "naam;bedrijf;werk\nJan;Acme;020 123 4567\nPiet;;0301234567\n"}, 200)
	if out["added"].(float64) != 2 {
		t.Fatalf("import: %v", out)
	}
	mustDo(t, c, "POST", "/api/phonebook", map[string]any{"name": "", "work": "1"}, 400)

	fb := mustDo(t, c, "POST", "/api/pbx/faxboxes", map[string]any{"name": "Office fax", "emails": "fax@example.com", "keepDays": 30, "enabled": true}, 200)
	mustDo(t, c, "POST", "/api/pbx/faxboxes", map[string]any{"name": "Bad", "emails": "not-an-address"}, 400)
	mustDo(t, c, "POST", "/api/pbx/inbound", map[string]any{"name": "Fax", "did": "0201234599", "destinationType": "fax",
		"destination": jsonID(fb["id"]), "enabled": true}, 200)
	dests := mustDo(t, c, "GET", "/api/pbx/destinations", nil, 200)
	if !strings.Contains(jsonID(dests["options"]), "Office fax") {
		t.Fatal("fax box missing from destinations")
	}

	mustDo(t, c, "POST", "/api/webhooks", map[string]any{"name": "Local", "url": "http://127.0.0.1/x", "events": []string{"call.ended"}}, 400)
	mustDo(t, c, "POST", "/api/webhooks", map[string]any{"name": "CRM", "url": "https://crm.example.com/hook", "events": []string{"call.ended", "fax.received"}, "enabled": true}, 200)
	m := hmac.New(sha256.New, []byte("s"))
	m.Write([]byte("1.{}"))
	if got := Sign("s", "1", []byte("{}")); got != "sha256="+hex.EncodeToString(m.Sum(nil)) {
		t.Fatal(got)
	}
}
