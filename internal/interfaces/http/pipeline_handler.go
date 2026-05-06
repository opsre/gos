package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/pipeline"
)

type PipelineHandler struct {
	syncer  *usecase.SyncPipelines
	query   *usecase.QueryPipeline
	binding *usecase.PipelineBindingManager
	manager *usecase.JenkinsPipelineManager
	authz   RequestAuthorizer
}

// NewPipelineHandler 创建并返回对应组件实例。
func NewPipelineHandler(
	syncer *usecase.SyncPipelines,
	query *usecase.QueryPipeline,
	binding *usecase.PipelineBindingManager,
	manager *usecase.JenkinsPipelineManager,
	authz RequestAuthorizer,
) *PipelineHandler {
	return &PipelineHandler{
		syncer:  syncer,
		query:   query,
		binding: binding,
		manager: manager,
		authz:   authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *PipelineHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/jenkins/pipelines/sync", h.Sync)
	router.POST("/jenkins/pipelines/raw", h.CreateRawPipeline)
	router.POST("/jenkins/pipelines/raw/preview-config-xml", h.PreviewRawPipelineConfigXML)
	router.GET("/pipelines", h.ListPipelines)
	router.GET("/pipelines/:id", h.GetPipelineByID)
	router.GET("/pipelines/:id/original-link", h.GetPipelineOriginalLink)
	router.GET("/pipelines/:id/config-xml", h.GetPipelineConfigXML)
	router.GET("/pipelines/:id/raw-script", h.GetPipelineRawScript)
	router.PUT("/pipelines/:id/raw", h.UpdateRawPipeline)
	router.DELETE("/pipelines/:id/raw", h.DeleteRawPipeline)
	router.POST("/pipelines/:id/verify", h.VerifyPipeline)

	router.POST("/applications/:id/pipeline-bindings", h.CreateBinding)
	router.GET("/applications/:id/pipeline-bindings", h.ListBindings)
	router.GET("/pipeline-bindings/:id", h.GetBindingByID)
	router.PUT("/pipeline-bindings/:id", h.UpdateBinding)
	router.DELETE("/pipeline-bindings/:id", h.DeleteBinding)
}

type PipelineResponse struct {
	ID             string     `json:"id"`
	Provider       string     `json:"provider"`
	JobFullName    string     `json:"job_full_name"`
	JobName        string     `json:"job_name"`
	JobURL         string     `json:"job_url"`
	Description    string     `json:"description"`
	CredentialRef  string     `json:"credential_ref"`
	DefaultBranch  string     `json:"default_branch"`
	Status         string     `json:"status"`
	LastVerifiedAt *time.Time `json:"last_verified_at"`
	LastSyncedAt   time.Time  `json:"last_synced_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type PipelineDataResponse struct {
	Data PipelineResponse `json:"data"`
}

type PipelineRawScriptDataResponse struct {
	Data struct {
		Pipeline        PipelineResponse `json:"pipeline"`
		DefinitionClass string           `json:"definition_class"`
		Description     string           `json:"description"`
		Script          string           `json:"script"`
		ScriptPath      string           `json:"script_path"`
		Sandbox         bool             `json:"sandbox"`
		FromSCM         bool             `json:"from_scm"`
	} `json:"data"`
}

type PipelineConfigXMLDataResponse struct {
	Data struct {
		Pipeline  PipelineResponse `json:"pipeline"`
		ConfigXML string           `json:"config_xml"`
	} `json:"data"`
}

type PipelineOriginalLinkDataResponse struct {
	Data struct {
		Pipeline     PipelineResponse `json:"pipeline"`
		OriginalLink string           `json:"original_link"`
	} `json:"data"`
}

type PipelineListResponse struct {
	Data     []PipelineResponse `json:"data"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int64              `json:"total"`
}

type VerifyPipelineResponse struct {
	Data struct {
		Verified bool             `json:"verified"`
		JobName  string           `json:"job_name"`
		JobURL   string           `json:"job_url"`
		Pipeline PipelineResponse `json:"pipeline"`
	} `json:"data"`
}

type SyncPipelinesResponse struct {
	Data usecase.SyncPipelinesOutput `json:"data"`
}

type CreateRawPipelineRequest struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Script      string `json:"script"`
	Sandbox     *bool  `json:"sandbox"`
}

type UpdateRawPipelineRequest struct {
	Description string `json:"description"`
	Script      string `json:"script"`
	Sandbox     *bool  `json:"sandbox"`
}

type PreviewRawPipelineConfigXMLRequest struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Script      string `json:"script"`
	Sandbox     *bool  `json:"sandbox"`
}

