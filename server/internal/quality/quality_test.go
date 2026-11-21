package quality

import (
	"fmt"
	"testing"
	"time"
)

func TestMOS(t *testing.T) {
	if m := MOS(0, 0, 0, "alaw"); m < 4.35 || m > 4.45 {
		t.Fatalf("perfect alaw = %v", m)
	}
	if MOS(0, 0, 0, "g729") >= MOS(0, 0, 0, "alaw") {
		t.Fatal("g729 should score lower than alaw")
	}
	if m := MOS(38, 5.6, 14, "g722"); m >= 3.85 || m < 3 {
		t.Fatalf("lossy call = %v", m)
	}
	if MOS(1000, 60, 2000, "ulaw") != 1 {
		t.Fatal("hopeless call must be 1")
	}
	if MOS(20, 0, 600, "alaw") >= MOS(20, 0, 50, "alaw") {
		t.Fatal("a long round trip must lower the MOS")
	}
}

func TestClassify(t *testing.T) {
	for ep, want := range map[string]string{"trunk-3": "trunk/3/", "201": "extension/0/201", "201-web": "extension/0/201", "201-app": "extension/0/201", "site-2": "other/0/", "trunk-x": "other/0/"} {
		k, id, ext := Classify(ep)
		if got := fmt.Sprintf("%s/%d/%s", k, id, ext); got != want {
			t.Errorf("%s: got %s, want %s", ep, got, want)
		}
	}
	if EndpointOf("PJSIP/trunk-2-0000001a") != "trunk-2" || EndpointOf("Local/201@x-0001;1") != "" {
		t.Fatal("EndpointOf")
	}
}

func ev(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestTracker(t *testing.T) {
	tr := NewTracker()
	now := time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)
	base := []string{"Channel", "PJSIP/204-00000012", "Uniqueid", "1731.12", "Linkedid", "1731.10"}
	_, isNew := tr.Event(ev(append(base, "Event", "RTCPSent", "ReportCount", "1", "Report0IAJitter", "24", "Report0FractionLost", "0", "To", "192.168.20.41:12000")...), now)
	if !isNew {
		t.Fatal("first report must be new")
	}
	tr.SetCodec("1731.12", "G722")
	for i := 1; i <= 20; i++ {
		at := now.Add(time.Duration(i) * 5 * time.Second)
		frac := "0"
		if i > 10 {
			frac = "26" // ~10% lost towards the phone
		}
		tr.Event(ev(append(base, "Event", "RTCPReceived", "ReportCount", "1", "Report0IAJitter", "240", "Report0FractionLost", frac, "RTT", "0.0140", "From", "192.168.20.41:12001")...), at)
		tr.Event(ev(append(base, "Event", "RTCPSent", "ReportCount", "1", "Report0IAJitter", "40", "Report0FractionLost", "0")...), at)
	}
	act := tr.Active(now.Add(101 * time.Second))
	if len(act) != 1 || act[0].Current == 0 || act[0].Current >= 3.6 {
		t.Fatalf("active: %+v", act)
	}
	done, _ := tr.Event(ev(append(base, "Event", "Hangup")...), now.Add(2*time.Minute))
	if done == nil {
		t.Fatal("hangup must finish the leg")
	}
	l := *done
	if l.Kind != KindExtension || l.Ext != "204" || l.Codec != "g722" || l.Remote != "192.168.20.41" || l.LinkedID != "1731.10" {
		t.Fatalf("leg: %+v", l)
	}
	if l.RX.Reports != 21 || l.TX.Reports != 20 || l.RX.Loss != 0 || l.TX.Loss < 4 || l.TX.Jitter != 30 || l.RTT != 14 {
		t.Fatalf("figures: rx %+v tx %+v rtt %v", l.RX, l.TX, l.RTT)
	}
	if l.MOS != l.TX.MOS || l.RX.MOS <= l.TX.MOS || l.MOSMin >= l.MOS || len(l.Samples) != 21 {
		t.Fatalf("mos %v rx %v tx %v min %v samples %d", l.MOS, l.RX.MOS, l.TX.MOS, l.MOSMin, len(l.Samples))
	}
	if c := Cause(l.MOS, l.RX.Jitter, l.TX.Jitter, l.RX.Loss, l.TX.Loss, l.RTT, Defaults()); c != CauseLossTo {
		t.Fatalf("cause %s", c)
	}
	if len(tr.Active(now)) != 0 {
		t.Fatal("leg must be gone after hangup")
	}
	// Local channels and reports without a report block are ignored.
	if _, n := tr.Event(ev("Event", "RTCPSent", "Channel", "Local/201@x-1;1", "Uniqueid", "9"), now); n {
		t.Fatal("local channel")
	}
}

