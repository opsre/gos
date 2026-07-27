package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	agentdomain "gos/internal/domain/agent"
	aidomain "gos/internal/domain/ai"
	appdomain "gos/internal/domain/application"
	argocddomain "gos/internal/domain/argocdapp"
	artifactrepodomain "gos/internal/domain/artifactrepo"
	pipelineparamdomain "gos/internal/domain/executorparam"
	gitopsdomain "gos/internal/domain/gitops"
	notificationdomain "gos/internal/domain/notification"
	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
	platformparamdomain "gos/internal/domain/platformparam"
	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/support/logx"
)

type ReleaseOrderManager struct {
	repo                     domain.Repository
	appRepo                  appdomain.Repository
	pipelineRepo             pipelinedomain.Repository
	paramRepo                pipelineparamdomain.Repository
	platformRepo             platformparamdomain.Repository
	artifactRepo             artifactrepodomain.Repository
	releaseSettings          ReleaseSettingsStore
	systemManagementSettings SystemManagementSettingsStore
	jenkins                  JenkinsReleaseExecutor
	agentRepo                agentdomain.Repository
	argocdRepo               argocddomain.Repository
	gitopsRepo               gitopsdomain.Repository
	notificationRepo         notificationdomain.Repository
	pipelineScanRepo         scandomain.Repository
	aiModelRepo              aidomain.ModelConfigRepository
	stageDiagnosisRepo       aidomain.StageDiagnosisRepository
	aiClientFactory          AIModelClientFactory
	argocdFactory            ArgoCDClientFactory
	gitopsFactory            GitOpsServiceFactory
	gitops                   GitOpsReleaseService
	approvalManagerResolver  ApprovalManagerResolver
	now                      func() time.Time
	runAsync                 func(func())
	orderLocksMu             sync.Mutex
	orderLocks               map[string]*releaseOrderOperationLock
}

type ApprovalManagerResolver interface {
	ResolveUserManager(ctx context.Context, userID string, level int) (userdomain.User, error)
}

type releaseOrderOperationLock struct {
	mu   sync.Mutex
	refs int
}

type CreateReleaseOrderInput struct {
	ApplicationID   string
	TemplateID      string
	ReleaseName     string
	PreviousOrderNo string
	EnvCode         string
	GitRef          string
	ImageTag        string
	TriggerType     domain.TriggerType
	Remark          string
	CreatorUserID   string
	TriggeredBy     string
	Params          []CreateReleaseOrderParamInput
	Steps           []CreateReleaseOrderStepInput
}

type UpdateReleaseOrderInput = CreateReleaseOrderInput

type CreateReleaseOrderParamInput struct {
	PipelineScope     domain.PipelineScope
	ParamKey          string
	ExecutorParamName string
	ParamValue        string
	ValueSource       domain.ValueSource
}

type CreateReleaseOrderStepInput struct {
	StepCode string
	StepName string
	SortNo   int
}

type ListReleaseOrderInput struct {
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []domain.ApplicationEnvScope
	VisibleToUserID             string
	ApprovalApproverUserID      string
	CreatorUserID               string
	Keyword                     string
	ConcurrentBatchNo           string
	ConcurrentBatchName         string
	TriggeredBy                 string
	BindingID                   string
	EnvCode                     string
	OperationType               domain.OperationType
	Status                      domain.OrderStatus
	TriggerType                 domain.TriggerType
	CreatedAtFrom               *time.Time
	CreatedAtTo                 *time.Time
	Page                        int
	PageSize                    int
}

type FinishReleaseOrderStepInput struct {
	Status  domain.StepStatus
	Message string
}

type JenkinsReleaseExecutor interface {
	TriggerBuild(ctx context.Context, fullName string, params map[string]string) (queueURL string, err error)
	GetQueueItem(ctx context.Context, queueURL string) (executableURL string, cancelled bool, why string, err error)
	AbortQueueItem(ctx context.Context, queueURL string) error
	AbortBuild(ctx context.Context, buildURL string) error
	GetBuildStages(ctx context.Context, buildURL string) ([]domain.ReleaseOrderPipelineStage, error)
	GetBuildStageLog(ctx context.Context, buildURL string, stageKey string) (domain.ReleaseOrderPipelineStageLog, error)
}

type JenkinsReleaseStageLogNamedReader interface {
	GetBuildStageLogWithName(ctx context.Context, buildURL string, stageKey string, stageName string) (domain.ReleaseOrderPipelineStageLog, error)
}

type JenkinsReleaseParamReader interface {
	GetJobParamSet(ctx context.Context, fullName string) (pipelineparamdomain.JenkinsJobParamSet, error)
}

type ArgoCDReleaseExecutor interface {
	ListApplications(ctx context.Context) ([]ArgoCDApplicationSnapshot, error)
	GetApplication(ctx context.Context, name string) (ArgoCDApplicationSnapshot, error)
	SyncApplication(ctx context.Context, name string) error
	SyncApplicationWithRevision(ctx context.Context, name string, revision string) error
}

// GitOpsReleaseService 只暴露 ArgoCD CD 模式下真正需要的最小能力：
// 在本地工作目录里受控修改 kustomization.yaml，并以平台身份提交推送。
type GitOpsReleaseService interface {
	UpdateKustomizationImage(
		ctx context.Context,
		repoURL string,
		sourcePath string,
		branch string,
		newTag string,
		commitMessage string,
	) (workspacePath string, manifestPath string, commitSHA string, previousTag string, changed bool, err error)
	ApplyManifestRules(
		ctx context.Context,
		repoURL string,
		branch string,
		rules []gitopsdomain.ManifestRule,
		commitMessage string,
	) (workspacePath string, changedFiles []string, commitSHA string, changed bool, err error)
	ApplyValuesRules(
		ctx context.Context,
		repoURL string,
		branch string,
		rules []gitopsdomain.ValuesRule,
		commitMessage string,
	) (workspacePath string, changedFiles []string, commitSHA string, changed bool, err error)
	BuildCommitMessage(fields map[string]string) string
	RenderTemplate(template string, fields map[string]string) string
}

// NewReleaseOrderManager 创建并返回对应组件实例。
func NewReleaseOrderManager(
	repo domain.Repository,
	appRepo appdomain.Repository,
	pipelineRepo pipelinedomain.Repository,
	paramRepo pipelineparamdomain.Repository,
	platformRepo platformparamdomain.Repository,
	releaseSettings ReleaseSettingsStore,
	jenkins JenkinsReleaseExecutor,
	agentRepo agentdomain.Repository,
	argocdRepo argocddomain.Repository,
	notificationRepo notificationdomain.Repository,
	argocdFactory ArgoCDClientFactory,
	gitopsRepo gitopsdomain.Repository,
	gitopsFactory GitOpsServiceFactory,
	gitops GitOpsReleaseService,
) *ReleaseOrderManager {
	return &ReleaseOrderManager{
		repo:             repo,
		appRepo:          appRepo,
		pipelineRepo:     pipelineRepo,
		paramRepo:        paramRepo,
		platformRepo:     platformRepo,
		releaseSettings:  releaseSettings,
		jenkins:          jenkins,
		agentRepo:        agentRepo,
		argocdRepo:       argocdRepo,
		gitopsRepo:       gitopsRepo,
		notificationRepo: notificationRepo,
		argocdFactory:    argocdFactory,
		gitopsFactory:    gitopsFactory,
		gitops:           gitops,
		orderLocks:       make(map[string]*releaseOrderOperationLock),
		runAsync: func(task func()) {
			go task()
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// lockOrderOperation serializes state-changing operations for one release order in this process.
// Tracker, cancel and dispatch all use it, so an external status read cannot race a user action's writes.
func (uc *ReleaseOrderManager) lockOrderOperation(orderID string) func() {
	key := strings.TrimSpace(orderID)
	uc.orderLocksMu.Lock()
	if uc.orderLocks == nil {
		uc.orderLocks = make(map[string]*releaseOrderOperationLock)
	}
	lock := uc.orderLocks[key]
	if lock == nil {
		lock = &releaseOrderOperationLock{}
		uc.orderLocks[key] = lock
	}
	lock.refs++
	uc.orderLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		uc.orderLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 && uc.orderLocks[key] == lock {
			delete(uc.orderLocks, key)
		}
		uc.orderLocksMu.Unlock()
	}
}

func (uc *ReleaseOrderManager) SetPipelineScanRepository(repo scandomain.Repository) {
	if uc == nil {
		return
	}
	uc.pipelineScanRepo = repo
}

func (uc *ReleaseOrderManager) SetApprovalManagerResolver(resolver ApprovalManagerResolver) {
	if uc != nil {
		uc.approvalManagerResolver = resolver
	}
}

func (uc *ReleaseOrderManager) SetAIModelRepository(repo aidomain.ModelConfigRepository) {
	if uc == nil {
		return
	}
	uc.aiModelRepo = repo
}

func (uc *ReleaseOrderManager) SetStageDiagnosisRepository(repo aidomain.StageDiagnosisRepository) {
	if uc == nil {
		return
	}
	uc.stageDiagnosisRepo = repo
}

func (uc *ReleaseOrderManager) SetAIClientFactory(factory AIModelClientFactory) {
	if uc == nil {
		return
	}
	uc.aiClientFactory = factory
}

func (uc *ReleaseOrderManager) SetArtifactRepository(repo artifactrepodomain.Repository) {
	if uc == nil {
		return
	}
	uc.artifactRepo = repo
}

func (uc *ReleaseOrderManager) SetSystemManagementSettingsStore(store SystemManagementSettingsStore) {
	if uc == nil {
		return
	}
	uc.systemManagementSettings = store
}

// Create 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) Create(
	ctx context.Context,
	input CreateReleaseOrderInput,
) (domain.ReleaseOrder, error) {
	applicationID := strings.TrimSpace(input.ApplicationID)
	templateID := strings.TrimSpace(input.TemplateID)
	logx.Info("release_order", "create_start",
		logx.F("application_id", applicationID),
		logx.F("template_id", templateID),
		logx.F("creator_user_id", input.CreatorUserID),
		logx.F("trigger_type", input.TriggerType),
		logx.F("env_code", input.EnvCode),
		logx.F("params_count", len(input.Params)),
	)
	if applicationID == "" || templateID == "" {
		err := fmt.Errorf("%w: application_id and template_id are required", ErrInvalidInput)
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
		)
		return domain.ReleaseOrder{}, err
	}
	input.EnvCode = strings.TrimSpace(input.EnvCode)

	app, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
		)
		return domain.ReleaseOrder{}, err
	}

	triggerType := input.TriggerType
	if triggerType == "" {
		triggerType = domain.TriggerTypeManual
	}
	if !triggerType.Valid() {
		logx.Error("release_order", "create_failed", ErrInvalidInput,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
			logx.F("reason", "invalid_trigger_type"),
			logx.F("trigger_type", triggerType),
		)
		return domain.ReleaseOrder{}, ErrInvalidInput
	}

	template, templateBindings, templateParams, templateHooks, err := uc.resolveTemplateForCreate(ctx, applicationID, templateID)
	if err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
		)
		return domain.ReleaseOrder{}, err
	}
	approvalFlowID, flowBindingErr := uc.repo.GetApplicationApprovalFlowID(ctx, applicationID)
	if flowBindingErr != nil {
		return domain.ReleaseOrder{}, flowBindingErr
	}
	if approvalFlowID != "" {
		flow, flowErr := uc.repo.GetApprovalFlowDefinitionByID(ctx, approvalFlowID)
		if flowErr != nil {
			return domain.ReleaseOrder{}, flowErr
		}
		if flow.Status != domain.ApprovalFlowStatusActive {
			return domain.ReleaseOrder{}, fmt.Errorf("%w: approval flow is disabled", ErrInvalidInput)
		}
		if _, flowErr = normalizeApprovalFlowNodes(flow.Nodes); flowErr != nil {
			return domain.ReleaseOrder{}, flowErr
		}
	}

	if strings.TrimSpace(input.ReleaseName) == "" {
		input.ReleaseName = fmt.Sprintf("%s-%s-%s发布", app.Name, template.Name, time.Now().In(time.FixedZone("CST", 8*3600)).Format("20060102150405"))
	}

	if err := uc.validateCreateTemplateParams(ctx, template.ID, templateBindings, templateParams, input.Params); err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
			logx.F("template_name", template.Name),
		)
		return domain.ReleaseOrder{}, err
	}
	executions := uc.buildCreateExecutions("", uc.now(), templateBindings)
	inputSummary := resolveReleaseOrderSummaryFields(input.Params)
	envCode := firstNonEmpty(input.EnvCode, inputSummary.EnvCode)
	if envCode == "" {
		err := fmt.Errorf("%w: env_code is required", ErrInvalidInput)
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
		)
		return domain.ReleaseOrder{}, err
	}
	resolvedParams, err := uc.materializeCreateTemplateParams(
		ctx,
		app,
		templateParams,
		input.Params,
		envCode,
		firstNonEmpty(strings.TrimSpace(input.GitRef), inputSummary.GitRef),
		inputSummary.ProjectName,
		firstNonEmpty(strings.TrimSpace(input.ImageTag), inputSummary.ImageTag),
		strings.TrimSpace(input.ReleaseName),
	)
	if err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
			logx.F("template_name", template.Name),
		)
		return domain.ReleaseOrder{}, err
	}
	summary := resolveReleaseOrderSummaryFields(resolvedParams)
	primaryExecution, ok := pickPrimaryExecution(executions)
	if !ok {
		err := fmt.Errorf("%w: release template has no enabled executions", ErrInvalidInput)
		logx.Error("release_order", "create_failed", err,
			logx.F("application_id", applicationID),
			logx.F("template_id", templateID),
		)
		return domain.ReleaseOrder{}, err
	}

	now := uc.now()
	useLegacyApproval := approvalFlowID == ""
	autoApproved := useLegacyApproval && shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, strings.TrimSpace(input.CreatorUserID))
	initialStatus := domain.OrderStatusPending
	if useLegacyApproval {
		initialStatus = resolveInitialReleaseOrderStatus(template, strings.TrimSpace(input.CreatorUserID))
	}
	var approvedAt *time.Time
	approvedBy := ""
	if autoApproved {
		approvedAt = &now
		approvedBy = firstNonEmpty(strings.TrimSpace(input.TriggeredBy), strings.TrimSpace(input.CreatorUserID))
	}
	order := domain.ReleaseOrder{
		ID:                    generateID("ro"),
		OrderNo:               generateOrderNo(now),
		ReleaseName:           strings.TrimSpace(input.ReleaseName),
		PreviousOrderNo:       strings.TrimSpace(input.PreviousOrderNo),
		OperationType:         domain.OperationTypeDeploy,
		ApplicationID:         applicationID,
		ApplicationName:       app.Name,
		TemplateID:            template.ID,
		TemplateName:          template.Name,
		BindingID:             primaryExecution.BindingID,
		PipelineID:            primaryExecution.PipelineID,
		EnvCode:               envCode,
		GitRef:                firstNonEmpty(strings.TrimSpace(input.GitRef), summary.GitRef),
		ImageTag:              firstNonEmpty(summary.ImageTag, strings.TrimSpace(input.ImageTag)),
		TriggerType:           triggerType,
		Status:                initialStatus,
		ApprovalRequired:      useLegacyApproval && template.ApprovalEnabled,
		ApprovalMode:          template.ApprovalMode,
		ApprovalApproverIDs:   append([]string(nil), template.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), template.ApprovalApproverNames...),
		ApprovedAt:            approvedAt,
		ApprovedBy:            approvedBy,
		Remark:                strings.TrimSpace(input.Remark),
		CreatorUserID:         strings.TrimSpace(input.CreatorUserID),
		TriggeredBy:           strings.TrimSpace(input.TriggeredBy),
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	executions = uc.buildCreateExecutions(order.ID, now, templateBindings)
	params, err := uc.buildCreateParams(order.ID, now, resolvedParams, executions)
	if err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("application_id", applicationID),
		)
		return domain.ReleaseOrder{}, err
	}
	steps, err := uc.buildCreateSteps(order.ID, now, executions, template.GitOpsType, templateHooks, input.Steps, order.EnvCode)
	if err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	if err := uc.repo.Create(ctx, order, executions, params, steps); err != nil {
		logx.Error("release_order", "create_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if err := uc.initializeApprovalFlow(ctx, order.ID, approvalFlowID); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if autoApproved {
		if err := uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
			ID:             generateID("rapr"),
			ReleaseOrderID: order.ID,
			Action:         domain.ReleaseOrderApprovalActionApprove,
			OperatorUserID: strings.TrimSpace(input.CreatorUserID),
			OperatorName:   approvedBy,
			Comment:        "发起人即审批人，系统已自动通过审批",
			CreatedAt:      now,
		}); err != nil {
			logx.Error("release_order", "create_auto_approval_record_failed", err,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			return domain.ReleaseOrder{}, err
		}
	}
	logx.Info("release_order", "create_success",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("application_id", order.ApplicationID),
		logx.F("template_id", order.TemplateID),
		logx.F("env_code", order.EnvCode),
		logx.F("executions_count", len(executions)),
		logx.F("params_count", len(params)),
		logx.F("steps_count", len(steps)),
	)
	return uc.repo.GetByID(ctx, order.ID)
}

