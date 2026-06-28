package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func newAuditTestRouter(t *testing.T, store *storage.SQLiteStorage, cfg *config.AppConfig) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	configDir := newAuditTestConfigDir(t)
	ensureAuditTestDiagnosticConfig(cfg)
	h := NewHandler(store, cfg, nil)
	h.SetMonitorStore(config.NewMonitorStore(filepath.Join(configDir, config.MonitorsDirName)))
	r := gin.New()
	r.GET("/api/audit/newapi/sync/status", h.GetAuditSyncStatus)
	r.POST("/api/audit/newapi/sync/channels", h.PostAuditSyncChannels)
	r.POST("/api/audit/newapi/sync/logs", h.PostAuditSyncLogs)
	r.POST("/api/audit/diagnostics", h.PostAuditDiagnosticSubmit)
	r.GET("/api/audit/channels", h.GetAuditChannels)
	r.GET("/api/audit/targets", h.GetAuditTargets)
	r.PUT("/api/audit/targets/credential", h.PutAuditTargetCredential)
	r.DELETE("/api/audit/targets/credential", h.DeleteAuditTargetCredential)
	r.GET("/api/audit/ranking", h.GetAuditRanking)
	r.GET("/api/audit/model-status", h.GetAuditModelStatus)
	r.GET("/api/audit/methodology", h.GetAuditMethodology)
	r.GET("/api/audit/diagnostics/latest", h.GetAuditDiagnosticLatest)
	r.GET("/api/audit/diagnostics/history", h.GetAuditDiagnosticHistory)
	r.POST("/api/audit/diagnostics/backfill", h.PostAuditDiagnosticBackfill)
	r.POST("/api/audit/template-probes/backfill", h.PostAuditTemplateProbeBackfill)
	r.GET("/api/audit/diagnostics/:run_id", h.GetAuditDiagnostic)
	r.GET("/api/audit/compare/:run_id", h.GetAuditCompare)
	return r
}

func newAuditTestConfigDir(t *testing.T) string {
	t.Helper()
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll templates: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, config.MonitorsDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll monitors.d: %v", err)
	}
	template := `{
		"model": "GPT",
		"request_model": "gpt-4o",
		"url": "{{BASE_URL}}/diagnostic/chat",
		"method": "POST",
		"headers": {
			"Authorization": "{{API_KEY}}",
			"Content-Type": "application/json",
			"Accept": "text/event-stream, application/json"
		},
		"body": {
			"model": "{{MODEL}}",
			"messages": [],
			"stream": true,
			"temperature": 0
		},
		"request_family": "openai_chat",
		"override_paths": {
			"messages": "$.messages",
			"model": "$.model",
			"stream": "$.stream"
		},
		"response_parser": "openai_chat_sse"
	}`
	if err := os.WriteFile(filepath.Join(configDir, "templates", "unit-diagnostic.json"), []byte(template), 0o644); err != nil {
		t.Fatalf("WriteFile template: %v", err)
	}
	return configDir
}

func ensureAuditTestDiagnosticConfig(cfg *config.AppConfig) {
	if cfg == nil {
		return
	}
	if cfg.Audit.Diagnostics.TemplateBinding.Default == nil {
		cfg.Audit.Diagnostics.TemplateBinding.Default = map[string]string{}
	}
	for _, service := range []string{"cc", "cx", "gm", "openai", "anthropic", "gemini"} {
		if strings.TrimSpace(cfg.Audit.Diagnostics.TemplateBinding.Default[service]) == "" {
			cfg.Audit.Diagnostics.TemplateBinding.Default[service] = "unit-diagnostic"
		}
	}
}

