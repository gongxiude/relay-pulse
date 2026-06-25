# RelayPulse new-api 只读质量审计需求文档

## 1. 目标与最终产物

RelayPulse 第一版作为 `new-api` 的只读外部质量审计系统。系统通过 `new-api` 现有 HTTP API 拉取渠道、模型、模型映射、启停状态和生产日志；生产稳定性优先从 `new-api` 日志聚合，主动探测只补充生产日志无法证明的模型真实性、协议指纹、SSE 行为和官方基线对比。

第一版不修改 `new-api`，不读取 `new-api` 数据库，不写回渠道配置，不自动切换主力、备选、兜底。系统只输出监控结果、诊断证据、质量排名、异常标签和调整依据。

最终产物：

| 产物 | 说明 |
|---|---|
| new-api 只读同步器 | 通过 `new-api` HTTP API 读取全部渠道、模型、模型映射、启停状态 |
| 监控对象生成器 | 按 `服务商 + 渠道 + 模型` 自动生成监控对象 |
| 生产日志分析器 | 从 `new-api` 日志聚合成功率、错误率、超时率、延迟、Tokens/s、cache usage |
| 主动诊断任务 | 对指定通道执行 `quick-probe-v1` 多步骤真实请求探针 |
| 官方基线对比 | 将待测通道与官方基线通道对照 |
| 协议指纹评分 | 输出 0-100 分的机器指纹分和各维度证据 |
| 质量排名 | 按服务商、通道、模型展示质量分、稳定性和趋势 |
| 证据页面 | 展示每步 prompt、响应、usage、cache、header、SSE 和错误详情 |

## 2. 架构原则与数据来源

为降低对 `new-api` 的侵入，RelayPulse 第一版只作为外部系统工作。

| 原则 | 要求 |
|---|---|
| 不改核心转发链路 | 不修改 `new-api` relay、billing、channel select、quota 等高风险路径 |
| 不读数据库 | RelayPulse 不直接连接 `new-api` DB，不绑定表结构和迁移细节 |
| 不写回配置 | 不调用修改渠道、权重、分组、启停状态的接口 |
| 优先读现有接口 | 渠道和日志均通过 `new-api` HTTP API 获取 |
| RelayPulse 承担分析 | 聚合、评分、诊断、页面、证据都放在 RelayPulse |
| 主动探测补齐证据 | 生产日志不能证明的模型真实性、协议指纹、SSE 行为由 RelayPulse 主动探测 |

整体数据流：

```text
new-api HTTP API
  -> 渠道同步：/api/channel/
  -> 生产日志：/api/log/
  -> RelayPulse 同步层
  -> RelayPulse 日志分析层
  -> RelayPulse 主动探针层
  -> RelayPulse 评分与证据页面
```

数据来源分工：

| 类型 | 数据来源 | 说明 |
|---|---|---|
| 渠道发现 | `new-api /api/channel/` | 读取渠道、模型、模型映射、启停、权重、分组 |
| 生产稳定性 | `new-api /api/log/` | 成功率、错误率、超时率、总延迟、P95/P99、token、cache |
| 通道级精细性能 | RelayPulse 主动探测 | channel 级 TTFT、TTFB、SSE 细节 |
| 真实性诊断 | RelayPulse `quick-probe-v1` | 偷换、降智、fallback、协议指纹、官方基线 |
| 计费准确性 | 第一版只做 usage/官方价重估 | 完整账单核验后置 |

## 3. new-api 接入边界

第一版数据来源固定为 `new-api` HTTP API。禁止读取 `new-api` 数据库，禁止写回 `new-api`，禁止调用任何修改渠道、权重、分组、启停状态的接口。

| 项目 | 要求 |
|---|---|
| 接入方式 | 通过 `new-api` 接口读取全部渠道和日志 |
| 权限 | 使用只读 token；若当前 `new-api` 暂无只读 scope，则使用最小可用管理权限并仅允许 GET |
| 写回 | 第一版禁止写回 |
| 数据同步 | 支持定时同步和手动刷新 |
| 失败处理 | 接口失败时保留最近一次成功同步结果，并标记同步异常 |

