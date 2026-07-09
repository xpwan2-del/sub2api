package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetModelStatusSnapshot returns the admin model status page snapshot.
// GET /api/v1/admin/ops/model-status/snapshot
func (h *OpsHandler) GetModelStatusSnapshot(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err == nil && v > 0 && v < pageSize {
			pageSize = v
		}
	}

	filter := &service.OpsModelStatusFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("provider")),
		Status:    strings.TrimSpace(c.Query("status")),
		Query:     strings.TrimSpace(c.Query("q")),
		Page:      page,
		PageSize:  pageSize,
	}
	if filter.Platform == "" {
		filter.Platform = strings.TrimSpace(c.Query("platform"))
	}

	data, err := h.opsService.GetModelStatusSnapshot(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}
