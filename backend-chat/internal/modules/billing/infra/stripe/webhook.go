package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
)

type WebhookVerifier struct {
	secret string
	clock  application.Clock
}

func NewWebhookVerifier(secret string, clock application.Clock) (*WebhookVerifier, error) {
	if !strings.HasPrefix(secret, "whsec_") || len(secret) <= 6 || len(secret) > 512 || strings.ContainsAny(secret, " \r\n\t") || clock == nil {
		return nil, application.ErrWebhookNotConfigured
	}
	return &WebhookVerifier{secret: secret, clock: clock}, nil
}

// Authentication uses the untouched body, not a re-marshaled JSON value.
// Delivery timestamp is fresh even when the event itself is an old redelivery.
func (v *WebhookVerifier) VerifyWebhook(body []byte, header string) (application.VerifiedWebhook, error) {
	invalid := func() (application.VerifiedWebhook, error) {
		return application.VerifiedWebhook{}, application.ErrInvalidWebhook
	}
	if len(body) == 0 || len(body) > 512*1024 || len(header) > 8192 {
		return invalid()
	}
	var timestamp string
	var signatures [][]byte
	parts := strings.Split(header, ",")
	if len(parts) > 32 {
		return invalid()
	}
	for _, part := range parts {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return invalid()
		}
		switch key {
		case "t":
			if timestamp != "" {
				return invalid()
			}
			timestamp = value
		case "v1":
			decoded, err := hex.DecodeString(value)
			if err != nil || len(decoded) != sha256.Size {
				return invalid()
			}
			signatures = append(signatures, decoded)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds <= 0 || len(signatures) == 0 {
		return invalid()
	}
	age := v.clock.Now().Sub(time.Unix(seconds, 0))
	if age > 5*time.Minute || age < -5*time.Minute {
		return invalid()
	}
	mac := hmac.New(sha256.New, []byte(v.secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	matched := false
	for _, signature := range signatures {
		matched = hmac.Equal(signature, expected) || matched
	}
	if !matched {
		return invalid()
	}
	return parseWebhookEnvelope(body)
}

// Envelope versions are independent from the API version used for resource
// reads. Both allowlisted envelopes provide the same fields consumed below;
// financial data is always re-read with APIVersion, never trusted from snapshots.
func parseWebhookEnvelope(body []byte) (application.VerifiedWebhook, error) {
	invalid := func() (application.VerifiedWebhook, error) {
		return application.VerifiedWebhook{}, application.ErrInvalidWebhook
	}
	var envelope struct {
		ID         string `json:"id"`
		Object     string `json:"object"`
		Type       string `json:"type"`
		Created    int64  `json:"created"`
		LiveMode   *bool  `json:"livemode"`
		APIVersion string `json:"api_version"`
		Account    string `json:"account"`
		Data       struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || !validID(envelope.ID, "evt_") || envelope.Object != "event" || len(envelope.Type) == 0 || len(envelope.Type) > 150 ||
		envelope.Created <= 0 || envelope.Created > 253402300799 || envelope.LiveMode == nil || *envelope.LiveMode || envelope.Account != "" {
		return invalid()
	}
	if envelope.APIVersion != APIVersion && envelope.APIVersion != "2026-09-30.endive" {
		return application.VerifiedWebhook{}, errors.Join(application.ErrInvalidWebhook, application.ErrUnsupportedWebhookVersion)
	}
	kind, prefix := webhookKind(envelope.Type)
	resourceID := ""
	if kind != "ignored" {
		var resource struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(envelope.Data.Object, &resource) != nil || !validID(resource.ID, prefix) {
			return invalid()
		}
		resourceID = resource.ID
	}
	return application.VerifiedWebhook{ID: envelope.ID, Type: envelope.Type, APIVersion: envelope.APIVersion, Kind: kind, ResourceID: resourceID, CreatedAt: time.Unix(envelope.Created, 0).UTC()}, nil
}

// ReadWebhookEvent is for an authenticated operator replay, not HTTP ingestion.
// It retrieves the event directly over authenticated HTTPS; it never accepts a
// caller-supplied payload or fabricates a Stripe-Signature. TEST only.
func (c *Client) ReadWebhookEvent(ctx context.Context, id string) (application.VerifiedWebhook, error) {
	if !validID(id, "evt_") {
		return application.VerifiedWebhook{}, application.ErrInvalidWebhook
	}
	var body json.RawMessage
	if err := c.request(ctx, "GET", "/events/"+id, "", nil, &body); err != nil {
		return application.VerifiedWebhook{}, err
	}
	event, err := parseWebhookEnvelope(body)
	if err != nil {
		return application.VerifiedWebhook{}, err
	}
	if event.ID != id {
		return application.VerifiedWebhook{}, application.ErrInvalidWebhook
	}
	return event, nil
}

func webhookKind(eventType string) (string, string) {
	switch eventType {
	case "checkout.session.completed", "checkout.session.expired":
		return "checkout", "cs_test_"
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.paused", "customer.subscription.resumed":
		return "subscription", "sub_"
	case "invoice.paid", "invoice.payment_failed", "invoice.payment_action_required", "invoice.voided", "invoice.marked_uncollectible", "invoice.updated":
		return "invoice", "in_"
	default:
		return "ignored", ""
	}
}
