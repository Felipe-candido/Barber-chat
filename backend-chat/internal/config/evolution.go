package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func (c *Config) loadEvolution(getenv func(string) string) error {
	c.EvolutionBaseURL = strings.TrimSpace(getenv("EVOLUTION_BASE_URL"))
	c.EvolutionAPIKey = strings.TrimSpace(getenv("EVOLUTION_API_KEY"))
	c.EvolutionWebhookURL = strings.TrimSpace(getenv("EVOLUTION_WEBHOOK_URL"))
	c.EvolutionWebhookSecret = strings.TrimSpace(getenv("EVOLUTION_WEBHOOK_SECRET"))
	c.EvolutionWebhookAddr = strings.TrimSpace(getenv("EVOLUTION_WEBHOOK_ADDR"))
	if c.EvolutionBaseURL == "" && c.EvolutionAPIKey == "" && c.EvolutionWebhookURL == "" && c.EvolutionWebhookSecret == "" && c.EvolutionWebhookAddr == "" {
		return nil
	}
	invalid := errors.New("configure EVOLUTION_BASE_URL, EVOLUTION_API_KEY, EVOLUTION_WEBHOOK_URL and a server-only EVOLUTION_WEBHOOK_SECRET of at least 32 characters")
	for _, raw := range []string{c.EvolutionBaseURL, c.EvolutionWebhookURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || u.ForceQuery || u.RawQuery != "" || strings.Contains(raw, "#") || (u.Scheme != "http" && u.Scheme != "https") {
			return invalid
		}
	}
	if len(c.EvolutionAPIKey) < 16 || len(c.EvolutionAPIKey) > 512 || strings.ContainsAny(c.EvolutionAPIKey, "\r\n") || len(c.EvolutionWebhookSecret) < 32 || len(c.EvolutionWebhookSecret) > 512 {
		return invalid
	}
	u, _ := url.Parse(c.EvolutionBaseURL)
	if u.Path != "" && u.Path != "/" {
		return invalid
	}
	u, _ = url.Parse(c.EvolutionWebhookURL)
	if u.Path != "/api/v1/webhooks/evolution" {
		return invalid
	}
	if c.EvolutionWebhookAddr != "" {
		host, port, err := net.SplitHostPort(c.EvolutionWebhookAddr)
		number, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || number < 1 || number > 65535 || host == "" {
			return errors.New("EVOLUTION_WEBHOOK_ADDR must be an explicit host:port")
		}
	}
	return nil
}
