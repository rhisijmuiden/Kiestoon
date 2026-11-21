package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// High availability on the primary node: pairing the standby, serving it snapshots and files,
// status and planned switch-over.

type haState struct {
	mu       sync.Mutex
	snap     *cluster.Snapshotter
	started  bool
	peer     cluster.Heartbeat
	peerSeen time.Time
	takeover bool
	promote  bool
	leave    bool
	busy     bool
	lastErr  string
}

const clusterPort = 51821

func (s *Server) clusterRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	s.ha.mu.Lock()
	if s.ha.snap == nil {
		s.ha.snap = &cluster.Snapshotter{DataDir: s.Cfg.DataDir, DBPath: s.Cfg.DatabasePath(), Log: s.Log}
	}
	s.ha.mu.Unlock()
	mux.Handle("GET /api/cluster", admin(s.clusterGet))
	mux.Handle("POST /api/cluster/enable", admin(s.clusterEnable))
	mux.Handle("POST /api/cluster/switchover", admin(s.clusterSwitchover))
	mux.Handle("POST /api/cluster/disable", admin(s.clusterDisable))
	mux.HandleFunc("POST /api/cluster/accept", s.clusterAccept)
	mux.HandleFunc("POST /api/setup/cluster-join", s.clusterJoin)
	mux.Handle("/api/cluster/v1/", &cluster.PrimaryHandler{
		DataDir: s.Cfg.DataDir,
		Snap:    s.ha.snap,
		Secret: func() string {
			n := cluster.LoadNode(s.Cfg.DataDir)
			if !n.Enabled || n.Peer == nil {
				return ""
			}
			return n.Secret
		},
		AllowedFrom: func(r *http.Request) bool {
			n := cluster.LoadNode(s.Cfg.DataDir)
			a, ok := remoteAddr(r)
			return ok && n.Peer != nil && a.String() == n.Peer.Tunnel
		},
		OnHeartbeat: s.clusterHeartbeat,
	})
}

func (s *Server) clusterHeartbeat(hb cluster.Heartbeat) cluster.PrimaryInfo {
	s.ha.mu.Lock()
	defer s.ha.mu.Unlock()
	s.ha.peer, s.ha.peerSeen = hb, time.Now()
	n := cluster.LoadNode(s.Cfg.DataDir)
	return cluster.PrimaryInfo{Name: n.Name, Version: version.Version, Snapshot: s.ha.snap.Meta(), Takeover: s.ha.takeover,
		Promote: s.ha.promote, Leave: s.ha.leave}
}

// startSnapshots starts taking database snapshots for the standby (once).
func (s *Server) startSnapshots(ctx context.Context) {
	s.ha.mu.Lock()
	defer s.ha.mu.Unlock()
	if s.ha.started {
		return
	}
	s.ha.started = true
	go s.ha.snap.Run(context.WithoutCancel(ctx), 5*time.Second)
}

