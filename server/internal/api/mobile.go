package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/push"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// The mobile app (iPhone and Android) is the softphone of the user portal in an app shell, like
// the Windows softphone, and links itself the same way (a pairing code, shown as a QR code). On
// top of that it registers its push token, so that the phone system can wake it for a call:
//
//  1. A call for an extension with the app linked also rings Local/<number>@kiestoon-appwake.
//  2. When the app is not connected, that context sends the UserEvent KiestoonWake and waits.
//  3. The server sends a "call" push to every linked phone of the user, through the push relay.
//  4. The phone shows the call (CallKit / Android call screen), the app connects and registers
//     <number>-app, and the waiting call rings it.
//  5. When the caller hangs up first, KiestoonWakeEnd makes the server send "cancel".
//
// See internal/push for the relay and why it needs little trust.

const mobileKeyKey = "mobile_install_key"

type mobileState struct {
	mu sync.Mutex
	// mwi is the last number of new voicemail messages per mailbox, to push only new messages.
	mwi map[string]int
	// relay overrides the relay client (tests).
	relay *push.Client
}

func (s *Server) mobileRoutes(mux *http.ServeMux, admin, full func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/mobile/settings", admin(s.mobileSettingsGet))
	mux.Handle("PUT /api/mobile/settings", admin(s.mobileSettingsSave))
	mux.Handle("POST /api/mobile/relay-test", admin(s.mobileRelayTest))
	mux.Handle("DELETE /api/mobile/devices/{id}", admin(s.mobileDeviceDelete))
	// The user portal and the app itself (a desktop session may use /api/portal/).
	mux.Handle("GET /api/portal/mobile", full(s.portalMobile))
	mux.Handle("DELETE /api/portal/mobile/devices/{id}", full(s.portalMobileDelete))
	mux.Handle("POST /api/portal/mobile/devices/{id}/test", full(s.portalMobileTest))
	mux.Handle("POST /api/portal/mobile/register", full(s.portalMobileRegister))
	mux.Handle("POST /api/portal/mobile/unregister", full(s.portalMobileUnregister))
	mux.Handle("GET /api/portal/mobile/sip", full(s.portalMobileSIP))
}

// installKey is the ed25519 key of this installation at the relay. It is made once.
func (s *Server) installKey(ctx context.Context) ed25519.PrivateKey {
	v, _ := store.GetSetting(ctx, s.DB, mobileKeyKey, "")
	if seed, err := base64.StdEncoding.DecodeString(v); err == nil && len(seed) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(seed)
	}
	seed := make([]byte, ed25519.SeedSize)
	_, _ = rand.Read(seed)
	_ = store.SetSetting(ctx, s.DB, mobileKeyKey, base64.StdEncoding.EncodeToString(seed))
	return ed25519.NewKeyFromSeed(seed)
}

func (s *Server) relayClient(ctx context.Context) (*push.Client, pbx.MobileSettings) {
	st := pbx.LoadMobileSettings(ctx, s.PBX)
	if s.mobile.relay != nil {
		return s.mobile.relay, st
	}
	return &push.Client{URL: st.RelayURL, Key: s.installKey(ctx)}, st
}

// mobileDevice is one linked phone.
type mobileDevice struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	User      string `json:"user"`
	UserName  string `json:"userName"`
	Extension string `json:"extension"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	CreatedAt string `json:"createdAt"`
	LastSeen  string `json:"lastSeen"`
	LastPush  string `json:"lastPush"`

	handle, alertHandle, secret, lang string
}

const mobileCols = `d.id, d.user_id, u.username, u.display_name, COALESCE(e.number, ''), d.name, d.platform, d.created_at, d.last_seen,
	d.last_push, d.handle, d.alert_handle, d.secret, u.language`

const mobileFrom = ` FROM mobile_devices d JOIN users u ON u.id = d.user_id LEFT JOIN extensions e ON e.id = u.extension_id`

func (s *Server) mobileDevices(ctx context.Context, where string, args ...any) ([]mobileDevice, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+mobileCols+mobileFrom+` `+where+` ORDER BY u.username, d.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mobileDevice{}
	for rows.Next() {
		var d mobileDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.User, &d.UserName, &d.Extension, &d.Name, &d.Platform, &d.CreatedAt,
			&d.LastSeen, &d.LastPush, &d.handle, &d.alertHandle, &d.secret, &d.lang); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---- admin ----------------------------------------------------------------------------------

