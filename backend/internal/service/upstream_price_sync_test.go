package service

import (
	"math"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestConvertPricing_RatioMode(t *testing.T) {
	// model_ratio=1.5, completion_ratio=2, cache_ratio=0.5, base=0.002
	m := UpstreamModelPricing{
		ModelName: "claude-test", ModelRatio: 1.5, CompletionRatio: 2, QuotaType: 0,
		CacheRatio: floatPtr(0.5), CreateCacheRatio: floatPtr(1.25),
	}
	got := ConvertPricing(m, 0.002)
	// input = 1.5 * 0.002 / 1000 = 0.000003
	if math.Abs(*got.InputPrice-0.000003) > 1e-12 {
		t.Fatalf("input = %v, want 0.000003", *got.InputPrice)
	}
	// output = input * 2
	if math.Abs(*got.OutputPrice-0.000006) > 1e-12 {
		t.Fatalf("output = %v, want 0.000006", *got.OutputPrice)
	}
	// cache_read = input * 0.5
	if math.Abs(*got.CacheReadPrice-0.0000015) > 1e-12 {
		t.Fatalf("cache_read = %v, want 0.0000015", *got.CacheReadPrice)
	}
	if got.BillingMode != BillingModeToken {
		t.Fatalf("mode = %v, want token", got.BillingMode)
	}
}

func TestConvertPricing_PerRequestMode(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", QuotaType: 1, ModelPrice: floatPtr(0.05)}
	got := ConvertPricing(m, 0.002)
	if got.PerRequestPrice == nil || math.Abs(*got.PerRequestPrice-0.05) > 1e-12 {
		t.Fatalf("per_request = %v, want 0.05", got.PerRequestPrice)
	}
	if got.BillingMode != BillingModePerRequest {
		t.Fatalf("mode = %v, want per_request", got.BillingMode)
	}
}

func TestConvertPricing_NilCache(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", ModelRatio: 1, CompletionRatio: 1, QuotaType: 0}
	got := ConvertPricing(m, 0.002)
	if got.CacheReadPrice != nil || got.CacheWritePrice != nil {
		t.Fatalf("cache should be nil when upstream ratio nil")
	}
}

func TestInferPlatform(t *testing.T) {
	cases := map[string]string{
		"gpt-4o": PlatformOpenAI, "chatgpt-4o-latest": PlatformOpenAI, "o3-mini": PlatformOpenAI,
		"claude-sonnet-4":  PlatformAnthropic,
		"gemini-2.0-flash": PlatformGemini,
		"grok-2":           PlatformGrok,
		"unknown-model":    "",
	}
	for name, want := range cases {
		if got := InferPlatform(name); got != want {
			t.Errorf("InferPlatform(%q) = %q, want %q", name, got, want)
		}
	}
}
