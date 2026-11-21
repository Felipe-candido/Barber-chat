//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
)

func professionalEditBindings(q *db.Queries) catalog.ProfessionalTx {
	return catalog.ProfessionalTx{Management: catalogpg.NewRepository(q), Calendar: bookingpg.NewRepository(q), Owner: identitypg.NewRepository(q)}
}
func professionalEditor(f *bookingFixture) *catalog.ManageProfessionals {
	return catalog.NewManageProfessionals(catalogpg.NewProfessionalTransactor(f.pool, professionalEditBindings))
}
func professionalEditRouter(f *bookingFixture, uc *catalog.ManageProfessionals, user uuid.UUID) http.Handler {
	q := db.New(f.pool)
	identity := identitypg.NewRepository(q)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewHandler(httpapi.HandlerConfig{ProfessionalManagement: cataloghttp.NewProfessionalManagementHandler(uc, 5*time.Second, logger), Authenticate: identityapp.NewAuthenticate(catalogTokenVerifier{user}, identity), AuthorizeShop: identityapp.NewAuthorizeShopAction(identity, shopspg.NewResolver(q)), Logger: logger})
}
func professionalEditPath(f *bookingFixture, id uuid.UUID) string {
	return "/api/v1/admin/shops/" + f.slug + "/professionals/" + id.String()
}
func professionalEditPayload(p catalog.ProfessionalDetail) string {
	hours := make([]map[string]int, 0, len(p.Schedule.WorkingHours))
	for _, w := range p.Schedule.WorkingHours {
		hours = append(hours, map[string]int{"weekday": w.Weekday, "opens_at_minute": w.OpensAtMinute, "closes_at_minute": w.ClosesAtMinute})
	}
	b, _ := json.Marshal(map[string]any{"name": p.Name, "active": p.Active, "service_ids": p.ServiceIDs, "revision": p.Revision, "schedule": map[string]any{"interval_minutes": p.Schedule.IntervalMinutes, "buffer_minutes": p.Schedule.BufferMinutes, "working_hours": hours}})
	return string(b)
}

