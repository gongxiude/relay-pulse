# RelayPulse 审计数据填充实施 Plan

## 1. 目标与最终产物

本计划只解决一个问题：把 RelayPulse 当前“有入口、有表、有页面壳，但诊断数据基本为空”的状态，改造成“首页、详情页、检测方法页都能展示真实审计数据”的可落地系统。

最终产物必须是：

| 产物 | 说明 | 验收标准 |
|---|---|---|
| 真实数据盘点页 | 明确哪些数据已经来自 `new-api`，哪些还没有 | 首页和接口不再把“全空”混成一个问题 |
| 诊断结果结构化 schema | `diagnostics/:run_id` 和 `compare/:run_id` 不再返回 `any` 壳 | 接口字段固定，前端不再靠猜字段 |
| baseline 对照链路 | 候选通道与官方基线可成组落库、对比、评分 | compare 页可展示 candidate vs baseline |
| 维度级评分数据 | 至少落第一版可执行的维度分、权重、证据 | 每次诊断有维度表、有 evidence |
| 首批真实种子数据 | 对现有启用通道生成一批可看的诊断结果 | 详情页不是空表，至少有可解释的样本 |
| 页面真实填充 | `/p/:provider`、检测方法页、compare 详情页使用本地真实审计数据 | 页面不再只显示静态说明或占位 |

## 2. 运行时边界与输入

执行本计划时，边界固定如下：

| 项目 | 要求 |
|---|---|
| `new-api` 接入 | 只通过 `NEWAPI_BASE_URL`、`NEWAPI_ACCESS_TOKEN`、`NEWAPI_USER_ID` 读取 |
| 对 `new-api` 的影响 | 只读，不改数据库，不写回渠道，不切主备 |
| 当前存储 | 先以 SQLite 跑通，保持 PostgreSQL 兼容 |
| 当前页面入口 | 首页 `/`、详情页 `/p/:provider?...`、检测方法页 |
| 评分参考 | 对齐 `https://diag.relaypulse.top/about` 的相对评分方法，但第一版只实现能落地的数据维度 |

## 3. 当前真实数据现状

截至 2026-06-25，本地运行中的接口已经有一部分真实数据，不是“全空”。

### 3.1 已有真实数据

来自本地接口 `http://localhost:8080`：

| 接口 | 当前状态 | 已确认数据 |
|---|---|---|
| `/api/audit/newapi/sync/status` | 有真实值 | `targets.total=149`、`targets.enabled=50`、`channels.channel_count=10`、`channels.enabled_count=3`、`log_cursor.last_id=832312` |
| `/api/audit/targets` | 有真实值 | 已生成 `provider + service + channel + model + request_model + enabled` 审计对象 |
| `/api/audit/ranking?window=24h` | 有真实值 | 已有生产日志聚合结果：`total/success/error/timeout/success_rate/p95/p99/tokens_per_second/avg_frt/score` |
| `/api/audit/channels` | 有真实值 | 已能显示 `new-api` 同步过来的渠道快照和启停状态 |

这部分说明当前系统已经具备：

1. `new-api` 只读同步
2. 监控对象自动生成
3. 基于 `new-api` 生产日志的稳定性聚合

### 3.2 当前仍然为空或不完整的数据

| 数据链路 | 当前问题 | 影响页面 |
|---|---|---|
| `POST /api/audit/diagnostics` 产出的 run 数据 | 已有 runner，但 prompt、步骤、评分都过薄 | 检测方法页、详情页 |
| `GET /api/audit/diagnostics/:run_id` | `auditDiagnosticResponse` 仍是 `any` | 前端无法稳定渲染 |
| `GET /api/audit/compare/:run_id` | 只是把 steps 原样返回，没有 baseline 结构 | compare 页无法做真正对照 |
| baseline 历史窗口 | 没有官方基线快照表，没有近 3 次窗口 | 无法实现相对评分核心 |
| 25 维 method data | 没有维度分、权重、skip 原因、evidence | 页面只能写说明，不能展示结果 |
| 首批诊断样本 | 没有为现有启用通道批量生成 run | 用户点进去看到的大多还是空 |

## 4. 当前代码边界

本次实施必须围绕现有文件补强，不能重新发明一套平行系统。

