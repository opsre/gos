package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/application"
	projectdomain "gos/internal/domain/project"
	userdomain "gos/internal/domain/user"
)

type ApplicationHandler struct {
	creator             *usecase.CreateApplication
	query               *usecase.QueryApplication
	updater             *usecase.UpdateApplication
	deleter             *usecase.DeleteApplication
	workbench           *usecase.ApplicationWorkbench
	approvalFlowManager *usecase.ReleaseOrderManager
	users               ApplicationUserReader
	authz               RequestAuthorizer
}

func (h *ApplicationHandler) SetApprovalFlowManager(manager *usecase.ReleaseOrderManager) {
	if h != nil {
		h.approvalFlowManager = manager
	}
}

// SetWorkbenchQuery 注入应用工作台聚合查询；保留构造函数签名以兼容已有调用方。
func (h *ApplicationHandler) SetWorkbenchQuery(query *usecase.ApplicationWorkbench) {
	if h != nil {
		h.workbench = query
	}
}

type ApplicationUserReader interface {
	GetUserByID(ctx context.Context, id string) (userdomain.User, error)
}

// NewApplicationHandler 创建并返回对应组件实例。
func NewApplicationHandler(
	creator *usecase.CreateApplication,
	query *usecase.QueryApplication,
	updater *usecase.UpdateApplication,
	deleter *usecase.DeleteApplication,
	users ApplicationUserReader,
	authz RequestAuthorizer,
) *ApplicationHandler {
	return &ApplicationHandler{
		creator: creator,
		query:   query,
		updater: updater,
		deleter: deleter,
		users:   users,
		authz:   authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ApplicationHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/applications", h.Create)
	router.GET("/applications/options", h.ListOptions)
	router.GET("/applications/workbench", h.Workbench)
	router.GET("/applications/:id/approval-flow", h.GetApprovalFlowBinding)
	router.PUT("/applications/:id/approval-flow", h.UpdateApprovalFlowBinding)
	router.GET("/applications/:id", h.GetByID)
	router.GET("/applications", h.List)
	router.PUT("/applications/:id", h.Update)
	router.DELETE("/applications/:id", h.Delete)
}

type CreateApplicationRequest struct {
	Name                 string                       `json:"name"`
	Key                  string                       `json:"key"`
	ProjectID            string                       `json:"project_id"`
	RepoURL              string                       `json:"repo_url"`
	Description          string                       `json:"description"`
	OwnerUserID          string                       `json:"owner_user_id"`
	Owner                string                       `json:"owner"`
	Status               string                       `json:"status"`
	ArtifactType         string                       `json:"artifact_type"`
	ArtifactRepositoryID string                       `json:"artifact_repository_id"`
	ArtifactDirectory    string                       `json:"artifact_directory"`
	Language             string                       `json:"language"`
	GitOpsBranchMappings []domain.GitOpsBranchMapping `json:"gitops_branch_mappings"`
	ReleaseBranches      []domain.ReleaseBranchOption `json:"release_branches"`
}

type UpdateApplicationRequest struct {
	Name                 string                       `json:"name"`
	Key                  string                       `json:"key"`
	ProjectID            string                       `json:"project_id"`
	RepoURL              string                       `json:"repo_url"`
	Description          string                       `json:"description"`
	OwnerUserID          string                       `json:"owner_user_id"`
	Owner                string                       `json:"owner"`
	Status               string                       `json:"status"`
	ArtifactType         string                       `json:"artifact_type"`
	ArtifactRepositoryID string                       `json:"artifact_repository_id"`
	ArtifactDirectory    string                       `json:"artifact_directory"`
	Language             string                       `json:"language"`
	GitOpsBranchMappings []domain.GitOpsBranchMapping `json:"gitops_branch_mappings"`
	ReleaseBranches      []domain.ReleaseBranchOption `json:"release_branches"`
}

type ApplicationResponse struct {
	ID                   string                       `json:"id"`
	Name                 string                       `json:"name"`
	Key                  string                       `json:"key"`
	ProjectID            string                       `json:"project_id"`
	ProjectName          string                       `json:"project_name"`
	ProjectKey           string                       `json:"project_key"`
	RepoURL              string                       `json:"repo_url"`
	Description          string                       `json:"description"`
	OwnerUserID          string                       `json:"owner_user_id"`
	Owner                string                       `json:"owner"`
	Status               string                       `json:"status"`
	ArtifactType         string                       `json:"artifact_type"`
	ArtifactRepositoryID string                       `json:"artifact_repository_id"`
	ArtifactDirectory    string                       `json:"artifact_directory"`
	Language             string                       `json:"language"`
	GitOpsBranchMappings []domain.GitOpsBranchMapping `json:"gitops_branch_mappings"`
	ReleaseBranches      []domain.ReleaseBranchOption `json:"release_branches"`
	CreatedAt            time.Time                    `json:"created_at"`
	UpdatedAt            time.Time                    `json:"updated_at"`
}

type ApplicationDataResponse struct {
	Data ApplicationResponse `json:"data"`
}

type ApplicationListResponse struct {
	Data     []ApplicationResponse `json:"data"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}

type ApplicationWorkbenchOverviewResponse struct {
	ApplicationIDs []string               `json:"application_ids"`
	ReleaseOrders  []ReleaseOrderResponse `json:"release_orders"`
}

type ApplicationWorkbenchResponse struct {
	Data                       []ApplicationResponse                `json:"data"`
	Page                       int                                  `json:"page"`
	PageSize                   int                                  `json:"page_size"`
	Total                      int64                                `json:"total"`
	TemplateNamesByApplication map[string][]string                  `json:"template_names_by_application"`
	RecentReleaseOrders        []ReleaseOrderResponse               `json:"recent_release_orders"`
	ReleaseStateSummaries      []AppReleaseStateSummaryResponse     `json:"release_state_summaries"`
	Overview                   ApplicationWorkbenchOverviewResponse `json:"overview"`
}

type ApplicationOptionResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ApplicationOptionListResponse struct {
	Data []ApplicationOptionResponse `json:"data"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// Create godoc
// @Summary      Create application
// @Tags         applications
// @Accept       json
// @Produce      json
// @Param        request  body      CreateApplicationRequest  true  "Create application request"
// @Success      201      {object}  ApplicationDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /applications [post]
func (h *ApplicationHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ownerUserID := strings.TrimSpace(req.OwnerUserID)
	if ownerUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "owner_user_id is required"})
		return
	}
	ownerName, ownerErr := h.resolveOwnerDisplayName(c, ownerUserID)
	if ownerErr != nil {
		writeHTTPError(c, ownerErr)
		return
	}

	app, err := h.creator.Execute(c.Request.Context(), usecase.CreateInput{
		Name:                 req.Name,
		Key:                  req.Key,
		ProjectID:            req.ProjectID,
		RepoURL:              req.RepoURL,
		Description:          req.Description,
		OwnerUserID:          ownerUserID,
		Owner:                ownerName,
		Status:               domain.Status(strings.TrimSpace(req.Status)),
		ArtifactType:         req.ArtifactType,
		ArtifactRepositoryID: req.ArtifactRepositoryID,
		ArtifactDirectory:    req.ArtifactDirectory,
		Language:             req.Language,
		GitOpsBranchMappings: req.GitOpsBranchMappings,
		ReleaseBranches:      req.ReleaseBranches,
	})
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toResponse(app)})
}

