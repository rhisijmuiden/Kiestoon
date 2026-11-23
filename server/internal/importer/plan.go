package importer

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// The import runs in two steps. Prepare compares the Bundle with the current Kiestoon
// configuration and proposes what to create: a free number for every extension, group and queue,
// the trunk with the matching provider template, a route for every phone number and a voice menu
// for every menu. Things that already exist in Kiestoon (by name) are reused, so running the
// import twice does not create duplicates. Apply creates what the administrator ticked, with the
// numbers they chose. The recordings are converted to telephone audio by the browser (like every
// other upload) and stored as prompts before Apply; Apply gets their prompt ids.
//
// Plan keys name the things of the plan: "u:<ref>" a user, "v:<ref>" the voicemail of a user,
// "g:" a ring group, "q:" a queue, "t:" a trunk, "m:" a menu, "c:" a time condition,
// "x:<number>" an external number, "n:<number>" a phone number and "" hang up.

// PlanExtension is a user that becomes an extension.
type PlanExtension struct {
	Key          string `json:"key"`
	SourceNumber string `json:"sourceNumber"`
	Name         string `json:"name"`
	Email        string `json:"email,omitempty"`
	Number       string `json:"number"`
	Voicemail    bool   `json:"voicemail"`
	// KeepPassword: the SIP password is kept, so phones only need the new server and number.
	KeepPassword bool `json:"keepPassword"`
	// ExistingID is set when Kiestoon already has this extension (same name): it is reused.
	ExistingID int64 `json:"existingId,omitempty"`
}

// PlanGroup is a ring group.
type PlanGroup struct {
	Key          string   `json:"key"`
	SourceNumber string   `json:"sourceNumber"`
	Name         string   `json:"name"`
	Number       string   `json:"number"`
	Strategy     string   `json:"strategy"`
	Members      []string `json:"members"` // extension keys and external numbers (x:)
	Failover     string   `json:"failover"`
	ExistingID   int64    `json:"existingId,omitempty"`
}

// PlanQueue is a call queue.
type PlanQueue struct {
	Key          string   `json:"key"`
	SourceNumber string   `json:"sourceNumber"`
	Name         string   `json:"name"`
	Number       string   `json:"number"`
	Strategy     string   `json:"strategy"`
	MaxWait      int      `json:"maxWait"`
	Agents       []string `json:"agents"` // extension keys
	Overflow     string   `json:"overflow"`
	Audio        []string `json:"audio"`
	ExistingID   int64    `json:"existingId,omitempty"`
}

// PlanTrunk is a SIP trunk.
type PlanTrunk struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"` // Kiestoon provider template
	Host       string   `json:"host"`
	Username   string   `json:"username"`
	Register   bool     `json:"register"`
	HasSecret  bool     `json:"hasSecret"`
	Numbers    []string `json:"numbers"`
	ExistingID int64    `json:"existingId,omitempty"`
}

// PlanTime is a time condition.
type PlanTime struct {
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Schedule   []pbx.TimeRange `json:"schedule"`
	Open       string          `json:"open"`
	Closed     string          `json:"closed"`
	ExistingID int64           `json:"existingId,omitempty"`
}

// PlanNumber is a phone number with where it goes.
type PlanNumber struct {
	Key    string `json:"key"`
	Number string `json:"number"`
	Target string `json:"target"` // key of the extension, group, queue, menu or time condition
	// Exists is set when Kiestoon already routes the number: it is left alone.
	Exists bool `json:"exists"`
}

// PlanStep is a menu step for the preview.
type PlanStep struct {
	ID       int         `json:"id"`
	Type     string      `json:"type"`
	Key      string      `json:"key,omitempty"`
	Title    string      `json:"title,omitempty"`
	Audio    string      `json:"audio,omitempty"`
	Greeting string      `json:"greeting,omitempty"`
	Target   string      `json:"target,omitempty"` // key of the target
	NoAction string      `json:"noAction,omitempty"`
	Children []*PlanStep `json:"children,omitempty"`
}

