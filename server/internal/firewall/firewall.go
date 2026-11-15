// Package firewall turns the firewall settings into an nftables ruleset and the fail2ban settings
// into a jail file. Both are applied by the root helper; this package only builds text, so it is
// easy to test and the helper never runs text it did not generate itself.
package firewall

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// SettingsKey is the settings row with Config as JSON.
const SettingsKey = "firewall"

// Config is what the administrator sets in the UI.
type Config struct {
	Enabled bool `json:"enabled"`
	// SIPWhitelist: only these addresses may send SIP (IPs or CIDR ranges). Trunk addresses are added automatically.
	SIPWhitelist []string `json:"sipWhitelist"`
	// AllowPrivateSIP lets every private network (LAN) send SIP.
	AllowPrivateSIP bool `json:"allowPrivateSip"`
	// SIPOpen disables the whitelist and lets everyone send SIP (fail2ban still applies).
	SIPOpen bool `json:"sipOpen"`
	// AdminNetworks may reach the portal and SSH. Empty means everyone.
	AdminNetworks []string `json:"adminNetworks"`
	// Fail2ban
	Fail2banEnabled bool     `json:"fail2banEnabled"`
	BanMinutes      int      `json:"banMinutes"`
	FindMinutes     int      `json:"findMinutes"`
	MaxRetry        int      `json:"maxRetry"`
	IgnoreIPs       []string `json:"ignoreIps"`
	// Geo-blocking: "off", "allow" (only these countries) or "block" (everyone except these countries).
	// Private networks, trunks and the SIP whitelist are never blocked.
	GeoMode      string   `json:"geoMode"`
	GeoCountries []string `json:"geoCountries"`
	// GeoPortal also applies geo-blocking to the web portal (not only to SIP).
	GeoPortal bool `json:"geoPortal"`
}

// Geo holds the networks of the countries in Config.GeoCountries.
type Geo struct {
	V4 []string
	V6 []string
}

// Default is the configuration of a fresh install: firewall off until the admin turns it on.
func Default() Config {
	return Config{AllowPrivateSIP: true, Fail2banEnabled: true, BanMinutes: 60, FindMinutes: 10, MaxRetry: 5,
		SIPWhitelist: []string{}, AdminNetworks: []string{}, IgnoreIPs: []string{}, GeoMode: "off", GeoCountries: []string{}, GeoPortal: true}
}

// Ports are the ports the ruleset opens.
type Ports struct {
	Portal []int
	// Public ports are open to everyone (internet mode: 80 and 443 in front of the portal).
	Public []int
	UDPTL  [2]int // T.38 fax range, zero when no fax is set up
	// LDAP ports of the phone book directory: only for addresses that may send SIP (phones).
	LDAP     []int
	SIP      int
	SIPTLS   int
	RTPStart int
	RTPEnd   int
	// WireGuard are the UDP ports of Kiestoon tunnels (open to everyone: WireGuard only answers
	// packets signed with a known key).
	WireGuard []int
	// Tunnels lets traffic that arrives through a Kiestoon tunnel (interfaces kt*) reach SIP, the
	// media ports and the portal.
	Tunnels bool
	// ClusterPeer is the other node of a high-availability pair: it may send VRRP.
	ClusterPeer string
}

