package pbx

import "testing"

func TestDutchHolidays(t *testing.T) {
	cases := map[string]string{"easterSunday": "2026-04-05", "easterMonday": "2026-04-06", "ascension": "2026-05-14",
		"whitMonday": "2026-05-25", "kingsDay": "2026-04-27", "goodFriday": "2026-04-03"}
	for k, want := range cases {
		if got := DutchHoliday(k, 2026).Format("2006-01-02"); got != want {
			t.Errorf("%s 2026 = %s, want %s", k, got, want)
		}
	}
	// 27 April 2025 was a Sunday: King's Day moved to the 26th.
	if got := DutchHoliday("kingsDay", 2025).Format("2006-01-02"); got != "2025-04-26" {
		t.Errorf("kingsDay 2025 = %s", got)
	}
	h := Holidays{Preset: []string{"christmas"}, Custom: []CustomHoliday{{Date: "2020-07-01", Name: "Outing", Annual: true}, {Date: "2027-01-02", Name: "Once"}}}
	days := h.ForYear(2026)
	if len(days) != 2 || days[0].Date != "2026-07-01" || days[1].Date != "2026-12-25" {
		t.Errorf("ForYear: %+v", days)
	}
}

func TestFeaturesValidate(t *testing.T) {
	f := Features{ForwardAlways: "06 1234 5678", FollowMe: FollowMe{Enabled: true, Numbers: []string{"", "+31612345678"}, Delay: 10}}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if f.ForwardAlways != "0612345678" || len(f.FollowMe.Numbers) != 1 {
		t.Errorf("not normalised: %+v", f)
	}
	bad := Features{ForwardBusy: "06;Dial(x)"}
	if bad.Validate() == nil {
		t.Error("dialplan injection accepted")
	}
}
