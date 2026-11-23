package pbx

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// SettingsKey is the settings row that holds Settings as JSON.
const SettingsKey = "pbx"

// Settings are the system-wide phone system options.
type Settings struct {
	// Digits is the length of internal numbers: 3, 4 or 5.
	Digits int `json:"digits"`
	// CountryCode without "+" (31 for the Netherlands), used to normalise numbers.
	CountryCode string `json:"countryCode"`
	// EmergencyCallerID is sent when someone dials 112. Empty: the trunk's caller ID.
	EmergencyCallerID string `json:"emergencyCallerId"`
	// EmergencyTrunkIDs is the order in which trunks are tried for 112. Empty: every enabled trunk.
	EmergencyTrunkIDs []int64 `json:"emergencyTrunkIds"`
	// ExternalAddress is the public IP or host name, needed when the PBX is behind NAT.
	ExternalAddress string `json:"externalAddress"`
	// LocalNets are the networks that are not behind NAT (CIDR).
	LocalNets []string `json:"localNets"`
	SIPPort   int      `json:"sipPort"`
	TLSPort   int      `json:"tlsPort"`
	RTPStart  int      `json:"rtpStart"`
	RTPEnd    int      `json:"rtpEnd"`
	// Codecs for phones, in order of preference.
	Codecs string `json:"codecs"`
	// Language is the caller language: everything callers hear (Asterisk prompts, queue
	// announcements, voicemail, numbers), the phone screens (unless set apart), the default
	// language of spoken text and of new users, and system e-mails: en or nl. The language of
	// the web interface is a choice per user.
	Language string `json:"language"`
	// NationalCallerID shows +31 20 … as 020 … on phones, so calling back works.
	NationalCallerID bool `json:"nationalCallerId"`
	// Timezone for voicemail and phones (IANA name).
	Timezone string `json:"timezone"`
}

// DefaultSettings are used on a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Digits: 3, CountryCode: "31", SIPPort: 5060, TLSPort: 5061, RTPStart: 10000, RTPEnd: 20000,
		Codecs: "g722,alaw,ulaw", Language: "en", NationalCallerID: true, Timezone: "Europe/Amsterdam",
		LocalNets:         []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		EmergencyTrunkIDs: []int64{},
	}
}

// LoadSettings reads the settings, filling in defaults.
func LoadSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	s := DefaultSettings()
	err := store.GetJSON(ctx, db, SettingsKey, &s)
	if s.EmergencyTrunkIDs == nil {
		s.EmergencyTrunkIDs = []int64{}
	}
	if s.LocalNets == nil {
		s.LocalNets = []string{}
	}
	return s, err
}

var (
	reDigits   = regexp.MustCompile(`^[0-9]+$`)
	reCallerID = regexp.MustCompile(`^\+?[0-9]{2,20}$`)
	reHost     = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	reSIPUser  = regexp.MustCompile(`^[A-Za-z0-9_.+\-@]{1,64}$`)
	rePattern  = regexp.MustCompile(`^[0-9XZN.!*#+\[\]\-]{0,40}$`)
	reCodecs   = regexp.MustCompile(`^(alaw|ulaw|g722|g729|gsm|opus)(,(alaw|ulaw|g722|g729|gsm|opus))*$`)
)

// Validate checks the settings.
func (s *Settings) Validate() error {
	if s.Digits < 3 || s.Digits > 5 {
		return invalid("invalid", "digits")
	}
	if !reDigits.MatchString(s.CountryCode) || len(s.CountryCode) > 3 {
		return invalid("invalid", "countryCode")
	}
	if s.EmergencyCallerID != "" && !reCallerID.MatchString(s.EmergencyCallerID) {
		return invalid("invalid", "emergencyCallerId")
	}
	if s.ExternalAddress != "" && net.ParseIP(s.ExternalAddress) == nil && !reHost.MatchString(s.ExternalAddress) {
		return invalid("invalid", "externalAddress")
	}
	for _, n := range s.LocalNets {
		if _, _, err := net.ParseCIDR(n); err != nil {
			return invalid("invalid", "localNets")
		}
	}
	ports := []int{s.SIPPort, s.TLSPort, s.RTPStart, s.RTPEnd}
	for _, p := range ports {
		if p < 1024 || p > 65535 {
			return invalid("invalid", "ports")
		}
	}
	if s.SIPPort == s.TLSPort || s.RTPEnd-s.RTPStart < 100 || (s.SIPPort >= s.RTPStart && s.SIPPort <= s.RTPEnd) ||
		(s.TLSPort >= s.RTPStart && s.TLSPort <= s.RTPEnd) {
		return invalid("invalid", "ports")
	}
	if !reCodecs.MatchString(s.Codecs) {
		return invalid("invalid", "codecs")
	}
	if s.Language != "en" && s.Language != "nl" {
		return invalid("invalid", "language")
	}
	if s.Timezone == "" || strings.ContainsAny(s.Timezone, " \n;|") {
		return invalid("invalid", "timezone")
	}
	return nil
}

