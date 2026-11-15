// Package provision builds configuration files for desk and DECT phones (Yealink, Snom, Grandstream,
// Fanvil, Gigaset, Poly) that fetch them from the portal over HTTP(S), for example via DHCP option 66.
package provision

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// SettingsKey is the settings row with Settings as JSON.
const SettingsKey = "provisioning"

// Settings apply to every phone.
type Settings struct {
	Enabled bool `json:"enabled"`
	// Username and Password protect the provisioning URL (HTTP basic authentication).
	Username string `json:"username"`
	Password string `json:"password"`
	// Server is the address phones use to reach the PBX (IP or host name). Empty: the address
	// the phone used to fetch its configuration.
	Server string `json:"server"`
	// AdminPassword is set as the web interface password of every phone.
	AdminPassword string `json:"adminPassword"`
	NTPServer     string `json:"ntpServer"`
	Language      string `json:"language"` // en | nl
}

// Load reads the settings and creates credentials on first use.
func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	s := Settings{Enabled: true, NTPServer: "pool.ntp.org", Language: "en"}
	if err := store.GetJSON(ctx, db, SettingsKey, &s); err != nil {
		return s, err
	}
	if s.Username == "" || s.Password == "" || s.AdminPassword == "" {
		if s.Username == "" {
			s.Username = "phone"
		}
		if s.Password == "" {
			s.Password = pbx.GenerateSecret(20)
		}
		if s.AdminPassword == "" {
			s.AdminPassword = pbx.GenerateSecret(12)
		}
		if err := store.SetJSON(ctx, db, SettingsKey, s); err != nil {
			return s, err
		}
	}
	return s, nil
}

var reSafe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Validate checks the settings.
func (s *Settings) Validate() error {
	if !reSafe.MatchString(s.Username) || len(s.Password) < 12 || strings.ContainsAny(s.Password, " :@/\n") {
		return fmt.Errorf("credentials")
	}
	if len(s.AdminPassword) < 8 || strings.ContainsAny(s.AdminPassword, "\n\r\"<>&") {
		return fmt.Errorf("adminPassword")
	}
	if s.Server != "" && !regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]{1,255}$`).MatchString(s.Server) {
		return fmt.Errorf("server")
	}
	if s.NTPServer == "" || !regexp.MustCompile(`^[A-Za-z0-9.-]{1,255}$`).MatchString(s.NTPServer) {
		return fmt.Errorf("ntpServer")
	}
	if s.Language != "en" && s.Language != "nl" {
		return fmt.Errorf("language")
	}
	return nil
}

// Model is a supported phone model.
type Model struct {
	Vendor string `json:"vendor"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	// Keys is the number of programmable keys the layout editor shows. 0: no key layout support.
	Keys int `json:"keys"`
	// Lines is the number of extensions the device can carry (DECT handsets). 0 means one.
	Lines int `json:"lines,omitempty"`
}

// MaxLines is the number of extensions the model can carry.
func (m Model) MaxLines() int {
	if m.Lines < 1 {
		return 1
	}
	return m.Lines
}

