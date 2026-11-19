package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// Standby is the portal of the standby node of a high-availability pair. It shows the replication
// state and lets an administrator of the primary promote this node by hand. Everything else waits
// until the node is promoted.
type Standby struct {
	Cfg    config.Config
	Log    *slog.Logger
	Helper *helper.Client
	Node   cluster.Node
	Rep    *cluster.Replicator
	limit  *auth.Limiter
}

// NewStandby wires up the standby portal.
func NewStandby(cfg config.Config, log *slog.Logger) *Standby {
	n := cluster.LoadNode(cfg.DataDir)
	sb := &Standby{Cfg: cfg, Log: log, Helper: &helper.Client{Socket: cfg.HelperSocket}, Node: n, limit: auth.NewLimiter(5, 10*time.Minute)}
	base := ""
	if n.Peer != nil {
		base = "http://" + n.Peer.Tunnel + ":" + strconv.Itoa(listenPort(cfg.Listen, 8080))
	}
	sb.Rep = &cluster.Replicator{DataDir: cfg.DataDir, BaseURL: base, Secret: n.Secret, Name: n.Name, Version: version.Version, Log: log,
		OnPromote: func() { sb.promote("switch-over") }, OnLeave: sb.leave}
	return sb
}

// Run sets up the tunnel to the primary and replicates until ctx ends.
func (sb *Standby) Run(ctx context.Context) {
	var tunnels []wg.Interface
	if i, ok := haInterface(sb.Node); ok {
		tunnels = append(tunnels, i)
	}
	for i := 0; i < 30; i++ {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err := sb.Helper.Call(cctx, "wireguard.apply", helper.WireGuardArgs{Interfaces: tunnels}, nil)
		cancel()
		if err == nil {
			break
		}
		sb.Log.Warn("standby: tunnel", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if err := sb.Helper.Call(cctx, "cluster.apply", nil, nil); err != nil {
		sb.Log.Warn("standby: keepalived", "err", err)
	}
	cancel()
	sb.Rep.Run(ctx)
}

func (sb *Standby) promote(reason string) {
	sb.Log.Info("standby: promoting", "reason", reason)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sb.Helper.Call(ctx, "cluster.promote", nil, nil); err != nil {
		sb.Log.Error("standby: promote", "err", err)
	}
}

// leave turns this node back into an unconfigured system when the primary switched HA off.
func (sb *Standby) leave() {
	sb.Log.Info("standby: high availability switched off by the primary")
	cluster.RemoveNode(sb.Cfg.DataDir)
	_ = os.Remove(cluster.ReplicaPath(sb.Cfg.DataDir))
	_ = os.Remove(cluster.ReplicaPath(sb.Cfg.DataDir) + ".json")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_ = sb.Helper.Call(ctx, "wireguard.apply", helper.WireGuardArgs{}, nil)
	_ = sb.Helper.Call(ctx, "cluster.apply", nil, nil)
	_ = sb.Helper.Call(ctx, "portal.restart", nil, nil)
}

// Routes serves the standby API under /api/.
func (sb *Standby) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "standby", "version": version.Version})
	})
	mux.HandleFunc("GET /api/setup/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"setupRequired": false, "tokenRequired": false, "standby": true})
	})
	mux.HandleFunc("GET /api/cluster/standby", sb.status)
	mux.HandleFunc("POST /api/cluster/standby/promote", sb.promoteRequest)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusServiceUnavailable, "standby") })
	return (&Server{}).jsonOnly(mux)
}

func (sb *Standby) status(w http.ResponseWriter, r *http.Request) {
	var st helper.ClusterStatus
	cctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	helperOK := sb.Helper.Call(cctx, "cluster.status", nil, &st) == nil
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]any{"node": publicNode(sb.Node), "role": cluster.LoadRole(sb.Cfg.DataDir), "replication": sb.Rep.Status(),
		"status": st, "helperAvailable": helperOK, "version": version.Version, "hostname": host})
}

// promoteRequest promotes this node by hand. The administrator signs in with an account of the
// primary (checked against the last copy of its database). While the primary still answers, only
// a forced promotion is accepted: two primaries would both run the phone system.
func (sb *Standby) promoteRequest(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if sb.limit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
		Force    bool   `json:"force"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if _, ok := cluster.LoadMeta(cluster.ReplicaPath(sb.Cfg.DataDir)); !ok {
		writeErr(w, http.StatusConflict, "no_replica")
		return
	}
	if err := sb.checkAdmin(r.Context(), req.Username, req.Password, req.Code); err != nil {
		sb.limit.Fail(ip)
		sb.Log.Warn("standby: promote refused", "ip", ip)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if st := sb.Rep.Status(); st.PrimaryReachable && time.Since(st.LastContact) < 15*time.Second && !req.Force {
		writeErr(w, http.StatusConflict, "primary_alive")
		return
	}
	sb.limit.Reset(ip)
	sb.Log.Warn("standby: promoted by hand", "user", req.Username, "ip", ip, "force", req.Force)
	go sb.promote("manual by " + req.Username)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// checkAdmin verifies the credentials on a temporary copy of the replica.
func (sb *Standby) checkAdmin(ctx context.Context, username, password, code string) error {
	dir, err := os.MkdirTemp(cluster.Dir(sb.Cfg.DataDir), "check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	src, err := os.ReadFile(cluster.ReplicaPath(sb.Cfg.DataDir))
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "check.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		return err
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	a := auth.New(db)
	u, err := a.CheckPassword(ctx, username, password)
	if err != nil {
		return err
	}
	if !u.IsAdmin() {
		return auth.ErrInvalidCredentials
	}
	if u.TOTPEnabled {
		return a.VerifySecondFactor(ctx, u, code)
	}
	return nil
}
