package api

import (
	"context"
	"crypto/rsa"
	"net/http"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Microsoft Teams Direct Routing: settings, Teams users, status and the PowerShell script.

func (s *Server) teamsRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/teams", admin(s.teamsGet))
	mux.Handle("PUT /api/teams", admin(s.teamsSave))
	mux.Handle("GET /api/teams/script", admin(s.teamsScript))
}

type teamsCert struct {
	Public   bool      `json:"public"`
	Subject  string    `json:"subject"`
	Issuer   string    `json:"issuer"`
	NotAfter time.Time `json:"notAfter"`
	DNSNames []string  `json:"dnsNames"`
	// RSA: Microsoft only accepts RSA certificates.
	RSA bool `json:"rsa"`
	// Covers: the certificate is valid for the SBC's FQDN.
	Covers bool `json:"covers"`
}

type teamsHub struct {
	Host   string  `json:"host"`
	Status string  `json:"status"`
	RTTMs  float64 `json:"rttMs"`
}

type teamsStatus struct {
	// Active: the Teams transport and endpoint are in the Asterisk configuration.
	Active        bool       `json:"active"`
	Certificate   *teamsCert `json:"certificate"`
	InternetOn    bool       `json:"internetOn"`
	UseForSIP     bool       `json:"useForSip"`
	Domain        string     `json:"domain"`
	FirewallOn    bool       `json:"firewallOn"`
	Hubs          []teamsHub `json:"hubs"`
	ActiveCalls   int        `json:"activeCalls"`
	RestartNeeded bool       `json:"restartNeeded"`
	Connected     bool       `json:"connected"`
	Nets          []string   `json:"nets"`
}

func (s *Server) teamsStatus(ctx context.Context, t pbx.TeamsSettings) teamsStatus {
	inet := s.internetSettings(ctx)
	st := teamsStatus{InternetOn: inet.Enabled, UseForSIP: inet.UseForSIP, Domain: inet.Domain, Hubs: []teamsHub{}, Nets: pbx.TeamsSignalingNets}
	st.FirewallOn = s.firewallConfig(ctx).Enabled
	if cp, _, public, err := certs.Current(s.Cfg.PBXDir()); err == nil && public {
		if c, err := certs.Read(cp); err == nil {
			info := certs.Describe(c)
			_, isRSA := c.PublicKey.(*rsa.PublicKey)
			tc := &teamsCert{Public: true, Subject: info.Subject, Issuer: info.Issuer, NotAfter: info.NotAfter, DNSNames: info.DNSNames, RSA: isRSA}
			tc.Covers = t.FQDN != "" && c.VerifyHostname(t.FQDN) == nil
			st.Certificate = tc
		}
	}
	st.Active = t.Enabled && t.FQDN != "" && st.Certificate != nil
	l := s.Live.Get(ctx)
	st.Connected = l.Connected
	for _, h := range pbx.TeamsHubs {
		hub := teamsHub{Host: h, Status: "unknown"}
		for _, c := range l.Contacts {
			if c.Endpoint == asterisk.TeamsEndpoint && strings.Contains(c.URI, "@"+h+":") || c.Endpoint == asterisk.TeamsEndpoint && strings.Contains(c.URI, ":"+h+":") {
				hub.Status, hub.RTTMs = strings.ToLower(c.Status), c.RTTMs
			}
		}
		if !st.Active {
			hub.Status = "off"
		}
		st.Hubs = append(st.Hubs, hub)
	}
	for _, c := range l.Calls {
		for _, leg := range c.Legs {
			if leg.Endpoint == asterisk.TeamsEndpoint {
				st.ActiveCalls++
				break
			}
		}
	}
	st.RestartNeeded = s.Ast.Status(ctx).RestartNeeded
	return st
}

func (s *Server) teamsGet(w http.ResponseWriter, r *http.Request) {
	t, err := pbx.LoadTeams(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": t, "status": s.teamsStatus(r.Context(), t)})
}

func (s *Server) teamsSave(w http.ResponseWriter, r *http.Request) {
	var t pbx.TeamsSettings
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ctx := r.Context()
	st, err := pbx.LoadSettings(ctx, s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	exts, err := s.PBX.Extensions(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	known := map[string]bool{}
	for _, e := range exts {
		known[e.Number] = true
	}
	if err := t.Validate(st, known); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if t.TrunkID != 0 {
		if _, err := s.PBX.Trunk(ctx, t.TrunkID); err != nil {
			fieldErr(w, "trunkId")
			return
		}
	}
	old, _ := pbx.LoadTeams(ctx, s.DB)
	if err := store.SetJSON(ctx, s.DB, pbx.TeamsKey, t); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "teams.settings", map[bool]string{true: "enabled", false: "disabled"}[t.Enabled])
	s.Ast.Changed()
	if old.Enabled != t.Enabled || old.Port != t.Port {
		s.firewallChanged(ctx)
	}
	s.teamsGet(w, r)
}

func (s *Server) teamsScript(w http.ResponseWriter, r *http.Request) {
	t, err := pbx.LoadTeams(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "nl" {
		lang = "en"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="kiestoon-teams.ps1"`)
	_, _ = w.Write([]byte("\ufeff" + pbx.TeamsScript(t, lang)))
}
