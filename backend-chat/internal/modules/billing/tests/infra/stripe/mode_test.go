package stripe_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
)

func TestBillingEnvironmentIsValidatedOnEveryExternalBoundary(t *testing.T) {
	now := time.Now()
	for _, mode := range []domain.BillingMode{domain.BillingTest, domain.BillingLive} {
		t.Run(string(mode), func(t *testing.T) {
			wrong := domain.BillingLive
			if mode.Live() {
				wrong = domain.BillingTest
			}
			if _, err := stripe.NewClientForMode("sk_"+string(wrong)+"_example", mode, nil); err == nil {
				t.Fatal("key from another environment accepted")
			}
			verifier, err := stripe.NewWebhookVerifierForMode("whsec_test_secret", application.ClockFunc(func() time.Time { return now }), mode)
			if err != nil {
				t.Fatal(err)
			}
			for _, live := range []bool{false, true} {
				body, _ := json.Marshal(map[string]any{"id": "evt_mode", "object": "event", "type": "customer.created", "api_version": stripe.APIVersion, "created": now.Unix(), "livemode": live, "data": map[string]any{"object": map[string]any{"id": "cus_example"}}})
				event, err := verifier.VerifyWebhook(body, webhookSignature(body, now))
				if live == mode.Live() {
					if err != nil || event.LiveMode != live {
						t.Fatal("valid mode rejected", err)
					}
				} else if err == nil {
					t.Fatal("signed event from another mode accepted")
				}
			}
			a := application.CheckoutAttempt{ID: uuid.New(), ShopID: uuid.New()}
			response := sessionJSON(a)
			response["id"] = "cs_" + string(mode) + "_example"
			response["livemode"] = mode.Live()
			client, err := stripe.NewClientForMode("sk_"+string(mode)+"_example", mode, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(response), nil })})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.FindCheckoutSession(context.Background(), response["id"].(string)); err != nil {
				t.Fatal("valid session rejected", err)
			}
			response["livemode"] = !mode.Live()
			if _, err := client.FindCheckoutSession(context.Background(), response["id"].(string)); err == nil {
				t.Fatal("cross-mode session accepted")
			}
			if _, err := client.FindCheckoutSession(context.Background(), "cs_"+string(wrong)+"_example"); err == nil {
				t.Fatal("cross-mode session ID accepted")
			}
			if _, err := client.ResolveBillingReference(context.Background(), application.VerifiedWebhook{Kind: "checkout", ResourceID: "cs_" + string(mode) + "_example", LiveMode: !mode.Live()}); err == nil {
				t.Fatal("cross-mode event reconciliation accepted")
			}
		})
	}
}

func TestLivePortalChecksCustomerConfigurationAndSessionMode(t *testing.T) {
	input := application.PortalRequest{ShopID: uuid.New(), CustomerID: "cus_live", ConfigurationID: "bpc_live", ReturnURL: "https://app.example/admin/shop/configuracoes"}
	for _, wrong := range []string{"", "policy", "customer", "session"} {
		t.Run(wrong, func(t *testing.T) {
			c, err := stripe.NewClientForMode("sk_live_example", domain.BillingLive, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/v1/billing_portal/configurations/bpc_live":
					p := portalPolicy()
					p["id"] = "bpc_live"
					p["livemode"] = wrong != "policy"
					return jsonResponse(p), nil
				case "/v1/customers/cus_live":
					return jsonResponse(map[string]any{"id": "cus_live", "object": "customer", "livemode": wrong != "customer", "metadata": map[string]string{"shop_id": input.ShopID.String()}}), nil
				case "/v1/billing_portal/sessions":
					return jsonResponse(map[string]any{"id": "bps_live", "object": "billing_portal.session", "livemode": wrong != "session", "customer": input.CustomerID, "configuration": input.ConfigurationID, "return_url": input.ReturnURL, "url": "https://billing.stripe.com/p/session/live", "created": time.Now().Unix()}), nil
				default:
					t.Fatal("unexpected provider path", r.URL.Path)
					return nil, nil
				}
			})})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CreatePortalSession(context.Background(), input)
			if (err == nil) != (wrong == "") {
				t.Fatal("incorrect portal environment decision", wrong, err)
			}
		})
	}
}
