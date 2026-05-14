export interface ReleaseOrderArtifactMetadataSummary {
  id: string
  release_order_id: string
  execution_id: string
  pipeline_scope: string
  artifact_name: string
  artifact_type: string
  artifact_version: string
  artifact_url: string
  repository_id: string
  repository_name: string
  bucket: string
  object_key: string
  checksum: string
  checksum_type: string
  size_bytes: number
  build_number: string
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
  release_order_no: string
  release_name: string
  release_display_name: string
  application_id: string
  application_name: string
  project_id: string
  project_name: string
  project_key: string
  env_code: string
  order_status: string
}

export interface ReleaseOrderArtifactListParams {
  project_id?: string
  application_id?: string
  release_order_id?: string
  artifact_name?: string
  artifact_type?: string
  pipeline_scope?: string
  repository_id?: string
  created_at_from?: string
  created_at_to?: string
  page?: number
  page_size?: number
}

export interface ReleaseOrderArtifactListResponse {
  data: ReleaseOrderArtifactMetadataSummary[]
  page: number
  page_size: number
  total: number
}

export interface ReleaseOrderArtifactMetadataPayload {
  execution_id?: string
  pipeline_scope?: string
  artifact_name: string
  artifact_type?: string
  artifact_version?: string
  artifact_url: string
  repository_id?: string
  repository_name?: string
  bucket?: string
  object_key?: string
  checksum?: string
  checksum_type?: string
  size_bytes?: number
  build_number?: string
  metadata?: Record<string, unknown>
}

export interface ReleaseOrderArtifactMetadataDataResponse {
  data: ReleaseOrderArtifactMetadataSummary
}
