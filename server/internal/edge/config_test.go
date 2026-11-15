package edge

import (
	"os"
	"strings"
	"testing"
)

func testConfig() ProxyConfig {
	return ProxyConfig{LANAddress: "192.168.1.20", TunnelIP: "10.70.3.2", HubIP: "10.70.3.1", HubSIPPort: 5060, RTPStart: RTPStart, RTPEnd: RTPEnd}
}

func TestKamailio(t *testing.T) {
	c := testConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	k := Kamailio(c)
	for _, want := range []string{"listen=udp:192.168.1.20:5060", "listen=udp:10.70.3.2:5060", `$du = "sip:10.70.3.1:5060"`, `$si == "10.70.3.1"`} {
		if !strings.Contains(k, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(k, "@") && strings.Contains(k, "@HUB@") {
		t.Error("placeholder left")
	}
	if p := os.Getenv("KAMAILIO_OUT"); p != "" {
		_ = os.WriteFile(p, []byte(k), 0o644)
		_ = os.WriteFile(p+".rtp", []byte(RTPEngine(c)), 0o644)
	}
}

func TestValidateRejects(t *testing.T) {
	for _, c := range []ProxyConfig{
		{LANAddress: "1.2.3.4\nlisten=x", TunnelIP: "10.0.0.2", HubIP: "10.0.0.1", HubSIPPort: 5060, RTPStart: 30000, RTPEnd: 30010},
		{LANAddress: "1.2.3.4", TunnelIP: "10.0.0.2", HubIP: "10.0.0.1", HubSIPPort: 0, RTPStart: 30000, RTPEnd: 30010},
		{LANAddress: "1.2.3.4", TunnelIP: "10.0.0.2", HubIP: "10.0.0.1", HubSIPPort: 5060, RTPStart: 30000, RTPEnd: 100},
	} {
		if c.Validate() == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}
