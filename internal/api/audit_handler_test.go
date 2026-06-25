package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"monitor/internal/config"
	"monitor/internal/storage"
)

func newAuditTestStore(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "audit-api.db")
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

func newAuditTestRouter(store *storage.SQLiteStorage, cfg *config.AppConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(store, cfg, nil)
	r := gin.New()
	r.GET("/api/audit/newapi/sync/status", h.GetAuditSyncStatus)
	r.POST("/api/audit/newapi/sync/channels", h.PostAuditSyncChannels)
	r.POST("/api/audit/newapi/sync/logs", h.PostAuditSyncLogs)
	r.POST("/api/audit/diagnostics", h.PostAuditDiagnosticSubmit)
	r.GET("/api/audit/channels", h.GetAuditChannels)
	r.GET("/api/audit/targets", h.GetAuditTargets)
	r.GET("/api/audit/ranking", h.GetAuditRanking)
	r.GET("/api/audit/methodology", h.GetAuditMethodology)
	r.GET("/api/audit/diagnostics/latest", h.GetAuditDiagnosticLatest)
	r.POST("/api/audit/diagnostics/backfill", h.PostAuditDiagnosticBackfill)
	r.GET("/api/audit/diagnostics/:run_id", h.GetAuditDiagnostic)
	r.GET("/api/audit/compare/:run_id", h.GetAuditCompare)
	return r
}

