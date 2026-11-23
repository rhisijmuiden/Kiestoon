package pbx_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func TestExtensionGroups(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := &pbx.Repo{DB: db}
	plan := pbx.NumberPlan{Digits: 3}
	a := &pbx.Extension{Number: "201", Name: "Anna", Email: "anna@example.com", Voicemail: true, VoicemailEmail: true, RecordMode: "ondemand", OutboundClass: "international", Enabled: true}
	b := &pbx.Extension{Number: "202", Name: "Bas", Voicemail: true, RecordMode: "ondemand", OutboundClass: "national", Enabled: true}
	for _, e := range []*pbx.Extension{a, b} {
		if err := r.SaveExtension(ctx, plan, e); err != nil {
			t.Fatal(err)
		}
	}
	off, nat := false, "national"
	g := &pbx.ExtensionGroup{Name: "Sales", CallerID: "020 111 1111", Members: []int64{a.ID, b.ID, a.ID},
		Settings: pbx.GroupSettings{Voicemail: &off, OutboundClass: &nat}}
	if err := r.SaveExtensionGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	if g.CallerID != "0201111111" || len(g.Members) != 2 {
		t.Fatalf("not cleaned: %+v", g)
	}
	// Enforced settings land on the members; others stay as they were.
	got, _ := r.Extension(ctx, a.ID)
	if got.Voicemail || got.OutboundClass != "national" || got.RecordMode != "ondemand" || got.GroupID == nil || *got.GroupID != g.ID {
		t.Fatalf("settings not applied: %+v", got)
	}
	// Saving the extension cannot undo an enforced setting.
	got.Voicemail, got.OutboundClass, got.RecordMode = true, "international", "always"
	got.GroupID = nil
	if err := r.SaveExtension(ctx, plan, got); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Extension(ctx, a.ID)
	if got.Voicemail || got.OutboundClass != "national" || got.RecordMode != "always" {
		t.Fatalf("group setting overridden: %+v", got)
	}
	// Same name twice, unknown member, bad setting.
	var ve *pbx.ValidationError
	if err := r.SaveExtensionGroup(ctx, &pbx.ExtensionGroup{Name: "sales"}); !asVE(err, &ve) || ve.Code != "duplicate" {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := r.SaveExtensionGroup(ctx, &pbx.ExtensionGroup{Name: "X", Members: []int64{99}}); !asVE(err, &ve) || ve.Field != "members" {
		t.Fatalf("unknown member: %v", err)
	}
	bad := 2
	if err := r.SaveExtensionGroup(ctx, &pbx.ExtensionGroup{Name: "Y", Settings: pbx.GroupSettings{RingSeconds: &bad}}); err == nil {
		t.Fatal("ring time 2 s accepted")
	}
	// An extension is in one group: adding it elsewhere moves it.
	g2 := &pbx.ExtensionGroup{Name: "Support", Members: []int64{b.ID}}
	if err := r.SaveExtensionGroup(ctx, g2); err != nil {
		t.Fatal(err)
	}
	gs, _ := r.ExtensionGroups(ctx)
	if len(gs) != 2 || len(gs[0].Members) != 1 || gs[0].Name != "Sales" || gs[1].Members[0] != b.ID {
		t.Fatalf("membership: %+v", gs)
	}
	// Taking an extension out through the extension form.
	zero := int64(0)
	got.GroupID = &zero
	if err := r.SaveExtension(ctx, plan, got); err != nil {
		t.Fatal(err)
	}
	if e, _ := r.Extension(ctx, a.ID); *e.GroupID != 0 {
		t.Fatal("still in group")
	}

	// Routes limited to groups; a group in use cannot be deleted.
	tr := &pbx.Trunk{Name: "T", AuthMode: "ip", Host: "1.2.3.4", Port: 5060, Transport: "udp", DIDSource: "to", Codecs: "alaw", Enabled: true}
	if err := r.SaveTrunk(ctx, tr); err != nil {
		t.Fatal(err)
	}
	rt := &pbx.OutboundRoute{Name: "Sales", Patterns: []pbx.DialPattern{{Match: "0X."}}, TrunkIDs: []int64{tr.ID}, GroupIDs: []int64{g2.ID, g.ID, g.ID}, Enabled: true}
	if err := r.SaveOutboundRoute(ctx, rt); err != nil {
		t.Fatal(err)
	}
	rs, _ := r.OutboundRoutes(ctx)
	if len(rs[0].GroupIDs) != 2 {
		t.Fatalf("group ids: %v", rs[0].GroupIDs)
	}
	if err := r.SaveOutboundRoute(ctx, &pbx.OutboundRoute{Name: "Z", Patterns: []pbx.DialPattern{{Match: "0X."}}, TrunkIDs: []int64{tr.ID}, GroupIDs: []int64{42}}); err == nil {
		t.Fatal("unknown group accepted")
	}
	if err := r.DeleteExtensionGroup(ctx, g.ID); !asVE(err, &ve) || ve.Code != "group_in_use" {
		t.Fatalf("delete in use: %v", err)
	}
	rt.GroupIDs = []int64{g2.ID}
	if err := r.SaveOutboundRoute(ctx, rt); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteExtensionGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting an extension takes it out of its group.
	if err := r.DeleteExtension(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if gs, _ := r.ExtensionGroups(ctx); len(gs) != 1 || len(gs[0].Members) != 0 {
		t.Fatalf("member kept: %+v", gs)
	}
}
