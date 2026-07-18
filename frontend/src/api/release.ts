import type { AxiosRequestConfig } from "axios";
import { apiBaseURL, http } from "./http";
import type {
  AppReleaseStateSummaryListResponse,
  ApprovalFlowDefinitionListResponse,
  ApprovalFlowDefinitionDataResponse,
  ApprovalFlowDefinitionPayload,
  ApplicationRollbackCapabilityResponse,
  ApplicationRollbackPrecheckResponse,
  BatchCreateReleaseOrdersPayload,
  BatchDeleteReleaseOrdersPayload,
  BatchExecuteReleaseOrdersPayload,
  CreateReleaseOrderPayload,
  ReleaseOrderApprovalActionPayload,
  ReleaseOrderApprovalRecordListResponse,
  ReleaseOrderApprovalFlowDataResponse,
  ReleaseOrderApprovalFlowTaskDataResponse,
  ReleaseApprovalWorkbenchListParams,
  ReleaseApprovalWorkbenchRecordListResponse,
  ReleaseApprovalWorkbenchTaskListResponse,
  ReleaseOrderApprovalRecordSummaryListParams,
  ReleaseOrderApprovalRecordSummaryListResponse,
  ReleaseOrderScheduleApprovalActionPayload,
  ReleaseOrderScheduleApprovalRecordListResponse,
  ReleaseOrderScheduleDataResponse,
  ReleaseOrderScheduleListParams,
  ReleaseOrderScheduleListResponse,
  ReleaseOrderScheduleMode,
  ReleaseOrderSchedulePayload,
  ReleaseOrderBatchDeleteResponse,
  ReleaseOrderBatchCreateResponse,
  ReleaseOrderBatchExecuteResponse,
  ReleaseOrderConcurrentBatchProgressResponse,
  ReleaseOrderDataResponse,
  ReleaseOrderExecutionListResponse,
  ReleaseOrderArtifactMetadataListResponse,
  ReleaseOrderListParams,
  ReleaseOrderStatsResponse,
  ReleaseOrderPrecheckResponse,
  ReleaseOrderRealtimeSnapshotResponse,
  ReleaseOrderPipelineStageListResponse,
  ReleaseOrderPipelineStageLogResponse,
  ReleaseOrderPipelineStageDiagnosisFollowUpPayload,
  ReleaseOrderPipelineStageDiagnosisFollowUpResponse,
  ReleaseOrderPipelineStageDiagnosisResponse,
  ReleaseOrderListResponse,
  ReleaseOrderParamListResponse,
  ReleaseOrderValueProgressListResponse,
  ReleaseOrderStepListResponse,
  ReleaseTemplate,
  ReleaseTemplateDataResponse,
  ReleaseTemplateListParams,
  ReleaseTemplateListResponse,
  ReleaseTemplatePayload,
  UpdateReleaseTemplatePayload,
} from "../types/release";

export type ReleaseOrderDispatchAction = "execute" | "build" | "deploy";

const RELEASE_TEMPLATE_QUERY_TIMEOUT_MS = 60_000;

export async function listReleaseOrders(
  params: ReleaseOrderListParams,
  config?: AxiosRequestConfig,
): Promise<ReleaseOrderListResponse> {
  const response = await http.get<ReleaseOrderListResponse>("/release-orders", {
    params,
    timeout: 30_000,
    ...config,
  });
  return response.data;
}

export async function listSchedulableReleaseOrdersForSchedule(
  params: ReleaseOrderListParams & { schedule_mode?: ReleaseOrderScheduleMode | "" },
  config?: AxiosRequestConfig,
): Promise<ReleaseOrderListResponse> {
  const response = await http.get<ReleaseOrderListResponse>(
    "/release-order-schedules/schedulable-release-orders",
    {
      params,
      ...config,
    },
  );
  return response.data;
}

export async function getReleaseOrderStats(
  params: ReleaseOrderListParams,
  config?: AxiosRequestConfig,
): Promise<ReleaseOrderStatsResponse> {
  const response = await http.get<ReleaseOrderStatsResponse>("/release-orders/stats", {
    params,
    ...config,
  });
  return response.data;
}

export async function createReleaseOrder(
  payload: CreateReleaseOrderPayload,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    "/release-orders",
    payload,
  );
  return response.data;
}

