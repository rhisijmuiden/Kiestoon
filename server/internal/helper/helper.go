// Package helper is the small root service that does the few things the portal may not do itself:
// apply the firewall, manage fail2ban, run backups and restores, and restart Asterisk.
//
// It listens on a Unix socket that only root and the kiestoon user can open, accepts a fixed set of
// commands with typed arguments, validates everything again and builds every command line itself.
package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// SocketPath is where the helper listens.
const SocketPath = "/run/kiestoon-helper/helper.sock"

// Request is one command for the helper.
type Request struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Response is the helper's answer.
type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// ErrUnavailable means the helper service is not running (for example during development).
var ErrUnavailable = errors.New("helper_unavailable")

// Client talks to the helper.
type Client struct{ Socket string }

// Call sends a command and decodes the result into out (may be nil).
func (c *Client) Call(ctx context.Context, cmd string, args, out any) error {
	sock := c.Socket
	if sock == "" {
		sock = SocketPath
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return ErrUnavailable
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	req := Request{Cmd: cmd}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return err
		}
		req.Args = b
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("helper: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out != nil && len(resp.Data) > 0 {
		return json.Unmarshal(resp.Data, out)
	}
	return nil
}

// Available reports whether the helper answers.
func (c *Client) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.Call(ctx, "ping", nil, nil) == nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
