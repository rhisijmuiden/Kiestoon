package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/crm"
	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// crmState is the live part of the CRM integration: calls waiting for their last CDR, calls that
// ring now (for the pop-up), notes typed before the call was logged, Exact sign-ins in progress
// and the pop-up subscribers.
type crmState struct {
	mu      sync.Mutex
	legs    map[string][]cdr.Record
	timers  map[string]*time.Timer
	ringing map[string]*crmRing
	notes   map[string]crmNote
	logins  map[string]crmLogin
	subs    map[*crmSub]bool
	// wake asks the worker to handle pending calls now.
	wake chan struct{}
}

type crmRing struct {
	LinkedID string
	Number   string
	DID      string
	Name     string
	Start    time.Time
	// Exts are the extensions it rang at (they got the pop-up); Answered the one that took it.
	Exts     map[string]bool
	Answered string
	Result   *crm.Result
}

type crmNote struct {
	Note     string
	Outcome  string
	TaskDays int
	At       time.Time
}

type crmLogin struct {
	user int64
	at   time.Time
}

// crmSub is one open pop-up stream: a portal tab, the Windows softphone or the switchboard.
type crmSub struct {
	user int64
	ext  string
	all  bool
	ch   chan crmEvent
}

// crmEvent is sent to the browser over the event stream.
type crmEvent struct {
	Type     string      `json:"type"` // ring, answered, stop, ended
	CallID   string      `json:"callId"`
	Number   string      `json:"number,omitempty"`
	Name     string      `json:"name,omitempty"`
	DID      string      `json:"did,omitempty"`
	Ext      string      `json:"ext,omitempty"`
	Result   *crm.Result `json:"result,omitempty"`
	At       string      `json:"at,omitempty"`
	TalkSec  int         `json:"talkSec,omitempty"`
	Loggable bool        `json:"loggable,omitempty"`
}

// crmDeskWait is how long a call from a trunk waits for the CRM name before the phones ring.
const crmDeskWait = 1500 * time.Millisecond

// crmRetry is the wait before the next attempt to write a call (then hourly, for 24 hours).
var crmRetry = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

// crmSettle is how long a call waits after its last CDR before it is logged (all legs are in).
var crmSettle = 6 * time.Second

func (s *Server) crmRoutes(mux *http.ServeMux, admin, recep, user func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/crm", admin(s.crmGet))
	mux.Handle("PUT /api/crm", admin(s.crmSave))
	mux.Handle("PUT /api/crm/{kind}/secret", admin(s.crmSetSecret))
	mux.Handle("DELETE /api/crm/{kind}/secret", admin(s.crmDisconnect))
	mux.Handle("POST /api/crm/{kind}/test", admin(s.crmTest))
	mux.Handle("GET /api/crm/{kind}/owners", admin(s.crmOwners))
	mux.Handle("POST /api/crm/exact/login", admin(s.crmExactLogin))
	// Exact sends the browser back here: the strict session cookie does not come along on that
	// cross-site navigation, so the one-time state proves which admin started the sign-in.
	mux.HandleFunc("GET /api/crm/exact/callback", s.crmExactCallback)
	mux.Handle("POST /api/crm/lookup", admin(s.crmTestLookup))
	mux.Handle("GET /api/crm/log", admin(s.crmLog))
	mux.Handle("POST /api/crm/calls/{id}/retry", admin(s.crmRetryCall))
	mux.Handle("GET /api/crm/calls", recep(s.crmCallsFor))

	mux.Handle("GET /api/portal/crm/status", user(s.crmPortalStatus))
	mux.Handle("GET /api/portal/crm/events", user(s.crmEvents))
	mux.Handle("GET /api/portal/crm/lookup", user(s.crmPortalLookup))
	mux.Handle("POST /api/portal/crm/note", user(s.crmNote))
	mux.Handle("POST /api/portal/crm/contact", user(s.crmCreateContact))
}

// ---- settings and secrets ---------------------------------------------------

func (s *Server) crmLoad(ctx context.Context) (crm.Settings, map[string]crm.Secret) {
	st := crm.Defaults()
	_ = store.GetJSON(ctx, s.DB, crm.SettingsKey, &st)
	st.Normalize()
	secrets := map[string]crm.Secret{}
	for _, k := range crm.Kinds {
		var sec crm.Secret
		_ = store.GetJSON(ctx, s.DB, crm.SecretKey(k), &sec)
		secrets[k] = sec
	}
	return st, secrets
}

// crmReload hands the stored settings to the CRM service.
func (s *Server) crmReload(ctx context.Context) {
	st, secrets := s.crmLoad(ctx)
	s.crm.Reload(st, secrets)
}

func (s *Server) startCRM() {
	s.crm = crm.NewService()
	s.crmLive = &crmState{legs: map[string][]cdr.Record{}, timers: map[string]*time.Timer{}, ringing: map[string]*crmRing{},
		notes: map[string]crmNote{}, logins: map[string]crmLogin{}, subs: map[*crmSub]bool{}, wake: make(chan struct{}, 1)}
	s.crm.SaveSecret = func(kind string, sec crm.Secret) {
		_ = store.SetJSON(context.Background(), s.DB, crm.SecretKey(kind), sec)
		if sec.LoginExpired {
			s.Log.Warn("crm sign-in refused, an admin must sign in again", "crm", kind)
		}
	}
	s.crm.Log = func(kind, action, detail string, took time.Duration, err error) {
		s.crmLogRow(context.Background(), kind, action, detail, took, err, 0)
	}
	s.crmReload(context.Background())
}

func (s *Server) crmLogRow(ctx context.Context, kind, action, detail string, took time.Duration, err error, callID int64) {
	msg := ""
	if err != nil {
		msg = crmErrText(err)
	}
	var cid any
	if callID > 0 {
		cid = callID
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO crm_log(at, crm, action, detail, ok, error, duration_ms, call_id) VALUES (?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), kind, action, detail, b2i(err == nil), msg, took.Milliseconds(), cid)
}

func crmErrText(err error) string {
	switch {
	case errors.Is(err, crm.ErrAuth):
		return "sign_in_expired"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	m := err.Error()
	if len(m) > 300 {
		m = m[:300]
	}
	return m
}

type crmStatus struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	LoginExpired bool   `json:"loginExpired"`
	Account      string `json:"account"`
	// HasSecret tells the form a secret is stored (the secret itself never leaves the server).
	HasSecret bool `json:"hasSecret"`
	Lookups   int  `json:"lookups"`
	Hits      int  `json:"hits"`
	Logged    int  `json:"logged"`
	Failed    int  `json:"failed"`
}