func (s *Server) applyClusterOnStart(ctx context.Context) {
	n := cluster.LoadNode(s.Cfg.DataDir)
	if !n.Enabled {
		return
	}
	if n.Peer != nil {
		s.startSnapshots(ctx)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.Helper.Call(cctx, "cluster.apply", nil, nil); err != nil {
		s.Log.Warn("cluster: keepalived", "err", err)
	}
}

type nodeOut struct {
	Enabled   bool          `json:"enabled"`
	Mode      string        `json:"mode"`
	Name      string        `json:"name"`
	Address   string        `json:"address"`
	Interface string        `json:"interface"`
	VIP       string        `json:"vip"`
	Tunnel    string        `json:"tunnel"`
	Peer      *cluster.Peer `json:"peer"`
	JoinOpen  bool          `json:"joinOpen"`
}

func publicNode(n cluster.Node) nodeOut {
	return nodeOut{Enabled: n.Enabled, Mode: n.Mode, Name: n.Name, Address: n.Address, Interface: n.Interface, VIP: n.VIP,
		Tunnel: n.Tunnel, Peer: n.Peer, JoinOpen: n.JoinHash != ""}
}

// defaultAddress is the address of this machine on the network of the default route.
func defaultAddress() string {
	c, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && a.IP.To4() != nil {
		return a.IP.String()
	}
	return ""
}

func (s *Server) clusterGet(w http.ResponseWriter, r *http.Request) {
	n := cluster.LoadNode(s.Cfg.DataDir)
	role := cluster.LoadRole(s.Cfg.DataDir)
	var st helper.ClusterStatus
	cctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	helperOK := s.Helper.Call(cctx, "cluster.status", nil, &st) == nil
	s.ha.mu.Lock()
	peer := map[string]any(nil)
	if n.Peer != nil && !s.ha.peerSeen.IsZero() {
		snap := s.ha.snap.Meta()
		lag := 0.0
		if snap.Newer(s.ha.peer.Replica) && !s.ha.peer.Replica.At.IsZero() {
			lag = snap.At.Sub(s.ha.peer.Replica.At).Seconds()
		}
		peer = map[string]any{"lastSeen": s.ha.peerSeen.UTC(), "version": s.ha.peer.Version, "replica": s.ha.peer.Replica,
			"lastFileSync": s.ha.peer.LastFileSync, "files": s.ha.peer.Files, "lagSeconds": lag,
			"inSync": !snap.Newer(s.ha.peer.Replica), "online": time.Since(s.ha.peerSeen) < 20*time.Second}
	}
	out := map[string]any{"node": publicNode(n), "role": role, "status": st, "helperAvailable": helperOK, "peerState": peer,
		"snapshot": s.ha.snap.Meta(), "busy": s.ha.busy, "lastError": s.ha.lastErr, "version": version.Version,
		"defaultAddress": defaultAddress()}
	s.ha.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func randomRouterID() int {
	n, _ := rand.Int(rand.Reader, big.NewInt(200))
	return int(n.Int64()) + 20
}

// clusterEnable switches high availability on (this node becomes the primary) and returns a join
// code for the second node. Called again it gives a new code.
func (s *Server) clusterEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode      string `json:"mode"`
		VIP       string `json:"vip"`
		Address   string `json:"address"`
		Interface string `json:"interface"`
		URL       string `json:"url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	n := cluster.LoadNode(s.Cfg.DataDir)
	if !n.Enabled {
		priv, pub, err := wg.NewKey()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		first, _ := cluster.RandomTunnel()
		n = cluster.Node{Enabled: true, Name: hostname(), RouterID: randomRouterID(), Priority: 150, Secret: multisite.NewToken(),
			PrivateKey: priv, PublicKey: pub, Tunnel: first, Port: clusterPort}
		if err := cluster.SaveRole(s.Cfg.DataDir, cluster.Role{Role: cluster.RolePrimary, Epoch: 1, Reason: "high availability on"}); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
	}
	n.Mode, n.VIP, n.Address, n.Interface = req.Mode, strings.TrimSpace(req.VIP), strings.TrimSpace(req.Address), strings.TrimSpace(req.Interface)
	if err := n.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	u, fp, err := s.portalURLFor(r, req.URL, n.Address)
	if err != nil {
		fieldErr(w, "url")
		return
	}
	secret := multisite.NewSecret()
	n.JoinHash, n.JoinExpires = multisite.Hash(secret), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339)
	if err := cluster.SaveNode(s.Cfg.DataDir, n); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "cluster.enabled", n.Mode+" "+n.VIP)
	s.clusterChanged(r.Context())
	code := multisite.Code{Kind: multisite.KindCluster, URL: u, Fingerprint: fp, Secret: secret}.Encode()
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "node": publicNode(n)})
}

// portalURLFor is the pairing address for the second node: the node's own address on port 8443,
// unless an address was given.
func (s *Server) portalURLFor(r *http.Request, override, addr string) (string, string, error) {
	if override == "" && addr != "" {
		override = fmt.Sprintf("https://%s:%d", addr, listenPort(s.Cfg.TLSListen, 8443))
	}
	return s.portalURL(r, override)
}

// clusterChanged applies the tunnel, keepalived and the firewall after a change.
func (s *Server) clusterChanged(ctx context.Context) {
	bg := context.WithoutCancel(ctx)
	go func() {
		_ = s.applyTunnels(bg)
		cctx, cancel := context.WithTimeout(bg, 30*time.Second)
		if err := s.Helper.Call(cctx, "cluster.apply", nil, nil); err != nil && !errors.Is(err, helper.ErrUnavailable) {
			s.Log.Warn("cluster: keepalived", "err", err)
		}
		cancel()
		if c := s.firewallConfig(bg); c.Enabled {
			_, _ = s.applyFirewall(bg, c, 0)
		}
	}()
}

// clusterAccept is called by the new standby with the secret of the join code.
func (s *Server) clusterAccept(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req multisite.ClusterJoinRequest
	if err := decode(r, &req); err != nil || !wg.ValidKey(req.PublicKey) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	a, err := netip.ParseAddr(strings.TrimSpace(req.Address))
	if err != nil || !a.Is4() {
		writeErr(w, http.StatusBadRequest, "bad_address")
		return
	}
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	n := cluster.LoadNode(s.Cfg.DataDir)
	exp, _ := time.Parse(time.RFC3339, n.JoinExpires)
	if !n.Enabled || n.JoinHash == "" || subtle.ConstantTimeCompare([]byte(n.JoinHash), []byte(multisite.Hash(req.Secret))) != 1 || time.Now().After(exp) {
		s.loginLimit.Fail(ip)
		store.Audit(r.Context(), s.DB, 0, "cluster.join_failed", req.Name, ip)
		writeErr(w, http.StatusForbidden, "invalid_pairing_code")
		return
	}
	if req.Version != version.Version {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_mismatch", "message": version.Version})
		return
	}
	if a.String() == n.Address {
		writeErr(w, http.StatusBadRequest, "bad_address")
		return
	}
	_, second := tunnelPair(n.Tunnel)
	n.Peer = &cluster.Peer{Name: strings.TrimSpace(req.Name), Address: a.String(), PublicKey: req.PublicKey, Tunnel: second}
	n.JoinHash, n.JoinExpires = "", ""
	if err := cluster.SaveNode(s.Cfg.DataDir, n); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(r.Context(), s.DB, 0, "cluster.joined", n.Peer.Name+" "+n.Peer.Address, ip)
	s.startSnapshots(r.Context())
	s.clusterChanged(r.Context())
	writeJSON(w, http.StatusOK, multisite.ClusterJoinResponse{Secret: n.Secret, PeerName: n.Name, PeerAddress: n.Address,
		PeerPublicKey: n.PublicKey, PeerTunnel: n.Tunnel, Tunnel: second, Port: n.Port, Mode: n.Mode, VIP: n.VIP, RouterID: n.RouterID})
}

// tunnelPair returns the two addresses of the /30 that first belongs to.
func tunnelPair(first string) (string, string) {
	a, err := netip.ParseAddr(first)
	if err != nil || !a.Is4() {
		return first, ""
	}
	b := a.As4()
	base := b[3] &^ 3
	b1, b2 := b, b
	b1[3], b2[3] = base+1, base+2
	return netip.AddrFrom4(b1).String(), netip.AddrFrom4(b2).String()
}

// clusterJoin runs on a freshly installed node: it pairs with the primary and restarts as standby.
// Like the setup wizard, it needs the setup token from the console.
func (s *Server) clusterJoin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Token   string `json:"token"`
		Code    string `json:"code"`
		Address string `json:"address"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	needed, err := s.setupNeeded(r.Context())
	if err != nil || !needed {
		writeErr(w, http.StatusConflict, "setup_done")
		return
	}
	if want, err := os.ReadFile(SetupTokenPath(s.Cfg.DataDir)); err == nil {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(want))), []byte(strings.TrimSpace(req.Token))) != 1 {
			s.loginLimit.Fail(ip)
			writeErr(w, http.StatusForbidden, "invalid_setup_token")
			return
		}
	}
	s.joinAsStandby(w, r, req.Code, req.Address)
}

