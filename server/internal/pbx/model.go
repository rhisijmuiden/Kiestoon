// Package pbx holds the phone system data (extensions, trunks, routes) and its validation rules.
//
// The database is the source of truth. The asterisk package turns a Snapshot of this data into
// Asterisk configuration files, so the configuration can always be rebuilt from the database.
package pbx

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ValidationError carries a stable code the UI translates, plus the field it is about.
type ValidationError struct {
	Code  string
	Field string
}

func (e *ValidationError) Error() string { return e.Code + ": " + e.Field }

func invalid(code, field string) error { return &ValidationError{Code: code, Field: field} }

// Extension is an internal phone number with its SIP account and voicemail box.
type Extension struct {
	ID              int64  `json:"id"`
	Number          string `json:"number"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	SIPSecret       string `json:"sipSecret,omitempty"`
	Transport       string `json:"transport"`
	SRTP            bool   `json:"srtp"`
	MaxContacts     int    `json:"maxContacts"`
	OutboundCID     string `json:"outboundCallerId"`
	RingSeconds     int    `json:"ringSeconds"`
	Voicemail       bool   `json:"voicemail"`
	VoicemailPIN    string `json:"voicemailPin,omitempty"`
	VoicemailEmail  bool   `json:"voicemailEmail"`
	VoicemailDelete bool   `json:"voicemailDelete"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"createdAt"`
	// Features are the call settings users change themselves in the user portal.
	Features Features `json:"features"`
	// RecordMode: off, ondemand (*1 during a call) or always.
	RecordMode string `json:"recordMode"`
	// PickupGroup: phones in the same group can pick up each other's calls with *8. Empty: no group.
	PickupGroup string `json:"pickupGroup"`
	// OutboundClass limits outbound calls: internal, national or international.
	OutboundClass string `json:"outboundClass"`
	AllowPremium  bool   `json:"allowPremium"`
	// WebRTC enables the browser softphone in the user portal.
	WebRTC       bool   `json:"webrtc"`
	WebRTCSecret string `json:"-"`
	// MobileApp is set in a Snapshot when the user of the extension has linked the mobile app (and
	// the app is switched on): calls then wake the phone with a push notification.
	MobileApp bool `json:"-"`
	// TeamsNumber is set in a Snapshot when a Microsoft Teams user is linked to the extension: calls
	// to the extension ring that Teams number too (or only, with TeamsOnly).
	TeamsNumber string `json:"-"`
	TeamsOnly   bool   `json:"-"`
}

// WebRTCEndpoint is the Asterisk endpoint of the browser softphone of an extension.
func (e Extension) WebRTCEndpoint() string { return e.Number + "-web" }

// AppEndpoint is the Asterisk endpoint of the mobile app of an extension. It uses the password of
// the browser softphone and exists only while the user has a phone with the app linked.
func (e Extension) AppEndpoint() string { return e.Number + "-app" }

// Trunk is a connection to a SIP provider.
type Trunk struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	AuthMode     string `json:"authMode"` // register | ip
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Transport    string `json:"transport"`
	Username     string `json:"username"`
	AuthUsername string `json:"authUsername"`
	Secret       string `json:"secret,omitempty"`
	SecretSet    bool   `json:"secretSet"`
	FromDomain   string `json:"fromDomain"`
	FromUser     string `json:"fromUser"`
	MatchIPs     string `json:"matchIps"`
	DIDSource    string `json:"didSource"` // ruri | to
	Codecs       string `json:"codecs"`
	MaxChannels  int    `json:"maxChannels"`
	CallerID     string `json:"callerId"`
	SRTP         bool   `json:"srtp"`
	Enabled      bool   `json:"enabled"`
	// T38 offers T.38 fax (UDPTL) to the provider; without it faxes come in as G.711 audio.
	T38 bool `json:"t38"`
}

// Endpoint is the Asterisk endpoint name of the trunk.
func (t Trunk) Endpoint() string { return fmt.Sprintf("trunk-%d", t.ID) }

// InboundRoute sends calls for a phone number (DID) to a destination.
type InboundRoute struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	DID             string `json:"did"` // "" catches every number without its own route
	DestinationType string `json:"destinationType"`
	Destination     string `json:"destination"`
	CIDPrefix       string `json:"cidPrefix"`
	Enabled         bool   `json:"enabled"`
}

// DialPattern matches dialled numbers: Prefix is removed, Match is an Asterisk pattern
// (digits, X, Z, N, [..], . and !), Prepend is added before the number goes to the trunk.
type DialPattern struct {
	Prefix  string `json:"prefix"`
	Match   string `json:"match"`
	Prepend string `json:"prepend"`
}

// OutboundRoute sends dialled numbers out over trunks, trying them in order.
type OutboundRoute struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Position int           `json:"position"`
	Patterns []DialPattern `json:"patterns"`
	TrunkIDs []int64       `json:"trunkIds"`
	CallerID string        `json:"callerId"`
	Enabled  bool          `json:"enabled"`
}

// Repo reads and writes phone system data.
type Repo struct{ DB *sql.DB }

// GenerateSecret returns a random password of n characters without look-alike characters.
func GenerateSecret(n int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b.WriteByte(alphabet[x.Int64()])
	}
	return b.String()
}

// GeneratePIN returns a random numeric PIN.
func GeneratePIN(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		x, _ := rand.Int(rand.Reader, big.NewInt(10))
		b.WriteByte(byte('0' + x.Int64()))
	}
	return b.String()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- extensions -------------------------------------------------------------

const extCols = `id, number, name, email, sip_secret, transport, srtp, max_contacts, outbound_cid, ring_seconds,
	voicemail, voicemail_pin, voicemail_email, voicemail_delete, enabled, created_at, features, record_mode, pickup_group,
	outbound_class, allow_premium, webrtc, webrtc_secret`

func scanExt(row interface{ Scan(...any) error }) (*Extension, error) {
	var e Extension
	var srtp, vm, vme, vmd, en, prem, web int
	var feat string
	err := row.Scan(&e.ID, &e.Number, &e.Name, &e.Email, &e.SIPSecret, &e.Transport, &srtp, &e.MaxContacts,
		&e.OutboundCID, &e.RingSeconds, &vm, &e.VoicemailPIN, &vme, &vmd, &en, &e.CreatedAt, &feat, &e.RecordMode,
		&e.PickupGroup, &e.OutboundClass, &prem, &web, &e.WebRTCSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	e.SRTP, e.Voicemail, e.VoicemailEmail, e.VoicemailDelete, e.Enabled = srtp == 1, vm == 1, vme == 1, vmd == 1, en == 1
	e.AllowPremium, e.WebRTC = prem == 1, web == 1
	_ = json.Unmarshal([]byte(feat), &e.Features)
	e.Features.normalize()
	return &e, err
}

// Extensions returns all extensions ordered by number.
func (r *Repo) Extensions(ctx context.Context) ([]Extension, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+extCols+` FROM extensions ORDER BY CAST(number AS INTEGER), number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Extension{}
	for rows.Next() {
		e, err := scanExt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Extension returns one extension.
func (r *Repo) Extension(ctx context.Context, id int64) (*Extension, error) {
	return scanExt(r.DB.QueryRowContext(ctx, `SELECT `+extCols+` FROM extensions WHERE id = ?`, id))
}

// SaveExtension validates and inserts (ID 0) or updates an extension.
func (r *Repo) SaveExtension(ctx context.Context, plan NumberPlan, e *Extension) error {
	if err := r.validateExtension(ctx, plan, e); err != nil {
		return err
	}
	if e.SIPSecret == "" {
		e.SIPSecret = GenerateSecret(24)
	}
	if e.Voicemail && e.VoicemailPIN == "" {
		e.VoicemailPIN = GeneratePIN(6)
	}
	if e.WebRTCSecret == "" {
		e.WebRTCSecret = GenerateSecret(32)
	}
	feat, _ := json.Marshal(e.Features)
	args := []any{e.Number, e.Name, e.Email, e.SIPSecret, e.Transport, b2i(e.SRTP), e.MaxContacts, e.OutboundCID,
		e.RingSeconds, b2i(e.Voicemail), e.VoicemailPIN, b2i(e.VoicemailEmail), b2i(e.VoicemailDelete), b2i(e.Enabled),
		string(feat), e.RecordMode, e.PickupGroup, e.OutboundClass, b2i(e.AllowPremium), b2i(e.WebRTC), e.WebRTCSecret}
	if e.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO extensions(number, name, email, sip_secret, transport, srtp, max_contacts,
			outbound_cid, ring_seconds, voicemail, voicemail_pin, voicemail_email, voicemail_delete, enabled,
			features, record_mode, pickup_group, outbound_class, allow_premium, webrtc, webrtc_secret)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "number")
		}
		e.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE extensions SET number=?, name=?, email=?, sip_secret=?, transport=?, srtp=?,
		max_contacts=?, outbound_cid=?, ring_seconds=?, voicemail=?, voicemail_pin=?, voicemail_email=?, voicemail_delete=?,
		enabled=?, features=?, record_mode=?, pickup_group=?, outbound_class=?, allow_premium=?, webrtc=?, webrtc_secret=?
		WHERE id=?`, append(args, e.ID)...)
	if err != nil {
		return uniqueErr(err, "number")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExtension removes an extension. Routes pointing at it are switched to "hang up".
func (r *Repo) DeleteExtension(ctx context.Context, id int64) error {
	e, err := r.Extension(ctx, id)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := replaceDestinations(ctx, tx, func(d *Destination) bool {
		if (d.Type == DestExtension || d.Type == DestVoicemail) && d.Target == e.Number {
			*d = Destination{Type: DestHangup}
			return true
		}
		return false
	}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM extensions WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func uniqueErr(err error, field string) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return invalid("duplicate", field)
	}
	return err
}

// ---- trunks -----------------------------------------------------------------

const trunkCols = `id, name, provider, auth_mode, host, port, transport, username, auth_username, secret, from_domain,
	from_user, match_ips, did_source, codecs, max_channels, caller_id, srtp, enabled, t38`

func scanTrunk(row interface{ Scan(...any) error }) (*Trunk, error) {
	var t Trunk
	var srtp, en, t38 int
	err := row.Scan(&t.ID, &t.Name, &t.Provider, &t.AuthMode, &t.Host, &t.Port, &t.Transport, &t.Username,
		&t.AuthUsername, &t.Secret, &t.FromDomain, &t.FromUser, &t.MatchIPs, &t.DIDSource, &t.Codecs, &t.MaxChannels,
		&t.CallerID, &srtp, &en, &t38)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	t.SRTP, t.Enabled, t.SecretSet, t.T38 = srtp == 1, en == 1, t.Secret != "", t38 == 1
	return &t, err
}

// Trunks returns all trunks, including their secrets (callers strip them before sending to the UI).
func (r *Repo) Trunks(ctx context.Context) ([]Trunk, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+trunkCols+` FROM trunks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trunk{}
	for rows.Next() {
		t, err := scanTrunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Trunk returns one trunk.
func (r *Repo) Trunk(ctx context.Context, id int64) (*Trunk, error) {
	return scanTrunk(r.DB.QueryRowContext(ctx, `SELECT `+trunkCols+` FROM trunks WHERE id = ?`, id))
}

// SaveTrunk validates and stores a trunk. An empty secret on update keeps the stored one.
func (r *Repo) SaveTrunk(ctx context.Context, t *Trunk) error {
	if t.ID != 0 && t.Secret == "" {
		old, err := r.Trunk(ctx, t.ID)
		if err != nil {
			return err
		}
		t.Secret = old.Secret
	}
	if err := validateTrunk(t); err != nil {
		return err
	}
	args := []any{t.Name, t.Provider, t.AuthMode, t.Host, t.Port, t.Transport, t.Username, t.AuthUsername, t.Secret,
		t.FromDomain, t.FromUser, t.MatchIPs, t.DIDSource, t.Codecs, t.MaxChannels, t.CallerID, b2i(t.SRTP), b2i(t.Enabled), b2i(t.T38)}
	if t.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO trunks(name, provider, auth_mode, host, port, transport, username,
			auth_username, secret, from_domain, from_user, match_ips, did_source, codecs, max_channels, caller_id, srtp, enabled, t38)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "name")
		}
		t.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE trunks SET name=?, provider=?, auth_mode=?, host=?, port=?, transport=?,
		username=?, auth_username=?, secret=?, from_domain=?, from_user=?, match_ips=?, did_source=?, codecs=?,
		max_channels=?, caller_id=?, srtp=?, enabled=?, t38=? WHERE id=?`, append(args, t.ID)...)
	return uniqueErr(err, "name")
}

// DeleteTrunk removes a trunk and takes it out of every outbound route.
func (r *Repo) DeleteTrunk(ctx context.Context, id int64) error {
	routes, err := r.OutboundRoutes(ctx)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rt := range routes {
		keep := []int64{}
		for _, tid := range rt.TrunkIDs {
			if tid != id {
				keep = append(keep, tid)
			}
		}
		if len(keep) != len(rt.TrunkIDs) {
			b, _ := json.Marshal(keep)
			if _, err := tx.ExecContext(ctx, `UPDATE outbound_routes SET trunk_ids = ? WHERE id = ?`, string(b), rt.ID); err != nil {
				return err
			}
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM trunks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// ---- inbound routes ---------------------------------------------------------

// InboundRoutes returns all inbound routes, catch-all last.
func (r *Repo) InboundRoutes(ctx context.Context) ([]InboundRoute, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name, did, destination_type, destination, cid_prefix, enabled
		FROM inbound_routes ORDER BY did = '', did`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InboundRoute{}
	for rows.Next() {
		var x InboundRoute
		var en int
		if err := rows.Scan(&x.ID, &x.Name, &x.DID, &x.DestinationType, &x.Destination, &x.CIDPrefix, &en); err != nil {
			return nil, err
		}
		x.Enabled = en == 1
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveInboundRoute validates and stores an inbound route.
func (r *Repo) SaveInboundRoute(ctx context.Context, x *InboundRoute) error {
	x.DID = NormalizeDID(x.DID)
	if err := r.validateInbound(ctx, x); err != nil {
		return err
	}
	args := []any{x.Name, x.DID, x.DestinationType, x.Destination, x.CIDPrefix, b2i(x.Enabled)}
	if x.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO inbound_routes(name, did, destination_type, destination, cid_prefix, enabled)
			VALUES (?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "did")
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE inbound_routes SET name=?, did=?, destination_type=?, destination=?, cid_prefix=?,
		enabled=? WHERE id=?`, append(args, x.ID)...)
	if err != nil {
		return uniqueErr(err, "did")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteInboundRoute removes an inbound route.
func (r *Repo) DeleteInboundRoute(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "inbound_routes", id)
}

func deleteRow(ctx context.Context, db *sql.DB, table string, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- outbound routes --------------------------------------------------------

// OutboundRoutes returns all outbound routes in the order they are tried.
func (r *Repo) OutboundRoutes(ctx context.Context) ([]OutboundRoute, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name, position, patterns, trunk_ids, caller_id, enabled
		FROM outbound_routes ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OutboundRoute{}
	for rows.Next() {
		var x OutboundRoute
		var pats, tids string
		var en int
		if err := rows.Scan(&x.ID, &x.Name, &x.Position, &pats, &tids, &x.CallerID, &en); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(pats), &x.Patterns)
		_ = json.Unmarshal([]byte(tids), &x.TrunkIDs)
		if x.Patterns == nil {
			x.Patterns = []DialPattern{}
		}
		if x.TrunkIDs == nil {
			x.TrunkIDs = []int64{}
		}
		x.Enabled = en == 1
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveOutboundRoute validates and stores an outbound route. New routes go to the end of the list.
func (r *Repo) SaveOutboundRoute(ctx context.Context, x *OutboundRoute) error {
	if err := r.validateOutbound(ctx, x); err != nil {
		return err
	}
	pats, _ := json.Marshal(x.Patterns)
	tids, _ := json.Marshal(x.TrunkIDs)
	if x.ID == 0 {
		var maxPos sql.NullInt64
		_ = r.DB.QueryRowContext(ctx, `SELECT MAX(position) FROM outbound_routes`).Scan(&maxPos)
		x.Position = int(maxPos.Int64) + 1
		res, err := r.DB.ExecContext(ctx, `INSERT INTO outbound_routes(name, position, patterns, trunk_ids, caller_id, enabled)
			VALUES (?,?,?,?,?,?)`, x.Name, x.Position, string(pats), string(tids), x.CallerID, b2i(x.Enabled))
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE outbound_routes SET name=?, patterns=?, trunk_ids=?, caller_id=?, enabled=?
		WHERE id=?`, x.Name, string(pats), string(tids), x.CallerID, b2i(x.Enabled), x.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderOutboundRoutes sets the order in which routes are tried.
func (r *Repo) ReorderOutboundRoutes(ctx context.Context, ids []int64) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE outbound_routes SET position = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteOutboundRoute removes an outbound route.
func (r *Repo) DeleteOutboundRoute(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "outbound_routes", id)
}
