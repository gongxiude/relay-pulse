# RelayPulse V1 目标推进 Plan

## 1. 目标与基线

本计划以 [docs/relaypulse-probe-requirements-zh.md](/Users/gongxiude/Documents/github/relay-pulse/docs/relaypulse-probe-requirements-zh.md) 为唯一需求基线。历史 plan 只作为进度证据，不再作为执行权威。

目标是把当前已经存在的 `new-api` 只读审计链路，收敛成一个可稳定运行的 V1 闭环：

1. 从 `new-api` 同步渠道、模型、模型映射和启停状态。
2. 从 `new-api` 日志聚合生产稳定性指标。
3. 用现有模板探测体系补齐日志无法说明的基础状态。
4. 用配置化 `quick-probe-v1` 做真实性、指纹和官方基线 compare。
5. 页面以 `/` 首页、`/p/:provider` 服务商详情、检测方法和 compare 为核心入口。

## 2. 历史计划复核结论

以下复核基于当前代码、主需求文档和历史 plan 文件。

| 历史计划 | 当前结论 | 保留价值 |
|---|---|---|
| `2026-06-25-relaypulse-newapi-audit.md` | 大部分后端审计骨架已经落地，但目标中“质量排名页”语义已被新基线替换 | 保留 new-api 同步、日志、存储、诊断 API 的实现证据 |
| `2026-06-25-audit-data-fill-plan.md` | 记录了真实运行态和失败样本，指出核心阻塞是缺独立 probe 凭证 | 保留数据现状、接口现状和失败原因证据 |
| `2026-06-25-frontend-reclosure-execution-plan-v3.md` | 前端入口收口方向与新基线一致 | 保留首页、详情页、Header、检测方法入口的验收边界 |
| `2026-06-25-header-footer-nav-cleanup.md` | 已被新基线吸收，Header 只保留“首页 / 检测方法” | 保留导航清理证据 |
| `2026-06-25-audit-channel-type-classification.md` | 通道类型四分类方向正确 | 保留“官方直连、混合、逆向、未知”分类实现证据 |

## 3. 当前真实进度

### 3.1 已完成或基本完成

| 模块 | 当前状态 | 证据 |
|---|---|---|
| `new-api` 配置入口 | 已有 `NEWAPI_*` 和 `NEWAPI_PROBE_*` 环境变量读取 | `internal/config/external.go`、`internal/config/lifecycle.go` |
| `new-api` 只读客户端 | 已有只允许 GET 的 client | `internal/newapi/client.go` |
| 渠道同步 | 已有 `/api/channel/` 同步和 target 生成 | `internal/newapi/sync_channels.go`、`internal/audit/targets.go` |
| 日志同步 | 已有 `/api/log/` 同步和 ranking 聚合 | `internal/newapi/sync_logs.go`、`internal/api/audit_handler.go` |
| 审计存储 | SQLite/PostgreSQL 审计表已存在 | `internal/storage/audit_models.go` |
| 诊断 API | 已有 submit、backfill、latest、diagnostics、compare | `internal/api/server.go` |
| 通道类型分类 | 已有官方直连、混合、逆向、未知分类 | `internal/api/audit_channel_type.go` |
| 首页入口 | `/` 已聚合服务商列表，不显示模型 | `frontend/src/App.tsx` |
| 服务商详情 | `/p/:provider` 已按同步渠道和模型展示状态 | `frontend/src/pages/ProviderPage.tsx` |
| Header | 已保留“首页 / 检测方法” | `frontend/src/components/Header.tsx` |

### 3.2 已验证

本次复核已运行：

```bash
go test ./internal/config ./internal/newapi ./internal/audit ./internal/api ./internal/storage -run 'TestNewAPI|TestAudit|TestDiagnostic|TestBuildAudit|TestResolveAuditChannelType'
```

结果：

```text
ok monitor/internal/config
ok monitor/internal/newapi
ok monitor/internal/audit
ok monitor/internal/api
ok monitor/internal/storage
```

### 3.4 当前执行状态（2026-06-26）

