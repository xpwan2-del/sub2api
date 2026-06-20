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
- 必须同时修完模型来源、模型去重、模型广场边界、画布模型选择、视频网关和生成媒体 R2 结果链路。
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
- [x] sub2api 视频内容接口已支持 `/content` 不可用时读取任务详情里的 `video.url` 并由后端取回视频流。
- [x] 参考素材继续走 R2 `temp/reference/`。
- [x] 本地代码已实现生成视频内容经过画布后端保存到 R2 `generated/`，前端优先使用返回的 R2 URL。
- [x] 生成视频内容已改为临时文件中转，不再整段读入内存。
- [ ] 生成图片仍需补齐 R2 真实来源和本地缓存兜底；不能只依赖浏览器 IndexedDB。
- [x] 模型广场没有公开商品数据时返回空列表，不再 fallback 暴露网关真实模型列表。
- [x] 模型广场已补回归测试：空数据返回空、重复模型去重、优先保留更明确/更低价格。
- [x] 生产 6 秒真实视频生成、`/content` 取回和 R2 保存已通过；固定 10 秒任务如作为产品要求需另测。

本轮本地验证结果：

- [x] `sub2api` public model catalog 定向测试通过。
- [x] `sub2api/frontend` production build 通过。
- [x] `infinite-canvas` Go tests 通过。
- [x] `infinite-canvas/web` format check 通过。
- [x] `infinite-canvas/web` production build 通过。
- [x] 两个项目 `git diff --check` 通过。

仍未做的验证：

- [ ] 图片生成结果保存到 R2 `generated/` 后，登录态、模型列表、图片/视频生成、素材恢复和余额扣费需要端到端复测。

### Production Reality Check - 2026-06-17

历史实测快照：当日代码能力已经合入，但生产配置没有完全按本文档执行，所以不能算生产完成。该小节保留问题演进证据；最终判断以 `Production Reality Check - 2026-06-20` 为准。

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
- 上游视频任务完成后返回 `https://vidgen.x.ai/...mp4`，但上游 `/v1/videos/:id/content` 返回 404。
- 旧代码没有从任务详情里的 `video.url` 取回真实视频流，导致画布后端无法保存到 R2 `generated/`。
- 图片生成成功后，“添加到素材”失败，浏览器报 `TypeError: Failed to fetch`，服务器日志出现 `请使用普通用户账号进入画布`。
- R2 对象列表未新增，说明本轮图片和视频都没有成功落到 R2。

当日判断，已被 2026-06-20 复测修正：

- 这不是 R2 域名不可用，也不是视频模型完全不可用。
- 当日怀疑图片添加素材问题来自生产 SSO 配置不一致；2026-06-20 复测确认普通用户 SSO 已通，当前剩余问题是图片仍依赖前端本地 IndexedDB 保存，线上 `uploadImage(image.dataUrl)` 转 Blob 失败。
- 当日视频保存问题来自 sub2api 对“不支持 `/content` 但任务详情提供 `video.url`”的上游模型兼容不足；2026-06-20 复测确认该链路已修复并能保存到 R2。
- 生成媒体白名单只影响旧的“前端拿到上游临时 URL 后再导入”兜底路径，不应作为线上画布视频保存的主依赖。

### Production Reality Check - 2026-06-20

生产二次实测结论：

