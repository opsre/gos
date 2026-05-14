export type PipelineScanCategory = 'artifact' | 'security' | 'credential' | 'naming' | 'custom'
export type PipelineScanSeverity = 'info' | 'warning' | 'error'
export type PipelineScanStatus = 'compliant' | 'warning' | 'failed' | 'unknown'
export type PipelineScanFindingStatus = 'open' | 'ignored' | 'fixed'
export type PipelineScanRuleType =
  | 'artifact_oss_command_format_standard'
  | 'artifact_oss_pipeline_params_standard'
  | 'artifact_gos_artifact_url_standard'
export type PipelineScanTemplateValidationScope = 'ci' | 'cd'

export interface PipelineScanRule {
  id: string
  rule_type?: PipelineScanRuleType | ''
  rule_code: string
  rule_name: string
  category: PipelineScanCategory
  severity: PipelineScanSeverity
  enabled: boolean
  builtin: boolean
  template_validation_scopes: PipelineScanTemplateValidationScope[]
  scope_json: string
  rule_dsl_json: string
  message: string
  suggestion: string
  created_at: string
  updated_at: string
}

export interface PipelineScanRulePayload {
  rule_type: PipelineScanRuleType
  rule_name: string
  category: PipelineScanCategory
  severity: PipelineScanSeverity
  enabled: boolean
  template_validation_scopes: PipelineScanTemplateValidationScope[]
  scope_json: string
  rule_dsl_json: string
  message: string
  suggestion?: string
}

export interface PipelineScanRuleListParams {
  keyword?: string
  category?: PipelineScanCategory | ''
  severity?: PipelineScanSeverity | ''
  enabled?: boolean
  page?: number
  page_size?: number
}

export interface PipelineScanRuleListResponse {
  data: PipelineScanRule[]
  page: number
  page_size: number
  total: number
}

export interface PipelineScanRuleDataResponse {
  data: PipelineScanRule
}

export interface PipelineScanResult {
  id: string
  pipeline_id: string
  pipeline_name: string
  scan_status: PipelineScanStatus
  total_findings: number
  error_count: number
  warning_count: number
  info_count: number
  script_hash: string
  last_scanned_at: string
  created_at: string
  updated_at: string
}

export interface PipelineScanFinding {
  id: string
  pipeline_id: string
  rule_id: string
  rule_code: string
  rule_name: string
  severity: PipelineScanSeverity
  line_no: number
  matched_text: string
  message: string
  suggestion: string
  details_json: string
  status: PipelineScanFindingStatus
  created_at: string
  updated_at: string
}

export interface PipelineScanResultData {
  result: PipelineScanResult
  findings: PipelineScanFinding[]
}

export interface PipelineScanResultDataResponse {
  data: PipelineScanResultData
}

export interface PipelineScanResultListParams {
  pipeline_name?: string
  scan_status?: PipelineScanStatus | ''
  page?: number
  page_size?: number
}

export interface PipelineScanResultListResponse {
  data: PipelineScanResult[]
  page: number
  page_size: number
  total: number
}

export interface PipelineScanBatchResult {
  total: number
  scanned: number
  skipped: number
  failed: number
}

export interface PipelineScanBatchResponse {
  data: PipelineScanBatchResult
}
