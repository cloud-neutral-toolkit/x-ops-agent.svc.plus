package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port         int
	DatabaseURL  string
	OtlpEndpoint string
	NatsURL      string
	KafkaBrokers []string
	Ops          OpsConfig
}

// OpsConfig contains the runtime settings for the OPS agent and MCP server.
type OpsConfig struct {
	HTTPAddr      string
	MCPServerName string
	WorkingDir    string
	EnvFile       string
	AIBaseURL     string
	AIAPIKey      string
	Codex         CodexConfig
	Gateway       GatewayConfig
}

// CodexConfig configures the local Codex CLI fallback.
type CodexConfig struct {
	Enabled bool
	Command string
	Model   string
	Sandbox string
	WorkDir string
	RepoDir string
}

// GatewayConfig configures OpenClaw gateway access and registration.
type GatewayConfig struct {
	Command         string
	URL             string
	Token           string
	Password        string
	AgentID         string
	AgentName       string
	Workspace       string
	Model           string
	RegisterOnStart bool
	Timeout         time.Duration
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return load(nil, "")
}

// LoadWithEnvFile reads configuration from environment variables and an optional env file.
// Process environment variables win over values defined in the file.
func LoadWithEnvFile(path string) Config {
	if strings.TrimSpace(path) == "" {
		return Load()
	}
	values, err := parseEnvFile(path)
	if err != nil {
		return load(nil, "")
	}
	return load(values, path)
}

// DetectEnvFile returns the configured env file or a default one when it exists.
func DetectEnvFile(defaultName string) string {
	if path := strings.TrimSpace(os.Getenv("XOPS_ENV_FILE")); path != "" {
		return path
	}
	if strings.TrimSpace(defaultName) == "" {
		return ""
	}
	if _, err := os.Stat(defaultName); err == nil {
		return defaultName
	}
	return ""
}

func load(fileValues map[string]string, envFile string) Config {
	port, _ := strconv.Atoi(getenv("PORT", "8080"))
	if port <= 0 {
		port = 8080
	}
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}
	httpAddr := getenvFrom(fileValues, ":"+strconv.Itoa(port), "OPS_HTTP_ADDR", "LISTEN_ADDR")
	timeoutSeconds := getenvIntFrom(fileValues, 90, "OPENCLAW_GATEWAY_TIMEOUT_SECONDS")
	return Config{
		Port:         port,
		DatabaseURL:  getenvFrom(fileValues, "postgres://postgres:postgres@localhost:5432/ops?sslmode=disable", "DATABASE_URL", "PG_URL"),
		OtlpEndpoint: getenvFrom(fileValues, "otelcol:4317", "OTEL_EXPORTER_OTLP_ENDPOINT"),
		NatsURL:      getenvFrom(fileValues, "nats://localhost:4222", "NATS_URL"),
		KafkaBrokers: splitCommaList(getenvFrom(fileValues, "localhost:9092", "KAFKA_BROKERS")),
		Ops: OpsConfig{
			HTTPAddr:      httpAddr,
			MCPServerName: getenvFrom(fileValues, "xops-mcp", "OPS_MCP_SERVER_NAME"),
			WorkingDir:    getenvFrom(fileValues, workingDir, "OPS_WORKING_DIR"),
			EnvFile:       envFile,
			AIBaseURL:     getenvFrom(fileValues, "", "AI_GATEWAY_URL"),
			AIAPIKey:      getenvFrom(fileValues, "", "AI_GATEWAY_API_KEY"),
			Codex: CodexConfig{
				Enabled: getenvBoolFrom(fileValues, true, "OPS_CODEX_ENABLED"),
				Command: getenvFrom(fileValues, "codex", "CODEX_CLI_PATH", "OPS_CODEX_COMMAND"),
				Model:   getenvFrom(fileValues, "gpt-5.2-codex", "CODEX_MODEL", "OPS_CODEX_MODEL"),
				Sandbox: getenvFrom(fileValues, "read-only", "CODEX_SANDBOX", "OPS_CODEX_SANDBOX"),
				WorkDir: getenvFrom(fileValues, workingDir, "CODEX_WORKDIR", "OPS_CODEX_WORKDIR"),
				RepoDir: getenvFrom(fileValues, filepath.Join(workingDir, "third_party", "codex"), "CODEX_REPO_DIR", "OPS_CODEX_REPO_DIR"),
			},
			Gateway: GatewayConfig{
				Command:         getenvFrom(fileValues, "openclaw", "OPENCLAW_COMMAND", "OPS_GATEWAY_COMMAND"),
				URL:             getenvFrom(fileValues, "", "OPENCLAW_GATEWAY_URL"),
				Token:           getenvFrom(fileValues, "", "OPENCLAW_GATEWAY_TOKEN"),
				Password:        getenvFrom(fileValues, "", "OPENCLAW_GATEWAY_PASSWORD"),
				AgentID:         getenvFrom(fileValues, "xops-agent", "OPENCLAW_AGENT_ID", "OPS_GATEWAY_AGENT_ID"),
				AgentName:       getenvFrom(fileValues, "XOpsAgent", "OPENCLAW_AGENT_NAME", "OPS_AGENT_NAME"),
				Workspace:       getenvFrom(fileValues, workingDir, "OPENCLAW_AGENT_WORKSPACE", "OPS_AGENT_WORKSPACE"),
				Model:           getenvFrom(fileValues, "gpt-5.2-codex", "OPENCLAW_AGENT_MODEL", "OPS_GATEWAY_AGENT_MODEL"),
				RegisterOnStart: getenvBoolFrom(fileValues, false, "OPENCLAW_REGISTER_ON_START"),
				Timeout:         time.Duration(timeoutSeconds) * time.Second,
			},
		},
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvFrom(fileValues map[string]string, def string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		if fileValues != nil {
			if v := strings.TrimSpace(fileValues[key]); v != "" {
				return v
			}
		}
	}
	return def
}

func getenvBoolFrom(fileValues map[string]string, def bool, keys ...string) bool {
	value := getenvFrom(fileValues, "", keys...)
	if value == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func getenvIntFrom(fileValues map[string]string, def int, keys ...string) int {
	value := getenvFrom(fileValues, "", keys...)
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return def
	}
	return parsed
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{"localhost:9092"}
	}
	return out
}

func parseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := splitEnvLine(line)
		if !ok {
			continue
		}
		normalizedKey := normalizeEnvKey(key)
		if normalizedKey == "" {
			continue
		}
		values[normalizedKey] = normalizeEnvValue(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func splitEnvLine(line string) (string, string, bool) {
	if idx := strings.IndexRune(line, '='); idx >= 0 {
		return line[:idx], line[idx+1:], true
	}
	if idx := strings.IndexRune(line, ':'); idx >= 0 {
		return line[:idx], line[idx+1:], true
	}
	return "", "", false
}

func normalizeEnvKey(key string) string {
	key = strings.TrimSpace(strings.Trim(key, `"'`))
	if key == "" {
		return ""
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", "-"), " ", ""))
	switch normalized {
	case "remote":
		return "OPENCLAW_GATEWAY_URL"
	case "remote-token":
		return "OPENCLAW_GATEWAY_TOKEN"
	case "remote-password":
		return "OPENCLAW_GATEWAY_PASSWORD"
	case "ai-gateway-url":
		return "AI_GATEWAY_URL"
	case "ai-gateway-apikey", "ai-gateway-api-key":
		return "AI_GATEWAY_API_KEY"
	default:
		return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
	}
}

func normalizeEnvValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"'`)
}
