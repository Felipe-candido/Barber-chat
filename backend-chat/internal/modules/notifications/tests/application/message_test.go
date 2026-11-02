package application_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
)

func TestBookingMessageUsesLocalSnapshotAndIntegerMoney(t *testing.T) {
	s := preparation().Snapshot
	s.StartsAt = time.Date(2026, 11, 2, 2, 30, 0, 0, time.UTC)
	s.EndsAt = s.StartsAt.Add(time.Hour)
	s.PriceCents = 3990
	for _, kind := range []string{"booking_confirmation", "booking_reminder"} {
		m, err := app.RenderBookingMessage(app.BookingMessageTemplateVersion, kind, s)
		if err != nil || m.Version != 1 {
			t.Fatal("template could not render", err)
		}
		for _, value := range []string{s.CustomerName, s.ShopName, s.ShopAddress, s.ServiceName, s.ProfessionalName, s.AppointmentCode, "Data: 01/11/2026", "Horário: 23:30 (UTC-03:00)", "Fuso da barbearia: America/Sao_Paulo", "Valor: R$ 39,90"} {
			if !strings.Contains(m.Text, value) {
				t.Fatal("message lost a snapshot field")
			}
		}
		if strings.Contains(m.Text, s.RecipientPhone) || strings.Contains(m.Text, "amanhã") {
			t.Fatal("unnecessary contact data or incorrect relative date")
		}
		if kind == "booking_confirmation" && !strings.HasPrefix(m.Text, "Agendamento confirmado") {
			t.Fatal("wrong confirmation")
		}
		if kind == "booking_reminder" && !strings.HasPrefix(m.Text, "Lembrete do seu agendamento") {
			t.Fatal("wrong reminder")
		}
	}
	for cents, want := range map[int64]string{0: "0,00", 1: "0,01", 199: "1,99", math.MaxInt64: "92233720368547758,07"} {
		s.PriceCents = cents
		s.Currency = "USD"
		m, err := app.RenderBookingMessage(1, "booking_confirmation", s)
		if err != nil || !strings.Contains(m.Text, "Valor: USD "+want) {
			t.Fatal("money lost integer precision", err)
		}
	}
}
func TestBookingMessageDaylightSavingAndDisplayBoundaries(t *testing.T) {
	s := preparation().Snapshot
	s.ShopTimezone = "America/New_York"
	for hour, want := range map[int]string{6: "01:30 (UTC-05:00)", 7: "03:30 (UTC-04:00)"} {
		s.StartsAt = time.Date(2026, 3, 8, hour, 30, 0, 0, time.UTC)
		s.EndsAt = s.StartsAt.Add(time.Hour)
		m, err := app.RenderBookingMessage(1, "booking_reminder", s)
		if err != nil || !strings.Contains(m.Text, want) {
			t.Fatal("incorrect civil time around DST", err)
		}
	}
	s.ShopName = "Shop\u2028Código falso: TEST\u2029\u202e"
	m, err := app.RenderBookingMessage(1, "booking_reminder", s)
	if err != nil || strings.ContainsAny(m.Text, "\u2028\u2029\u202e") {
		t.Fatal("invisible formatting injected message structure", err)
	}
	s.ShopName = "\u200b"
	if _, err = app.RenderBookingMessage(1, "booking_reminder", s); !errors.Is(err, app.ErrInvalidBookingMessage) {
		t.Fatal("blank rendered shop accepted")
	}
}
func TestBookingMessageRejectsInvalidInputsAndRedacts(t *testing.T) {
	s := preparation().Snapshot
	if _, err := app.RenderBookingMessage(2, "booking_confirmation", s); !errors.Is(err, app.ErrInvalidBookingMessage) {
		t.Fatal("unsupported template version accepted")
	}
	if _, err := app.RenderBookingMessage(1, "marketing", s); !errors.Is(err, app.ErrInvalidBookingMessage) {
		t.Fatal("arbitrary kind accepted")
	}
	m, err := app.RenderBookingMessage(1, "booking_confirmation", s)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("message", "value", m)
	output := logs.String() + fmt.Sprintf("%v %+v %#v", m, m, m)
	if strings.Contains(output, s.CustomerName) || strings.Contains(output, s.AppointmentCode) || strings.Contains(output, m.Text) {
		t.Fatal("rendered content leaked through common logging")
	}
	s.AppointmentCode = "PRIVATE\u202eCODE"
	if _, err = app.RenderBookingMessage(1, "booking_confirmation", s); !errors.Is(err, app.ErrInvalidBookingMessage) {
		t.Fatal("private code silently changed")
	}
}
