package application

import (
	"context"

	"github.com/google/uuid"
)

// Read-only boundaries; composition into the HTTP API remains a separate step.
// Reuse the shared pool from bootstrap. Never open a reporting-owned database.
type ReportShopReader interface {
	FindActiveReportShop(context.Context, uuid.UUID) (ReportShop, bool, error)
}

// Normalize search input without resolving, creating or authenticating a customer.
// Implement locally, without network or persistence. Invalid input returns
// ErrInvalidQuery; successful output is a canonical E.164 number.
type PhoneNormalizer interface {
	NormalizeReportPhone(raw, country string) (string, error)
}
