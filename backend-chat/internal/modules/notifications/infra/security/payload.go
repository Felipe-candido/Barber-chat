package security

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
)

const (
	payloadVersion    = 1
	maxPlaintextBytes = 16 * 1024
)

var (
	ErrInvalidKeyring = errors.New("invalid notification payload keyring")
	ErrInvalidPayload = errors.New("invalid protected notification payload")
	ErrPayloadExpired = errors.New("notification payload expired")
)

// PayloadCipher keeps immutable AES-256-GCM instances. A new active key seals new
// payloads; retained keys can open older ones until their expiry. Never log keys.
type PayloadCipher struct {
	activeKeyID string
	keys        map[string]cipher.AEAD
}

var _ application.PayloadSealer = (*PayloadCipher)(nil)

func NewPayloadCipher(activeKeyID string, keys map[string][]byte) (*PayloadCipher, error) {
	if !validKeyID(activeKeyID) || len(keys) == 0 {
		return nil, ErrInvalidKeyring
	}
	keyring := make(map[string]cipher.AEAD, len(keys))
	for id, key := range keys {
		if !validKeyID(id) || len(key) != 32 {
			return nil, ErrInvalidKeyring
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, ErrInvalidKeyring
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrInvalidKeyring
		}
		keyring[id] = aead
	}
	if keyring[activeKeyID] == nil {
		return nil, ErrInvalidKeyring
	}
	return &PayloadCipher{activeKeyID: activeKeyID, keys: keyring}, nil
}

func (c *PayloadCipher) Seal(ctx context.Context, scope application.PayloadScope, snapshot application.BookingSnapshot) (application.ProtectedPayload, error) {
	if err := ctx.Err(); err != nil {
		return application.ProtectedPayload{}, err
	}
	if !scope.Valid() {
		return application.ProtectedPayload{}, application.ErrInvalidPayloadScope
	}
	if !snapshot.Valid() || scope.ExpiresAt.After(snapshot.StartsAt) {
		return application.ProtectedPayload{}, application.ErrInvalidSnapshot
	}
	if c == nil || c.keys[c.activeKeyID] == nil {
		return application.ProtectedPayload{}, ErrInvalidKeyring
	}
	snapshot.StartsAt, snapshot.EndsAt = snapshot.StartsAt.UTC(), snapshot.EndsAt.UTC()
	plaintext, err := json.Marshal(snapshot)
	if err != nil || len(plaintext) > maxPlaintextBytes {
		return application.ProtectedPayload{}, ErrInvalidPayload
	}
	defer clear(plaintext)
	payload := application.ProtectedPayload{Version: payloadVersion, KeyID: c.activeKeyID}
	aad := payloadAAD(scope, payload)
	payload.Ciphertext = c.keys[c.activeKeyID].Seal(nil, nil, plaintext, aad)
	return payload, nil
}

// Open requires an explicit trusted clock observation. Expiry is authenticated
// and checked here, while durable cleanup of expired rows remains a separate job.
func (c *PayloadCipher) Open(ctx context.Context, now time.Time, scope application.PayloadScope, payload application.ProtectedPayload) (application.BookingSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return application.BookingSnapshot{}, err
	}
	if !scope.Valid() || now.IsZero() || now.UTC().Year() < 1 || now.UTC().Year() > 9999 {
		return application.BookingSnapshot{}, application.ErrInvalidPayloadScope
	}
	if c == nil || payload.Version != payloadVersion || c.keys[payload.KeyID] == nil {
		return application.BookingSnapshot{}, ErrInvalidPayload
	}
	aead := c.keys[payload.KeyID]
	if len(payload.Ciphertext) < aead.Overhead() || len(payload.Ciphertext) > maxPlaintextBytes+aead.Overhead() {
		return application.BookingSnapshot{}, ErrInvalidPayload
	}
	plaintext, err := aead.Open(nil, nil, payload.Ciphertext, payloadAAD(scope, payload))
	if err != nil {
		return application.BookingSnapshot{}, ErrInvalidPayload
	}
	defer clear(plaintext)
	if !now.Before(scope.ExpiresAt) {
		return application.BookingSnapshot{}, ErrPayloadExpired
	}
	var snapshot application.BookingSnapshot
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil || !snapshot.Valid() || scope.ExpiresAt.After(snapshot.StartsAt) {
		return application.BookingSnapshot{}, ErrInvalidPayload
	}
	if decoder.Decode(new(any)) != io.EOF {
		return application.BookingSnapshot{}, ErrInvalidPayload
	}
	return snapshot, nil
}

func payloadAAD(scope application.PayloadScope, payload application.ProtectedPayload) []byte {
	// A fixed-order document binds unambiguous identifiers and nanosecond instants.
	// Inputs have already been checked, so these fields cannot fail JSON encoding.
	data, _ := json.Marshal(struct {
		Protocol string
		Version  int
		KeyID    string
		Scope    application.PayloadScope
	}{"barber-chat/notification-payload", payload.Version, payload.KeyID, application.PayloadScope{
		ShopID: scope.ShopID, AppointmentID: scope.AppointmentID,
		ScheduleGeneration: scope.ScheduleGeneration, ExpiresAt: scope.ExpiresAt.UTC(),
	}})
	return data
}

func validKeyID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, ch := range s {
		if ch != '-' && ch != '_' && !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}
