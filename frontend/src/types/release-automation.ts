import type { ReleasePipelineScope, ReleaseValueSource } from "./release";

/**
 * 自动化派发方式：
 * - build：仅构建（只触发 CI）
 * - build_deploy：构建并发布（CI + CD 全流程）
 * - execute：仅发布（直接派发已构建产物）
 */
export type ReleaseAutomationDispatchMode = "build" | "build_deploy" | "execute";

/** 自动化参数覆盖项；留空的字段表示沿用发布模板默认配置。 */
export interface ReleaseAutomationParam {
  pipeline_scope: ReleasePipelineScope;
  param_key: string;
  executor_param_name: string;
  param_value: string;
  value_source: ReleaseValueSource | string;
}

/** 参数编辑器行，额外携带前端行标识，便于增删行时保持渲染稳定。 */
export interface ReleaseAutomationParamDraft extends ReleaseAutomationParam {
  row_id: string;
}

export interface ReleaseAutomation {
  id: string;
  name: string;
  application_id: string;
  application_name: string;
  template_id: string;
  template_name: string;
  env_code: string;
  git_ref: string;
  dispatch_mode: ReleaseAutomationDispatchMode;
  enabled: boolean;
  params: ReleaseAutomationParam[];
  /** 最近一次检查到的分支 HEAD sha。 */
  last_seen_sha: string;
  /** 最近一次触发时的 sha，与 last_seen_sha 不一致说明有新提交待发布。 */
  last_triggered_sha: string;
  last_order_id: string;
  last_checked_at: string | null;
  last_error: string;
  created_at: string;
  updated_at: string;
}

export interface ReleaseAutomationListParams {
  keyword?: string;
  /** 不传表示全部状态；true 只查启用，false 只查停用。 */
  enabled?: boolean;
  application_id?: string;
  page?: number;
  page_size?: number;
}

export interface ReleaseAutomationListResponse {
  data: ReleaseAutomation[];
  page: number;
  page_size: number;
  total: number;
}

export interface ReleaseAutomationDataResponse {
  data: ReleaseAutomation;
}

/** 创建后 check_git 默认开启，服务端会先校验 git 权限，失败返回 400 与可读原因。 */
export interface ReleaseAutomationPayload {
  name: string;
  application_id: string;
  template_id: string;
  env_code: string;
  git_ref: string;
  dispatch_mode: ReleaseAutomationDispatchMode;
  enabled: boolean;
  params?: ReleaseAutomationParam[];
  remark?: string;
  check_git?: boolean;
}

export interface ReleaseAutomationCheckResult {
  reachable: boolean;
  head_sha: string;
  message: string;
}

export interface ReleaseAutomationCheckResponse {
  data: ReleaseAutomationCheckResult;
}
