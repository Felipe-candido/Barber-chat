package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrActivityAccessDenied = errors.New("activity report access denied")

// Projection of current administrative access, not a role accepted from JSON.
type ActivityAudience struct {
	ActorID, ShopID, ProfessionalID uuid.UUID
	CanReadAll                      bool
}

func (a ActivityAudience) ProfessionalFor(actorID, shopID, requested uuid.UUID) (uuid.UUID, error) {
	if actorID == uuid.Nil || shopID == uuid.Nil || a.ActorID != actorID || a.ShopID != shopID {
		return uuid.Nil, ErrActivityAccessDenied
	}
	if a.CanReadAll {
		if a.ProfessionalID != uuid.Nil {
			return uuid.Nil, ErrInconsistentData
		}
		return requested, nil
	}
	if a.ProfessionalID == uuid.Nil || (requested != uuid.Nil && requested != a.ProfessionalID) {
		return uuid.Nil, ErrActivityAccessDenied
	}
	return a.ProfessionalID, nil
}

type ActivityAccess interface {
	RequireActivityAccess(context.Context, uuid.UUID, uuid.UUID) (ActivityAudience, error)
}

type ActivityOverviewReader interface {
	// Aggregate services and products independently before combining. Do not
	// multiply appointment values by joining directly to multiple sale items.
	// Product filters sum matching ITEMS and count DISTINCT matching sale IDs,
	// never the whole sale total. Cancelled sales contribute no valid value.
	// Counts/totals/groups share a snapshot. Group counts are not always additive
	// (one sale can contain several products). Preserve currency buckets.
	ReadActivityOverview(context.Context, ActivityQuery) (ActivityOverview, error)
}

type ActivityHistoryReader interface {
	// Apply tenant, authorized professional, period and branch filters BEFORE
	// keyset/limit. Order DESC by (occurred_at, source, id, item_id), using a
	// non-null internal item sort key for appointments. Return no public codes.
	ListActivityHistory(context.Context, ActivityHistoryQuery) ([]ActivityHistoryRow, error)
}

type ActivityReadTx struct {
	ObservedAt time.Time // Beginning of the database read transaction, not the browser clock.
	Access     ActivityAccess
	Shops      ReportShopReader
	Overview   ActivityOverviewReader
	History    ActivityHistoryReader
}

type ActivityReadTransactor interface {
	// A bounded coherent read snapshot for access, filters, totals and groups.
	// Reuse the bootstrap pool in a short read-only repeatable-read transaction.
	// Each SQL branch explicitly filters ShopID and the authorized professional.
	// No writes, providers or long external calls in this transaction.
	WithinActivityRead(context.Context, func(context.Context, ActivityReadTx) error) error
}
