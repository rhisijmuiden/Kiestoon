package unifitalk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// The import runs in two steps. Prepare compares the backup with the current Kiestoon
// configuration and proposes what to create: a free number for every extension, group and queue,
// the trunk with the matching provider template, a route for every phone number and a voice menu
// for every attendant. Things that already exist in Kiestoon (by name) are reused, so running the
// import twice does not create duplicates. Apply creates what the administrator ticked, with the
// numbers they chose. The recordings are converted to telephone audio by the browser (like every
// other upload) and stored as prompts before Apply; Apply gets their prompt ids.

// Note is a remark for the administrator, translated by the web app.
type Note struct {
	Code string            `json:"code"`
	Args map[string]string `json:"args,omitempty"`
}

// Source describes the backup.
type Source struct {
	Console     string `json:"console"`
	Firmware    string `json:"firmware"`
	TalkVersion string `json:"talkVersion"`
	CreatedAt   int64  `json:"createdAt"`
	Devices     int    `json:"devices"`
}

// PlanExtension is a Talk user that becomes an extension.
type PlanExtension struct {
	Key        string `json:"key"`
	TalkNumber string `json:"talkNumber"`
	Name       string `json:"name"`
	Number     string `json:"number"`
	Voicemail  bool   `json:"voicemail"`
	// KeepPassword: the SIP password from Talk is kept, so phones only need the new server and number.
	KeepPassword bool `json:"keepPassword"`
	// ExistingID is set when Kiestoon already has this extension (same name): it is reused.
	ExistingID int64 `json:"existingId,omitempty"`
}

// PlanGroup is a Talk call group that becomes a ring group.
type PlanGroup struct {
	Key        string   `json:"key"`
	TalkNumber string   `json:"talkNumber"`
	Name       string   `json:"name"`
	Number     string   `json:"number"`
	Strategy   string   `json:"strategy"`
	Members    []string `json:"members"` // extension keys
	Failover   string   `json:"failover"`
	ExistingID int64    `json:"existingId,omitempty"`
}

// PlanQueue is a Talk call queue.
type PlanQueue struct {
	Key        string   `json:"key"`
	TalkNumber string   `json:"talkNumber"`
	Name       string   `json:"name"`
	Number     string   `json:"number"`
	Strategy   string   `json:"strategy"`
	MaxWait    int      `json:"maxWait"`
	Agents     []string `json:"agents"` // extension keys
	Overflow   string   `json:"overflow"`
	Audio      []string `json:"audio"`
	ExistingID int64    `json:"existingId,omitempty"`
}

// PlanTrunk is a Talk SIP gateway.
type PlanTrunk struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"` // Kiestoon provider template
	Host       string   `json:"host"`
	Username   string   `json:"username"`
	HasSecret  bool     `json:"hasSecret"`
	Numbers    []string `json:"numbers"`
	ExistingID int64    `json:"existingId,omitempty"`
}

// PlanNumber is a phone number with where it goes.
type PlanNumber struct {
	Key    string `json:"key"`
	Number string `json:"number"`
	Target string `json:"target"` // key of the extension, group, queue or menu
	// Exists is set when Kiestoon already routes the number: it is left alone.
	Exists bool `json:"exists"`
}

// PlanStep is an attendant step for the preview.
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

// PlanMenu is a Talk attendant that becomes a voice menu.
type PlanMenu struct {
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	TalkNumber string    `json:"talkNumber"`
	Numbers    []string  `json:"numbers"`
	Steps      *PlanStep `json:"steps"`
	ExistingID int64     `json:"existingId,omitempty"`
}

// PlanAudio is a recording from the backup.
type PlanAudio struct {
	File   string   `json:"file"`
	Name   string   `json:"name"` // proposed prompt name
	Bytes  int      `json:"bytes"`
	UsedBy []string `json:"usedBy"`
}

// Plan is the proposal shown before the import.
type Plan struct {
	Source     Source          `json:"source"`
	Digits     int             `json:"digits"`
	Extensions []PlanExtension `json:"extensions"`
	RingGroups []PlanGroup     `json:"ringGroups"`
	Queues     []PlanQueue     `json:"queues"`
	Trunks     []PlanTrunk     `json:"trunks"`
	Numbers    []PlanNumber    `json:"numbers"`
	Menus      []PlanMenu      `json:"menus"`
	Audio      []PlanAudio     `json:"audio"`
	Notes      []Note          `json:"notes"`
}

