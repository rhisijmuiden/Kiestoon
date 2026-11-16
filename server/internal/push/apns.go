package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// APNsSender sends to iPhones with a token-based (.p8) key of the Apple developer account.
type APNsSender struct {
	TeamID string
	KeyID  string
	Key    *ecdsa.PrivateKey
	// Bundle is the bundle id of the app (nl.rhis.kiestoon); VoIP pushes go to Bundle + ".voip".
	Bundle string
	HTTP   *http.Client
	// BaseURL and SandboxURL override the Apple hosts (tests).
	BaseURL, SandboxURL string

	mu     sync.Mutex
	jwt    string
	jwtAt  time.Time
	nowFun func() time.Time
}

// ParseP8 reads the .p8 key file Apple hands out.
func ParseP8(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return nil, errors.New("apns: no PEM block in key")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apns: key is not an EC key")
	}
	return ek, nil
}

func (a *APNsSender) now() time.Time {
	if a.nowFun != nil {
		return a.nowFun()
	}
	return time.Now()
}

// token returns the provider token; Apple wants a new one at most every hour and at least every 20
// minutes apart.
func (a *APNsSender) token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && a.now().Sub(a.jwtAt) < 40*time.Minute {
		return a.jwt, nil
	}
	hdr, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": a.KeyID})
	claims, _ := json.Marshal(map[string]any{"iss": a.TeamID, "iat": a.now().Unix()})
	in := b64(hdr) + "." + b64(claims)
	sum := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, a.Key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	a.jwt, a.jwtAt = in+"."+b64(sig), a.now()
	return a.jwt, nil
}

// apnsTexts are the generic texts of visible notifications; the app shows the decrypted content
// instead when it can.
var apnsTexts = map[string]map[string][2]string{
	"en": {KindVoicemail: {"New voicemail", "Open Kiestoon to listen."}, KindTest: {"Kiestoon", "Test notification: notifications work."}},
	"nl": {KindVoicemail: {"Nieuwe voicemail", "Open Kiestoon om te luisteren."}, KindTest: {"Kiestoon", "Testmelding: meldingen werken."}},
}

// apnsRequest builds the headers and body Apple wants for one kind.
func apnsRequest(bundle string, m Message) (pushType, topic, priority string, payload map[string]any) {
	payload = map[string]any{"k": m.Kind}
	if m.ID != "" {
		payload["id"] = m.ID
	}
	if m.Data != "" {
		payload["e"] = m.Data
	}
	switch m.Kind {
	case KindCall:
		// A VoIP push starts the app, which must show the call with CallKit at once.
		return "voip", bundle + ".voip", "10", payload
	case KindCancel:
		payload["aps"] = map[string]any{"content-available": 1}
		return "background", bundle, "5", payload
	}
	// Voicemail and test: a visible notification with a generic text. A notification extension of
	// the app can decrypt the content and show the caller and the summary instead.
	texts, ok := apnsTexts[m.Lang]
	if !ok {
		texts = apnsTexts["en"]
	}
	t := texts[m.Kind]
	payload["aps"] = map[string]any{
		"alert":           map[string]any{"title": t[0], "body": t[1]},
		"sound":           "default",
		"mutable-content": 1,
		"thread-id":       m.Kind,
	}
	return "alert", bundle, "10", payload
}

// Send delivers one notification.
func (a *APNsSender) Send(ctx context.Context, m Message) error {
	// A PushKit token only takes calls, and an alert token never takes them.
	if (m.Platform == APNsVoIP) != (m.Kind == KindCall) {
		return ErrWrongKind
	}
	jwt, err := a.token()
	if err != nil {
		return err
	}
	pushType, topic, prio, payload := apnsRequest(a.Bundle, m)
	body, _ := json.Marshal(payload)
	base := a.BaseURL
	if base == "" {
		base = "https://api.push.apple.com"
	}
	if m.Sandbox {
		base = a.SandboxURL
		if base == "" {
			base = "https://api.sandbox.push.apple.com"
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/3/device/"+m.Token, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-push-type", pushType)
	req.Header.Set("apns-topic", topic)
	req.Header.Set("apns-priority", prio)
	if m.Kind == KindCall || m.Kind == KindCancel {
		// A call that is not delivered now is useless later.
		req.Header.Set("apns-expiration", "0")
	} else {
		req.Header.Set("apns-expiration", fmt.Sprint(a.now().Add(24*time.Hour).Unix()))
	}
	if m.ID != "" && pushType == "alert" {
		req.Header.Set("apns-collapse-id", m.ID)
	}
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusOK {
		return nil
	}
	var e struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &e)
	switch {
	case res.StatusCode == http.StatusGone, e.Reason == "BadDeviceToken", e.Reason == "Unregistered", e.Reason == "DeviceTokenNotForTopic":
		return ErrGone
	case e.Reason == "ExpiredProviderToken":
		a.mu.Lock()
		a.jwt = ""
		a.mu.Unlock()
	}
	return fmt.Errorf("apns: %d %s", res.StatusCode, e.Reason)
}
