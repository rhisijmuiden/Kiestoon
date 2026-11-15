package pbx

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
)

// FaxBox receives faxes (fax-to-email). Inbound routes send fax numbers to it; every received fax
// is converted to PDF and mailed to Emails.
type FaxBox struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Emails   string `json:"emails"` // comma separated
	KeepDays int    `json:"keepDays"`
	ECM      bool   `json:"ecm"` // error correction mode
	Enabled  bool   `json:"enabled"`
	// Number is the fax number of the box: the caller ID and station ID of sent faxes.
	Number string `json:"number"`
	// SendEnabled allows sending faxes from this box (portal and e-mail).
	SendEnabled bool `json:"sendEnabled"`
	// Senders are the e-mail addresses that may send a fax by e-mail (comma separated).
	Senders string `json:"senders"`
	// SendPIN, when set, has to be in the subject of a fax e-mail.
	SendPIN    string `json:"sendPin,omitempty"`
	SendPINSet bool   `json:"sendPinSet"`
}

// SenderList returns the allowed sender addresses in lower case.
func (f FaxBox) SenderList() []string {
	out := []string{}
	for _, e := range strings.Split(f.Senders, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// Recipients returns the e-mail addresses.
func (f FaxBox) Recipients() []string {
	out := []string{}
	for _, e := range strings.Split(f.Emails, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func (f *FaxBox) validate() error {
	f.Name = cleanText(f.Name, 60)
	if f.Name == "" {
		return invalid("required", "name")
	}
	list := []string{}
	for _, e := range strings.Split(f.Emails, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "@") || strings.ContainsAny(e, " <>\"\r\n;") || len(e) > 120 {
			return invalid("invalid", "emails")
		}
		list = append(list, e)
	}
	if len(list) == 0 || len(list) > 10 {
		return invalid("required", "emails")
	}
	f.Emails = strings.Join(list, ", ")
	if f.KeepDays < 0 || f.KeepDays > 3650 {
		return invalid("invalid", "keepDays")
	}
	f.Number = strings.TrimSpace(f.Number)
	if f.Number != "" && !reFaxNumber.MatchString(f.Number) {
		return invalid("invalid", "number")
	}
	if f.SendEnabled && f.Number == "" {
		return invalid("required", "number")
	}
	senders := []string{}
	for _, e := range strings.Split(f.Senders, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.Contains(e, "@") || strings.ContainsAny(e, " <>\"\r\n;") || len(e) > 120 {
			return invalid("invalid", "senders")
		}
		senders = append(senders, e)
	}
	if len(senders) > 50 {
		return invalid("invalid", "senders")
	}
	f.Senders = strings.Join(senders, ", ")
	f.SendPIN = strings.TrimSpace(f.SendPIN)
	if f.SendPIN != "" && !reFaxPIN.MatchString(f.SendPIN) {
		return invalid("invalid", "sendPin")
	}
	return nil
}

var (
	reFaxNumber = regexp.MustCompile(`^\+?[0-9]{3,20}$`)
	reFaxPIN    = regexp.MustCompile(`^[A-Za-z0-9]{4,20}$`)
	// reFaxDial is a number a fax can be sent to.
	reFaxDial = regexp.MustCompile(`^\+?[0-9]{3,20}$`)
)

// FaxDialNumber cleans a destination number ("020-123 45 67" becomes "0201234567"), or "".
func FaxDialNumber(s string) string {
	n := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')', '/':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if !reFaxDial.MatchString(n) {
		return ""
	}
	return n
}

const faxBoxCols = `id, name, emails, keep_days, ecm, enabled, number, send_enabled, senders, send_pin`

func scanFaxBox(row interface{ Scan(...any) error }) (*FaxBox, error) {
	var f FaxBox
	var ecm, en, send int
	err := row.Scan(&f.ID, &f.Name, &f.Emails, &f.KeepDays, &ecm, &en, &f.Number, &send, &f.Senders, &f.SendPIN)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	f.ECM, f.Enabled, f.SendEnabled, f.SendPINSet = ecm == 1, en == 1, send == 1, f.SendPIN != ""
	return &f, err
}

// FaxBoxes returns all fax boxes.
func (r *Repo) FaxBoxes(ctx context.Context) ([]FaxBox, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxBoxCols+` FROM fax_boxes ORDER BY name`, scanFaxBox)
}

// FaxBox returns one fax box.
func (r *Repo) FaxBox(ctx context.Context, id int64) (*FaxBox, error) {
	return scanFaxBox(r.DB.QueryRowContext(ctx, `SELECT `+faxBoxCols+` FROM fax_boxes WHERE id = ?`, id))
}

// SaveFaxBox validates and stores a fax box.
func (r *Repo) SaveFaxBox(ctx context.Context, f *FaxBox) error {
	if err := f.validate(); err != nil {
		return err
	}
	if f.ID != 0 && f.SendPIN == "" && f.SendPINSet {
		// Keep the stored PIN when the form did not send it again.
		if old, err := r.FaxBox(ctx, f.ID); err == nil {
			f.SendPIN = old.SendPIN
		}
	}
	f.SendPINSet = f.SendPIN != ""
	args := []any{f.Name, f.Emails, f.KeepDays, b2i(f.ECM), b2i(f.Enabled), f.Number, b2i(f.SendEnabled), f.Senders, f.SendPIN}
	if f.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO fax_boxes(name, emails, keep_days, ecm, enabled, number, send_enabled, senders, send_pin)
			VALUES (?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		f.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE fax_boxes SET name=?, emails=?, keep_days=?, ecm=?, enabled=?, number=?, send_enabled=?, senders=?, send_pin=?
		WHERE id=?`, append(args, f.ID), "name")
}

// DeleteFaxBox removes a fax box; routes to it hang up from then on. Received faxes stay.
func (r *Repo) DeleteFaxBox(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "fax_boxes", DestFax, id)
}

// Fax is a received fax.
type Fax struct {
	ID            int64  `json:"id"`
	BoxID         *int64 `json:"boxId"`
	BoxName       string `json:"boxName"`
	ReceivedAt    string `json:"receivedAt"`
	Caller        string `json:"caller"`
	CallerName    string `json:"callerName"`
	DID           string `json:"did"`
	RemoteStation string `json:"remoteStation"`
	Pages         int    `json:"pages"`
	Status        string `json:"status"` // success | failed
	Error         string `json:"error"`
	File          string `json:"-"`
	Size          int64  `json:"size"`
	Mailed        bool   `json:"mailed"`
}

// InsertFax stores a received fax.
func (r *Repo) InsertFax(ctx context.Context, f *Fax) error {
	res, err := r.DB.ExecContext(ctx, `INSERT INTO faxes(box_id, received_at, caller, caller_name, did, remote_station, pages, status, error, file, size, mailed)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, f.BoxID, f.ReceivedAt, f.Caller, f.CallerName, f.DID, f.RemoteStation, f.Pages, f.Status, f.Error,
		f.File, f.Size, b2i(f.Mailed))
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

// SetFaxMailed marks a fax as sent by e-mail.
func (r *Repo) SetFaxMailed(ctx context.Context, id int64) {
	_, _ = r.DB.ExecContext(ctx, `UPDATE faxes SET mailed = 1 WHERE id = ?`, id)
}

const faxCols = `f.id, f.box_id, COALESCE(b.name, ''), f.received_at, f.caller, f.caller_name, f.did, f.remote_station, f.pages, f.status,
	f.error, f.file, f.size, f.mailed`

func scanFax(row interface{ Scan(...any) error }) (*Fax, error) {
	var f Fax
	var box sql.NullInt64
	var mailed int
	err := row.Scan(&f.ID, &box, &f.BoxName, &f.ReceivedAt, &f.Caller, &f.CallerName, &f.DID, &f.RemoteStation, &f.Pages, &f.Status,
		&f.Error, &f.File, &f.Size, &mailed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if box.Valid {
		f.BoxID = &box.Int64
	}
	f.Mailed = mailed == 1
	return &f, err
}

// Faxes returns the latest received faxes.
func (r *Repo) Faxes(ctx context.Context, limit int) ([]Fax, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxCols+` FROM faxes f LEFT JOIN fax_boxes b ON b.id = f.box_id ORDER BY f.id DESC LIMIT ?`, scanFax, limit)
}

// Fax returns one received fax.
func (r *Repo) Fax(ctx context.Context, id int64) (*Fax, error) {
	return scanFax(r.DB.QueryRowContext(ctx, `SELECT `+faxCols+` FROM faxes f LEFT JOIN fax_boxes b ON b.id = f.box_id WHERE f.id = ?`, id))
}

// DeleteFax removes a fax record and returns its file name.
func (r *Repo) DeleteFax(ctx context.Context, id int64) (string, error) {
	f, err := r.Fax(ctx, id)
	if err != nil {
		return "", err
	}
	_, err = r.DB.ExecContext(ctx, `DELETE FROM faxes WHERE id = ?`, id)
	return f.File, err
}

// ExpiredFaxes returns the faxes older than the retention of their box.
func (r *Repo) ExpiredFaxes(ctx context.Context) ([]Fax, error) {
	return queryAll(ctx, r.DB, `SELECT `+faxCols+` FROM faxes f LEFT JOIN fax_boxes b ON b.id = f.box_id
		WHERE b.keep_days > 0 AND f.received_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-' || b.keep_days || ' days')`, scanFax)
}
