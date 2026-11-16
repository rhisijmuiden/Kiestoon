// Package push sends wake-up and voicemail notifications to the Kiestoon mobile app.
//
// A Kiestoon server cannot talk to Apple (APNs) or Google (FCM) itself: only the publisher of the
// app holds the keys for that. The push relay (cmd/kiestoon-push, run by RHIS at push.rhis.nl or
// by anyone who builds the app themselves) holds them and forwards notifications. It is built so
// that it needs to be trusted as little as possible:
//
//   - It stores nothing. When a phone registers, the relay seals the push token into an opaque
//     handle that only the relay can open. The Kiestoon server keeps the handle, the relay forgets
//     it.
//   - Every Kiestoon installation has its own ed25519 key. A handle is bound to the key of the
//     installation that registered it, and every request is signed, so an installation can only
//     wake its own phones, and a stolen handle is useless.
//   - The content of a notification (who is calling, the voicemail summary) is encrypted with a
//     key that only the Kiestoon server and the phone know. The relay forwards ciphertext.
//   - It logs no tokens, numbers or contents.
package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Platforms.
const (
	APNs = "apns" // iPhone: the token for visible notifications (voicemail, test)
	// APNsVoIP is the PushKit token of an iPhone: it only takes calls, which iOS shows with CallKit.
	APNsVoIP = "apns-voip"
	FCM      = "fcm" // Android
)

// Kinds of notification.
const (
	// KindCall wakes the app for an incoming call (a VoIP push on iPhone, a high priority data
	// message on Android). The app shows the call screen and connects to the server.
	KindCall = "call"
	// KindCancel ends the call screen of a wake: the caller hung up or someone else answered.
	KindCancel = "cancel"
	// KindVoicemail tells the user about a new voicemail message.
	KindVoicemail = "voicemail"
	// KindTest is the test notification from the portal.
	KindTest = "test"
)

// Kinds lists the kinds the relay accepts.
var Kinds = []string{KindCall, KindCancel, KindVoicemail, KindTest}

// Request headers of a signed request.
const (
	HeaderKey       = "X-Kiestoon-Key"
	HeaderTime      = "X-Kiestoon-Time"
	HeaderSignature = "X-Kiestoon-Signature"
)

// RegisterRequest asks the relay for a handle for a push token.
type RegisterRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"token"`
	// Sandbox sends to the APNs development environment (a build from Xcode or TestFlight
	// development).
	Sandbox bool `json:"sandbox,omitempty"`
	// Nonce makes every request unique (signatures are deterministic and a request works once).
	Nonce string `json:"nonce,omitempty"`
}

// RegisterResponse carries the handle.
type RegisterResponse struct {
	Handle string `json:"handle"`
}

// PushRequest asks the relay to send one notification.
type PushRequest struct {
	Handle string `json:"handle"`
	Kind   string `json:"kind"`
	// ID ties a cancel to its call and lets a newer voicemail notification replace an older one.
	ID string `json:"id,omitempty"`
	// Data is the encrypted content (see Seal), at most 2 KB.
	Data string `json:"data,omitempty"`
	// Lang is the language (en or nl) of the generic text of a visible notification on iPhone; the
	// app replaces it with the decrypted content where it can.
	Lang  string `json:"lang,omitempty"`
	Nonce string `json:"nonce,omitempty"`
}

// ErrCodeKind means a token cannot take this kind of notification (a call to an alert token, or a
// voicemail to a VoIP token).
const ErrCodeKind = "wrong_kind"

// MaxData is the largest Data a push may carry: APNs allows 4 KB per notification (5 KB for VoIP).
const MaxData = 2048

// Error codes of the relay.
const (
	ErrCodeGone      = "gone" // the app was removed or the token expired: forget the device
	ErrCodeBadHandle = "bad_handle"
	ErrCodeRate      = "rate_limited"
)

// signed is the text that is signed: a version, the time, the path and the body.
func signed(ts int64, path string, body []byte) []byte {
	b := make([]byte, 0, len(body)+64)
	b = append(b, "kiestoon-push-v1\n"...)
	b = strconv.AppendInt(b, ts, 10)
	b = append(b, '\n')
	b = append(b, path...)
	b = append(b, '\n')
	return append(b, body...)
}

// Sign signs a request body for the relay path.
func Sign(key ed25519.PrivateKey, ts int64, path string, body []byte) string {
	return b64(ed25519.Sign(key, signed(ts, path, body)))
}

// Verify checks a signature made by Sign.
func Verify(pub ed25519.PublicKey, ts int64, path string, body []byte, sig string) bool {
	s, err := unb64(sig)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, signed(ts, path, body), s)
}

func nonce() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return b64(b)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func unb64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// EncodeKey writes a public key the way the relay expects it.
func EncodeKey(pub ed25519.PublicKey) string { return b64(pub) }

// DecodeKey reads a public key written by EncodeKey.
func DecodeKey(s string) (ed25519.PublicKey, error) {
	b, err := unb64(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("push: bad key")
	}
	return ed25519.PublicKey(b), nil
}

// NewDeviceSecret makes the key the server and one phone share for the contents of notifications.
func NewDeviceSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b64(b)
}

// Seal encrypts the content of a notification for a device (AES-256-GCM; nonce || ciphertext).
func Seal(secret string, v any) (string, error) {
	key, err := unb64(secret)
	if err != nil || len(key) != 32 {
		return "", errors.New("push: bad device secret")
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	out, err := gcmSeal(key, plain, []byte("kiestoon-push-data"))
	if err != nil {
		return "", err
	}
	return b64(out), nil
}

// Open decrypts what Seal made (the app does the same).
func Open(secret, data string, v any) error {
	key, err := unb64(secret)
	if err != nil || len(key) != 32 {
		return errors.New("push: bad device secret")
	}
	raw, err := unb64(data)
	if err != nil {
		return err
	}
	plain, err := gcmOpen(key, raw, []byte("kiestoon-push-data"))
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, v)
}

func gcm(key []byte) (cipher.AEAD, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

func gcmSeal(key, plain, ad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	_, _ = rand.Read(nonce)
	return a.Seal(nonce, nonce, plain, ad), nil
}

func gcmOpen(key, raw, ad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(raw) < a.NonceSize() {
		return nil, errors.New("push: short ciphertext")
	}
	return a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], ad)
}

// deriveKey makes a 32-byte key for one purpose out of the relay secret.
func deriveKey(secret []byte, purpose string) []byte {
	h := sha256.New()
	h.Write([]byte("kiestoon-push/" + purpose + "\n"))
	h.Write(secret)
	return h.Sum(nil)
}
