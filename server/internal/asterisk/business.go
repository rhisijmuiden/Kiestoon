package asterisk

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// This file generates the dialplan for phase 2: per-extension call handling (forwarding, DND,
// follow-me), ring groups, queues, IVRs, time conditions, conferences, recording, parking,
// pickup, intercom, feature codes and outbound fraud checks.

// destGoto returns the dialplan application that sends a call to a destination.
func destGoto(d pbx.Destination) string {
	switch d.Type {
	case pbx.DestExtension:
		return fmt.Sprintf("Goto(kiestoon-ext-%s,s,1)", d.Target)
	case pbx.DestVoicemail:
		return fmt.Sprintf("Goto(kiestoon-vm,%s,1)", d.Target)
	case pbx.DestRingGroup:
		return fmt.Sprintf("Goto(kiestoon-rg-%s,s,1)", d.Target)
	case pbx.DestQueue:
		return fmt.Sprintf("Goto(kiestoon-queue-%s,s,1)", d.Target)
	case pbx.DestIVR:
		return fmt.Sprintf("Goto(kiestoon-ivr-%s,s,1)", d.Target)
	case pbx.DestTimeCondition:
		return fmt.Sprintf("Goto(kiestoon-tc-%s,s,1)", d.Target)
	case pbx.DestConference:
		return fmt.Sprintf("Goto(kiestoon-conf-%s,s,1)", d.Target)
	case pbx.DestExternal:
		return fmt.Sprintf("Goto(kiestoon-external,%s,1)", d.Target)
	case pbx.DestFax:
		return fmt.Sprintf("Goto(kiestoon-fax-%s,s,1)", d.Target)
	case pbx.DestFollowSun:
		return fmt.Sprintf("Goto(kiestoon-sun-%s,s,1)", d.Target)
	case pbx.DestRemote:
		return remoteGoto(d.Target)
	}
	return "Hangup()"
}

// promptPath is the file name (without extension) Asterisk plays for a prompt.
func promptPath(o Options, id int64) string {
	return filepath.Join(o.PromptDir, strconv.FormatInt(id, 10))
}

// devicesDial sets KT_DIAL to every registered device of an extension (desk phones and the browser).
func devicesDial(x *w, e pbx.Extension) {
	x.f(" same => n,Set(KT_DIAL=${PJSIP_DIAL_CONTACTS(%s)})", e.Number)
	x.f(` same => n,ExecIf($["${KT_DIAL}" = ""]?Set(KT_DIAL=PJSIP/%s))`, e.Number)
	if e.WebRTC {
		x.f(" same => n,Set(KT_WEB=${PJSIP_DIAL_CONTACTS(%s)})", e.WebRTCEndpoint())
		x.f(` same => n,ExecIf($["${KT_WEB}" != ""]?Set(KT_DIAL=${KT_DIAL}&${KT_WEB}))`)
	}
	x.f(" same => n,Set(_PICKUPMARK=%s)", e.Number)
}

func hint(e pbx.Extension) string {
	if e.WebRTC {
		return fmt.Sprintf("PJSIP/%s&PJSIP/%s", e.Number, e.WebRTCEndpoint())
	}
	return "PJSIP/" + e.Number
}

