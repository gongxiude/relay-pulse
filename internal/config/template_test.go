package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProbeTemplateKeepsLegacyTemplateCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(path, []byte(`{
		"model": "Haiku",
		"url": "{{BASE_URL}}/v1/messages",
		"method": "POST",
		"headers": {"Content-Type": "application/json"},
		"body": {"model": "{{MODEL}}"}
	}`), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	tmpl, err := LoadProbeTemplate(path)
	if err != nil {
		t.Fatalf("LoadProbeTemplate legacy: %v", err)
	}
	if tmpl.RequestFamily != "" || len(tmpl.OverridePaths) != 0 || tmpl.ResponseParser != "" {
		t.Fatalf("legacy template should not require diagnostic fields: %+v", tmpl)
	}
}

func TestLoadProbeTemplateParsesDiagnosticContract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "diag.json")
	if err := os.WriteFile(path, []byte(`{
		"model": "GPT",
		"request_model": "gpt-4o",
		"url": "{{BASE_URL}}/v1/chat/completions",
		"method": "POST",
		"headers": {"Content-Type": "application/json"},
		"body": {"model": "{{MODEL}}", "messages": [], "stream": true},
		"request_family": "openai_chat",
		"override_paths": {
			"messages": "$.messages",
			"model": "$.model",
			"stream": "$.stream"
		},
		"response_parser": "openai_chat_sse"
	}`), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	tmpl, err := LoadProbeTemplate(path)
	if err != nil {
		t.Fatalf("LoadProbeTemplate diagnostic: %v", err)
	}
	if tmpl.RequestFamily != "openai_chat" || tmpl.ResponseParser != "openai_chat_sse" {
		t.Fatalf("unexpected diagnostic metadata: %+v", tmpl)
	}
	if tmpl.OverridePaths["messages"] != "$.messages" || tmpl.OverridePaths["model"] != "$.model" {
		t.Fatalf("unexpected override paths: %+v", tmpl.OverridePaths)
	}
}

func TestLoadProbeTemplateRejectsInvalidDiagnosticOverridePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{
		"model": "GPT",
		"url": "{{BASE_URL}}/v1/chat/completions",
		"method": "POST",
		"headers": {"Content-Type": "application/json"},
		"body": {"model": "{{MODEL}}"},
		"request_family": "openai_chat",
		"override_paths": {"messages": "messages"},
		"response_parser": "openai_chat_sse"
	}`), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if _, err := LoadProbeTemplate(path); err == nil {
		t.Fatal("LoadProbeTemplate should reject invalid diagnostic override path")
	}
}
