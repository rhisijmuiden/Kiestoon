package freepbx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
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

const coreSQL = "-- MySQL dump 10.19  Distrib 10.5.22-MariaDB\n" +
	"DROP TABLE IF EXISTS `devices`;\n" +
	"CREATE TABLE `devices` (\n  `id` varchar(20) NOT NULL DEFAULT '',\n  `tech` varchar(10) NOT NULL DEFAULT '',\n  `dial` varchar(255) NOT NULL DEFAULT '',\n" +
	"  `devicetype` varchar(5) NOT NULL DEFAULT '',\n  `user` varchar(50) DEFAULT NULL,\n  `description` varchar(50) DEFAULT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=MyISAM;\n" +
	"/*!40000 ALTER TABLE `devices` DISABLE KEYS */;\n" +
	"INSERT INTO `devices` VALUES ('201','pjsip','PJSIP/201','fixed','201','Anna de Vries'),('202','pjsip','PJSIP/202','fixed','202','Bram O\\'Neil');\n" +
	"CREATE TABLE `sip` (\n  `id` varchar(20) NOT NULL DEFAULT '-1',\n  `keyword` varchar(30) NOT NULL DEFAULT '',\n  `data` varchar(255) NOT NULL,\n  `flags` int(1) NOT NULL DEFAULT 0\n);\n" +
	"INSERT INTO `sip` VALUES ('201','secret','9f8e7d6c5b4a3f2e1d0c',2),('201','context','from-internal',3),('202','secret','a1b2c3d4e5f6a7b8c9d0;x',2);\n"

