# TOP-AI Infinite Canvas Integration Plan

## Goal

把 Infinite Canvas 作为独立服务接入 TOP-AI。TOP-AI 负责账号、登录、余额、充值、计费、模型权限和网关调用；Infinite Canvas 负责画布交互、素材、作品数据和生成体验。

必须一次性修完当前链路问题，不做半截可用版本。

## Execution Mandate

本任务必须按本文档一次性完成当前问题闭环。

- 不允许乱堆代码。
- 不允许乱放文件。
- 不允许改变 `sub2api` 和 `infinite-canvas` 现有项目结构。
- 不允许为了省事新建没有项目先例的顶层目录。
- 不允许把业务逻辑塞进页面组件、路由文件或临时脚本里。
- 不允许把两个项目的源码互相搬运、复制或混合。
- 必须按各自项目现有代码规则、目录分层和命名风格开发。
- 必须同时修完模型来源、模型去重、模型广场边界、画布模型选择、视频网关和 R2 结果链路。
- 本地验证通过后，生产环境仍必须按最终验收清单逐项确认。
- 没有完成最终验收清单前，不算生产完成。

## Completed. Do Not Rework

- [x] `sub2api` 和 `infinite-canvas` 保持平级独立项目。
- [x] 入口路径使用 `/apps/canvas`，不使用 `/canvas`。
- [x] 不 iframe 嵌入，不把画布源码塞进 `sub2api/frontend`。
- [x] TOP-AI 首页和控制台已有画布入口。
- [x] Infinite Canvas 已支持 `CANVAS_BASE_PATH=/apps/canvas`。
- [x] sub2api 已有 TOP-AI session 校验接口：`GET /api/v1/app/canvas/session`。
- [x] Infinite Canvas 已有 TOP-AI 登录态换画布 token：`POST /api/auth/top-ai/session`。
- [x] 画布未登录时跳回 TOP-AI 登录页。
- [x] `CANVAS_DISABLE_LOCAL_AUTH=true` 已接入。
- [x] `CANVAS_DISABLE_LOCAL_CREDITS=true` 已接入。
- [x] `CANVAS_FORCE_TOP_AI_GATEWAY=true` 已接入。
- [x] 画布 AI 请求服务端转发 TOP-AI Gateway，不把 `TOP_AI_GATEWAY_API_KEY` 下发浏览器。
- [x] AI 请求已带 `X-Canvas-Source`、`X-Top-AI-User-ID`、`X-Client-Request-ID` 和 `metadata.source=canvas`。
- [x] R2 bucket `canvas` 已创建，公开域名 `https://media.888tech.club` 已配置。
- [x] R2 生命周期已配置：`temp/upload/` 1 天、`temp/reference/` 7 天、`failed/` 1 天、`generated/` 30 天。
- [x] 参考素材上传已支持 `MEDIA_STORAGE_DRIVER=local/r2`。
- [x] `webdav-proxy` 已做鉴权、内网阻断、host allowlist 和请求体限制。
- [x] sub2api Caddyfile 已有 `/apps/canvas*` 反代示例。

以上内容只做回归验证，不重复重构。

## Current Status And Remaining Validation

本地代码闭环状态：

- [x] 画布模型来源已改为受保护的 TOP-AI `/v1/models`，不再依赖模型广场 catalog。
- [x] 画布后端已按模型 id 做大小写不敏感去重，并返回 text/image/video/audio 分类。
- [x] 模型广场边界保持独立，只做公开商品展示，不作为画布权限来源。
- [x] `/apps/canvas/admin/settings` 在 TOP-AI 托管模式下不再展示本地模型、Key、余额配置入口。
- [x] sub2api 已补 `POST /v1/videos`、`GET /v1/videos/:id`、`GET /v1/videos/:id/content`，并挂到现有网关 routes/handler/service。
- [x] 参考素材继续走 R2 `temp/reference/`。
- [x] 本地代码已实现生成视频内容经过画布后端保存到 R2 `generated/`，前端优先使用返回的 R2 URL。
- [x] 生成视频内容已改为临时文件中转，不再整段读入内存。
- [x] 模型广场没有公开商品数据时返回空列表，不再 fallback 暴露网关真实模型列表。
- [x] 模型广场已补回归测试：空数据返回空、重复模型去重、优先保留更明确/更低价格。
- [ ] 远程环境变量和真实 10 秒视频生成需要上线后验收。

