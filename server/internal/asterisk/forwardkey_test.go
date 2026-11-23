package asterisk

import (
	"strings"
	"testing"
)

func TestForwardKeyCodes(t *testing.T) {
	s := sample()
	s.Extensions[1].Features.ForwardAlways = "0612345678"
	s.ForwardTargets = []string{"**053", "+31612345678"}
	s.Extensions[0].Features.ForwardBusy = "**0812"
	d := Generate(s, Options{})["extensions.conf"]
	for _, want := range []string{
		"exten => _*74X.,1,Set(KT_TO=${EXTEN:3})",
		"exten => *74201*+31612345678,hint,Custom:kt-fwd-201",
		"exten => *74201*+31612345678,1,Set(KT_TO=+31612345678)",
		`same => n,GotoIf($["${KT_EXT}" = "201"]?kiestoon-fc-201,fwdtoggle,1)`,
		"exten => *74202*+31612345678,hint,Custom:kt-fwd-202",
		"exten => *74201***053,hint,Custom:kt-fwd-201",
		"exten => *74201***053,1,Set(KT_TO=**053)",
		"same => n,Goto(kiestoon-oc-12,s,1)",
		`same => n,ExecIf($["${KT_TO:0:2}" != "**"]?SayDigits(${KT_TO}))`,
	} {
		if !strings.Contains(d, want) {
			t.Errorf("missing %q", want)
		}
	}
	// 201 does not forward: the key switches it on; 202 forwards: the key switches it off.
	fc := func(ext string) string {
		i := strings.Index(d, "[kiestoon-fc-"+ext+"]")
		j := strings.Index(d[i+1:], "\n[")
		return d[i : i+1+j]
	}
	if !strings.Contains(fc("201"), "exten => fwdtoggle,1,Goto(fwdon,1)") {
		t.Error("201 toggle should switch forwarding on")
	}
	if !strings.Contains(fc("202"), "exten => fwdtoggle,1,Goto(fwdoff,1)") {
		t.Error("202 toggle should switch forwarding off")
	}
}
