package firewall

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRuleset(t *testing.T) {
	c := Default()
	c.Enabled = true
	c.SIPWhitelist = []string{"198.51.100.7", "2001:db8::/32", "203.0.113.0/24"}
	c.AdminNetworks = []string{"192.168.1.0/24"}
	rs, err := Ruleset(c, Ports{Portal: []int{8080, 8443}, SIP: 5060, SIPTLS: 5061, RTPStart: 10000, RTPEnd: 20000}, []string{"192.0.2.10"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"policy drop", "192.0.2.10", "198.51.100.7", "2001:db8::/32", "10.0.0.0/8",
		"ip saddr @admin_allow_v4 tcp dport { 22, 8080, 8443 } accept", "udp dport 10000-20000 accept"} {
		if !strings.Contains(rs, want) {
			t.Errorf("ruleset misses %q:\n%s", want, rs)
		}
	}
	// With nft available (and root), let nft check the syntax without applying anything.
	if _, err := exec.LookPath("nft"); err == nil && os.Geteuid() == 0 {
		f, _ := os.CreateTemp(t.TempDir(), "*.nft")
		f.WriteString(rs)
		f.Close()
		if out, err := exec.Command("nft", "-c", "-f", f.Name()).CombinedOutput(); err != nil {
			t.Errorf("nft -c: %v\n%s", err, out)
		}
	}
	if _, err := Ruleset(Config{Enabled: true, SIPWhitelist: []string{"not-an-ip"}, BanMinutes: 1, FindMinutes: 1, MaxRetry: 1}, Ports{}, nil, nil); err == nil {
		t.Error("invalid address accepted")
	}
}

func TestRulesetGeo(t *testing.T) {
	c := Default()
	c.Enabled = true
	c.GeoMode, c.GeoCountries = "allow", []string{"nl", "BE"}
	p := Ports{Portal: []int{8080, 8443}, Public: []int{80, 443}, SIP: 5060, SIPTLS: 5061, RTPStart: 10000, RTPEnd: 20000, UDPTL: [2]int{4000, 4999}}
	rs, err := Ruleset(c, p, nil, &Geo{V4: []string{"2.56.0.0/22"}, V6: []string{"2a00:1450::/32"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"chain geo {", "ip saddr @geo_v4 return", "udp dport 5060 jump geo", "tcp dport { 8080, 8443, 443 } jump geo",
		"tcp dport { 80, 443 } accept", "udp dport 4000-4999 accept", "2.56.0.0/22"} {
		if !strings.Contains(rs, want) {
			t.Errorf("ruleset misses %q:\n%s", want, rs)
		}
	}
	// No data: no country filter, so an allow list cannot lock everyone out.
	rs, _ = Ruleset(c, p, nil, nil)
	if strings.Contains(rs, "jump geo") {
		t.Error("geo filter without data")
	}
	c.GeoMode = "block"
	rs, _ = Ruleset(c, p, nil, &Geo{V4: []string{"1.0.0.0/24"}})
	if !strings.Contains(rs, "ip saddr @geo_v4 drop") {
		t.Error(rs)
	}
	if _, err := Ruleset(Config{Enabled: true, GeoMode: "allow", BanMinutes: 1, FindMinutes: 1, MaxRetry: 1}, p, nil, nil); err == nil {
		t.Error("empty allow list accepted")
	}
	if os.Geteuid() == 0 {
		if _, err := exec.LookPath("nft"); err == nil {
			f, _ := os.CreateTemp(t.TempDir(), "*.nft")
			f.WriteString(rs)
			f.Close()
			if out, err := exec.Command("nft", "-c", "-f", f.Name()).CombinedOutput(); err != nil {
				t.Errorf("nft -c: %v\n%s", err, out)
			}
		}
	}
}

func TestJail(t *testing.T) {
	j := Jail(Default(), "5060,5061", "8080,8443")
	if !strings.Contains(j, "[kiestoon-sip]") || !strings.Contains(j, "bantime = 60m") {
		t.Error(j)
	}
}
