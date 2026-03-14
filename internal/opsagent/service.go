package opsagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourname/XOpsAgent/analyzer"
	"github.com/yourname/XOpsAgent/internal/config"
	"github.com/yourname/XOpsAgent/internal/model"
	"github.com/yourname/XOpsAgent/utils"
)

// Case represents the persisted state for an incident handled by the OPS agent.
type Case struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Severity  string          `json:"severity"`
	Summary   string          `json:"summary,omitempty"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Analysis  *AnalysisResult `json:"analysis,omitempty"`
	Plan      *PlanResult     `json:"plan,omitempty"`
}

// CreateCaseRequest defines the input for creating a case.
type CreateCaseRequest struct {
	Title    string `json:"title"`
	Severity string `json:"severity,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// AnalyzeRequest defines the input payload for a local incident analysis.
type AnalyzeRequest struct {
	CaseID   string         `json:"case_id,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Services []string       `json:"services,omitempty"`
	Metrics  []model.Metric `json:"metrics,omitempty"`
	Traces   []model.Trace  `json:"traces,omitempty"`
}

// AnalysisResult is the OPS agent's structured diagnosis output.
type AnalysisResult struct {
	SuspectServices  []string        `json:"suspect_services,omitempty"`
	TimeRange        utils.TimeRange `json:"time_range"`
	RootCause        model.RootCause `json:"root_cause"`
	Findings         []string        `json:"findings,omitempty"`
	SuggestedActions []string        `json:"suggested_actions,omitempty"`
	SupportingTrace  *TraceSummary   `json:"supporting_trace,omitempty"`
}

// TraceSummary contains the trace/span selected as the strongest signal.
type TraceSummary struct {
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	Operation string `json:"operation"`
	Pod       string `json:"pod,omitempty"`
	Node      string `json:"node,omitempty"`
}

// PlanRequest defines the input for remediation plan generation.
type PlanRequest struct {
	CaseID   string          `json:"case_id,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Analysis *AnalysisResult `json:"analysis,omitempty"`
	Metrics  []model.Metric  `json:"metrics,omitempty"`
	Traces   []model.Trace   `json:"traces,omitempty"`
}

// PlanResult is the structured remediation plan.
type PlanResult struct {
	Summary   string   `json:"summary"`
	Steps     []string `json:"steps"`
	Rollback  []string `json:"rollback"`
	RiskLevel string   `json:"risk_level"`
}

// AgentRunRequest defines the intelligent-agent execution request.
type AgentRunRequest struct {
	CaseID  string         `json:"case_id,omitempty"`
	Prompt  string         `json:"prompt,omitempty"`
	Summary string         `json:"summary,omitempty"`
	Metrics []model.Metric `json:"metrics,omitempty"`
	Traces  []model.Trace  `json:"traces,omitempty"`
}

// AgentRunResult captures the backend used and the generated response.
type AgentRunResult struct {
	Backend  string   `json:"backend"`
	Response string   `json:"response"`
	Warnings []string `json:"warnings,omitempty"`
}

// HealthResult describes runtime readiness for the OPS agent.
type HealthResult struct {
	Status            string `json:"status"`
	GatewayConfigured bool   `json:"gateway_configured"`
	CodexConfigured   bool   `json:"codex_configured"`
}

// CaseStore is the minimal persistence surface required by the service.
type CaseStore interface {
	Create(CreateCaseRequest) Case
	Get(id string) (Case, bool)
	SaveAnalysis(id string, result AnalysisResult) (Case, bool)
	SavePlan(id string, result PlanResult) (Case, bool)
}

// Service exposes the shared business logic used by HTTP and MCP surfaces.
type Service struct {
	cfg     config.Config
	store   CaseStore
	clients []assistantClient
}

// New creates a service backed by an in-memory case store.
func New(cfg config.Config) *Service {
	return NewWithStore(cfg, NewMemoryCaseStore())
}

// NewWithStore creates a service with an injected store.
func NewWithStore(cfg config.Config, store CaseStore) *Service {
	runner := execCommandRunner{}
	clients := make([]assistantClient, 0, 2)
	if strings.TrimSpace(cfg.Ops.Gateway.URL) != "" {
		clients = append(clients, newOpenClawAgentClient(cfg, runner))
	}
	if cfg.Ops.Codex.Enabled {
		clients = append(clients, newCodexCLIClient(cfg, runner))
	}
	return &Service{cfg: cfg, store: store, clients: clients}
}

