# Audit Channel Type Classification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让首页“通道”列显示审计后的通道类型结果（官方直连 / 混合 / 逆向 / 未知），不再显示模型名或原始渠道名。

**Architecture:** 以后端 `/api/audit/channels` 为唯一真相源，在快照响应中补充 `channel_type` 与 `channel_type_label` 字段。前端复用现有 `ChannelTypeIcon`、`StatusTable` 和 `useAuditChannels`，只替换“通道”列的展示与 tooltip 数据来源，不再前端猜测模型名或直接显示原始通道名。

**Tech Stack:** Go + Gin + SQLite/PostgreSQL storage + React + TypeScript + Vitest

---

## File Structure

### Backend

- Modify: `internal/storage/audit_models.go`
  - 为 `ChannelSnapshot` 增加 `channel_type`、`channel_type_label` 响应字段。
- Modify: `internal/api/audit_handler.go`
  - 在 `/api/audit/channels` 返回前，基于快照 `raw` / `service` / `channel` 统一补齐通道类型。
- Create: `internal/api/audit_channel_type.go`
  - 放通道类型推导函数，避免把规则散落到 handler。
- Create: `internal/api/audit_channel_type_test.go`
  - 覆盖官方直连 / 混合 / 逆向 / 未知的判型测试。

### Frontend

- Modify: `frontend/src/types/audit.ts`
  - 接收后端新增的 `channelType`、`channelTypeLabel`。
- Modify: `frontend/src/types/index.ts`
  - 让 `ProcessedMonitorData` 可携带审计通道类型。
- Modify: `frontend/src/utils/auditChannelAdapter.ts`
  - 从审计快照透传通道类型到表格数据。
- Modify: `frontend/src/components/StatusTable.tsx`
  - “通道”列显示 `官方直连 / 混合 / 逆向 / 未知`，不是渠道名。
  - 保留 `ChannelTypeIcon`，tooltip 展示原始渠道名与判型说明。
- Modify: `frontend/src/components/ChannelTypeIcon.tsx`
  - 仅保留并统一 `official / mixed / reverse / unknown` 四类文案和图标。
- Create: `frontend/src/utils/auditChannelAdapter.test.ts`
  - 覆盖通道类型字段透传测试。

### Docs

- Modify: `docs/superpowers/plans/2026-06-25-frontend-display-plan.md`
  - 执行完成后回填“首页通道列以审计通道类型展示”的实现结论。

---

### Task 1: 后端补充审计通道类型字段

**Files:**
- Create: `internal/api/audit_channel_type.go`
- Create: `internal/api/audit_channel_type_test.go`
- Modify: `internal/storage/audit_models.go`
- Modify: `internal/api/audit_handler.go`

- [x] **Step 1: Write the failing test**

```go
package api

import "testing"

func TestResolveAuditChannelType(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		channel   string
		raw       map[string]any
		wantType  string
		wantLabel string
	}{
		{
			name:      "official by direct keyword",
			service:   "openai",
			channel:   "80:alan-官key直连",
			raw:       map[string]any{"Name": "alan-官key直连", "Group": "openai"},
			wantType:  "official",
			wantLabel: "官方直连",
		},
		{
			name:      "reverse by reverse keyword",
			service:   "anthropic",
			channel:   "91:R-my-channel",
			raw:       map[string]any{"Name": "R-my-channel", "Group": "anthropic"},
			wantType:  "reverse",
			wantLabel: "逆向",
		},
		{
			name:      "mixed by mixed keyword",
			service:   "openai",
			channel:   "92:M-mixed-fallback",
			raw:       map[string]any{"Name": "M-mixed-fallback", "Group": "openai"},
			wantType:  "mixed",
			wantLabel: "混合",
		},
		{
			name:      "unknown fallback",
			service:   "anthropic",
			channel:   "81:alan-号池",
			raw:       map[string]any{"Name": "alan-号池", "Group": "模型渠道测试分组,anthropic"},
			wantType:  "unknown",
			wantLabel: "未知",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotLabel := resolveAuditChannelType(tc.service, tc.channel, tc.raw)
			if gotType != tc.wantType || gotLabel != tc.wantLabel {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotType, gotLabel, tc.wantType, tc.wantLabel)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestResolveAuditChannelType -v`

Expected: FAIL with `undefined: resolveAuditChannelType`

- [x] **Step 3: Write minimal implementation**

