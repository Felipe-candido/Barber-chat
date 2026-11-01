package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

type reportTxMarker struct{}
type queryFixture struct {
	t                                                  *testing.T
	actor, shop, prof, service, product                uuid.UUID
	at                                                 time.Time
	audience                                           app.ActivityAudience
	shopRow                                            app.ReportShop
	shopFound                                          bool
	accessErr, shopErr, readerErr, commitErr, phoneErr error
	phoneOutput                                        string
	phoneArgs                                          [2]string
	txCalls, accessCalls, historyCalls, overviewCalls  int
	inTx                                               bool
	trace                                              []string
	query                                              app.ActivityQuery
	historyQuery                                       app.ActivityHistoryQuery
	rows                                               []app.ActivityHistoryRow
	overview                                           func(app.ActivityQuery) app.ActivityOverview
}

func newQueryFixture(t *testing.T) *queryFixture {
	f := &queryFixture{t: t, actor: uuid.New(), shop: uuid.New(), prof: uuid.New(), service: uuid.New(), product: uuid.New(),
		at: time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC), shopFound: true, phoneOutput: "+5511912345678"}
	f.audience = app.ActivityAudience{ActorID: f.actor, ShopID: f.shop, CanReadAll: true}
	f.shopRow = app.ReportShop{ID: f.shop, Timezone: "America/Sao_Paulo"}
	return f
}

func (f *queryFixture) uc() *app.ActivityQueries { return app.NewActivityQueries(f, f) }
func (f *queryFixture) input() app.ActivityReportInput {
	return app.ActivityReportInput{ActorID: f.actor, ShopID: f.shop, FromDate: "2026-10-12", ThroughDate: "2026-10-12"}
}
func (f *queryFixture) WithinActivityRead(ctx context.Context, fn func(context.Context, app.ActivityReadTx) error) error {
	f.txCalls++
	f.inTx = true
	defer func() { f.inTx = false }()
	err := fn(context.WithValue(ctx, reportTxMarker{}, true), app.ActivityReadTx{ObservedAt: f.at, Access: f, Shops: f, Overview: f, History: f})
	if err != nil {
		return err
	}
	return f.commitErr
}
func (f *queryFixture) checkContext(ctx context.Context) {
	f.t.Helper()
	if ctx.Value(reportTxMarker{}) != true || !f.inTx {
		f.t.Fatal("read escaped its transaction context")
	}
}
func (f *queryFixture) RequireActivityAccess(ctx context.Context, actor, shop uuid.UUID) (app.ActivityAudience, error) {
	f.checkContext(ctx)
	f.accessCalls++
	f.trace = append(f.trace, "access")
	if actor != f.audience.ActorID || shop != f.audience.ShopID {
		return app.ActivityAudience{}, app.ErrActivityAccessDenied
	}
	return f.audience, f.accessErr
}
func (f *queryFixture) FindActiveReportShop(ctx context.Context, id uuid.UUID) (app.ReportShop, bool, error) {
	f.checkContext(ctx)
	f.trace = append(f.trace, "shop")
	return f.shopRow, f.shopFound, f.shopErr
}
func (f *queryFixture) ReadActivityOverview(ctx context.Context, q app.ActivityQuery) (app.ActivityOverview, error) {
	f.checkContext(ctx)
	f.overviewCalls++
	f.trace = append(f.trace, "overview")
	f.query = q
	if f.readerErr != nil {
		return app.ActivityOverview{}, f.readerErr
	}
	if f.overview != nil {
		return f.overview(q), nil
	}
	return app.ActivityOverview{Period: q.Period, Source: q.Source, ObservedAt: f.at}, nil
}
func (f *queryFixture) ListActivityHistory(ctx context.Context, q app.ActivityHistoryQuery) ([]app.ActivityHistoryRow, error) {
	f.checkContext(ctx)
	f.historyCalls++
	f.trace = append(f.trace, "history")
	f.historyQuery = q
	return f.rows, f.readerErr
}
func (f *queryFixture) NormalizeReportPhone(raw, country string) (string, error) {
	if f.inTx {
		f.t.Fatal("local normalization should precede database transaction")
	}
	f.phoneArgs = [2]string{raw, country}
	return f.phoneOutput, f.phoneErr
}
func (f *queryFixture) row(hour int) app.ActivityHistoryRow {
	return app.ActivityHistoryRow{ShopID: f.shop, Source: domain.ActivityServices, RecordID: uuid.New(), ServiceID: f.service, ProfessionalID: f.prof,
		Label: "Cut snapshot", ProfessionalName: "Barber snapshot", Currency: "BRL", AppointmentStatus: domain.StatusCompleted, AppointmentStatusSource: domain.SourceAutomatic,
		CustomerName: "Ana Silva", CustomerPhone: "+5511912345678", OccurredAt: time.Date(2026, 10, 12, hour, 30, 0, 0, time.UTC), Quantity: 1, UnitPriceCents: 5000, ValueCents: 5000}
}

