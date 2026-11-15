package pbx

import (
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
