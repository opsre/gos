export type ReleaseTriggerType = "manual" | "webhook" | "schedule";
export type ReleaseOrderDispatchAction = "execute" | "build" | "deploy";
export type ReleaseOrderStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "cancelled"
  | "draft"
  | "pending_approval"
  | "approving"
  | "approved"
  | "building"
  | "built_waiting_deploy"
  | "rejected"
  | "queued"
  | "deploying"
  | "deploy_success"
  | "deploy_failed";
export type ReleaseOrderBusinessStatus =
  | "draft"
  | "pending_execution"
  | "pending_approval"
  | "approving"
  | "approved"
  | "building"
  | "built_waiting_deploy"
  | "rejected"
  | "queued"
  | "deploying"
  | "deploy_success"
  | "deploy_failed"
  | "cancelled";
export type ReleaseOperationType = "deploy" | "rollback" | "replay";
export type ReleaseStepStatus = "pending" | "running" | "success" | "failed";
export type ReleasePipelineStageStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "cancelled"
  | "skipped";
export type ReleaseOrderValueProgressStatus =
  | "pending"
  | "running"
  | "resolved"
  | "failed"
  | "skipped";
export type ReleaseValueSource =
  | "application"
  | "environment"
  | "release_input"
  | "fixed"
  | "ci_param"
  | "builtin";
export type ReleaseTemplateStatus = "active" | "inactive";
export type ReleaseTemplateComplianceStatus = "compliant" | "violated" | "unknown" | "";
export type ReleasePipelineScope = "ci" | "cd";
export type ReleaseTemplateApprovalMode = "any" | "all";
export type ReleaseExecutionStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "cancelled"
  | "skipped";
export type ReleaseOrderScheduleMode = "build" | "deploy" | "build_deploy" | "execute";
export type ReleaseOrderScheduleStatus =
  | "pending_approval"
  | "approving"
  | "scheduled"
  | "dispatching"
  | "dispatched"
  | "expired"
  | "blocked"
  | "failed"
  | "skipped"
  | "cancelled"
  | "rejected";
export type ReleaseOrderScheduleApprovalAction = "submit" | "approve" | "reject";

