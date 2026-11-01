package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

func (c *Config) loadNotifications(getenv func(string) string) error {
	flag := strings.TrimSpace(getenv("NOTIFICATION_PLANNING_ENABLED"))
	if flag != "" {
		enabled, err := strconv.ParseBool(flag)
		if err != nil {
			return errors.New("NOTIFICATION_PLANNING_ENABLED must be a boolean")
		}
		c.NotificationPlanningEnabled = enabled
	}
	active := strings.TrimSpace(getenv("NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID"))
	raw := strings.TrimSpace(getenv("NOTIFICATION_PAYLOAD_KEYS"))
	if !c.NotificationPlanningEnabled && active == "" && raw == "" {
		return nil
	}
	invalid := errors.New("configure NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID and NOTIFICATION_PAYLOAD_KEYS with a JSON object of base64-encoded 32-byte server keys")
	validID := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	if !validID.MatchString(active) || len(raw) == 0 || len(raw) > 4096 {
		return invalid
	}
	var encoded map[string]string
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil || len(encoded) == 0 || len(encoded) > 16 {
		return invalid
	}
	keys := make(map[string][]byte, len(encoded))
	for id, value := range encoded {
		key, err := base64.StdEncoding.Strict().DecodeString(value)
		if !validID.MatchString(id) || err != nil || len(key) != 32 {
			return invalid
		}
		keys[id] = key
	}
	if keys[active] == nil {
		return invalid
	}
	c.NotificationPayloadActiveKeyID, c.NotificationPayloadKeys = active, keys
	return nil
}
