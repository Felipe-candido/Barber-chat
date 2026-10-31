package application

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

type SalesScope struct{ ActorID, ShopID uuid.UUID }

type SaleItemInput struct {
	ProductID                                                uuid.UUID
	Quantity, ExpectedProductVersion, ExpectedUnitPriceCents int64
}

// Expected price is a review precondition, never the authoritative sale price.
type RegisterSaleInput struct {
	Scope                         SalesScope
	ProfessionalID, AppointmentID uuid.UUID
	Items                         []SaleItemInput
	IdempotencyKey                string
}

type CancelSaleInput struct {
	Scope                  SalesScope
	SaleID                 uuid.UUID
	ExpectedVersion        int64
	Reason, IdempotencyKey string
	Restock                bool
}

type ProductForSale struct {
	ID, ShopID                          uuid.UUID
	Name, Currency                      string
	PriceCents, Version, QuantityOnHand int64
	Active                              bool
}

type SaleProfessional struct {
	ID, ShopID uuid.UUID
	Name       string
	Active     bool
}

// Consumer-owned projection: booking's domain is not imported here.
type SaleAppointmentState string

const (
	SaleAppointmentScheduled SaleAppointmentState = "scheduled"
	SaleAppointmentConfirmed SaleAppointmentState = "confirmed"
	SaleAppointmentCompleted SaleAppointmentState = "completed"
)

type AppointmentForSale struct {
	ID, ShopID, ProfessionalID uuid.UUID
	State                      SaleAppointmentState
}

type SaleStockItem struct {
	SaleItemID, ProductID uuid.UUID
	Quantity              int64
}

// Items must come from the persisted/server-validated sale, never raw JSON.
type SaleStockCommand struct {
	ShopID, ActorID uuid.UUID
	Items           []SaleStockItem
	OccurredAt      time.Time
	Reason          string
}

type SaleOperation string

const (
	RegisterSaleOperation SaleOperation = "record_sale"
	CancelSaleOperation   SaleOperation = "cancel_sale"
)

type SaleCommandKey struct {
	Operation           SaleOperation
	Digest, Fingerprint [32]byte
}

type SaleCursor struct {
	SoldAt time.Time
	SaleID uuid.UUID
}

type SaleListQuery struct {
	ShopID, ProfessionalID uuid.UUID // Nil professional = all, owner only.
	From, Until            time.Time // UTC [from, until), derived from the shop timezone.
	Status                 domain.SaleStatus
	After                  *SaleCursor
	Limit                  int
}

type ListSalesInput struct {
	Scope          SalesScope
	ProfessionalID uuid.UUID
	From, Through  string // Inclusive civil dates in the shop timezone, YYYY-MM-DD.
	Status         domain.SaleStatus
	PageSize       int
	Cursor         string
}

type SalePage struct {
	Items      []SaleOutput
	NextCursor string
	Timezone   string
}

type GetSaleInput struct {
	Scope  SalesScope
	SaleID uuid.UUID
}

type SaleItemOutput struct {
	ID, ProductID                            uuid.UUID
	ProductName, Currency                    string
	Quantity, UnitPriceCents, LineTotalCents int64
}

type SaleCancellationOutput struct {
	ActorID     uuid.UUID
	Reason      string
	Restock     bool
	CancelledAt time.Time
}

type SaleOutput struct {
	ID, ShopID, RecordedByUserID  uuid.UUID
	ProfessionalID, AppointmentID uuid.UUID
	ProfessionalName, Currency    string
	Items                         []SaleItemOutput
	Status                        domain.SaleStatus
	TotalCents, Version           int64
	SoldAt                        time.Time
	Cancellation                  *SaleCancellationOutput
	Replayed                      bool
}
