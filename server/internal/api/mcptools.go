package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
)

// mcpTool is one tool of the MCP server. Run calls the portal API in-process as the connection's user.
type mcpTool struct {
	Name    string
	Title   string
	Desc    string
	Scope   string
	MinRole string
	// Kind overrides the kind of the scope: read, act or write. Write tools need confirmation.
	Kind        string
	Props       map[string]any
	Required    []string
	Destructive bool
	// Needs hides the tool when the connection cannot use it (for example no linked extension).
	Needs   func(c *mcpCall) bool
	Run     func(c *mcpCall, a mcpArgs) (any, error)
	Preview func(c *mcpCall, a mcpArgs) (any, error)
}

func (t *mcpTool) kind() string {
	if t.Kind != "" {
		return t.Kind
	}
	sc, _ := mcpScopeByKey(t.Scope)
	return sc.Kind
}

func (t *mcpTool) describe(set mcpSettings) map[string]any {
	props := map[string]any{}
	for k, v := range t.Props {
		props[k] = v
	}
	desc := t.Desc
	if t.kind() == "write" && set.ConfirmWrites {
		props["confirm"] = map[string]any{"type": "string", "description": "Confirmation code from the proposal. Leave out on the first call."}
		desc += " First call returns a proposal; call again with the same arguments plus confirm after the user agreed."
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(t.Required) > 0 {
		schema["required"] = t.Required
	}
	k := t.kind()
	return map[string]any{
		"name": t.Name, "title": t.Title, "description": desc, "inputSchema": schema,
		"annotations": map[string]any{
			"title": t.Title, "readOnlyHint": k == "read", "destructiveHint": t.Destructive,
			"idempotentHint": k == "read", "openWorldHint": false,
		},
	}
}

func mcpToolByName(n string) (*mcpTool, bool) {
	for _, t := range mcpTools {
		if t.Name == n {
			return t, true
		}
	}
	return nil, false
}

// ---- arguments ---------------------------------------------------------------------

type mcpArgs map[string]any

func (a mcpArgs) str(k string) string {
	switch v := a[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

func (a mcpArgs) int(k string) int64 {
	switch v := a[k].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	}
	return 0
}

func (a mcpArgs) has(k string) bool { _, ok := a[k]; return ok }

func (a mcpArgs) obj(k string) map[string]any {
	m, _ := a[k].(map[string]any)
	return m
}

// mcpErr is an error the assistant can act on: an error code and, for validation, the field.
type mcpErr struct {
	Code  string
	Field string
	Msg   string
}

func (e *mcpErr) Error() string {
	s := e.Code
	if e.Field != "" {
		s += " (field: " + e.Field + ")"
	}
	if e.Msg != "" {
		s += ": " + e.Msg
	}
	return s
}

func argErr(field, msg string) error {
	return &mcpErr{Code: "invalid_argument", Field: field, Msg: msg}
}

// ---- in-process API calls --------------------------------------------------------------

// do calls the portal API as the connection's user and decodes the JSON answer.
func (c *mcpCall) do(method, path string, q url.Values, body any) (any, error) {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	ctx := context.WithValue(c.r.Context(), mcpSessKey{}, c.sess)
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return nil, err
	}
	req.Host = c.r.Host
	req.RemoteAddr = c.r.RemoteAddr
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"} {
		if v := c.r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c.s.apiHandler().ServeHTTP(rec, req)
	dec := json.NewDecoder(rec.Body)
	dec.UseNumber()
	var out any
	_ = dec.Decode(&out)
	if rec.Code >= 400 {
		e := &mcpErr{Code: fmt.Sprintf("http_%d", rec.Code)}
		if m, ok := out.(map[string]any); ok {
			if v, _ := m["error"].(string); v != "" {
				e.Code = v
			}
			e.Field, _ = m["field"].(string)
			e.Msg, _ = m["message"].(string)
		}
		return nil, e
	}
	return out, nil
}

func (c *mcpCall) get(path string, q url.Values) (any, error) {
	return c.do(http.MethodGet, path, q, nil)
}

// ---- tool calls ----------------------------------------------------------------------------

type mcpProposal struct {
	connID  int64
	tool    string
	hash    string
	expires time.Time
}

func argsHash(a mcpArgs) string {
	b, _ := json.Marshal(a) // map keys are sorted
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func toolText(v any, isErr bool) map[string]any {
	var text string
	if s, ok := v.(string); ok {
		text = s
	} else {
		b, _ := json.MarshalIndent(v, "", " ")
		text = string(b)
	}
	const max = 120_000
	if len(text) > max {
		text = text[:max] + "\n… (truncated: use search, limit or a narrower period)"
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
}

func (c *mcpCall) callTool(name string, args map[string]any) (any, *rpcError) {
	t, ok := mcpToolByName(name)
	if !ok {
		return nil, &rpcError{-32602, "unknown tool: " + name}
	}
	a := mcpArgs(args)
	if a == nil {
		a = mcpArgs{}
	}
	confirm := a.str("confirm")
	delete(a, "confirm")
	inputB, _ := json.Marshal(redact(map[string]any(a)))
	input := string(inputB)
	user := c.sess.User
	ip := clientIP(c.r)
	logc := func(result, status string) {
		c.s.logMCPCall(c.r.Context(), c.conn, user.ID, user.Username, t.Name, t.kind(), input, result, status, ip)
	}
	if !c.toolAllowed(t) {
		why := "this connection does not have scope " + t.Scope
		if slices.Contains(c.set.DisabledTools, t.Name) {
			why = "this tool is switched off by the administrator"
		} else if c.hasScope(t.Scope) {
			why = "the role of this connection may not use this tool"
		}
		logc(why, "denied")
		return toolText("Not allowed: "+why+".", true), nil
	}
	if !c.s.mcpRateOK(c.conn.ID) {
		logc("rate limited", "denied")
		return toolText(fmt.Sprintf("Too many calls: at most %d per minute. Wait a moment.", mcpRateLimit), true), nil
	}
	if t.kind() == "write" && c.set.ConfirmWrites {
		if confirm == "" {
			var preview any = a
			if t.Preview != nil {
				p, err := t.Preview(c, a)
				if err != nil {
					logc(err.Error(), "error")
					return toolText("Error: "+err.Error(), true), nil
				}
				preview = p
			}
			code := c.s.addProposal(c.conn.ID, t.Name, argsHash(a))
			logc("proposal "+code, "proposal")
			return toolText(map[string]any{
				"status":   "PROPOSAL - nothing has been saved yet",
				"change":   redact(preview),
				"confirm":  code,
				"expires":  "10 minutes",
				"nextStep": "Show the user this change and ask for an explicit yes. Then call " + t.Name + " again with exactly the same arguments plus \"confirm\": \"" + code + "\".",
			}, false), nil
		}
		if !c.s.takeProposal(c.conn.ID, t.Name, argsHash(a), confirm) {
			logc("invalid confirmation", "denied")
			return toolText("The confirmation code is unknown, expired or belongs to other arguments. Call the tool without confirm for a new proposal.", true), nil
		}
	}
	res, err := t.Run(c, a)
	if err != nil {
		logc(err.Error(), "error")
		return toolText("Error: "+err.Error(), true), nil
	}
	out := redact(res)
	if c.set.MaskNumbers && (t.Scope == "calls:read" || t.Scope == "media:read") {
		out = maskNumbers(out)
	}
	logc(summarise(out), "ok")
	return toolText(out, false), nil
}

func (s *Server) addProposal(connID int64, tool, hash string) string {
	code := newMCPSecret("p")[:9]
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	if s.mcp.proposals == nil {
		s.mcp.proposals = map[string]mcpProposal{}
	}
	now := time.Now()
	for k, p := range s.mcp.proposals {
		if now.After(p.expires) {
			delete(s.mcp.proposals, k)
		}
	}
	s.mcp.proposals[code] = mcpProposal{connID: connID, tool: tool, hash: hash, expires: now.Add(10 * time.Minute)}
	return code
}

func (s *Server) takeProposal(connID int64, tool, hash, code string) bool {
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	p, ok := s.mcp.proposals[code]
	if !ok || p.connID != connID || p.tool != tool || p.hash != hash || time.Now().After(p.expires) {
		return false
	}
	delete(s.mcp.proposals, code)
	return true
}

// summarise is a short result line for the activity log.
func summarise(v any) string {
	switch x := v.(type) {
	case []any:
		return fmt.Sprintf("%d items", len(x))
	case map[string]any:
		for _, k := range []string{"total", "items", "records"} {
			if n, ok := x[k]; ok {
				switch nn := n.(type) {
				case []any:
					return fmt.Sprintf("%d %s", len(nn), k)
				default:
					return fmt.Sprintf("%s %v", k, nn)
				}
			}
		}
		if ok, has := x["ok"]; has {
			return fmt.Sprintf("ok=%v", ok)
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 6 {
			keys = keys[:6]
		}
		return "{" + strings.Join(keys, ", ") + "}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ---- redaction and masking -----------------------------------------------------------------

const redacted = "[redacted]"

func secretKey(k string) bool {
	l := strings.ToLower(k)
	for _, p := range []string{"secret", "password", "passwd", "token", "apikey", "api_key", "privatekey", "pinhash", "totp", "passcode"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return l == "pin" || l == "hash" || l == "voicemailpin" || l == "sendpin"
}

// redact replaces secrets with "[redacted]" (or leaves them empty when they are empty).
func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if secretKey(k) {
				if s, ok := val.(string); ok && s == "" {
					out[k] = ""
				} else if _, isMap := val.(map[string]any); isMap {
					out[k] = redact(val)
				} else if _, isBool := val.(bool); isBool {
					out[k] = val
				} else {
					out[k] = redacted
				}
				continue
			}
			out[k] = redact(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = redact(val)
		}
		return out
	}
	return v
}

var phoneRe = regexp.MustCompile(`\+?\d[\d\- ]{6,}\d`)

func maskOne(s string) string {
	return phoneRe.ReplaceAllStringFunc(s, func(m string) string {
		digits := 0
		for _, r := range m {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits < 8 {
			return m
		}
		b := []rune(m)
		seen := 0
		for i, r := range b {
			if r >= '0' && r <= '9' {
				seen++
				if seen > 4 && seen <= digits-2 {
					b[i] = '•'
				}
			}
		}
		return string(b)
	})
}

// maskNumbers hides the middle digits of phone numbers of eight digits or more.
func maskNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = maskNumbers(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = maskNumbers(val)
		}
		return out
	case string:
		return maskOne(x)
	}
	return v
}

// ---- configuration kinds -------------------------------------------------------------------

type configKind struct {
	Key      string
	Path     string // list and create; item at Path/{id}
	Item     bool   // GET Path/{id} exists (otherwise the item is taken from the list)
	Writable bool
	MinRole  string
	What     string
}

var configKinds = []configKind{
	{"extensions", "/api/pbx/extensions", true, true, "admin", "extensions (users' phone numbers, voicemail, call features)"},
	{"ring_groups", "/api/pbx/ringgroups", false, true, "admin", "ring groups (ring several extensions at once)"},
	{"queues", "/api/pbx/queues", false, true, "admin", "call queues (members, strategy, timeouts, callback, SLA)"},
	{"ivrs", "/api/pbx/ivrs", false, true, "admin", "IVR menus (the visual call flow)"},
	{"time_conditions", "/api/pbx/timeconditions", false, true, "admin", "time conditions (opening hours, night mode)"},
	{"conferences", "/api/pbx/conferences", false, true, "admin", "conference rooms"},
	{"inbound_routes", "/api/pbx/inbound", false, true, "admin", "inbound routes (which number goes where)"},
	{"outbound_routes", "/api/pbx/outbound", false, true, "admin", "outbound routes (which trunk for which numbers)"},
	{"extension_groups", "/api/pbx/extension-groups", false, true, "admin", "extension groups (departments)"},
	{"fax_boxes", "/api/pbx/faxboxes", false, true, "admin", "fax boxes (fax to mail)"},
	{"oncall_teams", "/api/oncall/teams", false, true, "admin", "on-call teams"},
	{"trunks", "/api/pbx/trunks", false, false, "admin", "SIP trunks (read only via MCP)"},
	{"numbers", "/api/pbx/numbers", false, false, "admin", "phone numbers (DIDs) of the trunks"},
	{"prompts", "/api/pbx/prompts", false, false, "admin", "sound prompts"},
}

func configKindByKey(k string) (configKind, bool) {
	for _, ck := range configKinds {
		if ck.Key == k {
			return ck, true
		}
	}
	return configKind{}, false
}

func kindNames(writable bool) []string {
	out := []string{}
	for _, k := range configKinds {
		if !writable || k.Writable {
			out = append(out, k.Key)
		}
	}
	return out
}

func kindHelp(writable bool) string {
	parts := []string{}
	for _, k := range configKinds {
		if !writable || k.Writable {
			parts = append(parts, k.Key+": "+k.What)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *mcpCall) kind(a mcpArgs, writable bool) (configKind, error) {
	ck, ok := configKindByKey(a.str("kind"))
	if !ok || (writable && !ck.Writable) {
		return ck, argErr("kind", "one of "+strings.Join(kindNames(writable), ", "))
	}
	return ck, nil
}

func idOf(v any) int64 {
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	switch id := m["id"].(type) {
	case json.Number:
		n, _ := id.Int64()
		return n
	case float64:
		return int64(id)
	}
	return 0
}

// item fetches one object of a kind, unredacted, for merging.
func (c *mcpCall) item(ck configKind, id int64) (map[string]any, error) {
	if id <= 0 {
		return nil, argErr("id", "required")
	}
	if ck.Item {
		v, err := c.get(fmt.Sprintf("%s/%d", ck.Path, id), nil)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		return m, nil
	}
	v, err := c.get(ck.Path, nil)
	if err != nil {
		return nil, err
	}
	list, _ := v.([]any)
	for _, it := range list {
		if idOf(it) == id {
			m, _ := it.(map[string]any)
			return m, nil
		}
	}
	return nil, &mcpErr{Code: "not_found"}
}

func matches(v any, q string) bool {
	if q == "" {
		return true
	}
	b, _ := json.Marshal(redact(v))
	return strings.Contains(strings.ToLower(string(b)), strings.ToLower(q))
}

// merged applies changes to an object; a "[redacted]" value means "keep".
func merged(orig, changes map[string]any) (map[string]any, map[string]any) {
	out := make(map[string]any, len(orig))
	for k, v := range orig {
		out[k] = v
	}
	diff := map[string]any{}
	for k, v := range changes {
		if k == "id" {
			continue
		}
		if s, ok := v.(string); ok && s == redacted {
			continue
		}
		ob, _ := json.Marshal(orig[k])
		nb, _ := json.Marshal(v)
		if !bytes.Equal(ob, nb) {
			diff[k] = map[string]any{"from": orig[k], "to": v}
		}
		out[k] = v
	}
	return out, diff
}

func label(m map[string]any) string {
	parts := []string{}
	for _, k := range []string{"number", "name", "mac"} {
		if v, ok := m[k]; ok && v != nil && fmt.Sprint(v) != "" {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, " ")
}

// ---- helpers for tools ----------------------------------------------------------------------

func isRecep(c *mcpCall) bool { return auth.RoleRank(c.role()) >= auth.RoleRank("reception") }

func hasExtension(c *mcpCall) bool { return c.sess.User.ExtensionID != nil }

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

func limitList(v any, a mcpArgs, def int) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	q := a.str("search")
	n := int(a.int("limit"))
	if n <= 0 {
		n = def
	}
	out := []any{}
	total := 0
	for _, it := range list {
		if matches(it, q) {
			total++
			if len(out) < n {
				out = append(out, it)
			}
		}
	}
	return map[string]any{"total": total, "shown": len(out), "items": out}
}

func cdrQuery(a mcpArgs) url.Values {
	q := url.Values{}
	from, to := a.str("from"), a.str("to")
	if from == "" {
		from = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	q.Set("from", from)
	if to != "" {
		q.Set("to", to)
	}
	for _, k := range []string{"query", "direction", "disposition"} {
		if v := a.str(k); v != "" {
			key := k
			if k == "query" {
				key = "q"
			}
			q.Set(key, v)
		}
	}
	lim := a.int("limit")
	if lim <= 0 || lim > 500 {
		lim = 100
	}
	q.Set("limit", strconv.FormatInt(lim, 10))
	if off := a.int("offset"); off > 0 {
		q.Set("offset", strconv.FormatInt(off, 10))
	}
	return q
}

// ---- the catalogue ----------------------------------------------------------------------------

var mcpTools []*mcpTool

func init() {
	kindsRead := kindNames(false)
	kindsWrite := kindNames(true)
	mcpTools = []*mcpTool{
		// pbx:read
		{Name: "system_status", Title: "System status", Scope: "pbx:read", MinRole: "admin",
			Desc: "Version, update channel, Asterisk status, active channels and uptime of the phone system.",
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				info, err := c.get("/api/system/info", nil)
				if err != nil {
					return nil, err
				}
				st, err := c.get("/api/pbx/status", nil)
				if err != nil {
					return nil, err
				}
				return map[string]any{"system": info, "pbx": st}, nil
			}},
		{Name: "list_config", Title: "List configuration", Scope: "pbx:read", MinRole: "admin",
			Desc:     "List objects of one kind. Kinds: " + kindHelp(false) + ". Use search to filter on any text.",
			Props:    map[string]any{"kind": enum("What to list.", kindsRead...), "search": str("Optional text filter (name, number, …)."), "limit": integer("Maximum items (default 50).")},
			Required: []string{"kind"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, false)
				if err != nil {
					return nil, err
				}
				v, err := c.get(ck.Path, nil)
				if err != nil {
					return nil, err
				}
				return limitList(v, a, 50), nil
			}},
		{Name: "get_config", Title: "Get configuration object", Scope: "pbx:read", MinRole: "admin",
			Desc:     "Get one object of a kind by id, with all its fields (for example a queue with members or an IVR with its flow).",
			Props:    map[string]any{"kind": enum("Kind of object.", kindsRead...), "id": integer("Id of the object.")},
			Required: []string{"kind", "id"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, false)
				if err != nil {
					return nil, err
				}
				return c.item(ck, a.int("id"))
			}},
		{Name: "next_free_extension", Title: "Next free extension number", Scope: "pbx:read", MinRole: "admin",
			Desc: "The next free extension number in the number plan.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/pbx/extensions/next", nil) }},
		{Name: "diagnose_trunk", Title: "Diagnose trunk", Scope: "pbx:read", MinRole: "admin",
			Desc: "Registration state, last errors and reachability of a SIP trunk.", Props: map[string]any{"id": integer("Trunk id.")}, Required: []string{"id"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.get(fmt.Sprintf("/api/pbx/trunks/%d/diagnose", a.int("id")), nil)
			}},
		{Name: "list_holidays", Title: "Holidays", Scope: "pbx:read", MinRole: "admin",
			Desc: "Public holidays and closing days used by the time conditions.", Props: map[string]any{"year": integer("Year (default this year).")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				q := url.Values{}
				if y := a.int("year"); y > 0 {
					q.Set("year", strconv.FormatInt(y, 10))
				}
				return c.get("/api/pbx/holidays", q)
			}},
		{Name: "feature_codes", Title: "Feature codes", Scope: "pbx:read", MinRole: "admin",
			Desc: "Star codes users dial on their phone (voicemail, forwarding, pickup, …).",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/pbx/featurecodes", nil) }},

		// calls:read
		{Name: "search_calls", Title: "Search call history", Scope: "calls:read", MinRole: "user",
			Desc: "Search the call history (CDR). Reception and administrators see all calls, users only their own. Dates as YYYY-MM-DD; default the last 7 days.",
			Props: map[string]any{"from": str("First day, YYYY-MM-DD."), "to": str("Last day, YYYY-MM-DD."), "query": str("Number or name to look for."),
				"direction": enum("Direction.", "inbound", "outbound", "internal"), "disposition": enum("Outcome.", "ANSWERED", "NO ANSWER", "BUSY", "FAILED"),
				"limit": integer("Maximum records (default 100, max 500)."), "offset": integer("Skip this many records.")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				if isRecep(c) {
					return c.get("/api/cdr", cdrQuery(a))
				}
				return c.get("/api/portal/history", cdrQuery(a))
			}},
		{Name: "get_call_journey", Title: "Call journey", Scope: "calls:read", MinRole: "user",
			Desc:  "Step by step what happened to one call: IVR choices, queues, who rang, who answered, transfers, voicemail. Use the linkedId of a call from search_calls.",
			Props: map[string]any{"id": str("linkedId of the call.")}, Required: []string{"id"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				id := url.PathEscape(a.str("id"))
				if id == "" {
					return nil, argErr("id", "required")
				}
				if isRecep(c) {
					return c.get("/api/cdr/journey/"+id, nil)
				}
				return c.get("/api/portal/history/journey/"+id, nil)
			}},
		{Name: "queue_status", Title: "Queue status (live)", Scope: "calls:read", MinRole: "user",
			Desc: "Live state of the queues: waiting callers, agents logged in or paused. Users see the queues they are a member of.",
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				if isRecep(c) {
					return c.get("/api/reception/queues", nil)
				}
				return c.get("/api/portal/queues", nil)
			}},
		{Name: "wallboard_figures", Title: "Wallboard figures", Scope: "calls:read", MinRole: "reception",
			Desc: "Today's queue figures as on the wallboard: answered, abandoned, service level, waiting times, agents.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/reception/wallboard", nil) }},
		{Name: "call_quality", Title: "Call quality", Scope: "calls:read", MinRole: "admin",
			Desc:  "Call quality (MOS, jitter, packet loss) per trunk and extension, with the worst calls.",
			Props: map[string]any{"range": enum("Period.", "24h", "7d", "30d")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				q := url.Values{}
				if v := a.str("range"); v != "" {
					q.Set("range", v)
				}
				return c.get("/api/quality", q)
			}},
		{Name: "extension_states", Title: "Who is available", Scope: "calls:read", MinRole: "user",
			Desc: "Busy lamp state of extensions: idle, ringing, in a call, do not disturb.",
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				if isRecep(c) {
					return c.get("/api/reception/blf", nil)
				}
				return c.get("/api/portal/colleagues", nil)
			}},
		{Name: "list_callbacks", Title: "Callback requests", Scope: "calls:read", MinRole: "reception",
			Desc: "Callers that asked to be called back from a queue, and their state.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/reception/callbacks", nil) }},
		{Name: "my_phone", Title: "My phone", Scope: "calls:read", MinRole: "user", Needs: hasExtension,
			Desc: "The extension linked to this user: number, call forwarding, do not disturb, follow-me, voicemail.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/portal/me", nil) }},

		// phones:read
		{Name: "list_phones", Title: "Desk phones", Scope: "phones:read", MinRole: "admin",
			Desc:  "Provisioned desk phones: vendor, model, MAC, linked extension, firmware, last seen.",
			Props: map[string]any{"search": str("Optional text filter."), "limit": integer("Maximum items (default 100).")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				v, err := c.get("/api/phones", nil)
				if err != nil {
					return nil, err
				}
				return limitList(v, a, 100), nil
			}},
		{Name: "list_discovered_phones", Title: "Discovered phones", Scope: "phones:read", MinRole: "admin",
			Desc: "Phones that asked for provisioning but are not adopted yet.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/phones/discovered", nil) }},
		{Name: "firmware_status", Title: "Firmware status", Scope: "phones:read", MinRole: "admin",
			Desc: "Newest firmware per phone model and which phones are behind.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/firmware/latest", nil) }},

		// media:read
		{Name: "list_voicemail", Title: "My voicemail", Scope: "media:read", MinRole: "user", Needs: hasExtension,
			Desc: "Voicemail messages of the user's own extension, with transcript and AI summary when available.",
			Run:  func(c *mcpCall, a mcpArgs) (any, error) { return c.get("/api/portal/voicemail", nil) }},
		{Name: "list_recordings", Title: "Call recordings", Scope: "media:read", MinRole: "reception",
			Desc:  "Call recordings (metadata only).",
			Props: map[string]any{"query": str("Number or name to look for.")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				q := url.Values{}
				if v := a.str("query"); v != "" {
					q.Set("q", v)
				}
				return c.get("/api/recordings", q)
			}},
		{Name: "get_recording_summary", Title: "Recording transcript and summary", Scope: "media:read", MinRole: "reception",
			Desc:  "Transcript and AI summary of one call recording (when they were made).",
			Props: map[string]any{"name": str("File name of the recording.")}, Required: []string{"name"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.get("/api/recordings/"+url.PathEscape(a.str("name"))+"/ai", nil)
			}},

		// calls:act
		{Name: "click_to_call", Title: "Call a number", Scope: "calls:act", MinRole: "user",
			Desc:     "Ring a phone and, once it is answered, call the number. Without extension it rings the user's own phone; reception and administrators can name another extension number.",
			Props:    map[string]any{"number": str("Number to call."), "extension": str("Extension number that should make the call (reception/admin).")},
			Required: []string{"number"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				if a.str("number") == "" {
					return nil, argErr("number", "required")
				}
				if ext := a.str("extension"); ext != "" && isRecep(c) {
					return c.do(http.MethodPost, "/api/calls", nil, map[string]string{"extension": ext, "number": a.str("number")})
				}
				if !hasExtension(c) {
					return nil, &mcpErr{Code: "no_extension", Msg: "this user has no linked extension; give extension"}
				}
				return c.do(http.MethodPost, "/api/portal/call", nil, map[string]string{"number": a.str("number")})
			}},
		{Name: "set_call_forwarding", Title: "Forwarding and do not disturb", Scope: "calls:act", MinRole: "user", Needs: hasExtension,
			Desc: "Change call forwarding or do not disturb of the user's own phone. Give only what changes; an empty string switches a forward off.",
			Props: map[string]any{"dnd": boolean("Do not disturb."), "forwardAlways": str("Forward all calls to this number."),
				"forwardBusy": str("Forward when busy."), "forwardNoAnswer": str("Forward when not answered.")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				me, err := c.get("/api/portal/me", nil)
				if err != nil {
					return nil, err
				}
				m, _ := me.(map[string]any)
				f, _ := m["features"].(map[string]any)
				if f == nil {
					f = map[string]any{}
				}
				changes := map[string]any{}
				for _, k := range []string{"dnd", "forwardAlways", "forwardBusy", "forwardNoAnswer"} {
					if a.has(k) {
						changes[k] = a[k]
					}
				}
				if len(changes) == 0 {
					return nil, argErr("dnd", "give at least one of dnd, forwardAlways, forwardBusy, forwardNoAnswer")
				}
				nf, _ := merged(f, changes)
				return c.do(http.MethodPut, "/api/portal/features", nil, nf)
			}},
		{Name: "queue_agent", Title: "Log in or pause in a queue", Scope: "calls:act", MinRole: "user",
			Desc: "Log an agent in or out of a queue, or pause/unpause. Without extension_id it acts for the user's own extension; reception can name another extension id.",
			Props: map[string]any{"queue_id": integer("Queue id."), "action": enum("What to do.", "login", "logout", "pause", "unpause"),
				"extension_id": integer("Extension id of the agent (reception/admin).")},
			Required: []string{"queue_id", "action"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				act := a.str("action")
				if !slices.Contains([]string{"login", "logout", "pause", "unpause"}, act) {
					return nil, argErr("action", "login, logout, pause or unpause")
				}
				if ext := a.int("extension_id"); ext > 0 && isRecep(c) {
					return c.do(http.MethodPost, fmt.Sprintf("/api/reception/queues/%d/agents/%d", a.int("queue_id"), ext), nil, map[string]string{"action": act})
				}
				return c.do(http.MethodPost, fmt.Sprintf("/api/portal/queues/%d", a.int("queue_id")), nil, map[string]string{"action": act})
			}},
		{Name: "set_time_condition_mode", Title: "Night mode / override opening hours", Scope: "calls:act", MinRole: "reception",
			Desc:  "Override a time condition: open (day mode), closed (night mode) or auto (follow the schedule again).",
			Props: map[string]any{"id": integer("Time condition id."), "mode": enum("Mode.", "auto", "open", "closed")}, Required: []string{"id", "mode"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.do(http.MethodPut, fmt.Sprintf("/api/pbx/timeconditions/%d/mode", a.int("id")), nil, map[string]string{"mode": a.str("mode")})
			}},
		{Name: "cancel_callback", Title: "Cancel callback request", Scope: "calls:act", MinRole: "reception",
			Desc: "Cancel a pending callback request.", Props: map[string]any{"id": integer("Callback id.")}, Required: []string{"id"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.do(http.MethodPost, fmt.Sprintf("/api/reception/callbacks/%d/cancel", a.int("id")), nil, map[string]string{})
			}},

		// pbx:write
		{Name: "create_config", Title: "Create configuration object", Scope: "pbx:write", MinRole: "admin",
			Desc:     "Create an object of a kind (" + strings.Join(kindsWrite, ", ") + "). Look at an existing object with get_config for the fields; fields you leave out get their defaults.",
			Props:    map[string]any{"kind": enum("Kind of object.", kindsWrite...), "fields": map[string]any{"type": "object", "description": "The fields of the new object."}},
			Required: []string{"kind", "fields"},
			Preview: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				if a.obj("fields") == nil {
					return nil, argErr("fields", "required")
				}
				return map[string]any{"action": "create", "kind": ck.Key, "fields": a.obj("fields")}, nil
			},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				f := a.obj("fields")
				if f == nil {
					return nil, argErr("fields", "required")
				}
				delete(f, "id")
				return c.do(http.MethodPost, ck.Path, nil, f)
			}},
		{Name: "update_config", Title: "Change configuration object", Scope: "pbx:write", MinRole: "admin",
			Desc: "Change fields of an existing object. Give only the fields that change; a nested field (such as members or a flow) is replaced as a whole.",
			Props: map[string]any{"kind": enum("Kind of object.", kindsWrite...), "id": integer("Id of the object."),
				"changes": map[string]any{"type": "object", "description": "Fields with their new values."}},
			Required: []string{"kind", "id", "changes"},
			Preview: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				orig, err := c.item(ck, a.int("id"))
				if err != nil {
					return nil, err
				}
				_, diff := merged(orig, a.obj("changes"))
				if len(diff) == 0 {
					return nil, &mcpErr{Code: "no_changes", Msg: "the values are the same as now"}
				}
				return map[string]any{"action": "update", "kind": ck.Key, "id": a.int("id"), "object": label(orig), "changes": diff}, nil
			},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				orig, err := c.item(ck, a.int("id"))
				if err != nil {
					return nil, err
				}
				m, diff := merged(orig, a.obj("changes"))
				if len(diff) == 0 {
					return nil, &mcpErr{Code: "no_changes"}
				}
				return c.do(http.MethodPut, fmt.Sprintf("%s/%d", ck.Path, a.int("id")), nil, m)
			}},
		{Name: "delete_config", Title: "Delete configuration object", Scope: "pbx:write", MinRole: "admin", Destructive: true,
			Desc:     "Delete an object. Objects that are still used elsewhere are refused.",
			Props:    map[string]any{"kind": enum("Kind of object.", kindsWrite...), "id": integer("Id of the object.")},
			Required: []string{"kind", "id"},
			Preview: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				orig, err := c.item(ck, a.int("id"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"action": "delete", "kind": ck.Key, "id": a.int("id"), "object": label(orig)}, nil
			},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				ck, err := c.kind(a, true)
				if err != nil {
					return nil, err
				}
				return c.do(http.MethodDelete, fmt.Sprintf("%s/%d", ck.Path, a.int("id")), nil, nil)
			}},
		{Name: "assign_phone", Title: "Link a desk phone", Scope: "pbx:write", MinRole: "admin",
			Desc:     "Link a desk phone to an extension (or unlink it with extension_id 0) and push the new configuration to it.",
			Props:    map[string]any{"phone_id": integer("Phone id (see list_phones)."), "extension_id": integer("Extension id, 0 to unlink.")},
			Required: []string{"phone_id", "extension_id"},
			Preview: func(c *mcpCall, a mcpArgs) (any, error) {
				return map[string]any{"action": "assign phone", "phone": a.int("phone_id"), "extension": a.int("extension_id")}, nil
			},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				body := map[string]any{"resync": true}
				if e := a.int("extension_id"); e > 0 {
					body["extensionId"] = e
				} else {
					body["extensionId"] = nil
				}
				return c.do(http.MethodPut, fmt.Sprintf("/api/phones/%d/assign", a.int("phone_id")), nil, body)
			}},

		// system:write
		{Name: "list_backups", Title: "Backups", Scope: "system:write", MinRole: "admin", Kind: "read",
			Desc: "Backup targets (FTP, FTPS, SCP, OneDrive …), the schedule and the latest runs.",
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				targets, err := c.get("/api/backups/targets", nil)
				if err != nil {
					return nil, err
				}
				runs, _ := c.get("/api/backups/runs", nil)
				sched, _ := c.get("/api/backups/schedule", nil)
				return map[string]any{"targets": targets, "schedule": sched, "runs": runs}, nil
			}},
		{Name: "run_backup", Title: "Run a backup now", Scope: "system:write", MinRole: "admin", Kind: "act",
			Desc:     "Start a full or incremental backup to a backup target. Restoring is never possible via MCP.",
			Props:    map[string]any{"target_id": integer("Backup target id."), "kind": enum("Backup kind.", "full", "incremental")},
			Required: []string{"target_id", "kind"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.do(http.MethodPost, "/api/backups/run", nil, map[string]any{"targetId": a.int("target_id"), "kind": a.str("kind")})
			}},
		{Name: "check_updates", Title: "Check for updates", Scope: "system:write", MinRole: "admin", Kind: "read",
			Desc: "Check whether a newer Kiestoon version is available on the update channel. Installing is only possible in the portal.",
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.do(http.MethodPost, "/api/system/updates/check", nil, map[string]any{})
			}},
		{Name: "resync_phone", Title: "Push configuration to a phone", Scope: "system:write", MinRole: "admin", Kind: "act",
			Desc:  "Make a desk phone fetch its configuration again (it may restart).",
			Props: map[string]any{"phone_id": integer("Phone id.")}, Required: []string{"phone_id"},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				return c.do(http.MethodPost, fmt.Sprintf("/api/phones/%d/resync", a.int("phone_id")), nil, map[string]any{})
			}},
		{Name: "read_audit_log", Title: "Audit log", Scope: "system:write", MinRole: "admin", Kind: "read",
			Desc:  "The latest 200 entries of the audit log: who changed what, when and from where.",
			Props: map[string]any{"search": str("Optional text filter."), "limit": integer("Maximum entries (default 100).")},
			Run: func(c *mcpCall, a mcpArgs) (any, error) {
				v, err := c.get("/api/system/audit", nil)
				if err != nil {
					return nil, err
				}
				return limitList(v, a, 100), nil
			}},
	}
}
