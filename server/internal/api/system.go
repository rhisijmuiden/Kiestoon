package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/backup"
	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/firewall"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) systemRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/mail/settings", admin(s.mailSettings))
	mux.Handle("PUT /api/mail/settings", admin(s.mailSaveSettings))
	mux.Handle("POST /api/mail/test", admin(s.mailTest))
	mux.Handle("GET /api/mail/log", admin(s.mailLog))

	mux.Handle("GET /api/firewall", admin(s.firewallGet))
	mux.Handle("PUT /api/firewall", admin(s.firewallSave))
	mux.Handle("POST /api/firewall/confirm", admin(s.firewallConfirm))
	mux.Handle("GET /api/firewall/bans", admin(s.firewallBans))
	mux.Handle("POST /api/firewall/unban", admin(s.firewallUnban))

	mux.Handle("GET /api/backups/targets", admin(s.backupTargets))
	mux.Handle("POST /api/backups/targets", admin(s.backupSaveTarget))
	mux.Handle("PUT /api/backups/targets/{id}", admin(s.backupSaveTarget))
	mux.Handle("DELETE /api/backups/targets/{id}", admin(s.backupDeleteTarget))
	mux.Handle("GET /api/backups/targets/{id}/key", admin(s.backupRevealKey))
	mux.Handle("POST /api/backups/targets/{id}/test", admin(s.backupTest))
	mux.Handle("POST /api/backups/targets/{id}/onedrive/start", admin(s.oneDriveStart))
	mux.Handle("POST /api/backups/targets/{id}/onedrive/poll", admin(s.oneDrivePoll))
	mux.Handle("GET /api/backups/targets/{id}/snapshots", admin(s.backupSnapshots))
	mux.Handle("POST /api/backups/run", admin(s.backupRun))
	mux.Handle("GET /api/backups/runs", admin(s.backupRuns))
	mux.Handle("GET /api/backups/schedule", admin(s.backupSchedule))
	mux.Handle("PUT /api/backups/schedule", admin(s.backupSaveSchedule))
	mux.Handle("POST /api/backups/restore", admin(s.backupRestore))
	mux.Handle("GET /api/backups/restore/status", admin(s.backupRestoreStatus))

	mux.Handle("GET /api/provisioning/settings", admin(s.provSettings))
	mux.Handle("PUT /api/provisioning/settings", admin(s.provSaveSettings))
	mux.Handle("GET /api/provisioning/models", admin(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, provision.Models) }))
	mux.Handle("GET /api/provisioning/modules", admin(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, provision.ModuleTypes) }))
	mux.Handle("GET /api/phones", admin(s.phones))
	mux.Handle("POST /api/phones", admin(s.savePhone))
	mux.Handle("PUT /api/phones/{id}", admin(s.savePhone))
	mux.Handle("DELETE /api/phones/{id}", admin(s.deletePhone))
	mux.Handle("GET /api/phones/{id}/preview", admin(s.previewPhone))
	s.phoneAssignRoutes(mux, admin)
	s.hotdeskRoutes(mux, admin)
	s.adminPasswordRoutes(mux, admin)
	s.keyTemplateRoutes(mux, admin)
	s.extGroupRoutes(mux, admin)

	mux.Handle("GET /api/system/helper", admin(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"available": s.Helper.Available(r.Context())})
	}))
}

func (s *Server) helperErr(w http.ResponseWriter, err error) {
	if errors.Is(err, helper.ErrUnavailable) {
		writeErr(w, http.StatusServiceUnavailable, "helper_unavailable")
		return
	}
	msg := err.Error()
	if msg == "backup_busy" {
		writeErr(w, http.StatusConflict, "backup_busy")
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "helper_failed", "message": msg})
}

// ---- mail -------------------------------------------------------------------

func (s *Server) mailSettings(w http.ResponseWriter, r *http.Request) {
	m, err := mail.Load(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": m.Public(), "pending": s.Mail.Pending()})
}

func (s *Server) mailSaveSettings(w http.ResponseWriter, r *http.Request) {
	var m mail.Settings
	if err := decode(r, &m); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	old, _ := mail.Load(r.Context(), s.DB)
	m.Merge(old)
	if err := m.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, mail.SettingsKey, m); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "mail.settings", m.Host)
	s.Ast.Changed() // voicemail.conf uses the sender address
	writeJSON(w, http.StatusOK, m.Public())
}

