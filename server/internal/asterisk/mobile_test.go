package asterisk

import (
	"strings"
	"testing"
)

func TestMobileAppWake(t *testing.T) {
	s := sample()
	s.Extensions[0].MobileApp = true
	files := Generate(s, Options{})
	pj, dp := files["pjsip.conf"], files["extensions.conf"]
	if !strings.Contains(pj, "[201-app]\ntype=endpoint\ntransport=transport-ws") || strings.Contains(pj, "[202-app]") {
		t.Errorf("pjsip.conf app endpoint:\n%s", pj)
	}
	for _, want := range []string{
		"exten => 201,hint,PJSIP/201&PJSIP/201-app",
		"Set(KT_DIAL=${KT_DIAL}&Local/201@kiestoon-appwake)",
		"[kiestoon-appwake]\nexten => 201,1,NoOp(Mobile app of 201)",
		"UserEvent(KiestoonWake,Ext: 201,WakeID: ${KT_WAKEID},From: ${URIENCODE(${CALLERID(num)})}",
		"(dial),Dial(${KT_APP},,tT)",
		"exten => h,1,ExecIf($[\"${KT_WAKEID}\" != \"\"]?UserEvent(KiestoonWakeEnd,",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	// Intercom never wakes a phone.
	ic := dp[strings.Index(dp, "[kiestoon-intercom]"):]
	ic = ic[:strings.Index(ic, "\n\n")]
	if strings.Contains(ic, "kiestoon-appwake") {
		t.Error("intercom rings the mobile app")
	}
	// Without the app nothing changes.
	plain := Generate(sample(), Options{})["extensions.conf"]
	if strings.Contains(plain, "appwake)") || strings.Contains(plain, "-app") {
		t.Error("app dialplan without a linked app")
	}
}
