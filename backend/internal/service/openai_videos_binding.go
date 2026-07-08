package service

import (
	"context"
	"strings"
	"time"
)

// VideoTaskBindingTTL 是视频任务绑定（model + 粘性账号）在 redis 中的存活时间。
// 需覆盖视频生成的最长窗口（含排队与重试）再加余量。
const VideoTaskBindingTTL = 2 * time.Hour

// VideoTaskSessionHash 把视频 taskID 派生为粘性会话标识。
// 使用 "video:" 前缀以区别于普通会话哈希（xxhash/sha256），避免键空间冲突。
func VideoTaskSessionHash(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return "video:" + taskID
}

// IsTerminalOpenAIVideosTaskStatus 判断视频任务状态是否为终态
// （成功、失败或取消），后续无需继续轮询。
func IsTerminalOpenAIVideosTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "succeeded", "success",
		"failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

// BindVideoTask 在视频任务创建成功后写入 model 绑定，并把它粘性绑定到创建账号，
// 使后续 GET 查询能恢复 model 并命中同一上游账号。bundleSubID 记录归属订阅（bundle key），
// userID 记录创建者用户（标准 key 跨用户 IDOR 校验用）。标准 Key 的 bundleSubID 为 nil，
// bundle key 两者都写入。
// 写入失败仅返回错误（不阻断已成功的创建响应）。
func (s *OpenAIGatewayService) BindVideoTask(ctx context.Context, groupID *int64, taskID string, accountID int64, model string, bundleSubID, userID *int64, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || accountID <= 0 || s == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = VideoTaskBindingTTL
	}
	if err := s.cache.SetVideoTaskBinding(ctx, derefGroupID(groupID), taskID, VideoTaskBinding{AccountID: accountID, Model: model, BundleSubID: bundleSubID, OwnerUserID: userID}, ttl); err != nil {
		return err
	}
	// 粘性绑定到创建账号：查询时选号链路通过 sessionHash 命中，
	// 并由 tryStickySessionHit 复用可调度性 / 模型支持校验。
	_ = s.setStickySessionAccountID(ctx, groupID, VideoTaskSessionHash(taskID), accountID, ttl)
	return nil
}

// GetVideoTaskModel 读取视频任务创建时记录的 model；未命中返回 ("", false)。
//
// 归属校验（防 IDOR）：
//   - bundle key（bundleSubID 非 nil）：校验 binding.BundleSubID 匹配（订阅级隔离），拦截
//     跨订阅越权查询。
//   - 标准 key（bundleSubID 为 nil，userID 非 nil）：校验 binding.OwnerUserID 匹配（用户级
//     隔离），拦截同一 group 下不同用户的标准 key 跨用户越权查询。
//
// 该校验补齐了带 ?model= 的 GET 走 model 路由路径（中间件 ResolveGroup 不校验 task
// 归属）的缺口，与无 model 反查路径（ResolveGroupByVideoTask）语义统一。
func (s *OpenAIGatewayService) GetVideoTaskModel(ctx context.Context, groupID *int64, taskID string, bundleSubID, userID *int64) (string, bool) {
	if s == nil || s.cache == nil {
		return "", false
	}
	binding, err := s.cache.GetVideoTaskBinding(ctx, derefGroupID(groupID), strings.TrimSpace(taskID))
	if err != nil || strings.TrimSpace(binding.Model) == "" {
		return "", false
	}
	if bundleSubID != nil {
		// bundle key：订阅级归属校验。binding.BundleSubID 为 nil（标准 key 创建）或不匹配
		// → 视为未命中，拦截跨订阅 IDOR。
		if binding.BundleSubID == nil || *binding.BundleSubID != *bundleSubID {
			return "", false
		}
		return strings.TrimSpace(binding.Model), true
	}
	// 标准 key：用户级归属校验（fail-closed）。binding.OwnerUserID 为 nil（迁移前旧数据，
	// VideoTaskBindingTTL 内过期）或 userID 未提供/不匹配 → 一律拒绝，避免遗留绑定或调用方
	// 漏传 userID 绕过跨用户 IDOR 防护。
	if binding.OwnerUserID == nil || userID == nil || *binding.OwnerUserID != *userID {
		return "", false
	}
	return strings.TrimSpace(binding.Model), true
}

// UnbindVideoTask 在任务到达终态（或内容已取回）后清理 model 绑定与粘性会话。
func (s *OpenAIGatewayService) UnbindVideoTask(ctx context.Context, groupID *int64, taskID string) {
	if s == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	_ = s.cache.DeleteVideoTaskBinding(ctx, derefGroupID(groupID), taskID)
	_ = s.deleteStickySessionAccountID(ctx, groupID, VideoTaskSessionHash(taskID))
}
