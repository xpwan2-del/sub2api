# TOP-AI Security Audit Report

## Purpose

本文档记录当前 `sub2api` 和 `infinite-canvas` 集成后的安全审计结论。

本文档只记录已确认的问题、风险、证据、修复要求和验收标准，不写临时方案，不堆无关需求。

本文档不替代 `CANVAS_INTEGRATION_PLAN.md` 和 `MODEL_CATALOG_FRONTEND_PLAN.md`。功能开发仍按原开发文档执行；涉及安全风险时，以本文档的安全修复要求作为补充约束。

## Execution Rules

- 不允许乱堆代码。
- 不允许乱放文件。
- 不允许改变 `sub2api` 和 `infinite-canvas` 现有项目结构。
- 不允许把两个项目源码互相复制、搬运或混合。
- 不允许把业务逻辑塞进页面组件、路由文件或临时脚本。
- 所有修复必须按各自项目现有目录、分层和命名规则完成。
- 所有密钥、Token、API Key、代理密码、R2 Key 不得写入 Git。
- 安全修复必须本地验证后再上线。

## Audited Deployment Snapshot

- Audit date: 2026-06-15
- Production re-check: 2026-06-17
- Public site: `https://show.top-ai.band`
- TOP-AI backend container: `sub2api`
- Canvas container: `top-ai-canvas`
- Reverse proxy: `sub2api-caddy`
- Database: `sub2api-postgres`
- Cache: `sub2api-redis`
- Canvas mount path: `/apps/canvas`
- R2 generated media lifecycle: `generated/` delete after 30 days
- R2 temporary upload lifecycle: `temp/upload/` delete after 1 day
- R2 reference media lifecycle: `temp/reference/` delete after 7 days
- R2 failed object lifecycle: `failed/` delete after 1 day

## Critical Findings

### 1. Canvas Default Admin Login

Status: fixed in local code; verify after deployment.

Risk:

- The Canvas admin login endpoint must not accept the project default admin credential in TOP-AI hosted mode.
- TOP-AI admin users must not be treated as normal Canvas users unless an explicit mapping rule is added.

Evidence:

- `infinite-canvas/config/config.go`
- `infinite-canvas/service/auth.go`
- `infinite-canvas/handler/auth.go`
- `infinite-canvas/router/router.go`

Root cause:

- The Canvas project has default admin bootstrap logic.
- The admin login route is registered outside the admin-authenticated route group.
- The local admin login path is not blocked by TOP-AI hosted mode.

Implemented fix:

- `CANVAS_DISABLE_LOCAL_AUTH` / `CANVAS_FORCE_TOP_AI_GATEWAY` prevent default local admin bootstrap.
- Default admin credentials are rejected at startup when local auth is enabled.
- TOP-AI hosted login accepts only normal TOP-AI users for Canvas.
- Existing TOP-AI admin-derived Canvas sessions are rejected.

Production finding on 2026-06-17:

- The production Canvas container used `TOP_AI_SESSION_URL=http://sub2api:8080/api/v1/auth/me`.
- The required endpoint is `http://sub2api:8080/api/v1/app/canvas/session`.
- Using the generic `/api/v1/auth/me` endpoint can pass a TOP-AI admin session into Canvas.
- Canvas correctly rejects that role with `请使用普通用户账号进入画布`, but this breaks normal Canvas asset-save flows when the browser session is mixed or admin-derived.

Acceptance:

- `POST /apps/canvas/api/admin/login` must reject the default admin credential.
- TOP-AI normal users can still enter `/apps/canvas` through TOP-AI login.
- TOP-AI admin access and Canvas admin access must have an explicit mapping rule.
- Production `TOP_AI_SESSION_URL` must use `/api/v1/app/canvas/session`, not `/api/v1/auth/me`.

### 2. Exposed R2 Credentials Must Be Rotated

Status: must fix before production.

Risk:

- R2 token and access keys were shared during setup.
- Treat them as exposed even if the local file permission is correct.

Evidence:

- Local secret file permission is correct: `~/.ssh/top-ai-r2.env` is not world-readable.
- The exposure risk comes from the setup process, not from Git.

Required fix:

