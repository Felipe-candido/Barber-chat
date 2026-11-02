package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidProviderReceipt = errors.New("invalid provider receipt")

type ProviderReceipt struct {
	InstanceName string
	AttemptID    uuid.UUID
	Stage        int16
}
type ProviderReceiptStore interface {
	RecordReceipt(context.Context, ProviderReceipt) error
}
type ProviderReceiptRecorder interface {
	RecordReceipt(context.Context, ProviderReceipt) error
}
type RecordProviderReceipt struct{ store ProviderReceiptStore }

func NewRecordProviderReceipt(store ProviderReceiptStore) *RecordProviderReceipt {
	return &RecordProviderReceipt{store}
}
func (u *RecordProviderReceipt) RecordReceipt(ctx context.Context, r ProviderReceipt) error {
	if r.AttemptID == uuid.Nil || r.Stage < 1 || r.Stage > 3 || len(r.InstanceName) != 35 || !strings.HasPrefix(r.InstanceName, "bc_") || strings.Trim(r.InstanceName[3:], "0123456789abcdef") != "" {
		return ErrInvalidProviderReceipt
	}
	return u.store.RecordReceipt(ctx, r)
}
