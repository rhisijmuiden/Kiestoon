package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// Multi-site: the WireGuard hub and the Kiestoon Edge satellites of branch offices.

func (s *Server) siteRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/sites", admin(s.sitesGet))
	mux.Handle("PUT /api/sites/hub", admin(s.hubSave))
	mux.Handle("POST /api/sites/edges", admin(s.edgeCreate))
	mux.Handle("PUT /api/sites/edges/{id}", admin(s.edgeRename))
	mux.Handle("POST /api/sites/edges/{id}/pairing", admin(s.edgeRepair))
	mux.Handle("DELETE /api/sites/edges/{id}", admin(s.edgeDelete))
	// Called by the edges themselves.
	mux.HandleFunc("POST /api/edge/pair", s.edgePair)
	mux.HandleFunc("POST /api/edge/heartbeat", s.edgeHeartbeat)
}

// edgeAddrs are the tunnel addresses of paired edges. Their provisioning proxy forwards phone
// requests with X-Forwarded-For, so they count as trusted proxies.
var edgeAddrs atomic.Pointer[map[netip.Addr]bool]

func isEdgeProxy(a netip.Addr) bool {
	m := edgeAddrs.Load()
	return m != nil && (*m)[a.Unmap()]
}

func (s *Server) refreshEdgeProxies(ctx context.Context) {
	edges, err := s.PBX.EdgeSites(ctx)
	if err != nil {
		return
	}
	m := map[netip.Addr]bool{}
	for _, e := range edges {
		if a, err := netip.ParseAddr(e.TunnelIP); err == nil && e.Paired {
			m[a] = true
		}
	}
	edgeAddrs.Store(&m)
}

// edgeFromTunnel returns the edge a request came through (nil when it did not come through one).
func (s *Server) edgeFromTunnel(r *http.Request) *pbx.EdgeSite {
	a, ok := remoteAddr(r)
	if !ok || !isEdgeProxy(a) {
		return nil
	}
	edges, _ := s.PBX.EdgeSites(r.Context())
	for i := range edges {
		if edges[i].TunnelIP == a.String() {
			return &edges[i]
		}
	}
	return nil
}

// portalURL is the HTTPS address another system uses to pair with this one, and the fingerprint
// of the certificate it will see there (empty for a public certificate).
func (s *Server) portalURL(r *http.Request, override string) (string, string, error) {
	pub, _ := store.GetSetting(r.Context(), s.DB, "public_url", "")
	raw := strings.TrimRight(strings.TrimSpace(override), "/")
	if raw == "" && strings.HasPrefix(pub, "https://") {
		return pub, "", nil
	}
	if raw == "" {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		raw = fmt.Sprintf("https://%s:%d", host, listenPort(s.Cfg.TLSListen, 8443))
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") {
		return "", "", errors.New("url")
	}
	raw = u.Scheme + "://" + u.Host
	// The portal's own certificate is pinned unless the address is the public one (internet mode).
	if pub != "" && raw == strings.TrimRight(pub, "/") {
		return raw, "", nil
	}
	cert, _, _, err := certs.Current(s.Cfg.PBXDir())
	if err != nil {
		return "", "", err
	}
	c, err := certs.Read(cert)
	if err != nil {
		return "", "", err
	}
	return raw, multisite.Fingerprint(c), nil
}

// ---- hub --------------------------------------------------------------------

func (s *Server) hub(ctx context.Context) pbx.HubSettings {
	h, _ := pbx.LoadHub(ctx, s.DB)
	return h
}

func (s *Server) defaultHubHost(ctx context.Context, r *http.Request) string {
	if inet := s.internetSettings(ctx); inet.Enabled && inet.Domain != "" {
		return inet.Domain
	}
	st, _ := pbx.LoadSettings(ctx, s.DB)
	if st.ExternalAddress != "" {
		return st.ExternalAddress
	}
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h
	}
	return r.Host
}

func (s *Server) wgStatus(ctx context.Context) (helper.WireGuardStatus, bool) {
	var st helper.WireGuardStatus
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.Helper.Call(cctx, "wireguard.status", nil, &st)
	return st, err == nil
}

func peerConnected(st helper.WireGuardStatus, key string) (bool, string) {
	for _, p := range st.Peers {
		if p.PublicKey == key {
			return p.Connected(time.Now()), p.Endpoint
		}
	}
	return false, ""
}