| 文件 | 当前作用 | 本次要做什么 |
|---|---|---|
| [internal/api/audit_handler.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_handler.go) | 审计 API 入口 | 补结构化响应、compare 聚合响应、baseline 查询 |
| [internal/api/audit_types.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_types.go) | API schema | 去掉 `any`，改成明确结构 |
| [internal/audit/diagnostic_runner.go](/Users/gongxiude/Documents/github/relay-pulse/internal/audit/diagnostic_runner.go) | 当前 quick probe runner | 从“简版探针”升级到“带基线、带维度证据”的 runner |
| [internal/storage/audit_models.go](/Users/gongxiude/Documents/github/relay-pulse/internal/storage/audit_models.go) | 审计表定义 | 增加 baseline、dimension、evidence、snapshot 表 |
| [frontend/src/pages/DetectPage.tsx](/Users/gongxiude/Documents/github/relay-pulse/frontend/src/pages/DetectPage.tsx) | 检测方法页 | 改为消费真实诊断/方法数据，而不是静态壳 |
| 详情页相关前端页面 | 展示通道/模型详情 | 接入真实 compare、维度分、最近诊断摘要 |

## 5. `about` 页方法论拆解为可落地数据

根据 `https://diag.relaypulse.top/about`，rpdiag 核心不是“绝对分”，而是“同一时刻、同一组 prompt、同一抓包条件下，candidate 对 official baseline 的相对差异评分”。

### 5.1 第一版必须落地的评分契约

| 项目 | 第一版要求 |
|---|---|
| 评分类型 | 相对评分，不做绝对真假判定 |
| baseline | 没有近 3 次窗口时，允许先回退到“同组单次 baseline” |
| 分值输出 | `overall_score = Σ(weight × dim_score) / Σ(active_weight) × 10` |
| skip 规则 | baseline 缺失、信号不可比时必须 `skip`，不能强打 0 |
| evidence | 每个维度必须记录 `actual/baseline/result/reason` |

### 5.2 第一版可执行维度

这些维度可以基于现有 `new-api` 接入方式和当前 runner 补出来：

| 维度 | 数据来源 | 第一版实现方式 |
|---|---|---|
| `model_match` | 响应体 / usage / envelope | 比较请求模型与响应模型 |
| `identity_structured_match` | identity step 文本 | 解析 vendor/brand/model 三行 |
| `identity_free_clean` | identity_free step 文本 | 检测 wrapper 身份词 |
| `cutoff_match` | cutoff step 文本 + baseline | 比较知识截止月份 |
| `knowledge_recall_match` | knowledge step 文本 + baseline | 对固定事实题做逐题对照 |
| `instruction_following_lang` | identity_free step 文本 | 计算 CJK 占比 |
| `cache_hit_ratio_match` | usage / cache 字段 | 比较 `cache_read / total_input` |
| `cache_continuity_intra` | 连续步骤 usage | 比较相邻步骤上下文连续性 |
| `cache_sliding_correctness` | step1-3 usage | 判断 5m sliding 行为 |
| `cache_ttl_consistency` | usage/raw headers/raw json | 看 5m/1h 字段表面是否出现 |
| `anthropic_msg_id_format` | 响应 ID | 匹配 `msg_01` 等格式 |
| `anthropic_request_id_passthrough` | 响应 headers | 看是否透传上游 `req_*` |
| `service_tier_present` | usage/raw body | 检测 `service_tier` |
| `inference_geo_present` | raw body | 检测 geo 字段 |
| `latency_baseline_match` | `ttfb_ms`、`first_text_ms`、`duration_ms` | 与 baseline 中位数比较 |
| `buffer_dump_match` | raw SSE events | 比较 visible text span 与 chunk 分布 |

### 5.3 第二版再做的维度

这些维度第一版不要硬上：

| 维度/能力 | 暂缓原因 |
|---|---|
| 近 3 次基线完整滚动窗口 + 版本冻结混算 | 先把单次 baseline 链路跑通 |
| `thinking_volume_match`、`tier_thinking_volume_match` | 依赖更细的 SSE thinking block 解析 |
| 完整 25 维全部复刻 | 当前 runner 和本地页面还没有稳定 compare schema |
| 跨服务统一 scorer | 先把 Anthropic / OpenAI 主要路径做实 |

## 6. 数据模型补全方案

当前 `diagnostic_runs / steps / scores` 太薄，第一版至少要补 4 类表。

### 6.1 新增或扩展的存储实体

