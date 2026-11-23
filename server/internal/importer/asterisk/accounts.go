package asterisk

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/importer"
)

var (
	reCallerID = regexp.MustCompile(`^\s*"?([^"<]*?)"?\s*<\s*([^>]*)>\s*$`)
	reDigits   = regexp.MustCompile(`^[0-9]{2,8}$`)
	reURIHost  = regexp.MustCompile(`^(?:sips?:)?(?:([^@;]+)@)?([^:;>]+)(?::([0-9]+))?`)
)

// callerID splits `"Name" <100>` into name and number.
func callerID(s string) (name, number string) {
	if m := reCallerID.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return strings.Trim(strings.TrimSpace(s), `"`), ""
}

func yes(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "y", "t", "1", "on":
		return true
	}
	return false
}

// parseURI returns user, host and port of a SIP URI (sip:user@host:port;params).
func parseURI(u string) (user, host string, port int) {
	u = strings.TrimSpace(strings.Trim(strings.TrimSpace(u), "<>"))
	m := reURIHost.FindStringSubmatch(u)
	if m == nil {
		return "", "", 0
	}
	port, _ = strconv.Atoi(m[3])
	return m[1], m[2], port
}

func transportOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "tls"):
		return "tls"
	case strings.Contains(n, "tcp"):
		return "tcp"
	}
	return "udp"
}

// codecs turns allow= values into Kiestoon's codec list ("" for the default).
func codecs(allow []string) string {
	var out []string
	for _, a := range allow {
		for _, c := range strings.FieldsFunc(a, func(r rune) bool { return r == ',' || r == '&' || r == ' ' }) {
			c = strings.ToLower(strings.TrimSpace(c))
			switch c {
			case "alaw", "ulaw", "g722", "g729", "opus", "gsm":
				if !slices.Contains(out, c) {
					out = append(out, c)
				}
			}
		}
	}
	return strings.Join(out, ",")
}

// device is a SIP account: a phone of a user or a trunk.
type device struct {
	name     string
	user     *importer.User
	trunk    *importer.Trunk
	context  string
	mailbox  string
	external bool // a trunk
}

// accounts reads the phones and trunks of pjsip.conf, pjsip_wizard.conf, sip.conf and users.conf.
type accounts struct {
	devices []*device
	notes   []importer.Note
}

func (a *accounts) byName(n string) *device {
	for _, d := range a.devices {
		if strings.EqualFold(d.name, n) {
			return d
		}
	}
	return nil
}

func (a *accounts) addUser(name, ext, display, secret, context, mailbox string) *device {
	if ext == "" && reDigits.MatchString(name) {
		ext = name
	}
	if ext == "" {
		ext = name
	}
	d := &device{name: name, context: context, mailbox: mailbox,
		user: &importer.User{Ref: ext, Ext: ext, Name: display, SIPPassword: secret}}
	a.devices = append(a.devices, d)
	return d
}

func (a *accounts) addTrunk(name string, t *importer.Trunk, context string) {
	t.Ref, t.Name = name, name
	a.devices = append(a.devices, &device{name: name, trunk: t, context: context, external: true})
}