func (s *Server) crmStatuses(ctx context.Context, st crm.Settings, secrets map[string]crm.Secret) []crmStatus {
	since := time.Now().In(s.loc(ctx))
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location()).UTC()
	out := []crmStatus{}
	for _, k := range crm.Kinds {
		sec := secrets[k]
		cs := crmStatus{Kind: k, Name: crm.Names[k], Configured: sec.Configured(k), LoginExpired: sec.LoginExpired, Account: sec.Account,
			HasSecret: sec.Token != "" || sec.ClientSecret != ""}
		cs.Connected = st.Connectors[k].Enabled && cs.Configured && !sec.LoginExpired
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN ok = 1 AND detail LIKE '%→%' THEN 1 ELSE 0 END), 0)
			FROM crm_log WHERE crm = ? AND action = 'lookup' AND at >= ?`, k, since.Format(time.RFC3339)).Scan(&cs.Lookups, &cs.Hits)
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN state = 'logged' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state IN ('failed','retry') THEN 1 ELSE 0 END), 0) FROM crm_calls WHERE crm = ? AND start_at >= ?`,
			k, since.Format(time.RFC3339)).Scan(&cs.Logged, &cs.Failed)
		out = append(out, cs)
	}
	return out
}

func (s *Server) crmExactRedirect(r *http.Request) string {
	return s.pbxBase(r) + "/api/crm/exact/callback"
}

func (s *Server) crmGet(w http.ResponseWriter, r *http.Request) {
	st, secrets := s.crmLoad(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "status": s.crmStatuses(r.Context(), st, secrets),
		"exactRedirect": s.crmExactRedirect(r), "aiCalls": s.aiSettings(r.Context()).Calls})
}

func (s *Server) crmSave(w http.ResponseWriter, r *http.Request) {
	var st crm.Settings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	old, _ := s.crmLoad(r.Context())
	if err := store.SetJSON(r.Context(), s.DB, crm.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	// A changed Exact app or Salesforce domain needs a new sign-in.
	for _, k := range []string{crm.Exact, crm.Salesforce} {
		o, n := old.Connectors[k], st.Connectors[k]
		if o.ClientID != n.ClientID || o.Domain != n.Domain || o.Region != n.Region {
			var sec crm.Secret
			_ = store.GetJSON(r.Context(), s.DB, crm.SecretKey(k), &sec)
			sec.AccessToken, sec.RefreshToken, sec.Expiry, sec.Account = "", "", time.Time{}, ""
			_ = store.SetJSON(r.Context(), s.DB, crm.SecretKey(k), sec)
		}
	}
	s.crmSetDeskName(r.Context(), st)
	s.audit(r, "crm.settings", "")
	s.crmReload(r.Context())
	s.crmGet(w, r)
}

// crmSetDeskName tells the dialplan whether calls from trunks ask for the CRM name.
func (s *Server) crmSetDeskName(ctx context.Context, st crm.Settings) {
	on := st.DeskName && slices.ContainsFunc(crm.Kinds, func(k string) bool { return st.Connectors[k].Enabled })
	v := ""
	if on {
		v = "1"
	}
	if cur, _ := store.GetSetting(ctx, s.DB, crm.DeskNameKey, ""); cur != v {
		_ = store.SetSetting(ctx, s.DB, crm.DeskNameKey, v)
		s.Ast.Changed()
	}
}

func crmKind(w http.ResponseWriter, r *http.Request) (string, bool) {
	k := r.PathValue("kind")
	if crm.Names[k] == "" {
		writeErr(w, http.StatusNotFound, "not_found")
		return "", false
	}
	return k, true
}

// crmSetSecret stores a token or client secret and, where that is enough, checks it at once.
func (s *Server) crmSetSecret(w http.ResponseWriter, r *http.Request) {
	kind, ok := crmKind(w, r)
	if !ok {
		return
	}
	var req struct {
		Token        string `json:"token"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Token, req.ClientSecret = strings.TrimSpace(req.Token), strings.TrimSpace(req.ClientSecret)
	var sec crm.Secret
	switch kind {
	case crm.HubSpot, crm.Pipedrive:
		if req.Token == "" || len(req.Token) > 500 || strings.ContainsAny(req.Token, " \r\n") {
			fieldErr(w, "token")
			return
		}
		sec.Token = req.Token
	default:
		if req.ClientSecret == "" || len(req.ClientSecret) > 500 || strings.ContainsAny(req.ClientSecret, " \r\n") {
			fieldErr(w, "clientSecret")
			return
		}
		sec.ClientSecret = req.ClientSecret
	}
	st, _ := s.crmLoad(r.Context())
	cfg := st.Connectors[kind]
	msg := ""
	if kind != crm.Exact {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		c, err := crm.New(kind, cfg, sec, func(n crm.Secret) { sec = n })
		if err == nil {
			t := time.Now()
			sec.Account, err = c.Test(ctx)
			s.crmLogRow(r.Context(), kind, "connect", "", time.Since(t), err, 0)
		}
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "crm_failed", "field": crmSecretField(kind), "message": crmErrText(err)})
			return
		}
		msg = sec.Account
	}
	if err := store.SetJSON(r.Context(), s.DB, crm.SecretKey(kind), sec); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "crm.connected", kind+" "+msg)
	s.crmReload(r.Context())
	s.crmGet(w, r)
}

func crmSecretField(kind string) string {
	if kind == crm.HubSpot || kind == crm.Pipedrive {
		return "token"
	}
	return "clientSecret"
}

func (s *Server) crmDisconnect(w http.ResponseWriter, r *http.Request) {
	kind, ok := crmKind(w, r)
	if !ok {
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `DELETE FROM settings WHERE key = ?`, crm.SecretKey(kind)); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "crm.disconnected", kind)
	s.crmReload(r.Context())
	s.crmGet(w, r)
}

func (s *Server) crmTest(w http.ResponseWriter, r *http.Request) {
	kind, ok := crmKind(w, r)
	if !ok {
		return
	}
	st, secrets := s.crmLoad(r.Context())
	sec := secrets[kind]
	if !sec.Configured(kind) {
		writeErr(w, http.StatusConflict, "crm_not_connected")
		return
	}
	c, err := crm.New(kind, st.Connectors[kind], sec, func(n crm.Secret) { sec = n })
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	t := time.Now()
	account, err := c.Test(ctx)
	s.crmLogRow(r.Context(), kind, "test", "", time.Since(t), err, 0)
	if err != nil {
		if errors.Is(err, crm.ErrAuth) {
			sec.LoginExpired = true
			_ = store.SetJSON(r.Context(), s.DB, crm.SecretKey(kind), sec)
			s.crmReload(r.Context())
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": crmErrText(err)})
		return
	}
	sec.Account, sec.LoginExpired = account, false
	_ = store.SetJSON(r.Context(), s.DB, crm.SecretKey(kind), sec)
	s.crmReload(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "account": account})
}

// crmOwners lists the CRM owners and, per Kiestoon user, the owner calls are logged under.
func (s *Server) crmOwners(w http.ResponseWriter, r *http.Request) {
	kind, ok := crmKind(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	owners, err := s.crm.Owners(ctx, kind)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"owners": []crm.Owner{}, "users": []any{}, "error": crmErrText(err)})
		return
	}
	cfg := s.crm.Settings().Connectors[kind]
	type row struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Ext     string `json:"ext"`
		OwnerID string `json:"ownerId"`
		Match   string `json:"match"` // manual, email, none
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT u.id, COALESCE(NULLIF(u.display_name, ''), u.username), u.email, COALESCE(e.number, '')
		FROM users u LEFT JOIN extensions e ON e.id = u.extension_id WHERE u.enabled = 1 ORDER BY 2`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	users := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Name, &x.Email, &x.Ext) != nil {
			continue
		}
		x.Match = "none"
		if m := cfg.Owners[strconv.FormatInt(x.ID, 10)]; m != "" {
			x.OwnerID, x.Match = m, "manual"
		} else if x.Email != "" {
			for _, o := range owners {
				if strings.EqualFold(o.Email, x.Email) {
					x.OwnerID, x.Match = o.ID, "email"
					break
				}
			}
		}
		users = append(users, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"owners": owners, "users": users})
}

