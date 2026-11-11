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
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	customershttp "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/http"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	notificationshttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	reportinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/http"
	saleshttp "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/http"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type HandlerConfig struct {
	AppEnvironment          string
	TrustedProxyCIDRs       string
	Contacts                *customershttp.ContactHandler
	WhatsApp                *notificationshttp.ConnectionHandler
	Notifications           *notificationshttp.MessageHandler
	EvolutionWebhookEnabled bool
	CheckDB                 func(context.Context) error
	Catalog                 *cataloghttp.Handler
	Products                *cataloghttp.ProductHandler
	Sales                   *saleshttp.Handler
	Reporting               *reportinghttp.Handler
	Professionals           *cataloghttp.ProfessionalHandler
	Booking                 *bookinghttp.Handler
	Agenda                  *bookinghttp.AgendaHandler
	AppointmentLifecycle    *bookinghttp.LifecycleHandler
	CompletionSettings      *bookinghttp.CompletionSettingsHandler
	Accounts                *identityhttp.AccountHandler
	Authenticate            *identityapp.Authenticate
	AuthorizeShop           *identityapp.AuthorizeShopAction
	ListMyShops             *identityapp.ListMyShops
	Registration            *shopshttp.RegistrationHandler
	ShopSettings            *shopshttp.SettingsHandler
	ShopPublication         *shopshttp.PublicationHandler
	Checkout                *billinghttp.CheckoutHandler
	Portal                  *billinghttp.PortalHandler
	StripeWebhook           *billinghttp.WebhookHandler
	VerifyRegistration      *identityapp.VerifyRegistrationAccount
	PublicShopGate          func(http.Handler) http.Handler
	RequestTimeout          time.Duration
	DBTimeout               time.Duration
	Logger                  *slog.Logger
	FrontendOrigin          string
}

