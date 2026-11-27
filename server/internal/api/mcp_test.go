package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// mcpTestServer serves the API and the MCP endpoints like main.go does.
func mcpTestServer(t *testing.T, s *Server) string {
	mux := http.NewServeMux()
	mux.Handle("/api/", s.Routes())
	mux.Handle("/", s.MCPHandler())
	ts := httptest.NewServer(SecurityHeaders(mux))
	t.Cleanup(ts.Close)
	return ts.URL
}

type rpcOut struct {
	status int
	header http.Header
	body   map[string]any
}

func mcpRPC(t *testing.T, base, token, method string, params any) rpcOut {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", base+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return rpcOut{resp.StatusCode, resp.Header, out}
}

// toolCall returns the text of a tool result and whether it is an error.
func toolCall(t *testing.T, base, token, name string, args map[string]any) (string, bool) {
	t.Helper()
	o := mcpRPC(t, base, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if o.status != 200 {
		t.Fatalf("%s: %d %v", name, o.status, o.body)
	}
	res, ok := o.body["result"].(map[string]any)
	if !ok {
		t.Fatalf("%s: no result: %v", name, o.body)
	}
	content := res["content"].([]any)[0].(map[string]any)
	return content["text"].(string), res["isError"] == true
}

func toolNames(t *testing.T, base, token string) []string {
	o := mcpRPC(t, base, token, "tools/list", map[string]any{})
	names := []string{}
	for _, x := range o.body["result"].(map[string]any)["tools"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	return names
}

func hasName(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestMCPTokensToolsAndConfirmation(t *testing.T) {
	c, s := adminClient(t)
	base := mcpTestServer(t, s)
	ext := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true, "voicemail": true}, 200)
	extID := jsonID(ext["id"])

	// Switched off by default: no connections and the endpoint refuses.
	mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "x", "scopes": []string{"pbx:read"}}, 409)
	if o := mcpRPC(t, base, "kt_mcp_x", "initialize", map[string]any{}); o.status != 503 {
		t.Fatalf("disabled endpoint: %d", o.status)
	}
	mustDo(t, c, "PUT", "/api/mcp/settings", map[string]any{"enabled": true, "defaultReadOnly": true, "confirmWrites": true, "allowUsers": true, "disabledTools": []string{}}, 200)

	mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "x", "scopes": []string{"nope"}}, 400)
	mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "x", "scopes": []string{"pbx:read"}, "allowedIps": []string{"not an ip"}}, 400)
	rw := mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "Claude Code", "scopes": []string{"pbx:read", "pbx:write"}, "expiresDays": 90}, 201)
	tok := rw["token"].(string)
	if !strings.HasPrefix(tok, mcpTokenPrefix) {
		t.Fatal(tok)
	}

	// No token: 401 with the resource metadata for OAuth discovery.
	o := mcpRPC(t, base, "", "initialize", map[string]any{})
	if o.status != 401 || !strings.Contains(o.header.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/mcp") {
		t.Fatalf("unauthenticated: %d %q", o.status, o.header.Get("WWW-Authenticate"))
	}
	o = mcpRPC(t, base, tok, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	res := o.body["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" || o.header.Get("Mcp-Session-Id") == "" {
		t.Fatalf("initialize: %v", o.body)
	}
	// The MCP token is no REST token.
	if code, _ := bearer(t, c, tok, "GET", "/api/pbx/extensions", nil); code != 401 {
		t.Fatalf("mcp token on REST: %d", code)
	}

	names := toolNames(t, base, tok)
	if !hasName(names, "list_config") || !hasName(names, "update_config") || hasName(names, "search_calls") || hasName(names, "click_to_call") {
		t.Fatalf("tools: %v", names)
	}

	text, isErr := toolCall(t, base, tok, "list_config", map[string]any{"kind": "extensions"})
	if isErr || !strings.Contains(text, "Anna") {
		t.Fatalf("list: %s", text)
	}
	text, _ = toolCall(t, base, tok, "get_config", map[string]any{"kind": "extensions", "id": ext["id"]})
	if !strings.Contains(text, `"sipSecret": "[redacted]"`) {
		t.Fatalf("secret not redacted: %s", text)
	}
	if text, isErr = toolCall(t, base, tok, "search_calls", map[string]any{}); !isErr || !strings.Contains(text, "calls:read") {
		t.Fatalf("out of scope: %s", text)
	}

	// A change is a proposal first; nothing is saved until the confirmation code comes back.
	args := map[string]any{"kind": "extensions", "id": ext["id"], "changes": map[string]any{"name": "Anna de Vries"}}
	text, isErr = toolCall(t, base, tok, "update_config", args)
	if isErr || !strings.Contains(text, "PROPOSAL") || !strings.Contains(text, "Anna de Vries") {
		t.Fatalf("proposal: %s", text)
	}
	var prop map[string]any
	_ = json.Unmarshal([]byte(text), &prop)
	code := prop["confirm"].(string)
	got := mustDo(t, c, "GET", "/api/pbx/extensions/"+extID, nil, 200)
	if got["name"] != "Anna" {
		t.Fatal("saved before confirmation")
	}
	other := map[string]any{"kind": "extensions", "id": ext["id"], "changes": map[string]any{"name": "Someone else"}, "confirm": code}
	if text, isErr = toolCall(t, base, tok, "update_config", other); !isErr {
		t.Fatalf("code accepted for other arguments: %s", text)
	}
	text, _ = toolCall(t, base, tok, "update_config", args)
	_ = json.Unmarshal([]byte(text), &prop)
	args["confirm"] = prop["confirm"]
	if text, isErr = toolCall(t, base, tok, "update_config", args); isErr {
		t.Fatalf("confirmed update: %s", text)
	}
	got = mustDo(t, c, "GET", "/api/pbx/extensions/"+extID, nil, 200)
	if got["name"] != "Anna de Vries" || got["sipSecret"] == "" || got["sipSecret"] == redacted {
		t.Fatalf("after update: %v", got)
	}
	if text, isErr = toolCall(t, base, tok, "update_config", args); !isErr {
		t.Fatalf("confirmation reused: %s", text)
	}
	// Trunks are read only via MCP.
	if text, isErr = toolCall(t, base, tok, "create_config", map[string]any{"kind": "trunks", "fields": map[string]any{}}); !isErr {
		t.Fatalf("trunk create: %s", text)
	}

	// A read-only connection does not see the write tools, and its session cannot write either.
	ro := mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "Reader", "scopes": []string{"pbx:read"}}, 201)
	if names := toolNames(t, base, ro["token"].(string)); hasName(names, "update_config") {
		t.Fatalf("read-only tools: %v", names)
	}

	// Tools switched off by the administrator disappear.
	mustDo(t, c, "PUT", "/api/mcp/settings", map[string]any{"enabled": true, "confirmWrites": true, "allowUsers": true, "disabledTools": []string{"list_config"}}, 200)
	if names := toolNames(t, base, tok); hasName(names, "list_config") {
		t.Fatal("disabled tool listed")
	}

	// Resources and prompts.
	o = mcpRPC(t, base, tok, "resources/read", map[string]any{"uri": "kiestoon://guide"})
	if !strings.Contains(fmt.Sprint(o.body["result"]), "Kiestoon") {
		t.Fatalf("guide: %v", o.body)
	}
	o = mcpRPC(t, base, tok, "prompts/get", map[string]any{"name": "onboard_employee", "arguments": map[string]any{"name": "Lisa"}})
	if !strings.Contains(fmt.Sprint(o.body["result"]), "Lisa") {
		t.Fatalf("prompt: %v", o.body)
	}
	if o = mcpRPC(t, base, tok, "nope/nope", nil); o.body["error"] == nil {
		t.Fatal("unknown method")
	}

	// Activity log.
	_, act := c.do("GET", "/api/mcp/activity", nil)
	_ = act
	rows := mustDoList(t, c, "/api/mcp/activity?filter=denied")
	if len(rows) == 0 {
		t.Fatal("denied calls not logged")
	}

	// Allowed addresses.
	ipc := mustDo(t, c, "POST", "/api/mcp/connections", map[string]any{"name": "Office only", "scopes": []string{"pbx:read"}, "allowedIps": []string{"192.0.2.0/24"}}, 201)
	if o = mcpRPC(t, base, ipc["token"].(string), "initialize", map[string]any{}); o.status != 403 {
		t.Fatalf("ip allow list: %d", o.status)
	}

	// Revoked: 401.
	mustDo(t, c, "DELETE", "/api/mcp/connections/"+jsonID(rw["id"]), map[string]any{}, 200)
	if o = mcpRPC(t, base, tok, "tools/list", map[string]any{}); o.status != 401 {
		t.Fatalf("revoked: %d", o.status)
	}
}