func readPJSIP(c *Config, a *accounts) {
	byType := func(typ string) []*Section {
		var out []*Section
		for _, s := range c.Sections {
			if !s.Template && strings.EqualFold(s.Get("type"), typ) {
				out = append(out, s)
			}
		}
		return out
	}
	find := func(typ, name string) *Section {
		for _, s := range byType(typ) {
			if s.Name == name {
				return s
			}
		}
		return nil
	}
	regs := byType("registration")
	idents := byType("identify")
	for _, ep := range byType("endpoint") {
		var reg *Section
		for _, r := range regs {
			if r.Get("endpoint") == ep.Name || (r.Get("outbound_auth") != "" && r.Get("outbound_auth") == ep.Get("outbound_auth")) || r.Name == ep.Name {
				reg = r
				break
			}
		}
		var matches []string
		for _, id := range idents {
			if id.Get("endpoint") == ep.Name {
				for _, m := range id.All("match") {
					for _, x := range strings.Split(m, ",") {
						if x = strings.TrimSpace(x); x != "" {
							matches = append(matches, x)
						}
					}
				}
			}
		}
		var contact string
		for _, aorName := range strings.Split(ep.Get("aors"), ",") {
			if aor := find("aor", strings.TrimSpace(aorName)); aor != nil && aor.Get("contact") != "" {
				contact = aor.Get("contact")
			}
		}
		outAuth := find("auth", ep.Get("outbound_auth"))
		if reg != nil || len(matches) > 0 || (contact != "" && outAuth != nil) || (contact != "" && ep.Get("auth") == "") {
			t := &importer.Trunk{Register: reg != nil, Transport: transportOf(ep.Get("transport")), Codecs: codecs(ep.All("allow")),
				FromDomain: ep.Get("from_domain"), FromUser: ep.Get("from_user")}
			if reg != nil {
				_, t.Host, t.Port = parseURI(reg.Get("server_uri"))
				t.Username, _, _ = parseURI(reg.Get("client_uri"))
				if t.Transport == "udp" {
					t.Transport = transportOf(reg.Get("transport"))
				}
				if outAuth == nil {
					outAuth = find("auth", reg.Get("outbound_auth"))
				}
			}
			if t.Host == "" && contact != "" {
				_, t.Host, t.Port = parseURI(contact)
			}
			if outAuth != nil {
				if u := outAuth.Get("username"); u != "" {
					if t.Username == "" {
						t.Username = u
					} else if u != t.Username {
						t.AuthUsername = u
					}
				}
				t.Password = outAuth.Get("password")
			}
			for _, m := range matches {
				if !strings.ContainsAny(m, "abcdefghijklmnopqrstuvwxyz") || strings.Contains(m, ":") {
					t.Networks = append(t.Networks, m)
				}
			}
			_, cid := callerID(ep.Get("callerid"))
			t.CallerID = cid
			a.addTrunk(ep.Name, t, ep.Get("context"))
			continue
		}
		display, num := callerID(ep.Get("callerid"))
		secret := ""
		if au := find("auth", ep.Get("auth")); au != nil {
			secret = au.Get("password")
		}
		d := a.addUser(ep.Name, num, display, secret, ep.Get("context"), ep.Get("mailboxes"))
		if !reDigits.MatchString(ep.Name) && reDigits.MatchString(num) {
			d.user.Ref, d.user.Ext = num, num
		}
	}
	// pjsip_wizard.conf
	for _, w := range byType("wizard") {
		if yes(w.Get("sends_registrations")) || w.Get("remote_hosts") != "" && !yes(w.Get("accepts_registrations")) {
			t := &importer.Trunk{Register: yes(w.Get("sends_registrations")), Transport: transportOf(w.Get("transport")),
				Username: w.Get("outbound_auth/username"), Password: w.Get("outbound_auth/password"), Codecs: codecs(w.All("endpoint/allow")),
				FromDomain: w.Get("endpoint/from_domain"), FromUser: w.Get("endpoint/from_user")}
			hosts := strings.Split(w.Get("remote_hosts"), ",")
			if len(hosts) > 0 {
				h := strings.TrimSpace(hosts[0])
				if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
					t.Port, _ = strconv.Atoi(h[i+1:])
					h = h[:i]
				}
				t.Host = h
			}
			a.addTrunk(w.Name, t, w.Get("endpoint/context"))
			continue
		}
		display, num := callerID(w.Get("endpoint/callerid"))
		a.addUser(w.Name, num, display, w.Get("inbound_auth/password"), w.Get("endpoint/context"), w.Get("endpoint/mailboxes"))
	}
}

var reRegister = regexp.MustCompile(`^(?:([a-z]+)://)?([^:@]+)(?::([^:@]*))?(?::([^@]*))?@([^:/~]+)(?::([0-9]+))?(?:/(.*))?`)

