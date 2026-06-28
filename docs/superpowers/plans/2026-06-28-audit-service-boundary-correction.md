# Audit Service Boundary Correction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正服务商详情页和审计 API 的 service 边界，确保页面查询真实审计 service 的数据，同时证明 Go 探测请求继续使用 `audit_targets.base_url` 与 `audit_targets.api_key`。

**Architecture:** 保持历史 plan 的后端边界：quick-probe 和模板补洞由 Go 后端执行，目标配置只来自 `audit_targets.base_url/api_key`，不得 fallback 到 `NEWAPI_BASE_URL/NEWAPI_ACCESS_TOKEN`。前端保留 `cc/cx` 作为页面视图分类，但调用审计数据 API 时必须传 `currentSnapshot.service` 里的真实审计 service，例如 `anthropic/openai`；公开页面不得展示 `probe_fallback`、同步目标、渠道快照等内部控制面告警。

**Tech Stack:** Go, Gin, SQLite/PostgreSQL storage interfaces, existing audit diagnostic runner, React, TypeScript, Vitest, Vite.

---

## Evidence Base

执行本 plan 前必须先读取这些文件，不允许只按页面现象判断：

| 文件 | 必须确认的事实 |
|---|---|
| `docs/relaypulse-probe-requirements-zh.md` | 主动检测、模板补洞和 quick-probe-v1 必须使用 `audit_targets.base_url` 和 `audit_targets.api_key`；`NEWAPI_ACCESS_TOKEN` 只用于读取 new-api 渠道和日志。 |
| `docs/superpowers/plans/2026-06-28-fix-diagnostic-target-baseurl.md` | 历史修复目标是 Go 探测链路禁止 fallback 到全局 `NEWAPI_BASE_URL`。 |
| `docs/superpowers/plans/2026-06-28-public-data-display.md` | 服务商详情页要展示真实监测数据，但不能改变 `/detect/history` 和 `/detect/compare/:runId` 职责。 |
| `internal/api/audit_handler.go` | `PostAuditDiagnosticSubmit`、`PostAuditDiagnosticBackfill`、`PostAuditTemplateProbeBackfill` 是 Go 探测入口；`GetAuditModelStatus` 是页面聚合数据接口。 |
| `frontend/src/pages/ProviderPage.tsx` | 当前页面把 `inferAuditServiceType(currentSnapshot)` 的视图 service 传给审计 API，这是 `cc/cx` 与 `anthropic/openai` 混用的来源。 |

当前本地证据示例：

```bash
sqlite3 monitor.db "select provider, service, channel, model, base_url, length(api_key), source from audit_targets where provider='alan-官key直连' and channel='80:alan-官key直连';"
```

期望能看到：

```text
alan-官key直连|anthropic|80:alan-官key直连|claude-opus-4-8|http://72.61.77.104:4000|51|manual_baseline
```

对比当前错误查询：

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=cc&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '.data.meta.count'
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '.data.meta.count'
```

当前应分别为：

```text
0
5
```

这说明页面查询 service 错误，不说明 Go 探测未走 `base_url`。

## File Structure

Modify:

- `frontend/src/pages/ProviderPage.tsx`
  - 用真实审计 service 查询 `useAuditDiagnosticLatest` 和 `useAuditModelStatus`。
  - 移除对外页面的 `probe_fallback` 内部告警展示。
  - 保留 `cc/cx` 只作为页面服务视图和 rpdiag join 视图。

- `frontend/src/utils/auditServiceBoundary.ts`
  - 新增小工具函数，明确区分 audit data service 与 view service。

- `frontend/src/utils/auditServiceBoundary.test.ts`
  - 用 Vitest 固化 `anthropic/openai` 不应被转换为 `cc/cx` 后传给审计 API。

- `internal/api/audit_handler_test.go`
  - 增加 Go 后端回归测试：当 target 存储 service 为 `anthropic` 时，用 `service=cc` 提交 diagnostic 必须找不到 target；用 `service=anthropic` 才能发起请求，并且请求 URL 必须是 `audit_targets.base_url`。

No changes:

- 不修改 `new-api`。
- 不新增探测框架。
- 不让前端传递 `base_url/api_key`。
- 不把 `probe_fallback` 当作渠道检测凭证来源。
- 不提交 `frontend/dist` 或 `internal/api/frontend`，它们是本地运行产物。

---

## Task 1: Add Frontend Boundary Utility

**Files:**
- Create: `frontend/src/utils/auditServiceBoundary.ts`
- Create: `frontend/src/utils/auditServiceBoundary.test.ts`

- [ ] **Step 1: Create a failing test for real audit service**

Create `frontend/src/utils/auditServiceBoundary.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