- Revoke the old Cloudflare R2 API token.
- Revoke the old R2 Access Key ID and Secret Access Key.
- Generate new R2 credentials with only the required bucket permissions.
- Update server environment files manually.
- Keep R2 credentials out of Git.

Acceptance:

- Old R2 credentials no longer work.
- New credentials allow only required object operations on the `canvas` bucket.
- Local and server env files stay permission-restricted.

### 3. Server Main Environment File Permission Is Too Loose

Status: must fix.

Risk:

- Server `/opt/sub2api/deploy/.env` is readable by non-owner users when permission is `644`.
- Environment files may contain secrets.

Evidence:

- Server deployment inspection showed `/opt/sub2api/deploy/.env` permission as `644`.
- Server deployment inspection showed `/opt/sub2api/deploy/.env.canvas` permission as `600`.

Required fix:

- Change `/opt/sub2api/deploy/.env` to owner-only read/write.
- Keep `/opt/sub2api/deploy/.env.canvas` owner-only read/write.

Acceptance:

- Server env files use permission `600`.
- Env files are owned by the deploy user.
- Env files remain ignored by Git.

## High Findings

### 4. Canvas Generated Media Import Can Be Abused

Status: partially fixed; production allowlist required.

Risk:

- A logged-in Canvas user can ask the server to fetch a public URL and store the response to R2.
- Existing checks block private IPs and local metadata addresses, but the endpoint can still be abused for public URL fetching, storage cost, and bandwidth cost.
- DNS rebinding and redirect behavior must be handled carefully.

Evidence:

- `infinite-canvas/handler/media_reference.go`

Implemented fix:

- Only HTTPS generated media URLs are accepted.
- `GENERATED_MEDIA_ALLOWED_HOSTS` is required; empty allowlist rejects imports.
- The hardened transport rejects non-allowlisted hosts and private/non-public IPs.
- Redirects are bounded and rechecked.
- Object size limits remain enforced.

Still required:

- Configure `GENERATED_MEDIA_ALLOWED_HOSTS` on production with trusted upstream media domains.
- Keep per-user rate limits and storage monitoring enabled.
- Log `user_id`, task id, source, object key, and byte size.

Production finding on 2026-06-17:

- Production `GENERATED_MEDIA_ALLOWED_HOSTS` was missing.
- A real `grok-imagine-video` task returned a provider media URL under `https://vidgen.x.ai/...`.
- Canvas correctly blocked the import because the allowlist was empty.
- Result: video generation reached the provider, but the generated video was not saved to R2 `generated/`.
- Current trusted host required for this provider: `vidgen.x.ai`.

Acceptance:

- Arbitrary public URLs cannot be imported.
- Private IP, localhost, metadata, and DNS rebinding attempts are rejected.
- Valid provider-generated video/image URLs still import normally.

### 5. Generated R2 Media URLs Are Public

Status: acceptable for testing, should be changed for production privacy.

Risk:

- Anyone with the generated media URL can view the object until lifecycle deletion.
- Generated media may contain user-provided images, prompts, or private output.

Evidence:

- `infinite-canvas/service/reference_media.go`

Required fix:

- Keep the R2 bucket private for production.
- Return signed URLs instead of permanent public URLs.
- Keep lifecycle cleanup enabled.

Acceptance:

- Generated media is accessible to the owning user.
- A random user without a signed URL cannot read private media.
- Expired signed URLs no longer work.

### 6. Canvas Pages Lack Security Headers

Status: fixed in local Caddy config; verify after deployment.

Risk:

- Canvas pages do not have the same visible security header coverage as TOP-AI pages.
- Missing CSP and framing protections increase XSS and clickjacking impact.

Evidence:

- Header inspection showed TOP-AI pages returning security headers.
- Header inspection did not show equivalent security headers on `/apps/canvas`.

Implemented fix:

- `deploy/Caddyfile` adds CSP, `nosniff`, `DENY`, referrer, and permissions headers for `/apps/canvas*`.

Acceptance:

- `curl -I https://show.top-ai.band/apps/canvas` shows the required headers.
- Canvas still loads scripts, styles, images, and generated media correctly.

## Medium Findings

