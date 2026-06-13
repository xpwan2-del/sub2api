package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteLLMParser(t *testing.T) {
	raw := `{
      "gpt-4": {"input_cost_per_token": 0.00003, "output_cost_per_token": 0.00006},
      "claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075, "cache_creation_input_token_cost": 0.00001875}
    }`
	p := &LiteLLMParser{}
	out, err := p.Parse([]byte(raw), ParserConfig{AliasMap: map[string]string{"gpt-4": "gpt-4-turbo"}})
	require.NoError(t, err)
	require.Len(t, out, 2)
	m := map[string]UpstreamModelPrice{}
	for _, v := range out {
		m[v.ModelName] = v
	}
	assert.Equal(t, "gpt-4-turbo", m["gpt-4"].LocalModelName)               // alias 映射
	assert.Equal(t, "claude-opus-4-6", m["claude-opus-4-6"].LocalModelName) // 无 alias 保持原名
	assert.InDelta(t, 0.00003, m["gpt-4"].InputPrice, 1e-12)
	assert.InDelta(t, 0.000075, m["claude-opus-4-6"].OutputPrice, 1e-12)
	require.NotNil(t, m["claude-opus-4-6"].CacheWritePrice)
	assert.InDelta(t, 0.00001875, *m["claude-opus-4-6"].CacheWritePrice, 1e-12)
}

func TestLiteLLMParser_SkipsSampleSpec(t *testing.T) {
	raw := `{"sample_spec": {"input_cost_per_token": 0}, "gpt-4": {"input_cost_per_token": 0.00003, "output_cost_per_token": 0.00006}}`
	out, err := (&LiteLLMParser{}).Parse([]byte(raw), ParserConfig{})
	require.NoError(t, err)
	require.Len(t, out, 1) // sample_spec 被跳过
	assert.Equal(t, "gpt-4", out[0].ModelName)
}

func TestLiteLLMParser_InvalidJSON(t *testing.T) {
	_, err := (&LiteLLMParser{}).Parse([]byte("not json"), ParserConfig{})
	require.Error(t, err)
}

func TestCustomJSONPathParser(t *testing.T) {
	raw := `{"data":[{"model":"x","in":0.001,"out":0.002},{"model":"y","in":0.003,"out":0.004}]}`
	out, err := (&CustomJSONPathParser{}).Parse([]byte(raw), ParserConfig{})
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "x", out[0].ModelName)
	assert.InDelta(t, 0.001, out[0].InputPrice, 1e-12)
	assert.InDelta(t, 0.004, out[1].OutputPrice, 1e-12)
}

func TestOneAPIParser(t *testing.T) {
	raw := `{"data":[{"model_name":"gpt-4","model_ratio":3,"completion_ratio":4}]}`
	out, err := (&OneAPIParser{}).Parse([]byte(raw), ParserConfig{})
	require.NoError(t, err)
	require.Len(t, out, 1)
	// model_ratio=3 → input = 3 * 2 / 1e6 = 6e-6 ; output = input * completion_ratio(4) = 2.4e-5
	assert.InDelta(t, 6e-6, out[0].InputPrice, 1e-15)
	assert.InDelta(t, 2.4e-5, out[0].OutputPrice, 1e-15)
}

func TestOneAPIParser_SkipsZeroRatio(t *testing.T) {
	raw := `{"data":[{"model_name":"free","model_ratio":0},{"model_name":"gpt-4","model_ratio":3,"completion_ratio":4}]}`
	out, err := (&OneAPIParser{}).Parse([]byte(raw), ParserConfig{})
	require.NoError(t, err)
	require.Len(t, out, 1) // ratio=0 跳过
	assert.Equal(t, "gpt-4", out[0].ModelName)
}

func TestParserByType(t *testing.T) {
	assert.IsType(t, &LiteLLMParser{}, ParserByType("litellm"))
	assert.IsType(t, &CustomJSONPathParser{}, ParserByType("custom"))
	assert.IsType(t, &OneAPIParser{}, ParserByType("one_api"))
	assert.IsType(t, &OneAPIParser{}, ParserByType("unknown")) // default
}
