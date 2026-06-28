# Diagnostic History Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 点击服务商详情页模型行里的“检测历史”后，进入独立的检测历史页面，展示该通道、该模型的所有检测样本；每条样本提供“查看实际情况”链接，进入 `/detect/compare/:runId`。

**Architecture:** 新增一个独立前端页面 `DetectHistoryPage` 和路由 `/detect/history`，不要再复用 `DetectPage` 检测方法页承载历史列表。后端新增分页历史接口 `/api/audit/diagnostics/history`，复用现有诊断 run/score/usable 分类逻辑，但支持按 `provider/service/channel/model` 过滤和分页。服务商详情页的“检测历史”入口改为 `/detect/history?provider=...&service=...&channel=...&model=...`。

**Tech Stack:** Go + Gin API, SQLite/PostgreSQL storage, React + TypeScript + React Router, Tailwind CSS, Vite.

---

## Scope And Correction

上一版实现把“检测历史”导向 `/detect/compare?provider=...`，但 `/detect/compare` 当前承担的是“检测方法/最近证据入口”，不是独立历史页。这个方向不符合预期。

本计划替换该方向：

```text
服务商详情页 /p/:provider
  模型行：检测历史
    ↓
/detect/history?provider=...&service=...&channel=...&model=...
  检测历史专页：展示所有检测样本
    每条：查看实际情况
      ↓
/detect/compare/:runId
```

不再把模型历史入口指到：

```text
/detect/compare?provider=...&service=...&channel=...&model=...
```

`/detect/compare/:runId` 继续保留为单次检测实际情况页。

---

## File Structure

Create:

- `frontend/src/pages/DetectHistoryPage.tsx`
  - 独立检测历史页面。
  - 读取 URL query：`provider`、`service`、`channel`、`model`、`offset`、`limit`。
  - 调用新 hook 获取历史列表。
  - 使用现有 `Header`，使用与 `DetectPage` / `ProviderPage` 一致的表格和 badge 风格。

- `frontend/src/hooks/useAuditDiagnosticHistory.ts`
  - 调用 `/api/audit/diagnostics/history`。
  - 返回 `items`、`meta`、`loading`、`error`。

Modify:

- `frontend/src/router.tsx`
  - 新增 lazy import `DetectHistoryPage`。
  - 新增路由 `detect/history`。

- `frontend/src/types/audit.ts`
  - 新增 `AuditDiagnosticHistoryResponse` 和 `AuditDiagnosticHistoryMeta` 类型。
  - 复用 `AuditDiagnosticLatestItem` 作为历史 item 类型。

- `frontend/src/pages/ProviderPage.tsx`
  - `historyUrl` 从 `/detect/compare?...` 改为 `/detect/history?...`。
  - “检测历史”按钮打开独立页面。

- `internal/storage/audit_models.go`
  - `DiagnosticRunFilter` 增加 `Offset int`。
  - `ListDiagnosticRuns` SQLite/PostgreSQL 增加 `OFFSET` 支持。
  - 新增 `CountDiagnosticRunsFiltered(filter DiagnosticRunFilter) (int, error)` SQLite/PostgreSQL 实现。

- `internal/api/audit_handler.go`
  - `auditReadStore` 增加 `CountDiagnosticRunsFiltered(storage.DiagnosticRunFilter) (int, error)`。
  - 新增 `GetAuditDiagnosticHistory` handler。
  - 复用一个 helper 构建 `auditDiagnosticLatestItemResponse`，避免和 `GetAuditDiagnosticLatest` 重复。

- `internal/api/audit_types.go`
  - 新增 `auditDiagnosticHistoryResponse` 和 `auditDiagnosticHistoryMetaResponse`。

- `internal/api/server.go`
  - 注册 `GET /api/audit/diagnostics/history`。

Tests:

- `internal/storage/audit_models_test.go`
  - 覆盖 `Offset` 和 filtered count。

- `internal/api/audit_handler_test.go`
  - 覆盖 history API 按 provider/service/channel/model 过滤、分页、返回 compare URL。

No changes:

- 不改 quick-probe 执行逻辑。
- 不改 `/detect/compare/:runId` 页面。
- 不改 new-api 同步逻辑。

---

### Task 1: Add Storage Pagination And Filtered Count

**Files:**
- Modify: `internal/storage/audit_models.go`
- Test: `internal/storage/audit_models_test.go`

- [ ] **Step 1: Extend `DiagnosticRunFilter`**

