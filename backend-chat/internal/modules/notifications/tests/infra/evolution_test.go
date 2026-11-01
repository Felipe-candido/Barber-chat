package infra_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
)

const testInstance = "bc_0123456789abcdef0123456789abcdef"
const providerKey = "test-provider-key-never-log-this"
const callbackSecret = "test-callback-secret-at-least-32-characters"

func evolutionClient(t *testing.T, url string) *evolution.Client {
	t.Helper()
	c, err := evolution.NewClient(url, providerKey, "http://callback.invalid/api/v1/webhooks/evolution", callbackSecret, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEvolutionConnectCreatesOnceAndConfiguresAuthenticatedWebhook(t *testing.T) {
	created, creates, connects := false, 0, 0
	qr := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != providerKey {
			t.Error("missing provider authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/instance/connectionState/" + testInstance:
			if !created {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error":"Not Found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"instance":{"instanceName":"` + testInstance + `","state":"connecting"}}`))
		case "/instance/create":
			creates++
			created = true
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["instanceName"] != testInstance || body["integration"] != "WHATSAPP-BAILEYS" || body["syncFullHistory"] != false {
				t.Error("wrong instance creation contract")
			}
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"instance":{"instanceName":"` + testInstance + `"}}`))
		case "/webhook/set/" + testInstance:
			var body struct {
				Webhook struct {
					URL     string            `json:"url"`
					Headers map[string]string `json:"headers"`
					Events  []string          `json:"events"`
				} `json:"webhook"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Webhook.URL != "http://callback.invalid/api/v1/webhooks/evolution" || body.Webhook.Headers[evolution.WebhookHeader] != evolution.WebhookToken(callbackSecret, testInstance) || len(body.Webhook.Events) != 1 || body.Webhook.Events[0] != "CONNECTION_UPDATE" {
				t.Error("unsafe or incorrect webhook configuration")
			}
			_, _ = w.Write([]byte(`{"enabled":true}`))
		case "/instance/connect/" + testInstance:
			connects++
			_ = json.NewEncoder(w).Encode(map[string]any{"base64": qr, "count": 1})
		default:
			t.Error("unexpected provider endpoint")
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	c := evolutionClient(t, s.URL)
	for range 2 {
		out, err := c.Connect(context.Background(), testInstance)
		if err != nil || out.Observation.State != domain.ConnectionQRRequired || out.QRBase64 != qr {
			t.Fatal("challenge not returned", err)
		}
	}
	if creates != 1 || connects != 2 {
		t.Fatal("reconnect created another instance", creates, connects)
	}
}

func TestEvolutionRejectsActivationCredentialsAndMalformedResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"license", 503, `{"code":"LICENSE_REQUIRED","message":"private-provider-details"}`, app.ErrProviderActivationRequired},
		{"credentials", 401, `{"error":"private-provider-details"}`, app.ErrProviderCredentials},
		{"missing", 404, `{}`, app.ErrProviderInstanceMissing},
		{"unknown state", 200, `{"instance":{"instanceName":"` + testInstance + `","state":"surprise"}}`, app.ErrProviderProtocol},
		{"wrong instance", 200, `{"instance":{"instanceName":"other","state":"open"}}`, app.ErrProviderProtocol},
		{"error in 200", 200, `{"error":true,"message":"private-provider-details"}`, app.ErrProviderProtocol},
		{"HTML", 200, `<html>private-provider-details</html>`, app.ErrProviderProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, err := evolutionClient(t, s.URL).Observe(context.Background(), testInstance)
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "private-provider-details") {
				t.Fatal("incorrect or leaking provider error", err)
			}
		})
	}
}

func TestEvolutionDoesNotForwardCredentialsThroughRedirect(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer s.Close()
	if _, err := evolutionClient(t, s.URL).Observe(context.Background(), testInstance); err == nil || forwarded {
		t.Fatal("provider redirect leaked authentication")
	}
}

func TestEvolutionLogoutVerifiesTheNewState(t *testing.T) {
	connected := true
	logouts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			connected = false
			logouts++
			_, _ = w.Write([]byte(`{"status":"SUCCESS","error":false}`))
			return
		}
		state := "close"
		if connected {
			state = "open"
		}
		_, _ = w.Write([]byte(`{"instance":{"instanceName":"` + testInstance + `","state":"` + state + `"}}`))
	}))
	defer s.Close()
	c := evolutionClient(t, s.URL)
	for range 2 {
		out, err := c.Disconnect(context.Background(), testInstance)
		if err != nil || out.State != domain.ConnectionDisconnected {
			t.Fatal(err)
		}
	}
	if logouts != 1 {
		t.Fatal("already disconnected instance logged out repeatedly")
	}
	if _, err := c.Observe(context.Background(), "../../other-instance"); !errors.Is(err, app.ErrProviderProtocol) {
		t.Fatal("untrusted instance path accepted")
	}
}
