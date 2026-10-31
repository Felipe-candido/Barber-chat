package postgres

import (
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/google/uuid"
)

// SQL has already aggregated the bounded period. This mapping only assembles
// buckets; it never loads every appointment/item into API memory to total them.
func mapOverview(in application.ActivityQuery, observedAt time.Time, rows []db.ReadReportActivityBucketsRow) (application.ActivityOverview, error) {
	out := application.ActivityOverview{Period: in.Period, Source: in.Source, ObservedAt: observedAt,
		Values: []application.ActivityValues{}, ByDay: []application.DailyActivity{}, ByProfessional: []application.ActivityGroup{},
		ByService: []application.ActivityGroup{}, ByProduct: []application.ActivityGroup{}}
	days := map[string]int{}
	groups := map[string]map[uuid.UUID]int{"professional": {}, "service": {}, "product": {}}
	for _, r := range rows {
		counts := application.OutcomeCounts{Scheduled: r.Scheduled, Confirmed: r.Confirmed, Completed: r.Completed,
			CompletedAutomatically: r.CompletedAutomatically, Cancelled: r.Cancelled, NoShow: r.NoShow}
		values := application.ActivityValues{Currency: r.Currency, ScheduledServiceValueCents: r.ScheduledValue,
			CompletedServiceValueCents: r.ServiceValue, ProductSaleValueCents: r.ProductValue, CompletedAppointmentCount: r.Completed,
			ProductSaleCount: r.SaleCount, ProductUnitCount: r.ProductUnits}
		if _, err := values.CombinedValueCents(); err != nil {
			return application.ActivityOverview{}, err
		}
		if err := counts.Validate(); err != nil {
			return application.ActivityOverview{}, err
		}
		switch r.Bucket {
		case "total":
			if err := addCounts(&out.Counts, counts); err != nil {
				return application.ActivityOverview{}, err
			}
			out.Values = append(out.Values, values)
		case "day":
			day, err := time.Parse(time.DateOnly, r.GroupKey)
			if err != nil || day.Format(time.DateOnly) != r.GroupKey {
				return application.ActivityOverview{}, application.ErrInconsistentData
			}
			i, ok := days[r.GroupKey]
			if !ok {
				i = len(out.ByDay)
				days[r.GroupKey] = i
				out.ByDay = append(out.ByDay, application.DailyActivity{LocalDate: r.GroupKey, Values: []application.ActivityValues{}})
			}
			if err := addCounts(&out.ByDay[i].Counts, counts); err != nil {
				return application.ActivityOverview{}, err
			}
			out.ByDay[i].Values = append(out.ByDay[i].Values, values)
		case "professional", "service", "product":
			if r.Bucket != "professional" && r.GroupID == uuid.Nil {
				return application.ActivityOverview{}, application.ErrInconsistentData
			}
			var target *[]application.ActivityGroup
			switch r.Bucket {
			case "professional":
				target = &out.ByProfessional
			case "service":
				target = &out.ByService
			case "product":
				target = &out.ByProduct
			}
			i, ok := groups[r.Bucket][r.GroupID]
			if !ok {
				i = len(*target)
				groups[r.Bucket][r.GroupID] = i
				*target = append(*target, application.ActivityGroup{ID: r.GroupID, Label: r.Label, Values: []application.ActivityValues{}})
			}
			group := &(*target)[i]
			if err := addCounts(&group.Counts, counts); err != nil {
				return application.ActivityOverview{}, err
			}
			group.Values = append(group.Values, values)
		default:
			return application.ActivityOverview{}, application.ErrInconsistentData
		}
	}
	return out, nil
}

func addCounts(to *application.OutcomeCounts, more application.OutcomeCounts) error {
	// Each operand has been bounded to 2^53-1; adding two cannot overflow int64.
	to.Scheduled += more.Scheduled
	to.Confirmed += more.Confirmed
	to.Completed += more.Completed
	to.CompletedAutomatically += more.CompletedAutomatically
	to.Cancelled += more.Cancelled
	to.NoShow += more.NoShow
	return to.Validate()
}
