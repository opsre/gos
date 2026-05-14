import { http } from './http'
import type {
  ArtifactRepositoryConnectionTestResponse,
  ArtifactRepositoryDataResponse,
  ArtifactRepositoryListParams,
  ArtifactRepositoryListResponse,
  ArtifactRepositoryPayload,
} from '../types/artifact-repository'

export async function listArtifactRepositories(
  params: ArtifactRepositoryListParams,
): Promise<ArtifactRepositoryListResponse> {
  const response = await http.get<ArtifactRepositoryListResponse>('/artifact-repositories', { params })
  return response.data
}

export async function getArtifactRepositoryByID(id: string): Promise<ArtifactRepositoryDataResponse> {
  const response = await http.get<ArtifactRepositoryDataResponse>(`/artifact-repositories/${id}`)
  return response.data
}

export async function createArtifactRepository(
  payload: ArtifactRepositoryPayload,
): Promise<ArtifactRepositoryDataResponse> {
  const response = await http.post<ArtifactRepositoryDataResponse>('/artifact-repositories', payload)
  return response.data
}

export async function updateArtifactRepository(
  id: string,
  payload: ArtifactRepositoryPayload,
): Promise<ArtifactRepositoryDataResponse> {
  const response = await http.put<ArtifactRepositoryDataResponse>(`/artifact-repositories/${id}`, payload)
  return response.data
}

export async function deleteArtifactRepository(id: string): Promise<void> {
  await http.delete(`/artifact-repositories/${id}`)
}

export async function testArtifactRepositoryConnection(
  payload: ArtifactRepositoryPayload,
): Promise<ArtifactRepositoryConnectionTestResponse> {
  const response = await http.post<ArtifactRepositoryConnectionTestResponse>('/artifact-repositories/actions/test-connection', payload)
  return response.data
}