type CreateBindingRequest struct {
	BindingType string `json:"binding_type"`
	Provider    string `json:"provider"`
	PipelineID  string `json:"pipeline_id"`
	ExternalRef string `json:"external_ref"`
	TriggerMode string `json:"trigger_mode"`
	Status      string `json:"status"`
}

type UpdateBindingRequest struct {
	Provider    string `json:"provider"`
	PipelineID  string `json:"pipeline_id"`
	ExternalRef string `json:"external_ref"`
	TriggerMode string `json:"trigger_mode"`
	Status      string `json:"status"`
}

type PipelineBindingResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ApplicationID   string    `json:"application_id"`
	ApplicationName string    `json:"application_name"`
	BindingType     string    `json:"binding_type"`
	Provider        string    `json:"provider"`
	PipelineID      string    `json:"pipeline_id"`
	ExternalRef     string    `json:"external_ref"`
	TriggerMode     string    `json:"trigger_mode"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PipelineBindingDataResponse struct {
	Data PipelineBindingResponse `json:"data"`
}

type PipelineBindingListResponse struct {
	Data     []PipelineBindingResponse `json:"data"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int64                     `json:"total"`
}

// Sync godoc
// @Summary      Sync pipelines from Jenkins
// @Tags         pipelines
// @Produce      json
// @Success      200  {object}  SyncPipelinesResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /jenkins/pipelines/sync [post]
func (h *PipelineHandler) Sync(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	result, err := h.syncer.Execute(c.Request.Context())
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ListPipelines godoc
// @Summary      List pipelines
// @Tags         pipelines
// @Produce      json
// @Param        name      query     string  false  "Pipeline name"
// @Param        provider  query     string  false  "Provider, default jenkins"
// @Param        status    query     string  false  "Pipeline status"
// @Param        page      query     int     false  "Page number"
// @Param        page_size query     int     false  "Page size"
// @Success      200  {object}  PipelineListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines [get]
func (h *PipelineHandler) ListPipelines(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.view", "", "") {
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

	items, total, err := h.query.List(c.Request.Context(), domain.PipelineListFilter{
		Name:     c.Query("name"),
		Provider: domain.Provider(strings.TrimSpace(c.Query("provider"))),
		Status:   domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}

	resp := make([]PipelineResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toPipelineResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetPipelineByID godoc
// @Summary      Get pipeline by ID
// @Tags         pipelines
// @Produce      json
// @Param        id   path      string  true  "Pipeline ID"
// @Success      200  {object}  PipelineDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id} [get]
func (h *PipelineHandler) GetPipelineByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.view", "", "") {
		return
	}
	item, err := h.query.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPipelineResponse(item)})
}

// GetPipelineRawScript godoc
// @Summary      Get Jenkins pipeline raw script
// @Tags         pipelines
// @Produce      json
// @Param        id   path      string  true  "Pipeline ID"
// @Success      200  {object}  PipelineRawScriptDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/raw-script [get]
func (h *PipelineHandler) GetPipelineRawScript(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.view", "", "") {
		return
	}
	result, err := h.query.GetRawScript(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"pipeline":         toPipelineResponse(result.Pipeline),
			"definition_class": result.DefinitionClass,
			"description":      result.Description,
			"script":           result.Script,
			"script_path":      result.ScriptPath,
			"sandbox":          result.Sandbox,
			"from_scm":         result.FromSCM,
		},
	})
}

// GetPipelineConfigXML 获取Pipeline Config XML详情。
// @Summary      获取Pipeline Config XML详情
// @Description  获取Pipeline Config XML详情，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/config-xml [get]
func (h *PipelineHandler) GetPipelineConfigXML(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.view", "", "") {
		return
	}
	result, err := h.query.GetConfigXML(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"pipeline":   toPipelineResponse(result.Pipeline),
			"config_xml": result.ConfigXML,
		},
	})
}

// GetPipelineOriginalLink 获取Pipeline Original Link详情。
// @Summary      获取Pipeline Original Link详情
// @Description  获取Pipeline Original Link详情，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/original-link [get]
func (h *PipelineHandler) GetPipelineOriginalLink(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.view", "", "") {
		return
	}
	result, err := h.query.GetOriginalLink(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"pipeline":      toPipelineResponse(result.Pipeline),
			"original_link": result.OriginalLink,
		},
	})
}

// CreateRawPipeline 创建Raw Pipeline。
// @Summary      创建Raw Pipeline
// @Description  创建Raw Pipeline，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /jenkins/pipelines/raw [post]
func (h *PipelineHandler) CreateRawPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pipeline manager is not configured"})
		return
	}

	var req CreateRawPipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	sandbox := true
	if req.Sandbox != nil {
		sandbox = *req.Sandbox
	}
	item, err := h.manager.CreateRaw(c.Request.Context(), usecase.CreateJenkinsRawPipelineInput{
		FullName:    req.FullName,
		Description: req.Description,
		Script:      req.Script,
		Sandbox:     sandbox,
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toPipelineResponse(item)})
}