| 表/实体 | 用途 | 最少字段 |
|---|---|---|
| `diagnostic_baseline_runs` | 存官方 baseline run | `baseline_id`、`service`、`model_family`、`run_id`、`captured_at`、`methodology_version` |
| `diagnostic_run_groups` | 绑定 candidate 与 baseline 为同一测试组 | `group_id`、`candidate_run_id`、`baseline_run_id`、`window_fallback_mode` |
| `diagnostic_dimensions` | 每次 run 的维度级评分 | `run_id`、`dimension_key`、`weight`、`score_0_10`、`normalized_score`、`status`、`reason` |
| `diagnostic_evidences` | compare 页证据明细 | `run_id`、`step_index`、`dimension_key`、`actual_json`、`baseline_json`、`diff_json` |

### 6.2 现有表要补的字段

| 实体 | 要补字段 | 原因 |
|---|---|---|
| `DiagnosticRun` | `GroupID`、`BaselineMode`、`MethodologyVersion`、`WeightsHash`、`CandidateType` | 让每个分数可追溯 |
| `DiagnosticStep` | `StepName`、`SessionMode`、`HTTPTTFBMs`、`FirstTextMs`、`DurationMs`、`UsageJSON`、`HeadersJSON`、`SSEJSON` | compare 页不能只靠 `execution_meta` 黑盒 |
| `DiagnosticScore` | `OverallScore`、`ActiveWeight`、`SkippedDimensions`、`DimensionSummaryJSON` | 页面需要直接消费总分和摘要 |

## 7. 后端实施步骤

### Phase 1: 固化接口 schema

**目标**：先把“空壳接口”改成稳定契约。

**修改文件**
- [internal/api/audit_types.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_types.go)
- [internal/api/audit_handler.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_handler.go)

**执行项**
1. 把 `auditDiagnosticResponse.Run/Score/Steps` 从 `any` 改为明确类型。
2. 新增 `auditCompareResponse`：
   - `group`
   - `candidate`
   - `baseline`
   - `dimensions`
   - `steps`
   - `summary`
3. `GET /api/audit/compare/:run_id` 不再直接回 `compare: respSteps`。

**验收**
- 前端不需要再猜测 `execution_meta` 里的字段名。

### Phase 2: 跑通 baseline 存储与组装

**目标**：让一次诊断不再是单边 candidate，而是 candidate + baseline 成组。

**修改文件**
- [internal/storage/audit_models.go](/Users/gongxiude/Documents/github/relay-pulse/internal/storage/audit_models.go)
- [internal/audit/diagnostic_runner.go](/Users/gongxiude/Documents/github/relay-pulse/internal/audit/diagnostic_runner.go)

**执行项**
1. 定义 baseline 选择规则：
   - 优先近 3 次同服务、同模型族、同方法版本的 baseline
   - 没有历史窗口时回退到当前同组单次 baseline
2. runner 执行顺序改为：
   - 创建 `run_group`
   - 跑 baseline
   - 跑 candidate
   - 计算维度
   - 保存 compare 结果
3. baseline 也要保存完整 steps、usage、headers、sse 证据。

**验收**
- 任一 candidate run 都能追到对应 baseline run。

### Phase 3: 补第一版维度 scorer

**目标**：先把能落地的维度做实，不追求一次补满 25 维。

**修改文件**
- [internal/audit/diagnostic_runner.go](/Users/gongxiude/Documents/github/relay-pulse/internal/audit/diagnostic_runner.go)
- 新增 `internal/audit/scorers/*.go`

**执行项**
1. 把现有 `scoreDiagnosticRun` 从 tag 打分改为“维度列表 + 权重归一化”。
2. 每个 scorer 输出统一结构：
   - `dimension_key`
   - `weight`
   - `score_0_10`
   - `status = pass/fail/partial/skip`
   - `reason`
   - `actual`
   - `baseline`
3. 先实现第 5.2 节列出的第一版维度。

**验收**
- 单次 compare 可展示“总分 + 维度表 + 每维证据”。

### Phase 4: 批量回填首批真实样本

**目标**：解决“页面有 schema 但没有数据看”的问题。

**执行项**
1. 从 `/api/audit/targets` 中筛出 `enabled=true` 的对象。
2. 第一批只选：
   - 每个 `provider + service + channel` 的前 1-3 个主模型
   - 优先 24h 内有生产日志的对象
3. 生成批量诊断任务：
   - baseline 先跑一轮
   - candidate 再跑一轮
4. 将结果落库，形成“最近诊断摘要”。

**验收**
- 首页点进详情页时，至少能看到最近一次真实诊断。

