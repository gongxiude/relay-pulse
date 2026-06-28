# Fix Diagnostic Target BaseURL Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复渠道检测使用正确目标地址的问题：quick-probe 和模板补洞必须同时使用 `audit_targets.base_url` 与 `audit_targets.api_key`，不能只使用渠道 key 后继续回退到全局 `NEWAPI_BASE_URL`。

**Architecture:** 将 `audit_targets` 作为主动检测的唯一目标配置来源，保存并保留渠道级 `base_url/api_key/source`。诊断运行时从数据库解析完整目标，缺少 `api_key` 返回 `missing_credential`，缺少 `base_url` 返回 `missing_base_url`；失败步骤必须保存 `request_url/status_code/response_text`，方便后续定位 401、404、协议错误和路由错误。

**Tech Stack:** Go, SQLite, PostgreSQL, Gin API, existing `internal/audit` diagnostic runner, existing `internal/newapi` channel sync, existing `templates/*diagnostic*.json`.

---

## Execution Workspace Boundary

本 plan **必须在独立 git worktree 中实现或复核**，禁止混入前端页面改造。

推荐命令：

```bash
git worktree add ../relay-pulse-diagnostic-target-baseurl -b fix-diagnostic-target-baseurl
cd ../relay-pulse-diagnostic-target-baseurl
git status --short --branch
```

如果当前修复已经在 `main` 上存在，执行者必须先核对这些提交：

```bash
git log --oneline --max-count=8
```

已知本地相关提交：

```text
8836b1e fix(audit): persist channel base url for diagnostics
c5a8bbe fix(audit): preserve manual baseline targets
```

执行者不得回滚这两个提交；如果在新 worktree 中缺失，需要按本 plan 补齐。

---

## Problem Statement

用户确认渠道 key 正确，但历史诊断仍出现：

```text
status = failed_auth
reason = all diagnostic steps returned 401 unauthorized
base_url = http://llm-relay.yuexin.domain:33033
```

同一个渠道 key 直接请求真实渠道地址成功：

```text
channel = 80:alan-官key直连
model = claude-opus-4-8
key_last4 = cbMi
base_url = http://72.61.77.104:4000
direct request status = 200
```

根因：

```text
诊断链路只读取了 audit_targets.api_key，
但 target.BaseURL 仍可能来自全局 NEWAPI_BASE_URL。
```

正确结果：

```text
run_id = diag-2eb44ad1-6445-40ba-9353-9ef408d9eab2
status = done
base_url = http://72.61.77.104:4000
credential_source = audit_targets.api_key
overall_score = 96.67
```

---

## Target Behavior

| 场景 | 期望 |
|---|---|
| 渠道配置了 `api_key` 和 `base_url` | quick-probe 使用这两个字段发起请求 |
| 渠道缺少 `api_key` | 返回 `missing_credential`，不发请求 |
| 渠道缺少 `base_url` | 返回 `missing_base_url`，不发请求 |
| `NEWAPI_BASE_URL` 存在 | 只用于同步 new-api 渠道和日志，不作为该渠道检测地址 |
| 旧同步任务执行 | 不清空手工保存的 `api_key/base_url/source` |
| 请求返回 401/403/404/5xx | diagnostic step 保存 `request_url/status_code/response_text/response_headers` |
| API 响应 | 不返回明文 key |

---

## File Structure

Modify or verify:

- `internal/storage/audit_models.go`
  - `AuditTarget` 包含 `BaseURL string`、`Source string`、`APIKey string`.
  - SQLite/PostgreSQL `audit_targets` 包含 `base_url/source/api_key`.
  - `ReplaceAuditTargets` 只删除 `source=''` 或 `source='newapi_sync'` 的同步目标。
  - `ReplaceAuditTargets` 保留已有 `api_key/base_url/source`.

- `internal/storage/sqlite.go`
  - SQLite 初始化调用 `ensureAuditTargetsAPIKeyColumn`、`ensureAuditTargetsBaseURLColumn`、`ensureAuditTargetsSourceColumn`.

