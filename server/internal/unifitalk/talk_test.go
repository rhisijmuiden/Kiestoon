package unifitalk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/pgdump"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func tbl(schema, name string, cols string, rows ...string) *pgdump.Table {
	t := &pgdump.Table{Schema: schema, Name: name, Columns: strings.Split(cols, ",")}
	for _, r := range rows {
		m := map[string]string{}
		for i, v := range strings.Split(r, "|") {
			if v != "<null>" {
				m[t.Columns[i]] = v
			}
		}
		t.Rows = append(t.Rows, m)
	}
	return t
}

// fixture is a small Talk system like a real one: a main menu with three options (an extension,
// a sub menu with "press 9 for the menu again" through a forwarding attendant, a queue), a call
// group, a queue and a Voys trunk.
func fixture(t *testing.T) []byte {
	t.Helper()
	tables := []*pgdump.Table{
		tbl("public", "user", "id,ext,ulp_id,first_name,last_name,sip_password,voicemail_enabled",
			"1|0001|ulp-a|Anna|Jansen|Secret-Pass-123|t", "2|0002|ulp-b|Bert||short|f", "3|3010|ulp-c|Meldkamer||Another-Pass-456|t"),
		tbl("public", "group", "id,name,call_handling,no_answer_action,vm_receiver_uuids,transfer_to_user_uuid",
			`1|Office|sequential|voicemail|["ulp-a"]|<null>`),
		tbl("public", "group_ext", "group,ext", "1|0010"),
		tbl("public", "group_member", "group,member_id,sequential_call_order", "1|ulp-b|1", "1|ulp-a|0", "1|ulp-x|2"),
		tbl("public", "group_did", "group,did", "1|+31201234500"),
		tbl("public", "call_queue", "id,name,enabled,ext,strategy,max_wait_time,max_queue_size,agent_ring_time,agent_wrap_up_time,options,timeouts,overflow,wait_music_type",
			`1|Support|t|9101|ring-all|30|0|20|5|{"voicemail": {"user_ids": ["ulp-c"]}}|{"action": "play_message_and_send_to_voicemail", "play_message_file": "queue_1_1_timeouts_message_file.mp3"}|{"action": "drop_call"}|{"type": "standard", "title": "Upbeat.wav"}`),
		tbl("fsw", "tiers", "queue,agent,position", "1|1_ulp-c|2", "1|1_ulp-a|1"),
		tbl("public", "did_number", "did,sip_gateway", "+31201234500|2", "+31201234501|2"),
		tbl("public", "sip_gateway", "id,data",
			`2|{"name": "Voys", "gateway_params": {"proxy": "sip.voys.nl", "username": "123456", "password": "Trunk-Secret-1", "register": true, "register-transport": "udp"}, "acl_ip_cidr_list": ["195.35.114.0/24"]}`),
		tbl("public", "swb_item", "id,parent,data",
			`10|<null>|{"type": "root", "title": "Main", "ext": "0008", "numbers": ["+31201234501"], "greeting_type": "uploaded", "greeting_file_name": "custom_root.mp3", "noaction_transfer_to_user_uuid": "ulp-b"}`,
			`11|10|{"key": 1, "type": "ivr", "title": "Sales", "greeting_type": "uploaded", "greeting_file_name": "custom_sales.mp3"}`,
			`12|10|{"key": 2, "type": "ivr", "title": "Info", "greeting_type": "uploaded", "greeting_file_name": "custom_info.mp3"}`,
			`13|12|{"type": "audio", "greeting_type": "uploaded", "greeting_file_name": "custom_infotext.mp3"}`,
			`14|13|{"key": 9, "type": "ivr", "title": "Again", "greeting_type": "none"}`,
			`15|10|{"key": 3, "type": "ivr", "title": "Support", "greeting_type": "none"}`,
			`16|10|{"key": 4, "type": "ivr", "title": "Off", "disabled": true}`,
			`20|<null>|{"type": "root", "title": "Forward", "numbers": [], "greeting_type": "none", "sa_menu_noaction": "drop_call"}`),
		tbl("public", "swb_item_ulp_user", "swb_item,ulp_user,disabled", "11|ulp-a|f"),
		tbl("public", "swb_item_queue", "swb_item,queue_id,disabled", "15|1|f"),
		tbl("public", "swb_item_sa_transfer", "swb_item,transfer_target_id,disabled", "14|20|f", "20|10|f"),
		tbl("public", "device", "mac", "a", "b"),
	}
	var dump bytes.Buffer
	if err := pgdump.Write(&dump, "unifi-talk", tables); err != nil {
		t.Fatal(err)
	}
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	add := func(name string, b []byte) {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg})
		tw.Write(b)
	}
	add("backup/metadata.json", []byte(`{"hardware_shortname":"UDMPRO","firmware_version":"6.0.17","created_at":1795438324390}`))
	add("backup/package-versions.json", []byte(`{"controllers":{"talk":"5.4.8"}}`))
	add("backup/talk/config.json", []byte(`{"voicemail_greeting_filename":"global_greeting.mp3"}`))
	add("backup/talk/unifi-talk.dump", dump.Bytes())
	for _, f := range []string{"ivr/custom_root.mp3", "ivr/custom_sales.mp3", "ivr/custom_info.mp3", "ivr/custom_infotext.mp3",
		"call_center/queue_1_1_timeouts_message_file.mp3", "voicemail_greetings/global_greeting.mp3"} {
		add("backup/talk/talk_audio/"+f, []byte("ID3fake"))
	}
	add("backup/ucore/config/unifi-core.key", []byte("never read"))
	tw.Close()
	gz.Close()
	// Encrypt like UniFi OS: IV, then AES-256-CBC without padding (zero-filled to a block).
	plain := tgz.Bytes()
	plain = append(plain, make([]byte, (aes.BlockSize-len(plain)%aes.BlockSize)%aes.BlockSize)...)
	iv := bytes.Repeat([]byte{7}, aes.BlockSize)
	block, _ := aes.NewCipher(consoleKey)
	enc := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(enc, plain)
	return append(iv, enc...)
}

