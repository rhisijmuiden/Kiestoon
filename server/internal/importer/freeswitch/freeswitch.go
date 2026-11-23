// Package freeswitch imports FreeSWITCH systems: a plain FreeSWITCH configuration directory
// (directory users and groups, dialplan, IVR menus, mod_callcenter queues, gateways) and
// FusionPBX, which keeps the same things in its PostgreSQL database.
package freeswitch

import (
	"context"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/importer/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func init() { importer.Register(Adapter{}) }

// Adapter reads FreeSWITCH configuration files and FusionPBX backups.
type Adapter struct{}

// ID implements importer.Adapter.
func (Adapter) ID() string { return "freeswitch" }

// Detect implements importer.Adapter.
func (Adapter) Detect(u *importer.Upload) int {
	score := 0
	for _, e := range u.Entries() {
		n := strings.ToLower(e.Name)
		switch {
		case strings.HasSuffix(n, "freeswitch.xml"), strings.HasSuffix(n, "/vars.xml") || n == "vars.xml":
			score = max(score, 85)
		case strings.Contains(n, "fusionpbx") && (strings.HasSuffix(n, ".sql") || strings.HasSuffix(n, ".backup") || strings.HasSuffix(n, ".dump")):
			score = max(score, 90)
		case strings.Contains(n, "/directory/") && strings.HasSuffix(n, ".xml"), strings.Contains(n, "/ivr_menus/"):
			score = max(score, 70)
		}
	}
	if !u.IsArchive() {
		n := strings.ToLower(u.Name)
		if strings.HasSuffix(n, ".xml") {
			return 40
		}
		if h := string(u.Head(8192)); strings.Contains(h, "PGDMP") && strings.Contains(n, "fusion") || strings.Contains(h, "v_extensions") {
			return 85
		}
	}
	return score
}

// Read implements importer.Adapter.
func (Adapter) Read(ctx context.Context, u *importer.Upload) (*importer.Bundle, error) {
	b := &importer.Bundle{Info: importer.Info{Product: "FreeSWITCH"}}
	sounds := asterisk.NewSounds(u.Entries())
	fusion, err := readFusion(u, b, sounds)
	if err != nil {
		return nil, err
	}
	if !fusion {
		files, err := u.Files(func(name string, _ int64) bool { return strings.HasSuffix(strings.ToLower(name), ".xml") }, 8<<20)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, importer.ErrUnknownFormat
		}
		readXMLConfig(files, b, sounds)
	}
	if err := sounds.Load(u, b); err != nil {
		return nil, err
	}
	return b, nil
}

// fsConfig is a FreeSWITCH configuration read from its XML files.
type fsConfig struct {
	vars      map[string]string
	users     map[string]*importer.User // by id
	groups    map[string][]string       // directory group -> user ids
	contexts  map[string][]*node        // dialplan context -> extensions
	menus     map[string]*node          // IVR menu name -> menu
	queues    map[string]*node          // callcenter queue name (without @domain)
	agents    map[string]string         // agent name -> user id
	tiers     map[string][]string       // queue -> agent names
	gateways  []*node
	b         *importer.Bundle
	sounds    *asterisk.Sounds
	busy      map[string]bool
	memo      map[string]*importer.Target
	unknown   map[string]bool
	timeCount int
}