// ---- Exact Online sign-in ---------------------------------------------------

func (s *Server) crmExactLogin(w http.ResponseWriter, r *http.Request) {
	st, secrets := s.crmLoad(r.Context())
	cfg := st.Connectors[crm.Exact]
	if cfg.ClientID == "" {
		fieldErr(w, "exact.clientId")
		return
	}
	if secrets[crm.Exact].ClientSecret == "" {
		fieldErr(w, "clientSecret")
		return
	}
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	s.crmLive.mu.Lock()
	for k, l := range s.crmLive.logins {
		if time.Since(l.at) > 15*time.Minute {
			delete(s.crmLive.logins, k)
		}
	}
	s.crmLive.logins[state] = crmLogin{user: sessionFrom(r).User.ID, at: time.Now()}
	s.crmLive.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"url": crm.ExactAuthURL(cfg, s.crmExactRedirect(r), state)})
}

func (s *Server) crmExactCallback(w http.ResponseWriter, r *http.Request) {
	back := func(result string) {
		http.Redirect(w, r, "/integrations?tab=crm&exact="+url.QueryEscape(result), http.StatusFound)
	}
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	s.crmLive.mu.Lock()
	l, ok := s.crmLive.logins[state]
	delete(s.crmLive.logins, state)
	s.crmLive.mu.Unlock()
	if !ok || state == "" || time.Since(l.at) > 15*time.Minute {
		back("expired")
		return
	}
	if code == "" {
		back("denied")
		return
	}
	st, secrets := s.crmLoad(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	t := time.Now()
	sec, err := crm.ExactExchange(ctx, st.Connectors[crm.Exact], secrets[crm.Exact], code, s.crmExactRedirect(r))
	if err == nil {
		var c crm.Connector
		c, err = crm.New(crm.Exact, st.Connectors[crm.Exact], sec, func(n crm.Secret) { sec = n })
		if err == nil {
			sec.Account, err = c.Test(ctx)
		}
	}
	s.crmLogRow(r.Context(), crm.Exact, "connect", "", time.Since(t), err, 0)
	if err != nil {
		s.Log.Warn("exact sign-in", "err", err)
		back("failed")
		return
	}
	_ = store.SetJSON(r.Context(), s.DB, crm.SecretKey(crm.Exact), sec)
	store.Audit(r.Context(), s.DB, l.user, "crm.connected", "exact "+sec.Account, clientIP(r))
	s.crmReload(r.Context())
	back("ok")
}

// ---- look-ups ---------------------------------------------------------------

func (s *Server) crmNumber(ctx context.Context, raw string) (crm.Number, bool) {
	st, _ := pbx.LoadSettings(ctx, s.DB)
	return crm.ParseNumber(raw, st.CountryCode)
}

func (s *Server) crmTestLookup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Number string `json:"number"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	n, ok := s.crmNumber(r.Context(), req.Number)
	if !ok {
		fieldErr(w, "number")
		return
	}
	s.crm.Forget(n)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	res, err := s.crm.Lookup(ctx, n)
	if err != nil && !res.Found() {
		writeJSON(w, http.StatusOK, map[string]any{"result": res, "error": crmErrText(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

func (s *Server) crmPortalStatus(w http.ResponseWriter, r *http.Request) {
	st := s.crm.Settings()
	var crms []string
	for _, k := range st.Order {
		if c, _ := s.crm.Connector(k); c != nil {
			crms = append(crms, k)
		}
	}
	create := ""
	for _, k := range crms {
		create = k
		break
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": s.crm.Active(), "crms": crms, "createIn": create})
}

func (s *Server) crmPortalLookup(w http.ResponseWriter, r *http.Request) {
	n, ok := s.crmNumber(r.Context(), r.URL.Query().Get("number"))
	if !ok || !s.crm.Active() {
		writeJSON(w, http.StatusOK, map[string]any{"result": nil})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	res, _ := s.crm.Lookup(ctx, n)
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

// crmDeskName answers the dialplan (FastAGI) with the CRM name of a caller from a trunk.
func (s *Server) crmDeskName(ctx context.Context, number string) string {
	n, ok := s.crmNumber(ctx, number)
	if !ok {
		return ""
	}
	return agiSafe(s.crm.DeskName(n, crmDeskWait))
}

// agiSafe makes a name safe for SET VARIABLE and a phone display.
func agiSafe(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 32 {
			return -1
		}
		return r
	}, v)
	if r := []rune(v); len(r) > 60 {
		v = string(r[:60])
	}
	return strings.TrimSpace(v)
}

// ---- live pop-up --------------------------------------------------------------

// crmAMI follows calls from trunks for the pop-up: who it rings, who answers, when it ends.
func (s *Server) crmAMI(m asterisk.Message) {
	if s.crmLive == nil || !s.crm.Active() {
		return
	}
	lid := m.Get("Linkedid")
	switch m.Get("Event") {
	case "Newchannel":
		// The first channel of a call from outside is the trunk channel itself.
		if !strings.HasPrefix(m.Get("Channel"), "PJSIP/trunk-") || m.Get("Uniqueid") != lid {
			return
		}
		ring := &crmRing{LinkedID: lid, Number: m.Get("CallerIDNum"), Name: m.Get("CallerIDName"), Start: time.Now(), Exts: map[string]bool{}}
		s.crmLive.mu.Lock()
		s.crmLive.ringing[lid] = ring
		s.crmLive.mu.Unlock()
		// Start the look-up now: it is usually done before the first phone rings.
		go func() {
			if n, ok := s.crmNumber(context.Background(), ring.Number); ok {
				_, _ = s.crm.Lookup(context.Background(), n)
			}
		}()
	case "DialBegin":
		ext := crm.EndpointExt(m.Get("DestChannel"))
		if ext == "" {
			return
		}
		s.crmLive.mu.Lock()
		ring := s.crmLive.ringing[lid]
		if ring == nil || ring.Exts[ext] || ring.Answered != "" {
			s.crmLive.mu.Unlock()
			return
		}
		ring.Exts[ext] = true
		if ring.DID == "" {
			ring.DID = chanVar(m, "", "KT_DID")
		}
		first := len(ring.Exts) == 1
		s.crmLive.mu.Unlock()
		if s.crm.Settings().PopupAll {
			go s.crmRingEvent(ring, ext, first)
		}
	case "DialEnd":
		ext := crm.EndpointExt(m.Get("DestChannel"))
		s.crmLive.mu.Lock()
		ring := s.crmLive.ringing[lid]
		if ring == nil || ext == "" {
			s.crmLive.mu.Unlock()
			return
		}
		answered := m.Get("DialStatus") == "ANSWER" && ring.Answered == ""
		if answered {
			ring.Answered = ext
		}
		s.crmLive.mu.Unlock()
		if answered {
			if !s.crm.Settings().PopupAll {
				s.crmRingEvent(ring, ext, true)
			}
			s.crmSend(crmEvent{Type: "answered", CallID: lid, Ext: ext}, ring.Exts, true)
		} else if ring.Answered == "" {
			s.crmSend(crmEvent{Type: "stop", CallID: lid, Ext: ext}, map[string]bool{ext: true}, false)
		}
	case "Hangup":
		if m.Get("Uniqueid") != lid {
			return
		}
		s.crmLive.mu.Lock()
		ring := s.crmLive.ringing[lid]
		delete(s.crmLive.ringing, lid)
		s.crmLive.mu.Unlock()
		if ring == nil {
			return
		}
		ev := crmEvent{Type: "ended", CallID: lid, Ext: ring.Answered, TalkSec: int(time.Since(ring.Start).Seconds()), Loggable: true}
		targets := ring.Exts
		if ring.Answered != "" {
			targets = map[string]bool{ring.Answered: true}
		}
		s.crmSend(ev, targets, true)
	}
}

func (s *Server) crmRingEvent(ring *crmRing, ext string, toAll bool) {
	var res *crm.Result
	if n, ok := s.crmNumber(context.Background(), ring.Number); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		res, _ = s.crm.Lookup(ctx, n)
		cancel()
	}
	s.crmLive.mu.Lock()
	ring.Result = res
	done := ring.Answered != "" && ring.Answered != ext
	s.crmLive.mu.Unlock()
	if done {
		return
	}
	ev := crmEvent{Type: "ring", CallID: ring.LinkedID, Number: ring.Number, Name: ring.Name, DID: ring.DID, Ext: ext, Result: res,
		At: ring.Start.UTC().Format(time.RFC3339)}
	s.crmSend(ev, map[string]bool{ext: true}, toAll)
}

// crmSend delivers an event to the streams of these extensions, and to the switchboards when all.
func (s *Server) crmSend(ev crmEvent, exts map[string]bool, all bool) {
	s.crmLive.mu.Lock()
	defer s.crmLive.mu.Unlock()
	for sub := range s.crmLive.subs {
		if exts[sub.ext] && !sub.all || sub.all && all {
			select {
			case sub.ch <- ev:
			default:
			}
		}
	}
}

// crmEvents is the event stream of the pop-up (server-sent events).
func (s *Server) crmEvents(w http.ResponseWriter, r *http.Request) {
	u := sessionFrom(r).User
	sub := &crmSub{user: u.ID, ch: make(chan crmEvent, 32)}
	if r.URL.Query().Get("scope") == "all" {
		if !isReceptionRole(u.Role) {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		sub.all = true
	} else {
		if u.ExtensionID == nil {
			writeErr(w, http.StatusConflict, "no_extension")
			return
		}
		e, err := s.PBX.Extension(r.Context(), *u.ExtensionID)
		if err != nil {
			writeErr(w, http.StatusConflict, "no_extension")
			return
		}
		sub.ext = e.Number
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	_ = rc.Flush()

	s.crmLive.mu.Lock()
	s.crmLive.subs[sub] = true
	// A tab opened while the phone already rings still gets the pop-up.
	var now []crmEvent
	for _, ring := range s.crmLive.ringing {
		if ring.Answered == "" && (sub.all && len(ring.Exts) > 0 || ring.Exts[sub.ext]) {
			now = append(now, crmEvent{Type: "ring", CallID: ring.LinkedID, Number: ring.Number, Name: ring.Name, DID: ring.DID, Ext: sub.ext,
				Result: ring.Result, At: ring.Start.UTC().Format(time.RFC3339)})
		}
	}
	s.crmLive.mu.Unlock()
	defer func() {
		s.crmLive.mu.Lock()
		delete(s.crmLive.subs, sub)
		s.crmLive.mu.Unlock()
	}()
	send := func(ev crmEvent) bool {
		b, _ := json.Marshal(ev)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	for _, ev := range now {
		if !send(ev) {
			return
		}
	}
	beat := time.NewTicker(25 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.ch:
			if !send(ev) {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func isReceptionRole(role string) bool {
	return role == "reception" || role == "admin" || role == "superadmin"
}

// ---- after the call: note, task, new contact ---------------------------------

var crmOutcomes = []string{"connected", "voicemail", "noanswer", "busy"}

func (s *Server) crmNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CallID   string `json:"callId"`
		Note     string `json:"note"`
		Outcome  string `json:"outcome"`
		TaskDays int    `json:"taskDays"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	if len(req.Note) > 4000 {
		fieldErr(w, "note")
		return
	}
	if req.Outcome != "" && !slices.Contains(crmOutcomes, req.Outcome) {
		fieldErr(w, "outcome")
		return
	}
	if req.TaskDays < 0 || req.TaskDays > 60 {
		fieldErr(w, "taskDays")
		return
	}
	if req.CallID == "" || len(req.CallID) > 64 {
		fieldErr(w, "callId")
		return
	}
	if req.Note == "" && req.TaskDays == 0 && req.Outcome == "" {
		fieldErr(w, "note")
		return
	}
	u := sessionFrom(r).User
	ext := ""
	if u.ExtensionID != nil {
		if e, err := s.PBX.Extension(r.Context(), *u.ExtensionID); err == nil {
			ext = e.Number
		}
	}
	text := crmNoteText(req.Note, req.Outcome, u.DisplayName)
	// The note may arrive before the call is logged: keep it for the moment it is.
	var id int64
	var callExt string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id, ext FROM crm_calls WHERE linkedid = ?`, req.CallID).Scan(&id, &callExt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.crmLive.mu.Lock()
		if ring := s.crmLive.ringing[req.CallID]; ring != nil && ring.Answered != "" && ring.Answered != ext && !isReceptionRole(u.Role) {
			s.crmLive.mu.Unlock()
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		s.crmLive.notes[req.CallID] = crmNote{Note: text, TaskDays: req.TaskDays, At: time.Now()}
		s.crmLive.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "queued"})
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if callExt != ext && !isReceptionRole(u.Role) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE crm_calls SET note = ?, note_task = ?, note_state = 'pending' WHERE id = ?`, text, req.TaskDays, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.crmWake()
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "queued"})
}

