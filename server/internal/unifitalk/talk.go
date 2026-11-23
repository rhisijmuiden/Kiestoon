package unifitalk

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pgdump"
)

// The phone system as UniFi Talk stores it in its database, turned into an importer.Bundle.
// Users and group members are referred to by their ULP id (the UniFi identity), attendant steps
// by their switchboard item id; the bundle uses the same ids as refs.

func init() { importer.Register(adapter{}) }

type adapter struct{}

func (adapter) ID() string { return "unifi-talk" }

// Detect: a UniFi OS backup is encrypted, so it is recognised by its name; an unpacked one by the
// Talk database inside.
func (adapter) Detect(u *importer.Upload) int {
	if u.IsArchive() && u.Has("backup/talk/unifi-talk.dump") {
		return 100
	}
	if strings.HasSuffix(strings.ToLower(u.Name), ".unifi") && !u.IsArchive() {
		return 90
	}
	return 0
}

func (adapter) Read(ctx context.Context, u *importer.Upload) (*importer.Bundle, error) {
	f, err := u.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	a, err := Read(io.LimitReader(f, importer.MaxUpload))
	switch err {
	case nil:
	case ErrNotBackup:
		return nil, importer.ErrUnknownFormat
	case ErrNoTalk:
		return nil, importer.ErrEmpty
	default:
		return nil, importer.ErrDamaged
	}
	return a.Bundle(), nil
}