// Health reports local service readiness.
func (s *Service) Health() HealthResult {
	return HealthResult{
		Status:            "ok",
		GatewayConfigured: strings.TrimSpace(s.cfg.Ops.Gateway.URL) != "",
		CodexConfigured:   s.cfg.Ops.Codex.Enabled,
	}
}

// CreateCase inserts a new incident record in the local store.
func (s *Service) CreateCase(req CreateCaseRequest) Case {
	if strings.TrimSpace(req.Severity) == "" {
		req.Severity = "warning"
	}
	return s.store.Create(req)
}

// GetCase fetches an existing case.
func (s *Service) GetCase(id string) (Case, bool) {
	return s.store.Get(id)
}

// AnalyzeIncident produces a deterministic local diagnosis from metrics and traces.
func (s *Service) AnalyzeIncident(_ context.Context, req AnalyzeRequest) (AnalysisResult, error) {
	services := req.Services
	if len(services) == 0 {
		services = analyzer.AnalyzeAbnormalServices(req.Metrics)
	}
	timeRange := analyzer.DetectTimeRange(req.Metrics, services)
	errorTraces := analyzer.FindErrorTraces(req.Traces)

	rootCause := deriveMetricRootCause(req.Metrics, req.Summary)
	supporting := (*TraceSummary)(nil)
	if len(errorTraces) > 0 {
		trace := errorTraces[0]
		span := analyzer.LocateRootSpan(trace)
		pod, node := analyzer.ResolveLocation(span)
		rootCause = analyzer.InferRootCause(trace, pod, node)
		supporting = &TraceSummary{
			TraceID:   trace.ID,
			SpanID:    span.ID,
			Operation: span.Operation,
			Pod:       pod,
			Node:      node,
		}
	}

	suggestions := analyzer.SuggestAction(rootCause)
	findings := buildFindings(req.Summary, services, req.Metrics, rootCause, supporting)
	result := AnalysisResult{
		SuspectServices:  services,
		TimeRange:        timeRange,
		RootCause:        rootCause,
		Findings:         findings,
		SuggestedActions: suggestions.Suggestions,
		SupportingTrace:  supporting,
	}

	if strings.TrimSpace(req.CaseID) != "" {
		s.store.SaveAnalysis(req.CaseID, result)
	}
	return result, nil
}

// GeneratePlan creates a deterministic remediation plan from an analysis.
func (s *Service) GeneratePlan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	analysis := req.Analysis
	if analysis == nil {
		local, err := s.AnalyzeIncident(ctx, AnalyzeRequest{
			CaseID:  req.CaseID,
			Summary: req.Summary,
			Metrics: req.Metrics,
			Traces:  req.Traces,
		})
		if err != nil {
			return PlanResult{}, err
		}
		analysis = &local
	}

	steps, rollback, risk := buildPlanForCause(analysis.RootCause)
	plan := PlanResult{
		Summary:   buildPlanSummary(req.Summary, analysis.RootCause, risk),
		Steps:     steps,
		Rollback:  rollback,
		RiskLevel: risk,
	}
	if strings.TrimSpace(req.CaseID) != "" {
		s.store.SavePlan(req.CaseID, plan)
	}
	return plan, nil
}

// RunAgent routes the request through OpenClaw first and falls back to Codex/local synthesis.
func (s *Service) RunAgent(ctx context.Context, req AgentRunRequest) (AgentRunResult, error) {
	prompt, fallback, err := s.buildAgentPrompt(ctx, req)
	if err != nil {
		return AgentRunResult{}, err
	}

	warnings := make([]string, 0, len(s.clients))
	for _, client := range s.clients {
		response, err := client.RunPrompt(ctx, prompt)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", client.Name(), err))
			continue
		}
		if strings.TrimSpace(response) == "" {
			warnings = append(warnings, fmt.Sprintf("%s: empty response", client.Name()))
			continue
		}
		return AgentRunResult{
			Backend:  client.Name(),
			Response: response,
			Warnings: warnings,
		}, nil
	}

	return AgentRunResult{
		Backend:  "local",
		Response: fallback,
		Warnings: warnings,
	}, nil
}

