//go:build integration

package integration

import (
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
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
)

type agendaResult struct {
	ShopTimezone string  `json:"shop_timezone"`
	NextCursor   *string `json:"next_cursor"`
	Appointments []struct {
		ID               uuid.UUID `json:"id"`
		Status           string    `json:"status"`
		ServiceName      string    `json:"service_name"`
		ProfessionalName string    `json:"professional_name"`
		CustomerName     string    `json:"customer_name"`
		CustomerPhone    string    `json:"customer_phone"`
		PriceCents       int64     `json:"price_cents"`
	} `json:"appointments"`
}

func TestAdminAgendaReadsPublicBookingAndIsolatesTenants(t *testing.T) {
	f, other := newBookingFixture(t), newBookingFixture(t)
	w := f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey())
	if w.Code != 201 {
		t.Fatal("could not create public appointment", w.Code, w.Body.String())
	}
	created := bookingResult(t, w, 201)
	if strings.Contains(w.Body.String(), "customer_phone") || strings.Contains(w.Body.String(), "Customer") {
		t.Fatal("public receipt leaked contact")
	}
	if w := other.request("POST", "/appointments", other.body(other.start, other.service), bookingKey()); w.Code != 201 {
		t.Fatal("could not create other tenant fixture")
	}
	userID := uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id,display_name) VALUES ($1,'Agenda test')", []any{userID}},
		{"INSERT INTO shop_memberships (user_id,shop_id) VALUES ($1,$2)", []any{userID, f.shop}},
		{"UPDATE services SET name='Changed catalog',price_cents=9999 WHERE id=$1", []any{f.service}},
		{"UPDATE professionals SET name='Changed barber' WHERE id=$1", []any{f.professional}},
	} {
		if _, err := f.pool.Exec(f.ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(f.pool)
	shops, identities := shopspostgres.NewResolver(q), identitypostgres.NewRepository(q)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := httpapi.NewHandler(httpapi.HandlerConfig{
		Agenda:       bookinghttp.NewAgendaHandler(booking.NewListAgenda(shops, bookingpostgres.NewRepository(q), booking.ClockFunc(time.Now)), 5*time.Second, logger),
		Authenticate: identity.NewAuthenticate(catalogTokenVerifier{userID}, identities), AuthorizeShop: identity.NewAuthorizeShopAction(identities, shops), Logger: logger,
	})
	request := func(slug, query string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/shops/"+slug+"/appointments"+query, nil)
		if authenticated {
			r.Header.Set("Authorization", "Bearer integration-token")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	query := "?month=" + f.date[:7]
	if w := request(f.slug, query, false); w.Code != 401 {
		t.Fatal("unauthenticated agenda accepted", w.Code)
	}
	if w := request(other.slug, query, true); w.Code != 403 {
		t.Fatal("foreign tenant accepted", w.Code)
	}
	for _, bad := range []string{"?shop_id=" + other.shop.String(), "?month=2026-13", "?after=broken", "?month=2026-10&month=2026-11"} {
		if w := request(f.slug, bad, true); w.Code != 422 {
			t.Fatal("invalid input accepted", bad, w.Code)
		}
	}
	w = request(f.slug, query, true)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("agenda read failed", w.Code, w.Body.String())
	}
	var result agendaResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Appointments) != 1 || result.ShopTimezone != "America/Sao_Paulo" {
		t.Fatal("tenant agenda mismatch", w.Body.String())
	}
	a := result.Appointments[0]
	if a.ID != created.Appointment.ID || a.CustomerName != "Test Customer" || a.CustomerPhone != "+5511912345678" || a.ServiceName != "Haircut" || a.ProfessionalName != "Test barber" || a.PriceCents != 5000 {
		t.Fatal("private projection or snapshots incorrect", a)
	}
	if strings.Contains(w.Body.String(), created.Code) || strings.Contains(w.Body.String(), "code_hash") {
		t.Fatal("agenda exposed booking capability")
	}
	if w := f.request("POST", "/appointments/cancel", map[string]string{"code": created.Code}, ""); w.Code != 200 {
		t.Fatal("cancel failed", w.Code)
	}
	w = request(f.slug, query, true)
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Appointments[0].Status != "cancelled" {
		t.Fatal("cancellation missing from agenda", w.Body.String())
	}
	// Equal timestamps exercise the UUID tie-breaker across page boundaries.
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO appointments
		(id,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,created_at,updated_at,code_hash)
		SELECT gen_random_uuid(),shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,'cancelled',1,created_at,updated_at,decode(md5(gen_random_uuid()::text)||md5(gen_random_uuid()::text),'hex')
		FROM appointments CROSS JOIN generate_series(1,100) WHERE id=$1`, created.Appointment.ID); err != nil {
		t.Fatal(err)
	}
	w = request(f.slug, query, true)
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Appointments) != 100 || result.NextCursor == nil {
		t.Fatal("first page not bounded", w.Body.String())
	}
	seen := make(map[uuid.UUID]bool)
	for _, item := range result.Appointments {
		seen[item.ID] = true
	}
	w = request(f.slug, query+"&after="+url.QueryEscape(*result.NextCursor), true)
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Appointments) != 1 || result.NextCursor != nil || seen[result.Appointments[0].ID] {
		t.Fatal("keyset pagination lost or duplicated reservations", w.Body.String())
	}
	if w := request(f.slug, "?month=2000-01", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"appointments":[]`) {
		t.Fatal("empty agenda contract failed", w.Code, w.Body.String())
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", userID, f.shop); err != nil {
		t.Fatal(err)
	}
	if w := request(f.slug, query, true); w.Code != 403 {
		t.Fatal("inactive membership accepted", w.Code)
	}
}
