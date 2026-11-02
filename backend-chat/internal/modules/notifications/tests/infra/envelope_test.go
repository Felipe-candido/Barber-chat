package infra_test

import (
	"encoding/json"
	"strings"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notificationmq "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/rabbitmq"
	"github.com/google/uuid"
)

func TestBrokerEnvelopeContainsOnlyValidatedReferences(t *testing.T) {
	e := app.DispatchEnvelope{Version: 1, OutboxID: uuid.New(), ShopID: uuid.New(), JobID: uuid.New(), Sequence: 1}
	body, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := notificationmq.DecodeEnvelope(body)
	if err != nil || got != e {
		t.Fatal("valid envelope rejected", err)
	}
	for _, body := range []string{`null`, `{}`, `[]`, string(body) + ` {}`, strings.Replace(string(body), `"version":1`, `"version":2`, 1), strings.TrimSuffix(string(body), "}") + `,"phone":"forbidden"}`, strings.Repeat("x", 1025)} {
		if _, err := notificationmq.DecodeEnvelope([]byte(body)); err == nil {
			t.Fatal("untrusted broker document accepted")
		}
	}
}
