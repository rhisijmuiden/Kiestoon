package asterisk

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// priority is one step of an extension: an application with its arguments.
type priority struct {
	label string
	app   string // lower case
	args  string
}

type extension struct {
	name  string
	prios []priority
}

type dpContext struct {
	name     string
	exts     []*extension
	includes []string
}

func (c *dpContext) ext(name string) *extension {
	for _, e := range c.exts {
		if e.name == name {
			return e
		}
	}
	return nil
}

// Dialplan is extensions.conf.
type Dialplan struct {
	contexts []*dpContext
}

func (d *Dialplan) context(name string) *dpContext {
	for _, c := range d.contexts {
		if strings.EqualFold(c.name, name) {
			return c
		}
	}
	return nil
}

var reNumber = regexp.MustCompile(`^\+?[0-9]{3,20}$`)

var reApp = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:\((.*)\))?\s*$`)

// parseDialplan reads the contexts of extensions.conf.
func parseDialplan(c *Config) *Dialplan {
	d := &Dialplan{}
	for _, s := range c.Sections {
		low := strings.ToLower(s.Name)
		if low == "general" || low == "globals" || s.Template {
			continue
		}
		ctx := d.context(s.Name)
		if ctx == nil {
			ctx = &dpContext{name: s.Name}
			d.contexts = append(d.contexts, ctx)
		}
		var last *extension
		for _, kv := range s.Keys {
			switch kv.Key {
			case "include":
				inc := strings.TrimSpace(strings.Split(kv.Value, ",")[0])
				ctx.includes = append(ctx.includes, inc)
			case "exten", "same":
				v := kv.Value
				var name string
				if kv.Key == "exten" {
					i := strings.IndexByte(v, ',')
					if i < 0 {
						continue
					}
					name, v = strings.TrimSpace(v[:i]), v[i+1:]
					// "100/caller" (caller id match) is treated as the extension itself.
					if j := strings.IndexByte(name, '/'); j > 0 {
						name = name[:j]
					}
				} else if last == nil {
					continue
				}
				i := strings.IndexByte(v, ',')
				if i < 0 {
					continue
				}
				prio, appPart := strings.TrimSpace(v[:i]), strings.TrimSpace(v[i+1:])
				if strings.EqualFold(prio, "hint") {
					continue
				}
				label := ""
				if j := strings.IndexByte(prio, '('); j >= 0 && strings.HasSuffix(prio, ")") {
					label = prio[j+1 : len(prio)-1]
				}
				m := reApp.FindStringSubmatch(appPart)
				if m == nil {
					continue
				}
				var e *extension
				if kv.Key == "same" {
					e = last
				} else if e = ctx.ext(name); e == nil {
					e = &extension{name: name}
					ctx.exts = append(ctx.exts, e)
				}
				if p := strings.TrimSpace(strings.Split(prio, "(")[0]); p == "1" && kv.Key == "exten" && len(e.prios) > 0 {
					e.prios = nil // a second "exten => x,1" replaces the first
				}
				e.prios = append(e.prios, priority{label: label, app: strings.ToLower(m[1]), args: m[2]})
				last = e
			}
		}
	}
	return d
}

// splitArgs splits application arguments at commas outside brackets and quotes.
func splitArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	quoted := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case '(', '[', '{':
			if !quoted {
				depth++
			}
		case ')', ']', '}':
			if !quoted && depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 && !quoted {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// resolver follows the dialplan to find where a call goes.
type resolver struct {
	dp      *Dialplan
	acc     *accounts
	b       *importer.Bundle
	sounds  func(ref string) string // sound reference -> audio file in the upload ("" when absent)
	memo    map[string]*importer.Target
	busy    map[string]bool
	skipped map[string]bool
	groups  int
	times   int
	menus   int
}

func newResolver(dp *Dialplan, acc *accounts, b *importer.Bundle, sounds func(string) string) *resolver {
	return &resolver{dp: dp, acc: acc, b: b, sounds: sounds, memo: map[string]*importer.Target{}, busy: map[string]bool{}, skipped: map[string]bool{}}
}

// findExt finds the extension in the context or the contexts it includes.
func (r *resolver) findExt(ctxName, ext string, depth int) (*dpContext, *extension) {
	c := r.dp.context(ctxName)
	if c == nil || depth > 6 {
		return nil, nil
	}
	if e := c.ext(ext); e != nil {
		return c, e
	}
	for _, inc := range c.includes {
		if ic, e := r.findExt(inc, ext, depth+1); e != nil {
			return ic, e
		}
	}
	// Pattern match (_XXX) as a last resort.
	for _, e := range c.exts {
		if strings.HasPrefix(e.name, "_") && matchPattern(e.name[1:], ext) {
			return c, e
		}
	}
	return nil, nil
}

// matchPattern matches an Asterisk extension pattern (without the leading _).
func matchPattern(p, ext string) bool {
	i := 0
	for j := 0; j < len(p); j++ {
		if p[j] == '.' || p[j] == '!' {
			return i < len(ext) || p[j] == '!'
		}
		if i >= len(ext) {
			return false
		}
		c := ext[i]
		switch p[j] {
		case 'X', 'x':
			if c < '0' || c > '9' {
				return false
			}
		case 'Z', 'z':
			if c < '1' || c > '9' {
				return false
			}
		case 'N', 'n':
			if c < '2' || c > '9' {
				return false
			}
		case '[':
			k := strings.IndexByte(p[j:], ']')
			if k < 0 {
				return false
			}
			set := p[j+1 : j+k]
			if !inSet(set, c) {
				return false
			}
			j += k
		default:
			if p[j] != c {
				return false
			}
		}
		i++
	}
	return i == len(ext)
}

func inSet(set string, c byte) bool {
	for i := 0; i < len(set); i++ {
		if i+2 < len(set) && set[i+1] == '-' {
			if c >= set[i] && c <= set[i+2] {
				return true
			}
			i += 2
			continue
		}
		if set[i] == c {
			return true
		}
	}
	return false
}

// Resolve returns where a call to ext in the context goes (nil: hang up).
func (r *resolver) Resolve(ctxName, ext string) *importer.Target {
	return r.resolveAt(ctxName, ext, 0)
}

func (r *resolver) resolveAt(ctxName, ext string, start int) *importer.Target {
	key := ctxName + "/" + ext + "/" + strconv.Itoa(start)
	if t, ok := r.memo[key]; ok {
		return t
	}
	if r.busy[key] || len(r.busy) > 40 {
		// A loop, usually a menu that goes back to itself ("press 9 to repeat").
		ref := ctxName
		if ext != "s" {
			ref += "/" + ext
		}
		if r.b.Menu(ref) != nil {
			return &importer.Target{Kind: importer.TargetMenu, Ref: ref}
		}
		return nil
	}
	r.busy[key] = true
	defer delete(r.busy, key)
	c, e := r.findExt(ctxName, ext, 0)
	if e == nil {
		r.memo[key] = nil
		return nil
	}
	t := r.run(c, e, ext, start)
	r.memo[key] = t
	return t
}

// gotoTarget resolves Goto arguments: [[context,]exten,]priority or a label.
func (r *resolver) gotoTarget(c *dpContext, e *extension, ext, args string) *importer.Target {
	a := splitArgs(args)
	switch len(a) {
	case 1:
		// Goto(label) or Goto(priority) within this extension
		for i, p := range e.prios {
			if p.label == a[0] {
				return r.run(c, e, ext, i)
			}
		}
		if n, err := strconv.Atoi(a[0]); err == nil && n >= 1 && n-1 < len(e.prios) {
			return r.run(c, e, ext, n-1)
		}
		return nil
	case 2:
		return r.resolveLabel(c.name, a[0], a[1])
	}
	return r.resolveLabel(a[0], a[1], a[2])
}

func (r *resolver) resolveLabel(ctxName, ext, prio string) *importer.Target {
	if strings.Contains(ext, "${") {
		return nil
	}
	_, e := r.findExt(ctxName, ext, 0)
	if e == nil {
		return nil
	}
	start := 0
	for i, p := range e.prios {
		if p.label == prio && prio != "" {
			start = i
		}
	}
	if n, err := strconv.Atoi(prio); err == nil && n > 1 && n-1 < len(e.prios) {
		start = n - 1
	}
	return r.resolveAt(ctxName, ext, start)
}

// dialTargets turns a Dial argument into users, external numbers and other targets.
func (r *resolver) dialTargets(arg string) (users []string, external []string, other *importer.Target) {
	for _, part := range strings.Split(arg, "&") {
		part = strings.TrimSpace(part)
		tech, rest, ok := strings.Cut(part, "/")
		if !ok {
			continue
		}
		switch strings.ToLower(tech) {
		case "local":
			num, ctx, _ := strings.Cut(rest, "@")
			ctx, _, _ = strings.Cut(ctx, "/")
			if t := r.Resolve(ctx, num); t != nil {
				if t.Kind == importer.TargetUser {
					users = append(users, t.Ref)
				} else if t.Kind == importer.TargetExternal {
					external = append(external, t.Ref)
				} else if other == nil {
					other = t
				}
			}
		case "pjsip", "sip", "iax2":
			name, host, hasAt := strings.Cut(rest, "@")
			if strings.ToLower(tech) == "sip" && !hasAt {
				// SIP/trunk/number
				if tr, num, ok := strings.Cut(rest, "/"); ok {
					name, host, hasAt = num, tr, true
				}
			}
			if hasAt {
				if d := r.acc.byName(host); d != nil && d.external || r.acc.byName(host) == nil {
					if n := pbx.NormalizeDID(name); reNumber.MatchString(n) {
						external = append(external, n)
					}
					continue
				}
			}
			if d := r.acc.byName(name); d != nil && d.user != nil {
				users = append(users, d.user.Ref)
			}
		}
	}
	return
}

// run follows the priorities of an extension from start.
func (r *resolver) run(c *dpContext, e *extension, ext string, start int) *importer.Target {
	var audio []string
	var menuAudio string
	finish := func(t *importer.Target) *importer.Target {
		if len(audio) == 0 {
			return t
		}
		// An announcement before the destination becomes a small voice menu without keys.
		r.menus++
		ref := c.name + "/" + ext + "/play" + strconv.Itoa(start)
		if r.b.Menu(ref) == nil {
			s := &importer.Step{Ref: ref, Type: "root", Title: PromptTitle(c.name, ext), Audio: audio[0], Target: t}
			// Several files in a row: a chain of announcements.
			cur := s
			for _, a := range audio[1:] {
				n := &importer.Step{Type: "audio", Audio: a, Target: t}
				cur.Target, cur.Children = nil, []*importer.Step{n}
				cur = n
			}
			r.b.Menus = append(r.b.Menus, s)
		}
		return &importer.Target{Kind: importer.TargetMenu, Ref: ref}
	}
	for i := start; i < len(e.prios); i++ {
		p := e.prios[i]
		args := splitArgs(p.args)
		switch p.app {
		case "answer", "wait", "set", "noop", "ringing", "progress", "verbose", "log", "macro", "gosub", "gosubif", "execif",
			"setcallerpres", "setcidname", "setmusiconhold", "musiconhold", "ringgroupoptions", "waitforsilence", "playtones", "senddtmf",
			"mixmonitor", "monitor", "agi", "system", "userevent", "dumpchan", "resetcdr", "nocdr", "callerid", "sayunixtime", "sayalpha",
			"saydigits", "saynumber", "authenticate", "chanisavail":
			continue
		case "playback":
			for _, f := range strings.Split(args[0], "&") {
				if a := r.sounds(f); a != "" {
					audio = append(audio, a)
				}
			}
		case "background":
			for _, f := range strings.Split(args[0], "&") {
				if a := r.sounds(f); a != "" && menuAudio == "" {
					menuAudio = a
				}
			}
			if r.isMenu(c) {
				return finish(r.menu(c, ext, menuAudio))
			}
		case "waitexten", "read":
			if r.isMenu(c) {
				return finish(r.menu(c, ext, menuAudio))
			}
		case "dial":
			users, ext2, other := r.dialTargets(args[0])
			if other != nil && len(users) == 0 && len(ext2) == 0 {
				return finish(other)
			}
			if len(users) == 1 && len(ext2) == 0 {
				return finish(&importer.Target{Kind: importer.TargetUser, Ref: users[0]})
			}
			if len(users) == 0 && len(ext2) == 1 {
				return finish(&importer.Target{Kind: importer.TargetExternal, Ref: ext2[0]})
			}
			if len(users)+len(ext2) == 0 {
				continue
			}
			ring := 0
			if len(args) > 1 {
				ring = atoi(args[1])
			}
			return finish(r.group(c, e, ext, i, users, ext2, ring))
		case "queue":
			name := strings.TrimSpace(args[0])
			if q := r.queueRef(name); q != "" {
				if t := r.run(c, e, ext, i+1); t != nil {
					if qq := r.bQueue(q); qq != nil && qq.Overflow == nil {
						qq.Overflow = t
					}
				}
				return finish(&importer.Target{Kind: importer.TargetQueue, Ref: q})
			}
		case "voicemail":
			box, _, _ := strings.Cut(strings.Split(args[0], "&")[0], "@")
			box = strings.TrimLeft(box, "subu")
			if u := r.b.User(box); u != nil {
				u.Voicemail = true
				return finish(&importer.Target{Kind: importer.TargetVoicemail, Ref: u.Ref})
			}
		case "goto":
			return finish(r.gotoTarget(c, e, ext, p.args))
		case "gotoif":
			cond, branches, ok := strings.Cut(p.args, "?")
			_ = cond
			if !ok {
				continue
			}
			yes, _, _ := strings.Cut(branches, ":")
			if strings.TrimSpace(yes) == "" {
				continue
			}
			r.skipped["GotoIf"] = true
			return finish(r.gotoTarget(c, e, ext, yes))
		case "gotoiftime":
			return finish(r.timeCondition(c, e, ext, i, p.args))
		case "hangup", "busy", "congestion":
			return finish(nil)
		default:
			r.skipped[p.app] = true
		}
	}
	return finish(nil)
}

// PromptTitle names a step after its context and extension.
func PromptTitle(ctx, ext string) string {
	if ext == "s" || ext == "" {
		return ctx
	}
	return ctx + " " + ext
}

// isMenu reports whether a context has single-key extensions (a voice menu).
func (r *resolver) isMenu(c *dpContext) bool {
	n := 0
	for _, e := range c.exts {
		if len(e.name) == 1 && strings.Contains("0123456789*#", e.name) {
			n++
		}
	}
	return n > 0
}

// menu turns a context with key extensions into a voice menu.
func (r *resolver) menu(c *dpContext, ext, audio string) *importer.Target {
	ref := c.name
	if ext != "s" {
		ref += "/" + ext
	}
	t := &importer.Target{Kind: importer.TargetMenu, Ref: ref}
	if r.b.Menu(ref) != nil {
		return t
	}
	s := &importer.Step{Ref: ref, Type: "root", Title: PromptTitle(c.name, ext), Audio: audio}
	r.b.Menus = append(r.b.Menus, s) // before the keys, so a key back to this menu finds it
	for _, k := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "*", "#"} {
		if c.ext(k) == nil {
			continue
		}
		s.Children = append(s.Children, &importer.Step{Type: "menu", Key: k, Title: k, Target: r.Resolve(c.name, k)})
	}
	for _, x := range []string{"t", "i"} {
		if c.ext(x) != nil && s.NoAction == nil {
			s.NoAction = r.Resolve(c.name, x)
		}
	}
	return t
}

// group makes a ring group for a Dial to several phones. Several Dials in a row (each to one
// phone) form a sequential group.
func (r *resolver) group(c *dpContext, e *extension, ext string, at int, users, external []string, ring int) *importer.Target {
	ref := c.name + "/" + ext
	if g := r.bGroup(ref); g != nil {
		return &importer.Target{Kind: importer.TargetGroup, Ref: ref}
	}
	g := importer.Group{Ref: ref, Name: ext, Ext: ext, Strategy: "simultaneous", Members: users, External: external, RingSeconds: ring}
	if !reDigits.MatchString(ext) {
		g.Ext, g.Name = "", PromptTitle(c.name, ext)
	}
	r.b.Groups = append(r.b.Groups, g)
	if t := r.run(c, e, ext, at+1); t != nil {
		r.bGroup(ref).Failover = t
	}
	return &importer.Target{Kind: importer.TargetGroup, Ref: ref}
}

func (r *resolver) bGroup(ref string) *importer.Group {
	for i := range r.b.Groups {
		if r.b.Groups[i].Ref == ref {
			return &r.b.Groups[i]
		}
	}
	return nil
}

func (r *resolver) bQueue(ref string) *importer.Queue {
	for i := range r.b.Queues {
		if r.b.Queues[i].Ref == ref {
			return &r.b.Queues[i]
		}
	}
	return nil
}

func (r *resolver) queueRef(name string) string {
	if q := r.bQueue(name); q != nil {
		return q.Ref
	}
	return ""
}

var dayNames = map[string]int{"mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6, "sun": 7}

// parseDays turns "mon-fri" or "sat&sun" or "*" into day numbers.
func parseDays(s string) []int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "*" || s == "" {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	var out []int
	for _, part := range strings.Split(s, "&") {
		from, to, isRange := strings.Cut(part, "-")
		a, ok1 := dayNames[strings.TrimSpace(from)]
		if !ok1 {
			return nil
		}
		if !isRange {
			out = append(out, a)
			continue
		}
		b, ok2 := dayNames[strings.TrimSpace(to)]
		if !ok2 {
			return nil
		}
		for d := a; ; d = d%7 + 1 {
			out = append(out, d)
			if d == b {
				break
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// parseTimes turns "08:00-17:30" or "*" into from/to.
func parseTimes(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "*" || s == "" {
		return "00:00", "24:00", true
	}
	from, to, ok := strings.Cut(s, "-")
	if !ok {
		return "", "", false
	}
	norm := func(t string) string {
		h, m, _ := strings.Cut(strings.TrimSpace(t), ":")
		hh, mm := atoi(h), atoi(m)
		if hh == 23 && mm == 59 {
			return "24:00"
		}
		return strconv.Itoa(100 + hh)[1:] + ":" + strconv.Itoa(100 + mm)[1:]
	}
	return norm(from), norm(to), true
}

// timeCondition turns GotoIfTime(times,weekdays,mdays,months?true:false) into a time condition.
// Consecutive GotoIfTime lines to the same place form one schedule.
func (r *resolver) timeCondition(c *dpContext, e *extension, ext string, at int, args string) *importer.Target {
	ref := c.name + "/" + ext + "/" + strconv.Itoa(at)
	t := &importer.Target{Kind: importer.TargetTime, Ref: ref}
	for i := range r.b.TimeConditions {
		if r.b.TimeConditions[i].Ref == ref {
			return t
		}
	}
	tc := importer.TimeCondition{Ref: ref, Name: PromptTitle(c.name, ext) + " hours"}
	r.b.TimeConditions = append(r.b.TimeConditions, tc)
	var openBranch, closedBranch string
	i := at
	for ; i < len(e.prios) && e.prios[i].app == "gotoiftime"; i++ {
		spec, branches, ok := strings.Cut(e.prios[i].args, "?")
		if !ok {
			continue
		}
		yes, no, _ := strings.Cut(branches, ":")
		if openBranch == "" {
			openBranch, closedBranch = yes, no
		} else if yes != openBranch {
			break // a different place: the next time condition
		}
		f := splitArgs(strings.ReplaceAll(spec, "|", ","))
		for len(f) < 4 {
			f = append(f, "*")
		}
		from, to, ok1 := parseTimes(f[0])
		days := parseDays(f[1])
		if !ok1 || days == nil || (f[2] != "*" && f[2] != "") || (f[3] != "*" && f[3] != "") {
			r.b.Note("time_rule_skipped", "rule", spec)
			continue
		}
		r.timeRef(ref).Schedule = append(r.timeRef(ref).Schedule, pbx.TimeRange{Days: days, From: from, To: to})
	}
	r.timeRef(ref).Open = r.gotoTarget(c, e, ext, openBranch)
	if strings.TrimSpace(closedBranch) != "" {
		r.timeRef(ref).Closed = r.gotoTarget(c, e, ext, closedBranch)
	} else {
		r.timeRef(ref).Closed = r.run(c, e, ext, i)
	}
	return t
}

func (r *resolver) timeRef(ref string) *importer.TimeCondition {
	for i := range r.b.TimeConditions {
		if r.b.TimeConditions[i].Ref == ref {
			return &r.b.TimeConditions[i]
		}
	}
	return nil
}
