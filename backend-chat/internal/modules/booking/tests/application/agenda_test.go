package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/google/uuid"
)

type agendaReader struct {
	shop  app.ShopContext
	query app.AgendaQuery
	items []app.AdminAppointment
	err   error
}

func (r *agendaReader) FindAgendaShop(context.Context, uuid.UUID) (app.ShopContext, bool, error) {
	return r.shop, true, nil
}
func (r *agendaReader) ListAgendaAppointments(_ context.Context, query app.AgendaQuery) ([]app.AdminAppointment, error) {
	r.query = query
	return r.items, r.err
}

func TestAgendaShopTimezoneAndBoundedPagination(t *testing.T) {
	id := uuid.New()
	r := &agendaReader{shop: app.ShopContext{ShopID: id, Name: "Shop", Timezone: "America/New_York"}}
	for i := 0; i < app.AgendaPageSize+1; i++ {
		r.items = append(r.items, app.AdminAppointment{Appointment: app.AppointmentReceipt{AppointmentID: uuid.New(), StartsAt: time.Date(2026, 11, 2, 10, i, 0, 0, time.UTC)}})
	}
	uc := app.NewListAgenda(r, r, app.ClockFunc(func() time.Time { return time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC) }))
	output, err := uc.Execute(context.Background(), app.ListAgendaInput{ShopID: id, Month: "2026-11"})
	if err != nil {
		t.Fatal(err)
	}
	if output.Today != "2026-10-31" || output.FromDate != "2026-10-26" || output.UntilDate != "2026-12-07" {
		t.Fatal("dates were not calculated in the shop timezone", output)
	}
	if r.query.From != time.Date(2026, 10, 26, 4, 0, 0, 0, time.UTC) || r.query.Until != time.Date(2026, 12, 7, 5, 0, 0, 0, time.UTC) {
		t.Fatal("civil range did not preserve DST boundaries", r.query)
	}
	if r.query.ShopID != id || r.query.Limit != 101 || len(output.Appointments) != 100 || output.Next == nil || output.Next.ID != r.items[99].Appointment.AppointmentID {
		t.Fatal("invalid bounded pagination")
	}
	if output.Appointments[0].Appointment.ShopName != "Shop" {
		t.Fatal("missing shop metadata")
	}
	for _, input := range []app.ListAgendaInput{
		{Month: "2026-11"}, {ShopID: id, Month: "2026-13"}, {ShopID: id, Month: "2026-1"}, {ShopID: id, Month: "0000-01"},
		{ShopID: id, Month: "2026-11", After: &app.AgendaCursor{ID: uuid.New(), StartsAt: r.query.Until}},
		{ShopID: id, Month: "2026-11", After: &app.AgendaCursor{ID: uuid.Nil, StartsAt: r.query.From}},
	} {
		if _, err := uc.Execute(context.Background(), input); !errors.Is(err, app.ErrInvalidAgenda) {
			t.Fatal("invalid agenda accepted", input, err)
		}
	}
	r.items = nil
	output, err = uc.Execute(context.Background(), app.ListAgendaInput{ShopID: id})
	if err != nil || output.Month != "2026-10" || output.Appointments == nil {
		t.Fatal("default month/empty list contract", output, err)
	}
	r.err = context.DeadlineExceeded
	if _, err := uc.Execute(context.Background(), app.ListAgendaInput{ShopID: id}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("operational error lost", err)
	}
	r.shop.ShopID = uuid.New()
	if _, err := uc.Execute(context.Background(), app.ListAgendaInput{ShopID: id}); !errors.Is(err, app.ErrInconsistentBookingData) {
		t.Fatal("mismatched shop accepted", err)
	}
}
