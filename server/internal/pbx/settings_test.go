package pbx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPlanMigrationGrowAndShrink(t *testing.T) {
	ch, conf := PlanMigration([]string{"201", "550"}, 4)
	if len(conf) != 0 || !slices.Equal(ch, []Renumber{{"201", "2001"}, {"550", "5050"}}) {
		t.Fatalf("grow: %v %v", ch, conf)
	}
	ch, conf = PlanMigration([]string{"2001", "2101", "2010"}, 3)
	if !slices.Equal(conf, []string{"2101"}) {
		t.Fatalf("shrink conflicts: %v", conf)
	}
	if !slices.Equal(ch, []Renumber{{"2001", "201"}, {"2010", "210"}}) {
		t.Fatalf("shrink changes: %v", ch)
	}
	// Two numbers that would end up the same are a conflict.
	if _, conf := PlanMigration([]string{"2001", "201"}, 3); len(conf) != 1 {
		t.Fatalf("duplicate not detected: %v", conf)
	}
}

func TestPlanForRanges(t *testing.T) {
	p := PlanFor(4)
	if !p.InRange("extensions", "2000") || !p.InRange("extensions", "5999") || p.InRange("extensions", "6000") || p.InRange("extensions", "200") {
		t.Fatal("extension range wrong")
	}
	if !p.InRange("queues", "7001") {
		t.Fatal("queue range wrong")
	}
}

func TestDIDVariants(t *testing.T) {
	want := []string{"0201234567", "31201234567", "+31201234567", "0031201234567"}
	for _, in := range want {
		if got := DIDVariants(in, "31"); !slices.Equal(got, want) {
			t.Errorf("%s: %v", in, got)
		}
	}
	if got := DIDVariants("1234", "31"); !slices.Equal(got, []string{"1234"}) {
		t.Errorf("short: %v", got)
	}
	if NormalizeDID(" 020-123 45 67 ") != "0201234567" {
		t.Error("NormalizeDID")
	}
}

func TestNormalizeMAC(t *testing.T) {
	if got := NormalizeMAC("00:15:65:AB-CD.EF"); got != "001565abcdef" || !reMAC.MatchString(got) {
		t.Fatal(got)
	}
}

func TestValidateTrunkPort(t *testing.T) {
	srv := Trunk{Name: "Voys", AuthMode: "register", Host: "ha.voys.nl", Username: "123456789", Secret: "VoysSecret123", MatchIPs: VoysNetworks}
	if err := validateTrunk(&srv); err != nil || srv.Port != 0 {
		t.Fatalf("host name without port: err %v, port %d (want SRV, port 0)", err, srv.Port)
	}
	ip := Trunk{Name: "IP", AuthMode: "ip", Host: "192.0.2.20", Transport: "tls"}
	if err := validateTrunk(&ip); err != nil || ip.Port != 5061 {
		t.Fatalf("IP without port: err %v, port %d (want 5061)", err, ip.Port)
	}
	for _, p := range TrunkPresets {
		if p.ID == "voys" && (p.Host != "ha.voys.nl" || p.MatchIPs != VoysNetworks || p.Guide == "") {
			t.Fatalf("voys preset incomplete: %+v", p)
		}
	}
}

// Every provider template must have its hint (and guide, when it has one) in English and Dutch.
func TestTrunkPresetTranslations(t *testing.T) {
	for _, lang := range []string{"en", "nl"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "i18n", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Trunks struct {
				Notes  map[string]string `json:"notes"`
				Guides map[string]struct {
					Title    string   `json:"title"`
					Portal   []string `json:"portal"`
					Kiestoon []string `json:"kiestoon"`
					Terms    string   `json:"terms"`
				} `json:"guides"`
			} `json:"trunks"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, p := range TrunkPresets {
			if seen[p.ID] {
				t.Fatalf("duplicate preset id %q", p.ID)
			}
			seen[p.ID] = true
			if doc.Trunks.Notes[p.Note] == "" {
				t.Errorf("%s: note %q missing", lang, p.Note)
			}
			if p.Guide == "" {
				continue
			}
			g, ok := doc.Trunks.Guides[p.Guide]
			if !ok || g.Title == "" || len(g.Portal) == 0 || len(g.Kiestoon) == 0 || g.Terms == "" {
				t.Errorf("%s: guide %q incomplete", lang, p.Guide)
			}
		}
	}
}
