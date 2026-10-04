package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	access "github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

func TestRequestLogRecordsOutcomeWithoutSecrets(t *testing.T) {
	for _, status := range []int{201, 422, 503} {
		var buffer bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buffer, nil))
		router := chi.NewRouter()
		router.Use(access.RequestLog(logger))
		var handlerID string
		router.Post("/shops/{slug}/professionals", func(w http.ResponseWriter, r *http.Request) {
			handlerID = chimiddleware.GetReqID(r.Context())
			w.WriteHeader(status)
		})
		r := httptest.NewRequest("POST", "/shops/private-shop/professionals?code=private-code", strings.NewReader(`{"name":"private-name","phone":"private-phone"}`))
		r.Header.Set("Authorization", "Bearer private-token")
		r.Header.Set("Idempotency-Key", "private-key")
		r.Header.Set("X-Request-ID", "private-caller-id")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		for _, secret := range []string{"private-shop", "private-code", "private-name", "private-phone", "private-token", "private-key", "private-caller-id"} {
			if strings.Contains(buffer.String(), secret) {
				t.Fatalf("log leaked %s", secret)
			}
		}
		var entry struct {
			Method    string `json:"method"`
			Route     string `json:"route"`
			Status    int    `json:"status"`
			Level     string `json:"level"`
			RequestID string `json:"request_id"`
			Duration  int64  `json:"duration_ms"`
		}
		if err := json.Unmarshal(buffer.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		wantLevel := "INFO"
		if status >= 500 {
			wantLevel = "ERROR"
		} else if status >= 400 {
			wantLevel = "WARN"
		}
		if entry.Method != "POST" || entry.Route != "/shops/{slug}/professionals" || entry.Status != status || entry.Level != wantLevel || entry.Duration < 0 {
			t.Fatalf("unexpected HTTP metadata: %+v", entry)
		}
		if _, err := uuid.Parse(entry.RequestID); err != nil || entry.RequestID != handlerID || entry.RequestID != w.Header().Get("X-Request-ID") {
			t.Fatal("request correlation ID was not generated and propagated")
		}
	}
}

func TestRequestLogRecordsCORSAndUnmatchedRequestsWithoutRawPath(t *testing.T) {
	var buffer bytes.Buffer
	router := chi.NewRouter()
	router.Use(access.RequestLog(slog.New(slog.NewJSONHandler(&buffer, nil))))
	router.Use(access.CORS("http://127.0.0.1:3000"))
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, tc := range []struct {
		path, origin string
		status       int
	}{
		{"/health", "https://disallowed.example", 403},
		{"/private-code", "", 404},
	} {
		buffer.Reset()
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(buffer.String(), `"status":`) || strings.Contains(buffer.String(), "private-code") || strings.Contains(buffer.String(), "disallowed.example") {
			t.Fatalf("rejected request log: %d %s", w.Code, buffer.String())
		}
	}
}
