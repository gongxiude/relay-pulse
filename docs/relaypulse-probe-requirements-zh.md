# RelayPulse new-api 只读质量审计需求文档

## 1. 目标与最终产物

本文档是当前实施基线。历史补丁文档仅作为审计留痕，后续执行以本文档为准。

RelayPulse 第一版作为 `new-api` 的只读外部质量审计系统。系统通过 `new-api` HTTP API 同步渠道、模型、模型映射、启停状态和生产日志；生产稳定性优先从 `new-api` 日志聚合；日志无法证明的基础状态由现有模板探测体系补齐；模型真实性、协议指纹、SSE 行为和官方基线对比由 `quick-probe-v1` 诊断承担。

第一版不修改 `new-api`，不读取 `new-api` 数据库，不写回渠道配置，不自动切换主力、备选、兜底。系统只输出监控结果、诊断证据、异常标签、质量趋势和后续调整依据。

最终产物：

| 产物 | 说明 |
|---|---|
| new-api 只读同步器 | 通过 `new-api` HTTP API 读取渠道、模型、模型映射、启停状态 |
| 监控对象生成器 | 按 `服务商 + 渠道 + 模型` 自动生成监控对象 |
| 生产日志分析器 | 从 `new-api` 日志聚合成功率、错误率、超时率、延迟、Tokens/s、cache usage |
| 模板探测补洞任务 | 复用现有模板探测体系，补充日志无法证明的基础可用性和协议错误 |
| quick-probe-v1 诊断任务 | 对指定通道执行多步骤真实性、指纹和基线 compare |
| 页面展示 | 首页服务商列表、服务商详情、模型状态、检测方法和 compare 证据页 |

## 2. 第一版边界

第一版必须遵守以下边界。

| 边界 | 要求 |
|---|---|
| `new-api` 改造 | 不修改 `new-api` 核心转发、计费、选路、quota 逻辑 |
| 数据库 | 默认不读 `new-api` 数据库，不绑定其表结构 |
| 写回 | 不调用任何修改渠道、权重、分组、启停状态的接口 |
| 自动切换 | 不自动切换主力、备选、兜底，只输出依据 |
| perf_metrics | 第一版不接入 `new-api perf_metrics` |
| 计费核验 | 第一版只做 usage 采集和官方价重估，不做完整账单核验 |

只有在以下条件同时成立时，才允许讨论新增 `new-api` 只读 API 或只读 DB 方案：

1. 现有 HTTP API 无法提供关键字段。
2. RelayPulse 无法通过日志和探针补齐证据。
3. 该缺口直接阻塞验收项。
4. 已先评估新增 `new-api` 只读 API 是否比读 DB 更稳妥。

## 3. 四层数据闭环

第一版正式采用四层数据闭环，不再把所有主动能力混成一层。

```text
new-api HTTP API
  -> 渠道同步层
  -> 生产日志分析层
  -> 模板探测补洞层
  -> quick-probe-v1 真实性审计层
  -> 页面、评分、证据、调整依据
```

| 层级 | 数据来源 | 职责 |
|---|---|---|
| 渠道同步层 | `GET /api/channel/` | 发现渠道、模型、模型映射、分组、权重、优先级、启停状态 |
| 生产日志分析层 | `GET /api/log/`、`GET /api/log/stat` | 聚合生产成功率、错误率、超时率、延迟、Tokens/s、cache |
| 模板探测补洞层 | RelayPulse 现有模板探测体系 | 补充基础可用性、认证失败、超时、内容不匹配、协议模板兼容性 |
| quick-probe-v1 审计层 | 配置化诊断任务 | 验证模型真实性、身份、知识、协议指纹、官方基线 compare |

数据来源优先级：

1. 能从 `new-api` HTTP API 直接获取的数据，直接获取。
2. 能从 `new-api` 生产日志聚合的稳定性指标，优先使用日志。
3. 日志样本不足或日志无法说明的基础状态，使用模板探测补洞。
4. 模型真实性、协议指纹和官方基线差异，使用 `quick-probe-v1`。

## 4. new-api 接入

第一版通过环境变量连接 `new-api`，不把凭证写入 YAML。

| 环境变量 | 用途 |
|---|---|
| `NEWAPI_BASE_URL` | `new-api` HTTP API 和 relay 基地址 |
| `NEWAPI_ACCESS_TOKEN` | 读取渠道和日志 |
| `NEWAPI_USER_ID` | 同步接口透传用户身份 |
| `NEWAPI_PROBE_ACCESS_TOKEN` | 可选，主动探针使用的独立 relay 凭证 |
| `NEWAPI_PROBE_USER_ID` | 可选，主动探针使用的用户身份 |

