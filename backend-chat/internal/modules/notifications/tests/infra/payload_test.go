package infra_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/security"
	"github.com/google/uuid"
)

func payloadFixture() (time.Time, app.PayloadScope, app.BookingSnapshot) {
	now := time.Date(2026, 11, 1, 12, 30, 0, 123456000, time.UTC)
	return now, app.PayloadScope{ShopID: uuid.New(), AppointmentID: uuid.New(), ScheduleGeneration: 1, ExpiresAt: now.Add(49 * time.Hour)},
		app.BookingSnapshot{ShopName: "Test Shop", ShopSlug: "test-shop", ShopAddress: "Test Street, 123",
			CustomerName: "Private Customer", RecipientPhone: "+5511912345678", AppointmentCode: "PRIVATE-CODE-ONLY-FOR-TESTING",
			ProfessionalName: "Test Professional", ServiceName: "Test Service", PriceCents: 4000, Currency: "BRL",
			StartsAt: now.Add(72 * time.Hour), EndsAt: now.Add(73 * time.Hour), ShopTimezone: "America/Sao_Paulo"}
}

func newCipher(t *testing.T) *security.PayloadCipher {
	t.Helper()
	// Deterministic fixtures only; production key material must be random/secret.
	c, err := security.NewPayloadCipher("key-1", map[string][]byte{"key-1": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProtectedPayloadRoundTripAndRandomizedCiphertext(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	c := newCipher(t)
	first, err := c.Seal(context.Background(), scope, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Seal(context.Background(), scope, snapshot)
	if err != nil || bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("repeated sealing reused ciphertext", err)
	}
	for _, private := range []string{snapshot.CustomerName, snapshot.RecipientPhone, snapshot.AppointmentCode} {
		if bytes.Contains(first.Ciphertext, []byte(private)) {
			t.Fatal("protected payload contains plaintext")
		}
	}
	opened, err := c.Open(context.Background(), now, scope, first)
	if err != nil || !reflect.DeepEqual(opened, snapshot) {
		t.Fatal("round trip lost the protected snapshot", err)
	}
	// Equivalent expiry instants with different offsets must authenticate equally.
	scope.ExpiresAt = scope.ExpiresAt.In(time.FixedZone("shop", -3*3600))
	if _, err := c.Open(context.Background(), now, scope, first); err != nil {
		t.Fatal("equivalent UTC scope failed", err)
	}
}

func TestPayloadCannotMoveAcrossTenantAppointmentGenerationOrExpiry(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	c := newCipher(t)
	payload, err := c.Seal(context.Background(), scope, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*app.PayloadScope){
		"tenant":           func(s *app.PayloadScope) { s.ShopID = uuid.New() },
		"appointment":      func(s *app.PayloadScope) { s.AppointmentID = uuid.New() },
		"generation":       func(s *app.PayloadScope) { s.ScheduleGeneration++ },
		"extended expiry":  func(s *app.PayloadScope) { s.ExpiresAt = s.ExpiresAt.Add(time.Microsecond) },
		"shortened expiry": func(s *app.PayloadScope) { s.ExpiresAt = s.ExpiresAt.Add(-time.Microsecond) },
	} {
		t.Run(name, func(t *testing.T) {
			altered := scope
			change(&altered)
			opened, err := c.Open(context.Background(), now, altered, payload)
			if !errors.Is(err, security.ErrInvalidPayload) || opened != (app.BookingSnapshot{}) {
				t.Fatal("modified scope opened plaintext")
			}
		})
	}
}

func TestPayloadRejectsCorruptionAndMetadataSubstitution(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	key := bytes.Repeat([]byte{1}, 32)
	c, err := security.NewPayloadCipher("key-1", map[string][]byte{"key-1": key, "alias": key})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := c.Seal(context.Background(), scope, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*app.ProtectedPayload){
		"ciphertext":          func(p *app.ProtectedPayload) { p.Ciphertext[len(p.Ciphertext)-1] ^= 1 },
		"truncated":           func(p *app.ProtectedPayload) { p.Ciphertext = p.Ciphertext[:20] },
		"empty":               func(p *app.ProtectedPayload) { p.Ciphertext = nil },
		"oversized":           func(p *app.ProtectedPayload) { p.Ciphertext = make([]byte, 20*1024) },
		"version":             func(p *app.ProtectedPayload) { p.Version++ },
		"unknown key":         func(p *app.ProtectedPayload) { p.KeyID = "missing" },
		"same-material alias": func(p *app.ProtectedPayload) { p.KeyID = "alias" },
	} {
		t.Run(name, func(t *testing.T) {
			altered := payload
			altered.Ciphertext = bytes.Clone(payload.Ciphertext)
			change(&altered)
			opened, err := c.Open(context.Background(), now, scope, altered)
			if !errors.Is(err, security.ErrInvalidPayload) || opened != (app.BookingSnapshot{}) {
				t.Fatal("tampered envelope opened plaintext")
			}
		})
	}
}

func TestPayloadExpiryAndContextCancellationDoNotReturnPlaintext(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	c := newCipher(t)
	payload, err := c.Seal(context.Background(), scope, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Open(context.Background(), scope.ExpiresAt.Add(-time.Nanosecond), scope, payload); err != nil {
		t.Fatal("payload expired too early", err)
	}
	for _, at := range []time.Time{scope.ExpiresAt, scope.ExpiresAt.Add(time.Hour)} {
		opened, err := c.Open(context.Background(), at, scope, payload)
		if !errors.Is(err, security.ErrPayloadExpired) || opened != (app.BookingSnapshot{}) {
			t.Fatal("expired payload exposed its contents")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if opened, err := c.Open(ctx, now, scope, payload); !errors.Is(err, context.Canceled) || opened != (app.BookingSnapshot{}) {
		t.Fatal("cancelled decryption returned plaintext")
	}
	if sealed, err := c.Seal(ctx, scope, snapshot); !errors.Is(err, context.Canceled) || len(sealed.Ciphertext) != 0 {
		t.Fatal("cancelled encryption returned a payload")
	}
}

func TestPayloadKeyRotationAndConfigurationOwnership(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	oldKey, newKey := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	initial, err := security.NewPayloadCipher("old", map[string][]byte{"old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldPayload, err := initial.Seal(context.Background(), scope, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{"old": oldKey, "new": newKey}
	rotated, err := security.NewPayloadCipher("new", keys)
	if err != nil {
		t.Fatal(err)
	}
	// Changing caller-owned config after construction cannot change the cipher.
	clear(oldKey)
	clear(newKey)
	delete(keys, "old")
	if _, err := rotated.Open(context.Background(), now, scope, oldPayload); err != nil {
		t.Fatal("rotation lost an unexpired old payload", err)
	}
	newPayload, err := rotated.Seal(context.Background(), scope, snapshot)
	if err != nil || newPayload.KeyID != "new" {
		t.Fatal("rotation did not seal with the active key", err)
	}
	if _, err := initial.Open(context.Background(), now, scope, newPayload); !errors.Is(err, security.ErrInvalidPayload) {
		t.Fatal("old keyring unexpectedly opened a new-key payload")
	}
}

func TestKeyringAndPayloadInputsFailWithoutPanics(t *testing.T) {
	for _, tc := range []struct {
		active string
		keys   map[string][]byte
	}{
		{"", nil}, {"missing", map[string][]byte{"key": make([]byte, 32)}},
		{"key", map[string][]byte{"key": make([]byte, 16)}}, {"key", map[string][]byte{"key": make([]byte, 24)}},
		{"key", map[string][]byte{"key": nil}}, {"bad key", map[string][]byte{"bad key": make([]byte, 32)}},
	} {
		if _, err := security.NewPayloadCipher(tc.active, tc.keys); !errors.Is(err, security.ErrInvalidKeyring) {
			t.Fatal("invalid keyring accepted")
		}
	}
	now, scope, snapshot := payloadFixture()
	c := newCipher(t)
	invalidScope := scope
	invalidScope.ShopID = uuid.Nil
	if _, err := c.Seal(context.Background(), invalidScope, snapshot); !errors.Is(err, app.ErrInvalidPayloadScope) {
		t.Fatal("missing tenant accepted")
	}
	invalidScope = scope
	invalidScope.ExpiresAt = invalidScope.ExpiresAt.Add(time.Nanosecond)
	if _, err := c.Seal(context.Background(), invalidScope, snapshot); !errors.Is(err, app.ErrInvalidPayloadScope) {
		t.Fatal("sub-microsecond expiry cannot survive a PostgreSQL round trip")
	}
	invalidScope = scope
	invalidScope.ExpiresAt = snapshot.StartsAt.Add(time.Second)
	if _, err := c.Seal(context.Background(), invalidScope, snapshot); !errors.Is(err, app.ErrInvalidSnapshot) {
		t.Fatal("retention past appointment start accepted")
	}
	var absent *security.PayloadCipher
	if _, err := absent.Seal(context.Background(), scope, snapshot); !errors.Is(err, security.ErrInvalidKeyring) {
		t.Fatal("nil cipher should fail safely")
	}
	if _, err := c.Open(context.Background(), now, invalidScope, app.ProtectedPayload{}); !errors.Is(err, security.ErrInvalidPayload) {
		t.Fatal("empty protected envelope accepted")
	}
}

func TestPreparationUsesRealProtectionWithoutChangingBookingReceipt(t *testing.T) {
	now, scope, snapshot := payloadFixture()
	c := newCipher(t)
	uc := app.NewPrepareBookingNotifications(c)
	prepared, err := uc.Execute(context.Background(), app.PrepareBookingNotificationsInput{
		ShopID: scope.ShopID, AppointmentID: scope.AppointmentID, ScheduleGeneration: scope.ScheduleGeneration,
		CreatedAt: now, Policy: domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 1},
		PermissionGranted: true, Snapshot: snapshot,
	})
	if err != nil || !prepared.Scope.Valid() || !prepared.Schedule.Reminder.Valid() {
		t.Fatal("could not prepare protected plan", err)
	}
	opened, err := c.Open(context.Background(), prepared.Schedule.Reminder.DueAt, prepared.Scope, prepared.Payload)
	if err != nil || opened.AppointmentCode != snapshot.AppointmentCode || opened.RecipientPhone != snapshot.RecipientPhone {
		t.Fatal("worker cannot recover the same code at reminder time", err)
	}
}
