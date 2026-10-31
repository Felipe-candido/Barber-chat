package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

// SessionClient uses a publishable (or legacy anon) API key, not admin powers.
// It asks Auth for the current user only during registration. Ordinary admin
// requests continue to use the single shared verifier/cache and local identity.
type SessionClient struct {
	endpoint, key string
	client        *http.Client
}

func NewSessionClient(projectURL, publishableKey string, client *http.Client) (*SessionClient, error) {
	base := strings.TrimRight(strings.TrimSpace(projectURL), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Path != "" || u.ForceQuery || u.RawQuery != "" || strings.Contains(base, "#") || strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("invalid Supabase registration project URL")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid Supabase registration project port")
		}
	}
	key := strings.TrimSpace(publishableKey)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, application.ErrRegistrationNotConfigured
	}
	h := http.Client{}
	if client != nil {
		h = *client
	}
	if h.Timeout <= 0 || h.Timeout > requestTimeout {
		h.Timeout = requestTimeout
	}
	h.Jar = nil
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &SessionClient{endpoint: base + "/auth/v1/user", key: key, client: &h}, nil
}

func (c *SessionClient) ReadRegistrationAccount(ctx context.Context, token string) (application.RegistrationAccount, error) {
	if token == "" || len(token) > maxTokenBytes || strings.ContainsAny(token, " \r\n\t") {
		return application.RegistrationAccount{}, application.ErrInvalidAccessToken
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return application.RegistrationAccount{}, application.ErrRegistrationUnavailable
	}
	r.Header.Set("apikey", c.key)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return application.RegistrationAccount{}, ctx.Err()
		}
		return application.RegistrationAccount{}, application.ErrRegistrationUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return application.RegistrationAccount{}, application.ErrInvalidAccessToken
	}
	if resp.StatusCode != http.StatusOK {
		return application.RegistrationAccount{}, application.ErrRegistrationUnavailable
	}
	const limit = 64 * 1024
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(raw) > limit {
		return application.RegistrationAccount{}, application.ErrRegistrationUnavailable
	}
	var user struct {
		ID               uuid.UUID  `json:"id"`
		Email            string     `json:"email"`
		EmailConfirmedAt *time.Time `json:"email_confirmed_at"`
		Anonymous        bool       `json:"is_anonymous"`
	}
	if err := json.Unmarshal(raw, &user); err != nil || user.ID == uuid.Nil {
		return application.RegistrationAccount{}, application.ErrRegistrationUnavailable
	}
	return application.RegistrationAccount{UserID: user.ID, Anonymous: user.Anonymous,
		EmailConfirmed: strings.TrimSpace(user.Email) != "" && user.EmailConfirmedAt != nil && !user.EmailConfirmedAt.IsZero()}, nil
}
