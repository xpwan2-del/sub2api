package service

import "time"

type OpsModelStatus string

const (
	OpsModelStatusOperational     OpsModelStatus = "operational"
	OpsModelStatusDegraded        OpsModelStatus = "degraded"
	OpsModelStatusRateLimited     OpsModelStatus = "rate_limited"
	OpsModelStatusFailed          OpsModelStatus = "failed"
	OpsModelStatusNoRecentTraffic OpsModelStatus = "no_recent_traffic"
	OpsModelStatusUnknown         OpsModelStatus = "unknown"
	OpsModelStatusOrphanedHistory OpsModelStatus = "orphaned_history"
)

type OpsModelStatusFilter struct {
	StartTime time.Time
	EndTime   time.Time

	Platform string
	Status   string
	Query    string

	Page     int
	PageSize int
}

type OpsModelTrafficStats struct {
	Platform            string     `json:"platform"`
	Model               string     `json:"model"`
	RequestCount        int64      `json:"request_count"`
	SuccessCount        int64      `json:"success_count"`
	ErrorCount          int64      `json:"error_count"`
	TokenConsumed       int64      `json:"token_consumed"`
	AvgLatencyMs        *float64   `json:"avg_latency_ms,omitempty"`
	P95LatencyMs        *float64   `json:"p95_latency_ms,omitempty"`
	LastSeenAt          *time.Time `json:"last_seen_at,omitempty"`
	LastErrorAt         *time.Time `json:"last_error_at,omitempty"`
	LastErrorType       string     `json:"last_error_type,omitempty"`
	LastErrorStatusCode *int       `json:"last_error_status_code,omitempty"`
}

type OpsHealthHistoryPoint struct {
	BucketStart   time.Time `json:"bucket_start"`
	BucketEnd     time.Time `json:"bucket_end"`
	Status        string    `json:"status"`
	RequestCount  int64     `json:"request_count"`
	SuccessCount  int64     `json:"success_count"`
	ErrorCount    int64     `json:"error_count"`
	SuccessRate   *float64  `json:"success_rate,omitempty"`
	TokenConsumed int64     `json:"token_consumed"`
	AvgLatencyMs  *float64  `json:"avg_latency_ms,omitempty"`
	P50LatencyMs  *float64  `json:"p50_latency_ms,omitempty"`
	P95LatencyMs  *float64  `json:"p95_latency_ms,omitempty"`
	P99LatencyMs  *float64  `json:"p99_latency_ms,omitempty"`
}

type OpsModelHealthBucket struct {
	Platform string `json:"platform"`
	Model    string `json:"model"`
	OpsHealthHistoryPoint
}

type OpsGatewayRouteHealth struct {
	Endpoint      string   `json:"endpoint"`
	Status        string   `json:"status"`
	RequestCount  int64    `json:"request_count"`
	SuccessCount  int64    `json:"success_count"`
	ErrorCount    int64    `json:"error_count"`
	SuccessRate   *float64 `json:"success_rate,omitempty"`
	TokenConsumed int64    `json:"token_consumed"`
	AvgLatencyMs  *float64 `json:"avg_latency_ms,omitempty"`
	P50LatencyMs  *float64 `json:"p50_latency_ms,omitempty"`
	P95LatencyMs  *float64 `json:"p95_latency_ms,omitempty"`
	P99LatencyMs  *float64 `json:"p99_latency_ms,omitempty"`
}

type OpsModelStatusSnapshot struct {
	GeneratedAt         time.Time                    `json:"generated_at"`
	Window              OpsModelStatusWindow         `json:"window"`
	CloudMetrics        *OpsGoogleCloudMetricsResult `json:"cloud_metrics"`
	GatewaySummary      *OpsModelGatewaySummary      `json:"gateway_summary"`
	ModelSummary        OpsModelSummary              `json:"model_summary"`
	Providers           []*OpsProviderStatusItem     `json:"providers"`
	Models              []*OpsModelStatusItem        `json:"models"`
	AccountAvailability *OpsModelAccountAvailability `json:"account_availability"`
	RecentErrors        []*OpsModelRecentError       `json:"recent_errors"`
	Pagination          OpsModelStatusPagination     `json:"pagination"`
}

