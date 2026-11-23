// Package freepbx imports a FreePBX system (and the systems built on it: Issabel, Elastix,
// PBXact, older VitalPBX releases) from a backup.
//
// FreePBX 15 and later write a .tar.gz with modulejson/<Module>.json (the settings of each module)
// and mysqldump files for a few tables (devices, sip). FreePBX 2.x to 14 write a .tgz with a
// mysqldump of the whole asterisk database. Both are read into the same tables, so one mapping
// serves every version. The recordings are taken from var/lib/asterisk/sounds in the backup.
package freepbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
	"github.com/rhisijmuiden/kiestoon/server/internal/importer/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/importer/sqldump"
)

func init() { importer.Register(Adapter{}) }

// Adapter reads FreePBX backups.
type Adapter struct{}

// ID implements importer.Adapter.
func (Adapter) ID() string { return "freepbx" }

// tables are the FreePBX tables the import uses.
var tables = []string{"users", "devices", "sip", "pjsip", "trunks", "incoming", "ringgroups", "queues_config", "queues_details",
	"ivr_details", "ivr_entries", "timeconditions", "timegroups_groups", "timegroups_details", "recordings", "announcement",
	"miscdests", "freepbx_settings", "admin", "module_xml", "modules"}

func isSQL(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".sql") || strings.HasSuffix(n, ".sql.gz")
}

// Detect implements importer.Adapter.
func (Adapter) Detect(u *importer.Upload) int {
	score := 0
	for _, e := range u.Entries() {
		n := strings.ToLower(e.Name)
		switch {
		case strings.HasPrefix(n, "modulejson/") || strings.Contains(n, "/modulejson/"):
			return 95
		case strings.HasSuffix(n, "freepbx.conf"), strings.HasSuffix(n, "amportal.conf"):
			score = max(score, 60)
		case isSQL(n) && (strings.Contains(n, "mysql") || strings.Contains(n, "asterisk")):
			score = max(score, 70)
		case strings.HasSuffix(n, "extensions_additional.conf"), strings.HasSuffix(n, "issabel.conf"), strings.HasSuffix(n, "elastix.conf"):
			score = max(score, 85)
		}
	}
	if score > 0 && u.IsArchive() {
		return max(score, 85)
	}
	// One loose dump of the asterisk database.
	if !u.IsArchive() && isSQL(u.Name) {
		h := string(u.Head(4096))
		if strings.Contains(h, "asterisk") || strings.Contains(h, "MySQL dump") || strings.Contains(h, "MariaDB dump") {
			return 75
		}
	}
	return score
}

// db holds the rows of the FreePBX tables.
type db struct {
	t sqldump.Tables
	// vm are the voicemail boxes by extension.
	vm map[string]asterisk.VoicemailBox
	// version is the FreePBX version, "" when unknown.
	version string
	host    string
	legacy  bool
}

func (d *db) rows(table string) []map[string]string { return d.t[table] }

// kv returns the keyword/data rows of an id (sip, pjsip, queues_details).
func (d *db) kv(table, id string) map[string][]string {
	out := map[string][]string{}
	for _, r := range d.t[table] {
		if r["id"] == id {
			out[r["keyword"]] = append(out[r["keyword"]], r["data"])
		}
	}
	return out
}

func first(m map[string][]string, k string) string {
	if v := m[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// addJSONRows adds rows from a JSON value that is a list of objects.
func (d *db) addJSONRows(table string, v any) {
	list, ok := v.([]any)
	if !ok {
		// Some modules key their rows by id.
		if m, ok := v.(map[string]any); ok {
			for _, x := range m {
				list = append(list, x)
			}
		}
	}
	for _, x := range list {
		o, ok := x.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]string{}
		for k, val := range o {
			row[k] = jsonString(val)
		}
		d.t[table] = append(d.t[table], row)
	}
}

func jsonString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "1"
		}
		return "0"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func get(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		o, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = o[k]
	}
	return cur
}

