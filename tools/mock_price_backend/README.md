# mock_price_backend

一个假上游定价服务，用于端到端测试 sub2api 的**「上游价格同步」**子系统
（`backend/internal/service/upstream_price_sync_service.go` 等）。

它模拟一个 one-api / new-api 系中转站的 `/api/pricing` 接口，能在不改价格时
保持响应字节稳定（`sha256` 判变正确），并能动态改价制造各种变动（涨价/降价/
新增/下架/大变动/微变动），还能注入故障（错误状态码、人为延迟）。

## 快速开始

```bash
cd tools/mock_price_backend
go run .                    # 默认监听 127.0.0.1:9999，无鉴权
go run . -addr :9999 -token secret-sk-xxx -seed 5   # 启用 Bearer 校验
```

启动后打开浏览器访问 `http://127.0.0.1:9999/` 即为控制台，可点击测试。

> 只依赖 Go 标准库，独立 `go.mod`（`module mock_price_backend`），与主项目互不影响。

## 端点

### 被同步端点（受 `-token` 校验）

这些是 sub2api 的 `SyncSource` / `TestConnection` 会拉取的地址：

| 方法 | 路径 | 格式 | 对应 parser_type |
|---|---|---|---|
| GET | `/api/pricing` | one_api（默认） | `one_api` |
| GET | `/api/pricing/litellm` | litellm | `litellm` |
| GET | `/api/pricing/custom` | custom | `custom` |

三种格式对同一组模型产出**完全相同的 per-token 价**（统一从 `model_ratio` 推导），
便于对比三种解析器。one_api：`per_token_in = model_ratio × 2 / 1e6`。

### 控制端点（无鉴权，本地用）

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | `/` | HTML 控制台 |
| GET | `/healthz` | 健康检查 |
| GET | `/admin/state` | 当前模型集 + 请求日志 + 计数（JSON） |
| POST | `/admin/scenario/{name}` | 切换预设场景 |
| POST | `/admin/models` | upsert 模型（body: `model_name`/`model_ratio`/`completion_ratio`） |
| DELETE | `/admin/models/{name}` | 下架模型 |
| POST | `/admin/behaviour` | 故障注入（body: `fail_status`/`delay_ms`） |

### 预设场景

| name | 效果 | 触发的 change 类型 |
|---|---|---|
| `reset` | 重置为 5 个种子模型（基线） | — |
| `hike` | 全部模型 `model_ratio × 1.2` | `price_up`（批量） |
| `cut` | 全部模型 `model_ratio × 0.8` | `price_down`（批量） |
| `add` | 新增 `qwen-max`、`o1-preview` | `new_model` |
| `remove` | 下架末位模型 | `removed` |
| `big` | 首模型 `model_ratio × 1.5` | `price_up`，`+50%` 触发 **critical** 告警 |
| `tiny` | 首模型 `model_ratio × 1.01` | `price_up`，`+1%` 测 `AlertThresholdPct` 过滤 |

## 与 sub2api 对接测试

前提：后端已起（`cd backend && go run ./cmd/server/`），PostgreSQL + Redis 就绪。

### 1. 建立基线（让首次同步拿到初始价）

```bash
# 起 mock（一个终端）
cd tools/mock_price_backend && go run .

# 另一个终端：建价格源，指向 mock 的 one_api 端点
curl -s -X POST http://localhost:PORT/admin/upstream-price/sources \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"mock-one-api",
    "base_url":"http://127.0.0.1:9999",
    "pricing_endpoint":"/api/pricing",
    "parser_type":"one_api",
    "sync_interval_minutes":60,
    "alert_threshold_pct":5,
    "enabled":true
  }'
# 记下返回的 source id（假设为 1）

# 先测连通性
curl -s -X POST http://localhost:PORT/admin/upstream-price/sources/1/test
# → {"reachable":true,"model_count":5}

# 手动同步一次，建立基线快照
curl -s -X POST http://localhost:PORT/admin/upstream-price/sources/1/sync
```

> 首次同步时 `prev` 为空，所有模型算 `new_model` 入库；之后 `last_hash` 已记录，
> 不改价再同步不会产生新变动（`sha256` 判变）。

### 2. 制造变动并验证 diff

```bash
# 在 mock 这边切场景（涨价 20%）
curl -s -X POST http://127.0.0.1:9999/admin/scenario/hike

# 回 sub2api 触发同步
curl -s -X POST http://localhost:PORT/admin/upstream-price/sources/1/sync

# 查看检测到的变动
curl -s 'http://localhost:PORT/admin/upstream-price/changes?status=pending'
# → 应看到 5 条 price_up，input_delta_pct ≈ 0.2（20%）
```

把场景换成 `cut` / `add` / `remove` / `big` / `tiny` 再重复「同步 → 查 changes」，
即可分别验证 `price_down` / `new_model` / `removed` / critical 告警 / 阈值过滤。

### 3. 测 apply / revert / 批量

```bash
# 查某变动的 apply 目标（channels / groups 下拉）
curl -s http://localhost:PORT/admin/upstream-price/changes/1/targets

# follow_cost 应用到某 channel
curl -s -X POST http://localhost:PORT/admin/upstream-price/changes/1/apply \
  -H 'Content-Type: application/json' -d '{"mode":"follow_cost","target_id":<channel_id>}'

# 撤销
curl -s -X POST http://localhost:PORT/admin/upstream-price/changes/1/revert

# 批量 follow_cost
curl -s -X POST http://localhost:PORT/admin/upstream-price/changes/batch-apply-follow-cost \
  -H 'Content-Type: application/json' -d '{"source_id":1}'
```

### 4. 测三种 parser

只改建源时的 `pricing_endpoint` 与 `parser_type`：

| parser_type | pricing_endpoint |
|---|---|
| `one_api` | `/api/pricing` |
| `litellm` | `/api/pricing/litellm` |
| `custom` | `/api/pricing/custom` |

三种应解析出相同的 per-token 价（可对 `upstream_model_prices` 表验证）。

### 5. 测 api_key 鉴权

```bash
# mock 启动带 token
go run . -token my-secret-key

# sub2api 建源时填 api_key=my-secret-key（落库时会被 AES 加密，
# SyncSource 拉取时解密并以 Authorization: Bearer my-secret-key 发出）
```

mock 的请求日志会展示脱敏后的 token（`my***ey`），可确认鉴权链路通了。

### 6. 测故障注入

```bash
# 让定价端点恒返回 500 → sub2api 同步应失败，source.last_sync_status=failed
curl -s -X POST http://127.0.0.1:9999/admin/behaviour \
  -H 'Content-Type: application/json' -d '{"fail_status":500}'

# 让定价端点延迟 35s → 触发 SyncSource 的 30s 超时
curl -s -X POST http://127.0.0.1:9999/admin/behaviour \
  -H 'Content-Type: application/json' -d '{"delay_ms":35000}'

# 清除
curl -s -X POST http://127.0.0.1:9999/admin/behaviour \
  -H 'Content-Type: application/json' -d '{"fail_status":0,"delay_ms":0}'
```

## 命令行参数

| flag | 默认 | 说明 |
|---|---|---|
| `-addr` | `127.0.0.1:9999` | 监听地址 |
| `-token` | 空 | Bearer 校验 token（空=不校验） |
| `-seed` | `5` | 初始种子模型数量 |
