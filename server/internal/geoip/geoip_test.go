package geoip

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestRangePrefixes(t *testing.T) {
	cases := []struct {
		from, to string
		want     []string
	}{
		{"1.0.0.0", "1.0.0.255", []string{"1.0.0.0/24"}},
		{"1.0.0.0", "1.0.1.255", []string{"1.0.0.0/23"}},
		{"1.0.0.1", "1.0.0.6", []string{"1.0.0.1/32", "1.0.0.2/31", "1.0.0.4/31", "1.0.0.6/32"}},
		{"0.0.0.0", "255.255.255.255", []string{"0.0.0.0/0"}},
		{"10.0.0.5", "10.0.0.5", []string{"10.0.0.5/32"}},
		{"2001:db8::", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff", []string{"2001:db8::/32"}},
	}
	for _, c := range cases {
		var got []string
		for _, p := range RangePrefixes(netip.MustParseAddr(c.from), netip.MustParseAddr(c.to)) {
			got = append(got, p.String())
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s-%s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	csv := "1.0.0.0,1.0.0.255,AU\n2.56.0.0,2.56.3.255,NL\n2a00:1450::,2a00:1450:ffff:ffff:ffff:ffff:ffff:ffff,NL\nbad,line\n"
	s, err := Parse(strings.NewReader(csv), []string{"nl"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.V4, []string{"2.56.0.0/22"}) || !reflect.DeepEqual(s.V6, []string{"2a00:1450::/32"}) {
		t.Errorf("%+v", s)
	}
	if _, err := NormalizeCountries([]string{"nl", "BE", "nl", "xyz"}); err == nil {
		t.Error("invalid country accepted")
	}
	c, _ := NormalizeCountries([]string{"nl", "BE", "nl"})
	if !reflect.DeepEqual(c, []string{"BE", "NL"}) {
		t.Error(c)
	}
}
