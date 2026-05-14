package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	notificationdomain "gos/internal/domain/notification"
)

type NotificationHandler struct {
	manager *usecase.NotificationManager
	authz   RequestAuthorizer
}

// NewNotificationHandler 创建并返回对应组件实例。
func NewNotificationHandler(manager *usecase.NotificationManager, authz RequestAuthorizer) *NotificationHandler {
	return &NotificationHandler{manager: manager, authz: authz}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *NotificationHandler) RegisterRoutes(router gin.IRouter) {
	if h == nil {
		return
	}
	router.GET("/notification-sources", h.ListSources)
	router.POST("/notification-sources/actions/test-webhook", h.TestSourceWebhook)
	router.GET("/notification-sources/:id", h.GetSource)
	router.POST("/notification-sources", h.CreateSource)
	router.PUT("/notification-sources/:id", h.UpdateSource)
	router.DELETE("/notification-sources/:id", h.DeleteSource)

	router.GET("/notification-markdown-templates", h.ListMarkdownTemplates)
	router.GET("/notification-markdown-templates/:id", h.GetMarkdownTemplate)
	router.POST("/notification-markdown-templates", h.CreateMarkdownTemplate)
	router.PUT("/notification-markdown-templates/:id", h.UpdateMarkdownTemplate)
	router.DELETE("/notification-markdown-templates/:id", h.DeleteMarkdownTemplate)

	router.GET("/notification-hooks", h.ListHooks)
	router.GET("/notification-hooks/:id", h.GetHook)
	router.POST("/notification-hooks", h.CreateHook)
	router.PUT("/notification-hooks/:id", h.UpdateHook)
	router.DELETE("/notification-hooks/:id", h.DeleteHook)
}

type NotificationSourceListResponse struct {
	Data     []usecase.NotificationSourceOutput `json:"data"`
	Page     int                                `json:"page"`
	PageSize int                                `json:"page_size"`
	Total    int64                              `json:"total"`
}

type NotificationSourceDataResponse struct {
	Data usecase.NotificationSourceOutput `json:"data"`
}

type NotificationSourceWebhookTestResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

type NotificationMarkdownTemplateListResponse struct {
	Data     []usecase.NotificationMarkdownTemplateOutput `json:"data"`
	Page     int                                          `json:"page"`
	PageSize int                                          `json:"page_size"`
	Total    int64                                        `json:"total"`
}

type NotificationMarkdownTemplateDataResponse struct {
	Data usecase.NotificationMarkdownTemplateOutput `json:"data"`
}

