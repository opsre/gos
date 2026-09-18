import type { AxiosRequestConfig } from "axios";
import { http } from "./http";
import type {
  ReleaseAutomationCheckResponse,
  ReleaseAutomationDataResponse,
  ReleaseAutomationListParams,
  ReleaseAutomationListResponse,
  ReleaseAutomationPayload,
} from "../types/release-automation";

/**
 * git 校验与保存都会真实访问远端仓库，网络较慢时容易超过默认 10s，
 * 这里统一放宽到 60s，避免把「校验超时」误报成 git 权限错误。
 */
const RELEASE_AUTOMATION_GIT_TIMEOUT_MS = 60_000;

export async function listReleaseAutomations(
  params: ReleaseAutomationListParams,
  config?: AxiosRequestConfig,
): Promise<ReleaseAutomationListResponse> {
  const response = await http.get<ReleaseAutomationListResponse>(
    "/release-automations",
    {
      params,
      timeout: 30_000,
      ...config,
    },
  );
  return response.data;
}

export async function getReleaseAutomationByID(
  id: string,
): Promise<ReleaseAutomationDataResponse> {
  const response = await http.get<ReleaseAutomationDataResponse>(
    `/release-automations/${encodeURIComponent(String(id || "").trim())}`,
  );
  return response.data;
}

export async function createReleaseAutomation(
  payload: ReleaseAutomationPayload,
): Promise<ReleaseAutomationDataResponse> {
  const response = await http.post<ReleaseAutomationDataResponse>(
    "/release-automations",
    payload,
    { timeout: RELEASE_AUTOMATION_GIT_TIMEOUT_MS },
  );
  return response.data;
}

export async function updateReleaseAutomation(
  id: string,
  payload: ReleaseAutomationPayload,
): Promise<ReleaseAutomationDataResponse> {
  const response = await http.put<ReleaseAutomationDataResponse>(
    `/release-automations/${encodeURIComponent(String(id || "").trim())}`,
    payload,
    { timeout: RELEASE_AUTOMATION_GIT_TIMEOUT_MS },
  );
  return response.data;
}

export async function deleteReleaseAutomation(id: string): Promise<void> {
  await http.delete(
    `/release-automations/${encodeURIComponent(String(id || "").trim())}`,
  );
}

/** 立即检查：返回分支可达性、HEAD sha 与可读说明。 */
export async function checkReleaseAutomation(
  id: string,
): Promise<ReleaseAutomationCheckResponse> {
  const response = await http.post<ReleaseAutomationCheckResponse>(
    `/release-automations/${encodeURIComponent(String(id || "").trim())}/check`,
    undefined,
    { timeout: RELEASE_AUTOMATION_GIT_TIMEOUT_MS },
  );
  return response.data;
}
