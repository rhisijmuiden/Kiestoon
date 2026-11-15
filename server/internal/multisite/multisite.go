// Package multisite holds what Kiestoon systems exchange with each other: pairing codes, the
// pinned HTTPS client used while pairing, and the request and response bodies of the pairing APIs.
//
// Pairing is the only moment two systems talk over the internet. The code the administrator copies
// carries the address of the inviting system, the SHA-256 fingerprint of its HTTPS certificate and a
// one-time secret. The joining side only accepts that exact certificate, so a self-signed portal
// certificate is safe to pair with. After pairing everything runs through WireGuard.
package multisite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kinds of pairing codes.
const (
	KindEdge       = "edge"
	KindFederation = "fed"
	KindCluster    = "ha"
)

// Code is the content of a pairing code.
type Code struct {
	Kind string `json:"k"`
	// URL is the HTTPS address of the inviting portal, for example https://pbx.example.com:8443.
	URL string `json:"u"`
	// Fingerprint is the hex SHA-256 of the inviting portal's certificate (DER).
	Fingerprint string `json:"f"`
	// Secret is the one-time secret; the inviting side stores only its hash.
	Secret string `json:"s"`
}

const prefix = "KT1-"

// Encode turns a code into the text the administrator copies.
func (c Code) Encode() string {
	b, _ := json.Marshal(c)
	return prefix + strings.ToUpper(string(c.Kind[0])) + "-" + base64.RawURLEncoding.EncodeToString(b)
}

// ErrInvalidCode means the text is not a pairing code (of the expected kind).
var ErrInvalidCode = errors.New("invalid_code")

// Decode reads a pairing code of the given kind.
func Decode(text, kind string) (Code, error) {
	text = strings.Join(strings.Fields(text), "")
	if !strings.HasPrefix(text, prefix) || len(text) < len(prefix)+3 {
		return Code{}, ErrInvalidCode
	}
	raw, err := base64.RawURLEncoding.DecodeString(text[len(prefix)+2:])
	if err != nil {
		return Code{}, ErrInvalidCode
	}
	var c Code
	if json.Unmarshal(raw, &c) != nil || c.Kind != kind || c.Secret == "" {
		return Code{}, ErrInvalidCode
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return Code{}, ErrInvalidCode
	}
	if c.Fingerprint != "" {
		if b, err := hex.DecodeString(c.Fingerprint); err != nil || len(b) != sha256.Size {
			return Code{}, ErrInvalidCode
		}
	}
	return c, nil
}

// NewSecret returns a random one-time secret (160 bits, base32).
func NewSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

// NewToken returns a random API token for calls between paired systems.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "kts_" + hex.EncodeToString(b)
}

// Hash is the stored form of secrets and tokens.
func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Fingerprint returns the hex SHA-256 of a certificate.
func Fingerprint(c *x509.Certificate) string {
	h := sha256.Sum256(c.Raw)
	return hex.EncodeToString(h[:])
}

// PinnedClient talks HTTPS to exactly the certificate with this fingerprint. Without a fingerprint
// it verifies the certificate normally (for a portal with a public certificate).
func PinnedClient(fingerprint string, timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if fingerprint != "" {
		want := strings.ToLower(fingerprint)
		tr.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // replaced by the exact fingerprint check below
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 || Fingerprint(cs.PeerCertificates[0]) != want {
					return errors.New("certificate does not match the pairing code")
				}
				return nil
			},
		}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

// APIError is an error answer of the other system.
type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string { return fmt.Sprintf("remote: %d %s", e.Status, e.Code) }

// PostJSON sends in as JSON and decodes the answer into out. token, when set, goes in the
// Authorization header.
func PostJSON(ctx context.Context, c *http.Client, url, token string, in, out any) error {
	return doJSON(ctx, c, http.MethodPost, url, token, in, out)
}

// GetJSON fetches url and decodes the answer into out.
func GetJSON(ctx context.Context, c *http.Client, url, token string, out any) error {
	return doJSON(ctx, c, http.MethodGet, url, token, nil, out)
}

