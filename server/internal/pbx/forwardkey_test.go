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
	keys, err = CleanKeys([]Key{{Type: "forward", Value: "ivr:3"}, {Type: "forward", Value: "oncall:12"}}, 10, 1)
	if err != nil || keys[0].Value != "ivr:3" {
		t.Fatalf("clean dest: %v %+v", err, keys)
	}
	for _, bad := range []string{"ivr:", "ivr:x", "hangup:1", "remote:2", "bogus:3"} {
		if _, err := CleanKeys([]Key{{Type: "forward", Value: bad}}, 10, 1); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if c := ForwardCode(Destination{Type: DestIVR, Target: "3"}); c != "**053" {
		t.Errorf("code %q", c)
	}
	if d, ok := ParseForwardCode("**0812"); !ok || d.Type != DestOnCall || d.Target != "12" {
		t.Errorf("parse %+v %v", d, ok)
	}
	if _, ok := ParseForwardCode("0612345678"); ok {
		t.Error("plain number parsed as a destination")
	}
	f := Features{ForwardAlways: "**053"}
	if err := f.Validate(); err != nil {
		t.Errorf("forward code rejected by features: %v", err)
	}
	phones := []Phone{
		{Keys: []Key{{Type: "forward", Value: "0612345678"}, {Type: "blf", Value: "201"}}},
		{Modules: []Module{{Type: "EXP50", Keys: []Key{{Type: "forward", Value: "+31201234567"}, {Type: "forward", Value: "0612345678"}}}}},
		{Keys: []Key{{Type: "speed", Value: "0611111111"}}},
		{Keys: []Key{{Type: "forward", Value: "queue:4"}}},
	}
	if got := ForwardTargets(phones); !slices.Equal(got, []string{"**044", "+31201234567", "0612345678"}) {
		t.Errorf("targets %v", got)
	}
	if !phones[1].HasForwardKey() || phones[2].HasForwardKey() {
		t.Error("HasForwardKey")
	}
}
