package asterisk

import (
	"context"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func init() { importer.Register(Adapter{}) }

// Adapter reads plain Asterisk configuration files: loose files, or an archive of /etc/asterisk
// (optionally with /var/lib/asterisk/sounds).
type Adapter struct{}

// ID implements importer.Adapter.
func (Adapter) ID() string { return "asterisk" }

// configNames are the files the adapter reads.
var configNames = []string{"pjsip.conf", "pjsip_wizard.conf", "sip.conf", "users.conf", "extensions.conf", "voicemail.conf", "queues.conf"}

// Detect implements importer.Adapter.
func (Adapter) Detect(u *importer.Upload) int {
	score := 0
	for _, n := range []string{"extensions.conf", "pjsip.conf", "sip.conf"} {
		if u.Has(n) {
			score += 25
		}
	}
	if score > 0 && u.Has("asterisk.conf") {
		score += 10
	}
	return min(score, 80)
}

// confDir picks the directory with the configuration: the one that holds extensions.conf or
// pjsip.conf (an archive may hold more than one copy, e.g. a backup of a backup).
func confDir(u *importer.Upload) string {
	for _, n := range []string{"extensions.conf", "pjsip.conf", "sip.conf"} {
		if f := u.Find(n); f != "" {
			return path.Dir(f)
		}
	}
	return "."
}

// Read implements importer.Adapter.
func (Adapter) Read(ctx context.Context, u *importer.Upload) (*importer.Bundle, error) {
	dir := confDir(u)
	files := Files{}
	raw, err := u.Files(func(name string, size int64) bool {
		return path.Dir(name) == dir && strings.HasSuffix(name, ".conf")
	}, 16<<20)
	if err != nil {
		return nil, err
	}
	for n, b := range raw {
		files[path.Base(n)] = b
	}
	b := &importer.Bundle{Info: importer.Info{Product: "Asterisk"}}
	if v := versionOf(files); v != "" {
		b.Info.Product = "Asterisk " + v
	}
	audioIndex := indexAudio(u.Entries())
	used := map[string]bool{}
	sounds := func(ref string) string {
		f := audioIndex.find(ref)
		if f != "" {
			used[f] = true
		}
		return f
	}
	if err := Build(b, files, sounds); err != nil {
		return nil, err
	}
	if len(used) > 0 {
		audio, err := u.Files(func(name string, _ int64) bool { return used[name] }, importer.MaxAudioFile)
		if err != nil {
			return nil, err
		}
		b.Audio = audio
	}
	return b, nil
}

func versionOf(files Files) string {
	for _, f := range configNames {
		data := files[f]
		if i := strings.Index(string(data), "Asterisk "); i >= 0 {
			v := strings.Fields(string(data[i+9 : min(len(data), i+40)]))
			if len(v) > 0 && v[0] != "" && v[0][0] >= '1' && v[0][0] <= '9' {
				return strings.TrimRight(v[0], ".,;")
			}
		}
	}
	return ""
}

// Build fills the bundle from the configuration files. It is shared with adapters of systems
// built on Asterisk (they add what their database holds).
func Build(b *importer.Bundle, files Files, sounds func(string) string) error {
	acc := &accounts{}
	if _, ok := files["pjsip.conf"]; ok {
		readPJSIP(Parse("pjsip.conf", files), acc)
	}
	if _, ok := files["pjsip_wizard.conf"]; ok {
		readPJSIP(Parse("pjsip_wizard.conf", files), acc)
	}
	if _, ok := files["sip.conf"]; ok {
		readSIP(Parse("sip.conf", files), acc)
	}
	if _, ok := files["users.conf"]; ok {
		readUsersConf(Parse("users.conf", files), acc)
	}
	boxes := map[string]voicemailBox{}
	if _, ok := files["voicemail.conf"]; ok {
		boxes = readVoicemail(Parse("voicemail.conf", files))
	}
	// Users: one per extension number (a user with a desk phone and a softphone has two devices).
	for _, d := range acc.devices {
		if d.user == nil || b.User(d.user.Ref) != nil {
			if d.user != nil && b.User(d.user.Ref).SIPPassword == "" {
				b.User(d.user.Ref).SIPPassword = d.user.SIPPassword
			}
			continue
		}
		u := *d.user
		box := u.Ext
		if d.mailbox != "" {
			box, _, _ = strings.Cut(strings.Split(d.mailbox, ",")[0], "@")
		}
		if vm, ok := boxes[box]; ok {
			u.Voicemail, u.VoicemailPIN = true, vm.pin
			if u.Email == "" {
				u.Email = vm.email
			}
			u.VoicemailEmail = vm.attach
			if u.Name == "" {
				u.Name = vm.name
			}
		}
		b.Users = append(b.Users, u)
	}
	for _, d := range acc.devices {
		if d.trunk != nil {
			b.Trunks = append(b.Trunks, *d.trunk)
		}
	}
	if _, ok := files["queues.conf"]; ok {
		qc := Parse("queues.conf", files)
		for _, s := range qc.Sections {
			if s.Template || strings.EqualFold(s.Name, "general") {
				continue
			}
			q := importer.Queue{Ref: s.Name, Name: s.Name, Strategy: queueStrategies[strings.ToLower(s.Get("strategy"))],
				RingTime: atoi(s.Get("timeout")), WrapUp: atoi(s.Get("wrapuptime")), MaxSize: atoi(s.Get("maxlen")),
				MusicClass: s.Get("musicclass")}
			for _, m := range s.All("member") {
				dev := queueMember(m)
				if d := acc.byName(dev); d != nil && d.user != nil {
					q.Agents = append(q.Agents, d.user.Ref)
				} else if b.User(dev) != nil {
					q.Agents = append(q.Agents, dev)
				}
			}
			if a := s.Get("announce"); a != "" {
				q.Greeting = sounds(a)
			}
			b.Queues = append(b.Queues, q)
		}
	}
	if _, ok := files["extensions.conf"]; ok {
		readDialplan(b, acc, parseDialplan(Parse("extensions.conf", files)), sounds)
	} else {
		b.Note("no_dialplan")
	}
	return nil
}

// trunkContexts are the usual names of the context calls from the provider arrive in.
var trunkContexts = []string{"from-trunk", "from-pstn", "from-external", "incoming", "inbound", "from-provider", "from-sip", "did", "from-did-direct"}

func readDialplan(b *importer.Bundle, acc *accounts, dp *Dialplan, sounds func(string) string) {
	r := newResolver(dp, acc, b, sounds)
	// Inbound numbers: the extensions of the contexts the trunks send calls to.
	var inbound []string
	for _, d := range acc.devices {
		if d.trunk != nil && d.context != "" && !slices.Contains(inbound, d.context) {
			inbound = append(inbound, d.context)
		}
	}
	for _, c := range dp.contexts {
		if slices.Contains(trunkContexts, strings.ToLower(c.name)) && !slices.Contains(inbound, c.name) {
			inbound = append(inbound, c.name)
		}
	}
	catchAll := false
	for _, name := range inbound {
		c := dp.context(name)
		if c == nil {
			continue
		}
		for _, e := range c.exts {
			did := strings.TrimPrefix(e.name, "+")
			if d := pbx.NormalizeDID(e.name); reNumber.MatchString(d) && len(did) >= 6 {
				t := r.Resolve(c.name, e.name)
				b.Numbers = append(b.Numbers, importer.Number{Number: d, Target: t})
				continue
			}
			if e.name == "s" || strings.HasPrefix(e.name, "_") {
				catchAll = true
			}
		}
	}
	if catchAll {
		b.Note("catch_all_inbound")
	}
	// One trunk: its numbers are the inbound numbers.
	if len(b.Trunks) == 1 {
		for _, n := range b.Numbers {
			if !slices.Contains(b.Trunks[0].Numbers, n.Number) {
				b.Trunks[0].Numbers = append(b.Trunks[0].Numbers, n.Number)
			}
		}
	}
	// Internal numbers of groups, queues and menus: what the phones dial.
	var internal []string
	for _, d := range acc.devices {
		if d.user != nil && d.context != "" && !slices.Contains(internal, d.context) {
			internal = append(internal, d.context)
		}
	}
	if len(internal) == 0 {
		internal = []string{"default", "from-internal", "internal"}
	}
	seen := map[string]bool{}
	var visit func(name string, depth int)
	visit = func(name string, depth int) {
		c := dp.context(name)
		if c == nil || seen[c.name] || depth > 6 {
			return
		}
		seen[c.name] = true
		for _, e := range c.exts {
			if !reDigits.MatchString(e.name) || b.UserByExt(e.name) != nil {
				continue
			}
			t := r.Resolve(c.name, e.name)
			if t == nil {
				continue
			}
			switch t.Kind {
			case importer.TargetQueue:
				if q := r.bQueue(t.Ref); q != nil && q.Ext == "" {
					q.Ext = e.name
				}
			case importer.TargetGroup:
				if g := r.bGroup(t.Ref); g != nil && g.Ext == "" {
					g.Ext = e.name
				}
			case importer.TargetMenu:
				if m := b.Menu(t.Ref); m != nil && m.Ext == "" {
					m.Ext = e.name
				}
			}
		}
		for _, inc := range c.includes {
			visit(inc, depth+1)
		}
	}
	for _, name := range internal {
		visit(name, 0)
	}
	// Menus reached from a number get that number.
	for _, n := range b.Numbers {
		if n.Target != nil && n.Target.Kind == importer.TargetMenu {
			if m := b.Menu(n.Target.Ref); m != nil && !slices.Contains(m.Numbers, n.Number) {
				m.Numbers = append(m.Numbers, n.Number)
			}
		}
	}
	var skipped []string
	for app := range r.skipped {
		skipped = append(skipped, app)
	}
	if len(skipped) > 0 {
		sort.Strings(skipped)
		b.Note("dialplan_skipped", "apps", strings.Join(skipped, ", "))
	}
}

// audioIndex finds the sound files the dialplan refers to.
type audioIndex map[string][]string // lower-case name without extension -> files

// formatRank orders the formats of one sound: the best quality the browser can read first.
var formatRank = []string{".wav", ".sln16", ".sln", ".mp3", ".ulaw", ".ul", ".alaw", ".al", ".gsm", ".g722"}

func indexAudio(entries []importer.Entry) audioIndex {
	idx := audioIndex{}
	for _, e := range entries {
		if !importer.IsAudio(e.Name) || e.Size > importer.MaxAudioFile {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(e.Name, path.Ext(e.Name)))
		idx[key] = append(idx[key], e.Name)
	}
	return idx
}

// find returns the best file for a sound reference such as "custom/welcome", "" when absent.
func (idx audioIndex) find(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(ref))
	ref = strings.TrimSuffix(ref, path.Ext(ref))
	if ref == "" || strings.Contains(ref, "${") {
		return ""
	}
	ref = strings.TrimPrefix(ref, "/")
	if i := strings.Index(ref, "sounds/"); i >= 0 {
		ref = ref[i+7:]
	}
	var cands []string
	var bestKey string
	for key, files := range idx {
		if key == ref || strings.HasSuffix(key, "/"+ref) {
			// Prefer the shortest path (sounds/en/custom/x over a copy deep in a backup folder).
			if bestKey == "" || len(key) < len(bestKey) {
				bestKey, cands = key, files
			}
		}
	}
	if len(cands) == 0 {
		return ""
	}
	slices.SortFunc(cands, func(a, b string) int {
		return rank(path.Ext(a)) - rank(path.Ext(b))
	})
	return cands[0]
}

