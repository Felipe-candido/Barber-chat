package config

import (
	"strings"
	"testing"
)

func TestDevelopmentWritesRequireLoopbackListener(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "[::1]:8080", "0.0.0.0:8080", ":8080", "192.0.2.1:8080", "localhost:8080"} {
		t.Run(address, func(t *testing.T) {
			cfg, err := load(func(key string) string {
				return map[string]string{"DATABASE_URL": "postgres://localhost/test", "HTTP_ADDR": address, "DEV_SHOP_SLUG": " test-shop "}[key]
			})
			allowed := address == "127.0.0.1:8080" || address == "[::1]:8080"
			if allowed && (err != nil || cfg.DevShopSlug != "test-shop") {
				t.Fatalf("loopback config failed: %v", err)
			}
			if !allowed && (err == nil || !strings.Contains(err.Error(), "DEV_SHOP_SLUG")) {
				t.Fatalf("non-loopback config accepted: %v", err)
			}
		})
	}
}
