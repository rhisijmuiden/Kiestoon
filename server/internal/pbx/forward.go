package pbx

import (
	"regexp"
	"strings"
)

// A forward key (and unconditional forwarding set by one) can send calls to any destination,
// not only to a phone number. The key's value is a phone number for an external number, or
// "<type>:<target>" for a destination inside Kiestoon (for example "ivr:3" or "oncall:2").
//
// Phones dial digits and stars only, and Features.ForwardAlways holds digits and stars, so an
// internal destination is written there as its forward code: "**" + a two-digit type code +
// the target, for example "**053" for IVR 3. The dialplan turns that code back into a Goto.

// forwardTypeCodes are the destination types a forward key can use, with their code.
var forwardTypeCodes = map[string]string{
	DestExtension: "01", DestVoicemail: "02", DestRingGroup: "03", DestQueue: "04", DestIVR: "05",
	DestTimeCondition: "06", DestConference: "07", DestOnCall: "08", DestFollowSun: "09", DestFax: "10",
}

// ForwardKeyTypes are the destination types a forward key offers, in the order of the UI.
var ForwardKeyTypes = []string{DestExternal, DestExtension, DestVoicemail, DestRingGroup, DestQueue, DestIVR,
	DestTimeCondition, DestConference, DestOnCall, DestFollowSun, DestFax}

var reForwardTarget = regexp.MustCompile(`^[0-9]{1,12}$`)

// ParseForwardKey reads the value of a forward key. ok is false when it is not valid.
func ParseForwardKey(v string) (Destination, bool) {
	typ, target, found := strings.Cut(v, ":")
	if !found {
		if reKeyForward.MatchString(v) {
			return Destination{Type: DestExternal, Target: v}, true
		}
		return Destination{}, false
	}
	if typ == DestExternal {
		return Destination{Type: DestExternal, Target: target}, reKeyForward.MatchString(target)
	}
	if _, ok := forwardTypeCodes[typ]; !ok || !reForwardTarget.MatchString(target) {
		return Destination{}, false
	}
	return Destination{Type: typ, Target: target}, true
}

// ForwardCode is what a phone dials and Features.ForwardAlways stores for a destination: the
// number itself for an external number, "**<type code><target>" for the rest.
func ForwardCode(d Destination) string {
	if d.Type == DestExternal {
		return d.Target
	}
	return "**" + forwardTypeCodes[d.Type] + d.Target
}

// ParseForwardCode turns a forward code back into a destination; ok is false for a plain number.
func ParseForwardCode(code string) (Destination, bool) {
	if !strings.HasPrefix(code, "**") || len(code) < 5 {
		return Destination{}, false
	}
	for typ, c := range forwardTypeCodes {
		if code[2:4] == c && reForwardTarget.MatchString(code[4:]) {
			return Destination{Type: typ, Target: code[4:]}, true
		}
	}
	return Destination{}, false
}