// classVars makes forwarded and follow-me calls use the outbound rights of the extension that forwards.
func classVars(x *w, e pbx.Extension) {
	x.f(" same => n,Set(__KT_CLASS=%s)", e.OutboundClass)
	x.f(" same => n,Set(__KT_PREMIUM=%d)", b2i(e.AllowPremium))
	x.f(" same => n,Set(__KT_FWDBY=%s)", e.Number)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// forwardTo sends the call to a forward number: an internal number stays inside, anything else
// goes out through the outbound routes (with the forwarding extension's rights).
// mark writes a call-journey marker: a CEL user event the portal stores with the call, so the
// call history can show which steps a call went through. kv are key=value pairs.
func mark(x *w, name string, kv ...string) {
	x.f(" same => n,CELGenUserEvent(kt_%s,%s)", name, strings.Join(kv, "&"))
}

func forwardTo(x *w, num string) {
	mark(x, "fwd", "num="+num)
	x.f(" same => n,Set(KT_HOPS=$[${KT_HOPS} + 1])")
	x.f(` same => n,GotoIf($[${KT_HOPS} > 5]?loop)`)
	x.f(" same => n,Goto(kiestoon-forward,%s,1)", num)
}

func recordOpts(s *pbx.Snapshot) string {
	if s.Features.RecordingBeep {
		return "b,B(15)"
	}
	return "b"
}

func genExtensionsBusiness(x *w, s *pbx.Snapshot) {
	tcs := map[int64]bool{}
	for _, t := range s.TimeConditions {
		tcs[t.ID] = true
	}
	x.f("[kiestoon-extensions]")
	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		x.f("exten => %s,hint,%s", e.Number, hint(e))
		x.f("exten => %s,1,Goto(kiestoon-ext-%s,s,1)", e.Number, e.Number)
	}
	x.nl()

	// Direct dialling of extensions from an IVR.
	x.f("[kiestoon-direct]")
	for _, e := range s.Extensions {
		if e.Enabled {
			x.f("exten => %s,1,Goto(kiestoon-ext-%s,s,1)", e.Number, e.Number)
		}
	}
	x.nl()

	// Voicemail as a destination.
	x.f("[kiestoon-vm]")
	x.f("exten => _X.,1,Answer()")
	mark(x, "vm", "box=${EXTEN}")
	x.f(" same => n,Wait(0.5)")
	x.f(" same => n,VoiceMail(${EXTEN}@default,u)")
	x.f(" same => n,Hangup()")
	x.nl()

	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		f := e.Features
		x.f("[kiestoon-ext-%s]", e.Number)
		x.f("exten => s,1,NoOp(Call for %s)", e.Number)
		mark(x, "ext", "num="+e.Number)
		if e.RecordMode == "always" {
			x.f(" same => n,Gosub(kiestoon-record,s,1(%s))", e.Number)
		}
		if f.DND {
			mark(x, "dnd", "num="+e.Number)
			x.f(" same => n,Goto(busy)")
		}
		if f.ForwardAlways != "" {
			classVars(x, e)
			forwardTo(x, f.ForwardAlways)
		}
		if f.ScheduleID != 0 && f.ClosedForward != "" && tcs[f.ScheduleID] {
			x.f(" same => n,Gosub(kiestoon-tcstate-%d,s,1)", f.ScheduleID)
			x.f(` same => n,GotoIf($["${GOSUB_RETVAL}" != "open"]?closed)`)
		}
		devicesDial(x, e)
		recOpt := ""
		if e.RecordMode == "ondemand" {
			recOpt = "x"
			x.f(" same => n,Set(__TOUCH_MIXMONITOR=kt-${STRFTIME(${EPOCH},,%%Y%%m%%d-%%H%%M%%S)}-${UNIQUEID}-${FILTER(0-9+,${CALLERID(num)})}-%s)", e.Number)
			x.f(" same => n,Set(__TOUCH_MIXMONITOR_FORMAT=wav)")
		}
		fm := f.FollowMe
		if fm.Enabled && len(fm.Numbers) > 0 {
			classVars(x, e)
			extra := ""
			for _, n := range fm.Numbers {
				extra += "&Local/" + n + "@kiestoon-followme/n"
			}
			confirm := ""
			if fm.Confirm {
				confirm = "U(kiestoon-confirm)"
			}
			if fm.Delay > 0 && fm.Delay < e.RingSeconds {
				x.f(" same => n,Dial(${KT_DIAL},%d,tT%s)", fm.Delay, recOpt)
				x.f(` same => n,GotoIf($["${DIALSTATUS}" != "NOANSWER"]?result)`)
				x.f(" same => n,Dial(${KT_DIAL}%s,%d,tT%s%s)", extra, e.RingSeconds-fm.Delay, recOpt, confirm)
			} else {
				x.f(" same => n,Dial(${KT_DIAL}%s,%d,tT%s%s)", extra, e.RingSeconds, recOpt, confirm)
			}
		} else {
			x.f(" same => n,Dial(${KT_DIAL},%d,tT%s)", e.RingSeconds, recOpt)
		}
		x.f(" same => n(result),CELGenUserEvent(kt_extend,num=%s&status=${DIALSTATUS})", e.Number)
		x.f(` same => n,GotoIf($["${DIALSTATUS}" = "BUSY"]?busy)`)
		x.f(` same => n,GotoIf($["${DIALSTATUS}" = "ANSWER"]?done)`)
		// Not answered or not reachable.
		if f.ForwardNoAnswer != "" {
			classVars(x, e)
			forwardTo(x, f.ForwardNoAnswer)
		} else if e.Voicemail {
			mark(x, "vm", "box="+e.Number, "reason=noanswer")
			x.f(" same => n,VoiceMail(%s@default,u)", e.Number)
		} else {
			x.f(" same => n,Playback(vm-nobodyavail)")
		}
		x.f(" same => n(done),Hangup()")
		x.f(" same => n(busy),NoOp(Busy)")
		if f.ForwardBusy != "" {
			classVars(x, e)
			forwardTo(x, f.ForwardBusy)
		} else if e.Voicemail {
			mark(x, "vm", "box="+e.Number, "reason=busy")
			x.f(" same => n,VoiceMail(%s@default,b)", e.Number)
		} else {
			x.f(" same => n,Busy(10)")
		}
		x.f(" same => n,Hangup()")
		if f.ScheduleID != 0 && f.ClosedForward != "" && tcs[f.ScheduleID] {
			x.f(" same => n(closed),NoOp(Outside schedule)")
			mark(x, "closed", "num="+e.Number)
			classVars(x, e)
			forwardTo(x, f.ClosedForward)
		}
		x.f(" same => n(loop),NoOp(Forwarding loop)")
		x.f(" same => n,Playback(ss-noservice)")
		x.f(" same => n,Hangup()")
		x.nl()
	}

	// Ringing only the devices: used by ring groups, queues and pickup. DND counts as busy.
	x.f("[kiestoon-ring]")
	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		x.f("exten => %s,1,NoOp(Ring %s)", e.Number, e.Number)
		if e.Features.DND {
			x.f(" same => n,Busy(5)")
			continue
		}
		x.f(` same => n,GotoIf($["${KT_SKIPBUSY}" = "1" & "${EXTENSION_STATE(%s@kiestoon-extensions)}" = "INUSE"]?busy)`, e.Number)
		devicesDial(x, e)
		x.f(" same => n,Dial(${KT_DIAL},,tT)")
		x.f(" same => n,Hangup()")
		x.f(" same => n(busy),Busy(5)")
	}
	x.nl()

	// Intercom (*80<extension> or an intercom key): only to a known extension that is free and not on
	// DND, on all its devices (desk phones and the browser) with the auto-answer headers.
	x.f("[kiestoon-intercom]")
	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		x.f("exten => %s,1,NoOp(Intercom %s)", e.Number, e.Number)
		if e.Features.DND {
			x.f(" same => n,Goto(busy)")
		}
		x.f(` same => n,GotoIf($["${EXTENSION_STATE(%s@kiestoon-extensions)}" != "NOT_INUSE"]?busy)`, e.Number)
		devicesDial(x, e)
		x.f(" same => n,Set(CALLERID(name)=Intercom ${CALLERID(name)})")
		x.f(" same => n,Dial(${KT_DIAL},20,b(kiestoon-autoanswer^s^1))")
		x.f(" same => n,Hangup()")
		x.f(" same => n(busy),Busy(5)")
	}
	x.f("exten => i,1,Playback(pbx-invalid)")
	x.f(" same => n,Hangup()")
	x.nl()

	// Click-to-call from the portal: the user's phone answered, now call the number with the
	// rights of that extension.
	x.f("[kiestoon-click]")
	x.f("exten => _[0-9+*#].,1,Gosub(kiestoon-clickvars,${KT_CLICKFROM},1)")
	x.f(" same => n,Goto(kiestoon-internal,${EXTEN},1)")
	x.nl()
	x.f("[kiestoon-clickvars]")
	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		x.f("exten => %s,1,Set(KT_EXT=%s)", e.Number, e.Number)
		x.f(" same => n,Set(KT_CLASS=%s)", e.OutboundClass)
		x.f(" same => n,Set(KT_PREMIUM=%d)", b2i(e.AllowPremium))
		if e.OutboundCID != "" {
			x.f(" same => n,Set(KT_OUTCID=%s)", esc(e.OutboundCID))
		}
		x.f(" same => n,Set(CALLERID(all)=\"%s\" <%s>)", esc(strings.ReplaceAll(e.Name, `"`, "")), e.Number)
		x.f(" same => n,Return()")
	}
	x.f("exten => _X.,1,Set(KT_CLASS=internal)")
	x.f(" same => n,Return()")
	x.nl()

	// Forward targets: internal numbers, or out through the routes with the rights of the forwarder.
	x.f("[kiestoon-forward]")
	x.f("include => kiestoon-local")
	x.f("include => kiestoon-outbound")
	x.nl()
	x.f("[kiestoon-followme]")
	x.f("exten => _[0-9+*#].,1,NoOp(Follow-me to ${EXTEN})")
	x.f(" same => n,Goto(kiestoon-outbound,${EXTEN},1)")
	x.nl()
	x.f("[kiestoon-confirm]")
	x.f("exten => s,1,Read(KT_OK,followme/options,1,,2,6)")
	x.f(` same => n,ExecIf($["${KT_OK}" != "1"]?Set(GOSUB_RESULT=ABORT))`)
	x.f(" same => n,Return()")
	x.nl()
	// External numbers as a destination (set by an administrator, so full outbound rights).
	x.f("[kiestoon-external]")
	x.f("exten => _[0-9+].,1,CELGenUserEvent(kt_external,num=${EXTEN})")
	x.f(" same => n,Set(__KT_CLASS=international)")
	x.f(" same => n,Set(__KT_PREMIUM=1)")
	x.f(" same => n,Goto(kiestoon-outbound,${EXTEN},1)")
	x.nl()

	// Recording: one MixMonitor per call, into the monitor spool directory.
	x.f("[kiestoon-record]")
	x.f(`exten => s,1,GotoIf($["${KT_RECORDING}" = "1"]?done)`)
	x.f(" same => n,Set(__KT_RECORDING=1)")
	x.f(" same => n,MixMonitor(kt-${STRFTIME(${EPOCH},,%%Y%%m%%d-%%H%%M%%S)}-${UNIQUEID}-${FILTER(0-9+,${CALLERID(num)})}-${FILTER(0-9+*#q,${ARG1})}.wav,%s)", recordOpts(s))
	x.f(" same => n(done),Return()")
	x.nl()
}

