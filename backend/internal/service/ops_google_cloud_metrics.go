package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const googleMetadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

type OpsGoogleCloudMetricsResult struct {
	Enabled     bool                         `json:"enabled"`
	Status      string                       `json:"status"`
	Source      string                       `json:"source"`
	ProjectID   string                       `json:"project_id,omitempty"`
	InstanceID  string                       `json:"instance_id,omitempty"`
	Zone        string                       `json:"zone,omitempty"`
	CollectedAt *time.Time                   `json:"collected_at,omitempty"`
	Window      int                          `json:"window_seconds"`
	Error       string                       `json:"error,omitempty"`
	Metrics     OpsGoogleCloudMetricsPayload `json:"metrics"`
}

type OpsGoogleCloudMetricsPayload struct {
	CPUPercent        *float64 `json:"cpu_percent,omitempty"`
	MemoryPercent     *float64 `json:"memory_percent,omitempty"`
	DiskPercent       *float64 `json:"disk_percent,omitempty"`
	NetworkRxBytesSec *float64 `json:"network_rx_bytes_sec,omitempty"`
	NetworkTxBytesSec *float64 `json:"network_tx_bytes_sec,omitempty"`
	DBOK              *bool    `json:"db_ok,omitempty"`
	RedisOK           *bool    `json:"redis_ok,omitempty"`
	DBConnActive      *int     `json:"db_conn_active,omitempty"`
	DBConnIdle        *int     `json:"db_conn_idle,omitempty"`
	RedisConnTotal    *int     `json:"redis_conn_total,omitempty"`
	RedisConnIdle     *int     `json:"redis_conn_idle,omitempty"`
	GoroutineCount    *int     `json:"goroutine_count,omitempty"`
}

func (s *OpsService) GetGoogleCloudMetrics(ctx context.Context) *OpsGoogleCloudMetricsResult {
	cfg := config.OpsGoogleCloudMonitoringConfig{}
	if s != nil && s.cfg != nil {
		cfg = s.cfg.Ops.GoogleCloudMonitoring
	}

	result := &OpsGoogleCloudMetricsResult{
		Enabled:    cfg.Enabled,
		Status:     "disabled",
		Source:     "google_cloud_monitoring",
		ProjectID:  strings.TrimSpace(cfg.ProjectID),
		InstanceID: strings.TrimSpace(cfg.InstanceID),
		Zone:       strings.TrimSpace(cfg.Zone),
		Window:     cfg.WindowSeconds,
	}
	if result.Window <= 0 {
		result.Window = 300
	}
	if !cfg.Enabled {
		s.fillLocalSystemMetrics(ctx, result)
		return result
	}
	if result.ProjectID == "" || result.InstanceID == "" || result.Zone == "" {
		result.Status = "not_configured"
		result.Error = "project_id, instance_id and zone are required"
		return result
	}

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	token, err := fetchGoogleMetadataAccessToken(reqCtx)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}

	end := time.Now().UTC()
	start := end.Add(-time.Duration(result.Window) * time.Second)
	collectedAt := end
	result.CollectedAt = &collectedAt

	client := &http.Client{Timeout: 3 * time.Second}
	metricNames := cfg.Metrics
	queries := []struct {
		name   string
		metric string
		assign func(float64)
		scale  float64
	}{
		{"cpu", metricNames.CPUUtilization, func(v float64) { result.Metrics.CPUPercent = &v }, 100},
		{"memory", metricNames.MemoryPercentUsed, func(v float64) { result.Metrics.MemoryPercent = &v }, 1},
		{"disk", metricNames.DiskPercentUsed, func(v float64) { result.Metrics.DiskPercent = &v }, 1},
		{"network_rx", metricNames.NetworkReceived, func(v float64) { result.Metrics.NetworkRxBytesSec = &v }, 1 / float64(result.Window)},
		{"network_tx", metricNames.NetworkSent, func(v float64) { result.Metrics.NetworkTxBytesSec = &v }, 1 / float64(result.Window)},
	}

	successes := 0
	var lastErr error
	for _, q := range queries {
		if strings.TrimSpace(q.metric) == "" {
			continue
		}
		value, err := readGoogleMonitoringLatestValue(reqCtx, client, token, result.ProjectID, result.InstanceID, result.Zone, q.metric, start, end)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", q.name, err)
			continue
		}
		value *= q.scale
		q.assign(roundFloat(value, 2))
		successes++
	}

	if successes == 0 {
		result.Status = "error"
		if lastErr != nil {
			result.Error = lastErr.Error()
		} else {
			result.Error = "no metrics returned"
		}
		return result
	}
	if lastErr != nil {
		result.Status = "partial"
		result.Error = lastErr.Error()
		return result
	}
	result.Status = "ok"
	return result
}

