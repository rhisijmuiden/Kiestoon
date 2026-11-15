package qstats

import (
	"context"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func ev(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestEventsAndSummary(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/k.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Store{DB: db}
	loc := time.UTC
	at := time.Date(2026, 11, 16, 10, 0, 0, 0, loc)
	must := func(get func(string) string, t2 time.Time) {
		if err := s.Event(ctx, get, t2); err != nil {
			t.Fatal(err)
		}
	}
	q := "kiestoon-q-3"
	// 1: answered after 12 s, talked 100 s. The leave comes before the connect.
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "1.1", "CallerIDNum", "0612"), at)
	must(ev("Event", "QueueCallerLeave", "Queue", q, "Uniqueid", "1.1"), at.Add(12*time.Second))
	must(ev("Event", "AgentConnect", "Queue", q, "Uniqueid", "1.1", "HoldTime", "12", "Interface", "Local/201@kiestoon-ring/n", "MemberName", "Anna"), at.Add(12*time.Second))
	must(ev("Event", "AgentComplete", "Queue", q, "Uniqueid", "1.1", "TalkTime", "100"), at.Add(112*time.Second))
	// 2: answered after 40 s (over the 20 s target).
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "2.1"), at.Add(time.Hour))
	must(ev("Event", "AgentConnect", "Queue", q, "Uniqueid", "2.1", "HoldTime", "40", "Interface", "Local/201@kiestoon-ring/n"), at.Add(time.Hour))
	// 3: hung up after 30 s; 4: hung up after 2 s (not counted); 5: left to overflow after 60 s.
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "3.1"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerAbandon", "Queue", q, "Uniqueid", "3.1", "HoldTime", "30"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerLeave", "Queue", q, "Uniqueid", "3.1"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "4.1"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerAbandon", "Queue", q, "Uniqueid", "4.1", "HoldTime", "2"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "5.1"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerLeave", "Queue", q, "Uniqueid", "5.1"), at.Add(time.Hour+time.Minute))
	// 6: a callback placeholder answered by an agent; 7: another queue system's queue is ignored.
	must(ev("Event", "QueueCallerJoin", "Queue", q, "Uniqueid", "6.1", "Channel", "Local/9@kiestoon-qcbwait-3-0001;2"), at.Add(time.Hour))
	must(ev("Event", "AgentConnect", "Queue", q, "Uniqueid", "6.1", "HoldTime", "300", "Interface", "Local/202@kiestoon-ring/n"), at.Add(time.Hour))
	must(ev("Event", "QueueCallerJoin", "Queue", "other", "Uniqueid", "7.1"), at)

	calls, err := s.Calls(ctx, at.Add(-time.Hour), at.Add(3*time.Hour))
	if err != nil || len(calls) != 6 {
		t.Fatalf("calls: %v %v", calls, err)
	}
	sum := Summarize(calls, func(int64) int { return 20 }, loc)
	tt := sum.Queues[3]
	if tt.Offered != 5 || tt.Answered != 2 || tt.Abandoned != 2 || tt.ShortAbandoned != 1 || tt.Left != 1 || tt.Callbacks != 1 || tt.InTarget != 1 {
		t.Fatalf("totals: %+v", tt)
	}
	// Service level: 1 in target of 2 answered + 1 counted abandon.
	if int(tt.ServiceLevel) != 33 || tt.AvgWait != 26 || tt.AvgTalk != 100 || tt.LongestWait != 60 {
		t.Fatalf("figures: %+v", tt)
	}
	// Abandoned: 1 of 4 (5 offered minus the short abandon).
	if tt.AbandonedPct != 25 {
		t.Fatalf("abandoned pct: %v", tt.AbandonedPct)
	}
	h := sum.Hours[3]
	if h[10].Answered != 1 || h[11].Answered != 1 || h[11].Abandoned != 1 {
		t.Fatalf("hours: %+v", h)
	}
	if a := sum.Agents["201"]; a == nil || a.Calls != 2 || a.Name != "Anna" {
		t.Fatalf("agents: %+v", sum.Agents)
	}
	if n, _ := s.Purge(ctx, 1, at.AddDate(0, 0, 3)); n != 6 {
		t.Fatalf("purge: %d", n)
	}
}

func TestEmptyTotals(t *testing.T) {
	var tt Totals
	tt.Finish()
	if tt.ServiceLevel != -1 || tt.AbandonedPct != -1 {
		t.Fatalf("%+v", tt)
	}
}
