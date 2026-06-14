package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/gin-gonic/gin"
)

// UpstreamPriceHandler exposes the upstream-price-sync subsystem to admins:
// source CRUD, manual sync/test, change list/apply/dismiss, and a compare view.
//
// It depends only on services (no repository import) to satisfy depguard.
type UpstreamPriceHandler struct {
	sourceService *service.UpstreamPriceSourceService
	applyService  *service.UpstreamPriceApplyService
	syncService   *service.UpstreamPriceSyncService
}

// NewUpstreamPriceHandler creates a new upstream-price admin handler.
func NewUpstreamPriceHandler(
	sourceService *service.UpstreamPriceSourceService,
	applyService *service.UpstreamPriceApplyService,
	syncService *service.UpstreamPriceSyncService,
) *UpstreamPriceHandler {
	return &UpstreamPriceHandler{
		sourceService: sourceService,
		applyService:  applyService,
		syncService:   syncService,
	}
}

// ===== Sources =====

// CreateSourceRequest mirrors UpstreamPriceSource editable fields.
type CreateSourceRequest struct {
	Name                string                 `json:"name" binding:"required,max=100"`
	Platform            string                 `json:"platform"`
	BaseURL             string                 `json:"base_url" binding:"required,url"`
	PricingEndpoint     string                 `json:"pricing_endpoint" binding:"required"`
	APIKey              string                 `json:"api_key"`
	ParserType          string                 `json:"parser_type" binding:"required"`
	ParserConfig        map[string]interface{} `json:"parser_config"`
	ModelAliasMap       map[string]string      `json:"model_alias_map"`
	SyncIntervalMinutes int                    `json:"sync_interval_minutes"`
	Enabled             bool                   `json:"enabled"`
}

// UpdateSourceRequest mirrors editable fields for updates (all optional).
type UpdateSourceRequest struct {
	Name                *string                `json:"name" binding:"omitempty,max=100"`
	Platform            *string                `json:"platform"`
	BaseURL             *string                `json:"base_url" binding:"omitempty,url"`
	PricingEndpoint     *string                `json:"pricing_endpoint"`
	APIKey              *string                `json:"api_key"`
	ParserType          *string                `json:"parser_type"`
	ParserConfig        map[string]interface{} `json:"parser_config"`
	ModelAliasMap       map[string]string      `json:"model_alias_map"`
	SyncIntervalMinutes *int                   `json:"sync_interval_minutes"`
	Enabled             *bool                  `json:"enabled"`
}

// CreateSource POST /admin/upstream-price/sources
func (h *UpstreamPriceHandler) CreateSource(c *gin.Context) {
	var req CreateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	src := buildSourceFromCreate(&req)
	created, err := h.sourceService.Create(c.Request.Context(), src)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sourceToResponse(created))
}

// ListSources GET /admin/upstream-price/sources
func (h *UpstreamPriceHandler) ListSources(c *gin.Context) {
	list, err := h.sourceService.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*sourceResponse, 0, len(list))
	for _, s := range list {
		out = append(out, sourceToResponse(s))
	}
	response.Success(c, out)
}

// UpdateSource PUT /admin/upstream-price/sources/:id
func (h *UpstreamPriceHandler) UpdateSource(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	existing, getErr := h.sourceService.Get(c.Request.Context(), id)
	if getErr != nil {
		response.ErrorFrom(c, getErr)
		return
	}
	applySourceUpdate(existing, &req)
	if err := h.sourceService.Update(c.Request.Context(), existing); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updated, _ := h.sourceService.Get(c.Request.Context(), id)
	response.Success(c, sourceToResponse(updated))
}

// DeleteSource DELETE /admin/upstream-price/sources/:id
func (h *UpstreamPriceHandler) DeleteSource(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.sourceService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Source deleted successfully"})
}

