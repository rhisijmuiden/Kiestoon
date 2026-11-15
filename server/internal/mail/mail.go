// Package mail sends e-mail through the SMTP server the administrator configured.
//
// Supported: implicit TLS (port 465), STARTTLS (587) or plain (25, only for a trusted relay),
// authentication with username and password (PLAIN or LOGIN, only over TLS) or OAuth2 (XOAUTH2)
// for Microsoft 365 (client credentials) and Google Workspace (refresh token).
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// ErrDisabled is returned when no mail server is configured.
var ErrDisabled = errors.New("mail server is not configured")

// SettingsKey is the settings row with the SMTP configuration.
const SettingsKey = "mail"

// Settings is the SMTP configuration.
type Settings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"` // tls | starttls | none
	Auth        string `json:"auth"`     // none | password | oauth2_m365 | oauth2_google
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
	FromAddress string `json:"fromAddress"`
	FromName    string `json:"fromName"`
	// OAuth2
	TenantID        string `json:"tenantId"`
	ClientID        string `json:"clientId"`
	ClientSecret    string `json:"clientSecret,omitempty"`
	ClientSecretSet bool   `json:"clientSecretSet"`
	RefreshToken    string `json:"refreshToken,omitempty"`
	RefreshTokenSet bool   `json:"refreshTokenSet"`
	// SkipVerify accepts any certificate. Only for a relay with a self-signed certificate.
	SkipVerify bool `json:"skipVerify"`
	// NotifyAddress receives system notifications such as failed backups.
	NotifyAddress string `json:"notifyAddress"`
}

// Load reads the settings.
func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	s := Settings{Port: 587, Security: "starttls", Auth: "password", FromName: "Kiestoon"}
	err := store.GetJSON(ctx, db, SettingsKey, &s)
	return s, err
}

// Public returns a copy without secrets, for the UI.
func (s Settings) Public() Settings {
	s.PasswordSet, s.ClientSecretSet, s.RefreshTokenSet = s.Password != "", s.ClientSecret != "", s.RefreshToken != ""
	s.Password, s.ClientSecret, s.RefreshToken = "", "", ""
	return s
}

// Merge keeps stored secrets when the UI sends empty secret fields.
func (s *Settings) Merge(old Settings) {
	if s.Password == "" {
		s.Password = old.Password
	}
	if s.ClientSecret == "" {
		s.ClientSecret = old.ClientSecret
	}
	if s.RefreshToken == "" {
		s.RefreshToken = old.RefreshToken
	}
	s.PasswordSet, s.ClientSecretSet, s.RefreshTokenSet = false, false, false
}

// Validate checks the settings.
func (s *Settings) Validate() error {
	s.Host = strings.TrimSpace(s.Host)
	if !s.Enabled {
		return nil
	}
	if s.Host == "" || strings.ContainsAny(s.Host, " /:@") {
		return errors.New("host")
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("port")
	}
	switch s.Security {
	case "tls", "starttls", "none":
	default:
		return errors.New("security")
	}
	switch s.Auth {
	case "none":
	case "password":
		if s.Username == "" || s.Password == "" {
			return errors.New("username")
		}
	case "oauth2_m365":
		if s.Username == "" || s.TenantID == "" || s.ClientID == "" || s.ClientSecret == "" {
			return errors.New("oauth")
		}
	case "oauth2_google":
		if s.Username == "" || s.ClientID == "" || s.ClientSecret == "" || s.RefreshToken == "" {
			return errors.New("oauth")
		}
	default:
		return errors.New("auth")
	}
	if s.Auth != "none" && s.Security == "none" {
		return errors.New("auth_needs_tls")
	}
	if a, err := mail.ParseAddress(s.FromAddress); err != nil || a.Address != s.FromAddress {
		return errors.New("fromAddress")
	}
	if s.NotifyAddress != "" {
		if _, err := mail.ParseAddress(s.NotifyAddress); err != nil {
			return errors.New("notifyAddress")
		}
	}
	s.FromName = strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s.FromName)
	return nil
}

