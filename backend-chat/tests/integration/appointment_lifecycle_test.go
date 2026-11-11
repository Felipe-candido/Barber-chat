//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failLifecycleSave struct {
	booking.AppointmentLifecycleStore
}

func (s failLifecycleSave) SaveStaffStatus(ctx context.Context, a domain.Lifecycle, v int64) (bool, error) {
	if _, err := s.AppointmentLifecycleStore.SaveStaffStatus(ctx, a, v); err != nil {
		return false, err
	}
	return false, errors.New("injected failure after status and audit")
}

func TestAppointmentLifecycle(t *testing.T) {
	dsn := requiredEnv(t, "APPOINTMENT_LIFECYCLE_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_lifecycle_") {
		t.Fatal("disposable codex_lifecycle_ database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var auth bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name='auth')").Scan(&auth); err != nil || auth {
		t.Fatal("refusing auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := func() time.Time {
		t.Helper()
		var at time.Time
		if err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at.UTC()
	}
	key := func() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
	owner, barber, barberB, outsider := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	shop, other, profA, profB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	service, customer := uuid.New(), uuid.New()
	slug := "lifecycle-" + shop.String()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber A'),($3,'Barber B'),($4,'Outsider')", owner, barber, barberB, outsider)
	exec("INSERT INTO shops(id,name,slug) VALUES($1,'Lifecycle',$2),($3,'Other',$4)", shop, slug, other, "lifecycle-"+other.String())
	exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'A'),($3,$2,'B')", profA, shop, profB)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$2,'barber',$6),($7,$8,'owner',NULL)", owner, shop, barber, profA, barberB, profB, outsider, other)
	exec("INSERT INTO services(id,shop_id,name,duration_minutes,price_cents,currency) VALUES($1,$2,'Cut',40,5000,'BRL')", service, shop)
	exec("INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Test','Customer','+5511912345678',now())", customer, shop)
	freshProfessional := func() uuid.UUID {
		t.Helper()
		id := uuid.New()
		exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Fixture')", id, shop)
		return id
	}
	seed := func(prof uuid.UUID, end time.Time) (uuid.UUID, string) {
		t.Helper()
		id := uuid.New()
		code, hash, err := (security.Codes{}).New()
		if err != nil {
			t.Fatal(err)
		}
		start := end.Add(-time.Hour)
		created := start.Add(-time.Hour)
		if at := now(); created.After(at) {
			created = at.Add(-time.Minute)
		}
		exec("INSERT INTO appointments(id,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,code_hash,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'Cut',40,5000,'BRL','Snapshot','America/Sao_Paulo',$6,$7,'scheduled',1,$8,$9,$9)", id, shop, customer, service, prof, start, end, hash[:], created)
		return id, code
	}
	bind := func(q *db.Queries) booking.AppointmentLifecycleTx {
		return booking.AppointmentLifecycleTx{Access: identitypg.NewRepository(q), Store: bookingpg.NewLifecycleStore(q)}
	}
	tx := bookingpg.NewLifecycleTransactor(pool, bind)
	manage := booking.NewManageAppointmentStatus(tx)
	due := booking.NewAutoCompleteAppointments(bookingpg.NewLifecycleStore(db.New(pool)))
	scope := booking.LifecycleScope{ActorID: owner, ShopID: shop}
	input := func(id uuid.UUID, status domain.Status) booking.ChangeAppointmentStatusInput {
		return booking.ChangeAppointmentStatusInput{Scope: scope, AppointmentID: id, Status: status, ExpectedVersion: 1, Reason: "Correct record", IdempotencyKey: key()}
	}
	detail := func(id uuid.UUID) booking.AppointmentLifecycleOutput {
		t.Helper()
		out, err := manage.Detail(ctx, scope, id)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("automatic_completion_is_persisted_once_and_excludes_future_and_old_history", func(t *testing.T) {
		past, _ := seed(freshProfessional(), now())
		future, _ := seed(freshProfessional(), now().Add(2*time.Hour))
		legacy, _ := seed(freshProfessional(), now().Add(-48*time.Hour))
		if n, err := due.Execute(ctx); err != nil || n < 1 {
			t.Fatal(n, err)
		}
		out := detail(past)
		if out.Appointment.Appointment.Status != domain.StatusCompleted || out.Appointment.StatusSource != domain.SourceAutomatic || out.Appointment.Version != 2 || len(out.History) != 1 || out.History[0].ActorID != uuid.Nil {
			t.Fatal(out)
		}
		if n, err := due.Execute(ctx); err != nil || n != 0 {
			t.Fatal("job repeated a completion", n, err)
		}
		for _, id := range []uuid.UUID{future, legacy} {
			if out := detail(id); out.Appointment.Appointment.Status != domain.StatusScheduled || len(out.History) != 0 {
				t.Fatal("ineligible appointment changed", out)
			}
		}
	})
	t.Run("owner_manages_every_professional_barber_only_own_and_current_access", func(t *testing.T) {
		a, _ := seed(profA, now())
		b, _ := seed(profB, now())
		barberScope := booking.LifecycleScope{ActorID: barber, ShopID: shop}
		if _, err := manage.Detail(ctx, barberScope, b); !errors.Is(err, booking.ErrAppointmentAccessDenied) {
			t.Fatal("barber read other professional", err)
		}
		in := input(b, domain.StatusNoShow)
		in.Scope = barberScope
		if _, err := manage.Change(ctx, in); !errors.Is(err, booking.ErrAppointmentAccessDenied) {
			t.Fatal("barber changed other professional", err)
		}
		in = input(a, domain.StatusNoShow)
		in.Scope = barberScope
		if _, err := manage.Change(ctx, in); err != nil {
			t.Fatal(err)
		}
		if _, err := manage.Change(ctx, input(b, domain.StatusNoShow)); err != nil {
			t.Fatal("main account cannot manage another barber", err)
		}
		exec("UPDATE shop_memberships SET active=FALSE WHERE user_id=$1 AND shop_id=$2", barber, shop)
		_, err := manage.Change(ctx, in)
		exec("UPDATE shop_memberships SET active=TRUE WHERE user_id=$1 AND shop_id=$2", barber, shop)
		if !errors.Is(err, booking.ErrAppointmentAccessDenied) {
			t.Fatal("revoked access replayed", err)
		}
		if _, err := manage.Detail(ctx, booking.LifecycleScope{ActorID: outsider, ShopID: other}, a); !errors.Is(err, booking.ErrAppointmentNotFound) {
			t.Fatal("foreign tenant read", err)
		}
	})
	t.Run("two_automatic_executors_do_not_duplicate_completion", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		start := make(chan struct{})
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() { <-start; _, err := due.Execute(ctx); results <- err }()
		}
		close(start)
		for i := 0; i < 2; i++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		out := detail(id)
		if out.Appointment.Version != 2 || out.Appointment.StatusSource != domain.SourceAutomatic || len(out.History) != 1 {
			t.Fatal("automatic completion duplicated", out)
		}
	})
	t.Run("manual_exception_is_never_overwritten_and_automatic_result_can_be_corrected", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		if _, err := due.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		in := input(id, domain.StatusNoShow)
		in.ExpectedVersion = 2
		out, err := manage.Change(ctx, in)
		if err != nil || out.Appointment.Version != 3 || out.Appointment.StatusActorID != owner || len(out.History) != 2 {
			t.Fatal(out, err)
		}
		if _, err := due.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		if out := detail(id); out.Appointment.Appointment.Status != domain.StatusNoShow || out.Appointment.Version != 3 {
			t.Fatal(out)
		}
		in.ExpectedVersion = 3
		in.Status = domain.StatusCompleted
		in.IdempotencyKey = key()
		if _, err := manage.Change(ctx, in); err != nil {
			t.Fatal("correction cannot be undone", err)
		}
	})
	t.Run("identical_and_concurrent_commands_keep_one_audit_and_replay_current_state", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		in := input(id, domain.StatusNoShow)
		type result struct {
			out booking.AppointmentLifecycleOutput
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() { <-start; out, err := manage.Change(ctx, in); results <- result{out, err} }()
		}
		close(start)
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.out.Replayed == b.out.Replayed || len(detail(id).History) != 1 {
			t.Fatal(a, b)
		}
		change := input(id, domain.StatusCancelled)
		change.ExpectedVersion = 2
		if _, err := manage.Change(ctx, change); err != nil {
			t.Fatal(err)
		}
		replay, err := manage.Change(ctx, in)
		if err != nil || !replay.Replayed || replay.ResultVersion != 2 || replay.Appointment.Version != 3 || replay.Appointment.Appointment.Status != domain.StatusCancelled {
			t.Fatal(replay, err)
		}
		in.Reason = "Different"
		if _, err := manage.Change(ctx, in); !errors.Is(err, booking.ErrStatusCommandConflict) {
			t.Fatal(err)
		}
	})
	t.Run("automatic_and_manual_updates_race_without_overwriting_the_exception", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		in := input(id, domain.StatusNoShow)
		start := make(chan struct{})
		auto := make(chan error, 1)
		manual := make(chan error, 1)
		go func() { <-start; _, err := due.Execute(ctx); auto <- err }()
		go func() { <-start; _, err := manage.Change(ctx, in); manual <- err }()
		close(start)
		if err := <-auto; err != nil {
			t.Fatal(err)
		}
		if err := <-manual; err != nil {
			if !errors.Is(err, domain.ErrAppointmentVersionConflict) {
				t.Fatal(err)
			}
			in.ExpectedVersion = detail(id).Appointment.Version
			if _, err := manage.Change(ctx, in); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := due.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		out := detail(id)
		if out.Appointment.Appointment.Status != domain.StatusNoShow {
			t.Fatal(out)
		}
	})
	t.Run("rollback_covers_status_audit_and_command", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		in := input(id, domain.StatusNoShow)
		badTx := bookingpg.NewLifecycleTransactor(pool, func(q *db.Queries) booking.AppointmentLifecycleTx {
			b := bind(q)
			b.Store = failLifecycleSave{b.Store}
			return b
		})
		if _, err := booking.NewManageAppointmentStatus(badTx).Change(ctx, in); err == nil {
			t.Fatal("partial status change accepted")
		}
		if out := detail(id); out.Appointment.Version != 1 || len(out.History) != 0 {
			t.Fatal("rollback failed", out)
		}
		if out, err := manage.Change(ctx, in); err != nil || out.Replayed {
			t.Fatal("failed claim survived", out, err)
		}
	})
	t.Run("early_transitions_stale_versions_and_reoccupied_cancelled_intervals_conflict", func(t *testing.T) {
		prof := freshProfessional()
		end := now().Add(3 * time.Hour)
		id, _ := seed(prof, end)
		for _, status := range []domain.Status{domain.StatusNoShow, domain.StatusCompleted} {
			if _, err := manage.Change(ctx, input(id, status)); !errors.Is(err, domain.ErrAppointmentTransitionTooEarly) {
				t.Fatal(err)
			}
		}
		if _, err := manage.Change(ctx, input(id, domain.StatusCancelled)); err != nil {
			t.Fatal(err)
		}
		// A replacement appointment can occupy a cancelled interval. Restoring
		// history must retain the existing exclusion constraint, never bypass it.
		pastProf := freshProfessional()
		pastEnd := now()
		old, _ := seed(pastProf, pastEnd)
		if _, err := manage.Change(ctx, input(old, domain.StatusCancelled)); err != nil {
			t.Fatal(err)
		}
		_, _ = seed(pastProf, pastEnd)
		restore := input(old, domain.StatusCompleted)
		restore.ExpectedVersion = 2
		if _, err := manage.Change(ctx, restore); !errors.Is(err, booking.ErrSlotUnavailable) {
			t.Fatal("restored occupied interval", err)
		}
		if detail(old).Appointment.Appointment.Status != domain.StatusCancelled {
			t.Fatal("failed restore changed status")
		}
	})
	t.Run("inactive_professional_and_unpaid_shop_remain_manageable_by_owner", func(t *testing.T) {
		prof := freshProfessional()
		id, _ := seed(prof, now())
		exec("UPDATE professionals SET active=FALSE WHERE id=$1", prof)
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", shop)
		defer exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", shop)
		if _, err := manage.Change(ctx, input(id, domain.StatusNoShow)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("database_requires_version_audit_and_immutable_snapshots", func(t *testing.T) {
		id, _ := seed(freshProfessional(), now())
		for _, sql := range []string{"UPDATE appointments SET status='completed' WHERE shop_id=$1 AND id=$2", "UPDATE appointments SET price_cents=1 WHERE shop_id=$1 AND id=$2", "DELETE FROM appointments WHERE shop_id=$1 AND id=$2"} {
			if _, err := pool.Exec(ctx, sql, shop, id); err == nil {
				t.Fatal("unguarded history mutation", sql)
			}
		}
		out, err := manage.Change(ctx, input(id, domain.StatusNoShow))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE appointment_status_events SET reason='Changed' WHERE shop_id=$1 AND appointment_id=$2", shop, id); err == nil {
			t.Fatal("audit editable")
		}
		if _, err := pool.Exec(ctx, "INSERT INTO appointment_status_events(shop_id,appointment_id,version,previous_status,status,source,actor_id,reason,occurred_at) VALUES($1,$2,3,'no_show','completed','staff',$3,'Forged',now())", shop, id, owner); err == nil {
			t.Fatal("forged audit inserted")
		}
		if _, err := pool.Exec(ctx, "UPDATE appointment_completion_policy SET activated_at=now()-interval '10 years'"); err == nil {
			t.Fatal("cutover silently changed")
		}
		if out.Appointment.Appointment.Service.PriceCents != 5000 {
			t.Fatal("snapshot changed")
		}
	})
	t.Run("public_cancellation_keeps_deadline_and_generates_audit", func(t *testing.T) {
		bookingTx := bookingpg.NewTransactor(pool, func(q *db.Queries) booking.BookingTx {
			return booking.BookingTx{Shops: shopspg.NewResolver(q), Schedule: bookingpg.NewRepository(q), Appointments: bookingpg.NewRepository(q)}
		})
		public := booking.NewManageAppointment(bookingTx, security.Codes{}, booking.ClockFunc(time.Now))
		future, code := seed(freshProfessional(), now().Add(3*time.Hour))
		if _, err := public.Cancel(ctx, booking.AppointmentCodeInput{ShopSlug: slug, Code: code}); err != nil {
			t.Fatal(err)
		}
		out := detail(future)
		if out.Appointment.StatusSource != domain.SourcePublic || len(out.History) != 1 {
			t.Fatal(out)
		}
		_, pastCode := seed(freshProfessional(), now())
		if _, err := public.Cancel(ctx, booking.AppointmentCodeInput{ShopSlug: slug, Code: pastCode}); !errors.Is(err, domain.ErrCancellationClosed) {
			t.Fatal("public deadline expanded", err)
		}
	})
	t.Run("HTTP_owner_and_barber_scopes_strict_body_and_preflights", func(t *testing.T) {
		q := db.New(pool)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{AppointmentLifecycle: bookinghttp.NewLifecycleHandler(manage, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		id, _ := seed(freshProfessional(), now())
		path := "/api/v1/admin/shops/" + slug + "/appointments/" + id.String()
		request := func(method, path string, actor uuid.UUID, body, key string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", key)
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		for _, tc := range []struct {
			method, path string
			actor        uuid.UUID
			body         string
			status       int
		}{{"GET", path, uuid.Nil, "", 401}, {"GET", path, barber, "", 403}, {"GET", path, owner, "", 200}, {"POST", path + "/status", owner, "null", 400}, {"POST", path + "/status", owner, `{"status":"no_show","reason":"Missing version"}`, 400}, {"POST", path + "/status", owner, `{"status":"no_show","expected_version":1,"reason":"Wrong","role":"owner"}`, 400}, {"POST", path + "/status", owner, `{"status":"scheduled","expected_version":1,"reason":"Wrong"}`, 422}} {
			w := request(tc.method, tc.path, tc.actor, tc.body, key())
			if w.Code != tc.status {
				t.Fatal(w.Code, tc.status, w.Body.String())
			}
		}
		body := `{"status":"no_show","expected_version":1,"reason":"Did not attend"}`
		commandKey := key()
		first := request("POST", path+"/status", owner, body, commandKey)
		second := request("POST", path+"/status", owner, body, commandKey)
		var replay map[string]any
		if first.Code != 200 || second.Code != 200 || json.Unmarshal(second.Body.Bytes(), &replay) != nil || replay["replayed"] != true {
			t.Fatal(first.Code, first.Body.String(), second.Code, second.Body.String())
		}
		if !strings.Contains(first.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private status cached")
		}
		for _, tc := range []struct{ path, method string }{{path, "GET"}, {path + "/status", "POST"}} {
			r := httptest.NewRequest("OPTIONS", tc.path, nil)
			r.Header.Set("Origin", "http://127.0.0.1:3000")
			r.Header.Set("Access-Control-Request-Method", tc.method)
			r.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 204 {
				t.Fatal("preflight", w.Code)
			}
		}
	})
	t.Run("completion_settings_http_enforces_owner_scope_and_versions", func(t *testing.T) {
		isolated := uuid.New()
		isolatedSlug := "completion-" + isolated.String()
		exec("INSERT INTO shops(id,name,slug) VALUES($1,'Completion HTTP fixture',$2)", isolated, isolatedSlug)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, isolated)
		q := db.New(pool)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
		settings := booking.NewManageCompletionSettings(bookingpg.NewCompletionSettingsTransactor(pool, func(q *db.Queries) booking.CompletionSettingsAccess {
			return identitypg.NewCompletionSettingsAccess(q)
		}))
		h := httpapi.NewHandler(httpapi.HandlerConfig{CompletionSettings: bookinghttp.NewCompletionSettingsHandler(settings, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		path := "/api/v1/admin/shops/" + isolatedSlug + "/appointment-completion"
		request := func(method, target string, actor uuid.UUID, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, target, strings.NewReader(body))
			r.Header.Set("Origin", "http://127.0.0.1:3000")
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			if body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		for _, tc := range []struct {
			method, target string
			actor          uuid.UUID
			body           string
			want           int
		}{
			{"GET", path, uuid.Nil, "", 401},
			{"GET", path, outsider, "", 403},
			{"GET", "/api/v1/admin/shops/" + slug + "/appointment-completion", barber, "", 403},
			{"PUT", "/api/v1/admin/shops/" + slug + "/appointment-completion", barber, `{"mode":"manual","version":1}`, 403},
			{"GET", path, owner, "", 200},
			{"PUT", path, owner, `{"mode":"manual","version":1}`, 200},
			{"PUT", path, owner, `{"mode":"automatic","version":1}`, 409},
			{"PUT", path, owner, `{"mode":"automatic","version":2}`, 200},
		} {
			w := request(tc.method, tc.target, tc.actor, tc.body)
			if w.Code != tc.want {
				t.Fatalf("%s completion settings: got %d want %d: %s", tc.method, w.Code, tc.want, w.Body.String())
			}
		}
		var saved struct {
			ShopID  uuid.UUID `json:"shop_id"`
			Mode    string    `json:"mode"`
			Version int       `json:"version"`
		}
		if w := request("GET", path, owner, ""); json.Unmarshal(w.Body.Bytes(), &saved) != nil || saved.ShopID != isolated || saved.Mode != "automatic" || saved.Version != 3 {
			t.Fatal("completion mode was not persisted", saved)
		}
	})
	t.Run("shop_completion_mode_preserves_manual_backlog_and_serializes_with_scheduler", func(t *testing.T) {
		settings := booking.NewManageCompletionSettings(bookingpg.NewCompletionSettingsTransactor(pool, func(q *db.Queries) booking.CompletionSettingsAccess { return identitypg.NewCompletionSettingsAccess(q) }))
		ownerScope := booking.CompletionSettingsScope{ActorID: owner, ShopID: shop}
		barberScope := booking.CompletionSettingsScope{ActorID: barber, ShopID: shop}
		initial, err := settings.Read(ctx, ownerScope)
		if err != nil || initial.Mode != domain.CompletionAutomatic || initial.Version != 1 {
			t.Fatal("existing default", initial, err)
		}
		otherSettings, err := db.New(pool).GetShopAppointmentCompletion(ctx, other)
		if err != nil || otherSettings.Mode != "automatic" {
			t.Fatal("new shop default", otherSettings, err)
		}
		completed, _ := seed(freshProfessional(), now())
		if _, err = due.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		// A pending policy write must be skipped rather than completed from a stale snapshot.
		locked, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer locked.Rollback(context.Background())
		if _, err = locked.Exec(ctx, "UPDATE shop_appointment_completion_settings SET mode='manual',automatic_since=NULL,version=version+1 WHERE shop_id=$1", shop); err != nil {
			t.Fatal(err)
		}
		pending, _ := seed(freshProfessional(), now())
		short, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err = due.Execute(short)
		cancel()
		if err != nil {
			t.Fatal("scheduler blocked behind settings", err)
		}
		if detail(pending).Appointment.Version != 1 {
			t.Fatal("scheduler used stale automatic policy")
		}
		if err = locked.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		manual, err := settings.Read(ctx, ownerScope)
		if err != nil || manual.Mode != domain.CompletionManual || !manual.AutomaticSince.IsZero() {
			t.Fatal(manual, err)
		}
		for i := 0; i < 2; i++ {
			if _, err = due.Execute(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if got := detail(pending); got.Appointment.Version != 1 || got.CompletionMode != domain.CompletionManual {
			t.Fatal("manual appointment auto completed", got)
		}
		_, err = pool.Exec(ctx, "UPDATE appointments SET status='completed',version=version+1,status_source='automatic',status_reason='Reserved interval elapsed',status_actor_id=NULL,updated_at=clock_timestamp() WHERE id=$1", pending)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "appointment_completion_mode" {
			t.Fatal("database did not enforce manual mode", err)
		}
		if _, err = manage.Change(ctx, input(pending, domain.StatusCompleted)); err != nil {
			t.Fatal("owner manual completion", err)
		}
		if got := detail(completed); got.Appointment.StatusSource != domain.SourceAutomatic || got.Appointment.Version != 2 {
			t.Fatal("past completion was rewritten", got)
		}
		if _, err = settings.Save(ctx, ownerScope, domain.CompletionMode("invalid"), manual.Version); !errors.Is(err, booking.ErrInvalidCompletionSettings) {
			t.Fatal(err)
		}
		if _, err = settings.Save(ctx, barberScope, domain.CompletionAutomatic, manual.Version); !errors.Is(err, booking.ErrCompletionSettingsAccessDenied) {
			t.Fatal("barber changed shop policy", err)
		}
		if _, err = settings.Save(ctx, booking.CompletionSettingsScope{ActorID: outsider, ShopID: shop}, domain.CompletionAutomatic, manual.Version); !errors.Is(err, booking.ErrCompletionSettingsAccessDenied) {
			t.Fatal("tenant leak", err)
		}
		exec("UPDATE shop_memberships SET active=FALSE WHERE user_id=$1 AND shop_id=$2", owner, shop)
		_, err = settings.Read(ctx, ownerScope)
		exec("UPDATE shop_memberships SET active=TRUE WHERE user_id=$1 AND shop_id=$2", owner, shop)
		if !errors.Is(err, booking.ErrCompletionSettingsAccessDenied) {
			t.Fatal("revoked account read policy", err)
		}
		backlog, _ := seed(freshProfessional(), now())
		// Two tabs using the same version cannot overwrite one another.
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, s := range []booking.CompletionSettingsScope{ownerScope, ownerScope} {
			go func(s booking.CompletionSettingsScope) {
				<-start
				_, e := settings.Save(ctx, s, domain.CompletionAutomatic, manual.Version)
				results <- e
			}(s)
		}
		close(start)
		success, conflicts := 0, 0
		for i := 0; i < 2; i++ {
			e := <-results
			if e == nil {
				success++
			} else if errors.Is(e, booking.ErrCompletionSettingsConflict) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
		if success != 1 || conflicts != 1 {
			t.Fatal(success, conflicts)
		}
		automatic, err := settings.Read(ctx, ownerScope)
		if err != nil {
			t.Fatal(err)
		}
		// End after the cutover, using the database clock, without changing immutable dates.
		eligible, _ := seed(freshProfessional(), now())
		if _, err = due.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		if detail(backlog).Appointment.Version != 1 || detail(eligible).Appointment.StatusSource != domain.SourceAutomatic {
			t.Fatal("automatic cutover closed backlog or skipped new eligible booking")
		}
		same, err := settings.Save(ctx, ownerScope, domain.CompletionAutomatic, automatic.Version)
		if err != nil || !same.AutomaticSince.Equal(automatic.AutomaticSince) {
			t.Fatal("same-mode save moved cutover", same, err)
		}
		if _, err = settings.Save(ctx, ownerScope, domain.CompletionManual, manual.Version); !errors.Is(err, booking.ErrCompletionSettingsConflict) {
			t.Fatal("stale version accepted", err)
		}
	})
}
