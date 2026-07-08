//go:build unit

package service

import (
	"math"
	"testing"
	"time"
)

func TestComputeProrateCredit(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	expires := start.AddDate(0, 0, 30) // 30 天套餐
	paid := 100.0

	cases := []struct {
		name    string
		now     time.Time
		wantLow float64 // 下界（舍入波动）
		wantHi  float64 // 上界
	}{
		{"用了2天剩28天", start.AddDate(0, 0, 2), 93.0, 93.5},
		{"刚买1分钟", start.Add(time.Minute), 99.0, 100.0},
		{"剩1天", start.AddDate(0, 0, 29), 0.5, 4.0},
		{"已过期剩0", expires.Add(time.Hour), 0, 0},
		{"已过期(now==expires)", expires, 0, 0},
		{"无实付(兑换套餐)", time.Time{}, 0, 0}, // paidAmount=0 特例
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			amt := paid
			if c.name == "无实付(兑换套餐)" {
				amt = 0
			}
			got := computeProrateCredit(amt, start, expires, c.now)
			if math.IsNaN(got) || got < c.wantLow-0.01 || got > c.wantHi+0.01 {
				t.Errorf("credit=%.4f 不在 [%.2f, %.2f]", got, c.wantLow, c.wantHi)
			}
			if c.now.Equal(expires) || c.now.After(expires) {
				if got != 0 {
					t.Errorf("过期应得 0，得 %.4f", got)
				}
			}
		})
	}
}

func TestComputeProrateCredit_ZeroTotal(t *testing.T) {
	// startsAt==expiresAt 退化保护：总秒数为 0 不能除零
	zero := time.Now()
	if got := computeProrateCredit(100, zero, zero, zero); got != 0 {
		t.Errorf("总秒数为0应返回0，得 %.4f", got)
	}
}