func crmNoteText(note, outcome, by string) string {
	var b strings.Builder
	if outcome != "" {
		b.WriteString("Outcome: " + map[string]string{"connected": "connected", "voicemail": "left voicemail", "noanswer": "no answer", "busy": "busy"}[outcome] + "\n")
	}
	if note != "" {
		b.WriteString(note + "\n")
	}
	if by != "" {
		b.WriteString("— " + by)
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) crmCreateContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Number  string `json:"number"`
		Name    string `json:"name"`
		Company string `json:"company"`
		CRM     string `json:"crm"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Name, req.Company = strings.TrimSpace(req.Name), strings.TrimSpace(req.Company)
	if req.Name == "" || len(req.Name) > 120 {
		fieldErr(w, "name")
		return
	}
	if len(req.Company) > 120 {
		fieldErr(w, "company")
		return
	}
	n, ok := s.crmNumber(r.Context(), req.Number)
	if !ok {
		fieldErr(w, "number")
		return
	}
	kind := req.CRM
	if kind == "" {
		for _, k := range s.crm.Settings().Order {
			if c, _ := s.crm.Connector(k); c != nil {
				kind = k
				break
			}
		}
	}
	c, _ := s.crm.Connector(kind)
	if c == nil {
		writeErr(w, http.StatusConflict, "crm_not_connected")
		return
	}
	phone := n.Raw
	if n.E164 != "" {
		phone = n.E164
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	t := time.Now()
	contact, err := c.CreateContact(ctx, req.Name, req.Company, phone)
	s.crmLogRow(r.Context(), kind, "contact", req.Name+" "+phone, time.Since(t), err, 0)
	if err != nil {
		s.crm.Fail(kind, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "crm_failed", "message": crmErrText(err)})
		return
	}
	s.crm.Forget(n)
	s.audit(r, "crm.contact", kind+" "+req.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"contact": contact})
}

// ---- logging calls -----------------------------------------------------------

// crmCDR collects the CDRs of a call; a few seconds after the last one the call is logged.
func (s *Server) crmCDR(rec cdr.Record) {
	if s.crmLive == nil || rec.LinkedID == "" || !s.crm.Active() {
		return
	}
	lid := rec.LinkedID
	s.crmLive.mu.Lock()
	defer s.crmLive.mu.Unlock()
	s.crmLive.legs[lid] = append(s.crmLive.legs[lid], rec)
	if t := s.crmLive.timers[lid]; t != nil {
		t.Stop()
	}
	s.crmLive.timers[lid] = time.AfterFunc(crmSettle, func() { s.crmFinish(lid) })
}

func (s *Server) crmFinish(lid string) {
	s.crmLive.mu.Lock()
	legs := s.crmLive.legs[lid]
	delete(s.crmLive.legs, lid)
	delete(s.crmLive.timers, lid)
	note, hasNote := s.crmLive.notes[lid]
	delete(s.crmLive.notes, lid)
	for k, n := range s.crmLive.notes {
		if time.Since(n.At) > time.Hour {
			delete(s.crmLive.notes, k)
		}
	}
	s.crmLive.mu.Unlock()
	sum, ok := crm.Summarize(legs)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := s.crmRecord(ctx, sum)
	if err != nil || id == 0 {
		return
	}
	if hasNote {
		_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET note = ?, note_task = ?, note_state = 'pending' WHERE id = ?`, note.Note, note.TaskDays, id)
	}
	s.crmProcess(ctx, id)
}

