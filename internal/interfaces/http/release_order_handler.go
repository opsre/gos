package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	appdomain "gos/internal/domain/application"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
	"gos/internal/support/logx"
)

type ReleaseOrderHandler struct {
	manager     *usecase.ReleaseOrderManager
	logStreamer ReleaseOrderLogStreamer
	authz       RequestAuthorizer
	access      ReleaseParamAccessResolver
	realtime    *releaseOrderRealtimeCoordinator
}

type ReleaseParamAccessResolver interface {
	ResolveParamAccess(
		ctx context.Context,
		user userdomain.User,
		applicationID string,
		paramKey string,
	) (canView bool, canEdit bool, err error)
}

type ReleaseOrderLogStreamer interface {
	Stream(
		ctx context.Context,
		input usecase.StreamReleaseOrderLogInput,
		emit func(event usecase.ReleaseOrderLogEvent) error,
	) error
}

// NewReleaseOrderHandler 创建并返回对应组件实例。
func NewReleaseOrderHandler(
	manager *usecase.ReleaseOrderManager,
	logStreamer ReleaseOrderLogStreamer,
	authz RequestAuthorizer,
	access ReleaseParamAccessResolver,
) *ReleaseOrderHandler {
	handler := &ReleaseOrderHandler{
		manager:     manager,
		logStreamer: logStreamer,
		authz:       authz,
		access:      access,
	}
	handler.realtime = newReleaseOrderRealtimeCoordinator(handler.loadReleaseOrderRealtimeSnapshot, nil)
	return handler
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *ReleaseOrderHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/applications/:id/release-orders/rollback", h.CreateRollbackByApplication)
	router.POST("/applications/:id/rollback-capability", h.GetApplicationRollbackCapability)
	router.POST("/applications/:id/rollback-precheck", h.GetApplicationRollbackPrecheck)
	router.POST("/applications/:id/rollback-orders", h.CreateApplicationRollbackOrder)
	router.GET("/app-release-states/summaries", h.ListAppReleaseStateSummaries)
	router.POST("/release-orders", h.Create)
	router.POST("/release-orders/batch-create", h.BatchCreate)
	router.PUT("/release-orders/:id", h.Update)
	router.DELETE("/release-orders/:id", h.Delete)
	router.POST("/release-orders/batch-execute", h.BatchExecute)
	router.POST("/release-orders/batch-delete", h.BatchDelete)
	router.GET("/artifacts/release-order-metadata", h.ListArtifactMetadataSummaries)
	router.POST("/release-orders/:id/rollback", h.CreateRollbackByOrder)
	router.POST("/release-orders/:id/replay", h.CreateReplayByOrder)
	router.GET("/release-orders", h.List)
	router.GET("/release-orders/stats", h.Stats)
	router.GET("/release-approval-flows", h.ListApprovalFlows)
	router.POST("/release-approval-flows", h.CreateApprovalFlow)
	router.PUT("/release-approval-flows/:id", h.UpdateApprovalFlow)
	router.GET("/release-approval-tasks", h.ListApprovalWorkbench)
	router.GET("/release-approval-records", h.ListApprovalRecordSummaries)
	router.GET("/release-order-schedules", h.ListSchedules)
	router.GET("/release-order-schedules/schedulable-release-orders", h.ListSchedulableScheduleOrders)
	router.PUT("/release-order-schedules/:schedule_id", h.UpdateSchedule)
	router.POST("/release-order-schedules/:schedule_id/cancel", h.CancelSchedule)
	router.POST("/release-order-schedules/:schedule_id/submit-approval", h.SubmitScheduleApproval)
	router.POST("/release-order-schedules/:schedule_id/approve", h.ApproveSchedule)
	router.POST("/release-order-schedules/:schedule_id/reject", h.RejectSchedule)
	router.GET("/release-order-schedules/:schedule_id/approval-records", h.ListScheduleApprovalRecords)
	router.POST("/release-orders/:id/schedule", h.CreateSchedule)
	router.GET("/release-orders/:id/schedule", h.GetActiveSchedule)
	router.GET("/release-orders/:id/realtime-snapshot", h.GetRealtimeSnapshot)
	router.GET("/release-orders/:id/events", h.StreamRealtimeEvents)
	router.GET("/release-orders/:id", h.GetByID)
	router.GET("/release-orders/:id/precheck", h.GetPrecheck)
	router.GET("/release-orders/:id/concurrent-batch-progress", h.GetConcurrentBatchProgress)
	router.GET("/release-orders/:id/approval-records", h.ListApprovalRecords)
	router.GET("/release-orders/:id/approval-flow", h.GetApprovalFlow)
	router.POST("/release-orders/:id/approval-flow/tasks/:task_id/approve", h.ApproveApprovalFlowTask)
	router.POST("/release-orders/:id/approval-flow/tasks/:task_id/reject", h.RejectApprovalFlowTask)
	router.POST("/release-orders/:id/submit-approval", h.SubmitApproval)
	router.POST("/release-orders/:id/approve", h.Approve)
	router.POST("/release-orders/:id/reject", h.Reject)
	router.POST("/release-orders/:id/cancel", h.Cancel)
	router.POST("/release-orders/:id/confirm-live", h.ConfirmLive)
	router.POST("/release-orders/:id/build", h.Build)
	router.POST("/release-orders/:id/deploy", h.Deploy)
	router.POST("/release-orders/:id/execute", h.Execute)
	router.GET("/release-orders/:id/logs/stream", h.StreamLogs)

	router.GET("/release-orders/:id/params", h.ListParams)
	router.GET("/release-orders/:id/value-progress", h.ListValueProgress)
	router.GET("/release-orders/:id/executions", h.ListExecutions)
	router.GET("/release-orders/:id/artifact-metadata", h.ListArtifactMetadata)
	router.POST("/release-orders/:id/artifact-metadata", h.RecordArtifactMetadata)
	router.DELETE("/release-orders/:id/artifact-metadata/:artifact_id", h.DeleteArtifactMetadata)
	router.GET("/release-orders/:id/steps", h.ListSteps)
	router.GET("/release-orders/:id/pipeline-stages", h.ListPipelineStages)
	router.GET("/release-orders/:id/pipeline-stages/:stage_id/log", h.GetPipelineStageLog)
	router.POST("/release-orders/:id/pipeline-stages/:stage_id/diagnoses", h.CreatePipelineStageDiagnosis)
	router.GET("/release-orders/:id/pipeline-stages/:stage_id/diagnoses/latest", h.GetLatestPipelineStageDiagnosis)
	router.POST("/release-orders/:id/pipeline-stages/:stage_id/diagnoses/:diagnosis_id/follow-up", h.FollowUpPipelineStageDiagnosis)
	router.GET("/release-orders/:id/pipeline-stage-diagnoses/:diagnosis_id", h.GetPipelineStageDiagnosis)
	router.POST("/release-orders/:id/steps/:step_code/start", h.StartStep)
	router.POST("/release-orders/:id/steps/:step_code/finish", h.FinishStep)
}

type CreateReleaseOrderRequest struct {
	ApplicationID string                           `json:"application_id"`
	TemplateID    string                           `json:"template_id"`
	ReleaseName   string                           `json:"release_name"`
	EnvCode       string                           `json:"env_code"`
	ProjectName   string                           `json:"project_name"`
	GitRef        string                           `json:"git_ref"`
	ImageTag      string                           `json:"image_tag"`
	TriggerType   string                           `json:"trigger_type"`
	Remark        string                           `json:"remark"`
	TriggeredBy   string                           `json:"triggered_by"`
	Params        []CreateReleaseOrderParamRequest `json:"params"`
	Steps         []CreateReleaseOrderStepRequest  `json:"steps"`
}

type BatchCreateReleaseOrdersRequest struct {
	Orders []CreateReleaseOrderRequest `json:"orders"`
}

type CreateReleaseOrderParamRequest struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

type CreateReleaseOrderStepRequest struct {
	StepCode string `json:"step_code"`
	StepName string `json:"step_name"`
	SortNo   int    `json:"sort_no"`
}

type StartReleaseOrderStepRequest struct {
	Message string `json:"message"`
}

type FinishReleaseOrderStepRequest struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type RecordReleaseOrderArtifactMetadataRequest struct {
	ExecutionID      string         `json:"execution_id"`
	PipelineScope    string         `json:"pipeline_scope"`
	ArtifactName     string         `json:"artifact_name"`
	ArtifactType     string         `json:"artifact_type"`
	ArtifactVersion  string         `json:"artifact_version"`
	ArtifactURL      string         `json:"artifact_url"`
	RepositoryID     string         `json:"repository_id"`
	RepositoryName   string         `json:"repository_name"`
	Bucket           string         `json:"bucket"`
	ObjectKey        string         `json:"object_key"`
	Checksum         string         `json:"checksum"`
	ChecksumType     string         `json:"checksum_type"`
	SizeBytes        int64          `json:"size_bytes"`
	BuildNumber      string         `json:"build_number"`
	AdditionalFields map[string]any `json:"metadata"`
}

type ApplicationRollbackRequest struct {
	EnvCode string `json:"env_code"`
	Action  string `json:"action"`
}

type BatchExecuteReleaseOrdersRequest struct {
	OrderIDs           []string `json:"order_ids"`
	BatchName          string   `json:"batch_name"`
	StagedDispatchMode string   `json:"staged_dispatch_mode"`
}

type BatchDeleteReleaseOrdersRequest struct {
	OrderIDs []string `json:"order_ids"`
}

type ReleaseOrderResponse struct {
	ID                    string                               `json:"id"`
	OrderNo               string                               `json:"order_no"`
	ReleaseName           string                               `json:"release_name"`
	PreviousOrderNo       string                               `json:"previous_order_no"`
	OperationType         string                               `json:"operation_type"`
	SourceOrderID         string                               `json:"source_order_id"`
	SourceOrderNo         string                               `json:"source_order_no"`
	IsConcurrent          bool                                 `json:"is_concurrent"`
	ConcurrentBatchNo     string                               `json:"concurrent_batch_no"`
	ConcurrentBatchName   string                               `json:"concurrent_batch_name"`
	ConcurrentBatchSeq    int                                  `json:"concurrent_batch_seq"`
	CDProvider            string                               `json:"cd_provider"`
	HasCIExecution        bool                                 `json:"has_ci_execution"`
	HasCDExecution        bool                                 `json:"has_cd_execution"`
	ApplicationID         string                               `json:"application_id"`
	ApplicationName       string                               `json:"application_name"`
	TemplateID            string                               `json:"template_id"`
	TemplateName          string                               `json:"template_name"`
	BindingID             string                               `json:"binding_id"`
	PipelineID            string                               `json:"pipeline_id"`
	EnvCode               string                               `json:"env_code"`
	ProjectName           string                               `json:"project_name"`
	GitRef                string                               `json:"git_ref"`
	ImageTag              string                               `json:"image_tag"`
	TriggerType           string                               `json:"trigger_type"`
	Status                string                               `json:"status"`
	BusinessStatus        string                               `json:"business_status"`
	ApprovalRequired      bool                                 `json:"approval_required"`
	ApprovalMode          string                               `json:"approval_mode"`
	ApprovalApproverIDs   []string                             `json:"approval_approver_ids"`
	ApprovalApproverNames []string                             `json:"approval_approver_names"`
	ApprovedAt            *time.Time                           `json:"approved_at"`
	ApprovedBy            string                               `json:"approved_by"`
	RejectedAt            *time.Time                           `json:"rejected_at"`
	RejectedBy            string                               `json:"rejected_by"`
	RejectedReason        string                               `json:"rejected_reason"`
	QueuePosition         int                                  `json:"queue_position"`
	QueuedReason          string                               `json:"queued_reason"`
	Remark                string                               `json:"remark"`
	CreatorUserID         string                               `json:"creator_user_id"`
	TriggeredBy           string                               `json:"triggered_by"`
	LiveStateStatus       string                               `json:"live_state_status"`
	LiveStateIsCurrent    bool                                 `json:"live_state_is_current"`
	LiveStateCanConfirm   bool                                 `json:"live_state_can_confirm"`
	LiveStateConfirmedAt  *time.Time                           `json:"live_state_confirmed_at"`
	LiveStateConfirmedBy  string                               `json:"live_state_confirmed_by"`
	DeploySnapshots       []ReleaseOrderDeploySnapshotResponse `json:"deploy_snapshots"`
	StartedAt             *time.Time                           `json:"started_at"`
	FinishedAt            *time.Time                           `json:"finished_at"`
	CreatedAt             time.Time                            `json:"created_at"`
	UpdatedAt             time.Time                            `json:"updated_at"`
}

type ReleaseOrderDeploySnapshotResponse struct {
	ID               string    `json:"id"`
	ReleaseOrderID   string    `json:"release_order_id"`
	Provider         string    `json:"provider"`
	GitOpsType       string    `json:"gitops_type"`
	ArgoCDInstanceID string    `json:"argocd_instance_id"`
	GitOpsInstanceID string    `json:"gitops_instance_id"`
	ArgoCDAppName    string    `json:"argocd_app_name"`
	RepoURL          string    `json:"repo_url"`
	Branch           string    `json:"branch"`
	SourcePath       string    `json:"source_path"`
	EnvCode          string    `json:"env_code"`
	ImageVersion     string    `json:"image_version"`
	RulesCount       int       `json:"rules_count"`
	CreatedAt        time.Time `json:"created_at"`
}

type ReleaseOrderParamResponse struct {
	ID                string    `json:"id"`
	ReleaseOrderID    string    `json:"release_order_id"`
	PipelineScope     string    `json:"pipeline_scope"`
	BindingID         string    `json:"binding_id"`
	ParamKey          string    `json:"param_key"`
	ExecutorParamName string    `json:"executor_param_name"`
	ParamValue        string    `json:"param_value"`
	ValueSource       string    `json:"value_source"`
	CreatedAt         time.Time `json:"created_at"`
}

