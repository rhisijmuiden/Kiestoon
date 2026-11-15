package asterisk

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Manager regenerates the Asterisk configuration after every change and reloads Asterisk.
type Manager struct {
	Dir     string
	Repo    *pbx.Repo
	AMI     *AMI
	Log     *slog.Logger
	Options func(ctx context.Context) Options

	mu            sync.Mutex
	kick          chan struct{}
	lastApply     time.Time
	lastErr       string
	restartNeeded bool
	restartSince  time.Time
}

// Status is shown in the UI.
type Status struct {
	Connected     bool      `json:"connected"`
	Version       string    `json:"version"`
	LastApply     time.Time `json:"lastApply"`
	LastError     string    `json:"lastError"`
	RestartNeeded bool      `json:"restartNeeded"`
}

// Status returns the current state of the Asterisk connection and configuration.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	st := Status{LastApply: m.lastApply, LastError: m.lastErr, RestartNeeded: m.restartNeeded}
	m.mu.Unlock()
	st.Connected = m.AMI.Connected()
	if st.Connected {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if r, err := m.AMI.Action(cctx, Message{{"Action", "CoreSettings"}}); err == nil {
			st.Version = r.Get("AsteriskVersion")
		}
	}
	return st
}

// Changed asks for a regeneration soon. Many quick changes cause one reload.
func (m *Manager) Changed() {
	m.mu.Lock()
	if m.kick == nil {
		m.kick = make(chan struct{}, 1)
	}
	k := m.kick
	m.mu.Unlock()
	select {
	case k <- struct{}{}:
	default:
	}
}

// Run applies the configuration at start and after every change until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	if m.kick == nil {
		m.kick = make(chan struct{}, 1)
	}
	k := m.kick
	m.mu.Unlock()
	if err := m.Apply(ctx); err != nil {
		m.Log.Warn("asterisk configuration", "err", err)
	}
	wasConnected := false
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-k:
			// Debounce: wait for a burst of changes to settle.
			select {
			case <-time.After(400 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			if err := m.Apply(ctx); err != nil {
				m.Log.Warn("asterisk configuration", "err", err)
			}
		case <-tick.C:
			// After Asterisk (re)starts it reads the files itself; clear "restart needed" then.
			c := m.AMI.Connected()
			if c && !wasConnected {
				m.checkRestarted(ctx)
			}
			wasConnected = c
		}
	}
}

func (m *Manager) checkRestarted(ctx context.Context) {
	m.mu.Lock()
	need, since := m.restartNeeded, m.restartSince
	m.mu.Unlock()
	if !need {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	r, err := m.AMI.Action(cctx, Message{{"Action", "CoreStatus"}})
	if err != nil {
		return
	}
	started, err := time.ParseInLocation("2006-01-02 15:04:05", r.Get("CoreStartupDate")+" "+r.Get("CoreStartupTime"), time.Local)
	if err == nil && started.After(since.Add(-2*time.Second)) {
		m.mu.Lock()
		m.restartNeeded = false
		m.mu.Unlock()
	}
}

var reTransport = regexp.MustCompile(`(?ms)^\[transport-[a-z]+\].*?\n\n`)

// Apply generates the files, writes the ones that changed and reloads the matching modules.
func (m *Manager) Apply(ctx context.Context) error {
	snap, err := m.Repo.Snapshot(ctx)
	if err != nil {
		return m.fail(err)
	}
	files := Generate(snap, m.Options(ctx))
	if err := os.MkdirAll(m.Dir, 0o750); err != nil {
		return m.fail(err)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var changed []string
	transportsChanged := false
	for _, name := range names {
		path := filepath.Join(m.Dir, name)
		old, _ := os.ReadFile(path)
		if bytes.Equal(old, []byte(files[name])) {
			continue
		}
		if name == "pjsip.conf" && len(old) > 0 {
			a := reTransport.FindAllString(string(old), -1)
			b := reTransport.FindAllString(files[name], -1)
			transportsChanged = len(a) != len(b) || strconv.Quote(joinAll(a)) != strconv.Quote(joinAll(b))
		}
		if err := writeAtomic(path, []byte(files[name]), 0o640); err != nil {
			return m.fail(err)
		}
		changed = append(changed, name)
	}
	m.mu.Lock()
	m.lastApply = time.Now().UTC()
	m.lastErr = ""
	if transportsChanged {
		m.restartNeeded = true
		m.restartSince = time.Now()
	}
	m.mu.Unlock()
	if len(changed) == 0 || !m.AMI.Connected() {
		return nil
	}
	for _, cmd := range ChangedModules(changed) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := m.AMI.Command(cctx, cmd)
		cancel()
		if err != nil {
			return m.fail(err)
		}
	}
	m.Log.Info("asterisk configuration applied", "files", changed)
	return nil
}

// Restart restarts Asterisk when no calls are active, or right away when now is true.
func (m *Manager) Restart(ctx context.Context, now bool) error {
	cmd := "core restart when convenient"
	if now {
		cmd = "core restart now"
	}
	m.mu.Lock()
	m.restartSince = time.Now()
	m.mu.Unlock()
	_, err := m.AMI.Command(ctx, cmd)
	return err
}

func (m *Manager) fail(err error) error {
	m.mu.Lock()
	m.lastErr = err.Error()
	m.mu.Unlock()
	return err
}

func joinAll(s []string) string {
	var b bytes.Buffer
	for _, x := range s {
		b.WriteString(x)
	}
	return b.String()
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
