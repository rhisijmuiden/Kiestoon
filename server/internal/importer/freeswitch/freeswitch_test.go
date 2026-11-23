package freeswitch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	_ "github.com/rhisijmuiden/kiestoon/server/internal/importer/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func applyAll(t *testing.T, b *importer.Bundle) *importer.Result {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &pbx.Repo{DB: db}
	np := pbx.PlanFor(4)
	plan, err := importer.Prepare(ctx, r, np, b)
	if err != nil {
		t.Fatal(err)
	}
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
	for _, x := range plan.TimeConditions {
		c.TimeConditions = append(c.TimeConditions, x.Key)
	}
	for _, x := range plan.Numbers {
		c.Numbers = append(c.Numbers, x.Key)
	}
	for _, x := range plan.Menus {
		c.Menus = append(c.Menus, x.Key)
	}
	for _, a := range plan.Audio {
		p := pbx.Prompt{Name: a.Name}
		if err := r.SavePrompt(ctx, &p); err != nil {
			t.Fatal(err)
		}
		c.Prompts[a.File] = p.ID
	}
	res, err := importer.Apply(ctx, r, np, b, plan, c)
	if err != nil {
		t.Fatalf("%v %+v", err, res)
	}
	return res
}

func TestFreeSWITCHConfig(t *testing.T) {
	u, err := importer.NewUpload("freeswitch", "testdata")
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "freeswitch" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Users) != 2 || b.Users[0].Name != "Anna de Vries" || b.Users[0].SIPPassword != "Wachtw00rd1000" || b.Users[1].SIPPassword != "1234" ||
		b.Users[0].OutboundCID != "" {
		t.Errorf("users %+v", b.Users)
	}
	if len(b.Groups) != 1 || b.Groups[0].Ext != "2000" || !slices.Equal(b.Groups[0].Members, []string{"1000", "1001"}) {
		t.Errorf("groups %+v", b.Groups)
	}
	if len(b.Queues) != 1 || b.Queues[0].Ext != "7000" || b.Queues[0].Strategy != "leastrecent" || len(b.Queues[0].Agents) != 2 {
		t.Errorf("queues %+v", b.Queues)
	}
	m := b.Menu("main_menu")
	if m == nil || m.Ext != "5000" || !strings.HasSuffix(m.Audio, "welcome.wav") || len(m.Children) != 4 || m.NoAction == nil || m.NoAction.Ref != "1000" {
		t.Fatalf("menu %+v", m)
	}
	if m.Children[2].Target.Kind != importer.TargetExternal || m.Children[3].Target.Ref != "main_menu" {
		t.Errorf("keys %+v %+v", m.Children[2].Target, m.Children[3].Target)
	}
	if len(b.TimeConditions) != 1 || !slices.Equal(b.TimeConditions[0].Schedule[0].Days, []int{1, 2, 3, 4, 5}) || b.TimeConditions[0].Schedule[0].To != "17:30" ||
		b.TimeConditions[0].Open.Ref != "main_menu" || b.TimeConditions[0].Closed.Kind != importer.TargetMenu {
		t.Errorf("times %+v", b.TimeConditions)
	}
	if len(b.Numbers) != 2 || b.Numbers[0].Number != "+31201234567" || b.Numbers[0].Target.Kind != importer.TargetTime || b.Numbers[1].Target.Ref != "1001" {
		t.Errorf("numbers %+v", b.Numbers)
	}
	if len(b.Trunks) != 1 || b.Trunks[0].Host != "ha.voys.nl" || len(b.Trunks[0].Numbers) != 2 {
		t.Errorf("trunks %+v", b.Trunks)
	}
	res := applyAll(t, b)
	if len(res.Warnings) > 0 {
		t.Errorf("warnings %+v", res.Warnings)
	}
}

