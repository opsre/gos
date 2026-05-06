package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/platformparam"
)

type PlatformParamHandler struct {
	manager *usecase.PlatformParamDictManager
	authz   RequestAuthorizer
}

// NewPlatformParamHandler 创建并返回对应组件实例。
func NewPlatformParamHandler(manager *usecase.PlatformParamDictManager, authz RequestAuthorizer) *PlatformParamHandler {
	return &PlatformParamHandler{
		manager: manager,
		authz:   authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *PlatformParamHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/platform-param-dicts", h.Create)
	router.GET("/platform-param-dicts", h.List)
	router.GET("/platform-param-dicts/:id", h.GetByID)
	router.PUT("/platform-param-dicts/:id", h.Update)
	router.DELETE("/platform-param-dicts/:id", h.Delete)
}

type CreatePlatformParamDictRequest struct {
	ParamKey      string `json:"param_key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ParamType     string `json:"param_type"`
	Required      bool   `json:"required"`
	GitOpsLocator bool   `json:"gitops_locator"`
	CDSelfFill    bool   `json:"cd_self_fill"`
	Status        *int   `json:"status"`
}

type UpdatePlatformParamDictRequest struct {
	ParamKey      string `json:"param_key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ParamType     string `json:"param_type"`
	Required      bool   `json:"required"`
	GitOpsLocator bool   `json:"gitops_locator"`
	CDSelfFill    bool   `json:"cd_self_fill"`
	Status        int    `json:"status"`
}

type PlatformParamDictResponse struct {
	ID            string    `json:"id"`
	ParamKey      string    `json:"param_key"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	ParamType     string    `json:"param_type"`
	Required      bool      `json:"required"`
	GitOpsLocator bool      `json:"gitops_locator"`
	CDSelfFill    bool      `json:"cd_self_fill"`
	Builtin       bool      `json:"builtin"`
	Status        int       `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PlatformParamDictDataResponse struct {
	Data PlatformParamDictResponse `json:"data"`
}

type PlatformParamDictListResponse struct {
	Data     []PlatformParamDictResponse `json:"data"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int64                       `json:"total"`
}

// Create godoc
// @Summary      Create platform param dict
// @Tags         platform-param-dicts
// @Accept       json
// @Produce      json
// @Param        request  body      CreatePlatformParamDictRequest  true  "Create platform param dict request"
// @Success      201      {object}  PlatformParamDictDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /platform-param-dicts [post]
func (h *PlatformParamHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "platform_param.manage", "", "") {
		return
	}
	var req CreatePlatformParamDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.Create(c.Request.Context(), usecase.CreatePlatformParamDictInput{
		ParamKey:      req.ParamKey,
		Name:          req.Name,
		Description:   req.Description,
		ParamType:     domain.ParamType(strings.TrimSpace(req.ParamType)),
		Required:      req.Required,
		GitOpsLocator: req.GitOpsLocator,
		CDSelfFill:    req.CDSelfFill,
		Status:        createPlatformParamStatus(req.Status),
	})
	if err != nil {
		writePlatformParamHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toPlatformParamResponse(item)})
}

// List godoc
// @Summary      List platform param dicts
// @Tags         platform-param-dicts
// @Produce      json
// @Param        param_key  query     string  false  "Platform param key"
// @Param        name       query     string  false  "Platform param name"
// @Param        status     query     int     false  "Status: 1 enabled, 0 disabled"
// @Param        builtin    query     bool    false  "Builtin flag"
// @Param        page       query     int     false  "Page number"
// @Param        page_size  query     int     false  "Page size"
// @Success      200  {object}  PlatformParamDictListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /platform-param-dicts [get]
func (h *PlatformParamHandler) List(c *gin.Context) {
	if !ensurePermission(c, h.authz, "platform_param.manage", "", "") {
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
	status, err := parseOptionalPlatformParamStatus(c, "status")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	builtin, err := parseOptionalBool(c, "builtin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, total, err := h.manager.List(c.Request.Context(), domain.ListFilter{
		ParamKey: c.Query("param_key"),
		Name:     c.Query("name"),
		Status:   status,
		Builtin:  builtin,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writePlatformParamHTTPError(c, err)
		return
	}

	resp := make([]PlatformParamDictResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toPlatformParamResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetByID godoc
// @Summary      Get platform param dict by ID
// @Tags         platform-param-dicts
// @Produce      json
// @Param        id   path      string  true  "Platform param dict ID"
// @Success      200  {object}  PlatformParamDictDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /platform-param-dicts/{id} [get]
func (h *PlatformParamHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "platform_param.manage", "", "") {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePlatformParamHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPlatformParamResponse(item)})
}

// Update godoc
// @Summary      Update platform param dict
// @Tags         platform-param-dicts
// @Accept       json
// @Produce      json
// @Param        id       path      string                          true  "Platform param dict ID"
// @Param        request  body      UpdatePlatformParamDictRequest  true  "Update platform param dict request"
// @Success      200      {object}  PlatformParamDictDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /platform-param-dicts/{id} [put]
func (h *PlatformParamHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "platform_param.manage", "", "") {
		return
	}
	var req UpdatePlatformParamDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), domain.UpdateInput{
		ParamKey:      req.ParamKey,
		Name:          req.Name,
		Description:   req.Description,
		ParamType:     domain.ParamType(strings.TrimSpace(req.ParamType)),
		Required:      req.Required,
		GitOpsLocator: req.GitOpsLocator,
		CDSelfFill:    req.CDSelfFill,
		Status:        domain.Status(req.Status),
	})
	if err != nil {
		writePlatformParamHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPlatformParamResponse(item)})
}

// Delete godoc
// @Summary      Delete platform param dict
// @Tags         platform-param-dicts
// @Produce      json
// @Param        id   path  string  true  "Platform param dict ID"
// @Success      204  {string}  string  "No Content"
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /platform-param-dicts/{id} [delete]
func (h *PlatformParamHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "platform_param.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writePlatformParamHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// toPlatformParamResponse 将领域对象转换为接口响应结构。
func toPlatformParamResponse(item domain.PlatformParamDict) PlatformParamDictResponse {
	return PlatformParamDictResponse{
		ID:            item.ID,
		ParamKey:      item.ParamKey,
		Name:          item.Name,
		Description:   item.Description,
		ParamType:     string(item.ParamType),
		Required:      item.Required,
		GitOpsLocator: item.GitOpsLocator,
		CDSelfFill:    item.CDSelfFill,
		Builtin:       item.Builtin,
		Status:        int(item.Status),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

// writePlatformParamHTTPError 写入处理结果或错误信息。
func writePlatformParamHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus),
		errors.Is(err, usecase.ErrInvalidParamKey),
		errors.Is(err, usecase.ErrInvalidParamType):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrParamKeyDuplicated),
		errors.Is(err, usecase.ErrBuiltinProtected),
		errors.Is(err, usecase.ErrReferencedConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// parseOptionalPlatformParamStatus 解析输入内容并返回结构化结果。
func parseOptionalPlatformParamStatus(c *gin.Context, name string) (*domain.Status, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, errors.New(name + " must be 0 or 1")
	}
	status := domain.Status(value)
	if !status.Valid() {
		return nil, errors.New(name + " must be 0 or 1")
	}
	return &status, nil
}

// createPlatformParamStatus 创建业务资源并返回处理结果。
func createPlatformParamStatus(status *int) domain.Status {
	if status == nil {
		return domain.StatusEnabled
	}
	return domain.Status(*status)
}