func TestAuditTargetCredentialAPIHidesPlaintextKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "m1",
		RequestModel: "m1",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	body := `{"provider":"p1","service":"cc","channel":"101:demo","api_key":"sk-secret-1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/audit/targets/credential", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("credential update unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret-1234") {
		t.Fatalf("response leaked plaintext key: %s", rec.Body.String())
	}
	if !containsJSON(rec.Body.String(), `"key_last4":"1234"`) {
		t.Fatalf("response should expose key_last4 only: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/audit/targets/credential", strings.NewReader(`{"provider":"p1","service":"cc","channel":"101:demo"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("credential clear unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSON(rec.Body.String(), `"key_configured":false`) {
		t.Fatalf("clear response should show key unconfigured: %s", rec.Body.String())
	}
}

func TestAuditTargetsResponseDoesNotExposeAPIKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "p1",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "m1",
		RequestModel: "m1",
		Enabled:      true,
		APIKey:       "sk-secret-1234",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/targets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("targets unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret-1234") {
		t.Fatalf("targets leaked key: %s", rec.Body.String())
	}
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
	router := newAuditTestRouter(t, store, cfg)

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
		Output:    []byte(`{"tags":["request_error"]}`),
	}); err != nil {
		t.Fatalf("SaveDiagnosticRun bad: %v", err)
	}
	if err := store.SaveDiagnosticRun(&storage.DiagnosticRun{
		RunID:     "run-bad-terminal",
		Provider:  "OpenAI",
		Service:   "cc",
		Channel:   "101:demo",
		Model:     "gpt-4o",
		Status:    "failed_auth",
		CreatedAt: 1710000001,
		UpdatedAt: 1710000002,
		Input:     []byte(`{"group_id":"group-bad-terminal","request_model":"gpt-4o"}`),
		Output:    []byte(`{"run_status":"failed_auth","run_status_reason":"all diagnostic steps returned 401 unauthorized","tags":["request_error"]}`),
	}); err != nil {
		t.Fatalf("SaveDiagnosticRun bad terminal: %v", err)
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
	if err := store.SaveDiagnosticStep(&storage.DiagnosticStep{
		RunID:           "run-bad",
		StepIndex:       1,
		Prompt:          "ping",
		ResolvedPrompt:  "ping",
		ResponsePreview: "",
		ResultSummary:   "same_session",
		ErrorMessage:    "http 401",
		ExecutionMeta:   []byte(`{"step_name":"ping","error":"http 401"}`),
		CreatedAt:       1710000002,
	}); err != nil {
		t.Fatalf("SaveDiagnosticStep bad: %v", err)
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
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             "run-bad-terminal",
		AuthenticityScore: 0,
		ProtocolScore:     40,
		SSEScore:          100,
		Tags:              []byte(`["request_error"]`),
		CreatedAt:         1710000004,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore bad terminal: %v", err)
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
		Evidence:        []byte(`{"candidate_text":"pong","baseline_text":"official-pong"}`),
		CreatedAt:       1710000005,
	}); err != nil {
		t.Fatalf("SaveDiagnosticDimension: %v", err)
	}

	router := newAuditTestRouter(t, store, &config.AppConfig{DegradedWeight: 0.7})

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
	if compareResp.Data.Dimensions[0].DimensionKey != "authenticity_summary" || compareResp.Data.Dimensions[0].Evidence == nil {
		t.Fatalf("unexpected compare dimension payload: %+v", compareResp.Data.Dimensions[0])
	}
	if compareResp.Data.Summary.OverallScore <= 0 || compareResp.Data.Steps[0].Candidate.StepName != "ping" || compareResp.Data.Steps[0].Baseline == nil {
		t.Fatalf("unexpected compare summary/steps: %+v", compareResp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/run-bad", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics bad run unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var badDiagResp struct {
		Success bool                    `json:"success"`
		Data    auditDiagnosticResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &badDiagResp); err != nil {
		t.Fatalf("unmarshal bad diagnostics response: %v", err)
	}
	if !badDiagResp.Success ||
		badDiagResp.Data.Run.RunID != "run-bad" ||
		badDiagResp.Data.Run.RunStatus != "failed_auth" ||
		badDiagResp.Data.Run.RunStatusReason == "" {
		t.Fatalf("unexpected bad diagnostics payload: %+v", badDiagResp)
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
	if !latestResp.Data.Items[0].Usable || latestResp.Data.Items[0].FilterReason != "usable" {
		t.Fatalf("unexpected latest usability payload: %+v", latestResp.Data.Items[0])
	}
	if latestResp.Data.Items[0].CompareURL != "/api/audit/compare/run-1" {
		t.Fatalf("latest usable item should include compare url: %+v", latestResp.Data.Items[0])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/latest?provider=OpenAI&service=cc&channel=101:demo&include_filtered=1&limit=5", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest include_filtered unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var latestWithFilteredResp struct {
		Success bool                          `json:"success"`
		Data    auditDiagnosticLatestResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &latestWithFilteredResp); err != nil {
		t.Fatalf("unmarshal latest include_filtered response: %v", err)
	}
	if !latestWithFilteredResp.Success || len(latestWithFilteredResp.Data.Items) != 3 {
		t.Fatalf("unexpected latest include_filtered payload: %+v", latestWithFilteredResp)
	}
	if latestWithFilteredResp.Data.Items[0].Run.RunID != "run-bad-terminal" ||
		latestWithFilteredResp.Data.Items[0].Usable ||
		latestWithFilteredResp.Data.Items[0].FilterReason != "failed_auth" ||
		latestWithFilteredResp.Data.Items[0].Run.RunStatus != "failed_auth" {
		t.Fatalf("unexpected latest filtered first item: %+v", latestWithFilteredResp.Data.Items[0])
	}
	if latestWithFilteredResp.Data.Items[0].Run.RunStatusReason == "" {
		t.Fatalf("expected latest filtered first item to include run status reason: %+v", latestWithFilteredResp.Data.Items[0])
	}
	if latestWithFilteredResp.Data.Items[0].CompareURL != "/api/audit/compare/run-bad-terminal" {
		t.Fatalf("latest filtered first item should include compare url: %+v", latestWithFilteredResp.Data.Items[0])
	}
	if latestWithFilteredResp.Data.Items[1].Run.RunID != "run-bad" ||
		latestWithFilteredResp.Data.Items[1].Usable ||
		latestWithFilteredResp.Data.Items[1].FilterReason != "failed_auth" ||
		latestWithFilteredResp.Data.Items[1].Run.RunStatus != "failed_auth" {
		t.Fatalf("unexpected latest filtered second item: %+v", latestWithFilteredResp.Data.Items[1])
	}
	if latestWithFilteredResp.Data.Items[1].Run.RunStatusReason == "" {
		t.Fatalf("expected latest filtered second item to include run status reason: %+v", latestWithFilteredResp.Data.Items[1])
	}
	if latestWithFilteredResp.Data.Items[1].CompareURL != "/api/audit/compare/run-bad" {
		t.Fatalf("latest filtered second item should include compare url: %+v", latestWithFilteredResp.Data.Items[1])
	}
	if latestWithFilteredResp.Data.Items[2].Run.RunID != "run-1" || !latestWithFilteredResp.Data.Items[2].Usable {
		t.Fatalf("unexpected latest filtered third item: %+v", latestWithFilteredResp.Data.Items[2])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/audit/methodology", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("methodology unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var methodologyResp struct {
		Success bool                     `json:"success"`
		Data    auditMethodologyResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &methodologyResp); err != nil {
		t.Fatalf("unmarshal methodology response: %v", err)
	}
	if !methodologyResp.Success {
		t.Fatalf("unexpected methodology success flag: %+v", methodologyResp)
	}
	if methodologyResp.Data.Coverage.DoneRuns != 2 ||
		methodologyResp.Data.Coverage.DimensionRuns != 1 ||
		methodologyResp.Data.Coverage.DimensionRowCount != 1 ||
		methodologyResp.Data.Coverage.FailedAuthRuns != 2 ||
		methodologyResp.Data.Coverage.FailedRequestRuns != 0 ||
		methodologyResp.Data.Coverage.FilteredRuns != 2 {
		t.Fatalf("unexpected methodology coverage: %+v", methodologyResp.Data.Coverage)
	}
	if methodologyResp.Data.Runtime.ProbeCredentialMode != config.ProbeCredentialModeProbeFallback || methodologyResp.Data.Runtime.ProbeReady {
		t.Fatalf("unexpected methodology runtime: %+v", methodologyResp.Data.Runtime)
	}
}

func TestGetAuditDiagnosticHistoryFiltersAndPaginates(t *testing.T) {
	store := newAuditTestStore(t)
	runs := []*storage.DiagnosticRun{
		{RunID: "hist-1", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "done", CreatedAt: 100, UpdatedAt: 100},
		{RunID: "hist-2", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "failed_auth", CreatedAt: 200, UpdatedAt: 200},
		{RunID: "hist-3", Provider: "p1", Service: "anthropic", Channel: "ch1", Model: "m1", Status: "failed_request", CreatedAt: 300, UpdatedAt: 300},
		{RunID: "hist-other", Provider: "p1", Service: "openai", Channel: "ch1", Model: "m1", Status: "done", CreatedAt: 400, UpdatedAt: 400},
	}
	for _, run := range runs {
		if err := store.SaveDiagnosticRun(run); err != nil {
			t.Fatalf("SaveDiagnosticRun(%s): %v", run.RunID, err)
		}
		if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
			RunID:             run.RunID,
			AuthenticityScore: 80,
			ProtocolScore:     80,
			SSEScore:          80,
			CreatedAt:         run.CreatedAt,
		}); err != nil {
			t.Fatalf("SaveDiagnosticScore(%s): %v", run.RunID, err)
		}
	}

	router := newAuditTestRouter(t, store, &config.AppConfig{DegradedWeight: 0.7})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/history?provider=p1&service=anthropic&channel=ch1&model=m1&limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []auditDiagnosticLatestItemResponse `json:"items"`
			Meta  auditDiagnosticHistoryMetaResponse  `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success=false")
	}
	if resp.Data.Meta.Total != 3 || resp.Data.Meta.Count != 2 || resp.Data.Meta.Offset != 1 {
		t.Fatalf("bad meta: %+v", resp.Data.Meta)
	}
	if resp.Data.Items[0].Run.RunID != "hist-2" || resp.Data.Items[1].Run.RunID != "hist-1" {
		t.Fatalf("unexpected items: %+v", resp.Data.Items)
	}
	if resp.Data.Items[0].CompareURL != "/api/audit/compare/hist-2" {
		t.Fatalf("compare_url=%q", resp.Data.Items[0].CompareURL)
	}
}

func TestGetAuditChannelsIncludesManualBaselineTargets(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{
		{
			Provider:     "alan-官key直连",
			Service:      "anthropic",
			Channel:      "80:alan-官key直连",
			Model:        "claude-opus-4-8",
			RequestModel: "claude-opus-4-8",
			Enabled:      true,
			BaseURL:      "http://127.0.0.1:4000",
			Source:       "manual_baseline",
			APIKey:       "sk-test",
		},
		{
			Provider:     "alan-官key直连",
			Service:      "anthropic",
			Channel:      "80:alan-官key直连",
			Model:        "claude-sonnet-4-6",
			RequestModel: "claude-sonnet-4-6",
			Enabled:      true,
			Source:       "manual_baseline",
		},
	}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}

	router := newAuditTestRouter(t, store, &config.AppConfig{DegradedWeight: 0.7})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/channels", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsJSON(body, `"provider":"alan-官key直连"`) ||
		!containsJSON(body, `"channel":"80:alan-官key直连"`) ||
		!containsJSON(body, `"model":"claude-opus-4-8,claude-sonnet-4-6"`) ||
		!containsJSON(body, `"channelType":"official"`) {
		t.Fatalf("manual baseline channel missing: %s", body)
	}
}

func TestAuditModelStatusSeparatesSources(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	target := storage.AuditTarget{
		Provider:     "OpenAI",
		Service:      "cx",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
		APIKey:       "sk-status-1234",
	}
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{target}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	if err := store.SaveNewAPILogs([]storage.NewAPILog{{
		ID:               100,
		CreatedAt:        now,
		Type:             2,
		ModelName:        "gpt-4o",
		ChannelID:        101,
		PromptTokens:     10,
		CompletionTokens: 20,
		UseTime:          2,
	}}); err != nil {
		t.Fatalf("SaveNewAPILogs: %v", err)
	}
	if err := store.SaveRecord(&storage.ProbeRecord{
		Provider:  target.Provider,
		Service:   target.Service,
		Channel:   target.Channel,
		Model:     target.Model,
		Status:    0,
		SubStatus: storage.SubStatusAuthError,
		HttpCode:  401,
		Latency:   123,
		Timestamp: now,
	}); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}
	run := &storage.DiagnosticRun{
		RunID:     "run-model-status",
		Provider:  target.Provider,
		Service:   target.Service,
		Channel:   target.Channel,
		Model:     target.Model,
		Status:    "failed_auth",
		CreatedAt: now,
		UpdatedAt: now,
		Output:    []byte(`{"run_status":"failed_auth","run_status_reason":"all diagnostic steps returned 401 unauthorized","methodology_version":"quick-probe-v1"}`),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             run.RunID,
		AuthenticityScore: 0,
		ProtocolScore:     0,
		SSEScore:          0,
		Tags:              []byte(`["request_error"]`),
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore: %v", err)
	}

	router := newAuditTestRouter(t, store, &config.AppConfig{DegradedWeight: 0.7})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/model-status?provider=OpenAI&service=cx&channel=101:demo&window=24h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("model status unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool                     `json:"success"`
		Data    auditModelStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal model status: %v", err)
	}
	if !resp.Success || len(resp.Data.Items) != 1 {
		t.Fatalf("unexpected model status payload: %+v", resp)
	}
	item := resp.Data.Items[0]
	if item.Production.Source != "production_logs" || item.Production.Status != "ok" || item.Production.Total != 1 {
		t.Fatalf("unexpected production status: %+v", item.Production)
	}
	if !item.CredentialConfigured || item.CredentialLast4 != "1234" {
		t.Fatalf("unexpected credential status: %+v", item)
	}
	if item.TemplateProbe.Source != "template_probe" || item.TemplateProbe.Status != "unavailable" || item.TemplateProbe.SubStatus != "auth_error" {
		t.Fatalf("unexpected template probe status: %+v", item.TemplateProbe)
	}
	if item.QuickProbe.Source != "quick_probe" || item.QuickProbe.Status != "failed_auth" || item.QuickProbe.Usable {
		t.Fatalf("unexpected quick probe status: %+v", item.QuickProbe)
	}
	if item.QuickProbe.CompareURL != "/api/audit/compare/run-model-status" {
		t.Fatalf("quick probe status should expose compare url even when unusable: %+v", item.QuickProbe)
	}
}

func TestAuditDiagnosticSubmitUsesStoredChannelKey(t *testing.T) {
	store := newAuditTestStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "sk-channel-key" {
			t.Fatalf("Authorization = %q, want channel key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
		APIKey:       "sk-channel-key",
		BaseURL:      srv.URL,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}

	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://global-newapi-must-not-be-used.example.com",
			AccessToken: "sync-token-must-not-be-used",
			UserID:      "u1",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"OpenAI","service":"cc","channel":"101:demo","model":"gpt-4o","request_model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"run_id"`) {
		t.Fatalf("submit unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditDiagnosticSubmitUsesStoredBaseURLNotGlobalNewAPIBaseURL(t *testing.T) {
	store := newAuditTestStore(t)
	channelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "sk-channel-key" {
			t.Fatalf("Authorization = %q, want channel key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer channelServer.Close()

	globalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "global base url must not be used", http.StatusUnauthorized)
	}))
	defer globalServer.Close()

	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		BaseURL:      channelServer.URL,
		APIKey:       "sk-channel-key",
		Source:       "newapi_sync",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}

	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     globalServer.URL,
			AccessToken: "sync-token",
			UserID:      "1",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"alan-官key直连","service":"anthropic","channel":"80:alan-官key直连","model":"claude-opus-4-8","request_model":"claude-opus-4-8"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditDiagnosticSubmitRejectsMissingChannelKey(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://newapi.example.com",
			AccessToken: "sync-token",
			UserID:      "sync-user",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"OpenAI","service":"cc","channel":"101:demo","model":"gpt-4o","request_model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_credential") {
		t.Fatalf("submit should fail missing credential: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditDiagnosticSubmitRejectsMissingBaseURL(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		APIKey:       "sk-channel-key",
		Source:       "newapi_sync",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "http://global.example.invalid",
			AccessToken: "sync-token",
			UserID:      "1",
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"alan-官key直连","service":"anthropic","channel":"80:alan-官key直连","model":"claude-opus-4-8","request_model":"claude-opus-4-8"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_base_url") {
		t.Fatalf("submit should fail missing_base_url: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditModelStatusReturnsTemplateProbeSummary(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
		BaseURL:      "https://channel.example.com",
		APIKey:       "sk-channel-key",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	records := []*storage.ProbeRecord{
		{Provider: "OpenAI", Service: "cc", Channel: "101:demo", Model: "gpt-4o", Status: 1, Latency: 1200, Timestamp: now - 60},
		{Provider: "OpenAI", Service: "cc", Channel: "101:demo", Model: "gpt-4o", Status: 2, Latency: 2400, Timestamp: now - 120},
		{Provider: "OpenAI", Service: "cc", Channel: "101:demo", Model: "gpt-4o", Status: 0, SubStatus: storage.SubStatusResponseTimeout, HttpCode: 0, Timestamp: now - 180, ErrorDetail: "timeout"},
	}
	for _, record := range records {
		if err := store.SaveRecord(record); err != nil {
			t.Fatalf("SaveRecord: %v", err)
		}
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/model-status?provider=OpenAI&service=cc&channel=101:demo&window=24h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("model status unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool                     `json:"success"`
		Data    auditModelStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Success || len(resp.Data.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", resp)
	}
	item := resp.Data.Items[0]
	if item.TemplateProbe.Total != 3 || item.TemplateProbe.Success != 1 || item.TemplateProbe.Degraded != 1 || item.TemplateProbe.Timeout != 1 || item.TemplateProbe.NoResponse != 1 {
		t.Fatalf("unexpected template probe metrics: %+v", item.TemplateProbe)
	}
	if item.TemplateProbe.Availability < 66 || item.TemplateProbe.Availability > 67 {
		t.Fatalf("availability = %v, want about 66.7", item.TemplateProbe.Availability)
	}
	if resp.Data.Meta.Summary.TemplateProbeTotal != 3 || resp.Data.Meta.Summary.TemplateProbeSuccess != 2 {
		t.Fatalf("unexpected summary: %+v", resp.Data.Meta.Summary)
	}
}

func TestAuditModelStatusReturnsQuickProbeDimensionSummary(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
		BaseURL:      "https://channel.example.com",
		APIKey:       "sk-channel-key",
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	run := &storage.DiagnosticRun{
		RunID:     "run-model-status-summary",
		Provider:  "OpenAI",
		Service:   "cc",
		Channel:   "101:demo",
		Model:     "gpt-4o",
		Status:    "done",
		CreatedAt: now,
		UpdatedAt: now,
		Input:     []byte(`{"methodology_version":"quick-probe-v1"}`),
		Output:    []byte(`{"overall_score":88,"active_weight":20,"baseline_mode":"registered_baseline","methodology_version":"quick-probe-v1"}`),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:             run.RunID,
		AuthenticityScore: 88,
		ProtocolScore:     88,
		SSEScore:          88,
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("SaveDiagnosticScore: %v", err)
	}
	for _, dim := range []*storage.DiagnosticDimension{
		{RunID: run.RunID, DimensionKey: "model_match", Weight: 14, Score: 10, NormalizedScore: 1, Status: "pass", CreatedAt: now},
		{RunID: run.RunID, DimensionKey: "cutoff_match", Weight: 7, Score: 0, NormalizedScore: 0, Status: "fail", CreatedAt: now},
		{RunID: run.RunID, DimensionKey: "cache_ttl_consistency", Weight: 15, Score: 0, NormalizedScore: 0, Status: "skip", CreatedAt: now},
	} {
		if err := store.SaveDiagnosticDimension(dim); err != nil {
			t.Fatalf("SaveDiagnosticDimension: %v", err)
		}
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/model-status?provider=OpenAI&service=cc&channel=101:demo&window=24h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("model status unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool                     `json:"success"`
		Data    auditModelStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got := resp.Data.Items[0].QuickProbe
	if got.Status != "done" || got.Score != 88 || got.ActiveWeight != 20 {
		t.Fatalf("unexpected quick probe status: %+v", got)
	}
	if got.DimensionsTotal != 3 || got.DimensionsPass != 1 || got.DimensionsFail != 1 || got.DimensionsSkip != 1 {
		t.Fatalf("unexpected dimension summary: %+v", got)
	}
	if resp.Data.Meta.Summary.QuickProbeDone != 1 || resp.Data.Meta.Summary.BaselineCompared != 1 {
		t.Fatalf("unexpected meta summary: %+v", resp.Data.Meta.Summary)
	}
}

func TestAuditSyncStatusReportsProbeFallbackMode(t *testing.T) {
	store := newAuditTestStore(t)
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://newapi.example.com",
			AccessToken: "sync-token",
			UserID:      "sync-user",
		},
		Audit: config.AuditConfig{
			Diagnostics: config.DiagnosticsConfig{
				CredentialMode: config.ProbeCredentialModeProbeFallback,
			},
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/audit/newapi/sync/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !containsJSON(rec.Body.String(), `"probe_credential_mode":"probe_fallback"`) ||
		!containsJSON(rec.Body.String(), `"probe_ready":true`) ||
		!strings.Contains(rec.Body.String(), "回退使用同步凭证") {
		t.Fatalf("status should expose probe_fallback runtime warning: %s", rec.Body.String())
	}
}

func TestAuditDiagnosticBackfill(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "sk-backfill-channel-key" {
			t.Fatalf("Authorization = %q, want channel key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
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
			APIKey:       "sk-backfill-channel-key",
			BaseURL:      srv.URL,
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
	router := newAuditTestRouter(t, store, &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://global-newapi-must-not-be-used.example.com",
			AccessToken: "sync-token-must-not-be-used",
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

func TestAuditDiagnosticBackfillRejectsMissingChannelKey(t *testing.T) {
	store := newAuditTestStore(t)
	now := time.Now().Unix()
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cc",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}}); err != nil {
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
	}}); err != nil {
		t.Fatalf("SaveNewAPILogs: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:     "https://newapi.example.com",
			AccessToken: "sync-token",
			UserID:      "u1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics/backfill", strings.NewReader(`{"max_targets":1,"max_models_per_channel":1,"lookback_hours":24}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "missing_credential") {
		t.Fatalf("backfill should mark missing credential: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditTemplateProbeBackfillRequiresInlineProber(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "OpenAI",
		Service:      "cx",
		Channel:      "101:demo",
		Model:        "gpt-4o",
		RequestModel: "gpt-4o",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	router := newAuditTestRouter(t, store, &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL:          "https://newapi.example.com",
			ProbeAccessToken: "probe-token",
			ProbeUserID:      "probe-user",
		},
		Audit: config.AuditConfig{
			Diagnostics: config.DiagnosticsConfig{
				TemplateBinding: config.TemplateBindingConfig{
					Default: map[string]string{"cx": "cx-unit"},
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/audit/template-probes/backfill", strings.NewReader(`{"max_targets":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "模板探测器未初始化") {
		t.Fatalf("template probe backfill should require inline prober: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func containsJSON(body, needle string) bool {
	return len(body) > 0 && strings.Contains(body, needle)
}
