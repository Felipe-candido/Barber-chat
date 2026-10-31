package application_test

import (
	"errors"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
)

func TestHistoryRetainsOriginalValueButOnlyValidOutcomesContribute(t *testing.T) {
	for _, tc := range []struct {
		status domain.AppointmentStatus
		want   int64
	}{
		{domain.StatusScheduled, 0}, {domain.StatusConfirmed, 0},
		{domain.StatusCompleted, 5000}, {domain.StatusCancelled, 0}, {domain.StatusNoShow, 0},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			row := application.ActivityHistoryRow{Source: domain.ActivityServices, Currency: "BRL", ValueCents: 5000,
				AppointmentStatus: tc.status, AppointmentStatusSource: domain.SourceLegacy}
			value, err := row.RecordedValueCents()
			if err != nil || value != tc.want || row.ValueCents != 5000 {
				t.Fatalf("original history amount changed or wrong contribution: %+v, %d, %v", row, value, err)
			}
		})
	}
	for _, source := range []domain.AppointmentStatusSource{domain.SourceAutomatic, domain.SourceStaff, domain.SourceLegacy} {
		row := application.ActivityHistoryRow{Source: domain.ActivityServices, Currency: "BRL", ValueCents: 5000,
			AppointmentStatus: domain.StatusCompleted, AppointmentStatusSource: source}
		if value, err := row.RecordedValueCents(); err != nil || value != 5000 {
			t.Fatal("stored completed outcomes contribute independently of provenance", value, err)
		}
	}
	row := application.ActivityHistoryRow{Source: domain.ActivityProducts, Currency: "BRL", ValueCents: 9000, SaleStatus: domain.ProductSaleRecorded}
	if value, err := row.RecordedValueCents(); err != nil || value != 9000 {
		t.Fatal(value, err)
	}
	row.SaleStatus = domain.ProductSaleCancelled
	if value, err := row.RecordedValueCents(); err != nil || value != 0 || row.ValueCents != 9000 {
		t.Fatal("cancellation must exclude value, preserving the original amount", value, err)
	}
}

func TestHistoryRejectsAmbiguousOrUnknownMonetaryProjections(t *testing.T) {
	base := application.ActivityHistoryRow{Source: domain.ActivityServices, Currency: "BRL", ValueCents: 5000,
		AppointmentStatus: domain.StatusCompleted, AppointmentStatusSource: domain.SourceAutomatic}
	for _, mutate := range []func(*application.ActivityHistoryRow){
		func(r *application.ActivityHistoryRow) { r.Source = domain.ActivityAll },
		func(r *application.ActivityHistoryRow) { r.AppointmentStatus = "paid" },
		func(r *application.ActivityHistoryRow) { r.AppointmentStatusSource = "" },
		func(r *application.ActivityHistoryRow) { r.SaleStatus = domain.ProductSaleRecorded },
		func(r *application.ActivityHistoryRow) { r.Currency = "brl" },
		func(r *application.ActivityHistoryRow) { r.ValueCents = -1 },
		func(r *application.ActivityHistoryRow) { r.ValueCents = 9_007_199_254_740_992 },
		func(r *application.ActivityHistoryRow) {
			r.Source = domain.ActivityProducts
			r.SaleStatus = domain.ProductSaleRecorded
		},
		func(r *application.ActivityHistoryRow) {
			*r = application.ActivityHistoryRow{Source: domain.ActivityProducts, Currency: "BRL", SaleStatus: "paid"}
		},
	} {
		row := base
		mutate(&row)
		if _, err := row.RecordedValueCents(); !errors.Is(err, application.ErrInconsistentData) {
			t.Fatalf("invalid projection accepted: %+v, %v", row, err)
		}
	}
}

func TestForecastAndAutomaticSubsetDoNotInflateRecordedTotals(t *testing.T) {
	values := application.ActivityValues{Currency: "BRL", ScheduledServiceValueCents: 20000,
		CompletedServiceValueCents: 10000, CompletedAppointmentCount: 2,
		ProductSaleValueCents: 9000, ProductSaleCount: 1, ProductUnitCount: 3}
	if total, err := values.CombinedValueCents(); err != nil || total != 19000 {
		t.Fatal("forecast entered recorded total", total, err)
	}
	counts := application.OutcomeCounts{Scheduled: 4, Completed: 2, CompletedAutomatically: 1, NoShow: 1, Cancelled: 1}
	if err := counts.Validate(); err != nil {
		t.Fatal(err)
	}
	counts.CompletedAutomatically = 3
	if !errors.Is(counts.Validate(), application.ErrInconsistentData) {
		t.Fatal("automatic subset exceeded completed count")
	}
	counts = application.OutcomeCounts{NoShow: -1}
	if !errors.Is(counts.Validate(), application.ErrInconsistentData) {
		t.Fatal("negative outcome count accepted")
	}
}

func TestActivityBucketsRejectPrecisionLossAndImpossibleReceiptCounts(t *testing.T) {
	const max int64 = 9_007_199_254_740_991
	values := application.ActivityValues{Currency: "BRL", CompletedServiceValueCents: max, CompletedAppointmentCount: 1}
	if total, err := values.CombinedValueCents(); err != nil || total != max {
		t.Fatal("exact upper bound rejected", total, err)
	}
	for _, mutate := range []func(*application.ActivityValues){
		func(v *application.ActivityValues) {
			v.ProductSaleValueCents = 1
			v.ProductSaleCount = 1
			v.ProductUnitCount = 1
		},
		func(v *application.ActivityValues) { v.ScheduledServiceValueCents = max + 1 },
		func(v *application.ActivityValues) { v.CompletedAppointmentCount = max + 1 },
		func(v *application.ActivityValues) { v.CompletedAppointmentCount = 0 },
		func(v *application.ActivityValues) { v.ProductSaleCount = 1 },
		func(v *application.ActivityValues) { v.ProductUnitCount = 1 },
	} {
		bad := values
		mutate(&bad)
		if _, err := bad.CombinedValueCents(); !errors.Is(err, application.ErrInconsistentData) {
			t.Fatal("inconsistent amount/count accepted", bad, err)
		}
	}
}
