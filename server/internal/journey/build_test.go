package journey

import (
	"encoding/json"
	"testing"
	"time"
)

func amiGetter(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestFromAMIMarker(t *testing.T) {
	ev, ok := FromAMI(amiGetter(map[string]string{"Event": "CEL", "EventName": "kt_ivrkey", "EventTime": "1763226000.250000",
		"UniqueID": "1.1", "LinkedID": "1.1", "Channel": "PJSIP/trunk-1-00000001", "Extra": `{"extra":"id=3&key=1"}`}), time.UTC)
	if !ok || ev.Kind != "kt_ivrkey" || ev.Data["id"] != "3" || ev.Data["key"] != "1" {
		t.Fatalf("marker: %+v %v", ev, ok)
	}
	if ev.At.UnixMilli() != 1763226000250 {
		t.Fatalf("time: %v", ev.At)
	}
	if _, ok := FromAMI(amiGetter(map[string]string{"Event": "CEL", "EventName": "APP_START", "LinkedID": "x"}), time.UTC); ok {
		t.Fatal("APP_START kept")
	}
	ev, ok = FromAMI(amiGetter(map[string]string{"Event": "CEL", "EventName": "HANGUP", "LinkedID": "1.1", "UniqueID": "1.1",
		"Extra": `{"hangupcause":16,"hangupsource":"PJSIP/trunk-1-00000001","dialstatus":""}`}), time.UTC)
	if !ok || ev.Data["hangupcause"] != "16" || ev.Data["hangupsource"] != "PJSIP/trunk-1-00000001" {
		t.Fatalf("hangup: %+v", ev)
	}
}

func TestBuildInboundIVRQueue(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	caller := "PJSIP/trunk-1-00000001"
	evs := []Event{
		{At: at(0), Kind: "CHAN_START", UniqueID: "1.1", Channel: caller, Data: map[string]string{"num": "0612345678", "name": "Jan", "exten": "0201234567", "context": "kiestoon-from-trunk"}},
		{At: at(0.1), Kind: "kt_in", UniqueID: "1.1", Channel: caller, Data: map[string]string{"route": "1", "did": "0201234567"}},
		{At: at(0.1), Kind: "kt_tc", UniqueID: "1.1", Channel: caller, Data: map[string]string{"id": "2", "state": "open"}},
		{At: at(0.2), Kind: "kt_ivr", UniqueID: "1.1", Channel: caller, Data: map[string]string{"id": "3"}},
		{At: at(0.2), Kind: "ANSWER", UniqueID: "1.1", Channel: caller},
		{At: at(8), Kind: "kt_ivrkey", UniqueID: "1.1", Channel: caller, Data: map[string]string{"id": "3", "key": "1"}},
		{At: at(8.1), Kind: "kt_queue", UniqueID: "1.1", Channel: caller, Data: map[string]string{"id": "4"}},
		{At: at(8.2), Kind: "Q_JOIN", UniqueID: "1.1", Channel: caller, Data: map[string]string{"queue": "kiestoon-q-4", "position": "1"}},
		{At: at(8.3), Kind: "Q_CALL", UniqueID: "1.1", Channel: caller, Data: map[string]string{"interface": "Local/201@kiestoon-ring/n", "memberName": "Piet", "destUniqueid": "2.1"}},
		{At: at(8.3), Kind: "DIAL", UniqueID: "2.2", Channel: "Local/201@kiestoon-ring-00000001;2", Data: map[string]string{"dest": "PJSIP/201-00000002", "destId": "2.3"}},
		{At: at(23.3), Kind: "Q_NOANSWER", UniqueID: "1.1", Channel: caller, Data: map[string]string{"memberName": "Piet", "ringTime": "15", "destUniqueid": "2.1"}},
		{At: at(25), Kind: "Q_CALL", UniqueID: "1.1", Channel: caller, Data: map[string]string{"interface": "Local/202@kiestoon-ring/n", "memberName": "Kees", "destUniqueid": "3.1"}},
		{At: at(30), Kind: "Q_CONNECT", UniqueID: "1.1", Channel: caller, Data: map[string]string{"memberName": "Kees", "holdTime": "22", "ringTime": "5", "destUniqueid": "3.1"}},
		{At: at(90), Kind: "HANGUP", UniqueID: "1.1", Channel: caller, Data: map[string]string{"hangupsource": caller, "hangupcause": "16"}},
		{At: at(90.2), Kind: "LINKEDID_END", UniqueID: "1.1", Channel: caller},
	}
	names := func(kind, id string) string {
		return map[string]string{"ivr:3": "Hoofdmenu", "queue:4": "Support", "tc:2": "Kantooruren", "ext:201": "Piet", "ext:202": "Kees", "trunk:1": "KPN", "route:1": "Hoofdnummer"}[kind+":"+id]
	}
	j := Build("1.1", evs, names)
	kinds := []string{}
	for _, s := range j.Steps {
		kinds = append(kinds, s.Kind)
	}
	want := []string{"inbound", "timecondition", "ivr", "queue", "talk", "hangup"}
	if len(kinds) != len(want) {
		b, _ := json.MarshalIndent(j, "", " ")
		t.Fatalf("steps %v, want %v\n%s", kinds, want, b)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("steps %v, want %v", kinds, want)
		}
	}
	ivr, q, talk, hang := j.Steps[2], j.Steps[3], j.Steps[4], j.Steps[5]
	if ivr.Result != "1" || ivr.Name != "Hoofdmenu" || ivr.DurationMs != 7900 {
		t.Errorf("ivr: %+v", ivr)
	}
	if q.Result != "answered" || len(q.Subs) != 2 || q.Subs[0].Result != "noanswer" || q.Subs[0].DurationMs != 15000 || q.Subs[1].Result != "answered" {
		t.Errorf("queue: %+v subs %+v %+v", q, q.Subs[0], q.Subs[1])
	}
	if q.Detail["holdMs"] != "22000" {
		t.Errorf("hold %q", q.Detail["holdMs"])
	}
	if talk.Party == nil || talk.Party.Number != "202" || talk.DurationMs != 60000 {
		t.Errorf("talk: %+v", talk)
	}
	if hang.Party == nil || hang.Party.Kind != "caller" {
		t.Errorf("hangup: %+v", hang.Party)
	}
	if j.Result != "answered" || j.Direction != "inbound" || j.TalkMs != 60000 || j.WaitMs != 30000 || !j.Complete || j.To != "0201234567" {
		t.Errorf("summary: %+v", j)
	}
	if j.Steps[0].Detail["trunk"] != "KPN" || j.Steps[0].Detail["route"] != "Hoofdnummer" {
		t.Errorf("inbound: %+v", j.Steps[0].Detail)
	}
}

