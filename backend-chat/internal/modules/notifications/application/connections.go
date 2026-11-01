package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

var (
	ErrConnectionAccessDenied     = errors.New("WhatsApp owner access denied")
	ErrConnectionBusy             = errors.New("WhatsApp connection operation in progress")
	ErrProviderNotConfigured      = errors.New("WhatsApp provider not configured")
	ErrProviderUnavailable        = errors.New("WhatsApp provider unavailable")
	ErrProviderActivationRequired = errors.New("Evolution activation required")
	ErrProviderCredentials        = errors.New("Evolution credentials rejected")
	ErrProviderProtocol           = errors.New("invalid Evolution response")
	ErrProviderInstanceMissing    = errors.New("Evolution instance not found")
)

type ConnectionScope struct{ ActorID, ShopID uuid.UUID }
type ConnectionObservation struct {
	State     domain.ConnectionState
	ErrorCode string
}
type ConnectionChallenge struct {
	Observation  ConnectionObservation
	QRBase64     string
	DiscardAfter time.Time
}

func (ConnectionChallenge) String() string         { return "[redacted WhatsApp challenge]" }
func (c ConnectionChallenge) GoString() string     { return c.String() }
func (c ConnectionChallenge) LogValue() slog.Value { return slog.StringValue(c.String()) }

type ConnectionView struct {
	State                                       domain.ConnectionState
	DesiredConnected, Stale, ProviderConfigured bool
	VerifiedAt, LastConnectedAt                 time.Time
	ErrorCode                                   string
}
type ConnectResult struct {
	Connection ConnectionView
	Challenge  ConnectionChallenge
}

type ConnectionOwnerAccess interface {
	RequireConnectionOwner(context.Context, ConnectionScope) error
}
type ConnectionRepository interface {
	ReadOwner(context.Context, ConnectionScope) (domain.Connection, error)
	ClaimOwner(context.Context, ConnectionScope, bool) (domain.Connection, error)
	ClaimDue(context.Context) (domain.Connection, bool, error)
	Complete(context.Context, domain.Connection, ConnectionObservation) (bool, error)
	MarkDirty(context.Context, string) error
}
type ConnectionProvider interface {
	Connect(context.Context, string) (ConnectionChallenge, error)
	Observe(context.Context, string) (ConnectionObservation, error)
	Disconnect(context.Context, string) (ConnectionObservation, error)
}

type ManageConnections struct {
	repository ConnectionRepository
	provider   ConnectionProvider
}

func NewManageConnections(r ConnectionRepository, p ConnectionProvider) *ManageConnections {
	return &ManageConnections{r, p}
}

func (uc *ManageConnections) Status(ctx context.Context, scope ConnectionScope) (ConnectionView, error) {
	c, err := uc.repository.ReadOwner(ctx, scope)
	if err != nil {
		return ConnectionView{}, err
	}
	return uc.view(c), nil
}
func (uc *ManageConnections) RequestRecheck(ctx context.Context, name string) error {
	return uc.repository.MarkDirty(ctx, name)
}
func (uc *ManageConnections) view(c domain.Connection) ConnectionView {
	v := ConnectionView{State: c.State, DesiredConnected: c.DesiredConnected, Stale: !c.FreshAt(c.ObservedAt) || c.ErrorCode != "", ProviderConfigured: uc.provider != nil, VerifiedAt: c.VerifiedAt, LastConnectedAt: c.LastConnectedAt, ErrorCode: c.ErrorCode}
	if c.InstanceName == "" {
		v.State = domain.ConnectionNotConfigured
		return v
	}
	if uc.provider == nil {
		v.State = domain.ConnectionUnknown
		v.Stale = true
		v.ErrorCode = "provider_not_configured"
	} else if v.Stale && v.State == domain.ConnectionConnected {
		v.State = domain.ConnectionUnknown
	}
	return v
}
func (uc *ManageConnections) Connect(ctx context.Context, scope ConnectionScope) (ConnectResult, error) {
	return uc.change(ctx, scope, true)
}
func (uc *ManageConnections) Disconnect(ctx context.Context, scope ConnectionScope) (ConnectionView, error) {
	r, err := uc.change(ctx, scope, false)
	return r.Connection, err
}
func (uc *ManageConnections) change(ctx context.Context, scope ConnectionScope, desired bool) (ConnectResult, error) {
	// Authorize before disclosing provider configuration or claiming an action.
	if _, err := uc.repository.ReadOwner(ctx, scope); err != nil {
		return ConnectResult{}, err
	}
	if uc.provider == nil {
		return ConnectResult{}, ErrProviderNotConfigured
	}
	c, err := uc.repository.ClaimOwner(ctx, scope, desired)
	if err != nil {
		return ConnectResult{}, err
	}
	var challenge ConnectionChallenge
	if desired {
		challenge, err = uc.provider.Connect(ctx, c.InstanceName)
	} else {
		challenge.Observation, err = uc.provider.Disconnect(ctx, c.InstanceName)
	}
	observation := challenge.Observation
	if err != nil {
		observation = ConnectionObservation{State: domain.ConnectionUnknown, ErrorCode: connectionErrorCode(err)}
	}
	// Record an uncertain result even if the browser/request has disconnected.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	applied, saveErr := uc.repository.Complete(cleanup, c, observation)
	if saveErr != nil {
		return ConnectResult{}, ErrProviderUnavailable
	}
	if !applied {
		return ConnectResult{}, ErrConnectionBusy
	}
	if err != nil {
		return ConnectResult{}, err
	}
	// Permission may have been revoked during external I/O. Never return QR then.
	current, err := uc.repository.ReadOwner(ctx, scope)
	if err != nil {
		return ConnectResult{}, err
	}
	if current.Generation != c.Generation {
		return ConnectResult{}, ErrConnectionBusy
	}
	return ConnectResult{Connection: uc.view(current), Challenge: challenge}, nil
}

// ReconcileOne also repairs an external action that finished after its lease:
// the latest persisted desired state wins; no provider call holds a SQL lock.
func (uc *ManageConnections) ReconcileOne(ctx context.Context) (int, error) {
	if uc.provider == nil {
		return 0, nil
	}
	c, found, err := uc.repository.ClaimDue(ctx)
	if err != nil || !found {
		return 0, err
	}
	observation, err := uc.provider.Observe(ctx, c.InstanceName)
	if errors.Is(err, ErrProviderInstanceMissing) {
		observation = ConnectionObservation{State: domain.ConnectionDisconnected}
		err = nil
	}
	if err == nil && !c.DesiredConnected && observation.State != domain.ConnectionDisconnected {
		observation, err = uc.provider.Disconnect(ctx, c.InstanceName)
	}
	if err != nil {
		observation = ConnectionObservation{State: domain.ConnectionUnknown, ErrorCode: connectionErrorCode(err)}
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, saveErr := uc.repository.Complete(cleanup, c, observation)
	if saveErr != nil {
		return 0, saveErr
	}
	return 1, err
}

func connectionErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrProviderActivationRequired):
		return "activation_required"
	case errors.Is(err, ErrProviderCredentials):
		return "provider_credentials"
	case errors.Is(err, ErrProviderProtocol):
		return "provider_protocol"
	default:
		return "provider_unavailable"
	}
}