// UpdateRawPipeline 更新Raw Pipeline。
// @Summary      更新Raw Pipeline
// @Description  更新Raw Pipeline，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/raw [put]
func (h *PipelineHandler) UpdateRawPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pipeline manager is not configured"})
		return
	}

	var req UpdateRawPipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	sandbox := true
	if req.Sandbox != nil {
		sandbox = *req.Sandbox
	}
	item, err := h.manager.UpdateRaw(c.Request.Context(), c.Param("id"), usecase.UpdateJenkinsRawPipelineInput{
		Description: req.Description,
		Script:      req.Script,
		Sandbox:     sandbox,
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPipelineResponse(item)})
}

// DeleteRawPipeline 删除Raw Pipeline。
// @Summary      删除Raw Pipeline
// @Description  删除Raw Pipeline，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/raw [delete]
func (h *PipelineHandler) DeleteRawPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pipeline manager is not configured"})
		return
	}

	item, err := h.manager.DeleteRaw(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPipelineResponse(item)})
}

// PreviewRawPipelineConfigXML 处理Preview Raw Pipeline Config XML接口。
// @Summary      处理Preview Raw Pipeline Config XML接口
// @Description  处理Preview Raw Pipeline Config XML接口，并按统一响应结构返回处理结果。
// @Tags         pipelines
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /jenkins/pipelines/raw/preview-config-xml [post]
func (h *PipelineHandler) PreviewRawPipelineConfigXML(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pipeline manager is not configured"})
		return
	}

	var req PreviewRawPipelineConfigXMLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	sandbox := true
	if req.Sandbox != nil {
		sandbox = *req.Sandbox
	}
	configXML, err := h.manager.PreviewRawConfigXML(c.Request.Context(), usecase.PreviewJenkinsRawPipelineConfigInput{
		FullName:    req.FullName,
		Description: req.Description,
		Script:      req.Script,
		Sandbox:     sandbox,
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"config_xml": configXML}})
}

// VerifyPipeline godoc
// @Summary      Verify Jenkins pipeline availability
// @Tags         pipelines
// @Produce      json
// @Param        id   path      string  true  "Pipeline ID"
// @Success      200  {object}  VerifyPipelineResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/verify [post]
func (h *PipelineHandler) VerifyPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	result, err := h.query.Verify(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"verified": result.Verified,
			"job_name": result.JobName,
			"job_url":  result.JobURL,
			"pipeline": toPipelineResponse(result.Pipeline),
		},
	})
}

// CreateBinding godoc
// @Summary      Create pipeline binding for application
// @Tags         pipeline-bindings
// @Accept       json
// @Produce      json
// @Param        id       path      string                true  "Application ID"
// @Param        request  body      CreateBindingRequest  true  "Create binding request"
// @Success      201  {object}  PipelineBindingDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/pipeline-bindings [post]
func (h *PipelineHandler) CreateBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.binding.Create(c.Request.Context(), c.Param("id"), usecase.CreatePipelineBindingInput{
		BindingType: domain.BindingType(strings.TrimSpace(req.BindingType)),
		Provider:    domain.Provider(strings.TrimSpace(req.Provider)),
		PipelineID:  req.PipelineID,
		ExternalRef: req.ExternalRef,
		TriggerMode: domain.TriggerMode(strings.TrimSpace(req.TriggerMode)),
		Status:      domain.Status(strings.TrimSpace(req.Status)),
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toBindingResponse(item)})
}

// ListBindings godoc
// @Summary      List pipeline bindings for application
// @Tags         pipeline-bindings
// @Produce      json
// @Param        id        path      string  true   "Application ID"
// @Param        binding_type query  string  false  "Binding type: ci/cd"
// @Param        provider   query     string  false  "Provider: jenkins/argocd"
// @Param        status     query     string  false  "Binding status"
// @Param        page      query     int     false  "Page number"
// @Param        page_size query     int     false  "Page size"
// @Success      200  {object}  PipelineBindingListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/pipeline-bindings [get]
func (h *PipelineHandler) ListBindings(c *gin.Context) {
	if !ensurePipelineBindingListPermission(c, h.authz, c.Param("id")) {
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

	items, total, err := h.binding.ListByApplication(c.Request.Context(), domain.BindingListFilter{
		ApplicationID: c.Param("id"),
		BindingType:   domain.BindingType(strings.TrimSpace(c.Query("binding_type"))),
		Provider:      domain.Provider(strings.TrimSpace(c.Query("provider"))),
		Status:        domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}

	resp := make([]PipelineBindingResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toBindingResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// ensurePipelineBindingListPermission 校验前置条件，不满足时写入对应错误响应。
func ensurePipelineBindingListPermission(c *gin.Context, authz RequestAuthorizer, applicationID string) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	allowed, err := authz.HasPermission(c.Request.Context(), user, "pipeline.view", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if allowed {
		return true
	}
	return ensureAnyApplicationPermission(
		c,
		authz,
		applicationID,
		"application.view",
		"release.view",
		"release.create",
		"release.execute",
		"release.cancel",
	)
}

// GetBindingByID godoc
// @Summary      Get pipeline binding by ID
// @Tags         pipeline-bindings
// @Produce      json
// @Param        id   path      string  true  "Binding ID"
// @Success      200  {object}  PipelineBindingDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipeline-bindings/{id} [get]
func (h *PipelineHandler) GetBindingByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.view", "", "") {
		return
	}
	item, err := h.binding.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toBindingResponse(item)})
}

