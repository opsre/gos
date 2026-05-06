package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	announcementdomain "gos/internal/domain/announcement"
	"gos/internal/application/usecase"
)

type AnnouncementHandler struct {
	manager *usecase.AnnouncementManager
	authz   RequestAuthorizer
}

func NewAnnouncementHandler(manager *usecase.AnnouncementManager, authz RequestAuthorizer) *AnnouncementHandler {
	return &AnnouncementHandler{manager: manager, authz: authz}
}

func (h *AnnouncementHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/announcements", h.List)
	router.GET("/announcements/active", h.ListActive)
	router.GET("/announcements/:id", h.GetByID)
	router.POST("/announcements", h.Create)
	router.PUT("/announcements/:id", h.Update)
	router.PUT("/announcements/:id/toggle", h.ToggleEnabled)
	router.DELETE("/announcements/:id", h.Delete)
}

type AnnouncementRequest struct {
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content"`
	Enabled   *bool  `json:"enabled"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

type AnnouncementResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	CreatedBy string `json:"created_by"`
	UpdatedBy string `json:"updated_by"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ToggleRequest struct {
	Enabled bool `json:"enabled"`
}

func toAnnouncementResponse(item announcementdomain.Announcement) AnnouncementResponse {
	return AnnouncementResponse{
		ID:        item.ID,
		Title:     item.Title,
		Content:   item.Content,
		Enabled:   item.Enabled,
		StartTime: item.StartTime.Format("2006-01-02 15:04"),
		EndTime:   item.EndTime.Format("2006-01-02 15:04"),
		CreatedBy: item.CreatedBy,
		UpdatedBy: item.UpdatedBy,
		CreatedAt: item.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: item.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func parseTime(c *gin.Context, value string, field string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": field + " is required"})
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02 15:04", value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": field + " must be in format YYYY-MM-DD HH:mm"})
		return time.Time{}, false
	}
	return t, true
}

// ListActive 查询当前有效的公告，无需权限。
func (h *AnnouncementHandler) ListActive(c *gin.Context) {
	items, err := h.manager.ListActive(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	resp := make([]AnnouncementResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toAnnouncementResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// List 查询公告列表。
func (h *AnnouncementHandler) List(c *gin.Context) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		if user.Role != "admin" {
			return
		}
	}
	page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page_size", "20")))
	activeStr := strings.TrimSpace(c.Query("active"))
	var active *bool
	if activeStr == "true" {
		v := true
		active = &v
	} else if activeStr == "false" {
		v := false
		active = &v
	}
	items, total, err := h.manager.List(c.Request.Context(), announcementdomain.ListFilter{
		Keyword:  c.Query("keyword"),
		Active:   active,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	resp := make([]AnnouncementResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toAnnouncementResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

// GetByID 查询公告详情。
func (h *AnnouncementHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAnnouncementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toAnnouncementResponse(item)})
}

// Create 创建公告。
func (h *AnnouncementHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	startTime, ok := parseTime(c, req.StartTime, "start_time")
	if !ok {
		return
	}
	endTime, ok := parseTime(c, req.EndTime, "end_time")
	if !ok {
		return
	}
	currentUser, _ := getCurrentUser(c)
	createdBy := strings.TrimSpace(currentUser.DisplayName)
	if createdBy == "" {
		createdBy = strings.TrimSpace(currentUser.Username)
	}
	item, err := h.manager.Create(c.Request.Context(), usecase.CreateAnnouncementInput{
		Title:     req.Title,
		Content:   req.Content,
		Enabled:   req.Enabled == nil || *req.Enabled,
		StartTime: startTime,
		EndTime:   endTime,
		CreatedBy: createdBy,
	})
	if err != nil {
		writeAnnouncementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toAnnouncementResponse(item)})
}

// Update 更新公告。
func (h *AnnouncementHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	startTime, ok := parseTime(c, req.StartTime, "start_time")
	if !ok {
		return
	}
	endTime, ok := parseTime(c, req.EndTime, "end_time")
	if !ok {
		return
	}
	currentUser, _ := getCurrentUser(c)
	updatedBy := strings.TrimSpace(currentUser.DisplayName)
	if updatedBy == "" {
		updatedBy = strings.TrimSpace(currentUser.Username)
	}
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), usecase.UpdateAnnouncementInput{
		Title:     req.Title,
		Content:   req.Content,
		Enabled:   req.Enabled == nil || *req.Enabled,
		StartTime: startTime,
		EndTime:   endTime,
		UpdatedBy: updatedBy,
	})
	if err != nil {
		writeAnnouncementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toAnnouncementResponse(item)})
}

// ToggleEnabled 切换公告启用状态。
func (h *AnnouncementHandler) ToggleEnabled(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	currentUser, _ := getCurrentUser(c)
	updatedBy := strings.TrimSpace(currentUser.DisplayName)
	if updatedBy == "" {
		updatedBy = strings.TrimSpace(currentUser.Username)
	}
	item, err := h.manager.ToggleEnabled(c.Request.Context(), c.Param("id"), req.Enabled, updatedBy)
	if err != nil {
		writeAnnouncementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toAnnouncementResponse(item)})
}

// Delete 删除公告。
func (h *AnnouncementHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeAnnouncementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func writeAnnouncementHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, announcementdomain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
