package application

import (
	"context"

	"github.com/google/uuid"
)

// Planned read-only boundaries; no adapter or report use case is wired yet.
// Reuse the shared pool from bootstrap. Never open a reporting-owned database.
type ReportShopReader interface {
	FindActiveReportShop(context.Context, uuid.UUID) (ReportShop, bool, error)
}

// Normalize search input without resolving, creating or authenticating a customer.
// A future adapter can delegate to the existing phone normalization capability.
type PhoneNormalizer interface {
	NormalizeReportPhone(raw, country string) (string, error)
}
