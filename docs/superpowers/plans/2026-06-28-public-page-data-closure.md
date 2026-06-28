# Public Page Data Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让对外页面稳定展示真实监测数据：入口页展示来自 `new-api` 同步、生产日志和模板探测的可用率摘要；服务商详情页展示整体模板可用率、超时/无响应、quick-probe 与 baseline 对比；运行验证有文件化记录。

**Architecture:** 不新增平行探测体系，不修改 `new-api`。后端继续以 `/api/audit/model-status` 作为真实监测聚合接口；前端新增轻量全量汇总 hook，将 `model-status` 汇总接入首页行数据和入口概览。服务商详情页保留当前模型级展示，只补强空态、口径说明和可验证数据展示。

**Tech Stack:** Go, Gin, SQLite/PostgreSQL storage interfaces, React, TypeScript, Vitest, Vite, existing `StatusTable`, existing audit APIs.

---

## Evidence Base

执行前必须读取并确认：

| 文件 | 必须确认的事实 |
|---|---|
| `docs/relaypulse-probe-requirements-zh.md` | 首页必须作为入口页，服务商详情展示同步渠道和模型状态；稳定性优先来自 `new-api` 日志，日志不足由模板探测补洞，真实性由 `quick-probe-v1`。 |
| `docs/superpowers/plans/2026-06-28-public-data-display.md` | `/api/audit/model-status` 已扩展出 `production/template_probe/quick_probe` 和 `meta.summary`。 |
| `docs/superpowers/plans/2026-06-28-audit-service-boundary-correction.md` | 前端调用审计数据 API 时必须使用真实审计 service，例如 `anthropic/openai`，不能使用视图 service `cc/cx`。 |
| `frontend/src/App.tsx` | 首页当前用 `useAuditChannels` + `/api/status` 适配成 `StatusTable`，未直接消费 `model-status` 汇总。 |
| `frontend/src/pages/ProviderPage.tsx` | 服务商详情页已展示四张公开指标卡、模型状态、检测历史和 compare 入口。 |

当前已完成能力：

| 能力 | 当前证据 |
|---|---|
| 模板探测窗口汇总 | `auditTemplateProbeStatusResponse.total/success/degraded/timeout/no_response/availability` |
| quick-probe baseline 摘要 | `auditQuickProbeStatusResponse.baseline_mode/active_weight/dimensions_*` |
| 服务商详情页真实 service 查询 | `ProviderPage.tsx` 使用 `getAuditDataService(currentSnapshot)` |
| 本地验证样本 | `alan-官key直连` + `anthropic` 返回 `count=5`，`service=cc` 返回 0 |

## File Structure

Modify:

- `frontend/src/hooks/useAuditModelStatus.ts`
  - 保留现有 `useAuditModelStatus` 的严格过滤行为。
  - 新增 `useAuditModelStatusSummary`，允许无 provider/service/channel 时请求全量 `/api/audit/model-status?window=24h`。

- `frontend/src/utils/auditModelStatusSummary.ts`
  - 新增前端聚合工具：按 `provider + service + channel` 和 `provider + viewService` 汇总 `AuditModelStatusItem`。
  - 输出可直接覆盖首页行的 `uptime/status/history` 数据。

- `frontend/src/utils/auditModelStatusSummary.test.ts`
  - 固化生产日志优先、模板探测补洞、quick-probe 计数和 `anthropic -> cc` 视图映射的聚合逻辑。

- `frontend/src/App.tsx`
  - 首页调用 `useAuditModelStatusSummary({ window: '24h' })`。
  - `adaptAuditChannelsToMonitorData` 后，用真实汇总覆盖入口页行的可用率、状态和简化趋势。
  - 顶部展示真实数据覆盖率：服务商数、同步通道数、有生产日志模型数、有模板样本模型数、baseline 对比数。

- `frontend/src/utils/auditChannelAdapter.ts`
  - 如需要，导出已有 `splitAuditModels` 或新增小工具，不改变现有路由构造。

- `docs/superpowers/plans/2026-06-28-public-page-data-closure.md`
  - 执行过程中逐步勾选任务，记录本地接口和页面验证结果。