// readModuleJSON takes the rows out of modulejson/<Module>.json.
func (d *db) readModuleJSON(module string, data []byte) {
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return
	}
	cfg, _ := m["configs"].(map[string]any)
	if cfg == nil {
		return
	}
	if v, ok := m["pbx_version"].(string); ok && d.version == "" {
		d.version = v
	}
	// Modules that dump their tables as they are.
	if tb, ok := cfg["tables"].(map[string]any); ok {
		for name, rows := range tb {
			d.addJSONRows(name, rows)
		}
	}
	switch strings.ToLower(module) {
	case "core":
		d.addJSONRows("users", get(cfg, "Users", "users"))
		d.addJSONRows("trunks", get(cfg, "Trunks", "trunks"))
		d.addJSONRows("pjsip", get(cfg, "Trunks", "techTables", "pjsip"))
		d.addJSONRows("incoming", get(cfg, "Did"))
	case "ringgroups":
		d.addJSONRows("ringgroups", cfg["groups"])
	case "ivr":
		d.addJSONRows("ivr_details", cfg["ivrs"])
		d.addJSONRows("ivr_entries", cfg["entries"])
	case "recordings":
		d.addJSONRows("recordings", cfg["data"])
	case "announcement":
		d.addJSONRows("announcement", cfg)
		if _, ok := cfg["announcement_id"]; !ok {
			for _, v := range cfg {
				d.addJSONRows("announcement", v)
			}
		}
	case "miscdests":
		for _, v := range cfg {
			d.addJSONRows("miscdests", v)
		}
	case "voicemail":
		if boxes, ok := cfg["mailboxData"].(map[string]any); ok {
			for ext, x := range boxes {
				o, _ := x.(map[string]any)
				if o == nil || !strings.EqualFold(jsonString(o["voicemail_enable"]), "yes") {
					continue
				}
				d.vm[ext] = asterisk.VoicemailBox{PIN: jsonString(o["voicemail_vmpwd"]), Email: jsonString(o["voicemail_email"]),
					Attach: strings.Contains(strings.ToLower(jsonString(o["voicemail_options"])), "attach=yes")}
			}
		}
		if conf, ok := cfg["voicemailConf"].(map[string]any); ok {
			for _, ctxBoxes := range conf {
				boxes, _ := ctxBoxes.(map[string]any)
				for ext, x := range boxes {
					o, _ := x.(map[string]any)
					if o == nil || !reExt.MatchString(ext) {
						continue
					}
					if _, ok := d.vm[ext]; ok {
						continue
					}
					d.vm[ext] = asterisk.VoicemailBox{PIN: jsonString(o["pwd"]), Name: jsonString(o["name"]), Email: jsonString(o["email"])}
				}
			}
		}
	}
}

var reExt = regexp.MustCompile(`^[0-9]{2,8}$`)

func (d *db) readSQL(name string, data []byte) error {
	var r io.Reader = bytes.NewReader(data)
	if strings.HasSuffix(strings.ToLower(name), ".gz") {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil
		}
		r = gz
	}
	t, err := sqldump.Read(r, func(table string) bool { return slices.Contains(tables, table) })
	for k, v := range t {
		d.t[k] = append(d.t[k], v...)
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		return importer.ErrDamaged
	}
	return nil
}

