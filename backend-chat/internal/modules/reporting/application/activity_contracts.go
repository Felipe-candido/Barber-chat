package application

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

// Canonical reporting contracts. HTTP payloads are mapped by the transport adapter.
type ActivityFilterInput struct {
	ReportFilterInput
	ProductID uuid.UUID
}

type ActivityFilters struct {
	ReportFilters
	ProductID uuid.UUID
}

type ActivityReportInput struct {
	ActorID, ShopID       uuid.UUID
	FromDate, ThroughDate string
	Source                domain.ActivitySource
	Filters               ActivityFilterInput
}

type ActivityQuery struct {
	ShopID  uuid.UUID
	Period  domain.Period
	Source  domain.ActivitySource
	Filters ActivityFilters // Professional filter already clamped to current access.
}

type DailyActivity struct {
	LocalDate string
	Counts    OutcomeCounts
	Values    []ActivityValues
}

// Group by stable ID, never name. Label uses the latest matching snapshot by
// (OccurredAt, Source, RecordID, ItemID). A nil professional is the unassigned
// group, visible only to the owner. Service/product groups contain that branch.
type ActivityGroup struct {
	ID     uuid.UUID
	Label  string
	Counts OutcomeCounts
	Values []ActivityValues
}

type ActivityOverview struct {
	Period                               domain.Period
	Source                               domain.ActivitySource
	ObservedAt                           time.Time // Database snapshot instant; later corrections can change totals.
	Counts                               OutcomeCounts
	Values                               []ActivityValues
	ByDay                                []DailyActivity
	ByProfessional, ByService, ByProduct []ActivityGroup
}

// Order descending by (OccurredAt, Source, RecordID, ItemID). Bind the opaque
// cursor to actor, tenant, period, source, statuses, normalized filters AND the
// effective professional authority. Recheck access for every page; a fingerprint
// is a query binding, not a credential or substitute for authorization.
type ActivityCursor struct {
	OccurredAt        time.Time
	Source            domain.ActivitySource
	RecordID, ItemID  uuid.UUID
	FilterFingerprint [32]byte
}

type ActivityHistoryInput struct {
	ActivityReportInput
	AppointmentStatus domain.AppointmentStatus // Empty means all outcomes.
	SaleStatus        domain.ProductSaleStatus // Empty means all sale outcomes.
	After             *ActivityCursor
	PageSize          int // Zero defaults to 25; otherwise 1..100. Fetch page size + 1.
}

type ActivityHistoryQuery struct {
	ActivityQuery
	AppointmentStatus domain.AppointmentStatus // Empty means all outcomes.
	SaleStatus        domain.ProductSaleStatus
	After             *ActivityCursor
	Limit             int // Bounded fetch size, at most MaxHistoryPageSize + 1.
}

// One appointment OR one sale item. OccurredAt is appointment.starts_at or
// sale.sold_at, independently. ItemID is nil for an appointment. No booking code,
// status reason, actor details, hash or encrypted public receipt belongs here.
// Terms are snapshots; contact is current shop-local data. ValueCents preserves
// the ORIGINAL amount even when excluded from totals (see RecordedValueCents).
type ActivityHistoryRow struct {
	ShopID                               uuid.UUID
	Source                               domain.ActivitySource
	RecordID, ItemID                     uuid.UUID
	ServiceID, ProductID, ProfessionalID uuid.UUID
	Label, ProfessionalName, Currency    string
	AppointmentStatus                    domain.AppointmentStatus
	AppointmentStatusSource              domain.AppointmentStatusSource
	SaleStatus                           domain.ProductSaleStatus
	CustomerName, CustomerPhone          string // Empty for anonymous standalone sales.
	OccurredAt                           time.Time
	Quantity, UnitPriceCents, ValueCents int64 // Appointment quantity = 1; product = item quantity.
}

type ActivityHistoryPage struct {
	Period     domain.Period
	Source     domain.ActivitySource
	ObservedAt time.Time
	Rows       []ActivityHistoryRow
	Next       *ActivityCursor
}
