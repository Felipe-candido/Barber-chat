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
	bookingapp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	customersinfra "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	customerspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"

	catalogapp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
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
	shopResolver := shopspostgres.NewResolver(queries)

	identityRepository := identitypostgres.NewRepository(queries)
	authenticate := identityapp.NewAuthenticate(verifier, identityRepository)
	authorizeShop := identityapp.NewAuthorizeShopAction(identityRepository, shopResolver)
	listMyShops := identityapp.NewListMyShops(identityRepository, identityRepository)
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

	bookingTransactions := bookingpostgres.NewTransactor(pool, func(q *db.Queries) bookingapp.BookingTx {
		repository := bookingpostgres.NewRepository(q)
		return bookingapp.BookingTx{
			Shops: shopspostgres.NewResolver(q), Catalog: catalogpostgres.NewRepository(q),
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
	handler := httpapi.NewHandler(httpapi.HandlerConfig{
		CheckDB:        pool.Ping,
		Catalog:        catalogHandler,
		Professionals:  professionalHandler,
		Booking:        bookingHandler,
		Agenda:         agendaHandler,
		Accounts:       accountHandler,
		Authenticate:   authenticate,
		AuthorizeShop:  authorizeShop,
		ListMyShops:    listMyShops,
		RequestTimeout: cfg.HTTPTimeout,
		DBTimeout:      cfg.DBTimeout,
		Logger:         logger,
		FrontendOrigin: cfg.FrontendOrigin,
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
	return httpapi.Serve(
		ctx,
		listener,
		handler,
		cfg.HTTPTimeout,
		cfg.ShutdownTimeout,
		logger,
	)
}
