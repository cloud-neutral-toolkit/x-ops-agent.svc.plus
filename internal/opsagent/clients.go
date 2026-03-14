package opsagent

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

type assistantClient interface {
	Name() string
	RunPrompt(ctx context.Context, prompt string) (string, error)
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, []byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type openClawAgentClient struct {
	cfg    config.Config
	runner commandRunner
}

func newOpenClawAgentClient(cfg config.Config, runner commandRunner) assistantClient {
	return &openClawAgentClient{cfg: cfg, runner: runner}
}

func (c *openClawAgentClient) Name() string { return "openclaw-gateway" }

func (c *openClawAgentClient) RunPrompt(ctx context.Context, prompt string) (string, error) {
	agentID := strings.TrimSpace(c.cfg.Ops.Gateway.AgentID)
	if agentID == "" {
		return "", fmt.Errorf("gateway agent id is not configured")
	}

	args := []string{
		"agent",
		"--agent", agentID,
		"--message", prompt,
		"--timeout", fmt.Sprintf("%d", int(c.cfg.Ops.Gateway.Timeout.Seconds())),
		"--json",
	}

	stdout, stderr, err := c.runner.Run(ctx, c.cfg.Ops.Gateway.Command, args, shellEnv(c.cfg), c.cfg.Ops.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("openclaw agent failed: %w (%s)", err, strings.TrimSpace(string(stderr)))
	}

	var payload struct {
		Summary string `json:"summary"`
		Result  struct {
			Payloads []struct {
				Text string `json:"text"`
			} `json:"payloads"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		return "", fmt.Errorf("parse openclaw response: %w", err)
	}

	parts := make([]string, 0, len(payload.Result.Payloads)+1)
	if strings.TrimSpace(payload.Summary) != "" {
		parts = append(parts, strings.TrimSpace(payload.Summary))
	}
	for _, item := range payload.Result.Payloads {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("openclaw response contained no text")
	}
	return strings.Join(parts, "\n\n"), nil
}

type codexCLIClient struct {
	cfg    config.Config
	runner commandRunner
}

func newCodexCLIClient(cfg config.Config, runner commandRunner) assistantClient {
	return &codexCLIClient{cfg: cfg, runner: runner}
}

func (c *codexCLIClient) Name() string { return "codex-cli" }

func (c *codexCLIClient) RunPrompt(ctx context.Context, prompt string) (string, error) {
	lastMessageFile, err := os.CreateTemp("", "xops-codex-last-message-*.txt")
	if err != nil {
		return "", err
	}
	lastMessagePath := lastMessageFile.Name()
	_ = lastMessageFile.Close()
	defer os.Remove(lastMessagePath)

	args := []string{
		"exec",
		"--color", "never",
		"--sandbox", c.cfg.Ops.Codex.Sandbox,
		"--skip-git-repo-check",
		"--output-last-message", lastMessagePath,
	}
	if model := strings.TrimSpace(c.cfg.Ops.Codex.Model); model != "" {
		args = append(args, "--model", model)
	}
	if workDir := strings.TrimSpace(c.cfg.Ops.Codex.WorkDir); workDir != "" {
		args = append(args, "-C", workDir)
	}
	args = append(args, prompt)

	_, stderr, err := c.runner.Run(ctx, c.cfg.Ops.Codex.Command, args, shellEnv(c.cfg), c.cfg.Ops.Codex.WorkDir)
	if err != nil {
		return "", fmt.Errorf("codex exec failed: %w (%s)", err, strings.TrimSpace(string(stderr)))
	}
	body, err := os.ReadFile(lastMessagePath)
	if err != nil {
		return "", err
	}
	response := strings.TrimSpace(string(body))
	if response == "" {
		return "", fmt.Errorf("codex last message file was empty")
	}
	return response, nil
}

func shellEnv(cfg config.Config) []string {
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
	if value := strings.TrimSpace(cfg.Ops.AIBaseURL); value != "" {
		env = append(env, "OPENAI_BASE_URL="+value)
	}
	if value := strings.TrimSpace(cfg.Ops.AIAPIKey); value != "" {
		env = append(env, "OPENAI_API_KEY="+value)
	}
	return env
}