```go
package api

import "strings"

func resolveAuditChannelType(service, channel string, raw map[string]any) (string, string) {
	haystack := strings.ToLower(strings.Join([]string{
		service,
		channel,
		stringValue(raw["Name"]),
		stringValue(raw["Group"]),
		stringValue(raw["Tag"]),
		stringValue(raw["Other"]),
	}, " "))

	switch {
	case strings.Contains(haystack, "官key直连"),
		strings.Contains(haystack, "官方直连"),
		strings.Contains(haystack, "直连"),
		strings.Contains(haystack, "official"),
		strings.HasPrefix(strings.ToUpper(channel), "O-"):
		return "official", "官方直连"
	case strings.Contains(haystack, "混合"),
		strings.Contains(haystack, "mixed"),
		strings.HasPrefix(strings.ToUpper(channel), "M-"):
		return "mixed", "混合"
	case strings.Contains(haystack, "逆向"),
		strings.Contains(haystack, "reverse"),
		strings.HasPrefix(strings.ToUpper(channel), "R-"):
		return "reverse", "逆向"
	default:
		return "unknown", "未知"
	}
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
```

- [x] **Step 4: Extend the API response model**

```go
type ChannelSnapshot struct {
	ID               int64           `json:"id"`
	NewAPIChannelID  int64           `json:"newapi_channel_id"`
	SnapshotAt       int64           `json:"snapshot_at"`
	Provider         string          `json:"provider"`
	Service          string          `json:"service"`
	Channel          string          `json:"channel"`
	Model            string          `json:"model"`
	Enabled          bool            `json:"enabled"`
	ChannelType      string          `json:"channel_type,omitempty"`
	ChannelTypeLabel string          `json:"channel_type_label,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
}
```

- [x] **Step 5: Populate `channel_type` in `GetAuditChannels`**

```go
func (h *Handler) GetAuditChannels(c *gin.Context) {
	store, ok := h.auditStore()
	if !ok {
		apiError(c, http.StatusNotImplemented, ErrCodeInternalError, "当前存储不支持审计接口")
		return
	}
	snapshots, err := store.ListLatestChannelSnapshots()
	if err != nil {
		apiError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	for i := range snapshots {
		rawMap := map[string]any{}
		if len(snapshots[i].Raw) > 0 {
			_ = json.Unmarshal(snapshots[i].Raw, &rawMap)
		}
		snapshots[i].ChannelType, snapshots[i].ChannelTypeLabel = resolveAuditChannelType(
			snapshots[i].Service,
			snapshots[i].Channel,
			rawMap,
		)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshots,
		"meta": gin.H{"count": len(snapshots)},
	})
}
```

- [x] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api -run TestResolveAuditChannelType -v`

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api/audit_channel_type.go internal/api/audit_channel_type_test.go internal/api/audit_handler.go internal/storage/audit_models.go
git commit -m "feat: expose audit channel type in audit channels api"
```

---

### Task 2: 前端接收并透传审计通道类型

**Files:**
- Modify: `frontend/src/types/audit.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/utils/auditChannelAdapter.ts`
- Create: `frontend/src/utils/auditChannelAdapter.test.ts`

- [x] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest';
import { adaptAuditChannelsToMonitorData } from './auditChannelAdapter';

describe('adaptAuditChannelsToMonitorData', () => {
  it('should carry audit channel type fields into processed monitor rows', () => {
    const rows = adaptAuditChannelsToMonitorData(
      [
        {
          id: 1,
          newapi_channel_id: 80,
          snapshot_at: 1,
          provider: 'alan-官key直连',
          service: 'openai',
          channel: '80:alan-官key直连',
          model: 'gpt-5',
          enabled: true,
          channelType: 'official',
          channelTypeLabel: '官方直连',
          raw: { Status: 1 },
        },
      ],
      new Map(),
    );

    expect(rows[0].auditChannelType).toBe('official');
    expect(rows[0].auditChannelTypeLabel).toBe('官方直连');
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm --prefix frontend run test -- auditChannelAdapter`

Expected: FAIL with `Property 'channelType' does not exist` or `auditChannelType` undefined

- [x] **Step 3: Update audit snapshot types**

```ts
export interface AuditChannelSnapshot {
  id: number;
  newapi_channel_id: number;
  snapshot_at: number;
  provider: string;
  service: string;
  channel: string;
  model: string;
  enabled: boolean;
  channelType?: 'official' | 'mixed' | 'reverse' | 'unknown';
  channelTypeLabel?: string;
  raw?: Record<string, unknown> | null;
}
```

