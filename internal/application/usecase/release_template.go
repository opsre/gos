package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
	appdomain "gos/internal/domain/application"
	argocddomain "gos/internal/domain/argocdapp"
	pipelineparamdomain "gos/internal/domain/executorparam"
	gitopsdomain "gos/internal/domain/gitops"
	notificationdomain "gos/internal/domain/notification"
	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
	platformparamdomain "gos/internal/domain/platformparam"
	releasedomain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

var templateBuiltinSourceKeys = map[string]struct{}{
	"app_key":       {},
	"app_name":      {},
	"release_name":  {},
	"env":           {},
	"env_code":      {},
	"branch":        {},
	"git_ref":       {},
	"image_version": {},
	"image_tag":     {},
}

type ReleaseTemplateManager struct {
	repo             releasedomain.Repository
	appRepo          appdomain.Repository
	pipelineRepo     pipelinedomain.Repository
	paramRepo        pipelineparamdomain.Repository
	platformRepo     platformparamdomain.Repository
	argocdRepo       argocddomain.Repository
	agentRepo        agentdomain.Repository
	notificationRepo notificationdomain.Repository
	pipelineScanRepo scandomain.Repository
	gitopsReader     ReleaseTemplateGitOpsFieldCandidateReader
	now              func() time.Time
}

type ReleaseTemplateGitOpsFieldCandidateReader interface {
	ListFieldCandidates(ctx context.Context, appKey string) ([]gitopsdomain.FieldCandidate, error)
	ListValuesCandidates(ctx context.Context, appKey string) ([]gitopsdomain.ValuesCandidate, error)
}

type ReleaseTemplateGitOpsRuleInput struct {
	SourceParamKey   string
	SourceFrom       releasedomain.GitOpsRuleSourceFrom
	LocatorParamKey  string
	FilePathTemplate string
	DocumentKind     string
	DocumentName     string
	TargetPath       string
	ValueTemplate    string
}

type gitOpsValuesTargetSelection struct {
	FilePathTemplate string `json:"file_path_template"`
	TargetPath       string `json:"target_path"`
}

type CreateReleaseTemplateInput struct {
	Name                  string
	ApplicationID         string
	CIBindingID           string
	CDBindingID           string
	CDProvider            pipelinedomain.Provider
	GitOpsType            releasedomain.GitOpsType
	Status                releasedomain.TemplateStatus
	Remark                string
	ApprovalEnabled       bool
	ApprovalMode          releasedomain.TemplateApprovalMode
	ApprovalApproverIDs   []string
	ApprovalApproverNames []string
	CIParamDefIDs         []string
	CDParamDefIDs         []string
	CIParamConfigs        []ReleaseTemplateParamConfigInput
	CDParamConfigs        []ReleaseTemplateParamConfigInput
	GitOpsRules           []ReleaseTemplateGitOpsRuleInput
	Hooks                 []ReleaseTemplateHookInput
}

type UpdateReleaseTemplateInput struct {
	Name                  string
	CIBindingID           string
	CDBindingID           string
	CDProvider            pipelinedomain.Provider
	GitOpsType            releasedomain.GitOpsType
	Status                releasedomain.TemplateStatus
	Remark                string
	ApprovalEnabled       bool
	ApprovalMode          releasedomain.TemplateApprovalMode
	ApprovalApproverIDs   []string
	ApprovalApproverNames []string
	CIParamDefIDs         []string
	CDParamDefIDs         []string
	CIParamConfigs        []ReleaseTemplateParamConfigInput
	CDParamConfigs        []ReleaseTemplateParamConfigInput
	GitOpsRules           []ReleaseTemplateGitOpsRuleInput
	Hooks                 []ReleaseTemplateHookInput
}

type ReleaseTemplateParamConfigInput struct {
	ExecutorParamDefID string
	ValueSource        releasedomain.TemplateParamValueSource
	SourceParamKey     string
	FixedValue         string
}

type ReleaseTemplateHookInput struct {
	HookType         releasedomain.TemplateHookType
	Name             string
	ExecuteStage     releasedomain.TemplateHookExecuteStage
	ExecuteStages    []releasedomain.TemplateHookExecuteStage
	TriggerCondition releasedomain.TemplateHookTriggerCondition
	FailurePolicy    releasedomain.TemplateHookFailurePolicy
	EnvCodes         []string
	TargetID         string
	WebhookMethod    string
	WebhookURL       string
	WebhookBody      string
	Note             string
}

type ListReleaseTemplateInput struct {
	ApplicationID  string
	ApplicationIDs []string
	BindingID      string
	Status         releasedomain.TemplateStatus
	Page           int
	PageSize       int
}

type normalizedTemplateApprovalConfig struct {
	Enabled       bool
	Mode          releasedomain.TemplateApprovalMode
	ApproverIDs   []string
	ApproverNames []string
}

