import { http } from './http'
import type {
  ReleaseOrderArtifactListParams,
  ReleaseOrderArtifactListResponse,
  ReleaseOrderArtifactMetadataDataResponse,
  ReleaseOrderArtifactMetadataPayload,
} from '../types/artifact'

export async function listReleaseOrderArtifacts(
  params: ReleaseOrderArtifactListParams,
): Promise<ReleaseOrderArtifactListResponse> {
  const response = await http.get<ReleaseOrderArtifactListResponse>('/artifacts/release-order-metadata', { params })
  return response.data
}

export async function recordReleaseOrderArtifactMetadata(
  releaseOrderID: string,
  payload: ReleaseOrderArtifactMetadataPayload,
): Promise<ReleaseOrderArtifactMetadataDataResponse> {
  const id = encodeURIComponent(String(releaseOrderID || '').trim())
  const response = await http.post<ReleaseOrderArtifactMetadataDataResponse>(`/release-orders/${id}/artifact-metadata`, payload)
  return response.data
}

export async function deleteReleaseOrderArtifactMetadata(
  releaseOrderID: string,
  artifactID: string,
): Promise<void> {
  const releaseID = encodeURIComponent(String(releaseOrderID || '').trim())
  const id = encodeURIComponent(String(artifactID || '').trim())
  await http.delete(`/release-orders/${releaseID}/artifact-metadata/${id}`)
}