import { getAuditDataService } from './auditServiceBoundary';
import type { AuditChannelSnapshot } from '../types/audit';

function snapshot(service: string): Pick<AuditChannelSnapshot, 'service'> {
  return { service };
}

describe('getAuditDataService', () => {
  it('keeps anthropic as the audit API service', () => {
    expect(getAuditDataService(snapshot('anthropic'))).toBe('anthropic');
  });

  it('keeps openai as the audit API service', () => {
    expect(getAuditDataService(snapshot('openai'))).toBe('openai');
  });

  it('trims whitespace without converting to cc or cx', () => {
    expect(getAuditDataService(snapshot('  anthropic  '))).toBe('anthropic');
  });

  it('returns undefined for missing service', () => {
    expect(getAuditDataService(null)).toBeUndefined();
    expect(getAuditDataService({ service: '   ' })).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the focused frontend test and verify it fails**

Run:

```bash
cd frontend
npm test -- src/utils/auditServiceBoundary.test.ts
```

Expected:

```text
FAIL  src/utils/auditServiceBoundary.test.ts
Error: Failed to resolve import "./auditServiceBoundary"
```

- [ ] **Step 3: Create the utility implementation**

Create `frontend/src/utils/auditServiceBoundary.ts`:

```ts
import type { AuditChannelSnapshot } from '../types/audit';

export function getAuditDataService(
  snapshot?: Pick<AuditChannelSnapshot, 'service'> | null,
): string | undefined {
  const service = snapshot?.service?.trim();
  return service || undefined;
}
```

- [ ] **Step 4: Run the focused frontend test and verify it passes**

Run:

```bash
cd frontend
npm test -- src/utils/auditServiceBoundary.test.ts
```

Expected:

```text
PASS  src/utils/auditServiceBoundary.test.ts
```

- [ ] **Step 5: Commit Task 1**

Run:

```bash
git add frontend/src/utils/auditServiceBoundary.ts frontend/src/utils/auditServiceBoundary.test.ts
git commit -m "test(frontend): define audit service boundary"
```

---

## Task 2: Use Real Audit Service In ProviderPage

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`
- Test: `frontend/src/utils/auditServiceBoundary.test.ts`

- [ ] **Step 1: Add the utility import**

In `frontend/src/pages/ProviderPage.tsx`, add this import near the other utility imports:

```ts
import { getAuditDataService } from '../utils/auditServiceBoundary';
```

- [ ] **Step 2: Define `auditDataService` from `currentSnapshot.service`**

After `currentSnapshot` is defined, add:

```ts
  const auditDataService = useMemo(() => getAuditDataService(currentSnapshot), [currentSnapshot]);
```

The surrounding block should look like:

```ts
  const currentSnapshot = useMemo(() => {
    return sourceFilteredSnapshots.find((snapshot) => snapshot.channel === selectedChannel) || null;
  }, [sourceFilteredSnapshots, selectedChannel]);

  const auditDataService = useMemo(() => getAuditDataService(currentSnapshot), [currentSnapshot]);
```

- [ ] **Step 3: Keep `cc/cx` only for view joins**

Leave this existing code unchanged because it is for monitor/rpdiag view joins:

```ts
  const matchedMonitor = useMemo<ProcessedMonitorData | undefined>(() => {
    if (!currentSnapshot) return undefined;
    const serviceType = inferAuditServiceType(currentSnapshot);
    const channelName = extractAuditChannelName(currentSnapshot.channel).toLowerCase();
    return monitorIndex.get(
      buildAuditMonitorMatchKey(currentSnapshot.provider, serviceType, channelName),
    );
  }, [currentSnapshot, monitorIndex]);
```

Also leave `currentRpdiag` using `inferAuditServiceType(currentSnapshot)`.

- [ ] **Step 4: Change diagnostic latest API to use real audit service**

Replace:

```ts
  } = useAuditDiagnosticLatest({
    provider: currentSnapshot?.provider,
    service: currentSnapshot ? inferAuditServiceType(currentSnapshot) : undefined,
    channel: currentSnapshot?.channel,
    includeFiltered: true,
    limit: 10,
  });
```

with:

```ts
  } = useAuditDiagnosticLatest({
    provider: currentSnapshot?.provider,
    service: auditDataService,
    channel: currentSnapshot?.channel,
    includeFiltered: true,
    limit: 10,
  });
```

- [ ] **Step 5: Change model status API to use real audit service**

Replace:

```ts
  } = useAuditModelStatus({
    provider: currentSnapshot?.provider,
    service: currentSnapshot ? inferAuditServiceType(currentSnapshot) : undefined,
    channel: currentSnapshot?.channel,
    window: '24h',
  });
```

with:

```ts
  } = useAuditModelStatus({
    provider: currentSnapshot?.provider,
    service: auditDataService,
    channel: currentSnapshot?.channel,
    window: '24h',
  });
