package application

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
)

const BookingMessageTemplateVersion = 1
const MaxBookingMessageBytes = 12 * 1024

var ErrInvalidBookingMessage = errors.New("invalid booking message")

// BookingMessage is private rendered content, not a public HTTP response DTO.
type BookingMessage struct {
	Version int
	Text    string
}

func (BookingMessage) String() string         { return "[redacted booking message]" }
func (m BookingMessage) GoString() string     { return m.String() }
func (m BookingMessage) LogValue() slog.Value { return slog.StringValue(m.String()) }

// RenderBookingMessage is deterministic: use the protected snapshot, its IANA
// timezone and integer cents. Never use the server's timezone, current catalog
// prices or relative words such as "tomorrow" for a configurable reminder.
func RenderBookingMessage(version int, kind string, s BookingSnapshot) (BookingMessage, error) {
	if version != BookingMessageTemplateVersion || !s.Valid() {
		return BookingMessage{}, ErrInvalidBookingMessage
	}
	var title, greeting string
	switch kind {
	case "booking_confirmation":
		title, greeting = "Agendamento confirmado", "Seu horário está confirmado."
	case "booking_reminder":
		title, greeting = "Lembrete do seu agendamento", "Passando para lembrar do seu horário."
	default:
		return BookingMessage{}, ErrInvalidBookingMessage
	}
	location, err := time.LoadLocation(s.ShopTimezone)
	if err != nil {
		return BookingMessage{}, ErrInvalidBookingMessage
	}
	values := []string{s.CustomerName, s.ShopName, s.ShopAddress, s.ServiceName, s.ProfessionalName}
	for i, v := range values {
		values[i] = messageLine(v)
		if values[i] == "" {
			return BookingMessage{}, ErrInvalidBookingMessage
		}
	}
	// Preserve the capability exactly; never "clean up" a malformed private code.
	if messageLine(s.AppointmentCode) != s.AppointmentCode {
		return BookingMessage{}, ErrInvalidBookingMessage
	}
	local := s.StartsAt.In(location)
	currency := s.Currency
	if currency == "BRL" {
		currency = "R$"
	}
	// Customers read the shop's own civil date and time; offsets and IANA zone
	// names are omitted on purpose. The asterisks are WhatsApp bold on the title only.
	weekdays := [...]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	text := fmt.Sprintf("*%s*\n\nOlá, %s! %s\n\nBarbearia: %s\nServiço: %s\nProfissional: %s\nData: %s (%s)\nHorário: %s\nValor: %s %d,%02d\nEndereço: %s\n\nCódigo do agendamento: %s\n\nGuarde este código para consultar ou cancelar na página de agendamento da barbearia. Não compartilhe o código com outras pessoas.",
		title, values[0], greeting, values[1], values[3], values[4], local.Format("02/01/2006"), weekdays[local.Weekday()], local.Format("15:04"), currency, s.PriceCents/100, s.PriceCents%100, values[2], s.AppointmentCode)
	if len(text) > MaxBookingMessageBytes {
		return BookingMessage{}, ErrInvalidBookingMessage
	}
	return BookingMessage{Version: version, Text: text}, nil
}

func messageLine(s string) string {
	// Unicode line separators/bidi controls must not insert misleading fields.
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
