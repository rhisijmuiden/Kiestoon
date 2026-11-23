package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// The on-call module (storingsdienst): teams with a rotation, the escalation the PBX runs, and the
// swaps, hand-overs and take-overs engineers arrange among themselves in the portal.

func (s *Server) onCallRoutes(mux *http.ServeMux, admin, full func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/oncall/teams", admin(s.onCallTeams))
	mux.Handle("POST /api/oncall/teams", admin(s.onCallSave))
	mux.Handle("PUT /api/oncall/teams/{id}", admin(s.onCallSave))
	mux.Handle("DELETE /api/oncall/teams/{id}", admin(s.deleter("oncall", s.PBX.DeleteOnCallTeam)))
	mux.Handle("POST /api/oncall/teams/{id}/ical", admin(s.onCallNewToken))
	mux.Handle("GET /api/oncall/teams/{id}/timeline", admin(s.onCallTimeline))
	mux.Handle("POST /api/oncall/teams/{id}/shifts", admin(s.onCallAddShift))
	mux.Handle("GET /api/oncall/engineers", admin(s.onCallEngineers))
	mux.Handle("PUT /api/oncall/engineers/{id}/mobile", admin(s.onCallSetMobile))

	mux.Handle("GET /api/portal/oncall", full(s.portalOnCall))
	mux.Handle("PUT /api/portal/oncall/mobile", full(s.portalOnCallMobile))
	mux.Handle("POST /api/portal/oncall/requests", full(s.portalOnCallRequest))
	mux.Handle("POST /api/portal/oncall/takeover", full(s.portalOnCallTakeover))
	mux.Handle("POST /api/oncall/shifts/{id}/respond", full(s.onCallRespond))
	mux.Handle("POST /api/oncall/shifts/{id}/decide", full(s.onCallDecide))
	mux.Handle("DELETE /api/oncall/shifts/{id}", full(s.onCallDeleteShift))

	// The calendar link (iCal) for Outlook, Google and Apple calendars: the token is the secret.
	mux.HandleFunc("GET /api/oncall/ical/{token}", s.onCallICal)
}

type ocUser struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Mobile    string `json:"mobile"`
	Extension string `json:"extension"`
	Email     bool   `json:"email"`
}

