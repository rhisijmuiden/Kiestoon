// Package threecx imports a 3CX phone system (version 16, 18 and 20).
//
// 3CX does not document the layout of its backups. A backup is a zip with the configuration
// (an XML file and database table files) and the folders httpprompts (voice menu and queue
// recordings) and vmailprompts. When the backup has a password, the configuration inside the zip
// is encrypted and only 3CX itself can read it.
//
// The adapter therefore reads defensively: it looks for the 3CX objects (extensions, ring groups,
// queues, digital receptionists, trunks) by element and column names in every XML and CSV file
// of the upload, and takes the recordings by file name. The extension list that 3CX exports as
// CSV (Users > Export) can be uploaded as well, on its own or next to the backup.
package threecx

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
)

func init() { importer.Register(Adapter{}) }

// ErrEncrypted is returned when the configuration of the backup is encrypted.
var ErrEncrypted = errors.New("encrypted_backup")

// Adapter reads 3CX backups and extension exports.
type Adapter struct{}

// ID implements importer.Adapter.
func (Adapter) ID() string { return "3cx" }

// Detect implements importer.Adapter.
func (Adapter) Detect(u *importer.Upload) int {
	score := 0
	for _, e := range u.Entries() {
		n := strings.ToLower(e.Name)
		switch {
		case strings.HasPrefix(n, "httpprompts/") || strings.Contains(n, "/httpprompts/"),
			strings.HasPrefix(n, "vmailprompts/") || strings.Contains(n, "/vmailprompts/"):
			return 90
		case strings.Contains(n, "3cx"):
			score = max(score, 60)
		}
	}
	if !u.IsArchive() {
		h := strings.ToLower(string(u.Head(2048)))
		if strings.HasSuffix(strings.ToLower(u.Name), ".csv") && userHeader(firstLine(h)) {
			return 70
		}
		if strings.Contains(h, "3cx") {
			score = max(score, 50)
		}
	}
	return score
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// userHeader reports whether a CSV header looks like the 3CX extension export.
func userHeader(h string) bool {
	h = strings.ToLower(h)
	return strings.Contains(h, "number") && (strings.Contains(h, "firstname") || strings.Contains(h, "first name") || strings.Contains(h, "lastname"))
}

func isConfig(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".csv") || strings.HasSuffix(n, ".json") ||
		strings.HasSuffix(n, ".txt") || strings.HasSuffix(n, ".dat")
}