func (s *Service) buildAgentPrompt(ctx context.Context, req AgentRunRequest) (string, string, error) {
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		return prompt, localFallbackFromPrompt(prompt), nil
	}

	summary := strings.TrimSpace(req.Summary)
	var currentCase *Case
	if strings.TrimSpace(req.CaseID) != "" {
		if found, ok := s.store.Get(req.CaseID); ok {
			currentCase = &found
			if summary == "" {
				summary = found.Summary
			}
		}
	}

	analysis, err := s.AnalyzeIncident(ctx, AnalyzeRequest{
		CaseID:  req.CaseID,
		Summary: summary,
		Metrics: req.Metrics,
		Traces:  req.Traces,
	})
	if err != nil {
		return "", "", err
	}
	plan, err := s.GeneratePlan(ctx, PlanRequest{
		CaseID:   req.CaseID,
		Summary:  summary,
		Analysis: &analysis,
	})
	if err != nil {
		return "", "", err
	}

	payload := map[string]any{
		"summary":  summary,
		"analysis": analysis,
		"plan":     plan,
	}
	if currentCase != nil {
		payload["case"] = currentCase
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", "", err
	}

	prompt := strings.TrimSpace(strings.Join([]string{
		"You are an OPS incident response agent.",
		"Produce a concise, operator-grade response with:",
		"1. Situation summary",
		"2. Most likely root cause",
		"3. Immediate actions",
		"4. Safe rollback guidance",
		"5. Follow-up checks",
		string(body),
	}, "\n\n"))

	return prompt, renderLocalFallback(summary, analysis, plan), nil
}

func deriveMetricRootCause(metrics []model.Metric, summary string) model.RootCause {
	highestErrorRate := 0.0
	highestLatency := 0.0
	for _, metric := range metrics {
		if metric.ErrorRate > highestErrorRate {
			highestErrorRate = metric.ErrorRate
		}
		if metric.LatencyAvg > highestLatency {
			highestLatency = metric.LatencyAvg
		}
	}
	switch {
	case highestErrorRate >= 0.10:
		return model.RootCause{
			Type:        "Elevated Error Rate",
			Description: "Application errors spiked above the configured operational threshold.",
			Evidence:    []string{fmt.Sprintf("error_rate=%.2f", highestErrorRate)},
		}
	case highestLatency >= 500:
		return model.RootCause{
			Type:        "Latency Saturation",
			Description: "Average latency increased beyond the normal service budget.",
			Evidence:    []string{fmt.Sprintf("latency_avg_ms=%.0f", highestLatency)},
		}
	case strings.TrimSpace(summary) != "":
		return model.RootCause{
			Type:        "Operator Reported Incident",
			Description: summary,
		}
	default:
		return model.RootCause{
			Type:        "Unknown",
			Description: "No strong local signal was available; operator validation is required.",
		}
	}
}

func buildFindings(summary string, services []string, metrics []model.Metric, cause model.RootCause, supporting *TraceSummary) []string {
	findings := make([]string, 0, 4)
	if strings.TrimSpace(summary) != "" {
		findings = append(findings, summary)
	}
	if len(services) > 0 {
		findings = append(findings, fmt.Sprintf("Suspect services: %s", strings.Join(services, ", ")))
	}
	if len(metrics) > 0 {
		findings = append(findings, fmt.Sprintf("Metrics analysed: %d datapoints", len(metrics)))
	}
	findings = append(findings, fmt.Sprintf("Root cause classification: %s", cause.Type))
	if supporting != nil {
		findings = append(findings, fmt.Sprintf("Supporting trace: %s/%s", supporting.TraceID, supporting.Operation))
	}
	return findings
}

