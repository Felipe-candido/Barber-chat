//go:build integration

package integration

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	report "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	reportpg "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/postgres"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	salespg "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reportingFixture struct {
	t                                                             *testing.T
	ctx                                                           context.Context
	pool                                                          *pgxpool.Pool
	tx                                                            *reportpg.Transactor
	salesTx                                                       *salespg.Transactor
	owner, barber, barberB, shop, profA, profB, service, customer uuid.UUID
	products                                                      []catalog.ProductOutput
}

func newReportingFixture(t *testing.T) *reportingFixture {
	t.Helper()
	dsn := requiredEnv(t, "REPORTING_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_reporting_") {
		t.Fatal("requires disposable codex_reporting_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var auth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&auth); err != nil || auth {
		t.Fatal("refusing Auth schema", err)
	}
	f := &reportingFixture{t: t, ctx: ctx, pool: pool, owner: uuid.New(), barber: uuid.New(), barberB: uuid.New(), shop: uuid.New(), profA: uuid.New(), profB: uuid.New(), service: uuid.New(), customer: uuid.New()}
	f.exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber A'),($3,'Barber B')", f.owner, f.barber, f.barberB)
	f.exec("INSERT INTO shops(id,name,slug,timezone) VALUES($1,'Reports',$2,'America/Sao_Paulo')", f.shop, "reports-"+f.shop.String())
	f.exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'A'),($3,$2,'B')", f.profA, f.shop, f.profB)
	f.exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$2,'barber',$6)", f.owner, f.shop, f.barber, f.profA, f.barberB, f.profB)
	f.exec("INSERT INTO services(id,shop_id,name,duration_minutes,price_cents,currency) VALUES($1,$2,'Current cut',40,7000,'BRL')", f.service, f.shop)
	f.exec("INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Ana %_','Silva','+5511912345678',now())", f.customer, f.shop)
	f.tx = reportpg.NewTransactor(pool, reportingBind)
	f.salesTx = salespg.NewTransactor(pool, func(q *db.Queries) sales.SalesTx {
		store := salespg.NewStore(q)
		stock := catalogpg.NewSalesCatalog(q)
		return sales.SalesTx{Access: identitypg.NewSalesGuard(q), Eligibility: billingpg.NewSalesEligibility(q), Catalog: stock, Appointments: bookingpg.NewSalesAppointments(q), Stock: stock, Sales: store, Commands: store, Cancellation: store, Returns: stock, Reader: store, Calendar: shopspg.NewSalesCalendar(q)}
	})
	productTx := catalogpg.NewProductTransactor(pool, func(q *db.Queries) catalog.ProductTx {
		s := catalogpg.NewProductStore(q)
		return catalog.ProductTx{Access: identitypg.NewProductGuard(q), Products: s, Reader: s, Stock: s, Commands: s}
	})
	commands := catalog.NewProductCommands(productTx, func() time.Time { return reportInstant("2026-01-01T00:00:00Z") })
	for _, p := range []struct {
		name  string
		price int64
	}{{"Pomada", 3000}, {"Shampoo", 2000}} {
		out, err := commands.Create(ctx, catalog.CreateProductInput{Scope: catalog.ProductScope{ActorID: f.owner, ShopID: f.shop}, Name: p.name, PriceCents: p.price, InitialQuantity: 100, IdempotencyKey: reportKey()})
		if err != nil {
			t.Fatal(err)
		}
		f.products = append(f.products, out.Product)
	}
	return f
}

func reportingBind(q *db.Queries, at time.Time) report.ActivityReadTx {
	r := reportpg.NewReader(q, at)
	return report.ActivityReadTx{Access: identitypg.NewActivityGuard(q), Shops: shopspg.NewReportShops(q), Overview: r, History: r}
}

func reportInstant(s string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return at
}
func reportKey() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
func (f *reportingFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *reportingFixture) appointment(at string, prof uuid.UUID, status domain.AppointmentStatus, price int64, label string) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	start := reportInstant(at)
	f.exec("INSERT INTO appointments(id,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,code_hash,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,40,$7,'BRL','Snapshot barber','America/Sao_Paulo',$8,$9,'scheduled',1,$10,$11,$11)", id, f.shop, f.customer, f.service, prof, label, price, start, start.Add(time.Hour), []byte(reportKey())[:32], start.Add(-time.Hour))
	if status != domain.StatusScheduled {
		f.change(id, status)
	}
	return id
}