export async function batchCreateReleaseOrders(
  payload: BatchCreateReleaseOrdersPayload,
): Promise<ReleaseOrderBatchCreateResponse> {
  const response = await http.post<ReleaseOrderBatchCreateResponse>(
    "/release-orders/batch-create",
    payload,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export async function listApprovalFlows(): Promise<ApprovalFlowDefinitionListResponse> {
  const response = await http.get<ApprovalFlowDefinitionListResponse>('/release-approval-flows', {
    params: { status: 'active' },
  })
  return response.data
}

export async function createApprovalFlow(payload: ApprovalFlowDefinitionPayload): Promise<ApprovalFlowDefinitionDataResponse> {
  const response = await http.post<ApprovalFlowDefinitionDataResponse>('/release-approval-flows', payload)
  return response.data
}

export async function updateApprovalFlow(id: string, payload: ApprovalFlowDefinitionPayload): Promise<ApprovalFlowDefinitionDataResponse> {
  const response = await http.put<ApprovalFlowDefinitionDataResponse>(`/release-approval-flows/${encodeURIComponent(id)}`, payload)
  return response.data
}

export async function updateReleaseOrder(
  id: string,
  payload: CreateReleaseOrderPayload,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.put<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}`,
    payload,
  );
  return response.data;
}

export async function batchExecuteReleaseOrders(
  payload: BatchExecuteReleaseOrdersPayload,
): Promise<ReleaseOrderBatchExecuteResponse> {
  const response = await http.post<ReleaseOrderBatchExecuteResponse>(
    "/release-orders/batch-execute",
    payload,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export async function batchDeleteReleaseOrders(
  payload: BatchDeleteReleaseOrdersPayload,
): Promise<ReleaseOrderBatchDeleteResponse> {
  const response = await http.post<ReleaseOrderBatchDeleteResponse>(
    "/release-orders/batch-delete",
    payload,
  );
  return response.data;
}

export async function rollbackReleaseOrderByID(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}/rollback`,
  );
  return response.data;
}

export async function confirmReleaseOrderLive(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}/confirm-live`,
  );
  return response.data;
}

export async function listAppReleaseStateSummaries(
  applicationIDs: string[],
): Promise<AppReleaseStateSummaryListResponse> {
  const response = await http.get<AppReleaseStateSummaryListResponse>(
    "/app-release-states/summaries",
    {
      params: {
        application_ids: applicationIDs.join(","),
      },
    },
  );
  return response.data;
}

export async function getApplicationRollbackCapability(
  applicationID: string,
  payload: { env_code: string },
): Promise<ApplicationRollbackCapabilityResponse> {
  const response = await http.post<ApplicationRollbackCapabilityResponse>(
    `/applications/${encodeURIComponent(String(applicationID || "").trim())}/rollback-capability`,
    payload,
  );
  return response.data;
}

export async function getApplicationRollbackPrecheck(
  applicationID: string,
  payload: { env_code: string; action: "rollback" | "replay" },
): Promise<ApplicationRollbackPrecheckResponse> {
  const response = await http.post<ApplicationRollbackPrecheckResponse>(
    `/applications/${encodeURIComponent(String(applicationID || "").trim())}/rollback-precheck`,
    payload,
  );
  return response.data;
}

export async function createApplicationRollbackOrder(
  applicationID: string,
  payload: { env_code: string; action: "rollback" | "replay" },
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/applications/${encodeURIComponent(String(applicationID || "").trim())}/rollback-orders`,
    payload,
  );
  return response.data;
}

export async function replayReleaseOrderByID(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}/replay`,
  );
  return response.data;
}

export async function getReleaseOrderByID(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.get<ReleaseOrderDataResponse>(
    `/release-orders/${id}`,
  );
  return response.data;
}

export async function getReleaseOrderRealtimeSnapshot(
  id: string,
  config?: AxiosRequestConfig,
): Promise<ReleaseOrderRealtimeSnapshotResponse> {
  const orderID = encodeURIComponent(String(id || "").trim());
  const response = await http.get<ReleaseOrderRealtimeSnapshotResponse>(
    `/release-orders/${orderID}/realtime-snapshot`,
    config,
  );
  return response.data;
}

export async function listReleaseOrderSchedules(
  params: ReleaseOrderScheduleListParams,
  config?: AxiosRequestConfig,
): Promise<ReleaseOrderScheduleListResponse> {
  const response = await http.get<ReleaseOrderScheduleListResponse>(
    "/release-order-schedules",
    {
      params,
      ...config,
    },
  );
  return response.data;
}

export async function getReleaseOrderSchedule(
  releaseOrderID: string,
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.get<ReleaseOrderScheduleDataResponse>(
    `/release-orders/${encodeURIComponent(String(releaseOrderID || "").trim())}/schedule`,
  );
  return response.data;
}

export async function createReleaseOrderSchedule(
  releaseOrderID: string,
  payload: ReleaseOrderSchedulePayload,
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.post<ReleaseOrderScheduleDataResponse>(
    `/release-orders/${encodeURIComponent(String(releaseOrderID || "").trim())}/schedule`,
    payload,
  );
  return response.data;
}

export async function updateReleaseOrderSchedule(
  id: string,
  payload: ReleaseOrderSchedulePayload,
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.put<ReleaseOrderScheduleDataResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}`,
    payload,
  );
  return response.data;
}

