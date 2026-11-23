package freeswitch

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/importer/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pgdump"
)

// readFusion reads a FusionPBX database dump (custom format or plain SQL) when the upload has one.
func readFusion(u *importer.Upload, b *importer.Bundle, sounds *asterisk.Sounds) (bool, error) {
	var dump *pgdump.Dump
	files, err := u.Files(func(name string, size int64) bool {
		n := strings.ToLower(name)
		return strings.HasSuffix(n, ".sql") || strings.HasSuffix(n, ".sql.gz") || strings.HasSuffix(n, ".backup") || strings.HasSuffix(n, ".dump") ||
			strings.HasSuffix(n, ".pgdump") || !u.IsArchive()
	}, importer.MaxFile)
	if err != nil {
		return false, err
	}
	for name, data := range files {
		if strings.HasSuffix(strings.ToLower(name), ".gz") {
			if gz, err := gzip.NewReader(bytes.NewReader(data)); err == nil {
				if x, err := io.ReadAll(io.LimitReader(gz, importer.MaxFile)); err == nil {
					data = x
				}
			}
		}
		var d *pgdump.Dump
		if bytes.HasPrefix(data, []byte("PGDMP")) {
			d, err = pgdump.Read(bytes.NewReader(data))
		} else if bytes.Contains(data, []byte("v_extensions")) {
			d, err = pgdump.ReadPlain(bytes.NewReader(data))
		} else {
			continue
		}
		if err == nil && d != nil && len(d.Table("public", "v_extensions").Rows) > 0 {
			dump = d
			break
		}
	}
	if dump == nil {
		return false, nil
	}
	f := &fusion{d: dump, b: b, sounds: sounds, byNumber: map[string]*importer.Target{}, unknown: map[string]bool{}}
	f.build()
	return true, nil
}

type fusion struct {
	d        *pgdump.Dump
	b        *importer.Bundle
	sounds   *asterisk.Sounds
	byNumber map[string]*importer.Target // internal number -> what it is
	unknown  map[string]bool
}

func (f *fusion) rows(t string) []map[string]string { return f.d.Table("public", t).Rows }

func on(v string) bool { return v == "" || v == "true" || v == "t" }

