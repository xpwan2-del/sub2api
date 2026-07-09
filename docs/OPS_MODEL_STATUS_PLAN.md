# Model Status Health Dashboard Plan

## Purpose

Build an admin-only model health dashboard that answers:

- Is the server healthy?
- Is the API gateway healthy?
- Are configured/sold models healthy based on real traffic?

This page follows the confirmed CPA/status-page method: read existing operational signals, aggregate them into health history, and render the result as status cards. It must not actively call every model to prove availability.

## Delivery Rule

Implement this as one complete update.

The complete update includes:

- Backend structured-log aggregation.
- Health bucket/status calculation.
- Admin snapshot API response.
- CPA-style dashboard cards using sub2api visual styles.
- Silent refresh behavior.
- Focused backend/frontend tests.

Do not leave the health history, card layout, or refresh fix for later work. Optional probes are intentionally excluded from this plan.

## Confirmed Reference Method

The local CPA status page in `/Users/xiaodoubao/Documents/cpa` uses a lightweight passive method:

- `server.js` SSHes into the VPS and reads `docker logs --since 5m`.
- It parses access-log lines into status code, duration, method, and path.
- It aggregates route request count, success count, failed count, average latency, P50, P95, and P99.
- The frontend polls `/api/live` every 5 seconds and updates cards without reloading the page.

That local CPA page does not actively call models and does not spend model tokens.

There is also a separate `cpa-usage-keeper` container in the CPA folder. Its source code is not present locally, so only the local status page behavior above is treated as confirmed.

## CPA Logic Mapped To sub2api

CPA and sub2api can produce the same dashboard behavior, but from different data transports:

```text
CPA log line status code        -> sub2api usage_logs success rows + ops_error_logs status_code
CPA log line duration           -> sub2api usage_logs.duration_ms + ops_error_logs.duration_ms
CPA route path                  -> sub2api inbound_endpoint / upstream_endpoint
CPA route success rate          -> SQL count(success) / count(total)
CPA P50/P95/P99                 -> SQL percentile_cont over duration_ms
CPA 5-minute live payload       -> sub2api recent-window SQL aggregate
CPA 48h health blocks           -> sub2api fixed time buckets from usage_logs + ops_error_logs
CPA frontend in-place update    -> Vue snapshot refresh without global loading
```

Therefore the required data source exists in sub2api. We should use the existing structured tables instead of adding Docker-log parsing.

## sub2api Data Source

sub2api should not copy CPA's SSH/docker-log approach. Our project already has structured request data in PostgreSQL, which is more reliable than parsing text logs.

Use these existing tables:

- `usage_logs`: successful/completed billed requests.
- `ops_error_logs`: failed requests and upstream errors recorded by ops middleware.

Relevant existing fields:

- Time: `created_at`
- Model: `model`, `requested_model`, `upstream_model`
- Provider/platform: `platform` on `ops_error_logs`, and account/group platform joins for `usage_logs`
- Endpoint: `inbound_endpoint`, `upstream_endpoint`
- Latency: `duration_ms`
- Status/error: `status_code`, `upstream_status_code`, `error_type`
- Tokens: `input_tokens`, `output_tokens`, `cache_creation_tokens`, `cache_read_tokens`, image token fields

Local dev database confirmation:

- `usage_logs` exists and contains request rows.
- `ops_error_logs` exists and contains error rows.
- Current local data is old, so the page may show idle/no recent traffic until new requests are made.

## Cost Rule

The dashboard must be token-free by default.

Do not implement:

- Full-model polling.
- Scheduled generation probes.
- "Call every model every N seconds" checks.
- Frontend requests to upstream providers.
- Frontend requests to Google Cloud APIs.

The health dashboard reads existing logs and server metrics only. No extra model token cost is allowed for the normal refresh path.

## Health Semantics

This page is about health, not raw account availability.

Account availability is only a supporting metric. The main health indicator must come from real request results:

```text
health = success/failure buckets + latency + recent error signal
```

Bucket status rules:

```text
total == 0                     -> idle
success > 0 && failed == 0     -> operational
success > 0 && failed > 0      -> degraded
success == 0 && failed > 0     -> failed
high latency despite success   -> degraded
```

Important:

- `idle` means no sample, not failure.
- No-sample models should not show health 0%.
- Availability/account count must not be displayed as the main health bar.

## Backend Implementation

Keep the implementation inside the existing ops architecture.

### Repository

Use:

```text
backend/internal/repository/ops_repo_model_status.go
```

Add read-only aggregation methods for model/provider/gateway health history.

Expected repository responsibilities:

- Query `usage_logs` for successful requests.
- Query `ops_error_logs` for failed requests and upstream errors.
- Group by provider/model/endpoint.
- Generate fixed time buckets, for example 48 hourly buckets.
- Return empty buckets for periods with no traffic.
- Calculate request totals, success totals, error totals, token totals, average latency, and P95 latency.

Use PostgreSQL aggregation, not per-row Go loops over large result sets.

Do not add database tables or migrations for this implementation.

### Service

Use:

```text
backend/internal/service/ops_model_status.go
backend/internal/service/ops_model_status_models.go
```

Expected service responsibilities:

- Normalize provider/model keys.
- Merge configured model inventory with real traffic.
- Convert repository buckets into health bucket states.
- Calculate current health status and optional health score.
- Keep account availability as supporting data only.
- Return one snapshot payload for the admin page.

Snapshot response should include:

```text
generated_at
window
cloud_metrics
gateway_summary
provider_health
model_health
account_availability
recent_errors
pagination
```

### Handler And Route

Use:

```text
backend/internal/handler/admin/ops_model_status_handler.go
backend/internal/server/routes/admin.go
```

Keep the endpoint:

```text
GET /api/v1/admin/ops/model-status/snapshot
```

The handler must only parse filters and call `OpsService`. It must not contain SQL or health classification logic.

## Frontend Implementation

Use the existing admin ops page:

```text
frontend/src/views/admin/ops/ModelStatusDashboard.vue
```

The page should follow CPA's layout style and information hierarchy, but not copy CPA's colors. Use the existing sub2api admin design system, Tailwind tokens, `card`/`btn`/`input` classes, existing border radius, dark-mode behavior, and the project's current status colors.

CPA style elements to follow:

- Top summary strip: overall health, updated time, recent requests, recent tokens, error count.
- Server metrics card: CPU, memory, disk, network, uptime.
- Gateway health card: recent request count, success rate, P50/P95/P99, route/endpoint list.
- Provider cards: provider health status, 48h health blocks, request count, success rate, error count, P95.
- Model cards: model health status, health blocks, request count, success/error, latency, last seen.
- Detail table remains secondary for searching and auditing.

sub2api visual rules:

- Use project colors, not CPA's custom palette.
- Keep cards consistent with existing admin cards.
- Use existing green/yellow/red/gray status tones already used by the project.
- Keep compact dashboard density like CPA, but avoid foreign-looking decorative styles.
- Support light and dark mode with existing Tailwind dark classes.
- Do not introduce a separate CSS theme just for this page.

Add at most one shared visual component for health buckets:

```text
frontend/src/views/admin/ops/components/OpsHealthHistoryBar.vue
```

Existing card components may be corrected instead of adding more files:

```text
OpsProviderStatusCards.vue
OpsModelStatusCards.vue
OpsModelStatusSummary.vue
OpsCloudMetricsCard.vue
OpsModelStatusTable.vue
```

## Refresh Behavior

Auto refresh must be silent.

Do not set the whole page into loading state during background refresh. The current issue where the page looks like it globally refreshes should be fixed by separating:

- initial loading
- manual refresh loading
- silent background refresh

The refresh path should update the snapshot data in place.

Recommended refresh interval:

- 15 seconds for admin UI.
- Backend reads only DB/metrics, no model calls.

## Google Server Metrics

Google Cloud Monitoring is only for server/VM health:

- CPU
- memory
- disk
- network
- uptime

Google metrics must not decide model health by themselves.

If Google Cloud Monitoring is not configured or fails:

