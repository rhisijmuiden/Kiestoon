package pbx

import (
	"slices"
	"testing"
)

func TestForwardKeys(t *testing.T) {
	keys, err := CleanKeys([]Key{{Type: "forward", Value: "06 1234-5678", Label: "Mobile"}}, 10, 1)
	if err != nil || keys[0].Value != "0612345678" {
		t.Fatalf("clean: %v %+v", err, keys)
	}
	for _, bad := range []string{"", "1", "06*123", "abc", "#21"} {
		if _, err := CleanKeys([]Key{{Type: "forward", Value: bad}}, 10, 1); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	phones := []Phone{
		{Keys: []Key{{Type: "forward", Value: "0612345678"}, {Type: "blf", Value: "201"}}},
		{Modules: []Module{{Type: "EXP50", Keys: []Key{{Type: "forward", Value: "+31201234567"}, {Type: "forward", Value: "0612345678"}}}}},
		{Keys: []Key{{Type: "speed", Value: "0611111111"}}},
	}
	if got := ForwardTargets(phones); !slices.Equal(got, []string{"+31201234567", "0612345678"}) {
		t.Errorf("targets %v", got)
	}
	if !phones[1].HasForwardKey() || phones[2].HasForwardKey() {
		t.Error("HasForwardKey")
	}
}
