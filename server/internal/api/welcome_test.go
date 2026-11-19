package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// fakeSMTP accepts plain SMTP and hands every message to the channel.
func fakeSMTP(t *testing.T) (int, chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				rd := bufio.NewReader(c)
				fmt.Fprint(c, "220 fake\r\n")
				for {
					line, err := rd.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
						fmt.Fprint(c, "250-fake\r\n250 8BITMIME\r\n")
					case cmd == "DATA":
						fmt.Fprint(c, "354 go\r\n")
						var b strings.Builder
						for {
							l, err := rd.ReadString('\n')
							if err != nil {
								return
							}
							if l == ".\r\n" {
								break
							}
							b.WriteString(l)
						}
						out <- b.String()
						fmt.Fprint(c, "250 ok\r\n")
					case cmd == "QUIT":
						fmt.Fprint(c, "221 bye\r\n")
						return
					default:
						fmt.Fprint(c, "250 ok\r\n")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, out
}

func TestWelcomeMailAndPasswordLink(t *testing.T) {
	c, s := adminClient(t)
	s.Cfg.Listen, s.Cfg.TLSListen = ":8080", ":8443"

	// Without a mail server the account is still made and the admin gets the link.
	e := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1&lang=nl", map[string]any{"number": "201", "name": "Anna de Vries",
		"email": "anna@example.com", "enabled": true, "voicemail": true, "webrtc": true}, 200)
	w := e["welcome"].(map[string]any)
	if w["sent"] != false || w["error"] != "mail_disabled" || w["created"] != true || w["username"] != "anna@example.com" {
		t.Fatalf("welcome: %v", w)
	}
	link := w["link"].(string)
	token := link[strings.Index(link, "#")+1:]
	extID := int64(e["id"].(float64))
	u, err := s.Auth.UserByExtension(context.Background(), extID)
	if err != nil || u.Role != "user" || u.Language != "nl" || u.Email != "anna@example.com" {
		t.Fatalf("user: %+v %v", u, err)
	}

	// No welcome without the flag.
	e2 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "202", "name": "Bas", "email": "bas@example.com", "enabled": true}, 200)
	if _, ok := e2["welcome"]; ok {
		t.Fatal("welcome without flag")
	}

	// The set-password page.
	jar, _ := cookiejar.New(nil)
	anon := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	info := mustDo(t, anon, "POST", "/api/auth/password-link", map[string]string{"token": token}, 200)
	if info["username"] != "anna@example.com" || info["language"] != "nl" {
		t.Fatalf("info: %v", info)
	}
	mustDo(t, anon, "POST", "/api/auth/password-link/set", map[string]string{"token": token, "password": "short"}, 400)
	mustDo(t, anon, "POST", "/api/auth/password-link/set", map[string]string{"token": token, "password": "a long enough password"}, 200)
	mustDo(t, anon, "POST", "/api/auth/password-link/set", map[string]string{"token": token, "password": "a long enough password"}, 400)
	mustDo(t, anon, "POST", "/api/auth/login", map[string]string{"username": "anna@example.com", "password": "a long enough password"}, 200)
	me := mustDo(t, anon, "GET", "/api/me", nil, 200)
	if me["extension"] == nil {
		t.Fatalf("portal not linked: %v", me)
	}

	// Resend with a mail server: a real mail in Dutch, with the links and no password.
	port, got := fakeSMTP(t)
	st := mail.Settings{Enabled: true, Host: "127.0.0.1", Port: port, Security: "none", Auth: "none", FromAddress: "pbx@example.com", FromName: "Kiestoon"}
	if err := store.SetJSON(context.Background(), s.DB, mail.SettingsKey, st); err != nil {
		t.Fatal(err)
	}
	r := mustDo(t, c, "POST", fmt.Sprintf("/api/pbx/extensions/%d/welcome", extID), map[string]any{}, 200)
	if r["sent"] != true || r["created"] != false || r["link"] != nil {
		t.Fatalf("resend: %v", r)
	}
	raw := <-got
	if !strings.Contains(raw, "multipart/alternative") || !strings.Contains(raw, "To: anna@example.com") {
		t.Fatalf("mail headers: %s", raw)
	}
	body := decodeParts(t, raw)
	for _, want := range []string{"Wachtwoord instellen", "/set-password#", ":8080", "https://", ":8443", "201", "*97", "Powered by"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mail misses %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "a long enough password") {
		t.Fatal("password in mail")
	}

	// The user endpoint works too, and an extension without e-mail is refused.
	mustDo(t, c, "POST", fmt.Sprintf("/api/users/%d/welcome", u.ID), map[string]any{}, 200)
	<-got
	e3 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "203", "name": "Cas", "enabled": true}, 200)
	mustDo(t, c, "POST", fmt.Sprintf("/api/pbx/extensions/%v/welcome", e3["id"]), map[string]any{}, 400)

	// An existing account with the same e-mail address and no extension gets linked, not duplicated.
	mustDo(t, c, "POST", "/api/users", map[string]any{"username": "cor", "displayName": "Cor", "email": "cor@example.com", "role": "user",
		"enabled": true, "password": "another long password"}, 200)
	e4 := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1", map[string]any{"number": "204", "name": "Cor", "email": "cor@example.com", "enabled": true}, 200)
	if w := e4["welcome"].(map[string]any); w["created"] != false || w["username"] != "cor" || w["sent"] != true {
		t.Fatalf("link existing: %v", w)
	}
	<-got
}

var reB64 = regexp.MustCompile(`(?s)Content-Type: text/[a-z]+; charset=utf-8\r\n\r\n(.*?)\r\n--`)

func decodeParts(t *testing.T, raw string) string {
	t.Helper()
	var b strings.Builder
	for _, m := range reB64.FindAllStringSubmatch(raw, -1) {
		d, err := decodeB64(strings.ReplaceAll(m[1], "\r\n", ""))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(d)
	}
	return b.String()
}

func decodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return string(b), err
}
