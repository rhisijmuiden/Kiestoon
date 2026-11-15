package helper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/internet"
)

// InternetArgs turns internet mode (Caddy) on or off.
type InternetArgs struct {
	Settings internet.Settings `json:"settings"`
	// Upstream is the portal address Caddy forwards to, for example 127.0.0.1:8080.
	Upstream string `json:"upstream"`
}

// InternetStatus is the live state of Caddy and its certificate.
type InternetStatus struct {
	Installed   bool        `json:"installed"`
	Active      bool        `json:"active"`
	Certificate *certs.Info `json:"certificate"`
	// PortalCertificate is the certificate of the employee portal host name, when there is one.
	PortalCertificate *certs.Info `json:"portalCertificate"`
	// CopiedForSIP: the certificate was copied to SIP over TLS and port 8443 (changed this call).
	Copied bool `json:"copied"`
}

const (
	caddyfilePath = "/etc/caddy/Caddyfile"
	caddyStorage  = "/var/lib/caddy/.local/share/caddy/certificates"
)

func caddyInstalled() bool {
	_, err := exec.LookPath("caddy")
	return err == nil
}

func (s *Server) internetApply(ctx context.Context, a InternetArgs) (any, error) {
	if !caddyInstalled() {
		if !a.Settings.Enabled {
			return InternetStatus{}, nil
		}
		return nil, errors.New("caddy_missing")
	}
	text, err := internet.Caddyfile(a.Settings, a.Upstream)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(caddyfilePath), 0o755); err != nil {
		return nil, err
	}
	tmp := caddyfilePath + ".kiestoon"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return nil, err
	}
	if a.Settings.Enabled {
		if _, err := runCmd(ctx, "caddy", "validate", "--adapter", "caddyfile", "--config", tmp); err != nil {
			os.Remove(tmp)
			return nil, err
		}
	}
	if err := os.Rename(tmp, caddyfilePath); err != nil {
		return nil, err
	}
	if a.Settings.Enabled {
		_, _ = runCmd(ctx, "systemctl", "enable", "caddy.service")
		if _, err := runCmd(ctx, "systemctl", "restart", "caddy.service"); err != nil {
			return nil, err
		}
	} else {
		_, _ = runCmd(ctx, "systemctl", "disable", "--now", "caddy.service")
		_ = os.Remove(filepath.Join(s.DataDir, "pbx", "tls", certs.PublicCert))
		_ = os.Remove(filepath.Join(s.DataDir, "pbx", "tls", certs.PublicKey))
	}
	return s.internetStatus(ctx, a.Settings)
}

// caddyCert finds the certificate Caddy obtained for the domain (any ACME issuer directory).
func caddyCert(domain string) (cert, key string) {
	if domain == "" || strings.ContainsAny(domain, "/*?[") {
		return "", ""
	}
	matches, _ := filepath.Glob(filepath.Join(caddyStorage, "*", domain, domain+".crt"))
	var best time.Time
	for _, m := range matches {
		c, err := certs.Read(m)
		if err != nil || !c.NotAfter.After(best) {
			continue
		}
		k := strings.TrimSuffix(m, ".crt") + ".key"
		if _, err := os.Stat(k); err == nil {
			best, cert, key = c.NotAfter, m, k
		}
	}
	return cert, key
}

func (s *Server) internetStatus(ctx context.Context, set internet.Settings) (InternetStatus, error) {
	st := InternetStatus{Installed: caddyInstalled()}
	if !st.Installed {
		return st, nil
	}
	out, _ := runCmd(ctx, "systemctl", "is-active", "caddy.service")
	st.Active = strings.TrimSpace(out) == "active"
	if cp, kp := caddyCert(set.Domain); cp != "" {
		if c, err := certs.Read(cp); err == nil {
			info := certs.Describe(c)
			st.Certificate = &info
		}
		if set.Enabled && set.UseForSIP {
			changed, err := copyCert(cp, kp, filepath.Join(s.DataDir, "pbx", "tls"))
			if err != nil {
				s.Log.Warn("copy certificate", "err", err)
			}
			st.Copied = changed
		}
	}
	if set.PortalDomain != "" {
		if cp, _ := caddyCert(set.PortalDomain); cp != "" {
			if c, err := certs.Read(cp); err == nil {
				info := certs.Describe(c)
				st.PortalCertificate = &info
			}
		}
	}
	if set.Enabled && !set.UseForSIP {
		_ = os.Remove(filepath.Join(s.DataDir, "pbx", "tls", certs.PublicCert))
		_ = os.Remove(filepath.Join(s.DataDir, "pbx", "tls", certs.PublicKey))
	}
	return st, nil
}

// copyCert copies the certificate and key when they differ from the copy in dir. The key is
// readable by the portal and by Asterisk (group asterisk), like the self-signed one.
func copyCert(cert, key, dir string) (bool, error) {
	cb, err := os.ReadFile(cert)
	if err != nil {
		return false, err
	}
	kb, err := os.ReadFile(key)
	if err != nil {
		return false, err
	}
	dstC, dstK := filepath.Join(dir, certs.PublicCert), filepath.Join(dir, certs.PublicKey)
	if old, err := os.ReadFile(dstC); err == nil && string(old) == string(cb) {
		return false, nil
	}
	uid, gid := -1, -1
	if u, err := user.Lookup("kiestoon"); err == nil {
		uid, _ = strconv.Atoi(u.Uid)
	}
	if g, err := user.LookupGroup("asterisk"); err == nil {
		gid, _ = strconv.Atoi(g.Gid)
	}
	write := func(path string, b []byte, mode os.FileMode) error {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, b, mode); err != nil {
			return err
		}
		_ = os.Chown(tmp, uid, gid)
		_ = os.Chmod(tmp, mode)
		return os.Rename(tmp, path)
	}
	// Key first: a certificate on disk means the pair is complete.
	if err := write(dstK, kb, 0o640); err != nil {
		return false, err
	}
	return true, write(dstC, cb, 0o644)
}
