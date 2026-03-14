package a2a

import "testing"

func TestNegotiateDeclinesAutomationGoal(t *testing.T) {
	svc := NewService("xops-agent", "ops", "x-automation-agent")
	resp := svc.Negotiate(Request{
		FromAgentID: "x-automation-agent",
		RequestID:   "req-1",
		Intent:      "execute",
		Goal:        "apply terraform remediation",
	})
	if resp.Status != "declined" {
		t.Fatalf("expected declined, got %s", resp.Status)
	}
	if got := resp.Result["handoff_agent_id"]; got != "x-automation-agent" {
		t.Fatalf("expected automation handoff, got %#v", got)
	}
}

func TestCreateTaskStoresCompletedIncident(t *testing.T) {
	svc := NewService("xops-agent", "ops", "x-automation-agent")
	task := svc.CreateTask(Request{
		FromAgentID: "x-observability-agent",
		RequestID:   "req-2",
		Intent:      "analyze",
		Goal:        "incident root cause for checkout outage",
	})
	if task.Status != "completed" {
		t.Fatalf("expected completed, got %s", task.Status)
	}
	stored, ok := svc.GetTask(task.TaskID)
	if !ok {
		t.Fatalf("expected task to be persisted")
	}
	if stored.RequestID != "req-2" {
		t.Fatalf("unexpected request id: %s", stored.RequestID)
	}
}
