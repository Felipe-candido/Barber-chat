package postgres

import (
	"context"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
)

type SalesCalendar struct{ q *db.Queries }

func NewSalesCalendar(q *db.Queries) *SalesCalendar { return &SalesCalendar{q} }

var _ application.SalesCalendar = (*SalesCalendar)(nil)

func (s *SalesCalendar) ShopTimezone(ctx context.Context, shopID uuid.UUID) (string, error) {
	return s.q.ReadSalesTimezone(ctx, shopID)
}
