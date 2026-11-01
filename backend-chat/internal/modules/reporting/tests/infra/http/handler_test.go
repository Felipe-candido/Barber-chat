package http_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	reporthttp "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/http"
	"github.com/google/uuid"
)

type queryStub struct {
	overview     app.ActivityOverview
	history      app.ActivityHistoryPage
	input        app.ActivityReportInput
	historyInput app.ActivityHistoryInput
	calls        int
	err          error
	wait         bool
}

func (q *queryStub) Overview(ctx context.Context, in app.ActivityReportInput) (app.ActivityOverview, error) {
	q.calls++
	q.input = in
	if q.wait {
		<-ctx.Done()
		return app.ActivityOverview{}, ctx.Err()
	}
	return q.overview, q.err
}
func (q *queryStub) History(ctx context.Context, in app.ActivityHistoryInput) (app.ActivityHistoryPage, error) {
	q.calls++
	q.historyInput = in
	return q.history, q.err
}

func reportStub(t *testing.T) (*queryStub, uuid.UUID, uuid.UUID) {
	t.Helper()
	period, err := domain.NewPeriod("2026-10-12", "2026-10-12", "America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	return &queryStub{overview: app.ActivityOverview{Period: period, Source: domain.ActivityAll, ObservedAt: at}, history: app.ActivityHistoryPage{Period: period, Source: domain.ActivityAll, ObservedAt: at}}, uuid.New(), uuid.New()
}
func reportRequest(raw string, user, shop uuid.UUID) *http.Request {
	r := httptest.NewRequest("GET", "http://api.test/reports?"+raw, nil)
	ctx := requestctx.WithStaffIdentity(r.Context(), identity.StaffIdentityOutput{UserID: user})
	ctx = requestctx.WithShopScope(ctx, identity.AuthorizedShopScope{UserID: user, ShopID: shop, Role: identitydomain.RoleOwner})
	return r.WithContext(ctx)
}
func quietHandler(q *queryStub) *reporthttp.Handler {
	return reporthttp.NewHandler(q, time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func TestReportHTTPMapsPrivateOverviewAndServerScope(t *testing.T) {
	q, user, shop := reportStub(t)
	q.overview.Counts = app.OutcomeCounts{Completed: 2, CompletedAutomatically: 1}
	q.overview.Values = []app.ActivityValues{{Currency: "BRL", ScheduledServiceValueCents: 5000, CompletedServiceValueCents: 10000, ProductSaleValueCents: 9000, CompletedAppointmentCount: 2, ProductSaleCount: 1, ProductUnitCount: 3}}
	q.overview.ByProfessional = []app.ActivityGroup{{Label: "", Values: []app.ActivityValues{{Currency: "BRL", ProductSaleValueCents: 9000, ProductSaleCount: 1, ProductUnitCount: 3}}}}
	service, product, professional := uuid.New(), uuid.New(), uuid.New()
	params := url.Values{"from": {"2026-10-12"}, "through": {"2026-10-12"}, "source": {"all"}, "customer_name": {"Ana %_"}, "phone": {"+5511912345678"}, "professional_id": {professional.String()}, "service_id": {service.String()}, "product_id": {product.String()}}
	w := httptest.NewRecorder()
	quietHandler(q).Overview(w, reportRequest(params.Encode(), user, shop))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Body.String())
	}
	if q.input.ActorID != user || q.input.ShopID != shop || q.input.Filters.Phone != "+5511912345678" || q.input.Filters.CustomerName != "Ana %_" || q.input.Filters.ProductID != product || q.input.Filters.ServiceID != service || q.input.Filters.ProfessionalID != professional {
		t.Fatal(q.input)
	}
	var body struct {
		Period struct {
			FromDate, ThroughDate string
			Timezone              string
			FromAt                time.Time `json:"from_at"`
		}
		Counts struct {
			CompletedAutomatically int64 `json:"completed_automatically"`
		}
		Values []struct {
			Total    int64 `json:"combined_value_cents"`
			Forecast int64 `json:"scheduled_service_value_cents"`
		}
		ByService      []any                     `json:"by_service"`
		ByProfessional []struct{ ID *uuid.UUID } `json:"by_professional"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Period.Timezone != "America/Sao_Paulo" || body.Period.FromAt.Format(time.RFC3339) != "2026-10-12T03:00:00Z" || body.Counts.CompletedAutomatically != 1 || len(body.Values) != 1 || body.Values[0].Total != 19000 || body.Values[0].Forecast != 5000 || body.ByService == nil || len(body.ByProfessional) != 1 || body.ByProfessional[0].ID != nil {
		t.Fatal(w.Body.String())
	}
}

func TestReportHTTPHistoryKeepsOriginalCancelledValueAndOpaqueCursor(t *testing.T) {
	q, user, shop := reportStub(t)
	record, item := uuid.New(), uuid.New()
	at := time.Date(2026, 10, 12, 12, 30, 0, 0, time.UTC)
	q.history.Rows = []app.ActivityHistoryRow{{Source: domain.ActivityProducts, RecordID: record, ItemID: item, ProductID: uuid.New(), Currency: "BRL", SaleStatus: domain.ProductSaleCancelled, CustomerName: "Ana Silva", CustomerPhone: "+5511912345678", OccurredAt: at, Quantity: 3, UnitPriceCents: 3000, ValueCents: 9000}}
	q.history.Next = &app.ActivityCursor{OccurredAt: at, Source: domain.ActivityProducts, RecordID: record, ItemID: item, FilterFingerprint: [32]byte{1, 2, 3}}
	h := quietHandler(q)
	w := httptest.NewRecorder()
	h.History(w, reportRequest("from=2026-10-12&through=2026-10-12&limit=1&sale_status=cancelled", user, shop))
	var body struct {
		Items []struct {
			Value             int64      `json:"value_cents"`
			Recorded          int64      `json:"recorded_value_cents"`
			ProfessionalID    *uuid.UUID `json:"professional_id"`
			AppointmentStatus *string    `json:"appointment_status"`
		}
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Items) != 1 || body.Items[0].Value != 9000 || body.Items[0].Recorded != 0 || body.Items[0].ProfessionalID != nil || body.Items[0].AppointmentStatus != nil || body.Next == "" || q.historyInput.PageSize != 1 || q.historyInput.SaleStatus != domain.ProductSaleCancelled {
		t.Fatal(w.Code, w.Body.String())
	}
	decoded, err := base64.RawURLEncoding.DecodeString(body.Next)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(decoded), "Ana") || strings.Contains(string(decoded), "5511912345678") {
		t.Fatal("cursor disclosed contact")
	}
	for _, private := range []string{`"code_hash"`, `"access_token"`, `"status_reason"`, `"status_actor_id"`, `"filter_fingerprint"`} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private field disclosed", private)
		}
	}
	w = httptest.NewRecorder()
	h.History(w, reportRequest("from=2026-10-12&through=2026-10-12&cursor="+body.Next, user, shop))
	if w.Code != 200 || q.historyInput.After == nil || *q.historyInput.After != *q.history.Next {
		t.Fatal("cursor round trip changed position or binding", w.Code, q.historyInput.After)
	}
	for name, bad := range map[string]string{
		"padded": body.Next + "=", "trailing newline": body.Next + "\n", "oversized": strings.Repeat("a", 1025),
		"unknown version": base64.RawURLEncoding.EncodeToString(bytes.Replace(decoded, []byte(`"v":1`), []byte(`"v":2`), 1)),
		"duplicate field": base64.RawURLEncoding.EncodeToString(bytes.Replace(decoded, []byte(`"v":1`), []byte(`"v":1,"v":1`), 1)),
		"unknown field":   base64.RawURLEncoding.EncodeToString(append(append([]byte{}, decoded[:len(decoded)-1]...), []byte(`,"extra":true}`)...)),
		"invalid binding": base64.RawURLEncoding.EncodeToString(bytes.Replace(decoded, []byte(`"binding":"`), []byte(`"binding":"bad`), 1)),
		"null":            base64.RawURLEncoding.EncodeToString([]byte(`null`)),
	} {
		t.Run(name, func(t *testing.T) {
			calls := q.calls
			w := httptest.NewRecorder()
			h.History(w, reportRequest("from=2026-10-12&through=2026-10-12&cursor="+url.QueryEscape(bad), user, shop))
			if w.Code != 400 || q.calls != calls {
				t.Fatal("malformed cursor reached application", w.Code, w.Body.String())
			}
		})
	}
}

func TestReportHTTPRejectsAmbiguousQueriesAndScopeInjection(t *testing.T) {
	for _, tc := range []struct {
		query   string
		history bool
	}{
		{"", false}, {"from=2026-10-12", false}, {"from=&through=2026-10-12", false},
		{"from=2026-10-12&through=2026-10-12&from=2026-10-13", false},
		{"from=2026-10-12&through=2026-10-12&actor_id=" + uuid.NewString(), true},
		{"from=2026-10-12&through=2026-10-12&shop_id=" + uuid.NewString(), false},
		{"from=2026-10-12&through=2026-10-12&role=owner", false},
		{"from=2026-10-12&through=2026-10-12&professional_id=00000000-0000-0000-0000-000000000000", false},
		{"from=2026-10-12&through=2026-10-12&product_id=bad", true},
		{"from=2026-10-12&through=2026-10-12&cursor=abc", false},
		{"from=2026-10-12&through=2026-10-12&sale_status=recorded", false},
		{"from=2026-10-12&through=2026-10-12&limit=0", true},
		{"from=2026-10-12&through=2026-10-12&limit=101", true},
		{"from=2026-10-12&through=2026-10-12&limit=no", true},
		{"from=2026-10-12&through=2026-10-12&phone=%GG", true},
		{strings.Repeat("a", 4097), false},
	} {
		t.Run(tc.query[:min(len(tc.query), 65)], func(t *testing.T) {
			q, user, shop := reportStub(t)
			w := httptest.NewRecorder()
			r := reportRequest(tc.query, user, shop)
			h := quietHandler(q)
			if tc.history {
				h.History(w, r)
			} else {
				h.Overview(w, r)
			}
			if w.Code != 400 || q.calls != 0 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	q, user, shop := reportStub(t)
	for _, tc := range []struct {
		name   string
		r      *http.Request
		status int
	}{
		{"anonymous", httptest.NewRequest("GET", "/?from=x&through=x", nil), 401},
		{"mismatched scope", reportRequest("from=x&through=x", user, shop).WithContext(requestctx.WithShopScope(requestctx.WithStaffIdentity(context.Background(), identity.StaffIdentityOutput{UserID: user}), identity.AuthorizedShopScope{UserID: uuid.New(), ShopID: shop, Role: identitydomain.RoleOwner})), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			quietHandler(q).Overview(w, tc.r)
			if w.Code != tc.status || q.calls != 0 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestReportHTTPErrorsAndTimeoutDoNotDiscloseSensitiveInputs(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{app.ErrInvalidQuery, 400, "invalid_report_query"}, {app.ErrActivityAccessDenied, 403, "access_denied"}, {app.ErrShopNotFound, 404, "shop_not_found"},
		{app.ErrInconsistentData, 500, "report_unavailable"}, {errors.New("sensitive +5511912345678 token-should-not-be-logged"), 503, "temporarily_unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			q, user, shop := reportStub(t)
			q.err = tc.err
			var logs bytes.Buffer
			h := reporthttp.NewHandler(q, time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
			w := httptest.NewRecorder()
			h.Overview(w, reportRequest("from=2026-10-12&through=2026-10-12&phone=%2B5511912345678", user, shop))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(logs.String(), "5511912345678") || strings.Contains(logs.String(), "token-should-not-be-logged") || strings.Contains(w.Body.String(), "sensitive") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Code, w.Body.String(), logs.String())
			}
		})
	}
	q, user, shop := reportStub(t)
	q.wait = true
	h := reporthttp.NewHandler(q, 5*time.Millisecond, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	w := httptest.NewRecorder()
	h.Overview(w, reportRequest("from=2026-10-12&through=2026-10-12", user, shop))
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}