| 任务 | 状态 | 证据 |
|---|---|---|
| Task 1: 增加 `audit.diagnostics` 配置块 | 已完成 | 已新增配置结构、默认值、校验和配置测试 |
| Task 2: 打通开发专用 probe 凭证 | 已完成 | 诊断提交和 backfill 已按 `credential_mode` 解析凭证，status 接口返回 runtime warning |
| Task 3: 扩展诊断模板契约 | 已完成 | 已扩展模板 schema，旧模板兼容，新增 `cx-gpt-chat-diagnostic` 样例 |
| Task 4: 抽公共诊断请求执行器 | 已完成 | 已新增 `internal/audit/request_executor.go`，runner 复用模板 URL / Method / Headers / Body 骨架 |
| Task 5: 接入模板探测补洞层 | 已完成 | `/api/audit/template-probes/backfill` 已复用 `InlineProber.ProbeConfig` 写入 `probe_history`；`/api/audit/model-status` 已区分 `production_logs`、`template_probe`、`quick_probe` 三类来源 |
| Task 6: 运行态验证与页面收口 | 已完成 | 后端测试通过、前端 build 通过、18080 SQLite 运行态已验证；使用运行时 `NEWAPI_PROBE_ACCESS_TOKEN` 产出成功 quick-probe 样本 `diag-79fedc55-205b-4d39-96ea-4721bae5e446` |

本轮验证命令：

```bash
go test ./internal/audit -run 'TestDiagnosticRunner|TestBuildDimensions' -v
go test ./internal/config -run 'TestAuditDiagnostics' -v
go test ./internal/audit ./internal/api -run 'TestBuildTemplateProbeConfig|TestProbeRecordFromTemplateProbeResult|TestResolveTemplateProbeName|TestAuditTemplateProbeBackfillRequiresInlineProber|TestAuditDiagnosticBackfill' -v
go test ./internal/config ./internal/newapi ./internal/audit ./internal/api ./internal/storage
go test ./internal/config ./internal/probe -run 'Test.*Template|TestAuditDiagnostics' -v
go test ./internal/config ./internal/newapi ./internal/audit ./internal/api ./internal/storage
cd frontend && npm run build
```

结果：

```text
ok monitor/internal/config
ok monitor/internal/newapi
ok monitor/internal/audit
ok monitor/internal/api
ok monitor/internal/storage
ok monitor/internal/probe
```

本轮运行态证据（2026-06-26）：

```text
http://127.0.0.1:18080/                                      -> 200 text/html
http://127.0.0.1:18080/detect                                -> 200 text/html
http://127.0.0.1:18080/p/yuexin01-team5000-sunday-2133?...   -> 200 text/html
/api/audit/channels                                          -> 10 个 new-api channel snapshot
/api/audit/ranking?window=24h                                -> 149 个审计 target 排名项
/api/audit/model-status                                      -> 可区分 production_logs / template_probe / quick_probe
/api/audit/template-probes/backfill                          -> 已写入 auth_error/http_401 模板补洞记录
/api/audit/compare/diag-e8ffefe5-da63-44ab-8b3a-a7019ec145f4 -> 6 步失败 compare 证据可读
/api/audit/model-status?model=gpt-5.4                       -> failed_auth quick_probe 也返回 compare_url
/api/audit/diagnostics/latest?include_filtered=1             -> filtered failed_auth run 也返回 compare_url
/api/audit/diagnostics/diag-79fedc55-205b-4d39-96ea-4721bae5e446 -> 成功 quick-probe 样本，6 步均完成
/api/audit/compare/diag-79fedc55-205b-4d39-96ea-4721bae5e446 -> 成功样本 compare 证据可读，overall=96.67
```

诊断样本 `diag-e8ffefe5-da63-44ab-8b3a-a7019ec145f4` 记录了缺独立 probe 凭证时的 401 失败证据；诊断样本 `diag-79fedc55-205b-4d39-96ea-4721bae5e446` 记录了配置运行时 probe token 后的成功链路，6 步分别为 `ping`、`identity`、`cutoff`、`identity_free`、`knowledge_recall`、`digit_count`，状态 `done`，`usable=true`。

