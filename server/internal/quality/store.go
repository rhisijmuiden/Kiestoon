package quality

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const timeFmt = "2006-01-02T15:04:05Z"

// Store reads and writes the call_quality and quality_alerts tables.
type Store struct{ DB *sql.DB }

const legCols = `id, linkedid, uniqueid, channel, endpoint, kind, trunk_id, ext, codec, remote, started_at, ended_at,
	mos, mos_min, rx_mos, rx_jitter, rx_jitter_max, rx_loss, rx_reports, tx_mos, tx_jitter, tx_jitter_max, tx_loss, tx_reports,
	rtt, rtt_max, samples`

// Save stores a leg that ended.
func (s *Store) Save(ctx context.Context, l Leg) error {
	samples, _ := json.Marshal(l.Samples)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO call_quality(linkedid, uniqueid, channel, endpoint, kind, trunk_id, ext, codec, remote,
		started_at, ended_at, mos, mos_min, rx_mos, rx_jitter, rx_jitter_max, rx_loss, rx_reports, tx_mos, tx_jitter, tx_jitter_max,
		tx_loss, tx_reports, rtt, rtt_max, samples) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.LinkedID, l.UniqueID, l.Channel, l.Endpoint, l.Kind, l.TrunkID, l.Ext, l.Codec, l.Remote,
		l.Start.UTC().Format(timeFmt), l.End.UTC().Format(timeFmt), l.MOS, l.MOSMin,
		l.RX.MOS, l.RX.Jitter, l.RX.JitterMax, l.RX.Loss, l.RX.Reports, l.TX.MOS, l.TX.Jitter, l.TX.JitterMax, l.TX.Loss, l.TX.Reports,
		l.RTT, l.RTTMax, string(samples))
	return err
}

