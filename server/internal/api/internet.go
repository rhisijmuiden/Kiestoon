package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/geoip"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/internet"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) internetRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/internet", admin(s.internetGet))
	mux.Handle("PUT /api/internet", admin(s.internetSave))
	mux.Handle("GET /api/firewall/geo", admin(s.geoStatus))
	mux.Handle("POST /api/firewall/geo/update", admin(s.geoUpdate))
}

func (s *Server) internetSettings(ctx context.Context) internet.Settings {
	var st internet.Settings
	_ = store.GetJSON(ctx, s.DB, internet.SettingsKey, &st)
	return st
}

// upstream is the portal address Caddy forwards to.
func (s *Server) upstream() string {
	port := listenPort(s.Cfg.Listen, 8080)
	if port == 0 {
		port = 8080
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func (s *Server) internetGet(w http.ResponseWriter, r *http.Request) {
	set := s.internetSettings(r.Context())
	var st helper.InternetStatus
	err := s.Helper.Call(r.Context(), "internet.status", set, &st)
	resp := map[string]any{"settings": set, "status": st, "helperAvailable": err == nil}
	if set.Domain != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		ips, _ := net.DefaultResolver.LookupHost(ctx, set.Domain)
		cancel()
		if ips == nil {
			ips = []string{}
		}
		resp["dns"] = ips
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) internetSave(w http.ResponseWriter, r *http.Request) {
	var set internet.Settings
	if err := decode(r, &set); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := set.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	old := s.internetSettings(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var st helper.InternetStatus
	if err := s.Helper.Call(ctx, "internet.apply", helper.InternetArgs{Settings: set, Upstream: s.upstream()}, &st); err != nil {
		if strings.Contains(err.Error(), "caddy_missing") {
			writeErr(w, http.StatusConflict, "caddy_missing")
			return
		}
		s.helperErr(w, err)
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, internet.SettingsKey, set); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	// The public address is what identity providers send people back to.
	pub, _ := store.GetSetting(r.Context(), s.DB, "public_url", "")
	switch {
	case set.Enabled && (pub == "" || pub == old.PublicURL()):
		_ = store.SetSetting(r.Context(), s.DB, "public_url", set.PublicURL())
	case !set.Enabled && old.Domain != "" && pub == old.PublicURL():
		_ = store.SetSetting(r.Context(), s.DB, "public_url", "")
	}
	s.audit(r, "internet.saved", strings.TrimSpace(set.Domain+" enabled="+boolString(set.Enabled)))
	s.firewallChanged(r.Context())
	s.Ast.Changed() // the SIP TLS certificate may change
	writeJSON(w, http.StatusOK, map[string]any{"settings": set, "status": st})
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ---- geo-blocking data ------------------------------------------------------

func (s *Server) geoStatus(w http.ResponseWriter, r *http.Request) {
	age := geoip.Age(s.Cfg.DataDir)
	resp := map[string]any{"available": age >= 0, "attribution": geoip.Attribution}
	if age >= 0 {
		resp["updatedAt"] = time.Now().Add(-age).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) geoUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := geoip.Download(ctx, &http.Client{Timeout: 5 * time.Minute}, s.Cfg.DataDir); err != nil {
		s.Log.Warn("geoip download", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "geo_download_failed", "message": err.Error()})
		return
	}
	s.audit(r, "firewall.geo_updated", "")
	s.firewallChanged(r.Context())
	s.geoStatus(w, r)
}

// RunIntegrations does the periodic work of phase 3: the monthly geo-blocking update and copying a
// renewed Let's Encrypt certificate to SIP over TLS.
func (s *Server) RunIntegrations(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		s.integrationsTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) integrationsTick(ctx context.Context) {
	fw := s.firewallConfig(ctx)
	if fw.GeoMode != "" && fw.GeoMode != "off" {
		if age := geoip.Age(s.Cfg.DataDir); age < 0 || age > 30*24*time.Hour {
			dctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			err := geoip.Download(dctx, &http.Client{Timeout: 10 * time.Minute}, s.Cfg.DataDir)
			cancel()
			if err != nil {
				s.Log.Warn("geoip update", "err", err)
			} else {
				s.Log.Info("geo-blocking data updated")
				s.firewallChanged(ctx)
			}
		}
	}
	set := s.internetSettings(ctx)
	if set.Enabled && set.UseForSIP {
		var st helper.InternetStatus
		hctx, cancel := context.WithTimeout(ctx, time.Minute)
		err := s.Helper.Call(hctx, "internet.status", set, &st)
		cancel()
		if err != nil && !errors.Is(err, helper.ErrUnavailable) {
			s.Log.Warn("internet status", "err", err)
		}
		if st.Copied {
			s.Log.Info("public certificate copied to SIP over TLS")
			s.Ast.Changed()
		}
	}
	s.webhookHousekeeping(ctx)
	s.purgeFaxes(ctx)
}
