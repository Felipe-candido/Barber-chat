package stripe_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
)

func TestAuthoritativeStripeSnapshotAndPaidInvoiceValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any, map[string]any, map[string]any)
		paid   bool
		status domain.SubscriptionStatus
	}{
		{"paid monthly", func(_, _, _ map[string]any) {}, true, domain.SubscriptionActive},
		{"active without paid invoice", func(_, invoice, _ map[string]any) { invoice["status"] = "open" }, false, domain.SubscriptionActive},
		{"canceled current state", func(sub, _, _ map[string]any) { sub["status"] = "canceled" }, true, domain.SubscriptionCanceled},
		{"collection paused", func(sub, _, _ map[string]any) { sub["pause_collection"] = map[string]any{"behavior": "void"} }, true, domain.SubscriptionSuspended},
		{"trial never grants", func(sub, _, _ map[string]any) { sub["status"] = "trialing" }, true, domain.SubscriptionSuspended},
		{"manual paid not accepted", func(_, _, payments map[string]any) { payments["data"] = []any{} }, false, domain.SubscriptionActive},
		{"credit note invalidates", func(_, invoice, _ map[string]any) { invoice["post_payment_credit_notes_amount"] = 100 }, false, domain.SubscriptionActive},
		{"proration not coverage", func(_, invoice, _ map[string]any) {
			line := invoice["lines"].(map[string]any)["data"].([]any)[0].(map[string]any)
			line["parent"].(map[string]any)["subscription_item_details"].(map[string]any)["proration"] = true
		}, false, domain.SubscriptionActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shop, attempt := uuid.New(), uuid.New()
			a := application.CheckoutAttempt{ID: attempt, ShopID: shop, PriceID: "price_example", SessionID: "cs_test_example"}
			session := sessionJSON(a)
			session["status"] = "complete"
			session["url"] = nil
			session["subscription"] = "sub_example"
			price := map[string]any{"id": "price_example", "currency": "brl", "type": "recurring", "unit_amount": 9900, "recurring": map[string]any{"interval": "month", "interval_count": 1}}
			sub := map[string]any{"id": "sub_example", "object": "subscription", "customer": "cus_example", "livemode": false, "status": "active", "currency": "brl", "collection_method": "charge_automatically", "latest_invoice": "in_example",
				"items": map[string]any{"has_more": false, "data": []any{map[string]any{"quantity": 1, "price": price}}}}
			invoice := map[string]any{"id": "in_example", "object": "invoice", "customer": "cus_example", "livemode": false, "status": "paid", "currency": "brl", "collection_method": "charge_automatically", "billing_reason": "subscription_create", "amount_paid": 9900, "amount_due": 9900, "amount_remaining": 0,
				"parent": map[string]any{"type": "subscription_details", "subscription_details": map[string]any{"subscription": "sub_example"}},
				"lines": map[string]any{"has_more": false, "data": []any{map[string]any{"currency": "brl", "amount": 9900, "quantity": 1, "period": map[string]any{"start": time.Now().Add(-time.Hour).Unix(), "end": time.Now().Add(time.Hour).Unix()},
					"pricing": map[string]any{"type": "price_details", "price_details": map[string]any{"price": "price_example"}},
					"parent":  map[string]any{"type": "subscription_item_details", "subscription_item_details": map[string]any{"subscription": "sub_example", "proration": false}}}}}}
			payments := map[string]any{"object": "list", "has_more": false, "data": []any{map[string]any{"status": "paid", "amount_paid": 9900, "invoice": "in_example", "currency": "brl", "livemode": false, "payment": map[string]any{"type": "payment_intent", "payment_intent": "pi_example"}}}}
			tc.mutate(sub, invoice, payments)
			client, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("reconciliation mutated Stripe")
				}
				switch r.URL.Path {
				case "/v1/checkout/sessions":
					if r.URL.Query().Get("subscription") != "sub_example" || r.URL.Query().Get("limit") != "2" {
						t.Fatal("unscoped Session recovery query")
					}
					return jsonResponse(map[string]any{"object": "list", "has_more": false, "data": []any{session}}), nil
				case "/v1/checkout/sessions/cs_test_example":
					return jsonResponse(session), nil
				case "/v1/subscriptions/sub_example":
					return jsonResponse(sub), nil
				case "/v1/invoices/in_example":
					return jsonResponse(invoice), nil
				case "/v1/invoice_payments":
					if r.URL.Query().Get("invoice") != "in_example" {
						t.Fatal("wrong invoice payment query")
					}
					return jsonResponse(payments), nil
				default:
					t.Fatal("unexpected request", r.URL.Path)
					return nil, nil
				}
			})})
			target := application.ReconciliationTarget{Account: application.CheckoutAccount{ShopID: shop, CustomerID: "cus_example"}, Attempt: a, SubscriptionID: "sub_example"}
			if tc.name == "paid monthly" {
				target.Attempt.SessionID = ""
			}
			got, err := client.ReadSubscriptionSnapshot(context.Background(), target)
			if err != nil || got.Status != tc.status || len(got.Invoices) != 1 || got.Invoices[0].Eligible != tc.paid {
				t.Fatal(got, err)
			}
		})
	}
}