const fusionSQL = `--
-- PostgreSQL database dump
--
COPY public.v_domains (domain_uuid, domain_name, domain_enabled) FROM stdin;
d1	pbx.example.nl	true
\.
COPY public.v_extensions (extension_uuid, domain_uuid, extension, password, effective_caller_id_name, outbound_caller_id_number, enabled, description) FROM stdin;
e1	d1	101	Zx8!kLmn42Qa	Anna de Vries	\N	true	\N
e2	d1	102	Yq7?abcd99Rt	Bram Jansen	0201234567	true	\N
\.
COPY public.v_voicemails (voicemail_uuid, voicemail_id, voicemail_password, voicemail_mail_to, voicemail_enabled) FROM stdin;
v1	101	4321	anna@example.com	true
\.
COPY public.v_ring_groups (ring_group_uuid, ring_group_name, ring_group_extension, ring_group_strategy, ring_group_call_timeout, ring_group_timeout_app, ring_group_timeout_data) FROM stdin;
rg1	Verkoop	600	sequence	25	transfer	*99101 XML pbx.example.nl
\.
COPY public.v_ring_group_destinations (ring_group_destination_uuid, ring_group_uuid, destination_number) FROM stdin;
x1	rg1	101
x2	rg1	102
x3	rg1	0612345678
\.
COPY public.v_ivr_menus (ivr_menu_uuid, ivr_menu_name, ivr_menu_extension, ivr_menu_greet_long, ivr_menu_exit_app, ivr_menu_exit_data) FROM stdin;
i1	Hoofdmenu	500	welcome.wav	transfer	101 XML pbx.example.nl
\.
COPY public.v_ivr_menu_options (ivr_menu_option_uuid, ivr_menu_uuid, ivr_menu_option_digits, ivr_menu_option_action, ivr_menu_option_param, ivr_menu_option_description) FROM stdin;
o1	i1	1	menu-exec-app	transfer 600 XML pbx.example.nl	Verkoop
o2	i1	2	menu-exec-app	transfer 102 XML pbx.example.nl	Bram
\.
COPY public.v_gateways (gateway_uuid, gateway, username, password, proxy, realm, register, enabled) FROM stdin;
g1	Voys	150012345	TrunkPassw0rd	ha.voys.nl	\N	true	true
\.
COPY public.v_destinations (destination_uuid, destination_type, destination_number, destination_app, destination_data, destination_enabled, destination_actions) FROM stdin;
dd1	inbound	0201234567	\N	\N	true	[{"destination_app":"transfer","destination_data":"500 XML pbx.example.nl"}]
\.
`

func TestFusionPBX(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fusionpbx.sql"), []byte(fusionSQL), 0o600)
	os.MkdirAll(filepath.Join(dir, "recordings", "pbx.example.nl"), 0o755)
	w, _ := os.ReadFile("testdata/sounds/custom/welcome.wav")
	os.WriteFile(filepath.Join(dir, "recordings", "pbx.example.nl", "welcome.wav"), w, 0o600)
	u, _ := importer.NewUpload("fusionpbx.sql", dir)
	if a := importer.Detect(u); a == nil || a.ID() != "freeswitch" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.Product != "FusionPBX" || len(b.Users) != 2 || b.Users[0].VoicemailPIN != "4321" || b.Users[1].OutboundCID != "0201234567" {
		t.Errorf("users %+v", b.Users)
	}
	if len(b.Groups) != 1 || b.Groups[0].Strategy != "sequential" || len(b.Groups[0].External) != 1 || b.Groups[0].Failover.Kind != importer.TargetVoicemail {
		t.Errorf("groups %+v", b.Groups)
	}
	m := b.Menu("i1")
	if m == nil || len(m.Children) != 2 || m.Children[0].Target.Kind != importer.TargetGroup || m.NoAction.Ref != "101" || !strings.HasSuffix(m.Audio, "welcome.wav") {
		t.Errorf("menu %+v", m)
	}
	if len(b.Numbers) != 1 || b.Numbers[0].Target.Ref != "i1" || len(b.Trunks) != 1 {
		t.Errorf("numbers %+v trunks %+v", b.Numbers, b.Trunks)
	}
	res := applyAll(t, b)
	if len(res.Warnings) > 0 {
		t.Errorf("warnings %+v", res.Warnings)
	}
}
