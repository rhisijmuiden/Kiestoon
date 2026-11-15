// Command kiestoon runs the Kiestoon web portal and API.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/api"
	"github.com/rhisijmuiden/kiestoon/server/internal/backup"
	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
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
	case "setup-token":
		err = printSetupToken()
	case "helper":
		err = runHelper(log)
	case "sendmail":
		err = sendmail()
	default:
		fmt.Fprintf(os.Stderr, "usage: kiestoon [serve|version|setup-token|helper|sendmail]\n")
		os.Exit(2)
	}
	if err != nil {
		log.Error("kiestoon stopped", "err", err)
		os.Exit(1)
	}
}

// ensureSetupToken creates the one-time token the setup wizard asks for while no admin exists.
func ensureSetupToken(ctx context.Context, cfg config.Config, srv *api.Server, log *slog.Logger) {
	n, err := srv.Auth.UserCount(ctx)
	if err != nil || n > 0 {
		return
	}
	path := api.SetupTokenPath(cfg.DataDir)
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		log.Info("setup required: open the portal and enter the setup token (kiestoon setup-token)")
		return
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	tok := strings.ToUpper(hex.EncodeToString(b))
	tok = tok[:4] + "-" + tok[4:8] + "-" + tok[8:]
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		log.Warn("could not write setup token", "err", err)
		return
	}
	log.Info("setup required", "setup_token", tok)
}

func printSetupToken() error {
	cfg := config.Load()
	b, err := os.ReadFile(api.SetupTokenPath(cfg.DataDir))
	if err != nil {
		return errors.New("no setup token: setup is already done, or the service has not started yet")
	}
	fmt.Print(string(b))
	return nil
}

func serve(log *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	srv := api.New(cfg, db, log)
	ensureSetupToken(ctx, cfg, srv, log)
	srv.LoadProxySettings(ctx)

	go srv.AMI.Run(ctx)
	go srv.Ast.Run(ctx)
	go srv.Mail.Run(ctx)
	go srv.RunScheduler(ctx)
	go srv.ApplySecurityOnStart(ctx)
	go srv.RunMaintenance(ctx)
	go srv.RunIntegrations(ctx)
	go srv.RunLDAP(ctx)
	go srv.RunExtensions(ctx)
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
	mux.Handle("GET /metrics", srv.MetricsHandler())
	mux.Handle("/", web.Handler())

	newServer := func(addr string) *http.Server {
		return &http.Server{
			Addr:              addr,
			Handler:           api.SecurityHeaders(mux),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      120 * time.Second,
			IdleTimeout:       120 * time.Second,
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
