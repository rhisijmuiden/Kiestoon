package pbx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Report periods.
const (
	ReportDaily   = "daily"
	ReportWeekly  = "weekly"
	ReportMonthly = "monthly"
)

// Report is a scheduled e-mail with the queue figures of the last day, week or month.
type Report struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Period is daily (that day until the send time), weekly (the previous Monday to Sunday) or
	// monthly (the previous calendar month).
	Period string `json:"period"`
	// Time is when the report is sent (HH:MM, local time).
	Time string `json:"time"`
	// Weekday is the day a weekly report is sent (1 = Monday ... 7 = Sunday).
	Weekday int `json:"weekday"`
	// Workdays sends a daily report only Monday to Friday.
	Workdays bool `json:"workdays"`
	// Queues limits the report to these queues (empty = all queues).
	Queues []int64 `json:"queues"`
	// Sections: queues, agents, hours, trend.
	Sections   []string `json:"sections"`
	Recipients []string `json:"recipients"`
	Language   string   `json:"language"`
	Enabled    bool     `json:"enabled"`
	LastSent   string   `json:"lastSent"`
}

// ReportSections are the parts a report can hold.
var ReportSections = []string{"queues", "agents", "hours", "trend"}

var hhmm = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (r *Report) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len(r.Name) > 80 {
		return invalid("required", "name")
	}
	if r.Period != ReportDaily && r.Period != ReportWeekly && r.Period != ReportMonthly {
		return invalid("invalid", "period")
	}
	if !hhmm.MatchString(r.Time) {
		return invalid("invalid", "time")
	}
	if r.Weekday == 0 {
		r.Weekday = 1
	}
	if r.Weekday < 1 || r.Weekday > 7 {
		return invalid("invalid", "weekday")
	}
	if r.Queues == nil {
		r.Queues = []int64{}
	}
	secs := []string{}
	for _, s := range r.Sections {
		if !slices.Contains(ReportSections, s) {
			return invalid("invalid", "sections")
		}
		if !slices.Contains(secs, s) {
			secs = append(secs, s)
		}
	}
	if len(secs) == 0 {
		secs = []string{"queues"}
	}
	r.Sections = secs
	clean := []string{}
	for _, m := range r.Recipients {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if a, err := mail.ParseAddress(m); err != nil || a.Address != m {
			return invalid("invalid", "recipients")
		}
		if !slices.Contains(clean, m) {
			clean = append(clean, m)
		}
	}
	if len(clean) == 0 || len(clean) > 20 {
		return invalid("required", "recipients")
	}
	r.Recipients = clean
	if r.Language != "nl" {
		r.Language = "en"
	}
	return nil
}

// Due reports whether the report should be sent at now (local time): the send moment of today
// has passed and the report was not sent since.
func (r Report) Due(now time.Time) bool {
	if !r.Enabled {
		return false
	}
	at := r.lastMoment(now)
	if at.IsZero() || now.Before(at) {
		return false
	}
	last, err := time.Parse(time.RFC3339, r.LastSent)
	return err != nil || last.Before(at)
}

// lastMoment is the most recent planned send moment at or before the end of now's day.
func (r Report) lastMoment(now time.Time) time.Time {
	h, m := 0, 0
	if len(r.Time) == 5 {
		h, m = int(r.Time[0]-'0')*10+int(r.Time[1]-'0'), int(r.Time[3]-'0')*10+int(r.Time[4]-'0')
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	switch r.Period {
	case ReportDaily:
		if r.Workdays && wd > 5 {
			return time.Time{}
		}
	case ReportWeekly:
		if wd != r.Weekday {
			return time.Time{}
		}
	case ReportMonthly:
		if now.Day() != 1 {
			return time.Time{}
		}
	}
	return at
}

// Range returns the period a report sent at now covers, [from, to), and the period before it.
func (r Report) Range(now time.Time) (from, to, prevFrom, prevTo time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch r.Period {
	case ReportWeekly:
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		// The previous full week, Monday to Sunday.
		monday := day.AddDate(0, 0, 1-wd)
		return monday.AddDate(0, 0, -7), monday, monday.AddDate(0, 0, -14), monday.AddDate(0, 0, -7)
	case ReportMonthly:
		first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
		return first.AddDate(0, -1, 0), first, first.AddDate(0, -2, 0), first.AddDate(0, -1, 0)
	}
	// Daily: that day, compared with the same weekday a week earlier.
	return day, day.AddDate(0, 0, 1), day.AddDate(0, 0, -7), day.AddDate(0, 0, -6)
}

const reportCols = `id, name, period, time, weekday, workdays, queues, sections, recipients, language, enabled, last_sent`

func scanReport(row interface{ Scan(...any) error }) (*Report, error) {
	var r Report
	var qs, secs, rc string
	var wk, en int
	err := row.Scan(&r.ID, &r.Name, &r.Period, &r.Time, &r.Weekday, &wk, &qs, &secs, &rc, &r.Language, &en, &r.LastSent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal([]byte(qs), &r.Queues)
	_ = json.Unmarshal([]byte(secs), &r.Sections)
	_ = json.Unmarshal([]byte(rc), &r.Recipients)
	if r.Queues == nil {
		r.Queues = []int64{}
	}
	if r.Recipients == nil {
		r.Recipients = []string{}
	}
	r.Workdays, r.Enabled = wk == 1, en == 1
	return &r, err
}

// Reports returns the scheduled reports.
func (r *Repo) Reports(ctx context.Context) ([]Report, error) {
	return queryAll(ctx, r.DB, `SELECT `+reportCols+` FROM wallboard_reports ORDER BY name`, scanReport)
}

// Report returns one scheduled report.
func (r *Repo) Report(ctx context.Context, id int64) (*Report, error) {
	return scanReport(r.DB.QueryRowContext(ctx, `SELECT `+reportCols+` FROM wallboard_reports WHERE id = ?`, id))
}

// SaveReport creates or updates a scheduled report.
func (r *Repo) SaveReport(ctx context.Context, rep *Report) error {
	if err := rep.normalize(); err != nil {
		return err
	}
	qs, _ := json.Marshal(rep.Queues)
	secs, _ := json.Marshal(rep.Sections)
	rc, _ := json.Marshal(rep.Recipients)
	args := []any{rep.Name, rep.Period, rep.Time, rep.Weekday, b2i(rep.Workdays), string(qs), string(secs), string(rc), rep.Language, b2i(rep.Enabled)}
	if rep.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO wallboard_reports(name, period, time, weekday, workdays, queues, sections,
			recipients, language, enabled) VALUES (?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "name")
		}
		rep.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE wallboard_reports SET name=?, period=?, time=?, weekday=?, workdays=?, queues=?, sections=?,
		recipients=?, language=?, enabled=? WHERE id=?`, append(args, rep.ID), "name")
}

// ReportSent remembers when a report was sent.
func (r *Repo) ReportSent(ctx context.Context, id int64, at time.Time) {
	_, _ = r.DB.ExecContext(ctx, `UPDATE wallboard_reports SET last_sent = ? WHERE id = ?`, at.UTC().Format(time.RFC3339), id)
}

// DeleteReport removes a scheduled report.
func (r *Repo) DeleteReport(ctx context.Context, id int64) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM wallboard_reports WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
