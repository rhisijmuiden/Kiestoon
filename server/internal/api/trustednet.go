package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// TrustedNetworksKey is the settings row with TrustedNetworks as JSON.
const TrustedNetworksKey = "trusted_networks"

// TrustedNetwork is an IP range (an office LAN or VPN) from which portal sign-ins may skip MFA.
type TrustedNetwork struct {
	CIDR        string `json:"cidr"`
	Description string `json:"description"`
}

// TrustedNetworks lets password sign-ins from known networks skip the second factor. It never
// applies to single sign-on (the identity provider asks for MFA) or API tokens (no sign-in).
type TrustedNetworks struct {
	Enabled  bool             `json:"enabled"`
	Networks []TrustedNetwork `json:"networks"`
	// Roles that may skip MFA from a trusted network. Roles not listed always need their code.
	Roles []string `json:"roles"`
}

// DefaultTrustedNetworks is off, with every role except super admins allowed once switched on.
func DefaultTrustedNetworks() TrustedNetworks {
	return TrustedNetworks{Networks: []TrustedNetwork{}, Roles: []string{"user", "reception", "admin"}}
}

// Validate cleans up and checks the settings. The error is the field that is wrong.
func (t *TrustedNetworks) Validate() error {
	out := []TrustedNetwork{}
	seen := map[string]bool{}
	for _, n := range t.Networks {
		v := strings.TrimSpace(n.CIDR)
		if v == "" {
			continue
		}
		p, err := parsePrefix(v)
		if err != nil {
			return errors.New("networks")
		}
		if tooBroad(p) {
			return errors.New("networks_too_broad")
		}
		d := strings.TrimSpace(n.Description)
		if utf8.RuneCountInString(d) > 80 || strings.ContainsAny(d, "\r\n") {
			return errors.New("description")
		}
		if seen[p.String()] {
			continue
		}
		seen[p.String()] = true
		out = append(out, TrustedNetwork{CIDR: p.String(), Description: d})
	}
	if len(out) > 64 {
		return errors.New("networks")
	}
	roles := []string{}
	for _, r := range t.Roles {
		if !slices.Contains(auth.Roles, r) {
			return errors.New("roles")
		}
		if !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	if t.Enabled && len(out) == 0 {
		return errors.New("networks")
	}
	t.Networks, t.Roles = out, roles
	return nil
}

// tooBroad rejects ranges that cover a large part of the internet: 0.0.0.0/0, ::/0 and anything
// wider than a /8 (IPv4) or the fc00::/7 unique local block (IPv6).
func tooBroad(p netip.Prefix) bool {
	if p.Addr().Is4() {
		return p.Bits() < 8
	}
	return p.Bits() < 7
}

// isPublicRange reports whether a range holds internet addresses. Allowed (an office with a fixed
// public IP), but the page warns about it.
func isPublicRange(p netip.Prefix) bool {
	p = p.Masked()
	a := p.Addr()
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || cgnat.Contains(a) {
		// The whole range must stay inside the private block, so 10.0.0.0/7 counts as public.
		last := lastAddr(p)
		return !(last.IsPrivate() || last.IsLoopback() || last.IsLinkLocalUnicast() || cgnat.Contains(last))
	}
	return true
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().AsSlice()
	for i := range b {
		bits := p.Bits() - i*8
		switch {
		case bits >= 8:
		case bits <= 0:
			b[i] = 0xff
		default:
			b[i] |= 0xff >> bits
		}
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// activeTrusted is what the sign-in and every request check, read lock-free.
var activeTrusted atomic.Pointer[TrustedNetworks]

func setActiveTrusted(t TrustedNetworks) {
	if !t.Enabled {
		t = TrustedNetworks{}
	}
	activeTrusted.Store(&t)
}

// trustedNetworkFor returns the trusted network an address is in, when a user with this role may
// skip MFA from there.
func trustedNetworkFor(ip, role string) (TrustedNetwork, bool) {
	t := activeTrusted.Load()
	if t == nil || !t.Enabled || !slices.Contains(t.Roles, role) {
		return TrustedNetwork{}, false
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return TrustedNetwork{}, false
	}
	a = a.Unmap()
	for _, n := range t.Networks {
		if p, err := netip.ParsePrefix(n.CIDR); err == nil && p.Contains(a) {
			return n, true
		}
	}
	return TrustedNetwork{}, false
}

func (s *Server) trustedNetworks(ctx context.Context) TrustedNetworks {
	t := DefaultTrustedNetworks()
	_ = store.GetJSON(ctx, s.DB, TrustedNetworksKey, &t)
	if t.Networks == nil {
		t.Networks = []TrustedNetwork{}
	}
	if t.Roles == nil {
		t.Roles = []string{}
	}
	return t
}

// LoadTrustedNetworks activates the stored trusted networks; called at start-up.
func (s *Server) LoadTrustedNetworks(ctx context.Context) {
	setActiveTrusted(s.trustedNetworks(ctx))
}

func (s *Server) trustedBody(r *http.Request, t TrustedNetworks) map[string]any {
	public := []string{}
	for _, n := range t.Networks {
		if p, err := netip.ParsePrefix(n.CIDR); err == nil && isPublicRange(p) {
			public = append(public, n.CIDR)
		}
	}
	ip := clientIP(r)
	inside := ""
	if a, err := netip.ParseAddr(ip); err == nil {
		for _, n := range t.Networks {
			if p, err := netip.ParsePrefix(n.CIDR); err == nil && p.Contains(a.Unmap()) {
				inside = n.CIDR
				break
			}
		}
	}
	return map[string]any{"settings": t, "clientIp": ip, "public": public, "clientNetwork": inside}
}

func (s *Server) trustedGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.trustedBody(r, s.trustedNetworks(r.Context())))
}

func (s *Server) trustedSave(w http.ResponseWriter, r *http.Request) {
	var t TrustedNetworks
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := t.Validate(); err != nil {
		if err.Error() == "networks_too_broad" {
			writeErr(w, http.StatusBadRequest, "network_too_broad")
			return
		}
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, TrustedNetworksKey, t); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	setActiveTrusted(t)
	cidrs := make([]string, 0, len(t.Networks))
	for _, n := range t.Networks {
		cidrs = append(cidrs, n.CIDR)
	}
	s.audit(r, "trusted_networks.saved", fmt.Sprintf("enabled=%v roles=%s networks=%s", t.Enabled, strings.Join(t.Roles, ","), strings.Join(cidrs, ",")))
	writeJSON(w, http.StatusOK, s.trustedBody(r, t))
}
