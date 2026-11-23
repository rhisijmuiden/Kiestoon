package asterisk

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func readTestdata(t *testing.T) *importer.Bundle {
	t.Helper()
	u, err := importer.NewUpload("asterisk", "testdata")
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "asterisk" {
		t.Fatalf("detect: %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReadConfig(t *testing.T) {
	b := readTestdata(t)
	if b.Info.Product != "Asterisk 20.5.0" {
		t.Errorf("product %q", b.Info.Product)
	}
	var exts []string
	for _, u := range b.Users {
		exts = append(exts, u.Ext+" "+u.Name)
	}
	if want := []string{"201 Anna de Vries", "202 Bram Jansen", "203 Receptie"}; !slices.Equal(exts, want) {
		t.Errorf("users %v", exts)
	}
	anna := b.User("201")
	if !anna.Voicemail || anna.VoicemailPIN != "1234" || anna.Email != "anna@example.com" || anna.SIPPassword != "Secr3tPass201" {
		t.Errorf("anna %+v", anna)
	}
	if len(b.Trunks) != 1 {
		t.Fatalf("trunks %+v", b.Trunks)
	}
	tr := b.Trunks[0]
	if tr.Host != "ha.voys.nl" || tr.Username != "150012345" || tr.Password != "TrunkPassw0rd" || !tr.Register || tr.Codecs != "alaw,g722" ||
		!slices.Equal(tr.Networks, []string{"195.35.114.0/23"}) {
		t.Errorf("trunk %+v", tr)
	}
	if len(b.Queues) != 1 || b.Queues[0].Ext != "700" || !slices.Equal(b.Queues[0].Agents, []string{"201", "202"}) || b.Queues[0].Greeting == "" {
		t.Errorf("queues %+v", b.Queues)
	}
	if len(b.Groups) != 1 || b.Groups[0].Ext != "600" || b.Groups[0].Failover == nil || b.Groups[0].Failover.Kind != importer.TargetVoicemail {
		t.Errorf("groups %+v", b.Groups)
	}
	if len(b.TimeConditions) != 1 || len(b.TimeConditions[0].Schedule) != 2 {
		t.Fatalf("times %+v", b.TimeConditions)
	}
	tc := b.TimeConditions[0]
	if tc.Schedule[0].From != "08:30" || !slices.Equal(tc.Schedule[0].Days, []int{1, 2, 3, 4, 5}) || tc.Open.Kind != importer.TargetMenu ||
		tc.Closed == nil || tc.Closed.Kind != importer.TargetMenu {
		t.Errorf("time %+v open %+v closed %+v", tc, tc.Open, tc.Closed)
	}
	main := b.Menu("ivr-main")
	if main == nil || main.Ext != "800" || !strings.HasSuffix(main.Audio, "welcome.wav") || len(main.Children) != 4 || main.NoAction == nil {
		j, _ := json.MarshalIndent(b.Menus, "", " ")
		t.Fatalf("menus %s", j)
	}
	if k3 := main.Children[2]; k3.Key != "3" || k3.Target.Kind != importer.TargetExternal || k3.Target.Ref != "0612345678" {
		t.Errorf("key 3 %+v", k3.Target)
	}
	if k9 := main.Children[3]; k9.Target.Kind != importer.TargetMenu || k9.Target.Ref != "ivr-main" {
		t.Errorf("key 9 %+v", k9.Target)
	}
	var nums []string
	for _, n := range b.Numbers {
		nums = append(nums, n.Number)
	}
	if !slices.Equal(nums, []string{"+31201234567", "0201234568"}) || !slices.Equal(tr.Numbers, nums) {
		t.Errorf("numbers %v trunk %v", nums, tr.Numbers)
	}
	// The queue greeting (raw G.711) is converted to WAV for the browser.
	if g := b.Audio[b.Queues[0].Greeting]; !importer.AudioPlayable(g) {
		t.Errorf("greeting not playable: %q", b.Queues[0].Greeting)
	}
}

func TestImportConfig(t *testing.T) {
	ctx := context.Background()
	b := readTestdata(t)
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := &pbx.Repo{DB: db}
	np := pbx.PlanFor(3)
	plan, err := importer.Prepare(ctx, r, np, b)
	if err != nil {
		t.Fatal(err)
	}
	c := importer.Choice{Prompts: map[string]int64{}, NumberOf: map[string]string{}}
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
	for _, x := range plan.TimeConditions {
		c.TimeConditions = append(c.TimeConditions, x.Key)
	}
	for _, x := range plan.Numbers {
		c.Numbers = append(c.Numbers, x.Key)
	}
	for _, x := range plan.Menus {
		c.Menus = append(c.Menus, x.Key)
	}
	for i, a := range plan.Audio {
		p := pbx.Prompt{Name: a.Name}
		if err := r.SavePrompt(ctx, &p); err != nil {
			t.Fatal(err)
		}
		c.Prompts[a.File] = p.ID
		_ = i
	}
	res, err := importer.Apply(ctx, r, np, b, plan, c)
	if err != nil {
		j, _ := json.MarshalIndent(res, "", " ")
		t.Fatalf("%v\n%s", err, j)
	}
	t.Logf("warnings %v notes %v menus %d", res.Warnings, plan.Notes, len(plan.Menus))
	kinds := map[string]int{}
	for _, x := range res.Created {
		kinds[x.Kind]++
	}
	if kinds["extension"] != 3 || kinds["ringgroup"] != 1 || kinds["queue"] != 1 || kinds["trunk"] != 1 || kinds["timecondition"] != 1 ||
		kinds["route"] != 2 || kinds["ivr"] < 2 {
		t.Errorf("created %v warnings %v", kinds, res.Warnings)
	}
	exts, _ := r.Extensions(ctx)
	if exts[0].Number != "201" || exts[0].SIPSecret != "Secr3tPass201" || exts[0].VoicemailPIN != "1234" || exts[2].SIPSecret == "short" {
		t.Errorf("extensions %+v", exts)
	}
	trunks, _ := r.Trunks(ctx)
	if trunks[0].Provider != "voys" || len(trunks[0].Numbers) != 2 {
		t.Errorf("trunk %+v", trunks[0])
	}
	ivrs, _ := r.IVRs(ctx)
	for _, v := range ivrs {
		if v.Name == "ivr-main" && len(v.Options) != 4 {
			t.Errorf("ivr options %+v", v.Options)
		}
	}
	// A second import reuses everything.
	plan2, _ := importer.Prepare(ctx, r, np, b)
	for _, x := range plan2.Extensions {
		if x.ExistingID == 0 {
			t.Errorf("not reused: %+v", x)
		}
	}
	for _, x := range plan2.Numbers {
		if !x.Exists {
			t.Errorf("number not routed: %+v", x)
		}
	}
}
