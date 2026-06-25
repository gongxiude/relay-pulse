# RelayPulse new-api 只读审计系统 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 RelayPulse 改造成 `new-api` 的只读外部质量审计系统，自动同步渠道与日志，生成生产稳定性指标，并提供 quick-probe 诊断、compare 证据页和质量排名页。

**Architecture:** 保留现有 Go + Gin + React/Vite 主架构，不改 `new-api` 核心代码。新增一条独立审计链路：`new-api HTTP API` → `同步器` → `审计存储` → `生产指标聚合` → `quick-probe 诊断` → `评分/证据 API` → `前端审计工作台`。

**Tech Stack:** Go 1.x, Gin, SQLite/PostgreSQL 双存储, 现有 React 19 + Vite + TypeScript, 现有 `fetch`/hooks 模式, 现有 Vitest。

**Execution Audit:** 执行过程必须符合 `fullstack-dev` 的边界约束：配置集中、启动即校验、typed error、全局错误处理、结构化日志、健康检查、显式 CORS、优雅退出、前后端 API 边界清晰。第一版只做只读审计，不写回 `new-api`，不引入额外认证流和重实时通道。

---

### Task 1: 冻结 new-api 只读契约并补齐配置入口

**Files:**
- Modify: `internal/config/external.go`
- Modify: `internal/config/app_config.go`
- Modify: `internal/config/loader.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/dotenv.go`
- Modify: `config.yaml.example`
- Modify: `config.production.yaml.example`
- Modify: `.env.example`
- Modify: `docs/user/config.md`
- Test: `internal/config/*_test.go`

- [ ] **Step 1: 写失败测试**

  增加配置测试，覆盖环境变量 `NEWAPI_BASE_URL`、`NEWAPI_ACCESS_TOKEN`、`NEWAPI_USER_ID` 的解析、默认值和必填校验；同步间隔如果仍保留在配置文件里，也必须有默认值和启动期校验。

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/config -run 'Test.*NewAPI|Test.*Config' -v`
  Expected: 新配置尚未定义时，测试失败于缺少字段或校验逻辑。

- [ ] **Step 3: 最小实现**

  在 `internal/config/external.go` 新增 `NewAPIConfig`，并挂到 `AppConfig`。要求只读，禁止写回相关字段；`NEWAPI_BASE_URL`、`NEWAPI_ACCESS_TOKEN`、`NEWAPI_USER_ID` 只从环境变量读取，不能写进运行时 yaml。

  预期结构：
  ```go
  type NewAPIConfig struct {
      BaseURL string `yaml:"-" json:"base_url"`
      AccessToken string `yaml:"-" json:"-"`
      UserID string `yaml:"-" json:"-"`
      Timeout string `yaml:"timeout" json:"timeout"`
      ChannelSyncInterval string `yaml:"channel_sync_interval" json:"channel_sync_interval"`
      LogSyncInterval string `yaml:"log_sync_interval" json:"log_sync_interval"`
      PageSize int `yaml:"page_size" json:"page_size"`
  }
  ```

  环境变量映射：
  ```go
  // NEWAPI_BASE_URL -> NewAPIConfig.BaseURL
  // NEWAPI_ACCESS_TOKEN -> NewAPIConfig.AccessToken
  // NEWAPI_USER_ID -> NewAPIConfig.UserID
  ```

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/config ./... -run 'Test.*NewAPI|Test.*Config' -v`

- [ ] **Step 5: 提交**

  `git add internal/config config.yaml.example config.production.yaml.example docs/user/config.md`
  `git commit -m "feat: add new-api read-only config"`

---

### Task 2: 建立 new-api 客户端与响应解析层

**Files:**
- Create: `internal/newapi/client.go`
- Create: `internal/newapi/types.go`
- Create: `internal/newapi/client_test.go`
- Create: `internal/newapi/testdata/channel_list.json`
- Create: `internal/newapi/testdata/log_list.json`
- Create: `internal/newapi/testdata/log_stat.json`

