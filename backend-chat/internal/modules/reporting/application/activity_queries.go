package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

// ActivityQueries coordinates authorized read models, without owning booking,
// sales or customer state. Normalization is local; all database reads share tx.
type ActivityQueries struct {
	tx     ActivityReadTransactor
	phones PhoneNormalizer
}

func NewActivityQueries(tx ActivityReadTransactor, phones PhoneNormalizer) *ActivityQueries {
	return &ActivityQueries{tx: tx, phones: phones}
}

func (uc *ActivityQueries) Overview(ctx context.Context, raw ActivityReportInput) (ActivityOverview, error) {
	if err := ctx.Err(); err != nil {
		return ActivityOverview{}, err
	}
	input, filters, err := uc.normalize(raw)
	if err != nil {
		return ActivityOverview{}, err
	}
	if uc.tx == nil {
		return ActivityOverview{}, ErrInconsistentData
	}
	var out ActivityOverview
	err = uc.tx.WithinActivityRead(ctx, func(ctx context.Context, tx ActivityReadTx) error {
		q, _, err := prepareActivity(ctx, tx, input, filters)
		if err != nil {
			return err
		}
		if tx.Overview == nil {
			return ErrInconsistentData
		}
		result, err := tx.Overview.ReadActivityOverview(ctx, q)
		if err != nil {
			return fmt.Errorf("read activity overview: %w", err)
		}
		if err := validateOverview(q, tx.ObservedAt, result); err != nil {
			return err
		}
		normalizeOverviewCollections(&result)
		out = result
		return nil
	})
	if err != nil {
		return ActivityOverview{}, err
	}
	return out, nil
}

func (uc *ActivityQueries) History(ctx context.Context, raw ActivityHistoryInput) (ActivityHistoryPage, error) {
	if err := ctx.Err(); err != nil {
		return ActivityHistoryPage{}, err
	}
	input, filters, err := uc.normalize(raw.ActivityReportInput)
	if err != nil {
		return ActivityHistoryPage{}, err
	}
	pageSize := raw.PageSize
	if pageSize == 0 {
		pageSize = DefaultHistoryPageSize
	}
	if pageSize < 1 || pageSize > MaxHistoryPageSize ||
		(raw.AppointmentStatus != "" && (!raw.AppointmentStatus.Valid() || input.Source == domain.ActivityProducts)) ||
		(raw.SaleStatus != "" && (!raw.SaleStatus.Valid() || input.Source == domain.ActivityServices)) {
		return ActivityHistoryPage{}, ErrInvalidQuery
	}
	var after *ActivityCursor
	if raw.After != nil {
		copy := *raw.After
		copy.OccurredAt = copy.OccurredAt.UTC()
		after = &copy
	}
	if uc.tx == nil {
		return ActivityHistoryPage{}, ErrInconsistentData
	}
	var out ActivityHistoryPage
	err = uc.tx.WithinActivityRead(ctx, func(ctx context.Context, tx ActivityReadTx) error {
		q, audience, err := prepareActivity(ctx, tx, input, filters)
		if err != nil {
			return err
		}
		if tx.History == nil {
			return ErrInconsistentData
		}
		query := ActivityHistoryQuery{ActivityQuery: q, AppointmentStatus: raw.AppointmentStatus, SaleStatus: raw.SaleStatus, Limit: pageSize + 1}
		binding, err := activityFingerprint(input.ActorID, audience, query)
		if err != nil {
			return err
		}
		if after != nil {
			if !validCursor(query, *after) || after.FilterFingerprint != binding {
				return ErrInvalidQuery
			}
			query.After = after
		}
		rows, err := tx.History.ListActivityHistory(ctx, query)
		if err != nil {
			return fmt.Errorf("read activity history: %w", err)
		}
		if len(rows) > query.Limit {
			return ErrInconsistentData
		}
		previous := after
		for _, row := range rows {
			if err := row.ValidateFor(query); err != nil {
				return err
			}
			position := activityPosition(row)
			if previous != nil && comparePositions(position, *previous) >= 0 {
				return ErrInconsistentData
			}
			previous = &position
		}
		out = ActivityHistoryPage{Period: q.Period, Source: q.Source, ObservedAt: tx.ObservedAt, Rows: make([]ActivityHistoryRow, 0, pageSize)}
		if len(rows) > pageSize {
			rows = rows[:pageSize]
			last := activityPosition(rows[len(rows)-1])
			last.FilterFingerprint = binding
			out.Next = &last
		}
		out.Rows = append(out.Rows, rows...)
		return nil
	})
	if err != nil {
		return ActivityHistoryPage{}, err
	}
	return out, nil
}