In `internal/storage/audit_models.go`, replace:

```go
type DiagnosticRunFilter struct {
	Provider string
	Service  string
	Channel  string
	Model    string
	Status   string
	Limit    int
}
```

with:

```go
type DiagnosticRunFilter struct {
	Provider string
	Service  string
	Channel  string
	Model    string
	Status   string
	Limit    int
	Offset   int
}
```

- [ ] **Step 2: Add shared where-clause builders**

In `internal/storage/audit_models.go`, add these helpers near `DiagnosticRunFilter`:

```go
func sqliteDiagnosticRunWhere(filter DiagnosticRunFilter) ([]string, []any) {
	args := make([]any, 0, 5)
	clauses := make([]string, 0, 5)
	if strings.TrimSpace(filter.Provider) != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.Service) != "" {
		clauses = append(clauses, "service = ?")
		args = append(args, strings.TrimSpace(filter.Service))
	}
	if strings.TrimSpace(filter.Channel) != "" {
		clauses = append(clauses, "channel = ?")
		args = append(args, strings.TrimSpace(filter.Channel))
	}
	if strings.TrimSpace(filter.Model) != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, strings.TrimSpace(filter.Model))
	}
	if strings.TrimSpace(filter.Status) != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, strings.TrimSpace(filter.Status))
	}
	return clauses, args
}

func postgresDiagnosticRunWhere(filter DiagnosticRunFilter) ([]string, []any, int) {
	args := make([]any, 0, 5)
	clauses := make([]string, 0, 5)
	argIndex := 1
	if strings.TrimSpace(filter.Provider) != "" {
		clauses = append(clauses, fmt.Sprintf("provider = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Provider))
		argIndex++
	}
	if strings.TrimSpace(filter.Service) != "" {
		clauses = append(clauses, fmt.Sprintf("service = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Service))
		argIndex++
	}
	if strings.TrimSpace(filter.Channel) != "" {
		clauses = append(clauses, fmt.Sprintf("channel = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Channel))
		argIndex++
	}
	if strings.TrimSpace(filter.Model) != "" {
		clauses = append(clauses, fmt.Sprintf("model = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Model))
		argIndex++
	}
	if strings.TrimSpace(filter.Status) != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, strings.TrimSpace(filter.Status))
		argIndex++
	}
	return clauses, args, argIndex
}
```

- [ ] **Step 3: Update SQLite `ListDiagnosticRuns`**

In `func (s *SQLiteStorage) ListDiagnosticRuns`, replace the manual `clauses` and `args` construction with:

```go
	clauses, args := sqliteDiagnosticRunWhere(filter)
```

Then replace:

```go
	query += ` LIMIT ?`
	args = append(args, limit)
```

with:

```go
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
```

- [ ] **Step 4: Add SQLite filtered count**

Add after SQLite `ListDiagnosticRuns`:

```go
func (s *SQLiteStorage) CountDiagnosticRunsFiltered(filter DiagnosticRunFilter) (int, error) {
	ctx := s.effectiveCtx()
	clauses, args := sqliteDiagnosticRunWhere(filter)
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务列表失败: %w", err)
	}
	return count, nil
}
```

- [ ] **Step 5: Update PostgreSQL `ListDiagnosticRuns`**

In `func (s *PostgresStorage) ListDiagnosticRuns`, replace the manual `clauses` / `args` / `argIndex` construction with:

```go
	clauses, args, argIndex := postgresDiagnosticRunWhere(filter)
```

Then replace:

```go
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit)
```

with:

```go
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)
```

- [ ] **Step 6: Add PostgreSQL filtered count**

Add after PostgreSQL `ListDiagnosticRuns`:

```go
func (s *PostgresStorage) CountDiagnosticRunsFiltered(filter DiagnosticRunFilter) (int, error) {
	ctx := s.effectiveCtx()
	clauses, args, _ := postgresDiagnosticRunWhere(filter)
	query := `SELECT COUNT(*) FROM diagnostic_runs`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	var count int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计诊断任务列表失败 (PostgreSQL): %w", err)
	}
	return count, nil
}
```

- [ ] **Step 7: Add storage regression test**

In `internal/storage/audit_models_test.go`, add:

```go
func TestDiagnosticRunFilterOffsetAndCount(t *testing.T) {
	store := newTestSQLiteStorage(t)
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
```

If `newTestSQLiteStorage` is not the helper name in this file, use the existing helper that creates SQLite storage in the current test file.

