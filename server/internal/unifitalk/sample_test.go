package unifitalk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// TestSample imports a real backup when KT_TALK_SAMPLE points to one.
func TestSample(t *testing.T) {
	p := os.Getenv("KT_TALK_SAMPLE")
	if p == "" {
		t.Skip("KT_TALK_SAMPLE not set")
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	talk := a.Bundle()
	talk.Info.System = "unifi-talk"
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := &pbx.Repo{DB: db}
	np := pbx.PlanFor(4)
	plan, err := importer.Prepare(ctx, r, np, talk)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(plan, "", " ")
	os.WriteFile(filepath.Join(os.TempDir(), "talk-plan.json"), b, 0o600)
	c := importer.Choice{Prompts: map[string]int64{}}
	for _, x := range plan.Extensions {
		c.Extensions = append(c.Extensions, x.Key)
	}
	for _, x := range plan.RingGroups {
		c.RingGroups = append(c.RingGroups, x.Key)
	}
	for _, x := range plan.Queues {
		c.Queues = append(c.Queues, x.Key)
	}
	for _, x := range plan.Trunks {
		c.Trunks = append(c.Trunks, x.Key)
	}
	for _, x := range plan.Numbers {
		c.Numbers = append(c.Numbers, x.Key)
	}
	for _, x := range plan.Menus {
		c.Menus = append(c.Menus, x.Key)
	}
	for _, au := range plan.Audio {
		pr := pbx.Prompt{Name: au.Name, DurationMS: 1000}
		if err := r.SavePrompt(ctx, &pr); err != nil {
			t.Fatal(err)
		}
		c.Prompts[au.File] = pr.ID
	}
	res, err := importer.Apply(ctx, r, np, talk, plan, c)
	if err != nil {
		t.Fatalf("%v (%+v)", err, res)
	}
	b, _ = json.MarshalIndent(res, "", " ")
	t.Logf("%s", b)
	ivrs, _ := r.IVRs(ctx)
	b, _ = json.MarshalIndent(ivrs, "", " ")
	os.WriteFile(filepath.Join(os.TempDir(), "talk-ivrs.json"), b, 0o600)
	// A second run reuses everything.
	plan2, _ := importer.Prepare(ctx, r, np, talk)
	res2, err := importer.Apply(ctx, r, np, talk, plan2, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range res2.Created {
		if !x.Reused && x.Kind != "route" {
			t.Errorf("not reused: %+v", x)
		}
	}
}
