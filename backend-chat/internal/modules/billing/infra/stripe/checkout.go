package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
)

// Fixed API origin and version: callers cannot supply a URL that receives the
// secret. These few endpoints use net/http; no SDK types escape this adapter.
const APIVersion = "2026-10-28.endive"

var ErrProviderResponse = errors.New("Stripe checkout request could not be confirmed")
var providerID = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var pricePattern = regexp.MustCompile(`^price_[A-Za-z0-9]+$`)

type Client struct {
	secret string
	http   *http.Client
}

var _ application.CheckoutProvider = (*Client)(nil)

func ValidTestKey(key string) bool {
	return len(key) > len("sk_test_") && len(key) <= 512 && strings.HasPrefix(key, "sk_test_") && providerID.MatchString(key)
}

func ValidPriceID(id string) bool {
	return len(id) <= 255 && pricePattern.MatchString(id)
}

func NewClient(secret string, client *http.Client) (*Client, error) {
	if !ValidTestKey(secret) {
		return nil, errors.New("Stripe Checkout requires a server-side sk_test_ key; live mode is disabled")
	}
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	copy.Timeout = 8 * time.Second
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{secret: secret, http: &copy}, nil
}

func (c *Client) request(ctx context.Context, method, path, key string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.stripe.com/v1"+path, body)
	if err != nil {
		return ErrProviderResponse
	}
	req.SetBasicAuth(c.secret, "")
	req.Header.Set("Stripe-Version", APIVersion)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &application.ProviderFailure{Code: "transport_error"}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024+1))
	if err != nil || len(data) > 512*1024 {
		return &application.ProviderFailure{HTTPStatus: resp.StatusCode, Code: "invalid_response"}
	}
	if resp.StatusCode != http.StatusOK {
		return safeProviderFailure(resp, data)
	}
	if json.Unmarshal(data, out) != nil {
		return &application.ProviderFailure{HTTPStatus: resp.StatusCode, Code: "invalid_response"}
	}
	return nil
}

