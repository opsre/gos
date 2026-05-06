package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	userdomain "gos/internal/domain/user"
)

type AuthHandler struct {
	auth  *usecase.AuthSessionManager
	users *usecase.UserManagement
}

// NewAuthHandler 创建并返回对应组件实例。
func NewAuthHandler(auth *usecase.AuthSessionManager, users *usecase.UserManagement) *AuthHandler {
	return &AuthHandler{
		auth:  auth,
		users: users,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *AuthHandler) RegisterRoutes(router gin.IRouter) {
	h.RegisterPublicRoutes(router)
	h.RegisterProtectedRoutes(router)
}

// RegisterPublicRoutes 封装当前模块的业务处理逻辑。
func (h *AuthHandler) RegisterPublicRoutes(router gin.IRouter) {
	router.POST("/auth/login", h.Login)
}

// RegisterProtectedRoutes 封装当前模块的业务处理逻辑。
func (h *AuthHandler) RegisterProtectedRoutes(router gin.IRouter) {
	router.POST("/auth/logout", h.Logout)
	router.GET("/me", h.Me)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Data struct {
		AccessToken string       `json:"access_token"`
		ExpiredAt   time.Time    `json:"expired_at"`
		User        UserResponse `json:"user"`
	} `json:"data"`
}

type MeResponse struct {
	Data struct {
		User             UserResponse                  `json:"user"`
		Permissions      []UserPermissionResponse      `json:"permissions"`
		ParamPermissions []UserParamPermissionResponse `json:"param_permissions"`
	} `json:"data"`
}

// Login 登录系统。
// @Summary      登录系统
// @Description  登录系统，并按统一响应结构返回处理结果。
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	result, err := h.auth.Login(c.Request.Context(), usecase.LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		ClientIP:  c.ClientIP(),
		UserAgent: strings.TrimSpace(c.GetHeader("User-Agent")),
	})
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}

	resp := LoginResponse{}
	resp.Data.AccessToken = result.AccessToken
	resp.Data.ExpiredAt = result.ExpiredAt
	resp.Data.User = toUserResponse(result.User)
	c.JSON(http.StatusOK, resp)
}

// Logout 退出登录。
// @Summary      退出登录
// @Description  退出登录，并按统一响应结构返回处理结果。
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	token := extractBearerToken(c.GetHeader("Authorization"))
	if token != "" {
		_ = h.auth.Logout(c.Request.Context(), token)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

// Me 获取当前用户信息。
// @Summary      获取当前用户信息
// @Description  获取当前用户信息，并按统一响应结构返回处理结果。
// @Tags         auth
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	permissions, err := h.auth.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}

	paramPermissions, err := h.users.ListUserParamPermissions(c.Request.Context(), user.ID, "")
	if err != nil && !errors.Is(err, userdomain.ErrUserNotFound) {
		writeUserHTTPError(c, err)
		return
	}

	resp := MeResponse{}
	resp.Data.User = toUserResponse(user)
	resp.Data.Permissions = make([]UserPermissionResponse, 0, len(permissions))
	for _, item := range permissions {
		resp.Data.Permissions = append(resp.Data.Permissions, toUserPermissionResponse(item))
	}
	resp.Data.ParamPermissions = make([]UserParamPermissionResponse, 0, len(paramPermissions))
	for _, item := range paramPermissions {
		resp.Data.ParamPermissions = append(resp.Data.ParamPermissions, toUserParamPermissionResponse(item))
	}

	c.JSON(http.StatusOK, resp)
}
