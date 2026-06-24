package service

import (
	"context"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetModelStatusSnapshot(ctx context.Context, filter *OpsModelStatusFilter) (*OpsModelStatusSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_MODEL_STATUS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_MODEL_STATUS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_MODEL_STATUS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	filter.Platform = strings.TrimSpace(strings.ToLower(filter.Platform))
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	filter.Query = strings.TrimSpace(filter.Query)

	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}

	stats, err := s.opsRepo.GetModelTrafficStats(ctx, filter)
	if err != nil {
		return nil, err
	}
	historyFilter := buildOpsModelStatusHistoryFilter(filter)
	historyBuckets, err := s.opsRepo.GetModelHealthBuckets(ctx, historyFilter, opsModelHealthBucketSeconds)
	if err != nil {
		return nil, err
	}
	routeHealth, err := s.opsRepo.GetGatewayRouteHealth(ctx, filter, 14)
	if err != nil {
		return nil, err
	}
	for _, route := range routeHealth {
		classifyOpsGatewayRouteHealth(route)
	}
	historyStarts := opsModelHealthBucketStarts(historyFilter.StartTime, opsModelHealthBucketCount, opsModelHealthBucketSeconds)
	historyByKey := groupOpsModelHealthBuckets(historyBuckets)
	trafficByKey := make(map[string]*OpsModelTrafficStats, len(stats))
	for _, st := range stats {
		if st == nil || strings.TrimSpace(st.Model) == "" {
			continue
		}
		st.Platform = normalizeOpsStatusPlatform(st.Platform)
		key := opsModelKey(st.Platform, st.Model)
		trafficByKey[key] = st
	}

	accounts, _ := s.listAllAccountsForOps(ctx, filter.Platform)
	inventory := buildOpsModelInventory(accounts, trafficByKey, filter.Query)
	items := make([]*OpsModelStatusItem, 0, len(inventory))
	for key, inv := range inventory {
		st := trafficByKey[key]
		item := buildOpsModelStatusItem(inv, st)
		item.History = buildOpsModelHistory(historyStarts, historyByKey[key], opsModelHealthBucketSeconds)
		if filter.Status != "" && string(item.Status) != filter.Status {
			continue
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		ri := opsModelStatusRank(items[i].Status)
		rj := opsModelStatusRank(items[j].Status)
		if ri != rj {
			return ri < rj
		}
		if items[i].RequestCount != items[j].RequestCount {
			return items[i].RequestCount > items[j].RequestCount
		}
		if items[i].Platform != items[j].Platform {
			return items[i].Platform < items[j].Platform
		}
		return items[i].Model < items[j].Model
	})

	total := int64(len(items))
	startIdx := (filter.Page - 1) * filter.PageSize
	if startIdx > len(items) {
		startIdx = len(items)
	}
	endIdx := startIdx + filter.PageSize
	if endIdx > len(items) {
		endIdx = len(items)
	}
	pageItems := items[startIdx:endIdx]

	summary := summarizeOpsModels(items)
	providers := summarizeOpsProviders(items)
	accountAvailability := summarizeOpsAccountAvailability(accounts)
	gatewaySummary := s.buildOpsModelGatewaySummary(ctx, filter, aggregateOpsHealthHistories(modelHistories(items)), routeHealth)
	recentErrors := buildOpsModelRecentErrors(items)
	cloudMetrics := s.GetGoogleCloudMetrics(ctx)

	return &OpsModelStatusSnapshot{
		GeneratedAt: time.Now().UTC(),
		Window: OpsModelStatusWindow{
			StartTime: filter.StartTime.UTC(),
			EndTime:   filter.EndTime.UTC(),
			Seconds:   int64(filter.EndTime.Sub(filter.StartTime).Seconds()),
		},
		CloudMetrics:        cloudMetrics,
		GatewaySummary:      gatewaySummary,
		ModelSummary:        summary,
		Providers:           providers,
		Models:              pageItems,
		AccountAvailability: accountAvailability,
		RecentErrors:        recentErrors,
		Pagination: OpsModelStatusPagination{
			Page:     filter.Page,
			PageSize: filter.PageSize,
			Total:    total,
		},
	}, nil
}

