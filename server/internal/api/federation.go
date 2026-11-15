package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// Federation: two or more Kiestoon systems linked through WireGuard. Each side dials the other's
// numbers with a prefix, sees their BLF, can put their agents in its queues and route calls to
// them (follow the sun). The systems exchange their directory and BLF states over the tunnel.

type fedState struct {
	mu sync.Mutex
	// presence holds the BLF states received per link.
	presence map[int64]map[string]string
	// outbox holds local BLF changes waiting to be pushed.
	outbox map[string]string
	resync bool
	kick   chan struct{}
}

func (f *fedState) init() {
	if f.presence == nil {
		f.presence = map[int64]map[string]string{}
		f.outbox = map[string]string{}
		f.kick = make(chan struct{}, 1)
	}
}

func (s *Server) federationRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	s.fed.mu.Lock()
	s.fed.init()
	s.fed.mu.Unlock()
	mux.Handle("GET /api/federation", admin(s.fedGet))
	mux.Handle("POST /api/federation/invites", admin(s.fedInvite))
	mux.Handle("POST /api/federation/join", admin(s.fedJoin))
	mux.Handle("PUT /api/federation/peers/{id}", admin(s.fedSave))
	mux.Handle("POST /api/federation/peers/{id}/sync", admin(s.fedSyncOne))
	mux.Handle("DELETE /api/federation/peers/{id}", admin(s.fedDelete))
	mux.Handle("GET /api/pbx/sun-routes", admin(s.sunList))
	mux.Handle("POST /api/pbx/sun-routes", admin(s.sunSave))
	mux.Handle("PUT /api/pbx/sun-routes/{id}", admin(s.sunSave))
	mux.Handle("DELETE /api/pbx/sun-routes/{id}", admin(s.sunDelete))
	// Called by the other system.
	mux.HandleFunc("POST /api/federation/accept", s.fedAccept)
	mux.HandleFunc("GET /api/fed/v1/directory", s.fedPeerAPI(s.fedDirectory))
	mux.HandleFunc("POST /api/fed/v1/presence", s.fedPeerAPI(s.fedPresenceIn))
	mux.HandleFunc("POST /api/fed/v1/unlink", s.fedPeerAPI(s.fedUnlink))
}

func peerURL(p *pbx.FedPeer, path string) string {
	port := p.PortalPort
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("http://%s:%d%s", p.TunnelIP, port, path)
}

var fedClient = &http.Client{Timeout: 10 * time.Second}

// ---- admin --------------------------------------------------------------------

func (s *Server) fedGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	peers, err := s.PBX.FedPeers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	st, ok := s.wgStatus(ctx)
	type peerOut struct {
		pbx.FedPeer
		Connected bool              `json:"connected"`
		States    map[string]string `json:"states"`
	}
	out := []peerOut{}
	s.fed.mu.Lock()
	for _, p := range peers {
		c, _ := peerConnected(st, p.PublicKey)
		states := map[string]string{}
		for k, v := range s.fed.presence[p.ID] {
			states[k] = v
		}
		out = append(out, peerOut{FedPeer: p, Connected: c && p.State == "active", States: states})
	}
	s.fed.mu.Unlock()
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	h := s.hub(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"name": org, "hubEnabled": h.Enabled, "peers": out, "helperAvailable": ok})
}