// Models are the phones Kiestoon can provision.
var Models = []Model{
	{"yealink", "T31P", "Yealink T31P", 2, 0},
	{"yealink", "T33G", "Yealink T33G", 4, 0},
	{"yealink", "T43U", "Yealink T43U", 12, 0},
	{"yealink", "T46U", "Yealink T46U", 16, 0},
	{"yealink", "T48U", "Yealink T48U", 29, 0},
	{"yealink", "T53W", "Yealink T53W", 12, 0},
	{"yealink", "T54W", "Yealink T54W", 16, 0},
	{"yealink", "T57W", "Yealink T57W", 29, 0},
	{"snom", "D713", "Snom D713", 4, 0},
	{"snom", "D717", "Snom D717", 6, 0},
	{"snom", "D735", "Snom D735", 8, 0},
	{"snom", "D785", "Snom D785", 24, 0},
	{"snom", "D865", "Snom D865", 12, 0},
	{"grandstream", "GRP2612", "Grandstream GRP2612", 7, 0},
	{"grandstream", "GRP2614", "Grandstream GRP2614", 7, 0},
	{"grandstream", "GRP2616", "Grandstream GRP2616", 7, 0},
	{"grandstream", "GXP2170", "Grandstream GXP2170", 7, 0},
	{"fanvil", "X3U", "Fanvil X3U", 6, 0},
	{"fanvil", "X4U", "Fanvil X4U", 12, 0},
	{"fanvil", "X5U", "Fanvil X5U", 16, 0},
	{"fanvil", "X6U", "Fanvil X6U", 20, 0},
	{"fanvil", "X7", "Fanvil X7", 20, 0},
	{"gigaset", "N670", "Gigaset N670 IP PRO", 0, 20},
	{"gigaset", "N870", "Gigaset N870 IP PRO", 0, 100},
	{"gigaset", "N870E", "Gigaset N870E IP PRO", 0, 100},
	{"poly", "VVX150", "Poly VVX 150", 0, 0},
	{"poly", "VVX250", "Poly VVX 250", 3, 0},
	{"poly", "VVX350", "Poly VVX 350", 5, 0},
	{"poly", "VVX450", "Poly VVX 450", 11, 0},
	{"poly", "E220", "Poly Edge E220", 3, 0},
	{"poly", "E350", "Poly Edge E350", 7, 0},
	{"poly", "E450", "Poly Edge E450", 11, 0},
}

// FindModel returns the model, or nil.
func FindModel(vendor, id string) *Model {
	for i := range Models {
		if Models[i].Vendor == vendor && Models[i].ID == id {
			return &Models[i]
		}
	}
	return nil
}

// Input is everything one phone's configuration is built from.
type Input struct {
	Settings Settings
	PBX      pbx.Settings
	Phone    pbx.Phone
	Ext      *pbx.Extension
	Server   string // host the phone should register to
	Org      string
	// Lines are the extensions of DECT handsets 2 and up (Gigaset).
	Lines []*pbx.Extension
	// PhonebookURL is the address of the XML phone book for this vendor ("" when the phone book is off).
	PhonebookURL string
}

func transportCode(t string) (yealink, gs int, snom string) {
	switch t {
	case "tcp":
		return 1, 1, "tcp"
	case "tls":
		return 2, 2, "tls"
	}
	return 0, 0, "udp"
}

func (in Input) port() int {
	if in.Ext != nil && in.Ext.Transport == "tls" {
		return in.PBX.TLSPort
	}
	return in.PBX.SIPPort
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
}