// PlanMenu is a voice menu.
type PlanMenu struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	SourceNumber string    `json:"sourceNumber"`
	Numbers      []string  `json:"numbers"`
	Steps        *PlanStep `json:"steps"`
	ExistingID   int64     `json:"existingId,omitempty"`
}

// PlanAudio is a recording from the backup.
type PlanAudio struct {
	File   string   `json:"file"`
	Name   string   `json:"name"` // proposed prompt name
	Bytes  int      `json:"bytes"`
	UsedBy []string `json:"usedBy"`
	// Playable is false for formats the browser cannot convert (GSM, G.722): they are skipped.
	Playable bool `json:"playable"`
}

// Plan is the proposal shown before the import.
type Plan struct {
	Source         Info            `json:"source"`
	Digits         int             `json:"digits"`
	Extensions     []PlanExtension `json:"extensions"`
	RingGroups     []PlanGroup     `json:"ringGroups"`
	Queues         []PlanQueue     `json:"queues"`
	Trunks         []PlanTrunk     `json:"trunks"`
	TimeConditions []PlanTime      `json:"timeConditions"`
	Numbers        []PlanNumber    `json:"numbers"`
	Menus          []PlanMenu      `json:"menus"`
	Audio          []PlanAudio     `json:"audio"`
	Notes          []Note          `json:"notes"`
}

func keyUser(ref string) string      { return "u:" + ref }
func keyVoicemail(ref string) string { return "v:" + ref }
func keyGroup(ref string) string     { return "g:" + ref }
func keyQueue(ref string) string     { return "q:" + ref }
func keyTrunk(ref string) string     { return "t:" + ref }
func keyMenu(ref string) string      { return "m:" + ref }
func keyTime(ref string) string      { return "c:" + ref }
func keyNumber(did string) string    { return "n:" + did }
func keyExternal(n string) string    { return "x:" + n }

// targetKey is the plan key of a target ("" for hang up).
func (b *Bundle) targetKey(g *Target) string {
	if g == nil {
		return ""
	}
	switch g.Kind {
	case TargetUser:
		return keyUser(g.Ref)
	case TargetVoicemail:
		return keyVoicemail(g.Ref)
	case TargetGroup:
		return keyGroup(g.Ref)
	case TargetQueue:
		return keyQueue(g.Ref)
	case TargetMenu:
		return keyMenu(b.finalMenu(g.Ref))
	case TargetTime:
		return keyTime(g.Ref)
	case TargetExternal:
		return keyExternal(g.Ref)
	}
	return ""
}

// passThrough reports whether a menu only forwards to another menu: no recording, no keys, no
// numbers of its own (UniFi Talk users build "press 9 to hear the menu again" this way).
func passThrough(s *Step) bool {
	return s != nil && s.Audio == "" && s.Greeting == "" && len(s.Numbers) == 0 && s.Target != nil && s.Target.Kind == TargetMenu &&
		!slices.ContainsFunc(s.Children, func(c *Step) bool { return c.Key != "" })
}

// finalMenu follows menus that only forward to another one.
func (b *Bundle) finalMenu(ref string) string {
	for range 8 {
		s := b.Menu(ref)
		if !passThrough(s) {
			return ref
		}
		ref = s.Target.Ref
	}
	return ref
}

// numberAllocator hands out free numbers in a range of the number plan.
type numberAllocator struct {
	plan  pbx.NumberPlan
	taken map[string]bool
}

func (n *numberAllocator) inRange(purpose, num string) bool { return n.plan.InRange(purpose, num) }

