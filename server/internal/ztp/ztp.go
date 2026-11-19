// Package ztp registers phones at the redirection services of the vendors (zero-touch
// provisioning): a factory-new phone asks the vendor where its provisioning server is, and is sent to
// Kiestoon. Yealink RPS has a public API and is handled automatically; for the other vendors the portal
// shows the address to enter in their portal and exports the MAC addresses.
package ztp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SettingsKey is the settings row with Settings as JSON.
const SettingsKey = "ztp"

// Settings of zero-touch provisioning.
type Settings struct {
	// ServerURL is the provisioning address that phones on the internet can reach, for example
	// https://pbx.example.com/prov/ (credentials are added by Kiestoon). Empty: the portal address.
	ServerURL string  `json:"serverUrl"`
	Yealink   Yealink `json:"yealink"`
}

// Yealink holds the AccessKey of Yealink's device management (RPS enterprise) platform.
type Yealink struct {
	Enabled   bool   `json:"enabled"`
	AccessKey string `json:"accessKey"`
	Secret    string `json:"secret,omitempty"`
	SecretSet bool   `json:"secretSet"`
	// API is the API address; the default is Yealink's global RPS API.
	API string `json:"api"`
}

// DefaultYealinkAPI is the RPS management API.
const DefaultYealinkAPI = "https://api-dm.yealink.com:8443"

var reKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

// Validate checks the settings.
func (s *Settings) Validate() error {
	s.ServerURL = strings.TrimSpace(s.ServerURL)
	if s.ServerURL != "" {
		u, err := url.Parse(s.ServerURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" {
			return errors.New("serverUrl")
		}
		if !strings.HasSuffix(s.ServerURL, "/") {
			s.ServerURL += "/"
		}
	}
	y := &s.Yealink
	y.AccessKey = strings.TrimSpace(y.AccessKey)
	if y.API == "" {
		y.API = DefaultYealinkAPI
	}
	if u, err := url.Parse(y.API); err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("api")
	}
	y.API = strings.TrimRight(y.API, "/")
	if y.Enabled && (!reKey.MatchString(y.AccessKey) || y.Secret == "") {
		return errors.New("accessKey")
	}
	if y.AccessKey != "" && !reKey.MatchString(y.AccessKey) {
		return errors.New("accessKey")
	}
	return nil
}

// Client talks to the Yealink RPS API.
type Client struct {
	HTTP *http.Client
	Y    Yealink
	now  func() time.Time
}

// NewClient returns a client for the settings.
func NewClient(y Yealink) *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, Y: y, now: time.Now}
}

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sign computes X-Ca-Signature: HMAC-SHA256 over method, the system headers in alphabetical order, the
// path without the leading slash and the sorted query, as described in "Yealink JSON API for RPS".
func sign(secret, method, contentMD5, key, nonce, ts, path string, query url.Values) string {
	var b strings.Builder
	b.WriteString(method + "\n")
	if contentMD5 != "" {
		b.WriteString("Content-MD5:" + contentMD5 + "\n")
	}
	b.WriteString("X-Ca-Key:" + key + "\n")
	b.WriteString("X-Ca-Nonce:" + nonce + "\n")
	b.WriteString("X-Ca-Timestamp:" + ts + "\n")
	b.WriteString(strings.TrimPrefix(path, "/"))
	if len(query) > 0 {
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if v := strings.TrimSpace(query.Get(k)); v != "" {
				parts = append(parts, k+"="+v)
			} else {
				parts = append(parts, k)
			}
		}
		b.WriteString("\n" + strings.Join(parts, "&"))
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

type response struct {
	Ret    int             `json:"ret"`
	Data   json.RawMessage `json:"data"`
	Error  *apiError       `json:"error"`
	Errors *apiError       `json:"errors"`
}

type apiError struct {
	Msg         string `json:"msg"`
	ErrorCode   int    `json:"errorCode"`
	FieldErrors []struct {
		Msg string `json:"msg"`
	} `json:"fieldErrors"`
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var raw []byte
	md5sum := ""
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
		sum := md5.Sum(raw)
		md5sum = base64.StdEncoding.EncodeToString(sum[:])
	}
	u := c.Y.API + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)
	n := nonce()
	req.Header.Set("X-Ca-Key", c.Y.AccessKey)
	req.Header.Set("X-Ca-Timestamp", ts)
	req.Header.Set("X-Ca-Nonce", n)
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
		req.Header.Set("Content-MD5", md5sum)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Ca-Signature", sign(c.Y.Secret, method, md5sum, c.Y.AccessKey, n, ts, path, query))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var r response
	if err := json.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("yealink rps: http %d", resp.StatusCode)
	}
	if r.Ret < 0 || resp.StatusCode >= 400 {
		e := r.Error
		if e == nil {
			e = r.Errors
		}
		msg := fmt.Sprintf("http %d", resp.StatusCode)
		if e != nil {
			msg = e.Msg
			for _, f := range e.FieldErrors {
				if f.Msg != "" {
					msg = strings.TrimSpace(msg + " " + f.Msg)
				}
			}
			if msg == "" {
				msg = fmt.Sprintf("error %d", e.ErrorCode)
			}
		}
		return fmt.Errorf("yealink rps: %s", msg)
	}
	if out != nil && len(r.Data) > 0 {
		return json.Unmarshal(r.Data, out)
	}
	return nil
}