func TestProfessionalEditPreservesAppointmentsAndTimeOff(t *testing.T) {
	f, timeOff, owner, _ := timeOffFixture(t)
	uc := professionalEditor(f)
	scope := catalog.ProfessionalEditScope{ActorID: owner, ShopID: f.shop}
	h := professionalEditRouter(f, uc, owner)
	created := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()), 201)
	day, _ := time.Parse("2006-01-02", f.date)
	offDate := day.AddDate(0, 0, 3).Format("2006-01-02")
	offID := uuid.New()
	if w := timeOffRequest(timeOffRouter(f, timeOff, owner), "PUT", timeOffPath(f, f.professional, offID), `{"from_date":"`+offDate+`","through_date":"`+offDate+`"}`); w.Code != 200 {
		t.Fatal("time off fixture", w.Code, w.Body)
	}
	added := uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO services(id,shop_id,name,duration_minutes,price_cents) VALUES($1,$2,'Beard',30,3000)", added, f.shop); err != nil {
		t.Fatal(err)
	}
	p, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	weekday := int(f.start.In(loc).Weekday())
	p.Name = "Updated barber"
	p.ServiceIDs = []uuid.UUID{added, f.longService}
	p.Schedule = catalog.ProfessionalSchedule{IntervalMinutes: 30, BufferMinutes: 5, WorkingHours: []catalog.WorkingPeriod{{Weekday: weekday, OpensAtMinute: 600, ClosesAtMinute: 720}, {Weekday: weekday, OpensAtMinute: 780, ClosesAtMinute: 1020}}}
	path := professionalEditPath(f, f.professional)
	body := professionalEditPayload(p)
	for range 2 {
		if w := timeOffRequest(h, "PUT", path, body); w.Code != 200 {
			t.Fatalf("save/replay %d %s", w.Code, w.Body)
		}
	}
	current, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil || current.Name != p.Name || len(current.ServiceIDs) != 2 || len(current.Schedule.WorkingHours) != 2 || current.Schedule.IntervalMinutes != 30 {
		t.Fatalf("not persisted: %+v %v", current, err)
	}
	details := bookingResult(t, f.request("POST", "/appointments/lookup", map[string]string{"code": created.Code}, ""), 200)
	if details.Appointment != created.Appointment {
		t.Fatal("editing changed an existing booking snapshot")
	}
	var offCount int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM schedule_blocks WHERE shop_id=$1 AND id=$2", f.shop, offID).Scan(&offCount); err != nil || offCount != 1 {
		t.Fatal("time off removed", err)
	}
	w := f.request("GET", "/availability?service_id="+added.String()+"&professional_id="+f.professional.String()+"&date="+f.date, nil, "")
	var available struct {
		Slots []struct {
			StartsAt time.Time `json:"starts_at"`
			EndsAt   time.Time `json:"ends_at"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &available); err != nil || w.Code != 200 || len(available.Slots) == 0 {
		t.Fatalf("new service/scale %d %s", w.Code, w.Body)
	}
	for _, slot := range available.Slots {
		start, end := slot.StartsAt.In(loc), slot.EndsAt.In(loc)
		a, b := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
		if !((a >= 600 && b <= 720) || (a >= 780 && b <= 1020)) {
			t.Fatal("lunch break or weekly hours ignored", slot)
		}
	}
	if w := f.request("POST", "/appointments", f.body(f.start.Add(3*time.Hour), f.service), bookingKey()); w.Code != 404 {
		t.Fatal("removed service still offered", w.Code, w.Body)
	}
	current.ServiceIDs = []uuid.UUID{}
	empty, err := uc.Update(f.ctx, scope, current)
	if err != nil || len(empty.ServiceIDs) != 0 {
		t.Fatal("cannot remove all services", err)
	}
	empty.Active = false
	paused, err := uc.Update(f.ctx, scope, empty)
	if err != nil || paused.Active {
		t.Fatal("cannot pause professional", err)
	}
	paused.Active = true
	if _, err = uc.Update(f.ctx, scope, paused); err != nil {
		t.Fatal("cannot reactivate", err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE services SET active=false WHERE id=$1", f.longService); err != nil {
		t.Fatal(err)
	}
}

func TestProfessionalEditPermissionsValidationAndRollback(t *testing.T) {
	f, _, owner, barber := timeOffFixture(t)
	uc := professionalEditor(f)
	scope := catalog.ProfessionalEditScope{ActorID: owner, ShopID: f.shop}
	h := professionalEditRouter(f, uc, owner)
	path := professionalEditPath(f, f.professional)
	original, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PUT"} {
		if w := timeOffRequest(professionalEditRouter(f, uc, barber), method, path, professionalEditPayload(original)); w.Code != 403 {
			t.Fatal("barber edited team", w.Code)
		}
	}
	other := newBookingFixture(t)
	if w := timeOffRequest(h, "GET", professionalEditPath(other, other.professional), ""); w.Code != 403 {
		t.Fatal("foreign shop access", w.Code)
	}
	if w := timeOffRequest(h, "GET", professionalEditPath(f, other.professional), ""); w.Code != 404 {
		t.Fatal("foreign ID exposed", w.Code)
	}
	if w := timeOffRequest(h, "PUT", path, strings.TrimSuffix(professionalEditPayload(original), "}")+`,"shop_id":"`+other.shop.String()+`"}`); w.Code != 400 {
		t.Fatal("client tenant accepted", w.Code)
	}
	invalid := original
	invalid.Name = "Should roll back"
	invalid.ServiceIDs = []uuid.UUID{other.service}
	if w := timeOffRequest(h, "PUT", path, professionalEditPayload(invalid)); w.Code != 422 {
		t.Fatalf("foreign service %d %s", w.Code, w.Body)
	}
	invalid = original
	invalid.Schedule.WorkingHours = []catalog.WorkingPeriod{{Weekday: 1, OpensAtMinute: 600, ClosesAtMinute: 800}, {Weekday: 1, OpensAtMinute: 700, ClosesAtMinute: 900}}
	if w := timeOffRequest(h, "PUT", path, professionalEditPayload(invalid)); w.Code != 422 {
		t.Fatal("overlapping periods accepted", w.Code)
	}
	for _, body := range []string{`null`, `{"name":"Test"}`, strings.Replace(professionalEditPayload(original), `"active":true`, `"active":null`, 1)} {
		if w := timeOffRequest(h, "PUT", path, body); w.Code != 400 && w.Code != 422 {
			t.Fatal("incomplete edit accepted", w.Code, w.Body)
		}
	}
	// Force failure after both profile and calendar writes to verify transaction rollback.
	failing := catalog.NewManageProfessionals(catalogpg.NewProfessionalTransactor(f.pool, func(q *db.Queries) catalog.ProfessionalTx {
		tx := professionalEditBindings(q)
		tx.Calendar = failingProfessionalCalendar{tx.Calendar}
		return tx
	}))
	changed := original
	changed.Name = "Rollback"
	changed.ServiceIDs = []uuid.UUID{f.longService}
	changed.Schedule.WorkingHours = []catalog.WorkingPeriod{{Weekday: 1, OpensAtMinute: 600, ClosesAtMinute: 900}}
	if _, err = failing.Update(f.ctx, scope, changed); err == nil {
		t.Fatal("injected failure ignored")
	}
	after, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil || after.Revision != original.Revision {
		t.Fatal("failed edit left partial changes", err)
	}
	// Inactive, previously linked services can be retained but cannot be newly assigned.
	if _, err = f.pool.Exec(f.ctx, "UPDATE services SET active=false WHERE id=$1", f.service); err != nil {
		t.Fatal(err)
	}
	after.Name = "Retained inactive assignment"
	after, err = uc.Update(f.ctx, scope, after)
	if err != nil {
		t.Fatal("cannot preserve linked inactive service", err)
	}
	after.ServiceIDs = []uuid.UUID{f.longService}
	after, err = uc.Update(f.ctx, scope, after)
	if err != nil {
		t.Fatal(err)
	}
	after.ServiceIDs = append(after.ServiceIDs, f.service)
	if _, err = uc.Update(f.ctx, scope, after); !errors.Is(err, catalog.ErrInvalidProfessionalServices) {
		t.Fatal("new inactive assignment accepted", err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, f.shop); err != nil {
		t.Fatal(err)
	}
	if _, err = uc.Update(f.ctx, scope, original); !errors.Is(err, catalog.ErrProfessionalAccessDenied) {
		t.Fatal("revocation not rechecked inside transaction", err)
	}
}

type failingProfessionalCalendar struct {
	catalog.ProfessionalCalendarEditor
}

func (c failingProfessionalCalendar) ReplaceProfessionalSchedule(ctx context.Context, shop, id uuid.UUID, s catalog.ProfessionalSchedule) error {
	if err := c.ProfessionalCalendarEditor.ReplaceProfessionalSchedule(ctx, shop, id, s); err != nil {
		return err
	}
	return errors.New("injected calendar failure")
}

func TestProfessionalEditConcurrentChanges(t *testing.T) {
	f, _, owner, _ := timeOffFixture(t)
	uc := professionalEditor(f)
	scope := catalog.ProfessionalEditScope{ActorID: owner, ShopID: f.shop}
	original, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err = barrier.Exec(f.ctx, "SELECT id FROM professionals WHERE shop_id=$1 AND id=$2 FOR UPDATE", f.shop, f.professional); err != nil {
		t.Fatal(err)
	}
	type result struct {
		desired catalog.ProfessionalDetail
		err     error
	}
	results := make(chan result, 2)
	for _, name := range []string{"Concurrent A", "Concurrent B"} {
		go func(name string) {
			p := original
			p.Name = name
			_, err := uc.Update(f.ctx, scope, p)
			results <- result{p, err}
		}(name)
	}
	waiting := 0
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LockEditableProfessional%'").Scan(&waiting); err != nil {
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
	a, b := <-results, <-results
	if waiting < 2 {
		t.Fatal("edits did not contend on the same professional lock")
	}
	if a.err != nil {
		a, b = b, a
	}
	if a.err != nil || !errors.Is(b.err, catalog.ErrProfessionalChanged) {
		t.Fatalf("lost update not prevented: %v / %v", a.err, b.err)
	}
	live, err := uc.Get(f.ctx, scope, f.professional)
	if err != nil || live.Name != a.desired.Name {
		t.Fatal("winner not persisted", err)
	}
	if recovered, err := uc.Update(f.ctx, scope, a.desired); err != nil || recovered.Revision != live.Revision {
		t.Fatal("identical retry did not recover saved state", err)
	}
}