// Yealink builds <mac>.cfg.
func Yealink(in Input) string {
	var b strings.Builder
	w := func(k string, v any) { fmt.Fprintf(&b, "%s = %v\n", k, v) }
	b.WriteString("#!version:1.0.0.1\n## Generated by Kiestoon\n\n")
	w("static.security.user_password", "admin:"+clean(in.Settings.AdminPassword))
	w("local_time.ntp_server1", in.Settings.NTPServer)
	w("local_time.time_zone", "+1")
	w("local_time.time_zone_name", "Netherlands(Amsterdam)")
	w("local_time.summer_time", 2)
	w("local_time.time_format", 1)
	w("local_time.date_format", 5)
	if in.Settings.Language == "nl" {
		w("lang.gui", "Dutch")
		w("lang.wui", "Dutch")
	} else {
		w("lang.gui", "English")
		w("lang.wui", "English")
	}
	w("phone_setting.called_party_info_display.enable", 1)
	w("features.dnd.feature_key_sync.enable", 0)
	if in.Ext != nil {
		yt, _, _ := transportCode(in.Ext.Transport)
		w("account.1.enable", 1)
		w("account.1.label", clean(in.Ext.Number+" "+in.Ext.Name))
		w("account.1.display_name", clean(in.Ext.Name))
		w("account.1.auth_name", in.Ext.Number)
		w("account.1.user_name", in.Ext.Number)
		w("account.1.password", clean(in.Ext.SIPSecret))
		w("account.1.sip_server.1.address", in.Server)
		w("account.1.sip_server.1.port", in.port())
		w("account.1.sip_server.1.transport_type", yt)
		w("account.1.sip_server.1.expires", 300)
		w("account.1.subscribe_mwi", 1)
		w("account.1.subscribe_mwi_to_vm", 1)
		w("voice_mail.number.1", "*97")
		if in.Ext.SRTP {
			w("account.1.srtp_encryption", 2)
		} else {
			w("account.1.srtp_encryption", 0)
		}
		w("account.1.codec.g722.enable", 1)
		w("account.1.codec.pcma.enable", 1)
		w("account.1.codec.pcmu.enable", 1)
		w("account.1.dtmf.type", 1)
	} else {
		w("account.1.enable", 0)
	}
	if in.PhonebookURL != "" {
		w("features.remote_phonebook.enable", 1)
		w("features.remote_phonebook.flash_time", 3600)
		w("remote_phonebook.data.1.name", clean(in.Org))
		w("remote_phonebook.data.1.url", in.PhonebookURL)
		w("directory.incoming_call_match_enable", 1)
	}
	for i, k := range in.Phone.Keys {
		n := i + 1
		switch k.Type {
		case "line":
			w(fmt.Sprintf("linekey.%d.type", n), 15)
			w(fmt.Sprintf("linekey.%d.line", n), 1)
			w(fmt.Sprintf("linekey.%d.value", n), "")
		case "blf":
			w(fmt.Sprintf("linekey.%d.type", n), 16)
			w(fmt.Sprintf("linekey.%d.line", n), 1)
			w(fmt.Sprintf("linekey.%d.value", n), k.Value)
			w(fmt.Sprintf("linekey.%d.extension", n), "**")
		case "speed":
			w(fmt.Sprintf("linekey.%d.type", n), 13)
			w(fmt.Sprintf("linekey.%d.line", n), 1)
			w(fmt.Sprintf("linekey.%d.value", n), k.Value)
		default:
			w(fmt.Sprintf("linekey.%d.type", n), 0)
		}
		w(fmt.Sprintf("linekey.%d.label", n), clean(k.Label))
	}
	return b.String()
}

// YealinkCommon builds y000000000000.cfg, which every Yealink fetches first.
func YealinkCommon() string {
	return "#!version:1.0.0.1\n## Generated by Kiestoon: all settings are in <mac>.cfg\nstatic.auto_provision.custom.protect = 1\n"
}

func x(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(clean(s)))
	return b.String()
}

// Snom builds the per-phone XML settings file.
func Snom(in Input) string {
	var b strings.Builder
	w := func(tag, val string) { fmt.Fprintf(&b, "    <%s perm=\"R\">%s</%s>\n", tag, x(val), tag) }
	wi := func(tag, val string) { fmt.Fprintf(&b, "    <%s idx=\"1\" perm=\"R\">%s</%s>\n", tag, x(val), tag) }
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- Generated by Kiestoon -->\n<settings>\n  <phone-settings e=\"2\">\n")
	w("admin_mode_password", in.Settings.AdminPassword)
	w("http_user", "admin")
	w("http_pass", in.Settings.AdminPassword)
	w("ntp_server", in.Settings.NTPServer)
	w("timezone", "NLD+1")
	if in.Settings.Language == "nl" {
		w("language", "Nederlands")
		w("web_language", "Nederlands")
	} else {
		w("language", "English")
		w("web_language", "English")
	}
	w("tone_scheme", "NLD")
	w("dtmf_payload_type", "101")
	if in.Ext != nil {
		_, _, st := transportCode(in.Ext.Transport)
		wi("user_active", "on")
		wi("user_realname", in.Ext.Name)
		wi("user_name", in.Ext.Number)
		wi("user_pname", in.Ext.Number)
		wi("user_pass", in.Ext.SIPSecret)
		wi("user_host", fmt.Sprintf("%s:%d;transport=%s", in.Server, in.port(), st))
		wi("user_outbound", fmt.Sprintf("%s:%d;transport=%s", in.Server, in.port(), st))
		wi("user_mailbox", "*97")
		wi("user_expiry", "300")
		if in.Ext.SRTP {
			wi("user_srtp", "on")
		} else {
			wi("user_srtp", "off")
		}
		wi("codec_priority_list", "g722,pcma,pcmu,telephone-event")
	}
	if in.PhonebookURL != "" {
		w("dkey_directory", "url "+in.PhonebookURL)
	}
	b.WriteString("  </phone-settings>\n  <functionKeys e=\"2\">\n")
	for i, k := range in.Phone.Keys {
		val := ""
		switch k.Type {
		case "line":
			val = "line"
		case "blf":
			val = fmt.Sprintf("blf <sip:%s@%s>|**", k.Value, in.Server)
		case "speed":
			val = "speed " + k.Value
		default:
			val = "none"
		}
		fmt.Fprintf(&b, "    <fkey idx=\"%d\" context=\"1\" label=\"%s\" perm=\"R\">%s</fkey>\n", i, x(k.Label), x(val))
	}
	b.WriteString("  </functionKeys>\n</settings>\n")
	return b.String()
}

