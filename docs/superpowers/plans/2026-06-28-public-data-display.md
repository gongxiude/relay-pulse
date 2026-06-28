# Public Data Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在对外页面展示已经具备的真实监测数据：通道整体模板可用率、超时/无响应统计、生产日志状态、模板 probe 补洞状态，以及 quick-probe 与 baseline 的对比结果。

**Architecture:** 继续以 `/api/audit/model-status` 作为服务商详情页的数据聚合接口，不新增平行页面或平行探测体系。后端在现有 `production/template_probe/quick_probe` 三层状态上补充窗口级模板探测汇总和 quick-probe baseline 摘要；前端 Provider 详情页消费这些字段，渲染对外可读的概览卡、模型表格列和失败/无基线提示。首页仍保留服务商入口列表，详情页承担具体数据展示。

**Tech Stack:** Go, Gin API, SQLite/PostgreSQL storage interfaces, existing `probe_history`, existing `diagnostic_runs/diagnostic_dimensions`, React, TypeScript, Tailwind CSS, Vite.

---

## Evidence Base

本计划基于以下当前文件和历史计划：

| 来源 | 结论 |
|---|---|
| `docs/relaypulse-probe-requirements-zh.md` | 第一版数据闭环是 `new-api 渠道同步 -> 生产日志 -> 模板探测补洞 -> quick-probe-v1 -> 页面展示`。 |
| `docs/superpowers/plans/2026-06-25-frontend-display-plan.md` | 首页是入口页，详情页按模型展开，展示每个模型的真实性、质量、可用率和趋势。 |
| `docs/superpowers/plans/2026-06-25-audit-data-fill-plan.md` | 页面必须展示真实审计数据，不再只显示静态说明或占位。 |
| `docs/superpowers/plans/2026-06-28-diagnostic-history-page.md` | 模型行的检测历史入口已经独立为 `/detect/history?...`，单次详情为 `/detect/compare/:runId`。 |
| `docs/superpowers/plans/2026-06-28-fix-diagnostic-target-baseurl.md` | 主动诊断和模板补洞必须使用 `audit_targets.base_url/api_key`，不能 fallback 到全局 `NEWAPI_BASE_URL`。 |

当前代码事实：

| 文件 | 当前能力 |
|---|---|
| `internal/api/audit_handler.go` | 已有 `/api/audit/model-status`，按 target 聚合 `production/template_probe/quick_probe`。 |
| `internal/storage/storage.go` | 已有 `GetHistory(provider, service, channel, model, since)`，可读取模板探测历史。 |
| `frontend/src/pages/ProviderPage.tsx` | 已接入 `useAuditModelStatus`、`useAuditDiagnosticLatest`、`useRpdiagScores`，但整体可用率和 baseline 摘要还没有作为页面主信息展示。 |
| `frontend/src/types/audit.ts` | 已有 `AuditModelStatusItem` 类型，缺少窗口级模板探测汇总和 model-status meta summary 类型。 |

---

## File Structure

Modify:

- `internal/api/audit_types.go`
  - 扩展 `auditModelStatusMetaResponse`，新增 `Summary auditModelStatusSummaryResponse`。
  - 扩展 `auditTemplateProbeStatusResponse`，新增窗口级 `total/success/degraded/timeout/no_response/availability/window`。
  - 扩展 `auditQuickProbeStatusResponse`，新增 `baseline_mode/active_weight/dimensions_total/dimensions_pass/dimensions_fail/dimensions_skip`。

- `internal/api/audit_handler.go`
  - `auditModelStatusStore` 增加 `GetHistory(provider, service, channel, model string, since time.Time) ([]*storage.ProbeRecord, error)`。
  - `GetAuditModelStatus` 在 meta 中返回所有 item 的聚合 summary。
  - `buildAuditTemplateProbeStatus` 读取窗口内 `probe_history`，计算模板可用率、超时和无响应。
  - `buildAuditQuickProbeStatus` 读取 `diagnostic_dimensions`，返回 baseline/维度摘要。

