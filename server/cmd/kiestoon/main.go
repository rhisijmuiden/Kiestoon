// Command kiestoon runs the Kiestoon web portal and API.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/api"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
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
	default:
		fmt.Fprintf(os.Stderr, "usage: kiestoon [serve|version|setup-token]\n")
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
	mux.Handle("/", web.Handler())

	hs := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.SecurityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("kiestoon listening", "addr", cfg.Listen, "version", version.Version)
		errc <- hs.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return hs.Shutdown(shutdownCtx)
}