// crmRecord decides where and whether a call is logged and stores it. It returns 0 when there is
// nothing to do.
func (s *Server) crmRecord(ctx context.Context, sum crm.Summary) (int64, error) {
	n, ok := s.crmNumber(ctx, sum.Number)
	if !ok {
		return 0, nil
	}
	st := s.crm.Settings()
	res, err := s.crm.Lookup(ctx, n)
	state, kind := "skipped", ""
	var contact crm.Contact
	if res.Found() {
		kind, contact = res.CRM, res.First()
	} else if err == nil {
		// Unknown number: create a contact in the first CRM that wants that.
		for _, k := range st.Order {
			if c, cfg := s.crm.Connector(k); c != nil && cfg.CreateUnknown && sum.Direction == "inbound" {
				phone := n.Raw
				if n.E164 != "" {
					phone = n.E164
				}
				t := time.Now()
				contact, err = c.CreateContact(ctx, phone, "", phone)
				s.crmLogRow(ctx, k, "contact", phone, time.Since(t), err, 0)
				if err == nil {
					kind = k
					s.crm.Forget(n)
				}
				break
			}
		}
	} else {
		// The CRM could not be asked: try again later with the first connected CRM.
		for _, k := range st.Order {
			if c, _ := s.crm.Connector(k); c != nil {
				kind, state = k, "lookup"
				break
			}
		}
	}
	wantTask := false
	if kind != "" {
		cfg := st.Connectors[kind]
		log := cfg.LogCalls && (cfg.LogWhich != crm.LogInbound || sum.Direction == "inbound") && (cfg.LogWhich != crm.LogAnswered || sum.Answered)
		wantTask = cfg.MissedTask && sum.Direction == "inbound" && !sum.Answered && contact.ID != ""
		if state != "lookup" {
			state = "skipped"
			if log || wantTask {
				state = "pending"
			}
		}
	}
	recs := s.crmRecordings(ctx, sum)
	cj, _ := json.Marshal(contact)
	now := time.Now().UTC().Format(time.RFC3339)
	res2, err := s.DB.ExecContext(ctx, `INSERT INTO crm_calls(linkedid, crm, direction, number, line, ext, answered, start_at, talk_sec, contact,
		want_task, recordings, state, next_at, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(linkedid) DO NOTHING`,
		sum.LinkedID, kind, sum.Direction, sum.Number, sum.Line, sum.Ext, b2i(sum.Answered), sum.Start.UTC().Format(time.RFC3339), sum.TalkSec,
		string(cj), b2i(wantTask), strings.Join(recs, ","), state, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res2.LastInsertId()
	if n, _ := res2.RowsAffected(); n == 0 {
		return 0, nil
	}
	return id, nil
}

func (s *Server) crmRecordings(ctx context.Context, sum crm.Summary) []string {
	ids := map[string]bool{}
	for _, u := range sum.UniqueIDs {
		ids[u] = true
	}
	var out []string
	for _, r := range s.Spool.Recordings(s.loc(ctx), func(r media.Recording) bool { return ids[r.UniqueID] }) {
		out = append(out, r.Name)
	}
	return out
}

type crmCallRow struct {
	ID          int64
	LinkedID    string
	Kind        string
	Direction   string
	Number      string
	Line        string
	Ext         string
	Answered    bool
	Start       time.Time
	TalkSec     int
	Contact     crm.Contact
	Body        string
	ActivityID  string
	TaskID      string
	WantTask    bool
	Note        string
	NoteTask    int
	NoteState   string
	Recordings  []string
	SummaryDone int
	State       string
	Attempts    int
	Created     time.Time
}

func (s *Server) crmCallRow(ctx context.Context, id int64) (*crmCallRow, error) {
	var c crmCallRow
	var answered, want int
	var start, created, contact, recs string
	err := s.DB.QueryRowContext(ctx, `SELECT id, linkedid, crm, direction, number, line, ext, answered, start_at, talk_sec, contact, body,
		activity_id, task_id, want_task, note, note_task, note_state, recordings, summary_done, state, attempts, created_at FROM crm_calls WHERE id = ?`, id).
		Scan(&c.ID, &c.LinkedID, &c.Kind, &c.Direction, &c.Number, &c.Line, &c.Ext, &answered, &start, &c.TalkSec, &contact, &c.Body,
			&c.ActivityID, &c.TaskID, &want, &c.Note, &c.NoteTask, &c.NoteState, &recs, &c.SummaryDone, &c.State, &c.Attempts, &created)
	if err != nil {
		return nil, err
	}
	c.Answered, c.WantTask = answered == 1, want == 1
	c.Start, _ = time.Parse(time.RFC3339, start)
	c.Created, _ = time.Parse(time.RFC3339, created)
	_ = json.Unmarshal([]byte(contact), &c.Contact)
	if recs != "" {
		c.Recordings = strings.Split(recs, ",")
	}
	return &c, nil
}

type crmAgent struct {
	id          int64
	name, email string
}

// crmAgentOf finds the Kiestoon user of an extension.
func (s *Server) crmAgentOf(ctx context.Context, ext string) crmAgent {
	var a crmAgent
	if ext == "" {
		return a
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT u.id, COALESCE(NULLIF(u.display_name, ''), u.username), u.email FROM users u JOIN extensions e ON e.id = u.extension_id
		WHERE e.number = ? AND u.enabled = 1 ORDER BY u.id LIMIT 1`, ext).Scan(&a.id, &a.name, &a.email)
	if a.name == "" {
		_ = s.DB.QueryRowContext(ctx, `SELECT name FROM extensions WHERE number = ?`, ext).Scan(&a.name)
	}
	return a
}

func (s *Server) crmBody(ctx context.Context, c *crmCallRow, cfg *crm.Config, agent crmAgent) string {
	var b strings.Builder
	switch {
	case c.Direction == "outbound":
		fmt.Fprintf(&b, "Outbound call by %s (%s) to %s.", firstNonEmpty(agent.name, c.Ext), c.Ext, c.Number)
	case c.Answered:
		fmt.Fprintf(&b, "Inbound call from %s, answered by %s (%s).", c.Number, firstNonEmpty(agent.name, c.Ext), c.Ext)
	default:
		fmt.Fprintf(&b, "Missed call from %s.", c.Number)
	}
	if c.Line != "" && c.Direction == "inbound" {
		fmt.Fprintf(&b, " Number called: %s.", c.Line)
	}
	if c.TalkSec > 0 {
		fmt.Fprintf(&b, " Duration %d:%02d.", c.TalkSec/60, c.TalkSec%60)
	}
	if cfg.RecordingLink && len(c.Recordings) > 0 {
		if base, _ := store.GetSetting(ctx, s.DB, "public_url", ""); base != "" {
			fmt.Fprintf(&b, "\nRecording: %s/recordings?play=%s", strings.TrimRight(base, "/"), url.QueryEscape(c.Recordings[0]))
		}
	}
	b.WriteString("\nLogged by Kiestoon.")
	return b.String()
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// crmProcess writes what is due for one call: the activity, the call-back task, the agent's note
// and the AI summary. A failure schedules a retry.
func (s *Server) crmProcess(ctx context.Context, id int64) {
	c, err := s.crmCallRow(ctx, id)
	if err != nil {
		return
	}
	conn, cfg := s.crm.Connector(c.Kind)
	if conn == nil {
		return
	}
	// A look-up that failed before: try it again now.
	if c.State == "lookup" {
		n, _ := s.crmNumber(ctx, c.Number)
		res, err := s.crm.Lookup(ctx, n)
		if err != nil {
			s.crmFailed(ctx, c, err)
			return
		}
		if !res.Found() {
			_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET state = 'skipped', error = '' WHERE id = ?`, id)
			return
		}
		cj, _ := json.Marshal(res.First())
		c.Kind, c.Contact = res.CRM, res.First()
		conn, cfg = s.crm.Connector(c.Kind)
		c.WantTask = cfg.MissedTask && c.Direction == "inbound" && !c.Answered
		_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET crm = ?, contact = ?, want_task = ?, state = 'pending' WHERE id = ?`, c.Kind, string(cj), b2i(c.WantTask), id)
		c.State = "pending"
	}
	agent := s.crmAgentOf(ctx, c.Ext)
	ownerID := s.crm.OwnerFor(ctx, c.Kind, strconv.FormatInt(agent.id, 10), agent.email)
	if ownerID == "" {
		ownerID = c.Contact.OwnerID
	}
	call := crm.Call{Contact: c.Contact, Direction: c.Direction, Answered: c.Answered, Number: c.Number, Line: c.Line, Start: c.Start,
		TalkSec: c.TalkSec, OwnerID: ownerID, Agent: agent.name, Body: c.Body}
	logIt := cfg.LogCalls && (cfg.LogWhich != crm.LogInbound || c.Direction == "inbound") && (cfg.LogWhich != crm.LogAnswered || c.Answered)
	if c.State == "pending" || c.State == "retry" {
		if logIt && c.ActivityID == "" {
			call.Body = s.crmBody(ctx, c, cfg, agent)
			t := time.Now()
			aid, err := conn.LogCall(ctx, call)
			s.crmLogRow(ctx, c.Kind, "call", fmt.Sprintf("%s %s %d:%02d", c.Direction, c.Number, c.TalkSec/60, c.TalkSec%60), time.Since(t), err, id)
			if err != nil {
				s.crmFailed(ctx, c, err)
				return
			}
			c.ActivityID, c.Body = aid, call.Body
			_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET activity_id = ?, body = ? WHERE id = ?`, aid, call.Body, id)
		}
		if c.WantTask && c.TaskID == "" {
			t := time.Now()
			tid, err := conn.CreateTask(ctx, crm.Task{Contact: c.Contact, OwnerID: c.Contact.OwnerID, Subject: "Call back " + firstNonEmpty(c.Contact.Name, c.Number),
				Body: "Missed call from " + c.Number + " at " + c.Start.In(s.loc(ctx)).Format("2006-01-02 15:04") + ".", Due: time.Now()})
			s.crmLogRow(ctx, c.Kind, "task", c.Number, time.Since(t), err, id)
			if err != nil {
				s.crmFailed(ctx, c, err)
				return
			}
			c.TaskID = firstNonEmpty(tid, "-")
			_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET task_id = ? WHERE id = ?`, c.TaskID, id)
		}
		_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET state = 'logged', error = '' WHERE id = ?`, id)
		c.State = "logged"
	}
	if c.NoteState == "pending" && (c.State == "logged" || c.State == "skipped") {
		if err := s.crmWriteNote(ctx, c, conn, call, ownerID); err != nil {
			s.crmFailed(ctx, c, err)
			return
		}
	}
	if c.State == "logged" && c.SummaryDone == 0 && cfg.AISummary && c.ActivityID != "" {
		s.crmSummary(ctx, c, conn, call)
	}
}