## 8. 前端填充方案

### 8.1 首页与详情页的数据口径

| 页面 | 继续使用的数据 | 新补的数据 |
|---|---|---|
| 首页 `/` | `/api/audit/channels`、`/api/audit/ranking` | 最近诊断状态摘要 |
| 详情页 `/p/:provider?...` | `new-api` 同步模型、启停状态、24h/30d 稳定性 | 最近 compare 总分、维度摘要、检测入口 |
| 检测方法页 | 方法说明 | 真实维度定义、当前版本、最近样本 |
| compare 页 | 无 | candidate vs baseline 步骤对照、维度证据 |

### 8.2 前端落地要求

1. 检测方法页不再只放静态说明，必须显示：
   - 当前 `methodology_version`
   - active dimensions
   - 每维权重
   - 当前样本覆盖数
2. 详情页每个模型至少显示：
   - 当前启停状态
   - 24h/30d 生产成功率
   - 最近一次诊断总分
   - 最近一次诊断时间
   - compare 入口
3. compare 页至少显示：
   - candidate / baseline 概览
   - step-by-step 表
   - 维度分表
   - evidence 折叠区

## 9. 数据填充优先级

按用户当前可见价值排序：

| 优先级 | 任务 | 原因 |
|---|---|---|
| P0 | `diagnostics` / `compare` 结构化 | 没有 schema，前端无法落地 |
| P0 | baseline 成组执行 | 没有 baseline，就不是 rpdiag 式相对评分 |
| P0 | 首批真实 run 回填 | 没有样本，页面还是空 |
| P1 | 第一版维度 scorer | 没有维度分，compare 没意义 |
| P1 | 详情页显示最近诊断摘要 | 用户入口页直接看到结果 |
| P2 | 检测方法页显示当前版本、权重和覆盖数 | 方法页从“文案页”变成“运行中状态页” |
| P2 | 基线窗口扩展到近 3 次 | 先单次基线，后窗口 |

## 10. 测试与验收

### 10.1 后端测试

必须新增或补齐：

| 测试 | 验收 |
|---|---|
| `audit_types` / handler schema 测试 | compare 和 diagnostics 响应字段稳定 |
| storage 测试 | baseline/group/dimension/evidence 可读写，SQLite 与 PostgreSQL 都过 |
| scorer 测试 | 每个第一版维度都有 pass/fail/skip 用例 |
| runner 集成测试 | baseline + candidate 成组执行并落库 |

### 10.2 页面验收

必须满足：

1. 首页不再把“生产稳定性数据为空”和“诊断数据为空”混为一谈。
2. 详情页点入任一启用通道时，至少一个模型能看到最近诊断结果。
3. compare 页能同时展示 candidate 和 baseline 的步骤对照。
4. 检测方法页能显示真实版本号、维度数、权重和样本覆盖数。

## 11. 实施顺序

按这个顺序执行，不要并行乱改前后端：

1. 固化 API schema
2. 扩存储表
3. 跑通 baseline 组装
4. 实现第一版维度 scorer
5. 回填首批 enabled targets 的真实诊断样本
6. 前端接 compare / diagnostics / methodology 数据
7. 再做基线窗口和更多维度

## 12. 本计划对应的事实来源

| 来源 | 用途 |
|---|---|
| [docs/relaypulse-probe-requirements-zh.md](/Users/gongxiude/Documents/github/relay-pulse/docs/relaypulse-probe-requirements-zh.md) | 当前需求边界 |
| [internal/api/audit_handler.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_handler.go) | 当前接口实现 |
| [internal/api/audit_types.go](/Users/gongxiude/Documents/github/relay-pulse/internal/api/audit_types.go) | 当前 schema 空壳位置 |
| [internal/audit/diagnostic_runner.go](/Users/gongxiude/Documents/github/relay-pulse/internal/audit/diagnostic_runner.go) | 当前 quick probe 与评分逻辑 |
| [internal/storage/audit_models.go](/Users/gongxiude/Documents/github/relay-pulse/internal/storage/audit_models.go) | 当前表结构 |
| `http://localhost:8080/api/audit/newapi/sync/status` | 当前真实同步状态 |
| `http://localhost:8080/api/audit/targets` | 当前真实监控对象 |
| `http://localhost:8080/api/audit/ranking?window=24h` | 当前真实生产聚合数据 |
| https://diag.relaypulse.top/about | rpdiag 方法论与维度定义 |
