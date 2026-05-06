package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	pipelinedomain "gos/internal/domain/pipeline"
	releasedomain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
)

type ReleaseTemplateHandler struct {
	manager *usecase.ReleaseTemplateManager
	authz   RequestAuthorizer
}

// NewReleaseTemplateHandler 创建并返回对应组件实例。
func NewReleaseTemplateHandler(
	manager *usecase.ReleaseTemplateManager,
	authz RequestAuthorizer,
) *ReleaseTemplateHandler {
	return &ReleaseTemplateHandler{
		manager: manager,
		authz:   authz,
	}
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ReleaseTemplateHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/release-templates", h.List)
	router.POST("/release-templates", h.Create)
	router.GET("/release-templates/:id", h.GetByID)
	router.PUT("/release-templates/:id", h.Update)
	router.DELETE("/release-templates/:id", h.Delete)
}

type CreateReleaseTemplateRequest struct {
	Name                  string                              `json:"name"`
	ApplicationID         string                              `json:"application_id"`
	CIBindingID           string                              `json:"ci_binding_id"`
	CDBindingID           string                              `json:"cd_binding_id"`
	CDProvider            string                              `json:"cd_provider"`
	GitOpsType            string                              `json:"gitops_type"`
	Status                string                              `json:"status"`
	Remark                string                              `json:"remark"`
	ApprovalEnabled       bool                                `json:"approval_enabled"`
	ApprovalMode          string                              `json:"approval_mode"`
	ApprovalApproverIDs   []string                            `json:"approval_approver_ids"`
	ApprovalApproverNames []string                            `json:"approval_approver_names"`
	CIParamDefIDs         []string                            `json:"ci_param_def_ids"`
	CDParamDefIDs         []string                            `json:"cd_param_def_ids"`
	CIParamConfigs        []ReleaseTemplateParamConfigRequest `json:"ci_param_configs"`
	CDParamConfigs        []ReleaseTemplateParamConfigRequest `json:"cd_param_configs"`
	GitOpsRules           []ReleaseTemplateGitOpsRuleRequest  `json:"gitops_rules"`
	Hooks                 []ReleaseTemplateHookRequest        `json:"hooks"`
}

type UpdateReleaseTemplateRequest struct {
	Name                  string                              `json:"name"`
	CIBindingID           string                              `json:"ci_binding_id"`
	CDBindingID           string                              `json:"cd_binding_id"`
	CDProvider            string                              `json:"cd_provider"`
	GitOpsType            string                              `json:"gitops_type"`
	Status                string                              `json:"status"`
	Remark                string                              `json:"remark"`
	ApprovalEnabled       bool                                `json:"approval_enabled"`
	ApprovalMode          string                              `json:"approval_mode"`
	ApprovalApproverIDs   []string                            `json:"approval_approver_ids"`
	ApprovalApproverNames []string                            `json:"approval_approver_names"`
	CIParamDefIDs         []string                            `json:"ci_param_def_ids"`
	CDParamDefIDs         []string                            `json:"cd_param_def_ids"`
	CIParamConfigs        []ReleaseTemplateParamConfigRequest `json:"ci_param_configs"`
	CDParamConfigs        []ReleaseTemplateParamConfigRequest `json:"cd_param_configs"`
	GitOpsRules           []ReleaseTemplateGitOpsRuleRequest  `json:"gitops_rules"`
	Hooks                 []ReleaseTemplateHookRequest        `json:"hooks"`
}

type ReleaseTemplateParamConfigRequest struct {
	ExecutorParamDefID string `json:"executor_param_def_id"`
	ValueSource        string `json:"value_source"`
	SourceParamKey     string `json:"source_param_key"`
	FixedValue         string `json:"fixed_value"`
}

