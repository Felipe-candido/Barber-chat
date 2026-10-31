package stripe_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(value any) *http.Response {
	data, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data)))}
}

func TestStripeCheckoutRequests(t *testing.T) {
	ctx := context.Background()
	shop, attemptID := uuid.New(), uuid.New()
	attempt := application.CheckoutAttempt{ID: attemptID, ShopID: shop, Intent: domain.CheckoutPublish,
		PriceID: "price_monthly", ReturnURL: "http://127.0.0.1:3000/admin/test/configuracoes"}
	var original string
	calls := 0
	c, err := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "api.stripe.com" || r.Header.Get("Stripe-Version") != stripe.APIVersion {
			t.Fatal("untrusted endpoint/version")
		}
		key, pass, ok := r.BasicAuth()
		if !ok || key != "sk_test_example" || pass != "" {
			t.Fatal("unexpected authentication")
		}
		data, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(data))
		switch r.URL.Path {
		case "/v1/customers":
			if r.Method != "POST" || r.Header.Get("Idempotency-Key") != "barber:test:customer:"+shop.String() || !reflect.DeepEqual(form, url.Values{"metadata[shop_id]": {shop.String()}}) {
				t.Fatal("mutable or non-idempotent Customer request")
			}
			return jsonResponse(map[string]any{"id": "cus_example", "object": "customer", "livemode": false, "metadata": map[string]string{"shop_id": shop.String()}}), nil
		case "/v1/checkout/sessions":
			if r.Method != "POST" || r.Header.Get("Idempotency-Key") != "barber:test:checkout:"+attemptID.String() || form.Get("mode") != "subscription" || form.Get("line_items[0][price]") != attempt.PriceID || form.Get("line_items[0][quantity]") != "1" || form.Get("payment_method_types[0]") != "card" || form.Get("customer") != "cus_example" {
				t.Fatal("unexpected Session request")
			}
			if original != "" && original != string(data) {
				t.Fatal("retry changed provider parameters")
			}
			original = string(data)
			if form.Get("success_url") != attempt.ReturnURL+"?checkout=returned" || form.Get("cancel_url") != attempt.ReturnURL+"?checkout=canceled" || form.Get("subscription_data[metadata][attempt_id]") != attempt.ID.String() {
				t.Fatal("unexpected return or association")
			}
			return jsonResponse(sessionJSON(attempt)), nil
		default:
			t.Fatal("unexpected request", r.URL.Path)
			return nil, nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := c.CreateCheckoutCustomer(ctx, shop); err != nil || id != "cus_example" {
		t.Fatal(id, err)
	}
	for range 2 {
		got, err := c.CreateCheckoutSession(ctx, attempt, "cus_example")
		if err != nil || got.ShopID != shop || got.AttemptID != attemptID {
			t.Fatal(got, err)
		}
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}

func sessionJSON(a application.CheckoutAttempt) map[string]any {
	return map[string]any{"id": "cs_test_example", "object": "checkout.session", "customer": "cus_example", "livemode": false,
		"mode": "subscription", "status": "open", "url": "https://checkout.stripe.com/c/pay/example", "client_reference_id": a.ID.String(),
		"expires_at": time.Now().Add(time.Hour).Unix(), "metadata": map[string]string{"shop_id": a.ShopID.String(), "attempt_id": a.ID.String()}}
}

func TestStripeRejectsUnsafeOrMalformedSessions(t *testing.T) {
	attempt := application.CheckoutAttempt{ID: uuid.New(), ShopID: uuid.New()}
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"live mode", "livemode", true}, {"missing mode", "livemode", nil},
		{"wrong URL", "url", "https://checkout.stripe.com.evil.test/c/pay/fake"},
		{"insecure URL", "url", "http://checkout.stripe.com/c/pay/fake"},
		{"credentials", "url", "https://user:secret@checkout.stripe.com/c/pay/fake"},
		{"wrong reference", "client_reference_id", uuid.New().String()}, {"payment not subscription", "mode", "payment"},
		{"wrong object", "object", "customer"}, {"missing customer", "customer", ""}, {"invalid status", "status", "paid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := sessionJSON(attempt)
			value[tc.key] = tc.value
			c, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(value), nil })})
			if _, err := c.FindCheckoutSession(context.Background(), "cs_test_example"); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
	if _, err := stripe.NewClient("sk_live_secret", nil); err == nil {
		t.Fatal("live secret accepted")
	}
}

func TestStripeMonthlyPriceValidation(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
		allowed   bool
	}{
		{"monthly BRL", "currency", "brl", true}, {"wrong currency", "currency", "usd", false},
		{"live price", "livemode", true, false}, {"archived price", "active", false, false},
		{"zero price", "unit_amount", 0, false}, {"one-time price", "type", "one_time", false},
		{"missing amount", "unit_amount", nil, false}, {"fractional amount", "unit_amount", 99.5, false},
		{"unsafe amount", "unit_amount", int64(9007199254740992), false},
		{"custom amount", "custom_unit_amount", map[string]any{"enabled": true}, false},
		{"transformed quantity", "transform_quantity", map[string]any{"divide_by": 2, "round": "up"}, false},
		{"annual price", "recurring", map[string]any{"interval": "year", "interval_count": 1, "usage_type": "licensed"}, false},
		{"trial", "recurring", map[string]any{"interval": "month", "interval_count": 1, "usage_type": "licensed", "trial_period_days": 7}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := map[string]any{"id": "price_example", "object": "price", "active": true, "livemode": false,
				"currency": "brl", "type": "recurring", "billing_scheme": "per_unit", "unit_amount": 9900,
				"recurring": map[string]any{"interval": "month", "interval_count": 1, "usage_type": "licensed"}}
			value[tc.key] = tc.value
			c, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.Path != "/v1/prices/price_example" || r.Header.Get("Idempotency-Key") != "" {
					t.Fatal("unexpected price request")
				}
				return jsonResponse(value), nil
			})})
			price, err := c.ReadCheckoutPrice(context.Background(), "price_example")
			if (err == nil) != tc.allowed {
				t.Fatal(err)
			}
			if tc.allowed && (price.ID != "price_example" || price.AmountCents != 9900) {
				t.Fatal("incorrect fixed price projection", price)
			}
		})
	}
}

