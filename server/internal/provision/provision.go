// Package provision builds configuration files for desk and DECT phones (Yealink, Snom, Grandstream,
// Fanvil, Gigaset, Poly) that fetch them from the portal over HTTP(S), for example via DHCP option 66.
package provision

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"regexp"
	"slices"
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
	// AdminPasswordMode is how phones get the password of their own web interface: perPhone (a
	// generated password for each phone, the default) or shared (AdminPassword on every phone).
	AdminPasswordMode string `json:"adminPasswordMode"`
	// AdminPassword is the shared web interface password.
	AdminPassword string `json:"adminPassword"`
	NTPServer     string `json:"ntpServer"`
	// Language of the phone screens: system (the caller language of the phone system, the
	// default), en or nl.
	Language string `json:"language"`
	// systemLanguage is the caller language of the phone system, filled in by Load.
	systemLanguage string
}

// LangSystem lets the phone screens follow the caller language of the phone system.
const LangSystem = "system"

// Lang is the language the phones get: en or nl.
func (s Settings) Lang() string {
	if s.Language == "en" || s.Language == "nl" {
		return s.Language
	}
	if s.systemLanguage == "nl" {
		return "nl"
	}
	return "en"
}

// WithSystemLanguage returns the settings with the caller language of the phone system set (for
// settings that were not read with Load).
func (s Settings) WithSystemLanguage(lang string) Settings {
	s.systemLanguage = lang
	return s
}

// Admin password modes.
const (
	AdminPerPhone = "perPhone"
	AdminShared   = "shared"
)

