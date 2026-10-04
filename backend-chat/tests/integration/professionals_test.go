//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProfessionalSetupToPublicBooking(t *testing.T) {
	f := newBookingFixture(t)
	other := newBookingFixture(t)
	userID := uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO users (id,display_name) VALUES ($1,'Professional setup test')", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO shop_memberships (user_id,shop_id) VALUES ($1,$2)", userID, f.shop); err != nil {
		t.Fatal(err)
	}
	q := db.New(f.pool)
	repo := catalogpostgres.NewRepository(q)
	identities := identitypostgres.NewRepository(q)
	shops := shopspostgres.NewResolver(q)
	tx := catalogpostgres.NewProfessionalTransactor(f.pool, func(q *db.Queries) catalog.ProfessionalTx {
		return catalog.ProfessionalTx{Professionals: catalogpostgres.NewRepository(q), Schedule: bookingpostgres.NewRepository(q)}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := httpapi.NewHandler(httpapi.HandlerConfig{Professionals: cataloghttp.NewProfessionalHandler(catalog.NewCreateProfessional(tx), catalog.NewListProfessionals(repo), 3*time.Second, logger),
		Authenticate: identity.NewAuthenticate(catalogTokenVerifier{userID}, identities), AuthorizeShop: identity.NewAuthorizeShopAction(identities, shops), Logger: logger})
	request := func(slug, method string, body any, authenticated bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1/admin/shops/"+slug+"/professionals", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if authenticated {
			r.Header.Set("Authorization", "Bearer integration-token")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := map[string]any{"name": "New barber", "service_ids": []uuid.UUID{f.service, f.longService}, "schedule": map[string]any{"interval_minutes": 45, "buffer_minutes": 0, "weekdays": []int{1, 2, 3, 4, 5, 6}, "opens_at_minute": 600, "closes_at_minute": 1080}}
	if w := request(f.slug, "POST", body, false); w.Code != 401 {
		t.Fatal("unauthenticated professional creation accepted")
	}
	if w := request(other.slug, "POST", body, true); w.Code != 403 {
		t.Fatal("non-member professional creation accepted")
	}
	w := request(f.slug, "POST", body, true)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID         uuid.UUID   `json:"id"`
		ServiceIDs []uuid.UUID `json:"service_ids"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == uuid.Nil || len(created.ServiceIDs) != 2 {
		t.Fatal("invalid professional result")
	}
	var workingDays int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM working_hours WHERE shop_id=$1 AND professional_id=$2", f.shop, created.ID).Scan(&workingDays); err != nil || workingDays != 6 {
		t.Fatal("initial working week was not inserted completely", err)
	}
	if w := request(f.slug, "GET", nil, true); w.Code != 200 || !strings.Contains(w.Body.String(), "New barber") {
		t.Fatal("created professional not listed")
	}
	if w := f.request("GET", "/professionals?service_id="+f.service.String(), nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), created.ID.String()) {
		t.Fatal("created professional not bookable")
	}
	bookingBody := f.body(f.start.Add(30*time.Minute), f.service)
	bookingBody["professional_id"] = created.ID
	// Pick an enabled day even when the fixture's tomorrow happens to be Sunday.
	if f.start.Weekday() == time.Sunday {
		bookingBody["starts_at"] = f.start.Add(24*time.Hour + 30*time.Minute).Format(time.RFC3339)
	}
	booked := bookingResult(t, f.request("POST", "/appointments", bookingBody, bookingKey()), 201)
	if booked.Appointment.EndsAt.Sub(booked.Appointment.StartsAt) != 45*time.Minute {
		t.Fatal("professional interval was not applied")
	}
	// A schedule failure must undo the already inserted professional/associations.
	body["name"] = "Invalid calendar"
	body["schedule"].(map[string]any)["closes_at_minute"] = 500
	if w := request(f.slug, "POST", body, true); w.Code != 422 {
		t.Fatalf("invalid schedule: %d %s", w.Code, w.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM professionals WHERE shop_id=$1", f.shop).Scan(&count); err != nil || count != 2 {
		t.Fatalf("partial professional committed: %d %v", count, err)
	}
	body["schedule"].(map[string]any)["closes_at_minute"] = 1080
	body["service_ids"] = []uuid.UUID{f.service, other.service}
	if w := request(f.slug, "POST", body, true); w.Code != 422 {
		t.Fatal("cross-tenant services accepted")
	}
	body["service_ids"] = []uuid.UUID{f.service}
	body["shop_id"] = other.shop
	if w := request(f.slug, "POST", body, true); w.Code != 400 {
		t.Fatal("untrusted shop_id accepted")
	}
}

type delayedProfessionalQueries struct {
	calls atomic.Int32
}

func (tracer *delayedProfessionalQueries) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	tracer.calls.Add(1)
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return ctx
}
func (*delayedProfessionalQueries) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestProfessionalSetupWithRemoteDatabaseLatency(t *testing.T) {
	f := newBookingFixture(t)
	// Delay actual PostgreSQL round trips, without writing to the hosted database.
	config, err := pgxpool.ParseConfig(requiredEnv(t, "BOOKING_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal("cannot configure disposable test pool")
	}
	config.MaxConns = 1
	tracer := &delayedProfessionalQueries{}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(f.ctx); err != nil {
		t.Fatal(err)
	}
	tracer.calls.Store(0)
	tx := catalogpostgres.NewProfessionalTransactor(pool, func(q *db.Queries) catalog.ProfessionalTx {
		return catalog.ProfessionalTx{Professionals: catalogpostgres.NewRepository(q), Schedule: bookingpostgres.NewRepository(q)}
	})
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	started := time.Now()
	result, err := catalog.NewCreateProfessional(tx).Execute(ctx, catalog.CreateProfessionalInput{
		ShopID: f.shop, Name: "Remote latency fixture", ServiceIDs: []uuid.UUID{f.service, f.longService},
		Schedule: catalog.InitialSchedule{IntervalMinutes: 60, Weekdays: []int{1, 2, 3, 4, 5, 6}, OpensAtMinute: 570, ClosesAtMinute: 1110},
	})
	if err != nil {
		t.Fatal("valid setup failed with simulated remote latency", err)
	}
	if time.Since(started) <= 3*time.Second {
		t.Fatal("test did not exceed the previous three-second budget")
	}
	if count := tracer.calls.Load(); count != 9 {
		t.Fatalf("expected seven statements plus begin/commit, got %d", count)
	}
	var days, associations int
	if err := f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM working_hours WHERE shop_id=$1 AND professional_id=$2), (SELECT count(*) FROM professional_services WHERE shop_id=$1 AND professional_id=$2)", f.shop, result.ID).Scan(&days, &associations); err != nil || days != 6 || associations != 2 {
		t.Fatalf("incomplete bulk setup: days=%d services=%d error=%v", days, associations, err)
	}
}