func TestStripeDoesNotFollowRedirectsOrExposeErrors(t *testing.T) {
	requests := 0
	c, _ := stripe.NewClient("sk_test_secret", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://evil.test"}}, Body: io.NopCloser(strings.NewReader("secret provider error"))}, nil
	})})
	_, err := c.CreateCheckoutCustomer(context.Background(), uuid.New())
	if err == nil || strings.Contains(err.Error(), "secret") || requests != 1 {
		t.Fatal("unsafe redirect/error", err, requests)
	}
}

func TestStripeFailureExposesOnlySafeDiagnostics(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		code, param, requestID := "parameter_unknown", "payment_method_types", "req_test123"
		if unsafe {
			code, param, requestID = "secret-provider-text", "client@example.com", "secret key with spaces"
		}
		c, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			response := jsonResponse(map[string]any{"error": map[string]string{"code": code, "param": param, "message": "secret sk_test_example client@example.com"}})
			response.StatusCode = 400
			response.Header.Set("Request-Id", requestID)
			return response, nil
		})})
		_, err := c.CreateCheckoutCustomer(context.Background(), uuid.New())
		var failure *application.ProviderFailure
		if !errors.As(err, &failure) || !errors.Is(err, application.ErrCheckoutUnavailable) || failure.HTTPStatus != 400 {
			t.Fatal("provider diagnostics lost", err)
		}
		if unsafe {
			if failure.Code != "provider_error" || failure.Parameter != "" || failure.RequestID != "" {
				t.Fatal("unsafe diagnostics exposed", failure)
			}
		} else if failure.Code != code || failure.Parameter != param || failure.RequestID != requestID {
			t.Fatal("safe diagnostics missing", failure)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "@") {
			t.Fatal("raw provider message exposed")
		}
	}
}

func TestRejectedLegacyCardParameterUsesOneRecoverableCorrectedRequest(t *testing.T) {
	a := application.CheckoutAttempt{ID: uuid.New(), ShopID: uuid.New(), Intent: domain.CheckoutPublish,
		PriceID: "price_monthly", ReturnURL: "http://127.0.0.1:3000/admin/test/configuracoes"}
	key := "barber:test:checkout:" + a.ID.String()
	bodies := map[string]string{}
	calls, correctedCalls, effects := 0, 0, 0
	c, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(data))
		requestKey := r.Header.Get("Idempotency-Key")
		if previous, ok := bodies[requestKey]; ok && previous != string(data) {
			t.Fatal("changed parameters under an existing key")
		}
		bodies[requestKey] = string(data)
		if form.Has("payment_method_types[0]") {
			if requestKey != key {
				t.Fatal("legacy key changed")
			}
			response := jsonResponse(map[string]any{"error": map[string]string{"type": "invalid_request_error", "param": "payment_method_types"}})
			response.StatusCode = 400
			return response, nil
		}
		if requestKey != key+":card-filter-v2" || form.Get("allowed_payment_method_types[0]") != "card" || form.Get("line_items[0][price]") != a.PriceID || form.Get("customer") != "cus_example" || form.Get("client_reference_id") != a.ID.String() {
			t.Fatal("incorrect corrected request")
		}
		correctedCalls++
		if correctedCalls == 1 {
			effects++
			return nil, errors.New("lost corrected response after creation")
		}
		return jsonResponse(sessionJSON(a)), nil
	})})
	if _, err := c.CreateCheckoutSession(context.Background(), a, "cus_example"); err == nil {
		t.Fatal("lost response reported success")
	}
	if _, err := c.CreateCheckoutSession(context.Background(), a, "cus_example"); err != nil {
		t.Fatal(err)
	}
	if calls != 4 || effects != 1 || len(bodies) != 2 {
		t.Fatal("unexpected recovery", calls, effects, len(bodies))
	}
}

func TestAmbiguousOrUnrelatedSessionFailuresNeverUseAnotherKey(t *testing.T) {
	a := application.CheckoutAttempt{ID: uuid.New(), ShopID: uuid.New(), Intent: domain.CheckoutPublish, PriceID: "price_monthly", ReturnURL: "http://127.0.0.1:3000/admin/test/configuracoes"}
	for _, tc := range []struct {
		name        string
		status      int
		kind, param string
	}{
		{"network", 0, "", ""}, {"server error", 503, "api_error", "payment_method_types"},
		{"idempotency conflict", 400, "idempotency_error", "payment_method_types"},
		{"other parameter", 400, "invalid_request_error", "customer"},
		{"invalid success", 200, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.status == 0 {
					return nil, io.ErrUnexpectedEOF
				}
				response := jsonResponse(map[string]any{"error": map[string]string{"type": tc.kind, "param": tc.param}})
				response.StatusCode = tc.status
				return response, nil
			})})
			if _, err := c.CreateCheckoutSession(context.Background(), a, "cus_example"); err == nil || calls != 1 {
				t.Fatal("unsafe alternative request", err, calls)
			}
		})
	}
}
