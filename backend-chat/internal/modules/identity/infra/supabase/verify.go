package supabase

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Verifier validates Supabase ES256 access tokens using a shared JWKS cache.
// Construct it with NewVerifier; it is safe for concurrent use but must not be copied.
type Verifier struct {
	jwksURL string
	client  *http.Client
	parser  *jwt.Parser
	keys    keyCache
}

const (
	requestTimeout = 5 * time.Second
	maxTokenBytes  = 32 * 1024
	maxKeyIDBytes  = 256
)

// Verify the application port at compile time.
var _ application.AccessTokenVerifier = (*Verifier)(nil)

// Provider-specific claims never cross the application boundary.
type accessClaims struct {
	jwt.RegisteredClaims

	Role string `json:"role"`
	IsAnonymous bool   `json:"is_anonymous"`
}

// NewVerifier accepts a hosted project's HTTPS origin, not its Auth or JWKS URL.
// A nil client uses the default HTTP transport. A provided client is copied;
// redirects and cookies are disabled and its timeout is capped at five seconds.
// Construction performs no network requests. Reuse one verifier across requests.
func NewVerifier(supabaseURL string, client *http.Client) (*Verifier, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(supabaseURL), "/")

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("invalid Supabase URL")
	}

	if parsed.Scheme != "https" ||
		parsed.Hostname() == "" ||
		parsed.User != nil ||
		parsed.Opaque != "" ||
		parsed.Path != "" ||
		parsed.ForceQuery ||
		parsed.RawQuery != "" ||
		strings.Contains(baseURL, "#") ||
		strings.HasSuffix(parsed.Host, ":") {
		return nil, errors.New("invalid Supabase project URL")
	}

	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("invalid Supabase project URL port")
		}
	}

	issuer := baseURL + "/auth/v1"
	httpClient := http.Client{}
	if client != nil {
		httpClient = *client
	}
	if httpClient.Timeout <= 0 || httpClient.Timeout > requestTimeout {
		httpClient.Timeout = requestTimeout
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &Verifier{
		jwksURL: issuer + "/.well-known/jwks.json",
		client: &httpClient,
		keys: keyCache{gate: make(chan struct{}, 1)},
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{"ES256"}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience("authenticated"),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
		),
	}, nil
}

// VerifyAccessToken returns only a verified subject. It neither checks the
// current server-side session state nor grants local user or shop access.
func (v *Verifier) VerifyAccessToken(
	ctx context.Context,
	accessToken string,
) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}

	if len(accessToken) > maxTokenBytes || strings.TrimSpace(accessToken) == "" {
		return uuid.Nil, application.ErrInvalidAccessToken
	}

	var claims accessClaims

	// Preserve operational errors separately from parser/signature failures.
	var keyErr error

	token, err := v.parser.ParseWithClaims(
		accessToken,
		&claims,
		func(token *jwt.Token) (any, error) {
			// No critical JWS extensions are implemented by this adapter.
			if _, present := token.Header["crit"]; present {
				keyErr = application.ErrInvalidAccessToken
				return nil, keyErr
			}
			kid, ok := token.Header["kid"].(string)
			if !ok || strings.TrimSpace(kid) == "" || len(kid) > maxKeyIDBytes {
				keyErr = application.ErrInvalidAccessToken
				return nil, keyErr
			}

			key, err := v.fetchPublicKey(ctx, kid)
			if err != nil {
				keyErr = err
				return nil, err
			}

			return key, nil
		},
	)

	if ctxErr := ctx.Err(); ctxErr != nil {
		return uuid.Nil, ctxErr
	}

	// A failed key lookup is not necessarily an invalid credential.
	if keyErr != nil {
		return uuid.Nil, keyErr
	}

	if err != nil || token == nil || !token.Valid {
		// Never expose the token or parser details through the public error.
		return uuid.Nil, application.ErrInvalidAccessToken
	}

	// This role identifies a session, not permission to access a local shop.
	if claims.Role != "authenticated" || claims.IsAnonymous {
		return uuid.Nil, application.ErrInvalidAccessToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil || userID == uuid.Nil {
		return uuid.Nil, application.ErrInvalidAccessToken
	}

	return userID, nil
}
