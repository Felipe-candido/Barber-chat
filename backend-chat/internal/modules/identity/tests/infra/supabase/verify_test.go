package supabase_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestNewVerifier(t *testing.T) {
	for _, origin := range []string{projectURL, "  " + projectURL + "/  ", "https://auth.example.com", "https://localhost:8443"} {
		t.Run(origin, func(t *testing.T) {
			if _, err := supabase.NewVerifier(origin, nil); err != nil {
				t.Fatalf("valid project URL rejected: %v", err)
			}
		})
	}
	for _, origin := range []string{
		"", "not a URL", "http://project.supabase.co", "https:///project", "https://user:password@project.supabase.co",
		projectURL + "/auth/v1", projectURL + "?x=1", projectURL + "?", projectURL + "#fragment", projectURL + "#",
		projectURL + ":", projectURL + ":0", projectURL + ":65536", projectURL + ":invalid",
	} {
		t.Run(origin, func(t *testing.T) {
			if _, err := supabase.NewVerifier(origin, nil); err == nil {
				t.Fatal("invalid project URL accepted")
			}
		})
	}
}

func TestVerifyAccessTokenClaims(t *testing.T) {
	key := newKey(t)
	verifier, origin, _ := newTLSVerifier(t, jwksBytes(t, publicJWK(key, "current")), http.StatusOK)
	cases := []struct {
		name   string
		change func(jwt.MapClaims)
		valid  bool
	}{
		{"valid session", func(jwt.MapClaims) {}, true},
		{"audience list", func(c jwt.MapClaims) { c["aud"] = []string{"authenticated", "another"} }, true},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, false},
		{"missing expiration", func(c jwt.MapClaims) { delete(c, "exp") }, false},
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = projectURL + "/auth/v1" }, false},
		{"missing issuer", func(c jwt.MapClaims) { delete(c, "iss") }, false},
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = "anon" }, false},
		{"missing audience", func(c jwt.MapClaims) { delete(c, "aud") }, false},
		{"future nbf", func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }, false},
		{"future iat", func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Hour).Unix() }, false},
		{"anonymous", func(c jwt.MapClaims) { c["is_anonymous"] = true }, false},
		{"invalid anonymous type", func(c jwt.MapClaims) { c["is_anonymous"] = "false" }, false},
		{"service role", func(c jwt.MapClaims) { c["role"] = "service_role" }, false},
		{"missing role", func(c jwt.MapClaims) { delete(c, "role") }, false},
		{"invalid subject", func(c jwt.MapClaims) { c["sub"] = "not-a-uuid" }, false},
		{"zero subject", func(c jwt.MapClaims) { c["sub"] = uuid.Nil.String() }, false},
		{"missing subject", func(c jwt.MapClaims) { delete(c, "sub") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := validClaims(origin)
			tc.change(claims)
			id, err := verifier.VerifyAccessToken(t.Context(), signToken(t, key, "current", claims))
			if tc.valid {
				if err != nil || id != userID {
					t.Fatalf("got (%v, %v), want valid user", id, err)
				}
			} else if !errors.Is(err, application.ErrInvalidAccessToken) || id != uuid.Nil {
				t.Fatalf("got (%v, %v), want invalid token", id, err)
			}
		})
	}
}

func TestVerifyAccessTokenRejectsMalformedTokensBeforeHTTP(t *testing.T) {
	key := newKey(t)
	verifier, origin, requests := newTLSVerifier(t, jwksBytes(t, publicJWK(key, "current")), http.StatusOK)
	for _, kid := range []any{nil, "", "   ", 123, strings.Repeat("k", 257)} {
		token := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims(origin))
		if kid != nil {
			token.Header["kid"] = kid
		}
		if _, err := verifier.VerifyAccessToken(t.Context(), signed(t, token, key)); !errors.Is(err, application.ErrInvalidAccessToken) {
			t.Fatalf("invalid kid accepted: %v", err)
		}
	}
	hmac := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims(origin))
	hmac.Header["kid"] = "current"
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims(origin))
	unsigned.Header["kid"] = "current"
	critical := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims(origin))
	critical.Header["kid"] = "current"
	critical.Header["crit"] = []string{"unsupported"}
	for _, value := range []string{"", " ", "malformed", strings.Repeat("a", 32*1024+1),
		signed(t, hmac, []byte("not-a-real-secret")), signed(t, unsigned, jwt.UnsafeAllowNoneSignatureType), signed(t, critical, key)} {
		if id, err := verifier.VerifyAccessToken(t.Context(), value); !errors.Is(err, application.ErrInvalidAccessToken) || id != uuid.Nil {
			t.Fatalf("malformed token returned (%v, %v)", id, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("malformed tokens must not download keys")
	}
}

func TestVerifyAccessTokenSignatureAndKeySource(t *testing.T) {
	key := newKey(t)
	verifier, origin, requests := newTLSVerifier(t, jwksBytes(t, publicJWK(key, "current")), http.StatusOK)
	for _, value := range []string{
		signToken(t, newKey(t), "current", validClaims(origin)),
		signToken(t, key, "unknown", validClaims(origin)),
	} {
		if _, err := verifier.VerifyAccessToken(t.Context(), value); !errors.Is(err, application.ErrInvalidAccessToken) {
			t.Fatalf("untrusted signature/key accepted: %v", err)
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims(origin))
	token.Header["kid"] = "current"
	token.Header["jku"] = "https://untrusted.invalid/keys"
	token.Header["x5u"] = "https://untrusted.invalid/certificate"
	if id, err := verifier.VerifyAccessToken(t.Context(), signed(t, token, key)); err != nil || id != userID {
		t.Fatalf("configured key source was not used: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatal("expected a single request to the configured JWKS endpoint")
	}
}

func TestVerifyAccessTokenConcurrentCache(t *testing.T) {
	key := newKey(t)
	verifier, origin, requests := newTLSVerifier(t, jwksBytes(t, publicJWK(key, "current")), http.StatusOK)
	value := signToken(t, key, "current", validClaims(origin))
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id, err := verifier.VerifyAccessToken(t.Context(), value); err != nil || id != userID {
				t.Errorf("concurrent verification failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("got %d requests, want 1", requests.Load())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := verifier.VerifyAccessToken(ctx, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not preserved: %v", err)
	}
}
