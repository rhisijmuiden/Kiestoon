package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// ProxyKey is the settings row with ProxySettings as JSON.
const ProxyKey = "reverse_proxy"

// ProxySettings describe an own reverse proxy (nginx, Traefik, HAProxy, a load balancer) in front of
// the portal. Requests from the trusted addresses may set X-Forwarded-For and X-Forwarded-Proto.
// Caddy of internet mode on this machine (loopback) is always trusted.
type ProxySettings struct {
	Enabled bool `json:"enabled"`
	// TrustedProxies are IP addresses or CIDR ranges of the proxies.
	TrustedProxies []string `json:"trustedProxies"`
	// PublicURL is the address people use, for example https://pbx.example.com. Identity providers,
	// passkeys and mails use it.
	PublicURL string `json:"publicUrl"`
}

// Validate cleans up and checks the settings.
func (p *ProxySettings) Validate() error {
	out := []string{}
	for _, v := range p.TrustedProxies {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		pfx, err := parsePrefix(v)
		if err != nil {
			return errors.New("trustedProxies")
		}
		if pfx.Bits() == 0 {
			// 0.0.0.0/0 would let every client choose its own address.
			return errors.New("trustedProxies")
		}
		out = append(out, pfx.String())
	}
	if len(out) > 32 {
		return errors.New("trustedProxies")
	}
	p.TrustedProxies = out
	p.PublicURL = strings.TrimRight(strings.TrimSpace(p.PublicURL), "/")
	if p.PublicURL != "" {
		u, err := url.Parse(p.PublicURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
			return errors.New("publicUrl")
		}
	}
	if p.Enabled && len(p.TrustedProxies) == 0 {
		return errors.New("trustedProxies")
	}
	return nil
}

func parsePrefix(v string) (netip.Prefix, error) {
	if strings.Contains(v, "/") {
		p, err := netip.ParsePrefix(v)
		return p.Masked(), err
	}
	a, err := netip.ParseAddr(v)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// trusted holds the parsed proxy ranges; clientIP runs on every request, so it reads them lock-free.
var trusted atomic.Pointer[[]netip.Prefix]

// setTrustedProxies replaces the trusted proxy list (nil or empty: only loopback).
func setTrustedProxies(list []string) {
	out := []netip.Prefix{}
	for _, v := range list {
		if p, err := parsePrefix(v); err == nil {
			out = append(out, p)
		}
	}
	trusted.Store(&out)
}

func isTrustedProxy(a netip.Addr) bool {
	a = a.Unmap()
	if a.IsLoopback() {
		return true
	}
	if l := trusted.Load(); l != nil {
		for _, p := range *l {
			if p.Contains(a) {
				return true
			}
		}
	}
	return false
}

func remoteAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	return a.Unmap(), err == nil
}

// forwardedClient walks X-Forwarded-For from the right and returns the first address that is not a
// trusted proxy: everything left of that was written by the client and cannot be believed.
func forwardedClient(r *http.Request) (netip.Addr, bool) {
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	var last netip.Addr
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return last, last.IsValid()
		}
		a = a.Unmap()
		last = a
		if !isTrustedProxy(a) {
			return a, true
		}
	}
	return last, last.IsValid()
}

func (s *Server) proxySettings(ctx context.Context) ProxySettings {
	var p ProxySettings
	_ = store.GetJSON(ctx, s.DB, ProxyKey, &p)
	return p
}

// LoadProxySettings activates the stored trusted proxies; called at start-up.
func (s *Server) LoadProxySettings(ctx context.Context) {
	p := s.proxySettings(ctx)
	if p.Enabled {
		setTrustedProxies(p.TrustedProxies)
	} else {
		setTrustedProxies(nil)
	}
}

func (s *Server) proxyGet(w http.ResponseWriter, r *http.Request) {
	p := s.proxySettings(r.Context())
	if p.TrustedProxies == nil {
		p.TrustedProxies = []string{}
	}
	ra, _ := remoteAddr(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": p,
		// What the portal sees for this request, to check the proxy configuration.
		"remoteAddr": ra.String(), "clientIp": clientIP(r), "forwardedFor": r.Header.Get("X-Forwarded-For"),
		"forwardedProto": r.Header.Get("X-Forwarded-Proto"), "secure": s.secure(r), "host": r.Host,
	})
}

func (s *Server) proxySave(w http.ResponseWriter, r *http.Request) {
	var p ProxySettings
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := p.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	old := s.proxySettings(r.Context())
	if err := store.SetJSON(r.Context(), s.DB, ProxyKey, p); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	pub, _ := store.GetSetting(r.Context(), s.DB, "public_url", "")
	switch {
	case p.PublicURL != "":
		_ = store.SetSetting(r.Context(), s.DB, "public_url", p.PublicURL)
	case old.PublicURL != "" && pub == old.PublicURL:
		_ = store.SetSetting(r.Context(), s.DB, "public_url", "")
	}
	if p.Enabled {
		setTrustedProxies(p.TrustedProxies)
	} else {
		setTrustedProxies(nil)
	}
	s.audit(r, "proxy.saved", strings.Join(p.TrustedProxies, ","))
	writeJSON(w, http.StatusOK, map[string]any{"settings": p})
}
