package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
)

func TestCompletionSettingsRouteAndPreflight(t *testing.T) {
	const path = "/api/v1/admin/shops/example/appointment-completion"
	const origin = "https://app.example.com"
	h := httpapi.NewHandler(httpapi.HandlerConfig{
		FrontendOrigin: origin,
		Logger: testLogger(),
		CompletionSettings: bookinghttp.NewCompletionSettingsHandler(nil, 0, testLogger()),
	})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		r := httptest.NewRequest(http.MethodOptions, path, nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), method) {
			t.Fatalf("%s preflight unavailable: %d", method, w.Code)
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != 401 { t.Fatalf("%s must be registered and authenticated, got %d", method, w.Code) }
	}
}
