// Package journey records what happens to a call (Asterisk CEL events, the dialplan's own
// markers, dial and queue events) and turns those events into the steps shown in the call
// history: came in on a trunk, time condition, IVR choice, queue wait, answered by, transfers,
// voicemail and who hung up.
package journey

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Event is one stored event of a call. Kind is a CEL event name (CHAN_START, HANGUP...), a
// dialplan marker (kt_ivr, kt_queue...) or one of the AMI kinds below (DIAL, Q_CALL...).
type Event struct {
	ID       int64             `json:"-"`
	LinkedID string            `json:"-"`
	At       time.Time         `json:"at"`
	Kind     string            `json:"kind"`
	UniqueID string            `json:"uniqueId"`
	Channel  string            `json:"channel"`
	Data     map[string]string `json:"data,omitempty"`
}

// The CEL events worth keeping (cel.conf only sends these, this is a second filter).
var celKinds = map[string]bool{
	"CHAN_START": true, "ANSWER": true, "HANGUP": true, "BRIDGE_ENTER": true, "BRIDGE_EXIT": true, "PARK_START": true,
	"BLINDTRANSFER": true, "ATTENDEDTRANSFER": true, "PICKUP": true, "FORWARD": true, "LINKEDID_END": true,
}

// parseTime reads an AMI time: epoch seconds with microseconds ("1700000000.123456", the CEL and
// Timestamp format) or "2006-01-02 15:04:05[.000000]" in local time.
func parseTime(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 1e9 {
		sec := int64(f)
		return time.Unix(sec, int64((f-float64(sec))*1e6)*1000).UTC(), true
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// flatten turns the CEL Extra field (JSON) into strings. A CELGenUserEvent marker carries
// {"extra":"k=v&k2=v2"}; those pairs are unpacked.
func flatten(extra string, into map[string]string) {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(extra), &obj); err != nil {
		into["extra"] = extra
		return
	}
	for k, v := range obj {
		switch x := v.(type) {
		case string:
			if k == "extra" && strings.Contains(x, "=") {
				if q, err := url.ParseQuery(x); err == nil {
					for qk := range q {
						into[qk] = q.Get(qk)
					}
					continue
				}
			}
			into[k] = x
		case float64:
			into[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			into[k] = strconv.FormatBool(x)
		case nil:
		default:
			b, _ := json.Marshal(x)
			into[k] = string(b)
		}
	}
}

// FromAMI turns an AMI event into a journey event. ok is false for events that are not kept.
func FromAMI(get func(string) string, loc *time.Location) (Event, bool) {
	ev := Event{Data: map[string]string{}}
	at, ok := parseTime(get("Timestamp"), loc)
	if !ok {
		at = time.Now().UTC()
	}
	ev.At = at
	switch get("Event") {
	case "CEL":
		name := get("EventName")
		if !celKinds[name] && !strings.HasPrefix(name, "kt_") {
			return ev, false
		}
		ev.Kind, ev.LinkedID, ev.UniqueID, ev.Channel = name, get("LinkedID"), get("UniqueID"), get("Channel")
		if t, ok := parseTime(get("EventTime"), loc); ok {
			ev.At = t
		}
		if name == "CHAN_START" {
			ev.Data["num"], ev.Data["name"], ev.Data["exten"], ev.Data["context"] = get("CallerIDnum"), get("CallerIDname"), get("Exten"), get("Context")
		}
		if p := get("Peer"); p != "" {
			ev.Data["peer"] = p
		}
		flatten(get("Extra"), ev.Data)
	case "DialBegin", "DialEnd":
		ev.Kind = "DIAL"
		if get("Event") == "DialEnd" {
			ev.Kind = "DIALEND"
			ev.Data["status"] = get("DialStatus")
		}
		ev.LinkedID, ev.UniqueID, ev.Channel = get("Linkedid"), get("Uniqueid"), get("Channel")
		ev.Data["dest"], ev.Data["destId"] = get("DestChannel"), get("DestUniqueid")
		if ev.Data["dest"] == "" {
			return ev, false
		}
	case "QueueCallerJoin", "AgentCalled", "AgentRingNoAnswer", "AgentConnect", "AgentComplete", "QueueCallerAbandon":
		ev.Kind = map[string]string{"QueueCallerJoin": "Q_JOIN", "AgentCalled": "Q_CALL", "AgentRingNoAnswer": "Q_NOANSWER",
			"AgentConnect": "Q_CONNECT", "AgentComplete": "Q_DONE", "QueueCallerAbandon": "Q_ABANDON"}[get("Event")]
		ev.LinkedID, ev.UniqueID, ev.Channel = get("Linkedid"), get("Uniqueid"), get("Channel")
		ev.Data["queue"] = get("Queue")
		for _, k := range []string{"MemberName", "Interface", "Position", "HoldTime", "RingTime", "TalkTime", "Reason", "DestChannel", "DestUniqueid"} {
			if v := get(k); v != "" {
				ev.Data[strings.ToLower(k[:1])+k[1:]] = v
			}
		}
	default:
		return ev, false
	}
	if ev.LinkedID == "" {
		ev.LinkedID = ev.UniqueID
	}
	for k, v := range ev.Data {
		if v == "" {
			delete(ev.Data, k)
		}
	}
	return ev, ev.LinkedID != "" && ev.Kind != ""
}

// Store keeps the events in SQLite.
type Store struct{ DB *sql.DB }

// Insert stores events in one transaction.
func (s *Store) Insert(ctx context.Context, evs []Event) error {
	if len(evs) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `INSERT INTO call_events(linkedid, at, kind, uniqueid, channel, data) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range evs {
		data := "{}"
		if len(e.Data) > 0 {
			b, _ := json.Marshal(e.Data)
			data = string(b)
		}
		if _, err := st.ExecContext(ctx, e.LinkedID, e.At.UTC().Format(timeFmt), e.Kind, e.UniqueID, e.Channel, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// timeFmt sorts as text and keeps microseconds.
const timeFmt = "2006-01-02T15:04:05.000000Z"

// Events returns the events of one call in order.
func (s *Store) Events(ctx context.Context, linkedID string) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, linkedid, at, kind, uniqueid, channel, data FROM call_events
		WHERE linkedid = ? ORDER BY at, id LIMIT 5000`, linkedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var at, data string
		if err := rows.Scan(&e.ID, &e.LinkedID, &at, &e.Kind, &e.UniqueID, &e.Channel, &data); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(timeFmt, at)
		_ = json.Unmarshal([]byte(data), &e.Data)
		if e.Data == nil {
			e.Data = map[string]string{}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Has reports which of the linked ids have stored events (for the icon in the call list).
func (s *Store) Has(ctx context.Context, ids []string) map[string]bool {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `SELECT DISTINCT linkedid FROM call_events WHERE linkedid IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

// Purge removes the events of calls older than days (0 removes everything).
func (s *Store) Purge(ctx context.Context, days int, now time.Time) (int64, error) {
	cut := now.AddDate(0, 0, -days).UTC().Format(timeFmt)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM call_events WHERE at < ?`, cut)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Recorder buffers events and writes them in batches, so the AMI reader never waits on the disk.
type Recorder struct {
	Store *Store
	Log   *slog.Logger
	// Enabled is asked for every event; false (journeys switched off) drops it.
	Enabled func() bool
	ch      chan Event
}

// NewRecorder starts the writer; it stops when ctx ends.
func NewRecorder(ctx context.Context, st *Store, log *slog.Logger, enabled func() bool) *Recorder {
	r := &Recorder{Store: st, Log: log, Enabled: enabled, ch: make(chan Event, 8192)}
	go r.run(ctx)
	return r
}

// Add queues an event; it is dropped when the queue is full.
func (r *Recorder) Add(e Event) {
	if r == nil || (r.Enabled != nil && !r.Enabled()) {
		return
	}
	select {
	case r.ch <- e:
	default:
		r.Log.Warn("call journey queue full, event dropped")
	}
}

func (r *Recorder) run(ctx context.Context) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	var batch []Event
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := r.Store.Insert(c, batch); err != nil {
			r.Log.Warn("store call journey", "err", err, "events", len(batch))
		}
		cancel()
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case e := <-r.ch:
			batch = append(batch, e)
			if len(batch) >= 200 {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// String is used in logs and tests.
func (e Event) String() string {
	return fmt.Sprintf("%s %s %s %v", e.At.Format("15:04:05.000"), e.Kind, e.Channel, e.Data)
}
