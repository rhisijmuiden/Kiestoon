// Package qstats keeps one row per caller in a queue, for the wallboard figures of today and the
// scheduled reports. It is filled from the queue events of Asterisk and does not depend on the
// call journeys, which can be switched off.
package qstats

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Results of a queue call.
const (
	Answered  = "answered"
	Abandoned = "abandoned"
	// Left is a caller that left the queue without being answered and without hanging up: the
	// maximum wait time passed, the queue had no agents or the caller asked to be called back.
	Left = "left"
)

// ShortAbandon is the wait (seconds) under which a caller who hangs up is not counted against the
// service level: they most likely dialled the wrong number.
const ShortAbandon = 5

const timeFmt = "2006-01-02T15:04:05Z"

// Store reads and writes the queue_calls table.
type Store struct{ DB *sql.DB }

var (
	queueRe       = regexp.MustCompile(`^kiestoon-q-([0-9]+)$`)
	placeholderRe = regexp.MustCompile(`^Local/[0-9]+@kiestoon-qcbwait-`)
)

// AgentNumber returns the extension number of a queue member interface (Local/201@kiestoon-ring/n).
func AgentNumber(iface string) string {
	n := strings.TrimPrefix(iface, "Local/")
	if i := strings.IndexAny(n, "@/"); i > 0 {
		n = n[:i]
	}
	return n
}

