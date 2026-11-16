package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type fakeSender struct {
	mu   sync.Mutex
	got  []Message
	gone bool
}

func (f *fakeSender) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return ErrGone
	}
	f.got = append(f.got, m)
	return nil
}

func newKey(t *testing.T) ed25519.PrivateKey {
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func setup(t *testing.T) (*fakeSender, *httptest.Server) {
	f := &fakeSender{}
	rl := &Relay{Secret: []byte(strings.Repeat("s", 32)), Senders: map[string]Sender{APNs: f, FCM: f}, PerMinute: 5}
	srv := httptest.NewServer(rl.Handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestRelayRoundTrip(t *testing.T) {
	f, srv := setup(t)
	c := &Client{URL: srv.URL, Key: newKey(t)}
	ctx := context.Background()
	h, err := c.Register(ctx, APNs, "tok123", true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h, "tok123") {
		t.Fatal("handle shows the token")
	}
	sec := NewDeviceSecret()
	if err := c.Send(ctx, h, sec, KindCall, "1700000000.12", map[string]string{"from": "0612345678"}); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 || f.got[0].Token != "tok123" || !f.got[0].Sandbox || f.got[0].Kind != KindCall {
		t.Fatalf("got %+v", f.got)
	}
	if strings.Contains(f.got[0].Data, "0612345678") {
		t.Fatal("relay saw the caller in plain text")
	}
	var out map[string]string
	if err := Open(sec, f.got[0].Data, &out); err != nil || out["from"] != "0612345678" {
		t.Fatalf("open: %v %v", out, err)
	}
	// Another installation cannot use the handle.
	other := &Client{URL: srv.URL, Key: newKey(t)}
	if err := other.Send(ctx, h, sec, KindCall, "", nil); err == nil || !strings.Contains(err.Error(), ErrCodeBadHandle) {
		t.Fatalf("foreign handle: %v", err)
	}
	// A tampered handle is refused.
	if err := c.Send(ctx, h[:len(h)-2]+"AA", sec, KindTest, "", nil); err == nil {
		t.Fatal("tampered handle accepted")
	}
	// Unknown kinds are refused.
	if err := c.Send(ctx, h, sec, "spam", "", nil); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestRelayGoneAndLimits(t *testing.T) {
	f, srv := setup(t)
	c := &Client{URL: srv.URL, Key: newKey(t)}
	ctx := context.Background()
	h, err := c.Register(ctx, FCM, "tok", false)
	if err != nil {
		t.Fatal(err)
	}
	f.gone = true
	if err := c.Send(ctx, h, NewDeviceSecret(), KindTest, "", nil); !errors.Is(err, ErrGone) {
		t.Fatalf("want gone, got %v", err)
	}
	f.gone = false
	var last error
	for range 5 {
		last = c.Send(ctx, h, NewDeviceSecret(), KindTest, "", nil)
	}
	if last == nil || !strings.Contains(last.Error(), ErrCodeRate) {
		t.Fatalf("want rate limit, got %v", last)
	}
	// A clock that is far off is refused.
	skewed := &Client{URL: srv.URL, Key: newKey(t), Now: func() time.Time { return time.Now().Add(-time.Hour) }}
	if _, err := skewed.Register(ctx, FCM, "tok", false); err == nil || !strings.Contains(err.Error(), "clock_skew") {
		t.Fatalf("skew: %v", err)
	}
	// Unconfigured platform.
	rl := &Relay{Secret: []byte(strings.Repeat("s", 32)), Senders: map[string]Sender{FCM: f}}
	s2 := httptest.NewServer(rl.Handler())
	defer s2.Close()
	if _, err := (&Client{URL: s2.URL, Key: newKey(t)}).Register(ctx, APNs, "t", false); err == nil {
		t.Fatal("apns without sender accepted")
	}
}

func TestRelayReplay(t *testing.T) {
	_, srv := setup(t)
	k := newKey(t)
	body := []byte(`{"platform":"fcm","token":"x"}`)
	ts := time.Now().Unix()
	send := func() int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/register", strings.NewReader(string(body)))
		req.Header.Set(HeaderKey, EncodeKey(PublicKeyOf(k)))
		req.Header.Set(HeaderTime, itoa(ts))
		req.Header.Set(HeaderSignature, Sign(k, ts, "/v1/register", body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := send(); c != 200 {
		t.Fatalf("first: %d", c)
	}
	if c := send(); c != http.StatusUnauthorized {
		t.Fatalf("replay: %d", c)
	}
}

func itoa(n int64) string { return big.NewInt(n).String() }

func TestAPNsSender(t *testing.T) {
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	var hdr http.Header
	var body map[string]any
	var path string
	status, reason := 200, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr, path = r.Header.Clone(), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(status)
		if reason != "" {
			_, _ = w.Write([]byte(`{"reason":"` + reason + `"}`))
		}
	}))
	defer srv.Close()
	a := &APNsSender{TeamID: "TEAM", KeyID: "KID", Key: ek, Bundle: "nl.rhis.kiestoon", BaseURL: srv.URL, SandboxURL: srv.URL}
	if err := a.Send(context.Background(), Message{Token: "abc", Kind: KindCall, ID: "1.2", Data: "xyz"}); err != nil {
		t.Fatal(err)
	}
	if path != "/3/device/abc" || hdr.Get("apns-push-type") != "voip" || hdr.Get("apns-topic") != "nl.rhis.kiestoon.voip" || body["e"] != "xyz" {
		t.Fatalf("call: %s %v %v", path, hdr, body)
	}
	// The provider token is a valid ES256 JWT.
	jwt := strings.TrimPrefix(hdr.Get("authorization"), "bearer ")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	sig, _ := unb64(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&ek.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("jwt signature invalid")
	}
	if err := a.Send(context.Background(), Message{Token: "abc", Kind: KindVoicemail, ID: "vm-201"}); err != nil {
		t.Fatal(err)
	}
	if hdr.Get("apns-push-type") != "alert" || hdr.Get("apns-topic") != "nl.rhis.kiestoon" || hdr.Get("apns-collapse-id") != "vm-201" {
		t.Fatalf("voicemail: %v", hdr)
	}
	status, reason = 400, "BadDeviceToken"
	if err := a.Send(context.Background(), Message{Token: "abc", Kind: KindTest}); !errors.Is(err, ErrGone) {
		t.Fatalf("want gone, got %v", err)
	}
}

func TestFCMSender(t *testing.T) {
	var body map[string]any
	status, resp := 200, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" || r.URL.Path != "/v1/projects/p1/messages:send" {
			t.Errorf("bad request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	f := &FCMSender{ProjectID: "p1", Tokens: tokenFunc("at"), BaseURL: srv.URL}
	if err := f.Send(context.Background(), Message{Token: "t1", Kind: KindCall, Data: "enc"}); err != nil {
		t.Fatal(err)
	}
	msg := body["message"].(map[string]any)
	if msg["token"] != "t1" || msg["data"].(map[string]any)["e"] != "enc" || msg["android"].(map[string]any)["priority"] != "HIGH" {
		t.Fatalf("message %v", msg)
	}
	if _, ok := msg["notification"]; ok {
		t.Fatal("content must stay in data")
	}
	status, resp = 404, `{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`
	if err := f.Send(context.Background(), Message{Token: "t1", Kind: KindTest}); !errors.Is(err, ErrGone) {
		t.Fatalf("want gone, got %v", err)
	}
}

type tokenFunc string

func (t tokenFunc) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: string(t), TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}, nil
}
