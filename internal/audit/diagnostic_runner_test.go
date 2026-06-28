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

	"monitor/internal/config"
	"monitor/internal/storage"
)

func TestDiagnosticRunner(t *testing.T) {
	store := newDiagnosticStore(t)
	now := time.Unix(1710000000, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.URL.Path != "/diagnostic/chat" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token" {
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
		if payload["template_marker"] != "from-template" {
			t.Fatalf("template marker missing in payload: %+v", payload)
		}
		if _, ok := payload["messages"].([]any); !ok {
			t.Fatalf("messages not injected by diagnostic override: %+v", payload)
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
		Template: &config.ProbeTemplate{
			URL:    "{{BASE_URL}}/diagnostic/chat",
			Method: http.MethodPost,
			Headers: map[string]string{
				"Authorization": "{{API_KEY}}",
				"Content-Type":  "application/json",
				"Accept":        "text/event-stream, application/json",
			},
			BodyRaw:        json.RawMessage(`{"model":"placeholder","messages":[],"stream":false,"template_marker":"from-template"}`),
			RequestFamily:  "openai_chat",
			OverridePaths:  map[string]string{"model": "$.model", "messages": "$.messages", "stream": "$.stream"},
			ResponseParser: "openai_chat_sse",
		},
		TemplateName: "unit-diagnostic-template",
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
	if len(steps) != 10 {
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
	var input map[string]any
	if err := json.Unmarshal(got.Input, &input); err != nil {
		t.Fatalf("unmarshal run input: %v", err)
	}
	groupID, _ := input["group_id"].(string)
	if groupID == "" {
		t.Fatalf("missing group_id in run input: %s", string(got.Input))
	}
	group, err := store.GetDiagnosticRunGroup(groupID)
	if err != nil {
		t.Fatalf("GetDiagnosticRunGroup: %v", err)
	}
	if group == nil || group.CandidateRunID != run.RunID || group.MethodologyVersion != "quick-probe-v1" {
		t.Fatalf("unexpected group: %+v", group)
	}
	dimensions, err := store.ListDiagnosticDimensions(run.RunID)
	if err != nil {
		t.Fatalf("ListDiagnosticDimensions: %v", err)
	}
	if len(dimensions) != 3 {
		t.Fatalf("unexpected dimensions len: %d", len(dimensions))
	}
}

func testOpenAIChatDiagnosticTemplate() *config.ProbeTemplate {
	return &config.ProbeTemplate{
		URL:    "{{BASE_URL}}/diagnostic/chat",
		Method: http.MethodPost,
		Headers: map[string]string{
			"Authorization": "{{API_KEY}}",
			"Content-Type":  "application/json",
			"Accept":        "text/event-stream, application/json",
		},
		BodyRaw:        json.RawMessage(`{"model":"placeholder","messages":[],"stream":false}`),
		RequestFamily:  "openai_chat",
		OverridePaths:  map[string]string{"model": "$.model", "messages": "$.messages", "stream": "$.stream"},
		ResponseParser: "openai_chat_sse",
	}
}

func TestPhase2EvidenceExtraction(t *testing.T) {
	steps := []*storage.DiagnosticStep{
		{
			RunID:     "run-1",
			StepIndex: 1,
			ExecutionMeta: mustJSON(map[string]any{
				"response_headers": map[string]string{
					"x-request-id":     "req_01abc",
					"cf-ray":           "abc-SJC",
					"x-stainless-lang": "go",
				},
				"usage": map[string]any{
					"input_tokens":                100,
					"cache_read_input_tokens":     40,
					"cache_creation_input_tokens": 10,
				},
				"stream_chunks": []string{"a", "b", "c"},
				"request_body":  map[string]any{"model": "claude-opus-4-6"},
				"response_text": `{"id":"msg_01abc","thinking":"hidden chain"}`,
			}),
		},
	}
	if got := collectResponseIDs(steps); len(got) == 0 {
		t.Fatalf("expected response ids")
	}
	if got := collectInferenceGeoValues(steps); len(got) == 0 {
		t.Fatalf("expected geo values")
	}
	if got := collectSDKNames(steps); len(got) == 0 {
		t.Fatalf("expected sdk names")
	}
	if got := collectStreamChunkStats(steps); got.BufferedSteps != 0 || len(got.ChunkCounts) != 1 {
		t.Fatalf("unexpected stream stats: %+v", got)
	}
	if got := collectCacheUsageStats(steps); !got.HasCacheFields || got.CacheReadTokens != 40 {
		t.Fatalf("unexpected cache stats: %+v", got)
	}
	if got := collectThinkingStats(steps); !got.Present {
		t.Fatalf("expected thinking stats")
	}
}

func TestBuildDimensionsForRunWithPhase2ExistingEvidence(t *testing.T) {
	candidateSteps := []*storage.DiagnosticStep{
		{
			RunID:     "candidate",
			StepIndex: 2,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":      "identity",
				"response_model": "claude-opus-4-6",
				"response_id":    "msg_01candidate",
				"response_text":  "vendor: Anthropic\nbrand: Claude\nmodel: Claude Opus 4",
				"response_headers": map[string]string{
					"x-request-id":     "req_01candidate",
					"cf-ray":           "abc-SJC",
					"x-stainless-lang": "go",
				},
				"request_body":  map[string]any{"model": "claude-opus-4-6", "messages": []any{}},
				"stream_chunks": []string{"a", "b", "c"},
			}),
		},
	}
	baselineSteps := []*storage.DiagnosticStep{
		{
			RunID:     "baseline",
			StepIndex: 2,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":      "identity",
				"response_model": "claude-opus-4-6",
				"response_id":    "msg_01baseline",
				"response_text":  "vendor: Anthropic\nbrand: Claude\nmodel: Claude Opus 4",
				"response_headers": map[string]string{
					"x-request-id":     "req_01baseline",
					"cf-ray":           "xyz-SJC",
					"x-stainless-lang": "go",
				},
				"request_body":  map[string]any{"model": "claude-opus-4-6", "messages": []any{}},
				"stream_chunks": []string{"a", "b", "c"},
			}),
		},
	}
	dimensions := buildDimensionsForRun("run-1", &storage.DiagnosticScore{AuthenticityScore: 90, ProtocolScore: 90, SSEScore: 90}, nil, candidateSteps, baselineSteps, 1710000000)
	found := make(map[string]*storage.DiagnosticDimension, len(dimensions))
	for _, dimension := range dimensions {
		found[dimension.DimensionKey] = dimension
	}
	for _, key := range []string{
		"self_identity_consistency",
		"envelope_self_report_match",
		"anthropic_msg_id_format",
		"inference_geo_present",
		"system_prompt_clean",
		"sdk_consistency",
		"buffer_dump_match",
	} {
		if found[key] == nil {
			t.Fatalf("missing dimension %s", key)
		}
		if found[key].Status == "skip" {
			t.Fatalf("dimension %s skipped unexpectedly: %+v", key, found[key])
		}
	}
}

