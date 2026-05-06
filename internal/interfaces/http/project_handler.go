package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	projectdomain "gos/internal/domain/project"
)

type ProjectHandler struct {
	manager *usecase.ProjectManager
	authz   RequestAuthorizer
}

// NewProjectHandler 创建并返回对应组件实例。
func NewProjectHandler(manager *usecase.ProjectManager, authz RequestAuthorizer) *ProjectHandler {
	return &ProjectHandler{manager: manager, authz: authz}
}

// ensureProjectReadable 校验前置条件，不满足时写入对应错误响应。
func (h *ProjectHandler) ensureProjectReadable(c *gin.Context) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if h.authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}

	// application.manage 仍然是最强的全局权限，直接放行。
	manageAllowed, err := h.authz.HasPermission(c.Request.Context(), user, "application.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if manageAllowed {
		return true
	}

	// 兼容“仅有应用范围的 release.create / application.view”的账号。
	// 这类账号能看到/操作部分应用，但旧逻辑会导致项目列表永远 403，从而前端无法按项目筛选。
	items, err := h.authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	accepted := map[string]struct{}{
		"application.view": {},
		"release.view":     {},
		"release.create":   {},
		"release.execute":  {},
		"release.cancel":   {},
	}
	applicationIDs, envScopes := collectApplicationScopesFromPermissions(items, accepted)
	if len(applicationIDs) > 0 || len(envScopes) > 0 {
		return true
	}

	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
	return false
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ProjectHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/projects", h.List)
	router.GET("/projects/:id", h.GetByID)
	router.POST("/projects", h.Create)
	router.PUT("/projects/:id", h.Update)
	router.DELETE("/projects/:id", h.Delete)
}

type ProjectRequest struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type ProjectResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Key         string    `json:"key"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Create 创建资源。
// @Summary      创建资源
// @Description  创建资源，并按统一响应结构返回处理结果。
// @Tags         projects
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects [post]
func (h *ProjectHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}
	var req ProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Create(c.Request.Context(), usecase.CreateProjectInput{
		Name:        req.Name,
		Key:         req.Key,
		Description: req.Description,
		Status:      projectdomain.Status(strings.TrimSpace(req.Status)),
	})
	if err != nil {
		writeProjectHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toProjectResponse(item)})
}

// GetByID 获取By ID详情。
// @Summary      获取By ID详情
// @Description  获取By ID详情，并按统一响应结构返回处理结果。
// @Tags         projects
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects/{id} [get]
func (h *ProjectHandler) GetByID(c *gin.Context) {
	if !h.ensureProjectReadable(c) {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeProjectHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toProjectResponse(item)})
}

// List 查询资源列表。
// @Summary      查询资源列表
// @Description  查询资源列表，并按统一响应结构返回处理结果。
// @Tags         projects
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects [get]
func (h *ProjectHandler) List(c *gin.Context) {
	if !h.ensureProjectReadable(c) {
		return
	}
	page, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return
	}
	pageSize, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page_size", "20")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size"})
		return
	}
	items, total, err := h.manager.List(c.Request.Context(), projectdomain.ListFilter{
		Key:      c.Query("key"),
		Name:     c.Query("name"),
		Status:   projectdomain.Status(strings.TrimSpace(c.Query("status"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeProjectHTTPError(c, err)
		return
	}
	resp := make([]ProjectResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toProjectResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
	})
}

// Update 更新资源。
// @Summary      更新资源
// @Description  更新资源，并按统一响应结构返回处理结果。
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects/{id} [put]
func (h *ProjectHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}
	var req ProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), projectdomain.UpdateInput{
		Name:        req.Name,
		Key:         req.Key,
		Description: req.Description,
		Status:      projectdomain.Status(strings.TrimSpace(req.Status)),
	})
	if err != nil {
		writeProjectHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toProjectResponse(item)})
}

// Delete 删除资源。
// @Summary      删除资源
// @Description  删除资源，并按统一响应结构返回处理结果。
// @Tags         projects
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects/{id} [delete]
func (h *ProjectHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeProjectHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

// toProjectResponse 将领域对象转换为接口响应结构。
func toProjectResponse(item projectdomain.Project) ProjectResponse {
	return ProjectResponse{
		ID:          item.ID,
		Name:        item.Name,
		Key:         item.Key,
		Description: item.Description,
		Status:      string(item.Status),
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

// writeProjectHTTPError 写入处理结果或错误信息。
func writeProjectHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID), errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, projectdomain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, projectdomain.ErrKeyDuplicated), errors.Is(err, projectdomain.ErrInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
