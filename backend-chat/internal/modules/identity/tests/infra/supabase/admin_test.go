package supabase_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/google/uuid"
)

func TestAdminProviderInviteAndVerifiedLookup(t *testing.T) {
	id := uuid.New()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "server-test-key" || r.Header.Get("Authorization") != "Bearer server-test-key" {
			t.Error("missing server credentials")
		}
		if r.URL.Path == "/auth/v1/invite" {
			if r.Method != "POST" || r.URL.Query().Get("redirect_to") != "http://127.0.0.1:3000/definir-senha" {
				t.Error("wrong invitation destination")
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["email"] != "barber@example.com" || len(body) != 1 {
				t.Error("unexpected invitation input")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "email": "barber@example.com"})
		} else {
			if r.Method != "GET" || r.URL.Path != "/auth/v1/admin/users/"+id.String() {
				t.Error("wrong lookup")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": id, "email": "barber@example.com"}})
		}
	}))
	defer server.Close()
	client, err := supabase.NewAdminClient(server.URL, "server-test-key", "http://127.0.0.1:3000", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() (app.ProviderUser, error){func() (app.ProviderUser, error) { return client.Invite(context.Background(), "barber@example.com") }, func() (app.ProviderUser, error) { return client.FindUser(context.Background(), id) }} {
		user, err := call()
		if err != nil || user.ID != id || user.Email != "barber@example.com" {
			t.Fatal(user, err)
		}
	}
}

func TestAdminProviderDoesNotLeakErrorsOrFollowRedirects(t *testing.T) {
	for _, status := range []int{302, 422, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetCalls atomic.Int32
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1); w.WriteHeader(200) }))
			defer target.Close()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"secret-test-key email@example.com"}`))
			}))
			defer server.Close()
			client, err := supabase.NewAdminClient(server.URL, "secret-test-key", "https://frontend.example", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Invite(context.Background(), "barber@example.com")
			want := app.ErrAccountProviderUnavailable
			if status == 422 {
				want = app.ErrAccountProviderRejected
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "secret-test-key") || strings.Contains(err.Error(), "email@example.com") || targetCalls.Load() != 0 {
				t.Fatal("unsafe provider error", err, targetCalls.Load())
			}
		})
	}
	if _, err := supabase.NewAdminClient("http://example.com", "key", "https://frontend.example", nil); err == nil {
		t.Fatal("insecure provider accepted")
	}
	if _, err := supabase.NewAdminClient("https://auth.example", "key", "http://untrusted.example", nil); err == nil {
		t.Fatal("insecure redirect accepted")
	}
}