func safeProviderFailure(resp *http.Response, data []byte) error {
	failure := &application.ProviderFailure{HTTPStatus: resp.StatusCode, Code: "provider_error"}
	var envelope struct {
		Error struct{ Code, Type, Param string } `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil {
		// Allowlisted identifiers only; never log an arbitrary provider string.
		for _, code := range []string{envelope.Error.Code, envelope.Error.Type} {
			switch code {
			case "parameter_unknown", "parameter_missing", "resource_missing", "idempotency_key_in_use", "idempotency_error", "invalid_request_error", "authentication_error", "api_key_expired", "rate_limit", "rate_limit_error", "api_error":
				failure.Code = code
			}
			if failure.Code != "provider_error" {
				break
			}
		}
		switch envelope.Error.Param {
		case "payment_method_types", "allowed_payment_method_types", "line_items", "line_items[0][price]", "customer", "success_url", "cancel_url":
			failure.Parameter = envelope.Error.Param
		}
	}
	if id := resp.Header.Get("Request-Id"); validID(id, "req_") {
		failure.RequestID = id
	}
	return failure
}

func (c *Client) ReadCheckoutPrice(ctx context.Context, priceID string) (application.CheckoutPrice, error) {
	if !ValidPriceID(priceID) {
		return application.CheckoutPrice{}, ErrProviderResponse
	}
	var price struct {
		ID            string          `json:"id"`
		Object        string          `json:"object"`
		Active        bool            `json:"active"`
		LiveMode      *bool           `json:"livemode"`
		Currency      string          `json:"currency"`
		Type          string          `json:"type"`
		BillingScheme string          `json:"billing_scheme"`
		UnitAmount    *int64          `json:"unit_amount"`
		CustomAmount  json.RawMessage `json:"custom_unit_amount"`
		Transform     json.RawMessage `json:"transform_quantity"`
		Recurring     *struct {
			Interval      string `json:"interval"`
			IntervalCount int64  `json:"interval_count"`
			UsageType     string `json:"usage_type"`
			TrialDays     *int64 `json:"trial_period_days"`
		} `json:"recurring"`
	}
	if err := c.request(ctx, "GET", "/prices/"+priceID, "", nil, &price); err != nil {
		return application.CheckoutPrice{}, err
	}
	if price.ID != priceID || price.Object != "price" || !price.Active || price.LiveMode == nil || *price.LiveMode ||
		price.Currency != "brl" || price.Type != "recurring" || price.BillingScheme != "per_unit" || price.UnitAmount == nil || *price.UnitAmount <= 0 || *price.UnitAmount > 9007199254740991 ||
		(len(price.CustomAmount) != 0 && string(price.CustomAmount) != "null") ||
		(len(price.Transform) != 0 && string(price.Transform) != "null") || price.Recurring == nil ||
		price.Recurring.Interval != "month" || price.Recurring.IntervalCount != 1 || price.Recurring.UsageType != "licensed" ||
		(price.Recurring.TrialDays != nil && *price.Recurring.TrialDays != 0) {
		return application.CheckoutPrice{}, ErrProviderResponse
	}
	return application.CheckoutPrice{ID: price.ID, AmountCents: *price.UnitAmount}, nil
}

func (c *Client) CreateCheckoutCustomer(ctx context.Context, shopID uuid.UUID) (string, error) {
	if shopID == uuid.Nil {
		return "", ErrProviderResponse
	}
	var customer struct {
		ID       string            `json:"id"`
		Object   string            `json:"object"`
		LiveMode *bool             `json:"livemode"`
		Metadata map[string]string `json:"metadata"`
	}
	// Deliberately no mutable shop name/email/address in an idempotent request.
	err := c.request(ctx, "POST", "/customers", "barber:test:customer:"+shopID.String(), url.Values{
		"metadata[shop_id]": {shopID.String()},
	}, &customer)
	if err != nil {
		return "", err
	}
	if !validID(customer.ID, "cus_") || customer.Object != "customer" || customer.LiveMode == nil || *customer.LiveMode || customer.Metadata["shop_id"] != shopID.String() {
		return "", ErrProviderResponse
	}
	return customer.ID, nil
}

func (c *Client) CreateCheckoutSession(ctx context.Context, a application.CheckoutAttempt, customerID string) (application.CheckoutSession, error) {
	if a.ID == uuid.Nil || a.ShopID == uuid.Nil || !a.Intent.Valid() || !ValidPriceID(a.PriceID) || !validID(customerID, "cus_") {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	returnURL, err := url.Parse(a.ReturnURL)
	if err != nil || returnURL.Hostname() == "" || returnURL.User != nil || returnURL.RawQuery != "" || returnURL.Fragment != "" {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	form := url.Values{
		"mode": {"subscription"}, "customer": {customerID},
		"line_items[0][price]": {a.PriceID}, "line_items[0][quantity]": {"1"},
		"payment_method_types[0]": {"card"}, "locale": {"pt-BR"},
		"client_reference_id": {a.ID.String()},
		"metadata[shop_id]":   {a.ShopID.String()}, "metadata[attempt_id]": {a.ID.String()},
		"subscription_data[metadata][shop_id]":    {a.ShopID.String()},
		"subscription_data[metadata][attempt_id]": {a.ID.String()},
		"success_url": {a.ReturnURL + "?checkout=returned"},
		"cancel_url":  {a.ReturnURL + "?checkout=canceled"},
	}
	var raw sessionJSON
	key := "barber:test:checkout:" + a.ID.String()
	err = c.request(ctx, "POST", "/checkout/sessions", key, form, &raw)
	if legacyPaymentParameterRejected(err) {
		// Stripe can cache the old 400, so replacing its body under the same
		// key gives idempotency_error. Only this explicit parameter rejection
		// proves the original request did not create a Session. The corrected
		// wire format has its own deterministic key for the SAME local attempt.
		// Ambiguous failures, other errors and successful old responses never
		// enter this branch. Lost corrected responses reuse the same v2 key.
		form.Del("payment_method_types[0]")
		form.Set("allowed_payment_method_types[0]", "card")
		err = c.request(ctx, "POST", "/checkout/sessions", key+":card-filter-v2", form, &raw)
	}
	if err != nil {
		return application.CheckoutSession{}, err
	}
	return parseSession(raw)
}

func legacyPaymentParameterRejected(err error) bool {
	var failure *application.ProviderFailure
	return errors.As(err, &failure) && failure.HTTPStatus == 400 &&
		failure.Parameter == "payment_method_types" &&
		(failure.Code == "invalid_request_error" || failure.Code == "parameter_unknown")
}

func (c *Client) FindCheckoutSession(ctx context.Context, id string) (application.CheckoutSession, error) {
	if !validID(id, "cs_test_") {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	var raw sessionJSON
	if err := c.request(ctx, "GET", "/checkout/sessions/"+id, "", nil, &raw); err != nil {
		return application.CheckoutSession{}, err
	}
	result, err := parseSession(raw)
	if result.ID != id {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	return result, err
}

type sessionJSON struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"`
	Customer          string            `json:"customer"`
	LiveMode          *bool             `json:"livemode"`
	Mode              string            `json:"mode"`
	Status            string            `json:"status"`
	URL               string            `json:"url"`
	ClientReferenceID string            `json:"client_reference_id"`
	ExpiresAt         int64             `json:"expires_at"`
	Metadata          map[string]string `json:"metadata"`
	Subscription      string            `json:"subscription"`
	Currency          string            `json:"currency"`
	AmountSubtotal    int64             `json:"amount_subtotal"`
	AmountTotal       int64             `json:"amount_total"`
}

func parseSession(raw sessionJSON) (application.CheckoutSession, error) {
	attempt, aerr := uuid.Parse(raw.ClientReferenceID)
	shop, serr := uuid.Parse(raw.Metadata["shop_id"])
	if aerr != nil || serr != nil || attempt == uuid.Nil || shop == uuid.Nil || raw.Metadata["attempt_id"] != attempt.String() ||
		!validID(raw.ID, "cs_test_") || !validID(raw.Customer, "cus_") || raw.Object != "checkout.session" ||
		raw.LiveMode == nil || *raw.LiveMode || raw.Mode != "subscription" || raw.ExpiresAt <= 0 || raw.ExpiresAt > 253402300799 {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	if raw.Status != "open" && raw.Status != "complete" && raw.Status != "expired" {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	if raw.Status == "open" || raw.URL != "" {
		u, err := url.Parse(raw.URL)
		if err != nil || u.Scheme != "https" || u.Host != "checkout.stripe.com" || u.User != nil || !strings.HasPrefix(u.Path, "/c/") {
			return application.CheckoutSession{}, ErrProviderResponse
		}
	}
	return application.CheckoutSession{ID: raw.ID, CustomerID: raw.Customer, AttemptID: attempt, ShopID: shop,
		URL: raw.URL, Status: raw.Status, SubscriptionID: raw.Subscription, ExpiresAt: time.Unix(raw.ExpiresAt, 0).UTC(),
		Currency: raw.Currency, AmountSubtotal: raw.AmountSubtotal, AmountTotal: raw.AmountTotal}, nil
}

func validID(id, prefix string) bool {
	return len(id) > len(prefix) && len(id) <= 255 && strings.HasPrefix(id, prefix) && providerID.MatchString(id)
}
