package config

import (
	"errors"
	"net/netip"
	"strings"
)

func validateDeployment(c Config) error {
	if c.AppEnvironment != "development" && c.AppEnvironment != "test" && c.AppEnvironment != "production" {
		return errors.New("APP_ENV must be development, test or production")
	}
	if c.StripeMode != "test" && c.StripeMode != "live" {
		return errors.New("STRIPE_MODE must be test or live")
	}
	if c.StripeMode == "live" && (c.AppEnvironment != "production" || !strings.HasPrefix(c.FrontendOrigin, "https://") || c.StripeWebhookSecret == "" || c.StripeSecretKey == "" || c.StripeMonthlyPriceID == "") {
		return errors.New("live billing requires APP_ENV=production, HTTPS FRONTEND_ORIGIN, secret key, price and webhook secret")
	}
	if c.AppEnvironment == "production" && (c.DevFrontendOrigin != "" || !strings.HasPrefix(c.FrontendOrigin, "https://")) {
		return errors.New("production requires HTTPS FRONTEND_ORIGIN and no DEV_FRONTEND_ORIGIN")
	}
	if c.TrustedProxyCIDRs != "" {
		parts := strings.Split(c.TrustedProxyCIDRs, ",")
		if len(parts) > 16 {
			return errors.New("TRUSTED_PROXY_CIDRS accepts at most 16 explicit proxy networks")
		}
		for _, part := range parts {
			p, err := netip.ParsePrefix(strings.TrimSpace(part))
			if err != nil || p.Bits() == 0 || p.Addr().Is4In6() {
				return errors.New("TRUSTED_PROXY_CIDRS must contain explicit CIDRs, never an all-address network")
			}
		}
	}
	return nil
}