- [ ] **Step 1: 写失败测试**

  覆盖以下行为：
  1. 能解析 `/api/channel/` 返回的渠道、模型、模型映射、启停状态。
  2. 能解析 `/api/log/` 返回的消费/错误日志。
  3. 能解析 `/api/log/stat` 返回的统计信息。
  4. 遇到非 200、非法 JSON、超时，返回带上下文的错误。

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/newapi -v`
  Expected: 失败于 client 未实现。

- [ ] **Step 3: 最小实现**

  实现只读 HTTP client，固定只允许 GET。提供三个方法：
  - `ListChannels(ctx)`
  - `ListLogs(ctx, cursor)`
  - `GetLogStat(ctx)`

  解析时必须保留原始 JSON，避免 new-api 字段后续变化把审计链路打断。

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/newapi -v`

- [ ] **Step 5: 提交**

  `git add internal/newapi`
  `git commit -m "feat: add new-api read client"`

---

### Task 3: 设计审计存储表与跨库迁移

**Files:**
- Modify: `internal/storage/storage.go`
- Modify: `internal/storage/sqlite.go`
- Modify: `internal/storage/postgres.go`
- Create: `internal/storage/audit_models.go`
- Create: `internal/storage/audit_models_test.go`

- [ ] **Step 1: 写失败测试**

  覆盖审计实体的基本读写、分页和幂等行为。至少包括：
  - 渠道快照写入后可按 `newapi_channel_id + snapshot_at` 查询。
  - 日志游标可原子更新。
  - 诊断任务与步骤可以按 `run_id` 回读。
  - SQLite 和 PostgreSQL 的初始化都能通过。

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/storage -run 'Test.*Audit' -v`

- [ ] **Step 3: 最小实现**

  新增表建议如下：
  - `newapi_channel_snapshots`
  - `newapi_log_sync_cursors`
  - `audit_targets`
  - `production_metric_buckets`
  - `diagnostic_runs`
  - `diagnostic_steps`
  - `diagnostic_scores`

  原则：
  - 原始日志保留原始字段和 `other` JSON。
  - 聚合结果单独存 bucket，避免前端每次扫全量日志。
  - 诊断结果按 run/step 拆开，方便 compare 页回放。

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/storage ./... -run 'Test.*Audit' -v`

- [ ] **Step 5: 提交**

  `git add internal/storage`
  `git commit -m "feat: add audit storage schema"`

---

### Task 4: 实现 new-api 渠道同步与监控对象生成

**Files:**
- Create: `internal/newapi/sync_channels.go`
- Create: `internal/audit/targets.go`
- Create: `internal/audit/targets_test.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: 写失败测试**

  覆盖以下规则：
  - 通过渠道列表自动生成 `provider + channel + model` 监控对象。
  - `disabled=true` 的渠道只保留状态，不进入主动探测。
  - `model_mapping` 生效时，request_model 与展示 model 分离。
  - 同一个渠道下多个模型都能展开，不丢失排序。

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/audit -run 'Test.*Targets' -v`

- [ ] **Step 3: 最小实现**

  增加同步器，把 `/api/channel/` 读到的渠道快照落库，再转换成审计对象：
  - `provider`
  - `channel_id`
  - `channel_name`
  - `model`
  - `request_model`
  - `group`
  - `weight`
  - `priority`
  - `enabled`

  这一层只做读取和生成，不写回 new-api。

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/newapi ./internal/audit -v`

- [ ] **Step 5: 提交**

  `git add internal/newapi internal/audit cmd/server/main.go internal/api/server.go`
  `git commit -m "feat: sync new-api channels into audit targets"`

---

### Task 5: 实现日志增量同步与生产稳定性聚合

**Files:**
- Create: `internal/newapi/sync_logs.go`
- Create: `internal/audit/production_metrics.go`
- Create: `internal/audit/production_metrics_test.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 写失败测试**

  覆盖以下口径：
  - `LogTypeConsume = 2` 计入成功。
  - `LogTypeError = 5` 计入失败。
  - `logs.other` 里 `frt`、`cache_*`、`is_model_mapped`、`upstream_model_name` 可解析。
  - 统计 bucket 支持 24h、7d、30d。
  - `Tokens/s = sum(completion_tokens) / sum(use_time)`。

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/audit -run 'Test.*Production' -v`

- [ ] **Step 3: 最小实现**

  实现增量拉取：
  - 按 `created_at` 或日志 ID 维护 cursor。
  - 失败时保留最近一次成功游标。
  - 解析失败的 `other` 字段保留 raw 值。

  实现聚合结果：
  - 成功率
  - 错误率
  - 超时率
  - 平均延迟
  - P95/P99
  - Tokens/s
  - cache usage
  - FRT 参考值

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/newapi ./internal/audit -v`

