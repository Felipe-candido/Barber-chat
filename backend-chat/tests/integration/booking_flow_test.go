//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestBookingHTTPFlow(t *testing.T) {
	f := newBookingFixture(t)
	key := bookingKey()
	body := f.body(f.start, f.service)
	created := bookingResult(t, f.request("POST", "/appointments", body, key), 201)
	if created.Code == "" || created.Appointment.ID == uuid.Nil || created.Appointment.EndsAt.Sub(created.Appointment.StartsAt) != time.Hour {
		t.Fatalf("invalid receipt: %+v", created)
	}
	replayed := bookingResult(t, f.request("POST", "/appointments", body, key), 200)
	if !replayed.Replayed || replayed.Code != created.Code || replayed.Appointment != created.Appointment {
		t.Fatal("retry changed the receipt")
	}
	conflict := f.request("POST", "/appointments", f.body(f.start.Add(time.Hour), f.service), key)
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
		t.Fatal("different request reused a key")
	}
	lookup := f.request("POST", "/appointments/lookup", map[string]string{"code": strings.ToLower(created.Code)}, "")
	details := bookingResult(t, lookup, 200)
	if !details.CanCancel || details.Appointment.ID != created.Appointment.ID {
		t.Fatal("lookup mismatch")
	}
	if strings.Contains(lookup.Body.String(), "phone") || strings.Contains(lookup.Body.String(), "Customer") {
		t.Fatal("public receipt exposed contact data")
	}
	other := newBookingFixture(t)
	if other.request("POST", "/appointments/lookup", map[string]string{"code": created.Code}, "").Code != 404 {
		t.Fatal("code crossed tenant boundary")
	}
	if f.request("POST", "/appointments/lookup", map[string]string{"code": created.Appointment.ID.String()}, "").Code != 404 {
		t.Fatal("appointment ID is acting as a code")
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE services SET name='Changed',price_cents=9999,duration_minutes=80 WHERE shop_id=$1 AND id=$2", f.shop, f.service); err != nil {
		t.Fatal(err)
	}
	details = bookingResult(t, f.request("POST", "/appointments/lookup", map[string]string{"code": created.Code}, ""), 200)
	if details.Appointment.ServiceName != "Haircut" || details.Appointment.PriceCents != 5000 {
		t.Fatal("catalog change rewrote appointment history")
	}
	for range 2 {
		cancelled := bookingResult(t, f.request("POST", "/appointments/cancel", map[string]string{"code": created.Code}, ""), 200)
		if cancelled.CanCancel || cancelled.Appointment.Status != "cancelled" {
			t.Fatal("cancel failed")
		}
	}
	var version int64
	if err := f.pool.QueryRow(f.ctx, "SELECT version FROM appointments WHERE shop_id=$1 AND id=$2", f.shop, created.Appointment.ID).Scan(&version); err != nil || version != 2 {
		t.Fatalf("non-idempotent cancel: %d %v", version, err)
	}
	// Only cancellation releases the reserved interval.
	bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.longService), bookingKey()), 201)
	keyHash := sha256.Sum256([]byte(key))
	var encrypted []byte
	if err := f.pool.QueryRow(f.ctx, "SELECT encrypted_receipt FROM booking_requests WHERE shop_id=$1 AND key_hash=$2", f.shop, keyHash[:]).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), created.Code) {
		t.Fatal("plaintext capability stored in idempotency receipt")
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE booking_requests SET expires_at=now()-interval '1 second' WHERE shop_id=$1 AND key_hash=$2", f.shop, keyHash[:]); err != nil {
		t.Fatal(err)
	}
	if w := f.request("POST", "/appointments", body, key); w.Code != 409 || !strings.Contains(w.Body.String(), "idempotency_expired") {
		t.Fatal("expired request can be replayed/reused")
	}
}

