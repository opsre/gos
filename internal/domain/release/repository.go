package release

import (
	"context"
	"time"
)

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(
		ctx context.Context,
		order ReleaseOrder,
		executions []ReleaseOrderExecution,
		params []ReleaseOrderParam,
		steps []ReleaseOrderStep,
	) error
	UpdateEditable(
		ctx context.Context,
		order ReleaseOrder,
		executions []ReleaseOrderExecution,
		params []ReleaseOrderParam,
		steps []ReleaseOrderStep,
	) error
	CreateDeploySnapshot(ctx context.Context, snapshot DeploySnapshot) error
	ListDeploySnapshotsByOrderID(ctx context.Context, releaseOrderID string) ([]DeploySnapshot, error)
	GetDeploySnapshotByOrderID(ctx context.Context, releaseOrderID string) (DeploySnapshot, error)
	UpsertAppReleaseState(ctx context.Context, state AppReleaseState) error
	GetAppReleaseStateByOrderID(ctx context.Context, releaseOrderID string) (AppReleaseState, error)
	GetAppReleaseStateByID(ctx context.Context, id string) (AppReleaseState, error)
	GetCurrentAppReleaseState(ctx context.Context, applicationID string, envCode string) (AppReleaseState, error)
	IsLatestOrderByApplicationEnv(ctx context.Context, applicationID string, envCode string, releaseOrderID string) (bool, error)
	ConfirmAppReleaseState(ctx context.Context, releaseOrderID string, confirmedBy string, confirmedAt time.Time) (AppReleaseState, error)
	ListCurrentAppReleaseStateSummaries(ctx context.Context, applicationIDs []string) ([]AppReleaseStateSummary, error)
	UpdateConcurrentBatch(ctx context.Context, orderIDs []string, batchNo string, batchName string, isConcurrent bool) error
	ListByConcurrentBatchNo(ctx context.Context, batchNo string) ([]ReleaseOrder, error)
	FindActiveOrderByApplicationEnv(ctx context.Context, applicationID string, envCode string, excludeReleaseOrderID string) (ReleaseOrder, error)
	CountActiveOrdersByApplicationEnv(ctx context.Context, applicationID string, envCode string, excludeReleaseOrderID string) (int, error)
	FindActiveExecutionLock(ctx context.Context, lockKey string, excludeReleaseOrderID string, now time.Time) (ReleaseExecutionLock, error)
	AcquireExecutionLock(ctx context.Context, lock ReleaseExecutionLock, now time.Time) (ReleaseExecutionLock, bool, error)
	TouchExecutionLocksByOrderID(ctx context.Context, releaseOrderID string, expiredAt time.Time) error
	ReleaseExecutionLocksByOrderID(ctx context.Context, releaseOrderID string, status ExecutionLockStatus, releasedAt time.Time) error
	GetByID(ctx context.Context, id string) (ReleaseOrder, error)
	List(ctx context.Context, filter ListFilter) ([]ReleaseOrder, int64, error)
	ListStats(ctx context.Context, filter ListFilter) (ReleaseOrderStats, error)
	ListTrackableOrders(ctx context.Context, page int, pageSize int) ([]ReleaseOrder, int64, error)
	UpdateStatus(
		ctx context.Context,
		id string,
		status OrderStatus,
		startedAt *time.Time,
		finishedAt *time.Time,
		updatedAt time.Time,
	) (ReleaseOrder, error)
	UpdateExecutor(
		ctx context.Context,
		id string,
		executorUserID string,
		executorName string,
		updatedAt time.Time,
	) (ReleaseOrder, error)
	UpdateApprovalStatus(
		ctx context.Context,
		id string,
		status OrderStatus,
		approvedAt *time.Time,
		approvedBy string,
		rejectedAt *time.Time,
		rejectedBy string,
		rejectedReason string,
		updatedAt time.Time,
	) (ReleaseOrder, error)
	ListExecutions(ctx context.Context, releaseOrderID string) ([]ReleaseOrderExecution, error)
	GetExecutionByScope(ctx context.Context, releaseOrderID string, scope PipelineScope) (ReleaseOrderExecution, error)
	ClaimExecutionByScope(
		ctx context.Context,
		releaseOrderID string,
		scope PipelineScope,
		startedAt time.Time,
		updatedAt time.Time,
	) (ReleaseOrderExecution, bool, error)
	UpdateExecutionByScope(
		ctx context.Context,
		releaseOrderID string,
		scope PipelineScope,
		input ExecutionUpdateInput,
	) (ReleaseOrderExecution, error)
	UpsertArtifactMetadata(ctx context.Context, item ReleaseOrderArtifactMetadata) (ReleaseOrderArtifactMetadata, error)
	GetArtifactMetadataByID(ctx context.Context, id string) (ReleaseOrderArtifactMetadata, error)
	DeleteArtifactMetadata(ctx context.Context, id string) error
	ListArtifactMetadata(ctx context.Context, releaseOrderID string) ([]ReleaseOrderArtifactMetadata, error)
	ListArtifactMetadataSummaries(ctx context.Context, filter ArtifactMetadataListFilter) ([]ReleaseOrderArtifactMetadataSummary, int64, error)
	ListParams(ctx context.Context, releaseOrderID string) ([]ReleaseOrderParam, error)
	ListSteps(ctx context.Context, releaseOrderID string) ([]ReleaseOrderStep, error)
	GetStepByCode(ctx context.Context, releaseOrderID string, stepCode string) (ReleaseOrderStep, error)
	ReplaceSteps(ctx context.Context, releaseOrderID string, steps []ReleaseOrderStep) error
	ReplacePipelineStages(ctx context.Context, releaseOrderID string, stages []ReleaseOrderPipelineStage) error
	ListPipelineStages(ctx context.Context, releaseOrderID string) ([]ReleaseOrderPipelineStage, error)
	GetPipelineStageByID(ctx context.Context, releaseOrderID string, stageID string) (ReleaseOrderPipelineStage, error)
	UpdateStep(
		ctx context.Context,
		releaseOrderID string,
		stepCode string,
		input StepUpdateInput,
	) (ReleaseOrderStep, error)
	CreateTemplate(
		ctx context.Context,
		template ReleaseTemplate,
		bindings []ReleaseTemplateBinding,
		params []ReleaseTemplateParam,
		gitopsRules []ReleaseTemplateGitOpsRule,
		hooks []ReleaseTemplateHook,
	) error
	GetTemplateByID(
		ctx context.Context,
		id string,
	) (ReleaseTemplate, []ReleaseTemplateBinding, []ReleaseTemplateParam, []ReleaseTemplateGitOpsRule, []ReleaseTemplateHook, error)
	ListTemplates(ctx context.Context, filter TemplateListFilter) ([]ReleaseTemplate, int64, error)
	UpdateTemplate(
		ctx context.Context,
		template ReleaseTemplate,
		bindings []ReleaseTemplateBinding,
		params []ReleaseTemplateParam,
		gitopsRules []ReleaseTemplateGitOpsRule,
		hooks []ReleaseTemplateHook,
	) error
	DeleteTemplate(ctx context.Context, id string) error
	CreateApprovalRecord(ctx context.Context, item ReleaseOrderApprovalRecord) error
	ListApprovalRecords(ctx context.Context, releaseOrderID string) ([]ReleaseOrderApprovalRecord, error)
	ListApprovalRecordSummaries(ctx context.Context, filter ApprovalRecordListFilter) ([]ReleaseOrderApprovalRecordSummary, int64, error)
	CreateApprovalFlowDefinition(ctx context.Context, item ApprovalFlowDefinition) error
	UpdateApprovalFlowDefinition(ctx context.Context, item ApprovalFlowDefinition) error
	GetApprovalFlowDefinitionByID(ctx context.Context, id string) (ApprovalFlowDefinition, error)
	ListApprovalFlowDefinitions(ctx context.Context, status ApprovalFlowStatus) ([]ApprovalFlowDefinition, error)
	CreateApprovalFlowInstance(ctx context.Context, item ReleaseOrderApprovalFlowInstance) error
	GetApprovalFlowInstanceByOrderID(ctx context.Context, releaseOrderID string) (ReleaseOrderApprovalFlowInstance, error)
	UpdateApprovalFlowInstance(ctx context.Context, item ReleaseOrderApprovalFlowInstance) error
	CreateApprovalFlowTask(ctx context.Context, item ReleaseOrderApprovalFlowTask) error
	GetApprovalFlowTaskByID(ctx context.Context, id string) (ReleaseOrderApprovalFlowTask, error)
	ListApprovalFlowTasks(ctx context.Context, releaseOrderID string) ([]ReleaseOrderApprovalFlowTask, error)
	UpdateApprovalFlowTask(ctx context.Context, item ReleaseOrderApprovalFlowTask) error
	CreateApprovalFlowTaskRecord(ctx context.Context, item ReleaseOrderApprovalFlowTaskRecord) error
	ListApprovalFlowTaskRecords(ctx context.Context, taskID string) ([]ReleaseOrderApprovalFlowTaskRecord, error)
	GetApplicationApprovalFlowID(ctx context.Context, applicationID string) (string, error)
	UpsertApplicationApprovalFlowID(ctx context.Context, applicationID string, approvalFlowID string, updatedAt time.Time) error
	CreateSchedule(ctx context.Context, item ReleaseOrderSchedule) error
	UpdateSchedule(ctx context.Context, item ReleaseOrderSchedule) error
	GetScheduleByID(ctx context.Context, id string) (ReleaseOrderSchedule, error)
	GetActiveScheduleByOrderID(ctx context.Context, releaseOrderID string) (ReleaseOrderSchedule, error)
	FindActiveScheduleCDConflict(ctx context.Context, applicationID string, envCode string, cdConflictAt time.Time, excludeScheduleID string) (ReleaseOrderSchedule, error)
	ListSchedules(ctx context.Context, filter ScheduleListFilter) ([]ReleaseOrderSchedule, int64, error)
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]ReleaseOrderSchedule, error)
	UpdateScheduleStatus(ctx context.Context, id string, status ScheduleStatus, lastError string, updatedAt time.Time) (ReleaseOrderSchedule, error)
	CreateScheduleApprovalRecord(ctx context.Context, item ReleaseOrderScheduleApprovalRecord) error
	ListScheduleApprovalRecords(ctx context.Context, scheduleID string) ([]ReleaseOrderScheduleApprovalRecord, error)
}

