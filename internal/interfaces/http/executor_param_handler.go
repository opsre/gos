package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	platformparamdomain "gos/internal/domain/platformparam"
	userdomain "gos/internal/domain/user"
)

type ExecutorParamHandler struct {
	manager *usecase.ExecutorParamDefManager
	syncer  *usecase.SyncExecutorParamDefs
	authz   RequestAuthorizer
	access  ExecutorParamAccessResolver
}

type ExecutorParamAccessResolver interface {
	ResolveParamAccess(
		ctx context.Context,
		user userdomain.User,
		applicationID string,
		paramKey string,
	) (canView bool, canEdit bool, err error)
}

// NewExecutorParamHandler 创建并返回对应组件实例。
func NewExecutorParamHandler(
	manager *usecase.ExecutorParamDefManager,
	syncer *usecase.SyncExecutorParamDefs,
	authz RequestAuthorizer,
	access ExecutorParamAccessResolver,
) *ExecutorParamHandler {
	return &ExecutorParamHandler{
		manager: manager,
		syncer:  syncer,
		authz:   authz,
		access:  access,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ExecutorParamHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/applications/:id/executor-param-defs", h.ListByApplication)
	router.GET("/pipelines/:id/param-defs", h.ListByPipeline)
	router.GET("/executor-param-defs", h.List)
	router.GET("/executor-param-defs/:id", h.GetByID)
	router.PUT("/executor-param-defs/:id", h.Update)
	router.POST("/jenkins/executor-param-defs/sync", h.Sync)
}

type UpdateExecutorParamDefRequest struct {
	ParamKey string `json:"param_key"`
}

type ExecutorParamDefResponse struct {
	ID                string    `json:"id"`
	ApplicationID     string    `json:"application_id"`
	ApplicationName   string    `json:"application_name"`
	ApplicationKey    string    `json:"application_key"`
	BindingType       string    `json:"binding_type"`
	PipelineName      string    `json:"pipeline_name"`
	PipelineID        string    `json:"pipeline_id"`
	ExecutorType      string    `json:"executor_type"`
	ExecutorParamName string    `json:"executor_param_name"`
	ParamKey          string    `json:"param_key"`
	ParamType         string    `json:"param_type"`
	SingleSelect      bool      `json:"single_select"`
	Required          bool      `json:"required"`
	DefaultValue      string    `json:"default_value"`
	Description       string    `json:"description"`
	Visible           bool      `json:"visible"`
	Editable          bool      `json:"editable"`
	SourceFrom        string    `json:"source_from"`
	Status            string    `json:"status"`
	RawMeta           string    `json:"raw_meta"`
	SortNo            int       `json:"sort_no"`
	CanView           bool      `json:"can_view"`
	CanEdit           bool      `json:"can_edit"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ExecutorParamDefDataResponse struct {
	Data ExecutorParamDefResponse `json:"data"`
}

type ExecutorParamDefListResponse struct {
	Data     []ExecutorParamDefResponse `json:"data"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Total    int64                      `json:"total"`
}

type SyncExecutorParamDefsResponse struct {
	Data usecase.SyncExecutorParamDefsOutput `json:"data"`
}

// List godoc
// @Summary      List executor param definitions across applications
// @Tags         executor-param-defs
// @Produce      json
// @Param        application_id  query     string  false  "Application ID"
// @Param        keyword         query     string  false  "Application name, application key, or platform key"
// @Param        binding_type    query     string  false  "Binding type, default ci"
// @Param        visible         query     bool    false  "Visible flag"
// @Param        editable        query     bool    false  "Editable flag"
// @Param        status          query     string  false  "Status"
// @Param        page            query     int     false  "Page number"
// @Param        page_size       query     int     false  "Page size"
// @Success      200  {object}  ExecutorParamDefListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /executor-param-defs [get]
func (h *ExecutorParamHandler) List(c *gin.Context) {
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
	visible, err := parseOptionalBool(c, "visible")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	editable, err := parseOptionalBool(c, "editable")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allowAll, visibleApplicationIDs, ok := resolveVisibleApplicationIDsForApplications(c, h.authz)
	if !ok {
		return
	}
	if !allowAll && len(visibleApplicationIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data":      []ExecutorParamDefResponse{},
			"page":      resolvedPage(page),
			"page_size": resolvedPageSize(pageSize),
			"total":     0,
		})
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	manageAll := currentUser.Role == userdomain.RoleAdmin
	if !manageAll {
		allowed, authErr := h.authz.HasPermission(c.Request.Context(), currentUser, "pipeline_param.manage", "", "")
		if authErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		manageAll = allowed
	}

	items, total, err := h.manager.ListByApplications(c.Request.Context(), domain.ApplicationListFilter{
		ApplicationIDs: resolveApplicationFilterIDs(strings.TrimSpace(c.Query("application_id")), allowAll, visibleApplicationIDs),
		Keyword:        c.Query("keyword"),
		BindingType:    pipelinedomain.BindingType(strings.TrimSpace(c.Query("binding_type"))),
		Visible:        visible,
		Editable:       editable,
		Status:         domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}

	resp := make([]ExecutorParamDefResponse, 0, len(items))
	if manageAll {
		for _, item := range items {
			entry := toExecutorParamResponse(item)
			entry.CanView = true
			entry.CanEdit = true
			resp = append(resp, entry)
		}
		c.JSON(http.StatusOK, gin.H{
			"data":      resp,
			"page":      resolvedPage(page),
			"page_size": resolvedPageSize(pageSize),
			"total":     total,
		})
		return
	}

	releaseCreateCache := make(map[string]bool)
	type accessGrant struct {
		canView bool
		canEdit bool
	}
	accessCache := make(map[string]accessGrant)
	for _, item := range items {
		entry := toExecutorParamResponse(item)
		applicationID := strings.TrimSpace(item.ApplicationID)
		if applicationID == "" {
			continue
		}

		hasReleaseCreate, accessErr := h.resolveReleaseCreatePermission(
			c.Request.Context(),
			currentUser,
			applicationID,
			releaseCreateCache,
		)
		if accessErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if hasReleaseCreate && strings.TrimSpace(item.ParamKey) != "" {
			entry.CanView = true
			entry.CanEdit = true
			resp = append(resp, entry)
			continue
		}
		if h.access == nil {
			continue
		}

		cacheKey := applicationID + "\x00" + strings.TrimSpace(item.ParamKey)
		grant, exists := accessCache[cacheKey]
		if !exists {
			canView, canEdit, accessErr := h.access.ResolveParamAccess(
				c.Request.Context(),
				currentUser,
				applicationID,
				item.ParamKey,
			)
			if accessErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}
			grant = accessGrant{
				canView: canView,
				canEdit: canEdit,
			}
			accessCache[cacheKey] = grant
		}
		if !grant.canView {
			continue
		}
		entry.CanView = grant.canView
		entry.CanEdit = grant.canEdit
		resp = append(resp, entry)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// ListByApplication godoc
// @Summary      List bound executor param definitions by application
// @Tags         executor-param-defs
// @Produce      json
// @Param        id            path      string  true   "Application ID"
// @Param        binding_type  query     string  false  "Binding type, default ci"
// @Param        visible       query     bool    false  "Visible flag"
// @Param        editable      query     bool    false  "Editable flag"
// @Param        param_key     query     string  false  "Mapped platform param key"
// @Param        page          query     int     false  "Page number"
// @Param        page_size     query     int     false  "Page size"
// @Success      200  {object}  ExecutorParamDefListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/executor-param-defs [get]
func (h *ExecutorParamHandler) ListByApplication(c *gin.Context) {
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
	visible, err := parseOptionalBool(c, "visible")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	editable, err := parseOptionalBool(c, "editable")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, total, err := h.manager.ListByApplication(
		c.Request.Context(),
		c.Param("id"),
		pipelinedomain.BindingType(strings.TrimSpace(c.Query("binding_type"))),
		c.Query("binding_id"),
		domain.ListFilter{
			Visible:  visible,
			Editable: editable,
			ParamKey: c.Query("param_key"),
			Status:   domain.Status(strings.TrimSpace(c.Query("status"))),
			Page:     page,
			PageSize: pageSize,
		},
	)
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	manageAll := false
	hasReleaseCreate := false
	if currentUser.Role != userdomain.RoleAdmin {
		if h.authz == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
			return
		}
		allowed, authErr := h.authz.HasPermission(c.Request.Context(), currentUser, "pipeline_param.manage", "", "")
		if authErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		manageAll = allowed

		createAllowed, createErr := h.authz.HasPermission(
			c.Request.Context(),
			currentUser,
			"release.create",
			"application",
			c.Param("id"),
		)
		if createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		hasReleaseCreate = createAllowed
		if !manageAll && !hasReleaseCreate {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
			return
		}
	}

	resp := make([]ExecutorParamDefResponse, 0, len(items))
	for _, item := range items {
		entry := toExecutorParamResponse(item)
		if currentUser.Role == userdomain.RoleAdmin || manageAll {
			entry.CanView = true
			entry.CanEdit = true
			resp = append(resp, entry)
			continue
		}
		if h.access == nil {
			if hasReleaseCreate && strings.TrimSpace(item.ParamKey) != "" {
				entry.CanView = true
				entry.CanEdit = true
				resp = append(resp, entry)
			}
			continue
		}
		canView, canEdit, accessErr := h.access.ResolveParamAccess(
			c.Request.Context(),
			currentUser,
			c.Param("id"),
			item.ParamKey,
		)
		if accessErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if hasReleaseCreate && strings.TrimSpace(item.ParamKey) != "" {
			entry.CanView = true
			entry.CanEdit = true
			resp = append(resp, entry)
			continue
		}
		if !canView {
			continue
		}
		entry.CanView = canView
		entry.CanEdit = canEdit
		resp = append(resp, entry)
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// ListByPipeline godoc
// @Summary      List executor param definitions
// @Tags         executor-param-defs
// @Produce      json
// @Param        id             path      string  true   "Pipeline ID"
// @Param        executor_type  query     string  false  "Executor type"
// @Param        visible        query     bool    false  "Visible flag"
// @Param        editable       query     bool    false  "Editable flag"
// @Param        param_key      query     string  false  "Mapped platform param key"
// @Param        page           query     int     false  "Page number"
// @Param        page_size      query     int     false  "Page size"
// @Success      200  {object}  ExecutorParamDefListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /pipelines/{id}/param-defs [get]
func (h *ExecutorParamHandler) ListByPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline_param.manage", "", "") {
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
	visible, err := parseOptionalBool(c, "visible")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	editable, err := parseOptionalBool(c, "editable")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, total, err := h.manager.ListByPipeline(c.Request.Context(), domain.ListFilter{
		PipelineID:   c.Param("id"),
		ExecutorType: domain.ExecutorType(strings.TrimSpace(c.Query("executor_type"))),
		Visible:      visible,
		Editable:     editable,
		ParamKey:     c.Query("param_key"),
		Status:       domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}

	resp := make([]ExecutorParamDefResponse, 0, len(items))
	for _, item := range items {
		entry := toExecutorParamResponse(item)
		entry.CanView = entry.Visible
		entry.CanEdit = entry.Editable
		resp = append(resp, entry)
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetByID godoc
// @Summary      Get executor param definition by ID
// @Tags         executor-param-defs
// @Produce      json
// @Param        id   path      string  true  "Executor param definition ID"
// @Success      200  {object}  ExecutorParamDefDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /executor-param-defs/{id} [get]
func (h *ExecutorParamHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline_param.manage", "", "") {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}
	resp := toExecutorParamResponse(item)
	resp.CanView = resp.Visible
	resp.CanEdit = resp.Editable
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Update godoc
// @Summary      Update executor param definition mapping
// @Tags         executor-param-defs
// @Accept       json
// @Produce      json
// @Param        id       path      string                         true  "Executor param definition ID"
// @Param        request  body      UpdateExecutorParamDefRequest  true  "Update executor param definition request"
// @Success      200      {object}  ExecutorParamDefDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /executor-param-defs/{id} [put]
func (h *ExecutorParamHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline_param.manage", "", "") {
		return
	}
	var req UpdateExecutorParamDefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.UpdateParamKey(c.Request.Context(), c.Param("id"), req.ParamKey)
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}
	resp := toExecutorParamResponse(item)
	resp.CanView = resp.Visible
	resp.CanEdit = resp.Editable
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Sync godoc
// @Summary      Sync executor param definitions from Jenkins
// @Tags         executor-param-defs
// @Produce      json
// @Success      200  {object}  SyncExecutorParamDefsResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /jenkins/executor-param-defs/sync [post]
func (h *ExecutorParamHandler) Sync(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "pipeline_param.manage", "pipeline.manage") {
		return
	}
	result, err := h.syncer.Execute(c.Request.Context())
	if err != nil {
		writeExecutorParamHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// toExecutorParamResponse 将领域对象转换为接口响应结构。
func toExecutorParamResponse(item domain.ExecutorParamDef) ExecutorParamDefResponse {
	return ExecutorParamDefResponse{
		ID:                item.ID,
		ApplicationID:     item.ApplicationID,
		ApplicationName:   item.ApplicationName,
		ApplicationKey:    item.ApplicationKey,
		BindingType:       item.BindingType,
		PipelineName:      item.PipelineName,
		PipelineID:        item.PipelineID,
		ExecutorType:      string(item.ExecutorType),
		ExecutorParamName: item.ExecutorParamName,
		ParamKey:          item.ParamKey,
		ParamType:         string(item.ParamType),
		SingleSelect:      item.SingleSelect,
		Required:          item.Required,
		DefaultValue:      item.DefaultValue,
		Description:       item.Description,
		Visible:           item.Visible,
		Editable:          item.Editable,
		SourceFrom:        string(item.SourceFrom),
		Status:            string(item.Status),
		RawMeta:           item.RawMeta,
		SortNo:            item.SortNo,
		CanView:           item.Visible,
		CanEdit:           item.Editable,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

// resolveReleaseCreatePermission 解析上下文数据，得到后续流程需要的结果。
func (h *ExecutorParamHandler) resolveReleaseCreatePermission(
	ctx context.Context,
	user userdomain.User,
	applicationID string,
	cache map[string]bool,
) (bool, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return false, nil
	}
	if allowed, exists := cache[applicationID]; exists {
		return allowed, nil
	}
	allowed, err := h.authz.HasPermission(ctx, user, "release.create", "application", applicationID)
	if err != nil {
		return false, err
	}
	cache[applicationID] = allowed
	return allowed, nil
}

// writeExecutorParamHTTPError 写入处理结果或错误信息。
func writeExecutorParamHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus),
		errors.Is(err, usecase.ErrInvalidParamKey),
		errors.Is(err, usecase.ErrInvalidExecutorType),
		errors.Is(err, usecase.ErrInvalidBindingType):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, appdomain.ErrNotFound),
		errors.Is(err, pipelinedomain.ErrPipelineNotFound),
		errors.Is(err, pipelinedomain.ErrBindingNotFound),
		errors.Is(err, domain.ErrNotFound),
		errors.Is(err, platformparamdomain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// parseOptionalBool 解析输入内容并返回结构化结果。
func parseOptionalBool(c *gin.Context, name string) (*bool, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errors.New(name + " must be a boolean")
	}
	return &value, nil
}