当前同步 channel 不包含 per-channel probe key / api key 字段。第一版不能假定每个 channel 都能自动同步独立探针凭证。

`NEWAPI_PROBE_ACCESS_TOKEN` 只解决“RelayPulse 是否有权限通过 `new-api` relay 发起探测请求”的问题，不决定请求路径、HTTP method、headers 或 body 结构。主动诊断的请求路径和请求体必须由诊断模板提供，不能因为配置了 probe token 就绕过模板直接硬编码调用 `/v1/chat/completions`。

必须同步的渠道字段：

| 字段 | 用途 |
|---|---|
| 渠道 ID | 作为 `new-api` channel_id 关联键 |
| 渠道名称 | 页面展示和稳定标识 |
| 渠道类型 | 判断上游协议和通道类别 |
| Base URL | 作为探测目标入口 |
| 模型列表 | 展开模型级监控对象 |
| 模型映射 | 判断请求模型和实际上游模型 |
| 启停状态 | 展示当前状态；停用渠道不执行主动探测 |
| 分组 / priority / weight | 作为主力、备选、兜底调整依据 |
| tag / other | 保留原始扩展信息，供后续分类和审计 |

不要求从 `new-api` channel 同步密钥内容或密钥状态。页面需要展示的是探针运行模式、最近探针状态和失败原因。

监控对象生成规则：

| 字段 | 生成规则 |
|---|---|
| provider | 来自渠道类型、供应商名或配置映射 |
| channel | 使用 `channel_id + channel_name` 形成稳定标识 |
| model | 展开渠道 models 中的每个模型 |
| request_model | 若存在模型映射，使用映射后的实际上游模型；否则等于 model |
| enabled | 由 `new-api` 启停状态决定 |

## 5. 生产日志分析

生产稳定性指标优先从 `new-api` 日志接口获取，不要求 `new-api` 第一版改代码。

| 数据 | 来源 | 用途 |
|---|---|---|
| 请求日志 | `GET /api/log/` | 拉取消费日志和错误日志 |
| 日志统计 | `GET /api/log/stat` | 获取 quota、RPM、TPM 概览 |

从日志聚合的指标：

| 指标 | 来源字段 | 第一版口径 |
|---|---|---|
| 成功数 | `logs.type = 2` | 按 `channel_id + model_name + group + bucket` 计数 |
| 错误数 | `logs.type = 5` | 按 `channel_id + model_name + group + bucket` 计数 |
| 成功率 | consume / (consume + error) | 生产流量口径 |
| 错误率 | error / (consume + error) | 生产流量口径 |
| 超时率 | error log 的 `content`、`other.error_code`、`other.error_type`、`other.status_code` | 通过规则归类 timeout |
| 总延迟 | `logs.use_time` | 秒级粗粒度 |
| P95 / P99 | `logs.use_time` | 第一版秒级粗算 |
| Tokens/s | `sum(completion_tokens) / sum(use_time)` | 生产日志粗算 |
| cache read/create | `logs.other` | 解析 cache 相关字段 |
| 上游模型 | `logs.other.upstream_model_name` | 观察模型映射和实际使用模型 |
| 流式状态 | `logs.other.stream_status` | 观察流式结束状态和错误摘要 |

生产日志限制：

| 限制 | 处理 |
|---|---|
| `logs.use_time` 是秒级 | P95/P99 标注为日志粗粒度；精确 ms 级由探测补齐 |
| `other` 是 JSON 字符串 | 解析失败时保留原始值并标记字段缺失 |
| 错误日志可能受开关影响 | 页面展示日志覆盖率和最近同步时间 |
| `frt` 不是严格 HTTP TTFB | 严格 TTFB 由主动探测采集 |

## 6. 模板探测补洞层

模板探测补洞层复用 RelayPulse 现有模板探测体系，不另起一套基础探测框架。

职责：

1. 对日志样本不足的通道做轻量补探。
2. 对日志无法说明的基础协议错误做补证据。
3. 对 channel 状态做周期性基础可用性验证。
4. 对通道模板协议兼容性做低成本巡检。

模板探测结果用于：

| 结果 | 用途 |
|---|---|
| 可用 / 不可用 | 补充日志空窗期基础状态 |
| auth_error | 标记探针凭证或上游认证失败 |
| timeout / network_error | 标记网络和上游连接问题 |
| content_mismatch | 标记模板语义响应不符合预期 |
| slow_latency | 标记轻量探测慢请求 |

模板探测结果不直接替代 `quick-probe-v1` 的真实性结论。

