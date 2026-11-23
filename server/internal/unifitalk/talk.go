package unifitalk

import (
	"encoding/json"
	"github.com/rhisijmuiden/kiestoon/server/internal/pgdump"
	"slices"
	"strconv"
	"strings"
)

// The phone system as UniFi Talk stores it, read from its database. Users and group members are
// referred to by their ULP id (the UniFi identity), attendant steps by their switchboard item id.

// User is a Talk user: one extension with a SIP account.
type User struct {
	ULP         string
	Ext         string
	Name        string
	SIPPassword string
	Voicemail   bool
}

// Group is a Talk call group (ring group).
type Group struct {
	ID        string
	Name      string
	Ext       string
	Strategy  string   // simultaneous | sequential
	Members   []string // ULP ids in ring order
	NoAnswer  string   // voicemail | transfer | "" (hang up)
	VMUsers   []string // ULP ids whose voicemail gets the calls
	TransferU string   // ULP id on no answer
}

// Queue is a Talk call queue.
type Queue struct {
	ID        string
	Name      string
	Ext       string
	Strategy  string
	MaxWait   int
	RingTime  int
	WrapUp    int
	MaxSize   int
	Agents    []string // ULP ids
	VMUsers   []string // voicemail on overflow
	Overflow  string   // Talk action, e.g. play_message_and_send_to_voicemail, drop_call
	Messages  []string // audio files of the queue (below talk_audio/)
	Enabled   bool
	MusicName string
}

// Trunk is a Talk SIP gateway.
type Trunk struct {
	ID        string
	Name      string
	Host      string
	Username  string
	Password  string
	Register  bool
	Transport string
	Networks  []string
	Numbers   []string
}

// Target is where a call goes: a user, a group, a queue, another attendant, an external number or
// voicemail.
type Target struct {
	Kind string // user | group | queue | attendant | external | voicemail | hangup
	Ref  string // ULP id, group id, queue id, attendant item id or the number
}

// Step is one item of an attendant (switchboard): the attendant itself (root), a menu option
// (ivr, with a key) or an announcement (audio). Children with a key form a menu.
type Step struct {
	ID       int
	Type     string // root | ivr | audio
	Key      string // "" when the step is not a menu option
	Title    string
	Audio    string // recording below talk_audio/, "" when none
	Greeting string // the text of a text-to-speech greeting (when Audio is empty)
	Ext      string
	Numbers  []string
	// Target is where the call goes after the greeting (when the step is not a menu).
	Target *Target
	// NoAction is where the call goes when the caller presses nothing (menus).
	NoAction *Target
	Children []*Step
	Disabled bool
}