- [ ] **Step 8: Run storage tests**

Run:

```bash
go test ./internal/storage
```

Expected:

```text
ok  	monitor/internal/storage
```

---

### Task 2: Add Diagnostic History API

**Files:**
- Modify: `internal/api/audit_handler.go`
- Modify: `internal/api/audit_types.go`
- Modify: `internal/api/server.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Extend `auditReadStore` interface**

In `internal/api/audit_handler.go`, replace:

```go
	CountDiagnosticRuns(string) (int, error)
```

with:

```go
	CountDiagnosticRuns(string) (int, error)
	CountDiagnosticRunsFiltered(storage.DiagnosticRunFilter) (int, error)
```

- [ ] **Step 2: Add history response types**

In `internal/api/audit_types.go`, after `auditDiagnosticLatestMetaResponse`, add:

```go
type auditDiagnosticHistoryResponse struct {
	Items []auditDiagnosticLatestItemResponse `json:"items"`
	Meta  auditDiagnosticHistoryMetaResponse  `json:"meta"`
}

type auditDiagnosticHistoryMetaResponse struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	Count      int    `json:"count"`
	Total      int    `json:"total"`
	Provider   string `json:"provider,omitempty"`
	Service    string `json:"service,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status,omitempty"`
	NextOffset *int   `json:"next_offset,omitempty"`
}
```

- [ ] **Step 3: Extract item builder from latest handler**

In `internal/api/audit_handler.go`, add:

```go
func buildAuditDiagnosticListItem(store auditReadStore, run *storage.DiagnosticRun) (auditDiagnosticLatestItemResponse, error) {
	score, err := store.GetDiagnosticScore(run.RunID)
	if err != nil {
		return auditDiagnosticLatestItemResponse{}, err
	}
	usable, filterReason, classifiedSteps, err := classifyDiagnosticRun(store, run, score)
	if err != nil {
		return auditDiagnosticLatestItemResponse{}, err
	}
	item := auditDiagnosticLatestItemResponse{
		Run:          buildAuditDiagnosticRun(run),
		Score:        buildAuditDiagnosticScore(run, score),
		Usable:       usable,
		FilterReason: filterReason,
		CompareURL:   "/api/audit/compare/" + run.RunID,
	}
	if !usable && strings.TrimSpace(item.Run.RunStatus) == "" {
		item.Run.RunStatus = filterReason
	}
	if !usable && strings.TrimSpace(item.Run.RunStatusReason) == "" {
		item.Run.RunStatusReason = summarizeDiagnosticFailureReason(classifiedSteps, filterReason)
	}
	return item, nil
}
```

- [ ] **Step 4: Refactor `GetAuditDiagnosticLatest` to use helper**

Inside `GetAuditDiagnosticLatest`, replace the per-run score/classify/item construction with:

```go
		item, err := buildAuditDiagnosticListItem(store, run)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		if !includeFiltered && !item.Usable {
			continue
		}
		items = append(items, item)
```

Keep the existing `if len(items) >= limit { break }`.

- [ ] **Step 5: Add parse helpers for limit and offset**

In `internal/api/audit_handler.go`, add:

```go
func parseAuditHistoryLimit(c *gin.Context) (int, bool) {
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "limit 参数错误")
			return 0, false
		}
		if value > 200 {
			value = 200
		}
		limit = value
	}
	return limit, true
}

