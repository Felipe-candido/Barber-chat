//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	"github.com/google/uuid"
)

type committedConfirmationProbe struct {
	t     *testing.T
	n     *notificationFixture
	fresh int
}

func (p *committedConfirmationProbe) DispatchConfirmation(ctx context.Context, shop, appointment uuid.UUID, fresh bool) (booking.BookingConfirmationStatus, error) {
	if fresh {
		p.fresh++
	}
	var committed bool
	err := p.n.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM appointments a JOIN appointment_notification_plans p ON p.shop_id=a.shop_id AND p.appointment_id=a.id
 JOIN notification_jobs j ON j.shop_id=a.shop_id AND j.appointment_id=a.id WHERE a.shop_id=$1 AND a.id=$2)`, shop, appointment).Scan(&committed)
	if err != nil || !committed || shop != p.n.shop {
		p.t.Error("notification invoked before committed booking/plan/jobs", err)
	}
	return booking.BookingConfirmationStatus{}, errors.New("private provider error")
}
func TestNotificationConfirmationRunsAfterCommitWithoutBreakingBooking(t *testing.T) {
	n := newNotificationFixture(t)
	clock := booking.ClockFunc(time.Now)
	probe := &committedConfirmationProbe{t: t, n: n}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := bookinghttp.NewHandler(n.create, booking.NewListAvailability(n.tx, clock), booking.NewListProfessionals(n.tx, clock), booking.NewManageAppointment(n.tx, security.Codes{}, clock), 10*time.Second, logger).WithBookingConfirmation(probe)
	n.handler = httpapi.NewHandler(httpapi.HandlerConfig{Booking: h, Logger: logger})
	body := n.body(n.start.Add(48*time.Hour), n.service)
	body["whatsapp_notifications"] = true
	key := bookingKey()
	first := n.request("POST", "/appointments", body, key)
	saved := bookingResult(t, first, 201)
	var result struct {
		WhatsApp struct {
			State string `json:"state"`
		} `json:"whatsapp"`
	}
	if json.Unmarshal(first.Body.Bytes(), &result) != nil || result.WhatsApp.State != "unknown" || probe.fresh != 1 {
		t.Fatal("saved booking reclassified after message failure", first.Code)
	}
	replay := bookingResult(t, n.request("POST", "/appointments", body, key), 200)
	if replay.Code != saved.Code || probe.fresh != 1 || n.count(t, "appointments") != 1 {
		t.Fatal("replay initiated another notification or booking")
	}
}