const (
	opsModelHealthBucketSeconds = 3600
	opsModelHealthBucketCount   = 48
	opsLatencyDegradedP95Ms     = 60000
)

type opsModelInventoryItem struct {
	Platform          string
	Model             string
	SourceFlags       map[string]struct{}
	AvailableAccounts int64
	TotalAccounts     int64
}

func buildOpsModelInventory(accounts []Account, traffic map[string]*OpsModelTrafficStats, query string) map[string]*opsModelInventoryItem {
	query = strings.ToLower(strings.TrimSpace(query))
	out := make(map[string]*opsModelInventoryItem)

	add := func(platform, model, source string) *opsModelInventoryItem {
		platform = normalizeOpsStatusPlatform(platform)
		model = strings.TrimSpace(model)
		if model == "" {
			return nil
		}
		if query != "" && !strings.Contains(strings.ToLower(model), query) {
			return nil
		}
		key := opsModelKey(platform, model)
		item := out[key]
		if item == nil {
			item = &opsModelInventoryItem{
				Platform:    platform,
				Model:       model,
				SourceFlags: map[string]struct{}{},
			}
			out[key] = item
		}
		if source != "" {
			item.SourceFlags[source] = struct{}{}
		}
		return item
	}

	for _, acc := range accounts {
		platform := normalizeOpsStatusPlatform(acc.Platform)
		mapping := acc.GetModelMapping()
		for reqModel, upstreamModel := range mapping {
			for _, model := range []string{reqModel, upstreamModel} {
				add(platform, model, "account_mapping")
			}
		}
		for _, group := range acc.Groups {
			if group == nil {
				continue
			}
			groupPlatform := normalizeOpsStatusPlatform(group.Platform)
			if groupPlatform == "unknown" {
				groupPlatform = platform
			}
			for _, model := range group.ModelsListConfig.Models {
				add(groupPlatform, model, "group_models_list")
			}
			for pattern := range group.ModelRouting {
				add(groupPlatform, pattern, "group_model_routing")
			}
		}
	}

	for _, st := range traffic {
		if st == nil {
			continue
		}
		add(st.Platform, st.Model, "real_traffic")
	}

	for _, item := range out {
		for _, acc := range accounts {
			if normalizeOpsStatusPlatform(acc.Platform) != item.Platform {
				continue
			}
			if !acc.IsModelSupported(item.Model) {
				continue
			}
			item.TotalAccounts++
			if acc.IsSchedulable() {
				item.AvailableAccounts++
			}
		}
	}

	return out
}

func buildOpsModelStatusItem(inv *opsModelInventoryItem, st *OpsModelTrafficStats) *OpsModelStatusItem {
	item := &OpsModelStatusItem{
		Platform:          inv.Platform,
		Model:             inv.Model,
		SourceFlags:       sortedSourceFlags(inv.SourceFlags),
		AvailableAccounts: inv.AvailableAccounts,
		TotalAccounts:     inv.TotalAccounts,
	}
	if st != nil {
		item.RequestCount = st.RequestCount
		item.SuccessCount = st.SuccessCount
		item.ErrorCount = st.ErrorCount
		item.TokenConsumed = st.TokenConsumed
		item.AvgLatencyMs = st.AvgLatencyMs
		item.P95LatencyMs = st.P95LatencyMs
		item.LastSeenAt = st.LastSeenAt
		item.LastErrorAt = st.LastErrorAt
		item.LastErrorType = st.LastErrorType
		item.LastErrorStatus = st.LastErrorStatusCode
		if st.RequestCount > 0 {
			item.SuccessRate = roundFloat(float64(st.SuccessCount)*100/float64(st.RequestCount), 2)
		}
	}
	item.Status = classifyOpsModelStatus(item)
	return item
}