func (f *reportingFixture) change(id uuid.UUID, status domain.AppointmentStatus) {
	f.t.Helper()
	f.exec("UPDATE appointments SET status=$2,version=version+1,status_source='staff',status_actor_id=$3,status_reason='Test correction',updated_at=clock_timestamp() WHERE id=$1", id, string(status), f.owner)
}

func (f *reportingFixture) sale(at string, prof, appointment uuid.UUID, quantities ...int64) sales.SaleOutput {
	f.t.Helper()
	in := sales.RegisterSaleInput{Scope: sales.SalesScope{ActorID: f.owner, ShopID: f.shop}, ProfessionalID: prof, AppointmentID: appointment, IdempotencyKey: reportKey()}
	for i, quantity := range quantities {
		if quantity > 0 {
			p := f.products[i]
			in.Items = append(in.Items, sales.SaleItemInput{ProductID: p.ID, Quantity: quantity, ExpectedProductVersion: p.Version, ExpectedUnitPriceCents: p.PriceCents})
		}
	}
	out, err := sales.NewRegisterSale(f.salesTx, func() time.Time { return reportInstant(at) }).Execute(f.ctx, in)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *reportingFixture) cancel(id uuid.UUID) {
	f.t.Helper()
	_, err := sales.NewCancelSale(f.salesTx, time.Now).Execute(f.ctx, sales.CancelSaleInput{Scope: sales.SalesScope{ActorID: f.owner, ShopID: f.shop}, SaleID: id, ExpectedVersion: 1, Reason: "Correction", Restock: false, IdempotencyKey: reportKey()})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *reportingFixture) query() report.ActivityQuery {
	p, err := domain.NewPeriod("2026-10-12", "2026-10-12", "America/Sao_Paulo")
	if err != nil {
		f.t.Fatal(err)
	}
	return report.ActivityQuery{ShopID: f.shop, Period: p, Source: domain.ActivityAll}
}

// This helper models the future use case's mandatory access -> effective scope
// -> read sequence. The SQL reader is never itself an authentication endpoint.
func (f *reportingFixture) read(actor uuid.UUID, q report.ActivityQuery, fn func(context.Context, report.ActivityReadTx, report.ActivityQuery) error) error {
	return f.tx.WithinActivityRead(f.ctx, func(ctx context.Context, tx report.ActivityReadTx) error {
		a, err := tx.Access.RequireActivityAccess(ctx, actor, q.ShopID)
		if err != nil {
			return err
		}
		shop, found, err := tx.Shops.FindActiveReportShop(ctx, q.ShopID)
		if err != nil {
			return err
		}
		if !found || shop.ID != q.ShopID {
			return report.ErrShopNotFound
		}
		if shop.Timezone != q.Period.Timezone() {
			return report.ErrInvalidQuery
		}
		prof, err := a.ProfessionalFor(actor, q.ShopID, q.Filters.ProfessionalID)
		if err != nil {
			return err
		}
		q.Filters.ProfessionalID = prof
		return fn(ctx, tx, q)
	})
}

func (f *reportingFixture) overview(actor uuid.UUID, q report.ActivityQuery) (report.ActivityOverview, error) {
	var out report.ActivityOverview
	err := f.read(actor, q, func(ctx context.Context, tx report.ActivityReadTx, q report.ActivityQuery) error {
		var err error
		out, err = tx.Overview.ReadActivityOverview(ctx, q)
		if err == nil && !out.ObservedAt.Equal(tx.ObservedAt) {
			return errors.New("different report snapshot instants")
		}
		return err
	})
	return out, err
}

func reportValues(t *testing.T, o report.ActivityOverview, service, product, forecast, appointments, receipts, units int64) {
	t.Helper()
	if len(o.Values) != 1 {
		t.Fatalf("expected currency bucket, got %+v", o)
	}
	v := o.Values[0]
	total, err := v.CombinedValueCents()
	if err != nil || v.Currency != "BRL" || v.CompletedServiceValueCents != service || v.ProductSaleValueCents != product || v.ScheduledServiceValueCents != forecast || v.CompletedAppointmentCount != appointments || v.ProductSaleCount != receipts || v.ProductUnitCount != units || total != service+product {
		t.Fatalf("unexpected report: %+v total=%d err=%v", o, total, err)
	}
}

