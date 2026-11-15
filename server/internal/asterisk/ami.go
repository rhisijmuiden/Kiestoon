package asterisk

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// Message is an AMI response or event: an ordered list of "Key: Value" lines.
type Message []KV

// KV is one line of a Message.
type KV struct{ Key, Value string }

// Get returns the first value for key (case-insensitive).
func (m Message) Get(key string) string {
	for _, kv := range m {
		if strings.EqualFold(kv.Key, key) {
			return kv.Value
		}
	}
	return ""
}

// All returns every value for key, for example the Output lines of a Command response.
func (m Message) All(key string) []string {
	var out []string
	for _, kv := range m {
		if strings.EqualFold(kv.Key, key) {
			out = append(out, kv.Value)
		}
	}
	return out
}

// ErrNotConnected is returned while Asterisk is not reachable.
var ErrNotConnected = errors.New("asterisk not connected")

type pending struct {
	resp   Message
	events []Message
	list   bool
	done   chan struct{}
}

// AMI keeps a connection to the Asterisk Manager Interface and reconnects when it drops.
type AMI struct {
	Addr, User, Secret string
	Log                *slog.Logger
	// OnEvent is called for every event that is not part of an action's response list.
	OnEvent func(Message)

	mu        sync.Mutex
	conn      net.Conn
	pending   map[string]*pending
	connected bool
	version   string
}

// Connected reports whether the manager connection is up.
func (a *AMI) Connected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

// Run connects and reconnects until ctx ends.
func (a *AMI) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := a.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil && a.Log != nil {
			a.Log.Debug("ami disconnected", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (a *AMI) session(ctx context.Context) error {
	if a.Secret == "" {
		return errors.New("no AMI secret")
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", a.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	rd := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	banner, err := rd.ReadString('\n')
	if err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})

	a.mu.Lock()
	a.conn = conn
	a.pending = map[string]*pending{}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.conn, a.connected = nil, false
		for _, p := range a.pending {
			close(p.done)
		}
		a.pending = nil
		a.mu.Unlock()
	}()

	readErr := make(chan error, 1)
	go func() { readErr <- a.readLoop(rd) }()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	resp, _, err := a.send(ctx, Message{{"Action", "Login"}, {"Username", a.User}, {"Secret", a.Secret}, {"Events", "call,cdr,system,reporting"}}, false)
	if err != nil {
		return err
	}
	if !strings.EqualFold(resp.Get("Response"), "Success") {
		return fmt.Errorf("ami login failed: %s", resp.Get("Message"))
	}
	a.mu.Lock()
	a.connected = true
	a.version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(banner), "Asterisk Call Manager/"))
	a.mu.Unlock()
	if a.Log != nil {
		a.Log.Info("connected to asterisk manager", "addr", a.Addr)
	}
	// Keep the connection alive and notice a dead Asterisk quickly.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, _, err := a.send(pctx, Message{{"Action", "Ping"}}, false)
				cancel()
				if err != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	return <-readErr
}

func (a *AMI) readLoop(rd *bufio.Reader) error {
	var msg Message
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(msg) > 0 {
				a.dispatch(msg)
			}
			msg = nil
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			// Old style command output without a key.
			msg = append(msg, KV{"Output", line})
			continue
		}
		msg = append(msg, KV{strings.TrimSpace(k), strings.TrimSpace(v)})
	}
}

func (a *AMI) dispatch(m Message) {
	id := m.Get("ActionID")
	a.mu.Lock()
	p := a.pending[id]
	if p != nil {
		switch {
		case m.Get("Response") != "":
			p.resp = m
			if !p.list || !strings.EqualFold(m.Get("EventList"), "start") || strings.EqualFold(m.Get("Response"), "Error") {
				delete(a.pending, id)
				close(p.done)
			}
		case m.Get("Event") != "":
			if strings.EqualFold(m.Get("EventList"), "Complete") {
				delete(a.pending, id)
				close(p.done)
			} else {
				p.events = append(p.events, m)
			}
		}
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	if m.Get("Event") != "" && a.OnEvent != nil {
		a.OnEvent(m)
	}
}

func (a *AMI) send(ctx context.Context, m Message, list bool) (Message, []Message, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := &pending{list: list, done: make(chan struct{})}
	a.mu.Lock()
	if a.conn == nil {
		a.mu.Unlock()
		return nil, nil, ErrNotConnected
	}
	a.pending[id] = p
	var sb strings.Builder
	for _, kv := range m {
		// AMI is line based: a newline in a value would start a new header.
		v := strings.NewReplacer("\r", "", "\n", "").Replace(kv.Value)
		fmt.Fprintf(&sb, "%s: %s\r\n", kv.Key, v)
	}
	fmt.Fprintf(&sb, "ActionID: %s\r\n\r\n", id)
	_ = a.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := a.conn.Write([]byte(sb.String()))
	a.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	select {
	case <-p.done:
		if p.resp == nil {
			return nil, nil, ErrNotConnected
		}
		return p.resp, p.events, nil
	case <-ctx.Done():
		a.mu.Lock()
		if a.pending != nil {
			delete(a.pending, id)
		}
		a.mu.Unlock()
		return nil, nil, ctx.Err()
	}
}

// Action sends an action and waits for the response.
func (a *AMI) Action(ctx context.Context, m Message) (Message, error) {
	if !a.Connected() {
		return nil, ErrNotConnected
	}
	r, _, err := a.send(ctx, m, false)
	return r, err
}

// List sends an action that answers with a list of events (EventList: start ... Complete).
func (a *AMI) List(ctx context.Context, m Message) ([]Message, error) {
	if !a.Connected() {
		return nil, ErrNotConnected
	}
	r, ev, err := a.send(ctx, m, true)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(r.Get("Response"), "Error") {
		// "No endpoints found" and similar are an empty list, not a failure.
		return nil, nil
	}
	return ev, nil
}

// Command runs an Asterisk CLI command and returns its output.
func (a *AMI) Command(ctx context.Context, cmd string) (string, error) {
	r, err := a.Action(ctx, Message{{"Action", "Command"}, {"Command", cmd}})
	if err != nil {
		return "", err
	}
	if strings.EqualFold(r.Get("Response"), "Error") {
		return "", errors.New(r.Get("Message"))
	}
	return strings.Join(r.All("Output"), "\n"), nil
}
