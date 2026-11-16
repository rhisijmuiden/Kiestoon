// Command kiestoon-push runs the push relay of the Kiestoon mobile app: it forwards wake-up and
// voicemail notifications from Kiestoon servers to Apple and Google. See internal/push and the
// manual (mobile-app.md) for how it works and how to run your own.
//
// Configuration (environment):
//
//	KIESTOON_PUSH_LISTEN        address to listen on (default 127.0.0.1:8095; put Caddy or another
//	                            HTTPS proxy in front)
//	KIESTOON_PUSH_SECRET_FILE   file with the secret that seals handles (made on first start;
//	                            default /var/lib/kiestoon-push/secret)
//	KIESTOON_PUSH_APNS_KEY      .p8 key file of the Apple developer account
//	KIESTOON_PUSH_APNS_KEY_ID   its key id
//	KIESTOON_PUSH_APNS_TEAM_ID  the team id
//	KIESTOON_PUSH_APNS_BUNDLE   bundle id of the app (default nl.rhis.kiestoon)
//	KIESTOON_PUSH_FCM_CREDENTIALS  Firebase service account file (JSON)
//	KIESTOON_PUSH_LOG_ONLY      1: platforms without credentials only log (for a test setup)
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/push"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Printf("kiestoon-push %s (commit %s)\n", version.Version, version.Commit)
		return
	}
	if err := run(log); err != nil {
		log.Error("kiestoon-push", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// secret reads the handle secret, or makes one on the first start.
func secret(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) < 32 {
			return nil, errors.New("secret file is shorter than 32 bytes")
		}
		return b, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	b = make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return b, os.WriteFile(path, b, 0o600)
}

func run(log *slog.Logger) error {
	sec, err := secret(env("KIESTOON_PUSH_SECRET_FILE", "/var/lib/kiestoon-push/secret"))
	if err != nil {
		return fmt.Errorf("secret: %w", err)
	}
	senders := map[string]push.Sender{}
	if p := env("KIESTOON_PUSH_APNS_KEY", ""); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		k, err := push.ParseP8(raw)
		if err != nil {
			return err
		}
		a := &push.APNsSender{TeamID: env("KIESTOON_PUSH_APNS_TEAM_ID", ""), KeyID: env("KIESTOON_PUSH_APNS_KEY_ID", ""),
			Key: k, Bundle: env("KIESTOON_PUSH_APNS_BUNDLE", "nl.rhis.kiestoon")}
		senders[push.APNs], senders[push.APNsVoIP] = a, a
		log.Info("apns enabled")
	}
	if p := env("KIESTOON_PUSH_FCM_CREDENTIALS", ""); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := push.NewFCMSender(raw)
		if err != nil {
			return err
		}
		senders[push.FCM] = f
		log.Info("fcm enabled")
	}
	if env("KIESTOON_PUSH_LOG_ONLY", "") == "1" {
		for _, p := range []string{push.APNs, push.APNsVoIP, push.FCM} {
			if senders[p] == nil {
				senders[p] = push.LogSender{Platform: p, Log: log}
			}
		}
	}
	if len(senders) == 0 {
		return errors.New("no platform configured: set KIESTOON_PUSH_APNS_KEY and/or KIESTOON_PUSH_FCM_CREDENTIALS (or KIESTOON_PUSH_LOG_ONLY=1)")
	}
	rl := &push.Relay{Secret: sec, Senders: senders, Log: log}
	srv := &http.Server{Addr: env("KIESTOON_PUSH_LISTEN", "127.0.0.1:8095"), Handler: rl.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	log.Info("kiestoon-push listening", "addr", srv.Addr, "version", version.Version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