type ReleaseOrderStepResponse struct {
	ID                 string     `json:"id"`
	ReleaseOrderID     string     `json:"release_order_id"`
	StepScope          string     `json:"step_scope"`
	ExecutionID        string     `json:"execution_id"`
	StepCode           string     `json:"step_code"`
	StepName           string     `json:"step_name"`
	Status             string     `json:"status"`
	Message            string     `json:"message"`
	DetailLog          string     `json:"detail_log"`
	RelatedTaskSummary string     `json:"related_task_summary"`
	RelatedTaskIDs     []string   `json:"related_task_ids"`
	RelatedTaskCount   int        `json:"related_task_count"`
	SortNo             int        `json:"sort_no"`
	StartedAt          *time.Time `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

type ReleaseOrderDataResponse struct {
	Data ReleaseOrderResponse `json:"data"`
}

type BatchCreateReleaseOrderFailureResponse struct {
	Index       int    `json:"index"`
	ReleaseName string `json:"release_name"`
	Error       string `json:"error"`
}

type ReleaseOrderBatchCreateResponse struct {
	Data struct {
		Orders   []ReleaseOrderResponse                   `json:"orders"`
		Failures []BatchCreateReleaseOrderFailureResponse `json:"failures"`
	} `json:"data"`
}

type ReleaseOrderListResponse struct {
	Data     []ReleaseOrderResponse `json:"data"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Total    int64                  `json:"total"`
}

type ReleaseOrderStatsResponse struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
}

type ReleaseOrderParamListResponse struct {
	Data []ReleaseOrderParamResponse `json:"data"`
}

type ReleaseOrderExecutionResponse struct {
	ID             string     `json:"id"`
	ReleaseOrderID string     `json:"release_order_id"`
	PipelineScope  string     `json:"pipeline_scope"`
	BindingID      string     `json:"binding_id"`
	BindingName    string     `json:"binding_name"`
	Provider       string     `json:"provider"`
	PipelineID     string     `json:"pipeline_id"`
	Status         string     `json:"status"`
	QueueURL       string     `json:"queue_url"`
	BuildURL       string     `json:"build_url"`
	ExternalRunID  string     `json:"external_run_id"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ReleaseOrderExecutionListResponse struct {
	Data []ReleaseOrderExecutionResponse `json:"data"`
}

type ReleaseOrderArtifactMetadataResponse struct {
	ID               string         `json:"id"`
	ReleaseOrderID   string         `json:"release_order_id"`
	ExecutionID      string         `json:"execution_id"`
	PipelineScope    string         `json:"pipeline_scope"`
	ArtifactName     string         `json:"artifact_name"`
	ArtifactType     string         `json:"artifact_type"`
	ArtifactVersion  string         `json:"artifact_version"`
	ArtifactURL      string         `json:"artifact_url"`
	RepositoryID     string         `json:"repository_id"`
	RepositoryName   string         `json:"repository_name"`
	Bucket           string         `json:"bucket"`
	ObjectKey        string         `json:"object_key"`
	Checksum         string         `json:"checksum"`
	ChecksumType     string         `json:"checksum_type"`
	SizeBytes        int64          `json:"size_bytes"`
	BuildNumber      string         `json:"build_number"`
	AdditionalFields map[string]any `json:"metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type ReleaseOrderArtifactMetadataDataResponse struct {
	Data ReleaseOrderArtifactMetadataResponse `json:"data"`
}

type ReleaseOrderArtifactMetadataListResponse struct {
	Data []ReleaseOrderArtifactMetadataResponse `json:"data"`
}

type ReleaseOrderArtifactMetadataSummaryResponse struct {
	ReleaseOrderArtifactMetadataResponse
	ReleaseOrderNo     string `json:"release_order_no"`
	ReleaseName        string `json:"release_name"`
	ReleaseDisplayName string `json:"release_display_name"`
	ApplicationID      string `json:"application_id"`
	ApplicationName    string `json:"application_name"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectKey         string `json:"project_key"`
	EnvCode            string `json:"env_code"`
	OrderStatus        string `json:"order_status"`
}

type ReleaseOrderArtifactMetadataSummaryListResponse struct {
	Data     []ReleaseOrderArtifactMetadataSummaryResponse `json:"data"`
	Page     int                                           `json:"page"`
	PageSize int                                           `json:"page_size"`
	Total    int64                                         `json:"total"`
}

type ReleaseOrderStepListResponse struct {
	Data []ReleaseOrderStepResponse `json:"data"`
}

type ReleaseOrderStepActionResponse struct {
	Data struct {
		Order ReleaseOrderResponse     `json:"order"`
		Step  ReleaseOrderStepResponse `json:"step"`
	} `json:"data"`
}

type ReleaseOrderPrecheckItemResponse struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ReleaseOrderPrecheckResponse struct {
	Data struct {
		OrderID          string                             `json:"order_id"`
		OrderNo          string                             `json:"order_no"`
		Executable       bool                               `json:"executable"`
		WaitingForLock   bool                               `json:"waiting_for_lock"`
		AheadCount       int                                `json:"ahead_count"`
		LockEnabled      bool                               `json:"lock_enabled"`
		LockScope        string                             `json:"lock_scope"`
		ConflictStrategy string                             `json:"conflict_strategy"`
		LockKey          string                             `json:"lock_key"`
		ConflictOrderNo  string                             `json:"conflict_order_no"`
		ConflictMessage  string                             `json:"conflict_message"`
		Items            []ReleaseOrderPrecheckItemResponse `json:"items"`
	} `json:"data"`
}

type ReleaseOrderConcurrentBatchProgressItemResponse struct {
	OrderID             string     `json:"order_id"`
	OrderNo             string     `json:"order_no"`
	ApplicationID       string     `json:"application_id"`
	ApplicationName     string     `json:"application_name"`
	EnvCode             string     `json:"env_code"`
	Status              string     `json:"status"`
	OperationType       string     `json:"operation_type"`
	ConcurrentBatchSeq  int        `json:"concurrent_batch_seq"`
	QueueState          string     `json:"queue_state"`
	QueuePosition       int        `json:"queue_position"`
	HasRunningExecution bool       `json:"has_running_execution"`
	StartedAt           *time.Time `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
}

type ReleaseOrderConcurrentBatchProgressResponse struct {
	Data struct {
		OrderID      string                                            `json:"order_id"`
		OrderNo      string                                            `json:"order_no"`
		BatchNo      string                                            `json:"batch_no"`
		BatchName    string                                            `json:"batch_name"`
		IsConcurrent bool                                              `json:"is_concurrent"`
		Total        int                                               `json:"total"`
		Queued       int                                               `json:"queued"`
		Executing    int                                               `json:"executing"`
		Success      int                                               `json:"success"`
		Failed       int                                               `json:"failed"`
		Cancelled    int                                               `json:"cancelled"`
		Items        []ReleaseOrderConcurrentBatchProgressItemResponse `json:"items"`
	} `json:"data"`
}

type ReleaseOrderBatchExecuteResponse struct {
	Data struct {
		BatchNo        string                 `json:"batch_no"`
		BatchName      string                 `json:"batch_name"`
		Orders         []ReleaseOrderResponse `json:"orders"`
		DispatchErrors []string               `json:"dispatch_errors"`
	} `json:"data"`
}

type ReleaseOrderApprovalActionRequest struct {
	Comment string `json:"comment"`
}

type ReleaseOrderApprovalRecordResponse struct {
	ID             string    `json:"id"`
	ReleaseOrderID string    `json:"release_order_id"`
	Action         string    `json:"action"`
	OperatorUserID string    `json:"operator_user_id"`
	OperatorName   string    `json:"operator_name"`
	Comment        string    `json:"comment"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReleaseOrderApprovalRecordListResponse struct {
	Data []ReleaseOrderApprovalRecordResponse `json:"data"`
}

type ReleaseOrderScheduleRequest struct {
	ScheduleMode       string     `json:"schedule_mode"`
	BuildScheduledAt   *time.Time `json:"build_scheduled_at"`
	DeployScheduledAt  *time.Time `json:"deploy_scheduled_at"`
	ExecuteScheduledAt *time.Time `json:"execute_scheduled_at"`
	Timezone           string     `json:"timezone"`
	Remark             string     `json:"remark"`
}

type ReleaseOrderScheduleResponse struct {
	ID                    string     `json:"id"`
	ScheduleNo            string     `json:"schedule_no"`
	ReleaseOrderID        string     `json:"release_order_id"`
	ReleaseOrderNo        string     `json:"release_order_no"`
	ApplicationID         string     `json:"application_id"`
	ApplicationName       string     `json:"application_name"`
	EnvCode               string     `json:"env_code"`
	TemplateID            string     `json:"template_id"`
	TemplateName          string     `json:"template_name"`
	ScheduleMode          string     `json:"schedule_mode"`
	BuildScheduledAt      *time.Time `json:"build_scheduled_at"`
	DeployScheduledAt     *time.Time `json:"deploy_scheduled_at"`
	ExecuteScheduledAt    *time.Time `json:"execute_scheduled_at"`
	CDConflictAt          *time.Time `json:"cd_conflict_at"`
	Timezone              string     `json:"timezone"`
	Status                string     `json:"status"`
	ApprovalRequired      bool       `json:"approval_required"`
	ApprovalMode          string     `json:"approval_mode"`
	ApprovalApproverIDs   []string   `json:"approval_approver_ids"`
	ApprovalApproverNames []string   `json:"approval_approver_names"`
	ApprovedAt            *time.Time `json:"approved_at"`
	ApprovedBy            string     `json:"approved_by"`
	RejectedAt            *time.Time `json:"rejected_at"`
	RejectedBy            string     `json:"rejected_by"`
	RejectedReason        string     `json:"rejected_reason"`
	BuildDispatchedAt     *time.Time `json:"build_dispatched_at"`
	DeployDispatchedAt    *time.Time `json:"deploy_dispatched_at"`
	ExecuteDispatchedAt   *time.Time `json:"execute_dispatched_at"`
	ExpiredAt             *time.Time `json:"expired_at"`
	CancelledAt           *time.Time `json:"cancelled_at"`
	CancelledBy           string     `json:"cancelled_by"`
	LastError             string     `json:"last_error"`
	Remark                string     `json:"remark"`
	CreatorUserID         string     `json:"creator_user_id"`
	CreatorName           string     `json:"creator_name"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ReleaseOrderScheduleDataResponse struct {
	Data ReleaseOrderScheduleResponse `json:"data"`
}

type ReleaseOrderScheduleListResponse struct {
	Data     []ReleaseOrderScheduleResponse `json:"data"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
	Total    int64                          `json:"total"`
}

type ReleaseOrderScheduleApprovalRecordResponse struct {
	ID             string    `json:"id"`
	ScheduleID     string    `json:"schedule_id"`
	Action         string    `json:"action"`
	OperatorUserID string    `json:"operator_user_id"`
	OperatorName   string    `json:"operator_name"`
	Comment        string    `json:"comment"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReleaseOrderScheduleApprovalRecordListResponse struct {
	Data []ReleaseOrderScheduleApprovalRecordResponse `json:"data"`
}

type ReleaseOrderApprovalRecordSummaryResponse struct {
	ID              string    `json:"id"`
	ReleaseOrderID  string    `json:"release_order_id"`
	OrderNo         string    `json:"order_no"`
	OrderStatus     string    `json:"order_status"`
	BusinessStatus  string    `json:"business_status"`
	ApplicationID   string    `json:"application_id"`
	ApplicationName string    `json:"application_name"`
	EnvCode         string    `json:"env_code"`
	OperationType   string    `json:"operation_type"`
	TriggeredBy     string    `json:"triggered_by"`
	Action          string    `json:"action"`
	OperatorUserID  string    `json:"operator_user_id"`
	OperatorName    string    `json:"operator_name"`
	Comment         string    `json:"comment"`
	CreatedAt       time.Time `json:"created_at"`
}

type ReleaseOrderApprovalRecordSummaryListResponse struct {
	Data     []ReleaseOrderApprovalRecordSummaryResponse `json:"data"`
	Page     int                                         `json:"page"`
	PageSize int                                         `json:"page_size"`
	Total    int64                                       `json:"total"`
}

type AppReleaseStateSummaryResponse struct {
	ApplicationID          string     `json:"application_id"`
	ApplicationName        string     `json:"application_name"`
	EnvCode                string     `json:"env_code"`
	CurrentStateID         string     `json:"current_state_id"`
	CurrentReleaseOrderID  string     `json:"current_release_order_id"`
	CurrentReleaseOrderNo  string     `json:"current_release_order_no"`
	CurrentImageTag        string     `json:"current_image_tag"`
	CurrentConfirmedAt     *time.Time `json:"current_confirmed_at"`
	CurrentConfirmedBy     string     `json:"current_confirmed_by"`
	PreviousStateID        string     `json:"previous_state_id"`
	PreviousReleaseOrderID string     `json:"previous_release_order_id"`
	PreviousReleaseOrderNo string     `json:"previous_release_order_no"`
	PreviousImageTag       string     `json:"previous_image_tag"`
	PreviousConfirmedAt    *time.Time `json:"previous_confirmed_at"`
}

type AppReleaseStateSummaryListResponse struct {
	Data []AppReleaseStateSummaryResponse `json:"data"`
}

type ApplicationRollbackStateResponse struct {
	StateID        string     `json:"state_id"`
	ReleaseOrderID string     `json:"release_order_id"`
	ReleaseOrderNo string     `json:"release_order_no"`
	TemplateID     string     `json:"template_id"`
	TemplateName   string     `json:"template_name"`
	CDProvider     string     `json:"cd_provider"`
	GitRef         string     `json:"git_ref"`
	HasCIExecution bool       `json:"has_ci_execution"`
	HasCDExecution bool       `json:"has_cd_execution"`
	ImageTag       string     `json:"image_tag"`
	ConfirmedAt    *time.Time `json:"confirmed_at"`
	ConfirmedBy    string     `json:"confirmed_by"`
}

type ApplicationRollbackCapabilityResponse struct {
	ApplicationID   string                           `json:"application_id"`
	ApplicationName string                           `json:"application_name"`
	EnvCode         string                           `json:"env_code"`
	SupportedAction string                           `json:"supported_action"`
	Reason          string                           `json:"reason"`
	CurrentState    ApplicationRollbackStateResponse `json:"current_state"`
	TargetState     ApplicationRollbackStateResponse `json:"target_state"`
}

type ApplicationRollbackCapabilityDataResponse struct {
	Data ApplicationRollbackCapabilityResponse `json:"data"`
}

type ApplicationRollbackPrecheckParamResponse struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

type ApplicationRollbackPrecheckResponse struct {
	ApplicationID    string                                     `json:"application_id"`
	ApplicationName  string                                     `json:"application_name"`
	EnvCode          string                                     `json:"env_code"`
	Action           string                                     `json:"action"`
	SupportedAction  string                                     `json:"supported_action"`
	Reason           string                                     `json:"reason"`
	Executable       bool                                       `json:"executable"`
	WaitingForLock   bool                                       `json:"waiting_for_lock"`
	AheadCount       int                                        `json:"ahead_count"`
	LockEnabled      bool                                       `json:"lock_enabled"`
	LockScope        string                                     `json:"lock_scope"`
	ConflictStrategy string                                     `json:"conflict_strategy"`
	LockKey          string                                     `json:"lock_key"`
	ConflictOrderNo  string                                     `json:"conflict_order_no"`
	ConflictMessage  string                                     `json:"conflict_message"`
	PreviewScope     string                                     `json:"preview_scope"`
	TemplateID       string                                     `json:"template_id"`
	TemplateName     string                                     `json:"template_name"`
	CurrentState     ApplicationRollbackStateResponse           `json:"current_state"`
	TargetState      ApplicationRollbackStateResponse           `json:"target_state"`
	Items            []ReleaseOrderPrecheckItemResponse         `json:"items"`
	Params           []ApplicationRollbackPrecheckParamResponse `json:"params"`
}

type ApplicationRollbackPrecheckDataResponse struct {
	Data ApplicationRollbackPrecheckResponse `json:"data"`
}

// Create godoc
// @Summary      Create release order
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        request  body      CreateReleaseOrderRequest  true  "Create release order request"
// @Success      201      {object}  ReleaseOrderDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /release-orders [post]
func (h *ReleaseOrderHandler) Create(c *gin.Context) {
	var req CreateReleaseOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", req.ApplicationID, req.EnvCode) {
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	order, err := h.manager.Create(c.Request.Context(), buildReleaseOrderInput(req, currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)

	c.JSON(http.StatusCreated, gin.H{"data": toReleaseOrderResponse(order)})
}

// BatchCreate godoc
// @Summary      Batch create independent release orders
// @Description  Creates multiple independent release orders without starting approval or execution
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        request  body      BatchCreateReleaseOrdersRequest  true  "Batch create release orders request"
// @Success      201      {object}  ReleaseOrderBatchCreateResponse
// @Success      200      {object}  ReleaseOrderBatchCreateResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Failure      403      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /release-orders/batch-create [post]
func (h *ReleaseOrderHandler) BatchCreate(c *gin.Context) {
	const maxBatchSize = 50

	var req BatchCreateReleaseOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(req.Orders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "orders is required"})
		return
	}
	if len(req.Orders) > maxBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("a batch can create at most %d release orders", maxBatchSize)})
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Complete permission checks before creating anything so an authorization
	// failure never leaves behind a partially-created batch.
	for _, item := range req.Orders {
		if !ensureReleaseApplicationPermission(c, h.authz, "release.create", item.ApplicationID, item.EnvCode) {
			return
		}
	}

	resp := ReleaseOrderBatchCreateResponse{}
	resp.Data.Orders = make([]ReleaseOrderResponse, 0, len(req.Orders))
	resp.Data.Failures = make([]BatchCreateReleaseOrderFailureResponse, 0)
	for index, item := range req.Orders {
		order, err := h.manager.Create(c.Request.Context(), buildReleaseOrderInput(item, currentUser))
		if err != nil {
			resp.Data.Failures = append(resp.Data.Failures, BatchCreateReleaseOrderFailureResponse{
				Index:       index,
				ReleaseName: strings.TrimSpace(item.ReleaseName),
				Error:       normalizeReleaseOrderErrorMessage(err),
			})
			continue
		}
		order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)
		resp.Data.Orders = append(resp.Data.Orders, toReleaseOrderResponse(order))
	}

	status := http.StatusCreated
	if len(resp.Data.Failures) > 0 {
		status = http.StatusOK
	}
	c.JSON(status, resp)
}

