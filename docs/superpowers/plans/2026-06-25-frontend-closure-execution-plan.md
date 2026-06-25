# RelayPulse 前端收口执行 Plan

## 1. 目标与最终产物

本计划只处理当前前端收口，不扩展新功能，不改变后端边界，不改动 `8080` Docker 容器基线。

本轮最终产物必须是：

| 产物 | 说明 | 验收标准 |
|---|---|---|
| 首页 `/` 收口版 | 首页只承担服务商入口职责 | 只展示服务商列表，不显示模型；保留可用率与可用率趋势 |
| 服务商详情页 `/p/:provider` 收口版 | 点击首页服务商进入详情页 | 页面按 `new-api` 同步通道展开，并显示每个模型状态 |
| 检测方法入口收口版 | 统一通过同一个 `Header` 进入 | 右上角只保留“首页 / 检测方法”，检测方法固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt` |
| 旧页面清理版 | 删除已被否定的旧公开入口 | `联系我们`、`申请收录`、`申请变更` 页面及其路由不再保留 |
| 本地验证基线 | 固定使用 `18080` 做页面验收 | 前端构建、embed 同步、重启本地 Go 服务、页面核对全部完成 |

## 2. 当前运行边界

| 项目 | 固定边界 |
|---|---|
| `8080` | Docker 容器基线，只保留，不改动，只用于参考 |
| `18080` | 当前工作区本地验证态，后续所有页面验收以它为准 |
| 前端源码 | `frontend/src` |
| 前端构建产物 | `frontend/dist` |
| Go embed 副本 | `internal/api/frontend/dist` |
| embed 规则 | 只要前端有改动，必须执行 `npm run build -> copy embed -> 重启 18080` |
| `new-api` 接入 | 只读，仅读取 `NEWAPI_ACCESS_TOKEN`、`NEWAPI_BASE_URL`、`NEWAPI_USER_ID` |
| 后端边界 | 不推进 `perf_metrics`，不自动修改 `new-api` |

## 3. 当前真实数据来源

| 数据 | 当前来源 | 当前证据 |
|---|---|---|
| 服务商 / 渠道 / 模型 / 启停状态 | `/api/audit/channels` | `18080` 返回 `provider/service/channel/model/enabled/channelType/channelTypeLabel/raw.Status` |
| 监控对象 | `/api/audit/targets` | `18080` 返回 `provider + service + channel + model + request_model + enabled` |
| 首页可用率与趋势 | 现有 `useMonitorData` + `adaptAuditChannelsToMonitorData` | 首页当前已经基于 `StatusTable` 展示服务商列表 |
| 详情页模型状态 | `useAuditChannels` + `useAuditDiagnosticLatest` + `useAuditSyncStatus` | 当前 `ProviderPage.tsx` 已接入真实同步数据和最新诊断 |
| 检测方法数据 | `/api/audit/methodology` | 当前 `DetectPage.tsx` 已消费真实方法论接口 |
| compare 详情 | `/api/audit/compare/:run_id` | 当前 `DetectComparePage.tsx` 已消费真实 compare schema |

## 4. 页面入口与页面职责

| 页面 | 固定职责 | 禁止事项 |
|---|---|---|
| `/` | 服务商入口页 | 不再承载独立审计站首页语义，不增加新入口产品文案 |
| `/p/:provider` | 服务商详情页 | 不再展示伪造渠道；必须以 `new-api` 同步渠道为准 |
| `/detect` | 检测方法页 | 不单独做新 `Header`，不新增平行导航体系 |
| `/detect/compare/:runId` | 单次诊断详情页 | 继续展示真实 compare 维度、步骤、证据 |

## 5. 保留 / 删除 / 重做清单

### 5.1 保留并继续迭代

| 文件 | 当前作用 | 原因 | 下一步动作 |
|---|---|---|---|
| `frontend/src/App.tsx` | 首页 `/` 入口页 | 当前已做到首页不显示模型列，方向正确 | 继续收口首页尾部和非入口语义残留 |
| `frontend/src/components/StatusTable.tsx` | 首页服务商表格 | 可复用现有表格组件，不重写表格体系 | 保持复用，只按现有 props 控制展示 |
| `frontend/src/components/Header.tsx` | 全站头部导航 | 当前已收口到“首页 / 检测方法”双入口 | 保持共用，校验高亮和详情页表现 |
| `frontend/src/pages/ProviderPage.tsx` | 服务商详情页 | 当前已接入真实 `new-api` 同步数据与最新诊断 | 继续收口布局和字段组织，不改数据方向 |
| `frontend/src/pages/DetectPage.tsx` | 检测方法页 | 当前已复用 `Header` 且接入真实方法论接口 | 保持页面存在，只做展示收口 |
| `frontend/src/router.tsx` | 路由边界 | 当前已去掉旧联系/申请路由暴露 | 保持当前边界稳定 |
| `frontend/src/pages/DetectComparePage.tsx` | compare 详情页 | 当前消费真实 compare schema，方向正确 | 不作为本轮重点改动对象 |

### 5.2 删除

| 文件 | 当前作用 | 删除原因 | 下一步动作 |
|---|---|---|---|
| `frontend/src/pages/ContactPage.tsx` | 旧“联系我们”页面 | 已脱离路由，且产品语义被明确否定 | 物理删除文件 |
| `frontend/src/pages/OnboardingPage.tsx` | 旧“申请收录”页面 | 已脱离路由，属于旧公开提交流程 | 物理删除文件 |
| `frontend/src/pages/ChangeRequestPage.tsx` | 旧“申请变更”页面 | 已脱离路由，属于旧公开提交流程 | 物理删除文件 |

### 5.3 删除前先审计依赖

| 文件/目录 | 当前作用 | 原因 | 下一步动作 |
|---|---|---|---|
| `frontend/src/components/onboarding/*` | Onboarding / ChangeRequest 复用组件 | 很可能会随旧页面删除而变成废代码，但要先确认是否还有其他依赖 | 删除旧页面时做依赖审计，再决定是否一并删除 |

### 5.4 重做 / 收口

| 文件 | 当前作用 | 重做原因 | 下一步动作 |
|---|---|---|---|
| `frontend/src/components/Footer.tsx` | 页面底部信息 | 当前首页不该再挂载这类尾部内容；`App.tsx` 注释里还把它视为“免责声明”残留 | 先从首页移除，再判断其他页面是否保留 |
| `frontend/src/App.tsx` | 首页入口页 | 首页仍有非入口语义残留 | 调整首页末尾内容，保留服务商入口语义 |
| `frontend/src/pages/ProviderPage.tsx` | 服务商详情页 | 当前数据方向对，但展示结构还需进一步贴近确认后的详情页语义 | 在现有组件基础上继续收口 |

## 6. 本轮实际执行范围

本轮只执行以下范围：

1. 创建新的前端收口执行 plan 文件
2. 基于当前代码状态清理旧公开页面和首页残留内容
3. 在不改后端边界的前提下，继续收口首页和服务商详情页
4. 每次前端改动后，严格执行构建、embed 同步、重启 `18080`、页面核验

本轮不执行：

1. 不改 `8080` Docker 容器
2. 不扩展新页面
3. 不推进 `perf_metrics`
4. 不修改 `new-api`

## 7. 验收方式

每次前端修改后必须按以下顺序执行并记录结果：

1. `cd frontend && npm run build`
2. `rm -rf internal/api/frontend && mkdir -p internal/api/frontend && cp -r frontend/dist internal/api/frontend/`
3. 重启本地 `18080`
4. 核对 `http://127.0.0.1:18080/`
5. 从首页点击服务商进入详情页，核对 `http://127.0.0.1:18080/p/:provider?...`
6. 核对 `http://127.0.0.1:18080/detect`
7. 核对接口真实数据：
   - `/api/audit/channels`
   - `/api/audit/targets`
   - `/api/audit/methodology`
   - `/api/audit/diagnostics/latest?include_filtered=1`

## 8. 停止条件

满足以下条件才算本计划完成：

1. 首页 `/` 只承担服务商入口职责
2. 首页只显示可用率和可用率趋势，不显示模型
3. 首页不再显示“联系我们”与“平台声明”语义残留
4. 详情页使用 `new-api` 同步渠道数据，并展示每个模型状态
5. 页面右上角只保留“首页 / 检测方法”
6. 检测方法入口固定指向 `/p/claudecn-gpt?service=cc&channel=78%3AClaudeCN-gpt`
7. 旧公开页面及其无效入口被清理
8. `18080` 页面结果与当前工作区代码一致
