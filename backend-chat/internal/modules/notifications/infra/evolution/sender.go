package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

var _ app.NotificationSender = (*Client)(nil)

const maxSendResponseBytes = 64 * 1024

// Send makes exactly one HTTP attempt, with no retry or redirect. Preselected
// messageId correlates this attempt; it is NOT a provider idempotency guarantee.
// Do not reuse request(): connection-command errors and send ambiguity differ.
func (c *Client) Send(ctx context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
	if c == nil || c.http == nil {
		return notSent(domain.ReasonProviderUnavailable), nil
	}
	if m.AttemptID == uuid.Nil || !ValidInstanceName(m.InstanceName) {
		return notSent(domain.ReasonProviderRejected), nil
	}
	message, err := app.RenderBookingMessage(m.TemplateVersion, m.Kind, m.Snapshot)
	if err != nil {
		return notSent(domain.ReasonProviderRejected), nil
	}
	if ctx.Err() != nil {
		return notSent(domain.ReasonProviderUnavailable), nil
	}
	id := strings.ToUpper(strings.ReplaceAll(m.AttemptID.String(), "-", ""))
	data, err := json.Marshal(struct {
		Number      string `json:"number"`
		Text        string `json:"text"`
		MessageID   string `json:"messageId"`
		LinkPreview bool   `json:"linkPreview"`
		Delay       int    `json:"delay"`
	}{strings.TrimPrefix(m.Snapshot.RecipientPhone, "+"), message.Text, id, false, 0})
	if err != nil {
		return notSent(domain.ReasonProviderRejected), nil
	}
	defer clear(data)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/message/sendText/"+m.InstanceName, bytes.NewReader(data))
	if err != nil {
		return notSent(domain.ReasonProviderUnavailable), nil
	}
	r.Header.Set("apikey", c.key)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(r)
	if err != nil {
		return uncertain(domain.ReasonTransportUnknown), nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSendResponseBytes+1))
	defer clear(raw)
	if err != nil {
		return uncertain(domain.ReasonTransportUnknown), nil
	}
	if len(raw) > maxSendResponseBytes {
		return uncertain(domain.ReasonInvalidResponse), nil
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (media != "application/json" && !strings.HasSuffix(media, "+json")) {
		return uncertain(domain.ReasonInvalidResponse), nil
	}
	var response sendResponse
	if json.Unmarshal(raw, &response) != nil {
		return uncertain(domain.ReasonInvalidResponse), nil
	}
	if rejectedBeforeSend(resp.StatusCode, response) {
		return notSent(domain.ReasonProviderUnavailable), nil
	}
	// Even HTTP 400 can be thrown AFTER sendMessage by this pinned provider.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return uncertain(domain.ReasonTransportUnknown), nil
	}
	if response.Code != "" || !noProviderError(response.Error) || response.Key == nil || !response.Key.FromMe || response.Key.ID != id || !acceptedStatus(response.Status) || !matchesRecipient(response.Key, m.Snapshot.RecipientPhone) {
		return uncertain(domain.ReasonInvalidResponse), nil
	}
	return domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: id}, nil
}

type sendKey struct {
	ID           string `json:"id"`
	FromMe       bool   `json:"fromMe"`
	RemoteJID    string `json:"remoteJid"`
	RemoteJIDAlt string `json:"remoteJidAlt"`
}
type sendResponse struct {
	Key    *sendKey        `json:"key"`
	Status json.RawMessage `json:"status"`
	Error  json.RawMessage `json:"error"`
	Code   string          `json:"code"`
}

func rejectedBeforeSend(status int, r sendResponse) bool {
	if r.Key != nil {
		return false
	}
	var providerError string
	_ = json.Unmarshal(r.Error, &providerError)
	if status == 503 && r.Code == "LICENSE_REQUIRED" && providerError == "service not activated" {
		return true
	}
	var reportedStatus int
	if json.Unmarshal(r.Status, &reportedStatus) != nil || reportedStatus != status {
		return false
	}
	return (status == 401 && providerError == "Unauthorized") || (status == 403 && providerError == "Forbidden") || (status == 404 && providerError == "Not Found")
}
func noProviderError(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "false" || string(raw) == "null"
}
func acceptedStatus(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	switch s {
	case "PENDING", "SERVER_ACK", "DELIVERY_ACK", "READ", "PLAYED":
		return true
	}
	return false
}
func matchesRecipient(k *sendKey, phone string) bool {
	expected := strings.TrimPrefix(phone, "+") + "@s.whatsapp.net"
	if k.RemoteJID == expected {
		return k.RemoteJIDAlt == "" || k.RemoteJIDAlt == expected || validLID(k.RemoteJIDAlt)
	}
	return validLID(k.RemoteJID) && k.RemoteJIDAlt == expected
}
func validLID(s string) bool {
	suffix := "@lid"
	number := strings.TrimSuffix(s, suffix)
	return strings.HasSuffix(s, suffix) && len(number) > 0 && len(number) <= 20 && strings.Trim(number, "0123456789") == ""
}
func notSent(reason domain.DeliveryReason) domain.SendOutcome {
	return domain.SendOutcome{Disposition: domain.SendNotSent, Reason: reason}
}
func uncertain(reason domain.DeliveryReason) domain.SendOutcome {
	return domain.SendOutcome{Disposition: domain.SendUnknown, Reason: reason}
}
