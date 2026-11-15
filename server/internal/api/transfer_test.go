package api

import (
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestActiveCallsAndConsult(t *testing.T) {
	e := &pbx.Extension{Number: "201", WebRTC: true}
	l := &asterisk.Live{Connected: true, Calls: []asterisk.Call{{
		ID: "a",
		Legs: []asterisk.Channel{
			{Channel: "PJSIP/trunk-1-00000001", Endpoint: "trunk-1", State: "Up", Duration: 40, BridgeID: "b1"},
			{Channel: "PJSIP/201-web-00000002", Endpoint: "201-web", State: "Up", Duration: 38, Connected: "0612345678", ConnName: "Jan", BridgeID: "b1"},
		},
	}, {
		ID: "b",
		Legs: []asterisk.Channel{
			{Channel: "PJSIP/202-00000003", Endpoint: "202", State: "Up", Duration: 100},
			{Channel: "PJSIP/2010-00000004", Endpoint: "2010", State: "Up", Duration: 100},
		},
	}}}
	var x transfers
	calls := activeCalls(l, e, &x)
	if len(calls) != 1 || calls[0].Channel != "PJSIP/201-web-00000002" || calls[0].Device != "web" || calls[0].Peer != "0612345678" || calls[0].State != "talking" {
		t.Fatalf("active calls: %+v", calls)
	}

	// Consultation to 203: the transferer moved into a new bridge with the Local channel.
	x.set("PJSIP/201-web-00000002", consult{Target: "203", TargetName: "Piet", Peer: "0612345678", PeerName: "Jan"})
	l.Calls[0].Legs[1].BridgeID, l.Calls[0].Legs[1].Connected = "b2", "203"
	l.Calls = append(l.Calls, asterisk.Call{ID: "c", Legs: []asterisk.Channel{
		{Channel: "Local/203@kiestoon-local-00000005;1", Endpoint: "Local/203@kiestoon-local-00000005;1", State: "Ringing", BridgeID: ""},
	}})
	calls = activeCalls(l, e, &x)
	if c := calls[0]; c.Consult == nil || c.Consult.Target != "203" || c.Consult.Answered || c.Peer != "0612345678" {
		t.Fatalf("consult ringing: %+v %+v", c, c.Consult)
	}
	if legs := l.ConsultLegs(l.Calls[0].Legs[1], xferContext, "203"); len(legs) != 1 {
		t.Fatalf("consult legs while ringing: %+v", legs)
	}
	l.Calls[2].Legs[0].State, l.Calls[2].Legs[0].BridgeID = "Up", "b2"
	if c := activeCalls(l, e, &x)[0]; !c.Consult.Answered {
		t.Fatalf("consult answered: %+v", c.Consult)
	}

	x.prune(func(string) bool { return false })
	if _, ok := x.get("PJSIP/201-web-00000002"); ok {
		t.Fatal("prune kept a gone channel")
	}
}

func TestFindTarget(t *testing.T) {
	list := []TransferTarget{{Kind: "extension", Number: "201"}, {Kind: "queue", Number: "600"}}
	if tg, ok := findTarget(list, "600"); !ok || tg.Kind != "queue" {
		t.Fatal("queue not found")
	}
	if _, ok := findTarget(list, "0612345678"); ok {
		t.Fatal("external number accepted as a target")
	}
}
