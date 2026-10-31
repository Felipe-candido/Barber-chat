package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SalesAppointments struct{ q *db.Queries }

func NewSalesAppointments(q *db.Queries) *SalesAppointments { return &SalesAppointments{q} }

var _ sales.SaleAppointmentReader = (*SalesAppointments)(nil)

func (s *SalesAppointments) ReadSaleAppointment(ctx context.Context, shopID, id uuid.UUID) (sales.AppointmentForSale, bool, error) {
	r, err := s.q.ReadSaleAppointment(ctx, db.ReadSaleAppointmentParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.AppointmentForSale{}, false, nil
	}
	if err != nil {
		return sales.AppointmentForSale{}, false, err
	}
	return sales.AppointmentForSale{ID: r.ID, ShopID: r.ShopID, ProfessionalID: r.ProfessionalID, State: sales.SaleAppointmentState(r.Status)}, true, nil
}
func (s *SalesAppointments) LockSaleAppointment(ctx context.Context, shopID, id uuid.UUID) (sales.AppointmentForSale, bool, error) {
	r, err := s.q.LockSaleAppointment(ctx, db.LockSaleAppointmentParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.AppointmentForSale{}, false, nil
	}
	if err != nil {
		return sales.AppointmentForSale{}, false, err
	}
	return sales.AppointmentForSale{ID: r.ID, ShopID: r.ShopID, ProfessionalID: r.ProfessionalID, State: sales.SaleAppointmentState(r.Status)}, true, nil
}
