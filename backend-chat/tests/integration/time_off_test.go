//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
)

func timeOffFixture(t *testing.T) (*bookingFixture, *app.ManageTimeOff, uuid.UUID, uuid.UUID) {
	f := newBookingFixture(t)
	owner, barber := uuid.New(), uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO users(id,display_name) VALUES($1,'Time off owner'),($2,'Time off barber')", owner, barber); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4)", owner, f.shop, barber, f.professional); err != nil {
		t.Fatal(err)
	}
	tx := bookingpg.NewTimeOffTransactor(f.pool, func(q *db.Queries) app.TimeOffTx {
		return app.TimeOffTx{Access: identitypg.NewRepository(q), Shops: shopspg.NewResolver(q), Store: bookingpg.NewRepository(q)}
	})
	return f, app.NewManageTimeOff(tx, app.ClockFunc(time.Now)), owner, barber
}
func timeOffRouter(f *bookingFixture, uc *app.ManageTimeOff, user uuid.UUID) http.Handler {
	q := db.New(f.pool)
	identity := identitypg.NewRepository(q)
	shops := shopspg.NewResolver(q)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewHandler(httpapi.HandlerConfig{TimeOff: bookinghttp.NewTimeOffHandler(uc, 5*time.Second, logger), Authenticate: identityapp.NewAuthenticate(catalogTokenVerifier{user}, identity), AuthorizeShop: identityapp.NewAuthorizeShopAction(identity, shops), Logger: logger})
}
func timeOffRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer integration-token")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func timeOffPath(f *bookingFixture, professional, id uuid.UUID) string {
	return "/api/v1/admin/shops/" + f.slug + "/professionals/" + professional.String() + "/time-off/" + id.String()
}

func TestTimeOffHTTPBlocksAvailabilityAndReleasesDates(t *testing.T) {
	f, uc, owner, _ := timeOffFixture(t)
	h := timeOffRouter(f, uc, owner)
	id := uuid.New()
	last, _ := time.Parse("2006-01-02", f.date)
	through := last.AddDate(0, 0, 2).Format("2006-01-02")
	body := `{"from_date":"` + f.date + `","through_date":"` + through + `"}`
	path := timeOffPath(f, f.professional, id)
	for range 2 {
		w := timeOffRequest(h, "PUT", path, body)
		if w.Code != 200 {
			t.Fatalf("save %d %s", w.Code, w.Body)
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM schedule_blocks WHERE shop_id=$1", f.shop).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate block", count, err)
	}
	for _, day := range []string{f.date, through} {
		w := f.request("GET", "/availability?service_id="+f.service.String()+"&professional_id="+f.professional.String()+"&date="+day, nil, "")
		var result struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Slots) != 0 {
			t.Fatalf("blocked date: %s %d %s", day, w.Code, w.Body)
		}
	}
	if w := f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()); w.Code != 409 {
		t.Fatalf("blocked booking: %d %s", w.Code, w.Body)
	}
	if w := timeOffRequest(h, "PUT", timeOffPath(f, f.professional, uuid.New()), body); w.Code != 409 || !strings.Contains(w.Body.String(), "time_off_overlap") {
		t.Fatalf("overlap %d %s", w.Code, w.Body)
	}
	if w := timeOffRequest(h, "PUT", path, strings.Replace(body, through, last.AddDate(0, 0, 3).Format("2006-01-02"), 1)); w.Code != 409 {
		t.Fatal("PUT changed an existing resource")
	}
	for range 2 {
		if w := timeOffRequest(h, "DELETE", path, ""); w.Code != 204 {
			t.Fatalf("delete %d %s", w.Code, w.Body)
		}
	}
	bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()), 201)
}