// GetByID godoc
// @Summary      Get application by ID
// @Tags         applications
// @Produce      json
// @Param        id   path      string  true  "Application ID"
// @Success      200  {object}  ApplicationDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id} [get]
func (h *ApplicationHandler) GetByID(c *gin.Context) {
	app, err := h.query.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	if !ensureApplicationVisible(c, h.authz, app.ID) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toResponse(app)})
}

// List godoc
// @Summary      List applications
// @Tags         applications
// @Produce      json
// @Param        keyword    query     string  false  "Application keyword (name or key)"
// @Param        key        query     string  false  "Application key"
// @Param        name       query     string  false  "Application name"
// @Param        project_id query     string  false  "Project ID"
// @Param        status     query     string  false  "Application status"
// @Param        page       query     int     false  "Page number, starts from 1"
// @Param        page_size  query     int     false  "Page size, max 100"
// @Success      200     {object}  ApplicationListResponse
// @Failure      400     {object}  ErrorResponse
// @Failure      500     {object}  ErrorResponse
// @Router       /applications [get]
func (h *ApplicationHandler) List(c *gin.Context) {
	page, err := parsePositiveIntQuery(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveIntQuery(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allowAll, visibleApplicationIDs, ok := resolveVisibleApplicationIDsForApplications(c, h.authz)
	if !ok {
		return
	}
	if !allowAll && len(visibleApplicationIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data":      []ApplicationResponse{},
			"page":      resolvePage(page),
			"page_size": resolvePageSize(pageSize),
			"total":     0,
		})
		return
	}

	apps, total, err := h.query.List(c.Request.Context(), domain.ListFilter{
		Keyword:        c.Query("keyword"),
		Key:            c.Query("key"),
		Name:           c.Query("name"),
		ProjectID:      c.Query("project_id"),
		Status:         domain.Status(strings.TrimSpace(c.Query("status"))),
		ApplicationIDs: resolveApplicationFilterIDs(strings.TrimSpace(c.Query("application_id")), allowAll, visibleApplicationIDs),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		writeHTTPError(c, err)
		return
	}

	resp := make([]ApplicationResponse, 0, len(apps))
	for _, app := range apps {
		resp = append(resp, toResponse(app))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvePage(page),
		"page_size": resolvePageSize(pageSize),
		"total":     total,
	})
}

