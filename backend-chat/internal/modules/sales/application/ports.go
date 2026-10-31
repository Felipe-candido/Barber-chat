package application

import (
	"context"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

type SalesAccess interface {
	// Preliminary projection only; current access must be locked/rechecked later.
	ReadSalesAccess(context.Context, SalesScope) (SalesAuthority, error)
	// Current active user/shop/membership, locked on the same command transaction.
	RequireSalesAccess(context.Context, SalesScope) (SalesAuthority, error)
}

type SaleEligibility interface {
	// New writes only; authorized replay and existing-sale management bypass this.
	RequireNewSaleCoverage(context.Context, uuid.UUID, time.Time) error
}

type SaleCatalogReader interface {
	// Lock products AND stock rows in stable product-ID order. Return trusted
	// snapshots of this shop only. Missing/inactive rows cannot be sold.
	LockSaleProducts(ctx context.Context, shopID uuid.UUID, productIDs []uuid.UUID) ([]ProductForSale, error)
	FindSaleProfessional(ctx context.Context, shopID, professionalID uuid.UUID) (SaleProfessional, bool, error)
}

type SaleAppointmentReader interface {
	ReadSaleAppointment(ctx context.Context, shopID, appointmentID uuid.UUID) (AppointmentForSale, bool, error)
	// Follow booking's shared professional/appointment lock order.
	LockSaleAppointment(ctx context.Context, shopID, appointmentID uuid.UUID) (AppointmentForSale, bool, error)
}

type SaleStockRecorder interface {
	// Append source-unique catalog movements on this same transaction. The SQL
	// trigger applies quantities; no network call or second quantity update.
	RecordSaleStock(context.Context, SaleStockCommand) error
}

type SaleStockReturner interface {
	// Copy original immutable withdrawals, including inactive products.
	// Each withdrawal may have at most one full reversal.
	RestockCancelledSale(context.Context, SaleStockCommand) error
}

type SalesCalendar interface {
	ShopTimezone(context.Context, uuid.UUID) (string, error)
}

type SaleRecorder interface {
	InsertSale(context.Context, domain.Sale) error
	FindRecordedSale(ctx context.Context, shopID, saleID uuid.UUID) (domain.Sale, bool, error)
}

type SaleStore interface {
	SaleRecorder
	LockSale(ctx context.Context, shopID, saleID uuid.UUID) (domain.Sale, bool, error)
	// Save cancellation only, with expected version; never rewrite price/items.
	SaveSaleCancellation(context.Context, domain.Sale, int64) (bool, error)
}

type SaleReader interface {
	ListSales(context.Context, SaleListQuery) ([]domain.Sale, error)
	FindSale(ctx context.Context, shopID, saleID, professionalID uuid.UUID) (domain.Sale, bool, error)
}

type SaleCommandStore interface {
	// Bind to shop + actor + operation + digest, compare the fingerprint and
	// claim exclusively. completed=false means this transaction owns a new claim;
	// pending/conflicting commands return errors. Completed replay returns sale ID.
	ClaimSaleCommand(context.Context, SalesScope, SaleCommandKey) (saleID uuid.UUID, completed bool, err error)
	CompleteSaleCommand(context.Context, SalesScope, SaleCommandKey, uuid.UUID) error
}

type SalesTx struct {
	Access       SalesAccess
	Eligibility  SaleEligibility
	Catalog      SaleCatalogReader
	Appointments SaleAppointmentReader
	Stock        SaleStockRecorder
	Sales        SaleRecorder
	Commands     SaleCommandStore
	Cancellation SaleStore
	Returns      SaleStockReturner
	Reader       SaleReader
	Calendar     SalesCalendar
}

type SalesTransactor interface {
	WithinSalesTx(context.Context, func(context.Context, SalesTx) error) error
}
