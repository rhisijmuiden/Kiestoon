package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func teamsSample() *pbx.Snapshot {
	s := sample()
	s.Teams = pbx.TeamsSettings{Enabled: true, FQDN: "sbc.example.nl", Port: 5067, CallerID: "own", MaxSessions: 30, Users: []pbx.TeamsUser{
		{Name: "Anna", UPN: "anna@example.nl", Number: "+31201234501", Extension: "201", Ring: "both"},
		{Name: "Bas", UPN: "bas@example.nl", Number: "+31201234502", Extension: "202", Ring: "teams"},
		{Name: "Carla", UPN: "carla@example.nl", Number: "+31201234503", Ring: "teams"},
	}}
	s.Extensions[0].TeamsNumber = "+31201234501"
	s.Extensions[1].TeamsNumber, s.Extensions[1].TeamsOnly = "+31201234502", true
	return s
}

func TestTeamsConfig(t *testing.T) {
	o := Options{TLSCert: "/p/public.crt", TLSKey: "/p/public.key", PublicCert: "/p/public.crt", PublicKey: "/p/public.key"}
	files := Generate(teamsSample(), o)
	pj, dp := files["pjsip.conf"], files["extensions.conf"]
	for _, want := range []string{"[transport-teams]\ntype=transport\nprotocol=tls\nbind=0.0.0.0:5067", "external_signaling_hostname=sbc.example.nl",
		"verify_server=yes", "external_media_address=203.0.113.10", "contact=sip:sip2.pstnhub.microsoft.com:5061;transport=tls",
		"[kt-teams]\ntype=endpoint\ntransport=transport-teams", "from_domain=sbc.example.nl", "media_encryption=sdes",
		"aors=kt-teams-1,kt-teams-2,kt-teams-3", "match=52.112.0.0/14"} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf misses %q", want)
		}
	}
	if strings.Contains(pj, "[transport-teams]") && strings.Contains(strings.SplitN(strings.SplitN(pj, "[transport-teams]", 2)[1], "\n\n", 2)[0], "external_signaling_address") {
		t.Error("external_signaling_address and external_signaling_hostname are mutually exclusive")
	}
	for _, want := range []string{
		`Dial(PJSIP/kt-teams/sip:+${EXTEN}@sip.pstnhub.microsoft.com:5061\;transport=tls\;user=phone,,T)`,
		"Set(KT_DIAL=${KT_DIAL}&Local/31201234501@kiestoon-teams-out/n)",
		"Set(KT_DIAL=Local/31201234502@kiestoon-teams-out/n)",
		"exten => +31201234503,1,NoOp(Teams user carla@example.nl)",
		"exten => 31201234501,1,NoOp(Teams user anna@example.nl)",
		"Set(KT_OUTCID=+31201234501)",
		"exten => 31201234503,1,NoOp(Teams user carla@example.nl)\n same => n,Set(KT_TUSER=31201234503)\n same => n,Set(KT_OUTCID=+31201234503)\n same => n,Set(KT_CLASS=national)",
		"GotoIf($[${DIALPLAN_EXISTS(kiestoon-outbound,${KT_NUM},1)}]?kiestoon-outbound,${KT_NUM},1)",
		"[kiestoon-from-teams]",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	// The DID of a Teams user with an extension is left to the inbound routes.
	if strings.Contains(dp, "exten => +31201234501,1,NoOp(Teams user") {
		t.Error("teams user with extension got its own DID entry")
	}

	// Through one trunk.
	s := teamsSample()
	s.Teams.TrunkID = 1
	dp = Generate(s, o)["extensions.conf"]
	if !strings.Contains(dp, "Dial(PJSIP/${KT_NUM}@trunk-1,,T)") || !strings.Contains(dp, "Set(CALLERID(num)=${IF($[\"${KT_OUTCID}\" != \"\"]?${KT_OUTCID}:0201234567)})") {
		t.Error("teams calls do not use the chosen trunk")
	}
}

func TestTeamsNeedsPublicCert(t *testing.T) {
	files := Generate(teamsSample(), Options{TLSCert: "/p/self.crt", TLSKey: "/p/self.key"})
	if strings.Contains(files["pjsip.conf"], "transport-teams") || strings.Contains(files["extensions.conf"], "kiestoon-teams-out") {
		t.Error("teams configured without a public certificate")
	}
	// Without Teams the extension rings its own phones again.
	if strings.Contains(files["extensions.conf"], "Set(KT_DIAL=Local/") {
		t.Error("teams-only extension still dials teams")
	}
}
