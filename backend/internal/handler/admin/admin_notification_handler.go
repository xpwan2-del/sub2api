package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/gin-gonic/gin"
)

// AdminNotificationHandler exposes the admin in-site notification endpoints
// (unread list/count, mark read, mark all read). Depends only on services.
type AdminNotificationHandler struct {
	notifService *service.AdminNotificationService
}

// NewAdminNotificationHandler creates a new admin notification handler.
func NewAdminNotificationHandler(notifService *service.AdminNotificationService) *AdminNotificationHandler {
	return &AdminNotificationHandler{notifService: notifService}
}

// ListUnread GET /admin/admin-notifications/unread
func (h *AdminNotificationHandler) ListUnread(c *gin.Context) {
	userID := adminUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "admin user not found in context")
		return
	}
	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := h.notifService.ListUnread(c.Request.Context(), userID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]adminNotificationResponse, 0, len(items))
	for _, n := range items {
		out = append(out, notificationToResponse(n))
	}
	response.Success(c, out)
}

// CountUnread GET /admin/admin-notifications/unread/count
func (h *AdminNotificationHandler) CountUnread(c *gin.Context) {
	userID := adminUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "admin user not found in context")
		return
	}
	count, err := h.notifService.CountUnread(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"count": count})
}

// MarkRead POST /admin/admin-notifications/:id/read
func (h *AdminNotificationHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid notification id")
		return
	}
	userID := adminUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "admin user not found in context")
		return
	}
	if err := h.notifService.MarkRead(c.Request.Context(), userID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Notification marked as read"})
}

// MarkAllRead POST /admin/admin-notifications/read-all
func (h *AdminNotificationHandler) MarkAllRead(c *gin.Context) {
	userID := adminUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "admin user not found in context")
		return
	}
	if err := h.notifService.MarkAllRead(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "All notifications marked as read"})
}

// ===== DTO =====

type adminNotificationResponse struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Severity   string  `json:"severity"`
	TargetLink string  `json:"target_link,omitempty"`
	RelatedIDs []int64 `json:"related_ids"`
	CreatedAt  string  `json:"created_at"`
}

func notificationToResponse(n *dbent.AdminNotification) adminNotificationResponse {
	r := adminNotificationResponse{
		ID:         n.ID,
		Type:       n.Type,
		Title:      n.Title,
		Content:    n.Content,
		Severity:   n.Severity,
		TargetLink: n.TargetLink,
		RelatedIDs: n.RelatedIds,
		CreatedAt:  formatNotifTime(n.CreatedAt),
	}
	if r.RelatedIDs == nil {
		r.RelatedIDs = []int64{}
	}
	return r
}

// formatNotifTime renders time as RFC3339; empty string on zero value.
// Dedicated helper to keep this file independent of upstream_price_handler.
func formatNotifTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
