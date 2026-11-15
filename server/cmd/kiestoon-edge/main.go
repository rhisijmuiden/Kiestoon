// Command kiestoon-edge runs Kiestoon Edge, the satellite for a branch office.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/edge"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	dir := envOr("KIESTOON_EDGE_DIR", "/var/lib/kiestoon-edge")
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(dir, log)
	case "pair":
		if len(os.Args) < 3 {
			err = errors.New("usage: kiestoon-edge pair <code>")
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err = edge.NewAgent(dir, version.Version, log).Pair(ctx, os.Args[2])
		if err == nil {
			fmt.Println("paired; restart the service: systemctl restart kiestoon-edge")
		}
	case "reset":
		err = edge.NewAgent(dir, version.Version, log).Reset(context.Background())
		if err == nil {
			fmt.Println("pairing removed; restart the service: systemctl restart kiestoon-edge")
		}
	case "status":
		a := edge.NewAgent(dir, version.Version, log)
		b, _ := json.MarshalIndent(a.Status(), "", "  ")
		fmt.Println(string(b))
	case "version", "--version", "-v":
		fmt.Printf("kiestoon-edge %s (commit %s, built %s)\n", version.Version, version.Commit, version.BuildDate)
	default:
		fmt.Fprintln(os.Stderr, "usage: kiestoon-edge [serve|pair <code>|reset|status|version]")
		os.Exit(2)
	}
	if err != nil {
		log.Error("kiestoon-edge", "err", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func serve(dir string, log *slog.Logger) error {
	if os.Geteuid() != 0 {
		return errors.New("kiestoon-edge must run as root (it manages WireGuard, Kamailio and rtpengine)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	a := edge.NewAgent(dir, version.Version, log)
	go a.Run(ctx)
	srv := a.Server(envOr("KIESTOON_EDGE_LISTEN", ":80"))
	errc := make(chan error, 1)
	go func() {
		log.Info("kiestoon-edge listening", "addr", srv.Addr, "version", version.Version)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(sctx)
}
