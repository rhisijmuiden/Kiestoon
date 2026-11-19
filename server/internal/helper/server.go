package helper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/backup"
	"github.com/rhisijmuiden/kiestoon/server/internal/firewall"
	"github.com/rhisijmuiden/kiestoon/server/internal/geoip"
	"github.com/rhisijmuiden/kiestoon/server/internal/internet"
)

// FirewallArgs applies the firewall.
type FirewallArgs struct {
	Config   firewall.Config `json:"config"`
	Ports    firewall.Ports  `json:"ports"`
	TrunkIPs []string        `json:"trunkIps"`
	// ConfirmSeconds > 0 rolls the change back unless "firewall.confirm" arrives in time.
	ConfirmSeconds int `json:"confirmSeconds"`
}

// FirewallStatus is the live state of the firewall.
type FirewallStatus struct {
	Active      bool   `json:"active"`
	Pending     bool   `json:"pending"`
	SecondsLeft int    `json:"secondsLeft"`
	Ruleset     string `json:"ruleset"`
	RolledBack  bool   `json:"rolledBack"`
}

// Fail2banArgs configures fail2ban.
type Fail2banArgs struct {
	Config      firewall.Config `json:"config"`
	SIPPorts    string          `json:"sipPorts"`
	PortalPorts string          `json:"portalPorts"`
}

// Ban is a blocked address.
type Ban struct {
	Jail string `json:"jail"`
	IP   string `json:"ip"`
}

// BackupArgs selects a target and what to do with it.
type BackupArgs struct {
	Target    backup.Target    `json:"target"`
	Kind      string           `json:"kind"`
	Retention backup.Retention `json:"retention"`
	Snapshot  string           `json:"snapshot"`
}

