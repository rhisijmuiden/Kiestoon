package pbx

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FaxJob is a fax to send, from the portal or by e-mail.
type FaxJob struct {
	ID        int64  `json:"id"`
	BoxID     *int64 `json:"boxId"`
	BoxName   string `json:"boxName"`
	UserID    *int64 `json:"userId"`
	Source    string `json:"source"` // portal | email
	Sender    string `json:"sender"` // e-mail address or user name
	Number    string `json:"number"`
	Document  string `json:"document"` // original file name
	File      string `json:"-"`        // stored PDF (in <data>/fax/out)
	Pages     int    `json:"pages"`
	Status    string `json:"status"` // queued | sending | success | failed
	Attempts  int    `json:"attempts"`
	Error     string `json:"error"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	NextAt    string `json:"nextAt"`
}

// MaxFaxAttempts is how often a fax is tried (busy, no answer, failed transmission).
const MaxFaxAttempts = 3

const faxJobCols = `j.id, j.box_id, COALESCE(b.name, ''), j.user_id, j.source, j.sender, j.number, j.document, j.file, j.pages, j.status,
	j.attempts, j.error, j.created_at, j.updated_at, j.next_at`

func scanFaxJob(row interface{ Scan(...any) error }) (*FaxJob, error) {
	var j FaxJob
	var box, user sql.NullInt64
	err := row.Scan(&j.ID, &box, &j.BoxName, &user, &j.Source, &j.Sender, &j.Number, &j.Document, &j.File, &j.Pages, &j.Status,
		&j.Attempts, &j.Error, &j.CreatedAt, &j.UpdatedAt, &j.NextAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if box.Valid {
		j.BoxID = &box.Int64
	}
	if user.Valid {
		j.UserID = &user.Int64
	}
	return &j, err
}

const faxJobFrom = ` FROM fax_jobs j LEFT JOIN fax_boxes b ON b.id = j.box_id`

// InsertFaxJob queues a fax.
func (r *Repo) InsertFaxJob(ctx context.Context, j *FaxJob) error {
	res, err := r.DB.ExecContext(ctx, `INSERT INTO fax_jobs(box_id, user_id, source, sender, number, document, file, pages, status)
		VALUES (?,?,?,?,?,?,?,?, 'queued')`, j.BoxID, j.UserID, j.Source, j.Sender, j.Number, j.Document, j.File, j.Pages)
	if err != nil {
		return err
	}
	j.ID, _ = res.LastInsertId()
	return nil
}

// FaxJobs returns the latest sent faxes; userID limits them to one user (nil: all).
func (r *Repo) FaxJobs(ctx context.Context, userID *int64, limit int) ([]FaxJob, error) {
	if userID != nil {
		return queryAll(ctx, r.DB, `SELECT `+faxJobCols+faxJobFrom+` WHERE j.user_id = ? ORDER BY j.id DESC LIMIT ?`, scanFaxJob, *userID, limit)
	}
	return queryAll(ctx, r.DB, `SELECT `+faxJobCols+faxJobFrom+` ORDER BY j.id DESC LIMIT ?`, scanFaxJob, limit)
}

// FaxJob returns one job.
func (r *Repo) FaxJob(ctx context.Context, id int64) (*FaxJob, error) {
	return scanFaxJob(r.DB.QueryRowContext(ctx, `SELECT `+faxJobCols+faxJobFrom+` WHERE j.id = ?`, id))
}

// DueFaxJobs returns queued jobs whose time has come, oldest first.
func (r *Repo) DueFaxJobs(ctx context.Context, now time.Time, limit int) ([]FaxJob, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxJobCols+faxJobFrom+` WHERE j.status = 'queued' AND (j.next_at = '' OR j.next_at <= ?)
		ORDER BY j.id LIMIT ?`, scanFaxJob, now.UTC().Format(time.RFC3339), limit)
}

// SendingFaxJobs returns the jobs on the line right now.
func (r *Repo) SendingFaxJobs(ctx context.Context) ([]FaxJob, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxJobCols+faxJobFrom+` WHERE j.status = 'sending' ORDER BY j.id`, scanFaxJob)
}

// UpdateFaxJob stores the state of a job.
func (r *Repo) UpdateFaxJob(ctx context.Context, j *FaxJob) error {
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if len(j.Error) > 300 {
		j.Error = j.Error[:300]
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE fax_jobs SET status=?, attempts=?, error=?, pages=?, next_at=?, updated_at=? WHERE id=?`,
		j.Status, j.Attempts, j.Error, j.Pages, j.NextAt, j.UpdatedAt, j.ID)
	return err
}

// DeleteFaxJob removes a job and returns it.
func (r *Repo) DeleteFaxJob(ctx context.Context, id int64) (*FaxJob, error) {
	j, err := r.FaxJob(ctx, id)
	if err != nil {
		return nil, err
	}
	_, err = r.DB.ExecContext(ctx, `DELETE FROM fax_jobs WHERE id = ?`, id)
	return j, err
}

// ExpiredFaxJobs returns finished jobs older than the retention of their box (90 days without a box).
func (r *Repo) ExpiredFaxJobs(ctx context.Context) ([]FaxJob, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxJobCols+faxJobFrom+` WHERE j.status IN ('success','failed') AND
		j.created_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || CASE WHEN COALESCE(b.keep_days, 0) > 0 THEN b.keep_days ELSE 90 END || ' days')`, scanFaxJob)
}
