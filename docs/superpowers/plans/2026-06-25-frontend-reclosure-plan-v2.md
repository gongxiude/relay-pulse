# RelayPulse 前端二次收口 Plan

## 1. 目标与最终产物

本计划只做一件事：把当前已经偏离的前端重新收口到用户确认过的产品语义，不扩展新功能，不改后端边界，不动 `8080` 容器基线。

最终产物必须是：

| 产物 | 说明 | 验收标准 |
|---|---|---|
| 首页 `/` 收口版 | 首页只承担服务商入口职责 | 只展示服务商列表，只保留可用率和可用率趋势，不显示模型 |
| 服务商详情页 `/p/:provider` 收口版 | 点击首页服务商进入详情页 | 页面按 `new-api` 同步通道展开，模型状态按真实同步数据展示 |
| 统一 Header | 全站只保留同一套头部 | 右上角只保留“首页”“检测方法”，检测方法快捷入口固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt` |
| 无效前端内容清理版 | 删除或下线方向错误内容 | “联系我们”“平台声明”及旧公开提交流程不再出现在入口语义中 |
| 本地验证基线 | 只用 `18080` 验收 | build、embed、重启、本地页面与接口核对全部完成 |

## 2. 当前运行边界

执行本计划前，边界固定如下：

| 项目 | 固定边界 |
|---|---|
| `8080` | Docker 容器基线，只保留不动，只用于参考 |
| `18080` | 当前工作区本地验证态，后续所有验收都以它为准 |
| 前端源码 | `frontend/src` |
| 前端构建产物 | `frontend/dist` |
| Go embed 副本 | `internal/api/frontend/dist` |
| 前端改动后固定流程 | `cd frontend && npm run build` → `rm -rf internal/api/frontend && mkdir -p internal/api/frontend && cp -r frontend/dist internal/api/frontend/` → 重启本地 Go 服务 |
| `new-api` 接入 | 只读，仅读取 `NEWAPI_ACCESS_TOKEN`、`NEWAPI_BASE_URL`、`NEWAPI_USER_ID` |
| 后端边界 | 第一版不写回 `new-api`，不推进 `perf_metrics` |

## 3. 当前真实数据来源

以下结论基于 `18080` 当前运行态接口返回：

| 数据 | 来源接口 | 当前证据 |
|---|---|---|
| 服务商 / 渠道 / 模型 / 启停状态 | `/api/audit/channels` | 当前返回 `10` 条渠道快照，字段包含 `provider/service/channel/model/enabled/channelType/channelTypeLabel/raw.Status` |
| 监控对象 | `/api/audit/targets` | 当前按 `服务商 + 渠道 + 模型` 展开为真实目标集合 |
| 方法论统计 | `/api/audit/methodology` | 当前返回 `implemented_count=10`、`active_count=10`、`active_weight=69` |
| 详情页最近检测 | `/api/audit/diagnostics/latest` | 当前用于模型级最近检测状态与失败原因提示 |
| compare 详情 | `/api/audit/compare/:run_id` | 当前用于单次检测详情页展示维度、步骤和证据 |

## 4. 页面入口与页面职责

| 页面 | 固定职责 | 禁止事项 |
|---|---|---|
| `/` | 服务商入口页 | 不再承载旧监控大盘语义，不再保留公告、收藏、截图导出、网格卡片为首页主体 |
| `/p/:provider` | 服务商详情页 | 不允许展示伪造渠道，不允许脱离 `new-api` 快照自造数据 |
| `/detect` | 方法论说明页 | 继续复用同一套 Header，不再做平行导航 |
| `/detect/compare/:runId` | 单次检测详情页 | 继续展示真实 compare 数据，不在本轮扩展新职责 |

## 5. 保留 / 删除 / 重做清单

### 5.1 保留并继续迭代

| 文件 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/router.tsx` | 定义首页、详情页、方法页、compare 页路由边界 | 保留 | 当前路由边界基本正确，没有再暴露旧联系页 | 保持边界不变，只核对入口语义 |
| `frontend/src/components/StatusTable.tsx` | 首页服务商表格主组件 | 保留 | 这是现有可复用表格，不应重新发明 | 在首页只保留服务商需要的列与跳转 |
| `frontend/src/utils/auditChannelAdapter.ts` | `new-api` 渠道快照到前端展示数据的适配层 | 保留 | 当前首页与详情页都依赖它接真实数据 | 继续作为唯一适配层，不新增平行映射 |
| `frontend/src/pages/DetectPage.tsx` | 方法论说明页 | 保留 | 已接真实方法论接口，并复用 Header | 本轮不扩展，只做必要收口 |
| `frontend/src/pages/DetectComparePage.tsx` | 单次检测详情页 | 保留 | 当前已消费真实 compare schema | 本轮不作为重点改动对象 |