func writeBackup(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "20261123-120000-1700000000-16.0.40-123456.tar.gz")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte) {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		tw.Write(data)
	}
	js := func(v any) []byte { b, _ := json.Marshal(v); return b }
	add("metadata.json", js(map[string]any{"backupInfo": map[string]any{"backup_name": "nightly"}, "pbx_version": "16.0.40.7", "hostname": "pbx.example.nl"}))
	add("modulejson/Core.json", js(map[string]any{"configs": map[string]any{
		"Users": map[string]any{"users": []any{
			map[string]any{"extension": "201", "name": "Anna de Vries", "voicemail": "default", "outboundcid": "\"Anna\" <0201234567>"},
			map[string]any{"extension": "202", "name": "Bram O'Neil", "voicemail": "novm", "outboundcid": ""},
		}},
		"Trunks": map[string]any{
			"trunks": []any{map[string]any{"trunkid": "1", "name": "Voys", "tech": "pjsip", "outcid": "0201234567", "disabled": "off"}},
			"techTables": map[string]any{"pjsip": []any{
				map[string]any{"id": "1", "keyword": "sip_server", "data": "ha.voys.nl"},
				map[string]any{"id": "1", "keyword": "username", "data": "150012345"},
				map[string]any{"id": "1", "keyword": "secret", "data": "TrunkPassw0rd"},
				map[string]any{"id": "1", "keyword": "registration", "data": "send"},
				map[string]any{"id": "1", "keyword": "codecs", "data": "alaw,g722"},
			}},
		},
		"Did": []any{
			map[string]any{"extension": "0201234567", "cidnum": "", "destination": "timeconditions,1,1", "description": "Main"},
			map[string]any{"extension": "0201234568", "cidnum": "", "destination": "from-did-direct,202,1"},
			map[string]any{"extension": "", "cidnum": "", "destination": "app-blackhole,hangup,1"},
		},
	}}))
	add("modulejson/Ringgroups.json", js(map[string]any{"configs": map[string]any{"groups": []any{
		map[string]any{"grpnum": "600", "description": "Sales", "strategy": "ringall", "grptime": 20, "grplist": "201-202-0612345678#", "postdest": "ext-local,vmb201,1"},
	}}}))
	add("modulejson/Queues.json", js(map[string]any{"configs": map[string]any{"tables": map[string]any{
		"queues_config":  []any{map[string]any{"extension": "700", "descr": "Support", "maxwait": "300", "dest": "ext-group,600,1", "joinannounce_id": "2"}},
		"queues_details": []any{map[string]any{"id": "700", "keyword": "strategy", "data": "rrmemory"}, map[string]any{"id": "700", "keyword": "member", "data": "Local/201@from-queue/n,0"}, map[string]any{"id": "700", "keyword": "member", "data": "Local/202@from-queue/n,0"}},
	}}}))
	add("modulejson/Ivr.json", js(map[string]any{"configs": map[string]any{
		"ivrs":    []any{map[string]any{"id": "1", "name": "Hoofdmenu", "announcement": "1", "timeout_destination": "ext-queues,700,1", "invalid_destination": ""}},
		"entries": []any{map[string]any{"ivr_id": "1", "selection": "1", "dest": "ext-group,600,1"}, map[string]any{"ivr_id": "1", "selection": "2", "dest": "ext-queues,700,1"}, map[string]any{"ivr_id": "1", "selection": "9", "dest": "ivr-1,s,1"}},
	}}))
	add("modulejson/Timeconditions.json", js(map[string]any{"configs": map[string]any{"tables": map[string]any{
		"timeconditions":     []any{map[string]any{"timeconditions_id": "1", "displayname": "Kantoortijden", "time": "1", "truegoto": "ivr-1,s,1", "falsegoto": "ext-local,vmu201,1"}},
		"timegroups_groups":  []any{map[string]any{"id": "1", "description": "Office"}},
		"timegroups_details": []any{map[string]any{"id": "1", "timegroupid": "1", "time": "08:30-17:00|mon-fri|*|*"}, map[string]any{"id": "2", "timegroupid": "1", "time": "*|*|25|dec"}},
	}}}))
	add("modulejson/Recordings.json", js(map[string]any{"configs": map[string]any{"data": []any{
		map[string]any{"id": "1", "displayname": "welkom", "filename": "custom/welkom"},
		map[string]any{"id": "2", "displayname": "queue", "filename": "custom/wachtrij"},
	}}}))
	add("modulejson/Voicemail.json", js(map[string]any{"configs": map[string]any{"mailboxData": map[string]any{
		"201": map[string]any{"voicemail_enable": "yes", "voicemail_vmpwd": "1234", "voicemail_email": "anna@example.com", "voicemail_options": "attach=yes"},
	}}}))
	add("files/var/spool/asterisk/backup/Core.sql", []byte(coreSQL))
	wav := append([]byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x40\x1f\x00\x00\x80\x3e\x00\x00\x02\x00\x10\x00data\x00\x00\x00\x00"), make([]byte, 0)...)
	add("files/var/lib/asterisk/sounds/en/custom/welkom.wav", wav)
	add("files/var/lib/asterisk/sounds/en/custom/wachtrij.wav", wav)
	tw.Close()
	gz.Close()
	f.Close()
	return p
}

