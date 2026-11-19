package pbx

import (
	"strings"
	"testing"
	"time"
)

func TestTimeConditionOpenAt(t *testing.T) {
	tc := TimeCondition{Mode: "auto", UseHolidays: true, Schedule: []TimeRange{{Days: []int{1, 2, 3, 4, 5}, From: "08:30", To: "17:00"}}}
	mon := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		hol  []Holiday
		want bool
	}{
		{mon, nil, true},
		{mon.Add(-31 * time.Minute), nil, false},
		{mon.Add(8 * time.Hour), nil, false},
		{mon.AddDate(0, 0, 6), nil, false}, // Sunday
		{mon, []Holiday{{Date: "2026-11-16"}}, false},
	}
	for i, c := range cases {
		if got := tc.OpenAt(c.at, c.hol); got != c.want {
			t.Errorf("case %d: %v", i, got)
		}
	}
	tc.Mode = "open"
	if !tc.OpenAt(mon.AddDate(0, 0, 6), nil) {
		t.Error("manual open")
	}
}

func TestSLAAndScreenNormalize(t *testing.T) {
	o := SLAOptions{}
	if err := o.normalize(); err != nil || o.Target != 20 || o.Monitored() {
		t.Fatalf("%v %+v", err, o)
	}
	if err := (&SLAOptions{Norm: 120}).normalize(); err == nil {
		t.Fatal("norm over 100 accepted")
	}
	w := WallScreen{Name: " TV ", Layouts: []string{"agents", "agents", "overview"}}
	if err := w.normalize(); err != nil || w.Name != "TV" || len(w.Layouts) != 2 || w.Theme != "dark" {
		t.Fatalf("%v %+v", err, w)
	}
	if err := (&WallScreen{Name: "x", Layouts: []string{"bad"}}).normalize(); err == nil {
		t.Fatal("bad layout accepted")
	}
	if err := (&WallScreen{Name: "x", Rotate: 3}).normalize(); err == nil {
		t.Fatal("rotate 3 s accepted")
	}
	k := NewWallKey()
	if len(k) != 23 || strings.Count(k, "-") != 3 || k == NewWallKey() {
		t.Fatalf("key %q", k)
	}
}
