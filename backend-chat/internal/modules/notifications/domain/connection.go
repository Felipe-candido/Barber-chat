package domain

import (
	"github.com/google/uuid"
	"time"
)

type ConnectionState string

const (
	ConnectionNotConfigured ConnectionState = "not_configured"
	ConnectionConnecting    ConnectionState = "connecting"
	ConnectionQRRequired    ConnectionState = "qr_required"
	ConnectionConnected     ConnectionState = "connected"
	ConnectionDisconnected  ConnectionState = "disconnected"
	ConnectionUnknown       ConnectionState = "unknown"
	ConnectionFreshness                     = 2 * time.Minute
)

func (s ConnectionState) Valid() bool {
	switch s {
	case ConnectionNotConfigured, ConnectionConnecting, ConnectionQRRequired, ConnectionConnected, ConnectionDisconnected, ConnectionUnknown:
		return true
	}
	return false
}

// Connection is metadata only. The provider owns its phone session; QR data and
// API keys must never be persisted here.
type Connection struct {
	ShopID                      uuid.UUID
	InstanceName                string
	Generation                  int64
	DesiredConnected            bool
	State                       ConnectionState
	VerifiedAt, LastConnectedAt time.Time
	ErrorCode                   string
	OperationToken              uuid.UUID
	ObservedAt                  time.Time
}

func (c Connection) FreshAt(now time.Time) bool {
	return !now.IsZero() && !c.VerifiedAt.IsZero() && !now.Before(c.VerifiedAt) && now.Sub(c.VerifiedAt) < ConnectionFreshness
}
