package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/quality"
)

// rtcpCall feeds the reports of a call with a trunk leg and a phone leg, then hangs it up.
func rtcpCall(t *testing.T, s *Server, linked string, phoneLoss string) {
	t.Helper()
	legs := []struct{ ch, uid string }{{"PJSIP/trunk-1-0000000a", linked + "1"}, {"PJSIP/204-0000000b", linked + "2"}}
	for i := 0; i < 6; i++ {
		for _, l := range legs {
			loss := "0"
			if l.ch == "PJSIP/204-0000000b" {
				loss = phoneLoss
			}
			base := asterisk.Message{{Key: "Channel", Value: l.ch}, {Key: "Uniqueid", Value: l.uid}, {Key: "Linkedid", Value: linked}, {Key: "ReportCount", Value: "1"}}
			s.qualityEvent(append(base, asterisk.KV{Key: "Event", Value: "RTCPSent"}, asterisk.KV{Key: "Report0IAJitter", Value: "16"}, asterisk.KV{Key: "Report0FractionLost", Value: "0"}))
			s.qualityEvent(append(base, asterisk.KV{Key: "Event", Value: "RTCPReceived"}, asterisk.KV{Key: "Report0IAJitter", Value: "40"},
				asterisk.KV{Key: "Report0FractionLost", Value: loss}, asterisk.KV{Key: "RTT", Value: "0.020"}, asterisk.KV{Key: "From", Value: "10.0.0.5:4000"}))
		}
	}
	for _, l := range legs {
		done, _ := s.quality.init().Event(asterisk.Message{{Key: "Event", Value: "Hangup"}, {Key: "Uniqueid", Value: l.uid}}.Get, time.Now())
		if done == nil {
			t.Fatal("no leg")
		}
		s.qualityFinished(*done)
	}
}

func TestCallQuality(t *testing.T) {
	c, s := adminClient(t)

	// Settings: defaults, validation, recipients needed for mail.
	set := mustDo(t, c, "GET", "/api/quality/settings", nil, 200)
	if set["poor"].(float64) != 3.6 || set["keepDays"].(float64) != 90 {
		t.Fatalf("defaults: %v", set)
	}
	set["good"] = 3.0
	mustDo(t, c, "PUT", "/api/quality/settings", set, 400)
	set["good"], set["email"] = 4.0, true
	mustDo(t, c, "PUT", "/api/quality/settings", set, 400)
	set["email"] = false
	set["phone"] = map[string]any{"enabled": true, "minutes": 60, "count": 2}
	mustDo(t, c, "PUT", "/api/quality/settings", set, 200)

	// Two calls: the phone leg loses ~10% of the audio towards the phone.
	rtcpCall(t, s, "100.1", "26")
	rtcpCall(t, s, "100.2", "26")
	rtcpCall(t, s, "100.3", "0")

	o := mustDo(t, c, "GET", "/api/quality?range=24h", nil, 200)
	ov := o["overview"].(map[string]any)
	total := ov["total"].(map[string]any)
	if total["calls"].(float64) != 3 || total["poor"].(float64) != 2 {
		t.Fatalf("total: %v", total)
	}
	phones := ov["phones"].([]any)
	if len(phones) != 1 || phones[0].(map[string]any)["key"] != "204" || phones[0].(map[string]any)["cause"] != quality.CauseLossTo {
		t.Fatalf("phones: %v", phones)
	}
	if len(ov["trunks"].([]any)) != 1 || len(ov["buckets"].([]any)) != 24 {
		t.Fatalf("trunks: %v", ov["trunks"])
	}

	// The call history shows the MOS of each call and the legs in the journey.
	q, err := (&quality.Store{DB: s.DB}).CallMOS(context.Background(), []string{"100.1", "100.3", "nope"})
	if err != nil || len(q) != 2 || q["100.1"] >= 3.6 || q["100.3"] < 4 {
		t.Fatalf("call mos: %v %v", q, err)
	}
	v := s.qualityForCall(context.Background(), "100.1")
	if v == nil || len(v.Legs) != 2 || v.Rating != "poor" || v.Cause != quality.CauseLossTo || v.Where != "204" {
		t.Fatalf("call view: %+v", v)
	}
	if len(v.Legs[0].Samples) == 0 {
		t.Fatal("timeline missing")
	}

	// The phone rule (2 poor calls in 60 minutes) raises an alert; it ends when the calls age out.
	now := time.Now()
	s.checkQuality(context.Background(), now)
	alerts, _ := (&quality.Store{DB: s.DB}).ActiveAlerts(context.Background())
	if len(alerts) != 1 || alerts[0].Key != "phone/204" || alerts[0].Poor != 2 {
		t.Fatalf("alerts: %+v", alerts)
	}
	s.checkQuality(context.Background(), now.Add(2*time.Hour))
	alerts, _ = (&quality.Store{DB: s.DB}).ActiveAlerts(context.Background())
	if len(alerts) != 0 || (&quality.Store{DB: s.DB}).LastEnded(context.Background()) == "" {
		t.Fatalf("alert did not end: %+v", alerts)
	}

	// Test mail needs a valid address.
	mustDo(t, c, "POST", "/api/quality/test-mail", map[string]any{"recipients": []string{"nope"}}, 400)
}

func TestQualityMails(t *testing.T) {
	set := quality.Defaults()
	a := quality.Alert{Kind: "trunk", Ref: "1", Name: "Voys", Calls: 6, MOS: 3.42, Jitter: 12, Loss: 4.8, RTT: 44, Cause: quality.CauseLossFrom}
	at := time.Date(2026, 11, 21, 14, 37, 0, 0, time.UTC)
	sub, body := qualityAlertMail("nl", a, set, false, "https://pbx.example.com/quality", at)
	if sub != "Melding gesprekskwaliteit: trunk Voys" || !contains(body, "MOS 3,42") || !contains(body, "pakketverlies op de audio van de provider") || !contains(body, "https://pbx.example.com/quality") {
		t.Fatalf("nl: %s\n%s", sub, body)
	}
	sub, body = qualityAlertMail("en", quality.Alert{Kind: "phone", Ref: "204", Name: "204 Sanne", Calls: 5, Poor: 3, MOS: 3.3, Cause: quality.CauseLossTo}, set, false, "", at)
	if sub != "Call quality alert: phone 204 Sanne" || !contains(body, "3 of 5 calls") || !contains(body, "towards the phone") {
		t.Fatalf("en: %s\n%s", sub, body)
	}
	sub, _ = qualityAlertMail("en", a, set, true, "", at)
	if sub != "Call quality back to normal: trunk Voys" {
		t.Fatal(sub)
	}
	l := quality.Leg{LinkedID: "9.1", Kind: quality.KindExtension, Ext: "204", MOS: 3.1, TX: quality.Direction{Loss: 6, Reports: 3}, End: at}
	sub, body = qualityCallMail("en", l, qualityNames{exts: map[string]string{"204": "Sanne"}}, set, "https://x/quality", at)
	if sub != "Poor call: 204 Sanne" || !contains(body, "https://x/history?call=9.1") {
		t.Fatalf("call: %s\n%s", sub, body)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