// ---- number plan ------------------------------------------------------------

// Range is a block of numbers with a purpose.
type Range struct {
	Purpose string `json:"purpose"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// NumberPlan describes how internal numbers are laid out.
type NumberPlan struct {
	Digits int     `json:"digits"`
	Ranges []Range `json:"ranges"`
}

// Reserved numbers can never be an extension.
var Reserved = []string{"112", "911"}

// PlanFor returns the suggested number plan for a number length.
func PlanFor(digits int) NumberPlan {
	pad := func(lead int) (string, string) {
		from := strconv.Itoa(lead) + strings.Repeat("0", digits-1)
		to := strconv.Itoa(lead) + strings.Repeat("9", digits-1)
		return from, to
	}
	extFrom, _ := pad(2)
	_, extTo := pad(5)
	r := []Range{{Purpose: "extensions", From: extFrom, To: extTo}}
	for lead, p := range map[int]string{6: "ringGroups", 7: "queues", 8: "conferences"} {
		f, t := pad(lead)
		r = append(r, Range{Purpose: p, From: f, To: t})
	}
	slices.SortFunc(r, func(a, b Range) int { return strings.Compare(a.From, b.From) })
	return NumberPlan{Digits: digits, Ranges: r}
}

// InRange reports whether number lies in the range of the given purpose.
func (p NumberPlan) InRange(purpose, number string) bool {
	for _, r := range p.Ranges {
		if r.Purpose == purpose && len(number) == p.Digits && number >= r.From && number <= r.To {
			return true
		}
	}
	return false
}

// NextFree returns the first unused extension number in the extension range.
func (p NumberPlan) NextFree(used []string) string {
	for _, r := range p.Ranges {
		if r.Purpose != "extensions" {
			continue
		}
		from, _ := strconv.Atoi(r.From)
		to, _ := strconv.Atoi(r.To)
		// Start at x01 rather than x00, like most PBXs.
		for n := from + 1; n <= to; n++ {
			s := strconv.Itoa(n)
			if !slices.Contains(used, s) && !slices.Contains(Reserved, s) {
				return s
			}
		}
	}
	return ""
}

// Renumber is one change of the number plan migration.
type Renumber struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PlanMigration computes how existing numbers change when the number length changes.
//
// Growing inserts zeros after the first digit (201 -> 2001), so numbers stay in their range.
// Shrinking removes them again (2001 -> 201) and fails when a number would lose a digit other than 0.
func PlanMigration(numbers []string, to int) ([]Renumber, []string) {
	out := []Renumber{}
	conflicts := []string{}
	seen := map[string]bool{}
	for _, n := range numbers {
		var next string
		switch {
		case len(n) == to:
			next = n
		case len(n) < to:
			next = n[:1] + strings.Repeat("0", to-len(n)) + n[1:]
		default:
			cut := len(n) - to
			if strings.Trim(n[1:1+cut], "0") != "" {
				conflicts = append(conflicts, n)
				continue
			}
			next = n[:1] + n[1+cut:]
		}
		if seen[next] || slices.Contains(Reserved, next) {
			conflicts = append(conflicts, n)
			continue
		}
		seen[next] = true
		out = append(out, Renumber{From: n, To: next})
	}
	return out, conflicts
}

// planNumbers returns the numbers of a numbered table (extensions, ring groups, queues, conferences).
func planNumbers(ctx context.Context, db execQuerier, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT number FROM `+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

var numberedTables = []string{"extensions", "ring_groups", "queues", "conferences", "oncall_teams"}

// PlanMigrationAll computes the renumbering of every numbered table.
func (r *Repo) PlanMigrationAll(ctx context.Context, digits int) ([]Renumber, []string, error) {
	all, conflicts := []Renumber{}, []string{}
	for _, t := range numberedTables {
		nums, err := planNumbers(ctx, r.DB, t)
		if err != nil {
			return nil, nil, err
		}
		ch, co := PlanMigration(nums, digits)
		all = append(all, ch...)
		conflicts = append(conflicts, co...)
	}
	return all, conflicts, nil
}

// ApplyPlanMigration changes the number length and renumbers extensions, groups, queues,
// conferences, every destination and phone key that uses them.
func (r *Repo) ApplyPlanMigration(ctx context.Context, settings Settings, digits int) ([]Renumber, error) {
	all, conflicts, err := r.PlanMigrationAll(ctx, digits)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, invalid("renumber_conflict", strings.Join(conflicts, ","))
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	m := map[string]string{}
	for _, c := range all {
		m[c.From] = c.To
	}
	for _, t := range numberedTables {
		nums, err := planNumbers(ctx, tx, t)
		if err != nil {
			return nil, err
		}
		// Two steps through a temporary name, so swaps cannot hit the UNIQUE constraint.
		for _, n := range nums {
			if to, ok := m[n]; ok && to != n {
				if _, err := tx.ExecContext(ctx, `UPDATE `+t+` SET number = ? WHERE number = ?`, "tmp-"+to, n); err != nil {
					return nil, err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+t+` SET number = substr(number, 5) WHERE number LIKE 'tmp-%'`); err != nil {
			return nil, err
		}
	}
	if err := replaceDestinations(ctx, tx, func(d *Destination) bool {
		if to, ok := m[d.Target]; ok && (d.Type == DestExtension || d.Type == DestVoicemail) && to != d.Target {
			d.Target = to
			return true
		}
		return false
	}); err != nil {
		return nil, err
	}
	if err := renumberJSONColumn(ctx, tx, "ring_groups", "members", func(raw string) (string, bool) {
		var mem []string
		_ = json.Unmarshal([]byte(raw), &mem)
		changed := false
		for i, x := range mem {
			if to, ok := m[x]; ok {
				mem[i], changed = to, true
			}
		}
		b, _ := json.Marshal(mem)
		return string(b), changed
	}); err != nil {
		return nil, err
	}
	if err := renumberJSONColumn(ctx, tx, "extensions", "features", func(raw string) (string, bool) {
		var f Features
		_ = json.Unmarshal([]byte(raw), &f)
		changed := false
		for _, p := range []*string{&f.ForwardAlways, &f.ForwardBusy, &f.ForwardNoAnswer, &f.ClosedForward} {
			if to, ok := m[*p]; ok {
				*p, changed = to, true
			}
		}
		b, _ := json.Marshal(f)
		return string(b), changed
	}); err != nil {
		return nil, err
	}
	if err := renumberPhoneKeys(ctx, tx, all); err != nil {
		return nil, err
	}
	settings.Digits = digits
	if err := setJSONTx(ctx, tx, SettingsKey, settings); err != nil {
		return nil, err
	}
	return all, tx.Commit()
}

func renumberJSONColumn(ctx context.Context, tx *sql.Tx, table, col string, fn func(string) (string, bool)) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, `+col+` FROM `+table)
	if err != nil {
		return err
	}
	type upd struct {
		id  int64
		raw string
	}
	var ups []upd
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		if out, ok := fn(raw); ok {
			ups = append(ups, upd{id, out})
		}
	}
	rows.Close()
	for _, u := range ups {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET `+col+` = ? WHERE id = ?`, u.raw, u.id); err != nil {
			return err
		}
	}
	return nil
}

