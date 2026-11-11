package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
)

type limitBucket struct {
	count int
	until time.Time
}

// Bounded and process-local: shared by routes in this API instance, never keyed
// by a caller-supplied tenant, URL, email or forwarded header without validation.
type AbuseLimits struct {
	mu      sync.Mutex
	buckets map[string]limitBucket
	sweep   time.Time
}

func NewAbuseLimits() *AbuseLimits {
	return &AbuseLimits{buckets: make(map[string]limitBucket), sweep: time.Now().Add(time.Minute)}
}
func (l *AbuseLimits) allow(key string, limit int) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if !now.Before(l.sweep) {
		for k, b := range l.buckets {
			if !now.Before(b.until) {
				delete(l.buckets, k)
			}
		}
		l.sweep = now.Add(time.Minute)
	}
	b, found := l.buckets[key]
	if !found || !now.Before(b.until) {
		if !found && len(l.buckets) >= 4096 {
			return false, 60
		}
		b = limitBucket{until: now.Add(time.Minute)}
	}
	if b.count >= limit {
		return false, int(time.Until(b.until).Seconds()) + 1
	}
	b.count++
	l.buckets[key] = b
	return true, 0
}
func (l *AbuseLimits) Guard(operation string, ipLimit, actorLimit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retry := l.allow(operation+":ip:"+ClientAddress(r), ipLimit)
			if allowed && actorLimit > 0 {
				if actor, ok := requestctx.StaffIdentityFromContext(r.Context()); ok {
					allowed, retry = l.allow(operation+":actor:"+actor.UserID.String(), actorLimit)
				}
			}
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				response.Error(w, 429, "rate_limited", "too many requests; wait before trying again")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
