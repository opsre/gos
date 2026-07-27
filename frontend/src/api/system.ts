import { http } from './http'
import type {
  AIModelConfigDataResponse,
  AIModelConfigListResponse,
  AIModelConfigPayload,
  AIModelConfigTestResponse,
  ReleaseSettingsDataResponse,
  SystemManagementSettingsDataResponse,
  UpdateSystemManagementSettingsPayload,
  UpdateReleaseSettingsPayload,
} from '../types/system'

export async function getReleaseSettings(): Promise<ReleaseSettingsDataResponse> {
  const response = await http.get<ReleaseSettingsDataResponse>('/system/settings/release')
  return response.data
}

export async function updateReleaseSettings(
  payload: UpdateReleaseSettingsPayload,
): Promise<ReleaseSettingsDataResponse> {
  const response = await http.put<ReleaseSettingsDataResponse>('/system/settings/release', payload)
  return response.data
}

export async function getSystemManagementSettings(): Promise<SystemManagementSettingsDataResponse> {
  const response = await http.get<SystemManagementSettingsDataResponse>('/system/settings/system')
  return response.data
}

export async function updateSystemManagementSettings(
  payload: UpdateSystemManagementSettingsPayload,
): Promise<SystemManagementSettingsDataResponse> {
  const response = await http.put<SystemManagementSettingsDataResponse>(
    '/system/settings/system',
    payload,
  )
  return response.data
}

export async function listAIModelConfigs(): Promise<AIModelConfigListResponse> {
  const response = await http.get<AIModelConfigListResponse>('/system/ai-model-configs')
  return response.data
}

export async function createAIModelConfig(
  payload: AIModelConfigPayload,
): Promise<AIModelConfigDataResponse> {
  const response = await http.post<AIModelConfigDataResponse>('/system/ai-model-configs', payload)
  return response.data
}

export async function updateAIModelConfig(
  id: string,
  payload: AIModelConfigPayload,
): Promise<AIModelConfigDataResponse> {
  const response = await http.put<AIModelConfigDataResponse>(`/system/ai-model-configs/${id}`, payload)
  return response.data
}

export async function deleteAIModelConfig(id: string): Promise<{ data: { deleted: boolean } }> {
  const response = await http.delete<{ data: { deleted: boolean } }>(`/system/ai-model-configs/${id}`)
  return response.data
}

export async function testAIModelConfig(id: string): Promise<AIModelConfigTestResponse> {
  const response = await http.post<AIModelConfigTestResponse>(`/system/ai-model-configs/${id}/test`)
  return response.data
}

export async function setDiagnosisAIModelConfig(id: string): Promise<AIModelConfigDataResponse> {
  const response = await http.post<AIModelConfigDataResponse>(
    `/system/ai-model-configs/${id}/set-diagnosis-model`,
  )
  return response.data
}

export async function unsetDiagnosisAIModelConfig(id: string): Promise<AIModelConfigDataResponse> {
  const response = await http.post<AIModelConfigDataResponse>(
    `/system/ai-model-configs/${id}/unset-diagnosis-model`,
  )
  return response.data
}