func TestPhase2WorldKnowledgeAndThinkingScorers(t *testing.T) {
	candidateSteps := []*storage.DiagnosticStep{
		{
			RunID:     "candidate",
			StepIndex: 7,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "world_knowledge_tier",
				"response_text": "RP_WORLD_KNOWLEDGE=5",
			}),
		},
		{
			RunID:     "candidate",
			StepIndex: 8,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "thinking_probe",
				"response_text": "reasoning_content: compact hidden reasoning",
				"content": []any{
					map[string]any{"type": "thinking", "text": "hidden reasoning"},
					map[string]any{"type": "text", "text": "RP_THINKING_CHECK=410"},
				},
			}),
		},
	}
	baselineSteps := []*storage.DiagnosticStep{
		{
			RunID:     "baseline",
			StepIndex: 7,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "world_knowledge_tier",
				"response_text": "RP_WORLD_KNOWLEDGE=5",
			}),
		},
		{
			RunID:     "baseline",
			StepIndex: 8,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "thinking_probe",
				"response_text": "reasoning_content: compact hidden reasoning",
				"content": []any{
					map[string]any{"type": "thinking", "text": "hidden reasoning"},
					map[string]any{"type": "text", "text": "RP_THINKING_CHECK=410"},
				},
			}),
		},
	}
	dimensions := buildDimensionsForRun("run-1", &storage.DiagnosticScore{AuthenticityScore: 90, ProtocolScore: 90, SSEScore: 90}, nil, candidateSteps, baselineSteps, 1710000000)
	found := make(map[string]*storage.DiagnosticDimension, len(dimensions))
	for _, dimension := range dimensions {
		found[dimension.DimensionKey] = dimension
	}
	if found["world_knowledge_tier_match"] == nil || found["world_knowledge_tier_match"].Status != "pass" {
		t.Fatalf("unexpected world_knowledge_tier_match: %+v", found["world_knowledge_tier_match"])
	}
	for _, key := range []string{"thinking_present", "thinking_volume_match"} {
		if found[key] == nil {
			t.Fatalf("missing dimension %s", key)
		}
		if found[key].Status == "skip" {
			t.Fatalf("dimension %s skipped unexpectedly: %+v", key, found[key])
		}
	}
}

