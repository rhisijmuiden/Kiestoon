package helper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// WireGuardArgs is the complete set of Kiestoon tunnels. Interfaces that are not in the list and
// were written by Kiestoon are taken down and removed.
type WireGuardArgs struct {
	Interfaces []wg.Interface `json:"interfaces"`
}

// WireGuardStatus is the live state of the tunnels.
type WireGuardStatus struct {
	Installed bool            `json:"installed"`
	Up        []string        `json:"up"`
	Peers     []wg.PeerStatus `json:"peers"`
	Errors    []string        `json:"errors"`
}

const wgDir = "/etc/wireguard"

func wgInstalled() bool {
	_, err := exec.LookPath("wg-quick")
	return err == nil
}

func ownConfig(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.HasPrefix(string(b), wg.Marker)
}

func ifaceUp(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", name))
	return err == nil
}

func (s *Server) wireguardApply(ctx context.Context, a WireGuardArgs) (any, error) {
	if len(a.Interfaces) > 0 && !wgInstalled() {
		return nil, errors.New("wireguard_missing")
	}
	want := map[string]bool{}
	for i := range a.Interfaces {
		if err := a.Interfaces[i].Validate(); err != nil {
			return nil, err
		}
		if want[a.Interfaces[i].Name] {
			return nil, errors.New("duplicate interface")
		}
		want[a.Interfaces[i].Name] = true
	}
	if err := os.MkdirAll(wgDir, 0o700); err != nil {
		return nil, err
	}
	st := WireGuardStatus{Installed: wgInstalled()}
	// Remove tunnels that are no longer wanted.
	if entries, err := os.ReadDir(wgDir); err == nil {
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".conf")
			path := filepath.Join(wgDir, e.Name())
			if !strings.HasSuffix(e.Name(), ".conf") || !strings.HasPrefix(name, "kt") || want[name] || !ownConfig(path) {
				continue
			}
			_, _ = runCmd(ctx, "systemctl", "disable", "--now", "wg-quick@"+name+".service")
			_ = os.Remove(path)
		}
	}
	for _, i := range a.Interfaces {
		path := filepath.Join(wgDir, i.Name+".conf")
		text := i.Render(true)
		old, _ := os.ReadFile(path)
		if err := os.WriteFile(path+".tmp", []byte(text), 0o600); err != nil {
			return nil, err
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			return nil, err
		}
		_, _ = runCmd(ctx, "systemctl", "enable", "wg-quick@"+i.Name+".service")
		var err error
		switch {
		case !ifaceUp(i.Name):
			_, err = runCmd(ctx, "systemctl", "restart", "wg-quick@"+i.Name+".service")
		case string(old) == text:
			// Unchanged and up.
		case addressLine(string(old)) != addressLine(text):
			_, err = runCmd(ctx, "systemctl", "restart", "wg-quick@"+i.Name+".service")
		default:
			// Same address: change the peers without dropping the other tunnels on the interface.
			sync := filepath.Join(s.StateDir, i.Name+".sync")
			if err = os.WriteFile(sync, []byte(i.Render(false)), 0o600); err == nil {
				_, err = runCmd(ctx, "wg", "syncconf", i.Name, sync)
				_ = os.Remove(sync)
			}
		}
		if err != nil {
			st.Errors = append(st.Errors, i.Name+": "+err.Error())
		}
	}
	return s.wireguardStatus(ctx, st), nil
}

func addressLine(conf string) string {
	for _, l := range strings.Split(conf, "\n") {
		if strings.HasPrefix(l, "Address") {
			return l
		}
	}
	return ""
}

func (s *Server) wireguardStatus(ctx context.Context, st WireGuardStatus) WireGuardStatus {
	st.Installed = wgInstalled()
	st.Up = []string{}
	st.Peers = []wg.PeerStatus{}
	if !st.Installed {
		return st
	}
	if out, err := runCmd(ctx, "wg", "show", "all", "dump"); err == nil {
		st.Peers = wg.ParseDump(out)
		seen := map[string]bool{}
		for _, l := range strings.Split(out, "\n") {
			f := strings.Split(l, "\t")
			if len(f) == 5 && strings.HasPrefix(f[0], "kt") && !seen[f[0]] {
				seen[f[0]] = true
				st.Up = append(st.Up, f[0])
			}
		}
	}
	return st
}