func jsonMap(s string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(s), &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func strs(v any) []string {
	var out []string
	if a, ok := v.([]any); ok {
		for _, x := range a {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

func target(kind, ref string) *importer.Target {
	if ref == "" {
		return nil
	}
	return &importer.Target{Kind: kind, Ref: ref}
}

// Talk queue strategies (FreeSWITCH mod_callcenter) to Asterisk queue strategies.
var queueStrategies = map[string]string{
	"ring-all":                         "ringall",
	"ring-progressively":               "ringall",
	"longest-idle-agent":               "leastrecent",
	"round-robin":                      "rrmemory",
	"top-down":                         "linear",
	"sequentially-by-agent-order":      "linear",
	"sequentially-by-next-agent-order": "rrmemory",
	"agent-with-least-talk-time":       "fewestcalls",
	"agent-with-fewest-calls":          "fewestcalls",
	"random":                           "random",
}

// failover turns a Talk "no answer" action into a target.
func failover(action string, vm []string, transferUser string) *importer.Target {
	switch {
	case strings.Contains(action, "voicemail") && len(vm) > 0:
		return target(importer.TargetVoicemail, vm[0])
	case transferUser != "":
		return target(importer.TargetUser, transferUser)
	}
	return nil
}

// Bundle reads the phone system from the Talk database of the archive.
func (a *Archive) Bundle() *importer.Bundle {
	d := a.Dump
	b := &importer.Bundle{Audio: a.Audio, Info: importer.Info{Product: "UniFi Talk " + a.TalkVersion, CreatedAt: a.CreatedAt,
		Details: map[string]string{}}}
	if a.TalkVersion == "" {
		b.Info.Product = "UniFi Talk"
	}
	for k, v := range map[string]string{"console": a.Console, "firmware": a.Firmware, "timezone": a.TimeZone} {
		if v != "" {
			b.Info.Details[k] = v
		}
	}
	numbers := map[string]*importer.Target{}

	// Global voicemail greeting.
	if a.Config != nil {
		if s, ok := a.Config["voicemail_greeting_filename"].(string); ok && s != "" {
			if _, ok := a.Audio["voicemail_greetings/"+s]; ok {
				b.VoicemailGreeting = "voicemail_greetings/" + s
			}
		}
	}
	if b.VoicemailGreeting == "" {
		if _, ok := a.Audio["voicemail_greetings/global_greeting.mp3"]; ok {
			b.VoicemailGreeting = "voicemail_greetings/global_greeting.mp3"
		}
	}

	// Users.
	userID := map[string]string{}
	for _, r := range d.Table("public", "user").Rows {
		userID[r["id"]] = r["ulp_id"]
		if r["ext"] == "" || r["ulp_id"] == "" {
			continue
		}
		name := strings.TrimSpace(r["first_name"] + " " + r["last_name"])
		if name == "" {
			name = r["ext"]
		}
		email := r["talk_local_email"]
		if email == "" {
			email = r["sso_email"]
		}
		// outbound_caller_id is often a small index into Talk's own number list, not a number.
		cid := r["outbound_caller_id"]
		if len(strings.TrimLeft(cid, "+")) < 6 {
			cid = ""
		}
		b.Users = append(b.Users, importer.User{Ref: r["ulp_id"], Ext: r["ext"], Name: name, Email: email, SIPPassword: r["sip_password"],
			Voicemail: r["voicemail_enabled"] != "f", OutboundCID: cid})
	}
	slices.SortFunc(b.Users, func(x, y importer.User) int { return strings.Compare(x.Ext, y.Ext) })
	for _, r := range d.Table("public", "user_did").Rows {
		if u := userID[r["user"]]; u != "" && r["direction"] != "outbound" {
			numbers[r["did"]] = target(importer.TargetUser, u)
		}
	}

	// Call groups.
	exts := map[string]string{}
	for _, r := range d.Table("public", "group_ext").Rows {
		exts[r["group"]] = r["ext"]
	}
	type member struct {
		ulp   string
		order int
	}
	members := map[string][]member{}
	for _, r := range d.Table("public", "group_member").Rows {
		o := 1 << 20
		if r["sequential_call_order"] != "" {
			o = atoi(r["sequential_call_order"])
		}
		members[r["group"]] = append(members[r["group"]], member{r["member_id"], o})
	}
	for _, r := range d.Table("public", "group").Rows {
		g := importer.Group{Ref: r["id"], Name: r["name"], Ext: exts[r["id"]], Strategy: "simultaneous"}
		if r["call_handling"] == "sequential" {
			g.Strategy = "sequential"
		}
		if t := atoi(r["simultaneous_ring_timeout"]); t > 0 {
			g.RingSeconds = t
		}
		var vm []string
		_ = json.Unmarshal([]byte(r["vm_receiver_uuids"]), &vm)
		switch {
		case r["transfer_to_group_id"] != "":
			g.Failover = target(importer.TargetGroup, r["transfer_to_group_id"])
		case r["transfer_to_queue_id"] != "":
			g.Failover = target(importer.TargetQueue, r["transfer_to_queue_id"])
		case r["transfer_to_node_id"] != "":
			g.Failover = target(importer.TargetMenu, strings.TrimPrefix(r["transfer_to_node_id"], "swb_"))
		case r["transfer_to_contact_did"] != "":
			g.Failover = target(importer.TargetExternal, r["transfer_to_contact_did"])
		default:
			g.Failover = failover(r["no_answer_action"], vm, r["transfer_to_user_uuid"])
		}
		ms := members[r["id"]]
		slices.SortStableFunc(ms, func(x, y member) int { return x.order - y.order })
		for _, m := range ms {
			if !slices.Contains(g.Members, m.ulp) && b.User(m.ulp) != nil {
				g.Members = append(g.Members, m.ulp)
			}
		}
		b.Groups = append(b.Groups, g)
	}
	for _, r := range d.Table("public", "group_did").Rows {
		numbers[r["did"]] = target(importer.TargetGroup, r["group"])
	}

	// Call queues.
	agents := map[string][]string{}
	tiers := slices.Clone(d.Table("fsw", "tiers").Rows)
	slices.SortStableFunc(tiers, func(x, y map[string]string) int { return atoi(x["position"]) - atoi(y["position"]) })
	for _, r := range tiers {
		q, ulp, ok := strings.Cut(r["agent"], "_")
		if ok && q == r["queue"] && b.User(ulp) != nil {
			agents[q] = append(agents[q], ulp)
		}
	}
	for _, r := range d.Table("public", "call_queue").Rows {
		strategy := queueStrategies[r["strategy"]]
		if strategy == "" {
			strategy = "rrmemory"
		}
		q := importer.Queue{Ref: r["id"], Name: r["name"], Ext: r["ext"], Strategy: strategy, MaxWait: atoi(r["max_wait_time"]),
			RingTime: atoi(r["agent_ring_time"]), WrapUp: atoi(r["agent_wrap_up_time"]), MaxSize: atoi(r["max_queue_size"]),
			Agents: agents[r["id"]], Disabled: r["enabled"] == "f"}
		var vm []string
		if v, ok := jsonMap(r["options"])["voicemail"].(map[string]any); ok {
			vm = strs(v["user_ids"])
		}
		action := str(jsonMap(r["timeouts"]), "action")
		if action == "" {
			action = str(jsonMap(r["overflow"]), "action")
		}
		q.Overflow = failover(action, vm, "")
		if g := r["greeting_file"]; g != "" {
			if _, ok := a.Audio["call_center/"+g]; ok {
				q.Greeting = "call_center/" + g
			}
		}
		prefix := "call_center/queue_" + q.Ref + "_"
		for name := range a.Audio {
			if strings.HasPrefix(name, prefix) && name != q.Greeting {
				q.Messages = append(q.Messages, name)
			}
		}
		slices.Sort(q.Messages)
		b.Queues = append(b.Queues, q)
	}
	for _, r := range d.Table("public", "call_queue_did").Rows {
		numbers[r["did"]] = target(importer.TargetQueue, r["queue"])
	}

	// SIP trunks.
	gwNumbers := map[string][]string{}
	for _, r := range d.Table("public", "did_number").Rows {
		gwNumbers[r["sip_gateway"]] = append(gwNumbers[r["sip_gateway"]], r["did"])
	}
	for _, r := range d.Table("public", "sip_gateway").Rows {
		m := jsonMap(r["data"])
		p, _ := m["gateway_params"].(map[string]any)
		if p == nil {
			continue
		}
		tr := importer.Trunk{Ref: r["id"], Name: str(m, "name"), Host: str(p, "proxy"), Port: 5060, Username: str(p, "username"),
			Password: str(p, "password"), Register: str(p, "register") == "true", Transport: strings.ToLower(str(p, "register-transport")),
			Networks: strs(m["acl_ip_cidr_list"]), Numbers: gwNumbers[r["id"]]}
		if tr.Host == "" {
			tr.Host = str(p, "realm")
		}
		if h, port, ok := strings.Cut(tr.Host, ":"); ok && atoi(port) > 0 {
			tr.Host, tr.Port = h, atoi(port)
		}
		if tr.Transport != "tcp" && tr.Transport != "tls" {
			tr.Transport = "udp"
		}
		if tr.Name == "" {
			tr.Name = tr.Host
		}
		slices.Sort(tr.Numbers)
		b.Trunks = append(b.Trunks, tr)
	}

	// Smart attendants.
	parseAttendants(d, a, b, numbers)

	var dids []string
	for n := range numbers {
		dids = append(dids, n)
	}
	slices.Sort(dids)
	for _, n := range dids {
		b.Numbers = append(b.Numbers, importer.Number{Number: n, Target: numbers[n]})
	}

	if n := len(d.Table("public", "device").Rows); n > 0 {
		b.Note("phones_not_imported", "count", strconv.Itoa(n))
	}
	if b.VoicemailGreeting != "" {
		b.Note("greeting_as_prompt")
	}
	for _, q := range b.Queues {
		if len(q.Messages) > 0 {
			b.Note("queue_messages", "name", q.Name)
		}
	}
	return b
}

// noAction reads the "when nothing is pressed" target of an attendant item.
func noAction(m map[string]any) *importer.Target {
	for _, c := range []struct{ key, kind string }{
		{"noaction_transfer_to_user_uuid", importer.TargetUser},
		{"noaction_transfer_to_group_id", importer.TargetGroup},
		{"noaction_transfer_to_queue_id", importer.TargetQueue},
		{"noaction_transfer_to_node_id", importer.TargetMenu},
		{"noaction_transfer_to_contact_did", importer.TargetExternal},
	} {
		if v := str(m, c.key); v != "" {
			return target(c.kind, strings.TrimPrefix(v, "swb_"))
		}
	}
	if vm := strs(m["noaction_vm_receiver_uuids"]); len(vm) > 0 {
		return target(importer.TargetVoicemail, vm[0])
	}
	return nil
}

func parseAttendants(d *pgdump.Dump, a *Archive, b *importer.Bundle, numbers map[string]*importer.Target) {
	steps := map[int]*importer.Step{}
	parent := map[int]int{}
	var order []int
	for _, r := range d.Table("public", "swb_item").Rows {
		id := atoi(r["id"])
		m := jsonMap(r["data"])
		s := &importer.Step{ID: id, Title: strings.TrimSpace(str(m, "title")), Ext: str(m, "ext"),
			Numbers: strs(m["numbers"]), Disabled: str(m, "disabled") == "true", NoAction: noAction(m), Key: str(m, "key")}
		switch str(m, "type") {
		case "root":
			s.Type, s.Ref = "root", strconv.Itoa(id)
		case "audio":
			s.Type = "audio"
		default:
			s.Type = "menu"
		}
		if f := str(m, "greeting_file_name"); f != "" && str(m, "greeting_type") != "none" {
			for _, dir := range []string{"ivr/", ""} {
				if _, ok := a.Audio[dir+f]; ok {
					s.Audio = dir + f
					break
				}
			}
		}
		if s.Audio == "" && str(m, "greeting_type") == "tts" {
			s.Greeting = str(m, "greeting")
		}
		steps[id] = s
		if p := r["parent"]; p != "" {
			parent[id] = atoi(p)
		}
		order = append(order, id)
	}
	slices.Sort(order)
	// Where a menu option sends the call. A step can ring several targets; the first one counts.
	set := func(table, col, kind string) {
		for _, r := range d.Table("public", table).Rows {
			s := steps[atoi(r["swb_item"])]
			if s == nil || s.Target != nil || r["disabled"] == "t" {
				continue
			}
			ref := r[col]
			if kind == importer.TargetExternal && r["target_did"] != "" {
				ref = r["target_did"]
			}
			s.Target = target(kind, ref)
		}
	}
	set("swb_item_ulp_user", "ulp_user", importer.TargetUser)
	set("swb_item_group", "group_id", importer.TargetGroup)
	set("swb_item_queue", "queue_id", importer.TargetQueue)
	set("swb_item_sa_transfer", "transfer_target_id", importer.TargetMenu)
	set("swb_item_contact", "contact_uuid", importer.TargetExternal)
	for _, id := range order {
		s := steps[id]
		if p, ok := parent[id]; ok {
			if ps := steps[p]; ps != nil {
				if !s.Disabled {
					ps.Children = append(ps.Children, s)
				}
				continue
			}
		}
		if s.Type == "root" {
			b.Menus = append(b.Menus, s)
			for _, n := range s.Numbers {
				numbers[n] = target(importer.TargetMenu, s.Ref)
			}
		}
	}
	for _, s := range steps {
		slices.SortStableFunc(s.Children, func(x, y *importer.Step) int { return keyRank(x.Key) - keyRank(y.Key) })
	}
}

func keyRank(k string) int {
	if k == "" {
		return 100
	}
	if i := strings.Index("1234567890*#", k); i >= 0 {
		return i
	}
	return 50
}