func classifyOpsModelStatus(item *OpsModelStatusItem) OpsModelStatus {
	if item == nil {
		return OpsModelStatusUnknown
	}
	hasTraffic := item.RequestCount > 0
	hasConfig := hasSource(item.SourceFlags, "account_mapping") || hasSource(item.SourceFlags, "group_models_list") || hasSource(item.SourceFlags, "group_model_routing")
	if !hasConfig && hasTraffic {
		return OpsModelStatusOrphanedHistory
	}
	if !hasTraffic {
		if hasConfig {
			return OpsModelStatusNoRecentTraffic
		}
		return OpsModelStatusUnknown
	}
	if item.P95LatencyMs != nil && *item.P95LatencyMs >= opsLatencyDegradedP95Ms && item.SuccessRate >= 95 {
		return OpsModelStatusDegraded
	}
	if item.SuccessRate >= 95 {
		return OpsModelStatusOperational
	}
	if item.SuccessRate >= 80 {
		return OpsModelStatusDegraded
	}
	return OpsModelStatusFailed
}

func summarizeOpsModels(items []*OpsModelStatusItem) OpsModelSummary {
	out := OpsModelSummary{TotalModels: len(items)}
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.Status {
		case OpsModelStatusOperational:
			out.Operational++
		case OpsModelStatusDegraded:
			out.Degraded++
		case OpsModelStatusRateLimited:
			out.RateLimited++
		case OpsModelStatusFailed:
			out.Failed++
		case OpsModelStatusNoRecentTraffic:
			out.NoRecentTraffic++
		case OpsModelStatusOrphanedHistory:
			out.OrphanedHistory++
		default:
			out.Unknown++
		}
	}
	return out
}

func summarizeOpsProviders(items []*OpsModelStatusItem) []*OpsProviderStatusItem {
	byPlatform := map[string]*OpsProviderStatusItem{}
	for _, item := range items {
		if item == nil {
			continue
		}
		p := byPlatform[item.Platform]
		if p == nil {
			p = &OpsProviderStatusItem{Platform: item.Platform, Status: OpsModelStatusOperational}
			byPlatform[item.Platform] = p
		}
		p.TotalModels++
		p.AvailableAccounts += item.AvailableAccounts
		p.TotalAccounts += item.TotalAccounts
		p.RequestCount += item.RequestCount
		p.ErrorCount += item.ErrorCount
		p.History = mergeOpsHealthHistory(p.History, item.History)
		p.AvgLatencyMs = mergeLatencyPointer(p.AvgLatencyMs, item.AvgLatencyMs)
		p.P95LatencyMs = maxLatencyPointer(p.P95LatencyMs, item.P95LatencyMs)
		switch item.Status {
		case OpsModelStatusOperational:
			p.OperationalModels++
		case OpsModelStatusDegraded:
			p.DegradedModels++
		case OpsModelStatusFailed, OpsModelStatusRateLimited:
			p.FailedModels++
		}
		if opsModelStatusRank(item.Status) < opsModelStatusRank(p.Status) {
			p.Status = item.Status
		}
	}
	out := make([]*OpsProviderStatusItem, 0, len(byPlatform))
	for _, p := range byPlatform {
		if p.RequestCount > 0 {
			p.SuccessRate = roundFloat(float64(p.RequestCount-p.ErrorCount)*100/float64(p.RequestCount), 2)
		}
		for _, point := range p.History {
			classifyOpsHealthPoint(point)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Platform < out[j].Platform })
	return out
}