func scanLegs(rows *sql.Rows, withSamples bool) ([]Leg, error) {
	defer rows.Close()
	out := []Leg{}
	for rows.Next() {
		var l Leg
		var start, end, samples string
		if err := rows.Scan(&l.ID, &l.LinkedID, &l.UniqueID, &l.Channel, &l.Endpoint, &l.Kind, &l.TrunkID, &l.Ext, &l.Codec, &l.Remote,
			&start, &end, &l.MOS, &l.MOSMin, &l.RX.MOS, &l.RX.Jitter, &l.RX.JitterMax, &l.RX.Loss, &l.RX.Reports,
			&l.TX.MOS, &l.TX.Jitter, &l.TX.JitterMax, &l.TX.Loss, &l.TX.Reports, &l.RTT, &l.RTTMax, &samples); err != nil {
			return nil, err
		}
		l.Start, _ = time.Parse(timeFmt, start)
		l.End, _ = time.Parse(timeFmt, end)
		l.Samples = []int{}
		if withSamples {
			_ = json.Unmarshal([]byte(samples), &l.Samples)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Call returns the legs of one call, in the order they started.
func (s *Store) Call(ctx context.Context, linkedID string) ([]Leg, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+legCols+` FROM call_quality WHERE linkedid = ? ORDER BY started_at, id`, linkedID)
	if err != nil {
		return nil, err
	}
	return scanLegs(rows, true)
}

// Between returns the legs that ended in [from, to), without their timelines.
func (s *Store) Between(ctx context.Context, from, to time.Time) ([]Leg, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+legCols+` FROM call_quality WHERE ended_at >= ? AND ended_at < ? ORDER BY ended_at`,
		from.UTC().Format(timeFmt), to.UTC().Format(timeFmt))
	if err != nil {
		return nil, err
	}
	return scanLegs(rows, false)
}

// CallMOS returns the MOS of each of the given calls (the worst leg); calls without
// measurements are left out.
func (s *Store) CallMOS(ctx context.Context, linkedIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if len(linkedIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(linkedIDs))
	for i, id := range linkedIDs {
		args[i] = id
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT linkedid, MIN(mos) FROM call_quality WHERE mos > 0 AND linkedid IN (?`+
		strings.Repeat(",?", len(linkedIDs)-1)+`) GROUP BY linkedid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var m float64
		if err := rows.Scan(&id, &m); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// Purge removes the measurements and alerts older than days days.
func (s *Store) Purge(ctx context.Context, days int, now time.Time) (int64, error) {
	before := now.AddDate(0, 0, -days).UTC().Format(timeFmt)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM call_quality WHERE ended_at < ?`, before)
	if err != nil {
		return 0, err
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM quality_alerts WHERE ended_at != '' AND ended_at < ?`, before)
	return res.RowsAffected()
}

// Alert is a limit that was crossed, and for how long.
type Alert struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Kind  string `json:"kind"` // trunk, phone
	Ref   string `json:"ref"`  // trunk id or extension number
	Name  string `json:"name"`
	Calls int    `json:"calls"`
	// Poor is the number of poor calls (phone alerts).
	Poor    int     `json:"poor"`
	MOS     float64 `json:"mos"`
	Jitter  float64 `json:"jitter"`
	Loss    float64 `json:"loss"`
	RTT     float64 `json:"rtt"`
	Cause   string  `json:"cause"`
	Started string  `json:"started"`
	Ended   string  `json:"ended,omitempty"`
	Mailed  string  `json:"mailed,omitempty"`
}

const alertCols = `id, key, kind, ref, name, calls, poor, mos, jitter, loss, rtt, cause, started_at, ended_at, mailed_at`

func scanAlerts(rows *sql.Rows) ([]Alert, error) {
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Key, &a.Kind, &a.Ref, &a.Name, &a.Calls, &a.Poor, &a.MOS, &a.Jitter, &a.Loss, &a.RTT, &a.Cause,
			&a.Started, &a.Ended, &a.Mailed); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActiveAlerts returns the alerts that have not ended.
func (s *Store) ActiveAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+alertCols+` FROM quality_alerts WHERE ended_at = '' ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	return scanAlerts(rows)
}

// LastEnded returns when the last alert ended ("" when none did).
func (s *Store) LastEnded(ctx context.Context) string {
	var at string
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(ended_at), '') FROM quality_alerts WHERE ended_at != ''`).Scan(&at)
	return at
}

// LastMailed returns when an alert with this key was last mailed (zero when never).
func (s *Store) LastMailed(ctx context.Context, key string) time.Time {
	var at string
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(mailed_at), '') FROM quality_alerts WHERE key = ?`, key).Scan(&at)
	t, _ := time.Parse(timeFmt, at)
	return t
}

// Open stores a new alert and returns its id.
func (s *Store) Open(ctx context.Context, a Alert, now time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO quality_alerts(key, kind, ref, name, calls, poor, mos, jitter, loss, rtt, cause, started_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, a.Key, a.Kind, a.Ref, a.Name, a.Calls, a.Poor, a.MOS, a.Jitter, a.Loss, a.RTT, a.Cause,
		now.UTC().Format(timeFmt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update refreshes the figures of an active alert.
func (s *Store) Update(ctx context.Context, a Alert) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE quality_alerts SET name = ?, calls = ?, poor = ?, mos = ?, jitter = ?, loss = ?, rtt = ?, cause = ?
		WHERE id = ?`, a.Name, a.Calls, a.Poor, a.MOS, a.Jitter, a.Loss, a.RTT, a.Cause, a.ID)
	return err
}

// Close marks an alert as ended.
func (s *Store) Close(ctx context.Context, id int64, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE quality_alerts SET ended_at = ? WHERE id = ?`, now.UTC().Format(timeFmt), id)
	return err
}

// Mailed marks an alert as mailed.
func (s *Store) Mailed(ctx context.Context, id int64, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE quality_alerts SET mailed_at = ? WHERE id = ?`, now.UTC().Format(timeFmt), id)
	return err
}