export interface ReleaseOrder {
  id: string;
  order_no: string;
  release_name: string;
  previous_order_no: string;
  operation_type: ReleaseOperationType;
  source_order_id: string;
  source_order_no: string;
  is_concurrent: boolean;
  concurrent_batch_no: string;
  concurrent_batch_name: string;
  concurrent_batch_seq: number;
  cd_provider: string;
  has_ci_execution: boolean;
  has_cd_execution: boolean;
  application_id: string;
  application_name: string;
  template_id: string;
  template_name: string;
  binding_id: string;
  pipeline_id: string;
  env_code: string;
  project_name: string;
  git_ref: string;
  image_tag: string;
  trigger_type: ReleaseTriggerType;
  status: ReleaseOrderStatus;
  business_status: ReleaseOrderBusinessStatus;
  approval_required: boolean;
  approval_mode: ReleaseTemplateApprovalMode | "";
  approval_approver_ids: string[];
  approval_approver_names: string[];
  approved_at: string | null;
  approved_by: string;
  rejected_at: string | null;
  rejected_by: string;
  rejected_reason: string;
  queue_position: number;
  queued_reason: string;
  remark: string;
  creator_user_id?: string;
  triggered_by: string;
  live_state_status?: "pending_confirm" | "active" | "superseded" | "";
  live_state_is_current?: boolean;
  live_state_can_confirm?: boolean;
  live_state_confirmed_at?: string | null;
  live_state_confirmed_by?: string;
  deploy_snapshots?: ReleaseOrderDeploySnapshot[];
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ReleaseOrderDeploySnapshot {
  id: string;
  release_order_id: string;
  provider: string;
  gitops_type: string;
  argocd_instance_id: string;
  gitops_instance_id: string;
  argocd_app_name: string;
  repo_url: string;
  branch: string;
  source_path: string;
  env_code: string;
  image_version: string;
  rules_count: number;
  created_at: string;
}

export interface AppReleaseStateSummary {
  application_id: string;
  application_name: string;
  env_code: string;
  current_state_id: string;
  current_release_order_id: string;
  current_release_order_no: string;
  current_image_tag: string;
  current_confirmed_at: string | null;
  current_confirmed_by: string;
  previous_state_id: string;
  previous_release_order_id: string;
  previous_release_order_no: string;
  previous_image_tag: string;
  previous_confirmed_at: string | null;
}

export interface AppReleaseStateSummaryListResponse {
  data: AppReleaseStateSummary[];
}

export type RollbackSupportedAction = "rollback" | "replay" | "unsupported";

export interface ApplicationRollbackState {
  state_id: string;
  release_order_id: string;
  release_order_no: string;
  template_id: string;
  template_name: string;
  cd_provider: string;
  git_ref: string;
  has_ci_execution: boolean;
  has_cd_execution: boolean;
  image_tag: string;
  confirmed_at: string | null;
  confirmed_by: string;
}

export interface ApplicationRollbackCapability {
  application_id: string;
  application_name: string;
  env_code: string;
  supported_action: RollbackSupportedAction;
  reason: string;
  current_state: ApplicationRollbackState;
  target_state: ApplicationRollbackState;
}

export interface ApplicationRollbackCapabilityResponse {
  data: ApplicationRollbackCapability;
}

export interface ApplicationRollbackPrecheckParam {
  pipeline_scope: ReleasePipelineScope;
  param_key: string;
  executor_param_name: string;
  param_value: string;
  value_source: ReleaseValueSource | string;
}

export interface ApplicationRollbackPrecheck {
  application_id: string;
  application_name: string;
  env_code: string;
  action: RollbackSupportedAction;
  supported_action: RollbackSupportedAction;
  reason: string;
  executable: boolean;
  waiting_for_lock: boolean;
  ahead_count: number;
  lock_enabled: boolean;
  lock_scope: string;
  conflict_strategy: string;
  lock_key: string;
  conflict_order_no: string;
  conflict_message: string;
  preview_scope: string;
  template_id: string;
  template_name: string;
  current_state: ApplicationRollbackState;
  target_state: ApplicationRollbackState;
  items: ReleaseOrderPrecheckItem[];
  params: ApplicationRollbackPrecheckParam[];
}

export interface ApplicationRollbackPrecheckResponse {
  data: ApplicationRollbackPrecheck;
}

export interface ReleaseOrderParam {
  id: string;
  release_order_id: string;
  pipeline_scope: ReleasePipelineScope;
  binding_id: string;
  param_key: string;
  executor_param_name: string;
  param_value: string;
  value_source: ReleaseValueSource;
  created_at: string;
}

export interface ReleaseOrderValueProgress {
  pipeline_scope: ReleasePipelineScope;
  param_key: string;
  param_name: string;
  executor_param_name: string;
  required: boolean;
  pipeline_param: boolean;
  value_kind: "pipeline_param" | "builtin_field" | "execution_output";
  status: ReleaseOrderValueProgressStatus;
  value: string;
  value_source: string;
  message: string;
  updated_at: string | null;
  sort_no: number;
}

export interface ReleaseOrderStep {
  id: string;
  release_order_id: string;
  step_scope: "global" | ReleasePipelineScope;
  execution_id: string;
  step_code: string;
  step_name: string;
  status: ReleaseStepStatus;
  message: string;
  detail_log: string;
  related_task_summary: string;
  related_task_ids: string[];
  related_task_count: number;
  sort_no: number;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
}

export interface ReleaseOrderExecution {
  id: string;
  release_order_id: string;
  pipeline_scope: ReleasePipelineScope;
  binding_id: string;
  binding_name: string;
  provider: string;
  pipeline_id: string;
  status: ReleaseExecutionStatus;
  queue_url: string;
  build_url: string;
  external_run_id: string;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ReleaseOrderPipelineStage {
  id: string;
  release_order_id: string;
  pipeline_scope: string;
  executor_type: string;
  stage_name: string;
  status: ReleasePipelineStageStatus;
  raw_status: string;
  sort_no: number;
  duration_millis: number;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ReleaseOrderArtifactMetadata {
  id: string;
  release_order_id: string;
  execution_id: string;
  pipeline_scope: string;
  artifact_name: string;
  artifact_type: string;
  artifact_version: string;
  artifact_url: string;
  repository_id: string;
  repository_name: string;
  bucket: string;
  object_key: string;
  checksum: string;
  checksum_type: string;
  size_bytes: number;
  build_number: string;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ReleaseOrderListParams {
  application_id?: string;
  approval_approver_user_id?: string;
  keyword?: string;
  concurrent_batch_no?: string;
  concurrent_batch_name?: string;
  triggered_by?: string;
  env_code?: string;
  operation_type?: ReleaseOperationType;
  status?: ReleaseOrderStatus;
  trigger_type?: ReleaseTriggerType;
  created_at_from?: string;
  created_at_to?: string;
  page?: number;
  page_size?: number;
}

export interface ReleaseOrderListResponse {
  data: ReleaseOrder[];
  page: number;
  page_size: number;
  total: number;
}

export interface ReleaseOrderStatsResponse {
  total: number;
  pending: number;
  running: number;
  success: number;
  failed: number;
  cancelled: number;
}

export interface ReleaseOrderDataResponse {
  data: ReleaseOrder;
}

export interface BatchCreateReleaseOrdersPayload {
  orders: CreateReleaseOrderPayload[];
}

export interface BatchCreateReleaseOrderFailure {
  index: number;
  release_name: string;
  error: string;
}

export interface ReleaseOrderBatchCreateResult {
  orders: ReleaseOrder[];
  failures: BatchCreateReleaseOrderFailure[];
}

export interface ReleaseOrderBatchCreateResponse {
  data: ReleaseOrderBatchCreateResult;
}

export interface ReleaseOrderSchedule {
  id: string;
  schedule_no: string;
  release_order_id: string;
  release_order_no: string;
  application_id: string;
  application_name: string;
  env_code: string;
  template_id: string;
  template_name: string;
  schedule_mode: ReleaseOrderScheduleMode;
  build_scheduled_at: string | null;
  deploy_scheduled_at: string | null;
  execute_scheduled_at: string | null;
  cd_conflict_at: string | null;
  timezone: string;
  status: ReleaseOrderScheduleStatus;
  approval_required: boolean;
  approval_mode: ReleaseTemplateApprovalMode | "";
  approval_approver_ids: string[];
  approval_approver_names: string[];
  approved_at: string | null;
  approved_by: string;
  rejected_at: string | null;
  rejected_by: string;
  rejected_reason: string;
  build_dispatched_at: string | null;
  deploy_dispatched_at: string | null;
  execute_dispatched_at: string | null;
  expired_at: string | null;
  cancelled_at: string | null;
  cancelled_by: string;
  last_error: string;
  remark: string;
  creator_user_id: string;
  creator_name: string;
  created_at: string;
  updated_at: string;
}

export interface ReleaseOrderScheduleListParams {
  application_id?: string;
  release_order_id?: string;
  keyword?: string;
  env_code?: string;
  schedule_mode?: ReleaseOrderScheduleMode;
  status?: ReleaseOrderScheduleStatus;
  approval_approver_user_id?: string;
  scheduled_at_from?: string;
  scheduled_at_to?: string;
  page?: number;
  page_size?: number;
}

export interface ReleaseOrderScheduleListResponse {
  data: ReleaseOrderSchedule[];
  page: number;
  page_size: number;
  total: number;
}

export interface ReleaseOrderScheduleDataResponse {
  data: ReleaseOrderSchedule;
}

export interface ReleaseOrderSchedulePayload {
  schedule_mode: ReleaseOrderScheduleMode;
  build_scheduled_at?: string;
  deploy_scheduled_at?: string;
  execute_scheduled_at?: string;
  timezone: string;
  remark?: string;
}

export interface ReleaseOrderScheduleApprovalActionPayload {
  comment?: string;
}

export interface ReleaseOrderScheduleApprovalRecord {
  id: string;
  schedule_id: string;
  action: ReleaseOrderScheduleApprovalAction;
  operator_user_id: string;
  operator_name: string;
  comment: string;
  created_at: string;
}

export interface ReleaseOrderScheduleApprovalRecordListResponse {
  data: ReleaseOrderScheduleApprovalRecord[];
}

export type ReleaseOrderConcurrentBatchQueueState =
  | "pending"
  | "queued"
  | "executing"
  | "success"
  | "failed"
  | "cancelled";

export type BatchExecuteStagedDispatchMode = "execute" | "build";

export interface BatchExecuteReleaseOrdersPayload {
  order_ids: string[];
  batch_name: string;
  staged_dispatch_mode?: BatchExecuteStagedDispatchMode;
}

export interface BatchDeleteReleaseOrdersPayload {
  order_ids: string[];
}

export interface ReleaseOrderConcurrentBatchProgressItem {
  order_id: string;
  order_no: string;
  application_id: string;
  application_name: string;
  env_code: string;
  status: ReleaseOrderStatus;
  operation_type: ReleaseOperationType;
  concurrent_batch_seq: number;
  queue_state: ReleaseOrderConcurrentBatchQueueState;
  queue_position: number;
  has_running_execution: boolean;
  started_at: string | null;
  finished_at: string | null;
}

export interface ReleaseOrderConcurrentBatchProgress {
  order_id: string;
  order_no: string;
  batch_no: string;
  batch_name: string;
  is_concurrent: boolean;
  total: number;
  queued: number;
  executing: number;
  success: number;
  failed: number;
  cancelled: number;
  items: ReleaseOrderConcurrentBatchProgressItem[];
}

export interface ReleaseOrderConcurrentBatchProgressResponse {
  data: ReleaseOrderConcurrentBatchProgress;
}

export interface ReleaseOrderBatchExecuteResult {
  batch_no: string;
  batch_name: string;
  orders: ReleaseOrder[];
  dispatch_errors: string[];
}

export interface ReleaseOrderBatchExecuteResponse {
  data: ReleaseOrderBatchExecuteResult;
}

export interface ReleaseOrderBatchDeleteFailure {
  order_id: string;
  order_no: string;
  reason: string;
}

export interface ReleaseOrderBatchDeleteResult {
  deleted_order_ids: string[];
  failed: ReleaseOrderBatchDeleteFailure[];
}

export interface ReleaseOrderBatchDeleteResponse {
  data: ReleaseOrderBatchDeleteResult;
}

export interface ReleaseOrderPrecheckItem {
  key: string;
  name: string;
  status: "pass" | "warn" | "blocked";
  message: string;
}

export interface ReleaseOrderPrecheck {
  order_id: string;
  order_no: string;
  executable: boolean;
  waiting_for_lock: boolean;
  ahead_count: number;
  lock_enabled: boolean;
  lock_scope: string;
  conflict_strategy: string;
  lock_key: string;
  conflict_order_no: string;
  conflict_message: string;
  items: ReleaseOrderPrecheckItem[];
}

export interface ReleaseOrderPrecheckResponse {
  data: ReleaseOrderPrecheck;
}

export interface ReleaseOrderParamListResponse {
  data: ReleaseOrderParam[];
}

export interface ReleaseOrderValueProgressListResponse {
  data: ReleaseOrderValueProgress[];
}

export interface ReleaseOrderExecutionListResponse {
  data: ReleaseOrderExecution[];
}

export interface ReleaseOrderStepListResponse {
  data: ReleaseOrderStep[];
}

export interface ReleaseOrderPipelineStageListResponse {
  show_module: boolean;
  executor_type: string;
  message?: string;
  data: ReleaseOrderPipelineStage[];
}

export interface ReleaseOrderArtifactMetadataListResponse {
  data: ReleaseOrderArtifactMetadata[];
}

export interface ReleaseOrderRealtimeSnapshot {
  version: string;
  generated_at: string;
  order: ReleaseOrder;
  executions: ReleaseOrderExecution[];
  steps: ReleaseOrderStep[];
  value_progress: ReleaseOrderValueProgress[];
  value_progress_visible: boolean;
  pipeline_stage_view: ReleaseOrderPipelineStageListResponse;
  artifact_metadata: ReleaseOrderArtifactMetadata[];
  approval_records: ReleaseOrderApprovalRecord[];
  concurrent_batch_progress: ReleaseOrderConcurrentBatchProgress | null;
}

export interface ReleaseOrderRealtimeSnapshotResponse {
  data: ReleaseOrderRealtimeSnapshot;
}

export interface ReleaseOrderPipelineStageLogResponse {
  data: {
    stage: ReleaseOrderPipelineStage;
    content: string;
    has_more: boolean;
    raw_status: string;
    fetched_at: string;
  };
}

export interface ReleaseOrderPipelineStageDiagnosisRootCause {
  category: string;
  title: string;
  evidence: string;
  confidence: number;
}

export interface ReleaseOrderPipelineStageDiagnosisAction {
  priority: string;
  action: string;
  owner_hint: string;
}

export interface ReleaseOrderPipelineStageDiagnosisLogLine {
  line_hint: string;
  text: string;
}

export interface ReleaseOrderPipelineStageDiagnosisResult {
  summary: string;
  severity: string;
  confidence: number;
  root_causes: ReleaseOrderPipelineStageDiagnosisRootCause[];
  suggested_actions: ReleaseOrderPipelineStageDiagnosisAction[];
  related_log_lines: ReleaseOrderPipelineStageDiagnosisLogLine[];
  needs_human_review: boolean;
}

export interface ReleaseOrderPipelineStageDiagnosis {
  id: string;
  release_order_id: string;
  stage_id: string;
  ai_model_config_id: string;
  ai_model_name: string;
  ai_model: string;
  status: string;
  result: ReleaseOrderPipelineStageDiagnosisResult;
  error_message: string;
  created_at: string;
  finished_at: string | null;
}

export interface ReleaseOrderPipelineStageDiagnosisResponse {
  data: ReleaseOrderPipelineStageDiagnosis;
}

export type ReleaseOrderPipelineStageDiagnosisChatRole = "user" | "assistant";

export interface ReleaseOrderPipelineStageDiagnosisFollowUpMessage {
  role: ReleaseOrderPipelineStageDiagnosisChatRole;
  content: string;
}

export interface ReleaseOrderPipelineStageDiagnosisFollowUpPayload {
  question: string;
  messages?: ReleaseOrderPipelineStageDiagnosisFollowUpMessage[];
}

export interface ReleaseOrderPipelineStageDiagnosisFollowUp {
  question: string;
  answer: string;
  related_log_lines: ReleaseOrderPipelineStageDiagnosisLogLine[];
  suggested_actions: ReleaseOrderPipelineStageDiagnosisAction[];
  needs_human_review: boolean;
  created_at: string;
}

export interface ReleaseOrderPipelineStageDiagnosisFollowUpResponse {
  data: ReleaseOrderPipelineStageDiagnosisFollowUp;
}

export interface ReleaseOrderLogStreamEvent {
  type: "status" | "log" | "done" | "error" | string;
  timestamp: string;
  message?: string;
  content?: string;
  queue_url?: string;
  build_url?: string;
  offset?: number;
  more_data?: boolean;
  result?: string;
  order_status?: string;
}

export interface CreateReleaseOrderParamPayload {
  pipeline_scope: ReleasePipelineScope;
  param_key: string;
  executor_param_name: string;
  param_value: string;
  value_source?: ReleaseValueSource;
}

export interface CreateReleaseOrderStepPayload {
  step_code: string;
  step_name?: string;
  sort_no?: number;
}

export interface CreateReleaseOrderPayload {
  application_id: string;
  template_id: string;
  release_name?: string;
  env_code?: string;
  project_name?: string;
  git_ref?: string;
  image_tag?: string;
  trigger_type?: ReleaseTriggerType;
  remark?: string;
  triggered_by?: string;
  params?: CreateReleaseOrderParamPayload[];
  steps?: CreateReleaseOrderStepPayload[];
}

export type ApprovalFlowStatus = 'active' | 'disabled'
export type ApprovalFlowGate = 'before_execute' | 'before_ci' | 'before_cd'
export type ApprovalFlowNodeType = 'approval' | 'agent_task'
export type ApprovalFlowTaskStatus = 'pending' | 'running' | 'approved' | 'rejected' | 'failed'
export type ApprovalFlowApproverSource = 'users' | 'manager'

export interface ApprovalFlowNode {
  code: string
  name: string
  gate: ApprovalFlowGate
  node_type: ApprovalFlowNodeType
  applicable_env_codes: string[]
  approval_mode: ReleaseTemplateApprovalMode | ''
  approver_source: ApprovalFlowApproverSource
  manager_level: number
  approver_ids: string[]
  approver_names: string[]
  agent_task_id: string
  agent_task_name: string
  position_x: number
  position_y: number
  sort_no: number
}

export type ApprovalFlowExecutionScope = 'build_only' | 'deploy_only' | 'full_release'
export interface ApprovalFlowLink {
  from_code: string
  to_code: string
  execution_scopes: ApprovalFlowExecutionScope[]
  priority: number
}

export interface ApprovalFlowDefinition {
  id: string
  name: string
  status: ApprovalFlowStatus
  nodes: ApprovalFlowNode[]
  links: ApprovalFlowLink[]
  created_at: string
  updated_at: string
}

export interface ApprovalFlowDefinitionListResponse {
  data: ApprovalFlowDefinition[]
}

export interface ApprovalFlowDefinitionDataResponse {
  data: ApprovalFlowDefinition
}

export interface ApprovalFlowDefinitionPayload {
  name: string
  status: ApprovalFlowStatus
  nodes: Array<Pick<ApprovalFlowNode, 'code' | 'name' | 'gate' | 'node_type' | 'applicable_env_codes' | 'approval_mode' | 'approver_source' | 'manager_level' | 'approver_ids' | 'approver_names' | 'agent_task_id' | 'agent_task_name' | 'position_x' | 'position_y'>>
  links: ApprovalFlowLink[]
}

export interface ReleaseOrderApprovalFlowTaskRecord {
  id: string
  task_id: string
  action: "approve" | "reject"
  operator_user_id: string
  operator_name: string
  comment: string
  created_at: string
}

export interface ReleaseOrderApprovalFlowTask {
  id: string
  node_code: string
  node_name: string
  gate: ApprovalFlowGate
  node_type: ApprovalFlowNodeType
  approval_mode: ReleaseTemplateApprovalMode | ''
  approver_ids: string[]
  approver_names: string[]
  agent_task_id: string
  agent_task_name: string
  agent_batch_id: string
  message: string
  status: ApprovalFlowTaskStatus
  records: ReleaseOrderApprovalFlowTaskRecord[]
  created_at: string
  updated_at: string
}

export interface ReleaseOrderApprovalFlow {
  id: string
  flow_definition_id: string
  flow_name: string
  nodes: ApprovalFlowNode[]
  links: ApprovalFlowLink[]
  status: string
  current_gate: ApprovalFlowGate
  current_scope: ApprovalFlowExecutionScope | ''
  current_node_code: string
  current_task_id: string
  tasks: ReleaseOrderApprovalFlowTask[]
  created_at: string
  updated_at: string
}

export interface ReleaseOrderApprovalFlowDataResponse {
  data: ReleaseOrderApprovalFlow | null
}

export interface ReleaseOrderApprovalFlowTaskDataResponse {
  data: ReleaseOrderApprovalFlowTask
}

export type ReleaseApprovalWorkbenchSource = 'flow' | 'legacy'

export interface ReleaseApprovalWorkbenchTask {
  source: ReleaseApprovalWorkbenchSource
  task_id: string
  release_order_id: string
  order_no: string
  application_id: string
  application_name: string
  env_code: string
  operation_type: ReleaseOperationType
  triggered_by: string
  flow_name: string
  node_name: string
  gate: ApprovalFlowGate
  execution_scope: ApprovalFlowExecutionScope | ''
  approval_mode: ReleaseTemplateApprovalMode | ''
  approver_ids: string[]
  approver_names: string[]
  release_order_status: ReleaseOrderStatus
  created_at: string
  updated_at: string
}

export interface ReleaseApprovalWorkbenchRecord {
  id: string
  source: ReleaseApprovalWorkbenchSource
  task_id: string
  release_order_id: string
  order_no: string
  application_id: string
  application_name: string
  env_code: string
  operation_type: ReleaseOperationType
  triggered_by: string
  flow_name: string
  node_name: string
  gate: ApprovalFlowGate
  execution_scope: ApprovalFlowExecutionScope | ''
  action: 'approve' | 'reject'
  operator_user_id: string
  operator_name: string
  comment: string
  release_order_status: ReleaseOrderStatus
  created_at: string
}

export interface ReleaseApprovalWorkbenchListParams {
  view: 'pending' | 'handled'
  page?: number
  page_size?: number
}

export interface ReleaseApprovalWorkbenchTaskListResponse {
  data: ReleaseApprovalWorkbenchTask[]
  page: number
  page_size: number
  total: number
}

export interface ReleaseApprovalWorkbenchRecordListResponse {
  data: ReleaseApprovalWorkbenchRecord[]
  page: number
  page_size: number
  total: number
}

export interface ReleaseTemplate {
  id: string;
  name: string;
  application_id: string;
  application_name: string;
  binding_id: string;
  binding_name: string;
  binding_type: string;
  gitops_type: ReleaseTemplateGitOpsType;
  status: ReleaseTemplateStatus;
  approval_enabled: boolean;
  approval_mode: ReleaseTemplateApprovalMode | "";
  approval_approver_ids: string[];
  approval_approver_names: string[];
  remark: string;
  param_count: number;
  compliance_status: ReleaseTemplateComplianceStatus;
  compliance_summary: string;
  compliance_findings: ReleaseTemplateComplianceFinding[];
  created_at: string;
  updated_at: string;
}

export interface ReleaseTemplateComplianceFinding {
  pipeline_scope: ReleasePipelineScope;
  pipeline_id: string;
  pipeline_name: string;
  rule_id: string;
  rule_code: string;
  rule_name: string;
  severity: string;
  line_no: number;
  message: string;
  suggestion: string;
}

export interface ReleaseTemplateBinding {
  id: string;
  template_id: string;
  pipeline_scope: ReleasePipelineScope;
  binding_id: string;
  binding_name: string;
  provider: string;
  pipeline_id: string;
  enabled: boolean;
  sort_no: number;
  created_at: string;
  updated_at: string;
}

export interface ReleaseTemplateParam {
  id: string;
  template_id: string;
  template_binding_id: string;
  pipeline_scope: ReleasePipelineScope;
  binding_id: string;
  executor_param_def_id: string;
  param_key: string;
  param_name: string;
  executor_param_name: string;
  value_source: ReleaseTemplateParamValueSource;
  source_param_key: string;
  source_param_name: string;
  fixed_value: string;
  required: boolean;
  sort_no: number;
  created_at: string;
  updated_at: string;
}

export type ReleaseTemplateParamValueSource =
  | "release_input"
  | "fixed"
  | "ci_param"
  | "builtin";

export interface ReleaseTemplateParamConfigPayload {
  executor_param_def_id: string;
  value_source: ReleaseTemplateParamValueSource;
  source_param_key?: string;
  fixed_value?: string;
}

export type ReleaseTemplateGitOpsRuleSourceFrom = "ci" | "builtin" | "cd_input";
export type ReleaseTemplateGitOpsType = "" | "kustomize" | "helm";

export interface ReleaseTemplateGitOpsRule {
  id: string;
  template_id: string;
  pipeline_scope: ReleasePipelineScope;
  source_param_key: string;
  source_param_name: string;
  source_from: ReleaseTemplateGitOpsRuleSourceFrom;
  locator_param_key: string;
  locator_param_name: string;
  file_path_template: string;
  document_kind: string;
  document_name: string;
  target_path: string;
  value_template: string;
  sort_no: number;
  created_at: string;
  updated_at: string;
}

export interface ReleaseTemplateGitOpsRulePayload {
  source_param_key: string;
  source_from: ReleaseTemplateGitOpsRuleSourceFrom;
  locator_param_key?: string;
  file_path_template: string;
  document_kind: string;
  document_name?: string;
  target_path: string;
  value_template?: string;
}

export type ReleaseTemplateHookType = "agent_task" | "notification_hook" | "webhook_notification";
export type ReleaseTemplateHookExecuteStage = "post_release" | "build_complete";
export type ReleaseTemplateHookTriggerCondition = "on_success" | "on_failed" | "always";
export type ReleaseTemplateHookFailurePolicy = "block_release" | "warn_only";

export interface ReleaseTemplateHook {
  id: string;
  template_id: string;
  hook_type: ReleaseTemplateHookType;
  name: string;
  execute_stage: ReleaseTemplateHookExecuteStage;
  execute_stages: ReleaseTemplateHookExecuteStage[];
  trigger_condition: ReleaseTemplateHookTriggerCondition;
  failure_policy: ReleaseTemplateHookFailurePolicy;
  env_codes: string[];
  target_id: string;
  target_name: string;
  webhook_method: string;
  webhook_url: string;
  webhook_body: string;
  note: string;
  sort_no: number;
  created_at: string;
  updated_at: string;
}

export interface ReleaseTemplateHookPayload {
  hook_type: ReleaseTemplateHookType;
  name: string;
  execute_stage?: ReleaseTemplateHookExecuteStage;
  execute_stages: ReleaseTemplateHookExecuteStage[];
  trigger_condition: ReleaseTemplateHookTriggerCondition;
  failure_policy: ReleaseTemplateHookFailurePolicy;
  env_codes?: string[];
  target_id?: string;
  webhook_method?: string;
  webhook_url?: string;
  webhook_body?: string;
  note?: string;
}

export interface ReleaseTemplateListParams {
  application_id?: string;
  binding_id?: string;
  status?: ReleaseTemplateStatus;
  page?: number;
  page_size?: number;
}

export interface ReleaseTemplateListResponse {
  data: ReleaseTemplate[];
  page: number;
  page_size: number;
  total: number;
}

export interface ReleaseTemplateDataResponse {
  data: {
    template: ReleaseTemplate;
    bindings: ReleaseTemplateBinding[];
    params: ReleaseTemplateParam[];
    gitops_rules: ReleaseTemplateGitOpsRule[];
    hooks: ReleaseTemplateHook[];
  };
}

export interface ReleaseTemplatePayload {
  name: string;
  application_id: string;
  ci_binding_id?: string;
  cd_binding_id?: string;
  cd_provider?: string;
  gitops_type?: ReleaseTemplateGitOpsType;
  status: ReleaseTemplateStatus;
  approval_enabled?: boolean;
  approval_mode?: ReleaseTemplateApprovalMode;
  approval_approver_ids?: string[];
  approval_approver_names?: string[];
  remark?: string;
  ci_param_def_ids: string[];
  cd_param_def_ids: string[];
  ci_param_configs?: ReleaseTemplateParamConfigPayload[];
  cd_param_configs?: ReleaseTemplateParamConfigPayload[];
  gitops_rules?: ReleaseTemplateGitOpsRulePayload[];
  hooks?: ReleaseTemplateHookPayload[];
}

export interface ReleaseOrderApprovalRecord {
  id: string;
  release_order_id: string;
  action: "submit" | "approve" | "reject";
  operator_user_id: string;
  operator_name: string;
  comment: string;
  created_at: string;
}

export interface ReleaseOrderApprovalRecordListResponse {
  data: ReleaseOrderApprovalRecord[];
}

export interface ReleaseOrderApprovalRecordSummary {
  id: string;
  release_order_id: string;
  order_no: string;
  order_status: ReleaseOrderStatus;
  business_status: ReleaseOrderBusinessStatus;
  application_id: string;
  application_name: string;
  env_code: string;
  operation_type: ReleaseOperationType;
  triggered_by: string;
  action: "submit" | "approve" | "reject";
  operator_user_id: string;
  operator_name: string;
  comment: string;
  created_at: string;
}

export interface ReleaseOrderApprovalRecordSummaryListParams {
  application_id?: string;
  operator_user_id?: string;
  page?: number;
  page_size?: number;
}

export interface ReleaseOrderApprovalRecordSummaryListResponse {
  data: ReleaseOrderApprovalRecordSummary[];
  page: number;
  page_size: number;
  total: number;
}

export interface ReleaseOrderApprovalActionPayload {
  comment?: string;
}

export interface UpdateReleaseTemplatePayload {
  name: string;
  ci_binding_id?: string;
  cd_binding_id?: string;
  cd_provider?: string;
  gitops_type?: ReleaseTemplateGitOpsType;
  status: ReleaseTemplateStatus;
  approval_enabled?: boolean;
  approval_mode?: ReleaseTemplateApprovalMode;
  approval_approver_ids?: string[];
  approval_approver_names?: string[];
  remark?: string;
  ci_param_def_ids: string[];
  cd_param_def_ids: string[];
  ci_param_configs?: ReleaseTemplateParamConfigPayload[];
  cd_param_configs?: ReleaseTemplateParamConfigPayload[];
  gitops_rules?: ReleaseTemplateGitOpsRulePayload[];
  hooks?: ReleaseTemplateHookPayload[];
}