// Load reads the settings and creates credentials on first use.
func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	s := Settings{Enabled: true, NTPServer: "pool.ntp.org", Language: LangSystem}
	if err := store.GetJSON(ctx, db, SettingsKey, &s); err != nil {
		return s, err
	}
	if st, err := pbx.LoadSettings(ctx, db); err == nil {
		s.systemLanguage = st.Language
	}
	if s.AdminPasswordMode == "" {
		s.AdminPasswordMode = AdminPerPhone
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
	if s.AdminPasswordMode == "" {
		s.AdminPasswordMode = AdminPerPhone
	}
	if s.AdminPasswordMode != AdminPerPhone && s.AdminPasswordMode != AdminShared {
		return fmt.Errorf("adminPasswordMode")
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
	if s.Language != "en" && s.Language != "nl" && s.Language != LangSystem {
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
	// Lines is the number of extensions the device can carry: DECT handsets (Gigaset) or SIP
	// accounts (desk phones). 0 means one.
	Lines int `json:"lines,omitempty"`
	// PerPage is the number of keys the phone shows at once when its keys span pages. 0: the editor
	// picks a default.
	PerPage int `json:"perPage,omitempty"`
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
	{"yealink", "T31P", "Yealink T31P", 2, 2, 0},
	{"yealink", "T33G", "Yealink T33G", 4, 4, 0},
	// The T41 and T42 series have 6 physical line keys; with more than 6 keys set, the last one
	// switches between 3 pages of 5 (15 line keys). No expansion module fits these phones.
	{"yealink", "T41P", "Yealink T41P", 15, 6, 5},
	{"yealink", "T41S", "Yealink T41S", 15, 6, 5},
	{"yealink", "T41U", "Yealink T41U", 15, 6, 5},
	{"yealink", "T42G", "Yealink T42G", 15, 3, 5},
	{"yealink", "T42S", "Yealink T42S", 15, 12, 5},
	{"yealink", "T42U", "Yealink T42U", 15, 12, 5},
	{"yealink", "T43U", "Yealink T43U", 12, 12, 0},
	{"yealink", "T46U", "Yealink T46U", 16, 16, 0},
	{"yealink", "T48U", "Yealink T48U", 29, 16, 0},
	{"yealink", "T53W", "Yealink T53W", 12, 12, 0},
	{"yealink", "T54W", "Yealink T54W", 16, 16, 0},
	{"yealink", "T57W", "Yealink T57W", 29, 16, 0},
	{"snom", "D713", "Snom D713", 4, 4, 0},
	{"snom", "D717", "Snom D717", 6, 6, 0},
	{"snom", "D735", "Snom D735", 8, 12, 0},
	{"snom", "D785", "Snom D785", 24, 12, 0},
	{"snom", "D865", "Snom D865", 12, 12, 0},
	{"grandstream", "GRP2612", "Grandstream GRP2612", 7, 2, 0},
	{"grandstream", "GRP2614", "Grandstream GRP2614", 7, 4, 0},
	{"grandstream", "GRP2616", "Grandstream GRP2616", 7, 6, 0},
	{"grandstream", "GXP2170", "Grandstream GXP2170", 7, 6, 0},
	{"fanvil", "X3U", "Fanvil X3U", 6, 0, 0},
	{"fanvil", "X4U", "Fanvil X4U", 12, 0, 0},
	{"fanvil", "X5U", "Fanvil X5U", 16, 0, 0},
	{"fanvil", "X6U", "Fanvil X6U", 20, 0, 0},
	{"fanvil", "X7", "Fanvil X7", 20, 0, 0},
	{"gigaset", "N670", "Gigaset N670 IP PRO", 0, 20, 0},
	{"gigaset", "N870", "Gigaset N870 IP PRO", 0, 100, 0},
	{"gigaset", "N870E", "Gigaset N870E IP PRO", 0, 100, 0},
	{"poly", "VVX150", "Poly VVX 150", 0, 0, 0},
	{"poly", "VVX250", "Poly VVX 250", 3, 0, 0},
	{"poly", "VVX350", "Poly VVX 350", 5, 0, 0},
	{"poly", "VVX450", "Poly VVX 450", 11, 0, 0},
	{"poly", "E220", "Poly Edge E220", 3, 0, 0},
	{"poly", "E350", "Poly Edge E350", 7, 0, 0},
	{"poly", "E450", "Poly Edge E450", 11, 0, 0},
	{"htek", "UC912G", "Htek UC912G", 0, 0, 0},
	{"htek", "UC921G", "Htek UC921G", 0, 0, 0},
	{"htek", "UC923U", "Htek UC923U", 0, 0, 0},
	{"htek", "UC924W", "Htek UC924W", 0, 0, 0},
	{"htek", "UC926U", "Htek UC926U", 0, 0, 0},
	{"akuvox", "E12W", "Akuvox E12W (door phone)", 0, 0, 0},
	{"akuvox", "R20A", "Akuvox R20A (door phone)", 0, 0, 0},
	{"akuvox", "R20K", "Akuvox R20K (door phone)", 0, 0, 0},
	{"akuvox", "R29C", "Akuvox R29C (door phone)", 0, 0, 0},
	{"akuvox", "X915", "Akuvox X915 (door phone)", 0, 0, 0},
	{"cisco", "8811", "Cisco IP Phone 8811 (MPP)", 4, 0, 0},
	{"cisco", "8841", "Cisco IP Phone 8841 (MPP)", 4, 0, 0},
	{"cisco", "8845", "Cisco IP Phone 8845 (MPP)", 4, 0, 0},
	{"cisco", "8851", "Cisco IP Phone 8851 (MPP)", 4, 0, 0},
	{"cisco", "8861", "Cisco IP Phone 8861 (MPP)", 4, 0, 0},
	{"cisco", "8865", "Cisco IP Phone 8865 (MPP)", 4, 0, 0},
}

// ModuleType is an expansion module (sidecar) with extra programmable keys.
type ModuleType struct {
	Vendor  string   `json:"vendor"`
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Keys    int      `json:"keys"`    // keys per module, all pages together
	PerPage int      `json:"perPage"` // keys visible at once
	Max     int      `json:"max"`     // modules per phone
	Models  []string `json:"models"`  // phone models it fits
}

// ModuleTypes are the expansion modules Kiestoon configures.
var ModuleTypes = []ModuleType{
	{"yealink", "EXP40", "Yealink EXP40", 40, 20, 6, []string{"T46U", "T48U"}},
	{"yealink", "EXP43", "Yealink EXP43", 60, 20, 3, []string{"T43U", "T46U", "T48U"}},
	{"yealink", "EXP50", "Yealink EXP50", 60, 20, 3, []string{"T53W", "T54W", "T57W"}},
	{"snom", "D7", "Snom D7", 18, 18, 3, []string{"D717", "D735", "D785"}},
	{"grandstream", "GBX20", "Grandstream GBX20", 40, 20, 4, []string{"GXP2170", "GRP2616"}},
}

// Limits are what a phone model supports, for checking a key layout.
func Limits(vendor, model string) (pbx.PhoneLimits, bool) {
	m := FindModel(vendor, model)
	if m == nil {
		return pbx.PhoneLimits{}, false
	}
	lim := pbx.PhoneLimits{Keys: m.Keys, Lines: m.MaxLines(), Modules: map[string]pbx.ModuleLimit{}}
	for _, t := range ModuleTypes {
		if t.Vendor == vendor && slices.Contains(t.Models, model) {
			lim.Modules[t.ID] = pbx.ModuleLimit{Keys: t.Keys, Max: t.Max}
		}
	}
	return lim, true
}

// FirmwareVendors are the vendors whose configuration can point the phone to a firmware file.
var FirmwareVendors = []string{"yealink", "snom", "grandstream", "htek", "poly", "cisco"}

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
	// Firmware is the address of the firmware file for this model ("" when none is assigned);
	// FirmwareDir the directory it is in (Grandstream and Htek ask for a directory).
	Firmware    string
	FirmwareDir string
	// Base is the provisioning address with credentials (ends in /prov/).
	Base string
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
	if in.Settings.Lang() == "nl" {
		w("lang.gui", "Dutch")
		w("lang.wui", "Dutch")
	} else {
		w("lang.gui", "English")
		w("lang.wui", "English")
	}
	w("phone_setting.called_party_info_display.enable", 1)
	w("features.dnd.feature_key_sync.enable", 0)
	accts := in.accounts()
	for i, e := range accts {
		n := i + 1
		yt, _, _ := transportCode(e.Transport)
		w(fmt.Sprintf("account.%d.enable", n), 1)
		w(fmt.Sprintf("account.%d.label", n), clean(e.Number+" "+e.Name))
		w(fmt.Sprintf("account.%d.display_name", n), clean(e.Name))
		w(fmt.Sprintf("account.%d.auth_name", n), e.Number)
		w(fmt.Sprintf("account.%d.user_name", n), e.Number)
		w(fmt.Sprintf("account.%d.password", n), clean(e.SIPSecret))
		w(fmt.Sprintf("account.%d.sip_server.1.address", n), in.Server)
		w(fmt.Sprintf("account.%d.sip_server.1.port", n), in.portFor(e))
		w(fmt.Sprintf("account.%d.sip_server.1.transport_type", n), yt)
		w(fmt.Sprintf("account.%d.sip_server.1.expires", n), 300)
		// Outbound proxy: send all SIP to the PBX, also for calls to other hosts and behind NAT.
		// outbound_proxy.1.* is firmware V83 and up; outbound_host/port is the older spelling (T41P/T42G).
		w(fmt.Sprintf("account.%d.outbound_proxy_enable", n), 1)
		w(fmt.Sprintf("account.%d.outbound_proxy.1.address", n), in.Server)
		w(fmt.Sprintf("account.%d.outbound_proxy.1.port", n), in.portFor(e))
		w(fmt.Sprintf("account.%d.outbound_host", n), in.Server)
		w(fmt.Sprintf("account.%d.outbound_port", n), in.portFor(e))
		w(fmt.Sprintf("account.%d.subscribe_mwi", n), 1)
		w(fmt.Sprintf("account.%d.subscribe_mwi_to_vm", n), 1)
		w(fmt.Sprintf("voice_mail.number.%d", n), "*97")
		if e.SRTP {
			w(fmt.Sprintf("account.%d.srtp_encryption", n), 2)
		} else {
			w(fmt.Sprintf("account.%d.srtp_encryption", n), 0)
		}
		w(fmt.Sprintf("account.%d.codec.g722.enable", n), 1)
		w(fmt.Sprintf("account.%d.codec.pcma.enable", n), 1)
		w(fmt.Sprintf("account.%d.codec.pcmu.enable", n), 1)
		w(fmt.Sprintf("account.%d.dtmf.type", n), 1)
	}
	if len(accts) == 0 {
		w("account.1.enable", 0)
	}
	// Intercom: answer calls with the auto-answer header (Call-Info answer-after=0) by themselves.
	ic := in.Phone.Options.Intercom
	w("features.intercom.allow", b2i(ic.Allow))
	w("features.intercom.mute", b2i(ic.Mute))
	w("features.intercom.tone", b2i(ic.Tone))
	w("features.intercom.barge", b2i(ic.Barge))
	if in.Firmware != "" {
		w("static.firmware.url", in.Firmware)
	}
	if in.PhonebookURL != "" {
		w("features.remote_phonebook.enable", 1)
		w("features.remote_phonebook.flash_time", 3600)
		w("remote_phonebook.data.1.name", clean(in.Org))
		w("remote_phonebook.data.1.url", in.PhonebookURL)
		w("directory.incoming_call_match_enable", 1)
	}
	for i, k := range in.Phone.Keys {
		yealinkKey(w, fmt.Sprintf("linekey.%d.", i+1), k)
	}
	for m, mod := range in.Phone.Modules {
		for i, k := range mod.Keys {
			yealinkKey(w, fmt.Sprintf("expansion_module.%d.key.%d.", m+1, i+1), k)
		}
	}
	return b.String()
}

// yealinkKey writes one line key or expansion module key (prefix ends in ".").
func yealinkKey(w func(string, any), prefix string, k pbx.Key) {
	r := resolveKey(k)
	switch r.Kind {
	case "line":
		w(prefix+"type", 15)
		w(prefix+"line", r.Account)
		w(prefix+"value", "")
	case "blf":
		w(prefix+"type", 16)
		w(prefix+"line", 1)
		w(prefix+"value", r.Value)
		if r.Pickup {
			w(prefix+"extension", "**")
		} else {
			w(prefix+"extension", "")
		}
	case "speed":
		w(prefix+"type", 13)
		w(prefix+"line", 1)
		w(prefix+"value", r.Value)
	case "park":
		w(prefix+"type", 10)
		w(prefix+"line", 1)
		w(prefix+"value", r.Value)
	default:
		w(prefix+"type", 0)
	}
	w(prefix+"label", r.Label)
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
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- Generated by Kiestoon -->\n<settings>\n  <phone-settings e=\"2\">\n")
	w("admin_mode_password", in.Settings.AdminPassword)
	w("http_user", "admin")
	w("http_pass", in.Settings.AdminPassword)
	w("ntp_server", in.Settings.NTPServer)
	w("timezone", "NLD+1")
	if in.Settings.Lang() == "nl" {
		w("language", "Nederlands")
		w("web_language", "Nederlands")
	} else {
		w("language", "English")
		w("web_language", "English")
	}
	w("tone_scheme", "NLD")
	w("dtmf_payload_type", "101")
	for i, e := range in.accounts() {
		idx := i + 1
		wi := func(tag, val string) {
			fmt.Fprintf(&b, "    <%s idx=\"%d\" perm=\"R\">%s</%s>\n", tag, idx, x(val), tag)
		}
		_, _, st := transportCode(e.Transport)
		wi("user_active", "on")
		wi("user_realname", e.Name)
		wi("user_name", e.Number)
		wi("user_pname", e.Number)
		wi("user_pass", e.SIPSecret)
		wi("user_host", fmt.Sprintf("%s:%d;transport=%s", in.Server, in.portFor(e), st))
		wi("user_outbound", fmt.Sprintf("%s:%d;transport=%s", in.Server, in.portFor(e), st))
		wi("user_mailbox", "*97")
		wi("user_expiry", "300")
		if e.SRTP {
			wi("user_srtp", "on")
		} else {
			wi("user_srtp", "off")
		}
		wi("codec_priority_list", "g722,pcma,pcmu,telephone-event")
	}
	// Intercom: auto-answer on the Call-Info answer-after header. answer_after_policy is off, idle,
	// not_busy or always ("always" needs call waiting, which is on by default).
	ic := in.Phone.Options.Intercom
	switch {
	case !ic.Allow:
		w("answer_after_policy", "off")
	case ic.Barge:
		w("answer_after_policy", "always")
	default:
		w("answer_after_policy", "idle")
	}
	w("auto_connect_indication", onOff(ic.Tone))
	w("intercom_mute", onOff(ic.Mute))
	if in.PhonebookURL != "" {
		w("dkey_directory", "url "+in.PhonebookURL)
	}
	if in.Firmware != "" && in.Base != "" {
		w("update_policy", "auto_update")
		w("firmware_status", in.Base+"snom/firmware-"+strings.ToLower(in.Phone.Model)+".xml")
	}
	b.WriteString("  </phone-settings>\n  <functionKeys e=\"2\">\n")
	// Expansion module keys follow the keys of the phone itself.
	for i, k := range in.Phone.Keys {
		snomKey(&b, i, k, in.Server)
	}
	base := len(in.Phone.Keys)
	if m := FindModel(in.Phone.Vendor, in.Phone.Model); m != nil {
		base = m.Keys
	}
	for _, mod := range in.Phone.Modules {
		for i, k := range mod.Keys {
			snomKey(&b, base+i, k, in.Server)
		}
		base += moduleKeys(in.Phone.Vendor, mod.Type)
	}
	b.WriteString("  </functionKeys>\n</settings>\n")
	return b.String()
}

