package journey

import (
	"sort"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
)

// FromCDR builds a simple journey from the call detail records alone, for calls from before
// journeys were recorded or after their events were removed: who called, which phones rang and
// who answered.
func FromCDR(linkedID string, recs []cdr.Record, names Names) *Journey {
	if names == nil {
		names = func(string, string) string { return "" }
	}
	sort.SliceStable(recs, func(i, k int) bool { return recs[i].Start < recs[k].Start })
	j := &Journey{LinkedID: linkedID, Partial: true, Complete: true, Steps: []*Step{}, Recordings: []Recording{}}
	if len(recs) == 0 {
		return j
	}
	b := &builder{j: j, names: names}
	first := recs[0]
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }
	j.Start = at(first.Start)
	j.Direction = first.Direction
	j.To = first.Dst
	b.inbound = first.Direction == "inbound"
	if b.inbound {
		j.From = Party{Kind: "caller", Number: first.Src}
		b.add(&Step{Kind: "inbound", At: j.Start, Number: first.Src, Tone: "accent", Detail: map[string]string{"did": first.Dst}})
	} else {
		p := partyOf(first.SrcChannel, names, false)
		if p.Number == "" {
			p = Party{Kind: "ext", Number: first.Src, Name: names("ext", first.Src)}
		}
		j.From = p
		b.add(&Step{Kind: "internal", At: j.Start, Number: p.Number, Name: p.Name, Party: &p, Tone: "accent", Detail: map[string]string{"dialled": first.Dst}})
	}
	var end time.Time
	for _, r := range recs {
		if e := at(r.End); e.After(end) {
			end = e
		}
		if r.DstChannel == "" || isLocal(r.DstChannel) {
			continue
		}
		p := partyOf(r.DstChannel, names, b.inbound)
		start := at(r.Start)
		if b.cur == nil || (r.Disposition == "ANSWERED" && b.cur.Result == "answered") {
			kind := "extension"
			if p.Kind != "ext" {
				kind = "outbound"
			}
			b.cur = b.add(&Step{Kind: kind, At: start, Number: firstOf(p.Number, r.Dst), Name: p.Name, Tone: "accent", Detail: map[string]string{"trunk": r.Trunk}})
		}
		sub := &Sub{Kind: "ring", At: start, Party: &p}
		b.cur.Subs = append(b.cur.Subs, sub)
		if r.Disposition == "ANSWERED" && r.Answer != "" {
			sub.Result = "answered"
			sub.DurationMs = ms(at(r.Answer).Sub(start))
			b.cur.Result, b.cur.Tone = "answered", "ok"
			b.startTalk(at(r.Answer), p, "")
			b.talk.end, b.talk.closed = at(r.Answer).Add(time.Duration(r.BillSec)*time.Second), true
		} else {
			sub.Result = map[string]string{"NO ANSWER": "noanswer", "BUSY": "busy"}[r.Disposition]
			if sub.Result == "" {
				sub.Result = "failed"
			}
			sub.DurationMs = int64(r.Duration) * 1000
		}
	}
	b.ended = end
	b.last = end
	b.hangup = map[string]string{}
	b.finish()
	// The records do not say who hung up.
	if n := len(j.Steps); n > 0 && j.Steps[n-1].Kind == "hangup" {
		j.Steps[n-1].Party = &Party{Kind: "system"}
	}
	return j
}
