export type ArtifactRepositoryType = 'oss' | 'ftp' | 'sftp'
export type ArtifactRepositoryACL = 'private' | 'public-read'
export type ArtifactRepositoryStatus = 'enabled' | 'disabled'

export interface ArtifactRepository {
  id: string
  name: string
  type: ArtifactRepositoryType
  endpoint: string
  port: number
  bucket: string
  directory: string
  access_key_id: string
  username: string
  disable_epsv: boolean
  host_key_fingerprint: string
  // Credentials are never returned by the API; these flags only report whether
  // one is stored so the form can show state without the secret.
  secret_configured: boolean
  private_key_configured: boolean
  acl: ArtifactRepositoryACL
  status: ArtifactRepositoryStatus
  created_at: string
  updated_at: string
}

export interface ArtifactRepositoryPayload {
  name: string
  type: ArtifactRepositoryType
  endpoint: string
  port?: number
  bucket?: string
  directory?: string
  access_key_id?: string
  // Blank means "keep the stored secret" when editing.
  access_key_secret?: string
  username?: string
  password?: string
  private_key?: string
  disable_epsv?: boolean
  host_key_fingerprint?: string
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
