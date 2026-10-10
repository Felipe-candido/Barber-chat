package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

// AdminClient is server-only. The key must never be logged or returned to HTTP.
// Construct once; no automatic retries of external account creation.
type AdminClient struct {
	base, key, redirect string
	client              *http.Client
}

var _ application.StaffAccountProvider = (*AdminClient)(nil)

func NewAdminClient(projectURL, key, frontendOrigin string, client *http.Client) (*AdminClient, error) {
	projectURL = strings.TrimRight(strings.TrimSpace(projectURL), "/")
	u, err := url.Parse(projectURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.ForceQuery || u.RawQuery != "" || strings.Contains(projectURL, "#") {
		return nil, errors.New("invalid Supabase admin project URL")
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, application.ErrAccountNotConfigured
	}
	redirect, err := url.Parse(frontendOrigin)
	if err != nil || redirect.Hostname() == "" || redirect.User != nil || redirect.Path != "" || redirect.RawQuery != "" || redirect.ForceQuery || strings.Contains(frontendOrigin, "#") ||
		(redirect.Scheme != "https" && (redirect.Scheme != "http" || (redirect.Hostname() != "localhost" && !net.ParseIP(redirect.Hostname()).IsLoopback()))) {
		return nil, errors.New("staff invitations require a configured frontend origin")
	}
	httpClient := http.Client{}
	if client != nil {
		httpClient = *client
	}
	if httpClient.Timeout <= 0 || httpClient.Timeout > requestTimeout {
		httpClient.Timeout = requestTimeout
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &AdminClient{base: projectURL + "/auth/v1", key: key, redirect: frontendOrigin + "/definir-senha", client: &httpClient}, nil
}

func (c *AdminClient) Invite(ctx context.Context, email string) (application.ProviderUser, error) {
	body, _ := json.Marshal(struct {
		Email string `json:"email"`
	}{email})
	return c.request(ctx, http.MethodPost, "/invite?redirect_to="+url.QueryEscape(c.redirect), body)
}

func (c *AdminClient) FindUser(ctx context.Context, id uuid.UUID) (application.ProviderUser, error) {
	if id == uuid.Nil {
		return application.ProviderUser{}, application.ErrInvalidAccount
	}
	return c.request(ctx, http.MethodGet, "/admin/users/"+id.String(), nil)
}

func (c *AdminClient) request(ctx context.Context, method, path string, body []byte) (application.ProviderUser, error) {
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	r.Header.Set("apikey", c.key)
	r.Header.Set("Authorization", "Bearer "+c.key)
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return application.ProviderUser{}, ctx.Err()
		}
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return application.ProviderUser{}, application.ErrAccountNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return application.ProviderUser{}, application.ErrAccountProviderRejected
		}
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	const limit = 64 * 1024
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(raw) > limit {
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	type responseUser struct {
		ID    uuid.UUID `json:"id"`
		Email string    `json:"email"`
	}
	var envelope struct {
		responseUser
		User *responseUser `json:"user"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	user := envelope.responseUser
	if envelope.User != nil {
		user = *envelope.User
	}
	if user.ID == uuid.Nil || user.Email == "" {
		return application.ProviderUser{}, application.ErrAccountProviderUnavailable
	}
	return application.ProviderUser{ID: user.ID, Email: user.Email}, nil
}
