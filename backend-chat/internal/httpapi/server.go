package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	accessmiddleware "github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type HandlerConfig struct {
	CheckDB        func(context.Context) error
	Catalog        *cataloghttp.Handler
	Authenticate   *identityapp.Authenticate
	AuthorizeShop  *identityapp.AuthorizeShopAction
	ListMyShops    *identityapp.ListMyShops
	RequestTimeout time.Duration
	DBTimeout      time.Duration
	Logger         *slog.Logger
	FrontendOrigin string
}

func NewHandler(cfg HandlerConfig) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	if cfg.DBTimeout <= 0 {
		cfg.DBTimeout = 3 * time.Second
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(accessmiddleware.CORS(cfg.FrontendOrigin))
	router.Use(accessmiddleware.RequestTimeout(cfg.RequestTimeout))
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeStatus(w, http.StatusOK, "ok")
	})
	router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.DBTimeout)
		defer cancel()
		if cfg.CheckDB == nil || cfg.CheckDB(ctx) != nil {
			cfg.Logger.WarnContext(ctx, "readiness check failed", "dependency", "postgres")
			writeStatus(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeStatus(w, http.StatusOK, "ready")
	})
	router.Options("/api/v1/admin/me", accessmiddleware.Preflight(http.MethodGet))
	router.Options("/api/v1/admin/shops", accessmiddleware.Preflight(http.MethodGet))
	authenticate := accessmiddleware.Authentication(cfg.Authenticate, cfg.Logger)
	router.With(authenticate).Get("/api/v1/admin/me", identityhttp.Me)
	router.With(authenticate).Get("/api/v1/admin/shops", identityhttp.ListShops(cfg.ListMyShops, cfg.DBTimeout, cfg.Logger))
	if cfg.Catalog != nil {
		cataloghttp.RegisterPublicRoutes(router, cfg.Catalog)
		router.Options("/api/v1/public/shops/{slug}/services", accessmiddleware.Preflight(http.MethodGet))
		router.Options("/api/v1/admin/shops/{slug}/services", accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, accessmiddleware.ShopAccess(cfg.AuthorizeShop, cfg.Logger)).
			Get("/api/v1/admin/shops/{slug}/services", cfg.Catalog.ListShopServices)
		router.With(authenticate, accessmiddleware.ShopAccess(cfg.AuthorizeShop, cfg.Logger)).
			Post("/api/v1/admin/shops/{slug}/services", cfg.Catalog.CreateService)
	}
	return router
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, "{\"status\":%q}\n", status)
}

// Serve drains in-flight requests before its caller closes shared resources.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, shutdownTimeout time.Duration, logger *slog.Logger) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		logger.Info("HTTP shutdown started")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-done
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