必须读取并解析的渠道数据：

| 数据 | 用途 |
|---|---|
| 渠道 ID | 作为 `new-api` channel_id 关联键 |
| 渠道名称 | 作为通道展示名 |
| 渠道类型 | 判断 OpenAI、Anthropic、Gemini 等上游协议 |
| Base URL | 作为探测目标入口 |
| 模型列表 | 展开为模型级监控对象 |
| 模型映射 | 判断请求模型与实际上游模型 |
| 启停状态 | 停用渠道不执行主动探测，仅保留状态 |
| 分组 / priority / weight | 作为后续主力、备选、兜底调整依据 |
| 密钥状态 | 仅记录是否存在或可用，不暴露密钥内容 |

监控对象生成规则：

| 字段 | 生成规则 |
|---|---|
| provider | 来自渠道类型、供应商名或配置映射 |
| channel | 使用 `channel_id + channel_name` 形成稳定标识 |
| model | 展开渠道 models 中的每个模型 |
| request_model | 若存在模型映射，使用映射后的实际上游模型；否则等于 model |
| disabled | 渠道停用时标记为 disabled，不执行主动探测 |

生成对象格式：

```text
new-api channel
  -> provider
  -> channel
  -> model[]
  -> monitor targets: provider + channel + model
```

## 4. 生产日志分析

生产稳定性指标优先从 `new-api` 现有日志接口获取，不要求 `new-api` 第一版改代码。

当前 `new-api` 可用接口和字段：

| 数据 | 来源 | 用途 |
|---|---|---|
| 渠道列表 | `GET /api/channel/`、`GET /api/channel/search`、`GET /api/channel/:id` | 同步渠道、模型、映射、启停状态 |
| 请求日志 | `GET /api/log/` | 拉取消费日志和错误日志 |
| 日志统计 | `GET /api/log/stat` | 获取 quota、RPM、TPM 概览 |

日志类型：

| 类型 | new-api 常量 | 用途 |
|---|---|---|
| 消费日志 | `LogTypeConsume = 2` | 统计成功请求 |
| 错误日志 | `LogTypeError = 5` | 统计失败、上游错误、超时、认证异常 |

从 `new-api` 日志直接聚合的指标：

| 指标 | 来源字段 | 第一版口径 |
|---|---|---|
| 成功数 | `logs.type = 2` | 按 `channel_id + model_name + group + bucket` 计数 |
| 错误数 | `logs.type = 5` | 按 `channel_id + model_name + group + bucket` 计数 |
| 成功率 | consume / (consume + error) | 生产流量口径 |
| 错误率 | error / (consume + error) | 生产流量口径 |
| 超时率 | error log 的 `content`、`other.error_code`、`other.error_type`、`other.status_code` | 通过规则归类 timeout |
| 总延迟 | `logs.use_time` | 秒级粗粒度 |
| P95 / P99 | `logs.use_time` | 第一版秒级粗算 |
| Prompt Tokens | `logs.prompt_tokens` | 生产流量实际 token |
| Completion Tokens | `logs.completion_tokens` | 生产流量实际 token |
| Tokens/s | `sum(completion_tokens) / sum(use_time)` | 秒级粗算 |
| 请求路径 | `logs.other.request_path` | 区分 chat、responses、messages 等入口 |
| 请求 ID | `logs.request_id` | 本地请求追踪 |
| 上游请求 ID | `logs.upstream_request_id` | 上游链路追踪 |

从 `logs.other` 解析的增强指标：

| 字段 | 含义 |
|---|---|
| `frt` | 首响应耗时，可作为生产日志中的 TTFT/FRT 参考 |
| `cache_tokens` | cache read tokens |
| `cache_creation_tokens` | cache create tokens |
| `cache_creation_tokens_5m` | 5m cache create tokens |
| `cache_creation_tokens_1h` | 1h cache create tokens |
| `cache_write_tokens` | 归一后的 cache write tokens |
| `input_tokens_total` | 归一后的总输入 token |
| `is_model_mapped` | 是否发生模型映射 |
| `upstream_model_name` | 实际上游模型名 |
| `stream_status` | 流式结束状态、错误数量、错误信息 |
| `admin_info.use_channel` | 重试链路中使用过的渠道 |