func readXMLConfig(files map[string][]byte, b *importer.Bundle, sounds *asterisk.Sounds) {
	c := &fsConfig{vars: map[string]string{}, users: map[string]*importer.User{}, groups: map[string][]string{}, contexts: map[string][]*node{},
		menus: map[string]*node{}, queues: map[string]*node{}, agents: map[string]string{}, tiers: map[string][]string{}, b: b, sounds: sounds,
		busy: map[string]bool{}, memo: map[string]*importer.Target{}, unknown: map[string]bool{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
		if path.Base(n) == "vars.xml" {
			for k, v := range readVars(files[n]) {
				c.vars[k] = v
			}
		}
	}
	slices.Sort(names)
	for _, n := range names {
		root := parseXML(files[n])
		for _, u := range root.all("user") {
			id := u.attr("id")
			if id == "" || u.attr("type") == "pointer" {
				continue
			}
			p, v := u.params("param"), u.params("variable")
			user := &importer.User{Ref: id, Ext: id, Name: expand(v["effective_caller_id_name"], c.vars),
				SIPPassword: expand(p["password"], c.vars), VoicemailPIN: expand(p["vm-password"], c.vars),
				OutboundCID: expand(v["outbound_caller_id_number"], c.vars), Email: p["vm-mailto"]}
			user.Voicemail = !strings.EqualFold(p["vm-enabled"], "false")
			if user.Name == "Extension "+id {
				user.Name = ""
			}
			if strings.Contains(user.OutboundCID, "$") {
				user.OutboundCID = ""
			}
			c.users[id] = user
		}
		for _, g := range root.all("group") {
			for _, u := range g.all("user") {
				c.groups[g.attr("name")] = append(c.groups[g.attr("name")], u.attr("id"))
			}
		}
		for _, ctx := range root.all("context") {
			c.contexts[ctx.attr("name")] = append(c.contexts[ctx.attr("name")], ctx.kids("extension")...)
		}
		for _, m := range root.all("menu") {
			if m.attr("name") != "" {
				c.menus[m.attr("name")] = m
			}
		}
		for _, q := range root.all("queue") {
			if q.attr("name") != "" {
				name, _, _ := strings.Cut(q.attr("name"), "@")
				c.queues[name] = q
			}
		}
		for _, a := range root.all("agent") {
			if a.attr("name") != "" {
				c.agents[a.attr("name")] = contactUser(a.attr("contact"))
			}
		}
		for _, t := range root.all("tier") {
			q, _, _ := strings.Cut(t.attr("queue"), "@")
			c.tiers[q] = append(c.tiers[q], t.attr("agent"))
		}
		c.gateways = append(c.gateways, root.all("gateway")...)
	}
	ids := make([]string, 0, len(c.users))
	for id := range c.users {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		if x != y {
			return x - y
		}
		return strings.Compare(a, b)
	})
	for _, id := range ids {
		b.Users = append(b.Users, *c.users[id])
	}
	for _, g := range c.gateways {
		p := g.params("param")
		host := expand(p["proxy"], c.vars)
		if host == "" {
			host = expand(p["realm"], c.vars)
		}
		t := importer.Trunk{Ref: g.attr("name"), Name: g.attr("name"), Username: expand(p["username"], c.vars), Password: expand(p["password"], c.vars),
			AuthUsername: expand(p["auth-username"], c.vars), Register: !strings.EqualFold(p["register"], "false"), FromDomain: expand(p["from-domain"], c.vars),
			FromUser: expand(p["from-user"], c.vars), Transport: strings.ToLower(p["register-transport"])}
		if h, port, ok := strings.Cut(host, ":"); ok {
			host = h
			t.Port, _ = strconv.Atoi(port)
		}
		t.Host = host
		if t.AuthUsername == t.Username {
			t.AuthUsername = ""
		}
		if t.Host == "" || strings.Contains(t.Host, "$") {
			continue
		}
		b.Trunks = append(b.Trunks, t)
	}
	for name, q := range c.queues {
		p := q.params("param")
		qu := importer.Queue{Ref: name, Name: name, Strategy: callcenterStrategy(p["strategy"]), MaxWait: atoi(p["max-wait-time"])}
		for _, a := range c.tiers[name] {
			if id := c.agents[a]; id != "" && c.users[id] != nil {
				qu.Agents = append(qu.Agents, id)
			}
		}
		b.Queues = append(b.Queues, qu)
	}
	slices.SortFunc(b.Queues, func(x, y importer.Queue) int { return strings.Compare(x.Ref, y.Ref) })
	// Inbound numbers: the public context.
	for _, e := range c.contexts["public"] {
		for _, cond := range e.kids("condition") {
			if cond.attr("field") != "destination_number" {
				continue
			}
			if did := literalNumber(cond.attr("expression")); did != "" {
				t := c.resolve("public", did)
				b.Numbers = append(b.Numbers, importer.Number{Number: did, Target: t})
				if t != nil && t.Kind == importer.TargetMenu {
					if m := b.Menu(t.Ref); m != nil {
						m.Numbers = append(m.Numbers, did)
					}
				}
			}
			break
		}
	}
	if len(b.Trunks) == 1 {
		for _, n := range b.Numbers {
			b.Trunks[0].Numbers = append(b.Trunks[0].Numbers, n.Number)
		}
	}
	// Internal numbers of menus, groups and queues: what phones dial in the default context.
	for _, e := range c.contexts["default"] {
		for _, cond := range e.kids("condition") {
			if cond.attr("field") != "destination_number" {
				continue
			}
			num := literalNumber(cond.attr("expression"))
			if num == "" || c.users[num] != nil {
				break
			}
			if t := c.resolve("default", num); t != nil {
				switch t.Kind {
				case importer.TargetMenu:
					if m := b.Menu(t.Ref); m != nil && m.Ext == "" {
						m.Ext = num
					}
				case importer.TargetQueue:
					for i := range b.Queues {
						if b.Queues[i].Ref == t.Ref && b.Queues[i].Ext == "" {
							b.Queues[i].Ext = num
						}
					}
				case importer.TargetGroup:
					for i := range b.Groups {
						if b.Groups[i].Ref == t.Ref && b.Groups[i].Ext == "" {
							b.Groups[i].Ext = num
						}
					}
				}
			}
			break
		}
	}
	if len(c.unknown) > 0 {
		var list []string
		for k := range c.unknown {
			list = append(list, k)
		}
		slices.Sort(list)
		b.Note("dialplan_skipped", "apps", strings.Join(list, ", "))
	}
}

