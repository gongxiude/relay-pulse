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

func TestAuditTargetsPreserveAPIKeyOnReplace(t *testing.T) {
	store := newTestStore(t)

	first := []AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		APIKey:       "sk-channel-key",
	}}
	if err := store.ReplaceAuditTargets(first); err != nil {
		t.Fatalf("ReplaceAuditTargets first: %v", err)
	}

	second := []AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
	}}
	if err := store.ReplaceAuditTargets(second); err != nil {
		t.Fatalf("ReplaceAuditTargets second: %v", err)
	}

	targets, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(targets))
	}
	if targets[0].APIKey != "sk-channel-key" {
		t.Fatalf("APIKey = %q, want preserved key", targets[0].APIKey)
	}
}

func TestAuditTargetCredentialUpdateAppliesToChannelModels(t *testing.T) {
	store := newTestStore(t)
	targets := []AuditTarget{
		{Provider: "p1", Service: "cc", Channel: "101:demo", Model: "m1", RequestModel: "m1", Enabled: true},
		{Provider: "p1", Service: "cc", Channel: "101:demo", Model: "m2", RequestModel: "m2", Enabled: true},
		{Provider: "p1", Service: "cc", Channel: "102:other", Model: "m1", RequestModel: "m1", Enabled: true},
	}
	if err := store.ReplaceAuditTargets(targets); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	result, err := store.SetAuditTargetCredential("p1", "cc", "101:demo", "sk-channel-key-1234")
	if err != nil {
		t.Fatalf("SetAuditTargetCredential: %v", err)
	}
	if result.Updated != 2 || !result.KeyConfigured || result.KeyLast4 != "1234" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	got, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	keys := map[string]string{}
	for _, target := range got {
		keys[target.Channel+"|"+target.Model] = target.APIKey
	}
	if keys["101:demo|m1"] != "sk-channel-key-1234" || keys["101:demo|m2"] != "sk-channel-key-1234" {
		t.Fatalf("channel keys not applied: %+v", keys)
	}
	if keys["102:other|m1"] != "" {
		t.Fatalf("other channel key should stay empty: %+v", keys)
	}

	cleared, err := store.ClearAuditTargetCredential("p1", "cc", "101:demo")
	if err != nil {
		t.Fatalf("ClearAuditTargetCredential: %v", err)
	}
	if cleared.Updated != 2 || cleared.KeyConfigured || cleared.KeyLast4 != "" {
		t.Fatalf("unexpected clear result: %+v", cleared)
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

	runs, err := store.ListDiagnosticRuns(DiagnosticRunFilter{
		Provider: "Anthropic",
		Service:  "claude",
		Channel:  "cc",
		Model:    "claude-sonnet-4-6",
		Status:   "done",
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("ListDiagnosticRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != run.RunID {
		t.Fatalf("unexpected runs: %+v", runs)
	}

	doneCount, err := store.CountDiagnosticRuns("done")
	if err != nil {
		t.Fatalf("CountDiagnosticRuns: %v", err)
	}
	if doneCount != 1 {
		t.Fatalf("unexpected done count: %d", doneCount)
	}
}

func TestDiagnosticRunFilterOffsetAndCount(t *testing.T) {
	store := newTestStore(t)
	runs := []*DiagnosticRun{
		{RunID: "run-1", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "done", CreatedAt: 100, UpdatedAt: 100},
		{RunID: "run-2", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "failed_auth", CreatedAt: 200, UpdatedAt: 200},
		{RunID: "run-3", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "failed_request", CreatedAt: 300, UpdatedAt: 300},
		{RunID: "run-4", Provider: "p1", Service: "openai", Channel: "ch1", Model: "m1", Status: "done", CreatedAt: 400, UpdatedAt: 400},
	}
	for _, run := range runs {
		if err := store.SaveDiagnosticRun(run); err != nil {
			t.Fatalf("SaveDiagnosticRun(%s): %v", run.RunID, err)
		}
	}

	filter := DiagnosticRunFilter{
		Provider: "p1",
		Service:  "anthropic",
		Channel:  "ch1",
		Model:    "m1",
		Limit:    2,
		Offset:   1,
	}
	got, err := store.ListDiagnosticRuns(filter)
	if err != nil {
		t.Fatalf("ListDiagnosticRuns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0].RunID != "run-2" || got[1].RunID != "run-1" {
		t.Fatalf("unexpected paged order: got %s,%s", got[0].RunID, got[1].RunID)
	}

	count, err := store.CountDiagnosticRunsFiltered(filter)
	if err != nil {
		t.Fatalf("CountDiagnosticRunsFiltered: %v", err)
	}
	if count != 3 {
		t.Fatalf("count=%d, want 3", count)
	}
}

func TestAuditDiagnosticGroupAndDimensionsRoundTrip(t *testing.T) {
	store := newTestStore(t)

	if err := store.SaveDiagnosticRun(&DiagnosticRun{
		RunID:     "run-candidate",
		Provider:  "Anthropic",
		Service:   "claude",
		Channel:   "cc",
		Model:     "claude-sonnet-4-6",
		Status:    "done",
		CreatedAt: 1710000009,
		UpdatedAt: 1710000009,
	}); err != nil {
		t.Fatalf("SaveDiagnosticRun candidate: %v", err)
	}
	if err := store.SaveDiagnosticRun(&DiagnosticRun{
		RunID:     "run-baseline",
		Provider:  "official-provider",
		Service:   "anthropic",
		Channel:   "80:alan-官key直连",
		Model:     "claude-sonnet-4-6",
		Status:    "failed_auth",
		CreatedAt: 1710000009,
		UpdatedAt: 1710000009,
	}); err != nil {
		t.Fatalf("SaveDiagnosticRun baseline: %v", err)
	}

	group := &DiagnosticRunGroup{
		GroupID:            "group-1",
		CandidateRunID:     "run-candidate",
		BaselineRunID:      "run-baseline",
		BaselineMode:       "single_run_only",
		MethodologyVersion: "quick-probe-v1",
		WeightsHash:        "weights-v1",
		CreatedAt:          1710000010,
	}
	if err := store.SaveDiagnosticRunGroup(group); err != nil {
		t.Fatalf("SaveDiagnosticRunGroup: %v", err)
	}

	gotGroup, err := store.GetDiagnosticRunGroup(group.GroupID)
	if err != nil {
		t.Fatalf("GetDiagnosticRunGroup: %v", err)
	}
	if gotGroup == nil || gotGroup.CandidateRunID != group.CandidateRunID || gotGroup.BaselineRunID != group.BaselineRunID {
		t.Fatalf("unexpected group: %+v", gotGroup)
	}

	dim1 := &DiagnosticDimension{
		RunID:           "run-candidate",
		DimensionKey:    "model_match",
		Weight:          14,
		Score:           10,
		NormalizedScore: 7.4,
		Status:          "pass",
		Reason:          "request model equals response model",
		Evidence:        []byte(`{"actual":"claude-sonnet-4-6","baseline":"claude-sonnet-4-6"}`),
		CreatedAt:       1710000011,
	}
	dim2 := &DiagnosticDimension{
		RunID:           "run-candidate",
		DimensionKey:    "identity_free_clean",
		Weight:          7,
		Score:           5,
		NormalizedScore: 2.6,
		Status:          "partial",
		Reason:          "wrapper identity exposed once",
		Evidence:        []byte(`{"actual":"I am Claude","baseline":"I am Claude"}`),
		CreatedAt:       1710000012,
	}
	for _, dim := range []*DiagnosticDimension{dim1, dim2} {
		if err := store.SaveDiagnosticDimension(dim); err != nil {
			t.Fatalf("SaveDiagnosticDimension(%s): %v", dim.DimensionKey, err)
		}
	}

	dimensions, err := store.ListDiagnosticDimensions("run-candidate")
	if err != nil {
		t.Fatalf("ListDiagnosticDimensions: %v", err)
	}
	if len(dimensions) != 2 {
		t.Fatalf("unexpected dimensions len: %d", len(dimensions))
	}
	if dimensions[0].DimensionKey != "model_match" || dimensions[1].DimensionKey != "identity_free_clean" {
		t.Fatalf("unexpected dimension order: %+v", dimensions)
	}
	if !strings.Contains(string(dimensions[0].Evidence), `"actual":"claude-sonnet-4-6"`) {
		t.Fatalf("unexpected dimension evidence: %s", string(dimensions[0].Evidence))
	}

	baseline := &DiagnosticBaselineRun{
		BaselineID:         "baseline-1",
		Service:            "anthropic",
		ModelFamily:        "claude-sonnet-4",
		RunID:              "run-baseline",
		Provider:           "official-provider",
		Channel:            "80:alan-官key直连",
		Source:             "self_reported_official",
		MethodologyVersion: "quick-probe-v1",
		CapturedAt:         1710000020,
	}
	if err := store.SaveDiagnosticBaselineRun(baseline); err != nil {
		t.Fatalf("SaveDiagnosticBaselineRun: %v", err)
	}
	gotBaseline, err := store.GetLatestDiagnosticBaselineRun("anthropic", "claude-sonnet-4", "quick-probe-v1", "")
	if err != nil {
		t.Fatalf("GetLatestDiagnosticBaselineRun: %v", err)
	}
	if gotBaseline == nil || gotBaseline.RunID != baseline.RunID || gotBaseline.Source != baseline.Source {
		t.Fatalf("unexpected baseline: %+v", gotBaseline)
	}

	summary, err := store.GetDiagnosticDimensionSummary()
	if err != nil {
		t.Fatalf("GetDiagnosticDimensionSummary: %v", err)
	}
	if summary == nil || summary.RunCount != 1 || summary.DimensionCount != 2 {
		t.Fatalf("unexpected dimension summary: %+v", summary)
	}
}
