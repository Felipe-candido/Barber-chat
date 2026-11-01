package application_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type recordingSealer struct {
	calls    int
	scope    app.PayloadScope
	snapshot app.BookingSnapshot
	err      error
}

func (s *recordingSealer) Seal(_ context.Context, scope app.PayloadScope, snapshot app.BookingSnapshot) (app.ProtectedPayload, error) {
	s.calls++
	s.scope, s.snapshot = scope, snapshot
	if s.err != nil {
		return app.ProtectedPayload{}, s.err
	}
	return app.ProtectedPayload{Version: 1, KeyID: "test-key", Ciphertext: []byte("protected-test-payload")}, nil
}

func preparation() app.PrepareBookingNotificationsInput {
	created := time.Date(2026, 11, 1, 12, 30, 0, 0, time.UTC)
	return app.PrepareBookingNotificationsInput{
		ShopID: uuid.New(), AppointmentID: uuid.New(), ScheduleGeneration: 2, CreatedAt: created,
		Policy: domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 4}, PermissionGranted: true,
		Snapshot: app.BookingSnapshot{
			ShopName: "Test Shop", ShopSlug: "test-shop", ShopAddress: "Test Street, 123",
			CustomerName: "Private Customer", RecipientPhone: "+5511912345678", AppointmentCode: "PRIVATE-CODE",
			ProfessionalName: "Test Professional", ServiceName: "Test Service", PriceCents: 4000, Currency: "BRL",
			StartsAt: created.Add(72 * time.Hour), EndsAt: created.Add(73 * time.Hour), ShopTimezone: "America/Sao_Paulo",
		},
	}
}

func TestPrepareCalculatesPlanAndBindsProtectedSnapshot(t *testing.T) {
	input := preparation()
	sealer := &recordingSealer{}
	uc := app.NewPrepareBookingNotifications(sealer)
	out, err := uc.Execute(context.Background(), input)
	if err != nil || sealer.calls != 1 || !out.Schedule.Confirmation.Valid() || !out.Schedule.Reminder.Valid() || out.Schedule.PolicyVersion != 4 {
		t.Fatal("plan was not prepared", err)
	}
	if out.Scope.ShopID != input.ShopID || out.Scope.AppointmentID != input.AppointmentID ||
		out.Scope.ScheduleGeneration != 2 || !out.Scope.ExpiresAt.Equal(out.Schedule.RetainUntil()) ||
		sealer.scope != out.Scope || sealer.snapshot != input.Snapshot || len(out.Payload.Ciphertext) == 0 {
		t.Fatal("protected payload lost its scope or immutable snapshot")
	}
}

func TestPrepareDisabledOrUnpermittedDoesNotProtectOrRetainContact(t *testing.T) {
	for _, disabled := range []bool{true, false} {
		input := preparation()
		input.Policy.Enabled, input.PermissionGranted = !disabled, disabled
		sealer := &recordingSealer{}
		out, err := app.NewPrepareBookingNotifications(sealer).Execute(context.Background(), input)
		expected := domain.SkipPermissionMissing
		if disabled {
			expected = domain.SkipPolicyDisabled
		}
		if err != nil || sealer.calls != 0 || out.Schedule.SkipReason != expected || out.Schedule.PolicyVersion != input.Policy.Version ||
			out.Schedule.Confirmation.Valid() || out.Schedule.Reminder.Valid() || out.Scope != (app.PayloadScope{}) || len(out.Payload.Ciphertext) != 0 {
			t.Fatal("skipped plan retained sendable work", err)
		}
	}
}

func TestPrepareNearAppointmentRetainsOnlyConfirmation(t *testing.T) {
	input := preparation()
	input.Snapshot.StartsAt = input.CreatedAt.Add(30 * time.Minute)
	sealer := &recordingSealer{}
	out, err := app.NewPrepareBookingNotifications(sealer).Execute(context.Background(), input)
	if err != nil || sealer.calls != 1 || out.Schedule.Reminder.Valid() || out.Schedule.ReminderSkipReason != domain.SkipInsufficientLead ||
		!out.Scope.ExpiresAt.Equal(input.Snapshot.StartsAt) {
		t.Fatal("short-notice reservation must retain only its confirmation", err)
	}
}

