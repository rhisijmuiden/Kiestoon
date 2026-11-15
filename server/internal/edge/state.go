package edge

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"
)

// State is what the edge remembers in /var/lib/kiestoon-edge/state.json.
type State struct {
	Paired       bool   `json:"paired"`
	SiteID       int64  `json:"siteId,omitempty"`
	Name         string `json:"name,omitempty"`
	Token        string `json:"token,omitempty"`
	PrivateKey   string `json:"privateKey,omitempty"`
	Address      string `json:"address,omitempty"` // own tunnel address with prefix
	HubAddress   string `json:"hubAddress,omitempty"`
	HubPublicKey string `json:"hubPublicKey,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	HubURL       string `json:"hubUrl,omitempty"`
	SIPPort      int    `json:"sipPort,omitempty"`
	PortalPort   int    `json:"portalPort,omitempty"`
	RTPStart     int    `json:"rtpStart,omitempty"`
	RTPEnd       int    `json:"rtpEnd,omitempty"`
	// LANAddress pins the office address; empty means detect it.
	LANAddress string    `json:"lanAddress,omitempty"`
	PairedAt   time.Time `json:"pairedAt,omitempty"`
}

// TunnelIP is the own tunnel address without prefix.
func (s State) TunnelIP() string {
	if p, err := netip.ParsePrefix(s.Address); err == nil {
		return p.Addr().String()
	}
	return ""
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

// LoadState reads the state; a missing file is an unpaired edge.
func LoadState(dir string) State {
	var s State
	b, err := os.ReadFile(statePath(dir))
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// SaveState writes the state atomically (it holds the tunnel key and the token).
func SaveState(dir string, s State) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(dir))
}

// LocalAddress returns the address this machine uses to reach host, and the network it is in.
// Towards the public address of the central Kiestoon that is the office (LAN) address.
func LocalAddress(host string) (addr, network string, err error) {
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	c, err := net.Dial("udp", net.JoinHostPort(host, "9"))
	if err != nil {
		return "", "", err
	}
	defer c.Close()
	ua, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", "", errors.New("no local address")
	}
	a, _ := netip.AddrFromSlice(ua.IP)
	a = a.Unmap()
	return a.String(), networkOf(a), nil
}

func networkOf(a netip.Addr) string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, x := range addrs {
			n, ok := x.(*net.IPNet)
			if !ok {
				continue
			}
			ip, _ := netip.AddrFromSlice(n.IP)
			if ip.Unmap() == a {
				ones, _ := n.Mask.Size()
				return netip.PrefixFrom(a, ones).Masked().String()
			}
		}
	}
	return ""
}
