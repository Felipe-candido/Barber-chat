package infra_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	notificationhttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	"github.com/google/uuid"
)

type receiptRecorder struct{ received []app.ProviderReceipt }

func (s *receiptRecorder) RecordReceipt(_ context.Context, r app.ProviderReceipt) error {
	s.received = append(s.received, r)
	return nil
}

func TestEvolutionReceiptWebhook(t *testing.T) {
	id := uuid.New()
	hex := strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))
	for _, tc := range []struct {
		name, event           string
		data                  any
		auth                  bool
		wantStatus, wantCalls int
		stage                 int16
	}{
		{"delivered", "messages.update", map[string]any{"keyId": hex, "fromMe": true, "status": "DELIVERY_ACK"}, true, 204, 1, 2},
		{"read", "messages.update", map[string]any{"keyId": hex, "fromMe": true, "status": "READ"}, true, 204, 1, 3},
		{"send evidence", "send.message", map[string]any{"key": map[string]any{"id": hex, "fromMe": true}}, true, 204, 1, 1},
		{"unauthenticated", "messages.update", map[string]any{"keyId": hex, "fromMe": true, "status": "READ"}, false, 401, 0, 0},
		{"incoming", "messages.update", map[string]any{"keyId": hex, "fromMe": false, "status": "READ"}, true, 204, 0, 0},
		{"uncorrelated", "messages.update", map[string]any{"keyId": "provider-generated-id", "fromMe": true, "status": "READ"}, true, 204, 0, 0},
		{"error is not success", "messages.update", map[string]any{"keyId": hex, "fromMe": true, "status": "ERROR"}, true, 204, 0, 0},
		{"unknown future status", "messages.update", map[string]any{"keyId": hex, "fromMe": true, "status": "FUTURE"}, true, 204, 0, 0},
		{"wrong shape", "messages.update", []any{map[string]any{"keyId": hex, "fromMe": true, "status": "READ"}}, true, 204, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &receiptRecorder{}
			h := notificationhttp.NewConnectionHandler(nil, callbackSecret, time.Second).WithReceipts(store)
			body, _ := json.Marshal(map[string]any{"event": tc.event, "instance": testInstance, "data": tc.data, "apikey": "untrusted-private-key"})
			r := httptest.NewRequest("POST", "/api/v1/webhooks/evolution", bytes.NewReader(body))
			if tc.auth {
				r.Header.Set(evolution.WebhookHeader, evolution.WebhookToken(callbackSecret, testInstance))
			}
			w := httptest.NewRecorder()
			h.Webhook(w, r)
			if w.Code != tc.wantStatus || len(store.received) != tc.wantCalls {
				t.Fatal("unexpected webhook decision", w.Code, len(store.received))
			}
			if tc.wantCalls == 1 && (store.received[0].AttemptID != id || store.received[0].Stage != tc.stage) {
				t.Fatal("incorrect receipt correlation")
			}
			if strings.Contains(w.Body.String(), "private-key") || strings.Contains(w.Body.String(), hex) {
				t.Fatal("receipt response leaked input")
			}
		})
	}
}
