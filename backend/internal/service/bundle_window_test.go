// bundle_window_test.go 套餐滚动窗口过期判定与读时归零的单元测试。
// 覆盖自然日 0 点对齐语义：日(1)/周(7)/月(30) 三窗口的过期边界、零值、读时不修改原值。

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func TestBundleWindowExpired_Daily(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)
	yesterday0 := today0.AddDate(0, 0, -1)

	tests := []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"yesterday 00:00 → expired", yesterday0, true},
		{"today 00:00 → active", today0, false},
		{"today 23:59 → active", today0.Add(23*time.Hour + 59*time.Minute), false},
		{"zero value → expired", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BundleWindowExpired(tt.start, now, BundleDailyWindowDays); got != tt.want {
				t.Fatalf("BundleWindowExpired daily: want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestBundleWindowExpired_Weekly(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)

	if !BundleWindowExpired(today0.AddDate(0, 0, -7), now, BundleWeeklyWindowDays) {
		t.Fatal("7 days ago → weekly window should be expired")
	}
	if BundleWindowExpired(today0.AddDate(0, 0, -6), now, BundleWeeklyWindowDays) {
		t.Fatal("6 days ago → weekly window should still be active")
	}
}

func TestBundleWindowExpired_Monthly(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)

	if !BundleWindowExpired(today0.AddDate(0, 0, -30), now, BundleMonthlyWindowDays) {
		t.Fatal("30 days ago → monthly window should be expired")
	}
	if BundleWindowExpired(today0.AddDate(0, 0, -29), now, BundleMonthlyWindowDays) {
		t.Fatal("29 days ago → monthly window should still be active")
	}
}

func TestBundleWindowStart_AlignsToMidnight(t *testing.T) {
	now := time.Date(2026, 7, 9, 15, 30, 45, 0, time.UTC)
	got := BundleWindowStart(now)
	want := timezone.StartOfDay(now)
	if !got.Equal(want) {
		t.Fatalf("BundleWindowStart should return midnight of today, want %v, got %v", want, got)
	}
	if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
		t.Fatalf("BundleWindowStart must align to 00:00:00, got %v", got)
	}
}

// TestRolledBundleUsage_ZeroesExpiredWindows 验证读时归零：日窗口过期 → 日维度归零；
// 月窗口未过期 → 月维度保留原累计。这是解除死锁的核心——读路径不再把昨天累计当今天用量。
func TestRolledBundleUsage_ZeroesExpiredWindows(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)
	u := &BundleSubscriptionUsage{
		DailyWindowStart:       today0.AddDate(0, 0, -1), // 昨天 → 日过期
		DailyUsageUSD:          2.5,
		DailyImageUsageCount:   3,
		DailyVideoUsageCount:   1,
		MonthlyWindowStart:     today0, // 今天 → 月未过期
		MonthlyUsageUSD:        20.0,
		MonthlyImageUsageCount: 30,
		MonthlyVideoUsageCount: 5,
	}

	got := rolledBundleUsage(u, now)

	if got.DailyUsageUSD != 0 || got.DailyImageUsageCount != 0 || got.DailyVideoUsageCount != 0 {
		t.Fatalf("expired daily window must be zeroed, got usd=%v img=%d vid=%d",
			got.DailyUsageUSD, got.DailyImageUsageCount, got.DailyVideoUsageCount)
	}
	if got.MonthlyUsageUSD != 20.0 || got.MonthlyImageUsageCount != 30 || got.MonthlyVideoUsageCount != 5 {
		t.Fatalf("active monthly window must be preserved, got usd=%v img=%d vid=%d",
			got.MonthlyUsageUSD, got.MonthlyImageUsageCount, got.MonthlyVideoUsageCount)
	}
}

