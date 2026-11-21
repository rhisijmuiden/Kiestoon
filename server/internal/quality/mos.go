// Package quality measures the audio quality of calls from the RTCP reports of Asterisk: jitter,
// packet loss and round trip per call leg, in both directions, turned into a MOS with the ITU
// E-model. It keeps the legs that are going on in memory and stores each leg when it ends.
package quality

import (
	"math"
	"strings"
)

// ClockRate is the RTP clock rate of a codec as Asterisk names it. RTCP reports jitter in these
// timestamp units. G.722 uses an 8 kHz RTP clock although it samples at 16 kHz (RFC 3551).
func ClockRate(codec string) float64 {
	switch strings.ToLower(codec) {
	case "opus":
		return 48000
	case "slin16", "g7221", "siren7", "speex16":
		return 16000
	case "slin32", "siren14", "speex32":
		return 32000
	case "slin48":
		return 48000
	}
	return 8000
}

// impairment returns the equipment impairment Ie of a codec and its packet loss robustness Bpl
// (ITU-T G.113 appendix I). G.711 and G.722 are counted with packet loss concealment, which every
// current phone has.
func impairment(codec string) (ie, bpl float64) {
	switch strings.ToLower(codec) {
	case "g729":
		return 11, 19
	case "ilbc":
		return 10, 32
	case "gsm":
		return 20, 43
	case "g726", "g726aal2":
		return 7, 25.1
	case "opus":
		return 0, 30
	}
	return 0, 25.1
}

// MOS estimates the mean opinion score (1 to 4.5) of one direction of a call from its average
// jitter (ms), packet loss (percent) and round trip (ms), with the simplified ITU-T G.107 E-model:
// jitter counts double as delay because the jitter buffer holds back that much audio, and packet
// loss weighs in through the effective equipment impairment Ie-eff = Ie + (95 - Ie) * Ppl / (Ppl + Bpl).
func MOS(jitterMs, lossPct, rttMs float64, codec string) float64 {
	delay := rttMs/2 + jitterMs*2 + 10
	r := 93.2
	if delay < 160 {
		r -= delay / 40
	} else {
		r -= (delay - 120) / 10
	}
	ie, bpl := impairment(codec)
	lossPct = math.Max(0, lossPct)
	r -= ie + (95-ie)*lossPct/(lossPct+bpl)
	var mos float64
	switch {
	case r <= 0:
		mos = 1
	case r >= 100:
		mos = 4.5
	default:
		mos = 1 + 0.035*r + 0.000007*r*(r-60)*(100-r)
	}
	return round2(math.Max(1, math.Min(4.5, mos)))
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round1(v float64) float64 { return math.Round(v*10) / 10 }
