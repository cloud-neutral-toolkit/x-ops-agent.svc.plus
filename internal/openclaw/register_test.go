package openclaw

import "testing"

func TestNormalizeAgentID(t *testing.T) {
	if got := normalizeAgentID("X Ops/Agent"); got != "x-ops-agent" {
		t.Fatalf("unexpected normalized id: %s", got)
	}
}

func TestRegistrationCreateName(t *testing.T) {
	if got := registrationCreateName("xops-agent", "XOpsAgent"); got != "xops-agent" {
		t.Fatalf("expected create name to fall back to agent id, got %q", got)
	}
	if got := registrationCreateName("xops-agent", "XOps Agent"); got != "XOps Agent" {
		t.Fatalf("expected human readable name to be preserved, got %q", got)
	}
}
