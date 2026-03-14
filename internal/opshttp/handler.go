package opshttp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yourname/XOpsAgent/internal/a2a"
	"github.com/yourname/XOpsAgent/internal/mcp"
	"github.com/yourname/XOpsAgent/internal/opsagent"
)

// NewHandler returns an HTTP handler that exposes the OPS agent API, A2A routes, and MCP endpoint.
func NewHandler(service *opsagent.Service, mcpServer *mcp.Server, agentID string) http.Handler {
	mux := http.NewServeMux()
	a2aServer := a2a.NewService(agentID, "ops", "x-automation-agent")

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, service.Health())
	})

	mux.HandleFunc("POST /api/v1/cases", func(w http.ResponseWriter, r *http.Request) {
		var req opsagent.CreateCaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, service.CreateCase(req))
	})

	mux.HandleFunc("GET /api/v1/cases/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, ok := service.GetCase(strings.TrimSpace(r.PathValue("id")))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "case not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/analyze", func(w http.ResponseWriter, r *http.Request) {
		var req opsagent.AnalyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := service.AnalyzeIncident(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("POST /api/v1/plan", func(w http.ResponseWriter, r *http.Request) {
		var req opsagent.PlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := service.GeneratePlan(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("POST /api/v1/agent/run", func(w http.ResponseWriter, r *http.Request) {
		var req opsagent.AgentRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := service.RunAgent(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.Handle("/a2a/v1/", a2aServer.Handler())
	mux.Handle("/mcp", mcpServer)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
