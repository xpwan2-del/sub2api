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
- 本节的一次性闭环只针对 TOP-AI 集成、模型、网关、R2 媒体和生产验收；官方功能更新合并必须按 `Upstream Merge And Canvas Platform Slimdown Plan` 分提交执行。

## Completed. Do Not Rework

- [x] `sub2api` 和 `infinite-canvas` 保持平级独立项目。
- [x] 入口路径使用 `/apps/canvas`，不使用 `/canvas`。
- [x] 不 iframe 嵌入，不把画布源码塞进 `sub2api/frontend`。
- [x] TOP-AI 首页和控制台已有画布入口。
- [x] Infinite Canvas 已支持 `CANVAS_BASE_PATH=/apps/canvas`。
- [x] sub2api 已有 TOP-AI session 校验接口：`GET /api/v1/app/canvas/session`。
- [x] Infinite Canvas 已有 TOP-AI 登录态换画布 token：`POST /api/auth/top-ai/session`。
- [x] 画布未登录时跳回 TOP-AI 登录页。
- [x] 画布本地登录、注册、Linux.do 和自有 admin 后台入口已从本地代码移除。
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
- [x] `/apps/canvas/admin/settings` 和画布自有 admin 页面已从本地代码移除；模型、Key、余额和计费配置只属于 TOP-AI/sub2api。
- [x] sub2api 已补 `POST /v1/videos`、`GET /v1/videos/:id`、`GET /v1/videos/:id/content`，并挂到现有网关 routes/handler/service。
- [x] sub2api 视频内容接口已支持 `/content` 不可用时读取任务详情里的 `video.url` 并由后端取回视频流。
- [x] 参考素材继续走 R2 `temp/reference/`。
- [x] 本地代码已实现生成视频内容经过画布后端保存到 R2 `generated/`，前端优先使用返回的 R2 URL。
- [x] 生成视频内容已改为临时文件中转，不再整段读入内存。
- [x] 本地代码已实现生成图片保存到 R2 `generated/`，并保留本地缓存兜底；生产端到端仍需复测。
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
- [x] `infinite-canvas` 本地瘦身后 `go test ./...` 通过。
- [x] `infinite-canvas/web` 本地瘦身后 `npm run build` 通过，构建路由不再包含 `/login` 和 `/admin`。

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

当时生产真实问题：

- 视频生成、R2 保存、素材页显示已经跑通。
- 图片生成已经跑通。
- 图片加入“我的素材”失败，原因不是 R2 不可用，而是图片保存仍走前端 `uploadImage(image.dataUrl)` 到浏览器本地 IndexedDB，线上转换 Blob 时 `fetch` 失败。
- 当时生产代码里“我的素材”记录主要保存在浏览器本地 `localForage`；视频生成结果已具备 R2 `r2:generated/...` 兜底，图片生成结果仍主要是 `image:...` 本地缓存。
- 生产标准不能依赖上游临时 URL 或浏览器本地缓存作为唯一来源。

本地修正状态：

- `infinite-canvas` 本地提交 `89ced7a fix: persist generated images to r2` 已补齐生成图片 R2 保存、本地缓存、R2 签名 URL 兜底和素材 hydration。
- 该修正已通过 `go test ./...`、`infinite-canvas/web npm run build`、Prettier check 和 `git diff --check`。
- 生产环境仍必须重新实测图片生成、添加素材、刷新、清缓存或换浏览器后的 R2 恢复能力；未通过前不能把图片 R2 链路标记为生产完成。

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

### 4. Canvas Admin Settings Removed

`/apps/canvas/admin/settings` 是 Infinite Canvas 原项目的本地后台设置页。TOP-AI 集成后，普通客户不应该通过这里配置模型、Key 或余额。本地代码已删除画布自有 admin 页面和对应 `/api/admin/*` 后端入口。

当前状态：

- `/apps/canvas/admin/settings` 不再作为页面存在。
- `/api/admin/*` 不再作为后端路由存在。
- 本地登录、注册、Linux.do OAuth、本地 admin login 不再作为入口存在。
- 普通客户不能把任何画布页面当成模型、Key、余额或计费配置入口。
- 模型、Key、余额、计费和权限配置只属于 TOP-AI/sub2api。

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

