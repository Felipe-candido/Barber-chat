package application

import (
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

// ValidateFor checks a projection against an already-authorized query. Both
// adapters and use cases apply the same invariant; this never grants access.
func (r ActivityHistoryRow) ValidateFor(q ActivityHistoryQuery) error {
	if _, err := r.RecordedValueCents(); err != nil {
		return err
	}
	if r.ShopID != q.ShopID || !validCursor(q, activityPosition(r)) ||
		(q.Filters.ProfessionalID != uuid.Nil && r.ProfessionalID != q.Filters.ProfessionalID) ||
		(q.Filters.PhoneE164 != "" && r.CustomerPhone != q.Filters.PhoneE164) ||
		(q.Filters.CustomerName != "" && strings.TrimSpace(r.CustomerName) == "") ||
		strings.TrimSpace(r.Label) == "" || r.Quantity <= 0 || r.UnitPriceCents < 0 ||
		r.ValueCents%r.Quantity != 0 || r.ValueCents/r.Quantity != r.UnitPriceCents {
		return ErrInconsistentData
	}
	if r.Source == domain.ActivityServices {
		if r.ItemID != uuid.Nil || r.ProductID != uuid.Nil || r.ServiceID == uuid.Nil || r.ProfessionalID == uuid.Nil || r.Quantity != 1 ||
			strings.TrimSpace(r.ProfessionalName) == "" ||
			(q.Filters.ServiceID != uuid.Nil && r.ServiceID != q.Filters.ServiceID) ||
			(q.AppointmentStatus != "" && r.AppointmentStatus != q.AppointmentStatus) {
			return ErrInconsistentData
		}
	} else if r.ServiceID != uuid.Nil || r.ProductID == uuid.Nil || r.ItemID == uuid.Nil || r.UnitPriceCents == 0 ||
		(q.Filters.ProductID != uuid.Nil && r.ProductID != q.Filters.ProductID) ||
		(q.SaleStatus != "" && r.SaleStatus != q.SaleStatus) ||
		(r.ProfessionalID != uuid.Nil && strings.TrimSpace(r.ProfessionalName) == "") {
		return ErrInconsistentData
	}
	return nil
}

func validateOverview(q ActivityQuery, observedAt time.Time, out ActivityOverview) error {
	if !out.Period.From().Equal(q.Period.From()) || !out.Period.Until().Equal(q.Period.Until()) || out.Period.Timezone() != q.Period.Timezone() ||
		out.Source != q.Source || !out.ObservedAt.Equal(observedAt) {
		return ErrInconsistentData
	}
	if err := validateMeasures(q.Source, out.Counts, out.Values); err != nil {
		return err
	}
	if (q.Source == domain.ActivityServices && len(out.ByProduct) != 0) || (q.Source == domain.ActivityProducts && len(out.ByService) != 0) {
		return ErrInconsistentData
	}
	loc, _ := time.LoadLocation(q.Period.Timezone()) // prepareActivity already validated it.
	first, until := q.Period.From().In(loc).Format(time.DateOnly), q.Period.Until().In(loc).Format(time.DateOnly)
	previous := ""
	for _, day := range out.ByDay {
		at, err := time.Parse(time.DateOnly, day.LocalDate)
		if err != nil || at.Format(time.DateOnly) != day.LocalDate || day.LocalDate < first || day.LocalDate >= until || day.LocalDate <= previous {
			return ErrInconsistentData
		}
		if err := validateMeasures(q.Source, day.Counts, day.Values); err != nil {
			return err
		}
		previous = day.LocalDate
	}
	for _, dimension := range []struct {
		groups       []ActivityGroup
		source       domain.ActivitySource
		filter       uuid.UUID
		professional bool
	}{
		{out.ByProfessional, q.Source, q.Filters.ProfessionalID, true},
		{out.ByService, domain.ActivityServices, q.Filters.ServiceID, false},
		{out.ByProduct, domain.ActivityProducts, q.Filters.ProductID, false},
	} {
		seen := map[uuid.UUID]bool{}
		for _, group := range dimension.groups {
			if seen[group.ID] || (!dimension.professional && group.ID == uuid.Nil) ||
				(dimension.filter != uuid.Nil && group.ID != dimension.filter) ||
				(group.ID != uuid.Nil && strings.TrimSpace(group.Label) == "") {
				return ErrInconsistentData
			}
			if err := validateMeasures(dimension.source, group.Counts, group.Values); err != nil {
				return err
			}
			seen[group.ID] = true
		}
	}
	return nil
}

func validateMeasures(source domain.ActivitySource, counts OutcomeCounts, values []ActivityValues) error {
	if err := counts.Validate(); err != nil {
		return err
	}
	if source == domain.ActivityProducts && counts != (OutcomeCounts{}) {
		return ErrInconsistentData
	}
	if len(values) == 0 && counts != (OutcomeCounts{}) {
		return ErrInconsistentData
	}
	seen := map[string]bool{}
	var completed int64
	for _, v := range values {
		if _, err := v.CombinedValueCents(); err != nil {
			return err
		}
		if seen[v.Currency] || v.CompletedAppointmentCount > maxExactInteger-completed ||
			(source == domain.ActivityProducts && (v.CompletedAppointmentCount != 0 || v.CompletedServiceValueCents != 0 || v.ScheduledServiceValueCents != 0)) ||
			(source == domain.ActivityServices && (v.ProductSaleCount != 0 || v.ProductUnitCount != 0 || v.ProductSaleValueCents != 0)) {
			return ErrInconsistentData
		}
		seen[v.Currency] = true
		completed += v.CompletedAppointmentCount
	}
	if completed != counts.Completed {
		return ErrInconsistentData
	}
	return nil
}

func normalizeOverviewCollections(out *ActivityOverview) {
	if out.Values == nil {
		out.Values = []ActivityValues{}
	}
	if out.ByDay == nil {
		out.ByDay = []DailyActivity{}
	}
	if out.ByProfessional == nil {
		out.ByProfessional = []ActivityGroup{}
	}
	if out.ByService == nil {
		out.ByService = []ActivityGroup{}
	}
	if out.ByProduct == nil {
		out.ByProduct = []ActivityGroup{}
	}
}
