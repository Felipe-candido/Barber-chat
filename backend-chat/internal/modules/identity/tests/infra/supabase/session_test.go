package supabase_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/google/uuid"
)

func TestRegistrationSessionReader(t *testing.T) {
	id := uuid.New()
	for _, confirmed := range []bool{false, true} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" || r.URL.Path != "/auth/v1/user" || r.Header.Get("Authorization") != "Bearer session-token" || r.Header.Get("apikey") != "public-key" {
				t.Error("wrong session lookup")
			}
			var date any
			if confirmed {
				date = "2026-10-29T12:00:00Z"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "email": "owner@example.com", "email_confirmed_at": date, "is_anonymous": false, "user_metadata": map[string]any{"role": "owner", "email_verified": true}})
		}))
		client, err := supabase.NewSessionClient(server.URL, "public-key", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		got, err := client.ReadRegistrationAccount(context.Background(), "session-token")
		server.Close()
		if err != nil || got.UserID != id || got.EmailConfirmed != confirmed || got.Anonymous {
			t.Fatalf("%+v %v", got, err)
		}
	}
}

func TestRegistrationSessionFailuresAreSanitized(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		want   error
	}{
		{401, `{"message":"private-token"}`, app.ErrInvalidAccessToken},
		{429, `{"message":"private-token"}`, app.ErrRegistrationUnavailable},
		{503, `{"message":"private-token"}`, app.ErrRegistrationUnavailable},
		{200, `{"id":"broken"}`, app.ErrRegistrationUnavailable},
		{200, strings.Repeat("x", 65537), app.ErrRegistrationUnavailable},
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.status)
			_, _ = w.Write([]byte(test.body))
		}))
		client, err := supabase.NewSessionClient(server.URL, "public-key", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.ReadRegistrationAccount(context.Background(), "session-token")
		server.Close()
		if !errors.Is(err, test.want) || strings.Contains(err.Error(), "private-token") {
			t.Fatal(err)
		}
	}
	targetCalls := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	client, err := supabase.NewSessionClient(server.URL, "public-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadRegistrationAccount(context.Background(), "session-token"); !errors.Is(err, app.ErrRegistrationUnavailable) || targetCalls != 0 {
		t.Fatal("redirect was followed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ReadRegistrationAccount(ctx, "session-token"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := supabase.NewSessionClient(server.URL, "", nil); !errors.Is(err, app.ErrRegistrationNotConfigured) {
		t.Fatal(err)
	}
}