// Update 更新业务资源并返回处理结果。
func (uc *ReleaseOrderManager) Update(
	ctx context.Context,
	orderID string,
	input UpdateReleaseOrderInput,
) (domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}

	existing, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !isEditableOrderStatus(existing.Status) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: only pending release orders can be edited", ErrInvalidStatus)
	}
	if existing.OperationType != domain.OperationTypeDeploy || strings.TrimSpace(existing.SourceOrderID) != "" {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: only original deploy release orders can be edited", ErrInvalidInput)
	}

	applicationID := strings.TrimSpace(input.ApplicationID)
	templateID := strings.TrimSpace(input.TemplateID)
	logx.Info("release_order", "update_start",
		logx.F("order_id", existing.ID),
		logx.F("order_no", existing.OrderNo),
		logx.F("application_id", applicationID),
		logx.F("template_id", templateID),
		logx.F("operator_user_id", input.CreatorUserID),
		logx.F("env_code", input.EnvCode),
		logx.F("params_count", len(input.Params)),
	)
	if applicationID == "" || templateID == "" {
		err := fmt.Errorf("%w: application_id and template_id are required", ErrInvalidInput)
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if applicationID != strings.TrimSpace(existing.ApplicationID) {
		err := fmt.Errorf("%w: application_id cannot be changed when editing release order", ErrInvalidInput)
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
			logx.F("existing_application_id", existing.ApplicationID),
			logx.F("requested_application_id", applicationID),
		)
		return domain.ReleaseOrder{}, err
	}
	input.EnvCode = strings.TrimSpace(input.EnvCode)

	app, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	triggerType := existing.TriggerType
	if strings.TrimSpace(string(input.TriggerType)) != "" {
		triggerType = input.TriggerType
	}
	if triggerType == "" {
		triggerType = domain.TriggerTypeManual
	}
	if !triggerType.Valid() {
		logx.Error("release_order", "update_failed", ErrInvalidInput,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
			logx.F("reason", "invalid_trigger_type"),
			logx.F("trigger_type", triggerType),
		)
		return domain.ReleaseOrder{}, ErrInvalidInput
	}

	template, templateBindings, templateParams, templateHooks, err := uc.resolveTemplateForCreate(ctx, applicationID, templateID)
	if err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if err := uc.validateCreateTemplateParams(ctx, template.ID, templateBindings, templateParams, input.Params); err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
			logx.F("template_name", template.Name),
		)
		return domain.ReleaseOrder{}, err
	}
	executions := uc.buildCreateExecutions("", uc.now(), templateBindings)
	inputSummary := resolveReleaseOrderSummaryFields(input.Params)
	envCode := firstNonEmpty(input.EnvCode, inputSummary.EnvCode)
	if envCode == "" {
		err := fmt.Errorf("%w: env_code is required", ErrInvalidInput)
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	resolvedParams, err := uc.materializeCreateTemplateParams(
		ctx,
		app,
		templateParams,
		input.Params,
		envCode,
		firstNonEmpty(strings.TrimSpace(input.GitRef), inputSummary.GitRef),
		inputSummary.ProjectName,
		firstNonEmpty(strings.TrimSpace(input.ImageTag), inputSummary.ImageTag),
		strings.TrimSpace(input.ReleaseName),
	)
	if err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
			logx.F("template_name", template.Name),
		)
		return domain.ReleaseOrder{}, err
	}
	summary := resolveReleaseOrderSummaryFields(resolvedParams)
	primaryExecution, ok := pickPrimaryExecution(executions)
	if !ok {
		err := fmt.Errorf("%w: release template has no enabled executions", ErrInvalidInput)
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", existing.ID),
			logx.F("order_no", existing.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	now := uc.now()
	autoApproved := shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, strings.TrimSpace(existing.CreatorUserID))
	initialStatus := resolveInitialReleaseOrderStatus(template, strings.TrimSpace(existing.CreatorUserID))
	var approvedAt *time.Time
	approvedBy := ""
	if autoApproved {
		approvedAt = &now
		approvedBy = firstNonEmpty(strings.TrimSpace(existing.TriggeredBy), strings.TrimSpace(existing.CreatorUserID))
	}
	order := domain.ReleaseOrder{
		ID:                    existing.ID,
		OrderNo:               existing.OrderNo,
		ReleaseName:           strings.TrimSpace(input.ReleaseName),
		PreviousOrderNo:       existing.PreviousOrderNo,
		OperationType:         existing.OperationType,
		SourceOrderID:         existing.SourceOrderID,
		SourceOrderNo:         existing.SourceOrderNo,
		ApplicationID:         applicationID,
		ApplicationName:       app.Name,
		TemplateID:            template.ID,
		TemplateName:          template.Name,
		BindingID:             primaryExecution.BindingID,
		PipelineID:            primaryExecution.PipelineID,
		EnvCode:               envCode,
		GitRef:                firstNonEmpty(strings.TrimSpace(input.GitRef), summary.GitRef),
		ImageTag:              firstNonEmpty(summary.ImageTag, strings.TrimSpace(input.ImageTag)),
		TriggerType:           triggerType,
		Status:                initialStatus,
		ApprovalRequired:      template.ApprovalEnabled,
		ApprovalMode:          template.ApprovalMode,
		ApprovalApproverIDs:   append([]string(nil), template.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), template.ApprovalApproverNames...),
		ApprovedAt:            approvedAt,
		ApprovedBy:            approvedBy,
		RejectedAt:            nil,
		RejectedBy:            "",
		RejectedReason:        "",
		QueuePosition:         0,
		QueuedReason:          "",
		Remark:                strings.TrimSpace(input.Remark),
		CreatorUserID:         existing.CreatorUserID,
		TriggeredBy:           existing.TriggeredBy,
		StartedAt:             nil,
		FinishedAt:            nil,
		CreatedAt:             existing.CreatedAt,
		UpdatedAt:             now,
	}

	executions = uc.buildCreateExecutions(order.ID, now, templateBindings)
	params, err := uc.buildCreateParams(order.ID, now, resolvedParams, executions)
	if err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	steps, err := uc.buildCreateSteps(order.ID, now, executions, template.GitOpsType, templateHooks, input.Steps, order.EnvCode)
	if err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	if err := uc.repo.UpdateEditable(ctx, order, executions, params, steps); err != nil {
		logx.Error("release_order", "update_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if autoApproved {
		if err := uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
			ID:             generateID("rapr"),
			ReleaseOrderID: order.ID,
			Action:         domain.ReleaseOrderApprovalActionApprove,
			OperatorUserID: strings.TrimSpace(existing.CreatorUserID),
			OperatorName:   approvedBy,
			Comment:        "发起人即审批人，系统已自动通过审批",
			CreatedAt:      now,
		}); err != nil {
			logx.Error("release_order", "update_auto_approval_record_failed", err,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			return domain.ReleaseOrder{}, err
		}
	}
	logx.Info("release_order", "update_success",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("application_id", order.ApplicationID),
		logx.F("template_id", order.TemplateID),
		logx.F("env_code", order.EnvCode),
		logx.F("executions_count", len(executions)),
		logx.F("params_count", len(params)),
		logx.F("steps_count", len(steps)),
	)
	return uc.repo.GetByID(ctx, order.ID)
}

// CreateRollbackByApplication 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) CreateRollbackByApplication(
	ctx context.Context,
	applicationID string,
	creatorUserID string,
	triggeredBy string,
) (domain.ReleaseOrder, error) {
	_ = applicationID
	_ = creatorUserID
	_ = triggeredBy
	return domain.ReleaseOrder{}, fmt.Errorf("%w: 按应用自动恢复已废弃，请基于指定发布单发起重放", ErrInvalidInput)
}

// CreateStandardRollbackByOrder 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) CreateStandardRollbackByOrder(
	ctx context.Context,
	sourceOrderID string,
	creatorUserID string,
	triggeredBy string,
) (domain.ReleaseOrder, error) {
	sourceOrderID = strings.TrimSpace(sourceOrderID)
	logx.Info("release_order", "rollback_create_start",
		logx.F("source_order_id", sourceOrderID),
		logx.F("creator_user_id", creatorUserID),
	)
	sourceOrder, sourceExecutions, err := uc.loadRecoverySourceOrder(ctx, sourceOrderID)
	if err != nil {
		logx.Error("release_order", "rollback_create_failed", err,
			logx.F("source_order_id", sourceOrderID),
		)
		return domain.ReleaseOrder{}, err
	}
	sourceCDExecution, err := resolveCDExecution(sourceExecutions)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !isArgoCDExecution(sourceCDExecution) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前发布单不支持 Argo 重放", ErrInvalidInput)
	}
	if !canCreateArgoReplayFromStatus(sourceOrder.Status) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前发布单状态不支持发起 Argo 重放", ErrInvalidInput)
	}
	sourceParams, err := uc.repo.ListParams(ctx, sourceOrder.ID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}

	template, templateBindings, _, _, templateHooks, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(sourceOrder.TemplateID))
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	cdBinding, ok := selectRecoveryTemplateBinding(templateBindings, domain.PipelineScopeCD)
	if !ok {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前模板未配置可用的 CD 执行器", ErrInvalidInput)
	}
	if !strings.EqualFold(strings.TrimSpace(cdBinding.Provider), string(pipelinedomain.ProviderArgoCD)) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前模板的 CD 方式不是 ArgoCD，无法执行 Argo 重放", ErrInvalidInput)
	}
	order, err := uc.createRecoveryOrder(
		ctx,
		sourceOrder,
		sourceParams,
		template,
		templateHooks,
		cdBinding,
		domain.PipelineScopeCD,
		nil,
		domain.OperationTypeRollback,
		buildRollbackRemark(sourceOrder),
		strings.TrimSpace(creatorUserID),
		strings.TrimSpace(triggeredBy),
	)
	if err != nil {
		logx.Error("release_order", "rollback_create_failed", err,
			logx.F("source_order_id", sourceOrder.ID),
			logx.F("source_order_no", sourceOrder.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	logx.Info("release_order", "rollback_create_success",
		logx.F("source_order_id", sourceOrder.ID),
		logx.F("source_order_no", sourceOrder.OrderNo),
		logx.F("new_order_id", order.ID),
		logx.F("new_order_no", order.OrderNo),
	)
	return order, nil
}

// CreatePipelineReplayByOrder 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) CreatePipelineReplayByOrder(
	ctx context.Context,
	sourceOrderID string,
	creatorUserID string,
	triggeredBy string,
) (domain.ReleaseOrder, error) {
	sourceOrderID = strings.TrimSpace(sourceOrderID)
	logx.Info("release_order", "pipeline_replay_create_start",
		logx.F("source_order_id", sourceOrderID),
		logx.F("creator_user_id", creatorUserID),
	)
	sourceOrder, sourceExecutions, err := uc.loadRecoverySourceOrder(ctx, sourceOrderID)
	if err != nil {
		logx.Error("release_order", "pipeline_replay_create_failed", err,
			logx.F("source_order_id", sourceOrderID),
		)
		return domain.ReleaseOrder{}, err
	}
	if err := ensureReplaySourceOrderCanReplay(sourceOrder); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if !canCreatePipelineReplayFromStatus(sourceOrder.Status) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 仅支持从成功或失败发布单发起一键重发", ErrInvalidInput)
	}
	sourceReplayExecution, err := resolveReplayExecution(sourceExecutions)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if isArgoCDExecution(sourceReplayExecution) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: ArgoCD 发布单请使用 Argo 重放，不支持标准重放", ErrInvalidInput)
	}

	sourceParams, err := uc.repo.ListParams(ctx, sourceOrder.ID)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	replayScope := sourceReplayExecution.PipelineScope
	replayParamsFromSource := filterReleaseOrderParamsByScope(sourceParams, replayScope)
	if replayScope == domain.PipelineScopeCD && len(replayParamsFromSource) == 0 {
		ciParams := filterReleaseOrderParamsByScope(sourceParams, domain.PipelineScopeCI)
		if len(ciParams) > 0 {
			replayScope = domain.PipelineScopeCI
			replayParamsFromSource = ciParams
		}
	}
	if len(replayParamsFromSource) == 0 {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 来源发布单缺少可重放参数快照，无法执行参数重放", ErrInvalidInput)
	}

	template, templateBindings, templateParams, _, templateHooks, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(sourceOrder.TemplateID))
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	replayBinding, ok := selectRecoveryTemplateBinding(templateBindings, replayScope)
	if !ok {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前模板未配置可用的 %s 执行器", ErrInvalidInput, strings.ToUpper(string(replayScope)))
	}
	if strings.EqualFold(strings.TrimSpace(replayBinding.Provider), string(pipelinedomain.ProviderArgoCD)) {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前模板的 %s 方式不是管线，无法执行参数重放", ErrInvalidInput, strings.ToUpper(string(replayScope)))
	}
	if err := ensureReplayParamsMatchTemplate(templateParams, replayParamsFromSource, replayScope); err != nil {
		return domain.ReleaseOrder{}, err
	}

	replayParams := make([]CreateReleaseOrderParamInput, 0, len(replayParamsFromSource))
	for _, item := range replayParamsFromSource {
		replayParams = append(replayParams, CreateReleaseOrderParamInput{
			PipelineScope:     replayScope,
			ParamKey:          strings.TrimSpace(item.ParamKey),
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			ParamValue:        strings.TrimSpace(item.ParamValue),
			ValueSource:       item.ValueSource,
		})
	}
	order, err := uc.createRecoveryOrder(
		ctx,
		sourceOrder,
		sourceParams,
		template,
		templateHooks,
		replayBinding,
		replayScope,
		replayParams,
		domain.OperationTypeReplay,
		buildReplayRemark(sourceOrder, replayScope),
		strings.TrimSpace(creatorUserID),
		strings.TrimSpace(triggeredBy),
	)
	if err != nil {
		logx.Error("release_order", "pipeline_replay_create_failed", err,
			logx.F("source_order_id", sourceOrder.ID),
			logx.F("source_order_no", sourceOrder.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	logx.Info("release_order", "pipeline_replay_create_success",
		logx.F("source_order_id", sourceOrder.ID),
		logx.F("source_order_no", sourceOrder.OrderNo),
		logx.F("new_order_id", order.ID),
		logx.F("new_order_no", order.OrderNo),
	)
	return order, nil
}

// validateCreateTemplateParams 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) validateCreateTemplateParams(
	ctx context.Context,
	templateID string,
	templateBindings []domain.ReleaseTemplateBinding,
	templateParams []domain.ReleaseTemplateParam,
	params []CreateReleaseOrderParamInput,
) error {
	allowed := make(map[string]releasedTemplateParamRule)
	allowedByParamKey := make(map[string]releasedTemplateParamRule)
	duplicateParamKeys := make(map[string]struct{})
	required := make(map[string]releasedTemplateParamRule)
	bindingByScope := make(map[domain.PipelineScope]domain.ReleaseTemplateBinding, len(templateBindings))
	for _, item := range templateBindings {
		bindingByScope[item.PipelineScope] = item
	}

	if templateID != "" {
		for _, item := range templateParams {
			if uc.paramRepo != nil {
				paramDef, err := uc.paramRepo.GetByID(ctx, item.ExecutorParamDefID)
				if err != nil {
					return err
				}
				if err := ensureActiveExecutorParamDef(paramDef, item.ParamName); err != nil {
					return err
				}
			}
			key := buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)
			rule := releasedTemplateParamRule{
				PipelineScope:     item.PipelineScope,
				ParamKey:          strings.ToLower(strings.TrimSpace(item.ParamKey)),
				ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
				Required: item.Required ||
					item.ValueSource == "" ||
					item.ValueSource == domain.TemplateParamValueSourceReleaseInput,
				ValueSource: item.ValueSource,
			}
			allowed[key] = rule
			paramKey := buildReleaseTemplateScopeParamKey(item.PipelineScope, item.ParamKey)
			if paramKey != "" {
				if item.ValueSource == domain.TemplateParamValueSourceReleaseInput || item.ValueSource == "" {
					if _, exists := allowedByParamKey[paramKey]; exists {
						delete(allowedByParamKey, paramKey)
						duplicateParamKeys[paramKey] = struct{}{}
					} else if _, duplicated := duplicateParamKeys[paramKey]; !duplicated {
						allowedByParamKey[paramKey] = rule
					}
				}
			}
			if item.ValueSource == domain.TemplateParamValueSourceReleaseInput || item.ValueSource == "" {
				required[key] = rule
			}
		}
	}

	submitted := make(map[string]struct{}, len(params))
	for _, item := range params {
		scope := item.PipelineScope
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		executorParamName := strings.TrimSpace(item.ExecutorParamName)
		if templateID == "" {
			return fmt.Errorf("%w: extra release params require release template", ErrInvalidInput)
		}
		if !scope.Valid() {
			return fmt.Errorf("%w: pipeline_scope is required", ErrInvalidInput)
		}
		if _, ok := bindingByScope[scope]; !ok {
			return fmt.Errorf("%w: scope %s is not enabled in selected release template", ErrInvalidInput, strings.ToUpper(string(scope)))
		}
		rule, key, ok := resolveReleaseTemplateRule(allowed, allowedByParamKey, scope, paramKey, executorParamName)
		if !ok {
			return fmt.Errorf("%w: param %s is not included in selected release template", ErrInvalidInput, executorParamNameOrKey(executorParamName, paramKey))
		}
		if rule.ValueSource != "" && rule.ValueSource != domain.TemplateParamValueSourceReleaseInput {
			return fmt.Errorf("%w: param %s is fixed or derived by selected release template", ErrInvalidInput, executorParamNameOrKey(executorParamName, paramKey))
		}
		if strings.TrimSpace(item.ParamValue) == "" {
			if rule.Required {
				return fmt.Errorf("%w: param %s is required by selected release template", ErrInvalidInput, executorParamNameOrKey(executorParamName, paramKey))
			}
			continue
		}
		submitted[key] = struct{}{}
	}

	for key, rule := range required {
		if _, ok := submitted[key]; ok {
			continue
		}
		return fmt.Errorf("%w: param %s is required by selected release template", ErrInvalidInput, executorParamNameOrKey(rule.ExecutorParamName, rule.ParamKey))
	}
	return nil
}

