# Provider Detail Layout Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 简化服务商详情页，去掉重复的通道状态信息，并把模型选择框移动到“模型状态”标题右侧，替换当前右侧的“未知”展示。

**Architecture:** 只调整 `ProviderPage` 的页面组合，不改 new-api 同步、诊断历史、后端接口和检测逻辑。保留页面顶部的服务商名、通道类型、启停状态、服务视图和同步分组作为唯一的通道概览；删除重复的通道卡片、状态 SummaryCard 和下方筛选卡里的通道类型/通道选择，把模型筛选变成模型表格区域的局部控件。

**Tech Stack:** React, TypeScript, React Router `useSearchParams`, Tailwind CSS, Vite, Go test suite for backend regression.

---

## Current State

当前工作树已有上一轮未提交修改：

```text
M frontend/src/hooks/useAuditDiagnosticLatest.ts
M frontend/src/pages/DetectPage.tsx
M frontend/src/pages/ProviderPage.tsx
```

本计划只继续修改 `frontend/src/pages/ProviderPage.tsx`。执行前必须确认这些未提交改动仍然是上一轮“检测历史入口”改动，不要误删。

当前页面重复点：

- 顶部已经显示 `providerDisplayName`、通道类型 badge、`SnapshotStatusBadge`、服务视图和同步分组。
- “同步通道”区块再次展示通道名、channel id、未知/已启用、模型 23、分组 anthropic。
- SummaryCard 再次展示当前通道、当前状态、模型数量、同步分组。
- “筛选当前详情”卡片再次展示通道类型、通道、模型。
- “模型状态”标题右侧的 `未知` badge 对用户没有实际操作价值，应替换成模型选择框。

目标页面结构：

```text
顶部概览：
  服务商名 + 通道类型 + 启停状态
  服务视图 + 当前同步分组

检测样本统计：
  最近有效样本 / 401失败 / 请求失败 / 其他

probe warning:
  当前通道尚无有效检测样本

模型状态：
  左侧：标题 + 说明 + 模型数量
  右侧：模型选择框
  下方：模型表格
```

---

## File Structure

Modify only:

- `frontend/src/pages/ProviderPage.tsx`
  - Remove redundant `同步通道` section rendering.
  - Remove redundant SummaryCard grid rendering.
  - Remove `筛选当前详情` section rendering.
  - Move model `FilterField` into `模型状态` header right side.
  - Keep `FilterField` helper because it is reused for the model select.
  - Delete `SummaryCard` helper if unused after cleanup.

No backend file changes.

No docs update required beyond this plan.

---

### Task 1: Add Model Selector To Model Status Header

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Locate current model status header**

Find this block in `frontend/src/pages/ProviderPage.tsx`:

```tsx
          <section className="mb-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-primary">模型状态</h2>
                <p className="mt-1 text-sm text-secondary">
                  当前表格按模型展开，优先展示模型是否启用、最近检测状态和可用率趋势。
                </p>
              </div>
              {currentSourceMeta ? (
                <div className="inline-flex items-center gap-2 rounded-full border border-default/70 bg-surface/70 px-3 py-1 text-xs text-secondary">
                  {currentSourceMeta.icon}
                  <span>{currentSnapshot?.channelTypeLabel || currentSourceMeta.label}</span>
                </div>
              ) : null}
            </div>
          </section>
```

- [ ] **Step 2: Replace the right-side unknown badge with a model select**

Replace the block above with:

```tsx
          <section className="mb-3">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h2 className="text-lg font-semibold text-primary">模型状态</h2>
                <p className="mt-1 text-sm text-secondary">
                  当前表格按模型展开，优先展示模型是否启用、最近检测状态和可用率趋势。
                </p>
                <div className="mt-2 text-xs text-muted">
                  模型数量 {modelRows.length}
                  {selectedModel !== 'all' ? ` / 当前筛选 ${selectedModel}` : ''}
                </div>
              </div>
              <div className="w-full sm:w-80">
                <FilterField
                  label="模型"
                  value={selectedModel}
                  onChange={(value) => updateParam(setSearchParams, searchParams, { model: value === 'all' ? undefined : value })}
                  options={[{ value: 'all', label: '全部模型' }, ...modelOptions.map((model) => ({ value: model, label: model }))]}
                />
              </div>
            </div>
          </section>
```

