package release

import "time"

type PipelineScope string

const (
	PipelineScopeCI PipelineScope = "ci"
	PipelineScopeCD PipelineScope = "cd"
)

// Valid 封装当前模块的业务处理逻辑。
func (s PipelineScope) Valid() bool {
	switch s {
	case PipelineScopeCI, PipelineScopeCD:
		return true
	default:
		return false
	}
}

type StepScope string

const (
	StepScopeGlobal StepScope = "global"
	StepScopeCI     StepScope = "ci"
	StepScopeCD     StepScope = "cd"
)

// Valid 封装当前模块的业务处理逻辑。
func (s StepScope) Valid() bool {
	switch s {
	case StepScopeGlobal, StepScopeCI, StepScopeCD:
		return true
	default:
		return false
	}
}

type TriggerType string

const (
	TriggerTypeManual   TriggerType = "manual"
	TriggerTypeWebhook  TriggerType = "webhook"
	TriggerTypeSchedule TriggerType = "schedule"
)

// Valid 封装当前模块的业务处理逻辑。
func (t TriggerType) Valid() bool {
	switch t {
	case TriggerTypeManual, TriggerTypeWebhook, TriggerTypeSchedule:
		return true
	default:
		return false
	}
}

type OperationType string

const (
	OperationTypeDeploy   OperationType = "deploy"
	OperationTypeRollback OperationType = "rollback"
	OperationTypeReplay   OperationType = "replay"
)

// Valid 封装当前模块的业务处理逻辑。
func (t OperationType) Valid() bool {
	switch t {
	case OperationTypeDeploy, OperationTypeRollback, OperationTypeReplay:
		return true
	default:
		return false
	}
}

type ReleaseBusinessStatus string

const (
	ReleaseBusinessStatusDraft              ReleaseBusinessStatus = "draft"
	ReleaseBusinessStatusPendingExecution   ReleaseBusinessStatus = "pending_execution"
	ReleaseBusinessStatusPendingApproval    ReleaseBusinessStatus = "pending_approval"
	ReleaseBusinessStatusApproving          ReleaseBusinessStatus = "approving"
	ReleaseBusinessStatusApproved           ReleaseBusinessStatus = "approved"
	ReleaseBusinessStatusBuilding           ReleaseBusinessStatus = "building"
	ReleaseBusinessStatusBuiltWaitingDeploy ReleaseBusinessStatus = "built_waiting_deploy"
	ReleaseBusinessStatusRejected           ReleaseBusinessStatus = "rejected"
	ReleaseBusinessStatusQueued             ReleaseBusinessStatus = "queued"
	ReleaseBusinessStatusDeploying          ReleaseBusinessStatus = "deploying"
	ReleaseBusinessStatusDeploySuccess      ReleaseBusinessStatus = "deploy_success"
	ReleaseBusinessStatusDeployFailed       ReleaseBusinessStatus = "deploy_failed"
	ReleaseBusinessStatusCancelled          ReleaseBusinessStatus = "cancelled"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ReleaseBusinessStatus) Valid() bool {
	switch s {
	case ReleaseBusinessStatusDraft,
		ReleaseBusinessStatusPendingExecution,
		ReleaseBusinessStatusPendingApproval,
		ReleaseBusinessStatusApproving,
		ReleaseBusinessStatusApproved,
		ReleaseBusinessStatusBuilding,
		ReleaseBusinessStatusBuiltWaitingDeploy,
		ReleaseBusinessStatusRejected,
		ReleaseBusinessStatusQueued,
		ReleaseBusinessStatusDeploying,
		ReleaseBusinessStatusDeploySuccess,
		ReleaseBusinessStatusDeployFailed,
		ReleaseBusinessStatusCancelled:
		return true
	default:
		return false
	}
}

type OrderStatus string