// Update godoc
// @Summary      Update editable release order
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "Release order id"
// @Param        request  body      CreateReleaseOrderRequest  true  "Update release order request"
// @Success      200      {object}  ReleaseOrderDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      403      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      409      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /release-orders/{id} [put]
func (h *ReleaseOrderHandler) Update(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "release order id is required"})
		return
	}

	existing, err := h.manager.GetByID(c.Request.Context(), orderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderEditableActor(c, currentUser, existing) {
		return
	}

	var req CreateReleaseOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	order, err := h.manager.Update(c.Request.Context(), orderID, buildReleaseOrderInput(req, currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)

	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(order)})
}

// Delete 删除资源。
// @Summary      删除资源
// @Description  删除资源，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id} [delete]
func (h *ReleaseOrderHandler) Delete(c *gin.Context) {
	if _, ok := ensureCurrentUserAdmin(c); !ok {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id": strings.TrimSpace(c.Param("id")),
		},
	})
}

// BatchDelete 批量处理Delete。
// @Summary      批量处理Delete
// @Description  批量处理Delete，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/batch-delete [post]
func (h *ReleaseOrderHandler) BatchDelete(c *gin.Context) {
	if _, ok := ensureCurrentUserAdmin(c); !ok {
		return
	}
	var req BatchDeleteReleaseOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.BatchDelete(c.Request.Context(), usecase.BatchDeleteReleaseOrdersInput{
		OrderIDs: req.OrderIDs,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// BatchExecute 批量处理Execute。
// @Summary      批量处理Execute
// @Description  批量处理Execute，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/batch-execute [post]
func (h *ReleaseOrderHandler) BatchExecute(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	var req BatchExecuteReleaseOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(req.OrderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_ids is required"})
		return
	}
	for _, orderID := range req.OrderIDs {
		item, err := h.manager.GetByID(c.Request.Context(), orderID)
		if err != nil {
			writeReleaseOrderHTTPError(c, err)
			return
		}
		if !ensureReleaseOrderVisible(c, h.authz, item) {
			return
		}
		if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", item.ApplicationID, item.EnvCode) {
			return
		}
	}
	output, err := h.manager.BatchExecute(c.Request.Context(), usecase.BatchExecuteReleaseOrdersInput{
		OrderIDs:           req.OrderIDs,
		BatchName:          strings.TrimSpace(req.BatchName),
		StagedDispatchMode: usecase.BatchExecuteStagedDispatchMode(strings.TrimSpace(req.StagedDispatchMode)),
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := ReleaseOrderBatchExecuteResponse{}
	resp.Data.BatchNo = output.BatchNo
	resp.Data.BatchName = output.BatchName
	resp.Data.DispatchErrors = append(resp.Data.DispatchErrors, output.DispatchErrors...)
	resp.Data.Orders = make([]ReleaseOrderResponse, 0, len(output.Orders))
	for _, item := range output.Orders {
		enriched := h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
		resp.Data.Orders = append(resp.Data.Orders, toReleaseOrderResponse(enriched))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateRollbackByApplication godoc
// @Summary      Create rollback release order by application
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Application ID"
// @Success      201  {object}  ReleaseOrderDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/release-orders/rollback [post]
func (h *ReleaseOrderHandler) CreateRollbackByApplication(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "按应用自动恢复已废弃，请基于指定发布单发起重放"})
}

// GetApplicationRollbackCapability 获取Application Rollback Capability详情。
// @Summary      获取Application Rollback Capability详情
// @Description  获取Application Rollback Capability详情，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/rollback-capability [post]
func (h *ReleaseOrderHandler) GetApplicationRollbackCapability(c *gin.Context) {
	applicationID := strings.TrimSpace(c.Param("id"))
	var req ApplicationRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	envCode := strings.TrimSpace(req.EnvCode)
	if applicationID == "" || envCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application_id and env_code are required"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", applicationID, envCode) {
		return
	}
	output, err := h.manager.GetApplicationRollbackCapability(c.Request.Context(), applicationID, envCode)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toApplicationRollbackCapabilityResponse(output)})
}

// GetApplicationRollbackPrecheck 获取Application Rollback Precheck详情。
// @Summary      获取Application Rollback Precheck详情
// @Description  获取Application Rollback Precheck详情，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/rollback-precheck [post]
func (h *ReleaseOrderHandler) GetApplicationRollbackPrecheck(c *gin.Context) {
	applicationID := strings.TrimSpace(c.Param("id"))
	var req ApplicationRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	envCode := strings.TrimSpace(req.EnvCode)
	if applicationID == "" || envCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application_id and env_code are required"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", applicationID, envCode) {
		return
	}
	output, err := h.manager.GetApplicationRollbackPrecheck(
		c.Request.Context(),
		applicationID,
		envCode,
		usecase.RollbackSupportedAction(strings.ToLower(strings.TrimSpace(req.Action))),
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toApplicationRollbackPrecheckResponse(output)})
}

// CreateApplicationRollbackOrder 创建Application Rollback Order。
// @Summary      创建Application Rollback Order
// @Description  创建Application Rollback Order，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /applications/{id}/rollback-orders [post]
func (h *ReleaseOrderHandler) CreateApplicationRollbackOrder(c *gin.Context) {
	applicationID := strings.TrimSpace(c.Param("id"))
	var req ApplicationRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	envCode := strings.TrimSpace(req.EnvCode)
	if applicationID == "" || envCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application_id and env_code are required"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", applicationID, envCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	order, err := h.manager.CreateApplicationRollbackOrder(
		c.Request.Context(),
		applicationID,
		envCode,
		usecase.RollbackSupportedAction(strings.ToLower(strings.TrimSpace(req.Action))),
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)
	c.JSON(http.StatusCreated, gin.H{"data": h.toReleaseOrderResponse(c.Request.Context(), order)})
}

// ListAppReleaseStateSummaries 查询App Release State Summaries列表。
// @Summary      查询App Release State Summaries列表
// @Description  查询App Release State Summaries列表，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /app-release-states/summaries [get]
func (h *ReleaseOrderHandler) ListAppReleaseStateSummaries(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	allowAll, visibleApplicationIDs, _, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	requestedIDs := splitTrimmedCSV(c.Query("application_ids"))
	if !allowAll {
		if len(requestedIDs) == 0 {
			requestedIDs = append([]string(nil), visibleApplicationIDs...)
		} else {
			allowed := make(map[string]struct{}, len(visibleApplicationIDs))
			for _, item := range visibleApplicationIDs {
				allowed[item] = struct{}{}
			}
			filtered := make([]string, 0, len(requestedIDs))
			for _, item := range requestedIDs {
				if _, exists := allowed[item]; exists {
					filtered = append(filtered, item)
				}
			}
			requestedIDs = filtered
		}
	}
	items, err := h.manager.ListCurrentAppReleaseStateSummaries(c.Request.Context(), requestedIDs)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]AppReleaseStateSummaryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toAppReleaseStateSummaryResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ConfirmLive 处理Confirm Live接口。