// QueueID returns the queue id of a Kiestoon queue name, 0 for other queues.
func QueueID(name string) int64 {
	m := queueRe.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// Event records one AMI queue event. get returns the fields of the event.
func (s *Store) Event(ctx context.Context, get func(string) string, now time.Time) error {
	qid := QueueID(get("Queue"))
	if qid == 0 {
		return nil
	}
	uid := get("Uniqueid")
	at := now.UTC().Format(timeFmt)
	atoi := func(k string) int { n, _ := strconv.Atoi(get(k)); return n }
	// The row of this caller in this queue that is still open (or only marked as left: Asterisk
	// may report the leave before the connect or the abandon).
	const open = `(SELECT id FROM queue_calls WHERE uniqueid = ? AND queue_id = ? AND result IN ('', 'left') ORDER BY id DESC LIMIT 1)`
	var err error
	switch get("Event") {
	case "QueueCallerJoin":
		cb := 0
		if placeholderRe.MatchString(get("Channel")) {
			cb = 1
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO queue_calls(queue_id, uniqueid, linkedid, caller, callback, joined_at)
			VALUES (?,?,?,?,?,?)`, qid, uid, get("Linkedid"), get("CallerIDNum"), cb, at)
	case "AgentConnect":
		_, err = s.DB.ExecContext(ctx, `UPDATE queue_calls SET result = 'answered', hold = ?, agent = ?, agent_name = ?, ended_at = ''
			WHERE id = `+open, atoi("HoldTime"), AgentNumber(get("Interface")), get("MemberName"), uid, qid)
	case "AgentComplete":
		_, err = s.DB.ExecContext(ctx, `UPDATE queue_calls SET talk = ?, ended_at = ? WHERE id = (SELECT id FROM queue_calls
			WHERE uniqueid = ? AND queue_id = ? AND result = 'answered' AND ended_at = '' ORDER BY id DESC LIMIT 1)`,
			atoi("TalkTime"), at, uid, qid)
	case "QueueCallerAbandon":
		_, err = s.DB.ExecContext(ctx, `UPDATE queue_calls SET result = 'abandoned', hold = ?, ended_at = ? WHERE id = `+open,
			atoi("HoldTime"), at, uid, qid)
	case "QueueCallerLeave":
		_, err = s.DB.ExecContext(ctx, `UPDATE queue_calls SET result = 'left', ended_at = ?,
			hold = CAST(strftime('%s', ?) - strftime('%s', joined_at) AS INTEGER)
			WHERE id = (SELECT id FROM queue_calls WHERE uniqueid = ? AND queue_id = ? AND result = '' ORDER BY id DESC LIMIT 1)`,
			at, at, uid, qid)
	}
	return err
}

// Call is one caller in a queue.
type Call struct {
	QueueID   int64
	Joined    time.Time
	Result    string // "" while waiting or talking
	Hold      int
	Talk      int
	Agent     string
	AgentName string
	Callback  bool // a placeholder for a scheduled callback
}

// Calls returns the queue calls that joined in [from, to).
func (s *Store) Calls(ctx context.Context, from, to time.Time) ([]Call, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT queue_id, joined_at, result, hold, talk, agent, agent_name, callback FROM queue_calls
		WHERE joined_at >= ? AND joined_at < ? ORDER BY joined_at`, from.UTC().Format(timeFmt), to.UTC().Format(timeFmt))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Call{}
	for rows.Next() {
		var c Call
		var at string
		var cb int
		if err := rows.Scan(&c.QueueID, &at, &c.Result, &c.Hold, &c.Talk, &c.Agent, &c.AgentName, &cb); err != nil {
			return nil, err
		}
		c.Joined, _ = time.Parse(timeFmt, at)
		c.Callback = cb == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// Purge removes the rows older than days days.
func (s *Store) Purge(ctx context.Context, days int, now time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM queue_calls WHERE joined_at < ?`, now.AddDate(0, 0, -days).UTC().Format(timeFmt))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Totals are the figures of a set of queue calls.
type Totals struct {
	// Offered counts the callers that joined (callbacks not included).
	Offered   int `json:"offered"`
	Answered  int `json:"answered"`
	Abandoned int `json:"abandoned"`
	// ShortAbandoned are abandoned calls that waited less than ShortAbandon seconds.
	ShortAbandoned int `json:"shortAbandoned"`
	Left           int `json:"left"`
	// Callbacks counts the callbacks that reached an agent.
	Callbacks int `json:"callbacks"`
	// InTarget counts the answered calls that waited no longer than the target.
	InTarget    int `json:"inTarget"`
	AvgWait     int `json:"avgWait"`
	AvgTalk     int `json:"avgTalk"`
	LongestWait int `json:"longestWait"`
	// ServiceLevel is the percentage answered within the target, of the answered and abandoned
	// calls (short abandons not counted); -1 when there were none.
	ServiceLevel float64 `json:"serviceLevel"`
	// AbandonedPct is the share of offered calls that hung up (short abandons not counted); -1 when none.
	AbandonedPct float64 `json:"abandonedPct"`

	waitSum, talkSum, talkN int
}

// Add counts one call. target is the service level target (seconds) of its queue.
func (t *Totals) Add(c Call, target int) {
	if c.Callback {
		if c.Result == Answered {
			t.Callbacks++
		}
		return
	}
	t.Offered++
	switch c.Result {
	case Answered:
		t.Answered++
		t.waitSum += c.Hold
		if c.Hold <= target {
			t.InTarget++
		}
		if c.Talk > 0 {
			t.talkSum += c.Talk
			t.talkN++
		}
	case Abandoned:
		t.Abandoned++
		if c.Hold < ShortAbandon {
			t.ShortAbandoned++
		}
	case Left:
		t.Left++
	}
	if c.Result != "" && c.Hold > t.LongestWait {
		t.LongestWait = c.Hold
	}
}

// Finish computes the averages and percentages.
func (t *Totals) Finish() {
	t.ServiceLevel, t.AbandonedPct = -1, -1
	if t.Answered > 0 {
		t.AvgWait = t.waitSum / t.Answered
	}
	if t.talkN > 0 {
		t.AvgTalk = t.talkSum / t.talkN
	}
	counted := t.Abandoned - t.ShortAbandoned
	if d := t.Answered + counted; d > 0 {
		t.ServiceLevel = float64(t.InTarget) * 100 / float64(d)
	}
	if d := t.Offered - t.ShortAbandoned; d > 0 {
		t.AbandonedPct = float64(counted) * 100 / float64(d)
	}
}

// Hour is the number of calls answered and abandoned in one hour of the day.
type Hour struct {
	Answered  int `json:"answered"`
	Abandoned int `json:"abandoned"`
}

// Agent are the figures of one agent.
type Agent struct {
	Number  string `json:"number"`
	Name    string `json:"name"`
	Calls   int    `json:"calls"`
	AvgTalk int    `json:"avgTalk"`

	talkSum, talkN int
}

// Summary sums up calls per queue, per hour (in loc) and per agent.
type Summary struct {
	Queues map[int64]*Totals
	Hours  map[int64]*[24]Hour
	Agents map[string]*Agent
}

// Summarize sums up calls. target returns the service level target of a queue.
func Summarize(calls []Call, target func(queueID int64) int, loc *time.Location) Summary {
	s := Summary{Queues: map[int64]*Totals{}, Hours: map[int64]*[24]Hour{}, Agents: map[string]*Agent{}}
	for _, c := range calls {
		t := s.Queues[c.QueueID]
		if t == nil {
			t = &Totals{}
			s.Queues[c.QueueID] = t
			s.Hours[c.QueueID] = &[24]Hour{}
		}
		t.Add(c, target(c.QueueID))
		h := c.Joined.In(loc).Hour()
		switch {
		case c.Result == Answered && !c.Callback:
			s.Hours[c.QueueID][h].Answered++
		case c.Result == Abandoned && c.Hold >= ShortAbandon:
			s.Hours[c.QueueID][h].Abandoned++
		}
		if c.Result == Answered && c.Agent != "" {
			a := s.Agents[c.Agent]
			if a == nil {
				a = &Agent{Number: c.Agent, Name: c.AgentName}
				s.Agents[c.Agent] = a
			}
			a.Calls++
			if c.Talk > 0 {
				a.talkSum += c.Talk
				a.talkN++
				a.AvgTalk = a.talkSum / a.talkN
			}
		}
	}
	for _, t := range s.Queues {
		t.Finish()
	}
	return s
}

// Total sums up calls of any queues into one set of figures.
func Total(calls []Call, target func(queueID int64) int) Totals {
	var t Totals
	for _, c := range calls {
		t.Add(c, target(c.QueueID))
	}
	t.Finish()
	return t
}
