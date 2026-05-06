import type { AxiosRequestConfig } from "axios";
import { apiBaseURL, http } from "./http";
import type {
  AppReleaseStateSummaryListResponse,
  ApplicationRollbackCapabilityResponse,
  ApplicationRollbackPrecheckResponse,
  BatchDeleteReleaseOrdersPayload,
  BatchExecuteReleaseOrdersPayload,
  CreateReleaseOrderPayload,
  ReleaseOrderApprovalActionPayload,
  ReleaseOrderApprovalRecordListResponse,
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
  ReleaseOrderBatchExecuteResponse,
  ReleaseOrderConcurrentBatchProgressResponse,
  ReleaseOrderDataResponse,
  ReleaseOrderExecutionListResponse,
  ReleaseOrderListParams,
  ReleaseOrderStatsResponse,
  ReleaseOrderPrecheckResponse,
  ReleaseOrderPipelineStageListResponse,
  ReleaseOrderPipelineStageLogResponse,
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

export async function listReleaseTemplates(
  params: ReleaseTemplateListParams,
): Promise<ReleaseTemplateListResponse> {
  const response = await http.get<ReleaseTemplateListResponse>(
    "/release-templates",
    { params },
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
