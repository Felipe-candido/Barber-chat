package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billingapp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	billingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	billingstripe "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	bookingapp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	customersinfra "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	customerspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	notificationsapp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notificationsinfra "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra"
	notificationsevolution "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	notificationshttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	notificationssecurity "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/security"
	reportingapp "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	reportinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/http"
	reportingpg "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/postgres"
	salesapp "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	saleshttp "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/http"
	salespg "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/jobs"

	catalogapp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	shopsapp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"

	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).With("service", "api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, cfg, logger)
	stop()
	if err != nil {
		logger.Error("API stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("API stopped")
}

func run(
	ctx context.Context,
	cfg config.Config,
	logger *slog.Logger,
) error {
	// Construct one shared verifier. Invalid Auth configuration prevents startup.
	verifier, err := supabase.NewVerifier(cfg.SupabaseURL, nil)
	if err != nil {
		return fmt.Errorf("configure SUPABASE_URL: %w", err)
	}
	// Create one shared PostgreSQL pool.
	pool, err := postgres.Open(
		ctx,
		cfg.DatabaseURL,
		cfg.DBTimeout,
	)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Wire generated queries and concrete adapters.
	queries := db.New(pool)

	catalogRepository := catalogpostgres.NewRepository(queries)
	productTx := catalogpostgres.NewProductTransactor(pool, func(q *db.Queries) catalogapp.ProductTx {
		store := catalogpostgres.NewProductStore(q)
		return catalogapp.ProductTx{Access: identitypostgres.NewProductGuard(q), Products: store, Stock: store, Commands: store, Reader: store}
	})
	productHandler := cataloghttp.NewProductHandler(catalogapp.NewProductCommands(productTx, time.Now), catalogapp.NewProductQueries(productTx), cfg.OperationTimeout, logger)
	salesTx := salespg.NewTransactor(pool, func(q *db.Queries) salesapp.SalesTx {
		store := salespg.NewStore(q)
		catalog := catalogpostgres.NewSalesCatalog(q)
		return salesapp.SalesTx{Access: identitypostgres.NewSalesGuard(q), Eligibility: billingpostgres.NewSalesEligibility(q), Catalog: catalog, Appointments: bookingpostgres.NewSalesAppointments(q), Stock: catalog, Sales: store, Commands: store, Cancellation: store, Returns: catalog, Reader: store, Calendar: shopspostgres.NewSalesCalendar(q)}
	})
	salesHandler := saleshttp.NewHandler(salesapp.NewRegisterSale(salesTx, time.Now), salesapp.NewCancelSale(salesTx, time.Now), salesapp.NewSaleQueries(salesTx), cfg.OperationTimeout, logger)
	reportingTx := reportingpg.NewTransactor(pool, func(q *db.Queries, at time.Time) reportingapp.ActivityReadTx {
		reader := reportingpg.NewReader(q, at)
		return reportingapp.ActivityReadTx{Access: identitypostgres.NewActivityGuard(q), Shops: shopspostgres.NewReportShops(q), Overview: reader, History: reader}
	})
	reportingHandler := reportinghttp.NewHandler(reportingapp.NewActivityQueries(reportingTx, customersinfra.ContactNormalizer{}), cfg.OperationTimeout, logger)
	shopResolver := shopspostgres.NewResolver(queries)

	identityRepository := identitypostgres.NewRepository(queries)
	authenticate := identityapp.NewAuthenticate(verifier, identityRepository)
	authorizeShop := identityapp.NewAuthorizeShopAction(identityRepository, shopResolver)
	listMyShops := identityapp.NewListMyShops(identityRepository, identityRepository)
	var registrationAccountReader identityapp.RegistrationAccountReader
	if cfg.SupabasePublishableKey != "" {
		registrationAccountReader, err = supabase.NewSessionClient(cfg.SupabaseURL, cfg.SupabasePublishableKey, nil)
		if err != nil {
			return fmt.Errorf("configure registration: %w", err)
		}
	}
	verifyRegistration := identityapp.NewVerifyRegistrationAccount(verifier, registrationAccountReader)
	creationTx := shopspostgres.NewCreationTransactor(pool, func(q *db.Queries) shopsapp.ShopCreationTx {
		return shopsapp.ShopCreationTx{Owners: identitypostgres.NewInitialOwnerStore(q), Shops: shopspostgres.NewCreationStore(q)}
	})
	registrationHandler := shopshttp.NewRegistrationHandler(shopsapp.NewCreateShop(creationTx), cfg.OperationTimeout, logger)
	settingsTx := shopspostgres.NewSettingsTransactor(pool, func(q *db.Queries) shopsapp.ShopSettingsTx {
		return shopsapp.ShopSettingsTx{Access: identitypostgres.NewSettingsOwnerGuard(q), Shops: shopspostgres.NewSettingsStore(q)}
	})
	settingsHandler := shopshttp.NewSettingsHandler(shopsapp.NewGetShopSettings(settingsTx), shopsapp.NewUpdateShopSettings(settingsTx), cfg.OperationTimeout, logger)
	publicationTx := shopspostgres.NewPublicationTransactor(pool, func(q *db.Queries) shopsapp.PublicationTx {
		return shopsapp.PublicationTx{
			Access: identitypostgres.NewSettingsOwnerGuard(q), Shops: shopspostgres.NewPublicationStore(q),
			Catalog: catalogpostgres.NewRepository(q), Schedule: bookingpostgres.NewRepository(q),
			Billing: billingpostgres.NewPublicationCoverage(billingpostgres.NewReader(q), time.Now),
		}
	})
	publicationHandler := shopshttp.NewPublicationHandler(shopsapp.NewShopPublication(publicationTx), cfg.OperationTimeout, logger)
	entitlements := billingapp.NewGetShopEntitlements(billingpostgres.NewReader(queries), billingapp.ClockFunc(time.Now))
	publicShopGate := shopshttp.PublicBookingGate(shopsapp.NewCheckPublicBooking(shopResolver, entitlements), cfg.OperationTimeout, logger)
	var checkoutProvider billingapp.CheckoutProvider
	var portalProvider billingapp.PortalProvider
	var stripeClient *billingstripe.Client
	if cfg.StripeSecretKey != "" {
		stripeClient, err = billingstripe.NewClient(cfg.StripeSecretKey, nil)
		if err != nil {
			return fmt.Errorf("configure Stripe test checkout: %w", err)
		}
		// The browser flow must have its confirmation channel configured too.
		// Checkout alone cannot grant access after a payment.
		if cfg.StripeWebhookSecret != "" {
			checkoutProvider = stripeClient
			portalProvider = stripeClient
		}
	}
	var webhookVerifier billingapp.WebhookVerifier
	var receiveWebhook *billingapp.ReceiveWebhook
	if cfg.StripeWebhookSecret != "" {
		webhookVerifier, err = billingstripe.NewWebhookVerifier(cfg.StripeWebhookSecret, billingapp.ClockFunc(time.Now))
		if err != nil {
			return fmt.Errorf("configure Stripe test webhook: %w", err)
		}
		receiveWebhook = billingapp.NewReceiveWebhook(billingpostgres.NewWebhookStore(pool), stripeClient)
	}
	webhookHandler := billinghttp.NewWebhookHandler(webhookVerifier, receiveWebhook, cfg.OperationTimeout, logger)
	checkoutTx := billingpostgres.NewCheckoutTransactor(pool, func(q *db.Queries) billingapp.CheckoutTx {
		return billingapp.CheckoutTx{Access: identitypostgres.NewBillingOwnerGuard(q), Shops: shopspostgres.NewResolver(q), Store: billingpostgres.NewCheckoutStore(q)}
	})
	checkoutHandler := billinghttp.NewCheckoutHandler(billingapp.NewStartCheckout(checkoutTx, checkoutProvider, billingapp.ClockFunc(time.Now),
		billingapp.CheckoutPlan{Key: "standard_monthly", PriceID: cfg.StripeMonthlyPriceID, FrontendOrigin: cfg.FrontendOrigin}), cfg.OperationTimeout, logger)
	var accountProvider identityapp.StaffAccountProvider
	if cfg.SupabaseServiceRoleKey != "" {
		accountProvider, err = supabase.NewAdminClient(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.FrontendOrigin, nil)
		if err != nil {
			return fmt.Errorf("configure staff invitations: %w", err)
		}
	}
	accountHandler := identityhttp.NewAccountHandler(identityapp.NewStaffAccounts(identitypostgres.NewAccountRepository(pool), accountProvider), cfg.OperationTimeout, logger)

	createService := catalogapp.NewCreateService(catalogRepository)
	listServices := catalogapp.NewListServices(catalogRepository, shopResolver)
	listAuthorizedServices := catalogapp.NewListAuthorizedServices(catalogRepository)

	catalogHandler := cataloghttp.NewHandler(createService, listServices, listAuthorizedServices, cfg.DBTimeout, logger)
	professionalTransactions := catalogpostgres.NewProfessionalTransactor(pool, func(q *db.Queries) catalogapp.ProfessionalTx {
		return catalogapp.ProfessionalTx{Professionals: catalogpostgres.NewRepository(q), Schedule: bookingpostgres.NewRepository(q)}
	})
	professionalHandler := cataloghttp.NewProfessionalHandler(catalogapp.NewCreateProfessional(professionalTransactions),
		catalogapp.NewListProfessionals(catalogRepository), cfg.OperationTimeout, logger)

	var payloadSealer notificationsapp.PayloadSealer
	if len(cfg.NotificationPayloadKeys) > 0 {
		payloadSealer, err = notificationssecurity.NewPayloadCipher(cfg.NotificationPayloadActiveKeyID, cfg.NotificationPayloadKeys)
		if err != nil {
			return fmt.Errorf("configure notification payload protection: %w", err)
		}
	}
	prepareNotifications := notificationsapp.NewPrepareBookingNotifications(payloadSealer)
	bookingTransactions := bookingpostgres.NewTransactor(pool, func(q *db.Queries) bookingapp.BookingTx {
		repository := bookingpostgres.NewRepository(q)
		return bookingapp.BookingTx{
			Notifications: notificationsinfra.NewBookingPlanner(notificationsapp.NewPlanBookingNotifications(
				cfg.NotificationPlanningEnabled, prepareNotifications, notificationspg.NewPlanningStore(q),
				customerspostgres.NewBookingPermissions(q), shopspostgres.NewMessageShops(q))),
			StaffEligibility: billingpostgres.NewBookingEligibility(q, billingapp.ClockFunc(time.Now)),
			PublicAccess:     shopsapp.NewCheckPublicBooking(shopspostgres.NewResolver(q), billingapp.NewGetShopEntitlements(billingpostgres.NewReader(q), billingapp.ClockFunc(time.Now))),
			Shops:            shopspostgres.NewResolver(q), Catalog: catalogpostgres.NewRepository(q),
			Schedule: repository, Customers: customerspostgres.NewBookingContacts(q),
			Appointments: repository, Idempotency: repository,
			Staff: identitypostgres.NewRepository(q),
		}
	})
	clock := bookingapp.ClockFunc(time.Now)
	codes := security.Codes{}
	bookingHandler := bookinghttp.NewHandler(
		bookingapp.NewCreateAppointment(bookingTransactions, customersinfra.ContactNormalizer{}, codes, clock),
		bookingapp.NewListAvailability(bookingTransactions, clock),
		bookingapp.NewListProfessionals(bookingTransactions, clock),
		bookingapp.NewManageAppointment(bookingTransactions, codes, clock), cfg.OperationTimeout, logger,
	)

	// Register HTTP adapters and dependency probes.
	agendaHandler := bookinghttp.NewAgendaHandler(bookingapp.NewListAgenda(shopResolver, bookingpostgres.NewRepository(queries), clock), cfg.OperationTimeout, logger)
	lifecycleTx := bookingpostgres.NewLifecycleTransactor(pool, func(q *db.Queries) bookingapp.AppointmentLifecycleTx {
		return bookingapp.AppointmentLifecycleTx{Access: identitypostgres.NewRepository(q), Store: bookingpostgres.NewLifecycleStore(q)}
	})
	lifecycleHandler := bookinghttp.NewLifecycleHandler(bookingapp.NewManageAppointmentStatus(lifecycleTx), cfg.OperationTimeout, logger)
	var connectionProvider notificationsapp.ConnectionProvider
	if cfg.EvolutionBaseURL != "" {
		connectionProvider, err = notificationsevolution.NewClient(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey, cfg.EvolutionWebhookURL, cfg.EvolutionWebhookSecret, nil)
		if err != nil {
			return fmt.Errorf("configure Evolution client: %w", err)
		}
	}
	connectionRepository := notificationspg.NewConnections(pool, func(q *db.Queries) notificationsapp.ConnectionOwnerAccess {
		return identitypostgres.NewConnectionOwnerGuard(q)
	})
	manageConnections := notificationsapp.NewManageConnections(connectionRepository, connectionProvider)
	connectionHandler := notificationshttp.NewConnectionHandler(manageConnections, cfg.EvolutionWebhookSecret, cfg.OperationTimeout)
	handler := httpapi.NewHandler(httpapi.HandlerConfig{
		WhatsApp: connectionHandler, EvolutionWebhookEnabled: connectionProvider != nil,
		CheckDB:              pool.Ping,
		Catalog:              catalogHandler,
		Professionals:        professionalHandler,
		Booking:              bookingHandler,
		Agenda:               agendaHandler,
		AppointmentLifecycle: lifecycleHandler,
		Accounts:             accountHandler,
		Authenticate:         authenticate,
		AuthorizeShop:        authorizeShop,
		ListMyShops:          listMyShops,
		Registration:         registrationHandler,
		ShopSettings:         settingsHandler,
		ShopPublication:      publicationHandler,
		Products:             productHandler,
		Sales:                salesHandler,
		Reporting:            reportingHandler,
		Checkout:             checkoutHandler,
		Portal:               billinghttp.NewPortalHandler(billingapp.NewOpenPortal(checkoutTx, portalProvider, billingapp.ClockFunc(time.Now), cfg.StripePortalConfigurationID, cfg.FrontendOrigin), cfg.OperationTimeout, logger),
		StripeWebhook:        webhookHandler,
		VerifyRegistration:   verifyRegistration,
		PublicShopGate:       publicShopGate,
		RequestTimeout:       cfg.HTTPTimeout,
		DBTimeout:            cfg.DBTimeout,
		Logger:               logger,
		FrontendOrigin:       cfg.FrontendOrigin,
	})

	// Open the HTTP listener.
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info(
		"HTTP listener opened",
		"address",
		listener.Addr().String(),
		"operation_timeout", cfg.OperationTimeout.String(),
		"request_timeout", cfg.HTTPTimeout.String(),
	)

	// Serve requests until shutdown.
	jobCtx, stopJobs := context.WithCancel(ctx)
	jobsDone := make(chan struct{})
	completion := bookingapp.NewAutoCompleteAppointments(bookingpostgres.NewLifecycleStore(queries))
	go func() {
		defer close(jobsDone)
		jobs.Run(jobCtx, 15*time.Second, cfg.OperationTimeout, completion.Execute, logger)
	}()
	defer func() { stopJobs(); <-jobsDone }()
	if connectionProvider != nil {
		monitorDone := make(chan struct{})
		go func() {
			defer close(monitorDone)
			notificationsinfra.RunConnectionMonitor(jobCtx, manageConnections, logger)
		}()
		defer func() { stopJobs(); <-monitorDone }()
	}
	return serveWithEvolutionWebhook(ctx, listener, handler, connectionHandler, cfg, logger)
}
