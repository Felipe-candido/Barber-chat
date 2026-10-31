package application

import "github.com/google/uuid"

const (
	DefaultHistoryPageSize = 25
	MaxHistoryPageSize     = 100
)

// Shared projections for Activity* contracts. These are not HTTP payloads.
// Actor and shop IDs come from authenticated scope, but IDs alone grant no
// access. Revalidate current access through ActivityAccess.
type ReportShop struct {
	ID       uuid.UUID
	Timezone string
}

// Contact filters search current shop-local contact data, not booking snapshots.
// They neither authenticate nor create a customer. Standalone sales without an
// appointment cannot match a name or phone filter. Resource IDs remain bound to
// ShopID, including inactive catalog resources.
type ReportFilters struct {
	CustomerName   string
	PhoneE164      string
	ProfessionalID uuid.UUID
	ServiceID      uuid.UUID
}

// Normalize phone input through PhoneNormalizer before constructing the query.
// Country is required for national-format numbers; phone is not a person ID.
type ReportFilterInput struct {
	CustomerName   string
	Phone          string
	PhoneCountry   string
	ProfessionalID uuid.UUID
	ServiceID      uuid.UUID
}

// Count each appointment once, irrespective of how many products were sold.
// CompletedAutomatically is a SUBSET of Completed, based on the current stored
// status source. Never infer completion during a report read.
type OutcomeCounts struct {
	Scheduled              int64
	Confirmed              int64
	Completed              int64
	CompletedAutomatically int64
	Cancelled              int64
	NoShow                 int64
}
