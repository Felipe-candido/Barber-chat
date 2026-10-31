package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Reader struct {
	queries *db.Queries
}

var _ application.BillingStateReader = (*Reader)(nil)

// Queries may wrap the shared pool or a transaction; never open a private pool.
func NewReader(queries *db.Queries) *Reader { return &Reader{queries: queries} }

func (r *Reader) FindShopBillingState(ctx context.Context, shopID uuid.UUID) (application.ShopBillingState, bool, error) {
	row, err := r.queries.GetShopBillingState(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ShopBillingState{}, false, nil
	}
	if err != nil {
		return application.ShopBillingState{}, false, err
	}
	state := application.ShopBillingState{ShopID: row.ShopID, Active: row.Active, PlanKey: row.PlanKey}
	// The query coalesces the absent LEFT JOIN to nil UUID, not SQL NULL into
	// non-nullable Go fields. An existing subscription always has a tenant.
	if row.SubscriptionShopID == uuid.Nil {
		return state, true, nil
	}
	paidFrom, err := optionalTime(row.PaidFrom)
	if err != nil {
		return application.ShopBillingState{}, false, err
	}
	paidUntil, err := optionalTime(row.PaidUntil)
	if err != nil {
		return application.ShopBillingState{}, false, err
	}
	graceUntil, err := optionalTime(row.GraceUntil)
	if err != nil {
		return application.ShopBillingState{}, false, err
	}
	subscription := domain.Subscription{
		ShopID: row.SubscriptionShopID, Status: domain.SubscriptionStatus(row.Status),
		PaidFrom: paidFrom, PaidUntil: paidUntil, GraceUntil: graceUntil,
		CancelAtPeriodEnd: row.CancelAtPeriodEnd,
	}
	if len(row.ConfirmedPeriods) > 0 && string(row.ConfirmedPeriods) != "null" {
		if err := json.Unmarshal(row.ConfirmedPeriods, &subscription.PaidPeriods); err != nil {
			return application.ShopBillingState{}, false, err
		}
		if subscription.PaidPeriods == nil {
			subscription.PaidPeriods = []domain.Coverage{}
		}
	}
	if err := subscription.Validate(); err != nil {
		return application.ShopBillingState{}, false, err
	}
	state.Subscription = &subscription
	return state, true, nil
}

func optionalTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid {
		return time.Time{}, nil
	}
	if value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return time.Time{}, domain.ErrInvalidSubscription
	}
	return value.Time.UTC(), nil
}