// genLocal is everything an internal caller can dial besides extensions and outbound numbers.
func genLocal(x *w, s *pbx.Snapshot) {
	x.f("[kiestoon-local]")
	x.f("include => kiestoon-extensions")
	for _, g := range s.RingGroups {
		if g.Enabled {
			x.f("exten => %s,1,Goto(kiestoon-rg-%d,s,1)", g.Number, g.ID)
		}
	}
	for _, q := range s.Queues {
		if q.Enabled {
			x.f("exten => %s,1,Goto(kiestoon-queue-%d,s,1)", q.Number, q.ID)
		}
	}
	for _, c := range s.Conferences {
		if c.Enabled {
			x.f("exten => %s,1,Goto(kiestoon-conf-%d,s,1)", c.Number, c.ID)
		}
	}
	x.nl()
}

func memberDial(m string, digits int) string {
	if len(m) == digits && !strings.HasPrefix(m, "0") && !strings.HasPrefix(m, "+") {
		return "Local/" + m + "@kiestoon-ring/n"
	}
	return "Local/" + m + "@kiestoon-followme/n"
}

func genRingGroups(x *w, s *pbx.Snapshot) {
	exts := map[string]pbx.Extension{}
	for _, e := range s.Extensions {
		exts[e.Number] = e
	}
	for _, g := range s.RingGroups {
		if !g.Enabled {
			continue
		}
		x.f("[kiestoon-rg-%d]", g.ID)
		x.f("exten => s,1,NoOp(Ring group %s %s)", g.Number, esc(g.Name))
		mark(x, "rg", fmt.Sprintf("id=%d", g.ID))
		if g.CIDPrefix != "" {
			x.f(` same => n,ExecIf($["${KT_RGPFX}" = ""]?Set(CALLERID(name)=%s${CALLERID(name)}))`, esc(g.CIDPrefix))
			x.f(" same => n,Set(KT_RGPFX=1)")
		}
		x.f(" same => n,Set(__KT_CLASS=international)")
		x.f(" same => n,Set(__KT_PREMIUM=1)")
		x.f(" same => n,Set(__KT_SKIPBUSY=%d)", b2i(g.SkipBusy))
		members := []string{}
		for _, m := range g.Members {
			if e, ok := exts[m]; ok && !e.Enabled {
				continue
			}
			members = append(members, m)
		}
		switch g.Strategy {
		case "paging":
			devs := []string{}
			for _, m := range members {
				if e, ok := exts[m]; ok {
					devs = append(devs, "PJSIP/"+e.Number)
				}
			}
			if len(devs) > 0 {
				x.f(" same => n,Page(%s,ib(kiestoon-autoanswer^s^1),%d)", strings.Join(devs, "&"), g.RingSeconds)
			}
			x.f(" same => n,Hangup()")
			x.nl()
			continue
		case "simultaneous":
			if len(members) > 0 {
				ds := make([]string, len(members))
				for i, m := range members {
					ds[i] = memberDial(m, s.Settings.Digits)
				}
				x.f(" same => n,Dial(%s,%d,tT)", strings.Join(ds, "&"), g.RingSeconds)
				x.f(` same => n,GotoIf($["${DIALSTATUS}" = "ANSWER"]?done)`)
			}
		default: // sequential and rotating
			n := len(members)
			for i, m := range members {
				x.f(" same => n,Set(KT_M%d=%s)", i, memberDial(m, s.Settings.Digits))
			}
			x.f(" same => n,Set(KT_I=0)")
			if g.Strategy == "rotating" {
				x.f(` same => n,Set(KT_P=${IF($["${GLOBAL(KT_RG_%d)}" = ""]?0:${GLOBAL(KT_RG_%d)})})`, g.ID, g.ID)
			} else {
				x.f(" same => n,Set(KT_P=0)")
			}
			x.f(" same => n(next),GotoIf($[${KT_I} >= %d]?fail)", n)
			x.f(" same => n,Set(KT_X=$[(${KT_P} + ${KT_I}) %% %d])", max(n, 1))
			if g.Strategy == "rotating" {
				x.f(" same => n,Set(GLOBAL(KT_RG_%d)=$[(${KT_X} + 1) %% %d])", g.ID, max(n, 1))
			}
			x.f(" same => n,Set(KT_I=$[${KT_I} + 1])")
			x.f(" same => n,Dial(${KT_M${KT_X}},%d,tT)", g.RingSeconds)
			x.f(` same => n,GotoIf($["${DIALSTATUS}" = "ANSWER"]?done:next)`)
		}
		x.f(" same => n(fail),NoOp(Nobody answered)")
		mark(x, "rgend", fmt.Sprintf("id=%d", g.ID))
		x.f(" same => n,%s", destGoto(g.Failover))
		x.f(" same => n(done),Hangup()")
		x.nl()
	}
	// Auto-answer headers for intercom and paging (Yealink, Snom, Grandstream understand one of them).
	x.f("[kiestoon-autoanswer]")
	x.f("exten => s,1,Set(PJSIP_HEADER(add,Call-Info)=<uri>\\;answer-after=0)")
	x.f(" same => n,Set(PJSIP_HEADER(add,Alert-Info)=info=alert-autoanswer)")
	x.f(" same => n,Return()")
	x.nl()
}