// @Summary      处理Confirm Live接口
// @Description  处理Confirm Live接口，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/confirm-live [post]
func (h *ReleaseOrderHandler) ConfirmLive(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	existing, err := h.manager.GetByID(c.Request.Context(), orderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	state, err := h.manager.ConfirmAppReleaseState(c.Request.Context(), orderID, resolveTriggeredBy(currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	existing = h.enrichReleaseOrderResponseMeta(c.Request.Context(), existing)
	resp := toReleaseOrderResponse(existing, &state)
	resp.LiveStateCanConfirm = false
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// CreateRollbackByOrder 创建Rollback By Order。
// @Summary      创建Rollback By Order
// @Description  创建Rollback By Order，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/rollback [post]
func (h *ReleaseOrderHandler) CreateRollbackByOrder(c *gin.Context) {
	sourceOrderID := strings.TrimSpace(c.Param("id"))
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	sourceOrder, err := h.manager.GetByID(c.Request.Context(), sourceOrderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", sourceOrder.ApplicationID, sourceOrder.EnvCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	order, err := h.manager.CreateStandardRollbackByOrder(
		c.Request.Context(),
		sourceOrderID,
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)
	c.JSON(http.StatusCreated, gin.H{"data": toReleaseOrderResponse(order)})
}

// CreateReplayByOrder 创建Replay By Order。
// @Summary      创建Replay By Order
// @Description  创建Replay By Order，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/replay [post]
func (h *ReleaseOrderHandler) CreateReplayByOrder(c *gin.Context) {
	sourceOrderID := strings.TrimSpace(c.Param("id"))
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	sourceOrder, err := h.manager.GetByID(c.Request.Context(), sourceOrderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", sourceOrder.ApplicationID, sourceOrder.EnvCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	order, err := h.manager.CreatePipelineReplayByOrder(
		c.Request.Context(),
		sourceOrderID,
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order = h.enrichReleaseOrderResponseMeta(c.Request.Context(), order)
	c.JSON(http.StatusCreated, gin.H{"data": toReleaseOrderResponse(order)})
}

// List godoc
// @Summary      List release orders
// @Tags         release-orders
// @Produce      json
// @Param        application_id    query     string  false  "Application ID"
// @Param        keyword           query     string  false  "Order keyword"
// @Param        concurrent_batch_no    query     string  false  "Concurrent batch no"
// @Param        concurrent_batch_name  query     string  false  "Concurrent batch name"
// @Param        triggered_by      query     string  false  "Triggered by"
// @Param        env_code          query     string  false  "Environment code"
// @Param        operation_type    query     string  false  "Operation type"
// @Param        status            query     string  false  "Release status"
// @Param        trigger_type      query     string  false  "Trigger type"
// @Param        created_at_from   query     string  false  "Created at from (RFC3339)"
// @Param        created_at_to     query     string  false  "Created at to (RFC3339)"
// @Param        page              query     int     false  "Page number"
// @Param        page_size         query     int     false  "Page size"
// @Success      200  {object}  ReleaseOrderListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders [get]
func (h *ReleaseOrderHandler) List(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
	applicationID := strings.TrimSpace(c.Query("application_id"))
	allowAll, visibleApplicationIDs, visibleEnvScopes, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}

	items, total, err := h.manager.List(c.Request.Context(), usecase.ListReleaseOrderInput{
		ApplicationID:               applicationID,
		ApplicationIDs:              resolveReleaseListApplicationIDs(applicationID, allowAll, visibleApplicationIDs),
		VisibleApplicationEnvScopes: visibleEnvScopes,
		VisibleToUserID:             visibleToUserID,
		ApprovalApproverUserID:      strings.TrimSpace(c.Query("approval_approver_user_id")),
		CreatorUserID:               resolveReleaseOrderCreatorFilter(currentUser),
		Keyword:                     strings.TrimSpace(c.Query("keyword")),
		ConcurrentBatchNo:           strings.TrimSpace(c.Query("concurrent_batch_no")),
		ConcurrentBatchName:         strings.TrimSpace(c.Query("concurrent_batch_name")),
		TriggeredBy:                 strings.TrimSpace(c.Query("triggered_by")),
		EnvCode:                     c.Query("env_code"),
		OperationType:               domain.OperationType(strings.TrimSpace(c.Query("operation_type"))),
		Status:                      domain.OrderStatus(strings.TrimSpace(c.Query("status"))),
		TriggerType:                 domain.TriggerType(strings.TrimSpace(c.Query("trigger_type"))),
		CreatedAtFrom:               parseOptionalTime(c.Query("created_at_from")),
		CreatedAtTo:                 parseOptionalTime(c.Query("created_at_to")),
		Page:                        page,
		PageSize:                    pageSize,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	for idx := range items {
		items[idx] = h.enrichReleaseOrderResponseMeta(c.Request.Context(), items[idx])
	}

	resp := make([]ReleaseOrderResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, h.toReleaseOrderResponse(c.Request.Context(), item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

func (h *ReleaseOrderHandler) ListSchedules(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
	applicationID := strings.TrimSpace(c.Query("application_id"))
	allowAll, visibleApplicationIDs, _, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}
	items, total, err := h.manager.ListSchedules(c.Request.Context(), usecase.ListReleaseOrderScheduleInput{
		ApplicationID:          applicationID,
		ApplicationIDs:         resolveReleaseListApplicationIDs(applicationID, allowAll, visibleApplicationIDs),
		VisibleToUserID:        visibleToUserID,
		ApprovalApproverUserID: strings.TrimSpace(c.Query("approval_approver_user_id")),
		CreatorUserID:          strings.TrimSpace(c.Query("creator_user_id")),
		Keyword:                strings.TrimSpace(c.Query("keyword")),
		EnvCode:                strings.TrimSpace(c.Query("env_code")),
		ScheduleMode:           domain.ScheduleMode(strings.TrimSpace(c.Query("schedule_mode"))),
		Status:                 domain.ScheduleStatus(strings.TrimSpace(c.Query("status"))),
		ScheduledAtFrom:        parseOptionalTime(c.Query("scheduled_at_from")),
		ScheduledAtTo:          parseOptionalTime(c.Query("scheduled_at_to")),
		Page:                   page,
		PageSize:               pageSize,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderScheduleResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderScheduleResponse(item))
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleListResponse{
		Data:     resp,
		Page:     resolvedPage(page),
		PageSize: resolvedPageSize(pageSize),
		Total:    total,
	})
}

func (h *ReleaseOrderHandler) ListSchedulableScheduleOrders(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
	applicationID := strings.TrimSpace(c.Query("application_id"))
	allowAll, visibleApplicationIDs, visibleEnvScopes, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}
	items, total, err := h.manager.ListSchedulableOrders(c.Request.Context(), usecase.ListSchedulableReleaseOrderInput{
		ListReleaseOrderInput: usecase.ListReleaseOrderInput{
			ApplicationID:               applicationID,
			ApplicationIDs:              resolveReleaseListApplicationIDs(applicationID, allowAll, visibleApplicationIDs),
			VisibleApplicationEnvScopes: visibleEnvScopes,
			VisibleToUserID:             visibleToUserID,
			CreatorUserID:               resolveReleaseOrderCreatorFilter(currentUser),
			Keyword:                     strings.TrimSpace(c.Query("keyword")),
			EnvCode:                     strings.TrimSpace(c.Query("env_code")),
			OperationType:               domain.OperationType(strings.TrimSpace(c.Query("operation_type"))),
			Page:                        page,
			PageSize:                    pageSize,
		},
		ScheduleMode: domain.ScheduleMode(strings.TrimSpace(c.Query("schedule_mode"))),
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	for idx := range items {
		items[idx] = h.enrichReleaseOrderResponseMeta(c.Request.Context(), items[idx])
	}
	resp := make([]ReleaseOrderResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, h.toReleaseOrderResponse(c.Request.Context(), item))
	}
	c.JSON(http.StatusOK, ReleaseOrderListResponse{
		Data:     resp,
		Page:     resolvedPage(page),
		PageSize: resolvedPageSize(pageSize),
		Total:    total,
	})
}

func (h *ReleaseOrderHandler) CreateSchedule(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", order.ApplicationID, order.EnvCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req ReleaseOrderScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.CreateSchedule(c.Request.Context(), order.ID, buildReleaseOrderScheduleInput(req, currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ReleaseOrderScheduleDataResponse{Data: toReleaseOrderScheduleResponse(item)})
}

func (h *ReleaseOrderHandler) GetActiveSchedule(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	item, err := h.manager.GetActiveScheduleByOrderID(c.Request.Context(), order.ID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleDataResponse{Data: toReleaseOrderScheduleResponse(item)})
}

func (h *ReleaseOrderHandler) UpdateSchedule(c *gin.Context) {
	item, err := h.manager.GetScheduleByID(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", item.ApplicationID, item.EnvCode) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderScheduleCreatorActor(c, currentUser, item, "edit this release order schedule") {
		return
	}
	var req ReleaseOrderScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	updated, err := h.manager.UpdateSchedule(c.Request.Context(), item.ID, buildReleaseOrderScheduleInput(req, currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleDataResponse{Data: toReleaseOrderScheduleResponse(updated)})
}

func (h *ReleaseOrderHandler) CancelSchedule(c *gin.Context) {
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	item, err := h.manager.GetScheduleByID(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", item.ApplicationID, item.EnvCode) {
		return
	}
	if !ensureReleaseOrderScheduleCreatorActor(c, currentUser, item, "cancel this release order schedule") {
		return
	}
	updated, err := h.manager.CancelSchedule(c.Request.Context(), item.ID, resolveTriggeredBy(currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleDataResponse{Data: toReleaseOrderScheduleResponse(updated)})
}

func (h *ReleaseOrderHandler) SubmitScheduleApproval(c *gin.Context) {
	h.handleScheduleApprovalAction(c, "submit")
}

func (h *ReleaseOrderHandler) ApproveSchedule(c *gin.Context) {
	h.handleScheduleApprovalAction(c, "approve")
}

func (h *ReleaseOrderHandler) RejectSchedule(c *gin.Context) {
	h.handleScheduleApprovalAction(c, "reject")
}

func (h *ReleaseOrderHandler) handleScheduleApprovalAction(c *gin.Context, action string) {
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	schedule, err := h.manager.GetScheduleByID(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), schedule.ReleaseOrderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	if action == "submit" && !ensureReleaseOrderScheduleCreatorActor(c, currentUser, schedule, "submit this release order schedule approval") {
		return
	}
	var req ReleaseOrderApprovalActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	var item domain.ReleaseOrderSchedule
	switch action {
	case "submit":
		item, err = h.manager.SubmitScheduleApproval(c.Request.Context(), c.Param("schedule_id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser), req.Comment)
	case "approve":
		item, err = h.manager.ApproveSchedule(c.Request.Context(), c.Param("schedule_id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser), req.Comment)
	case "reject":
		item, err = h.manager.RejectSchedule(c.Request.Context(), c.Param("schedule_id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser), req.Comment)
	default:
		err = usecase.ErrInvalidInput
	}
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleDataResponse{Data: toReleaseOrderScheduleResponse(item)})
}

func (h *ReleaseOrderHandler) ListScheduleApprovalRecords(c *gin.Context) {
	schedule, err := h.manager.GetScheduleByID(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), schedule.ReleaseOrderID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	items, err := h.manager.ListScheduleApprovalRecords(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderScheduleApprovalRecordResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderScheduleApprovalRecordResponse(item))
	}
	c.JSON(http.StatusOK, ReleaseOrderScheduleApprovalRecordListResponse{Data: resp})
}

// Stats 处理Stats接口。
// @Summary      处理Stats接口
// @Description  处理Stats接口，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/stats [get]
func (h *ReleaseOrderHandler) Stats(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	applicationID := strings.TrimSpace(c.Query("application_id"))
	allowAll, visibleApplicationIDs, visibleEnvScopes, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}
	stats, err := h.manager.ListStats(c.Request.Context(), usecase.ListReleaseOrderInput{
		ApplicationID:               applicationID,
		ApplicationIDs:              resolveReleaseListApplicationIDs(applicationID, allowAll, visibleApplicationIDs),
		VisibleApplicationEnvScopes: visibleEnvScopes,
		VisibleToUserID:             visibleToUserID,
		ApprovalApproverUserID:      strings.TrimSpace(c.Query("approval_approver_user_id")),
		CreatorUserID:               resolveReleaseOrderCreatorFilter(currentUser),
		Keyword:                     strings.TrimSpace(c.Query("keyword")),
		ConcurrentBatchNo:           strings.TrimSpace(c.Query("concurrent_batch_no")),
		ConcurrentBatchName:         strings.TrimSpace(c.Query("concurrent_batch_name")),
		TriggeredBy:                 strings.TrimSpace(c.Query("triggered_by")),
		EnvCode:                     c.Query("env_code"),
		OperationType:               domain.OperationType(strings.TrimSpace(c.Query("operation_type"))),
		Status:                      domain.OrderStatus(strings.TrimSpace(c.Query("status"))),
		TriggerType:                 domain.TriggerType(strings.TrimSpace(c.Query("trigger_type"))),
		CreatedAtFrom:               parseOptionalTime(c.Query("created_at_from")),
		CreatedAtTo:                 parseOptionalTime(c.Query("created_at_to")),
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderStatsResponse(stats))
}

// ListApprovalRecordSummaries 查询Approval Record Summaries列表。
// @Summary      查询Approval Record Summaries列表
// @Description  查询Approval Record Summaries列表，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-approval-records [get]
func (h *ReleaseOrderHandler) ListApprovalRecordSummaries(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
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
	applicationID := strings.TrimSpace(c.Query("application_id"))
	allowAll, visibleApplicationIDs, visibleEnvScopes, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}

	items, total, err := h.manager.ListApprovalRecordSummaries(c.Request.Context(), usecase.ListApprovalRecordSummaryInput{
		ApplicationID:               applicationID,
		ApplicationIDs:              resolveReleaseListApplicationIDs(applicationID, allowAll, visibleApplicationIDs),
		VisibleApplicationEnvScopes: visibleEnvScopes,
		VisibleToUserID:             visibleToUserID,
		OperatorUserID:              strings.TrimSpace(c.Query("operator_user_id")),
		Page:                        page,
		PageSize:                    pageSize,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderApprovalRecordSummaryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderApprovalRecordSummaryResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

// GetByID godoc
// @Summary      Get release order by ID
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Release order ID"
// @Success      200  {object}  ReleaseOrderDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id} [get]
func (h *ReleaseOrderHandler) GetByID(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, item) {
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": h.toReleaseOrderDetailResponse(c.Request.Context(), item)})
}

// GetPrecheck 获取Precheck详情。
// @Summary      获取Precheck详情
// @Description  获取Precheck详情，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/precheck [get]
func (h *ReleaseOrderHandler) GetPrecheck(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(c.Query("action")))
	var output usecase.ReleaseOrderPrecheckOutput
	switch action {
	case "build":
		output, err = h.manager.PrecheckBuild(c.Request.Context(), c.Param("id"))
	case "deploy":
		output, err = h.manager.PrecheckDeploy(c.Request.Context(), c.Param("id"))
	default:
		output, err = h.manager.PrecheckExecute(c.Request.Context(), c.Param("id"))
	}
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := ReleaseOrderPrecheckResponse{}
	resp.Data.OrderID = output.OrderID
	resp.Data.OrderNo = output.OrderNo
	resp.Data.Executable = output.Executable
	resp.Data.WaitingForLock = output.WaitingForLock
	resp.Data.AheadCount = output.AheadCount
	resp.Data.LockEnabled = output.LockEnabled
	resp.Data.LockScope = output.LockScope
	resp.Data.ConflictStrategy = output.ConflictStrategy
	resp.Data.LockKey = output.LockKey
	resp.Data.ConflictOrderNo = output.ConflictOrderNo
	resp.Data.ConflictMessage = output.ConflictMessage
	resp.Data.Items = make([]ReleaseOrderPrecheckItemResponse, 0, len(output.Items))
	for _, item := range output.Items {
		resp.Data.Items = append(resp.Data.Items, ReleaseOrderPrecheckItemResponse{
			Key:     item.Key,
			Name:    item.Name,
			Status:  string(item.Status),
			Message: item.Message,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// GetConcurrentBatchProgress 获取Concurrent Batch Progress详情。
// @Summary      获取Concurrent Batch Progress详情
// @Description  获取Concurrent Batch Progress详情，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/concurrent-batch-progress [get]
func (h *ReleaseOrderHandler) GetConcurrentBatchProgress(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	output, err := h.manager.GetConcurrentBatchProgress(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := ReleaseOrderConcurrentBatchProgressResponse{}
	resp.Data.OrderID = output.OrderID
	resp.Data.OrderNo = output.OrderNo
	resp.Data.BatchNo = output.BatchNo
	resp.Data.BatchName = output.BatchName
	resp.Data.IsConcurrent = output.IsConcurrent
	resp.Data.Total = output.Total
	resp.Data.Queued = output.Queued
	resp.Data.Executing = output.Executing
	resp.Data.Success = output.Success
	resp.Data.Failed = output.Failed
	resp.Data.Cancelled = output.Cancelled
	resp.Data.Items = make([]ReleaseOrderConcurrentBatchProgressItemResponse, 0, len(output.Items))
	for _, item := range output.Items {
		resp.Data.Items = append(resp.Data.Items, ReleaseOrderConcurrentBatchProgressItemResponse{
			OrderID:             item.OrderID,
			OrderNo:             item.OrderNo,
			ApplicationID:       item.ApplicationID,
			ApplicationName:     item.ApplicationName,
			EnvCode:             item.EnvCode,
			Status:              string(item.Status),
			OperationType:       string(item.OperationType),
			ConcurrentBatchSeq:  item.ConcurrentBatchSeq,
			QueueState:          string(item.QueueState),
			QueuePosition:       item.QueuePosition,
			HasRunningExecution: item.HasRunningExecution,
			StartedAt:           item.StartedAt,
			FinishedAt:          item.FinishedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// ListApprovalRecords 查询Approval Records列表。
// @Summary      查询Approval Records列表
// @Description  查询Approval Records列表，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/approval-records [get]
func (h *ReleaseOrderHandler) ListApprovalRecords(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	items, err := h.manager.ListApprovalRecords(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderApprovalRecordResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderApprovalRecordResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// SubmitApproval 处理Submit Approval接口。
// @Summary      处理Submit Approval接口
// @Description  处理Submit Approval接口，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/submit-approval [post]
func (h *ReleaseOrderHandler) SubmitApproval(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderCreatorActor(c, currentUser, existing, "submit approval") {
		return
	}
	var req ReleaseOrderApprovalActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.SubmitApproval(
		c.Request.Context(),
		c.Param("id"),
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
		req.Comment,
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Approve 审批通过发布单。
// @Summary      审批通过发布单
// @Description  审批通过发布单，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/approve [post]
func (h *ReleaseOrderHandler) Approve(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderApprovalActor(c, currentUser, existing) {
		return
	}
	var req ReleaseOrderApprovalActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Approve(
		c.Request.Context(),
		c.Param("id"),
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
		req.Comment,
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Reject 驳回发布单。
// @Summary      驳回发布单
// @Description  驳回发布单，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/reject [post]
func (h *ReleaseOrderHandler) Reject(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderApprovalActor(c, currentUser, existing) {
		return
	}
	var req ReleaseOrderApprovalActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Reject(
		c.Request.Context(),
		c.Param("id"),
		strings.TrimSpace(currentUser.ID),
		resolveTriggeredBy(currentUser),
		req.Comment,
	)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Cancel godoc
// @Summary      Cancel release order
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Release order ID"
// @Success      200  {object}  ReleaseOrderDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/cancel [post]
func (h *ReleaseOrderHandler) Cancel(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseOrderCancelActor(c, currentUser, existing) {
		return
	}
	item, err := h.manager.Cancel(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Execute godoc
// @Summary      Execute release order
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Release order ID"
// @Success      200  {object}  ReleaseOrderDataResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/execute [post]
func (h *ReleaseOrderHandler) Execute(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	item, err := h.manager.Execute(c.Request.Context(), c.Param("id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Build 处理Build接口。
// @Summary      处理Build接口
// @Description  处理Build接口，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/build [post]
func (h *ReleaseOrderHandler) Build(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	item, err := h.manager.Build(c.Request.Context(), c.Param("id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// Deploy 处理Deploy接口。
// @Summary      处理Deploy接口
// @Description  处理Deploy接口，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/deploy [post]
func (h *ReleaseOrderHandler) Deploy(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	item, err := h.manager.Deploy(c.Request.Context(), c.Param("id"), strings.TrimSpace(currentUser.ID), resolveTriggeredBy(currentUser))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	item = h.enrichReleaseOrderResponseMeta(c.Request.Context(), item)
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderResponse(item)})
}

// StreamLogs godoc
// @Summary      Stream release order logs
// @Tags         release-orders
// @Produce      text/event-stream
// @Param        id     path   string  true   "Release order ID"
// @Param        start  query  int     false  "Start offset for progressive logs"
// @Success      200
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/logs/stream [get]
func (h *ReleaseOrderHandler) StreamLogs(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	if h.logStreamer == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "log stream is not configured"})
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}

	startOffset, err := parseNonNegativeInt64Query(c, "start")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event usecase.ReleaseOrderLogEvent) error {
		eventName := strings.TrimSpace(event.Type)
		if eventName == "" {
			eventName = "message"
		}
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return marshalErr
		}
		if _, writeErr := fmt.Fprintf(c.Writer, "event: %s\n", eventName); writeErr != nil {
			return writeErr
		}
		if _, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); writeErr != nil {
			return writeErr
		}
		flusher.Flush()
		return nil
	}

	streamErr := h.logStreamer.Stream(c.Request.Context(), usecase.StreamReleaseOrderLogInput{
		ReleaseOrderID: c.Param("id"),
		PipelineScope:  domain.PipelineScope(strings.ToLower(strings.TrimSpace(c.Query("scope")))),
		StartOffset:    startOffset,
	}, writeEvent)
	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		_ = writeEvent(usecase.ReleaseOrderLogEvent{
			Type:      usecase.ReleaseOrderLogEventTypeError,
			Timestamp: time.Now().UTC(),
			Message:   normalizeReleaseOrderErrorMessage(streamErr),
		})
	}
}

// ListParams godoc
// @Summary      List release order params
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Release order ID"
// @Success      200  {object}  ReleaseOrderParamListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/params [get]
func (h *ReleaseOrderHandler) ListParams(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	if !ensurePermission(c, h.authz, "release.param_snapshot.view", "", "") {
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	items, err := h.manager.ListParams(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	resp := make([]ReleaseOrderParamResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderParamResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ListExecutions 查询Executions列表。
// @Summary      查询Executions列表
// @Description  查询Executions列表，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/executions [get]
func (h *ReleaseOrderHandler) ListExecutions(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	items, err := h.manager.ListExecutions(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderExecutionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderExecutionResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ListArtifactMetadata 查询发布单制品元信息列表。
// @Summary      查询发布单制品元信息列表
// @Description  查询发布单制品元信息列表，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  ReleaseOrderArtifactMetadataListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/artifact-metadata [get]
func (h *ReleaseOrderHandler) ListArtifactMetadata(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	items, err := h.manager.ListArtifactMetadata(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderArtifactMetadataResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderArtifactMetadataResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ListArtifactMetadataSummaries 查询制品中心发布单制品列表。
// @Summary      查询制品中心发布单制品列表
// @Description  按项目、应用、发布单和制品字段聚合查询制品元信息。
// @Tags         artifacts
// @Produce      json
// @Param        project_id        query  string  false  "Project ID"
// @Param        application_id    query  string  false  "Application ID"
// @Param        release_order_id  query  string  false  "Release order ID"
// @Param        keyword           query  string  false  "Keyword"
// @Param        artifact_name     query  string  false  "Artifact name"
// @Param        artifact_type     query  string  false  "Artifact type"
// @Param        pipeline_scope    query  string  false  "Pipeline scope"
// @Param        repository_id     query  string  false  "Repository ID"
// @Param        created_at_from   query  string  false  "Created at from (RFC3339)"
// @Param        created_at_to     query  string  false  "Created at to (RFC3339)"
// @Param        page              query  int     false  "Page number"
// @Param        page_size         query  int     false  "Page size"
// @Success      200  {object}  ReleaseOrderArtifactMetadataSummaryListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /artifacts/release-order-metadata [get]
func (h *ReleaseOrderHandler) ListArtifactMetadataSummaries(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	currentUser, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
	allowAll, visibleApplicationIDs, visibleEnvScopes, ok := resolveVisibleReleaseOrderApplicationIDs(c, h.authz)
	if !ok {
		return
	}
	visibleToUserID := ""
	if !allowAll {
		visibleToUserID = currentUser.ID
	}

	items, total, err := h.manager.ListArtifactMetadataSummaries(c.Request.Context(), usecase.ListReleaseOrderArtifactMetadataInput{
		ProjectID:                   strings.TrimSpace(c.Query("project_id")),
		ApplicationID:               strings.TrimSpace(c.Query("application_id")),
		ApplicationIDs:              resolveReleaseListApplicationIDs(strings.TrimSpace(c.Query("application_id")), allowAll, visibleApplicationIDs),
		VisibleApplicationEnvScopes: visibleEnvScopes,
		VisibleToUserID:             visibleToUserID,
		ReleaseOrderID:              strings.TrimSpace(c.Query("release_order_id")),
		Keyword:                     strings.TrimSpace(c.Query("keyword")),
		ArtifactName:                strings.TrimSpace(c.Query("artifact_name")),
		ArtifactType:                strings.TrimSpace(c.Query("artifact_type")),
		PipelineScope:               strings.TrimSpace(c.Query("pipeline_scope")),
		RepositoryID:                strings.TrimSpace(c.Query("repository_id")),
		CreatedAtFrom:               parseOptionalTime(c.Query("created_at_from")),
		CreatedAtTo:                 parseOptionalTime(c.Query("created_at_to")),
		Page:                        page,
		PageSize:                    pageSize,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	resp := make([]ReleaseOrderArtifactMetadataSummaryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderArtifactMetadataSummaryResponse(item))
	}
	c.JSON(http.StatusOK, ReleaseOrderArtifactMetadataSummaryListResponse{
		Data:     resp,
		Page:     resolvedPage(page),
		PageSize: resolvedPageSize(pageSize),
		Total:    total,
	})
}

// RecordArtifactMetadata 接收发布单制品元信息。
// @Summary      接收发布单制品元信息
// @Description  接收发布单制品元信息，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id       path      string                                     true  "资源 ID"
// @Param        request  body      RecordReleaseOrderArtifactMetadataRequest  true  "制品元信息"
// @Success      200      {object}  ReleaseOrderArtifactMetadataDataResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Failure      403      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /release-orders/{id}/artifact-metadata [post]
func (h *ReleaseOrderHandler) RecordArtifactMetadata(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}

	var req RecordReleaseOrderArtifactMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.RecordArtifactMetadata(c.Request.Context(), c.Param("id"), usecase.RecordReleaseOrderArtifactMetadataInput{
		ExecutionID:      req.ExecutionID,
		PipelineScope:    req.PipelineScope,
		ArtifactName:     req.ArtifactName,
		ArtifactType:     req.ArtifactType,
		ArtifactVersion:  req.ArtifactVersion,
		ArtifactURL:      req.ArtifactURL,
		RepositoryID:     req.RepositoryID,
		RepositoryName:   req.RepositoryName,
		Bucket:           req.Bucket,
		ObjectKey:        req.ObjectKey,
		Checksum:         req.Checksum,
		ChecksumType:     req.ChecksumType,
		SizeBytes:        req.SizeBytes,
		BuildNumber:      req.BuildNumber,
		AdditionalFields: req.AdditionalFields,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderArtifactMetadataResponse(output)})
}

// DeleteArtifactMetadata 删除手动添加的发布单制品元信息。
func (h *ReleaseOrderHandler) DeleteArtifactMetadata(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	if err := h.manager.DeleteArtifactMetadata(c.Request.Context(), c.Param("id"), c.Param("artifact_id")); err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSteps godoc
// @Summary      List release order steps
// @Tags         release-orders
// @Produce      json
// @Param        id   path      string  true  "Release order ID"
// @Success      200  {object}  ReleaseOrderStepListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/steps [get]
func (h *ReleaseOrderHandler) ListSteps(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	items, err := h.manager.ListSteps(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	resp := make([]ReleaseOrderStepResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReleaseOrderStepResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// StartStep godoc
// @Summary      Start release order step
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id         path      string                        true  "Release order ID"
// @Param        step_code  path      string                        true  "Step code"
// @Param        request    body      StartReleaseOrderStepRequest  false "Start step request"
// @Success      200        {object}  ReleaseOrderStepActionResponse
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /release-orders/{id}/steps/{step_code}/start [post]
func (h *ReleaseOrderHandler) StartStep(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	var req StartReleaseOrderStepRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	step, order, err := h.manager.StartStep(c.Request.Context(), c.Param("id"), c.Param("step_code"), req.Message)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"order": toReleaseOrderResponse(order),
			"step":  toReleaseOrderStepResponse(step),
		},
	})
}

// FinishStep godoc
// @Summary      Finish release order step
// @Tags         release-orders
// @Accept       json
// @Produce      json
// @Param        id         path      string                         true  "Release order ID"
// @Param        step_code  path      string                         true  "Step code"
// @Param        request    body      FinishReleaseOrderStepRequest  false "Finish step request"
// @Success      200        {object}  ReleaseOrderStepActionResponse
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /release-orders/{id}/steps/{step_code}/finish [post]
func (h *ReleaseOrderHandler) FinishStep(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.execute", existing.ApplicationID, existing.EnvCode) {
		return
	}
	var req FinishReleaseOrderStepRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	step, order, err := h.manager.FinishStep(c.Request.Context(), c.Param("id"), c.Param("step_code"), usecase.FinishReleaseOrderStepInput{
		Status:  domain.StepStatus(strings.TrimSpace(req.Status)),
		Message: req.Message,
	})
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"order": toReleaseOrderResponse(order),
			"step":  toReleaseOrderStepResponse(step),
		},
	})
}

// toReleaseOrderResponse 将领域对象转换为接口响应结构。
func toReleaseOrderResponse(item domain.ReleaseOrder, states ...*domain.AppReleaseState) ReleaseOrderResponse {
	resp := ReleaseOrderResponse{
		ID:                    item.ID,
		OrderNo:               item.OrderNo,
		ReleaseName:           item.ReleaseName,
		PreviousOrderNo:       item.PreviousOrderNo,
		OperationType:         string(item.OperationType),
		SourceOrderID:         item.SourceOrderID,
		SourceOrderNo:         item.SourceOrderNo,
		IsConcurrent:          item.IsConcurrent,
		ConcurrentBatchNo:     item.ConcurrentBatchNo,
		ConcurrentBatchName:   item.ConcurrentBatchName,
		ConcurrentBatchSeq:    item.ConcurrentBatchSeq,
		CDProvider:            item.CDProvider,
		HasCIExecution:        item.HasCIExecution,
		HasCDExecution:        item.HasCDExecution,
		ApplicationID:         item.ApplicationID,
		ApplicationName:       item.ApplicationName,
		TemplateID:            item.TemplateID,
		TemplateName:          item.TemplateName,
		BindingID:             item.BindingID,
		PipelineID:            item.PipelineID,
		EnvCode:               item.EnvCode,
		ProjectName:           "",
		GitRef:                item.GitRef,
		ImageTag:              item.ImageTag,
		TriggerType:           string(item.TriggerType),
		Status:                string(item.Status),
		BusinessStatus:        string(item.BusinessStatus),
		ApprovalRequired:      item.ApprovalRequired,
		ApprovalMode:          string(item.ApprovalMode),
		ApprovalApproverIDs:   append([]string(nil), item.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), item.ApprovalApproverNames...),
		ApprovedAt:            item.ApprovedAt,
		ApprovedBy:            item.ApprovedBy,
		RejectedAt:            item.RejectedAt,
		RejectedBy:            item.RejectedBy,
		RejectedReason:        item.RejectedReason,
		QueuePosition:         item.QueuePosition,
		QueuedReason:          item.QueuedReason,
		Remark:                item.Remark,
		CreatorUserID:         item.CreatorUserID,
		TriggeredBy:           item.TriggeredBy,
		StartedAt:             item.StartedAt,
		FinishedAt:            item.FinishedAt,
		CreatedAt:             item.CreatedAt,
		UpdatedAt:             item.UpdatedAt,
	}
	if len(states) > 0 && states[0] != nil {
		resp.LiveStateStatus = string(states[0].StateStatus)
		resp.LiveStateIsCurrent = states[0].IsCurrentLive
		resp.LiveStateConfirmedAt = states[0].ConfirmedAt
		resp.LiveStateConfirmedBy = states[0].ConfirmedBy
	}
	return resp
}

func buildReleaseOrderScheduleInput(req ReleaseOrderScheduleRequest, currentUser userdomain.User) usecase.CreateReleaseOrderScheduleInput {
	return usecase.CreateReleaseOrderScheduleInput{
		ScheduleMode:       domain.ScheduleMode(strings.TrimSpace(req.ScheduleMode)),
		BuildScheduledAt:   req.BuildScheduledAt,
		DeployScheduledAt:  req.DeployScheduledAt,
		ExecuteScheduledAt: req.ExecuteScheduledAt,
		Timezone:           strings.TrimSpace(req.Timezone),
		Remark:             strings.TrimSpace(req.Remark),
		CreatorUserID:      strings.TrimSpace(currentUser.ID),
		CreatorName:        resolveTriggeredBy(currentUser),
	}
}

func toReleaseOrderScheduleResponse(item domain.ReleaseOrderSchedule) ReleaseOrderScheduleResponse {
	return ReleaseOrderScheduleResponse{
		ID:                    item.ID,
		ScheduleNo:            item.ScheduleNo,
		ReleaseOrderID:        item.ReleaseOrderID,
		ReleaseOrderNo:        item.ReleaseOrderNo,
		ApplicationID:         item.ApplicationID,
		ApplicationName:       item.ApplicationName,
		EnvCode:               item.EnvCode,
		TemplateID:            item.TemplateID,
		TemplateName:          item.TemplateName,
		ScheduleMode:          string(item.ScheduleMode),
		BuildScheduledAt:      item.BuildScheduledAt,
		DeployScheduledAt:     item.DeployScheduledAt,
		ExecuteScheduledAt:    item.ExecuteScheduledAt,
		CDConflictAt:          item.CDConflictAt,
		Timezone:              item.Timezone,
		Status:                string(item.Status),
		ApprovalRequired:      item.ApprovalRequired,
		ApprovalMode:          string(item.ApprovalMode),
		ApprovalApproverIDs:   append([]string(nil), item.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), item.ApprovalApproverNames...),
		ApprovedAt:            item.ApprovedAt,
		ApprovedBy:            item.ApprovedBy,
		RejectedAt:            item.RejectedAt,
		RejectedBy:            item.RejectedBy,
		RejectedReason:        item.RejectedReason,
		BuildDispatchedAt:     item.BuildDispatchedAt,
		DeployDispatchedAt:    item.DeployDispatchedAt,
		ExecuteDispatchedAt:   item.ExecuteDispatchedAt,
		ExpiredAt:             item.ExpiredAt,
		CancelledAt:           item.CancelledAt,
		CancelledBy:           item.CancelledBy,
		LastError:             item.LastError,
		Remark:                item.Remark,
		CreatorUserID:         item.CreatorUserID,
		CreatorName:           item.CreatorName,
		CreatedAt:             item.CreatedAt,
		UpdatedAt:             item.UpdatedAt,
	}
}

func toReleaseOrderScheduleApprovalRecordResponse(item domain.ReleaseOrderScheduleApprovalRecord) ReleaseOrderScheduleApprovalRecordResponse {
	return ReleaseOrderScheduleApprovalRecordResponse{
		ID:             item.ID,
		ScheduleID:     item.ScheduleID,
		Action:         string(item.Action),
		OperatorUserID: item.OperatorUserID,
		OperatorName:   item.OperatorName,
		Comment:        item.Comment,
		CreatedAt:      item.CreatedAt,
	}
}

// toAppReleaseStateSummaryResponse 将领域对象转换为接口响应结构。
func toAppReleaseStateSummaryResponse(item domain.AppReleaseStateSummary) AppReleaseStateSummaryResponse {
	return AppReleaseStateSummaryResponse{
		ApplicationID:          item.ApplicationID,
		ApplicationName:        item.ApplicationName,
		EnvCode:                item.EnvCode,
		CurrentStateID:         item.CurrentStateID,
		CurrentReleaseOrderID:  item.CurrentReleaseOrderID,
		CurrentReleaseOrderNo:  item.CurrentReleaseOrderNo,
		CurrentImageTag:        item.CurrentImageTag,
		CurrentConfirmedAt:     item.CurrentConfirmedAt,
		CurrentConfirmedBy:     item.CurrentConfirmedBy,
		PreviousStateID:        item.PreviousStateID,
		PreviousReleaseOrderID: item.PreviousReleaseOrderID,
		PreviousReleaseOrderNo: item.PreviousReleaseOrderNo,
		PreviousImageTag:       item.PreviousImageTag,
		PreviousConfirmedAt:    item.PreviousConfirmedAt,
	}
}

// toApplicationRollbackCapabilityResponse 将领域对象转换为接口响应结构。
func toApplicationRollbackCapabilityResponse(item usecase.ApplicationRollbackCapabilityOutput) ApplicationRollbackCapabilityResponse {
	return ApplicationRollbackCapabilityResponse{
		ApplicationID:   item.ApplicationID,
		ApplicationName: item.ApplicationName,
		EnvCode:         item.EnvCode,
		SupportedAction: string(item.SupportedAction),
		Reason:          item.Reason,
		CurrentState: ApplicationRollbackStateResponse{
			StateID:        item.CurrentState.StateID,
			ReleaseOrderID: item.CurrentState.ReleaseOrderID,
			ReleaseOrderNo: item.CurrentState.ReleaseOrderNo,
			TemplateID:     item.CurrentState.TemplateID,
			TemplateName:   item.CurrentState.TemplateName,
			CDProvider:     item.CurrentState.CDProvider,
			GitRef:         item.CurrentState.GitRef,
			HasCIExecution: item.CurrentState.HasCIExecution,
			HasCDExecution: item.CurrentState.HasCDExecution,
			ImageTag:       item.CurrentState.ImageTag,
			ConfirmedAt:    item.CurrentState.ConfirmedAt,
			ConfirmedBy:    item.CurrentState.ConfirmedBy,
		},
		TargetState: ApplicationRollbackStateResponse{
			StateID:        item.TargetState.StateID,
			ReleaseOrderID: item.TargetState.ReleaseOrderID,
			ReleaseOrderNo: item.TargetState.ReleaseOrderNo,
			TemplateID:     item.TargetState.TemplateID,
			TemplateName:   item.TargetState.TemplateName,
			CDProvider:     item.TargetState.CDProvider,
			GitRef:         item.TargetState.GitRef,
			HasCIExecution: item.TargetState.HasCIExecution,
			HasCDExecution: item.TargetState.HasCDExecution,
			ImageTag:       item.TargetState.ImageTag,
			ConfirmedAt:    item.TargetState.ConfirmedAt,
			ConfirmedBy:    item.TargetState.ConfirmedBy,
		},
	}
}

// toApplicationRollbackPrecheckResponse 将领域对象转换为接口响应结构。
func toApplicationRollbackPrecheckResponse(item usecase.ApplicationRollbackPrecheckOutput) ApplicationRollbackPrecheckResponse {
	resp := ApplicationRollbackPrecheckResponse{
		ApplicationID:    item.ApplicationID,
		ApplicationName:  item.ApplicationName,
		EnvCode:          item.EnvCode,
		Action:           string(item.Action),
		SupportedAction:  string(item.SupportedAction),
		Reason:           item.Reason,
		Executable:       item.Executable,
		WaitingForLock:   item.WaitingForLock,
		AheadCount:       item.AheadCount,
		LockEnabled:      item.LockEnabled,
		LockScope:        item.LockScope,
		ConflictStrategy: item.ConflictStrategy,
		LockKey:          item.LockKey,
		ConflictOrderNo:  item.ConflictOrderNo,
		ConflictMessage:  item.ConflictMessage,
		PreviewScope:     item.PreviewScope,
		TemplateID:       item.TemplateID,
		TemplateName:     item.TemplateName,
		CurrentState: ApplicationRollbackStateResponse{
			StateID:        item.CurrentState.StateID,
			ReleaseOrderID: item.CurrentState.ReleaseOrderID,
			ReleaseOrderNo: item.CurrentState.ReleaseOrderNo,
			TemplateID:     item.CurrentState.TemplateID,
			TemplateName:   item.CurrentState.TemplateName,
			CDProvider:     item.CurrentState.CDProvider,
			GitRef:         item.CurrentState.GitRef,
			HasCIExecution: item.CurrentState.HasCIExecution,
			HasCDExecution: item.CurrentState.HasCDExecution,
			ImageTag:       item.CurrentState.ImageTag,
			ConfirmedAt:    item.CurrentState.ConfirmedAt,
			ConfirmedBy:    item.CurrentState.ConfirmedBy,
		},
		TargetState: ApplicationRollbackStateResponse{
			StateID:        item.TargetState.StateID,
			ReleaseOrderID: item.TargetState.ReleaseOrderID,
			ReleaseOrderNo: item.TargetState.ReleaseOrderNo,
			TemplateID:     item.TargetState.TemplateID,
			TemplateName:   item.TargetState.TemplateName,
			CDProvider:     item.TargetState.CDProvider,
			GitRef:         item.TargetState.GitRef,
			HasCIExecution: item.TargetState.HasCIExecution,
			HasCDExecution: item.TargetState.HasCDExecution,
			ImageTag:       item.TargetState.ImageTag,
			ConfirmedAt:    item.TargetState.ConfirmedAt,
			ConfirmedBy:    item.TargetState.ConfirmedBy,
		},
		Items:  make([]ReleaseOrderPrecheckItemResponse, 0, len(item.Items)),
		Params: make([]ApplicationRollbackPrecheckParamResponse, 0, len(item.Params)),
	}
	for _, precheckItem := range item.Items {
		resp.Items = append(resp.Items, ReleaseOrderPrecheckItemResponse{
			Key:     precheckItem.Key,
			Name:    precheckItem.Name,
			Status:  string(precheckItem.Status),
			Message: precheckItem.Message,
		})
	}
	for _, param := range item.Params {
		resp.Params = append(resp.Params, ApplicationRollbackPrecheckParamResponse{
			PipelineScope:     param.PipelineScope,
			ParamKey:          param.ParamKey,
			ExecutorParamName: param.ExecutorParamName,
			ParamValue:        param.ParamValue,
			ValueSource:       param.ValueSource,
		})
	}
	return resp
}

// toReleaseOrderResponse 将领域对象转换为接口响应结构。
func (h *ReleaseOrderHandler) toReleaseOrderResponse(ctx context.Context, item domain.ReleaseOrder) ReleaseOrderResponse {
	state := h.lookupAppReleaseStateByOrderID(ctx, item.ID)
	resp := toReleaseOrderResponse(item, state)
	resp.LiveStateCanConfirm = h.canConfirmLiveForOrder(ctx, item.ID, state)
	return resp
}

func (h *ReleaseOrderHandler) toReleaseOrderDetailResponse(ctx context.Context, item domain.ReleaseOrder) ReleaseOrderResponse {
	resp := h.toReleaseOrderResponse(ctx, item)
	resp.DeploySnapshots = h.listDeploySnapshotResponses(ctx, item.ID)
	return resp
}

func (h *ReleaseOrderHandler) listDeploySnapshotResponses(ctx context.Context, releaseOrderID string) []ReleaseOrderDeploySnapshotResponse {
	if h == nil || h.manager == nil || strings.TrimSpace(releaseOrderID) == "" {
		return nil
	}
	snapshots, err := h.manager.ListDeploySnapshotsByOrderID(ctx, releaseOrderID)
	if err != nil || len(snapshots) == 0 {
		return nil
	}
	result := make([]ReleaseOrderDeploySnapshotResponse, 0, len(snapshots))
	for _, item := range snapshots {
		result = append(result, toReleaseOrderDeploySnapshotResponse(item))
	}
	return result
}

func toReleaseOrderDeploySnapshotResponse(item domain.DeploySnapshot) ReleaseOrderDeploySnapshotResponse {
	imageVersion, rulesCount := summarizeDeploySnapshotPayload(item.SnapshotPayload)
	return ReleaseOrderDeploySnapshotResponse{
		ID:               item.ID,
		ReleaseOrderID:   item.ReleaseOrderID,
		Provider:         item.Provider,
		GitOpsType:       string(item.GitOpsType),
		ArgoCDInstanceID: item.ArgoCDInstanceID,
		GitOpsInstanceID: item.GitOpsInstanceID,
		ArgoCDAppName:    item.ArgoCDAppName,
		RepoURL:          item.RepoURL,
		Branch:           item.Branch,
		SourcePath:       item.SourcePath,
		EnvCode:          item.EnvCode,
		ImageVersion:     imageVersion,
		RulesCount:       rulesCount,
		CreatedAt:        item.CreatedAt,
	}
}

func summarizeDeploySnapshotPayload(payload string) (string, int) {
	var parsed struct {
		ImageVersion string `json:"image_version"`
		Rules        []struct {
			FilePath   string `json:"file_path"`
			TargetPath string `json:"target_path"`
			Value      string `json:"value"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &parsed); err != nil {
		return "", 0
	}
	return strings.TrimSpace(parsed.ImageVersion), len(parsed.Rules)
}

// canConfirmLiveForOrder 封装当前模块的业务处理逻辑。
func (h *ReleaseOrderHandler) canConfirmLiveForOrder(
	ctx context.Context,
	releaseOrderID string,
	state *domain.AppReleaseState,
) bool {
	if h == nil || h.manager == nil || state == nil {
		return false
	}
	if state.StateStatus != domain.AppReleaseStateStatusPendingConfirm {
		return false
	}
	ok, err := h.manager.CanConfirmAppReleaseState(ctx, releaseOrderID)
	if err != nil {
		return false
	}
	return ok
}

// lookupAppReleaseStateByOrderID 封装当前模块的业务处理逻辑。
func (h *ReleaseOrderHandler) lookupAppReleaseStateByOrderID(ctx context.Context, releaseOrderID string) *domain.AppReleaseState {
	if h == nil || h.manager == nil || strings.TrimSpace(releaseOrderID) == "" {
		return nil
	}
	state, err := h.manager.GetAppReleaseStateByOrderID(ctx, strings.TrimSpace(releaseOrderID))
	if err != nil {
		return nil
	}
	return &state
}

// splitTrimmedCSV 封装当前模块的业务处理逻辑。
func splitTrimmedCSV(value string) []string {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	raw := strings.Split(text, ",")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

// enrichReleaseOrderResponseMeta 封装当前模块的业务处理逻辑。
func (h *ReleaseOrderHandler) enrichReleaseOrderResponseMeta(ctx context.Context, item domain.ReleaseOrder) domain.ReleaseOrder {
	if h == nil || h.manager == nil || strings.TrimSpace(item.ID) == "" {
		return item
	}
	executions, err := h.manager.ListExecutions(ctx, item.ID)
	if err != nil {
		return item
	}
	hasRunningExecution := false
	for _, execution := range executions {
		if execution.Status == domain.ExecutionStatusRunning {
			hasRunningExecution = true
		}
		if execution.PipelineScope == domain.PipelineScopeCI {
			item.HasCIExecution = true
		}
		if execution.PipelineScope != domain.PipelineScopeCD {
			continue
		}
		item.HasCDExecution = true
		item.CDProvider = strings.TrimSpace(execution.Provider)
	}
	item.BusinessStatus = deriveReleaseBusinessStatus(item.Status, hasRunningExecution)
	if item.BusinessStatus == domain.ReleaseBusinessStatusDeploying ||
		item.BusinessStatus == domain.ReleaseBusinessStatusQueued ||
		item.BusinessStatus == domain.ReleaseBusinessStatusBuilding {
		if progress, progressErr := h.manager.GetConcurrentBatchProgress(ctx, item.ID); progressErr == nil {
			for _, current := range progress.Items {
				if strings.TrimSpace(current.OrderID) != strings.TrimSpace(item.ID) {
					continue
				}
				switch current.QueueState {
				case usecase.ReleaseOrderConcurrentBatchQueueStateQueued:
					if item.BusinessStatus != domain.ReleaseBusinessStatusBuilding {
						item.BusinessStatus = domain.ReleaseBusinessStatusQueued
					}
					item.QueuePosition = current.QueuePosition
					if item.QueuePosition > 0 {
						item.QueuedReason = fmt.Sprintf("并发批次排队中，当前位次 %d", item.QueuePosition)
					}
				case usecase.ReleaseOrderConcurrentBatchQueueStateExecuting:
					if item.BusinessStatus != domain.ReleaseBusinessStatusBuilding {
						item.BusinessStatus = domain.ReleaseBusinessStatusDeploying
					}
				case usecase.ReleaseOrderConcurrentBatchQueueStateSuccess:
					item.BusinessStatus = domain.ReleaseBusinessStatusDeploySuccess
				case usecase.ReleaseOrderConcurrentBatchQueueStateFailed:
					item.BusinessStatus = domain.ReleaseBusinessStatusDeployFailed
				case usecase.ReleaseOrderConcurrentBatchQueueStateCancelled:
					item.BusinessStatus = domain.ReleaseBusinessStatusCancelled
				}
				break
			}
		}
	}
	if item.BusinessStatus == domain.ReleaseBusinessStatusDeploying ||
		item.BusinessStatus == domain.ReleaseBusinessStatusQueued {
		if precheck, precheckErr := h.manager.PrecheckExecute(ctx, item.ID); precheckErr == nil && precheck.WaitingForLock {
			item.BusinessStatus = domain.ReleaseBusinessStatusQueued
			if item.QueuePosition <= 0 {
				item.QueuePosition = 0
			}
			if strings.TrimSpace(precheck.ConflictMessage) != "" {
				item.QueuedReason = strings.TrimSpace(precheck.ConflictMessage)
			}
		}
	}
	return item
}

// deriveReleaseBusinessStatus 封装当前模块的业务处理逻辑。
func deriveReleaseBusinessStatus(status domain.OrderStatus, hasRunningExecution bool) domain.ReleaseBusinessStatus {
	switch status {
	case domain.OrderStatusDraft:
		return domain.ReleaseBusinessStatusDraft
	case domain.OrderStatusPendingApproval:
		return domain.ReleaseBusinessStatusPendingApproval
	case domain.OrderStatusApproving:
		return domain.ReleaseBusinessStatusApproving
	case domain.OrderStatusApproved:
		return domain.ReleaseBusinessStatusApproved
	case domain.OrderStatusBuilding:
		return domain.ReleaseBusinessStatusBuilding
	case domain.OrderStatusBuiltWaitingDeploy:
		return domain.ReleaseBusinessStatusBuiltWaitingDeploy
	case domain.OrderStatusRejected:
		return domain.ReleaseBusinessStatusRejected
	case domain.OrderStatusQueued:
		return domain.ReleaseBusinessStatusQueued
	case domain.OrderStatusDeploying:
		return domain.ReleaseBusinessStatusDeploying
	case domain.OrderStatusDeploySuccess:
		return domain.ReleaseBusinessStatusDeploySuccess
	case domain.OrderStatusDeployFailed:
		return domain.ReleaseBusinessStatusDeployFailed
	case domain.OrderStatusCancelled:
		return domain.ReleaseBusinessStatusCancelled
	case domain.OrderStatusSuccess:
		return domain.ReleaseBusinessStatusDeploySuccess
	case domain.OrderStatusFailed:
		return domain.ReleaseBusinessStatusDeployFailed
	case domain.OrderStatusRunning:
		if hasRunningExecution {
			return domain.ReleaseBusinessStatusDeploying
		}
		return domain.ReleaseBusinessStatusQueued
	case domain.OrderStatusPending:
		return domain.ReleaseBusinessStatusPendingExecution
	default:
		return domain.ReleaseBusinessStatusPendingExecution
	}
}

// toReleaseOrderParamResponse 将领域对象转换为接口响应结构。
func toReleaseOrderParamResponse(item domain.ReleaseOrderParam) ReleaseOrderParamResponse {
	return ReleaseOrderParamResponse{
		ID:                item.ID,
		ReleaseOrderID:    item.ReleaseOrderID,
		PipelineScope:     string(item.PipelineScope),
		BindingID:         item.BindingID,
		ParamKey:          item.ParamKey,
		ExecutorParamName: item.ExecutorParamName,
		ParamValue:        item.ParamValue,
		ValueSource:       string(item.ValueSource),
		CreatedAt:         item.CreatedAt,
	}
}

// toReleaseOrderStepResponse 将领域对象转换为接口响应结构。
func toReleaseOrderStepResponse(item domain.ReleaseOrderStep) ReleaseOrderStepResponse {
	return ReleaseOrderStepResponse{
		ID:                 item.ID,
		ReleaseOrderID:     item.ReleaseOrderID,
		StepScope:          string(item.StepScope),
		ExecutionID:        item.ExecutionID,
		StepCode:           item.StepCode,
		StepName:           item.StepName,
		Status:             string(item.Status),
		Message:            item.Message,
		DetailLog:          item.DetailLog,
		RelatedTaskSummary: item.RelatedTaskSummary,
		RelatedTaskIDs:     append([]string(nil), item.RelatedTaskIDs...),
		RelatedTaskCount:   item.RelatedTaskCount,
		SortNo:             item.SortNo,
		StartedAt:          item.StartedAt,
		FinishedAt:         item.FinishedAt,
		CreatedAt:          item.CreatedAt,
	}
}

// toReleaseOrderExecutionResponse 将领域对象转换为接口响应结构。
func toReleaseOrderExecutionResponse(item domain.ReleaseOrderExecution) ReleaseOrderExecutionResponse {
	return ReleaseOrderExecutionResponse{
		ID:             item.ID,
		ReleaseOrderID: item.ReleaseOrderID,
		PipelineScope:  string(item.PipelineScope),
		BindingID:      item.BindingID,
		BindingName:    item.BindingName,
		Provider:       item.Provider,
		PipelineID:     item.PipelineID,
		Status:         string(item.Status),
		QueueURL:       item.QueueURL,
		BuildURL:       item.BuildURL,
		ExternalRunID:  item.ExternalRunID,
		StartedAt:      item.StartedAt,
		FinishedAt:     item.FinishedAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

// toReleaseOrderArtifactMetadataResponse 将业务输出转换为接口响应结构。
func toReleaseOrderArtifactMetadataResponse(item usecase.ReleaseOrderArtifactMetadataOutput) ReleaseOrderArtifactMetadataResponse {
	return ReleaseOrderArtifactMetadataResponse{
		ID:               item.ID,
		ReleaseOrderID:   item.ReleaseOrderID,
		ExecutionID:      item.ExecutionID,
		PipelineScope:    item.PipelineScope,
		ArtifactName:     item.ArtifactName,
		ArtifactType:     item.ArtifactType,
		ArtifactVersion:  item.ArtifactVersion,
		ArtifactURL:      item.ArtifactURL,
		RepositoryID:     item.RepositoryID,
		RepositoryName:   item.RepositoryName,
		Bucket:           item.Bucket,
		ObjectKey:        item.ObjectKey,
		Checksum:         item.Checksum,
		ChecksumType:     item.ChecksumType,
		SizeBytes:        item.SizeBytes,
		BuildNumber:      item.BuildNumber,
		AdditionalFields: item.AdditionalFields,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

func toReleaseOrderArtifactMetadataSummaryResponse(
	item usecase.ReleaseOrderArtifactMetadataSummaryOutput,
) ReleaseOrderArtifactMetadataSummaryResponse {
	return ReleaseOrderArtifactMetadataSummaryResponse{
		ReleaseOrderArtifactMetadataResponse: toReleaseOrderArtifactMetadataResponse(item.ReleaseOrderArtifactMetadataOutput),
		ReleaseOrderNo:                       item.ReleaseOrderNo,
		ReleaseName:                          item.ReleaseName,
		ReleaseDisplayName:                   item.ReleaseDisplayName,
		ApplicationID:                        item.ApplicationID,
		ApplicationName:                      item.ApplicationName,
		ProjectID:                            item.ProjectID,
		ProjectName:                          item.ProjectName,
		ProjectKey:                           item.ProjectKey,
		EnvCode:                              item.EnvCode,
		OrderStatus:                          item.OrderStatus,
	}
}

// toReleaseOrderApprovalRecordResponse 将领域对象转换为接口响应结构。
func toReleaseOrderApprovalRecordResponse(item domain.ReleaseOrderApprovalRecord) ReleaseOrderApprovalRecordResponse {
	return ReleaseOrderApprovalRecordResponse{
		ID:             item.ID,
		ReleaseOrderID: item.ReleaseOrderID,
		Action:         string(item.Action),
		OperatorUserID: item.OperatorUserID,
		OperatorName:   item.OperatorName,
		Comment:        item.Comment,
		CreatedAt:      item.CreatedAt,
	}
}

// toReleaseOrderApprovalRecordSummaryResponse 将领域对象转换为接口响应结构。
func toReleaseOrderApprovalRecordSummaryResponse(item domain.ReleaseOrderApprovalRecordSummary) ReleaseOrderApprovalRecordSummaryResponse {
	return ReleaseOrderApprovalRecordSummaryResponse{
		ID:              item.ID,
		ReleaseOrderID:  item.ReleaseOrderID,
		OrderNo:         item.OrderNo,
		OrderStatus:     string(item.OrderStatus),
		BusinessStatus:  string(deriveReleaseBusinessStatus(item.OrderStatus, false)),
		ApplicationID:   item.ApplicationID,
		ApplicationName: item.ApplicationName,
		EnvCode:         item.EnvCode,
		OperationType:   string(item.OperationType),
		TriggeredBy:     item.TriggeredBy,
		Action:          string(item.Action),
		OperatorUserID:  item.OperatorUserID,
		OperatorName:    item.OperatorName,
		Comment:         item.Comment,
		CreatedAt:       item.CreatedAt,
	}
}

// writeReleaseOrderHTTPError 写入处理结果或错误信息。
func writeReleaseOrderHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus),
		errors.Is(err, usecase.ErrInvalidSourceFrom):
		c.JSON(http.StatusBadRequest, gin.H{"error": normalizeReleaseOrderErrorMessage(err)})

	case errors.Is(err, appdomain.ErrNotFound),
		errors.Is(err, pipelinedomain.ErrBindingNotFound),
		errors.Is(err, domain.ErrOrderNotFound),
		errors.Is(err, domain.ErrExecutionNotFound),
		errors.Is(err, domain.ErrStepNotFound),
		errors.Is(err, domain.ErrPipelineStageNotFound),
		errors.Is(err, domain.ErrTemplateNotFound),
		errors.Is(err, domain.ErrScheduleNotFound),
		errors.Is(err, domain.ErrDeploySnapshotNotFound),
		errors.Is(err, domain.ErrArtifactMetadataNotFound),
		errors.Is(err, domain.ErrAppReleaseStateNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})

	case errors.Is(err, domain.ErrOrderDuplicated),
		errors.Is(err, domain.ErrTemplateDuplicated),
		errors.Is(err, domain.ErrScheduleDuplicated),
		errors.Is(err, domain.ErrAppReleaseStateNotConfirmable),
		errors.Is(err, usecase.ErrConcurrentReleaseBlocked):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})

	default:
		logx.Error("release_order_handler", "unhandled_internal_error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error: " + err.Error()})
	}
}

// normalizeReleaseOrderErrorMessage 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseOrderErrorMessage(err error) string {
	if err == nil {
		return "invalid input"
	}

	message := strings.Join(strings.Fields(strings.TrimSpace(err.Error())), " ")
	if message == "" {
		return "invalid input"
	}
	if errors.Is(err, usecase.ErrInvalidInput) {
		message = stripReleaseOrderErrorPrefix(message, usecase.ErrInvalidInput.Error(), "请求参数无效")
	}

	lower := strings.ToLower(message)
	const triggerPrefix = "trigger jenkins failed:"
	if index := strings.Index(lower, triggerPrefix); index >= 0 {
		reason := strings.TrimSpace(message[index+len(triggerPrefix):])
		if reason == "" {
			return "发布执行失败"
		}
		if messageIndex := strings.Index(strings.ToLower(reason), "message="); messageIndex >= 0 {
			reason = strings.TrimSpace(reason[messageIndex+len("message="):])
		}
		if len(reason) > 220 {
			reason = reason[:220] + "..."
		}
		return "发布执行失败：" + reason
	}

	if len(message) > 220 {
		return message[:220] + "..."
	}
	return message
}

// stripReleaseOrderErrorPrefix 移除对用户无意义的内部错误分类前缀。
func stripReleaseOrderErrorPrefix(message string, prefix string, fallback string) string {
	message = strings.TrimSpace(message)
	prefix = strings.TrimSpace(prefix)
	if message == "" || prefix == "" {
		return message
	}
	if strings.EqualFold(message, prefix) {
		return fallback
	}
	colonPrefix := prefix + ":"
	if strings.HasPrefix(strings.ToLower(message), strings.ToLower(colonPrefix)) {
		message = strings.TrimSpace(message[len(colonPrefix):])
	}
	if message == "" {
		return fallback
	}
	return message
}

// ensureCurrentUserAdmin 校验前置条件，不满足时写入对应错误响应。
func ensureCurrentUserAdmin(c *gin.Context) (userdomain.User, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return userdomain.User{}, false
	}
	if user.Role != userdomain.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only admin can perform this action"})
		return userdomain.User{}, false
	}
	return user, true
}

// ensureReleaseApplicationPermission 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseApplicationPermission(
	c *gin.Context,
	authz RequestAuthorizer,
	permissionCode string,
	applicationID string,
	envCode string,
) bool {
	scopeType := "application"
	scopeValue := strings.TrimSpace(applicationID)
	if strings.TrimSpace(envCode) != "" {
		scopeType = "application_env"
		scopeValue = buildApplicationEnvScopeValue(applicationID, envCode)
	}
	return ensurePermissionWithMessage(
		c,
		authz,
		permissionCode,
		scopeType,
		scopeValue,
		"无权限：当前应用的发布权限已变更，请刷新页面后重试",
	)
}

// ensureReleaseOrderApprovalActor 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderApprovalActor(
	c *gin.Context,
	user userdomain.User,
	order domain.ReleaseOrder,
) bool {
	if user.Role == userdomain.RoleAdmin {
		return true
	}
	currentUserID := strings.TrimSpace(user.ID)
	for _, item := range order.ApprovalApproverIDs {
		if strings.TrimSpace(item) == currentUserID {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: current user is not in approval approver list"})
	return false
}

// ensureReleaseOrderExecuteActor 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderExecuteActor(
	c *gin.Context,
	user userdomain.User,
	order domain.ReleaseOrder,
) bool {
	return ensureReleaseOrderCreatorActor(c, user, order, "execute this release order")
}

// ensureReleaseOrderCancelActor 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderCancelActor(
	c *gin.Context,
	user userdomain.User,
	order domain.ReleaseOrder,
) bool {
	if user.Role == userdomain.RoleAdmin {
		return true
	}
	return ensureReleaseOrderCreatorActor(c, user, order, "cancel this release order")
}

// ensureReleaseOrderEditableActor 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderEditableActor(
	c *gin.Context,
	user userdomain.User,
	order domain.ReleaseOrder,
) bool {
	if user.Role == userdomain.RoleAdmin {
		return true
	}
	return ensureReleaseOrderCreatorActor(c, user, order, "edit this release order")
}

// ensureReleaseOrderCreatorActor 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderCreatorActor(
	c *gin.Context,
	user userdomain.User,
	order domain.ReleaseOrder,
	action string,
) bool {
	currentUserID := strings.TrimSpace(user.ID)
	if currentUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if strings.TrimSpace(order.CreatorUserID) == currentUserID {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only release order creator can " + strings.TrimSpace(action)})
	return false
}

func ensureReleaseOrderScheduleCreatorActor(
	c *gin.Context,
	user userdomain.User,
	schedule domain.ReleaseOrderSchedule,
	action string,
) bool {
	if user.Role == userdomain.RoleAdmin {
		return true
	}
	currentUserID := strings.TrimSpace(user.ID)
	if currentUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if strings.TrimSpace(schedule.CreatorUserID) == currentUserID {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only schedule creator can " + strings.TrimSpace(action)})
	return false
}

// ensureAnyReleaseOrderDisplayPermission 校验前置条件，不满足时写入对应错误响应。
func ensureAnyReleaseOrderDisplayPermission(c *gin.Context, authz RequestAuthorizer) bool {
	if _, ok := getCurrentUser(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	return true
}

// ensureReleaseOrderVisible 校验前置条件，不满足时写入对应错误响应。
func ensureReleaseOrderVisible(
	c *gin.Context,
	authz RequestAuthorizer,
	order domain.ReleaseOrder,
) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	allowed, err := canCurrentUserViewReleaseOrder(c.Request.Context(), authz, user, order)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return false
	}
	return true
}

// buildReleaseOrderInput 组装业务执行所需的输入数据。
func buildReleaseOrderInput(req CreateReleaseOrderRequest, currentUser userdomain.User) usecase.CreateReleaseOrderInput {
	params := make([]usecase.CreateReleaseOrderParamInput, 0, len(req.Params))
	for _, item := range req.Params {
		params = append(params, usecase.CreateReleaseOrderParamInput{
			PipelineScope:     domain.PipelineScope(strings.ToLower(strings.TrimSpace(item.PipelineScope))),
			ParamKey:          strings.ToLower(strings.TrimSpace(item.ParamKey)),
			ExecutorParamName: item.ExecutorParamName,
			ParamValue:        item.ParamValue,
			ValueSource:       domain.ValueSource(strings.TrimSpace(item.ValueSource)),
		})
	}

	steps := make([]usecase.CreateReleaseOrderStepInput, 0, len(req.Steps))
	for _, item := range req.Steps {
		steps = append(steps, usecase.CreateReleaseOrderStepInput{
			StepCode: item.StepCode,
			StepName: item.StepName,
			SortNo:   item.SortNo,
		})
	}

	return usecase.CreateReleaseOrderInput{
		ApplicationID: req.ApplicationID,
		TemplateID:    req.TemplateID,
		ReleaseName:   req.ReleaseName,
		EnvCode:       req.EnvCode,
		GitRef:        req.GitRef,
		ImageTag:      req.ImageTag,
		TriggerType:   domain.TriggerType(strings.TrimSpace(req.TriggerType)),
		Remark:        req.Remark,
		CreatorUserID: strings.TrimSpace(currentUser.ID),
		TriggeredBy:   firstNonEmpty(resolveTriggeredBy(currentUser), req.TriggeredBy),
		Params:        params,
		Steps:         steps,
	}
}

// resolveVisibleReleaseOrderApplicationIDs 解析上下文数据，得到后续流程需要的结果。
func resolveVisibleReleaseOrderApplicationIDs(
	c *gin.Context,
	authz RequestAuthorizer,
) (allowAll bool, applicationIDs []string, envScopes []domain.ApplicationEnvScope, ok bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false, nil, nil, false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false, nil, nil, false
	}
	if user.Role == userdomain.RoleAdmin {
		return true, nil, nil, true
	}
	manageAllowed, err := authz.HasPermission(c.Request.Context(), user, "application.manage", "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, nil, false
	}
	if manageAllowed {
		return true, nil, nil, true
	}

	items, err := authz.ListEffectivePermissions(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false, nil, nil, false
	}
	accepted := map[string]struct{}{
		"release.view":    {},
		"release.create":  {},
		"release.execute": {},
		"release.cancel":  {},
	}
	applicationIDs, envScopes = collectApplicationScopesFromPermissions(items, accepted)
	applicationIDs = mergeReleaseVisibleApplicationIDs(applicationIDs, envScopes)
	return false, applicationIDs, envScopes, true
}

// resolveReleaseListApplicationIDs 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseListApplicationIDs(applicationID string, allowAll bool, visibleApplicationIDs []string) []string {
	if allowAll {
		return nil
	}
	return visibleApplicationIDs
}

// resolveReleaseOrderCreatorFilter 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseOrderCreatorFilter(user userdomain.User) string {
	_ = user
	return ""
}

// canCurrentUserViewReleaseOrder 封装当前模块的业务处理逻辑。
func canCurrentUserViewReleaseOrder(
	ctx context.Context,
	authz RequestAuthorizer,
	user userdomain.User,
	order domain.ReleaseOrder,
) (bool, error) {
	if user.Role == userdomain.RoleAdmin {
		return true, nil
	}
	currentUserID := strings.TrimSpace(user.ID)
	if currentUserID != "" {
		if strings.TrimSpace(order.CreatorUserID) == currentUserID {
			return true, nil
		}
		for _, item := range order.ApprovalApproverIDs {
			if strings.TrimSpace(item) == currentUserID {
				return true, nil
			}
		}
	}
	return canViewReleaseOrderApplication(ctx, authz, user, order.ApplicationID, order.EnvCode)
}

// canViewReleaseOrderApplication 封装当前模块的业务处理逻辑。
func canViewReleaseOrderApplication(
	ctx context.Context,
	authz RequestAuthorizer,
	user userdomain.User,
	applicationID string,
	envCode string,
) (bool, error) {
	if user.Role == userdomain.RoleAdmin {
		return true, nil
	}
	appID := strings.TrimSpace(applicationID)
	if appID == "" {
		return false, nil
	}
	manageAllowed, err := authz.HasPermission(ctx, user, "application.manage", "", "")
	if err != nil {
		return false, err
	}
	if manageAllowed {
		return true, nil
	}

	accepted := map[string]struct{}{
		"release.view":    {},
		"release.create":  {},
		"release.execute": {},
		"release.cancel":  {},
	}
	items, err := authz.ListEffectivePermissions(ctx, user)
	if err != nil {
		return false, err
	}
	applicationIDs, envScopes := collectApplicationScopesFromPermissions(items, accepted)
	applicationIDs = mergeReleaseVisibleApplicationIDs(applicationIDs, envScopes)
	if containsString(applicationIDs, appID) {
		return true, nil
	}

	for _, code := range []string{"release.view", "release.create", "release.execute", "release.cancel"} {
		allowed, err := authz.HasPermission(ctx, user, code, "application", appID)
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
		if env := strings.TrimSpace(envCode); env != "" {
			allowed, err = authz.HasPermission(ctx, user, code, "application_env", buildApplicationEnvScopeValue(appID, env))
			if err != nil {
				return false, err
			}
			if allowed {
				return true, nil
			}
		}
	}
	return false, nil
}

// mergeReleaseVisibleApplicationIDs 封装当前模块的业务处理逻辑。
func mergeReleaseVisibleApplicationIDs(
	applicationIDs []string,
	envScopes []domain.ApplicationEnvScope,
) []string {
	seen := make(map[string]struct{}, len(applicationIDs)+len(envScopes))
	result := make([]string, 0, len(applicationIDs)+len(envScopes))
	for _, item := range applicationIDs {
		appID := strings.TrimSpace(item)
		if appID == "" {
			continue
		}
		if _, exists := seen[appID]; exists {
			continue
		}
		seen[appID] = struct{}{}
		result = append(result, appID)
	}
	for _, item := range envScopes {
		appID := strings.TrimSpace(item.ApplicationID)
		if appID == "" {
			continue
		}
		if _, exists := seen[appID]; exists {
			continue
		}
		seen[appID] = struct{}{}
		result = append(result, appID)
	}
	sort.Strings(result)
	return result
}

// writeEmptyReleaseOrderList 写入处理结果或错误信息。
func writeEmptyReleaseOrderList(c *gin.Context, page int, pageSize int) {
	c.JSON(http.StatusOK, gin.H{
		"data":      []ReleaseOrderResponse{},
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     0,
	})
}

// containsString 封装当前模块的业务处理逻辑。
func containsString(items []string, target string) bool {
	value := strings.TrimSpace(target)
	if value == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
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

// resolveTriggeredBy 解析上下文数据，得到后续流程需要的结果。
func resolveTriggeredBy(user userdomain.User) string {
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName != "" {
		return displayName
	}
	username := strings.TrimSpace(user.Username)
	if username != "" {
		return username
	}
	return strings.TrimSpace(user.ID)
}

// parseNonNegativeInt64Query 解析输入内容并返回结构化结果。
func parseNonNegativeInt64Query(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	if value < 0 {
		return 0, errors.New(name + " must be greater than or equal to 0")
	}
	return value, nil
}
