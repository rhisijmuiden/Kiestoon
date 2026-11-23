package asterisk

import (
	"fmt"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Microsoft Teams Direct Routing.
//
// Kiestoon is the SBC of the Teams tenant. Microsoft only accepts SIP over TLS with a public
// certificate for the SBC's FQDN, and needs that FQDN in the Contact and Via headers. That is why
// Teams has its own TLS transport (transport-teams) with external_signaling_hostname (Asterisk
// 22.11+), on its own port. Media is SRTP (SDES). Calls to Teams try Microsoft's three SIP proxies
// in turn; calls from Teams are recognised by Microsoft's address ranges.

// TeamsEndpoint is the PJSIP endpoint of Microsoft Teams.
const TeamsEndpoint = "kt-teams"

// TeamsAOR is the AOR (and qualify target) of hub i (0-based) of pbx.TeamsHubs.
func TeamsAOR(i int) string { return fmt.Sprintf("kt-teams-%d", i+1) }

// TeamsCodecs are the codecs offered to Teams. Microsoft's media processors take G.722 and G.711.
const TeamsCodecs = "g722,alaw,ulaw"

// teamsActive reports whether Direct Routing goes into the configuration: switched on, with a
// domain name and a public certificate (Microsoft refuses self-signed certificates).
func teamsActive(s *pbx.Snapshot, o Options) bool {
	return s.Teams.Enabled && s.Teams.FQDN != "" && o.PublicCert != "" && o.PublicKey != ""
}

// CABundle is the system list of public CAs, used to check Microsoft's certificate.
const CABundle = "/etc/ssl/certs/ca-certificates.crt"

func genTeamsPJSIP(x *w, s *pbx.Snapshot, o Options) {
	if !teamsActive(s, o) {
		return
	}
	t := s.Teams
	st := s.Settings
	x.f("; ---- Microsoft Teams Direct Routing")
	x.f("[transport-teams]")
	x.f("type=transport")
	x.f("protocol=tls")
	x.f("bind=0.0.0.0:%d", t.Port)
	x.f("cert_file=%s", esc(o.PublicCert))
	x.f("priv_key_file=%s", esc(o.PublicKey))
	x.f("ca_list_file=%s", CABundle)
	x.f("method=tlsv1_2")
	x.f("verify_client=no")
	x.f("verify_server=yes")
	x.f("external_signaling_hostname=%s", esc(t.FQDN))
	x.f("external_signaling_port=%d", t.Port)
	if st.ExternalAddress != "" {
		x.f("external_media_address=%s", esc(st.ExternalAddress))
	}
	for _, n := range st.LocalNets {
		x.f("local_net=%s", esc(n))
	}
	x.f("tos=cs3")
	x.f("cos=3")
	x.nl()
	aors := []string{}
	for i, h := range pbx.TeamsHubs {
		aors = append(aors, TeamsAOR(i))
		x.f("[%s]", TeamsAOR(i))
		x.f("type=aor")
		x.f("contact=sip:%s:5061;transport=tls", h)
		x.f("qualify_frequency=60")
		x.f("qualify_timeout=5")
		x.nl()
	}
	x.f("[%s]", TeamsEndpoint)
	x.f("type=endpoint")
	x.f("transport=transport-teams")
	x.f("context=kiestoon-from-teams")
	codecLines(x, TeamsCodecs)
	x.f("aors=%s", strings.Join(aors, ","))
	x.f("from_domain=%s", esc(t.FQDN))
	x.f("media_encryption=sdes")
	x.f("media_encryption_optimistic=no")
	x.f("direct_media=no")
	x.f("rtp_symmetric=yes")
	x.f("rewrite_contact=no")
	x.f("dtmf_mode=rfc4733")
	x.f("send_pai=yes")
	x.f("trust_id_inbound=yes")
	x.f("trust_id_outbound=yes")
	x.f("refer_blind_progress=no")
	x.f("timers=yes")
	x.f("tos_audio=ef")
	x.f("cos_audio=5")
	x.f("language=%s", st.Language)
	x.f("set_var=KT_TEAMS=1")
	x.f("set_var=KT_CLASS=international")
	x.f("set_var=KT_PREMIUM=0")
	x.nl()
	x.f("[%s]", TeamsEndpoint)
	x.f("type=identify")
	x.f("endpoint=%s", TeamsEndpoint)
	for _, n := range pbx.TeamsSignalingNets {
		x.f("match=%s", n)
	}
	x.nl()
}

// teamsE164 writes dialplan lines that turn variable v into E.164 (+31…) for Teams: national
// numbers get the country code, 00 becomes +. Short internal numbers stay as they are.
func teamsE164(x *w, v, cc string, digits int) {
	x.f(` same => n,ExecIf($["${%s:0:2}" = "00"]?Set(%s=+${%s:2}))`, v, v, v)
	x.f(` same => n,ExecIf($["${%s:0:1}" = "0" & ${LEN(${%s})} > %d]?Set(%s=+%s${%s:1}))`, v, v, digits, v, cc, v)
}

func genTeamsDialplan(x *w, s *pbx.Snapshot, o Options) {
	if !teamsActive(s, o) {
		return
	}
	t := s.Teams
	st := s.Settings
	cc := st.CountryCode
	exts := map[string]pbx.Extension{}
	for _, e := range s.Extensions {
		exts[e.Number] = e
	}

	// Ring a Teams number (digits without +). Tries Microsoft's proxies in turn.
	x.f("[kiestoon-teams-out]")
	x.f("exten => _X.,1,NoOp(To Microsoft Teams +${EXTEN})")
	x.f(" same => n,Set(KT_TCID=${CALLERID(num)})")
	teamsE164(x, "KT_TCID", cc, st.Digits)
	x.f(" same => n,Set(CALLERID(num)=${KT_TCID})")
	x.f(" same => n,Set(CDR(kt_trunk)=Microsoft Teams)")
	for i, h := range pbx.TeamsHubs {
		x.f(" same => n,Dial(PJSIP/%s/sip:+${EXTEN}@%s:5061\\;transport=tls\\;user=phone,,T)", TeamsEndpoint, h)
		if i < len(pbx.TeamsHubs)-1 {
			x.f(` same => n,GotoIf($["${DIALSTATUS}" != "CHANUNAVAIL" & "${DIALSTATUS}" != "CONGESTION"]?done)`)
		}
	}
	x.f(` same => n(done),GotoIf($["${DIALSTATUS}" = "BUSY"]?busy)`)
	x.f(" same => n,Hangup()")
	x.f(" same => n(busy),Busy(5)")
	x.nl()

	// Teams users: who is calling, and with which rights.
	x.f("[kiestoon-teams-user]")
	for _, u := range t.Users {
		x.f("exten => %s,1,NoOp(Teams user %s)", u.Digits(), esc(u.UPN))
		x.f(" same => n,Set(KT_TUSER=%s)", u.Digits())
		if t.CallerID == "own" {
			x.f(" same => n,Set(KT_OUTCID=%s)", u.Number)
		}
		if e, ok := exts[u.Extension]; ok && e.Enabled {
			x.f(" same => n,Set(KT_TEXT=%s)", e.Number)
			if g := groupVar(s, e); g != "" {
				x.f(" same => n,Set(KT_GROUP=%s)", g)
			}
			x.f(" same => n,Set(KT_CLASS=%s)", e.OutboundClass)
			x.f(" same => n,Set(KT_PREMIUM=%d)", b2i(e.AllowPremium))
			x.f(" same => n,Set(KT_TNAME=%s)", esc(strings.NewReplacer(",", " ", "(", "", ")", "", `"`, "").Replace(e.Name)))
		} else {
			// Without an extension: national calls, no premium numbers.
			x.f(" same => n,Set(KT_CLASS=national)")
			x.f(" same => n,Set(KT_PREMIUM=0)")
		}
		x.f(" same => n,Return()")
	}
	x.nl()

	// Calls from Teams users. Only known Teams numbers may call out; everyone else reaches
	// internal numbers only.
	x.f("[kiestoon-from-teams]")
	for _, p := range []string{"_X.", "_+X.", "_*.", "_#."} {
		x.f("exten => %s,1,NoOp(From Microsoft Teams ${CALLERID(num)} to ${EXTEN})", p)
		x.f(" same => n,Goto(kiestoon-from-teams-go,${FILTER(0-9+*#,${EXTEN})},1)")
	}
	x.nl()
	x.f("[kiestoon-from-teams-go]")
	x.f("exten => _[+0-9*#].,1,Set(KT_TNUM=${FILTER(0-9,${CALLERID(num)})})")
	x.f(" same => n,Set(KT_TUSER=)")
	x.f(" same => n,Set(KT_CLASS=internal)")
	x.f(` same => n,ExecIf($[${DIALPLAN_EXISTS(kiestoon-teams-user,${KT_TNUM},1)}]?Gosub(kiestoon-teams-user,${KT_TNUM},1))`)
	mark(x, "teams", "from=${KT_TNUM}")
	// Internal numbers: the caller shows as their extension, like a desk phone.
	x.f(` same => n,GotoIf($[${DIALPLAN_EXISTS(kiestoon-local,${EXTEN},1)}]?internal)`)
	x.f(" same => n,Set(KT_NUM=${EXTEN})")
	cl := len(cc)
	x.f(` same => n,ExecIf($["${KT_NUM:0:%d}" = "+%s"]?Set(KT_NUM=0${KT_NUM:%d}))`, cl+1, cc, cl+1)
	x.f(` same => n,ExecIf($["${KT_NUM:0:1}" = "+"]?Set(KT_NUM=00${KT_NUM:1}))`)
	x.f(` same => n,GotoIf($[${DIALPLAN_EXISTS(kiestoon-emergency,${KT_NUM},1)}]?kiestoon-emergency,${KT_NUM},1)`)
	x.f(` same => n,GotoIf($["${KT_TUSER}" = ""]?denied)`)
	if tr, ok := enabledTrunk(s, t.TrunkID); ok {
		x.f(" same => n,Gosub(kiestoon-allowed,s,1(${KT_NUM}))")
		x.f(` same => n,GotoIf($["${GOSUB_RETVAL}" != "ok"]?denied)`)
		cid := tr.CallerID
		if cid != "" {
			x.f(` same => n,Set(CALLERID(num)=${IF($["${KT_OUTCID}" != ""]?${KT_OUTCID}:%s)})`, cid)
		} else {
			x.f(` same => n,ExecIf($["${KT_OUTCID}" != ""]?Set(CALLERID(num)=${KT_OUTCID}))`)
		}
		x.f(" same => n,Set(CDR(kt_trunk)=%s)", esc(tr.Name))
		mark(x, "trunk", fmt.Sprintf("id=%d", tr.ID))
		x.f(" same => n,Dial(PJSIP/${KT_NUM}@%s,,T)", tr.Endpoint())
		x.f(" same => n,Hangup()")
	} else {
		x.f(` same => n,GotoIf($[${DIALPLAN_EXISTS(kiestoon-outbound,${KT_NUM},1)}]?kiestoon-outbound,${KT_NUM},1)`)
		x.f(" same => n,Goto(denied)")
	}
	x.f(` same => n(internal),ExecIf($["${KT_TEXT}" != ""]?Set(CALLERID(num)=${KT_TEXT}))`)
	x.f(` same => n,ExecIf($["${KT_TNAME}" != ""]?Set(CALLERID(name)=${KT_TNAME}))`)
	x.f(` same => n,ExecIf($["${KT_TEXT}" != ""]?Set(KT_EXT=${KT_TEXT}))`)
	x.f(" same => n,Goto(kiestoon-local,${EXTEN},1)")
	x.f(" same => n(denied),Playback(ss-noservice)")
	x.f(" same => n,Hangup()")
	x.nl()
}

func enabledTrunk(s *pbx.Snapshot, id int64) (pbx.Trunk, bool) {
	if id == 0 {
		return pbx.Trunk{}, false
	}
	for _, t := range s.Trunks {
		if t.ID == id && t.Enabled {
			return t, true
		}
	}
	return pbx.Trunk{}, false
}

// genTeamsDIDs sends calls to the number of a Teams user without an extension straight to Teams,
// unless an inbound route already handles that number. done holds the numbers already written.
func genTeamsDIDs(x *w, s *pbx.Snapshot, o Options, done map[string]bool) {
	if !teamsActive(s, o) {
		return
	}
	for _, u := range s.Teams.Users {
		if u.Extension != "" {
			continue
		}
		vs := pbx.DIDVariants(u.Number, s.Settings.CountryCode)
		taken := false
		for _, v := range vs {
			if done[v] {
				taken = true
			}
		}
		if taken {
			continue
		}
		for _, v := range vs {
			done[v] = true
			x.f("exten => %s,1,NoOp(Teams user %s)", v, esc(u.UPN))
			mark(x, "teamsin", "num="+u.Digits())
			x.f(" same => n,Dial(Local/%s@kiestoon-teams-out/n,,tT)", u.Digits())
			x.f(" same => n,Hangup()")
		}
	}
}

// withoutTeams returns the extensions with their Teams link removed, for when Direct Routing is
// not active: calls then ring the extension's own devices.
func withoutTeams(in []pbx.Extension) []pbx.Extension {
	out := make([]pbx.Extension, len(in))
	copy(out, in)
	for i := range out {
		out[i].TeamsNumber, out[i].TeamsOnly = "", false
	}
	return out
}
