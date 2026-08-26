package infrastructure

import (
	"sync"
	"time"
)

// window is one client's fixed window: the instant it opened and how many
// permits were consumed. The window anchors at the client's first request,
// exactly like the .NET FixedWindowRateLimiter — no queueing (QueueLimit 0
// parity): the request over the limit is rejected outright.
type window struct {
	start time.Time
	count int
}

// RateLimiter is the fixed-window per-client limiter behind the auth
// endpoints' 429s (ADR 0006): a map of windows under one mutex, lazy reset on
// expiry bounding the map. In-process by design, matching the .NET kit —
// multiple replicas would need a shared store.
type RateLimiter struct {
	permitLimit int
	windowSize  time.Duration

	mu      sync.Mutex
	windows map[string]*window
}

// NewRateLimiter builds the limiter; permitLimit and windowSize come from
// RateLimiting:Auth (defaults 10 permits / 30 seconds).
func NewRateLimiter(permitLimit int, windowSize time.Duration) *RateLimiter {
	return &RateLimiter{
		permitLimit: permitLimit,
		windowSize:  windowSize,
		windows:     make(map[string]*window),
	}
}

// Allow consumes one permit for the client key when the current window has
// budget left, opening or resetting the window as needed, and reports whether
// the request fits.
func (l *RateLimiter) Allow(clientKey string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	current, ok := l.windows[clientKey]
	if !ok || now.Sub(current.start) >= l.windowSize {
		l.windows[clientKey] = &window{start: now, count: 1}
		return true
	}

	if current.count >= l.permitLimit {
		return false
	}
	current.count++
	return true
}