// TestConnection POST /admin/upstream-price/sources/:id/test
func (h *UpstreamPriceHandler) TestConnection(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	src, getErr := h.sourceService.Get(c.Request.Context(), id)
	if getErr != nil {
		response.ErrorFrom(c, getErr)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	reachable, modelCount, testErr := h.sourceService.TestConnection(ctx, src)
	if testErr != nil {
		response.Success(c, gin.H{"reachable": false, "model_count": 0, "error": testErr.Error()})
		return
	}
	response.Success(c, gin.H{"reachable": reachable, "model_count": modelCount})
}

// SyncSource POST /admin/upstream-price/sources/:id/sync
func (h *UpstreamPriceHandler) SyncSource(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	if syncErr := h.syncService.SyncSource(ctx, id); syncErr != nil {
		response.ErrorFrom(c, syncErr)
		return
	}
	response.Success(c, gin.H{"message": "Source synced successfully"})
}

// ===== Changes =====

// ListChanges GET /admin/upstream-price/changes?source_id=&status=
func (h *UpstreamPriceHandler) ListChanges(c *gin.Context) {
	// source/apply/sync services don't expose a list-changes method directly;
	// the repo-level ListPendingChanges is invoked through a small helper on
	// the apply service. We fall back to an empty slice if filtering yields
	// nothing, per the API-empty-result convention.
	filters := service.ChangeFilters{
		Status: strings.TrimSpace(c.Query("status")),
		Limit:  200,
	}
	if sid := c.Query("source_id"); sid != "" {
		if v, perr := strconv.ParseInt(sid, 10, 64); perr == nil {
			filters.SourceID = v
		}
	}
	if filters.Status == "" {
		filters.Status = service.UpstreamPriceChangeStatusPending
	}

	changes, err := h.applyService.ListChanges(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*changeResponse, 0, len(changes))
	for _, ch := range changes {
		out = append(out, changeToResponse(ch))
	}
	response.Success(c, out)
}

// ApplyChangeRequest is the body for applying a single change.
type ApplyChangeRequest struct {
	Mode     string `json:"mode" binding:"required,oneof=follow_cost lock_price"`
	TargetID int64  `json:"target_id"`
}

// ApplyChange POST /admin/upstream-price/changes/:id/apply
func (h *UpstreamPriceHandler) ApplyChange(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req ApplyChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	adminID := adminUserID(c)
	if applyErr := h.applyService.Apply(c.Request.Context(), service.ApplyRequest{
		ChangeID: id,
		Mode:     service.ApplyMode(req.Mode),
		TargetID: req.TargetID,
	}, adminID); applyErr != nil {
		response.ErrorFrom(c, applyErr)
		return
	}
	response.Success(c, gin.H{"message": "Change applied successfully"})
}

// DismissChange POST /admin/upstream-price/changes/:id/dismiss
func (h *UpstreamPriceHandler) DismissChange(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	adminID := adminUserID(c)
	if derr := h.applyService.Dismiss(c.Request.Context(), id, adminID); derr != nil {
		response.ErrorFrom(c, derr)
		return
	}
	response.Success(c, gin.H{"message": "Change dismissed successfully"})
}

// ===== Compare =====

// ComparePrices GET /admin/upstream-price/compare?source_id=
// Returns a row-per-model comparison of upstream reference price vs local
// channel pricing. Returns [] when no data.
func (h *UpstreamPriceHandler) ComparePrices(c *gin.Context) {
	sourceID, _ := strconv.ParseInt(c.Query("source_id"), 10, 64)
	if sourceID == 0 {
		response.Success(c, []compareRow{})
		return
	}

	rows, err := h.applyService.ComparePrices(c.Request.Context(), sourceID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rows)
}

// ===== helpers =====

// parseIDParam is provided by payment_handler.go (same package) and returns
// (int64, bool); it already emits response.BadRequest on failure.

// adminUserID extracts the admin user id from gin context; 0 if unauthenticated.
func adminUserID(c *gin.Context) int64 {
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		return subject.UserID
	}
	return 0
}

// ===== DTO =====

type sourceResponse struct {
	ID                  int64                  `json:"id"`
	Name                string                 `json:"name"`
	Platform            string                 `json:"platform"`
	BaseURL             string                 `json:"base_url"`
	PricingEndpoint     string                 `json:"pricing_endpoint"`
	ParserType          string                 `json:"parser_type"`
	ParserConfig        map[string]interface{} `json:"parser_config"`
	ModelAliasMap       map[string]string      `json:"model_alias_map"`
	SyncIntervalMinutes int                    `json:"sync_interval_minutes"`
	Enabled             bool                   `json:"enabled"`
	LastSyncAt          *string                `json:"last_sync_at,omitempty"`
	LastSyncStatus      string                 `json:"last_sync_status,omitempty"`
	LastSyncError       string                 `json:"last_sync_error,omitempty"`
	CreatedAt           string                 `json:"created_at"`
	UpdatedAt           string                 `json:"updated_at"`
}

