package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// The MCP server lets AI assistants (Claude on claude.ai, Claude Desktop, Claude Code and other MCP
// clients) work with Kiestoon. A connection is either a token (kt_mcp_…) made in the portal or an
// OAuth sign-in from the client. Every connection belongs to a user, never gets more rights than
// that user has, and only reaches the tools of the scopes it was given. Tools run through the same
// API handlers as the portal, so validation, roles, Asterisk reloads and the audit log stay the same.

const (
	mcpSettingsKey   = "mcp_settings"
	mcpTokenPrefix   = "kt_mcp_"
	mcpAccessPrefix  = "kt_mcpa_"
	mcpRefreshPrefix = "kt_mcpr_"
	// mcpAccessTTL is how long an OAuth access token lives; the client refreshes it.
	mcpAccessTTL = time.Hour
	// mcpCallsKeep is how long the activity log keeps tool calls.
	mcpCallsKeep = 90 * 24 * time.Hour
)

// mcpSettings are the installation-wide switches of the MCP server.
type mcpSettings struct {
	Enabled bool `json:"enabled"`
	// DefaultReadOnly preselects only the read scopes for new connections.
	DefaultReadOnly bool `json:"defaultReadOnly"`
	// ConfirmWrites makes configuration changes a proposal first: they are saved only when the
	// client calls the tool again with the confirmation code, after the user said yes.
	ConfirmWrites bool `json:"confirmWrites"`
	// MaskNumbers hides the middle digits of phone numbers in call history and voicemail.
	MaskNumbers bool `json:"maskNumbers"`
	// ShareMedia allows the media:read scope (voicemail and recordings with transcripts).
	ShareMedia bool `json:"shareMedia"`
	// AnthropicOnly accepts OAuth connections only from Anthropic's published address ranges.
	AnthropicOnly bool `json:"anthropicOnly"`
	// AllowUsers lets users that are not administrators connect an assistant to their own phone.
	AllowUsers    bool     `json:"allowUsers"`
	DisabledTools []string `json:"disabledTools"`
}

func defaultMCPSettings() mcpSettings {
	return mcpSettings{DefaultReadOnly: true, ConfirmWrites: true, AllowUsers: true, DisabledTools: []string{}}
}

func (s *Server) mcpSettings(ctx context.Context) mcpSettings {
	set := defaultMCPSettings()
	_ = store.GetJSON(ctx, s.DB, mcpSettingsKey, &set)
	if set.DisabledTools == nil {
		set.DisabledTools = []string{}
	}
	return set
}

// anthropicRanges are the addresses Anthropic publishes for outgoing MCP connector traffic.
var anthropicRanges = []netip.Prefix{
	netip.MustParsePrefix("160.79.104.0/21"),
	netip.MustParsePrefix("2607:6bc0::/48"),
}

func fromAnthropic(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range anthropicRanges {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ---- scopes -------------------------------------------------------------------

// mcpScope groups tools. Kind is read, act (start or steer calls) or write (change configuration).
type mcpScope struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	MinRole string `json:"minRole"`
}

var mcpScopes = []mcpScope{
	{"pbx:read", "read", "admin"},
	{"calls:read", "read", "user"},
	{"phones:read", "read", "admin"},
	{"media:read", "read", "user"},
	{"calls:act", "act", "user"},
	{"pbx:write", "write", "admin"},
	{"system:write", "write", "admin"},
}

func mcpScopeByKey(k string) (mcpScope, bool) {
	for _, sc := range mcpScopes {
		if sc.Key == k {
			return sc, true
		}
	}
	return mcpScope{}, false
}

// cleanScopes keeps the known scopes the role may have, in catalogue order.
func cleanScopes(in []string, role string, set mcpSettings) []string {
	out := []string{}
	for _, sc := range mcpScopes {
		if !slices.Contains(in, sc.Key) || auth.RoleRank(sc.MinRole) > auth.RoleRank(role) {
			continue
		}
		if sc.Key == "media:read" && !set.ShareMedia {
			continue
		}
		out = append(out, sc.Key)
	}
	return out
}

// tokenRole is the highest role a token or connection may have for a user (superadmin counts as admin).
func tokenRole(u *auth.User) string {
	if u.IsAdmin() {
		return "admin"
	}
	return u.Role
}

// ---- connections ----------------------------------------------------------------

type mcpConn struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	ClientID     string   `json:"clientId"`
	UserID       int64    `json:"userId"`
	Username     string   `json:"username"`
	Role         string   `json:"role"`
	Scopes       []string `json:"scopes"`
	AllowedIPs   []string `json:"allowedIps"`
	Prefix       string   `json:"prefix"`
	ExpiresAt    string   `json:"expiresAt"`
	LastUsedAt   string   `json:"lastUsedAt"`
	LastIP       string   `json:"lastIp"`
	CreatedAt    string   `json:"createdAt"`
	CallsToday   int      `json:"callsToday"`
	accessExpiry string
}

