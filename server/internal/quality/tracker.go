package quality

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kinds of call legs.
const (
	KindTrunk     = "trunk"
	KindExtension = "extension"
	KindOther     = "other"
)

// Direction is the quality of the audio in one direction of a leg.
type Direction struct {
	// MOS is 0 when no report came in for this direction.
	MOS       float64 `json:"mos"`
	Jitter    float64 `json:"jitter"`    // average, ms
	JitterMax float64 `json:"jitterMax"` // ms
	Loss      float64 `json:"loss"`      // percent
	Reports   int     `json:"reports"`
}

// Leg is the quality of one channel of a call: the part between Kiestoon and a phone or a provider.
type Leg struct {
	ID       int64  `json:"id,omitempty"`
	LinkedID string `json:"linkedId"`
	UniqueID string `json:"uniqueId"`
	Channel  string `json:"channel"`
	Endpoint string `json:"endpoint"`
	Kind     string `json:"kind"`
	TrunkID  int64  `json:"trunkId,omitempty"`
	Ext      string `json:"ext,omitempty"`
	Codec    string `json:"codec"`
	// Remote is the media address of the other side.
	Remote string    `json:"remote"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	// RX is what Kiestoon receives from the other side, TX what the other side reports it receives.
	RX     Direction `json:"rx"`
	TX     Direction `json:"tx"`
	RTT    float64   `json:"rtt"`    // average round trip, ms
	RTTMax float64   `json:"rttMax"` // ms
	// MOS is the worst of both directions, MOSMin the worst report interval.
	MOS    float64 `json:"mos"`
	MOSMin float64 `json:"mosMin"`
	// Samples is the MOS of each report interval (about 5 seconds) in hundredths, oldest first.
	Samples []int `json:"samples"`
	// Current is the MOS of the last few intervals; only for legs that are going on.
	Current float64 `json:"current,omitempty"`
}

// Poor reports whether the leg is under the poor limit.
func (l Leg) Poor(limit float64) bool { return l.MOS > 0 && l.MOS < limit }

var extEndpoint = regexp.MustCompile(`^([0-9]+)(-web|-app)?$`)

// Classify returns the kind, trunk id and extension number of an endpoint.
func Classify(endpoint string) (kind string, trunkID int64, ext string) {
	if rest, ok := strings.CutPrefix(endpoint, "trunk-"); ok {
		if id, err := strconv.ParseInt(rest, 10, 64); err == nil {
			return KindTrunk, id, ""
		}
	}
	if m := extEndpoint.FindStringSubmatch(endpoint); m != nil {
		return KindExtension, 0, m[1]
	}
	return KindOther, 0, ""
}

// EndpointOf returns the endpoint of a PJSIP channel name: PJSIP/trunk-2-0000001a -> trunk-2.
func EndpointOf(channel string) string {
	s, ok := strings.CutPrefix(channel, "PJSIP/")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(s, "-"); i > 0 {
		s = s[:i]
	}
	return s
}

// report is one RTCP report block: what one side received during the last interval.
type report struct {
	tx     bool    // the other side's report about what Kiestoon sent
	jitter float64 // RTP timestamp units
	frac   float64 // fraction lost, 0..255
	rtt    float64 // ms, 0 when unknown
}

type live struct {
	leg     Leg
	reports []report
	last    time.Time
}

// maxReports keeps a leg of many hours from growing without end: older intervals are merged.
const maxReports = 1440

// Tracker follows the legs that are going on.
type Tracker struct {
	mu   sync.Mutex
	legs map[string]*live
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker { return &Tracker{legs: map[string]*live{}} }

// Event takes an AMI event. It returns the leg when it ended (Hangup) and whether this was the
// first report of a new leg (the caller then looks up its codec).
func (t *Tracker) Event(get func(string) string, now time.Time) (done *Leg, isNew bool) {
	ev := get("Event")
	uid := get("Uniqueid")
	if uid == "" {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if ev == "Hangup" {
		l := t.legs[uid]
		if l == nil {
			return nil, false
		}
		delete(t.legs, uid)
		leg := l.finish(now, false)
		return &leg, false
	}
	if ev != "RTCPSent" && ev != "RTCPReceived" {
		return nil, false
	}
	ch := get("Channel")
	ep := EndpointOf(ch)
	if ep == "" {
		return nil, false
	}
	l := t.legs[uid]
	if l == nil {
		kind, trunk, ext := Classify(ep)
		l = &live{leg: Leg{LinkedID: get("Linkedid"), UniqueID: uid, Channel: ch, Endpoint: ep, Kind: kind, TrunkID: trunk, Ext: ext, Start: now}}
		t.legs[uid] = l
		isNew = true
	}
	l.last = now
	if n, _ := strconv.Atoi(get("ReportCount")); n < 1 {
		return nil, isNew
	}
	r := report{tx: ev == "RTCPReceived"}
	r.jitter, _ = strconv.ParseFloat(get("Report0IAJitter"), 64)
	r.frac, _ = strconv.ParseFloat(get("Report0FractionLost"), 64)
	if r.tx {
		if rtt, err := strconv.ParseFloat(get("RTT"), 64); err == nil && rtt > 0 && rtt < 30 {
			r.rtt = rtt * 1000
		}
		if from := get("From"); from != "" {
			l.leg.Remote = hostOf(from)
		}
	} else if to := get("To"); to != "" && l.leg.Remote == "" {
		l.leg.Remote = hostOf(to)
	}
	l.reports = append(l.reports, r)
	if len(l.reports) > maxReports {
		l.reports = halve(l.reports)
	}
	return nil, isNew
}

// halve merges pairs of reports of the same direction, keeping the worst of each pair.
func halve(rs []report) []report {
	out := make([]report, 0, len(rs)/2+2)
	var pending [2]*report
	for _, r := range rs {
		i := 0
		if r.tx {
			i = 1
		}
		if p := pending[i]; p != nil {
			m := *p
			m.jitter, m.frac, m.rtt = max(m.jitter, r.jitter), max(m.frac, r.frac), max(m.rtt, r.rtt)
			out = append(out, m)
			pending[i] = nil
			continue
		}
		c := r
		pending[i] = &c
	}
	for _, p := range pending {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}

func hostOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i > 0 && !strings.HasSuffix(addr, "]") {
		addr = addr[:i]
	}
	return strings.Trim(addr, "[]")
}

// SetCodec sets the codec of a leg (Asterisk does not put it in the RTCP events).
func (t *Tracker) SetCodec(uid, codec string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if l := t.legs[uid]; l != nil {
		l.leg.Codec = strings.ToLower(codec)
	}
}

// Active returns the legs that are going on, with their figures so far.
func (t *Tracker) Active(now time.Time) []Leg {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Leg, 0, len(t.legs))
	for _, l := range t.legs {
		if len(l.reports) == 0 {
			continue
		}
		out = append(out, l.finish(now, true))
	}
	return out
}

// Stale removes and returns the legs without a report for longer than idle: their hangup was
// missed, for example while the manager connection was down.
func (t *Tracker) Stale(now time.Time, idle time.Duration) []Leg {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Leg
	for uid, l := range t.legs {
		if now.Sub(l.last) > idle {
			delete(t.legs, uid)
			out = append(out, l.finish(l.last, false))
		}
	}
	return out
}

// finish computes the figures of a leg from its reports.
func (l *live) finish(end time.Time, ongoing bool) Leg {
	leg := l.leg
	leg.End = end
	leg.Samples = []int{}
	rate := ClockRate(leg.Codec)
	toMs := func(units float64) float64 { return units / rate * 1000 }
	var rx, tx acc
	var rttSum float64
	var rttN int
	for _, r := range l.reports {
		if r.tx {
			tx.add(r, toMs)
		} else {
			rx.add(r, toMs)
		}
		if r.rtt > 0 {
			rttSum += r.rtt
			rttN++
			leg.RTTMax = max(leg.RTTMax, r.rtt)
		}
	}
	if rttN > 0 {
		leg.RTT = round1(rttSum / float64(rttN))
	}
	leg.RTTMax = round1(leg.RTTMax)
	leg.RX, leg.TX = rx.direction(leg.RTT, leg.Codec), tx.direction(leg.RTT, leg.Codec)
	leg.MOS = worst(leg.RX.MOS, leg.TX.MOS)

	// The timeline: at each report of Kiestoon (or of the other side when Kiestoon sends none),
	// the worst of the latest interval of both directions.
	var lastRX, lastTX *report
	var rtt float64
	emitTX := rx.n == 0
	intervalMOS := func(r *report) float64 {
		if r == nil {
			return 0
		}
		return MOS(toMs(r.jitter), r.frac/256*100, rtt, leg.Codec)
	}
	for i := range l.reports {
		r := &l.reports[i]
		if r.rtt > 0 {
			rtt = r.rtt
		}
		if r.tx {
			lastTX = r
		} else {
			lastRX = r
		}
		if r.tx != emitTX {
			continue
		}
		m := worst(intervalMOS(lastRX), intervalMOS(lastTX))
		leg.Samples = append(leg.Samples, int(m*100+0.5))
		if leg.MOSMin == 0 || m < leg.MOSMin {
			leg.MOSMin = m
		}
	}
	if ongoing {
		n := len(leg.Samples)
		from := max(0, n-3)
		for _, s := range leg.Samples[from:] {
			v := float64(s) / 100
			if leg.Current == 0 || v < leg.Current {
				leg.Current = v
			}
		}
	}
	return leg
}

type acc struct {
	n                    int
	jitSum, jitMax, frac float64
}

func (a *acc) add(r report, toMs func(float64) float64) {
	a.n++
	j := toMs(r.jitter)
	a.jitSum += j
	a.jitMax = max(a.jitMax, j)
	a.frac += r.frac
}

func (a acc) direction(rtt float64, codec string) Direction {
	if a.n == 0 {
		return Direction{}
	}
	d := Direction{Reports: a.n, Jitter: round1(a.jitSum / float64(a.n)), JitterMax: round1(a.jitMax), Loss: round2(a.frac / float64(a.n) / 256 * 100)}
	d.MOS = MOS(d.Jitter, d.Loss, rtt, codec)
	return d
}

// worst returns the lowest MOS that is known (0 = unknown).
func worst(a, b float64) float64 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return min(a, b)
}