func (uc *ActivityQueries) normalize(input ActivityReportInput) (ActivityReportInput, ActivityFilters, error) {
	if input.ActorID == uuid.Nil || input.ShopID == uuid.Nil {
		return input, ActivityFilters{}, ErrActivityAccessDenied
	}
	if input.Source == "" {
		input.Source = domain.ActivityAll
	}
	if !input.Source.AllowsFilters(input.Filters.ServiceID != uuid.Nil, input.Filters.ProductID != uuid.Nil) {
		return input, ActivityFilters{}, ErrInvalidQuery
	}
	name := input.Filters.CustomerName
	if len(name) > 4*201 || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return input, ActivityFilters{}, ErrInvalidQuery
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 201 {
		return input, ActivityFilters{}, ErrInvalidQuery
	}
	phone := strings.TrimSpace(input.Filters.Phone)
	country := strings.ToUpper(strings.TrimSpace(input.Filters.PhoneCountry))
	if len(phone) > 40 || (country != "" && (len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z')) ||
		(phone == "" && country != "") {
		return input, ActivityFilters{}, ErrInvalidQuery
	}
	filters := ActivityFilters{ReportFilters: ReportFilters{CustomerName: name, ProfessionalID: input.Filters.ProfessionalID, ServiceID: input.Filters.ServiceID}, ProductID: input.Filters.ProductID}
	if phone != "" {
		if uc.phones == nil {
			return input, ActivityFilters{}, ErrInconsistentData
		}
		canonical, err := uc.phones.NormalizeReportPhone(phone, country)
		if errors.Is(err, ErrInvalidQuery) {
			return input, ActivityFilters{}, ErrInvalidQuery
		}
		if err != nil {
			return input, ActivityFilters{}, fmt.Errorf("normalize report phone: %w", err)
		}
		if !validReportPhone(canonical) {
			return input, ActivityFilters{}, ErrInconsistentData
		}
		filters.PhoneE164 = canonical
	}
	return input, filters, nil
}

func validReportPhone(phone string) bool {
	if len(phone) < 3 || len(phone) > 16 || phone[0] != '+' || phone[1] < '1' || phone[1] > '9' {
		return false
	}
	for _, digit := range phone[2:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func prepareActivity(ctx context.Context, tx ActivityReadTx, input ActivityReportInput, filters ActivityFilters) (ActivityQuery, ActivityAudience, error) {
	if tx.Access == nil || tx.Shops == nil || tx.ObservedAt.IsZero() {
		return ActivityQuery{}, ActivityAudience{}, ErrInconsistentData
	}
	audience, err := tx.Access.RequireActivityAccess(ctx, input.ActorID, input.ShopID)
	if err != nil {
		return ActivityQuery{}, ActivityAudience{}, fmt.Errorf("read activity access: %w", err)
	}
	professional, err := audience.ProfessionalFor(input.ActorID, input.ShopID, filters.ProfessionalID)
	if err != nil {
		return ActivityQuery{}, ActivityAudience{}, err
	}
	shop, found, err := tx.Shops.FindActiveReportShop(ctx, input.ShopID)
	if err != nil {
		return ActivityQuery{}, ActivityAudience{}, fmt.Errorf("read report shop: %w", err)
	}
	if !found {
		return ActivityQuery{}, ActivityAudience{}, ErrShopNotFound
	}
	if shop.ID != input.ShopID || shop.Timezone == "" || shop.Timezone == "Local" {
		return ActivityQuery{}, ActivityAudience{}, ErrInconsistentData
	}
	if _, err := time.LoadLocation(shop.Timezone); err != nil {
		return ActivityQuery{}, ActivityAudience{}, ErrInconsistentData
	}
	period, err := domain.NewPeriod(input.FromDate, input.ThroughDate, shop.Timezone)
	if err != nil {
		return ActivityQuery{}, ActivityAudience{}, ErrInvalidQuery
	}
	filters.ProfessionalID = professional
	return ActivityQuery{ShopID: shop.ID, Period: period, Source: input.Source, Filters: filters}, audience, nil
}
