package secrets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "speech", "secrets.key")
	b := &Box{Path: path}
	s, err := b.Seal("a", "sk_secret")
	if err != nil || !strings.HasPrefix(s, prefix) || strings.Contains(s, "sk_secret") {
		t.Fatalf("seal: %q %v", s, err)
	}
	// A new box reads the same key file.
	got, err := (&Box{Path: path}).Open("a", s)
	if err != nil || got != "sk_secret" {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := b.Open("b", s); err == nil {
		t.Fatal("value opened under another name")
	}
	if got, _ := b.Open("a", "plain"); got != "plain" {
		t.Fatal("plain value changed")
	}
	if s, _ := b.Seal("a", ""); s != "" {
		t.Fatal("empty value sealed")
	}
}
