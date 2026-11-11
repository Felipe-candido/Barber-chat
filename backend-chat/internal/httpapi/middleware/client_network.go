package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
)

type clientNetworkKey struct{}
type clientNetwork struct {
	address string
	secure  bool
}
type ClientNetworkPolicy struct{ proxies []netip.Prefix }

func NewClientNetworkPolicy(raw string) (*ClientNetworkPolicy, error) {
	p := &ClientNetworkPolicy{}
	if raw == "" {
		return p, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 16 {
		return nil, errors.New("too many trusted proxies")
	}
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
			return nil, errors.New("invalid trusted proxy network")
		}
		p.proxies = append(p.proxies, prefix.Masked())
	}
	return p, nil
}
func (p *ClientNetworkPolicy) trusted(a netip.Addr) bool {
	for _, prefix := range p.proxies {
		if prefix.Contains(a) {
			return true
		}
	}
	return false
}
func peerAddress(raw string) netip.Addr {
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}
func ClientAddress(r *http.Request) string {
	if c, ok := r.Context().Value(clientNetworkKey{}).(clientNetwork); ok {
		return c.address
	}
	if a := peerAddress(r.RemoteAddr); a.IsValid() {
		return a.String()
	}
	return "unknown"
}
func RequestIsSecure(r *http.Request) bool {
	c, _ := r.Context().Value(clientNetworkKey{}).(clientNetwork)
	return r.TLS != nil || c.secure
}

// Ignore forwarded headers from untrusted TCP peers. Walk the trusted chain
// from right to left, stopping at the first untrusted address, not the leftmost
// caller-controlled entry. Never rewrite RemoteAddr or log forwarded headers.
func (p *ClientNetworkPolicy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := peerAddress(r.RemoteAddr)
		address := peer
		secure := r.TLS != nil
		if peer.IsValid() && p.trusted(peer) {
			forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
			if len(forwarded) > 4096 {
				response.Error(w, 400, "invalid_proxy_headers", "invalid forwarding metadata")
				return
			}
			if forwarded != "" {
				entries := strings.Split(forwarded, ",")
				if len(entries) > 32 {
					response.Error(w, 400, "invalid_proxy_headers", "invalid forwarding metadata")
					return
				}
				for i := len(entries) - 1; i >= 0 && p.trusted(address); i-- {
					a, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
					if err != nil || a.Zone() != "" {
						response.Error(w, 400, "invalid_proxy_headers", "invalid forwarding metadata")
						return
					}
					address = a.Unmap()
				}
			}
			proto := r.Header.Values("X-Forwarded-Proto")
			if len(proto) > 1 || (len(proto) == 1 && proto[0] != "http" && proto[0] != "https") {
				response.Error(w, 400, "invalid_proxy_headers", "invalid forwarding metadata")
				return
			}
			secure = secure || (len(proto) == 1 && proto[0] == "https")
		}
		value := "unknown"
		if address.IsValid() {
			value = address.String()
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientNetworkKey{}, clientNetwork{value, secure})))
	})
}

func SecurityHeaders(production bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			if production && r.URL.Path != "/health" && r.URL.Path != "/ready" {
				if !RequestIsSecure(r) {
					response.Error(w, 426, "https_required", "HTTPS is required")
					return
				}
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