func TestBookingAvailabilityAndFullInterval(t *testing.T) {
	f := newBookingFixture(t)
	professionals := f.request("GET", "/professionals?service_id="+f.service.String(), nil, "")
	if professionals.Code != 200 || !strings.Contains(professionals.Body.String(), f.professional.String()) {
		t.Fatal("professionals unavailable")
	}
	path := "/availability?service_id=" + f.service.String() + "&professional_id=" + f.professional.String() + "&date=" + f.date
	w := f.request("GET", path, nil, "")
	var before struct {
		Slots []booking.AvailabilitySlot `json:"-"`
		Raw   []struct {
			StartsAt time.Time `json:"starts_at"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &before); err != nil || w.Code != 200 || len(before.Raw) != 9 {
		t.Fatalf("availability: %s %v", w.Body.String(), err)
	}
	long := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.longService), bookingKey()), 201)
	if long.Appointment.EndsAt.Sub(long.Appointment.StartsAt) != 2*time.Hour {
		t.Fatal("long service did not reserve two blocks")
	}
	if w := f.request("POST", "/appointments", f.body(f.start.Add(time.Hour), f.service), bookingKey()); w.Code != 409 {
		t.Fatalf("second block accepted: %s", w.Body.String())
	}
	bookingResult(t, f.request("POST", "/appointments", f.body(f.start.Add(2*time.Hour), f.service), bookingKey()), 201)
	if f.request("POST", "/appointments", f.body(f.start.Add(3*time.Hour+time.Second), f.service), bookingKey()).Code != 422 {
		t.Fatal("seconds were silently truncated")
	}
	if f.request("POST", "/appointments", f.body(f.start.Add(3*time.Hour+30*time.Minute), f.service), bookingKey()).Code != 409 {
		t.Fatal("off-grid start accepted")
	}
	if f.request("POST", "/appointments", f.body(f.start.Add(8*time.Hour), f.longService), bookingKey()).Code != 409 {
		t.Fatal("reservation extends past closing")
	}
	other := newBookingFixture(t)
	if f.request("POST", "/appointments", f.body(f.start.Add(4*time.Hour), other.service), bookingKey()).Code != 404 {
		t.Fatal("foreign service booked")
	}
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO professional_services (shop_id,professional_id,service_id) VALUES ($1,$2,$3)", f.shop, f.professional, other.service); err == nil {
		t.Fatal("database accepted cross-tenant service association")
	}
	_, err := f.pool.Exec(f.ctx, `INSERT INTO appointments (id,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,code_hash,created_at,updated_at)
SELECT $1,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,$2,created_at,updated_at FROM appointments WHERE shop_id=$3 AND id=$4`, uuid.New(), make([]byte, 32), f.shop, long.Appointment.ID)
	var constraint *pgconn.PgError
	if !errors.As(err, &constraint) || constraint.Code != "23P01" || constraint.ConstraintName != "appointments_no_overlap" {
		t.Fatalf("direct SQL overlap not rejected by exclusion constraint: %v", err)
	}
}

type failingAppointmentWriter struct{ booking.AppointmentRepository }

func (failingAppointmentWriter) Insert(context.Context, domain.Appointment, [32]byte) (domain.Appointment, error) {
	return domain.Appointment{}, errors.New("injected write failure")
}
func TestBookingRollbackAndPrivacy(t *testing.T) {
	f := newBookingFixture(t)
	tx := bookingpostgres.NewTransactor(f.pool, func(q *db.Queries) booking.BookingTx {
		ports := bindBookingQueries(q)
		ports.Appointments = failingAppointmentWriter{ports.Appointments}
		return ports
	})
	uc := booking.NewCreateAppointment(tx, customers.ContactNormalizer{}, security.Codes{}, booking.ClockFunc(time.Now))
	_, err := uc.Execute(f.ctx, booking.CreateAppointmentInput{ShopSlug: f.slug, ServiceID: f.service, ProfessionalID: f.professional, StartsAt: f.start, IdempotencyKey: bookingKey(), Customer: booking.CustomerInput{FirstName: "Test", LastName: "Rollback", Phone: "11912345678", PhoneCountry: "BR"}})
	if err == nil {
		t.Fatal("injected failure ignored")
	}
	for _, table := range []string{"customers", "appointments", "booking_requests"} {
		var count int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE shop_id=$1", f.shop).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial commit in %s: %d %v", table, count, err)
		}
	}
	for _, table := range []string{"shops", "services", "professionals", "professional_services", "schedule_settings", "working_hours", "schedule_blocks", "customers", "appointments", "booking_requests"} {
		var protected bool
		if err := f.pool.QueryRow(f.ctx, "SELECT relrowsecurity FROM pg_class WHERE oid=$1::regclass", "public."+table).Scan(&protected); err != nil || !protected {
			t.Fatalf("RLS missing on %s: %v", table, err)
		}
	}
}

func TestBookingHTTPInputBoundaries(t *testing.T) {
	f := newBookingFixture(t)
	body := f.body(f.start, f.service)
	body["shop_id"] = f.shop
	if f.request("POST", "/appointments", body, bookingKey()).Code != 400 {
		t.Fatal("client tenant accepted")
	}
	if f.request("POST", "/appointments", f.body(f.start, f.service), "").Code != 422 {
		t.Fatal("request key not required")
	}
	r := httptest.NewRequest("OPTIONS", "/api/v1/public/shops/"+f.slug+"/appointments", nil)
	r.Header.Set("Origin", "http://127.0.0.1:3000")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "Content-Type, Idempotency-Key")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatal("idempotency CORS preflight rejected")
	}
	for i := 0; i < 21; i++ {
		w = f.request("POST", "/appointments/lookup", map[string]string{"code": "invalid"}, "")
	}
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("public request limiter ineffective")
	}
}

func TestBookingManualProvisioning(t *testing.T) {
	f := newBookingFixture(t)
	script, err := os.ReadFile("../../db/seeds/provision_booking_calendar.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ReplaceAll(string(script), "'ALTERE-O-SLUG';", "'"+f.slug+"';")
	sql = strings.ReplaceAll(sql, "'ALTERE-O-NOME';", "'Manual fixture';")
	for range 2 {
		if _, err := f.pool.Exec(f.ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM professionals WHERE shop_id=$1 AND name='Manual fixture'", f.shop).Scan(&count); err != nil || count != 1 {
		t.Fatalf("provision not idempotent: %d %v", count, err)
	}
}
