package a2a

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Request struct {
	FromAgentID string         `json:"from_agent_id"`
	ToAgentID   string         `json:"to_agent_id"`
	RequestID   string         `json:"request_id"`
	Intent      string         `json:"intent"`
	Goal        string         `json:"goal"`
	Context     map[string]any `json:"context,omitempty"`
	Artifacts   map[string]any `json:"artifacts,omitempty"`
	Constraints []string       `json:"constraints,omitempty"`
}

type Response struct {
	Status         string         `json:"status"`
	OwnerAgentID   string         `json:"owner_agent_id"`
	Summary        string         `json:"summary"`
	RequiredInputs []string       `json:"required_inputs,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
}

type TaskRecord struct {
	TaskID       string         `json:"task_id"`
	RequestID    string         `json:"request_id"`
	FromAgentID  string         `json:"from_agent_id"`
	ToAgentID    string         `json:"to_agent_id"`
	Intent       string         `json:"intent"`
	Goal         string         `json:"goal"`
	Status       string         `json:"status"`
	OwnerAgentID string         `json:"owner_agent_id"`
	Summary      string         `json:"summary"`
	Result       map[string]any `json:"result,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Service struct {
	agentID string
	role    string
	mu      sync.RWMutex
	tasks   map[string]TaskRecord
}

func NewService(agentID, role, defaultHandoff string) *Service {
	if strings.TrimSpace(agentID) == "" {
		agentID = "xops-agent"
	}
	if strings.TrimSpace(defaultHandoff) == "" {
		defaultHandoff = "x-automation-agent"
	}
	return &Service{
		agentID: strings.TrimSpace(agentID),
		role:    strings.TrimSpace(role),
		tasks:   make(map[string]TaskRecord),
	}
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /a2a/v1/negotiate", s.handleNegotiate)
	mux.HandleFunc("POST /a2a/v1/tasks", s.handleTaskCreate)
	mux.HandleFunc("GET /a2a/v1/tasks/{task_id}", s.handleTaskGet)
	return mux
}

func (s *Service) handleNegotiate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	resp := s.Negotiate(req)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	record := s.CreateTask(req)
	writeJSON(w, http.StatusAccepted, record)
}

func (s *Service) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	record, ok := s.GetTask(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Service) Negotiate(req Request) Response {
	text := strings.ToLower(strings.Join([]string{req.Intent, req.Goal, stringify(req.Context)}, " "))
	status := "accepted"
	summary := "xops-agent accepts the incident investigation handoff."
	result := map[string]any{
		"role":         s.role,
		"decision":     "accepted",
		"next_action":  "analyze and produce remediation guidance",
		"request_id":   req.RequestID,
		"target_agent": s.agentID,
	}

	if containsAny(text, []string{"terraform", "pulumi", "dns", "playbook", "iac", "automation"}) {
		status = "declined"
		summary = "xops-agent declines infrastructure automation execution and recommends x-automation-agent."
		result["decision"] = "handoff"
		result["handoff_agent_id"] = "x-automation-agent"
	}

	if containsAny(text, []string{"logs", "metrics", "traces", "topology", "alert", "observability"}) {
		status = "needs_input"
		summary = "xops-agent needs evidence from x-observability-agent before finalizing the incident judgment."
		result["decision"] = "consult"
		result["handoff_agent_id"] = "x-observability-agent"
	}

	log.Printf("a2a negotiate request_id=%s from=%s to=%s status=%s", req.RequestID, req.FromAgentID, s.agentID, status)
	return Response{
		Status:       status,
		OwnerAgentID: s.agentID,
		Summary:      summary,
		Result:       result,
	}
}

func (s *Service) CreateTask(req Request) TaskRecord {
	taskID := newTaskID()
	negotiated := s.Negotiate(req)
	record := TaskRecord{
		TaskID:       taskID,
		RequestID:    req.RequestID,
		FromAgentID:  req.FromAgentID,
		ToAgentID:    fallback(req.ToAgentID, s.agentID),
		Intent:       req.Intent,
		Goal:         req.Goal,
		Status:       negotiated.Status,
		OwnerAgentID: s.agentID,
		Summary:      negotiated.Summary,
		Result:       negotiated.Result,
		CreatedAt:    time.Now().UTC(),
	}
	if record.Status == "accepted" {
		record.Status = "completed"
		record.Summary = "xops-agent completed incident negotiation and produced an ops-side recommendation."
		record.Result["deliverable"] = "incident_analysis"
	}

	s.mu.Lock()
	s.tasks[taskID] = record
	s.mu.Unlock()
	log.Printf("a2a task request_id=%s task_id=%s from=%s to=%s status=%s", req.RequestID, taskID, req.FromAgentID, s.agentID, record.Status)
	return record
}

func (s *Service) GetTask(taskID string) (TaskRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.tasks[taskID]
	return record, ok
}

func decodeRequest(w http.ResponseWriter, r *http.Request) (Request, bool) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return Request{}, false
	}
	req.FromAgentID = strings.TrimSpace(req.FromAgentID)
	req.ToAgentID = strings.TrimSpace(req.ToAgentID)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.Intent = strings.TrimSpace(req.Intent)
	req.Goal = strings.TrimSpace(req.Goal)
	if req.RequestID == "" {
		req.RequestID = taskSeed()
	}
	return req, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func containsAny(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, candidate) {
			return true
		}
	}
	return false
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	blob, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(blob)
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(def)
}

func newTaskID() string {
	return "a2a-" + taskSeed()
}

func taskSeed() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return hex.EncodeToString(buf[:])
}
