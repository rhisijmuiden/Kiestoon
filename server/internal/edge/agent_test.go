package edge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/wg"
)

type fakeSys struct {
	mu    sync.Mutex
	files map[string]string
	cmds  []string
}

func (f *fakeSys) WriteFile(path string, data []byte, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = string(data)
	return nil
}

func (f *fakeSys) Run(ctx context.Context, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	if name == "systemctl" && args[0] == "is-active" {
		return "active\n", nil
	}
	return "", nil
}

func TestPairAndApply(t *testing.T) {
	_, hubPub, _ := wg.NewKey()
	var got multisite.EdgePairRequest
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/edge/pair" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		if got.Secret != "s3cret" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"invalid_pairing_code"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(multisite.EdgePairResponse{SiteID: 3, Name: "Utrecht", Token: "kts_abc", Address: "10.70.3.2/32",
			HubAddress: "10.70.3.1", HubPublicKey: hubPub, Endpoint: "pbx.example.com:51820", SIPPort: 5060, PortalPort: 8080})
	}))
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	fp := hex.EncodeToString(sum[:])

	dir := t.TempDir()
	fs := &fakeSys{files: map[string]string{}}
	a := NewAgent(dir, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.Sys = fs
	ctx := context.Background()

	bad := multisite.Code{Kind: multisite.KindEdge, URL: srv.URL, Fingerprint: fp, Secret: "wrong"}.Encode()
	if err := a.Pair(ctx, bad); err == nil {
		t.Fatal("wrong secret accepted")
	}
	wrongFP := multisite.Code{Kind: multisite.KindEdge, URL: srv.URL, Fingerprint: strings.Repeat("0", 64), Secret: "s3cret"}.Encode()
	if err := a.Pair(ctx, wrongFP); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	code := multisite.Code{Kind: multisite.KindEdge, URL: srv.URL, Fingerprint: fp, Secret: "s3cret"}.Encode()
	if err := a.Pair(ctx, code); err != nil {
		t.Fatal(err)
	}
	if !wg.ValidKey(got.PublicKey) || got.LANAddress == "" {
		t.Fatalf("pair request %+v", got)
	}
	st := LoadState(dir)
	if !st.Paired || st.Token != "kts_abc" || st.TunnelIP() != "10.70.3.2" {
		t.Fatalf("state %+v", st)
	}
	conf := fs.files["/etc/wireguard/kt-edge.conf"]
	if !strings.Contains(conf, "Endpoint = pbx.example.com:51820") || !strings.Contains(conf, "AllowedIPs = 10.70.3.1/32") || !strings.Contains(conf, "PersistentKeepalive = 25") {
		t.Fatalf("wireguard conf:\n%s", conf)
	}
	if !strings.Contains(fs.files["/etc/kamailio/kamailio.cfg"], `$du = "sip:10.70.3.1:5060"`) {
		t.Fatal("kamailio not written")
	}
	if !strings.Contains(fs.files["/etc/rtpengine/rtpengine.conf"], "wg/10.70.3.2") {
		t.Fatal("rtpengine not written")
	}
	if err := a.Pair(ctx, code); err != ErrPaired {
		t.Fatalf("second pairing: %v", err)
	}
	// Nothing changed: no restarts.
	n := len(fs.cmds)
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fs.cmds) != n {
		t.Fatalf("needless restarts: %v", fs.cmds[n:])
	}
	if err := a.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if LoadState(dir).Paired {
		t.Fatal("still paired after reset")
	}
}

func TestPairFromLocalPage(t *testing.T) {
	a := NewAgent(t.TempDir(), "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.Sys = &fakeSys{files: map[string]string{}}
	h := a.Handler()
	req := httptest.NewRequest("POST", "/api/pair", strings.NewReader(`{"code":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin: %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/pair", strings.NewReader(`{"code":"nonsense"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_code") {
		t.Fatalf("bad code: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Kiestoon Edge") {
		t.Fatalf("page: %d", w.Code)
	}
}