func keyUser(ulp string) string    { return "u:" + ulp }
func keyGroup(id string) string    { return "g:" + id }
func keyQueue(id string) string    { return "q:" + id }
func keyTrunk(id string) string    { return "t:" + id }
func keyMenu(id string) string     { return "m:" + id }
func keyNumber(did string) string  { return "n:" + did }
func keyExternal(n string) string  { return "x:" + n }
func keyVoicemail(u string) string { return "v:" + u }

// targetKey is the plan key of a Talk target ("" for hang up).
func (t *Talk) targetKey(g *Target) string {
	if g == nil {
		return ""
	}
	switch g.Kind {
	case "user":
		return keyUser(g.Ref)
	case "group":
		return keyGroup(g.Ref)
	case "queue":
		return keyQueue(g.Ref)
	case "attendant":
		return keyMenu(t.finalAttendant(g.Ref))
	case "external":
		return keyExternal(g.Ref)
	case "voicemail":
		return keyVoicemail(g.Ref)
	}
	return ""
}

// passThrough reports whether an attendant only forwards to another attendant: no recording,
// no menu, no numbers of its own (Talk users build "press 9 to hear the menu again" this way).
func passThrough(s *Step) bool {
	return s != nil && s.Audio == "" && s.Greeting == "" && len(s.Numbers) == 0 && s.Target != nil && s.Target.Kind == "attendant" &&
		!slices.ContainsFunc(s.Children, func(c *Step) bool { return c.Key != "" })
}

// finalAttendant follows attendants that only forward to another one.
func (t *Talk) finalAttendant(id string) string {
	for range 8 {
		s := t.Attendant(id)
		if !passThrough(s) {
			return id
		}
		id = s.Target.Ref
	}
	return id
}

// numberAllocator hands out free numbers in a range of the number plan.
type numberAllocator struct {
	plan  pbx.NumberPlan
	taken map[string]bool
}

func (n *numberAllocator) inRange(purpose, num string) bool {
	if purpose == "extensions" {
		return len(num) == n.plan.Digits && n.plan.InRange("extensions", num)
	}
	return n.plan.InRange(purpose, num)
}

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

// Talk queue strategies (FreeSWITCH mod_callcenter) to Asterisk queue strategies.
var queueStrategies = map[string]string{
	"ring-all":                         "ringall",
	"longest-idle-agent":               "leastrecent",
	"round-robin":                      "rrmemory",
	"top-down":                         "linear",
	"agent-with-least-talk-time":       "fewestcalls",
	"agent-with-fewest-calls":          "fewestcalls",
	"sequentially-by-agent-order":      "linear",
	"sequentially-by-next-agent-order": "rrmemory",
	"random":                           "random",
	"ring-progressively":               "ringall",
}

// trunkProvider picks the Kiestoon provider template for a Talk gateway host.
func trunkProvider(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "voys.nl"):
		return "voys"
	case strings.Contains(h, "cheapconnect"):
		return "cheapconnect"
	case strings.Contains(h, "speakup"):
		return "speakup"
	case strings.Contains(h, "sipnl.net"):
		return "ziggo"
	case strings.Contains(h, "telnyx"):
		return "telnyx"
	}
	return "generic"
}