func TestBuildInternalTransferVoicemail(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	a := "PJSIP/201-00000001"
	evs := []Event{
		{At: at(0), Kind: "CHAN_START", UniqueID: "1", Channel: a, Data: map[string]string{"num": "201", "name": "Piet", "exten": "202"}},
		{At: at(0.1), Kind: "kt_ext", UniqueID: "1", Channel: a, Data: map[string]string{"num": "202"}},
		{At: at(0.2), Kind: "DIAL", UniqueID: "1", Channel: a, Data: map[string]string{"dest": "PJSIP/202-00000002", "destId": "2"}},
		{At: at(20.2), Kind: "DIALEND", UniqueID: "1", Channel: a, Data: map[string]string{"dest": "PJSIP/202-00000002", "destId": "2", "status": "NOANSWER"}},
		{At: at(20.3), Kind: "kt_extend", UniqueID: "1", Channel: a, Data: map[string]string{"num": "202", "status": "NOANSWER"}},
		{At: at(20.3), Kind: "kt_vm", UniqueID: "1", Channel: a, Data: map[string]string{"box": "202", "reason": "noanswer"}},
		{At: at(40), Kind: "HANGUP", UniqueID: "1", Channel: a, Data: map[string]string{"hangupsource": a}},
		{At: at(40), Kind: "LINKEDID_END", UniqueID: "1", Channel: a},
	}
	j := Build("1", evs, nil)
	if j.Result != "voicemail" || j.Direction != "internal" {
		t.Fatalf("summary %+v", j)
	}
	ext := j.Steps[1]
	if ext.Kind != "extension" || ext.Result != "noanswer" || len(ext.Subs) != 1 || ext.Subs[0].Result != "noanswer" {
		t.Errorf("ext: %+v", ext)
	}
	if h := j.Steps[len(j.Steps)-1]; h.Kind != "hangup" || h.Party.Number != "201" {
		t.Errorf("hangup %+v", h)
	}

	// Answered, then a blind transfer to 203.
	evs = []Event{
		{At: at(0), Kind: "CHAN_START", UniqueID: "1", Channel: "PJSIP/trunk-2-00000009", Data: map[string]string{"num": "0611", "exten": "0201"}},
		{At: at(0.1), Kind: "kt_ext", UniqueID: "1", Data: map[string]string{"num": "202"}},
		{At: at(0.2), Kind: "DIAL", UniqueID: "1", Data: map[string]string{"dest": "PJSIP/202-0000000a", "destId": "2"}},
		{At: at(3), Kind: "DIALEND", UniqueID: "1", Data: map[string]string{"dest": "PJSIP/202-0000000a", "destId": "2", "status": "ANSWER"}},
		{At: at(33), Kind: "BLINDTRANSFER", UniqueID: "2", Channel: "PJSIP/202-0000000a", Data: map[string]string{"extension": "203"}},
		{At: at(33.1), Kind: "kt_ext", UniqueID: "1", Data: map[string]string{"num": "203"}},
		{At: at(33.2), Kind: "DIAL", UniqueID: "1", Data: map[string]string{"dest": "PJSIP/203-0000000b", "destId": "3"}},
		{At: at(36), Kind: "DIALEND", UniqueID: "1", Data: map[string]string{"dest": "PJSIP/203-0000000b", "destId": "3", "status": "ANSWER"}},
		{At: at(96), Kind: "HANGUP", UniqueID: "3", Channel: "PJSIP/203-0000000b", Data: map[string]string{"hangupsource": "PJSIP/203-0000000b"}},
		{At: at(96), Kind: "HANGUP", UniqueID: "1", Channel: "PJSIP/trunk-2-00000009", Data: map[string]string{"hangupsource": "PJSIP/203-0000000b"}},
		{At: at(96), Kind: "LINKEDID_END", UniqueID: "1"},
	}
	j = Build("1", evs, nil)
	kinds := ""
	for _, s := range j.Steps {
		kinds += s.Kind + " "
	}
	if kinds != "inbound extension talk transfer extension talk hangup " {
		t.Fatalf("steps: %s", kinds)
	}
	if j.TalkMs != 90000 || j.Steps[2].DurationMs != 30000 || j.Steps[3].Result != "blind" || j.Steps[3].Party.Number != "202" {
		t.Errorf("transfer: %+v talk=%d", j.Steps[3], j.TalkMs)
	}
	if h := j.Steps[6]; h.Party.Number != "203" {
		t.Errorf("hangup by %+v", h.Party)
	}
}
