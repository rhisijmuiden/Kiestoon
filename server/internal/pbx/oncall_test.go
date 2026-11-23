package pbx

import (
	"testing"
	"time"
)

func ams(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("no tzdata")
	}
	return loc
}

func weeklyTeam() *OnCallTeam {
	// Monday 23 November 2026 08:00 hand-over; Jan(1), Pieter(2), Fatima(3), Lars(4).
	return &OnCallTeam{ID: 1, Mode: "weekly", HandoverDay: 0, HandoverTime: "08:00", StartDate: "2026-11-25",
		Members: []int64{1, 2, 3, 4}, Backup: "next", LeadUser: 5}
}

func TestOnCallRotation(t *testing.T) {
	loc := ams(t)
	tm := weeklyTeam()
	// The start date is a Wednesday: the rotation starts on the Monday before it.
	if got := tm.handoverAt(0, loc); !got.Equal(time.Date(2026, 11, 23, 8, 0, 0, 0, loc)) {
		t.Fatalf("anchor %v", got)
	}
	cases := []struct {
		at      time.Time
		p, b    int64
		untilOK time.Time
	}{
		{time.Date(2026, 11, 23, 19, 42, 0, 0, loc), 1, 2, time.Date(2026, 11, 30, 8, 0, 0, 0, loc)},
		{time.Date(2026, 11, 30, 7, 59, 0, 0, loc), 1, 2, time.Date(2026, 11, 30, 8, 0, 0, 0, loc)},
		{time.Date(2026, 11, 30, 8, 0, 0, 0, loc), 2, 3, time.Date(2026, 12, 7, 8, 0, 0, 0, loc)},
		{time.Date(2026, 12, 21, 9, 0, 0, 0, loc), 1, 2, time.Date(2026, 12, 28, 8, 0, 0, 0, loc)},
		// Before the start the rotation runs backwards.
		{time.Date(2026, 11, 20, 12, 0, 0, 0, loc), 4, 1, time.Date(2026, 11, 23, 8, 0, 0, 0, loc)},
		// Over the switch to summer time the hand-over stays at 08:00 local time.
		{time.Date(2027, 3, 29, 8, 0, 0, 0, loc), tm.Members[mod(18, 4)], tm.Members[mod(19, 4)], time.Date(2027, 4, 5, 8, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		p, b, until := tm.OnDuty(nil, c.at, loc)
		if p != c.p || b != c.b || !until.Equal(c.untilOK) {
			t.Errorf("%v: got %d/%d until %v, want %d/%d until %v", c.at, p, b, until, c.p, c.b, c.untilOK)
		}
	}
}

func TestOnCallDaily(t *testing.T) {
	loc := ams(t)
	tm := &OnCallTeam{ID: 1, Mode: "daily", HandoverTime: "17:00", StartDate: "2026-11-23", Members: []int64{7, 8}, Backup: "none"}
	p, b, until := tm.OnDuty(nil, time.Date(2026, 11, 24, 16, 0, 0, 0, loc), loc)
	if p != 7 || b != 0 || !until.Equal(time.Date(2026, 11, 24, 17, 0, 0, 0, loc)) {
		t.Fatalf("got %d/%d until %v", p, b, until)
	}
	if p, _, _ := tm.OnDuty(nil, time.Date(2026, 11, 24, 17, 0, 0, 0, loc), loc); p != 8 {
		t.Fatalf("after hand-over: %d", p)
	}
}

func TestOnCallShiftsBeatRotation(t *testing.T) {
	loc := ams(t)
	tm := weeklyTeam()
	d := func(day, h int) time.Time { return time.Date(2026, 11, day, h, 0, 0, 0, loc) }
	shifts := []OnCallShift{
		// Fatima takes Wednesday evening over from Jan.
		{ID: 10, TeamID: 1, Line: 1, UserID: 3, Start: d(25, 17), End: d(26, 8), Kind: "handover", Status: "approved", CreatedAt: "1"},
		// A newer take-over inside it wins.
		{ID: 11, TeamID: 1, Line: 1, UserID: 4, Start: d(25, 20), End: d(25, 22), Kind: "takeover", Status: "approved", CreatedAt: "2"},
		// Pending and other teams do not count.
		{ID: 12, TeamID: 1, Line: 1, UserID: 2, Start: d(24, 0), End: d(24, 23), Kind: "swap", Status: "pending", CreatedAt: "3"},
		{ID: 13, TeamID: 9, Line: 1, UserID: 2, Start: d(24, 0), End: d(24, 23), Kind: "shift", Status: "approved", CreatedAt: "4"},
	}
	check := func(at time.Time, want int64, until time.Time) {
		t.Helper()
		p, _, u := tm.OnDuty(shifts, at, loc)
		if p != want || !u.Equal(until) {
			t.Errorf("%v: got %d until %v, want %d until %v", at, p, u, want, until)
		}
	}
	check(d(24, 12), 1, d(25, 17))
	check(d(25, 18), 3, d(25, 20))
	check(d(25, 21), 4, d(25, 22))
	check(d(25, 22), 3, d(26, 8))
	check(d(26, 8), 1, d(30, 8))

	segs := tm.Timeline(shifts, d(25, 0), d(27, 0), loc)
	var line1 []OnCallSegment
	for _, s := range segs {
		if s.Line == 1 {
			line1 = append(line1, s)
		}
	}
	want := []struct {
		user int64
		kind string
	}{{1, "rotation"}, {3, "handover"}, {4, "takeover"}, {3, "handover"}, {1, "rotation"}}
	if len(line1) != len(want) {
		t.Fatalf("segments: %+v", line1)
	}
	for i, w := range want {
		if line1[i].UserID != w.user || line1[i].Kind != w.kind {
			t.Errorf("segment %d: %+v", i, line1[i])
		}
	}
}

func TestOnCallManualGaps(t *testing.T) {
	loc := ams(t)
	tm := &OnCallTeam{ID: 2, Mode: "manual", HandoverTime: "08:00", StartDate: "2026-11-23", Backup: "fixed", BackupUser: 5}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, loc)
	shifts := []OnCallShift{{ID: 1, TeamID: 2, Line: 1, UserID: 1, Start: from.Add(17 * time.Hour), End: from.Add(32 * time.Hour), Kind: "shift", Status: "approved"}}
	segs := tm.Timeline(shifts, from, from.Add(48*time.Hour), loc)
	gaps := 0
	for _, s := range segs {
		if s.Line == 1 && s.Kind == "gap" {
			gaps++
		}
		if s.Line == 2 && s.UserID != 5 {
			t.Errorf("fixed backup: %+v", s)
		}
	}
	if gaps != 2 {
		t.Fatalf("want 2 gaps, got %+v", segs)
	}
	if p, b, _ := tm.OnDuty(shifts, from.Add(20*time.Hour), loc); p != 1 || b != 5 {
		t.Fatalf("got %d/%d", p, b)
	}
}
