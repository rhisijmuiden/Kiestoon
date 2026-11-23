package pbx

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---- on-call (storingsdienst) -----------------------------------------------

// DestOnCall sends a call to an on-call team: the PBX rings the engineer on duty on their own
// mobile number and escalates when nobody answers.
const DestOnCall = "oncall"

// OnCallStep is one step of the escalation: who is called and for how long.
type OnCallStep struct {
	// Kind: primary (1st line on duty), backup (2nd line), lead (team lead), all (every member),
	// user (one fixed member, Target = user id) or number (a fixed phone number, Target).
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Seconds int    `json:"seconds"`
}

// OnCallTeam is an on-call service with its schedule and escalation.
type OnCallTeam struct {
	ID      int64  `json:"id"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Mode: weekly or daily rotation through Members, or manual (only planned shifts).
	Mode string `json:"mode"`
	// HandoverDay is the weekday a weekly rotation hands over (0 Monday … 6 Sunday).
	HandoverDay  int    `json:"handoverDay"`
	HandoverTime string `json:"handoverTime"` // HH:MM, local time of the PBX
	// StartDate is the first day of the rotation (YYYY-MM-DD): Members[0] is on duty from then.
	StartDate string  `json:"startDate"`
	Members   []int64 `json:"members"`
	// Backup: next (the next one in the rotation), fixed (BackupUser) or none.
	Backup     string       `json:"backup"`
	BackupUser int64        `json:"backupUser"`
	LeadUser   int64        `json:"leadUser"`
	Steps      []OnCallStep `json:"steps"`
	// Repeat runs the steps this many extra times before Final.
	Repeat int `json:"repeat"`
	// Confirm makes the engineer press 1 to accept, so a mobile's own voicemail never takes the call.
	Confirm bool `json:"confirm"`
	// RingExtension also rings the engineer's own extension (desk phone, softphone, mobile app).
	RingExtension  bool        `json:"ringExtension"`
	Music          bool        `json:"music"`
	GreetingPrompt int64       `json:"greetingPrompt"`
	CIDPrefix      string      `json:"cidPrefix"`
	Final          Destination `json:"final"`
	// SwapApproval: none (accepted by the colleague is enough), lead or admin.
	SwapApproval string `json:"swapApproval"`
	// Reminders mail engineers the day before their shift starts.
	Reminders bool `json:"reminders"`
	// ICalToken is the secret part of the calendar link.
	ICalToken string `json:"icalToken,omitempty"`
}

// OnCallShift is a planned shift or a change on top of the rotation: a swap, hand-over or
// take-over. Approved shifts beat the regular rotation; the newest one wins when they overlap.
type OnCallShift struct {
	ID     int64 `json:"id"`
	TeamID int64 `json:"teamId"`
	// Line 1 is the engineer on duty, 2 the backup.
	Line   int       `json:"line"`
	UserID int64     `json:"userId"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	// Kind: shift (planned by an admin), override (admin), handover, swap or takeover.
	Kind string `json:"kind"`
	// Status: pending, approved, declined or cancelled.
	Status      string `json:"status"`
	RequestedBy int64  `json:"requestedBy"`
	FromUser    int64  `json:"fromUser"`
	PairID      int64  `json:"pairId"`
	Accepted    bool   `json:"accepted"`
	Note        string `json:"note"`
	DecidedBy   int64  `json:"decidedBy"`
	CreatedAt   string `json:"createdAt"`
}

// Engineer is a user as the on-call module needs it.
type Engineer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Language  string `json:"language"`
	Mobile    string `json:"mobile"`
	Extension string `json:"extension"`
	Enabled   bool   `json:"enabled"`
}

