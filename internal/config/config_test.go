package config

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validEnvironment() map[string]string {
	return map[string]string{"MASTER_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}
}

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(environment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.PublicURL != "http://localhost:8080" || cfg.DatabasePath != "data/phajay.db" {
		t.Fatalf("unexpected server defaults: address=%q URL=%q database=%q", cfg.HTTPAddr, cfg.PublicURL, cfg.DatabasePath)
	}
	if cfg.Environment != "production" || cfg.AllowLocalWebhooks || cfg.SecureCookies || len(cfg.TrustedProxies) != 0 {
		t.Fatal("unexpected security defaults")
	}
	if cfg.SessionTTL != 24*time.Hour || cfg.PaymentTTL != 15*time.Minute || cfg.WebhookTimeout != 10*time.Second || cfg.WebhookMaxAttempts != 8 || cfg.ShutdownTimeout != 15*time.Second {
		t.Fatal("unexpected lifecycle defaults")
	}
	if cfg.WebDistDir != "web/dist" || cfg.MigrationsDir != "migrations" {
		t.Fatal("unexpected asset directories")
	}
	if string(cfg.MasterKey[:]) != strings.Repeat("k", 32) {
		t.Fatal("master key was not decoded")
	}
}

func TestLoadOverrides(t *testing.T) {
	values := validEnvironment()
	for key, value := range map[string]string{
		"HTTP_ADDR": "[::]:8443", "PUBLIC_URL": "https://sandbox.example.com/",
		"DATABASE_PATH": "/app/data/phajay.db", "WEB_DIST_DIR": "/app/web", "MIGRATIONS_DIR": "/app/migrations",
		"APP_ENV": "development", "ALLOW_LOCAL_WEBHOOKS": "true",
		"TRUSTED_PROXIES": "127.0.0.1, 172.18.0.0/16, ::1",
		"SESSION_TTL":     "48h", "PAYMENT_TTL": "30m", "WEBHOOK_TIMEOUT": "5s",
		"WEBHOOK_MAX_ATTEMPTS": "5", "SHUTDOWN_TIMEOUT": "20s",
	} {
		values[key] = value
	}
	cfg, err := load(environment(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "[::]:8443" || cfg.PublicURL != "https://sandbox.example.com" || !cfg.SecureCookies {
		t.Fatal("HTTPS origin or listening address was not configured")
	}
	if cfg.DatabasePath != "/app/data/phajay.db" || cfg.WebDistDir != "/app/web" || cfg.MigrationsDir != "/app/migrations" {
		t.Fatal("persistent storage and asset paths were not configured")
	}
	if cfg.Environment != "development" || !cfg.AllowLocalWebhooks {
		t.Fatal("explicit development webhook configuration was not accepted")
	}
	if !reflect.DeepEqual(cfg.TrustedProxies, []string{"127.0.0.1", "172.18.0.0/16", "::1"}) {
		t.Fatalf("trusted proxies = %v", cfg.TrustedProxies)
	}
	if cfg.SessionTTL != 48*time.Hour || cfg.PaymentTTL != 30*time.Minute || cfg.WebhookTimeout != 5*time.Second || cfg.WebhookMaxAttempts != 5 || cfg.ShutdownTimeout != 20*time.Second {
		t.Fatal("lifecycle overrides were not applied")
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	for _, tt := range []struct{ name, key, value string }{
		{"missing encryption key", "MASTER_KEY", ""},
		{"invalid key encoding", "MASTER_KEY", "private-invalid-secret"},
		{"short encryption key", "MASTER_KEY", base64.StdEncoding.EncodeToString([]byte("private-short-secret"))},
		{"long encryption key", "MASTER_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 33)))},
		{"unsupported environment", "APP_ENV", "staging"},
		{"missing port", "HTTP_ADDR", "localhost"},
		{"ephemeral port", "HTTP_ADDR", ":0"},
		{"port overflow", "HTTP_ADDR", ":65536"},
		{"invalid port", "HTTP_ADDR", ":http"},
		{"invalid host", "HTTP_ADDR", "bad host:8080"},
		{"relative public URL", "PUBLIC_URL", "/sandbox"},
		{"unsupported URL scheme", "PUBLIC_URL", "ftp://sandbox.example.com"},
		{"missing URL host", "PUBLIC_URL", "https:///"},
		{"URL credentials", "PUBLIC_URL", "https://secret:password@sandbox.example.com"},
		{"URL path", "PUBLIC_URL", "https://sandbox.example.com/base"},
		{"URL query", "PUBLIC_URL", "https://sandbox.example.com?key=private"},
		{"empty URL query", "PUBLIC_URL", "https://sandbox.example.com?"},
		{"URL fragment", "PUBLIC_URL", "https://sandbox.example.com#private"},
		{"URL bad port", "PUBLIC_URL", "https://sandbox.example.com:70000"},
		{"memory database", "DATABASE_PATH", ":memory:"},
		{"database URI", "DATABASE_PATH", "file:phajay.db?mode=memory"},
		{"root database path", "DATABASE_PATH", "/"},
		{"invalid local webhook flag", "ALLOW_LOCAL_WEBHOOKS", "sometimes"},
		{"production local webhooks", "ALLOW_LOCAL_WEBHOOKS", "true"},
		{"invalid proxy", "TRUSTED_PROXIES", "proxy.example.com"},
		{"wildcard proxy", "TRUSTED_PROXIES", "*"},
		{"all IPv4 addresses", "TRUSTED_PROXIES", "0.0.0.0/0"},
		{"all IPv6 addresses", "TRUSTED_PROXIES", "::/0"},
		{"empty proxy entry", "TRUSTED_PROXIES", "127.0.0.1,"},
		{"malformed session lifetime", "SESSION_TTL", "tomorrow"},
		{"negative session lifetime", "SESSION_TTL", "-1h"},
		{"excessive session lifetime", "SESSION_TTL", "1000h"},
		{"short payment lifetime", "PAYMENT_TTL", "1s"},
		{"excessive payment lifetime", "PAYMENT_TTL", "25h"},
		{"zero webhook timeout", "WEBHOOK_TIMEOUT", "0s"},
		{"excessive webhook timeout", "WEBHOOK_TIMEOUT", "31s"},
		{"zero retry attempts", "WEBHOOK_MAX_ATTEMPTS", "0"},
		{"excessive retry attempts", "WEBHOOK_MAX_ATTEMPTS", "13"},
		{"malformed retry attempts", "WEBHOOK_MAX_ATTEMPTS", "many"},
		{"zero shutdown timeout", "SHUTDOWN_TIMEOUT", "0s"},
		{"excessive shutdown timeout", "SHUTDOWN_TIMEOUT", "61s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			values := validEnvironment()
			values[tt.key] = tt.value
			_, err := load(environment(values))
			if err == nil {
				t.Fatalf("accepted invalid %s", tt.key)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("error should identify %s: %v", tt.key, err)
			}
			if tt.key == "MASTER_KEY" && tt.value != "" && strings.Contains(err.Error(), tt.value) {
				t.Fatal("configuration error disclosed the encryption secret")
			}
		})
	}
}

func TestLoadUsesEnvironment(t *testing.T) {
	for _, name := range []string{"MASTER_KEY", "APP_ENV", "HTTP_ADDR", "PUBLIC_URL", "DATABASE_PATH", "WEB_DIST_DIR", "MIGRATIONS_DIR", "ALLOW_LOCAL_WEBHOOKS", "TRUSTED_PROXIES", "SESSION_TTL", "PAYMENT_TTL", "WEBHOOK_TIMEOUT", "WEBHOOK_MAX_ATTEMPTS", "SHUTDOWN_TIMEOUT"} {
		t.Setenv(name, "")
	}
	t.Setenv("MASTER_KEY", validEnvironment()["MASTER_KEY"])
	t.Setenv("PUBLIC_URL", "https://sandbox.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://sandbox.example.com" || !cfg.SecureCookies {
		t.Fatal("Load did not use the process environment")
	}
}

func TestLoadNormalizesBrowserOrigin(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"https://SANDBOX.EXAMPLE.COM:443/", "https://sandbox.example.com"},
		{"http://LOCALHOST:80", "http://localhost"},
		{"https://SANDBOX.EXAMPLE.COM:8443", "https://sandbox.example.com:8443"},
		{"https://sandbox.example.com:00443", "https://sandbox.example.com"},
		{"http://localhost:08080", "http://localhost:8080"},
		{"https://[2001:DB8::1]:443", "https://[2001:db8::1]"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			values := validEnvironment()
			values["PUBLIC_URL"] = tt.input
			cfg, err := load(environment(values))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PublicURL != tt.want {
				t.Fatalf("PUBLIC_URL = %q, want browser origin %q", cfg.PublicURL, tt.want)
			}
		})
	}
}