func (s *Server) crmWriteNote(ctx context.Context, c *crmCallRow, conn crm.Connector, call crm.Call, ownerID string) error {
	if c.Note != "" {
		t := time.Now()
		var err error
		if c.ActivityID != "" {
			err = conn.AddNote(ctx, c.ActivityID, call, c.Note)
		} else {
			// The call itself was not logged (settings): the note becomes its own activity.
			call.Body = c.Note
			_, err = conn.LogCall(ctx, call)
		}
		s.crmLogRow(ctx, c.Kind, "note", c.Number, time.Since(t), err, c.ID)
		if err != nil {
			return err
		}
		if c.ActivityID != "" {
			c.Body = strings.TrimSpace(c.Body + "\n\n" + c.Note)
			_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET body = ? WHERE id = ?`, c.Body, c.ID)
		}
	}
	if c.NoteTask > 0 {
		t := time.Now()
		_, err := conn.CreateTask(ctx, crm.Task{Contact: c.Contact, OwnerID: ownerID, Subject: "Follow up: " + firstNonEmpty(c.Contact.Name, c.Number),
			Body: c.Note, Due: time.Now().AddDate(0, 0, c.NoteTask)})
		s.crmLogRow(ctx, c.Kind, "task", c.Number, time.Since(t), err, c.ID)
		if err != nil {
			return err
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE crm_calls SET note_state = 'done', note_task = 0 WHERE id = ?`, c.ID)
	return err
}

// crmSummary adds the AI summary of the recording once it is ready (gives up after two hours).
func (s *Server) crmSummary(ctx context.Context, c *crmCallRow, conn crm.Connector, call crm.Call) {
	if len(c.Recordings) == 0 || !s.aiSettings(ctx).Calls || time.Since(c.Created) > 2*time.Hour {
		_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET summary_done = 2 WHERE id = ?`, c.ID)
		return
	}
	res := s.aiLoad("rec-" + c.Recordings[0])
	if res == nil {
		return
	}
	if res.Status != "done" || res.Summary == nil || res.Summary.Summary == "" {
		_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET summary_done = 2 WHERE id = ?`, c.ID)
		return
	}
	text := "AI summary: " + res.Summary.Summary
	for _, a := range res.Summary.Actions {
		text += "\n- " + a
	}
	call.Body = c.Body
	t := time.Now()
	err := conn.AddNote(ctx, c.ActivityID, call, text)
	s.crmLogRow(ctx, c.Kind, "summary", c.Number, time.Since(t), err, c.ID)
	if err != nil {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET summary_done = 1, body = ? WHERE id = ?`, strings.TrimSpace(c.Body+"\n\n"+text), c.ID)
}

// crmFailed schedules the next attempt: after 1, 5 and 30 minutes, then hourly for 24 hours.
// After that the call is given up and the admins get one mail.
func (s *Server) crmFailed(ctx context.Context, c *crmCallRow, err error) {
	s.crm.Fail(c.Kind, err)
	c.Attempts++
	wait := time.Hour
	if c.Attempts <= len(crmRetry) {
		wait = crmRetry[c.Attempts-1]
	}
	state := "retry"
	if c.State == "lookup" {
		state = "lookup"
	}
	if c.State == "logged" || c.State == "skipped" {
		state = c.State
	}
	if time.Since(c.Created)+wait > 24*time.Hour {
		state = "failed"
		subject := "Kiestoon could not write a call to " + crm.Names[c.Kind]
		body := fmt.Sprintf("Kiestoon tried for 24 hours to write the call with %s (%s) to %s and gave up.\n\nLast error: %s\n\nSee Integrations → CRM → sync log; you can retry it there.\n",
			c.Number, c.Start.In(s.loc(ctx)).Format("2006-01-02 15:04"), crm.Names[c.Kind], crmErrText(err))
		go s.Mail.Notify(context.Background(), subject, body)
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE crm_calls SET state = ?, attempts = ?, next_at = ?, error = ? WHERE id = ?`,
		state, c.Attempts, time.Now().Add(wait).UTC().Format(time.RFC3339), crmErrText(err), c.ID)
}

