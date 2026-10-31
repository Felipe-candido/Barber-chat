package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail("Check the backend .env configuration.")
	}
	client, err := stripe.NewClient(cfg.StripeSecretKey, nil)
	if err != nil {
		fail("A server-side Stripe TEST key is required.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if cfg.StripePortalConfigurationID != "" {
		if err := client.CheckPortalConfiguration(ctx, cfg.StripePortalConfigurationID); err != nil {
			fail("Configured TEST portal policy could not be validated. No replacement was created.")
		}
		fmt.Printf("STRIPE_PORTAL_CONFIGURATION_ID=%s\n", cfg.StripePortalConfigurationID)
		return
	}
	id, err := client.ProvisionTestPortalConfiguration(ctx)
	if err != nil {
		var failure *application.ProviderFailure
		if errors.As(err, &failure) {
			fmt.Fprintf(os.Stderr, "Stripe status=%d code=%s request=%s\n", failure.HTTPStatus, failure.Code, failure.RequestID)
		}
		fail("TEST portal configuration was not confirmed. Check Stripe Workbench; no subscription was changed.")
	}
	fmt.Printf("STRIPE_PORTAL_CONFIGURATION_ID=%s\n", id)
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