func TestPhase2CacheScorers(t *testing.T) {
	candidateSteps := []*storage.DiagnosticStep{
		{
			RunID:     "candidate",
			StepIndex: 9,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name": "cache_seed",
				"usage": map[string]any{
					"input_tokens":            100,
					"cache_read_input_tokens": 30,
				},
				"response_text": "RP_CACHE_SEEDED=1",
			}),
		},
		{
			RunID:     "candidate",
			StepIndex: 10,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "cache_recall",
				"response_text": "RP_CACHE_MARKER=blue-17-river",
			}),
		},
	}
	baselineSteps := []*storage.DiagnosticStep{
		{
			RunID:     "baseline",
			StepIndex: 9,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name": "cache_seed",
				"usage": map[string]any{
					"input_tokens":            100,
					"cache_read_input_tokens": 30,
				},
				"response_text": "RP_CACHE_SEEDED=1",
			}),
		},
		{
			RunID:     "baseline",
			StepIndex: 10,
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "cache_recall",
				"response_text": "RP_CACHE_MARKER=blue-17-river",
			}),
		},
	}
	dimensions := buildDimensionsForRun("run-1", &storage.DiagnosticScore{AuthenticityScore: 90, ProtocolScore: 90, SSEScore: 90}, nil, candidateSteps, baselineSteps, 1710000000)
	found := make(map[string]*storage.DiagnosticDimension, len(dimensions))
	for _, dimension := range dimensions {
		found[dimension.DimensionKey] = dimension
	}
	for _, key := range []string{"cache_hit_ratio_match", "cache_continuity_intra"} {
		if found[key] == nil || found[key].Status != "pass" {
			t.Fatalf("unexpected %s: %+v", key, found[key])
		}
	}
	for _, key := range []string{"cache_sliding_correctness", "cache_ttl_consistency", "tier_thinking_volume_match"} {
		if found[key] == nil || found[key].Status != "skip" {
			t.Fatalf("expected inactive skip %s: %+v", key, found[key])
		}
	}
}

