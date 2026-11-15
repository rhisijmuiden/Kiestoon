package asterisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func multisiteSample() *pbx.Snapshot {
	s := sample()
	s.Settings.LocalNets = []string{"192.168.0.0/16"}
	s.Hub = pbx.HubSettings{Enabled: true, Subnet: "10.77.12.0/24", ListenPort: 51820, Host: "pbx.example.com"}
	s.Peers = []pbx.FedPeer{
		{ID: 1, Name: "New York", Prefix: "8", Role: "hosted", State: "active", TunnelIP: "10.77.12.5", LocalIP: "10.77.12.1", SIPPort: 5060,
			Directory: multisite.Directory{Entries: []multisite.DirectoryEntry{{Number: "301", Name: "John", Kind: "extension"}, {Number: "400", Name: "Support", Kind: "queue"}}}},
		{ID: 2, Name: "London", Prefix: "7", Role: "joined", State: "active", TunnelIP: "10.90.3.1", LocalIP: "10.90.3.9", SIPPort: 5060, DirectDial: true,
			Directory: multisite.Directory{Entries: []multisite.DirectoryEntry{{Number: "501", Name: "Mary", Kind: "extension"}}}},
		{ID: 3, Name: "Pending", Prefix: "6", Role: "hosted", State: "invited"},
	}
	s.Queues = []pbx.Queue{{ID: 1, Number: "600", Name: "Sales", Strategy: "ringall", AgentTimeout: 15, Retry: 5, MaxWait: 300, Enabled: true,
		Overflow:      pbx.Destination{Type: "hangup"},
		Members:       []pbx.QueueMember{{ExtensionID: 1, Number: "201", Name: "Anna de Vries"}},
		RemoteMembers: []pbx.RemoteMember{{PeerID: 1, Number: "301", Name: "John", Site: "New York"}}}}
	s.SunRoutes = []pbx.SunRoute{{ID: 1, Name: "Support 24h", Enabled: true, Fallback: pbx.Destination{Type: "remote", Target: "1:400"}, Rules: []pbx.SunRule{
		{Label: "Amsterdam", Timezone: "Europe/Amsterdam", Days: []int{1, 2, 3, 4, 5}, From: "08:00", To: "17:00", Destination: pbx.Destination{Type: "queue", Target: "1"}},
		{Label: "New York", Timezone: "America/New_York", Days: []int{1, 2, 3, 4, 5}, From: "09:00", To: "17:00", Destination: pbx.Destination{Type: "remote", Target: "1:400"}},
	}}}
	s.Inbound = append(s.Inbound, pbx.InboundRoute{ID: 3, Name: "Support", DID: "0207777777", DestinationType: "followsun", Destination: "1", Enabled: true})
	return s
}

func TestGenerateMultisite(t *testing.T) {
	files := Generate(multisiteSample(), Options{AMIUser: "kiestoon", AMISecret: "amisecret", TLSCert: "/tmp/c.crt", TLSKey: "/tmp/c.key"})
	pj, dp, q := files["pjsip.conf"], files["extensions.conf"], files["queues.conf"]
	for _, want := range []string{"[fed-1]\ntype=endpoint", "[fed-2]\ntype=endpoint", "local_net=10.77.12.0/24", "match=10.77.12.5", "match=10.90.3.1"} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf misses %q", want)
		}
	}
	if strings.Contains(pj, "fed-3") {
		t.Error("an invited peer must not get an endpoint")
	}
	for _, want := range []string{"include => kiestoon-fed", "[kiestoon-fedout-1]", "[kiestoon-from-fed-2]", "Custom:ktfed1-301", "[kiestoon-sun-1]", "Europe/Amsterdam", "America/New_York"} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	if strings.Index(dp, "include => kiestoon-fed") > strings.Index(dp, "include => kiestoon-outbound") {
		t.Error("federation numbers must come before outbound routes")
	}
	if !strings.Contains(q, "Local/301@kiestoon-fedout-1/n") {
		t.Errorf("queues.conf misses the remote agent:\n%s", q)
	}
	// Directory entries arrive from the other system: nothing in them may break out of a line.
	s := multisiteSample()
	s.Peers[0].Directory.Entries = append(s.Peers[0].Directory.Entries, multisite.DirectoryEntry{Number: "30\n[evil]", Name: "x\ninclude => y", Kind: "extension"})
	evil := Generate(s, Options{})
	if strings.Contains(evil["extensions.conf"], "[evil]") || strings.Contains(evil["extensions.conf"], "include => y") {
		t.Error("directory entry injected configuration")
	}
	if out := os.Getenv("KT_GEN_OUT"); out != "" {
		for n, c := range files {
			_ = os.WriteFile(filepath.Join(out, n), []byte(c), 0o644)
		}
	}
}