type ReleaseTemplateResponse struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	ApplicationID         string    `json:"application_id"`
	ApplicationName       string    `json:"application_name"`
	BindingID             string    `json:"binding_id"`
	BindingName           string    `json:"binding_name"`
	BindingType           string    `json:"binding_type"`
	GitOpsType            string    `json:"gitops_type"`
	Status                string    `json:"status"`
	ApprovalEnabled       bool      `json:"approval_enabled"`
	ApprovalMode          string    `json:"approval_mode"`
	ApprovalApproverIDs   []string  `json:"approval_approver_ids"`
	ApprovalApproverNames []string  `json:"approval_approver_names"`
	Remark                string    `json:"remark"`
	ParamCount            int       `json:"param_count"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ReleaseTemplateParamResponse struct {
	ID                 string    `json:"id"`
	TemplateID         string    `json:"template_id"`
	TemplateBindingID  string    `json:"template_binding_id"`
	PipelineScope      string    `json:"pipeline_scope"`
	BindingID          string    `json:"binding_id"`
	ExecutorParamDefID string    `json:"executor_param_def_id"`
	ParamKey           string    `json:"param_key"`
	ParamName          string    `json:"param_name"`
	ExecutorParamName  string    `json:"executor_param_name"`
	ValueSource        string    `json:"value_source"`
	SourceParamKey     string    `json:"source_param_key"`
	SourceParamName    string    `json:"source_param_name"`
	FixedValue         string    `json:"fixed_value"`
	Required           bool      `json:"required"`
	SortNo             int       `json:"sort_no"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ReleaseTemplateBindingResponse struct {
	ID            string    `json:"id"`
	TemplateID    string    `json:"template_id"`
	PipelineScope string    `json:"pipeline_scope"`
	BindingID     string    `json:"binding_id"`
	BindingName   string    `json:"binding_name"`
	Provider      string    `json:"provider"`
	PipelineID    string    `json:"pipeline_id"`
	Enabled       bool      `json:"enabled"`
	SortNo        int       `json:"sort_no"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ReleaseTemplateGitOpsRuleRequest struct {
	SourceParamKey   string `json:"source_param_key"`
	SourceFrom       string `json:"source_from"`
	LocatorParamKey  string `json:"locator_param_key"`
	FilePathTemplate string `json:"file_path_template"`
	DocumentKind     string `json:"document_kind"`
	DocumentName     string `json:"document_name"`
	TargetPath       string `json:"target_path"`
	ValueTemplate    string `json:"value_template"`
}

type ReleaseTemplateGitOpsRuleResponse struct {
	ID               string    `json:"id"`
	TemplateID       string    `json:"template_id"`
	PipelineScope    string    `json:"pipeline_scope"`
	SourceParamKey   string    `json:"source_param_key"`
	SourceParamName  string    `json:"source_param_name"`
	SourceFrom       string    `json:"source_from"`
	LocatorParamKey  string    `json:"locator_param_key"`
	LocatorParamName string    `json:"locator_param_name"`
	FilePathTemplate string    `json:"file_path_template"`
	DocumentKind     string    `json:"document_kind"`
	DocumentName     string    `json:"document_name"`
	TargetPath       string    `json:"target_path"`
	ValueTemplate    string    `json:"value_template"`
	SortNo           int       `json:"sort_no"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReleaseTemplateHookRequest struct {
	HookType         string   `json:"hook_type"`
	Name             string   `json:"name"`
	ExecuteStage     string   `json:"execute_stage"`
	ExecuteStages    []string `json:"execute_stages"`
	TriggerCondition string   `json:"trigger_condition"`
	FailurePolicy    string   `json:"failure_policy"`
	EnvCodes         []string `json:"env_codes"`
	TargetID         string   `json:"target_id"`
	WebhookMethod    string   `json:"webhook_method"`
	WebhookURL       string   `json:"webhook_url"`
	WebhookBody      string   `json:"webhook_body"`
	Note             string   `json:"note"`
}

type ReleaseTemplateHookResponse struct {
	ID               string    `json:"id"`
	TemplateID       string    `json:"template_id"`
	HookType         string    `json:"hook_type"`
	Name             string    `json:"name"`
	ExecuteStage     string    `json:"execute_stage"`
	ExecuteStages    []string  `json:"execute_stages"`
	TriggerCondition string    `json:"trigger_condition"`
	FailurePolicy    string    `json:"failure_policy"`
	EnvCodes         []string  `json:"env_codes"`
	TargetID         string    `json:"target_id"`
	TargetName       string    `json:"target_name"`
	WebhookMethod    string    `json:"webhook_method"`
	WebhookURL       string    `json:"webhook_url"`
	WebhookBody      string    `json:"webhook_body"`
	Note             string    `json:"note"`
	SortNo           int       `json:"sort_no"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReleaseTemplateDataResponse struct {
	Data struct {
		Template    ReleaseTemplateResponse             `json:"template"`
		Bindings    []ReleaseTemplateBindingResponse    `json:"bindings"`
		Params      []ReleaseTemplateParamResponse      `json:"params"`
		GitOpsRules []ReleaseTemplateGitOpsRuleResponse `json:"gitops_rules"`
		Hooks       []ReleaseTemplateHookResponse       `json:"hooks"`
	} `json:"data"`
}

type ReleaseTemplateListResponse struct {
	Data     []ReleaseTemplateResponse `json:"data"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int64                     `json:"total"`
}

// Create 创建资源。
// @Summary      创建资源
// @Description  创建资源，并按统一响应结构返回处理结果。
// @Tags         release-templates
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-templates [post]
func (h *ReleaseTemplateHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "release.template.manage", "", "") {
		return
	}
	var req CreateReleaseTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	template, bindings, params, gitopsRules, hooks, err := h.manager.Create(c.Request.Context(), usecase.CreateReleaseTemplateInput{
		Name:                  req.Name,
		ApplicationID:         req.ApplicationID,
		CIBindingID:           req.CIBindingID,
		CDBindingID:           req.CDBindingID,
		CDProvider:            pipelinedomain.Provider(strings.ToLower(strings.TrimSpace(req.CDProvider))),
		GitOpsType:            releasedomain.GitOpsType(strings.ToLower(strings.TrimSpace(req.GitOpsType))),
		Status:                releasedomain.TemplateStatus(strings.TrimSpace(req.Status)),
		Remark:                req.Remark,
		ApprovalEnabled:       req.ApprovalEnabled,
		ApprovalMode:          releasedomain.TemplateApprovalMode(strings.ToLower(strings.TrimSpace(req.ApprovalMode))),
		ApprovalApproverIDs:   append([]string(nil), req.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), req.ApprovalApproverNames...),
		CIParamDefIDs:         req.CIParamDefIDs,
		CDParamDefIDs:         req.CDParamDefIDs,
		CIParamConfigs:        toReleaseTemplateParamConfigInputs(req.CIParamConfigs),
		CDParamConfigs:        toReleaseTemplateParamConfigInputs(req.CDParamConfigs),
		GitOpsRules:           toReleaseTemplateGitOpsRuleInputs(req.GitOpsRules),
		Hooks:                 toReleaseTemplateHookInputs(req.Hooks),
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toReleaseTemplateDataResponse(template, bindings, params, gitopsRules, hooks))
}