func TestMethodologyPhase2Summary(t *testing.T) {
	spec := CurrentMethodologySpec()
	if spec.ImplementedCount != 25 {
		t.Fatalf("ImplementedCount=%d, want 25", spec.ImplementedCount)
	}
	if spec.ActiveCount != 22 {
		t.Fatalf("ActiveCount=%d, want 22", spec.ActiveCount)
	}
	if spec.TotalWeight != 200 {
		t.Fatalf("TotalWeight=%d, want 200", spec.TotalWeight)
	}
	if spec.ImplementedWeight != 200 {
		t.Fatalf("ImplementedWeight=%d, want 200", spec.ImplementedWeight)
	}
	if spec.ActiveWeight != 164 {
		t.Fatalf("ActiveWeight=%d, want 164", spec.ActiveWeight)
	}
	unimplemented := make([]string, 0)
	inactive := make([]string, 0)
	for _, dimension := range spec.Dimensions {
		if !dimension.Implemented {
			unimplemented = append(unimplemented, dimension.Key)
		}
		if dimension.Implemented && !dimension.Active {
			inactive = append(inactive, dimension.Key)
		}
	}
	if len(unimplemented) != 0 {
		t.Fatalf("unexpected unimplemented dimensions: %v", unimplemented)
	}
	for _, key := range []string{"cache_sliding_correctness", "cache_ttl_consistency", "tier_thinking_volume_match"} {
		if !stringSliceContains(inactive, key) {
			t.Fatalf("expected inactive %s in %v", key, inactive)
		}
	}
	if len(inactive) != 3 {
		t.Fatalf("unexpected inactive dimensions: %v", inactive)
	}
}

