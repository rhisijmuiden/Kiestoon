package auth

import (
	"sync"
	"time"
)

// Limiter blocks a key (an IP address) after too many failed attempts within a window.
type Limiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	now     func() time.Time
	entries map[string][]time.Time
}

// NewLimiter allows max failures per key within window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, entries: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	kept := l.entries[key][:0]
	for _, t := range l.entries[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.entries, key)
		return nil
	}
	l.entries[key] = kept
	return kept
}

// Blocked reports whether the key has used up its attempts.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) >= l.max
}

// Fail records a failed attempt.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[key] = append(l.prune(key), l.now())
}

// Reset clears the key after a successful attempt.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
