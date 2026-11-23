package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestGenerateOnCall(t *testing.T) {
	s := sample()
	jan := pbx.Engineer{ID: 1, Name: "Jan", Mobile: "0612345678", Extension: "201"}
	lars := pbx.Engineer{ID: 4, Name: "Lars", Mobile: "0698765432"}
	sanne := pbx.Engineer{ID: 5, Name: "Sanne", Mobile: "0655667788"}
	nomobile := pbx.Engineer{ID: 6, Name: "Piet"}
	team := pbx.OnCallTeam{ID: 3, Number: "690", Name: "IT on-call", Enabled: true, Confirm: true, RingExtension: true, Music: true,
		Repeat: 1, CIDPrefix: "Storing: ", GreetingPrompt: 7, Final: pbx.Destination{Type: "voicemail", Target: "201"},
		Steps: []pbx.OnCallStep{{Kind: "primary", Seconds: 25}, {Kind: "backup", Seconds: 25}, {Kind: "lead", Seconds: 30},
			{Kind: "user", Target: "6", Seconds: 20}, {Kind: "all", Seconds: 30}, {Kind: "number", Target: "0201234567", Seconds: 20}}}
	s.OnCall = []pbx.OnCallLive{{Team: team, Primary: &jan, Backup: &lars, Lead: &sanne, All: []pbx.Engineer{jan, lars, nomobile},
		Users: map[int64]pbx.Engineer{1: jan, 4: lars, 5: sanne, 6: nomobile}}}
	dp := Generate(s, Options{PromptDir: "/p"})["extensions.conf"]
	for _, want := range []string{
		"[kiestoon-oc-3]",
		"exten => 690,1,Goto(kiestoon-oc-3,s,1)",
		"Playback(/p/7)",
		"Set(CALLERID(name)=Storing: ${CALLERID(name)})",
		"Dial(Local/0612345678@kiestoon-followme/n&Local/201@kiestoon-ring/n,25,tTmU(kiestoon-ocok^3^1))",
		"Dial(Local/0698765432@kiestoon-followme/n,25,",
		"Dial(Local/0655667788@kiestoon-followme/n,30,",
		"kt_ocstep,id=3&step=4&kind=user&users=&skip=1",
		"Dial(Local/0612345678@kiestoon-followme/n&Local/201@kiestoon-ring/n&Local/0698765432@kiestoon-followme/n,30,",
		"Dial(Local/0201234567@kiestoon-followme/n,20,",
		"GotoIf($[${KT_OCR} <= 1]?loop)",
		"Goto(kiestoon-vm,201,1)",
		"[kiestoon-ocok]",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	// No engineer on duty: the step is skipped and the call goes to the final destination.
	s.OnCall[0].Primary, s.OnCall[0].Backup, s.OnCall[0].Lead, s.OnCall[0].All = nil, nil, nil, nil
	s.OnCall[0].Team.Steps = []pbx.OnCallStep{{Kind: "primary", Seconds: 25}}
	s.OnCall[0].Team.Final = pbx.Destination{Type: "oncall", Target: "99"}
	dp = Generate(s, Options{})["extensions.conf"]
	sec := dp[strings.Index(dp, "[kiestoon-oc-3]"):]
	sec = sec[:strings.Index(sec, "\n\n")]
	if strings.Contains(sec, "Dial(") || strings.Contains(sec, "KT_OCR} <=") || !strings.Contains(sec, "Hangup()") {
		t.Errorf("empty team:\n%s", sec)
	}
}