### 7. Browser Tokens Are Stored In localStorage

Status: known risk.

Risk:

- If any XSS exists, localStorage tokens can be stolen.

Evidence:

- `sub2api/frontend/src/api/auth.ts`
- `infinite-canvas/web/src/services/api/auth.ts`

Required fix:

- Long term: move auth tokens to HttpOnly, Secure, SameSite cookies.
- Short term: reduce XSS surface and add strict CSP.

Acceptance:

- No new feature should introduce unsanitized HTML.
- Auth token storage strategy should be documented before production.

### 8. Home Page Custom HTML Uses v-html

Status: fixed in local code.

Risk:

- Admin-configured custom home HTML can execute script in user browsers if not sanitized.

Evidence:

- `sub2api/frontend/src/views/HomeView.vue`

Implemented fix:

- `HomeView.vue` renders admin-configured custom HTML through DOMPurify.

Acceptance:

- Script tags, event handlers, and dangerous URLs are removed.
- Existing safe custom content still renders.

### 9. Video Content Proxy Reads Large Files Into Memory

Status: fixed in local code; verify with real video generation after deployment.

Risk:

- The previous implementation read the full video response into memory before returning or storing it.
- That could hurt a small server during video generation or download spikes.

Evidence:

- `infinite-canvas/handler/ai.go`

Implemented fix:

- Video content is spooled to a temporary file with the existing max byte limit.
- The temp file is reused for R2 upload and for the browser response.
- R2 failure does not block returning the generated video to the user.

Acceptance:

- Large video responses do not cause high memory spikes.
- Video still appears in Canvas after generation.

### 10. sub2api Backend Port Is Bound To All Interfaces

Status: fixed in local compose defaults; verify the deployed compose/Caddy topology.

Risk:

- Previous compose defaults bound `sub2api` port `8080` to `0.0.0.0`.
- Current cloud firewall appears to block external access, but the safer deployment is internal-only.

Evidence:

- `sub2api/deploy/docker-compose.caddy.yml`
- `sub2api/deploy/docker-compose.local.yml`

Implemented fix:

- Compose defaults now bind the backend host port to `127.0.0.1`.

Acceptance:

- Public traffic enters through Caddy only.
- `sub2api` remains reachable from Caddy.
- Direct external access to backend port is not possible.

### 11. WebDAV Proxy Must Use An Allowlist In Production

Status: partially hardened, still needs production config.

Risk:

- WebDAV proxy has authentication, private IP blocking, and body size limits.
- Code supports a host allowlist, but production must configure a non-empty allowlist.
- If the host allowlist is empty, authenticated users can proxy requests to arbitrary public WebDAV hosts.

Evidence:

- `infinite-canvas/web/src/app/webdav-proxy/route.ts`

Required fix:

- Configure `WEBDAV_PROXY_ALLOWED_HOSTS` in production.
- If WebDAV is not needed, disable the proxy path.
- Do not treat "allowlist support exists in code" as "production allowlist is already configured".

Acceptance:

- Requests to non-allowlisted WebDAV hosts are rejected.
- Requests to approved hosts still work.

## Low Findings

### 12. Public Model Catalog Is Safe For Display But Can Be Scraped

Status: locally hardened; production cache/rate limiting still required.

Risk:

- The public model catalog does not expose keys or upstream account details.
- It can still reveal the public product shelf and can be scraped.

Evidence:

- `sub2api/backend/internal/handler/public_model_catalog_handler.go`
- `sub2api/backend/internal/handler/public_model_catalog_handler_test.go`

Implemented fix:

- Keep the response as a strict public DTO.
- Do not add account id, channel id, group id, upstream base URL, proxy config, health state, balance, or route rules.
- Do not fallback to TOP-AI Gateway `/v1/models` or account `model_mapping` when public catalog data is empty.
- Return an empty catalog when no public sellable data is configured.
- Deduplicate repeated model rows by platform and model name.

Still required:

- Add Cloudflare cache and rate limiting for production.

Acceptance:

- Catalog page can render model cards without login.
- Catalog API cannot be used to call models or consume quota.
- Empty catalog does not leak gateway/account model availability.

