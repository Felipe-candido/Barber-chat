// reconcile-stripe-event replays an existing TEST event through the normal
// financial reconciliation, authenticating its origin by Stripe API retrieval.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("usage: go run ./cmd/reconcile-stripe-event evt_...")
	}
	cfg, err := config.Load()
	if err != nil {
		return errors.New("configuration could not be validated; check the backend .env")
	}
	client, err := stripe.NewClient(cfg.StripeSecretKey, nil)
	if err != nil {
		return errors.New("a server-side Stripe TEST key is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	event, err := client.ReadWebhookEvent(ctx, os.Args[1])
	if err != nil {
		return errors.New("event retrieval or TEST envelope validation failed; no reconciliation performed")
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("database connection could not be initialized")
	}
	defer pool.Close()
	if err := application.NewReceiveWebhook(postgres.NewWebhookStore(pool), client).Execute(ctx, event); err != nil {
		return errors.New("reconciliation not confirmed; the same event can be replayed after resolving dependencies")
	}
	var state string
	var shopID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT state,shop_id FROM billing_webhook_events WHERE stripe_event_id=$1", event.ID).Scan(&state, &shopID); err != nil {
		return errors.New("event handled but receipt state could not be read; replaying the same event is safe")
	}
	if state == "ignored" {
		fmt.Println("Stripe TEST event ignored: no eligible local billing association.")
		return nil
	}
	if state != "processed" || !shopID.Valid {
		return errors.New("event not confirmed as processed for a local shop")
	}
	result, err := application.NewGetShopEntitlements(postgres.NewReader(db.New(pool)), application.ClockFunc(time.Now)).Execute(ctx, application.GetShopEntitlementsInput{ShopID: shopID.Bytes})
	if err != nil {
		return errors.New("event processed but current access could not be read; no new charge was made")
	}
	fmt.Printf("Stripe TEST event reconciled. operation_allowed=%t. No charge, publication or booking was created.\n", result.Entitlement.Allowed)
	return nil
}