- `internal/api/audit_handler_test.go`
  - 覆盖 model-status 返回模板窗口汇总、meta summary 和 quick-probe 维度摘要。

- `frontend/src/types/audit.ts`
  - 同步新增的 API 字段。

- `frontend/src/hooks/useAuditModelStatus.ts`
  - 返回 `meta`，让 ProviderPage 使用后端聚合 summary。

- `frontend/src/pages/ProviderPage.tsx`
  - 顶部新增“整体可用率 / 正常请求 / 超时与无响应 / quick-probe 对比”概览卡。
  - 表格中把“可用率 30D”改为真实窗口口径“模板可用率 24H”。
  - 表格新增或强化 baseline/维度摘要展示，明确 quick-probe 是否已完成、是否有 baseline、通过/失败/跳过维度数。
  - 不删除检测历史入口，不把检测历史重新放回 `/detect` 方法页。

No changes:

- 不新增新的探测框架。
- 不修改 `new-api`。
- 不返回明文 API key。
- 不改变 `/detect/history` 与 `/detect/compare/:runId` 路由职责。

---

## Task 1: Enrich Model Status API Schema

**Files:**
- Modify: `internal/api/audit_types.go`

- [ ] **Step 1: Add model-status summary type**

In `internal/api/audit_types.go`, replace:

```go
type auditModelStatusMetaResponse struct {
	Window string `json:"window"`
	Count  int    `json:"count"`
}
```

with:

```go
type auditModelStatusMetaResponse struct {
	Window  string                          `json:"window"`
	Count   int                             `json:"count"`
	Summary auditModelStatusSummaryResponse `json:"summary"`
}

type auditModelStatusSummaryResponse struct {
	TotalModels             int     `json:"total_models"`
	EnabledModels           int     `json:"enabled_models"`
	TemplateProbeTotal      int     `json:"template_probe_total"`
	TemplateProbeSuccess    int     `json:"template_probe_success"`
	TemplateProbeTimeout    int     `json:"template_probe_timeout"`
	TemplateProbeNoResponse int     `json:"template_probe_no_response"`
	TemplateAvailability    float64 `json:"template_availability"`
	ProductionTotal         int     `json:"production_total"`
	ProductionSuccess       int     `json:"production_success"`
	ProductionSuccessRate   float64 `json:"production_success_rate"`
	QuickProbeDone          int     `json:"quick_probe_done"`
	QuickProbeFailed        int     `json:"quick_probe_failed"`
	QuickProbeMissing       int     `json:"quick_probe_missing"`
	BaselineCompared        int     `json:"baseline_compared"`
}
```

- [ ] **Step 2: Extend template probe status response**

In the same file, replace `auditTemplateProbeStatusResponse` with:

```go
type auditTemplateProbeStatusResponse struct {
	Source       string  `json:"source"`
	Status       string  `json:"status"`
	SubStatus    string  `json:"sub_status,omitempty"`
	HTTPCode     int     `json:"http_code,omitempty"`
	Latency      int     `json:"latency,omitempty"`
	UpdatedAt    int64   `json:"updated_at,omitempty"`
	Error        string  `json:"error,omitempty"`
	Window       string  `json:"window,omitempty"`
	Total        int     `json:"total"`
	Success      int     `json:"success"`
	Degraded     int     `json:"degraded"`
	Timeout      int     `json:"timeout"`
	NoResponse   int     `json:"no_response"`
	Availability float64 `json:"availability"`
}
```

- [ ] **Step 3: Extend quick probe status response**

In the same file, replace `auditQuickProbeStatusResponse` with:

```go
type auditQuickProbeStatusResponse struct {
	Source          string  `json:"source"`
	Status          string  `json:"status"`
	RunID           string  `json:"run_id,omitempty"`
	CompareURL      string  `json:"compare_url,omitempty"`
	Usable          bool    `json:"usable"`
	Reason          string  `json:"reason,omitempty"`
	Score           float64 `json:"score,omitempty"`
	UpdatedAt       int64   `json:"updated_at,omitempty"`
	Methodology     string  `json:"methodology,omitempty"`
	BaselineMode    string  `json:"baseline_mode,omitempty"`
	ActiveWeight    int     `json:"active_weight,omitempty"`
	DimensionsTotal int     `json:"dimensions_total,omitempty"`
	DimensionsPass  int     `json:"dimensions_pass,omitempty"`
	DimensionsFail  int     `json:"dimensions_fail,omitempty"`
	DimensionsSkip  int     `json:"dimensions_skip,omitempty"`
}
```