func (s *Server) mobileSettingsGet(w http.ResponseWriter, r *http.Request) {
	st := pbx.LoadMobileSettings(r.Context(), s.PBX)
	devs, err := s.mobileDevices(r.Context(), "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": st.Enabled, "relayURL": st.RelayURL, "defaultRelayURL": push.DefaultRelay,
		"publicKey": push.EncodeKey(push.PublicKeyOf(s.installKey(r.Context()))), "devices": devs,
	})
}

func (s *Server) mobileSettingsSave(w http.ResponseWriter, r *http.Request) {
	var st pbx.MobileSettings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	st.RelayURL = strings.TrimRight(strings.TrimSpace(st.RelayURL), "/")
	if st.RelayURL == "" {
		st.RelayURL = push.DefaultRelay
	}
	if u, err := url.Parse(st.RelayURL); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		writeErr(w, http.StatusBadRequest, "invalid_relay")
		return
	}
	old := pbx.LoadMobileSettings(r.Context(), s.PBX)
	if err := store.SetJSON(r.Context(), s.DB, pbx.MobileKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if old.Enabled != st.Enabled {
		s.Ast.Changed()
	}
	s.audit(r, "mobile.settings", strconv.FormatBool(st.Enabled))
	writeJSON(w, http.StatusOK, st)
}

// mobileRelayTest asks the relay which platforms it serves.
func (s *Server) mobileRelayTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RelayURL string `json:"relayURL"`
	}
	_ = decode(r, &req)
	c, st := s.relayClient(r.Context())
	base := strings.TrimRight(strings.TrimSpace(req.RelayURL), "/")
	if base == "" {
		base = st.RelayURL
	}
	if u, err := url.Parse(base); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		writeErr(w, http.StatusBadRequest, "invalid_relay")
		return
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	res, err := hc.Do(req2)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "unreachable"})
		return
	}
	defer res.Body.Close()
	var out struct {
		OK        bool     `json:"ok"`
		Platforms []string `json:"platforms"`
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK || json.Unmarshal(raw, &out) != nil || !out.OK {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "not_a_relay"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "platforms": out.Platforms})
}