- [ ] **Step 3: Run frontend typecheck/build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

If TypeScript reports `FilterField` is undefined, stop. That means the helper was accidentally removed too early.

---

### Task 2: Remove Redundant Channel And Summary Sections

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Remove the “同步通道” section**

Delete this entire section:

```tsx
          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-primary">同步通道</h2>
                <p className="mt-1 text-sm text-secondary">
                  下列通道全部来自 `new-api` 同步快照。先选定当前通道，再查看该通道下每个模型的状态。
                </p>
              </div>
              <div className="text-xs text-muted">
                当前类型：{currentSnapshot?.channelTypeLabel || currentSourceMeta?.label || '未知'}
              </div>
            </div>

            <div className="grid gap-3 lg:grid-cols-2">
              {sourceFilteredSnapshots.map((snapshot) => {
                const active = snapshot.channel === selectedChannel;
                const modelsCount = splitModels(snapshot.model).length;
                return (
                  <button
                    key={snapshot.channel}
                    type="button"
                    onClick={() => updateParam(setSearchParams, searchParams, { channel: snapshot.channel, model: undefined })}
                    className={`rounded-xl border px-4 py-4 text-left transition ${
                      active
                        ? 'border-accent bg-accent/10 shadow-sm'
                        : 'border-default/70 bg-surface/65 hover:bg-elevated/45'
                    }`}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <div className="font-medium text-primary">{extractAuditChannelName(snapshot.channel)}</div>
                        <div className="mt-1 text-xs text-secondary">{snapshot.channel}</div>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="inline-flex items-center rounded-full border border-default/70 bg-surface/70 px-2.5 py-1 text-xs text-secondary">
                          {snapshot.channelTypeLabel || inferSourceKey(snapshot)}
                        </span>
                        <SnapshotStatusBadge snapshot={snapshot} compact />
                      </div>
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
                      <span>模型 {modelsCount}</span>
                      <span>分组 {snapshot.service || '--'}</span>
                    </div>
                  </button>
                );
              })}
            </div>
          </section>
```

- [ ] **Step 2: Remove the SummaryCard grid**

Delete this entire section:

```tsx
          <section className="mb-4 grid gap-4 lg:grid-cols-4 md:grid-cols-2">
            <SummaryCard
              label="当前通道"
              value={currentSnapshot ? extractAuditChannelName(currentSnapshot.channel) : '--'}
              hint={currentSnapshot?.channel || '未选定通道'}
            />
            <SummaryCard
              label="当前状态"
              value={currentSnapshot ? <SnapshotStatusBadge snapshot={currentSnapshot} compact /> : '--'}
              hint={currentSnapshot?.channelTypeLabel || '以同步快照为准'}
            />
            <SummaryCard
              label="模型数量"
              value={String(modelRows.length)}
              hint={selectedModel === 'all' ? '当前通道全部模型' : selectedModel}
            />
            <SummaryCard
              label="同步分组"
              value={currentServiceGroup}
              hint={`当前服务视图：${currentServiceViewLabel}`}
            />
          </section>
```

- [ ] **Step 3: Remove unused SummaryCard helper**

Delete this helper if TypeScript confirms there are no remaining references:

```tsx
function SummaryCard({ label, value, hint }: { label: string; value: React.ReactNode; hint?: string }) {
  return (
    <div className="rounded-xl border border-default/70 bg-surface/70 px-4 py-4">
      <div className="text-sm text-secondary">{label}</div>
      <div className="mt-2 text-xl font-semibold text-primary break-all">{value}</div>
      {hint ? <div className="mt-1 text-xs leading-relaxed text-muted">{hint}</div> : null}
    </div>
  );
}
```

- [ ] **Step 4: Run grep for removed concepts**

Run:

