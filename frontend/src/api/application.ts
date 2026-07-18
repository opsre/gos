import { http } from './http'
import type {
  ApplicationDataResponse,
  ApplicationListParams,
  ApplicationListResponse,
  ApplicationWorkbenchResponse,
  ApplicationOptionListResponse,
  ApplicationPayload,
} from '../types/application'

export async function listApplications(params: ApplicationListParams): Promise<ApplicationListResponse> {
  const response = await http.get<ApplicationListResponse>('/applications', { params })
  return response.data
}

export async function getApplicationWorkbench(params: ApplicationListParams): Promise<ApplicationWorkbenchResponse> {
  const response = await http.get<ApplicationWorkbenchResponse>('/applications/workbench', { params })
  return response.data
}

export async function getApplicationByID(id: string): Promise<ApplicationDataResponse> {
  const response = await http.get<ApplicationDataResponse>(`/applications/${id}`)
  return response.data
}

export async function listApplicationOptions(): Promise<ApplicationOptionListResponse> {
  const response = await http.get<ApplicationOptionListResponse>('/applications/options')
  return response.data
}

export async function createApplication(payload: ApplicationPayload): Promise<ApplicationDataResponse> {
  const response = await http.post<ApplicationDataResponse>('/applications', payload)
  return response.data
}

export async function updateApplication(id: string, payload: ApplicationPayload): Promise<ApplicationDataResponse> {
  const response = await http.put<ApplicationDataResponse>(`/applications/${id}`, payload)
  return response.data
}

export async function deleteApplication(id: string): Promise<void> {
  await http.delete(`/applications/${id}`)
}

export interface ApplicationApprovalFlowBindingResponse {
  data: { application_id: string; approval_flow_id: string }
}

export async function getApplicationApprovalFlowBinding(id: string): Promise<ApplicationApprovalFlowBindingResponse> {
  const response = await http.get<ApplicationApprovalFlowBindingResponse>(`/applications/${encodeURIComponent(id)}/approval-flow`)
  return response.data
}

export async function updateApplicationApprovalFlowBinding(id: string, approvalFlowID: string): Promise<ApplicationApprovalFlowBindingResponse> {
  const response = await http.put<ApplicationApprovalFlowBindingResponse>(`/applications/${encodeURIComponent(id)}/approval-flow`, {
    approval_flow_id: approvalFlowID,
  })
  return response.data
}