func TestPreparationFailsClosedOnValidationOrProtectionFailure(t *testing.T) {
	for name, change := range map[string]func(*app.PrepareBookingNotificationsInput){
		"tenant":      func(i *app.PrepareBookingNotificationsInput) { i.ShopID = uuid.Nil },
		"appointment": func(i *app.PrepareBookingNotificationsInput) { i.AppointmentID = uuid.Nil },
		"generation":  func(i *app.PrepareBookingNotificationsInput) { i.ScheduleGeneration = 0 },
		"policy":      func(i *app.PrepareBookingNotificationsInput) { i.Policy.Version = 0 },
		"date":        func(i *app.PrepareBookingNotificationsInput) { i.CreatedAt = i.Snapshot.StartsAt },
		"contact":     func(i *app.PrepareBookingNotificationsInput) { i.Snapshot.RecipientPhone = "(11) 91234-5678" },
	} {
		t.Run(name, func(t *testing.T) {
			i := preparation()
			change(&i)
			sealer := &recordingSealer{}
			out, err := app.NewPrepareBookingNotifications(sealer).Execute(context.Background(), i)
			if err == nil || sealer.calls != 0 || out.Schedule.Confirmation.Valid() || len(out.Payload.Ciphertext) != 0 {
				t.Fatal("invalid input produced a usable plan")
			}
		})
	}
	problem := errors.New("test protection unavailable")
	sealer := &recordingSealer{err: problem}
	out, err := app.NewPrepareBookingNotifications(sealer).Execute(context.Background(), preparation())
	if !errors.Is(err, problem) || out.Scope.Valid() || out.Schedule.Confirmation.Valid() || len(out.Payload.Ciphertext) != 0 {
		t.Fatal("protection failure returned partial work")
	}
	if _, err := app.NewPrepareBookingNotifications(nil).Execute(context.Background(), preparation()); !errors.Is(err, app.ErrInvalidPreparation) {
		t.Fatal("missing sealer accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sealer.calls = 0
	if _, err := app.NewPrepareBookingNotifications(sealer).Execute(ctx, preparation()); !errors.Is(err, context.Canceled) || sealer.calls != 0 {
		t.Fatal("cancelled preparation protected data")
	}
}

func TestSnapshotValidationAndAccidentalLogRedaction(t *testing.T) {
	snapshot := preparation().Snapshot
	for name, change := range map[string]func(*app.BookingSnapshot){
		"address":  func(s *app.BookingSnapshot) { s.ShopAddress = "" },
		"control":  func(s *app.BookingSnapshot) { s.CustomerName += "\nInjected" },
		"encoding": func(s *app.BookingSnapshot) { s.ServiceName = string([]byte{0xff}) },
		"length":   func(s *app.BookingSnapshot) { s.ShopName = strings.Repeat("a", 201) },
		"slug":     func(s *app.BookingSnapshot) { s.ShopSlug = "shop--name" },
		"phone":    func(s *app.BookingSnapshot) { s.RecipientPhone = "+05511912345678" },
		"code":     func(s *app.BookingSnapshot) { s.AppointmentCode = "PRIVATE CODE" },
		"price":    func(s *app.BookingSnapshot) { s.PriceCents = -1 },
		"currency": func(s *app.BookingSnapshot) { s.Currency = "brl" },
		"end":      func(s *app.BookingSnapshot) { s.EndsAt = s.StartsAt },
		"timezone": func(s *app.BookingSnapshot) { s.ShopTimezone = "Local" },
	} {
		t.Run(name, func(t *testing.T) {
			s := snapshot
			change(&s)
			if s.Valid() {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
	snapshot.ShopName = strings.Repeat("á", 150)
	if !snapshot.Valid() {
		t.Fatal("valid Unicode shop name was rejected using byte length")
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logger.Info("test", "snapshot", snapshot)
	logs.WriteString(fmt.Sprintf("%v %+v %#v", snapshot, snapshot, snapshot))
	for _, private := range []string{snapshot.CustomerName, snapshot.RecipientPhone, snapshot.AppointmentCode} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("snapshot leaked through ordinary structured or formatted logging")
		}
	}
}