// OnCallSegment is a stretch of time with one engineer on a line.
type OnCallSegment struct {
	Line   int       `json:"line"`
	UserID int64     `json:"userId"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	// Kind: rotation, gap, or the kind of the shift that set it.
	Kind    string `json:"kind"`
	ShiftID int64  `json:"shiftId,omitempty"`
}

// OnCallLive is who a team rings right now, for the dialplan.
type OnCallLive struct {
	Team    OnCallTeam
	Primary *Engineer
	Backup  *Engineer
	Lead    *Engineer
	All     []Engineer
	Users   map[int64]Engineer
}

func newToken() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func scanOnCall(row interface{ Scan(...any) error }) (*OnCallTeam, error) {
	var t OnCallTeam
	var cfg, number, name, token string
	var id int64
	var en int
	err := row.Scan(&id, &number, &name, &en, &cfg, &token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// The columns win over the copy in the configuration.
	_ = json.Unmarshal([]byte(cfg), &t)
	t.ID, t.Number, t.Name, t.ICalToken, t.Enabled = id, number, name, token, en == 1
	if t.Members == nil {
		t.Members = []int64{}
	}
	if t.Steps == nil {
		t.Steps = []OnCallStep{}
	}
	return &t, nil
}

const ocCols = `id, number, name, enabled, config, ical_token`

// OnCallTeams returns all on-call teams.
func (r *Repo) OnCallTeams(ctx context.Context) ([]OnCallTeam, error) {
	return queryAll(ctx, r.DB, `SELECT `+ocCols+` FROM oncall_teams ORDER BY number`, scanOnCall)
}

// OnCallTeam returns one team.
func (r *Repo) OnCallTeam(ctx context.Context, id int64) (*OnCallTeam, error) {
	return scanOnCall(r.DB.QueryRowContext(ctx, `SELECT `+ocCols+` FROM oncall_teams WHERE id = ?`, id))
}

// OnCallTeamByToken returns the team of a calendar link.
func (r *Repo) OnCallTeamByToken(ctx context.Context, token string) (*OnCallTeam, error) {
	if len(token) < 20 {
		return nil, ErrNotFound
	}
	return scanOnCall(r.DB.QueryRowContext(ctx, `SELECT `+ocCols+` FROM oncall_teams WHERE ical_token = ?`, token))
}

// DefaultOnCallSteps is the escalation of a new team.
func DefaultOnCallSteps() []OnCallStep {
	return []OnCallStep{{Kind: "primary", Seconds: 25}, {Kind: "primary", Seconds: 20}, {Kind: "backup", Seconds: 25}, {Kind: "lead", Seconds: 30}}
}

// SaveOnCallTeam validates and stores a team.
func (r *Repo) SaveOnCallTeam(ctx context.Context, plan NumberPlan, t *OnCallTeam) error {
	t.Name = cleanText(t.Name, 60)
	t.CIDPrefix = cleanText(t.CIDPrefix, 20)
	if t.Name == "" {
		return invalid("required", "name")
	}
	if !plan.InRange("ringGroups", t.Number) {
		return invalid("number_out_of_range", "number")
	}
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM ring_groups WHERE number = ?) + (SELECT COUNT(*) FROM queues WHERE number = ?)
		+ (SELECT COUNT(*) FROM conferences WHERE number = ?)`, t.Number, t.Number, t.Number).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return invalid("duplicate", "number")
	}
	switch t.Mode {
	case "":
		t.Mode = "weekly"
	case "weekly", "daily", "manual":
	default:
		return invalid("invalid", "mode")
	}
	if t.HandoverDay < 0 || t.HandoverDay > 6 {
		return invalid("invalid", "handoverDay")
	}
	if t.HandoverTime == "" {
		t.HandoverTime = "08:00"
	}
	if !reHHMM.MatchString(t.HandoverTime) {
		return invalid("invalid", "handoverTime")
	}
	if t.StartDate == "" {
		t.StartDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", t.StartDate); err != nil {
		return invalid("invalid", "startDate")
	}
	users, err := r.Engineers(ctx)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	mem := []int64{}
	for _, id := range t.Members {
		if _, ok := users[id]; !ok {
			return invalid("unknown_member", "members")
		}
		if !seen[id] {
			seen[id] = true
			mem = append(mem, id)
		}
	}
	if len(mem) > 50 {
		return invalid("invalid", "members")
	}
	t.Members = mem
	if t.Mode != "manual" && len(mem) == 0 {
		return invalid("required", "members")
	}
	switch t.Backup {
	case "":
		t.Backup = "next"
	case "next", "none":
	case "fixed":
		if _, ok := users[t.BackupUser]; !ok {
			return invalid("required", "backupUser")
		}
	default:
		return invalid("invalid", "backup")
	}
	if t.Backup != "fixed" {
		t.BackupUser = 0
	}
	if t.LeadUser != 0 {
		if _, ok := users[t.LeadUser]; !ok {
			return invalid("invalid", "leadUser")
		}
	}
	switch t.SwapApproval {
	case "":
		t.SwapApproval = "lead"
	case "none", "lead", "admin":
	default:
		return invalid("invalid", "swapApproval")
	}
	if t.SwapApproval == "lead" && t.LeadUser == 0 {
		t.SwapApproval = "admin"
	}
	if len(t.Steps) == 0 {
		return invalid("required", "steps")
	}
	if len(t.Steps) > 10 {
		return invalid("invalid", "steps")
	}
	for i := range t.Steps {
		st := &t.Steps[i]
		field := "steps." + strconv.Itoa(i)
		if st.Seconds == 0 {
			st.Seconds = 25
		}
		if st.Seconds < 5 || st.Seconds > 300 {
			return invalid("invalid", field)
		}
		switch st.Kind {
		case "primary", "backup", "lead", "all":
			st.Target = ""
		case "user":
			id, _ := strconv.ParseInt(st.Target, 10, 64)
			if _, ok := users[id]; !ok {
				return invalid("unknown_member", field)
			}
		case "number":
			st.Target = NormalizeDID(st.Target)
			if !reExternal.MatchString(st.Target) {
				return invalid("invalid", field)
			}
		default:
			return invalid("invalid", field)
		}
	}
	if t.Repeat < 0 || t.Repeat > 5 {
		return invalid("invalid", "repeat")
	}
	if t.GreetingPrompt != 0 {
		var c int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM prompts WHERE id = ?`, t.GreetingPrompt).Scan(&c); err != nil {
			return err
		}
		if c == 0 {
			return invalid("invalid", "greetingPrompt")
		}
	}
	if t.Final.Type == "" {
		t.Final.Type = DestHangup
	}
	if err := r.ValidateDestination(ctx, &t.Final, "final"); err != nil {
		return err
	}
	if t.Final.Type == DestOnCall && t.Final.Target == strconv.FormatInt(t.ID, 10) {
		return invalid("loop", "final")
	}
	token := t.ICalToken
	t.ICalToken = ""
	cfg, _ := json.Marshal(t)
	if t.ID == 0 {
		t.ICalToken = newToken()
		res, err := r.DB.ExecContext(ctx, `INSERT INTO oncall_teams(number, name, enabled, config, ical_token) VALUES (?,?,?,?,?)`,
			t.Number, t.Name, b2i(t.Enabled), string(cfg), t.ICalToken)
		if err != nil {
			return uniqueErr(err, "number")
		}
		t.ID, _ = res.LastInsertId()
		return nil
	}
	t.ICalToken = token
	return updateRow(ctx, r.DB, `UPDATE oncall_teams SET number=?, name=?, enabled=?, config=? WHERE id=?`,
		[]any{t.Number, t.Name, b2i(t.Enabled), string(cfg), t.ID}, "number")
}

// NewOnCallToken replaces the calendar link of a team.
func (r *Repo) NewOnCallToken(ctx context.Context, id int64) (string, error) {
	tok := newToken()
	return tok, updateRow(ctx, r.DB, `UPDATE oncall_teams SET ical_token=? WHERE id=?`, []any{tok, id}, "")
}

// DeleteOnCallTeam removes a team (with its shifts) and points everything that used it at "hang up".
func (r *Repo) DeleteOnCallTeam(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "oncall_teams", DestOnCall, id)
}

// Engineers returns every enabled user with the fields the on-call module needs.
func (r *Repo) Engineers(ctx context.Context) (map[int64]Engineer, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT u.id, u.display_name, u.email, u.language, u.mobile, COALESCE(e.number, ''), u.enabled
		FROM users u LEFT JOIN extensions e ON e.id = u.extension_id AND e.enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Engineer{}
	for rows.Next() {
		var e Engineer
		var en int
		if err := rows.Scan(&e.ID, &e.Name, &e.Email, &e.Language, &e.Mobile, &e.Extension, &en); err != nil {
			return nil, err
		}
		e.Enabled = en == 1
		if e.Enabled {
			out[e.ID] = e
		}
	}
	return out, rows.Err()
}

// SetMobile stores a user's own mobile number ("" clears it).
func (r *Repo) SetMobile(ctx context.Context, userID int64, mobile string) (string, error) {
	mobile = NormalizeDID(mobile)
	if mobile != "" && !reExternal.MatchString(mobile) {
		return "", invalid("invalid", "mobile")
	}
	return mobile, updateRow(ctx, r.DB, `UPDATE users SET mobile=? WHERE id=?`, []any{mobile, userID}, "")
}

// ---- shifts -----------------------------------------------------------------

const shiftCols = `id, team_id, line, user_id, start_at, end_at, kind, status, requested_by, from_user, pair_id, accepted, note, decided_by, created_at`

func scanShift(row interface{ Scan(...any) error }) (*OnCallShift, error) {
	var s OnCallShift
	var start, end string
	var acc int
	err := row.Scan(&s.ID, &s.TeamID, &s.Line, &s.UserID, &start, &end, &s.Kind, &s.Status, &s.RequestedBy, &s.FromUser, &s.PairID,
		&acc, &s.Note, &s.DecidedBy, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Start, _ = time.Parse(time.RFC3339, start)
	s.End, _ = time.Parse(time.RFC3339, end)
	s.Accepted = acc == 1
	return &s, nil
}

// OnCallShifts returns the shifts of a team (0: all teams) that end after from.
func (r *Repo) OnCallShifts(ctx context.Context, team int64, from time.Time) ([]OnCallShift, error) {
	q := `SELECT ` + shiftCols + ` FROM oncall_shifts WHERE end_at > ?`
	args := []any{from.UTC().Format(time.RFC3339)}
	if team != 0 {
		q += ` AND team_id = ?`
		args = append(args, team)
	}
	return queryAll(ctx, r.DB, q+` ORDER BY start_at, id`, scanShift, args...)
}

// OnCallShift returns one shift.
func (r *Repo) OnCallShift(ctx context.Context, id int64) (*OnCallShift, error) {
	return scanShift(r.DB.QueryRowContext(ctx, `SELECT `+shiftCols+` FROM oncall_shifts WHERE id = ?`, id))
}

// AddOnCallShift validates and stores a new shift.
func (r *Repo) AddOnCallShift(ctx context.Context, t *OnCallTeam, s *OnCallShift) error {
	s.Note = cleanText(s.Note, 300)
	if s.Line != 1 && s.Line != 2 {
		return invalid("invalid", "line")
	}
	if s.Start.IsZero() {
		return invalid("required", "start")
	}
	if !s.End.After(s.Start) {
		return invalid("invalid", "end")
	}
	if s.End.Sub(s.Start) > 92*24*time.Hour {
		return invalid("too_long", "end")
	}
	if !slices.Contains(t.Members, s.UserID) && s.UserID != t.LeadUser && s.UserID != t.BackupUser {
		return invalid("unknown_member", "userId")
	}
	switch s.Kind {
	case "shift", "override", "handover", "swap", "takeover":
	default:
		return invalid("invalid", "kind")
	}
	switch s.Status {
	case "pending", "approved":
	default:
		return invalid("invalid", "status")
	}
	s.TeamID = t.ID
	s.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.DB.ExecContext(ctx, `INSERT INTO oncall_shifts(team_id, line, user_id, start_at, end_at, kind, status, requested_by,
		from_user, pair_id, accepted, note, decided_by, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.TeamID, s.Line, s.UserID, s.Start.UTC().Format(time.RFC3339), s.End.UTC().Format(time.RFC3339), s.Kind, s.Status,
		s.RequestedBy, s.FromUser, s.PairID, b2i(s.Accepted), s.Note, s.DecidedBy, s.CreatedAt)
	if err != nil {
		return err
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

// LinkOnCallShifts makes two shifts the two halves of one swap.
func (r *Repo) LinkOnCallShifts(ctx context.Context, a, b int64) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE oncall_shifts SET pair_id = CASE id WHEN ? THEN ? ELSE ? END WHERE id IN (?, ?)`, a, b, a, a, b)
	return err
}

// UpdateOnCallShift stores the state of a shift (and of its other half, for a swap).
func (r *Repo) UpdateOnCallShift(ctx context.Context, s *OnCallShift) error {
	q := `UPDATE oncall_shifts SET status=?, accepted=?, decided_by=? WHERE id IN (?, ?)`
	_, err := r.DB.ExecContext(ctx, q, s.Status, b2i(s.Accepted), s.DecidedBy, s.ID, s.PairID)
	return err
}

// DeleteOnCallShift removes a shift (and the other half of a swap).
func (r *Repo) DeleteOnCallShift(ctx context.Context, s *OnCallShift) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM oncall_shifts WHERE id IN (?, ?)`, s.ID, s.PairID)
	return err
}

