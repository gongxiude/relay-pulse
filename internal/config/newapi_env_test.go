package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAPIEnvOverrides(t *testing.T) {
	t.Setenv("NEWAPI_BASE_URL", "https://newapi.example.com")
	t.Setenv("NEWAPI_ACCESS_TOKEN", "token-123")
	t.Setenv("NEWAPI_USER_ID", "user-456")

	cfg := &AppConfig{}
	cfg.applyEnvOverrides()

	if cfg.NewAPI.BaseURL != "https://newapi.example.com" {
		t.Fatalf("NewAPI.BaseURL = %q, want %q", cfg.NewAPI.BaseURL, "https://newapi.example.com")
	}
	if cfg.NewAPI.AccessToken != "token-123" {
		t.Fatalf("NewAPI.AccessToken = %q, want %q", cfg.NewAPI.AccessToken, "token-123")
	}
	if cfg.NewAPI.UserID != "user-456" {
		t.Fatalf("NewAPI.UserID = %q, want %q", cfg.NewAPI.UserID, "user-456")
	}
}

func TestLoadDotenvKeepsNewAPIEnv(t *testing.T) {
	dir := t.TempDir()
	dotenv := []byte("NEWAPI_BASE_URL=https://dotenv.example.com\nNEWAPI_ACCESS_TOKEN=dotenv-token\nNEWAPI_USER_ID=dotenv-user\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), dotenv, 0o600); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}

	t.Setenv("NEWAPI_BASE_URL", "https://env.example.com")
	t.Setenv("NEWAPI_ACCESS_TOKEN", "env-token")
	t.Setenv("NEWAPI_USER_ID", "env-user")

	if err := LoadDotenvFromConfigDir(dir+"/config.yaml", false); err != nil {
		t.Fatalf("LoadDotenvFromConfigDir: %v", err)
	}

	if got := os.Getenv("NEWAPI_BASE_URL"); got != "https://env.example.com" {
		t.Fatalf("env NEWAPI_BASE_URL = %q, want %q", got, "https://env.example.com")
	}
	if got := os.Getenv("NEWAPI_ACCESS_TOKEN"); got != "env-token" {
		t.Fatalf("env NEWAPI_ACCESS_TOKEN = %q, want %q", got, "env-token")
	}
	if got := os.Getenv("NEWAPI_USER_ID"); got != "env-user" {
		t.Fatalf("env NEWAPI_USER_ID = %q, want %q", got, "env-user")
	}
}

func TestLoaderRejectsMissingNewAPIEnv(t *testing.T) {
	t.Setenv("NEWAPI_BASE_URL", "")
	t.Setenv("NEWAPI_ACCESS_TOKEN", "")
	t.Setenv("NEWAPI_USER_ID", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := []byte(`
interval: "1m"
slow_latency: "5s"
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
		t.Fatal("Loader.Load should fail when NEWAPI_* env vars are missing")
	}
}
