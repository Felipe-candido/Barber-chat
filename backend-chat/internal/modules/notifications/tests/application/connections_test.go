package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type connectionFake struct {
	row                    domain.Connection
	deny, stale, completed bool
	callErr                error
	reads, calls           int
	result                 app.ConnectionObservation
	changeAccess           bool
}

func (f *connectionFake) ReadOwner(ctx context.Context, s app.ConnectionScope) (domain.Connection, error) {
	f.reads++
	if f.deny {
		return domain.Connection{}, app.ErrConnectionAccessDenied
	}
	return f.row, nil
}
func (f *connectionFake) ClaimOwner(context.Context, app.ConnectionScope, bool) (domain.Connection, error) {
	return f.row, nil
}
func (f *connectionFake) ClaimDue(context.Context) (domain.Connection, bool, error) {
	return f.row, true, nil
}
func (f *connectionFake) Complete(ctx context.Context, c domain.Connection, o app.ConnectionObservation) (bool, error) {
	f.completed = ctx.Err() == nil
	f.result = o
	f.row.State = o.State
	return !f.stale, nil
}
func (f *connectionFake) MarkDirty(context.Context, string) error { return nil }
func (f *connectionFake) Connect(context.Context, string) (app.ConnectionChallenge, error) {
	f.calls++
	if f.changeAccess {
		f.deny = true
	}
	return app.ConnectionChallenge{Observation: app.ConnectionObservation{State: domain.ConnectionQRRequired}, QRBase64: "private-qr"}, f.callErr
}
func (f *connectionFake) Observe(context.Context, string) (app.ConnectionObservation, error) {
	return app.ConnectionObservation{State: domain.ConnectionConnected}, f.callErr
}
func (f *connectionFake) Disconnect(context.Context, string) (app.ConnectionObservation, error) {
	f.calls++
	return app.ConnectionObservation{State: domain.ConnectionDisconnected}, f.callErr
}
func connectionTestFake() *connectionFake {
	now := time.Now().UTC()
	return &connectionFake{row: domain.Connection{ShopID: uuid.New(), InstanceName: "opaque", Generation: 2, OperationToken: uuid.New(), State: domain.ConnectionConnected, VerifiedAt: now, ObservedAt: now}}
}

func TestConnectionRequiresOwnerAgainBeforeReturningQR(t *testing.T) {
	f := connectionTestFake()
	f.changeAccess = true
	out, err := app.NewManageConnections(f, f).Connect(context.Background(), app.ConnectionScope{})
	if !errors.Is(err, app.ErrConnectionAccessDenied) || out.Challenge.QRBase64 != "" || !f.completed {
		t.Fatal("revoked owner received a QR or outcome was not recorded")
	}
}
func TestConnectionLateOrUncertainResultsDoNotReturnQR(t *testing.T) {
	for _, late := range []bool{true, false} {
		f := connectionTestFake()
		f.stale = late
		if !late {
			f.callErr = app.ErrProviderActivationRequired
		}
		out, err := app.NewManageConnections(f, f).Connect(context.Background(), app.ConnectionScope{})
		if err == nil || out.Challenge.QRBase64 != "" || !f.completed {
			t.Fatal("uncertain result exposed a challenge")
		}
		if !late && (f.result.State != domain.ConnectionUnknown || f.result.ErrorCode != "activation_required") {
			t.Fatal("activation outcome not recorded")
		}
	}
}
func TestStaleConnectedProjectionIsUnknown(t *testing.T) {
	f := connectionTestFake()
	f.row.ObservedAt = f.row.VerifiedAt.Add(2 * time.Minute)
	out, err := app.NewManageConnections(f, f).Status(context.Background(), app.ConnectionScope{})
	if err != nil || out.State != domain.ConnectionUnknown || !out.Stale {
		t.Fatal("stale projection claims a live connection")
	}
	if text := fmt.Sprintf("%v %#v", app.ConnectionChallenge{QRBase64: "private-qr"}, app.ConnectionChallenge{QRBase64: "private-qr"}); strings.Contains(text, "private-qr") {
		t.Fatal("challenge logging leaked a QR")
	}
}
func TestReconcilerConvergesToDisconnectedIntent(t *testing.T) {
	f := connectionTestFake()
	f.row.DesiredConnected = false
	if _, err := app.NewManageConnections(f, f).ReconcileOne(context.Background()); err != nil || f.calls != 1 || f.result.State != domain.ConnectionDisconnected {
		t.Fatal("late external connection was not disconnected", err)
	}
}