func (s *Server) crmWake() {
	select {
	case s.crmLive.wake <- struct{}{}:
	default:
	}
}

// runCRM retries failed writes, writes notes and summaries, and keeps the tables small.
func (s *Server) runCRM(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.crmLive.wake:
		}
		if !s.crm.Active() {
			continue
		}
		now := time.Now().UTC()
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM crm_calls WHERE (state IN ('pending','retry','lookup') AND next_at <= ?)
			OR note_state = 'pending' OR (state = 'logged' AND summary_done = 0 AND recordings <> '') ORDER BY id LIMIT 100`, now.Format(time.RFC3339))
		if err != nil {
			continue
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			s.crmProcess(cctx, id)
			cancel()
		}
		cut := now.Add(-400 * 24 * time.Hour).Format(time.RFC3339)
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM crm_calls WHERE start_at < ?`, cut)
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM crm_log WHERE at < ?`, now.Add(-30*24*time.Hour).Format(time.RFC3339))
	}
}

// ---- sync log and call history -------------------------------------------------

func (s *Server) crmLog(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		ID         int64  `json:"id"`
		At         string `json:"at"`
		CRM        string `json:"crm"`
		Action     string `json:"action"`
		Detail     string `json:"detail"`
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		DurationMs int64  `json:"durationMs"`
		CallID     int64  `json:"callId"`
		CallState  string `json:"callState"`
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT l.id, l.at, l.crm, l.action, l.detail, l.ok, l.error, l.duration_ms, COALESCE(l.call_id, 0), COALESCE(c.state, '')
		FROM crm_log l LEFT JOIN crm_calls c ON c.id = l.call_id ORDER BY l.id DESC LIMIT 200`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	out := []entry{}
	for rows.Next() {
		var e entry
		var ok int
		if rows.Scan(&e.ID, &e.At, &e.CRM, &e.Action, &e.Detail, &ok, &e.Error, &e.DurationMs, &e.CallID, &e.CallState) == nil {
			e.OK = ok == 1
			out = append(out, e)
		}
	}
	var pending, failed int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(CASE WHEN state IN ('pending','retry','lookup') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN state = 'failed' THEN 1 ELSE 0 END), 0) FROM crm_calls`).Scan(&pending, &failed)
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "pending": pending, "failed": failed})
}

func (s *Server) crmRetryCall(w http.ResponseWriter, r *http.Request) {
	res, err := s.DB.ExecContext(r.Context(), `UPDATE crm_calls SET state = CASE WHEN state = 'failed' AND activity_id = '' THEN 'retry' ELSE state END,
		note_state = CASE WHEN note_state = 'failed' THEN 'pending' ELSE note_state END, next_at = ?, attempts = 0, created_at = ?
		WHERE id = ? AND state IN ('failed','retry','lookup')`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), pathID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "crm.retry", strconv.FormatInt(pathID(r), 10))
	s.crmWake()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// crmCallInfo is the CRM column of the call history.
type crmCallInfo struct {
	ID      int64  `json:"id"`
	CRM     string `json:"crm"`
	State   string `json:"state"`
	Task    bool   `json:"task"`
	Contact string `json:"contact"`
	Company string `json:"company"`
	URL     string `json:"url"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) crmCallsFor(w http.ResponseWriter, r *http.Request) {
	ids := strings.Split(r.URL.Query().Get("linked"), ",")
	if len(ids) > 200 {
		ids = ids[:200]
	}
	out := map[string]crmCallInfo{}
	for _, lid := range ids {
		lid = strings.TrimSpace(lid)
		if lid == "" {
			continue
		}
		var c crmCallInfo
		var contact, task string
		err := s.DB.QueryRowContext(r.Context(), `SELECT id, crm, state, task_id, contact, error FROM crm_calls WHERE linkedid = ?`, lid).
			Scan(&c.ID, &c.CRM, &c.State, &task, &contact, &c.Error)
		if err != nil {
			continue
		}
		var ct crm.Contact
		_ = json.Unmarshal([]byte(contact), &ct)
		c.Task, c.Contact, c.Company, c.URL = task != "", ct.Name, ct.Company, ct.URL
		out[lid] = c
	}
	writeJSON(w, http.StatusOK, out)
}
