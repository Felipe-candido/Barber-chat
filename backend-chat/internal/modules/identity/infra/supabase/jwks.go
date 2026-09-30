package supabase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/go-jose/go-jose/v4"
)

const (
	keyCacheTTL     = 5 * time.Minute
	refreshCooldown = 30 * time.Second
	maxJWKSBytes    = 128 * 1024
	maxJWKSKeys     = 64
)

// ErrJWKSUnavailable is an operational failure, not proof of an invalid token.
var ErrJWKSUnavailable = errors.New("supabase signing keys unavailable")

// Keep transport details out of ordinary logs while preserving errors.Is/As.
type jwksError struct{ cause error }

func (e *jwksError) Error() string        { return ErrJWKSUnavailable.Error() }
func (e *jwksError) Unwrap() error        { return e.cause }
func (e *jwksError) Is(target error) bool { return target == ErrJWKSUnavailable }

type keyCache struct {
	// The gate serializes refreshes, but does not block fresh cache hits.
	gate chan struct{}
	mu   sync.Mutex
	// The mutex protects these fields and is never held during network I/O.
	publicKeys    map[string]*ecdsa.PublicKey
	expiresAt     time.Time
	nextRefresh   time.Time
	refreshFailed bool
}

func (v *Verifier) lockRefresh(ctx context.Context) error {
	select {
	case v.keys.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			v.unlockRefresh()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (v *Verifier) unlockRefresh() { <-v.keys.gate }

func (v *Verifier) fetchPublicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	key, shouldRefresh, err := v.lookupKey(kid)
	if !shouldRefresh {
		return key, err
	}
	if err := v.lockRefresh(ctx); err != nil {
		return nil, err
	}
	defer v.unlockRefresh()

	// Another caller may have populated the cache while this one waited.
	key, shouldRefresh, err = v.lookupKey(kid)
	if !shouldRefresh {
		return key, err
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	key, _, err = v.lookupKey(kid)
	return key, err
}

// lookupKey returns either a cached decision or a request to refresh the keys.
func (v *Verifier) lookupKey(kid string) (key *ecdsa.PublicKey, shouldRefresh bool, err error) {
	v.keys.mu.Lock()
	defer v.keys.mu.Unlock()
	now := time.Now()
	if key := v.keys.publicKeys[kid]; key != nil && now.Before(v.keys.expiresAt) {
		return key, false, nil
	}

	// Random kids must not cause one external request per invalid token. Failed
	// refreshes also back off. Expired keys are never used as a fallback.
	if now.Before(v.keys.nextRefresh) {
		if v.keys.refreshFailed || !now.Before(v.keys.expiresAt) {
			return nil, false, ErrJWKSUnavailable
		}
		return nil, false, application.ErrInvalidAccessToken
	}
	return nil, true, nil
}

// RefreshKeys explicitly replaces the local cache, for warm-up or rotation.
// It bypasses the automatic refresh cooldown; call only from trusted internal
// code, never once per incoming token. Upstream JWKS caches can still apply.
func (v *Verifier) RefreshKeys(ctx context.Context) error {
	if err := v.lockRefresh(ctx); err != nil {
		return err
	}
	defer v.unlockRefresh()
	return v.refreshKeys(ctx)
}

// refreshKeys requires the refresh gate to be held.
func (v *Verifier) refreshKeys(ctx context.Context) error {
	keys, err := v.downloadKeys(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		// One caller's cancellation must not delay other callers' refreshes.
		return ctxErr
	}
	v.keys.mu.Lock()
	defer v.keys.mu.Unlock()
	if err != nil {
		v.keys.nextRefresh = time.Now().Add(refreshCooldown)
		v.keys.refreshFailed = true
		return err
	}

	now := time.Now()
	// Replace instead of merging so keys removed upstream disappear locally.
	v.keys.publicKeys = keys
	v.keys.expiresAt = now.Add(keyCacheTTL)
	v.keys.nextRefresh = now.Add(refreshCooldown)
	v.keys.refreshFailed = false
	return nil
}

func (v *Verifier) downloadKeys(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, &jwksError{cause: err}
	}
	req.Header.Set("Accept", "application/json")
	response, err := v.client.Do(req)
	if err != nil {
		return nil, &jwksError{cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrJWKSUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSBytes+1))
	if err != nil {
		return nil, &jwksError{cause: err}
	}
	if len(body) > maxJWKSBytes {
		return nil, ErrJWKSUnavailable
	}
	return parseKeys(body)
}

func parseKeys(body []byte) (map[string]*ecdsa.PublicKey, error) {
	var document struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &document); err != nil || document.Keys == nil || len(document.Keys) > maxJWKSKeys {
		return nil, ErrJWKSUnavailable
	}

	keys := make(map[string]*ecdsa.PublicKey)
	seen := make(map[string]bool)
	for _, raw := range document.Keys {
		var metadata struct {
			KeyID     string   `json:"kid"`
			KeyType   string   `json:"kty"`
			Curve     string   `json:"crv"`
			Algorithm string   `json:"alg"`
			Use       string   `json:"use"`
			KeyOps    []string `json:"key_ops"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, ErrJWKSUnavailable
		}
		if metadata.KeyID == "" || len(metadata.KeyID) > maxKeyIDBytes {
			continue
		}
		if seen[metadata.KeyID] {
			return nil, ErrJWKSUnavailable
		}
		seen[metadata.KeyID] = true

		// Ignore unrelated keys while keeping ES256 as an explicit policy.
		if metadata.KeyType != "EC" || metadata.Curve != "P-256" ||
			(metadata.Algorithm != "" && metadata.Algorithm != "ES256") ||
			(metadata.Use != "" && metadata.Use != "sig") {
			continue
		}
		if metadata.KeyOps != nil && (len(metadata.KeyOps) != 1 || metadata.KeyOps[0] != "verify") {
			continue
		}

		// Delegate JWK decoding and curve-point validation to the JOSE library.
		var jwk jose.JSONWebKey
		if err := json.Unmarshal(raw, &jwk); err != nil || !jwk.IsPublic() || !jwk.Valid() {
			return nil, ErrJWKSUnavailable
		}
		key, ok := jwk.Key.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P256() {
			return nil, ErrJWKSUnavailable
		}
		keys[metadata.KeyID] = key
	}
	return keys, nil
}