func summarizeOpsAccountAvailability(accounts []Account) *OpsModelAccountAvailability {
	out := &OpsModelAccountAvailability{}
	for _, acc := range accounts {
		out.TotalAccounts++
		switch {
		case acc.IsSchedulable():
			out.AvailableAccounts++
		case acc.RateLimitResetAt != nil && time.Now().Before(*acc.RateLimitResetAt):
			out.RateLimitedAccounts++
		case acc.Status == StatusError:
			out.ErrorAccounts++
		}
	}
	return out
}

func (s *OpsService) buildOpsModelGatewaySummary(ctx context.Context, filter *OpsModelStatusFilter, history []*OpsHealthHistoryPoint, routes []*OpsGatewayRouteHealth) *OpsModelGatewaySummary {
	dashboardFilter := &OpsDashboardFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
		Platform:  filter.Platform,
		QueryMode: OpsQueryModeRaw,
	}
	overview, err := s.GetDashboardOverview(ctx, dashboardFilter)
	if err != nil || overview == nil {
		return &OpsModelGatewaySummary{History: history, Routes: routes}
	}
	out := &OpsModelGatewaySummary{
		RequestCountTotal: overview.RequestCountTotal,
		RequestCountSLA:   overview.RequestCountSLA,
		SuccessCount:      overview.SuccessCount,
		ErrorCountTotal:   overview.ErrorCountTotal,
		TokenConsumed:     overview.TokenConsumed,
		SLA:               overview.SLA,
		ErrorRate:         overview.ErrorRate,
		QPS:               overview.QPS,
		TPS:               overview.TPS,
		History:           history,
		Routes:            routes,
	}
	if filter.EndTime.Sub(filter.StartTime) <= time.Hour {
		if realtime, err := s.GetRealtimeTrafficSummary(ctx, dashboardFilter); err == nil && realtime != nil {
			out.QPS = realtime.QPS
			out.TPS = realtime.TPS
		}
	}
	return out
}

func buildOpsModelRecentErrors(items []*OpsModelStatusItem) []*OpsModelRecentError {
	withErrors := make([]*OpsModelStatusItem, 0, len(items))
	for _, item := range items {
		if item != nil && item.LastErrorAt != nil {
			withErrors = append(withErrors, item)
		}
	}
	sort.Slice(withErrors, func(i, j int) bool {
		return withErrors[i].LastErrorAt.After(*withErrors[j].LastErrorAt)
	})
	limit := len(withErrors)
	if limit > 10 {
		limit = 10
	}
	out := make([]*OpsModelRecentError, 0, limit)
	for i := 0; i < limit; i++ {
		item := withErrors[i]
		out = append(out, &OpsModelRecentError{
			Platform:   item.Platform,
			Model:      item.Model,
			ErrorType:  item.LastErrorType,
			StatusCode: item.LastErrorStatus,
			At:         item.LastErrorAt,
		})
	}
	return out
}

func buildOpsModelStatusHistoryFilter(filter *OpsModelStatusFilter) *OpsModelStatusFilter {
	end := filter.EndTime.UTC().Truncate(time.Hour).Add(time.Hour)
	start := end.Add(-time.Duration(opsModelHealthBucketCount*opsModelHealthBucketSeconds) * time.Second)
	return &OpsModelStatusFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  filter.Platform,
		Query:     filter.Query,
		Page:      1,
		PageSize:  filter.PageSize,
	}
}

func opsModelHealthBucketStarts(start time.Time, count int, bucketSeconds int) []time.Time {
	out := make([]time.Time, 0, count)
	cursor := start.UTC()
	for i := 0; i < count; i++ {
		out = append(out, cursor)
		cursor = cursor.Add(time.Duration(bucketSeconds) * time.Second)
	}
	return out
}