const mcpConnCols = `c.id, c.name, c.kind, c.client_id, c.user_id, COALESCE(u.username, ''), c.role, c.scopes, c.allowed_ips, c.prefix,
	c.expires_at, c.last_used_at, c.last_ip, c.created_at, c.access_expires`

func scanMCPConn(sc interface{ Scan(...any) error }) (mcpConn, error) {
	var c mcpConn
	var scopes, ips string
	err := sc.Scan(&c.ID, &c.Name, &c.Kind, &c.ClientID, &c.UserID, &c.Username, &c.Role, &scopes, &ips, &c.Prefix,
		&c.ExpiresAt, &c.LastUsedAt, &c.LastIP, &c.CreatedAt, &c.accessExpiry)
	_ = json.Unmarshal([]byte(scopes), &c.Scopes)
	_ = json.Unmarshal([]byte(ips), &c.AllowedIPs)
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	if c.AllowedIPs == nil {
		c.AllowedIPs = []string{}
	}
	return c, err
}

func hashMCP(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func newMCPSecret(prefix string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func expired(at string) bool {
	if at == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, at)
	return err == nil && time.Now().After(t)
}

// ipAllowed checks an address against a list of addresses and networks; an empty list allows all.
func ipAllowed(list []string, ip string) bool {
	if len(list) == 0 {
		return true
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, n := range list {
		if p, err := netip.ParsePrefix(n); err == nil && p.Contains(a) {
			return true
		}
		if b, err := netip.ParseAddr(n); err == nil && b.Unmap() == a {
			return true
		}
	}
	return false
}

func cleanIPList(in []string) ([]string, bool) {
	out := []string{}
	for _, n := range in {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, err := netip.ParsePrefix(n); err != nil {
			if _, err := netip.ParseAddr(n); err != nil {
				return nil, false
			}
		}
		out = append(out, n)
	}
	return out, len(out) <= 20
}

// mcpAuthError says why a bearer token was refused; the protocol handler maps it to a status.
type mcpAuthError string

// mcpAuthenticate finds the connection of a bearer token and the session it acts as.
func (s *Server) mcpAuthenticate(r *http.Request, set mcpSettings) (*mcpConn, *auth.Session, mcpAuthError) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, nil, "missing_token"
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if !strings.HasPrefix(tok, mcpTokenPrefix) && !strings.HasPrefix(tok, mcpAccessPrefix) {
		return nil, nil, "invalid_token"
	}
	row := s.DB.QueryRowContext(r.Context(), `SELECT `+mcpConnCols+` FROM mcp_connections c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.token_hash = ?`, hashMCP(tok))
	c, err := scanMCPConn(row)
	if err != nil {
		return nil, nil, "invalid_token"
	}
	if expired(c.ExpiresAt) || (c.Kind == "oauth" && (c.accessExpiry == "" || expired(c.accessExpiry))) {
		return nil, nil, "invalid_token"
	}
	ip := clientIP(r)
	if !ipAllowed(c.AllowedIPs, ip) || (c.Kind == "oauth" && set.AnthropicOnly && !fromAnthropic(ip) && !isLocal(ip)) {
		return nil, nil, "ip_not_allowed"
	}
	u, err := s.Auth.UserByID(r.Context(), c.UserID)
	if err != nil || !u.Enabled {
		return nil, nil, "invalid_token"
	}
	if !u.IsAdmin() && !set.AllowUsers {
		return nil, nil, "users_not_allowed"
	}
	// Never more than the user has now, and media only while the installation shares it.
	if auth.RoleRank(c.Role) > auth.RoleRank(tokenRole(u)) {
		c.Role = tokenRole(u)
	}
	c.Scopes = cleanScopes(c.Scopes, c.Role, set)
	tu := *u
	tu.Role = c.Role
	method := "token-ro"
	for _, k := range c.Scopes {
		if sc, _ := mcpScopeByKey(k); sc.Kind != "read" {
			method = "token-mcp"
		}
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE mcp_connections SET last_used_at = strftime('%Y-%m-%dT%H:%M:%SZ','now'), last_ip = ? WHERE id = ?`, ip, c.ID)
	return &c, &auth.Session{User: &tu, MFAPassed: true, Method: method}, ""
}

// isLocal reports loopback and private addresses: a client on the own network is never "the cloud".
func isLocal(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && (a.Unmap().IsLoopback() || a.Unmap().IsPrivate())
}

// ---- in-process dispatch ------------------------------------------------------------

type mcpSessKey struct{}

// mcpDenied are API routes a tool may never reach, whatever the tool catalogue says.
var mcpDenied = []string{"/api/tokens", "/api/account", "/api/users", "/api/auth", "/api/mcp", "/api/setup",
	"/api/backups/restore", "/api/webhooks", "/api/system/updates/apply", "/api/system/channel", "/api/system/proxy",
	"/api/system/hostnames", "/api/ai/key", "/api/sso", "/api/internet", "/api/cluster"}

func mcpRouteAllowed(r *http.Request) bool {
	p := r.URL.Path
	for _, pre := range mcpDenied {
		if strings.HasPrefix(p, pre) {
			return false
		}
	}
	return !strings.Contains(p, "/admin-password") && !strings.HasSuffix(p, "/key") && !strings.HasSuffix(p, "/secret")
}

// apiHandler is the API mux, built once, for tools to call in-process.
func (s *Server) apiHandler() http.Handler {
	s.mcp.once.Do(func() { s.mcp.api = s.Routes() })
	return s.mcp.api
}

// mcpState is what the MCP server keeps in memory.
type mcpState struct {
	once sync.Once
	api  http.Handler
	mu   sync.Mutex
	// proposals are changes waiting for the user's yes, by confirmation code.
	proposals map[string]mcpProposal
	// codes are OAuth authorisation codes, by code hash.
	codes map[string]oauthCode
	// rate counts tool calls per connection per minute.
	rate      map[int64]*rateWindow
	lastPurge time.Time
}

type rateWindow struct {
	start time.Time
	n     int
}

// mcpRateLimit is the number of tool calls a connection may make per minute.
const mcpRateLimit = 120

func (s *Server) mcpRateOK(connID int64) bool {
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	if s.mcp.rate == nil {
		s.mcp.rate = map[int64]*rateWindow{}
	}
	w := s.mcp.rate[connID]
	now := time.Now()
	if w == nil || now.Sub(w.start) > time.Minute {
		s.mcp.rate[connID] = &rateWindow{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= mcpRateLimit
}

// ---- activity log ----------------------------------------------------------------

type mcpCallRow struct {
	ID       int64  `json:"id"`
	At       string `json:"at"`
	ConnID   int64  `json:"connId"`
	ConnName string `json:"connName"`
	Username string `json:"username"`
	Tool     string `json:"tool"`
	Kind     string `json:"kind"`
	Input    string `json:"input"`
	Result   string `json:"result"`
	Status   string `json:"status"`
	IP       string `json:"ip"`
}

func (s *Server) logMCPCall(ctx context.Context, c *mcpConn, userID int64, username, tool, kind, input, result, status, ip string) {
	var cid, uid any
	name := ""
	if c != nil {
		cid, name = c.ID, c.Name
	}
	if userID > 0 {
		uid = userID
	}
	if len(input) > 500 {
		input = input[:500] + "…"
	}
	if len(result) > 300 {
		result = result[:300] + "…"
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO mcp_calls(conn_id, conn_name, user_id, username, tool, kind, input, result, status, ip) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		cid, name, uid, username, tool, kind, input, result, status, ip)
	s.mcp.mu.Lock()
	purge := time.Since(s.mcp.lastPurge) > time.Hour
	if purge {
		s.mcp.lastPurge = time.Now()
	}
	s.mcp.mu.Unlock()
	if purge {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM mcp_calls WHERE at < ?`, time.Now().Add(-mcpCallsKeep).UTC().Format(time.RFC3339))
	}
}

