package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditDiagnosticsDefaults(t *testing.T) {
	cfg := &AppConfig{}
	if err := cfg.Audit.normalize(); err != nil {
		t.Fatalf("Audit.normalize: %v", err)
	}
	diag := cfg.Audit.Diagnostics
	if !diag.IsEnabled() {
		t.Fatal("diagnostics should be enabled by default")
	}
	if diag.Methodology != "quick-probe-v1" {
		t.Fatalf("Methodology = %q, want quick-probe-v1", diag.Methodology)
	}
	if diag.CredentialMode != ProbeCredentialModeProbeFallback {
		t.Fatalf("CredentialMode = %q, want %q", diag.CredentialMode, ProbeCredentialModeProbeFallback)
	}
	if diag.RequestTimeoutDur != 60*time.Second {
		t.Fatalf("RequestTimeoutDur = %v, want 60s", diag.RequestTimeoutDur)
	}
	if diag.StepGapMinDur != time.Minute || diag.StepGapMaxDur != 4*time.Minute {
		t.Fatalf("step gaps = %v/%v, want 1m/4m", diag.StepGapMinDur, diag.StepGapMaxDur)
	}
}

func TestAuditDiagnosticsRejectsInvalidCredentialMode(t *testing.T) {
	cfg := &AppConfig{
		Audit: AuditConfig{
			Diagnostics: DiagnosticsConfig{CredentialMode: "direct"},
		},
	}
	if err := cfg.Audit.normalize(); err == nil {
		t.Fatal("Audit.normalize should reject invalid credential mode")
	}
}

func TestAuditDiagnosticsRejectsInvalidDuration(t *testing.T) {
	cfg := &AppConfig{
		Audit: AuditConfig{
			Diagnostics: DiagnosticsConfig{RequestTimeout: "soon"},
		},
	}
	if err := cfg.Audit.normalize(); err == nil {
		t.Fatal("Audit.normalize should reject invalid request_timeout")
	}
}

func TestAuditDiagnosticsNormalizesTemplateBinding(t *testing.T) {
	cfg := &AppConfig{
		Audit: AuditConfig{
			Diagnostics: DiagnosticsConfig{
				TemplateBinding: TemplateBindingConfig{
					Default: map[string]string{
						" cx ": " cx-gpt-chat ",
						"":     "ignored",
						"cc":   "",
					},
					ModelFamily: map[string]map[string]string{
						" gpt ": map[string]string{" cx ": " cx-gpt-chat "},
					},
					ChannelType: map[string]map[string]string{
						" official_direct ": map[string]string{" cc ": " cc-sonnet-arith "},
					},
				},
			},
		},
	}
	if err := cfg.Audit.normalize(); err != nil {
		t.Fatalf("Audit.normalize: %v", err)
	}
	got := cfg.Audit.Diagnostics.TemplateBinding.Default
	if got["cx"] != "cx-gpt-chat" {
		t.Fatalf("template binding cx=%q", got["cx"])
	}
	if _, ok := got[""]; ok {
		t.Fatalf("empty template binding key should be dropped: %+v", got)
	}
	if _, ok := got["cc"]; ok {
		t.Fatalf("empty template binding value should be dropped: %+v", got)
	}
	if cfg.Audit.Diagnostics.TemplateBinding.ModelFamily["gpt"]["cx"] != "cx-gpt-chat" {
		t.Fatalf("model_family binding not normalized: %+v", cfg.Audit.Diagnostics.TemplateBinding.ModelFamily)
	}
	if cfg.Audit.Diagnostics.TemplateBinding.ChannelType["official_direct"]["cc"] != "cc-sonnet-arith" {
		t.Fatalf("channel_type binding not normalized: %+v", cfg.Audit.Diagnostics.TemplateBinding.ChannelType)
	}
}

func TestLoaderRejectsProbeOnlyWithoutProbeCredential(t *testing.T) {
	t.Setenv("NEWAPI_BASE_URL", "https://newapi.example.com")
	t.Setenv("NEWAPI_ACCESS_TOKEN", "sync-token")
	t.Setenv("NEWAPI_USER_ID", "sync-user")
	t.Setenv("NEWAPI_PROBE_ACCESS_TOKEN", "")
	t.Setenv("NEWAPI_PROBE_USER_ID", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := []byte(`
interval: "1m"
slow_latency: "5s"
audit:
  diagnostics:
    credential_mode: "probe_only"
storage:
  type: "sqlite"
  sqlite:
    path: "test.db"
monitors:
  - provider: "demo"
    service: "cc"
    channel: "main"
    base_url: "https://example.com"
    url_pattern: "{{BASE_URL}}"
    method: "GET"
    category: "public"
    sponsor: "test"
`)
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := NewLoader()
	if _, err := loader.Load(cfgPath); err == nil {
		t.Fatal("Loader.Load should fail when probe_only is configured without probe credentials")
	}
}
