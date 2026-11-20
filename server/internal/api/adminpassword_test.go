package api

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

func TestPhoneAdminPassword(t *testing.T) {
	c, s := adminClient(t)
	ctx := context.Background()
	prov, _ := provision.Load(ctx, s.DB)
	if prov.AdminPasswordMode != provision.AdminPerPhone {
		t.Fatalf("default mode: %q", prov.AdminPasswordMode)
	}
	config := func(mac string) string {
		req := httptest.NewRequest("GET", "/prov/"+mac+".cfg", nil)
		req.SetBasicAuth(prov.Username, prov.Password)
		rec := httptest.NewRecorder()
		s.ProvisioningHandler().ServeHTTP(rec, req)
		return rec.Body.String()
	}
	p1 := mustDo(t, c, "POST", "/api/phones", map[string]any{"mac": "001565bb0001", "vendor": "yealink", "model": "T42U", "label": "Desk 1"}, 200)
	p2 := mustDo(t, c, "POST", "/api/phones", map[string]any{"mac": "001565bb0002", "vendor": "yealink", "model": "T42U"}, 200)
	id1, id2 := jsonID(p1["id"]), jsonID(p2["id"])

	v1 := mustDo(t, c, "GET", "/api/phones/"+id1+"/admin-password", nil, 200)
	pw1, _ := v1["password"].(string)
	if len(pw1) != adminPasswordLen || v1["user"] != "admin" || v1["mode"] != "perPhone" || v1["applied"] != false {
		t.Fatalf("view: %v", v1)
	}
	if !strings.Contains(config("001565bb0001"), "static.security.user_password = admin:"+pw1+"\n") {
		t.Fatal("phone 1 config lacks its own password")
	}
	cfg2 := config("001565bb0002")
	if strings.Contains(cfg2, pw1) || strings.Contains(cfg2, prov.AdminPassword) {
		t.Fatal("phone 2 shares a password")
	}
	if v := mustDo(t, c, "GET", "/api/phones/"+id1+"/admin-password", nil, 200); v["password"] != pw1 || v["applied"] != true {
		t.Fatalf("after fetch: %v", v)
	}
	// The phone list and the preview never show it.
	resp, err := c.hc.Get(c.url + "/api/phones")
	if err != nil {
		t.Fatal(err)
	}
	list, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(list), "001565bb0001") || strings.Contains(string(list), pw1) {
		t.Fatal("password in phone list")
	}
	if prev := mustDo(t, c, "GET", "/api/phones/"+id1+"/preview", nil, 200); strings.Contains(prev["body"].(string), pw1) {
		t.Fatal("password in preview")
	}
	// A new password is pending until the phone fetches its configuration.
	n := mustDo(t, c, "POST", "/api/phones/"+id1+"/admin-password", map[string]any{}, 200)
	if n["password"] == pw1 || n["applied"] != false {
		t.Fatalf("new: %v", n)
	}
	var audits int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('phone.admin_password_viewed', 'phone.admin_password_changed')`).Scan(&audits)
	if audits != 3 {
		t.Fatalf("audit entries: %d", audits)
	}

	// One password for all phones.
	set := mustDo(t, c, "GET", "/api/provisioning/settings", nil, 200)
	set["adminPasswordMode"], set["adminPassword"] = "shared", "Kt-Noordkade-2026"
	mustDo(t, c, "PUT", "/api/provisioning/settings", set, 200)
	for _, mac := range []string{"001565bb0001", "001565bb0002"} {
		if !strings.Contains(config(mac), "static.security.user_password = admin:Kt-Noordkade-2026\n") {
			t.Fatalf("%s lacks the shared password", mac)
		}
	}
	if v := mustDo(t, c, "GET", "/api/phones/"+id2+"/admin-password", nil, 200); v["mode"] != "shared" || v["password"] != "Kt-Noordkade-2026" || v["applied"] != true {
		t.Fatalf("shared view: %v", v)
	}
	mustDo(t, c, "POST", "/api/phones/"+id2+"/admin-password", map[string]any{}, 400)
	set["adminPasswordMode"] = "sometimes"
	mustDo(t, c, "PUT", "/api/provisioning/settings", set, 400)

	// Back to one per phone: the phones get their own passwords again, pending until they fetch.
	set["adminPasswordMode"] = "perPhone"
	mustDo(t, c, "PUT", "/api/provisioning/settings", set, 200)
	if v := mustDo(t, c, "GET", "/api/phones/"+id2+"/admin-password", nil, 200); v["applied"] != false || v["password"] == "Kt-Noordkade-2026" {
		t.Fatalf("back to per phone: %v", v)
	}
}