生产日志分析限制：

| 限制 | 处理 |
|---|---|
| `logs.use_time` 是秒级 | 第一版 P95/P99 标注为日志粗粒度；精确 ms 级由主动探测补齐 |
| `other` 是 JSON 字符串 | RelayPulse 解析失败时保留原始值并标记字段缺失 |
| 错误日志可能受开关影响 | 指标页展示日志覆盖率和最近同步时间 |
| `frt` 不是严格 HTTP TTFB | 严格 TTFB 由 RelayPulse 主动探测采集 |

## 5. 主动探测与运行模式

系统同时支持生产日志分析和主动诊断任务。

| 模式 | 触发方式 | 目的 | 输出 |
|---|---|---|---|
| 生产日志分析 | 定时拉取 `new-api` 日志 | 判断真实生产流量下渠道是否稳定 | 24h / 7d / 30d 历史、排名指标 |
| 主动诊断任务 | 用户选择通道后触发 | 判断是否偷换、降智、fallback、协议异常 | compare 详情页、指纹分、证据 |

主动探测不替代生产日志分析。主动探测只用于补齐以下证据：

| 证据 | 为什么不能只靠日志 |
|---|---|
| 严格 TTFB | `new-api` 日志通常只记录总耗时或 FRT |
| SSE 事件细节 | 生产日志只保存 `stream_status` 摘要 |
| 模型身份 | 日志无法证明模型真实自报和响应内容 |
| 偷换 / 降智 / fallback | 需要官方基线和固定探针题对照 |
| 协议指纹 | 需要响应头、body traits、ID 格式等证据 |
| 官方基线对比 | 需要同一探针在官方通道和待测通道上执行 |

## 6. quick-probe-v1 诊断流程

系统必须支持 `quick-probe-v1` 多步骤探针。该探针用于复现 RPDiag 对比页的诊断方式。

| 步骤 | 名称 | 会话 | 目的 | 主要信号 |
|---|---|---|---|---|
| 1 | ping | 同一会话 | 单字指令遵循，建立缓存上下文 | 可用性、TTFB、TTFT、cache_create |
| 2 | identity | 同一会话 | 结构化身份自报 | vendor、brand、model 是否可解析 |
| 3 | cutoff | 同一会话 | 跨 5 分钟 cache 边界追问 | sliding 5m cache、知识截止 |
| 4 | identity_free | 同一会话 | 自由身份暴露 | 包装层、Kiro、自定义身份、官方身份 |
| 5 | knowledge_recall | 同一会话 | 公共事实题 | 知识层级、模型档位、降智信号 |
| 6 | digit_count | 独立新会话 | 档位判别 | output_tokens、推理行为、同厂降级信号 |

执行规则：

| 规则 | 要求 |
|---|---|
| 会话复用 | 第 1-5 步必须复用同一会话上下文 |
| 会话隔离 | 第 6 步必须使用独立新会话 |
| 步骤等待 | 第 1-5 步之间支持 1-4 分钟随机等待 |
| cache 边界 | 第 3 步必须跨越 5 分钟边界 |
| 失败处理 | 单个通道失败不阻塞其他通道；失败通道从主对比表移除，并保留错误详情 |
| 证据保留 | 每步 prompt、响应摘要、usage、header、错误必须可追溯 |

## 7. 主动探针采集指标

每个探针步骤必须保存以下指标。

| 指标 | 说明 |
|---|---|
| step_name | 步骤名称 |
| prompt | 本步实际 prompt |
| start_time / end_time | 请求开始和结束时间 |
| duration_ms | 总耗时 |
| http_ttfb_ms | HTTP 响应头到达耗时 |
| first_text_ms | 首个 SSE 文本 delta 到达耗时 |
| input_tokens | 输入 token |
| output_tokens | 输出 token |
| cache_creation_tokens | 缓存创建 token |
| cache_read_tokens | 缓存读取 token |
| cache_5m_tokens | 5 分钟缓存 token |
| cache_1h_tokens | 1 小时缓存 token |
| tokens_per_second | 输出速度 |
| response_text | 响应摘要或完整响应 |
| raw_usage | 原始 usage 字段 |
| raw_headers | 响应头摘要 |
| raw_sse_events | 流式事件摘要 |
| error_detail | 请求失败时的错误详情 |