type NotificationHookListResponse struct {
	Data     []usecase.NotificationHookOutput `json:"data"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
	Total    int64                            `json:"total"`
}

type NotificationHookDataResponse struct {
	Data usecase.NotificationHookOutput `json:"data"`
}

type upsertNotificationSourceRequest struct {
	Name              string `json:"name"`
	SourceType        string `json:"source_type"`
	WebhookURL        string `json:"webhook_url"`
	VerificationParam string `json:"verification_param"`
	Enabled           bool   `json:"enabled"`
	Remark            string `json:"remark"`
}

type testNotificationSourceWebhookRequest struct {
	SourceID          string `json:"source_id"`
	Name              string `json:"name"`
	SourceType        string `json:"source_type"`
	WebhookURL        string `json:"webhook_url"`
	VerificationParam string `json:"verification_param"`
}

type notificationMarkdownTemplateConditionRequest struct {
	ParamKey      string `json:"param_key"`
	Operator      string `json:"operator"`
	ExpectedValue string `json:"expected_value"`
	MarkdownText  string `json:"markdown_text"`
}

type upsertNotificationMarkdownTemplateRequest struct {
	Name          string                                         `json:"name"`
	TitleTemplate string                                         `json:"title_template"`
	BodyTemplate  string                                         `json:"body_template"`
	Conditions    []notificationMarkdownTemplateConditionRequest `json:"conditions"`
	Enabled       bool                                           `json:"enabled"`
	Remark        string                                         `json:"remark"`
}

type upsertNotificationHookRequest struct {
	Name               string `json:"name"`
	SourceID           string `json:"source_id"`
	MarkdownTemplateID string `json:"markdown_template_id"`
	Enabled            bool   `json:"enabled"`
	Remark             string `json:"remark"`
}

// ListSources 查询Sources列表。
// @Summary      查询Sources列表
// @Description  查询Sources列表，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources [get]
func (h *NotificationHandler) ListSources(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value := raw == "1" || strings.EqualFold(raw, "true")
		enabled = &value
	}
	output, err := h.manager.ListSources(c.Request.Context(), notificationdomain.SourceListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Type:     notificationdomain.SourceType(strings.ToLower(strings.TrimSpace(c.Query("source_type")))),
		Enabled:  enabled,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output.Items, "page": resolvedPage(page), "page_size": resolvedPageSize(pageSize), "total": output.Total})
}

// GetSource 获取Source详情。
// @Summary      获取Source详情
// @Description  获取Source详情，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources/{id} [get]
func (h *NotificationHandler) GetSource(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	output, err := h.manager.GetSource(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// CreateSource 创建Source。
// @Summary      创建Source
// @Description  创建Source，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources [post]
func (h *NotificationHandler) CreateSource(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.CreateSource(c.Request.Context(), usecase.CreateNotificationSourceInput{
		Name:              req.Name,
		SourceType:        req.SourceType,
		WebhookURL:        req.WebhookURL,
		VerificationParam: req.VerificationParam,
		Enabled:           req.Enabled,
		Remark:            req.Remark,
		CreatedBy:         currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// TestSourceWebhook 发送测试消息验证通知源 Webhook。
// @Summary      测试通知源 Webhook
// @Description  发送模拟通知验证通知源 Webhook 是否可达。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources/actions/test-webhook [post]
func (h *NotificationHandler) TestSourceWebhook(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req testNotificationSourceWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.TestSourceWebhook(c.Request.Context(), usecase.TestNotificationSourceWebhookInput{
		SourceID:          req.SourceID,
		Name:              req.Name,
		SourceType:        req.SourceType,
		WebhookURL:        req.WebhookURL,
		VerificationParam: req.VerificationParam,
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, NotificationSourceWebhookTestResponse{
		Success:    output.Success,
		Message:    output.Message,
		StatusCode: output.StatusCode,
	})
}

// UpdateSource 更新Source。
// @Summary      更新Source
// @Description  更新Source，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources/{id} [put]
func (h *NotificationHandler) UpdateSource(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.UpdateSource(c.Request.Context(), c.Param("id"), usecase.UpdateNotificationSourceInput{
		Name:              req.Name,
		SourceType:        req.SourceType,
		WebhookURL:        req.WebhookURL,
		VerificationParam: req.VerificationParam,
		Enabled:           req.Enabled,
		Remark:            req.Remark,
		UpdatedBy:         currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteSource 删除Source。
// @Summary      删除Source
// @Description  删除Source，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-sources/{id} [delete]
func (h *NotificationHandler) DeleteSource(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	if err := h.manager.DeleteSource(c.Request.Context(), c.Param("id")); err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListMarkdownTemplates 查询Markdown Templates列表。
// @Summary      查询Markdown Templates列表
// @Description  查询Markdown Templates列表，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-markdown-templates [get]
func (h *NotificationHandler) ListMarkdownTemplates(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value := raw == "1" || strings.EqualFold(raw, "true")
		enabled = &value
	}
	output, err := h.manager.ListMarkdownTemplates(c.Request.Context(), notificationdomain.MarkdownTemplateListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Enabled:  enabled,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output.Items, "page": resolvedPage(page), "page_size": resolvedPageSize(pageSize), "total": output.Total})
}

// GetMarkdownTemplate 获取Markdown Template详情。
// @Summary      获取Markdown Template详情
// @Description  获取Markdown Template详情，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-markdown-templates/{id} [get]
func (h *NotificationHandler) GetMarkdownTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	output, err := h.manager.GetMarkdownTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// CreateMarkdownTemplate 创建Markdown Template。
// @Summary      创建Markdown Template
// @Description  创建Markdown Template，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-markdown-templates [post]
func (h *NotificationHandler) CreateMarkdownTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationMarkdownTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.CreateMarkdownTemplate(c.Request.Context(), usecase.CreateNotificationMarkdownTemplateInput{
		Name:          req.Name,
		TitleTemplate: req.TitleTemplate,
		BodyTemplate:  req.BodyTemplate,
		Conditions:    toNotificationMarkdownConditionInputs(req.Conditions),
		Enabled:       req.Enabled,
		Remark:        req.Remark,
		CreatedBy:     currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// UpdateMarkdownTemplate 更新Markdown Template。
// @Summary      更新Markdown Template
// @Description  更新Markdown Template，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-markdown-templates/{id} [put]
func (h *NotificationHandler) UpdateMarkdownTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationMarkdownTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.UpdateMarkdownTemplate(c.Request.Context(), c.Param("id"), usecase.UpdateNotificationMarkdownTemplateInput{
		Name:          req.Name,
		TitleTemplate: req.TitleTemplate,
		BodyTemplate:  req.BodyTemplate,
		Conditions:    toNotificationMarkdownConditionInputs(req.Conditions),
		Enabled:       req.Enabled,
		Remark:        req.Remark,
		UpdatedBy:     currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteMarkdownTemplate 删除Markdown Template。
// @Summary      删除Markdown Template
// @Description  删除Markdown Template，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-markdown-templates/{id} [delete]
func (h *NotificationHandler) DeleteMarkdownTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	if err := h.manager.DeleteMarkdownTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListHooks 查询Hooks列表。
// @Summary      查询Hooks列表
// @Description  查询Hooks列表，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-hooks [get]
func (h *NotificationHandler) ListHooks(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "system.notification.manage", "release.template.manage") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value := raw == "1" || strings.EqualFold(raw, "true")
		enabled = &value
	}
	output, err := h.manager.ListHooks(c.Request.Context(), notificationdomain.HookListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Enabled:  enabled,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output.Items, "page": resolvedPage(page), "page_size": resolvedPageSize(pageSize), "total": output.Total})
}

// GetHook 获取Hook详情。
// @Summary      获取Hook详情
// @Description  获取Hook详情，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-hooks/{id} [get]
func (h *NotificationHandler) GetHook(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "system.notification.manage", "release.template.manage") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	output, err := h.manager.GetHook(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// CreateHook 创建Hook。
// @Summary      创建Hook
// @Description  创建Hook，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-hooks [post]
func (h *NotificationHandler) CreateHook(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationHookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.CreateHook(c.Request.Context(), usecase.CreateNotificationHookInput{
		Name:               req.Name,
		SourceID:           req.SourceID,
		MarkdownTemplateID: req.MarkdownTemplateID,
		Enabled:            req.Enabled,
		Remark:             req.Remark,
		CreatedBy:          currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// UpdateHook 更新Hook。
// @Summary      更新Hook
// @Description  更新Hook，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-hooks/{id} [put]
func (h *NotificationHandler) UpdateHook(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	var req upsertNotificationHookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.UpdateHook(c.Request.Context(), c.Param("id"), usecase.UpdateNotificationHookInput{
		Name:               req.Name,
		SourceID:           req.SourceID,
		MarkdownTemplateID: req.MarkdownTemplateID,
		Enabled:            req.Enabled,
		Remark:             req.Remark,
		UpdatedBy:          currentUserDisplay(c),
	})
	if err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteHook 删除Hook。
// @Summary      删除Hook
// @Description  删除Hook，并按统一响应结构返回处理结果。
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /notification-hooks/{id} [delete]
func (h *NotificationHandler) DeleteHook(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.notification.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "notification manager is not configured"})
		return
	}
	if err := h.manager.DeleteHook(c.Request.Context(), c.Param("id")); err != nil {
		writeNotificationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// toNotificationMarkdownConditionInputs 将领域对象转换为接口响应结构。
func toNotificationMarkdownConditionInputs(items []notificationMarkdownTemplateConditionRequest) []usecase.NotificationMarkdownTemplateConditionInput {
	result := make([]usecase.NotificationMarkdownTemplateConditionInput, 0, len(items))
	for _, item := range items {
		result = append(result, usecase.NotificationMarkdownTemplateConditionInput{
			ParamKey:      item.ParamKey,
			Operator:      item.Operator,
			ExpectedValue: item.ExpectedValue,
			MarkdownText:  item.MarkdownText,
		})
	}
	return result
}

// currentUserDisplay 封装当前模块的业务处理逻辑。
func currentUserDisplay(c *gin.Context) string {
	if currentUser, ok := getCurrentUser(c); ok {
		if display := strings.TrimSpace(currentUser.DisplayName); display != "" {
			return display
		}
		if username := strings.TrimSpace(currentUser.Username); username != "" {
			return username
		}
		return strings.TrimSpace(currentUser.ID)
	}
	return ""
}

// writeNotificationHTTPError 写入处理结果或错误信息。
func writeNotificationHTTPError(c *gin.Context, err error) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, notificationdomain.ErrSourceNotFound), errors.Is(err, notificationdomain.ErrMarkdownTemplateNotFound), errors.Is(err, notificationdomain.ErrHookNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