func NewHandler(cfg HandlerConfig) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	if cfg.DBTimeout <= 0 {
		cfg.DBTimeout = 3 * time.Second
	}
	router := chi.NewRouter()
	network, networkErr := accessmiddleware.NewClientNetworkPolicy(cfg.TrustedProxyCIDRs)
	if networkErr != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.Error(w, 503, "configuration_error", "invalid network configuration")
		})
	}
	limits := accessmiddleware.NewAbuseLimits()
	router.Use(accessmiddleware.RequestLog(cfg.Logger))
	router.Use(middleware.Recoverer)
	router.Use(accessmiddleware.CORS(cfg.FrontendOrigin))
	router.Use(network.Middleware)
	router.Use(accessmiddleware.SecurityHeaders(cfg.AppEnvironment == "production"))
	router.Use(accessmiddleware.RequestTimeout(cfg.RequestTimeout))
	if cfg.Contacts != nil {
		base := "/api/v1/admin/shops/{slug}/contacts"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.Options(base+"/{contactID}", accessmiddleware.Preflight(http.MethodGet))
		auth := accessmiddleware.Authentication(cfg.Authenticate, cfg.Logger)
		shop := accessmiddleware.ShopAccess(cfg.AuthorizeShop, cfg.Logger)
		router.With(auth, shop).Method(http.MethodGet, base, cfg.Contacts)
		router.With(auth, shop).Method(http.MethodPost, base, cfg.Contacts)
		router.With(auth, shop).Method(http.MethodGet, base+"/{contactID}", cfg.Contacts)
	}
	if cfg.WhatsApp != nil && cfg.EvolutionWebhookEnabled {
		router.Post("/api/v1/webhooks/evolution", cfg.WhatsApp.Webhook)
	}
	if cfg.StripeWebhook != nil {
		router.Post("/api/v1/webhooks/stripe", cfg.StripeWebhook.Receive)
	}
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
	if cfg.Registration != nil {
		router.Options("/api/v1/onboarding/shops", accessmiddleware.Preflight(http.MethodPost))
		router.With(limits.Guard("onboarding", 30, 0), accessmiddleware.RegistrationAuthentication(cfg.VerifyRegistration, cfg.Logger)).Post("/api/v1/onboarding/shops", cfg.Registration.Create)
	}
	publicGates := []func(http.Handler) http.Handler{}
	if cfg.PublicShopGate != nil {
		publicGates = append(publicGates, cfg.PublicShopGate)
	}
	shopAccess := accessmiddleware.ShopAccess(cfg.AuthorizeShop, cfg.Logger)
	if cfg.Notifications != nil {
		base := "/api/v1/admin/shops/{slug}"
		router.Options(base+"/notifications/options", accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(base+"/notifications/options", cfg.Notifications.PublicOptions)
		router.Options(base+"/notifications/settings", accessmiddleware.Preflight(http.MethodGet, http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).MethodFunc(http.MethodGet, base+"/notifications/settings", cfg.Notifications.Settings)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).MethodFunc(http.MethodPut, base+"/notifications/settings", cfg.Notifications.Settings)
		status := base + "/appointments/{appointmentID}/notifications"
		router.Options(status, accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(status, cfg.Notifications.BookingStatus)
		router.Options(status+"/confirmation", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, limits.Guard("messages", 120, 60)).Post(status+"/confirmation", cfg.Notifications.SendConfirmation)
		router.Options(status+"/reminder", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, limits.Guard("messages", 120, 60)).Post(status+"/reminder", cfg.Notifications.SendReminder)
		public := "/api/v1/public/shops/{slug}/notification-options"
		router.Options(public, accessmiddleware.Preflight(http.MethodGet))
		router.With(publicGates...).Get(public, cfg.Notifications.PublicOptions)
	}
	if cfg.WhatsApp != nil {
		base := "/api/v1/admin/shops/{slug}/whatsapp"
		router.Options(base+"/connection", accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Get(base+"/connection", cfg.WhatsApp.Status)
		router.Options(base+"/connect", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("connection", 60, 30)).Post(base+"/connect", cfg.WhatsApp.Connect)
		router.Options(base+"/disconnect", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("connection", 60, 30)).Post(base+"/disconnect", cfg.WhatsApp.Disconnect)
	}
	if cfg.Reporting != nil {
		base := "/api/v1/admin/shops/{slug}/reports/activity"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(base, cfg.Reporting.Overview)
		router.Options(base+"/history", accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(base+"/history", cfg.Reporting.History)
	}
	if cfg.Sales != nil {
		base := "/api/v1/admin/shops/{slug}/sales"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess).Post(base, cfg.Sales.Register)
		router.With(authenticate, shopAccess).Get(base, cfg.Sales.List)
		router.Options(base+"/{saleID}", accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(base+"/{saleID}", cfg.Sales.Get)
		router.Options(base+"/{saleID}/cancel", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Post(base+"/{saleID}/cancel", cfg.Sales.Cancel)
	}
	if cfg.Products != nil {
		base := "/api/v1/admin/shops/{slug}/products"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess).Get(base, cfg.Products.List)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Post(base, cfg.Products.Create)
		router.Options(base+"/{productID}", accessmiddleware.Preflight(http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Put(base+"/{productID}", cfg.Products.Update)
		router.Options(base+"/{productID}/stock-minimum", accessmiddleware.Preflight(http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Put(base+"/{productID}/stock-minimum", cfg.Products.SetMinimum)
		router.Options(base+"/{productID}/stock-movements", accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Get(base+"/{productID}/stock-movements", cfg.Products.Movements)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Post(base+"/{productID}/stock-movements", cfg.Products.RecordStock)
	}
	router.Options("/api/v1/admin/shops/{slug}/access", accessmiddleware.Preflight(http.MethodGet))
	router.With(authenticate, shopAccess).Get("/api/v1/admin/shops/{slug}/access", identityhttp.Access)
	if cfg.CompletionSettings != nil {
		base := "/api/v1/admin/shops/{slug}/appointment-completion"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Method(http.MethodGet, base, cfg.CompletionSettings)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Method(http.MethodPut, base, cfg.CompletionSettings)
	}
	if cfg.ShopSettings != nil {
		base := "/api/v1/admin/shops/{slug}/settings"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Get(base, cfg.ShopSettings.Get)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Put(base, cfg.ShopSettings.Update)
	}
	if cfg.Checkout != nil {
		base := "/api/v1/admin/shops/{slug}/billing/checkout"
		router.Options(base, accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("billing", 120, 60)).Post(base, cfg.Checkout.Start)
		router.Options("/api/v1/admin/shops/{slug}/billing/offer", accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("billing-offer", 240, 120)).Get("/api/v1/admin/shops/{slug}/billing/offer", cfg.Checkout.Offer)
	}
	if cfg.Portal != nil {
		base := "/api/v1/admin/shops/{slug}/billing/portal"
		router.Options(base, accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("billing", 120, 60)).Post(base, cfg.Portal.Open)
	}
	if cfg.ShopPublication != nil {
		base := "/api/v1/admin/shops/{slug}"
		router.Options(base+"/commercial-status", accessmiddleware.Preflight(http.MethodGet))
		router.Options(base+"/publication", accessmiddleware.Preflight(http.MethodPut))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Get(base+"/commercial-status", cfg.ShopPublication.Get)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Put(base+"/publication", cfg.ShopPublication.Set)
	}
	if cfg.Catalog != nil {
		cataloghttp.RegisterPublicRoutes(router, cfg.Catalog, publicGates...)
		router.Options("/api/v1/public/shops/{slug}/services", accessmiddleware.Preflight(http.MethodGet))
		router.Options("/api/v1/admin/shops/{slug}/services", accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess).
			Get("/api/v1/admin/shops/{slug}/services", cfg.Catalog.ListShopServices)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).
			Post("/api/v1/admin/shops/{slug}/services", cfg.Catalog.CreateService)
	}
	if cfg.Booking != nil {
		cfg.Booking.Register(router, publicGates...)
		for _, path := range []string{"/booking/professionals", "/booking/availability"} {
			router.Options("/api/v1/admin/shops/{slug}"+path, accessmiddleware.Preflight(http.MethodGet))
		}
		router.With(authenticate, shopAccess).Get("/api/v1/admin/shops/{slug}/booking/professionals", cfg.Booking.ListProfessionalsForStaff)
		router.With(authenticate, shopAccess).Get("/api/v1/admin/shops/{slug}/booking/availability", cfg.Booking.ListAvailabilityForStaff)
		router.With(authenticate, shopAccess).Post("/api/v1/admin/shops/{slug}/appointments", cfg.Booking.CreateForStaff)
	}
	if cfg.Agenda != nil || cfg.Booking != nil {
		methods := []string{}
		if cfg.Agenda != nil {
			methods = append(methods, http.MethodGet)
		}
		if cfg.Booking != nil {
			methods = append(methods, http.MethodPost)
		}
		router.Options("/api/v1/admin/shops/{slug}/appointments", accessmiddleware.Preflight(methods...))
	}
	if cfg.Agenda != nil {
		router.With(authenticate, shopAccess).
			Get("/api/v1/admin/shops/{slug}/appointments", cfg.Agenda.List)
	}
	if cfg.AppointmentLifecycle != nil {
		base := "/api/v1/admin/shops/{slug}/appointments/{appointmentID}"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet))
		router.With(authenticate, shopAccess).Get(base, cfg.AppointmentLifecycle.Get)
		router.Options(base+"/status", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess).Post(base+"/status", cfg.AppointmentLifecycle.Change)
	}
	if cfg.Accounts != nil {
		base := "/api/v1/admin/shops/{slug}/accounts"
		router.Options(base, accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Get(base, cfg.Accounts.List)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("accounts", 120, 60)).Post(base, cfg.Accounts.Invite)
		router.Options(base+"/{accountID}/link", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Post(base+"/{accountID}/link", cfg.Accounts.Link)
		router.Options(base+"/{accountID}/status", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).Post(base+"/{accountID}/status", cfg.Accounts.SetActive)
		router.Options(base+"/{accountID}/replace", accessmiddleware.Preflight(http.MethodPost))
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly, limits.Guard("accounts", 120, 60)).Post(base+"/{accountID}/replace", cfg.Accounts.Replace)
	}
	if cfg.Professionals != nil {
		router.Options("/api/v1/admin/shops/{slug}/professionals", accessmiddleware.Preflight(http.MethodGet, http.MethodPost))
		router.With(authenticate, shopAccess).
			Get("/api/v1/admin/shops/{slug}/professionals", cfg.Professionals.List)
		router.With(authenticate, shopAccess, accessmiddleware.OwnerOnly).
			Post("/api/v1/admin/shops/{slug}/professionals", cfg.Professionals.Create)
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
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, requestTimeout, shutdownTimeout time.Duration, logger *slog.Logger) error {
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Leave time to write the timeout response after the application deadline.
		WriteTimeout: requestTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
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
