package pbx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
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
var KeyTypes = []string{"none", "line", "blf", "speed", "park", "parkslot", "intercom", "dnd", "voicemail"}

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

// PhoneOptions are per-phone settings beyond keys and lines.
type PhoneOptions struct {
	Intercom IntercomOptions `json:"intercom"`
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
}

var reMAC = regexp.MustCompile(`^[0-9a-f]{12}$`)

// NormalizeMAC turns "00:15:65:AB:CD:EF" into "001565abcdef".
func NormalizeMAC(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if r == ':' || r == '-' || r == '.' || r == ' ' {
			return -1
		}
		return r
	}, s))
}

const phoneCols = `id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, lines, ztp_state, ztp_error, ztp_at, modules, options`

func scanPhone(row interface{ Scan(...any) error }) (*Phone, error) {
	var p Phone
	var ext sql.NullInt64
	var keys, lines, modules, options string
	err := row.Scan(&p.ID, &p.MAC, &p.Vendor, &p.Model, &ext, &p.Label, &keys, &p.LastSeenAt, &p.LastIP, &p.UserAgent, &lines, &p.ZTPState, &p.ZTPError, &p.ZTPAt, &modules, &options)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if ext.Valid {
		p.ExtensionID = &ext.Int64
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
