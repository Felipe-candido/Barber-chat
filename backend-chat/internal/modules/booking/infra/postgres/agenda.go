package postgres

import (
	"context"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

// This private, read-only projection joins local contact data without modifying
// customers. Snapshots are not replaced with today's catalog names or prices.
func (r *Repository) ListAgendaAppointments(ctx context.Context, query application.AgendaQuery) ([]application.AdminAppointment, error) {
	afterAt, afterID := query.From, uuid.Nil
	if query.After != nil {
		afterAt, afterID = query.After.StartsAt, query.After.ID
	}
	rows, err := r.queries.ListAdminAgenda(ctx, db.ListAdminAgendaParams{
		ShopID: query.ShopID, FromAt: timestamp(query.From), UntilAt: timestamp(query.Until),
		ProfessionalID: query.ProfessionalID,
		HasCursor:      query.After != nil, AfterAt: timestamp(afterAt), AfterID: afterID, PageLimit: int32(query.Limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]application.AdminAppointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, application.AdminAppointment{
			Appointment: application.AppointmentReceipt{
				AppointmentID: row.ID, Status: domain.Status(row.Status),
				Service:        domain.ServiceSnapshot{Name: row.ServiceName, DurationMinutes: int(row.DurationMinutes), PriceCents: row.PriceCents, Currency: row.Currency},
				ProfessionalID: row.ProfessionalID, ProfessionalName: row.ProfessionalName,
				StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(), ShopTimezone: row.ShopTimezone,
			}, CustomerName: row.CustomerName, CustomerPhone: row.PhoneE164,
		})
	}
	return items, nil
}