// ---- OAuth2 -----------------------------------------------------------------

type token struct {
	value   string
	expires time.Time
}

var (
	tokenMu    sync.Mutex
	tokenCache = map[string]token{}
	httpClient = &http.Client{Timeout: 20 * time.Second}
	// Endpoints can be replaced in tests.
	m365TokenURL   = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

func accessToken(ctx context.Context, s Settings) (string, error) {
	key := s.Auth + "|" + s.TenantID + "|" + s.ClientID + "|" + s.Username
	tokenMu.Lock()
	if t, ok := tokenCache[key]; ok && time.Until(t.expires) > time.Minute {
		tokenMu.Unlock()
		return t.value, nil
	}
	tokenMu.Unlock()
	form := url.Values{"client_id": {s.ClientID}, "client_secret": {s.ClientSecret}}
	var endpoint string
	if s.Auth == "oauth2_m365" {
		endpoint = fmt.Sprintf(m365TokenURL, url.PathEscape(s.TenantID))
		form.Set("grant_type", "client_credentials")
		form.Set("scope", "https://outlook.office365.com/.default")
	} else {
		endpoint = googleTokenURL
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", s.RefreshToken)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth2 token: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK || body.AccessToken == "" {
		msg := body.Error
		if body.Description != "" {
			msg += ": " + strings.SplitN(body.Description, "\r\n", 2)[0]
		}
		return "", fmt.Errorf("oauth2 token: %s (HTTP %d)", msg, resp.StatusCode)
	}
	if body.ExpiresIn == 0 {
		body.ExpiresIn = 3600
	}
	tokenMu.Lock()
	tokenCache[key] = token{body.AccessToken, time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)}
	tokenMu.Unlock()
	return body.AccessToken, nil
}

type xoauth2 struct{ user, token string }

func (a xoauth2) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

func (a xoauth2) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// The server sent an error as JSON; answer empty so it reports the failure.
		return []byte{}, nil
	}
	return nil, nil
}

type loginAuth struct{ user, pass string }

func (a loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSuffix(string(fromServer), ":")) {
	case "username":
		return []byte(a.user), nil
	case "password":
		return []byte(a.pass), nil
	}
	return nil, fmt.Errorf("unexpected LOGIN challenge %q", fromServer)
}

// plainAuth is PLAIN without net/smtp's check for TLS; Send only offers it over TLS.
type plainAuth struct{ user, pass string }

func (a plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
}
func (a plainAuth) Next([]byte, bool) ([]byte, error) { return nil, nil }

// ---- sending ----------------------------------------------------------------

