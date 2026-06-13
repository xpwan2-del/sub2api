package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiff_NewModel(t *testing.T) {
	curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	ch := DiffPrices(curr, map[string]PriceSnapshot{})
	require.Len(t, ch, 1)
	assert.Equal(t, PriceChangeNew, ch[0].Type)
	assert.Nil(t, ch[0].PrevInputPrice)
}

func TestDiff_PriceUp(t *testing.T) {
	prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.000036, OutputPrice: 0.00006}}
	ch := DiffPrices(curr, prev)
	require.Len(t, ch, 1)
	assert.Equal(t, PriceChangeUp, ch[0].Type)
	assert.InDelta(t, 0.2, ch[0].InputDeltaPct, 1e-6) // +20%
}

func TestDiff_PriceDown(t *testing.T) {
	prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.000024, OutputPrice: 0.00006}}
	ch := DiffPrices(curr, prev)
	require.Len(t, ch, 1)
	assert.Equal(t, PriceChangeDown, ch[0].Type)
	assert.InDelta(t, -0.2, ch[0].InputDeltaPct, 1e-6) // -20%
}

func TestDiff_Removed(t *testing.T) {
	prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	ch := DiffPrices(map[string]PriceSnapshot{}, prev)
	require.Len(t, ch, 1)
	assert.Equal(t, PriceChangeGone, ch[0].Type)
	require.NotNil(t, ch[0].PrevInputPrice)
	assert.InDelta(t, 0.00003, *ch[0].PrevInputPrice, 1e-12)
}

func TestDiff_NoChangeEpsilon(t *testing.T) {
	s := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	ch := DiffPrices(s, s)
	assert.Empty(t, ch)
}

func TestDiff_BothEmpty(t *testing.T) {
	ch := DiffPrices(map[string]PriceSnapshot{}, map[string]PriceSnapshot{})
	assert.Empty(t, ch)
}

func TestDiff_OutputOnlyChange(t *testing.T) {
	// input 不变，output 变 → 仍算变动（按 output 涨/跌）
	prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
	curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00009}}
	ch := DiffPrices(curr, prev)
	require.Len(t, ch, 1)
	assert.Equal(t, PriceChangeUp, ch[0].Type)
	assert.InDelta(t, 0, ch[0].InputDeltaPct, 1e-9)
	assert.InDelta(t, 0.5, ch[0].OutputDeltaPct, 1e-6) // +50%
}