本轮本地验证结果：

- [x] `sub2api` public model catalog 定向测试通过。
- [x] `sub2api/frontend` production build 通过。
- [x] `infinite-canvas` Go tests 通过。
- [x] `infinite-canvas/web` format check 通过。
- [x] `infinite-canvas/web` production build 通过。
- [x] 两个项目 `git diff --check` 通过。

仍未做的验证：

- [ ] 生产环境变量配置后，登录态、模型列表、10 秒视频生成、R2 保存和余额扣费需要端到端验收。

### Production Reality Check - 2026-06-17

生产实测结论：代码能力已经合入，但生产配置没有完全按本文档执行，所以不能算生产完成。

已确认可用：

- `/apps/canvas`、`/apps/canvas/image`、`/apps/canvas/video` 页面返回 200。
- 普通用户 `xpwan1@gmail.com` 后端登录接口可用，余额正常。
- 画布编辑页可打开，文本节点、撤销、配置弹窗、Agent 面板、素材面板可以渲染。
- 画布配置弹窗能读取 TOP-AI 网关模型，当前可见 19 个模型。
- 图片生成请求可通过 TOP-AI Gateway，`gpt-image-2` 返回 200。
- 视频任务创建和轮询可通过 TOP-AI Gateway，`grok-imagine-video` 返回 200。
- `https://media.888tech.club` 可访问 R2 旧视频文件，R2 公共域名本身可用。

未通过：

- 生产 `TOP_AI_SESSION_URL` 当前配置成 `http://sub2api:8080/api/v1/auth/me`，与本文档要求的专用接口不一致。
- 正确配置必须是 `http://sub2api:8080/api/v1/app/canvas/session`。
- 当前错误配置会导致 Canvas SSO 可能拿到 TOP-AI admin 登录态，触发 `请使用普通用户账号进入画布`，影响添加素材和会话刷新。
- 生产未配置 `GENERATED_MEDIA_ALLOWED_HOSTS`。
- 上游视频返回 `https://vidgen.x.ai/...mp4` 后，被画布安全策略拦截：`blocked generated media import`。
- 因此本次真实视频没有保存到 R2 `generated/`。
- 图片生成成功后，“添加到素材”失败，浏览器报 `TypeError: Failed to fetch`，服务器日志出现 `请使用普通用户账号进入画布`。
- R2 对象列表未新增，说明本轮图片和视频都没有成功落到 R2。

当前判断：

- 这不是 R2 域名不可用，也不是视频模型完全不可用。
- 这是生产 SSO 配置和生成媒体白名单没有按计划文件补齐。
- 安全限制本身是正确的，但缺少生产白名单会把正常上游视频也拦掉。

### 1. Canvas Model Source

已废弃的错误链路：

```text
Infinite Canvas -> /api/v1/public/models/catalog -> 模型广场
```

当前状态：

- TOP-AI `/v1/models` 能返回 19 个真实可用模型。
- 画布后端读取受保护的 TOP-AI `/v1/models`。
- 模型广场 `/api/v1/public/models/catalog` 只展示公开商品数据。
- 模型广场为空时返回空列表，不影响画布。

正确链路：

```text
Infinite Canvas 后端 -> TOP-AI /v1/models
Authorization: Bearer TOP_AI_GATEWAY_API_KEY
```

画布不能依赖模型广场接口。

### 2. Model Deduplication Is Required

画布模型列表必须在后端统一去重：

- 按模型 `id` 去重。
- 大小写不敏感。
- 去掉空模型和无 `id` 数据。
- 保留第一次出现的原始模型名。
- 分类为 text/image/video/audio 后再次去重。
- 前端只做防御性兜底，不把去重主逻辑放页面组件里。

### 3. Model Marketplace Must Stay Separate

模型广场只负责公开渲染商品数据，不负责画布模型权限。

模型广场可以展示：

- 模型名
- 平台
- 能力标签
- 计费类型
- 美元价格
- 简短介绍

模型广场不能暴露：

