package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

func TestDiscoverAdoptAndAssignPhone(t *testing.T) {
	c, s := adminClient(t)
	ctx := context.Background()
	prov, _ := provision.Load(ctx, s.DB)
	fetch := func(file, ua string) int {
		req := httptest.NewRequest("GET", "/prov/"+file, nil)
		req.SetBasicAuth(prov.Username, prov.Password)
		req.Header.Set("User-Agent", ua)
		rec := httptest.NewRecorder()
		s.ProvisioningHandler().ServeHTTP(rec, req)
		return rec.Code
	}
	// An unknown phone with the right credentials is recorded with its model.
	if code := fetch("001565ABCDEF.cfg", "Yealink SIP-T54W 96.86.0.70 00:15:65:ab:cd:ef"); code != 404 {
		t.Fatalf("unknown phone served: %d", code)
	}
	fetch("001565abcdef.cfg", "Yealink SIP-T54W 96.86.0.70 00:15:65:ab:cd:ef")
	// Without the credentials nothing is recorded.
	rec := httptest.NewRecorder()
	s.ProvisioningHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/prov/0015650000aa.cfg", nil))
	list := mustDoList(t, c, "/api/phones/discovered")
	if len(list) != 1 || list[0]["mac"] != "001565abcdef" || list[0]["model"] != "T54W" || list[0]["vendor"] != "yealink" || list[0]["hits"] != float64(2) {
		t.Fatalf("discovered: %v", list)
	}

	e1 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "transport": "udp", "enabled": true, "maxContacts": 3, "ringSeconds": 25}, 200)
	e2 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "202", "name": "Bob", "transport": "udp", "enabled": true, "maxContacts": 3, "ringSeconds": 25}, 200)

	mustDo(t, c, "POST", "/api/phones/discovered/001565abcdef/adopt", map[string]any{"vendor": "yealink", "model": "NOPE"}, 400)
	ph := mustDo(t, c, "POST", "/api/phones/discovered/001565abcdef/adopt", map[string]any{"vendor": "yealink", "model": "T54W", "extensionId": e1["id"]}, 200)
	if len(mustDoList(t, c, "/api/phones/discovered")) != 0 {
		t.Fatal("adopted phone still discovered")
	}
	// Now the phone gets its configuration with the account of 201.
	req := httptest.NewRequest("GET", "/prov/001565abcdef.cfg", nil)
	req.SetBasicAuth(prov.Username, prov.Password)
	rec = httptest.NewRecorder()
	s.ProvisioningHandler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "account.1.user_name = 201") {
		t.Fatalf("config after adopt: %d %s", rec.Code, rec.Body.String())
	}

	// Reassign to 202: the phone is not registered here, so it cannot be told.
	id := jsonID(ph["id"])
	out := mustDo(t, c, "PUT", "/api/phones/"+id+"/assign", map[string]any{"extensionId": e2["id"]}, 200)
	if out["resync"] != "failed" || out["message"] != "asterisk_offline" || out["phone"].(map[string]any)["extensionId"] != e2["id"] {
		t.Fatalf("assign: %v", out)
	}
	mustDo(t, c, "PUT", "/api/phones/"+id+"/assign", map[string]any{"extensionId": 99999}, 400)
	out = mustDo(t, c, "PUT", "/api/phones/"+id+"/assign", map[string]any{"extensionId": nil, "resync": false}, 200)
	if out["resync"] != "skipped" || out["phone"].(map[string]any)["extensionId"] != nil {
		t.Fatalf("unassign: %v", out)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/prov/001565abcdef.cfg", nil)
	req.SetBasicAuth(prov.Username, prov.Password)
	s.ProvisioningHandler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "account.1.enable = 0") {
		t.Fatalf("config after unassign: %s", rec.Body.String())
	}
	mustDo(t, c, "POST", "/api/phones/"+id+"/resync", map[string]any{}, 200)

	// Dismissing a discovery.
	fetch("cfg000b82abcdef.xml", "Grandstream Model HW GRP2614 SW 1.0.5.33")
	mustDo(t, c, "DELETE", "/api/phones/discovered/000b82abcdef", map[string]any{}, 200)
	mustDo(t, c, "DELETE", "/api/phones/discovered/000b82abcdef", map[string]any{}, 404)
}
