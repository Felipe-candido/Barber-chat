//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	customerspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bookingFixture struct {
	ctx                                      context.Context
	pool                                     *pgxpool.Pool
	handler                                  http.Handler
	shop, service, longService, professional uuid.UUID
	slug, date                               string
	start                                    time.Time
}

func bindBookingQueries(q *db.Queries) booking.BookingTx {
	r := bookingpostgres.NewRepository(q)
	return booking.BookingTx{Shops: shopspostgres.NewResolver(q), Catalog: catalogpostgres.NewRepository(q), Schedule: r,
		Customers: customerspostgres.NewBookingContacts(q), Appointments: r, Idempotency: r}
}
func newBookingFixture(t *testing.T) *bookingFixture {
	t.Helper()
	dsn := requiredEnv(t, "BOOKING_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_booking_") {
		t.Fatal("BOOKING_TEST_DATABASE_URL must name a disposable codex_booking_* database with common migrations applied")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := postgres.Open(ctx, dsn, 3*time.Second)
	if err != nil {
		t.Fatal("cannot open booking test database")
	}
	t.Cleanup(pool.Close)
	f := &bookingFixture{ctx: ctx, pool: pool, shop: uuid.New(), service: uuid.New(), longService: uuid.New(), professional: uuid.New()}
	f.slug = "booking-" + f.shop.String()
	location, _ := time.LoadLocation("America/Sao_Paulo")
	tomorrow := time.Now().In(location).AddDate(0, 0, 1)
	f.start = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 9, 30, 0, 0, location).UTC()
	f.date = f.start.In(location).Format("2006-01-02")
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO shops (id,name,slug,timezone) VALUES ($1,'Booking fixture',$2,'America/Sao_Paulo')", []any{f.shop, f.slug}},
		{"INSERT INTO services (id,shop_id,name,duration_minutes,price_cents) VALUES ($1,$2,'Haircut',40,5000),($3,$2,'Long service',80,8000)", []any{f.service, f.shop, f.longService}},
		{"INSERT INTO professionals (id,shop_id,name) VALUES ($1,$2,'Test barber')", []any{f.professional, f.shop}},
		{"INSERT INTO professional_services (shop_id,professional_id,service_id) VALUES ($1,$2,$3),($1,$2,$4)", []any{f.shop, f.professional, f.service, f.longService}},
		{"INSERT INTO schedule_settings (shop_id,professional_id,interval_minutes,buffer_minutes) VALUES ($1,$2,60,0)", []any{f.shop, f.professional}},
		{"INSERT INTO working_hours (id,shop_id,professional_id,weekday,opens_at_minute,closes_at_minute) SELECT gen_random_uuid(),$1,$2,d,570,1110 FROM generate_series(0,6) d", []any{f.shop, f.professional}},
	} {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	tx := bookingpostgres.NewTransactor(pool, bindBookingQueries)
	clock := booking.ClockFunc(time.Now)
	codes := security.Codes{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := bookinghttp.NewHandler(booking.NewCreateAppointment(tx, customers.ContactNormalizer{}, codes, clock), booking.NewListAvailability(tx, clock),
		booking.NewListProfessionals(tx, clock), booking.NewManageAppointment(tx, codes, clock), 5*time.Second, logger)
	f.handler = httpapi.NewHandler(httpapi.HandlerConfig{Booking: h, FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
	return f
}
func bookingKey() string {
	var key [32]byte
	_, _ = rand.Read(key[:])
	return hex.EncodeToString(key[:])
}
func (f *bookingFixture) body(start time.Time, service uuid.UUID) map[string]any {
	return map[string]any{"service_id": service, "professional_id": f.professional, "starts_at": start.Format(time.RFC3339),
		"customer": map[string]any{"first_name": "Test", "last_name": "Customer", "phone": "(11) 91234-5678", "phone_country": "BR"}}
}
func (f *bookingFixture) request(method, path string, body any, key string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "/api/v1/public/shops/"+f.slug+path, bytes.NewReader(raw))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

type bookingHTTPResult struct {
	Code        string `json:"code"`
	Replayed    bool   `json:"replayed"`
	CanCancel   bool   `json:"can_cancel"`
	Appointment struct {
		ID          uuid.UUID `json:"id"`
		Status      string    `json:"status"`
		ServiceName string    `json:"service_name"`
		StartsAt    time.Time `json:"starts_at"`
		EndsAt      time.Time `json:"ends_at"`
		PriceCents  int64     `json:"price_cents"`
	} `json:"appointment"`
}

func bookingResult(t *testing.T, w *httptest.ResponseRecorder, want int) bookingHTTPResult {
	t.Helper()
	if w.Code != want {
		t.Fatalf("HTTP %d, want %d: %s", w.Code, want, w.Body.String())
	}
	var result bookingHTTPResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
