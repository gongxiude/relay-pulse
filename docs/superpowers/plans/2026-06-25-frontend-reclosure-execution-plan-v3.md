# RelayPulse 前端收口执行 Plan V3

## 1. 目标与最终产物

本计划只处理当前已经确认的前端收口任务，不扩展新页面，不改变后端边界，不触碰 `8080` Docker 容器基线。

最终产物必须是：

| 产物 | 说明 | 验收标准 |
|---|---|---|
| 首页 `/` 收口版 | 首页只承担服务商入口职责 | 只展示服务商列表，只保留可用率和可用率趋势，不显示模型 |
| 服务商详情页 `/p/:provider` 收口版 | 从首页点击服务商进入详情页 | 页面按 `new-api` 同步的真实渠道展开，并展示每个模型的状态 |
| 统一 Header 收口版 | 全站使用同一套头部 | 右上角只保留“首页”“检测方法”，检测方法固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt` |
| 旧入口语义清理版 | 删除被否定的入口语义 | 页面主入口中不再出现“联系我们”“平台声明” |
| 本地验证基线 | 固定使用 `18080` 验证 | 前端 build、embed 同步、重启本地 Go 服务、页面与接口核对全部完成 |

## 2. 当前运行边界

| 项目 | 固定边界 |
|---|---|
| `8080` | Docker 容器基线，保留不动，只用于参考 |
| `18080` | 当前工作区本地验证态，后续所有页面验收都以它为准 |
| 前端源码 | `frontend/src` |
| 前端构建产物 | `frontend/dist` |
| Go embed 副本 | `internal/api/frontend/dist` |
| embed 规则 | 只要前端有改动，必须执行 `cd frontend && npm run build` → `rm -rf internal/api/frontend && mkdir -p internal/api/frontend && cp -r frontend/dist internal/api/frontend/` → 重启本地 Go 服务 |
| `new-api` 接入 | 只读，仅读取 `NEWAPI_ACCESS_TOKEN`、`NEWAPI_BASE_URL`、`NEWAPI_USER_ID` |
| 后端边界 | 第一版不写回 `new-api`，不推进 `perf_metrics` |

## 3. 当前真实数据来源

以下结论基于当前工作区与 `http://127.0.0.1:18080` 的真实返回：

| 数据 | 来源接口/代码 | 当前证据 |
|---|---|---|
| 服务商 / 渠道 / 模型 / 启停状态 | `/api/audit/channels` | 当前返回 `10` 条渠道快照，字段包含 `provider/service/channel/model/enabled/channelType/channelTypeLabel` |
| 监控对象 | `/api/audit/targets` | 当前返回 `149` 条目标，按 `服务商 + 渠道 + 模型` 展开 |
| 首页可用率和趋势 | `useMonitorData` + `adaptAuditChannelsToMonitorData` + `StatusTable` | 当前首页已从监控数据和同步快照聚合出服务商列表 |
| 详情页最新检测状态 | `/api/audit/diagnostics/latest` | 当前 `ProviderPage.tsx` 已按 provider/service/channel 读取最近检测 |
| 方法论统计 | `/api/audit/methodology` | 当前子任务 plan 已记录 `implemented_count=10`、`active_count=10`、`active_weight=69` |
| compare 详情 | `/api/audit/compare/:run_id` | 当前 `DetectComparePage.tsx` 已消费真实 compare schema |

## 4. 页面入口与页面职责

| 页面 | 固定职责 | 禁止事项 |
|---|---|---|
| `/` | 服务商入口页 | 不再承载旧监控大盘、旧公告、旧营销入口语义 |
| `/p/:provider` | 服务商详情页 | 不允许展示伪造渠道；必须以 `new-api` 同步渠道为准 |
| `/detect` | 检测方法页 | 继续复用同一个 Header，不新增第二套头部 |
| `/detect/compare/:runId` | 单次检测详情页 | 保持真实 compare 数据展示，不扩展新职责 |

## 5. 保留 / 删除 / 重做清单

### 5.1 保留并继续迭代

