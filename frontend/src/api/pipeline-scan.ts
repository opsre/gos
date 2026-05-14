import { http } from './http'
import type {
  PipelineScanBatchResponse,
  PipelineScanResultDataResponse,
  PipelineScanResultListParams,
  PipelineScanResultListResponse,
  PipelineScanRuleDataResponse,
  PipelineScanRuleListParams,
  PipelineScanRuleListResponse,
  PipelineScanRulePayload,
} from '../types/pipeline-scan'

export async function listPipelineScanRules(
  params: PipelineScanRuleListParams,
): Promise<PipelineScanRuleListResponse> {
  const response = await http.get<PipelineScanRuleListResponse>('/pipeline-scan/rules', { params })
  return response.data
}

export async function getPipelineScanRule(id: string): Promise<PipelineScanRuleDataResponse> {
  const response = await http.get<PipelineScanRuleDataResponse>(`/pipeline-scan/rules/${id}`)
  return response.data
}

export async function createPipelineScanRule(
  payload: PipelineScanRulePayload,
): Promise<PipelineScanRuleDataResponse> {
  const response = await http.post<PipelineScanRuleDataResponse>('/pipeline-scan/rules', payload)
  return response.data
}

export async function updatePipelineScanRule(
  id: string,
  payload: PipelineScanRulePayload,
): Promise<PipelineScanRuleDataResponse> {
  const response = await http.put<PipelineScanRuleDataResponse>(`/pipeline-scan/rules/${id}`, payload)
  return response.data
}

export async function setPipelineScanRuleEnabled(
  id: string,
  enabled: boolean,
): Promise<PipelineScanRuleDataResponse> {
  const response = await http.patch<PipelineScanRuleDataResponse>(`/pipeline-scan/rules/${id}/enabled`, { enabled })
  return response.data
}

export async function deletePipelineScanRule(id: string): Promise<void> {
  await http.delete(`/pipeline-scan/rules/${id}`)
}

export async function listPipelineScanResults(
  params: PipelineScanResultListParams,
): Promise<PipelineScanResultListResponse> {
  const response = await http.get<PipelineScanResultListResponse>('/pipeline-scan/results', { params })
  return response.data
}

export async function getPipelineScanResult(id: string): Promise<PipelineScanResultDataResponse> {
  const response = await http.get<PipelineScanResultDataResponse>(`/pipelines/${id}/scan-result`)
  return response.data
}

export async function scanPipeline(id: string): Promise<PipelineScanResultDataResponse> {
  const response = await http.post<PipelineScanResultDataResponse>(`/pipelines/${id}/scan`, undefined, {
    timeout: 120_000,
  })
  return response.data
}

export async function scanAllPipelines(): Promise<PipelineScanBatchResponse> {
  const response = await http.post<PipelineScanBatchResponse>('/pipeline-scan/scan', undefined, {
    timeout: 120_000,
  })
  return response.data
}
