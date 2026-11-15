package asterisk

import (
	"fmt"
	"slices"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Supervision: a supervisor listens (*33), whispers to the agent (*34) or barges in (*35) on a
// queue call. Agents connected by a queue with supervision on get SPYGROUP=ktq on their member
// channel (Local/<ext>@kiestoon-ring), so ChanSpy only reaches queue calls, never private ones.
// During the spy, 4 = listen, 5 = whisper and 6 = barge switch the mode.

// SpyModes maps the API modes to the digit used in the dialplan.
var SpyModes = map[string]string{"listen": "3", "whisper": "4", "barge": "5"}

// SpyContext is where the portal sends a supervisor's phone for a supervision call.
func SpyContext(supervisor string) string { return "kiestoon-spy-" + supervisor }

// supervisedTargets are the local members of queues with supervision on.
func supervisedTargets(s *pbx.Snapshot) []string {
	var out []string
	for _, q := range s.Queues {
		if !q.Enabled || !q.Options.Supervision {
			continue
		}
		for _, m := range q.Members {
			if !slices.Contains(out, m.Number) {
				out = append(out, m.Number)
			}
		}
	}
	slices.Sort(out)
	return out
}

// CanSupervise tells whether supervisor may supervise target (same rules as the dialplan).
func CanSupervise(s *pbx.Snapshot, supervisor, target string) bool {
	return supervisor != target && slices.Contains(s.Supervisors, supervisor) && slices.Contains(supervisedTargets(s), target)
}

func genSupervision(x *w, s *pbx.Snapshot) {
	targets := supervisedTargets(s)
	x.f("[kiestoon-qconnect]")
	x.f("; runs on the agent's member channel when a supervised queue connects the call")
	x.f("exten => s,1,Set(SPYGROUP=ktq)")
	x.f(" same => n,Return()")
	x.nl()
	x.f("[kiestoon-spy]")
	x.f("; ARG1 = agent extension, ARG2 = ChanSpy mode option (\"\" listen, w whisper, B barge)")
	x.f("exten => s,1,Answer()")
	x.f(" same => n,CELGenUserEvent(kt_spy,target=${ARG1}&mode=${ARG2})")
	x.f(" same => n,ChanSpy(Local/${ARG1}@kiestoon-ring-,qdESg(ktq)${ARG2})")
	x.f(" same => n,Playback(beeperr)")
	x.f(" same => n,Hangup()")
	x.nl()
	for _, sup := range s.Supervisors {
		if len(targets) == 0 {
			break
		}
		x.f("[%s]", SpyContext(sup))
		for _, t := range targets {
			if t == sup {
				continue
			}
			x.f("exten => 3%s,1,Gosub(kiestoon-spy,s,1(%s,))", t, t)
			x.f("exten => 4%s,1,Gosub(kiestoon-spy,s,1(%s,w))", t, t)
			x.f("exten => 5%s,1,Gosub(kiestoon-spy,s,1(%s,B))", t, t)
		}
		x.nl()
	}
}

// Queue callback: queues.conf sends a waiting caller who presses 1 to kiestoon-qcb-<id>. The caller
// confirms the number (or types another), the portal is told with a UserEvent and the caller hangs
// up. The portal then puts a placeholder call (Local/<id>@kiestoon-qcbwait) in the queue at the
// caller's old position; when an agent answers it, the portal moves the agent's channel to
// kiestoon-qcbcall, which calls the caller through the outbound routes.

func genCallbacks(x *w, s *pbx.Snapshot) {
	any := false
	for _, q := range s.Queues {
		if !q.Enabled || !q.Options.Callback.Enabled {
			continue
		}
		any = true
		x.f("[kiestoon-qcb-%d]", q.ID)
		x.f("exten => 1,1,NoOp(Callback requested in queue %s)", q.Number)
		x.f(" same => n,Set(KT_QPOS=${QUEUEPOSITION})")
		x.f(" same => n,Set(KT_CBNUM=${FILTER(0-9+,${CALLERID(num)})})")
		x.f(" same => n,Set(KT_TRIES=0)")
		x.f(` same => n,GotoIf($[${LEN(${KT_CBNUM})} < 6]?other)`)
		x.f(" same => n(confirm),Playback(kt-callback-number)")
		x.f(" same => n,SayDigits(${KT_CBNUM})")
		x.f(" same => n,Read(KT_OK,kt-callback-confirm,1,,2,6)")
		x.f(` same => n,GotoIf($["${KT_OK}" = "1"]?ok)`)
		x.f(` same => n,GotoIf($["${KT_OK}" = "2"]?other)`)
		x.f(" same => n,Goto(back)")
		x.f(" same => n(other),Set(KT_TRIES=$[${KT_TRIES} + 1])")
		x.f(` same => n,GotoIf($[${KT_TRIES} > 3]?back)`)
		x.f(" same => n,Read(KT_CBNUM,kt-callback-enter,16,,1,10)")
		x.f(" same => n,Set(KT_CBNUM=${FILTER(0-9,${KT_CBNUM})})")
		x.f(` same => n,GotoIf($[${LEN(${KT_CBNUM})} >= 6]?confirm:other)`)
		x.f(" same => n(back),Goto(kiestoon-queue-%d,s,join)", q.ID)
		x.f(" same => n(ok),UserEvent(KiestoonCallback,Action: request,Queue: %d,Number: ${KT_CBNUM},"+
			"Name: ${FILTER(A-Za-z0-9 .-,${CALLERID(name)})},Position: ${KT_QPOS},Linkedid: ${CHANNEL(linkedid)})", q.ID)
		mark(x, "callback", fmt.Sprintf("queue=%d", q.ID), "num=${KT_CBNUM}")
		x.f(" same => n,Playback(kt-callback-thanks)")
		x.f(" same => n,Hangup()")
		x.nl()
	}
	if !any {
		return
	}
	x.f("[kiestoon-qcbwait]")
	x.f("; placeholder in the queue for callback EXTEN; KT_CBQ = queue, KT_CBPOS = position")
	x.f("exten => _X.,1,Answer()")
	x.f(" same => n,Set(KT_CBID=${EXTEN})")
	x.f(" same => n,Queue(kiestoon-q-${KT_CBQ},,,,,,,,${KT_CBPOS})")
	x.f(" same => n,UserEvent(KiestoonCallback,Action: left,Id: ${KT_CBID},Status: ${QUEUESTATUS})")
	x.f(" same => n,Hangup()")
	x.nl()
	x.f("[kiestoon-qcbhold]")
	x.f("exten => s,1,Wait(86400)")
	x.f(" same => n,Hangup()")
	x.nl()
	x.f("[kiestoon-qcbcall]")
	x.f("; the agent's member channel, moved here by the portal (KT_CBID set) to call EXTEN")
	x.f("exten => _[0-9+].,1,NoOp(Queue callback ${KT_CBID} to ${EXTEN})")
	x.f(" same => n,Set(CHANNEL(hangup_handler_push)=kiestoon-qcbdone,s,1(${KT_CBID}))")
	x.f(" same => n,Playback(kt-callback-agent)")
	x.f(" same => n,Set(KT_CLASS=international)")
	x.f(" same => n,Set(KT_PREMIUM=0)")
	x.f(` same => n,ExecIf($["${KT_CBCID}" != ""]?Set(KT_OUTCID=${KT_CBCID}))`)
	x.f(" same => n,Set(CDR(kt_callback)=${KT_CBID})")
	x.f(" same => n,Goto(kiestoon-qcbout,${EXTEN},1)")
	x.nl()
	x.f("[kiestoon-qcbout]")
	x.f("exten => _+X.,1,Goto(kiestoon-qcbout,00${EXTEN:1},1)")
	x.f("exten => _X.,1,Goto(kiestoon-outbound,${EXTEN},1)")
	x.nl()
	x.f("[kiestoon-qcbdone]")
	x.f("exten => s,1,UserEvent(KiestoonCallback,Action: done,Id: ${ARG1},Status: ${DIALSTATUS},Answered: ${ANSWEREDTIME})")
	x.f(" same => n,Return()")
	x.nl()
}
