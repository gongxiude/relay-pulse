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

func TestBuildAuditTargetsInfersServiceFromGroupAndModels(t *testing.T) {
	channels := []ChannelSpec{
		{
			ID:     80,
			Type:   14,
			Status: 1,
			Name:   "alan-官key直连",
			Models: "claude-sonnet-4-6,claude-opus-4-7",
			Group:  "模型渠道测试分组",
		},
		{
			ID:     64,
			Type:   1,
			Status: 1,
			Name:   "yuexin01-team5000-sunday-2133",
			Models: "gpt-5.4,gpt-5.5",
			Group:  "openai",
		},
	}

	targets := BuildAuditTargets(channels)
	if len(targets) != 4 {
		t.Fatalf("targets len = %d, want 4", len(targets))
	}
	if targets[0].Service != "anthropic" {
		t.Fatalf("expected claude channel service to be anthropic: %+v", targets[0])
	}
	if targets[2].Service != "openai" {
		t.Fatalf("expected gpt channel service to be openai: %+v", targets[2])
	}
	if targets[0].Group != "模型渠道测试分组" {
		t.Fatalf("group should retain original new-api group: %+v", targets[0])
	}
}
