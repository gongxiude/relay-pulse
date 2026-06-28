package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"monitor/internal/config"
	"monitor/internal/probe"
	"monitor/internal/storage"
)

func TestBuildTemplateProbeConfigUsesExistingTemplate(t *testing.T) {
	configDir := t.TempDir()
	templatesDir := filepath.Join(configDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll templates: %v", err)
	}
	template := `{
		"model": "GPT",
		"request_model": "gpt-4o-mini",
		"url": "{{BASE_URL}}/v1/chat/completions",
		"method": "POST",
		"headers": {
			"Authorization": "{{API_KEY}}",
			"Content-Type": "application/json"
		},
		"body": {"model":"{{MODEL}}","messages":[{"role":"user","content":"ping"}]},
		"response": {"success_contains": "pong"},
		"probe": {"slow_latency": "5s", "timeout": "10s", "retry": 0}
	}`
	if err := os.WriteFile(filepath.Join(templatesDir, "cx-unit.json"), []byte(template), 0o644); err != nil {
		t.Fatalf("WriteFile template: %v", err)
	}
	app := &config.AppConfig{
		IntervalDuration:       time.Minute,
		SlowLatencyDuration:    3 * time.Second,
		TimeoutDuration:        15 * time.Second,
		RetryBaseDelayDuration: 200 * time.Millisecond,
		RetryMaxDelayDuration:  2 * time.Second,
	}
	target := storage.AuditTarget{
		Provider:     "OpenAI",
		Service:      "cx",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
	}
	cfg, err := BuildTemplateProbeConfig(app, target, TemplateProbeCredentials{
		BaseURL:     "https://newapi.example.com",
		AccessToken: "probe-token",
		UserID:      "u1",
	}, "cx-unit", configDir)
	if err != nil {
		t.Fatalf("BuildTemplateProbeConfig: %v", err)
	}
	if cfg.Provider != "OpenAI" || cfg.Service != "cx" || cfg.Channel != "101:demo" || cfg.Model != "gpt-4o" {
		t.Fatalf("unexpected PSCM: %+v", cfg)
	}
	if cfg.URLPattern != "{{BASE_URL}}/v1/chat/completions" || cfg.Method != "POST" || cfg.APIKey != "probe-token" {
		t.Fatalf("template fields not applied: %+v", cfg)
	}
	if cfg.SuccessContains != "pong" || cfg.TimeoutDuration != 10*time.Second || cfg.SlowLatencyDuration != 5*time.Second {
		t.Fatalf("probe options not applied: success=%q timeout=%v slow=%v", cfg.SuccessContains, cfg.TimeoutDuration, cfg.SlowLatencyDuration)
	}
}

func TestBuildTemplateProbeConfigUsesTargetCredential(t *testing.T) {
	configDir := t.TempDir()
	templatesDir := filepath.Join(configDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll templates: %v", err)
	}
	template := `{
		"model": "GPT",
		"request_model": "gpt-4o",
		"url": "{{BASE_URL}}/v1/chat/completions",
		"method": "POST",
		"headers": {
			"Authorization": "{{API_KEY}}",
			"Content-Type": "application/json"
		},
		"body": {"model":"{{MODEL}}","messages":[{"role":"user","content":"ping"}]},
		"response": {"success_contains": "pong"}
	}`
	if err := os.WriteFile(filepath.Join(templatesDir, "cx-gpt-chat-diagnostic.json"), []byte(template), 0o644); err != nil {
		t.Fatalf("WriteFile template: %v", err)
	}
	app := &config.AppConfig{}
	target := storage.AuditTarget{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}
	cfg, err := BuildTemplateProbeConfig(app, target, TemplateProbeCredentials{
		BaseURL:     "https://newapi.example.com",
		AccessToken: "sk-channel-key",
		UserID:      "u1",
	}, "cx-gpt-chat-diagnostic", configDir)
	if err != nil {
		t.Fatalf("BuildTemplateProbeConfig: %v", err)
	}
	if cfg.APIKey != "sk-channel-key" {
		t.Fatalf("APIKey = %q, want channel key", cfg.APIKey)
	}
}

func TestProbeRecordFromTemplateProbeResult(t *testing.T) {
	target := storage.AuditTarget{
		Provider: "OpenAI",
		Service:  "cx",
		Channel:  "101:demo",
		Model:    "gpt-4o",
	}
	record, err := ProbeRecordFromTemplateProbeResult(target, &probe.Result{
		ProbeStatus:  0,
		SubStatus:    "auth_error",
		HTTPCode:     401,
		Latency:      123,
		ErrorMessage: "invalid token",
	}, time.Unix(1710000000, 0))
	if err != nil {
		t.Fatalf("ProbeRecordFromTemplateProbeResult: %v", err)
	}
	if record.Provider != "OpenAI" || record.Service != "cx" || record.Channel != "101:demo" || record.Model != "gpt-4o" {
		t.Fatalf("unexpected record identity: %+v", record)
	}
	if record.Status != 0 || record.SubStatus != storage.SubStatusAuthError || record.HttpCode != 401 || record.Latency != 123 || record.Timestamp != 1710000000 {
		t.Fatalf("unexpected record status: %+v", record)
	}
	if record.ErrorDetail != "invalid token" {
		t.Fatalf("unexpected error detail: %q", record.ErrorDetail)
	}
}

func TestResolveTemplateProbeNameUsesServiceDefault(t *testing.T) {
	app := &config.AppConfig{
		Audit: config.AuditConfig{
			Diagnostics: config.DiagnosticsConfig{
				TemplateBinding: config.TemplateBindingConfig{
					Default: map[string]string{"cx": "cx-unit"},
				},
			},
		},
	}
	got, err := ResolveTemplateProbeName(app, "cx", "")
	if err != nil {
		t.Fatalf("ResolveTemplateProbeName: %v", err)
	}
	if got != "cx-unit" {
		t.Fatalf("template=%q", got)
	}
	if explicit, err := ResolveTemplateProbeName(app, "cx", "cx-explicit"); err != nil || explicit != "cx-explicit" {
		t.Fatalf("explicit template=%q err=%v", explicit, err)
	}
}
