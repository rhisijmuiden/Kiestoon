// Package cluster runs an active/passive pair of Kiestoon nodes (high availability).
//
// The primary node works as usual. The standby node runs only a small status portal: it copies a
// consistent snapshot of the primary's database every few seconds and keeps the voicemail,
// recordings, prompts, faxes, firmware and certificates in sync. Asterisk is stopped on the standby.
//
// A virtual IP address (keepalived, VRRP) moves to the standby when the primary fails; keepalived
// then calls `kiestoon cluster-notify master` and the root helper promotes the standby: it puts the
// newest database in place, restarts the portal as primary and starts Asterisk. The portal
// regenerates every configuration file from the database, so nothing else needs copying.
// Without a virtual IP (DNS SRV records point the phones at both nodes) the switch is made by hand.
//
// All traffic between the nodes runs through a WireGuard tunnel (interface kt-ha).
package cluster

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Roles of a node.
const (
	RolePrimary = "primary"
	RoleStandby = "standby"
)

// Modes of a pair.
const (
	// ModeVIP moves a virtual IP address with keepalived and fails over automatically.
	ModeVIP = "vip"
	// ModeManual has no shared address (DNS SRV records); switching is done in the portal.
	ModeManual = "manual"
)

// Interface is the WireGuard interface between the nodes.
const Interface = "kt-ha"

// Peer is the other node.
type Peer struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	PublicKey string `json:"publicKey"`
	Tunnel    string `json:"tunnel"`
}

// Node is the configuration of this node (DataDir/cluster/node.json, written by the portal).
type Node struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Name    string `json:"name"`
	// Address is the IP address of this node in the network both nodes share.
	Address string `json:"address"`
	// Interface carries the virtual IP; empty is the interface of the default route.
	Interface string `json:"interface"`
	// VIP is the virtual IP address with prefix, for example 192.168.1.50/24.
	VIP      string `json:"vip"`
	RouterID int    `json:"routerId"`
	// Priority: the node that started the pair is preferred when both start at the same time.
	Priority   int    `json:"priority"`
	Secret     string `json:"secret"`
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	Tunnel     string `json:"tunnel"`
	Port       int    `json:"port"`
	Peer       *Peer  `json:"peer,omitempty"`
	// JoinHash and JoinExpires: a join code waiting for the second node.
	JoinHash    string `json:"joinHash,omitempty"`
	JoinExpires string `json:"joinExpires,omitempty"`
}

// Role is the current role of this node (DataDir/cluster/role.json, written by the root helper).
type Role struct {
	Role string `json:"role"`
	// Epoch counts promotions. The data in the local database belongs to this epoch.
	Epoch int64 `json:"epoch"`
	// LastSeq is the last snapshot taken while this node was primary in Epoch.
	LastSeq   int64     `json:"lastSeq"`
	ChangedAt time.Time `json:"changedAt"`
	Reason    string    `json:"reason"`
}

// Meta describes a database snapshot.
type Meta struct {
	Epoch  int64     `json:"epoch"`
	Seq    int64     `json:"seq"`
	SHA256 string    `json:"sha256"`
	Size   int64     `json:"size"`
	At     time.Time `json:"at"`
}

// Newer reports whether m is a later snapshot than o.
func (m Meta) Newer(o Meta) bool {
	return m.Epoch > o.Epoch || (m.Epoch == o.Epoch && m.Seq > o.Seq)
}

// Dir is the cluster directory inside the data directory. It is never replicated.
func Dir(dataDir string) string { return filepath.Join(dataDir, "cluster") }

func nodePath(dataDir string) string { return filepath.Join(Dir(dataDir), "node.json") }
func rolePath(dataDir string) string { return filepath.Join(Dir(dataDir), "role.json") }

// SnapshotPath is the last snapshot the primary took.
func SnapshotPath(dataDir string) string { return filepath.Join(Dir(dataDir), "snapshot.db") }

// ReplicaPath is the last snapshot the standby received.
func ReplicaPath(dataDir string) string { return filepath.Join(Dir(dataDir), "replica.db") }

// LoadNode reads the node configuration; a missing file is a disabled node.
func LoadNode(dataDir string) Node {
	var n Node
	b, err := os.ReadFile(nodePath(dataDir))
	if err == nil {
		_ = json.Unmarshal(b, &n)
	}
	return n
}

// SaveNode writes the node configuration.
func SaveNode(dataDir string, n Node) error { return writeJSON(nodePath(dataDir), n) }

// RemoveNode switches high availability off on this node.
func RemoveNode(dataDir string) {
	_ = os.Remove(nodePath(dataDir))
	_ = os.Remove(rolePath(dataDir))
}

// LoadRole reads the role; without a file the node is primary of epoch 1.
func LoadRole(dataDir string) Role {
	r := Role{Role: RolePrimary, Epoch: 1}
	b, err := os.ReadFile(rolePath(dataDir))
	if err == nil {
		_ = json.Unmarshal(b, &r)
	}
	if r.Role != RoleStandby {
		r.Role = RolePrimary
	}
	return r
}