export async function cancelReleaseOrderSchedule(
  id: string,
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.post<ReleaseOrderScheduleDataResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}/cancel`,
  );
  return response.data;
}

export async function submitReleaseOrderScheduleApproval(
  id: string,
  payload: ReleaseOrderScheduleApprovalActionPayload = {},
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.post<ReleaseOrderScheduleDataResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}/submit-approval`,
    payload,
  );
  return response.data;
}

export async function approveReleaseOrderSchedule(
  id: string,
  payload: ReleaseOrderScheduleApprovalActionPayload = {},
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.post<ReleaseOrderScheduleDataResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}/approve`,
    payload,
  );
  return response.data;
}

export async function rejectReleaseOrderSchedule(
  id: string,
  payload: ReleaseOrderScheduleApprovalActionPayload,
): Promise<ReleaseOrderScheduleDataResponse> {
  const response = await http.post<ReleaseOrderScheduleDataResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}/reject`,
    payload,
  );
  return response.data;
}

export async function listReleaseOrderScheduleApprovalRecords(
  id: string,
): Promise<ReleaseOrderScheduleApprovalRecordListResponse> {
  const response = await http.get<ReleaseOrderScheduleApprovalRecordListResponse>(
    `/release-order-schedules/${encodeURIComponent(String(id || "").trim())}/approval-records`,
  );
  return response.data;
}

export async function getReleaseOrderPrecheck(
  id: string,
  action: ReleaseOrderDispatchAction = "execute",
): Promise<ReleaseOrderPrecheckResponse> {
  const response = await http.get<ReleaseOrderPrecheckResponse>(
    `/release-orders/${id}/precheck`,
    {
      params: {
        action,
      },
    },
  );
  return response.data;
}

export async function getReleaseOrderConcurrentBatchProgress(
  id: string,
): Promise<ReleaseOrderConcurrentBatchProgressResponse> {
  const response = await http.get<ReleaseOrderConcurrentBatchProgressResponse>(
    `/release-orders/${id}/concurrent-batch-progress`,
  );
  return response.data;
}

export async function listReleaseOrderApprovalRecords(
  id: string,
): Promise<ReleaseOrderApprovalRecordListResponse> {
  const response = await http.get<ReleaseOrderApprovalRecordListResponse>(
    `/release-orders/${id}/approval-records`,
  );
  return response.data;
}

export async function getReleaseOrderApprovalFlow(
  id: string,
): Promise<ReleaseOrderApprovalFlowDataResponse> {
  const response = await http.get<ReleaseOrderApprovalFlowDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || '').trim())}/approval-flow`,
  )
  return response.data
}

export async function approveReleaseOrderApprovalFlowTask(
  orderID: string,
  taskID: string,
  payload: ReleaseOrderApprovalActionPayload = {},
): Promise<ReleaseOrderApprovalFlowTaskDataResponse> {
  const response = await http.post<ReleaseOrderApprovalFlowTaskDataResponse>(
    `/release-orders/${encodeURIComponent(String(orderID || '').trim())}/approval-flow/tasks/${encodeURIComponent(String(taskID || '').trim())}/approve`,
    payload,
  )
  return response.data
}

export async function rejectReleaseOrderApprovalFlowTask(
  orderID: string,
  taskID: string,
  payload: ReleaseOrderApprovalActionPayload,
): Promise<ReleaseOrderApprovalFlowTaskDataResponse> {
  const response = await http.post<ReleaseOrderApprovalFlowTaskDataResponse>(
    `/release-orders/${encodeURIComponent(String(orderID || '').trim())}/approval-flow/tasks/${encodeURIComponent(String(taskID || '').trim())}/reject`,
    payload,
  )
  return response.data
}

export async function listReleaseApprovalWorkbenchTasks(
  params: Omit<ReleaseApprovalWorkbenchListParams, 'view'>,
): Promise<ReleaseApprovalWorkbenchTaskListResponse> {
  const response = await http.get<ReleaseApprovalWorkbenchTaskListResponse>('/release-approval-tasks', {
    params: { ...params, view: 'pending' },
  })
  return response.data
}

export async function listReleaseApprovalWorkbenchRecords(
  params: Omit<ReleaseApprovalWorkbenchListParams, 'view'>,
): Promise<ReleaseApprovalWorkbenchRecordListResponse> {
  const response = await http.get<ReleaseApprovalWorkbenchRecordListResponse>('/release-approval-tasks', {
    params: { ...params, view: 'handled' },
  })
  return response.data
}

export async function listReleaseApprovalRecordSummaries(
  params: ReleaseOrderApprovalRecordSummaryListParams,
): Promise<ReleaseOrderApprovalRecordSummaryListResponse> {
  const response = await http.get<ReleaseOrderApprovalRecordSummaryListResponse>(
    "/release-approval-records",
    { params },
  );
  return response.data;
}

export async function submitReleaseOrderApproval(
  id: string,
  payload: ReleaseOrderApprovalActionPayload = {},
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${id}/submit-approval`,
    payload,
  );
  return response.data;
}

