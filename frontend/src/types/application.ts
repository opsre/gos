export type ApplicationStatus = 'active' | 'inactive'

export interface GitOpsBranchMapping {
  env_code: string
  branch: string
}

export interface ReleaseBranchOption {
  name: string
  branch: string
}

export interface Application {
  id: string
  name: string
  key: string
  project_id: string
  project_name: string
  project_key: string
  repo_url: string
  description: string
  owner_user_id: string
  owner: string
  status: ApplicationStatus
  artifact_type: string
  artifact_repository_id: string
  artifact_directory: string
  language: string
  gitops_branch_mappings: GitOpsBranchMapping[]
  release_branches: ReleaseBranchOption[]
  created_at: string
  updated_at: string
}

export interface ApplicationPayload {
  name: string
  key: string
  project_id: string
  repo_url: string
  description: string
  owner_user_id: string
  status: ApplicationStatus
  artifact_type: string
  artifact_repository_id: string
  artifact_directory: string
  language: string
  gitops_branch_mappings: GitOpsBranchMapping[]
  release_branches: ReleaseBranchOption[]
}

export interface ApplicationListParams {
  keyword?: string
  key?: string
  name?: string
  project_id?: string
  status?: ApplicationStatus
  page?: number
  page_size?: number
}

export interface ApplicationDataResponse {
  data: Application
}

export interface ApplicationListResponse {
  data: Application[]
  page: number
  page_size: number
  total: number
}

export interface ApplicationOption {
  id: string
  name: string
  key: string
}

export interface ApplicationOptionListResponse {
  data: ApplicationOption[]
}

export interface ErrorResponse {
  error: string
}
