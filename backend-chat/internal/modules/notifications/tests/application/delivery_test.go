package application_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
)

func TestDeliveryTransportValuesRedactSensitiveContent(t *testing.T) {
	const phone = "+5511912345678"
	const code = "PRIVATE_BOOKING_CODE"
	message := app.OutboundNotification{InstanceName: "private_instance", Snapshot: app.BookingSnapshot{RecipientPhone: phone, AppointmentCode: code}}
	permission := app.DeliveryPermission{Phone: phone, Allowed: true}
	var b bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&b, nil))
	logger.Info("delivery", "message", message, "permission", permission)
	output := b.String() + fmt.Sprintf("%v %+v %#v %+v %#v", message, message, message, permission, permission)
	for _, secret := range []string{phone, code, "private_instance"} {
		if strings.Contains(output, secret) {
			t.Fatal("sensitive delivery value exposed by normal formatting")
		}
	}
}
