package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/gitcredential"
)

const (
	gitCredentialViewPermission   = "component.credential.view"
	gitCredentialManagePermission = "component.credential.manage"
)

type GitCredentialHandler struct {
	manager *usecase.GitCredentialManager
	commits *usecase.GitCommitManager
	authz   RequestAuthorizer
}

// NewGitCredentialHandler 创建并返回对应组件实例。
func NewGitCredentialHandler(
	manager *usecase.GitCredentialManager,
	commits *usecase.GitCommitManager,
	authz RequestAuthorizer,
) *GitCredentialHandler {
	return &GitCredentialHandler{
		manager: manager,
		commits: commits,
		authz:   authz,
	}
}

// RegisterRoutes 注册当前模块的路由。
func (h *GitCredentialHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/git-credentials", h.List)
	router.GET("/git-credentials/:id", h.GetByID)
	router.POST("/git-credentials", h.Create)
	router.PUT("/git-credentials/:id", h.Update)
	router.DELETE("/git-credentials/:id", h.Delete)
	router.POST("/git-credentials/:id/test", h.TestConnection)
	// 发布单最近提交仍然属于 /release-orders 语义，但它的数据来源是 Git 凭证
	// 解析。注册在这里可以避免既有 release handler 额外依赖凭证仓库，同时路径
	// 与前端契约保持一致。
	router.GET("/release-orders/recent-commits", h.RecentCommits)
}

type GitCredentialRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Secret   string `json:"secret"`
	AuthType string `json:"auth_type"`
	Status   string `json:"status"`
	Remark   string `json:"remark"`
}

// GitCredentialResponse deliberately carries no secret value. SecretConfigured
// lets the UI show credential state, and a blank secret on update means "keep
// the stored one".
type GitCredentialResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	BaseURL          string    `json:"base_url"`
	Username         string    `json:"username"`
	AuthType         string    `json:"auth_type"`
	Status           string    `json:"status"`
	SecretConfigured bool      `json:"secret_configured"`
	Remark           string    `json:"remark"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GitCredentialConnectionTestResponse struct {
	OK             bool   `json:"ok"`
	Message        string `json:"message"`
	GitLabUsername string `json:"gitlab_username,omitempty"`
}

func (h *GitCredentialHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialManagePermission, "", "") {
		return
	}
	var req GitCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Create(c.Request.Context(), toGitCredentialInput(req))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toGitCredentialResponse(item)})
}

func (h *GitCredentialHandler) List(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialViewPermission, "", "") {
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
	items, total, err := h.manager.List(c.Request.Context(), domain.ListFilter{
		Keyword:  c.Query("keyword"),
		Status:   domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	resp := make([]GitCredentialResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toGitCredentialResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

func (h *GitCredentialHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialViewPermission, "", "") {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toGitCredentialResponse(item)})
}

func (h *GitCredentialHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialManagePermission, "", "") {
		return
	}
	var req GitCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), toGitCredentialInput(req))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toGitCredentialResponse(item)})
}

func (h *GitCredentialHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialManagePermission, "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// TestConnection reports the GitLab user the stored credential authenticates as.
// A rejected credential is a normal result, not a server error, so it comes back
// as ok=false with a readable message.
func (h *GitCredentialHandler) TestConnection(c *gin.Context) {
	if !ensurePermission(c, h.authz, gitCredentialManagePermission, "", "") {
		return
	}
	result, err := h.manager.TestConnection(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": GitCredentialConnectionTestResponse{
		OK:             result.OK,
		Message:        result.Message,
		GitLabUsername: result.GitLabUsername,
	}})
}

// RecentCommits serves the release order list and detail views. Order ids the
// caller cannot see are dropped here, before the manager touches GitLab.
func (h *GitCredentialHandler) RecentCommits(c *gin.Context) {
	allowAll, visibleApplicationIDs, ok := resolveVisibleApplicationIDsForApplications(c, h.authz)
	if !ok {
		return
	}
	if h.commits == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "recent commits lookup is not configured"})
		return
	}
	limit, err := parsePositiveInt(c, "limit")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	query := usecase.GitCommitQuery{
		OrderIDs: splitGitCommitOrderIDs(c.Query("order_ids")),
		Limit:    limit,
	}
	if !allowAll {
		// An empty, non-nil slice means "no application is visible"; nil would
		// mean "no restriction", so it must never be sent for a restricted user.
		visible := visibleApplicationIDs
		if visible == nil {
			visible = []string{}
		}
		query.VisibleApplicationIDs = visible
	}
	result, err := h.commits.RecentCommits(c.Request.Context(), query)
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func toGitCredentialInput(req GitCredentialRequest) usecase.GitCredentialInput {
	return usecase.GitCredentialInput{
		Name:     req.Name,
		Provider: req.Provider,
		BaseURL:  req.BaseURL,
		Username: req.Username,
		Secret:   req.Secret,
		AuthType: req.AuthType,
		Status:   req.Status,
		Remark:   req.Remark,
	}
}

func toGitCredentialResponse(item domain.Credential) GitCredentialResponse {
	return GitCredentialResponse{
		ID:               item.ID,
		Name:             item.Name,
		Provider:         string(item.Provider),
		BaseURL:          item.BaseURL,
		Username:         item.Username,
		AuthType:         string(item.AuthType),
		Status:           string(item.Status),
		SecretConfigured: strings.TrimSpace(item.Secret) != "",
		Remark:           item.Remark,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

// splitGitCommitOrderIDs turns the comma separated order_ids query into a slice.
// The de-duplication and the upper bound live in the use case.
func splitGitCommitOrderIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		result = append(result, id)
	}
	return result
}
