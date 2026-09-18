package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	releasedomain "gos/internal/domain/release"
	automationdomain "gos/internal/domain/releaseautomation"
)

const (
	permissionReleaseAutomationView   = "release.automation.view"
	permissionReleaseAutomationManage = "release.automation.manage"
)

// ReleaseAutomationHandler 暴露「发布自动化」的配置 CRUD 与 git 检查接口。
type ReleaseAutomationHandler struct {
	manager *usecase.ReleaseAutomationManager
	authz   RequestAuthorizer
}

func NewReleaseAutomationHandler(
	manager *usecase.ReleaseAutomationManager,
	authz RequestAuthorizer,
) *ReleaseAutomationHandler {
	return &ReleaseAutomationHandler{
		manager: manager,
		authz:   authz,
	}
}

func (h *ReleaseAutomationHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/release-automations", h.List)
	router.POST("/release-automations", h.Create)
	router.GET("/release-automations/:id", h.GetByID)
	router.PUT("/release-automations/:id", h.Update)
	router.DELETE("/release-automations/:id", h.Delete)
	router.POST("/release-automations/:id/check", h.Check)
}

type ReleaseAutomationParamRequest struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

type ReleaseAutomationRequest struct {
	Name          string                          `json:"name"`
	ApplicationID string                          `json:"application_id"`
	TemplateID    string                          `json:"template_id"`
	EnvCode       string                          `json:"env_code"`
	GitRef        string                          `json:"git_ref"`
	DispatchMode  string                          `json:"dispatch_mode"`
	Enabled       *bool                           `json:"enabled"`
	Params        []ReleaseAutomationParamRequest `json:"params"`
	Remark        string                          `json:"remark"`
	// CheckGit 缺省为 true：保存前先读一次分支 HEAD，读不到就拒绝落库。
	CheckGit *bool `json:"check_git"`
}

type ReleaseAutomationParamResponse struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

