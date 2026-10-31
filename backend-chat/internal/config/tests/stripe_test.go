package config_test

import (
	"strings"
	"testing"
)

func TestStripeCheckoutConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, key, price, origin, want string
	}{
		{"disabled", "", "", "", ""},
		{"test mode", "sk_test_example", "price_monthly", "http://127.0.0.1:3000", ""},
		{"live forbidden", "sk_live_secret", "price_monthly", "https://example.com", "STRIPE_SECRET_KEY"},
		{"public key forbidden", "pk_test_secret", "price_monthly", "https://example.com", "STRIPE_SECRET_KEY"},
		{"incomplete configuration", "sk_test_secret", "", "https://example.com", "STRIPE_MONTHLY_PRICE_ID"},
		{"invalid price", "sk_test_secret", "price_bad/path", "https://example.com", "STRIPE_MONTHLY_PRICE_ID"},
		{"missing origin", "sk_test_secret", "price_monthly", "", "FRONTEND_ORIGIN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://localhost/test", "STRIPE_SECRET_KEY": tc.key,
				"STRIPE_MONTHLY_PRICE_ID": tc.price, "FRONTEND_ORIGIN": tc.origin}
			_, err := loadConfig(t, func(name string) string { return values[name] })
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
				t.Fatal("expected sanitized configuration error", err)
			}
		})
	}
}

func TestStripeWebhookConfiguration(t *testing.T) {
	for _, tc := range []struct{ secret, key, want string }{
		{"", "sk_test_example", ""}, {"whsec_example", "sk_test_example", ""},
		{"wrong_secret", "sk_test_example", "STRIPE_WEBHOOK_SECRET"}, {"whsec_example", "", "STRIPE_SECRET_KEY"},
	} {
		t.Run(tc.secret+tc.want, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://localhost/test", "STRIPE_WEBHOOK_SECRET": tc.secret, "STRIPE_SECRET_KEY": tc.key}
			if tc.key != "" {
				values["STRIPE_MONTHLY_PRICE_ID"] = "price_monthly"
				values["FRONTEND_ORIGIN"] = "http://127.0.0.1:3000"
			}
			_, err := loadConfig(t, func(name string) string { return values[name] })
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "wrong_secret") {
				t.Fatal("invalid or exposed configuration error", err)
			}
		})
	}
}

func TestStripePortalConfiguration(t *testing.T) {
	for _, id := range []string{"", "bpc_test", "bpc_bad/path", "bpc_", "https://example.com"} {
		values := map[string]string{"DATABASE_URL": "postgres://localhost/test", "STRIPE_SECRET_KEY": "sk_test_example", "STRIPE_MONTHLY_PRICE_ID": "price_monthly", "FRONTEND_ORIGIN": "http://127.0.0.1:3000", "STRIPE_PORTAL_CONFIGURATION_ID": id}
		_, err := loadConfig(t, func(name string) string { return values[name] })
		if id == "" || id == "bpc_test" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "STRIPE_PORTAL_CONFIGURATION_ID") {
			t.Fatal("invalid portal config accepted")
		}
	}
}
