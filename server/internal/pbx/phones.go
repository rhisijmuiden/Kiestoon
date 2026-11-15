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

// Key is one programmable button on a phone.
type Key struct {
	Type  string `json:"type"` // line | blf | speed | none
	Value string `json:"value"`
	Label string `json:"label"`
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

const phoneCols = `id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, lines`

func scanPhone(row interface{ Scan(...any) error }) (*Phone, error) {
	var p Phone
	var ext sql.NullInt64
	var keys, lines string
	err := row.Scan(&p.ID, &p.MAC, &p.Vendor, &p.Model, &ext, &p.Label, &keys, &p.LastSeenAt, &p.LastIP, &p.UserAgent, &lines)
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
	return &p, err
}

// Vendors are the phone brands Kiestoon provisions.
var Vendors = []string{"yealink", "snom", "grandstream", "fanvil", "gigaset", "poly"}

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

// SavePhone validates and stores a phone. maxKeys is the number of keys of the model and maxLines
// the number of accounts (or DECT handsets) it has.
func (r *Repo) SavePhone(ctx context.Context, p *Phone, maxKeys, maxLines int) error {
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
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines-1 {
		return invalid("invalid", "lines")
	}
	p.Lines = lines
	if p.Keys == nil {
		p.Keys = []Key{}
	}
	if len(p.Keys) > maxKeys {
		p.Keys = p.Keys[:maxKeys]
	}
	for i := range p.Keys {
		k := &p.Keys[i]
		k.Label = cleanText(k.Label, 20)
		k.Value = strings.TrimSpace(k.Value)
		switch k.Type {
		case "none", "line":
			k.Value = ""
		case "blf", "speed":
			if !regexp.MustCompile(`^[0-9*#+]{1,20}$`).MatchString(k.Value) {
				return invalid("invalid", "keys")
			}
		default:
			return invalid("invalid", "keys")
		}
	}
	keys, _ := json.Marshal(p.Keys)
	linesJSON, _ := json.Marshal(p.Lines)
	var ext any
	if p.ExtensionID != nil {
		ext = *p.ExtensionID
	}
	if p.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO phones(mac, vendor, model, extension_id, label, keys, lines) VALUES (?,?,?,?,?,?,?)`,
			p.MAC, p.Vendor, p.Model, ext, p.Label, string(keys), string(linesJSON))
		if err != nil {
			return uniqueErr(err, "mac")
		}
		p.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE phones SET mac=?, vendor=?, model=?, extension_id=?, label=?, keys=?, lines=? WHERE id=?`,
		p.MAC, p.Vendor, p.Model, ext, p.Label, string(keys), string(linesJSON), p.ID)
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

func renumberPhoneKeys(ctx context.Context, tx *sql.Tx, changes []Renumber) error {
	m := map[string]string{}
	for _, c := range changes {
		m[c.From] = c.To
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, keys FROM phones`)
	if err != nil {
		return err
	}
	type upd struct {
		id   int64
		keys string
	}
	var ups []upd
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var keys []Key
		_ = json.Unmarshal([]byte(raw), &keys)
		changed := false
		for i := range keys {
			if to, ok := m[keys[i].Value]; ok && (keys[i].Type == "blf" || keys[i].Type == "speed") {
				keys[i].Value = to
				changed = true
			}
		}
		if changed {
			b, _ := json.Marshal(keys)
			ups = append(ups, upd{id, string(b)})
		}
	}
	rows.Close()
	for _, u := range ups {
		if _, err := tx.ExecContext(ctx, `UPDATE phones SET keys = ? WHERE id = ?`, u.keys, u.id); err != nil {
			return err
		}
	}
	return nil
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