const (
	OrderStatusPending            OrderStatus = "pending"
	OrderStatusRunning            OrderStatus = "running"
	OrderStatusSuccess            OrderStatus = "success"
	OrderStatusFailed             OrderStatus = "failed"
	OrderStatusCancelled          OrderStatus = "cancelled"
	OrderStatusDraft              OrderStatus = "draft"
	OrderStatusPendingApproval    OrderStatus = "pending_approval"
	OrderStatusApproving          OrderStatus = "approving"
	OrderStatusApproved           OrderStatus = "approved"
	OrderStatusBuilding           OrderStatus = "building"
	OrderStatusBuiltWaitingDeploy OrderStatus = "built_waiting_deploy"
	OrderStatusRejected           OrderStatus = "rejected"
	OrderStatusQueued             OrderStatus = "queued"
	OrderStatusDeploying          OrderStatus = "deploying"
	OrderStatusDeploySuccess      OrderStatus = "deploy_success"
	OrderStatusDeployFailed       OrderStatus = "deploy_failed"
)

// Valid 封装当前模块的业务处理逻辑。
func (s OrderStatus) Valid() bool {
	switch s {
	case OrderStatusPending,
		OrderStatusRunning,
		OrderStatusSuccess,
		OrderStatusFailed,
		OrderStatusCancelled,
		OrderStatusDraft,
		OrderStatusPendingApproval,
		OrderStatusApproving,
		OrderStatusApproved,
		OrderStatusBuilding,
		OrderStatusBuiltWaitingDeploy,
		OrderStatusRejected,
		OrderStatusQueued,
		OrderStatusDeploying,
		OrderStatusDeploySuccess,
		OrderStatusDeployFailed:
		return true
	default:
		return false
	}
}

// IsTerminal 封装当前模块的业务处理逻辑。
func (s OrderStatus) IsTerminal() bool {
	switch s {
	case OrderStatusSuccess,
		OrderStatusFailed,
		OrderStatusCancelled,
		OrderStatusRejected,
		OrderStatusDeploySuccess,
		OrderStatusDeployFailed:
		return true
	default:
		return false
	}
}

type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusSuccess   ExecutionStatus = "success"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusSkipped   ExecutionStatus = "skipped"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ExecutionStatus) Valid() bool {
	switch s {
	case ExecutionStatusPending,
		ExecutionStatusRunning,
		ExecutionStatusSuccess,
		ExecutionStatusFailed,
		ExecutionStatusCancelled,
		ExecutionStatusSkipped:
		return true
	default:
		return false
	}
}

// IsTerminal 封装当前模块的业务处理逻辑。
func (s ExecutionStatus) IsTerminal() bool {
	switch s {
	case ExecutionStatusSuccess, ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusSkipped:
		return true
	default:
		return false
	}
}

type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusSuccess StepStatus = "success"
	StepStatusFailed  StepStatus = "failed"
)

// Valid 封装当前模块的业务处理逻辑。
func (s StepStatus) Valid() bool {
	switch s {
	case StepStatusPending, StepStatusRunning, StepStatusSuccess, StepStatusFailed:
		return true
	default:
		return false
	}
}

type ValueSource string

const (
	ValueSourceApplication  ValueSource = "application"
	ValueSourceEnvironment  ValueSource = "environment"
	ValueSourceReleaseInput ValueSource = "release_input"
	ValueSourceFixed        ValueSource = "fixed"
	ValueSourceCIParam      ValueSource = "ci_param"
	ValueSourceBuiltin      ValueSource = "builtin"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ValueSource) Valid() bool {
	switch s {
	case ValueSourceApplication, ValueSourceEnvironment, ValueSourceReleaseInput, ValueSourceFixed, ValueSourceCIParam, ValueSourceBuiltin:
		return true
	default:
		return false
	}
}

