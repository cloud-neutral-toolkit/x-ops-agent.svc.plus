package config

import (
	"os"
	"reflect"
	"testing"
)

func unsetEnv(keys ...string) func() {
	prev := make(map[string]string)
	for _, k := range keys {
		prev[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for k, v := range prev {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	restore := unsetEnv(
		"PORT",
		"DATABASE_URL",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"NATS_URL",
		"KAFKA_BROKERS",
		"OPS_HTTP_ADDR",
		"OPENCLAW_GATEWAY_URL",
		"OPENCLAW_GATEWAY_TOKEN",
	)
	defer restore()

	cfg := Load()
	if cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://postgres:postgres@localhost:5432/ops?sslmode=disable" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.OtlpEndpoint != "otelcol:4317" {
		t.Fatalf("unexpected OTLP endpoint: %s", cfg.OtlpEndpoint)
	}
	if cfg.NatsURL != "nats://localhost:4222" {
		t.Fatalf("unexpected nats url: %s", cfg.NatsURL)
	}
	expBrokers := []string{"localhost:9092"}
	if !reflect.DeepEqual(cfg.KafkaBrokers, expBrokers) {
		t.Fatalf("unexpected brokers: %#v", cfg.KafkaBrokers)
	}
	if cfg.Ops.HTTPAddr != ":8080" {
		t.Fatalf("expected default ops addr :8080, got %s", cfg.Ops.HTTPAddr)
	}
	if !cfg.Ops.Codex.Enabled {
		t.Fatalf("expected codex fallback enabled by default")
	}
}

func TestLoadOverrides(t *testing.T) {
	restore := unsetEnv(
		"PORT",
		"DATABASE_URL",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"NATS_URL",
		"KAFKA_BROKERS",
		"OPS_HTTP_ADDR",
		"OPENCLAW_GATEWAY_URL",
		"OPENCLAW_GATEWAY_TOKEN",
		"AI_GATEWAY_URL",
		"AI_GATEWAY_API_KEY",
	)
	defer restore()

	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/app")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel:55680")
	os.Setenv("NATS_URL", "nats://nats:4222")
	os.Setenv("KAFKA_BROKERS", "k1:9092,k2:9093")
	os.Setenv("OPS_HTTP_ADDR", ":18080")
	os.Setenv("OPENCLAW_GATEWAY_URL", "wss://gateway.example/ws")
	os.Setenv("OPENCLAW_GATEWAY_TOKEN", "secret")
	os.Setenv("AI_GATEWAY_URL", "https://api.example/v1")
	os.Setenv("AI_GATEWAY_API_KEY", "api-key")

	cfg := Load()
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/app" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.OtlpEndpoint != "otel:55680" {
		t.Fatalf("unexpected OTLP endpoint: %s", cfg.OtlpEndpoint)
	}
	if cfg.NatsURL != "nats://nats:4222" {
		t.Fatalf("unexpected nats url: %s", cfg.NatsURL)
	}
	expBrokers := []string{"k1:9092", "k2:9093"}
	if !reflect.DeepEqual(cfg.KafkaBrokers, expBrokers) {
		t.Fatalf("unexpected brokers: %#v", cfg.KafkaBrokers)
	}
	if cfg.Ops.HTTPAddr != ":18080" {
		t.Fatalf("unexpected ops http addr: %s", cfg.Ops.HTTPAddr)
	}
	if cfg.Ops.Gateway.URL != "wss://gateway.example/ws" {
		t.Fatalf("unexpected gateway url: %s", cfg.Ops.Gateway.URL)
	}
	if cfg.Ops.AIBaseURL != "https://api.example/v1" {
		t.Fatalf("unexpected ai gateway url: %s", cfg.Ops.AIBaseURL)
	}
	if cfg.Ops.AIAPIKey != "api-key" {
		t.Fatalf("unexpected ai gateway key: %s", cfg.Ops.AIAPIKey)
	}
}

func TestLoadWithEnvFileSupportsLegacyAliases(t *testing.T) {
	restore := unsetEnv("OPENCLAW_GATEWAY_URL", "OPENCLAW_GATEWAY_TOKEN", "AI_GATEWAY_URL", "AI_GATEWAY_API_KEY")
	defer restore()

	file, err := os.CreateTemp(t.TempDir(), "xops-env-*.env")
	if err != nil {
		t.Fatalf("create temp env: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(`
remote: wss://openclaw.example/ws
remote-token: token-123
"AI-Gateway-Url": "https://api.svc.plus/v1",
"AI-Gateway-apiKey": "gateway-key",
`); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cfg := LoadWithEnvFile(file.Name())
	if cfg.Ops.Gateway.URL != "wss://openclaw.example/ws" {
		t.Fatalf("unexpected gateway url from env file: %s", cfg.Ops.Gateway.URL)
	}
	if cfg.Ops.Gateway.Token != "token-123" {
		t.Fatalf("unexpected gateway token from env file: %s", cfg.Ops.Gateway.Token)
	}
	if cfg.Ops.AIBaseURL != "https://api.svc.plus/v1" {
		t.Fatalf("unexpected ai gateway url from env file: %s", cfg.Ops.AIBaseURL)
	}
	if cfg.Ops.AIAPIKey != "gateway-key" {
		t.Fatalf("unexpected ai gateway key from env file: %s", cfg.Ops.AIAPIKey)
	}
}