// next returns want when it fits and is free, else the first free number of the range.
func (n *numberAllocator) next(purpose, want string) string {
	if want != "" && n.inRange(purpose, want) && !n.taken[want] && !slices.Contains(pbx.Reserved, want) {
		n.taken[want] = true
		return want
	}
	for _, r := range n.plan.Ranges {
		if r.Purpose != purpose {
			continue
		}
		from, _ := strconv.Atoi(r.From)
		to, _ := strconv.Atoi(r.To)
		if purpose == "extensions" {
			from++ // x01 rather than x00, like most PBXs
		}
		for i := from; i <= to; i++ {
			s := fmt.Sprintf("%0*d", n.plan.Digits, i)
			if !n.taken[s] && !slices.Contains(pbx.Reserved, s) {
				n.taken[s] = true
				return s
			}
		}
	}
	return ""
}

// existing holds what Kiestoon already has.
type existing struct {
	exts    []pbx.Extension
	groups  []pbx.RingGroup
	queues  []pbx.Queue
	ivrs    []pbx.IVR
	trunks  []pbx.Trunk
	times   []pbx.TimeCondition
	inbound []pbx.InboundRoute
}

func loadExisting(ctx context.Context, r *pbx.Repo) (*existing, error) {
	var e existing
	var err error
	if e.exts, err = r.Extensions(ctx); err != nil {
		return nil, err
	}
	if e.groups, err = r.RingGroups(ctx); err != nil {
		return nil, err
	}
	if e.queues, err = r.Queues(ctx); err != nil {
		return nil, err
	}
	if e.ivrs, err = r.IVRs(ctx); err != nil {
		return nil, err
	}
	if e.trunks, err = r.Trunks(ctx); err != nil {
		return nil, err
	}
	if e.times, err = r.TimeConditions(ctx); err != nil {
		return nil, err
	}
	if e.inbound, err = r.InboundRoutes(ctx); err != nil {
		return nil, err
	}
	return &e, nil
}

func sameName(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

// takenNumbers are the internal numbers in use in Kiestoon.
func (e *existing) takenNumbers(ctx context.Context, r *pbx.Repo) map[string]bool {
	taken := map[string]bool{}
	for _, x := range e.exts {
		taken[x.Number] = true
	}
	for _, x := range e.groups {
		taken[x.Number] = true
	}
	for _, x := range e.queues {
		taken[x.Number] = true
	}
	if cs, err := r.Conferences(ctx); err == nil {
		for _, c := range cs {
			taken[c.Number] = true
		}
	}
	return taken
}

// TrunkProvider picks the Kiestoon provider template for a trunk host ("generic" when none fits).
func TrunkProvider(host string, register bool) string {
	h := strings.ToLower(strings.TrimSpace(host))
	switch {
	case strings.Contains(h, "voys.nl"):
		return "voys"
	case strings.Contains(h, "cheapconnect"):
		return "cheapconnect"
	case strings.Contains(h, "speakup"):
		return "speakup"
	case strings.Contains(h, "sipnl.net"):
		return "ziggo"
	}
	for _, p := range pbx.TrunkPresets {
		if p.Host != "" && h != "" && (h == strings.ToLower(p.Host) || strings.HasSuffix(h, "."+strings.ToLower(p.Host))) {
			return p.ID
		}
	}
	if !register {
		return "generic-ip"
	}
	return "generic"
}

// PromptName joins the parts to a prompt name of at most 60 characters.
func PromptName(parts ...string) string {
	var p []string
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			p = append(p, s)
		}
	}
	name := strings.Join(p, " - ")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	return name
}

// SystemNames are the short names used in prompt and route names.
var SystemNames = map[string]string{
	"unifi-talk": "Talk", "asterisk": "Asterisk", "freepbx": "FreePBX", "3cx": "3CX", "freeswitch": "FreeSWITCH",
	"fusionpbx": "FusionPBX", "issabel": "Issabel", "csv": "CSV",
}

func (b *Bundle) shortName() string {
	if n := SystemNames[b.Info.System]; n != "" {
		return n
	}
	return "Import"
}

var reQueueFile = regexp.MustCompile(`^(?i)queue_?[0-9]+(_[0-9]+)*_|_file$`)

