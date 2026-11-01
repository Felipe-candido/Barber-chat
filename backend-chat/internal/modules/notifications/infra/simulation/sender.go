// Package simulation provides a deterministic sender for local delivery tests.
// It never performs I/O or retains booking/contact content. Production bootstrap
// must explicitly inject a real sender; this adapter is not an implicit fallback.
package simulation

import (
	"context"
	"sync/atomic"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
)

type Sender struct {
	disposition domain.SendDisposition
	calls       atomic.Int64
}

func NewSender(disposition domain.SendDisposition) *Sender { return &Sender{disposition: disposition} }
func (s *Sender) Calls() int64                             { return s.calls.Load() }
func (s *Sender) Send(ctx context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
	s.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return domain.SendOutcome{}, err
	}
	switch s.disposition {
	case domain.SendAccepted:
		return domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: "simulation_" + m.AttemptID.String()}, nil
	case domain.SendNotSent:
		return domain.SendOutcome{Disposition: domain.SendNotSent, Reason: domain.ReasonProviderRejected}, nil
	default:
		return domain.SendOutcome{Disposition: domain.SendUnknown, Reason: domain.ReasonTransportUnknown}, nil
	}
}