func genQueues(x *w, s *pbx.Snapshot, o Options) {
	for _, q := range s.Queues {
		if !q.Enabled {
			continue
		}
		x.f("[kiestoon-queue-%d]", q.ID)
		x.f("exten => s,1,NoOp(Queue %s %s)", q.Number, esc(q.Name))
		mark(x, "queue", fmt.Sprintf("id=%d", q.ID))
		if q.CIDPrefix != "" {
			x.f(" same => n,Set(CALLERID(name)=%s${CALLERID(name)})", esc(q.CIDPrefix))
		}
		x.f(" same => n,Answer()")
		if q.GreetingPrompt != 0 {
			x.f(" same => n,Playback(%s)", promptPath(o, q.GreetingPrompt))
		}
		if q.Record {
			x.f(" same => n,Gosub(kiestoon-record,s,1(q%s))", q.Number)
		}
		maxWait := ""
		if q.MaxWait > 0 {
			maxWait = strconv.Itoa(q.MaxWait)
		}
		gosub := ""
		if q.Options.Supervision {
			gosub = "kiestoon-qconnect"
		}
		x.f(" same => n(join),Queue(kiestoon-q-%d,tT,,,%s,,%s,,${KT_QPOS})", q.ID, maxWait, gosub)
		x.f(" same => n,NoOp(Queue result ${QUEUESTATUS})")
		mark(x, "queueend", fmt.Sprintf("id=%d", q.ID), "status=${QUEUESTATUS}")
		x.f(" same => n,%s", destGoto(q.Overflow))
		x.f(" same => n,Hangup()")
		x.nl()
	}
}

// genQueuesConf builds queues.conf. Only signed-in members are listed: the database is the
// source of truth for who is signed in, and the file is regenerated when that changes.
func genQueuesConf(s *pbx.Snapshot) string {
	var x w
	x.b.WriteString(header)
	x.f("[general]")
	x.f("persistentmembers=no")
	x.f("autofill=yes")
	x.f("shared_lastcall=yes")
	x.f("log_membername_as_agent=yes")
	x.nl()
	for _, q := range s.Queues {
		if !q.Enabled {
			continue
		}
		x.f("[kiestoon-q-%d]", q.ID)
		x.f("strategy=%s", q.Strategy)
		x.f("musicclass=%s", q.Music)
		x.f("timeout=%d", q.AgentTimeout)
		x.f("retry=%d", q.Retry)
		x.f("wrapuptime=%d", q.Wrapup)
		x.f("maxlen=%d", q.MaxCallers)
		x.f("ringinuse=no")
		x.f("setinterfacevar=yes")
		x.f("announce-holdtime=no")
		if q.Options.Callback.Enabled {
			x.f("context=kiestoon-qcb-%d", q.ID)
			x.f("periodic-announce=kt-callback-offer")
			x.f("periodic-announce-frequency=%d", q.Options.Callback.OfferEvery)
		}
		if q.AnnouncePosition {
			x.f("announce-position=yes")
			x.f("announce-frequency=%d", q.AnnounceFrequency)
			x.f("min-announce-frequency=15")
		} else {
			x.f("announce-position=no")
			x.f("announce-frequency=0")
		}
		if q.JoinEmpty {
			x.f("joinempty=no")
			x.f("leavewhenempty=no")
		} else {
			x.f("joinempty=invalid,unavailable")
			x.f("leavewhenempty=invalid,unavailable")
		}
		for _, m := range q.Members {
			if !m.LoggedIn {
				continue
			}
			x.f("member => Local/%s@kiestoon-ring/n,%d,%s,hint:%s@kiestoon-extensions", m.Number, m.Penalty,
				esc(strings.ReplaceAll(m.Name, ",", " ")), m.Number)
		}
		remoteQueueMembers(&x, s, q)
		x.nl()
	}
	return x.b.String()
}