// Send delivers a raw RFC 5322 message to the recipients.
func Send(ctx context.Context, s Settings, from string, to []string, msg []byte) error {
	if !s.Enabled {
		return errors.New("mail server is not configured")
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsCfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: s.SkipVerify} //nolint:gosec // opt-in for self-signed relays
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if s.Security == "tls" {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	deadline := time.Now().Add(60 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp greeting: %w", err)
	}
	defer c.Close()
	host, _ := os.Hostname()
	if host == "" {
		host = "kiestoon"
	}
	if err := c.Hello(host); err != nil {
		return fmt.Errorf("EHLO: %w", err)
	}
	if s.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the server does not offer STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if s.Auth != "none" {
		var auth smtp.Auth
		switch s.Auth {
		case "password":
			_, mechs := c.Extension("AUTH")
			if strings.Contains(strings.ToUpper(mechs), "PLAIN") || mechs == "" {
				auth = plainAuth{s.Username, s.Password}
			} else {
				auth = loginAuth{s.Username, s.Password}
			}
		default:
			tok, err := accessToken(ctx, s)
			if err != nil {
				return err
			}
			auth = xoauth2{s.Username, tok}
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("authentication: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", r, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("message rejected: %w", err)
	}
	return c.Quit()
}

// Attachment is a file added to a composed message.
type Attachment struct {
	Name, ContentType string
	Data              []byte
}

func messageID(domain string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}

// Compose builds a UTF-8 text message with optional attachments.
func Compose(s Settings, to, subject, body string, atts ...Attachment) []byte {
	var buf bytes.Buffer
	from := mail.Address{Name: s.FromName, Address: s.FromAddress}
	domain := "kiestoon.local"
	if i := strings.LastIndex(s.FromAddress, "@"); i >= 0 {
		domain = s.FromAddress[i+1:]
	}
	h := textproto.MIMEHeader{}
	h.Set("From", from.String())
	h.Set("To", to)
	h.Set("Subject", mime.QEncoding.Encode("utf-8", subject))
	h.Set("Date", time.Now().Format(time.RFC1123Z))
	h.Set("Message-ID", messageID(domain))
	h.Set("MIME-Version", "1.0")
	h.Set("Auto-Submitted", "auto-generated")
	h.Set("X-Mailer", "Kiestoon")
	textPart := func(w *bytes.Buffer) {
		w.WriteString(strings.ReplaceAll(base64Wrap([]byte(body)), "\n", "\r\n"))
	}
	if len(atts) == 0 {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "base64")
		writeHeader(&buf, h)
		textPart(&buf)
		return buf.Bytes()
	}
	var parts bytes.Buffer
	mw := multipart.NewWriter(&parts)
	h.Set("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
	writeHeader(&buf, h)
	tw, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"base64"}})
	var tb bytes.Buffer
	textPart(&tb)
	_, _ = tw.Write(tb.Bytes())
	for _, a := range atts {
		aw, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {a.ContentType + "; name=\"" + a.Name + "\""},
			"Content-Disposition":       {"attachment; filename=\"" + a.Name + "\""},
			"Content-Transfer-Encoding": {"base64"},
		})
		_, _ = aw.Write([]byte(strings.ReplaceAll(base64Wrap(a.Data), "\n", "\r\n")))
	}
	mw.Close()
	buf.Write(parts.Bytes())
	return buf.Bytes()
}

// ComposeHTML builds a multipart/alternative message with a plain text and an HTML version.
func ComposeHTML(s Settings, to, subject, text, html string) []byte {
	var buf bytes.Buffer
	from := mail.Address{Name: s.FromName, Address: s.FromAddress}
	domain := "kiestoon.local"
	if i := strings.LastIndex(s.FromAddress, "@"); i >= 0 {
		domain = s.FromAddress[i+1:]
	}
	var parts bytes.Buffer
	mw := multipart.NewWriter(&parts)
	h := textproto.MIMEHeader{}
	h.Set("From", from.String())
	h.Set("To", to)
	h.Set("Subject", mime.QEncoding.Encode("utf-8", subject))
	h.Set("Date", time.Now().Format(time.RFC1123Z))
	h.Set("Message-ID", messageID(domain))
	h.Set("MIME-Version", "1.0")
	h.Set("Auto-Submitted", "auto-generated")
	h.Set("X-Mailer", "Kiestoon")
	h.Set("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	writeHeader(&buf, h)
	for _, p := range []struct{ ct, body string }{{"text/plain; charset=utf-8", text}, {"text/html; charset=utf-8", html}} {
		w, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {p.ct}, "Content-Transfer-Encoding": {"base64"}})
		_, _ = w.Write([]byte(strings.ReplaceAll(base64Wrap([]byte(p.body)), "\n", "\r\n")))
	}
	mw.Close()
	buf.Write(parts.Bytes())
	return buf.Bytes()
}

func writeHeader(buf *bytes.Buffer, h textproto.MIMEHeader) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			buf.WriteString(k + ": " + strings.NewReplacer("\r", "", "\n", "").Replace(v) + "\r\n")
		}
	}
	buf.WriteString("\r\n")
}

func base64Wrap(b []byte) string {
	enc := base64.StdEncoding.EncodeToString(b)
	var sb strings.Builder
	for len(enc) > 76 {
		sb.WriteString(enc[:76] + "\n")
		enc = enc[76:]
	}
	sb.WriteString(enc + "\n")
	return sb.String()
}

