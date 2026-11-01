package http

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

type periodResponse struct {
	FromDate    string    `json:"from_date"`
	ThroughDate string    `json:"through_date"`
	FromAt      time.Time `json:"from_at"`
	UntilAt     time.Time `json:"until_at"`
	Timezone    string    `json:"timezone"`
}
type countsResponse struct {
	Scheduled              int64 `json:"scheduled"`
	Confirmed              int64 `json:"confirmed"`
	Completed              int64 `json:"completed"`
	CompletedAutomatically int64 `json:"completed_automatically"`
	Cancelled              int64 `json:"cancelled"`
	NoShow                 int64 `json:"no_show"`
}
type valuesResponse struct {
	Currency                   string `json:"currency"`
	ScheduledServiceValueCents int64  `json:"scheduled_service_value_cents"`
	CompletedServiceValueCents int64  `json:"completed_service_value_cents"`
	ProductSaleValueCents      int64  `json:"product_sale_value_cents"`
	CombinedValueCents         int64  `json:"combined_value_cents"`
	CompletedAppointmentCount  int64  `json:"completed_appointment_count"`
	ProductSaleCount           int64  `json:"product_sale_count"`
	ProductUnitCount           int64  `json:"product_unit_count"`
}
type dayResponse struct {
	LocalDate string           `json:"local_date"`
	Counts    countsResponse   `json:"counts"`
	Values    []valuesResponse `json:"values"`
}
type groupResponse struct {
	ID     *uuid.UUID       `json:"id"`
	Label  string           `json:"label"`
	Counts countsResponse   `json:"counts"`
	Values []valuesResponse `json:"values"`
}
type overviewResponse struct {
	Period         periodResponse        `json:"period"`
	Source         domain.ActivitySource `json:"source"`
	ObservedAt     time.Time             `json:"observed_at"`
	Counts         countsResponse        `json:"counts"`
	Values         []valuesResponse      `json:"values"`
	ByDay          []dayResponse         `json:"by_day"`
	ByProfessional []groupResponse       `json:"by_professional"`
	ByService      []groupResponse       `json:"by_service"`
	ByProduct      []groupResponse       `json:"by_product"`
}

func periodBody(p domain.Period) (periodResponse, error) {
	if !p.Valid() {
		return periodResponse{}, application.ErrInconsistentData
	}
	loc, err := time.LoadLocation(p.Timezone())
	if err != nil {
		return periodResponse{}, application.ErrInconsistentData
	}
	return periodResponse{FromDate: p.From().In(loc).Format(time.DateOnly), ThroughDate: p.Until().In(loc).AddDate(0, 0, -1).Format(time.DateOnly),
		FromAt: p.From().UTC(), UntilAt: p.Until().UTC(), Timezone: p.Timezone()}, nil
}
func countsBody(c application.OutcomeCounts) countsResponse {
	return countsResponse{c.Scheduled, c.Confirmed, c.Completed, c.CompletedAutomatically, c.Cancelled, c.NoShow}
}
func valuesBody(values []application.ActivityValues) ([]valuesResponse, error) {
	out := make([]valuesResponse, 0, len(values))
	for _, v := range values {
		total, err := v.CombinedValueCents()
		if err != nil {
			return nil, err
		}
		out = append(out, valuesResponse{v.Currency, v.ScheduledServiceValueCents, v.CompletedServiceValueCents, v.ProductSaleValueCents, total, v.CompletedAppointmentCount, v.ProductSaleCount, v.ProductUnitCount})
	}
	return out, nil
}
func groupsBody(groups []application.ActivityGroup) ([]groupResponse, error) {
	out := make([]groupResponse, 0, len(groups))
	for _, g := range groups {
		values, err := valuesBody(g.Values)
		if err != nil {
			return nil, err
		}
		out = append(out, groupResponse{optionalID(g.ID), g.Label, countsBody(g.Counts), values})
	}
	return out, nil
}
func overviewBody(o application.ActivityOverview) (overviewResponse, error) {
	p, err := periodBody(o.Period)
	if err != nil {
		return overviewResponse{}, err
	}
	values, err := valuesBody(o.Values)
	if err != nil {
		return overviewResponse{}, err
	}
	out := overviewResponse{Period: p, Source: o.Source, ObservedAt: o.ObservedAt.UTC(), Counts: countsBody(o.Counts), Values: values, ByDay: make([]dayResponse, 0, len(o.ByDay))}
	for _, day := range o.ByDay {
		values, err := valuesBody(day.Values)
		if err != nil {
			return overviewResponse{}, err
		}
		out.ByDay = append(out.ByDay, dayResponse{day.LocalDate, countsBody(day.Counts), values})
	}
	if out.ByProfessional, err = groupsBody(o.ByProfessional); err != nil {
		return overviewResponse{}, err
	}
	if out.ByService, err = groupsBody(o.ByService); err != nil {
		return overviewResponse{}, err
	}
	if out.ByProduct, err = groupsBody(o.ByProduct); err != nil {
		return overviewResponse{}, err
	}
	return out, nil
}

