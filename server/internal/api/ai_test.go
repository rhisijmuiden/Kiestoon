package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAISettingsAndKey(t *testing.T) {
	c, s := adminClient(t)
	code, body := c.do("GET", "/api/ai", nil)
	st, _ := body["settings"].(map[string]any)
	if code != 200 || st["engine"] != "local" || body["keySet"] != false {
		t.Fatalf("get: %d %v", code, body)
	}
	st["engine"], st["calls"] = "cloud", true
	if code, body := c.do("PUT", "/api/ai", st); code != 200 || body["settings"].(map[string]any)["engine"] != "cloud" {
		t.Fatalf("save: %d %v", code, body)
	}
	st["cloudModel"] = "gpt-5"
	if code, _ := c.do("PUT", "/api/ai", st); code != 400 {
		t.Fatalf("bad model accepted: %d", code)
	}
	if code, _ := c.do("PUT", "/api/ai/key", map[string]string{"key": "not-a-key"}); code != 400 {
		t.Fatalf("bad key accepted: %d", code)
	}
	code, body = c.do("PUT", "/api/ai/key", map[string]string{"key": "sk-ant-secret-1234"})
	if code != 200 || body["keySet"] != true || body["keyHint"] != "…1234" {
		t.Fatalf("key: %d %v", code, body)
	}
	// The key itself never comes back.
	_, all := c.do("GET", "/api/ai", nil)
	if b, _ := json.Marshal(all); strings.Contains(string(b), "secret") {
		t.Fatal("key leaked")
	}

	// The test button goes to Claude with the stored key (fake API here).
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-ant-secret-1234" && r.Header.Get("x-api-key") != "sk-ant-secret-1234" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"{\"summary\":\"Order missing, call back before three.\",\"actions\":[\"Call back\"],\"topic\":\"Missing order\",\"sentiment\":\"negative\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer api.Close()
	t.Setenv("KIESTOON_AI_BASE_URL", api.URL)
	st["cloudModel"] = "claude-opus-5-5"
	code, body = c.do("POST", "/api/ai/test", st)
	if code != 200 || body["summary"].(map[string]any)["topic"] != "Missing order" {
		t.Fatalf("test: %d %v", code, body)
	}
	_ = s
	if code, _ := c.do("PUT", "/api/ai/key", map[string]string{"key": ""}); code != 200 {
		t.Fatal("remove key")
	}
	if code, body := c.do("POST", "/api/ai/test", st); code != 409 || body["error"] != "ai_no_key" {
		t.Fatalf("no key: %d %v", code, body)
	}
}
