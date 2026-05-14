export type ArtifactRepositoryType = 'oss'
export type ArtifactRepositoryACL = 'private' | 'public-read'
export type ArtifactRepositoryStatus = 'enabled' | 'disabled'

export interface ArtifactRepository {
  id: string
  name: string
  type: ArtifactRepositoryType
  endpoint: string
  bucket: string
  directory: string
  access_key_id: string
  access_key_secret: string
  acl: ArtifactRepositoryACL
  status: ArtifactRepositoryStatus
  created_at: string
  updated_at: string
}

export interface ArtifactRepositoryPayload {
  name: string
  type: ArtifactRepositoryType
  endpoint: string
  bucket: string
  directory: string
  access_key_id: string
  access_key_secret: string
  acl: ArtifactRepositoryACL
  status: ArtifactRepositoryStatus
}

export interface ArtifactRepositoryListParams {
  keyword?: string
  type?: ArtifactRepositoryType
  status?: ArtifactRepositoryStatus
  page?: number
  page_size?: number
}

export interface ArtifactRepositoryDataResponse {
  data: ArtifactRepository
}

export interface ArtifactRepositoryListResponse {
  data: ArtifactRepository[]
  page: number
  page_size: number
  total: number
}

export interface ArtifactRepositoryConnectionTestResponse {
  success: boolean
  message: string
}
