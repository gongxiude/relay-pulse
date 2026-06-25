package storage

import (
	"strings"
	"testing"
)

func TestAuditChannelSnapshotRoundTrip(t *testing.T) {
	store := newTestStore(t)

	snap := &ChannelSnapshot{
		NewAPIChannelID: 101,
		SnapshotAt:      1710000000,
		Provider:        "Anthropic",
		Service:         "claude",
		Channel:         "cc",
		Model:           "claude-sonnet-4-6",
		Enabled:         true,
		Raw:             []byte(`{"id":101,"status":1}`),
	}
	if err := store.SaveChannelSnapshot(snap); err != nil {
		t.Fatalf("SaveChannelSnapshot: %v", err)
	}

	got, err := store.GetChannelSnapshot(101, 1710000000)
	if err != nil {
		t.Fatalf("GetChannelSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("GetChannelSnapshot returned nil")
	}
	if got.Provider != snap.Provider || got.Model != snap.Model || !got.Enabled {
		t.Fatalf("unexpected snapshot: %+v", got)
	}

	stats, err := store.GetLatestChannelSnapshotStats()
	if err != nil {
		t.Fatalf("GetLatestChannelSnapshotStats: %v", err)
	}
	if stats == nil || stats.SnapshotAt != snap.SnapshotAt || stats.ChannelCount != 1 || stats.EnabledCount != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestAuditLogCursorUpsert(t *testing.T) {
	store := newTestStore(t)

	cur := &LogSyncCursor{Name: "default", LastID: 10, LastTime: 100, UpdatedAt: 1}
	if err := store.UpsertLogSyncCursor(cur); err != nil {
		t.Fatalf("UpsertLogSyncCursor: %v", err)
	}
	cur.LastID = 20
	cur.LastTime = 200
	cur.UpdatedAt = 2
	if err := store.UpsertLogSyncCursor(cur); err != nil {
		t.Fatalf("UpsertLogSyncCursor update: %v", err)
	}

	got, err := store.GetLogSyncCursor("default")
	if err != nil {
		t.Fatalf("GetLogSyncCursor: %v", err)
	}
	if got == nil || got.LastID != 20 || got.LastTime != 200 || got.UpdatedAt != 2 {
		t.Fatalf("unexpected cursor: %+v", got)
	}
}

func TestAuditNewAPILogsRoundTrip(t *testing.T) {
	store := newTestStore(t)

	input := []NewAPILog{
		{
			ID:               10,
			CreatedAt:        1710000001,
			Type:             2,
			ModelName:        "gpt-4o",
			CompletionTokens: 42,
			UseTime:          7,
			IsStream:         true,
			ChannelID:        99,
			Group:            "default",
			RequestID:        "req-1",
			Other:            []byte(`{"frt":123}`),
		},
		{
			ID:               11,
			CreatedAt:        1710000002,
			Type:             5,
			ModelName:        "gpt-4o",
			CompletionTokens: 0,
			UseTime:          3,
			IsStream:         false,
			ChannelID:        99,
			Group:            "default",
			RequestID:        "req-2",
			Other:            []byte(`{"error_type":"timeout"}`),
		},
	}
	if err := store.SaveNewAPILogs(input); err != nil {
		t.Fatalf("SaveNewAPILogs: %v", err)
	}

	got, err := store.ListNewAPILogsSince(1710000000)
	if err != nil {
		t.Fatalf("ListNewAPILogsSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got len = %d, want 2", len(got))
	}
	if got[0].ID != 11 || got[1].ID != 10 {
		t.Fatalf("unexpected order: %+v", got)
	}
	if !strings.Contains(string(got[1].Other), `"frt":123`) {
		t.Fatalf("unexpected other payload: %s", string(got[1].Other))
	}
}

func TestAuditDiagnosticRoundTrip(t *testing.T) {
	store := newTestStore(t)

	run := &DiagnosticRun{
		RunID:     "run-1",
		Provider:  "Anthropic",
		Service:   "claude",
		Channel:   "cc",
		Model:     "claude-sonnet-4-6",
		Status:    "done",
		CreatedAt: 1710000000,
		UpdatedAt: 1710000001,
		Input:     []byte(`{"window":"30d"}`),
		Output:    []byte(`{"result":"ok"}`),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
	}

	step := &DiagnosticStep{
		RunID:               run.RunID,
		StepIndex:           1,
		Prompt:              "ping",
		ResolvedPrompt:      "ping",
		ResponsePreview:     "pong",
		ResultSummary:       "ok",
		ExecutionMeta:       []byte(`{"latency_ms":123}`),
		ChannelFingerprint:  "cfp",
		ProviderFingerprint: "pfp",
		ErrorMessage:        "",
		CreatedAt:           1710000002,
	}
	if err := store.SaveDiagnosticStep(step); err != nil {
		t.Fatalf("SaveDiagnosticStep: %v", err)
	}

	score := &DiagnosticScore{
		RunID:             run.RunID,
		AuthenticityScore: 90,
		ProtocolScore:     80,
		SSEScore:          70,
		Tags:              []byte(`["fallback"]`),
		CreatedAt:         1710000003,
	}
	if err := store.SaveDiagnosticScore(score); err != nil {
		t.Fatalf("SaveDiagnosticScore: %v", err)
	}

	gotRun, err := store.GetDiagnosticRun(run.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticRun: %v", err)
	}
	if gotRun == nil || gotRun.Status != "done" {
		t.Fatalf("unexpected run: %+v", gotRun)
	}

	steps, err := store.ListDiagnosticSteps(run.RunID)
	if err != nil {
		t.Fatalf("ListDiagnosticSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].StepIndex != 1 {
		t.Fatalf("unexpected steps: %+v", steps)
	}

	gotScore, err := store.GetDiagnosticScore(run.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticScore: %v", err)
	}
	if gotScore == nil || gotScore.AuthenticityScore != 90 {
		t.Fatalf("unexpected score: %+v", gotScore)
	}
}