func (s *Server) mobileDeviceDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !s.deleteMobileDevice(r.Context(), "id = ?", id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "mobile.device_removed", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteMobileDevice removes devices and regenerates the dialplan (the -app endpoint goes away with
// the last phone of a user).
func (s *Server) deleteMobileDevice(ctx context.Context, where string, args ...any) bool {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_devices WHERE `+where, args...)
	if err != nil {
		return false
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false
	}
	s.Ast.Changed()
	return true
}

// ---- user portal and app ----------------------------------------------------------------------

func (s *Server) portalMobile(w http.ResponseWriter, r *http.Request) {
	u := sessionFrom(r).User
	st := pbx.LoadMobileSettings(r.Context(), s.PBX)
	devs, err := s.mobileDevices(r.Context(), "WHERE d.user_id = ?", u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": st.Enabled, "devices": devs})
}

func (s *Server) portalMobileDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !s.deleteMobileDevice(r.Context(), "id = ? AND user_id = ?", id, sessionFrom(r).User.ID) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "mobile.device_removed", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalMobileTest(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	devs, err := s.mobileDevices(r.Context(), "WHERE d.id = ? AND d.user_id = ?", id, sessionFrom(r).User.ID)
	if err != nil || len(devs) == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.pushTo(r.Context(), devs[0], push.KindTest, "test", map[string]any{"at": time.Now().Unix()}); err != nil {
		if errors.Is(err, push.ErrGone) {
			writeErr(w, http.StatusGone, "device_gone")
			return
		}
		s.Log.Warn("mobile test push", "err", err)
		writeErr(w, http.StatusBadGateway, "relay_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// portalMobileRegister is called by the app after linking and whenever its push token changes.
func (s *Server) portalMobileRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID    string `json:"appId"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Token    string `json:"token"`
		// AlertToken is the second token of an iPhone, for visible notifications: Token is then its
		// PushKit (VoIP) token, which only takes calls.
		AlertToken string `json:"alertToken"`
		Sandbox    bool   `json:"sandbox"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) > 60 {
		req.Name = req.Name[:60]
	}
	if req.AppID == "" || len(req.AppID) > 64 || (req.Platform != push.APNs && req.Platform != push.FCM) || req.Token == "" || len(req.Token) > 4096 || len(req.AlertToken) > 4096 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	u := sessionFrom(r).User
	if s.myExtension(w, r) == nil {
		return
	}
	c, st := s.relayClient(r.Context())
	if !st.Enabled {
		writeErr(w, http.StatusConflict, "mobile_disabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tokenPlatform := req.Platform
	if req.Platform == push.APNs {
		tokenPlatform = push.APNsVoIP
	}
	handle, err := c.Register(ctx, tokenPlatform, req.Token, req.Sandbox)
	alertHandle := ""
	if err == nil && req.Platform == push.APNs && req.AlertToken != "" {
		alertHandle, err = c.Register(ctx, push.APNs, req.AlertToken, req.Sandbox)
	}
	if err != nil {
		s.Log.Warn("mobile register", "err", err)
		writeErr(w, http.StatusBadGateway, "relay_failed")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// A phone that registers again keeps its row (and its id) but gets a new secret.
	secret := push.NewDeviceSecret()
	var id int64
	var existed bool
	err = s.DB.QueryRowContext(r.Context(), `SELECT id FROM mobile_devices WHERE user_id = ? AND app_id = ?`, u.ID, req.AppID).Scan(&id)
	switch {
	case err == nil:
		existed = true
		_, err = s.DB.ExecContext(r.Context(), `UPDATE mobile_devices SET name = ?, platform = ?, handle = ?, alert_handle = ?, secret = ?, last_seen = ?
			WHERE id = ?`, req.Name, req.Platform, handle, alertHandle, secret, now, id)
	case errors.Is(err, sql.ErrNoRows):
		var res sql.Result
		res, err = s.DB.ExecContext(r.Context(), `INSERT INTO mobile_devices(user_id, app_id, name, platform, handle, alert_handle, secret, created_at, last_seen)
			VALUES (?,?,?,?,?,?,?,?,?)`, u.ID, req.AppID, req.Name, req.Platform, handle, alertHandle, secret, now, now)
		if err == nil {
			id, _ = res.LastInsertId()
		}
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if !existed {
		s.Ast.Changed()
		s.audit(r, "mobile.device_linked", req.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "secret": secret})
}

func (s *Server) portalMobileUnregister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID string `json:"appId"`
	}
	if err := decode(r, &req); err != nil || req.AppID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.deleteMobileDevice(r.Context(), "user_id = ? AND app_id = ?", sessionFrom(r).User.ID, req.AppID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// portalMobileSIP gives the app its account (<number>-app).
func (s *Server) portalMobileSIP(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	u := sessionFrom(r).User
	if n, _ := s.countMobile(r.Context(), u.ID); n == 0 || !pbx.LoadMobileSettings(r.Context(), s.PBX).Enabled {
		writeErr(w, http.StatusConflict, "not_registered")
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE mobile_devices SET last_seen = ? WHERE user_id = ?`, time.Now().UTC().Format(time.RFC3339), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"wsPath": "/api/webrtc/ws", "username": e.AppEndpoint(), "password": e.WebRTCSecret, "number": e.Number,
		"name": e.Name, "domain": "kiestoon.local",
	})
}

