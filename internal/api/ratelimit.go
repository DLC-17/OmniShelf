package api

import (
	"sync"
	"time"
)

// Default budget for the auth endpoints: 10 failed attempts per source IP in
// a 15-minute window. Successful logins reset the counter, so legitimate
// users only ever hit this by repeatedly failing.
const (
	authFailureLimit  = 10
	authFailureWindow = 15 * time.Minute
)

// failureLimiter is a fixed-window, per-key counter of failed attempts used
// to slow credential and invite-code brute force. Only failures consume
// budget; a success clears the key. State is in-memory (single-process app),
// so a restart clears it — acceptable, since the window is short.
type failureLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newFailureLimiter(max int, window time.Duration) *failureLimiter {
	return &failureLimiter{max: max, window: window, hits: make(map[string][]time.Time)}
}

// blocked reports whether key has exhausted its failure budget within the
// current window, pruning expired entries as a side effect.
func (l *failureLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) >= l.max
}

// fail records one failed attempt for key.
func (l *failureLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.prune(key), time.Now())
}

// reset clears key after a successful attempt.
func (l *failureLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// prune drops entries older than the window and returns the remainder.
// Callers must hold l.mu.
func (l *failureLimiter) prune(key string) []time.Time {
	cutoff := time.Now().Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = kept
	return kept
}
