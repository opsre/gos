export type ReleaseConcurrencyLockScope = 'application' | 'application_env' | 'gitops_repo_branch'
export type ReleaseConcurrencyConflictStrategy = 'reject' | 'queue'

export interface ReleaseConcurrencySettings {
  enabled: boolean
  lock_scope: ReleaseConcurrencyLockScope
  conflict_strategy: ReleaseConcurrencyConflictStrategy
  lock_timeout_sec: number
}

export interface ReleaseGitOpsConfig {
  helm_scan_path: string
  kustomize_scan_path: string
}

export interface ReleaseEnvironmentConfig {
  code: string
  description: string
}

export interface ReleaseSettings {
  env_options: string[]
  env_configs: ReleaseEnvironmentConfig[]
  default_env_code: string
  concurrency: ReleaseConcurrencySettings
  gitops_config: ReleaseGitOpsConfig
}

export interface ReleaseSettingsDataResponse {
  data: ReleaseSettings
}

export interface UpdateReleaseSettingsPayload {
  env_options: string[]
  env_configs: ReleaseEnvironmentConfig[]
  default_env_code: string
  concurrency: ReleaseConcurrencySettings
  gitops_config: ReleaseGitOpsConfig
}

export interface SystemManagementSettings {
  current_site_url: string
}

export interface SystemManagementSettingsDataResponse {
  data: SystemManagementSettings
}

export type UpdateSystemManagementSettingsPayload = SystemManagementSettings

export interface AIModelConfig {
  id: string
  name: string
  provider: string
  base_url: string
  model: string
  api_key_configured: boolean
  temperature: number
  max_tokens: number
  timeout_sec: number
  enabled: boolean
  is_diagnosis_model: boolean
  created_by: string
  created_at: string
  updated_at: string
}

export interface AIModelConfigPayload {
  name: string
  provider: string
  base_url: string
  model: string
  api_key?: string
  temperature: number
  max_tokens: number
  timeout_sec: number
  enabled: boolean
}

export interface AIModelConfigListResponse {
  data: AIModelConfig[]
}

export interface AIModelConfigDataResponse {
  data: AIModelConfig
}

export interface AIModelConfigTestResponse {
  data: {
    ok: boolean
  }
}
