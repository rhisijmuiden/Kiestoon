package pbx

import (
	"context"
	"database/sql"
	"regexp"
	"slices"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// HolidaysKey is the settings row that holds the holiday calendar.
const HolidaysKey = "pbx_holidays"

// CustomHoliday is a day the business is closed, for example a company outing.
type CustomHoliday struct {
	Date   string `json:"date"` // YYYY-MM-DD
	Name   string `json:"name"`
	Annual bool   `json:"annual"` // repeats every year on the same day and month
}

// Holidays is the holiday calendar used by time conditions.
type Holidays struct {
	// Preset holidays that are switched on (keys of DutchHolidays).
	Preset []string        `json:"preset"`
	Custom []CustomHoliday `json:"custom"`
}

// Holiday is one closed day in a given year.
type Holiday struct {
	Date string `json:"date"`
	Key  string `json:"key,omitempty"` // translation key for preset holidays
	Name string `json:"name,omitempty"`
}

// DutchHolidays are the national holidays of the Netherlands. Bevrijdingsdag is a day off every
// five years for many collective agreements, so it is listed but not on by default.
var DutchHolidays = []string{"newYear", "goodFriday", "easterSunday", "easterMonday", "kingsDay", "liberationDay",
	"ascension", "whitSunday", "whitMonday", "christmas", "boxingDay"}

// DefaultHolidays are on for a fresh install.
func DefaultHolidays() Holidays {
	return Holidays{Preset: []string{"newYear", "easterSunday", "easterMonday", "kingsDay", "ascension", "whitSunday",
		"whitMonday", "christmas", "boxingDay"}, Custom: []CustomHoliday{}}
}

// LoadHolidays reads the holiday calendar.
func LoadHolidays(ctx context.Context, db *sql.DB) (Holidays, error) {
	h := DefaultHolidays()
	err := store.GetJSON(ctx, db, HolidaysKey, &h)
	if h.Preset == nil {
		h.Preset = []string{}
	}
	if h.Custom == nil {
		h.Custom = []CustomHoliday{}
	}
	return h, err
}

var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Validate checks the calendar.
func (h *Holidays) Validate() error {
	out := []string{}
	for _, k := range h.Preset {
		if !slices.Contains(DutchHolidays, k) {
			return invalid("invalid", "preset")
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	h.Preset = out
	if len(h.Custom) > 100 {
		return invalid("invalid", "custom")
	}
	for i := range h.Custom {
		c := &h.Custom[i]
		c.Name = cleanText(c.Name, 60)
		if _, err := time.Parse("2006-01-02", c.Date); err != nil || !reDate.MatchString(c.Date) {
			return invalid("invalid", "custom")
		}
	}
	return nil
}

// Easter returns Easter Sunday of a year (Anonymous Gregorian algorithm).
func Easter(year int) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// DutchHoliday returns the date of a preset holiday in a year.
func DutchHoliday(key string, year int) time.Time {
	e := Easter(year)
	d := func(m time.Month, day int) time.Time { return time.Date(year, m, day, 0, 0, 0, 0, time.UTC) }
	switch key {
	case "newYear":
		return d(time.January, 1)
	case "goodFriday":
		return e.AddDate(0, 0, -2)
	case "easterSunday":
		return e
	case "easterMonday":
		return e.AddDate(0, 0, 1)
	case "kingsDay":
		// 27 April, or 26 April when the 27th is a Sunday.
		k := d(time.April, 27)
		if k.Weekday() == time.Sunday {
			k = d(time.April, 26)
		}
		return k
	case "liberationDay":
		return d(time.May, 5)
	case "ascension":
		return e.AddDate(0, 0, 39)
	case "whitSunday":
		return e.AddDate(0, 0, 49)
	case "whitMonday":
		return e.AddDate(0, 0, 50)
	case "christmas":
		return d(time.December, 25)
	case "boxingDay":
		return d(time.December, 26)
	}
	return time.Time{}
}

// ForYear lists the closed days of a year, sorted by date.
func (h Holidays) ForYear(year int) []Holiday {
	out := []Holiday{}
	for _, k := range h.Preset {
		out = append(out, Holiday{Date: DutchHoliday(k, year).Format("2006-01-02"), Key: k})
	}
	for _, c := range h.Custom {
		t, err := time.Parse("2006-01-02", c.Date)
		if err != nil {
			continue
		}
		if c.Annual {
			t = time.Date(year, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		} else if t.Year() != year {
			continue
		}
		out = append(out, Holiday{Date: t.Format("2006-01-02"), Name: c.Name})
	}
	slices.SortStableFunc(out, func(a, b Holiday) int {
		if a.Date < b.Date {
			return -1
		}
		if a.Date > b.Date {
			return 1
		}
		return 0
	})
	return out
}
