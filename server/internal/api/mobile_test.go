package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/push"
)

type recSender struct {
	mu   sync.Mutex
	got  []push.Message
	gone bool
}

func (r *recSender) Send(_ context.Context, m push.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gone {
		return push.ErrGone
	}
	r.got = append(r.got, m)
	return nil
}

func (r *recSender) take() []push.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.got
	r.got = nil
	return out
}

func TestMobileApp(t *testing.T) {
	c, s := adminClient(t)
	snd := &recSender{}
	rl := &push.Relay{Secret: []byte(strings.Repeat("k", 32)), Senders: map[string]push.Sender{push.APNs: snd, push.FCM: snd}}
	relay := httptest.NewServer(rl.Handler())
	defer relay.Close()
	s.mobile.relay = &push.Client{URL: relay.URL, Key: s.installKey(context.Background())}

	e := mustDo(t, c, "POST", "/api/pbx/extensions?welcome=1&lang=en", map[string]any{"number": "201", "name": "Anna",
		"email": "anna@example.com", "enabled": true, "voicemail": true}, 200)
	link := e["welcome"].(map[string]any)["link"].(string)
	jar, _ := cookiejar.New(nil)
	anna := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}
	mustDo(t, anna, "POST", "/api/auth/password-link/set", map[string]string{"token": link[strings.Index(link, "#")+1:], "password": "a long enough password"}, 200)
	mustDo(t, anna, "POST", "/api/auth/login", map[string]string{"username": "anna@example.com", "password": "a long enough password"}, 200)

	// The app links like the Windows softphone.
	p := mustDo(t, anna, "POST", "/api/desktop/pair", map[string]any{}, 200)
	jar2, _ := cookiejar.New(nil)
	app := &client{t: t, url: c.url, hc: &http.Client{Jar: jar2}}
	mustDo(t, app, "POST", "/api/desktop/claim", map[string]string{"code": p["code"].(string)}, 200)

	reg := map[string]any{"appId": "A1", "name": "iPhone van Anna", "platform": "apns", "token": "devtoken", "sandbox": true}
	// Off by default.
	mustDo(t, app, "POST", "/api/portal/mobile/register", reg, 409)
	mustDo(t, c, "PUT", "/api/mobile/settings", map[string]any{"enabled": true, "relayURL": "http://insecure"}, 400)
	mustDo(t, c, "PUT", "/api/mobile/settings", map[string]any{"enabled": true, "relayURL": ""}, 200)
	mustDo(t, app, "POST", "/api/portal/mobile/register", map[string]any{"appId": "A1", "platform": "blackberry", "token": "x"}, 400)
	r1 := mustDo(t, app, "POST", "/api/portal/mobile/register", reg, 200)
	secret := r1["secret"].(string)
	// Registering again keeps the device.
	r2 := mustDo(t, app, "POST", "/api/portal/mobile/register", reg, 200)
	if r2["id"] != r1["id"] {
		t.Fatalf("re-register made a new device: %v %v", r1, r2)
	}
	secret = r2["secret"].(string)

	// The token is not stored, only the sealed handle.
	var handle string
	_ = s.DB.QueryRow(`SELECT handle FROM mobile_devices`).Scan(&handle)
	if strings.Contains(handle, "devtoken") || handle == "" {
		t.Fatalf("handle %q", handle)
	}
	sip := mustDo(t, app, "GET", "/api/portal/mobile/sip", nil, 200)
	if sip["username"] != "201-app" || sip["password"] == "" {
		t.Fatalf("sip %v", sip)
	}
	st := mustDo(t, c, "GET", "/api/mobile/settings", nil, 200)
	if st["publicKey"] == "" || len(st["devices"].([]any)) != 1 {
		t.Fatalf("settings %v", st)
	}
	if strings.Contains(fmt.Sprint(st), secret) || strings.Contains(fmt.Sprint(st), handle) {
		t.Fatal("admin view leaks the device secret or handle")
	}

	// The dialplan rings the app through the wake context.
	snap, err := s.PBX.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, x := range snap.Extensions {
		if x.Number == "201" {
			found = x.MobileApp
		}
	}
	if !found {
		t.Fatal("snapshot does not mark the app")
	}

	// A wake pushes the caller, encrypted for the phone.
	ev := func(kv ...string) asterisk.Message {
		m := asterisk.Message{}
		for i := 0; i+1 < len(kv); i += 2 {
			m = append(m, asterisk.KV{Key: kv[i], Value: kv[i+1]})
		}
		return m
	}
	s.onMobileWake(ev("Event", "UserEvent", "UserEvent", "KiestoonWake", "Ext", "201", "WakeID", "1700.5",
		"From", url.QueryEscape("0612345678"), "FromName", url.QueryEscape("Jan, de Vries")))
	got := snd.take()
	if len(got) != 1 || got[0].Kind != push.KindCall || got[0].ID != "1700.5" || got[0].Token != "devtoken" || !got[0].Sandbox {
		t.Fatalf("wake push %+v", got)
	}
	var data map[string]any
	if err := push.Open(secret, got[0].Data, &data); err != nil || data["from"] != "0612345678" || data["fromName"] != "Jan, de Vries" {
		t.Fatalf("payload %v %v", data, err)
	}
	s.onMobileWakeEnd(ev("Ext", "201", "WakeID", "1700.5", "Status", ""))
	if got := snd.take(); len(got) != 1 || got[0].Kind != push.KindCancel {
		t.Fatalf("cancel %+v", got)
	}
	// Voicemail: only when the number of new messages goes up.
	s.onMobileMWI(ev("Mailbox", "201@default", "New", "1"))
	s.onMobileMWI(ev("Mailbox", "201@default", "New", "0"))
	if got := snd.take(); len(got) != 1 || got[0].Kind != push.KindVoicemail {
		t.Fatalf("voicemail %+v", got)
	}

	// Test from the portal.
	id := fmt.Sprint(r1["id"])
	mustDo(t, anna, "POST", "/api/portal/mobile/devices/"+id+"/test", map[string]any{}, 200)
	if got := snd.take(); len(got) != 1 || got[0].Kind != push.KindTest {
		t.Fatalf("test %+v", got)
	}
	// Another user cannot touch the phone.
	mustDo(t, c, "POST", "/api/portal/mobile/devices/"+id+"/test", map[string]any{}, 404)

	// A removed app is forgotten.
	snd.gone = true
	mustDo(t, anna, "POST", "/api/portal/mobile/devices/"+id+"/test", map[string]any{}, 410)
	if l := mustDo(t, anna, "GET", "/api/portal/mobile", nil, 200); len(l["devices"].([]any)) != 0 {
		t.Fatalf("gone device kept: %v", l)
	}
	mustDo(t, app, "GET", "/api/portal/mobile/sip", nil, 409)
}