- API Key
- 上游账号
- 上游 base_url
- 账号 ID
- 渠道 ID
- 分组 ID
- 路由规则
- 代理配置
- 并发、余额、健康状态
- 任何真实调用接口

模型广场接口必须是只读公开数据：

```text
GET /api/v1/public/models/catalog
```

它不能触发上游测试，不能消耗模型额度，不能给浏览器调用模型的能力。

### 4. Canvas Admin Settings Is Misleading

`/apps/canvas/admin/settings` 是 Infinite Canvas 原项目的本地后台设置页。TOP-AI 集成后，普通客户不应该通过这里配置模型、Key 或余额。

必须处理：

- 普通客户不能把它当成模型配置入口。
- `CANVAS_FORCE_TOP_AI_GATEWAY=true` 时，本地渠道配置必须隐藏或明确显示“由 TOP-AI 统一管理”。
- TOP-AI admin 是否映射为画布 admin 要单独控制，不能影响普通客户流程。

### 5. Video Gateway Status

本地代码已接入 TOP-AI 网关 `/v1/videos` 链路。真实可用性仍取决于生产模型账号、网关配置、余额扣费和上游视频任务状态。

已接入接口：

```text
POST /v1/videos
GET /v1/videos/:id
GET /v1/videos/:id/content
```

这些接口必须继续走现有 TOP-AI 网关能力：API Key 校验、分组模型权限、账号选择、失败切换、余额扣费、usage log、`source=canvas`。

生产仍需验收：

- 真实 `grok-imagine-video` 或等价视频模型能创建 10 秒视频任务。
- 查询任务状态能拿到完成结果。
- 内容下载能保存到 R2 `generated/`。
- TOP-AI 余额和 usage log 与 `source=canvas` 正常记录。

### 6. Video Media Must Use R2

视频和大素材不能长期压业务服务器本地磁盘。

生产链路：

```text
参考素材 -> R2 temp/reference/
生成结果 -> R2 generated/
画布请求只传 URL
TOP-AI 记录用量和 source=canvas
```

不能靠单纯调大 Nginx/Caddy 超时和 body size 当最终方案。

## Required Final Architecture

```text
TOP-AI 登录
  -> /apps/canvas
  -> Infinite Canvas 校验 TOP-AI 登录态
  -> Canvas 后端读取 TOP-AI /v1/models
  -> Canvas 前端展示去重后的模型
  -> 用户选择 grok-imagine-video
  -> Canvas 后端调用 TOP-AI /v1/videos
  -> TOP-AI 扣费并记录 source=canvas
  -> 结果写入 R2 generated/
```

## Model Packaging And DeepSeek Resolver Note

模型包装能力已经存在，不属于本轮未完成项。

当前已确认：

- 账号级 `model_mapping` 支持把用户看到的 A 模型包装成实际上游 B 模型。
- 渠道级 `ModelMapping` 支持按渠道/分组做模型映射。
- 网关请求会按映射结果替换请求体里的 `model`。
- 用量日志保留 `requested_model`、`upstream_model`、`model_mapping_chain` 等链路字段。

不要把上面的包装能力和 `resolveModelPlatform` 混为一谈。

`backend/internal/service/bundle_route_resolver.go` 里的 `resolveModelPlatform` 只是套餐路由的模型平台猜测逻辑。远程已有提交明确把 unknown 默认改为 `anthropic` 并移除了 `deepseek- -> openai`：

```text
d56405ea fix(resolver): default unknown model platform to anthropic, remove deepseek prefix
```

该提交存在于 `friend/dev`、`friend/plusversion-canvas-test`、`friend/sync-model-price`。

因此本轮不修改 DeepSeek / unknown 的平台推断，避免和朋友正在做的套餐/模型价格同步分支冲突。后续如果要改，必须单独评审套餐路由设计，而不是夹在画布/R2/模型广场任务里顺手改。

## Code Placement Rules

严禁乱堆代码、乱放文件、改变现有项目结构。

### sub2api

- 前端页面继续放：`frontend/src/views/`
- 前端组件继续放：`frontend/src/components/`
- 前端 API wrapper 继续放：`frontend/src/api/`
- 后端 handler 继续放：`backend/internal/handler/`
- 后端 routes 继续放：`backend/internal/server/routes/`
- 后端 service 继续放：`backend/internal/service/`
- 不把 Infinite Canvas 的 React/Next.js 代码放进 sub2api。
- 不新增随意顶层目录。