### 13. xlsx Dependency Has Known Advisories

Status: low practical risk in current usage.

Risk:

- The dependency scanner reports known advisories for `xlsx`.
- Current usage appears to be export-focused, not parsing untrusted uploads.

Evidence:

- `sub2api/frontend/src/views/admin/UsageView.vue`

Required fix:

- Avoid parsing user-uploaded Excel files with this dependency.
- Replace or upgrade the dependency when practical.

Acceptance:

- Export still works.
- No untrusted xlsx parsing is introduced.

### 14. Logs Include Operational Metadata

Status: monitor.

Risk:

- Logs include request ids, model names, account ids, task ids, and client IPs.
- No API keys were observed in sampled logs, but retention should remain controlled.

Evidence:

- Server log sampling showed operational metadata in logs.
- Server log sampling did not show API keys in the sampled lines.

Required fix:

- Keep key/token redaction.
- Keep log retention limited.
- Do not log full Authorization headers, API keys, proxy credentials, R2 credentials, prompts containing secrets, or signed URLs.

Acceptance:

- Sample logs do not contain secrets.
- Retention policy is documented.

### 15. Server Has No Swap And Limited Memory

Status: availability concern.

Risk:

- The server has limited RAM and no swap.
- Build tasks, video proxying, and concurrent media work can exhaust memory.

Evidence:

- Server resource inspection showed limited memory.
- Server resource inspection showed no swap configured.

Required fix:

- Prefer building images off the production server.
- Add swap or increase instance memory if video traffic grows.

Acceptance:

- Normal web, admin, model catalog, and Canvas pages remain responsive.
- Video generation and media fetch do not make the host unstable.

## Confirmed Safe Or Acceptable Areas

- TOP-AI admin routes require authentication.
- TOP-AI user routes require authentication.
- TOP-AI gateway routes require API key authentication.
- Model catalog endpoint is public display data only.
- Canvas is mounted as a separate service under `/apps/canvas`, not copied into `sub2api/frontend`.
- R2 lifecycle cleanup rules are configured.
- `.env` style files are ignored by Git.
- `webdav-proxy` already blocks localhost, private IP, and metadata addresses, but still needs a production host allowlist.

## Required Fix Order

1. Rotate exposed R2 credentials and Canvas token secret.
2. Fix server env file permissions.
3. Configure `GENERATED_MEDIA_ALLOWED_HOSTS` with trusted provider media domains.
4. Change generated media delivery from public R2 URL to signed URL before production privacy launch.
5. Configure WebDAV proxy allowlist or disable it.
6. Add Cloudflare cache and rate limiting for public catalog.
7. Re-verify Canvas default admin login, security headers, internal backend port binding, home HTML sanitization, video temp-file proxying, and public catalog no-fallback after deployment.
8. Review and replace flagged frontend dependencies when practical.

## Verification Checklist

- [ ] Default Canvas admin credential cannot log in on production.
- [ ] TOP-AI login still opens `/apps/canvas`.
- [ ] Canvas admin access has an explicit production rule.
- [ ] Old R2 credentials are revoked.
- [ ] New R2 credentials are not committed to Git.
- [ ] Server env files are permission `600`.
- [ ] `/apps/canvas` returns CSP, nosniff, frame, referrer, and permissions headers.
- [ ] Arbitrary public media URLs cannot be imported.
- [ ] Valid provider-generated video URLs can still be saved to R2.
- [ ] Generated media access uses private or signed URLs in production.
- [ ] WebDAV proxy rejects non-allowlisted hosts.
- [ ] Large video fetches do not spike process memory.
- [ ] Direct external access to backend port is blocked by deployment design.
- [ ] Model catalog renders public model data without exposing internal account or route data.
- [ ] Empty model catalog does not fallback to TOP-AI Gateway `/v1/models`.
- [ ] Home custom HTML strips dangerous tags, attributes, and URLs.

## Scan Limitations

- Automatic Go vulnerability scanning was not completed because the Go module proxy timed out during tool installation.
- Dependency scanning should be repeated in CI or on a network that can reach the required package registries.
- This report is based on code review, local repository inspection, server deployment inspection, and selected endpoint verification.
