package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ActivityGuard struct{ q *db.Queries }

func NewActivityGuard(q *db.Queries) *ActivityGuard { return &ActivityGuard{q: q} }

var _ application.ActivityAccess = (*ActivityGuard)(nil)

func (g *ActivityGuard) RequireActivityAccess(ctx context.Context, actor, shop uuid.UUID) (application.ActivityAudience, error) {
	if actor == uuid.Nil || shop == uuid.Nil {
		return application.ActivityAudience{}, application.ErrActivityAccessDenied
	}
	r, err := g.q.ReadActivityReportAccess(ctx, db.ReadActivityReportAccessParams{ActorID: actor, ShopID: shop})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ActivityAudience{}, application.ErrActivityAccessDenied
	}
	if err != nil {
		return application.ActivityAudience{}, err
	}
	audience := application.ActivityAudience{ActorID: actor, ShopID: shop}
	switch {
	case r.Role == "owner" && r.ProfessionalID == uuid.Nil:
		audience.CanReadAll = true
	case r.Role == "barber" && r.ProfessionalID != uuid.Nil:
		audience.ProfessionalID = r.ProfessionalID
	default:
		return application.ActivityAudience{}, application.ErrInconsistentData
	}
	return audience, nil
}
