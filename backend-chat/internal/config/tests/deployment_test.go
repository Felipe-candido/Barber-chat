package config_test

import "testing"

func TestExplicitLiveBillingConfiguration(t *testing.T) {
	base := map[string]string{"APP_ENV": "production", "STRIPE_MODE": "live", "DATABASE_URL": "postgres://localhost/test", "FRONTEND_ORIGIN": "https://app.example", "STRIPE_SECRET_KEY": "sk_live_example", "STRIPE_MONTHLY_PRICE_ID": "price_example", "STRIPE_WEBHOOK_SECRET": "whsec_example", "TRUSTED_PROXY_CIDRS": "10.20.0.2/32"}
	for _, tc := range []struct {
		name, key, value string
		valid            bool
	}{
		{"live explicitly enabled", "", "", true}, {"test key in live mode", "STRIPE_SECRET_KEY", "sk_test_example", false},
		{"live requires production", "APP_ENV", "development", false}, {"live requires HTTPS", "FRONTEND_ORIGIN", "http://127.0.0.1:3000", false},
		{"live requires webhook", "STRIPE_WEBHOOK_SECRET", "", false}, {"never trust entire Internet", "TRUSTED_PROXY_CIDRS", "0.0.0.0/0", false},
		{"invalid mode", "STRIPE_MODE", "automatic", false}, {"production cannot use dev override", "DEV_FRONTEND_ORIGIN", "https://app.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			for k, v := range base {
				values[k] = v
			}
			if tc.key != "" {
				values[tc.key] = tc.value
			}
			cfg, err := loadConfig(t, func(k string) string { return values[k] })
			if (err == nil) != tc.valid {
				t.Fatal(err)
			}
			if err == nil && cfg.StripeMode != "live" {
				t.Fatal(cfg.StripeMode)
			}
		})
	}
}
