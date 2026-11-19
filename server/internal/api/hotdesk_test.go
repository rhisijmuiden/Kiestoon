package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

func TestHotdeskLoginLogout(t *testing.T) {
	c, s := adminClient(t)
	ctx := context.Background()
	prov, _ := provision.Load(ctx, s.DB)
	config := func(mac string) string {
		req := httptest.NewRequest("GET", "/prov/"+mac+".cfg", nil)
		req.SetBasicAuth(prov.Username, prov.Password)
		rec := httptest.NewRecorder()
		s.ProvisioningHandler().ServeHTTP(rec, req)
		return rec.Body.String()
	}
	guest1 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "290", "name": "Flex 1", "enabled": true}, 200)
	guest2 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "291", "name": "Flex 2", "enabled": true}, 200)
	anna := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true, "voicemail": true, "voicemailPin": "4321"}, 200)
	mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "202", "name": "Bas", "enabled": true}, 200)
	desk := map[string]any{"mac": "001565ee0001", "vendor": "yealink", "model": "T42U", "extensionId": guest1["id"],
		"options": map[string]any{"hotdesk": map[string]any{"enabled": true, "autoLogout": true}}}
	d1 := mustDo(t, c, "POST", "/api/phones", desk, 200)
	desk["mac"], desk["extensionId"] = "001565ee0002", guest2["id"]
	d2 := mustDo(t, c, "POST", "/api/phones", desk, 200)
	mustDo(t, c, "POST", "/api/phones", map[string]any{"mac": "001565ee0003", "vendor": "yealink", "model": "T42U", "extensionId": anna["id"]}, 200)

	if !strings.Contains(config("001565ee0001"), "account.1.user_name = 290") {
		t.Fatal("guest account missing")
	}
	// Wrong PIN, unknown number, extension without PIN, a call from a phone that is no hot desk.
	for _, try := range [][4]string{{"login", "290", "201", "1111"}, {"login", "290", "209", "4321"}, {"login", "290", "202", ""}, {"login", "201", "201", "4321"}} {
		want := hdWrong
		if try[1] == "201" {
			want = hdNotDesk
		}
		if got := s.hotdeskAGI(ctx, try[0], try[1], "10.0.0.5:5060", try[2], try[3]); got != want {
			t.Errorf("%v: %s, want %s", try, got, want)
		}
	}
	if got := s.hotdeskAGI(ctx, "login", "290", "10.0.0.5:5060", "201", "4321"); got != hdOK {
		t.Fatalf("login: %s", got)
	}
	cfg := config("001565ee0001")
	if !strings.Contains(cfg, "account.1.user_name = 201") || strings.Contains(cfg, "user_name = 290") {
		t.Fatalf("config after login: %s", cfg)
	}
	// Anna moves to desk 2: desk 1 is free again.
	if got := s.hotdeskAGI(ctx, "login", "291", "", "201", "4321"); got != hdOK {
		t.Fatalf("login desk 2: %s", got)
	}
	if !strings.Contains(config("001565ee0001"), "account.1.user_name = 290") || !strings.Contains(config("001565ee0002"), "account.1.user_name = 201") {
		t.Fatal("anna not moved to desk 2")
	}
	// *56 from desk 2 (registered as 201 now).
	if got := s.hotdeskAGI(ctx, "logout", "201", "", "", ""); got != hdOK {
		t.Fatalf("logout: %s", got)
	}
	if got := s.hotdeskAGI(ctx, "logout", "291", "", "", ""); got != hdNotDesk {
		t.Fatalf("logout of a free desk: %s", got)
	}
	// The admin logs Anna in, and the night logs her out.
	id1 := jsonID(d1["id"])
	p := mustDo(t, c, "PUT", "/api/phones/"+id1+"/hotdesk", map[string]any{"extensionId": anna["id"]}, 200)
	if p["hotdeskExtensionId"] != anna["id"] || p["hotdeskAt"] == "" {
		t.Fatalf("admin login: %v", p)
	}
	// Saving the phone keeps the login; switching hot desking off ends it.
	desk["mac"], desk["extensionId"] = "001565ee0001", guest1["id"]
	p = mustDo(t, c, "PUT", "/api/phones/"+id1, desk, 200)
	if p["hotdeskExtensionId"] != anna["id"] {
		t.Fatalf("save lost login: %v", p)
	}
	if n := s.hotdeskLogoutAll(ctx); n != 1 {
		t.Fatalf("auto logout: %d", n)
	}
	mustDo(t, c, "PUT", "/api/phones/"+id1+"/hotdesk", map[string]any{"extensionId": anna["id"]}, 200)
	desk["options"] = map[string]any{"hotdesk": map[string]any{"enabled": false}}
	p = mustDo(t, c, "PUT", "/api/phones/"+id1, desk, 200)
	if p["hotdeskExtensionId"] != nil || !strings.Contains(config("001565ee0001"), "account.1.user_name = 290") {
		t.Fatalf("hot desking off: %v", p)
	}
	mustDo(t, c, "PUT", "/api/phones/"+id1+"/hotdesk", map[string]any{"extensionId": anna["id"]}, 400)
	_ = d2

	// Five wrong PINs lock the desk for a while, even for the right PIN.
	for i := 0; i < hdMaxFails; i++ {
		s.hotdeskAGI(ctx, "login", "291", "", "201", "0000")
	}
	if got := s.hotdeskAGI(ctx, "login", "291", "", "201", "4321"); got != hdLocked {
		t.Fatalf("lock: %s", got)
	}
}
