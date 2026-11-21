package crm

import (
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
)

// Summary is one finished call with an outside number, made from the CDRs of all its legs.
type Summary struct {
	LinkedID  string
	Direction string // inbound, outbound
	Number    string // the customer's number
	Line      string // the number they called (inbound) or the extension (outbound)
	Ext       string // the extension that answered (inbound) or called (outbound); "" when missed
	Answered  bool
	Start     time.Time
	TalkSec   int
	UniqueIDs []string
}

// isTrunk tells whether a channel is a trunk ("PJSIP/trunk-3-0000001").
func isTrunk(ch string) bool { return strings.HasPrefix(ch, "PJSIP/trunk-") }

// EndpointExt returns the extension number of a phone channel: PJSIP/201-0000002a, PJSIP/201-web-…
// and PJSIP/201-mobile-… all give 201. Trunks and other channels give "".
func EndpointExt(ch string) string {
	if !strings.HasPrefix(ch, "PJSIP/") || isTrunk(ch) {
		return ""
	}
	c := strings.TrimPrefix(ch, "PJSIP/")
	i := 0
	for i < len(c) && c[i] >= '0' && c[i] <= '9' {
		i++
	}
	if i == 0 || i < len(c) && c[i] != '-' {
		return ""
	}
	return c[:i]
}

// Summarize turns the CDRs of one call into a Summary. ok is false for internal calls.
func Summarize(legs []cdr.Record) (Summary, bool) {
	var s Summary
	if len(legs) == 0 {
		return s, false
	}
	s.LinkedID = legs[0].LinkedID
	for _, l := range legs {
		s.UniqueIDs = append(s.UniqueIDs, l.UniqueID)
		if t, err := time.Parse(time.RFC3339, l.Start); err == nil && (s.Start.IsZero() || t.Before(s.Start)) {
			s.Start = t
		}
	}
	for _, l := range legs {
		if isTrunk(l.SrcChannel) {
			s.Direction, s.Number = "inbound", l.Src
			if Digits(l.Dst) == l.Dst && l.Dst != "" {
				s.Line = l.Dst
			}
			break
		}
	}
	if s.Direction == "" {
		for _, l := range legs {
			if isTrunk(l.DstChannel) {
				s.Direction, s.Number, s.Ext = "outbound", l.Dst, EndpointExt(l.SrcChannel)
				s.Line = s.Ext
				if l.BillSec > 0 {
					s.Answered, s.TalkSec = true, l.BillSec
				}
			}
		}
		return s, s.Direction != "" && s.Number != ""
	}
	// Inbound: the phone that talked longest answered it.
	for _, l := range legs {
		ext := EndpointExt(l.DstChannel)
		if ext == "" || l.BillSec <= 0 || l.Disposition != "ANSWERED" {
			continue
		}
		if l.BillSec > s.TalkSec {
			s.Answered, s.Ext, s.TalkSec = true, ext, l.BillSec
		}
	}
	return s, s.Number != ""
}