func buildPlanForCause(cause model.RootCause) ([]string, []string, string) {
	switch cause.Type {
	case "OOM":
		return []string{
				"Confirm the affected pods restarted with OOMKill events in the incident window.",
				"Increase memory requests and limits, then roll out the workload in a controlled manner.",
				"Check whether a recent deploy or traffic burst changed allocation patterns before closing the incident.",
			},
			[]string{
				"Revert memory-related workload changes if restart stability does not improve.",
				"Roll back the last deploy that introduced the memory regression.",
			},
			"medium"
	case "TCP Reset":
		return []string{
				"Inspect downstream resets and validate whether the connection closes originate upstream or locally.",
				"Review service mesh, ingress, proxy, and keepalive timeout settings.",
				"Roll back the most recent network-policy or proxy change if resets continue.",
			},
			[]string{
				"Restore the last known-good proxy or ingress configuration.",
				"Disable the new routing path until end-to-end connection health is stable.",
			},
			"medium"
	case "Elevated Error Rate":
		return []string{
				"Correlate the error spike with deploys, feature-flag changes, and downstream dependency health.",
				"Sample failed requests and group them by status code or error signature.",
				"Mitigate by disabling the suspicious rollout or feature if the blast radius is still growing.",
			},
			[]string{
				"Roll back the latest release or feature-flag change associated with the spike.",
				"Route traffic away from the failing dependency if a fallback path exists.",
			},
			"high"
	default:
		return []string{
				"Preserve logs, traces, and deployment metadata for the incident window.",
				"Validate the most recent change set and compare it with the last known-good release.",
				"Prepare a low-risk rollback if customer impact remains active while the investigation continues.",
			},
			[]string{
				"Revert the latest rollout if impact is ongoing and root cause remains unclear.",
				"Restore the previous configuration snapshot before retrying any change.",
			},
			"high"
	}
}

func buildPlanSummary(summary string, cause model.RootCause, risk string) string {
	if strings.TrimSpace(summary) == "" {
		return fmt.Sprintf("Generated a %s-risk remediation plan for %s.", risk, cause.Type)
	}
	return fmt.Sprintf("%s Most likely root cause: %s. Risk level: %s.", summary, cause.Type, risk)
}

func renderLocalFallback(summary string, analysis AnalysisResult, plan PlanResult) string {
	lines := []string{
		"Situation summary:",
		valueOrDefault(summary, "No summary provided; using local telemetry signals only."),
		"",
		fmt.Sprintf("Most likely root cause: %s", analysis.RootCause.Type),
		analysis.RootCause.Description,
		"",
		"Immediate actions:",
	}
	for _, step := range plan.Steps {
		lines = append(lines, "- "+step)
	}
	lines = append(lines, "", "Safe rollback guidance:")
	for _, step := range plan.Rollback {
		lines = append(lines, "- "+step)
	}
	lines = append(lines, "", "Follow-up checks:")
	for _, action := range analysis.SuggestedActions {
		lines = append(lines, "- "+action)
	}
	return strings.Join(lines, "\n")
}

func localFallbackFromPrompt(prompt string) string {
	return strings.TrimSpace(strings.Join([]string{
		"Gateway and Codex were unavailable, so this is a local fallback response.",
		"Original prompt:",
		prompt,
	}, "\n\n"))
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// MemoryCaseStore is a concurrency-safe in-memory case store.
type MemoryCaseStore struct {
	mu    sync.RWMutex
	cases map[string]Case
}

// NewMemoryCaseStore returns a ready-to-use in-memory case store.
func NewMemoryCaseStore() *MemoryCaseStore {
	return &MemoryCaseStore{cases: make(map[string]Case)}
}

// Create inserts a new case and returns the stored copy.
func (s *MemoryCaseStore) Create(req CreateCaseRequest) Case {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	item := Case{
		ID:        newID("case"),
		Title:     strings.TrimSpace(req.Title),
		Severity:  strings.ToLower(strings.TrimSpace(req.Severity)),
		Summary:   strings.TrimSpace(req.Summary),
		Status:    "new",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.cases[item.ID] = item
	return item
}

// Get fetches a case by id.
func (s *MemoryCaseStore) Get(id string) (Case, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.cases[id]
	return item, ok
}

// SaveAnalysis attaches an analysis result to an existing case.
func (s *MemoryCaseStore) SaveAnalysis(id string, result AnalysisResult) (Case, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.cases[id]
	if !ok {
		return Case{}, false
	}
	item.Analysis = &result
	item.Status = "analysed"
	item.UpdatedAt = time.Now().UTC()
	s.cases[id] = item
	return item, true
}

// SavePlan attaches a remediation plan to an existing case.
func (s *MemoryCaseStore) SavePlan(id string, result PlanResult) (Case, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.cases[id]
	if !ok {
		return Case{}, false
	}
	item.Plan = &result
	item.Status = "planned"
	item.UpdatedAt = time.Now().UTC()
	s.cases[id] = item
	return item, true
}

func newID(prefix string) string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(raw[:]))
}