// joinAsStandby pairs this fresh node with the primary of the code and restarts it as standby.
func (s *Server) joinAsStandby(w http.ResponseWriter, r *http.Request, rawCode, address string) bool {
	ip := clientIP(r)
	code, err := multisite.Decode(rawCode, multisite.KindCluster)
	if err != nil {
		fieldErr(w, "code")
		return false
	}
	addr := strings.TrimSpace(address)
	if addr == "" {
		addr = addressTowards(code.URL)
	}
	if a, err := netip.ParseAddr(addr); err != nil || !a.Is4() {
		fieldErr(w, "address")
		return false
	}
	priv, pub, err := wg.NewKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return false
	}
	var resp multisite.ClusterJoinResponse
	cctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	err = multisite.PostJSON(cctx, multisite.PinnedClient(code.Fingerprint, 20*time.Second), code.URL+"/api/cluster/accept", "",
		multisite.ClusterJoinRequest{Secret: code.Secret, Name: hostname(), Address: addr, PublicKey: pub, Version: version.Version}, &resp)
	if err != nil {
		var ae *multisite.APIError
		if errors.As(err, &ae) && ae.Code != "" {
			writeErr(w, http.StatusBadGateway, ae.Code)
			return false
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unreachable", "message": err.Error()})
		return false
	}
	if !wg.ValidKey(resp.PeerPublicKey) || resp.Secret == "" {
		writeErr(w, http.StatusBadGateway, "remote_invalid")
		return false
	}
	n := cluster.Node{Enabled: true, Mode: resp.Mode, Name: hostname(), Address: addr, VIP: resp.VIP, RouterID: resp.RouterID, Priority: 100,
		Secret: resp.Secret, PrivateKey: priv, PublicKey: pub, Tunnel: resp.Tunnel, Port: resp.Port,
		Peer: &cluster.Peer{Name: resp.PeerName, Address: resp.PeerAddress, PublicKey: resp.PeerPublicKey, Tunnel: resp.PeerTunnel}}
	if err := cluster.SaveNode(s.Cfg.DataDir, n); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return false
	}
	if err := cluster.SaveRole(s.Cfg.DataDir, cluster.Role{Role: cluster.RoleStandby, Epoch: 0, Reason: "joined"}); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return false
	}
	store.Audit(r.Context(), s.DB, 0, "cluster.joined_as_standby", resp.PeerName, ip)
	// The portal restarts as standby.
	_ = s.Helper.Call(r.Context(), "portal.restart", nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "primary": resp.PeerName})
	return true
}