每个通道诊断任务必须汇总以下指标。

| 指标 | 说明 |
|---|---|
| total_input_tokens | 总输入 token，包含缓存 |
| total_output_tokens | 总输出 token |
| total_cache_creation_tokens | 总缓存创建 |
| total_cache_read_tokens | 总缓存读取 |
| avg_tokens_per_second | 平均输出速度 |
| total_execution_time | 实际请求执行时间 |
| total_wall_time | 包含步骤等待的墙钟时间 |
| estimated_official_cost | 按官方价目表重估成本 |

## 8. 指标口径审计

第一版必须明确每个指标的来源和可信度。

| 指标 | 第一版来源 | 可信度 | 说明 |
|---|---|---|---|
| 成功率 | `new-api logs` | 高 | 生产流量真实成功率 |
| 错误率 | `new-api logs` | 高 | 依赖错误日志覆盖率 |
| 超时率 | `new-api error logs + other` | 中高 | 需要错误码和文本归类规则 |
| 平均延迟 | `new-api logs.use_time` | 中 | 秒级粗粒度 |
| P95 / P99 | `new-api logs.use_time` | 中 | 秒级粗算 |
| Tokens/s | `completion_tokens / use_time` | 中 | 生产日志粗算 |
| cache read/create | `logs.other` | 中高 | 取决于上游 usage 是否完整 |
| TTFT/FRT | `logs.other.frt` | 中 | 生产口径可参考；精确 channel 级 TTFT 由主动探测补齐 |
| TTFB | RelayPulse 主动探测 | 高 | 探针侧精确采集 |
| SSE 异常 | RelayPulse 主动探测 | 高 | 生产日志只保留摘要 |
| 偷换 / 降智 / fallback | RelayPulse 探针 + 官方基线 | 高 | 需要固定题和证据 |
| 计费准确性 | 后续 | 第一版不做完整核验 | 第一版只做 usage 采集和官方价重估 |

## 9. 协议指纹提取

系统必须从响应头、响应体、usage、SSE 和 ID 字段中提取机器指纹。

| 指纹项 | 示例 |
|---|---|
| platform | `one-api`、`new-api`、`direct`、`unknown` |
| upstream | Anthropic 直连、OpenAI 直连、Gemini 直连、未知代理 |
| cdn | cloudflare、nginx、空 |
| id_format | `msg_`、`chatcmpl-`、`resp_` 等 |
| request_id_chain | `cf-ray`、`x-oneapi-request-id`、官方 request id |
| response_headers_notable | 关键响应头 |
| response_body_traits | `service_tier`、`inference_geo`、cache fields、extra fields |
| signals | 可机器判断的指纹信号列表 |

指纹证据必须能在页面中展开查看，不得只展示最终分数。

## 10. 官方基线与评分

系统必须支持官方基线。官方基线可以来自官方 API 通道的诊断结果，也可以来自管理员维护的基线快照。

评分原则：

1. 以官方基线为参考。
2. 单个异常只记录证据，不直接下最终结论。
3. 连续异常或多个信号一致时，才判定异常标签。
4. 分数用于排名、趋势观察和调整依据，不等于安全认证。

协议指纹评分必须输出总分和维度明细。

| 维度 | 要求 |
|---|---|
| 总分 | 0-100 |
| 基线状态 | 是否已对照官方基线 |
| cache 命中比 | 对比官方基线 cache_read 表现 |
| cache TTL 一致性 | 判断 5m / 1h cache 行为是否合理 |
| 模型匹配 | 比对请求模型、响应模型、自报模型 |
| 缓存连续性 | 判断前后步骤 cache 创建/读取是否连续 |
| sliding 5m | 判断跨 5 分钟后是否仍命中缓存 |
| 原生 msg-ID | 判断响应 ID 是否接近官方格式 |
| 系统提示纯净 | 判断是否暴露包装层或被注入额外身份 |
| 身份结构化 | 判断 vendor/brand/model 是否符合预期 |
| 知识截止 | 与官方基线对比 |
| 身份自由格式 | 检测 Kiro、代理包装、自定义身份 |
| service_tier | 是否包含官方类似字段 |
| inference_geo | 是否包含官方类似字段 |
| 延迟基线 | 与官方基线延迟对比 |
| 流式投递 | SSE delta 是否正常 |
| Req-ID 透传 | 请求链路 ID 是否可追踪 |