- `cloud_metrics.status` should become `disabled`, `not_configured`, or `error`.
- Model and gateway health should still render from `usage_logs` and `ops_error_logs`.
- Business traffic must not be affected.

## File Boundaries

Allowed backend files:

```text
backend/internal/repository/ops_repo_model_status.go
backend/internal/repository/ops_repo_model_status_test.go
backend/internal/service/ops_model_status.go
backend/internal/service/ops_model_status_models.go
backend/internal/service/ops_model_status_test.go
backend/internal/service/ops_google_cloud_metrics.go
backend/internal/service/ops_port.go
backend/internal/handler/admin/ops_model_status_handler.go
backend/internal/server/routes/admin.go
backend/internal/config/config.go
deploy/config.example.yaml
```

Allowed frontend files:

```text
frontend/src/api/admin/ops.ts
frontend/src/views/admin/ops/ModelStatusDashboard.vue
frontend/src/views/admin/ops/components/OpsCloudMetricsCard.vue
frontend/src/views/admin/ops/components/OpsModelStatusSummary.vue
frontend/src/views/admin/ops/components/OpsProviderStatusCards.vue
frontend/src/views/admin/ops/components/OpsModelStatusCards.vue
frontend/src/views/admin/ops/components/OpsModelStatusTable.vue
frontend/src/views/admin/ops/components/OpsHealthHistoryBar.vue
frontend/src/router/index.ts
frontend/src/components/layout/AppSidebar.vue
frontend/src/i18n/locales/zh.ts
frontend/src/i18n/locales/en.ts
```

Do not add:

```text
backend/ent/schema/*
backend/migrations/*
new top-level directories
frontend global stores for this page
frontend direct DB/provider/Google access
```

Coding standards:

- Keep SQL in repository files only.
- Keep health classification in service files only.
- Keep handlers thin: parse request, call service, return response.
- Keep Vue components under the existing admin ops directory.
- Reuse existing API client, i18n, router, sidebar, and layout patterns.
- Do not introduce unrelated refactors while implementing this feature.
- Do not move existing modules or create a new feature root.

## Expected Backend Query Shape

The health history query should conceptually do this:

```text
1. generate fixed buckets for the requested window
2. aggregate usage_logs into success counts and latency per bucket
3. aggregate ops_error_logs into failure counts per bucket
4. full/left join the aggregates onto the fixed bucket list
5. return idle buckets when no rows exist
```

This mirrors the CPA method, but uses structured tables instead of parsing Docker logs.

## UI State Mapping

Use these colors consistently:

```text
operational -> green
degraded    -> yellow/orange
failed      -> red
idle        -> gray
unknown     -> gray
```

Tooltips for each bucket should show:

```text
time range
status
requests
success
failed
success rate
avg/p95 latency when available
```

## Acceptance Criteria

- The dashboard does not trigger upstream model calls.
- The dashboard can render with only `usage_logs` and `ops_error_logs`.
- No-traffic models show idle/no recent traffic, not failed.
- Health cards use real success/failure/latency buckets, not account availability.
- Account availability is shown as secondary context.
- Background refresh does not blank or reload the whole page.
- Google metrics failure does not break model/gateway health.
- Tests cover bucket classification and repository aggregation.

## Verification Commands

Run focused backend tests:

```bash
cd backend
go test ./internal/service -run 'TestOpsServiceGetModelStatusSnapshot|Test.*ModelStatus'
go test ./internal/repository -run 'TestOpsRepositoryGetModelTrafficStats|Test.*ModelStatus'
go test ./internal/handler/admin -run 'Test.*ModelStatus|TestOps'
```

Run frontend checks:

```bash
cd frontend
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/vite build
```

Run formatting safety check:

```bash
git diff --check
```

## Final Decision

The implementation should follow the CPA idea, not CPA's exact transport:

```text
CPA local page: docker logs -> parser -> route health -> cards
sub2api: usage_logs + ops_error_logs -> SQL buckets -> model/gateway health -> cards
```

This is the confirmed low-cost path.