// Workbench 返回应用工作台一次渲染与一次轮询所需的聚合快照。
// @Summary      查询应用工作台聚合数据
// @Tags         applications
// @Produce      json
// @Router       /applications/workbench [get]
func (h *ApplicationHandler) Workbench(c *gin.Context) {
	if h.workbench == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "application workbench is not configured"})
		return
	}
	page, err := parsePositiveIntQuery(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveIntQuery(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allowAllApplications, visibleApplicationIDs, ok := resolveVisibleApplicationIDsForApplications(c, h.authz)
	if !ok {
		return
	}
	allowAllReleases, visibleReleaseApplicationIDs, releaseScopes, releaseOK := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !releaseOK {
		return
	}
	allowAllTemplates, visibleTemplateApplicationIDs, templateOK := resolveVisibleTemplateApplicationIDs(c, h.authz)
	if !templateOK {
		return
	}

	applicationFilter := domain.ListFilter{
		Keyword:        c.Query("keyword"),
		Key:            c.Query("key"),
		Name:           c.Query("name"),
		ProjectID:      c.Query("project_id"),
		Status:         domain.Status(strings.TrimSpace(c.Query("status"))),
		ApplicationIDs: resolveApplicationFilterIDs(strings.TrimSpace(c.Query("application_id")), allowAllApplications, visibleApplicationIDs),
		Page:           page,
		PageSize:       pageSize,
	}
	if !allowAllApplications && len(visibleApplicationIDs) == 0 {
		applicationFilter.ApplicationIDs = []string{"__none__"}
	}

	overviewApplicationIDs := resolveApplicationFilterIDs("", allowAllApplications, visibleApplicationIDs)
	if !allowAllApplications && len(overviewApplicationIDs) == 0 {
		overviewApplicationIDs = []string{"__none__"}
	}
	output, err := h.workbench.Load(c.Request.Context(), usecase.ApplicationWorkbenchInput{
		ApplicationFilter:       applicationFilter,
		OverviewApplicationIDs:  overviewApplicationIDs,
		ReleaseApplicationIDs:   releaseApplicationIDsForWorkbench(allowAllReleases, visibleReleaseApplicationIDs),
		ReleaseApplicationScope: releaseScopes,
		TemplateApplicationIDs:  templateApplicationIDsForWorkbench(allowAllTemplates, visibleTemplateApplicationIDs),
		IncludeReleaseData:      allowAllReleases || len(visibleReleaseApplicationIDs) > 0 || len(releaseScopes) > 0,
		IncludeTemplateData:     allowAllTemplates || len(visibleTemplateApplicationIDs) > 0,
	})
	if err != nil {
		writeHTTPError(c, err)
		return
	}

	applications := make([]ApplicationResponse, 0, len(output.Applications))
	for _, item := range output.Applications {
		applications = append(applications, toResponse(item))
	}
	templateNames := make(map[string][]string)
	for _, item := range output.Templates {
		applicationID := strings.TrimSpace(item.ApplicationID)
		name := strings.TrimSpace(item.Name)
		if applicationID == "" || name == "" || containsTemplateName(templateNames[applicationID], name) {
			continue
		}
		templateNames[applicationID] = append(templateNames[applicationID], name)
	}
	recentOrders := make([]ReleaseOrderResponse, 0, len(output.RecentReleaseOrders))
	for _, item := range output.RecentReleaseOrders {
		recentOrders = append(recentOrders, toReleaseOrderResponse(item))
	}
	stateSummaries := make([]AppReleaseStateSummaryResponse, 0, len(output.ReleaseStateSummaries))
	for _, item := range output.ReleaseStateSummaries {
		stateSummaries = append(stateSummaries, toAppReleaseStateSummaryResponse(item))
	}
	overviewOrders := make([]ReleaseOrderResponse, 0, len(output.OverviewReleaseOrders))
	for _, item := range output.OverviewReleaseOrders {
		overviewOrders = append(overviewOrders, toReleaseOrderResponse(item))
	}

	c.JSON(http.StatusOK, ApplicationWorkbenchResponse{
		Data:                       applications,
		Page:                       output.Page,
		PageSize:                   output.PageSize,
		Total:                      output.Total,
		TemplateNamesByApplication: templateNames,
		RecentReleaseOrders:        recentOrders,
		ReleaseStateSummaries:      stateSummaries,
		Overview: ApplicationWorkbenchOverviewResponse{
			ApplicationIDs: output.OverviewApplicationIDs,
			ReleaseOrders:  overviewOrders,
		},
	})
}

func releaseApplicationIDsForWorkbench(allowAll bool, visibleIDs []string) []string {
	if allowAll {
		return nil
	}
	return append([]string(nil), visibleIDs...)
}

func templateApplicationIDsForWorkbench(allowAll bool, visibleIDs []string) []string {
	if allowAll {
		return nil
	}
	return append([]string(nil), visibleIDs...)
}

func containsTemplateName(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// ListOptions 查询Options列表。
// @Summary      查询Options列表
// @Description  查询Options列表，并按统一响应结构返回处理结果。
// @Tags         applications
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/options [get]
func (h *ApplicationHandler) ListOptions(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "application.manage", "system.permission.manage") {
		return
	}

	const pageSize = 100
	page := 1
	items := make([]domain.Application, 0)
	for {
		batch, total, err := h.query.List(c.Request.Context(), domain.ListFilter{
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			writeHTTPError(c, err)
			return
		}
		items = append(items, batch...)
		if len(items) >= int(total) || len(batch) < pageSize {
			break
		}
		page++
	}

	sort.Slice(items, func(i, j int) bool {
		leftName := strings.TrimSpace(items[i].Name)
		rightName := strings.TrimSpace(items[j].Name)
		if leftName == rightName {
			return strings.TrimSpace(items[i].Key) < strings.TrimSpace(items[j].Key)
		}
		return leftName < rightName
	})

	resp := make([]ApplicationOptionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, ApplicationOptionResponse{
			ID:   item.ID,
			Name: item.Name,
			Key:  item.Key,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ensureApplicationVisible 校验前置条件，不满足时写入对应错误响应。
func ensureApplicationVisible(c *gin.Context, authz RequestAuthorizer, applicationID string) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	if user.Role == userdomain.RoleAdmin {
		return true
	}

	manageAllowed, err := authz.HasPermission(c.Request.Context(), user, "application.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if manageAllowed {
		return true
	}

	allowed, err := authz.HasPermission(c.Request.Context(), user, "application.view", "application", strings.TrimSpace(applicationID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if allowed {
		return true
	}

	releaseAllowed, err := authz.HasPermission(c.Request.Context(), user, "release.create", "application", strings.TrimSpace(applicationID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if !releaseAllowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return false
	}
	return true
}

// resolveVisibleApplicationIDsForApplications 解析上下文数据，得到后续流程需要的结果。
func resolveVisibleApplicationIDsForApplications(
	c *gin.Context,
	authz RequestAuthorizer,
) (allowAll bool, applicationIDs []string, ok bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false, nil, false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false, nil, false
	}
	if user.Role == userdomain.RoleAdmin {
		return true, nil, true
	}

	manageAllowed, err := authz.HasPermission(c.Request.Context(), user, "application.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	if manageAllowed {
		return true, nil, true
	}

	items, err := authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	accepted := map[string]struct{}{
		"application.view": {},
		"release.view":     {},
		"release.create":   {},
		"release.execute":  {},
		"release.cancel":   {},
	}
	result, envScopes := collectApplicationScopesFromPermissions(items, accepted)
	seen := make(map[string]struct{}, len(result)+len(envScopes))
	for _, item := range result {
		seen[item] = struct{}{}
	}
	for _, item := range envScopes {
		applicationID := strings.TrimSpace(item.ApplicationID)
		if applicationID == "" {
			continue
		}
		if _, exists := seen[applicationID]; exists {
			continue
		}
		seen[applicationID] = struct{}{}
		result = append(result, applicationID)
	}
	sort.Strings(result)
	return false, result, true
}

// resolveVisibleTemplateApplicationIDs 保持工作台模板可见性与模板列表一致：
// 管理员/模板管理员可见全部，其余用户只得到具备 release.create 权限的应用。
func resolveVisibleTemplateApplicationIDs(
	c *gin.Context,
	authz RequestAuthorizer,
) (allowAll bool, applicationIDs []string, ok bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false, nil, false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false, nil, false
	}
	if user.Role == userdomain.RoleAdmin {
		return true, nil, true
	}
	manageAllowed, err := authz.HasPermission(c.Request.Context(), user, "release.template.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	if manageAllowed {
		return true, nil, true
	}
	items, err := authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	applicationIDs, scopes := collectApplicationScopesFromPermissions(items, map[string]struct{}{
		"release.create": {},
	})
	seen := make(map[string]struct{}, len(applicationIDs)+len(scopes))
	for _, item := range applicationIDs {
		seen[item] = struct{}{}
	}
	for _, item := range scopes {
		applicationID := strings.TrimSpace(item.ApplicationID)
		if applicationID == "" {
			continue
		}
		if _, exists := seen[applicationID]; exists {
			continue
		}
		seen[applicationID] = struct{}{}
		applicationIDs = append(applicationIDs, applicationID)
	}
	sort.Strings(applicationIDs)
	return false, applicationIDs, true
}

// resolveApplicationFilterIDs 解析上下文数据，得到后续流程需要的结果。
func resolveApplicationFilterIDs(applicationID string, allowAll bool, visibleApplicationIDs []string) []string {
	applicationID = strings.TrimSpace(applicationID)
	if allowAll {
		if applicationID == "" {
			return nil
		}
		return []string{applicationID}
	}
	if applicationID != "" {
		for _, item := range visibleApplicationIDs {
			if strings.TrimSpace(item) == applicationID {
				return []string{applicationID}
			}
		}
		return []string{"__none__"}
	}
	return visibleApplicationIDs
}

// Update godoc
// @Summary      Update application
// @Tags         applications
// @Accept       json
// @Produce      json
// @Param        id       path      string                    true  "Application ID"
// @Param        request  body      UpdateApplicationRequest  true  "Update application request"
// @Success      200      {object}  ApplicationDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /applications/{id} [put]
func (h *ApplicationHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}

	var req UpdateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ownerUserID := strings.TrimSpace(req.OwnerUserID)
	if ownerUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "owner_user_id is required"})
		return
	}
	ownerName, ownerErr := h.resolveOwnerDisplayName(c, ownerUserID)
	if ownerErr != nil {
		writeHTTPError(c, ownerErr)
		return
	}

	app, err := h.updater.Execute(c.Request.Context(), c.Param("id"), domain.UpdateInput{
		Name:                 req.Name,
		Key:                  req.Key,
		ProjectID:            req.ProjectID,
		RepoURL:              req.RepoURL,
		Description:          req.Description,
		OwnerUserID:          ownerUserID,
		Owner:                ownerName,
		Status:               domain.Status(strings.TrimSpace(req.Status)),
		ArtifactType:         req.ArtifactType,
		ArtifactRepositoryID: req.ArtifactRepositoryID,
		ArtifactDirectory:    req.ArtifactDirectory,
		Language:             req.Language,
		GitOpsBranchMappings: req.GitOpsBranchMappings,
		ReleaseBranches:      req.ReleaseBranches,
	})
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toResponse(app)})
}