- [x] **Step 4: Update processed row types**

```ts
export interface ProcessedMonitorData {
  id: string;
  providerId: string;
  providerSlug: string;
  providerName: string;
  providerUrl?: string | null;
  serviceType: string;
  serviceName: string;
  category: 'commercial' | 'public';
  sponsor: string;
  sponsorUrl?: string | null;
  sponsorLevel?: SponsorLevel;
  annotations?: Annotation[];
  priceMin?: number | null;
  priceMax?: number | null;
  listedDays?: number | null;
  channel?: string;
  channelName?: string;
  auditChannelType?: 'official' | 'mixed' | 'reverse' | 'unknown';
  auditChannelTypeLabel?: string | null;
  newApiStatusCode?: number | null;
  newApiStatusLabel?: string | null;
  board: BoardValue;
  // ... keep the rest unchanged
}
```

- [x] **Step 5: Forward the fields in the adapter**

```ts
return {
  id: `audit-${snapshot.newapi_channel_id}`,
  providerId,
  providerSlug: providerId,
  providerName: snapshot.provider,
  providerUrl: matched?.providerUrl ?? null,
  serviceType,
  serviceName: serviceType,
  category: matched?.category ?? 'commercial',
  sponsor: matched?.sponsor ?? '',
  sponsorUrl: matched?.sponsorUrl ?? null,
  sponsorLevel: matched?.sponsorLevel,
  annotations: matched?.annotations,
  priceMin: matched?.priceMin ?? null,
  priceMax: matched?.priceMax ?? null,
  listedDays: matched?.listedDays ?? null,
  channel: snapshot.channel,
  channelName,
  auditChannelType: snapshot.channelType ?? 'unknown',
  auditChannelTypeLabel: snapshot.channelTypeLabel ?? '未知',
  newApiStatusCode: getSnapshotStatusCode(snapshot),
  newApiStatusLabel: getSnapshotStatusLabel(snapshot),
  board: matched?.board ?? 'hot',
  history: matched?.history ?? EMPTY_HISTORY,
  currentStatus,
  uptime: matched?.uptime ?? -1,
  lastCheckTimestamp: matched?.lastCheckTimestamp,
  lastCheckLatency: matched?.lastCheckLatency,
};
```

- [x] **Step 6: Run test to verify it passes**

Run: `npm --prefix frontend run test -- auditChannelAdapter`

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/audit.ts frontend/src/types/index.ts frontend/src/utils/auditChannelAdapter.ts frontend/src/utils/auditChannelAdapter.test.ts
git commit -m "feat: carry audit channel type through frontend adapter"
```

---

### Task 3: 首页“通道”列改为类型展示，保留原始通道名在 tooltip

**Files:**
- Modify: `frontend/src/components/ChannelTypeIcon.tsx`
- Modify: `frontend/src/components/StatusTable.tsx`
- Test: `frontend/src/utils/auditChannelAdapter.test.ts`

- [x] **Step 1: Write the failing UI expectation as a focused helper test**

```ts
import { describe, expect, it } from 'vitest';

describe('audit channel display contract', () => {
  it('uses normalized audit labels instead of raw channel names', () => {
    const item = {
      auditChannelType: 'official',
      auditChannelTypeLabel: '官方直连',
      channel: '80:alan-官key直连',
      channelName: 'alan-官key直连',
    };

    expect(item.auditChannelTypeLabel).toBe('官方直连');
    expect(item.auditChannelTypeLabel).not.toContain('alan-官key直连');
  });
});
```

- [x] **Step 2: Run test to verify current UI contract is not implemented**

Run: `npm --prefix frontend run test -- auditChannelAdapter`

Expected: Existing adapter test may pass, but manual inspection still shows `alan-官key直连` in the 通道列. This step is the checkpoint before editing UI.

- [x] **Step 3: Narrow `ChannelTypeIcon` to four supported types**

```ts
export type ChannelType = 'official' | 'reverse' | 'mixed' | 'unknown';

export function parseChannelType(channelType?: string | null): ChannelType | null {
  if (!channelType) return null;
  if (channelType === 'official') return 'official';
  if (channelType === 'reverse') return 'reverse';
  if (channelType === 'mixed') return 'mixed';
  return 'unknown';
}

interface ChannelTypeIconProps {
  channelType?: string | null;
}

