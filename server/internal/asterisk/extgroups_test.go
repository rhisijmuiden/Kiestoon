package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestExtensionGroupsOutbound(t *testing.T) {
	s := sample()
	// 201 is in Sales (caller ID 0201111111); 202 keeps its own caller ID although it is in Sales too.
	s.ExtGroups = []pbx.ExtensionGroup{{ID: 7, Name: "Sales", CallerID: "0201111111", Members: []int64{1, 2}}}
	// Route 3 only for Sales, tried first; others fall through to the routes after it.
	s.Outbound = append([]pbx.OutboundRoute{{ID: 3, Name: "Sales line", Patterns: []pbx.DialPattern{{Match: "0[1-9]XXXXXXXX"}},
		TrunkIDs: []int64{2}, GroupIDs: []int64{7, 9}, Enabled: true}}, s.Outbound...)
	f := Generate(s, Options{})
	pj, dp := f["pjsip.conf"], f["extensions.conf"]

	for _, want := range []string{
		"set_var=KT_OUTCID=0201111111", // 201 gets the group's number
		"set_var=KT_OUTCID=0207654321", // 202 keeps its own
		"set_var=KT_GROUP=7",
	} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf misses %q", want)
		}
	}
	if strings.Count(pj, "set_var=KT_OUTCID=0201111111") < 1 || strings.Contains(pj, "KT_OUTCID=0201111111\nset_var=KT_OUTCID") {
		t.Error("double caller ID")
	}
	for _, want := range []string{
		"include => kiestoon-out-3\ninclude => kiestoon-out-1\ninclude => kiestoon-out-2",
		"[kiestoon-outafter-3]\ninclude => kiestoon-out-1\ninclude => kiestoon-out-2\ninclude => kiestoon-out-none",
		`same => n,GotoIf($["${KT_GROUP}" = "7" | "${KT_GROUP}" = "9"]?kiestoon-route-3,s,1)`,
		"same => n,Goto(kiestoon-outafter-3,${EXTEN},1)",
		"[kiestoon-out-none]",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	// Routes for everyone go straight to their route.
	if !strings.Contains(dp, "[kiestoon-out-1]") || strings.Contains(dp, "[kiestoon-outafter-1]") {
		t.Error("open route got a group check")
	}
	// Click-to-call uses the extension's group and caller ID.
	if !strings.Contains(dp, "exten => 201,1,Set(KT_EXT=201)") || !strings.Contains(dp, "Set(KT_GROUP=7)") || !strings.Contains(dp, "Set(KT_OUTCID=0201111111)") {
		t.Error("click-to-call without group")
	}

	// Without groups nothing changes.
	d2 := Generate(sample(), Options{})["extensions.conf"]
	if strings.Contains(d2, "kiestoon-out-none") || strings.Contains(d2, "KT_GROUP}") {
		t.Error("group dialplan without groups")
	}
}