## Upstream Merge And Canvas Platform Slimdown Plan

### Why This Plan Exists

Infinite Canvas 官方在 `v0.4.0` 方向主动移除了 Go 后端、后台管理、账号系统和数据库，项目定位改为个人本地画布工具。官方这样做是为了降低部署门槛，让用户在浏览器本地配置 Base URL、API Key 和模型，前端直连 OpenAI 兼容接口。

TOP-AI 托管画布不能直接照搬这个架构。我们的画布本质上也是前端工具，但它是 TOP-AI 平台里的前端工具，不是独立个人工具。

因此合并策略必须拆成两件事：

- 吸收官方前端功能更新。
- 清理画布自己的历史后台和本地账号体系，只保留 TOP-AI 平台桥。

不能整体 merge 官方 `origin/main`。官方新版删除了 `handler/`、`service/`、`repository/`、`router/`、`config/`、`go.mod` 和大量后台页面，直接合并会冲掉当前生产依赖的 SSO、模型、网关、视频 `/content`、R2 保存和媒体签名读取能力。

### Target Shape

目标形态：

```text
官方 Infinite Canvas 前端功能
  + TOP-AI Canvas 平台桥
  - 画布自己的本地账号系统
  - 画布自己的 admin 后台
  - 画布自己的用户/积分/模型渠道数据库管理
```

最终画布应该是：

- 前端页面、画布交互、Agent、图片/视频/音频工作台尽量跟随官方。
- 登录、模型、权限、余额、计费、网关调用、R2 媒体保存由 TOP-AI/sub2api 提供。
- 画布服务端只作为平台桥，不再承担一套独立后台产品。

### Keep: TOP-AI Platform Bridge

以下能力必须保留，不能在合并官方代码时删除：

- `POST /api/auth/top-ai/session`
- `GET /api/auth/me`
- `GET /api/settings` 中与 TOP-AI 托管模式相关的公开配置。
- `GET /api/v1/models`
- `POST /api/v1/images/generations`
- `POST /api/v1/images/edits`
- `POST /api/v1/chat/completions`
- `POST /api/v1/audio/speech`
- `POST /api/v1/videos`
- `GET /api/v1/videos/:id`
- `GET /api/v1/videos/:id/content`
- `POST /api/v1/media/references`
- `POST /api/v1/media/generated`
- `GET /api/v1/media/generated`
- 参考素材 R2 `temp/reference/`
- 生成图片/视频 R2 `generated/`
- `X-Canvas-Source`、`X-Top-AI-User-ID`、`X-Client-Request-ID`
- `metadata.source=canvas`
- TOP-AI Gateway API Key 仅服务端可见。
- R2 写入密钥仅服务端可见。

这些代码只能放在现有职责目录内：

```text
infinite-canvas/handler/
infinite-canvas/service/
infinite-canvas/middleware/
infinite-canvas/router/router.go
infinite-canvas/config/config.go
infinite-canvas/web/src/services/
infinite-canvas/web/src/stores/
```

如果后续要把平台桥进一步独立成更小的后端包，也必须先单独评审，不允许在合官方前端功能时顺手乱挪目录。

### Local Slimdown Implementation Status

本地 `infinite-canvas` 已按本文档完成第一笔瘦身改动，范围只处理画布自有后台和本地账号体系，不合并官方功能更新，不改变数据库结构。

已删除或断开的入口：

- 前端 `web/src/app/(admin)/admin/*`。
- 前端 `web/src/app/(user)/login/page.tsx`。
- 前端 `web/src/services/api/admin.ts`。
- 用户菜单里的画布 admin 入口和本地登录链接。
- 后端 `/api/auth/login`、`/api/auth/register`。
- 后端 `/api/auth/linux-do/authorize`、`/api/auth/linux-do/callback`。
- 后端 `/api/admin/login` 和整组 `/api/admin/*`。
- 启动时创建默认本地 admin 的逻辑。
- 本地 admin/settings/channel-test、Linux.do OAuth、本地用户/credit 管理的可调用 service/handler 能力。