// List 查询资源列表。
// @Summary      查询资源列表
// @Description  查询资源列表，并按统一响应结构返回处理结果。
// @Tags         release-templates
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-templates [get]
func (h *ReleaseTemplateHandler) List(c *gin.Context) {
	allowAll, applicationIDs, ok := h.resolveListApplications(c)
	if !ok {
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
	items, total, err := h.manager.List(c.Request.Context(), usecase.ListReleaseTemplateInput{
		ApplicationID:  c.Query("application_id"),
		ApplicationIDs: resolveReleaseTemplateListFilterApplications(strings.TrimSpace(c.Query("application_id")), allowAll, applicationIDs),
		BindingID:      c.Query("binding_id"),
		Status:         releasedomain.TemplateStatus(strings.TrimSpace(c.Query("status"))),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseTemplateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseTemplateResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetByID 获取By ID详情。
// @Summary      获取By ID详情
// @Description  获取By ID详情，并按统一响应结构返回处理结果。
// @Tags         release-templates
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-templates/{id} [get]
func (h *ReleaseTemplateHandler) GetByID(c *gin.Context) {
	template, bindings, params, gitopsRules, hooks, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !h.ensureTemplateAccess(c, template) {
		return
	}
	c.JSON(http.StatusOK, toReleaseTemplateDataResponse(template, bindings, params, gitopsRules, hooks))
}

// Update 更新资源。
// @Summary      更新资源
// @Description  更新资源，并按统一响应结构返回处理结果。
// @Tags         release-templates
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-templates/{id} [put]
func (h *ReleaseTemplateHandler) Update(c *gin.Context) {
	template, _, _, _, _, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensurePermission(c, h.authz, "release.template.manage", "", "") {
		return
	}
	var req UpdateReleaseTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	updated, bindings, params, gitopsRules, hooks, err := h.manager.Update(c.Request.Context(), template.ID, usecase.UpdateReleaseTemplateInput{
		Name:                  req.Name,
		CIBindingID:           req.CIBindingID,
		CDBindingID:           req.CDBindingID,
		CDProvider:            pipelinedomain.Provider(strings.ToLower(strings.TrimSpace(req.CDProvider))),
		GitOpsType:            releasedomain.GitOpsType(strings.ToLower(strings.TrimSpace(req.GitOpsType))),
		Status:                releasedomain.TemplateStatus(strings.TrimSpace(req.Status)),
		Remark:                req.Remark,
		ApprovalEnabled:       req.ApprovalEnabled,
		ApprovalMode:          releasedomain.TemplateApprovalMode(strings.ToLower(strings.TrimSpace(req.ApprovalMode))),
		ApprovalApproverIDs:   append([]string(nil), req.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), req.ApprovalApproverNames...),
		CIParamDefIDs:         req.CIParamDefIDs,
		CDParamDefIDs:         req.CDParamDefIDs,
		CIParamConfigs:        toReleaseTemplateParamConfigInputs(req.CIParamConfigs),
		CDParamConfigs:        toReleaseTemplateParamConfigInputs(req.CDParamConfigs),
		GitOpsRules:           toReleaseTemplateGitOpsRuleInputs(req.GitOpsRules),
		Hooks:                 toReleaseTemplateHookInputs(req.Hooks),
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, toReleaseTemplateDataResponse(updated, bindings, params, gitopsRules, hooks))
}

// Delete 删除资源。
// @Summary      删除资源
// @Description  删除资源，并按统一响应结构返回处理结果。
// @Tags         release-templates
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-templates/{id} [delete]
func (h *ReleaseTemplateHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "release.template.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// resolveListApplications 解析上下文数据，得到后续流程需要的结果。
func (h *ReleaseTemplateHandler) resolveListApplications(c *gin.Context) (allowAll bool, applicationIDs []string, ok bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false, nil, false
	}
	if user.Role == userdomain.RoleAdmin {
		return true, nil, true
	}
	if h.authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false, nil, false
	}
	manageAllowed, err := h.authz.HasPermission(c.Request.Context(), user, "release.template.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	if manageAllowed {
		return true, nil, true
	}
	applicationID := strings.TrimSpace(c.Query("application_id"))
	if applicationID != "" {
		allowed, err := h.authz.HasPermission(c.Request.Context(), user, "release.create", "application", applicationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return false, nil, false
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
			return false, nil, false
		}
		return false, nil, true
	}

	items, err := h.authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, false
	}
	result, envScopes := collectApplicationScopesFromPermissions(items, map[string]struct{}{
		"release.create": {},
	})
	seen := make(map[string]struct{}, len(result)+len(envScopes))
	for _, item := range result {
		seen[item] = struct{}{}
	}
	for _, item := range envScopes {
		value := strings.TrimSpace(item.ApplicationID)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return false, result, true
}

// ensureTemplateAccess 校验前置条件，不满足时写入对应错误响应。
func (h *ReleaseTemplateHandler) ensureTemplateAccess(c *gin.Context, template releasedomain.ReleaseTemplate) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if user.Role == userdomain.RoleAdmin {
		return true
	}
	manageAllowed, err := h.authz.HasPermission(c.Request.Context(), user, "release.template.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if manageAllowed {
		return true
	}
	createAllowed, err := h.authz.HasPermission(c.Request.Context(), user, "release.create", "application", template.ApplicationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if !createAllowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return false
	}
	return true
}

// resolveReleaseTemplateListFilterApplications 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseTemplateListFilterApplications(
	applicationID string,
	allowAll bool,
	visibleApplicationIDs []string,
) []string {
	if allowAll || strings.TrimSpace(applicationID) != "" {
		return nil
	}
	if len(visibleApplicationIDs) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(visibleApplicationIDs))
	for _, item := range visibleApplicationIDs {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

// toReleaseTemplateResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateResponse(item releasedomain.ReleaseTemplate) ReleaseTemplateResponse {
	return ReleaseTemplateResponse{
		ID:                    item.ID,
		Name:                  item.Name,
		ApplicationID:         item.ApplicationID,
		ApplicationName:       item.ApplicationName,
		BindingID:             item.BindingID,
		BindingName:           item.BindingName,
		BindingType:           item.BindingType,
		GitOpsType:            string(item.GitOpsType),
		Status:                string(item.Status),
		ApprovalEnabled:       item.ApprovalEnabled,
		ApprovalMode:          string(item.ApprovalMode),
		ApprovalApproverIDs:   append([]string(nil), item.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), item.ApprovalApproverNames...),
		Remark:                item.Remark,
		ParamCount:            item.ParamCount,
		CreatedAt:             item.CreatedAt,
		UpdatedAt:             item.UpdatedAt,
	}
}

// toReleaseTemplateParamResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateParamResponse(item releasedomain.ReleaseTemplateParam) ReleaseTemplateParamResponse {
	return ReleaseTemplateParamResponse{
		ID:                 item.ID,
		TemplateID:         item.TemplateID,
		TemplateBindingID:  item.TemplateBindingID,
		PipelineScope:      string(item.PipelineScope),
		BindingID:          item.BindingID,
		ExecutorParamDefID: item.ExecutorParamDefID,
		ParamKey:           item.ParamKey,
		ParamName:          item.ParamName,
		ExecutorParamName:  item.ExecutorParamName,
		ValueSource:        string(item.ValueSource),
		SourceParamKey:     item.SourceParamKey,
		SourceParamName:    item.SourceParamName,
		FixedValue:         item.FixedValue,
		Required:           item.Required,
		SortNo:             item.SortNo,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

// toReleaseTemplateBindingResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateBindingResponse(item releasedomain.ReleaseTemplateBinding) ReleaseTemplateBindingResponse {
	return ReleaseTemplateBindingResponse{
		ID:            item.ID,
		TemplateID:    item.TemplateID,
		PipelineScope: string(item.PipelineScope),
		BindingID:     item.BindingID,
		BindingName:   item.BindingName,
		Provider:      item.Provider,
		PipelineID:    item.PipelineID,
		Enabled:       item.Enabled,
		SortNo:        item.SortNo,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

// toReleaseTemplateGitOpsRuleInputs 将领域对象转换为接口响应结构。
func toReleaseTemplateGitOpsRuleInputs(items []ReleaseTemplateGitOpsRuleRequest) []usecase.ReleaseTemplateGitOpsRuleInput {
	result := make([]usecase.ReleaseTemplateGitOpsRuleInput, 0, len(items))
	for _, item := range items {
		result = append(result, usecase.ReleaseTemplateGitOpsRuleInput{
			SourceParamKey:   item.SourceParamKey,
			SourceFrom:       releasedomain.GitOpsRuleSourceFrom(strings.ToLower(strings.TrimSpace(item.SourceFrom))),
			LocatorParamKey:  item.LocatorParamKey,
			FilePathTemplate: item.FilePathTemplate,
			DocumentKind:     item.DocumentKind,
			DocumentName:     item.DocumentName,
			TargetPath:       item.TargetPath,
			ValueTemplate:    item.ValueTemplate,
		})
	}
	return result
}

// toReleaseTemplateParamConfigInputs 将领域对象转换为接口响应结构。
func toReleaseTemplateParamConfigInputs(items []ReleaseTemplateParamConfigRequest) []usecase.ReleaseTemplateParamConfigInput {
	result := make([]usecase.ReleaseTemplateParamConfigInput, 0, len(items))
	for _, item := range items {
		result = append(result, usecase.ReleaseTemplateParamConfigInput{
			ExecutorParamDefID: item.ExecutorParamDefID,
			ValueSource:        releasedomain.TemplateParamValueSource(strings.ToLower(strings.TrimSpace(item.ValueSource))),
			SourceParamKey:     item.SourceParamKey,
			FixedValue:         item.FixedValue,
		})
	}
	return result
}

// toReleaseTemplateHookInputs 将领域对象转换为接口响应结构。
func toReleaseTemplateHookInputs(items []ReleaseTemplateHookRequest) []usecase.ReleaseTemplateHookInput {
	result := make([]usecase.ReleaseTemplateHookInput, 0, len(items))
	for _, item := range items {
		result = append(result, usecase.ReleaseTemplateHookInput{
			HookType:         releasedomain.TemplateHookType(strings.ToLower(strings.TrimSpace(item.HookType))),
			Name:             item.Name,
			ExecuteStage:     releasedomain.TemplateHookExecuteStage(strings.ToLower(strings.TrimSpace(item.ExecuteStage))),
			ExecuteStages:    normalizeReleaseTemplateHookExecuteStages(item.ExecuteStages),
			TriggerCondition: releasedomain.TemplateHookTriggerCondition(strings.ToLower(strings.TrimSpace(item.TriggerCondition))),
			FailurePolicy:    releasedomain.TemplateHookFailurePolicy(strings.ToLower(strings.TrimSpace(item.FailurePolicy))),
			EnvCodes:         append([]string(nil), item.EnvCodes...),
			TargetID:         item.TargetID,
			WebhookMethod:    item.WebhookMethod,
			WebhookURL:       item.WebhookURL,
			WebhookBody:      item.WebhookBody,
			Note:             item.Note,
		})
	}
	return result
}

// toReleaseTemplateGitOpsRuleResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateGitOpsRuleResponse(item releasedomain.ReleaseTemplateGitOpsRule) ReleaseTemplateGitOpsRuleResponse {
	return ReleaseTemplateGitOpsRuleResponse{
		ID:               item.ID,
		TemplateID:       item.TemplateID,
		PipelineScope:    string(item.PipelineScope),
		SourceParamKey:   item.SourceParamKey,
		SourceParamName:  item.SourceParamName,
		SourceFrom:       string(item.SourceFrom),
		LocatorParamKey:  item.LocatorParamKey,
		LocatorParamName: item.LocatorParamName,
		FilePathTemplate: normalizeReleaseTemplateGitOpsFilePathTemplate(item.FilePathTemplate),
		DocumentKind:     item.DocumentKind,
		DocumentName:     item.DocumentName,
		TargetPath:       item.TargetPath,
		ValueTemplate:    item.ValueTemplate,
		SortNo:           item.SortNo,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

// normalizeReleaseTemplateGitOpsFilePathTemplate 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseTemplateGitOpsFilePathTemplate(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || !strings.HasPrefix(value, "apps/") {
		return value
	}
	parts := strings.Split(strings.TrimPrefix(value, "apps/"), "/")
	if len(parts) < 3 {
		return value
	}
	if strings.EqualFold(parts[0], "helm") {
		return value
	}
	if strings.EqualFold(parts[1], "helm") {
		return filepath.ToSlash(filepath.Join("apps", "helm", filepath.Join(parts[2:]...)))
	}
	return value
}

// toReleaseTemplateHookResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateHookResponse(item releasedomain.ReleaseTemplateHook) ReleaseTemplateHookResponse {
	return ReleaseTemplateHookResponse{
		ID:               item.ID,
		TemplateID:       item.TemplateID,
		HookType:         string(item.HookType),
		Name:             item.Name,
		ExecuteStage:     string(releasedomain.PrimaryTemplateHookExecuteStage(item.ExecuteStages, item.ExecuteStage)),
		ExecuteStages:    releaseTemplateHookExecuteStagesToStrings(item.ExecuteStages, item.ExecuteStage),
		TriggerCondition: string(item.TriggerCondition),
		FailurePolicy:    string(item.FailurePolicy),
		EnvCodes:         append([]string(nil), item.EnvCodes...),
		TargetID:         item.TargetID,
		TargetName:       item.TargetName,
		WebhookMethod:    item.WebhookMethod,
		WebhookURL:       item.WebhookURL,
		WebhookBody:      item.WebhookBody,
		Note:             item.Note,
		SortNo:           item.SortNo,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

// normalizeReleaseTemplateHookExecuteStages 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseTemplateHookExecuteStages(values []string) []releasedomain.TemplateHookExecuteStage {
	result := make([]releasedomain.TemplateHookExecuteStage, 0, len(values))
	for _, item := range values {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		result = append(result, releasedomain.TemplateHookExecuteStage(value))
	}
	return result
}

// releaseTemplateHookExecuteStagesToStrings 封装当前模块的业务处理逻辑。
func releaseTemplateHookExecuteStagesToStrings(values []releasedomain.TemplateHookExecuteStage, legacy releasedomain.TemplateHookExecuteStage) []string {
	stages := releasedomain.NormalizeTemplateHookExecuteStages(values, legacy)
	result := make([]string, 0, len(stages))
	for _, item := range stages {
		result = append(result, string(item))
	}
	return result
}

// toReleaseTemplateDataResponse 将领域对象转换为接口响应结构。
func toReleaseTemplateDataResponse(
	template releasedomain.ReleaseTemplate,
	bindings []releasedomain.ReleaseTemplateBinding,
	params []releasedomain.ReleaseTemplateParam,
	gitopsRules []releasedomain.ReleaseTemplateGitOpsRule,
	hooks []releasedomain.ReleaseTemplateHook,
) ReleaseTemplateDataResponse {
	resp := ReleaseTemplateDataResponse{}
	resp.Data.Template = toReleaseTemplateResponse(template)
	resp.Data.Bindings = make([]ReleaseTemplateBindingResponse, 0, len(bindings))
	for _, item := range bindings {
		resp.Data.Bindings = append(resp.Data.Bindings, toReleaseTemplateBindingResponse(item))
	}
	resp.Data.Params = make([]ReleaseTemplateParamResponse, 0, len(params))
	for _, item := range params {
		resp.Data.Params = append(resp.Data.Params, toReleaseTemplateParamResponse(item))
	}
	resp.Data.GitOpsRules = make([]ReleaseTemplateGitOpsRuleResponse, 0, len(gitopsRules))
	for _, item := range gitopsRules {
		resp.Data.GitOpsRules = append(resp.Data.GitOpsRules, toReleaseTemplateGitOpsRuleResponse(item))
	}
	resp.Data.Hooks = make([]ReleaseTemplateHookResponse, 0, len(hooks))
	for _, item := range hooks {
		resp.Data.Hooks = append(resp.Data.Hooks, toReleaseTemplateHookResponse(item))
	}
	return resp
}
