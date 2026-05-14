export type NotificationSourceType = 'dingtalk' | 'wecom' | 'feishu'
export type NotificationConditionOperator =
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  | 'is_empty'
  | 'not_empty'

export interface NotificationSource {
  id: string
  name: string
  source_type: NotificationSourceType
  webhook_url: string
  has_verification_param: boolean
  enabled: boolean
  remark: string
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}

export interface NotificationSourcePayload {
  name: string
  source_type: NotificationSourceType
  webhook_url: string
  verification_param?: string
  enabled: boolean
  remark?: string
}

export interface NotificationSourceWebhookTestPayload extends NotificationSourcePayload {
  source_id?: string
}

export interface NotificationSourceWebhookTestResponse {
  success: boolean
  message: string
  status_code: number
}

export interface NotificationSourceListParams {
  keyword?: string
  source_type?: NotificationSourceType
  enabled?: boolean
  page?: number
  page_size?: number
}

export interface NotificationSourceListResponse {
  data: NotificationSource[]
  page: number
  page_size: number
  total: number
}

export interface NotificationSourceDataResponse {
  data: NotificationSource
}

export interface NotificationMarkdownTemplateCondition {
  param_key: string
  operator: NotificationConditionOperator
  expected_value: string
  markdown_text: string
  sort_no: number
}

export interface NotificationMarkdownTemplate {
  id: string
  name: string
  title_template: string
  body_template: string
  conditions: NotificationMarkdownTemplateCondition[]
  enabled: boolean
  remark: string
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}

export interface NotificationMarkdownTemplateConditionPayload {
  param_key: string
  operator: NotificationConditionOperator
  expected_value?: string
  markdown_text: string
}

export interface NotificationMarkdownTemplatePayload {
  name: string
  title_template?: string
  body_template?: string
  conditions?: NotificationMarkdownTemplateConditionPayload[]
  enabled: boolean
  remark?: string
}

export interface NotificationMarkdownTemplateListParams {
  keyword?: string
  enabled?: boolean
  page?: number
  page_size?: number
}

export interface NotificationMarkdownTemplateListResponse {
  data: NotificationMarkdownTemplate[]
  page: number
  page_size: number
  total: number
}

export interface NotificationMarkdownTemplateDataResponse {
  data: NotificationMarkdownTemplate
}

export interface NotificationHook {
  id: string
  name: string
  source_id: string
  source_name: string
  source_type: NotificationSourceType
  markdown_template_id: string
  markdown_template_name: string
  enabled: boolean
  remark: string
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}

export interface NotificationHookPayload {
  name: string
  source_id: string
  markdown_template_id: string
  enabled: boolean
  remark?: string
}

export interface NotificationHookListParams {
  keyword?: string
  enabled?: boolean
  page?: number
  page_size?: number
}

export interface NotificationHookListResponse {
  data: NotificationHook[]
  page: number
  page_size: number
  total: number
}

export interface NotificationHookDataResponse {
  data: NotificationHook
}