func parseAuditHistoryOffset(c *gin.Context) (int, bool) {
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			apiError(c, http.StatusBadRequest, ErrCodeInvalidParam, "offset 参数错误")
			return 0, false
		}
		offset = value
	}
	return offset, true
}
```

- [ ] **Step 6: Add `GetAuditDiagnosticHistory` handler**

In `internal/api/audit_handler.go`, add:

```go
func (h *Handler) GetAuditDiagnosticHistory(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	limit, ok := parseAuditHistoryLimit(c)
	if !ok {
		return
	}
	offset, ok := parseAuditHistoryOffset(c)
	if !ok {
		return
	}

	filter := storage.DiagnosticRunFilter{
		Provider: strings.TrimSpace(c.Query("provider")),
		Service:  strings.TrimSpace(c.Query("service")),
		Channel:  strings.TrimSpace(c.Query("channel")),
		Model:    strings.TrimSpace(c.Query("model")),
		Status:   strings.TrimSpace(c.Query("status")),
		Limit:    limit,
		Offset:   offset,
	}

	total, err := store.CountDiagnosticRunsFiltered(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	runs, err := store.ListDiagnosticRuns(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	items := make([]auditDiagnosticLatestItemResponse, 0, len(runs))
	for _, run := range runs {
		item, err := buildAuditDiagnosticListItem(store, run)
		if err != nil {
			apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
			return
		}
		items = append(items, item)
	}

	var nextOffset *int
	if offset+len(items) < total {
		next := offset + len(items)
		nextOffset = &next
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": auditDiagnosticHistoryResponse{
			Items: items,
			Meta: auditDiagnosticHistoryMetaResponse{
				Limit:      limit,
				Offset:     offset,
				Count:      len(items),
				Total:      total,
				Provider:   filter.Provider,
				Service:    filter.Service,
				Channel:    filter.Channel,
				Model:      filter.Model,
				Status:     filter.Status,
				NextOffset: nextOffset,
			},
		},
	})
}
```

- [ ] **Step 7: Register route**

In `internal/api/server.go`, add before `diagnostics/:run_id`:

```go
	router.GET("/api/audit/diagnostics/history", handler.GetAuditDiagnosticHistory)
```

Final route block must be:

```go
	router.GET("/api/audit/diagnostics/latest", handler.GetAuditDiagnosticLatest)
	router.GET("/api/audit/diagnostics/history", handler.GetAuditDiagnosticHistory)
	router.GET("/api/audit/diagnostics/:run_id", handler.GetAuditDiagnostic)
	router.GET("/api/audit/compare/:run_id", handler.GetAuditCompare)
```

- [ ] **Step 8: Add API test**

In `internal/api/audit_handler_test.go`, add a test that:

```go
func TestGetAuditDiagnosticHistoryFiltersAndPaginates(t *testing.T) {
	store := newAuditHandlerTestStore(t)
	handler := NewHandler(store, testConfig())

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
		if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{RunID: run.RunID, OverallScore: 80, CreatedAt: run.CreatedAt}); err != nil {
			t.Fatalf("SaveDiagnosticScore(%s): %v", run.RunID, err)
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/diagnostics/history?provider=p1&service=anthropic&channel=ch1&model=m1&limit=2&offset=1", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.GetAuditDiagnosticHistory(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data struct {
			Items []auditDiagnosticLatestItemResponse `json:"items"`
			Meta auditDiagnosticHistoryMetaResponse `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
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
```

If `newAuditHandlerTestStore` or `testConfig` do not exist, use the existing handler test helper names in `internal/api/audit_handler_test.go`.

- [ ] **Step 9: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected:

```text
ok  	monitor/internal/api
```

---

### Task 3: Add Frontend Types And Hook

**Files:**
- Modify: `frontend/src/types/audit.ts`
- Create: `frontend/src/hooks/useAuditDiagnosticHistory.ts`

- [ ] **Step 1: Add history response types**

In `frontend/src/types/audit.ts`, after `AuditDiagnosticLatestResponse`, add:

```ts
export interface AuditDiagnosticHistoryMeta {
  limit: number;
  offset: number;
  count: number;
  total: number;
  provider?: string;
  service?: string;
  channel?: string;
  model?: string;
  status?: string;
  next_offset?: number | null;
}

export interface AuditDiagnosticHistoryResponse {
  success: boolean;
  data: {
    items: AuditDiagnosticLatestItem[];
    meta: AuditDiagnosticHistoryMeta;
  };
}
```

- [ ] **Step 2: Create history hook**

Create `frontend/src/hooks/useAuditDiagnosticHistory.ts`:

```ts
import { useEffect, useMemo, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditDiagnosticHistoryMeta, AuditDiagnosticHistoryResponse, AuditDiagnosticLatestItem } from '../types/audit';

interface UseAuditDiagnosticHistoryArgs {
  provider?: string;
  service?: string;
  channel?: string;
  model?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

interface UseAuditDiagnosticHistoryResult {
  items: AuditDiagnosticLatestItem[];
  meta: AuditDiagnosticHistoryMeta | null;
  loading: boolean;
  error: string | null;
}

export function useAuditDiagnosticHistory({
  provider,
  service,
  channel,
  model,
  status,
  limit = 50,
  offset = 0,
}: UseAuditDiagnosticHistoryArgs): UseAuditDiagnosticHistoryResult {
  const [items, setItems] = useState<AuditDiagnosticLatestItem[]>([]);
  const [meta, setMeta] = useState<AuditDiagnosticHistoryMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => {
    const params = new URLSearchParams();
    if (provider) params.set('provider', provider);
    if (service) params.set('service', service);
    if (channel) params.set('channel', channel);
    if (model) params.set('model', model);
    if (status) params.set('status', status);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return params.toString();
  }, [provider, service, channel, model, status, limit, offset]);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    apiGet<AuditDiagnosticHistoryResponse>(`/api/audit/diagnostics/history?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
        setMeta(response?.data?.meta ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载检测历史失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [query]);

  return { items, meta, loading, error };
}
```

- [ ] **Step 3: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

---

### Task 4: Build Dedicated Detect History Page

**Files:**
- Create: `frontend/src/pages/DetectHistoryPage.tsx`

- [ ] **Step 1: Create page component**

Create `frontend/src/pages/DetectHistoryPage.tsx`:

```tsx
import { useMemo } from 'react';
import { Helmet } from 'react-helmet-async';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { ExternalLink, History } from 'lucide-react';

