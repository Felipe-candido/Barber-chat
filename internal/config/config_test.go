package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadEnvFile(t *testing.T) {
	for _, test := range []struct {
		name        string
		contents    string
		missing     bool
		directory   bool
		environment map[string]string
		wantURL     string
		wantError   string
	}{
		{
			name:     "file settings",
			contents: "# Local settings\nDATABASE_URL='postgres://user:pa$word@localhost/from_file'\nDB_TIMEOUT=8s\n",
			wantURL:  "postgres://user:pa$word@localhost/from_file",
		},
		{
			name:        "environment takes precedence",
			contents:    "DATABASE_URL=postgres://localhost/from_file\n",
			environment: map[string]string{"DATABASE_URL": "postgres://localhost/from_environment"},
			wantURL:     "postgres://localhost/from_environment",
		},
		{
			name:        "explicitly empty environment does not fall back",
			contents:    "DATABASE_URL=postgres://localhost/from_file\n",
			environment: map[string]string{"DATABASE_URL": ""},
			wantError:   "DATABASE_URL",
		},
		{
			name:        "environment without file",
			missing:     true,
			environment: map[string]string{"DATABASE_URL": "postgres://localhost/production"},
			wantURL:     "postgres://localhost/production",
		},
		{name: "missing required setting", missing: true, wantError: "DATABASE_URL"},
		{name: "invalid file", contents: "DATABASE_URL=postgres://localhost/barber\nINVALID!KEY=secret\n", wantError: "could not read .env"},
		{name: "unreadable file", directory: true, wantError: "could not read .env"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for _, key := range []string{"HTTP_ADDR", "DATABASE_URL", "RABBITMQ_URL", "LOG_LEVEL", "DB_TIMEOUT", "SHUTDOWN_TIMEOUT", "WORKER_INTERVAL", "DEV_SHOP_SLUG"} {
				// Register restoration before unsetting inherited configuration.
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			for key, value := range test.environment {
				t.Setenv(key, value)
			}
			if test.directory {
				if err := os.Mkdir(".env", 0700); err != nil {
					t.Fatal(err)
				}
			} else if !test.missing {
				if err := os.WriteFile(".env", []byte(test.contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("expected %q, got %v", test.wantError, err)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("error leaked file contents")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if cfg.DatabaseURL != test.wantURL {
					t.Fatal("unexpected database configuration")
				}
				if test.name == "file settings" && cfg.DBTimeout != 8*time.Second {
					t.Fatal("file timeout was not loaded")
				}
			}
			if _, exists := test.environment["DATABASE_URL"]; !exists {
				if _, exists := os.LookupEnv("DATABASE_URL"); exists {
					t.Fatal("loading configuration mutated the process environment")
				}
			}
		})
	}
}

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