## 7. quick-probe-v1 配置化诊断

`quick-probe-v1` 用于真实性、身份、知识、协议指纹和官方基线 compare。它不是普通健康巡检。

配置入口使用独立 `audit.diagnostics` 块，不挂到普通 `monitors` 下。

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
    template_binding:
      default:
        cc: "cx-gpt-chat-diagnostic"
        cx: "cx-gpt-chat-diagnostic"
        gm: "cx-gpt-chat-diagnostic"
        openai: "cx-gpt-chat-diagnostic"
        anthropic: "cx-gpt-chat-diagnostic-notemp"
        gemini: "cx-gpt-chat-diagnostic"
      model_family:
        claude:
          cc: "cx-gpt-chat-diagnostic"
        gpt:
          cx: "cx-gpt-chat-diagnostic"
      channel_type:
        official_direct:
          cc: "cx-gpt-chat-diagnostic"
```

配置字段：

| 字段 | 用途 |
|---|---|
| `enabled` | 是否启用诊断能力 |
| `methodology` | 方法学版本，第一版固定为 `quick-probe-v1` |
| `request_timeout` | 单步请求超时 |
| `step_gap_min` / `step_gap_max` | 第 1-5 步之间的随机等待窗口 |
| `cross_5m_boundary` | 第 3 步是否强制跨 5 分钟缓存边界 |
| `baseline_enabled` | 是否参与官方基线 compare |
| `credential_mode` | 主动诊断使用哪套凭证策略 |
| `template_binding.default` | service 级默认模板 |
| `template_binding.model_family` | model family 级覆盖 |
| `template_binding.channel_type` | channel type / upstream protocol 级覆盖 |

第一版执行路径先使用 `template_binding.default` 完成 service 级模板绑定。当前 `quick-probe-v1` runner 支持 `openai_chat` 请求族，因此绑定的模板必须声明 `request_family`、`override_paths` 和 `response_parser`。OpenAI 兼容通道可使用 `templates/cx-gpt-chat-diagnostic.json`；Anthropic / AWS Bedrock 转 OpenAI 兼容通道必须使用不带 `temperature` 的 `templates/cx-gpt-chat-diagnostic-notemp.json`，否则 `claude-opus-4-8` 会返回 `temperature is deprecated for this model`。普通巡检模板如 `cc-sonnet-arith`、`gm-flash-arith` 可继续用于模板探测补洞，但不能作为 `quick-probe-v1` 的默认诊断模板。

`model_family` 和 `channel_type` 作为配置结构保留，后续用于更细粒度覆盖；未实现覆盖优先级前，不得假定它们已经影响实际探测模板选择。

凭证模式：

| `credential_mode` | 含义 |
|---|---|
| `probe_only` | 只允许使用独立 probe 凭证，没有则任务失败 |
| `probe_fallback` | 优先 probe 凭证，缺失时回退到 `NEWAPI_ACCESS_TOKEN` |
| `newapi_only` | 只使用 `NEWAPI_ACCESS_TOKEN` / `NEWAPI_USER_ID` |

默认使用 `probe_fallback`。

## 8. quick-probe-v1 流程与模板复用

`quick-probe-v1` 负责流程编排，模板体系负责请求骨架。不能把 `quick-probe-v1` 降级成一次模板探活，也不能让普通模板巡检共用 `quick-probe-v1` 的状态机。

职责边界必须固定为：

1. 模板决定怎么请求，包括 URL、HTTP method、headers、body 骨架、协议族和响应解析器。
2. `NEWAPI_PROBE_ACCESS_TOKEN` 决定是否有权限发起这个模板请求。
3. `quick-probe-v1` 决定发什么 prompt、执行几步、是否复用会话、如何保存证据。
4. `quick-probe-v1` 不允许绕过模板直接拼接 `/v1/chat/completions`。

| 层次 | 归属 |
|---|---|
| 六步流程编排 | `quick-probe-v1` |
| 同会话 / 新会话切换 | `quick-probe-v1` |
| prompt 内容 | `quick-probe-v1` |
| compare / baseline | `quick-probe-v1` |
| URL / Method / Headers / Body 骨架 | 模板体系 |
| 变量注入 | 模板体系 |
| 超时 / 重试默认值 | 模板体系，可被 `audit.diagnostics` 覆盖 |
| 基础请求发送 | 公共执行器复用 |

六步流程：

| 步骤 | 名称 | 会话 | 目的 |
|---|---|---|---|
| 1 | ping | 同一会话 | 单字指令遵循，建立缓存上下文 |
| 2 | identity | 同一会话 | 结构化身份自报 |
| 3 | cutoff | 同一会话 | 跨 5 分钟 cache 边界追问 |
| 4 | identity_free | 同一会话 | 自由身份暴露 |
| 5 | knowledge_recall | 同一会话 | 公共事实题 |
| 6 | digit_count | 独立新会话 | 档位判别 |

执行规则：

| 规则 | 要求 |
|---|---|
| 会话复用 | 第 1-5 步必须复用同一会话上下文 |
| 会话隔离 | 第 6 步必须使用独立新会话 |
| 步骤等待 | 第 1-5 步之间支持配置化随机等待 |
| cache 边界 | 第 3 步按配置跨越 5 分钟边界 |
| 失败处理 | 单个通道失败不阻塞其他通道；失败通道保留错误详情 |
| 证据保留 | 每步 prompt、响应摘要、usage、header、错误必须可追溯 |

诊断模板契约：

| 字段 | 要求 |
|---|---|
| `request_family` | 声明请求族，例如 `openai_chat`、`anthropic_messages`、`openai_responses` |
| `url` / `method` / `headers` | 由模板提供，可使用现有变量注入 |
| `body` | 提供请求骨架 |
| `override_paths` | 声明运行时可覆写字段路径，例如 messages、model、stream |
| `response_parser` | 声明响应解析器，用于提取文本、usage、SSE、request id |
| `probe` | 声明模板级 timeout、retry、slow_latency 默认值 |

诊断链路在运行时只能覆写模板契约允许的字段。

## 9. 采集指标与评分

每个诊断步骤必须保存以下指标。

| 指标 | 说明 |
|---|---|
| step_name | 步骤名称 |
| prompt | 本步实际 prompt |
| start_time / end_time | 请求开始和结束时间 |
| duration_ms | 总耗时 |
| http_ttfb_ms | HTTP 响应头到达耗时 |
| first_text_ms | 首个 SSE 文本 delta 到达耗时 |
| input_tokens / output_tokens | 输入和输出 token |
| cache_creation_tokens / cache_read_tokens | cache 创建和读取 |
| cache_5m_tokens / cache_1h_tokens | 5 分钟和 1 小时 cache 指标 |
| tokens_per_second | 输出速度 |
| response_text | 响应摘要或完整响应 |
| raw_usage | 原始 usage 字段 |
| raw_headers | 响应头摘要 |
| raw_sse_events | 流式事件摘要 |
| error_detail | 请求失败时的错误详情 |

V1 必须输出基础评分和异常证据。完整维度化 0-100 评分属于 V1.1 增强。

V1 必做：

| 能力 | 要求 |
|---|---|
| 基础诊断状态 | done、failed_auth、failed_request、partial |
| 基础异常标签 | 通道不可用、认证失败、超时、疑似 fallback、疑似模型不匹配 |
| compare 证据 | 能展示每步 prompt、响应、usage、headers、错误 |
| 官方基线 | 支持基线列或基线快照参与对比 |

V1.1 增强：

| 能力 | 要求 |
|---|---|
| 0-100 指纹分 | 输出总分和维度权重 |
| 维度证据 | 每个维度保存 score、weight、evidence、baseline_value、observed_value |
| 更多异常标签 | 偷换、降智、fallback、缓存异常、协议异常 |
| 长周期趋势 | 30 天诊断分、异常标签、评分趋势 |

## 10. 页面需求

第一版页面以现有首页为入口，不新增 `/audit` 和 `/audit/ranking` 作为核心入口。

页面结构：

| 页面 | 路径 | 要求 |
|---|---|---|
| 首页 | `/` | 服务商列表；只显示当前状态、可用率、可用率趋势；不显示模型列表 |
| 服务商详情 | `/p/:provider` | 点击首页服务商进入；展示该服务商下同步渠道和每个模型状态 |
| 检测方法 | 固定入口 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt` | 使用与首页相同 Header，展示检测方法和最近诊断证据 |
| compare 详情 | `/compare/:run_id` 或现有 compare 路由 | 展示多通道横向对比和官方基线 |