func rank(ext string) int {
	if i := slices.Index(formatRank, strings.ToLower(ext)); i >= 0 {
		return i
	}
	return len(formatRank)
}

// VoicemailBox is one mailbox of voicemail.conf.
type VoicemailBox struct {
	PIN, Name, Email string
	Attach           bool
}

// Voicemail reads the mailboxes of voicemail.conf by number.
func Voicemail(data []byte) map[string]VoicemailBox {
	out := map[string]VoicemailBox{}
	for k, b := range readVoicemail(Parse("voicemail.conf", Files{"voicemail.conf": data})) {
		out[k] = VoicemailBox{PIN: b.pin, Name: b.name, Email: b.email, Attach: b.attach}
	}
	return out
}

// Sounds finds sound files in an upload by the name Asterisk uses ("custom/welcome").
type Sounds struct {
	idx  audioIndex
	Used map[string]bool
}

// NewSounds indexes the audio files of an upload.
func NewSounds(entries []importer.Entry) *Sounds {
	return &Sounds{idx: indexAudio(entries), Used: map[string]bool{}}
}

// Find returns the best file for the reference, "" when absent, and remembers it.
func (s *Sounds) Find(ref string) string {
	f := s.idx.find(ref)
	if f != "" {
		s.Used[f] = true
	}
	return f
}

// Load reads the files that were found into the bundle.
func (s *Sounds) Load(u *importer.Upload, b *importer.Bundle) error {
	if len(s.Used) == 0 {
		return nil
	}
	audio, err := u.Files(func(name string, _ int64) bool { return s.Used[name] }, importer.MaxAudioFile)
	if err != nil {
		return err
	}
	if b.Audio == nil {
		b.Audio = map[string][]byte{}
	}
	for k, v := range audio {
		b.Audio[k] = v
	}
	return nil
}

// ParseTimeSpec turns a GotoIfTime/time group spec ("08:00-17:00|mon-fri|*|*") into a time
// range; ok is false for rules with dates or months.
func ParseTimeSpec(spec string) (pbx.TimeRange, bool) {
	f := splitArgs(strings.ReplaceAll(spec, "|", ","))
	for len(f) < 4 {
		f = append(f, "*")
	}
	from, to, ok := parseTimes(f[0])
	days := parseDays(f[1])
	if !ok || days == nil || (f[2] != "*" && f[2] != "") || (f[3] != "*" && f[3] != "") {
		return pbx.TimeRange{}, false
	}
	return pbx.TimeRange{Days: days, From: from, To: to}, true
}