func TestReadAndImport(t *testing.T) {
	a, err := Read(bytes.NewReader(fixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Console != "UDMPRO" || a.TalkVersion != "5.4.8" || len(a.Audio) != 6 {
		t.Fatalf("archive: %s %s %d", a.Console, a.TalkVersion, len(a.Audio))
	}
	talk := a.Bundle()
	talk.Info.System = "unifi-talk"
	if len(talk.Users) != 3 || len(talk.Menus) != 2 || talk.VoicemailGreeting == "" || talk.Info.Product != "UniFi Talk 5.4.8" {
		t.Fatalf("talk: %+v", talk)
	}
	if talk.Notes[0].Code != "phones_not_imported" || talk.Notes[0].Args["count"] != "2" {
		t.Errorf("notes: %+v", talk.Notes)
	}
	// ulp-x is no user: left out.
	if g := talk.Groups[0]; g.Strategy != "sequential" || !slices.Equal(g.Members, []string{"ulp-a", "ulp-b"}) || g.Failover == nil || g.Failover.Kind != "voicemail" {
		t.Fatalf("group: %+v", g)
	}
	if q := talk.Queues[0]; !slices.Equal(q.Agents, []string{"ulp-a", "ulp-c"}) || len(q.Messages) != 1 || q.Strategy != "ringall" {
		t.Fatalf("queue: %+v", q)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := &pbx.Repo{DB: db}
	np := pbx.PlanFor(3)
	// An extension that already exists by name is reused; its number is taken.
	anna := pbx.Extension{Number: "200", Name: "Anna Jansen", Enabled: true, Voicemail: true}
	if err := r.SaveExtension(ctx, np, &anna); err != nil {
		t.Fatal(err)
	}
	plan, err := importer.Prepare(ctx, r, np, talk)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Menus) != 1 {
		t.Fatalf("the forwarding attendant should be folded away: %+v", plan.Menus)
	}
	ext := map[string]importer.PlanExtension{}
	for _, e := range plan.Extensions {
		ext[e.Key] = e
	}
	if e := ext["u:ulp-a"]; e.ExistingID != anna.ID || e.Number != "200" {
		t.Errorf("anna not reused: %+v", e)
	}
	if e := ext["u:ulp-b"]; e.Number != "201" || e.KeepPassword {
		t.Errorf("bert: %+v", e)
	}
	if e := ext["u:ulp-c"]; e.Number != "202" || !e.KeepPassword {
		t.Errorf("meldkamer: %+v", e)
	}
	if plan.RingGroups[0].Number != "600" || plan.Queues[0].Number != "700" || plan.Trunks[0].Provider != "voys" {
		t.Errorf("numbers: %+v %+v %+v", plan.RingGroups, plan.Queues, plan.Trunks)
	}
	if len(plan.Numbers) != 2 || plan.Numbers[1].Target != "m:10" || plan.Numbers[0].Target != "g:1" {
		t.Errorf("phone numbers: %+v", plan.Numbers)
	}

	c := importer.Choice{Prompts: map[string]int64{}, NumberOf: map[string]string{"q:1": "701"}}
	for _, e := range plan.Extensions {
		c.Extensions = append(c.Extensions, e.Key)
	}
	c.RingGroups, c.Queues, c.Trunks, c.Menus = []string{"g:1"}, []string{"q:1"}, []string{"t:2"}, []string{"m:10"}
	for _, n := range plan.Numbers {
		c.Numbers = append(c.Numbers, n.Key)
	}
	for _, au := range plan.Audio {
		p := pbx.Prompt{Name: au.Name, DurationMS: 500}
		if err := r.SavePrompt(ctx, &p); err != nil {
			t.Fatal(err)
		}
		c.Prompts[au.File] = p.ID
	}
	res, err := importer.Apply(ctx, r, np, talk, plan, c)
	if err != nil {
		t.Fatalf("%v %+v", err, res)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings: %+v", res.Warnings)
	}
	g, _ := r.RingGroups(ctx)
	if len(g) != 1 || !slices.Equal(g[0].Members, []string{"200", "201"}) || g[0].Failover.Type != pbx.DestVoicemail {
		t.Errorf("ring group: %+v", g)
	}
	q, _ := r.Queues(ctx)
	if len(q) != 1 || q[0].Number != "701" || q[0].Strategy != "ringall" || len(q[0].Members) != 2 || q[0].Overflow.Target != "202" {
		t.Errorf("queue: %+v", q)
	}
	tr, _ := r.Trunks(ctx)
	if len(tr) != 1 || tr[0].Host != "ha.voys.nl" || tr[0].Username != "123456" || len(tr[0].Numbers) != 2 {
		t.Errorf("trunk: %+v", tr)
	}
	routes, _ := r.InboundRoutes(ctx)
	if len(routes) != 2 {
		t.Errorf("routes: %+v", routes)
	}
	ivrs, _ := r.IVRs(ctx)
	if len(ivrs) != 1 {
		t.Fatalf("ivrs: %+v", ivrs)
	}
	v := ivrs[0]
	f := v.Flow
	opts := map[string]pbx.Destination{}
	for _, o := range v.Options {
		opts[o.Digit] = o.Destination
	}
	if len(opts) != 3 || opts["1"].Target != "200" || opts["3"].Type != pbx.DestQueue || v.TimeoutDest.Target != "201" {
		t.Errorf("menu options: %+v timeout %+v", v.Options, v.TimeoutDest)
	}
	if f.Entry().PromptID == 0 {
		t.Error("welcome recording missing")
	}
	// Key 2: info recording, then a menu with 9 that goes back to this menu.
	k2 := f.Next("entry", "out")
	for _, k := range f.MenuKeys() {
		if k.Digit == "2" {
			k2 = k
		}
	}
	a1 := f.Next(k2.ID, "out")
	if a1.Type != pbx.NodeAudio || a1.PromptID == 0 {
		t.Fatalf("key 2 should play the info recording: %+v", a1)
	}
	m := f.Next(a1.ID, "out")
	if m.Type != pbx.NodeMenu || m.PromptID == 0 {
		t.Fatalf("then a menu with the text: %+v", m)
	}
	k9 := f.KeysOf(m.ID)
	if len(k9) != 1 || k9[0].Digit != "9" {
		t.Fatalf("sub menu keys: %+v", k9)
	}
	back := f.Next(k9[0].ID, "out")
	if back.Type != pbx.NodeDestination || back.Destination.Type != pbx.DestIVR || back.Destination.Target != "1" {
		t.Errorf("9 should go back to the main menu: %+v", back)
	}

	// Again: everything is reused, nothing is made twice.
	plan2, err := importer.Prepare(ctx, r, np, talk)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range plan2.Numbers {
		if !n.Exists {
			t.Errorf("number should be routed already: %+v", n)
		}
	}
	res2, err := importer.Apply(ctx, r, np, talk, plan2, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range res2.Created {
		if !x.Reused {
			t.Errorf("created twice: %+v", x)
		}
	}
}

func TestReadRejects(t *testing.T) {
	if _, err := Read(bytes.NewReader(bytes.Repeat([]byte{1}, 4096))); err != ErrNotBackup {
		t.Errorf("random data: %v", err)
	}
	if _, err := Read(bytes.NewReader([]byte("x"))); err != ErrNotBackup {
		t.Errorf("tiny file: %v", err)
	}
	// A backup of another app (no Talk).
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "backup/metadata.json", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("{}"))
	tw.Close()
	gz.Close()
	if _, err := Read(bytes.NewReader(tgz.Bytes())); err != ErrNoTalk {
		t.Errorf("no talk: %v", err)
	}
}

func TestAdapter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "unifi_os_backup_for_talk.unifi")
	if err := os.WriteFile(p, fixture(t), 0o600); err != nil {
		t.Fatal(err)
	}
	u, err := importer.NewUpload("unifi_os_backup_for_talk.unifi", p)
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "unifi-talk" {
		t.Fatalf("detect: %v", a)
	}
	b, err := importer.Read(context.Background(), u, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.System != "unifi-talk" || len(b.Menus) != 2 || len(b.Audio) != 6 {
		t.Errorf("bundle: %+v", b.Info)
	}
	// Another file with the same extension is refused with a clear error.
	if err := os.WriteFile(p, bytes.Repeat([]byte{3}, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	u, _ = importer.NewUpload("other.unifi", p)
	if _, err := importer.Read(context.Background(), u, "auto"); err != importer.ErrUnknownFormat {
		t.Errorf("garbage: %v", err)
	}
}