// Prepare builds the plan.
func Prepare(ctx context.Context, r *pbx.Repo, np pbx.NumberPlan, b *Bundle) (*Plan, error) {
	ex, err := loadExisting(ctx, r)
	if err != nil {
		return nil, err
	}
	sys := b.shortName()
	alloc := &numberAllocator{plan: np, taken: ex.takenNumbers(ctx, r)}
	p := &Plan{Source: b.Info, Digits: np.Digits, Extensions: []PlanExtension{}, RingGroups: []PlanGroup{}, Queues: []PlanQueue{},
		Trunks: []PlanTrunk{}, TimeConditions: []PlanTime{}, Numbers: []PlanNumber{}, Menus: []PlanMenu{}, Audio: []PlanAudio{},
		Notes: slices.Clone(b.Notes)}
	if p.Notes == nil {
		p.Notes = []Note{}
	}

	for _, u := range b.Users {
		pe := PlanExtension{Key: keyUser(u.Ref), SourceNumber: u.Ext, Name: u.Name, Email: u.Email, Voicemail: u.Voicemail,
			KeepPassword: SecretOK(u.SIPPassword)}
		if pe.Name == "" {
			pe.Name = u.Ext
		}
		for _, x := range ex.exts {
			if sameName(x.Name, pe.Name) {
				pe.ExistingID, pe.Number = x.ID, x.Number
				break
			}
		}
		if pe.ExistingID == 0 {
			pe.Number = alloc.next("extensions", u.Ext)
		}
		p.Extensions = append(p.Extensions, pe)
	}
	for _, g := range b.Groups {
		pg := PlanGroup{Key: keyGroup(g.Ref), SourceNumber: g.Ext, Name: g.Name, Strategy: g.Strategy, Members: []string{},
			Failover: b.targetKey(g.Failover)}
		if pg.Name == "" {
			pg.Name = "Group " + g.Ext
		}
		for _, m := range g.Members {
			if b.User(m) != nil && !slices.Contains(pg.Members, keyUser(m)) {
				pg.Members = append(pg.Members, keyUser(m))
			}
		}
		for _, x := range g.External {
			if d := pbx.NormalizeDID(x); d != "" {
				pg.Members = append(pg.Members, keyExternal(d))
			}
		}
		for _, x := range ex.groups {
			if sameName(x.Name, pg.Name) {
				pg.ExistingID, pg.Number = x.ID, x.Number
				break
			}
		}
		if pg.ExistingID == 0 {
			pg.Number = alloc.next("ringGroups", g.Ext)
		}
		p.RingGroups = append(p.RingGroups, pg)
	}
	audio := map[string]*PlanAudio{}
	use := func(file, name, by string) {
		if file == "" {
			return
		}
		data, ok := b.Audio[file]
		if !ok {
			return
		}
		pa := audio[file]
		if pa == nil {
			pa = &PlanAudio{File: file, Name: name, Bytes: len(data), UsedBy: []string{}, Playable: AudioPlayable(data)}
			audio[file] = pa
		}
		if !slices.Contains(pa.UsedBy, by) {
			pa.UsedBy = append(pa.UsedBy, by)
		}
	}
	for _, q := range b.Queues {
		pq := PlanQueue{Key: keyQueue(q.Ref), SourceNumber: q.Ext, Name: q.Name, Strategy: q.Strategy, MaxWait: q.MaxWait, Agents: []string{},
			Overflow: b.targetKey(q.Overflow), Audio: []string{}}
		if pq.Name == "" {
			pq.Name = "Queue " + q.Ext
		}
		if !slices.Contains([]string{"ringall", "leastrecent", "fewestcalls", "random", "rrmemory", "linear", "wrandom"}, pq.Strategy) {
			pq.Strategy = "rrmemory"
		}
		for _, ag := range q.Agents {
			if b.User(ag) != nil && !slices.Contains(pq.Agents, keyUser(ag)) {
				pq.Agents = append(pq.Agents, keyUser(ag))
			}
		}
		for _, x := range ex.queues {
			if sameName(x.Name, pq.Name) {
				pq.ExistingID, pq.Number = x.ID, x.Number
				break
			}
		}
		if pq.ExistingID == 0 {
			pq.Number = alloc.next("queues", q.Ext)
		}
		if q.Greeting != "" {
			pq.Audio = append(pq.Audio, q.Greeting)
			use(q.Greeting, PromptName(sys, pq.Name, "greeting"), pq.Key)
		}
		for _, f := range q.Messages {
			pq.Audio = append(pq.Audio, f)
			kind := strings.TrimSuffix(pathBase(f), pathExt(f))
			// UniFi Talk names them queue_<id>_<n>_timeouts_message_file.mp3: keep "timeouts message".
			kind = reQueueFile.ReplaceAllString(kind, "")
			use(f, PromptName(sys, pq.Name, strings.TrimSpace(strings.ReplaceAll(kind, "_", " "))), pq.Key)
		}
		if len(q.Messages) > 0 {
			p.Notes = append(p.Notes, NewNote("queue_messages", "name", pq.Name))
		}
		p.Queues = append(p.Queues, pq)
	}
	usedNumbers := map[string]bool{}
	for _, x := range ex.trunks {
		for _, n := range x.Numbers {
			usedNumbers[pbx.NormalizeDID(n)] = true
		}
	}
	for _, tr := range b.Trunks {
		pt := PlanTrunk{Key: keyTrunk(tr.Ref), Name: tr.Name, Provider: TrunkProvider(tr.Host, tr.Register), Host: tr.Host, Username: tr.Username,
			Register: tr.Register, HasSecret: tr.Password != "", Numbers: []string{}}
		if pt.Name == "" {
			pt.Name = PromptName(sys, tr.Host)
		}
		for _, x := range ex.trunks {
			if sameName(x.Name, pt.Name) {
				pt.ExistingID = x.ID
				break
			}
		}
		for _, n := range tr.Numbers {
			if d := pbx.NormalizeDID(n); d != "" && !usedNumbers[d] && !slices.Contains(pt.Numbers, d) {
				pt.Numbers = append(pt.Numbers, d)
			}
		}
		p.Trunks = append(p.Trunks, pt)
	}
	for _, tc := range b.TimeConditions {
		pt := PlanTime{Key: keyTime(tc.Ref), Name: tc.Name, Schedule: tc.Schedule, Open: b.targetKey(tc.Open), Closed: b.targetKey(tc.Closed)}
		if pt.Schedule == nil {
			pt.Schedule = []pbx.TimeRange{}
		}
		if pt.Name == "" {
			pt.Name = PromptName(sys, "schedule", tc.Ref)
		}
		for _, x := range ex.times {
			if sameName(x.Name, pt.Name) {
				pt.ExistingID = x.ID
				break
			}
		}
		p.TimeConditions = append(p.TimeConditions, pt)
	}
	routed := map[string]bool{}
	for _, x := range ex.inbound {
		routed[pbx.NormalizeDID(x.DID)] = true
	}
	seenDID := map[string]bool{}
	for _, n := range b.Numbers {
		d := pbx.NormalizeDID(n.Number)
		if d == "" || seenDID[d] {
			continue
		}
		seenDID[d] = true
		p.Numbers = append(p.Numbers, PlanNumber{Key: keyNumber(d), Number: d, Target: b.targetKey(n.Target), Exists: routed[d]})
	}
	slices.SortFunc(p.Numbers, func(x, y PlanNumber) int { return strings.Compare(x.Number, y.Number) })
	for _, s := range b.Menus {
		if passThrough(s) && b.finalMenu(s.Ref) != s.Ref {
			continue // forwarding helper: the menus point straight at the menu it forwards to
		}
		name := s.Title
		if name == "" {
			name = "Menu " + s.Ref
		}
		pm := PlanMenu{Key: keyMenu(s.Ref), Name: name, SourceNumber: s.Ext, Numbers: []string{}}
		for _, n := range s.Numbers {
			if d := pbx.NormalizeDID(n); d != "" {
				pm.Numbers = append(pm.Numbers, d)
			}
		}
		for _, x := range ex.ivrs {
			if sameName(x.Name, name) {
				pm.ExistingID = x.ID
				break
			}
		}
		pm.Steps = b.planStep(s)
		var walk func(ps *PlanStep, path []string)
		// Prompt names follow the path of titles: "Talk - Main - Sales". The welcome of the menu
		// itself is "... - welcome"; a step without a title or key (an announcement) is "... - info".
		walk = func(ps *PlanStep, path []string) {
			here := path
			label := ps.Title
			switch {
			case ps.Type == "root":
				label = "welcome"
			case label == "" && ps.Key != "":
				label = ps.Key
			case label == "":
				label = "info"
			}
			if ps.Type != "root" && ps.Title != "" || ps.Type != "root" && ps.Key != "" {
				here = append(slices.Clone(path), label)
			}
			if ps.Audio != "" {
				own := here
				if ps.Type == "root" || (ps.Title == "" && ps.Key == "") {
					own = append(slices.Clone(here), label)
				}
				use(ps.Audio, PromptName(append([]string{sys}, own...)...), pm.Key)
			}
			for _, c := range ps.Children {
				walk(c, here[:len(here):len(here)])
			}
		}
		walk(pm.Steps, []string{name})
		p.Menus = append(p.Menus, pm)
	}
	if b.VoicemailGreeting != "" {
		use(b.VoicemailGreeting, PromptName(sys, "voicemail greeting"), "greeting")
		p.Notes = append(p.Notes, NewNote("greeting_as_prompt"))
	}
	for _, pa := range audio {
		p.Audio = append(p.Audio, *pa)
	}
	slices.SortFunc(p.Audio, func(x, y PlanAudio) int { return strings.Compare(x.Name, y.Name) })
	// Prompt names must be unique: number the duplicates.
	seen := map[string]int{}
	for i := range p.Audio {
		n := strings.ToLower(p.Audio[i].Name)
		seen[n]++
		if seen[n] > 1 {
			p.Audio[i].Name = PromptName(p.Audio[i].Name[:min(len(p.Audio[i].Name), 55)], strconv.Itoa(seen[n]))
		}
		if !p.Audio[i].Playable {
			p.Notes = append(p.Notes, NewNote("audio_format", "file", p.Audio[i].File))
		}
	}
	if len(b.Phones) > 0 {
		p.Notes = append(p.Notes, NewNote("phones_not_imported", "count", strconv.Itoa(len(b.Phones))))
	}
	return p, nil
}

func pathBase(f string) string {
	if i := strings.LastIndexAny(f, "/\\"); i >= 0 {
		return f[i+1:]
	}
	return f
}

func pathExt(f string) string {
	b := pathBase(f)
	if i := strings.LastIndex(b, "."); i > 0 {
		return b[i:]
	}
	return ""
}

func (b *Bundle) planStep(s *Step) *PlanStep {
	ps := &PlanStep{ID: s.ID, Type: s.Type, Key: s.Key, Title: s.Title, Audio: s.Audio, Greeting: s.Greeting,
		Target: b.targetKey(s.Target), NoAction: b.targetKey(s.NoAction)}
	for _, c := range s.Children {
		if !c.Disabled {
			ps.Children = append(ps.Children, b.planStep(c))
		}
	}
	return ps
}

// SecretOK reports whether a SIP password can be kept as it is.
func SecretOK(s string) bool {
	if len(s) < 8 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e || r == ';' || r == '\\' || r == '"' {
			return false
		}
	}
	return true
}
