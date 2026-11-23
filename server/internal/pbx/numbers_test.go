package pbx_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func TestTrunkNumbers(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := &pbx.Repo{DB: db}
	a := &pbx.Trunk{Name: "A", AuthMode: "register", Host: "sip.a.nl", Port: 5060, Transport: "udp", Username: "u", Secret: "secret-long-enough-1",
		DIDSource: "to", Codecs: "alaw", Enabled: true, Numbers: []string{"020 123 4500", "0201234501", "0201234500", ""}}
	if err := r.SaveTrunk(ctx, a); err != nil {
		t.Fatal(err)
	}
	if len(a.Numbers) != 2 || a.Numbers[0] != "0201234500" {
		t.Fatalf("numbers not cleaned: %v", a.Numbers)
	}
	// A number of another trunk is refused, and the new trunk is not left behind.
	b := &pbx.Trunk{Name: "B", AuthMode: "ip", Host: "1.2.3.4", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw",
		Enabled: true, Numbers: []string{"0201234501"}}
	var ve *pbx.ValidationError
	if err := r.SaveTrunk(ctx, b); err == nil || !asVE(err, &ve) || ve.Field != "numbers" {
		t.Fatalf("clash accepted: %v", err)
	}
	if ts, _ := r.Trunks(ctx); len(ts) != 1 {
		t.Fatalf("half-made trunk kept: %d", len(ts))
	}
	if err := r.SaveTrunk(ctx, &pbx.Trunk{Name: "C", AuthMode: "ip", Host: "1.2.3.4", Transport: "udp", DIDSource: "to", Codecs: "alaw", Numbers: []string{"12ab"}}); err == nil {
		t.Fatal("letters accepted")
	}
	// Saving without a numbers list keeps them.
	a.Numbers = nil
	a.Secret = ""
	if err := r.SaveTrunk(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Trunk(ctx, a.ID)
	if len(got.Numbers) != 2 {
		t.Fatalf("numbers lost: %v", got.Numbers)
	}
	// A route on a number shows in the number list.
	rt := &pbx.InboundRoute{Name: "Main", DID: "0201234500", DestinationType: "hangup", Enabled: true}
	if err := r.SaveInboundRoute(ctx, rt); err != nil {
		t.Fatal(err)
	}
	ns, err := r.Numbers(ctx)
	if err != nil || len(ns) != 2 || ns[0].RouteID != rt.ID || ns[0].TrunkName != "A" || ns[1].RouteID != 0 {
		t.Fatalf("numbers: %v %+v", err, ns)
	}
	// Deleting the trunk deletes its numbers.
	if err := r.DeleteTrunk(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if ns, _ := r.Numbers(ctx); len(ns) != 0 {
		t.Fatalf("numbers left: %v", ns)
	}
}

func asVE(err error, ve **pbx.ValidationError) bool {
	v, ok := err.(*pbx.ValidationError)
	*ve = v
	return ok
}
