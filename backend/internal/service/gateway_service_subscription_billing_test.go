//go:build unit

package service

import (
	"testing"
)

// TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier locks in the fix
// that subscription-mode billing honours the group (and any user-specific) rate
// multiplier — i.e. cmd.SubscriptionCost tracks ActualCost (= TotalCost *
// RateMultiplier), not raw TotalCost.
func TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	subID := int64(42)

	tests := []struct {
		name           string
		totalCost      float64
		actualCost     float64
		isSubscription bool
		wantSub        float64
		wantBalance    float64
	}{
		{
			name:           "subscription with 2x multiplier consumes 2x quota",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: true,
			wantSub:        2.0,
			wantBalance:    0,
		},
		{
			name:           "subscription with 0.5x multiplier consumes 0.5x quota",
			totalCost:      1.0,
			actualCost:     0.5,
			isSubscription: true,
			wantSub:        0.5,
			wantBalance:    0,
		},
		{
			name:           "free subscription (multiplier 0) consumes no quota",
			totalCost:      1.0,
			actualCost:     0,
			isSubscription: true,
			wantSub:        0,
			wantBalance:    0,
		},
		{
			name:           "balance billing keeps using ActualCost (regression)",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: false,
			wantSub:        0,
			wantBalance:    2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &postUsageBillingParams{
				Cost:               &CostBreakdown{TotalCost: tt.totalCost, ActualCost: tt.actualCost},
				User:               &User{ID: 1},
				APIKey:             &APIKey{ID: 2, GroupID: &groupID},
				Account:            &Account{ID: 3},
				Subscription:       &UserSubscription{ID: subID},
				IsSubscriptionBill: tt.isSubscription,
				ImageCount:         0,
				VideoCount:         0,
			}

			cmd := buildUsageBillingCommand("req-1", nil, p)
			if cmd == nil {
				t.Fatal("buildUsageBillingCommand returned nil")
			}
			if cmd.SubscriptionCost != tt.wantSub {
				t.Errorf("SubscriptionCost = %v, want %v", cmd.SubscriptionCost, tt.wantSub)
			}
			if cmd.BalanceCost != tt.wantBalance {
				t.Errorf("BalanceCost = %v, want %v", cmd.BalanceCost, tt.wantBalance)
			}
		})
	}
}

// TestPostUsageBillingParams_ShouldAccumulateBundleUsage 锁定：按次计费与成本解耦——
// ActualCost=0 但有媒体产出（ImageCount/VideoCount>0）时仍累加套餐用量，否则免费/低价媒体的次数限额失效。
func TestPostUsageBillingParams_ShouldAccumulateBundleUsage(t *testing.T) {
	t.Parallel()

	bundleSubID := int64(42)
	groupID := int64(7)
	bundleSub := &UserSubscription{ID: 1, GroupID: groupID, BundleSubscriptionID: &bundleSubID}
	plainSub := &UserSubscription{ID: 1, GroupID: groupID} // 非 bundle 订阅

	tests := []struct {
		name        string
		sub         *UserSubscription
		actualCost  float64
		imageCount  int
		videoCount  int
		want        bool
	}{
		{"cost only", bundleSub, 1.0, 0, 0, true},
		{"image count only (free media)", bundleSub, 0.0, 2, 0, true},   // Bug3 核心
		{"video count only (free media)", bundleSub, 0.0, 0, 1, true},   // 视频维度同理解耦
		{"both cost and image count", bundleSub, 1.5, 3, 0, true},
		{"both image and video count", bundleSub, 0.0, 2, 1, true},
		{"neither cost nor count", bundleSub, 0.0, 0, 0, false},
		{"non-bundle subscription", plainSub, 1.0, 2, 1, false},
		{"nil subscription", nil, 1.0, 2, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &postUsageBillingParams{
				Cost:         &CostBreakdown{ActualCost: tt.actualCost},
				Subscription: tt.sub,
				ImageCount:   tt.imageCount,
				VideoCount:   tt.videoCount,
			}
			if got := p.shouldAccumulateBundleUsage(); got != tt.want {
				t.Errorf("shouldAccumulateBundleUsage() = %v, want %v (actualCost=%v imageCount=%d videoCount=%d)", got, tt.want, tt.actualCost, tt.imageCount, tt.videoCount)
			}
		})
	}
}