func (s *Server) sitesGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h := s.hub(ctx)
	if h.Host == "" {
		h.Host = s.defaultHubHost(ctx, r)
	}
	h.PrivateKey = ""
	edges, err := s.PBX.EdgeSites(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	st, ok := s.wgStatus(ctx)
	type edgeOut struct {
		pbx.EdgeSite
		Connected bool   `json:"connected"`
		Remote    string `json:"remote"`
	}
	out := []edgeOut{}
	for _, e := range edges {
		c, ep := peerConnected(st, e.PublicKey)
		out = append(out, edgeOut{EdgeSite: e, Connected: c && e.PublicKey != "", Remote: ep})
	}
	url, fp, _ := s.portalURL(r, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"hub": h, "edges": out, "helperAvailable": ok, "wireguardInstalled": st.Installed, "tunnelsUp": st.Up,
		"tunnelErrors": s.tunnelErrors(), "portalUrl": url, "pinned": fp != "",
	})
}

func (s *Server) hubSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled    bool   `json:"enabled"`
		Host       string `json:"host"`
		ListenPort int    `json:"listenPort"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ctx := r.Context()
	h := s.hub(ctx)
	req.Host = strings.TrimSpace(req.Host)
	if req.ListenPort == 0 {
		req.ListenPort = 51820
	}
	if req.ListenPort < 1024 || req.ListenPort > 65535 {
		fieldErr(w, "listenPort")
		return
	}
	if req.Enabled && (req.Host == "" || !wg.ValidEndpoint(fmt.Sprintf("%s:%d", hostForEndpoint(req.Host), req.ListenPort))) {
		fieldErr(w, "host")
		return
	}
	if h.PrivateKey == "" {
		priv, pub, err := wg.NewKey()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		h.PrivateKey, h.PublicKey = priv, pub
	}
	if h.Subnet == "" {
		h.Subnet = wg.RandomSubnet().String()
	}
	h.Enabled, h.Host, h.ListenPort = req.Enabled, req.Host, req.ListenPort
	if err := store.SetJSON(ctx, s.DB, pbx.HubKey, h); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "sites.hub", fmt.Sprintf("enabled=%v %s", h.Enabled, h.Endpoint()))
	s.multisiteChanged(ctx, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func hostForEndpoint(h string) string {
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		return "[" + h + "]"
	}
	return h
}

// hubAddress is the hub's own tunnel address (.1 of the subnet).
func hubAddress(h pbx.HubSettings) (netip.Addr, netip.Prefix, error) {
	pfx, err := netip.ParsePrefix(h.Subnet)
	if err != nil {
		return netip.Addr{}, pfx, err
	}
	a, err := wg.HostAddr(pfx, 1)
	return a, pfx, err
}

// usedTunnelIPs are the hub addresses already handed out.
func (s *Server) usedTunnelIPs(ctx context.Context) []string {
	used := []string{}
	edges, _ := s.PBX.EdgeSites(ctx)
	for _, e := range edges {
		if e.TunnelIP != "" {
			used = append(used, e.TunnelIP)
		}
	}
	peers, _ := s.PBX.FedPeers(ctx)
	for _, p := range peers {
		if p.Role == "hosted" && p.TunnelIP != "" {
			used = append(used, p.TunnelIP)
		}
	}
	return used
}

// ---- tunnels ----------------------------------------------------------------

// tunnels is the full set of WireGuard interfaces this system needs.
func (s *Server) tunnels(ctx context.Context) []wg.Interface {
	out := []wg.Interface{}
	h := s.hub(ctx)
	if h.Enabled && h.PrivateKey != "" {
		if addr, pfx, err := hubAddress(h); err == nil {
			i := wg.Interface{Name: "kt0", PrivateKey: h.PrivateKey, Address: netip.PrefixFrom(addr, pfx.Bits()).String(), ListenPort: h.ListenPort}
			edges, _ := s.PBX.EdgeSites(ctx)
			for _, e := range edges {
				if e.PublicKey != "" && e.TunnelIP != "" {
					i.Peers = append(i.Peers, wg.Peer{PublicKey: e.PublicKey, AllowedIPs: []string{e.TunnelIP + "/32"}})
				}
			}
			peers, _ := s.PBX.FedPeers(ctx)
			for _, p := range peers {
				if p.Role == "hosted" && p.State == "active" && p.PublicKey != "" {
					i.Peers = append(i.Peers, wg.Peer{PublicKey: p.PublicKey, AllowedIPs: []string{p.TunnelIP + "/32"}})
				}
			}
			out = append(out, i)
		}
	}
	peers, _ := s.PBX.FedPeers(ctx)
	for _, p := range peers {
		if p.Role != "joined" || p.State != "active" || p.PrivateKey == "" {
			continue
		}
		out = append(out, wg.Interface{Name: p.Interface(), PrivateKey: p.PrivateKey, Address: p.LocalIP + "/32",
			Peers: []wg.Peer{{PublicKey: p.PublicKey, AllowedIPs: []string{p.TunnelIP + "/32"}, Endpoint: p.Endpoint, Keepalive: 25}}})
	}
	if i, ok := haInterface(cluster.LoadNode(s.Cfg.DataDir)); ok {
		out = append(out, i)
	}
	return out
}

// haInterface is the tunnel between the two nodes of a high-availability pair.
func haInterface(n cluster.Node) (wg.Interface, bool) {
	if !n.Enabled || n.Peer == nil || n.PrivateKey == "" || n.Peer.PublicKey == "" {
		return wg.Interface{}, false
	}
	return wg.Interface{Name: cluster.Interface, PrivateKey: n.PrivateKey, Address: n.Tunnel + "/32", ListenPort: n.Port,
		Peers: []wg.Peer{{PublicKey: n.Peer.PublicKey, AllowedIPs: []string{n.Peer.Tunnel + "/32"},
			Endpoint: fmt.Sprintf("%s:%d", n.Peer.Address, n.Port), Keepalive: 25}}}, true
}

var tunnelErrs struct {
	sync.Mutex
	list []string
}

func (s *Server) tunnelErrors() []string {
	tunnelErrs.Lock()
	defer tunnelErrs.Unlock()
	return append([]string{}, tunnelErrs.list...)
}

// applyTunnels sends the tunnels to the root helper.
func (s *Server) applyTunnels(ctx context.Context) error {
	var st helper.WireGuardStatus
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := s.Helper.Call(cctx, "wireguard.apply", helper.WireGuardArgs{Interfaces: s.tunnels(ctx)}, &st)
	tunnelErrs.Lock()
	tunnelErrs.list = st.Errors
	if err != nil && !errors.Is(err, helper.ErrUnavailable) {
		tunnelErrs.list = append(tunnelErrs.list, err.Error())
	}
	tunnelErrs.Unlock()
	return err
}

// multisiteChanged applies tunnels, the firewall (for the WireGuard port) and the Asterisk configuration.
func (s *Server) multisiteChanged(ctx context.Context, firewall bool) {
	s.refreshEdgeProxies(ctx)
	s.Ast.Changed()
	bg := context.WithoutCancel(ctx)
	go func() {
		if err := s.applyTunnels(bg); err != nil && !errors.Is(err, helper.ErrUnavailable) {
			s.Log.Warn("tunnels", "err", err)
		}
		if firewall {
			if c := s.firewallConfig(bg); c.Enabled {
				if _, err := s.applyFirewall(bg, c, 0); err != nil {
					s.Log.Warn("firewall after tunnel change", "err", err)
				}
			}
		}
	}()
}

// ---- edges ------------------------------------------------------------------

func (s *Server) edgeCode(r *http.Request, secret, override string) (string, error) {
	u, fp, err := s.portalURL(r, override)
	if err != nil {
		return "", err
	}
	return multisite.Code{Kind: multisite.KindEdge, URL: u, Fingerprint: fp, Secret: secret}.Encode(), nil
}

func (s *Server) edgeCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !s.hub(r.Context()).Enabled {
		writeErr(w, http.StatusConflict, "hub_disabled")
		return
	}
	e, secret, err := s.PBX.CreateEdgeSite(r.Context(), req.Name)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	code, err := s.edgeCode(r, secret, req.URL)
	if err != nil {
		fieldErr(w, "url")
		return
	}
	s.audit(r, "sites.edge.created", e.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"edge": e, "code": code})
}

func (s *Server) edgeRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.PBX.RenameEdgeSite(r.Context(), pathID(r), req.Name); err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) edgeRepair(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	_ = decode(r, &req)
	secret, err := s.PBX.RenewEdgePairing(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	code, err := s.edgeCode(r, secret, req.URL)
	if err != nil {
		fieldErr(w, "url")
		return
	}
	s.audit(r, "sites.edge.pairing", fmt.Sprint(pathID(r)))
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (s *Server) edgeDelete(w http.ResponseWriter, r *http.Request) {
	e, err := s.PBX.EdgeSite(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if err := s.PBX.DeleteEdgeSite(r.Context(), e.ID); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "sites.edge.deleted", e.Name)
	s.multisiteChanged(r.Context(), false)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// edgePair is called by a new edge with the secret of its pairing code.
func (s *Server) edgePair(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req multisite.EdgePairRequest
	if err := decode(r, &req); err != nil || !wg.ValidKey(req.PublicKey) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ctx := r.Context()
	h := s.hub(ctx)
	if !h.Enabled {
		writeErr(w, http.StatusConflict, "hub_disabled")
		return
	}
	addr, pfx, err := hubAddress(h)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	free, err := wg.NextFree(pfx, s.usedTunnelIPs(ctx))
	if err != nil {
		writeErr(w, http.StatusConflict, "subnet_full")
		return
	}
	lanAddr, lanNet := cleanAddr(req.LANAddress), cleanNet(req.LANNet)
	e, token, err := s.PBX.PairEdge(ctx, req.Secret, req.PublicKey, free.String(), req.Hostname, req.Version, lanAddr, lanNet)
	if errors.Is(err, pbx.ErrPairing) {
		s.loginLimit.Fail(ip)
		store.Audit(ctx, s.DB, 0, "sites.edge.pair_failed", "", ip)
		writeErr(w, http.StatusForbidden, "invalid_pairing_code")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(ctx, s.DB, 0, "sites.edge.paired", e.Name+" "+e.Hostname, ip)
	s.multisiteChanged(ctx, false)
	st, _ := pbx.LoadSettings(ctx, s.DB)
	writeJSON(w, http.StatusOK, multisite.EdgePairResponse{
		SiteID: e.ID, Name: e.Name, Token: token, Address: free.String() + "/32", HubAddress: addr.String(),
		HubPublicKey: h.PublicKey, Endpoint: h.Endpoint(), SIPPort: st.SIPPort, PortalPort: listenPort(s.Cfg.Listen, 8080),
	})
}

func cleanAddr(v string) string {
	if a, err := netip.ParseAddr(strings.TrimSpace(v)); err == nil {
		return a.String()
	}
	return ""
}

func cleanNet(v string) string {
	if p, err := netip.ParsePrefix(strings.TrimSpace(v)); err == nil {
		return p.Masked().String()
	}
	return ""
}

// edgeHeartbeat comes through the tunnel every 30 seconds.
func (s *Server) edgeHeartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, err := s.PBX.EdgeByToken(ctx, multisite.BearerToken(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	if a, ok := remoteAddr(r); !ok || a.String() != e.TunnelIP {
		writeErr(w, http.StatusForbidden, "not_through_tunnel")
		return
	}
	var hb multisite.EdgeHeartbeat
	if err := decode(r, &hb); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	hb.LANAddress, hb.LANNet = cleanAddr(hb.LANAddress), cleanNet(hb.LANNet)
	_ = s.PBX.EdgeHeartbeat(ctx, e.ID, hb)
	st, _ := pbx.LoadSettings(ctx, s.DB)
	writeJSON(w, http.StatusOK, multisite.EdgeConfig{Name: e.Name, SIPPort: st.SIPPort, RTPStart: 30000, RTPEnd: 39999})
}

// RunMultisite applies the tunnels once the helper answers and keeps federated systems in sync.
func (s *Server) RunMultisite(ctx context.Context) {
	s.refreshEdgeProxies(ctx)
	for i := 0; i < 30; i++ {
		err := s.applyTunnels(ctx)
		if err == nil || !errors.Is(err, helper.ErrUnavailable) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	s.applyClusterOnStart(ctx)
	s.runFederation(ctx)
}

// hostname of this machine, for pairing.
func hostname() string {
	h, _ := os.Hostname()
	return h
}
