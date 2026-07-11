FROM golang:1.26.5-alpine

WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
# VERSION 走上游基线版本号（VERSION 文件）；BUILD 为自研 CalVer（git commit date + short SHA），
# 同 backend/Makefile / 根 Dockerfile 的口径。.git 被 .dockerignore 排除时 BUILD 回退为 dev。
RUN VERSION_VALUE="$(./scripts/resolve-version.sh)" && \
    BUILD_VALUE="$(git log -1 --format=%cd --date=format:'%Y.%m.%d' 2>/dev/null)-$(git rev-parse --short=8 HEAD 2>/dev/null)" && \
    if [ -z "${BUILD_VALUE}" ] || [ "${BUILD_VALUE}" = "-" ]; then BUILD_VALUE="dev"; fi && \
    go build -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Build=${BUILD_VALUE}" -o main ./cmd/server/

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./main"]
