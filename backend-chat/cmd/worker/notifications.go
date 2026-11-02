package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	customerspg "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	notificationmq "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/rabbitmq"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/security"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runNotifications(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) error {
	if cfg.RabbitMQURL == "" {
		return errors.New("RABBITMQ_URL is required for the notification worker")
	}
	cipher, err := security.NewPayloadCipher(cfg.NotificationPayloadActiveKeyID, cfg.NotificationPayloadKeys)
	if err != nil {
		return err
	}
	provider, err := evolution.NewClient(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey, cfg.EvolutionWebhookURL, cfg.EvolutionWebhookSecret, nil)
	if err != nil {
		return err
	}
	tx := notificationspg.NewDeliveryTransactor(pool, func(q *db.Queries) app.DeliveryTx {
		return app.DeliveryTx{Store: notificationspg.NewDeliveryStore(q), Shops: shopspg.NewMessageShops(q), Permissions: customerspg.NewBookingPermissions(q), Bookings: bookingpg.NewNotificationDeliveryBookings(q), Access: identitypg.NewConnectionOwnerGuard(q)}
	})
	deliver := app.NewDeliverNotification(tx, cipher, provider)
	store := notificationspg.NewDispatchStore(pool)
	dispatch := app.NewDispatchNotifications(store, deliver)
	connections := app.NewManageConnections(notificationspg.NewConnections(pool, func(q *db.Queries) app.ConnectionOwnerAccess { return identitypg.NewConnectionOwnerGuard(q) }), provider)
	return notifications.RunNotificationWorker(ctx, notifications.WorkerOptions{BrokerURL: cfg.RabbitMQURL, DBTimeout: cfg.DBTimeout, OperationTimeout: cfg.OperationTimeout, ShutdownTimeout: cfg.ShutdownTimeout, Names: notificationmq.DefaultNames()}, dispatch, store, connections, logger)
}