Header 要求：

1. 右上角只保留“首页”和“检测方法”快捷链接。
2. 删除“联系我们”。
3. 删除平台声明入口。
4. 检测方法页面必须复用相同 Header，不新建一套 Header。

首页要求：

| 区块 | 要求 |
|---|---|
| 服务商列表 | 数据来自 `new-api` 同步结果和日志聚合 |
| 当前状态 | 展示从 `new-api` 同步来的启停状态和最近检测状态 |
| 可用率 | 优先来自生产日志聚合 |
| 可用率趋势 | 优先来自生产日志聚合 |
| 模型 | 首页不展示模型列表 |

服务商详情要求：

| 区块 | 要求 |
|---|---|
| 渠道列表 | 展示从 `new-api` 同步来的渠道 |
| 当前状态 | 展示 `new-api` 同步状态、最近日志状态、最近探测状态 |
| 模型状态 | 展示该渠道下每个模型的状态 |
| 通道类型 | 展示“官方直连、混合、逆向、未知” |
| 最近诊断 | 展示最近一次 `quick-probe-v1` 或模板补洞结果 |

通道类型分类只使用以下四类：

| 类型 | 说明 |
|---|---|
| 官方直连 | 官方 API 或明确直连官方上游 |
| 混合 | 多种上游或代理链路混合 |
| 逆向 | 非官方或逆向协议链路 |
| 未知 | 无法判断 |