// SnomIndex is what a Snom fetches first (snom<model>.htm): it points to the per-phone file.
func SnomIndex(base string) string {
	return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<setting-files>\n  <file url=\"" + x(base) + "snom/{mac}.xml\"/>\n</setting-files>\n"
}

// Grandstream builds cfg<mac>.xml.
func Grandstream(in Input) string {
	var b strings.Builder
	p := func(n int, v any) { fmt.Fprintf(&b, "    <P%d>%s</P%d>\n", n, x(fmt.Sprint(v)), n) }
	fmt.Fprintf(&b, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- Generated by Kiestoon -->\n<gs_provision version=\"1\">\n  <mac>%s</mac>\n  <config version=\"1\">\n", in.Phone.MAC)
	p(2, in.Settings.AdminPassword) // admin password
	p(30, in.Settings.NTPServer)    // NTP server
	p(64, "CET-1CEST-2,M3.5.0/02:00:00,M10.5.0/03:00:00")
	if in.Settings.Language == "nl" {
		p(1362, "nl")
	} else {
		p(1362, "en")
	}
	if in.Ext != nil {
		_, gt, _ := transportCode(in.Ext.Transport)
		p(271, 1) // account active
		p(270, in.Ext.Number+" "+in.Ext.Name)
		p(47, fmt.Sprintf("%s:%d", in.Server, in.port()))
		p(35, in.Ext.Number)
		p(36, in.Ext.Number)
		p(34, in.Ext.SIPSecret)
		p(3, in.Ext.Name)
		p(33, "*97")
		p(130, gt)
		if in.Ext.SRTP {
			p(183, 2)
		} else {
			p(183, 0)
		}
		p(57, 9) // first codec G.722
		p(58, 8) // then PCMA
		p(59, 0) // then PCMU
	} else {
		p(271, 0)
	}
	if in.PhonebookURL != "" {
		// Phone book XML download: P330 protocol (1 HTTP, 3 HTTPS), P331 the path (the phone adds
		// phonebook.xml), P332 the interval in minutes.
		proto, path := 1, strings.TrimPrefix(in.PhonebookURL, "http://")
		if strings.HasPrefix(in.PhonebookURL, "https://") {
			proto, path = 3, strings.TrimPrefix(in.PhonebookURL, "https://")
		}
		p(330, proto)
		p(331, strings.TrimSuffix(path, "/phonebook.xml"))
		p(332, 60)
	}
	// Multi-purpose keys 1-7: mode P323+i, account P301+3i, name P302+3i, value P303+3i.
	for i, k := range in.Phone.Keys {
		if i >= 7 {
			break
		}
		mode, value := 0, ""
		switch k.Type {
		case "blf":
			mode, value = 1, k.Value
		case "speed":
			mode, value = 0, k.Value
		}
		p(323+i, mode)
		p(301+3*i, 0)
		p(302+3*i, clean(k.Label))
		p(303+3*i, value)
	}
	b.WriteString("  </config>\n</gs_provision>\n")
	return b.String()
}

