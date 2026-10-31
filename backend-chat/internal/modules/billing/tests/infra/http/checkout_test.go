package http_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

type failingCheckout struct{}

func (failingCheckout) Offer(context.Context, application.BillingScope) (application.CheckoutOffer, error) {
	return application.CheckoutOffer{}, checkoutFailure()
}
func (failingCheckout) Execute(context.Context, application.StartCheckoutInput) (application.CheckoutOutput, error) {
	return application.CheckoutOutput{}, checkoutFailure()
}
func checkoutFailure() error {
	return errors.Join(errors.New("secret-provider-message client@example.com sk_test_private"),
		&application.ProviderFailure{HTTPStatus: 400, Code: "parameter_unknown", Parameter: "payment_method_types", RequestID: "req_safe123"})
}

func TestCheckoutLogsSafeProviderDiagnosticsWithoutReturningThem(t *testing.T) {
	var logs bytes.Buffer
	handler := billinghttp.NewCheckoutHandler(failingCheckout{}, time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
	actor := uuid.New()
	ctx := requestctx.WithStaffIdentity(context.Background(), identity.StaffIdentityOutput{UserID: actor})
	ctx = requestctx.WithShopScope(ctx, identity.AuthorizedShopScope{UserID: actor, ShopID: uuid.New(), Role: domain.RoleOwner})
	r := httptest.NewRequest("POST", "/billing/checkout", strings.NewReader(`{"intent":"publish","offer_id":"`+strings.Repeat("a", 64)+`"}`)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Start(w, r)
	if w.Code != 503 || !strings.Contains(logs.String(), `"provider_status":400`) || !strings.Contains(logs.String(), "req_safe123") || !strings.Contains(logs.String(), "parameter_unknown") {
		t.Fatal("missing safe diagnostics", w.Code, logs.String())
	}
	for _, forbidden := range []string{"secret-provider-message", "client@example.com", "sk_test_private"} {
		if strings.Contains(logs.String()+w.Body.String(), forbidden) {
			t.Fatal("sensitive diagnostics leaked")
		}
	}
	if strings.Contains(w.Body.String(), "req_safe123") || strings.Contains(w.Body.String(), "parameter_unknown") {
		t.Fatal("provider diagnostics returned to browser")
	}
}