func ocUsers(m map[int64]pbx.Engineer, withMobile bool) []ocUser {
	out := []ocUser{}
	for _, e := range m {
		u := ocUser{ID: e.ID, Name: e.Name, Extension: e.Extension, Email: e.Email != ""}
		if withMobile {
			u.Mobile = e.Mobile
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (s *Server) onCallTeams(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.OnCallTeams(r.Context())
	if err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) onCallSave(w http.ResponseWriter, r *http.Request) {
	var t pbx.OnCallTeam
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t.ID = pathID(r)
	if t.ID != 0 {
		old, err := s.PBX.OnCallTeam(r.Context(), t.ID)
		if err != nil {
			s.saveErr(w, err)
			return
		}
		t.ICalToken = old.ICalToken
	}
	if err := s.PBX.SaveOnCallTeam(r.Context(), s.plan(r.Context()), &t); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "oncall", t.Number, t)
}

func (s *Server) onCallNewToken(w http.ResponseWriter, r *http.Request) {
	tok, err := s.PBX.NewOnCallToken(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.ical_reset", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]string{"icalToken": tok})
}

func (s *Server) onCallEngineers(w http.ResponseWriter, r *http.Request) {
	m, err := s.PBX.Engineers(r.Context())
	if err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ocUsers(m, true))
}

func (s *Server) onCallSetMobile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mobile string `json:"mobile"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	m, err := s.PBX.SetMobile(r.Context(), pathID(r), in.Mobile)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.mobile", r.PathValue("id"))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]string{"mobile": m})
}

// ocView is a team as the admin page and the portal show it.
type ocView struct {
	Team     pbx.OnCallTeam      `json:"team"`
	Segments []pbx.OnCallSegment `json:"segments"`
	Shifts   []pbx.OnCallShift   `json:"shifts"`
	Primary  int64               `json:"primary"`
	Backup   int64               `json:"backup"`
	Until    time.Time           `json:"until"`
	Warnings []ocWarning         `json:"warnings"`
}

type ocWarning struct {
	Kind   string    `json:"kind"` // gap, nomobile
	Line   int       `json:"line,omitempty"`
	UserID int64     `json:"userId,omitempty"`
	Start  time.Time `json:"start,omitzero"`
	End    time.Time `json:"end,omitzero"`
}

func (s *Server) onCallView(ctx context.Context, t *pbx.OnCallTeam, users map[int64]pbx.Engineer, from, to, now time.Time) (*ocView, error) {
	loc := s.loc(ctx)
	shifts, err := s.PBX.OnCallShifts(ctx, t.ID, from)
	if err != nil {
		return nil, err
	}
	v := &ocView{Team: *t, Shifts: []pbx.OnCallShift{}, Warnings: []ocWarning{}}
	for _, sh := range shifts {
		if sh.Status != "declined" && sh.Status != "cancelled" {
			v.Shifts = append(v.Shifts, sh)
		}
	}
	v.Segments = t.Timeline(shifts, from.In(loc), to.In(loc), loc)
	v.Primary, v.Backup, v.Until = t.OnDuty(shifts, now.In(loc), loc)
	// Gaps in the coming two weeks, and engineers who cannot be called.
	for _, sg := range t.Timeline(shifts, now.In(loc), now.Add(14*24*time.Hour).In(loc), loc) {
		if sg.Kind == "gap" && (sg.Line == 1 || t.Backup != "none") {
			v.Warnings = append(v.Warnings, ocWarning{Kind: "gap", Line: sg.Line, Start: sg.Start, End: sg.End})
		}
	}
	seen := map[int64]bool{}
	for _, id := range append(append([]int64{}, t.Members...), t.LeadUser, t.BackupUser) {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		if e, ok := users[id]; ok && e.Mobile == "" && (e.Extension == "" || !t.RingExtension) {
			v.Warnings = append(v.Warnings, ocWarning{Kind: "nomobile", UserID: id})
		}
	}
	return v, nil
}

func (s *Server) onCallTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.PBX.OnCallTeam(ctx, pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	users, err := s.PBX.Engineers(ctx)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	now := time.Now()
	from, days := ocRange(r, now, s.loc(ctx))
	v, err := s.onCallView(ctx, t, users, from, from.AddDate(0, 0, days), now)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"view": v, "users": ocUsers(users, true), "now": now})
}

// ocRange reads ?from=YYYY-MM-DD&days=N (default: today, 14 days).
func ocRange(r *http.Request, now time.Time, loc *time.Location) (time.Time, int) {
	n := now.In(loc)
	from := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if d, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("from"), loc); err == nil {
		from = d
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 62 {
		days = 14
	}
	return from, days
}

func (s *Server) onCallAddShift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.PBX.OnCallTeam(ctx, pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	var sh pbx.OnCallShift
	if err := decode(r, &sh); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	me := sessionFrom(r).User
	if sh.Kind != "shift" {
		sh.Kind = "override"
	}
	sh.ID, sh.PairID, sh.Status, sh.Accepted, sh.RequestedBy, sh.DecidedBy = 0, 0, "approved", true, me.ID, me.ID
	loc := s.loc(ctx)
	shifts, _ := s.PBX.OnCallShifts(ctx, t.ID, time.Now().Add(-time.Minute))
	if p, b, _ := t.OnDuty(shifts, sh.Start.In(loc), loc); sh.Line == 1 {
		sh.FromUser = p
	} else {
		sh.FromUser = b
	}
	if err := s.PBX.AddOnCallShift(ctx, t, &sh); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.shift", fmt.Sprintf("%s line %d user %d %s-%s", t.Name, sh.Line, sh.UserID, sh.Start.Format(time.RFC3339), sh.End.Format(time.RFC3339)))
	s.onCallChanged(ctx)
	users, _ := s.PBX.Engineers(ctx)
	if sh.UserID != me.ID {
		s.ocMailShift(ctx, users[sh.UserID], t, &sh, "assigned")
	}
	if sh.FromUser != 0 && sh.FromUser != sh.UserID && sh.FromUser != me.ID {
		s.ocMailShift(ctx, users[sh.FromUser], t, &sh, "relieved")
	}
	writeJSON(w, http.StatusOK, sh)
}

// onCallChanged regenerates the dialplan when who is on duty changed.
func (s *Server) onCallChanged(ctx context.Context) {
	s.Ast.Changed()
}

// ---- portal -----------------------------------------------------------------

func (s *Server) portalOnCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := sessionFrom(r).User
	teams, err := s.PBX.OnCallTeams(ctx)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	users, err := s.PBX.Engineers(ctx)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	now := time.Now()
	loc := s.loc(ctx)
	n := now.In(loc)
	from := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	views := []*ocView{}
	for i := range teams {
		t := &teams[i]
		if !t.Enabled || !(slices.Contains(t.Members, me.ID) || t.LeadUser == me.ID || t.BackupUser == me.ID) {
			continue
		}
		// Start two weeks back, so the shift someone is on now shows its real start.
		v, err := s.onCallView(ctx, t, users, from.AddDate(0, 0, -14), from.AddDate(0, 0, 62), now)
		if err != nil {
			s.saveErr(w, err)
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"me": me.ID, "mobile": users[me.ID].Mobile, "teams": views, "users": ocUsers(users, false), "now": now})
}

func (s *Server) portalOnCallMobile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mobile string `json:"mobile"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	me := sessionFrom(r).User
	m, err := s.PBX.SetMobile(r.Context(), me.ID, in.Mobile)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.mobile", strconv.FormatInt(me.ID, 10))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]string{"mobile": m})
}

// coveredBy reports whether one engineer is on a line for the whole of [a, b).
func coveredBy(segs []pbx.OnCallSegment, line int, user int64, a, b time.Time) bool {
	at := a
	for _, sg := range segs {
		if sg.Line != line || !sg.End.After(at) || !sg.Start.Before(b) {
			continue
		}
		if sg.Start.After(at) || sg.UserID != user {
			return false
		}
		at = sg.End
		if !at.Before(b) {
			return true
		}
	}
	return !at.Before(b)
}

type ocRequest struct {
	TeamID int64 `json:"teamId"`
	// Kind: handover (the colleague takes over) or swap (and I take a shift of theirs).
	Kind      string    `json:"kind"`
	Line      int       `json:"line"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	To        int64     `json:"to"`
	SwapStart time.Time `json:"swapStart"`
	SwapEnd   time.Time `json:"swapEnd"`
	Note      string    `json:"note"`
}

func (s *Server) portalOnCallRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in ocRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	me := sessionFrom(r).User
	t, err := s.PBX.OnCallTeam(ctx, in.TeamID)
	if err != nil || !t.Enabled {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !slices.Contains(t.Members, me.ID) && t.LeadUser != me.ID && t.BackupUser != me.ID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if in.To == me.ID || (!slices.Contains(t.Members, in.To) && t.LeadUser != in.To && t.BackupUser != in.To) {
		s.saveErr(w, &pbx.ValidationError{Code: "unknown_member", Field: "to"})
		return
	}
	now := time.Now()
	if !in.End.After(in.Start) || !in.End.After(now) {
		s.saveErr(w, &pbx.ValidationError{Code: "invalid", Field: "end"})
		return
	}
	if in.Start.Before(now) {
		in.Start = now.Truncate(time.Minute)
	}
	loc := s.loc(ctx)
	shifts, err := s.PBX.OnCallShifts(ctx, t.ID, now.Add(-time.Minute))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	horizon := maxT(in.End, in.SwapEnd)
	segs := t.Timeline(shifts, now.In(loc), horizon.In(loc), loc)
	if !coveredBy(segs, in.Line, me.ID, in.Start, in.End) {
		s.saveErr(w, &pbx.ValidationError{Code: "not_on_duty", Field: "start"})
		return
	}
	a := pbx.OnCallShift{Line: in.Line, UserID: in.To, Start: in.Start, End: in.End, Kind: "handover", Status: "pending",
		RequestedBy: me.ID, FromUser: me.ID, Note: in.Note}
	var b *pbx.OnCallShift
	switch in.Kind {
	case "handover":
	case "swap":
		a.Kind = "swap"
		if !in.SwapEnd.After(in.SwapStart) || in.SwapStart.Before(now) {
			s.saveErr(w, &pbx.ValidationError{Code: "invalid", Field: "swapStart"})
			return
		}
		if !coveredBy(segs, in.Line, in.To, in.SwapStart, in.SwapEnd) {
			s.saveErr(w, &pbx.ValidationError{Code: "not_on_duty", Field: "swapStart"})
			return
		}
		b = &pbx.OnCallShift{Line: in.Line, UserID: me.ID, Start: in.SwapStart, End: in.SwapEnd, Kind: "swap", Status: "pending",
			RequestedBy: me.ID, FromUser: in.To, Note: in.Note}
	default:
		s.saveErr(w, &pbx.ValidationError{Code: "invalid", Field: "kind"})
		return
	}
	if err := s.PBX.AddOnCallShift(ctx, t, &a); err != nil {
		s.saveErr(w, err)
		return
	}
	if b != nil {
		if err := s.PBX.AddOnCallShift(ctx, t, b); err != nil {
			_ = s.PBX.DeleteOnCallShift(ctx, &a)
			s.saveErr(w, err)
			return
		}
		_ = s.PBX.LinkOnCallShifts(ctx, a.ID, b.ID)
		a.PairID = b.ID
	}
	s.audit(r, "oncall.request", fmt.Sprintf("%s %s to %d", t.Name, a.Kind, in.To))
	users, _ := s.PBX.Engineers(ctx)
	s.ocMailRequest(ctx, users[in.To], users[me.ID], t, &a, b)
	writeJSON(w, http.StatusOK, a)
}

func maxT(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// portalOnCallTakeover lets an engineer take over at once, for example when a colleague is ill.
func (s *Server) portalOnCallTakeover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		TeamID int64     `json:"teamId"`
		Line   int       `json:"line"`
		End    time.Time `json:"end"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	me := sessionFrom(r).User
	t, err := s.PBX.OnCallTeam(ctx, in.TeamID)
	if err != nil || !t.Enabled {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !slices.Contains(t.Members, me.ID) && t.LeadUser != me.ID && t.BackupUser != me.ID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if in.Line == 0 {
		in.Line = 1
	}
	now := time.Now().Truncate(time.Minute)
	loc := s.loc(ctx)
	shifts, _ := s.PBX.OnCallShifts(ctx, t.ID, now.Add(-time.Minute))
	p, b, until := t.OnDuty(shifts, now.In(loc), loc)
	from := p
	if in.Line == 2 {
		from = b
	}
	if from == me.ID {
		s.saveErr(w, &pbx.ValidationError{Code: "already_on_duty", Field: "line"})
		return
	}
	if in.End.IsZero() || !in.End.After(now) {
		in.End = until
	}
	sh := pbx.OnCallShift{Line: in.Line, UserID: me.ID, Start: now, End: in.End, Kind: "takeover", Status: "approved", Accepted: true,
		RequestedBy: me.ID, FromUser: from}
	if err := s.PBX.AddOnCallShift(ctx, t, &sh); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.takeover", fmt.Sprintf("%s line %d", t.Name, in.Line))
	s.onCallChanged(ctx)
	users, _ := s.PBX.Engineers(ctx)
	for _, id := range []int64{from, t.LeadUser} {
		if id != 0 && id != me.ID {
			s.ocMailShift(ctx, users[id], t, &sh, "takenover")
		}
	}
	writeJSON(w, http.StatusOK, sh)
}

// shiftFor loads a shift and its team.
func (s *Server) shiftFor(w http.ResponseWriter, r *http.Request) (*pbx.OnCallShift, *pbx.OnCallTeam) {
	sh, err := s.PBX.OnCallShift(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return nil, nil
	}
	t, err := s.PBX.OnCallTeam(r.Context(), sh.TeamID)
	if err != nil {
		s.saveErr(w, err)
		return nil, nil
	}
	return sh, t
}

func isAdmin(u *auth.User) bool { return auth.RoleRank(u.Role) >= auth.RoleRank("admin") }

// onCallRespond: the colleague accepts or declines a request.
func (s *Server) onCallRespond(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Accept bool `json:"accept"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sh, t := s.shiftFor(w, r)
	if sh == nil {
		return
	}
	me := sessionFrom(r).User
	// The colleague asked is the one who takes over (the first half of a swap is theirs).
	target := sh.UserID
	if sh.Kind == "swap" && sh.RequestedBy == sh.UserID {
		target = sh.FromUser
	}
	if target != me.ID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if sh.Status != "pending" || sh.Accepted {
		writeErr(w, http.StatusConflict, "already_decided")
		return
	}
	if !in.Accept {
		sh.Status = "declined"
	} else {
		sh.Accepted = true
		if t.SwapApproval == "none" {
			sh.Status = "approved"
		}
	}
	if err := s.PBX.UpdateOnCallShift(ctx, sh); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.respond", fmt.Sprintf("%d %s", sh.ID, sh.Status))
	users, _ := s.PBX.Engineers(ctx)
	requester := users[sh.RequestedBy]
	switch {
	case sh.Status == "declined":
		s.ocMailShift(ctx, requester, t, sh, "declined")
	case sh.Status == "approved":
		s.onCallChanged(ctx)
		s.ocMailShift(ctx, requester, t, sh, "approved")
	default:
		s.ocMailApproval(ctx, t, users, sh)
	}
	writeJSON(w, http.StatusOK, sh)
}

// onCallDecide: the team lead (or an admin) approves or rejects a request.
func (s *Server) onCallDecide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Approve bool `json:"approve"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sh, t := s.shiftFor(w, r)
	if sh == nil {
		return
	}
	me := sessionFrom(r).User
	lead := t.SwapApproval == "lead" && t.LeadUser == me.ID
	if !isAdmin(me) && !lead {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if sh.Status != "pending" {
		writeErr(w, http.StatusConflict, "already_decided")
		return
	}
	if !sh.Accepted && !isAdmin(me) {
		writeErr(w, http.StatusConflict, "not_accepted")
		return
	}
	sh.DecidedBy = me.ID
	if in.Approve {
		sh.Status, sh.Accepted = "approved", true
	} else {
		sh.Status = "declined"
	}
	if err := s.PBX.UpdateOnCallShift(ctx, sh); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.decide", fmt.Sprintf("%d %s", sh.ID, sh.Status))
	if sh.Status == "approved" {
		s.onCallChanged(ctx)
	}
	users, _ := s.PBX.Engineers(ctx)
	for _, id := range []int64{sh.RequestedBy, sh.UserID, sh.FromUser} {
		if id != me.ID && id != 0 {
			s.ocMailShift(ctx, users[id], t, sh, sh.Status)
		}
	}
	writeJSON(w, http.StatusOK, sh)
}

// onCallDeleteShift: an admin removes any shift; the requester withdraws a pending request.
func (s *Server) onCallDeleteShift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sh, t := s.shiftFor(w, r)
	if sh == nil {
		return
	}
	me := sessionFrom(r).User
	if !isAdmin(me) && !(sh.RequestedBy == me.ID && (sh.Status == "pending" || sh.Kind == "takeover")) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.PBX.DeleteOnCallShift(ctx, sh); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "oncall.shift_deleted", fmt.Sprintf("%s %d", t.Name, sh.ID))
	if sh.Status == "approved" {
		s.onCallChanged(ctx)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- calendar ---------------------------------------------------------------

func icsEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", "").Replace(s)
}

func (s *Server) onCallICal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.PBX.OnCallTeamByToken(ctx, strings.TrimSuffix(r.PathValue("token"), ".ics"))
	if err != nil {
		time.Sleep(200 * time.Millisecond)
		http.NotFound(w, r)
		return
	}
	users, _ := s.PBX.Engineers(ctx)
	loc := s.loc(ctx)
	now := time.Now()
	from := now.AddDate(0, 0, -14)
	shifts, _ := s.PBX.OnCallShifts(ctx, t.ID, from)
	st, _ := pbx.LoadSettings(ctx, s.DB)
	nl := st.Language == "nl"
	lineName := map[int]string{1: "1st line", 2: "backup"}
	if nl {
		lineName = map[int]string{1: "1e lijn", 2: "back-up"}
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//RHIS//Kiestoon//EN\r\nCALSCALE:GREGORIAN\r\n")
	b.WriteString("X-WR-CALNAME:" + icsEscape(t.Name) + "\r\n")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, sg := range t.Timeline(shifts, from.In(loc), now.AddDate(0, 0, 120).In(loc), loc) {
		if sg.UserID == 0 {
			continue
		}
		name := users[sg.UserID].Name
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:kiestoon-oc-%d-%d-%d-%d@kiestoon\r\n", t.ID, sg.Line, sg.UserID, sg.Start.Unix())
		b.WriteString("DTSTAMP:" + stamp + "\r\n")
		b.WriteString("DTSTART:" + sg.Start.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTEND:" + sg.End.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("SUMMARY:" + icsEscape(t.Name+" · "+lineName[sg.Line]+": "+name) + "\r\n")
		b.WriteString("TRANSP:TRANSPARENT\r\nEND:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// ---- background: dialplan at hand-overs, reminders, gaps --------------------

func (s *Server) runOnCall(ctx context.Context) {
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	last, lastMail := "", time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		now := time.Now().In(s.loc(ctx))
		live, err := s.PBX.OnCallLive(ctx, now)
		if err != nil {
			continue
		}
		fp := pbx.OnCallFingerprint(live)
		if last != "" && fp != last {
			s.Ast.Changed()
		}
		last = fp
		if now.Sub(lastMail) >= 5*time.Minute {
			lastMail = now
			s.onCallMails(ctx, now)
		}
	}
}

// notifyOnce remembers that a mail went out; it reports false when it already did.
func (s *Server) notifyOnce(ctx context.Context, key string, now time.Time) bool {
	res, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO oncall_notified(key, at) VALUES (?, ?)`, key, now.UTC().Format(time.RFC3339))
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (s *Server) onCallMails(ctx context.Context, now time.Time) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM oncall_notified WHERE at < ?`, now.AddDate(0, 0, -120).UTC().Format(time.RFC3339))
	teams, err := s.PBX.OnCallTeams(ctx)
	if err != nil {
		return
	}
	users, err := s.PBX.Engineers(ctx)
	if err != nil {
		return
	}
	loc := now.Location()
	for i := range teams {
		t := &teams[i]
		if !t.Enabled {
			continue
		}
		shifts, err := s.PBX.OnCallShifts(ctx, t.ID, now.Add(-time.Minute))
		if err != nil {
			continue
		}
		segs := t.Timeline(shifts, now, now.Add(7*24*time.Hour), loc)
		for _, sg := range segs {
			switch {
			case sg.Kind == "gap" && (sg.Line == 1 || t.Backup != "none"):
				key := fmt.Sprintf("gap:%d:%d:%d", t.ID, sg.Line, sg.Start.Unix())
				if sg.Start.After(now) && s.notifyOnce(ctx, key, now) {
					s.ocMailGap(ctx, t, users, sg)
				}
			case t.Reminders && sg.UserID != 0 && sg.Start.After(now) && sg.Start.Sub(now) <= 24*time.Hour:
				// Only the start of a stretch: not when the same engineer simply carries on.
				prev := t.Timeline(shifts, sg.Start.Add(-time.Minute), sg.Start, loc)
				carry := false
				for _, p := range prev {
					if p.Line == sg.Line && p.UserID == sg.UserID {
						carry = true
					}
				}
				key := fmt.Sprintf("rem:%d:%d:%d:%d", t.ID, sg.Line, sg.UserID, sg.Start.Unix())
				if !carry && s.notifyOnce(ctx, key, now) {
					end := sg.End
					for _, n := range segs {
						if n.Line == sg.Line && n.UserID == sg.UserID && n.Start.Equal(end) {
							end = n.End
						}
					}
					x := sg
					x.End = end
					s.ocMailReminder(ctx, users[sg.UserID], t, x)
				}
			}
		}
	}
}

// ---- mails ------------------------------------------------------------------

var ocDays = map[string][]string{
	"en": {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	"nl": {"zo", "ma", "di", "wo", "do", "vr", "za"},
}
var ocMonths = map[string][]string{
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"nl": {"jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
}

func ocLang(e pbx.Engineer) string {
	if e.Language == "nl" {
		return "nl"
	}
	return "en"
}

func (s *Server) ocTime(ctx context.Context, lang string, t time.Time) string {
	t = t.In(s.loc(ctx))
	return fmt.Sprintf("%s %d %s %02d:%02d", ocDays[lang][t.Weekday()], t.Day(), ocMonths[lang][t.Month()-1], t.Hour(), t.Minute())
}

func (s *Server) ocPeriod(ctx context.Context, lang string, a, b time.Time) string {
	return s.ocTime(ctx, lang, a) + " → " + s.ocTime(ctx, lang, b)
}

func ocLine(lang string, line int) string {
	if lang == "nl" {
		if line == 2 {
			return "back-up (2e lijn)"
		}
		return "1e lijn"
	}
	if line == 2 {
		return "backup (2nd line)"
	}
	return "1st line"
}

func (s *Server) ocSend(ctx context.Context, to pbx.Engineer, subject, body string) {
	if to.Email == "" {
		return
	}
	link := ""
	if base, _ := store.GetSetting(ctx, s.DB, "public_url", ""); base != "" {
		link = "\n\n" + strings.TrimRight(base, "/") + "/me/oncall"
	}
	if err := s.Mail.SendText(ctx, "oncall", to.Email, subject, body+link+"\n\n-- \nKiestoon\n"); err != nil && !errors.Is(err, context.Canceled) {
		s.Log.Warn("oncall mail", "to", to.Email, "err", err)
	}
}

func (s *Server) ocMailRequest(ctx context.Context, to, from pbx.Engineer, t *pbx.OnCallTeam, a, b *pbx.OnCallShift) {
	l := ocLang(to)
	var subj, body string
	if l == "nl" {
		subj = fmt.Sprintf("%s: %s vraagt of je een dienst overneemt", t.Name, from.Name)
		body = fmt.Sprintf("%s vraagt of jij de storingsdienst (%s) overneemt:\n%s", from.Name, ocLine(l, a.Line), s.ocPeriod(ctx, l, a.Start, a.End))
		if b != nil {
			body += fmt.Sprintf("\n\nIn ruil neemt %s jouw dienst over:\n%s", from.Name, s.ocPeriod(ctx, l, b.Start, b.End))
		}
		if a.Note != "" {
			body += "\n\nBericht: " + a.Note
		}
		body += "\n\nAccepteren of weigeren doe je in het portaal onder Mijn storingsdiensten."
	} else {
		subj = fmt.Sprintf("%s: %s asks you to take over a shift", t.Name, from.Name)
		body = fmt.Sprintf("%s asks you to take over on-call duty (%s):\n%s", from.Name, ocLine(l, a.Line), s.ocPeriod(ctx, l, a.Start, a.End))
		if b != nil {
			body += fmt.Sprintf("\n\nIn return %s takes over your shift:\n%s", from.Name, s.ocPeriod(ctx, l, b.Start, b.End))
		}
		if a.Note != "" {
			body += "\n\nMessage: " + a.Note
		}
		body += "\n\nAccept or decline in the portal under My on-call shifts."
	}
	s.ocSend(ctx, to, subj, body)
}

func (s *Server) ocMailApproval(ctx context.Context, t *pbx.OnCallTeam, users map[int64]pbx.Engineer, sh *pbx.OnCallShift) {
	from, to := users[sh.FromUser], users[sh.UserID]
	if sh.Kind == "swap" && sh.RequestedBy == sh.UserID {
		from, to = users[sh.UserID], users[sh.FromUser]
	}
	subjEN := fmt.Sprintf("%s: approve a swap between %s and %s", t.Name, from.Name, to.Name)
	bodyEN := fmt.Sprintf("%s and %s agreed on a change of the on-call schedule. Approve or reject it on the On-call page.", from.Name, to.Name)
	subjNL := fmt.Sprintf("%s: keur een ruil tussen %s en %s goed", t.Name, from.Name, to.Name)
	bodyNL := fmt.Sprintf("%s en %s hebben een wijziging van het storingsrooster afgesproken. Keur die goed of af op de pagina Storingsdienst.", from.Name, to.Name)
	if t.SwapApproval == "lead" {
		if lead, ok := users[t.LeadUser]; ok {
			if ocLang(lead) == "nl" {
				s.ocSend(ctx, lead, subjNL, bodyNL)
			} else {
				s.ocSend(ctx, lead, subjEN, bodyEN)
			}
			return
		}
	}
	s.Mail.Notify(ctx, subjEN, bodyEN+"\n\n"+bodyNL)
}

func (s *Server) ocMailShift(ctx context.Context, to pbx.Engineer, t *pbx.OnCallTeam, sh *pbx.OnCallShift, what string) {
	if to.ID == 0 {
		return
	}
	l := ocLang(to)
	period := s.ocPeriod(ctx, l, sh.Start, sh.End)
	users, _ := s.PBX.Engineers(ctx)
	who := users[sh.UserID].Name
	texts := map[string][2]string{
		"assigned":  {"You are on call: %[2]s, %[3]s", "Je hebt storingsdienst: %[2]s, %[3]s"},
		"relieved":  {"%[1]s takes over your on-call duty: %[2]s, %[3]s", "%[1]s neemt je storingsdienst over: %[2]s, %[3]s"},
		"takenover": {"%[1]s has taken over on-call duty now: %[2]s, %[3]s", "%[1]s heeft de storingsdienst nu overgenomen: %[2]s, %[3]s"},
		"approved":  {"Approved: %[1]s is on call %[2]s, %[3]s", "Goedgekeurd: %[1]s heeft storingsdienst %[2]s, %[3]s"},
		"declined":  {"Declined: the change for %[2]s, %[3]s does not go ahead", "Geweigerd: de wijziging voor %[2]s, %[3]s gaat niet door"},
	}
	tx, ok := texts[what]
	if !ok {
		return
	}
	i := 0
	if l == "nl" {
		i = 1
	}
	line := fmt.Sprintf(tx[i], who, ocLine(l, sh.Line), period)
	s.ocSend(ctx, to, t.Name+": "+strings.SplitN(line, ":", 2)[0], line)
}

func (s *Server) ocMailReminder(ctx context.Context, to pbx.Engineer, t *pbx.OnCallTeam, sg pbx.OnCallSegment) {
	l := ocLang(to)
	period := s.ocPeriod(ctx, l, sg.Start, sg.End)
	if l == "nl" {
		body := fmt.Sprintf("Herinnering: je hebt storingsdienst (%s)\n%s", ocLine(l, sg.Line), period)
		if to.Mobile == "" {
			body += "\n\nLet op: er staat nog geen mobiel nummer in je profiel. Vul het in onder Mijn storingsdiensten."
		} else {
			body += "\n\nJe wordt gebeld op " + to.Mobile + "."
		}
		s.ocSend(ctx, to, t.Name+": je dienst begint "+s.ocTime(ctx, l, sg.Start), body)
		return
	}
	body := fmt.Sprintf("Reminder: you are on call (%s)\n%s", ocLine(l, sg.Line), period)
	if to.Mobile == "" {
		body += "\n\nNote: your profile has no mobile number yet. Add it under My on-call shifts."
	} else {
		body += "\n\nYou will be called on " + to.Mobile + "."
	}
	s.ocSend(ctx, to, t.Name+": your shift starts "+s.ocTime(ctx, l, sg.Start), body)
}

func (s *Server) ocMailGap(ctx context.Context, t *pbx.OnCallTeam, users map[int64]pbx.Engineer, sg pbx.OnCallSegment) {
	en := fmt.Sprintf("Nobody is on call (%s) for %s:\n%s\n\nPlan someone on the On-call page.", ocLine("en", sg.Line), t.Name, s.ocPeriod(ctx, "en", sg.Start, sg.End))
	nl := fmt.Sprintf("Niemand heeft storingsdienst (%s) voor %s:\n%s\n\nPlan iemand in op de pagina Storingsdienst.", ocLine("nl", sg.Line), t.Name, s.ocPeriod(ctx, "nl", sg.Start, sg.End))
	if lead, ok := users[t.LeadUser]; ok && lead.Email != "" {
		if ocLang(lead) == "nl" {
			s.ocSend(ctx, lead, t.Name+": gat in het rooster", nl)
		} else {
			s.ocSend(ctx, lead, t.Name+": gap in the schedule", en)
		}
		return
	}
	s.Mail.Notify(ctx, t.Name+": gap in the on-call schedule", en+"\n\n"+nl)
}
