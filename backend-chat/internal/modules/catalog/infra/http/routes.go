package http

import "github.com/go-chi/chi/v5"

// RegisterPublicRoutes leaves customer-facing catalog reads unauthenticated.
// Administrative routes are registered by the server with its access middleware.
func RegisterPublicRoutes(router chi.Router, handler *Handler) {
	router.Get("/api/v1/public/shops/{slug}/services", handler.ListServices)
}
