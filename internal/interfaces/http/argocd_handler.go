package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/argocdapp"
)

type ArgoCDHandler struct {
	syncer    *usecase.SyncArgoCDApplications
	query     *usecase.QueryArgoCDApplications
	instances *usecase.ArgoCDInstanceManager
	authz     RequestAuthorizer
}

// NewArgoCDHandler 创建并返回对应组件实例。
func NewArgoCDHandler(
	syncer *usecase.SyncArgoCDApplications,
	query *usecase.QueryArgoCDApplications,
	instances *usecase.ArgoCDInstanceManager,
	authz RequestAuthorizer,
) *ArgoCDHandler {
	return &ArgoCDHandler{syncer: syncer, query: query, instances: instances, authz: authz}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ArgoCDHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/argocd/applications", h.ListApplications)
	router.GET("/argocd/applications/:id", h.GetApplicationByID)
	router.POST("/argocd/applications/sync", h.SyncApplications)
	router.GET("/argocd/applications/:id/original-link", h.GetOriginalLink)

	router.GET("/argocd/instances", h.ListInstances)
	router.POST("/argocd/instances", h.CreateInstance)
	router.PUT("/argocd/instances/:id", h.UpdateInstance)
	router.POST("/argocd/instances/:id/check", h.CheckInstance)
	router.GET("/argocd/env-bindings", h.ListEnvBindings)
	router.PUT("/argocd/env-bindings", h.UpdateEnvBindings)
}

type ArgoCDApplicationResponse struct {
	ID               string    `json:"id"`
	ArgoCDInstanceID string    `json:"argocd_instance_id"`
	InstanceCode     string    `json:"instance_code"`
	InstanceName     string    `json:"instance_name"`
	ClusterName      string    `json:"cluster_name"`
	InstanceBaseURL  string    `json:"instance_base_url"`
	AppName          string    `json:"app_name"`
	Project          string    `json:"project"`
	RepoURL          string    `json:"repo_url"`
	SourcePath       string    `json:"source_path"`
	TargetRevision   string    `json:"target_revision"`
	DestServer       string    `json:"dest_server"`
	DestNamespace    string    `json:"dest_namespace"`
	SyncStatus       string    `json:"sync_status"`
	HealthStatus     string    `json:"health_status"`
	OperationPhase   string    `json:"operation_phase"`
	ArgoCDURL        string    `json:"argocd_url"`
	Status           string    `json:"status"`
	RawMeta          string    `json:"raw_meta"`
	LastSyncedAt     time.Time `json:"last_synced_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ArgoCDApplicationDataResponse struct {
	Data ArgoCDApplicationResponse `json:"data"`
}

type ArgoCDApplicationListResponse struct {
	Data     []ArgoCDApplicationResponse `json:"data"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int64                       `json:"total"`
}

type ArgoCDApplicationSyncResponse struct {
	Data usecase.SyncArgoCDApplicationsOutput `json:"data"`
}

type ArgoCDOriginalLinkDataResponse struct {
	Data struct {
		Application  ArgoCDApplicationResponse `json:"application"`
		OriginalLink string                    `json:"original_link"`
	} `json:"data"`
}

type ArgoCDInstanceResponse struct {
	ID                 string    `json:"id"`
	InstanceCode       string    `json:"instance_code"`
	Name               string    `json:"name"`
	BaseURL            string    `json:"base_url"`
	InsecureSkipVerify bool      `json:"insecure_skip_verify"`
	AuthMode           string    `json:"auth_mode"`
	Username           string    `json:"username"`
	GitOpsInstanceID   string    `json:"gitops_instance_id"`
	GitOpsInstanceCode string    `json:"gitops_instance_code"`
	GitOpsInstanceName string    `json:"gitops_instance_name"`
	ClusterName        string    `json:"cluster_name"`
	DefaultNamespace   string    `json:"default_namespace"`
	Status             string    `json:"status"`
	HealthStatus       string    `json:"health_status"`
	LastCheckAt        time.Time `json:"last_check_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Remark             string    `json:"remark"`
}

type ArgoCDInstanceListResponse struct {
	Data     []ArgoCDInstanceResponse `json:"data"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	Total    int64                    `json:"total"`
}

