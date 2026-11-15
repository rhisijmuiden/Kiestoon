package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

// MetricsKey is the settings row with MetricsSettings as JSON.
const MetricsKey = "metrics"

// MetricsSettings protect the Prometheus endpoint /metrics. A scraper needs the bearer token, or an
// address in AllowFrom.
type MetricsSettings struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

func (m *MetricsSettings) validate() error {
	out := []string{}
	for _, v := range m.AllowFrom {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		p, err := parsePrefix(v)
		if err != nil || p.Bits() == 0 {
			return errors.New("allowFrom")
		}
		out = append(out, p.String())
	}
	if len(out) > 32 {
		return errors.New("allowFrom")
	}
	m.AllowFrom = out
	if m.Enabled && len(m.Token) < 20 {
		return errors.New("token")
	}
	return nil
}

func (s *Server) metricsSettings(ctx context.Context) MetricsSettings {
	var m MetricsSettings
	_ = store.GetJSON(ctx, s.DB, MetricsKey, &m)
	if m.AllowFrom == nil {
		m.AllowFrom = []string{}
	}
	return m
}

func (s *Server) metricsGet(w http.ResponseWriter, r *http.Request) {
	m := s.metricsSettings(r.Context())
	if m.Token == "" {
		m.Token = "kt_metrics_" + pbx.GenerateSecret(32)
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": m, "url": s.publicBase(r) + "/metrics"})
}

func (s *Server) metricsSave(w http.ResponseWriter, r *http.Request) {
	var m MetricsSettings
	if err := decode(r, &m); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := m.validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, MetricsKey, m); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "metrics.saved", fmt.Sprintf("enabled=%v", m.Enabled))
	s.metricsGet(w, r)
}

