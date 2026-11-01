package http

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

type cursorEnvelope struct {
	Version  int                   `json:"v"`
	At       time.Time             `json:"at"`
	Source   domain.ActivitySource `json:"source"`
	RecordID uuid.UUID             `json:"record_id"`
	ItemID   uuid.UUID             `json:"item_id"`
	Binding  string                `json:"binding"`
}

func encodeCursor(c *application.ActivityCursor) (string, error) {
	if c == nil {
		return "", nil
	}
	if c.OccurredAt.IsZero() || c.OccurredAt.UTC().Year() < 1 || c.OccurredAt.UTC().Year() > 9999 || !c.Source.IsRowSource() || c.RecordID == uuid.Nil ||
		(c.Source == domain.ActivityServices && c.ItemID != uuid.Nil) || (c.Source == domain.ActivityProducts && c.ItemID == uuid.Nil) || c.FilterFingerprint == ([32]byte{}) {
		return "", application.ErrInconsistentData
	}
	data, err := json.Marshal(cursorEnvelope{1, c.OccurredAt.UTC(), c.Source, c.RecordID, c.ItemID, hex.EncodeToString(c.FilterFingerprint[:])})
	if err != nil {
		return "", application.ErrInconsistentData
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeCursor(raw string) (*application.ActivityCursor, error) {
	if len(raw) == 0 || len(raw) > 1024 {
		return nil, application.ErrInvalidQuery
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return nil, application.ErrInvalidQuery
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire cursorEnvelope
	if err := decoder.Decode(&wire); err != nil || wire.Version != 1 {
		return nil, application.ErrInvalidQuery
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, application.ErrInvalidQuery
	}
	binding, err := hex.DecodeString(wire.Binding)
	if err != nil || len(binding) != 32 {
		return nil, application.ErrInvalidQuery
	}
	c := &application.ActivityCursor{OccurredAt: wire.At, Source: wire.Source, RecordID: wire.RecordID, ItemID: wire.ItemID}
	copy(c.FilterFingerprint[:], binding)
	// Tokens are emitted only by us. Canonical re-encoding also rejects duplicate
	// JSON fields, nulls, extra whitespace, CRLF and noncanonical base64 variants.
	canonical, err := encodeCursor(c)
	if err != nil || canonical != raw {
		return nil, application.ErrInvalidQuery
	}
	return c, nil
}
