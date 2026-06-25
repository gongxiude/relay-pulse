package audit

import (
	"testing"
)

func TestBuildAuditTargetsExpandsModels(t *testing.T) {
	mapJSON := `{"gpt-4o":"gpt-4o","claude-sonnet-4-6":"claude-3-5-sonnet"}`
	channels := []ChannelSpec{
		{
			ID:           7,
			Type:         1,
			Status:       1,
			Name:         "demo",
			Models:       "gpt-4o,claude-sonnet-4-6",
			Group:        "default",
			ModelMapping: &mapJSON,
			Other:        []byte(`{"provider":"Anthropic","service":"cc"}`),
		},
		{
			ID:     8,
			Type:   2,
			Status: 0,
			Name:   "disabled",
			Models: "gemini-2.0-pro",
			Group:  "group-a",
			Other:  []byte(`{"provider":"Google","service":"gemini"}`),
		},
	}

	targets := BuildAuditTargets(channels)
	if len(targets) != 3 {
		t.Fatalf("targets len = %d, want 3", len(targets))
	}

	if targets[0].Provider != "Anthropic" || targets[0].Service != "cc" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[0].RequestModel != "claude-sonnet-4-6" || targets[0].Model != "claude-3-5-sonnet" {
		t.Fatalf("unexpected model mapping: %+v", targets[0])
	}
	if !targets[0].Enabled {
		t.Fatalf("expected enabled target")
	}

	if targets[2].Enabled {
		t.Fatalf("expected disabled target: %+v", targets[2])
	}
}
