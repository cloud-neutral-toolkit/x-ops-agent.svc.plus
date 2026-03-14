package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/yourname/XOpsAgent/internal/opsagent"
)

// Server implements a minimal MCP-compatible tools server over stdio and HTTP JSON-RPC.
type Server struct {
	service *opsagent.Service
	name    string
	version string
}

// NewServer returns a server backed by the shared OPS service.
func NewServer(service *opsagent.Service, name string) *Server {
	if strings.TrimSpace(name) == "" {
		name = "xops-mcp"
	}
	return &Server{
		service: service,
		name:    name,
		version: "0.1.0",
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Run serves MCP stdio using Content-Length framing.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		body, err := readFrame(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		response, err := s.HandleJSONRPC(ctx, body)
		if err != nil {
			return err
		}
		if len(response) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(response)); err != nil {
			return err
		}
		if _, err := out.Write(response); err != nil {
			return err
		}
	}
}

// ServeHTTP exposes a simple JSON-RPC endpoint for local MCP debugging.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := s.HandleJSONRPC(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(response) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(response)
}

// HandleJSONRPC processes a single JSON-RPC request body.
func (s *Server) HandleJSONRPC(ctx context.Context, body []byte) ([]byte, error) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
	}

	if req.Method == "notifications/initialized" || req.Method == "$/cancelRequest" {
		return nil, nil
	}

	id := rawID(req.ID)
	switch req.Method {
	case "initialize":
		return json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    s.name,
					"version": s.version,
				},
			},
		})
	case "ping":
		return json.Marshal(rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{}})
	case "tools/list":
		return json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]any{
				"tools": s.tools(),
			},
		})
	case "tools/call":
		var params toolsCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return json.Marshal(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid tool call params"}})
		}
		result, err := s.callTool(ctx, params.Name, params.Arguments)
		if err != nil {
			return json.Marshal(rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result: map[string]any{
					"content": []map[string]string{{"type": "text", "text": err.Error()}},
					"isError": true,
				},
			})
		}
		return json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]any{
				"content": []map[string]string{{"type": "text", "text": result}},
			},
		})
	default:
		return json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcError{Code: -32601, Message: "method not found"},
		})
	}
}

func (s *Server) tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "ops_health",
			"description": "Return the current health and backend readiness for the OPS agent.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "ops_create_case",
			"description": "Create a new incident case in the local OPS store.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}}, "required": []string{"title"}},
		},
		{
			"name":        "ops_get_case",
			"description": "Fetch a case by its id.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}},
		},
		{
			"name":        "ops_analyze_incident",
			"description": "Analyze metrics and traces and return a structured diagnosis.",
			"inputSchema": map[string]any{"type": "object"},
		},
		{
			"name":        "ops_generate_plan",
			"description": "Generate a remediation plan from an analysis or raw telemetry.",
			"inputSchema": map[string]any{"type": "object"},
		},
		{
			"name":        "ops_run_agent",
			"description": "Run the intelligent OPS agent via OpenClaw or Codex fallback.",
			"inputSchema": map[string]any{"type": "object"},
		},
	}
}

func (s *Server) callTool(ctx context.Context, name string, rawArgs json.RawMessage) (string, error) {
	switch name {
	case "ops_health":
		body, _ := json.MarshalIndent(s.service.Health(), "", "  ")
		return string(body), nil
	case "ops_create_case":
		var req opsagent.CreateCaseRequest
		if err := json.Unmarshal(rawArgs, &req); err != nil {
			return "", err
		}
		body, _ := json.MarshalIndent(s.service.CreateCase(req), "", "  ")
		return string(body), nil
	case "ops_get_case":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rawArgs, &req); err != nil {
			return "", err
		}
		item, ok := s.service.GetCase(strings.TrimSpace(req.ID))
		if !ok {
			return "", fmt.Errorf("case %q not found", req.ID)
		}
		body, _ := json.MarshalIndent(item, "", "  ")
		return string(body), nil
	case "ops_analyze_incident":
		var req opsagent.AnalyzeRequest
		if err := json.Unmarshal(rawArgs, &req); err != nil {
			return "", err
		}
		item, err := s.service.AnalyzeIncident(ctx, req)
		if err != nil {
			return "", err
		}
		body, _ := json.MarshalIndent(item, "", "  ")
		return string(body), nil
	case "ops_generate_plan":
		var req opsagent.PlanRequest
		if err := json.Unmarshal(rawArgs, &req); err != nil {
			return "", err
		}
		item, err := s.service.GeneratePlan(ctx, req)
		if err != nil {
			return "", err
		}
		body, _ := json.MarshalIndent(item, "", "  ")
		return string(body), nil
	case "ops_run_agent":
		var req opsagent.AgentRunRequest
		if err := json.Unmarshal(rawArgs, &req); err != nil {
			return "", err
		}
		item, err := s.service.RunAgent(ctx, req)
		if err != nil {
			return "", err
		}
		body, _ := json.MarshalIndent(item, "", "  ")
		return string(body), nil
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			value = strings.TrimSpace(strings.TrimPrefix(value, "content-length:"))
			contentLength, err = strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
		}
	}
	if contentLength <= 0 {
		return nil, io.EOF
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}

func rawID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(id, &out); err != nil {
		return string(id)
	}
	return out
}