// ---- resolver ---------------------------------------------------------------

// handoverAt returns the k-th hand-over moment of the rotation (k may be negative).
func (t *OnCallTeam) handoverAt(k int, loc *time.Location) time.Time {
	d, err := time.ParseInLocation("2006-01-02", t.StartDate, loc)
	if err != nil {
		d = time.Date(2026, 1, 5, 0, 0, 0, 0, loc)
	}
	hh, _ := strconv.Atoi(t.HandoverTime[:2])
	mm, _ := strconv.Atoi(t.HandoverTime[3:])
	days := 1
	if t.Mode == "weekly" {
		days = 7
		// The rotation starts on the hand-over day on or before the start date.
		back := (int(d.Weekday()) + 6 - t.HandoverDay) % 7 // Weekday: Sunday 0; ours: Monday 0
		d = d.AddDate(0, 0, -back)
	}
	return time.Date(d.Year(), d.Month(), d.Day()+k*days, hh, mm, 0, 0, loc)
}

// periodAt returns k with handoverAt(k) <= at < handoverAt(k+1).
func (t *OnCallTeam) periodAt(at time.Time, loc *time.Location) int {
	days := 1.0
	if t.Mode == "weekly" {
		days = 7
	}
	k := int(at.Sub(t.handoverAt(0, loc)).Hours() / 24 / days)
	for t.handoverAt(k, loc).After(at) {
		k--
	}
	for !t.handoverAt(k+1, loc).After(at) {
		k++
	}
	return k
}