func TestFreePBX16Backup(t *testing.T) {
	ctx := context.Background()
	p := writeBackup(t)
	u, err := importer.NewUpload(filepath.Base(p), p)
	if err != nil {
		t.Fatal(err)
	}
	if a := importer.Detect(u); a == nil || a.ID() != "freepbx" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.Product != "FreePBX 16.0.40.7" || b.Info.Host != "pbx.example.nl" {
		t.Errorf("info %+v", b.Info)
	}
	if len(b.Users) != 2 || b.Users[0].SIPPassword != "9f8e7d6c5b4a3f2e1d0c" || b.Users[0].VoicemailPIN != "1234" || b.Users[1].Name != "Bram O'Neil" ||
		b.Users[1].Voicemail || b.Users[0].OutboundCID != "0201234567" {
		t.Errorf("users %+v", b.Users)
	}
	if len(b.Groups) != 1 || !slices.Equal(b.Groups[0].Members, []string{"201", "202"}) || !slices.Equal(b.Groups[0].External, []string{"0612345678"}) ||
		b.Groups[0].Failover.Kind != importer.TargetVoicemail {
		t.Errorf("groups %+v", b.Groups)
	}
	if len(b.Queues) != 1 || len(b.Queues[0].Agents) != 2 || !strings.HasSuffix(b.Queues[0].Greeting, "wachtrij.wav") || b.Queues[0].Overflow.Kind != importer.TargetGroup {
		t.Errorf("queues %+v", b.Queues)
	}
	if len(b.TimeConditions) != 1 || len(b.TimeConditions[0].Schedule) != 1 || b.TimeConditions[0].Open.Ref != "ivr-1" {
		t.Errorf("times %+v", b.TimeConditions)
	}
	if m := b.Menu("ivr-1"); m == nil || len(m.Children) != 3 || !strings.HasSuffix(m.Audio, "welkom.wav") || m.NoAction.Kind != importer.TargetQueue {
		t.Errorf("menu %+v", m)
	}
	if len(b.Trunks) != 1 || b.Trunks[0].Host != "ha.voys.nl" || !b.Trunks[0].Register || len(b.Trunks[0].Numbers) != 2 {
		t.Errorf("trunks %+v", b.Trunks)
	}
	if len(b.Numbers) != 2 || b.Numbers[0].Target.Kind != importer.TargetTime {
		t.Errorf("numbers %+v", b.Numbers)
	}

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
	res, err := importer.Apply(ctx, r, np, b, plan, c)
	if err != nil {
		t.Fatalf("%v %+v", err, res)
	}
	if len(res.Created) != 2+1+1+1+1+2+1 {
		t.Errorf("created %+v warnings %+v", res.Created, res.Warnings)
	}
	g, _ := r.RingGroups(ctx)
	if len(g) != 1 || !slices.Equal(g[0].Members, []string{"201", "202", "0612345678"}) || g[0].Failover.Type != pbx.DestVoicemail {
		t.Errorf("group %+v", g)
	}
}

// A FreePBX 2.x-14 backup, or a mysqldump of the asterisk database, has everything in SQL.
func TestLegacyDump(t *testing.T) {
	sql := coreSQL +
		"CREATE TABLE `users` (\n  `extension` varchar(20),\n  `password` varchar(20),\n  `name` varchar(50),\n  `voicemail` varchar(50)\n);\n" +
		"INSERT INTO `users` VALUES ('201','','Anna de Vries','default'),('202',NULL,'Bram','novm');\n" +
		"CREATE TABLE `ringgroups` (\n  `grpnum` varchar(20),\n  `strategy` varchar(50),\n  `grptime` smallint,\n  `grppre` varchar(100),\n  `grplist` varchar(255),\n  `postdest` varchar(255),\n  `description` varchar(35)\n);\n" +
		"INSERT INTO `ringgroups` VALUES ('600','hunt',15,'','201-202','app-blackhole,hangup,1','Hunt; group');\n" +
		"CREATE TABLE `incoming` (\n  `cidnum` varchar(20),\n  `extension` varchar(50),\n  `destination` varchar(50)\n);\n" +
		"INSERT INTO `incoming` VALUES ('','+31201234567','ext-group,600,1');\n"
	p := filepath.Join(t.TempDir(), "asterisk.sql")
	os.WriteFile(p, []byte(sql), 0o600)
	u, _ := importer.NewUpload("asterisk.sql", p)
	if a := importer.Detect(u); a == nil || a.ID() != "freepbx" {
		t.Fatalf("detect %v", a)
	}
	b, err := importer.Read(context.Background(), u, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Users) != 2 || b.Users[1].SIPPassword != "a1b2c3d4e5f6a7b8c9d0;x" || len(b.Groups) != 1 || b.Groups[0].Strategy != "sequential" ||
		b.Groups[0].Name != "Hunt; group" || len(b.Numbers) != 1 || b.Numbers[0].Target.Ref != "600" {
		t.Errorf("bundle users %+v groups %+v numbers %+v", b.Users, b.Groups, b.Numbers)
	}
}
