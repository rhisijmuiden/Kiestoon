package quality

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Causes of poor quality, from the side and direction where it goes wrong. "To" is the audio
// Kiestoon sends to the phone or provider, "From" the audio it receives from them.
const (
	CauseOK         = "ok"
	CauseLossTo     = "lossTo"
	CauseLossFrom   = "lossFrom"
	CauseJitterTo   = "jitterTo"
	CauseJitterFrom = "jitterFrom"
	CauseRTT        = "rtt"
	CauseMixed      = "mixed"
)

// Figures are the averages of a set of legs.
type Figures struct {
	Legs     int     `json:"legs"`
	Calls    int     `json:"calls"`
	MOS      float64 `json:"mos"`
	Poor     int     `json:"poor"`
	PoorPct  float64 `json:"poorPct"`
	RxJitter float64 `json:"rxJitter"`
	TxJitter float64 `json:"txJitter"`
	RxLoss   float64 `json:"rxLoss"`
	TxLoss   float64 `json:"txLoss"`
	RTT      float64 `json:"rtt"`
	// Jitter and Loss are the worst direction.
	Jitter float64 `json:"jitter"`
	Loss   float64 `json:"loss"`
	Cause  string  `json:"cause"`

	mosSum, rxJ, txJ, rxL, txL, rtt float64
	rxN, txN, rttN                  int
	calls                           map[string]bool
}

func (f *Figures) add(l Leg, poor float64) {
	if l.MOS == 0 {
		return
	}
	if f.calls == nil {
		f.calls = map[string]bool{}
	}
	f.Legs++
	f.calls[l.LinkedID] = true
	f.mosSum += l.MOS
	if l.Poor(poor) {
		f.Poor++
	}
	if l.RX.Reports > 0 {
		f.rxJ += l.RX.Jitter
		f.rxL += l.RX.Loss
		f.rxN++
	}
	if l.TX.Reports > 0 {
		f.txJ += l.TX.Jitter
		f.txL += l.TX.Loss
		f.txN++
	}
	if l.RTT > 0 {
		f.rtt += l.RTT
		f.rttN++
	}
}

func (f *Figures) finish(s Settings) {
	f.Calls = len(f.calls)
	if f.Legs == 0 {
		f.Cause = CauseOK
		return
	}
	f.MOS = round2(f.mosSum / float64(f.Legs))
	f.PoorPct = round1(float64(f.Poor) * 100 / float64(f.Legs))
	if f.rxN > 0 {
		f.RxJitter, f.RxLoss = round1(f.rxJ/float64(f.rxN)), round2(f.rxL/float64(f.rxN))
	}
	if f.txN > 0 {
		f.TxJitter, f.TxLoss = round1(f.txJ/float64(f.txN)), round2(f.txL/float64(f.txN))
	}
	if f.rttN > 0 {
		f.RTT = round1(f.rtt / float64(f.rttN))
	}
	f.Jitter, f.Loss = max(f.RxJitter, f.TxJitter), max(f.RxLoss, f.TxLoss)
	f.Cause = Cause(f.MOS, f.RxJitter, f.TxJitter, f.RxLoss, f.TxLoss, f.RTT, s)
}

// Cause tells where the quality goes wrong. It returns CauseOK when the MOS is not poor and all
// figures are within the limits.
func Cause(mos, rxJitter, txJitter, rxLoss, txLoss, rtt float64, s Settings) string {
	switch {
	case max(rxLoss, txLoss) >= s.Loss:
		if txLoss >= rxLoss {
			return CauseLossTo
		}
		return CauseLossFrom
	case max(rxJitter, txJitter) >= s.Jitter:
		if txJitter >= rxJitter {
			return CauseJitterTo
		}
		return CauseJitterFrom
	case rtt >= s.RTT:
		return CauseRTT
	case mos > 0 && mos < s.Poor:
		return CauseMixed
	}
	return CauseOK
}

// Group are the figures of one trunk or phone.
type Group struct {
	Figures
	Key  string `json:"key"`
	Name string `json:"name"`
	// Remote is the media address of the last leg.
	Remote string `json:"remote"`
	// Buckets is the average MOS per time bucket, 0 where there were no calls.
	Buckets []float64 `json:"buckets"`
}

// Overview are the figures of a period.
type Overview struct {
	From    time.Time `json:"from"`
	Bucket  int       `json:"bucket"` // seconds
	Total   Figures   `json:"total"`
	Trunks  []*Group  `json:"trunks"`
	Phones  []*Group  `json:"phones"`
	Buckets []float64 `json:"buckets"`
}