func TestAuditSyncEndpointsAndReads(t *testing.T) {
	store := newAuditTestStore(t)
	mapJSON := `{"gpt-4o":"gpt-4o"}`
	now := time.Now().Unix()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channel/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []map[string]any{{
						"id":            101,
						"type":          1,
						"status":        1,
						"name":          "demo-官key直连",
						"models":        "gpt-4o",
						"group":         "default",
						"model_mapping": mapJSON,
						"other":         json.RawMessage(`{"provider":"OpenAI","service":"cc"}`),
					}},
					"total":       1,
					"page":        1,
					"page_size":   10,
					"type_counts": map[string]int{"1": 1},
				},
			})
		case "/api/log/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "",
				"data": map[string]any{
					"items": []map[string]any{{
						"id":                  11,
						"created_at":          now,
						"type":                2,
						"content":             "ok",
						"model_name":          "gpt-4o",
						"quota":               10,
						"prompt_tokens":       2,
						"completion_tokens":   20,
						"use_time":            4,
						"is_stream":           true,
						"channel":             101,
						"group":               "default",
						"request_id":          "r1",
						"upstream_request_id": "u1",
						"other":               json.RawMessage(`{"frt":80}`),
					}},
					"total":     1,
					"page":      1,
					"page_size": 10,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:             upstream.URL,
			AccessToken:         "token",
			UserID:              "1",
			Timeout:             "10s",
			ChannelSyncInterval: "5m",
			LogSyncInterval:     "1m",
		},
		DegradedWeight: 0.7,
	}
	router := newAuditTestRouter(store, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/audit/newapi/sync/channels", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync channels code = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/audit/newapi/sync/logs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync logs code = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/newapi/sync/status", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"total":1`) {
		t.Fatalf("sync status unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/targets", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"provider":"OpenAI"`) {
		t.Fatalf("targets unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/channels", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK ||
		!containsJSON(rec.Body.String(), `"channel":"101:demo-官key直连"`) ||
		!containsJSON(rec.Body.String(), `"channelType":"official"`) ||
		!containsJSON(rec.Body.String(), `"channelTypeLabel":"官方直连"`) {
		t.Fatalf("channels unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/ranking?window=24h", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"success_rate":100`) {
		t.Fatalf("ranking unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/methodology", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK ||
		!containsJSON(rec.Body.String(), `"version":"v3.24.1"`) ||
		!containsJSON(rec.Body.String(), `"total_dimensions":25`) {
		t.Fatalf("methodology unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditDiagnosticAndCompare(t *testing.T) {
	store := newAuditTestStore(t)
	baselineRun := &storage.DiagnosticRun{
		RunID:     "baseline-1",
		Provider:  "OpenAI Official",
		Service:   "cc",
		Channel:   "80:official",
		Model:     "gpt-4o",
		Status:    "done",
		CreatedAt: 1709999990,
		UpdatedAt: 1709999991,
	}
	if err := store.SaveDiagnosticRun(baselineRun); err != nil {
		t.Fatalf("SaveDiagnosticRun baseline: %v", err)
	}
	if err := store.SaveDiagnosticStep(&storage.DiagnosticStep{
		RunID:           baselineRun.RunID,
		StepIndex:       1,
		Prompt:          "ping",
		ResolvedPrompt:  "ping",
		ResponsePreview: "official-pong",
		ResultSummary:   "ok",
		ExecutionMeta:   []byte(`{"step_name":"ping","response_text":"official-pong","latency_ms":88}`),
		CreatedAt:       1709999992,
	}); err != nil {
		t.Fatalf("SaveDiagnosticStep baseline: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             baselineRun.RunID,
		AuthenticityScore: 100,
		ProtocolScore:     100,
		SSEScore:          100,
		CreatedAt:         1709999993,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore baseline: %v", err)
	}
	run := &storage.DiagnosticRun{
		RunID:     "run-1",
		Provider:  "OpenAI",
		Service:   "cc",
		Channel:   "101:demo",
		Model:     "gpt-4o",
		Status:    "done",
		CreatedAt: 1710000000,
		UpdatedAt: 1710000001,
		Input:     []byte(`{"group_id":"group-1","request_model":"gpt-4o"}`),
		Output:    []byte(`{"baseline_mode":"latest_registered_baseline","baseline_run_id":"baseline-1","candidate_type":"candidate_with_baseline","methodology_version":"quick-probe-v1","weights_hash":"legacy-summary-v1"}`),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
	}
	if err := store.SaveDiagnosticRun(&storage.DiagnosticRun{
		RunID:     "run-bad",
		Provider:  "OpenAI",
		Service:   "cc",
		Channel:   "101:demo",
		Model:     "gpt-4o",
		Status:    "done",
		CreatedAt: 1710000000,
		UpdatedAt: 1710000001,
		Input:     []byte(`{"group_id":"group-bad","request_model":"gpt-4o"}`),
		Output:    []byte(`{"run_status":"failed_auth","tags":["request_error"]}`),
	}); err != nil {
		t.Fatalf("SaveDiagnosticRun bad: %v", err)
	}
	if err := store.SaveDiagnosticStep(&storage.DiagnosticStep{
		RunID:           run.RunID,
		StepIndex:       1,
		Prompt:          "ping",
		ResolvedPrompt:  "ping",
		ResponsePreview: "pong",
		ResultSummary:   "ok",
		CreatedAt:       1710000002,
	}); err != nil {
		t.Fatalf("SaveDiagnosticStep: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             run.RunID,
		AuthenticityScore: 95,
		ProtocolScore:     88,
		SSEScore:          70,
		CreatedAt:         1710000003,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             "run-bad",
		AuthenticityScore: 0,
		ProtocolScore:     0,
		SSEScore:          0,
		Tags:              []byte(`["request_error"]`),
		CreatedAt:         1710000003,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore bad: %v", err)
	}
	if err := store.SaveDiagnosticRunGroup(&storage.DiagnosticRunGroup{
		GroupID:            "group-1",
		CandidateRunID:     run.RunID,
		BaselineRunID:      baselineRun.RunID,
		BaselineMode:       "latest_registered_baseline",
		MethodologyVersion: "quick-probe-v1",
		WeightsHash:        "legacy-summary-v1",
		CreatedAt:          1710000004,
	}); err != nil {
		t.Fatalf("SaveDiagnosticRunGroup: %v", err)
	}
	if err := store.SaveDiagnosticDimension(&storage.DiagnosticDimension{
		RunID:           run.RunID,
		DimensionKey:    "authenticity_summary",
		Weight:          1,
		Score:           9.5,
		NormalizedScore: 95,
		Status:          "pass",
		Reason:          "legacy summary score",
		CreatedAt:       1710000005,
	}); err != nil {
		t.Fatalf("SaveDiagnosticDimension: %v", err)
	}

	router := newAuditTestRouter(store, &config.AppConfig{DegradedWeight: 0.7})

	req := httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/run-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var diagResp struct {
		Success bool                    `json:"success"`
		Data    auditDiagnosticResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &diagResp); err != nil {
		t.Fatalf("unmarshal diagnostics response: %v", err)
	}
	if !diagResp.Success || diagResp.Data.Run.RunID != "run-1" {
		t.Fatalf("unexpected diagnostics payload: %+v", diagResp)
	}
	if diagResp.Data.Score == nil || diagResp.Data.Score.OverallScore <= 0 || diagResp.Data.Score.ActiveWeight <= 0 {
		t.Fatalf("unexpected diagnostics score: %+v", diagResp.Data.Score)
	}
	if len(diagResp.Data.Steps) != 1 || diagResp.Data.Steps[0].StepName != "ping" {
		t.Fatalf("unexpected diagnostics steps: %+v", diagResp.Data.Steps)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/compare/run-1", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compare unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var compareResp struct {
		Success bool                 `json:"success"`
		Data    auditCompareResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &compareResp); err != nil {
		t.Fatalf("unmarshal compare response: %v", err)
	}
	if !compareResp.Success || compareResp.Data.Group.CandidateRunID != "run-1" {
		t.Fatalf("unexpected compare payload: %+v", compareResp)
	}
	if compareResp.Data.Candidate.Run.RunID != "run-1" || len(compareResp.Data.Steps) != 1 {
		t.Fatalf("unexpected compare candidate: %+v", compareResp.Data)
	}
	if compareResp.Data.Baseline == nil || compareResp.Data.Baseline.Run.RunID != "baseline-1" {
		t.Fatalf("unexpected compare baseline: %+v", compareResp.Data.Baseline)
	}
	if len(compareResp.Data.Dimensions) != 1 {
		t.Fatalf("unexpected compare dimensions: %+v", compareResp.Data.Dimensions)
	}
	if compareResp.Data.Summary.OverallScore <= 0 || compareResp.Data.Steps[0].Candidate.StepName != "ping" || compareResp.Data.Steps[0].Baseline == nil {
		t.Fatalf("unexpected compare summary/steps: %+v", compareResp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/latest?provider=OpenAI&service=cc&channel=101:demo&model=gpt-4o", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var latestResp struct {
		Success bool                          `json:"success"`
		Data    auditDiagnosticLatestResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &latestResp); err != nil {
		t.Fatalf("unmarshal latest response: %v", err)
	}
	if !latestResp.Success || len(latestResp.Data.Items) != 1 || latestResp.Data.Items[0].Run.RunID != "run-1" {
		t.Fatalf("unexpected latest payload: %+v", latestResp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/methodology", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("methodology unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSON(rec.Body.String(), `"done_runs":1`) || !containsJSON(rec.Body.String(), `"dimension_runs":1`) {
		t.Fatalf("unexpected methodology payload: %s", rec.Body.String())
	}
}

func TestAuditDiagnosticSubmit(t *testing.T) {
	store := newAuditTestStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     srv.URL,
			AccessToken: "token",
			UserID:      "u1",
		},
	}
	router := newAuditTestRouter(store, cfg)
	body := `{"provider":"OpenAI","service":"cc","channel":"101:demo","model":"gpt-4o","request_model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"run_id"`) {
		t.Fatalf("submit unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditDiagnosticBackfill(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{
		{
			Provider:     "OpenAI",
			Service:      "cc",
			Channel:      "101:demo",
			Model:        "gpt-4o",
			RequestModel: "gpt-4o",
			Weight:       10,
			Priority:     5,
			Enabled:      true,
		},
	}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	if err := store.SaveNewAPILogs([]storage.NewAPILog{{
		ID:               1,
		CreatedAt:        now,
		Type:             2,
		ModelName:        "gpt-4o",
		ChannelID:        101,
		PromptTokens:     10,
		CompletionTokens: 20,
		UseTime:          1,
	}}); err != nil {
		t.Fatalf("SaveNewAPILogs: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	router := newAuditTestRouter(store, &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     srv.URL,
			AccessToken: "token",
			UserID:      "u1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics/backfill", strings.NewReader(`{"max_targets":1,"max_models_per_channel":1,"lookback_hours":24}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backfill unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool                            `json:"success"`
		Data    auditDiagnosticBackfillResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal backfill response: %v", err)
	}
	if !resp.Success || resp.Data.Selected != 1 || resp.Data.Started != 1 || len(resp.Data.Items) != 1 || resp.Data.Items[0].RunID == "" {
		t.Fatalf("unexpected backfill payload: %+v", resp)
	}
}

func containsJSON(body, needle string) bool {
	return len(body) > 0 && strings.Contains(body, needle)
}
