package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	report "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	reporthttp "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/infra/http"
	"github.com/google/uuid"
)

type reportingRouteReader struct {
	calls int
	input report.ActivityReportInput
}

func (r *reportingRouteReader) Overview(_ context.Context, in report.ActivityReportInput) (report.ActivityOverview, error) {
	r.calls++
	r.input = in
	p, err := domain.NewPeriod(in.FromDate, in.ThroughDate, "America/Sao_Paulo")
	return report.ActivityOverview{Period: p, Source: domain.ActivityAll, ObservedAt: time.Now().UTC()}, err
}
func (r *reportingRouteReader) History(_ context.Context, in report.ActivityHistoryInput) (report.ActivityHistoryPage, error) {
	r.calls++
	r.input = in.ActivityReportInput
	p, err := domain.NewPeriod(in.FromDate, in.ThroughDate, "America/Sao_Paulo")
	return report.ActivityHistoryPage{Period: p, Source: domain.ActivityAll, ObservedAt: time.Now().UTC()}, err
}

func TestReportingRoutesRequireRealVerifiedIdentityAndShopMembership(t *testing.T) {
	for _, suffix := range []string{"", "/history"} {
		t.Run(suffix, func(t *testing.T) {
			f := newAccessFixture(t)
			reader := &reportingRouteReader{}
			logger := slog.New(slog.NewJSONHandler(&f.logs, nil))
			h := httpapi.NewHandler(httpapi.HandlerConfig{Reporting: reporthttp.NewHandler(reader, time.Second, logger), Authenticate: identity.NewAuthenticate(f.verifier, f.store), AuthorizeShop: identity.NewAuthorizeShopAction(f.store, f.store), Logger: logger, FrontendOrigin: browserOrigin})
			path := "/api/v1/admin/shops/shop-a/reports/activity" + suffix + "?from=2026-10-12&through=2026-10-12"
			if w := accessRequest(h, "GET", path, "", ""); w.Code != 401 || reader.calls != 0 {
				t.Fatal("anonymous reached reporting", w.Code)
			}
			if w := accessRequest(h, "GET", path, "invalid-token", ""); w.Code != 401 || reader.calls != 0 {
				t.Fatal("invalid token reached reporting", w.Code)
			}
			token := f.token(t, nil)
			foreign := strings.Replace(path, "shop-a", "shop-b", 1)
			if w := accessRequest(h, "GET", foreign, token, ""); w.Code != 403 || reader.calls != 0 {
				t.Fatal("foreign unit reached reporting", w.Code, w.Body.String())
			}
			shop := f.store.shops["shop-a"]
			for _, role := range []identitydomain.Role{identitydomain.RoleOwner, identitydomain.RoleBarber} {
				membership := f.store.memberships[shop]
				membership.Role = role
				if role == identitydomain.RoleBarber {
					membership.ProfessionalID = uuid.New()
				}
				f.store.memberships[shop] = membership
				w := accessRequest(h, "GET", path+"&phone=%2B5511912345678&customer_name=PrivateName", token, "")
				if w.Code != 200 || reader.input.ActorID != f.store.user.ID || reader.input.ShopID != shop || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal(role, w.Code, w.Body.String())
				}
			}
			if strings.Contains(f.logs.String(), token) || strings.Contains(f.logs.String(), "5511912345678") || strings.Contains(f.logs.String(), "PrivateName") {
				t.Fatal("request logger disclosed report query or token")
			}
			calls := reader.calls
			membership := f.store.memberships[shop]
			membership.Active = false
			f.store.memberships[shop] = membership
			if w := accessRequest(h, "GET", path, token, ""); w.Code != 403 || reader.calls != calls {
				t.Fatal("inactive membership reached reporting", w.Code)
			}
			if w := accessRequest(h, "POST", path, token, ""); w.Code != 405 || reader.calls != calls {
				t.Fatal("write method registered for reports", w.Code)
			}
			r := httptest.NewRequest(http.MethodOptions, path, nil)
			r.Header.Set("Origin", browserOrigin)
			r.Header.Set("Access-Control-Request-Method", "GET")
			r.Header.Set("Access-Control-Request-Headers", "authorization")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 204 || reader.calls != calls {
				t.Fatal("report preflight not available", w.Code, w.Body.String())
			}
		})
	}
}