// ---- portal API ----------------------------------------------------------------------

func (s *Server) mcpRoutes(mux *http.ServeMux, admin, full func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/mcp/overview", full(s.mcpOverview))
	mux.Handle("PUT /api/mcp/settings", admin(s.mcpSaveSettings))
	mux.Handle("GET /api/mcp/connections", full(s.mcpConnList))
	mux.Handle("POST /api/mcp/connections", full(s.mcpConnCreate))
	mux.Handle("DELETE /api/mcp/connections/{id}", full(s.mcpConnDelete))
	mux.Handle("GET /api/mcp/tools", full(s.mcpToolList))
	mux.Handle("GET /api/mcp/activity", full(s.mcpActivity))
	mux.Handle("GET /api/mcp/activity/export", admin(s.mcpActivityExport))
	mux.Handle("GET /api/mcp/oauth/request", full(s.oauthRequestInfo))
	mux.Handle("POST /api/mcp/oauth/decide", full(s.oauthDecide))
}

// mcpUserAllowed: administrators always, others when the installation lets users connect.
func (s *Server) mcpUserAllowed(r *http.Request) bool {
	u := sessionFrom(r).User
	return u.IsAdmin() || s.mcpSettings(r.Context()).AllowUsers
}

func (s *Server) mcpOverview(w http.ResponseWriter, r *http.Request) {
	set := s.mcpSettings(r.Context())
	u := sessionFrom(r).User
	out := map[string]any{
		"enabled":     set.Enabled,
		"allowUsers":  set.AllowUsers,
		"endpoint":    s.mcpEndpointBase(r) + "/mcp",
		"version":     mcpProtocolVersions[0],
		"internet":    strings.HasPrefix(s.mcpEndpointBase(r), "https://"),
		"scopes":      mcpScopesFor(tokenRole(u), set),
		"defaultRead": set.DefaultReadOnly,
		"maxRole":     tokenRole(u),
		"canConnect":  set.Enabled && (u.IsAdmin() || set.AllowUsers),
	}
	if u.IsAdmin() {
		out["settings"] = set
		var conns, oauth, calls, writes, denied int
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*), COALESCE(SUM(kind = 'oauth'), 0) FROM mcp_connections WHERE expires_at = '' OR expires_at > ?`,
			time.Now().UTC().Format(time.RFC3339)).Scan(&conns, &oauth)
		since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*), COALESCE(SUM(kind != 'read' AND status = 'ok'), 0), COALESCE(SUM(status = 'denied'), 0)
			FROM mcp_calls WHERE at >= ?`, since).Scan(&calls, &writes, &denied)
		out["stats"] = map[string]int{"connections": conns, "oauth": oauth, "tokens": conns - oauth, "calls": calls, "changes": writes, "denied": denied}
		out["anthropicRanges"] = []string{anthropicRanges[0].String(), anthropicRanges[1].String()}
	}
	writeJSON(w, http.StatusOK, out)
}

