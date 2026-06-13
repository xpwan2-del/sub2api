package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestion_FollowCost(t *testing.T) {
	s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000036, CurrentMultiplier: 1.5})
	assert.Equal(t, SuggestionFollowCost, s.Mode)
	assert.InDelta(t, 0.000036, s.SuggestedInputPrice, 1e-12) // = 新成本
}

func TestSuggestion_LockPriceMath(t *testing.T) {
	// 售价不变: oldCost*mult = newCost*newMult
	// 0.00003 * 1.5 = 0.000045; newMult = 0.000045 / 0.000036 = 1.25
	s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000036, CurrentMultiplier: 1.5})
	require.NotNil(t, s.SuggestedMultiplier)
	assert.InDelta(t, 1.25, *s.SuggestedMultiplier, 1e-6)
}

func TestSuggestion_LockPriceCostDown(t *testing.T) {
	// 成本降 → 倍率应上升（维持售价不变则毛利扩大）
	s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000024, CurrentMultiplier: 1.5})
	require.NotNil(t, s.SuggestedMultiplier)
	assert.Greater(t, *s.SuggestedMultiplier, 1.5)
	// 验证数学: 1.5 * 0.00003/0.000024 = 1.5 * 1.25 = 1.875
	assert.InDelta(t, 1.875, *s.SuggestedMultiplier, 1e-6)
}

func TestSuggestion_ZeroOldCost_NoMultiplier(t *testing.T) {
	// old=0 无法算倍率比 → SuggestedMultiplier 为 nil（防除零）
	s := CalcSuggestion(SuggestionInput{OldInputPrice: 0, NewInputPrice: 0.00003, CurrentMultiplier: 1.5})
	assert.Equal(t, SuggestionFollowCost, s.Mode)
	assert.InDelta(t, 0.00003, s.SuggestedInputPrice, 1e-12)
	assert.Nil(t, s.SuggestedMultiplier) // old=0 不算倍率
}

func TestSuggestion_ZeroMultiplier_NoMultiplier(t *testing.T) {
	s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000036, CurrentMultiplier: 0})
	assert.Nil(t, s.SuggestedMultiplier)
	assert.InDelta(t, 0.000036, s.SuggestedInputPrice, 1e-12)
}
