package pbx

import (
	"testing"
	"time"
)

func TestReportDueAndRange(t *testing.T) {
	loc := time.UTC
	mon := time.Date(2026, 11, 16, 8, 30, 0, 0, loc) // Monday
	w := Report{Period: ReportWeekly, Time: "08:00", Weekday: 1, Enabled: true}
	if !w.Due(mon) || w.Due(mon.Add(-time.Hour)) || w.Due(mon.AddDate(0, 0, 1)) {
		t.Fatal("weekly due")
	}
	w.LastSent = mon.Add(-10 * time.Minute).Format(time.RFC3339)
	if w.Due(mon) {
		t.Fatal("sent twice")
	}
	from, to, pf, pt := w.Range(mon)
	if from.Format("2006-01-02") != "2026-11-09" || to.Format("2006-01-02") != "2026-11-16" || pf.Format("2006-01-02") != "2026-11-02" || !pt.Equal(from) {
		t.Fatalf("week range %v %v %v %v", from, to, pf, pt)
	}
	d := Report{Period: ReportDaily, Time: "18:00", Workdays: true, Enabled: true}
	if d.Due(time.Date(2026, 11, 15, 19, 0, 0, 0, loc)) || !d.Due(time.Date(2026, 11, 16, 19, 0, 0, 0, loc)) {
		t.Fatal("daily workdays")
	}
	m := Report{Period: ReportMonthly, Time: "07:00", Enabled: true}
	first := time.Date(2026, 12, 1, 7, 5, 0, 0, loc)
	if !m.Due(first) || m.Due(first.AddDate(0, 0, 1)) {
		t.Fatal("monthly due")
	}
	from, to, pf, _ = m.Range(first)
	if from.Format("2006-01-02") != "2026-11-01" || to.Format("2006-01-02") != "2026-12-01" || pf.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("month range %v %v %v", from, to, pf)
	}
	m.Enabled = false
	if m.Due(first) {
		t.Fatal("disabled report due")
	}
}

func TestReportNormalize(t *testing.T) {
	r := Report{Name: " Week ", Period: ReportWeekly, Time: "08:00", Recipients: []string{"a@example.com", " a@example.com "}, Language: "de"}
	if err := r.normalize(); err != nil || r.Name != "Week" || len(r.Recipients) != 1 || r.Language != "en" || r.Sections[0] != "queues" || r.Weekday != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	for _, bad := range []Report{
		{Name: "x", Period: "yearly", Time: "08:00", Recipients: []string{"a@example.com"}},
		{Name: "x", Period: ReportDaily, Time: "25:00", Recipients: []string{"a@example.com"}},
		{Name: "x", Period: ReportDaily, Time: "08:00"},
		{Name: "x", Period: ReportDaily, Time: "08:00", Recipients: []string{"a@example.com"}, Sections: []string{"bad"}},
	} {
		if err := bad.normalize(); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