### 3.3 当前核心阻塞

| 阻塞 | 现状 | 影响 |
|---|---|---|
| 无 | 已通过运行时 probe token 产出成功诊断样本 | 当前 V1 计划内验收项已完成 |

当前不再阻塞：

1. Task 5 页面分层展示已完成，模型状态接口和详情页都能区分生产日志、模板补洞、quick-probe。
2. Task 6 的本地 SQLite 页面验收已完成，`/`、`/p/:provider`、`/detect`、`/detect/compare/:runId` 均返回 200。

## 4. 目标执行顺序

### Task 1: 增加 `audit.diagnostics` 配置块

**目标：** 让 `quick-probe-v1` 的运行参数和凭证模式可配置。

**文件：**

- Modify: `internal/config/app_config.go`
- Modify: `internal/config/external.go` 或新增 `internal/config/audit.go`
- Modify: `internal/config/normalize.go`
- Modify: `internal/config/validate.go`
- Modify: `config.yaml.example`
- Modify: `docs/user/config.md`
- Test: `internal/config/*_test.go`

**配置结构：**

```yaml
audit:
  diagnostics:
    enabled: true
    methodology: "quick-probe-v1"
    request_timeout: "60s"
    step_gap_min: "1m"
    step_gap_max: "4m"
    cross_5m_boundary: true
    baseline_enabled: true
    credential_mode: "probe_fallback"
```

**验收：**

1. 支持 `probe_only`、`probe_fallback`、`newapi_only`。
2. 默认 `credential_mode=probe_fallback`。
3. `probe_only` 缺少 `NEWAPI_PROBE_ACCESS_TOKEN` 时启动或诊断前明确失败。
4. 配置测试覆盖默认值、非法 duration、非法 credential mode。

### Task 2: 打通开发专用 probe 凭证

**目标：** 解决当前同步渠道没有 key 导致诊断链路不通的问题。

Task 2 只解决“是否有权限发起探测请求”，不解决“请求怎么构造”。请求路径、HTTP method、headers、body、协议族和响应解析必须由 Task 3/Task 4 的模板契约与模板执行器决定。不能把 Task 2 实现成继续硬编码调用 `/v1/chat/completions`。

**原则：**

1. 不同步 channel key。
2. 不读取 `new-api` DB。
3. 探测请求使用 `NEWAPI_PROBE_ACCESS_TOKEN` 作为 relay 访问凭证。
4. 模板决定请求路径、method、headers 和 body；probe token 只决定是否有权限发送该模板请求。
5. 页面和接口明确展示当前 probe 凭证模式。

**文件：**

- Modify: `internal/api/audit_handler.go`
- Modify: `internal/api/audit_types.go`
- Modify: `docs/user/config.md`
- Test: `internal/api/audit_handler_test.go`

**验收：**

1. `/api/audit/newapi/sync/status` 返回 `credential_mode`、`probe_ready`、`warning`。
2. `probe_only` 缺凭证时不执行诊断，并返回明确错误。
3. `probe_fallback` 在没有 probe token 时回退同步 token，并保留 warning。
4. 文档明确开发环境推荐配置：

```bash
NEWAPI_PROBE_ACCESS_TOKEN=dev-probe-token
NEWAPI_PROBE_USER_ID=dev-user-id
```

### Task 3: 扩展诊断模板契约

**目标：** 让模板能声明诊断请求族和允许覆写字段，为 `quick-probe-v1` 复用模板请求骨架做准备。

**文件：**

- Modify: `internal/config/template.go`
- Modify: `internal/probe/registry.go`
- Add/Modify: `templates/*.json`
- Test: `internal/config/*template*_test.go`

**模板字段：**

```json
{
  "request_family": "openai_chat",
  "override_paths": {
    "messages": "$.messages",
    "model": "$.model",
    "stream": "$.stream"
  },
  "response_parser": "openai_chat_sse"
}
```

**验收：**