type OpsModelStatusWindow struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Seconds   int64     `json:"seconds"`
}

type OpsModelGatewaySummary struct {
	RequestCountTotal int64                    `json:"request_count_total"`
	RequestCountSLA   int64                    `json:"request_count_sla"`
	SuccessCount      int64                    `json:"success_count"`
	ErrorCountTotal   int64                    `json:"error_count_total"`
	TokenConsumed     int64                    `json:"token_consumed"`
	SLA               float64                  `json:"sla"`
	ErrorRate         float64                  `json:"error_rate"`
	QPS               OpsRateSummary           `json:"qps"`
	TPS               OpsRateSummary           `json:"tps"`
	History           []*OpsHealthHistoryPoint `json:"history"`
	Routes            []*OpsGatewayRouteHealth `json:"routes"`
}

type OpsModelSummary struct {
	TotalModels     int `json:"total_models"`
	Operational     int `json:"operational"`
	Degraded        int `json:"degraded"`
	RateLimited     int `json:"rate_limited"`
	Failed          int `json:"failed"`
	NoRecentTraffic int `json:"no_recent_traffic"`
	Unknown         int `json:"unknown"`
	OrphanedHistory int `json:"orphaned_history"`
}

type OpsProviderStatusItem struct {
	Platform          string                   `json:"platform"`
	Status            OpsModelStatus           `json:"status"`
	TotalModels       int                      `json:"total_models"`
	OperationalModels int                      `json:"operational_models"`
	DegradedModels    int                      `json:"degraded_models"`
	FailedModels      int                      `json:"failed_models"`
	AvailableAccounts int64                    `json:"available_accounts"`
	TotalAccounts     int64                    `json:"total_accounts"`
	RequestCount      int64                    `json:"request_count"`
	ErrorCount        int64                    `json:"error_count"`
	SuccessRate       float64                  `json:"success_rate"`
	History           []*OpsHealthHistoryPoint `json:"history"`
	AvgLatencyMs      *float64                 `json:"avg_latency_ms,omitempty"`
	P95LatencyMs      *float64                 `json:"p95_latency_ms,omitempty"`
}

type OpsModelStatusItem struct {
	Platform          string                   `json:"platform"`
	Model             string                   `json:"model"`
	Status            OpsModelStatus           `json:"status"`
	SourceFlags       []string                 `json:"source_flags"`
	RequestCount      int64                    `json:"request_count"`
	SuccessCount      int64                    `json:"success_count"`
	ErrorCount        int64                    `json:"error_count"`
	SuccessRate       float64                  `json:"success_rate"`
	TokenConsumed     int64                    `json:"token_consumed"`
	AvgLatencyMs      *float64                 `json:"avg_latency_ms,omitempty"`
	P95LatencyMs      *float64                 `json:"p95_latency_ms,omitempty"`
	AvailableAccounts int64                    `json:"available_accounts"`
	TotalAccounts     int64                    `json:"total_accounts"`
	LastSeenAt        *time.Time               `json:"last_seen_at,omitempty"`
	LastErrorAt       *time.Time               `json:"last_error_at,omitempty"`
	LastErrorType     string                   `json:"last_error_type,omitempty"`
	LastErrorStatus   *int                     `json:"last_error_status_code,omitempty"`
	History           []*OpsHealthHistoryPoint `json:"history"`
}

type OpsModelAccountAvailability struct {
	TotalAccounts       int64 `json:"total_accounts"`
	AvailableAccounts   int64 `json:"available_accounts"`
	RateLimitedAccounts int64 `json:"rate_limited_accounts"`
	ErrorAccounts       int64 `json:"error_accounts"`
}

type OpsModelRecentError struct {
	Platform   string     `json:"platform"`
	Model      string     `json:"model"`
	ErrorType  string     `json:"error_type"`
	StatusCode *int       `json:"status_code,omitempty"`
	At         *time.Time `json:"at,omitempty"`
}

type OpsModelStatusPagination struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}
