package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

func (s *Server) integrationRoutes(mux *http.ServeMux, admin, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/tokens", admin(s.tokenList))
	mux.Handle("POST /api/tokens", admin(s.tokenCreate))
	mux.Handle("DELETE /api/tokens/{id}", admin(s.tokenDelete))

	mux.Handle("GET /api/webhooks", admin(s.webhookList))
	mux.Handle("POST /api/webhooks", admin(s.webhookSave))
	mux.Handle("PUT /api/webhooks/{id}", admin(s.webhookSave))
	mux.Handle("DELETE /api/webhooks/{id}", admin(s.webhookDelete))
	mux.Handle("POST /api/webhooks/{id}/test", admin(s.webhookTest))
	mux.Handle("GET /api/webhooks/{id}/deliveries", admin(s.webhookDeliveries))

	// Click-to-call for CRMs: ring an extension, then call the number.
	mux.Handle("POST /api/calls", recep(s.apiCall))
}

// ---- API tokens -------------------------------------------------------------

// TokenPrefix starts every API token, so secret scanners recognise them.
const TokenPrefix = "kt_"

type apiToken struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	ReadOnly   bool   `json:"readOnly"`
	ExpiresAt  string `json:"expiresAt"`
	LastUsedAt string `json:"lastUsedAt"`
	LastIP     string `json:"lastIp"`
	CreatedAt  string `json:"createdAt"`
}

func hashAPIToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// tokenSession turns "Authorization: Bearer kt_…" into a session with the role of the token.
func (s *Server) tokenSession(r *http.Request) (*auth.Session, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer "+TokenPrefix) {
		return nil, false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	var id, uid int64
	var role, expires string
	var ro int
	err := s.DB.QueryRowContext(r.Context(), `SELECT id, user_id, role, read_only, expires_at FROM api_tokens WHERE token_hash = ?`,
		hashAPIToken(tok)).Scan(&id, &uid, &role, &ro, &expires)
	if err != nil {
		return nil, true
	}
	if expires != "" {
		if t, err := time.Parse(time.RFC3339, expires); err == nil && time.Now().After(t) {
			return nil, true
		}
	}
	u, err := s.Auth.UserByID(r.Context(), uid)
	if err != nil || !u.Enabled {
		return nil, true
	}
	// The token never has more rights than its owner has now.
	if auth.RoleRank(role) > auth.RoleRank(u.Role) {
		role = u.Role
	}
	tu := *u
	tu.Role = role
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE api_tokens SET last_used_at = strftime('%Y-%m-%dT%H:%M:%SZ','now'), last_ip = ? WHERE id = ?`,
		clientIP(r), id)
	method := "token"
	if ro == 1 {
		method = "token-ro"
	}
	return &auth.Session{User: &tu, MFAPassed: true, Method: method}, true
}

// tokenAllowed keeps tokens away from the account, token and user management routes, and read-only
// tokens away from every change.
func tokenAllowed(r *http.Request, sess *auth.Session) bool {
	if !strings.HasPrefix(sess.Method, "token") {
		return true
	}
	p := r.URL.Path
	for _, pre := range []string{"/api/tokens", "/api/account", "/api/users", "/api/auth", "/api/system/updates", "/api/backups/restore", "/api/webhooks"} {
		if strings.HasPrefix(p, pre) {
			return false
		}
	}
	if sess.Method == "token-ro" && r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return true
}

func (s *Server) tokenList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT t.id, t.name, t.prefix, t.user_id, COALESCE(u.username, ''), t.role, t.read_only,
		t.expires_at, t.last_used_at, t.last_ip, t.created_at FROM api_tokens t LEFT JOIN users u ON u.id = t.user_id ORDER BY t.id`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	out := []apiToken{}
	for rows.Next() {
		var t apiToken
		var ro int
		if rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.UserID, &t.Username, &t.Role, &ro, &t.ExpiresAt, &t.LastUsedAt, &t.LastIP, &t.CreatedAt) == nil {
			t.ReadOnly = ro == 1
			out = append(out, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) tokenCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		ReadOnly    bool   `json:"readOnly"`
		ExpiresDays int    `json:"expiresDays"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 60 {
		fieldErr(w, "name")
		return
	}
	u := sessionFrom(r).User
	if !slices.Contains([]string{"admin", "reception", "user"}, req.Role) || auth.RoleRank(req.Role) > auth.RoleRank(u.Role) {
		fieldErr(w, "role")
		return
	}
	if req.ExpiresDays < 0 || req.ExpiresDays > 3650 {
		fieldErr(w, "expiresDays")
		return
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	tok := TokenPrefix + hex.EncodeToString(b)
	exp := ""
	if req.ExpiresDays > 0 {
		exp = time.Now().AddDate(0, 0, req.ExpiresDays).UTC().Format(time.RFC3339)
	}
	res, err := s.DB.ExecContext(r.Context(), `INSERT INTO api_tokens(name, token_hash, prefix, user_id, role, read_only, expires_at) VALUES (?,?,?,?,?,?,?)`,
		req.Name, hashAPIToken(tok), tok[:10], u.ID, req.Role, b2i(req.ReadOnly), exp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	id, _ := res.LastInsertId()
	s.audit(r, "token.created", req.Name+" "+req.Role)
	// The token itself is shown once and never again.
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": tok})
}

func (s *Server) tokenDelete(w http.ResponseWriter, r *http.Request) {
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM api_tokens WHERE id = ?`, pathID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "token.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- click-to-call ----------------------------------------------------------

func (s *Server) apiCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Extension string `json:"extension"`
		Number    string `json:"number"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	num := strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(req.Number)
	if !reDial.MatchString(num) {
		fieldErr(w, "number")
		return
	}
	e, err := s.PBX.ExtensionByNumber(r.Context(), strings.TrimSpace(req.Extension))
	if err != nil || !e.Enabled {
		fieldErr(w, "extension")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.AMI.Originate(ctx, e.Number, e.Name, num); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "call_failed", "message": err.Error()})
		return
	}
	s.audit(r, "call.originate", e.Number+" -> "+num)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// ---- webhooks ---------------------------------------------------------------

// WebhookEvents are the events a webhook can subscribe to.
var WebhookEvents = []string{"call.ringing", "call.answered", "call.ended", "voicemail.updated", "fax.received", "queue.abandoned"}

type webhook struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"createdAt"`
}

func (s *Server) webhooks(ctx context.Context) ([]webhook, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, url, events, secret, enabled, created_at FROM webhooks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []webhook{}
	for rows.Next() {
		var h webhook
		var ev string
		var en int
		if err := rows.Scan(&h.ID, &h.Name, &h.URL, &ev, &h.Secret, &en, &h.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ev), &h.Events)
		if h.Events == nil {
			h.Events = []string{}
		}
		h.Enabled = en == 1
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Server) webhookList(w http.ResponseWriter, r *http.Request) {
	hs, err := s.webhooks(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hs, "events": WebhookEvents})
}

func validWebhookURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(raw) > 500 {
		return false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		return false
	}
	return !strings.EqualFold(host, "localhost")
}

