package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/qstats"
)

func TestWallboardScreensAndSLA(t *testing.T) {
	c, s := adminClient(t)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true}, 200)
	qid := q["id"]

	// Thresholds are stored in the queue options and survive a normal save of the queue.
	mustDo(t, c, "PUT", "/api/wallboard/sla", []map[string]any{{"queueId": qid, "sla": map[string]any{"target": 30, "norm": 80, "maxWaiting": 3}}}, 200)
	mustDo(t, c, "PUT", "/api/wallboard/sla", []map[string]any{{"queueId": qid, "sla": map[string]any{"norm": 101}}}, 400)
	got := mustDoList(t, c, "/api/pbx/queues")[0]
	sla := got["options"].(map[string]any)["sla"].(map[string]any)
	if sla["target"].(float64) != 30 || sla["norm"].(float64) != 80 || sla["maxWaiting"].(float64) != 3 {
		t.Fatalf("sla: %v", sla)
	}

	// Alert settings: mail needs recipients, addresses are checked.
	mustDo(t, c, "PUT", "/api/wallboard/alerts", map[string]any{"email": true}, 400)
	mustDo(t, c, "PUT", "/api/wallboard/alerts", map[string]any{"email": true, "recipients": []string{"not an address"}}, 400)
	a := mustDo(t, c, "PUT", "/api/wallboard/alerts", map[string]any{"screens": true, "email": true, "recipients": []string{" lead@example.com", "lead@example.com"}}, 200)
	if a["quiet"].(float64) != 15 || len(a["recipients"].([]any)) != 1 {
		t.Fatalf("alerts: %v", a)
	}

	// A tv link works without signing in, only with the right key.
	sc := mustDo(t, c, "POST", "/api/wallboard/screens", map[string]any{"name": "Support room", "queues": []any{qid}, "layouts": []string{"overview", "agents"}, "rotate": 30, "hideNames": true}, 200)
	key := sc["key"].(string)
	jar, _ := cookiejar.New(nil)
	anon := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	if code, _ := anon.do("GET", "/api/reception/wallboard", nil); code != 401 {
		t.Fatalf("wallboard without session: %d", code)
	}
	code, d := anon.do("GET", "/api/wall/"+key, nil)
	if code != 200 || d["screen"].(map[string]any)["name"] != "Support room" || len(d["queues"].([]any)) != 1 {
		t.Fatalf("tv: %d %v", code, d)
	}
	if code, _ := anon.do("GET", "/api/wall/"+strings.Repeat("x", 23), nil); code != 404 {
		t.Fatalf("wrong key: %d", code)
	}
	// A new key replaces the old one; revoking stops the link.
	nk := mustDo(t, c, "POST", fmt.Sprintf("/api/wallboard/screens/%v/key", sc["id"]), map[string]any{}, 200)
	if code, _ := anon.do("GET", "/api/wall/"+key, nil); code != 404 {
		t.Fatalf("old key still works: %d", code)
	}
	if code, _ := anon.do("GET", "/api/wall/"+nk["key"].(string), nil); code != 200 {
		t.Fatalf("new key: %d", code)
	}
	set := mustDo(t, c, "GET", "/api/wallboard/settings", nil, 200)
	if len(set["screens"].([]any)) != 1 || len(set["queues"].([]any)) != 1 {
		t.Fatalf("settings: %v", set)
	}
	mustDo(t, c, "DELETE", fmt.Sprintf("/api/wallboard/screens/%v", sc["id"]), map[string]any{}, 200)
	if code, _ := anon.do("GET", "/api/wall/"+nk["key"].(string), nil); code != 404 {
		t.Fatalf("revoked key: %d", code)
	}
	_ = s
}

func TestWallboardTodayFigures(t *testing.T) {
	c, s := adminClient(t)
	q := mustDo(t, c, "POST", "/api/pbx/queues", map[string]any{"number": "701", "name": "Support", "enabled": true}, 200)
	name := fmt.Sprintf("kiestoon-q-%v", q["id"])
	s.queueStatEvent(asterisk.Message{{Key: "Event", Value: "QueueCallerJoin"}, {Key: "Queue", Value: name}, {Key: "Uniqueid", Value: "1.1"}})
	s.queueStatEvent(asterisk.Message{{Key: "Event", Value: "AgentConnect"}, {Key: "Queue", Value: name}, {Key: "Uniqueid", Value: "1.1"},
		{Key: "HoldTime", Value: "8"}, {Key: "Interface", Value: "Local/201@kiestoon-ring/n"}, {Key: "CallerIDNum", Value: "0612345678"}})
	d := mustDo(t, c, "GET", "/api/reception/wallboard", nil, 200)
	wq := d["queues"].([]any)[0].(map[string]any)
	today := wq["today"].(map[string]any)
	if today["answered"].(float64) != 1 || today["inTarget"].(float64) != 1 || today["serviceLevel"].(float64) != 100 {
		t.Fatalf("today: %v", today)
	}
	s.wall.mu.Lock()
	if s.wall.calls["201"].number != "0612345678" {
		t.Fatalf("agent call not tracked: %+v", s.wall.calls)
	}
	s.wall.mu.Unlock()
	s.queueStatEvent(asterisk.Message{{Key: "Event", Value: "AgentComplete"}, {Key: "Queue", Value: name}, {Key: "Uniqueid", Value: "1.1"},
		{Key: "TalkTime", Value: "60"}, {Key: "Interface", Value: "Local/201@kiestoon-ring/n"}})
	calls, _ := (&qstats.Store{DB: s.DB}).Calls(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if len(calls) != 1 || calls[0].Talk != 60 {
		t.Fatalf("calls: %+v", calls)
	}
}

func TestSLABreachesAndMail(t *testing.T) {
	q := wallQueue{ID: 1, Name: "Support", Live: true, Waiting: 4, LongestWait: 200}
	q.SLA.Target, q.SLA.MaxWaiting, q.SLA.MaxWait, q.SLA.Norm, q.SLA.MaxAbandoned = 20, 3, 120, 80, 10
	q.Today = qstats.Totals{Offered: 12, Answered: 8, Abandoned: 4, InTarget: 5}
	q.Today.Finish()
	kinds := []string{}
	for _, a := range slaBreaches(q) {
		kinds = append(kinds, a.Kind)
	}
	if strings.Join(kinds, ",") != "waiting,wait,level,abandoned" {
		t.Fatalf("breaches: %v (%+v)", kinds, q.Today)
	}
	q.Live = false
	if len(slaBreaches(q)) != 0 {
		t.Fatal("offline queue alerted")
	}
	subj, body := slaMail("nl", slaAlert{Queue: "Support", Kind: "wait", Value: 192, Limit: 120}, false, time.Date(2026, 11, 16, 10, 39, 0, 0, time.UTC))
	if subj != "SLA-melding: Support, wachttijd" || !strings.Contains(body, "3:12") || !strings.Contains(body, "2:00") || !strings.Contains(body, "10:39") {
		t.Fatalf("%s\n%s", subj, body)
	}
	if subj, _ := slaMail("en", slaAlert{Queue: "Support", Kind: "level"}, true, time.Now()); subj != "SLA back to normal: Support" {
		t.Fatal(subj)
	}
}