func TestActivityOverviewResolvesCurrentScopeAndNormalizesSharedFilters(t *testing.T) {
	f := newQueryFixture(t)
	f.audience.CanReadAll = false
	f.audience.ProfessionalID = f.prof
	in := f.input()
	in.Filters.CustomerName = "  Ana %_  "
	in.Filters.Phone = " (11) 91234-5678 "
	in.Filters.PhoneCountry = " br "
	out, err := f.uc().Overview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	q := f.query
	if strings.Join(f.trace, ",") != "access,shop,overview" || q.ShopID != f.shop || q.Filters.ProfessionalID != f.prof || q.Source != domain.ActivityAll ||
		q.Filters.CustomerName != "Ana %_" || q.Filters.PhoneE164 != "+5511912345678" || f.phoneArgs != [2]string{"(11) 91234-5678", "BR"} ||
		q.Period.From().Format(time.RFC3339) != "2026-10-12T03:00:00Z" || q.Period.Until().Format(time.RFC3339) != "2026-10-13T03:00:00Z" {
		t.Fatal(f.trace, q, f.phoneArgs)
	}
	if out.Values == nil || out.ByDay == nil || out.ByProfessional == nil || out.ByService == nil || out.ByProduct == nil || !out.ObservedAt.Equal(f.at) {
		t.Fatal("empty collections/metadata lost", out)
	}
	if in.Filters.Phone != " (11) 91234-5678 " || in.Source != "" {
		t.Fatal("caller input mutated", in)
	}
	in.Filters.ProfessionalID = uuid.New()
	if _, err := f.uc().Overview(context.Background(), in); !errors.Is(err, app.ErrActivityAccessDenied) || f.overviewCalls != 1 {
		t.Fatal("barber expanded authority", err)
	}
}

func TestActivityQueriesRejectInvalidInputAndBrokenDependencies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*queryFixture, *app.ActivityReportInput)
		want   error
	}{
		{"missing actor", func(f *queryFixture, i *app.ActivityReportInput) { i.ActorID = uuid.Nil }, app.ErrActivityAccessDenied},
		{"missing shop", func(f *queryFixture, i *app.ActivityReportInput) { i.ShopID = uuid.Nil }, app.ErrActivityAccessDenied},
		{"unknown source", func(f *queryFixture, i *app.ActivityReportInput) { i.Source = "mixed" }, app.ErrInvalidQuery},
		{"incompatible product", func(f *queryFixture, i *app.ActivityReportInput) {
			i.Source = domain.ActivityServices
			i.Filters.ProductID = f.product
		}, app.ErrInvalidQuery},
		{"control in name", func(f *queryFixture, i *app.ActivityReportInput) { i.Filters.CustomerName = "Ana\n" }, app.ErrInvalidQuery},
		{"invalid unicode", func(f *queryFixture, i *app.ActivityReportInput) { i.Filters.CustomerName = string([]byte{0xff}) }, app.ErrInvalidQuery},
		{"long name", func(f *queryFixture, i *app.ActivityReportInput) { i.Filters.CustomerName = strings.Repeat("ç", 202) }, app.ErrInvalidQuery},
		{"orphan country", func(f *queryFixture, i *app.ActivityReportInput) { i.Filters.PhoneCountry = "BR" }, app.ErrInvalidQuery},
		{"invalid phone", func(f *queryFixture, i *app.ActivityReportInput) {
			i.Filters.Phone = "123"
			f.phoneErr = app.ErrInvalidQuery
		}, app.ErrInvalidQuery},
		{"invalid canonical result", func(f *queryFixture, i *app.ActivityReportInput) {
			i.Filters.Phone = "123"
			f.phoneOutput = "11912345678"
		}, app.ErrInconsistentData},
		{"reversed dates", func(f *queryFixture, i *app.ActivityReportInput) { i.FromDate = "2026-10-13" }, app.ErrInvalidQuery},
		{"unbounded period", func(f *queryFixture, i *app.ActivityReportInput) { i.FromDate = "2025-01-01" }, app.ErrInvalidQuery},
		{"missing date", func(f *queryFixture, i *app.ActivityReportInput) { i.ThroughDate = "" }, app.ErrInvalidQuery},
		{"access revoked", func(f *queryFixture, i *app.ActivityReportInput) { f.accessErr = app.ErrActivityAccessDenied }, app.ErrActivityAccessDenied},
		{"barber missing professional", func(f *queryFixture, i *app.ActivityReportInput) { f.audience.CanReadAll = false }, app.ErrActivityAccessDenied},
		{"inactive shop", func(f *queryFixture, i *app.ActivityReportInput) { f.shopFound = false }, app.ErrShopNotFound},
		{"foreign shop projection", func(f *queryFixture, i *app.ActivityReportInput) { f.shopRow.ID = uuid.New() }, app.ErrInconsistentData},
		{"invalid stored zone", func(f *queryFixture, i *app.ActivityReportInput) { f.shopRow.Timezone = "not-a-zone" }, app.ErrInconsistentData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueryFixture(t)
			in := f.input()
			tc.mutate(f, &in)
			if _, err := f.uc().Overview(context.Background(), in); !errors.Is(err, tc.want) || f.overviewCalls != 0 || f.historyCalls != 0 {
				t.Fatal("invalid input reached reader", err, f.trace)
			}
		})
	}
	dependencyErr := errors.New("injected dependency error")
	for _, stage := range []string{"access", "shop", "phone", "reader", "commit"} {
		t.Run(stage+" error", func(t *testing.T) {
			f := newQueryFixture(t)
			in := f.input()
			switch stage {
			case "access":
				f.accessErr = dependencyErr
			case "shop":
				f.shopErr = dependencyErr
			case "phone":
				in.Filters.Phone = "+5511912345678"
				f.phoneErr = dependencyErr
			case "reader":
				f.readerErr = dependencyErr
			case "commit":
				f.commitErr = dependencyErr
			}
			out, err := f.uc().Overview(context.Background(), in)
			if !errors.Is(err, dependencyErr) || out.Period.Valid() || out.Values != nil {
				t.Fatal("dependency failure returned partial report", out, err)
			}
		})
	}
}

