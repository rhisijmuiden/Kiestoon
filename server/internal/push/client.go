package push

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultRelay is the relay of the Kiestoon app that RHIS publishes.
const DefaultRelay = "https://push.rhis.nl"

// ErrWrongKind means the token cannot take this kind of notification.
var ErrWrongKind = errors.New("push: wrong kind for this token")

// ErrGone means the phone no longer accepts notifications (the app was removed): forget it.
var ErrGone = errors.New("push: device gone")

// Client talks to a relay for one Kiestoon installation.
type Client struct {
	URL  string
	Key  ed25519.PrivateKey
	HTTP *http.Client
	// Now is the clock (tests).
	Now func() time.Time
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ts := now().Unix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Kiestoon")
	req.Header.Set(HeaderKey, EncodeKey(c.Key.Public().(ed25519.PublicKey)))
	req.Header.Set(HeaderTime, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, Sign(c.Key, ts, path, body))
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode == http.StatusGone {
		return ErrGone
	}
	if res.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return fmt.Errorf("push relay: %s", e.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Register trades a push token of a phone for a handle.
func (c *Client) Register(ctx context.Context, platform, token string, sandbox bool) (string, error) {
	var out RegisterResponse
	if err := c.post(ctx, "/v1/register", RegisterRequest{Platform: platform, Token: token, Sandbox: sandbox, Nonce: nonce()}, &out); err != nil {
		return "", err
	}
	if out.Handle == "" {
		return "", errors.New("push relay: no handle")
	}
	return out.Handle, nil
}

// Send sends one notification. data is encrypted with the device secret first.
func (c *Client) Send(ctx context.Context, handle, secret, kind, id, lang string, data any) error {
	req := PushRequest{Handle: handle, Kind: kind, ID: id, Lang: lang, Nonce: nonce()}
	if data != nil {
		enc, err := Seal(secret, data)
		if err != nil {
			return err
		}
		req.Data = enc
	}
	return c.post(ctx, "/v1/push", req, nil)
}
