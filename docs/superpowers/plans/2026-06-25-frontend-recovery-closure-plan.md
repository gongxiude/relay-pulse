# RelayPulse 前端收口实施 Plan

## 1. 目标与最终产物

本计划只解决当前前端语义跑偏的问题：在不改动 `new-api`、不改动 `8080` 容器基线的前提下，把本地工作区前端收口到用户已经确认的页面职责。

最终产物必须是：

| 产物 | 说明 | 验收标准 |
|---|---|---|
| 首页 `/` 收口版 | 首页只承担服务商入口职责 | 页面展示服务商列表，仅保留可用率和可用率趋势，不显示模型 |
| 服务商详情页 `/p/:provider` 收口版 | 点击首页服务商进入详情页 | 页面按通道/模型展示真实同步数据，状态来自 `new-api` |
| 检测方法页收口版 | 复用现有头部与现有审计数据能力 | 顶部使用同一 `Header`，入口保留“首页 / 检测方法” |
| 删除清单 | 清除方向错误或无效前端实现 | 不再保留错误入口语义、无效导航和误导性静态块 |
| 验证基线 | 固定 18080 为唯一本地验证口 | 前端 build、embed 同步、本地服务启动、页面核对全部通过 |

## 2. 当前运行时边界

| 项目 | 固定边界 |
|---|---|
| `8080` | Docker 容器基线，只保留，不改动，只用于参考 |
| `18080` | 当前工作区本地验证态，后续所有页面验收以它为准 |
| 前端源码 | `frontend/src` |
| 前端构建产物 | `frontend/dist` |
| Go embed 副本 | `internal/api/frontend/dist` |
| embed 规则 | 任何前端改动后，必须 build -> copy embed -> 重启 18080 |
| `new-api` 接入 | 只读，仅读取 `NEWAPI_BASE_URL`、`NEWAPI_ACCESS_TOKEN`、`NEWAPI_USER_ID` |

## 3. 当前真实数据来源

| 数据 | 当前来源 | 说明 |
|---|---|---|
| 服务商/渠道/模型/启停状态 | `/api/audit/channels`、`/api/audit/targets` | 来自 `new-api` 同步，不允许前端伪造 |
| 首页可用率与趋势 | 现有监控聚合接口 + `auditChannels` 适配层 | 首页只展示服务商入口所需的聚合数据 |
| 详情页模型状态 | `new-api` 同步快照 + latest diagnostics | 必须能区分启用/停用、最近诊断状态、失败原因 |
| 检测方法数据 | `/api/audit/methodology`、`/api/audit/ranking` | 当前先展示已实现的真实审计数据 |
| compare 详情 | `/api/audit/compare/:run_id` | 继续复用当前 compare schema 和 evidence |

## 4. 页面入口与页面职责

| 页面 | 固定职责 | 禁止事项 |
|---|---|---|
| `/` | 服务商入口页 | 不再改造成独立审计站首页，不增加新的产品语义 |
| `/p/:provider` | 服务商详情页 | 不再显示伪造渠道；必须以真实同步渠道为准 |
| `/detect` | 检测方法页 | 不单独做新 header，不引入新导航体系 |
| `/detect/compare/:runId` | 单次诊断详情页 | 继续展示 compare 维度、步骤、证据 |

## 5. 保留 / 删除 / 重做范围

### 5.1 保留

| 文件/能力 | 保留原因 |
|---|---|
| `frontend/src/components/Header.tsx` | 已经具备“首页 / 检测方法”双入口，可继续复用 |
| `frontend/src/components/StatusTable.tsx` | 首页表格基础组件已存在，应继续复用而不是重写 |
| `frontend/src/pages/ProviderPage.tsx` 的真实数据接入方向 | 已接入 `useAuditChannels`、`useAuditDiagnosticLatest`、`useAuditSyncStatus` |
| `frontend/src/pages/DetectComparePage.tsx` 的 compare/evidence 展示 | 已消费真实 compare schema，方向正确 |
| 后端 compare/methodology 结构化接口 | 当前未提交后端实现解决的是数据真实性问题，不属于错误方向 |

### 5.2 删除

| 文件/能力 | 删除原因 |
|---|---|
| 首页中的“平台声明”块 | 用户明确要求删除 |
| Header / 路由中的“联系我们”入口语义 | 用户明确要求删除 |
| 与首页入口语义冲突的独立审计入口思路 | 用户已否定，不再保留为产品方向 |

### 5.3 重做

| 项目 | 重做原因 |
|---|---|
| 首页展示口径 | 当前虽然已经隐藏模型，但仍保留过多旧站元素，需要进一步收口到“服务商入口” |
| 服务商详情页布局 | 当前数据方向正确，但视觉和信息组织还未对齐用户要求的详情页格式 |
| 检测方法页入口联动 | 需要固定复用同一 Header，并把目标链接与入口语义统一 |

## 6. 实施步骤

### Phase 1：先做文件级分拣

输出未提交改动的保留/删除/重做清单，逐文件说明：
- 当前作用
- 是否保留
- 原因
- 下一步动作

### Phase 2：收口首页 `/`

目标：
- 继续复用 `App.tsx + StatusTable`
- 首页只保留服务商入口所需信息
- 仅显示可用率和可用率趋势
- 移除“平台声明”及相关静态占位

涉及文件：
- `frontend/src/App.tsx`
- `frontend/src/components/Header.tsx`
- `frontend/src/components/Footer.tsx`
- 如有必要，补充现有 i18n 文案，但不新增平行页面

### Phase 3：收口服务商详情页 `/p/:provider`

目标：
- 渠道必须来自 `new-api` 同步快照
- 详情页显示每个模型的状态
- 当前状态显示 `new-api` 启停状态
- 最近一次诊断状态/失败原因继续展示
- 优先复用现有详情组件与表格样式，不再重造表格体系

涉及文件：
- `frontend/src/pages/ProviderPage.tsx`
- 仅在必要时补充现有适配工具

### Phase 4：收口检测方法页 `/detect`

目标：
- 复用当前 `Header`
- 右上角仅保留“首页 / 检测方法”
- 维持当前真实 methodology / ranking 数据消费
- 检测方法相关跳转统一到既定入口

涉及文件：
- `frontend/src/pages/DetectPage.tsx`
- `frontend/src/components/Header.tsx`
- `frontend/src/router.tsx`

## 7. 验收方式

每次前端修改后必须按以下顺序验证：

1. `cd frontend && npm run build`
2. `rm -rf internal/api/frontend && mkdir -p internal/api/frontend && cp -r frontend/dist internal/api/frontend/`
3. 重启本地 `18080`
4. 核对 `http://127.0.0.1:18080/`
5. 核对 `http://127.0.0.1:18080/p/:provider?...`
6. 核对 `http://127.0.0.1:18080/detect`
7. 核对接口真实数据：
   - `/api/audit/channels`
   - `/api/audit/targets`
   - `/api/audit/methodology`
   - `/api/audit/diagnostics/latest?include_filtered=1`

## 8. 停止条件

满足以下条件才算本计划完成：

1. 首页 `/` 只承担服务商入口职责
2. 首页不再显示模型，不再显示“平台声明”
3. 详情页渠道与状态全部来自 `new-api` 同步数据
4. 详情页能展示每个模型状态
5. Header 只保留“首页 / 检测方法”快捷入口
6. `18080` 页面结果与当前工作区代码一致
7. 验证步骤全部通过并有明确证据
