package service

const priceEpsilon = 1e-12

type PriceChangeType string

const (
	PriceChangeUp   PriceChangeType = "price_up"
	PriceChangeDown PriceChangeType = "price_down"
	PriceChangeNew  PriceChangeType = "new_model"
	PriceChangeGone PriceChangeType = "removed"
)

// PriceSnapshot 是参与 diff 的单模型价格快照（per-token USD）。
type PriceSnapshot struct {
	InputPrice  float64
	OutputPrice float64
}

// PriceChange 描述一次价格变动。
type PriceChange struct {
	ModelName       string
	Type            PriceChangeType
	PrevInputPrice  *float64
	CurrInputPrice  float64
	PrevOutputPrice *float64
	CurrOutputPrice float64
	InputDeltaPct   float64
	OutputDeltaPct  float64
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func ptrPrice(f float64) *float64 { return &f }

// DiffPrices 对比当前快照与上次快照，返回变动列表。
//   - 新模型(curr 有 prev 无) → PriceChangeNew
//   - 下架(prev 有 curr 无) → PriceChangeGone
//   - 价格变化超 epsilon → PriceChangeUp/Down(按净方向)，含 delta_pct
//   - 价格未变 → 不产出
func DiffPrices(curr, prev map[string]PriceSnapshot) []PriceChange {
	changes := make([]PriceChange, 0)
	for name, c := range curr {
		p, ok := prev[name]
		if !ok {
			changes = append(changes, PriceChange{
				ModelName:       name,
				Type:            PriceChangeNew,
				CurrInputPrice:  c.InputPrice,
				CurrOutputPrice: c.OutputPrice,
			})
			continue
		}
		inDelta, outDelta := c.InputPrice-p.InputPrice, c.OutputPrice-p.OutputPrice
		if absFloat(inDelta) <= priceEpsilon && absFloat(outDelta) <= priceEpsilon {
			continue
		}
		ch := PriceChange{
			ModelName:       name,
			CurrInputPrice:  c.InputPrice,
			CurrOutputPrice: c.OutputPrice,
			PrevInputPrice:  ptrPrice(p.InputPrice),
			PrevOutputPrice: ptrPrice(p.OutputPrice),
		}
		if p.InputPrice > priceEpsilon {
			ch.InputDeltaPct = inDelta / p.InputPrice
		}
		if p.OutputPrice > priceEpsilon {
			ch.OutputDeltaPct = outDelta / p.OutputPrice
		}
		if inDelta > 0 || outDelta > 0 {
			ch.Type = PriceChangeUp
		} else {
			ch.Type = PriceChangeDown
		}
		changes = append(changes, ch)
	}
	for name, p := range prev {
		if _, ok := curr[name]; !ok {
			changes = append(changes, PriceChange{
				ModelName:       name,
				Type:            PriceChangeGone,
				PrevInputPrice:  ptrPrice(p.InputPrice),
				PrevOutputPrice: ptrPrice(p.OutputPrice),
			})
		}
	}
	return changes
}
