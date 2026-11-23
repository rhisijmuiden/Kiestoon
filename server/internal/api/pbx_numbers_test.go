package api

import (
	"context"
	"testing"
	"time"
)

func TestWaitRegistration(t *testing.T) {
	seq := []string{"Unregistered", "Unregistered", "Registered"}
	i := 0
	get := func(context.Context) (string, error) {
		s := seq[min(i, len(seq)-1)]
		i++
		return s, nil
	}
	if got := waitRegistration(context.Background(), get, time.Second, time.Millisecond); got != "Registered" {
		t.Fatalf("got %q", got)
	}
	stuck := func(context.Context) (string, error) { return "Unregistered", nil }
	if got := waitRegistration(context.Background(), stuck, 20*time.Millisecond, 5*time.Millisecond); got != "pending" {
		t.Fatalf("stuck: %q", got)
	}
	rej := func(context.Context) (string, error) { return "Rejected", nil }
	if got := waitRegistration(context.Background(), rej, time.Second, time.Millisecond); got != "Rejected" {
		t.Fatalf("rejected: %q", got)
	}
}