type releasedTemplateParamRule struct {
	PipelineScope     domain.PipelineScope
	ParamKey          string
	ExecutorParamName string
	Required          bool
	ValueSource       domain.TemplateParamValueSource
}

// buildReleaseTemplateScopeParamKey 组装业务执行所需的输入数据。
func buildReleaseTemplateScopeParamKey(scope domain.PipelineScope, paramKey string) string {
	return strings.ToLower(strings.TrimSpace(string(scope))) + "::" + strings.ToLower(strings.TrimSpace(paramKey))
}

// buildReleaseTemplateParamKey 组装业务执行所需的输入数据。
func buildReleaseTemplateParamKey(scope domain.PipelineScope, paramKey string, executorParamName string) string {
	return buildReleaseTemplateScopeParamKey(scope, paramKey) + "::" + strings.ToLower(strings.TrimSpace(executorParamName))
}

// resolveReleaseTemplateRule 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseTemplateRule(
	allowed map[string]releasedTemplateParamRule,
	allowedByParamKey map[string]releasedTemplateParamRule,
	scope domain.PipelineScope,
	paramKey string,
	executorParamName string,
) (releasedTemplateParamRule, string, bool) {
	key := buildReleaseTemplateParamKey(scope, paramKey, executorParamName)
	if rule, ok := allowed[key]; ok {
		return rule, key, true
	}
	if strings.TrimSpace(executorParamName) != "" {
		return releasedTemplateParamRule{}, "", false
	}
	rule, ok := allowedByParamKey[buildReleaseTemplateScopeParamKey(scope, paramKey)]
	if !ok {
		return releasedTemplateParamRule{}, "", false
	}
	return rule, buildReleaseTemplateParamKey(rule.PipelineScope, rule.ParamKey, rule.ExecutorParamName), true
}

// executorParamNameOrKey 封装当前模块的业务处理逻辑。
func executorParamNameOrKey(executorParamName string, paramKey string) string {
	if strings.TrimSpace(executorParamName) != "" {
		return strings.TrimSpace(executorParamName)
	}
	return strings.TrimSpace(paramKey)
}

const (
	standardParamGOSArtifactURL  = "gos_artifact_url"
	standardParamGOSArtifactPath = "gos_artifact_path"
)

// isCIOnlyStandardParamKey 判断标准字段是否只能从 CI 执行单元取值。
func isCIOnlyStandardParamKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), standardParamGOSArtifactURL)
}

// templateUsesArgoCD 封装当前模块的业务处理逻辑。
func templateUsesArgoCD(bindings []domain.ReleaseTemplateBinding) bool {
	for _, item := range bindings {
		if item.PipelineScope != domain.PipelineScopeCD {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Provider), string(pipelinedomain.ProviderArgoCD)) {
			return true
		}
	}
	return false
}

// materializeCreateTemplateParams 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) materializeCreateTemplateParams(
	ctx context.Context,
	app appdomain.Application,
	templateParams []domain.ReleaseTemplateParam,
	input []CreateReleaseOrderParamInput,
	envCode string,
	gitRef string,
	projectName string,
	imageTag string,
	releaseName string,
) ([]CreateReleaseOrderParamInput, error) {
	submittedByTemplateKey := make(map[string]CreateReleaseOrderParamInput, len(input))
	submittedByScopeParamKey := make(map[string]CreateReleaseOrderParamInput, len(input))
	for _, item := range input {
		templateKey := buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)
		submittedByTemplateKey[templateKey] = item
		submittedByScopeParamKey[buildReleaseTemplateScopeParamKey(item.PipelineScope, item.ParamKey)] = item
	}

	appKey := strings.TrimSpace(app.Key)
	resolvedParams := make([]CreateReleaseOrderParamInput, 0, len(templateParams))
	resolvedValues := map[domain.PipelineScope]map[string]string{
		domain.PipelineScopeCI: {},
		domain.PipelineScopeCD: {},
	}
	artifactValues, err := uc.resolveApplicationArtifactParamValues(ctx, app)
	if err != nil {
		return nil, err
	}
	executorDefaultValues, err := uc.loadTemplateParamDefaultValues(ctx, templateParams)
	if err != nil {
		return nil, err
	}

	resolveSubmitted := func(item domain.ReleaseTemplateParam) (CreateReleaseOrderParamInput, bool) {
		templateKey := buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)
		if submitted, ok := submittedByTemplateKey[templateKey]; ok {
			return submitted, true
		}
		submitted, ok := submittedByScopeParamKey[buildReleaseTemplateScopeParamKey(item.PipelineScope, item.ParamKey)]
		return submitted, ok
	}
	executorDefaultValue := func(item domain.ReleaseTemplateParam) string {
		if isCIOnlyStandardParamKey(item.ParamKey) || isCIOnlyStandardParamKey(item.SourceParamKey) {
			return ""
		}
		return executorDefaultValues[buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)]
	}

	appendResolved := func(item domain.ReleaseTemplateParam, value string, source domain.ValueSource) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if source == "" {
			source = domain.ValueSourceReleaseInput
		}
		resolvedParams = append(resolvedParams, CreateReleaseOrderParamInput{
			PipelineScope:     item.PipelineScope,
			ParamKey:          strings.TrimSpace(item.ParamKey),
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			ParamValue:        value,
			ValueSource:       source,
		})
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if paramKey != "" {
			resolvedValues[item.PipelineScope][paramKey] = value
		}
	}

	for _, scope := range []domain.PipelineScope{domain.PipelineScopeCI, domain.PipelineScopeCD} {
		for _, item := range templateParams {
			if item.PipelineScope != scope {
				continue
			}
			switch item.ValueSource {
			case "", domain.TemplateParamValueSourceReleaseInput:
				submitted, ok := resolveSubmitted(item)
				if !ok {
					continue
				}
				appendResolved(item, submitted.ParamValue, domain.ValueSourceReleaseInput)
			case domain.TemplateParamValueSourceFixed:
				appendResolved(item, item.FixedValue, domain.ValueSourceFixed)
			case domain.TemplateParamValueSourceCIParam:
				appendResolved(
					item,
					firstNonEmpty(
						resolvedValues[domain.PipelineScopeCI][strings.ToLower(strings.TrimSpace(item.SourceParamKey))],
						resolveCreateStandardFieldValue(strings.TrimSpace(item.SourceParamKey), envCode, projectName, gitRef, imageTag, releaseName, appKey, artifactValues, resolvedValues),
					),
					domain.ValueSourceCIParam,
				)
			case domain.TemplateParamValueSourceBuiltin:
				appendResolved(
					item,
					firstNonEmpty(
						resolveCreateStandardFieldValue(strings.TrimSpace(item.SourceParamKey), envCode, projectName, gitRef, imageTag, releaseName, appKey, artifactValues, resolvedValues),
						executorDefaultValue(item),
					),
					domain.ValueSourceBuiltin,
				)
			default:
				return nil, fmt.Errorf("%w: unsupported template param value_source %s", ErrInvalidInput, item.ValueSource)
			}
		}
	}

	return resolvedParams, nil
}

// resolveApplicationArtifactParamValues 从应用绑定的制品库配置解析 OSS 内置字段。
func (uc *ReleaseOrderManager) resolveApplicationArtifactParamValues(
	ctx context.Context,
	app appdomain.Application,
) (map[string]string, error) {
	result := make(map[string]string)
	put := func(key string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		result[key] = value
	}
	put(standardParamGOSArtifactPath, app.ArtifactDirectory)
	if uc == nil || uc.artifactRepo == nil {
		return result, nil
	}
	repositoryID := strings.TrimSpace(app.ArtifactRepositoryID)
	if repositoryID == "" {
		return result, nil
	}
	repo, err := uc.artifactRepo.GetByID(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if repo.Status != "" && repo.Status != artifactrepodomain.StatusEnabled {
		return nil, fmt.Errorf("%w: artifact repository is disabled", ErrInvalidInput)
	}
	put("oss_endpoint", repo.Endpoint)
	put("oss_bucket", repo.Bucket)
	put("oss_dir", firstNonEmpty(strings.TrimSpace(app.ArtifactDirectory), strings.TrimSpace(repo.Directory)))
	put("oss_acl", string(repo.ACL))
	put("oss_access_key_id", repo.AccessKeyID)
	put("oss_access_key_secret", repo.AccessKeySecret)
	return result, nil
}

// loadTemplateParamDefaultValues 读取模板参数对应执行器参数的默认值。
func (uc *ReleaseOrderManager) loadTemplateParamDefaultValues(
	ctx context.Context,
	templateParams []domain.ReleaseTemplateParam,
) (map[string]string, error) {
	result := make(map[string]string)
	if uc == nil || uc.paramRepo == nil {
		return result, nil
	}
	defByID := make(map[string]pipelineparamdomain.ExecutorParamDef)
	for _, item := range templateParams {
		id := strings.TrimSpace(item.ExecutorParamDefID)
		if id == "" {
			continue
		}
		paramDef, ok := defByID[id]
		if !ok {
			var err error
			paramDef, err = uc.paramRepo.GetByID(ctx, id)
			if err != nil {
				return nil, err
			}
			defByID[id] = paramDef
		}
		defaultValue := strings.TrimSpace(paramDef.DefaultValue)
		if defaultValue == "" {
			continue
		}
		key := buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)
		if key == "" {
			continue
		}
		result[key] = defaultValue
	}
	return result, nil
}

// loadTemplateParamDefaultValuesBestEffort 用于详情展示，参数定义异常不应阻断页面。
func (uc *ReleaseOrderManager) loadTemplateParamDefaultValuesBestEffort(
	ctx context.Context,
	templateParams []domain.ReleaseTemplateParam,
) map[string]string {
	values, err := uc.loadTemplateParamDefaultValues(ctx, templateParams)
	if err != nil {
		return map[string]string{}
	}
	return values
}

// resolveCreateStandardFieldValue 解析上下文数据，得到后续流程需要的结果。
func resolveCreateStandardFieldValue(
	key string,
	envCode string,
	projectName string,
	gitRef string,
	imageTag string,
	releaseName string,
	appKey string,
	artifactValues map[string]string,
	resolved map[domain.PipelineScope]map[string]string,
) string {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return ""
	}
	pickResolved := func(keys ...string) string {
		for _, scope := range []domain.PipelineScope{domain.PipelineScopeCD, domain.PipelineScopeCI} {
			for _, candidate := range keys {
				if value := strings.TrimSpace(resolved[scope][strings.ToLower(strings.TrimSpace(candidate))]); value != "" {
					return value
				}
			}
		}
		return ""
	}
	switch normalizedKey {
	case "env", "env_code":
		return firstNonEmpty(pickResolved("env", "env_code"), envCode)
	case "project_name":
		return firstNonEmpty(pickResolved("project_name"), projectName)
	case "release_name":
		return firstNonEmpty(pickResolved("release_name"), releaseName)
	case "branch", "git_ref":
		return firstNonEmpty(pickResolved("branch", "git_ref"), gitRef)
	case "image_version", "image_tag":
		return firstNonEmpty(pickResolved("image_version", "image_tag"), imageTag)
	case "app_key":
		return firstNonEmpty(pickResolved("app_key"), appKey)
	case standardParamGOSArtifactURL:
		return strings.TrimSpace(resolved[domain.PipelineScopeCI][standardParamGOSArtifactURL])
	case standardParamGOSArtifactPath:
		return strings.TrimSpace(artifactValues[standardParamGOSArtifactPath])
	case "oss_endpoint", "oss_bucket", "oss_dir", "oss_acl", "oss_access_key_id", "oss_access_key_secret":
		return firstNonEmpty(pickResolved(normalizedKey), strings.TrimSpace(artifactValues[normalizedKey]))
	default:
		return pickResolved(normalizedKey)
	}
}

// resolveTemplateForCreate 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveTemplateForCreate(
	ctx context.Context,
	applicationID string,
	templateID string,
) (domain.ReleaseTemplate, []domain.ReleaseTemplateBinding, []domain.ReleaseTemplateParam, []domain.ReleaseTemplateHook, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return domain.ReleaseTemplate{}, nil, nil, nil, fmt.Errorf("%w: template_id is required", ErrInvalidInput)
	}
	template, templateBindings, templateParams, _, templateHooks, err := uc.repo.GetTemplateByID(ctx, templateID)
	if err != nil {
		return domain.ReleaseTemplate{}, nil, nil, nil, err
	}
	if template.Status != domain.TemplateStatusActive {
		return domain.ReleaseTemplate{}, nil, nil, nil, fmt.Errorf("%w: release template is disabled", ErrInvalidInput)
	}
	if strings.TrimSpace(template.ApplicationID) != strings.TrimSpace(applicationID) {
		return domain.ReleaseTemplate{}, nil, nil, nil, fmt.Errorf("%w: release template does not belong to application", ErrInvalidInput)
	}
	if len(templateBindings) == 0 {
		return domain.ReleaseTemplate{}, nil, nil, nil, fmt.Errorf("%w: release template has no enabled pipeline scopes", ErrInvalidInput)
	}
	compliance := evaluateReleaseTemplateCompliance(ctx, uc.pipelineScanRepo, templateBindings)
	if compliance.Status == domain.TemplateComplianceStatusViolated {
		return domain.ReleaseTemplate{}, nil, nil, nil, fmt.Errorf("%w: 模板违反管线规范，%s", ErrInvalidInput, compliance.Summary)
	}
	return template, templateBindings, templateParams, templateHooks, nil
}

type releaseOrderSummaryFields struct {
	EnvCode     string
	ProjectName string
	GitRef      string
	ImageTag    string
}

func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
}

// resolveReleaseOrderSummaryFields 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseOrderSummaryFields(params []CreateReleaseOrderParamInput) releaseOrderSummaryFields {
	result := releaseOrderSummaryFields{}
	for _, item := range params {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		value := strings.TrimSpace(item.ParamValue)
		if value == "" {
			continue
		}
		switch key {
		case "env", "env_code":
			if result.EnvCode == "" {
				result.EnvCode = value
			}
		case "project_name":
			if result.ProjectName == "" {
				result.ProjectName = value
			}
		case "git_ref":
			if result.GitRef == "" {
				result.GitRef = value
			}
		case "image_tag":
			if result.ImageTag == "" {
				result.ImageTag = value
			}
		case "image_version":
			if result.ImageTag == "" {
				result.ImageTag = value
			}
		}
	}
	return result
}

// firstNonEmpty 封装当前模块的业务处理逻辑。
func firstNonEmpty(values ...string) string {
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value != "" {
			return value
		}
	}
	return ""
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func summarizeNames(values []string, limit int) string {
	items := uniqueNonEmptyStrings(values)
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || len(items) <= limit {
		return strings.Join(items, ",")
	}
	return fmt.Sprintf("%s 等 %d 个", strings.Join(items[:limit], ","), len(items))
}

// buildRollbackRemark 组装业务执行所需的输入数据。
func buildRollbackRemark(source domain.ReleaseOrder) string {
	orderNo := strings.TrimSpace(source.OrderNo)
	if orderNo == "" {
		return "重发创建"
	}
	return fmt.Sprintf("重发自发布单 %s", orderNo)
}

// buildReplayRemark 组装业务执行所需的输入数据。
func buildReplayRemark(source domain.ReleaseOrder, scope domain.PipelineScope) string {
	orderNo := strings.TrimSpace(source.OrderNo)
	scopeLabel := strings.ToUpper(string(scope))
	if scopeLabel == "" {
		scopeLabel = "参数"
	}
	if orderNo == "" {
		return scopeLabel + " 参数重放创建"
	}
	return fmt.Sprintf("按发布单 %s 的 %s 参数重放", orderNo, scopeLabel)
}

// loadRecoverySourceOrder 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadRecoverySourceOrder(
	ctx context.Context,
	sourceOrderID string,
) (domain.ReleaseOrder, []domain.ReleaseOrderExecution, error) {
	if sourceOrderID == "" {
		return domain.ReleaseOrder{}, nil, fmt.Errorf("%w: source_order_id is required", ErrInvalidInput)
	}
	sourceOrder, err := uc.repo.GetByID(ctx, sourceOrderID)
	if err != nil {
		return domain.ReleaseOrder{}, nil, err
	}
	sourceExecutions, err := uc.repo.ListExecutions(ctx, sourceOrder.ID)
	if err != nil {
		return domain.ReleaseOrder{}, nil, err
	}
	return sourceOrder, sourceExecutions, nil
}

// canCreateArgoReplayFromStatus 创建业务资源并返回处理结果。
func canCreateArgoReplayFromStatus(status domain.OrderStatus) bool {
	switch status {
	case domain.OrderStatusPending,
		domain.OrderStatusDraft,
		domain.OrderStatusPendingApproval,
		domain.OrderStatusApproving,
		domain.OrderStatusApproved:
		return false
	default:
		return true
	}
}

// canCreatePipelineReplayFromStatus 创建业务资源并返回处理结果。
func canCreatePipelineReplayFromStatus(status domain.OrderStatus) bool {
	switch status {
	case domain.OrderStatusSuccess,
		domain.OrderStatusDeploySuccess,
		domain.OrderStatusFailed,
		domain.OrderStatusDeployFailed:
		return true
	default:
		return false
	}
}