func (s *Server) fedInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
		URL    string `json:"url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !s.hub(r.Context()).Enabled {
		writeErr(w, http.StatusConflict, "hub_disabled")
		return
	}
	u, fp, err := s.portalURL(r, req.URL)
	if err != nil {
		fieldErr(w, "url")
		return
	}
	p, secret, err := s.PBX.CreateInvite(r.Context(), req.Name, req.Prefix)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "federation.invite", p.Name)
	code := multisite.Code{Kind: multisite.KindFederation, URL: u, Fingerprint: fp, Secret: secret}.Encode()
	writeJSON(w, http.StatusCreated, map[string]any{"peer": p, "code": code})
}

// fedJoin links this system to the hub of another Kiestoon with its invitation code.
func (s *Server) fedJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ctx := r.Context()
	code, err := multisite.Decode(req.Code, multisite.KindFederation)
	if err != nil {
		fieldErr(w, "code")
		return
	}
	if err := s.PBX.ValidatePrefix(ctx, req.Prefix, 0); err != nil {
		s.saveErr(w, err)
		return
	}
	priv, pub, err := wg.NewKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	tokenIn := multisite.NewToken()
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	st, _ := pbx.LoadSettings(ctx, s.DB)
	h := s.hub(ctx)
	in := multisite.FedAcceptRequest{Secret: code.Secret, Name: org, PublicKey: pub, Token: tokenIn, SIPPort: st.SIPPort,
		PortalPort: listenPort(s.Cfg.Listen, 8080), Version: version.Version}
	if h.Enabled {
		in.Subnet = h.Subnet
	}
	var resp multisite.FedAcceptResponse
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := multisite.PostJSON(cctx, multisite.PinnedClient(code.Fingerprint, 20*time.Second), code.URL+"/api/federation/accept", "", in, &resp); err != nil {
		var ae *multisite.APIError
		if errors.As(err, &ae) && ae.Code != "" {
			writeErr(w, http.StatusBadGateway, ae.Code)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unreachable", "message": err.Error()})
		return
	}
	local, err := netip.ParsePrefix(resp.Address)
	if err != nil || !wg.ValidKey(resp.HubPublicKey) || !wg.ValidEndpoint(resp.Endpoint) {
		writeErr(w, http.StatusBadGateway, "remote_invalid")
		return
	}
	hubIP, err := netip.ParseAddr(resp.HubAddress)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "remote_invalid")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = resp.Name
	}
	p, err := s.PBX.AddJoinedPeer(ctx, pbx.JoinedPeer{Name: name, Prefix: req.Prefix, PublicKey: resp.HubPublicKey, TunnelIP: hubIP.String(),
		LocalIP: local.Addr().String(), PrivateKey: priv, Endpoint: resp.Endpoint, TokenOut: resp.Token, TokenIn: tokenIn,
		Version: resp.Version, SIPPort: resp.SIPPort, PortalPort: resp.PortalPort})
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "federation.joined", p.Name+" "+code.URL)
	s.multisiteChanged(ctx, true)
	s.kickFederation()
	writeJSON(w, http.StatusCreated, map[string]any{"peer": p})
}

func (s *Server) fedSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Prefix     string `json:"prefix"`
		DirectDial bool   `json:"directDial"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.PBX.SaveFedPeer(r.Context(), pathID(r), req.Name, req.Prefix, req.DirectDial); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "federation.saved", req.Name+" "+req.Prefix)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) fedSyncOne(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.FedPeer(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if err := s.syncPeer(r.Context(), p); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unreachable", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) fedDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.PBX.FedPeer(ctx, pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if p.State == "active" && p.TokenOut != "" {
		// Tell the other side, so its configuration does not keep a dead link (best effort).
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = multisite.PostJSON(cctx, fedClient, peerURL(p, "/api/fed/v1/unlink"), p.TokenOut, map[string]bool{"unlink": true}, nil)
		cancel()
	}
	if err := s.PBX.DeleteFedPeer(ctx, p.ID); err != nil {
		s.saveErr(w, err)
		return
	}
	s.fed.mu.Lock()
	delete(s.fed.presence, p.ID)
	s.fed.mu.Unlock()
	s.audit(r, "federation.deleted", p.Name)
	s.multisiteChanged(ctx, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- called by the other system --------------------------------------------------

func (s *Server) fedAccept(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req multisite.FedAcceptRequest
	if err := decode(r, &req); err != nil || !wg.ValidKey(req.PublicKey) || !strings.HasPrefix(req.Token, "kts_") || len(req.Token) > 100 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.SIPPort < 1 || req.SIPPort > 65535 {
		req.SIPPort = 5060
	}
	if req.PortalPort < 1 || req.PortalPort > 65535 {
		req.PortalPort = 8080
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
	if other, err := netip.ParsePrefix(req.Subnet); err == nil && other.Overlaps(pfx) {
		writeErr(w, http.StatusConflict, "subnet_conflict")
		return
	}
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	free, err := wg.NextFree(pfx, s.usedTunnelIPs(ctx))
	if err != nil {
		writeErr(w, http.StatusConflict, "subnet_full")
		return
	}
	p, tokenIn, err := s.PBX.AcceptInvite(ctx, req.Secret, req, free.String())
	if errors.Is(err, pbx.ErrPairing) {
		s.loginLimit.Fail(ip)
		store.Audit(ctx, s.DB, 0, "federation.accept_failed", "", ip)
		writeErr(w, http.StatusForbidden, "invalid_pairing_code")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(ctx, s.DB, 0, "federation.accepted", p.Name+" "+req.Name, ip)
	s.multisiteChanged(ctx, true)
	s.kickFederation()
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	st, _ := pbx.LoadSettings(ctx, s.DB)
	writeJSON(w, http.StatusOK, multisite.FedAcceptResponse{Name: org, Token: tokenIn, Address: free.String() + "/32", HubAddress: addr.String(),
		HubPublicKey: h.PublicKey, Endpoint: h.Endpoint(), SIPPort: st.SIPPort, PortalPort: listenPort(s.Cfg.Listen, 8080), Version: version.Version})
}

type peerHandler func(w http.ResponseWriter, r *http.Request, p *pbx.FedPeer)

// fedPeerAPI checks the token of a linked system and that the request came through its tunnel.
func (s *Server) fedPeerAPI(h peerHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.PBX.FedPeerByToken(r.Context(), multisite.BearerToken(r))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		if a, ok := remoteAddr(r); !ok || a.String() != p.TunnelIP {
			writeErr(w, http.StatusForbidden, "not_through_tunnel")
			return
		}
		s.PBX.TouchPeer(r.Context(), p.ID)
		h(w, r, p)
	}
}

// localDirectory is what this system publishes to linked systems.
func (s *Server) localDirectory(ctx context.Context) multisite.Directory {
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	st, _ := pbx.LoadSettings(ctx, s.DB)
	d := multisite.Directory{Name: org, Timezone: st.Timezone, Version: version.Version, Entries: []multisite.DirectoryEntry{}}
	if exts, err := s.PBX.Extensions(ctx); err == nil {
		for _, e := range exts {
			if e.Enabled {
				d.Entries = append(d.Entries, multisite.DirectoryEntry{Number: e.Number, Name: e.Name, Kind: "extension"})
			}
		}
	}
	if qs, err := s.PBX.Queues(ctx); err == nil {
		for _, q := range qs {
			if q.Enabled {
				d.Entries = append(d.Entries, multisite.DirectoryEntry{Number: q.Number, Name: q.Name, Kind: "queue"})
			}
		}
	}
	if gs, err := s.PBX.RingGroups(ctx); err == nil {
		for _, g := range gs {
			if g.Enabled {
				d.Entries = append(d.Entries, multisite.DirectoryEntry{Number: g.Number, Name: g.Name, Kind: "ringgroup"})
			}
		}
	}
	if cs, err := s.PBX.Conferences(ctx); err == nil {
		for _, c := range cs {
			if c.Enabled {
				d.Entries = append(d.Entries, multisite.DirectoryEntry{Number: c.Number, Name: c.Name, Kind: "conference"})
			}
		}
	}
	return d
}

func (s *Server) fedDirectory(w http.ResponseWriter, r *http.Request, _ *pbx.FedPeer) {
	writeJSON(w, http.StatusOK, s.localDirectory(r.Context()))
}

func (s *Server) fedPresenceIn(w http.ResponseWriter, r *http.Request, p *pbx.FedPeer) {
	var in multisite.Presence
	if err := decode(r, &in); err != nil || len(in.States) > 5000 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.applyPresence(r.Context(), p, in)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// applyPresence mirrors the BLF of another system in custom device states.
func (s *Server) applyPresence(ctx context.Context, p *pbx.FedPeer, in multisite.Presence) {
	known := map[string]bool{}
	for _, e := range p.Directory.Entries {
		known[e.Number] = true
	}
	s.fed.mu.Lock()
	cur := s.fed.presence[p.ID]
	if cur == nil || in.Full {
		if cur == nil {
			cur = map[string]string{}
		}
		s.fed.presence[p.ID] = cur
	}
	changed := map[string]string{}
	for num, st := range in.States {
		if !known[num] {
			continue
		}
		if cur[num] != st {
			changed[num] = st
		}
		cur[num] = st
	}
	s.fed.mu.Unlock()
	if !s.AMI.Connected() {
		return
	}
	for num, st := range changed {
		s.AMI.SetDeviceState(ctx, p.DeviceState(num), asterisk.FedDeviceState(st))
	}
}

func (s *Server) fedUnlink(w http.ResponseWriter, r *http.Request, p *pbx.FedPeer) {
	if err := s.PBX.DeleteFedPeer(r.Context(), p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	store.Audit(r.Context(), s.DB, 0, "federation.unlinked", p.Name, clientIP(r))
	s.multisiteChanged(r.Context(), true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- sync ----------------------------------------------------------------------

// fedLocalState queues a BLF change of a local extension for the linked systems.
func (s *Server) fedLocalState(exten, status string) {
	s.fed.mu.Lock()
	if s.fed.outbox == nil {
		s.fed.mu.Unlock()
		return
	}
	if exten == "" {
		s.fed.resync = true
	} else {
		s.fed.outbox[exten] = status
	}
	k := s.fed.kick
	s.fed.mu.Unlock()
	select {
	case k <- struct{}{}:
	default:
	}
}

// kickFederation syncs every link soon (after pairing).
func (s *Server) kickFederation() { s.fedLocalState("", "") }

// syncPeer fetches the directory of a linked system and sends it all local BLF states.
func (s *Server) syncPeer(ctx context.Context, p *pbx.FedPeer) error {
	if p.State != "active" || p.TokenOut == "" {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var d multisite.Directory
	if err := multisite.GetJSON(cctx, fedClient, peerURL(p, "/api/fed/v1/directory"), p.TokenOut, &d); err != nil {
		return err
	}
	if len(d.Entries) > 5000 {
		d.Entries = d.Entries[:5000]
	}
	clean := d.Entries[:0]
	for _, e := range d.Entries {
		e.Name = strings.TrimSpace(e.Name)
		if len(e.Name) > 60 {
			e.Name = e.Name[:60]
		}
		if _, err := strconv.ParseUint(e.Number, 10, 64); err == nil && len(e.Number) <= 10 {
			clean = append(clean, e)
		}
	}
	d.Entries = clean
	changed, err := s.PBX.SetPeerDirectory(ctx, p.ID, d)
	if err != nil {
		return err
	}
	if changed {
		s.Ast.Changed()
	}
	return s.pushPresence(ctx, p, multisite.Presence{Full: true, States: s.States.Extensions(ctx)})
}

func (s *Server) pushPresence(ctx context.Context, p *pbx.FedPeer, pr multisite.Presence) error {
	if len(pr.States) == 0 && !pr.Full {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return multisite.PostJSON(cctx, fedClient, peerURL(p, "/api/fed/v1/presence"), p.TokenOut, pr, nil)
}

// runFederation keeps linked systems in sync: BLF changes right away, directories every minute.
func (s *Server) runFederation(ctx context.Context) {
	s.fed.mu.Lock()
	s.fed.init()
	kick := s.fed.kick
	s.fed.mu.Unlock()
	full := time.NewTicker(60 * time.Second)
	defer full.Stop()
	syncAll := func() {
		peers, _ := s.PBX.FedPeers(ctx)
		for i := range peers {
			if err := s.syncPeer(ctx, &peers[i]); err != nil && ctx.Err() == nil {
				s.Log.Debug("federation sync", "peer", peers[i].Name, "err", err)
			}
		}
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}
	syncAll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-full.C:
			syncAll()
		case <-kick:
			select {
			case <-time.After(300 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			s.fed.mu.Lock()
			batch, resync := s.fed.outbox, s.fed.resync
			s.fed.outbox, s.fed.resync = map[string]string{}, false
			s.fed.mu.Unlock()
			if resync {
				syncAll()
				continue
			}
			peers, _ := s.PBX.FedPeers(ctx)
			for i := range peers {
				if peers[i].State == "active" {
					_ = s.pushPresence(ctx, &peers[i], multisite.Presence{States: batch})
				}
			}
		}
	}
}

// ---- follow the sun ----------------------------------------------------------------

func (s *Server) sunList(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.SunRoutes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) sunSave(w http.ResponseWriter, r *http.Request) {
	var sr pbx.SunRoute
	if err := decode(r, &sr); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if id := pathID(r); id != 0 {
		sr.ID = id
	} else {
		sr.ID = 0
	}
	if err := s.PBX.SaveSunRoute(r.Context(), &sr); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "sunroute.saved", sr.Name)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, sr)
}

func (s *Server) sunDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteSunRoute(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "sunroute.deleted", strconv.FormatInt(pathID(r), 10))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
