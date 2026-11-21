package pbx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Microsoft Teams Direct Routing: Kiestoon acts as the session border controller (SBC) of a
// Microsoft 365 tenant. Teams users then call through Kiestoon and its trunks, and Kiestoon rings
// Teams users like any extension. Microsoft talks SIP over TLS (with a public certificate for the
// SBC's domain name) and SRTP only.

// TeamsKey is the settings row with TeamsSettings as JSON.
const TeamsKey = "teams"

// TeamsHubs are Microsoft's SIP proxies, in the order Kiestoon tries them.
var TeamsHubs = []string{"sip.pstnhub.microsoft.com", "sip2.pstnhub.microsoft.com", "sip3.pstnhub.microsoft.com"}

// TeamsSignalingNets are the networks Microsoft's SIP proxies and media processors use
// (Microsoft 365 "Direct Routing" ranges). The firewall lets them reach the Teams port.
var TeamsSignalingNets = []string{"52.112.0.0/14", "52.122.0.0/15"}

// TeamsSettings is the Direct Routing configuration.
type TeamsSettings struct {
	Enabled bool `json:"enabled"`
	// FQDN is the SBC's host name as added in Teams (New-CsOnlinePSTNGateway). It must be on a
	// domain verified in the Microsoft 365 tenant and covered by Kiestoon's public certificate.
	FQDN string `json:"fqdn"`
	// Port is the SIP TLS port of the Teams transport. It is separate from the TLS port for
	// phones, because Microsoft needs the FQDN in the Contact header.
	Port int `json:"port"`
	// TrunkID sends calls from Teams to the outside world through one trunk. 0: the normal
	// outbound routes.
	TrunkID int64 `json:"trunkId"`
	// CallerID for outside calls from Teams: "own" (the user's Teams number) or "trunk" (the
	// route's or trunk's caller ID).
	CallerID string `json:"callerId"`
	// MaxSessions is the limit of simultaneous calls told to Teams in the script.
	MaxSessions int `json:"maxSessions"`
	// RingBoth is the ring mode for new users: Teams and the desk phone together.
	RingBoth bool `json:"ringBoth"`
	// Users are the Teams users with a Direct Routing number.
	Users []TeamsUser `json:"users"`
}

// TeamsUser links a Teams account to a phone number and, optionally, an extension.
type TeamsUser struct {
	Name string `json:"name"`
	// UPN is the Microsoft 365 sign-in name (user@example.com).
	UPN string `json:"upn"`
	// Number is the E.164 number (+31201234567) the user has in Teams.
	Number string `json:"number"`
	// Extension is the Kiestoon extension of this person (optional).
	Extension string `json:"extension"`
	// Ring is where calls to the extension ring: "teams" or "both" (Teams and the extension's
	// own devices). Without an extension the number always rings in Teams only.
	Ring string `json:"ring"`
}

// DefaultTeams is the configuration of a fresh install.
func DefaultTeams() TeamsSettings {
	return TeamsSettings{Port: 5067, CallerID: "own", MaxSessions: 30, RingBoth: true, Users: []TeamsUser{}}
}

// LoadTeams reads the Teams settings.
func LoadTeams(ctx context.Context, db *sql.DB) (TeamsSettings, error) {
	s := DefaultTeams()
	err := store.GetJSON(ctx, db, TeamsKey, &s)
	if s.Users == nil {
		s.Users = []TeamsUser{}
	}
	return s, err
}

var (
	reFQDN = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	reE164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

// TeamsNumber cleans a phone number into E.164 (+31201234567). National numbers (020 …) get the
// country code. It returns "" when the result is not a valid E.164 number.
func TeamsNumber(n, country string) string {
	n = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(strings.TrimSpace(n))
	switch {
	case strings.HasPrefix(n, "00"):
		n = "+" + n[2:]
	case strings.HasPrefix(n, "0") && country != "":
		n = "+" + country + n[1:]
	}
	if !reE164.MatchString(n) {
		return ""
	}
	return n
}

// Validate checks and cleans the settings. The error is the name of the wrong field; for a user
// it is users.<index>.<field>. ports are the ports already in use (SIP, TLS, RTP range).
func (s *TeamsSettings) Validate(st Settings, extensions map[string]bool) error {
	s.FQDN = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s.FQDN), "."))
	if s.FQDN != "" && (!reFQDN.MatchString(s.FQDN) || len(s.FQDN) > 253) {
		return errors.New("fqdn")
	}
	if s.Enabled && s.FQDN == "" {
		return errors.New("fqdn")
	}
	if s.Port < 1 || s.Port > 65535 || s.Port == st.SIPPort || s.Port == st.TLSPort || (s.Port >= st.RTPStart && s.Port <= st.RTPEnd) {
		return errors.New("port")
	}
	if s.TrunkID < 0 {
		return errors.New("trunkId")
	}
	if s.CallerID != "own" && s.CallerID != "trunk" {
		return errors.New("callerId")
	}
	if s.MaxSessions < 1 || s.MaxSessions > 10000 {
		return errors.New("maxSessions")
	}
	if s.Users == nil {
		s.Users = []TeamsUser{}
	}
	seenNum, seenUPN, seenExt := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range s.Users {
		u := &s.Users[i]
		field := func(f string) error { return fmt.Errorf("users.%d.%s", i, f) }
		u.Name = strings.TrimSpace(u.Name)
		u.UPN = strings.ToLower(strings.TrimSpace(u.UPN))
		u.Extension = strings.TrimSpace(u.Extension)
		if len(u.Name) > 100 || strings.ContainsAny(u.Name, "\"`$;\r\n") {
			return field("name")
		}
		if a, err := mail.ParseAddress(u.UPN); err != nil || a.Address != u.UPN || strings.ContainsAny(u.UPN, "\"'`$; ") {
			return field("upn")
		}
		if seenUPN[u.UPN] {
			return field("upn")
		}
		seenUPN[u.UPN] = true
		u.Number = TeamsNumber(u.Number, st.CountryCode)
		if u.Number == "" || seenNum[u.Number] {
			return field("number")
		}
		seenNum[u.Number] = true
		if u.Extension != "" {
			if !extensions[u.Extension] || seenExt[u.Extension] {
				return field("extension")
			}
			seenExt[u.Extension] = true
		}
		if u.Ring != "teams" && u.Ring != "both" {
			return field("ring")
		}
		if u.Extension == "" {
			u.Ring = "teams"
		}
	}
	return nil
}