// ensureReplaySourceOrderCanReplay 校验标准重放来源单。
func ensureReplaySourceOrderCanReplay(sourceOrder domain.ReleaseOrder) error {
	if sourceOrder.OperationType == domain.OperationTypeReplay {
		return fmt.Errorf("%w: 重放单不支持再次重放，继续重发请从原始单发起", ErrInvalidInput)
	}
	return nil
}

// ensureRollbackDeploySnapshot 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureRollbackDeploySnapshot(
	ctx context.Context,
	sourceOrder domain.ReleaseOrder,
	sourceExecutions []domain.ReleaseOrderExecution,
) (domain.DeploySnapshot, error) {
	snapshot, err := uc.repo.GetDeploySnapshotByOrderID(ctx, sourceOrder.ID)
	if err == nil {
		return snapshot, nil
	}
	if !errors.Is(err, domain.ErrDeploySnapshotNotFound) {
		return domain.DeploySnapshot{}, err
	}

	sourceCDExecution, execErr := resolveCDExecution(sourceExecutions)
	if execErr != nil {
		return domain.DeploySnapshot{}, execErr
	}
	if !isArgoCDExecution(sourceCDExecution) {
		return domain.DeploySnapshot{}, fmt.Errorf("%w: 当前成功单不支持标准回滚", ErrInvalidInput)
	}

	sourceParams, paramErr := uc.repo.ListParams(ctx, sourceOrder.ID)
	if paramErr != nil {
		return domain.DeploySnapshot{}, paramErr
	}
	template, _, _, templateGitOpsRules, _, templateErr := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(sourceOrder.TemplateID))
	if templateErr != nil {
		return domain.DeploySnapshot{}, templateErr
	}
	gitopsType := normalizeTemplateGitOpsType(template.GitOpsType, true)
	if gitopsType != domain.GitOpsTypeHelm {
		return domain.DeploySnapshot{}, fmt.Errorf("%w: 当前成功单仅支持 Helm 标准回滚", ErrInvalidInput)
	}

	binding, argocdInstance, client, contextErr := uc.resolveArgoCDExecutionContext(ctx, sourceOrder, sourceCDExecution, sourceParams)
	if contextErr != nil {
		return domain.DeploySnapshot{}, contextErr
	}
	gitopsService, gitopsErr := uc.resolveGitOpsService(ctx, argocdInstance)
	if gitopsErr != nil {
		return domain.DeploySnapshot{}, gitopsErr
	}

	appKey := ""
	appArtifactPath := ""
	if uc.appRepo != nil {
		if appRecord, appErr := uc.appRepo.GetByID(ctx, strings.TrimSpace(sourceOrder.ApplicationID)); appErr == nil {
			appKey = strings.TrimSpace(appRecord.Key)
			appArtifactPath = strings.TrimSpace(appRecord.ArtifactDirectory)
		}
	}
	environment := uc.resolveArgoCDEnvironment(sourceOrder, sourceParams)
	appName, app, appErr := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, environment, gitopsType)
	if appErr != nil {
		return domain.DeploySnapshot{}, fmt.Errorf("%w: get argocd application failed: %v", ErrInvalidInput, appErr)
	}
	repoURL := strings.TrimSpace(app.GetRepoURL())
	sourcePath := strings.TrimSpace(app.GetSourcePath())
	if repoURL == "" || sourcePath == "" {
		return domain.DeploySnapshot{}, fmt.Errorf("%w: argocd application source repo/path is incomplete", ErrInvalidInput)
	}
	imageVersion := uc.resolveArgoCDImageVersion(sourceOrder, sourceParams, sourceExecutions)
	if imageVersion == "" {
		return domain.DeploySnapshot{}, fmt.Errorf("%w: image_version is required when rebuilding deploy snapshot", ErrInvalidInput)
	}
	commitFields := buildGitOpsCommitMessageFields(sourceOrder, sourceParams, appKey, environment, imageVersion, sourcePath, appArtifactPath)
	valuesRules, rulesErr := uc.buildArgoCDValuesRules(gitopsService, templateGitOpsRules, commitFields)
	if rulesErr != nil {
		return domain.DeploySnapshot{}, rulesErr
	}
	if saveErr := uc.saveHelmDeploySnapshot(
		ctx,
		sourceOrder,
		argocdInstance,
		appName,
		repoURL,
		uc.resolveGitOpsTargetBranch(ctx, sourceOrder, sourceParams, argocdInstance, app),
		sourcePath,
		environment,
		imageVersion,
		valuesRules,
	); saveErr != nil {
		return domain.DeploySnapshot{}, saveErr
	}
	return uc.repo.GetDeploySnapshotByOrderID(ctx, sourceOrder.ID)
}

// createRecoveryOrder 创建业务资源并返回处理结果。
func (uc *ReleaseOrderManager) createRecoveryOrder(
	ctx context.Context,
	sourceOrder domain.ReleaseOrder,
	sourceParams []domain.ReleaseOrderParam,
	template domain.ReleaseTemplate,
	templateHooks []domain.ReleaseTemplateHook,
	targetBinding domain.ReleaseTemplateBinding,
	targetScope domain.PipelineScope,
	paramsInput []CreateReleaseOrderParamInput,
	operationType domain.OperationType,
	remark string,
	creatorUserID string,
	triggeredBy string,
) (domain.ReleaseOrder, error) {
	now := uc.now()
	executions := uc.buildCreateExecutions("", now, []domain.ReleaseTemplateBinding{targetBinding})
	primaryExecution, ok := pickPrimaryExecution(executions)
	if !ok {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: 当前模板未配置可用执行单元", ErrInvalidInput)
	}

	autoApproved := shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, strings.TrimSpace(creatorUserID))
	initialStatus := resolveInitialReleaseOrderStatus(template, strings.TrimSpace(creatorUserID))
	var approvedAt *time.Time
	approvedBy := ""
	if autoApproved {
		approvedAt = &now
		approvedBy = firstNonEmpty(strings.TrimSpace(triggeredBy), strings.TrimSpace(creatorUserID))
	}

	order := domain.ReleaseOrder{
		ID:                    generateID("ro"),
		OrderNo:               generateOrderNo(now),
		ReleaseName:           sourceOrder.ReleaseName,
		PreviousOrderNo:       sourceOrder.OrderNo,
		OperationType:         operationType,
		SourceOrderID:         sourceOrder.ID,
		SourceOrderNo:         sourceOrder.OrderNo,
		ApplicationID:         sourceOrder.ApplicationID,
		ApplicationName:       firstNonEmpty(template.ApplicationName, sourceOrder.ApplicationName),
		TemplateID:            sourceOrder.TemplateID,
		TemplateName:          firstNonEmpty(template.Name, sourceOrder.TemplateName),
		BindingID:             primaryExecution.BindingID,
		PipelineID:            primaryExecution.PipelineID,
		EnvCode:               sourceOrder.EnvCode,
		GitRef:                sourceOrder.GitRef,
		ImageTag:              sourceOrder.ImageTag,
		TriggerType:           domain.TriggerTypeManual,
		Status:                initialStatus,
		ApprovalRequired:      template.ApprovalEnabled,
		ApprovalMode:          template.ApprovalMode,
		ApprovalApproverIDs:   append([]string(nil), template.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), template.ApprovalApproverNames...),
		ApprovedAt:            approvedAt,
		ApprovedBy:            approvedBy,
		Remark:                strings.TrimSpace(remark),
		CreatorUserID:         creatorUserID,
		TriggeredBy:           triggeredBy,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	executions = uc.buildCreateExecutions(order.ID, now, []domain.ReleaseTemplateBinding{targetBinding})
	paramsInput, err := uc.buildRecoveryParamsInput(ctx, sourceOrder, sourceParams, targetScope, paramsInput)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	params, err := uc.buildCreateParams(order.ID, now, paramsInput, executions)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	steps, err := uc.buildCreateSteps(
		order.ID,
		now,
		executions,
		normalizeTemplateGitOpsType(template.GitOpsType, true),
		templateHooks,
		nil,
		order.EnvCode,
	)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	if err := uc.repo.Create(ctx, order, executions, params, steps); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if autoApproved {
		if err := uc.repo.CreateApprovalRecord(ctx, domain.ReleaseOrderApprovalRecord{
			ID:             generateID("rapr"),
			ReleaseOrderID: order.ID,
			Action:         domain.ReleaseOrderApprovalActionApprove,
			OperatorUserID: strings.TrimSpace(creatorUserID),
			OperatorName:   approvedBy,
			Comment:        "发起人即审批人，系统已自动通过审批",
			CreatedAt:      now,
		}); err != nil {
			return domain.ReleaseOrder{}, err
		}
	}
	return uc.repo.GetByID(ctx, order.ID)
}

// buildRecoveryParamsInput 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildRecoveryParamsInput(
	ctx context.Context,
	sourceOrder domain.ReleaseOrder,
	sourceParams []domain.ReleaseOrderParam,
	targetScope domain.PipelineScope,
	base []CreateReleaseOrderParamInput,
) ([]CreateReleaseOrderParamInput, error) {
	result := append([]CreateReleaseOrderParamInput(nil), base...)
	if uc == nil || uc.platformRepo == nil {
		return result, nil
	}

	status := platformparamdomain.StatusEnabled
	items, _, err := uc.platformRepo.List(ctx, platformparamdomain.ListFilter{
		Status:   &status,
		Page:     1,
		PageSize: 1000,
	})
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		allowed[key] = struct{}{}
	}
	if len(allowed) == 0 {
		return result, nil
	}

	existing := make(map[string]struct{}, len(result))
	for _, item := range result {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		existing[key] = struct{}{}
	}
	appendParam := func(key string, value string, source domain.ValueSource, executorParamName string) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		normalizedKey := strings.ToLower(key)
		if normalizedKey == "" || value == "" {
			return
		}
		if _, ok := allowed[normalizedKey]; !ok {
			return
		}
		if _, ok := existing[normalizedKey]; ok {
			return
		}
		if source == "" {
			source = domain.ValueSourceReleaseInput
		}
		result = append(result, CreateReleaseOrderParamInput{
			PipelineScope:     targetScope,
			ParamKey:          key,
			ExecutorParamName: strings.TrimSpace(executorParamName),
			ParamValue:        value,
			ValueSource:       source,
		})
		existing[normalizedKey] = struct{}{}
	}

	for _, item := range sourceParams {
		appendParam(item.ParamKey, item.ParamValue, item.ValueSource, item.ExecutorParamName)
	}

	appendParam("env", sourceOrder.EnvCode, domain.ValueSourceEnvironment, "")
	appendParam("env_code", sourceOrder.EnvCode, domain.ValueSourceEnvironment, "")
	appendParam("release_name", sourceOrder.ReleaseName, domain.ValueSourceBuiltin, "")
	appendParam("branch", sourceOrder.GitRef, domain.ValueSourceReleaseInput, "")
	appendParam("git_ref", sourceOrder.GitRef, domain.ValueSourceReleaseInput, "")
	appendParam("image_version", sourceOrder.ImageTag, domain.ValueSourceReleaseInput, "")
	appendParam("image_tag", sourceOrder.ImageTag, domain.ValueSourceReleaseInput, "")
	if uc.appRepo != nil && strings.TrimSpace(sourceOrder.ApplicationID) != "" {
		if appRecord, appErr := uc.appRepo.GetByID(ctx, strings.TrimSpace(sourceOrder.ApplicationID)); appErr == nil {
			appendParam("app_key", appRecord.Key, domain.ValueSourceApplication, "")
		}
	}
	return result, nil
}

// filterReleaseOrderParamsByScope 封装当前模块的业务处理逻辑。
func filterReleaseOrderParamsByScope(items []domain.ReleaseOrderParam, scope domain.PipelineScope) []domain.ReleaseOrderParam {
	filtered := make([]domain.ReleaseOrderParam, 0)
	for _, item := range items {
		if item.PipelineScope != scope {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// selectRecoveryTemplateBinding 封装当前模块的业务处理逻辑。
func selectRecoveryTemplateBinding(
	bindings []domain.ReleaseTemplateBinding,
	scope domain.PipelineScope,
) (domain.ReleaseTemplateBinding, bool) {
	for _, item := range bindings {
		if !item.Enabled || item.PipelineScope != scope {
			continue
		}
		return item, true
	}
	return domain.ReleaseTemplateBinding{}, false
}

// resolveCDExecution 解析上下文数据，得到后续流程需要的结果。
func resolveCDExecution(items []domain.ReleaseOrderExecution) (domain.ReleaseOrderExecution, error) {
	for _, item := range items {
		if item.PipelineScope == domain.PipelineScopeCD {
			return item, nil
		}
	}
	return domain.ReleaseOrderExecution{}, fmt.Errorf("%w: 来源成功单缺少 CD 执行单元", ErrInvalidInput)
}

// resolveReplayExecution 解析上下文数据，得到后续流程需要的结果。
func resolveReplayExecution(items []domain.ReleaseOrderExecution) (domain.ReleaseOrderExecution, error) {
	for _, item := range items {
		if item.PipelineScope == domain.PipelineScopeCD {
			return item, nil
		}
	}
	for _, item := range items {
		if item.PipelineScope == domain.PipelineScopeCI {
			return item, nil
		}
	}
	return domain.ReleaseOrderExecution{}, fmt.Errorf("%w: 来源发布单缺少可重放执行单元", ErrInvalidInput)
}

// ensureReplayParamsMatchTemplate 校验前置条件，不满足时写入对应错误响应。
func ensureReplayParamsMatchTemplate(
	templateParams []domain.ReleaseTemplateParam,
	sourceScopeParams []domain.ReleaseOrderParam,
	scope domain.PipelineScope,
) error {
	allowed := make(map[string]releasedTemplateParamRule)
	allowedByParamKey := make(map[string]releasedTemplateParamRule)
	duplicateParamKeys := make(map[string]struct{})
	for _, item := range templateParams {
		if item.PipelineScope != scope {
			continue
		}
		key := buildReleaseTemplateParamKey(item.PipelineScope, item.ParamKey, item.ExecutorParamName)
		rule := releasedTemplateParamRule{
			PipelineScope:     item.PipelineScope,
			ParamKey:          strings.ToLower(strings.TrimSpace(item.ParamKey)),
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			Required:          item.Required,
		}
		allowed[key] = rule
		paramKey := buildReleaseTemplateScopeParamKey(item.PipelineScope, item.ParamKey)
		if paramKey == "" {
			continue
		}
		if _, exists := allowedByParamKey[paramKey]; exists {
			delete(allowedByParamKey, paramKey)
			duplicateParamKeys[paramKey] = struct{}{}
			continue
		}
		if _, duplicated := duplicateParamKeys[paramKey]; duplicated {
			continue
		}
		allowedByParamKey[paramKey] = rule
	}

	for _, item := range sourceScopeParams {
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		executorParamName := strings.TrimSpace(item.ExecutorParamName)
		if _, _, ok := resolveReleaseTemplateRule(allowed, allowedByParamKey, scope, paramKey, executorParamName); ok {
			continue
		}
		return fmt.Errorf("%w: 当前模板已不再包含 %s 参数 %s，无法执行参数重放", ErrInvalidInput, strings.ToUpper(string(scope)), executorParamNameOrKey(executorParamName, paramKey))
	}
	return nil
}

// List 查询并返回列表数据。
func (uc *ReleaseOrderManager) List(ctx context.Context, input ListReleaseOrderInput) ([]domain.ReleaseOrder, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.Keyword = strings.TrimSpace(input.Keyword)
	input.ConcurrentBatchNo = strings.TrimSpace(input.ConcurrentBatchNo)
	input.ConcurrentBatchName = strings.TrimSpace(input.ConcurrentBatchName)
	input.TriggeredBy = strings.TrimSpace(input.TriggeredBy)
	input.BindingID = strings.TrimSpace(input.BindingID)
	input.EnvCode = strings.TrimSpace(input.EnvCode)
	if input.OperationType != "" && !input.OperationType.Valid() {
		return nil, 0, ErrInvalidInput
	}
	if input.Status != "" && !input.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if input.TriggerType != "" && !input.TriggerType.Valid() {
		return nil, 0, ErrInvalidInput
	}
	if input.CreatedAtFrom != nil && input.CreatedAtTo != nil && input.CreatedAtTo.Before(*input.CreatedAtFrom) {
		return nil, 0, ErrInvalidInput
	}
	if input.Page <= 0 {
		input.Page = defaultPage
	}
	if input.PageSize <= 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageSize > maxPageSize {
		input.PageSize = maxPageSize
	}

	items, total, err := uc.repo.List(ctx, domain.ListFilter{
		ApplicationID:               input.ApplicationID,
		ApplicationIDs:              normalizeReleaseApplicationIDs(input.ApplicationIDs),
		VisibleApplicationEnvScopes: normalizeReleaseApplicationEnvScopes(input.VisibleApplicationEnvScopes),
		VisibleToUserID:             strings.TrimSpace(input.VisibleToUserID),
		ApprovalApproverUserID:      strings.TrimSpace(input.ApprovalApproverUserID),
		CreatorUserID:               strings.TrimSpace(input.CreatorUserID),
		Keyword:                     input.Keyword,
		ConcurrentBatchNo:           input.ConcurrentBatchNo,
		ConcurrentBatchName:         input.ConcurrentBatchName,
		TriggeredBy:                 input.TriggeredBy,
		BindingID:                   input.BindingID,
		EnvCode:                     input.EnvCode,
		OperationType:               input.OperationType,
		Status:                      input.Status,
		TriggerType:                 input.TriggerType,
		CreatedAtFrom:               input.CreatedAtFrom,
		CreatedAtTo:                 input.CreatedAtTo,
		Page:                        input.Page,
		PageSize:                    input.PageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	// 列表查询必须保持只读。订单状态只能由显式调度动作或后台 tracker 推进，
	// 否则一次普通页面刷新可能在发布后 Hook 尚未完成时提前把整单写成成功。
	return items, total, nil
}

// ListStats 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListStats(ctx context.Context, input ListReleaseOrderInput) (domain.ReleaseOrderStats, error) {
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.Keyword = strings.TrimSpace(input.Keyword)
	input.ConcurrentBatchNo = strings.TrimSpace(input.ConcurrentBatchNo)
	input.ConcurrentBatchName = strings.TrimSpace(input.ConcurrentBatchName)
	input.TriggeredBy = strings.TrimSpace(input.TriggeredBy)
	input.BindingID = strings.TrimSpace(input.BindingID)
	input.EnvCode = strings.TrimSpace(input.EnvCode)
	if input.OperationType != "" && !input.OperationType.Valid() {
		return domain.ReleaseOrderStats{}, ErrInvalidInput
	}
	if input.Status != "" && !input.Status.Valid() {
		return domain.ReleaseOrderStats{}, ErrInvalidStatus
	}
	if input.TriggerType != "" && !input.TriggerType.Valid() {
		return domain.ReleaseOrderStats{}, ErrInvalidInput
	}
	if input.CreatedAtFrom != nil && input.CreatedAtTo != nil && input.CreatedAtTo.Before(*input.CreatedAtFrom) {
		return domain.ReleaseOrderStats{}, ErrInvalidInput
	}
	return uc.repo.ListStats(ctx, domain.ListFilter{
		ApplicationID:               input.ApplicationID,
		ApplicationIDs:              normalizeReleaseApplicationIDs(input.ApplicationIDs),
		VisibleApplicationEnvScopes: normalizeReleaseApplicationEnvScopes(input.VisibleApplicationEnvScopes),
		VisibleToUserID:             strings.TrimSpace(input.VisibleToUserID),
		ApprovalApproverUserID:      strings.TrimSpace(input.ApprovalApproverUserID),
		CreatorUserID:               strings.TrimSpace(input.CreatorUserID),
		Keyword:                     input.Keyword,
		ConcurrentBatchNo:           input.ConcurrentBatchNo,
		ConcurrentBatchName:         input.ConcurrentBatchName,
		TriggeredBy:                 input.TriggeredBy,
		BindingID:                   input.BindingID,
		EnvCode:                     input.EnvCode,
		OperationType:               input.OperationType,
		Status:                      input.Status,
		TriggerType:                 input.TriggerType,
		CreatedAtFrom:               input.CreatedAtFrom,
		CreatedAtTo:                 input.CreatedAtTo,
	})
}

// normalizeReleaseApplicationIDs 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseApplicationIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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
	if len(result) == 0 {
		return nil
	}
	return result
}

// normalizeReleaseApplicationEnvScopes 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseApplicationEnvScopes(values []domain.ApplicationEnvScope) []domain.ApplicationEnvScope {
	if len(values) == 0 {
		return nil
	}
	result := make([]domain.ApplicationEnvScope, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		applicationID := strings.TrimSpace(item.ApplicationID)
		envCode := strings.TrimSpace(item.EnvCode)
		if applicationID == "" || envCode == "" {
			continue
		}
		key := applicationID + "::" + envCode
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, domain.ApplicationEnvScope{
			ApplicationID: applicationID,
			EnvCode:       envCode,
		})
	}
	return result
}

// GetByID 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) GetByID(ctx context.Context, id string) (domain.ReleaseOrder, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	// 详情查询必须保持只读，状态收敛由 TrackReleaseExecution 统一负责。
	return order, nil
}

