package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"monitor/internal/storage"
)

func TestDiagnosticRunner(t *testing.T) {
	store := newDiagnosticStore(t)
	now := time.Unix(1710000000, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization=%q", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "u1" {
			t.Fatalf("user=%q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		model := payload["model"].(string)
		if model != "gpt-4o" {
			t.Fatalf("model=%s", model)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	runner := NewDiagnosticRunner(srv.Client())
	runner.Now = func() time.Time { return now }
	target := DiagnosticTarget{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		BaseURL:      srv.URL,
		AccessToken:  "token",
		UserID:       "u1",
	}
	run, err := runner.Run(context.Background(), target, store)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run == nil || run.Status != "done" {
		t.Fatalf("run=%+v", run)
	}
	got, err := store.GetDiagnosticRun(run.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticRun: %v", err)
	}
	if got == nil || got.Status != "done" {
		t.Fatalf("stored run=%+v", got)
	}
	steps, err := store.ListDiagnosticSteps(run.RunID)
	if err != nil {
		t.Fatalf("ListDiagnosticSteps: %v", err)
	}
	if len(steps) != 6 {
		t.Fatalf("steps=%d", len(steps))
	}
	score, err := store.GetDiagnosticScore(run.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticScore: %v", err)
	}
	if score == nil || score.AuthenticityScore <= 0 || score.ProtocolScore <= 0 || score.SSEScore <= 0 {
		t.Fatalf("score=%+v", score)
	}
	if !strings.Contains(string(score.Tags), "buffered_stream") && !strings.Contains(string(score.Tags), "request_error") {
		t.Fatalf("tags=%s", string(score.Tags))
	}
}

func newDiagnosticStore(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "diag.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