func TestActivityHistoryBuildsNextFromLastReturnedRowAndRechecksAccess(t *testing.T) {
	f := newQueryFixture(t)
	f.rows = []app.ActivityHistoryRow{f.row(14), f.row(13), f.row(12)}
	in := app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: 2}
	first, err := f.uc().History(context.Background(), in)
	if err != nil || len(first.Rows) != 2 || first.Next == nil || first.Next.RecordID != f.rows[1].RecordID || first.Next.FilterFingerprint == ([32]byte{}) || f.historyQuery.Limit != 3 {
		t.Fatal(first, err, f.historyQuery)
	}
	in.After = first.Next
	f.rows = f.rows[2:]
	in.PageSize = 1
	second, err := f.uc().History(context.Background(), in)
	if err != nil || len(second.Rows) != 1 || second.Next != nil || f.accessCalls != 2 || f.historyQuery.After == in.After || f.historyQuery.After.RecordID != in.After.RecordID {
		t.Fatal(second, err)
	}
	f.accessErr = app.ErrActivityAccessDenied
	if _, err := f.uc().History(context.Background(), in); !errors.Is(err, app.ErrActivityAccessDenied) || f.historyCalls != 2 {
		t.Fatal("page replay bypassed revocation", err)
	}
}

func TestActivityHistoryCursorBindsEveryFilterAndEffectiveAuthority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*queryFixture, *app.ActivityHistoryInput)
	}{
		{"actor", func(f *queryFixture, i *app.ActivityHistoryInput) {
			i.ActorID = uuid.New()
			f.audience.ActorID = i.ActorID
		}},
		{"tenant", func(f *queryFixture, i *app.ActivityHistoryInput) {
			i.ShopID = uuid.New()
			f.audience.ShopID = i.ShopID
			f.shopRow.ID = i.ShopID
		}},
		{"professional", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Filters.ProfessionalID = uuid.New() }},
		{"owner became barber", func(f *queryFixture, i *app.ActivityHistoryInput) {
			f.audience.CanReadAll = false
			f.audience.ProfessionalID = f.prof
		}},
		{"from", func(f *queryFixture, i *app.ActivityHistoryInput) { i.FromDate = "2026-10-11" }},
		{"through", func(f *queryFixture, i *app.ActivityHistoryInput) { i.ThroughDate = "2026-10-13" }},
		{"timezone same offset", func(f *queryFixture, i *app.ActivityHistoryInput) { f.shopRow.Timezone = "America/Fortaleza" }},
		{"source", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Source = domain.ActivityServices }},
		{"name", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Filters.CustomerName = "Maria" }},
		{"phone", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Filters.Phone = "+5511912345678" }},
		{"service", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Filters.ServiceID = uuid.New() }},
		{"product", func(f *queryFixture, i *app.ActivityHistoryInput) { i.Filters.ProductID = uuid.New() }},
		{"appointment outcome", func(f *queryFixture, i *app.ActivityHistoryInput) { i.AppointmentStatus = domain.StatusCompleted }},
		{"sale outcome", func(f *queryFixture, i *app.ActivityHistoryInput) { i.SaleStatus = domain.ProductSaleCancelled }},
		{"forged fingerprint", func(f *queryFixture, i *app.ActivityHistoryInput) { i.After.FilterFingerprint = [32]byte{} }},
		{"all as row origin", func(f *queryFixture, i *app.ActivityHistoryInput) { i.After.Source = domain.ActivityAll }},
		{"out of period", func(f *queryFixture, i *app.ActivityHistoryInput) {
			i.After.OccurredAt = i.After.OccurredAt.AddDate(0, 0, -1)
		}},
		{"item on service", func(f *queryFixture, i *app.ActivityHistoryInput) { i.After.ItemID = uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueryFixture(t)
			f.rows = []app.ActivityHistoryRow{f.row(14), f.row(13)}
			in := app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: 1}
			in.Filters.ProfessionalID = f.prof
			first, err := f.uc().History(context.Background(), in)
			if err != nil || first.Next == nil {
				t.Fatal(first, err)
			}
			in.After = first.Next
			tc.mutate(f, &in)
			if _, err := f.uc().History(context.Background(), in); !errors.Is(err, app.ErrInvalidQuery) || f.historyCalls != 1 {
				t.Fatal("cursor escaped its binding", err, f.trace)
			}
		})
	}
}