### 5.2 保留但必须重做

| 文件 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/App.tsx` | 当前首页实际入口 | 保留，但必须重做 | 当前仍背着旧监控大盘语义，包含公告、收藏、筛选抽屉、网格视图、截图模式等大量非首页入口职责 | 收口为纯服务商入口页，优先复用现有表格组件 |
| `frontend/src/components/Header.tsx` | 全站 Header | 保留，但必须重做 | 当前桌面端已接近要求，但仍混入分享、统计、语言切换等旧语义，且要继续确保“首页 / 检测方法”是主导航 | 收口为统一头部，保留必要能力，去掉无关入口语义 |
| `frontend/src/pages/ProviderPage.tsx` | 服务商详情页 | 保留，但必须重做 | 当前数据方向正确，但展示结构还没有完全贴合“先看通道，再看模型状态”的产品要求 | 复用现有布局和 badge，重排结构，不再发散字段 |

### 5.3 当前已成废代码，待删除

| 文件路径 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/components/AnnouncementsBanner.tsx` | 首页公告横幅 | 不保留 | 首页产品语义已收口为服务商入口，公告横幅不再属于入口职责 | 先从 `App.tsx` 脱钩，再物理删除 |
| `frontend/src/hooks/useAnnouncements.ts` | 公告数据拉取 | 不保留 | 仅服务于首页公告横幅 | 随公告横幅一起删除 |
| `frontend/src/components/EmptyFavorites.tsx` | 收藏空状态 | 不保留 | 首页不再以收藏视图作为入口能力 | 先从 `App.tsx` 脱钩，再删除 |
| `frontend/src/hooks/useFavorites.ts` | 首页收藏逻辑 | 不保留 | 只服务旧监控大盘语义 | 评估 `Admin` 外无依赖后删除 |
| `frontend/src/components/StatusCard.tsx` | 首页网格卡片视图 | 不保留 | 当前首页入口不需要双视图体系，用户要求先收口表格入口 | 先从 `App.tsx` 脱钩，再删除 |

### 5.4 暂不删除，但要在第二步审计后决定

| 文件路径 | 当前作用 | 是否保留 | 原因 | 下一步动作 |
|---|---|---|---|---|
| `frontend/src/components/Controls.tsx` | 首页筛选 / 时间 / 板块控制条 | 待定 | 是否保留要取决于首页最终是否只保留最小筛选集；当前这块明显比目标复杂 | 先收口首页，再决定裁剪还是删除 |
| `frontend/src/components/Tooltip.tsx` | 首页热力图提示层 | 待定 | 若首页只保留可用率趋势，可能仍需趋势 hover，但不一定需要当前复杂 tooltip | 随首页最小展示一起审计 |

## 6. 本轮实际执行范围

本轮只做以下事情：

1. 固定真实边界与数据来源
2. 输出当前代码的保留 / 删除 / 重做清单
3. 用本文件作为新的前端收口基线

本轮不做以下事情：

1. 不修改后端边界
2. 不动 `8080` 容器
3. 不扩展新页面
4. 不新增一套前端组件体系

## 7. 验收方式

后续每次真正改前端代码，都必须按以下顺序执行并记录结果：

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
3. 首页不再保留“联系我们”“平台声明”以及旧监控大盘语义残留
4. 详情页使用 `new-api` 同步渠道数据，并按模型展示状态
5. 右上角只保留“首页”“检测方法”
6. 检测方法快捷入口固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt`
7. `18080` 的页面结果与当前工作区代码一致
