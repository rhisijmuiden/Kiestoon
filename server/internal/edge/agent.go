package edge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// Services the edge runs, by systemd unit.
var Services = []string{"wg-quick@" + Interface, "kamailio", "rtpengine-daemon"}

// PairingFiles are read at start while the edge is not paired, so a Raspberry Pi can be paired
// without a screen: write the code into kiestoon-edge.txt on the boot partition.
var PairingFiles = []string{"/boot/firmware/kiestoon-edge.txt", "/boot/kiestoon-edge.txt", "/etc/kiestoon-edge/pairing.txt"}

// System is everything the agent does to the machine. Tests replace it.
type System interface {
	WriteFile(path string, data []byte, perm os.FileMode) error
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// OS is the real machine.
type OS struct{}

// WriteFile writes atomically.
func (OS) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".kiestoon-tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Run runs a command and returns its combined output.
func (OS) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// Agent is the Kiestoon Edge service.
type Agent struct {
	Dir     string
	Version string
	Log     *slog.Logger
	Sys     System

	mu        sync.Mutex
	state     State
	applied   string // the configuration last written, to skip needless restarts
	lastOK    time.Time
	lastErr   string
	lanAddr   string
	lanNet    string
	started   time.Time
	services  map[string]string
	kickHeart chan struct{}
}

// NewAgent loads the state from dir.
func NewAgent(dir, version string, log *slog.Logger) *Agent {
	return &Agent{Dir: dir, Version: version, Log: log, Sys: OS{}, state: LoadState(dir), started: time.Now(), kickHeart: make(chan struct{}, 1)}
}

// State returns a copy of the current state.
func (a *Agent) State() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// ErrPaired means the edge is already paired; reset it first.
var ErrPaired = errors.New("already_paired")

// Pair connects the edge to the central Kiestoon with a pairing code from its portal.
func (a *Agent) Pair(ctx context.Context, text string) error {
	code, err := multisite.Decode(text, multisite.KindEdge)
	if err != nil {
		return err
	}
	if a.State().Paired {
		return ErrPaired
	}
	u, _ := url.Parse(code.URL)
	lan, lanNet, err := LocalAddress(u.Host)
	if err != nil {
		return fmt.Errorf("no network: %w", err)
	}
	priv, pub, err := wg.NewKey()
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	var resp multisite.EdgePairResponse
	cl := multisite.PinnedClient(code.Fingerprint, 20*time.Second)
	err = multisite.PostJSON(ctx, cl, strings.TrimRight(code.URL, "/")+"/api/edge/pair", "", multisite.EdgePairRequest{
		Secret: code.Secret, PublicKey: pub, Hostname: host, Version: a.Version, LANAddress: lan, LANNet: lanNet,
	}, &resp)
	if err != nil {
		return err
	}
	st := State{Paired: true, SiteID: resp.SiteID, Name: resp.Name, Token: resp.Token, PrivateKey: priv, Address: resp.Address,
		HubAddress: resp.HubAddress, HubPublicKey: resp.HubPublicKey, Endpoint: resp.Endpoint, HubURL: code.URL,
		SIPPort: resp.SIPPort, PortalPort: resp.PortalPort, RTPStart: RTPStart, RTPEnd: RTPEnd, PairedAt: time.Now().UTC()}
	if err := st.validate(); err != nil {
		return err
	}
	if err := SaveState(a.Dir, st); err != nil {
		return err
	}
	a.mu.Lock()
	a.state, a.applied = st, ""
	a.mu.Unlock()
	a.Log.Info("edge: paired", "site", st.Name, "tunnel", st.Address)
	if err := a.Apply(ctx); err != nil {
		a.Log.Warn("edge: apply", "err", err)
	}
	a.kick()
	return nil
}

func (s State) validate() error {
	if !wg.ValidKey(s.HubPublicKey) || !wg.ValidEndpoint(s.Endpoint) || s.Token == "" {
		return errors.New("invalid answer from the central Kiestoon")
	}
	if _, err := netip4(s.HubAddress); err != nil {
		return err
	}
	if _, err := netip4(s.TunnelIP()); err != nil {
		return err
	}
	if s.PortalPort < 1 || s.PortalPort > 65535 || s.SIPPort < 1 || s.SIPPort > 65535 {
		return errors.New("invalid ports")
	}
	return nil
}

func netip4(v string) (net.IP, error) {
	ip := net.ParseIP(v)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("invalid address %q", v)
	}
	return ip, nil
}

