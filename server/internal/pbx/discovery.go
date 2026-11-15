package pbx

import (
	"context"
)

// Discovery is a phone that fetched its configuration with the right provisioning credentials but
// whose MAC address is not known yet. An admin adopts it by assigning it to an extension.
type Discovery struct {
	MAC       string `json:"mac"`
	Vendor    string `json:"vendor"` // best guess from the file name and User-Agent ("" unknown)
	Model     string `json:"model"`  // best guess ("" unknown)
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
	File      string `json:"file"`
	Hits      int    `json:"hits"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
}

// maxDiscoveries caps the table: only phones with the provisioning password get here, but a
// misconfigured network should not fill the database.
const maxDiscoveries = 200

// RecordDiscovery stores or refreshes an unknown phone. A known vendor or model is never replaced
// by an unknown one (Grandstream and Htek share file names, the User-Agent tells them apart).
func (r *Repo) RecordDiscovery(ctx context.Context, d Discovery) {
	d.MAC = NormalizeMAC(d.MAC)
	if !reMAC.MatchString(d.MAC) {
		return
	}
	if len(d.UserAgent) > 200 {
		d.UserAgent = d.UserAgent[:200]
	}
	if len(d.File) > 100 {
		d.File = d.File[:100]
	}
	_, _ = r.DB.ExecContext(ctx, `INSERT INTO phone_discoveries(mac, vendor, model, ip, user_agent, file) VALUES (?,?,?,?,?,?)
		ON CONFLICT(mac) DO UPDATE SET
			vendor = CASE WHEN excluded.vendor != '' THEN excluded.vendor ELSE vendor END,
			model = CASE WHEN excluded.model != '' THEN excluded.model ELSE model END,
			ip = excluded.ip, user_agent = excluded.user_agent, file = excluded.file,
			hits = hits + 1, last_seen = strftime('%Y-%m-%dT%H:%M:%SZ','now')`,
		d.MAC, d.Vendor, d.Model, d.IP, d.UserAgent, d.File)
	_, _ = r.DB.ExecContext(ctx, `DELETE FROM phone_discoveries WHERE mac NOT IN
		(SELECT mac FROM phone_discoveries ORDER BY last_seen DESC LIMIT ?)`, maxDiscoveries)
}

// Discoveries lists unknown phones, newest first. MACs that were added as a phone in the meantime
// are left out.
func (r *Repo) Discoveries(ctx context.Context) ([]Discovery, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT mac, vendor, model, ip, user_agent, file, hits, first_seen, last_seen
		FROM phone_discoveries WHERE mac NOT IN (SELECT mac FROM phones) ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Discovery{}
	for rows.Next() {
		var d Discovery
		if err := rows.Scan(&d.MAC, &d.Vendor, &d.Model, &d.IP, &d.UserAgent, &d.File, &d.Hits, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Discovery returns one unknown phone.
func (r *Repo) Discovery(ctx context.Context, mac string) (*Discovery, error) {
	var d Discovery
	err := r.DB.QueryRowContext(ctx, `SELECT mac, vendor, model, ip, user_agent, file, hits, first_seen, last_seen
		FROM phone_discoveries WHERE mac = ?`, NormalizeMAC(mac)).
		Scan(&d.MAC, &d.Vendor, &d.Model, &d.IP, &d.UserAgent, &d.File, &d.Hits, &d.FirstSeen, &d.LastSeen)
	if err != nil {
		return nil, ErrNotFound
	}
	return &d, nil
}

// ForgetDiscovery removes an unknown phone (adopted or dismissed).
func (r *Repo) ForgetDiscovery(ctx context.Context, mac string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM phone_discoveries WHERE mac = ?`, NormalizeMAC(mac))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignPhone links a phone to an extension (nil: unassigned). The extension is taken out of the
// extra lines of the phone if it was one of them.
func (r *Repo) AssignPhone(ctx context.Context, id int64, ext *int64) (*Phone, error) {
	p, err := r.Phone(ctx, id)
	if err != nil {
		return nil, err
	}
	lines := []int64{}
	for _, l := range p.Lines {
		if ext == nil || l != *ext {
			lines = append(lines, l)
		}
	}
	linesJSON, _ := jsonMarshal(lines)
	var v any
	if ext != nil {
		v = *ext
	}
	if _, err := r.DB.ExecContext(ctx, `UPDATE phones SET extension_id = ?, lines = ? WHERE id = ?`, v, linesJSON, id); err != nil {
		return nil, err
	}
	return r.Phone(ctx, id)
}
