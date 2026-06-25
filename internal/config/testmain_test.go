package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("NEWAPI_BASE_URL") == "" {
		_ = os.Setenv("NEWAPI_BASE_URL", "https://test.newapi.local")
	}
	if os.Getenv("NEWAPI_ACCESS_TOKEN") == "" {
		_ = os.Setenv("NEWAPI_ACCESS_TOKEN", "test-token")
	}
	if os.Getenv("NEWAPI_USER_ID") == "" {
		_ = os.Setenv("NEWAPI_USER_ID", "test-user")
	}
	os.Exit(m.Run())
}