func doJSON(ctx context.Context, c *http.Client, method, url, token string, in, out any) error {
	var body *strings.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	} else {
		body = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return &APIError{Status: resp.StatusCode, Code: e.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// BearerToken returns the token of an Authorization: Bearer header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ---- Edge pairing -----------------------------------------------------------

// EdgePairRequest is sent by a Kiestoon Edge to the central portal (POST /api/edge/pair).
type EdgePairRequest struct {
	Secret    string `json:"secret"`
	PublicKey string `json:"publicKey"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
	// LANAddress is the address of the edge in the office network, phones use it as SIP server.
	LANAddress string `json:"lanAddress"`
	LANNet     string `json:"lanNet"`
}

// EdgePairResponse tells the edge how to build its tunnel.
type EdgePairResponse struct {
	SiteID       int64  `json:"siteId"`
	Name         string `json:"name"`
	Token        string `json:"token"`
	Address      string `json:"address"` // own tunnel address with prefix (/32)
	HubAddress   string `json:"hubAddress"`
	HubPublicKey string `json:"hubPublicKey"`
	Endpoint     string `json:"endpoint"`
	SIPPort      int    `json:"sipPort"`
	PortalPort   int    `json:"portalPort"`
}

// EdgeHeartbeat is sent by the edge every 30 seconds through the tunnel.
type EdgeHeartbeat struct {
	Version    string            `json:"version"`
	Hostname   string            `json:"hostname"`
	LANAddress string            `json:"lanAddress"`
	LANNet     string            `json:"lanNet"`
	Uptime     int64             `json:"uptime"`
	Services   map[string]string `json:"services"`
}

// EdgeConfig is the answer to a heartbeat.
type EdgeConfig struct {
	Name    string `json:"name"`
	SIPPort int    `json:"sipPort"`
	// RTPStart and RTPEnd are the media ports of the edge relay.
	RTPStart int `json:"rtpStart"`
	RTPEnd   int `json:"rtpEnd"`
}

// ---- Federation ---------------------------------------------------------------

// FedAcceptRequest is sent by the joining Kiestoon to the inviting one.
type FedAcceptRequest struct {
	Secret    string `json:"secret"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	// Token is what the inviting side uses to call the joining side.
	Token      string `json:"token"`
	SIPPort    int    `json:"sipPort"`
	PortalPort int    `json:"portalPort"`
	Version    string `json:"version"`
	// Subnet is the hub range of the joining side, to detect overlaps.
	Subnet string `json:"subnet"`
}

// FedAcceptResponse tells the joining side how to reach the inviting one.
type FedAcceptResponse struct {
	Name         string `json:"name"`
	Token        string `json:"token"`
	Address      string `json:"address"`
	HubAddress   string `json:"hubAddress"`
	HubPublicKey string `json:"hubPublicKey"`
	Endpoint     string `json:"endpoint"`
	SIPPort      int    `json:"sipPort"`
	PortalPort   int    `json:"portalPort"`
	Version      string `json:"version"`
}

// DirectoryEntry is a number of a federated site.
type DirectoryEntry struct {
	Number string `json:"number"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // extension, queue, ringgroup, conference
}

// Directory is what a federated site publishes.
type Directory struct {
	Name     string           `json:"name"`
	Timezone string           `json:"timezone"`
	Version  string           `json:"version"`
	Entries  []DirectoryEntry `json:"entries"`
}

// Presence carries BLF states (number -> idle, inuse, busy, ringing, onhold, unavailable).
type Presence struct {
	Full   bool              `json:"full"`
	States map[string]string `json:"states"`
}

// ---- High availability ---------------------------------------------------------

// ClusterJoinRequest is sent by a new standby node to the primary.
type ClusterJoinRequest struct {
	Secret    string `json:"secret"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	PublicKey string `json:"publicKey"`
	Version   string `json:"version"`
}

// ClusterJoinResponse configures the standby.
type ClusterJoinResponse struct {
	Secret        string `json:"secret"`
	PeerName      string `json:"peerName"`
	PeerAddress   string `json:"peerAddress"`
	PeerPublicKey string `json:"peerPublicKey"`
	PeerTunnel    string `json:"peerTunnel"`
	Tunnel        string `json:"tunnel"`
	Port          int    `json:"port"`
	Mode          string `json:"mode"`
	VIP           string `json:"vip"`
	RouterID      int    `json:"routerId"`
}
