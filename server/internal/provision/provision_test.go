package provision

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]Request{
		"y000000000000.cfg":         {Vendor: "yealink", Common: true},
		"001565ABCDEF.cfg":          {Vendor: "yealink", MAC: "001565abcdef"},
		"snom/000413abcdef.xml":     {Vendor: "snom", MAC: "000413abcdef"},
		"snomD785.htm":              {Vendor: "snom", Model: "D785", Common: true},
		"snomD785-000413ABCDEF.htm": {Vendor: "snom", Model: "D785", MAC: "000413abcdef"},
		"cfg000b82abcdef.xml":       {Vendor: "grandstream", MAC: "000b82abcdef"},
		"cfg000b82abcdef":           {Vendor: "grandstream", MAC: "000b82abcdef"},
	}
	for in, want := range cases {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("%s: %+v %v", in, got, ok)
		}
	}
	for _, bad := range []string{"../etc/passwd", "001565abcde.cfg", "snom/../x.xml", ""} {
		if _, ok := Parse(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}