type ArgoCDInstanceDataResponse struct {
	Data ArgoCDInstanceResponse `json:"data"`
}

type ArgoCDEnvBindingResponse struct {
	ID                 string    `json:"id"`
	EnvCode            string    `json:"env_code"`
	ArgoCDInstanceID   string    `json:"argocd_instance_id"`
	ArgoCDInstanceCode string    `json:"argocd_instance_code"`
	ArgoCDInstanceName string    `json:"argocd_instance_name"`
	ClusterName        string    `json:"cluster_name"`
	Priority           int       `json:"priority"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ArgoCDEnvBindingListResponse struct {
	Data []ArgoCDEnvBindingResponse `json:"data"`
}

type upsertArgoCDInstanceRequest struct {
	InstanceCode       string `json:"instance_code"`
	Name               string `json:"name"`
	BaseURL            string `json:"base_url"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	AuthMode           string `json:"auth_mode"`
	Token              string `json:"token"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	GitOpsInstanceID   string `json:"gitops_instance_id"`
	ClusterName        string `json:"cluster_name"`
	DefaultNamespace   string `json:"default_namespace"`
	Status             string `json:"status"`
	Remark             string `json:"remark"`
}

type updateArgoCDEnvBindingsRequest struct {
	Bindings []struct {
		EnvCode          string `json:"env_code"`
		ArgoCDInstanceID string `json:"argocd_instance_id"`
		Status           string `json:"status"`
	} `json:"bindings"`
}

// ListApplications 查询Applications列表。
// @Summary      查询Applications列表
// @Description  查询Applications列表，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/applications [get]
func (h *ArgoCDHandler) ListApplications(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.view", "component.argocd.manage", "component.argocd.instance.view", "component.argocd.instance.manage") {
		return
	}
	if h.query == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd manager is not configured"})
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
	items, total, err := h.query.List(c.Request.Context(), domain.ListFilter{
		ArgoCDInstanceID: strings.TrimSpace(c.Query("argocd_instance_id")),
		AppName:          c.Query("app_name"),
		Project:          c.Query("project"),
		SyncStatus:       c.Query("sync_status"),
		HealthStatus:     c.Query("health_status"),
		Status:           domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:             page,
		PageSize:         pageSize,
	})
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	resp := make([]ArgoCDApplicationResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toArgoCDApplicationResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetApplicationByID 获取Application By ID详情。
// @Summary      获取Application By ID详情
// @Description  获取Application By ID详情，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/applications/{id} [get]
func (h *ArgoCDHandler) GetApplicationByID(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.view", "component.argocd.manage", "component.argocd.instance.view", "component.argocd.instance.manage") {
		return
	}
	if h.query == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd manager is not configured"})
		return
	}
	item, err := h.query.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArgoCDApplicationResponse(item)})
}

// SyncApplications 同步Applications。
// @Summary      同步Applications
// @Description  同步Applications，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/applications/sync [post]
func (h *ArgoCDHandler) SyncApplications(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.manage", "component.argocd.instance.manage") {
		return
	}
	if h.syncer == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd manager is not configured"})
		return
	}
	result, err := h.syncer.Execute(c.Request.Context())
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetOriginalLink 获取Original Link详情。
// @Summary      获取Original Link详情
// @Description  获取Original Link详情，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/applications/{id}/original-link [get]
func (h *ArgoCDHandler) GetOriginalLink(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.view", "component.argocd.manage", "component.argocd.instance.view", "component.argocd.instance.manage") {
		return
	}
	if h.query == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd manager is not configured"})
		return
	}
	item, err := h.query.GetOriginalLink(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	resp := ArgoCDOriginalLinkDataResponse{}
	resp.Data.Application = toArgoCDApplicationResponse(item.Application)
	resp.Data.OriginalLink = item.OriginalLink
	c.JSON(http.StatusOK, resp)
}

// ListInstances 查询Instances列表。
// @Summary      查询Instances列表
// @Description  查询Instances列表，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/instances [get]
func (h *ArgoCDHandler) ListInstances(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.instance.view", "component.argocd.instance.manage", "component.argocd.binding.view", "component.argocd.binding.manage", "component.argocd.view", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
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
	items, total, err := h.instances.List(c.Request.Context(), domain.InstanceListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Status:   domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	resp := make([]ArgoCDInstanceResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toArgoCDInstanceResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// CreateInstance 创建Instance。
// @Summary      创建Instance
// @Description  创建Instance，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/instances [post]
func (h *ArgoCDHandler) CreateInstance(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.instance.manage", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
		return
	}
	var req upsertArgoCDInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.instances.Create(c.Request.Context(), usecase.CreateArgoCDInstanceInput{
		InstanceCode:       req.InstanceCode,
		Name:               req.Name,
		BaseURL:            req.BaseURL,
		InsecureSkipVerify: req.InsecureSkipVerify,
		AuthMode:           req.AuthMode,
		Token:              req.Token,
		Username:           req.Username,
		Password:           req.Password,
		GitOpsInstanceID:   req.GitOpsInstanceID,
		ClusterName:        req.ClusterName,
		DefaultNamespace:   req.DefaultNamespace,
		Status:             domain.Status(strings.TrimSpace(req.Status)),
		Remark:             req.Remark,
	})
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArgoCDInstanceResponse(item)})
}

// UpdateInstance 更新Instance。
// @Summary      更新Instance
// @Description  更新Instance，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/instances/{id} [put]
func (h *ArgoCDHandler) UpdateInstance(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.instance.manage", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
		return
	}
	var req upsertArgoCDInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.instances.Update(c.Request.Context(), c.Param("id"), usecase.UpdateArgoCDInstanceInput{
		InstanceCode:       req.InstanceCode,
		Name:               req.Name,
		BaseURL:            req.BaseURL,
		InsecureSkipVerify: req.InsecureSkipVerify,
		AuthMode:           req.AuthMode,
		Token:              req.Token,
		Username:           req.Username,
		Password:           req.Password,
		GitOpsInstanceID:   req.GitOpsInstanceID,
		ClusterName:        req.ClusterName,
		DefaultNamespace:   req.DefaultNamespace,
		Status:             domain.Status(strings.TrimSpace(req.Status)),
		Remark:             req.Remark,
	})
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArgoCDInstanceResponse(item)})
}

// CheckInstance 检查Instance。
// @Summary      检查Instance
// @Description  检查Instance，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/instances/{id}/check [post]
func (h *ArgoCDHandler) CheckInstance(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.instance.manage", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
		return
	}
	item, err := h.instances.Check(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArgoCDInstanceResponse(item)})
}

// ListEnvBindings 查询Env Bindings列表。
// @Summary      查询Env Bindings列表
// @Description  查询Env Bindings列表，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/env-bindings [get]
func (h *ArgoCDHandler) ListEnvBindings(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.binding.view", "component.argocd.binding.manage", "component.argocd.instance.view", "component.argocd.instance.manage", "component.argocd.view", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
		return
	}
	items, err := h.instances.ListEnvBindings(c.Request.Context())
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	resp := make([]ArgoCDEnvBindingResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toArgoCDEnvBindingResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// UpdateEnvBindings 更新Env Bindings。
// @Summary      更新Env Bindings
// @Description  更新Env Bindings，并按统一响应结构返回处理结果。
// @Tags         argocd
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /argocd/env-bindings [put]
func (h *ArgoCDHandler) UpdateEnvBindings(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.argocd.binding.manage", "component.argocd.instance.manage", "component.argocd.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "argocd instance manager is not configured"})
		return
	}
	var req updateArgoCDEnvBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	items := make([]usecase.UpdateArgoCDEnvBindingItem, 0, len(req.Bindings))
	for _, item := range req.Bindings {
		items = append(items, usecase.UpdateArgoCDEnvBindingItem{
			EnvCode:          item.EnvCode,
			ArgoCDInstanceID: item.ArgoCDInstanceID,
			Status:           domain.Status(strings.TrimSpace(item.Status)),
		})
	}
	result, err := h.instances.UpdateEnvBindings(c.Request.Context(), items)
	if err != nil {
		writeArgoCDHTTPError(c, err)
		return
	}
	resp := make([]ArgoCDEnvBindingResponse, 0, len(result))
	for _, item := range result {
		resp = append(resp, toArgoCDEnvBindingResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// toArgoCDApplicationResponse 将领域对象转换为接口响应结构。
func toArgoCDApplicationResponse(item domain.Application) ArgoCDApplicationResponse {
	return ArgoCDApplicationResponse{
		ID:               item.ID,
		ArgoCDInstanceID: item.ArgoCDInstanceID,
		InstanceCode:     item.InstanceCode,
		InstanceName:     item.InstanceName,
		ClusterName:      item.ClusterName,
		InstanceBaseURL:  item.InstanceBaseURL,
		AppName:          item.AppName,
		Project:          item.Project,
		RepoURL:          item.RepoURL,
		SourcePath:       item.SourcePath,
		TargetRevision:   item.TargetRevision,
		DestServer:       item.DestServer,
		DestNamespace:    item.DestNamespace,
		SyncStatus:       item.SyncStatus,
		HealthStatus:     item.HealthStatus,
		OperationPhase:   item.OperationPhase,
		ArgoCDURL:        item.ArgoCDURL,
		Status:           string(item.Status),
		RawMeta:          item.RawMeta,
		LastSyncedAt:     item.LastSyncedAt,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

// toArgoCDInstanceResponse 将领域对象转换为接口响应结构。
func toArgoCDInstanceResponse(item domain.Instance) ArgoCDInstanceResponse {
	return ArgoCDInstanceResponse{
		ID:                 item.ID,
		InstanceCode:       item.InstanceCode,
		Name:               item.Name,
		BaseURL:            item.BaseURL,
		InsecureSkipVerify: item.InsecureSkipVerify,
		AuthMode:           item.AuthMode,
		Username:           item.Username,
		GitOpsInstanceID:   item.GitOpsInstanceID,
		GitOpsInstanceCode: item.GitOpsInstanceCode,
		GitOpsInstanceName: item.GitOpsInstanceName,
		ClusterName:        item.ClusterName,
		DefaultNamespace:   item.DefaultNamespace,
		Status:             string(item.Status),
		HealthStatus:       item.HealthStatus,
		LastCheckAt:        item.LastCheckAt,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
		Remark:             item.Remark,
	}
}

// toArgoCDEnvBindingResponse 将领域对象转换为接口响应结构。
func toArgoCDEnvBindingResponse(item domain.EnvBinding) ArgoCDEnvBindingResponse {
	return ArgoCDEnvBindingResponse{
		ID:                 item.ID,
		EnvCode:            item.EnvCode,
		ArgoCDInstanceID:   item.ArgoCDInstanceID,
		ArgoCDInstanceCode: item.ArgoCDInstanceCode,
		ArgoCDInstanceName: item.ArgoCDInstanceName,
		ClusterName:        item.ClusterName,
		Priority:           item.Priority,
		Status:             string(item.Status),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

// writeArgoCDHTTPError 写入处理结果或错误信息。
func writeArgoCDHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID), errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInstanceNotFound), errors.Is(err, domain.ErrEnvBindingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
