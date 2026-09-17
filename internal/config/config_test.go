package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg, err := load(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://localhost/barber"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" || cfg.DBTimeout != 3*time.Second || cfg.RabbitMQURL != "" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"DATABASE_URL", ""},
		{"DATABASE_URL", "postgres://user:secret@[invalid"},
		{"DATABASE_URL", "https://localhost/db"},
		{"RABBITMQ_URL", "http://user:secret@localhost"},
		{"HTTP_ADDR", "localhost"},
		{"HTTP_ADDR", "localhost:0"},
		{"HTTP_ADDR", "localhost:65536"},
		{"DB_TIMEOUT", "0s"},
		{"SHUTDOWN_TIMEOUT", "-1s"},
		{"WORKER_INTERVAL", "oops"},
		{"LOG_LEVEL", "oops"},
	} {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://localhost/barber", test.key: test.value}
			_, err := load(func(key string) string { return values[key] })
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("expected error for %s, got %v", test.key, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("error leaked credentials")
			}
		})
	}
}