## 11. 实施里程碑

| 阶段 | 交付 |
|---|---|
| M1 数据接入 | new-api 渠道同步、日志同步、监控对象生成 |
| M2 生产稳定性 | 成功率、错误率、超时率、延迟、Tokens/s、cache 聚合 |
| M3 配置化诊断 | 增加 `audit.diagnostics` 配置块和凭证模式 |
| M4 模板复用 | 抽公共请求执行器，`quick-probe-v1` 复用模板请求骨架 |
| M5 诊断闭环 | `quick-probe-v1` 六步任务、证据保存、基础 compare |
| M6 页面收口 | 首页、服务商详情、检测方法、compare 详情 |
| M7 V1.1 增强 | 完整评分、维度证据、30 天趋势、更多异常标签 |

## 12. V1 验收标准

| 编号 | 验收项 |
|---|---|
| A1 | 配置 `NEWAPI_BASE_URL`、`NEWAPI_ACCESS_TOKEN`、`NEWAPI_USER_ID` 后，系统能拉取全部渠道 |
| A2 | 系统能识别渠道启停状态，停用渠道不执行主动诊断 |
| A3 | 系统能读取每个渠道的模型列表和模型映射 |
| A4 | 系统能自动生成 `服务商 + 渠道 + 模型` 监控对象 |
| A5 | 系统运行期间不会调用任何修改 `new-api` 的接口 |
| A6 | 系统能从 `new-api /api/log/` 聚合成功率、错误率、超时率、延迟、Tokens/s |
| A7 | 系统能解析 `logs.other` 中的 FRT、cache、模型映射、stream_status 字段 |
| A8 | 日志样本不足时，能通过现有模板探测体系补充基础状态 |
| A9 | `quick-probe-v1` 方法学参数可通过 `audit.diagnostics` 配置块管理 |
| A10 | `quick-probe-v1` 请求链路复用模板请求骨架和变量注入 |
| A11 | 对同一模型的多个通道发起诊断后，能生成 compare 证据页 |
| A12 | compare 页面展示 6 个步骤的请求指标、token、cache、响应内容 |
| A13 | 官方基线列或基线快照可参与对比 |
| A14 | 不可用通道不会影响其他通道完成诊断 |
| A15 | 首页服务商列表数据来自真实同步和日志聚合，不使用手填假数据 |
| A16 | 服务商详情页展示同步渠道和每个模型状态 |
| A17 | 整条链路不读 `new-api` DB、不写回 `new-api` |

## 13. 风险与处理

| 风险 | 处理 |
|---|---|
| `/api/log/` 分页拉取成本高 | RelayPulse 做增量同步游标，按时间窗口拉取；后续再考虑只读聚合 API |
| `logs.other` JSON 解析失败 | 保留原始值，标记字段缺失，不阻断同步 |
| channel 不含探针密钥 | 使用 `credential_mode` 明确 probe 凭证和 fallback 行为 |
| admin token 权限过大 | 第一版配置最小可用 token；后续推动只读 scope |
| 诊断任务成本高 | 手动触发为主，周期性只跑轻量模板探测 |
| 模板与诊断协议不匹配 | 增加诊断模板契约，禁止运行时覆写未声明字段 |
| 官方基线漂移 | 基线结果必须带模型、探针版本、采集时间和基线来源 |
| 日志保留时间不足 | RelayPulse 增量同步后自行保留 24h / 7d / 30d 聚合结果 |

## 14. 一句话总结

RelayPulse 第一版作为 `new-api` 的只读外部质量审计系统：通过 `new-api` API 同步渠道和日志，使用生产日志作为稳定性主口径，复用现有模板探测体系补齐基础状态，用配置化 `quick-probe-v1` 做真实性、指纹和官方基线 compare；第一版不读 DB、不写回，只输出后续主力、备选、兜底调整依据。