// Summarize sums up the legs that ended in [from, from+n*bucket) per trunk and per phone.
func Summarize(legs []Leg, from time.Time, bucket time.Duration, n int, s Settings) Overview {
	o := Overview{From: from, Bucket: int(bucket.Seconds()), Trunks: []*Group{}, Phones: []*Group{}}
	trunks, phones := map[string]*Group{}, map[string]*Group{}
	type sum struct {
		v float64
		n int
	}
	all := make([]sum, n)
	per := map[*Group][]sum{}
	group := func(m map[string]*Group, key string) *Group {
		g := m[key]
		if g == nil {
			g = &Group{Key: key}
			m[key] = g
			per[g] = make([]sum, n)
		}
		return g
	}
	for _, l := range legs {
		if l.MOS == 0 {
			continue
		}
		o.Total.add(l, s.Poor)
		i := int(l.End.Sub(from) / bucket)
		if i < 0 || i >= n {
			i = -1
		}
		var g *Group
		switch l.Kind {
		case KindTrunk:
			g = group(trunks, strconv.FormatInt(l.TrunkID, 10))
		case KindExtension:
			g = group(phones, l.Ext)
		}
		if g != nil {
			g.add(l, s.Poor)
			g.Remote = l.Remote
		}
		if i >= 0 {
			all[i].v += l.MOS
			all[i].n++
			if g != nil {
				per[g][i].v += l.MOS
				per[g][i].n++
			}
		}
	}
	// The MOS of a call is its worst leg: count poor calls by call, not by leg.
	worstOf := map[string]float64{}
	for _, l := range legs {
		if l.MOS == 0 {
			continue
		}
		if w, ok := worstOf[l.LinkedID]; !ok || l.MOS < w {
			worstOf[l.LinkedID] = l.MOS
		}
	}
	o.Total.finish(s)
	o.Total.Poor = 0
	for _, w := range worstOf {
		if w < s.Poor {
			o.Total.Poor++
		}
	}
	if o.Total.Calls > 0 {
		o.Total.PoorPct = round1(float64(o.Total.Poor) * 100 / float64(o.Total.Calls))
	}
	avg := func(ss []sum) []float64 {
		out := make([]float64, len(ss))
		for i, x := range ss {
			if x.n > 0 {
				out[i] = round2(x.v / float64(x.n))
			}
		}
		return out
	}
	o.Buckets = avg(all)
	for m, list := range map[*map[string]*Group]*[]*Group{&trunks: &o.Trunks, &phones: &o.Phones} {
		for _, g := range *m {
			g.finish(s)
			g.Buckets = avg(per[g])
			*list = append(*list, g)
		}
	}
	sort.Slice(o.Trunks, func(i, j int) bool { return o.Trunks[i].Legs > o.Trunks[j].Legs })
	// Phones: the weakest first.
	sort.Slice(o.Phones, func(i, j int) bool {
		if o.Phones[i].MOS != o.Phones[j].MOS {
			return o.Phones[i].MOS < o.Phones[j].MOS
		}
		return o.Phones[i].Key < o.Phones[j].Key
	})
	return o
}

// Breaches returns the trunks and phones that are over their alert rule now. legs are the legs
// that ended in the longest rule window plus those going on.
func Breaches(legs []Leg, s Settings, now time.Time) []Alert {
	out := []Alert{}
	if s.Trunk.Enabled {
		since := now.Add(-time.Duration(s.Trunk.Minutes) * time.Minute)
		per := map[int64]*Figures{}
		for _, l := range legs {
			if l.Kind != KindTrunk || l.End.Before(since) || l.MOS == 0 {
				continue
			}
			if per[l.TrunkID] == nil {
				per[l.TrunkID] = &Figures{}
			}
			per[l.TrunkID].add(l, s.Poor)
		}
		for id, f := range per {
			f.finish(s)
			if f.Legs >= s.Trunk.Count && f.MOS < s.Poor {
				ref := strconv.FormatInt(id, 10)
				out = append(out, Alert{Key: "trunk/" + ref, Kind: "trunk", Ref: ref, Calls: f.Legs, Poor: f.Poor, MOS: f.MOS,
					Jitter: f.Jitter, Loss: f.Loss, RTT: f.RTT, Cause: f.Cause})
			}
		}
	}
	if s.Phone.Enabled {
		since := now.Add(-time.Duration(s.Phone.Minutes) * time.Minute)
		per := map[string]*Figures{}
		for _, l := range legs {
			if l.Kind != KindExtension || l.End.Before(since) || l.MOS == 0 {
				continue
			}
			if per[l.Ext] == nil {
				per[l.Ext] = &Figures{}
			}
			per[l.Ext].add(l, s.Poor)
		}
		for ext, f := range per {
			f.finish(s)
			if f.Poor >= s.Phone.Count {
				out = append(out, Alert{Key: "phone/" + ext, Kind: "phone", Ref: ext, Calls: f.Legs, Poor: f.Poor, MOS: f.MOS,
					Jitter: f.Jitter, Loss: f.Loss, RTT: f.RTT, Cause: f.Cause})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Rating names a MOS: good, fair or poor.
func Rating(mos float64, s Settings) string {
	switch {
	case mos >= s.Good:
		return "good"
	case mos >= s.Poor:
		return "fair"
	}
	return "poor"
}

// FormatMOS writes a MOS with two decimals, with a comma in Dutch.
func FormatMOS(v float64, lang string) string {
	return decimal(fmt.Sprintf("%.2f", v), lang)
}

func decimal(s, lang string) string {
	if lang == "nl" {
		b := []byte(s)
		for i, c := range b {
			if c == '.' {
				b[i] = ','
			}
		}
		return string(b)
	}
	return s
}
