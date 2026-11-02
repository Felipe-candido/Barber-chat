package infra_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

func outboundFixture() app.OutboundNotification {
	_, _, snapshot := payloadFixture()
	return app.OutboundNotification{AttemptID: uuid.MustParse("f69d67c0-f36f-4e52-93a7-782006f854b9"), TemplateVersion: 1, InstanceName: testInstance, Kind: "booking_confirmation", Snapshot: snapshot}
}
func successResponse(m app.OutboundNotification) map[string]any {
	return map[string]any{"key": map[string]any{"id": strings.ToUpper(strings.ReplaceAll(m.AttemptID.String(), "-", "")), "fromMe": true, "remoteJid": strings.TrimPrefix(m.Snapshot.RecipientPhone, "+") + "@s.whatsapp.net"}, "status": "PENDING", "message": map[string]any{"conversation": "private-provider-content"}}
}
func TestEvolutionSendContractAndCorrelation(t *testing.T) {
	for _, lid := range []bool{false, true} {
		t.Run(map[bool]string{false: "phone", true: "lid_with_phone_mapping"}[lid], func(t *testing.T) {
			m := outboundFixture()
			var calls atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/message/sendText/"+testInstance || r.URL.RawQuery != "" || r.Header.Get("apikey") != providerKey || r.Header.Get("Content-Type") != "application/json" {
					t.Error("incorrect or unsafe send request")
				}
				var body map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 5 {
					t.Error("unexpected send fields")
				}
				var number, text, id string
				_ = json.Unmarshal(body["number"], &number)
				_ = json.Unmarshal(body["text"], &text)
				_ = json.Unmarshal(body["messageId"], &id)
				if number != strings.TrimPrefix(m.Snapshot.RecipientPhone, "+") || id != "F69D67C0F36F4E5293A7782006F854B9" || !strings.Contains(text, m.Snapshot.AppointmentCode) || !strings.Contains(text, m.Snapshot.ShopAddress) || string(body["linkPreview"]) != "false" || string(body["delay"]) != "0" {
					t.Error("snapshot or transport options incorrect")
				}
				if strings.Contains(text, m.Snapshot.RecipientPhone) || r.Header.Get("Idempotency-Key") != "" {
					t.Error("unnecessary contact or unsupported idempotency promise")
				}
				response := successResponse(m)
				if lid {
					key := response["key"].(map[string]any)
					key["remoteJidAlt"] = key["remoteJid"]
					key["remoteJid"] = "123456789012345@lid"
					response["status"] = "READ"
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(201)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer s.Close()
			out, err := evolutionClient(t, s.URL).Send(context.Background(), m)
			if err != nil || out.Disposition != domain.SendAccepted || out.ProviderMessageID != "F69D67C0F36F4E5293A7782006F854B9" || calls.Load() != 1 {
				t.Fatal("valid provider evidence not accepted", out, err)
			}
		})
	}
}
func TestEvolutionSendRejectsAmbiguousAndMismatchedEvidence(t *testing.T) {
	m := outboundFixture()
	type failure struct {
		name   string
		status int
		change func(map[string]any)
		raw    string
		want   domain.SendDisposition
	}
	cases := []failure{
		{name: "unauthorized_before_handler", status: 401, raw: `{"status":401,"error":"Unauthorized","response":{"message":"private-provider-content"}}`, want: domain.SendNotSent},
		{name: "forbidden_before_handler", status: 403, raw: `{"status":403,"error":"Forbidden"}`, want: domain.SendNotSent},
		{name: "missing_instance_guard", status: 404, raw: `{"status":404,"error":"Not Found"}`, want: domain.SendNotSent},
		{name: "activation_guard", status: 503, raw: `{"error":"service not activated","code":"LICENSE_REQUIRED","message":"private-provider-content"}`, want: domain.SendNotSent},
		{name: "400_after_possible_send", status: 400, raw: `{"status":400,"error":"Bad Request","message":"private-provider-content"}`},
		{name: "rate_limit", status: 429, raw: `{"error":"private-provider-content"}`},
		{name: "server_failure", status: 500, raw: `{"error":"private-provider-content"}`},
		{name: "generic_unavailable", status: 503, raw: `{"error":"private-provider-content"}`},
		{name: "empty_2xx", status: 201, raw: `{}`},
		{name: "non_object", status: 201, raw: `[]`},
		{name: "trailing_json", status: 201, raw: `{} {}`},
		{name: "truncated", status: 201, raw: `{"key":`},
		{name: "html", status: 201, raw: `<html>private-provider-content</html>`},
		{name: "wrong_message", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["id"] = "unrelated_message" }},
		{name: "wrong_recipient", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["remoteJid"] = "5511988887777@s.whatsapp.net" }},
		{name: "group", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["remoteJid"] = "123456789@g.us" }},
		{name: "lid_without_mapping", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["remoteJid"] = "123456789@lid" }},
		{name: "contradictory_phone", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["remoteJidAlt"] = "5511988887777@s.whatsapp.net" }},
		{name: "not_from_this_sender", status: 201, change: func(r map[string]any) { r["key"].(map[string]any)["fromMe"] = false }},
		{name: "error_status", status: 201, change: func(r map[string]any) { r["status"] = "ERROR" }},
		{name: "numeric_status", status: 201, change: func(r map[string]any) { r["status"] = 1 }},
		{name: "contradictory_error", status: 201, change: func(r map[string]any) { r["error"] = "private-provider-content" }},
		{name: "contradictory_auth", status: 401, change: func(r map[string]any) { r["status"] = 401; r["error"] = "Unauthorized" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.raw != "" {
					_, _ = io.WriteString(w, tc.raw)
					return
				}
				response := successResponse(m)
				if tc.change != nil {
					tc.change(response)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer s.Close()
			got, err := evolutionClient(t, s.URL).Send(context.Background(), m)
			want := tc.want
			if want == "" {
				want = domain.SendUnknown
			}
			if err != nil || got.Disposition != want || got.ProviderMessageID != "" || calls.Load() != 1 {
				t.Fatal("unsafe provider outcome", got, err)
			}
			if strings.Contains(string(got.Reason), "private-provider-content") {
				t.Fatal("provider body leaked")
			}
		})
	}
}
func TestEvolutionSendNeverRetriesLostResponseOrRedirect(t *testing.T) {
	t.Run("provider_accepted_then_connection_lost", func(t *testing.T) {
		var calls atomic.Int64
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			_, _ = io.Copy(io.Discard, r.Body)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
		}))
		defer s.Close()
		got, err := evolutionClient(t, s.URL).Send(context.Background(), outboundFixture())
		if err != nil || got.Disposition != domain.SendUnknown || calls.Load() != 1 {
			t.Fatal("lost response repeated or asserted failure", got, err)
		}
	})
	t.Run("redirect_cannot_receive_credential_or_content", func(t *testing.T) {
		var forwarded atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Add(1) }))
		defer target.Close()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer s.Close()
		got, err := evolutionClient(t, s.URL).Send(context.Background(), outboundFixture())
		if err != nil || got.Disposition != domain.SendUnknown || forwarded.Load() != 0 {
			t.Fatal("redirect forwarded private send", got, err)
		}
	})
}
func TestEvolutionSendChecksBeforeIOAndBoundsResponse(t *testing.T) {
	for _, change := range []string{"invalid_instance", "invalid_phone", "invalid_kind", "invalid_version", "cancelled"} {
		t.Run(change, func(t *testing.T) {
			var calls atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer s.Close()
			m := outboundFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch change {
			case "invalid_instance":
				m.InstanceName = "../../other"
			case "invalid_phone":
				m.Snapshot.RecipientPhone = "123@g.us"
			case "invalid_kind":
				m.Kind = "marketing"
			case "invalid_version":
				m.TemplateVersion = 0
			case "cancelled":
				cancel()
			}
			got, err := evolutionClient(t, s.URL).Send(ctx, m)
			if err != nil || got.Disposition != domain.SendNotSent || calls.Load() != 0 {
				t.Fatal("invalid request reached provider", got, err)
			}
		})
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"large":"`+strings.Repeat("x", 65536)+`"}`)
	}))
	defer s.Close()
	got, err := evolutionClient(t, s.URL).Send(context.Background(), outboundFixture())
	if err != nil || got.Disposition != domain.SendUnknown || got.Reason != domain.ReasonInvalidResponse {
		t.Fatal("oversized response accepted", got, err)
	}
}
