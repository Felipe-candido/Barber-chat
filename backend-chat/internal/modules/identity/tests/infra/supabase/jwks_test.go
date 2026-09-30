package supabase_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/go-jose/go-jose/v4"
)

func TestJWKSFailures(t *testing.T) {
	key := newKey(t)
	public := publicJWK(key, "current")
	private := public
	private.Key = key
	tooMany := make([]jose.JSONWebKey, 65)
	for i := range tooMany {
		tooMany[i] = publicJWK(key, fmt.Sprint(i))
	}
	cases := []struct {
		name   string
		body   []byte
		status int
	}{
		{"server failure", []byte("sensitive response body"), http.StatusInternalServerError},
		{"not found", nil, http.StatusNotFound},
		{"invalid JSON", []byte("not json"), http.StatusOK},
		{"trailing JSON", []byte(`{"keys":[]} {}`), http.StatusOK},
		{"missing keys", []byte(`{}`), http.StatusOK},
		{"null keys", []byte(`{"keys":null}`), http.StatusOK},
		{"duplicate kid", jwksBytes(t, public, public), http.StatusOK},
		{"private key", jwksBytes(t, private), http.StatusOK},
		{"invalid curve point", []byte(`{"keys":[{"kid":"current","kty":"EC","crv":"P-256","x":"AA","y":"AA"}]}`), http.StatusOK},
		{"too many keys", jwksBytes(t, tooMany...), http.StatusOK},
		{"oversized response", []byte(strings.Repeat(" ", 128*1024+1)), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier, origin, requests := newTLSVerifier(t, tc.body, tc.status)
			value := signToken(t, key, "current", validClaims(origin))
			for range 2 {
				_, err := verifier.VerifyAccessToken(t.Context(), value)
				if !errors.Is(err, supabase.ErrJWKSUnavailable) || errors.Is(err, application.ErrInvalidAccessToken) {
					t.Fatalf("expected operational failure, got %v", err)
				}
				if err.Error() != supabase.ErrJWKSUnavailable.Error() {
					t.Fatal("error must not disclose response data")
				}
			}
			if requests.Load() != 1 {
				t.Fatal("failed JWKS requests must be rate limited")
			}
		})
	}
}

func TestJWKSMissingOrIncompatibleKey(t *testing.T) {
	key := newKey(t)
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"different kid", func(k map[string]any) { k["kid"] = "another" }},
		{"missing kid", func(k map[string]any) { delete(k, "kid") }},
		{"wrong algorithm", func(k map[string]any) { k["alg"] = "HS256" }},
		{"wrong curve", func(k map[string]any) { k["crv"] = "P-384" }},
		{"encryption key", func(k map[string]any) { k["use"] = "enc" }},
		{"sign only", func(k map[string]any) { k["key_ops"] = []string{"sign"} }},
		{"no operations", func(k map[string]any) { k["key_ops"] = []string{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal(jsonBytes(t, publicJWK(key, "current")), &raw); err != nil {
				t.Fatal(err)
			}
			tc.change(raw)
			body := jsonBytes(t, map[string]any{"keys": []any{raw}})
			verifier, origin, _ := newTLSVerifier(t, body, http.StatusOK)
			if _, err := verifier.VerifyAccessToken(t.Context(), signToken(t, key, "current", validClaims(origin))); !errors.Is(err, application.ErrInvalidAccessToken) {
				t.Fatalf("incompatible key accepted: %v", err)
			}
		})
	}
	t.Run("empty set", func(t *testing.T) {
		verifier, origin, _ := newTLSVerifier(t, []byte(`{"keys":[]}`), http.StatusOK)
		if _, err := verifier.VerifyAccessToken(t.Context(), signToken(t, key, "current", validClaims(origin))); !errors.Is(err, application.ErrInvalidAccessToken) {
			t.Fatalf("empty key set should reject token: %v", err)
		}
	})
}

func TestJWKSRotationAndExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		oldKey, newSigningKey := newKey(t), newKey(t)
		body := jwksBytes(t, publicJWK(oldKey, "old"))
		status, requests := http.StatusOK, 0
		verifier := newStubVerifier(t, func(*http.Request) (*http.Response, error) {
			requests++
			return response(body, status), nil
		})
		oldToken := signToken(t, oldKey, "old", validClaims(projectURL))
		newToken := signToken(t, newSigningKey, "new", validClaims(projectURL))
		if _, err := verifier.VerifyAccessToken(t.Context(), oldToken); err != nil {
			t.Fatal(err)
		}
		body = jwksBytes(t, publicJWK(newSigningKey, "new"))
		for range 10 {
			if _, err := verifier.VerifyAccessToken(t.Context(), newToken); !errors.Is(err, application.ErrInvalidAccessToken) {
				t.Fatalf("unknown kid during cooldown: %v", err)
			}
		}
		if requests != 1 {
			t.Fatal("unknown kids bypassed cooldown")
		}
		time.Sleep(31 * time.Second)
		if _, err := verifier.VerifyAccessToken(t.Context(), newToken); err != nil || requests != 2 {
			t.Fatalf("rotation was not discovered: %v (%d requests)", err, requests)
		}
		if _, err := verifier.VerifyAccessToken(t.Context(), oldToken); !errors.Is(err, application.ErrInvalidAccessToken) {
			t.Fatalf("removed key survived cache replacement: %v", err)
		}

		status = http.StatusServiceUnavailable
		if err := verifier.RefreshKeys(t.Context()); !errors.Is(err, supabase.ErrJWKSUnavailable) {
			t.Fatalf("refresh failure not returned: %v", err)
		}
		if _, err := verifier.VerifyAccessToken(t.Context(), newToken); err != nil {
			t.Fatalf("still-fresh cached key should remain usable: %v", err)
		}
		time.Sleep(5 * time.Minute)
		if _, err := verifier.VerifyAccessToken(t.Context(), newToken); !errors.Is(err, supabase.ErrJWKSUnavailable) {
			t.Fatalf("expired cache used during outage: %v", err)
		}
		status = http.StatusOK
		time.Sleep(31 * time.Second)
		if _, err := verifier.VerifyAccessToken(t.Context(), newToken); err != nil {
			t.Fatalf("failed to recover after outage: %v", err)
		}
	})
}

