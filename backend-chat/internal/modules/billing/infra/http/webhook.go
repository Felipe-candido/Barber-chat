package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type WebhookHandler struct {
	verifier application.WebhookVerifier
	receive  *application.ReceiveWebhook
	timeout  time.Duration
	logger   *slog.Logger
}

func NewWebhookHandler(verifier application.WebhookVerifier, receive *application.ReceiveWebhook, timeout time.Duration, logger *slog.Logger) *WebhookHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookHandler{verifier: verifier, receive: receive, timeout: timeout, logger: logger}
}

func (h *WebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	if h.verifier == nil || h.receive == nil {
		response.Error(w, 503, "webhook_not_configured", "Stripe test webhook is not configured")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "body_too_large", "webhook exceeds 512 KiB")
		} else {
			response.Error(w, 400, "invalid_webhook", "webhook could not be verified")
		}
		return
	}
	event, err := h.verifier.VerifyWebhook(body, r.Header.Get("Stripe-Signature"))
	if err != nil {
		reason := "signature_or_payload"
		if errors.Is(err, application.ErrUnsupportedWebhookVersion) {
			reason = "unsupported_api_version"
		}
		h.logger.WarnContext(r.Context(), "Stripe webhook rejected", "request_id", chimiddleware.GetReqID(r.Context()), "reason", reason)
		response.Error(w, 400, "invalid_webhook", "webhook could not be verified")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.receive.Execute(ctx, event); err != nil {
		if errors.Is(err, application.ErrInvalidWebhook) {
			response.Error(w, 400, "invalid_webhook", "webhook could not be verified")
			return
		}
		h.logger.WarnContext(ctx, "Stripe webhook reconciliation pending", "request_id", chimiddleware.GetReqID(ctx))
		response.Error(w, 503, "webhook_pending", "event was not confirmed; redelivery is required")
		return
	}
	response.JSON(w, 200, struct {
		Received bool `json:"received"`
	}{true})
}
