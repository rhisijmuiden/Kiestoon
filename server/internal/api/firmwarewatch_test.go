package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/firmware"
)

func TestFirmwareWatch(t *testing.T) {
	c, s := adminClient(t)
	ctx := context.Background()
	rom := []byte("NEW-T4XU-FIRMWARE")
	sum := sha256.Sum256(rom)
	downloads := 0
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		_, _ = w.Write(rom)
	}))
	defer vendor.Close()
	releases := []firmware.Release{
		{Vendor: "yealink", Models: []string{"T42U", "T43U"}, Version: "108.86.0.60", URL: vendor.URL + "/T4xU-108.86.0.60.rom", SHA256: hex.EncodeToString(sum[:]), Source: "catalog"},
		{Vendor: "yealink", Models: []string{"T54W"}, Version: "96.86.0.70", URL: vendor.URL + "/T54W-96.86.0.70.rom", SHA256: "00" + hex.EncodeToString(sum[1:]), Source: "catalog"},
	}
	old := fwReleases
	fwReleases = func(context.Context, string) ([]firmware.Release, []string) { return releases, nil }
	defer func() { fwReleases = old }()

	phones := []struct{ mac, model, ua string }{
		{"001565aa0001", "T42U", "Yealink SIP-T42U 108.86.0.45 00:15:65:aa:00:01"},
		{"001565aa0002", "T42U", "Yealink SIP-T42U 108.86.0.60 00:15:65:aa:00:02"},
		{"001565aa0003", "T43U", "Yealink SIP-T43U 108.86.0.20 00:15:65:aa:00:03"},
		{"001565aa0004", "T54W", "Yealink SIP-T54W 96.86.0.10 00:15:65:aa:00:04"},
		{"001565aa0005", "T31P", "Yealink SIP-T31P 124.86.0.40 00:15:65:aa:00:05"},
	}
	for _, p := range phones {
		ph := mustDo(t, c, "POST", "/api/phones", map[string]any{"mac": p.mac, "vendor": "yealink", "model": p.model}, 200)
		s.PBX.TouchPhone(ctx, int64(ph["id"].(float64)), "10.0.0.9", p.ua)
	}
	state := func() map[string]map[string]any {
		res := mustDo(t, c, "GET", "/api/firmware/latest?refresh=1", nil, 200)
		out := map[string]map[string]any{}
		for _, m := range res["models"].([]any) {
			mm := m.(map[string]any)
			out[mm["model"].(string)] = mm
		}
		return out
	}
	st := state()
	if st["T42U"]["state"] != "update" || st["T42U"]["behind"] != float64(1) || st["T42U"]["phones"] != float64(2) {
		t.Fatalf("T42U: %v", st["T42U"])
	}
	if st["T31P"]["state"] != "nosource" || st["T54W"]["state"] != "update" {
		t.Fatalf("states: %v / %v", st["T31P"], st["T54W"])
	}

	// One download covers both models of the release.
	mustDo(t, c, "POST", "/api/firmware/latest/install", map[string]any{"vendor": "yealink", "model": "T42U"}, 200)
	for _, m := range []string{"T42U", "T43U"} {
		f := s.Firmware.ForModel(ctx, "yealink", m)
		if f == nil || f.Version != "108.86.0.60" || f.Filename != "T4xU-108.86.0.60.rom" {
			t.Fatalf("%s not rolled out: %+v", m, f)
		}
	}
	st = state()
	if st["T42U"]["state"] != "rolling" || st["T43U"]["state"] != "rolling" {
		t.Fatalf("after install: %v / %v", st["T42U"]["state"], st["T43U"]["state"])
	}
	// A second install reuses the stored file.
	mustDo(t, c, "POST", "/api/firmware/latest/install", map[string]any{"vendor": "yealink", "model": "T43U"}, 200)
	if downloads != 1 {
		t.Fatalf("downloads: %d", downloads)
	}
	// A file that does not match the catalog's checksum is thrown away.
	res := mustDo(t, c, "POST", "/api/firmware/latest/install", map[string]any{"vendor": "yealink", "model": "T54W"}, 502)
	if res["error"] != "checksum_mismatch" || s.Firmware.FindVersion(ctx, "yealink", "96.86.0.70") != nil {
		t.Fatalf("checksum: %v", res)
	}
	mustDo(t, c, "POST", "/api/firmware/latest/install", map[string]any{"vendor": "yealink", "model": "T31P"}, 404)

	// Settings, and the nightly run in automatic mode.
	mustDo(t, c, "PUT", "/api/firmware/latest/settings", map[string]any{"mode": "sometimes"}, 400)
	mustDo(t, c, "PUT", "/api/firmware/latest/settings", map[string]any{"mode": "auto", "catalogUrl": "ftp://x"}, 400)
	mustDo(t, c, "PUT", "/api/firmware/latest/settings", map[string]any{"mode": "off"}, 200)
	if got := s.firmwareWatchOnce(ctx); got != nil {
		t.Fatalf("off: %v", got)
	}
	releases[1].SHA256 = hex.EncodeToString(sum[:])
	mustDo(t, c, "PUT", "/api/firmware/latest/settings", map[string]any{"mode": "auto"}, 200)
	if got := s.firmwareWatchOnce(ctx); len(got) != 1 || got[0] != "yealink/T54W" {
		t.Fatalf("auto: %v", got)
	}
	if s.Firmware.ForModel(ctx, "yealink", "T54W") == nil {
		t.Fatal("T54W not rolled out at night")
	}
	// Notify mode reports a version once.
	releases[0].Version = "108.86.0.70"
	mustDo(t, c, "PUT", "/api/firmware/latest/settings", map[string]any{"mode": "notify"}, 200)
	if got := s.firmwareWatchOnce(ctx); len(got) != 2 {
		t.Fatalf("notify: %v", got)
	}
	if got := s.firmwareWatchOnce(ctx); len(got) != 0 {
		t.Fatalf("notified twice: %v", got)
	}
}
