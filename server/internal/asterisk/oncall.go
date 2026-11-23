package asterisk

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// The on-call (storingsdienst) dialplan. The portal works out who is on duty and regenerates the
// dialplan when that changes (at a hand-over, swap or take-over), so a call never waits for the
// portal and keeps working when the portal is briefly down.

// engineerDial returns the channels that ring one engineer: their own mobile number (out through
// the outbound routes) and, if the team wants it, their extension (desk phone, softphone, app).
func engineerDial(e *pbx.Engineer, withExt bool) []string {
	if e == nil {
		return nil
	}
	out := []string{}
	if e.Mobile != "" {
		out = append(out, "Local/"+e.Mobile+"@kiestoon-followme/n")
	}
	if withExt && e.Extension != "" {
		out = append(out, "Local/"+e.Extension+"@kiestoon-ring/n")
	}
	return out
}

// onCallStepDial returns the channels a step rings and the user ids it rings.
func onCallStepDial(l pbx.OnCallLive, st pbx.OnCallStep) ([]string, []string) {
	withExt := l.Team.RingExtension
	var who []*pbx.Engineer
	switch st.Kind {
	case "primary":
		who = []*pbx.Engineer{l.Primary}
	case "backup":
		who = []*pbx.Engineer{l.Backup}
	case "lead":
		who = []*pbx.Engineer{l.Lead}
	case "all":
		for i := range l.All {
			who = append(who, &l.All[i])
		}
	case "user":
		id, _ := strconv.ParseInt(st.Target, 10, 64)
		if e, ok := l.Users[id]; ok {
			who = []*pbx.Engineer{&e}
		}
	case "number":
		return []string{"Local/" + st.Target + "@kiestoon-followme/n"}, []string{"n" + st.Target}
	}
	chans, ids := []string{}, []string{}
	for _, e := range who {
		for _, c := range engineerDial(e, withExt) {
			if !slices.Contains(chans, c) {
				chans = append(chans, c)
			}
		}
		if e != nil && len(engineerDial(e, withExt)) > 0 {
			ids = append(ids, strconv.FormatInt(e.ID, 10))
		}
	}
	return chans, ids
}

func genOnCall(x *w, s *pbx.Snapshot, o Options) {
	teams := map[string]bool{}
	for _, l := range s.OnCall {
		teams[strconv.FormatInt(l.Team.ID, 10)] = true
	}
	for _, l := range s.OnCall {
		t := l.Team
		x.f("[kiestoon-oc-%d]", t.ID)
		x.f("exten => s,1,NoOp(On-call %s %s)", t.Number, esc(t.Name))
		mark(x, "oncall", fmt.Sprintf("id=%d", t.ID))
		x.f(" same => n,Set(__KT_CLASS=international)")
		x.f(" same => n,Set(__KT_PREMIUM=1)")
		if t.CIDPrefix != "" {
			x.f(` same => n,ExecIf($["${KT_OCPFX}" = ""]?Set(CALLERID(name)=%s${CALLERID(name)}))`, esc(t.CIDPrefix))
			x.f(" same => n,Set(KT_OCPFX=1)")
		}
		if t.GreetingPrompt != 0 {
			x.f(" same => n,Answer()")
			x.f(" same => n,Playback(%s)", promptPath(o, t.GreetingPrompt))
		}
		opts := "tT"
		if t.Music {
			opts += "m"
		}
		confirm := 0
		if t.Confirm {
			confirm = 1
		}
		opts += fmt.Sprintf("U(kiestoon-ocok^%d^%d)", t.ID, confirm)
		x.f(" same => n,Set(KT_OCR=0)")
		x.f(" same => n(loop),NoOp(Escalation round ${KT_OCR})")
		rang := false
		for i, st := range t.Steps {
			chans, ids := onCallStepDial(l, st)
			if len(chans) == 0 {
				mark(x, "ocstep", fmt.Sprintf("id=%d", t.ID), fmt.Sprintf("step=%d", i+1), "kind="+st.Kind, "users=", "skip=1")
				continue
			}
			rang = true
			mark(x, "ocstep", fmt.Sprintf("id=%d", t.ID), fmt.Sprintf("step=%d", i+1), "kind="+st.Kind, "users="+strings.Join(ids, "-"))
			x.f(" same => n,Dial(%s,%d,%s)", strings.Join(chans, "&"), st.Seconds, opts)
			x.f(` same => n,GotoIf($["${DIALSTATUS}" = "ANSWER"]?done)`)
		}
		if rang && t.Repeat > 0 {
			x.f(" same => n,Set(KT_OCR=$[${KT_OCR} + 1])")
			x.f(" same => n,GotoIf($[${KT_OCR} <= %d]?loop)", t.Repeat)
		}
		x.f(" same => n(fail),NoOp(Nobody on call answered)")
		mark(x, "ocend", fmt.Sprintf("id=%d", t.ID))
		final := t.Final
		if final.Type == pbx.DestOnCall && !teams[final.Target] {
			final = pbx.Destination{Type: pbx.DestHangup}
		}
		x.f(" same => n,%s", destGoto(final))
		x.f(" same => n(done),Hangup()")
		x.nl()
	}
	if len(s.OnCall) == 0 {
		return
	}
	// Runs on the engineer's leg when they pick up: with confirm they press 1 first, so a
	// mobile's own voicemail never takes the call. ARG1 team id, ARG2 confirm.
	x.f("[kiestoon-ocok]")
	x.f(`exten => s,1,GotoIf($["${ARG2}" != "1"]?ok)`)
	x.f(" same => n,Read(KT_OK,followme/options,1,,2,6)")
	x.f(` same => n,GotoIf($["${KT_OK}" = "1"]?ok)`)
	x.f(" same => n,Set(GOSUB_RESULT=ABORT)")
	x.f(" same => n,Return()")
	x.f(" same => n(ok),CELGenUserEvent(kt_ocok,id=${ARG1}&num=${CUT(CUT(CHANNEL,@,1),/,2)})")
	x.f(" same => n,Return()")
	x.nl()
}
