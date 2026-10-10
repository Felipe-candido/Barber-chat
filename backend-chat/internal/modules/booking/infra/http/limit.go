package http

import (
	"net"
	"sync"
	"time"
)

type requestCount struct{ reads, writes int }
type requestLimiter struct {
	mu      sync.Mutex
	window  time.Time
	clients map[string]requestCount
	total   int
}

func newRequestLimiter() *requestLimiter {
	return &requestLimiter{window: time.Now(), clients: make(map[string]requestCount)}
}

// A bounded, single-instance safety net for the initial API. It trusts only the
// TCP peer, never client-supplied X-Forwarded-For. Deployment behind a proxy needs
// edge rate limits and an explicit trusted-proxy policy; this is not a global quota.
func (l *requestLimiter) allow(remote string, write bool) bool {
	address, _, err := net.SplitHostPort(remote)
	if err != nil {
		address = remote
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.window) >= time.Minute {
		l.window = time.Now()
		l.clients = make(map[string]requestCount)
		l.total = 0
	}
	if l.total >= 1000 {
		return false
	}
	counts, exists := l.clients[address]
	if !exists && len(l.clients) >= 1000 {
		return false
	}
	if write {
		if counts.writes >= 20 {
			return false
		}
		counts.writes++
	} else {
		if counts.reads >= 120 {
			return false
		}
		counts.reads++
	}
	l.clients[address] = counts
	l.total++
	return true
}
