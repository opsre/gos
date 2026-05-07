package httpapi

import (
	"errors"
	"net/http"

	"gos/internal/application/usecase"
	"gos/internal/domain/k8sinstance"

	"github.com/gin-gonic/gin"
)

type K8sClusterRefHandler struct {
	manager *usecase.K8sClusterRefManager
	authz   RequestAuthorizer
}

func NewK8sClusterRefHandler(manager *usecase.K8sClusterRefManager, authz RequestAuthorizer) *K8sClusterRefHandler {
	return &K8sClusterRefHandler{manager: manager, authz: authz}
}

func (h *K8sClusterRefHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/k8s-cluster-refs", h.list)
	router.POST("/k8s-cluster-refs", h.create)
	router.GET("/k8s-cluster-refs/:id", h.get)
	router.PUT("/k8s-cluster-refs/:id", h.update)
	router.DELETE("/k8s-cluster-refs/:id", h.delete)
}

type createK8sClusterRefRequest struct {
	Code                   string `json:"code"`
	ClusterName            string `json:"cluster_name"`
	EnvironmentCode        string `json:"environment_code"`
	APIServer              string `json:"api_server"`
	DefaultNamespace       string `json:"default_namespace"`
	ArgoCDInstanceID       string `json:"argocd_instance_id"`
	SupportsNativeStrategy bool   `json:"supports_native_strategy"`
	SupportsRollouts       bool   `json:"supports_rollouts"`
	TrafficProvider        string `json:"traffic_provider"`
}

type k8sClusterRefResponse struct {
	ID                     string `json:"id"`
	Code                   string `json:"code"`
	ClusterName            string `json:"cluster_name"`
	EnvironmentCode        string `json:"environment_code"`
	APIServer              string `json:"api_server"`
	DefaultNamespace       string `json:"default_namespace"`
	AccessMode             string `json:"access_mode"`
	ArgoCDInstanceID       string `json:"argocd_instance_id"`
	SupportsNativeStrategy bool   `json:"supports_native_strategy"`
	SupportsRollouts       bool   `json:"supports_rollouts"`
	TrafficProvider        string `json:"traffic_provider"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

func toK8sClusterRefResponse(item k8sinstance.K8sClusterRef) k8sClusterRefResponse {
	return k8sClusterRefResponse{
		ID:                     item.ID,
		Code:                   item.Code,
		ClusterName:            item.ClusterName,
		EnvironmentCode:        item.EnvironmentCode,
		APIServer:              item.APIServer,
		DefaultNamespace:       item.DefaultNamespace,
		AccessMode:             string(item.AccessMode),
		ArgoCDInstanceID:       item.ArgoCDInstanceID,
		SupportsNativeStrategy: item.SupportsNativeStrategy,
		SupportsRollouts:       item.SupportsRollouts,
		TrafficProvider:        item.TrafficProvider,
		CreatedAt:              item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:              item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *K8sClusterRefHandler) list(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var filter k8sinstance.ListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	items, total, err := h.manager.List(c.Request.Context(), filter)
	if err != nil {
		writeK8sClusterRefHTTPError(c, err)
		return
	}

	responses := make([]k8sClusterRefResponse, len(items))
	for i, item := range items {
		responses[i] = toK8sClusterRefResponse(item)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": responses, "total": total}})
}

func (h *K8sClusterRefHandler) create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req createK8sClusterRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.Create(c.Request.Context(), usecase.CreateK8sClusterRefInput{
		Code:                   req.Code,
		ClusterName:            req.ClusterName,
		EnvironmentCode:        req.EnvironmentCode,
		APIServer:              req.APIServer,
		DefaultNamespace:       req.DefaultNamespace,
		ArgoCDInstanceID:       req.ArgoCDInstanceID,
		SupportsNativeStrategy: req.SupportsNativeStrategy,
		SupportsRollouts:       req.SupportsRollouts,
		TrafficProvider:        req.TrafficProvider,
	})
	if err != nil {
		writeK8sClusterRefHTTPError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toK8sClusterRefResponse(item)})
}

func (h *K8sClusterRefHandler) get(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	item, err := h.manager.GetByID(c.Request.Context(), id)
	if err != nil {
		writeK8sClusterRefHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toK8sClusterRefResponse(item)})
}

type updateK8sClusterRefRequest struct {
	Code                   *string `json:"code"`
	ClusterName            *string `json:"cluster_name"`
	EnvironmentCode        *string `json:"environment_code"`
	APIServer              *string `json:"api_server"`
	DefaultNamespace       *string `json:"default_namespace"`
	ArgoCDInstanceID       *string `json:"argocd_instance_id"`
	SupportsNativeStrategy *bool   `json:"supports_native_strategy"`
	SupportsRollouts       *bool   `json:"supports_rollouts"`
	TrafficProvider        *string `json:"traffic_provider"`
}

func (h *K8sClusterRefHandler) update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	var req updateK8sClusterRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.Update(c.Request.Context(), id, usecase.UpdateK8sClusterRefInput{
		Code:                   req.Code,
		ClusterName:            req.ClusterName,
		EnvironmentCode:        req.EnvironmentCode,
		APIServer:              req.APIServer,
		DefaultNamespace:       req.DefaultNamespace,
		ArgoCDInstanceID:       req.ArgoCDInstanceID,
		SupportsNativeStrategy: req.SupportsNativeStrategy,
		SupportsRollouts:       req.SupportsRollouts,
		TrafficProvider:        req.TrafficProvider,
	})
	if err != nil {
		writeK8sClusterRefHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toK8sClusterRefResponse(item)})
}

func (h *K8sClusterRefHandler) delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	id := c.Param("id")
	if err := h.manager.Delete(c.Request.Context(), id); err != nil {
		writeK8sClusterRefHTTPError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func writeK8sClusterRefHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, k8sinstance.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "k8s cluster ref not found"})
	case errors.Is(err, k8sinstance.ErrCodeDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": "k8s cluster ref code already exists"})
	case errors.Is(err, k8sinstance.ErrInUse):
		c.JSON(http.StatusConflict, gin.H{"error": "k8s cluster ref is in use by runtime bindings"})
	case errors.Is(err, usecase.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
