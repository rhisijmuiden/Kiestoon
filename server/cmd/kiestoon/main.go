// Command kiestoon runs the Kiestoon web portal and API.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/api"
	"github.com/rhisijmuiden/kiestoon/server/internal/backup"
	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
	"github.com/rhisijmuiden/kiestoon/server/internal/helper"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
	"github.com/rhisijmuiden/kiestoon/server/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(log)
	case "version", "--version", "-v":
		fmt.Printf("kiestoon %s (commit %s, built %s)\n", version.Version, version.Commit, version.BuildDate)
	case "initial-admin", "setup-token":
		err = printInitialAdmin()
	case "helper":
		err = runHelper(log)
	case "sendmail":
		err = sendmail()
	case "cluster-notify":
		err = clusterNotify()
	default:
		fmt.Fprintf(os.Stderr, "usage: kiestoon [serve|version|initial-admin|helper|sendmail|cluster-notify]\n")
		os.Exit(2)
	}
	if err != nil {
		log.Error("kiestoon stopped", "err", err)
		os.Exit(1)
	}
}

// printInitialAdmin shows the generated first administrator while the first-run wizard is pending.
func printInitialAdmin() error {
	cfg := config.Load()
	a, ok := api.LoadInitialAdmin(cfg.DataDir)
	if !ok {
		return errors.New("no initial password: the first sign-in is already done (or the service has not started yet)")
	}
	fmt.Printf("Username: %s\nPassword: %s\n", a.Username, a.Password)
	return nil
}

func serve(log *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if cluster.IsStandby(cfg.DataDir) {
		return serveStandby(ctx, cfg, log)
	}

	db, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	srv := api.New(cfg, db, log)
	if err := srv.EnsureInitialAdmin(ctx); err != nil {
		log.Warn("first administrator", "err", err)
	}
	srv.LoadProxySettings(ctx)
	srv.LoadTrustedNetworks(ctx)

	go srv.AMI.Run(ctx)
	go srv.Ast.Run(ctx)
	go srv.Mail.Run(ctx)
	go srv.RunScheduler(ctx)
	go srv.ApplySecurityOnStart(ctx)
	go srv.RunMaintenance(ctx)
	go srv.RunIntegrations(ctx)
	go srv.RunLDAP(ctx)
	go srv.RunExtensions(ctx)
	go srv.RunMultisite(ctx)
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.Auth.PurgeExpiredSessions(ctx)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Routes())
	mux.Handle("/prov/", srv.ProvisioningHandler())
	mux.Handle("/download/", srv.SoftphoneDownload())
	mux.Handle("GET /metrics", srv.MetricsHandler())
	// MCP server for AI assistants, with its OAuth endpoints.
	mcp := srv.MCPHandler()
	for _, p := range []string{"/mcp", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server", "/.well-known/oauth-authorization-server/mcp",
		"/oauth/authorize", "/oauth/token", "/oauth/register", "/oauth/revoke"} {
		mux.Handle(p, mcp)
	}
	mux.Handle("/", web.Handler())

	return listen(ctx, cfg, log, mux)
}

// listen serves the portal on HTTP and HTTPS until ctx ends.
func listen(ctx context.Context, cfg config.Config, log *slog.Logger, mux http.Handler) error {
	newServer := func(addr string) *http.Server {
		return &http.Server{
			Addr:              addr,
			Handler:           api.SecurityHeaders(mux),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			// Long enough for the standby to download a database snapshot or a large recording.
			WriteTimeout: 30 * time.Minute,
			IdleTimeout:  120 * time.Second,
		}
	}
	hs := newServer(cfg.Listen)
	errc := make(chan error, 2)
	go func() {
		log.Info("kiestoon listening", "addr", cfg.Listen, "version", version.Version)
		errc <- hs.ListenAndServe()
	}()
	var tlsSrv *http.Server
	if cfg.TLSListen != "" && cfg.TLSListen != "off" {
		loader := &certs.Loader{Dir: cfg.PBXDir()}
		if _, err := loader.GetCertificate(nil); err != nil {
			log.Warn("no TLS certificate, HTTPS disabled", "err", err)
		} else {
			tlsSrv = newServer(cfg.TLSListen)
			// The certificate is read again when it changes (internet mode copies the Let's Encrypt one).
			tlsSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: loader.GetCertificate}
			go func() {
				log.Info("kiestoon listening (HTTPS)", "addr", cfg.TLSListen)
				if err := tlsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Warn("HTTPS listener stopped", "err", err)
				}
			}()
		}
	}
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if tlsSrv != nil {
		_ = tlsSrv.Shutdown(shutdownCtx)
	}
	return hs.Shutdown(shutdownCtx)
}

// serveStandby runs the small status portal of a standby node and replicates from the primary.
func serveStandby(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	log.Info("kiestoon runs as standby node of a high-availability pair")
	sb := api.NewStandby(cfg, log)
	go sb.Run(ctx)
	mux := http.NewServeMux()
	mux.Handle("/api/", sb.Routes())
	mux.Handle("/", web.Handler())
	return listen(ctx, cfg, log, mux)
}

// clusterNotify is called by keepalived when this node becomes master, backup or faulty.
func clusterNotify() error {
	if len(os.Args) < 3 {
		return errors.New("usage: kiestoon cluster-notify master|backup|fault")
	}
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := &helper.Client{Socket: cfg.HelperSocket}
	return c.Call(ctx, "cluster.notify", map[string]string{"state": os.Args[2]}, nil)
}

// runHelper starts the root helper service (kiestoon-helper.service).
func runHelper(log *slog.Logger) error {
	if os.Geteuid() != 0 {
		return errors.New("the helper must run as root")
	}
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	state := "/var/lib/kiestoon-helper"
	if err := os.MkdirAll(state, 0o700); err != nil {
		return err
	}
	h := &helper.Server{Log: log, StateDir: state, DataDir: cfg.DataDir, Runner: backup.NewRunner()}
	return h.Serve(ctx, cfg.HelperSocket)
}

// sendmail is called by Asterisk (app_voicemail mailcmd) with a complete message on stdin. It
// queues the message; the portal sends it through the configured SMTP server.
func sendmail() error {
	cfg := config.Load()
	msg, err := io.ReadAll(io.LimitReader(os.Stdin, 50<<20))
	if err != nil {
		return err
	}
	return mail.Enqueue(mail.QueueDir(cfg.DataDir), msg)
}