- 普通用户 `xpwan1@gmail.com` 可登录 TOP-AI，Canvas SSO 可换取 Canvas 用户 `topai-2`。
- Canvas 模型接口可返回 3 个图片模型和 2 个视频模型，包含 `gpt-image-2`、`grok-imagine-video`、`grok-imagine-video-1.5-preview`。
- API 级图片生成成功，`gpt-image-2` 返回 1 张 `b64_json` 图片。
- 页面级图片生成成功，结果区出现 `生成结果 1`、`添加到素材`、`下载`。
- 页面级图片点击 `添加到素材` 后，`/apps/canvas/assets` 未新增图片；浏览器报 `TypeError: Failed to fetch`。
- API 级视频生成成功，`grok-imagine-video` 6 秒 720p 任务完成，任务状态为 `done`，进度 100。
- `GET /apps/canvas/api/v1/videos/:id/content?model=grok-imagine-video` 返回 `200`、`video/mp4`、`X-Canvas-Media-URL` 和 `X-Canvas-Media-Storage-Key`。
- 页面级视频生成成功，结果区视频可播放，视频元素 `readyState=4`，尺寸 1280x720，源地址为 R2 签名 URL。
- 视频点击 `添加到素材` 后，`/apps/canvas/assets` 能看到 `生成视频 / 视频 / 1280x720 / video/mp4`。

当前真实问题：

- 视频生成、R2 保存、素材页显示已经跑通。
- 图片生成已经跑通。
- 图片加入“我的素材”失败，原因不是 R2 不可用，而是图片保存仍走前端 `uploadImage(image.dataUrl)` 到浏览器本地 IndexedDB，线上转换 Blob 时 `fetch` 失败。
- 当前代码里“我的素材”记录主要保存在浏览器本地 `localForage`；视频生成结果已具备 R2 `r2:generated/...` 兜底，图片生成结果仍主要是 `image:...` 本地缓存。
- 生产标准不能依赖上游临时 URL 或浏览器本地缓存作为唯一来源。

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

`GET /v1/videos/:id/content` 的主语义是“给 Canvas 返回真实视频流”。如果上游原生 `/content` 不支持并返回 404/405，sub2api 必须查询同一任务详情，从 `video.url`、`data.video.url`、`output[0].url`、`videos[0].url`、`content.video_url` 等字段提取视频地址，再由服务端安全取回视频流返回给 Canvas。该兜底不能针对单一模型写死，且不能把 TOP-AI Gateway API Key 转发给第三方媒体 URL。

生产仍需验收：

- 真实 `grok-imagine-video` 或等价视频模型能创建视频任务；6 秒任务已通过，固定 10 秒任务如作为产品要求需另测。
- 查询任务状态能拿到完成结果。
- `/v1/videos/:id/content` 能返回真实视频流；即使上游 `/content` 返回 404，也能通过任务详情 `video.url` 取回视频。
- 画布后端能把该视频流保存到 R2 `generated/`，刷新后素材仍可播放。
- TOP-AI 余额和 usage log 与 `source=canvas` 正常记录。

### 6. Generated Media Must Use R2 As Source Of Truth

图片、视频和大素材不能长期压业务服务器本地磁盘，也不能把浏览器本地 IndexedDB 当成唯一来源。

生产标准链路：

```text
参考素材 -> R2 temp/reference/
生成图片 -> R2 generated/
生成视频 -> R2 generated/
我的素材记录 -> storageKey + metadata
本地 IndexedDB -> 只做缓存
画布展示 -> 本地缓存命中则用本地；缓存缺失则用 storageKey 换 R2 签名 URL
TOP-AI 记录用量和 source=canvas
```

不能靠单纯调大 Nginx/Caddy 超时和 body size 当最终方案。

生产规则：

- R2 是生成媒体的真实来源；浏览器本地缓存只能用于加速和离线临时体验。
- `storageKey` 必须代表不可变文件。文件内容变化时必须生成新的 `storageKey`，不能覆盖旧 R2 对象。
- 我的素材可以更新标题、标签、备注和当前引用的 `storageKey`，但不能复用同一个 key 写入不同内容。
- 老的 `image:...`、`video:...` 本地素材继续兼容；新生成的图片和视频必须优先保存为 `r2:generated/...`。
- 本地缓存 key 必须绑定远程 `storageKey`。如果素材记录的 `storageKey` 变化，本地旧缓存不能继续作为当前素材内容。
- 上游模型返回的临时 URL 只允许作为服务端搬运来源，不能长期写入用户素材作为最终地址。
- 浏览器不能得到 R2 写入密钥；上传和签名 URL 生成必须在 Canvas 后端完成。
- R2 bucket 生产环境应保持私有；浏览器只接收短期签名 URL 或受控公共 CDN URL。