### infinite-canvas

- 前端 API 请求放：`web/src/services/api/`
- 前端状态和模型选择逻辑放：`web/src/stores/`
- 前端组件只做展示和交互，不写大段业务转换。
- 后端 HTTP 入口放：`handler/`
- 后端业务逻辑放：`service/`
- 路由只改：`router/router.go`
- 配置只改：`config/config.go`
- 不复制 sub2api 登录、充值、余额逻辑。
- 不把服务端密钥写入前端构建产物。

## Implementation Requirements

### A. Canvas Model API

在 Infinite Canvas 新增画布专用模型读取能力，不能复用模型广场。

必须放置：

```text
infinite-canvas/service/topai_models.go
infinite-canvas/handler/models.go
infinite-canvas/router/router.go
infinite-canvas/web/src/services/api/models.ts
infinite-canvas/web/src/stores/use-config-store.ts
```

要求：

- 服务端调用 TOP-AI `/v1/models`。
- 使用 `TOP_AI_GATEWAY_API_KEY`，只存在服务端环境。
- 兼容 OpenAI models list 返回格式。
- 返回去重后的 `models/textModels/imageModels/videoModels/audioModels`。
- 浏览器不能拿到内部 key、上游地址、账号、渠道、分组细节。

### B. Canvas Model Picker

所有画布模型下拉统一使用新模型列表。

涉及：

```text
web/src/components/model-picker.tsx
web/src/components/layout/app-config-modal.tsx
web/src/app/(user)/video/page.tsx
web/src/app/(user)/image/page.tsx
web/src/app/(user)/canvas/
```

要求：

- 视频页能选 `grok-imagine-video`。
- 图片页能选图片模型。
- 节点里的模型选择与全局设置一致。
- 模型为空时提示清楚，不引导客户去画布 admin/settings 配 Key。

### C. Model Marketplace

模型广场保留，但只做公开商品展示。

要求：

- 数据来自明确发布的模型和美元价格。
- 只返回安全 DTO。
- 公开接口只读、缓存、限流。
- 不自动暴露账号 `model_mapping`。
- 不 fallback 到 TOP-AI Gateway 可用模型。
- 不给画布当模型权限来源。
- 后端测试必须覆盖空目录、重复模型去重和价格优先级。

### D. Video Gateway

sub2api 已接入视频网关，不能绕过现有计费。

必须放置：

```text
sub2api/backend/internal/handler/
sub2api/backend/internal/service/
sub2api/backend/internal/server/routes/
```

要求：

- `POST /v1/videos`
- `GET /v1/videos/:id`
- `GET /v1/videos/:id/content`
- 复用现有鉴权、分组、调度、扣费、日志机制。
- 保留 `source=canvas`。
- 不新增独立野接口。
- 本地代码已接入；生产仍需要真实模型、真实余额和真实 10 秒视频生成验收。

### E. R2 Media Flow

继续使用现有 R2 配置。

要求：

- 参考素材走 `temp/reference/`。
- 生成结果走 `generated/`。
- 失败残留走 `failed/`。
- 不把大视频长期保存在服务器本地。
- 不把完整生成视频读入内存；使用临时文件或流式链路。
- R2 凭证只在服务端环境和本地私密文件，不进 Git。
- 当前本地实现使用临时文件中转生成视频内容，并复用该临时文件完成 R2 保存和浏览器响应。

## Configuration Rules

服务端配置示例：

```text
TOP_AI_PUBLIC_BASE_URL=https://show.top-ai.band
TOP_AI_INTERNAL_BASE_URL=http://sub2api:8080
TOP_AI_MODELS_URL=http://sub2api:8080/v1/models
TOP_AI_GATEWAY_API_KEY=<server-side-only>
TOP_AI_SESSION_URL=http://sub2api:8080/api/v1/app/canvas/session
CANVAS_BASE_PATH=/apps/canvas
NEXT_PUBLIC_CANVAS_BASE_PATH=/apps/canvas
NEXT_PUBLIC_TOP_AI_LOGIN_PATH=/login
CANVAS_DISABLE_LOCAL_AUTH=true
CANVAS_DISABLE_LOCAL_CREDITS=true
CANVAS_FORCE_TOP_AI_GATEWAY=true
MEDIA_STORAGE_DRIVER=r2
R2_BUCKET=canvas
R2_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
R2_PUBLIC_BASE_URL=https://media.888tech.club
R2_ACCESS_KEY_ID=<server-side-only>
R2_SECRET_ACCESS_KEY=<server-side-only>
R2_TEMP_REFERENCE_PREFIX=temp/reference
R2_GENERATED_PREFIX=generated
GENERATED_MEDIA_ALLOWED_HOSTS=vidgen.x.ai
```

