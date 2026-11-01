package config_test

import (
	"strings"
	"testing"
)

func TestEvolutionConfigurationRequiresCompleteServerSettings(t *testing.T) {
	for _, scenario := range []string{"off", "valid", "incomplete", "bad url", "short secret", "wrong callback", "bad listener"} {
		t.Run(scenario, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://localhost/test"}
			if scenario != "off" {
				values["EVOLUTION_BASE_URL"] = "http://127.0.0.1:8081"
				values["EVOLUTION_API_KEY"] = "test-api-key-at-least-16"
				values["EVOLUTION_WEBHOOK_URL"] = "http://host.docker.internal:8082/api/v1/webhooks/evolution"
				values["EVOLUTION_WEBHOOK_SECRET"] = strings.Repeat("s", 32)
			}
			switch scenario {
			case "incomplete":
				values["EVOLUTION_API_KEY"] = ""
			case "bad url":
				values["EVOLUTION_BASE_URL"] = "http://user:secret@host"
			case "short secret":
				values["EVOLUTION_WEBHOOK_SECRET"] = "private-short"
			case "wrong callback":
				values["EVOLUTION_WEBHOOK_URL"] = "http://host/wrong"
			case "bad listener":
				values["EVOLUTION_WEBHOOK_ADDR"] = ":8082"
			}
			_, err := loadConfig(t, func(k string) string { return values[k] })
			valid := scenario == "off" || scenario == "valid"
			if (err == nil) != valid {
				t.Fatal("incorrect config validation", err)
			}
			if err != nil && (strings.Contains(err.Error(), "private-short") || strings.Contains(err.Error(), "user:secret")) {
				t.Fatal("configuration error leaked secret")
			}
		})
	}
}
