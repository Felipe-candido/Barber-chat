package http

import (
	"net/http"
	"strings"
)

// localCORS opts one local browser origin into catalog access. It is not authentication.
// Requests go directly to the loopback API; no proxy strips security headers.
func (h *Handler) localCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if h.devFrontendOrigin != "" && r.Header.Get("Origin") == h.devFrontendOrigin && localRequest(r, h.devFrontendOrigin) {
			w.Header().Set("Access-Control-Allow-Origin", h.devFrontendOrigin)
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) preflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")
	if h.devFrontendOrigin == "" || r.Header.Get("Origin") != h.devFrontendOrigin || !localRequest(r, h.devFrontendOrigin) {
		writeError(w, http.StatusForbidden, "admin_access_unavailable", "browser origin is not allowed")
		return
	}
	method := r.Header.Get("Access-Control-Request-Method")
	expected := "GET"
	if r.URL.Path == "/api/v1/admin/services" {
		expected = "POST"
	}
	if method != expected {
		writeError(w, http.StatusForbidden, "admin_access_unavailable", "method is not allowed")
		return
	}
	for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		if header = strings.TrimSpace(header); header != "" && !strings.EqualFold(header, "Content-Type") {
			writeError(w, http.StatusForbidden, "admin_access_unavailable", "header is not allowed")
			return
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", expected)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