func (s *OpsService) fillLocalSystemMetrics(ctx context.Context, result *OpsGoogleCloudMetricsResult) {
	if s == nil || s.opsRepo == nil || result == nil {
		return
	}
	metrics, err := s.opsRepo.GetLatestSystemMetrics(ctx, 1)
	if err != nil || metrics == nil || metrics.CreatedAt.IsZero() {
		return
	}

	result.Enabled = true
	result.Status = "ok"
	result.Source = "local_system_metrics"
	result.CollectedAt = &metrics.CreatedAt
	result.Window = metrics.WindowMinutes * 60
	if result.Window <= 0 {
		result.Window = 60
	}
	result.Metrics.CPUPercent = metrics.CPUUsagePercent
	result.Metrics.MemoryPercent = metrics.MemoryUsagePercent
	result.Metrics.DBOK = metrics.DBOK
	result.Metrics.RedisOK = metrics.RedisOK
	result.Metrics.DBConnActive = metrics.DBConnActive
	result.Metrics.DBConnIdle = metrics.DBConnIdle
	result.Metrics.RedisConnTotal = metrics.RedisConnTotal
	result.Metrics.RedisConnIdle = metrics.RedisConnIdle
	result.Metrics.GoroutineCount = metrics.GoroutineCount

	if (metrics.DBOK != nil && !*metrics.DBOK) || (metrics.RedisOK != nil && !*metrics.RedisOK) {
		result.Status = "partial"
	}
}

func fetchGoogleMetadataAccessToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleMetadataTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("metadata token request returned %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode metadata token: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("metadata token response missing access_token")
	}
	return payload.AccessToken, nil
}

func readGoogleMonitoringLatestValue(ctx context.Context, client *http.Client, token, projectID, instanceID, zone, metricType string, start, end time.Time) (float64, error) {
	filter := fmt.Sprintf(`metric.type="%s" AND resource.type="gce_instance" AND resource.labels.instance_id="%s" AND resource.labels.zone="%s"`, metricType, instanceID, zone)
	values := url.Values{}
	values.Set("filter", filter)
	values.Set("interval.startTime", start.Format(time.RFC3339))
	values.Set("interval.endTime", end.Format(time.RFC3339))
	values.Set("view", "FULL")
	values.Set("pageSize", "1")

	endpoint := fmt.Sprintf("https://monitoring.googleapis.com/v3/projects/%s/timeSeries?%s", url.PathEscape(projectID), values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("monitoring API returned %d", resp.StatusCode)
	}

	var payload struct {
		TimeSeries []struct {
			Points []struct {
				Value struct {
					DoubleValue       *float64 `json:"doubleValue,omitempty"`
					Int64Value        *string  `json:"int64Value,omitempty"`
					DistributionValue *struct {
						Mean *float64 `json:"mean,omitempty"`
					} `json:"distributionValue,omitempty"`
				} `json:"value"`
			} `json:"points"`
		} `json:"timeSeries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}
	for _, ts := range payload.TimeSeries {
		for _, point := range ts.Points {
			switch {
			case point.Value.DoubleValue != nil:
				return *point.Value.DoubleValue, nil
			case point.Value.Int64Value != nil:
				var v float64
				if _, err := fmt.Sscanf(*point.Value.Int64Value, "%f", &v); err == nil {
					return v, nil
				}
			case point.Value.DistributionValue != nil && point.Value.DistributionValue.Mean != nil:
				return *point.Value.DistributionValue.Mean, nil
			}
		}
	}
	return 0, fmt.Errorf("metric %s has no points", metricType)
}

func roundFloat(v float64, places int) float64 {
	if places <= 0 {
		return float64(int(v + 0.5))
	}
	mul := 1.0
	for i := 0; i < places; i++ {
		mul *= 10
	}
	if v >= 0 {
		return float64(int(v*mul+0.5)) / mul
	}
	return float64(int(v*mul-0.5)) / mul
}