export async function approveReleaseOrder(
  id: string,
  payload: ReleaseOrderApprovalActionPayload = {},
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${id}/approve`,
    payload,
  );
  return response.data;
}

export async function rejectReleaseOrder(
  id: string,
  payload: ReleaseOrderApprovalActionPayload,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${id}/reject`,
    payload,
  );
  return response.data;
}

export async function cancelReleaseOrder(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${id}/cancel`,
  );
  return response.data;
}

export async function deleteReleaseOrder(id: string): Promise<void> {
  await http.delete(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}`,
  );
}

export async function executeReleaseOrder(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${id}/execute`,
    undefined,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export async function buildReleaseOrder(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}/build`,
    undefined,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export async function deployReleaseOrder(
  id: string,
): Promise<ReleaseOrderDataResponse> {
  const response = await http.post<ReleaseOrderDataResponse>(
    `/release-orders/${encodeURIComponent(String(id || "").trim())}/deploy`,
    undefined,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export async function listReleaseOrderParams(
  id: string,
): Promise<ReleaseOrderParamListResponse> {
  const response = await http.get<ReleaseOrderParamListResponse>(
    `/release-orders/${id}/params`,
  );
  return response.data;
}

export async function listReleaseOrderValueProgress(
  id: string,
): Promise<ReleaseOrderValueProgressListResponse> {
  const response = await http.get<ReleaseOrderValueProgressListResponse>(
    `/release-orders/${id}/value-progress`,
  );
  return response.data;
}

export async function listReleaseOrderExecutions(
  id: string,
): Promise<ReleaseOrderExecutionListResponse> {
  const response = await http.get<ReleaseOrderExecutionListResponse>(
    `/release-orders/${id}/executions`,
  );
  return response.data;
}

export async function listReleaseOrderArtifactMetadata(
  releaseOrderID: string,
): Promise<ReleaseOrderArtifactMetadataListResponse> {
  const id = encodeURIComponent(String(releaseOrderID || "").trim());
  const response = await http.get<ReleaseOrderArtifactMetadataListResponse>(
    `/release-orders/${id}/artifact-metadata`,
  );
  return response.data;
}

export async function listReleaseOrderSteps(
  id: string,
): Promise<ReleaseOrderStepListResponse> {
  const response = await http.get<ReleaseOrderStepListResponse>(
    `/release-orders/${id}/steps`,
  );
  return response.data;
}

export async function listReleaseOrderPipelineStages(
  id: string,
  scope?: string,
): Promise<ReleaseOrderPipelineStageListResponse> {
  const response = await http.get<ReleaseOrderPipelineStageListResponse>(
    `/release-orders/${id}/pipeline-stages`,
    {
      params: scope ? { scope } : undefined,
    },
  );
  return response.data;
}

export async function getReleaseOrderPipelineStageLog(
  releaseOrderID: string,
  stageID: string,
): Promise<ReleaseOrderPipelineStageLogResponse> {
  const response = await http.get<ReleaseOrderPipelineStageLogResponse>(
    `/release-orders/${releaseOrderID}/pipeline-stages/${stageID}/log`,
    {
      timeout: 180_000,
    },
  );
  return response.data;
}

export async function createReleaseOrderPipelineStageDiagnosis(
  releaseOrderID: string,
  stageID: string,
  forceRefresh = false,
): Promise<ReleaseOrderPipelineStageDiagnosisResponse> {
  const response = await http.post<ReleaseOrderPipelineStageDiagnosisResponse>(
    `/release-orders/${releaseOrderID}/pipeline-stages/${stageID}/diagnoses`,
    {
      force_refresh: forceRefresh,
    },
    {
      timeout: 180_000,
    },
  );
  return response.data;
}

export async function getLatestReleaseOrderPipelineStageDiagnosis(
  releaseOrderID: string,
  stageID: string,
): Promise<ReleaseOrderPipelineStageDiagnosisResponse> {
  const response = await http.get<ReleaseOrderPipelineStageDiagnosisResponse>(
    `/release-orders/${releaseOrderID}/pipeline-stages/${stageID}/diagnoses/latest`,
  );
  return response.data;
}

export async function followUpReleaseOrderPipelineStageDiagnosis(
  releaseOrderID: string,
  stageID: string,
  diagnosisID: string,
  payload: ReleaseOrderPipelineStageDiagnosisFollowUpPayload,
): Promise<ReleaseOrderPipelineStageDiagnosisFollowUpResponse> {
  const response =
    await http.post<ReleaseOrderPipelineStageDiagnosisFollowUpResponse>(
      `/release-orders/${releaseOrderID}/pipeline-stages/${stageID}/diagnoses/${diagnosisID}/follow-up`,
      payload,
      {
        timeout: 180_000,
      },
    );
  return response.data;
}

export async function listReleaseTemplates(
  params: ReleaseTemplateListParams,
): Promise<ReleaseTemplateListResponse> {
  const response = await http.get<ReleaseTemplateListResponse>(
    "/release-templates",
    {
      params,
      timeout: RELEASE_TEMPLATE_QUERY_TIMEOUT_MS,
    },
  );
  return response.data;
}

export async function listAllReleaseTemplates(
  params: Omit<ReleaseTemplateListParams, "page" | "page_size">,
  pageSize = 200,
): Promise<ReleaseTemplate[]> {
  const items: ReleaseTemplate[] = [];
  let page = 1;
  let total = 0;

  do {
    const response = await listReleaseTemplates({
      ...params,
      page,
      page_size: pageSize,
    });
    items.push(...response.data);
    total = response.total;
    if (response.data.length === 0) {
      break;
    }
    page += 1;
  } while (items.length < total && page <= 50);

  return items;
}

export async function getReleaseTemplateByID(
  id: string,
): Promise<ReleaseTemplateDataResponse> {
  const response = await http.get<ReleaseTemplateDataResponse>(
    `/release-templates/${id}`,
    {
      timeout: RELEASE_TEMPLATE_QUERY_TIMEOUT_MS,
    },
  );
  return response.data;
}

export async function createReleaseTemplate(
  payload: ReleaseTemplatePayload,
): Promise<ReleaseTemplateDataResponse> {
  const response = await http.post<ReleaseTemplateDataResponse>(
    "/release-templates",
    payload,
  );
  return response.data;
}

export async function updateReleaseTemplate(
  id: string,
  payload: UpdateReleaseTemplatePayload,
): Promise<ReleaseTemplateDataResponse> {
  const response = await http.put<ReleaseTemplateDataResponse>(
    `/release-templates/${id}`,
    payload,
  );
  return response.data;
}

export async function deleteReleaseTemplate(id: string): Promise<void> {
  await http.delete(`/release-templates/${id}`);
}

export async function syncTemplateExecutorParamDefs(
  templateID: string,
): Promise<{ data: { total: number; created: number; updated: number; inactivated: number; skipped: number } }> {
  const response = await http.post<{ data: { total: number; created: number; updated: number; inactivated: number; skipped: number } }>(
    `/release-templates/${encodeURIComponent(String(templateID || "").trim())}/sync-executor-param-defs`,
    undefined,
    {
      timeout: 120_000,
    },
  );
  return response.data;
}

export function buildReleaseOrderLogStreamURL(
  id: string,
  start = 0,
  accessToken = "",
  scope = "",
): string {
  const base = apiBaseURL.replace(/\/+$/, "");
  const orderID = encodeURIComponent(String(id || "").trim());
  const offset = Number.isFinite(start) && start > 0 ? Math.floor(start) : 0;
  const token = String(accessToken || "").trim();
  const scopeParam = String(scope || "").trim();
  const params = [`start=${offset}`];
  if (scopeParam) {
    params.push(`scope=${encodeURIComponent(scopeParam)}`);
  }
  if (!token) {
    return `${base}/release-orders/${orderID}/logs/stream?${params.join("&")}`;
  }
  params.push(`access_token=${encodeURIComponent(token)}`);
  return `${base}/release-orders/${orderID}/logs/stream?${params.join("&")}`;
}

export function buildReleaseOrderRealtimeEventsURL(id: string): string {
  const base = apiBaseURL.replace(/\/+$/, "");
  const orderID = encodeURIComponent(String(id || "").trim());
  return `${base}/release-orders/${orderID}/events`;
}
