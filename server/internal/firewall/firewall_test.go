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
	rs, err := Ruleset(c, Ports{Portal: []int{8080, 8443}, SIP: 5060, SIPTLS: 5061, RTPStart: 10000, RTPEnd: 20000}, []string{"192.0.2.10"})
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
	if _, err := Ruleset(Config{Enabled: true, SIPWhitelist: []string{"not-an-ip"}, BanMinutes: 1, FindMinutes: 1, MaxRetry: 1}, Ports{}, nil); err == nil {
		t.Error("invalid address accepted")
	}
}

func TestJail(t *testing.T) {
	j := Jail(Default(), "5060,5061", "8080,8443")
	if !strings.Contains(j, "[kiestoon-sip]") || !strings.Contains(j, "bantime = 60m") {
		t.Error(j)
	}
}