type ReleaseOrder struct {
	ID                    string
	OrderNo               string
	ReleaseName           string
	PreviousOrderNo       string
	OperationType         OperationType
	SourceOrderID         string
	SourceOrderNo         string
	IsConcurrent          bool
	ConcurrentBatchNo     string
	ConcurrentBatchName   string
	ConcurrentBatchSeq    int
	CDProvider            string
	HasCIExecution        bool
	HasCDExecution        bool
	ApplicationID         string
	ApplicationName       string
	TemplateID            string
	TemplateName          string
	BindingID             string
	PipelineID            string
	EnvCode               string
	SonService            string
	GitRef                string
	ImageTag              string
	TriggerType           TriggerType
	Status                OrderStatus
	BusinessStatus        ReleaseBusinessStatus
	ApprovalRequired      bool
	ApprovalMode          TemplateApprovalMode
	ApprovalApproverIDs   []string
	ApprovalApproverNames []string
	ApprovedAt            *time.Time
	ApprovedBy            string
	RejectedAt            *time.Time
	RejectedBy            string
	RejectedReason        string
	QueuePosition         int
	QueuedReason          string
	Remark                string
	CreatorUserID         string
	TriggeredBy           string
	ExecutorUserID        string
	ExecutorName          string
	StartedAt             *time.Time
	FinishedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ScheduleMode string

const (
	ScheduleModeBuild       ScheduleMode = "build"
	ScheduleModeDeploy      ScheduleMode = "deploy"
	ScheduleModeBuildDeploy ScheduleMode = "build_deploy"
	ScheduleModeExecute     ScheduleMode = "execute"
)

func (s ScheduleMode) Valid() bool {
	switch s {
	case ScheduleModeBuild, ScheduleModeDeploy, ScheduleModeBuildDeploy, ScheduleModeExecute:
		return true
	default:
		return false
	}
}

type ScheduleStatus string

const (
	ScheduleStatusPendingApproval ScheduleStatus = "pending_approval"
	ScheduleStatusApproving       ScheduleStatus = "approving"
	ScheduleStatusScheduled       ScheduleStatus = "scheduled"
	ScheduleStatusDispatching     ScheduleStatus = "dispatching"
	ScheduleStatusDispatched      ScheduleStatus = "dispatched"
	ScheduleStatusExpired         ScheduleStatus = "expired"
	ScheduleStatusBlocked         ScheduleStatus = "blocked"
	ScheduleStatusFailed          ScheduleStatus = "failed"
	ScheduleStatusSkipped         ScheduleStatus = "skipped"
	ScheduleStatusCancelled       ScheduleStatus = "cancelled"
	ScheduleStatusRejected        ScheduleStatus = "rejected"
)

func (s ScheduleStatus) Valid() bool {
	switch s {
	case ScheduleStatusPendingApproval,
		ScheduleStatusApproving,
		ScheduleStatusScheduled,
		ScheduleStatusDispatching,
		ScheduleStatusDispatched,
		ScheduleStatusExpired,
		ScheduleStatusBlocked,
		ScheduleStatusFailed,
		ScheduleStatusSkipped,
		ScheduleStatusCancelled,
		ScheduleStatusRejected:
		return true
	default:
		return false
	}
}

func (s ScheduleStatus) IsActive() bool {
	switch s {
	case ScheduleStatusPendingApproval, ScheduleStatusApproving, ScheduleStatusScheduled, ScheduleStatusDispatching:
		return true
	default:
		return false
	}
}

type ReleaseOrderSchedule struct {
	ID                    string
	ScheduleNo            string
	ReleaseOrderID        string
	ReleaseOrderNo        string
	ApplicationID         string
	ApplicationName       string
	EnvCode               string
	TemplateID            string
	TemplateName          string
	ScheduleMode          ScheduleMode
	BuildScheduledAt      *time.Time
	DeployScheduledAt     *time.Time
	ExecuteScheduledAt    *time.Time
	CDConflictAt          *time.Time
	Timezone              string
	Status                ScheduleStatus
	ApprovalRequired      bool
	ApprovalMode          TemplateApprovalMode
	ApprovalApproverIDs   []string
	ApprovalApproverNames []string
	ApprovedAt            *time.Time
	ApprovedBy            string
	RejectedAt            *time.Time
	RejectedBy            string
	RejectedReason        string
	BuildDispatchedAt     *time.Time
	DeployDispatchedAt    *time.Time
	ExecuteDispatchedAt   *time.Time
	ExpiredAt             *time.Time
	CancelledAt           *time.Time
	CancelledBy           string
	LastError             string
	Remark                string
	CreatorUserID         string
	CreatorName           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ReleaseOrderScheduleApprovalRecord struct {
	ID             string
	ScheduleID     string
	Action         ReleaseOrderApprovalAction
	OperatorUserID string
	OperatorName   string
	Comment        string
	CreatedAt      time.Time
}

type DeploySnapshot struct {
	ID               string
	ReleaseOrderID   string
	Provider         string
	GitOpsType       GitOpsType
	ArgoCDInstanceID string
	GitOpsInstanceID string
	ArgoCDAppName    string
	RepoURL          string
	Branch           string
	SourcePath       string
	EnvCode          string
	SnapshotPayload  string
	CreatedAt        time.Time
}

type AppReleaseStateStatus string

const (
	AppReleaseStateStatusPendingConfirm AppReleaseStateStatus = "pending_confirm"
	AppReleaseStateStatusActive         AppReleaseStateStatus = "active"
	AppReleaseStateStatusSuperseded     AppReleaseStateStatus = "superseded"
)

// Valid 封装当前模块的业务处理逻辑。
func (s AppReleaseStateStatus) Valid() bool {
	switch s {
	case AppReleaseStateStatusPendingConfirm, AppReleaseStateStatusActive, AppReleaseStateStatusSuperseded:
		return true
	default:
		return false
	}
}

type AppReleaseState struct {
	ID                    string
	ReleaseOrderID        string
	ReleaseOrderNo        string
	ApplicationID         string
	ApplicationName       string
	EnvCode               string
	OperationType         OperationType
	TemplateID            string
	TemplateName          string
	CDProvider            string
	GitOpsType            GitOpsType
	HasCIExecution        bool
	HasCDExecution        bool
	GitRef                string
	ImageTag              string
	StateStatus           AppReleaseStateStatus
	IsCurrentLive         bool
	PreviousStateID       string
	ConfirmedAt           *time.Time
	ConfirmedBy           string
	ParamsSnapshotJSON    string
	ExecutionSnapshotJSON string
	DeploySnapshotJSON    string
	ResultSnapshotJSON    string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AppReleaseStateSummary struct {
	ApplicationID          string
	ApplicationName        string
	EnvCode                string
	CurrentStateID         string
	CurrentReleaseOrderID  string
	CurrentReleaseOrderNo  string
	CurrentImageTag        string
	CurrentConfirmedAt     *time.Time
	CurrentConfirmedBy     string
	PreviousStateID        string
	PreviousReleaseOrderID string
	PreviousReleaseOrderNo string
	PreviousImageTag       string
	PreviousConfirmedAt    *time.Time
}

type ReleaseOrderParam struct {
	ID                string
	ReleaseOrderID    string
	PipelineScope     PipelineScope
	BindingID         string
	ParamKey          string
	ExecutorParamName string
	ParamValue        string
	ValueSource       ValueSource
	CreatedAt         time.Time
}

type ReleaseOrderExecution struct {
	ID             string
	ReleaseOrderID string
	PipelineScope  PipelineScope
	BindingID      string
	BindingName    string
	Provider       string
	PipelineID     string
	Status         ExecutionStatus
	QueueURL       string
	BuildURL       string
	ExternalRunID  string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ExecutionLockScope string

const (
	ExecutionLockScopeApplication      ExecutionLockScope = "application"
	ExecutionLockScopeApplicationEnv   ExecutionLockScope = "application_env"
	ExecutionLockScopeGitOpsRepoBranch ExecutionLockScope = "gitops_repo_branch"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ExecutionLockScope) Valid() bool {
	switch s {
	case ExecutionLockScopeApplication, ExecutionLockScopeApplicationEnv, ExecutionLockScopeGitOpsRepoBranch:
		return true
	default:
		return false
	}
}

type ExecutionLockStatus string

const (
	ExecutionLockStatusActive   ExecutionLockStatus = "active"
	ExecutionLockStatusReleased ExecutionLockStatus = "released"
	ExecutionLockStatusExpired  ExecutionLockStatus = "expired"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ExecutionLockStatus) Valid() bool {
	switch s {
	case ExecutionLockStatusActive, ExecutionLockStatusReleased, ExecutionLockStatusExpired:
		return true
	default:
		return false
	}
}

type ReleaseExecutionLock struct {
	ID             string
	LockScope      ExecutionLockScope
	LockKey        string
	ApplicationID  string
	EnvCode        string
	ReleaseOrderID string
	ReleaseOrderNo string
	Status         ExecutionLockStatus
	OwnerType      string
	CreatedAt      time.Time
	ExpiredAt      *time.Time
	ReleasedAt     *time.Time
}

type ReleaseOrderStep struct {
	ID                 string
	ReleaseOrderID     string
	StepScope          StepScope
	ExecutionID        string
	StepCode           string
	StepName           string
	Status             StepStatus
	Message            string
	DetailLog          string
	RelatedTaskSummary string
	RelatedTaskIDs     []string
	RelatedTaskCount   int
	SortNo             int
	StartedAt          *time.Time
	FinishedAt         *time.Time
	CreatedAt          time.Time
}

type PipelineStageStatus string

const (
	PipelineStageStatusPending   PipelineStageStatus = "pending"
	PipelineStageStatusRunning   PipelineStageStatus = "running"
	PipelineStageStatusSuccess   PipelineStageStatus = "success"
	PipelineStageStatusFailed    PipelineStageStatus = "failed"
	PipelineStageStatusCancelled PipelineStageStatus = "cancelled"
	PipelineStageStatusSkipped   PipelineStageStatus = "skipped"
)

// Valid 封装当前模块的业务处理逻辑。
func (s PipelineStageStatus) Valid() bool {
	switch s {
	case PipelineStageStatusPending,
		PipelineStageStatusRunning,
		PipelineStageStatusSuccess,
		PipelineStageStatusFailed,
		PipelineStageStatusCancelled,
		PipelineStageStatusSkipped:
		return true
	default:
		return false
	}
}

type ReleaseOrderPipelineStage struct {
	ID             string
	ReleaseOrderID string
	ExecutionID    string
	PipelineScope  string
	ExecutorType   string
	StageKey       string
	StageName      string
	Status         PipelineStageStatus
	RawStatus      string
	SortNo         int
	DurationMillis int64
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ReleaseOrderPipelineStageLog struct {
	ReleaseOrderID string
	StageID        string
	PipelineScope  string
	ExecutorType   string
	StageName      string
	RawStatus      string
	Content        string
	HasMore        bool
	FetchedAt      time.Time
}

type TemplateStatus string

const (
	TemplateStatusActive   TemplateStatus = "active"
	TemplateStatusInactive TemplateStatus = "inactive"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateStatus) Valid() bool {
	switch s {
	case TemplateStatusActive, TemplateStatusInactive:
		return true
	default:
		return false
	}
}

type ReleaseTemplate struct {
	ID                    string
	Name                  string
	ApplicationID         string
	ApplicationName       string
	BindingID             string
	BindingName           string
	BindingType           string
	GitOpsType            GitOpsType
	Status                TemplateStatus
	ApprovalEnabled       bool
	ApprovalMode          TemplateApprovalMode
	ApprovalApproverIDs   []string
	ApprovalApproverNames []string
	Remark                string
	ParamCount            int
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type TemplateApprovalMode string

const (
	TemplateApprovalModeAny TemplateApprovalMode = "any"
	TemplateApprovalModeAll TemplateApprovalMode = "all"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateApprovalMode) Valid() bool {
	switch s {
	case "", TemplateApprovalModeAny, TemplateApprovalModeAll:
		return true
	default:
		return false
	}
}

type ReleaseTemplateBinding struct {
	ID            string
	TemplateID    string
	PipelineScope PipelineScope
	BindingID     string
	BindingName   string
	Provider      string
	PipelineID    string
	Enabled       bool
	SortNo        int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ReleaseTemplateParam struct {
	ID                string
	TemplateID        string
	TemplateBindingID string
	PipelineScope     PipelineScope
	BindingID         string
	// ExecutorParamDefID 关联统一的执行器参数定义。
	// 这里不再使用 pipeline_param_def 的命名，是为了让发布模板同时兼容 Jenkins 与 ArgoCD。
	ExecutorParamDefID string
	ParamKey           string
	ParamName          string
	ExecutorParamName  string
	ValueSource        TemplateParamValueSource
	SourceParamKey     string
	SourceParamName    string
	FixedValue         string
	Required           bool
	SortNo             int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type TemplateParamValueSource string

const (
	TemplateParamValueSourceReleaseInput TemplateParamValueSource = "release_input"
	TemplateParamValueSourceFixed        TemplateParamValueSource = "fixed"
	TemplateParamValueSourceCIParam      TemplateParamValueSource = "ci_param"
	TemplateParamValueSourceBuiltin      TemplateParamValueSource = "builtin"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateParamValueSource) Valid() bool {
	switch s {
	case TemplateParamValueSourceReleaseInput, TemplateParamValueSourceFixed, TemplateParamValueSourceCIParam, TemplateParamValueSourceBuiltin:
		return true
	default:
		return false
	}
}

type GitOpsRuleSourceFrom string

const (
	GitOpsRuleSourceCI      GitOpsRuleSourceFrom = "ci"
	GitOpsRuleSourceBuiltin GitOpsRuleSourceFrom = "builtin"
	GitOpsRuleSourceCDInput GitOpsRuleSourceFrom = "cd_input"
)

// Valid 封装当前模块的业务处理逻辑。
func (s GitOpsRuleSourceFrom) Valid() bool {
	switch s {
	case GitOpsRuleSourceCI, GitOpsRuleSourceBuiltin, GitOpsRuleSourceCDInput:
		return true
	default:
		return false
	}
}

type GitOpsType string

const (
	GitOpsTypeKustomize GitOpsType = "kustomize"
	GitOpsTypeHelm      GitOpsType = "helm"
)

// Valid 封装当前模块的业务处理逻辑。
func (s GitOpsType) Valid() bool {
	switch s {
	case "", GitOpsTypeKustomize, GitOpsTypeHelm:
		return true
	default:
		return false
	}
}

// ReleaseTemplateGitOpsRule 描述 CD=ArgoCD 时，标准平台 Key 与 YAML 字段之间的替换关系。
//
// 当前版本先把规则挂在“发布模板”上，而不是直接挂在应用上：
// 1. 模板已经定义了本次发布采用的 CI/CD 结构；
// 2. GitOps 字段替换与模板里的参数暴露方式强相关；
// 3. 这样可以让不同模板在同一应用下拥有不同的 GitOps 写回策略。
type ReleaseTemplateGitOpsRule struct {
	ID               string
	TemplateID       string
	PipelineScope    PipelineScope
	SourceParamKey   string
	SourceParamName  string
	SourceFrom       GitOpsRuleSourceFrom
	LocatorParamKey  string
	LocatorParamName string
	FilePathTemplate string
	DocumentKind     string
	DocumentName     string
	TargetPath       string
	ValueTemplate    string
	SortNo           int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TemplateHookType string

const (
	TemplateHookTypeAgentTask           TemplateHookType = "agent_task"
	TemplateHookTypeNotificationHook    TemplateHookType = "notification_hook"
	TemplateHookTypeWebhookNotification TemplateHookType = "webhook_notification"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateHookType) Valid() bool {
	switch s {
	case TemplateHookTypeAgentTask, TemplateHookTypeNotificationHook, TemplateHookTypeWebhookNotification:
		return true
	default:
		return false
	}
}

type TemplateHookTriggerCondition string

const (
	TemplateHookTriggerOnSuccess TemplateHookTriggerCondition = "on_success"
	TemplateHookTriggerOnFailed  TemplateHookTriggerCondition = "on_failed"
	TemplateHookTriggerAlways    TemplateHookTriggerCondition = "always"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateHookTriggerCondition) Valid() bool {
	switch s {
	case TemplateHookTriggerOnSuccess, TemplateHookTriggerOnFailed, TemplateHookTriggerAlways:
		return true
	default:
		return false
	}
}

type TemplateHookExecuteStage string

const (
	TemplateHookExecuteStagePostRelease   TemplateHookExecuteStage = "post_release"
	TemplateHookExecuteStageBuildComplete TemplateHookExecuteStage = "build_complete"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateHookExecuteStage) Valid() bool {
	switch s {
	case TemplateHookExecuteStagePostRelease, TemplateHookExecuteStageBuildComplete:
		return true
	default:
		return false
	}
}

// NormalizeTemplateHookExecuteStages 标准化输入值，保证后续逻辑使用统一格式。
func NormalizeTemplateHookExecuteStages(stages []TemplateHookExecuteStage, legacy TemplateHookExecuteStage) []TemplateHookExecuteStage {
	result := make([]TemplateHookExecuteStage, 0, len(stages))
	seen := make(map[TemplateHookExecuteStage]struct{}, len(stages))
	for _, item := range stages {
		if !item.Valid() {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	if len(result) > 0 {
		return result
	}
	if legacy.Valid() {
		return []TemplateHookExecuteStage{legacy}
	}
	return []TemplateHookExecuteStage{TemplateHookExecuteStagePostRelease}
}

// PrimaryTemplateHookExecuteStage 封装当前模块的业务处理逻辑。
func PrimaryTemplateHookExecuteStage(stages []TemplateHookExecuteStage, legacy TemplateHookExecuteStage) TemplateHookExecuteStage {
	normalized := NormalizeTemplateHookExecuteStages(stages, legacy)
	if len(normalized) == 0 {
		return TemplateHookExecuteStagePostRelease
	}
	return normalized[0]
}

// TemplateHookHasExecuteStage 封装当前模块的业务处理逻辑。
func TemplateHookHasExecuteStage(stages []TemplateHookExecuteStage, legacy TemplateHookExecuteStage, target TemplateHookExecuteStage) bool {
	for _, item := range NormalizeTemplateHookExecuteStages(stages, legacy) {
		if item == target {
			return true
		}
	}
	return false
}

type TemplateHookFailurePolicy string

const (
	TemplateHookFailurePolicyBlockRelease TemplateHookFailurePolicy = "block_release"
	TemplateHookFailurePolicyWarnOnly     TemplateHookFailurePolicy = "warn_only"
)

// Valid 封装当前模块的业务处理逻辑。
func (s TemplateHookFailurePolicy) Valid() bool {
	switch s {
	case TemplateHookFailurePolicyBlockRelease, TemplateHookFailurePolicyWarnOnly:
		return true
	default:
		return false
	}
}

type ReleaseTemplateHook struct {
	ID               string
	TemplateID       string
	HookType         TemplateHookType
	Name             string
	ExecuteStage     TemplateHookExecuteStage
	ExecuteStages    []TemplateHookExecuteStage
	TriggerCondition TemplateHookTriggerCondition
	FailurePolicy    TemplateHookFailurePolicy
	EnvCodes         []string
	TargetID         string
	TargetName       string
	WebhookMethod    string
	WebhookURL       string
	WebhookBody      string
	Note             string
	SortNo           int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ReleaseOrderApprovalAction string

const (
	ReleaseOrderApprovalActionSubmit  ReleaseOrderApprovalAction = "submit"
	ReleaseOrderApprovalActionApprove ReleaseOrderApprovalAction = "approve"
	ReleaseOrderApprovalActionReject  ReleaseOrderApprovalAction = "reject"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ReleaseOrderApprovalAction) Valid() bool {
	switch s {
	case ReleaseOrderApprovalActionSubmit, ReleaseOrderApprovalActionApprove, ReleaseOrderApprovalActionReject:
		return true
	default:
		return false
	}
}

type ReleaseOrderApprovalRecord struct {
	ID             string
	ReleaseOrderID string
	Action         ReleaseOrderApprovalAction
	OperatorUserID string
	OperatorName   string
	Comment        string
	CreatedAt      time.Time
}

type ReleaseOrderApprovalRecordSummary struct {
	ReleaseOrderApprovalRecord
	OrderNo         string
	OrderStatus     OrderStatus
	ApplicationID   string
	ApplicationName string
	EnvCode         string
	OperationType   OperationType
	TriggeredBy     string
}