| 文件路径 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/App.tsx` | 首页入口页，已按 `providerId + serviceType` 聚合服务商行 | 保留 | 方向正确，已满足“首页是 `/`、不显示模型”；但首页文案和表格语义还需继续收口 | 继续压缩为纯服务商入口语义 |
| `frontend/src/components/Header.tsx` | 全站统一头部 | 保留 | 当前已经只保留“首页 / 检测方法”主导航，方向正确 | 继续清理非必要头部语义，保持同一套 Header |
| `frontend/src/pages/ProviderPage.tsx` | 服务商详情页，按真实同步渠道展示模型状态 | 保留 | 数据链路正确，已经使用 `useAuditChannels`、`useAuditDiagnosticLatest`、`useAuditSyncStatus` | 继续重排页面结构，贴近“先看渠道，再看每个模型状态” |
| `frontend/src/components/StatusTable.tsx` | 现有主表格组件 | 保留 | 这是当前最值得复用的表格组件，不能重写一套新表格 | 继续用 props 裁剪，不新增平行表格组件 |
| `frontend/src/pages/DetectPage.tsx` | 检测方法页 | 保留 | 已复用统一 Header 并接真实 methodology 数据 | 本轮只做必要收口，不新增新结构 |
| `frontend/src/router.tsx` | 路由边界定义 | 保留 | 当前 `/`、`/p/:provider`、`/detect`、`/detect/compare/:runId` 边界正确 | 不改路由边界 |
| `frontend/src/utils/auditChannelAdapter.ts` | `new-api` 渠道快照适配层 | 保留 | 首页和详情页都依赖这层接真实数据 | 继续作为唯一适配入口使用 |

### 5.2 需要继续删除或清理的内容

| 文件路径 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/i18n/locales/en-US.json` | 英文文案资源 | 保留，但要清理 | 当前仍残留 `Contact Us` 等旧入口文案 | 删除“联系我们”相关入口文案，避免旧语义回流 |
| `frontend/src/i18n/locales/ru-RU.json` | 俄文文案资源 | 保留，但要清理 | 当前仍残留旧联系页文案 | 与其他 locale 一起清理旧入口词条 |
| `frontend/dist` | 前端构建产物 | 保留，但不直接编辑 | 当前构建产物里仍能搜到旧文案，这是源码残留的结果 | 后续通过重新 build 自动更新 |
| `internal/api/frontend/dist` | Go embed 副本 | 保留，但不直接编辑 | `18080` 当前实际运行的是这份副本 | 源码修改后重新同步 embed |

### 5.3 已确认是废代码并已删除

| 文件路径 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/components/AnnouncementsBanner.tsx` | 旧首页公告横幅 | 不保留 | 不属于服务商入口页职责 | 已删除，不再恢复 |
| `frontend/src/components/EmptyFavorites.tsx` | 旧收藏空状态 | 不保留 | 首页不再以收藏视图作为入口能力 | 已删除，不再恢复 |
| `frontend/src/components/StatusCard.tsx` | 旧首页卡片视图 | 不保留 | 当前首页不需要双视图体系 | 已删除，不再恢复 |
| `frontend/src/hooks/useAnnouncements.ts` | 旧公告数据拉取 | 不保留 | 仅服务旧首页公告 | 已删除，不再恢复 |
| `frontend/src/hooks/useFavorites.ts` | 旧收藏逻辑 | 不保留 | 仅服务旧监控大盘语义 | 已删除，不再恢复 |

## 6. 本轮实际执行范围

本轮只执行以下范围：

1. 新建本收口 plan 文件，不覆盖旧 plan
2. 固定当前真实运行边界和数据来源
3. 明确接下来只改首页、统一 Header、详情页和必要的 i18n 文案清理

本轮不执行：

1. 不改 `8080`
2. 不扩展新页面
3. 不改后端边界
4. 不推进 `perf_metrics`

## 7. 验收方式

后续每次真正修改前端源码后，都必须按以下顺序执行并记录结果：

1. `cd frontend && npm run build`
2. `rm -rf internal/api/frontend && mkdir -p internal/api/frontend && cp -r frontend/dist internal/api/frontend/`
3. 重启本地 `18080`
4. 核对 `http://127.0.0.1:18080/`
5. 从首页点击服务商进入 `http://127.0.0.1:18080/p/:provider?...`
6. 核对 `http://127.0.0.1:18080/detect`
7. 核对真实接口：
   - `/api/audit/channels`
   - `/api/audit/targets`
   - `/api/audit/methodology`
   - `/api/audit/diagnostics/latest?include_filtered=1`

## 8. 停止条件

满足以下条件才算本计划完成：

1. 首页 `/` 只承担服务商入口职责
2. 首页只显示可用率和可用率趋势，不显示模型
3. 点击服务商后进入详情页
4. 详情页使用 `new-api` 同步渠道数据，并显示每个模型的状态
5. 页面右上角只保留“首页”和“检测方法”
6. 页面主入口中不再出现“联系我们”“平台声明”
7. 检测方法快捷入口固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt`
8. `18080` 页面结果与当前工作区代码一致