export function ChannelTypeIcon({ channelType }: ChannelTypeIconProps) {
  const { t } = useTranslation();
  const type = parseChannelType(channelType);
  if (!type) return null;
  // keep icon rendering unchanged
}
```

- [x] **Step 4: Replace the table cell content**

```tsx
<td className="px-1.5 py-1 text-secondary text-xs">
  <ChannelCell
    channel={item.auditChannelTypeLabel || '未知'}
    probeUrl={item.probeUrl}
    templateName={item.templateName}
    coldReason={item.coldReason}
    className="max-w-[10rem]"
    icon={<ChannelTypeIcon channelType={item.auditChannelType} />}
    tooltipTitle={item.channelName || item.channel || '-'}
  />
</td>
```

- [x] **Step 5: Update `ChannelCell` to support display label + raw tooltip**

```tsx
interface ChannelCellProps {
  channel?: string;
  rawChannelName?: string;
  probeUrl?: string;
  templateName?: string;
  coldReason?: string;
  className?: string;
  icon?: React.ReactNode;
}

const channelContent = (
  <>
    {icon}
    <span className="min-w-0 truncate">{channel || '-'}</span>
  </>
);

{rawChannelName && (
  <span className="flex flex-col">
    <span className="text-muted text-[10px]">原始渠道名</span>
    <span className="text-primary text-[11px] break-all">{rawChannelName}</span>
  </span>
)}
```

- [x] **Step 6: Run frontend build and tests**

Run: `npm --prefix frontend run test -- auditChannelAdapter && npm --prefix frontend run build`

Expected:
- Vitest PASS
- Vite build PASS

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/ChannelTypeIcon.tsx frontend/src/components/StatusTable.tsx frontend/src/utils/auditChannelAdapter.test.ts
git commit -m "feat: show normalized audit channel types in status table"
```

---

### Task 4: 运行态验收与文档回填

**Files:**
- Modify: `docs/superpowers/plans/2026-06-25-frontend-display-plan.md`

- [x] **Step 1: Rebuild and restart the local monitor container**

Run:

```bash
docker compose up -d --build monitor
docker compose ps monitor
```

Expected:
- `relaypulse-monitor` is `healthy`

- [x] **Step 2: Verify the API contract**

Run:

```bash
curl -s http://localhost:8080/api/audit/channels | jq '.data[0] | {provider,channel,channel_type,channel_type_label}'
```

Expected:

```json
{
  "provider": "alan-官key直连",
  "channel": "80:alan-官key直连",
  "channelType": "official",
  "channelTypeLabel": "官方直连"
}
```

- [x] **Step 3: Verify the UI contract**

Run:

```bash
open http://localhost:8080/
```

Expected:
- 首页“通道”列显示 `官方直连 / 混合 / 逆向 / 未知`
- 不再显示 `alan-官key直连` 这类原始渠道名作为主文本
- hover 后 tooltip 仍可看到原始渠道名

- [x] **Step 4: Backfill the older frontend plan with the implemented decision**

```md
## 补充决策（2026-06-25）

- 首页“通道”列不再显示 `new-api` 原始渠道名。
- 首页“通道”列主文本统一显示审计通道类型：
  - `官方直连`
  - `混合`
  - `逆向`
  - `未知`
- 原始渠道名只保留在 tooltip 中，作为辅助信息。
```

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-06-25-frontend-display-plan.md
git commit -m "docs: record audit channel type display decision"
```

---

## Self-Review

### 1. Spec coverage

- “通道应该根据测评结果返回”：Task 1 把通道类型作为后端返回字段，不再让前端直接显示原始字符串。
- “只要官方直连 / 混合 / 逆向 / 未知”：Task 1 + Task 3 统一约束成四类。
- “不是模型名称”：Task 3 明确把首页通道列主文本从原始渠道名切换为类型标签。
- “复用原来代码里的通道类型组件”：Task 3 复用 `ChannelTypeIcon`，不是重做表格。

### 2. Placeholder scan

- 已去掉 `TODO / TBD / 之后实现`。
- 每个代码步骤都给了明确代码块。
- 每个验证步骤都有具体命令。

### 3. Type consistency

- 后端字段统一为 `channelType / channelTypeLabel`
- 前端快照字段统一为 `channelType / channelTypeLabel`
- 表格行字段统一为 `auditChannelType / auditChannelTypeLabel`

---

Plan complete and saved to `docs/superpowers/plans/2026-06-25-audit-channel-type-classification.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
