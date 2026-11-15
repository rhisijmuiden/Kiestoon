package wg

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestKeysAndRender(t *testing.T) {
	priv, pub, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PublicKey(priv); got != pub {
		t.Fatalf("public key mismatch")
	}
	_, peer, _ := NewKey()
	i := Interface{Name: "kt0", PrivateKey: priv, Address: "10.77.1.1/24", ListenPort: 51820,
		Peers: []Peer{{PublicKey: peer, AllowedIPs: []string{"10.77.1.2/32"}, Endpoint: "pbx.example.com:51820", Keepalive: 25}}}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	full := i.Render(true)
	for _, want := range []string{Marker, "Address = 10.77.1.1/24", "ListenPort = 51820", "AllowedIPs = 10.77.1.2/32", "Endpoint = pbx.example.com:51820", "PersistentKeepalive = 25"} {
		if !strings.Contains(full, want) {
			t.Errorf("missing %q in\n%s", want, full)
		}
	}
	if strings.Contains(i.Render(false), "Address") {
		t.Error("syncconf format must not contain Address")
	}
	bad := i
	bad.Name = "eth0"
	if bad.Validate() == nil {
		t.Error("non-kt name accepted")
	}
	bad = i
	bad.Peers = []Peer{{PublicKey: peer, AllowedIPs: []string{"10.77.1.2/32"}, Endpoint: "x;rm -rf /:1"}}
	if bad.Validate() == nil {
		t.Error("bad endpoint accepted")
	}
}

func TestAddresses(t *testing.T) {
	sub := netip.MustParsePrefix("10.77.5.0/24")
	a, _ := HostAddr(sub, 1)
	if a.String() != "10.77.5.1" {
		t.Fatal(a)
	}
	n, err := NextFree(sub, []string{"10.77.5.2/32", "10.77.5.3"})
	if err != nil || n.String() != "10.77.5.4" {
		t.Fatal(n, err)
	}
	if _, err := HostAddr(sub, 255); err == nil {
		t.Fatal("broadcast address handed out")
	}
	r := RandomSubnet()
	if !netip.MustParsePrefix("10.64.0.0/10").Contains(r.Addr()) || r.Bits() != 24 {
		t.Fatal(r)
	}
}

func TestParseDump(t *testing.T) {
	out := "kt0\tPRIV\tPUB\t51820\toff\n" +
		"kt0\tPEERKEY\t(none)\t203.0.113.5:40000\t10.77.1.2/32\t1700000000\t1200\t3400\t25\n" +
		"kt0\tOTHER\t(none)\t(none)\t10.77.1.3/32\t0\t0\t0\toff\n"
	ps := ParseDump(out)
	if len(ps) != 2 || ps[0].Endpoint != "203.0.113.5:40000" || ps[0].RxBytes != 1200 || ps[1].Endpoint != "" {
		t.Fatalf("%+v", ps)
	}
	if !ps[0].Connected(time.Unix(1700000060, 0)) || ps[1].Connected(time.Now()) {
		t.Fatal("connected state")
	}
}
