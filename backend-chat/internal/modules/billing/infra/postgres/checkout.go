package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CheckoutStore struct{ queries *db.Queries }

var _ application.CheckoutStore = (*CheckoutStore)(nil)

func NewCheckoutStore(q *db.Queries) *CheckoutStore { return &CheckoutStore{queries: q} }

func (s *CheckoutStore) FindPortalAccount(ctx context.Context, shopID uuid.UUID) (application.CheckoutAccount, bool, error) {
	row, err := s.queries.FindBillingPortalAccount(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.CheckoutAccount{}, false, nil
	}
	if err != nil {
		return application.CheckoutAccount{}, false, err
	}
	created, err := optionalTime(row.CreatedAt)
	if err != nil {
		return application.CheckoutAccount{}, false, err
	}
	return application.CheckoutAccount{ShopID: row.ShopID, CustomerID: row.StripeCustomerID.String, Mode: row.StripeMode, CreatedAt: created}, true, nil
}

func (s *CheckoutStore) ReadCheckoutOfferState(ctx context.Context, shopID uuid.UUID) (application.CheckoutOfferState, error) {
	row, err := s.queries.GetBillingCheckoutOfferState(ctx, shopID)
	if err != nil {
		return application.CheckoutOfferState{}, err
	}
	state := application.CheckoutOfferState{HasSubscription: row.HasSubscription}
	if row.AccountMode != "" {
		created, err := optionalTime(row.AccountCreatedAt)
		if err != nil {
			return state, err
		}
		state.Account = &application.CheckoutAccount{ShopID: shopID, Mode: row.AccountMode, CustomerID: row.CustomerID, CreatedAt: created}
	}
	if row.AttemptID != uuid.Nil {
		created, err := optionalTime(row.AttemptCreatedAt)
		if err != nil {
			return state, err
		}
		state.Attempt = &application.CheckoutAttempt{ID: row.AttemptID, ShopID: shopID, Protocol: row.ProtocolVersion, Intent: domain.CheckoutIntent(row.Intent),
			PlanKey: row.PlanKey, PriceID: row.PriceID, State: row.State, SessionID: row.SessionID, CreatedAt: created}
	}
	return state, nil
}

func (s *CheckoutStore) ReserveAccount(ctx context.Context, shopID uuid.UUID) (application.CheckoutAccount, error) {
	if err := s.queries.EnsureCheckoutAccount(ctx, shopID); err != nil {
		return application.CheckoutAccount{}, err
	}
	row, err := s.queries.LockCheckoutAccount(ctx, shopID)
	if err != nil {
		return application.CheckoutAccount{}, err
	}
	created, err := optionalTime(row.CreatedAt)
	if err != nil {
		return application.CheckoutAccount{}, err
	}
	return application.CheckoutAccount{ShopID: row.ShopID, CustomerID: row.CustomerID, Mode: row.StripeMode, CreatedAt: created}, nil
}

func (s *CheckoutStore) HasCurrentSubscription(ctx context.Context, shopID uuid.UUID) (bool, error) {
	return s.queries.HasCurrentBillingSubscription(ctx, shopID)
}

func (s *CheckoutStore) FindPendingCheckout(ctx context.Context, shopID uuid.UUID) (application.CheckoutAttempt, bool, error) {
	row, err := s.queries.FindPendingBillingCheckout(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.CheckoutAttempt{}, false, nil
	}
	if err != nil {
		return application.CheckoutAttempt{}, false, err
	}
	created, err := optionalTime(row.CreatedAt)
	if err != nil {
		return application.CheckoutAttempt{}, false, err
	}
	return application.CheckoutAttempt{ID: row.ID, ShopID: row.ShopID, Intent: domain.CheckoutIntent(row.Intent),
		PlanKey: row.PlanKey, PriceID: row.PriceID, ReturnURL: row.ReturnUrl,
		State: row.State, SessionID: row.SessionID, CreatedAt: created, Protocol: row.ProtocolVersion}, true, nil
}

func (s *CheckoutStore) InsertCheckout(ctx context.Context, a application.CheckoutAttempt) error {
	return s.queries.InsertBillingCheckout(ctx, db.InsertBillingCheckoutParams{ID: a.ID, ShopID: a.ShopID, Intent: string(a.Intent),
		PlanKey: a.PlanKey, CreatedAt: pgtype.Timestamptz{Time: a.CreatedAt, Valid: true}, StripePriceID: checkoutText(a.PriceID), ReturnUrl: checkoutText(a.ReturnURL)})
}

func (s *CheckoutStore) LinkCheckoutCustomer(ctx context.Context, shopID uuid.UUID, customerID string) (bool, error) {
	count, err := s.queries.LinkBillingCheckoutCustomer(ctx, db.LinkBillingCheckoutCustomerParams{ShopID: shopID, CustomerID: checkoutText(customerID)})
	return count == 1, err
}

func (s *CheckoutStore) RecordCheckoutSession(ctx context.Context, a application.CheckoutAttempt, session application.CheckoutSession, state string) (bool, error) {
	count, err := s.queries.RecordBillingCheckoutSession(ctx, db.RecordBillingCheckoutSessionParams{AttemptID: a.ID, ShopID: a.ShopID,
		CustomerID: checkoutText(session.CustomerID), SessionID: checkoutText(session.ID), CheckoutUrl: checkoutText(session.URL),
		ExpiresAt: pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true}, NextState: state})
	return count == 1, err
}

func checkoutText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }

type CheckoutTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.CheckoutTx
}

func NewCheckoutTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.CheckoutTx) *CheckoutTransactor {
	return &CheckoutTransactor{pool: pool, bind: bind}
}

func (t *CheckoutTransactor) WithinCheckoutTx(ctx context.Context, fn func(context.Context, application.CheckoutTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
