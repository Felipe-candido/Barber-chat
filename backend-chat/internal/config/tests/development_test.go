package config_test

import (
	"strings"
	"testing"
)

func TestLegacyFrontendOriginRequiresLoopbackListener(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "[::1]:8080", "0.0.0.0:8080", ":8080", "192.0.2.1:8080", "localhost:8080"} {
		t.Run(address, func(t *testing.T) {
			cfg, err := loadConfig(t, func(key string) string {
				return map[string]string{"DATABASE_URL": "postgres://localhost/test", "HTTP_ADDR": address, "DEV_FRONTEND_ORIGIN": "http://localhost:3000"}[key]
			})
			allowed := address == "127.0.0.1:8080" || address == "[::1]:8080"
			if allowed && (err != nil || cfg.FrontendOrigin != "http://localhost:3000") {
				t.Fatalf("loopback config failed: %v", err)
			}
			if !allowed && (err == nil || !strings.Contains(err.Error(), "DEV_FRONTEND_ORIGIN")) {
				t.Fatalf("non-loopback config accepted: %v", err)
			}
		})
	}
}

func TestFixedShopSettingIsIgnored(t *testing.T) {
	_, err := loadConfig(t, func(key string) string {
		return map[string]string{"DATABASE_URL": "postgres://localhost/test", "HTTP_ADDR": "0.0.0.0:8080", "DEV_SHOP_SLUG": "old-fixed-shop"}[key]
	})
	if err != nil {
		t.Fatal("obsolete fixed shop setting still influences startup", err)
	}
}
