# 同步上游模型价格 — P1 follow-ups & P2 roadmap

## P1 状态
分支 `feat/sync-upstream-model-price`。13 个实现任务 + 跨任务 json-tag fix + final-review fix(I1/I4)全部完成;final whole-branch review 评审:**Ready-with-followups**(无 Critical)。后端 `go test ./internal/service/ ./internal/repository/` + 前端 `pnpm typecheck` 全过。

## P1 follow-ups（非阻塞,ship 后跟进）

| ID | 问题 | 位置 |
|---|---|---|
| I2 | `ReviewItem` pending 检查 + `UpdateItemStatus` 非原子(并发双 apply race) | `service/upstream_price_sync_service.go` + repo |
| I3 | list/get 返回明文 `api_key`(应 mask,对齐 Account 模式) | repo `scanConfig` |
| I5 | `SyncNow` 同 config 并发未锁(spec §9) | service |
| I6 | `request.summary` 不更新、`partially_applied` 不自动流转 | repo/service |
| M1 | `ratio_config` 硬编码 `QuotaType=0`(按次模型误算) | client |
| M2 | `CreateConfig` 返回原始 DB 错误(应包装 BadRequest) | repo |
| M3 | review 路径 `:id` 未与 `item.request_id` 交叉校验 | handler |
| M4 | re-apply 已 applied item 返回错误(spec §9 应 idempotent 成功) | service |
| M5 | 前端双大小写 cruft(json-tag 修复后可清) | `PriceChangeRequestsView` |
| M6 | `getRequest` 前端类型与后端扁平 shape 不符(可工作,仅读 items) | api/handler |
| M7 | 内部错误消息泄漏到 admin 响应 | handler |
| M8 | `DiffPricing` `channelID` 死参 | `upstream_price_sync.go` |
| M9 | `NewRequestWithContext` 错误被吞 | client |
| M10 | `batch-review` 未实现(前端循环单条) | handler/前端 |
| M11 | `loadChannels` 拉 1000 条 | `UpstreamSourcesView` |
| M12 | summary 列恒空(I6 后果) | `PriceChangeRequestsView` |
| — | desktop row-click 不展开(DataTable 既有局限,chevron 可用) | `PriceChangeRequestsView` |
| — | `-tags=unit` 构建时 `testConfig` 在两个 test 文件重复声明(本功能外,预存在) | service test |

## 联调待办（本环境无 DB,验证留联调）
- 迁移 `179_create_upstream_price_sync.sql` 实际跑(PostgreSQL)
- repo 层 DB 集成测试(目前靠编译 + service 集成测试 fakeRepo 覆盖)
- 浏览器端到端:建源 → 立即同步 → 审批(approve/ignore)→ 应用 → 回渠道定价页验证
- 真实 new-api 上游联调(`/api/ratio_config`、`/api/pricing`)
- cache_ratio / create_cache_ratio 语义按上游版本核对(spec §7.1)

## P2 roadmap（spec §12,本 P1 未实现）
1. 分组倍率同步:`group_ratio → Group.rate_multiplier`(经 `group_mapping`)
2. 余额同步(`/api/user/self` + dashboard token)+ 账号详情展示 + 低额告警(扩展 `BalanceNotifyService`)
3. 定时自动化:`timing_wheel`(⚠️ 评估 1h 延迟上限 vs 6h 周期)
4. 模型广场联动:审批应用后自动可见;可选「新增模型」提醒
5. 审批单生成邮件提醒(复用 `NotificationEmailService`)

## 前置条件（spec §11）
- new-api 上游 `/api/pricing` 公开 或 开 `expose_ratio_enabled`(否则需 root token `/api/option`)
- 余额(P2)需 dashboard access token(`sk-xxx` 无法查余额)
- `base_price_per_1k` 默认 0.002(`QuotaPerUnit=500000`)