import { Header } from '../components/Header';
import { useAuditDiagnosticHistory } from '../hooks/useAuditDiagnosticHistory';
import type { AuditDiagnosticLatestItem } from '../types/audit';

function toInt(value: string | null, fallback: number): number {
  if (!value) return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}

function formatTime(timestamp?: number): string {
  if (!timestamp) return '--';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(timestamp * 1000));
}

function statusText(item: AuditDiagnosticLatestItem): string {
  const status = item.run.run_status || item.filter_reason || item.run.status || 'unknown';
  switch (status) {
    case 'done':
      return item.usable ? '有效' : '完成但无效';
    case 'failed_auth':
      return '认证失败';
    case 'failed_request':
      return '请求失败';
    case 'not_done':
      return '未完成';
    default:
      return status;
  }
}

function StatusBadge({ item }: { item: AuditDiagnosticLatestItem }) {
  const text = statusText(item);
  const raw = (item.run.run_status || item.filter_reason || item.run.status || '').toLowerCase();
  let className = 'bg-slate-500/15 text-slate-300';
  if (item.usable) className = 'bg-emerald-500/15 text-emerald-300';
  else if (raw === 'failed_auth' || raw === 'failed_request') className = 'bg-rose-500/15 text-rose-300';
  else if (raw === 'done') className = 'bg-amber-500/15 text-amber-300';
  return <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold ${className}`}>{text}</span>;
}

function scoreText(item: AuditDiagnosticLatestItem): string {
  if (!item.score) return '--';
  return String(Math.round(item.score.overall_score));
}

function updateOffset(searchParams: URLSearchParams, offset: number): string {
  const next = new URLSearchParams(searchParams);
  next.set('offset', String(Math.max(0, offset)));
  return `?${next.toString()}`;
}

export default function DetectHistoryPage() {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const provider = searchParams.get('provider') || undefined;
  const service = searchParams.get('service') || undefined;
  const channel = searchParams.get('channel') || undefined;
  const model = searchParams.get('model') || undefined;
  const status = searchParams.get('status') || undefined;
  const limit = toInt(searchParams.get('limit'), 50);
  const offset = toInt(searchParams.get('offset'), 0);

  const { items, meta, loading, error } = useAuditDiagnosticHistory({
    provider,
    service,
    channel,
    model,
    status,
    limit,
    offset,
  });

  const headerStats = useMemo(() => ({
    total: meta?.total ?? items.length,
    healthy: items.filter((item) => item.usable).length,
    issues: Math.max(0, items.length - items.filter((item) => item.usable).length),
  }), [items, meta]);

  const detailPath = (runId: string) => {
    const prefixMatch = location.pathname.match(/^\/(en|ru|ja)(\/|$)/);
    const prefix = prefixMatch ? `/${prefixMatch[1]}` : '';
    return `${prefix}/detect/compare/${runId}`;
  };

  return (
    <>
      <Helmet>
        <title>检测历史 - RelayPulse</title>
        <meta name="description" content="按服务商、通道和模型查看检测历史样本。" />
      </Helmet>
      <div className="min-h-screen bg-page text-primary">
        <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
          <Header stats={headerStats} />

          <section className="mb-6 rounded-2xl border border-default/70 bg-surface/55 px-5 py-5">
            <div className="flex flex-wrap items-center gap-3">
              <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent/10 text-accent">
                <History size={20} />
              </span>
              <div>
                <h1 className="text-2xl font-bold text-primary">检测历史</h1>
                <p className="mt-1 text-sm text-secondary">当前页面展示该通道、该模型的检测样本；每条样本可进入实际检测情况。</p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-2 text-xs">
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">服务商 {provider || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">服务 {service || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">通道 {channel || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">模型 {model || '--'}</span>
            </div>
          </section>

          {loading && items.length === 0 ? (
            <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">正在加载检测历史...</div>
          ) : error ? (
            <div className="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">{error}</div>
          ) : items.length === 0 ? (
            <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">当前通道和模型还没有检测历史。</div>
          ) : (
            <>
              <div className="mb-3 text-sm text-secondary">
                共 {meta?.total ?? items.length} 条，当前显示 {items.length} 条
              </div>
              <div className="overflow-x-auto rounded-2xl border border-default bg-surface">
                <table className="w-full min-w-[980px] text-sm">
                  <thead>
                    <tr className="border-b border-default/60 text-left text-muted">
                      <th className="px-3 py-3 font-medium">时间</th>
                      <th className="px-3 py-3 font-medium">状态</th>
                      <th className="px-3 py-3 font-medium">模型</th>
                      <th className="px-3 py-3 font-medium">Run ID</th>
                      <th className="px-3 py-3 text-right font-medium">分数</th>
                      <th className="px-3 py-3 text-right font-medium">实际情况</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((item) => (
                      <tr key={item.run.run_id} className="border-b border-default/30 last:border-b-0 hover:bg-elevated/40">
                        <td className="px-3 py-3 font-mono text-xs text-muted">{formatTime(item.run.created_at)}</td>
                        <td className="px-3 py-3"><StatusBadge item={item} /></td>
                        <td className="px-3 py-3 font-mono text-xs text-secondary">{item.run.model || item.run.request_model || '--'}</td>
                        <td className="px-3 py-3 font-mono text-xs text-muted">{item.run.run_id}</td>
                        <td className="px-3 py-3 text-right font-mono text-primary">{scoreText(item)}</td>
                        <td className="px-3 py-3 text-right">
                          <Link to={detailPath(item.run.run_id)} className="inline-flex items-center gap-1 text-accent hover:underline">
                            查看实际情况
                            <ExternalLink size={12} />
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="mt-4 flex items-center justify-end gap-2">
                {offset > 0 ? (
                  <Link to={updateOffset(searchParams, Math.max(0, offset - limit))} className="rounded-lg border border-default px-3 py-1.5 text-sm text-secondary hover:text-primary">
                    上一页
                  </Link>
                ) : null}
                {meta?.next_offset != null ? (
                  <Link to={updateOffset(searchParams, meta.next_offset)} className="rounded-lg border border-default px-3 py-1.5 text-sm text-secondary hover:text-primary">
                    下一页
                  </Link>
                ) : null}
              </div>
            </>
          )}
        </div>
      </div>
    </>
  );
}
```

- [ ] **Step 2: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

---

### Task 5: Add History Route

**Files:**
- Modify: `frontend/src/router.tsx`

- [ ] **Step 1: Add lazy import**

In `frontend/src/router.tsx`, add:

```tsx
const DetectHistoryPage = lazy(() => import('./pages/DetectHistoryPage'));
```

near:

```tsx
const DetectPage = lazy(() => import('./pages/DetectPage'));
const DetectComparePage = lazy(() => import('./pages/DetectComparePage'));
```

- [ ] **Step 2: Add route**

In `renderChildRoutes`, add:

```tsx
      <Route path="detect/history" element={<DetectHistoryPage />} />
```

near:

```tsx
      <Route path="detect" element={<DetectPage />} />
      <Route path="detect/compare" element={<DetectPage />} />
      <Route path="detect/compare/:runId" element={<DetectComparePage />} />
```

Final block:

```tsx
      <Route path="detect" element={<DetectPage />} />
      <Route path="detect/history" element={<DetectHistoryPage />} />
      <Route path="detect/compare" element={<DetectPage />} />
      <Route path="detect/compare/:runId" element={<DetectComparePage />} />
```

- [ ] **Step 3: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

---

### Task 6: Update Provider Page History Link

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Change history URL**

Find:

```tsx
          historyUrl: `${langPrefix}/detect/compare?${historyParams.toString()}`,
```

Replace with:

```tsx
          historyUrl: `${langPrefix}/detect/history?${historyParams.toString()}`,
```

- [ ] **Step 2: Keep per-run evidence link**

Do not remove:

```tsx
          compareUrl: localCompareUrl,
```

Reason: model row should keep both concepts:

- `检测历史` -> `/detect/history?...`
- `最新证据` / `失败详情` -> `/detect/compare/:runId`

- [ ] **Step 3: Run grep**

Run:

```bash
rg -n "historyUrl:.*detect/compare|detect/history|检测历史" frontend/src/pages/ProviderPage.tsx frontend/src/router.tsx
```

Expected:

```text
frontend/src/pages/ProviderPage.tsx:<line>:          historyUrl: `${langPrefix}/detect/history?${historyParams.toString()}`,
frontend/src/pages/ProviderPage.tsx:<line>:                            检测历史
frontend/src/router.tsx:<line>:      <Route path="detect/history" element={<DetectHistoryPage />} />
```

No `historyUrl` should point to `detect/compare`.

---

### Task 7: Remove Query-Filtered History From DetectPage

**Files:**
- Modify: `frontend/src/pages/DetectPage.tsx`

- [ ] **Step 1: Remove `useSearchParams` import**

Remove:

```tsx
import { useSearchParams } from 'react-router-dom';
```

- [ ] **Step 2: Restore `LatestDiagnosticEvidence` to global recent evidence only**

In `LatestDiagnosticEvidence`, replace:

```tsx
  const [searchParams] = useSearchParams();
  const langPrefix = LANGUAGE_PATH_MAP[i18n.language as SupportedLanguage];
  const provider = searchParams.get('provider') || undefined;
  const service = searchParams.get('service') || undefined;
  const channel = searchParams.get('channel') || undefined;
  const model = searchParams.get('model') || undefined;
  const filtered = Boolean(provider || service || channel || model);
  const { items, loading, error } = useAuditDiagnosticLatest({
    provider,
    service,
    channel,
    model,
    includeFiltered: true,
    limit: filtered ? 20 : 10,
  });
```

with:

```tsx
  const langPrefix = LANGUAGE_PATH_MAP[i18n.language as SupportedLanguage];
  const { items, loading, error } = useAuditDiagnosticLatest({
    includeFiltered: true,
    limit: 10,
  });
```

- [ ] **Step 3: Remove filtered summary UI**

Delete the `DiagnosticFilterSummary` component and all calls to it from `DetectPage`.

The empty state should return to:

```tsx
  if (items.length === 0) {
    return <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">当前还没有检测证据。完成一次 quick-probe 后会显示在这里。</div>;
  }
```

- [ ] **Step 4: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

---

### Task 8: End-To-End Verification

**Files:**
- Modify generated embedded frontend under `internal/api/frontend`.

- [ ] **Step 1: Run all planned tests**

Run:

```bash
go test ./internal/storage ./internal/api ./internal/audit ./internal/config
npm run build
```

Expected:

```text
ok  	monitor/internal/storage
ok  	monitor/internal/api
ok  	monitor/internal/audit
ok  	monitor/internal/config
✓ built
```

- [ ] **Step 2: Embed frontend assets**

Run:

```bash
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -R frontend/dist internal/api/frontend/
```

- [ ] **Step 3: Restart local server**

Run:

```bash
pid=$(lsof -tiTCP:18080 -sTCP:LISTEN || true)
if [ -n "$pid" ]; then kill $pid; sleep 1; fi
PORT=18080 go run ./cmd/server/main.go config.yaml
```

Expected log:

```text
监测服务已启动 ... web_ui=http://localhost:18080
```

- [ ] **Step 4: Verify API**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/diagnostics/history?provider=yuexin01-team7000-sunday-2133&service=anthropic&channel=65%3Ayuexin01-team7000-sunday-2133&model=claude-fable-5&limit=20&offset=0' \
  | jq '{success, total: .data.meta.total, count: .data.meta.count, first: .data.items[0].run.run_id}'
```

Expected:

```json
{
  "success": true,
  "total": 2,
  "count": 2,
  "first": "diag-..."
}
```

The exact `run_id` may differ.

- [ ] **Step 5: Verify pages**

Run:

```bash
curl -sS -o /tmp/provider_history_entry.html -w '%{http_code} %{content_type}\n' \
  'http://127.0.0.1:18080/p/yuexin01-team7000-sunday-2133?service=cc&channel=65%3Ayuexin01-team7000-sunday-2133'

curl -sS -o /tmp/detect_history.html -w '%{http_code} %{content_type}\n' \
  'http://127.0.0.1:18080/detect/history?provider=yuexin01-team7000-sunday-2133&service=anthropic&channel=65%3Ayuexin01-team7000-sunday-2133&model=claude-fable-5'

curl -sS -o /tmp/detect_compare.html -w '%{http_code} %{content_type}\n' \
  'http://127.0.0.1:18080/detect/compare/diag-8ae0ef20-a735-4df3-a78b-a30f9f0f1a37'
```

Expected:

```text
200 text/html; charset=utf-8
200 text/html; charset=utf-8
200 text/html; charset=utf-8
```

- [ ] **Step 6: Manual browser acceptance**

Open:

```text
http://127.0.0.1:18080/p/yuexin01-team7000-sunday-2133?service=cc&channel=65%3Ayuexin01-team7000-sunday-2133
```

Acceptance:

- 模型表格“结果详情”列有 `检测历史`。
- 点击 `检测历史` 进入 `/detect/history?...`，不是 `/detect/compare?...`。
- 历史页标题是 `检测历史`。
- 历史页展示服务商、服务、通道、模型筛选摘要。
- 历史页表格展示多条检测样本。
- 每条样本右侧有 `查看实际情况`。
- 点击 `查看实际情况` 进入 `/detect/compare/:runId`。

---

### Task 9: Commit

**Files:**
- Commit code and plan file.

- [ ] **Step 1: Check status**

Run:

```bash
git status --short
```

Expected source changes include:

```text
M internal/storage/audit_models.go
M internal/api/audit_handler.go
M internal/api/audit_types.go
M internal/api/server.go
M internal/storage/audit_models_test.go
M internal/api/audit_handler_test.go
M frontend/src/types/audit.ts
A frontend/src/hooks/useAuditDiagnosticHistory.ts
A frontend/src/pages/DetectHistoryPage.tsx
M frontend/src/router.tsx
M frontend/src/pages/ProviderPage.tsx
M frontend/src/pages/DetectPage.tsx
A docs/superpowers/plans/2026-06-28-diagnostic-history-page.md
```

Also include existing prior uncommitted frontend changes if they are still part of the desired product behavior:

```text
M frontend/src/hooks/useAuditDiagnosticLatest.ts
```

- [ ] **Step 2: Commit**

Run:

```bash
git add internal/storage/audit_models.go \
  internal/api/audit_handler.go \
  internal/api/audit_types.go \
  internal/api/server.go \
  internal/storage/audit_models_test.go \
  internal/api/audit_handler_test.go \
  frontend/src/types/audit.ts \
  frontend/src/hooks/useAuditDiagnosticHistory.ts \
  frontend/src/hooks/useAuditDiagnosticLatest.ts \
  frontend/src/pages/DetectHistoryPage.tsx \
  frontend/src/pages/ProviderPage.tsx \
  frontend/src/pages/DetectPage.tsx \
  frontend/src/router.tsx \
  docs/superpowers/plans/2026-06-28-diagnostic-history-page.md
git commit -m "feat(audit): add diagnostic history page"
```

If `internal/api/frontend` has tracked embedded asset changes after build, include it:

```bash
git add internal/api/frontend
git commit -m "feat(audit): add diagnostic history page"
```

Expected:

```text
[main <hash>] feat(audit): add diagnostic history page
```

---

## Self-Review

Spec coverage:

- “点击检测历史” implemented by Task 6 changing `historyUrl` to `/detect/history?...`.
- “进入检测历史的单独页面，而不是当前检测方法页面” implemented by Tasks 4 and 5 creating `DetectHistoryPage` and route `/detect/history`.
- “页面中是检测历史的显示页面” implemented by Task 4 table rendering history samples.
- “显示所有的检测情况” implemented by Tasks 1 and 2 backend history API with pagination and total count.
- “每条检测样本后面有一个链接” implemented by Task 4 `查看实际情况`.
- “点击链接后进入 /detect/compare/:ID” implemented by Task 4 `detailPath(item.run.run_id)`.

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No “similar to”.
- Each code edit includes exact file path and concrete code.

Type consistency:

- Backend item type reuses `auditDiagnosticLatestItemResponse`.
- Frontend item type reuses `AuditDiagnosticLatestItem`.
- Route name is consistently `/detect/history`.
- Detail route remains `/detect/compare/:runId`.