// Request is a parsed provisioning file name.
type Request struct {
	// Vendor is "" for <mac>.cfg, which Yealink, Fanvil and Poly all ask for: the stored phone decides.
	Vendor string
	MAC    string // "" for common/index files
	Model  string
	Common bool
	// PolyConfig is the second Poly file (kiestoon-<mac>.cfg) with the actual settings.
	PolyConfig bool
	// Phonebook is the XML phone book of a vendor (MAC empty).
	Phonebook bool
}

var (
	reMACcfg   = regexp.MustCompile(`^([0-9a-fA-F]{12})\.cfg$`)
	reSnomMAC  = regexp.MustCompile(`^snom/([0-9a-fA-F]{12})\.xml$`)
	reSnomIdx  = regexp.MustCompile(`^snom([A-Za-z0-9]{2,8})(-([0-9A-Fa-f]{12}))?\.htm$`)
	reGrand    = regexp.MustCompile(`^cfg([0-9a-fA-F]{12})(\.xml)?$`)
	reYealinkC = regexp.MustCompile(`^y0{12}\.cfg$`)
	rePolyC    = regexp.MustCompile(`^0{12}\.cfg$`)
	rePolyCfg  = regexp.MustCompile(`^kiestoon-([0-9a-fA-F]{12})\.cfg$`)
	reGigaset  = regexp.MustCompile(`^(gigaset/)?([0-9a-fA-F]{12})\.xml$`)
	rePhonebk  = regexp.MustCompile(`^phonebook/(yealink|snom|grandstream|fanvil|poly)(/phonebook)?\.xml$`)
)

// Parse recognises the file names the vendors ask for.
func Parse(name string) (Request, bool) {
	lower := func(re *regexp.Regexp, i int) string { return strings.ToLower(re.FindStringSubmatch(name)[i]) }
	switch {
	case reYealinkC.MatchString(name):
		return Request{Vendor: "yealink", Common: true}, true
	case rePolyC.MatchString(name):
		return Request{Vendor: "poly", Common: true}, true
	case reMACcfg.MatchString(name):
		return Request{MAC: lower(reMACcfg, 1)}, true
	case rePolyCfg.MatchString(name):
		return Request{Vendor: "poly", MAC: lower(rePolyCfg, 1), PolyConfig: true}, true
	case reSnomMAC.MatchString(name):
		return Request{Vendor: "snom", MAC: lower(reSnomMAC, 1)}, true
	case reSnomIdx.MatchString(name):
		m := reSnomIdx.FindStringSubmatch(name)
		if m[3] != "" {
			return Request{Vendor: "snom", Model: m[1], MAC: strings.ToLower(m[3])}, true
		}
		return Request{Vendor: "snom", Model: m[1], Common: true}, true
	case reGrand.MatchString(name):
		return Request{Vendor: "grandstream", MAC: lower(reGrand, 1)}, true
	case reGigaset.MatchString(name):
		return Request{Vendor: "gigaset", MAC: lower(reGigaset, 2)}, true
	case rePhonebk.MatchString(name):
		return Request{Vendor: rePhonebk.FindStringSubmatch(name)[1], Phonebook: true}, true
	}
	return Request{}, false
}

// Matches reports whether the stored phone may be served the file of the request.
func (r Request) Matches(vendor string) bool {
	if r.Vendor == "" {
		return vendor == "yealink" || vendor == "fanvil" || vendor == "poly"
	}
	return r.Vendor == vendor
}

// Render builds the configuration for a phone. For Poly this is the file with the settings; the
// master file that points to it is PolyMaster.
func Render(in Input) (body, contentType string) {
	switch in.Phone.Vendor {
	case "snom":
		return Snom(in), "application/xml; charset=utf-8"
	case "grandstream":
		return Grandstream(in), "application/xml; charset=utf-8"
	case "fanvil":
		return Fanvil(in), "text/plain; charset=utf-8"
	case "gigaset":
		return Gigaset(in), "application/xml; charset=utf-8"
	case "poly":
		return Poly(in), "application/xml; charset=utf-8"
	}
	return Yealink(in), "text/plain; charset=utf-8"
}