func mod(a, n int) int { return ((a % n) + n) % n }

// rotationUser returns who the rotation puts on a line in period k (0: nobody).
func (t *OnCallTeam) rotationUser(k, line int) int64 {
	n := len(t.Members)
	if t.Mode == "manual" || n == 0 {
		return 0
	}
	if line == 1 {
		return t.Members[mod(k, n)]
	}
	switch t.Backup {
	case "fixed":
		return t.BackupUser
	case "next":
		if n > 1 {
			return t.Members[mod(k+1, n)]
		}
	}
	return 0
}

// Timeline returns who is on each line between from and to: the rotation with the approved
// shifts on top (the newest wins), and gaps where nobody is on duty.
func (t *OnCallTeam) Timeline(shifts []OnCallShift, from, to time.Time, loc *time.Location) []OnCallSegment {
	out := []OnCallSegment{}
	if !to.After(from) {
		return out
	}
	approved := []OnCallShift{}
	for _, s := range shifts {
		if s.TeamID == t.ID && s.Status == "approved" && s.End.After(from) && s.Start.Before(to) {
			approved = append(approved, s)
		}
	}
	// Older first, so newer shifts are laid over them.
	sort.SliceStable(approved, func(i, j int) bool { return approved[i].CreatedAt < approved[j].CreatedAt })
	for line := 1; line <= 2; line++ {
		if line == 2 && t.Backup == "none" && !hasLine(approved, 2) {
			continue
		}
		segs := []OnCallSegment{}
		if t.Mode == "manual" || len(t.Members) == 0 {
			if line == 2 && t.Backup == "fixed" && t.BackupUser != 0 {
				segs = append(segs, OnCallSegment{Line: line, UserID: t.BackupUser, Start: from, End: to, Kind: "rotation"})
			} else {
				segs = append(segs, OnCallSegment{Line: line, Start: from, End: to, Kind: "gap"})
			}
		} else {
			for k := t.periodAt(from, loc); ; k++ {
				a, b := t.handoverAt(k, loc), t.handoverAt(k+1, loc)
				if !a.Before(to) {
					break
				}
				u := t.rotationUser(k, line)
				kind := "rotation"
				if u == 0 {
					kind = "gap"
				}
				segs = append(segs, OnCallSegment{Line: line, UserID: u, Start: maxTime(a, from), End: minTime(b, to), Kind: kind})
			}
		}
		for _, s := range approved {
			if s.Line != line {
				continue
			}
			segs = overlay(segs, OnCallSegment{Line: line, UserID: s.UserID, Start: maxTime(s.Start, from), End: minTime(s.End, to), Kind: s.Kind, ShiftID: s.ID})
		}
		out = append(out, merge(segs)...)
	}
	return out
}

