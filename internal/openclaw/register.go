package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yourname/XOpsAgent/internal/config"
)

// RegistrationResult captures the gateway-side result of a registration run.
type RegistrationResult struct {
	Operation string `json:"operation"`
	AgentID   string `json:"agent_id"`
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
}

// RegisterOrUpdateAgent creates or updates an OpenClaw gateway agent entry.
func RegisterOrUpdateAgent(ctx context.Context, cfg config.Config) (RegistrationResult, error) {
	desiredName := strings.TrimSpace(cfg.Ops.Gateway.AgentName)
	agentID := normalizeAgentID(cfg.Ops.Gateway.AgentID)
	if desiredName == "" {
		desiredName = agentID
	}
	if agentID == "" {
		return RegistrationResult{}, fmt.Errorf("openclaw agent id is not configured")
	}

	listResult, err := runGatewayCall(ctx, cfg, "agents.list", map[string]any{})
	if err != nil {
		return RegistrationResult{}, err
	}

	var listPayload struct {
		Agents []struct {
			ID string `json:"id"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(listResult, &listPayload); err != nil {
		return RegistrationResult{}, fmt.Errorf("parse agents.list response: %w", err)
	}

	exists := false
	for _, entry := range listPayload.Agents {
		if normalizeAgentID(entry.ID) == agentID {
			exists = true
			break
		}
	}

	workspace := strings.TrimSpace(cfg.Ops.Gateway.Workspace)
	if workspace == "" {
		workspace = cfg.Ops.WorkingDir
	}
	if exists {
		if _, err := runGatewayCall(ctx, cfg, "agents.update", map[string]any{
			"agentId":   agentID,
			"name":      desiredName,
			"workspace": workspace,
			"model":     strings.TrimSpace(cfg.Ops.Gateway.Model),
		}); err != nil {
			return RegistrationResult{}, err
		}
		return RegistrationResult{
			Operation: "updated",
			AgentID:   agentID,
			Workspace: workspace,
			Model:     strings.TrimSpace(cfg.Ops.Gateway.Model),
		}, nil
	}

	createName := registrationCreateName(agentID, desiredName)
	createResult, err := runGatewayCall(ctx, cfg, "agents.create", map[string]any{
		"name":      createName,
		"workspace": workspace,
	})
	if err != nil {
		return RegistrationResult{}, err
	}

	var createPayload struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(createResult, &createPayload); err != nil {
		return RegistrationResult{}, fmt.Errorf("parse agents.create response: %w", err)
	}
	createdID := normalizeAgentID(createPayload.AgentID)
	if createdID == "" {
		createdID = normalizeAgentID(createName)
	}
	if createdID != agentID {
		return RegistrationResult{}, fmt.Errorf(
			"openclaw created unexpected agent id %q (expected %q); check OPENCLAW_AGENT_ID/OPENCLAW_AGENT_NAME",
			createdID,
			agentID,
		)
	}

	if model := strings.TrimSpace(cfg.Ops.Gateway.Model); model != "" || desiredName != createName {
		if _, err := runGatewayCall(ctx, cfg, "agents.update", map[string]any{
			"agentId":   agentID,
			"name":      desiredName,
			"workspace": workspace,
			"model":     model,
		}); err != nil {
			return RegistrationResult{}, err
		}
	}
	return RegistrationResult{
		Operation: "created",
		AgentID:   agentID,
		Workspace: workspace,
		Model:     strings.TrimSpace(cfg.Ops.Gateway.Model),
	}, nil
}

func runGatewayCall(ctx context.Context, cfg config.Config, method string, params map[string]any) ([]byte, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, cfg.Ops.Gateway.Command, "gateway", "call", method, "--json", "--params", string(body))
	cmd.Env = gatewayEnv(cfg)
	cmd.Dir = cfg.Ops.WorkingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("openclaw gateway call %s failed: %w (%s)", method, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func gatewayEnv(cfg config.Config) []string {
	env := os.Environ()
	if value := strings.TrimSpace(cfg.Ops.Gateway.URL); value != "" {
		env = append(env, "OPENCLAW_GATEWAY_URL="+value)
	}
	if value := strings.TrimSpace(cfg.Ops.Gateway.Token); value != "" {
		env = append(env, "OPENCLAW_GATEWAY_TOKEN="+value)
	}
	if value := strings.TrimSpace(cfg.Ops.Gateway.Password); value != "" {
		env = append(env, "OPENCLAW_GATEWAY_PASSWORD="+value)
	}
	return env
}

func normalizeAgentID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "/", "-", ":", "-", ".", "-").Replace(value)
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	})
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "-")
}

func registrationCreateName(agentID, desiredName string) string {
	if normalizeAgentID(desiredName) == agentID {
		return desiredName
	}
	return agentID
}
