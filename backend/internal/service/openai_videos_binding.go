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
// 使后续 GET 查询能恢复 model 并命中同一上游账号。bundleSubID 记录归属订阅，供 bundle
// key 的 GET 反查做 scope 校验（标准 Key 传 nil）。
// 写入失败仅返回错误（不阻断已成功的创建响应）。
func (s *OpenAIGatewayService) BindVideoTask(ctx context.Context, groupID *int64, taskID string, accountID int64, model string, bundleSubID *int64, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || accountID <= 0 || s == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = VideoTaskBindingTTL
	}
	if err := s.cache.SetVideoTaskBinding(ctx, derefGroupID(groupID), taskID, VideoTaskBinding{AccountID: accountID, Model: model, BundleSubID: bundleSubID}, ttl); err != nil {
		return err
	}
	// 粘性绑定到创建账号：查询时选号链路通过 sessionHash 命中，
	// 并由 tryStickySessionHit 复用可调度性 / 模型支持校验。
	_ = s.setStickySessionAccountID(ctx, groupID, VideoTaskSessionHash(taskID), accountID, ttl)
	return nil
}

// GetVideoTaskModel 读取视频任务创建时记录的 model；未命中返回 ("", false)。
func (s *OpenAIGatewayService) GetVideoTaskModel(ctx context.Context, groupID *int64, taskID string) (string, bool) {
	if s == nil || s.cache == nil {
		return "", false
	}
	binding, err := s.cache.GetVideoTaskBinding(ctx, derefGroupID(groupID), strings.TrimSpace(taskID))
	if err != nil || strings.TrimSpace(binding.Model) == "" {
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