func groupOpsModelHealthBuckets(buckets []*OpsModelHealthBucket) map[string]map[int64]*OpsModelHealthBucket {
	out := make(map[string]map[int64]*OpsModelHealthBucket)
	for _, bucket := range buckets {
		if bucket == nil || strings.TrimSpace(bucket.Model) == "" {
			continue
		}
		bucket.Platform = normalizeOpsStatusPlatform(bucket.Platform)
		key := opsModelKey(bucket.Platform, bucket.Model)
		if out[key] == nil {
			out[key] = make(map[int64]*OpsModelHealthBucket)
		}
		classifyOpsHealthPoint(&bucket.OpsHealthHistoryPoint)
		out[key][bucket.BucketStart.UTC().Unix()] = bucket
	}
	return out
}

func buildOpsModelHistory(starts []time.Time, raw map[int64]*OpsModelHealthBucket, bucketSeconds int) []*OpsHealthHistoryPoint {
	out := make([]*OpsHealthHistoryPoint, 0, len(starts))
	for _, start := range starts {
		if bucket := raw[start.UTC().Unix()]; bucket != nil {
			point := bucket.OpsHealthHistoryPoint
			classifyOpsHealthPoint(&point)
			out = append(out, &point)
			continue
		}
		out = append(out, &OpsHealthHistoryPoint{
			BucketStart: start.UTC(),
			BucketEnd:   start.UTC().Add(time.Duration(bucketSeconds) * time.Second),
			Status:      "idle",
		})
	}
	return out
}

func modelHistories(items []*OpsModelStatusItem) [][]*OpsHealthHistoryPoint {
	out := make([][]*OpsHealthHistoryPoint, 0, len(items))
	for _, item := range items {
		if item != nil && len(item.History) > 0 {
			out = append(out, item.History)
		}
	}
	return out
}

func aggregateOpsHealthHistories(histories [][]*OpsHealthHistoryPoint) []*OpsHealthHistoryPoint {
	if len(histories) == 0 {
		return []*OpsHealthHistoryPoint{}
	}
	out := make([]*OpsHealthHistoryPoint, len(histories[0]))
	for _, history := range histories {
		for idx, point := range history {
			if point == nil || idx >= len(out) {
				continue
			}
			if out[idx] == nil {
				out[idx] = &OpsHealthHistoryPoint{
					BucketStart: point.BucketStart,
					BucketEnd:   point.BucketEnd,
				}
			}
			mergeOpsHealthPoint(out[idx], point)
		}
	}
	for _, point := range out {
		if point != nil {
			finalizeOpsHealthPoint(point)
		}
	}
	return out
}

func mergeOpsHealthHistory(base, next []*OpsHealthHistoryPoint) []*OpsHealthHistoryPoint {
	if len(base) == 0 {
		copied := make([]*OpsHealthHistoryPoint, len(next))
		for i, point := range next {
			if point == nil {
				continue
			}
			cp := *point
			copied[i] = &cp
		}
		return copied
	}
	for idx, point := range next {
		if point == nil || idx >= len(base) {
			continue
		}
		if base[idx] == nil {
			cp := *point
			base[idx] = &cp
			continue
		}
		mergeOpsHealthPoint(base[idx], point)
	}
	return base
}

func mergeOpsHealthPoint(base, next *OpsHealthHistoryPoint) {
	if base == nil || next == nil {
		return
	}
	if base.BucketStart.IsZero() {
		base.BucketStart = next.BucketStart
	}
	if base.BucketEnd.IsZero() {
		base.BucketEnd = next.BucketEnd
	}
	base.RequestCount += next.RequestCount
	base.SuccessCount += next.SuccessCount
	base.ErrorCount += next.ErrorCount
	base.TokenConsumed += next.TokenConsumed
	base.AvgLatencyMs = mergeWeightedLatency(base.AvgLatencyMs, base.RequestCount-next.RequestCount, next.AvgLatencyMs, next.RequestCount)
	base.P50LatencyMs = maxLatencyPointer(base.P50LatencyMs, next.P50LatencyMs)
	base.P95LatencyMs = maxLatencyPointer(base.P95LatencyMs, next.P95LatencyMs)
	base.P99LatencyMs = maxLatencyPointer(base.P99LatencyMs, next.P99LatencyMs)
}

