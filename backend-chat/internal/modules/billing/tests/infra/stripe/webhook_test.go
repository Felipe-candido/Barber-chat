package stripe_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
)

func webhookSignature(body []byte, at time.Time) string {
	seconds := fmt.Sprint(at.Unix())
	mac := hmac.New(sha256.New, []byte("whsec_test_secret"))
	_, _ = mac.Write([]byte(seconds + "."))
	_, _ = mac.Write(body)
	return "t=" + seconds + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestOperatorReplayRetrievesOnlyAnAuthenticatedTestEvent(t *testing.T) {
	for _, tc := range []struct {
		name, version, id string
		live              bool
		allowed           bool
	}{
		{"current", stripe.APIVersion, "evt_example", false, true},
		{"CLI snapshot", "2026-09-30.endive", "evt_example", false, true},
		{"unknown", "2026-11-30.endive", "evt_example", false, false},
		{"live", stripe.APIVersion, "evt_example", true, false},
		{"wrong event identity", stripe.APIVersion, "evt_other", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client, _ := stripe.NewClient("sk_test_example", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				key, _, authenticated := r.BasicAuth()
				if r.Method != "GET" || r.URL.Path != "/v1/events/evt_example" || r.Header.Get("Stripe-Version") != stripe.APIVersion || !authenticated || key != "sk_test_example" {
					t.Fatal("unsafe event retrieval")
				}
				return jsonResponse(map[string]any{"id": tc.id, "object": "event", "api_version": tc.version, "type": "invoice.paid", "created": time.Now().Add(-time.Hour).Unix(), "livemode": tc.live,
					"data": map[string]any{"object": map[string]any{"id": "in_example", "customer_email": "private@example.test", "amount_paid": 999999999}}}), nil
			})})
			event, err := client.ReadWebhookEvent(context.Background(), "evt_example")
			if (err == nil) != tc.allowed || calls != 1 {
				t.Fatal(event, err, calls)
			}
			if tc.allowed && (event.ResourceID != "in_example" || event.Kind != "invoice" || event.APIVersion != tc.version) {
				t.Fatal("unsafe event projection", event)
			}
			encoded, _ := json.Marshal(event)
			if strings.Contains(string(encoded), "private@example.test") || strings.Contains(string(encoded), "amount_paid") {
				t.Fatal("snapshot financial/PII fields retained")
			}
		})
	}
}
func TestWebhookAuthenticatesRawBodyAndEnvelope(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	verifier, err := stripe.NewWebhookVerifier("whsec_test_secret", application.ClockFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, key string
		value     any
		allowed   bool
	}{
		{"valid", "type", "invoice.paid", true}, {"old event fresh signature", "created", now.Add(-48 * time.Hour).Unix(), true},
		{"live forbidden", "livemode", true, false}, {"missing mode", "livemode", nil, false},
		{"API version", "api_version", "2024-10-28.acacia", false}, {"Connect forbidden", "account", "acct_other", false},
		{"CLI account default version", "api_version", "2026-09-30.endive", true},
		{"unknown monthly version", "api_version", "2026-11-30.endive", false},
		{"empty version", "api_version", "", false},
		{"unknown event ignored", "type", "customer.created", true}, {"missing reference", "data", map[string]any{"object": map[string]any{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := map[string]any{"id": "evt_example", "object": "event", "type": "invoice.paid", "created": now.Unix(), "livemode": false, "api_version": stripe.APIVersion,
				"data": map[string]any{"object": map[string]any{"id": "in_example", "customer_email": "not-to-be-retained@example.test"}}}
			value[tc.key] = tc.value
			body, _ := json.Marshal(value)
			got, err := verifier.VerifyWebhook(body, webhookSignature(body, now))
			if (err == nil) != tc.allowed {
				t.Fatal(got, err)
			}
			if tc.allowed && got.APIVersion != value["api_version"] {
				t.Fatal("event version not preserved")
			}
			if tc.allowed && got.Type == "customer.created" && (got.Kind != "ignored" || got.ResourceID != "") {
				t.Fatal(got)
			}
			projection, _ := json.Marshal(got)
			if strings.Contains(string(projection), "not-to-be-retained") {
				t.Fatal("PII leaked into verified projection")
			}
		})
	}
	body := []byte(`{"id":"evt_test","object":"event","type":"customer.created","created":1,"livemode":false,"api_version":"` + stripe.APIVersion + `"}`)
	valid := webhookSignature(body, now)
	for _, header := range []string{"", webhookSignature(body, now.Add(-6*time.Minute)), webhookSignature(body, now.Add(6*time.Minute)), valid + ",t=1", "t=1,v1=bad"} {
		if _, err := verifier.VerifyWebhook(body, header); err == nil {
			t.Fatal("invalid/stale signature accepted")
		}
	}
	if _, err := verifier.VerifyWebhook(append(body, ' '), valid); err == nil {
		t.Fatal("modified raw body accepted")
	}
	if _, err := verifier.VerifyWebhook(body, valid+",v1="+strings.Repeat("0", 64)); err != nil {
		t.Fatal("signature rotation failed", err)
	}
}