func TestReportingPostgres(t *testing.T) {
	t.Run("empty projections and bounded history inputs", func(t *testing.T) {
		f := newReportingFixture(t)
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		if o.ObservedAt.IsZero() || o.Counts != (report.OutcomeCounts{}) || o.Values == nil || o.ByDay == nil || o.ByService == nil || o.ByProduct == nil || o.ByProfessional == nil || len(o.Values) != 0 {
			t.Fatal("empty report must retain metadata and empty collections", o)
		}
		err = f.read(f.owner, f.query(), func(ctx context.Context, tx report.ActivityReadTx, q report.ActivityQuery) error {
			rows, err := tx.History.ListActivityHistory(ctx, report.ActivityHistoryQuery{ActivityQuery: q, Limit: 101})
			if err != nil {
				return err
			}
			if rows == nil || len(rows) != 0 {
				t.Fatal(rows)
			}
			for _, in := range []report.ActivityHistoryQuery{
				{ActivityQuery: q, Limit: 0}, {ActivityQuery: q, Limit: 102},
				{ActivityQuery: q, Limit: 1, AppointmentStatus: "paid"},
				{ActivityQuery: q, Limit: 1, After: &report.ActivityCursor{Source: domain.ActivityAll}},
			} {
				if _, err := tx.History.ListActivityHistory(ctx, in); !errors.Is(err, report.ErrInvalidQuery) {
					t.Fatal("invalid history input accepted", in, err)
				}
			}
			q.Source = domain.ActivityServices
			q.Filters.ProductID = f.products[0].ID
			if _, err := tx.Overview.ReadActivityOverview(ctx, q); !errors.Is(err, report.ErrInvalidQuery) {
				t.Fatal("incompatible resource filter accepted", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		if err := f.tx.WithinActivityRead(ctx, func(context.Context, report.ActivityReadTx) error {
			t.Fatal("callback ran after cancellation")
			return nil
		}); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("190 example and historical snapshots survive inactive catalog", func(t *testing.T) {
		f := newReportingFixture(t)
		a := f.appointment("2026-10-12T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "Old cut")
		f.appointment("2026-10-12T13:30:00Z", f.profA, domain.StatusCompleted, 5000, "New cut snapshot")
		f.sale("2026-10-12T14:30:00Z", f.profA, a, 3)
		f.exec("UPDATE services SET name='Renamed',price_cents=9900,active=false WHERE id=$1", f.service)
		f.exec("UPDATE professionals SET name='Renamed barber',active=false WHERE id=$1", f.profA)
		f.exec("UPDATE products SET name='Renamed product',price_cents=9900,active=false,version=version+1 WHERE id=$1", f.products[0].ID)
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 10000, 9000, 0, 2, 1, 3)
		if o.Counts.Completed != 2 || len(o.ByService) != 1 || o.ByService[0].Label != "New cut snapshot" || len(o.ByProduct) != 1 || o.ByProduct[0].Label != "Pomada" || len(o.ByDay) != 1 || o.ByDay[0].LocalDate != "2026-10-12" {
			t.Fatal(o)
		}
		if _, err := f.overview(f.barber, f.query()); !errors.Is(err, report.ErrActivityAccessDenied) {
			t.Fatal("inactive barber retained access", err)
		}
	})
	t.Run("matching items distinct receipts independent source filters and contact", func(t *testing.T) {
		f := newReportingFixture(t)
		a := f.appointment("2026-10-12T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "Cut")
		f.sale("2026-10-12T13:30:00Z", f.profA, a, 2, 1)
		f.sale("2026-10-12T14:30:00Z", uuid.Nil, uuid.Nil, 1)
		q := f.query()
		o, err := f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 11000, 0, 1, 2, 4)
		if len(o.ByProduct) != 2 {
			t.Fatal(o)
		}
		q.Filters.ProductID = f.products[0].ID
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 9000, 0, 1, 2, 3)
		q.Filters.PhoneE164 = "+5511912345678"
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 6000, 0, 1, 1, 2)
		// A second contact proves that % and _ are literals rather than wildcards.
		originalCustomer := f.customer
		f.customer = uuid.New()
		f.exec("INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Maria','Other','+5511987654321',now())", f.customer, f.shop)
		f.appointment("2026-10-12T17:30:00Z", f.profA, domain.StatusCompleted, 9999, "Other contact")
		f.customer = originalCustomer
		q.Filters.PhoneE164 = ""
		q.Filters.CustomerName = "aNA %_"
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 6000, 0, 1, 1, 2)
		q.Filters.CustomerName = "nonmatching%"
		o, err = f.overview(f.owner, q)
		if err != nil || len(o.Values) != 0 {
			t.Fatal("LIKE wildcard was not literal", o, err)
		}
		q = f.query()
		q.Source = domain.ActivityProducts
		q.Filters.ProductID = f.products[1].ID
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 0, 2000, 0, 0, 1, 1)
		q = f.query()
		q.Filters.ServiceID = uuid.New()
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 0, 11000, 0, 0, 2, 4)
		f.exec("UPDATE customers SET phone_e164='+5511911111111' WHERE id=$1", f.customer)
		q = f.query()
		q.Filters.PhoneE164 = "+5511912345678"
		if o, err := f.overview(f.owner, q); err != nil || len(o.Values) != 0 {
			t.Fatal("old contact matched after correction", o, err)
		}
		q.Filters.PhoneE164 = "+5511911111111"
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 8000, 0, 1, 1, 3)
	})
	t.Run("exceptions forecast and later cancellations correct original period", func(t *testing.T) {
		f := newReportingFixture(t)
		a := f.appointment("2026-10-12T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "Cut")
		f.appointment("2026-10-12T13:30:00Z", f.profA, domain.StatusNoShow, 5000, "Cut")
		f.appointment("2026-10-12T14:30:00Z", f.profA, domain.StatusCancelled, 5000, "Cut")
		f.appointment("2026-10-12T15:30:00Z", f.profA, domain.StatusScheduled, 5000, "Cut")
		sale := f.sale("2026-10-12T16:30:00Z", f.profA, a, 1)
		f.change(a, domain.StatusCancelled)
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 0, 3000, 5000, 0, 1, 1)
		if o.Counts.Cancelled != 2 || o.Counts.NoShow != 1 || o.Counts.Scheduled != 1 {
			t.Fatal(o)
		}
		f.cancel(sale.ID)
		o, err = f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 0, 0, 5000, 0, 0, 0)
		err = f.read(f.owner, f.query(), func(ctx context.Context, tx report.ActivityReadTx, q report.ActivityQuery) error {
			rows, err := tx.History.ListActivityHistory(ctx, report.ActivityHistoryQuery{ActivityQuery: q, Limit: 100, AppointmentStatus: domain.StatusCancelled, SaleStatus: domain.ProductSaleCancelled})
			if err != nil {
				return err
			}
			if len(rows) != 3 {
				t.Fatalf("outcome filters lost original history: %+v", rows)
			}
			for _, r := range rows {
				v, e := r.RecordedValueCents()
				if e != nil || v != 0 || r.ValueCents <= 0 {
					t.Fatal(r, e)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unit and professional scope are applied before aggregation", func(t *testing.T) {
		f := newReportingFixture(t)
		g := newReportingFixture(t)
		f.appointment("2026-10-12T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "A")
		f.appointment("2026-10-12T12:30:00Z", f.profB, domain.StatusCompleted, 8000, "B")
		g.appointment("2026-10-12T12:30:00Z", g.profA, domain.StatusCompleted, 9900, "Other tenant")
		f.sale("2026-10-12T14:30:00Z", f.profA, uuid.Nil, 1)
		f.sale("2026-10-12T14:30:00Z", f.profB, uuid.Nil, 2)
		f.sale("2026-10-12T14:30:00Z", uuid.Nil, uuid.Nil, 1)
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 13000, 12000, 0, 2, 3, 4)
		if len(o.ByProfessional) != 3 {
			t.Fatal("unassigned owner group missing", o)
		}
		o, err = f.overview(f.barber, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 3000, 0, 1, 1, 1)
		q := f.query()
		q.Filters.ProfessionalID = f.profB
		if _, err := f.overview(f.barber, q); !errors.Is(err, report.ErrActivityAccessDenied) {
			t.Fatal("barber selected another professional", err)
		}
		if _, err := f.overview(g.owner, f.query()); !errors.Is(err, report.ErrActivityAccessDenied) {
			t.Fatal("foreign owner saw unit", err)
		}
		q = f.query()
		q.Source = domain.ActivityProducts
		q.Filters.ProductID = g.products[0].ID
		if o, err := f.overview(f.owner, q); err != nil || len(o.Values) != 0 {
			t.Fatal("foreign product matched", o, err)
		}
	})
	t.Run("keyset keeps tied sources receipts and items without duplicates", func(t *testing.T) {
		f := newReportingFixture(t)
		a := f.appointment("2026-10-12T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "Cut")
		f.sale("2026-10-12T12:30:00Z", f.profA, a, 1, 1)
		f.sale("2026-10-12T12:30:00Z", f.profA, a, 1, 1)
		f.sale("2026-10-12T15:30:00Z", f.profB, uuid.Nil, 1)
		seen := map[string]bool{}
		var after *report.ActivityCursor
		for page := 0; page < 8; page++ {
			var rows []report.ActivityHistoryRow
			err := f.read(f.barber, f.query(), func(ctx context.Context, tx report.ActivityReadTx, q report.ActivityQuery) error {
				var err error
				rows, err = tx.History.ListActivityHistory(ctx, report.ActivityHistoryQuery{ActivityQuery: q, Limit: 1, After: after})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				break
			}
			r := rows[0]
			if r.ProfessionalID != f.profA || (page == 0 && r.Source != domain.ActivityServices) {
				t.Fatal("scope or source ordering changed", r)
			}
			key := string(r.Source) + r.RecordID.String() + r.ItemID.String()
			if seen[key] {
				t.Fatal("duplicate page row", r)
			}
			seen[key] = true
			after = &report.ActivityCursor{OccurredAt: r.OccurredAt, Source: r.Source, RecordID: r.RecordID, ItemID: r.ItemID}
		}
		if len(seen) != 5 {
			t.Fatal("tied rows skipped or scope applied after limit", len(seen))
		}
	})
	t.Run("civil day boundaries and DST use starts and sold instants independently", func(t *testing.T) {
		f := newReportingFixture(t)
		f.appointment("2026-10-12T03:00:00Z", f.profA, domain.StatusCompleted, 5000, "Cut")
		f.appointment("2026-10-13T02:30:00Z", f.profA, domain.StatusCompleted, 5000, "Cut")
		f.appointment("2026-10-12T02:00:00Z", f.profB, domain.StatusCompleted, 5000, "Before")
		f.appointment("2026-10-13T03:00:00Z", f.profB, domain.StatusCompleted, 5000, "After")
		a := f.appointment("2026-10-11T12:30:00Z", f.profA, domain.StatusCompleted, 5000, "Yesterday")
		f.sale("2026-10-12T03:00:00Z", f.profA, a, 1)
		f.sale("2026-10-13T03:00:00Z", f.profA, a, 1)
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 10000, 3000, 0, 2, 1, 1)
		f.exec("UPDATE shops SET timezone='America/New_York' WHERE id=$1", f.shop)
		f.appointment("2026-03-08T05:00:00Z", f.profA, domain.StatusCompleted, 5000, "DST")
		f.appointment("2026-03-09T03:00:00Z", f.profA, domain.StatusCompleted, 5000, "DST")
		f.appointment("2026-03-09T04:00:00Z", f.profA, domain.StatusCompleted, 5000, "Next day")
		p, err := domain.NewPeriod("2026-03-08", "2026-03-08", "America/New_York")
		if err != nil {
			t.Fatal(err)
		}
		q := f.query()
		q.Period = p
		o, err = f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 10000, 0, 0, 2, 0, 0)
		if len(o.ByDay) != 1 || o.ByDay[0].LocalDate != "2026-03-08" {
			t.Fatal(o)
		}
	})
	t.Run("repeatable read preserves totals while next read sees cancellation", func(t *testing.T) {
		f := newReportingFixture(t)
		s := f.sale("2026-10-12T12:30:00Z", uuid.Nil, uuid.Nil, 1)
		err := f.read(f.owner, f.query(), func(ctx context.Context, tx report.ActivityReadTx, q report.ActivityQuery) error {
			before, err := tx.Overview.ReadActivityOverview(ctx, q)
			if err != nil {
				return err
			}
			reportValues(t, before, 0, 3000, 0, 0, 1, 1)
			f.cancel(s.ID)
			after, err := tx.Overview.ReadActivityOverview(ctx, q)
			if err != nil {
				return err
			}
			reportValues(t, after, 0, 3000, 0, 0, 1, 1)
			rows, err := tx.History.ListActivityHistory(ctx, report.ActivityHistoryQuery{ActivityQuery: q, Limit: 100})
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].SaleStatus != domain.ProductSaleRecorded {
				t.Fatal("history observed a different snapshot", rows)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		o, err := f.overview(f.owner, f.query())
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 0, 0, 0, 0, 0, 0)
	})
	t.Run("revocation rechecked and read transaction rejects writes", func(t *testing.T) {
		f := newReportingFixture(t)
		for _, table := range []string{"users", "shop_memberships", "shops"} {
			var sql string
			switch table {
			case "users":
				sql = "UPDATE users SET active=$1 WHERE id=$2"
			case "shop_memberships":
				sql = "UPDATE shop_memberships SET active=$1 WHERE user_id=$2"
			case "shops":
				sql = "UPDATE shops SET active=$1 WHERE id=$2"
			}
			id := f.owner
			if table == "shops" {
				id = f.shop
			}
			f.exec(sql, false, id)
			if _, err := f.overview(f.owner, f.query()); !errors.Is(err, report.ErrActivityAccessDenied) {
				t.Fatal("inactive scope accepted", table, err)
			}
			f.exec(sql, true, id)
		}
		var bound *db.Queries
		tx := reportpg.NewTransactor(f.pool, func(q *db.Queries, at time.Time) report.ActivityReadTx { bound = q; return reportingBind(q, at) })
		err := tx.WithinActivityRead(f.ctx, func(ctx context.Context, _ report.ActivityReadTx) error {
			return bound.InsertRecordedSale(ctx, db.InsertRecordedSaleParams{})
		})
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "25006" {
			t.Fatal("report transaction allowed writes", err)
		}
		if _, err := f.overview(f.owner, f.query()); err != nil {
			t.Fatal("failed read transaction was not cleaned up", err)
		}
	})
	t.Run("automatic completion provenance and aggregation overflow", func(t *testing.T) {
		f := newReportingFixture(t)
		var at time.Time
		if err := f.pool.QueryRow(f.ctx, "SELECT activated_at+interval '1 millisecond' FROM appointment_completion_policy").Scan(&at); err != nil {
			t.Fatal(err)
		}
		a := f.appointment(at.Add(-time.Hour).Format(time.RFC3339Nano), f.profA, domain.StatusScheduled, 5000, "Auto")
		f.exec("UPDATE appointments SET status='completed',version=version+1,status_source='automatic',status_reason='Automatic completion',updated_at=clock_timestamp() WHERE id=$1", a)
		loc, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			t.Fatal(err)
		}
		date := at.Add(-time.Hour).In(loc).Format(time.DateOnly)
		p, err := domain.NewPeriod(date, date, "America/Sao_Paulo")
		if err != nil {
			t.Fatal(err)
		}
		q := f.query()
		q.Period = p
		o, err := f.overview(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		reportValues(t, o, 5000, 0, 0, 1, 0, 0)
		if o.Counts.CompletedAutomatically != 1 || o.Counts.Completed != 1 {
			t.Fatal("automatic completion counted twice", o)
		}
		f.appointment("2026-10-12T12:30:00Z", f.profB, domain.StatusCompleted, 9_007_199_254_740_991, "Large")
		f.appointment("2026-10-12T13:30:00Z", f.profB, domain.StatusCompleted, 1, "Large")
		if _, err := f.overview(f.owner, f.query()); !errors.Is(err, report.ErrInconsistentData) {
			t.Fatal("unsafe aggregate rounded instead of failing", err)
		}
	})
}