```bash
rg -n "SummaryCard|同步通道|当前通道全部模型|以同步快照为准" frontend/src/pages/ProviderPage.tsx
```

Expected:

```text
```

The command should return no matches.

---

### Task 3: Remove Redundant Filter Card And Keep Model Selection Only

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Remove the “筛选当前详情” section**

Delete this entire block:

```tsx
          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-4">
            <div className="mb-3 text-sm font-semibold text-primary">筛选当前详情</div>
            <div className="grid gap-4 md:grid-cols-3">
              <FilterField
                label="通道类型"
                value={selectedSource}
                onChange={(value) => updateParam(setSearchParams, searchParams, { source: value === 'all' ? undefined : value, channel: undefined, model: undefined })}
                options={sourceOptions.map((option) => ({
                  value: option,
                  label: option === 'all' ? '全部类型' : SOURCE_META[option as Exclude<SourceKey, 'all'>].label,
                }))}
              />
              <FilterField
                label="通道"
                value={selectedChannel}
                onChange={(value) => updateParam(setSearchParams, searchParams, { channel: value, model: undefined })}
                options={channelOptions}
              />
              <FilterField
                label="模型"
                value={selectedModel}
                onChange={(value) => updateParam(setSearchParams, searchParams, { model: value === 'all' ? undefined : value })}
                options={[{ value: 'all', label: '全部模型' }, ...modelOptions.map((model) => ({ value: model, label: model }))]}
              />
            </div>
          </section>
```

- [ ] **Step 2: Keep URL-driven channel/source behavior intact**

Do not remove these existing computed values unless TypeScript proves they are unused after Tasks 1-3:

```tsx
selectedSource
selectedChannel
sourceOptions
channelOptions
sourceFilteredSnapshots
```

Reason: even without visible channel/type selectors, existing deep links still use `source` and `channel` query params. The page must continue to honor URLs like:

```text
/p/yuexin01-team7000-sunday-2133?service=cc&channel=65%3Ayuexin01-team7000-sunday-2133
```

- [ ] **Step 3: Remove unused variables only after build tells you**

Run:

```bash
npm run build
```

If TypeScript reports unused variables, remove only the exact unused declarations. Do not remove `selectedChannel`, `currentSnapshot`, `modelOptions`, or `selectedModel`.

Expected:

```text
✓ built
```

---

### Task 4: Tighten Top Copy Without Losing Context

**Files:**
- Modify: `frontend/src/pages/ProviderPage.tsx`

- [ ] **Step 1: Replace the top explanatory paragraph**

Find:

```tsx
            <p className="mt-3 text-secondary text-base leading-relaxed">
              当前页只使用 `new-api` 同步的真实渠道快照。先选中当前要看的通道，再按模型查看启停状态、最近检测状态和可用率趋势。
            </p>
```

Replace with:

```tsx
            <p className="mt-3 text-secondary text-base leading-relaxed">
              数据来自 `new-api` 同步快照；模型表格展示启停状态、最近检测状态和可用率趋势。
            </p>
```

- [ ] **Step 2: Keep top status chips**

Do not remove this part:

```tsx
              {currentSourceMeta ? (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-default/70 bg-surface/70 px-3 py-1 text-sm text-secondary">
                  {currentSourceMeta.icon}
                  {currentSnapshot?.channelTypeLabel || currentSourceMeta.label}
                </span>
              ) : null}
              {currentSnapshot ? <SnapshotStatusBadge snapshot={currentSnapshot} /> : null}
```

Reason: the user explicitly said this top area already explains the channel state. It should remain as the single source of channel status.

- [ ] **Step 3: Run visual grep for removed repeated headings**

Run:

```bash
rg -n "同步通道|筛选当前详情|当前通道|当前状态|同步分组" frontend/src/pages/ProviderPage.tsx
```

Expected remaining matches:

```text
当前同步分组
```

If `当前通道`, `当前状态`, or `同步分组` remain inside cards or headings, remove those card remnants.

---

### Task 5: Build, Embed Frontend, Restart Local Server, Verify