`TOP_AI_MODELS_URL` 是画布模型来源，必须指向受保护的 TOP-AI `/v1/models`；模型广场 catalog 不允许作为画布权限来源。

`TOP_AI_SESSION_URL` 必须指向专用的 Canvas SSO 接口 `/api/v1/app/canvas/session`，不能用通用 `/api/v1/auth/me`。通用接口可能把 TOP-AI admin 登录态带给画布，导致普通用户流程和素材保存被拒绝。

`GENERATED_MEDIA_ALLOWED_HOSTS` 是生产必填项。为空时，画布会拒绝导入所有上游远程视频，防止开放代理和任意公网下载。当前 `grok-imagine-video` 返回的视频域名是 `vidgen.x.ai`，所以生产至少需要包含 `vidgen.x.ai`。新增其他视频上游后，只追加经过确认的可信媒体域名。

## Security Requirements

- 浏览器不能得到 TOP-AI Gateway Key。
- 浏览器不能得到 R2 写入密钥。
- 模型广场不能暴露任何内部调用接口。
- 模型广场不能消耗上游额度。
- 画布生成必须经过 TOP-AI Gateway。
- 普通客户不能使用画布本地充值、余额、上游 Key 配置。
- `webdav-proxy` 继续保持鉴权、禁私网、禁 metadata 地址。
- 所有公开接口必须有缓存或限流。

## Final Acceptance Checklist

- [ ] `/apps/canvas` 未登录时跳 TOP-AI 登录。
- [ ] 登录后进入画布，识别同一个 TOP-AI 普通用户。
- [ ] 生产 `TOP_AI_SESSION_URL` 使用 `/api/v1/app/canvas/session`，不能使用 `/api/v1/auth/me`。
- [x] 画布模型接口不再读 `/api/v1/public/models/catalog`。
- [x] 画布模型接口能返回 TOP-AI `/v1/models` 的真实模型。
- [x] 模型列表已去重。
- [x] 视频模型列表包含 `grok-imagine-video`。
- [x] 图片/文本/视频模型分类正确。
- [x] 模型广场为空或有数据都不影响画布。
- [x] 模型广场只展示公开商品数据，不暴露内部接口。
- [x] 模型广场不 fallback 暴露网关真实模型列表。
- [x] `/apps/canvas/admin/settings` 不再误导客户配置模型。
- [x] `/v1/videos` 创建、查询、取内容链路已接入现有网关。
- [ ] 10 秒视频生成可跑通。
- [ ] 生产 `GENERATED_MEDIA_ALLOWED_HOSTS` 包含真实视频上游媒体域名。
- [ ] 参考素材使用 R2 `temp/reference/`。
- [x] 本地代码已实现生成结果使用 R2 `generated/`。
- [ ] 生产真实生成结果已成功保存到 R2 `generated/`。
- [x] 视频内容代理不再整段读入内存。
- [x] TOP-AI 后台能看到 `source=canvas` 用量。
- [ ] TOP-AI 余额按规则扣费。
- [ ] 图片生成后可以添加到“我的素材”，并且刷新后仍可用。
- [ ] 视频生成后可以插入画布并播放，刷新后仍可用。
- [x] 前端构建产物没有服务端密钥。
- [x] sub2api 和 infinite-canvas 目录结构没有被破坏。

## Rollback Rule

如果上线失败，优先关闭画布入口或生成能力，不允许回滚到以下状态：

- 开放代理
- 密钥下发浏览器
- 绕过 TOP-AI 计费
- 大文件无限制穿透业务服务器
- 公开接口暴露内部账号或渠道数据
