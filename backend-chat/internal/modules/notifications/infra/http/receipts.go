package http

import (
	"encoding/json"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
)

// Only authenticated provider events reach this parser. No body, destination,
// timestamp or apikey is retained; random message IDs must match our attempts.
func receiptHint(event, instance string, data json.RawMessage) (app.ProviderReceipt, bool) {
	var id, status string
	var fromMe bool
	switch event {
	case "send.message":
		var d struct {
			Key struct {
				ID     string `json:"id"`
				FromMe bool   `json:"fromMe"`
			} `json:"key"`
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &d) != nil {
			return app.ProviderReceipt{}, false
		}
		id, status, fromMe = d.Key.ID, d.Status, d.Key.FromMe
	case "messages.update":
		var d struct {
			ID     string `json:"keyId"`
			Status string `json:"status"`
			FromMe bool   `json:"fromMe"`
		}
		if json.Unmarshal(data, &d) != nil {
			return app.ProviderReceipt{}, false
		}
		id, status, fromMe = d.ID, d.Status, d.FromMe
	default:
		return app.ProviderReceipt{}, false
	}
	if !fromMe || len(id) != 32 {
		return app.ProviderReceipt{}, false
	}
	attempt, err := uuid.Parse(id)
	if err != nil || attempt == uuid.Nil {
		return app.ProviderReceipt{}, false
	}
	var stage int16
	switch status {
	case "PENDING", "SERVER_ACK":
		stage = 1
	case "DELIVERY_ACK":
		stage = 2
	case "READ", "PLAYED":
		stage = 3
	case "":
		if event == "send.message" {
			stage = 1
		}
	}
	if stage == 0 {
		return app.ProviderReceipt{}, false
	}
	return app.ProviderReceipt{InstanceName: instance, AttemptID: attempt, Stage: stage}, true
}