func genIVRs(x *w, s *pbx.Snapshot, o Options) {
	for _, v := range s.IVRs {
		f := v.Flow
		if f == nil || f.Entry() == nil {
			f = pbx.LegacyFlow(&v, nil)
		}
		steps := f.Steps()
		flowCtx := fmt.Sprintf("kiestoon-ivr-%d-flow", v.ID)
		// gotoNode sends the call to the step a port leads to; nothing connected hangs up.
		gotoNode := func(n *pbx.FlowNode) string {
			if n == nil {
				return "Hangup()"
			}
			if i, ok := steps[n.ID]; ok {
				return fmt.Sprintf("Goto(%s,%d,1)", flowCtx, i)
			}
			return "Hangup()"
		}
		entry := f.Entry()
		x.f("[kiestoon-ivr-%d]", v.ID)
		if v.DirectDial {
			x.f("include => kiestoon-direct")
		}
		x.f("exten => s,1,NoOp(IVR %s)", esc(v.Name))
		x.f(" same => n,Answer()")
		mark(x, "ivr", fmt.Sprintf("id=%d", v.ID))
		x.f(" same => n,Set(KT_TRIES=0)")
		x.f(" same => n(start),Set(TIMEOUT(digit)=3)")
		x.f(" same => n,Set(TIMEOUT(response)=%d)", v.Timeout)
		if entry.PromptID != 0 {
			x.f(" same => n,Background(%s)", promptPath(o, entry.PromptID))
		}
		keys := f.MenuKeys()
		for _, k := range keys {
			if k.PromptID != 0 {
				x.f(" same => n,Background(%s)", promptPath(o, k.PromptID))
			}
		}
		x.f(" same => n,WaitExten(%d)", v.Timeout)
		for _, k := range keys {
			x.f("exten => %s,1,CELGenUserEvent(kt_ivrkey,id=%d&key=${EXTEN})", k.Digit, v.ID)
			x.f(" same => n,%s", gotoNode(f.Next(k.ID, "out")))
		}
		timeout := f.TimeoutNode()
		for _, ev := range []string{"t", "i"} {
			x.f("exten => %s,1,Set(KT_TRIES=$[${KT_TRIES} + 1])", ev)
			mark(x, map[string]string{"t": "ivrtimeout", "i": "ivrinvalid"}[ev], fmt.Sprintf("id=%d", v.ID), "try=${KT_TRIES}")
			if ev == "i" {
				x.f(" same => n,Playback(option-is-invalid)")
			}
			x.f(" same => n,GotoIf($[${KT_TRIES} <= %d]?s,start)", v.Retries)
			if timeout == nil {
				x.f(" same => n,Hangup()")
				continue
			}
			if timeout.PromptID != 0 {
				x.f(" same => n,Playback(%s)", promptPath(o, timeout.PromptID))
			}
			x.f(" same => n,%s", gotoNode(f.Next(timeout.ID, "out")))
		}
		x.nl()

		// The steps after a key: audio, destinations, time checks and "play the menu again".
		x.f("[%s]", flowCtx)
		for _, n := range f.Nodes {
			i, ok := steps[n.ID]
			if !ok {
				continue
			}
			x.f("exten => %d,1,NoOp(%s %s)", i, n.Type, esc(n.Label))
			switch n.Type {
			case pbx.NodeAudio:
				if n.PromptID != 0 {
					x.f(" same => n,Playback(%s)", promptPath(o, n.PromptID))
				}
				x.f(" same => n,%s", gotoNode(f.Next(n.ID, "out")))
			case pbx.NodeDestination:
				d := pbx.Destination{Type: pbx.DestHangup}
				if n.Destination != nil {
					d = *n.Destination
				}
				x.f(" same => n,%s", destGoto(d))
			case pbx.NodeTime:
				if n.Destination == nil || n.Destination.Type != pbx.DestTimeCondition {
					x.f(" same => n,Hangup()")
					continue
				}
				x.f(" same => n,Gosub(kiestoon-tcstate-%s,s,1)", n.Destination.Target)
				mark(x, "tc", "id="+n.Destination.Target, "state=${GOSUB_RETVAL}")
				x.f(` same => n,GotoIf($["${GOSUB_RETVAL}" = "open"]?open)`)
				x.f(" same => n,%s", gotoNode(f.Next(n.ID, "closed")))
				x.f(" same => n(open),%s", gotoNode(f.Next(n.ID, "open")))
			case pbx.NodeSpeech:
				genSpeechStep(x, o, v.ID, f, &n, gotoNode)
			case pbx.NodeRepeat:
				mark(x, "ivrrepeat", fmt.Sprintf("id=%d", v.ID))
				x.f(" same => n,Set(KT_TRIES=0)")
				x.f(" same => n,Goto(kiestoon-ivr-%d,s,start)", v.ID)
			}
		}
		x.nl()
	}
}