// Reset forgets the pairing and stops the relay. The central Kiestoon keeps the site; pair again
// with a new code from its portal.
func (a *Agent) Reset(ctx context.Context) error {
	a.mu.Lock()
	a.state, a.applied = State{}, ""
	a.mu.Unlock()
	if err := os.Remove(statePath(a.Dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, s := range Services {
		_, _ = a.Sys.Run(ctx, "systemctl", "disable", "--now", s)
	}
	_ = os.Remove(filepath.Join("/etc/wireguard", Interface+".conf"))
	return nil
}

// Interface returns the WireGuard interface of the edge.
func (s State) Interface() wg.Interface {
	return wg.Interface{Name: Interface, PrivateKey: s.PrivateKey, Address: s.Address, Peers: []wg.Peer{{
		PublicKey: s.HubPublicKey, AllowedIPs: []string{s.HubAddress + "/32"}, Endpoint: s.Endpoint, Keepalive: 25,
	}}}
}

// proxyConfig describes the relay for the current state and office address.
func (a *Agent) proxyConfig() (ProxyConfig, error) {
	st := a.State()
	lan := st.LANAddress
	if lan == "" {
		host, _, _ := net.SplitHostPort(st.Endpoint)
		addr, network, err := LocalAddress(host)
		if err != nil {
			// The name does not resolve right now: the default route leads to the office network too.
			if addr, network, err = LocalAddress("203.0.113.1"); err != nil {
				return ProxyConfig{}, err
			}
		}
		lan = addr
		a.mu.Lock()
		a.lanAddr, a.lanNet = addr, network
		a.mu.Unlock()
	} else {
		a.mu.Lock()
		a.lanAddr, a.lanNet = lan, networkOfString(lan)
		a.mu.Unlock()
	}
	c := ProxyConfig{LANAddress: lan, TunnelIP: st.TunnelIP(), HubIP: st.HubAddress, HubSIPPort: st.SIPPort, RTPStart: st.RTPStart, RTPEnd: st.RTPEnd}
	if c.RTPStart == 0 {
		c.RTPStart, c.RTPEnd = RTPStart, RTPEnd
	}
	return c, c.Validate()
}

func networkOfString(v string) string {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return ""
	}
	return networkOf(a)
}

// Apply writes the tunnel, Kamailio and rtpengine configuration and (re)starts what changed.
func (a *Agent) Apply(ctx context.Context) error {
	st := a.State()
	if !st.Paired {
		return nil
	}
	iface := st.Interface()
	if err := iface.Validate(); err != nil {
		return err
	}
	pc, err := a.proxyConfig()
	if err != nil {
		return err
	}
	wgConf, kam, rtp := iface.Render(true), Kamailio(pc), RTPEngine(pc)
	sum := wgConf + kam + rtp
	a.mu.Lock()
	same := sum == a.applied
	a.mu.Unlock()
	if same {
		return nil
	}
	tunnelChanged := !fileEquals("/etc/wireguard/"+Interface+".conf", wgConf)
	if err := a.Sys.WriteFile("/etc/wireguard/"+Interface+".conf", []byte(wgConf), 0o600); err != nil {
		return err
	}
	if tunnelChanged || !a.active(ctx, Services[0]) {
		_, _ = a.Sys.Run(ctx, "systemctl", "enable", Services[0])
		if out, err := a.Sys.Run(ctx, "systemctl", "restart", Services[0]); err != nil {
			return fmt.Errorf("tunnel: %v %s", err, strings.TrimSpace(out))
		}
	}
	// Check the Kamailio configuration before it replaces a working one.
	tmp := filepath.Join(a.Dir, "kamailio.cfg.check")
	if err := a.Sys.WriteFile(tmp, []byte(kam), 0o640); err != nil {
		return err
	}
	if out, err := a.Sys.Run(ctx, "kamailio", "-c", "-f", tmp); err != nil {
		return fmt.Errorf("kamailio configuration: %v %s", err, lastLine(out))
	}
	_ = os.Remove(tmp)
	if err := a.Sys.WriteFile("/etc/kamailio/kamailio.cfg", []byte(kam), 0o644); err != nil {
		return err
	}
	if err := a.Sys.WriteFile("/etc/rtpengine/rtpengine.conf", []byte(rtp), 0o644); err != nil {
		return err
	}
	for _, s := range Services[1:] {
		_, _ = a.Sys.Run(ctx, "systemctl", "enable", s)
		if out, err := a.Sys.Run(ctx, "systemctl", "restart", s); err != nil {
			return fmt.Errorf("%s: %v %s", s, err, strings.TrimSpace(out))
		}
	}
	a.mu.Lock()
	a.applied = sum
	a.mu.Unlock()
	a.Log.Info("edge: configuration applied", "lan", pc.LANAddress, "tunnel", pc.TunnelIP)
	return nil
}

func fileEquals(path, content string) bool {
	b, err := os.ReadFile(path)
	return err == nil && string(b) == content
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func (a *Agent) active(ctx context.Context, unit string) bool {
	out, _ := a.Sys.Run(ctx, "systemctl", "is-active", unit)
	return strings.TrimSpace(out) == "active"
}

func (a *Agent) kick() {
	select {
	case a.kickHeart <- struct{}{}:
	default:
	}
}

// Run keeps the edge going: headless pairing, configuration, heartbeats.
func (a *Agent) Run(ctx context.Context) {
	if !a.State().Paired {
		a.pairFromFile(ctx)
	}
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if a.State().Paired {
			if err := a.Apply(ctx); err != nil {
				a.setErr(err)
				a.Log.Warn("edge: apply", "err", err)
			}
			a.heartbeat(ctx)
		} else {
			a.pairFromFile(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-a.kickHeart:
		}
	}
}

func (a *Agent) setErr(err error) {
	a.mu.Lock()
	a.lastErr = err.Error()
	a.mu.Unlock()
}

// pairFromFile pairs with a code from the boot partition and removes the file (it holds a secret).
func (a *Agent) pairFromFile(ctx context.Context) {
	for _, f := range PairingFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		code := ""
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); strings.HasPrefix(l, "KT1-") {
				code = l
			}
		}
		if code == "" {
			continue
		}
		if err := a.Pair(ctx, code); err != nil {
			a.setErr(err)
			a.Log.Warn("edge: pairing from file", "file", f, "err", err)
			if !errors.Is(err, multisite.ErrInvalidCode) {
				return // try again later; network may not be up yet
			}
		}
		_ = os.Remove(f)
		return
	}
}

