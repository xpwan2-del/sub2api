.PHONY: build build-backend build-frontend build-datamanagementd docker-build test test-backend test-frontend test-frontend-critical test-datamanagementd secret-scan dev-backend dev-frontend

FRONTEND_CRITICAL_VITEST := \
	src/i18n/__tests__/localeKeyCompleteness.spec.ts \
	src/api/__tests__/client.spec.ts \
	src/api/__tests__/tokenRefresh.spec.ts \
	src/api/__tests__/channelMonitorV2.spec.ts \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/views/user/__tests__/ChannelStatusView.mode.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts \
	src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorZoom.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 自研发布版本号（CalVer: YYYY.MM.DD-shortsha），与 backend/Makefile 推导逻辑一致，
# 同一 commit 在任何机器构建结果一致（可复现）。
GIT_DATE   := $(shell git log -1 --format=%cd --date=format:'%Y.%m.%d')
GIT_SHORT  := $(shell git rev-parse --short=8 HEAD)
GIT_COMMIT := $(shell git rev-parse HEAD)
BUILD      ?= $(GIT_DATE)-$(GIT_SHORT)

# 构建本地 Docker 镜像。双 tag：sub2api:$(BUILD) 为不可变版本 tag（与面板主版本号、
# 镜像 label org.opencontainers.image.version 一致），sub2api:local 为开发别名。
# 自动把 BUILD/COMMIT/DATE 通过 --build-arg 注入根目录 Dockerfile（版本进 main.Build
# 与 OCI label）；.git 被 .dockerignore 排除，无法容器内推导，必须在此算好传入。
docker-build:
	docker build --build-arg BUILD=$(BUILD) \
	  --build-arg COMMIT=$(GIT_COMMIT) \
	  --build-arg DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) \
	  -t sub2api:$(BUILD) -t sub2api:local .

# 编译 datamanagementd（宿主机数据管理进程）
build-datamanagementd:
	@cd datamanagement && go build -o datamanagementd ./cmd/datamanagementd

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)

test-datamanagementd:
	@cd datamanagement && go test ./...

# 本地开发：快速启动后端
dev-backend:
	@cd backend && go run ./cmd/server/

# 本地开发：启动前端热重载
dev-frontend:
	@cd frontend && pnpm run dev

secret-scan:
	@python3 tools/secret_scan.py
