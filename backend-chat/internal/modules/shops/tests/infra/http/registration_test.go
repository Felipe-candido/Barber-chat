package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	"github.com/google/uuid"
)

type verifier struct{ id uuid.UUID }

func (v verifier) VerifyAccessToken(_ context.Context, token string) (uuid.UUID, error) {
	if token != "session" {
		return uuid.Nil, identity.ErrInvalidAccessToken
	}
	return v.id, nil
}

type accountReader struct {
	id        uuid.UUID
	confirmed bool
}

func (r accountReader) ReadRegistrationAccount(context.Context, string) (identity.RegistrationAccount, error) {
	return identity.RegistrationAccount{UserID: r.id, EmailConfirmed: r.confirmed}, nil
}

type store struct {
	owner uuid.UUID
	calls int
}

func (s *store) EnsureActiveUser(_ context.Context, id uuid.UUID, _ string) error {
	s.owner = id
	return nil
}
func (s *store) CreateOwnerMembership(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *store) RequireActiveOwner(context.Context, uuid.UUID, uuid.UUID) error    { return nil }
func (s *store) FindCreation(context.Context, uuid.UUID, [32]byte) (app.ShopCreationRecord, bool, error) {
	return app.ShopCreationRecord{}, false, nil
}
func (s *store) Create(context.Context, domain.Shop) error { s.calls++; return nil }
func (s *store) RecordCreation(context.Context, uuid.UUID, [32]byte, [32]byte, uuid.UUID) error {
	return nil
}
func (s *store) WithinShopCreationTx(ctx context.Context, fn func(context.Context, app.ShopCreationTx) error) error {
	return fn(ctx, app.ShopCreationTx{Owners: s, Shops: s})
}

func TestRegistrationHTTPBoundary(t *testing.T) {
	id := uuid.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	valid := `{"display_name":"Owner","name":"Shop","slug":"my-shop","timezone":"America/Sao_Paulo","address":{"postal_code":"01310-100","street":"Avenida Paulista","number":"100","district":"Bela Vista","city":"São Paulo","state":"SP"}}`
	for _, test := range []struct {
		name, body, token, key, contentType string
		confirmed                           bool
		status                              int
	}{
		{"valid", valid, "session", strings.Repeat("ab", 32), "application/json", true, 201},
		{"address required", `{"display_name":"Owner","name":"Shop","slug":"my-shop"}`, "session", strings.Repeat("ab", 32), "application/json", true, 422},
		{"invalid address", strings.Replace(valid, `"state":"SP"`, `"state":"ZZ"`, 1), "session", strings.Repeat("ab", 32), "application/json", true, 422},
		{"missing session", valid, "", strings.Repeat("ab", 32), "application/json", true, 401},
		{"invalid session", valid, "expired", strings.Repeat("ab", 32), "application/json", true, 401},
		{"unconfirmed email", valid, "session", strings.Repeat("ab", 32), "application/json", false, 403},
		{"cannot choose owner", `{"display_name":"Owner","name":"Shop","slug":"my-shop","user_id":"` + uuid.New().String() + `"}`, "session", strings.Repeat("ab", 32), "application/json", true, 400},
		{"cannot choose role", `{"role":"owner"}`, "session", strings.Repeat("ab", 32), "application/json", true, 400},
		{"null", "null", "session", strings.Repeat("ab", 32), "application/json", true, 400},
		{"extra JSON", valid + ` {}`, "session", strings.Repeat("ab", 32), "application/json", true, 400},
		{"missing key", valid, "session", "", "application/json", true, 422},
		{"wrong content type", valid, "session", strings.Repeat("ab", 32), "text/plain", true, 415},
		{"oversized", `{"name":"` + strings.Repeat("a", 9000) + `"}`, "session", strings.Repeat("ab", 32), "application/json", true, 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &store{}
			h := httpapi.NewHandler(httpapi.HandlerConfig{Registration: shopshttp.NewRegistrationHandler(app.NewCreateShop(s), 0, logger), VerifyRegistration: identity.NewVerifyRegistrationAccount(verifier{id}, accountReader{id, test.confirmed}), Logger: logger, FrontendOrigin: "http://127.0.0.1:3000"})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/shops", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.contentType)
			if test.token != "" {
				r.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.key != "" {
				r.Header.Set("Idempotency-Key", test.key)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if test.status == 201 {
				if s.owner != id || s.calls != 1 {
					t.Fatal("wrong owner")
				}
			} else if s.calls != 0 {
				t.Fatal("rejected input reached creation")
			}
		})
	}
	t.Run("registration identity is not staff identity", func(t *testing.T) {
		h := middleware.RegistrationAuthentication(identity.NewVerifyRegistrationAccount(verifier{id}, accountReader{id, true}), logger)(http.HandlerFunc(identityhttp.Me))
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("onboarding identity escalated to staff", w.Code)
		}
	})
	t.Run("preflight accepts idempotency header", func(t *testing.T) {
		h := httpapi.NewHandler(httpapi.HandlerConfig{Registration: shopshttp.NewRegistrationHandler(app.NewCreateShop(&store{}), 0, logger), Logger: logger, FrontendOrigin: "http://127.0.0.1:3000"})
		r := httptest.NewRequest("OPTIONS", "/api/v1/onboarding/shops", nil)
		r.Header.Set("Origin", "http://127.0.0.1:3000")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type,Idempotency-Key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	})
}
