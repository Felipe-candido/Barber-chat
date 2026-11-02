//go:build integration

package integration

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
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
)

func TestNotificationEvolutionSenderPersistence(t *testing.T) {
	for _, mode := range []string{"accepted", "400_after_possible_send", "lost_response"} {
		t.Run(mode, func(t *testing.T) {
			f := newDeliveryFixture(t)
			var calls atomic.Int64
			var requestedID string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Number    string `json:"number"`
					Text      string `json:"text"`
					MessageID string `json:"messageId"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid send body")
				}
				requestedID = body.MessageID
				if body.Number != "5511912345678" || !strings.Contains(body.Text, "Código do agendamento:") || !strings.Contains(body.Text, "Valor: R$ 50,00") {
					t.Error("protected booking not rendered")
				}
				if mode == "lost_response" {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "400_after_possible_send" {
					w.WriteHeader(400)
					_, _ = io.WriteString(w, `{"status":400,"error":"Bad Request","message":"private provider error after sending"}`)
					return
				}
				w.WriteHeader(201)
				_ = json.NewEncoder(w).Encode(map[string]any{"key": map[string]any{"id": body.MessageID, "fromMe": true, "remoteJid": body.Number + "@s.whatsapp.net"}, "status": "PENDING"})
			}))
			defer s.Close()
			sender, err := evolution.NewClient(s.URL, "integration-provider-key-private", "http://callback.invalid/api/v1/webhooks/evolution", "integration-callback-secret-at-least-32-characters", nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.run(sender)
			want := "unknown"
			if mode == "accepted" {
				want = "accepted"
			}
			if err != nil || result.State != want || f.state(t) != want {
				t.Fatal("incorrect durable sender result", result, err)
			}
			result, err = f.run(sender)
			if err != nil || result.State != want || calls.Load() != 1 || f.count(t, "notification_attempts") != 1 {
				t.Fatal("uncertain/accepted send was repeated", result, err)
			}
			var token, outcome, storedID, reason string
			var templateVersion int
			if err := f.pool.QueryRow(f.ctx, `SELECT attempt_id::text,outcome,COALESCE(provider_message_id,''),reason,template_version
   FROM notification_attempts WHERE shop_id=$1 AND job_id=$2`, f.shop, f.key.JobID).Scan(&token, &outcome, &storedID, &reason, &templateVersion); err != nil {
				t.Fatal(err)
			}
			if templateVersion != app.BookingMessageTemplateVersion || strings.ToUpper(strings.ReplaceAll(token, "-", "")) != requestedID {
				t.Fatal("attempt correlation/template lost")
			}
			if mode == "accepted" {
				if outcome != "accepted" || storedID != requestedID || reason != "" {
					t.Fatal("acceptance evidence not saved")
				}
			} else if outcome != "unknown" || storedID != "" || reason != string(domain.ReasonTransportUnknown) {
				t.Fatal("ambiguous response asserted a definite result")
			}
			// Later failures cannot regress a known acceptance.
			if mode == "accepted" {
				err = f.tx.WithinDeliveryTx(f.ctx, func(ctx context.Context, tx app.DeliveryTx) error {
					record, _, err := tx.Store.Lock(ctx, f.key)
					if err != nil {
						return err
					}
					return tx.Store.Complete(ctx, app.DeliveryClaim{Key: f.key, Token: record.Token}, domain.SendOutcome{Disposition: domain.SendUnknown, Reason: domain.ReasonTransportUnknown})
				})
				if err != nil || f.state(t) != "accepted" {
					t.Fatal("acceptance regressed", err)
				}
			}
		})
	}
}