// SaveRole writes the role.
func SaveRole(dataDir string, r Role) error {
	r.ChangedAt = time.Now().UTC()
	return writeJSON(rolePath(dataDir), r)
}

// LoadMeta reads the description of a snapshot file (path + ".json").
func LoadMeta(dbPath string) (Meta, bool) {
	var m Meta
	b, err := os.ReadFile(dbPath + ".json")
	if err != nil || json.Unmarshal(b, &m) != nil {
		return Meta{}, false
	}
	if _, err := os.Stat(dbPath); err != nil {
		return Meta{}, false
	}
	return m, true
}

// SaveMeta writes the description of a snapshot file.
func SaveMeta(dbPath string, m Meta) error { return writeJSON(dbPath+".json", m) }

// IsStandby reports whether this node must run as standby.
func IsStandby(dataDir string) bool {
	return LoadNode(dataDir).Enabled && LoadRole(dataDir).Role == RoleStandby
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RandomTunnel picks the tunnel addresses of a pair in 10.128.0.0/10 (.1 and .2 of a /30).
func RandomTunnel() (first, second string) {
	var b [3]byte
	_, _ = rand.Read(b[:])
	base := [4]byte{10, 128 + b[0]%64, b[1], (b[2] % 64) * 4}
	a1, a2 := base, base
	a1[3]++
	a2[3] += 2
	return netip.AddrFrom4(a1).String(), netip.AddrFrom4(a2).String()
}

// Validate checks the settings an administrator enters.
func (n *Node) Validate() error {
	if n.Mode != ModeVIP && n.Mode != ModeManual {
		return errors.New("mode")
	}
	if a, err := netip.ParseAddr(n.Address); err != nil || !a.Is4() {
		return errors.New("address")
	}
	if n.Mode == ModeVIP {
		p, err := netip.ParsePrefix(n.VIP)
		if err != nil || !p.Addr().Is4() || p.Bits() < 8 || p.Bits() > 32 {
			return errors.New("vip")
		}
		if p.Addr().String() == n.Address {
			return errors.New("vip")
		}
	} else {
		n.VIP = ""
	}
	if n.Interface != "" && !validIface(n.Interface) {
		return errors.New("interface")
	}
	return nil
}

func validIface(s string) bool {
	if len(s) == 0 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '@') {
			return false
		}
	}
	return true
}

// Keepalived builds /etc/keepalived/keepalived.conf. Both nodes start as BACKUP with nopreempt, so
// a node that comes back after a failure does not take the address back by itself.
func Keepalived(n Node, iface string) (string, error) {
	if n.Peer == nil || n.Mode != ModeVIP {
		return "", errors.New("no peer")
	}
	if !validIface(iface) {
		return "", errors.New("interface")
	}
	if _, err := netip.ParseAddr(n.Peer.Address); err != nil {
		return "", errors.New("peer address")
	}
	if _, err := netip.ParseAddr(n.Address); err != nil {
		return "", errors.New("address")
	}
	if _, err := netip.ParsePrefix(n.VIP); err != nil {
		return "", errors.New("vip")
	}
	if n.RouterID < 1 || n.RouterID > 255 || n.Priority < 1 || n.Priority > 254 {
		return "", errors.New("router id")
	}
	pass := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, n.Secret)
	if len(pass) > 8 {
		pass = pass[:8]
	}
	if len(pass) < 8 {
		return "", errors.New("secret")
	}
	var b strings.Builder
	b.WriteString("# Generated by Kiestoon. Changes are overwritten.\n")
	b.WriteString("global_defs {\n  router_id kiestoon\n  enable_script_security\n  script_user root\n}\n\n")
	b.WriteString("vrrp_instance KIESTOON {\n")
	b.WriteString("  state BACKUP\n  nopreempt\n")
	fmt.Fprintf(&b, "  interface %s\n", iface)
	fmt.Fprintf(&b, "  virtual_router_id %d\n", n.RouterID)
	fmt.Fprintf(&b, "  priority %d\n", n.Priority)
	b.WriteString("  advert_int 1\n")
	fmt.Fprintf(&b, "  unicast_src_ip %s\n", n.Address)
	fmt.Fprintf(&b, "  unicast_peer {\n    %s\n  }\n", n.Peer.Address)
	fmt.Fprintf(&b, "  authentication {\n    auth_type PASS\n    auth_pass %s\n  }\n", pass)
	fmt.Fprintf(&b, "  virtual_ipaddress {\n    %s dev %s\n  }\n", n.VIP, iface)
	b.WriteString("  garp_master_delay 1\n  garp_master_repeat 3\n")
	b.WriteString("  notify_master \"/usr/bin/kiestoon cluster-notify master\"\n")
	b.WriteString("  notify_backup \"/usr/bin/kiestoon cluster-notify backup\"\n")
	b.WriteString("  notify_fault \"/usr/bin/kiestoon cluster-notify fault\"\n")
	b.WriteString("}\n")
	return b.String(), nil
}
