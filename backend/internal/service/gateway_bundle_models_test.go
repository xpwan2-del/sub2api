// gateway_bundle_models_test.go 套餐 Key 拉取可用模型的汇总逻辑测试。
// 覆盖 GetBundleAvailableModels：多 group 模型汇总去重、model scope 按 pattern 收窄、空/缺失订阅兜底。

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetBundleAvailableModels_AggregatesAndFiltersByPattern(t *testing.T) {
	plan := &BundlePlan{
		ID: 1,
		GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 100, GroupPlatform: PlatformOpenAI, QuotaScope: QuotaScopePlatform},
			{GroupID: 200, GroupPlatform: PlatformAnthropic, QuotaScope: QuotaScopeModel, ModelPattern: "claude-*"},
		},
	}
	sub := &BundleSubscription{ID: 9, PlanID: 1, Status: BundleStatusActive}

	repo := &modelsListAccountRepoStub{
		byGroup: map[int64][]Account{
			100: {{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-4o": "gpt-4o", "gpt-4o-mini": "gpt-4o-mini"},
				},
			}},
			200: {{
				ID:       2,
				Platform: PlatformAnthropic,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"claude-sonnet-4": "claude-sonnet-4",
						"claude-opus-4":   "claude-opus-4",
						"gpt-4o":          "gpt-4o", // 不匹配 claude-*，model scope 下应被过滤
					},
				},
			}},
		},
	}

	bus := NewBundleUsageService(nil, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})
	svc := &GatewayService{
		accountRepo:        repo,
		bundleUsageService: bus,
	}

	got := svc.GetBundleAvailableModels(context.Background(), 9)
	// group100(platform scope) 全量 → gpt-4o, gpt-4o-mini
	// group200(model scope, claude-*) → claude-sonnet-4, claude-opus-4（gpt-4o 被过滤）
	// 汇总去重排序
	require.Equal(t, []string{"claude-opus-4", "claude-sonnet-4", "gpt-4o", "gpt-4o-mini"}, got)
}

func TestGetBundleAvailableModels_NoQuotasReturnsNil(t *testing.T) {
	plan := &BundlePlan{ID: 1, GroupQuotas: nil}
	sub := &BundleSubscription{ID: 9, PlanID: 1, Status: BundleStatusActive}
	bus := NewBundleUsageService(nil, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})
	svc := &GatewayService{
		accountRepo:        &modelsListAccountRepoStub{},
		bundleUsageService: bus,
	}

	got := svc.GetBundleAvailableModels(context.Background(), 9)
	require.Nil(t, got)
}

func TestGetBundleAvailableModels_SubscriptionMissingReturnsNil(t *testing.T) {
	// 订阅不存在 → GetBundlePlan 返回 error → 返回 nil，不 panic
	bus := NewBundleUsageService(nil, &fakeSubRepo{sub: nil}, &fakePlanRepo{plan: nil})
	svc := &GatewayService{
		accountRepo:        &modelsListAccountRepoStub{},
		bundleUsageService: bus,
	}

	got := svc.GetBundleAvailableModels(context.Background(), 999)
	require.Nil(t, got)
}