func (a *Agent) heartbeat(ctx context.Context) {
	st := a.State()
	services := map[string]string{}
	for _, s := range Services {
		out, _ := a.Sys.Run(ctx, "systemctl", "is-active", s)
		services[s] = strings.TrimSpace(out)
	}
	host, _ := os.Hostname()
	a.mu.Lock()
	a.services = services
	lan, lanNet := a.lanAddr, a.lanNet
	a.mu.Unlock()
	var cfg multisite.EdgeConfig
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	u := "http://" + net.JoinHostPort(st.HubAddress, strconv.Itoa(st.PortalPort)) + "/api/edge/heartbeat"
	err := multisite.PostJSON(cctx, &http.Client{Timeout: 15 * time.Second}, u, st.Token, multisite.EdgeHeartbeat{
		Version: a.Version, Hostname: host, LANAddress: lan, LANNet: lanNet, Uptime: int64(time.Since(a.started).Seconds()), Services: services,
	}, &cfg)
	if err != nil {
		a.setErr(err)
		return
	}
	a.mu.Lock()
	a.lastOK, a.lastErr = time.Now(), ""
	changed := false
	if cfg.Name != "" && cfg.Name != a.state.Name {
		a.state.Name, changed = cfg.Name, true
	}
	if cfg.SIPPort > 0 && cfg.SIPPort != a.state.SIPPort {
		a.state.SIPPort, changed = cfg.SIPPort, true
	}
	if cfg.RTPStart >= 1024 && cfg.RTPEnd > cfg.RTPStart && (cfg.RTPStart != a.state.RTPStart || cfg.RTPEnd != a.state.RTPEnd) {
		a.state.RTPStart, a.state.RTPEnd, changed = cfg.RTPStart, cfg.RTPEnd, true
	}
	st = a.state
	a.mu.Unlock()
	if changed {
		_ = SaveState(a.Dir, st)
		if err := a.Apply(ctx); err != nil {
			a.setErr(err)
		}
	}
}

// Status is what the local page shows.
type Status struct {
	Version     string            `json:"version"`
	Hostname    string            `json:"hostname"`
	Paired      bool              `json:"paired"`
	Name        string            `json:"name,omitempty"`
	HubURL      string            `json:"hubUrl,omitempty"`
	TunnelIP    string            `json:"tunnelIp,omitempty"`
	HubAddress  string            `json:"hubAddress,omitempty"`
	LANAddress  string            `json:"lanAddress,omitempty"`
	LANNet      string            `json:"lanNet,omitempty"`
	Connected   bool              `json:"connected"`
	LastContact time.Time         `json:"lastContact,omitempty"`
	LastError   string            `json:"lastError,omitempty"`
	Services    map[string]string `json:"services,omitempty"`
	PairedAt    time.Time         `json:"pairedAt,omitempty"`
}

// Status reports the state of the edge.
func (a *Agent) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	host, _ := os.Hostname()
	st := a.state
	return Status{Version: a.Version, Hostname: host, Paired: st.Paired, Name: st.Name, HubURL: st.HubURL, TunnelIP: st.TunnelIP(),
		HubAddress: st.HubAddress, LANAddress: a.lanAddr, LANNet: a.lanNet, Connected: !a.lastOK.IsZero() && time.Since(a.lastOK) < 2*time.Minute,
		LastContact: a.lastOK, LastError: a.lastErr, Services: a.services, PairedAt: st.PairedAt}
}

// ProvisioningProxy forwards /prov/ requests of the office phones to the central Kiestoon. The Host
// header is kept, so the configuration it hands out names this edge as SIP server.
func (a *Agent) ProvisioningProxy() http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			st := a.State()
			r.SetURL(&url.URL{Scheme: "http", Host: net.JoinHostPort(st.HubAddress, strconv.Itoa(st.PortalPort))})
			r.Out.Host = r.In.Host
			r.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "central Kiestoon not reachable", http.StatusBadGateway)
		},
	}
}