// mcpEndpointBase is the address clients use: the PBX address when it is set, otherwise this request.
func (s *Server) mcpEndpointBase(r *http.Request) string {
	return s.pbxBase(r)
}

func mcpScopesFor(role string, set mcpSettings) []map[string]any {
	out := []map[string]any{}
	for _, sc := range mcpScopes {
		if auth.RoleRank(sc.MinRole) > auth.RoleRank(role) {
			continue
		}
		tools := []string{}
		for _, t := range mcpTools {
			if t.Scope == sc.Key && auth.RoleRank(t.MinRole) <= auth.RoleRank(role) && !slices.Contains(set.DisabledTools, t.Name) {
				tools = append(tools, t.Name)
			}
		}
		out = append(out, map[string]any{"key": sc.Key, "kind": sc.Kind, "tools": tools,
			"available": sc.Key != "media:read" || set.ShareMedia})
	}
	return out
}

func (s *Server) mcpSaveSettings(w http.ResponseWriter, r *http.Request) {
	var set mcpSettings
	if err := decode(r, &set); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	clean := []string{}
	for _, n := range set.DisabledTools {
		if _, ok := mcpToolByName(n); ok && !slices.Contains(clean, n) {
			clean = append(clean, n)
		}
	}
	set.DisabledTools = clean
	if err := store.SetJSON(r.Context(), s.DB, mcpSettingsKey, set); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "mcp.settings", fmt.Sprintf("enabled=%t confirm=%t mask=%t media=%t anthropic=%t users=%t disabled=%d",
		set.Enabled, set.ConfirmWrites, set.MaskNumbers, set.ShareMedia, set.AnthropicOnly, set.AllowUsers, len(clean)))
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) mcpConnList(w http.ResponseWriter, r *http.Request) {
	u := sessionFrom(r).User
	q := `SELECT ` + mcpConnCols + ` FROM mcp_connections c LEFT JOIN users u ON u.id = c.user_id`
	args := []any{}
	// Administrators see every connection; others only their own (also with ?mine=1 for admins).
	if !u.IsAdmin() || r.URL.Query().Get("mine") == "1" {
		q += ` WHERE c.user_id = ?`
		args = append(args, u.ID)
	}
	rows, err := s.DB.QueryContext(r.Context(), q+` ORDER BY c.last_used_at DESC, c.id DESC`, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	out := []mcpConn{}
	for rows.Next() {
		if c, err := scanMCPConn(rows); err == nil {
			out = append(out, c)
		}
	}
	rows.Close()
	since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	for i := range out {
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM mcp_calls WHERE conn_id = ? AND at >= ?`, out[i].ID, since).Scan(&out[i].CallsToday)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) mcpConnCreate(w http.ResponseWriter, r *http.Request) {
	set := s.mcpSettings(r.Context())
	if !set.Enabled {
		writeErr(w, http.StatusConflict, "mcp_disabled")
		return
	}
	if !s.mcpUserAllowed(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Role        string   `json:"role"`
		Scopes      []string `json:"scopes"`
		ExpiresDays int      `json:"expiresDays"`
		AllowedIPs  []string `json:"allowedIps"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u := sessionFrom(r).User
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 60 {
		fieldErr(w, "name")
		return
	}
	if req.Role == "" {
		req.Role = tokenRole(u)
	}
	if !slices.Contains([]string{"admin", "reception", "user"}, req.Role) || auth.RoleRank(req.Role) > auth.RoleRank(tokenRole(u)) {
		fieldErr(w, "role")
		return
	}
	scopes := cleanScopes(req.Scopes, req.Role, set)
	if len(scopes) == 0 {
		fieldErr(w, "scopes")
		return
	}
	if req.ExpiresDays < 0 || req.ExpiresDays > 730 {
		fieldErr(w, "expiresDays")
		return
	}
	ips, ok := cleanIPList(req.AllowedIPs)
	if !ok {
		fieldErr(w, "allowedIps")
		return
	}
	tok := newMCPSecret(mcpTokenPrefix)
	exp := ""
	if req.ExpiresDays > 0 {
		exp = time.Now().AddDate(0, 0, req.ExpiresDays).UTC().Format(time.RFC3339)
	}
	sj, _ := json.Marshal(scopes)
	ij, _ := json.Marshal(ips)
	res, err := s.DB.ExecContext(r.Context(), `INSERT INTO mcp_connections(name, kind, user_id, role, scopes, allowed_ips, prefix, token_hash, expires_at)
		VALUES (?, 'token', ?, ?, ?, ?, ?, ?, ?)`, req.Name, u.ID, req.Role, string(sj), string(ij), tok[:len(mcpTokenPrefix)+4], hashMCP(tok), exp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	id, _ := res.LastInsertId()
	s.audit(r, "mcp.connection.created", req.Name+" "+req.Role+" "+strings.Join(scopes, ","))
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": tok, "scopes": scopes})
}

func (s *Server) mcpConnDelete(w http.ResponseWriter, r *http.Request) {
	u := sessionFrom(r).User
	q := `DELETE FROM mcp_connections WHERE id = ?`
	args := []any{pathID(r)}
	if !u.IsAdmin() {
		q += ` AND user_id = ?`
		args = append(args, u.ID)
	}
	res, err := s.DB.ExecContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "mcp.connection.revoked", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) mcpToolList(w http.ResponseWriter, r *http.Request) {
	set := s.mcpSettings(r.Context())
	type view struct {
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Kind     string `json:"kind"`
		MinRole  string `json:"minRole"`
		Disabled bool   `json:"disabled"`
		Desc     string `json:"description"`
	}
	out := []view{}
	for _, t := range mcpTools {
		out = append(out, view{t.Name, t.Scope, t.kind(), t.MinRole, slices.Contains(set.DisabledTools, t.Name), t.Desc})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) mcpActivityQuery(r *http.Request) ([]mcpCallRow, error) {
	u := sessionFrom(r).User
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []any{}
	if !u.IsAdmin() {
		where = append(where, "user_id = ?")
		args = append(args, u.ID)
	}
	switch q.Get("filter") {
	case "read":
		where = append(where, "kind = 'read'")
	case "change":
		where = append(where, "kind != 'read'")
	case "denied":
		where = append(where, "status IN ('denied','error')")
	}
	if c, _ := strconv.ParseInt(q.Get("conn"), 10, 64); c > 0 {
		where = append(where, "conn_id = ?")
		args = append(args, c)
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		where = append(where, "(tool LIKE ? OR conn_name LIKE ? OR username LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like, like)
	}
	if d := q.Get("since"); d != "" {
		where = append(where, "at >= ?")
		args = append(args, d)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id, at, COALESCE(conn_id, 0), conn_name, username, tool, kind, input, result, status, ip
		FROM mcp_calls WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mcpCallRow{}
	for rows.Next() {
		var c mcpCallRow
		if rows.Scan(&c.ID, &c.At, &c.ConnID, &c.ConnName, &c.Username, &c.Tool, &c.Kind, &c.Input, &c.Result, &c.Status, &c.IP) == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Server) mcpActivity(w http.ResponseWriter, r *http.Request) {
	out, err := s.mcpActivityQuery(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) mcpActivityExport(w http.ResponseWriter, r *http.Request) {
	out, err := s.mcpActivityQuery(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="kiestoon-mcp-%s.csv"`, time.Now().Format("20060102")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time", "connection", "user", "tool", "kind", "input", "result", "status", "ip"})
	for _, c := range out {
		_ = cw.Write([]string{c.At, c.ConnName, c.Username, c.Tool, c.Kind, c.Input, c.Result, c.Status, c.IP})
	}
	cw.Flush()
}
