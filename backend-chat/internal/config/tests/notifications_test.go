package config_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestNotificationPlanningConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	for _, tc := range []struct {
		name, flag, active, keys string
		valid, enabled           bool
	}{
		{name: "default off", valid: true},
		{name: "configured", flag: "true", active: "key-1", keys: `{"key-1":"` + key + `"}`, valid: true, enabled: true},
		{name: "retained keys while off", flag: "false", active: "key-1", keys: `{"key-1":"` + key + `"}`, valid: true},
		{name: "missing keyring", flag: "true"},
		{name: "missing active key", flag: "true", active: "missing", keys: `{"key-1":"` + key + `"}`},
		{name: "invalid key size", flag: "true", active: "key-1", keys: `{"key-1":"YWJj"}`},
		{name: "invalid flag", flag: "sometimes"},
		{name: "invalid secret JSON", flag: "true", active: "key-1", keys: `private-test-value`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(t, func(name string) string {
				return map[string]string{
					"DATABASE_URL": "postgres://localhost/test", "NOTIFICATION_PLANNING_ENABLED": tc.flag,
					"NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID": tc.active, "NOTIFICATION_PAYLOAD_KEYS": tc.keys,
				}[name]
			})
			if tc.valid && (err != nil || cfg.NotificationPlanningEnabled != tc.enabled) {
				t.Fatal("valid configuration rejected", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if err != nil && (strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "private-test-value")) {
				t.Fatal("configuration error exposed a secret")
			}
		})
	}
}
