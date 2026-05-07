package httpapi

import (
	"errors"
	"net/http"

	"gos/internal/application/usecase"
	"gos/internal/domain/strategy"

	"github.com/gin-gonic/gin"
)

type StrategyHandler struct {
	manager *usecase.StrategyTemplateManager
	authz   RequestAuthorizer
}

func NewStrategyHandler(manager *usecase.StrategyTemplateManager, authz RequestAuthorizer) *StrategyHandler {
	return &StrategyHandler{manager: manager, authz: authz}
}

func (h *StrategyHandler) RegisterRoutes(router gin.IRouter) {
	templates := router.Group("/release-strategy-templates")
	{
		templates.GET("", h.listTemplates)
		templates.POST("", h.createTemplate)
		templates.GET("/:id", h.getTemplate)
		templates.PUT("/:id", h.updateTemplate)
		templates.DELETE("/:id", h.deleteTemplate)
	}

	runtimeBindings := router.Group("/application-env-runtime-bindings")
	{
		runtimeBindings.GET("", h.listRuntimeBindings)
		runtimeBindings.POST("", h.createRuntimeBinding)
		runtimeBindings.GET("/:id", h.getRuntimeBinding)
		runtimeBindings.PUT("/:id", h.updateRuntimeBinding)
		runtimeBindings.DELETE("/:id", h.deleteRuntimeBinding)
	}

	strategyBindings := router.Group("/application-env-strategy-bindings")
	{
		strategyBindings.GET("", h.listStrategyBindings)
		strategyBindings.POST("", h.createStrategyBinding)
		strategyBindings.GET("/:id", h.getStrategyBinding)
		strategyBindings.PUT("/:id", h.updateStrategyBinding)
		strategyBindings.DELETE("/:id", h.deleteStrategyBinding)
	}
}

type templateResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StrategyEngine string `json:"strategy_engine"`
	StrategyType   string `json:"strategy_type"`
	StrategyConfig string `json:"strategy_config"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toTemplateResponse(item strategy.ReleaseStrategyTemplate) templateResponse {
	return templateResponse{
		ID:             item.ID,
		Name:           item.Name,
		StrategyEngine: string(item.StrategyEngine),
		StrategyType:   string(item.StrategyType),
		StrategyConfig: item.StrategyConfig,
		Description:    item.Description,
		Status:         string(item.Status),
		CreatedAt:      item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

type runtimeBindingResponse struct {
	ID              string `json:"id"`
	ApplicationID   string `json:"application_id"`
	EnvCode         string `json:"env_code"`
	K8sClusterRefID string `json:"k8s_cluster_ref_id"`
	Namespace       string `json:"namespace"`
	WorkloadName    string `json:"workload_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func toRuntimeBindingResponse(item strategy.ApplicationEnvRuntimeBinding) runtimeBindingResponse {
	return runtimeBindingResponse{
		ID:              item.ID,
		ApplicationID:   item.ApplicationID,
		EnvCode:         item.EnvCode,
		K8sClusterRefID: item.K8sClusterRefID,
		Namespace:       item.Namespace,
		WorkloadName:    item.WorkloadName,
		CreatedAt:       item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

type strategyBindingResponse struct {
	ID                 string `json:"id"`
	ApplicationID      string `json:"application_id"`
	EnvCode            string `json:"env_code"`
	StrategyTemplateID string `json:"strategy_template_id"`
	OverridesConfig    string `json:"overrides_config"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

func toStrategyBindingResponse(item strategy.ApplicationEnvStrategyBinding) strategyBindingResponse {
	return strategyBindingResponse{
		ID:                 item.ID,
		ApplicationID:      item.ApplicationID,
		EnvCode:            item.EnvCode,
		StrategyTemplateID: item.StrategyTemplateID,
		OverridesConfig:    item.OverridesConfig,
		CreatedAt:          item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *StrategyHandler) listTemplates(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var filter strategy.TemplateListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	items, total, err := h.manager.ListTemplates(c.Request.Context(), filter)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	responses := make([]templateResponse, len(items))
	for i, item := range items {
		responses[i] = toTemplateResponse(item)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": responses, "total": total}})
}

type createTemplateRequest struct {
	Name           string `json:"name"`
	StrategyEngine string `json:"strategy_engine"`
	StrategyType   string `json:"strategy_type"`
	StrategyConfig string `json:"strategy_config"`
	Description    string `json:"description"`
}

func (h *StrategyHandler) createTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.CreateTemplate(c.Request.Context(), usecase.CreateTemplateInput{
		Name:           req.Name,
		StrategyEngine: req.StrategyEngine,
		StrategyType:   req.StrategyType,
		StrategyConfig: req.StrategyConfig,
		Description:    req.Description,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toTemplateResponse(item)})
}

func (h *StrategyHandler) getTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	item, err := h.manager.GetTemplate(c.Request.Context(), id)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toTemplateResponse(item)})
}

type updateTemplateRequest struct {
	Name           *string `json:"name"`
	StrategyEngine *string `json:"strategy_engine"`
	StrategyType   *string `json:"strategy_type"`
	StrategyConfig *string `json:"strategy_config"`
	Description    *string `json:"description"`
	Status         *string `json:"status"`
}

func (h *StrategyHandler) updateTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	var req updateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.UpdateTemplate(c.Request.Context(), id, usecase.UpdateTemplateInput{
		Name:           req.Name,
		StrategyEngine: req.StrategyEngine,
		StrategyType:   req.StrategyType,
		StrategyConfig: req.StrategyConfig,
		Description:    req.Description,
		Status:         req.Status,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toTemplateResponse(item)})
}

func (h *StrategyHandler) deleteTemplate(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	if err := h.manager.DeleteTemplate(c.Request.Context(), id); err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *StrategyHandler) listRuntimeBindings(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var filter strategy.RuntimeBindingListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	items, total, err := h.manager.ListRuntimeBindings(c.Request.Context(), filter)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	responses := make([]runtimeBindingResponse, len(items))
	for i, item := range items {
		responses[i] = toRuntimeBindingResponse(item)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": responses, "total": total}})
}

type createRuntimeBindingRequest struct {
	ApplicationID   string `json:"application_id"`
	EnvCode         string `json:"env_code"`
	K8sClusterRefID string `json:"k8s_cluster_ref_id"`
	Namespace       string `json:"namespace"`
	WorkloadName    string `json:"workload_name"`
}

func (h *StrategyHandler) createRuntimeBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req createRuntimeBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.CreateRuntimeBinding(c.Request.Context(), usecase.CreateRuntimeBindingInput{
		ApplicationID:   req.ApplicationID,
		EnvCode:         req.EnvCode,
		K8sClusterRefID: req.K8sClusterRefID,
		Namespace:       req.Namespace,
		WorkloadName:    req.WorkloadName,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toRuntimeBindingResponse(item)})
}

func (h *StrategyHandler) getRuntimeBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	item, err := h.manager.GetRuntimeBinding(c.Request.Context(), id)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toRuntimeBindingResponse(item)})
}

type updateRuntimeBindingRequest struct {
	K8sClusterRefID *string `json:"k8s_cluster_ref_id"`
	Namespace       *string `json:"namespace"`
	WorkloadName    *string `json:"workload_name"`
}

func (h *StrategyHandler) updateRuntimeBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	var req updateRuntimeBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.UpdateRuntimeBinding(c.Request.Context(), id, usecase.UpdateRuntimeBindingInput{
		K8sClusterRefID: req.K8sClusterRefID,
		Namespace:       req.Namespace,
		WorkloadName:    req.WorkloadName,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toRuntimeBindingResponse(item)})
}

func (h *StrategyHandler) deleteRuntimeBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	if err := h.manager.DeleteRuntimeBinding(c.Request.Context(), id); err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *StrategyHandler) listStrategyBindings(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var filter strategy.StrategyBindingListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	items, total, err := h.manager.ListStrategyBindings(c.Request.Context(), filter)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	responses := make([]strategyBindingResponse, len(items))
	for i, item := range items {
		responses[i] = toStrategyBindingResponse(item)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": responses, "total": total}})
}

type createStrategyBindingRequest struct {
	ApplicationID      string `json:"application_id"`
	EnvCode            string `json:"env_code"`
	StrategyTemplateID string `json:"strategy_template_id"`
	OverridesConfig    string `json:"overrides_config"`
}

func (h *StrategyHandler) createStrategyBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req createStrategyBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.CreateStrategyBinding(c.Request.Context(), usecase.CreateStrategyBindingInput{
		ApplicationID:      req.ApplicationID,
		EnvCode:            req.EnvCode,
		StrategyTemplateID: req.StrategyTemplateID,
		OverridesConfig:    req.OverridesConfig,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toStrategyBindingResponse(item)})
}

func (h *StrategyHandler) getStrategyBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	item, err := h.manager.GetStrategyBinding(c.Request.Context(), id)
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toStrategyBindingResponse(item)})
}

type updateStrategyBindingRequest struct {
	StrategyTemplateID *string `json:"strategy_template_id"`
	OverridesConfig    *string `json:"overrides_config"`
}

func (h *StrategyHandler) updateStrategyBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	var req updateStrategyBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.UpdateStrategyBinding(c.Request.Context(), id, usecase.UpdateStrategyBindingInput{
		StrategyTemplateID: req.StrategyTemplateID,
		OverridesConfig:    req.OverridesConfig,
	})
	if err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toStrategyBindingResponse(item)})
}

func (h *StrategyHandler) deleteStrategyBinding(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	if err := h.manager.DeleteStrategyBinding(c.Request.Context(), id); err != nil {
		writeStrategyHTTPError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func writeStrategyHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, strategy.ErrTemplateNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy template not found"})
	case errors.Is(err, strategy.ErrTemplateNameDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": "strategy template name already exists"})
	case errors.Is(err, strategy.ErrTemplateInUse):
		c.JSON(http.StatusConflict, gin.H{"error": "strategy template is in use by strategy bindings"})
	case errors.Is(err, strategy.ErrRuntimeBindingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "runtime binding not found"})
	case errors.Is(err, strategy.ErrRuntimeBindingDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": "runtime binding already exists for this application and environment"})
	case errors.Is(err, strategy.ErrStrategyBindingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy binding not found"})
	case errors.Is(err, strategy.ErrStrategyBindingDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": "strategy binding already exists for this application and environment"})
	case errors.Is(err, usecase.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
