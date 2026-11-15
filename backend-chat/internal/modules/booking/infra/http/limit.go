package http

import (
	"net"
	"sync"
	"time"
)

// Per-minute budgets. One client or one shop link can be throttled without
// taking every other shop's public booking down with it. The instance-wide
// totals only bound memory and worst-case load.
const (
	clientReadLimit  = 120
	clientWriteLimit = 20
	shopRequestLimit = 600
	trackedClients   = 10000
	trackedShops     = 2000
	instanceLimit    = 20000
)

type requestCount struct{ reads, writes int }
type requestLimiter struct {
	mu      sync.Mutex
	window  time.Time
	clients map[string]requestCount
	shops   map[string]int
	total   int
}

func newRequestLimiter() *requestLimiter {
	return &requestLimiter{window: time.Now(), clients: make(map[string]requestCount), shops: make(map[string]int)}
}

// A bounded, single-instance safety net. The address comes from the trusted
// proxy policy, never from a raw client-supplied X-Forwarded-For. The shop key
// is the unvalidated URL slug: it only selects a counter, and unknown slugs
// beyond the tracked set fall back to the per-client and instance budgets.
func (l *requestLimiter) allow(remote, shop string, write bool) bool {
	address, _, err := net.SplitHostPort(remote)
	if err != nil {
		address = remote
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.window) >= time.Minute {
		l.window = time.Now()
		l.clients = make(map[string]requestCount)
		l.shops = make(map[string]int)
		l.total = 0
	}
	if l.total >= instanceLimit {
		return false
	}
	counts, exists := l.clients[address]
	if !exists && len(l.clients) >= trackedClients {
		return false
	}
	if write && counts.writes >= clientWriteLimit || !write && counts.reads >= clientReadLimit {
		return false
	}
	shopCount, tracked := l.shops[shop]
	trackShop := shop != "" && len(shop) <= 100 && (tracked || len(l.shops) < trackedShops)
	if trackShop && shopCount >= shopRequestLimit {
		return false
	}
	if write {
		counts.writes++
	} else {
		counts.reads++
	}
	l.clients[address] = counts
	if trackShop {
		l.shops[shop] = shopCount + 1
	}
	l.total++
	return true
}
