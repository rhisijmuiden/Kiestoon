package pbx

import (
	"strings"
	"testing"
)

func TestTeamsNumber(t *testing.T) {
	for in, want := range map[string]string{"020 123 45 67": "+31201234567", "+31 6-1234 5678": "+31612345678", "0032 2 123 45 67": "+3221234567", "12": "", "+0123456789": ""} {
		if got := TeamsNumber(in, "31"); got != want {
			t.Errorf("TeamsNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTeamsValidate(t *testing.T) {
	st := DefaultSettings()
	exts := map[string]bool{"201": true}
	ok := func() TeamsSettings {
		s := DefaultTeams()
		s.Enabled, s.FQDN = true, "SBC.Example.nl."
		s.Users = []TeamsUser{{Name: "Anna", UPN: "Anna@Example.nl", Number: "020 1234501", Extension: "201", Ring: "both"}, {UPN: "bas@example.nl", Number: "+31201234502", Ring: "both"}}
		return s
	}
	s := ok()
	if err := s.Validate(st, exts); err != nil {
		t.Fatal(err)
	}
	if s.FQDN != "sbc.example.nl" || s.Users[0].UPN != "anna@example.nl" || s.Users[0].Number != "+31201234501" || s.Users[1].Ring != "teams" {
		t.Errorf("not cleaned: %+v", s)
	}
	for field, mod := range map[string]func(*TeamsSettings){
		"fqdn":              func(s *TeamsSettings) { s.FQDN = "" },
		"port":              func(s *TeamsSettings) { s.Port = 5061 },
		"callerId":          func(s *TeamsSettings) { s.CallerID = "x" },
		"users.0.upn":       func(s *TeamsSettings) { s.Users[0].UPN = "anna'@example.nl" },
		"users.1.upn":       func(s *TeamsSettings) { s.Users[1].UPN = "anna@example.nl" },
		"users.1.number":    func(s *TeamsSettings) { s.Users[1].Number = "0201234501" },
		"users.0.extension": func(s *TeamsSettings) { s.Users[0].Extension = "999" },
		"users.0.ring":      func(s *TeamsSettings) { s.Users[0].Ring = "" },
	} {
		s := ok()
		mod(&s)
		if err := s.Validate(st, exts); err == nil || err.Error() != field {
			t.Errorf("%s: got %v", field, err)
		}
	}
}

func TestTeamsScript(t *testing.T) {
	s := DefaultTeams()
	s.FQDN = "sbc.example.nl"
	s.Users = []TeamsUser{{UPN: "o'brien@example.nl", Number: "+31201234501"}}
	ps := TeamsScript(s, "nl")
	for _, want := range []string{"New-CsOnlinePSTNGateway -Fqdn 'sbc.example.nl' @gw", "SipSignalingPort = 5067", "MaxConcurrentSessions = 30",
		"Set-CsPhoneNumberAssignment -Identity 'o''brien@example.nl' -PhoneNumber '+31201234501' -PhoneNumberType DirectRouting",
		"Grant-CsOnlineVoiceRoutingPolicy -Identity 'o''brien@example.nl' -PolicyName 'Kiestoon'", "# 4. Nummers van de gebruikers"} {
		if !strings.Contains(ps, want) {
			t.Errorf("script misses %q", want)
		}
	}
}
