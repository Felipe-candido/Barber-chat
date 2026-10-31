package stripe

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
)

var _ application.PortalProvider = (*Client)(nil)

type portalToggle struct {
	Enabled *bool `json:"enabled"`
}
type portalConfiguration struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Active      *bool             `json:"active"`
	LiveMode    *bool             `json:"livemode"`
	Application json.RawMessage   `json:"application"`
	Metadata    map[string]string `json:"metadata"`
	LoginPage   portalToggle      `json:"login_page"`
	Features    struct {
		CustomerUpdate      portalToggle `json:"customer_update"`
		InvoiceHistory      portalToggle `json:"invoice_history"`
		PaymentMethodUpdate portalToggle `json:"payment_method_update"`
		SubscriptionUpdate  portalToggle `json:"subscription_update"`
		SubscriptionCancel  struct {
			Enabled           *bool           `json:"enabled"`
			Mode              string          `json:"mode"`
			ProrationBehavior string          `json:"proration_behavior"`
			Retention         json.RawMessage `json:"retention"`
		} `json:"subscription_cancel"`
	} `json:"features"`
}

func portalBool(value *bool, want bool) bool { return value != nil && *value == want }
func portalNull(value json.RawMessage) bool  { return len(value) == 0 || string(value) == "null" }

func (p portalConfiguration) valid(id string) bool {
	return p.ID == id && validID(id, "bpc_") && p.Object == "billing_portal.configuration" &&
		portalBool(p.Active, true) && portalBool(p.LiveMode, false) && portalNull(p.Application) &&
		p.Metadata["barber_portal_protocol"] == "1" && portalBool(p.LoginPage.Enabled, false) &&
		portalBool(p.Features.CustomerUpdate.Enabled, false) && portalBool(p.Features.SubscriptionUpdate.Enabled, false) &&
		portalBool(p.Features.InvoiceHistory.Enabled, true) && portalBool(p.Features.PaymentMethodUpdate.Enabled, true) &&
		portalBool(p.Features.SubscriptionCancel.Enabled, true) && p.Features.SubscriptionCancel.Mode == "at_period_end" &&
		p.Features.SubscriptionCancel.ProrationBehavior == "none" && portalNull(p.Features.SubscriptionCancel.Retention)
}

// ProvisionTestPortalConfiguration creates a dedicated, repeatable TEST policy.
// It does not change existing Customers, subscriptions or the account default.
func (c *Client) ProvisionTestPortalConfiguration(ctx context.Context) (string, error) {
	var configuration portalConfiguration
	err := c.request(ctx, "POST", "/billing_portal/configurations", "barber:test:portal-policy:v1:reasons-v2", url.Values{
		"name":                                                           {"BarberChat - gerenciamento de assinatura TEST v1"},
		"metadata[barber_portal_protocol]":                               {"1"},
		"login_page[enabled]":                                            {"false"},
		"features[customer_update][enabled]":                             {"false"},
		"features[invoice_history][enabled]":                             {"true"},
		"features[payment_method_update][enabled]":                       {"true"},
		"features[subscription_update][enabled]":                         {"false"},
		"features[subscription_cancel][enabled]":                         {"true"},
		"features[subscription_cancel][mode]":                            {"at_period_end"},
		"features[subscription_cancel][proration_behavior]":              {"none"},
		"features[subscription_cancel][cancellation_reason][enabled]":    {"false"},
		"features[subscription_cancel][cancellation_reason][options][0]": {"other"},
		"features[subscription_cancel][cancellation_reason][options][1]": {"unused"},
	}, &configuration)
	if err != nil {
		return "", err
	}
	if !configuration.valid(configuration.ID) {
		return "", ErrProviderResponse
	}
	return configuration.ID, nil
}

func (c *Client) CheckPortalConfiguration(ctx context.Context, id string) error {
	if !validID(id, "bpc_") {
		return ErrProviderResponse
	}
	var configuration portalConfiguration
	if err := c.request(ctx, "GET", "/billing_portal/configurations/"+id, "", nil, &configuration); err != nil {
		return err
	}
	if !configuration.valid(id) {
		return ErrProviderResponse
	}
	return nil
}

func (c *Client) CreatePortalSession(ctx context.Context, input application.PortalRequest) (application.PortalSession, error) {
	if input.ShopID == uuid.Nil || !validID(input.CustomerID, "cus_") || !validID(input.ConfigurationID, "bpc_") {
		return application.PortalSession{}, ErrProviderResponse
	}
	back, err := url.Parse(input.ReturnURL)
	if err != nil || back.Hostname() == "" || back.User != nil || (back.Scheme != "http" && back.Scheme != "https") {
		return application.PortalSession{}, ErrProviderResponse
	}
	if err := c.CheckPortalConfiguration(ctx, input.ConfigurationID); err != nil {
		return application.PortalSession{}, err
	}
	var customer struct {
		ID       string            `json:"id"`
		Object   string            `json:"object"`
		LiveMode *bool             `json:"livemode"`
		Deleted  bool              `json:"deleted"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := c.request(ctx, "GET", "/customers/"+input.CustomerID, "", nil, &customer); err != nil {
		return application.PortalSession{}, err
	}
	if customer.ID != input.CustomerID || customer.Object != "customer" || !portalBool(customer.LiveMode, false) || customer.Deleted || customer.Metadata["shop_id"] != input.ShopID.String() {
		return application.PortalSession{}, ErrProviderResponse
	}
	var session struct {
		ID            string          `json:"id"`
		Object        string          `json:"object"`
		Configuration string          `json:"configuration"`
		Customer      string          `json:"customer"`
		Created       int64           `json:"created"`
		LiveMode      *bool           `json:"livemode"`
		ReturnURL     string          `json:"return_url"`
		URL           string          `json:"url"`
		Flow          json.RawMessage `json:"flow"`
		OnBehalfOf    json.RawMessage `json:"on_behalf_of"`
	}
	// Each explicit click creates a fresh short-lived session. Never retry POST
	// automatically or reuse a shop-wide key that might return an expired URL.
	err = c.request(ctx, "POST", "/billing_portal/sessions", "barber:test:portal:"+uuid.NewString(), url.Values{
		"customer": {input.CustomerID}, "configuration": {input.ConfigurationID}, "return_url": {input.ReturnURL}, "locale": {"pt-BR"},
	}, &session)
	if err != nil {
		return application.PortalSession{}, err
	}
	if !validID(session.ID, "bps_") || session.Object != "billing_portal.session" || !portalBool(session.LiveMode, false) ||
		session.Customer != input.CustomerID || session.Configuration != input.ConfigurationID || session.ReturnURL != input.ReturnURL ||
		!portalNull(session.Flow) || !portalNull(session.OnBehalfOf) || !ValidPortalURL(session.URL) || session.Created <= 0 {
		return application.PortalSession{}, ErrProviderResponse
	}
	return application.PortalSession{URL: session.URL, CreatedAt: time.Unix(session.Created, 0).UTC()}, nil
}

func ValidPortalURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 8192 && u.Scheme == "https" && u.Host == "billing.stripe.com" && u.User == nil && u.Fragment == "" &&
		(u.Path == "/p/session" || (len(u.Path) > len("/p/session/") && u.Path[:len("/p/session/")] == "/p/session/"))
}
