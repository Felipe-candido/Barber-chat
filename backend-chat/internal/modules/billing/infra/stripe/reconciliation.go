package stripe

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

var _ application.WebhookProvider = (*Client)(nil)

type subscriptionJSON struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"`
	Customer          string            `json:"customer"`
	LiveMode          *bool             `json:"livemode"`
	Status            string            `json:"status"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	CancelAt          *int64            `json:"cancel_at"`
	LatestInvoice     string            `json:"latest_invoice"`
	CollectionMethod  string            `json:"collection_method"`
	Currency          string            `json:"currency"`
	PauseCollection   json.RawMessage   `json:"pause_collection"`
	Metadata          map[string]string `json:"metadata"`
	Items             struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Quantity int64 `json:"quantity"`
			Price    struct {
				ID         string `json:"id"`
				Currency   string `json:"currency"`
				Type       string `json:"type"`
				UnitAmount int64  `json:"unit_amount"`
				Recurring  struct {
					Interval      string `json:"interval"`
					IntervalCount int64  `json:"interval_count"`
				} `json:"recurring"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

type invoiceJSON struct {
	ID               string `json:"id"`
	Object           string `json:"object"`
	Customer         string `json:"customer"`
	LiveMode         *bool  `json:"livemode"`
	Status           string `json:"status"`
	Currency         string `json:"currency"`
	CollectionMethod string `json:"collection_method"`
	BillingReason    string `json:"billing_reason"`
	AmountPaid       int64  `json:"amount_paid"`
	AmountDue        int64  `json:"amount_due"`
	AmountRemaining  int64  `json:"amount_remaining"`
	PostCredits      int64  `json:"post_payment_credit_notes_amount"`
	PreCredits       int64  `json:"pre_payment_credit_notes_amount"`
	Parent           struct {
		Type                string `json:"type"`
		SubscriptionDetails struct {
			Subscription string `json:"subscription"`
		} `json:"subscription_details"`
	} `json:"parent"`
	Lines struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Currency string `json:"currency"`
			Amount   int64  `json:"amount"`
			Quantity int64  `json:"quantity"`
			Period   struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"period"`
			Pricing struct {
				Type         string `json:"type"`
				PriceDetails struct {
					Price string `json:"price"`
				} `json:"price_details"`
			} `json:"pricing"`
			Parent struct {
				Type                    string `json:"type"`
				SubscriptionItemDetails struct {
					Subscription string `json:"subscription"`
					Proration    bool   `json:"proration"`
				} `json:"subscription_item_details"`
			} `json:"parent"`
		} `json:"data"`
	} `json:"lines"`
	Payments struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Status     string `json:"status"`
			AmountPaid int64  `json:"amount_paid"`
			Invoice    string `json:"invoice"`
			Currency   string `json:"currency"`
			LiveMode   *bool  `json:"livemode"`
			Payment    struct {
				Type          string `json:"type"`
				PaymentIntent string `json:"payment_intent"`
			} `json:"payment"`
		} `json:"data"`
	} `json:"payments"`
}

func (c *Client) readSubscription(ctx context.Context, id string) (subscriptionJSON, error) {
	var sub subscriptionJSON
	if !validID(id, "sub_") {
		return sub, ErrProviderResponse
	}
	if err := c.request(ctx, "GET", "/subscriptions/"+id, "", nil, &sub); err != nil {
		return sub, err
	}
	if sub.ID != id || sub.Object != "subscription" || !validID(sub.Customer, "cus_") || sub.LiveMode == nil || *sub.LiveMode || sub.Status == "" {
		return sub, ErrProviderResponse
	}
	return sub, nil
}

func (c *Client) readInvoice(ctx context.Context, id string, loadPayments bool) (invoiceJSON, error) {
	var invoice invoiceJSON
	if !validID(id, "in_") {
		return invoice, ErrProviderResponse
	}
	if err := c.request(ctx, "GET", "/invoices/"+id, "", nil, &invoice); err != nil {
		return invoice, err
	}
	if invoice.ID != id || invoice.Object != "invoice" || !validID(invoice.Customer, "cus_") || invoice.LiveMode == nil || *invoice.LiveMode {
		return invoice, ErrProviderResponse
	}
	if invoice.Status == "paid" && loadPayments {
		path := "/invoice_payments?invoice=" + url.QueryEscape(id) + "&status=paid&limit=100"
		if err := c.request(ctx, "GET", path, "", nil, &invoice.Payments); err != nil {
			return invoice, err
		}
	}
	return invoice, nil
}

func (c *Client) ResolveBillingReference(ctx context.Context, event application.VerifiedWebhook) (application.BillingReference, error) {
	if event.Kind == "checkout" {
		session, err := c.FindCheckoutSession(ctx, event.ResourceID)
		if err != nil {
			return application.BillingReference{}, err
		}
		if session.SubscriptionID == "" && session.Status != "expired" {
			return application.BillingReference{}, ErrProviderResponse
		}
		return application.BillingReference{CustomerID: session.CustomerID, SubscriptionID: session.SubscriptionID, AttemptID: session.AttemptID, ShopID: session.ShopID}, nil
	}
	id := event.ResourceID
	var invoiceCustomer string
	if event.Kind == "invoice" {
		invoice, err := c.readInvoice(ctx, id, false)
		if err != nil {
			return application.BillingReference{}, err
		}
		id = invoice.Parent.SubscriptionDetails.Subscription
		if invoice.Parent.Type != "subscription_details" || id == "" {
			return application.BillingReference{CustomerID: invoice.Customer}, nil
		}
		invoiceCustomer = invoice.Customer
	}
	sub, err := c.readSubscription(ctx, id)
	if err != nil {
		return application.BillingReference{}, err
	}
	if invoiceCustomer != "" && invoiceCustomer != sub.Customer {
		return application.BillingReference{}, ErrProviderResponse
	}
	attempt, _ := uuid.Parse(sub.Metadata["attempt_id"])
	shop, _ := uuid.Parse(sub.Metadata["shop_id"])
	return application.BillingReference{CustomerID: sub.Customer, SubscriptionID: sub.ID, AttemptID: attempt, ShopID: shop}, nil
}

func (c *Client) subscriptionSession(ctx context.Context, target application.ReconciliationTarget) (application.CheckoutSession, error) {
	if target.Attempt.SessionID != "" {
		return c.FindCheckoutSession(ctx, target.Attempt.SessionID)
	}
	if target.Event.Kind == "checkout" {
		return c.FindCheckoutSession(ctx, target.Event.ResourceID)
	}
	var list struct {
		Object  string        `json:"object"`
		HasMore bool          `json:"has_more"`
		Data    []sessionJSON `json:"data"`
	}
	path := "/checkout/sessions?subscription=" + url.QueryEscape(target.SubscriptionID) + "&limit=2"
	if err := c.request(ctx, "GET", path, "", nil, &list); err != nil {
		return application.CheckoutSession{}, err
	}
	if list.Object != "list" || list.HasMore || len(list.Data) != 1 {
		return application.CheckoutSession{}, ErrProviderResponse
	}
	return parseSession(list.Data[0])
}

func (c *Client) ReadSubscriptionSnapshot(ctx context.Context, target application.ReconciliationTarget) (application.SubscriptionSnapshot, error) {
	session, err := c.subscriptionSession(ctx, target)
	if err != nil {
		return application.SubscriptionSnapshot{}, err
	}
	if session.AttemptID != target.Attempt.ID || session.ShopID != target.Account.ShopID || session.CustomerID != target.Account.CustomerID || session.SubscriptionID != target.SubscriptionID {
		return application.SubscriptionSnapshot{}, ErrProviderResponse
	}
	if target.SubscriptionID == "" {
		if session.Status != "expired" {
			return application.SubscriptionSnapshot{}, ErrProviderResponse
		}
		return application.SubscriptionSnapshot{CustomerID: session.CustomerID, Session: session}, nil
	}
	sub, err := c.readSubscription(ctx, target.SubscriptionID)
	if err != nil {
		return application.SubscriptionSnapshot{}, err
	}
	if sub.Customer != target.Account.CustomerID {
		return application.SubscriptionSnapshot{}, ErrProviderResponse
	}
	status := localStatus(sub)
	if session.Status != "complete" && (session.Status != "expired" || status != domain.SubscriptionCanceled) {
		return application.SubscriptionSnapshot{}, ErrProviderResponse
	}
	price := ""
	if len(sub.Items.Data) == 1 && !sub.Items.HasMore {
		item := sub.Items.Data[0]
		price = item.Price.ID
		if item.Quantity != 1 || item.Price.Currency != "brl" || item.Price.Type != "recurring" || item.Price.UnitAmount <= 0 || item.Price.Recurring.Interval != "month" || item.Price.Recurring.IntervalCount != 1 {
			status = domain.SubscriptionSuspended
		}
	} else {
		status = domain.SubscriptionSuspended
	}
	if sub.CollectionMethod != "charge_automatically" || sub.Currency != "brl" || price != target.Attempt.PriceID {
		status = domain.SubscriptionSuspended
	}
	result := application.SubscriptionSnapshot{ID: sub.ID, CustomerID: sub.Customer, PriceID: target.Attempt.PriceID, Status: status,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd || (sub.CancelAt != nil && *sub.CancelAt > 0), Session: session}
	ids := []string{}
	if sub.LatestInvoice != "" {
		ids = append(ids, sub.LatestInvoice)
	}
	if target.Event.Kind == "invoice" && target.Event.ResourceID != sub.LatestInvoice {
		ids = append(ids, target.Event.ResourceID)
	}
	for _, id := range ids {
		invoice, err := c.readInvoice(ctx, id, true)
		if err != nil {
			return application.SubscriptionSnapshot{}, err
		}
		if invoice.Customer != sub.Customer || invoice.Parent.SubscriptionDetails.Subscription != sub.ID {
			return application.SubscriptionSnapshot{}, ErrProviderResponse
		}
		result.Invoices = append(result.Invoices, confirmedInvoice(invoice, target.Attempt.PriceID))
	}
	return result, nil
}

func localStatus(sub subscriptionJSON) domain.SubscriptionStatus {
	if sub.Status == "canceled" || sub.Status == "incomplete_expired" {
		return domain.SubscriptionCanceled
	}
	if len(sub.PauseCollection) != 0 && string(sub.PauseCollection) != "null" {
		return domain.SubscriptionSuspended
	}
	switch sub.Status {
	case "active":
		return domain.SubscriptionActive
	case "past_due":
		return domain.SubscriptionPastDue
	case "incomplete":
		return domain.SubscriptionPending
	default:
		return domain.SubscriptionSuspended // No trial/grace or unknown-state access.
	}
}

func confirmedInvoice(invoice invoiceJSON, priceID string) application.ConfirmedInvoice {
	result := application.ConfirmedInvoice{ID: invoice.ID, PriceID: priceID}
	if invoice.Status != "paid" || invoice.Currency != "brl" || invoice.CollectionMethod != "charge_automatically" ||
		(invoice.BillingReason != "subscription_create" && invoice.BillingReason != "subscription_cycle") || invoice.AmountPaid <= 0 ||
		invoice.AmountRemaining != 0 || invoice.AmountDue != invoice.AmountPaid || invoice.PostCredits != 0 || invoice.PreCredits != 0 ||
		invoice.Lines.HasMore || len(invoice.Lines.Data) != 1 || invoice.Payments.HasMore {
		return result
	}
	line := invoice.Lines.Data[0]
	if line.Currency != "brl" || line.Amount <= 0 || line.Quantity != 1 || line.Pricing.Type != "price_details" || line.Pricing.PriceDetails.Price != priceID ||
		line.Parent.Type != "subscription_item_details" || line.Parent.SubscriptionItemDetails.Proration ||
		line.Parent.SubscriptionItemDetails.Subscription != invoice.Parent.SubscriptionDetails.Subscription ||
		line.Period.Start <= 0 || line.Period.End <= line.Period.Start || line.Period.End > 253402300799 {
		return result
	}
	var paid int64
	for _, payment := range invoice.Payments.Data {
		if payment.Status == "paid" && payment.AmountPaid > 0 && payment.AmountPaid <= invoice.AmountPaid-paid && payment.Currency == "brl" && payment.Invoice == invoice.ID &&
			payment.LiveMode != nil && !*payment.LiveMode && payment.Payment.Type == "payment_intent" && validID(payment.Payment.PaymentIntent, "pi_") {
			paid += payment.AmountPaid
		}
	}
	if paid != invoice.AmountPaid {
		return result
	} // Do not grant from manual/out-of-band "paid" alone.
	result.Eligible = true
	result.AmountPaid = invoice.AmountPaid
	result.Period = domain.Coverage{From: time.Unix(line.Period.Start, 0).UTC(), Until: time.Unix(line.Period.End, 0).UTC()}
	return result
}
