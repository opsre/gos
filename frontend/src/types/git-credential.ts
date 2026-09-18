export type GitCredentialProvider = 'gitlab'
export type GitCredentialAuthType = 'token' | 'password'
export type GitCredentialStatus = 'active' | 'disabled'

export interface GitCredential {
  id: string
  name: string
  provider: GitCredentialProvider
  base_url: string
  username: string
  auth_type: GitCredentialAuthType
  status: GitCredentialStatus
  // 密钥只写不读，接口仅回传是否已配置，页面据此提示「留空表示不修改」。
  secret_configured: boolean
  remark: string
  created_at: string
  updated_at: string
}

export interface GitCredentialPayload {
  name: string
  provider?: GitCredentialProvider
  base_url: string
  username: string
  // 编辑时留空表示保留原密钥，此时不提交该字段。
  secret?: string
  auth_type: GitCredentialAuthType
  status?: GitCredentialStatus
  remark?: string
}

export interface GitCredentialListParams {
  keyword?: string
  status?: GitCredentialStatus
  page?: number
  page_size?: number
}

export interface GitCredentialListResponse {
  data: GitCredential[]
  page: number
  page_size: number
  total: number
}

export interface GitCredentialDataResponse {
  data: GitCredential
}

export interface GitCredentialTestResult {
  ok: boolean
  message: string
  gitlab_username?: string
}

export interface GitCredentialTestResponse {
  data: GitCredentialTestResult
}