// addressTowards is the local address used to reach the host of u.
func addressTowards(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	host := p.Hostname()
	c, err := net.Dial("udp", net.JoinHostPort(host, "9"))
	if err != nil {
		return defaultAddress()
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && a.IP.To4() != nil {
		return a.IP.String()
	}
	return ""
}

// clusterSwitchover hands over to the standby: a last snapshot, wait until the standby has it,
// then this node steps down.
func (s *Server) clusterSwitchover(w http.ResponseWriter, r *http.Request) {
	n := cluster.LoadNode(s.Cfg.DataDir)
	s.ha.mu.Lock()
	online := time.Since(s.ha.peerSeen) < 20*time.Second
	busy := s.ha.busy
	if n.Enabled && n.Peer != nil && online && !busy {
		s.ha.busy, s.ha.lastErr = true, ""
	}
	s.ha.mu.Unlock()
	switch {
	case !n.Enabled || n.Peer == nil:
		writeErr(w, http.StatusConflict, "no_standby")
		return
	case !online:
		writeErr(w, http.StatusConflict, "standby_offline")
		return
	case busy:
		writeErr(w, http.StatusConflict, "switchover_busy")
		return
	}
	s.audit(r, "cluster.switchover", n.Peer.Name)
	go s.switchover(context.WithoutCancel(r.Context()), n)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) switchover(ctx context.Context, n cluster.Node) {
	fail := func(msg string) {
		s.ha.mu.Lock()
		s.ha.busy, s.ha.takeover, s.ha.promote, s.ha.lastErr = false, false, false, msg
		s.ha.mu.Unlock()
		s.Log.Warn("cluster switch-over failed", "err", msg)
	}
	s.ha.mu.Lock()
	s.ha.takeover = true
	s.ha.mu.Unlock()
	final, err := s.ha.snap.Take(ctx, true)
	if err != nil {
		fail(err.Error())
		return
	}
	caughtUp := func() bool {
		s.ha.mu.Lock()
		defer s.ha.mu.Unlock()
		return !final.Newer(s.ha.peer.Replica)
	}
	deadline := time.Now().Add(90 * time.Second)
	for !caughtUp() {
		if time.Now().After(deadline) {
			fail("standby_not_in_sync")
			return
		}
		time.Sleep(time.Second)
	}
	if n.Mode == cluster.ModeManual {
		// The standby promotes itself when it reads this in its next heartbeat.
		s.ha.mu.Lock()
		s.ha.promote = true
		seen := s.ha.peerSeen
		s.ha.mu.Unlock()
		deadline = time.Now().Add(20 * time.Second)
		for {
			s.ha.mu.Lock()
			again := s.ha.peerSeen.After(seen)
			s.ha.mu.Unlock()
			if again {
				break
			}
			if time.Now().After(deadline) {
				fail("standby_offline")
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err := s.Helper.Call(ctx, "cluster.release", nil, nil); err != nil {
		fail(err.Error())
	}
}

// clusterDisable switches high availability off. The standby hears it in its next heartbeat and
// becomes an unconfigured system again.
func (s *Server) clusterDisable(w http.ResponseWriter, r *http.Request) {
	n := cluster.LoadNode(s.Cfg.DataDir)
	if !n.Enabled {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if n.Peer != nil {
		s.ha.mu.Lock()
		s.ha.leave = true
		seen := s.ha.peerSeen
		s.ha.mu.Unlock()
		// Give the standby a heartbeat to hear it (it asks every 5 seconds).
		for i := 0; i < 14 && time.Since(seen) < 20*time.Second; i++ {
			s.ha.mu.Lock()
			heard := s.ha.peerSeen.After(seen)
			s.ha.mu.Unlock()
			if heard {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	cluster.RemoveNode(s.Cfg.DataDir)
	_ = os.Remove(cluster.SnapshotPath(s.Cfg.DataDir))
	_ = os.Remove(cluster.SnapshotPath(s.Cfg.DataDir) + ".json")
	s.ha.mu.Lock()
	s.ha.leave, s.ha.peerSeen = false, time.Time{}
	s.ha.mu.Unlock()
	s.audit(r, "cluster.disabled", "")
	s.clusterChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