- [ ] **Step 5: 提交**

  `git add internal/newapi internal/audit cmd/server/main.go`
  `git commit -m "feat: aggregate new-api production metrics"`

---

### Task 6: 实现 quick-probe-v1 诊断引擎和 compare 数据模型

**Files:**
- Create: `internal/audit/diagnostic_runner.go`
- Create: `internal/audit/diagnostic_steps.go`
- Create: `internal/audit/fingerprint.go`
- Create: `internal/audit/diagnostic_runner_test.go`
- Create: `internal/audit/testdata/quick_probe/*.json`

- [ ] **Step 1: 写失败测试**

  覆盖六步探针：
  1. ping
  2. identity
  3. cutoff
  4. identity_free
  5. knowledge_recall
  6. digit_count

  同时覆盖 compare 所需字段：
  - `step_index`
  - `prompt`
  - `resolved_prompt`
  - `response_preview`
  - `result_summary`
  - `execution_meta`
  - `channel_fingerprint`
  - `provider_fingerprint`
  - `error_message`

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/audit -run 'Test.*Diagnostic' -v`

- [ ] **Step 3: 最小实现**

  诊断 runner 只负责三件事：
  - 同会话复用前 5 步
  - 第 6 步新会话隔离
  - 保存完整证据和原始响应

  指纹评分先做规则版，输出：
  - 真实性分
  - 协议指纹分
  - SSE 行为分
  - 偷换 / 降智 / fallback 标签

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/audit -v`

- [ ] **Step 5: 提交**

  `git add internal/audit`
  `git commit -m "feat: add quick-probe diagnostic engine"`

---

### Task 7: 新增审计 API 与任务编排入口

**Files:**
- Create: `internal/api/audit_handler.go`
- Create: `internal/api/audit_types.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/query.go`

- [ ] **Step 1: 写失败测试**

  覆盖 API 契约：
  - `GET /api/audit/newapi/sync/status`
  - `POST /api/audit/newapi/sync/channels`
  - `POST /api/audit/newapi/sync/logs`
  - `GET /api/audit/targets`
  - `GET /api/audit/ranking`
  - `GET /api/audit/diagnostics/:run_id`
  - `GET /api/audit/compare/:run_id`

- [ ] **Step 2: 跑测试确认失败**

  Run: `go test ./internal/api -run 'Test.*Audit' -v`

- [ ] **Step 3: 最小实现**

  新增只读审计接口，默认返回：
  - 同步状态
  - 目标列表
  - 生产排名
  - 诊断任务详情
  - compare 页面数据

  第一版不提供任何写回 new-api 的接口。

- [ ] **Step 4: 跑测试确认通过**

  Run: `go test ./internal/api ./... -run 'Test.*Audit' -v`

- [ ] **Step 5: 提交**

  `git add internal/api`
  `git commit -m "feat: expose audit APIs"`

---

### Task 8: 搭建前端审计工作台

**Files:**
- Modify: `frontend/src/router.tsx`
- Modify: `frontend/src/App.tsx`
- Create: `frontend/src/pages/AuditHomePage.tsx`
- Create: `frontend/src/pages/AuditRankingPage.tsx`
- Create: `frontend/src/pages/AuditComparePage.tsx`
- Create: `frontend/src/pages/AuditSubmitPage.tsx`
- Create: `frontend/src/hooks/useAuditData.ts`
- Create: `frontend/src/types/audit.ts`
- Modify: `frontend/src/components/Footer.tsx`
- Modify: `frontend/src/components/Header.tsx`

- [ ] **Step 1: 写失败测试**

  覆盖前端数据转换和页面基本渲染：
  - 排名页能消费 ranking export 风格 JSON。
  - compare 页能渲染六步证据和标签。
  - 提交页能发起诊断任务并显示 run_id。

- [ ] **Step 2: 跑测试确认失败**

  Run: `cd frontend && npm test`