type historyItemResponse struct {
	Source                  domain.ActivitySource `json:"source"`
	RecordID                uuid.UUID             `json:"record_id"`
	ItemID                  *uuid.UUID            `json:"item_id"`
	ServiceID               *uuid.UUID            `json:"service_id"`
	ProductID               *uuid.UUID            `json:"product_id"`
	ProfessionalID          *uuid.UUID            `json:"professional_id"`
	Label                   string                `json:"label"`
	ProfessionalName        string                `json:"professional_name"`
	Currency                string                `json:"currency"`
	AppointmentStatus       *string               `json:"appointment_status"`
	AppointmentStatusSource *string               `json:"appointment_status_source"`
	SaleStatus              *string               `json:"sale_status"`
	CustomerName            string                `json:"customer_name"`
	CustomerPhone           string                `json:"customer_phone"`
	OccurredAt              time.Time             `json:"occurred_at"`
	Quantity                int64                 `json:"quantity"`
	UnitPriceCents          int64                 `json:"unit_price_cents"`
	ValueCents              int64                 `json:"value_cents"`
	RecordedValueCents      int64                 `json:"recorded_value_cents"`
}
type historyResponse struct {
	Period     periodResponse        `json:"period"`
	Source     domain.ActivitySource `json:"source"`
	ObservedAt time.Time             `json:"observed_at"`
	Items      []historyItemResponse `json:"items"`
	NextCursor string                `json:"next_cursor"`
}

func historyBody(o application.ActivityHistoryPage) (historyResponse, error) {
	p, err := periodBody(o.Period)
	if err != nil {
		return historyResponse{}, err
	}
	cursor, err := encodeCursor(o.Next)
	if err != nil {
		return historyResponse{}, err
	}
	out := historyResponse{Period: p, Source: o.Source, ObservedAt: o.ObservedAt.UTC(), Items: make([]historyItemResponse, 0, len(o.Rows)), NextCursor: cursor}
	for _, r := range o.Rows {
		value, err := r.RecordedValueCents()
		if err != nil {
			return historyResponse{}, err
		}
		out.Items = append(out.Items, historyItemResponse{Source: r.Source, RecordID: r.RecordID, ItemID: optionalID(r.ItemID), ServiceID: optionalID(r.ServiceID), ProductID: optionalID(r.ProductID),
			ProfessionalID: optionalID(r.ProfessionalID), Label: r.Label, ProfessionalName: r.ProfessionalName, Currency: r.Currency,
			AppointmentStatus: optionalText(string(r.AppointmentStatus)), AppointmentStatusSource: optionalText(string(r.AppointmentStatusSource)), SaleStatus: optionalText(string(r.SaleStatus)),
			CustomerName: r.CustomerName, CustomerPhone: r.CustomerPhone, OccurredAt: r.OccurredAt.UTC(), Quantity: r.Quantity, UnitPriceCents: r.UnitPriceCents, ValueCents: r.ValueCents, RecordedValueCents: value})
	}
	return out, nil
}
func optionalID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