func snomKey(b *strings.Builder, idx int, k pbx.Key, server string) {
	r := resolveKey(k)
	val := "none"
	switch r.Kind {
	case "line":
		val = "line"
	case "blf":
		if r.Pickup {
			val = fmt.Sprintf("blf <sip:%s@%s>|**", r.Value, server)
		} else {
			val = fmt.Sprintf("blf <sip:%s@%s>", r.Value, server)
		}
	case "speed":
		val = "speed " + r.Value
	case "park":
		val = "transfer " + r.Value
	}
	fmt.Fprintf(b, "    <fkey idx=\"%d\" context=\"%d\" label=\"%s\" perm=\"R\">%s</fkey>\n", idx, r.Account, x(r.Label), x(val))
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// moduleKeys is the number of keys of an expansion module type.
func moduleKeys(vendor, id string) int {
	for _, t := range ModuleTypes {
		if t.Vendor == vendor && t.ID == id {
			return t.Keys
		}
	}
	return 0
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
	if in.Settings.Lang() == "nl" {
		p(1362, "nl")
	} else {
		p(1362, "en")
	}
	accts := in.accounts()
	if len(accts) > 0 {
		e := accts[0]
		_, gt, _ := transportCode(e.Transport)
		p(271, 1) // account active
		p(270, e.Number+" "+e.Name)
		p(47, fmt.Sprintf("%s:%d", in.Server, in.portFor(e)))
		p(35, e.Number)
		p(36, e.Number)
		p(34, e.SIPSecret)
		p(3, e.Name)
		p(33, "*97")
		p(130, gt)
		if e.SRTP {
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
	// Accounts 2 and up use the same P-values in blocks: account 2 at 400, 3 at 500, 4 at 600,
	// 5 at 1700 and 6 at 1800 (active +1, server +2, user ID +4, auth ID +5, password +6,
	// name +7, account name +17, voicemail +26). They use the transport of the phone (UDP).
	for i, e := range accts {
		if i == 0 || i > 5 {
			continue
		}
		base := []int{0, 400, 500, 600, 1700, 1800}[i]
		p(base+1, 1)
		p(base+2, fmt.Sprintf("%s:%d", in.Server, in.portFor(e)))
		p(base+4, e.Number)
		p(base+5, e.Number)
		p(base+6, e.SIPSecret)
		p(base+7, e.Name)
		p(base+17, e.Number+" "+e.Name)
		p(base+26, "*97")
	}
	// Intercom: auto-answer on Call-Info (account 1).
	p(298, b2i(in.Phone.Options.Intercom.Allow))
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
	firmwareP(&b, p, in)
	// Multi-purpose keys 1-7: mode P323+i, account P301+3i, name P302+3i, value P303+3i.
	for i, k := range in.Phone.Keys {
		if i >= 7 {
			break
		}
		mode, account, value := gsKey(k)
		p(323+i, mode)
		p(301+3*i, account)
		p(302+3*i, clean(k.Label))
		p(303+3*i, value)
	}
	// GBX20 expansion modules, all keys in a row: mode P6001+n, account P6201+n, name P6401+n,
	// value P6601+n.
	n := 0
	for _, mod := range in.Phone.Modules {
		for _, k := range mod.Keys {
			mode, account, value := gsKey(k)
			p(6001+n, mode)
			p(6201+n, account)
			p(6401+n, clean(k.Label))
			p(6601+n, value)
			n++
		}
		n += moduleKeys(in.Phone.Vendor, mod.Type) - len(mod.Keys)
	}
	b.WriteString("  </config>\n</gs_provision>\n")
	return b.String()
}

// gsKey is the Grandstream key mode, account (0-based) and value of a key.
func gsKey(k pbx.Key) (mode, account int, value string) {
	r := resolveKey(k)
	switch r.Kind {
	case "blf":
		return 1, 0, r.Value
	case "speed":
		return 0, 0, r.Value
	case "park":
		return 9, 0, r.Value
	case "line":
		// A line key is a speed dial without a number on the chosen account: it opens that line.
		return 0, r.Account - 1, ""
	}
	return 0, 0, ""
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
	// Firmware is the Snom firmware pointer of a model.
	Firmware bool
	// Shared lists the vendors that ask for the same file name.
	Shared []string
}

var (
	reMACcfg   = regexp.MustCompile(`^([0-9a-fA-F]{12})\.cfg$`)
	reSnomMAC  = regexp.MustCompile(`^snom/([0-9a-fA-F]{12})\.xml$`)
	reSnomFW   = regexp.MustCompile(`^snom/firmware-([a-z0-9]{2,10})\.xml$`)
	reSnomIdx  = regexp.MustCompile(`^snom([A-Za-z0-9]{2,8})(-([0-9A-Fa-f]{12}))?\.htm$`)
	reGrand    = regexp.MustCompile(`^cfg([0-9a-fA-F]{12})(\.xml)?$`)
	reYealinkC = regexp.MustCompile(`^y0{12}\.cfg$`)
	rePolyC    = regexp.MustCompile(`^0{12}\.cfg$`)
	rePolyCfg  = regexp.MustCompile(`^kiestoon-([0-9a-fA-F]{12})\.cfg$`)
	reGigaset  = regexp.MustCompile(`^(gigaset/)?([0-9a-fA-F]{12})\.xml$`)
	rePhonebk  = regexp.MustCompile(`^phonebook/(yealink|snom|grandstream|fanvil|poly|cisco)(/phonebook)?\.xml$`)
	reCisco    = regexp.MustCompile(`^cisco/([0-9a-fA-F]{12})\.xml$`)
	reCiscoC   = regexp.MustCompile(`^(?i)(CP-?\d{4}[A-Z]?(-3PCC)?|spa\d{3,4})\.xml$`)
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
	case reSnomFW.MatchString(name):
		return Request{Vendor: "snom", Model: reSnomFW.FindStringSubmatch(name)[1], Firmware: true}, true
	case reSnomMAC.MatchString(name):
		return Request{Vendor: "snom", MAC: lower(reSnomMAC, 1)}, true
	case reSnomIdx.MatchString(name):
		m := reSnomIdx.FindStringSubmatch(name)
		if m[3] != "" {
			return Request{Vendor: "snom", Model: m[1], MAC: strings.ToLower(m[3])}, true
		}
		return Request{Vendor: "snom", Model: m[1], Common: true}, true
	case reGrand.MatchString(name):
		// Grandstream and Htek both ask for cfg<mac>.xml: the stored phone decides.
		return Request{Vendor: "grandstream", MAC: lower(reGrand, 1), Shared: []string{"grandstream", "htek"}}, true
	case reCisco.MatchString(name):
		return Request{Vendor: "cisco", MAC: lower(reCisco, 1)}, true
	case reCiscoC.MatchString(name):
		return Request{Vendor: "cisco", Common: true}, true
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
		return vendor == "yealink" || vendor == "fanvil" || vendor == "poly" || vendor == "akuvox"
	}
	if len(r.Shared) > 0 {
		return slices.Contains(r.Shared, vendor)
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
	case "htek":
		return Htek(in), "application/xml; charset=utf-8"
	case "akuvox":
		return Akuvox(in), "text/plain; charset=utf-8"
	case "cisco":
		return Cisco(in), "application/xml; charset=utf-8"
	}
	return Yealink(in), "text/plain; charset=utf-8"
}
