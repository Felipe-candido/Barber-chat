package domain

import (
	"strings"
	"time"
)

type DeliveryState string

const (
	DeliveryReady       DeliveryState = "ready"
	DeliveryLeased      DeliveryState = "leased"
	DeliverySending     DeliveryState = "sending"
	DeliveryAccepted    DeliveryState = "accepted"
	DeliveryFailed      DeliveryState = "failed"
	DeliveryUnknown     DeliveryState = "unknown"
	DeliveryExpired     DeliveryState = "expired"
	DeliveryInvalidated DeliveryState = "invalidated"
)

// Only a reservation that has not started external I/O may be reclaimed.
func (s DeliveryState) ReclaimableAt(now, leaseUntil time.Time) bool {
	return s == DeliveryReady || (s == DeliveryLeased && !now.Before(leaseUntil))
}

func (s DeliveryState) StartedLeaseExpired(now, leaseUntil time.Time) bool {
	return s == DeliverySending && !now.Before(leaseUntil)
}

type SendDisposition string

const (
	SendAccepted SendDisposition = "accepted"
	SendNotSent  SendDisposition = "not_sent"
	SendUnknown  SendDisposition = "unknown"
)

// Only these bounded categories may reach persistence. Provider text is private.
type DeliveryReason string

const (
	ReasonIntentInvalidated   DeliveryReason = "intent_invalidated"
	ReasonIneligible          DeliveryReason = "ineligible"
	ReasonExpired             DeliveryReason = "expired"
	ReasonPayloadUnavailable  DeliveryReason = "payload_unavailable"
	ReasonProviderRejected    DeliveryReason = "provider_rejected"
	ReasonProviderUnavailable DeliveryReason = "provider_unavailable"
	ReasonTransportUnknown    DeliveryReason = "transport_unknown"
	ReasonInvalidResponse     DeliveryReason = "invalid_response"
)

type SendOutcome struct {
	Disposition       SendDisposition
	ProviderMessageID string
	Reason            DeliveryReason
}

// Normalize is deliberately conservative: malformed/contradictory responses
// cannot establish that no external effect happened. Never retry them blindly.
func (o SendOutcome) Normalize() SendOutcome {
	switch {
	case o.Disposition == SendAccepted && o.Reason == "" && validMessageID(o.ProviderMessageID):
		return o
	case o.Disposition == SendNotSent && o.ProviderMessageID == "" &&
		(o.Reason == ReasonProviderRejected || o.Reason == ReasonProviderUnavailable):
		return o
	case o.Disposition == SendUnknown && o.ProviderMessageID == "" &&
		(o.Reason == ReasonTransportUnknown || o.Reason == ReasonInvalidResponse):
		return o
	default:
		return SendOutcome{Disposition: SendUnknown, Reason: ReasonInvalidResponse}
	}
}

func (o SendOutcome) State() DeliveryState {
	switch o.Disposition {
	case SendAccepted:
		return DeliveryAccepted
	case SendNotSent:
		return DeliveryFailed
	default:
		return DeliveryUnknown
	}
}

func validMessageID(s string) bool {
	return len(s) > 0 && len(s) <= 200 && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789:_-") == ""
}