func TestDiagnosticRunnerUsesRegisteredBaselineForCandidate(t *testing.T) {
	store := newDiagnosticStore(t)
	now := time.Unix(1710000100, 0)

	baselineCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baselineCalls++
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

	officialTarget := DiagnosticTarget{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-sonnet-4-6",
		RequestModel: "claude-sonnet-4-6",
		BaseURL:      srv.URL,
		AccessToken:  "token",
		UserID:       "u1",
		Template:     testOpenAIChatDiagnosticTemplate(),
		TemplateName: "unit-diagnostic-template",
	}
	officialRun, err := runner.Run(context.Background(), officialTarget, store)
	if err != nil {
		t.Fatalf("Run official: %v", err)
	}
	if officialRun == nil {
		t.Fatal("official run is nil")
	}

	candidateTarget := DiagnosticTarget{
		Provider:     "third-party",
		Service:      "anthropic",
		Channel:      "81:demo",
		Model:        "claude-sonnet-4-6",
		RequestModel: "claude-sonnet-4-6",
		BaseURL:      srv.URL,
		AccessToken:  "token",
		UserID:       "u1",
		Template:     testOpenAIChatDiagnosticTemplate(),
		TemplateName: "unit-diagnostic-template",
	}
	candidateRun, err := runner.Run(context.Background(), candidateTarget, store)
	if err != nil {
		t.Fatalf("Run candidate: %v", err)
	}

	gotCandidate, err := store.GetDiagnosticRun(candidateRun.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticRun candidate: %v", err)
	}
	var candidateInput map[string]any
	if err := json.Unmarshal(gotCandidate.Input, &candidateInput); err != nil {
		t.Fatalf("unmarshal candidate input: %v", err)
	}
	groupID, _ := candidateInput["group_id"].(string)
	group, err := store.GetDiagnosticRunGroup(groupID)
	if err != nil {
		t.Fatalf("GetDiagnosticRunGroup candidate: %v", err)
	}
	if group == nil || group.BaselineRunID != officialRun.RunID || group.BaselineMode != "latest_registered_baseline" {
		t.Fatalf("unexpected candidate group: %+v", group)
	}
	dimensions, err := store.ListDiagnosticDimensions(candidateRun.RunID)
	if err != nil {
		t.Fatalf("ListDiagnosticDimensions candidate: %v", err)
	}
	if len(dimensions) < 9 {
		t.Fatalf("expected baseline-aware dimensions, got %d", len(dimensions))
	}
	gotCandidateRun, err := store.GetDiagnosticRun(candidateRun.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticRun candidate final: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal(gotCandidateRun.Output, &output); err != nil {
		t.Fatalf("unmarshal candidate output: %v", err)
	}
	if _, ok := output["overall_score"].(float64); !ok {
		t.Fatalf("missing overall_score in output: %s", string(gotCandidateRun.Output))
	}
	if activeWeight, ok := output["active_weight"].(float64); !ok || activeWeight <= 0 {
		t.Fatalf("missing active_weight in output: %s", string(gotCandidateRun.Output))
	}
	if baselineCalls <= 0 {
		t.Fatalf("expected server to be called, got %d", baselineCalls)
	}
}

func TestDiagnosticRunnerMarksAll401AsFailedAuth(t *testing.T) {
	store := newDiagnosticStore(t)
	now := time.Unix(1710000200, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	runner := NewDiagnosticRunner(srv.Client())
	runner.Now = func() time.Time { return now }
	run, err := runner.Run(context.Background(), DiagnosticTarget{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		BaseURL:      srv.URL,
		AccessToken:  "token",
		UserID:       "u1",
		Template:     testOpenAIChatDiagnosticTemplate(),
		TemplateName: "unit-diagnostic-template",
	}, store)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run == nil || run.Status != "failed_auth" {
		t.Fatalf("run=%+v", run)
	}
	got, err := store.GetDiagnosticRun(run.RunID)
	if err != nil {
		t.Fatalf("GetDiagnosticRun: %v", err)
	}
	if got == nil || got.Status != "failed_auth" {
		t.Fatalf("stored run=%+v", got)
	}
	baseline, err := store.GetLatestDiagnosticBaselineRun("cc", "gpt-4o", "quick-probe-v1", "")
	if err != nil {
		t.Fatalf("GetLatestDiagnosticBaselineRun: %v", err)
	}
	if baseline != nil {
		t.Fatalf("unexpected baseline registration: %+v", baseline)
	}
}

func TestBuildDimensionsForRunWithBaselineAwareScorers(t *testing.T) {
	score := &storage.DiagnosticScore{
		AuthenticityScore: 90,
		ProtocolScore:     80,
		SSEScore:          70,
		Tags:              []byte(`["buffered_stream"]`),
	}
	candidateSteps := []*storage.DiagnosticStep{
		{
			StepIndex: 2,
			Prompt:    "identity",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":      "identity",
				"response_model": "claude-sonnet-4-6",
				"request_body":   map[string]any{"model": "claude-sonnet-4-6"},
				"response_text":  "data: {\"choices\":[{\"delta\":{\"content\":\"vendor: Anthropic\\nbrand: Claude\\nmodel: claude-sonnet-4-6\"}}]}",
			}),
		},
		{
			StepIndex: 4,
			Prompt:    "identity free",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "identity_free",
				"response_text": `data: {"choices":[{"delta":{"content":"I am Kiro assistant"}}]}`,
			}),
		},
		{
			StepIndex: 1,
			Prompt:    "ping",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":        "ping",
				"latency_ms":       180,
				"ttft_ms":          240,
				"first_text_ms":    240,
				"finish_reason":    "stop",
				"usage":            map[string]any{"service_tier": "standard"},
				"response_headers": map[string]any{"request-id": "req_candidate_01"},
			}),
		},
		{
			StepIndex: 3,
			Prompt:    "cutoff",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "cutoff",
				"response_text": `data: {"choices":[{"delta":{"content":"我的知识截止日期是 2024年06月"}}]}`,
			}),
		},
		{
			StepIndex: 5,
			Prompt:    "knowledge",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "knowledge_recall",
				"response_text": `data: {"choices":[{"delta":{"content":"365 天"}}]}`,
			}),
		},
	}
	baselineSteps := []*storage.DiagnosticStep{
		{
			StepIndex: 2,
			Prompt:    "identity",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":      "identity",
				"response_model": "claude-sonnet-4-6",
				"request_body":   map[string]any{"model": "claude-sonnet-4-6"},
				"response_text":  "data: {\"choices\":[{\"delta\":{\"content\":\"vendor: Anthropic\\nbrand: Claude\\nmodel: claude-sonnet-4-6\"}}]}",
			}),
		},
		{
			StepIndex: 4,
			Prompt:    "identity free",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "identity_free",
				"response_text": `data: {"choices":[{"delta":{"content":"我是 Claude，由 Anthropic 训练。"}}]}`,
			}),
		},
		{
			StepIndex: 1,
			Prompt:    "ping",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":        "ping",
				"latency_ms":       90,
				"ttft_ms":          100,
				"first_text_ms":    100,
				"finish_reason":    "stop",
				"usage":            map[string]any{"service_tier": "standard"},
				"response_headers": map[string]any{"request-id": "req_baseline_01"},
			}),
		},
		{
			StepIndex: 3,
			Prompt:    "cutoff",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "cutoff",
				"response_text": `data: {"choices":[{"delta":{"content":"我的知识截止日期是 2024年06月"}}]}`,
			}),
		},
		{
			StepIndex: 5,
			Prompt:    "knowledge",
			ExecutionMeta: mustJSON(map[string]any{
				"step_name":     "knowledge_recall",
				"response_text": `data: {"choices":[{"delta":{"content":"365 天"}}]}`,
			}),
		},
	}

	dimensions := buildDimensionsForRun("run-1", score, []string{"buffered_stream"}, candidateSteps, baselineSteps, 1710000200)
	if len(dimensions) != 27 {
		t.Fatalf("unexpected dimensions len: %d", len(dimensions))
	}
	found := make(map[string]*storage.DiagnosticDimension, len(dimensions))
	for _, dim := range dimensions {
		found[dim.DimensionKey] = dim
	}
	if found["identity_free_clean"] == nil || found["identity_free_clean"].Status != "fail" {
		t.Fatalf("unexpected identity_free_clean: %+v", found["identity_free_clean"])
	}
	if found["model_match"] == nil || found["model_match"].Status != "pass" {
		t.Fatalf("unexpected model_match: %+v", found["model_match"])
	}
	if found["identity_structured_match"] == nil || found["identity_structured_match"].Status != "pass" {
		t.Fatalf("unexpected identity_structured_match: %+v", found["identity_structured_match"])
	}
	if found["service_tier_present"] == nil || found["service_tier_present"].Status != "pass" {
		t.Fatalf("unexpected service_tier_present: %+v", found["service_tier_present"])
	}
	if found["anthropic_request_id_passthrough"] == nil || found["anthropic_request_id_passthrough"].Status != "pass" {
		t.Fatalf("unexpected anthropic_request_id_passthrough: %+v", found["anthropic_request_id_passthrough"])
	}
	if found["stop_reason_present"] == nil || found["stop_reason_present"].Status != "pass" {
		t.Fatalf("unexpected stop_reason_present: %+v", found["stop_reason_present"])
	}
	if found["cutoff_match"] == nil || found["cutoff_match"].Status != "pass" {
		t.Fatalf("unexpected cutoff_match: %+v", found["cutoff_match"])
	}
	if found["knowledge_recall_match"] == nil || found["knowledge_recall_match"].Status != "pass" {
		t.Fatalf("unexpected knowledge_recall_match: %+v", found["knowledge_recall_match"])
	}
	if found["instruction_following_lang"] == nil || found["instruction_following_lang"].Status != "fail" {
		t.Fatalf("unexpected instruction_following_lang: %+v", found["instruction_following_lang"])
	}
	if found["latency_baseline_match"] == nil || found["latency_baseline_match"].Status != "fail" || found["latency_baseline_match"].Score != 0 {
		t.Fatalf("unexpected latency_baseline_match: %+v", found["latency_baseline_match"])
	}
	overall, activeWeight, skipped := dimensionScoringSummary(dimensions)
	if overall <= 0 || activeWeight <= 0 {
		t.Fatalf("unexpected scoring summary: overall=%v active=%d skipped=%v", overall, activeWeight, skipped)
	}
	for _, key := range []string{"anthropic_msg_id_format", "buffer_dump_match", "inference_geo_present", "sdk_consistency"} {
		if !stringSliceContains(skipped, key) {
			t.Fatalf("expected skipped %s in %v", key, skipped)
		}
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

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