// ListDeploySnapshotsByOrderID 查询并返回指定资源数据。
func (uc *ReleaseOrderManager) ListDeploySnapshotsByOrderID(ctx context.Context, releaseOrderID string) ([]domain.DeploySnapshot, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	releaseOrderID = strings.TrimSpace(releaseOrderID)
	if releaseOrderID == "" {
		return nil, ErrInvalidID
	}
	return uc.repo.ListDeploySnapshotsByOrderID(ctx, releaseOrderID)
}

// Cancel 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) Cancel(ctx context.Context, id string) (domain.ReleaseOrder, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	unlock := uc.lockOrderOperation(id)
	defer unlock()
	logx.Info("release_order", "cancel_start", logx.F("order_id", id))

	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		logx.Error("release_order", "cancel_failed", err, logx.F("order_id", id))
		return domain.ReleaseOrder{}, err
	}

	switch order.Status {
	case domain.OrderStatusPending,
		domain.OrderStatusRunning,
		domain.OrderStatusQueued,
		domain.OrderStatusBuilding,
		domain.OrderStatusBuiltWaitingDeploy,
		domain.OrderStatusDeploying,
		domain.OrderStatusApproved,
		domain.OrderStatusPendingApproval,
		domain.OrderStatusApproving:
		// allowed
	default:
		err := fmt.Errorf("%w: release order cannot be cancelled in current status", ErrInvalidInput)
		logx.Warn("release_order", "cancel_failed",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("status", order.Status),
			logx.F("reason", err.Error()),
		)
		return domain.ReleaseOrder{}, err
	}

	now := uc.now()
	steps, err := uc.repo.ListSteps(ctx, id)
	if err != nil {
		logx.Error("release_order", "cancel_failed", err, logx.F("order_id", order.ID))
		return domain.ReleaseOrder{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, id)
	if err != nil {
		logx.Error("release_order", "cancel_failed", err, logx.F("order_id", order.ID))
		return domain.ReleaseOrder{}, err
	}

	cancelNotes := make([]string, 0)
	for _, execution := range executions {
		note := uc.abortExecution(ctx, execution)
		if note != "" {
			cancelNotes = append(cancelNotes, note)
		}
		if execution.Status.IsTerminal() {
			continue
		}
		_, updateErr := uc.repo.UpdateExecutionByScope(ctx, id, execution.PipelineScope, domain.ExecutionUpdateInput{
			Status:     domain.ExecutionStatusCancelled,
			QueueURL:   execution.QueueURL,
			BuildURL:   execution.BuildURL,
			StartedAt:  execution.StartedAt,
			FinishedAt: &now,
			UpdatedAt:  now,
		})
		if updateErr != nil && !errors.Is(updateErr, domain.ErrExecutionNotFound) {
			logx.Error("release_order", "cancel_failed", updateErr,
				logx.F("order_id", order.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
			)
			return domain.ReleaseOrder{}, updateErr
		}
	}

	for _, step := range steps {
		if !shouldFinishStepOnCancel(step) {
			continue
		}
		startedAt := step.StartedAt
		if startedAt == nil {
			startedAt = &now
		}
		message := "发布已取消"
		if len(cancelNotes) > 0 {
			message = message + "，" + strings.Join(cancelNotes, "；")
		}
		_, updateErr := uc.repo.UpdateStep(ctx, id, step.StepCode, domain.StepUpdateInput{
			Status:     domain.StepStatusFailed,
			Message:    message,
			StartedAt:  startedAt,
			FinishedAt: &now,
		})
		if updateErr != nil && !errors.Is(updateErr, domain.ErrStepNotFound) {
			logx.Error("release_order", "cancel_failed", updateErr,
				logx.F("order_id", order.ID),
				logx.F("step_code", step.StepCode),
			)
			return domain.ReleaseOrder{}, updateErr
		}
	}

	item, err := uc.repo.UpdateStatus(ctx, id, domain.OrderStatusCancelled, order.StartedAt, &now, now)
	if err != nil {
		logx.Error("release_order", "cancel_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if err := uc.releaseExecutionLocks(ctx, id, domain.ExecutionLockStatusReleased); err != nil {
		logx.Error("release_order", "cancel_release_lock_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
	}
	logx.Info("release_order", "cancel_success",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("cancel_notes_count", len(cancelNotes)),
	)
	return item, nil
}

// shouldFinishStepOnCancel 封装当前模块的业务处理逻辑。
func shouldFinishStepOnCancel(step domain.ReleaseOrderStep) bool {
	if step.Status == domain.StepStatusRunning {
		return true
	}
	if step.Status != domain.StepStatusPending {
		return false
	}
	return step.StepScope != domain.StepScopeGlobal || step.StepCode == "global:release_finish"
}

// shouldFinishStepOnFailure 封装当前模块的业务处理逻辑。
func shouldFinishStepOnFailure(step domain.ReleaseOrderStep) bool {
	if strings.HasPrefix(strings.TrimSpace(step.StepCode), "hook:") {
		return false
	}
	if step.Status == domain.StepStatusRunning {
		return true
	}
	if step.Status != domain.StepStatusPending {
		return false
	}
	return step.StepScope != domain.StepScopeGlobal || step.StepCode == "global:release_finish"
}

// abortExecution 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) abortExecution(ctx context.Context, execution domain.ReleaseOrderExecution) string {
	if uc.jenkins == nil {
		return ""
	}
	if strings.ToLower(strings.TrimSpace(execution.Provider)) != string(pipelinedomain.ProviderJenkins) {
		return ""
	}
	queueURL := strings.TrimSpace(execution.QueueURL)
	buildURL := strings.TrimSpace(execution.BuildURL)
	if queueURL == "" && buildURL == "" {
		return ""
	}
	if buildURL != "" {
		if err := uc.jenkins.AbortBuild(ctx, buildURL); err != nil {
			return "尝试停止 Jenkins 构建失败"
		}
		return "已发送 Jenkins 停止构建请求"
	}
	if err := uc.jenkins.AbortQueueItem(ctx, queueURL); err != nil {
		return "尝试取消 Jenkins 队列失败"
	}
	return "已发送 Jenkins 取消队列请求"
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) Execute(ctx context.Context, id string, operatorUserID string, operatorName string) (domain.ReleaseOrder, error) {
	return uc.dispatchOrder(ctx, id, ReleaseOrderDispatchActionExecute, operatorUserID, operatorName)
}

// Build 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) Build(ctx context.Context, id string, operatorUserID string, operatorName string) (domain.ReleaseOrder, error) {
	return uc.dispatchOrder(ctx, id, ReleaseOrderDispatchActionBuild, operatorUserID, operatorName)
}

// Deploy 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) Deploy(ctx context.Context, id string, operatorUserID string, operatorName string) (domain.ReleaseOrder, error) {
	return uc.dispatchOrder(ctx, id, ReleaseOrderDispatchActionDeploy, operatorUserID, operatorName)
}

// dispatchOrder 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) dispatchOrder(
	ctx context.Context,
	id string,
	action ReleaseOrderDispatchAction,
	operatorUserID string,
	operatorName string,
) (domain.ReleaseOrder, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	unlock := uc.lockOrderOperation(id)
	defer unlock()
	logx.Info("release_order", "execute_start",
		logx.F("order_id", id),
		logx.F("action", action),
	)
	if uc.jenkins == nil && uc.argocdFactory == nil {
		err := fmt.Errorf("%w: release executor is not configured", ErrInvalidInput)
		logx.Error("release_order", "execute_failed", err, logx.F("order_id", id))
		return domain.ReleaseOrder{}, err
	}

	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		logx.Error("release_order", "execute_failed", err, logx.F("order_id", id))
		return domain.ReleaseOrder{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		logx.Error("release_order", "execute_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if len(executions) == 0 {
		err := fmt.Errorf("%w: release order has no executions", ErrInvalidInput)
		logx.Error("release_order", "execute_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	orderParams, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		logx.Error("release_order", "execute_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}

	precheck, err := uc.buildOrderPrecheck(ctx, order, executions, orderParams, action)
	if err != nil {
		logx.Error("release_order", "execute_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return domain.ReleaseOrder{}, err
	}
	if !precheck.Executable {
		reason := strings.TrimSpace(precheck.ConflictMessage)
		if reason == "" {
			for _, item := range precheck.Items {
				if item.Status == ReleaseOrderPrecheckItemStatusBlocked {
					reason = strings.TrimSpace(item.Message)
					break
				}
			}
		}
		if reason == "" {
			reason = currentDispatchBlockedMessage(action)
		}
		err := fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, reason)
		logx.Warn("release_order", "execute_failed",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("reason", reason),
		)
		return domain.ReleaseOrder{}, err
	}
	if err := uc.ensureApprovalFlowDispatchAllowedForExecutions(ctx, order.ID, action, executions); err != nil {
		if errors.Is(err, ErrApprovalFlowPending) {
			now := uc.now()
			updatedOrder, updateErr := uc.repo.UpdateApprovalStatus(
				ctx,
				order.ID,
				domain.OrderStatusPendingApproval,
				nil,
				"",
				nil,
				"",
				"",
				now,
			)
			if updateErr != nil {
				return domain.ReleaseOrder{}, updateErr
			}
			if strings.TrimSpace(operatorUserID) != "" || strings.TrimSpace(operatorName) != "" {
				updatedOrder, updateErr = uc.repo.UpdateExecutor(ctx, order.ID, strings.TrimSpace(operatorUserID), strings.TrimSpace(operatorName), now)
				if updateErr != nil {
					return domain.ReleaseOrder{}, updateErr
				}
			}
			logx.Info("release_order", "approval_flow_started",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("action", action),
			)
			return updatedOrder, nil
		}
		return domain.ReleaseOrder{}, err
	}

	stageFullRelease, err := uc.shouldStageGraphFullRelease(ctx, order.ID, action, executions)
	if err != nil {
		return domain.ReleaseOrder{}, err
	}
	pendingExecution, dispatchStatus, err := resolveDispatchExecution(order, executions, action, stageFullRelease)
	if err != nil {
		logx.Warn("release_order", "execute_failed",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("reason", err.Error()),
			logx.F("action", action),
		)
		return domain.ReleaseOrder{}, err
	}

	var dispatchStartedAt *time.Time
	var dispatchGuard releaseDispatchGuard
	if pendingExecution != nil {
		var acquired bool
		var guardErr error
		dispatchGuard, acquired, guardErr = uc.ensureExecutionLock(ctx, order, *pendingExecution, orderParams)
		if guardErr != nil {
			logx.Warn("release_order", "execute_failed",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("reason", guardErr.Error()),
			)
			return domain.ReleaseOrder{}, guardErr
		}
		if acquired {
			startedAt := uc.now()
			dispatchStartedAt = firstNonNilTime(order.StartedAt, &startedAt)
		} else {
			dispatchStartedAt = order.StartedAt
			// 未取得锁时统一使用明确的 queued 状态。tracker 真正领取 CI 后会切回 building。
			dispatchStatus = domain.OrderStatusQueued
		}
	}
	updatedAt := uc.now()
	updatedOrder, err := uc.repo.UpdateStatus(ctx, order.ID, dispatchStatus, dispatchStartedAt, nil, updatedAt)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			logx.Warn("release_order", "execute_status_reload_failed",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("action", action),
				logx.F("status", dispatchStatus),
			)
			order.Status = dispatchStatus
			order.StartedAt = dispatchStartedAt
			order.FinishedAt = nil
			order.UpdatedAt = updatedAt
		} else {
			if dispatchStartedAt != nil {
				_ = uc.releaseExecutionLocks(ctx, order.ID, domain.ExecutionLockStatusReleased)
			}
			logx.Error("release_order", "execute_failed", err,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			return domain.ReleaseOrder{}, err
		}
	} else {
		order = updatedOrder
	}
	if strings.TrimSpace(operatorUserID) != "" || strings.TrimSpace(operatorName) != "" {
		updatedOrder, err = uc.repo.UpdateExecutor(ctx, order.ID, strings.TrimSpace(operatorUserID), strings.TrimSpace(operatorName), updatedAt)
		if err != nil {
			if errors.Is(err, domain.ErrOrderNotFound) {
				logx.Warn("release_order", "execute_executor_reload_failed",
					logx.F("order_id", order.ID),
					logx.F("order_no", order.OrderNo),
					logx.F("action", action),
				)
				order.ExecutorUserID = strings.TrimSpace(operatorUserID)
				order.ExecutorName = strings.TrimSpace(operatorName)
				order.UpdatedAt = updatedAt
			} else {
				if dispatchStartedAt != nil {
					_ = uc.releaseExecutionLocks(ctx, order.ID, domain.ExecutionLockStatusReleased)
				}
				logx.Error("release_order", "execute_failed", err,
					logx.F("order_id", order.ID),
					logx.F("order_no", order.OrderNo),
				)
				return domain.ReleaseOrder{}, err
			}
		} else {
			order = updatedOrder
		}
	}
	if err := uc.markApprovalFlowDispatched(ctx, order.ID, action); err != nil {
		return domain.ReleaseOrder{}, err
	}

	_ = uc.markStepRunning(ctx, order.ID, "global:param_resolve", "开始解析发布参数")
	paramResolveMessage := currentDispatchResolveMessage(action, len(orderParams))
	if (dispatchStatus == domain.OrderStatusQueued || (action == ReleaseOrderDispatchActionBuild && strings.TrimSpace(dispatchGuard.Message) != "")) &&
		strings.TrimSpace(dispatchGuard.Message) != "" {
		paramResolveMessage = strings.TrimSpace(dispatchGuard.Message)
	}
	_ = uc.markStepFinished(ctx, order.ID, "global:param_resolve", domain.StepStatusSuccess, paramResolveMessage)

	logx.Info("release_order", "execute_dispatched",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("executions_count", len(executions)),
		logx.F("params_count", len(orderParams)),
		logx.F("dispatch_status", dispatchStatus),
		logx.F("dispatch_mode", "async_tracker"),
		logx.F("action", action),
	)
	reloadedOrder, reloadErr := uc.repo.GetByID(ctx, order.ID)
	if reloadErr != nil {
		logx.Warn("release_order", "execute_dispatched_reload_failed",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("action", action),
			logx.F("reload_error", reloadErr.Error()),
		)
		return order, nil
	}
	return reloadedOrder, nil
}

// resolveDispatchExecution 解析上下文数据，得到后续流程需要的结果。
func resolveDispatchExecution(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	action ReleaseOrderDispatchAction,
	stageFullRelease bool,
) (*domain.ReleaseOrderExecution, domain.OrderStatus, error) {
	switch action {
	case ReleaseOrderDispatchActionBuild:
		if !isBuildExecutableOrderStatus(order.Status, order.ApprovalRequired) {
			return nil, "", fmt.Errorf("%w: only pending or approved release order can be built", ErrInvalidInput)
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending)
		if target == nil {
			return nil, "", fmt.Errorf("%w: release order has no pending ci execution to build", ErrInvalidInput)
		}
		if !hasExecutionForScope(executions, domain.PipelineScopeCD) {
			return nil, "", fmt.Errorf("%w: release order has no cd execution to deploy after build", ErrInvalidInput)
		}
		return target, domain.OrderStatusBuilding, nil
	case ReleaseOrderDispatchActionDeploy:
		if order.Status != domain.OrderStatusBuiltWaitingDeploy {
			return nil, "", fmt.Errorf("%w: only built release order can be deployed", ErrInvalidInput)
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending)
		if target == nil {
			return nil, "", fmt.Errorf("%w: release order has no pending cd execution to deploy", ErrInvalidInput)
		}
		return target, domain.OrderStatusDeploying, nil
	default:
		if !isExecutableOrderStatus(order.Status) {
			return nil, "", fmt.Errorf("%w: only pending or approved release order can be executed", ErrInvalidInput)
		}
		target := findExecutionByStatus(executions, domain.ExecutionStatusPending)
		if target == nil {
			return nil, "", fmt.Errorf("%w: release order has no pending executions to dispatch", ErrInvalidInput)
		}
		if stageFullRelease && target.PipelineScope == domain.PipelineScopeCI && hasExecutionForScope(executions, domain.PipelineScopeCD) {
			return target, domain.OrderStatusBuilding, nil
		}
		return target, domain.OrderStatusDeploying, nil
	}
}

// currentDispatchBlockedMessage 封装当前模块的业务处理逻辑。
func currentDispatchBlockedMessage(action ReleaseOrderDispatchAction) string {
	switch action {
	case ReleaseOrderDispatchActionBuild:
		return "当前发布单未通过构建前预检"
	case ReleaseOrderDispatchActionDeploy:
		return "当前发布单未通过部署前预检"
	default:
		return "当前发布单未通过执行前预检"
	}
}

// currentDispatchResolveMessage 解析上下文数据，得到后续流程需要的结果。
func currentDispatchResolveMessage(action ReleaseOrderDispatchAction, paramCount int) string {
	switch action {
	case ReleaseOrderDispatchActionBuild:
		return fmt.Sprintf("构建参数解析完成，总计 %d 项", paramCount)
	case ReleaseOrderDispatchActionDeploy:
		return fmt.Sprintf("部署参数解析完成，总计 %d 项", paramCount)
	default:
		return fmt.Sprintf("参数解析完成，总计 %d 项", paramCount)
	}
}

// ListParams 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListParams(ctx context.Context, orderID string) ([]domain.ReleaseOrderParam, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return nil, err
	}
	return uc.repo.ListParams(ctx, orderID)
}