// Digits is the number without "+", as used in dialplan extension names.
func (u TeamsUser) Digits() string { return strings.TrimPrefix(u.Number, "+") }

// psQuote quotes a value for a PowerShell single-quoted string.
func psQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

// TeamsScript is the PowerShell script that adds Kiestoon to the Teams tenant: the gateway, a
// voice route for every number, the voice routing policy and the number of every user. It is
// safe to run again: existing objects are updated instead of added. lang is "en" or "nl".
func TeamsScript(s TeamsSettings, lang string) string {
	c := map[string][]string{
		"en": {"# Kiestoon: Microsoft Teams Direct Routing", "# Run in PowerShell with the MicrosoftTeams module, signed in as Teams administrator.",
			"# 1. Kiestoon as gateway (SBC)", "# 2. Route every number through Kiestoon", "# 3. Voice routing policy", "# 4. Numbers of the users"},
		"nl": {"# Kiestoon: Microsoft Teams Direct Routing", "# Uitvoeren in PowerShell met de module MicrosoftTeams, aangemeld als Teams-beheerder.",
			"# 1. Kiestoon als gateway (SBC)", "# 2. Alle nummers via Kiestoon", "# 3. Belbeleid (voice routing policy)", "# 4. Nummers van de gebruikers"},
	}[lang]
	if c == nil {
		return TeamsScript(s, "en")
	}
	fqdn := s.FQDN
	if fqdn == "" {
		fqdn = "sbc.example.com"
	}
	q := psQuote(fqdn)
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	line("%s", c[0])
	line("%s", c[1])
	line("$ErrorActionPreference = 'Stop'")
	line("Import-Module MicrosoftTeams")
	line("Connect-MicrosoftTeams | Out-Null")
	line("")
	line("%s", c[2])
	line("$gw = @{ SipSignalingPort = %d; MaxConcurrentSessions = %d; ForwardCallHistory = $true; ForwardPai = $true; Enabled = $true }", s.Port, s.MaxSessions)
	line("if (Get-CsOnlinePSTNGateway -Identity %s -ErrorAction SilentlyContinue) {", q)
	line("  Set-CsOnlinePSTNGateway -Identity %s @gw", q)
	line("} else {")
	line("  New-CsOnlinePSTNGateway -Fqdn %s @gw", q)
	line("}")
	line("")
	line("%s", c[3])
	line("Set-CsOnlinePstnUsage -Identity Global -Usage @{Add='Kiestoon'} -ErrorAction SilentlyContinue")
	line("if (Get-CsOnlineVoiceRoute -Identity 'Kiestoon' -ErrorAction SilentlyContinue) {")
	line("  Set-CsOnlineVoiceRoute -Identity 'Kiestoon' -NumberPattern '.*' -OnlinePstnGatewayList %s -OnlinePstnUsages 'Kiestoon'", q)
	line("} else {")
	line("  New-CsOnlineVoiceRoute -Identity 'Kiestoon' -NumberPattern '.*' -OnlinePstnGatewayList %s -OnlinePstnUsages 'Kiestoon'", q)
	line("}")
	line("")
	line("%s", c[4])
	line("if (-not (Get-CsOnlineVoiceRoutingPolicy -Identity 'Kiestoon' -ErrorAction SilentlyContinue)) {")
	line("  New-CsOnlineVoiceRoutingPolicy -Identity 'Kiestoon' -OnlinePstnUsages 'Kiestoon' | Out-Null")
	line("}")
	line("")
	line("%s", c[5])
	for _, u := range s.Users {
		line("Set-CsPhoneNumberAssignment -Identity %s -PhoneNumber %s -PhoneNumberType DirectRouting", psQuote(u.UPN), psQuote(u.Number))
		line("Grant-CsOnlineVoiceRoutingPolicy -Identity %s -PolicyName 'Kiestoon'", psQuote(u.UPN))
	}
	return b.String()
}

// markTeams links extensions to their Teams user in a Snapshot.
func markTeams(s *Snapshot) {
	if !s.Teams.Enabled {
		return
	}
	byExt := map[string]TeamsUser{}
	for _, u := range s.Teams.Users {
		if u.Extension != "" {
			byExt[u.Extension] = u
		}
	}
	for i := range s.Extensions {
		if u, ok := byExt[s.Extensions[i].Number]; ok {
			s.Extensions[i].TeamsNumber = u.Number
			s.Extensions[i].TeamsOnly = u.Ring == "teams"
		}
	}
}