// Talk is the phone system read from a backup.
type Talk struct {
	Users      []User
	Groups     []Group
	Queues     []Queue
	Trunks     []Trunk
	Attendants []*Step
	// Numbers maps every phone number to what it rings first (attendant, group, queue or user).
	Numbers map[string]Target
	// GlobalGreeting is the voicemail greeting for everyone, "" when none.
	GlobalGreeting string
	// Devices is the number of phones in Talk (not imported: Kiestoon provisions its own).
	Devices int
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

// Parse reads the phone system from the Talk database of an archive.
func (a *Archive) Parse() *Talk {
	d := a.Dump
	t := &Talk{Numbers: map[string]Target{}}
	if a.Config != nil {
		if s, ok := a.Config["voicemail_greeting_filename"].(string); ok && s != "" {
			if _, ok := a.Audio["voicemail_greetings/"+s]; ok {
				t.GlobalGreeting = "voicemail_greetings/" + s
			}
		}
	}
	if t.GlobalGreeting == "" {
		if _, ok := a.Audio["voicemail_greetings/global_greeting.mp3"]; ok {
			t.GlobalGreeting = "voicemail_greetings/global_greeting.mp3"
		}
	}
	t.Devices = len(d.Table("public", "device").Rows)

	// Users.
	for _, r := range d.Table("public", "user").Rows {
		if r["ext"] == "" || r["ulp_id"] == "" {
			continue
		}
		name := strings.TrimSpace(r["first_name"] + " " + r["last_name"])
		if name == "" {
			name = r["ext"]
		}
		t.Users = append(t.Users, User{ULP: r["ulp_id"], Ext: r["ext"], Name: name, SIPPassword: r["sip_password"], Voicemail: r["voicemail_enabled"] != "f"})
	}
	slices.SortFunc(t.Users, func(x, y User) int { return strings.Compare(x.Ext, y.Ext) })
	for _, r := range d.Table("public", "user_did").Rows {
		if u := t.userByID(d, r["user"]); u != "" && r["direction"] != "outbound" {
			t.Numbers[r["did"]] = Target{Kind: "user", Ref: u}
		}
	}

	// Groups.
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
		g := Group{ID: r["id"], Name: r["name"], Ext: exts[r["id"]], Strategy: "simultaneous", NoAnswer: r["no_answer_action"],
			TransferU: r["transfer_to_user_uuid"]}
		if r["call_handling"] == "sequential" {
			g.Strategy = "sequential"
		}
		var vm []string
		_ = json.Unmarshal([]byte(r["vm_receiver_uuids"]), &vm)
		g.VMUsers = vm
		ms := members[r["id"]]
		slices.SortStableFunc(ms, func(x, y member) int { return x.order - y.order })
		for _, m := range ms {
			if !slices.Contains(g.Members, m.ulp) {
				g.Members = append(g.Members, m.ulp)
			}
		}
		t.Groups = append(t.Groups, g)
	}
	for _, r := range d.Table("public", "group_did").Rows {
		t.Numbers[r["did"]] = Target{Kind: "group", Ref: r["group"]}
	}

	// Queues.
	agents := map[string][]string{}
	tiers := d.Table("fsw", "tiers").Rows
	slices.SortStableFunc(tiers, func(x, y map[string]string) int { return atoi(x["position"]) - atoi(y["position"]) })
	for _, r := range tiers {
		q, ulp, ok := strings.Cut(r["agent"], "_")
		if ok && q == r["queue"] {
			agents[q] = append(agents[q], ulp)
		}
	}
	for _, r := range d.Table("public", "call_queue").Rows {
		q := Queue{ID: r["id"], Name: r["name"], Ext: r["ext"], Strategy: r["strategy"], MaxWait: atoi(r["max_wait_time"]),
			RingTime: atoi(r["agent_ring_time"]), WrapUp: atoi(r["agent_wrap_up_time"]), MaxSize: atoi(r["max_queue_size"]),
			Agents: agents[r["id"]], Enabled: r["enabled"] != "f"}
		opts := jsonMap(r["options"])
		if vm, ok := opts["voicemail"].(map[string]any); ok {
			q.VMUsers = strs(vm["user_ids"])
		}
		tm := jsonMap(r["timeouts"])
		q.Overflow = str(tm, "action")
		if q.Overflow == "" {
			q.Overflow = str(jsonMap(r["overflow"]), "action")
		}
		q.MusicName = str(jsonMap(r["wait_music_type"]), "title")
		prefix := "call_center/queue_" + q.ID + "_"
		for name := range a.Audio {
			if strings.HasPrefix(name, prefix) {
				q.Messages = append(q.Messages, name)
			}
		}
		slices.Sort(q.Messages)
		t.Queues = append(t.Queues, q)
	}
	for _, r := range d.Table("public", "call_queue_did").Rows {
		t.Numbers[r["did"]] = Target{Kind: "queue", Ref: r["queue"]}
	}

	// Trunks.
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
		tr := Trunk{ID: r["id"], Name: str(m, "name"), Host: str(p, "proxy"), Username: str(p, "username"), Password: str(p, "password"),
			Register: str(p, "register") == "true", Transport: strings.ToLower(str(p, "register-transport")),
			Networks: strs(m["acl_ip_cidr_list"]), Numbers: gwNumbers[r["id"]]}
		if tr.Host == "" {
			tr.Host = str(p, "realm")
		}
		if tr.Name == "" {
			tr.Name = tr.Host
		}
		slices.Sort(tr.Numbers)
		t.Trunks = append(t.Trunks, tr)
	}

	t.parseAttendants(d, a)
	return t
}