主流平台参考结论：

- OpenAI/Sora 的完成视频应通过 `/videos/{id}/content` 下载二进制内容后保存到自有存储。
- Runway、Replicate、Kling、部分 OpenAI 图片 URL 和其他视频模型平台的输出 URL 多为临时资源，长期使用必须由业务方自行保存。
- 因此 TOP-AI 托管 Canvas 必须把生成媒体及时落到自己的 R2，而不是依赖上游临时 URL 或浏览器本地缓存。

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
- 本地代码已接入；生产 6 秒真实视频生成、`/content` 取回和 R2 保存已通过。后续如果产品要求固定 10 秒任务，需按同一链路单独复测，不影响当前视频 R2 链路结论。

### E. R2 Media Flow

继续使用现有 R2 配置。

要求：

- 参考素材走 `temp/reference/`。
- 生成图片和生成视频都走 `generated/`。
- 失败残留走 `failed/`。
- 不把大视频长期保存在服务器本地。
- 不把完整生成视频读入内存；使用临时文件或流式链路。
- 不把浏览器本地 IndexedDB 当成生成媒体的唯一来源。
- R2 凭证只在服务端环境和本地私密文件，不进 Git。
- 当前本地实现使用临时文件中转生成视频内容，并复用该临时文件完成 R2 保存和浏览器响应。

### F. Generated Image R2 Source Of Truth

当前图片生成结果仍主要走前端本地缓存，未达到生产标准。下一次实现必须补齐以下范围，不允许散落到临时脚本或页面杂逻辑里。

文件摆放硬规则：

- 默认不新增生产文件；优先在现有职责文件内补齐能力。
- 如确需新增生产文件，必须先证明现有 `handler`、`service`、`services`、`stores` 分层无法承载，且只能放在对应职责目录内，不能新建顶层目录。
- 后端生成媒体入口只允许在 `infinite-canvas/handler/` 和 `infinite-canvas/service/` 分层内实现。
- 前端媒体缓存、R2 签名 URL 解析、素材 hydration 只允许放在 `infinite-canvas/web/src/services/`、`infinite-canvas/web/src/stores/` 和对应页面现有文件内。
- 不允许把 R2 上传、签名 URL、storageKey 解析、安全校验这类业务逻辑堆进 React 页面组件。
- 测试只补在已有同类测试文件旁边，不能为临时验证新建脚本目录。

需要修改的生产文件：

- `infinite-canvas/handler/media_reference.go`
  - 将 `POST /api/v1/media/generated` 从“生成视频导入”扩展为“生成媒体保存”。
  - 支持图片和视频的服务端保存。
  - 保留 HTTPS、host allowlist、私网 IP 阻断、重定向限制和大小限制。
  - 错误文案从“生成视频”泛化为“生成媒体”，但不要降低现有安全限制。
- `infinite-canvas/service/reference_media.go`
  - 继续复用 `SaveGeneratedMedia` 和 `r2:generated/...` key 规则。
  - 确保生成图片和生成视频都生成新的不可变 R2 key。
- `infinite-canvas/web/src/services/image-storage.ts`
  - 支持识别 `r2:generated/...` 图片 key。
  - 本地缓存命中时用本地 Blob；缓存缺失时调用 `/api/v1/media/generated?key=...` 换 R2 签名 URL。
  - `image:...` 旧本地缓存继续兼容。
- `infinite-canvas/web/src/app/(user)/image/page.tsx`
  - 生图成功后先把图片保存到 R2，再写入我的素材。
  - 我的素材保存 R2 `storageKey`、`mimeType`、`bytes`、`width`、`height` 和必要 metadata。
  - 本地缓存失败不能导致 R2 成功结果丢失。
