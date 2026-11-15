package api

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/cluster"
	"github.com/rhisijmuiden/kiestoon/server/internal/config"
	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/version"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

// tlsServer serves a Kiestoon over HTTPS with its own certificate, like the real portal on 8443,
// so pairing codes pin the certificate the other side really sees.
func tlsServer(t *testing.T, admin bool) (*client, *Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, Repo: "o/r", DefaultChannel: "production", Listen: ":8080", TLSListen: ":8443"}
	db, err := store.Open(context.Background(), cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cp, kp, err := certs.Ensure(cfg.PBXDir())
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.LoadX509KeyPair(cp, kp)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(SecurityHeaders(s.Routes()))
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	ts.StartTLS()
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, url: ts.URL, hc: &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}}
	if admin {
		os.WriteFile(SetupTokenPath(dir), []byte("T\n"), 0o600)
		if code, _ := c.do("POST", "/api/setup", map[string]string{"token": "T", "username": "admin", "password": "correct horse battery"}); code != 201 {
			t.Fatal("setup failed")
		}
		_, enr := c.do("POST", "/api/account/mfa/enroll", map[string]any{})
		otp, _ := totp.GenerateCode(enr["secret"].(string), time.Now())
		if st, _ := c.do("POST", "/api/account/mfa/confirm", map[string]string{"code": otp}); st != 200 {
			t.Fatal("mfa confirm failed")
		}
	}
	return c, s, ts.URL
}