func pkcePair() (string, string) {
	v := strings.Repeat("abcdefghij", 5)
	h := sha256.Sum256([]byte(v))
	return v, base64.RawURLEncoding.EncodeToString(h[:])
}

func TestMCPOAuthFlow(t *testing.T) {
	c, s := adminClient(t)
	base := mcpTestServer(t, s)
	mustDo(t, c, "PUT", "/api/mcp/settings", map[string]any{"enabled": true, "defaultReadOnly": true, "confirmWrites": true, "allowUsers": true, "disabledTools": []string{}}, 200)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	var meta map[string]any
	resp, _ := http.Get(base + "/.well-known/oauth-protected-resource/mcp")
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	resp.Body.Close()
	if meta["resource"] != base+"/mcp" {
		t.Fatalf("resource metadata: %v", meta)
	}
	resp, _ = http.Get(base + "/.well-known/oauth-authorization-server")
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	resp.Body.Close()
	if meta["registration_endpoint"] != base+"/oauth/register" {
		t.Fatalf("server metadata: %v", meta)
	}

	cb := "https://claude.ai/api/mcp/auth_callback"
	b, _ := json.Marshal(map[string]any{"client_name": "Claude", "redirect_uris": []string{cb}})
	resp, _ = http.Post(base+"/oauth/register", "application/json", bytes.NewReader(b))
	var reg map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d %v", resp.StatusCode, reg)
	}
	clientID := reg["client_id"].(string)
	b, _ = json.Marshal(map[string]any{"client_name": "Evil", "redirect_uris": []string{"http://evil.example/cb"}})
	if resp, _ = http.Post(base+"/oauth/register", "application/json", bytes.NewReader(b)); resp.StatusCode != 400 {
		t.Fatalf("http redirect accepted: %d", resp.StatusCode)
	}

	verifier, challenge := pkcePair()
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {cb}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"xyz"}, "scope": {"calls:read pbx:read"}}
	resp, _ = noRedirect.Get(base + "/oauth/authorize?" + q.Encode())
	if resp.StatusCode != 302 || !strings.HasPrefix(resp.Header.Get("Location"), "/oauth/consent?") {
		t.Fatalf("authorize: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	bad := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"https://evil.example/cb"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	if resp, _ = noRedirect.Get(base + "/oauth/authorize?" + bad.Encode()); resp.StatusCode != 400 {
		t.Fatalf("unknown redirect: %d", resp.StatusCode)
	}

	info := mustDo(t, c, "GET", "/api/mcp/oauth/request?q="+url.QueryEscape(q.Encode()), nil, 200)
	if info["client"] != "Claude" || info["redirectHost"] != "claude.ai" {
		t.Fatalf("request info: %v", info)
	}
	dec := mustDo(t, c, "POST", "/api/mcp/oauth/decide", map[string]any{"query": q.Encode(), "allow": true, "scopes": []string{"calls:read"}, "days": 30}, 200)
	ru, _ := url.Parse(dec["redirect"].(string))
	if ru.Query().Get("state") != "xyz" || ru.Query().Get("code") == "" {
		t.Fatalf("decide: %v", dec)
	}

	token := func(form url.Values) (int, map[string]any) {
		resp, err := http.PostForm(base+"/oauth/token", form)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	codeForm := url.Values{"grant_type": {"authorization_code"}, "code": {ru.Query().Get("code")}, "client_id": {clientID}, "redirect_uri": {cb}, "code_verifier": {"wrong" + verifier}}
	if st, _ := token(codeForm); st != 400 {
		t.Fatalf("bad verifier: %d", st)
	}
	// The code was used up by the failed attempt; get a new one.
	dec = mustDo(t, c, "POST", "/api/mcp/oauth/decide", map[string]any{"query": q.Encode(), "allow": true, "scopes": []string{"calls:read"}, "days": 30}, 200)
	ru, _ = url.Parse(dec["redirect"].(string))
	codeForm.Set("code", ru.Query().Get("code"))
	codeForm.Set("code_verifier", verifier)
	st, tk := token(codeForm)
	if st != 200 || !strings.HasPrefix(tk["access_token"].(string), mcpAccessPrefix) {
		t.Fatalf("token: %d %v", st, tk)
	}
	names := toolNames(t, base, tk["access_token"].(string))
	if !hasName(names, "search_calls") || hasName(names, "list_config") {
		t.Fatalf("oauth tools: %v", names)
	}

	st, tk2 := token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tk["refresh_token"].(string)}, "client_id": {clientID}})
	if st != 200 || tk2["access_token"] == tk["access_token"] {
		t.Fatalf("refresh: %d %v", st, tk2)
	}
	if st, _ = token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tk["refresh_token"].(string)}, "client_id": {clientID}}); st != 400 {
		t.Fatalf("old refresh token reused: %d", st)
	}
	if o := mcpRPC(t, base, tk["access_token"].(string), "tools/list", nil); o.status != 401 {
		t.Fatalf("old access token: %d", o.status)
	}
	if o := mcpRPC(t, base, tk2["access_token"].(string), "tools/list", nil); o.status != 200 {
		t.Fatalf("new access token: %d", o.status)
	}
	conns := mustDoList(t, c, "/api/mcp/connections")
	if len(conns) != 1 || conns[0]["kind"] != "oauth" || conns[0]["name"] != "Claude" {
		t.Fatalf("connections: %v", conns)
	}

	// Denying sends the client an access_denied.
	dec = mustDo(t, c, "POST", "/api/mcp/oauth/decide", map[string]any{"query": q.Encode(), "allow": false}, 200)
	if !strings.Contains(dec["redirect"].(string), "error=access_denied") {
		t.Fatalf("deny: %v", dec)
	}
}