// ErrOtherOwner means the phone is registered in another RPS account (for example the reseller's):
// it has to be released there first.
var ErrOtherOwner = errors.New("registered_elsewhere")

// Register points a phone (MAC without separators) to the provisioning address, with the
// credentials the phone uses for it.
func (c *Client) Register(ctx context.Context, mac, serverURL, user, password, remark string) error {
	var chk struct {
		Existed bool  `json:"existed"`
		Self    *bool `json:"self"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/open/v1/device/checkMac", url.Values{"mac": {mac}}, nil, &chk); err != nil {
		return err
	}
	if chk.Existed && chk.Self != nil && !*chk.Self {
		return ErrOtherOwner
	}
	if chk.Existed {
		// Ours already: replace it, so the address and credentials are current.
		if err := c.Remove(ctx, mac); err != nil {
			return err
		}
	}
	body := map[string]any{"macs": []string{mac}, "uniqueServerUrl": serverURL, "authName": user, "password": password}
	if remark != "" {
		if len(remark) > 200 {
			remark = remark[:200]
		}
		body["remark"] = remark
	}
	return c.call(ctx, http.MethodPost, "/api/open/v1/device/add", nil, body, nil)
}

// Remove deletes a phone from the account.
func (c *Client) Remove(ctx context.Context, mac string) error {
	return c.call(ctx, http.MethodPost, "/api/open/v1/device/delete", nil, map[string]any{"macs": []string{mac}}, nil)
}

// Status returns the RPS state of a phone: Unknown, Unregistered, Registered Elsewhere or Registered,
// with the bound address.
func (c *Client) Status(ctx context.Context, mac string) (string, string, error) {
	var st struct {
		Status   string  `json:"status"`
		BoundURL *string `json:"boundUrl"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/open/v1/device/checkDeviceBoundUrl", url.Values{"mac": {mac}}, nil, &st); err != nil {
		return "", "", err
	}
	bound := ""
	if st.BoundURL != nil {
		bound = *st.BoundURL
	}
	return st.Status, bound, nil
}

// Guide describes the redirection service of a vendor without an API in Kiestoon.
type Guide struct {
	Vendor  string `json:"vendor"`
	Service string `json:"service"`
	Portal  string `json:"portal"`
	// URL is what to enter as provisioning address; it may contain {mac} placeholders the service uses.
	Path string `json:"path"`
}

// Guides are shown in the portal for the vendors that are registered by hand.
var Guides = []Guide{
	{"snom", "Snom SRAPS", "https://secure-provisioning.snom.com", "snom{model}.htm"},
	{"grandstream", "Grandstream GDMS", "https://www.gdms.cloud", ""},
	{"fanvil", "Fanvil FDPS", "https://fdps.fanvil.com", ""},
	{"htek", "Htek RPS", "https://rps.htek.com", ""},
	{"poly", "Poly ZTP (Poly Lens)", "https://lens.poly.com", ""},
	{"gigaset", "Gigaset redirect server", "https://teamwork.gigaset.com", ""},
	{"cisco", "Cisco EDOS / Webex Device Activation", "https://upgrade.cisco.com", "cisco/$MA.xml"},
}
