#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。
#
# 自研发布版本号 Build（CalVer: YYYY.MM.DD-shortsha）会自动从 git 推导并注入镜像，
# 推导逻辑与 Makefile / .goreleaser.yaml 一致（同一 commit 在任何机器结果一致，可复现）。
# 镜像内 main.Build 因此为真实 CalVer，而非兜底的 "dev"。
#
# 环境变量覆盖（可选）:
#   BUILD       指定自研版本号；未设则从 git 自动推导（.git 需存在）。
#   IMAGE_NAME  镜像名，默认 topai/sub2api。
#   IMAGE_TAG   镜像 tag，默认 latest。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- 自研发布版本号 Build（CalVer）-------------------------------------------
# 优先用环境变量 BUILD；否则从 git commit date + short SHA 自动推导。
if [[ -z "${BUILD:-}" ]]; then
  GIT_DATE="$(git -C "${REPO_ROOT}" log -1 --format=%cd --date=format:'%Y.%m.%d')"
  GIT_SHORT="$(git -C "${REPO_ROOT}" rev-parse --short=8 HEAD)"
  BUILD="${GIT_DATE}-${GIT_SHORT}"
fi

IMAGE_NAME="${IMAGE_NAME:-topai/sub2api}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
# 前端 npm/pnpm/corepack 镜像源，默认淘宝镜像（与 deploy/docker-compose.dev.yml 一致）。
# corepack 下载 pnpm 本体也走此源（Dockerfile 内映射为 COREPACK_NPM_REGISTRY）。
# 想直连官方源：NPM_REGISTRY=https://registry.npmjs.org ./deploy/build_image.sh
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

echo "==> 自研版本号 Build: ${BUILD}"
echo "==> 构建镜像: ${IMAGE_NAME}:${IMAGE_TAG}"
echo "==> 前端镜像源: ${NPM_REGISTRY}"

docker build --no-cache -t "${IMAGE_NAME}:${IMAGE_TAG}" \
  --build-arg BUILD="${BUILD}" \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  --build-arg GOSUMDB=sum.golang.google.cn \
  --build-arg NPM_CONFIG_REGISTRY="${NPM_REGISTRY}" \
  -f "${REPO_ROOT}/Dockerfile" \
  "${REPO_ROOT}"

echo "==> 完成: ${IMAGE_NAME}:${IMAGE_TAG} (Build: ${BUILD})"