func (s *Server) mailTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	if err := decode(r, &req); err != nil || !strings.Contains(req.To, "@") || strings.ContainsAny(req.To, "\r\n,") {
		fieldErr(w, "to")
		return
	}
	lang := sessionFrom(r).User.Language
	subject, body := "Kiestoon test e-mail", "This is a test e-mail from your Kiestoon phone system. The mail server settings work."
	if lang == "nl" {
		subject, body = "Kiestoon test-e-mail", "Dit is een test-e-mail van je Kiestoon telefooncentrale. De mailserverinstellingen werken."
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.Mail.SendText(ctx, "test", req.To, subject, body); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "mail_failed", "message": err.Error()})
		return
	}
	s.audit(r, "mail.test", req.To)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) mailLog(w http.ResponseWriter, r *http.Request) {
	l, err := s.Mail.Log(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// ---- firewall ---------------------------------------------------------------

func (s *Server) firewallConfig(ctx context.Context) firewall.Config {
	c := firewall.Default()
	_ = store.GetJSON(ctx, s.DB, firewall.SettingsKey, &c)
	return c
}

func listenPort(addr string, fallback int) int {
	if addr == "" || addr == "off" {
		return 0
	}
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return fallback
	}
	return n
}

func (s *Server) firewallPorts(ctx context.Context) firewall.Ports {
	st, _ := pbx.LoadSettings(ctx, s.DB)
	p := firewall.Ports{SIP: st.SIPPort, SIPTLS: st.TLSPort, RTPStart: st.RTPStart, RTPEnd: st.RTPEnd}
	inet := s.internetSettings(ctx)
	web := []int{listenPort(s.Cfg.Listen, 8080), listenPort(s.Cfg.TLSListen, 8443)}
	if inet.Enabled {
		// Caddy serves the portal to the whole internet on 80 and 443.
		p.Public = []int{80, 443}
	} else {
		web = append(web, 80, 443)
	}
	for _, port := range web {
		if port > 0 {
			p.Portal = append(p.Portal, port)
		}
	}
	if s.faxInUse(ctx) {
		p.UDPTL = [2]int{4000, 4999}
	}
	if ls := s.ldapSettings(ctx); ls.LDAPEnabled {
		p.LDAP = ls.Ports()
	}
	// Multi-site and high availability tunnels.
	if h := s.hub(ctx); h.Enabled {
		p.WireGuard = append(p.WireGuard, h.ListenPort)
	}
	if n := cluster.LoadNode(s.Cfg.DataDir); n.Enabled && n.Peer != nil {
		p.WireGuard = append(p.WireGuard, n.Port)
		p.ClusterPeer = n.Peer.Address
	}
	p.Tunnels = len(s.tunnels(ctx)) > 0
	if t, _ := pbx.LoadTeams(ctx, s.DB); t.Enabled {
		p.Teams, p.TeamsNets = t.Port, pbx.TeamsSignalingNets
	}
	return p
}

// trunkIPs resolves the addresses of every enabled trunk, so the firewall lets the providers in.
func (s *Server) trunkIPs(ctx context.Context) []string {
	trunks, _ := s.PBX.Trunks(ctx)
	out := []string{}
	for _, t := range trunks {
		if !t.Enabled {
			continue
		}
		if ip := net.ParseIP(t.Host); ip != nil {
			out = append(out, ip.String())
		} else {
			lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			ips, _ := net.DefaultResolver.LookupIP(lctx, "ip", t.Host)
			cancel()
			for _, ip := range ips {
				out = append(out, ip.String())
			}
		}
		for _, m := range strings.Split(t.MatchIPs, ",") {
			if m != "" {
				out = append(out, m)
			}
		}
	}
	return out
}

func (s *Server) applyFirewall(ctx context.Context, c firewall.Config, confirm int) (helper.FirewallStatus, error) {
	var st helper.FirewallStatus
	err := s.Helper.Call(ctx, "firewall.apply", helper.FirewallArgs{Config: c, Ports: s.firewallPorts(ctx), TrunkIPs: s.trunkIPs(ctx), ConfirmSeconds: confirm}, &st)
	return st, err
}

func (s *Server) applyFail2ban(ctx context.Context, c firewall.Config) error {
	p := s.firewallPorts(ctx)
	sip := fmt.Sprintf("%d,%d", p.SIP, p.SIPTLS)
	portal := make([]string, len(p.Portal))
	for i, x := range p.Portal {
		portal[i] = strconv.Itoa(x)
	}
	return s.Helper.Call(ctx, "fail2ban.apply", helper.Fail2banArgs{Config: c, SIPPorts: sip, PortalPorts: strings.Join(portal, ",")}, nil)
}

// ApplySecurityOnStart writes the fail2ban jails once the helper answers, so a fresh install is
// protected without anyone opening the firewall page. The nftables rules are restored by the helper.
func (s *Server) ApplySecurityOnStart(ctx context.Context) {
	for i := 0; i < 30; i++ {
		if s.Helper.Available(ctx) {
			cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			err := s.applyFail2ban(cctx, s.firewallConfig(ctx))
			cancel()
			if err != nil {
				s.Log.Warn("fail2ban apply", "err", err)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// firewallChanged re-applies an active firewall after trunks or ports changed.
func (s *Server) firewallChanged(ctx context.Context) {
	c := s.firewallConfig(ctx)
	if !c.Enabled {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.applyFirewall(ctx, c, 0); err != nil && !errors.Is(err, helper.ErrUnavailable) {
			s.Log.Warn("firewall update", "err", err)
		}
	}()
}

func (s *Server) firewallGet(w http.ResponseWriter, r *http.Request) {
	c := s.firewallConfig(r.Context())
	var st helper.FirewallStatus
	err := s.Helper.Call(r.Context(), "firewall.status", nil, &st)
	writeJSON(w, http.StatusOK, map[string]any{
		"config": c, "status": st, "helperAvailable": err == nil, "trunkIps": s.trunkIPs(r.Context()),
		"ports": s.firewallPorts(r.Context()), "clientIp": clientIP(r),
	})
}

func (s *Server) firewallSave(w http.ResponseWriter, r *http.Request) {
	var c firewall.Config
	if err := decode(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := c.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "addresses", "message": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// Safe mode: the change is rolled back after 90 seconds unless the browser confirms it.
	st, err := s.applyFirewall(ctx, c, 90)
	if err != nil {
		s.helperErr(w, err)
		return
	}
	if err := s.applyFail2ban(ctx, c); err != nil {
		s.Log.Warn("fail2ban", "err", err)
	}
	if err := store.SetJSON(r.Context(), s.DB, firewall.SettingsKey, c); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "firewall.saved", fmt.Sprintf("enabled=%v", c.Enabled))
	writeJSON(w, http.StatusOK, map[string]any{"config": c, "status": st})
}

func (s *Server) firewallConfirm(w http.ResponseWriter, r *http.Request) {
	var st helper.FirewallStatus
	if err := s.Helper.Call(r.Context(), "firewall.confirm", nil, &st); err != nil {
		s.helperErr(w, err)
		return
	}
	s.audit(r, "firewall.confirmed", "")
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) firewallBans(w http.ResponseWriter, r *http.Request) {
	var bans []helper.Ban
	if err := s.Helper.Call(r.Context(), "fail2ban.bans", nil, &bans); err != nil {
		s.helperErr(w, err)
		return
	}
	if bans == nil {
		bans = []helper.Ban{}
	}
	writeJSON(w, http.StatusOK, bans)
}

func (s *Server) firewallUnban(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := decode(r, &req); err != nil || net.ParseIP(req.IP) == nil {
		fieldErr(w, "ip")
		return
	}
	if err := s.Helper.Call(r.Context(), "fail2ban.unban", req, nil); err != nil {
		s.helperErr(w, err)
		return
	}
	s.audit(r, "firewall.unban", req.IP)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- backups ----------------------------------------------------------------

const targetCols = `id, name, kind, host, port, username, password, private_key, path, implicit_tls, repo_password, enabled, onedrive, extra`

func scanTarget(row interface{ Scan(...any) error }) (*backup.Target, error) {
	var t backup.Target
	var tls, en int
	var od, extra string
	if err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.Host, &t.Port, &t.Username, &t.Password, &t.PrivateKey, &t.Path, &tls, &t.RepoPassword, &en, &od, &extra); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(extra), &t.Extra)
	t.ImplicitTLS, t.Enabled = tls == 1, en == 1
	if t.Kind == "onedrive" {
		t.OneDrive = &backup.OneDrive{}
		_ = json.Unmarshal([]byte(od), t.OneDrive)
	}
	return &t, nil
}

func (s *Server) loadTargets(ctx context.Context) ([]backup.Target, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+targetCols+` FROM backup_targets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []backup.Target{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Server) loadTarget(ctx context.Context, id int64) (*backup.Target, error) {
	return scanTarget(s.DB.QueryRowContext(ctx, `SELECT `+targetCols+` FROM backup_targets WHERE id = ?`, id))
}

func publicTarget(t backup.Target) backup.Target {
	t.PasswordSet, t.KeySet = t.Password != "", t.PrivateKey != ""
	t.Password, t.PrivateKey, t.RepoPassword = "", "", ""
	if t.OneDrive != nil {
		o := *t.OneDrive
		o.SecretSet, o.Connected = o.ClientSecret != "", o.RefreshToken != "" && o.DriveID != ""
		o.ClientSecret, o.RefreshToken, o.AccessToken = "", "", ""
		t.OneDrive = &o
	}
	return t
}

func oneDriveJSON(t backup.Target) string {
	if t.OneDrive == nil {
		return "{}"
	}
	o := *t.OneDrive
	o.AccessToken, o.Expiry = "", time.Time{}
	b, _ := json.Marshal(o)
	return string(b)
}

func (s *Server) backupTargets(w http.ResponseWriter, r *http.Request) {
	ts, err := s.loadTargets(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	for i := range ts {
		ts[i] = publicTarget(ts[i])
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) backupSaveTarget(w http.ResponseWriter, r *http.Request) {
	var t backup.Target
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t.ID = pathID(r)
	if t.ID != 0 {
		old, err := s.loadTarget(r.Context(), t.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		if t.Password == "" {
			t.Password = old.Password
		}
		if t.PrivateKey == "" {
			t.PrivateKey = old.PrivateKey
		}
		if t.RepoPassword == "" {
			t.RepoPassword = old.RepoPassword
		}
		if t.OneDrive != nil && old.OneDrive != nil {
			if t.OneDrive.ClientSecret == "" {
				t.OneDrive.ClientSecret = old.OneDrive.ClientSecret
			}
			// The Microsoft sign-in stays valid while the app registration stays the same.
			if t.OneDrive.ClientID == old.OneDrive.ClientID && t.OneDrive.Tenant == old.OneDrive.Tenant {
				t.OneDrive.RefreshToken, t.OneDrive.DriveID, t.OneDrive.DriveType, t.OneDrive.Account =
					old.OneDrive.RefreshToken, old.OneDrive.DriveID, old.OneDrive.DriveType, old.OneDrive.Account
			} else {
				t.OneDrive.RefreshToken, t.OneDrive.DriveID, t.OneDrive.DriveType, t.OneDrive.Account = "", "", "", ""
			}
		}
	} else if t.RepoPassword == "" {
		t.RepoPassword = pbx.GenerateSecret(32)
	}
	if t.OneDrive != nil && t.ID == 0 {
		t.OneDrive.RefreshToken, t.OneDrive.DriveID, t.OneDrive.Account = "", "", ""
	}
	if err := t.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	args := []any{t.Name, t.Kind, t.Host, t.Port, t.Username, t.Password, t.PrivateKey, t.Path, b2i(t.ImplicitTLS), t.RepoPassword, b2i(t.Enabled), oneDriveJSON(t), extraJSON(t)}
	if t.ID == 0 {
		res, err := s.DB.ExecContext(r.Context(), `INSERT INTO backup_targets(name, kind, host, port, username, password, private_key,
			path, implicit_tls, repo_password, enabled, onedrive, extra) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		t.ID, _ = res.LastInsertId()
	} else if _, err := s.DB.ExecContext(r.Context(), `UPDATE backup_targets SET name=?, kind=?, host=?, port=?, username=?, password=?,
		private_key=?, path=?, implicit_tls=?, repo_password=?, enabled=?, onedrive=?, extra=? WHERE id=?`, append(args, t.ID)...); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "backup.target.saved", t.Name)
	writeJSON(w, http.StatusOK, publicTarget(t))
}

func extraJSON(t backup.Target) string {
	b, _ := json.Marshal(t.Extra)
	return string(b)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) backupDeleteTarget(w http.ResponseWriter, r *http.Request) {
	if _, err := s.DB.ExecContext(r.Context(), `DELETE FROM backup_targets WHERE id = ?`, pathID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "backup.target.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) backupRevealKey(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTarget(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "backup.key.revealed", t.Name)
	writeJSON(w, http.StatusOK, map[string]string{"repoPassword": t.RepoPassword})
}

func (s *Server) backupTest(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTarget(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var res map[string]bool
	if err := s.backupHelper(ctx, "backup.test", helper.BackupArgs{Target: *t}, &res); err != nil {
		s.helperErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) backupSnapshots(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTarget(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var snaps []backup.Snapshot
	if err := s.backupHelper(ctx, "backup.snapshots", helper.BackupArgs{Target: *t}, &snaps); err != nil {
		s.helperErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

// Schedule says when automatic backups run.
type Schedule struct {
	Enabled bool `json:"enabled"`
	// Time of day, HH:MM in the server's time zone.
	Time string `json:"time"`
	// FullWeekday is the day of the weekly full backup (0 = Sunday); the other days are incremental.
	FullWeekday int              `json:"fullWeekday"`
	Retention   backup.Retention `json:"retention"`
}

func (s *Server) schedule(ctx context.Context) Schedule {
	sc := Schedule{Time: "02:30", FullWeekday: 0, Retention: backup.Retention{Daily: 7, Weekly: 4, Monthly: 6}}
	_ = store.GetJSON(ctx, s.DB, "backup_schedule", &sc)
	return sc
}

func (s *Server) backupSchedule(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.schedule(r.Context()))
}

func (s *Server) backupSaveSchedule(w http.ResponseWriter, r *http.Request) {
	var sc Schedule
	if err := decode(r, &sc); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if _, err := time.Parse("15:04", sc.Time); err != nil {
		fieldErr(w, "time")
		return
	}
	if sc.FullWeekday < 0 || sc.FullWeekday > 6 || sc.Retention.Daily < 0 || sc.Retention.Weekly < 0 || sc.Retention.Monthly < 0 ||
		sc.Retention.Daily+sc.Retention.Weekly+sc.Retention.Monthly == 0 {
		fieldErr(w, "retention")
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, "backup_schedule", sc); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "backup.schedule", sc.Time)
	writeJSON(w, http.StatusOK, sc)
}

// stageDatabase writes a consistent copy of the live database for the backup.
func (s *Server) stageDatabase(ctx context.Context) error {
	dir := filepath.Join(s.Cfg.DataDir, backup.StagingDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(dir, "kiestoon.db")
	_ = os.Remove(dst)
	_, err := s.DB.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

// runBackup makes one backup and records it. It is used by the button and by the schedule.
func (s *Server) runBackup(ctx context.Context, t backup.Target, kind, trigger string) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	start := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO backup_runs(target_id, kind, trigger, started_at) VALUES (?,?,?,?)`, t.ID, kind, trigger, start)
	if err != nil {
		return
	}
	runID, _ := res.LastInsertId()
	finish := func(state, msg string, r *backup.Result) {
		if r == nil {
			r = &backup.Result{}
		}
		_, _ = s.DB.ExecContext(context.Background(), `UPDATE backup_runs SET finished_at=?, state=?, message=?, snapshot_id=?, bytes_added=?, files=? WHERE id=?`,
			time.Now().UTC().Format(time.RFC3339), state, msg, r.SnapshotID, r.BytesAdded, r.Files, runID)
	}
	if err := s.stageDatabase(ctx); err != nil {
		finish("failed", "database copy: "+err.Error(), nil)
		return
	}
	var out backup.Result
	err = s.backupHelper(ctx, "backup.run", helper.BackupArgs{Target: t, Kind: kind, Retention: s.schedule(ctx).Retention}, &out)
	if err != nil {
		finish("failed", err.Error(), nil)
		s.Mail.Notify(context.Background(), "Kiestoon: backup failed",
			fmt.Sprintf("The %s backup to %q failed:\n\n%s", kind, t.Name, err.Error()))
		return
	}
	finish("success", "", &out)
}

func (s *Server) backupRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetID int64  `json:"targetId"`
		Kind     string `json:"kind"`
	}
	if err := decode(r, &req); err != nil || (req.Kind != "full" && req.Kind != "incremental") {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t, err := s.loadTarget(r.Context(), req.TargetID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !s.Helper.Available(r.Context()) {
		writeErr(w, http.StatusServiceUnavailable, "helper_unavailable")
		return
	}
	s.audit(r, "backup.run", req.Kind+" "+t.Name)
	go s.runBackup(context.Background(), *t, req.Kind, "manual")
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

type backupRunRow struct {
	ID         int64  `json:"id"`
	Target     string `json:"target"`
	Kind       string `json:"kind"`
	Trigger    string `json:"trigger"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	State      string `json:"state"`
	Message    string `json:"message"`
	SnapshotID string `json:"snapshotId"`
	BytesAdded int64  `json:"bytesAdded"`
	Files      int64  `json:"files"`
}

func (s *Server) backupRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT b.id, COALESCE(t.name, ''), b.kind, b.trigger, b.started_at, b.finished_at,
		b.state, b.message, b.snapshot_id, b.bytes_added, b.files FROM backup_runs b LEFT JOIN backup_targets t ON t.id = b.target_id
		ORDER BY b.id DESC LIMIT 50`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	defer rows.Close()
	out := []backupRunRow{}
	for rows.Next() {
		var x backupRunRow
		if rows.Scan(&x.ID, &x.Target, &x.Kind, &x.Trigger, &x.StartedAt, &x.FinishedAt, &x.State, &x.Message, &x.SnapshotID, &x.BytesAdded, &x.Files) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) backupRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetID int64  `json:"targetId"`
		Snapshot string `json:"snapshot"`
		Confirm  string `json:"confirm"`
	}
	if err := decode(r, &req); err != nil || req.Snapshot == "" {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.Confirm != "RESTORE" {
		fieldErr(w, "confirm")
		return
	}
	t, err := s.loadTarget(r.Context(), req.TargetID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "backup.restore", t.Name+" "+req.Snapshot)
	if err := s.backupHelper(r.Context(), "backup.restore", helper.BackupArgs{Target: *t, Snapshot: req.Snapshot}, nil); err != nil {
		s.helperErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "running"})
}

func (s *Server) backupRestoreStatus(w http.ResponseWriter, r *http.Request) {
	var st helper.RestoreStatus
	b, err := os.ReadFile(filepath.Join(s.Cfg.DataDir, "restore-status.json"))
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	writeJSON(w, http.StatusOK, st)
}

// RunScheduler starts scheduled backups until ctx ends.
func (s *Server) RunScheduler(ctx context.Context) {
	// A backup cannot survive a restart of the portal, and a restored database holds the row of the
	// backup that made it. Either way a "running" row at start-up is stale.
	_, _ = s.DB.ExecContext(ctx, `UPDATE backup_runs SET state='failed', message='interrupted', finished_at=? WHERE state='running'`,
		time.Now().UTC().Format(time.RFC3339))
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			sc := s.schedule(ctx)
			if !sc.Enabled || now.Format("15:04") != sc.Time || last == now.Format("2006-01-02") {
				continue
			}
			last = now.Format("2006-01-02")
			kind := "incremental"
			if int(now.Weekday()) == sc.FullWeekday {
				kind = "full"
			}
			targets, err := s.loadTargets(ctx)
			if err != nil {
				continue
			}
			for _, tg := range targets {
				if tg.Enabled {
					k := kind
					var n int
					_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_runs WHERE target_id = ? AND state = 'success'`, tg.ID).Scan(&n)
					if n == 0 {
						k = "full"
					}
					s.runBackup(ctx, tg, k, "schedule")
				}
			}
		}
	}
}

// ---- provisioning -----------------------------------------------------------

func (s *Server) provSettings(w http.ResponseWriter, r *http.Request) {
	p, err := provision.Load(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) provSaveSettings(w http.ResponseWriter, r *http.Request) {
	var p provision.Settings
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := p.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, provision.SettingsKey, p); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "provisioning.settings", "")
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) phones(w http.ResponseWriter, r *http.Request) {
	ps, err := s.PBX.Phones(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) savePhone(w http.ResponseWriter, r *http.Request) {
	p := pbx.Phone{Options: pbx.DefaultPhoneOptions()}
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	p.ID = pathID(r)
	lim, ok := provision.Limits(p.Vendor, p.Model)
	if !ok {
		fieldErr(w, "model")
		return
	}
	if p.ExtensionID != nil {
		if _, err := s.PBX.Extension(r.Context(), *p.ExtensionID); err != nil {
			fieldErr(w, "extensionId")
			return
		}
	}
	for _, id := range p.Lines {
		if _, err := s.PBX.Extension(r.Context(), id); err != nil {
			fieldErr(w, "lines")
			return
		}
	}
	var old *pbx.Phone
	if p.ID != 0 {
		old, _ = s.PBX.Phone(r.Context(), p.ID)
	}
	// A phone that gets (another) extension takes the key template of that extension's group.
	if p.ExtensionID != nil && (old == nil || old.ExtensionID == nil || *old.ExtensionID != *p.ExtensionID) {
		if t := s.groupTemplateFor(r.Context(), *p.ExtensionID, p.Vendor, p.Model); t != nil {
			copyTemplate(t, &p)
		}
	}
	if err := s.PBX.SavePhone(r.Context(), &p, lim); err != nil {
		s.saveErr(w, err)
		return
	}
	if p.HasForwardKey() {
		s.Ast.Changed() // the dialplan holds the forward key codes
	}
	p.HotdeskExtensionID, p.HotdeskAt = nil, ""
	if old != nil && old.HotdeskExtensionID != nil {
		if p.Options.Hotdesk.Enabled {
			p.HotdeskExtensionID, p.HotdeskAt = old.HotdeskExtensionID, old.HotdeskAt
		} else {
			_, _ = s.PBX.SetHotdesk(r.Context(), p.ID, nil)
		}
	}
	s.audit(r, "phone.saved", p.MAC)
	_ = s.PBX.ForgetDiscovery(r.Context(), p.MAC)
	s.ztpAfterSave(r, &p, old)
	// A changed phone fetches its new keys and lines right away, through the account it uses now.
	state := ""
	if old != nil {
		state, _ = s.notifyPhone(r.Context(), &p, s.registeredAs(r.Context(), old))
	}
	writeJSON(w, http.StatusOK, struct {
		pbx.Phone
		Resync string `json:"resync,omitempty"`
	}{p, state})
}

func (s *Server) deletePhone(w http.ResponseWriter, r *http.Request) {
	old, _ := s.PBX.Phone(r.Context(), pathID(r))
	if err := s.PBX.DeletePhone(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	if old != nil {
		s.ztpRemove(old)
	}
	s.audit(r, "phone.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) provInput(ctx context.Context, p *pbx.Phone, host, scheme string) (provision.Input, error) {
	set, err := provision.Load(ctx, s.DB)
	if err != nil {
		return provision.Input{}, err
	}
	st, _ := pbx.LoadSettings(ctx, s.DB)
	in := provision.Input{Settings: set, PBX: st, Phone: *p, Server: set.Server}
	if p.ID != 0 {
		if pw, err := s.adminPasswordFor(ctx, set, p.ID); err == nil && pw != "" {
			in.Settings.AdminPassword = pw
		}
	}
	if in.Server == "" {
		if h, _, err := net.SplitHostPort(host); err == nil {
			in.Server = h
		} else {
			in.Server = host
		}
	}
	if id := p.ActiveExtensionID(); id != nil {
		e, err := s.PBX.Extension(ctx, *id)
		if err == nil && e.Enabled {
			in.Ext = e
		}
	}
	for _, id := range p.Lines {
		if e, err := s.PBX.Extension(ctx, id); err == nil && e.Enabled {
			in.Lines = append(in.Lines, e)
		}
	}
	in.Org, _ = store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	in.Base = s.provBase(scheme, set, host)
	if fw := s.Firmware.ForModel(ctx, p.Vendor, p.Model); fw != nil {
		in.FirmwareDir = s.firmwareBase(scheme, host) + fw.Token + "/"
		in.Firmware = in.FirmwareDir + fw.Filename
	}
	if pb := s.ldapSettings(ctx); pb.XMLEnabled {
		switch p.Vendor {
		case "yealink", "snom", "fanvil", "cisco":
			in.PhonebookURL = s.provBase(scheme, set, host) + "phonebook/" + p.Vendor + ".xml"
		case "grandstream":
			// Grandstream appends phonebook.xml to the directory it is given.
			in.PhonebookURL = s.provBase(scheme, set, host) + "phonebook/grandstream/phonebook.xml"
		}
	}
	return in, nil
}

// provBase is the provisioning address with the credentials phones use.
func (s *Server) provBase(scheme string, set provision.Settings, host string) string {
	return fmt.Sprintf("%s://%s:%s@%s/prov/", scheme, url.PathEscape(set.Username), url.PathEscape(set.Password), host)
}

func (s *Server) previewPhone(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	in, err := s.provInput(r.Context(), p, r.Host, s.scheme(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	body, _ := provision.Render(in)
	if in.Ext != nil {
		body = strings.ReplaceAll(body, in.Ext.SIPSecret, "••••••••")
	}
	body = strings.ReplaceAll(body, in.Settings.AdminPassword, "••••••••")
	writeJSON(w, http.StatusOK, map[string]string{"body": body})
}

// ProvisioningHandler serves /prov/: configuration files for phones, protected with HTTP basic
// authentication (phones send it when the URL contains user:password@).
func (s *Server) ProvisioningHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ip := clientIP(r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.provLimit.Blocked(ip) {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/prov/fw/") {
			s.serveFirmware(w, r, ip)
			return
		}
		set, err := provision.Load(r.Context(), s.DB)
		if err != nil || !set.Enabled {
			http.NotFound(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(set.Username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(set.Password)) != 1 {
			if ok {
				s.provLimit.Fail(ip)
				s.Log.Warn("provisioning auth failed", "ip", ip)
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="Kiestoon provisioning"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/prov/")
		req, ok := provision.Parse(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if req.Phonebook {
			s.provLimit.Reset(ip)
			s.phonebookXML(w, r, req.Vendor)
			return
		}
		if req.Firmware {
			s.snomFirmware(w, r, req.Model)
			return
		}
		if req.Common {
			switch req.Vendor {
			case "yealink":
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(provision.YealinkCommon()))
			case "poly":
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				_, _ = w.Write([]byte(provision.PolyMaster("")))
			case "cisco":
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				_, _ = w.Write([]byte(provision.CiscoCommon(s.provBase(s.scheme(r), set, r.Host))))
			default:
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				_, _ = w.Write([]byte(provision.SnomIndex(s.provBase(s.scheme(r), set, r.Host))))
			}
			return
		}
		phone, err := s.PBX.PhoneByMAC(r.Context(), req.MAC)
		if errors.Is(err, pbx.ErrNotFound) || (err == nil && !req.Matches(phone.Vendor)) {
			s.Log.Info("provisioning request for unknown phone", "mac", req.MAC, "ip", ip)
			if errors.Is(err, pbx.ErrNotFound) {
				s.recordDiscovery(r.Context(), req, name, ip, r.UserAgent())
			}
			http.NotFound(w, r)
			return
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Poly asks for <mac>.cfg first: that is the master file pointing to kiestoon-<mac>.cfg.
		if phone.Vendor == "poly" && !req.PolyConfig {
			fw := ""
			if f := s.Firmware.ForModel(r.Context(), phone.Vendor, phone.Model); f != nil {
				fw = s.firmwareBase(s.scheme(r), r.Host) + f.Token + "/" + f.Filename
			}
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			_, _ = w.Write([]byte(provision.PolyMaster(fw)))
			return
		}
		in, err := s.provInput(r.Context(), phone, r.Host, s.scheme(r))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Phones in a branch office fetch their configuration through the Kiestoon Edge: they
		// register at the edge's proxy, which is the address they used.
		if s.edgeFromTunnel(r) != nil {
			if h, _, err := net.SplitHostPort(r.Host); err == nil {
				in.Server = h
			} else {
				in.Server = r.Host
			}
		}
		body, ct := provision.Render(in)
		s.PBX.TouchPhone(r.Context(), phone.ID, ip, r.UserAgent())
		s.PBX.AdminPasswordSent(r.Context(), phone.ID, in.Settings.AdminPassword)
		s.provLimit.Reset(ip)
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write([]byte(body))
	})
}

// backupHelper runs a backup command in the root helper. For OneDrive it first gets a fresh access
// token and stores the new refresh token Microsoft hands out.
func (s *Server) backupHelper(ctx context.Context, cmd string, a helper.BackupArgs, out any) error {
	if a.Target.Kind == "onedrive" && a.Target.OneDrive != nil {
		o := *a.Target.OneDrive
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := o.Refresh(rctx, &http.Client{Timeout: 30 * time.Second})
		cancel()
		if err != nil {
			return fmt.Errorf("onedrive sign-in: %w", err)
		}
		a.Target.OneDrive = &o
		_, _ = s.DB.ExecContext(ctx, `UPDATE backup_targets SET onedrive = ? WHERE id = ?`, oneDriveJSON(a.Target), a.Target.ID)
	}
	return s.Helper.Call(ctx, cmd, a, out)
}

// ---- OneDrive sign-in -------------------------------------------------------

type oneDriveLogin struct {
	device  string
	expires time.Time
}

func (s *Server) oneDriveStart(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTarget(r.Context(), pathID(r))
	if err != nil || t.OneDrive == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	dc, err := t.OneDrive.StartDeviceLogin(ctx, &http.Client{Timeout: 20 * time.Second})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "onedrive_failed", "message": err.Error()})
		return
	}
	s.odMu.Lock()
	if s.odLogins == nil {
		s.odLogins = map[int64]oneDriveLogin{}
	}
	s.odLogins[t.ID] = oneDriveLogin{device: dc.DeviceCode, expires: time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)}
	s.odMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"userCode": dc.UserCode, "verificationUri": dc.VerificationURI, "expiresIn": dc.ExpiresIn,
		"interval": max(dc.Interval, 3)})
}

func (s *Server) oneDrivePoll(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTarget(r.Context(), pathID(r))
	if err != nil || t.OneDrive == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.odMu.Lock()
	l, ok := s.odLogins[t.ID]
	s.odMu.Unlock()
	if !ok || time.Now().After(l.expires) {
		writeErr(w, http.StatusGone, "onedrive_expired")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	o := *t.OneDrive
	err = o.PollDeviceLogin(ctx, &http.Client{Timeout: 20 * time.Second}, l.device)
	if errors.Is(err, backup.ErrPending) {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "onedrive_failed", "message": err.Error()})
		return
	}
	s.odMu.Lock()
	delete(s.odLogins, t.ID)
	s.odMu.Unlock()
	t.OneDrive = &o
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE backup_targets SET onedrive = ? WHERE id = ?`, oneDriveJSON(*t), t.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "backup.onedrive.connected", t.Name+" "+o.Account)
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "account": o.Account, "driveType": o.DriveType})
}

// scheme is the URL scheme the client used (https also behind the Caddy proxy).
func (s *Server) scheme(r *http.Request) string {
	if s.secure(r) {
		return "https"
	}
	return "http"
}
