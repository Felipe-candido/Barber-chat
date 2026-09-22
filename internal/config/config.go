package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RabbitMQURL     string
	LogLevel        slog.Level
	DBTimeout       time.Duration
	ShutdownTimeout time.Duration
	WorkerInterval  time.Duration
	DevShopSlug     string
}

// Load reads the optional .env in the working directory, then validates settings.
// Process environment values take precedence, including explicitly empty values.
func Load() (Config, error) {
	fileValues, err := godotenv.Read(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Parser errors can contain file contents, including credentials.
		return Config{}, errors.New("could not read .env: check file access and KEY=VALUE syntax")
	}
	return load(func(key string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return fileValues[key]
	})
}

func load(getenv func(string) string) (Config, error) {
	value := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback
	}
	c := Config{
		HTTPAddr:    value("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		RabbitMQURL: getenv("RABBITMQ_URL"),
		DevShopSlug: strings.TrimSpace(getenv("DEV_SHOP_SLUG")),
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR must contain a host and port between 1 and 65535")
	}
	if c.DevShopSlug != "" {
		host, _, _ := net.SplitHostPort(c.HTTPAddr)
		if !net.ParseIP(host).IsLoopback() {
			return Config{}, fmt.Errorf("DEV_SHOP_SLUG requires a loopback IP in HTTP_ADDR")
		}
	}
	if err := validateURL(c.DatabaseURL, "DATABASE_URL", "postgres", "postgresql"); err != nil {
		return Config{}, err
	}
	if c.RabbitMQURL != "" {
		if err := validateURL(c.RabbitMQURL, "RABBITMQ_URL", "amqp", "amqps"); err != nil {
			return Config{}, err
		}
	}
	if err := c.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL", "INFO"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL must be a valid slog level")
	}
	for _, setting := range []struct {
		key      string
		fallback string
		target   *time.Duration
	}{
		{"DB_TIMEOUT", "3s", &c.DBTimeout},
		{"SHUTDOWN_TIMEOUT", "10s", &c.ShutdownTimeout},
		{"WORKER_INTERVAL", "30s", &c.WorkerInterval},
	} {
		d, err := time.ParseDuration(value(setting.key, setting.fallback))
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", setting.key)
		}
		*setting.target = d
	}
	return c, nil
}

func validateURL(raw, key string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err == nil && u.Hostname() != "" {
		for _, scheme := range schemes {
			if u.Scheme == scheme {
				return nil
			}
		}
	}
	// Never return the raw value: connection URLs may contain credentials.
	return fmt.Errorf("%s must be a URL with a host and scheme %s", key, strings.Join(schemes, " or "))
}