func (s *Server) countMobile(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mobile_devices WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// ---- notifications --------------------------------------------------------------------------

// errNoAlertToken: an iPhone that gave no token for visible notifications.
var errNoAlertToken = errors.New("no alert token")

// pushTo sends one notification to one phone and forgets the phone when the app is gone.
func (s *Server) pushTo(ctx context.Context, d mobileDevice, kind, id string, data any) error {
	c, st := s.relayClient(ctx)
	if !st.Enabled {
		return errors.New("mobile app disabled")
	}
	handle := d.handle
	if d.Platform == push.APNs {
		switch kind {
		case push.KindCall:
		case push.KindCancel:
			// iOS ends the CallKit call itself: when the caller hangs up before the app has
			// connected, no call arrives and the app gives up after a short wait.
			return nil
		default:
			if d.alertHandle == "" {
				return errNoAlertToken
			}
			handle = d.alertHandle
		}
	}
	err := c.Send(ctx, handle, d.secret, kind, id, d.lang, data)
	if errors.Is(err, push.ErrGone) {
		s.deleteMobileDevice(context.Background(), "id = ?", d.ID)
		return err
	}
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE mobile_devices SET last_push = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), d.ID)
	}
	return err
}

// pushExtension sends a notification to every phone of the users of an extension.
func (s *Server) pushExtension(ext, kind, id string, data any) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if !pbx.LoadMobileSettings(ctx, s.PBX).Enabled {
		return
	}
	devs, err := s.mobileDevices(ctx, "WHERE e.number = ? AND u.enabled = 1", ext)
	if err != nil || len(devs) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, d := range devs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.pushTo(ctx, d, kind, id, data); err != nil && !errors.Is(err, push.ErrGone) {
				s.Log.Warn("mobile push", "kind", kind, "device", d.ID, "err", err)
			}
		}()
	}
	wg.Wait()
}

func unescapeAMI(v string) string {
	if u, err := url.QueryUnescape(v); err == nil {
		return u
	}
	return v
}

// onMobileWake handles KiestoonWake: wake the phones of the extension for the waiting call.
func (s *Server) onMobileWake(m asterisk.Message) {
	from, name := unescapeAMI(m.Get("From")), unescapeAMI(m.Get("FromName"))
	if name == from || name == "<unknown>" {
		name = ""
	}
	s.pushExtension(m.Get("Ext"), push.KindCall, m.Get("WakeID"), map[string]any{
		"from": from, "fromName": name, "ext": m.Get("Ext"), "at": time.Now().Unix(),
	})
}

// onMobileWakeEnd handles KiestoonWakeEnd: the call ended before or while the phone rang.
func (s *Server) onMobileWakeEnd(m asterisk.Message) {
	reason := "missed"
	if m.Get("Status") == "ANSWER" {
		reason = "answered"
	}
	s.pushExtension(m.Get("Ext"), push.KindCancel, m.Get("WakeID"), map[string]any{"reason": reason})
}

// onMobileMWI pushes a voicemail notification when a mailbox gets a new message.
func (s *Server) onMobileMWI(m asterisk.Message) {
	mb := strings.TrimSuffix(m.Get("Mailbox"), "@default")
	nw, _ := strconv.Atoi(m.Get("New"))
	s.mobile.mu.Lock()
	if s.mobile.mwi == nil {
		s.mobile.mwi = map[string]int{}
	}
	prev := s.mobile.mwi[mb]
	s.mobile.mwi[mb] = nw
	s.mobile.mu.Unlock()
	if nw <= prev || mb == "" {
		return
	}
	s.pushExtension(mb, push.KindVoicemail, "vm-"+mb, map[string]any{"mailbox": mb, "new": nw})
}

// pushVoicemailSummary sends the AI summary of a new voicemail message as an update of the
// voicemail notification.
func (s *Server) pushVoicemailSummary(mailbox, from, summary string) {
	if summary == "" {
		return
	}
	s.pushExtension(mailbox, push.KindVoicemail, "vm-"+mailbox, map[string]any{"mailbox": mailbox, "from": from, "summary": summary})
}