// blockedIP refuses this machine and cloud metadata addresses; LAN addresses (a CRM in the office)
// are allowed.
func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func (s *Server) webhookSave(w http.ResponseWriter, r *http.Request) {
	var h webhook
	if err := decode(r, &h); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	h.ID = pathID(r)
	h.Name, h.URL = strings.TrimSpace(h.Name), strings.TrimSpace(h.URL)
	if h.Name == "" || len(h.Name) > 60 {
		fieldErr(w, "name")
		return
	}
	if !validWebhookURL(h.URL) {
		fieldErr(w, "url")
		return
	}
	evs := []string{}
	for _, e := range h.Events {
		if !slices.Contains(WebhookEvents, e) {
			fieldErr(w, "events")
			return
		}
		if !slices.Contains(evs, e) {
			evs = append(evs, e)
		}
	}
	if len(evs) == 0 {
		fieldErr(w, "events")
		return
	}
	ev, _ := json.Marshal(evs)
	if h.ID == 0 {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		h.Secret = "whsec_" + hex.EncodeToString(b)
		res, err := s.DB.ExecContext(r.Context(), `INSERT INTO webhooks(name, url, events, secret, enabled) VALUES (?,?,?,?,?)`,
			h.Name, h.URL, string(ev), h.Secret, b2i(h.Enabled))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		h.ID, _ = res.LastInsertId()
	} else {
		res, err := s.DB.ExecContext(r.Context(), `UPDATE webhooks SET name=?, url=?, events=?, enabled=? WHERE id=?`,
			h.Name, h.URL, string(ev), b2i(h.Enabled), h.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
	}
	s.audit(r, "webhook.saved", h.Name)
	writeJSON(w, http.StatusOK, map[string]any{"id": h.ID})
}

func (s *Server) webhookDelete(w http.ResponseWriter, r *http.Request) {
	if _, err := s.DB.ExecContext(r.Context(), `DELETE FROM webhooks WHERE id = ?`, pathID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "webhook.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) webhookTest(w http.ResponseWriter, r *http.Request) {
	hs, _ := s.webhooks(r.Context())
	for _, h := range hs {
		if h.ID == pathID(r) {
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			status, err := s.deliver(ctx, h, "test", map[string]any{"message": "Test from Kiestoon"}, 1)
			resp := map[string]any{"status": status, "ok": err == nil}
			if err != nil {
				resp["error"] = err.Error()
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "not_found")
}

func (s *Server) webhookDeliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id, at, event, attempt, status, error, duration_ms FROM webhook_deliveries
		WHERE webhook_id = ? ORDER BY id DESC LIMIT 100`, pathID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	type delivery struct {
		ID         int64  `json:"id"`
		At         string `json:"at"`
		Event      string `json:"event"`
		Attempt    int    `json:"attempt"`
		Status     int    `json:"status"`
		Error      string `json:"error"`
		DurationMS int64  `json:"durationMs"`
	}
	out := []delivery{}
	for rows.Next() {
		var d delivery
		if rows.Scan(&d.ID, &d.At, &d.Event, &d.Attempt, &d.Status, &d.Error, &d.DurationMS) == nil {
			out = append(out, d)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// webhookHousekeeping keeps the last 200 deliveries per webhook.
func (s *Server) webhookHousekeeping(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE id IN (SELECT id FROM (SELECT id,
		ROW_NUMBER() OVER (PARTITION BY webhook_id ORDER BY id DESC) AS n FROM webhook_deliveries) WHERE n > 200)`)
}

type hookJob struct {
	hook    webhook
	event   string
	data    any
	attempt int
}

type webhookQueue struct {
	once sync.Once
	ch   chan hookJob
}

var hookClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
			host, _, _ := net.SplitHostPort(address)
			if ip := net.ParseIP(host); ip == nil || blockedIP(ip) {
				return errors.New("address not allowed")
			}
			return nil
		}}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConns:        10,
		IdleConnTimeout:     60 * time.Second,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// emit sends an event to every enabled webhook that subscribed to it.
