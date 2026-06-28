# Quick Probe Coverage Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补齐 `quick-probe-v1` 覆盖率闭环：页面能明确展示哪些渠道已测、哪些缺 key、哪些等待 baseline；批量 probe 只对具备 `audit_targets.base_url/api_key` 的目标执行，避免误用全局凭证。

**Architecture:** 不修改 `new-api`，不读取 `new-api` DB。继续使用现有 `/api/audit/diagnostics/backfill` 和 `/api/audit/model-status`，新增覆盖率聚合和页面展示；批量执行保持上限和只读边界，缺少 key 的目标只展示 `missing_credential`，不发请求。

**Tech Stack:** Go, Gin, SQLite/PostgreSQL storage interfaces, existing quick-probe runner, React, TypeScript, existing provider/home pages.

---

## Evidence Base

当前本地证据：

| 项 | 当前值 |
|---|---|
| `audit_targets` 总数 | 138 model targets |
| enabled targets | 42 |
| 有 `base_url` | 138 |
| 有 `api_key` | 5 |
| `model-status` quick_probe_done | 1 |
| `model-status` quick_probe_failed | 5 |
| `model-status` quick_probe_missing | 132 |
| `model-status` baseline_compared | 4 |

结论：

1. 页面已经能展示真实数据，但全量 quick-probe 还没有完成。
2. 缺失的主要原因是本地 `audit_targets.api_key` 覆盖不足。
3. 正式诊断不得 fallback 到 `NEWAPI_ACCESS_TOKEN`。
4. “发送所有渠道 probe”必须拆成两个状态：可执行目标批量执行、不可执行目标展示缺 key 原因。

## File Structure

Modify:

- `internal/api/audit_types.go`
  - 新增 quick-probe coverage response 类型，展示 target 总数、enabled 数、有 key 数、可执行数、done/failed/missing、baseline compared。

- `internal/api/audit_handler.go`
  - 新增 `GET /api/audit/diagnostics/coverage`，只读聚合覆盖率。
  - 不触发 probe，不返回明文 key。

- `internal/api/audit_handler_test.go`
  - 覆盖有 key / 无 key / done / failed / missing 的覆盖率统计。

- `frontend/src/hooks/useAuditDiagnosticCoverage.ts`
  - 新增 coverage hook。

- `frontend/src/types/audit.ts`
  - 新增 coverage 类型。

- `frontend/src/pages/ProviderPage.tsx`
  - 在详情页公开展示 quick-probe 覆盖状态：已测、失败、缺 key、缺 baseline。

- `frontend/src/App.tsx`
  - 首页摘要补充“可执行 probe 目标 / 缺 key 目标”，避免把缺失误读为系统没数据。

- `docs/superpowers/plans/2026-06-28-quick-probe-coverage-closure.md`
  - 执行过程逐步勾选并记录本地验证。

No changes:

- 不自动对全部 138 个 target 发起请求。
- 不用全局 `NEWAPI_ACCESS_TOKEN` 做诊断。
- 不提交真实 API key。
- 不提交 `monitor.db`、`.env`、`frontend/dist`、`internal/api/frontend`。

---

## Task 1: Add Coverage API

**Files:**
- Modify: `internal/api/audit_types.go`
- Modify: `internal/api/audit_handler.go`
- Modify: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add failing API test**

Add `TestAuditDiagnosticCoverageReportsCredentialAndProbeState` in `internal/api/audit_handler_test.go`.

The test must:

1. Insert three targets:
   - enabled + key + diagnostic done
   - enabled + no key + no diagnostic
   - disabled + key + no diagnostic
