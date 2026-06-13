package service

// SuggestionMode 标识建议值的调价策略。
type SuggestionMode string

const (
	// SuggestionFollowCost 跟随成本：单价更新为上游最新价，倍率不变（维持毛利率%）。
	SuggestionFollowCost SuggestionMode = "follow_cost"
	// SuggestionLockPrice 锁死售价：单价更新为上游价，倍率反推以维持对用户售价不变。
	SuggestionLockPrice SuggestionMode = "lock_price"
)

// SuggestionInput 是建议值计算的输入。
type SuggestionInput struct {
	OldInputPrice     float64 // 上游旧成本单价（per-token USD）
	NewInputPrice     float64 // 上游新成本单价（per-token USD）
	CurrentMultiplier float64 // 该模型相关 group 的当前倍率（rate_multiplier）
}

// Suggestion 是两种调价策略的建议值。
//
// SuggestedInputPrice 两种模式都用上游新成本作单价。
// SuggestedMultiplier 仅在能反推时（old/new 成本都 >0 且当前倍率 >0）非 nil：
//   lock_price 新倍率 = 旧倍率 × (旧成本/新成本)，使对用户售价不变。
type Suggestion struct {
	Mode                SuggestionMode
	SuggestedInputPrice float64  // 建议单价（per-token USD）
	SuggestedMultiplier *float64 // 建议倍率（仅 lock_price 可用；nil 表示无法反推）
}

// CalcSuggestion 根据上游成本变化计算建议值。
//
// 售价 ∝ 单价 × 倍率。维持售价不变 ⟺ 新单价×新倍率 = 旧单价×旧倍率，
// 故新倍率 = 旧倍率 × (旧成本/新成本)。成本涨 → 倍率降（毛利压缩）；成本降 → 倍率升。
func CalcSuggestion(in SuggestionInput) Suggestion {
	s := Suggestion{Mode: SuggestionFollowCost, SuggestedInputPrice: in.NewInputPrice}
	if in.OldInputPrice > priceEpsilon && in.NewInputPrice > priceEpsilon && in.CurrentMultiplier > 0 {
		m := in.CurrentMultiplier * (in.OldInputPrice / in.NewInputPrice)
		s.SuggestedMultiplier = &m
	}
	return s
}
