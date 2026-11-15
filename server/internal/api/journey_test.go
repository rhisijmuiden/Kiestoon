package api

import (
	"context"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
)

func TestCallJourney(t *testing.T) {
	c, s := adminClient(t)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	ctx := context.Background()
	start := time.Now().Add(-time.Hour).UTC()
	ts := func(sec float64) string {
		return time.Unix(start.Unix(), 0).Add(time.Duration(sec * float64(time.Second))).Format("2006-01-02 15:04:05.000000")
	}
	trunk := "PJSIP/trunk-1-00000001"
	msgs := []asterisk.Message{
		{{Key: "Event", Value: "CEL"}, {Key: "EventName", Value: "CHAN_START"}, {Key: "EventTime", Value: ts(0)}, {Key: "Channel", Value: trunk},
			{Key: "UniqueID", Value: "100.1"}, {Key: "LinkedID", Value: "100.1"}, {Key: "CallerIDnum", Value: "0612345678"}, {Key: "Exten", Value: "0201234567"}},
		{{Key: "Event", Value: "CEL"}, {Key: "EventName", Value: "kt_ext"}, {Key: "EventTime", Value: ts(0.1)}, {Key: "Channel", Value: trunk},
			{Key: "UniqueID", Value: "100.1"}, {Key: "LinkedID", Value: "100.1"}, {Key: "Extra", Value: `{"extra":"num=201"}`}},
		{{Key: "Event", Value: "DialBegin"}, {Key: "Channel", Value: trunk}, {Key: "Uniqueid", Value: "100.1"}, {Key: "Linkedid", Value: "100.1"},
			{Key: "DestChannel", Value: "PJSIP/201-00000002"}, {Key: "DestUniqueid", Value: "100.2"}},
		{{Key: "Event", Value: "DialEnd"}, {Key: "Channel", Value: trunk}, {Key: "Uniqueid", Value: "100.1"}, {Key: "Linkedid", Value: "100.1"},
			{Key: "DestChannel", Value: "PJSIP/201-00000002"}, {Key: "DestUniqueid", Value: "100.2"}, {Key: "DialStatus", Value: "ANSWER"}},
		{{Key: "Event", Value: "CEL"}, {Key: "EventName", Value: "HANGUP"}, {Key: "EventTime", Value: ts(60)}, {Key: "Channel", Value: trunk},
			{Key: "UniqueID", Value: "100.1"}, {Key: "LinkedID", Value: "100.1"}, {Key: "Extra", Value: `{"hangupcause":16,"hangupsource":"PJSIP/201-00000002"}`}},
		{{Key: "Event", Value: "CEL"}, {Key: "EventName", Value: "APP_START"}, {Key: "UniqueID", Value: "100.1"}, {Key: "LinkedID", Value: "100.1"}},
	}
	for _, m := range msgs {
		s.onAMIEvent(m)
	}
	if err := s.CDR.Insert(ctx, cdr.Record{UniqueID: "100.1", LinkedID: "100.1", Start: start.Format(time.RFC3339), Src: "0612345678", Dst: "201",
		SrcChannel: trunk, DstChannel: "PJSIP/201-00000002", Disposition: "ANSWERED", Direction: "inbound"}); err != nil {
		t.Fatal(err)
	}
	// The recorder writes in batches.
	deadline := time.Now().Add(3 * time.Second)
	for {
		evs, _ := s.journeys.store.Events(ctx, "100.1")
		if len(evs) == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored %d events, want 5 (APP_START is not kept)", len(evs))
		}
		time.Sleep(100 * time.Millisecond)
	}
	j := mustDo(t, c, "GET", "/api/cdr/journey/100.1", nil, 200)
	if j["result"] != "answered" || j["direction"] != "inbound" {
		t.Fatalf("journey: %v", j)
	}
	steps := j["steps"].([]any)
	kinds := []string{}
	for _, st := range steps {
		kinds = append(kinds, st.(map[string]any)["kind"].(string))
	}
	if len(kinds) != 4 || kinds[1] != "extension" || kinds[2] != "talk" || kinds[3] != "hangup" {
		t.Fatalf("steps %v", kinds)
	}
	if n := steps[1].(map[string]any)["name"]; n != "Anna" {
		t.Errorf("extension name %v", n)
	}
	mustDo(t, c, "GET", "/api/cdr/journey/nope", nil, 404)

	// A call with only a call record gets the short version.
	_ = s.CDR.Insert(ctx, cdr.Record{UniqueID: "200.1", LinkedID: "200.1", Start: start.Format(time.RFC3339), Src: "201", Dst: "0612345678",
		SrcChannel: "PJSIP/201-00000009", DstChannel: "PJSIP/trunk-1-0000000a", Disposition: "NO ANSWER", Direction: "outbound"})
	p := mustDo(t, c, "GET", "/api/cdr/journey/200.1", nil, 200)
	if p["partial"] != true || p["result"] != "missed" {
		t.Errorf("partial journey: %v", p)
	}

	// Switched off: nothing is recorded and the old events are removed.
	// Only the new field: the others keep their value.
	mustDo(t, c, "PUT", "/api/pbx/features", map[string]any{"journeyDays": 0}, 200)
	f := mustDo(t, c, "GET", "/api/pbx/features", nil, 200)["settings"].(map[string]any)
	if f["journeyDays"] != float64(0) || f["recordingDays"] != float64(90) {
		t.Fatalf("features: %v", f)
	}
	if !s.journeys.off.Load() {
		t.Fatal("journeys still on")
	}
	s.purgeJourneys(ctx)
	if evs, _ := s.journeys.store.Events(ctx, "100.1"); len(evs) != 0 {
		t.Errorf("%d events left after purge", len(evs))
	}
}
