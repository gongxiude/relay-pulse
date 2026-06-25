# 快速部署指南 ⚡

> **3 分钟用 Docker 跑起来。深入配置见 [docs/user/config.md](docs/user/config.md)，容器细节见 [docs/user/docker.md](docs/user/docker.md)。**

## 前置要求

- Docker 20.10+
- Docker Compose v2.0+

## 3 步启动

```bash
# 1. 拉取部署文件与最小配置模板
mkdir relay-pulse && cd relay-pulse
curl -O https://raw.githubusercontent.com/prehisle/relay-pulse/main/docker-compose.yaml
mkdir -p config
curl -o config/config.yaml https://raw.githubusercontent.com/prehisle/relay-pulse/main/config.yaml.example

# 2. 编辑 config/config.yaml，把示例里的 base_url / api_key 替换成你自己的
vim config/config.yaml

# 3. 启动
docker compose up -d
```

访问：

- Web 界面：http://localhost:8080
- API：http://localhost:8080/api/status
- 健康检查：http://localhost:8080/health
- 审计工作台：http://localhost:8080/audit

## 启用 new-api 只读审计

如果你要把 RelayPulse 当成 `new-api` 的只读审计系统，启动前额外配置 3 个环境变量：

```bash
export NEWAPI_BASE_URL=https://new-api.example.com
export NEWAPI_ACCESS_TOKEN=your-token
export NEWAPI_USER_ID=1
```

审计链路说明：

- 只通过 `new-api` HTTP API 读取渠道和日志
- 不连接 `new-api` 数据库
- 不写回 `new-api`

启动后可直接验证：

```bash
curl http://localhost:8080/api/audit/newapi/sync/status
curl http://localhost:8080/api/audit/targets
curl "http://localhost:8080/api/audit/ranking?window=24h"
```

页面入口：

- `/audit`：同步状态与入口
- `/audit/ranking`：生产稳定性排名
- `/audit/targets`：自动生成的审计目标
- `/audit/submit`：提交 quick-probe 诊断
- `/audit/compare/:runId`：查看 compare 证据

## 常用命令

```bash
docker compose ps               # 查看状态
docker compose logs -f monitor  # 实时日志
docker compose restart          # 重启
docker compose pull && docker compose up -d   # 升级到最新镜像
docker compose down             # 停止（保留数据）
docker compose down -v          # 停止并删除数据卷 ⚠️ 会丢失历史数据
```

## 故障排查

| 现象 | 排查命令 |
|------|----------|
| 容器无法启动 | `docker compose logs monitor` |
| 配置不生效 | `docker compose config` 检查 YAML 语法 |
| 8080 端口被占用 | `lsof -i :8080`，或在 docker-compose.yaml 改成 `"3000:8080"` |
| 无法访问 Web | `curl http://localhost:8080/health` 确认端口映射 |

## 下一步

配置文件默认只写了 2 个示例监测项，以下能力在 `config.yaml.example` 中未展示，按需启用：

| 能力 | 参考文档 |
|------|----------|
| 用环境变量注入 API Key（生产推荐） | [docs/user/config.md](docs/user/config.md) · "环境变量覆盖" 章节 |
| 配置热更新（改文件自动重载，无需重启） | [docs/user/config.md](docs/user/config.md) · "热更新" 章节 |
| 切到 PostgreSQL 存储 | [docs/user/deploy-postgres.md](docs/user/deploy-postgres.md) |
| 资源限制 / 日志轮转 / Cloudflare 反代 | [docs/user/docker.md](docs/user/docker.md) |
| 热板 / 备板 / 冷板三层板块系统 | [docs/user/config.md](docs/user/config.md) · `boards` 章节 |
| 赞助商置顶、标签、事件通知 | [docs/user/config.md](docs/user/config.md) |
| Telegram / QQ Bot 推送 | [notifier/README.md](notifier/README.md) |

## 更多文档

- **项目入口**：[README.md](README.md)
- **配置手册**：[docs/user/config.md](docs/user/config.md)
- **监测方法论**：[docs/user/methodology.md](docs/user/methodology.md)
- **赞助体系**：[docs/user/sponsorship.md](docs/user/sponsorship.md)
- **贡献指南**：[CONTRIBUTING.md](CONTRIBUTING.md)

## 支持

- GitHub Issues：https://github.com/prehisle/relay-pulse/issues
- 仓库：https://github.com/prehisle/relay-pulse

**祝监测愉快！** 🚀
