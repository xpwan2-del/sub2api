#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。
#
# 自研发布版本号 Build（CalVer: YYYY.MM.DD-shortsha）会自动从 git 推导并注入镜像，
# 推导逻辑与 Makefile / .goreleaser.yaml 一致（同一 commit 在任何机器结果一致，可复现）。
# 镜像内 main.Build 因此为真实 CalVer，而非兜底的 "dev"。
#
# 版本 tag 体系统一口径（镜像 tag == main.Build == 面板主版本号 == OCI label
# org.opencontainers.image.version）：
#   <BUILD>  不可变版本 tag，无条件打（--push 时无条件推，Harbor 版本留痕）
#   latest   可移动分发别名（部署侧 ./ops upgrade 拉取的就是它）
#
# 默认仅本地构建；传 --push 时构建完成后才打 Harbor 标签并推送。
#
# 用法:
#   ./deploy/build_image.sh [--push]
#
#   --push      构建完成后打 Harbor 标签并推送。
#   -h, --help  显示帮助。
#
# 环境变量覆盖（可选）:
#   BUILD       指定自研版本号；未设则从 git 自动推导（.git 需存在）。
#   IMAGE_NAME  镜像名，默认 topai/sub2api。
#   IMAGE_TAG   分发别名 tag，默认 latest（部署侧 upgrade 拉取的就是它）。
#               无论设为何值，脚本都会额外打并推送不可变版本 tag <BUILD>。
#   COMMIT/DATE 可选覆盖 revision/created label 来源；默认取 HEAD 与当前 UTC 时间。
#   HARBOR_REGISTRY  Harbor 仓库地址，默认 harbor.equa-data.com:8077。

set -euo pipefail

usage() {
  cat <<'EOF'
用法: ./deploy/build_image.sh [--push]

参数:
  --push      构建完成后打 Harbor 标签并推送（默认不推送）。
  -h, --help  显示本帮助。

环境变量覆盖（可选）:
  BUILD            指定自研版本号；未设则从 git 自动推导（.git 需存在）。
  IMAGE_NAME       镜像名，默认 topai/sub2api。
  IMAGE_TAG        分发别名 tag，默认 latest（部署侧 upgrade 拉取的就是它）；
                   无论设为何值，都会额外打并推送不可变版本 tag <BUILD>。
  COMMIT / DATE    可选覆盖 revision/created label 来源；默认取 HEAD 与当前 UTC 时间。
  HARBOR_REGISTRY  Harbor 仓库地址，默认 harbor.equa-data.com:8077。
EOF
}

# --- 参数解析 ----------------------------------------------------------------
# 默认仅本地构建；--push 时构建后打 Harbor 标签并推送。
PUSH=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --push) PUSH=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "!!> 未知参数: $1（见 --help）" >&2; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- 自研发布版本号 Build（CalVer）-------------------------------------------
# 优先用环境变量 BUILD；否则从 git commit date + short SHA 自动推导。
if [[ -z "${BUILD:-}" ]]; then
  GIT_DATE="$(git -C "${REPO_ROOT}" log -1 --format=%cd --date=format:'%Y.%m.%d')"
  GIT_SHORT="$(git -C "${REPO_ROOT}" rev-parse --short=8 HEAD)"
  BUILD="${GIT_DATE}-${GIT_SHORT}"
fi
# label 来源（允许像 BUILD 一样被环境变量覆盖；BUILD 外部指定时 COMMIT 取当前
# HEAD 为 best-effort）
BUILD_COMMIT="${COMMIT:-$(git -C "${REPO_ROOT}" rev-parse HEAD)}"
BUILD_DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

IMAGE_NAME="${IMAGE_NAME:-topai/sub2api}"
IMAGE_TAG="${IMAGE_TAG:-latest}" # 分发别名（部署侧 upgrade 拉取的就是它）
# --- 推送 Harbor -------------------------------------------------------------
HARBOR_REGISTRY="${HARBOR_REGISTRY:-harbor.equa-data.com:8077}"
HARBOR_VERSION_IMAGE="${HARBOR_REGISTRY}/${IMAGE_NAME}:${BUILD}"   # 不可变版本 tag
HARBOR_ALIAS_IMAGE="${HARBOR_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}" # 分发别名
# 前端 npm/pnpm/corepack 镜像源，默认淘宝镜像（与 deploy/docker-compose.dev.yml 一致）。
# corepack 下载 pnpm 本体也走此源（Dockerfile 内映射为 COREPACK_NPM_REGISTRY）。
# 想直连官方源：NPM_REGISTRY=https://registry.npmjs.org ./deploy/build_image.sh
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

echo "==> 自研版本号 Build: ${BUILD}"
echo "==> 构建镜像: ${IMAGE_NAME}:${BUILD} (版本 tag) + ${IMAGE_NAME}:${IMAGE_TAG} (分发别名)"
echo "==> 前端镜像源: ${NPM_REGISTRY}"
if [[ "${PUSH}" == "1" ]]; then
  echo "==> 推送目标: ${HARBOR_VERSION_IMAGE} + ${HARBOR_ALIAS_IMAGE}"
else
  echo "==> 跳过推送（默认仅构建，--push 启用推送）"
fi

docker build --no-cache \
  -t "${IMAGE_NAME}:${BUILD}" \
  -t "${IMAGE_NAME}:${IMAGE_TAG}" \
  --build-arg BUILD="${BUILD}" \
  --build-arg COMMIT="${BUILD_COMMIT}" \
  --build-arg DATE="${BUILD_DATE}" \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  --build-arg GOSUMDB=sum.golang.google.cn \
  --build-arg NPM_CONFIG_REGISTRY="${NPM_REGISTRY}" \
  -f "${REPO_ROOT}/Dockerfile" \
  "${REPO_ROOT}"

echo "==> 构建 完成: ${IMAGE_NAME}:${BUILD} / ${IMAGE_NAME}:${IMAGE_TAG} (Build: ${BUILD})"

# --- 推送 Harbor -------------------------------------------------------------
if [[ "${PUSH}" == "1" ]]; then
  # 版本 tag 与分发别名各推一份（两者同一 image，第二个推送 blob 全复用，
  # 仅多一次 manifest 上传）；IMAGE_TAG 显式设为 <BUILD> 时去重。
  targets=("${HARBOR_VERSION_IMAGE}" "${HARBOR_ALIAS_IMAGE}")
  if [[ "${HARBOR_ALIAS_IMAGE}" == "${HARBOR_VERSION_IMAGE}" ]]; then
    targets=("${HARBOR_VERSION_IMAGE}")
  fi
  for target in "${targets[@]}"; do
    echo "==> 打标签: ${IMAGE_NAME}:${BUILD} -> ${target}"
    docker tag "${IMAGE_NAME}:${BUILD}" "${target}"

    echo "==> 推送: ${target}"
    if ! docker push "${target}"; then
      echo "!!> 推送失败：如未登录请先执行 docker login ${HARBOR_REGISTRY}" >&2
      exit 1
    fi
    echo "==> 推送 完成: ${target}"
  done
fi
