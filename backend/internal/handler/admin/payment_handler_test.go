package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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