- `infinite-canvas/web/src/stores/use-asset-store.ts`
  - 我的素材 hydration 要兼容图片 `r2:generated/...`。
  - 老 `image:...` 资产不迁移、不删除，继续可显示。
- `infinite-canvas/web/src/app/(user)/assets/page.tsx`
  - 图片预览和下载要兼容 R2 图片签名 URL。
  - 视频现有 R2 行为不能回退。

需要补的测试：

- `infinite-canvas/handler/media_reference_test.go`
  - 覆盖生成图片类型识别、生成视频类型识别、非法 URL 拒绝、未配置 allowlist 拒绝。
- `infinite-canvas/service/reference_media_test.go`
  - 覆盖 `r2:generated/...` key 解析、防路径穿越、生成媒体保存结果字段。

不在本轮顺手扩大的范围：

- 不重构全部画布节点、裁剪、助手面板、参考图上传等所有 `uploadImage()` 调用。
- 不改变旧本地素材导出/导入格式，除非为兼容 R2 key 做最小改动。
- 不把素材记录改进 sub2api 数据库；当前“我的素材”记录仍按 Canvas 现有本地 store 机制保存，媒体文件由 R2 兜底。
- 不暴露 R2 写入凭证到浏览器。

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
```

`TOP_AI_MODELS_URL` 是画布模型来源，必须指向受保护的 TOP-AI `/v1/models`；模型广场 catalog 不允许作为画布权限来源。

`TOP_AI_SESSION_URL` 必须指向专用的 Canvas SSO 接口 `/api/v1/app/canvas/session`，不能用通用 `/api/v1/auth/me`。通用接口可能把 TOP-AI admin 登录态带给画布，导致普通用户流程和素材保存被拒绝。

`GENERATED_MEDIA_ALLOWED_HOSTS` 只用于旧的上游临时 URL 导入兜底路径。线上托管画布的主链路必须优先走 `/api/v1/videos/:id/content`，由 Canvas 后端通过 TOP-AI Gateway 获取真实视频流并保存到 R2，避免每接入一个视频模型都维护一次上游临时媒体域名白名单。若保留 URL 导入兜底，则白名单必须只包含确认可信的媒体域名。

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
- [x] 真实视频生成可跑通。
- [x] 上游 `/content` 返回 404/405 时，sub2api 能通过任务详情 `video.url` 取回视频流。
- [ ] 参考素材使用 R2 `temp/reference/`。
- [x] 视频生成结果使用 R2 `generated/`。
- [ ] 图片生成结果使用 R2 `generated/`。
- [x] 生产真实视频生成结果已成功保存到 R2 `generated/`。
- [ ] 生产真实图片生成结果已成功保存到 R2 `generated/`。
- [ ] 我的素材图片记录使用 R2 `storageKey`，本地缓存丢失后仍可从 R2 恢复。
- [ ] 本地缓存命中时不重复下载 R2；素材 `storageKey` 变化时不复用旧缓存。
- [x] 视频内容代理不再整段读入内存。
- [x] TOP-AI 后台能看到 `source=canvas` 用量。
- [ ] TOP-AI 余额按规则扣费。
- [ ] 图片生成后可以添加到“我的素材”，刷新、清缓存或换浏览器后仍可用。
- [x] 视频生成后可以添加到“我的素材”并播放，刷新后仍可用。
- [x] 前端构建产物没有服务端密钥。
- [x] sub2api 和 infinite-canvas 目录结构没有被破坏。

## Rollback Rule

如果上线失败，优先关闭画布入口或生成能力，不允许回滚到以下状态：

- 开放代理
- 密钥下发浏览器
- 绕过 TOP-AI 计费
- 大文件无限制穿透业务服务器
- 公开接口暴露内部账号或渠道数据