// ---- validation -------------------------------------------------------------

func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ';' || r == '"' || r == '<' || r == '>' || r == '[' || r == ']' || r == '\\' || r == '$' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

// secretOK allows printable ASCII without characters that break Asterisk configuration files.
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

func (r *Repo) validateExtension(ctx context.Context, plan NumberPlan, e *Extension) error {
	e.Name = cleanText(e.Name, 60)
	e.Email = strings.TrimSpace(e.Email)
	if !reDigits.MatchString(e.Number) || len(e.Number) != plan.Digits {
		return invalid("invalid_number_length", "number")
	}
	if slices.Contains(Reserved, e.Number) {
		return invalid("reserved_number", "number")
	}
	if e.Name == "" {
		return invalid("required", "name")
	}
	if e.Email != "" {
		if a, err := mail.ParseAddress(e.Email); err != nil || a.Address != e.Email {
			return invalid("invalid", "email")
		}
	}
	if e.SIPSecret != "" && !secretOK(e.SIPSecret) {
		return invalid("invalid", "sipSecret")
	}
	switch e.Transport {
	case "":
		e.Transport = "udp"
	case "udp", "tcp", "tls":
	default:
		return invalid("invalid", "transport")
	}
	if e.SRTP && e.Transport != "tls" {
		return invalid("srtp_needs_tls", "srtp")
	}
	if e.MaxContacts == 0 {
		e.MaxContacts = 3
	}
	if e.MaxContacts < 1 || e.MaxContacts > 10 {
		return invalid("invalid", "maxContacts")
	}
	if e.RingSeconds == 0 {
		e.RingSeconds = 25
	}
	if e.RingSeconds < 5 || e.RingSeconds > 300 {
		return invalid("invalid", "ringSeconds")
	}
	if e.OutboundCID != "" && !reCallerID.MatchString(e.OutboundCID) {
		return invalid("invalid", "outboundCallerId")
	}
	if e.VoicemailPIN != "" && (!reDigits.MatchString(e.VoicemailPIN) || len(e.VoicemailPIN) < 4 || len(e.VoicemailPIN) > 10) {
		return invalid("invalid", "voicemailPin")
	}
	if e.VoicemailEmail && e.Voicemail && e.Email == "" {
		e.VoicemailEmail = false
	}
	switch e.RecordMode {
	case "":
		e.RecordMode = "ondemand"
	case "off", "ondemand", "always":
	default:
		return invalid("invalid", "recordMode")
	}
	e.PickupGroup = strings.TrimSpace(e.PickupGroup)
	if e.PickupGroup != "" && !regexp.MustCompile(`^[0-9]{1,2}$`).MatchString(e.PickupGroup) {
		return invalid("invalid", "pickupGroup")
	}
	switch e.OutboundClass {
	case "":
		e.OutboundClass = "national"
	case "internal", "national", "international":
	default:
		return invalid("invalid", "outboundClass")
	}
	return e.Features.Validate()
}