func readSIP(c *Config, a *accounts) {
	var registers []string
	for _, s := range c.Named("general") {
		registers = append(registers, s.All("register")...)
	}
	for _, kv := range c.Global {
		if kv.Key == "register" {
			registers = append(registers, kv.Value)
		}
	}
	for _, s := range c.Sections {
		if s.Template || strings.EqualFold(s.Name, "general") || strings.EqualFold(s.Name, "authentication") {
			continue
		}
		typ := strings.ToLower(s.Get("type"))
		if typ != "friend" && typ != "peer" && typ != "user" {
			continue
		}
		host := s.Get("host")
		if strings.EqualFold(host, "dynamic") || host == "" {
			display, num := callerID(s.Get("callerid"))
			d := a.addUser(s.Name, num, display, s.Get("secret"), s.Get("context"), s.Get("mailbox"))
			if !reDigits.MatchString(s.Name) && reDigits.MatchString(num) {
				d.user.Ref, d.user.Ext = num, num
			}
			continue
		}
		port, _ := strconv.Atoi(s.Get("port"))
		t := &importer.Trunk{Host: host, Port: port, Transport: transportOf(s.Get("transport")), Username: s.Get("username"),
			AuthUsername: s.Get("authuser"), Password: s.Get("secret"), FromDomain: s.Get("fromdomain"), FromUser: s.Get("fromuser"),
			Codecs: codecs(s.All("allow"))}
		if t.Username == "" {
			t.Username = s.Get("defaultuser")
		}
		for _, r := range registers {
			m := reRegister.FindStringSubmatch(strings.TrimSpace(strings.TrimPrefix(r, ">")))
			if m != nil && (strings.EqualFold(m[5], host) || strings.EqualFold(m[2], t.Username)) {
				t.Register = true
				if t.Username == "" {
					t.Username = m[2]
				}
				if t.Password == "" {
					t.Password = m[3]
				}
				if m[7] != "" && reDigits.MatchString(m[7]) == false && len(m[7]) > 6 {
					t.Numbers = append(t.Numbers, m[7])
				}
			}
		}
		if !t.Register && t.Host != "" {
			t.Networks = append(t.Networks, t.Host)
		}
		a.addTrunk(s.Name, t, s.Get("context"))
	}
}

func readUsersConf(c *Config, a *accounts) {
	for _, s := range c.Sections {
		if s.Template || strings.EqualFold(s.Name, "general") || a.byName(s.Name) != nil {
			continue
		}
		if !yes(s.Get("hassip")) && s.Get("secret") == "" {
			continue
		}
		d := a.addUser(s.Name, s.Get("cid_number"), s.Get("fullname"), s.Get("secret"), s.Get("context"), "")
		d.user.Email = s.Get("email")
		if yes(s.Get("hasvoicemail")) {
			d.user.Voicemail = true
			d.user.VoicemailPIN = s.Get("vmsecret")
		}
	}
}

// voicemailBox is one mailbox of voicemail.conf.
type voicemailBox struct {
	pin, name, email string
	attach           bool
}

func readVoicemail(c *Config) map[string]voicemailBox {
	out := map[string]voicemailBox{}
	for _, s := range c.Sections {
		n := strings.ToLower(s.Name)
		if n == "general" || n == "zonemessages" {
			continue
		}
		for _, kv := range s.Keys {
			f := strings.Split(kv.Value, ",")
			box := voicemailBox{pin: strings.TrimPrefix(strings.TrimSpace(f[0]), "-")}
			if len(f) > 1 {
				box.name = strings.TrimSpace(f[1])
			}
			if len(f) > 2 {
				box.email = strings.TrimSpace(f[2])
			}
			box.attach = box.email != ""
			if len(f) > 4 && strings.Contains(f[4], "attach=no") {
				box.attach = false
			}
			// The first context wins (usually "default").
			if _, ok := out[kv.Key]; !ok {
				out[kv.Key] = box
			}
		}
	}
	return out
}

var reMember = regexp.MustCompile(`(?i)^(?:pjsip|sip|iax2|local)/([^@,/&]+)`)

// queueMember returns the device name or number of a queues.conf member.
func queueMember(v string) string {
	if m := reMember.FindStringSubmatch(strings.TrimSpace(v)); m != nil {
		return m[1]
	}
	return ""
}

var queueStrategies = map[string]string{
	"ringall": "ringall", "leastrecent": "leastrecent", "fewestcalls": "fewestcalls", "random": "random",
	"rrmemory": "rrmemory", "rrordered": "rrmemory", "linear": "linear", "wrandom": "wrandom",
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
