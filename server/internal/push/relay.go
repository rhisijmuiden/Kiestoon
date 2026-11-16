package push

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Message is what a Sender delivers to one phone.
type Message struct {
	Token   string
	Sandbox bool
	Kind    string
	ID      string
	Data    string
}

// Sender delivers to one platform.
type Sender interface {
	// Send delivers m. It returns ErrGone when the token is no longer valid.
	Send(ctx context.Context, m Message) error
}

// handleData is what a handle seals.
type handleData struct {
	Platform string `json:"p"`
	Token    string `json:"t"`
	Sandbox  bool   `json:"s,omitempty"`
	// Key is the public key of the installation that registered the handle.
	Key string `json:"k"`
}

// Relay is the push relay. It keeps no state other than short-lived counters.
type Relay struct {
	// Secret seals the handles (at least 32 random bytes). Changing it makes every handle invalid:
	// the apps register again the next time they start.
	Secret  []byte
	Senders map[string]Sender
	Log     *slog.Logger
	// PerMinute is the number of notifications one installation may send per minute (default 120).
	PerMinute int
	Now       func() time.Time

	mu    sync.Mutex
	seen  map[[32]byte]time.Time // signatures of the last minutes: a request works once
	count map[string]*window     // per installation key
}

type window struct {
	start time.Time
	n     int
}

// MaxSkew is how far the clock of a Kiestoon server may be off.
const MaxSkew = 5 * time.Minute

func (rl *Relay) now() time.Time {
	if rl.Now != nil {
		return rl.Now()
	}
	return time.Now()
}

// Handler serves /v1/register, /v1/push and /healthz.
func (rl *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		platforms := []string{}
		for p := range rl.Senders {
			platforms = append(platforms, p)
		}
		slices.Sort(platforms)
		relayJSON(w, http.StatusOK, map[string]any{"ok": true, "platforms": platforms})
	})
	mux.HandleFunc("POST /v1/register", rl.register)
	mux.HandleFunc("POST /v1/push", rl.push)
	return mux
}

func relayJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func relayErr(w http.ResponseWriter, code int, e string) {
	relayJSON(w, code, map[string]string{"error": e})
}

// verified reads and checks a signed request. It returns the body and the installation key.
func (rl *Relay) verified(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		relayErr(w, http.StatusBadRequest, "bad_request")
		return nil, "", false
	}
	keyS := r.Header.Get(HeaderKey)
	pub, err := DecodeKey(keyS)
	if err != nil {
		relayErr(w, http.StatusUnauthorized, "bad_signature")
		return nil, "", false
	}
	ts, err := strconv.ParseInt(r.Header.Get(HeaderTime), 10, 64)
	now := rl.now()
	if err != nil || now.Sub(time.Unix(ts, 0)).Abs() > MaxSkew {
		relayErr(w, http.StatusUnauthorized, "clock_skew")
		return nil, "", false
	}
	sig := r.Header.Get(HeaderSignature)
	if !Verify(pub, ts, r.URL.Path, body, sig) {
		relayErr(w, http.StatusUnauthorized, "bad_signature")
		return nil, "", false
	}
	// A signed request works once: replaying a captured request would ring a phone again.
	h := sha256.Sum256([]byte(sig))
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.seen == nil {
		rl.seen, rl.count = map[[32]byte]time.Time{}, map[string]*window{}
	}
	for k, t := range rl.seen {
		if now.Sub(t) > 2*MaxSkew {
			delete(rl.seen, k)
		}
	}
	if _, dup := rl.seen[h]; dup {
		relayErr(w, http.StatusUnauthorized, "replayed")
		return nil, "", false
	}
	rl.seen[h] = now
	limit := rl.PerMinute
	if limit <= 0 {
		limit = 120
	}
	c := rl.count[keyS]
	if c == nil || now.Sub(c.start) >= time.Minute {
		c = &window{start: now}
		rl.count[keyS] = c
	}
	c.n++
	if c.n > limit {
		relayErr(w, http.StatusTooManyRequests, ErrCodeRate)
		return nil, "", false
	}
	return body, keyS, true
}

func (rl *Relay) register(w http.ResponseWriter, r *http.Request) {
	body, key, ok := rl.verified(w, r)
	if !ok {
		return
	}
	var req RegisterRequest
	if json.Unmarshal(body, &req) != nil || req.Token == "" || len(req.Token) > 4096 {
		relayErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if _, ok := rl.Senders[req.Platform]; !ok {
		relayErr(w, http.StatusBadRequest, "platform_unavailable")
		return
	}
	plain, _ := json.Marshal(handleData{Platform: req.Platform, Token: req.Token, Sandbox: req.Sandbox, Key: key})
	sealed, err := gcmSeal(deriveKey(rl.Secret, "handle"), plain, []byte("kiestoon-push-handle"))
	if err != nil {
		relayErr(w, http.StatusInternalServerError, "internal")
		return
	}
	relayJSON(w, http.StatusOK, RegisterResponse{Handle: "h1." + b64(sealed)})
}

func (rl *Relay) openHandle(h string) (*handleData, error) {
	if len(h) < 4 || h[:3] != "h1." {
		return nil, errors.New("bad handle")
	}
	raw, err := unb64(h[3:])
	if err != nil {
		return nil, err
	}
	plain, err := gcmOpen(deriveKey(rl.Secret, "handle"), raw, []byte("kiestoon-push-handle"))
	if err != nil {
		return nil, err
	}
	var d handleData
	if err := json.Unmarshal(plain, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (rl *Relay) push(w http.ResponseWriter, r *http.Request) {
	body, key, ok := rl.verified(w, r)
	if !ok {
		return
	}
	var req PushRequest
	if json.Unmarshal(body, &req) != nil || !slices.Contains(Kinds, req.Kind) || len(req.Data) > MaxData || len(req.ID) > 64 {
		relayErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	d, err := rl.openHandle(req.Handle)
	// The handle must belong to the installation that signed the request.
	if err != nil || subtle.ConstantTimeCompare([]byte(d.Key), []byte(key)) != 1 {
		relayErr(w, http.StatusForbidden, ErrCodeBadHandle)
		return
	}
	snd, ok := rl.Senders[d.Platform]
	if !ok {
		relayErr(w, http.StatusServiceUnavailable, "platform_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err = snd.Send(ctx, Message{Token: d.Token, Sandbox: d.Sandbox, Kind: req.Kind, ID: req.ID, Data: req.Data})
	switch {
	case errors.Is(err, ErrGone):
		relayErr(w, http.StatusGone, ErrCodeGone)
	case err != nil:
		// Never log the token or the content: only the platform, the kind and the error.
		if rl.Log != nil {
			rl.Log.Warn("push failed", "platform", d.Platform, "kind", req.Kind, "err", err)
		}
		relayErr(w, http.StatusBadGateway, "upstream")
	default:
		relayJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// LogSender is used when a platform has no credentials: it only logs that a notification would be
// sent (for a test setup).
type LogSender struct {
	Platform string
	Log      *slog.Logger
}

// Send logs the kind.
func (l LogSender) Send(_ context.Context, m Message) error {
	if l.Log != nil {
		l.Log.Info("push (log only)", "platform", l.Platform, "kind", m.Kind)
	}
	return nil
}

// PublicKeyOf returns the public key of a private key.
func PublicKeyOf(k ed25519.PrivateKey) ed25519.PublicKey { return k.Public().(ed25519.PublicKey) }