每个维度必须保存：

| 字段 | 说明 |
|---|---|
| score | 0-10 维度分 |
| weight | 权重 |
| evidence | 判分证据 |
| baseline_value | 官方基线值 |
| observed_value | 待测通道值 |

## 11. 异常标签

系统必须输出异常标签，但不能因单个异常直接下结论。

| 标签 | 判定依据 |
|---|---|
| 疑似偷换模型 | 响应模型、自报模型、知识层级、digit_count 与请求模型明显不一致 |
| 疑似降智 | 同厂但输出 token、知识层级、推理行为低于官方基线 |
| 疑似 fallback | 部分步骤表现为另一模型、另一供应商或另一协议特征 |
| 缓存异常 | cache_read/cache_create 与官方或预期行为不一致 |
| 协议异常 | SSE、headers、body traits、ID 格式明显偏离 |
| 通道不可用 | 请求失败、认证失败、上游 4xx/5xx、超时 |

异常标签必须关联证据，包括触发步骤、原始响应摘要、指纹差异和评分维度。

## 12. 页面需求

第一版页面必须支撑“查看排名、发起诊断、查看证据”三个核心动作。

| 页面 | 内容 |
|---|---|
| 诊断首页 | 最近诊断任务、同步状态、生产日志覆盖率、诊断入口 |
| 排名页 | 服务商/通道/模型质量排名，区分生产稳定性分和主动诊断分 |
| 提交测试页 | 选择 new-api 渠道和模型，发起诊断 |
| 对比详情页 | 多通道横向对比，包含官方基线 |
| 通道详情页 | 单通道生产历史、评分、异常证据 |
| 模型详情页 | 同模型不同通道对比 |
| 30 天历史页 | 稳定性、评分趋势、异常趋势 |
| 登录后状态页 | 内部查看完整证据和敏感字段 |

对比详情页必须展示：

| 区块 | 要求 |
|---|---|
| 测试说明 | 展示探针版本和每步说明 |
| 摘要卡片 | 通道数、步骤数、官方基线、低分数量、失败数量 |
| 不可用通道 | 展示失败通道、错误类型、响应摘要 |
| 主对比表 | 按步骤横向展示各通道指标 |
| Prompt 展开 | 每步可查看实际 prompt |
| 响应展开 | 每步可查看响应内容 |
| 总计区 | token、cache、耗时、官方成本重估 |
| 指纹区 | 平台、上游、CDN、ID 格式、headers/body signals |
| 评分区 | 总分和各维度分数、权重、证据 |

## 13. 第一版边界

第一版必须实现：

1. 通过 `new-api` 接口只读发现全部渠道。
2. 自动生成 `服务商 + 渠道 + 模型` 监控对象。
3. 从 `new-api` 日志聚合生产稳定性指标。
4. 手动发起 `quick-probe-v1` 诊断任务。
5. 支持官方基线对比。
6. 保存每步指标和证据。
7. 输出协议指纹评分。
8. 展示对比详情页。
9. 展示通道不可用错误。
10. 生成疑似偷换、降智、fallback 标签。
11. 展示 24h / 7d / 30d 历史趋势。

第一版不实现：

| 项目 | 处理 |
|---|---|
| 修改 `new-api` 核心代码 | 不做，保持与官方同步低冲突 |
| 自动写回 new-api | 后续做，必须审批 |
| 自动切主力/备选/兜底 | 后续做，第一版只输出依据 |
| 完整计费核验 | 后续做，第一版只做 usage 采集和官方价重估 |
| Incident / RCA | 后续做 |
| 告警通知 | 后续做 |
| SEV 分级 | 后续做 |
| `new-api perf_metrics` 接入 | 后续可选增强，第一版先不接入 |
| channel 级 perf_metrics schema 改造 | 后续可选增强 |