- [ ] **Step 4: Run focused compile check**

Run:

```bash
go test ./internal/api -run TestNonExistent
```

Expected before Task 2 implementation:

```text
FAIL
```

The failure should be compile errors for missing fields or changed constructors. If the package still compiles, continue; later tasks still add behavior and tests.

---

## Task 2: Compute Template Probe Window Metrics And Summary

**Files:**
- Modify: `internal/api/audit_handler.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Extend `auditModelStatusStore`**

In `internal/api/audit_handler.go`, replace:

```go
type auditModelStatusStore interface {
	auditReadStore
	GetLatest(provider, service, channel, model string) (*storage.ProbeRecord, error)
}
```

with:

```go
type auditModelStatusStore interface {
	auditReadStore
	GetLatest(provider, service, channel, model string) (*storage.ProbeRecord, error)
	GetHistory(provider, service, channel, model string, since time.Time) ([]*storage.ProbeRecord, error)
}
```

- [ ] **Step 2: Pass window into template probe status**

In `buildAuditModelStatusItems`, replace:

```go
templateProbe, err := buildAuditTemplateProbeStatus(store, target)
```

with:

```go
templateProbe, err := buildAuditTemplateProbeStatus(store, target, window, now)
```

- [ ] **Step 3: Replace `buildAuditTemplateProbeStatus`**

Replace the full function with:

```go
func buildAuditTemplateProbeStatus(store auditModelStatusStore, target storage.AuditTarget, window string, now time.Time) (auditTemplateProbeStatusResponse, error) {
	record, err := store.GetLatest(target.Provider, target.Service, target.Channel, target.Model)
	if err != nil {
		return auditTemplateProbeStatusResponse{}, err
	}
	history, err := store.GetHistory(target.Provider, target.Service, target.Channel, target.Model, now.Add(-windowDuration(window)))
	if err != nil {
		return auditTemplateProbeStatusResponse{}, err
	}

	resp := auditTemplateProbeStatusResponse{
		Source: "template_probe",
		Status: "missing",
		Window: window,
	}
	if record != nil {
		resp.Status = templateProbeStatusLabel(record.Status)
		resp.SubStatus = string(record.SubStatus)
		resp.HTTPCode = record.HttpCode
		resp.Latency = record.Latency
		resp.UpdatedAt = record.Timestamp
		resp.Error = record.ErrorDetail
	}
	for _, item := range history {
		if item == nil {
			continue
		}
		resp.Total++
		switch item.Status {
		case 1:
			resp.Success++
		case 2:
			resp.Degraded++
		}
		subStatus := strings.ToLower(strings.TrimSpace(string(item.SubStatus)))
		if subStatus == "response_timeout" || strings.Contains(subStatus, "timeout") {
			resp.Timeout++
		}
		if item.Status == 0 && item.HttpCode == 0 {
			resp.NoResponse++
		}
	}
	if resp.Total > 0 {
		resp.Availability = float64(resp.Success+resp.Degraded) / float64(resp.Total) * 100
	}
	return resp, nil
}

