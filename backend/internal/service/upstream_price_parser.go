package service

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
)

// UpstreamModelPrice 是解析后的标准化上游模型价格。
//
// 单位：per-token USD（与 channel_model_pricing / LiteLLM input_cost_per_token 一致；
// 持久化到 ent.UpstreamModelPrice 时直接存原值，无需 ×1e6 单位转换）。
type UpstreamModelPrice struct {
	ModelName       string
	LocalModelName  string
	InputPrice      float64
	OutputPrice     float64
	CacheWritePrice *float64
	CacheReadPrice  *float64
	RawPayload      map[string]any
}

// ParserConfig 携带解析所需配置（别名映射、custom 路径）。
type ParserConfig struct {
	AliasMap map[string]string
}

// PriceParser 把上游返回的原始 JSON 解析为标准价格列表。
type PriceParser interface {
	Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error)
}

func applyAlias(name string, m map[string]string) string {
	if v, ok := m[name]; ok && v != "" {
		return v
	}
	return name
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// LiteLLMParser 解析 LiteLLM model_prices_and_context_window.json 格式：
// {"model_name": {"input_cost_per_token":..., "output_cost_per_token":..., "cache_creation_input_token_cost":...}}
type LiteLLMParser struct{}

func (p *LiteLLMParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
	var doc map[string]map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("litellm parse: %w", err)
	}
	out := make([]UpstreamModelPrice, 0, len(doc))
	for name, fields := range doc {
		if name == "sample_spec" {
			continue
		}
		m := UpstreamModelPrice{
			ModelName:      name,
			LocalModelName: applyAlias(name, cfg.AliasMap),
			RawPayload:     fields,
			InputPrice:     toFloat(fields["input_cost_per_token"]),
			OutputPrice:    toFloat(fields["output_cost_per_token"]),
		}
		if v, ok := fields["cache_creation_input_token_cost"]; ok && v != nil {
			f := toFloat(v)
			m.CacheWritePrice = &f
		}
		if v, ok := fields["cache_read_input_token_cost"]; ok && v != nil {
			f := toFloat(v)
			m.CacheReadPrice = &f
		}
		out = append(out, m)
	}
	return out, nil
}

// CustomJSONPathParser 解析自定义结构 {"data":[{"model","in","out"}]}。
type CustomJSONPathParser struct{}

func (p *CustomJSONPathParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
	arr := gjson.GetBytes(raw, "data").Array()
	out := make([]UpstreamModelPrice, 0, len(arr))
	for _, item := range arr {
		m := UpstreamModelPrice{
			ModelName:   item.Get("model").String(),
			InputPrice:  item.Get("in").Float(),
			OutputPrice: item.Get("out").Float(),
		}
		m.LocalModelName = applyAlias(m.ModelName, cfg.AliasMap)
		out = append(out, m)
	}
	return out, nil
}

// OneAPIParser 解析 new-api/one-api 系 /api/pricing 返回。
// 字段语义：model_ratio(相对 $2/M token 基准的倍率), completion_ratio(output/input)。
// per_token_input = model_ratio * 2 / 1e6 ; per_token_output = per_token_input * completion_ratio。
// 注意：不同 fork 字段可能不同，需用真实上游返回校准。
type OneAPIParser struct{}

func (p *OneAPIParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
	arr := gjson.GetBytes(raw, "data").Array()
	const baseRatePerMillion = 2.0
	out := make([]UpstreamModelPrice, 0, len(arr))
	for _, item := range arr {
		ratio := item.Get("model_ratio").Float()
		if ratio == 0 {
			continue
		}
		compRatio := item.Get("completion_ratio").Float()
		if compRatio == 0 {
			compRatio = 1
		}
		inPerToken := ratio * baseRatePerMillion / 1e6
		m := UpstreamModelPrice{
			ModelName:   item.Get("model_name").String(),
			InputPrice:  inPerToken,
			OutputPrice: inPerToken * compRatio,
		}
		m.LocalModelName = applyAlias(m.ModelName, cfg.AliasMap)
		out = append(out, m)
	}
	return out, nil
}

// ParserByType 按 parser_type 字符串返回对应解析器，默认 OneAPI。
func ParserByType(t string) PriceParser {
	switch t {
	case "litellm":
		return &LiteLLMParser{}
	case "custom":
		return &CustomJSONPathParser{}
	default:
		return &OneAPIParser{}
	}
}
