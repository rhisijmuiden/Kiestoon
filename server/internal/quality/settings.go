package quality

import (
	"context"
	"database/sql"
	"net/mail"
	"slices"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// SettingsKey is the settings row with Settings as JSON.
const SettingsKey = "call_quality"

// Rule says when a trunk or phone gets an alert: Count calls (trunk: at least this many calls with
// an average MOS under the poor limit; phone: at least this many poor calls) in the last Minutes.
type Rule struct {
	Enabled bool `json:"enabled"`
	Minutes int  `json:"minutes"`
	Count   int  `json:"count"`
}

// Settings are the limits and alerts of call quality.
type Settings struct {
	// Good and Poor are MOS limits: good from Good, poor under Poor, fair in between.
	Good   float64 `json:"good"`
	Poor   float64 `json:"poor"`
	Jitter float64 `json:"jitter"` // ms
	Loss   float64 `json:"loss"`   // percent
	RTT    float64 `json:"rtt"`    // ms
	Trunk  Rule    `json:"trunk"`
	Phone  Rule    `json:"phone"`
	// EachCall mails every call leg under the poor limit.
	EachCall bool `json:"eachCall"`
	// Recovered mails when a trunk or phone is back within its limits.
	Recovered  bool     `json:"recovered"`
	Email      bool     `json:"email"`
	Recipients []string `json:"recipients"`
	// Quiet is the time (minutes) before the same alert is mailed again.
	Quiet int `json:"quiet"`
	// KeepDays is how long measurements are kept.
	KeepDays int `json:"keepDays"`
}

// Defaults are the settings of a new installation.
func Defaults() Settings {
	return Settings{
		Good: 4.0, Poor: 3.6, Jitter: 30, Loss: 2, RTT: 300,
		Trunk:     Rule{Enabled: true, Minutes: 15, Count: 3},
		Phone:     Rule{Enabled: true, Minutes: 60, Count: 3},
		Recovered: true, Recipients: []string{}, Quiet: 30, KeepDays: 90,
	}
}

// Load returns the settings, with defaults.
func Load(ctx context.Context, db *sql.DB) Settings {
	s := Defaults()
	_ = store.GetJSON(ctx, db, SettingsKey, &s)
	if s.Recipients == nil {
		s.Recipients = []string{}
	}
	return s
}

// FieldError names the setting that is not valid.
type FieldError struct{ Field, Code string }

func (e *FieldError) Error() string { return e.Code + ": " + e.Field }

func bad(field string) error { return &FieldError{Field: field, Code: "invalid"} }

// Validate checks the settings and cleans the recipients.
func (s *Settings) Validate() error {
	switch {
	case s.Poor < 1 || s.Poor > 4.5:
		return bad("poor")
	case s.Good <= s.Poor || s.Good > 4.5:
		return bad("good")
	case s.Jitter < 1 || s.Jitter > 1000:
		return bad("jitter")
	case s.Loss < 0.1 || s.Loss > 50:
		return bad("loss")
	case s.RTT < 10 || s.RTT > 5000:
		return bad("rtt")
	case s.Trunk.Minutes < 5 || s.Trunk.Minutes > 1440:
		return bad("trunkMinutes")
	case s.Trunk.Count < 1 || s.Trunk.Count > 1000:
		return bad("trunkCount")
	case s.Phone.Minutes < 5 || s.Phone.Minutes > 1440:
		return bad("phoneMinutes")
	case s.Phone.Count < 1 || s.Phone.Count > 1000:
		return bad("phoneCount")
	case s.Quiet < 1 || s.Quiet > 1440:
		return bad("quiet")
	case s.KeepDays < 1 || s.KeepDays > 730:
		return bad("keepDays")
	}
	clean := []string{}
	for _, m := range s.Recipients {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if a, err := mail.ParseAddress(m); err != nil || a.Address != m {
			return bad("recipients")
		}
		if !slices.Contains(clean, m) {
			clean = append(clean, m)
		}
	}
	if len(clean) > 20 {
		return bad("recipients")
	}
	s.Recipients = clean
	if s.Email && len(clean) == 0 {
		return &FieldError{Field: "recipients", Code: "required"}
	}
	return nil
}

// Save checks and stores the settings.
func Save(ctx context.Context, db *sql.DB, s *Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return store.SetJSON(ctx, db, SettingsKey, s)
}
