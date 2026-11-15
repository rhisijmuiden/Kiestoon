package api

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
)

func TestScheduledReports(t *testing.T) {
	c, s := adminClient(t)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true}, 200)
	name := fmt.Sprintf("kiestoon-q-%v", q["id"])
	ev := func(kv ...string) {
		m := asterisk.Message{}
		for i := 0; i+1 < len(kv); i += 2 {
			m = append(m, asterisk.KV{Key: kv[i], Value: kv[i+1]})
		}
		s.queueStatEvent(m)
	}
	ev("Event", "QueueCallerJoin", "Queue", name, "Uniqueid", "1.1")
	ev("Event", "AgentConnect", "Queue", name, "Uniqueid", "1.1", "HoldTime", "8", "Interface", "Local/201@kiestoon-ring/n", "MemberName", "Anna <b>")
	ev("Event", "AgentComplete", "Queue", name, "Uniqueid", "1.1", "TalkTime", "90", "Interface", "Local/201@kiestoon-ring/n")

	mustDo(t, c, "POST", "/api/wallboard/reports", map[string]any{"name": "Daily", "period": "daily", "time": "08:00"}, 400)
	r := mustDo(t, c, "POST", "/api/wallboard/reports", map[string]any{"name": "Daily", "period": "daily", "time": "23:59", "enabled": true,
		"sections": []string{"queues", "agents", "hours", "trend"}, "recipients": []string{"lead@example.com"}, "language": "nl"}, 200)
	if r["lastSent"] == "" {
		t.Fatal("a new report should not go out for a moment already passed")
	}
	if l := mustDoList(t, c, "/api/wallboard/reports"); len(l) != 1 {
		t.Fatalf("list %v", l)
	}
	p := mustDo(t, c, "GET", fmt.Sprintf("/api/wallboard/reports/%v/preview", r["id"]), nil, 200)
	html := p["html"].(string)
	for _, want := range []string{"Dagoverzicht", "Per wachtrij", "Support", "Anna &lt;b&gt;", "100%", "Powered by RHIS"} {
		if !strings.Contains(html, want) {
			t.Fatalf("preview misses %q:\n%s", want, html)
		}
	}
	if !strings.HasPrefix(p["subject"].(string), "Daily · ") {
		t.Fatalf("subject %v", p["subject"])
	}
	// The admin has no e-mail address: "send to me" says so.
	mustDo(t, c, "POST", fmt.Sprintf("/api/wallboard/reports/%v/send", r["id"]), map[string]any{}, 409)

	// The scheduler marks a due report as sent, also when the mail server is off.
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `UPDATE wallboard_reports SET time = '00:00', last_sent = ''`); err != nil {
		t.Fatal(err)
	}
	s.sendDueReports(ctx, time.Now())
	rep, _ := s.PBX.Report(ctx, int64(r["id"].(float64)))
	if rep.LastSent == "" {
		t.Fatal("not marked as sent")
	}
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/wallboard/reports/%v", r["id"]), map[string]any{}, 200)
}