- `internal/storage/postgres.go`
  - PostgreSQL 初始化调用 `ensureAuditTargetsAPIKeyColumn`、`ensureAuditTargetsBaseURLColumn`、`ensureAuditTargetsSourceColumn`.

- `internal/audit/targets.go`
  - 从 `new-api` 同步渠道时把 `base_url` 写入 `ChannelSpec` 和 `AuditTarget`.
  - 同步生成目标时标记 `Source: "newapi_sync"`.

- `internal/newapi/types.go`
  - `Channel` 解析 `base_url`.

- `internal/newapi/sync_channels.go`
  - 把 `Channel.BaseURL` 传给 `audit.ChannelSpec`.

- `internal/api/audit_handler.go`
  - `PostAuditDiagnosticSubmit` 从 `audit_targets` 读取完整目标。
  - `PostAuditDiagnosticBackfill` 使用每个 target 的 `base_url/api_key`.
  - `PostAuditTemplateProbeBackfill` 使用每个 target 的 `base_url/api_key`.

- `internal/audit/diagnostic_runner.go`
  - `DiagnosticRun.Input` 记录 `base_url` 和 `credential_source`。
  - 失败步骤也保存 execution meta。

- `internal/audit/request_executor.go`
  - 非 2xx 响应返回前保留 `StatusCode/RequestURL/RequestBody/ResponseHeaders/ResponseText`.

Tests:

- `internal/storage/audit_models_test.go`
- `internal/newapi/sync_channels_test.go`
- `internal/api/audit_handler_test.go`
- `internal/audit/diagnostic_runner_test.go`

---

## Task 1: Verify Storage Schema And Preservation

**Files:**
- Modify: `internal/storage/audit_models.go`
- Modify: `internal/storage/sqlite.go`
- Modify: `internal/storage/postgres.go`
- Test: `internal/storage/audit_models_test.go`

- [ ] **Step 1: Confirm `AuditTarget` fields**

Expected struct:

