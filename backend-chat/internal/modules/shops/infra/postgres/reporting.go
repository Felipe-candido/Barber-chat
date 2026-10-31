package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReportShops struct{ q *db.Queries }

func NewReportShops(q *db.Queries) *ReportShops { return &ReportShops{q: q} }

var _ application.ReportShopReader = (*ReportShops)(nil)

func (r *ReportShops) FindActiveReportShop(ctx context.Context, id uuid.UUID) (application.ReportShop, bool, error) {
	row, err := r.q.FindActiveReportShop(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ReportShop{}, false, nil
	}
	if err != nil {
		return application.ReportShop{}, false, err
	}
	return application.ReportShop{ID: row.ID, Timezone: row.Timezone}, true, nil
}