func finalizeOpsHealthPoint(point *OpsHealthHistoryPoint) {
	if point == nil {
		return
	}
	if point.RequestCount > 0 {
		rate := roundFloat(float64(point.SuccessCount)*100/float64(point.RequestCount), 2)
		point.SuccessRate = &rate
	}
	classifyOpsHealthPoint(point)
}

func classifyOpsHealthPoint(point *OpsHealthHistoryPoint) {
	if point == nil {
		return
	}
	if point.RequestCount == 0 {
		point.Status = "idle"
		point.SuccessRate = nil
		return
	}
	if point.SuccessRate == nil {
		rate := roundFloat(float64(point.SuccessCount)*100/float64(point.RequestCount), 2)
		point.SuccessRate = &rate
	}
	switch {
	case point.SuccessCount == 0 && point.ErrorCount > 0:
		point.Status = "failed"
	case point.ErrorCount > 0:
		point.Status = "degraded"
	case point.P95LatencyMs != nil && *point.P95LatencyMs >= opsLatencyDegradedP95Ms:
		point.Status = "degraded"
	default:
		point.Status = "operational"
	}
}

func classifyOpsGatewayRouteHealth(route *OpsGatewayRouteHealth) {
	if route == nil {
		return
	}
	point := &OpsHealthHistoryPoint{
		RequestCount:  route.RequestCount,
		SuccessCount:  route.SuccessCount,
		ErrorCount:    route.ErrorCount,
		SuccessRate:   route.SuccessRate,
		TokenConsumed: route.TokenConsumed,
		AvgLatencyMs:  route.AvgLatencyMs,
		P50LatencyMs:  route.P50LatencyMs,
		P95LatencyMs:  route.P95LatencyMs,
		P99LatencyMs:  route.P99LatencyMs,
	}
	classifyOpsHealthPoint(point)
	route.Status = point.Status
	if point.SuccessRate != nil {
		rate := roundFloat(*point.SuccessRate, 2)
		route.SuccessRate = &rate
	}
}

func mergeLatencyPointer(base, next *float64) *float64 {
	if base != nil {
		return base
	}
	return next
}

func maxLatencyPointer(base, next *float64) *float64 {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}
	if *next > *base {
		return next
	}
	return base
}

func mergeWeightedLatency(base *float64, baseCount int64, next *float64, nextCount int64) *float64 {
	if base == nil {
		return next
	}
	if next == nil || nextCount <= 0 {
		return base
	}
	if baseCount <= 0 {
		return next
	}
	merged := ((*base * float64(baseCount)) + (*next * float64(nextCount))) / float64(baseCount+nextCount)
	merged = roundFloat(merged, 2)
	return &merged
}

func normalizeOpsStatusPlatform(platform string) string {
	platform = strings.TrimSpace(strings.ToLower(platform))
	if platform == "" {
		return "unknown"
	}
	return platform
}

func opsModelKey(platform, model string) string {
	return normalizeOpsStatusPlatform(platform) + "\x00" + strings.TrimSpace(model)
}

func sortedSourceFlags(flags map[string]struct{}) []string {
	out := make([]string, 0, len(flags))
	for k := range flags {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasSource(flags []string, source string) bool {
	for _, flag := range flags {
		if flag == source {
			return true
		}
	}
	return false
}

func opsModelStatusRank(status OpsModelStatus) int {
	switch status {
	case OpsModelStatusFailed:
		return 0
	case OpsModelStatusRateLimited:
		return 1
	case OpsModelStatusDegraded:
		return 2
	case OpsModelStatusUnknown:
		return 3
	case OpsModelStatusOrphanedHistory:
		return 4
	case OpsModelStatusNoRecentTraffic:
		return 5
	case OpsModelStatusOperational:
		return 6
	default:
		return 7
	}
}
