package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

func TestPhoneKeysModulesAndTemplates(t *testing.T) {
	c, s := adminClient(t)
	e1 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "201", "name": "Anna", "enabled": true}, 200)
	e2 := mustDo(t, c, "POST", "/api/pbx/extensions", map[string]any{"number": "250", "name": "Sales", "enabled": true}, 200)
	phone := map[string]any{"mac": "001565aa0001", "vendor": "yealink", "model": "T54W", "extensionId": e1["id"], "lines": []any{e2["id"]},
		"keys":    []map[string]string{{"type": "line"}, {"type": "line", "value": "2"}, {"type": "parkslot", "value": "71"}, {"type": "intercom", "value": "250"}},
		"modules": []map[string]any{{"type": "EXP50", "keys": []map[string]string{{"type": "blf", "value": "250", "label": "Sales"}}}}}
	p := mustDo(t, c, "POST", "/api/phones", phone, 200)
	if p["options"].(map[string]any)["intercom"].(map[string]any)["allow"] != true {
		t.Fatalf("default intercom options: %v", p["options"])
	}
	// Invalid layouts are refused: a module that does not fit, a line beyond the accounts, a bad park slot.
	bad := func(field string, v any) {
		t.Helper()
		q := map[string]any{}
		for k, x := range phone {
			q[k] = x
		}
		q["mac"], q[field] = "001565aa0002", v
		mustDo(t, c, "POST", "/api/phones", q, 400)
	}
	bad("modules", []map[string]any{{"type": "EXP40", "keys": []any{}}})
	bad("modules", []map[string]any{{"type": "EXP50"}, {"type": "EXP50"}, {"type": "EXP50"}, {"type": "EXP50"}})
	bad("keys", []map[string]string{{"type": "line", "value": "17"}})
	bad("keys", []map[string]string{{"type": "parkslot", "value": "80"}})
	bad("keys", []map[string]string{{"type": "fax"}})

	prov, _ := provision.Load(context.Background(), s.DB)
	req := httptest.NewRequest("GET", "/prov/001565aa0001.cfg", nil)
	req.SetBasicAuth(prov.Username, prov.Password)
	rec := httptest.NewRecorder()
	s.ProvisioningHandler().ServeHTTP(rec, req)
	for _, want := range []string{"account.2.user_name = 250", "linekey.2.line = 2", "linekey.4.value = *80250", "expansion_module.1.key.1.value = 250"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("config: missing %q", want)
		}
	}

	// Templates: saved per model, applied to phones of that model only.
	mustDo(t, c, "POST", "/api/key-templates", map[string]any{"name": "", "vendor": "yealink", "model": "T54W"}, 400)
	tpl := mustDo(t, c, "POST", "/api/key-templates", map[string]any{"name": "Reception", "vendor": "yealink", "model": "T54W",
		"keys":    []map[string]string{{"type": "line"}, {"type": "dnd", "label": "DND"}},
		"modules": []map[string]any{{"type": "EXP50", "keys": []map[string]string{{"type": "blf", "value": "201"}}}}}, 200)
	other := mustDo(t, c, "POST", "/api/phones", map[string]any{"mac": "000413aa0003", "vendor": "snom", "model": "D785"}, 200)
	tid := jsonID(tpl["id"])
	mustDo(t, c, "POST", "/api/key-templates/"+tid+"/apply", map[string]any{"phoneIds": []any{other["id"]}}, 400)
	out := mustDo(t, c, "POST", "/api/key-templates/"+tid+"/apply", map[string]any{"phoneIds": []any{p["id"]}}, 200)
	if out["applied"] != float64(1) {
		t.Fatalf("apply: %v", out)
	}
	got, _ := s.PBX.Phone(context.Background(), int64(p["id"].(float64)))
	if len(got.Keys) != 2 || got.Keys[1].Type != "dnd" || got.Modules[0].Keys[0].Value != "201" {
		t.Fatalf("after apply: %+v", got)
	}
	if len(mustDoList(t, c, "/api/key-templates")) != 1 {
		t.Fatal("template list")
	}
	mustDo(t, c, "DELETE", "/api/key-templates/"+tid, map[string]any{}, 200)
}