// MetricsHandler serves /metrics in the Prometheus text format.
func (s *Server) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := s.metricsSettings(r.Context())
		if !m.Enabled {
			http.NotFound(w, r)
			return
		}
		ok := false
		if tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found && m.Token != "" {
			ok = subtle.ConstantTimeCompare([]byte(tok), []byte(m.Token)) == 1
		}
		if !ok {
			if a, err := netip.ParseAddr(clientIP(r)); err == nil {
				for _, v := range m.AllowFrom {
					if p, err := parsePrefix(v); err == nil && p.Contains(a.Unmap()) {
						ok = true
					}
				}
			}
		}
		if !ok {
			ip := clientIP(r)
			if s.provLimit.Blocked(ip) {
				http.Error(w, "too many attempts", http.StatusTooManyRequests)
				return
			}
			s.provLimit.Fail(ip)
			w.Header().Set("WWW-Authenticate", `Bearer realm="Kiestoon metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(s.metricsText(ctx)))
	})
}

type promWriter struct {
	b    strings.Builder
	seen map[string]bool
}

func (p *promWriter) metric(name, typ, help string, value float64, labels ...string) {
	if p.seen == nil {
		p.seen = map[string]bool{}
	}
	if !p.seen[name] {
		p.seen[name] = true
		fmt.Fprintf(&p.b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}
	p.b.WriteString(name)
	if len(labels) > 0 {
		p.b.WriteString("{")
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				p.b.WriteString(",")
			}
			v := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(labels[i+1])
			fmt.Fprintf(&p.b, `%s="%s"`, labels[i], v)
		}
		p.b.WriteString("}")
	}
	fmt.Fprintf(&p.b, " %g\n", value)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (s *Server) metricsText(ctx context.Context) string {
	var p promWriter
	p.metric("kiestoon_info", "gauge", "Version of Kiestoon.", 1, "version", version.Version)
	p.metric("kiestoon_uptime_seconds", "gauge", "Seconds since the portal started.", time.Since(s.started).Seconds())
	p.metric("kiestoon_asterisk_connected", "gauge", "1 when the portal is connected to Asterisk (AMI).", b2f(s.AMI.Connected()))

	live := s.Live.Get(ctx)
	p.metric("kiestoon_active_calls", "gauge", "Calls in progress.", float64(len(live.Calls)))
	dir := map[string]int{}
	for _, c := range live.Calls {
		dir[c.Direction]++
	}
	for _, d := range []string{"inbound", "outbound", "internal"} {
		p.metric("kiestoon_active_calls_by_direction", "gauge", "Calls in progress by direction.", float64(dir[d]), "direction", d)
	}
	reg := map[string]bool{}
	for _, c := range live.Contacts {
		if !strings.EqualFold(c.Status, "Unavailable") {
			reg[c.Endpoint] = true
		}
	}
	p.metric("kiestoon_registered_endpoints", "gauge", "Phones and softphones with at least one registered contact.", float64(len(reg)))
	if exts, err := s.PBX.Extensions(ctx); err == nil {
		n := 0
		for _, e := range exts {
			if e.Enabled {
				n++
			}
		}
		p.metric("kiestoon_extensions", "gauge", "Enabled extensions.", float64(n))
	}
	if trunks, err := s.PBX.Trunks(ctx); err == nil {
		state := map[string]string{}
		for _, rg := range live.Registrations {
			state[rg.Endpoint] = rg.Status
		}
		for _, t := range trunks {
			if !t.Enabled {
				continue
			}
			st, has := state[t.Endpoint()]
			up := !has || strings.EqualFold(st, "Registered")
			p.metric("kiestoon_trunk_up", "gauge", "1 when a trunk is registered (or uses IP authentication).", b2f(up), "trunk", t.Name)
		}
	}
	for _, q := range s.States.Queues(ctx) {
		name := q.Name
		if qq, err := s.PBX.Queue(ctx, q.ID); err == nil {
			name = qq.Name
		}
		avail := 0
		for _, m := range q.Members {
			if !m.Paused && m.Status != "unavailable" && m.Status != "invalid" {
				avail++
			}
		}
		p.metric("kiestoon_queue_waiting_callers", "gauge", "Callers waiting in a queue.", float64(q.Calls), "queue", name)
		p.metric("kiestoon_queue_longest_wait_seconds", "gauge", "Longest wait of the callers in a queue.", float64(q.LongestWait), "queue", name)
		p.metric("kiestoon_queue_available_agents", "gauge", "Agents signed in, not paused.", float64(avail), "queue", name)
		p.metric("kiestoon_queue_abandoned_total", "counter", "Abandoned calls since Asterisk started.", float64(q.Abandoned), "queue", name)
		p.metric("kiestoon_queue_completed_total", "counter", "Answered calls since Asterisk started.", float64(q.Completed), "queue", name)
	}
	st := s.CDR.TodayStats(ctx)
	p.metric("kiestoon_calls_today", "gauge", "Calls since midnight.", float64(st.Today), "result", "all")
	p.metric("kiestoon_calls_today", "gauge", "Calls since midnight.", float64(st.Answered), "result", "answered")
	p.metric("kiestoon_calls_today", "gauge", "Calls since midnight.", float64(st.Missed), "result", "missed")
	p.metric("kiestoon_mail_queue_length", "gauge", "Mails waiting to be sent.", float64(s.Mail.Pending()))

	rows, err := s.DB.QueryContext(ctx, `SELECT t.name, MAX(r.finished_at) FROM backup_targets t
		LEFT JOIN backup_runs r ON r.target_id = t.id AND r.state = 'success' WHERE t.enabled = 1 GROUP BY t.id`)
	if err == nil {
		for rows.Next() {
			var name string
			var at *string
			if rows.Scan(&name, &at) != nil {
				continue
			}
			v := 0.0
			if at != nil {
				if t, err := time.Parse(time.RFC3339, *at); err == nil {
					v = float64(t.Unix())
				}
			}
			p.metric("kiestoon_backup_last_success_timestamp_seconds", "gauge", "Time of the last successful backup (0: never).", v, "target", name)
		}
		rows.Close()
	}
	counts := map[string]int{}
	if rows, err := s.DB.QueryContext(ctx, `SELECT status, COUNT(*) FROM fax_jobs GROUP BY status`); err == nil {
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				counts[k] = n
			}
		}
		rows.Close()
	}
	keys := []string{"queued", "sending", "success", "failed"}
	sort.Strings(keys)
	for _, k := range keys {
		p.metric("kiestoon_fax_jobs", "gauge", "Sent faxes by status.", float64(counts[k]), "status", k)
	}
	return p.b.String()
}
