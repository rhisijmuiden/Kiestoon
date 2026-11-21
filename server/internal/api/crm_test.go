package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/crm"
)

// fakeHubSpot answers the HubSpot API calls Kiestoon makes and fails the first call write when
// failFirst is set.
func fakeHubSpot(t *testing.T, failFirst bool) (*[]string, func(string) string) {
	var mu sync.Mutex
	var reqs []string
	bodies := map[string]string{}
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		key := r.Method + " " + r.URL.Path
		reqs = append(reqs, key)
		bodies[key] = string(b)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/account-info/v3/details":
			_, _ = io.WriteString(w, `{"portalId":42,"uiDomain":"app.hubspot.com"}`)
		case r.URL.Path == "/crm/v3/objects/contacts" && r.Method == "GET":
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.URL.Path == "/crm/v3/objects/contacts/search":
			_, _ = io.WriteString(w, `{"results":[{"id":"51","properties":{"firstname":"Jan","lastname":"de Vries","company":"Bakkerij","phone":"+31612345678","hubspot_owner_id":"7"}}]}`)
		case strings.HasSuffix(r.URL.Path, "/search"):
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.URL.Path == "/crm/v3/owners":
			_, _ = io.WriteString(w, `{"results":[{"id":"7","email":"anna@example.com","firstName":"Anna","lastName":"A"}]}`)
		case r.URL.Path == "/crm/v3/objects/calls" && r.Method == "POST":
			calls++
			if failFirst && calls == 1 {
				w.WriteHeader(502)
				_, _ = io.WriteString(w, `{"message":"bad gateway"}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"c1"}`)
		case strings.HasPrefix(r.URL.Path, "/crm/v3/objects/calls/"):
			_, _ = io.WriteString(w, `{"id":"c1"}`)
		case r.URL.Path == "/crm/v3/objects/tasks":
			_, _ = io.WriteString(w, `{"id":"t1"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ts.Close)
	old := crm.HubSpotBase
	crm.HubSpotBase = ts.URL
	t.Cleanup(func() { crm.HubSpotBase = old })
	return &reqs, func(k string) string {
		mu.Lock()
		defer mu.Unlock()
		return bodies[k]
	}
}

func TestCRMSettingsAndLogging(t *testing.T) {
	reqs, body := fakeHubSpot(t, true)
	c, s := adminClient(t)
	ctx := context.Background()
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	if _, err := s.DB.Exec(`INSERT INTO users(username, display_name, password_hash, role, email, extension_id, created_at)
		SELECT 'anna', 'Anna', 'x', 'user', 'anna@example.com', id, '2026-01-01T00:00:00Z' FROM extensions WHERE number = '201'`); err != nil {
		t.Fatal(err)
	}

	got := mustDo(t, c, "GET", "/api/crm", nil, 200)
	if len(got["status"].([]any)) != 4 {
		t.Fatal(got)
	}
	st := crm.Defaults()
	st.Connectors[crm.HubSpot].Enabled = true
	st.Connectors[crm.HubSpot].RecordingLink = true
	mustDo(t, c, "PUT", "/api/crm", st, 200)
	// A wrong token is refused before it is stored.
	good := crm.HubSpotBase
	crm.HubSpotBase = "http://127.0.0.1:1"
	if code, out := c.do("PUT", "/api/crm/hubspot/secret", map[string]string{"token": "pat-bad"}); code != 422 || out["field"] != "token" {
		t.Fatal(code, out)
	}
	crm.HubSpotBase = good
	out := mustDo(t, c, "PUT", "/api/crm/hubspot/secret", map[string]string{"token": "pat-eu1-good"}, 200)
	if strings.Contains(mustJSON(out), "pat-eu1-good") {
		t.Fatal("the token went back to the browser")
	}
	hs := out["status"].([]any)[0].(map[string]any)
	if hs["connected"] != true || hs["account"] != "Portal 42" {
		t.Fatal(hs)
	}
	if !s.crm.Active() {
		t.Fatal("crm not active")
	}
	// The dialplan asks for the CRM name on calls from trunks.
	if v, _ := getSetting(s, crm.DeskNameKey); v != "1" {
		t.Fatal("desk name setting", v)
	}

	// An answered inbound call: logged under Anna's HubSpot owner, after one failed attempt.
	sum := crm.Summary{LinkedID: "1700.1", Direction: "inbound", Number: "0612345678", Line: "0201234567", Ext: "201", Answered: true,
		Start: time.Now().Add(-time.Minute), TalkSec: 402, UniqueIDs: []string{"1700.1"}}
	id, err := s.crmRecord(ctx, sum)
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	s.crmProcess(ctx, id)
	row, _ := s.crmCallRow(ctx, id)
	if row.State != "retry" || row.Attempts != 1 {
		t.Fatalf("after a failure: %+v", row)
	}
	// A note typed in the pop-up waits for the retry.
	if _, err := s.DB.Exec(`UPDATE crm_calls SET note = 'Firmware teruggezet', note_task = 3, note_state = 'pending' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE crm_calls SET next_at = '' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	s.crmProcess(ctx, id)
	row, _ = s.crmCallRow(ctx, id)
	if row.State != "logged" || row.ActivityID != "c1" || row.NoteState != "done" || !strings.Contains(row.Body, "Firmware teruggezet") {
		t.Fatalf("logged: %+v", row)
	}
	call := body("POST /crm/v3/objects/calls")
	if !strings.Contains(call, `"hubspot_owner_id":"7"`) || !strings.Contains(call, "answered by Anna (201)") || !strings.Contains(call, `"hs_call_duration":"402000"`) {
		t.Fatal(call)
	}
	if !strings.Contains(body("POST /crm/v3/objects/tasks"), "Follow up: Jan de Vries") {
		t.Fatal("follow-up task:", body("POST /crm/v3/objects/tasks"))
	}
	// The call history shows where it went.
	info := mustDo(t, c, "GET", "/api/crm/calls?linked=1700.1,nope", nil, 200)
	if x := info["1700.1"].(map[string]any); x["state"] != "logged" || x["contact"] != "Jan de Vries" {
		t.Fatal(info)
	}
	// The same call is never logged twice.
	if id2, _ := s.crmRecord(ctx, sum); id2 != 0 {
		t.Fatal("logged twice")
	}

	// A missed call: a call-back task.
	missed := sum
	missed.LinkedID, missed.Answered, missed.Ext, missed.TalkSec = "1700.2", false, "", 0
	id, _ = s.crmRecord(ctx, missed)
	s.crmProcess(ctx, id)
	row, _ = s.crmCallRow(ctx, id)
	if row.State != "logged" || row.TaskID != "t1" {
		t.Fatalf("missed: %+v", row)
	}
	log := mustDo(t, c, "GET", "/api/crm/log", nil, 200)
	if len(log["entries"].([]any)) < 4 {
		t.Fatal(log)
	}
	_ = reqs
}

func TestCRMPopupEvents(t *testing.T) {
	_, _ = fakeHubSpot(t, false)
	c, s := adminClient(t)
	st := crm.Defaults()
	st.Connectors[crm.HubSpot].Enabled = true
	mustDo(t, c, "PUT", "/api/crm", st, 200)
	mustDo(t, c, "PUT", "/api/crm/hubspot/secret", map[string]string{"token": "pat-1"}, 200)

	sub := &crmSub{ext: "201", ch: make(chan crmEvent, 8)}
	sw := &crmSub{all: true, ch: make(chan crmEvent, 8)}
	other := &crmSub{ext: "202", ch: make(chan crmEvent, 8)}
	s.crmLive.mu.Lock()
	s.crmLive.subs[sub], s.crmLive.subs[sw], s.crmLive.subs[other] = true, true, true
	s.crmLive.mu.Unlock()
	ev := func(kv ...string) asterisk.Message {
		var m asterisk.Message
		for i := 0; i < len(kv); i += 2 {
			m = append(m, asterisk.KV{Key: kv[i], Value: kv[i+1]})
		}
		return m
	}
	s.crmAMI(ev("Event", "Newchannel", "Channel", "PJSIP/trunk-1-0001", "Uniqueid", "99.1", "Linkedid", "99.1", "CallerIDNum", "0612345678"))
	s.crmAMI(ev("Event", "DialBegin", "Channel", "PJSIP/trunk-1-0001", "DestChannel", "PJSIP/201-0002", "Linkedid", "99.1"))
	got := wait(t, sub.ch)
	if got.Type != "ring" || !got.Result.Found() || got.Result.First().Name != "Jan de Vries" {
		t.Fatalf("%+v", got)
	}
	if wait(t, sw.ch).Type != "ring" {
		t.Fatal("switchboard got no pop-up")
	}
	select {
	case e := <-other.ch:
		t.Fatalf("202 did not ring: %+v", e)
	default:
	}
	s.crmAMI(ev("Event", "DialEnd", "Channel", "PJSIP/trunk-1-0001", "DestChannel", "PJSIP/201-0002", "Linkedid", "99.1", "DialStatus", "ANSWER"))
	if e := wait(t, sub.ch); e.Type != "answered" || e.Ext != "201" {
		t.Fatalf("%+v", e)
	}
	s.crmAMI(ev("Event", "Hangup", "Channel", "PJSIP/trunk-1-0001", "Uniqueid", "99.1", "Linkedid", "99.1"))
	if e := wait(t, sub.ch); e.Type != "ended" || !e.Loggable {
		t.Fatalf("%+v", e)
	}
	// The desk phone name.
	if name := s.crmDeskName(context.Background(), "+31612345678"); name != "Jan de Vries - Bakkerij" {
		t.Fatal(name)
	}
	// CDRs of an internal call are not logged.
	s.crmCDR(cdr.Record{LinkedID: "5.1", SrcChannel: "PJSIP/201-1", DstChannel: "PJSIP/202-2"})
}

func wait(t *testing.T, ch chan crmEvent) crmEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("no event")
	}
	return crmEvent{}
}

func getSetting(s *Server, key string) (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
