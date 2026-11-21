package crm

import "strings"

// Number is a caller's number prepared for look-ups.
type Number struct {
	// Raw is the number as the PBX saw it (0612345678, +31612345678, 0031612345678).
	Raw string
	// Digits are the digits of Raw.
	Digits string
	// Key is the last 9 digits: the same for every way the number can be written in a country with
	// 9-digit national numbers (the Netherlands, Belgium...), and still specific elsewhere.
	Key string
	// E164 is +<country><national> when it can be worked out, else "".
	E164 string
}

// KeyLen is how many trailing digits must match.
const KeyLen = 9

// Digits keeps only the digits of s.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// KeyOf returns the match key of a number as written anywhere (a CRM field, a caller ID).
func KeyOf(s string) string {
	d := Digits(s)
	if len(d) > KeyLen {
		d = d[len(d)-KeyLen:]
	}
	return d
}

// ParseNumber prepares a caller's number. countryCode is the PBX's country (31). It returns false
// for numbers too short to look up (internal extensions, anonymous).
func ParseNumber(raw, countryCode string) (Number, bool) {
	raw = strings.TrimSpace(raw)
	d := Digits(raw)
	if len(d) < 7 {
		return Number{}, false
	}
	n := Number{Raw: raw, Digits: d, Key: KeyOf(d)}
	switch {
	case strings.HasPrefix(raw, "+"):
		n.E164 = "+" + d
	case strings.HasPrefix(d, "00"):
		n.E164 = "+" + d[2:]
	case strings.HasPrefix(d, "0") && countryCode != "":
		n.E164 = "+" + countryCode + d[1:]
	case countryCode != "" && strings.HasPrefix(d, countryCode) && len(d) > 10:
		n.E164 = "+" + d
	}
	return n, true
}

// Matches tells whether a number stored in a CRM is this number.
func (n Number) Matches(stored string) bool {
	k := KeyOf(stored)
	return len(k) >= 7 && (k == n.Key || len(n.Digits) < KeyLen && Digits(stored) == n.Digits)
}
