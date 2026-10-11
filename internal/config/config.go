// Package config loads and validates environment-based server configuration.
package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config contains the settings required to start one sandbox application instance.
// MasterKey is sensitive and must never be included in logs or API responses.
type Config struct {
	Environment        string
	HTTPAddr           string
	PublicURL          string
	DatabasePath       string
	WebDistDir         string
	MigrationsDir      string
	MasterKey          [32]byte `json:"-"`
	SecureCookies      bool
	AllowLocalWebhooks bool
	TrustedProxies     []string
	SessionTTL         time.Duration
	PaymentTTL         time.Duration
	WebhookTimeout     time.Duration
	WebhookMaxAttempts int
	ShutdownTimeout    time.Duration
}

// Load reads configuration from the process environment.
func Load() (Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (Config, error) {
	value := func(name, fallback string) string {
		if raw := getenv(name); raw != "" {
			return raw
		}
		return fallback
	}
	cfg := Config{
		Environment:   value("APP_ENV", "production"),
		HTTPAddr:      value("HTTP_ADDR", ":8080"),
		PublicURL:     value("PUBLIC_URL", "http://localhost:8080"),
		DatabasePath:  value("DATABASE_PATH", "data/phajay.db"),
		WebDistDir:    value("WEB_DIST_DIR", "web/dist"),
		MigrationsDir: value("MIGRATIONS_DIR", "migrations"),
	}
	key, err := base64.StdEncoding.DecodeString(getenv("MASTER_KEY"))
	if err != nil || len(key) != len(cfg.MasterKey) {
		return Config{}, fmt.Errorf("MASTER_KEY must be a base64-encoded 32-byte encryption key")
	}
	copy(cfg.MasterKey[:], key)
	if cfg.Environment != "production" && cfg.Environment != "development" {
		return Config{}, fmt.Errorf("APP_ENV must be production or development")
	}
	host, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil || strings.ContainsAny(host, " \t\r\n/?#") || !validPort(port) {
		return Config{}, fmt.Errorf("HTTP_ADDR must be a host:port address with a port between 1 and 65535")
	}
	origin, err := url.Parse(cfg.PublicURL)
	if err != nil || origin.Hostname() == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawPath != "" || origin.RawQuery != "" || origin.ForceQuery || strings.Contains(cfg.PublicURL, "#") || strings.HasSuffix(origin.Host, ":") {
		return Config{}, fmt.Errorf("PUBLIC_URL must be an absolute HTTP(S) origin without credentials, a path, query, or fragment")
	}
	if port := origin.Port(); port != "" && !validPort(port) {
		return Config{}, fmt.Errorf("PUBLIC_URL port must be between 1 and 65535")
	}
	// Browsers lowercase hostnames and omit default ports from Origin headers.
	// Use that same representation for CSRF checks and generated payment URLs.
	hostname := strings.ToLower(origin.Hostname())
	port = origin.Port()
	if port != "" {
		numericPort, _ := strconv.Atoi(port) // Already validated above.
		port = strconv.Itoa(numericPort)
		if (origin.Scheme == "http" && numericPort == 80) || (origin.Scheme == "https" && numericPort == 443) {
			port = ""
		}
	}
	if port != "" {
		origin.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		origin.Host = "[" + hostname + "]"
	} else {
		origin.Host = hostname
	}
	origin.Path = ""
	cfg.PublicURL = origin.String()
	cfg.SecureCookies = origin.Scheme == "https"
	if cfg.DatabasePath == ":memory:" || strings.HasPrefix(strings.ToLower(cfg.DatabasePath), "file:") || strings.ContainsRune(cfg.DatabasePath, '\x00') {
		return Config{}, fmt.Errorf("DATABASE_PATH must be a persistent file path, not a SQLite URI or memory database")
	}
	cfg.DatabasePath = filepath.Clean(cfg.DatabasePath)
	if cfg.DatabasePath == "." || cfg.DatabasePath == string(filepath.Separator) {
		return Config{}, fmt.Errorf("DATABASE_PATH must identify a database file")
	}
	cfg.AllowLocalWebhooks, err = strconv.ParseBool(value("ALLOW_LOCAL_WEBHOOKS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("ALLOW_LOCAL_WEBHOOKS must be true or false")
	}
	if cfg.AllowLocalWebhooks && cfg.Environment != "development" {
		return Config{}, fmt.Errorf("ALLOW_LOCAL_WEBHOOKS requires APP_ENV=development")
	}
	if raw := getenv("TRUSTED_PROXIES"); raw != "" {
		for _, entry := range strings.Split(raw, ",") {
			entry = strings.TrimSpace(entry)
			if _, err := netip.ParseAddr(entry); err != nil {
				prefix, err := netip.ParsePrefix(entry)
				if err != nil || prefix.Bits() == 0 {
					return Config{}, fmt.Errorf("TRUSTED_PROXIES must contain explicit IP addresses or CIDRs; wildcard trust is prohibited")
				}
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, entry)
		}
	}
	for _, setting := range []struct {
		name                       string
		fallback, minimum, maximum time.Duration
		destination                *time.Duration
	}{
		{"SESSION_TTL", 24 * time.Hour, 5 * time.Minute, 30 * 24 * time.Hour, &cfg.SessionTTL},
		{"PAYMENT_TTL", 15 * time.Minute, time.Minute, 24 * time.Hour, &cfg.PaymentTTL},
		{"WEBHOOK_TIMEOUT", 10 * time.Second, time.Second, 30 * time.Second, &cfg.WebhookTimeout},
		{"SHUTDOWN_TIMEOUT", 15 * time.Second, time.Second, time.Minute, &cfg.ShutdownTimeout},
	} {
		duration, err := time.ParseDuration(value(setting.name, setting.fallback.String()))
		if err != nil || duration < setting.minimum || duration > setting.maximum {
			return Config{}, fmt.Errorf("%s must be a duration between %s and %s", setting.name, setting.minimum, setting.maximum)
		}
		*setting.destination = duration
	}
	cfg.WebhookMaxAttempts, err = strconv.Atoi(value("WEBHOOK_MAX_ATTEMPTS", "8"))
	if err != nil || cfg.WebhookMaxAttempts < 1 || cfg.WebhookMaxAttempts > 12 {
		return Config{}, fmt.Errorf("WEBHOOK_MAX_ATTEMPTS must be an integer between 1 and 12")
	}
	return cfg, nil
}

func validPort(raw string) bool {
	port, err := strconv.Atoi(raw)
	return err == nil && port >= 1 && port <= 65535
}