func (t *Talk) userByID(d *pgdump.Dump, id string) string {
	for _, r := range d.Table("public", "user").Rows {
		if r["id"] == id {
			return r["ulp_id"]
		}
	}
	return ""
}

// noAction reads the "when nothing is pressed" target of an attendant item.
func noAction(m map[string]any) *Target {
	for _, c := range []struct{ key, kind string }{
		{"noaction_transfer_to_user_uuid", "user"},
		{"noaction_transfer_to_group_id", "group"},
		{"noaction_transfer_to_queue_id", "queue"},
		{"noaction_transfer_to_node_id", "attendant"},
		{"noaction_transfer_to_contact_did", "external"},
	} {
		if v := str(m, c.key); v != "" {
			if c.kind == "attendant" {
				v = strings.TrimPrefix(v, "swb_")
			}
			return &Target{Kind: c.kind, Ref: v}
		}
	}
	if vm := strs(m["noaction_vm_receiver_uuids"]); len(vm) > 0 {
		return &Target{Kind: "voicemail", Ref: vm[0]}
	}
	if str(m, "sa_menu_noaction") == "drop_call" {
		return &Target{Kind: "hangup"}
	}
	return nil
}

func (t *Talk) parseAttendants(d *pgdump.Dump, a *Archive) {
	steps := map[int]*Step{}
	parent := map[int]int{}
	var order []int
	for _, r := range d.Table("public", "swb_item").Rows {
		id := atoi(r["id"])
		m := jsonMap(r["data"])
		s := &Step{ID: id, Type: str(m, "type"), Title: strings.TrimSpace(str(m, "title")), Ext: str(m, "ext"),
			Numbers: strs(m["numbers"]), Disabled: str(m, "disabled") == "true", NoAction: noAction(m)}
		if k := str(m, "key"); k != "" {
			s.Key = k
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
			if kind == "external" && r["target_did"] != "" {
				ref = r["target_did"]
			}
			s.Target = &Target{Kind: kind, Ref: ref}
		}
	}
	set("swb_item_ulp_user", "ulp_user", "user")
	set("swb_item_group", "group_id", "group")
	set("swb_item_queue", "queue_id", "queue")
	set("swb_item_sa_transfer", "transfer_target_id", "attendant")
	set("swb_item_contact", "contact_uuid", "external")
	for _, id := range order {
		s := steps[id]
		if p, ok := parent[id]; ok {
			if ps := steps[p]; ps != nil {
				ps.Children = append(ps.Children, s)
				continue
			}
		}
		if s.Type == "root" {
			t.Attendants = append(t.Attendants, s)
			for _, n := range s.Numbers {
				t.Numbers[n] = Target{Kind: "attendant", Ref: strconv.Itoa(s.ID)}
			}
		}
	}
	for _, s := range steps {
		slices.SortStableFunc(s.Children, func(x, y *Step) int { return keyRank(x.Key) - keyRank(y.Key) })
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

// User returns the user with the ULP id, or nil.
func (t *Talk) User(ulp string) *User {
	for i := range t.Users {
		if t.Users[i].ULP == ulp {
			return &t.Users[i]
		}
	}
	return nil
}

// Attendant returns the attendant (root step) with the id, or nil.
func (t *Talk) Attendant(id string) *Step {
	for _, s := range t.Attendants {
		if strconv.Itoa(s.ID) == id {
			return s
		}
	}
	return nil
}