func TestRolledBundleUsage_KeepsAllActiveWindows(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)
	u := &BundleSubscriptionUsage{
		DailyWindowStart:       today0,
		DailyUsageUSD:          1.0,
		WeeklyWindowStart:      today0,
		WeeklyUsageUSD:         5.0,
		MonthlyWindowStart:     today0,
		MonthlyUsageUSD:        20.0,
		MonthlyImageUsageCount: 7,
	}

	got := rolledBundleUsage(u, now)

	if got.DailyUsageUSD != 1.0 || got.WeeklyUsageUSD != 5.0 || got.MonthlyUsageUSD != 20.0 || got.MonthlyImageUsageCount != 7 {
		t.Fatalf("all-active windows must be preserved, got %+v", got)
	}
}

// TestRolledBundleUsage_DoesNotMutateInput 守护「读时归零不写库」契约：归零只作用于返回的拷贝，
// 原记录保持不变（真正的清零只发生在写路径 IncrementUsage 事务内）。
func TestRolledBundleUsage_DoesNotMutateInput(t *testing.T) {
	now := time.Now()
	today0 := timezone.StartOfDay(now)
	u := &BundleSubscriptionUsage{
		DailyWindowStart:     today0.AddDate(0, 0, -1), // 过期
		DailyUsageUSD:        9.9,
		DailyImageUsageCount: 4,
	}

	_ = rolledBundleUsage(u, now)

	if u.DailyUsageUSD != 9.9 || u.DailyImageUsageCount != 4 {
		t.Fatalf("rolledBundleUsage must not mutate input, got usd=%v img=%d", u.DailyUsageUSD, u.DailyImageUsageCount)
	}
}

// TestBundleWindowExpired_AnchoredToPurchaseDay 文档化周/月窗口「锚定购买日、每 N 天滚动」语义。
// ActivateBundle 激活时把 window_start 对齐到购买日 0 点，故：
//   - 周窗口：周一购买 → 本周日（+6 天）未过期，下周一（+7 天）0 点过期重置；
//   - 月窗口：1 号购买 → 30 号（+29 天）未过期，31 号（+30 天）0 点过期重置。
//
// 重置后 window_start 推进到当天 0 点，下一周期继续按 +7/+30 天滚动。
func TestBundleWindowExpired_AnchoredToPurchaseDay(t *testing.T) {
	purchaseDay := timezone.StartOfDay(time.Now()) // 模拟 ActivateBundle 对齐后的购买日 0 点

	// 周窗口：购买后 6 天未过期，第 7 天过期（下周一 0 点重置）。
	if BundleWindowExpired(purchaseDay, purchaseDay.AddDate(0, 0, 6), BundleWeeklyWindowDays) {
		t.Fatal("6 days after purchase: weekly window must NOT be expired (resets at day 7)")
	}
	if !BundleWindowExpired(purchaseDay, purchaseDay.AddDate(0, 0, 7), BundleWeeklyWindowDays) {
		t.Fatal("7 days after purchase: weekly window must be expired (next week same weekday 00:00)")
	}

	// 月窗口：购买后 29 天未过期，第 30 天过期。
	if BundleWindowExpired(purchaseDay, purchaseDay.AddDate(0, 0, 29), BundleMonthlyWindowDays) {
		t.Fatal("29 days after purchase: monthly window must NOT be expired")
	}
	if !BundleWindowExpired(purchaseDay, purchaseDay.AddDate(0, 0, 30), BundleMonthlyWindowDays) {
		t.Fatal("30 days after purchase: monthly window must be expired")
	}

	// 重置后（window_start 推进到第 7 天 0 点）继续滚动：第 14 天再次过期。
	rolledStart := timezone.StartOfDay(purchaseDay.AddDate(0, 0, 7))
	if BundleWindowExpired(rolledStart, rolledStart.AddDate(0, 0, 6), BundleWeeklyWindowDays) {
		t.Fatal("6 days after rollover: weekly window must NOT be expired")
	}
	if !BundleWindowExpired(rolledStart, rolledStart.AddDate(0, 0, 7), BundleWeeklyWindowDays) {
		t.Fatal("14 days after purchase: weekly window must be expired again (rolling every 7 days)")
	}
}