保留的生产桥能力：

- `POST /api/auth/top-ai/session`。
- `GET /api/auth/me`。
- `GET /api/settings`。
- `/api/v1/*` 生成、模型、媒体和视频 `/content` 链路。
- TOP-AI Gateway 服务端调用、R2 参考素材和生成媒体保存。
- Canvas 用户映射所需的最小用户表、JWT、用户鉴权中间件。
- 公开只读提示词和素材读取接口，因为当前用户页面仍在使用；它们不再有画布自有后台写入口。

本地验证结果：

- `go test ./...` 通过。
- `infinite-canvas/web npm run build` 通过。
- 生产环境仍需按下方 `Required Tests After Slimdown` 和最终验收清单做页面级复测。

### Remove Or Retire: Canvas-Owned Backend Product

以下属于画布自己的历史后台产品能力，TOP-AI 托管模式不需要。本地代码已完成第一轮删除或废弃；后续如继续移除更底层的数据模型字段，必须另做评审，不能影响现有生产数据库。

前端可删除或改为不可达：

- `web/src/app/(admin)/admin/*`
- `web/src/services/api/admin.ts`
- 画布本地登录/注册入口。
- `user-status-actions` 里的画布 admin 入口。
- 引导用户配置本地模型渠道、API Key、余额和注册开关的后台页面。

后端可删除或废弃：

- 画布本地 admin login。
- 画布本地 register/login。
- Linux.do OAuth。
- 本地用户管理。
- 本地积分/credit 管理。
- 本地 settings 管理。
- 本地渠道模型管理。
- 本地提示词后台管理。
- 本地公共素材后台管理。

候选文件范围：

```text
infinite-canvas/handler/admin.go
infinite-canvas/handler/assets.go
infinite-canvas/handler/settings.go
infinite-canvas/handler/prompts.go
infinite-canvas/middleware/admin.go
infinite-canvas/repository/asset.go
infinite-canvas/repository/prompt.go
infinite-canvas/repository/setting.go
infinite-canvas/service/assets.go
infinite-canvas/service/prompts.go
infinite-canvas/service/prompt_fetch.go
infinite-canvas/service/prompt_sync_scheduler.go
infinite-canvas/service/settings.go
infinite-canvas/web/src/app/(admin)/admin/
```

不能在没有替代方案前删除：

- `repository/user.go` 中 TOP-AI canvas user 映射仍依赖的最小用户存储能力。
- `model/user.go` 中 Canvas session token 仍依赖的用户类型。
- `service/auth_topai.go`。
- `service/context.go`。
- `middleware` 中用户鉴权能力。

如果最终决定完全不在 Canvas DB 保存用户映射，必须先设计 JWT payload 和 TOP-AI session 刷新策略，确保 `topAIUserIDForCanvasUser`、usage metadata、WebDAV 鉴权和媒体保存日志仍能稳定识别用户。

### Official Frontend Updates To Import

官方更新按优先级分批移植，不整体合并。

第一优先级：生成停止功能。

- 来源提交：`73090da feat: 生成增加停止功能`
- 价值：用户可中断图片、视频、音频、文本生成；视频轮询不会被迫等到超时。
- 需要移植：
  - Canvas 页面里的 `AbortController` 管理。
  - `requestGeneration`、`requestEdit`、`requestVideoGeneration`、`requestAudioGeneration` 的 `signal` 参数。
  - 生成中节点的停止按钮和取消状态处理。
- TOP-AI 适配要求：
  - 取消前端请求只表示客户端停止等待；已经发到 TOP-AI Gateway 的上游任务是否取消，取决于上游能力。
  - 取消不能触发错误退款逻辑错乱。
  - 已完成并保存到 R2 的结果不能因取消状态被删除。

第二优先级：Agent 面板和 Tool Calling。

- 来源提交：`d3923e2`、`7893bd7`、`27bd6f0`、`1b289f6`、`f7b030e`、`bce9c2d`、`f038406`
- 价值：
  - 新增 `canvas-agent-chat-ui.tsx`。
  - 在线 Agent 从模型输出 JSON 改成 function/tool calling。
  - 支持读取画布、创建节点、更新节点、连接节点、删除节点、触发生成。
  - 支持流式回复、工具执行日志、确认工具调用、只读问题回答。
