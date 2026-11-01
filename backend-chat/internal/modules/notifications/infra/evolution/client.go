package evolution

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
)

var instancePattern = regexp.MustCompile(`^bc_[0-9a-f]{32}$`)

const WebhookHeader = "X-BarberChat-Evolution"

type Client struct {
	base, key, callback, secret string
	http                        *http.Client
}

func NewClient(base, key, callback, secret string, client *http.Client) (*Client, error) {
	for _, raw := range []string{base, callback} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, app.ErrProviderProtocol
		}
	}
	if len(key) < 16 || len(key) > 512 || strings.ContainsAny(key, "\r\n") || len(secret) < 32 || len(secret) > 512 {
		return nil, app.ErrProviderProtocol
	}
	h := http.Client{Timeout: 8 * time.Second}
	if client != nil {
		h = *client
		if h.Timeout <= 0 || h.Timeout > 8*time.Second {
			h.Timeout = 8 * time.Second
		}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{strings.TrimRight(base, "/"), key, callback, secret, &h}, nil
}

func WebhookToken(secret, name string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte("barber-chat/evolution-webhook/v1/" + name))
	return hex.EncodeToString(m.Sum(nil))
}
func ValidInstanceName(name string) bool { return instancePattern.MatchString(name) }

func (c *Client) request(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, 0, app.ErrProviderProtocol
		}
	}
	defer clear(data)
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(data))
	if err != nil {
		return nil, 0, app.ErrProviderProtocol
	}
	r.Header.Set("apikey", c.key)
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, 0, app.ErrProviderUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024+1))
	if err != nil {
		return nil, resp.StatusCode, app.ErrProviderUnavailable
	}
	if len(raw) > 512*1024 {
		return nil, resp.StatusCode, app.ErrProviderProtocol
	}
	var failure struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &failure)
	if failure.Code == "LICENSE_REQUIRED" {
		return nil, resp.StatusCode, app.ErrProviderActivationRequired
	}
	if resp.StatusCode == 404 {
		return nil, 404, app.ErrProviderInstanceMissing
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, resp.StatusCode, app.ErrProviderCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, app.ErrProviderUnavailable
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil || envelope == nil || string(envelope["error"]) == "true" {
		return nil, resp.StatusCode, app.ErrProviderProtocol
	}
	return raw, resp.StatusCode, nil
}

type instanceResponse struct {
	Instance struct {
		InstanceName string `json:"instanceName"`
		State        string `json:"state"`
	} `json:"instance"`
	Base64 string `json:"base64"`
	Count  *int   `json:"count"`
	QRCode struct {
		Base64 string `json:"base64"`
	} `json:"qrcode"`
	Error bool `json:"error"`
}

func state(raw string) (domain.ConnectionState, error) {
	switch raw {
	case "open":
		return domain.ConnectionConnected, nil
	case "close":
		return domain.ConnectionDisconnected, nil
	case "connecting":
		return domain.ConnectionConnecting, nil
	}
	return domain.ConnectionUnknown, app.ErrProviderProtocol
}
func (c *Client) Observe(ctx context.Context, name string) (app.ConnectionObservation, error) {
	if !ValidInstanceName(name) {
		return app.ConnectionObservation{}, app.ErrProviderProtocol
	}
	raw, _, err := c.request(ctx, "GET", "/instance/connectionState/"+name, nil)
	if err != nil {
		return app.ConnectionObservation{}, err
	}
	defer clear(raw)
	var r instanceResponse
	if json.Unmarshal(raw, &r) != nil || r.Error || r.Instance.InstanceName != name {
		return app.ConnectionObservation{}, app.ErrProviderProtocol
	}
	s, err := state(r.Instance.State)
	return app.ConnectionObservation{State: s}, err
}

func (c *Client) Connect(ctx context.Context, name string) (app.ConnectionChallenge, error) {
	o, err := c.Observe(ctx, name)
	if errors.Is(err, app.ErrProviderInstanceMissing) {
		raw, status, createErr := c.request(ctx, "POST", "/instance/create", map[string]any{"instanceName": name, "integration": "WHATSAPP-BAILEYS", "qrcode": false, "groupsIgnore": true, "readMessages": false, "readStatus": false, "syncFullHistory": false, "alwaysOnline": false})
		clear(raw)
		if createErr != nil {
			if status != 403 {
				return app.ConnectionChallenge{}, createErr
			}
			if _, err = c.Observe(ctx, name); err != nil {
				return app.ConnectionChallenge{}, err
			}
		}
	} else if err != nil {
		return app.ConnectionChallenge{}, err
	}
	_, _, err = c.request(ctx, "POST", "/webhook/set/"+name, map[string]any{"webhook": map[string]any{"enabled": true, "url": c.callback, "headers": map[string]string{WebhookHeader: WebhookToken(c.secret, name)}, "byEvents": false, "base64": false, "events": []string{"CONNECTION_UPDATE"}}})
	if err != nil {
		return app.ConnectionChallenge{}, err
	}
	if o.State == domain.ConnectionConnected {
		return app.ConnectionChallenge{Observation: o}, nil
	}
	raw, _, err := c.request(ctx, "GET", "/instance/connect/"+name, nil)
	if err != nil {
		return app.ConnectionChallenge{}, err
	}
	defer clear(raw)
	var r instanceResponse
	if json.Unmarshal(raw, &r) != nil || r.Error {
		return app.ConnectionChallenge{}, app.ErrProviderProtocol
	}
	qr := r.Base64
	if qr == "" {
		qr = r.QRCode.Base64
	}
	if qr != "" {
		if !validQR(qr) {
			return app.ConnectionChallenge{}, app.ErrProviderProtocol
		}
		return app.ConnectionChallenge{Observation: app.ConnectionObservation{State: domain.ConnectionQRRequired}, QRBase64: qr, DiscardAfter: time.Now().UTC().Add(20 * time.Second)}, nil
	}
	if r.Instance.InstanceName != "" && r.Instance.InstanceName != name {
		return app.ConnectionChallenge{}, app.ErrProviderProtocol
	}
	if r.Instance.State != "" {
		s, err := state(r.Instance.State)
		return app.ConnectionChallenge{Observation: app.ConnectionObservation{State: s}}, err
	}
	if r.Count == nil || *r.Count < 0 {
		return app.ConnectionChallenge{}, app.ErrProviderProtocol
	}
	return app.ConnectionChallenge{Observation: app.ConnectionObservation{State: domain.ConnectionConnecting}}, nil
}

func (c *Client) Disconnect(ctx context.Context, name string) (app.ConnectionObservation, error) {
	o, err := c.Observe(ctx, name)
	if errors.Is(err, app.ErrProviderInstanceMissing) {
		return app.ConnectionObservation{State: domain.ConnectionDisconnected}, nil
	}
	if err != nil {
		return app.ConnectionObservation{}, err
	}
	if o.State == domain.ConnectionDisconnected {
		return o, nil
	}
	raw, _, err := c.request(ctx, "DELETE", "/instance/logout/"+name, nil)
	clear(raw)
	if err != nil {
		return app.ConnectionObservation{}, err
	}
	return c.Observe(ctx, name)
}
func validQR(qr string) bool {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(qr, prefix) || len(qr) > 200000 {
		return false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(qr, prefix))
	return err == nil && len(data) > 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
}