func (f *fusion) build() {
	b := f.b
	b.Info.Product = "FusionPBX"
	for _, r := range f.rows("v_domains") {
		b.Info.Host = r["domain_name"]
		break
	}
	vm := map[string]map[string]string{}
	for _, r := range f.rows("v_voicemails") {
		vm[r["voicemail_id"]] = r
	}
	for _, r := range f.rows("v_extensions") {
		ext := r["extension"]
		if ext == "" {
			continue
		}
		u := importer.User{Ref: ext, Ext: ext, Name: r["effective_caller_id_name"], SIPPassword: r["password"], OutboundCID: r["outbound_caller_id_number"],
			Disabled: !on(r["enabled"])}
		if u.Name == "" {
			u.Name = r["description"]
		}
		if v, ok := vm[ext]; ok && on(v["voicemail_enabled"]) {
			u.Voicemail, u.VoicemailPIN, u.Email = true, v["voicemail_password"], v["voicemail_mail_to"]
			u.VoicemailEmail = u.Email != ""
		}
		b.Users = append(b.Users, u)
		f.byNumber[ext] = &importer.Target{Kind: importer.TargetUser, Ref: ext}
	}
	for _, r := range f.rows("v_ring_groups") {
		f.byNumber[r["ring_group_extension"]] = &importer.Target{Kind: importer.TargetGroup, Ref: r["ring_group_uuid"]}
	}
	for _, r := range f.rows("v_call_center_queues") {
		f.byNumber[r["queue_extension"]] = &importer.Target{Kind: importer.TargetQueue, Ref: r["call_center_queue_uuid"]}
	}
	for _, r := range f.rows("v_ivr_menus") {
		f.byNumber[r["ivr_menu_extension"]] = &importer.Target{Kind: importer.TargetMenu, Ref: r["ivr_menu_uuid"]}
	}
	recordings := map[string]string{}
	for _, r := range f.rows("v_recordings") {
		if a := f.sounds.Find(r["recording_filename"]); a != "" {
			recordings[r["recording_filename"]] = a
		}
	}
	sound := func(s string) string {
		if a, ok := recordings[s]; ok {
			return a
		}
		return f.sounds.Find(s)
	}
	for _, r := range f.rows("v_ring_groups") {
		g := importer.Group{Ref: r["ring_group_uuid"], Ext: r["ring_group_extension"], Name: r["ring_group_name"], RingSeconds: atoi(r["ring_group_call_timeout"]),
			Failover: f.app(r["ring_group_timeout_app"], r["ring_group_timeout_data"])}
		switch r["ring_group_strategy"] {
		case "sequence", "rollover", "enterprise":
			g.Strategy = "sequential"
		case "random":
			g.Strategy = "rotating"
		default:
			g.Strategy = "simultaneous"
		}
		for _, d := range f.rows("v_ring_group_destinations") {
			if d["ring_group_uuid"] != g.Ref {
				continue
			}
			n := d["destination_number"]
			if b.User(n) != nil {
				g.Members = append(g.Members, n)
			} else if reNum.MatchString(n) {
				g.External = append(g.External, n)
			}
		}
		b.Groups = append(b.Groups, g)
	}
	agents := map[string]string{}
	for _, a := range f.rows("v_call_center_agents") {
		id := contactUser(a["agent_contact"])
		if id == "" {
			id = a["agent_id"]
		}
		agents[a["call_center_agent_uuid"]] = id
	}
	for _, r := range f.rows("v_call_center_queues") {
		q := importer.Queue{Ref: r["call_center_queue_uuid"], Ext: r["queue_extension"], Name: r["queue_name"], Strategy: callcenterStrategy(r["queue_strategy"]),
			MaxWait: atoi(r["queue_max_wait_time"]), Greeting: sound(r["queue_greeting"])}
		if act := r["queue_timeout_action"]; act != "" {
			app, data, _ := strings.Cut(act, ":")
			q.Overflow = f.app(app, data)
		}
		for _, t := range f.rows("v_call_center_tiers") {
			if t["call_center_queue_uuid"] == q.Ref {
				if id := agents[t["call_center_agent_uuid"]]; b.User(id) != nil {
					q.Agents = append(q.Agents, id)
				}
			}
		}
		b.Queues = append(b.Queues, q)
	}
	for _, r := range f.rows("v_ivr_menus") {
		s := &importer.Step{Ref: r["ivr_menu_uuid"], Type: "root", Title: r["ivr_menu_name"], Ext: r["ivr_menu_extension"], Audio: sound(r["ivr_menu_greet_long"]),
			NoAction: f.app(r["ivr_menu_exit_app"], r["ivr_menu_exit_data"])}
		var opts []map[string]string
		for _, o := range f.rows("v_ivr_menu_options") {
			if o["ivr_menu_uuid"] == s.Ref {
				opts = append(opts, o)
			}
		}
		slices.SortFunc(opts, func(a, b map[string]string) int { return strings.Compare(a["ivr_menu_option_digits"], b["ivr_menu_option_digits"]) })
		for _, o := range opts {
			d := o["ivr_menu_option_digits"]
			if len(d) != 1 {
				continue
			}
			app, data, _ := strings.Cut(o["ivr_menu_option_param"], " ")
			if o["ivr_menu_option_action"] == "menu-top" {
				s.Children = append(s.Children, &importer.Step{Type: "menu", Key: d, Title: o["ivr_menu_option_description"], Target: &importer.Target{Kind: importer.TargetMenu, Ref: s.Ref}})
				continue
			}
			if o["ivr_menu_option_action"] == "menu-sub" {
				for _, m := range f.rows("v_ivr_menus") {
					if m["ivr_menu_name"] == o["ivr_menu_option_param"] || m["ivr_menu_uuid"] == o["ivr_menu_option_param"] {
						s.Children = append(s.Children, &importer.Step{Type: "menu", Key: d, Title: o["ivr_menu_option_description"], Target: &importer.Target{Kind: importer.TargetMenu, Ref: m["ivr_menu_uuid"]}})
					}
				}
				continue
			}
			s.Children = append(s.Children, &importer.Step{Type: "menu", Key: d, Title: o["ivr_menu_option_description"], Target: f.app(app, data)})
		}
		b.Menus = append(b.Menus, s)
	}
	for _, r := range f.rows("v_gateways") {
		if !on(r["enabled"]) {
			continue
		}
		host := r["proxy"]
		if host == "" {
			host = r["realm"]
		}
		t := importer.Trunk{Ref: r["gateway_uuid"], Name: r["gateway"], Username: r["username"], Password: r["password"], AuthUsername: r["auth_username"],
			Register: r["register"] != "false", FromDomain: r["from_domain"], FromUser: r["from_user"], Transport: strings.ToLower(r["register_transport"])}
		if h, port, ok := strings.Cut(host, ":"); ok {
			host = h
			t.Port, _ = strconv.Atoi(port)
		}
		t.Host = host
		if t.AuthUsername == t.Username {
			t.AuthUsername = ""
		}
		if t.Host != "" {
			b.Trunks = append(b.Trunks, t)
		}
	}
	for _, r := range f.rows("v_destinations") {
		if r["destination_type"] != "" && r["destination_type"] != "inbound" || !on(r["destination_enabled"]) {
			continue
		}
		num := strings.TrimSpace(r["destination_number"])
		if num == "" || !reNum.MatchString(strings.TrimPrefix(num, "+")) {
			continue
		}
		var t *importer.Target
		if acts := r["destination_actions"]; acts != "" {
			var list []map[string]string
			if json.Unmarshal([]byte(acts), &list) == nil && len(list) > 0 {
				t = f.app(list[0]["destination_app"], list[0]["destination_data"])
			}
		}
		if t == nil {
			t = f.app(r["destination_app"], r["destination_data"])
		}
		b.Numbers = append(b.Numbers, importer.Number{Number: num, Target: t})
		if t != nil && t.Kind == importer.TargetMenu {
			if m := b.Menu(t.Ref); m != nil {
				m.Numbers = append(m.Numbers, num)
			}
		}
	}
	if len(b.Trunks) == 1 {
		for _, n := range b.Numbers {
			b.Trunks[0].Numbers = append(b.Trunks[0].Numbers, n.Number)
		}
	}
	if len(f.rows("v_dialplans")) > 0 {
		b.Note("fusion_time_conditions")
	}
	if len(f.unknown) > 0 {
		var list []string
		for k := range f.unknown {
			list = append(list, k)
		}
		slices.Sort(list)
		b.Note("dialplan_skipped", "apps", strings.Join(list, ", "))
	}
}

