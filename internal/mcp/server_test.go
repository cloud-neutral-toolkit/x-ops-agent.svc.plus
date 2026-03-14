package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/yourname/XOpsAgent/internal/config"
	"github.com/yourname/XOpsAgent/internal/opsagent"
)

func TestToolsListAndCall(t *testing.T) {
	service := opsagent.New(config.Config{
		Ops: config.OpsConfig{
			Codex: config.CodexConfig{Enabled: false},
		},
	})
	server := NewServer(service, "xops-test")

	listResponse, err := server.HandleJSONRPC(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("tools/list returned error: %v", err)
	}
	if !bytes.Contains(listResponse, []byte(`"ops_health"`)) {
		t.Fatalf("expected ops_health tool in response: %s", string(listResponse))
	}

	callBody := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ops_health","arguments":{}}}`)
	callResponse, err := server.HandleJSONRPC(context.Background(), callBody)
	if err != nil {
		t.Fatalf("tools/call returned error: %v", err)
	}

	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResponse, &response); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	if len(response.Result.Content) == 0 || response.Result.Content[0].Text == "" {
		t.Fatalf("expected text content in tools/call response: %s", string(callResponse))
	}
}