func TestActivityHistoryCursorUsesCanonicalPhoneAndAllowsPageSizeChange(t *testing.T) {
	f := newQueryFixture(t)
	f.rows = []app.ActivityHistoryRow{f.row(14), f.row(13)}
	in := app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: 1}
	in.Filters.Phone = "(11) 91234-5678"
	in.Filters.PhoneCountry = "br"
	in.Filters.CustomerName = " Ana "
	first, err := f.uc().History(context.Background(), in)
	if err != nil || first.Next == nil {
		t.Fatal(first, err)
	}
	in.After = first.Next
	in.PageSize = 50
	in.Filters.Phone = "+5511912345678"
	in.Filters.PhoneCountry = ""
	in.Filters.CustomerName = "Ana"
	f.rows = f.rows[1:]
	out, err := f.uc().History(context.Background(), in)
	if err != nil || len(out.Rows) != 1 || f.historyQuery.Limit != 51 {
		t.Fatal("equivalent normalized filters invalidated cursor", out, err)
	}
}

func TestActivityHistoryRejectsOutOfScopeMalformedAndUnorderedReaderResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*queryFixture)
	}{
		{"foreign tenant in extra row", func(f *queryFixture) { f.rows[1].ShopID = uuid.New() }},
		{"foreign professional", func(f *queryFixture) { f.rows[0].ProfessionalID = uuid.New() }},
		{"duplicate", func(f *queryFixture) { f.rows[1] = f.rows[0] }},
		{"ascending", func(f *queryFixture) { f.rows[0], f.rows[1] = f.rows[1], f.rows[0] }},
		{"wrong amount", func(f *queryFixture) { f.rows[0].UnitPriceCents = 1 }},
		{"wrong outcome", func(f *queryFixture) { f.rows[0].AppointmentStatus = domain.StatusNoShow }},
		{"wrong resource", func(f *queryFixture) { f.rows[0].ServiceID = uuid.New() }},
		{"ambiguous origin", func(f *queryFixture) { f.rows[0].SaleStatus = domain.ProductSaleRecorded }},
		{"outside period", func(f *queryFixture) { f.rows[0].OccurredAt = f.rows[0].OccurredAt.AddDate(0, 0, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueryFixture(t)
			f.audience.CanReadAll = false
			f.audience.ProfessionalID = f.prof
			f.rows = []app.ActivityHistoryRow{f.row(14), f.row(13)}
			tc.mutate(f)
			in := app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: 1, AppointmentStatus: domain.StatusCompleted}
			in.Filters.ServiceID = f.service
			out, err := f.uc().History(context.Background(), in)
			if !errors.Is(err, app.ErrInconsistentData) || out.Rows != nil || out.Next != nil {
				t.Fatal("malformed read disclosed partial history", out, err)
			}
		})
	}
	f := newQueryFixture(t)
	f.rows = []app.ActivityHistoryRow{f.row(14), f.row(13)}
	f.commitErr = errors.New("read commit failed")
	out, err := f.uc().History(context.Background(), app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: 1})
	if !errors.Is(err, f.commitErr) || out.Next != nil || out.Rows != nil {
		t.Fatal("failed transaction exposed page/cursor", out, err)
	}
}

