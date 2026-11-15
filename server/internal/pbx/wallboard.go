package pbx

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// SLAOptions are the service level thresholds of a queue. A zero threshold is not monitored.
type SLAOptions struct {
	// Target is the time (seconds) within which a call should be answered.
	Target int `json:"target"`
	// Norm is the share (percent) of calls that should be answered within the target.
	Norm int `json:"norm"`
	// MaxWaiting is the most callers that may wait at the same time.
	MaxWaiting int `json:"maxWaiting"`
	// MaxWait is the longest a caller may wait (seconds).
	MaxWait int `json:"maxWait"`
	// MaxAbandoned is the largest share (percent) of callers that may hang up while waiting.
	MaxAbandoned int `json:"maxAbandoned"`
}

// Monitored reports whether any threshold is set.
func (o SLAOptions) Monitored() bool {
	return o.Norm > 0 || o.MaxWaiting > 0 || o.MaxWait > 0 || o.MaxAbandoned > 0
}

func (o *SLAOptions) normalize() error {
	if o.Target == 0 {
		o.Target = 20
	}
	check := func(v, min, max int, field string) error {
		if v < min || v > max {
			return invalid("invalid", "options.sla."+field)
		}
		return nil
	}
	for _, c := range []struct {
		v, min, max int
		f           string
	}{{o.Target, 5, 600, "target"}, {o.Norm, 0, 100, "norm"}, {o.MaxWaiting, 0, 500, "maxWaiting"},
		{o.MaxWait, 0, 7200, "maxWait"}, {o.MaxAbandoned, 0, 100, "maxAbandoned"}} {
		if err := check(c.v, c.min, c.max, c.f); err != nil {
			return err
		}
	}
	return nil
}

// SetQueueSLA changes only the service level thresholds of a queue.
func (r *Repo) SetQueueSLA(ctx context.Context, id int64, sla SLAOptions) error {
	q, err := r.Queue(ctx, id)
	if err != nil {
		return err
	}
	if err := sla.normalize(); err != nil {
		return err
	}
	q.Options.SLA = sla
	b, _ := json.Marshal(q.Options)
	_, err = r.DB.ExecContext(ctx, `UPDATE queues SET options = ? WHERE id = ?`, string(b), id)
	return err
}

// ---- wallboard screens (tv links) -------------------------------------------

// Wallboard layouts.
const (
	LayoutOverview = "overview"
	LayoutQueue    = "queue"
	LayoutAgents   = "agents"
)

// WallScreen is a tv link: a secret key that shows the wallboard without signing in.
type WallScreen struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Key is the secret part of the link /wall/<key>.
	Key string `json:"key"`
	// Queues limits the screen to these queues (empty = all queues).
	Queues  []int64  `json:"queues"`
	Layouts []string `json:"layouts"`
	// Rotate is the time (seconds) each layout is shown; 0 = do not rotate.
	Rotate int `json:"rotate"`
	// Theme is dark or light.
	Theme string `json:"theme"`
	// HideNames shows only the last 4 digits of callers.
	HideNames bool   `json:"hideNames"`
	CreatedAt string `json:"createdAt"`
	LastSeen  string `json:"lastSeen"`
}

// NewWallKey makes a new random key for a tv link: four groups of four letters and digits.
func NewWallKey() string {
	b := make([]byte, 15)
	_, _ = rand.Read(b)
	s := strings.NewReplacer("-", "x", "_", "y").Replace(base64.RawURLEncoding.EncodeToString(b))
	return s[0:5] + "-" + s[5:10] + "-" + s[10:15] + "-" + s[15:20]
}

func (w *WallScreen) normalize() error {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" || len(w.Name) > 80 {
		return invalid("required", "name")
	}
	if w.Queues == nil {
		w.Queues = []int64{}
	}
	if len(w.Queues) > 50 {
		return invalid("invalid", "queues")
	}
	layouts := []string{}
	for _, l := range w.Layouts {
		if l != LayoutOverview && l != LayoutQueue && l != LayoutAgents {
			return invalid("invalid", "layouts")
		}
		if !slices.Contains(layouts, l) {
			layouts = append(layouts, l)
		}
	}
	if len(layouts) == 0 {
		layouts = []string{LayoutOverview}
	}
	w.Layouts = layouts
	if w.Rotate != 0 && (w.Rotate < 10 || w.Rotate > 600) {
		return invalid("invalid", "rotate")
	}
	if w.Theme != "light" {
		w.Theme = "dark"
	}
	return nil
}

const wallCols = `id, name, key, queues, layouts, rotate, theme, hide_names, created_at, last_seen`

