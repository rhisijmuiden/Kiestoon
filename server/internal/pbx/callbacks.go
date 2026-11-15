package pbx

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Callback statuses. A callback is scheduled until a placeholder call waits for it in the
// queue (waiting); when an agent answers, the caller is called (calling). It ends as done,
// failed (no answer after all attempts, or expired) or cancelled.
const (
	CallbackScheduled = "scheduled"
	CallbackWaiting   = "waiting"
	CallbackCalling   = "calling"
	CallbackDone      = "done"
	CallbackFailed    = "failed"
	CallbackCancelled = "cancelled"
)

// Callback is a caller who asked to be called back by a queue.
type Callback struct {
	ID        int64  `json:"id"`
	QueueID   int64  `json:"queueId"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	Agent     string `json:"agent"`
	Linkedid  string `json:"linkedid"`
	Note      string `json:"note"`
	NextAt    string `json:"nextAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

const callbackCols = `id, queue_id, number, name, position, status, attempts, agent, linkedid, note, next_at, created_at, updated_at`

func scanCallback(row interface{ Scan(...any) error }) (*Callback, error) {
	var c Callback
	err := row.Scan(&c.ID, &c.QueueID, &c.Number, &c.Name, &c.Position, &c.Status, &c.Attempts, &c.Agent, &c.Linkedid,
		&c.Note, &c.NextAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func ts(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// AddCallback stores a new callback request, scheduled right away.
func (r *Repo) AddCallback(ctx context.Context, c *Callback) error {
	c.Number = cleanText(c.Number, 25)
	c.Name = cleanText(c.Name, 60)
	if c.Number == "" {
		return invalid("required", "number")
	}
	now := ts(time.Now())
	res, err := r.DB.ExecContext(ctx, `INSERT INTO queue_callbacks(queue_id, number, name, position, status, linkedid, next_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, c.QueueID, c.Number, c.Name, c.Position, CallbackScheduled, c.Linkedid, now, now, now)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.Status, c.NextAt, c.CreatedAt, c.UpdatedAt = CallbackScheduled, now, now, now
	return nil
}

// Callback returns one callback.
func (r *Repo) Callback(ctx context.Context, id int64) (*Callback, error) {
	return scanCallback(r.DB.QueryRowContext(ctx, `SELECT `+callbackCols+` FROM queue_callbacks WHERE id = ?`, id))
}

// Callbacks lists the open callbacks and those of the last day.
func (r *Repo) Callbacks(ctx context.Context) ([]Callback, error) {
	since := ts(time.Now().Add(-24 * time.Hour))
	return queryAll(ctx, r.DB, `SELECT `+callbackCols+` FROM queue_callbacks
		WHERE status IN ('scheduled','waiting','calling') OR updated_at >= ? ORDER BY id DESC LIMIT 500`, scanCallback, since)
}

// DueCallbacks are scheduled callbacks whose time has come.
func (r *Repo) DueCallbacks(ctx context.Context, now time.Time) ([]Callback, error) {
	return queryAll(ctx, r.DB, `SELECT `+callbackCols+` FROM queue_callbacks WHERE status = 'scheduled' AND next_at <= ? ORDER BY id`,
		scanCallback, ts(now))
}

// OpenCallbacks are callbacks in progress (scheduled, waiting or calling).
func (r *Repo) OpenCallbacks(ctx context.Context) ([]Callback, error) {
	return queryAll(ctx, r.DB, `SELECT `+callbackCols+` FROM queue_callbacks WHERE status IN ('scheduled','waiting','calling') ORDER BY id`,
		scanCallback)
}

// UpdateCallback stores a new status. Only the fields given are changed: attempts is added to
// the count, next is used when not zero, agent and note when not "".
func (r *Repo) UpdateCallback(ctx context.Context, id int64, from []string, status string, attempts int, next time.Time, agent, note string) (bool, error) {
	q := `UPDATE queue_callbacks SET status = ?, attempts = attempts + ?, updated_at = ?`
	args := []any{status, attempts, ts(time.Now())}
	if !next.IsZero() {
		q += `, next_at = ?`
		args = append(args, ts(next))
	}
	if agent != "" {
		q += `, agent = ?`
		args = append(args, cleanText(agent, 60))
	}
	if note != "" {
		q += `, note = ?`
		args = append(args, cleanText(note, 120))
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	if len(from) > 0 {
		q += ` AND status IN (` + placeholders(len(from)) + `)`
		for _, f := range from {
			args = append(args, f)
		}
	}
	res, err := r.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func placeholders(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += "?"
	}
	return s
}