No changes:

- 不新增 `/audit` 或 `/audit/ranking` 作为核心入口。
- 不改 `new-api`。
- 不读取 `new-api` 数据库。
- 不提交 `frontend/dist`、`internal/api/frontend`、`.env`、`monitor.db`。
- 不在前端展示明文 API key。

---

## Task 1: Add Frontend Audit Summary Aggregator

**Files:**
- Create: `frontend/src/utils/auditModelStatusSummary.ts`
- Create: `frontend/src/utils/auditModelStatusSummary.test.ts`
- Modify: `frontend/src/types/index.ts`

- [x] **Step 1: Add processed row audit summary types**

In `frontend/src/types/index.ts`, add this interface near `ProcessedMonitorData` related interfaces:

```ts
export interface AuditDisplaySummary {
  source: 'audit_model_status';
  provider: string;
  service: string;
  viewService: string;
  channel?: string;
  totalModels: number;
  enabledModels: number;
  productionTotal: number;
  productionSuccess: number;
  productionSuccessRate: number;
  templateProbeTotal: number;
  templateProbeSuccess: number;
  templateProbeTimeout: number;
  templateProbeNoResponse: number;
  templateAvailability: number;
  quickProbeDone: number;
  quickProbeFailed: number;
  quickProbeMissing: number;
  baselineCompared: number;
}
```

Then add this optional field to `ProcessedMonitorData`:

```ts
  auditSummary?: AuditDisplaySummary | null;
```

- [x] **Step 2: Write the failing aggregator test**

Create `frontend/src/utils/auditModelStatusSummary.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

import {
  aggregateAuditModelStatusForChannels,
  buildAuditChannelSummaryKey,
  buildAuditProviderSummaryKey,
} from './auditModelStatusSummary';
import type { AuditChannelSnapshot, AuditModelStatusItem } from '../types/audit';

function channel(overrides: Partial<AuditChannelSnapshot>): AuditChannelSnapshot {
  return {
    id: 1,
    newapi_channel_id: 80,
    snapshot_at: 1,
    provider: 'alan-官key直连',
    service: 'anthropic',
    channel: '80:alan-官key直连',
    model: 'claude-opus-4-8,claude-sonnet-4-5',
    enabled: true,
    raw: { Status: 1 },
    ...overrides,
  };
}

function item(overrides: Partial<AuditModelStatusItem>): AuditModelStatusItem {
  return {
    provider: 'alan-官key直连',
    service: 'anthropic',
    channel: '80:alan-官key直连',
    model: 'claude-opus-4-8',
    request_model: 'claude-opus-4-8',
    enabled: true,
    production: {
      source: 'production_logs',
      status: 'ok',
      total: 10,
      success: 9,
      error: 1,
      timeout: 1,
      success_rate: 90,
      p95: 2,
      p99: 3,
      updated_at: 100,
    },
    template_probe: {
      source: 'template_probe',
      status: 'available',
      window: '24h',
      total: 2,
      success: 1,
      degraded: 1,
      timeout: 0,
      no_response: 0,
      availability: 100,
    },
    quick_probe: {
      source: 'quick_probe',
      status: 'done',
      usable: true,
      baseline_mode: 'registered_baseline',
    },
    ...overrides,
  };
}

describe('aggregateAuditModelStatusForChannels', () => {
  it('aggregates real audit service data into channel and provider summaries', () => {
    const result = aggregateAuditModelStatusForChannels(
      [channel({})],
      [
        item({ model: 'claude-opus-4-8' }),
        item({
          model: 'claude-sonnet-4-5',
          production: {
            source: 'production_logs',
            status: 'ok',
            total: 5,
            success: 5,
            error: 0,
            timeout: 0,
            success_rate: 100,
            p95: 1,
            p99: 1,
          },
          template_probe: {
            source: 'template_probe',
            status: 'unavailable',
            window: '24h',
            total: 1,
            success: 0,
            degraded: 0,
            timeout: 1,
            no_response: 1,
            availability: 0,
          },
          quick_probe: {
            source: 'quick_probe',
            status: 'missing',
            usable: false,
          },
        }),
      ],
    );

    const channelSummary = result.byChannel.get(
      buildAuditChannelSummaryKey('alan-官key直连', 'anthropic', '80:alan-官key直连'),
    );
    expect(channelSummary?.viewService).toBe('cc');
    expect(channelSummary?.productionTotal).toBe(15);
    expect(channelSummary?.productionSuccess).toBe(14);
    expect(channelSummary?.productionSuccessRate).toBeCloseTo(93.333, 2);
    expect(channelSummary?.templateProbeTotal).toBe(3);
    expect(channelSummary?.templateProbeSuccess).toBe(2);
    expect(channelSummary?.templateProbeTimeout).toBe(1);
    expect(channelSummary?.templateProbeNoResponse).toBe(1);
    expect(channelSummary?.templateAvailability).toBeCloseTo(66.666, 2);
    expect(channelSummary?.quickProbeDone).toBe(1);
    expect(channelSummary?.quickProbeMissing).toBe(1);
    expect(channelSummary?.baselineCompared).toBe(1);

    const providerSummary = result.byProvider.get(
      buildAuditProviderSummaryKey('alan-官key直连', 'cc'),
    );
    expect(providerSummary?.totalModels).toBe(2);
    expect(providerSummary?.enabledModels).toBe(2);
    expect(providerSummary?.productionSuccessRate).toBeCloseTo(93.333, 2);
  });
});
```

