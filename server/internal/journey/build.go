package journey

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Journey is a call as the call history shows it: a summary and the steps in order.
type Journey struct {
	LinkedID   string    `json:"linkedId"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	DurationMs int64     `json:"durationMs"`
	TalkMs     int64     `json:"talkMs"`
	WaitMs     int64     `json:"waitMs"`
	Direction  string    `json:"direction"` // inbound, outbound, internal
	From       Party     `json:"from"`
	To         string    `json:"to"`
	// Result: answered, voicemail, abandoned, missed, busy, failed.
	Result string `json:"result"`
	// Complete is false while the call is still going on (no LINKEDID_END yet).
	Complete bool `json:"complete"`
	// Partial is true when only the call detail records were there to build it from.
	Partial    bool        `json:"partial"`
	Steps      []*Step     `json:"steps"`
	Recordings []Recording `json:"recordings"`
}

// Recording is a recording that belongs to the call.
type Recording struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"durationMs"`
	From       string `json:"from"`
	To         string `json:"to"`
}

// Party is someone on the call.
type Party struct {
	Kind   string `json:"kind"` // ext, caller, remote, system
	Number string `json:"number,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Step is one box in the flow.
type Step struct {
	ID   int    `json:"id"`
	Kind string `json:"kind"`
	// inbound, internal, outbound (start), route, timecondition, ivr, queue, ringgroup, extension,
	// talk, transfer, voicemail, forward, park, pickup, conference, fax, external, noroute, hangup
	At         time.Time         `json:"at"`
	DurationMs int64             `json:"durationMs"`
	Ref        string            `json:"ref,omitempty"` // id of the IVR, queue... (for a link)
	Name       string            `json:"name,omitempty"`
	Number     string            `json:"number,omitempty"`
	Result     string            `json:"result,omitempty"`
	Tone       string            `json:"tone"` // ok, warn, danger, info, accent
	Party      *Party            `json:"party,omitempty"`
	Detail     map[string]string `json:"detail,omitempty"`
	Subs       []*Sub            `json:"subs,omitempty"`
	end        time.Time
	closed     bool
}

// Sub is a detail inside a step: a key pressed in an IVR, a phone that rang.
type Sub struct {
	Kind       string    `json:"kind"` // key, timeout, invalid, repeat, ring
	At         time.Time `json:"at"`
	DurationMs int64     `json:"durationMs"`
	Value      string    `json:"value,omitempty"`
	Party      *Party    `json:"party,omitempty"`
	Result     string    `json:"result,omitempty"` // answered, noanswer, busy, cancel, unavailable
	id         string
	open       bool
}

// Names resolves ids to the names an admin gave them. kind: ext, ivr, queue, rg, tc, route,
// outroute, trunk, conf, fax. An empty result keeps the id.
type Names func(kind, id string) string

var reEndpoint = regexp.MustCompile(`^(?:PJSIP|SIP)/(.+)-[0-9a-f]{6,}(?:;\d)?$`)
var reTrunk = regexp.MustCompile(`^trunk-(\d+)$`)
var reLocal = regexp.MustCompile(`^Local/([^@]+)@`)

// partyOf names the owner of a channel.
func partyOf(ch string, names Names, inbound bool) Party {
	if m := reEndpoint.FindStringSubmatch(ch); m != nil {
		ep := m[1]
		if t := reTrunk.FindStringSubmatch(ep); t != nil {
			kind := "remote"
			if inbound {
				kind = "caller"
			}
			return Party{Kind: kind, Name: names("trunk", t[1])}
		}
		ep = strings.TrimSuffix(ep, "-web")
		if isDigits(ep) {
			return Party{Kind: "ext", Number: ep, Name: names("ext", ep)}
		}
		return Party{Kind: "remote", Name: ep}
	}
	if m := reLocal.FindStringSubmatch(ch); m != nil && isDigits(m[1]) {
		return Party{Kind: "ext", Number: m[1], Name: names("ext", m[1])}
	}
	return Party{Kind: "system"}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isLocal(ch string) bool { return strings.HasPrefix(ch, "Local/") }

func ms(d time.Duration) int64 { return max(0, d.Milliseconds()) }

func secs(s string) int64 {
	n, _ := strconv.ParseFloat(s, 64)
	return int64(n * 1000)
}

type builder struct {
	j       *Journey
	names   Names
	cur     *Step // the step that rings phones: extension, ring group, queue, outbound route
	talk    *Step
	primary string // unique id of the first channel
	inbound bool
	hangup  map[string]string
	ended   time.Time
	last    time.Time
}

func (b *builder) add(s *Step) *Step {
	// The previous step ends where this one starts (talk ends when the call or transfer does).
	if n := len(b.j.Steps); n > 0 {
		p := b.j.Steps[n-1]
		if !p.closed {
			p.end, p.closed = s.At, true
		}
	}
	s.ID = len(b.j.Steps) + 1
	if s.Tone == "" {
		s.Tone = "info"
	}
	b.j.Steps = append(b.j.Steps, s)
	return s
}

func (b *builder) lastStep(kind string) *Step {
	for i := len(b.j.Steps) - 1; i >= 0; i-- {
		if b.j.Steps[i].Kind == kind {
			return b.j.Steps[i]
		}
	}
	return nil
}

func (b *builder) startTalk(at time.Time, p Party, via string) {
	if b.talk != nil && !b.talk.closed && b.talk.Party != nil && *b.talk.Party == p {
		return
	}
	b.closeTalk(at)
	d := map[string]string{}
	if via != "" {
		d["via"] = via
	}
	b.talk = b.add(&Step{Kind: "talk", At: at, Party: &p, Name: p.Name, Number: p.Number, Tone: "ok", Detail: d})
}

func (b *builder) closeTalk(at time.Time) {
	if b.talk != nil && !b.talk.closed {
		b.talk.end, b.talk.closed = at, true
	}
}

func (b *builder) ring(at time.Time, id string, p Party) {
	if b.cur == nil {
		b.cur = b.add(&Step{Kind: "extension", At: at, Number: p.Number, Name: p.Name, Ref: p.Number})
	}
	for _, s := range b.cur.Subs {
		if s.id == id && id != "" {
			return
		}
	}
	b.cur.Subs = append(b.cur.Subs, &Sub{Kind: "ring", At: at, Party: &p, id: id, open: true})
}

func (b *builder) ringEnd(at time.Time, id string, p Party, result string) *Sub {
	if b.cur == nil {
		return nil
	}
	for i := len(b.cur.Subs) - 1; i >= 0; i-- {
		s := b.cur.Subs[i]
		if s.Kind == "ring" && s.open && ((id != "" && s.id == id) || (id == "" && s.Party != nil && *s.Party == p)) {
			s.open, s.Result, s.DurationMs = false, result, ms(at.Sub(s.At))
			return s
		}
	}
	return nil
}

var dialResult = map[string]string{"ANSWER": "answered", "NOANSWER": "noanswer", "BUSY": "busy", "CANCEL": "cancel",
	"CHANUNAVAIL": "unavailable", "CONGESTION": "failed", "DONTCALL": "failed", "TORTURE": "failed", "INVALIDARGS": "failed"}

// Build turns the events of one call into its journey.
func Build(linkedID string, evs []Event, names Names) *Journey {
	if names == nil {
		names = func(string, string) string { return "" }
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At.Before(evs[j].At) })
	j := &Journey{LinkedID: linkedID, Steps: []*Step{}, Recordings: []Recording{}}
	b := &builder{j: j, names: names}
	for _, e := range evs {
		b.event(e)
		b.last = e.At
	}
	b.finish()
	return j
}

func (b *builder) event(e Event) {
	d := e.Data
	switch e.Kind {
	case "CHAN_START":
		if b.primary != "" {
			return
		}
		b.primary = e.UniqueID
		b.j.Start = e.At
		p := partyOf(e.Channel, b.names, true)
		switch {
		case p.Kind == "caller":
			b.inbound = true
			b.j.Direction = "inbound"
			b.j.From = Party{Kind: "caller", Number: d["num"], Name: d["name"]}
			b.j.To = d["exten"]
			b.add(&Step{Kind: "inbound", At: e.At, Number: d["num"], Name: d["name"], Tone: "accent",
				Detail: map[string]string{"trunk": p.Name, "did": d["exten"]}})
		default:
			b.j.Direction = "internal"
			if p.Kind != "ext" {
				p = Party{Kind: "ext", Number: d["num"], Name: d["name"]}
			}
			if p.Name == "" {
				p.Name = d["name"]
			}
			b.j.From = p
			b.j.To = d["exten"]
			b.add(&Step{Kind: "internal", At: e.At, Number: p.Number, Name: p.Name, Party: &p, Tone: "accent",
				Detail: map[string]string{"dialled": d["exten"]}})
		}
	case "kt_in":
		if s := b.lastStep("inbound"); s != nil {
			s.Ref = d["route"]
			s.Detail["route"] = b.names("route", d["route"])
			if d["did"] != "" {
				s.Detail["did"], b.j.To = d["did"], d["did"]
			}
		}
	case "kt_noroute":
		b.add(&Step{Kind: "noroute", At: e.At, Number: d["did"], Tone: "danger"})
	case "kt_tc":
		tone := "ok"
		if d["state"] != "open" {
			tone = "warn"
		}
		b.add(&Step{Kind: "timecondition", At: e.At, Ref: d["id"], Name: b.names("tc", d["id"]), Result: d["state"], Tone: tone})
	case "kt_ivr":
		b.cur = nil
		b.add(&Step{Kind: "ivr", At: e.At, Ref: d["id"], Name: b.names("ivr", d["id"]), Tone: "accent"})
	case "kt_ivrkey", "kt_ivrtimeout", "kt_ivrinvalid", "kt_ivrrepeat", "kt_ivrspeech":
		s := b.lastStep("ivr")
		if s == nil || s.Ref != d["id"] {
			s = b.add(&Step{Kind: "ivr", At: e.At, Ref: d["id"], Name: b.names("ivr", d["id"]), Tone: "accent"})
		}
		kind := strings.TrimPrefix(e.Kind, "kt_ivr")
		val := d["key"]
		if kind == "timeout" || kind == "invalid" {
			val = d["try"]
		}
		if kind == "speech" {
			val = d["intent"]
		}
		s.Subs = append(s.Subs, &Sub{Kind: kind, At: e.At, Value: val, DurationMs: ms(e.At.Sub(s.At))})
		if kind == "key" {
			s.Result = val
		} else if s.Result == "" || kind != "repeat" {
			s.Result = kind
		}
	case "kt_queue":
		b.cur = b.add(&Step{Kind: "queue", At: e.At, Ref: d["id"], Name: b.names("queue", d["id"]), Tone: "accent", Detail: map[string]string{}})
	case "Q_JOIN":
		if s := b.inQueue(); s != nil {
			s.Detail["position"] = d["position"]
		}
	case "Q_CALL":
		if s := b.inQueue(); s != nil {
			p := partyOf(d["interface"], b.names, b.inbound)
			if p.Kind != "ext" {
				p = Party{Kind: "ext", Name: d["memberName"]}
			}
			s.Subs = append(s.Subs, &Sub{Kind: "ring", At: e.At, Party: &p, id: d["destUniqueid"], open: true})
		}
	case "Q_NOANSWER":
		if s := b.inQueue(); s != nil {
			b.queueRingEnd(s, e, "noanswer")
		}
	case "Q_CONNECT":
		if s := b.inQueue(); s != nil {
			sub := b.queueRingEnd(s, e, "answered")
			s.Result, s.Tone = "answered", "ok"
			s.Detail["holdMs"] = strconv.FormatInt(secs(d["holdTime"]), 10)
			p := Party{Kind: "ext", Name: d["memberName"]}
			if sub != nil && sub.Party != nil {
				p = *sub.Party
			}
			b.startTalk(e.At, p, "queue")
			b.cur = nil
		}
	case "Q_DONE":
		if b.talk != nil && d["reason"] != "" {
			b.talk.Detail["ended"] = d["reason"]
		}
	case "Q_ABANDON":
		if s := b.lastStep("queue"); s != nil {
			s.Result, s.Tone = "abandoned", "danger"
			s.Detail["holdMs"] = strconv.FormatInt(secs(d["holdTime"]), 10)
			for _, sub := range s.Subs {
				if sub.open {
					sub.open, sub.Result, sub.DurationMs = false, "cancel", ms(e.At.Sub(sub.At))
				}
			}
		}
	case "kt_queueend":
		if s := b.lastStep("queue"); s != nil && s.Result == "" {
			s.Result, s.Tone = strings.ToLower(d["status"]), "warn"
		}
		b.cur = nil
	case "kt_callback":
		// The caller pressed 1 to be called back (the callback itself is a new call).
		if s := b.lastStep("queue"); s != nil {
			s.Result, s.Tone = "callback", "accent"
			if s.Detail == nil {
				s.Detail = map[string]string{}
			}
			s.Detail["callbackNumber"] = d["num"]
		}
		b.cur = nil
	case "kt_rg":
		b.cur = b.add(&Step{Kind: "ringgroup", At: e.At, Ref: d["id"], Name: b.names("rg", d["id"]), Tone: "accent"})
	case "kt_rgend":
		if s := b.lastStep("ringgroup"); s != nil && s.Result == "" {
			s.Result, s.Tone = "noanswer", "warn"
		}
		b.cur = nil
	case "kt_ext":
		// Ring groups and queues dial the phones directly, not through the extension's own handling.
		b.cur = b.add(&Step{Kind: "extension", At: e.At, Ref: d["num"], Number: d["num"], Name: b.names("ext", d["num"]), Tone: "accent"})
	case "kt_extend":
		if s := b.lastStep("extension"); s != nil && s.Number == d["num"] && s.Result == "" {
			s.Result = dialResult[d["status"]]
			if s.Result == "" {
				s.Result = strings.ToLower(d["status"])
			}
			if s.Result == "answered" {
				s.Tone = "ok"
			} else {
				s.Tone = "warn"
			}
		}
	case "kt_dnd", "kt_closed":
		if s := b.lastStep("extension"); s != nil && s.Number == d["num"] {
			s.Result, s.Tone = strings.TrimPrefix(e.Kind, "kt_"), "warn"
		}
	case "kt_fwd":
		b.cur = nil
		b.add(&Step{Kind: "forward", At: e.At, Number: d["num"], Tone: "accent"})
	case "kt_vm":
		b.cur = nil
		b.add(&Step{Kind: "voicemail", At: e.At, Number: d["box"], Name: b.names("ext", d["box"]), Result: d["reason"], Tone: "warn"})
	case "kt_conf":
		b.cur = nil
		b.add(&Step{Kind: "conference", At: e.At, Ref: d["id"], Name: b.names("conf", d["id"]), Tone: "ok"})
	case "kt_fax":
		b.cur = nil
		b.add(&Step{Kind: "fax", At: e.At, Ref: d["id"], Name: b.names("fax", d["id"]), Tone: "accent"})
	case "kt_external":
		b.add(&Step{Kind: "external", At: e.At, Number: d["num"], Tone: "accent"})
	case "kt_out":
		if b.j.Direction == "internal" {
			b.j.Direction = "outbound"
		}
		b.j.To = d["num"]
		b.cur = b.add(&Step{Kind: "outbound", At: e.At, Ref: d["route"], Name: b.names("outroute", d["route"]), Number: d["num"], Tone: "accent",
			Detail: map[string]string{}})
	case "kt_trunk":
		if s := b.lastStep("outbound"); s != nil {
			s.Detail["trunk"] = b.names("trunk", d["id"])
		}
	case "DIAL":
		if b.cur != nil && b.cur.Kind == "queue" {
			return
		}
		if isLocal(d["dest"]) {
			return
		}
		p := partyOf(d["dest"], b.names, b.inbound)
		if p.Kind == "remote" && b.cur != nil && b.cur.Kind == "outbound" {
			p.Number = b.cur.Number
		}
		b.ring(e.At, d["destId"], p)
	case "DIALEND":
		if (b.cur != nil && b.cur.Kind == "queue") || isLocal(d["dest"]) {
			return
		}
		p := partyOf(d["dest"], b.names, b.inbound)
		res := dialResult[d["status"]]
		sub := b.ringEnd(e.At, d["destId"], p, res)
		if res == "answered" {
			if sub != nil && sub.Party != nil {
				p = *sub.Party
			}
			if b.cur != nil {
				b.cur.Result, b.cur.Tone = "answered", "ok"
			}
			via := ""
			if b.cur != nil {
				via = b.cur.Kind
			}
			b.startTalk(e.At, p, via)
			b.cur = nil
		}
	case "PICKUP":
		p := partyOf(d["pickup_channel"], b.names, b.inbound)
		b.add(&Step{Kind: "pickup", At: e.At, Party: &p, Number: p.Number, Name: p.Name, Tone: "accent"})
		b.startTalk(e.At, p, "pickup")
	case "BLINDTRANSFER":
		by := partyOf(e.Channel, b.names, b.inbound)
		b.closeTalk(e.At)
		b.cur = nil
		b.add(&Step{Kind: "transfer", At: e.At, Party: &by, Number: d["extension"], Name: b.names("ext", d["extension"]), Result: "blind", Tone: "accent"})
	case "ATTENDEDTRANSFER":
		by := partyOf(e.Channel, b.names, b.inbound)
		to := partyOf(firstOf(d["transfer_target_channel_name"], d["channel2_name"]), b.names, b.inbound)
		b.closeTalk(e.At)
		b.cur = nil
		b.add(&Step{Kind: "transfer", At: e.At, Party: &by, Number: to.Number, Name: to.Name, Result: "attended", Tone: "accent"})
		if to.Kind == "ext" {
			b.startTalk(e.At, to, "transfer")
		}
	case "PARK_START":
		by := partyOf(firstOf(d["parker_dial_string"], ""), b.names, b.inbound)
		b.closeTalk(e.At)
		b.add(&Step{Kind: "park", At: e.At, Party: &by, Name: d["parking_lot"], Tone: "warn"})
	case "HANGUP":
		if b.hangup == nil && (e.UniqueID == b.primary || (b.talk != nil && !b.talk.closed && *b.talk.Party == partyOf(e.Channel, b.names, b.inbound))) {
			b.hangup = map[string]string{"source": d["hangupsource"], "cause": d["hangupcause"], "channel": e.Channel}
			b.closeTalk(e.At)
			b.ended = e.At
		}
	case "LINKEDID_END":
		b.j.Complete = true
		b.ended = e.At
	}
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (b *builder) inQueue() *Step {
	if b.cur != nil && b.cur.Kind == "queue" {
		return b.cur
	}
	return nil
}

func (b *builder) queueRingEnd(s *Step, e Event, result string) *Sub {
	id := e.Data["destUniqueid"]
	for i := len(s.Subs) - 1; i >= 0; i-- {
		sub := s.Subs[i]
		if sub.open && (id == "" || sub.id == id || sub.id == "") {
			sub.open, sub.Result = false, result
			if rt := e.Data["ringTime"]; rt != "" {
				sub.DurationMs = secs(rt)
			} else {
				sub.DurationMs = ms(e.At.Sub(sub.At))
			}
			return sub
		}
	}
	return nil
}

func (b *builder) finish() {
	j := b.j
	end := b.ended
	if end.IsZero() {
		end = b.last
	}
	if b.hangup != nil {
		by := partyOf(b.hangup["source"], b.names, b.inbound)
		if b.hangup["source"] == "" {
			by = Party{Kind: "system"}
		}
		det := map[string]string{"cause": b.hangup["cause"]}
		b.add(&Step{Kind: "hangup", At: end, Party: &by, Number: by.Number, Name: by.Name, Tone: "info", Detail: det})
	} else if j.Complete {
		b.add(&Step{Kind: "hangup", At: end, Tone: "info", Party: &Party{Kind: "system"}})
	}
	j.End = end
	if !j.Start.IsZero() {
		j.DurationMs = ms(end.Sub(j.Start))
	}
	for i, s := range j.Steps {
		if !s.closed || s.end.IsZero() {
			if i+1 < len(j.Steps) {
				s.end = j.Steps[i+1].At
			} else {
				s.end = end
			}
		}
		s.DurationMs = ms(s.end.Sub(s.At))
		for _, sub := range s.Subs {
			if sub.open {
				sub.open, sub.DurationMs = false, ms(s.end.Sub(sub.At))
				if sub.Result == "" {
					sub.Result = "cancel"
				}
			}
		}
	}
	answered, firstTalk := false, time.Time{}
	for _, s := range j.Steps {
		if s.Kind == "talk" {
			j.TalkMs += s.DurationMs
			if !answered {
				firstTalk = s.At
			}
			answered = true
		}
	}
	switch {
	case answered:
		j.Result = "answered"
		j.WaitMs = ms(firstTalk.Sub(j.Start))
	case b.has("voicemail"):
		j.Result = "voicemail"
	case b.lastResult("queue") == "abandoned":
		j.Result = "abandoned"
	case b.lastResult("extension") == "busy":
		j.Result = "busy"
	case b.has("noroute"):
		j.Result = "failed"
	case b.has("conference"), b.has("fax"):
		j.Result = "answered"
	default:
		j.Result = "missed"
	}
	if j.Direction == "" {
		j.Direction = "internal"
	}
}

func (b *builder) has(kind string) bool { return b.lastStep(kind) != nil }

func (b *builder) lastResult(kind string) string {
	if s := b.lastStep(kind); s != nil {
		return s.Result
	}
	return ""
}

// UniqueIDs returns the channels of the call, to find its recordings.
func UniqueIDs(evs []Event) map[string]bool {
	out := map[string]bool{}
	for _, e := range evs {
		if e.UniqueID != "" {
			out[e.UniqueID] = true
		}
		if id := e.Data["destId"]; id != "" {
			out[id] = true
		}
	}
	return out
}