func sourceToResponse(s *dbent.UpstreamPriceSource) *sourceResponse {
	if s == nil {
		return nil
	}
	r := &sourceResponse{
		ID:                  s.ID,
		Name:                s.Name,
		Platform:            s.Platform,
		BaseURL:             s.BaseURL,
		PricingEndpoint:     s.PricingEndpoint,
		ParserType:          s.ParserType,
		ParserConfig:        s.ParserConfig,
		ModelAliasMap:       s.ModelAliasMap,
		SyncIntervalMinutes: s.SyncIntervalMinutes,
		Enabled:             s.Enabled,
		LastSyncStatus:      s.LastSyncStatus,
		LastSyncError:       s.LastSyncError,
		CreatedAt:           formatTime(s.CreatedAt),
		UpdatedAt:           formatTime(s.UpdatedAt),
	}
	if s.LastSyncAt != nil {
		ts := formatTime(*s.LastSyncAt)
		r.LastSyncAt = &ts
	}
	if r.ParserConfig == nil {
		r.ParserConfig = map[string]interface{}{}
	}
	if r.ModelAliasMap == nil {
		r.ModelAliasMap = map[string]string{}
	}
	return r
}

func buildSourceFromCreate(req *CreateSourceRequest) *dbent.UpstreamPriceSource {
	return &dbent.UpstreamPriceSource{
		Name:                req.Name,
		Platform:            req.Platform,
		BaseURL:             req.BaseURL,
		PricingEndpoint:     req.PricingEndpoint,
		APIKey:              req.APIKey,
		ParserType:          req.ParserType,
		ParserConfig:        req.ParserConfig,
		ModelAliasMap:       req.ModelAliasMap,
		SyncIntervalMinutes: req.SyncIntervalMinutes,
		Enabled:             req.Enabled,
	}
}

func applySourceUpdate(s *dbent.UpstreamPriceSource, req *UpdateSourceRequest) {
	if req.Name != nil {
		s.Name = *req.Name
	}
	if req.Platform != nil {
		s.Platform = *req.Platform
	}
	if req.BaseURL != nil {
		s.BaseURL = *req.BaseURL
	}
	if req.PricingEndpoint != nil {
		s.PricingEndpoint = *req.PricingEndpoint
	}
	if req.APIKey != nil {
		s.APIKey = *req.APIKey
	}
	if req.ParserType != nil {
		s.ParserType = *req.ParserType
	}
	if req.ParserConfig != nil {
		s.ParserConfig = req.ParserConfig
	}
	if req.ModelAliasMap != nil {
		s.ModelAliasMap = req.ModelAliasMap
	}
	if req.SyncIntervalMinutes != nil {
		s.SyncIntervalMinutes = *req.SyncIntervalMinutes
	}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
}

type changeResponse struct {
	ID                  int64    `json:"id"`
	SourceID            int64    `json:"source_id"`
	ModelName           string   `json:"model_name"`
	LocalModelName      string   `json:"local_model_name"`
	ChangeType          string   `json:"change_type"`
	PrevInputPrice      *float64 `json:"prev_input_price"`
	PrevOutputPrice     *float64 `json:"prev_output_price"`
	CurrInputPrice      float64  `json:"curr_input_price"`
	CurrOutputPrice     float64  `json:"curr_output_price"`
	InputDeltaPct       float64  `json:"input_delta_pct"`
	OutputDeltaPct      float64  `json:"output_delta_pct"`
	SuggestedInputPrice float64  `json:"suggested_input_price"`
	SuggestedMultiplier *float64 `json:"suggested_multiplier"`
	Status              string   `json:"status"`
	DetectedAt          string   `json:"detected_at"`
}

func changeToResponse(ch *dbent.UpstreamPriceChange) *changeResponse {
	if ch == nil {
		return nil
	}
	return &changeResponse{
		ID:                  ch.ID,
		SourceID:            ch.SourceID,
		ModelName:           ch.ModelName,
		LocalModelName:      ch.LocalModelName,
		ChangeType:          ch.ChangeType,
		PrevInputPrice:      ch.PrevInputPrice,
		PrevOutputPrice:     ch.PrevOutputPrice,
		CurrInputPrice:      ch.CurrInputPrice,
		CurrOutputPrice:     ch.CurrOutputPrice,
		InputDeltaPct:       ch.InputDeltaPct,
		OutputDeltaPct:      ch.OutputDeltaPct,
		SuggestedInputPrice: ch.SuggestedInputPrice,
		SuggestedMultiplier: ch.SuggestedMultiplier,
		Status:              ch.Status,
		DetectedAt:          formatTime(ch.DetectedAt),
	}
}

// compareRow is a single model's upstream-vs-local price comparison.
type compareRow = service.PriceCompareRow

// formatTime renders a time.Time as RFC3339; empty string on zero value.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