func promptName(parts ...string) string {
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

// Prepare builds the plan.
func Prepare(ctx context.Context, r *pbx.Repo, np pbx.NumberPlan, a *Archive, t *Talk) (*Plan, error) {
	ex, err := loadExisting(ctx, r)
	if err != nil {
		return nil, err
	}
	alloc := &numberAllocator{plan: np, taken: ex.takenNumbers(ctx, r)}
	p := &Plan{Digits: np.Digits, Extensions: []PlanExtension{}, RingGroups: []PlanGroup{}, Queues: []PlanQueue{}, Trunks: []PlanTrunk{},
		Numbers: []PlanNumber{}, Menus: []PlanMenu{}, Audio: []PlanAudio{}, Notes: []Note{},
		Source: Source{Console: a.Console, Firmware: a.Firmware, TalkVersion: a.TalkVersion, CreatedAt: a.CreatedAt, Devices: t.Devices}}

	for _, u := range t.Users {
		pe := PlanExtension{Key: keyUser(u.ULP), TalkNumber: u.Ext, Name: u.Name, Voicemail: u.Voicemail, KeepPassword: secretOK(u.SIPPassword)}
		for _, x := range ex.exts {
			if sameName(x.Name, u.Name) {
				pe.ExistingID, pe.Number = x.ID, x.Number
				break
			}
		}
		if pe.ExistingID == 0 {
			pe.Number = alloc.next("extensions", u.Ext)
		}
		p.Extensions = append(p.Extensions, pe)
	}
	failover := func(action string, vm []string, transfer string) string {
		switch {
		case strings.Contains(action, "voicemail") && len(vm) > 0:
			return keyVoicemail(vm[0])
		case transfer != "":
			return keyUser(transfer)
		}
		return ""
	}
	for _, g := range t.Groups {
		pg := PlanGroup{Key: keyGroup(g.ID), TalkNumber: g.Ext, Name: g.Name, Strategy: g.Strategy, Members: []string{},
			Failover: failover(g.NoAnswer, g.VMUsers, g.TransferU)}
		for _, m := range g.Members {
			if t.User(m) != nil {
				pg.Members = append(pg.Members, keyUser(m))
			}
		}
		for _, x := range ex.groups {
			if sameName(x.Name, g.Name) {
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
		b, ok := a.Audio[file]
		if !ok {
			return
		}
		pa := audio[file]
		if pa == nil {
			pa = &PlanAudio{File: file, Name: name, Bytes: len(b), UsedBy: []string{}}
			audio[file] = pa
		}
		if !slices.Contains(pa.UsedBy, by) {
			pa.UsedBy = append(pa.UsedBy, by)
		}
	}
	for _, q := range t.Queues {
		strategy := queueStrategies[q.Strategy]
		if strategy == "" {
			strategy = "rrmemory"
		}
		pq := PlanQueue{Key: keyQueue(q.ID), TalkNumber: q.Ext, Name: q.Name, Strategy: strategy, MaxWait: q.MaxWait, Agents: []string{},
			Overflow: failover(q.Overflow, q.VMUsers, ""), Audio: q.Messages}
		if pq.Audio == nil {
			pq.Audio = []string{}
		}
		for _, ag := range q.Agents {
			if t.User(ag) != nil {
				pq.Agents = append(pq.Agents, keyUser(ag))
			}
		}
		for _, x := range ex.queues {
			if sameName(x.Name, q.Name) {
				pq.ExistingID, pq.Number = x.ID, x.Number
				break
			}
		}
		if pq.ExistingID == 0 {
			pq.Number = alloc.next("queues", q.Ext)
		}
		for _, f := range q.Messages {
			kind := strings.TrimSuffix(strings.TrimPrefix(f, "call_center/queue_"+q.ID+"_"), ".mp3")
			kind = strings.TrimSuffix(strings.TrimLeft(kind, "0123456789_"), "_file")
			use(f, promptName("Talk", q.Name, strings.ReplaceAll(kind, "_", " ")), pq.Key)
		}
		p.Queues = append(p.Queues, pq)
	}
	usedNumbers := map[string]bool{}
	for _, x := range ex.trunks {
		for _, n := range x.Numbers {
			usedNumbers[pbx.NormalizeDID(n)] = true
		}
	}
	for _, tr := range t.Trunks {
		pt := PlanTrunk{Key: keyTrunk(tr.ID), Name: tr.Name, Provider: trunkProvider(tr.Host), Host: tr.Host, Username: tr.Username,
			HasSecret: tr.Password != "", Numbers: []string{}}
		for _, x := range ex.trunks {
			if sameName(x.Name, tr.Name) {
				pt.ExistingID = x.ID
				break
			}
		}
		for _, n := range tr.Numbers {
			if d := pbx.NormalizeDID(n); d != "" && !usedNumbers[d] {
				pt.Numbers = append(pt.Numbers, d)
			}
		}
		p.Trunks = append(p.Trunks, pt)
	}
	routed := map[string]bool{}
	for _, x := range ex.inbound {
		routed[pbx.NormalizeDID(x.DID)] = true
	}
	var dids []string
	for n := range t.Numbers {
		dids = append(dids, n)
	}
	slices.Sort(dids)
	for _, n := range dids {
		g := t.Numbers[n]
		d := pbx.NormalizeDID(n)
		if d == "" {
			continue
		}
		p.Numbers = append(p.Numbers, PlanNumber{Key: keyNumber(d), Number: d, Target: t.targetKey(&g), Exists: routed[d]})
	}
	for _, s := range t.Attendants {
		if passThrough(s) && slices.ContainsFunc(t.Attendants, func(o *Step) bool { return o != s && strconv.Itoa(o.ID) == t.finalAttendant(strconv.Itoa(s.ID)) }) {
			continue // forwarding helper: the menus point straight at the attendant it forwards to
		}
		name := s.Title
		if name == "" {
			name = "Attendant " + strconv.Itoa(s.ID)
		}
		pm := PlanMenu{Key: keyMenu(strconv.Itoa(s.ID)), Name: name, TalkNumber: s.Ext, Numbers: []string{}}
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
		pm.Steps = t.planStep(s)
		var walk func(ps *PlanStep, path []string)
		walk = func(ps *PlanStep, path []string) {
			label := ps.Title
			if ps.Type == "root" {
				label = "welcome"
			} else if label == "" && ps.Key != "" {
				label = ps.Key
			}
			here := append(slices.Clone(path), label)
			use(ps.Audio, promptName(append([]string{"Talk"}, here...)...), pm.Key)
			for _, c := range ps.Children {
				walk(c, here[:len(here):len(here)])
			}
		}
		walk(pm.Steps, []string{name})
		p.Menus = append(p.Menus, pm)
	}
	if t.GlobalGreeting != "" {
		use(t.GlobalGreeting, "Talk - voicemail greeting", "greeting")
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
			p.Audio[i].Name = promptName(p.Audio[i].Name[:min(len(p.Audio[i].Name), 55)], strconv.Itoa(seen[n]))
		}
	}
	if t.Devices > 0 {
		p.Notes = append(p.Notes, Note{Code: "phones_not_imported", Args: map[string]string{"count": strconv.Itoa(t.Devices)}})
	}
	if t.GlobalGreeting != "" {
		p.Notes = append(p.Notes, Note{Code: "greeting_as_prompt"})
	}
	for _, q := range p.Queues {
		if len(q.Audio) > 0 {
			p.Notes = append(p.Notes, Note{Code: "queue_messages", Args: map[string]string{"name": q.Name}})
		}
	}
	return p, nil
}

func (t *Talk) planStep(s *Step) *PlanStep {
	ps := &PlanStep{ID: s.ID, Type: s.Type, Key: s.Key, Title: s.Title, Audio: s.Audio, Greeting: s.Greeting,
		Target: t.targetKey(s.Target), NoAction: t.targetKey(s.NoAction)}
	for _, c := range s.Children {
		if !c.Disabled {
			ps.Children = append(ps.Children, t.planStep(c))
		}
	}
	return ps
}

func secretOK(s string) bool {
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

// Choice is what the administrator ticked, with the numbers they chose.
type Choice struct {
	Extensions []string          `json:"extensions"`
	RingGroups []string          `json:"ringGroups"`
	Queues     []string          `json:"queues"`
	Trunks     []string          `json:"trunks"`
	Numbers    []string          `json:"numbers"`
	Menus      []string          `json:"menus"`
	NumberOf   map[string]string `json:"numberOf"` // key -> internal number
	Prompts    map[string]int64  `json:"prompts"`  // audio file -> prompt id
}

// Created is one thing the import made or reused.
type Created struct {
	Kind   string `json:"kind"` // extension | ringgroup | queue | trunk | route | ivr
	Key    string `json:"key"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Number string `json:"number,omitempty"`
	Reused bool   `json:"reused,omitempty"`
}

// Result is what the import did.
type Result struct {
	Created  []Created `json:"created"`
	Warnings []Note    `json:"warnings"`
}

// FieldError is a validation error of one planned item.
type FieldError struct {
	Key   string
	Field string
	Code  string
}

func (e *FieldError) Error() string { return e.Key + ": " + e.Code + " " + e.Field }

func wrap(key string, err error) error {
	var ve *pbx.ValidationError
	if errors.As(err, &ve) {
		return &FieldError{Key: key, Field: ve.Field, Code: ve.Code}
	}
	return err
}

type applier struct {
	ctx    context.Context
	r      *pbx.Repo
	np     pbx.NumberPlan
	t      *Talk
	p      *Plan
	c      Choice
	res    *Result
	ext    map[string]pbx.Extension // user key -> extension
	groups map[string]int64
	queues map[string]int64
	menus  map[string]int64
}

func (ap *applier) warn(code string, args ...string) {
	n := Note{Code: code, Args: map[string]string{}}
	for i := 0; i+1 < len(args); i += 2 {
		n.Args[args[i]] = args[i+1]
	}
	if len(n.Args) == 0 {
		n.Args = nil
	}
	ap.res.Warnings = append(ap.res.Warnings, n)
}

func (ap *applier) number(key, planned string) string {
	if n, ok := ap.c.NumberOf[key]; ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	return planned
}

// dest turns a plan target key into a Kiestoon destination.
func (ap *applier) dest(key string) pbx.Destination {
	kind, ref, _ := strings.Cut(key, ":")
	switch kind {
	case "u":
		if e, ok := ap.ext[key]; ok {
			return pbx.Destination{Type: pbx.DestExtension, Target: e.Number}
		}
	case "v":
		if e, ok := ap.ext[keyUser(ref)]; ok && e.Voicemail {
			return pbx.Destination{Type: pbx.DestVoicemail, Target: e.Number}
		}
	case "g":
		if id, ok := ap.groups[key]; ok {
			return pbx.Destination{Type: pbx.DestRingGroup, Target: strconv.FormatInt(id, 10)}
		}
	case "q":
		if id, ok := ap.queues[key]; ok {
			return pbx.Destination{Type: pbx.DestQueue, Target: strconv.FormatInt(id, 10)}
		}
	case "m":
		if id, ok := ap.menus[key]; ok {
			return pbx.Destination{Type: pbx.DestIVR, Target: strconv.FormatInt(id, 10)}
		}
	case "x":
		if d := pbx.NormalizeDID(ref); d != "" {
			return pbx.Destination{Type: pbx.DestExternal, Target: d}
		}
	case "":
		return pbx.Destination{Type: pbx.DestHangup}
	}
	ap.warn("target_missing", "target", key)
	return pbx.Destination{Type: pbx.DestHangup}
}

// Apply creates what was chosen. It stops at the first item Kiestoon does not accept; what was
// created before stays (it can be removed in the normal pages, or kept: a second import reuses it).
func Apply(ctx context.Context, r *pbx.Repo, np pbx.NumberPlan, t *Talk, p *Plan, c Choice) (*Result, error) {
	ap := &applier{ctx: ctx, r: r, np: np, t: t, p: p, c: c, res: &Result{Created: []Created{}, Warnings: []Note{}},
		ext: map[string]pbx.Extension{}, groups: map[string]int64{}, queues: map[string]int64{}, menus: map[string]int64{}}
	if err := ap.extensions(); err != nil {
		return ap.res, err
	}
	if err := ap.ringGroups(); err != nil {
		return ap.res, err
	}
	if err := ap.queuesStep(); err != nil {
		return ap.res, err
	}
	if err := ap.trunks(); err != nil {
		return ap.res, err
	}
	if err := ap.menusStep(); err != nil {
		return ap.res, err
	}
	if err := ap.routes(); err != nil {
		return ap.res, err
	}
	return ap.res, nil
}

func (ap *applier) extensions() error {
	for _, pe := range ap.p.Extensions {
		if pe.ExistingID != 0 {
			// Reused whether ticked or not, so the groups and menus can ring it.
			e, err := ap.r.Extension(ap.ctx, pe.ExistingID)
			if err == nil {
				ap.ext[pe.Key] = *e
				if slices.Contains(ap.c.Extensions, pe.Key) {
					ap.res.Created = append(ap.res.Created, Created{Kind: "extension", Key: pe.Key, ID: e.ID, Name: e.Name, Number: e.Number, Reused: true})
				}
			}
			continue
		}
		if !slices.Contains(ap.c.Extensions, pe.Key) {
			continue
		}
		u := ap.t.User(strings.TrimPrefix(pe.Key, "u:"))
		if u == nil {
			continue
		}
		e := pbx.Extension{Number: ap.number(pe.Key, pe.Number), Name: pe.Name, Voicemail: pe.Voicemail, Enabled: true,
			OutboundClass: "national", RecordMode: "off"}
		if pe.KeepPassword {
			e.SIPSecret = u.SIPPassword
		}
		if err := ap.r.SaveExtension(ap.ctx, ap.np, &e); err != nil {
			return wrap(pe.Key, err)
		}
		ap.ext[pe.Key] = e
		ap.res.Created = append(ap.res.Created, Created{Kind: "extension", Key: pe.Key, ID: e.ID, Name: e.Name, Number: e.Number})
	}
	return nil
}

func (ap *applier) members(keys []string, owner string) []string {
	var out []string
	for _, k := range keys {
		if e, ok := ap.ext[k]; ok {
			out = append(out, e.Number)
		} else {
			ap.warn("member_missing", "member", k, "owner", owner)
		}
	}
	return out
}

func (ap *applier) ringGroups() error {
	for _, pg := range ap.p.RingGroups {
		if pg.ExistingID != 0 {
			ap.groups[pg.Key] = pg.ExistingID
			if slices.Contains(ap.c.RingGroups, pg.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "ringgroup", Key: pg.Key, ID: pg.ExistingID, Name: pg.Name, Number: pg.Number, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.RingGroups, pg.Key) {
			continue
		}
		g := pbx.RingGroup{Number: ap.number(pg.Key, pg.Number), Name: pg.Name, Strategy: pg.Strategy, Members: ap.members(pg.Members, pg.Key),
			Enabled: true, Failover: pbx.Destination{Type: pbx.DestHangup}}
		if len(g.Members) == 0 {
			ap.warn("group_empty", "name", pg.Name)
			continue
		}
		if err := ap.r.SaveRingGroup(ap.ctx, ap.np, &g); err != nil {
			return wrap(pg.Key, err)
		}
		ap.groups[pg.Key] = g.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "ringgroup", Key: pg.Key, ID: g.ID, Name: g.Name, Number: g.Number})
	}
	// Failovers can point at each other: set them once every group exists.
	for _, pg := range ap.p.RingGroups {
		id, ok := ap.groups[pg.Key]
		if !ok || pg.ExistingID != 0 || pg.Failover == "" {
			continue
		}
		g, err := ap.r.RingGroup(ap.ctx, id)
		if err != nil {
			continue
		}
		g.Failover = ap.dest(pg.Failover)
		if err := ap.r.SaveRingGroup(ap.ctx, ap.np, g); err != nil {
			return wrap(pg.Key, err)
		}
	}
	return nil
}

func (ap *applier) queuesStep() error {
	for _, pq := range ap.p.Queues {
		if pq.ExistingID != 0 {
			ap.queues[pq.Key] = pq.ExistingID
			if slices.Contains(ap.c.Queues, pq.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "queue", Key: pq.Key, ID: pq.ExistingID, Name: pq.Name, Number: pq.Number, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.Queues, pq.Key) {
			continue
		}
		tq := ap.t.queue(strings.TrimPrefix(pq.Key, "q:"))
		q := pbx.Queue{Number: ap.number(pq.Key, pq.Number), Name: pq.Name, Strategy: pq.Strategy, Enabled: true, Members: []pbx.QueueMember{},
			MaxWait: min(max(pq.MaxWait, 0), 7200), Overflow: pbx.Destination{Type: pbx.DestHangup}}
		if tq != nil {
			q.AgentTimeout = min(max(tq.RingTime, 5), 120)
			q.Wrapup = min(max(tq.WrapUp, 0), 600)
			q.MaxCallers = min(max(tq.MaxSize, 0), 500)
			q.Enabled = tq.Enabled
		}
		for _, k := range pq.Agents {
			if e, ok := ap.ext[k]; ok {
				q.Members = append(q.Members, pbx.QueueMember{ExtensionID: e.ID})
			} else {
				ap.warn("member_missing", "member", k, "owner", pq.Key)
			}
		}
		if pq.Overflow != "" {
			q.Overflow = ap.dest(pq.Overflow)
		}
		if err := ap.r.SaveQueue(ap.ctx, ap.np, &q); err != nil {
			return wrap(pq.Key, err)
		}
		ap.queues[pq.Key] = q.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "queue", Key: pq.Key, ID: q.ID, Name: q.Name, Number: q.Number})
	}
	return nil
}

func (t *Talk) queue(id string) *Queue {
	for i := range t.Queues {
		if t.Queues[i].ID == id {
			return &t.Queues[i]
		}
	}
	return nil
}

func (t *Talk) trunk(id string) *Trunk {
	for i := range t.Trunks {
		if t.Trunks[i].ID == id {
			return &t.Trunks[i]
		}
	}
	return nil
}

func (ap *applier) trunks() error {
	for _, pt := range ap.p.Trunks {
		if !slices.Contains(ap.c.Trunks, pt.Key) {
			continue
		}
		if pt.ExistingID != 0 {
			ap.res.Created = append(ap.res.Created, Created{Kind: "trunk", Key: pt.Key, ID: pt.ExistingID, Name: pt.Name, Reused: true})
			continue
		}
		tt := ap.t.trunk(strings.TrimPrefix(pt.Key, "t:"))
		if tt == nil {
			continue
		}
		tr := pbx.Trunk{Name: pt.Name, Provider: pt.Provider, AuthMode: "register", Host: tt.Host, Port: 5060, Transport: tt.Transport,
			Username: tt.Username, Secret: tt.Password, DIDSource: "to", Codecs: "alaw,ulaw,g722", Enabled: true,
			MatchIPs: strings.Join(tt.Networks, ","), Numbers: pt.Numbers}
		if !tt.Register {
			tr.AuthMode = "ip"
		}
		for _, preset := range pbx.TrunkPresets {
			if preset.ID == pt.Provider && preset.ID != "generic" {
				tr.Host, tr.Port, tr.Transport, tr.DIDSource, tr.Codecs = preset.Host, preset.Port, preset.Transport, preset.DIDSource, preset.Codecs
				if preset.MatchIPs != "" {
					tr.MatchIPs = preset.MatchIPs
				}
			}
		}
		if tr.Transport != "tcp" && tr.Transport != "tls" {
			tr.Transport = "udp"
		}
		if err := ap.r.SaveTrunk(ap.ctx, &tr); err != nil {
			return wrap(pt.Key, err)
		}
		ap.res.Created = append(ap.res.Created, Created{Kind: "trunk", Key: pt.Key, ID: tr.ID, Name: tr.Name})
	}
	return nil
}

func (ap *applier) menusStep() error {
	// First every menu, empty, so menus can send calls to each other (and to themselves).
	for _, pm := range ap.p.Menus {
		if pm.ExistingID != 0 {
			ap.menus[pm.Key] = pm.ExistingID
			if slices.Contains(ap.c.Menus, pm.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "ivr", Key: pm.Key, ID: pm.ExistingID, Name: pm.Name, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.Menus, pm.Key) {
			continue
		}
		v := pbx.IVR{Name: pm.Name, Timeout: 5, Retries: 2, TimeoutDest: pbx.Destination{Type: pbx.DestHangup}}
		if err := ap.r.SaveIVR(ap.ctx, &v); err != nil {
			return wrap(pm.Key, err)
		}
		ap.menus[pm.Key] = v.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "ivr", Key: pm.Key, ID: v.ID, Name: v.Name})
	}
	for _, pm := range ap.p.Menus {
		if pm.ExistingID != 0 || !slices.Contains(ap.c.Menus, pm.Key) {
			continue
		}
		v, err := ap.r.IVR(ap.ctx, ap.menus[pm.Key])
		if err != nil {
			return err
		}
		v.Flow = ap.flow(pm)
		if err := ap.r.SaveIVR(ap.ctx, v); err != nil {
			return wrap(pm.Key, err)
		}
	}
	return nil
}

// flowBuilder lays out the attendant tree as a flow: one column per level, one row per leaf.
type flowBuilder struct {
	ap   *applier
	f    *pbx.Flow
	row  int
	seq  int
	self string
}

func (b *flowBuilder) id(prefix string) string {
	b.seq++
	return prefix + strconv.Itoa(b.seq)
}

func (b *flowBuilder) add(n pbx.FlowNode, col int) string {
	n.X = float64(col) * 320
	n.Y = float64(b.row) * 140
	b.f.Nodes = append(b.f.Nodes, n)
	return n.ID
}

func (b *flowBuilder) edge(from, port, to string) {
	if from != "" && to != "" {
		b.f.Edges = append(b.f.Edges, pbx.FlowEdge{From: from, Port: port, To: to})
	}
}

func (b *flowBuilder) prompt(file string) int64 {
	if file == "" {
		return 0
	}
	if id, ok := b.ap.c.Prompts[file]; ok {
		return id
	}
	b.ap.warn("audio_missing", "file", file)
	return 0
}

func (b *flowBuilder) destination(key string, col int) string {
	d := b.ap.dest(key)
	id := b.add(pbx.FlowNode{ID: b.id("d"), Type: pbx.NodeDestination, Destination: &d}, col)
	b.row++
	return id
}

func keyed(s *PlanStep) []*PlanStep {
	var out []*PlanStep
	for _, c := range s.Children {
		if c.Key != "" {
			out = append(out, c)
		}
	}
	return out
}

// menu connects the keys and the timeout of a menu node (the entry or a menu step).
func (b *flowBuilder) menu(from string, s *PlanStep, col int) {
	seen := map[string]bool{}
	for _, c := range keyed(s) {
		if seen[c.Key] || len(c.Key) != 1 || !strings.Contains("0123456789*#", c.Key) {
			continue
		}
		seen[c.Key] = true
		k := b.add(pbx.FlowNode{ID: b.id("k"), Type: pbx.NodeKey, Digit: c.Key, Label: c.Title}, col)
		b.edge(from, "out", k)
		b.edge(k, "out", b.enter(c, col+1))
	}
	if s.NoAction != "" {
		t := b.add(pbx.FlowNode{ID: b.id("t"), Type: pbx.NodeTimeout}, col)
		b.edge(from, "out", t)
		b.edge(t, "out", b.destination(s.NoAction, col+1))
	}
}

// enter returns the node a call reaches when it enters the step.
func (b *flowBuilder) enter(s *PlanStep, col int) string {
	if len(keyed(s)) > 0 {
		m := b.add(pbx.FlowNode{ID: b.id("m"), Type: pbx.NodeMenu, Label: s.Title, PromptID: b.prompt(s.Audio)}, col)
		b.menu(m, s, col+1)
		return m
	}
	var audio string
	next := col
	if p := b.prompt(s.Audio); p != 0 {
		audio = b.add(pbx.FlowNode{ID: b.id("a"), Type: pbx.NodeAudio, Label: s.Title, PromptID: p}, col)
		next = col + 1
	}
	var to string
	switch {
	case s.Target != "":
		to = b.destination(s.Target, next)
	case len(s.Children) > 0:
		to = b.enter(s.Children[0], next)
	case s.NoAction != "":
		to = b.destination(s.NoAction, next)
	default:
		to = b.destination("", next)
	}
	if audio == "" {
		return to
	}
	b.edge(audio, "out", to)
	return audio
}

func (ap *applier) flow(pm PlanMenu) *pbx.Flow {
	b := &flowBuilder{ap: ap, f: &pbx.Flow{Nodes: []pbx.FlowNode{}, Edges: []pbx.FlowEdge{}}, self: pm.Key}
	s := pm.Steps
	entry := b.add(pbx.FlowNode{ID: "entry", Type: pbx.NodeEntry, PromptID: b.prompt(s.Audio)}, 0)
	if len(keyed(s)) > 0 {
		b.menu(entry, s, 1)
	} else {
		var to string
		switch {
		case s.Target != "":
			to = b.destination(s.Target, 1)
		case len(s.Children) > 0:
			to = b.enter(s.Children[0], 1)
		default:
			to = b.destination(s.NoAction, 1)
		}
		b.edge(entry, "out", to)
	}
	// Centre the entry on the rows it leads to.
	if b.row > 1 {
		b.f.Nodes[0].Y = float64(b.row-1) * 70
	}
	return b.f
}

func (ap *applier) routes() error {
	for _, pn := range ap.p.Numbers {
		if pn.Exists || !slices.Contains(ap.c.Numbers, pn.Key) {
			continue
		}
		d := ap.dest(pn.Target)
		x := pbx.InboundRoute{Name: promptName("Talk", pn.Number), DID: pn.Number, DestinationType: d.Type, Destination: d.Target, Enabled: true}
		if err := ap.r.SaveInboundRoute(ap.ctx, &x); err != nil {
			return wrap(pn.Key, err)
		}
		ap.res.Created = append(ap.res.Created, Created{Kind: "route", Key: pn.Key, ID: x.ID, Name: x.Name, Number: x.DID})
	}
	return nil
}
