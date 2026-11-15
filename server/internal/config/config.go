// Package config reads Kiestoon's runtime settings from the environment.
//
// On an installed system these come from /etc/kiestoon/kiestoon.env, loaded by systemd.
package config

import (
	"os"
	"path/filepath"
)

// Config holds the settings the server needs to start.
type Config struct {
	// Listen is the address the web portal listens on, ":8080" by default.
	Listen string
	// DataDir holds the database, update requests and pre-update backups.
	DataDir string
	// Repo is the GitHub repository ("owner/name") that releases are fetched from.
	Repo string
	// SecureCookies forces the Secure flag on session cookies (set when served over HTTPS).
	SecureCookies bool
	// DefaultChannel is the update channel used until an admin picks one in the UI ("production" or "testing").
	DefaultChannel string
	// TLSListen is the HTTPS address (self-signed certificate) for the portal and phone provisioning. "off" disables it.
	TLSListen string
	// AMIAddr is the Asterisk Manager Interface address.
	AMIAddr string
	// HelperSocket is the Unix socket of the root helper service.
	HelperSocket string
	// AsteriskHTTP is Asterisk's local HTTP server, which carries the WebSocket of the browser softphone.
	AsteriskHTTP string
	// SoftphoneDir holds the Windows softphone installer (MSI) that the portal offers for download.
	SoftphoneDir string
}

// Load reads the configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		Listen:         env("KIESTOON_LISTEN", ":8080"),
		DataDir:        env("KIESTOON_DATA_DIR", "/var/lib/kiestoon"),
		Repo:           env("KIESTOON_REPO", "rhisijmuiden/kiestoon"),
		SecureCookies:  os.Getenv("KIESTOON_SECURE_COOKIES") == "true",
		DefaultChannel: env("KIESTOON_CHANNEL", "production"),
		TLSListen:      env("KIESTOON_TLS_LISTEN", ":8443"),
		AMIAddr:        env("KIESTOON_AMI_ADDR", "127.0.0.1:5038"),
		HelperSocket:   env("KIESTOON_HELPER_SOCKET", "/run/kiestoon-helper/helper.sock"),
		AsteriskHTTP:   env("KIESTOON_ASTERISK_HTTP", "127.0.0.1:8088"),
		SoftphoneDir:   env("KIESTOON_SOFTPHONE_DIR", "/usr/share/kiestoon/softphone"),
	}
}

// DatabasePath is the location of the SQLite database.
func (c Config) DatabasePath() string { return filepath.Join(c.DataDir, "kiestoon.db") }

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// PBXDir holds the generated Asterisk configuration and the TLS certificate.
func (c Config) PBXDir() string { return filepath.Join(c.DataDir, "pbx") }

// SpoolDir is Asterisk's spool directory (voicemail and recordings) inside the data directory.
func (c Config) SpoolDir() string { return filepath.Join(c.DataDir, "asterisk", "spool") }