// ListExecutions 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListExecutions(ctx context.Context, orderID string) ([]domain.ReleaseOrderExecution, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return nil, err
	}
	// 执行单元读取不再根据步骤或阶段反向写库；tracker 会在持有订单锁时收敛状态。
	return uc.repo.ListExecutions(ctx, orderID)
}

// ptrTime 封装当前模块的业务处理逻辑。
func ptrTime(value time.Time) *time.Time {
	return &value
}

// firstNonNilTime 封装当前模块的业务处理逻辑。
func firstNonNilTime(values ...*time.Time) *time.Time {
	for _, item := range values {
		if item != nil {
			return item
		}
	}
	return nil
}

// startNextPendingExecution 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) startNextPendingExecution(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
) error {
	for _, execution := range executions {
		if execution.Status == domain.ExecutionStatusRunning {
			logx.Info("release_order", "start_next_skip_running_exists",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("pipeline_scope", execution.PipelineScope),
				logx.F("provider", execution.Provider),
			)
			return nil
		}
	}

	for _, execution := range orderExecutionsByScope(executions) {
		if execution.Status != domain.ExecutionStatusPending {
			continue
		}
		logx.Info("release_order", "execution_start_attempt",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
			logx.F("provider", execution.Provider),
		)
		guard, acquired, guardErr := uc.ensureExecutionLock(ctx, order, execution, orderParams)
		if guardErr != nil {
			uc.markExecutionStartFailed(ctx, order, execution, guardErr.Error())
			logx.Error("release_order", "execution_lock_failed", guardErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
				logx.F("lock_key", guard.LockKey),
			)
			return guardErr
		}
		if !acquired {
			queuedStatus := nextQueuedOrderStatus(order.Status)
			if order.Status != queuedStatus {
				if _, updateErr := uc.repo.UpdateStatus(ctx, order.ID, queuedStatus, order.StartedAt, nil, uc.now()); updateErr != nil {
					return updateErr
				}
			}
			if strings.TrimSpace(guard.Message) != "" {
				_ = uc.markStepFinished(ctx, order.ID, "global:param_resolve", domain.StepStatusSuccess, guard.Message)
			}
			logx.Info("release_order", "execution_waiting_for_lock",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
				logx.F("lock_key", guard.LockKey),
				logx.F("conflict_strategy", guard.Settings.ConflictStrategy),
			)
			return nil
		}
		claimTime := uc.now()
		claimedExecution, claimed, claimErr := uc.repo.ClaimExecutionByScope(ctx, order.ID, execution.PipelineScope, claimTime, claimTime)
		if claimErr != nil {
			uc.markExecutionStartFailed(ctx, order, execution, claimErr.Error())
			logx.Error("release_order", "execution_claim_failed", claimErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
			)
			return claimErr
		}
		if !claimed {
			logx.Warn("release_order", "execution_claim_skipped_already_taken",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", claimedExecution.ID),
				logx.F("pipeline_scope", claimedExecution.PipelineScope),
				logx.F("current_status", claimedExecution.Status),
			)
			return nil
		}
		execution = claimedExecution
		runningStatus := nextRunningOrderStatus(order.Status, execution, executions)
		if order.Status != runningStatus {
			startedAt := order.StartedAt
			now := claimTime
			if startedAt == nil {
				startedAt = &now
			}
			updatedOrder, updateErr := uc.repo.UpdateStatus(ctx, order.ID, runningStatus, startedAt, nil, now)
			if updateErr != nil {
				return updateErr
			}
			order = updatedOrder
		}
		switch strings.ToLower(strings.TrimSpace(execution.Provider)) {
		case string(pipelinedomain.ProviderJenkins):
			// Jenkins 执行继续走现有触发链路。
		case string(pipelinedomain.ProviderArgoCD):
			if err := uc.startArgoCDExecution(ctx, order, execution, orderParams, executions); err != nil {
				uc.markExecutionStartFailed(ctx, order, execution, err.Error())
				logx.Error("release_order", "execution_start_failed", err,
					logx.F("order_id", order.ID),
					logx.F("order_no", order.OrderNo),
					logx.F("execution_id", execution.ID),
					logx.F("pipeline_scope", execution.PipelineScope),
					logx.F("provider", execution.Provider),
				)
				return err
			}
			logx.Info("release_order", "execution_start_success",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
				logx.F("provider", execution.Provider),
			)
			return nil
		default:
			now := uc.now()
			_, err := uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
				Status:     domain.ExecutionStatusSkipped,
				StartedAt:  &now,
				FinishedAt: &now,
				UpdatedAt:  now,
			})
			if err != nil {
				logx.Error("release_order", "execution_skip_failed", err,
					logx.F("order_id", order.ID),
					logx.F("execution_id", execution.ID),
					logx.F("provider", execution.Provider),
				)
				return err
			}
			for idx, code := range executionStepCodes(execution) {
				message := strings.ToUpper(string(execution.PipelineScope)) + " 非受支持执行器暂记为跳过"
				if idx == len(executionStepCodes(execution))-1 {
					message = strings.ToUpper(string(execution.PipelineScope)) + " 已跳过"
				}
				_ = uc.markStepFinished(ctx, order.ID, code, domain.StepStatusSuccess, message)
			}
			continue
		}

		pipelineID := strings.TrimSpace(execution.PipelineID)
		if strings.TrimSpace(execution.BindingID) != "" {
			binding, err := uc.pipelineRepo.GetBindingByID(ctx, execution.BindingID)
			if err != nil {
				if !errors.Is(err, pipelinedomain.ErrBindingNotFound) || pipelineID == "" {
					logx.Error("release_order", "execution_start_failed", err,
						logx.F("order_id", order.ID),
						logx.F("execution_id", execution.ID),
						logx.F("binding_id", execution.BindingID),
					)
					return err
				}
				logx.Warn("release_order", "execution_binding_missing_fallback_pipeline",
					logx.F("order_id", order.ID),
					logx.F("order_no", order.OrderNo),
					logx.F("execution_id", execution.ID),
					logx.F("binding_id", execution.BindingID),
					logx.F("pipeline_id", pipelineID),
				)
			} else if pipelineID == "" {
				pipelineID = strings.TrimSpace(binding.PipelineID)
			}
		}
		if pipelineID == "" {
			err := fmt.Errorf("%w: pipeline_id is required", ErrInvalidInput)
			logx.Error("release_order", "execution_start_failed", err,
				logx.F("order_id", order.ID),
				logx.F("execution_id", execution.ID),
				logx.F("binding_id", execution.BindingID),
			)
			return err
		}
		pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, pipelineID)
		if err != nil {
			logx.Error("release_order", "execution_start_failed", err,
				logx.F("order_id", order.ID),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_id", pipelineID),
			)
			return err
		}
		if err := ensureActivePipelineRecord(pipeline, "绑定管线"); err != nil {
			return err
		}
		if pipeline.Provider != pipelinedomain.ProviderJenkins {
			return fmt.Errorf("%w: bound pipeline provider is not jenkins", ErrInvalidInput)
		}
		if strings.TrimSpace(pipeline.JobFullName) == "" {
			return fmt.Errorf("%w: jenkins job full name is empty", ErrInvalidInput)
		}
		if mismatches := uc.validateJenkinsExecutionChoiceCandidates(ctx, pipeline, execution, orderParams); len(mismatches) > 0 {
			message := "Jenkins 管线参数候选值校验失败：" + strings.Join(mismatches, "；")
			uc.markExecutionStartFailed(ctx, order, execution, message)
			return fmt.Errorf("%w: %s", ErrInvalidInput, message)
		}

		buildParams, err := uc.buildJenkinsExecutionParams(ctx, order, execution, orderParams, executions)
		if err != nil {
			logx.Error("release_order", "execution_start_failed", err,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
			)
			return err
		}

		triggerCode := scopeStepCode(execution.PipelineScope, "trigger_pipeline")
		runningCode := scopeStepCode(execution.PipelineScope, "pipeline_running")
		successCode := scopeStepCode(execution.PipelineScope, "pipeline_success")

		_ = uc.markStepRunning(ctx, order.ID, triggerCode, "开始触发 "+strings.ToUpper(string(execution.PipelineScope))+" Jenkins 管线")
		queueURL, triggerErr := uc.jenkins.TriggerBuild(ctx, pipeline.JobFullName, buildParams)
		if triggerErr != nil {
			uc.markExecutionStartFailed(ctx, order, execution, "触发 Jenkins 失败: "+triggerErr.Error())
			logx.Error("release_order", "execution_start_failed", triggerErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", execution.ID),
				logx.F("pipeline_scope", execution.PipelineScope),
				logx.F("pipeline_id", pipeline.ID),
				logx.F("job_full_name", pipeline.JobFullName),
			)
			return fmt.Errorf("%w: trigger jenkins failed: %v", ErrInvalidInput, triggerErr)
		}

		now := uc.now()
		_, err = uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
			Status:    domain.ExecutionStatusRunning,
			QueueURL:  strings.TrimSpace(queueURL),
			BuildURL:  "",
			StartedAt: &now,
			UpdatedAt: now,
		})
		if err != nil {
			logx.Error("release_order", "execution_start_failed", err,
				logx.F("order_id", order.ID),
				logx.F("execution_id", execution.ID),
				logx.F("queue_url", queueURL),
			)
			return err
		}

		triggerMessage := "Jenkins 触发成功"
		if strings.TrimSpace(queueURL) != "" {
			triggerMessage += "，queue: " + strings.TrimSpace(queueURL)
		}
		_ = uc.markStepFinished(ctx, order.ID, triggerCode, domain.StepStatusSuccess, triggerMessage)
		_ = uc.markStepRunning(ctx, order.ID, runningCode, "管线已触发，等待执行结果回传，queue: "+strings.TrimSpace(queueURL))
		_ = uc.markStep(ctx, order.ID, successCode, domain.StepStatusPending, "", nil, nil)
		logx.Info("release_order", "execution_start_success",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
			logx.F("provider", execution.Provider),
			logx.F("pipeline_id", pipeline.ID),
			logx.F("job_full_name", pipeline.JobFullName),
			logx.F("queue_url", queueURL),
			logx.F("build_params_count", len(buildParams)),
		)
		return nil
	}

	for _, execution := range executions {
		if execution.Status == domain.ExecutionStatusRunning {
			return nil
		}
	}

	_ = uc.markStepRunning(ctx, order.ID, "global:release_finish", "所有执行单元已完成")
	_ = uc.markStepFinished(ctx, order.ID, "global:release_finish", domain.StepStatusSuccess, "发布完成")
	logx.Info("release_order", "all_executions_finished",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
	)
	return nil
}

func (uc *ReleaseOrderManager) validateJenkinsExecutionChoiceCandidates(
	ctx context.Context,
	pipeline pipelinedomain.Pipeline,
	execution domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
) []string {
	if uc == nil || uc.paramRepo == nil {
		return nil
	}
	reader, ok := uc.jenkins.(JenkinsReleaseParamReader)
	if !ok || reader == nil {
		return nil
	}
	jobFullName := strings.TrimSpace(pipeline.JobFullName)
	if jobFullName == "" {
		return nil
	}
	jobSet, err := reader.GetJobParamSet(ctx, jobFullName)
	if err != nil {
		logx.Warn("release_order", "jenkins_live_params_read_failed",
			logx.F("pipeline_id", pipeline.ID),
			logx.F("job_full_name", jobFullName),
			logx.F("error", err.Error()),
		)
		return nil
	}
	liveByName := make(map[string]pipelineparamdomain.JenkinsParamSnapshot, len(jobSet.Params))
	for _, item := range jobSet.Params {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name == "" {
			continue
		}
		liveByName[name] = item
	}
	if len(liveByName) == 0 {
		return nil
	}

	paramDefs, _, err := uc.paramRepo.ListByPipeline(ctx, pipelineparamdomain.ListFilter{
		PipelineID:   strings.TrimSpace(pipeline.ID),
		ExecutorType: pipelineparamdomain.ExecutorTypeJenkins,
		Status:       pipelineparamdomain.StatusActive,
		Page:         1,
		PageSize:     500,
	})
	if err != nil {
		logx.Warn("release_order", "jenkins_local_params_read_failed",
			logx.F("pipeline_id", pipeline.ID),
			logx.F("error", err.Error()),
		)
		return nil
	}
	defByName := make(map[string]pipelineparamdomain.ExecutorParamDef, len(paramDefs))
	for _, item := range paramDefs {
		name := strings.ToLower(strings.TrimSpace(item.ExecutorParamName))
		if name == "" {
			continue
		}
		defByName[name] = item
	}
	if len(defByName) == 0 {
		return nil
	}

	scopeLabel := strings.ToUpper(string(execution.PipelineScope))
	mismatches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range orderParams {
		if item.PipelineScope != execution.PipelineScope {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(item.ExecutorParamName))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		paramDef, ok := defByName[name]
		if !ok {
			continue
		}
		liveParam, ok := liveByName[name]
		if !ok {
			continue
		}
		paramLabel := executorParamNameOrKey(item.ExecutorParamName, firstNonEmpty(item.ParamKey, paramDef.ParamKey, paramDef.ID))
		mismatches = append(mismatches, compareJenkinsChoiceCandidates(scopeLabel, paramLabel, paramDef, liveParam, item.ParamValue)...)
	}
	return mismatches
}

