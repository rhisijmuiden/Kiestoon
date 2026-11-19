package asterisk

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Multi-site (phase 5).
//
// Edges: phones in a branch office register through the Kamailio proxy of a Kiestoon Edge, which
// adds a Path header. support_path makes Asterisk send calls back through that proxy.
//
// Federation: every linked Kiestoon is a PJSIP endpoint fed-<id> on the other end of a WireGuard
// tunnel. Its numbers are dialled as <prefix><number>; the BLF of those numbers is mirrored in
// custom device states (Custom:ktfed<id>-<number>) that the portal updates from the other system.

// activePeers returns the federation links that completed pairing, sorted by ID.
func activePeers(s *pbx.Snapshot) []pbx.FedPeer {
	out := []pbx.FedPeer{}
	for _, p := range s.Peers {
		if p.State == "active" && p.TunnelIP != "" && p.Prefix != "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// tunnelNets are the tunnel ranges Asterisk must treat as local (no external address in the SDP).
func tunnelNets(s *pbx.Snapshot) []string {
	nets := []string{}
	if s.Hub.Enabled && s.Hub.Subnet != "" {
		nets = append(nets, s.Hub.Subnet)
	}
	for _, p := range activePeers(s) {
		if p.Role == "joined" {
			nets = append(nets, p.TunnelIP+"/32")
		}
	}
	// Leave out what the configured local networks already cover.
	out := []string{}
	for _, n := range nets {
		pfx, err := netip.ParsePrefix(n)
		if err != nil {
			continue
		}
		covered := false
		for _, l := range s.Settings.LocalNets {
			if lp, err := netip.ParsePrefix(l); err == nil && lp.Bits() <= pfx.Bits() && lp.Contains(pfx.Addr()) {
				covered = true
			}
		}
		if !covered {
			out = append(out, pfx.Masked().String())
		}
	}
	return out
}

func genFederationPJSIP(x *w, s *pbx.Snapshot) {
	for _, p := range activePeers(s) {
		ep := p.SIPEndpoint()
		x.f("; ---- linked Kiestoon %d: %s", p.ID, esc(p.Name))
		x.f("[%s]", ep)
		x.f("type=aor")
		x.f("contact=sip:%s:%d;transport=tcp", p.TunnelIP, p.SIPPort)
		x.f("qualify_frequency=30")
		x.nl()
		x.f("[%s]", ep)
		x.f("type=endpoint")
		x.f("transport=transport-tcp")
		x.f("context=kiestoon-from-fed-%d", p.ID)
		codecLines(x, s.Settings.Codecs)
		x.f("aors=%s", ep)
		x.f("direct_media=no")
		x.f("rtp_symmetric=yes")
		x.f("dtmf_mode=rfc4733")
		x.f("send_pai=yes")
		x.f("trust_id_inbound=yes")
		x.f("trust_id_outbound=yes")
		x.f("tos_audio=ef")
		x.f("cos_audio=5")
		x.f("language=%s", s.Settings.Language)
		x.f("set_var=KT_CLASS=internal")
		x.f("set_var=KT_PREMIUM=0")
		x.f("set_var=KT_FED=%d", p.ID)
		x.nl()
		x.f("[%s]", ep)
		x.f("type=identify")
		x.f("endpoint=%s", ep)
		x.f("match=%s", p.TunnelIP)
		x.nl()
	}
}

// FedDeviceState maps a BLF state from the other system to an Asterisk device state.
func FedDeviceState(status string) string {
	switch status {
	case "idle":
		return "NOT_INUSE"
	case "inuse":
		return "INUSE"
	case "busy":
		return "BUSY"
	case "ringing":
		return "RINGING"
	case "ringinuse":
		return "RINGINUSE"
	case "onhold":
		return "ONHOLD"
	}
	return "UNAVAILABLE"
}

func localSet(s *pbx.Snapshot) map[string]bool {
	m := map[string]bool{}
	for _, e := range s.Extensions {
		m[e.Number] = true
	}
	for _, g := range s.RingGroups {
		m[g.Number] = true
	}
	for _, q := range s.Queues {
		m[q.Number] = true
	}
	for _, c := range s.Conferences {
		m[c.Number] = true
	}
	for _, r := range pbx.Reserved {
		m[r] = true
	}
	return m
}

func genFederationDialplan(x *w, s *pbx.Snapshot) {
	peers := activePeers(s)
	// Dialled from phones (included in kiestoon-internal, before the outside lines).
	x.f("[kiestoon-fed]")
	local := localSet(s)
	direct := map[string]bool{}
	for _, p := range peers {
		x.f("; %s: dial %s + number", esc(p.Name), p.Prefix)
		x.f("exten => _%sX.,1,Goto(kiestoon-fedout-%d,${EXTEN:%d},1)", p.Prefix, p.ID, len(p.Prefix))
		for _, e := range p.Directory.Entries {
			if !validRemote(e.Number) {
				continue
			}
			if e.Kind == "extension" {
				x.f("exten => %s%s,hint,%s", p.Prefix, e.Number, p.DeviceState(e.Number))
			}
			// Without a prefix too, when the number plans do not overlap.
			if p.DirectDial && !local[e.Number] && !direct[e.Number] {
				direct[e.Number] = true
				x.f("exten => %s,1,Goto(kiestoon-fedout-%d,%s,1)", e.Number, p.ID, e.Number)
			}
		}
	}
	x.nl()
	// The caller's own number is marked internal, so the other side can show it with our prefix.
	x.f("[kiestoon-fedhdr]")
	x.f("exten => s,1,Set(PJSIP_HEADER(add,X-Kiestoon-Internal)=${ARG1})")
	x.f(" same => n,Return()")
	x.nl()
	for _, p := range peers {
		x.f("[kiestoon-fedout-%d]", p.ID)
		x.f("exten => _X.,1,NoOp(To %s)", esc(p.Name))
		x.f(` same => n,Set(KT_INTERNAL=${IF($["${KT_EXT}" != ""]?1:0)})`)
		x.f(" same => n,Dial(PJSIP/${EXTEN}@%s,90,Tb(kiestoon-fedhdr^s^1(${KT_INTERNAL})))", p.SIPEndpoint())
		x.f(` same => n,GotoIf($["${DIALSTATUS}" = "BUSY"]?busy)`)
		x.f(" same => n,Hangup()")
		x.f(" same => n(busy),Busy(10)")
		x.nl()
		// Calls from the other system may reach internal numbers only: never the outside lines.
		x.f("[kiestoon-from-fed-%d]", p.ID)
		x.f("exten => _X.,1,NoOp(From %s)", esc(p.Name))
		x.f(` same => n,ExecIf($["${PJSIP_HEADER(read,X-Kiestoon-Internal)}" = "1"]?Set(CALLERID(num)=%s${CALLERID(num)}))`, p.Prefix)
		x.f(` same => n,ExecIf($["${PJSIP_HEADER(read,X-Kiestoon-Internal)}" = "1"]?Set(CALLERID(name)=${CALLERID(name)} (%s)))`, esc(strings.ReplaceAll(p.Name, ")", "")))
		x.f(` same => n,GotoIf($[${DIALPLAN_EXISTS(kiestoon-local,${EXTEN},1)}]?kiestoon-local,${EXTEN},1)`)
		x.f(" same => n,Playback(ss-noservice)")
		x.f(" same => n,Hangup()")
		x.nl()
	}
}

func validRemote(n string) bool {
	if len(n) < 2 || len(n) > 10 {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// genSunRoutes builds the follow-the-sun routes: GotoIfTime with the time zone of every rule.
func genSunRoutes(x *w, s *pbx.Snapshot) {
	for _, r := range s.SunRoutes {
		x.f("[kiestoon-sun-%d]", r.ID)
		x.f("exten => s,1,NoOp(Follow the sun %s)", esc(r.Name))
		if r.Enabled {
			for i, rule := range r.Rules {
				to := rule.To
				if to == "24:00" {
					to = "23:59"
				}
				x.f(" same => n,GotoIfTime(%s-%s,%s,*,*,%s?r%d)", rule.From, to, dayList(rule.Days), rule.Timezone, i+1)
			}
		}
		x.f(" same => n,%s", destGoto(r.Fallback))
		if r.Enabled {
			for i, rule := range r.Rules {
				x.f(" same => n(r%d),%s", i+1, destGoto(rule.Destination))
			}
		}
		x.nl()
	}
}

// remoteGoto is the dialplan for a destination on a linked system.
func remoteGoto(target string) string {
	id, num, ok := pbx.ParseRemote(target)
	if !ok {
		return "Hangup()"
	}
	return fmt.Sprintf("Goto(kiestoon-fedout-%d,%s,1)", id, num)
}

// remoteQueueMembers are the queues.conf member lines for agents on linked systems.
func remoteQueueMembers(x *w, s *pbx.Snapshot, q pbx.Queue) {
	peers := map[int64]pbx.FedPeer{}
	for _, p := range activePeers(s) {
		peers[p.ID] = p
	}
	for _, m := range q.RemoteMembers {
		p, ok := peers[m.PeerID]
		if !ok || !validRemote(m.Number) {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.Number
		}
		x.f("member => Local/%s@kiestoon-fedout-%d/n,%d,%s (%s),%s", m.Number, p.ID, m.Penalty,
			esc(strings.ReplaceAll(name, ",", " ")), esc(strings.ReplaceAll(p.Name, ",", " ")), p.DeviceState(m.Number))
	}
}