1. 老模板不带诊断字段仍可作为普通 monitor 模板使用。
2. 诊断模板必须声明 `request_family` 和 `override_paths`。
3. 非法覆写路径在加载或诊断前失败。

### Task 4: 抽公共诊断请求执行器

**目标：** 移除 `diagnostic_runner.go` 中硬编码的 `/v1/chat/completions` 请求构造。

**文件：**

- Add: `internal/audit/request_executor.go`
- Modify: `internal/audit/diagnostic_runner.go`
- Test: `internal/audit/diagnostic_runner_test.go`

**要求：**

1. 从模板加载 URL / Method / Headers / Body 骨架。
2. 运行时只覆写模板契约允许的字段。
3. 保留 SSE 读取、TTFB/TTFT、usage、headers、错误详情采集。
4. 支持至少 `openai_chat` 请求族。

**验收：**

1. runner 不再直接拼 `baseURL + "/v1/chat/completions"`。
2. `quick-probe-v1` 仍能生成 6 步 run/step。
3. compare schema 不发生破坏性变化。

### Task 5: 接入模板探测补洞层

**目标：** 日志样本不足或诊断不可用时，能复用现有模板探测体系补基础状态。

**文件：**

- Add: `internal/audit/template_probe_backfill.go`
- Modify: `internal/api/audit_handler.go`
- Modify: `internal/storage/audit_models.go`
- Test: `internal/audit/*probe*_test.go`

**验收：**

1. 能按 `服务商 + 渠道 + 模型` 生成基础探测结果。
2. 能记录 `auth_error`、`timeout`、`network_error`、`content_mismatch`。
3. 页面最近状态能区分：生产日志状态、模板补洞状态、quick-probe 状态。

### Task 6: 运行态验证与页面收口

**目标：** 以 SQLite 和本地运行态验证 V1 闭环。

**命令：**

```bash
go test ./internal/config ./internal/newapi ./internal/audit ./internal/api ./internal/storage
cd frontend && npm run build
rm -rf internal/api/frontend
mkdir -p internal/api/frontend
cp -r frontend/dist internal/api/frontend/
PORT=18080 MONITOR_STORAGE_TYPE=sqlite MONITOR_SQLITE_PATH=./relaypulse-v1.db go run ./cmd/server ./config.yaml
```

**页面验收：**

1. `/` 首页服务商列表来自真实同步数据。
2. `/p/:provider` 展示同步渠道和每个模型状态。
3. 检测方法入口可打开，不 404。
4. `/api/audit/newapi/sync/status` probe runtime 状态清晰。
5. 有开发 probe token 时能产出至少 1 条成功诊断样本。
6. compare 页面能展示 6 步证据。

## 5. 当前不做

以下内容不进入当前目标 plan：

1. 不读 `new-api` 数据库。
2. 不同步 channel 上游 key。
3. 不自动写回 `new-api`。
4. 不做自动切主力、备选、兜底。
5. 不接入 `new-api perf_metrics`。
6. 不把 V1.1 的完整 0-100 维度评分作为当前阻塞项。
7. 不恢复 `/audit`、`/audit/ranking` 为核心入口。

## 6. 成功标准

本计划完成时必须满足：

1. `audit.diagnostics` 配置块存在并有测试。
2. 开发专用 probe 凭证可配置、可观测、可失败说明。
3. `quick-probe-v1` 通过模板请求骨架发请求，不再完全硬编码。
4. 日志空窗或样本不足时能通过模板探测补基础状态。
5. 首页、服务商详情、检测方法、compare 四个入口都能展示真实链路数据。
6. 全链路仍保持只读：不读 DB、不写回 `new-api`。

## 7. 建议立即执行的第一步

先执行 Task 1 和 Task 2。

原因：

1. 当前所有成功诊断样本的主要阻塞是 probe 凭证。
2. 没有 `credential_mode`，后续无法明确判断是配置问题、权限问题还是模型/渠道问题。
3. Task 1/2 不需要重构 runner，风险最小，但能立刻让链路状态可解释。