// buildJenkinsExecutionParams 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildJenkinsExecutionParams(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
	executions []domain.ReleaseOrderExecution,
) (map[string]string, error) {
	buildParams := make(map[string]string)
	for _, item := range orderParams {
		if item.PipelineScope != execution.PipelineScope {
			continue
		}
		name := strings.TrimSpace(item.ExecutorParamName)
		if name == "" {
			continue
		}
		value := strings.TrimSpace(item.ParamValue)
		if value == "" {
			continue
		}
		buildParams[name] = value
	}

	if strings.TrimSpace(order.TemplateID) == "" {
		return buildParams, nil
	}
	_, _, templateParams, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return nil, err
	}
	if len(templateParams) == 0 {
		return buildParams, nil
	}

	appKey := ""
	artifactValues := map[string]string{}
	if uc.appRepo != nil && strings.TrimSpace(order.ApplicationID) != "" {
		if appRecord, appErr := uc.appRepo.GetByID(ctx, strings.TrimSpace(order.ApplicationID)); appErr == nil {
			appKey = strings.TrimSpace(appRecord.Key)
			values, artifactErr := uc.resolveApplicationArtifactParamValues(ctx, appRecord)
			if artifactErr != nil {
				return nil, artifactErr
			}
			artifactValues = values
		}
	}

	for _, item := range templateParams {
		if item.PipelineScope != execution.PipelineScope {
			continue
		}
		executorParamName := strings.TrimSpace(item.ExecutorParamName)
		if executorParamName == "" {
			continue
		}
		if strings.TrimSpace(buildParams[executorParamName]) != "" {
			continue
		}
		value := strings.TrimSpace(uc.resolveTemplateExecutionParamValue(order, execution.PipelineScope, item, orderParams, executions, appKey, artifactValues))
		if value == "" {
			if item.Required {
				return nil, fmt.Errorf("%w: 未解析到 %s，无法继续执行 %s 管线", ErrInvalidInput, firstNonEmpty(strings.TrimSpace(item.ParamName), strings.TrimSpace(item.ParamKey), executorParamName), strings.ToUpper(string(execution.PipelineScope)))
			}
			continue
		}
		buildParams[executorParamName] = value
	}
	return buildParams, nil
}

// resolveTemplateExecutionParamValue 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveTemplateExecutionParamValue(
	order domain.ReleaseOrder,
	scope domain.PipelineScope,
	item domain.ReleaseTemplateParam,
	orderParams []domain.ReleaseOrderParam,
	executions []domain.ReleaseOrderExecution,
	appKey string,
	artifactValues map[string]string,
) string {
	paramKey := strings.TrimSpace(item.ParamKey)
	if isCIOnlyStandardParamKey(paramKey) || isCIOnlyStandardParamKey(item.SourceParamKey) {
		return findReleaseParamValue(orderParams, domain.PipelineScopeCI, firstNonEmpty(strings.TrimSpace(item.SourceParamKey), paramKey))
	}
	if strings.EqualFold(paramKey, standardParamGOSArtifactPath) || strings.EqualFold(item.SourceParamKey, standardParamGOSArtifactPath) {
		return strings.TrimSpace(artifactValues[standardParamGOSArtifactPath])
	}
	if value := findReleaseParamValue(orderParams, scope, paramKey); value != "" {
		return value
	}
	switch item.ValueSource {
	case "", domain.TemplateParamValueSourceReleaseInput:
		return ""
	case domain.TemplateParamValueSourceFixed:
		return strings.TrimSpace(item.FixedValue)
	case domain.TemplateParamValueSourceCIParam:
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, item.SourceParamKey),
			uc.resolveStandardFieldValue(order, orderParams, executions, appKey, artifactValues, item.SourceParamKey),
		)
	case domain.TemplateParamValueSourceBuiltin:
		return uc.resolveStandardFieldValue(order, orderParams, executions, appKey, artifactValues, item.SourceParamKey)
	default:
		return ""
	}
}

// resolveStandardFieldValue 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveStandardFieldValue(
	order domain.ReleaseOrder,
	orderParams []domain.ReleaseOrderParam,
	executions []domain.ReleaseOrderExecution,
	appKey string,
	artifactValues map[string]string,
	key string,
) string {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return ""
	}
	switch normalizedKey {
	case "env", "env_code":
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, "env", "env_code"),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, "env", "env_code"),
			strings.TrimSpace(order.EnvCode),
		)
	case "project_name":
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, "project_name"),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, "project_name"),
		)
	case "release_name":
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, "release_name"),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, "release_name"),
			findReleaseParamValue(orderParams, "", "release_name"),
			strings.TrimSpace(order.ReleaseName),
		)
	case "branch", "git_ref":
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, "branch", "git_ref"),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, "branch", "git_ref"),
			strings.TrimSpace(order.GitRef),
		)
	case "image_version", "image_tag":
		return uc.resolveArgoCDImageVersion(order, orderParams, executions)
	case "app_key":
		return strings.TrimSpace(appKey)
	case "app_name":
		return strings.TrimSpace(order.ApplicationName)
	case standardParamGOSArtifactURL:
		return findReleaseParamValue(orderParams, domain.PipelineScopeCI, standardParamGOSArtifactURL)
	case standardParamGOSArtifactPath:
		return strings.TrimSpace(artifactValues[standardParamGOSArtifactPath])
	case "oss_endpoint", "oss_bucket", "oss_dir", "oss_acl", "oss_access_key_id", "oss_access_key_secret":
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, normalizedKey),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, normalizedKey),
			findReleaseParamValue(orderParams, "", normalizedKey),
			strings.TrimSpace(artifactValues[normalizedKey]),
		)
	default:
		return firstNonEmpty(
			findReleaseParamValue(orderParams, domain.PipelineScopeCD, normalizedKey),
			findReleaseParamValue(orderParams, domain.PipelineScopeCI, normalizedKey),
			findReleaseParamValue(orderParams, "", normalizedKey),
		)
	}
}

// markExecutionStartFailed 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) markExecutionStartFailed(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	message string,
) {
	message = strings.TrimSpace(message)
	logx.Error("release_order", "execution_mark_failed", fmt.Errorf("%s", message),
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("execution_id", execution.ID),
		logx.F("pipeline_scope", execution.PipelineScope),
		logx.F("provider", execution.Provider),
	)
	now := uc.now()
	persistCtx, cancel := uc.backgroundPersistenceContext()
	defer cancel()
	if _, err := uc.repo.UpdateExecutionByScope(persistCtx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
		Status:     domain.ExecutionStatusFailed,
		StartedAt:  &now,
		FinishedAt: &now,
		UpdatedAt:  now,
	}); err != nil {
		logx.Error("release_order", "execution_mark_failed_persist_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
		)
	}
	if err := uc.markOpenExecutionStepsFailed(persistCtx, order.ID, execution, message); err != nil {
		logx.Error("release_order", "execution_mark_failed_persist_steps_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
		)
	}
	if err := uc.markStepFinished(persistCtx, order.ID, "global:release_finish", domain.StepStatusFailed, message); err != nil {
		logx.Error("release_order", "execution_mark_failed_persist_global_finish_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
		)
	}
	if _, err := uc.repo.UpdateStatus(persistCtx, order.ID, domain.OrderStatusFailed, order.StartedAt, &now, now); err != nil {
		logx.Error("release_order", "execution_mark_failed_persist_order_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
		)
	}
	if err := uc.releaseExecutionLocks(persistCtx, order.ID, domain.ExecutionLockStatusReleased); err != nil {
		logx.Error("release_order", "execution_mark_failed_release_lock_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
			logx.F("pipeline_scope", execution.PipelineScope),
		)
	}
}

// backgroundPersistenceContext 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) backgroundPersistenceContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// markOpenExecutionStepsFailed 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) markOpenExecutionStepsFailed(
	ctx context.Context,
	orderID string,
	execution domain.ReleaseOrderExecution,
	message string,
) error {
	steps, err := uc.repo.ListSteps(ctx, orderID)
	if err != nil {
		return err
	}
	for _, code := range executionStepCodes(execution) {
		current := findStepByCode(steps, code)
		if current == nil || current.Status == domain.StepStatusSuccess || current.Status == domain.StepStatusFailed {
			continue
		}
		if err := uc.markStepFinished(ctx, orderID, code, domain.StepStatusFailed, message); err != nil {
			return err
		}
	}
	return nil
}

// resolveOrderGitOpsType 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveOrderGitOpsType(
	ctx context.Context,
	order domain.ReleaseOrder,
) (domain.GitOpsType, error) {
	template, _, _, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return "", err
	}
	return normalizeTemplateGitOpsType(template.GitOpsType, true), nil
}

// scopeStepCode 封装当前模块的业务处理逻辑。
func scopeStepCode(scope domain.PipelineScope, suffix string) string {
	return strings.ToLower(strings.TrimSpace(string(scope))) + ":" + strings.TrimSpace(suffix)
}

// markStepRunning 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) markStepRunning(ctx context.Context, orderID string, stepCode string, message string) error {
	now := uc.now()
	return uc.markStep(ctx, orderID, stepCode, domain.StepStatusRunning, strings.TrimSpace(message), &now, nil)
}

// markStepFinished 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) markStepFinished(
	ctx context.Context,
	orderID string,
	stepCode string,
	status domain.StepStatus,
	message string,
) error {
	if status != domain.StepStatusSuccess && status != domain.StepStatusFailed {
		return ErrInvalidStatus
	}
	current, err := uc.repo.GetStepByCode(ctx, orderID, stepCode)
	if err != nil {
		if errors.Is(err, domain.ErrStepNotFound) {
			return nil
		}
		return err
	}

	startedAt := current.StartedAt
	now := uc.now()
	if startedAt == nil {
		startedAt = &now
	}
	return uc.markStep(ctx, orderID, stepCode, status, strings.TrimSpace(message), startedAt, &now)
}

// markStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) markStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	status domain.StepStatus,
	message string,
	startedAt *time.Time,
	finishedAt *time.Time,
) error {
	_, err := uc.repo.UpdateStep(ctx, orderID, stepCode, domain.StepUpdateInput{
		Status:     status,
		Message:    message,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	})
	if err != nil {
		if errors.Is(err, domain.ErrStepNotFound) {
			return nil
		}
		return err
	}
	if syncErr := uc.syncPipelineStageFromStep(ctx, orderID, stepCode, status, message, startedAt, finishedAt); syncErr != nil {
		return syncErr
	}
	return nil
}

// syncPipelineStageFromStep 同步外部或内部状态数据。
func (uc *ReleaseOrderManager) syncPipelineStageFromStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	status domain.StepStatus,
	message string,
	startedAt *time.Time,
	finishedAt *time.Time,
) error {
	scope, suffix, ok := strings.Cut(strings.TrimSpace(stepCode), ":")
	if !ok {
		return nil
	}
	if !isArgoCDPipelineStageKey(suffix) {
		return nil
	}
	stages, err := uc.repo.ListPipelineStages(ctx, orderID)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		return nil
	}
	now := uc.now()
	changed := false
	for idx := range stages {
		if !strings.EqualFold(strings.TrimSpace(stages[idx].PipelineScope), scope) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stages[idx].StageKey), suffix) {
			continue
		}
		itemChanged := false
		nextStatus := pipelineStageStatusFromStepStatus(status)
		if stages[idx].Status != nextStatus {
			stages[idx].Status = nextStatus
			itemChanged = true
		}
		if strings.TrimSpace(message) != strings.TrimSpace(stages[idx].RawStatus) {
			stages[idx].RawStatus = strings.TrimSpace(message)
			itemChanged = true
		}
		if startedAt != nil && (stages[idx].StartedAt == nil || !stages[idx].StartedAt.Equal(*startedAt)) {
			value := startedAt.UTC()
			stages[idx].StartedAt = &value
			itemChanged = true
		}
		if finishedAt != nil && (stages[idx].FinishedAt == nil || !stages[idx].FinishedAt.Equal(*finishedAt)) {
			value := finishedAt.UTC()
			stages[idx].FinishedAt = &value
			itemChanged = true
		}
		duration := computePipelineStageDurationFromTimes(stages[idx].StartedAt, stages[idx].FinishedAt, status, now)
		if stages[idx].DurationMillis != duration {
			stages[idx].DurationMillis = duration
			itemChanged = true
		}
		if itemChanged {
			stages[idx].UpdatedAt = now
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return uc.repo.ReplacePipelineStages(ctx, orderID, stages)
}

// isArgoCDPipelineStageKey 封装当前模块的业务处理逻辑。
func isArgoCDPipelineStageKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "gitops_update", "git_commit", "git_push", "argocd_sync", "health_check":
		return true
	default:
		return false
	}
}

// pipelineStageStatusFromStepStatus 封装当前模块的业务处理逻辑。
func pipelineStageStatusFromStepStatus(status domain.StepStatus) domain.PipelineStageStatus {
	switch status {
	case domain.StepStatusSuccess:
		return domain.PipelineStageStatusSuccess
	case domain.StepStatusFailed:
		return domain.PipelineStageStatusFailed
	case domain.StepStatusRunning:
		return domain.PipelineStageStatusRunning
	default:
		return domain.PipelineStageStatusPending
	}
}

// computePipelineStageDurationFromTimes 封装当前模块的业务处理逻辑。
func computePipelineStageDurationFromTimes(startedAt *time.Time, finishedAt *time.Time, status domain.StepStatus, now time.Time) int64 {
	if startedAt == nil {
		return 0
	}
	end := finishedAt
	if end == nil && status == domain.StepStatusRunning {
		current := now.UTC()
		end = &current
	}
	if end == nil || end.Before(*startedAt) {
		return 0
	}
	return end.Sub(*startedAt).Milliseconds()
}

// ListSteps 查询并返回列表数据。
func (uc *ReleaseOrderManager) ListSteps(ctx context.Context, orderID string) ([]domain.ReleaseOrderStep, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return nil, err
	}
	items, err := uc.repo.ListSteps(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// 步骤读取只做展示增强，不再触发终态协调写入。
	return uc.enrichAgentTaskStepDetails(ctx, items), nil
}

// StartStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) StartStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	message string,
) (domain.ReleaseOrderStep, domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	stepCode = strings.TrimSpace(stepCode)
	message = strings.TrimSpace(message)
	if orderID == "" || stepCode == "" {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, ErrInvalidID
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}
	if order.Status.IsTerminal() {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, fmt.Errorf("%w: release order is already finished", ErrInvalidInput)
	}

	step, err := uc.repo.GetStepByCode(ctx, orderID, stepCode)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}
	if step.Status != domain.StepStatusPending {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, fmt.Errorf("%w: step is not pending", ErrInvalidInput)
	}

	allSteps, err := uc.repo.ListSteps(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}
	if err := ensureStepOrder(allSteps, step); err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}

	now := uc.now()
	updatedStep, err := uc.repo.UpdateStep(ctx, orderID, stepCode, domain.StepUpdateInput{
		Status:     domain.StepStatusRunning,
		Message:    message,
		StartedAt:  &now,
		FinishedAt: nil,
	})
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}

	if order.Status == domain.OrderStatusPending {
		order, err = uc.repo.UpdateStatus(ctx, orderID, domain.OrderStatusRunning, &now, nil, now)
		if err != nil {
			return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
		}
	} else {
		order, err = uc.repo.GetByID(ctx, orderID)
		if err != nil {
			return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
		}
	}
	if order.Status == domain.OrderStatusSuccess || order.Status == domain.OrderStatusDeploySuccess {
		if stateErr := uc.RecordAppReleaseState(ctx, order.ID); stateErr != nil {
			logx.Error("release_order", "finish_step_record_app_release_state_failed", stateErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("step_code", stepCode),
				logx.F("status", order.Status),
			)
		}
	}

	return updatedStep, order, nil
}

// FinishStep 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) FinishStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	input FinishReleaseOrderStepInput,
) (domain.ReleaseOrderStep, domain.ReleaseOrder, error) {
	orderID = strings.TrimSpace(orderID)
	stepCode = strings.TrimSpace(stepCode)
	input.Message = strings.TrimSpace(input.Message)
	if orderID == "" || stepCode == "" {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, ErrInvalidID
	}

	if input.Status == "" {
		input.Status = domain.StepStatusSuccess
	}
	if input.Status != domain.StepStatusSuccess && input.Status != domain.StepStatusFailed {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, ErrInvalidStatus
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}
	if order.Status.IsTerminal() {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, fmt.Errorf("%w: release order is already finished", ErrInvalidInput)
	}

	step, err := uc.repo.GetStepByCode(ctx, orderID, stepCode)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}
	if step.Status != domain.StepStatusRunning {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, fmt.Errorf("%w: step is not running", ErrInvalidInput)
	}

	now := uc.now()
	startedAt := step.StartedAt
	if startedAt == nil {
		startedAt = &now
	}

	updatedStep, err := uc.repo.UpdateStep(ctx, orderID, stepCode, domain.StepUpdateInput{
		Status:     input.Status,
		Message:    input.Message,
		StartedAt:  startedAt,
		FinishedAt: &now,
	})
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}

	steps, err := uc.repo.ListSteps(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
	}

	nextOrderStatus, shouldUpdateOrder := deriveOrderStatusFromSteps(steps)
	if input.Status == domain.StepStatusFailed {
		nextOrderStatus = domain.OrderStatusFailed
		shouldUpdateOrder = true
	}
	if shouldUpdateOrder && nextOrderStatus != order.Status {
		started := order.StartedAt
		if started == nil {
			started = &now
		}
		finished := &now
		if nextOrderStatus == domain.OrderStatusRunning {
			finished = nil
		}
		order, err = uc.repo.UpdateStatus(ctx, orderID, nextOrderStatus, started, finished, now)
		if err != nil {
			return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
		}
	} else {
		order, err = uc.repo.GetByID(ctx, orderID)
		if err != nil {
			return domain.ReleaseOrderStep{}, domain.ReleaseOrder{}, err
		}
	}

	return updatedStep, order, nil
}