func (s *Server) emit(event string, data any) {
	if s.hooks == nil {
		return
	}
	s.hooks.once.Do(func() {
		s.hooks.ch = make(chan hookJob, 1000)
		for i := 0; i < 2; i++ {
			go s.hookWorker()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hs, err := s.webhooks(ctx)
	if err != nil {
		return
	}
	for _, h := range hs {
		if h.Enabled && slices.Contains(h.Events, event) {
			select {
			case s.hooks.ch <- hookJob{hook: h, event: event, data: data, attempt: 1}:
			default:
				s.Log.Warn("webhook queue full, event dropped", "event", event)
			}
		}
	}
}

func (s *Server) hookWorker() {
	for j := range s.hooks.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, err := s.deliver(ctx, j.hook, j.event, j.data, j.attempt)
		cancel()
		if err != nil && j.attempt < 3 {
			next := j
			next.attempt++
			delay := map[int]time.Duration{2: 10 * time.Second, 3: time.Minute}[next.attempt]
			time.AfterFunc(delay, func() {
				select {
				case s.hooks.ch <- next:
				default:
				}
			})
		}
	}
}

// Sign returns the X-Kiestoon-Signature value: HMAC-SHA256 over "<timestamp>.<body>".
func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp + "."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (s *Server) deliver(ctx context.Context, h webhook, event string, data any, attempt int) (int, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{"id": id, "event": event, "timestamp": now.Format(time.RFC3339), "data": data})
	ts := strconv.FormatInt(now.Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Kiestoon-Webhook/"+version.Version)
	req.Header.Set("X-Kiestoon-Event", event)
	req.Header.Set("X-Kiestoon-Delivery", id)
	req.Header.Set("X-Kiestoon-Timestamp", ts)
	req.Header.Set("X-Kiestoon-Signature", Sign(h.Secret, ts, body))
	start := time.Now()
	resp, err := hookClient.Do(req)
	status := 0
	if err == nil {
		status = resp.StatusCode
		resp.Body.Close()
		if status < 200 || status > 299 {
			err = fmt.Errorf("HTTP %d", status)
		}
	}
	msg := ""
	if err != nil {
		msg = err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
	}
	_, _ = s.DB.ExecContext(context.Background(), `INSERT INTO webhook_deliveries(webhook_id, at, event, attempt, status, error, duration_ms) VALUES (?,?,?,?,?,?,?)`,
		h.ID, now.Format(time.RFC3339), event, attempt, status, msg, time.Since(start).Milliseconds())
	return status, err
}

// ---- call events ------------------------------------------------------------

// chanVar reads a channel variable that manager.conf adds to events (channelvars).
func chanVar(m asterisk.Message, prefix, name string) string {
	for _, kv := range m {
		if strings.EqualFold(kv.Key, prefix+"ChanVariable") {
			if k, v, ok := strings.Cut(kv.Value, "="); ok && k == name {
				return v
			}
		}
	}
	return ""
}

// endpointNumber turns "PJSIP/201-0000002a" (or the browser endpoint 201-web) into "201".
func endpointNumber(channel string) string {
	c := strings.TrimPrefix(channel, "PJSIP/")
	if i := strings.LastIndex(c, "-"); i > 0 {
		c = c[:i]
	}
	return strings.TrimSuffix(c, "-web")
}

// callEvent turns AMI events into webhook events.
func (s *Server) callEvent(m asterisk.Message) {
	switch m.Get("Event") {
	case "DialBegin", "DialEnd":
		ch, dest := m.Get("Channel"), m.Get("DestChannel")
		if !strings.HasPrefix(ch, "PJSIP/trunk-") || !strings.HasPrefix(dest, "PJSIP/") || strings.HasPrefix(dest, "PJSIP/trunk-") {
			return
		}
		data := map[string]any{"callId": m.Get("Linkedid"), "from": m.Get("CallerIDNum"), "fromName": m.Get("CallerIDName"),
			"did": chanVar(m, "", "KT_DID"), "extension": endpointNumber(dest), "direction": "inbound"}
		if m.Get("Event") == "DialBegin" {
			go s.emit("call.ringing", data)
		} else if m.Get("DialStatus") == "ANSWER" {
			go s.emit("call.answered", data)
		}
	case "MessageWaiting":
		mb := strings.TrimSuffix(m.Get("Mailbox"), "@default")
		nw, _ := strconv.Atoi(m.Get("New"))
		old, _ := strconv.Atoi(m.Get("Old"))
		go s.emit("voicemail.updated", map[string]any{"mailbox": mb, "new": nw, "old": old})
	case "QueueCallerAbandon":
		pos, _ := strconv.Atoi(m.Get("Position"))
		hold, _ := strconv.Atoi(m.Get("HoldTime"))
		go s.emit("queue.abandoned", map[string]any{"queue": m.Get("Queue"), "from": m.Get("CallerIDNum"), "fromName": m.Get("CallerIDName"),
			"position": pos, "holdTime": hold, "callId": m.Get("Linkedid")})
	}
}

// cdrEvent sends call.ended.
func (s *Server) cdrEvent(rec cdr.Record) {
	s.emit("call.ended", rec)
}
