package service

import (
	"math"
	"time"
)

// computeProrateCredit 按剩余有效期线性折算旧套餐剩余价值。
// credit = paidAmount × max(0, 剩余秒) / 总秒。过期或总额为0返回0。
// 不追溯已消费的请求额度——套餐卖的是"有效期内使用权"而非预付token。
func computeProrateCredit(paidAmount float64, startsAt, expiresAt, now time.Time) float64 {
	if paidAmount <= 0 {
		return 0
	}
	totalSec := expiresAt.Sub(startsAt).Seconds()
	if totalSec <= 0 {
		return 0
	}
	remainSec := expiresAt.Sub(now).Seconds()
	if remainSec <= 0 {
		return 0
	}
	credit := paidAmount * remainSec / totalSec
	if credit < 0 || math.IsNaN(credit) {
		return 0
	}
	// 货币精度：2 位小数
	return math.Round(credit*100) / 100
}
