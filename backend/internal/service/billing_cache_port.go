package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status       string
	ExpiresAt    time.Time
	DailyUsage   float64
	WeeklyUsage  float64
	MonthlyUsage float64
	DailyLimit   float64
	WeeklyLimit  float64
	MonthlyLimit float64
	Version      int64
}

// BundleSubscriptionCacheData represents cached bundle subscription data for a user.
type BundleSubscriptionCacheData struct {
	ID               int64
	PlanID           int64
	PlanName         string
	Tier             string
	Status           string
	ExpiresAt        int64 // unix seconds
	ConcurrencyLimit int
	RPMLimit         int
	Source           string
}