- [ ] **Step 3: 最小实现**

  前端页面按 RPDiag 的信息组织方式落地：
  - 首页：同步状态 + 最近任务 + 目标数量
  - 排名页：生产稳定性 + 指纹分 + 标签
  - compare 页：每步 prompt、usage、cache、SSE、header、错误
  - 提交页：选择服务商/渠道/模型/官方基线

  路由保持现有 React Router，不切换 Astro。

- [ ] **Step 4: 跑测试确认通过**

  Run: `cd frontend && npm test && npm run build`

- [ ] **Step 5: 提交**

  `git add frontend/src`
  `git commit -m "feat: add audit workbench frontend"`

---

### Task 9: 补齐文档、样例配置与验收步骤

**Files:**
- Modify: `README.md`
- Modify: `QUICKSTART.md`
- Modify: `docs/user/config.md`
- Modify: `docs/relaypulse-probe-requirements-zh.md`
- Modify: `config.yaml.example`
- Modify: `config.production.yaml.example`

- [ ] **Step 1: 写失败测试**

  先不写代码测试，改成文档验收清单：确保文档里明确写出 `new-api` 只读边界、日志同步、quick-probe、compare、前端入口和禁写回约束。

- [ ] **Step 2: 跑校验命令**

  Run: `./scripts/check-docs.sh`
  Expected: 标题结构、代码块闭合、命令示例可读。

- [ ] **Step 3: 最小实现**

  只补齐读者真正要执行的内容：
  - 配置什么
  - 运行什么
  - 验证什么
  - 哪些接口只读
  - 哪些功能第一版不做

- [ ] **Step 4: 跑校验命令**

  Run: `./scripts/check-docs.sh`

- [ ] **Step 5: 提交**

  `git add README.md QUICKSTART.md docs/user/config.md docs/relaypulse-probe-requirements-zh.md config.yaml.example config.production.yaml.example`
  `git commit -m "docs: add audit rollout guide"`

---

### Task 10: 端到端验证与发布前门禁

**Files:**
- Modify: `Makefile`
- Modify: `scripts/check-docs.sh`
- Modify: `scripts/verify-env.sh`

- [ ] **Step 1: 写失败测试**

  补齐端到端门禁脚本，检查：
  - Go 测试通过
  - 前端 build 通过
  - SQLite 初始化通过
  - PostgreSQL 初始化通过
  - 审计 API 能返回目标、排名和 compare 结构

- [ ] **Step 2: 跑测试确认失败**

  Run: `make test`

- [ ] **Step 3: 最小实现**

  把审计相关命令接入现有 CI 入口，不引入新的脚本体系：
  - `make test`
  - `make lint`
  - `make build`

- [ ] **Step 4: 跑测试确认通过**

  Run: `make lint && make test && make build`

- [ ] **Step 5: 提交**

  `git add Makefile scripts/check-docs.sh scripts/verify-env.sh`
  `git commit -m "chore: add audit release gates"`

---

## 验收映射

| 需求 | 对应任务 |
|---|---|
| 只读读取 new-api 渠道、模型、映射、启停状态 | Task 1, Task 2, Task 4 |
| 自动生成 `服务商 + 渠道 + 模型` 监控对象 | Task 4 |
| 生产稳定性从 new-api 日志聚合 | Task 5 |
| quick-probe-v1 多步骤诊断 | Task 6 |
| 官方基线对比 | Task 6, Task 7, Task 8 |
| 证据、usage、cache、SSE、header 保存 | Task 6, Task 7, Task 8 |
| 协议指纹评分与标签 | Task 6 |
| 24h / 7d / 30d 趋势 | Task 5, Task 7, Task 8 |
| 不改 new-api 核心代码、不写回 | Task 1, Task 4, Task 7 |
| SQLite / PostgreSQL 双环境 | Task 3, Task 10 |

## 自检清单

- [ ] 计划中所有任务都能落到仓库里真实存在或明确要创建的文件。
- [ ] 没有把 `new-api` 写回、数据库直连或自动切主力/备选/兜底混进第一版。
- [ ] `perf_metrics` 未纳入第一版主路径。
- [ ] 前端实现复用现有 React/Vite，不换技术栈。
- [ ] 每个任务都包含测试、实现、验证、提交四个动作。
