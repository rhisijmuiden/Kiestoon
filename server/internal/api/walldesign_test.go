package api

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestWallScreenOwnAddress(t *testing.T) {
	c, _ := adminClient(t)
	jar, _ := cookiejar.New(nil)
	anon := &client{t: t, url: c.url, hc: &http.Client{Jar: jar}}

	// A new screen gets an address made from its name.
	sc := mustDo(t, c, "POST", "/api/wallboard/screens", map[string]any{"name": "Receptie TV"}, 200)
	id := sc["id"]
	if sc["slug"] != "receptie-tv" || sc["access"] != "link" {
		t.Fatalf("new screen: %v", sc)
	}
	// With access "link" only the secret link works.
	if code, _ := anon.do("GET", "/api/wall/receptie-tv", nil); code != 404 {
		t.Fatalf("slug with link access: %d", code)
	}

	// Open access: the own address works; bad addresses and duplicates are refused.
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "Bad Slug!", "access": "open"}, 400)
	sc = mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "open"}, 200)
	if code, d := anon.do("GET", "/api/wall/receptie", nil); code != 200 || d["screen"].(map[string]any)["name"] != "Receptie TV" {
		t.Fatalf("open slug: %d %v", code, d)
	}
	other := mustDo(t, c, "POST", "/api/wallboard/screens", map[string]any{"name": "Receptie"}, 200)
	if other["slug"] != "receptie-2" {
		t.Fatalf("free slug: %v", other["slug"])
	}
	code, e := c.do("PUT", fmt.Sprintf("/api/wallboard/screens/%v", other["id"]), map[string]any{"name": "Receptie", "slug": "receptie"})
	if code != 400 || e["field"] != "slug" {
		t.Fatalf("duplicate slug: %d %v", code, e)
	}

	// Networks: the test client comes from 127.0.0.1.
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "open", "networks": []string{"10.0.0.0/8"}}, 200)
	if code, _ := anon.do("GET", "/api/wall/receptie", nil); code != 403 {
		t.Fatalf("network not allowed: %d", code)
	}
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "open", "networks": []string{"127.0.0.1"}}, 200)
	if code, _ := anon.do("GET", "/api/wall/receptie", nil); code != 200 {
		t.Fatalf("network allowed: %d", code)
	}

	// Pin: needs a pin, the tv swaps the pin for the key and sends that along.
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "pin"}, 400)
	sc = mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "pin", "pin": "4321"}, 200)
	if sc["hasPin"] != true || sc["pin"] != nil {
		t.Fatalf("pin stored: %v", sc)
	}
	if code, d := anon.do("GET", "/api/wall/receptie", nil); code != 401 || d["error"] != "pin_required" {
		t.Fatalf("no pin: %d %v", code, d)
	}
	if code, _ := anon.do("POST", "/api/wall/receptie/pin", map[string]string{"pin": "0000"}); code != 401 {
		t.Fatalf("wrong pin: %d", code)
	}
	_, k := anon.do("POST", "/api/wall/receptie/pin", map[string]string{"pin": "4321"})
	if k["key"] != sc["key"] {
		t.Fatalf("pin key: %v", k)
	}
	anon.hdr = map[string]string{"X-Wall-Key": k["key"].(string)}
	if code, _ := anon.do("GET", "/api/wall/receptie", nil); code != 200 {
		t.Fatalf("with key: %d", code)
	}
	// Saving again without a pin keeps the pin.
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "pin"}, 200)
	anon.hdr = nil

	// Design: a draft is not on the tv until it is published.
	design := map[string]any{"format": "landscape", "pages": []any{map[string]any{"id": "p1", "name": "Overzicht", "seconds": 20,
		"tiles": []any{map[string]any{"id": "t1", "type": "kpi", "x": 11, "y": 0, "w": 3, "h": 2, "metric": "waiting"}}}},
		"style": map[string]any{"theme": "auto", "accent": "#22d3ee"}}
	bad := map[string]any{"pages": []any{map[string]any{"id": "p1", "tiles": []any{map[string]any{"id": "t1", "type": "nope"}}}}}
	mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "open", "design": bad}, 400)
	sc = mustDo(t, c, "PUT", fmt.Sprintf("/api/wallboard/screens/%v", id), map[string]any{"name": "Receptie TV", "slug": "receptie", "access": "open", "design": design}, 200)
	tile := sc["design"].(map[string]any)["pages"].([]any)[0].(map[string]any)["tiles"].([]any)[0].(map[string]any)
	if tile["x"].(float64) != 9 || sc["design"].(map[string]any)["style"].(map[string]any)["accent"] != "#22D3EE" {
		t.Fatalf("design normalized: %v", sc["design"])
	}
	_, d := anon.do("GET", "/api/wall/receptie", nil)
	if d["screen"].(map[string]any)["design"] != nil {
		t.Fatalf("draft on tv: %v", d["screen"])
	}
	mustDo(t, c, "POST", fmt.Sprintf("/api/wallboard/screens/%v/publish", id), map[string]any{}, 200)
	_, d = anon.do("GET", "/api/wall/receptie", nil)
	if d["screen"].(map[string]any)["design"] == nil {
		t.Fatalf("published design missing: %v", d["screen"])
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"Receptie TV": "receptie-tv", "Callcenter – vloer 2!": "callcenter-vloer-2", "Café Één": "cafe-een", "!": "wallboard"} {
		if got := pbx.Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