func templateProbeStatusLabel(status int) string {
	switch status {
	case 1:
		return "available"
	case 2:
		return "degraded"
	default:
		return "unavailable"
	}
}
```

- [ ] **Step 4: Add meta summary builder**

Add this helper near `buildAuditModelStatusItems`:

```go
func buildAuditModelStatusSummary(items []auditModelStatusItemResponse) auditModelStatusSummaryResponse {
	summary := auditModelStatusSummaryResponse{TotalModels: len(items)}
	for _, item := range items {
		if item.Enabled {
			summary.EnabledModels++
		}
		summary.TemplateProbeTotal += item.TemplateProbe.Total
		summary.TemplateProbeSuccess += item.TemplateProbe.Success + item.TemplateProbe.Degraded
		summary.TemplateProbeTimeout += item.TemplateProbe.Timeout
		summary.TemplateProbeNoResponse += item.TemplateProbe.NoResponse
		summary.ProductionTotal += item.Production.Total
		summary.ProductionSuccess += item.Production.Success
		switch item.QuickProbe.Status {
		case "done":
			summary.QuickProbeDone++
		case "missing":
			summary.QuickProbeMissing++
		default:
			summary.QuickProbeFailed++
		}
		if item.QuickProbe.BaselineMode != "" && item.QuickProbe.BaselineMode != "candidate_only" {
			summary.BaselineCompared++
		}
	}
	if summary.TemplateProbeTotal > 0 {
		summary.TemplateAvailability = float64(summary.TemplateProbeSuccess) / float64(summary.TemplateProbeTotal) * 100
	}
	if summary.ProductionTotal > 0 {
		summary.ProductionSuccessRate = float64(summary.ProductionSuccess) / float64(summary.ProductionTotal) * 100
	}
	return summary
}
```

- [ ] **Step 5: Return summary in `GetAuditModelStatus`**

In `GetAuditModelStatus`, replace the `Meta` construction:

```go
Meta: auditModelStatusMetaResponse{
	Window: window,
	Count:  len(items),
},
```

with:

```go
Meta: auditModelStatusMetaResponse{
	Window:  window,
	Count:   len(items),
	Summary: buildAuditModelStatusSummary(items),
},
```

- [ ] **Step 6: Add API test for template summary**

In `internal/api/audit_handler_test.go`, add:

```go
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
		{Provider: "OpenAI", Service: "cc", Channel: "101:demo", Model: "gpt-4o", Status: 0, SubStatus: storage.SubStatus("response_timeout"), HttpCode: 0, Timestamp: now - 180, ErrorDetail: "timeout"},
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
```

- [ ] **Step 7: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected:

```text
ok  	monitor/internal/api
```

- [ ] **Step 8: Commit**

```bash
git add internal/api/audit_types.go internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "feat(audit): expose template probe availability summary"
```

---

## Task 3: Add Quick-Probe Baseline And Dimension Summary

**Files:**
- Modify: `internal/api/audit_handler.go`
- Test: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add dimension summary helper**

Add this helper near `buildAuditQuickProbeStatus`:

```go
func summarizeDiagnosticDimensions(dimensions []*storage.DiagnosticDimension) (total, pass, fail, skip int) {
	for _, dimension := range dimensions {
		if dimension == nil {
			continue
		}
		total++
		switch strings.ToLower(strings.TrimSpace(dimension.Status)) {
		case "pass":
			pass++
		case "skip":
			skip++
		default:
			fail++
		}
	}
	return total, pass, fail, skip
}
```

- [ ] **Step 2: Enrich `buildAuditQuickProbeStatus`**

Inside `buildAuditQuickProbeStatus`, after:

```go
runResp := buildAuditDiagnosticRun(run)
```

insert:

```go
dimensions, err := store.ListDiagnosticDimensions(run.RunID)
if err != nil {
	return auditQuickProbeStatusResponse{}, err
}
dimensionsTotal, dimensionsPass, dimensionsFail, dimensionsSkip := summarizeDiagnosticDimensions(dimensions)
```

Then, in the `resp := auditQuickProbeStatusResponse{...}` literal, add:

```go
BaselineMode:    runResp.BaselineMode,
DimensionsTotal: dimensionsTotal,
DimensionsPass:  dimensionsPass,
DimensionsFail:  dimensionsFail,
DimensionsSkip:  dimensionsSkip,
```

After `if scoreResp := buildAuditDiagnosticScore(run, score); scoreResp != nil { ... }`, add:

```go
if scoreResp := buildAuditDiagnosticScore(run, score); scoreResp != nil {
	resp.Score = scoreResp.OverallScore
	resp.ActiveWeight = scoreResp.ActiveWeight
}
```

If the existing block already sets `resp.Score`, replace that block with the two-field version above.

- [ ] **Step 3: Add API test for quick-probe summary**

In `internal/api/audit_handler_test.go`, add:

```go
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
		Input:     []byte(`{"baseline_mode":"registered_baseline","methodology_version":"quick-probe-v1"}`),
		Output:    []byte(`{"overall_score":88,"active_weight":20}`),
	}
	if err := store.SaveDiagnosticRun(run); err != nil {
		t.Fatalf("SaveDiagnosticRun: %v", err)
	}
	if err := store.SaveDiagnosticScore(&storage.DiagnosticScore{
		RunID:         run.RunID,
		OverallScore:  88,
		ActiveWeight:  20,
		CreatedAt:     now,
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
```

- [ ] **Step 4: Run API tests**

Run:

```bash
go test ./internal/api
```

Expected:

```text
ok  	monitor/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "feat(audit): expose quick probe compare summary"
```

---

## Task 4: Update Frontend Types And Hook

**Files:**
- Modify: `frontend/src/types/audit.ts`
- Modify: `frontend/src/hooks/useAuditModelStatus.ts`

- [ ] **Step 1: Extend `AuditModelStatusItem`**

In `frontend/src/types/audit.ts`, replace the `template_probe` shape inside `AuditModelStatusItem` with:

```ts
  template_probe: {
    source: 'template_probe';
    status: string;
    sub_status?: string;
    http_code?: number;
    latency?: number;
    updated_at?: number;
    error?: string;
    window?: string;
    total: number;
    success: number;
    degraded: number;
    timeout: number;
    no_response: number;
    availability: number;
  };
```

Replace the `quick_probe` shape with:

```ts
  quick_probe: {
    source: 'quick_probe';
    status: string;
    run_id?: string;
    compare_url?: string;
    usable: boolean;
    reason?: string;
    score?: number;
    updated_at?: number;
    methodology?: string;
    baseline_mode?: string;
    active_weight?: number;
    dimensions_total?: number;
    dimensions_pass?: number;
    dimensions_fail?: number;
    dimensions_skip?: number;
  };
```

- [ ] **Step 2: Add `AuditModelStatusMeta`**

Add this interface near `AuditModelStatusResponse`:

```ts
export interface AuditModelStatusMeta {
  window?: string;
  count?: number;
  summary?: {
    total_models: number;
    enabled_models: number;
    template_probe_total: number;
    template_probe_success: number;
    template_probe_timeout: number;
    template_probe_no_response: number;
    template_availability: number;
    production_total: number;
    production_success: number;
    production_success_rate: number;
    quick_probe_done: number;
    quick_probe_failed: number;
    quick_probe_missing: number;
    baseline_compared: number;
  };
}
```

Then replace the meta type inside `AuditModelStatusResponse`:

```ts
    meta?: {
      window?: string;
      count?: number;
    };
```

with:

```ts
    meta?: AuditModelStatusMeta;
```

- [ ] **Step 3: Return meta from hook**

In `frontend/src/hooks/useAuditModelStatus.ts`, update imports:

```ts
import type { AuditModelStatusItem, AuditModelStatusMeta, AuditModelStatusResponse } from '../types/audit';
```

Replace `UseAuditModelStatusResult` with:

```ts
interface UseAuditModelStatusResult {
  items: AuditModelStatusItem[];
  meta: AuditModelStatusMeta | null;
  loading: boolean;
  error: string | null;
}
```

Add state:

```ts
const [meta, setMeta] = useState<AuditModelStatusMeta | null>(null);
```

When provider/service/channel is missing, also call:

```ts
setMeta(null);
```

Inside the `.then`, after `setItems(...)`, add:

```ts
setMeta(response?.data?.meta ?? null);
```

In `.catch`, add:

```ts
setMeta(null);
```

Return:

```ts
return { items, meta, loading, error };
```

- [ ] **Step 4: Run frontend build**

Run:

```bash
npm run build
```

from `frontend/`.

Expected:

```text
✓ built
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types/audit.ts frontend/src/hooks/useAuditModelStatus.ts
git commit -m "feat(frontend): type model status summaries"
```

---

## Task 5: Render Public Data Summary On Provider Page

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Read `meta` from `useAuditModelStatus`**

In `ProviderPage.tsx`, replace:

```ts
const {
  items: sourceStatuses,
  loading: sourceStatusLoading,
  error: sourceStatusError,
} = useAuditModelStatus({
```

with:

```ts
const {
  items: sourceStatuses,
  meta: sourceStatusMeta,
  loading: sourceStatusLoading,
  error: sourceStatusError,
} = useAuditModelStatus({
```

- [ ] **Step 2: Add page summary derived from meta**

After `diagnosticSummary`, add:

```ts
const publicDataSummary = sourceStatusMeta?.summary;
const templateAvailability = publicDataSummary?.template_availability ?? null;
const templateTotal = publicDataSummary?.template_probe_total ?? 0;
const templateSuccess = publicDataSummary?.template_probe_success ?? 0;
const templateTimeout = publicDataSummary?.template_probe_timeout ?? 0;
const templateNoResponse = publicDataSummary?.template_probe_no_response ?? 0;
const quickProbeDone = publicDataSummary?.quick_probe_done ?? 0;
const quickProbeFailed = publicDataSummary?.quick_probe_failed ?? 0;
const baselineCompared = publicDataSummary?.baseline_compared ?? 0;
```

- [ ] **Step 3: Add summary card section**

After the existing diagnostic sample summary section:

```tsx
          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-3 text-sm text-secondary">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              <span>最近有效样本 {diagnosticSummary.usable}</span>
              <span>401失败 {diagnosticSummary.failedAuth}</span>
              <span>请求失败 {diagnosticSummary.failedRequest}</span>
              <span>其他 {diagnosticSummary.pending}</span>
            </div>
          </section>
```

insert:

```tsx
          <section className="mb-5 grid gap-3 md:grid-cols-4">
            <PublicMetricCard
              label="整体模板可用率"
              value={templateAvailability == null ? '--' : `${templateAvailability.toFixed(templateAvailability >= 99.95 ? 0 : 1)}%`}
              hint={`${sourceStatusMeta?.window || '24h'} 正常请求模板探测`}
              tone={templateAvailability == null ? 'muted' : templateAvailability >= 95 ? 'good' : templateAvailability >= 80 ? 'warn' : 'bad'}
            />
            <PublicMetricCard
              label="正常请求"
              value={`${templateSuccess}/${templateTotal || 0}`}
              hint="来自原模板探测历史，不使用 quick-probe 真实性流程"
              tone={templateTotal > 0 && templateSuccess === templateTotal ? 'good' : 'muted'}
            />
            <PublicMetricCard
              label="超时 / 无响应"
              value={`${templateTimeout} / ${templateNoResponse}`}
              hint="用于识别请求超时、网络错误或没有响应内容"
              tone={templateTimeout + templateNoResponse > 0 ? 'warn' : 'good'}
            />
            <PublicMetricCard
              label="Baseline 对比"
              value={`${baselineCompared}/${quickProbeDone + quickProbeFailed + baselineCompared}`}
              hint={`quick-probe 完成 ${quickProbeDone}，失败 ${quickProbeFailed}`}
              tone={baselineCompared > 0 ? 'good' : 'muted'}
            />
          </section>
```

- [ ] **Step 4: Update table columns**

In the model table header, replace:

```tsx
<th className="px-4 py-4 font-medium">可用率 30D</th>
<th className="px-4 py-4 font-medium">趋势</th>
```

with:

```tsx
<th className="px-4 py-4 font-medium">模板可用率 24H</th>
<th className="px-4 py-4 font-medium">Baseline 对比</th>
```

- [ ] **Step 5: Render template availability and baseline summary per row**

In the row cell that currently renders:

```tsx
<AvailabilityBadge value={row.uptime} enabled={row.enabled} />
```

replace it with:

```tsx
<TemplateAvailabilityCell status={row.sourceStatus?.template_probe ?? null} enabled={row.enabled} />
```

In the next cell that currently renders:

```tsx
<TrendSparkline trend={row.trend} enabled={row.enabled} />
```

replace it with:

```tsx
<QuickProbeCompareCell status={row.sourceStatus?.quick_probe ?? null} enabled={row.enabled} />
```

- [ ] **Step 6: Add rendering helpers**

Add these helpers before `FilterField`:

```tsx
function PublicMetricCard({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: string;
  hint: string;
  tone: 'good' | 'warn' | 'bad' | 'muted';
}) {
  const toneClass = {
    good: 'border-emerald-500/25 bg-emerald-500/8 text-emerald-200',
    warn: 'border-amber-500/25 bg-amber-500/8 text-amber-200',
    bad: 'border-rose-500/25 bg-rose-500/8 text-rose-200',
    muted: 'border-default/70 bg-surface/70 text-primary',
  }[tone];
  return (
    <div className={`rounded-xl border px-4 py-4 ${toneClass}`}>
      <div className="text-xs text-secondary">{label}</div>
      <div className="mt-2 text-2xl font-bold">{value}</div>
      <div className="mt-1 text-xs leading-relaxed text-muted">{hint}</div>
    </div>
  );
}

function TemplateAvailabilityCell({
  status,
  enabled,
}: {
  status: AuditModelStatusItem['template_probe'] | null;
  enabled: boolean;
}) {
  if (!enabled) {
    return <span className="text-sm text-muted">不可测</span>;
  }
  if (!status || status.total <= 0) {
    return <span className="text-sm text-muted">暂无模板样本</span>;
  }
  return (
    <div className="space-y-1">
      <AvailabilityBadge value={status.availability} enabled={enabled} />
      <div className="text-xs text-muted">
        正常 {status.success + status.degraded}/{status.total}
        {status.timeout || status.no_response ? ` · 超时 ${status.timeout} · 无响应 ${status.no_response}` : ''}
      </div>
    </div>
  );
}

function QuickProbeCompareCell({
  status,
  enabled,
}: {
  status: AuditModelStatusItem['quick_probe'] | null;
  enabled: boolean;
}) {
  if (!enabled) return <span className="text-sm text-muted">不可测</span>;
  if (!status || status.status === 'missing') return <span className="text-sm text-muted">暂无 quick-probe</span>;
  const hasBaseline = status.baseline_mode && status.baseline_mode !== 'candidate_only';
  return (
    <div className="space-y-1 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <SourceStatusBadge status={status.status} />
        {status.score ? <span className="font-mono text-primary">{Math.round(status.score)} 分</span> : null}
      </div>
      <div className="text-muted">
        {hasBaseline ? `基线 ${status.baseline_mode}` : '无 baseline 对照'}
        {status.dimensions_total ? ` · 维度 ${status.dimensions_pass || 0}/${status.dimensions_total} 通过` : ''}
      </div>
      {status.active_weight ? <div className="text-muted">active weight {status.active_weight}</div> : null}
    </div>
  );
}
```

- [ ] **Step 7: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/ProviderPage.tsx
git commit -m "feat(frontend): show public monitoring summaries"
```

---

## Task 6: Runtime Verification And Embed Build

**Files:**
- No source edits unless verification finds a concrete bug.

- [ ] **Step 1: Run focused tests**

Run from repo root:

```bash
go test ./internal/api ./internal/audit ./internal/storage ./internal/config
```

Expected:

```text
ok  	monitor/internal/api
ok  	monitor/internal/audit
ok  	monitor/internal/storage
ok  	monitor/internal/config
```

- [ ] **Step 2: Build frontend and embed assets**

Run:

```bash
cd frontend
npm run build
cd ..
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -R frontend/dist internal/api/frontend/
```

Expected:

```text
✓ built
```

- [ ] **Step 3: Build server**

Run:

```bash
go build -o /tmp/relay-pulse-server ./cmd/server
```

Expected: command exits 0.

- [ ] **Step 4: Start local server**

If port 18080 is occupied:

```bash
pid=$(lsof -tiTCP:18080 -sTCP:LISTEN || true)
if [ -n "$pid" ]; then kill $pid; sleep 1; fi
```

Then start from the repo root:

```bash
PORT=18080 /tmp/relay-pulse-server config.yaml
```

Expected log includes:

```text
监测服务已启动 ... web_ui=http://localhost:18080
```

- [ ] **Step 5: Verify model-status JSON**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' \
  | jq '{count:.data.meta.count, summary:.data.meta.summary, first:.data.items[0] | {model, template_probe, quick_probe}}'
```

Expected:

```text
summary.template_probe_total exists
summary.template_availability exists
first.template_probe.total exists
first.quick_probe.dimensions_total exists when quick_probe has a run
```

- [ ] **Step 6: Verify page route renders**

Run:

```bash
curl -sS -o /tmp/provider-page.html -w '%{http_code} %{content_type}\n' \
  'http://127.0.0.1:18080/p/alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E?service=cc&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E'
```

Expected:

```text
200 text/html; charset=utf-8
```

- [ ] **Step 7: Verify built ProviderPage asset contains public labels**

Run:

```bash
rg -n "整体模板可用率|正常请求|超时 / 无响应|Baseline 对比|模板可用率 24H" frontend/dist/assets internal/api/frontend/dist/assets
```

Expected: matches in the built frontend asset.

- [ ] **Step 8: Commit verification fixes only if needed**

If verification required source edits:

```bash
git add internal frontend
git commit -m "fix(frontend): stabilize public data display"
```

Do not create an empty commit.

---

## Acceptance Criteria

| ID | Acceptance |
|---|---|
| A1 | `/api/audit/model-status` returns `meta.summary` with template availability, timeout/no-response counts, production totals, quick-probe totals, and baseline compared count. |
| A2 | Each model-status item returns template probe window metrics from `probe_history`: `total/success/degraded/timeout/no_response/availability/window`. |
| A3 | Each model-status item returns quick-probe compare summary: score, active weight, baseline mode, dimension pass/fail/skip counts. |
| A4 | Provider detail page renders overall public summary cards for template availability, normal requests, timeout/no-response, and baseline compare. |
| A5 | Provider model table shows template availability using the original template probe history, not only rpdiag or static placeholders. |
| A6 | Provider model table shows quick-probe/baseline comparison status and keeps links to detection history and single-run compare pages. |
| A7 | `/detect` remains a methodology page and does not regain a detection history table. |
| A8 | Frontend builds successfully and Go focused tests pass. |
| A9 | Local runtime on `http://127.0.0.1:18080` returns 200 for a provider detail page and JSON fields for `model-status`. |

---

## Self-Review

Spec coverage:

- “整体的可用率，使用原来模版的测评方式”：Task 2 reads `probe_history` via `GetHistory` and computes template availability from original template probe records.
- “持续发送正常的请求，验证是否存在超时，没有响应的内容”：Task 2 exposes `timeout` and `no_response` counts from probe history; Task 5 renders them publicly.
- “发送probe请求，获取所有渠道的测评数据与基线数据的对比结果”：existing backfill endpoints remain the execution mechanism; Task 3 surfaces quick-probe/baseline compare summaries per model; Task 5 renders them with detection history and compare links.
- “页面正常渲染，数据正常显示”：Task 6 validates build, API JSON, page route, and built labels.
- “历史执行有痕迹”：this plan is saved under `docs/superpowers/plans/` and each task commits independently.

Placeholder scan:

- No TBD/TODO/fill-later placeholders.
- All test snippets, commands, and expected outputs are explicit.

Type consistency:

- Go fields use snake_case JSON matching TypeScript interfaces.
- `template_probe` and `quick_probe` names preserve the existing API contract.
- Frontend helper types reference `AuditModelStatusItem['template_probe']` and `AuditModelStatusItem['quick_probe']`, matching Task 4.
