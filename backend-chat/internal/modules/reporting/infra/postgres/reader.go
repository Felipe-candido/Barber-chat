package postgres

import (
	"context"
	"strings"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reader receives queries bound to the shared read transaction. Its query scope
// must already be resolved from ActivityAccess, never decoded directly from HTTP.
type Reader struct {
	q          *db.Queries
	observedAt time.Time
}

func NewReader(q *db.Queries, observedAt time.Time) *Reader {
	return &Reader{q: q, observedAt: observedAt.UTC()}
}

var _ application.ActivityOverviewReader = (*Reader)(nil)
var _ application.ActivityHistoryReader = (*Reader)(nil)

func validQuery(q application.ActivityQuery) bool {
	return q.ShopID != uuid.Nil && q.Period.Valid() &&
		q.Source.AllowsFilters(q.Filters.ServiceID != uuid.Nil, q.Filters.ProductID != uuid.Nil)
}

func reportTime(at time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: at.UTC(), Valid: true}
}

func namePattern(name string) string {
	if name == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(name) + "%"
}

func (r *Reader) ListActivityHistory(ctx context.Context, in application.ActivityHistoryQuery) ([]application.ActivityHistoryRow, error) {
	if !validQuery(in.ActivityQuery) || in.Limit < 1 || in.Limit > application.MaxHistoryPageSize+1 ||
		(in.AppointmentStatus != "" && (!in.AppointmentStatus.Valid() || in.Source == domain.ActivityProducts)) ||
		(in.SaleStatus != "" && (!in.SaleStatus.Valid() || in.Source == domain.ActivityServices)) {
		return nil, application.ErrInvalidQuery
	}
	p := db.ListReportActivityParams{ShopID: in.ShopID, Source: string(in.Source), Timezone: in.Period.Timezone(),
		FromAt: reportTime(in.Period.From()), UntilAt: reportTime(in.Period.Until()),
		ProfessionalID: in.Filters.ProfessionalID, ServiceID: in.Filters.ServiceID, ProductID: in.Filters.ProductID,
		NamePattern: namePattern(in.Filters.CustomerName), Phone: in.Filters.PhoneE164,
		AppointmentStatus: string(in.AppointmentStatus), SaleStatus: string(in.SaleStatus),
		PageLimit: int32(in.Limit), AfterAt: reportTime(in.Period.From())}
	if c := in.After; c != nil {
		if !c.Source.IsRowSource() || c.RecordID == uuid.Nil || c.OccurredAt.Before(in.Period.From()) || !c.OccurredAt.Before(in.Period.Until()) ||
			(in.Source != domain.ActivityAll && in.Source != c.Source) ||
			(c.Source == domain.ActivityServices && c.ItemID != uuid.Nil) || (c.Source == domain.ActivityProducts && c.ItemID == uuid.Nil) {
			return nil, application.ErrInvalidQuery
		}
		p.HasCursor = true
		p.AfterAt = reportTime(c.OccurredAt)
		p.AfterSource = string(c.Source)
		p.AfterID = c.RecordID
		p.AfterItemID = c.ItemID
	}
	rows, err := r.q.ListReportActivity(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]application.ActivityHistoryRow, 0, len(rows))
	for _, v := range rows {
		if !v.OccurredAt.Valid || v.OccurredAt.InfinityModifier != pgtype.Finite {
			return nil, application.ErrInconsistentData
		}
		row := application.ActivityHistoryRow{ShopID: v.ShopID, Source: domain.ActivitySource(v.Source), RecordID: v.RecordID, ItemID: v.ItemID,
			ServiceID: v.ServiceID, ProductID: v.ProductID, ProfessionalID: v.ProfessionalID, Label: v.Label, ProfessionalName: v.ProfessionalName, Currency: v.Currency,
			AppointmentStatus: domain.AppointmentStatus(v.AppointmentStatus), AppointmentStatusSource: domain.AppointmentStatusSource(v.StatusSource),
			SaleStatus: domain.ProductSaleStatus(v.SaleStatus), CustomerName: v.CustomerName, CustomerPhone: v.CustomerPhone,
			OccurredAt: v.OccurredAt.Time.UTC(), Quantity: v.Quantity, UnitPriceCents: v.UnitPriceCents, ValueCents: v.ValueCents}
		if err := validateHistoryRow(in.ActivityQuery, row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func validateHistoryRow(q application.ActivityQuery, r application.ActivityHistoryRow) error {
	if _, err := r.RecordedValueCents(); err != nil {
		return err
	}
	if r.ShopID != q.ShopID || r.RecordID == uuid.Nil || strings.TrimSpace(r.Label) == "" || r.Currency != "BRL" ||
		r.OccurredAt.Before(q.Period.From()) || !r.OccurredAt.Before(q.Period.Until()) ||
		(q.Filters.ProfessionalID != uuid.Nil && r.ProfessionalID != q.Filters.ProfessionalID) || r.Quantity <= 0 || r.UnitPriceCents < 0 ||
		r.ValueCents%r.Quantity != 0 || r.ValueCents/r.Quantity != r.UnitPriceCents {
		return application.ErrInconsistentData
	}
	if r.Source == domain.ActivityServices {
		if r.ItemID != uuid.Nil || r.ServiceID == uuid.Nil || r.ProductID != uuid.Nil || r.ProfessionalID == uuid.Nil || r.Quantity != 1 {
			return application.ErrInconsistentData
		}
	} else if r.ItemID == uuid.Nil || r.ProductID == uuid.Nil || r.ServiceID != uuid.Nil || r.UnitPriceCents == 0 {
		return application.ErrInconsistentData
	}
	return nil
}

func (r *Reader) ReadActivityOverview(ctx context.Context, in application.ActivityQuery) (application.ActivityOverview, error) {
	if !validQuery(in) {
		return application.ActivityOverview{}, application.ErrInvalidQuery
	}
	if r.observedAt.IsZero() {
		return application.ActivityOverview{}, application.ErrInconsistentData
	}
	rows, err := r.q.ReadReportActivityBuckets(ctx, db.ReadReportActivityBucketsParams{ShopID: in.ShopID, Source: string(in.Source), Timezone: in.Period.Timezone(),
		FromAt: reportTime(in.Period.From()), UntilAt: reportTime(in.Period.Until()), ProfessionalID: in.Filters.ProfessionalID,
		ServiceID: in.Filters.ServiceID, ProductID: in.Filters.ProductID, NamePattern: namePattern(in.Filters.CustomerName), Phone: in.Filters.PhoneE164})
	if err != nil {
		return application.ActivityOverview{}, err
	}
	return mapOverview(in, r.observedAt, rows)
}