// ---- queue ------------------------------------------------------------------

// QueueDir is where `kiestoon sendmail` (run by Asterisk for voicemail e-mails) drops messages.
func QueueDir(dataDir string) string { return filepath.Join(dataDir, "mailq") }

// Enqueue stores a raw message in the queue. Used by `kiestoon sendmail`.
func Enqueue(dir string, msg []byte) error {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := fmt.Sprintf("%d-%s.eml", time.Now().UnixNano(), hex.EncodeToString(b))
	tmp := filepath.Join(dir, "."+name)
	if err := os.WriteFile(tmp, msg, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

// Service sends queued and direct mail and keeps a log.
type Service struct {
	DB       *sql.DB
	QueueDir string
	// Prepare can change a queued message once before it is sent; nil leaves it as it is.
	Prepare  func(ctx context.Context, raw []byte) []byte
	mu       sync.Mutex
	attempts map[string]retry
}

type retry struct {
	n    int
	next time.Time
}

func (m *Service) log(ctx context.Context, to, subject, kind string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
	}
	ok := 0
	if err == nil {
		ok = 1
	}
	_, _ = m.DB.ExecContext(ctx, `INSERT INTO mail_log(recipient, subject, kind, ok, error) VALUES (?,?,?,?,?)`, to, subject, kind, ok, msg)
	_, _ = m.DB.ExecContext(ctx, `DELETE FROM mail_log WHERE id <= (SELECT MAX(id) - 1000 FROM mail_log)`)
}

// SendText composes and sends a plain text e-mail.
func (m *Service) SendText(ctx context.Context, kind, to, subject, body string, atts ...Attachment) error {
	s, err := Load(ctx, m.DB)
	if err != nil {
		return err
	}
	err = Send(ctx, s, s.FromAddress, []string{to}, Compose(s, to, subject, body, atts...))
	m.log(ctx, to, subject, kind, err)
	return err
}

// SendHTML composes and sends an e-mail with a text and an HTML version.
func (m *Service) SendHTML(ctx context.Context, kind, to, subject, text, html string) error {
	s, err := Load(ctx, m.DB)
	if err != nil {
		return err
	}
	if !s.Enabled {
		return ErrDisabled
	}
	err = Send(ctx, s, s.FromAddress, []string{to}, ComposeHTML(s, to, subject, text, html))
	m.log(ctx, to, subject, kind, err)
	return err
}

// Notify sends a system notification to the notify address, if one is set.
func (m *Service) Notify(ctx context.Context, subject, body string) {
	s, err := Load(ctx, m.DB)
	if err != nil || !s.Enabled || s.NotifyAddress == "" {
		return
	}
	_ = m.SendText(ctx, "notification", s.NotifyAddress, subject, body)
}

// Run sends queued messages every few seconds until ctx ends.
func (m *Service) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.flush(ctx)
		}
	}
}