var reLiteral = regexp.MustCompile(`^\^?\(?(\\?\+?[0-9]{3,20})\)?\$?$`)

// literalNumber returns the number a regular expression like ^(0201234567)$ matches, "" when it
// matches more than one number.
func literalNumber(expr string) string {
	m := reLiteral.FindStringSubmatch(strings.TrimSpace(expr))
	if m == nil {
		return ""
	}
	return strings.TrimPrefix(m[1], `\`)
}

// contactUser returns the user of a contact string like "[call_timeout=10]user/1000@domain".
func contactUser(s string) string {
	if i := strings.LastIndex(s, "]"); i >= 0 {
		s = s[i+1:]
	}
	if rest, ok := strings.CutPrefix(s, "user/"); ok {
		id, _, _ := strings.Cut(rest, "@")
		return id
	}
	return ""
}

// callcenterStrategy maps a mod_callcenter strategy to the Asterisk name.
func callcenterStrategy(s string) string {
	switch s {
	case "ring-all", "ring-progressively":
		return "ringall"
	case "longest-idle-agent", "agent-with-least-talk-time":
		return "leastrecent"
	case "agent-with-fewest-calls":
		return "fewestcalls"
	case "top-down", "sequentially-by-agent-order":
		return "linear"
	case "random":
		return "random"
	}
	return "rrmemory"
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// resolve finds where a call to number in the context goes.
func (c *fsConfig) resolve(ctx, number string) *importer.Target {
	key := ctx + "/" + number
	if t, ok := c.memo[key]; ok {
		return t
	}
	if c.busy[key] || len(c.busy) > 30 {
		return nil
	}
	c.busy[key] = true
	defer delete(c.busy, key)
	if c.users[number] != nil && ctx != "public" {
		// The default dialplan rings a user for every directory number.
		t := &importer.Target{Kind: importer.TargetUser, Ref: number}
		if !c.hasOwnExtension(ctx, number) {
			c.memo[key] = t
			return t
		}
	}
	for _, e := range c.contexts[ctx] {
		for ci, cond := range e.kids("condition") {
			if cond.attr("field") != "destination_number" {
				continue
			}
			re, err := regexp.Compile(cond.attr("expression"))
			if err != nil {
				break
			}
			m := re.FindStringSubmatch(number)
			if m == nil {
				break
			}
			t := c.actions(ctx, e, e.kids("condition")[ci:], m)
			c.memo[key] = t
			return t
		}
	}
	c.memo[key] = nil
	return nil
}

func (c *fsConfig) hasOwnExtension(ctx, number string) bool {
	for _, e := range c.contexts[ctx] {
		for _, cond := range e.kids("condition") {
			if cond.attr("field") == "destination_number" && literalNumber(cond.attr("expression")) == number {
				return true
			}
		}
	}
	return false
}

var reGroupArg = regexp.MustCompile(`\$([0-9])`)

func subst(s string, m []string) string {
	return reGroupArg.ReplaceAllStringFunc(s, func(x string) string {
		i := int(x[1] - '0')
		if i < len(m) {
			return m[i]
		}
		return x
	})
}

// isTime reports whether a condition tests the time of day or the weekday.
func isTime(cond *node) bool {
	for _, a := range []string{"wday", "hour", "time-of-day", "minute-of-day"} {
		if cond.attr(a) != "" {
			return true
		}
	}
	return false
}

var fsDays = map[string]int{"sun": 7, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

// fsWday parses a FreeSWITCH wday ("2-6", "mon-fri", "1,7"): 1 is Sunday there.
func fsWday(s string) []int {
	if s == "" {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	conv := func(x string) int {
		x = strings.ToLower(strings.TrimSpace(x))
		if d, ok := fsDays[x]; ok {
			return d
		}
		n := atoi(x)
		if n < 1 || n > 7 {
			return 0
		}
		if n == 1 {
			return 7
		}
		return n - 1
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		from, to, isRange := strings.Cut(part, "-")
		a := conv(from)
		if a == 0 {
			return nil
		}
		if !isRange {
			out = append(out, a)
			continue
		}
		z := conv(to)
		if z == 0 {
			return nil
		}
		for d := a; ; d = d%7 + 1 {
			out = append(out, d)
			if d == z {
				break
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// fsTimes parses time-of-day ("08:00-17:30") or hour ("8-17").
func fsTimes(cond *node) (string, string, bool) {
	hhmm := func(s string) string {
		h, m, _ := strings.Cut(strings.TrimSpace(s), ":")
		return strconv.Itoa(100 + atoi(h))[1:] + ":" + strconv.Itoa(100 + atoi(m))[1:]
	}
	if tod := cond.attr("time-of-day"); tod != "" {
		from, to, ok := strings.Cut(tod, "-")
		return hhmm(from), hhmm(to), ok
	}
	if h := cond.attr("hour"); h != "" {
		from, to, ok := strings.Cut(h, "-")
		if !ok {
			to = from
		}
		end := atoi(to) + 1
		if end >= 24 {
			return hhmm(from), "24:00", true
		}
		return hhmm(from), strconv.Itoa(100 + end)[1:] + ":00", true
	}
	return "00:00", "24:00", true
}

// actions follows the actions of an extension.
func (c *fsConfig) actions(ctx string, e *node, conds []*node, m []string) *importer.Target {
	var audio []string
	finish := func(t *importer.Target) *importer.Target {
		if len(audio) == 0 {
			return t
		}
		ref := ctx + "/" + e.attr("name") + "/play"
		if c.b.Menu(ref) == nil {
			c.b.Menus = append(c.b.Menus, &importer.Step{Ref: ref, Type: "root", Title: e.attr("name"), Audio: audio[0], Target: t})
		}
		return &importer.Target{Kind: importer.TargetMenu, Ref: ref}
	}
	for i, cond := range conds {
		if i > 0 && isTime(cond) {
			// A time condition: the actions run during the hours, the anti-actions outside them.
			from, to, ok := fsTimes(cond)
			days := fsWday(cond.attr("wday"))
			c.timeCount++
			ref := ctx + "/" + e.attr("name") + "/" + strconv.Itoa(c.timeCount)
			tc := importer.TimeCondition{Ref: ref, Name: e.attr("name")}
			if ok && days != nil {
				tc.Schedule = []pbx.TimeRange{{Days: days, From: from, To: to}}
			}
			tc.Open = c.run(ctx, cond.kids("action"), m)
			tc.Closed = c.run(ctx, cond.kids("anti-action"), m)
			c.b.TimeConditions = append(c.b.TimeConditions, tc)
			return finish(&importer.Target{Kind: importer.TargetTime, Ref: ref})
		}
		var t *importer.Target
		var done bool
		t, done, audio = c.step(ctx, cond.kids("action"), m, audio)
		if done {
			return finish(t)
		}
	}
	return finish(nil)
}

func (c *fsConfig) run(ctx string, acts []*node, m []string) *importer.Target {
	t, _, audio := c.step(ctx, acts, m, nil)
	if len(audio) > 0 {
		ref := ctx + "/anti/" + strconv.Itoa(len(c.b.Menus))
		c.b.Menus = append(c.b.Menus, &importer.Step{Ref: ref, Type: "root", Title: ctx, Audio: audio[0], Target: t})
		return &importer.Target{Kind: importer.TargetMenu, Ref: ref}
	}
	return t
}

// step runs actions until one sends the call somewhere.
func (c *fsConfig) step(ctx string, acts []*node, m []string, audio []string) (*importer.Target, bool, []string) {
	for i, a := range acts {
		app := strings.ToLower(a.attr("application"))
		data := subst(expand(a.attr("data"), c.vars), m)
		switch app {
		case "answer", "set", "export", "sleep", "hash", "ring_ready", "pre_answer", "info", "log", "unset", "limit", "record_session",
			"bind_meta_app", "set_zombie_exec", "lua", "db", "event", "respond", "send_display", "multiset":
			continue
		case "playback", "endless_playback":
			if f := c.sounds.Find(data); f != "" {
				audio = append(audio, f)
			}
		case "bridge":
			return c.bridge(ctx, data), true, audio
		case "transfer":
			f := strings.Fields(data)
			if len(f) == 0 {
				continue
			}
			target := ctx
			if len(f) >= 3 {
				target = f[2]
			}
			return c.resolve(target, f[0]), true, audio
		case "ivr":
			t := c.menu(data)
			if mm := c.b.Menu(t.Ref); mm != nil && mm.NoAction == nil {
				mm.NoAction, _, _ = c.step(ctx, acts[i+1:], m, nil)
			}
			return t, true, audio
		case "callcenter":
			name, _, _ := strings.Cut(data, "@")
			for qi := range c.b.Queues {
				if c.b.Queues[qi].Ref == name {
					if c.b.Queues[qi].Overflow == nil {
						c.b.Queues[qi].Overflow, _, _ = c.step(ctx, acts[i+1:], m, nil)
					}
					return &importer.Target{Kind: importer.TargetQueue, Ref: name}, true, audio
				}
			}
		case "voicemail":
			f := strings.Fields(data)
			if len(f) > 0 {
				box := f[len(f)-1]
				if c.users[box] != nil {
					return &importer.Target{Kind: importer.TargetVoicemail, Ref: box}, true, audio
				}
			}
		case "hangup":
			return nil, true, audio
		default:
			c.unknown[app] = true
		}
	}
	return nil, false, audio
}

// bridge turns a bridge string into a user, a group or an external number.
func (c *fsConfig) bridge(ctx, data string) *importer.Target {
	var users, external []string
	for _, part := range strings.FieldsFunc(data, func(r rune) bool { return r == ',' || r == '|' }) {
		part = strings.TrimSpace(part)
		if i := strings.LastIndex(part, "]"); i >= 0 {
			part = part[i+1:]
		}
		for _, p := range strings.Split(part, ":_:") {
			switch {
			case strings.HasPrefix(p, "user/"):
				if id := contactUser(p); c.users[id] != nil {
					users = append(users, id)
				}
			case strings.HasPrefix(p, "group/"):
				name, _, _ := strings.Cut(strings.TrimPrefix(p, "group/"), "@")
				if _, ok := c.groups[name]; ok && !slices.ContainsFunc(c.b.Groups, func(g importer.Group) bool { return g.Ref == name }) {
					g := importer.Group{Ref: name, Name: name, Strategy: "simultaneous"}
					for _, id := range c.groups[name] {
						if c.users[id] != nil {
							g.Members = append(g.Members, id)
						}
					}
					c.b.Groups = append(c.b.Groups, g)
				}
				return &importer.Target{Kind: importer.TargetGroup, Ref: name}
			case strings.HasPrefix(p, "sofia/gateway/"):
				rest := strings.TrimPrefix(p, "sofia/gateway/")
				if _, num, ok := strings.Cut(rest, "/"); ok {
					if n := pbx.NormalizeDID(num); regexp.MustCompile(`^\+?[0-9]{3,20}$`).MatchString(n) {
						external = append(external, n)
					}
				}
			}
		}
	}
	switch {
	case len(users) == 1 && len(external) == 0:
		return &importer.Target{Kind: importer.TargetUser, Ref: users[0]}
	case len(users) == 0 && len(external) == 1:
		return &importer.Target{Kind: importer.TargetExternal, Ref: external[0]}
	case len(users)+len(external) > 1:
		ref := ctx + "/" + strings.Join(append(users, external...), "+")
		if !slices.ContainsFunc(c.b.Groups, func(g importer.Group) bool { return g.Ref == ref }) {
			c.b.Groups = append(c.b.Groups, importer.Group{Ref: ref, Name: "Group " + strings.Join(users, " "), Strategy: "simultaneous", Members: users, External: external})
		}
		return &importer.Target{Kind: importer.TargetGroup, Ref: ref}
	}
	return nil
}

// menu turns an IVR menu into a voice menu.
func (c *fsConfig) menu(name string) *importer.Target {
	t := &importer.Target{Kind: importer.TargetMenu, Ref: name}
	if c.b.Menu(name) != nil {
		return t
	}
	n := c.menus[name]
	s := &importer.Step{Ref: name, Type: "root", Title: name}
	c.b.Menus = append(c.b.Menus, s)
	if n == nil {
		return t
	}
	s.Audio = c.sounds.Find(expand(n.attr("greet-long"), c.vars))
	for _, e := range n.kids("entry") {
		digits := e.attr("digits")
		if len(digits) != 1 {
			continue
		}
		step := &importer.Step{Type: "menu", Key: digits, Title: digits}
		param := expand(e.attr("param"), c.vars)
		switch e.attr("action") {
		case "menu-exec-app":
			app, data, _ := strings.Cut(param, " ")
			step.Target, _, _ = c.step("default", []*node{{Name: "action", Attr: map[string]string{"application": app, "data": data}}}, nil, nil)
		case "menu-sub":
			step.Target = c.menu(param)
		case "menu-top":
			step.Target = t
		case "menu-play-sound":
			step.Audio = c.sounds.Find(param)
			step.Type = "audio"
			step.Target = t
		default:
			continue
		}
		s.Children = append(s.Children, step)
	}
	return t
}
