package stripe_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
)

func portalPolicy() map[string]any {
	return map[string]any{"id": "bpc_test", "object": "billing_portal.configuration", "active": true, "livemode": false,
		"metadata": map[string]any{"barber_portal_protocol": "1"}, "login_page": map[string]any{"enabled": false},
		"features": map[string]any{
			"customer_update": map[string]any{"enabled": false}, "subscription_update": map[string]any{"enabled": false},
			"invoice_history": map[string]any{"enabled": true}, "payment_method_update": map[string]any{"enabled": true},
			"subscription_cancel": map[string]any{"enabled": true, "mode": "at_period_end", "proration_behavior": "none"},
		}}
}

func TestPortalProviderContractAndPolicy(t *testing.T) {
	input := application.PortalRequest{ShopID: uuid.New(), CustomerID: "cus_test", ConfigurationID: "bpc_test", ReturnURL: "http://127.0.0.1:3000/admin/test/configuracoes?portal=returned#publication"}
	for _, scenario := range []string{"valid", "live policy", "inactive policy", "plan updates", "immediate cancellation", "retention coupon", "public login", "other customer tenant", "deleted customer", "live customer", "wrong session customer", "wrong session policy", "wrong return", "live session", "foreign URL", "Connect session", "deep link"} {
		t.Run(scenario, func(t *testing.T) {
			calls, posts := 0, 0
			client, err := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "api.stripe.com" || r.URL.Scheme != "https" || r.Header.Get("Stripe-Version") != stripe.APIVersion {
					t.Fatal("unsafe endpoint")
				}
				key, _, _ := r.BasicAuth()
				if key != "sk_test_example" {
					t.Fatal("missing server authentication")
				}
				switch r.URL.Path {
				case "/v1/billing_portal/configurations/bpc_test":
					policy := portalPolicy()
					features := policy["features"].(map[string]any)
					cancel := features["subscription_cancel"].(map[string]any)
					switch scenario {
					case "live policy":
						policy["livemode"] = true
					case "inactive policy":
						policy["active"] = false
					case "plan updates":
						features["subscription_update"] = map[string]any{"enabled": true}
					case "immediate cancellation":
						cancel["mode"] = "immediately"
					case "retention coupon":
						cancel["retention"] = map[string]any{"type": "coupon_offer"}
					case "public login":
						policy["login_page"] = map[string]any{"enabled": true}
					}
					return jsonResponse(policy), nil
				case "/v1/customers/cus_test":
					customer := map[string]any{"id": "cus_test", "object": "customer", "livemode": false, "metadata": map[string]any{"shop_id": input.ShopID.String()}}
					if scenario == "other customer tenant" {
						customer["metadata"] = map[string]any{"shop_id": uuid.NewString()}
					}
					if scenario == "deleted customer" {
						customer["deleted"] = true
					}
					if scenario == "live customer" {
						customer["livemode"] = true
					}
					return jsonResponse(customer), nil
				case "/v1/billing_portal/sessions":
					posts++
					if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Idempotency-Key"), "barber:test:portal:") {
						t.Fatal("invalid session request")
					}
					if err := r.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if len(r.PostForm) != 4 || r.PostForm.Get("customer") != input.CustomerID || r.PostForm.Get("configuration") != input.ConfigurationID || r.PostForm.Get("return_url") != input.ReturnURL || r.PostForm.Get("locale") != "pt-BR" {
						t.Fatal("untrusted session parameters")
					}
					s := map[string]any{"id": "bps_test", "object": "billing_portal.session", "livemode": false, "configuration": input.ConfigurationID, "customer": input.CustomerID, "return_url": input.ReturnURL, "created": time.Now().Unix(), "url": "https://billing.stripe.com/p/session/test"}
					switch scenario {
					case "wrong session customer":
						s["customer"] = "cus_other"
					case "wrong session policy":
						s["configuration"] = "bpc_other"
					case "wrong return":
						s["return_url"] = "https://elsewhere.test"
					case "live session":
						s["livemode"] = true
					case "foreign URL":
						s["url"] = "https://billing.stripe.com.evil.test/p/session/test"
					case "Connect session":
						s["on_behalf_of"] = "acct_other"
					case "deep link":
						s["flow"] = map[string]any{"type": "subscription_cancel"}
					}
					return jsonResponse(s), nil
				default:
					t.Fatal("unexpected endpoint")
					return nil, nil
				}
			})})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.CreatePortalSession(context.Background(), input)
			if scenario == "valid" {
				if err != nil || result.URL == "" || calls != 3 || posts != 1 {
					t.Fatal("valid portal rejected", err)
				}
			} else if err == nil || result.URL != "" {
				t.Fatal("incompatible portal accepted")
			}
		})
	}
}

func TestPortalURLValidation(t *testing.T) {
	for _, raw := range []string{"https://billing.stripe.com/p/session?secret=test_abc", "https://billing.stripe.com/p/session/test_abc"} {
		if !stripe.ValidPortalURL(raw) {
			t.Fatal("valid URL rejected")
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "http://billing.stripe.com/p/session/test", "https://billing.stripe.com.evil.test/p/session/test", "https://user:secret@billing.stripe.com/p/session/test", "https://billing.stripe.com:444/p/session/test", "https://billing.stripe.com/other", "https://billing.stripe.com/p/session/test#fragment"} {
		if stripe.ValidPortalURL(raw) {
			t.Fatal("unsafe URL accepted")
		}
	}
}

func TestPortalPolicyProvisioning(t *testing.T) {
	client, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v1/billing_portal/configurations" || r.Header.Get("Idempotency-Key") != "barber:test:portal-policy:v1:reasons-v2" {
			t.Fatal("unexpected provisioning")
		}
		_ = r.ParseForm()
		if r.PostForm.Get("features[subscription_cancel][mode]") != "at_period_end" || r.PostForm.Get("features[subscription_update][enabled]") != "false" || r.PostForm.Get("login_page[enabled]") != "false" {
			t.Fatal("unsafe policy")
		}
		if r.PostForm.Get("features[subscription_cancel][cancellation_reason][options][0]") != "other" || r.PostForm.Get("features[subscription_cancel][cancellation_reason][options][1]") != "unused" {
			t.Fatal("Stripe requires at least two reason options even when collection is disabled")
		}
		return jsonResponse(portalPolicy()), nil
	})})
	if id, err := client.ProvisionTestPortalConfiguration(context.Background()); err != nil || id != "bpc_test" {
		t.Fatal("provision failed", err)
	}
}