func TestActivityHistoryDefaultLimitStatusRulesAndCancellation(t *testing.T) {
	for _, size := range []int{0, 100} {
		f := newQueryFixture(t)
		out, err := f.uc().History(context.Background(), app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: size})
		want := 101
		if size == 0 {
			want = 26
		}
		if err != nil || f.historyQuery.Limit != want || out.Rows == nil || out.Next != nil {
			t.Fatal(out, err, f.historyQuery)
		}
	}
	for _, tc := range []struct {
		size        int
		source      domain.ActivitySource
		appointment domain.AppointmentStatus
		sale        domain.ProductSaleStatus
	}{
		{-1, "", "", ""}, {101, "", "", ""}, {1, "", "paid", ""}, {1, "", "", "paid"},
		{1, domain.ActivityProducts, domain.StatusCompleted, ""}, {1, domain.ActivityServices, "", domain.ProductSaleRecorded},
	} {
		f := newQueryFixture(t)
		in := app.ActivityHistoryInput{ActivityReportInput: f.input(), PageSize: tc.size, AppointmentStatus: tc.appointment, SaleStatus: tc.sale}
		in.Source = tc.source
		if _, err := f.uc().History(context.Background(), in); !errors.Is(err, app.ErrInvalidQuery) || f.txCalls != 0 {
			t.Fatal("invalid history selection opened transaction", tc, err)
		}
	}
	f := newQueryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.uc().Overview(ctx, f.input()); !errors.Is(err, context.Canceled) || f.txCalls != 0 {
		t.Fatal(err)
	}
	if _, err := f.uc().History(ctx, app.ActivityHistoryInput{ActivityReportInput: f.input()}); !errors.Is(err, context.Canceled) || f.txCalls != 0 {
		t.Fatal(err)
	}
}

func TestActivityOverviewRejectsScopeMetadataAndValueContradictions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*queryFixture, *app.ActivityOverview)
	}{
		{"wrong source", func(f *queryFixture, o *app.ActivityOverview) { o.Source = domain.ActivityProducts }},
		{"wrong snapshot", func(f *queryFixture, o *app.ActivityOverview) { o.ObservedAt = o.ObservedAt.Add(time.Second) }},
		{"foreign professional group", func(f *queryFixture, o *app.ActivityOverview) {
			o.ByProfessional = []app.ActivityGroup{{ID: uuid.New(), Label: "Other"}}
		}},
		{"outside day", func(f *queryFixture, o *app.ActivityOverview) {
			o.ByDay = []app.DailyActivity{{LocalDate: "2026-10-13"}}
		}},
		{"duplicate day", func(f *queryFixture, o *app.ActivityOverview) {
			o.ByDay = []app.DailyActivity{{LocalDate: "2026-10-12"}, {LocalDate: "2026-10-12"}}
		}},
		{"money without count", func(f *queryFixture, o *app.ActivityOverview) {
			o.Values = []app.ActivityValues{{Currency: "BRL", CompletedServiceValueCents: 5000}}
		}},
		{"count without currency", func(f *queryFixture, o *app.ActivityOverview) { o.Counts.Completed = 1 }},
		{"currency counts disagree", func(f *queryFixture, o *app.ActivityOverview) {
			o.Values = []app.ActivityValues{{Currency: "BRL", CompletedServiceValueCents: 5000, CompletedAppointmentCount: 1}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueryFixture(t)
			f.audience.CanReadAll = false
			f.audience.ProfessionalID = f.prof
			f.overview = func(q app.ActivityQuery) app.ActivityOverview {
				o := app.ActivityOverview{Period: q.Period, Source: q.Source, ObservedAt: f.at}
				tc.mutate(f, &o)
				return o
			}
			if _, err := f.uc().Overview(context.Background(), f.input()); !errors.Is(err, app.ErrInconsistentData) {
				t.Fatal("contradictory overview accepted", err)
			}
		})
	}
}