- [x] **Step 3: Run the failing test**

Run:

```bash
cd frontend
npm test -- src/utils/auditModelStatusSummary.test.ts
```

Expected:

```text
FAIL  src/utils/auditModelStatusSummary.test.ts
Error: Cannot find module './auditModelStatusSummary'
```

- [x] **Step 4: Implement the aggregator**

Create `frontend/src/utils/auditModelStatusSummary.ts`:

```ts
import type { AuditChannelSnapshot, AuditModelStatusItem } from '../types/audit';
import type { AuditDisplaySummary } from '../types';
import { canonicalize } from './monitorDataProcessor';
import { extractAuditChannelName, inferAuditServiceType } from './auditChannelAdapter';

export interface AuditSummaryIndexes {
  byChannel: Map<string, AuditDisplaySummary>;
  byProvider: Map<string, AuditDisplaySummary>;
  totals: AuditDisplaySummary;
}

export function buildAuditChannelSummaryKey(provider: string, service: string, channel: string): string {
  return `${canonicalize(provider)}|${service.trim().toLowerCase()}|${channel.trim().toLowerCase()}`;
}

export function buildAuditProviderSummaryKey(provider: string, viewService: string): string {
  return `${canonicalize(provider)}|${viewService.trim().toLowerCase()}`;
}

export function aggregateAuditModelStatusForChannels(
  channels: AuditChannelSnapshot[],
  items: AuditModelStatusItem[],
): AuditSummaryIndexes {
  const channelViewService = new Map<string, string>();
  channels.forEach((snapshot) => {
    channelViewService.set(
      buildAuditChannelSummaryKey(snapshot.provider, snapshot.service, snapshot.channel),
      inferAuditServiceType(snapshot),
    );
  });

  const byChannel = new Map<string, AuditDisplaySummary>();
  const byProvider = new Map<string, AuditDisplaySummary>();
  const totals = emptySummary('全部', 'all', 'all');

  items.forEach((item) => {
    const channelKey = buildAuditChannelSummaryKey(item.provider, item.service, item.channel);
    const viewService = channelViewService.get(channelKey) || inferViewServiceFromItem(item);
    const channelSummary = getOrCreateSummary(
      byChannel,
      channelKey,
      item.provider,
      item.service,
      viewService,
      item.channel,
    );
    addItem(channelSummary, item);

    const providerKey = buildAuditProviderSummaryKey(item.provider, viewService);
    const providerSummary = getOrCreateSummary(
      byProvider,
      providerKey,
      item.provider,
      item.service,
      viewService,
    );
    addItem(providerSummary, item);
    addItem(totals, item);
  });

  finalizeSummary(totals);
  byChannel.forEach(finalizeSummary);
  byProvider.forEach(finalizeSummary);
  return { byChannel, byProvider, totals };
}

function emptySummary(
  provider: string,
  service: string,
  viewService: string,
  channel?: string,
): AuditDisplaySummary {
  return {
    source: 'audit_model_status',
    provider,
    service,
    viewService,
    channel,
    totalModels: 0,
    enabledModels: 0,
    productionTotal: 0,
    productionSuccess: 0,
    productionSuccessRate: 0,
    templateProbeTotal: 0,
    templateProbeSuccess: 0,
    templateProbeTimeout: 0,
    templateProbeNoResponse: 0,
    templateAvailability: 0,
    quickProbeDone: 0,
    quickProbeFailed: 0,
    quickProbeMissing: 0,
    baselineCompared: 0,
  };
}

function getOrCreateSummary(
  map: Map<string, AuditDisplaySummary>,
  key: string,
  provider: string,
  service: string,
  viewService: string,
  channel?: string,
): AuditDisplaySummary {
  const existing = map.get(key);
  if (existing) return existing;
  const created = emptySummary(provider, service, viewService, channel);
  map.set(key, created);
  return created;
}

function addItem(summary: AuditDisplaySummary, item: AuditModelStatusItem): void {
  summary.totalModels += 1;
  if (item.enabled) summary.enabledModels += 1;
  summary.productionTotal += item.production.total || 0;
  summary.productionSuccess += item.production.success || 0;
  summary.templateProbeTotal += item.template_probe.total || 0;
  summary.templateProbeSuccess += (item.template_probe.success || 0) + (item.template_probe.degraded || 0);
  summary.templateProbeTimeout += item.template_probe.timeout || 0;
  summary.templateProbeNoResponse += item.template_probe.no_response || 0;

  if (item.quick_probe.status === 'done') {
    summary.quickProbeDone += 1;
  } else if (item.quick_probe.status === 'missing') {
    summary.quickProbeMissing += 1;
  } else {
    summary.quickProbeFailed += 1;
  }
  if (item.quick_probe.baseline_mode && item.quick_probe.baseline_mode !== 'candidate_only') {
    summary.baselineCompared += 1;
  }
}

function finalizeSummary(summary: AuditDisplaySummary): void {
  summary.productionSuccessRate =
    summary.productionTotal > 0 ? (summary.productionSuccess / summary.productionTotal) * 100 : 0;
  summary.templateAvailability =
    summary.templateProbeTotal > 0 ? (summary.templateProbeSuccess / summary.templateProbeTotal) * 100 : 0;
}

function inferViewServiceFromItem(item: AuditModelStatusItem): string {
  const probe = `${item.provider} ${item.service} ${item.channel} ${item.model}`.toLowerCase();
  if (probe.includes('anthropic') || probe.includes('claude')) return 'cc';
  if (probe.includes('gemini') || probe.includes('google')) return 'gm';
  return 'cx';
}

export function chooseAuditDisplayAvailability(summary?: AuditDisplaySummary | null): number {
  if (!summary) return -1;
  if (summary.productionTotal > 0) return roundAvailability(summary.productionSuccessRate);
  if (summary.templateProbeTotal > 0) return roundAvailability(summary.templateAvailability);
  return -1;
}

export function buildAuditDisplayHistory(summary?: AuditDisplaySummary | null): Array<{
  index: number;
  status: 'AVAILABLE' | 'DEGRADED' | 'UNAVAILABLE' | 'MISSING';
  timestamp: string;
  timestampNum: number;
  latency: number;
  availability: number;
  statusCounts: { available: number; unavailable: number; degraded: number; missing: number };
}> {
  const availability = chooseAuditDisplayAvailability(summary);
  if (availability < 0) return [];
  const status = availability >= 99.5 ? 'AVAILABLE' : availability > 0 ? 'DEGRADED' : 'UNAVAILABLE';
  const now = Math.floor(Date.now() / 1000);
  return [2, 1, 0].map((offset) => ({
    index: 2 - offset,
    status,
    timestamp: new Date((now - offset * 3600) * 1000).toISOString(),
    timestampNum: now - offset * 3600,
    latency: 0,
    availability,
    statusCounts: {
      available: status === 'AVAILABLE' ? 1 : 0,
      unavailable: status === 'UNAVAILABLE' ? 1 : 0,
      degraded: status === 'DEGRADED' ? 1 : 0,
      missing: 0,
    },
  }));
}

function roundAvailability(value: number): number {
  return Math.round(value * 100) / 100;
}
```