func TestMCPHelpers(t *testing.T) {
	r := redact(map[string]any{"name": "x", "sipSecret": "abc", "hasPassword": true, "voicemailPin": "", "nested": []any{map[string]any{"apiKey": "k"}}}).(map[string]any)
	if r["sipSecret"] != redacted || r["hasPassword"] != true || r["voicemailPin"] != "" || r["nested"].([]any)[0].(map[string]any)["apiKey"] != redacted {
		t.Fatalf("redact: %v", r)
	}
	if got := maskOne("call from +31612345678 to 201"); got != "call from +3161•••••78 to 201" {
		t.Fatalf("mask: %q", got)
	}
	v, ch := pkcePair()
	if !pkceOK(v, ch) || pkceOK(v+"x", ch) {
		t.Fatal("pkce")
	}
	if !ipAllowed(nil, "1.2.3.4") || !ipAllowed([]string{"10.0.0.0/8"}, "10.1.2.3") || ipAllowed([]string{"10.0.0.0/8"}, "11.1.2.3") {
		t.Fatal("ip allow")
	}
	if !fromAnthropic("160.79.105.10") || fromAnthropic("8.8.8.8") {
		t.Fatal("anthropic ranges")
	}
	set := defaultMCPSettings()
	if got := cleanScopes([]string{"media:read", "calls:read", "pbx:write"}, "user", set); len(got) != 1 || got[0] != "calls:read" {
		t.Fatalf("scopes for user: %v", got)
	}
	set.ShareMedia = true
	if got := cleanScopes([]string{"media:read"}, "user", set); len(got) != 1 {
		t.Fatalf("media when shared: %v", got)
	}
	if !validRedirect("https://claude.ai/cb") || !validRedirect("http://localhost:3000/cb") || validRedirect("http://example.com/cb") || validRedirect("https://x/cb#f") {
		t.Fatal("redirects")
	}
}