// Delete godoc
// @Summary      Delete application
// @Tags         applications
// @Produce      json
// @Param        id   path  string  true  "Application ID"
// @Success      204  {string}  string  "No Content"
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id} [delete]
func (h *ApplicationHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}
	err := h.deleter.Execute(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// toResponse 将领域对象转换为接口响应结构。
func toResponse(app domain.Application) ApplicationResponse {
	return ApplicationResponse{
		ID:                   app.ID,
		Name:                 app.Name,
		Key:                  app.Key,
		ProjectID:            app.ProjectID,
		ProjectName:          app.ProjectName,
		ProjectKey:           app.ProjectKey,
		RepoURL:              app.RepoURL,
		Description:          app.Description,
		OwnerUserID:          app.OwnerUserID,
		Owner:                app.Owner,
		Status:               string(app.Status),
		ArtifactType:         app.ArtifactType,
		ArtifactRepositoryID: app.ArtifactRepositoryID,
		ArtifactDirectory:    app.ArtifactDirectory,
		Language:             app.Language(),
		GitOpsBranchMappings: app.GitOpsBranchMappings,
		ReleaseBranches:      app.ReleaseBranches,
		CreatedAt:            app.CreatedAt,
		UpdatedAt:            app.UpdatedAt,
	}
}

// resolveOwnerDisplayName 解析上下文数据，得到后续流程需要的结果。
func (h *ApplicationHandler) resolveOwnerDisplayName(c *gin.Context, ownerUserID string) (string, error) {
	if h.users == nil {
		return "", errors.New("owner user resolver is not configured")
	}
	user, err := h.users.GetUserByID(c.Request.Context(), ownerUserID)
	if err != nil {
		return "", err
	}
	if user.Status != userdomain.StatusActive {
		return "", fmt.Errorf("%w: owner user is inactive", usecase.ErrInvalidInput)
	}
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = strings.TrimSpace(user.Username)
	}
	return name, nil
}

// writeHTTPError 写入处理结果或错误信息。
func writeHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID), errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, userdomain.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKeyDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, usecase.ErrReferencedConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, projectdomain.ErrNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, projectdomain.ErrKeyDuplicated), errors.Is(err, projectdomain.ErrInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// parsePositiveIntQuery 解析输入内容并返回结构化结果。
func parsePositiveIntQuery(c *gin.Context, name string) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	if value < 1 {
		return 0, errors.New(name + " must be greater than 0")
	}
	return value, nil
}

// resolvePage 解析上下文数据，得到后续流程需要的结果。
func resolvePage(page int) int {
	if page > 0 {
		return page
	}
	return 1
}

// resolvePageSize 解析上下文数据，得到后续流程需要的结果。
func resolvePageSize(pageSize int) int {
	const (
		defaultPageSize = 20
		maxPageSize     = 100
	)
	if pageSize < 1 {
		return defaultPageSize
	}
	if pageSize > maxPageSize {
		return maxPageSize
	}
	return pageSize
}