2. Save one `diagnostic_run` for the first target.
3. Call `GET /api/audit/diagnostics/coverage`.
4. Assert:
   - `targets_total = 3`
   - `targets_enabled = 2`
   - `credential_configured = 2`
   - `runnable_targets = 1`
   - `quick_probe_done = 1`
   - `quick_probe_missing = 2`
   - response does not contain the plaintext key.

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/api -run TestAuditDiagnosticCoverageReportsCredentialAndProbeState -count=1
```

Expected: fails because route/type does not exist.

- [ ] **Step 3: Add response types**

Add:

```go
type auditDiagnosticCoverageResponse struct {
	TargetsTotal          int `json:"targets_total"`
	TargetsEnabled        int `json:"targets_enabled"`
	CredentialConfigured  int `json:"credential_configured"`
	BaseURLConfigured     int `json:"base_url_configured"`
	RunnableTargets       int `json:"runnable_targets"`
	QuickProbeDone        int `json:"quick_probe_done"`
	QuickProbeFailed      int `json:"quick_probe_failed"`
	QuickProbeMissing     int `json:"quick_probe_missing"`
	BaselineCompared      int `json:"baseline_compared"`
	MissingCredential     int `json:"missing_credential"`
	MissingBaseURL        int `json:"missing_base_url"`
}
```

- [ ] **Step 4: Implement handler and route**

Add `GetAuditDiagnosticCoverage` and register:

```go
r.GET("/api/audit/diagnostics/coverage", h.GetAuditDiagnosticCoverage)
```

The handler must use `ListAuditTargets`, `ListDiagnosticRuns`, and existing quick-probe status helpers; it must not execute probes.

- [ ] **Step 5: Run focused test**

```bash
go test ./internal/api -run TestAuditDiagnosticCoverageReportsCredentialAndProbeState -count=1
```

Expected: pass.

- [ ] **Step 6: Commit Task 1**

```bash
git add internal/api/audit_types.go internal/api/audit_handler.go internal/api/audit_handler_test.go
git commit -m "feat(audit): expose quick probe coverage"
```

## Task 2: Show Coverage In Public Pages

**Files:**
- Modify: `frontend/src/types/audit.ts`
- Create: `frontend/src/hooks/useAuditDiagnosticCoverage.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Add frontend type and hook**

Add `AuditDiagnosticCoverageResponse` and `useAuditDiagnosticCoverage`.

- [ ] **Step 2: Add home coverage summary**

Homepage must show:

- `可执行 probe`
- `缺 key`
- `已完成 quick-probe`
- `Baseline 对比`

- [ ] **Step 3: Add provider detail coverage hint**

Provider detail must show coverage context near public metric cards:

- done / failed / missing
- missing credential count
- runnable target count

- [ ] **Step 4: Run frontend tests and build**

```bash
cd frontend
npm test -- src/utils/auditModelStatusSummary.test.ts src/hooks/useAuditModelStatus.test.ts src/utils/auditServiceBoundary.test.ts
npm run build
```

- [ ] **Step 5: Commit Task 2**

```bash
git add frontend/src/types/audit.ts frontend/src/hooks/useAuditDiagnosticCoverage.ts frontend/src/App.tsx frontend/src/pages/ProviderPage.tsx
git commit -m "feat(frontend): show quick probe coverage"
```

## Task 3: Runtime Verification

**Files:**
- Modify: `docs/superpowers/plans/2026-06-28-quick-probe-coverage-closure.md`

- [ ] **Step 1: Run backend and frontend checks**

```bash
go test ./internal/api ./internal/audit ./internal/storage ./internal/config
cd frontend && npm run build
```

- [ ] **Step 2: Restart local 18080 and verify API**

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/diagnostics/coverage' | jq
```

Expected on current local SQLite: `missing_credential` is greater than 0 and `runnable_targets` is much smaller than `targets_total`.

- [ ] **Step 3: Verify rendered pages**

Use headless Chrome or browser to verify `/` and `/p/:provider` render coverage labels.

- [ ] **Step 4: Record observed output and push**

Update this plan with exact verification values, commit, then push `main`.

---

## Boundary Note

This plan does not promise that all channels can be probed immediately. The current database proves most synced targets lack `api_key`; those targets must be shown as `missing_credential` until credentials are configured. Only targets with both `base_url` and `api_key` are eligible for actual quick-probe execution.
