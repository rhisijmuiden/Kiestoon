package pbx

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Key is one programmable button on a phone or expansion module.
type Key struct {
	// Type is line (value: account number, "" for 1), blf (extension), speed (number), park (park
	// the current call), parkslot (watch and pick up a parking slot, 71-79), intercom (extension),
	// dnd, voicemail or none.
	Type  string `json:"type"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// KeyTypes are the key functions the editor offers.
var KeyTypes = []string{"none", "line", "blf", "speed", "park", "parkslot", "intercom", "dnd", "voicemail", "forward"}

// Module is an expansion module (sidecar) attached to a phone, such as a Yealink EXP50.
type Module struct {
	Type string `json:"type"`
	Keys []Key  `json:"keys"`
}

// IntercomOptions decide how the phone answers intercom calls (auto-answer).
type IntercomOptions struct {
	Allow bool `json:"allow"` // answer intercom calls automatically
	Mute  bool `json:"mute"`  // start with the microphone muted
	Tone  bool `json:"tone"`  // play a warning tone first
	Barge bool `json:"barge"` // also during another call
}

// HotdeskOptions make a phone a hot desk (flexplek): anyone can log in on it with *55, their
// extension number and PIN, and log out with *56. The phone's own extension is the guest account
// it uses while nobody is logged in.
type HotdeskOptions struct {
	Enabled bool `json:"enabled"`
	// AutoLogout logs everyone out at night (03:00), so desks start the day free.
	AutoLogout bool `json:"autoLogout"`
}

// PhoneOptions are per-phone settings beyond keys and lines.
type PhoneOptions struct {
	Intercom IntercomOptions `json:"intercom"`
	Hotdesk  HotdeskOptions  `json:"hotdesk"`
}

// DefaultPhoneOptions apply to phones that never had their options saved.
func DefaultPhoneOptions() PhoneOptions {
	return PhoneOptions{Intercom: IntercomOptions{Allow: true, Tone: true}}
}

// ModuleLimit is what one type of expansion module takes.
type ModuleLimit struct {
	Keys int // keys per module
	Max  int // modules per phone
}

// PhoneLimits are what a phone model supports; the API fills them in from the model list.
type PhoneLimits struct {
	Keys    int
	Lines   int
	Modules map[string]ModuleLimit
}

// Phone is a desk phone that fetches its configuration from Kiestoon.
type Phone struct {
	ID          int64  `json:"id"`
	MAC         string `json:"mac"`
	Vendor      string `json:"vendor"`
	Model       string `json:"model"`
	ExtensionID *int64 `json:"extensionId"`
	Label       string `json:"label"`
	Keys        []Key  `json:"keys"`
	// Lines are extra extensions on the same device: handsets 2 and up of a DECT base (Gigaset)
	// or accounts 2 and up of a desk phone.
	Lines      []int64 `json:"lines"`
	LastSeenAt string  `json:"lastSeenAt"`
	LastIP     string  `json:"lastIp"`
	UserAgent  string  `json:"userAgent"`
	// ZTP is the state at the vendor's redirection service (registered, error, elsewhere; "" unknown).
	ZTPState string `json:"ztpState"`
	ZTPError string `json:"ztpError"`
	ZTPAt    string `json:"ztpAt"`
	// Modules are the expansion modules in the order they are connected.
	Modules []Module     `json:"modules"`
	Options PhoneOptions `json:"options"`
	// HotdeskExtensionID is the extension logged in on this hot desk (nil: nobody), since HotdeskAt.
	// It is changed with SetHotdesk only, never by saving the phone.
	HotdeskExtensionID *int64 `json:"hotdeskExtensionId"`
	HotdeskAt          string `json:"hotdeskAt"`
}

// ActiveExtensionID is the extension the phone carries now: the one logged in on a hot desk, else
// its own extension.
func (p *Phone) ActiveExtensionID() *int64 {
	if p.Options.Hotdesk.Enabled && p.HotdeskExtensionID != nil {
		return p.HotdeskExtensionID
	}
	return p.ExtensionID
}

var reMAC = regexp.MustCompile(`^[0-9a-f]{12}$`)

// AdminPassword returns the phone's own admin password and when it was set ("" when it has none).
// It is kept out of Phone so it never travels with the phone list.
func (r *Repo) AdminPassword(ctx context.Context, id int64) (string, string, error) {
	var pw, at string
	err := r.DB.QueryRowContext(ctx, `SELECT admin_password, admin_password_at FROM phones WHERE id = ?`, id).Scan(&pw, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return pw, at, err
}

// SetAdminPassword stores a new admin password for a phone.
func (r *Repo) SetAdminPassword(ctx context.Context, id int64, pw string) (string, error) {
	at := time.Now().UTC().Format(time.RFC3339)
	res, err := r.DB.ExecContext(ctx, `UPDATE phones SET admin_password = ?, admin_password_at = ? WHERE id = ?`, pw, at, id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return at, nil
}

// AdminPasswordSent records which admin password the phone received (as a hash).
func (r *Repo) AdminPasswordSent(ctx context.Context, id int64, pw string) {
	_, _ = r.DB.ExecContext(ctx, `UPDATE phones SET admin_password_sent = ? WHERE id = ?`, hashAdminPassword(pw), id)
}

// AdminPasswordReceived reports whether the phone last received this admin password.
func (r *Repo) AdminPasswordReceived(ctx context.Context, id int64, pw string) bool {
	var sent string
	_ = r.DB.QueryRowContext(ctx, `SELECT admin_password_sent FROM phones WHERE id = ?`, id).Scan(&sent)
	return sent != "" && sent == hashAdminPassword(pw)
}

func hashAdminPassword(pw string) string {
	sum := sha256.Sum256([]byte("kiestoon-admin-password:" + pw))
	return hex.EncodeToString(sum[:])
}

// NormalizeMAC turns "00:15:65:AB:CD:EF" into "001565abcdef".
func NormalizeMAC(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if r == ':' || r == '-' || r == '.' || r == ' ' {
			return -1
		}
		return r
	}, s))
}

const phoneCols = `id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, lines, ztp_state, ztp_error, ztp_at, modules, options, hotdesk_ext, hotdesk_at`

func scanPhone(row interface{ Scan(...any) error }) (*Phone, error) {
	var p Phone
	var ext, hd sql.NullInt64
	var keys, lines, modules, options string
	err := row.Scan(&p.ID, &p.MAC, &p.Vendor, &p.Model, &ext, &p.Label, &keys, &p.LastSeenAt, &p.LastIP, &p.UserAgent, &lines, &p.ZTPState, &p.ZTPError, &p.ZTPAt, &modules, &options, &hd, &p.HotdeskAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if ext.Valid {
		p.ExtensionID = &ext.Int64
	}
	if hd.Valid {
		p.HotdeskExtensionID = &hd.Int64
	}
	_ = json.Unmarshal([]byte(keys), &p.Keys)
	if p.Keys == nil {
		p.Keys = []Key{}
	}
	_ = json.Unmarshal([]byte(lines), &p.Lines)
	if p.Lines == nil {
		p.Lines = []int64{}
	}
	_ = json.Unmarshal([]byte(modules), &p.Modules)
	if p.Modules == nil {
		p.Modules = []Module{}
	}
	for i := range p.Modules {
		if p.Modules[i].Keys == nil {
			p.Modules[i].Keys = []Key{}
		}
	}
	p.Options = DefaultPhoneOptions()
	_ = json.Unmarshal([]byte(options), &p.Options)
	return &p, err
}

// Vendors are the phone brands Kiestoon provisions.
var Vendors = []string{"yealink", "snom", "grandstream", "fanvil", "gigaset", "poly", "htek", "akuvox", "cisco"}

// SetPhoneZTP stores the result of a registration at the vendor's redirection service.
func (r *Repo) SetPhoneZTP(ctx context.Context, id int64, state, msg string) {
	if len(msg) > 300 {
		msg = msg[:300]
	}
	_, _ = r.DB.ExecContext(ctx, `UPDATE phones SET ztp_state = ?, ztp_error = ?, ztp_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`, state, msg, id)
}

// Phones returns all phones.
func (r *Repo) Phones(ctx context.Context) ([]Phone, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+phoneCols+` FROM phones ORDER BY label, mac`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Phone{}
	for rows.Next() {
		p, err := scanPhone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Phone returns one phone by ID.
func (r *Repo) Phone(ctx context.Context, id int64) (*Phone, error) {
	return scanPhone(r.DB.QueryRowContext(ctx, `SELECT `+phoneCols+` FROM phones WHERE id = ?`, id))
}

// PhoneByMAC returns the phone with this MAC address.
func (r *Repo) PhoneByMAC(ctx context.Context, mac string) (*Phone, error) {
	return scanPhone(r.DB.QueryRowContext(ctx, `SELECT `+phoneCols+` FROM phones WHERE mac = ?`, NormalizeMAC(mac)))
}

var (
	reKeyNumber = regexp.MustCompile(`^[0-9*#+]{1,20}$`)
	reKeyExt    = regexp.MustCompile(`^[0-9]{2,8}$`)
	reParkSlot  = regexp.MustCompile(`^7[1-9]$`)
	// reKeyForward is the external number a forward key forwards to.
	reKeyForward = regexp.MustCompile(`^\+?[0-9]{2,20}$`)
)

// CleanKeys checks and normalises a key layout of at most max keys on a phone with lines accounts.
func CleanKeys(keys []Key, max, lines int) ([]Key, error) {
	if keys == nil {
		keys = []Key{}
	}
	if len(keys) > max {
		keys = keys[:max]
	}
	out := make([]Key, len(keys))
	for i, k := range keys {
		k.Label = cleanText(k.Label, 20)
		k.Value = strings.TrimSpace(k.Value)
		switch k.Type {
		case "", "none":
			k = Key{Type: "none"}
		case "dnd", "voicemail", "park":
			k.Value = ""
		case "line":
			if k.Value == "1" {
				k.Value = ""
			}
			if k.Value != "" {
				n := 0
				for _, c := range k.Value {
					if c < '0' || c > '9' {
						return nil, invalid("invalid", "keys")
					}
					n = n*10 + int(c-'0')
					if n > 99 {
						break
					}
				}
				if n < 1 || n > lines {
					return nil, invalid("invalid", "keys")
				}
			}
		case "blf", "speed":
			if !reKeyNumber.MatchString(k.Value) {
				return nil, invalid("invalid", "keys")
			}
		case "intercom":
			if !reKeyExt.MatchString(k.Value) {
				return nil, invalid("invalid", "keys")
			}
		case "parkslot":
			if !reParkSlot.MatchString(k.Value) {
				return nil, invalid("invalid", "keys")
			}
		case "forward":
			k.Value = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(k.Value)
			if !reKeyForward.MatchString(k.Value) {
				return nil, invalid("invalid", "keys")
			}
		default:
			return nil, invalid("invalid", "keys")
		}
		out[i] = k
	}
	return out, nil
}

// CleanModules checks the expansion modules of a phone: one type, at most its maximum.
func CleanModules(mods []Module, lim PhoneLimits) ([]Module, error) {
	if len(mods) == 0 {
		return []Module{}, nil
	}
	out := make([]Module, len(mods))
	for i, m := range mods {
		ml, ok := lim.Modules[m.Type]
		if !ok || m.Type != mods[0].Type || len(mods) > ml.Max {
			return nil, invalid("invalid", "modules")
		}
		keys, err := CleanKeys(m.Keys, ml.Keys, lim.Lines)
		if err != nil {
			return nil, invalid("invalid", "modules")
		}
		out[i] = Module{Type: m.Type, Keys: keys}
	}
	return out, nil
}

// SavePhone validates and stores a phone against the limits of its model.
func (r *Repo) SavePhone(ctx context.Context, p *Phone, lim PhoneLimits) error {
	p.MAC = NormalizeMAC(p.MAC)
	p.Label = cleanText(p.Label, 40)
	if !reMAC.MatchString(p.MAC) {
		return invalid("invalid", "mac")
	}
	if !slices.Contains(Vendors, p.Vendor) {
		return invalid("invalid", "vendor")
	}
	if p.Lines == nil {
		p.Lines = []int64{}
	}
	seen := map[int64]bool{}
	if p.ExtensionID != nil {
		seen[*p.ExtensionID] = true
	}
	lines := []int64{}
	for _, id := range p.Lines {
		if id > 0 && !seen[id] {
			seen[id] = true
			lines = append(lines, id)
		}
	}
	if lim.Lines < 1 {
		lim.Lines = 1
	}
	if len(lines) > lim.Lines-1 {
		return invalid("invalid", "lines")
	}
	p.Lines = lines
	var err error
	if p.Keys, err = CleanKeys(p.Keys, lim.Keys, lim.Lines); err != nil {
		return err
	}
	if p.Modules, err = CleanModules(p.Modules, lim); err != nil {
		return err
	}
	keys, _ := json.Marshal(p.Keys)
	linesJSON, _ := json.Marshal(p.Lines)
	modules, _ := json.Marshal(p.Modules)
	options, _ := json.Marshal(p.Options)
	var ext any
	if p.ExtensionID != nil {
		ext = *p.ExtensionID
	}
	if p.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO phones(mac, vendor, model, extension_id, label, keys, lines, modules, options) VALUES (?,?,?,?,?,?,?,?,?)`,
			p.MAC, p.Vendor, p.Model, ext, p.Label, string(keys), string(linesJSON), string(modules), string(options))
		if err != nil {
			return uniqueErr(err, "mac")
		}
		p.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE phones SET mac=?, vendor=?, model=?, extension_id=?, label=?, keys=?, lines=?, modules=?, options=? WHERE id=?`,
		p.MAC, p.Vendor, p.Model, ext, p.Label, string(keys), string(linesJSON), string(modules), string(options), p.ID)
	if err != nil {
		return uniqueErr(err, "mac")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetHotdesk logs ext in on hot desk phone id (nil logs out). An extension is logged in on one
// hot desk at a time: logging in elsewhere logs it out here. It returns the phones that were
// logged out that way.
func (r *Repo) SetHotdesk(ctx context.Context, id int64, ext *int64) ([]int64, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var others []int64
	if ext != nil {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM phones WHERE hotdesk_ext = ? AND id <> ?`, *ext, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var o int64
			if rows.Scan(&o) == nil {
				others = append(others, o)
			}
		}
		rows.Close()
		if _, err := tx.ExecContext(ctx, `UPDATE phones SET hotdesk_ext = NULL, hotdesk_at = '' WHERE hotdesk_ext = ? AND id <> ?`, *ext, id); err != nil {
			return nil, err
		}
	}
	var v any
	at := ""
	if ext != nil {
		v, at = *ext, time.Now().UTC().Format(time.RFC3339)
	}
	res, err := tx.ExecContext(ctx, `UPDATE phones SET hotdesk_ext = ?, hotdesk_at = ? WHERE id = ?`, v, at, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return others, tx.Commit()
}

// DeletePhone removes a phone.
func (r *Repo) DeletePhone(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "phones", id)
}

// TouchPhone records that a phone fetched its configuration.
func (r *Repo) TouchPhone(ctx context.Context, id int64, ip, ua string) {
	if len(ua) > 200 {
		ua = ua[:200]
	}
	_, _ = r.DB.ExecContext(ctx, `UPDATE phones SET last_seen_at = strftime('%Y-%m-%dT%H:%M:%SZ','now'), last_ip = ?, user_agent = ? WHERE id = ?`, ip, ua, id)
}

func renumberKeys(keys []Key, m map[string]string) bool {
	changed := false
	for i := range keys {
		switch keys[i].Type {
		case "blf", "speed", "intercom":
			if to, ok := m[keys[i].Value]; ok {
				keys[i].Value = to
				changed = true
			}
		}
	}
	return changed
}

func renumberPhoneKeys(ctx context.Context, tx *sql.Tx, changes []Renumber) error {
	m := map[string]string{}
	for _, c := range changes {
		m[c.From] = c.To
	}
	type upd struct {
		table, keys, modules string
		id                   int64
	}
	var ups []upd
	for _, table := range []string{"phones", "key_templates"} {
		rows, err := tx.QueryContext(ctx, `SELECT id, keys, modules FROM `+table)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var rawKeys, rawMods string
			if err := rows.Scan(&id, &rawKeys, &rawMods); err != nil {
				rows.Close()
				return err
			}
			var keys []Key
			var mods []Module
			_ = json.Unmarshal([]byte(rawKeys), &keys)
			_ = json.Unmarshal([]byte(rawMods), &mods)
			changed := renumberKeys(keys, m)
			for i := range mods {
				changed = renumberKeys(mods[i].Keys, m) || changed
			}
			if changed {
				k, _ := json.Marshal(keys)
				md, _ := json.Marshal(mods)
				ups = append(ups, upd{table, string(k), string(md), id})
			}
		}
		rows.Close()
	}
	for _, u := range ups {
		if _, err := tx.ExecContext(ctx, `UPDATE `+u.table+` SET keys = ?, modules = ? WHERE id = ?`, u.keys, u.modules, u.id); err != nil {
			return err
		}
	}
	return nil
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// ForwardTargets lists the numbers the forward keys on these phones forward to, sorted and
// without duplicates. The dialplan makes a toggle code with a BLF hint for each of them.
func ForwardTargets(phones []Phone) []string {
	seen := map[string]bool{}
	add := func(keys []Key) {
		for _, k := range keys {
			if k.Type == "forward" && reKeyForward.MatchString(k.Value) {
				seen[k.Value] = true
			}
		}
	}
	for _, p := range phones {
		add(p.Keys)
		for _, m := range p.Modules {
			add(m.Keys)
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// HasForwardKey reports whether the phone has a forward key (its targets go into the dialplan).
func (p *Phone) HasForwardKey() bool {
	return len(ForwardTargets([]Phone{*p})) > 0
}
