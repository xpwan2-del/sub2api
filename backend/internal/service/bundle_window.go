// bundle_window.go 套餐用量滚动窗口的过期判定与读时归零。
//
// 日/周/月三个窗口统一采用「自然日 0 点对齐」语义：
//   - 日窗口：start 所在自然日不是今天（已跨过当天 0 点）即过期，次日 0 点起视为新窗口；
//   - 周/月窗口：start 所在自然日距今满 7 / 30 个自然日即过期。
// 过期后窗口起点推进到「now 所在自然日 0 点」，因此重置永远落在 0 点。
//
// 关键不变量：读路径（CheckQuotaEligibility / GetBundleUsageProgress）与写路径
// （IncrementUsage → rollBundleWindow）必须共用 BundleWindowExpired，保证读写判定逐字一致。
// 否则会出现「读判过期放行、写判未过期把新用量累加到旧值上」的计费偏差。
//
// 本模块同时承载死锁修复：历史上窗口重置只挂在写路径，读路径不认过期，导致「达到日限额后
// pre-flight 永远拒绝 → 永远进不到写路径 → 窗口永不重置」的自锁。读路径经 rolledBundleUsage
// 在读取即归零，次日 0 点恢复额度、请求得以放行进入写路径完成真正清零，死锁解除。

package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// 滚动窗口天数（自然日 0 点对齐）：日=1 / 周=7 / 月=30。
// window_start 所在自然日距 now 所在自然日达到该天数即视为窗口过期。
const (
	BundleDailyWindowDays   = 1
	BundleWeeklyWindowDays  = 7
	BundleMonthlyWindowDays = 30
)

// BundleWindowExpired 判断滚动窗口是否已过期（自然日 0 点对齐语义）。
//   - start 零值视为过期（与历史 rollBundleWindow 的 now.Sub(zero)>=dur 行为一致），
//     保证读路径对缺失 window_start 的历史行也归零，避免把陈旧累计当成有效用量；
//   - windowDays 为窗口天数（BundleDailyWindowDays / BundleWeeklyWindowDays / BundleMonthlyWindowDays）。
//
// 判定基于「start 所在自然日」与「now 所在自然日」的天数差，不依赖 start 是否精确对齐 0 点，
// 因此对历史遗留的、window_start 为请求时刻（非 0 点）的数据同样正确。
func BundleWindowExpired(start, now time.Time, windowDays int) bool {
	if start.IsZero() {
		return true
	}
	startDay := timezone.StartOfDay(start)
	today := timezone.StartOfDay(now)
	diffDays := int(today.Sub(startDay) / (24 * time.Hour))
	return diffDays >= windowDays
}

// BundleWindowStart 返回窗口重置时应推进到的起点：now 所在自然日的 0 点。
// 写路径 IncrementUsage 过期清零时用本函数设置新 window_start，保证 DB 中 window_start
// 始终对齐到自然日 0 点（语义清晰、下次重置时刻落在 0 点）。
func BundleWindowStart(now time.Time) time.Time {
	return timezone.StartOfDay(now)
}

// rolledBundleUsage 对一条用量记录应用「读时归零」：已过期窗口的累计视为 0（不写 DB）。
// 返回归零后的拷贝，不修改传入的原记录。供 CheckQuotaEligibility（额度判定）与
// GetBundleUsageProgress（前端展示）共用，与写路径 IncrementUsage 的过期判定逐字一致。
//
// 这是解除「达到限额后窗口永不重置」死锁的关键：读路径不再把昨天的陈旧累计当成今天的用量，
// 次日 0 点起即恢复额度，请求得以放行进入写路径完成真正的清零。
func rolledBundleUsage(u *BundleSubscriptionUsage, now time.Time) BundleSubscriptionUsage {
	out := *u
	if BundleWindowExpired(u.DailyWindowStart, now, BundleDailyWindowDays) {
		out.DailyUsageUSD = 0
		out.DailyImageUsageCount = 0
		out.DailyVideoUsageCount = 0
	}
	if BundleWindowExpired(u.WeeklyWindowStart, now, BundleWeeklyWindowDays) {
		out.WeeklyUsageUSD = 0
		out.WeeklyImageUsageCount = 0
		out.WeeklyVideoUsageCount = 0
	}
	if BundleWindowExpired(u.MonthlyWindowStart, now, BundleMonthlyWindowDays) {
		out.MonthlyUsageUSD = 0
		out.MonthlyImageUsageCount = 0
		out.MonthlyVideoUsageCount = 0
	}
	return out
}