// load reads the tables from an upload (and from archives inside it, one level deep).
func load(u *importer.Upload, d *db) error {
	files, err := u.Files(func(name string, _ int64) bool {
		n := strings.ToLower(name)
		return isSQL(n) || strings.Contains(n, "modulejson/") && strings.HasSuffix(n, ".json") || path.Base(n) == "metadata.json" ||
			path.Base(n) == "voicemail.conf"
	}, importer.MaxFile)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		data := files[n]
		switch {
		case path.Base(n) == "metadata.json":
			var m map[string]any
			if json.Unmarshal(data, &m) == nil {
				for _, k := range []string{"pbx_version", "freepbx_version", "version"} {
					if v, ok := m[k].(string); ok && d.version == "" && strings.ContainsAny(v, "0123456789") {
						d.version = v
					}
				}
				if v, ok := m["hostname"].(string); ok {
					d.host = v
				}
			}
		case strings.HasSuffix(strings.ToLower(n), ".json"):
			d.readModuleJSON(strings.TrimSuffix(path.Base(n), ".json"), data)
		case path.Base(n) == "voicemail.conf":
			for k, v := range asterisk.Voicemail(data) {
				if _, ok := d.vm[k]; !ok {
					d.vm[k] = v
				}
			}
		default:
			if err := d.readSQL(n, data); err != nil {
				return err
			}
			d.legacy = d.legacy || !strings.Contains(strings.ToLower(n), "modulejson")
		}
	}
	if len(d.t["users"]) == 0 && len(d.t["devices"]) == 0 {
		// Issabel and some FreePBX backups put the database in an archive inside the backup.
		for _, e := range u.Entries() {
			n := strings.ToLower(e.Name)
			if strings.HasSuffix(n, ".tar") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".tar.gz") {
				inner, err := u.Nested(e.Name)
				if err != nil {
					continue
				}
				err = load(inner, d)
				inner.Close()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Read implements importer.Adapter.
func (Adapter) Read(ctx context.Context, u *importer.Upload) (*importer.Bundle, error) {
	d := &db{t: sqldump.Tables{}, vm: map[string]asterisk.VoicemailBox{}}
	if err := load(u, d); err != nil {
		return nil, err
	}
	if len(d.t["users"]) == 0 && len(d.t["devices"]) == 0 && len(d.t["ringgroups"]) == 0 {
		return nil, importer.ErrUnknownFormat
	}
	b := &importer.Bundle{Info: importer.Info{Product: "FreePBX", Host: d.host}}
	for _, r := range d.rows("freepbx_settings") {
		if r["keyword"] == "FREEPBX_SYSTEM_IDENT" && b.Info.Host == "" {
			b.Info.Host = r["value"]
		}
	}
	if d.version == "" {
		for _, r := range d.rows("admin") {
			if r["variable"] == "version" {
				d.version = r["value"]
			}
		}
	}
	if d.version != "" {
		b.Info.Product = "FreePBX " + d.version
	}
	sounds := asterisk.NewSounds(u.Entries())
	m := &mapper{d: d, b: b, sounds: sounds, recordings: map[string]string{}}
	m.build()
	if err := sounds.Load(u, b); err != nil {
		return nil, err
	}
	if d.legacy && len(d.t["users"]) > 0 && !strings.HasPrefix(d.version, "15") && !strings.HasPrefix(d.version, "16") && !strings.HasPrefix(d.version, "17") {
		b.Note("freepbx_legacy")
	}
	b.Note("feature_codes")
	return b, nil
}

type mapper struct {
	d          *db
	b          *importer.Bundle
	sounds     *asterisk.Sounds
	recordings map[string]string // recording id -> first audio file
	unknown    map[string]bool
}

func (m *mapper) build() {
	m.unknown = map[string]bool{}
	m.readRecordings()
	m.readUsers()
	m.readTrunks()
	m.readGroups()
	m.readQueues()
	m.readTimeConditions()
	m.readMenus()
	m.readAnnouncements()
	m.readIncoming()
	if len(m.unknown) > 0 {
		var list []string
		for k := range m.unknown {
			list = append(list, k)
		}
		slices.Sort(list)
		m.b.Note("dialplan_skipped", "apps", strings.Join(list, ", "))
	}
}

func (m *mapper) readRecordings() {
	for _, r := range m.d.rows("recordings") {
		files := r["filename"]
		if files == "" {
			files = r["files"]
		}
		for _, f := range strings.Split(files, "&") {
			if a := m.sounds.Find(f); a != "" {
				m.recordings[r["id"]] = a
				break
			}
		}
	}
}

func (m *mapper) readUsers() {
	devices := map[string]map[string]string{}
	for _, r := range m.d.rows("devices") {
		devices[r["id"]] = r
	}
	users := m.d.rows("users")
	if len(users) == 0 {
		// Only the device table: one user per device.
		for _, r := range m.d.rows("devices") {
			users = append(users, map[string]string{"extension": r["id"], "name": r["description"], "voicemail": "default"})
		}
	}
	for _, r := range users {
		ext := r["extension"]
		if ext == "" {
			continue
		}
		u := importer.User{Ref: ext, Ext: ext, Name: r["name"], OutboundCID: cidNumber(r["outboundcid"])}
		sip := m.d.kv("sip", ext)
		u.SIPPassword = first(sip, "secret")
		if dev := devices[ext]; dev != nil && u.Name == "" {
			u.Name = dev["description"]
		}
		if vm := r["voicemail"]; vm != "" && vm != "novm" {
			u.Voicemail = true
		}
		if box, ok := m.d.vm[ext]; ok {
			u.Voicemail = true
			u.VoicemailPIN, u.Email, u.VoicemailEmail = box.PIN, box.Email, box.Attach || box.Email != ""
		}
		m.b.Users = append(m.b.Users, u)
	}
}

var reCID = regexp.MustCompile(`<\s*(\+?[0-9]{3,20})\s*>|^\s*(\+?[0-9]{3,20})\s*$`)

func cidNumber(s string) string {
	if x := reCID.FindStringSubmatch(s); x != nil {
		return x[1] + x[2]
	}
	return ""
}

func (m *mapper) readTrunks() {
	for _, r := range m.d.rows("trunks") {
		if r["disabled"] == "on" {
			continue
		}
		id := r["trunkid"]
		t := importer.Trunk{Ref: id, Name: r["name"], CallerID: cidNumber(r["outcid"])}
		switch strings.ToLower(r["tech"]) {
		case "pjsip":
			kv := m.d.kv("pjsip", id)
			t.Host = first(kv, "sip_server")
			t.Port, _ = strconv.Atoi(first(kv, "sip_server_port"))
			t.Username = first(kv, "username")
			t.AuthUsername = first(kv, "auth_username")
			t.Password = first(kv, "secret")
			t.Register = first(kv, "registration") == "send"
			t.FromDomain, t.FromUser = first(kv, "from_domain"), first(kv, "from_user")
			tr := first(kv, "transport")
			t.Transport = "udp"
			if strings.Contains(tr, "tcp") {
				t.Transport = "tcp"
			} else if strings.Contains(tr, "tls") {
				t.Transport = "tls"
			}
			t.Codecs = cleanCodecs(first(kv, "codecs"))
			if !t.Register && t.Host != "" {
				t.Networks = append(t.Networks, t.Host)
			}
			if t.Username == t.AuthUsername {
				t.AuthUsername = ""
			}
		case "sip":
			peer := m.d.kv("sip", "tr-peer-"+id)
			t.Host = first(peer, "host")
			t.Port, _ = strconv.Atoi(first(peer, "port"))
			t.Username = first(peer, "username")
			if t.Username == "" {
				t.Username = first(peer, "defaultuser")
			}
			t.Password = first(peer, "secret")
			t.FromDomain, t.FromUser = first(peer, "fromdomain"), first(peer, "fromuser")
			t.Codecs = cleanCodecs(strings.Join(peer["allow"], ","))
			if reg := first(m.d.kv("sip", "tr-reg-"+id), "register"); reg != "" {
				t.Register = true
				user, rest, _ := strings.Cut(reg, "@")
				u, pw, _ := strings.Cut(user, ":")
				if t.Username == "" {
					t.Username = u
				}
				if t.Password == "" {
					pw, _, _ = strings.Cut(pw, ":")
					t.Password = pw
				}
				if t.Host == "" {
					t.Host, _, _ = strings.Cut(strings.Split(rest, "/")[0], ":")
				}
			} else if t.Host != "" && t.Host != "dynamic" {
				t.Networks = append(t.Networks, t.Host)
			}
		default:
			continue // DAHDI, IAX and custom trunks have no Kiestoon equivalent
		}
		if t.Host == "" {
			continue
		}
		m.b.Trunks = append(m.b.Trunks, t)
	}
}

func cleanCodecs(s string) string {
	var out []string
	for _, c := range strings.Split(s, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		switch c {
		case "alaw", "ulaw", "g722", "g729", "opus", "gsm":
			if !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return strings.Join(out, ",")
}

// dest turns a FreePBX destination ("ext-group,600,1") into a target.
func (m *mapper) dest(s string) *importer.Target {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	ctx := parts[0]
	ext := ""
	if len(parts) > 1 {
		ext = parts[1]
	}
	switch {
	case ctx == "from-did-direct" || ctx == "ext-local" && reExt.MatchString(ext) || ctx == "from-internal" && m.b.User(ext) != nil:
		if m.b.User(ext) != nil {
			return &importer.Target{Kind: importer.TargetUser, Ref: ext}
		}
	case ctx == "ext-local" && len(ext) > 3 && strings.HasPrefix(ext, "vm"):
		box := strings.TrimLeft(ext[2:], "bsuiBSUI")
		if m.b.User(box) != nil {
			return &importer.Target{Kind: importer.TargetVoicemail, Ref: box}
		}
	case ctx == "ext-group":
		return &importer.Target{Kind: importer.TargetGroup, Ref: ext}
	case ctx == "ext-queues":
		return &importer.Target{Kind: importer.TargetQueue, Ref: ext}
	case strings.HasPrefix(ctx, "ivr-"):
		return &importer.Target{Kind: importer.TargetMenu, Ref: ctx}
	case ctx == "timeconditions":
		return &importer.Target{Kind: importer.TargetTime, Ref: ext}
	case strings.HasPrefix(ctx, "app-announcement-"):
		return &importer.Target{Kind: importer.TargetMenu, Ref: "ann-" + strings.TrimPrefix(ctx, "app-announcement-")}
	case ctx == "ext-miscdests":
		for _, r := range m.d.rows("miscdests") {
			if r["id"] == ext {
				num := strings.TrimSpace(r["destdial"])
				if m.b.User(num) != nil {
					return &importer.Target{Kind: importer.TargetUser, Ref: num}
				}
				if regexp.MustCompile(`^\+?[0-9]{3,20}$`).MatchString(num) {
					return &importer.Target{Kind: importer.TargetExternal, Ref: num}
				}
			}
		}
	case ctx == "app-blackhole":
		return nil
	}
	m.unknown[strings.TrimSuffix(strings.TrimRight(ctx, "0123456789"), "-")] = true
	return nil
}

func (m *mapper) readGroups() {
	for _, r := range m.d.rows("ringgroups") {
		g := importer.Group{Ref: r["grpnum"], Ext: r["grpnum"], Name: r["description"], RingSeconds: atoi(r["grptime"]), Failover: m.dest(r["postdest"])}
		switch strings.ToLower(r["strategy"]) {
		case "hunt", "hunt-prim", "memoryhunt", "memoryhunt-prim", "firstavailable", "firstnotonphone":
			g.Strategy = "sequential"
		case "random":
			g.Strategy = "rotating"
		default:
			g.Strategy = "simultaneous"
		}
		for _, x := range strings.FieldsFunc(r["grplist"], func(r rune) bool { return r == '-' || r == '\n' || r == ',' }) {
			x = strings.TrimSpace(x)
			switch {
			case strings.HasSuffix(x, "#"):
				g.External = append(g.External, strings.TrimSuffix(x, "#"))
			case m.b.User(x) != nil:
				g.Members = append(g.Members, x)
			}
		}
		m.b.Groups = append(m.b.Groups, g)
	}
}

var reMember = regexp.MustCompile(`(?i)^(?:local|pjsip|sip)/([0-9]+)`)

func (m *mapper) readQueues() {
	for _, r := range m.d.rows("queues_config") {
		ext := r["extension"]
		kv := m.d.kv("queues_details", ext)
		q := importer.Queue{Ref: ext, Ext: ext, Name: r["descr"], RingTime: atoi(first(kv, "timeout")), WrapUp: atoi(first(kv, "wrapuptime")),
			MaxSize: atoi(first(kv, "maxlen")), MaxWait: atoi(r["maxwait"]), Overflow: m.dest(r["dest"]), MusicClass: first(kv, "music")}
		switch s := strings.ToLower(first(kv, "strategy")); s {
		case "ringall", "leastrecent", "fewestcalls", "random", "rrmemory", "linear", "wrandom":
			q.Strategy = s
		case "rrordered":
			q.Strategy = "rrmemory"
		default:
			q.Strategy = "ringall"
		}
		for _, mem := range kv["member"] {
			if x := reMember.FindStringSubmatch(mem); x != nil && m.b.User(x[1]) != nil && !slices.Contains(q.Agents, x[1]) {
				q.Agents = append(q.Agents, x[1])
			}
		}
		if id := r["joinannounce_id"]; id != "" && id != "0" {
			q.Greeting = m.recordings[id]
		}
		m.b.Queues = append(m.b.Queues, q)
	}
}

func (m *mapper) readTimeConditions() {
	groups := map[string]string{}
	for _, g := range m.d.rows("timegroups_groups") {
		groups[g["id"]] = g["description"]
	}
	for _, r := range m.d.rows("timeconditions") {
		id := r["timeconditions_id"]
		tc := importer.TimeCondition{Ref: id, Name: r["displayname"], Open: m.dest(r["truegoto"]), Closed: m.dest(r["falsegoto"])}
		if tc.Name == "" {
			tc.Name = groups[r["time"]]
		}
		for _, d := range m.d.rows("timegroups_details") {
			if d["timegroupid"] != r["time"] {
				continue
			}
			if tr, ok := asterisk.ParseTimeSpec(d["time"]); ok {
				tc.Schedule = append(tc.Schedule, tr)
			} else {
				m.b.Note("time_rule_skipped", "rule", d["time"])
			}
		}
		m.b.TimeConditions = append(m.b.TimeConditions, tc)
	}
}

func (m *mapper) readMenus() {
	for _, r := range m.d.rows("ivr_details") {
		id := r["id"]
		s := &importer.Step{Ref: "ivr-" + id, Type: "root", Title: r["name"], Audio: m.recordings[r["announcement"]]}
		if s.Title == "" {
			s.Title = "IVR " + id
		}
		var entries []map[string]string
		for _, e := range m.d.rows("ivr_entries") {
			if e["ivr_id"] == id {
				entries = append(entries, e)
			}
		}
		slices.SortFunc(entries, func(a, b map[string]string) int { return strings.Compare(a["selection"], b["selection"]) })
		for _, e := range entries {
			key := e["selection"]
			if len(key) != 1 {
				continue
			}
			s.Children = append(s.Children, &importer.Step{Type: "menu", Key: key, Title: key, Target: m.dest(e["dest"])})
		}
		if t := m.dest(r["timeout_destination"]); t != nil {
			s.NoAction = t
		} else {
			s.NoAction = m.dest(r["invalid_destination"])
		}
		m.b.Menus = append(m.b.Menus, s)
	}
}

func (m *mapper) readAnnouncements() {
	for _, r := range m.d.rows("announcement") {
		id := r["announcement_id"]
		if id == "" {
			continue
		}
		s := &importer.Step{Ref: "ann-" + id, Type: "root", Title: r["description"], Audio: m.recordings[r["recording_id"]], Target: m.dest(r["post_dest"])}
		if s.Title == "" {
			s.Title = fmt.Sprintf("Announcement %s", id)
		}
		m.b.Menus = append(m.b.Menus, s)
	}
}

func (m *mapper) readIncoming() {
	catchAll := false
	for _, r := range m.d.rows("incoming") {
		did := strings.TrimSpace(r["extension"])
		if did == "" || strings.HasPrefix(did, "_") {
			catchAll = true
			continue
		}
		if r["cidnum"] != "" {
			continue // routes per caller id have no Kiestoon equivalent
		}
		t := m.dest(r["destination"])
		m.b.Numbers = append(m.b.Numbers, importer.Number{Number: did, Target: t})
		if t != nil && t.Kind == importer.TargetMenu {
			if s := m.b.Menu(t.Ref); s != nil {
				s.Numbers = append(s.Numbers, did)
			}
		}
	}
	if catchAll {
		m.b.Note("catch_all_inbound")
	}
	// One trunk: its numbers are the inbound numbers.
	if len(m.b.Trunks) == 1 {
		for _, n := range m.b.Numbers {
			m.b.Trunks[0].Numbers = append(m.b.Trunks[0].Numbers, n.Number)
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