// NewReleaseTemplateManager 创建并返回对应组件实例。
func NewReleaseTemplateManager(
	repo releasedomain.Repository,
	appRepo appdomain.Repository,
	pipelineRepo pipelinedomain.Repository,
	paramRepo pipelineparamdomain.Repository,
	platformRepo platformparamdomain.Repository,
	argocdRepo argocddomain.Repository,
	agentRepo agentdomain.Repository,
	notificationRepo notificationdomain.Repository,
	gitopsReader ReleaseTemplateGitOpsFieldCandidateReader,
) *ReleaseTemplateManager {
	return &ReleaseTemplateManager{
		repo:             repo,
		appRepo:          appRepo,
		pipelineRepo:     pipelineRepo,
		paramRepo:        paramRepo,
		platformRepo:     platformRepo,
		argocdRepo:       argocdRepo,
		agentRepo:        agentRepo,
		notificationRepo: notificationRepo,
		gitopsReader:     gitopsReader,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (uc *ReleaseTemplateManager) SetPipelineScanRepository(repo scandomain.Repository) {
	if uc == nil {
		return
	}
	uc.pipelineScanRepo = repo
}

// Create 创建业务资源并返回处理结果。
func (uc *ReleaseTemplateManager) Create(
	ctx context.Context,
	input CreateReleaseTemplateInput,
) (releasedomain.ReleaseTemplate, []releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, error) {
	name := strings.TrimSpace(input.Name)
	applicationID := strings.TrimSpace(input.ApplicationID)
	logx.Info("release_template", "create_start",
		logx.F("name", name),
		logx.F("application_id", applicationID),
		logx.F("ci_binding_id", input.CIBindingID),
		logx.F("cd_binding_id", input.CDBindingID),
		logx.F("cd_provider", input.CDProvider),
		logx.F("gitops_type", input.GitOpsType),
	)
	if name == "" || applicationID == "" {
		err := fmt.Errorf("%w: name and application_id are required", ErrInvalidInput)
		logx.Error("release_template", "create_failed", err,
			logx.F("name", name),
			logx.F("application_id", applicationID),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}

	status := input.Status
	if status == "" {
		status = releasedomain.TemplateStatusActive
	}
	if !status.Valid() {
		logx.Error("release_template", "create_failed", ErrInvalidStatus,
			logx.F("name", name),
			logx.F("application_id", applicationID),
			logx.F("status", status),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, ErrInvalidStatus
	}
	// 审批策略已迁移至应用级发布流程；模板不再承载审批配置。
	approvalConfig := normalizedTemplateApprovalConfig{}

	templateBindings, params, gitopsRules, hooks, appName, err := uc.buildTemplatePayload(
		ctx,
		applicationID,
		input.CIBindingID,
		input.CDBindingID,
		input.CDProvider,
		input.GitOpsType,
		input.CIParamDefIDs,
		input.CDParamDefIDs,
		input.CIParamConfigs,
		input.CDParamConfigs,
		input.GitOpsRules,
		input.Hooks,
		nil,
	)
	if err != nil {
		logx.Error("release_template", "create_failed", err,
			logx.F("name", name),
			logx.F("application_id", applicationID),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}

	now := uc.now()
	summaryName, summaryType := summarizeTemplateBindings(templateBindings)
	template := releasedomain.ReleaseTemplate{
		ID:                    generateID("rt"),
		Name:                  name,
		ApplicationID:         applicationID,
		ApplicationName:       appName,
		BindingID:             applicationID,
		BindingName:           summaryName,
		BindingType:           summaryType,
		GitOpsType:            normalizeTemplateGitOpsType(input.GitOpsType, templateUsesArgoCD(templateBindings)),
		Status:                status,
		ApprovalEnabled:       approvalConfig.Enabled,
		ApprovalMode:          approvalConfig.Mode,
		ApprovalApproverIDs:   append([]string(nil), approvalConfig.ApproverIDs...),
		ApprovalApproverNames: append([]string(nil), approvalConfig.ApproverNames...),
		Remark:                strings.TrimSpace(input.Remark),
		ParamCount:            len(params),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	for idx := range templateBindings {
		templateBindings[idx].TemplateID = template.ID
		templateBindings[idx].CreatedAt = now
		templateBindings[idx].UpdatedAt = now
	}
	for idx := range params {
		params[idx].TemplateID = template.ID
		params[idx].CreatedAt = now
		params[idx].UpdatedAt = now
	}
	for idx := range gitopsRules {
		gitopsRules[idx].TemplateID = template.ID
		gitopsRules[idx].CreatedAt = now
		gitopsRules[idx].UpdatedAt = now
	}
	for idx := range hooks {
		hooks[idx].TemplateID = template.ID
		hooks[idx].CreatedAt = now
		hooks[idx].UpdatedAt = now
	}

	if err := uc.repo.CreateTemplate(ctx, template, templateBindings, params, gitopsRules, hooks); err != nil {
		logx.Error("release_template", "create_failed", err,
			logx.F("template_id", template.ID),
			logx.F("name", template.Name),
			logx.F("application_id", template.ApplicationID),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}
	logx.Info("release_template", "create_success",
		logx.F("template_id", template.ID),
		logx.F("name", template.Name),
		logx.F("application_id", template.ApplicationID),
		logx.F("bindings_count", len(templateBindings)),
		logx.F("params_count", len(params)),
		logx.F("gitops_rules_count", len(gitopsRules)),
	)
	return uc.GetByID(ctx, template.ID)
}

// GetByID 查询并返回指定资源数据。
func (uc *ReleaseTemplateManager) GetByID(
	ctx context.Context,
	id string,
) (releasedomain.ReleaseTemplate, []releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, ErrInvalidID
	}
	template, bindings, params, gitopsRules, hooks, err := uc.repo.GetTemplateByID(ctx, id)
	if err != nil {
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}
	uc.applyTemplateCompliance(ctx, &template, bindings)
	return template, bindings, params, gitopsRules, hooks, nil
}

// List 查询并返回列表数据。
func (uc *ReleaseTemplateManager) List(
	ctx context.Context,
	input ListReleaseTemplateInput,
) ([]releasedomain.ReleaseTemplate, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter := releasedomain.TemplateListFilter{
		ApplicationID:  strings.TrimSpace(input.ApplicationID),
		ApplicationIDs: append([]string(nil), input.ApplicationIDs...),
		BindingID:      strings.TrimSpace(input.BindingID),
		Status:         input.Status,
		Page:           input.Page,
		PageSize:       input.PageSize,
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize <= 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}
	items, total, err := uc.repo.ListTemplates(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for idx := range items {
		_, bindings, _, _, _, detailErr := uc.repo.GetTemplateByID(ctx, items[idx].ID)
		if detailErr != nil {
			items[idx].ComplianceStatus = releasedomain.TemplateComplianceStatusUnknown
			items[idx].ComplianceSummary = "模板规范状态暂不可用"
			continue
		}
		uc.applyTemplateCompliance(ctx, &items[idx], bindings)
	}
	return items, total, nil
}

// Update 更新业务资源并返回处理结果。
func (uc *ReleaseTemplateManager) Update(
	ctx context.Context,
	id string,
	input UpdateReleaseTemplateInput,
) (releasedomain.ReleaseTemplate, []releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, error) {
	id = strings.TrimSpace(id)
	logx.Info("release_template", "update_start",
		logx.F("template_id", id),
		logx.F("ci_binding_id", input.CIBindingID),
		logx.F("cd_binding_id", input.CDBindingID),
		logx.F("cd_provider", input.CDProvider),
		logx.F("gitops_type", input.GitOpsType),
	)
	if id == "" {
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, ErrInvalidID
	}
	current, _, _, currentGitOpsRules, _, err := uc.repo.GetTemplateByID(ctx, id)
	if err != nil {
		logx.Error("release_template", "update_failed", err, logx.F("template_id", id))
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = current.Name
	}

	status := input.Status
	if status == "" {
		status = current.Status
	}
	if !status.Valid() {
		logx.Error("release_template", "update_failed", ErrInvalidStatus,
			logx.F("template_id", id),
			logx.F("status", status),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, ErrInvalidStatus
	}
	// 审批策略已迁移至应用级发布流程；模板不再承载审批配置。
	approvalConfig := normalizedTemplateApprovalConfig{}

	templateBindings, params, gitopsRules, hooks, appName, err := uc.buildTemplatePayload(
		ctx,
		current.ApplicationID,
		input.CIBindingID,
		input.CDBindingID,
		input.CDProvider,
		input.GitOpsType,
		input.CIParamDefIDs,
		input.CDParamDefIDs,
		input.CIParamConfigs,
		input.CDParamConfigs,
		input.GitOpsRules,
		input.Hooks,
		currentGitOpsRules,
	)
	if err != nil {
		logx.Error("release_template", "update_failed", err,
			logx.F("template_id", id),
			logx.F("application_id", current.ApplicationID),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}

	now := uc.now()
	summaryName, summaryType := summarizeTemplateBindings(templateBindings)
	template := releasedomain.ReleaseTemplate{
		ID:                    current.ID,
		Name:                  name,
		ApplicationID:         current.ApplicationID,
		ApplicationName:       appName,
		BindingID:             current.ApplicationID,
		BindingName:           summaryName,
		BindingType:           summaryType,
		GitOpsType:            normalizeTemplateGitOpsType(input.GitOpsType, templateUsesArgoCD(templateBindings)),
		Status:                status,
		ApprovalEnabled:       approvalConfig.Enabled,
		ApprovalMode:          approvalConfig.Mode,
		ApprovalApproverIDs:   append([]string(nil), approvalConfig.ApproverIDs...),
		ApprovalApproverNames: append([]string(nil), approvalConfig.ApproverNames...),
		Remark:                strings.TrimSpace(input.Remark),
		ParamCount:            len(params),
		CreatedAt:             current.CreatedAt,
		UpdatedAt:             now,
	}
	for idx := range templateBindings {
		templateBindings[idx].TemplateID = template.ID
		templateBindings[idx].CreatedAt = now
		templateBindings[idx].UpdatedAt = now
	}
	for idx := range params {
		params[idx].TemplateID = template.ID
		params[idx].CreatedAt = now
		params[idx].UpdatedAt = now
	}
	for idx := range gitopsRules {
		gitopsRules[idx].TemplateID = template.ID
		gitopsRules[idx].CreatedAt = now
		gitopsRules[idx].UpdatedAt = now
	}
	for idx := range hooks {
		hooks[idx].TemplateID = template.ID
		hooks[idx].CreatedAt = now
		hooks[idx].UpdatedAt = now
	}

	if err := uc.repo.UpdateTemplate(ctx, template, templateBindings, params, gitopsRules, hooks); err != nil {
		logx.Error("release_template", "update_failed", err,
			logx.F("template_id", template.ID),
			logx.F("application_id", template.ApplicationID),
		)
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, err
	}
	logx.Info("release_template", "update_success",
		logx.F("template_id", template.ID),
		logx.F("name", template.Name),
		logx.F("application_id", template.ApplicationID),
		logx.F("bindings_count", len(templateBindings)),
		logx.F("params_count", len(params)),
		logx.F("gitops_rules_count", len(gitopsRules)),
	)
	return uc.GetByID(ctx, template.ID)
}

func (uc *ReleaseTemplateManager) applyTemplateCompliance(
	ctx context.Context,
	template *releasedomain.ReleaseTemplate,
	bindings []releasedomain.ReleaseTemplateBinding,
) {
	if template == nil {
		return
	}
	result := evaluateReleaseTemplateCompliance(ctx, uc.pipelineScanRepo, bindings)
	template.ComplianceStatus = result.Status
	template.ComplianceSummary = result.Summary
	template.ComplianceFindings = append([]releasedomain.ReleaseTemplateComplianceFinding(nil), result.Findings...)
}

// Delete 删除业务资源并返回处理结果。
func (uc *ReleaseTemplateManager) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	logx.Info("release_template", "delete_start", logx.F("template_id", id))
	if err := uc.repo.DeleteTemplate(ctx, id); err != nil {
		logx.Error("release_template", "delete_failed", err, logx.F("template_id", id))
		return err
	}
	logx.Info("release_template", "delete_success", logx.F("template_id", id))
	return nil
}

// normalizeTemplateApprovalConfig 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTemplateApprovalConfig(
	enabled bool,
	mode releasedomain.TemplateApprovalMode,
	approverIDs []string,
	approverNames []string,
) (normalizedTemplateApprovalConfig, error) {
	if !enabled {
		return normalizedTemplateApprovalConfig{
			Enabled:       false,
			Mode:          "",
			ApproverIDs:   []string{},
			ApproverNames: []string{},
		}, nil
	}
	if mode == "" {
		mode = releasedomain.TemplateApprovalModeAny
	}
	if !mode.Valid() {
		return normalizedTemplateApprovalConfig{}, fmt.Errorf("%w: invalid approval_mode", ErrInvalidInput)
	}
	normalizedIDs := make([]string, 0, len(approverIDs))
	normalizedNames := make([]string, 0, len(approverIDs))
	seen := make(map[string]struct{}, len(approverIDs))
	for idx, rawID := range approverIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		name := id
		if idx < len(approverNames) {
			if candidate := strings.TrimSpace(approverNames[idx]); candidate != "" {
				name = candidate
			}
		}
		normalizedIDs = append(normalizedIDs, id)
		normalizedNames = append(normalizedNames, name)
	}
	if len(normalizedIDs) == 0 {
		return normalizedTemplateApprovalConfig{}, fmt.Errorf("%w: approval approvers are required", ErrInvalidInput)
	}
	return normalizedTemplateApprovalConfig{
		Enabled:       true,
		Mode:          mode,
		ApproverIDs:   normalizedIDs,
		ApproverNames: normalizedNames,
	}, nil
}

// buildTemplatePayload 组装业务执行所需的输入数据。
func (uc *ReleaseTemplateManager) buildTemplatePayload(
	ctx context.Context,
	applicationID string,
	ciBindingID string,
	cdBindingID string,
	cdProvider pipelinedomain.Provider,
	gitopsType releasedomain.GitOpsType,
	ciParamDefIDs []string,
	cdParamDefIDs []string,
	ciParamConfigs []ReleaseTemplateParamConfigInput,
	cdParamConfigs []ReleaseTemplateParamConfigInput,
	gitopsRuleInputs []ReleaseTemplateGitOpsRuleInput,
	hookInputs []ReleaseTemplateHookInput,
	existingGitOpsRules []releasedomain.ReleaseTemplateGitOpsRule,
) ([]releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, string, error) {
	bindings := make([]releasedomain.ReleaseTemplateBinding, 0, 2)
	params := make([]releasedomain.ReleaseTemplateParam, 0)

	appName := ""
	if uc.pipelineRepo == nil {
		return nil, nil, nil, nil, "", fmt.Errorf("%w: pipeline repository is not configured", ErrInvalidInput)
	}

	ciBinding, ciParams, appName, err := uc.buildTemplateScopePayload(
		ctx,
		applicationID,
		releasedomain.PipelineScopeCI,
		ciBindingID,
		"",
		ciParamDefIDs,
		ciParamConfigs,
		nil,
		1,
	)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	if ciBinding != nil {
		bindings = append(bindings, *ciBinding)
		params = append(params, ciParams...)
	}

	cdBinding, cdParams, derivedAppName, err := uc.buildTemplateScopePayload(
		ctx,
		applicationID,
		releasedomain.PipelineScopeCD,
		cdBindingID,
		cdProvider,
		cdParamDefIDs,
		cdParamConfigs,
		ciParams,
		2,
	)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	if appName == "" {
		appName = derivedAppName
	}
	if cdBinding != nil {
		bindings = append(bindings, *cdBinding)
		params = append(params, cdParams...)
	}

	if len(bindings) == 0 {
		return nil, nil, nil, nil, "", fmt.Errorf("%w: at least one of ci/cd must be enabled", ErrInvalidInput)
	}
	gitopsType = normalizeTemplateGitOpsType(gitopsType, templateUsesArgoCD(bindings))
	if err := uc.validateArgoCDTemplateConfig(ctx, bindings, params, gitopsType); err != nil {
		return nil, nil, nil, nil, "", err
	}
	gitopsRules, err := uc.buildGitOpsRules(ctx, applicationID, bindings, params, gitopsType, gitopsRuleInputs, existingGitOpsRules)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	hooks, err := uc.buildTemplateHooks(ctx, hookInputs)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	if !templateUsesScope(bindings, releasedomain.PipelineScopeCI) {
		for _, item := range hooks {
			if releasedomain.TemplateHookHasExecuteStage(item.ExecuteStages, item.ExecuteStage, releasedomain.TemplateHookExecuteStageBuildComplete) {
				return nil, nil, nil, nil, "", fmt.Errorf("%w: 构建完成 Hook 需要启用 CI 执行单元", ErrInvalidInput)
			}
		}
	}
	if appName == "" && len(bindings) > 0 {
		appName = bindings[0].BindingName
	}
	return bindings, params, gitopsRules, hooks, appName, nil
}

// buildTemplateScopePayload 组装业务执行所需的输入数据。
func (uc *ReleaseTemplateManager) buildTemplateScopePayload(
	ctx context.Context,
	applicationID string,
	scope releasedomain.PipelineScope,
	bindingID string,
	desiredProvider pipelinedomain.Provider,
	paramDefIDs []string,
	paramConfigs []ReleaseTemplateParamConfigInput,
	availableCIParams []releasedomain.ReleaseTemplateParam,
	sortNo int,
) (*releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, string, error) {
	bindingID = strings.TrimSpace(bindingID)
	normalizedConfigs := normalizeTemplateParamConfigInputs(paramDefIDs, paramConfigs)
	if bindingID == "" {
		if scope == releasedomain.PipelineScopeCD && desiredProvider == pipelinedomain.ProviderArgoCD {
			if len(normalizedConfigs) > 0 {
				return nil, nil, "", fmt.Errorf("%w: argocd cd 暂不支持额外执行器参数", ErrInvalidInput)
			}
			if uc.appRepo == nil {
				return nil, nil, "", fmt.Errorf("%w: application repository is not configured", ErrInvalidInput)
			}
			app, err := uc.appRepo.GetByID(ctx, applicationID)
			if err != nil {
				return nil, nil, "", err
			}
			// ArgoCD 模式下，CD 执行器不再依赖单独的“管线绑定”记录；
			// 模板只要显式启用 CD 且未选择 Jenkins 绑定，就视为走 ArgoCD。
			return &releasedomain.ReleaseTemplateBinding{
				ID:            generateID("rtb"),
				PipelineScope: scope,
				BindingID:     "",
				BindingName:   "ArgoCD",
				Provider:      string(pipelinedomain.ProviderArgoCD),
				PipelineID:    "",
				Enabled:       true,
				SortNo:        sortNo,
			}, nil, app.Name, nil
		}
		if len(normalizedConfigs) > 0 {
			return nil, nil, "", fmt.Errorf("%w: %s binding is required", ErrInvalidInput, strings.ToUpper(string(scope)))
		}
		return nil, nil, "", nil
	}

	binding, err := uc.pipelineRepo.GetBindingByID(ctx, bindingID)
	if err != nil {
		return nil, nil, "", err
	}
	if strings.TrimSpace(binding.ApplicationID) != strings.TrimSpace(applicationID) {
		return nil, nil, "", fmt.Errorf("%w: binding does not belong to application", ErrInvalidInput)
	}
	if strings.TrimSpace(string(binding.BindingType)) != string(scope) {
		return nil, nil, "", fmt.Errorf("%w: binding scope does not match template scope", ErrInvalidInput)
	}
	if binding.Status != pipelinedomain.StatusActive {
		return nil, nil, "", fmt.Errorf("%w: selected binding is disabled", ErrInvalidInput)
	}
	if scope == releasedomain.PipelineScopeCI && binding.Provider != pipelinedomain.ProviderJenkins {
		return nil, nil, "", fmt.Errorf("%w: ci binding only supports jenkins", ErrInvalidInput)
	}
	if strings.TrimSpace(binding.PipelineID) != "" {
		pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, binding.PipelineID)
		if err != nil {
			return nil, nil, "", err
		}
		if err := ensureActivePipelineRecord(pipeline, "绑定管线"); err != nil {
			return nil, nil, "", err
		}
	}

	templateBinding := &releasedomain.ReleaseTemplateBinding{
		ID:            generateID("rtb"),
		PipelineScope: scope,
		BindingID:     binding.ID,
		BindingName:   strings.TrimSpace(binding.Name),
		Provider:      strings.TrimSpace(string(binding.Provider)),
		PipelineID:    strings.TrimSpace(binding.PipelineID),
		Enabled:       true,
		SortNo:        sortNo,
	}

	if len(normalizedConfigs) == 0 {
		return templateBinding, nil, binding.ApplicationName, nil
	}
	if binding.Provider != pipelinedomain.ProviderJenkins {
		return nil, nil, "", fmt.Errorf("%w: only jenkins binding supports template params", ErrInvalidInput)
	}

	builtinParams, err := uc.listBuiltinPlatformParamsForTemplate(ctx)
	if err != nil {
		return nil, nil, "", err
	}

	params := make([]releasedomain.ReleaseTemplateParam, 0, len(normalizedConfigs))
	ciSourceParams := make(map[string]releasedomain.ReleaseTemplateParam)
	for _, item := range availableCIParams {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		ciSourceParams[key] = item
	}
	for idx, config := range normalizedConfigs {
		paramDef, err := uc.paramRepo.GetByID(ctx, config.ExecutorParamDefID)
		if err != nil {
			return nil, nil, "", err
		}
		if err := ensureActiveExecutorParamDef(paramDef, "所选模板参数"); err != nil {
			return nil, nil, "", err
		}
		if strings.TrimSpace(paramDef.PipelineID) != strings.TrimSpace(binding.PipelineID) {
			return nil, nil, "", fmt.Errorf("%w: template param does not belong to selected binding", ErrInvalidInput)
		}
		paramKey := strings.ToLower(strings.TrimSpace(paramDef.ParamKey))
		if paramKey == "" {
			return nil, nil, "", fmt.Errorf("%w: template param must be mapped to platform key", ErrInvalidInput)
		}
		dict, err := uc.platformRepo.GetByParamKey(ctx, paramKey)
		if err != nil {
			return nil, nil, "", err
		}
		if dict.Status != platformparamdomain.StatusEnabled {
			return nil, nil, "", fmt.Errorf("%w: platform param dict is disabled", ErrInvalidInput)
		}
		valueSource := config.ValueSource
		if valueSource == "" {
			valueSource = releasedomain.TemplateParamValueSourceReleaseInput
		}
		if !valueSource.Valid() {
			return nil, nil, "", fmt.Errorf("%w: template param value_source is invalid", ErrInvalidInput)
		}
		switch scope {
		case releasedomain.PipelineScopeCI:
			if valueSource != releasedomain.TemplateParamValueSourceReleaseInput &&
				valueSource != releasedomain.TemplateParamValueSourceFixed &&
				valueSource != releasedomain.TemplateParamValueSourceBuiltin {
				return nil, nil, "", fmt.Errorf("%w: CI 参数仅支持发布时填写、固定值或发布基础字段", ErrInvalidInput)
			}
		case releasedomain.PipelineScopeCD:
			if valueSource != releasedomain.TemplateParamValueSourceReleaseInput &&
				valueSource != releasedomain.TemplateParamValueSourceFixed &&
				valueSource != releasedomain.TemplateParamValueSourceCIParam &&
				valueSource != releasedomain.TemplateParamValueSourceBuiltin {
				return nil, nil, "", fmt.Errorf("%w: CD 管线参数来源无效", ErrInvalidInput)
			}
		}

		sourceParamKey := strings.ToLower(strings.TrimSpace(config.SourceParamKey))
		sourceParamName := ""
		fixedValue := strings.TrimSpace(config.FixedValue)
		switch valueSource {
		case releasedomain.TemplateParamValueSourceFixed:
			if fixedValue == "" {
				return nil, nil, "", fmt.Errorf("%w: 固定值不能为空：%s", ErrInvalidInput, firstNonEmpty(strings.TrimSpace(dict.Name), paramKey))
			}
		case releasedomain.TemplateParamValueSourceCIParam:
			if sourceParamKey == "" {
				return nil, nil, "", fmt.Errorf("%w: 请选择来源 CI 标准字段：%s", ErrInvalidInput, firstNonEmpty(strings.TrimSpace(dict.Name), paramKey))
			}
			sourceParam, ok := ciSourceParams[sourceParamKey]
			if !ok {
				return nil, nil, "", fmt.Errorf("%w: CD 参数只能引用已选择的 CI 标准字段：%s", ErrInvalidInput, sourceParamKey)
			}
			sourceParamName = firstNonEmpty(sourceParam.ParamName, sourceParamKey)
		case releasedomain.TemplateParamValueSourceBuiltin:
			if sourceParamKey == "" {
				return nil, nil, "", fmt.Errorf("%w: 请选择来源发布基础字段：%s", ErrInvalidInput, firstNonEmpty(strings.TrimSpace(dict.Name), paramKey))
			}
			builtinDict, ok := builtinParams[sourceParamKey]
			if !ok {
				return nil, nil, "", fmt.Errorf("%w: 仅允许引用已启用的发布基础字段：%s", ErrInvalidInput, sourceParamKey)
			}
			sourceParamName = firstNonEmpty(strings.TrimSpace(builtinDict.Name), sourceParamKey)
		default:
			sourceParamKey = ""
		}

		templateParam := releasedomain.ReleaseTemplateParam{
			ID:                 generateID("rtp"),
			TemplateBindingID:  templateBinding.ID,
			PipelineScope:      scope,
			BindingID:          binding.ID,
			ExecutorParamDefID: paramDef.ID,
			ParamKey:           paramKey,
			ParamName:          strings.TrimSpace(dict.Name),
			ExecutorParamName:  strings.TrimSpace(paramDef.ExecutorParamName),
			ValueSource:        valueSource,
			SourceParamKey:     sourceParamKey,
			SourceParamName:    sourceParamName,
			FixedValue:         fixedValue,
			Required:           paramDef.Required,
			SortNo:             idx + 1,
		}
		params = append(params, templateParam)
		if scope == releasedomain.PipelineScopeCI {
			ciSourceParams[paramKey] = templateParam
		}
	}
	return templateBinding, params, binding.ApplicationName, nil
}

// normalizeTemplateParamConfigInputs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTemplateParamConfigInputs(
	paramDefIDs []string,
	configs []ReleaseTemplateParamConfigInput,
) []ReleaseTemplateParamConfigInput {
	if len(configs) > 0 {
		result := make([]ReleaseTemplateParamConfigInput, 0, len(configs))
		seen := make(map[string]struct{}, len(configs))
		for _, item := range configs {
			id := strings.TrimSpace(item.ExecutorParamDefID)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, ReleaseTemplateParamConfigInput{
				ExecutorParamDefID: id,
				ValueSource:        releasedomain.TemplateParamValueSource(strings.ToLower(strings.TrimSpace(string(item.ValueSource)))),
				SourceParamKey:     strings.TrimSpace(item.SourceParamKey),
				FixedValue:         item.FixedValue,
			})
		}
		return result
	}

	ids := normalizeStringIDs(paramDefIDs)
	result := make([]ReleaseTemplateParamConfigInput, 0, len(ids))
	for _, id := range ids {
		result = append(result, ReleaseTemplateParamConfigInput{
			ExecutorParamDefID: id,
			ValueSource:        releasedomain.TemplateParamValueSourceReleaseInput,
		})
	}
	return result
}

// normalizeStringIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeStringIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// normalizeHookEnvCodes 标准化输入值，保证后续逻辑使用统一格式。
func normalizeHookEnvCodes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

// normalizeTemplateHookExecuteStages 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTemplateHookExecuteStages(
	values []releasedomain.TemplateHookExecuteStage,
	legacy releasedomain.TemplateHookExecuteStage,
) []releasedomain.TemplateHookExecuteStage {
	normalized := make([]releasedomain.TemplateHookExecuteStage, 0, len(values))
	for _, item := range values {
		stage := releasedomain.TemplateHookExecuteStage(strings.ToLower(strings.TrimSpace(string(item))))
		if stage == "" {
			continue
		}
		normalized = append(normalized, stage)
	}
	legacyStage := releasedomain.TemplateHookExecuteStage(strings.ToLower(strings.TrimSpace(string(legacy))))
	return releasedomain.NormalizeTemplateHookExecuteStages(normalized, legacyStage)
}

// buildTemplateHooks 组装业务执行所需的输入数据。
func (uc *ReleaseTemplateManager) buildTemplateHooks(
	ctx context.Context,
	inputs []ReleaseTemplateHookInput,
) ([]releasedomain.ReleaseTemplateHook, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	result := make([]releasedomain.ReleaseTemplateHook, 0, len(inputs))
	seenNames := make(map[string]struct{}, len(inputs))
	for idx, input := range inputs {
		hookType := releasedomain.TemplateHookType(strings.ToLower(strings.TrimSpace(string(input.HookType))))
		if !hookType.Valid() {
			return nil, fmt.Errorf("%w: unsupported hook_type", ErrInvalidInput)
		}
		executeStages := normalizeTemplateHookExecuteStages(input.ExecuteStages, input.ExecuteStage)
		if len(executeStages) == 0 {
			return nil, fmt.Errorf("%w: invalid hook execute_stage", ErrInvalidInput)
		}
		executeStage := releasedomain.PrimaryTemplateHookExecuteStage(executeStages, "")
		name := strings.TrimSpace(input.Name)
		if name == "" {
			switch hookType {
			case releasedomain.TemplateHookTypeAgentTask:
				switch {
				case len(executeStages) > 1:
					name = "多阶段 Agent 任务"
				case executeStage == releasedomain.TemplateHookExecuteStageBuildComplete:
					name = "构建完成 Agent 任务"
				default:
					name = "发布后 Agent 任务"
				}
			case releasedomain.TemplateHookTypeNotificationHook:
				switch {
				case len(executeStages) > 1:
					name = "多阶段通知 Hook"
				case executeStage == releasedomain.TemplateHookExecuteStageBuildComplete:
					name = "构建完成通知 Hook"
				default:
					name = "发布后通知 Hook"
				}
			case releasedomain.TemplateHookTypeWebhookNotification:
				switch {
				case len(executeStages) > 1:
					name = "多阶段 Webhook 通知"
				case executeStage == releasedomain.TemplateHookExecuteStageBuildComplete:
					name = "构建完成 Webhook 通知"
				default:
					name = "发布后 Webhook 通知"
				}
			}
		}
		nameKey := strings.ToLower(name)
		if _, exists := seenNames[nameKey]; exists {
			return nil, fmt.Errorf("%w: duplicated hook name %s", ErrInvalidInput, name)
		}
		seenNames[nameKey] = struct{}{}

		triggerCondition := releasedomain.TemplateHookTriggerCondition(strings.ToLower(strings.TrimSpace(string(input.TriggerCondition))))
		if triggerCondition == "" {
			triggerCondition = releasedomain.TemplateHookTriggerOnSuccess
		}
		if !triggerCondition.Valid() {
			return nil, fmt.Errorf("%w: invalid hook trigger_condition", ErrInvalidInput)
		}
		if len(executeStages) == 1 &&
			executeStages[0] == releasedomain.TemplateHookExecuteStageBuildComplete &&
			triggerCondition != releasedomain.TemplateHookTriggerOnSuccess {
			return nil, fmt.Errorf("%w: 构建完成 Hook 仅支持成功后触发", ErrInvalidInput)
		}

		failurePolicy := releasedomain.TemplateHookFailurePolicy(strings.ToLower(strings.TrimSpace(string(input.FailurePolicy))))
		if failurePolicy == "" {
			failurePolicy = releasedomain.TemplateHookFailurePolicyWarnOnly
		}
		if !failurePolicy.Valid() {
			return nil, fmt.Errorf("%w: invalid hook failure_policy", ErrInvalidInput)
		}

		item := releasedomain.ReleaseTemplateHook{
			ID:               generateID("rth"),
			HookType:         hookType,
			Name:             name,
			ExecuteStage:     executeStage,
			ExecuteStages:    append([]releasedomain.TemplateHookExecuteStage(nil), executeStages...),
			TriggerCondition: triggerCondition,
			FailurePolicy:    failurePolicy,
			EnvCodes:         normalizeHookEnvCodes(input.EnvCodes),
			SortNo:           idx + 1,
			Note:             strings.TrimSpace(input.Note),
		}

		switch hookType {
		case releasedomain.TemplateHookTypeAgentTask:
			taskID := strings.TrimSpace(input.TargetID)
			if taskID == "" {
				return nil, fmt.Errorf("%w: agent task hook requires target task", ErrInvalidInput)
			}
			if uc.agentRepo == nil {
				return nil, fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
			}
			task, err := uc.agentRepo.GetTaskByID(ctx, taskID)
			if err != nil {
				return nil, err
			}
			if !isReusableAgentTaskHookTarget(task) {
				return nil, fmt.Errorf("%w: agent hook must reference a manual temporary task; dispatched history tasks cannot be used as hook templates", ErrInvalidInput)
			}
			item.TargetID = task.ID
			item.TargetName = firstNonEmpty(strings.TrimSpace(task.Name), task.ID)
		case releasedomain.TemplateHookTypeNotificationHook:
			hookID := strings.TrimSpace(input.TargetID)
			if hookID == "" {
				return nil, fmt.Errorf("%w: notification hook requires target hook", ErrInvalidInput)
			}
			if uc.notificationRepo == nil {
				return nil, fmt.Errorf("%w: notification repository is not configured", ErrInvalidInput)
			}
			notificationHook, err := uc.notificationRepo.GetHookByID(ctx, hookID)
			if err != nil {
				return nil, err
			}
			item.TargetID = notificationHook.ID
			item.TargetName = firstNonEmpty(strings.TrimSpace(notificationHook.Name), notificationHook.ID)
		case releasedomain.TemplateHookTypeWebhookNotification:
			webhookURL := strings.TrimSpace(input.WebhookURL)
			if webhookURL == "" {
				return nil, fmt.Errorf("%w: webhook hook requires webhook_url", ErrInvalidInput)
			}
			method := strings.ToUpper(strings.TrimSpace(input.WebhookMethod))
			if method == "" {
				method = "POST"
			}
			switch method {
			case "POST", "PUT", "PATCH":
			default:
				return nil, fmt.Errorf("%w: unsupported webhook_method", ErrInvalidInput)
			}
			item.WebhookMethod = method
			item.WebhookURL = webhookURL
			item.WebhookBody = strings.TrimSpace(input.WebhookBody)
		}

		result = append(result, item)
	}
	return result, nil
}

// normalizeTemplateGitOpsType 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTemplateGitOpsType(candidate releasedomain.GitOpsType, usesArgoCD bool) releasedomain.GitOpsType {
	if !usesArgoCD {
		return ""
	}
	candidate = releasedomain.GitOpsType(strings.ToLower(strings.TrimSpace(string(candidate))))
	if candidate == "" {
		return releasedomain.GitOpsTypeKustomize
	}
	if !candidate.Valid() {
		return releasedomain.GitOpsTypeKustomize
	}
	return candidate
}

// templateUsesScope 封装当前模块的业务处理逻辑。
func templateUsesScope(bindings []releasedomain.ReleaseTemplateBinding, scope releasedomain.PipelineScope) bool {
	for _, item := range bindings {
		if item.Enabled && item.PipelineScope == scope {
			return true
		}
	}
	return false
}

// buildGitOpsRules 组装业务执行所需的输入数据。
func (uc *ReleaseTemplateManager) buildGitOpsRules(
	ctx context.Context,
	applicationID string,
	bindings []releasedomain.ReleaseTemplateBinding,
	params []releasedomain.ReleaseTemplateParam,
	gitopsType releasedomain.GitOpsType,
	inputs []ReleaseTemplateGitOpsRuleInput,
	existingRules []releasedomain.ReleaseTemplateGitOpsRule,
) ([]releasedomain.ReleaseTemplateGitOpsRule, error) {
	if !templateUsesArgoCD(bindings) {
		if len(inputs) > 0 {
			return nil, fmt.Errorf("%w: 仅当 cd 使用 argocd 时才可配置 gitops 替换规则", ErrInvalidInput)
		}
		return nil, nil
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	if uc.appRepo == nil {
		return nil, fmt.Errorf("%w: application repository is not configured", ErrInvalidInput)
	}
	if uc.platformRepo == nil {
		return nil, fmt.Errorf("%w: platform param repository is not configured", ErrInvalidInput)
	}
	if uc.gitopsReader == nil {
		return nil, fmt.Errorf("%w: gitops manager is not configured", ErrInvalidInput)
	}
	gitopsType = normalizeTemplateGitOpsType(gitopsType, true)

	app, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	appKey := strings.TrimSpace(app.Key)
	if appKey == "" {
		return nil, fmt.Errorf("%w: application key is required for argocd gitops rules", ErrInvalidInput)
	}

	fieldCandidateSet := make(map[string]gitopsdomain.FieldCandidate)
	valuesCandidateSet := make(map[string]gitopsdomain.ValuesCandidate)
	switch gitopsType {
	case releasedomain.GitOpsTypeHelm:
		candidates, listErr := uc.gitopsReader.ListValuesCandidates(ctx, appKey)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range candidates {
			valuesCandidateSet[buildGitOpsValuesCandidateKey(item.FilePathTemplate, item.TargetPath)] = item
		}
	default:
		candidates, listErr := uc.gitopsReader.ListFieldCandidates(ctx, appKey)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range candidates {
			fieldCandidateSet[buildGitOpsCandidateKey(item.FilePathTemplate, item.DocumentKind, item.DocumentName, item.TargetPath)] = item
		}
	}

	builtinParams, err := uc.listBuiltinPlatformParamsForTemplate(ctx)
	if err != nil {
		return nil, err
	}
	cdInputParams, err := uc.listCDInputPlatformParamsForTemplate(ctx)
	if err != nil {
		return nil, err
	}
	ciParams := make(map[string]releasedomain.ReleaseTemplateParam)
	for _, item := range params {
		if item.PipelineScope != releasedomain.PipelineScopeCI {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		ciParams[key] = item
	}

	result := make([]releasedomain.ReleaseTemplateGitOpsRule, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	existingRuleSet := buildExistingGitOpsRuleSet(gitopsType, existingRules)
	for idx, input := range inputs {
		paramKey := strings.ToLower(strings.TrimSpace(input.SourceParamKey))
		if paramKey == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则的 source_param_key 不能为空", ErrInvalidInput)
		}

		sourceFrom := input.SourceFrom
		if sourceFrom == "" {
			if _, ok := ciParams[paramKey]; ok {
				sourceFrom = releasedomain.GitOpsRuleSourceCI
			} else if _, ok := cdInputParams[paramKey]; ok {
				sourceFrom = releasedomain.GitOpsRuleSourceCDInput
			} else if _, ok := builtinParams[paramKey]; ok {
				sourceFrom = releasedomain.GitOpsRuleSourceBuiltin
			}
		}
		if !sourceFrom.Valid() {
			return nil, fmt.Errorf("%w: gitops 替换规则的 source_from 无效", ErrInvalidInput)
		}

		var sourceName string
		switch sourceFrom {
		case releasedomain.GitOpsRuleSourceCI:
			param, ok := ciParams[paramKey]
			if !ok {
				return nil, fmt.Errorf("%w: gitops 替换规则只能引用 ci 中已勾选的标准字段：%s", ErrInvalidInput, paramKey)
			}
			sourceName = firstNonEmpty(param.ParamName, paramKey)
		case releasedomain.GitOpsRuleSourceCDInput:
			dict, ok := cdInputParams[paramKey]
			if !ok {
				return nil, fmt.Errorf("%w: gitops 替换规则只能引用已启用的 CD 自填字段：%s", ErrInvalidInput, paramKey)
			}
			sourceName = firstNonEmpty(dict.Name, paramKey)
		case releasedomain.GitOpsRuleSourceBuiltin:
			dict, ok := builtinParams[paramKey]
			if !ok {
				return nil, fmt.Errorf("%w: gitops 替换规则只能引用已启用的内置字段：%s", ErrInvalidInput, paramKey)
			}
			sourceName = firstNonEmpty(dict.Name, paramKey)
		}

		locatorParamKey := strings.ToLower(strings.TrimSpace(input.LocatorParamKey))
		locatorParamName := ""
		if locatorParamKey != "" {
			dict, err := uc.platformRepo.GetByParamKey(ctx, locatorParamKey)
			if err != nil {
				return nil, fmt.Errorf("%w: gitops 定位字段不存在：%s", ErrInvalidInput, locatorParamKey)
			}
			if dict.Status != platformparamdomain.StatusEnabled {
				return nil, fmt.Errorf("%w: gitops 定位字段已停用：%s", ErrInvalidInput, locatorParamKey)
			}
			if !dict.GitOpsLocator {
				return nil, fmt.Errorf("%w: 请选择已标记为 gitops 定位字段的标准 Key：%s", ErrInvalidInput, locatorParamKey)
			}
			if _, ok := ciParams[locatorParamKey]; !ok {
				if _, builtin := builtinParams[locatorParamKey]; !builtin {
					return nil, fmt.Errorf("%w: gitops 定位字段必须来自 ci 已勾选字段或系统内置字段：%s", ErrInvalidInput, locatorParamKey)
				}
			}
			locatorParamName = firstNonEmpty(dict.Name, locatorParamKey)
		}

		filePathTemplate := filepathSlash(strings.TrimSpace(input.FilePathTemplate))
		targetPath := strings.TrimSpace(input.TargetPath)
		documentKind := strings.TrimSpace(input.DocumentKind)
		documentName := strings.TrimSpace(input.DocumentName)
		if gitopsType == releasedomain.GitOpsTypeHelm {
			selection := gitOpsValuesTargetSelection{}
			if targetPath != "" && strings.HasPrefix(targetPath, "{") && json.Unmarshal([]byte(targetPath), &selection) == nil {
				filePathTemplate = firstNonEmpty(filepathSlash(selection.FilePathTemplate), filePathTemplate)
				targetPath = strings.TrimSpace(selection.TargetPath)
			}
			filePathTemplate = normalizeHelmValuesFilePathTemplate(filePathTemplate)
		}
		if filePathTemplate == "" || targetPath == "" {
			switch gitopsType {
			case releasedomain.GitOpsTypeHelm:
				return nil, fmt.Errorf("%w: gitops 替换规则必须选择 values 路径目标", ErrInvalidInput)
			default:
				return nil, fmt.Errorf("%w: gitops 替换规则必须选择 YAML 字段目标", ErrInvalidInput)
			}
		}

		candidateIdentity := ""
		switch gitopsType {
		case releasedomain.GitOpsTypeHelm:
			documentKind = "values"
			documentName = ""
			if !isPlatformValuesFileTemplate(filePathTemplate) {
				return nil, fmt.Errorf("%w: helm 模式下，gitops 替换规则只能写入 platform.values-{env}.yaml", ErrInvalidInput)
			}
			candidateKey := buildGitOpsValuesCandidateKey(filePathTemplate, targetPath)
			if _, ok := valuesCandidateSet[candidateKey]; !ok {
				if !existingRuleSet[buildExistingGitOpsRuleIdentityKey(gitopsType, paramKey, sourceFrom, locatorParamKey, filePathTemplate, documentKind, documentName, targetPath)] {
					return nil, fmt.Errorf("%w: 选中的 values 路径目标不存在或已变更", ErrInvalidInput)
				}
			}
			candidateIdentity = candidateKey
		default:
			if documentKind == "" {
				return nil, fmt.Errorf("%w: gitops 替换规则必须选择 YAML 字段目标", ErrInvalidInput)
			}
			candidateKey := buildGitOpsCandidateKey(filePathTemplate, documentKind, documentName, targetPath)
			if _, ok := fieldCandidateSet[candidateKey]; !ok && !matchGitOpsCandidateTemplate(fieldCandidateSet, filePathTemplate, documentKind, documentName, targetPath, locatorParamKey) {
				if !existingRuleSet[buildExistingGitOpsRuleIdentityKey(gitopsType, paramKey, sourceFrom, locatorParamKey, filePathTemplate, documentKind, documentName, targetPath)] {
					return nil, fmt.Errorf("%w: 选中的 YAML 字段目标不存在或已变更", ErrInvalidInput)
				}
			}
			candidateIdentity = candidateKey
		}
		seenKey := strings.Join([]string{candidateIdentity, paramKey, locatorParamKey}, "::")
		if _, exists := seen[seenKey]; exists {
			return nil, fmt.Errorf("%w: gitops 替换规则存在重复项", ErrInvalidInput)
		}
		seen[seenKey] = struct{}{}

		valueTemplate := strings.TrimSpace(input.ValueTemplate)
		if valueTemplate == "" && sourceFrom != releasedomain.GitOpsRuleSourceCDInput {
			valueTemplate = "{" + paramKey + "}"
		}
		if sourceFrom == releasedomain.GitOpsRuleSourceCDInput && valueTemplate == "" {
			return nil, fmt.Errorf("%w: CD 自填字段必须在模板里填写固定值", ErrInvalidInput)
		}

		result = append(result, releasedomain.ReleaseTemplateGitOpsRule{
			ID:               generateID("rtgr"),
			PipelineScope:    releasedomain.PipelineScopeCD,
			SourceParamKey:   paramKey,
			SourceParamName:  sourceName,
			SourceFrom:       sourceFrom,
			LocatorParamKey:  locatorParamKey,
			LocatorParamName: locatorParamName,
			FilePathTemplate: filePathTemplate,
			DocumentKind:     documentKind,
			DocumentName:     documentName,
			TargetPath:       targetPath,
			ValueTemplate:    valueTemplate,
			SortNo:           idx + 1,
		})
	}
	return result, nil
}

// listBuiltinPlatformParamsForTemplate 查询并返回列表数据。
func (uc *ReleaseTemplateManager) listBuiltinPlatformParamsForTemplate(
	ctx context.Context,
) (map[string]platformparamdomain.PlatformParamDict, error) {
	status := platformparamdomain.StatusEnabled
	items, _, err := uc.platformRepo.List(ctx, platformparamdomain.ListFilter{
		Status:   &status,
		Page:     1,
		PageSize: 500,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]platformparamdomain.PlatformParamDict, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		if !item.Builtin {
			if _, ok := templateBuiltinSourceKeys[key]; !ok {
				continue
			}
		}
		result[key] = item
	}
	return result, nil
}

// listCDInputPlatformParamsForTemplate 查询并返回列表数据。
func (uc *ReleaseTemplateManager) listCDInputPlatformParamsForTemplate(
	ctx context.Context,
) (map[string]platformparamdomain.PlatformParamDict, error) {
	cdSelfFill := true
	status := platformparamdomain.StatusEnabled
	items, _, err := uc.platformRepo.List(ctx, platformparamdomain.ListFilter{
		CDSelfFill: &cdSelfFill,
		Status:     &status,
		Page:       1,
		PageSize:   500,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]platformparamdomain.PlatformParamDict, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		result[key] = item
	}
	return result, nil
}

// buildGitOpsCandidateKey 组装业务执行所需的输入数据。
func buildGitOpsCandidateKey(filePathTemplate string, documentKind string, documentName string, targetPath string) string {
	return strings.Join([]string{
		filepathSlash(strings.TrimSpace(filePathTemplate)),
		strings.TrimSpace(documentKind),
		strings.TrimSpace(documentName),
		strings.TrimSpace(targetPath),
	}, "::")
}

// buildGitOpsValuesCandidateKey 组装业务执行所需的输入数据。
func buildGitOpsValuesCandidateKey(filePathTemplate string, targetPath string) string {
	return strings.Join([]string{
		filepathSlash(strings.TrimSpace(filePathTemplate)),
		strings.TrimSpace(targetPath),
	}, "::")
}

// buildExistingGitOpsRuleIdentityKey 组装业务执行所需的输入数据。
func buildExistingGitOpsRuleIdentityKey(
	gitopsType releasedomain.GitOpsType,
	sourceParamKey string,
	sourceFrom releasedomain.GitOpsRuleSourceFrom,
	locatorParamKey string,
	filePathTemplate string,
	documentKind string,
	documentName string,
	targetPath string,
) string {
	targetIdentity := buildGitOpsCandidateKey(filePathTemplate, documentKind, documentName, targetPath)
	if gitopsType == releasedomain.GitOpsTypeHelm {
		targetIdentity = buildGitOpsValuesCandidateKey(filePathTemplate, targetPath)
	}
	return strings.Join([]string{
		targetIdentity,
		strings.ToLower(strings.TrimSpace(sourceParamKey)),
		strings.ToLower(strings.TrimSpace(string(sourceFrom))),
		strings.ToLower(strings.TrimSpace(locatorParamKey)),
	}, "::")
}

// buildExistingGitOpsRuleSet 组装业务执行所需的输入数据。
func buildExistingGitOpsRuleSet(
	gitopsType releasedomain.GitOpsType,
	items []releasedomain.ReleaseTemplateGitOpsRule,
) map[string]bool {
	result := make(map[string]bool, len(items))
	for _, item := range items {
		filePathTemplate := filepathSlash(strings.TrimSpace(item.FilePathTemplate))
		documentKind := strings.TrimSpace(item.DocumentKind)
		documentName := strings.TrimSpace(item.DocumentName)
		targetPath := strings.TrimSpace(item.TargetPath)
		if gitopsType == releasedomain.GitOpsTypeHelm {
			filePathTemplate = normalizeHelmValuesFilePathTemplate(filePathTemplate)
			documentKind = "values"
			documentName = ""
		}
		key := buildExistingGitOpsRuleIdentityKey(
			gitopsType,
			item.SourceParamKey,
			item.SourceFrom,
			item.LocatorParamKey,
			filePathTemplate,
			documentKind,
			documentName,
			targetPath,
		)
		result[key] = true
	}
	return result
}

// isPlatformValuesFileTemplate 封装当前模块的业务处理逻辑。
func isPlatformValuesFileTemplate(filePathTemplate string) bool {
	base := strings.TrimSpace(filepath.Base(filepathSlash(filePathTemplate)))
	if base == "" {
		return false
	}
	matched, _ := regexp.MatchString(`(?i)^platform\.values(?:-[^.]+)?\.ya?ml$`, base)
	return matched
}

// matchGitOpsCandidateTemplate 封装当前模块的业务处理逻辑。
func matchGitOpsCandidateTemplate(
	candidateSet map[string]gitopsdomain.FieldCandidate,
	filePathTemplate string,
	documentKind string,
	documentName string,
	targetPath string,
	locatorParamKey string,
) bool {
	locatorParamKey = strings.TrimSpace(locatorParamKey)
	if locatorParamKey == "" {
		return false
	}
	placeholder := "{" + locatorParamKey + "}"
	for _, candidate := range candidateSet {
		if strings.TrimSpace(candidate.DocumentKind) != strings.TrimSpace(documentKind) {
			continue
		}
		if strings.TrimSpace(candidate.TargetPath) != strings.TrimSpace(targetPath) {
			continue
		}
		fileTemplate := buildLocatorTemplateFromCandidate(candidate.FilePathTemplate, candidate.DocumentName, placeholder)
		documentTemplate := buildLocatorDocumentTemplate(candidate.DocumentName, placeholder)
		if fileTemplate == filepathSlash(strings.TrimSpace(filePathTemplate)) &&
			strings.TrimSpace(documentTemplate) == strings.TrimSpace(documentName) {
			return true
		}
	}
	return false
}

// buildLocatorTemplateFromCandidate 组装业务执行所需的输入数据。
func buildLocatorTemplateFromCandidate(filePathTemplate string, documentName string, placeholder string) string {
	filePathTemplate = filepathSlash(filePathTemplate)
	placeholder = strings.TrimSpace(placeholder)
	if filePathTemplate == "" || placeholder == "" || strings.TrimSpace(documentName) == "" {
		return filePathTemplate
	}
	baseName := filepath.Base(filePathTemplate)
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	if stem == "" {
		return filePathTemplate
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(documentName)), strings.ToLower(stem)) {
		return filePathTemplate
	}
	replacedBase := replaceLocatorToken(baseName, stem, placeholder)
	return strings.TrimSuffix(filePathTemplate, baseName) + replacedBase
}

// buildLocatorDocumentTemplate 组装业务执行所需的输入数据。
func buildLocatorDocumentTemplate(documentName string, placeholder string) string {
	documentName = strings.TrimSpace(documentName)
	placeholder = strings.TrimSpace(placeholder)
	if documentName == "" || placeholder == "" {
		return documentName
	}
	base := documentName
	if idx := strings.IndexAny(documentName, "-_."); idx > 0 {
		base = documentName[:idx]
	}
	return replaceLocatorToken(documentName, base, placeholder)
}

// replaceLocatorToken 封装当前模块的业务处理逻辑。
func replaceLocatorToken(value string, token string, placeholder string) string {
	value = strings.TrimSpace(value)
	token = strings.TrimSpace(token)
	placeholder = strings.TrimSpace(placeholder)
	if value == "" || token == "" || placeholder == "" {
		return value
	}
	replacer := regexp.MustCompile(`(^|[-_./])` + regexp.QuoteMeta(token) + `($|[-_./])`)
	return replacer.ReplaceAllString(value, "${1}"+placeholder+"${2}")
}

// filepathSlash 封装当前模块的业务处理逻辑。
func filepathSlash(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
}

// normalizeHelmValuesFilePathTemplate 标准化输入值，保证后续逻辑使用统一格式。
func normalizeHelmValuesFilePathTemplate(value string) string {
	value = filepathSlash(value)
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
		return filepathSlash(filepath.Join("apps", "helm", filepath.Join(parts[2:]...)))
	}
	return value
}

// summarizeTemplateBindings 封装当前模块的业务处理逻辑。
func summarizeTemplateBindings(bindings []releasedomain.ReleaseTemplateBinding) (string, string) {
	if len(bindings) == 0 {
		return "-", ""
	}
	scopeLabels := make([]string, 0, len(bindings))
	scopeTypes := make([]string, 0, len(bindings))
	for _, item := range bindings {
		scope := strings.ToLower(strings.TrimSpace(string(item.PipelineScope)))
		if scope == "" {
			continue
		}
		scopeTypes = append(scopeTypes, scope)
		switch scope {
		case "ci":
			scopeLabels = append(scopeLabels, "CI")
		case "cd":
			scopeLabels = append(scopeLabels, "CD")
		default:
			scopeLabels = append(scopeLabels, strings.ToUpper(scope))
		}
	}
	return strings.Join(scopeLabels, " + "), strings.Join(scopeTypes, "+")
}

// validateArgoCDTemplateConfig 封装当前模块的业务处理逻辑。
func (uc *ReleaseTemplateManager) validateArgoCDTemplateConfig(
	ctx context.Context,
	bindings []releasedomain.ReleaseTemplateBinding,
	params []releasedomain.ReleaseTemplateParam,
	gitopsType releasedomain.GitOpsType,
) error {
	var (
		hasCIBinding bool
		hasArgoCDCD  bool
	)
	for _, item := range bindings {
		if item.PipelineScope == releasedomain.PipelineScopeCI {
			hasCIBinding = true
		}
		if item.PipelineScope == releasedomain.PipelineScopeCD && strings.EqualFold(strings.TrimSpace(item.Provider), string(pipelinedomain.ProviderArgoCD)) {
			hasArgoCDCD = true
		}
	}
	if !hasArgoCDCD {
		return nil
	}
	if !hasCIBinding {
		return fmt.Errorf("%w: cd 选择 argocd 时，必须同时启用 ci 执行单元", ErrInvalidInput)
	}
	if gitopsType != "" && !gitopsType.Valid() {
		return fmt.Errorf("%w: gitops_type 无效", ErrInvalidInput)
	}
	if uc.argocdRepo != nil {
		instances, _, err := uc.argocdRepo.ListInstances(ctx, argocddomain.InstanceListFilter{
			Status:   argocddomain.StatusActive,
			Page:     1,
			PageSize: 200,
		})
		if err != nil {
			return err
		}
		if len(instances) == 0 {
			return fmt.Errorf("%w: cd 选择 argocd 时，请先在组件管理中配置可用的 ArgoCD 实例", ErrInvalidInput)
		}
		bindings, err := uc.argocdRepo.ListEnvBindings(ctx)
		if err != nil {
			return err
		}
		activeBindingCount := 0
		for _, item := range bindings {
			if item.Status == argocddomain.StatusActive {
				activeBindingCount++
			}
		}
		if len(instances) > 1 && activeBindingCount == 0 {
			return fmt.Errorf("%w: 当前存在多个 ArgoCD 实例，请先配置环境与 ArgoCD 的绑定关系", ErrInvalidInput)
		}
	}
	_ = params
	return nil
}