func scanWall(row interface{ Scan(...any) error }) (*WallScreen, error) {
	var w WallScreen
	var qs, ls string
	var hide int
	err := row.Scan(&w.ID, &w.Name, &w.Key, &qs, &ls, &w.Rotate, &w.Theme, &hide, &w.CreatedAt, &w.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal([]byte(qs), &w.Queues)
	_ = json.Unmarshal([]byte(ls), &w.Layouts)
	if w.Queues == nil {
		w.Queues = []int64{}
	}
	w.HideNames = hide == 1
	return &w, err
}

// WallScreens returns all tv links.
func (r *Repo) WallScreens(ctx context.Context) ([]WallScreen, error) {
	return queryAll(ctx, r.DB, `SELECT `+wallCols+` FROM wallboard_screens ORDER BY name`, scanWall)
}

// WallScreenByKey finds the tv link with this key.
func (r *Repo) WallScreenByKey(ctx context.Context, key string) (*WallScreen, error) {
	if len(key) < 20 || len(key) > 40 {
		return nil, ErrNotFound
	}
	list, err := r.WallScreens(ctx)
	if err != nil {
		return nil, err
	}
	// Compare in constant time, so the key cannot be guessed from response times.
	for i := range list {
		if subtle.ConstantTimeCompare([]byte(list[i].Key), []byte(key)) == 1 {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

// SaveWallScreen creates or updates a tv link. A new link gets a new key.
func (r *Repo) SaveWallScreen(ctx context.Context, w *WallScreen) error {
	if err := w.normalize(); err != nil {
		return err
	}
	qs, _ := json.Marshal(w.Queues)
	ls, _ := json.Marshal(w.Layouts)
	if w.ID == 0 {
		w.Key = NewWallKey()
		res, err := r.DB.ExecContext(ctx, `INSERT INTO wallboard_screens(name, key, queues, layouts, rotate, theme, hide_names)
			VALUES (?,?,?,?,?,?,?)`, w.Name, w.Key, string(qs), string(ls), w.Rotate, w.Theme, b2i(w.HideNames))
		if err != nil {
			return err
		}
		w.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE wallboard_screens SET name=?, queues=?, layouts=?, rotate=?, theme=?, hide_names=? WHERE id=?`,
		[]any{w.Name, string(qs), string(ls), w.Rotate, w.Theme, b2i(w.HideNames), w.ID}, "name")
}

// NewWallScreenKey gives a tv link a new key; the old link stops working.
func (r *Repo) NewWallScreenKey(ctx context.Context, id int64) (string, error) {
	key := NewWallKey()
	res, err := r.DB.ExecContext(ctx, `UPDATE wallboard_screens SET key = ? WHERE id = ?`, key, id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return key, nil
}

// TouchWallScreen remembers when a tv last showed the wallboard (at most once a minute).
func (r *Repo) TouchWallScreen(ctx context.Context, id int64, now time.Time) {
	_, _ = r.DB.ExecContext(ctx, `UPDATE wallboard_screens SET last_seen = ? WHERE id = ? AND last_seen < ?`,
		now.UTC().Format(time.RFC3339), id, now.Add(-time.Minute).UTC().Format(time.RFC3339))
}

// DeleteWallScreen revokes a tv link.
func (r *Repo) DeleteWallScreen(ctx context.Context, id int64) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM wallboard_screens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- SLA alerts -------------------------------------------------------------

// AlertSettingsKey is the settings row with who gets SLA alerts.
const AlertSettingsKey = "wallboard_alerts"

// AlertSettings say where SLA alerts go.
type AlertSettings struct {
	// Screens shows alerts on the wallboard and the switchboard.
	Screens bool `json:"screens"`
	// Email sends alerts to Recipients.
	Email      bool     `json:"email"`
	Recipients []string `json:"recipients"`
	// Quiet is the time (minutes) before the same alert of the same queue is mailed again.
	Quiet int `json:"quiet"`
	// TimeCondition only mails during the opening hours of this time condition (0 = always).
	TimeCondition int64 `json:"timeCondition"`
	// Recovered also mails when a queue is back within its thresholds.
	Recovered bool `json:"recovered"`
}

// LoadAlertSettings returns the SLA alert settings, with defaults.
func LoadAlertSettings(ctx context.Context, db *sql.DB) AlertSettings {
	a := AlertSettings{Screens: true, Quiet: 15, Recipients: []string{}}
	_ = store.GetJSON(ctx, db, AlertSettingsKey, &a)
	if a.Recipients == nil {
		a.Recipients = []string{}
	}
	return a
}

// SaveAlertSettings checks and stores the SLA alert settings.
func SaveAlertSettings(ctx context.Context, r *Repo, a *AlertSettings) error {
	if a.Quiet == 0 {
		a.Quiet = 15
	}
	if a.Quiet < 1 || a.Quiet > 1440 {
		return invalid("invalid", "quiet")
	}
	clean := []string{}
	for _, m := range a.Recipients {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if addr, err := mail.ParseAddress(m); err != nil || addr.Address != m {
			return invalid("invalid", "recipients")
		}
		if !slices.Contains(clean, m) {
			clean = append(clean, m)
		}
	}
	if len(clean) > 20 {
		return invalid("invalid", "recipients")
	}
	a.Recipients = clean
	if a.Email && len(clean) == 0 {
		return invalid("required", "recipients")
	}
	if a.TimeCondition != 0 {
		if _, err := r.TimeCondition(ctx, a.TimeCondition); err != nil {
			return invalid("invalid", "timeCondition")
		}
	}
	return store.SetJSON(ctx, r.DB, AlertSettingsKey, a)
}

// OpenAt reports whether the time condition sends calls to its open destination at t (in the
// local time of the phone system). holidays are the closed days of that year.
func (t TimeCondition) OpenAt(at time.Time, holidays []Holiday) bool {
	switch t.Mode {
	case "open":
		return true
	case "closed":
		return false
	}
	day := at.Format("2006-01-02")
	if t.UseHolidays {
		for _, h := range holidays {
			if h.Date == day {
				return false
			}
		}
	}
	wd := int(at.Weekday())
	if wd == 0 {
		wd = 7
	}
	now := at.Hour()*60 + at.Minute()
	mins := func(s string) int {
		h, m, _ := strings.Cut(s, ":")
		hh, _ := strconv.Atoi(h)
		mm, _ := strconv.Atoi(m)
		return hh*60 + mm
	}
	for _, r := range t.Schedule {
		if !slices.Contains(r.Days, wd) {
			continue
		}
		if from, to := mins(r.From), mins(r.To); now >= from && now < to {
			return true
		}
	}
	return false
}