func (m *Service) flush(ctx context.Context) {
	entries, err := os.ReadDir(m.QueueDir)
	if err != nil {
		return
	}
	s, err := Load(ctx, m.DB)
	if err != nil || !s.Enabled {
		return
	}
	m.mu.Lock()
	if m.attempts == nil {
		m.attempts = map[string]retry{}
	}
	m.mu.Unlock()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".eml") {
			continue
		}
		m.mu.Lock()
		r := m.attempts[name]
		m.mu.Unlock()
		if time.Now().Before(r.next) {
			continue
		}
		path := filepath.Join(m.QueueDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Prepare may add to a message once (the voicemail transcript); the result is stored, so a
		// retry does not repeat the work.
		if m.Prepare != nil && !bytes.Contains(headerBlock(raw), []byte("X-Kiestoon-Prepared:")) {
			if out := m.Prepare(ctx, raw); out != nil {
				raw = append([]byte("X-Kiestoon-Prepared: 1\r\n"), out...)
				_ = os.WriteFile(path, raw, 0o640)
			}
		}
		msg, rcpts, subject, err := prepareQueued(raw, s)
		if err == nil {
			err = Send(ctx, s, s.FromAddress, rcpts, msg)
		}
		m.log(ctx, strings.Join(rcpts, ", "), subject, "voicemail", err)
		m.mu.Lock()
		if err == nil {
			delete(m.attempts, name)
			m.mu.Unlock()
			_ = os.Remove(path)
			continue
		}
		// Back off: 30 s, 1 min, 2 min … up to one hour between attempts.
		r.n++
		wait := time.Duration(30<<min(r.n-1, 7)) * time.Second
		r.next = time.Now().Add(min(wait, time.Hour))
		m.attempts[name] = r
		m.mu.Unlock()
		// Give up after a day.
		if info, ierr := e.Info(); ierr == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.MkdirAll(filepath.Join(m.QueueDir, "failed"), 0o750)
			_ = os.Rename(path, filepath.Join(m.QueueDir, "failed", name))
		}
	}
}

// headerBlock is the part of a message before the first empty line.
func headerBlock(raw []byte) []byte {
	if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		raw = raw[:i]
	}
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// prepareQueued reads recipients from the message headers (like sendmail -t) and replaces the
// sender with the configured address, so SPF and DKIM of the mail provider apply.
func prepareQueued(raw []byte, s Settings) ([]byte, []string, string, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, "", err
	}
	var rcpts []string
	for _, h := range []string{"To", "Cc", "Bcc"} {
		if list, err := msg.Header.AddressList(h); err == nil {
			for _, a := range list {
				rcpts = append(rcpts, a.Address)
			}
		}
	}
	if len(rcpts) == 0 {
		return nil, nil, "", errors.New("message has no recipients")
	}
	dec := new(mime.WordDecoder)
	subject, _ := dec.DecodeHeader(msg.Header.Get("Subject"))
	// Rebuild the header block without From/Sender/Bcc, then add our own From.
	var out bytes.Buffer
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	sep := 4
	if headerEnd < 0 {
		headerEnd = bytes.Index(raw, []byte("\n\n"))
		sep = 2
	}
	if headerEnd < 0 {
		return nil, nil, "", errors.New("message has no body")
	}
	lines := strings.Split(strings.ReplaceAll(string(raw[:headerEnd]), "\r\n", "\n"), "\n")
	skip := false
	for _, l := range lines {
		if l != "" && (l[0] == ' ' || l[0] == '\t') {
			if !skip {
				out.WriteString(l + "\r\n")
			}
			continue
		}
		k, _, _ := strings.Cut(l, ":")
		k = strings.ToLower(strings.TrimSpace(k))
		skip = k == "from" || k == "sender" || k == "bcc" || k == "return-path"
		if !skip {
			out.WriteString(l + "\r\n")
		}
	}
	from := mail.Address{Name: s.FromName, Address: s.FromAddress}
	out.WriteString("From: " + from.String() + "\r\n\r\n")
	body := raw[headerEnd+sep:]
	body = bytes.ReplaceAll(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
	out.Write(body)
	return out.Bytes(), rcpts, subject, nil
}

// LogEntry is a line of the mail log.
type LogEntry struct {
	ID        int64  `json:"id"`
	At        string `json:"at"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Kind      string `json:"kind"`
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
}

// Log returns the last 100 sent messages.
func (m *Service) Log(ctx context.Context) ([]LogEntry, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT id, at, recipient, subject, kind, ok, error FROM mail_log ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		var ok int
		if err := rows.Scan(&e.ID, &e.At, &e.Recipient, &e.Subject, &e.Kind, &ok, &e.Error); err != nil {
			return nil, err
		}
		e.OK = ok == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// Pending counts messages waiting in the queue.
func (m *Service) Pending() int {
	entries, _ := os.ReadDir(m.QueueDir)
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".eml") && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}