type ReleaseAutomationResponse struct {
	ID              string                           `json:"id"`
	Name            string                           `json:"name"`
	ApplicationID   string                           `json:"application_id"`
	ApplicationName string                           `json:"application_name"`
	TemplateID      string                           `json:"template_id"`
	TemplateName    string                           `json:"template_name"`
	EnvCode         string                           `json:"env_code"`
	GitRef          string                           `json:"git_ref"`
	DispatchMode    string                           `json:"dispatch_mode"`
	Enabled         bool                             `json:"enabled"`
	Params          []ReleaseAutomationParamResponse `json:"params"`
	LastSeenSHA     string                           `json:"last_seen_sha"`
	LastTriggered   string                           `json:"last_triggered_sha"`
	LastOrderID     string                           `json:"last_order_id"`
	// LastCheckedAt 为空表示这条配置还从未被轮询检查过，前端显示占位而不是 1970 年。
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastError     string     `json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ReleaseAutomationCheckResponse struct {
	Reachable bool   `json:"reachable"`
	HeadSHA   string `json:"head_sha"`
	Message   string `json:"message"`
}

func (h *ReleaseAutomationHandler) List(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationView, "", "") {
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
	filter := automationdomain.ListFilter{
		Keyword:       c.Query("keyword"),
		ApplicationID: c.Query("application_id"),
		Page:          page,
		PageSize:      pageSize,
	}
	// enabled 只在显式传 true/false 时过滤；未传、空串或 all 都表示「全部」，
	// 这样前端把空查询参数直接拼进 URL 时不会误过滤成「只看已停用」。
	if raw, exists := c.GetQuery("enabled"); exists {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "on":
			enabled := true
			filter.Enabled = &enabled
		case "0", "false", "no", "off":
			enabled := false
			filter.Enabled = &enabled
		}
	}
	items, total, err := h.manager.List(c.Request.Context(), filter)
	if err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	resp := make([]ReleaseAutomationResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseAutomationResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

func (h *ReleaseAutomationHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationView, "", "") {
		return
	}
	item, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toReleaseAutomationResponse(item)})
}

func (h *ReleaseAutomationHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationManage, "", "") {
		return
	}
	var req ReleaseAutomationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	operatorID, operatorName := currentOperator(c)
	item, err := h.manager.Create(c.Request.Context(), toReleaseAutomationInput(req, operatorID, operatorName))
	if err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toReleaseAutomationResponse(item)})
}

func (h *ReleaseAutomationHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationManage, "", "") {
		return
	}
	var req ReleaseAutomationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	operatorID, operatorName := currentOperator(c)
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), toReleaseAutomationInput(req, operatorID, operatorName))
	if err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toReleaseAutomationResponse(item)})
}

func (h *ReleaseAutomationHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationManage, "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Check 是只读的「检查 git 权限」：无论成功与否都返回 200，
// 由 reachable/message 表达结果，页面不需要区分 HTTP 错误分支。
func (h *ReleaseAutomationHandler) Check(c *gin.Context) {
	if !ensurePermission(c, h.authz, permissionReleaseAutomationView, "", "") {
		return
	}
	result, err := h.manager.CheckNow(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseAutomationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ReleaseAutomationCheckResponse{
		Reachable: result.Reachable,
		HeadSHA:   result.HeadSHA,
		Message:   result.Message,
	}})
}

// currentOperator 取当前登录用户，作为自动建单时的发起人。
func currentOperator(c *gin.Context) (userID string, userName string) {
	user, ok := getCurrentUser(c)
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(user.ID), resolveTriggeredBy(user)
}

func toReleaseAutomationInput(req ReleaseAutomationRequest, userID string, userName string) usecase.ReleaseAutomationInput {
	params := make([]usecase.CreateReleaseOrderParamInput, 0, len(req.Params))
	for _, item := range req.Params {
		// 与发布单创建接口保持同一套归一化规则：scope 小写、参数名去空格。
		params = append(params, usecase.CreateReleaseOrderParamInput{
			PipelineScope:     releasedomain.PipelineScope(strings.ToLower(strings.TrimSpace(item.PipelineScope))),
			ParamKey:          strings.ToLower(strings.TrimSpace(item.ParamKey)),
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			ParamValue:        strings.TrimSpace(item.ParamValue),
			ValueSource:       releasedomain.ValueSource(strings.TrimSpace(item.ValueSource)),
		})
	}
	return usecase.ReleaseAutomationInput{
		Name:           req.Name,
		ApplicationID:  req.ApplicationID,
		TemplateID:     req.TemplateID,
		EnvCode:        req.EnvCode,
		GitRef:         req.GitRef,
		DispatchMode:   automationdomain.DispatchMode(strings.TrimSpace(req.DispatchMode)),
		Enabled:        req.Enabled,
		Params:         params,
		Remark:         req.Remark,
		CheckGit:       req.CheckGit,
		OperatorUserID: userID,
		OperatorName:   userName,
	}
}

func toReleaseAutomationResponse(item automationdomain.Automation) ReleaseAutomationResponse {
	params := make([]ReleaseAutomationParamResponse, 0, len(item.Params))
	for _, param := range item.Params {
		params = append(params, ReleaseAutomationParamResponse{
			PipelineScope:     param.PipelineScope,
			ParamKey:          param.ParamKey,
			ExecutorParamName: param.ExecutorParamName,
			ParamValue:        param.ParamValue,
			ValueSource:       param.ValueSource,
		})
	}
	return ReleaseAutomationResponse{
		ID:              item.ID,
		Name:            item.Name,
		ApplicationID:   item.ApplicationID,
		ApplicationName: item.ApplicationName,
		TemplateID:      item.TemplateID,
		TemplateName:    item.TemplateName,
		EnvCode:         item.EnvCode,
		GitRef:          item.GitRef,
		DispatchMode:    string(item.DispatchMode),
		Enabled:         item.Enabled,
		Params:          params,
		LastSeenSHA:     item.LastSeenSHA,
		LastTriggered:   item.LastTriggeredSHA,
		LastOrderID:     item.LastOrderID,
		LastCheckedAt:   item.LastCheckedAt,
		LastError:       item.LastError,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}

// writeReleaseAutomationHTTPError 把领域错误映射成状态码。
//
// git 校验失败必须返回 400 + 可读原因：这是「保存前先验证分支可读」的接口契约，
// 前端会把 error 文案直接展示在表单上。
func writeReleaseAutomationHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrAutomationGitUnreachable):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, automationdomain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, automationdomain.ErrDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
