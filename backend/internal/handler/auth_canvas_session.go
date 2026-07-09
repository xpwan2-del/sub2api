package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

type CanvasSessionResponse struct {
	User CanvasSessionUser `json:"user"`
}

type CanvasSessionUser struct {
	ID          int64   `json:"id"`
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	Role        string  `json:"role"`
	AvatarURL   string  `json:"avatar_url"`
	Balance     float64 `json:"balance"`
	Concurrency int     `json:"concurrency"`
	Status      string  `json:"status"`
}

// GetCanvasSession returns the minimal authenticated TOP-AI user profile used by
// external apps mounted under the same domain, such as Infinite Canvas.
func (h *AuthHandler) GetCanvasSession(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, CanvasSessionResponse{
		User: CanvasSessionUser{
			ID:          user.ID,
			Email:       user.Email,
			Username:    user.Username,
			Role:        user.Role,
			AvatarURL:   user.AvatarURL,
			Balance:     user.Balance,
			Concurrency: user.Concurrency,
			Status:      user.Status,
		},
	})
}