// RestoreStatus is written to the data directory, so the portal can show it after its restart.
type RestoreStatus struct {
	State    string    `json:"state"` // running | success | failed
	Snapshot string    `json:"snapshot"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

// Server is the root helper.
type Server struct {
	Log      *slog.Logger
	StateDir string // /var/lib/kiestoon-helper
	DataDir  string // /var/lib/kiestoon
	Runner   *backup.Runner

	fwMu       sync.Mutex
	fwPending  *time.Timer
	fwDeadline time.Time
	fwPrevious string
	fwRolled   bool
	jobMu      sync.Mutex // one backup or restore at a time
	clusterMu  sync.Mutex // one promotion or demotion at a time
}

const jails = "kiestoon-sip kiestoon-portal"

// Serve listens on the socket until ctx ends.
func (s *Server) Serve(ctx context.Context, socket string) error {
	if err := os.MkdirAll(filepath.Dir(socket), 0o750); err != nil {
		return err
	}
	_ = os.Remove(socket)
	l, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer l.Close()
	allowed := map[uint32]bool{0: true}
	if u, err := user.Lookup("kiestoon"); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		allowed[uint32(uid)] = true
		_ = os.Chown(filepath.Dir(socket), 0, gid)
		_ = os.Chown(socket, 0, gid)
	}
	_ = os.Chmod(socket, 0o660)
	s.restoreFirewallAtBoot(ctx)
	s.clusterBoot(ctx)
	go func() { <-ctx.Done(); l.Close() }()
	s.Log.Info("kiestoon helper listening", "socket", socket)
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		go s.handle(ctx, conn.(*net.UnixConn), allowed)
	}
}

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var cred *syscall.Ucred
	var cerr error
	err = raw.Control(func(fd uintptr) {
		cred, cerr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return 0, err
	}
	if cerr != nil {
		return 0, cerr
	}
	return cred.Uid, nil
}

func (s *Server) handle(ctx context.Context, c *net.UnixConn, allowed map[uint32]bool) {
	defer c.Close()
	uid, err := peerUID(c)
	if err != nil || !allowed[uid] {
		_ = json.NewEncoder(c).Encode(Response{Error: "forbidden"})
		return
	}
	var req Request
	dec := json.NewDecoder(bufio.NewReader(&limitedConn{c, 1 << 20}))
	if err := dec.Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(Response{Error: "bad_request"})
		return
	}
	data, err := s.dispatch(ctx, req)
	resp := Response{OK: err == nil}
	if err != nil {
		resp.Error = err.Error()
		s.Log.Warn("helper command failed", "cmd", req.Cmd, "err", err)
	} else if data != nil {
		resp.Data, _ = json.Marshal(data)
	}
	_ = json.NewEncoder(c).Encode(resp)
}

type limitedConn struct {
	c *net.UnixConn
	n int
}

func (l *limitedConn) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("request too large")
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.c.Read(p)
	l.n -= n
	return n, err
}

func (s *Server) dispatch(ctx context.Context, req Request) (any, error) {
	switch req.Cmd {
	case "ping":
		return "pong", nil
	case "firewall.apply":
		var a FirewallArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return s.firewallApply(ctx, a)
	case "firewall.confirm":
		return s.firewallConfirm(ctx)
	case "firewall.status":
		return s.firewallStatus(ctx), nil
	case "fail2ban.apply":
		var a Fail2banArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return nil, s.fail2banApply(ctx, a)
	case "fail2ban.bans":
		return s.fail2banBans(ctx)
	case "fail2ban.unban":
		var a struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(req.Args, &a); err != nil || net.ParseIP(a.IP) == nil {
			return nil, errors.New("invalid ip")
		}
		_, err := runCmd(ctx, "fail2ban-client", "unban", net.ParseIP(a.IP).String())
		return nil, err
	case "backup.test", "backup.run", "backup.snapshots", "backup.restore":
		var a BackupArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return s.backupCmd(ctx, req.Cmd, a)
	case "internet.apply":
		var a InternetArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return s.internetApply(ctx, a)
	case "internet.status":
		var a internet.Settings
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		if err := a.Validate(); err != nil {
			return nil, err
		}
		return s.internetStatus(ctx, a)
	case "asterisk.restart":
		_, err := runCmd(ctx, "systemctl", "restart", "asterisk.service")
		return nil, err
	case "wireguard.apply":
		var a WireGuardArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return s.wireguardApply(ctx, a)
	case "wireguard.status":
		return s.wireguardStatus(ctx, WireGuardStatus{}), nil
	case "cluster.apply":
		return s.clusterApply(ctx)
	case "cluster.status":
		return s.clusterStatus(ctx), nil
	case "cluster.notify":
		var a struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
		return nil, s.clusterNotify(ctx, a.State)
	case "cluster.promote":
		// Runs after the answer: promotion restarts the portal that asked for it.
		go func() { _ = s.promote(context.Background(), "manual") }()
		return map[string]string{"state": "promoting"}, nil
	case "cluster.release":
		go func() { _ = s.demote(context.Background(), "switch-over", true) }()
		return map[string]string{"state": "releasing"}, nil
	case "portal.restart":
		go func() {
			time.Sleep(500 * time.Millisecond)
			_, _ = runCmd(context.Background(), "systemctl", "restart", "kiestoon.service")
		}()
		return nil, nil
	}
	return nil, fmt.Errorf("unknown command %q", req.Cmd)
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(out.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return out.String(), fmt.Errorf("%s: %v: %s", name, err, msg)
	}
	return out.String(), nil
}

// ---- firewall ---------------------------------------------------------------

func (s *Server) persisted() string { return filepath.Join(s.StateDir, "firewall.nft") }

// geo reads the country networks for geo-blocking from the database the portal downloaded.
// Without data no country filter is applied (and the portal shows a warning).
func (s *Server) geo(c firewall.Config) *firewall.Geo {
	if c.GeoMode == "" || c.GeoMode == "off" || len(c.GeoCountries) == 0 {
		return nil
	}
	sets, err := geoip.Load(geoip.Path(s.DataDir), c.GeoCountries)
	if err != nil {
		s.Log.Warn("geo-blocking: no country data", "err", err)
		return nil
	}
	return &firewall.Geo{V4: sets.V4, V6: sets.V6}
}

func (s *Server) liveRuleset(ctx context.Context) string {
	out, err := runCmd(ctx, "nft", "list", "table", "inet", "kiestoon")
	if err != nil {
		return ""
	}
	return out
}

func (s *Server) loadRuleset(ctx context.Context, text string) error {
	f := filepath.Join(s.StateDir, "apply.nft")
	if err := os.WriteFile(f, []byte(text), 0o600); err != nil {
		return err
	}
	_, err := runCmd(ctx, "nft", "-f", f)
	return err
}

func (s *Server) removeTable(ctx context.Context) {
	_, _ = runCmd(ctx, "nft", "delete", "table", "inet", "kiestoon")
}

func (s *Server) firewallApply(ctx context.Context, a FirewallArgs) (any, error) {
	s.fwMu.Lock()
	defer s.fwMu.Unlock()
	if s.fwPending != nil {
		s.fwPending.Stop()
		s.fwPending = nil
	} else {
		s.fwPrevious = s.liveRuleset(ctx)
	}
	s.fwRolled = false
	if !a.Config.Enabled {
		s.removeTable(ctx)
		_ = os.Remove(s.persisted())
		return FirewallStatus{}, nil
	}
	if a.Ports.SIP == 0 || len(a.Ports.Portal) == 0 || a.Ports.RTPStart == 0 {
		return nil, errors.New("invalid ports")
	}
	text, err := firewall.Ruleset(a.Config, a.Ports, a.TrunkIPs, s.geo(a.Config))
	if err != nil {
		return nil, err
	}
	if err := s.loadRuleset(ctx, text); err != nil {
		return nil, err
	}
	if a.ConfirmSeconds <= 0 {
		return FirewallStatus{Active: true}, os.WriteFile(s.persisted(), []byte(text), 0o600)
	}
	if a.ConfirmSeconds > 600 {
		a.ConfirmSeconds = 600
	}
	_ = os.WriteFile(s.persisted()+".pending", []byte(text), 0o600)
	s.fwDeadline = time.Now().Add(time.Duration(a.ConfirmSeconds) * time.Second)
	prev := s.fwPrevious
	s.fwPending = time.AfterFunc(time.Duration(a.ConfirmSeconds)*time.Second, func() {
		s.fwMu.Lock()
		defer s.fwMu.Unlock()
		bg := context.Background()
		s.Log.Warn("firewall change not confirmed: rolling back")
		if prev == "" {
			s.removeTable(bg)
		} else if err := s.loadRuleset(bg, "table inet kiestoon\ndelete table inet kiestoon\n"+prev); err != nil {
			s.Log.Error("firewall rollback failed, removing table", "err", err)
			s.removeTable(bg)
		}
		_ = os.Remove(s.persisted() + ".pending")
		s.fwPending = nil
		s.fwRolled = true
	})
	return FirewallStatus{Active: true, Pending: true, SecondsLeft: a.ConfirmSeconds}, nil
}

func (s *Server) firewallConfirm(ctx context.Context) (any, error) {
	s.fwMu.Lock()
	defer s.fwMu.Unlock()
	if s.fwPending == nil {
		return nil, errors.New("nothing to confirm")
	}
	s.fwPending.Stop()
	s.fwPending = nil
	if err := os.Rename(s.persisted()+".pending", s.persisted()); err != nil {
		return nil, err
	}
	return FirewallStatus{Active: true}, nil
}

func (s *Server) firewallStatus(ctx context.Context) FirewallStatus {
	s.fwMu.Lock()
	defer s.fwMu.Unlock()
	st := FirewallStatus{Ruleset: s.liveRuleset(ctx), RolledBack: s.fwRolled}
	st.Active = st.Ruleset != ""
	if s.fwPending != nil {
		st.Pending = true
		st.SecondsLeft = max(0, int(time.Until(s.fwDeadline).Seconds()))
	}
	return st
}

func (s *Server) restoreFirewallAtBoot(ctx context.Context) {
	_ = os.Remove(s.persisted() + ".pending")
	b, err := os.ReadFile(s.persisted())
	if err != nil {
		return
	}
	if err := s.loadRuleset(ctx, string(b)); err != nil {
		s.Log.Error("could not load the saved firewall", "err", err)
	}
}

// ---- fail2ban ---------------------------------------------------------------

var rePorts = regexp.MustCompile(`^[0-9]{1,5}(,[0-9]{1,5}){0,9}$`)

func (s *Server) fail2banApply(ctx context.Context, a Fail2banArgs) error {
	if err := a.Config.Validate(); err != nil {
		return err
	}
	if !rePorts.MatchString(a.SIPPorts) || !rePorts.MatchString(a.PortalPorts) {
		return errors.New("invalid ports")
	}
	// fail2ban refuses to start a jail whose log file does not exist yet.
	for _, f := range []string{"/var/log/asterisk/security.log", "/var/log/asterisk/messages.log"} {
		if _, err := os.Stat(f); err != nil {
			_ = os.MkdirAll(filepath.Dir(f), 0o750)
			_ = os.WriteFile(f, nil, 0o640)
			if u, err := user.Lookup("asterisk"); err == nil {
				uid, _ := strconv.Atoi(u.Uid)
				gid, _ := strconv.Atoi(u.Gid)
				_ = os.Chown(f, uid, gid)
			}
		}
	}
	if err := os.WriteFile("/etc/fail2ban/jail.d/kiestoon.local", []byte(firewall.Jail(a.Config, a.SIPPorts, a.PortalPorts)), 0o644); err != nil {
		return err
	}
	if _, err := runCmd(ctx, "systemctl", "enable", "--now", "fail2ban.service"); err != nil {
		return err
	}
	_, err := runCmd(ctx, "systemctl", "reload-or-restart", "fail2ban.service")
	return err
}

var reBanned = regexp.MustCompile(`Banned IP list:\s*(.*)`)

func (s *Server) fail2banBans(ctx context.Context) ([]Ban, error) {
	out := []Ban{}
	for _, j := range strings.Fields(jails) {
		res, err := runCmd(ctx, "fail2ban-client", "status", j)
		if err != nil {
			continue
		}
		if m := reBanned.FindStringSubmatch(res); m != nil {
			for _, ip := range strings.Fields(m[1]) {
				out = append(out, Ban{Jail: j, IP: ip})
			}
		}
	}
	return out, nil
}

// ---- backups ----------------------------------------------------------------

func (s *Server) backupCmd(ctx context.Context, cmd string, a BackupArgs) (any, error) {
	if cmd == "backup.restore" {
		if !s.jobMu.TryLock() {
			return nil, errors.New("backup_busy")
		}
		go func() {
			defer s.jobMu.Unlock()
			s.restore(a)
		}()
		return map[string]string{"state": "running"}, nil
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()
	env, err := s.Runner.Env(cctx, a.Target)
	if err != nil {
		return nil, err
	}
	switch cmd {
	case "backup.test":
		created, err := s.Runner.EnsureRepo(cctx, env)
		return map[string]bool{"created": created}, err
	case "backup.snapshots":
		return s.Runner.Snapshots(cctx, env)
	}
	if a.Kind != "full" && a.Kind != "incremental" {
		return nil, errors.New("invalid kind")
	}
	if _, err := s.Runner.EnsureRepo(cctx, env); err != nil {
		return nil, err
	}
	res, err := s.Runner.Backup(cctx, env, a.Kind)
	if err != nil {
		return nil, err
	}
	if err := s.Runner.Forget(cctx, env, a.Retention); err != nil {
		s.Log.Warn("backup retention failed", "err", err)
	}
	return res, nil
}

func (s *Server) writeRestoreStatus(st RestoreStatus) {
	st.At = time.Now().UTC()
	b, _ := json.Marshal(st)
	p := filepath.Join(s.DataDir, "restore-status.json")
	_ = os.WriteFile(p, b, 0o640)
	if u, err := user.Lookup("kiestoon"); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown(p, uid, gid)
	}
}

func (s *Server) restore(a BackupArgs) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	snap := a.Snapshot
	fail := func(err error) {
		s.Log.Error("restore failed", "err", err)
		s.writeRestoreStatus(RestoreStatus{State: "failed", Snapshot: snap, Message: err.Error()})
	}
	s.writeRestoreStatus(RestoreStatus{State: "running", Snapshot: snap})
	env, err := s.Runner.Env(ctx, a.Target)
	if err != nil {
		fail(err)
		return
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	work := filepath.Join("/var/lib/kiestoon-backups", "restore-"+stamp)
	if err := os.MkdirAll(work, 0o700); err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(work)
	if err := s.Runner.RestoreTo(ctx, env, snap, work); err != nil {
		fail(err)
		return
	}
	restored := filepath.Join(work, strings.TrimPrefix(s.DataDir, "/"))
	db := filepath.Join(restored, backup.StagingDir, "kiestoon.db")
	if _, err := os.Stat(db); err != nil {
		fail(errors.New("the backup does not contain a database"))
		return
	}
	_, _ = runCmd(ctx, "systemctl", "stop", "kiestoon.service", "asterisk.service")
	old := filepath.Join("/var/lib/kiestoon-backups", "pre-restore-"+stamp)
	if err := os.Rename(s.DataDir, old); err != nil {
		_, _ = runCmd(ctx, "systemctl", "start", "asterisk.service", "kiestoon.service")
		fail(err)
		return
	}
	if err := os.Rename(restored, s.DataDir); err != nil {
		_ = os.Rename(old, s.DataDir)
		_, _ = runCmd(ctx, "systemctl", "start", "asterisk.service", "kiestoon.service")
		fail(err)
		return
	}
	_ = os.Rename(filepath.Join(s.DataDir, backup.StagingDir, "kiestoon.db"), filepath.Join(s.DataDir, "kiestoon.db"))
	if env, err := os.ReadFile(filepath.Join(work, "etc/kiestoon/kiestoon.env")); err == nil {
		_ = os.WriteFile("/etc/kiestoon/kiestoon.env", env, 0o640)
	}
	// User and group IDs can differ on a new machine: set the ownership again.
	if _, err := runCmd(ctx, "/usr/lib/kiestoon/fix-permissions"); err != nil {
		s.Log.Warn("fix permissions", "err", err)
	}
	s.writeRestoreStatus(RestoreStatus{State: "success", Snapshot: snap, Message: "previous data kept in " + old})
	_, _ = runCmd(ctx, "systemctl", "start", "asterisk.service", "kiestoon.service")
	s.Log.Info("restore finished", "snapshot", snap)
}
