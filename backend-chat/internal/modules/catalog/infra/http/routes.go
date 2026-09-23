package http

import "github.com/go-chi/chi/v5"

func RegisterRoutes(router chi.Router, handler *Handler) {
	router.With(handler.localCORS).Post("/api/v1/admin/services", handler.CreateService)
	router.With(handler.localCORS).Get("/api/v1/public/shops/{slug}/services", handler.ListServices)
	router.With(handler.localCORS).Options("/api/v1/admin/services", handler.preflight)
	router.With(handler.localCORS).Options("/api/v1/public/shops/{slug}/services", handler.preflight)
}