func TestEdgePairing(t *testing.T) {
	c, _, url := tlsServer(t, true)
	if code, body := c.do("POST", "/api/sites/edges", map[string]string{"name": "Utrecht"}); code != http.StatusConflict || body["error"] != "hub_disabled" {
		t.Fatalf("edge without hub: %d %v", code, body)
	}
	mustDo(t, c, "PUT", "/api/sites/hub", map[string]any{"enabled": true, "host": "pbx.example.com", "listenPort": 51820}, 200)
	out := mustDo(t, c, "POST", "/api/sites/edges", map[string]string{"name": "Utrecht", "url": url}, 201)
	code, err := multisite.Decode(out["code"].(string), multisite.KindEdge)
	if err != nil || code.URL != url || code.Fingerprint == "" {
		t.Fatalf("code %+v %v", code, err)
	}
	_, pub, _ := wg.NewKey()
	edge := &client{t: t, url: url, hc: multisite.PinnedClient(code.Fingerprint, 5*time.Second)}
	if st, body := edge.do("POST", "/api/edge/pair", map[string]string{"secret": "wrong", "publicKey": pub}); st != 403 || body["error"] != "invalid_pairing_code" {
		t.Fatalf("wrong secret: %d %v", st, body)
	}
	st, pair := edge.do("POST", "/api/edge/pair", map[string]string{"secret": code.Secret, "publicKey": pub, "hostname": "pi", "lanAddress": "192.168.5.20", "lanNet": "192.168.5.0/24"})
	if st != 200 || pair["token"] == "" || !strings.HasSuffix(pair["address"].(string), "/32") || pair["endpoint"] != "pbx.example.com:51820" {
		t.Fatalf("pair: %d %v", st, pair)
	}
	if st, _ := edge.do("POST", "/api/edge/pair", map[string]string{"secret": code.Secret, "publicKey": pub}); st != 403 {
		t.Fatal("pairing code used twice")
	}
	// The heartbeat only counts when it comes through the tunnel.
	req, _ := http.NewRequest("POST", url+"/api/edge/heartbeat", strings.NewReader(`{"version":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pair["token"].(string))
	resp, err2 := edge.hc.Do(req)
	if err2 != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("heartbeat outside the tunnel: %v %v", resp, err2)
	}
	resp.Body.Close()
	sites := mustDo(t, c, "GET", "/api/sites", nil, 200)
	edges := sites["edges"].([]any)
	if len(edges) != 1 || edges[0].(map[string]any)["paired"] != true || edges[0].(map[string]any)["lanAddress"] != "192.168.5.20" {
		t.Fatalf("sites: %v", sites)
	}
	if h := sites["hub"].(map[string]any); h["privateKey"] != nil && h["privateKey"] != "" {
		t.Fatal("hub private key leaked")
	}
}

func TestFederationPairing(t *testing.T) {
	a, _, urlA := tlsServer(t, true)
	b, _, _ := tlsServer(t, true)
	mustDo(t, a, "PUT", "/api/sites/hub", map[string]any{"enabled": true, "host": "nl.example.com", "listenPort": 51820}, 200)
	inv := mustDo(t, a, "POST", "/api/federation/invites", map[string]string{"name": "New York", "prefix": "8", "url": urlA}, 201)
	if st, body := b.do("POST", "/api/federation/join", map[string]string{"code": "KT1-F-nonsense", "prefix": "7"}); st != 400 {
		t.Fatalf("bad code: %d %v", st, body)
	}
	mustDo(t, b, "POST", "/api/federation/join", map[string]string{"code": inv["code"].(string), "name": "Amsterdam", "prefix": "4"}, 201)
	if st, _ := b.do("POST", "/api/federation/join", map[string]string{"code": inv["code"].(string), "name": "Again", "prefix": "6"}); st == 201 {
		t.Fatal("invitation used twice")
	}
	pa := mustDo(t, a, "GET", "/api/federation", nil, 200)["peers"].([]any)
	pb := mustDo(t, b, "GET", "/api/federation", nil, 200)["peers"].([]any)
	if len(pa) != 1 || pa[0].(map[string]any)["state"] != "active" || pa[0].(map[string]any)["role"] != "hosted" {
		t.Fatalf("a: %v", pa)
	}
	if len(pb) != 1 || pb[0].(map[string]any)["role"] != "joined" || pb[0].(map[string]any)["prefix"] != "4" || pb[0].(map[string]any)["endpoint"] != "nl.example.com:51820" {
		t.Fatalf("b: %v", pb)
	}
	// The peer API needs the token and the tunnel.
	if st, _ := (&client{t: t, url: urlA, hc: a.hc}).do("GET", "/api/fed/v1/directory", nil); st != 401 && st != 403 {
		t.Fatalf("directory without token: %d", st)
	}
	// Remote destinations show up for routing.
	dests := mustDo(t, b, "GET", "/api/pbx/destinations", nil, 200)
	found := false
	for _, ty := range dests["types"].([]any) {
		found = found || ty == "followsun"
	}
	if !found {
		t.Fatalf("destination types: %v", dests["types"])
	}
}

func TestClusterJoin(t *testing.T) {
	a, sa, urlA := tlsServer(t, true)
	b, sb, _ := tlsServer(t, false)
	out := mustDo(t, a, "POST", "/api/cluster/enable", map[string]string{"mode": "manual", "address": "192.0.2.10", "url": urlA}, 200)
	if _, err := multisite.Decode(out["code"].(string), multisite.KindCluster); err != nil {
		t.Fatal(err)
	}
	if st, _ := a.do("POST", "/api/cluster/enable", map[string]string{"mode": "vip", "vip": "bad", "address": "192.0.2.10"}); st != 400 {
		t.Fatal("bad VIP accepted")
	}
	out = mustDo(t, a, "POST", "/api/cluster/enable", map[string]string{"mode": "manual", "address": "192.0.2.10", "url": urlA}, 200)
	os.WriteFile(SetupTokenPath(sb.Cfg.DataDir), []byte("S\n"), 0o600)
	if st, _ := b.do("POST", "/api/setup/cluster-join", map[string]string{"token": "wrong", "code": out["code"].(string), "address": "192.0.2.11"}); st != 403 {
		t.Fatalf("join without setup token: %d", st)
	}
	mustDo(t, b, "POST", "/api/setup/cluster-join", map[string]string{"token": "S", "code": out["code"].(string), "address": "192.0.2.11"}, 200)
	if !cluster.IsStandby(sb.Cfg.DataDir) {
		t.Fatal("b is not standby")
	}
	na := cluster.LoadNode(sa.Cfg.DataDir)
	if na.Peer == nil || na.Peer.Address != "192.0.2.11" || na.JoinHash != "" {
		t.Fatalf("a: %+v", na)
	}
	nb := cluster.LoadNode(sb.Cfg.DataDir)
	if nb.Secret != na.Secret || nb.Peer.PublicKey != na.PublicKey || nb.Tunnel != na.Peer.Tunnel {
		t.Fatal("pair does not match")
	}
	if cluster.LoadRole(sa.Cfg.DataDir).Role != cluster.RolePrimary {
		t.Fatal("a is not primary")
	}
	// The replication API answers only with the shared secret.
	req, _ := http.NewRequest("GET", urlA+"/api/cluster/v1/heartbeat", nil)
	resp, err := a.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("heartbeat without secret")
	}
	_ = version.Version
}
