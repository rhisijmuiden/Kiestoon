package provision

import (
	"strconv"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Feature codes the key types use (see pbx.FeatureCodes and the dialplan).
const (
	codeDND       = "*76"
	codeForward   = "*74"
	codeVoicemail = "*97"
	codeIntercom  = "*80"
	codePark      = "70"
)

// rkey is a key reduced to what phones understand: a line, a BLF, a speed dial or a park key.
type rkey struct {
	Kind    string // line | blf | speed | park | none
	Value   string
	Label   string
	Account int  // 1-based account the key belongs to
	Pickup  bool // BLF: pressing it while the colleague rings picks up the call
}

// resolveKey turns the Kiestoon key types into the four kinds every vendor knows.
func resolveKey(k pbx.Key) rkey {
	r := rkey{Kind: "none", Label: clean(k.Label), Account: 1}
	switch k.Type {
	case "line":
		r.Kind = "line"
		if n, err := strconv.Atoi(k.Value); err == nil && n > 1 {
			r.Account = n
		}
	case "blf":
		r.Kind, r.Value, r.Pickup = "blf", k.Value, true
	case "parkslot":
		// Watching a parking slot: lit while a call is parked there, a press takes it back.
		r.Kind, r.Value = "blf", k.Value
	case "speed":
		r.Kind, r.Value = "speed", k.Value
	case "intercom":
		r.Kind, r.Value = "speed", codeIntercom+k.Value
	case "dnd":
		r.Kind, r.Value = "speed", codeDND
	case "voicemail":
		r.Kind, r.Value = "speed", codeVoicemail
	case "park":
		r.Kind, r.Value = "park", codePark
	case "forward":
		// Value is "<ext>*<number>" (see forwardKeys): a BLF on *74<ext>*<number>, lit while the
		// extension forwards; a press switches forwarding to <number> on or off.
		if k.Value != "" {
			r.Kind, r.Value = "blf", codeForward+k.Value
		}
	}
	return r
}

// basicKey is resolveKey for vendors without a park key: parking becomes a speed dial to 70.
func basicKey(k pbx.Key) rkey {
	r := resolveKey(k)
	if r.Kind == "park" {
		r.Kind = "speed"
	}
	return r
}

// accounts lists the extensions on a phone: the main one first, then the extra lines.
func (in Input) accounts() []*pbx.Extension {
	out := []*pbx.Extension{}
	if in.Ext != nil {
		out = append(out, in.Ext)
	}
	return append(out, in.Lines...)
}

// portFor is the SIP port for an extension's transport.
func (in Input) portFor(e *pbx.Extension) int {
	if e != nil && e.Transport == "tls" {
		return in.PBX.TLSPort
	}
	return in.PBX.SIPPort
}

// simpleKeys reduces keys to line, blf and speed for the vendors that only know those
// (Fanvil, Poly, Cisco).
func simpleKeys(keys []pbx.Key) []pbx.Key {
	out := make([]pbx.Key, len(keys))
	for i, k := range keys {
		r := basicKey(k)
		out[i] = pbx.Key{Type: r.Kind, Value: r.Value, Label: k.Label}
	}
	return out
}

// forwardKeys gives the forward keys of the phone the code of its own extension: the dialplan
// toggles forwarding of <ext> with *74<ext>*<number>. Without an extension they do nothing.
func forwardKeys(in Input) Input {
	ext := ""
	if in.Ext != nil {
		ext = in.Ext.Number
	}
	fix := func(keys []pbx.Key) []pbx.Key {
		out := make([]pbx.Key, len(keys))
		for i, k := range keys {
			if k.Type == "forward" {
				if ext == "" || k.Value == "" {
					k = pbx.Key{Type: "none"}
				} else {
					if k.Label == "" {
						k.Label = k.Value
					}
					k.Value = ext + "*" + k.Value
				}
			}
			out[i] = k
		}
		return out
	}
	in.Phone.Keys = fix(in.Phone.Keys)
	mods := make([]pbx.Module, len(in.Phone.Modules))
	for i, m := range in.Phone.Modules {
		mods[i] = pbx.Module{Type: m.Type, Keys: fix(m.Keys)}
	}
	in.Phone.Modules = mods
	return in
}
