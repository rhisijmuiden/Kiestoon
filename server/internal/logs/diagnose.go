package logs

import (
	"regexp"
	"strings"
)

// Diagnosis explains, from the Asterisk log, why a trunk is not registered or not reachable.
type Diagnosis struct {
	// Reason is a key the UI translates: auth_failed, forbidden, not_found, timeout, dns,
	// unavailable, transport, tls, other, or "" when the log has nothing about the trunk.
	Reason string `json:"reason"`
	// Code is the SIP response code when the provider answered with one.
	Code  string  `json:"code,omitempty"`
	Lines []Entry `json:"lines"`
}

// TrunkMatch says which log lines are about one trunk.
type TrunkMatch struct {
	Endpoint string // trunk-3
	Host     string // sip.provider.nl
	Username string
}

func (m TrunkMatch) matches(e Entry) bool {
	t := strings.ToLower(e.Text)
	if m.Endpoint != "" && containsWord(t, strings.ToLower(m.Endpoint)) {
		return true
	}
	if m.Host == "" {
		return false
	}
	h := strings.ToLower(m.Host)
	if !strings.Contains(t, h) {
		return false
	}
	// A line about the host is about this trunk when it comes from the SIP stack, not from a call
	// to some other number on the same provider.
	mod := strings.ToLower(e.Module)
	return strings.Contains(mod, "pjsip") || strings.Contains(mod, "resolver") || strings.Contains(mod, "pjproject") ||
		strings.Contains(t, "registration") || strings.Contains(t, "resolve") || strings.Contains(t, "qualify")
}

// containsWord reports whether s contains w not followed by a digit, so trunk-1 does not match trunk-12.
func containsWord(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		end := i + j + len(w)
		if end >= len(s) || s[end] < '0' || s[end] > '9' {
			return true
		}
		i = end
	}
}

var reCode = regexp.MustCompile(`(?i)(?:response|status)\s*'?(\d{3})'?|'(\d{3})'\s*received|\b(\d{3}) (?:Unauthorized|Forbidden|Not Found|Request Timeout|Service Unavailable|Proxy Authentication)`)

// classify gives the reason of one line, or "".
func classify(e Entry) (reason, code string) {
	t := strings.ToLower(e.Text)
	if m := reCode.FindStringSubmatch(e.Text); m != nil {
		for _, c := range m[1:] {
			if c != "" {
				code = c
				break
			}
		}
	}
	switch {
	case code == "401" || code == "407":
		return "auth_failed", code
	case code == "403":
		return "forbidden", code
	case code == "404":
		return "not_found", code
	case code == "408":
		return "timeout", code
	case strings.HasPrefix(code, "5"):
		return "unavailable", code
	case strings.Contains(t, "no response received") || strings.Contains(t, "timed out") || strings.Contains(t, "timeout"):
		return "timeout", code
	case strings.Contains(t, "resolve") || strings.Contains(t, "dns") || strings.Contains(t, "srv") && strings.Contains(t, "fail"):
		return "dns", code
	case strings.Contains(t, "tls") || strings.Contains(t, "certificate") || strings.Contains(t, "ssl"):
		return "tls", code
	case strings.Contains(t, "auth") && (strings.Contains(t, "fail") || strings.Contains(t, "unable")):
		return "auth_failed", code
	case strings.Contains(t, "transport") || strings.Contains(t, "unable to send") || strings.Contains(t, "connection refused") ||
		strings.Contains(t, "network is unreachable") || strings.Contains(t, "no route"):
		return "transport", code
	case code != "" && code[0] >= '4':
		return "other", code
	}
	return "", code
}

// Diagnose looks at the newest log lines about the trunk (entries newest first) and returns the
// reason of the newest line that explains a failure, with up to 15 lines about the trunk.
func Diagnose(entries []Entry, m TrunkMatch) Diagnosis {
	d := Diagnosis{Lines: []Entry{}}
	for _, e := range entries {
		if !m.matches(e) {
			continue
		}
		if len(d.Lines) < 15 {
			d.Lines = append(d.Lines, e)
		}
		if d.Reason == "" && (e.Level == "error" || e.Level == "warning" || e.Level == "notice") {
			if r, c := classify(e); r != "" {
				d.Reason, d.Code = r, c
			}
		}
		if d.Reason != "" && len(d.Lines) >= 15 {
			break
		}
	}
	if d.Reason == "" && len(d.Lines) > 0 {
		d.Reason = "other"
	}
	return d
}