// Read implements importer.Adapter.
func (Adapter) Read(ctx context.Context, u *importer.Upload) (*importer.Bundle, error) {
	files, err := u.Files(func(n string, _ int64) bool { return isConfig(n) || importer.IsAudio(n) }, importer.MaxFile)
	if err != nil {
		return nil, err
	}
	if !u.IsArchive() {
		// A single file arrives under the upload's own name.
		for k, v := range files {
			delete(files, k)
			files[u.Name] = v
		}
	}
	b := &importer.Bundle{Info: importer.Info{System: "3cx", Product: "3CX"}, Audio: map[string][]byte{}}
	r := &reader{b: b, users: map[string]int{}, byNumber: map[string]importer.Target{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	encrypted, configs := false, 0
	for _, n := range names {
		data := files[n]
		lower := strings.ToLower(n)
		switch {
		case importer.IsAudio(n):
			if inPrompts(lower) {
				b.Audio[path.Base(n)] = data
			}
		case strings.HasSuffix(lower, ".xml"):
			if looksEncrypted(data) {
				encrypted = true
				continue
			}
			configs++
			r.readXML(parseXML(data))
		case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".dat"):
			if looksEncrypted(data) {
				encrypted = encrypted || strings.HasSuffix(lower, ".csv")
				continue
			}
			if r.readCSV(path.Base(lower), data) {
				configs++
			}
		}
	}
	r.finish()
	if encrypted {
		if len(b.Users) == 0 {
			return nil, ErrEncrypted
		}
		b.Notes = append(b.Notes, importer.NewNote("encrypted_backup"))
	}
	if configs == 0 && len(b.Users) == 0 {
		b.Notes = append(b.Notes, importer.NewNote("no_database"))
	}
	if r.version != "" {
		b.Info.Product = "3CX " + r.version
	}
	b.Notes = append(b.Notes, importer.NewNote("threecx_partial"))
	return b, nil
}

func inPrompts(n string) bool {
	return strings.Contains(n, "httpprompts/") || strings.Contains(n, "prompts/") || !strings.Contains(n, "/")
}

// looksEncrypted reports whether a configuration file is not text.
func looksEncrypted(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if len(b) == 0 {
		return false
	}
	if len(b) > 4096 {
		b = b[:4096]
	}
	if !utf8.Valid(b) {
		return true
	}
	bad := 0
	for _, c := range b {
		if c < 9 || (c > 13 && c < 32) {
			bad++
		}
	}
	return bad*20 > len(b)
}

// ---- XML ---------------------------------------------------------------------

type node struct {
	Name     string
	Attr     map[string]string // lower-case keys
	Text     string
	Children []*node
}

func parseXML(data []byte) *node {
	root := &node{Name: "#root", Attr: map[string]string{}}
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{Name: strings.ToLower(t.Name.Local), Attr: map[string]string{}}
			for _, a := range t.Attr {
				n.Attr[strings.ToLower(a.Name.Local)] = a.Value
			}
			p := stack[len(stack)-1]
			p.Children = append(p.Children, n)
			stack = append(stack, n)
		case xml.CharData:
			stack[len(stack)-1].Text += string(t)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return root
}

// get returns the first of the fields (attribute or child element) that has a value.
func (n *node) get(keys ...string) string {
	for _, k := range keys {
		k = strings.ToLower(k)
		if v := strings.TrimSpace(n.Attr[k]); v != "" {
			return v
		}
		for _, c := range n.Children {
			if c.Name == k && len(c.Children) == 0 {
				if v := strings.TrimSpace(c.Text); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func (n *node) walk(fn func(*node) bool) {
	for _, c := range n.Children {
		if fn(c) {
			c.walk(fn)
		}
	}
}

// fields returns the leaf values of n (attributes and childless elements) as a lower-case map.
func (n *node) fields() map[string]string {
	out := map[string]string{}
	for k, v := range n.Attr {
		out[k] = v
	}
	for _, c := range n.Children {
		if len(c.Children) == 0 {
			if _, ok := out[c.Name]; !ok {
				out[c.Name] = strings.TrimSpace(c.Text)
			}
		}
	}
	return out
}

// ---- reading -----------------------------------------------------------------

type reader struct {
	b        *importer.Bundle
	users    map[string]int // extension number -> index in b.Users
	byNumber map[string]importer.Target
	version  string
	// pending destinations resolved once every object is known.
	groups []pendingGroup
	queues []pendingQueue
	menus  []pendingMenu
	routes []pendingRoute
}

type pendingGroup struct {
	g       importer.Group
	members []string
	noAns   map[string]string
}

type pendingQueue struct {
	q      importer.Queue
	agents []string
	noAns  map[string]string
}

type pendingMenu struct {
	ext, name, prompt string
	keys              map[string]map[string]string
	timeout           map[string]string
}

type pendingRoute struct {
	number string
	dest   map[string]string
}

var reExt = regexp.MustCompile(`^[0-9]{2,8}$`)

func (r *reader) addUser(f map[string]string) {
	num := pick(f, "number", "extension", "extensionnumber", "dn", "ext")
	if !reExt.MatchString(num) {
		return
	}
	name := strings.TrimSpace(pick(f, "firstname", "first name") + " " + pick(f, "lastname", "last name"))
	if name == "" {
		name = pick(f, "name", "displayname", "fullname")
	}
	if name == "" {
		name = num
	}
	u := importer.User{
		Ref:          num,
		Ext:          num,
		Name:         name,
		Email:        pick(f, "emailaddress", "email", "email address"),
		SIPPassword:  pick(f, "authpassword", "auth password", "sippassword", "password"),
		VoicemailPIN: pick(f, "vmpin", "voicemailpin", "pin", "voicemail pin"),
		OutboundCID:  digits(pick(f, "outboundcallerid", "outbound caller id", "callerid")),
	}
	u.Voicemail = !isFalse(pick(f, "vmenabled", "voicemailenabled", "voicemail"))
	u.VoicemailEmail = u.Email != "" && isTrue(pick(f, "vmemailoptions", "vmemail", "voicemailemail", "sendvmtoemail"))
	if v := pick(f, "enabled"); v != "" {
		u.Disabled = isFalse(v)
	}
	if v := pick(f, "disabled"); v != "" {
		u.Disabled = isTrue(v)
	}
	if i, ok := r.users[num]; ok {
		// Several files describe the same extension: fill the gaps.
		x := &r.b.Users[i]
		if x.Name == x.Ext && u.Name != num {
			x.Name = u.Name
		}
		x.Email = first(x.Email, u.Email)
		x.SIPPassword = first(x.SIPPassword, u.SIPPassword)
		x.VoicemailPIN = first(x.VoicemailPIN, u.VoicemailPIN)
		x.OutboundCID = first(x.OutboundCID, u.OutboundCID)
		return
	}
	r.users[num] = len(r.b.Users)
	r.b.Users = append(r.b.Users, u)
	r.byNumber[num] = importer.Target{Kind: importer.TargetUser, Ref: num}
}

func (r *reader) readXML(root *node) {
	if v := root.get("version"); v != "" && r.version == "" {
		r.version = v
	}
	root.walk(func(n *node) bool {
		if v := n.Attr["version"]; v != "" && r.version == "" {
			r.version = v
		}
		switch n.Name {
		case "version", "productversion":
			if r.version == "" && len(n.Children) == 0 {
				r.version = strings.TrimSpace(n.Text)
			}
		case "extension", "user":
			r.addUser(n.fields())
			return false
		case "ringgroup":
			r.groups = append(r.groups, pendingGroup{g: importer.Group{
				Ref: n.get("number"), Ext: n.get("number"), Name: n.get("name"),
				Strategy:    groupStrategy(n.get("ringstrategy", "strategy")),
				RingSeconds: atoi(n.get("ringtime", "ringtimeout")),
			}, members: memberNumbers(n), noAns: destFields(n, "noanswer", "forward", "destination")})
			return false
		case "callqueue", "queue":
			r.queues = append(r.queues, pendingQueue{q: importer.Queue{
				Ref: n.get("number"), Ext: n.get("number"), Name: n.get("name"),
				Strategy: queueStrategy(n.get("pollingstrategy", "ringstrategy", "strategy")),
				MaxWait:  atoi(n.get("masterTimeout", "maxwaittime", "maxcallerwaittime")),
				RingTime: atoi(n.get("ringtimeout", "ringtime")),
				WrapUp:   atoi(n.get("wrapuptime", "wrapup")),
				Greeting: promptFile(n.get("introfile", "greetingfile", "onholdfile")),
			}, agents: memberNumbers(n), noAns: destFields(n, "noanswer", "forward", "destination")})
			return false
		case "ivr", "digitalreceptionist", "receptionist":
			m := pendingMenu{ext: n.get("number"), name: n.get("name"), prompt: promptFile(n.get("promptfilename", "prompt", "filename")),
				keys: map[string]map[string]string{}, timeout: destFields(n, "timeout", "noanswer", "invalid")}
			n.walk(func(c *node) bool {
				if c.Name == "forward" || c.Name == "ivrforward" || c.Name == "option" {
					if k := c.get("input", "key", "number", "digit"); len(k) == 1 {
						m.keys[k] = c.fields()
					}
					return false
				}
				return true
			})
			r.menus = append(r.menus, m)
			return false
		case "did", "inboundrule", "inboundrule2":
			if num := digits(n.get("number", "did", "mask")); len(num) >= 6 {
				dest := destFields(n, "destination", "forward", "office", "route")
				if dest == nil {
					dest = n.fields()
					delete(dest, "number")
					delete(dest, "did")
					delete(dest, "mask")
				}
				r.routes = append(r.routes, pendingRoute{number: num, dest: dest})
			}
			return false
		case "gateway", "voipprovider", "siptrunk", "trunk":
			r.addTrunk(n.fields())
			return false
		}
		return true
	})
}

func (r *reader) addTrunk(f map[string]string) {
	host := pick(f, "host", "registrarhost", "proxyhost", "server", "sipserver")
	if host == "" {
		return
	}
	t := importer.Trunk{
		Ref:          first(pick(f, "id", "number", "name"), host),
		Name:         first(pick(f, "name"), host),
		Host:         host,
		Port:         atoi(pick(f, "port", "registrarport")),
		Username:     pick(f, "authid", "username", "user"),
		AuthUsername: pick(f, "authid", "authusername"),
		Password:     pick(f, "authpassword", "password"),
		Register:     !isFalse(pick(f, "requireregistration", "register")),
	}
	for _, x := range r.b.Trunks {
		if x.Ref == t.Ref {
			return
		}
	}
	if t.Username == "" {
		t.Register = false
	}
	r.b.Trunks = append(r.b.Trunks, t)
}

// readCSV reads a 3CX extension export or database table export. It reports whether the file
// held anything 3CX.
func (r *reader) readCSV(name string, data []byte) bool {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	head := strings.ToLower(firstLine(string(data)))
	sep := ','
	if strings.Count(head, ";") > strings.Count(head, ",") {
		sep = ';'
	} else if strings.Count(head, "\t") > strings.Count(head, ",") {
		sep = '\t'
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = sep
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil || len(rows) < 2 {
		return false
	}
	cols := make([]string, len(rows[0]))
	for i, c := range rows[0] {
		cols[i] = strings.ToLower(strings.TrimSpace(c))
	}
	if !userHeader(strings.Join(cols, ",")) && !strings.Contains(name, "extension") {
		return false
	}
	n := 0
	for _, row := range rows[1:] {
		f := map[string]string{}
		for i, v := range row {
			if i < len(cols) {
				f[cols[i]] = strings.TrimSpace(v)
			}
		}
		before := len(r.b.Users)
		r.addUser(f)
		if len(r.b.Users) > before {
			n++
		}
	}
	return n > 0
}

// finish resolves the ring groups, queues, menus and routes now that every number is known.
func (r *reader) finish() {
	for _, p := range r.groups {
		if p.g.Ref == "" {
			continue
		}
		r.byNumber[p.g.Ref] = importer.Target{Kind: importer.TargetGroup, Ref: p.g.Ref}
	}
	for _, p := range r.queues {
		if p.q.Ref == "" {
			continue
		}
		r.byNumber[p.q.Ref] = importer.Target{Kind: importer.TargetQueue, Ref: p.q.Ref}
	}
	for _, m := range r.menus {
		if m.ext == "" {
			continue
		}
		r.byNumber[m.ext] = importer.Target{Kind: importer.TargetMenu, Ref: "ivr-" + m.ext}
	}
	for _, p := range r.groups {
		if p.g.Ref == "" {
			continue
		}
		g := p.g
		g.Name = first(g.Name, "Ring group "+g.Ext)
		for _, m := range p.members {
			if _, ok := r.users[m]; ok {
				g.Members = append(g.Members, m)
			} else if len(m) >= 6 {
				g.External = append(g.External, m)
			}
		}
		g.Failover = r.target(p.noAns)
		r.b.Groups = append(r.b.Groups, g)
	}
	for _, p := range r.queues {
		if p.q.Ref == "" {
			continue
		}
		q := p.q
		q.Name = first(q.Name, "Queue "+q.Ext)
		for _, m := range p.agents {
			if _, ok := r.users[m]; ok {
				q.Agents = append(q.Agents, m)
			}
		}
		q.Overflow = r.target(p.noAns)
		if q.Greeting != "" && r.b.Audio[q.Greeting] == nil {
			q.Greeting = ""
		}
		r.b.Queues = append(r.b.Queues, q)
	}
	for _, m := range r.menus {
		if m.ext == "" {
			continue
		}
		root := &importer.Step{Ref: "ivr-" + m.ext, Type: "root", Title: first(m.name, "Menu "+m.ext), Ext: m.ext}
		if r.b.Audio[m.prompt] != nil {
			root.Audio = m.prompt
		}
		keys := make([]string, 0, len(m.keys))
		for k := range m.keys {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if t := r.target(m.keys[k]); t != nil {
				root.Children = append(root.Children, &importer.Step{Type: "menu", Key: k, Title: k, Target: t})
			}
		}
		root.NoAction = r.target(m.timeout)
		r.b.Menus = append(r.b.Menus, root)
	}
	for _, p := range r.routes {
		r.b.Numbers = append(r.b.Numbers, importer.Number{Number: p.number, Target: r.target(p.dest)})
	}
}

// target turns 3CX destination fields (a type and a number) into a Target.
func (r *reader) target(f map[string]string) *importer.Target {
	if len(f) == 0 {
		return nil
	}
	kind := strings.ToLower(pick(f, "type", "forwardtype", "destinationtype", "action", "noanswertype", "timeouttype"))
	num := digits(pick(f, "to", "dn", "destination", "forwardto", "number", "forwarddn", "noanswerdestination", "timeoutdestination", "external", "externalnumber"))
	switch {
	case strings.Contains(kind, "end") || strings.Contains(kind, "hangup") || strings.Contains(kind, "disconnect"):
		return &importer.Target{Kind: importer.TargetHangup}
	case strings.Contains(kind, "voicemail") || strings.Contains(kind, "vmail"):
		if _, ok := r.users[num]; ok {
			return &importer.Target{Kind: importer.TargetVoicemail, Ref: num}
		}
	case strings.Contains(kind, "external") || strings.Contains(kind, "outside"):
		if len(num) >= 6 {
			return &importer.Target{Kind: importer.TargetExternal, Ref: num}
		}
	}
	if t, ok := r.byNumber[num]; ok {
		return &t
	}
	if len(num) >= 6 {
		return &importer.Target{Kind: importer.TargetExternal, Ref: num}
	}
	return nil
}

// destFields returns the fields of the first child element whose name contains one of the words.
func destFields(n *node, words ...string) map[string]string {
	for _, c := range n.Children {
		for _, w := range words {
			if strings.Contains(c.Name, w) && (len(c.Children) > 0 || len(c.Attr) > 0) {
				return c.fields()
			}
		}
	}
	// Flat form: NoAnswerType="..." NoAnswerDestination="...".
	f := n.fields()
	out := map[string]string{}
	for k, v := range f {
		for _, w := range words {
			if strings.HasPrefix(k, w) {
				out[strings.TrimPrefix(k, w)] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// memberNumbers returns the extension numbers of the members (or agents) below n.
func memberNumbers(n *node) []string {
	var out []string
	n.walk(func(c *node) bool {
		switch c.Name {
		case "member", "agent", "ringgroupmember", "queueagent", "dn":
			v := c.get("number", "dn", "extension")
			if v == "" && len(c.Children) == 0 {
				v = strings.TrimSpace(c.Text)
			}
			if v = digits(v); v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
			return false
		}
		return true
	})
	return out
}

func promptFile(s string) string {
	s = strings.ReplaceAll(s, `\`, "/")
	return path.Base(strings.TrimSpace(s))
}

func groupStrategy(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "hunt") || strings.Contains(s, "priority") || strings.Contains(s, "sequen"):
		return "sequential"
	case strings.Contains(s, "paging"):
		return "paging"
	case strings.Contains(s, "round") || strings.Contains(s, "rotat"):
		return "rotating"
	}
	return "simultaneous"
}

func queueStrategy(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "ringall") || strings.Contains(s, "ring all") || strings.Contains(s, "simultan"):
		return "ringall"
	case strings.Contains(s, "hunt") || strings.Contains(s, "priority"):
		return "linear"
	case strings.Contains(s, "random"):
		return "random"
	case strings.Contains(s, "least") || strings.Contains(s, "longest"):
		return "leastrecent"
	case strings.Contains(s, "fewest"):
		return "fewestcalls"
	}
	return "rrmemory"
}

func pick(f map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(f[k]); v != "" {
			return v
		}
	}
	return ""
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func digits(s string) string {
	var b strings.Builder
	for i, c := range strings.TrimSpace(s) {
		if c >= '0' && c <= '9' || (c == '+' && i == 0) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true
	}
	return false
}

func isFalse(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false", "no", "off", "n":
		return true
	}
	return false
}