**Files:**
- Modify generated embedded frontend under `internal/api/frontend` by copying `frontend/dist`.

- [ ] **Step 1: Run frontend build**

Run:

```bash
npm run build
```

Expected:

```text
✓ built
```

- [ ] **Step 2: Run backend regression tests**

Run:

```bash
go test ./internal/api ./internal/audit ./internal/config
```

Expected:

```text
ok  	monitor/internal/api
ok  	monitor/internal/audit
ok  	monitor/internal/config
```

- [ ] **Step 3: Embed frontend assets**

Run:

```bash
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -R frontend/dist internal/api/frontend/
```

- [ ] **Step 4: Restart local server on 18080**

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

- [ ] **Step 5: Verify HTTP responses**

Run:

```bash
curl -sS -o /tmp/provider_layout.html -w '%{http_code} %{content_type}\n' \
  'http://127.0.0.1:18080/p/yuexin01-team7000-sunday-2133?service=cc&channel=65%3Ayuexin01-team7000-sunday-2133'
```

Expected:

```text
200 text/html; charset=utf-8
```

- [ ] **Step 6: Verify final source shape**

Run:

```bash
rg -n "同步通道|筛选当前详情|SummaryCard|当前通道全部模型|以同步快照为准" frontend/src/pages/ProviderPage.tsx
```

Expected:

```text
```

- [ ] **Step 7: Manual browser acceptance**

Open:

```text
http://127.0.0.1:18080/p/yuexin01-team7000-sunday-2133?service=cc&channel=65%3Ayuexin01-team7000-sunday-2133
```

Expected visual result:

- 顶部保留服务商名、未知/通道类型、已启用、服务视图、当前同步分组。
- 不再出现“同步通道”大卡片。
- 不再出现“当前通道 / 当前状态 / 模型数量 / 同步分组”四个 SummaryCard。
- 不再出现“筛选当前详情”卡片。
- “模型状态”右侧出现模型选择框。
- 切换模型选择框时，URL 的 `model` 参数变化，表格只显示该模型。
- 结果详情列仍然有“检测历史”入口。

---

### Task 6: Commit

**Files:**
- Commit modified frontend source and embedded frontend assets if the repository tracks `internal/api/frontend`.

- [ ] **Step 1: Check status**

Run:

```bash
git status --short
```

Expected changed source files:

```text
M frontend/src/hooks/useAuditDiagnosticLatest.ts
M frontend/src/pages/DetectPage.tsx
M frontend/src/pages/ProviderPage.tsx
```

If embedded frontend assets are tracked, they may also appear. Include them only if this repository normally commits embedded assets.

- [ ] **Step 2: Commit**

Run:

```bash
git add frontend/src/hooks/useAuditDiagnosticLatest.ts frontend/src/pages/DetectPage.tsx frontend/src/pages/ProviderPage.tsx
git commit -m "feat(frontend): simplify provider detail filters"
```

If `git status --short internal/api/frontend` shows tracked embedded asset changes, use:

```bash
git add internal/api/frontend
git commit -m "feat(frontend): simplify provider detail filters"
```

Expected:

```text
[main <hash>] feat(frontend): simplify provider detail filters
```

---

## Self-Review

Spec coverage:

- 去掉重复内容：Tasks 2, 3, 4 删除同步通道、SummaryCard、筛选当前详情重复信息。
- 保留顶部通道状态说明：Task 4 明确保留顶部 channel type 和 enabled badge。
- 保留模型数量 23 的信息：Task 1 将模型数量移到模型状态标题下。
- “未知”的位置换成模型选择框：Task 1 替换模型状态右侧 badge 为 `FilterField label="模型"`。
- 不改后端、不改检测链路：File Structure 限定只改 `ProviderPage` 页面组合。

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No “similar to”.
- Each code edit includes exact code to find or replace.

Type consistency:

- `FilterField` already exists in `ProviderPage`.
- `selectedModel`, `modelOptions`, `updateParam`, `setSearchParams`, and `searchParams` are existing values in `ProviderPage`.
- `modelRows.length` is already available in the render scope.
