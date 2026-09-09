export interface OnboardingParameter {
  scope: 'ci' | 'cd'; name: string; param_key: string; new_field_name: string
  value_source: string; source_param_key: string; fixed_value: string; omit: boolean
}
export interface OnboardingDraft {
  identity: { project_id: string; project_name: string; project_key: string; name: string; key: string; owner_user_id: string; description: string }
  ci_pipeline_id: string; cd_pipeline_id: string; params: OnboardingParameter[]; template_name: string
  change_approval_flow: boolean; approval_flow_id: string; save_app_sources: boolean; repo_url: string; branch: string
}
export interface OnboardingIssue { code: string; severity: string; step: string; field_path: string; message: string; remedy: string }
export interface OnboardingCheck { status: 'ready' | 'blocked' | 'unknown'; issues: OnboardingIssue[] }
export interface OnboardingSession {
  id: string; owner_user_id: string; mode: 'create_application' | 'complete_application'
  status: string; current_step: string; version: number; draft: OnboardingDraft
  refs: { project_id: string; application_id: string; ci_binding_id: string; cd_binding_id: string; template_id: string }
  check: OnboardingCheck; first_release_order_id: string; created_at: string; updated_at: string
}
export interface OnboardingField { key: string; name: string; type: string; builtin: boolean }
export interface OnboardingRow {
  id: string; scope: 'ci' | 'cd'; pipeline_id: string; name: string; description: string; type: string
  required: boolean; default_value: string; choices: string[]; mapped_key: string; suggested_key: string
  candidates: string[]; new_key: string; problem: string; runtime: boolean
}
export interface OnboardingInspection { rows: OnboardingRow[]; fields: OnboardingField[]; issues: OnboardingIssue[] }
export interface OnboardingStatus { sessions: OnboardingSession[]; jenkins_enabled: boolean; pipeline_count: number; env_options: string[] }
export interface ApplicationSetupStatus {
  application_id: string; status: OnboardingCheck['status']
  templates: { template_id: string; template_name: string; check: OnboardingCheck }[]
}