// UpdateBinding godoc
// @Summary      Update pipeline binding
// @Tags         pipeline-bindings
// @Accept       json
// @Produce      json
// @Param        id       path      string                true  "Binding ID"
// @Param        request  body      UpdateBindingRequest  true  "Update binding request"
// @Success      200  {object}  PipelineBindingDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipeline-bindings/{id} [put]
func (h *PipelineHandler) UpdateBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	var req UpdateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.binding.Update(c.Request.Context(), c.Param("id"), domain.BindingUpdateInput{
		Provider:    domain.Provider(strings.TrimSpace(req.Provider)),
		PipelineID:  req.PipelineID,
		ExternalRef: req.ExternalRef,
		TriggerMode: domain.TriggerMode(strings.TrimSpace(req.TriggerMode)),
		Status:      domain.Status(strings.TrimSpace(req.Status)),
	})
	if err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toBindingResponse(item)})
}

// DeleteBinding godoc
// @Summary      Delete pipeline binding
// @Tags         pipeline-bindings
// @Produce      json
// @Param        id   path      string  true  "Binding ID"
// @Success      204  {string}  string  "No Content"
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipeline-bindings/{id} [delete]
func (h *PipelineHandler) DeleteBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if err := h.binding.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writePipelineHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// toPipelineResponse 将领域对象转换为接口响应结构。
func toPipelineResponse(item domain.Pipeline) PipelineResponse {
	return PipelineResponse{
		ID:             item.ID,
		Provider:       string(item.Provider),
		JobFullName:    item.JobFullName,
		JobName:        item.JobName,
		JobURL:         item.JobURL,
		Description:    item.Description,
		CredentialRef:  item.CredentialRef,
		DefaultBranch:  item.DefaultBranch,
		Status:         string(item.Status),
		LastVerifiedAt: item.LastVerifiedAt,
		LastSyncedAt:   item.LastSyncedAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

// toBindingResponse 将领域对象转换为接口响应结构。
func toBindingResponse(item domain.PipelineBinding) PipelineBindingResponse {
	return PipelineBindingResponse{
		ID:              item.ID,
		Name:            item.Name,
		ApplicationID:   item.ApplicationID,
		ApplicationName: item.ApplicationName,
		BindingType:     string(item.BindingType),
		Provider:        string(item.Provider),
		PipelineID:      item.PipelineID,
		ExternalRef:     item.ExternalRef,
		TriggerMode:     string(item.TriggerMode),
		Status:          string(item.Status),
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}

// writePipelineHTTPError 写入处理结果或错误信息。
func writePipelineHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus),
		errors.Is(err, usecase.ErrInvalidProvider),
		errors.Is(err, usecase.ErrInvalidBindingType),
		errors.Is(err, usecase.ErrInvalidTriggerMode):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, appdomain.ErrNotFound),
		errors.Is(err, domain.ErrPipelineNotFound),
		errors.Is(err, domain.ErrBindingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrBindingDuplicated), errors.Is(err, domain.ErrPipelineDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// parsePositiveInt 解析输入内容并返回结构化结果。
func parsePositiveInt(c *gin.Context, name string) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	if value < 1 {
		return 0, errors.New(name + " must be greater than 0")
	}
	return value, nil
}

// resolvedPage 解析上下文数据，得到后续流程需要的结果。
func resolvedPage(page int) int {
	if page > 0 {
		return page
	}
	return 1
}

// resolvedPageSize 解析上下文数据，得到后续流程需要的结果。
func resolvedPageSize(pageSize int) int {
	const (
		defaultPageSize = 20
		maxPageSize     = 100
	)
	if pageSize < 1 {
		return defaultPageSize
	}
	if pageSize > maxPageSize {
		return maxPageSize
	}
	return pageSize
}