- [x] **Step 5: Run the aggregator test**

Run:

```bash
cd frontend
npm test -- src/utils/auditModelStatusSummary.test.ts
```

Expected:

```text
PASS  src/utils/auditModelStatusSummary.test.ts
```

- [x] **Step 6: Commit Task 1**

Run:

```bash
git add frontend/src/types/index.ts frontend/src/utils/auditModelStatusSummary.ts frontend/src/utils/auditModelStatusSummary.test.ts
git commit -m "test(frontend): aggregate audit model status summaries"
```

---

## Task 2: Add Global Model Status Hook

**Files:**
- Modify: `frontend/src/hooks/useAuditModelStatus.ts`
- Test: `frontend/src/hooks/useAuditModelStatus.test.ts`

- [x] **Step 1: Write hook behavior test**

Create `frontend/src/hooks/useAuditModelStatus.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

function buildModelStatusQuery({
  provider,
  service,
  channel,
  window = '24h',
}: {
  provider?: string;
  service?: string;
  channel?: string;
  window?: string;
}): string {
  const params = new URLSearchParams();
  if (provider) params.set('provider', provider);
  if (service) params.set('service', service);
  if (channel) params.set('channel', channel);
  params.set('window', window);
  return params.toString();
}

describe('model status query', () => {
  it('can build a global summary query without provider/service/channel', () => {
    expect(buildModelStatusQuery({ window: '24h' })).toBe('window=24h');
  });

  it('keeps filtered detail query parameters when provided', () => {
    expect(
      buildModelStatusQuery({
        provider: 'alan-官key直连',
        service: 'anthropic',
        channel: '80:alan-官key直连',
        window: '24h',
      }),
    ).toBe(
      'provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h',
    );
  });
});
```

