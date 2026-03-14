package opsagent

import (
	"context"
	"testing"
	"time"

	"github.com/yourname/XOpsAgent/internal/config"
	"github.com/yourname/XOpsAgent/internal/model"
)

func TestAnalyzeIncident(t *testing.T) {
	svc := NewMemoryCaseStore()
	service := NewWithStore(structConfig(), svc)

	result, err := service.AnalyzeIncident(context.Background(), AnalyzeRequest{
		Summary: "checkout latency spiked after rollout",
		Metrics: []model.Metric{
			{ServiceName: "checkout", Timestamp: time.Unix(10, 0), LatencyAvg: 650, LatencyMax: 1100, ErrorRate: 0.02},
			{ServiceName: "inventory", Timestamp: time.Unix(12, 0), LatencyAvg: 80, LatencyMax: 120, ErrorRate: 0.00},
		},
		Traces: []model.Trace{
			{
				ID: "trace-1",
				Spans: []model.Span{
					{ID: "root", Service: "checkout", Operation: "HTTP GET /cart", ParentID: "", DurationMs: 100, Tags: map[string]string{"k8s.pod.name": "checkout-7df", "k8s.node.name": "node-a"}},
					{ID: "child", Service: "checkout", Operation: "postgres query", ParentID: "root", DurationMs: 600, Error: true, Tags: map[string]string{"error": "true", "pod": "checkout-7df", "node": "node-a"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeIncident returned error: %v", err)
	}
	if len(result.SuspectServices) == 0 || result.SuspectServices[0] != "checkout" {
		t.Fatalf("expected checkout as suspect service, got %#v", result.SuspectServices)
	}
	if result.RootCause.Type != "Unknown" {
		t.Fatalf("expected trace-based fallback cause Unknown, got %s", result.RootCause.Type)
	}
	if result.SupportingTrace == nil || result.SupportingTrace.TraceID != "trace-1" {
		t.Fatalf("expected supporting trace to be populated: %#v", result.SupportingTrace)
	}
}

func TestGeneratePlanAndRunAgentFallback(t *testing.T) {
	service := NewWithStore(structConfig(), NewMemoryCaseStore())
	caseItem := service.CreateCase(CreateCaseRequest{
		Title:    "checkout errors",
		Severity: "critical",
		Summary:  "customer checkout is failing",
	})

	plan, err := service.GeneratePlan(context.Background(), PlanRequest{
		CaseID:  caseItem.ID,
		Summary: caseItem.Summary,
		Metrics: []model.Metric{
			{ServiceName: "checkout", Timestamp: time.Unix(10, 0), ErrorRate: 0.12},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if plan.RiskLevel == "" || len(plan.Steps) == 0 {
		t.Fatalf("expected populated plan, got %#v", plan)
	}

	result, err := service.RunAgent(context.Background(), AgentRunRequest{
		CaseID:  caseItem.ID,
		Summary: caseItem.Summary,
	})
	if err != nil {
		t.Fatalf("RunAgent returned error: %v", err)
	}
	if result.Backend != "local" {
		t.Fatalf("expected local fallback backend, got %s", result.Backend)
	}
	if result.Response == "" {
		t.Fatalf("expected non-empty fallback response")
	}
}

func structConfig() config.Config {
	return config.Config{
		Ops: config.OpsConfig{
			Codex: config.CodexConfig{Enabled: false},
		},
	}
}