- TOP-AI 适配要求：
  - Agent 文本模型仍从 TOP-AI 模型列表选择。
  - Agent 触发生成仍走 TOP-AI Gateway 和 R2 保存链路。
  - 不允许 Agent 读到服务端密钥、R2 凭证、内部渠道配置。
  - Tool schema 可移植，账号和配置模式不能照搬官方本地 key 模式。

第三优先级：Gemini API Format 支持。

- 来源提交：`8cbe00e feat(api): add support for Gemini API format and enhance channel configuration`
- 价值：
  - 支持 Gemini 文本、图片、流式响应、tool calling、模型列表。
  - 为后续接入 Gemini 兼容模型提供前端调用格式能力。
- TOP-AI 适配要求：
  - 主模式仍必须是 `channelMode=remote`，由 TOP-AI/sub2api 控制模型、权限、计费和 Key。
  - 如果 TOP-AI Gateway 已经把 Gemini 包装成 OpenAI 兼容接口，前端不应绕过网关直连 Gemini。
  - 只有当 sub2api 明确暴露“模型需要 Gemini 调用格式”的安全 DTO 时，前端才启用 Gemini 格式。
  - 不允许用户在托管生产里输入自己的 Gemini API Key 覆盖平台计费。

第四优先级：模型选择和配置体验优化。

- 可借鉴：
  - 模型按 image/video/text/audio 分类。
  - 模型选项展示模型名和渠道/来源标签。
  - 配置弹窗提醒“模型需要加入对应可选项才会显示”。
- 不能照搬：
  - 用户本地多渠道 API Key 管理。
  - 浏览器保存平台 API Key。
  - `channelMode` 强制改为官方 `local`。

第五优先级：小交互优化。

- Mention 菜单 active item 自动滚动。
- Agent 面板历史、日志和只读回答优化。
- 生成配置节点、提示词面板的细节状态处理。

跳过：

- Vercel 一键部署作为主生产形态。
- 纯前端 API Key 模式作为 TOP-AI 生产主模式。
- 删除平台桥的官方架构变更。

### Merge Workflow

每次同步官方更新必须按下面流程执行。

1. 先 fetch 官方：

```bash
git fetch origin --prune
```

2. 只看官方改动范围，不直接 merge：

```bash
git log --oneline <last-upstream-base>..origin/main -- web/src canvas-agent docs CHANGELOG.md
git diff --name-status <last-upstream-base>..origin/main -- web/src canvas-agent
```

3. 按功能 cherry-pick 或手工移植。

优先使用小范围 cherry-pick；如果提交包含“删除后端/纯前端化/本地 Key 模式”，必须手工移植需要的前端片段，不能直接吃整提交。

4. 平台桥冲突处理规则：

- 官方删除 `handler/`、`service/`、`router/`、`config/` 时，默认拒绝该删除。
- 官方删除登录、用户、auth store 时，只能采纳 UI 简化思路，不能删除 TOP-AI SSO。
- 官方改 `services/api/image.ts`、`services/api/video.ts` 时，必须重新套回 `remote` 模式、TOP-AI token、R2 结果保存和 `/content` 兜底。
- 官方改 `use-config-store.ts` 时，不能把默认 `channelMode` 改回 `local`。

5. 每个功能一笔提交。

示例提交顺序：

```text
refactor(canvas): remove legacy canvas admin surface
fix(canvas): keep top-ai platform bridge after admin removal
feat(canvas): import upstream generation cancellation
feat(canvas): import upstream agent tool calling
feat(canvas): add gateway-safe gemini format support
```

不能把“删后台”“合官方功能”“修 R2 bug”“改部署”混成一个大提交。

### Required Tests After Slimdown

瘦身完成后必须验证：

