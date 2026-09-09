import type { OnboardingField, OnboardingParameter, OnboardingRow } from '../types/onboarding'

export const onboardingSteps = [
  { key: 'preflight', title: '基础检查', help: '确认已有执行端可用。这里不创建或执行远端管线。' },
  { key: 'identity', title: '项目与应用', help: '选择已有项目或在此创建。应用用于组织发布，不需要填写应用访问地址。' },
  { key: 'pipelines', title: '绑定管线', help: '选择实际使用的 CI/CD 管线。保存后读取参数，不触发构建。' },
  { key: 'parameters', title: '参数与标准字段', help: '先看管线需要什么，再复用或就地补建标准字段，无需提前规划字库。' },
  { key: 'template_flow', title: '模板与流程', help: '使用前面确认的配置生成模板；审批属于应用，已有流程不会被默认清空。' },
  { key: 'review', title: '完成检查', help: '核对每个配置环节。配置完成不代表管线已经执行成功。' },
  { key: 'first_release', title: '开始使用', help: '创建该应用的首个发布单，或继续接入下一个应用。创建不自动执行。' },
]
export function newOnboardingParameter(row: OnboardingRow, fields: OnboardingField[]): OnboardingParameter {
  const key = row.mapped_key || row.suggested_key
  const field = fields.find(item => item.key === key)
  const source = field?.builtin ? 'builtin' : 'release_input'
  return { scope: row.scope, name: row.name, param_key: key || row.new_key, new_field_name: key ? '' : row.name, value_source: source, source_param_key: source === 'builtin' ? key : '', fixed_value: '', omit: false }
}
export function onboardingSourceLabel(param: OnboardingParameter, fields: OnboardingField[]): string {
  if (param.omit) return '使用管线默认值（不传递）'
  if (param.value_source === 'fixed') return '固定值（发布时无需填写）'
  if (param.value_source === 'ci_param') return `沿用 CI：${param.source_param_key || '尚未选择'}`
  if (param.value_source === 'builtin') return `自动取值：${fields.find(item => item.key === param.source_param_key)?.name || param.source_param_key || '尚未选择'}`
  return '发布时填写'
}
