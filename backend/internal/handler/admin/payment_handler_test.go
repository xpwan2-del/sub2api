package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSanitizeAdminPaymentOrderForResponseAddsCurrency(t *testing.T) {
	now := time.Now()
	order := &dbent.PaymentOrder{
		ID:          1,
		UserID:      2,
		Amount:      100,
		PayAmount:   108,
		FeeRate:     8,
		OutTradeNo:  "sub2_202606250001",
		PaymentType: "stripe",
		OrderType:   "subscription",
		Status:      "COMPLETED",
		ExpiresAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "USD",
		},
	}

	got := sanitizeAdminPaymentOrderForResponse(order)
	if got == nil {
		t.Fatal("expected sanitized order")
	}
	if got.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", got.Currency)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal sanitized order: %v", err)
	}
	if strings.Contains(string(body), "provider_snapshot") {
		t.Fatalf("expected provider_snapshot to be omitted, got %s", string(body))
	}
}

// TestSanitizeAdminPaymentOrderForResponseIncludesBalanceDeduct locks in the
// balance_deduct_amount field on the admin order DTO. Bundle orders that mix
// balance + gateway payment must surface the deducted amount; previously this
// field was missing from the DTO so admin order detail always showed 0.
func TestSanitizeAdminPaymentOrderForResponseIncludesBalanceDeduct(t *testing.T) {
	order := &dbent.PaymentOrder{
		ID:                  1,
		Amount:              100,
		PayAmount:           69.5,
		BalanceDeductAmount: 30.5,
		OrderType:           "bundle",
		PaymentType:         "stripe",
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "USD",
		},
	}

	got := sanitizeAdminPaymentOrderForResponse(order)
	if got == nil {
		t.Fatal("expected sanitized order")
	}
	if got.BalanceDeductAmount != 30.5 {
		t.Fatalf("expected balance_deduct_amount=30.5, got %v", got.BalanceDeductAmount)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal sanitized order: %v", err)
	}
	if !strings.Contains(string(body), `"balance_deduct_amount":30.5`) {
		t.Fatalf("expected JSON to contain balance_deduct_amount, got %s", string(body))
	}
}

// TestSanitizeAdminPaymentOrderForResponseIncludesProrateCredit locks in the
// prorate_credit field on the admin order DTO. bundle_upgrade orders must
// surface the old-bundle prorated credit so admin order detail matches the
// user-facing detail; previously AdminPaymentOrderResult omitted this field,
// so admin detail always showed 0 while the user side showed the real value.
func TestSanitizeAdminPaymentOrderForResponseIncludesProrateCredit(t *testing.T) {
	order := &dbent.PaymentOrder{
		ID:            1,
		Amount:        99,
		PayAmount:     89.1,
		ProrateCredit: 9.9,
		OrderType:     "bundle_upgrade",
		PaymentType:   "alipay",
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "USD",
		},
	}

	got := sanitizeAdminPaymentOrderForResponse(order)
	if got == nil {
		t.Fatal("expected sanitized order")
	}
	if got.ProrateCredit != 9.9 {
		t.Fatalf("expected prorate_credit=9.9, got %v", got.ProrateCredit)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal sanitized order: %v", err)
	}
	if !strings.Contains(string(body), `"prorate_credit":9.9`) {
		t.Fatalf("expected JSON to contain prorate_credit, got %s", string(body))
	}
}

func TestAdminSubscriptionPlansForResponseIncludesCompositeGroupInfo(t *testing.T) {
	weekly := 25.0
	now := time.Now()
	plans := []*dbent.SubscriptionPlan{
		{
			ID:           11,
			GroupID:      7,
			Name:         "All models",
			Description:  "Composite access",
			Price:        19.99,
			Currency:     "CNY",
			ValidityDays: 30,
			ValidityUnit: "days",
			Features:     "OpenAI\nClaude\nGemini\nGrok",
			ProductName:  "Sub2API",
			ForSale:      true,
			SortOrder:    1,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	groupInfo := map[int64]service.PlanGroupInfo{
		7: {
			Platform:       service.PlatformComposite,
			Name:           "Bucket 2 composite",
			RateMultiplier: 1.5,
			WeeklyLimitUSD: &weekly,
			ModelScopes:    []string{"openai", "claude", "gemini", "grok"},
		},
	}

	got := adminSubscriptionPlansForResponse(plans, groupInfo)

	if len(got) != 1 {
		t.Fatalf("expected one plan, got %d", len(got))
	}
	if got[0].GroupPlatform != service.PlatformComposite {
		t.Fatalf("expected composite group platform, got %q", got[0].GroupPlatform)
	}
	if got[0].GroupName != "Bucket 2 composite" {
		t.Fatalf("expected group name to be included, got %q", got[0].GroupName)
	}
	if got[0].WeeklyLimitUSD == nil || *got[0].WeeklyLimitUSD != weekly {
		t.Fatalf("expected weekly limit to be included, got %#v", got[0].WeeklyLimitUSD)
	}
	if strings.Join(got[0].ModelScopes, ",") != "openai,claude,gemini,grok" {
		t.Fatalf("expected model scopes to be preserved, got %#v", got[0].ModelScopes)
	}
	// 投影必须保留 ent 原始响应的全部套餐字段：currency 丢失曾导致编辑保存时
	// 静默清空套餐货币（PlanEditDialog 回传空串 → SetCurrency("")）。
	if got[0].Currency != "CNY" {
		t.Fatalf("expected currency to be preserved, got %q", got[0].Currency)
	}
	if !got[0].CreatedAt.Equal(now) || !got[0].UpdatedAt.Equal(now) {
		t.Fatalf("expected created_at/updated_at to be preserved, got %v / %v", got[0].CreatedAt, got[0].UpdatedAt)
	}
}