- `/apps/canvas` 未登录跳 TOP-AI 登录。
- TOP-AI 登录后 Canvas SSO 成功。
- `/api/auth/top-ai/session` 可换 Canvas token。
- `/api/auth/me` 返回普通 Canvas 用户，不误识别 TOP-AI admin。
- `/api/v1/models` 返回 TOP-AI 模型列表。
- 图片生成走 TOP-AI Gateway。
- 图片生成结果保存 R2 `generated/`。
- 视频生成走 TOP-AI Gateway。
- 视频 `/content` 能保存 R2 `generated/`。
- `/api/v1/media/generated?key=r2:generated/...` 可返回签名 URL。
- 我的素材刷新后仍能显示图片和视频。
- 清浏览器 IndexedDB 后仍能通过 R2 恢复生成媒体。
- TOP-AI usage log 能看到 `source=canvas`。
- TOP-AI 余额扣费正确。
- 前端构建产物不含 TOP-AI Gateway Key 或 R2 写密钥。

### Required Tests After Each Upstream Feature Import

生成停止功能：

- 生图中点击停止，UI 回到非运行态。
- 生视频轮询中点击停止，UI 不继续轮询。
- 停止后已完成结果不丢失。
- 停止后再次生成可用。

Agent Tool Calling：

- 只读询问只读取画布，不改节点。
- 创建文本节点成功。
- 创建生成配置节点并连线成功。
- 触发生图/生视频仍走 TOP-AI Gateway。
- 工具确认模式下，未确认不改画布。

Gemini Format：

- OpenAI 兼容模型不受影响。
- Gemini 格式只在安全模型配置允许时启用。
- Gemini 文本、图片、tool calling 至少各测一次。
- 不绕过 TOP-AI 计费。

### Documentation Rule

每次合官方功能或删除历史后台，都必须更新本文档：

- 更新已移植的官方提交范围。
- 更新哪些官方提交被明确跳过。
- 更新保留的平台桥接口列表。
- 更新生产验证结果。

如果文档和代码冲突，以代码和生产实测为准，但必须在同一轮修正文档。

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

本地代码已补齐生成图片 R2 真实来源和本地缓存兜底。该小节保留为回归保护和生产验收标准：后续修改不能把生成图片退回到只依赖浏览器 IndexedDB，也不能把相关逻辑散落到临时脚本或页面杂逻辑里。

文件摆放硬规则：

- 默认不新增生产文件；优先在现有职责文件内补齐能力。
- 如确需新增生产文件，必须先证明现有 `handler`、`service`、`services`、`stores` 分层无法承载，且只能放在对应职责目录内，不能新建顶层目录。
- 后端生成媒体入口只允许在 `infinite-canvas/handler/` 和 `infinite-canvas/service/` 分层内实现。
- 前端媒体缓存、R2 签名 URL 解析、素材 hydration 只允许放在 `infinite-canvas/web/src/services/`、`infinite-canvas/web/src/stores/` 和对应页面现有文件内。
- 不允许把 R2 上传、签名 URL、storageKey 解析、安全校验这类业务逻辑堆进 React 页面组件。
- 测试只补在已有同类测试文件旁边，不能为临时验证新建脚本目录。

已修改并必须保持职责清晰的生产文件：

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

已补并必须保持覆盖的测试：

- `infinite-canvas/handler/media_reference_test.go`
  - 覆盖生成图片类型识别、生成视频类型识别、非法 URL 拒绝、未配置 allowlist 拒绝。
- `infinite-canvas/service/reference_media_test.go`
  - 覆盖 `r2:generated/...` key 解析、防路径穿越、生成媒体保存结果字段。

仍不应顺手扩大的范围：

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
- 普通客户不能使用画布本地登录、注册、充值、余额、上游 Key 配置或画布自有 admin。
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
- [x] `/apps/canvas/admin/settings` 和画布自有 admin 页面已从本地代码移除。
- [x] `/v1/videos` 创建、查询、取内容链路已接入现有网关。
- [x] 真实视频生成可跑通。
- [x] 上游 `/content` 返回 404/405 时，sub2api 能通过任务详情 `video.url` 取回视频流。
- [ ] 参考素材使用 R2 `temp/reference/`。
- [x] 视频生成结果使用 R2 `generated/`。
- [ ] 生产图片生成结果使用 R2 `generated/`。
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