type ReleaseOrderStats struct {
	Total     int64
	Pending   int64
	Running   int64
	Success   int64
	Failed    int64
	Cancelled int64
}

type ListFilter struct {
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []ApplicationEnvScope
	VisibleToUserID             string
	ApprovalApproverUserID      string
	CreatorUserID               string
	Keyword                     string
	ConcurrentBatchNo           string
	ConcurrentBatchName         string
	TriggeredBy                 string
	BindingID                   string
	EnvCode                     string
	OperationType               OperationType
	Status                      OrderStatus
	TriggerType                 TriggerType
	CreatedAtFrom               *time.Time
	CreatedAtTo                 *time.Time
	Page                        int
	PageSize                    int
}

type ArtifactMetadataListFilter struct {
	ProjectID                   string
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []ApplicationEnvScope
	VisibleToUserID             string
	ReleaseOrderID              string
	Keyword                     string
	ArtifactName                string
	ArtifactType                string
	PipelineScope               PipelineScope
	RepositoryID                string
	CreatedAtFrom               *time.Time
	CreatedAtTo                 *time.Time
	Page                        int
	PageSize                    int
}

type StepUpdateInput struct {
	Status     StepStatus
	Message    string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

type ExecutionUpdateInput struct {
	Status        ExecutionStatus
	QueueURL      string
	BuildURL      string
	ExternalRunID string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	UpdatedAt     time.Time
}

type TemplateListFilter struct {
	ApplicationID  string
	ApplicationIDs []string
	BindingID      string
	Status         TemplateStatus
	Page           int
	PageSize       int
}

type ApprovalRecordListFilter struct {
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []ApplicationEnvScope
	VisibleToUserID             string
	OperatorUserID              string
	Page                        int
	PageSize                    int
}

type ApprovalWorkbenchListFilter struct {
	UserID   string
	Page     int
	PageSize int
}

// ApprovalWorkbenchRepository 提供审批待办需要的跨发布单任务聚合查询。
// 独立于 Repository，避免只关注发布执行的轻量测试仓储被迫实现工作台查询。
type ApprovalWorkbenchRepository interface {
	ListApprovalWorkbenchTasks(ctx context.Context, filter ApprovalWorkbenchListFilter) ([]ReleaseApprovalWorkbenchTask, int64, error)
	ListApprovalWorkbenchRecords(ctx context.Context, filter ApprovalWorkbenchListFilter) ([]ReleaseApprovalWorkbenchRecord, int64, error)
}

type ScheduleListFilter struct {
	ApplicationID          string
	ApplicationIDs         []string
	VisibleToUserID        string
	ApprovalApproverUserID string
	CreatorUserID          string
	Keyword                string
	EnvCode                string
	ScheduleMode           ScheduleMode
	Status                 ScheduleStatus
	ScheduledAtFrom        *time.Time
	ScheduledAtTo          *time.Time
	Page                   int
	PageSize               int
}

type ApplicationEnvScope struct {
	ApplicationID string
	EnvCode       string
}