```go
type AuditTarget struct {
	Provider     string `json:"provider"`
	Service      string `json:"service"`
	Channel      string `json:"channel"`
	Model        string `json:"model"`
	RequestModel string `json:"request_model"`
	Group        string `json:"group"`
	Weight       int    `json:"weight"`
	Priority     int    `json:"priority"`
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url,omitempty"`
	Source       string `json:"source,omitempty"`
	APIKey       string `json:"-"`
}
```

- [ ] **Step 2: Confirm SQLite schema**

`audit_targets` create SQL must include:

```sql
base_url TEXT NOT NULL DEFAULT '',
source TEXT NOT NULL DEFAULT 'newapi_sync',
api_key TEXT NOT NULL DEFAULT '',
```

- [ ] **Step 3: Confirm PostgreSQL schema**

`audit_targets` create SQL must include:

```sql
base_url TEXT NOT NULL DEFAULT '',
source TEXT NOT NULL DEFAULT 'newapi_sync',
api_key TEXT NOT NULL DEFAULT '',
```

- [ ] **Step 4: Confirm migrations are called**

SQLite init must call:

```go
if err := s.ensureAuditTargetsAPIKeyColumn(ctx); err != nil {
	return err
}
if err := s.ensureAuditTargetsBaseURLColumn(ctx); err != nil {
	return err
}
if err := s.ensureAuditTargetsSourceColumn(ctx); err != nil {
	return err
}
```

PostgreSQL init must call the same three helpers.

- [ ] **Step 5: Add preservation test**

Add or verify this test:

```go
func TestAuditTargetsPreserveCredentialBaseURLAndSourceOnReplace(t *testing.T) {
	store := newTestStore(t)
	first := []AuditTarget{{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		BaseURL:      "http://72.61.77.104:4000",
		Source:       "self_reported_official",
		APIKey:       "sk-channel-key",
	}}
	if err := store.ReplaceAuditTargets(first); err != nil {
		t.Fatalf("ReplaceAuditTargets first: %v", err)
	}

	second := []AuditTarget{{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		Enabled:      true,
		Source:       "newapi_sync",
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
	got := targets[0]
	if got.APIKey != "sk-channel-key" {
		t.Fatalf("APIKey = %q, want preserved key", got.APIKey)
	}
	if got.BaseURL != "http://72.61.77.104:4000" {
		t.Fatalf("BaseURL = %q, want preserved base url", got.BaseURL)
	}
	if got.Source != "self_reported_official" {
		t.Fatalf("Source = %q, want self_reported_official", got.Source)
	}
}
```

- [ ] **Step 6: Run storage tests**

```bash
go test ./internal/storage
```

Expected:

```text
ok  	monitor/internal/storage
```

- [ ] **Step 7: Commit**

```bash
git add internal/storage/audit_models.go internal/storage/sqlite.go internal/storage/postgres.go internal/storage/audit_models_test.go
git commit -m "fix(audit): preserve diagnostic target base urls"
```

---

## Task 2: Ensure NewAPI Sync Carries Channel BaseURL

**Files:**
- Modify: `internal/newapi/types.go`
- Modify: `internal/newapi/sync_channels.go`
- Modify: `internal/audit/targets.go`
- Test: `internal/newapi/sync_channels_test.go`
- Test: `internal/audit/targets_test.go`

- [ ] **Step 1: Verify `newapi.Channel` has BaseURL**

Expected:

```go
type Channel struct {
	ID           int             `json:"id"`
	Type         int             `json:"type"`
	Status       int             `json:"status"`
	Name         string          `json:"name"`
	Weight       *uint           `json:"weight"`
	Priority     *int64          `json:"priority"`
	BaseURL      *string         `json:"base_url"`
	Models       string          `json:"models"`
	Group        string          `json:"group"`
	ModelMapping *string         `json:"model_mapping"`
	Tag          *string         `json:"tag"`
	Other        json.RawMessage `json:"other,omitempty"`
}
```

- [ ] **Step 2: Verify `audit.ChannelSpec` has BaseURL**

Expected:

```go
type ChannelSpec struct {
	ID           int
	Type         int
	Status       int
	Name         string
	BaseURL      string
	Models       string
	Group        string
	Weight       *uint
	Priority     *int64
	ModelMapping *string
	Other        json.RawMessage
}
```

- [ ] **Step 3: Verify sync maps BaseURL**

`internal/newapi/sync_channels.go` must map:

```go
BaseURL: derefString(ch.BaseURL),
```

If no helper exists, add:

```go
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
```

- [ ] **Step 4: Verify target builder sets BaseURL and Source**

`BuildAuditTargets` must set:

```go
BaseURL: strings.TrimSpace(ch.BaseURL),
Source:  "newapi_sync",
```

- [ ] **Step 5: Add sync test**

Add:

```go
func TestSyncChannelsWritesBaseURLToTargets(t *testing.T) {
	store, err := storage.NewSQLiteStorage(t.TempDir() + "/audit.db")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	baseURL := "http://72.61.77.104:4000"
	got, err := SyncChannels(context.Background(), fakeLister{list: &ChannelList{
		Items: []Channel{{
			ID:      80,
			Type:    14,
			Status:  1,
			Name:    "alan-官key直连",
			BaseURL: &baseURL,
			Models:  "claude-opus-4-8",
			Group:   "anthropic",
			Other:   []byte(`{"provider":"alan-官key直连","service":"anthropic"}`),
		}},
	}}, store)
	if err != nil {
		t.Fatalf("SyncChannels: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got len = %d, want 1", len(got))
	}
	targets, err := store.ListAuditTargets()
	if err != nil {
		t.Fatalf("ListAuditTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(targets))
	}
	if targets[0].BaseURL != baseURL {
		t.Fatalf("BaseURL = %q, want %q", targets[0].BaseURL, baseURL)
	}
	if targets[0].Source != "newapi_sync" {
		t.Fatalf("Source = %q, want newapi_sync", targets[0].Source)
	}
}
```

- [ ] **Step 6: Run sync tests**

```bash
go test ./internal/newapi ./internal/audit
```

Expected:

```text
ok  	monitor/internal/newapi
ok  	monitor/internal/audit
```

- [ ] **Step 7: Commit**

```bash
git add internal/newapi/types.go internal/newapi/sync_channels.go internal/audit/targets.go internal/newapi/sync_channels_test.go internal/audit/targets_test.go
git commit -m "fix(audit): sync channel base urls into targets"
```

---

## Task 3: Use Stored BaseURL In Diagnostic Submit And Backfill

**Files:**
- Modify: `internal/api/audit_handler.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Enforce target base URL in submit**

`PostAuditDiagnosticSubmit` must set:

```go
target.BaseURL = strings.TrimSpace(storedTarget.BaseURL)
if target.BaseURL == "" {
	apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "missing_base_url: 当前渠道未配置检测 base_url")
	return
}
target.AccessToken = strings.TrimSpace(storedTarget.APIKey)
target.UserID = strings.TrimSpace(appCfg.NewAPI.UserID)
target.CredentialSource = "audit_targets.api_key"
```

Do not use this fallback in formal diagnostics:

```go
firstNonEmptyString(storedTarget.BaseURL, appCfg.NewAPI.BaseURL)
```

Rationale: fallback hides configuration errors and can recreate the exact bug: correct key sent to wrong base URL.

- [ ] **Step 2: Enforce target base URL in diagnostic backfill**

For each selected target:

```go
runTarget.BaseURL = strings.TrimSpace(target.BaseURL)
if runTarget.BaseURL == "" {
	item := auditDiagnosticBackfillItemResponse{
		Provider: runTarget.Provider,
		Service:  runTarget.Service,
		Channel:  runTarget.Channel,
		Model:    runTarget.Model,
		Status:   "failed",
		Error:    "missing_base_url: 当前渠道未配置检测 base_url",
	}
	failed++
	items = append(items, item)
	continue
}
```

- [ ] **Step 3: Enforce target base URL in template probe backfill**

Before `BuildTemplateProbeConfig`:

```go
if strings.TrimSpace(target.BaseURL) == "" {
	item.Status = "failed"
	item.Error = "missing_base_url: 当前渠道未配置检测 base_url"
	failed++
	items = append(items, item)
	continue
}
```

Then pass:

```go
BaseURL: strings.TrimSpace(target.BaseURL),
```

- [ ] **Step 4: Add submit test for stored base URL**

Test server must fail if global base URL is used:

```go
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
```

- [ ] **Step 5: Add missing base URL test**

```go
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
```

- [ ] **Step 6: Run API tests**

```bash
go test ./internal/api
```

Expected:

```text
ok  	monitor/internal/api
```

- [ ] **Step 7: Commit**

```bash
git add internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "fix(audit): require stored base url for diagnostics"
```

---

## Task 4: Preserve Failure Evidence In Diagnostic Steps

**Files:**
- Modify: `internal/audit/diagnostic_runner.go`
- Test: `internal/audit/diagnostic_runner_test.go`

- [ ] **Step 1: Fix failed request metadata**

When `executeOpenAIChat` returns `resp != nil` and `err != nil`, save response metadata, not only:

```go
"step_name": stepDef.Name,
"error": err.Error(),
```

Expected failure meta:

```go
meta := map[string]any{
	"step_name": stepDef.Name,
	"error":     err.Error(),
}
if resp != nil {
	meta["status_code"] = resp.StatusCode
	meta["latency_ms"] = resp.LatencyMs
	meta["ttft_ms"] = resp.TTFTMs
	meta["stream_chunks"] = resp.StreamChunks
	meta["response_model"] = resp.ResponseModel
	meta["finish_reason"] = resp.FinishReason
	meta["usage"] = resp.Usage
	meta["request_url"] = resp.RequestURL
	meta["request_body"] = resp.RequestBody
	meta["response_text"] = resp.ResponseText
	meta["response_headers"] = resp.ResponseHeaders
}
step.ExecutionMeta = mustJSON(meta)
step.ResponsePreview = previewText(resp.ResponseText)
```

Guard nil:

```go
if resp == nil {
	step.ExecutionMeta = mustJSON(meta)
}
```

- [ ] **Step 2: Add failing HTTP test**

Add:

```go
func TestDiagnosticRunnerStoresHTTPErrorEvidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test-Error", "auth")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	store := newMemoryDiagnosticStore()
	runner := NewDiagnosticRunner(srv.Client())
	_, err := runner.Run(context.Background(), DiagnosticTarget{
		Provider:         "p1",
		Service:          "anthropic",
		Channel:          "80:demo",
		Model:            "claude-opus-4-8",
		RequestModel:     "claude-opus-4-8",
		BaseURL:          srv.URL,
		AccessToken:      "sk-bad",
		UserID:           "1",
		CredentialSource: "audit_targets.api_key",
		Template:         diagnosticTestTemplate(),
		TemplateName:     "test-template",
	}, store)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.steps) == 0 {
		t.Fatal("expected saved steps")
	}
	var meta map[string]any
	if err := json.Unmarshal(store.steps[0].ExecutionMeta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if got := int(meta["status_code"].(float64)); got != http.StatusUnauthorized {
		t.Fatalf("status_code = %d, want 401", got)
	}
	if got := meta["request_url"].(string); got != srv.URL+"/v1/chat/completions" {
		t.Fatalf("request_url = %q", got)
	}
	if !strings.Contains(meta["response_text"].(string), "bad key") {
		t.Fatalf("response_text missing error body: %+v", meta)
	}
}
```

Use existing test helper style in `internal/audit/diagnostic_runner_test.go`; if helper names differ, add local helpers in that file with the exact same request template contract:

```go
func diagnosticTestTemplate() *config.ProbeTemplate {
	return &config.ProbeTemplate{
		URL:            "{{BASE_URL}}/v1/chat/completions",
		Method:         "POST",
		Headers:        map[string]string{"Authorization": "{{API_KEY}}", "Content-Type": "application/json"},
		BodyRaw:        json.RawMessage(`{"model":"{{MODEL}}","messages":[],"stream":true}`),
		RequestFamily:  "openai_chat",
		ResponseParser: "openai_chat_sse",
		OverridePaths: map[string]string{
			"model":    "$.model",
			"messages": "$.messages",
			"stream":   "$.stream",
		},
	}
}
```

- [ ] **Step 3: Run audit tests**

```bash
go test ./internal/audit
```

Expected:

```text
ok  	monitor/internal/audit
```

- [ ] **Step 4: Commit**

```bash
git add internal/audit/diagnostic_runner.go internal/audit/diagnostic_runner_test.go
git commit -m "fix(audit): store diagnostic error evidence"
```

---

## Task 5: Manual Verification With Real Sample

**Files:**
- No code edits unless verification reveals a concrete bug.

- [ ] **Step 1: Start local server on 18080**

Use a foreground session so the server remains alive:

```bash
go build -o /tmp/relay-pulse-server ./cmd/server
PORT=18080 /tmp/relay-pulse-server config.yaml
```

Expected log:

```text
监测服务已启动 ... web_ui=http://localhost:18080
```

- [ ] **Step 2: Confirm target config**

```bash
sqlite3 monitor.db "SELECT provider, service, channel, model, CASE WHEN api_key='' THEN 0 ELSE 1 END AS has_key, substr(api_key, length(api_key)-3, 4) AS last4, base_url FROM audit_targets WHERE provider='alan-官key直连' AND channel='80:alan-官key直连' AND model='claude-opus-4-8';"
```

Expected:

```text
alan-官key直连|anthropic|80:alan-官key直连|claude-opus-4-8|1|cbMi|http://72.61.77.104:4000
```

- [ ] **Step 3: Submit diagnostic**

```bash
curl -sS -m 90 -X POST http://127.0.0.1:18080/api/audit/diagnostics \
  -H 'Content-Type: application/json' \
  -d '{"provider":"alan-官key直连","service":"anthropic","channel":"80:alan-官key直连","model":"claude-opus-4-8","request_model":"claude-opus-4-8"}'
```

Expected:

```json
{"success":true,"data":{"run_id":"diag-..."}}
```

- [ ] **Step 4: Verify run base_url and status**

Replace `diag-...` with the returned run id:

```bash
curl -sS http://127.0.0.1:18080/api/audit/diagnostics/diag-... \
  | jq '{status:.data.run.status, base_url:.data.run.input.base_url, credential_source:.data.run.input.credential_source, score:.data.score.overall_score}'
```

Expected:

```json
{
  "status": "done",
  "base_url": "http://72.61.77.104:4000",
  "credential_source": "audit_targets.api_key",
  "score": 90
}
```

`score` 不要求固定等于 `96.67`，但必须大于 `0`，且不得是 `failed_auth`。

- [ ] **Step 5: Verify no plaintext key leaks**

```bash
curl -sS http://127.0.0.1:18080/api/audit/targets | rg 'sk-' && echo "LEAK" || echo "OK"
```

Expected:

```text
OK
```

- [ ] **Step 6: Run focused tests**

```bash
go test ./internal/storage ./internal/newapi ./internal/audit ./internal/api
```

Expected:

```text
ok  	monitor/internal/storage
ok  	monitor/internal/newapi
ok  	monitor/internal/audit
ok  	monitor/internal/api
```

- [ ] **Step 7: Commit verification fixes**

Only if code changed during verification:

```bash
git add internal docs
git commit -m "test(audit): verify diagnostic target base url"
```

Do not create an empty commit.

---

## Acceptance Criteria

| ID | Acceptance |
|---|---|
| A1 | `audit_targets` stores `base_url/source/api_key` in SQLite and PostgreSQL |
| A2 | new-api channel sync writes channel `base_url` into `audit_targets` |
| A3 | sync preserves manually configured `api_key/base_url/source` |
| A4 | quick-probe submit uses `audit_targets.base_url`, not global `NEWAPI_BASE_URL` |
| A5 | diagnostic backfill uses `audit_targets.base_url` |
| A6 | template probe backfill uses `audit_targets.base_url` |
| A7 | missing target key returns `missing_credential` |
| A8 | missing target base URL returns `missing_base_url` |
| A9 | failed HTTP diagnostic steps store request URL, status code, response headers and response body |
| A10 | real sample `alan-官key直连 / 80 / claude-opus-4-8` runs against `http://72.61.77.104:4000` and does not fail with 401 |

---

## Out Of Scope

- 不实现前端 key 管理页面。
- 不把 key 明文返回给 API 或前端。
- 不修改 `new-api`。
- 不从 `new-api` 数据库读取 key。
- 不改变普通 monitor 配置体系。

---

## Self-Review

Spec coverage:

- 修复正确 key 仍 401 的根因：Task 3 和 Task 5。
- 保留渠道 base_url：Task 1 和 Task 2。
- 防止同步覆盖手工配置：Task 1。
- 失败证据可追踪：Task 4。
- 本地真实样本验收：Task 5。

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No `implement later`.
- No `Similar to`.

Type consistency:

- Stored field: `AuditTarget.BaseURL`.
- Database column: `audit_targets.base_url`.
- Credential field: `AuditTarget.APIKey`.
- Missing key marker: `missing_credential`.
- Missing base URL marker: `missing_base_url`.