```

- [ ] **Step 6: Ensure history links prefer real audit service**

In the `historyParams` block, replace:

```ts
        historyParams.set('service', sourceStatus?.service || currentSnapshot.service || selectedService);
```

with:

```ts
        historyParams.set('service', sourceStatus?.service || auditDataService || currentSnapshot.service);
```

- [ ] **Step 7: Run TypeScript build**

Run:

```bash
cd frontend
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 8: Verify no audit data API call still uses view service**

Run:

```bash
rg -n "service: currentSnapshot \\? inferAuditServiceType\\(currentSnapshot\\)|service: inferAuditServiceType\\(currentSnapshot\\)" frontend/src/pages/ProviderPage.tsx
```

Expected: no output.

- [ ] **Step 9: Commit Task 2**

Run:

```bash
git add frontend/src/pages/ProviderPage.tsx
git commit -m "fix(frontend): query audit APIs with stored service"
```

---

## Task 3: Remove Internal Probe Runtime Warning From Public Provider Page

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Remove the sync status import**

Remove this import:

```ts
import { useAuditSyncStatus } from '../hooks/useAuditSyncStatus';
```

- [ ] **Step 2: Remove the sync status hook**

Remove:

```ts
  const { data: syncStatus } = useAuditSyncStatus();
```

- [ ] **Step 3: Remove `showProbeWarning`**

Delete this block:

```ts
  const showProbeWarning = useMemo(() => {
    if (!currentSnapshot || latestDiagnosticsLoading) return false;
    if (latestDiagnostics.some((item) => item.usable)) return false;
    return Boolean(syncStatus?.probe_runtime?.warning);
  }, [currentSnapshot, latestDiagnostics, latestDiagnosticsLoading, syncStatus]);
```

- [ ] **Step 4: Remove the public warning section**

Delete this JSX block:

```tsx
          {showProbeWarning && (
            <section className="mb-6 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-4 text-sm">
              <div className="font-semibold text-amber-200">当前通道尚无有效检测样本</div>
              <p className="mt-1 text-amber-100 leading-relaxed">
                {syncStatus?.probe_runtime.warning}
              </p>
              <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-amber-200/90">
                <span>同步目标 {syncStatus?.targets.enabled ?? 0}/{syncStatus?.targets.total ?? 0}</span>
                <span>渠道快照 {syncStatus?.channels?.channel_count ?? 0}</span>
                <span>凭证模式 {syncStatus?.probe_runtime.probe_credential_mode ?? 'missing'}</span>
              </div>
            </section>
          )}
```

- [ ] **Step 5: Replace row fallback text**

Replace:

```tsx
                            <span className="text-muted text-sm">{showProbeWarning ? '等待有效样本' : '暂无检测记录'}</span>
```

with:

```tsx
                            <span className="text-muted text-sm">暂无 quick-probe 样本</span>
```

- [ ] **Step 6: Verify internal runtime warning text is absent from ProviderPage**

Run:

```bash
rg -n "probe_runtime|probe_fallback|同步目标|渠道快照|凭证模式|当前通道尚无有效检测样本|showProbeWarning|useAuditSyncStatus" frontend/src/pages/ProviderPage.tsx
```

Expected: no output.

- [ ] **Step 7: Build frontend**

Run:

```bash
cd frontend
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 8: Commit Task 3**

Run:

```bash
git add frontend/src/pages/ProviderPage.tsx
git commit -m "fix(frontend): hide internal probe runtime warning"
```

---

## Task 4: Add Go Regression For Stored Service And BaseURL

**Files:**
- Modify: `internal/api/audit_handler_test.go`

- [ ] **Step 1: Add a failing test for view service mismatch**

In `internal/api/audit_handler_test.go`, add this test near `TestAuditDiagnosticSubmitUsesStoredBaseURLNotGlobalNewAPIBaseURL`:

```go
func TestAuditDiagnosticSubmitDoesNotTreatViewServiceAsStoredService(t *testing.T) {
	store := newAuditTestStore(t)
	if err := store.ReplaceAuditTargets([]storage.AuditTarget{{
		Provider:     "alan-官key直连",
		Service:      "anthropic",
		Channel:      "80:alan-官key直连",
		Model:        "claude-opus-4-8",
		RequestModel: "claude-opus-4-8",
		BaseURL:      "http://72.61.77.104:4000",
		APIKey:       "sk-channel-key",
		Enabled:      true,
	}}); err != nil {
		t.Fatalf("ReplaceAuditTargets: %v", err)
	}
	cfg := &config.AppConfig{
		NewAPI: config.NewAPIConfig{
			BaseURL: "https://global-newapi-must-not-be-used.example.com",
			UserID:  "sync-user",
		},
		Audit: config.AuditConfig{
			Diagnostics: config.DiagnosticsConfig{Enabled: boolPtr(true)},
		},
	}
	router := newAuditTestRouter(t, store, cfg)
	body := `{"provider":"alan-官key直连","service":"cc","channel":"80:alan-官key直连","model":"claude-opus-4-8","request_model":"claude-opus-4-8"}`
	req := httptest.NewRequest(http.MethodPost, "/api/audit/diagnostics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected service alias cc to be rejected, code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "audit target not found") {
		t.Fatalf("expected audit target not found for view service alias, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the focused test**

Run:

```bash
go test ./internal/api -run TestAuditDiagnosticSubmitDoesNotTreatViewServiceAsStoredService -count=1
```

Expected:

```text
ok  	monitor/internal/api
```

This test should pass on current backend code. It exists to document that `cc/cx` is not a backend audit service.

- [ ] **Step 3: Verify existing stored BaseURL test still passes**

Run:

```bash
go test ./internal/api -run 'TestAuditDiagnosticSubmitUsesStoredBaseURLNotGlobalNewAPIBaseURL|TestAuditDiagnosticSubmitDoesNotTreatViewServiceAsStoredService' -count=1
```

Expected:

```text
ok  	monitor/internal/api
```

- [ ] **Step 4: Commit Task 4**

Run:

```bash
git add internal/api/audit_handler_test.go
git commit -m "test(audit): document stored service boundary"
```

---

## Task 5: Runtime Verification Against Local Database

**Files:**
- No source changes.
- Local-only generated files: `frontend/dist`, `internal/api/frontend`, `/tmp/relay-pulse-server`.

- [ ] **Step 1: Build frontend**

Run:

```bash
cd frontend
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 2: Sync frontend dist into Go embed directory for local run**

Run from repo root:

```bash
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -R frontend/dist internal/api/frontend/
rg -n "整体模板可用率|Baseline 对比|模板可用率 24H" internal/api/frontend/dist/assets
```

Expected: `rg` prints matches inside `ProviderPage-*.js`.

- [ ] **Step 3: Build Go server**

Run:

```bash
go build -o /tmp/relay-pulse-server ./cmd/server
```

Expected: command exits 0.

- [ ] **Step 4: Restart local server in tmux**

Run:

```bash
pid=$(lsof -tiTCP:18080 -sTCP:LISTEN || true)
if [ -n "$pid" ]; then kill $pid; fi
/opt/homebrew/bin/tmux kill-session -t relay-pulse-18080 2>/dev/null || true
/opt/homebrew/bin/tmux new-session -d -s relay-pulse-18080 'cd /Users/gongxiude/Documents/github/relay-pulse && PORT=18080 /tmp/relay-pulse-server config.yaml'
sleep 1
curl -sS -o /tmp/health.txt -w '%{http_code}\n' http://127.0.0.1:18080/health
```

Expected:

```text
200
```

- [ ] **Step 5: Verify wrong service returns no audit data**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=cc&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '.data.meta.count'
```

Expected:

```text
0
```

- [ ] **Step 6: Verify real audit service returns data**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '{count:.data.meta.count, summary:.data.meta.summary}'
```

Expected:

```json
{
  "count": 5,
  "summary": {
    "total_models": 5,
    "enabled_models": 5,
    "template_probe_total": 1,
    "template_probe_success": 0,
    "template_probe_timeout": 0,
    "template_probe_no_response": 0,
    "template_availability": 0,
    "production_total": 1,
    "production_success": 1,
    "production_success_rate": 100,
    "quick_probe_done": 1,
    "quick_probe_failed": 1,
    "quick_probe_missing": 3,
    "baseline_compared": 2
  }
}
```

`production_total` may be higher than 1 on a live local database; it must be greater than 0 for this channel.

- [ ] **Step 7: Verify current ProviderPage asset no longer exposes internal warning**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/assets/ProviderPage-DrytdMdK.js' | rg -n "probe_fallback|同步目标|渠道快照|凭证模式|当前通道尚无有效检测样本"
```

Expected: no output.

- [ ] **Step 8: Verify current ProviderPage asset contains public metrics**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/assets/ProviderPage-DrytdMdK.js' | rg -o "整体模板可用率|正常请求|超时 / 无响应|Baseline 对比|模板可用率 24H" | sort | uniq
```

Expected:

```text
Baseline 对比
整体模板可用率
模板可用率 24H
正常请求
超时 / 无响应
```

- [ ] **Step 9: Verify served page loads new asset**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/p/alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E?service=cc&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E' | rg -o 'assets/index-[^" ]+\.js'
```

Expected: the printed `assets/index-*.js` must match the latest `frontend/dist/index.html` asset.

---

## Task 6: Full Verification And Push

**Files:**
- No code changes beyond Tasks 1-4.

- [ ] **Step 1: Run Go focused tests**

Run:

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

- [ ] **Step 2: Run frontend tests**

Run:

```bash
cd frontend
npm test -- src/utils/auditServiceBoundary.test.ts
```

Expected:

```text
PASS  src/utils/auditServiceBoundary.test.ts
```

- [ ] **Step 3: Run frontend build**

Run:

```bash
cd frontend
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 4: Check git status before push**

Run:

```bash
git status --short --branch
```

Expected before push:

```text
## <branch-name>
```

No `frontend/dist`, `internal/api/frontend`, `.env`, `monitor.db`, or `frontend/node_modules` files should appear.

- [ ] **Step 5: Merge to main if implemented in a worktree branch**

If execution used a branch named `audit-service-boundary-correction`, run from main worktree:

```bash
git fetch origin
git checkout main
git merge --no-ff audit-service-boundary-correction -m "Merge branch 'audit-service-boundary-correction'"
```

Expected:

```text
Merge made by the 'ort' strategy.
```

- [ ] **Step 6: Push main**

Run:

```bash
git push origin main
```

Expected:

```text
main -> main
```

---

## Self-Review

Spec coverage:

- `audit_targets.base_url/api_key` requirement is covered by Task 4 and runtime SQL verification in Task 5.
- Page data display requirement is covered by Task 2 and Task 5 public metrics asset verification.
- Internal `probe_fallback` warning removal is covered by Task 3 and Task 5 asset grep.
- Historical plan alignment is covered by the Evidence Base and the Go regression tests.
- No `new-api` modification is planned.

Placeholder scan:

- No `TBD`, `TODO`, `implement later`, or unspecified edge handling remains.
- Every code change step includes exact code or exact replacement text.
- Every verification step includes exact command and expected result.

Type consistency:

- `getAuditDataService()` returns `string | undefined`, matching `useAuditDiagnosticLatest` and `useAuditModelStatus` hook argument types.
- `AuditChannelSnapshot.service` is the only source for audit API service.
- `inferAuditServiceType()` remains only for frontend view grouping and rpdiag/monitor joins.

Execution note:

- The plan deliberately does not ask the frontend to pass `base_url` or `api_key`; Go remains the only layer that executes probes.
- The plan deliberately does not convert `cc` to `anthropic` in the backend; the backend stores and queries exact audit service values.
