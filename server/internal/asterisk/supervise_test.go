package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestSupervisionAndCallback(t *testing.T) {
	s := sample()
	s.Supervisors = []string{"202"}
	s.Queues = []pbx.Queue{{ID: 7, Number: "701", Name: "Support", Strategy: "rrmemory", Music: "default", AgentTimeout: 20, Retry: 5,
		Enabled: true, Members: []pbx.QueueMember{{ExtensionID: 1, Number: "201", Name: "Anna", LoggedIn: true}},
		Options: pbx.QueueOptions{Supervision: true, Callback: pbx.CallbackOptions{Enabled: true, OfferEvery: 45}}}}
	files := Generate(s, Options{})
	dp := files["extensions.conf"]
	for _, want := range []string{
		"(join),Queue(kiestoon-q-7,tT,,,,,kiestoon-qconnect,,${KT_QPOS})",
		"exten => s,1,Set(SPYGROUP=ktq)",
		"ChanSpy(Local/${ARG1}@kiestoon-ring-,qdESg(ktq)${ARG2})",
		"[kiestoon-spy-202]\nexten => 3201,1,Gosub(kiestoon-spy,s,1(201,))",
		"exten => 4201,1,Gosub(kiestoon-spy,s,1(201,w))", "exten => 5201,1,Gosub(kiestoon-spy,s,1(201,B))",
		"exten => _*3[345]X.,1,GotoIf(${DIALPLAN_EXISTS(kiestoon-spy-${KT_EXT},${EXTEN:2},1)}",
		"[kiestoon-qcb-7]", "UserEvent(KiestoonCallback,Action: request,Queue: 7,", "Goto(kiestoon-queue-7,s,join)",
		"[kiestoon-qcbwait]", "Queue(kiestoon-q-${KT_CBQ},,,,,,,,${KT_CBPOS})", "[kiestoon-qcbcall]",
		"hangup_handler_push)=kiestoon-qcbdone,s,1(${KT_CBID})",
	} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	if strings.Contains(dp, "[kiestoon-spy-201]") {
		t.Error("an agent who is no supervisor got a spy context")
	}
	qc := files["queues.conf"]
	for _, want := range []string{"context=kiestoon-qcb-7", "periodic-announce=kt-callback-offer", "periodic-announce-frequency=45"} {
		if !strings.Contains(qc, want) {
			t.Errorf("queues.conf misses %q", want)
		}
	}
	if !CanSupervise(s, "202", "201") || CanSupervise(s, "201", "202") || CanSupervise(s, "202", "202") {
		t.Error("CanSupervise rules")
	}
	// Without supervision and callback nothing extra is generated.
	s.Queues[0].Options = pbx.QueueOptions{}
	dp = Generate(s, Options{})["extensions.conf"]
	if strings.Contains(dp, "kiestoon-qconnect,,") || strings.Contains(dp, "[kiestoon-qcb-7]") || strings.Contains(dp, "[kiestoon-spy-202]") {
		t.Error("supervision or callback generated while off")
	}
}