func validateTrunk(t *Trunk) error {
	t.Name = cleanText(t.Name, 40)
	t.Host = strings.TrimSpace(t.Host)
	if t.Name == "" {
		return invalid("required", "name")
	}
	if t.Provider == "" {
		t.Provider = "generic"
	}
	if t.AuthMode != "register" && t.AuthMode != "ip" {
		return invalid("invalid", "authMode")
	}
	if net.ParseIP(t.Host) == nil && !reHost.MatchString(t.Host) {
		return invalid("invalid", "host")
	}
	// Port 0 on a host name means "look up DNS SRV records" (RFC 3263); an IP address needs a port.
	if t.Port == 0 && net.ParseIP(t.Host) != nil {
		t.Port = 5060
		if t.Transport == "tls" {
			t.Port = 5061
		}
	}
	if t.Port < 0 || t.Port > 65535 {
		return invalid("invalid", "port")
	}
	switch t.Transport {
	case "":
		t.Transport = "udp"
	case "udp", "tcp", "tls":
	default:
		return invalid("invalid", "transport")
	}
	if t.SRTP && t.Transport != "tls" {
		return invalid("srtp_needs_tls", "srtp")
	}
	if t.AuthMode == "register" {
		if !reSIPUser.MatchString(t.Username) {
			return invalid("required", "username")
		}
		if !secretOK(t.Secret) {
			return invalid("invalid", "secret")
		}
	}
	if t.AuthUsername != "" && !reSIPUser.MatchString(t.AuthUsername) {
		return invalid("invalid", "authUsername")
	}
	if t.FromUser != "" && !reSIPUser.MatchString(t.FromUser) {
		return invalid("invalid", "fromUser")
	}
	if t.FromDomain != "" && !reHost.MatchString(t.FromDomain) {
		return invalid("invalid", "fromDomain")
	}
	ips := []string{}
	for _, f := range strings.FieldsFunc(t.MatchIPs, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
		if net.ParseIP(f) == nil {
			if _, _, err := net.ParseCIDR(f); err != nil {
				return invalid("invalid", "matchIps")
			}
		}
		ips = append(ips, f)
	}
	t.MatchIPs = strings.Join(ips, ",")
	if t.DIDSource == "" {
		t.DIDSource = "ruri"
	}
	if t.DIDSource != "ruri" && t.DIDSource != "to" {
		return invalid("invalid", "didSource")
	}
	if t.Codecs == "" {
		t.Codecs = "alaw,ulaw,g722"
	}
	if !reCodecs.MatchString(t.Codecs) {
		return invalid("invalid", "codecs")
	}
	if t.MaxChannels < 0 || t.MaxChannels > 1000 {
		return invalid("invalid", "maxChannels")
	}
	if t.CallerID != "" && !reCallerID.MatchString(t.CallerID) {
		return invalid("invalid", "callerId")
	}
	return nil
}