var reNum = regexp.MustCompile(`^\+?[0-9]{3,20}$`)

// app turns a FusionPBX application with data ("transfer", "1001 XML example.com") into a target.
func (f *fusion) app(app, data string) *importer.Target {
	app = strings.TrimSpace(app)
	data = strings.TrimSpace(data)
	switch app {
	case "":
		return nil
	case "transfer":
		num := strings.Fields(data)
		if len(num) == 0 {
			return nil
		}
		n := num[0]
		if strings.HasPrefix(n, "*99") && f.b.User(n[3:]) != nil {
			return &importer.Target{Kind: importer.TargetVoicemail, Ref: n[3:]}
		}
		if t, ok := f.byNumber[n]; ok {
			return t
		}
		if reNum.MatchString(n) && len(n) > 6 {
			return &importer.Target{Kind: importer.TargetExternal, Ref: n}
		}
		return nil
	case "bridge":
		if strings.Contains(data, "sofia/gateway/") {
			_, rest, _ := strings.Cut(data, "sofia/gateway/")
			if _, num, ok := strings.Cut(rest, "/"); ok && reNum.MatchString(num) {
				return &importer.Target{Kind: importer.TargetExternal, Ref: num}
			}
		}
		if id := contactUser(data); f.b.User(id) != nil {
			return &importer.Target{Kind: importer.TargetUser, Ref: id}
		}
	case "voicemail":
		fs := strings.Fields(data)
		if len(fs) > 0 && f.b.User(fs[len(fs)-1]) != nil {
			return &importer.Target{Kind: importer.TargetVoicemail, Ref: fs[len(fs)-1]}
		}
	case "hangup":
		return nil
	}
	f.unknown[app] = true
	return nil
}
