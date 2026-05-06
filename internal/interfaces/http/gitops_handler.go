package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/gitops"
)

type GitOpsHandler struct {
	templateFields   *usecase.QueryGitOpsTemplateFields
	fieldCandidates  *usecase.QueryGitOpsFieldCandidates
	valuesCandidates *usecase.QueryGitOpsValuesCandidates
	scanPathStatus   *usecase.QueryGitOpsScanPathStatus
	instances        *usecase.GitOpsInstanceManager
	authz            RequestAuthorizer
}

// NewGitOpsHandler 创建并返回对应组件实例。
func NewGitOpsHandler(
	templateFields *usecase.QueryGitOpsTemplateFields,
	fieldCandidates *usecase.QueryGitOpsFieldCandidates,
	valuesCandidates *usecase.QueryGitOpsValuesCandidates,
	scanPathStatus *usecase.QueryGitOpsScanPathStatus,
	instances *usecase.GitOpsInstanceManager,
	authz RequestAuthorizer,
) *GitOpsHandler {
	return &GitOpsHandler{
		templateFields:   templateFields,
		fieldCandidates:  fieldCandidates,
		valuesCandidates: valuesCandidates,
		scanPathStatus:   scanPathStatus,
		instances:        instances,
		authz:            authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *GitOpsHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/gitops/instances", h.ListInstances)
	router.POST("/gitops/instances", h.CreateInstance)
	router.PUT("/gitops/instances/:id", h.UpdateInstance)
	router.GET("/gitops/instances/:id/status", h.GetInstanceStatus)
	router.GET("/gitops/template-fields", h.ListTemplateFields)
	router.GET("/gitops/field-candidates", h.ListFieldCandidates)
	router.GET("/gitops/values-candidates", h.ListValuesCandidates)
	router.GET("/gitops/scan-path-status", h.CheckScanPath)
}

type GitOpsInstanceResponse struct {
	ID                    string    `json:"id"`
	InstanceCode          string    `json:"instance_code"`
	Name                  string    `json:"name"`
	LocalRoot             string    `json:"local_root"`
	DefaultBranch         string    `json:"default_branch"`
	Username              string    `json:"username"`
	AuthorName            string    `json:"author_name"`
	AuthorEmail           string    `json:"author_email"`
	CommitMessageTemplate string    `json:"commit_message_template"`
	CommandTimeoutSec     int       `json:"command_timeout_sec"`
	Status                string    `json:"status"`
	Remark                string    `json:"remark"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type GitOpsInstanceListResponse struct {
	Data     []GitOpsInstanceResponse `json:"data"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	Total    int64                    `json:"total"`
}

type GitOpsInstanceDataResponse struct {
	Data GitOpsInstanceResponse `json:"data"`
}

type GitOpsInstanceStatusData struct {
	Instance GitOpsInstanceResponse          `json:"instance"`
	Status   usecase.QueryGitOpsStatusOutput `json:"status"`
}

type GitOpsInstanceStatusDataResponse struct {
	Data GitOpsInstanceStatusData `json:"data"`
}

type GitOpsTemplateFieldsResponse struct {
	Data []usecase.QueryGitOpsTemplateFieldOutput `json:"data"`
}

type GitOpsFieldCandidatesResponse struct {
	Data []usecase.QueryGitOpsFieldCandidateOutput `json:"data"`
}

type GitOpsValuesCandidatesResponse struct {
	Data []usecase.QueryGitOpsValuesCandidateOutput `json:"data"`
}

type upsertGitOpsInstanceRequest struct {
	InstanceCode          string `json:"instance_code"`
	Name                  string `json:"name"`
	LocalRoot             string `json:"local_root"`
	DefaultBranch         string `json:"default_branch"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	Token                 string `json:"token"`
	AuthorName            string `json:"author_name"`
	AuthorEmail           string `json:"author_email"`
	CommitMessageTemplate string `json:"commit_message_template"`
	CommandTimeoutSec     int    `json:"command_timeout_sec"`
	Status                string `json:"status"`
	Remark                string `json:"remark"`
}

// ListInstances 查询Instances列表。
// @Summary      查询Instances列表
// @Description  查询Instances列表，并按统一响应结构返回处理结果。
// @Tags         gitops
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/instances [get]
func (h *GitOpsHandler) ListInstances(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "component.gitops.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops instance manager is not configured"})
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
		writeGitOpsHTTPError(c, err)
		return
	}
	resp := make([]GitOpsInstanceResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toGitOpsInstanceResponse(item))
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
// @Tags         gitops
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/instances [post]
func (h *GitOpsHandler) CreateInstance(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.gitops.manage", "", "") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops instance manager is not configured"})
		return
	}
	var req upsertGitOpsInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.instances.Create(c.Request.Context(), usecase.CreateGitOpsInstanceInput{
		InstanceCode:          req.InstanceCode,
		Name:                  req.Name,
		LocalRoot:             req.LocalRoot,
		DefaultBranch:         req.DefaultBranch,
		Username:              req.Username,
		Password:              req.Password,
		Token:                 req.Token,
		AuthorName:            req.AuthorName,
		AuthorEmail:           req.AuthorEmail,
		CommitMessageTemplate: req.CommitMessageTemplate,
		CommandTimeoutSec:     req.CommandTimeoutSec,
		Status:                domain.Status(strings.TrimSpace(req.Status)),
		Remark:                req.Remark,
	})
	if err != nil {
		writeGitOpsHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toGitOpsInstanceResponse(item)})
}

// UpdateInstance 更新Instance。
// @Summary      更新Instance
// @Description  更新Instance，并按统一响应结构返回处理结果。
// @Tags         gitops
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/instances/{id} [put]
func (h *GitOpsHandler) UpdateInstance(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.gitops.manage", "", "") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops instance manager is not configured"})
		return
	}
	var req upsertGitOpsInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.instances.Update(c.Request.Context(), c.Param("id"), usecase.UpdateGitOpsInstanceInput{
		InstanceCode:          req.InstanceCode,
		Name:                  req.Name,
		LocalRoot:             req.LocalRoot,
		DefaultBranch:         req.DefaultBranch,
		Username:              req.Username,
		Password:              req.Password,
		Token:                 req.Token,
		AuthorName:            req.AuthorName,
		AuthorEmail:           req.AuthorEmail,
		CommitMessageTemplate: req.CommitMessageTemplate,
		CommandTimeoutSec:     req.CommandTimeoutSec,
		Status:                domain.Status(strings.TrimSpace(req.Status)),
		Remark:                req.Remark,
	})
	if err != nil {
		writeGitOpsHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toGitOpsInstanceResponse(item)})
}

// GetInstanceStatus 获取Instance Status详情。
// @Summary      获取Instance Status详情
// @Description  获取Instance Status详情，并按统一响应结构返回处理结果。
// @Tags         gitops
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/instances/{id}/status [get]
func (h *GitOpsHandler) GetInstanceStatus(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "component.gitops.manage") {
		return
	}
	if h.instances == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops instance manager is not configured"})
		return
	}
	status, item, err := h.instances.GetStatus(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeGitOpsHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": GitOpsInstanceStatusData{
			Instance: toGitOpsInstanceResponse(item),
			Status: usecase.QueryGitOpsStatusOutput{
				Enabled:               status.Enabled,
				LocalRoot:             strings.TrimSpace(status.LocalRoot),
				Mode:                  strings.TrimSpace(status.Mode),
				DefaultBranch:         strings.TrimSpace(status.DefaultBranch),
				Username:              strings.TrimSpace(status.Username),
				AuthorName:            strings.TrimSpace(status.AuthorName),
				AuthorEmail:           strings.TrimSpace(status.AuthorEmail),
				CommitMessageTemplate: strings.TrimSpace(status.CommitMessageTemplate),
				CommandTimeoutSec:     status.CommandTimeoutSec,
				PathExists:            status.PathExists,
				IsGitRepo:             status.IsGitRepo,
				RemoteOrigin:          strings.TrimSpace(status.RemoteOrigin),
				RemoteReachable:       status.RemoteReachable,
				CurrentBranch:         strings.TrimSpace(status.CurrentBranch),
				HeadCommit:            strings.TrimSpace(status.HeadCommit),
				HeadCommitShort:       shortGitOpsCommit(strings.TrimSpace(status.HeadCommit)),
				HeadCommitSubject:     strings.TrimSpace(status.HeadCommitSubject),
				WorktreeDirty:         status.WorktreeDirty,
				StatusSummary:         append([]string(nil), status.StatusSummary...),
			},
		},
	})
}

// ListTemplateFields godoc
// @Summary      List GitOps commit template fields
// @Tags         gitops
// @Produce      json
// @Success      200  {object}  GitOpsTemplateFieldsResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/template-fields [get]
func (h *GitOpsHandler) ListTemplateFields(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "component.gitops.manage") {
		return
	}
	if h.templateFields == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops manager is not configured"})
		return
	}
	output, err := h.templateFields.Execute(c.Request.Context())
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

// ListFieldCandidates godoc
// @Summary      List GitOps YAML field candidates
// @Tags         gitops
// @Produce      json
// @Param        application_id  query     string  true  "application id"
// @Success      200  {object}  GitOpsFieldCandidatesResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/field-candidates [get]
func (h *GitOpsHandler) ListFieldCandidates(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "release.template.manage") {
		return
	}
	if h.fieldCandidates == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops manager is not configured"})
		return
	}
	output, err := h.fieldCandidates.Execute(c.Request.Context(), c.Query("application_id"))
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

// ListValuesCandidates godoc
// @Summary      List GitOps Helm values candidates
// @Tags         gitops
// @Produce      json
// @Param        application_id  query     string  true  "application id"
// @Success      200  {object}  GitOpsValuesCandidatesResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/values-candidates [get]
func (h *GitOpsHandler) ListValuesCandidates(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "release.template.manage") {
		return
	}
	if h.valuesCandidates == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops manager is not configured"})
		return
	}
	output, err := h.valuesCandidates.Execute(c.Request.Context(), c.Query("application_id"))
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

// toGitOpsInstanceResponse 将领域对象转换为接口响应结构。
func toGitOpsInstanceResponse(item domain.Instance) GitOpsInstanceResponse {
	return GitOpsInstanceResponse{
		ID:                    item.ID,
		InstanceCode:          item.InstanceCode,
		Name:                  item.Name,
		LocalRoot:             item.LocalRoot,
		DefaultBranch:         item.DefaultBranch,
		Username:              item.Username,
		AuthorName:            item.AuthorName,
		AuthorEmail:           item.AuthorEmail,
		CommitMessageTemplate: item.CommitMessageTemplate,
		CommandTimeoutSec:     item.CommandTimeoutSec,
		Status:                string(item.Status),
		Remark:                item.Remark,
		CreatedAt:             item.CreatedAt,
		UpdatedAt:             item.UpdatedAt,
	}
}

// writeGitOpsHTTPError 写入处理结果或错误信息。
func writeGitOpsHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID), errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrInstanceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// shortGitOpsCommit 封装当前模块的业务处理逻辑。
func shortGitOpsCommit(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

// CheckScanPath 检查Scan Path。
// @Summary      检查Scan Path
// @Description  检查Scan Path，并按统一响应结构返回处理结果。
// @Tags         gitops
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /gitops/scan-path-status [get]
func (h *GitOpsHandler) CheckScanPath(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.gitops.view", "release.template.manage") {
		return
	}
	if h.scanPathStatus == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gitops manager is not configured"})
		return
	}
	applicationID := c.Query("application_id")
	gitopsType := c.Query("gitops_type")
	if applicationID == "" || gitopsType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application_id and gitops_type are required"})
		return
	}
	output, err := h.scanPathStatus.Execute(c.Request.Context(), applicationID, gitopsType)
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
