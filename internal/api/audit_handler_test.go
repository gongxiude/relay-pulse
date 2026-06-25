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
}

func TestAuditDiagnosticAndCompare(t *testing.T) {
	store := newAuditTestStore(t)
	run := &storage.DiagnosticRun{
		RunID:     "run-1",
		Provider:  "OpenAI",
		Service:   "cc",
		Channel:   "101:demo",
		Model:     "gpt-4o",
		Status:    "done",
		CreatedAt: 1710000000,
		UpdatedAt: 1710000001,
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
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

	router := newAuditTestRouter(store, &config.AppConfig{DegradedWeight: 0.7})

	for _, path := range []string{"/api/audit/diagnostics/run-1", "/api/audit/compare/run-1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !containsJSON(rec.Body.String(), `"run_id":"run-1"`) {
			t.Fatalf("%s unexpected: code=%d body=%s", path, rec.Code, rec.Body.String())
		}
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

func containsJSON(body, needle string) bool {
	return len(body) > 0 && strings.Contains(body, needle)
}