// NormalizeDID strips spaces and dashes from a phone number.
func NormalizeDID(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// DIDVariants returns the formats a provider may use for the same number, so a route for
// 020 123 4567 also matches 31201234567 and +31201234567.
func DIDVariants(did, country string) []string {
	if did == "" {
		return nil
	}
	national := ""
	switch {
	case strings.HasPrefix(did, "+"+country):
		national = did[1+len(country):]
	case strings.HasPrefix(did, "00"+country):
		national = did[2+len(country):]
	case strings.HasPrefix(did, country) && len(did) > 9:
		national = did[len(country):]
	case strings.HasPrefix(did, "0") && !strings.HasPrefix(did, "00"):
		national = did[1:]
	}
	if national == "" {
		return []string{did}
	}
	v := []string{"0" + national, country + national, "+" + country + national, "00" + country + national}
	if !slices.Contains(v, did) {
		v = append([]string{did}, v...)
	}
	return v
}

func (r *Repo) validateInbound(ctx context.Context, x *InboundRoute) error {
	x.Name = cleanText(x.Name, 60)
	x.CIDPrefix = cleanText(x.CIDPrefix, 20)
	if x.Name == "" {
		return invalid("required", "name")
	}
	if x.DID != "" && !reCallerID.MatchString(x.DID) {
		return invalid("invalid", "did")
	}
	d := Destination{Type: x.DestinationType, Target: x.Destination}
	if err := r.ValidateDestination(ctx, &d, "destination"); err != nil {
		return err
	}
	x.DestinationType, x.Destination = d.Type, d.Target
	return nil
}

func (r *Repo) validateOutbound(ctx context.Context, x *OutboundRoute) error {
	x.Name = cleanText(x.Name, 60)
	if x.Name == "" {
		return invalid("required", "name")
	}
	if len(x.Patterns) == 0 || len(x.Patterns) > 50 {
		return invalid("required", "patterns")
	}
	for i := range x.Patterns {
		p := &x.Patterns[i]
		p.Prefix, p.Match, p.Prepend = strings.TrimSpace(p.Prefix), strings.ToUpper(strings.TrimSpace(p.Match)), strings.TrimSpace(p.Prepend)
		if p.Match == "" || !rePattern.MatchString(p.Match) || strings.Count(p.Match, "[") != strings.Count(p.Match, "]") {
			return invalid("invalid", "patterns")
		}
		if (p.Prefix != "" && !regexp.MustCompile(`^[0-9*#]{1,10}$`).MatchString(p.Prefix)) ||
			(p.Prepend != "" && !regexp.MustCompile(`^\+?[0-9*#]{1,10}$`).MatchString(p.Prepend)) {
			return invalid("invalid", "patterns")
		}
	}
	if len(x.TrunkIDs) == 0 {
		return invalid("required", "trunkIds")
	}
	for _, id := range x.TrunkIDs {
		var n int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM trunks WHERE id = ?`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return invalid("unknown_trunk", "trunkIds")
		}
	}
	if x.CallerID != "" && !reCallerID.MatchString(x.CallerID) {
		return invalid("invalid", "callerId")
	}
	return nil
}

func setJSONTx(ctx context.Context, tx *sql.Tx, key string, v any) error {
	b, err := jsonMarshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, b)
	return err
}

// ValidLanguageOverride reports whether v is a language for one queue or IVR: en, nl, or empty
// for the caller language of the phone system.
func ValidLanguageOverride(v string) bool { return v == "" || v == "en" || v == "nl" }

// CallerLanguage is the language callers hear in a queue or IVR with the override v.
func (s Settings) CallerLanguage(v string) string {
	if v == "en" || v == "nl" {
		return v
	}
	if s.Language == "nl" {
		return "nl"
	}
	return "en"
}
