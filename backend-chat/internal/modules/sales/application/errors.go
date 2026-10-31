package application

import "errors"

var (
	ErrInvalidSaleCommandKey      = errors.New("invalid sale idempotency key")
	ErrSalesAccessDenied          = errors.New("sales access denied")
	ErrInconsistentSalesData      = errors.New("inconsistent sales data")
	ErrInvalidSalesInput          = errors.New("invalid sales input")
	ErrSaleNotFound               = errors.New("sale not found in the authorized scope")
	ErrSaleProductUnavailable     = errors.New("product is inactive or unavailable")
	ErrSalePriceChanged           = errors.New("reviewed product price or version changed")
	ErrSaleStockUnavailable       = errors.New("insufficient sale stock")
	ErrSaleStockLimitExceeded     = errors.New("stock return exceeds quantity limit")
	ErrInvalidSalesQuery          = errors.New("invalid sales filters or cursor")
	ErrSaleAppointmentUnavailable = errors.New("appointment cannot be linked to this sale")
	ErrSaleSubscriptionRequired   = errors.New("subscription coverage is required for a new sale")
	ErrSaleCommandConflict        = errors.New("sale command key was used with another input")
	ErrSaleCommandPending         = errors.New("sale command is still pending")
)