func TestStaleAndHalve(t *testing.T) {
	tr := NewTracker()
	now := time.Now()
	g := ev("Event", "RTCPSent", "Channel", "PJSIP/trunk-1-0001", "Uniqueid", "u1", "ReportCount", "1", "Report0IAJitter", "8", "Report0FractionLost", "0")
	for i := 0; i < maxReports+5; i++ {
		tr.Event(g, now)
	}
	if got := tr.Stale(now.Add(time.Minute), 5*time.Minute); len(got) != 0 {
		t.Fatal("not stale yet")
	}
	got := tr.Stale(now.Add(6*time.Minute), 5*time.Minute)
	if len(got) != 1 || got[0].Kind != KindTrunk || got[0].TrunkID != 1 || got[0].RX.Reports > maxReports {
		t.Fatalf("stale: %+v", got)
	}
}

func leg(link, kind string, trunk int64, ext string, mos, txLoss float64, end time.Time) Leg {
	return Leg{LinkedID: link, Kind: kind, TrunkID: trunk, Ext: ext, MOS: mos, End: end,
		RX: Direction{MOS: mos, Reports: 3, Jitter: 5}, TX: Direction{MOS: mos, Reports: 3, Jitter: 5, Loss: txLoss}, RTT: 20}
}

func TestSummarizeAndBreaches(t *testing.T) {
	s := Defaults()
	from := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	legs := []Leg{
		leg("a", KindTrunk, 1, "", 4.3, 0, from.Add(30*time.Minute)),
		leg("a", KindExtension, 0, "204", 3.2, 5, from.Add(30*time.Minute)),
		leg("b", KindTrunk, 2, "", 3.4, 4, from.Add(90*time.Minute)),
		leg("c", KindExtension, 0, "201", 4.4, 0, from.Add(90*time.Minute)),
		leg("d", KindOther, 0, "", 0, 0, from.Add(90*time.Minute)),
	}
	o := Summarize(legs, from, time.Hour, 24, s)
	if o.Total.Calls != 3 || o.Total.Poor != 2 || o.Total.Legs != 4 {
		t.Fatalf("total: %+v", o.Total)
	}
	if len(o.Trunks) != 2 || len(o.Phones) != 2 || o.Phones[0].Key != "204" || o.Phones[0].Cause != CauseLossTo {
		t.Fatalf("groups: %+v %+v", o.Trunks, o.Phones[0])
	}
	if o.Buckets[0] == 0 || o.Buckets[1] == 0 || o.Buckets[2] != 0 {
		t.Fatalf("buckets: %v", o.Buckets[:3])
	}

	now := from.Add(100 * time.Minute)
	recent := []Leg{}
	for i := 0; i < 3; i++ {
		recent = append(recent, leg(fmt.Sprint("t", i), KindTrunk, 2, "", 3.3, 4, now.Add(-time.Duration(i)*time.Minute)))
		recent = append(recent, leg(fmt.Sprint("t", i), KindExtension, 0, "204", 3.3, 4, now.Add(-time.Duration(i)*time.Minute)))
	}
	recent = append(recent, leg("old", KindTrunk, 1, "", 2, 9, now.Add(-time.Hour)))
	b := Breaches(recent, s, now)
	if len(b) != 2 || b[0].Key != "phone/204" || b[1].Key != "trunk/2" || b[1].Calls != 3 {
		t.Fatalf("breaches: %+v", b)
	}
	s.Phone.Count = 4
	if len(Breaches(recent, s, now)) != 1 {
		t.Fatal("phone rule needs 4 poor calls")
	}
}

func TestSettingsValidate(t *testing.T) {
	s := Defaults()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Good = 3.5
	if err := s.Validate(); err == nil {
		t.Fatal("good under poor")
	}
	s = Defaults()
	s.Email = true
	if err := s.Validate(); err == nil {
		t.Fatal("mail without recipients")
	}
	s.Recipients = []string{" it@example.com ", "it@example.com", ""}
	if err := s.Validate(); err != nil || len(s.Recipients) != 1 {
		t.Fatalf("%v %v", err, s.Recipients)
	}
}
