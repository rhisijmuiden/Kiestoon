package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTeamsSettings(t *testing.T) {
	c, _ := adminClient(t)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	get := mustDo(t, c, "GET", "/api/teams", nil, 200)
	set := get["settings"].(map[string]any)
	if set["enabled"] != false || set["port"].(float64) != 5067 {
		t.Fatalf("defaults: %v", set)
	}
	st := get["status"].(map[string]any)
	if st["active"] != false || len(st["hubs"].([]any)) != 3 {
		t.Fatalf("status: %v", st)
	}
	body := map[string]any{"enabled": true, "fqdn": "sbc.example.nl", "port": 5067, "callerId": "own", "maxSessions": 20, "ringBoth": true,
		"users": []map[string]any{{"name": "Anna", "upn": "anna@example.nl", "number": "020 123 4501", "extension": "201", "ring": "both"}}}
	out := mustDo(t, c, "PUT", "/api/teams", body, 200)
	if u := out["settings"].(map[string]any)["users"].([]any)[0].(map[string]any); u["number"] != "+31201234501" {
		t.Fatalf("number not cleaned: %v", u)
	}
	body["users"] = []map[string]any{{"upn": "x@example.nl", "number": "+31201234501", "extension": "999", "ring": "both"}}
	if code, res := c.do("PUT", "/api/teams", body); code != 400 || res["field"] != "users.0.extension" {
		t.Fatalf("unknown extension: %d %v", code, res)
	}
	body["users"] = []map[string]any{}
	body["trunkId"] = 99
	if code, res := c.do("PUT", "/api/teams", body); code != 400 || res["field"] != "trunkId" {
		t.Fatalf("unknown trunk: %d %v", code, res)
	}
}

func TestTeamsScriptDownload(t *testing.T) {
	c, _ := adminClient(t)
	code, text := c.raw("GET", "/api/teams/script?lang=nl")
	if code != 200 || !strings.Contains(text, "New-CsOnlinePSTNGateway") || !strings.Contains(text, "Nummers van de gebruikers") {
		t.Fatalf("script: %d %s", code, text)
	}
}

func (c *client) raw(method, path string) (int, string) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.url+path, nil)
	for k, v := range c.hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
