package http

import "github.com/go-chi/chi/v5"

func RegisterRoutes(router chi.Router, handler *Handler) {
	router.Post("/api/v1/admin/services", handler.CreateService)
	router.Get("/api/v1/public/shops/{slug}/services", handler.ListServices)
}
