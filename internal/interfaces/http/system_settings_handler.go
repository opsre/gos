package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
)

type SystemSettingsHandler struct {
	query  *usecase.QueryReleaseSettings
	update *usecase.UpdateReleaseSettings
	authz  RequestAuthorizer
}

// NewSystemSettingsHandler 创建并返回对应组件实例。
func NewSystemSettingsHandler(
	query *usecase.QueryReleaseSettings,
	update *usecase.UpdateReleaseSettings,
	authz RequestAuthorizer,
) *SystemSettingsHandler {
	return &SystemSettingsHandler{
		query:  query,
		update: update,
		authz:  authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *SystemSettingsHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/system/settings/release", h.GetReleaseSettings)
	router.PUT("/system/settings/release", h.UpdateReleaseSettings)
}

type ReleaseSettingsResponse struct {
	Data usecase.ReleaseSettingsOutput `json:"data"`
}

type UpdateReleaseSettingsRequest struct {
	EnvOptions   []string                                `json:"env_options"`
	Concurrency  usecase.ReleaseConcurrencySettingsInput `json:"concurrency"`
	GitOpsConfig usecase.ReleaseGitOpsConfigInput        `json:"gitops_config"`
}

// GetReleaseSettings 获取Release Settings详情。
// @Summary      获取Release Settings详情
// @Description  获取Release Settings详情，并按统一响应结构返回处理结果。
// @Tags         system-settings
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /system/settings/release [get]
func (h *SystemSettingsHandler) GetReleaseSettings(c *gin.Context) {
	if !h.ensureReleaseSettingsVisible(c) {
		return
	}
	if h.query == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "release settings are not configured"})
		return
	}
	output, err := h.query.Execute(c.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ensureReleaseSettingsVisible 校验前置条件，不满足时写入对应错误响应。
func (h *SystemSettingsHandler) ensureReleaseSettingsVisible(c *gin.Context) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if h.authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}

	for _, code := range []string{"release.template.manage", "system.permission.manage"} {
		allowed, err := h.authz.HasPermission(c.Request.Context(), user, code, "", "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return false
		}
		if allowed {
			return true
		}
	}

	items, err := h.authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	applicationIDs, envScopes := collectApplicationScopesFromPermissions(items, map[string]struct{}{
		"release.create": {},
	})
	if len(applicationIDs) > 0 || len(envScopes) > 0 {
		return true
	}

	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
	return false
}

// UpdateReleaseSettings 更新Release Settings。
// @Summary      更新Release Settings
// @Description  更新Release Settings，并按统一响应结构返回处理结果。
// @Tags         system-settings
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /system/settings/release [put]
func (h *SystemSettingsHandler) UpdateReleaseSettings(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	if h.update == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "release settings are not configured"})
		return
	}
	var req UpdateReleaseSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.update.Execute(c.Request.Context(), usecase.UpdateReleaseSettingsInput{
		EnvOptions:   req.EnvOptions,
		Concurrency:  req.Concurrency,
		GitOpsConfig: req.GitOpsConfig,
	})
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}