## 14. 后续可选增强

后续如确实需要改 `new-api`，只做小型、独立、可选增强，避免影响官方同步。

| 增强 | 目的 |
|---|---|
| 新增只读聚合 API | 例如 `/api/log/relaypulse/metrics`，减少 RelayPulse 分页拉日志成本 |
| 接入 `new-api /api/perf-metrics` | 后续补充 model+group 级性能趋势 |
| perf_metrics 增加 channel_id 维度 | 支持 channel 级 TTFT、Latency、SuccessRate、AvgTps |
| 暴露结构化只读渠道能力接口 | 减少 RelayPulse 对现有管理接口响应结构的适配成本 |
| 增加只读 token scope | 避免使用完整 admin 权限 |
| 日志增加毫秒级 latency | 支持更准确的 P95/P99 |

## 15. 风险与处理

| 风险 | 处理 |
|---|---|
| `/api/log/` 分页拉取成本高 | RelayPulse 做增量同步游标，按时间窗口拉取；后续再考虑只读聚合 API |
| `logs.other` JSON 解析失败 | 保留原始值，标记字段缺失，不阻断同步 |
| `perf_metrics` 没有 channel_id | 第一版不接入；后续如需要趋势补充，再先接 model+group 维度 |
| admin token 权限过大 | 第一版配置最小可用 token；后续推动只读 scope |
| 诊断任务成本高 | 手动触发为主，周期性只跑轻量探测 |
| 官方基线漂移 | 基线结果必须带模型、探针版本、采集时间和基线来源 |
| 日志保留时间不足 | RelayPulse 增量同步后自行保留 24h / 7d / 30d 聚合结果 |

## 16. 实施里程碑

| 阶段 | 交付 |
|---|---|
| M1 数据接入 | new-api 渠道同步、日志同步、监控对象生成 |
| M2 生产稳定性 | 成功率、错误率、超时率、延迟、Tokens/s、cache 聚合 |
| M3 诊断探针 | `quick-probe-v1` 六步任务、证据保存 |
| M4 基线评分 | 官方基线、协议指纹、异常标签 |
| M5 页面落地 | 排名页、通道详情、compare 详情、30 天趋势 |
| M6 后续增强 | 告警、SEV、RCA、只读 scope、`new-api perf_metrics` 接入、channel 级 perf_metrics |

## 17. 验收标准

| 编号 | 验收项 |
|---|---|
| A1 | 配置 `new-api` 地址和只读 token 后，系统能拉取全部渠道 |
| A2 | 系统能识别渠道启停状态，停用渠道不执行主动探测 |
| A3 | 系统能读取每个渠道的模型列表和模型映射 |
| A4 | 系统能自动生成 `服务商 + 渠道 + 模型` 监控对象 |
| A5 | 系统运行期间不会调用任何修改 `new-api` 的接口 |
| A6 | 系统能从 `new-api /api/log/` 聚合成功率、错误率、超时率、延迟、Tokens/s |
| A7 | 系统能解析 `logs.other` 中的 FRT、cache、模型映射、stream_status 字段 |
| A8 | 对同一模型的多个通道发起诊断后，能生成一个 compare 页面 |
| A9 | compare 页面展示 6 个步骤的请求指标、token、cache、响应内容 |
| A10 | 官方基线列可参与对比 |
| A11 | 不可用通道不会影响其他通道完成诊断 |
| A12 | 每个通道都有 0-100 指纹分和各维度分 |
| A13 | 每个评分维度能展开看到证据 |
| A14 | 系统能识别并标记至少三类异常：偷换、降智、fallback |
| A15 | 诊断结果和日志聚合结果可长期保存，用于后续主力、备选、兜底调整依据 |

## 18. 一句话总结

RelayPulse 第一版作为 `new-api` 的只读外部质量审计系统：生产稳定性优先从 `new-api` 日志聚合，主动探测补充生产日志无法证明的模型真实性、协议指纹、SSE 行为和官方基线对比；第一版不写回 `new-api`，只输出主力、备选、兜底调整依据。