This test intentionally duplicates the query helper first. Step 3 will move the helper into production code and update the test import.

- [x] **Step 2: Run the test**

Run:

```bash
cd frontend
npm test -- src/hooks/useAuditModelStatus.test.ts
```

Expected:

```text
PASS  src/hooks/useAuditModelStatus.test.ts
```

- [x] **Step 3: Export the shared query builder and add global hook**

In `frontend/src/hooks/useAuditModelStatus.ts`, add:

```ts
export function buildAuditModelStatusQuery({
  provider,
  service,
  channel,
  window = '24h',
}: UseAuditModelStatusArgs): string {
  const params = new URLSearchParams();
  if (provider) params.set('provider', provider);
  if (service) params.set('service', service);
  if (channel) params.set('channel', channel);
  params.set('window', window);
  return params.toString();
}
```

Replace the duplicated query construction in `useAuditModelStatus` with:

```ts
  const query = useMemo(
    () => buildAuditModelStatusQuery({ provider, service, channel, window }),
    [provider, service, channel, window],
  );
```

Then add this hook below `useAuditModelStatus`:

```ts
export function useAuditModelStatusSummary({
  window = '24h',
}: {
  window?: string;
} = {}): UseAuditModelStatusResult {
  const [items, setItems] = useState<AuditModelStatusItem[]>([]);
  const [meta, setMeta] = useState<AuditModelStatusMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => buildAuditModelStatusQuery({ window }), [window]);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditModelStatusResponse>(`/api/audit/model-status?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
        setMeta(response?.data?.meta ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setMeta(null);
        setError(err instanceof Error ? err.message : '加载审计汇总失败');
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

- [x] **Step 4: Update the test to import production query builder**

Replace `frontend/src/hooks/useAuditModelStatus.test.ts` with:

```ts
import { describe, expect, it } from 'vitest';

import { buildAuditModelStatusQuery } from './useAuditModelStatus';

describe('buildAuditModelStatusQuery', () => {
  it('can build a global summary query without provider/service/channel', () => {
    expect(buildAuditModelStatusQuery({ window: '24h' })).toBe('window=24h');
  });

  it('keeps filtered detail query parameters when provided', () => {
    expect(
      buildAuditModelStatusQuery({
        provider: 'alan-官key直连',
        service: 'anthropic',
        channel: '80:alan-官key直连',
        window: '24h',
      }),
    ).toBe(
      'provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h',
    );
  });
});
```

- [x] **Step 5: Run hook test**

Run:

```bash
cd frontend
npm test -- src/hooks/useAuditModelStatus.test.ts
```

Expected:

```text
PASS  src/hooks/useAuditModelStatus.test.ts
```

- [x] **Step 6: Commit Task 2**

Run:

```bash
git add frontend/src/hooks/useAuditModelStatus.ts frontend/src/hooks/useAuditModelStatus.test.ts
git commit -m "feat(frontend): fetch global audit model status summary"
```

---

## Task 3: Wire Audit Summary Into Home Page

**Files:**
- Modify: `frontend/src/App.tsx`
- Test: `frontend/src/utils/auditModelStatusSummary.test.ts`

- [x] **Step 1: Import the summary hook and aggregator**

In `frontend/src/App.tsx`, change:

```ts
import { useAuditChannels } from './hooks/useAuditChannels';
```

to:

```ts
import { useAuditChannels } from './hooks/useAuditChannels';
import { useAuditModelStatusSummary } from './hooks/useAuditModelStatus';
```

Add utility imports:

```ts
import {
  aggregateAuditModelStatusForChannels,
  buildAuditChannelSummaryKey,
  buildAuditDisplayHistory,
  buildAuditProviderSummaryKey,
  chooseAuditDisplayAvailability,
} from './utils/auditModelStatusSummary';
```

- [x] **Step 2: Fetch global model-status summary**

In `App()`, after `useAuditChannels()`, add:

```ts
  const {
    items: auditStatusItems,
    meta: auditStatusMeta,
    loading: auditStatusLoading,
    error: auditStatusError,
  } = useAuditModelStatusSummary({ window: '24h' });
```

- [x] **Step 3: Build summary indexes**

After `rows` is defined, add:

```ts
  const auditSummaryIndexes = useMemo(() => {
    return aggregateAuditModelStatusForChannels(auditChannels, auditStatusItems);
  }, [auditChannels, auditStatusItems]);
```

- [x] **Step 4: Overlay channel summaries before provider aggregation**

Replace:

```ts
  const providerRows = useMemo(() => aggregateProviderRows(rows), [rows]);
```

with:

```ts
  const rowsWithAuditSummary = useMemo(() => {
    return rows.map((row) => {
      const summary = auditSummaryIndexes.byChannel.get(
        buildAuditChannelSummaryKey(row.providerName, row.auditSummary?.service || inferAuditServiceForHome(row), row.channel),
      );
      const availability = chooseAuditDisplayAvailability(summary);
      if (!summary || availability < 0) return row;
      return {
        ...row,
        auditSummary: summary,
        uptime: availability,
        history: row.history.length > 0 ? row.history : buildAuditDisplayHistory(summary),
        currentStatus:
          availability >= 99.5 ? 'AVAILABLE' : availability > 0 ? 'DEGRADED' : 'UNAVAILABLE',
      } satisfies ProcessedMonitorData;
    });
  }, [rows, auditSummaryIndexes]);

  const providerRows = useMemo(() => aggregateProviderRows(rowsWithAuditSummary, auditSummaryIndexes.byProvider), [rowsWithAuditSummary, auditSummaryIndexes]);
```

Then add this helper above `App()`:

```ts
function inferAuditServiceForHome(row: ProcessedMonitorData): string {
  const text = `${row.serviceType} ${row.channel} ${row.channelName} ${row.providerName}`.toLowerCase();
  if (text.includes('claude') || text.includes('anthropic') || row.serviceType === 'cc') return 'anthropic';
  if (text.includes('gemini') || text.includes('google') || row.serviceType === 'gm') return 'gemini';
  return 'openai';
}
```

- [x] **Step 5: Update provider aggregation to preserve summary**

Change the signature:

```ts
function aggregateProviderRows(rows: ProcessedMonitorData[]): ProcessedMonitorData[] {
```

to:

```ts
function aggregateProviderRows(
  rows: ProcessedMonitorData[],
  providerSummaries = new Map<string, NonNullable<ProcessedMonitorData['auditSummary']>>(),
): ProcessedMonitorData[] {
```

Inside the returned object construction, before `return {`, add:

```ts
    const providerSummary = providerSummaries.get(
      buildAuditProviderSummaryKey(primary.providerName, primary.serviceType),
    );
    const auditAvailability = chooseAuditDisplayAvailability(providerSummary);
```

Replace:

```ts
      uptime: aggregateUptime,
```

with:

```ts
      auditSummary: providerSummary ?? primary.auditSummary ?? null,
      uptime: auditAvailability >= 0 ? auditAvailability : aggregateUptime,
      history: primary.history.length > 0 ? primary.history : buildAuditDisplayHistory(providerSummary ?? primary.auditSummary),
```

- [x] **Step 6: Add home summary counters**

Before `return`, add:

```ts
  const auditSummary = auditStatusMeta?.summary;
```

In the hero stats block, replace:

```tsx
              <span>服务商 {providerRows.length}</span>
              <span>同步通道 {rows.length}</span>
```

with:

```tsx
              <span>服务商 {providerRows.length}</span>
              <span>同步通道 {rows.length}</span>
              <span>生产日志样本 {auditSummary?.production_total ?? 0}</span>
              <span>模板样本 {auditSummary?.template_probe_total ?? 0}</span>
              <span>Baseline 对比 {auditSummary?.baseline_compared ?? 0}</span>
```

- [x] **Step 7: Include global audit loading/error in page state**

Replace:

```ts
  const effectiveError = auditChannelsError || monitorError;
  const loading = auditChannelsLoading || monitorLoading;
```

with:

```ts
  const effectiveError = auditChannelsError || monitorError || auditStatusError;
  const loading = auditChannelsLoading || monitorLoading || auditStatusLoading;
```

- [x] **Step 8: Run frontend tests**

Run:

```bash
cd frontend
npm test -- src/utils/auditModelStatusSummary.test.ts src/hooks/useAuditModelStatus.test.ts src/utils/auditServiceBoundary.test.ts
```

Expected:

```text
PASS
```

- [x] **Step 9: Run frontend build**

Run:

```bash
cd frontend
npm run build
```

Expected:

```text
✓ built
```

- [x] **Step 10: Commit Task 3**

Run:

```bash
git add frontend/src/App.tsx
git commit -m "feat(frontend): show audit summaries on home page"
```

---

## Task 4: Runtime Verification And Plan Record

**Files:**
- Modify: `docs/superpowers/plans/2026-06-28-public-page-data-closure.md`

- [x] **Step 1: Run backend tests**

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

- [x] **Step 2: Refresh local embedded frontend**

Run:

```bash
cd frontend
npm run build
cd ..
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -R frontend/dist internal/api/frontend/
go build -o /tmp/relay-pulse-server ./cmd/server
```

Expected: all commands exit 0.

- [x] **Step 3: Restart local server on 18080**

Run:

```bash
pid=$(lsof -tiTCP:18080 -sTCP:LISTEN || true)
if [ -n "$pid" ]; then kill $pid; fi
/opt/homebrew/bin/tmux kill-session -t relay-pulse-18080 2>/dev/null || true
/opt/homebrew/bin/tmux new-session -d -s relay-pulse-18080 'cd /Users/gongxiude/Documents/github/relay-pulse && PORT=18080 /tmp/relay-pulse-server config.yaml'
sleep 1
curl -sS -o /tmp/relay-pulse-health.txt -w '%{http_code}\n' http://127.0.0.1:18080/health
```

Expected:

```text
200
```

- [x] **Step 4: Verify global model-status returns real data**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?window=24h' | jq -c '{count:.data.meta.count, summary:.data.meta.summary}'
```

Expected: `count` is greater than 0 and `summary.production_total` is greater than 0 on the current local SQLite database.

- [x] **Step 5: Verify home page asset contains audit summary labels**

Run:

```bash
asset=$(basename frontend/dist/assets/App-*.js)
curl -sS "http://127.0.0.1:18080/assets/$asset" | rg -o "生产日志样本|模板样本|Baseline 对比" | sort | uniq
```

Expected:

```text
Baseline 对比
模板样本
生产日志样本
```

- [x] **Step 6: Verify provider page still uses real audit service**

Run:

```bash
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=cc&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '.data.meta.count'
curl -sS 'http://127.0.0.1:18080/api/audit/model-status?provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h' | jq '.data.meta.count'
```

Expected:

```text
0
5
```

- [x] **Step 7: Update this plan with observed verification results**

Add a short `## Execution Record` section at the bottom with:

```markdown
## Execution Record

- Frontend tests:
- Frontend build:
- Go tests:
- Local server:
- Global model-status:
- Provider service-boundary:
- Git commits:
```

Fill each line with exact observed output summary.

- [x] **Step 8: Commit Task 4**

Run:

```bash
git add docs/superpowers/plans/2026-06-28-public-page-data-closure.md
git commit -m "docs: record public page data closure verification"
```

- [x] **Step 9: Push main**

Run:

```bash
git status --short --branch
git push origin main
```

Expected:

```text
## main...origin/main [ahead 1]
main -> main
```

---

## Self-Review

Spec coverage:

- 首页继续作为入口页，不新增 `/audit` 核心入口。
- 首页接入 `new-api` 同步渠道和 `/api/audit/model-status` 真实汇总，展示状态、可用率和趋势。
- 服务商详情页沿用已实现的真实 service 查询、模板可用率、超时/无响应、quick-probe baseline 对比。
- 所有执行和验证结果写回本 plan，保留历史痕迹。

Known boundaries:

- 首页趋势使用现有 `/api/status` 历史优先；当旧模板历史缺失时，用 24h 审计汇总生成简化趋势占位，避免空白。
- 长周期 7d/30d 生产日志趋势不在本计划内，属于后续 V1.1 趋势增强。

## Execution Record

- Plan commit: `e0735d7 docs: add public page data closure plan`
- Task 1 commit: `e90791f test(frontend): aggregate audit model status summaries`
- Task 2 commit: `e839448 feat(frontend): fetch global audit model status summary`
- Task 3 commit: `03ffb0b feat(frontend): show audit summaries on home page`
- Frontend tests: `npm test -- src/utils/auditModelStatusSummary.test.ts src/hooks/useAuditModelStatus.test.ts src/utils/auditServiceBoundary.test.ts` passed, 3 files / 7 tests.
- Frontend build: `npm run build` passed, Vite built `App-B4i4MD7l.js` and `ProviderPage-BYgyXhUZ.js`.
- Go tests: `go test ./internal/api ./internal/audit ./internal/storage ./internal/config` passed.
- Local server: `PORT=18080 /tmp/relay-pulse-server config.yaml` running in tmux session `relay-pulse-18080`; `/health` returned `200`.
- Global model-status: `/api/audit/model-status?window=24h` returned `count=133`, `production_total=1775`, `production_success_rate=100`, `quick_probe_done=1`, `quick_probe_failed=5`, `quick_probe_missing=127`, `baseline_compared=4`.
- Home asset verification: `assets/App-B4i4MD7l.js` contains `生产日志样本`、`模板样本`、`Baseline 对比`.
- Provider service-boundary: `alan-官key直连` with `service=cc` returned `0`; with `service=anthropic` returned `5`.
- Render verification: headless Chrome rendered `/` with `服务商列表`、`生产日志样本`、`模板样本`、`Baseline 对比`; rendered provider page with `整体模板可用率`、`正常请求`、`超时 / 无响应`、`模型状态`、`检测历史`.
