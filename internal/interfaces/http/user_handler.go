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

type UserHandler struct {
	users *usecase.UserManagement
	authz RequestAuthorizer
}

// NewUserHandler 创建并返回对应组件实例。
func NewUserHandler(users *usecase.UserManagement, authz RequestAuthorizer) *UserHandler {
	return &UserHandler{
		users: users,
		authz: authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *UserHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/users", h.ListUsers)
	router.GET("/users/options", h.ListUserOptions)
	router.GET("/users/organization", h.ListUserOrganization)
	router.GET("/users/:id", h.GetUserByID)
	router.POST("/users", h.CreateUser)
	router.PUT("/users/:id", h.UpdateUser)
	router.DELETE("/users/:id", h.DeleteUser)
	router.GET("/users/:id/manager", h.GetUserManager)
	router.PUT("/users/:id/manager", h.SetUserManager)

	router.GET("/permissions", h.ListPermissions)
	router.GET("/users/:id/permissions", h.ListUserPermissions)
	router.POST("/users/:id/permissions", h.GrantUserPermissions)
	router.DELETE("/users/:id/permissions", h.RevokeUserPermissions)

	router.GET("/users/:id/param-permissions", h.ListUserParamPermissions)
	router.POST("/users/:id/param-permissions", h.UpsertUserParamPermission)
	router.PUT("/users/:id/param-permissions/:permission_id", h.UpsertUserParamPermission)
	router.DELETE("/users/:id/param-permissions/:permission_id", h.DeleteUserParamPermission)
}

type UserResponse struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserDataResponse struct {
	Data UserResponse `json:"data"`
}

type UserListResponse struct {
	Data     []UserResponse `json:"data"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int64          `json:"total"`
}

type UserOptionResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type UserOptionListResponse struct {
	Data []UserOptionResponse `json:"data"`
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Password    string `json:"password"`
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Password    string `json:"password"`
}

type UserManagerRequest struct {
	ManagerUserID string `json:"manager_user_id"`
}

type UserManagerResponse struct {
	UserID        string `json:"user_id"`
	ManagerUserID string `json:"manager_user_id"`
}

type UserOrganizationNodeResponse struct {
	UserResponse
	ManagerUserID string `json:"manager_user_id"`
}

type UserPermissionRequest struct {
	Items []UserPermissionItem `json:"items"`
}

type UserPermissionItem struct {
	PermissionCode string `json:"permission_code"`
	ScopeType      string `json:"scope_type"`
	ScopeValue     string `json:"scope_value"`
}

type UserPermissionResponse struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	PermissionCode string    `json:"permission_code"`
	ScopeType      string    `json:"scope_type"`
	ScopeValue     string    `json:"scope_value"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UserPermissionListResponse struct {
	Data []UserPermissionResponse `json:"data"`
}

type PermissionResponse struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Module      string    `json:"module"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PermissionListResponse struct {
	Data []PermissionResponse `json:"data"`
}

type UserParamPermissionRequest struct {
	ParamKey      string `json:"param_key"`
	ApplicationID string `json:"application_id"`
	CanView       bool   `json:"can_view"`
	CanEdit       bool   `json:"can_edit"`
}

type UserParamPermissionResponse struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	ParamKey      string    `json:"param_key"`
	ApplicationID string    `json:"application_id"`
	CanView       bool      `json:"can_view"`
	CanEdit       bool      `json:"can_edit"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UserParamPermissionDataResponse struct {
	Data UserParamPermissionResponse `json:"data"`
}

type UserParamPermissionListResponse struct {
	Data []UserParamPermissionResponse `json:"data"`
}

// ListUsers 查询Users列表。
// @Summary      查询Users列表
// @Description  查询Users列表，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
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
	items, total, err := h.users.ListUsers(c.Request.Context(), userdomain.UserListFilter{
		Username: c.Query("username"),
		Name:     c.Query("name"),
		Role:     userdomain.Role(strings.TrimSpace(c.Query("role"))),
		Status:   userdomain.Status(strings.TrimSpace(c.Query("status"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}

	resp := make([]UserResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toUserResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// ListUserOptions 查询User Options列表。
// @Summary      查询User Options列表
// @Description  查询User Options列表，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/options [get]
func (h *UserHandler) ListUserOptions(c *gin.Context) {
	if !ensureAnyPermission(
		c,
		h.authz,
		"application.manage",
		"release.template.manage",
		"system.user.manage",
		"system.permission.manage",
	) {
		return
	}
	items, err := h.users.ListUserOptions(c.Request.Context())
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	resp := make([]UserOptionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, UserOptionResponse{
			ID:          item.ID,
			Username:    item.Username,
			DisplayName: item.DisplayName,
			Role:        string(item.Role),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) ListUserOrganization(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	items, err := h.users.ListUserOrganization(c.Request.Context())
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	resp := make([]UserOrganizationNodeResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, UserOrganizationNodeResponse{
			UserResponse:  toUserResponse(item.User),
			ManagerUserID: item.ManagerUserID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// GetUserByID 获取User By ID详情。
// @Summary      获取User By ID详情
// @Description  获取User By ID详情，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	item, err := h.users.GetUserByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toUserResponse(item)})
}

func (h *UserHandler) GetUserManager(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	managerUserID, err := h.users.GetUserManagerID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": UserManagerResponse{UserID: strings.TrimSpace(c.Param("id")), ManagerUserID: managerUserID}})
}

func (h *UserHandler) SetUserManager(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	var req UserManagerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.users.SetUserManagerID(c.Request.Context(), c.Param("id"), req.ManagerUserID); err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": UserManagerResponse{UserID: strings.TrimSpace(c.Param("id")), ManagerUserID: strings.TrimSpace(req.ManagerUserID)}})
}

// CreateUser 创建User。
// @Summary      创建User
// @Description  创建User，并按统一响应结构返回处理结果。
// @Tags         users
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.users.CreateUser(c.Request.Context(), usecase.CreateUserInput{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Phone:       req.Phone,
		Role:        userdomain.Role(strings.TrimSpace(req.Role)),
		Status:      userdomain.Status(strings.TrimSpace(req.Status)),
		Password:    req.Password,
	})
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toUserResponse(item)})
}

// UpdateUser 更新User。
// @Summary      更新User
// @Description  更新User，并按统一响应结构返回处理结果。
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.users.UpdateUser(c.Request.Context(), c.Param("id"), usecase.UpdateUserInput{
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Phone:       req.Phone,
		Role:        userdomain.Role(strings.TrimSpace(req.Role)),
		Status:      userdomain.Status(strings.TrimSpace(req.Status)),
		Password:    req.Password,
	})
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toUserResponse(item)})
}

// DeleteUser 删除User。
// @Summary      删除User
// @Description  删除User，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.user.manage", "", "") {
		return
	}
	if err := h.users.DeleteUser(c.Request.Context(), c.Param("id")); err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListPermissions 查询Permissions列表。
// @Summary      查询Permissions列表
// @Description  查询Permissions列表，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /permissions [get]
func (h *UserHandler) ListPermissions(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	items, err := h.users.ListPermissions(c.Request.Context(), userdomain.PermissionFilter{
		Module: c.Query("module"),
		Action: c.Query("action"),
	})
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	resp := make([]PermissionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, PermissionResponse{
			ID:          item.ID,
			Code:        item.Code,
			Name:        item.Name,
			Module:      item.Module,
			Action:      item.Action,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ListUserPermissions 查询User Permissions列表。
// @Summary      查询User Permissions列表
// @Description  查询User Permissions列表，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/permissions [get]
func (h *UserHandler) ListUserPermissions(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	items, err := h.users.ListUserPermissions(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	resp := make([]UserPermissionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toUserPermissionResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// GrantUserPermissions 处理Grant User Permissions接口。
// @Summary      处理Grant User Permissions接口
// @Description  处理Grant User Permissions接口，并按统一响应结构返回处理结果。
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/permissions [post]
func (h *UserHandler) GrantUserPermissions(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req UserPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	items := make([]userdomain.UserPermissionGrant, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, userdomain.UserPermissionGrant{
			PermissionCode: item.PermissionCode,
			ScopeType:      item.ScopeType,
			ScopeValue:     item.ScopeValue,
		})
	}
	if err := h.users.GrantUserPermissions(c.Request.Context(), c.Param("id"), items); err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

// RevokeUserPermissions 处理Revoke User Permissions接口。
// @Summary      处理Revoke User Permissions接口
// @Description  处理Revoke User Permissions接口，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/permissions [delete]
func (h *UserHandler) RevokeUserPermissions(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req UserPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	items := make([]userdomain.UserPermissionGrant, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, userdomain.UserPermissionGrant{
			PermissionCode: item.PermissionCode,
			ScopeType:      item.ScopeType,
			ScopeValue:     item.ScopeValue,
		})
	}
	if err := h.users.RevokeUserPermissions(c.Request.Context(), c.Param("id"), items); err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

// ListUserParamPermissions 查询User Param Permissions列表。
// @Summary      查询User Param Permissions列表
// @Description  查询User Param Permissions列表，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/param-permissions [get]
func (h *UserHandler) ListUserParamPermissions(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	items, err := h.users.ListUserParamPermissions(c.Request.Context(), c.Param("id"), c.Query("application_id"))
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	resp := make([]UserParamPermissionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toUserParamPermissionResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// UpsertUserParamPermission 处理Upsert User Param Permission接口。
// @Summary      处理Upsert User Param Permission接口
// @Description  处理Upsert User Param Permission接口，并按统一响应结构返回处理结果。
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        permission_id  path  string  true  "参数权限 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/param-permissions [post]
// @Router       /users/{id}/param-permissions/{permission_id} [put]
func (h *UserHandler) UpsertUserParamPermission(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req UserParamPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item := userdomain.UserParamPermission{
		ID:            strings.TrimSpace(c.Param("permission_id")),
		UserID:        strings.TrimSpace(c.Param("id")),
		ParamKey:      req.ParamKey,
		ApplicationID: req.ApplicationID,
		CanView:       req.CanView,
		CanEdit:       req.CanEdit,
	}
	updated, err := h.users.UpsertUserParamPermission(c.Request.Context(), item)
	if err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toUserParamPermissionResponse(updated)})
}

// DeleteUserParamPermission 删除User Param Permission。
// @Summary      删除User Param Permission
// @Description  删除User Param Permission，并按统一响应结构返回处理结果。
// @Tags         users
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        permission_id  path  string  true  "参数权限 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id}/param-permissions/{permission_id} [delete]
func (h *UserHandler) DeleteUserParamPermission(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	if err := h.users.DeleteUserParamPermission(c.Request.Context(), c.Param("permission_id")); err != nil {
		writeUserHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// toUserResponse 将领域对象转换为接口响应结构。
func toUserResponse(item userdomain.User) UserResponse {
	return UserResponse{
		ID:          item.ID,
		Username:    item.Username,
		DisplayName: item.DisplayName,
		Email:       item.Email,
		Phone:       item.Phone,
		Role:        string(item.Role),
		Status:      string(item.Status),
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

// toUserPermissionResponse 将领域对象转换为接口响应结构。
func toUserPermissionResponse(item userdomain.UserPermission) UserPermissionResponse {
	return UserPermissionResponse{
		ID:             item.ID,
		UserID:         item.UserID,
		PermissionCode: item.PermissionCode,
		ScopeType:      item.ScopeType,
		ScopeValue:     item.ScopeValue,
		Enabled:        item.Enabled,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

// toUserParamPermissionResponse 将领域对象转换为接口响应结构。
func toUserParamPermissionResponse(item userdomain.UserParamPermission) UserParamPermissionResponse {
	return UserParamPermissionResponse{
		ID:            item.ID,
		UserID:        item.UserID,
		ParamKey:      item.ParamKey,
		ApplicationID: item.ApplicationID,
		CanView:       item.CanView,
		CanEdit:       item.CanEdit,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

// writeUserHTTPError 写入处理结果或错误信息。
func writeUserHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, userdomain.ErrUserNotFound),
		errors.Is(err, userdomain.ErrSessionNotFound),
		errors.Is(err, userdomain.ErrPermissionNotFound),
		errors.Is(err, userdomain.ErrParamPermissionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, userdomain.ErrUsernameDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, userdomain.ErrUserManagerCycle):
		c.JSON(http.StatusConflict, gin.H{"error": "直属主管关系形成循环"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