var weekdayNames = []string{"", "mon", "tue", "wed", "thu", "fri", "sat", "sun"}
var monthNames = []string{"", "jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// dayList turns [1 2 3 4 5] into "mon-fri" and [1 3] into "mon&wed".
func dayList(days []int) string {
	if len(days) == 0 {
		return "*"
	}
	contiguous := true
	for i := 1; i < len(days); i++ {
		if days[i] != days[i-1]+1 {
			contiguous = false
		}
	}
	if contiguous && len(days) > 1 {
		return weekdayNames[days[0]] + "-" + weekdayNames[days[len(days)-1]]
	}
	names := make([]string, len(days))
	for i, d := range days {
		names[i] = weekdayNames[d]
	}
	return strings.Join(names, "&")
}

func genTimeConditions(x *w, s *pbx.Snapshot) {
	tz := s.Settings.Timezone
	now := s.Now
	if now.IsZero() {
		now = time.Now()
	}
	holidays := s.Holidays.ForYear(now.Year())
	for _, t := range s.TimeConditions {
		// The schedule alone (used by the day/night feature code too).
		x.f("[kiestoon-tcauto-%d]", t.ID)
		x.f("exten => s,1,NoOp(Schedule %s)", esc(t.Name))
		if t.UseHolidays {
			for _, h := range holidays {
				d, err := time.Parse("2006-01-02", h.Date)
				if err != nil {
					continue
				}
				x.f(" same => n,GotoIfTime(*,*,%d,%s,%s?holiday)", d.Day(), monthNames[d.Month()], tz)
			}
		}
		for _, r := range t.Schedule {
			to := r.To
			if to == "24:00" {
				to = "23:59"
			}
			x.f(" same => n,GotoIfTime(%s-%s,%s,*,*,%s?open)", r.From, to, dayList(r.Days), tz)
		}
		x.f(" same => n,Return(closed)")
		x.f(" same => n(open),Return(open)")
		x.f(" same => n(holiday),Return(holiday)")
		x.nl()
		// The current state, including the manual switch.
		x.f("[kiestoon-tcstate-%d]", t.ID)
		switch t.Mode {
		case "open", "closed":
			x.f("exten => s,1,Return(%s)", t.Mode)
		default:
			x.f("exten => s,1,Gosub(kiestoon-tcauto-%d,s,1)", t.ID)
			x.f(" same => n,Return(${GOSUB_RETVAL})")
		}
		x.nl()
		x.f("[kiestoon-tc-%d]", t.ID)
		x.f("exten => s,1,Gosub(kiestoon-tcstate-%d,s,1)", t.ID)
		mark(x, "tc", fmt.Sprintf("id=%d", t.ID), "state=${GOSUB_RETVAL}")
		x.f(` same => n,GotoIf($["${GOSUB_RETVAL}" = "open"]?open)`)
		if t.Holiday.Type != "" {
			x.f(` same => n,GotoIf($["${GOSUB_RETVAL}" = "holiday"]?holiday)`)
		}
		x.f(" same => n,%s", destGoto(t.Closed))
		x.f(" same => n(open),%s", destGoto(t.Open))
		if t.Holiday.Type != "" {
			x.f(" same => n(holiday),%s", destGoto(t.Holiday))
		}
		x.nl()
	}
}

func genConferences(x *w, s *pbx.Snapshot) {
	for _, c := range s.Conferences {
		if !c.Enabled {
			continue
		}
		x.f("[kiestoon-conf-%d]", c.ID)
		x.f("exten => s,1,NoOp(Conference %s %s)", c.Number, esc(c.Name))
		mark(x, "conf", fmt.Sprintf("id=%d", c.ID))
		x.f(" same => n,Answer()")
		x.f(" same => n,Set(KT_PROFILE=kiestoon_user)")
		if c.WaitAdmin {
			x.f(" same => n,Set(KT_PROFILE=kiestoon_waiter)")
		}
		if c.PIN != "" || c.AdminPIN != "" {
			x.f(" same => n,Set(KT_TRIES=0)")
			x.f(" same => n(pin),Read(KT_PIN,conf-getpin,10,,1,10)")
			if c.AdminPIN != "" {
				x.f(` same => n,GotoIf($["${KT_PIN}" = "%s"]?admin)`, c.AdminPIN)
			}
			if c.PIN != "" {
				x.f(` same => n,GotoIf($["${KT_PIN}" = "%s"]?join)`, c.PIN)
			} else {
				x.f(" same => n,Goto(join)")
			}
			x.f(" same => n,Set(KT_TRIES=$[${KT_TRIES} + 1])")
			x.f(" same => n,Playback(conf-invalidpin)")
			x.f(" same => n,GotoIf($[${KT_TRIES} < 3]?pin)")
			x.f(" same => n,Hangup()")
			if c.AdminPIN != "" {
				x.f(" same => n(admin),Set(KT_PROFILE=kiestoon_admin)")
			}
		}
		x.f(" same => n(join),ConfBridge(kiestoon-%d,kiestoon_bridge_%d,${KT_PROFILE},kiestoon_menu)", c.ID, c.ID)
		x.f(" same => n,Hangup()")
		x.nl()
	}
}

// genConfBridge builds confbridge.conf with one bridge profile per room.
func genConfBridge(s *pbx.Snapshot) string {
	var x w
	x.b.WriteString(header)
	x.f("[general]")
	x.nl()
	user := func(name string, extra ...string) {
		x.f("[%s]", name)
		x.f("type=user")
		x.f("announce_user_count=yes")
		x.f("announce_join_leave=no")
		x.f("music_on_hold_when_empty=yes")
		x.f("dsp_drop_silence=yes")
		for _, e := range extra {
			x.f("%s", e)
		}
		x.nl()
	}
	user("kiestoon_user")
	user("kiestoon_waiter", "wait_marked=yes", "end_marked=yes")
	user("kiestoon_admin", "admin=yes", "marked=yes")
	x.f("[kiestoon_menu]")
	x.f("type=menu")
	x.f("*=playback_and_continue(conf-usermenu)")
	x.f("1=toggle_mute")
	x.f("4=decrease_listening_volume")
	x.f("6=increase_listening_volume")
	x.f("7=decrease_talking_volume")
	x.f("9=increase_talking_volume")
	x.f("2=admin_toggle_conference_lock")
	x.f("3=admin_kick_last")
	x.nl()
	for _, c := range s.Conferences {
		x.f("[kiestoon_bridge_%d]", c.ID)
		x.f("type=bridge")
		x.f("max_members=%d", c.MaxMembers)
		x.f("language=%s", s.Settings.Language)
		if c.Record {
			x.f("record_conference=yes")
			x.f("record_file=kt-conf%s-${STRFTIME(${EPOCH},,%%Y%%m%%d-%%H%%M%%S)}.wav", c.Number)
			x.f("record_file_timestamp=no")
		}
		x.nl()
	}
	return x.b.String()
}

// genParking builds res_parking.conf: park on 70, slots 71 and up.
func genParking(s *pbx.Snapshot) string {
	var x w
	x.b.WriteString(header)
	x.f("[general]")
	x.f("parkeddynamic=no")
	x.nl()
	n := s.Features.ParkingSlots
	if n <= 0 {
		return x.b.String()
	}
	x.f("[default]")
	x.f("context=kiestoon-parkedcalls")
	x.f("parkext=70")
	x.f("parkpos=%s-%s", pbx.ParkSlot(1), pbx.ParkSlot(n))
	x.f("parkinghints=yes")
	x.f("parkingtime=%d", s.Features.ParkingTimeout)
	x.f("comebacktoorigin=yes")
	x.f("comebackdialtime=30")
	x.f("findslot=next")
	x.f("parkedmusicclass=default")
	x.f("parkedcalltransfers=caller")
	x.f("parkedcallreparking=caller")
	return x.b.String()
}

// genFeatureCodes builds the star codes, with the current state of each extension baked in, so
// for example *76 knows whether it switches DND on or off. A change goes back to Kiestoon as a
// UserEvent over AMI; Kiestoon stores it and regenerates the dialplan.
func genFeatureCodes(x *w, s *pbx.Snapshot) {
	x.f("[kiestoon-features]")
	x.f("; *97 own voicemail, *98 any mailbox, *43 echo test")
	x.f("exten => *97,1,Answer()")
	x.f(" same => n,VoiceMailMain(${KT_EXT}@default)")
	x.f(" same => n,Hangup()")
	x.f("exten => *98,1,Answer()")
	x.f(" same => n,VoiceMailMain(@default)")
	x.f(" same => n,Hangup()")
	x.f("exten => *43,1,Answer()")
	x.f(" same => n,Playback(demo-echotest)")
	x.f(" same => n,Echo()")
	x.f(" same => n,Hangup()")
	x.f("exten => *76,1,Goto(kiestoon-fc-${KT_EXT},dnd,1)")
	x.f("exten => *73,1,Goto(kiestoon-fc-${KT_EXT},fwdoff,1)")
	x.f("exten => _*72X.,1,Set(KT_TO=${FILTER(0-9+,${EXTEN:3})})")
	x.f(" same => n,Goto(kiestoon-fc-${KT_EXT},fwdon,1)")
	x.f("exten => *45,1,Goto(kiestoon-fc-${KT_EXT},queues,1)")
	x.f("exten => _*45X.,1,Goto(kiestoon-fc-${KT_EXT},q${EXTEN:3},1)")
	x.f("exten => *8,1,Pickup()")
	x.f(" same => n,Hangup()")
	x.f("exten => _**X.,1,Pickup(${EXTEN:2}@PICKUPMARK)")
	x.f(" same => n,Hangup()")
	x.f("exten => _*80X.,1,Goto(kiestoon-intercom,${FILTER(0-9,${EXTEN:3})},1)")
	x.f("; *33 listen, *34 whisper, *35 barge on a queue call of <extension> (supervisors only)")
	x.f("exten => _*3[345]X.,1,GotoIf(${DIALPLAN_EXISTS(kiestoon-spy-${KT_EXT},${EXTEN:2},1)}?kiestoon-spy-${KT_EXT},${EXTEN:2},1)")
	x.f(" same => n,Answer()")
	x.f(" same => n,Playback(pbx-invalid)")
	x.f(" same => n,Hangup()")
	for _, t := range s.TimeConditions {
		x.f("exten => *28%d,hint,Custom:kt-tc-%d", t.ID, t.ID)
		x.f("exten => *28%d,1,Answer()", t.ID)
		if t.Mode != "auto" {
			// Manual switch is on: back to the schedule.
			x.f(" same => n,UserEvent(Kiestoon,Action: tcmode,Id: %d,Value: auto,Ext: ${KT_EXT})", t.ID)
			x.f(" same => n,Playback(de-activated)")
		} else {
			x.f(" same => n,Gosub(kiestoon-tcauto-%d,s,1)", t.ID)
			x.f(` same => n,Set(KT_NEW=${IF($["${GOSUB_RETVAL}" = "open"]?closed:open)})`)
			x.f(" same => n,UserEvent(Kiestoon,Action: tcmode,Id: %d,Value: ${KT_NEW},Ext: ${KT_EXT})", t.ID)
			x.f(" same => n,Playback(activated)")
		}
		x.f(" same => n,Hangup()")
	}
	x.nl()

	queuesOf := map[int64][]pbx.Queue{}
	for _, q := range s.Queues {
		for _, m := range q.Members {
			if m.Dynamic {
				queuesOf[m.ExtensionID] = append(queuesOf[m.ExtensionID], q)
			}
		}
	}
	for _, e := range s.Extensions {
		if !e.Enabled {
			continue
		}
		x.f("[kiestoon-fc-%s]", e.Number)
		x.f("exten => dnd,1,Answer()")
		if e.Features.DND {
			x.f(" same => n,UserEvent(Kiestoon,Action: dnd,Ext: %s,Value: off)", e.Number)
			x.f(" same => n,Playback(do-not-disturb&de-activated)")
		} else {
			x.f(" same => n,UserEvent(Kiestoon,Action: dnd,Ext: %s,Value: on)", e.Number)
			x.f(" same => n,Playback(do-not-disturb&activated)")
		}
		x.f(" same => n,Hangup()")
		x.f("exten => fwdoff,1,Answer()")
		x.f(" same => n,UserEvent(Kiestoon,Action: forward,Ext: %s,Value: )", e.Number)
		x.f(" same => n,Playback(call-fwd-cancelled)")
		x.f(" same => n,Hangup()")
		x.f("exten => fwdon,1,Answer()")
		x.f(" same => n,UserEvent(Kiestoon,Action: forward,Ext: %s,Value: ${KT_TO})", e.Number)
		x.f(" same => n,Playback(call-fwd-unconditional)")
		x.f(" same => n,SayDigits(${KT_TO})")
		x.f(" same => n,Hangup()")
		qs := queuesOf[e.ID]
		anyIn := false
		for _, q := range qs {
			for _, m := range q.Members {
				if m.ExtensionID == e.ID && m.LoggedIn {
					anyIn = true
				}
			}
		}
		x.f("exten => queues,1,Answer()")
		switch {
		case len(qs) == 0:
			x.f(" same => n,Playback(pbx-invalid)")
		case anyIn:
			x.f(" same => n,UserEvent(Kiestoon,Action: queue,Ext: %s,Queue: 0,Value: out)", e.Number)
			x.f(" same => n,Playback(agent-loggedoff)")
		default:
			x.f(" same => n,UserEvent(Kiestoon,Action: queue,Ext: %s,Queue: 0,Value: in)", e.Number)
			x.f(" same => n,Playback(agent-loginok)")
		}
		x.f(" same => n,Hangup()")
		for _, q := range qs {
			in := false
			for _, m := range q.Members {
				if m.ExtensionID == e.ID && m.LoggedIn {
					in = true
				}
			}
			x.f("exten => q%s,1,Answer()", q.Number)
			if in {
				x.f(" same => n,UserEvent(Kiestoon,Action: queue,Ext: %s,Queue: %d,Value: out)", e.Number, q.ID)
				x.f(" same => n,Playback(agent-loggedoff)")
			} else {
				x.f(" same => n,UserEvent(Kiestoon,Action: queue,Ext: %s,Queue: %d,Value: in)", e.Number, q.ID)
				x.f(" same => n,Playback(agent-loginok)")
			}
			x.f(" same => n,Hangup()")
		}
		x.nl()
	}
}

// genAllowed checks an outbound number against the caller's rights and the fraud settings.
// It returns "ok" or "deny" in GOSUB_RETVAL.
func genAllowed(x *w, s *pbx.Snapshot) {
	cc := s.Settings.CountryCode
	fr := s.Features.Fraud
	x.f("[kiestoon-allowed]")
	x.f("exten => s,1,Set(KT_N=${ARG1})")
	x.f(` same => n,ExecIf($["${KT_N:0:%d}" = "+%s"]?Set(KT_N=0${KT_N:%d}))`, len(cc)+1, cc, len(cc)+1)
	x.f(` same => n,ExecIf($["${KT_N:0:%d}" = "00%s"]?Set(KT_N=0${KT_N:%d}))`, len(cc)+2, cc, len(cc)+2)
	x.f(` same => n,ExecIf($["${KT_N:0:1}" = "+"]?Set(KT_N=00${KT_N:1}))`)
	x.f(" same => n,Set(KT_C=national)")
	x.f(` same => n,ExecIf($["${KT_N:0:2}" = "00"]?Set(KT_C=international))`)
	for _, c := range fr.BlockedCountries {
		x.f(` same => n,ExecIf($["${KT_N:0:%d}" = "00%s"]?Set(KT_C=blocked))`, len(c)+2, c)
	}
	for _, p := range fr.PremiumPrefixes {
		x.f(` same => n,ExecIf($["${KT_C}" = "national" & "${KT_N:0:%d}" = "%s"]?Set(KT_C=premium))`, len(p), p)
	}
	x.f(` same => n,GotoIf($["${KT_C}" = "blocked"]?deny)`)
	x.f(` same => n,GotoIf($["${KT_CLASS}" = "internal"]?deny)`)
	x.f(` same => n,GotoIf($["${KT_C}" = "international" & "${KT_CLASS}" != "international"]?deny)`)
	x.f(` same => n,GotoIf($["${KT_C}" = "premium" & "${KT_PREMIUM}" != "1"]?deny)`)
	if s.FraudLocked {
		x.f(` same => n,GotoIf($["${KT_C}" = "international" | "${KT_C}" = "premium"]?deny)`)
	}
	if fr.MaxOutbound > 0 {
		x.f(` same => n,GotoIf($[${GROUP_COUNT(out@kt_out)} >= %d]?full)`, fr.MaxOutbound)
		x.f(" same => n,Set(GROUP(kt_out)=out)")
	}
	x.f(" same => n,Set(CDR(kt_class)=${KT_C})")
	x.f(" same => n,Return(ok)")
	x.f(" same => n(deny),NoOp(Outbound call to ${KT_N} (${KT_C}) not allowed for class ${KT_CLASS})")
	x.f(" same => n,Set(CDR(kt_blocked)=${KT_C})")
	x.f(" same => n,Return(deny)")
	x.f(" same => n(full),NoOp(Too many outbound calls)")
	x.f(" same => n,Set(CDR(kt_blocked)=limit)")
	x.f(" same => n,Return(deny)")
	x.nl()
}

// genSpeechStep asks a question, records the answer and lets the server recognise it (FastAGI).
// The server sets KT_INTENT (the intent number, empty when nothing matched) and KT_INTENTL (its
// label, safe for the journey mark). Without speech support the call takes the "nomatch" line.
func genSpeechStep(x *w, o Options, ivr int64, f *pbx.Flow, n *pbx.FlowNode, gotoNode func(*pbx.FlowNode) string) {
	if n.PromptID != 0 {
		x.f(" same => n,Playback(%s)", promptPath(o, n.PromptID))
	}
	if o.SpeechDir == "" || o.AGIAddr == "" || len(n.Intents) == 0 {
		mark(x, "ivrspeech", fmt.Sprintf("id=%d", ivr), "intent=")
		x.f(" same => n,%s", gotoNode(f.Next(n.ID, "nomatch")))
		return
	}
	x.f(" same => n,Set(KT_INTENT=)")
	x.f(" same => n,Set(KT_INTENTL=)")
	x.f(" same => n,Set(KT_SPF=%s/${UNIQUEID}-${EPOCH}.wav)", o.SpeechDir)
	// Up to 12 seconds; 3 seconds of silence ends it. q: no beep.
	x.f(" same => n,Record(${KT_SPF},3,12,q)")
	x.f(" same => n,StartMusicOnHold()")
	x.f(" same => n,AGI(agi://%s/speech,%d,%s,${KT_SPF})", o.AGIAddr, ivr, n.ID)
	x.f(" same => n,StopMusicOnHold()")
	mark(x, "ivrspeech", fmt.Sprintf("id=%d", ivr), "intent=${KT_INTENTL}")
	for i := range n.Intents {
		x.f(` same => n,GotoIf($["${KT_INTENT}" = "%d"]?%s)`, i+1, pbx.SpeechPort(i+1))
	}
	x.f(" same => n,%s", gotoNode(f.Next(n.ID, "nomatch")))
	for i := range n.Intents {
		x.f(" same => n(%s),%s", pbx.SpeechPort(i+1), gotoNode(f.Next(n.ID, pbx.SpeechPort(i+1))))
	}
}