func normalizeList(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			s = ip.String()
		} else if _, n, err := net.ParseCIDR(s); err == nil {
			s = n.String()
		} else {
			return nil, fmt.Errorf("invalid address %q", s)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Validate cleans up and checks the configuration.
func (c *Config) Validate() error {
	var err error
	if c.SIPWhitelist, err = normalizeList(c.SIPWhitelist); err != nil {
		return err
	}
	if c.AdminNetworks, err = normalizeList(c.AdminNetworks); err != nil {
		return err
	}
	if c.IgnoreIPs, err = normalizeList(c.IgnoreIPs); err != nil {
		return err
	}
	if c.GeoMode == "" {
		c.GeoMode = "off"
	}
	if c.GeoMode != "off" && c.GeoMode != "allow" && c.GeoMode != "block" {
		return errors.New("invalid geo mode")
	}
	if c.GeoCountries == nil {
		c.GeoCountries = []string{}
	}
	for i, cc := range c.GeoCountries {
		cc = strings.ToUpper(strings.TrimSpace(cc))
		if len(cc) != 2 || cc[0] < 'A' || cc[0] > 'Z' || cc[1] < 'A' || cc[1] > 'Z' {
			return fmt.Errorf("invalid country %q", cc)
		}
		c.GeoCountries[i] = cc
	}
	sort.Strings(c.GeoCountries)
	if c.GeoMode == "allow" && len(c.GeoCountries) == 0 {
		return errors.New("geo allow list is empty")
	}
	if c.BanMinutes < 1 || c.BanMinutes > 60*24*365 || c.FindMinutes < 1 || c.FindMinutes > 60*24 || c.MaxRetry < 1 || c.MaxRetry > 100 {
		return errors.New("invalid fail2ban settings")
	}
	return nil
}

var privateNets = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7", "fe80::/10"}

func split(list []string) (v4, v6 []string) {
	for _, s := range list {
		if strings.Contains(s, ":") {
			v6 = append(v6, s)
		} else {
			v4 = append(v4, s)
		}
	}
	return
}

func set(b *strings.Builder, name, typ string, elems []string) {
	fmt.Fprintf(b, "\tset %s {\n\t\ttype %s\n\t\tflags interval\n\t\tauto-merge\n", name, typ)
	if len(elems) > 0 {
		fmt.Fprintf(b, "\t\telements = { %s }\n", strings.Join(elems, ", "))
	}
	b.WriteString("\t}\n")
}

func ports(p []int) string {
	s := make([]string, len(p))
	for i, x := range p {
		s[i] = fmt.Sprint(x)
	}
	if len(s) == 1 {
		return s[0]
	}
	return "{ " + strings.Join(s, ", ") + " }"
}

// GeoActive reports whether the ruleset filters by country with the given data.
func GeoActive(c Config, g *Geo) bool {
	return c.GeoMode != "" && c.GeoMode != "off" && len(c.GeoCountries) > 0 && g != nil && len(g.V4)+len(g.V6) > 0
}

// Ruleset builds the nftables table "inet kiestoon". trunkIPs are the resolved trunk addresses and
// geo the networks of the geo-blocking countries (nil when there is no data yet: then no country
// filter is applied, so an allow list can never lock everybody out).
// The input policy is drop: only established traffic, ping, SSH, the portal, SIP from allowed
// addresses and RTP (media) get in.
func Ruleset(c Config, p Ports, trunkIPs []string, geo *Geo) (string, error) {
	cc := c
	if err := cc.Validate(); err != nil {
		return "", err
	}
	sip := append([]string{}, cc.SIPWhitelist...)
	if cc.AllowPrivateSIP {
		sip = append(sip, privateNets...)
	}
	sip = append(sip, trunkIPs...)
	sip, err := normalizeList(sip)
	if err != nil {
		return "", err
	}
	sip4, sip6 := split(sip)
	adm4, adm6 := split(cc.AdminNetworks)

	var b strings.Builder
	b.WriteString("# Generated by Kiestoon. Changes are overwritten.\n")
	b.WriteString("table inet kiestoon\ndelete table inet kiestoon\n")
	b.WriteString("table inet kiestoon {\n")
	set(&b, "sip_allow_v4", "ipv4_addr", sip4)
	set(&b, "sip_allow_v6", "ipv6_addr", sip6)
	set(&b, "admin_allow_v4", "ipv4_addr", adm4)
	set(&b, "admin_allow_v6", "ipv6_addr", adm6)
	geoOn := GeoActive(cc, geo)
	if geoOn {
		g4, err := normalizeList(geo.V4)
		if err != nil {
			return "", err
		}
		g6, err := normalizeList(geo.V6)
		if err != nil {
			return "", err
		}
		p4, p6 := split(privateNets)
		set(&b, "geo_v4", "ipv4_addr", g4)
		set(&b, "geo_v6", "ipv6_addr", g6)
		set(&b, "private_v4", "ipv4_addr", p4)
		set(&b, "private_v6", "ipv6_addr", p6)
		// Trunks, the SIP whitelist and private networks are never blocked by country.
		b.WriteString("\tchain geo {\n")
		b.WriteString("\t\tip saddr @private_v4 return\n\t\tip6 saddr @private_v6 return\n")
		b.WriteString("\t\tip saddr @sip_allow_v4 return\n\t\tip6 saddr @sip_allow_v6 return\n")
		b.WriteString("\t\tip saddr @admin_allow_v4 return\n\t\tip6 saddr @admin_allow_v6 return\n")
		if cc.GeoMode == "allow" {
			b.WriteString("\t\tip saddr @geo_v4 return\n\t\tip6 saddr @geo_v6 return\n")
			b.WriteString("\t\tlimit rate 5/minute log prefix \"kiestoon-geo: \" level info\n\t\tdrop\n")
		} else {
			b.WriteString("\t\tip saddr @geo_v4 limit rate 5/minute log prefix \"kiestoon-geo: \" level info\n")
			b.WriteString("\t\tip saddr @geo_v4 drop\n\t\tip6 saddr @geo_v6 drop\n")
		}
		b.WriteString("\t}\n")
	}
	b.WriteString("\tchain input {\n\t\ttype filter hook input priority filter; policy drop;\n")
	b.WriteString("\t\tct state established,related accept\n")
	b.WriteString("\t\tct state invalid drop\n")
	b.WriteString("\t\tiif lo accept\n")
	b.WriteString("\t\tip protocol icmp icmp type { echo-request, destination-unreachable, time-exceeded, parameter-problem } accept\n")
	b.WriteString("\t\tip6 nexthdr ipv6-icmp accept\n")
	b.WriteString("\t\tudp sport 67 udp dport 68 accept\n")
	b.WriteString("\t\tudp sport 547 udp dport 546 accept\n")

	if geoOn {
		fmt.Fprintf(&b, "\t\tudp dport %d jump geo\n", p.SIP)
		fmt.Fprintf(&b, "\t\ttcp dport { %d, %d } jump geo\n", p.SIP, p.SIPTLS)
		if cc.GeoPortal {
			// Port 80 stays reachable: Let's Encrypt checks the domain from all over the world.
			web := []int{}
			for _, x := range append(append([]int{}, p.Portal...), p.Public...) {
				if x != 80 {
					web = append(web, x)
				}
			}
			if len(web) > 0 {
				fmt.Fprintf(&b, "\t\ttcp dport %s jump geo\n", ports(web))
			}
		}
	}
	if len(p.Public) > 0 {
		fmt.Fprintf(&b, "\t\ttcp dport %s accept\n", ports(p.Public))
	}
	adminPorts := append([]int{22}, p.Portal...)
	if len(cc.AdminNetworks) == 0 {
		fmt.Fprintf(&b, "\t\ttcp dport %s accept\n", ports(adminPorts))
	} else {
		fmt.Fprintf(&b, "\t\tip saddr @admin_allow_v4 tcp dport %s accept\n", ports(adminPorts))
		fmt.Fprintf(&b, "\t\tip6 saddr @admin_allow_v6 tcp dport %s accept\n", ports(adminPorts))
		// Phones in the LAN fetch their configuration from the portal port.
		fmt.Fprintf(&b, "\t\tip saddr @sip_allow_v4 tcp dport %s accept\n", ports(p.Portal))
		fmt.Fprintf(&b, "\t\tip6 saddr @sip_allow_v6 tcp dport %s accept\n", ports(p.Portal))
	}
	if len(p.LDAP) > 0 {
		fmt.Fprintf(&b, "\t\tip saddr @sip_allow_v4 tcp dport %s accept\n", ports(p.LDAP))
		fmt.Fprintf(&b, "\t\tip6 saddr @sip_allow_v6 tcp dport %s accept\n", ports(p.LDAP))
	}
	if cc.SIPOpen {
		fmt.Fprintf(&b, "\t\tudp dport %d accept\n", p.SIP)
		fmt.Fprintf(&b, "\t\ttcp dport { %d, %d } accept\n", p.SIP, p.SIPTLS)
	} else {
		for _, fam := range []string{"ip", "ip6"} {
			setName := "sip_allow_v4"
			if fam == "ip6" {
				setName = "sip_allow_v6"
			}
			fmt.Fprintf(&b, "\t\t%s saddr @%s udp dport %d accept\n", fam, setName, p.SIP)
			fmt.Fprintf(&b, "\t\t%s saddr @%s tcp dport { %d, %d } accept\n", fam, setName, p.SIP, p.SIPTLS)
		}
	}
	if len(p.WireGuard) > 0 {
		fmt.Fprintf(&b, "\t\tudp dport %s accept\n", ports(p.WireGuard))
	}
	if p.Tunnels {
		fmt.Fprintf(&b, "\t\tiifname \"kt*\" udp dport %d accept\n", p.SIP)
		fmt.Fprintf(&b, "\t\tiifname \"kt*\" tcp dport %s accept\n", ports(append([]int{p.SIP, p.SIPTLS}, p.Portal...)))
	}
	if ip := net.ParseIP(p.ClusterPeer); ip != nil && ip.To4() != nil {
		fmt.Fprintf(&b, "\t\tip saddr %s ip protocol vrrp accept\n", ip.String())
	}
	fmt.Fprintf(&b, "\t\tudp dport %d-%d accept\n", p.RTPStart, p.RTPEnd)
	if p.UDPTL[0] > 0 && p.UDPTL[1] >= p.UDPTL[0] {
		fmt.Fprintf(&b, "\t\tudp dport %d-%d accept\n", p.UDPTL[0], p.UDPTL[1])
	}
	b.WriteString("\t\tlimit rate 5/minute log prefix \"kiestoon-drop: \" level info\n")
	b.WriteString("\t}\n}\n")
	return b.String(), nil
}

// Jail builds /etc/fail2ban/jail.d/kiestoon.local.
func Jail(c Config, sipPorts string, portalPorts string) string {
	ignore := append([]string{"127.0.0.1/8", "::1"}, c.IgnoreIPs...)
	enabled := "false"
	if c.Fail2banEnabled {
		enabled = "true"
	}
	return fmt.Sprintf(`# Generated by Kiestoon. Changes are overwritten.
[DEFAULT]
banaction = nftables-allports
banaction_allports = nftables-allports
ignoreip = %s
bantime = %dm
findtime = %dm
maxretry = %d

[kiestoon-sip]
enabled = %s
filter = asterisk
port = %s
logpath = /var/log/asterisk/security.log
          /var/log/asterisk/messages.log
backend = auto

[kiestoon-portal]
enabled = %s
filter = kiestoon-portal
port = %s
backend = systemd
journalmatch = _SYSTEMD_UNIT=kiestoon.service
`, strings.Join(ignore, " "), c.BanMinutes, c.FindMinutes, c.MaxRetry, enabled, sipPorts, enabled, portalPorts)
}
