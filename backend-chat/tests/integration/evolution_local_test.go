//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	connectionhttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	"github.com/google/uuid"
)

// A recheck sink proves the real Docker -> Windows webhook path. It is not a
// substitute for the separate PostgreSQL authorization/lease integration tests.
type localEvolutionEvents struct{ events chan struct{} }

func (f *localEvolutionEvents) MarkDirty(context.Context, string) error {
	select {
	case f.events <- struct{}{}:
	default:
	}
	return nil
}
func (*localEvolutionEvents) ReadOwner(context.Context, app.ConnectionScope) (domain.Connection, error) {
	return domain.Connection{}, app.ErrConnectionAccessDenied
}
func (*localEvolutionEvents) ClaimOwner(context.Context, app.ConnectionScope, bool) (domain.Connection, error) {
	return domain.Connection{}, app.ErrConnectionAccessDenied
}
func (*localEvolutionEvents) ClaimDue(context.Context) (domain.Connection, bool, error) {
	return domain.Connection{}, false, nil
}
func (*localEvolutionEvents) Complete(context.Context, domain.Connection, app.ConnectionObservation) (bool, error) {
	return false, nil
}

func TestEvolutionLocalQRAndWebhook(t *testing.T) {
	base, key := requiredEnv(t, "EVOLUTION_LOCAL_TEST_URL"), requiredEnv(t, "EVOLUTION_LOCAL_TEST_KEY")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() != "8081" {
		t.Fatal("this test requires the dedicated local Evolution on 127.0.0.1:8081")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	var secretBytes [32]byte
	if _, err := rand.Read(secretBytes[:]); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(secretBytes[:])
	events := &localEvolutionEvents{events: make(chan struct{}, 8)}
	h := connectionhttp.NewConnectionHandler(app.NewManageConnections(events, nil), secret, 5*time.Second)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/webhooks/evolution", h.Webhook)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = server.Shutdown(cleanup)
	})
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	client, err := evolution.NewClient(base, key, "http://host.docker.internal:"+port+"/api/v1/webhooks/evolution", secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	name := "bc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		// Only remove the random instance created by this test, never user sessions.
		for _, path := range []string{"/instance/logout/", "/instance/delete/"} {
			r, _ := http.NewRequest("DELETE", base+path+name, nil)
			r.Header.Set("apikey", key)
			resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(r)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	})
	gotQR := false
	for range 5 {
		challenge, err := client.Connect(ctx, name)
		if err != nil {
			t.Fatal("local provider did not return a challenge", err)
		}
		if challenge.QRBase64 != "" {
			gotQR = true
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("provider challenge deadline")
		case <-time.After(2 * time.Second):
		}
	}
	if !gotQR {
		t.Fatal("local provider did not generate a QR")
	}
	select {
	case <-events.events:
	case <-time.After(8 * time.Second):
		t.Fatal("authenticated Evolution webhook did not reach the host")
	}
}