func hasLine(s []OnCallShift, line int) bool {
	for _, x := range s {
		if x.Line == line {
			return true
		}
	}
	return false
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// overlay lays n over segs, cutting what it covers.
func overlay(segs []OnCallSegment, n OnCallSegment) []OnCallSegment {
	out := make([]OnCallSegment, 0, len(segs)+2)
	for _, s := range segs {
		if !s.End.After(n.Start) || !s.Start.Before(n.End) {
			out = append(out, s)
			continue
		}
		if s.Start.Before(n.Start) {
			l := s
			l.End = n.Start
			out = append(out, l)
		}
		if s.End.After(n.End) {
			r := s
			r.Start = n.End
			out = append(out, r)
		}
	}
	out = append(out, n)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// merge joins neighbouring segments of the same engineer and kind.
func merge(segs []OnCallSegment) []OnCallSegment {
	out := []OnCallSegment{}
	for _, s := range segs {
		if !s.End.After(s.Start) {
			continue
		}
		if n := len(out); n > 0 && out[n-1].UserID == s.UserID && out[n-1].Kind == s.Kind && out[n-1].ShiftID == s.ShiftID && out[n-1].End.Equal(s.Start) {
			out[n-1].End = s.End
			continue
		}
		out = append(out, s)
	}
	return out
}

// OnDuty returns who is on line 1 and 2 at a moment, and until when the line-1 engineer stays.
func (t *OnCallTeam) OnDuty(shifts []OnCallShift, at time.Time, loc *time.Location) (primary, backup int64, until time.Time) {
	horizon := at.Add(45 * 24 * time.Hour)
	segs := t.Timeline(shifts, at, horizon, loc)
	for _, s := range segs {
		if s.Start.Equal(at) {
			if s.Line == 1 {
				primary = s.UserID
			} else {
				backup = s.UserID
			}
		}
	}
	until = horizon
	for _, s := range segs {
		if s.Line == 1 && s.Start.After(at) && s.UserID != primary {
			until = s.Start
			break
		}
	}
	if backup == primary {
		backup = 0
	}
	return primary, backup, until
}

// OnCallLive returns who every enabled team rings right now.
func (r *Repo) OnCallLive(ctx context.Context, now time.Time) ([]OnCallLive, error) {
	teams, err := r.OnCallTeams(ctx)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}
	users, err := r.Engineers(ctx)
	if err != nil {
		return nil, err
	}
	shifts, err := r.OnCallShifts(ctx, 0, now.Add(-time.Minute))
	if err != nil {
		return nil, err
	}
	loc := now.Location()
	out := []OnCallLive{}
	for _, t := range teams {
		if !t.Enabled {
			continue
		}
		l := OnCallLive{Team: t, Users: users}
		p, b, _ := t.OnDuty(shifts, now, loc)
		if e, ok := users[p]; ok {
			l.Primary = &e
		}
		if e, ok := users[b]; ok {
			l.Backup = &e
		}
		if e, ok := users[t.LeadUser]; ok {
			l.Lead = &e
		}
		for _, id := range t.Members {
			if e, ok := users[id]; ok {
				l.All = append(l.All, e)
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// OnCallFingerprint names who is on duty in every team, so a change can regenerate the dialplan.
func OnCallFingerprint(live []OnCallLive) string {
	var b strings.Builder
	id := func(e *Engineer) string {
		if e == nil {
			return "0"
		}
		return strconv.FormatInt(e.ID, 10) + "/" + e.Mobile + "/" + e.Extension
	}
	for _, l := range live {
		b.WriteString(strconv.FormatInt(l.Team.ID, 10) + ":" + id(l.Primary) + "," + id(l.Backup) + ";")
	}
	return b.String()
}