func TestTimeOffPermissionsAndExistingAppointments(t *testing.T) {
	f, uc, owner, barber := timeOffFixture(t)
	h := timeOffRouter(f, uc, barber)
	another := uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Another barber')", another, f.shop); err != nil {
		t.Fatal(err)
	}
	body := `{"from_date":"` + f.date + `","through_date":"` + f.date + `"}`
	ownerRouter := timeOffRouter(f, uc, owner)
	ownerList := timeOffRequest(ownerRouter, "GET", "/api/v1/admin/shops/"+f.slug+"/time-off", "")
	if ownerList.Code != 200 || !bytes.Contains(ownerList.Body.Bytes(), []byte(another.String())) {
		t.Fatalf("owner choices: %d %s", ownerList.Code, ownerList.Body)
	}
	otherBlock := uuid.New()
	if w := timeOffRequest(ownerRouter, "PUT", timeOffPath(f, another, otherBlock), body); w.Code != 200 {
		t.Fatalf("owner cannot manage team: %d %s", w.Code, w.Body)
	}
	if w := timeOffRequest(h, "DELETE", timeOffPath(f, another, otherBlock), ""); w.Code != 403 {
		t.Fatal("barber removed another professional's block", w.Code)
	}
	if w := timeOffRequest(h, "PUT", timeOffPath(f, another, uuid.New()), body); w.Code != 403 {
		t.Fatalf("foreign professional %d %s", w.Code, w.Body)
	}
	w := timeOffRequest(h, "GET", "/api/v1/admin/shops/"+f.slug+"/time-off", "")
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(another.String())) || !bytes.Contains(w.Body.Bytes(), []byte(f.professional.String())) {
		t.Fatalf("list scope %d %s", w.Code, w.Body)
	}
	other := newBookingFixture(t)
	if w := timeOffRequest(h, "PUT", timeOffPath(other, other.professional, uuid.New()), body); w.Code != 403 {
		t.Fatalf("foreign shop %d %s", w.Code, w.Body)
	}
	if w := timeOffRequest(h, "PUT", timeOffPath(f, f.professional, uuid.New()), strings.TrimSuffix(body, "}")+`,"shop_id":"`+other.shop.String()+`"}`); w.Code != 400 {
		t.Fatal("accepted client tenant", w.Code)
	}
	if w := timeOffRequest(h, "PUT", timeOffPath(f, f.professional, uuid.New()), `{"from_date":"1900-01-01","through_date":"1900-01-01"}`); w.Code != 422 {
		t.Fatal("accepted past period", w.Code)
	}
	created := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()), 201)
	id := uuid.New()
	path := timeOffPath(f, f.professional, id)
	if w := timeOffRequest(h, "PUT", path, body); w.Code != 409 || !strings.Contains(w.Body.String(), "time_off_has_appointments") {
		t.Fatalf("existing booking %d %s", w.Code, w.Body)
	}
	var status string
	if err := f.pool.QueryRow(f.ctx, "SELECT status FROM appointments WHERE shop_id=$1 AND id=$2", f.shop, created.Appointment.ID).Scan(&status); err != nil || status != "scheduled" {
		t.Fatal("appointment modified", status, err)
	}
	bookingResult(t, f.request("POST", "/appointments/cancel", map[string]string{"code": created.Code}, ""), 200)
	if w := timeOffRequest(timeOffRouter(f, uc, owner), "PUT", path, body); w.Code != 200 {
		t.Fatalf("after cancellation %d %s", w.Code, w.Body)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", barber, f.shop); err != nil {
		t.Fatal(err)
	}
	if w := timeOffRequest(h, "DELETE", path, ""); w.Code != 403 {
		t.Fatal("revoked access changed calendar", w.Code)
	}
}

func TestTimeOffConcurrentBooking(t *testing.T) {
	for i := 0; i < 4; i++ {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			f, uc, owner, _ := timeOffFixture(t)
			barrier, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback(context.Background())
			if _, err = barrier.Exec(f.ctx, "SELECT id FROM professionals WHERE shop_id=$1 AND id=$2 FOR UPDATE", f.shop, f.professional); err != nil {
				t.Fatal(err)
			}
			blockResult := make(chan error, 1)
			bookingStatus := make(chan int, 1)
			go func() {
				_, err := uc.Save(f.ctx, app.StaffBookingScope{UserID: owner, ShopID: f.shop}, f.professional, uuid.New(), f.date, f.date)
				blockResult <- err
			}()
			go func() {
				bookingStatus <- f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()).Code
			}()
			waiting := 0
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LockProfessionalCalendar%'").Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting >= 2 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err = barrier.Rollback(f.ctx); err != nil {
				t.Fatal(err)
			}
			blockErr, bookingCode := <-blockResult, <-bookingStatus
			if waiting < 2 {
				t.Fatal("writers did not both contend on the calendar lock")
			}
			if !((blockErr == nil && bookingCode == 409) || (errors.Is(blockErr, app.ErrTimeOffAppointments) && bookingCode == 201)) {
				t.Fatalf("inconsistent race: block=%v booking=%d", blockErr, bookingCode)
			}
		})
	}
}
