.PHONY: dev dev-backend dev-frontend embed-frontend test lint build ci

# 一键启动前后端热重载开发环境
# 前端: Vite dev server (:5173) 带 HMR
# 后端: Air 热重载 (:8080)
# 开发时访问 http://localhost:5173
dev:
	@echo "Starting dev environment..."
	@echo "  Frontend: http://localhost:5173 (HMR)"
	@echo "  Backend:  http://localhost:8080 (API)"
	@trap 'kill 0' EXIT; \
	  $(MAKE) dev-backend & \
	  $(MAKE) dev-frontend & \
	  wait

dev-backend:
	$(HOME)/go/bin/air

dev-frontend:
	cd frontend && npm run dev

# ==================== 本地 CI 聚合命令 ====================
# Go embed 不支持符号链接：编译 internal/api 前必须把 frontend/dist 复制到
# internal/api/frontend/dist，否则 go build/vet/test 都会因 //go:embed 失败。
# test / lint / build 共同依赖本目标，make 在单次调用内只执行一次。
embed-frontend:
	cd frontend && npm ci && npm run build
	rm -rf internal/api/frontend
	mkdir -p internal/api/frontend
	cp -r frontend/dist internal/api/frontend/

# 聚合测试：根模块 + notifier 子模块 + 前端
test: embed-frontend
	go test ./...
	cd notifier && go test ./...
	cd frontend && npm run test -- --run

# 聚合静态检查：根 vet + notifier vet + 前端 lint
lint: embed-frontend
	go vet ./...
	cd notifier && go vet ./...
	cd frontend && npm run lint

# 聚合构建：前端构建（含 embed）+ Go 二进制
build: embed-frontend
	go build -o monitor ./cmd/server

# 本地模拟 CI：静态检查 + 测试 + 构建（不含 govulncheck，见 ci-release.yml）
ci: lint test build