// buildCreateParams 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildCreateParams(
	orderID string,
	now time.Time,
	input []CreateReleaseOrderParamInput,
	executions []domain.ReleaseOrderExecution,
) ([]domain.ReleaseOrderParam, error) {
	bindingByScope := make(map[domain.PipelineScope]domain.ReleaseOrderExecution, len(executions))
	for _, item := range executions {
		bindingByScope[item.PipelineScope] = item
	}
	items := make([]domain.ReleaseOrderParam, 0, len(input))
	for _, item := range input {
		if !item.PipelineScope.Valid() {
			return nil, fmt.Errorf("%w: pipeline_scope is required", ErrInvalidInput)
		}
		execution, ok := bindingByScope[item.PipelineScope]
		if !ok {
			return nil, fmt.Errorf("%w: pipeline_scope %s is not enabled", ErrInvalidInput, strings.ToUpper(string(item.PipelineScope)))
		}
		paramKey := strings.TrimSpace(item.ParamKey)
		if paramKey == "" {
			return nil, fmt.Errorf("%w: param_key is required", ErrInvalidInput)
		}
		source := item.ValueSource
		if source == "" {
			source = domain.ValueSourceReleaseInput
		}
		if !source.Valid() {
			return nil, ErrInvalidSourceFrom
		}
		items = append(items, domain.ReleaseOrderParam{
			ID:                generateID("rop"),
			ReleaseOrderID:    orderID,
			PipelineScope:     item.PipelineScope,
			BindingID:         execution.BindingID,
			ParamKey:          paramKey,
			ExecutorParamName: strings.TrimSpace(item.ExecutorParamName),
			ParamValue:        strings.TrimSpace(item.ParamValue),
			ValueSource:       source,
			CreatedAt:         now,
		})
	}
	return items, nil
}

// buildCreateExecutions 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildCreateExecutions(
	orderID string,
	now time.Time,
	bindings []domain.ReleaseTemplateBinding,
) []domain.ReleaseOrderExecution {
	items := make([]domain.ReleaseOrderExecution, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		items = append(items, domain.ReleaseOrderExecution{
			ID:             generateID("roe"),
			ReleaseOrderID: orderID,
			PipelineScope:  binding.PipelineScope,
			BindingID:      binding.BindingID,
			BindingName:    binding.BindingName,
			Provider:       binding.Provider,
			PipelineID:     binding.PipelineID,
			Status:         domain.ExecutionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	return items
}

// buildCreateSteps 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildCreateSteps(
	orderID string,
	now time.Time,
	executions []domain.ReleaseOrderExecution,
	gitopsType domain.GitOpsType,
	templateHooks []domain.ReleaseTemplateHook,
	input []CreateReleaseOrderStepInput,
	orderEnvCode string,
) ([]domain.ReleaseOrderStep, error) {
	if len(input) == 0 {
		return defaultReleaseOrderSteps(orderID, executions, now, gitopsType, templateHooks, orderEnvCode), nil
	}

	items := make([]domain.ReleaseOrderStep, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for idx, item := range input {
		stepCode := strings.TrimSpace(item.StepCode)
		if stepCode == "" {
			return nil, fmt.Errorf("%w: step_code is required", ErrInvalidInput)
		}
		if _, exists := seen[stepCode]; exists {
			return nil, fmt.Errorf("%w: duplicated step_code %s", ErrInvalidInput, stepCode)
		}
		seen[stepCode] = struct{}{}

		stepName := strings.TrimSpace(item.StepName)
		if stepName == "" {
			stepName = stepCode
		}
		sortNo := item.SortNo
		if sortNo <= 0 {
			sortNo = idx + 1
		}
		items = append(items, domain.ReleaseOrderStep{
			ID:             generateID("ros"),
			ReleaseOrderID: orderID,
			StepScope:      domain.StepScopeGlobal,
			StepCode:       stepCode,
			StepName:       stepName,
			Status:         domain.StepStatusPending,
			Message:        "",
			SortNo:         sortNo,
			CreatedAt:      now,
		})
	}
	return items, nil
}

type releaseExecutionStepDef struct {
	Suffix string
	Name   string
}

// defaultExecutionStepDefs 封装当前模块的业务处理逻辑。
func defaultExecutionStepDefs(
	execution domain.ReleaseOrderExecution,
	gitopsType domain.GitOpsType,
) []releaseExecutionStepDef {
	scopeLabel := strings.ToUpper(string(execution.PipelineScope))
	if isArgoCDExecution(execution) {
		updateName := "CD 更新 GitOps 配置"
		if normalizeTemplateGitOpsType(gitopsType, true) == domain.GitOpsTypeHelm {
			updateName = "CD 更新 Helm Values"
		}
		return []releaseExecutionStepDef{
			{Suffix: "gitops_update", Name: updateName},
			{Suffix: "git_commit", Name: "CD Git 提交"},
			{Suffix: "git_push", Name: "CD Git 推送"},
			{Suffix: "argocd_sync", Name: "CD 触发 ArgoCD"},
			{Suffix: "health_check", Name: "CD 健康检查"},
		}
	}
	return []releaseExecutionStepDef{
		{Suffix: "trigger_pipeline", Name: scopeLabel + " 触发管线"},
		{Suffix: "pipeline_running", Name: scopeLabel + " 管线运行"},
		{Suffix: "pipeline_success", Name: scopeLabel + " 发布完成"},
	}
}

// executionStepCodes 封装当前模块的业务处理逻辑。
func executionStepCodes(execution domain.ReleaseOrderExecution) []string {
	defs := defaultExecutionStepDefs(execution, "")
	result := make([]string, 0, len(defs))
	for _, item := range defs {
		result = append(result, scopeStepCode(execution.PipelineScope, item.Suffix))
	}
	return result
}

// defaultReleaseOrderSteps 封装当前模块的业务处理逻辑。
func defaultReleaseOrderSteps(orderID string, executions []domain.ReleaseOrderExecution, now time.Time, gitopsType domain.GitOpsType, templateHooks []domain.ReleaseTemplateHook, orderEnvCode string) []domain.ReleaseOrderStep {
	items := make([]domain.ReleaseOrderStep, 0, 8)
	sortNo := 1
	appendStep := func(scope domain.StepScope, executionID string, code string, name string) {
		items = append(items, domain.ReleaseOrderStep{
			ID:             generateID("ros"),
			ReleaseOrderID: orderID,
			StepScope:      scope,
			ExecutionID:    executionID,
			StepCode:       code,
			StepName:       name,
			Status:         domain.StepStatusPending,
			SortNo:         sortNo,
			CreatedAt:      now,
		})
		sortNo++
	}

	appendStep(domain.StepScopeGlobal, "", "global:param_resolve", "参数解析")
	buildStageHooks := make([]domain.ReleaseTemplateHook, 0)
	postReleaseHooks := make([]domain.ReleaseTemplateHook, 0)
	for _, item := range templateHooks {
		if !hookMatchesEnv(item.EnvCodes, orderEnvCode) {
			continue
		}
		stages := domain.NormalizeTemplateHookExecuteStages(item.ExecuteStages, item.ExecuteStage)
		for _, stage := range stages {
			switch stage {
			case domain.TemplateHookExecuteStageBuildComplete:
				buildStageHooks = append(buildStageHooks, item)
			default:
				postReleaseHooks = append(postReleaseHooks, item)
			}
		}
	}

	for _, execution := range orderExecutionsByScope(executions) {
		stepScope := domain.StepScope(strings.ToLower(string(execution.PipelineScope)))
		for _, stepDef := range defaultExecutionStepDefs(execution, gitopsType) {
			appendStep(stepScope, execution.ID, scopeStepCode(execution.PipelineScope, stepDef.Suffix), stepDef.Name)
		}
		if execution.PipelineScope == domain.PipelineScopeCI {
			for _, item := range buildStageHooks {
				stepName := strings.TrimSpace(item.Name)
				if stepName == "" {
					stepName = "构建完成 Hook"
				}
				items = append(items, domain.ReleaseOrderStep{
					ID:             generateID("ros"),
					ReleaseOrderID: orderID,
					StepScope:      domain.StepScopeGlobal,
					StepCode:       fmt.Sprintf("hook:%s:%s:%d", domain.TemplateHookExecuteStageBuildComplete, strings.TrimSpace(string(item.HookType)), item.SortNo),
					StepName:       stepName,
					Status:         domain.StepStatusPending,
					Message:        buildTemplateHookStepMessage(item, domain.TemplateHookExecuteStageBuildComplete),
					SortNo:         sortNo,
					CreatedAt:      now,
				})
				sortNo++
			}
		}
	}
	for _, item := range postReleaseHooks {
		stepName := strings.TrimSpace(item.Name)
		if stepName == "" {
			stepName = "发布后 Hook"
		}
		items = append(items, domain.ReleaseOrderStep{
			ID:             generateID("ros"),
			ReleaseOrderID: orderID,
			StepScope:      domain.StepScopeGlobal,
			StepCode:       fmt.Sprintf("hook:%s:%s:%d", domain.TemplateHookExecuteStagePostRelease, strings.TrimSpace(string(item.HookType)), item.SortNo),
			StepName:       stepName,
			Status:         domain.StepStatusPending,
			Message:        buildTemplateHookStepMessage(item, domain.TemplateHookExecuteStagePostRelease),
			SortNo:         sortNo,
			CreatedAt:      now,
		})
		sortNo++
	}
	appendStep(domain.StepScopeGlobal, "", "global:release_finish", "发布完成")
	return items
}

// buildTemplateHookStepMessage 组装业务执行所需的输入数据。
func buildTemplateHookStepMessage(item domain.ReleaseTemplateHook, stage domain.TemplateHookExecuteStage) string {
	stageLabel := "发布完成时"
	if stage == domain.TemplateHookExecuteStageBuildComplete {
		stageLabel = "构建完成时"
	}
	switch item.HookType {
	case domain.TemplateHookTypeAgentTask:
		target := strings.TrimSpace(item.TargetName)
		if target == "" {
			target = strings.TrimSpace(item.TargetID)
		}
		if target == "" {
			target = "未命名 Agent 任务"
		}
		return fmt.Sprintf("%s · Agent 任务：%s", stageLabel, target)
	case domain.TemplateHookTypeNotificationHook:
		target := strings.TrimSpace(item.TargetName)
		if target == "" {
			target = strings.TrimSpace(item.TargetID)
		}
		if target == "" {
			target = "未命名通知 Hook"
		}
		return fmt.Sprintf("%s · 通知 Hook：%s", stageLabel, target)
	case domain.TemplateHookTypeWebhookNotification:
		method := strings.ToUpper(strings.TrimSpace(item.WebhookMethod))
		if method == "" {
			method = "POST"
		}
		target := strings.TrimSpace(item.WebhookURL)
		if target == "" {
			target = "未配置 Webhook URL"
		}
		return fmt.Sprintf("%s · Webhook：%s %s", stageLabel, method, target)
	default:
		return strings.TrimSpace(item.Note)
	}
}

// orderExecutionsByScope 封装当前模块的业务处理逻辑。
func orderExecutionsByScope(items []domain.ReleaseOrderExecution) []domain.ReleaseOrderExecution {
	result := make([]domain.ReleaseOrderExecution, 0, len(items))
	var ci, cd *domain.ReleaseOrderExecution
	for idx := range items {
		switch items[idx].PipelineScope {
		case domain.PipelineScopeCI:
			ci = &items[idx]
		case domain.PipelineScopeCD:
			cd = &items[idx]
		}
	}
	if ci != nil {
		result = append(result, *ci)
	}
	if cd != nil {
		result = append(result, *cd)
	}
	return result
}

// pickPrimaryExecution 封装当前模块的业务处理逻辑。
func pickPrimaryExecution(items []domain.ReleaseOrderExecution) (domain.ReleaseOrderExecution, bool) {
	ordered := orderExecutionsByScope(items)
	if len(ordered) == 0 {
		return domain.ReleaseOrderExecution{}, false
	}
	return ordered[0], true
}

// deriveOrderStatusFromSteps 封装当前模块的业务处理逻辑。
func deriveOrderStatusFromSteps(steps []domain.ReleaseOrderStep) (domain.OrderStatus, bool) {
	if len(steps) == 0 {
		return domain.OrderStatusRunning, false
	}

	allSuccess := true
	for _, step := range steps {
		if step.StepScope == domain.StepScopeGlobal && step.StepCode == "global:release_finish" {
			switch step.Status {
			case domain.StepStatusFailed:
				return domain.OrderStatusFailed, true
			case domain.StepStatusSuccess:
				continue
			case domain.StepStatusPending, domain.StepStatusRunning:
				allSuccess = false
				continue
			}
		}
		switch step.Status {
		case domain.StepStatusFailed:
			return domain.OrderStatusFailed, true
		case domain.StepStatusSuccess:
			// continue
		case domain.StepStatusPending, domain.StepStatusRunning:
			allSuccess = false
		default:
			allSuccess = false
		}
	}

	if allSuccess {
		return domain.OrderStatusSuccess, true
	}
	return domain.OrderStatusRunning, false
}

// isExecutableOrderStatus 封装当前模块的业务处理逻辑。
func isExecutableOrderStatus(status domain.OrderStatus) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(status)))
	return normalized == "pending" || normalized == "pengding" || normalized == "approved"
}

// isBuildExecutableOrderStatus 组装业务执行所需的输入数据。
func isBuildExecutableOrderStatus(status domain.OrderStatus, approvalRequired bool) bool {
	if approvalRequired {
		return status == domain.OrderStatusApproved
	}
	return status == domain.OrderStatusPending || status == domain.OrderStatusApproved
}

// isEditableOrderStatus 封装当前模块的业务处理逻辑。
func isEditableOrderStatus(status domain.OrderStatus) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(status)))
	return normalized == "pending" || normalized == "pengding"
}

// nextQueuedOrderStatus 封装当前模块的业务处理逻辑。
func nextQueuedOrderStatus(current domain.OrderStatus) domain.OrderStatus {
	return domain.OrderStatusQueued
}

// nextRunningOrderStatus 封装当前模块的业务处理逻辑。
func nextRunningOrderStatus(
	current domain.OrderStatus,
	execution domain.ReleaseOrderExecution,
	executions []domain.ReleaseOrderExecution,
) domain.OrderStatus {
	if current == domain.OrderStatusBuilding ||
		(execution.PipelineScope == domain.PipelineScopeCI && hasExecutionForScope(executions, domain.PipelineScopeCD)) {
		return domain.OrderStatusBuilding
	}
	return domain.OrderStatusDeploying
}

// findExecutionByScopeAndStatus 封装当前模块的业务处理逻辑。
func findExecutionByScopeAndStatus(
	items []domain.ReleaseOrderExecution,
	scope domain.PipelineScope,
	status domain.ExecutionStatus,
) *domain.ReleaseOrderExecution {
	for idx := range items {
		if items[idx].PipelineScope == scope && items[idx].Status == status {
			return &items[idx]
		}
	}
	return nil
}

// hasExecutionForScope 封装当前模块的业务处理逻辑。
func hasExecutionForScope(items []domain.ReleaseOrderExecution, scope domain.PipelineScope) bool {
	for _, item := range items {
		if item.PipelineScope == scope {
			return true
		}
	}
	return false
}

// resolveInitialReleaseOrderStatus 解析上下文数据，得到后续流程需要的结果。
func resolveInitialReleaseOrderStatus(template domain.ReleaseTemplate, creatorUserID string) domain.OrderStatus {
	if template.ApprovalEnabled {
		if shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, creatorUserID) {
			return domain.OrderStatusApproved
		}
		return domain.OrderStatusPendingApproval
	}
	return domain.OrderStatusPending
}

// shouldAutoApproveOnCreate 创建业务资源并返回处理结果。
func shouldAutoApproveOnCreate(approvalEnabled bool, approverIDs []string, creatorUserID string) bool {
	if !approvalEnabled {
		return false
	}
	creatorUserID = strings.TrimSpace(creatorUserID)
	if creatorUserID == "" {
		return false
	}
	hasApprover := false
	for _, item := range approverIDs {
		approverID := strings.TrimSpace(item)
		if approverID == "" {
			continue
		}
		hasApprover = true
		if approverID != creatorUserID {
			return false
		}
	}
	return hasApprover
}

// ensureStepOrder 校验前置条件，不满足时写入对应错误响应。
func ensureStepOrder(steps []domain.ReleaseOrderStep, current domain.ReleaseOrderStep) error {
	for _, item := range steps {
		if item.SortNo < current.SortNo && item.Status != domain.StepStatusSuccess {
			return fmt.Errorf("%w: previous step %s is not success", ErrInvalidInput, item.StepCode)
		}
	}
	return nil
}

// generateOrderNo 封装当前模块的业务处理逻辑。
func generateOrderNo(now time.Time) string {
	entropy := make([]byte, 4)
	if _, err := rand.Read(entropy); err != nil {
		return "RO-" + now.UTC().Format("20060102150405")
	}
	return "RO-" + now.UTC().Format("20060102150405") + "-" + strings.ToUpper(hex.EncodeToString(entropy))
}

// hookMatchesEnv 检查 Hook 的环境配置是否与发布单的执行单元环境匹配
func hookMatchesEnv(hookEnvCodes []string, orderEnvCode string) bool {
	if len(hookEnvCodes) == 0 {
		return true // 未配置环境限制，所有环境都执行
	}
	orderEnv := strings.TrimSpace(orderEnvCode)
	if orderEnv == "" {
		return false // 发布单没有环境信息，保守跳过
	}
	// 检查是否有任意一个 hook 环境匹配发布单的环境
	for _, hookEnv := range hookEnvCodes {
		if strings.EqualFold(strings.TrimSpace(hookEnv), orderEnv) {
			return true
		}
	}
	return false
}
