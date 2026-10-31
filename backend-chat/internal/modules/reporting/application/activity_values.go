package application

import "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"

// Bound integer amounts and counts to the exact JSON number range used by the
// frontend. Reject overflow/inconsistent projections rather than rounding money.
const maxExactInteger int64 = 9_007_199_254_740_991

// A currency bucket. Counts of appointments, receipts and units are separate
// measures. ProductSaleCount is DISTINCT recorded sale IDs matching item filters;
// value and units sum only matching items, never the full filtered sale header.
type ActivityValues struct {
	Currency                                                      string
	ScheduledServiceValueCents                                    int64 // Scheduled + confirmed; forecast only.
	CompletedServiceValueCents, ProductSaleValueCents             int64
	CompletedAppointmentCount, ProductSaleCount, ProductUnitCount int64
}

// CombinedValueCents measures recorded activity, not cash received or profit.
// Forecasts, cancelled sales, cancelled appointments and no-shows are excluded.
func (v ActivityValues) CombinedValueCents() (int64, error) {
	if !validAmounts(v.Currency, v.ScheduledServiceValueCents, v.CompletedServiceValueCents, v.ProductSaleValueCents,
		v.CompletedAppointmentCount, v.ProductSaleCount, v.ProductUnitCount) ||
		v.CompletedServiceValueCents > maxExactInteger-v.ProductSaleValueCents ||
		(v.CompletedAppointmentCount == 0 && v.CompletedServiceValueCents != 0) ||
		(v.ProductSaleCount == 0 && (v.ProductSaleValueCents != 0 || v.ProductUnitCount != 0)) ||
		(v.ProductSaleCount > 0 && (v.ProductSaleValueCents == 0 || v.ProductUnitCount < v.ProductSaleCount)) {
		return 0, ErrInconsistentData
	}
	return v.CompletedServiceValueCents + v.ProductSaleValueCents, nil
}

// RecordedValueCents applies outcome rules to an original history amount without
// mutating it. This validates monetary/state projections; it does not authorize
// access or validate resource identities. The reader/use case must do both.
func (r ActivityHistoryRow) RecordedValueCents() (int64, error) {
	if !validAmounts(r.Currency, r.ValueCents) {
		return 0, ErrInconsistentData
	}
	switch r.Source {
	case domain.ActivityServices:
		if !r.AppointmentStatus.Valid() || !r.AppointmentStatusSource.Valid() || r.SaleStatus != "" {
			return 0, ErrInconsistentData
		}
		if r.AppointmentStatus == domain.StatusCompleted {
			return r.ValueCents, nil
		}
	case domain.ActivityProducts:
		if !r.SaleStatus.Valid() || r.AppointmentStatus != "" || r.AppointmentStatusSource != "" {
			return 0, ErrInconsistentData
		}
		if r.SaleStatus == domain.ProductSaleRecorded {
			return r.ValueCents, nil
		}
	default:
		return 0, ErrInconsistentData
	}
	return 0, nil
}

func (c OutcomeCounts) Validate() error {
	for _, count := range []int64{c.Scheduled, c.Confirmed, c.Completed, c.CompletedAutomatically, c.Cancelled, c.NoShow} {
		if count < 0 || count > maxExactInteger {
			return ErrInconsistentData
		}
	}
	if c.CompletedAutomatically > c.Completed {
		return ErrInconsistentData
	}
	return nil
}

func validAmounts(currency string, amounts ...int64) bool {
	if len(currency) != 3 {
		return false
	}
	for _, letter := range currency {
		if letter < 'A' || letter > 'Z' {
			return false
		}
	}
	for _, amount := range amounts {
		if amount < 0 || amount > maxExactInteger {
			return false
		}
	}
	return true
}
