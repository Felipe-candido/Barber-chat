package config_test

import (
	"testing"
	"time"
)

func TestAuthenticatedFrontendOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"", true}, {"http://localhost:3000", true}, {"http://127.0.0.1:3000", true}, {"http://[::1]:3000", true},
		{"https://frontend.example", true}, {"http://frontend.example", false}, {"*", false}, {"null", false},
		{"https://user:secret@frontend.example", false}, {"https://frontend.example/", false},
		{"https://frontend.example?", false}, {"https://frontend.example#", false},
		{"https://frontend.example:", false}, {"https://frontend.example:0", false}, {"https://frontend.example:65536", false},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			_, err := loadConfig(t, func(key string) string {
				return map[string]string{"DATABASE_URL": "postgres://localhost/test", "FRONTEND_ORIGIN": tc.origin, "HTTP_ADDR": "0.0.0.0:8080"}[key]
			})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
		})
	}
}

func TestAuthenticationConfigurationAndLegacyOriginAlias(t *testing.T) {
	cfg, err := loadConfig(t, func(key string) string {
		return map[string]string{
			"DATABASE_URL": "postgres://localhost/test", "SUPABASE_URL": " https://project.supabase.co ",
			"DEV_FRONTEND_ORIGIN": "http://localhost:3000", "HTTP_TIMEOUT": "5s",
		}[key]
	})
	if err != nil || cfg.SupabaseURL != "https://project.supabase.co" || cfg.FrontendOrigin != "http://localhost:3000" || cfg.HTTPTimeout != 5*time.Second {
		t.Fatal("authentication configuration was not loaded", err)
	}
	_, err = loadConfig(t, func(key string) string {
		return map[string]string{"DATABASE_URL": "postgres://localhost/test", "FRONTEND_ORIGIN": "http://localhost:3001", "DEV_FRONTEND_ORIGIN": "http://localhost:3000"}[key]
	})
	if err == nil {
		t.Fatal("conflicting origin settings accepted")
	}
}
