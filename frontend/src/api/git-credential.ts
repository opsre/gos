import { http } from './http'
import type {
  GitCredentialDataResponse,
  GitCredentialListParams,
  GitCredentialListResponse,
  GitCredentialPayload,
  GitCredentialTestResponse,
} from '../types/git-credential'

export async function listGitCredentials(
  params: GitCredentialListParams,
): Promise<GitCredentialListResponse> {
  const response = await http.get<GitCredentialListResponse>('/git-credentials', { params })
  return response.data
}

export async function getGitCredentialByID(id: string): Promise<GitCredentialDataResponse> {
  const response = await http.get<GitCredentialDataResponse>(`/git-credentials/${id}`)
  return response.data
}

export async function createGitCredential(
  payload: GitCredentialPayload,
): Promise<GitCredentialDataResponse> {
  const response = await http.post<GitCredentialDataResponse>('/git-credentials', payload)
  return response.data
}

export async function updateGitCredential(
  id: string,
  payload: GitCredentialPayload,
): Promise<GitCredentialDataResponse> {
  const response = await http.put<GitCredentialDataResponse>(`/git-credentials/${id}`, payload)
  return response.data
}

export async function deleteGitCredential(id: string): Promise<void> {
  await http.delete(`/git-credentials/${id}`)
}

export async function testGitCredentialByID(id: string): Promise<GitCredentialTestResponse> {
  const response = await http.post<GitCredentialTestResponse>(`/git-credentials/${id}/test`)
  return response.data
}
