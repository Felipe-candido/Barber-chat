package supabase_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const projectURL = "https://project.supabase.co"

var userID = uuid.MustParse("603a4d04-6a86-4688-9bd0-b0c7924cba65")

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func validClaims(origin string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss": origin + "/auth/v1", "aud": "authenticated", "sub": userID.String(),
		"exp": now.Add(time.Hour).Unix(), "iat": now.Add(-time.Minute).Unix(),
		"nbf": now.Add(-time.Minute).Unix(), "role": "authenticated", "is_anonymous": false,
	}
}

func signToken(t *testing.T, key *ecdsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	return signed(t, token, key)
}

func signed(t *testing.T, token *jwt.Token, key any) string {
	t.Helper()
	value, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func publicJWK(key *ecdsa.PrivateKey, kid string) jose.JSONWebKey {
	return jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: "ES256", Use: "sig"}
}

func jsonBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func jwksBytes(t *testing.T, keys ...jose.JSONWebKey) []byte {
	t.Helper()
	return jsonBytes(t, jose.JSONWebKeySet{Keys: keys})
}

func newTLSVerifier(t *testing.T, body []byte, status int) (*supabase.Verifier, string, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" || r.Method != http.MethodGet {
			t.Errorf("unexpected JWKS request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials must not be sent to the public JWKS endpoint")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("expected JSON Accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	verifier, err := supabase.NewVerifier(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return verifier, server.URL, &requests
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(body []byte, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func newStubVerifier(t *testing.T, transport roundTripFunc) *supabase.Verifier {
	t.Helper()
	verifier, err := supabase.NewVerifier(projectURL, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}