func TestRefreshKeysReplacesFreshCache(t *testing.T) {
	key := newKey(t)
	body := jwksBytes(t, publicJWK(key, "current"))
	verifier := newStubVerifier(t, func(*http.Request) (*http.Response, error) { return response(body, http.StatusOK), nil })
	value := signToken(t, key, "current", validClaims(projectURL))
	if _, err := verifier.VerifyAccessToken(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	body = []byte(`{"keys":[]}`)
	if err := verifier.RefreshKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.VerifyAccessToken(t.Context(), value); !errors.Is(err, application.ErrInvalidAccessToken) {
		t.Fatalf("explicit refresh did not remove revoked key: %v", err)
	}
}

func TestFreshCacheHitsDoNotWaitForRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := newKey(t)
		body := jwksBytes(t, publicJWK(key, "current"))
		requests := 0
		verifier := newStubVerifier(t, func(r *http.Request) (*http.Response, error) {
			requests++
			if requests == 2 {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return response(body, http.StatusOK), nil
		})
		value := signToken(t, key, "current", validClaims(projectURL))
		if _, err := verifier.VerifyAccessToken(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- verifier.RefreshKeys(ctx) }()
		synctest.Wait()
		start := time.Now()
		if _, err := verifier.VerifyAccessToken(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		if time.Since(start) != 0 {
			t.Fatal("a fresh cache hit waited for the in-flight refresh")
		}
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight refresh was not canceled: %v", err)
		}
	})
}

func TestJWKSClientSafety(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := url.Parse(projectURL)
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "must-not-leak"}})
	requests := 0
	client := &http.Client{
		Jar: jar,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			if r.Header.Get("Cookie") != "" {
				t.Error("client cookies leaked")
			}
			result := response(nil, http.StatusFound)
			result.Header.Set("Location", "https://untrusted.invalid/keys")
			return result, nil
		}),
	}
	verifier, err := supabase.NewVerifier(projectURL, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.RefreshKeys(t.Context()); !errors.Is(err, supabase.ErrJWKSUnavailable) || requests != 1 {
		t.Fatalf("redirect must not be followed: %v (%d requests)", err, requests)
	}
	if client.Timeout != 0 || client.CheckRedirect != nil || client.Jar != jar {
		t.Fatal("constructor modified the caller's client")
	}
}

func TestJWKSTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		verifier := newStubVerifier(t, func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		start := time.Now()
		err := verifier.RefreshKeys(t.Context())
		if !errors.Is(err, supabase.ErrJWKSUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout cause lost: %v", err)
		}
		if time.Since(start) != 5*time.Second {
			t.Fatalf("request timeout = %s", time.Since(start))
		}
	})
}

func TestJWKSCancellationDuringFetchAndWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := newKey(t)
		body := jwksBytes(t, publicJWK(key, "current"))
		requests := 0
		verifier := newStubVerifier(t, func(r *http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return response(body, http.StatusOK), nil
		})
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() { result <- verifier.RefreshKeys(ctx) }()
		synctest.Wait()
		waitingCtx, cancelWait := context.WithCancel(t.Context())
		waitingResult := make(chan error, 1)
		go func() { waitingResult <- verifier.RefreshKeys(waitingCtx) }()
		synctest.Wait()
		cancelWait()
		if err := <-waitingResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting caller did not cancel: %v", err)
		}
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("fetching caller did not cancel: %v", err)
		}
		value := signToken(t, key, "current", validClaims(projectURL))
		if _, err := verifier.VerifyAccessToken(t.Context(), value); err != nil || requests != 2 {
			t.Fatalf("canceled fetch poisoned cache: %v", err)
		}
	})
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (brokenBody) Close() error             { return nil }

func TestJWKSReadErrorPreserved(t *testing.T) {
	verifier := newStubVerifier(t, func(*http.Request) (*http.Response, error) {
		result := response(nil, http.StatusOK)
		result.Body = brokenBody{}
		return result, nil
	})
	err := verifier.RefreshKeys(t.Context())
	if !errors.Is(err, supabase.ErrJWKSUnavailable) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("body read failure not preserved: %v", err)
	}
}
