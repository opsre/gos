<script setup lang="ts">
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CopyOutlined,
  DeploymentUnitOutlined,
  ExclamationCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  RocketOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { listApplications } from '../../api/application'
import { listAllAgentTasks } from '../../api/agent'
import { checkGitOpsScanPath, listGitOpsFieldCandidates, listGitOpsValuesCandidates } from '../../api/gitops'
import { listNotificationHooks } from '../../api/notification'
import { listPlatformParamDicts } from '../../api/platform-param'
import { getPipelineBindingByID, listPipelineBindings, listApplicationExecutorParamDefs } from '../../api/pipeline'
import { getPipelineScanResult, listPipelineScanRules } from '../../api/pipeline-scan'
import { getReleaseSettings } from '../../api/system'
import { listUserOptions } from '../../api/user'
import {
  createReleaseTemplate,
  deleteReleaseTemplate,
  getReleaseTemplateByID,
  listReleaseTemplates,
  updateReleaseTemplate,
} from '../../api/release'
import { useResizableColumns } from '../../composables/useResizableColumns'
import type { Application } from '../../types/application'
import type { AgentTask } from '../../types/agent'
import type { NotificationHook } from '../../types/notification'
import type { PipelineBinding, ExecutorParamDef } from '../../types/pipeline'
import type { PipelineScanRule } from '../../types/pipeline-scan'
import type { GitOpsFieldCandidate, GitOpsValuesCandidate } from '../../types/gitops'
import type { PlatformParamDict } from '../../types/platform-param'
import type { UserOption } from '../../types/user'
import type {
  ReleasePipelineScope,
  ReleaseTemplate,
  ReleaseTemplateBinding,
  ReleaseTemplateComplianceFinding,
  ReleaseTemplateHook,
  ReleaseTemplateHookExecuteStage,
  ReleaseTemplateHookPayload,
  ReleaseTemplateApprovalMode,
  ReleaseTemplateParamConfigPayload,
  ReleaseTemplateParamValueSource,
  ReleaseTemplateGitOpsRule,
  ReleaseTemplateGitOpsRulePayload,
  ReleaseTemplateGitOpsType,
  ReleaseTemplatePayload,
  ReleaseTemplateStatus,
  UpdateReleaseTemplatePayload,
} from '../../types/release'
import { extractHTTPErrorMessage } from '../../utils/http-error'
import { buildCloneName } from '../../utils/clone-name'
import { useAuthStore } from '../../stores/auth'

type FormMode = 'create' | 'edit' | 'clone'
type CDMode = 'pipeline' | 'argocd'

type ScopeState = {
  enabled: boolean
  binding_id: string
  selected_param_def_ids: string[]
  selectable_params: ExecutorParamDef[]
  loading_params: boolean
}

type TemplateParamConfigState = {
  value_source: ReleaseTemplateParamValueSource
  source_param_key: string
  fixed_value: string
}

interface TemplateFormState {
  id: string
  name: string
  application_id: string
  status: ReleaseTemplateStatus
  approval_enabled: boolean
  approval_mode: ReleaseTemplateApprovalMode
  remark: string
}

interface SelectOption {
  label: string
  value: string
}

interface BindingOption {
  label: string
  value: string
  binding_type: PipelineBinding['binding_type']
  provider: PipelineBinding['provider']
  pipeline_id: string
}

interface GitOpsRuleFormItem {
  local_id: string
  source_param_key: string
  source_from: 'ci' | 'builtin' | 'cd_input'
  file_path_template: string
  document_kind: string
  document_name: string
  target_path: string
  value_template: string
}

type ReleaseTemplateHookTypePreview = 'agent_task' | 'notification_hook' | 'webhook_notification'
type ReleaseTemplateHookExecuteStagePreview = ReleaseTemplateHookExecuteStage
type ReleaseTemplateHookTriggerConditionPreview = 'on_success' | 'on_failed' | 'always'
type ReleaseTemplateHookFailurePolicyPreview = 'block_release' | 'warn_only'

interface HookFormItem {
  local_id: string
  name: string
  hook_type: ReleaseTemplateHookTypePreview
  execute_stages: ReleaseTemplateHookExecuteStagePreview[]
  trigger_condition: ReleaseTemplateHookTriggerConditionPreview
  failure_policy: ReleaseTemplateHookFailurePolicyPreview
  env_codes: string[]
  target_id: string
  target_name: string
  webhook_method: 'POST' | 'PUT' | 'PATCH'
  webhook_url: string
  webhook_body_template: string
  note: string
}

type TemplateExecutionUnitKey = 'ci' | 'cd'

interface TemplateExecutionUnit {
  key: TemplateExecutionUnitKey
  label: string
}

const builtinTemplateSourceKeys = new Set([
  'app_key',
  'app_name',
  'repo_url',
  'release_name',
  'env',
  'env_code',
  'branch',
  'git_ref',
  'image_version',
  'image_tag',
  'ci_job',
  'ci_build',
  'gos_artifact_path',
])

const loading = ref(false)
const submitting = ref(false)
const deletingID = ref('')
const dataSource = ref<ReleaseTemplate[]>([])
const total = ref(0)
const complianceDetailVisible = ref(false)
const complianceDetailTemplate = ref<ReleaseTemplate | null>(null)

const modalVisible = ref(false)
const modalMode = ref<FormMode>('create')
const modalLoading = ref(false)
const formRef = ref<FormInstance>()
const formState = reactive<TemplateFormState>({
  id: '',
  name: '',
  application_id: '',
  status: 'active',
  approval_enabled: false,
  approval_mode: 'any',
  remark: '',
})
const approvalApproverIDs = ref<string[]>([])
const userOptions = ref<UserOption[]>([])

const scopeStates = reactive<Record<ReleasePipelineScope, ScopeState>>({
  ci: {
    enabled: true,
    binding_id: '',
    selected_param_def_ids: [],
    selectable_params: [],
    loading_params: false,
  },
  cd: {
    enabled: false,
    binding_id: '',
    selected_param_def_ids: [],
    selectable_params: [],
    loading_params: false,
  },
})

const scopeParamConfigs = reactive<Record<ReleasePipelineScope, Record<string, TemplateParamConfigState>>>({
  ci: {},
  cd: {},
})

const filters = reactive({
  application_id: '',
  status: '' as '' | ReleaseTemplateStatus,
  page: 1,
  pageSize: 20,
})

const applicationRecords = ref<Application[]>([])
const applicationOptions = ref<SelectOption[]>([])
const bindingOptions = ref<BindingOption[]>([])
const loadedTemplateBindings = ref<ReleaseTemplateBinding[]>([])
const scopeBindingWarnings = reactive<Record<ReleasePipelineScope, string>>({
  ci: '',
  cd: '',
})
const scopeComplianceWarnings = reactive<Record<ReleasePipelineScope, string>>({
  ci: '',
  cd: '',
})
const templateComplianceFindings = ref<ReleaseTemplateComplianceFinding[]>([])
const templateValidationRuleCache = ref<PipelineScanRule[] | null>(null)
const pipelineComplianceWarningCache = ref<Record<string, string>>({})
const templateBindingWarnings = ref<Record<string, string>>({})
const templateBindingWarningCache = ref<Record<string, string>>({})
const loadingBindings = ref(false)
const platformParamNameMap = ref<Record<string, string>>({})
const platformParamDicts = ref<PlatformParamDict[]>([])
const gitOpsFieldCandidates = ref<GitOpsFieldCandidate[]>([])
const gitOpsValuesCandidates = ref<GitOpsValuesCandidate[]>([])
const loadingGitOpsFieldCandidates = ref(false)
const gitopsRules = ref<GitOpsRuleFormItem[]>([])
const gitopsRulesByType = ref<Record<string, GitOpsRuleFormItem[]>>({ helm: [], kustomize: [] })
const gitOpsType = ref<ReleaseTemplateGitOpsType>('kustomize')
const cdMode = ref<CDMode>('argocd')
const argocdInfoActiveKeys = ref<string[]>([])
const gitopsRuleActiveKeys = ref<string[]>([])
const gitopsScanPathExists = ref<boolean | null>(null)
const gitopsScanPathTemplate = ref('')
const templateHooks = ref<HookFormItem[]>([])
const hookTypePickerVisible = ref(false)
const pendingHookType = ref<ReleaseTemplateHookTypePreview>('agent_task')
const agentTaskTemplates = ref<AgentTask[]>([])
const loadingAgentTaskTemplates = ref(false)
const notificationHooks = ref<NotificationHook[]>([])
const loadingNotificationHooks = ref(false)
const hookEnvOptions = ref<SelectOption[]>([])
const loadingHookEnvOptions = ref(false)
const bindingOptionsCache = ref<Record<string, BindingOption[]>>({})
const selectableParamsCache = ref<Record<string, ExecutorParamDef[]>>({})
const gitOpsFieldCandidateCache = ref<Record<string, GitOpsFieldCandidate[]>>({})
const gitOpsValuesCandidateCache = ref<Record<string, GitOpsValuesCandidate[]>>({})
const bindingLookupCache = ref<Record<string, boolean>>({})
const authStore = useAuthStore()
const route = useRoute()
const canManageTemplate = computed(() => authStore.hasPermission('release.template.manage'))
const templateEditorViewportInset = ref(0)
const templateEditorMaskStyle = computed(() => ({
  left: `${templateEditorViewportInset.value}px`,
  width: `calc(100% - ${templateEditorViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalVisible.value ? 'auto' : 'none',
}))
const templateEditorWrapProps = computed(() => ({
  style: {
    left: `${templateEditorViewportInset.value}px`,
    width: `calc(100% - ${templateEditorViewportInset.value}px)`,
    pointerEvents: modalVisible.value ? 'auto' : 'none',
  },
}))
let templateEditorViewportObserver: ResizeObserver | null = null

const scopeTitles: Record<ReleasePipelineScope, string> = {
  ci: 'CI 配置',
  cd: 'CD 配置',
}

const scopeDescriptions: Record<ReleasePipelineScope, string> = {
  ci: 'CI 固定使用 Jenkins；参数必须已完成平台标准 Key 映射，并可继续映射发布基础字段、固定值或发布时填写',
  cd: '先明确当前模板的 CD 方式：选择管线时可配置发布时填写、固定值、沿用 CI 字段或发布基础字段；选择 ArgoCD 时只配置 GitOps / ArgoCD 相关内容',
}

const hookVariableSourceTags = ['固定值', '标准字段', '内置字段']
const hookExecuteStageOptions = [
  { label: '发布完成时', value: 'post_release' },
  { label: '构建完成时', value: 'build_complete' },
] satisfies Array<{ label: string; value: ReleaseTemplateHookExecuteStagePreview }>

const initialColumns: TableColumnsType<ReleaseTemplate> = [
  { title: '模板名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '执行单元', dataIndex: 'binding_name', key: 'binding_name', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '操作', key: 'actions', width: 240, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, { minWidth: 100, maxWidth: 560, hitArea: 10 })

const selectableParamColumns: TableColumnsType<ExecutorParamDef> = [
  { title: '平台字段', key: 'param_name', width: 180 },
  { title: '平台 Key', dataIndex: 'param_key', key: 'param_key', width: 160 },
  { title: '执行器参数', dataIndex: 'executor_param_name', key: 'executor_param_name', width: 220 },
  { title: '类型', dataIndex: 'param_type', key: 'param_type', width: 120 },
  { title: '必填', dataIndex: 'required', key: 'required', width: 90 },
  { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
]

const statusOptions = [
  { label: 'active', value: 'active' },
  { label: 'inactive', value: 'inactive' },
] as const

const templateApplicationFilterOptions = computed<SelectOption[]>(() => applicationOptions.value)

const templateStatusFilterOptions = computed<SelectOption[]>(() => [...statusOptions])

const complianceDetailFindings = computed(() => complianceDetailTemplate.value?.compliance_findings || [])

const complianceDetailScopes = computed<ReleasePipelineScope[]>(() => {
  const scopes = new Set<ReleasePipelineScope>()
  complianceDetailFindings.value.forEach((item) => {
    if (item.pipeline_scope === 'ci' || item.pipeline_scope === 'cd') {
      scopes.add(item.pipeline_scope)
    }
  })
  return (['ci', 'cd'] as ReleasePipelineScope[]).filter((scope) => scopes.has(scope))
})

const applicationFilterValue = computed<string | undefined>({
  get: () => filters.application_id || undefined,
  set: (value) => {
    filters.application_id = String(value || '').trim()
  },
})

const statusFilterValue = computed<ReleaseTemplateStatus | undefined>({
  get: () => filters.status || undefined,
  set: (value) => {
    filters.status = (value || '') as '' | ReleaseTemplateStatus
  },
})

const approvalModeOptions = [
  { label: '任一人通过', value: 'any' },
  { label: '全部通过', value: 'all' },
] as const

const templateExecutionUnitLabels: Record<TemplateExecutionUnitKey, string> = {
  ci: 'CI',
  cd: 'CD',
}

const userOptionChoices = computed(() =>
  userOptions.value.map((item) => ({
    label: `${item.display_name || item.username} (${item.username})`,
    value: item.id,
  })),
)

function templateExecutionUnits(record: ReleaseTemplate) {
  const scopeKeys = new Set<TemplateExecutionUnitKey>()

  String(record.binding_type || '')
    .split(/[^a-z]+/i)
    .map((item) => item.trim().toLowerCase())
    .forEach((item) => {
      if (item === 'ci' || item === 'cd') {
        scopeKeys.add(item)
      }
    })

  if (!scopeKeys.size) {
    const fallback = String(record.binding_name || '').trim().toUpperCase()
    if (fallback.includes('CI')) {
      scopeKeys.add('ci')
    }
    if (fallback.includes('CD')) {
      scopeKeys.add('cd')
    }
  }

  return (['ci', 'cd'] as TemplateExecutionUnitKey[])
    .filter((key) => scopeKeys.has(key))
    .map((key) => ({
      key,
      label: templateExecutionUnitLabels[key],
    })) satisfies TemplateExecutionUnit[]
}

function templateExecutionSummary(record: ReleaseTemplate) {
  return String(record.binding_name || '').trim() || '-'
}

function hookTypeLabel(type: ReleaseTemplateHookTypePreview) {
  switch (type) {
    case 'agent_task':
      return 'Agent 任务'
    case 'notification_hook':
      return '通知 Hook'
    default:
      return 'Webhook 通知'
  }
}

function createHookFormItem(type: ReleaseTemplateHookTypePreview): HookFormItem {
  return {
    local_id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    name:
      type === 'agent_task'
        ? 'Agent 任务 Hook'
        : type === 'notification_hook'
          ? '通知 Hook'
          : 'Webhook 通知 Hook',
    hook_type: type,
    execute_stages: ['post_release'],
    trigger_condition: 'on_success',
    failure_policy: type === 'webhook_notification' ? 'warn_only' : 'block_release',
    env_codes: [],
    target_id: '',
    target_name: '',
    webhook_method: 'POST',
    webhook_url: '',
    webhook_body_template: `{
  "order_no": "{order_no}",
  "env": "{env}"
}`,
    note: '',
  }
}

function createHookFormItemFromResponse(item: ReleaseTemplateHook): HookFormItem {
  return {
    local_id: item.id || `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    name: item.name,
    hook_type: item.hook_type,
    execute_stages: normalizeHookExecuteStages(item.execute_stages, item.execute_stage),
    trigger_condition: item.trigger_condition,
    failure_policy: item.failure_policy,
    env_codes: normalizeHookEnvCodes(item.env_codes || []),
    target_id: item.target_id || '',
    target_name: item.target_name || '',
    webhook_method: ((item.webhook_method || 'POST').toUpperCase() as 'POST' | 'PUT' | 'PATCH'),
    webhook_url: item.webhook_url || '',
    webhook_body_template: item.webhook_body || '',
    note: item.note || '',
  }
}

function normalizeHookEnvCodes(values: string[]) {
  const result: string[] = []
  const seen = new Set<string>()
  values.forEach((item) => {
    const value = String(item || '').trim()
    if (!value) {
      return
    }
    const key = value.toLowerCase()
    if (seen.has(key)) {
      return
    }
    seen.add(key)
    result.push(value)
  })
  return result
}

function normalizeHookExecuteStages(
  values: Array<ReleaseTemplateHookExecuteStagePreview | string> = [],
  legacy?: ReleaseTemplateHookExecuteStagePreview | string,
) {
  const normalized: ReleaseTemplateHookExecuteStagePreview[] = []
  const seen = new Set<string>()
  ;[...values, legacy || ''].forEach((item) => {
    const value = item === 'build_complete' ? 'build_complete' : item === 'post_release' ? 'post_release' : ''
    if (!value || seen.has(value)) {
      return
    }
    seen.add(value)
    normalized.push(value)
  })
  return normalized.length ? normalized : ['post_release']
}

function hookHasBuildStage(stages: Array<ReleaseTemplateHookExecuteStagePreview | string>) {
  return normalizeHookExecuteStages(stages).includes('build_complete')
}

function mergeHookEnvOptions(extraValues: string[] = []) {
  const merged = new Map<string, SelectOption>()
  ;[...hookEnvOptions.value.map((item) => item.value), ...extraValues].forEach((item) => {
    const value = String(item || '').trim()
    if (!value) {
      return
    }
    const key = value.toLowerCase()
    if (!merged.has(key)) {
      merged.set(key, {
        label: value.toUpperCase(),
        value,
      })
    }
  })
  return [...merged.values()]
}

function hookEnvLabel(envCodes: string[]) {
  const normalized = normalizeHookEnvCodes(envCodes)
  if (!normalized.length) {
    return '全部环境'
  }
  return normalized.join(' / ')
}

const hookSummaryItems = computed(() => [
  {
    label: 'Hook 阶段',
    value: templateHooks.value.length
      ? Array.from(new Set(templateHooks.value.flatMap((item) => hookStageLabels(item.execute_stages)))).join(' / ')
      : '待配置',
  },
  { label: '执行方式', value: templateHooks.value.length ? '串行执行' : '待配置' },
  { label: 'Hook 数量', value: `${templateHooks.value.length} 个` },
  { label: '变量', value: '标准平台 Key / 内置字段' },
])

function agentTaskTypeLabel(taskType: string) {
  switch (taskType) {
    case 'script_file_task':
      return '脚本文件任务'
    case 'file_distribution_task':
      return '文件下发任务'
    default:
      return 'Shell 脚本'
  }
}

function isAgentTaskTemplateCandidate(item: AgentTask) {
  return item.task_mode === 'temporary' && !String(item.source_task_id || '').trim()
}

const agentTaskTemplateOptions = computed<SelectOption[]>(() => {
  const temps = agentTaskTemplates.value.filter(isAgentTaskTemplateCandidate)
  const nameKeyCounts = new Map<string, number>()
  temps.forEach((item) => {
    const key = String(item.name || '')
      .trim()
      .toLowerCase() || item.id
    nameKeyCounts.set(key, (nameKeyCounts.get(key) || 0) + 1)
  })
  return temps.map((item) => {
    const baseName = String(item.name || '').trim() || item.id
    const key = baseName.toLowerCase() || item.id
    const duplicate = (nameKeyCounts.get(key) || 0) > 1
    const shortId = String(item.id || '').slice(0, 8)
    return {
      label: duplicate && shortId ? `${baseName}（${shortId}）` : baseName,
      value: item.id,
    }
  })
})

const notificationHookOptions = computed<SelectOption[]>(() =>
  notificationHooks.value
    .filter((item) => item.enabled)
    .map((item) => ({
      label: `${item.name} · ${item.source_name} / ${item.markdown_template_name}`,
      value: item.id,
    })),
)

function findAgentTaskTemplate(taskID: string) {
  const normalized = String(taskID || '').trim()
  if (!normalized) {
    return null
  }
  return agentTaskTemplates.value.find((item) => item.id === normalized) || null
}

function summarizeAgentTaskScript(taskID: string) {
  const task = findAgentTaskTemplate(taskID)
  const content = String(task?.script_text || '')
    .replace(/\r\n/g, '\n')
    .trim()
  if (!content) {
    return '暂无脚本内容'
  }
  const lines = content.split('\n')
  const preview = lines.slice(0, 10).join('\n')
  const truncatedByLines = lines.length > 10
  const truncatedByLength = preview.length > 800
  const clipped = truncatedByLength ? `${preview.slice(0, 800).trimEnd()}\n...` : preview
  return truncatedByLines && !truncatedByLength ? `${clipped}\n...` : clipped
}

function syncHookTargetName(item: HookFormItem) {
  if (item.hook_type === 'agent_task') {
    const selected = findAgentTaskTemplate(item.target_id)
    item.target_name = selected?.name || ''
    return
  }
  if (item.hook_type === 'notification_hook') {
    const selected = notificationHooks.value.find((candidate) => candidate.id === item.target_id)
    item.target_name = selected?.name || ''
    return
  }
  if (item.hook_type !== 'webhook_notification') {
    item.target_name = ''
    return
  }
  item.target_name = item.webhook_url.trim()
}

function openHookTypePicker() {
  pendingHookType.value = 'agent_task'
  hookTypePickerVisible.value = true
}

function confirmAddHook() {
  templateHooks.value.push(createHookFormItem(pendingHookType.value))
  hookTypePickerVisible.value = false
}

function removeHook(localID: string) {
  templateHooks.value = templateHooks.value.filter((item) => item.local_id !== localID)
}

function moveHook(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (index < 0 || targetIndex < 0 || index >= templateHooks.value.length || targetIndex >= templateHooks.value.length) {
    return
  }
  const next = [...templateHooks.value]
  const [item] = next.splice(index, 1)
  if (!item) {
    return
  }
  next.splice(targetIndex, 0, item)
  templateHooks.value = next
}

function hookStageLabel(type: ReleaseTemplateHookExecuteStagePreview) {
  return type === 'build_complete' ? '构建完成时' : '发布完成时'
}

function hookStageLabels(stages: Array<ReleaseTemplateHookExecuteStagePreview | string>) {
  return normalizeHookExecuteStages(stages).map((item) => hookStageLabel(item))
}

function hookTriggerLabel(
  type: ReleaseTemplateHookTriggerConditionPreview,
  stages: Array<ReleaseTemplateHookExecuteStagePreview | string> = ['post_release'],
) {
  const normalizedStages = normalizeHookExecuteStages(stages)
  if (normalizedStages.length === 1 && normalizedStages[0] === 'build_complete') {
    return '构建成功后'
  }
  switch (type) {
    case 'on_success':
      return hookHasBuildStage(normalizedStages) ? '成功后触发' : '主流程成功后'
    case 'on_failed':
      return hookHasBuildStage(normalizedStages) ? '失败后触发' : '主流程失败后'
    default:
      return hookHasBuildStage(normalizedStages) ? '始终触发' : '始终触发'
  }
}

function hookFailureLabel(type: ReleaseTemplateHookFailurePolicyPreview) {
  return type === 'block_release' ? '失败阻断发布单' : '失败仅告警'
}

function hookTriggerOptions(item: HookFormItem) {
  const normalizedStages = normalizeHookExecuteStages(item.execute_stages)
  if (normalizedStages.length === 1 && normalizedStages[0] === 'build_complete') {
    return [{ label: '构建成功后', value: 'on_success' }]
  }
  return [
    { label: '仅成功后', value: 'on_success' },
    { label: '仅失败后', value: 'on_failed' },
    { label: '始终触发', value: 'always' },
  ]
}

function handleHookStageChange(item: HookFormItem, values: string[]) {
  item.execute_stages = normalizeHookExecuteStages(values)
  if (item.execute_stages.length === 1 && item.execute_stages[0] === 'build_complete') {
    item.trigger_condition = 'on_success'
    return
  }
  const validValues = hookTriggerOptions(item).map((option) => option.value)
  if (!validValues.includes(item.trigger_condition)) {
    item.trigger_condition = 'on_success'
  }
}

async function loadHookEnvOptions(extraValues: string[] = [], silent = false) {
  const extra = normalizeHookEnvCodes(extraValues)
  if (hookEnvOptions.value.length > 0 && extra.every((item) => hookEnvOptions.value.some((option) => option.value.toLowerCase() === item.toLowerCase()))) {
    return
  }
  loadingHookEnvOptions.value = true
  try {
    const response = await getReleaseSettings()
    hookEnvOptions.value = mergeHookEnvOptions([...(response.data.env_options || []), ...extra])
  } catch (error) {
    hookEnvOptions.value = mergeHookEnvOptions(extra)
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, '发布环境选项加载失败'))
    }
  } finally {
    loadingHookEnvOptions.value = false
  }
}

const modalTitle = computed(() => {
  if (modalMode.value === 'edit') {
    return '编辑发布模板'
  }
  if (modalMode.value === 'clone') {
    return '克隆发布模板'
  }
  return '新增发布模板'
})

const bindingOptionsByScope = computed<Record<ReleasePipelineScope, BindingOption[]>>(() => ({
  ci: bindingOptions.value.filter((item) => item.binding_type === 'ci' && item.provider === 'jenkins'),
  cd: bindingOptions.value.filter((item) => item.binding_type === 'cd' && item.provider === 'jenkins'),
}))

const gitOpsSourceOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = []
  const seen = new Set<string>()
  const selectedCIParamIDs = new Set(scopeStates.ci.selected_param_def_ids)

  scopeStates.ci.selectable_params.forEach((item) => {
    if (!selectedCIParamIDs.has(item.id)) {
      return
    }
    const key = String(item.param_key || '').trim().toLowerCase()
    if (!key || seen.has(key)) {
      return
    }
    seen.add(key)
    options.push({
      label: `${resolvePlatformParamName(key)} (${key}) · 来自 CI`,
      value: key,
    })
  })

  platformParamDicts.value.forEach((item) => {
    const key = String(item.param_key || '').trim().toLowerCase()
    if (!item.builtin || item.status !== 1 || !key || seen.has(key)) {
      return
    }
    seen.add(key)
    options.push({
      label: `${item.name} (${key}) · 系统内置`,
      value: key,
    })
  })

  platformParamDicts.value.forEach((item) => {
    const key = String(item.param_key || '').trim().toLowerCase()
    if (!item.cd_self_fill || item.status !== 1 || !key || seen.has(key)) {
      return
    }
    seen.add(key)
    options.push({
      label: `${item.name} (${key}) · CD 自填字段`,
      value: key,
    })
  })

  return options
})

const ciTemplateSourceOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = []
  const selectedCIParamIDs = new Set(scopeStates.ci.selected_param_def_ids)
  scopeStates.ci.selectable_params.forEach((item) => {
    if (!selectedCIParamIDs.has(item.id)) {
      return
    }
    const key = String(item.param_key || '').trim().toLowerCase()
    if (!key) {
      return
    }
    options.push({
      label: `${resolvePlatformParamName(key)} (${key})`,
      value: key,
    })
  })
  return options
})

const builtinTemplateSourceOptions = computed<SelectOption[]>(() =>
  platformParamDicts.value
    .filter((item) => {
      const key = String(item.param_key || '').trim().toLowerCase()
      return item.status === 1 && (item.builtin || builtinTemplateSourceKeys.has(key))
    })
    .map((item) => ({
      label: `${item.name} (${item.param_key})`,
      value: String(item.param_key || '').trim().toLowerCase(),
    })),
)

const builtinPlatformParamKeySet = computed(() => {
  const keys = new Set<string>()
  platformParamDicts.value.forEach((item) => {
    const key = String(item.param_key || '').trim().toLowerCase()
    if (item.status === 1 && item.builtin && key) {
      keys.add(key)
    }
  })
  return keys
})

function selectedBinding(scope: ReleasePipelineScope) {
  return bindingOptionsByScope.value[scope].find((item) => item.value === scopeStates[scope].binding_id)
}

function isTemplateViolated(record: ReleaseTemplate) {
  return record.compliance_status === 'violated'
}

function templateComplianceWarning(record: ReleaseTemplate) {
  return isTemplateViolated(record)
    ? (record.compliance_summary || '发布模板违反已启用的管线规范')
    : ''
}

function templateComplianceFindingsByScope(scope: ReleasePipelineScope) {
  return templateComplianceFindings.value.filter((item) => item.pipeline_scope === scope)
}

function formatComplianceFinding(item: ReleaseTemplateComplianceFinding) {
  const ruleName = String(item.rule_name || item.rule_code || '管线规范').trim()
  const pipelineName = String(item.pipeline_name || item.pipeline_id || '').trim()
  return pipelineName ? `${pipelineName} · ${ruleName}` : ruleName
}

function openTemplateComplianceDetail(record: ReleaseTemplate) {
  complianceDetailTemplate.value = record
  complianceDetailVisible.value = true
}

function closeTemplateComplianceDetail() {
  complianceDetailVisible.value = false
  complianceDetailTemplate.value = null
}

function complianceDetailFindingsByScope(scope: ReleasePipelineScope) {
  return complianceDetailFindings.value.filter((item) => item.pipeline_scope === scope)
}

function complianceScopeText(scope: ReleasePipelineScope) {
  return scope === 'ci' ? 'CI 管线' : 'CD 管线'
}

function complianceSeverityColor(severity: string) {
  const value = String(severity || '').trim().toLowerCase()
  if (value === 'error') {
    return 'red'
  }
  if (value === 'warning') {
    return 'orange'
  }
  return 'blue'
}

function complianceSeverityText(severity: string) {
  const value = String(severity || '').trim().toLowerCase()
  if (value === 'error') {
    return '错误'
  }
  if (value === 'warning') {
    return '警告'
  }
  if (value === 'info') {
    return '提示'
  }
  return severity || '提示'
}

function complianceFindingLineText(item: ReleaseTemplateComplianceFinding) {
  const lineNo = Number(item.line_no || 0)
  return lineNo > 0 ? `第 ${lineNo} 行` : '未定位到行号'
}

async function loadTemplateValidationRules() {
  if (templateValidationRuleCache.value) {
    return templateValidationRuleCache.value
  }
  try {
    const response = await listPipelineScanRules({ enabled: true, page: 1, page_size: 100 })
    templateValidationRuleCache.value = response.data || []
  } catch {
    templateValidationRuleCache.value = []
  }
  return templateValidationRuleCache.value
}

async function refreshScopeComplianceWarning(scope: ReleasePipelineScope) {
  scopeComplianceWarnings[scope] = ''
  if (!scopeStates[scope].enabled || (scope === 'cd' && !isCDUsingPipeline())) {
    return
  }
  const binding = selectedBinding(scope)
  const pipelineID = String(binding?.pipeline_id || '').trim()
  if (!pipelineID) {
    return
  }
  const cacheKey = `${scope}:${pipelineID}`
  if (pipelineComplianceWarningCache.value[cacheKey] !== undefined) {
    scopeComplianceWarnings[scope] = pipelineComplianceWarningCache.value[cacheKey] || ''
    return
  }
  const rules = await loadTemplateValidationRules()
  const scopedRules = rules.filter((item) => item.enabled && (item.template_validation_scopes || []).includes(scope))
  if (!scopedRules.length) {
    pipelineComplianceWarningCache.value[cacheKey] = ''
    return
  }
  const ruleIDs = new Set(scopedRules.map((item) => String(item.id || '').trim()).filter(Boolean))
  const ruleCodes = new Set(scopedRules.map((item) => String(item.rule_code || '').trim()).filter(Boolean))
  try {
    const response = await getPipelineScanResult(pipelineID)
    const finding = (response.data.findings || []).find((item) => {
      if (item.status && item.status !== 'open') {
        return false
      }
      return ruleIDs.has(String(item.rule_id || '').trim()) || ruleCodes.has(String(item.rule_code || '').trim())
    })
    const warning = finding
      ? `${scope.toUpperCase()} 管线违反发布模板校验：${finding.rule_name || finding.rule_code || '管线规范'}`
      : ''
    pipelineComplianceWarningCache.value[cacheKey] = warning
    scopeComplianceWarnings[scope] = warning
  } catch {
    pipelineComplianceWarningCache.value[cacheKey] = ''
  }
}

function isCDUsingArgoCD() {
  return scopeStates.cd.enabled && cdMode.value === 'argocd'
}

function isCDUsingPipeline() {
  return scopeStates.cd.enabled && cdMode.value === 'pipeline'
}

function defaultTemplateParamValueSource(scope: ReleasePipelineScope): ReleaseTemplateParamValueSource {
  return 'release_input'
}

function isBuiltinMappedExecutorParam(item?: ExecutorParamDef | null) {
  const paramKey = String(item?.param_key || '').trim().toLowerCase()
  return Boolean(paramKey && builtinPlatformParamKeySet.value.has(paramKey))
}

function resolveBuiltinMappedExecutorParamLabel(item?: ExecutorParamDef | null) {
  const paramKey = String(item?.param_key || '').trim().toLowerCase()
  if (!paramKey) {
    return '-'
  }
  return `${resolvePlatformParamName(paramKey)} (${paramKey})`
}

function enforceBuiltinMappedTemplateParamConfig(config: TemplateParamConfigState, item?: ExecutorParamDef | null) {
  if (!isBuiltinMappedExecutorParam(item)) {
    return config
  }
  const paramKey = String(item?.param_key || '').trim().toLowerCase()
  config.value_source = 'builtin'
  config.source_param_key = paramKey
  config.fixed_value = ''
  return config
}

function createTemplateParamConfigState(
  scope: ReleasePipelineScope,
  partial?: Partial<TemplateParamConfigState>,
  item?: ExecutorParamDef | null,
): TemplateParamConfigState {
  const config = {
    value_source: partial?.value_source || defaultTemplateParamValueSource(scope),
    source_param_key: String(partial?.source_param_key || '').trim().toLowerCase(),
    fixed_value: String(partial?.fixed_value || ''),
  }
  return enforceBuiltinMappedTemplateParamConfig(config, item)
}

function selectedScopeParamDefs(scope: ReleasePipelineScope) {
  const selected = new Set(scopeStates[scope].selected_param_def_ids)
  return scopeStates[scope].selectable_params.filter((item) => selected.has(item.id))
}

function syncScopeParamConfigs(scope: ReleasePipelineScope) {
  const state = scopeStates[scope]
  const paramByID = new Map(state.selectable_params.map((item) => [item.id, item]))
  const nextConfigs: Record<string, TemplateParamConfigState> = {}
  state.selected_param_def_ids.forEach((id) => {
    nextConfigs[id] = createTemplateParamConfigState(scope, scopeParamConfigs[scope][id], paramByID.get(id) || null)
  })
  scopeParamConfigs[scope] = nextConfigs
}

function getTemplateParamConfig(scope: ReleasePipelineScope, paramDefID: string, item?: ExecutorParamDef | null) {
  if (!scopeParamConfigs[scope][paramDefID]) {
    scopeParamConfigs[scope][paramDefID] = createTemplateParamConfigState(scope, undefined, item)
  }
  return enforceBuiltinMappedTemplateParamConfig(scopeParamConfigs[scope][paramDefID], item)
}

function handleTemplateParamValueSourceChange(
  scope: ReleasePipelineScope,
  paramDefID: string,
  value: ReleaseTemplateParamValueSource,
) {
  const paramDef = scopeStates[scope].selectable_params.find((item) => item.id === paramDefID) || null
  const config = getTemplateParamConfig(scope, paramDefID, paramDef)
  if (isBuiltinMappedExecutorParam(paramDef)) {
    enforceBuiltinMappedTemplateParamConfig(config, paramDef)
    return
  }
  config.value_source = value
  if (value !== 'fixed') {
    config.fixed_value = ''
  }
  if (value !== 'ci_param' && value !== 'builtin') {
    config.source_param_key = ''
  }
}

function resolveTemplateParamSourceOptions(scope: ReleasePipelineScope, config: TemplateParamConfigState) {
  if (scope === 'ci' && config.value_source === 'builtin') {
    return builtinTemplateSourceOptions.value
  }
  if (scope === 'cd' && config.value_source === 'ci_param') {
    return ciTemplateSourceOptions.value
  }
  if (scope === 'cd' && config.value_source === 'builtin') {
    return builtinTemplateSourceOptions.value
  }
  return []
}

function resolveTemplateParamSourceLabel(scope: ReleasePipelineScope, config: TemplateParamConfigState) {
  if (scope === 'ci') {
    switch (config.value_source) {
      case 'fixed':
        return '固定值'
      case 'builtin':
        return '发布基础字段'
      default:
        return '发布时填写'
    }
  }
  switch (config.value_source) {
    case 'fixed':
      return '固定值'
    case 'ci_param':
      return '沿用 CI 标准字段'
    case 'builtin':
      return '发布基础字段'
    default:
      return '发布时填写'
  }
}

function buildTemplateParamConfigs(scope: ReleasePipelineScope): ReleaseTemplateParamConfigPayload[] {
  const paramByID = new Map(scopeStates[scope].selectable_params.map((item) => [item.id, item]))
  return scopeStates[scope].selected_param_def_ids.map((id) => {
    const paramDef = paramByID.get(id) || null
    const config = getTemplateParamConfig(scope, id, paramDef)
    return {
      executor_param_def_id: id,
      value_source: config.value_source,
      source_param_key: config.source_param_key || undefined,
      fixed_value: config.fixed_value || undefined,
    }
  })
}

function normalizedGitOpsType(type?: string): ReleaseTemplateGitOpsType {
  const value = String(type || '').trim().toLowerCase()
  return value === 'helm' ? 'helm' : 'kustomize'
}

function isHelmGitOps() {
  return isCDUsingArgoCD() && normalizedGitOpsType(gitOpsType.value) === 'helm'
}

function isUnsupportedKustomizeGitOps() {
  return isCDUsingArgoCD() && normalizedGitOpsType(gitOpsType.value) === 'kustomize'
}

function statusColor(status: ReleaseTemplateStatus) {
  return status === 'active' ? 'green' : 'default'
}

function resolvePlatformParamName(paramKey: string) {
  const key = String(paramKey || '').trim().toLowerCase()
  return platformParamNameMap.value[key] || key || '-'
}

function createGitOpsRuleFormItem(partial?: Partial<GitOpsRuleFormItem>): GitOpsRuleFormItem {
  const sourceKey = String(partial?.source_param_key || '').trim().toLowerCase()
  return {
    local_id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    source_param_key: sourceKey,
    source_from: partial?.source_from || resolveGitOpsRuleSourceFrom(sourceKey),
    file_path_template: normalizeHelmValuesFilePathTemplate(String(partial?.file_path_template || '').trim()),
    document_kind: String(partial?.document_kind || '').trim(),
    document_name: String(partial?.document_name || '').trim(),
    target_path: String(partial?.target_path || '').trim(),
    value_template: String(
      partial?.value_template ||
        (sourceKey && partial?.source_from !== 'cd_input' && resolveGitOpsRuleSourceFrom(sourceKey) !== 'cd_input'
          ? `{${sourceKey}}`
          : ''),
    ).trim(),
  }
}

function resolveGitOpsRuleSourceFrom(paramKey: string): 'ci' | 'builtin' | 'cd_input' {
  const normalized = String(paramKey || '').trim().toLowerCase()
  if (!normalized) {
    return 'ci'
  }
  const manualItem = platformParamDicts.value.find(
    (item) => String(item.param_key || '').trim().toLowerCase() === normalized && item.cd_self_fill && item.status === 1,
  )
  if (manualItem) {
    return 'cd_input'
  }
  const selectedCIParamIDs = new Set(scopeStates.ci.selected_param_def_ids)
  const fromCI = scopeStates.ci.selectable_params.some(
    (item) => selectedCIParamIDs.has(item.id) && String(item.param_key || '').trim().toLowerCase() === normalized,
  )
  return fromCI ? 'ci' : 'builtin'
}

function normalizeHelmValuesFilePathTemplate(value: string) {
  const normalized = String(value || '').trim().replace(/\\/g, '/')
  if (!normalized.startsWith('apps/')) {
    return normalized
  }
  const rest = normalized.slice('apps/'.length)
  const parts = rest.split('/')
  if (parts.length < 3) {
    return normalized
  }
  if (parts[0] === 'helm') {
    return normalized
  }
  if (parts[1] === 'helm') {
    return ['apps', 'helm', ...parts.slice(2)].join('/')
  }
  return normalized
}

function yamlCandidatesForRule(rule: GitOpsRuleFormItem) {
  return gitOpsFieldCandidates.value
}

function pathBaseName(value: string) {
  const normalized = String(value || '').trim()
  if (!normalized) {
    return ''
  }
  const segments = normalized.split('/')
  return segments[segments.length - 1] || normalized
}

function isPlatformValuesFileTemplate(filePathTemplate: string) {
  const baseName = pathBaseName(filePathTemplate)
  return /^platform\.values(?:-[^.]+)?\.ya?ml$/i.test(baseName)
}

function platformValuesCandidates() {
  return gitOpsValuesCandidates.value.filter((item) => isPlatformValuesFileTemplate(item.file_path_template))
}

function yamlFileOptions(rule: GitOpsRuleFormItem): SelectOption[] {
  const seen = new Set<string>()
  return yamlCandidatesForRule(rule)
    .filter((item) => {
      const key = String(item.file_path_template || '').trim()
      if (!key || seen.has(key)) {
        return false
      }
      seen.add(key)
      return true
    })
    .map((item) => ({
      label: `${pathBaseName(item.file_path_template)} · ${item.file_path_template}`,
      value: item.file_path_template,
    }))
}

function yamlDocumentOptions(rule: GitOpsRuleFormItem): SelectOption[] {
  if (!rule.file_path_template) {
    return []
  }
  const seen = new Set<string>()
  return yamlCandidatesForRule(rule)
    .filter((item) => String(item.file_path_template || '').trim() === String(rule.file_path_template || '').trim())
    .filter((item) => {
      const key = `${item.document_kind}::${item.document_name || ''}`
      if (!item.document_kind || seen.has(key)) {
        return false
      }
      seen.add(key)
      return true
    })
    .map((item) => ({
      label: item.document_name ? `${item.document_kind} / ${item.document_name}` : item.document_kind,
      value: JSON.stringify({
        document_kind: item.document_kind,
        document_name: item.document_name || '',
      }),
    }))
}

function yamlFieldOptions(rule: GitOpsRuleFormItem): SelectOption[] {
  if (!rule.file_path_template || !rule.document_kind) {
    return []
  }
  return yamlCandidatesForRule(rule)
    .filter((item) =>
      String(item.file_path_template || '').trim() === String(rule.file_path_template || '').trim() &&
      String(item.document_kind || '').trim() === String(rule.document_kind || '').trim() &&
      String(item.document_name || '').trim() === String(rule.document_name || '').trim(),
    )
    .map((item) => ({
      label: `${item.target_path}${item.sample_value ? ` · 示例: ${item.sample_value}` : ''}`,
      value: item.target_path,
    }))
}

function valuesFileOptions(): SelectOption[] {
  const seen = new Set<string>()
  return platformValuesCandidates()
    .filter((item) => {
      const key = String(item.file_path_template || '').trim()
      if (!key || seen.has(key)) {
        return false
      }
      seen.add(key)
      return true
    })
    .map((item) => ({
      label: `${pathBaseName(item.file_path_template)} · ${item.file_path_template}`,
      value: item.file_path_template,
    }))
}

function valuesPathOptions(rule: GitOpsRuleFormItem): SelectOption[] {
  const selectedFileRaw = String(rule.file_path_template || '').trim()
  const selectedFile = isPlatformValuesFileTemplate(selectedFileRaw) ? selectedFileRaw : ''
  return platformValuesCandidates()
    .filter((item) => {
      if (!selectedFile) {
        return true
      }
      return String(item.file_path_template || '').trim() === selectedFile
    })
    .map((item) => ({
      label: selectedFile
        ? `${item.target_path}${item.sample_value ? ` · 示例: ${item.sample_value}` : ''}`
        : `${pathBaseName(item.file_path_template)} · ${item.target_path}${item.sample_value ? ` · 示例: ${item.sample_value}` : ''}`,
      value: JSON.stringify({
        file_path_template: item.file_path_template,
        target_path: item.target_path,
      }),
    }))
}

function selectedYamlDocumentValue(rule: GitOpsRuleFormItem) {
  if (!rule.document_kind) {
    return undefined
  }
  return JSON.stringify({
    document_kind: rule.document_kind,
    document_name: rule.document_name || '',
  })
}

async function reloadCurrentGitOpsCandidates() {
  const appID = String(formState.application_id || '').trim()
  if (!appID) {
    return
  }
  if (isHelmGitOps()) {
    await loadGitOpsValuesCandidates(appID, false, true)
  } else {
    await loadGitOpsFieldCandidates(appID, false, true)
  }
  void checkCurrentScanPath()
}

async function checkCurrentScanPath() {
  const appID = String(formState.application_id || '').trim()
  if (!appID) {
    return
  }
  const type = isHelmGitOps() ? 'helm' : 'kustomize'
  try {
    const result = await checkGitOpsScanPath(appID, type)
    gitopsScanPathExists.value = result.exists
    gitopsScanPathTemplate.value = result.path_template || (type === 'helm' ? 'apps/helm' : 'apps/{app_key}/overlays/{env}')
  } catch {
    gitopsScanPathExists.value = true
    gitopsScanPathTemplate.value = type === 'helm' ? 'apps/helm' : 'apps/{app_key}/overlays/{env}'
  }
}

function resolveGitOpsSourceLabel(paramKey: string) {
  const normalized = String(paramKey || '').trim().toLowerCase()
  if (!normalized) {
    return '未选择标准字段'
  }
  return gitOpsSourceOptions.value.find((item) => item.value === normalized)?.label || `${resolvePlatformParamName(normalized)} (${normalized})`
}

function gitOpsRuleUsesCDInput(rule: GitOpsRuleFormItem) {
  return rule.source_from === 'cd_input'
}

function resolveGitOpsValueTemplatePlaceholder(rule: GitOpsRuleFormItem) {
  if (gitOpsRuleUsesCDInput(rule)) {
    return '请填写 CD 固定值，例如 registry.example.com/app:stable'
  }
  return isHelmGitOps() ? '默认会使用 {标准Key}，例如 {image_version}' : '默认会使用 {标准Key}，例如 {param_key}-config'
}

function resolveArgoCDModeDescription() {
  return isHelmGitOps()
    ? '当前 GitOps 类型为 Helm，平台会在发布时根据基础环境 env 自动命中已配置的 ArgoCD 实例，并按规则修改 Helm values 文件后触发同步，不会直接修改 Helm 渲染后的 Kubernetes YAML。image_version 在 Jenkins CI 下默认取本次构建号 BUILD_NUMBER。'
    : '当前 GitOps 类型为 Kustomize，平台会在发布时自动更新 kustomization.yaml 的 images[0].newTag 并触发 ArgoCD Sync。如需替换其他 YAML 字段（如 replicas、resources 等），可在下方「新增规则」中添加可选的 manifest 替换规则。'
}

function matchesGitOpsRuleCandidate(rule: GitOpsRuleFormItem, candidate: GitOpsFieldCandidate) {
  return (
    candidate.file_path_template === rule.file_path_template &&
    candidate.document_kind === rule.document_kind &&
    String(candidate.document_name || '') === String(rule.document_name || '') &&
    candidate.target_path === rule.target_path
  )
}

function findGitOpsFieldCandidate(rule: GitOpsRuleFormItem) {
  return gitOpsFieldCandidates.value.find((item) => matchesGitOpsRuleCandidate(rule, item))
}

function formatGitOpsRuleTargetSummary(rule: GitOpsRuleFormItem) {
  if (normalizedGitOpsType(gitOpsType.value) === 'helm') {
    const candidate = platformValuesCandidates().find(
      (item) =>
        String(item.file_path_template || '').trim() === String(rule.file_path_template || '').trim() &&
        String(item.target_path || '').trim() === String(rule.target_path || '').trim(),
    )
    if (candidate) {
      return {
        title: 'Values 路径',
        file: candidate.file_path_template,
        path: candidate.target_path,
        sample: candidate.sample_value || '-',
        stale: false,
      }
    }
    return {
      title: 'Values 路径',
      file: rule.file_path_template || '-',
      path: rule.target_path || '-',
      sample: '-',
      stale: Boolean(rule.file_path_template || rule.target_path),
    }
  }
  const candidate = findGitOpsFieldCandidate(rule)
  if (candidate) {
    return {
      title: candidate.display_name || `${candidate.document_kind} / ${candidate.document_name || '-'}`,
      file: candidate.file_path_template,
      path: candidate.target_path,
      sample: candidate.sample_value || '-',
      stale: false,
    }
  }
  return {
    title: rule.document_name ? `${rule.document_kind} / ${rule.document_name}` : rule.document_kind || 'YAML 字段',
    file: rule.file_path_template || '-',
    path: rule.target_path || '-',
    sample: '-',
    stale: Boolean(rule.file_path_template || rule.target_path),
  }
}

function formatGitOpsRulePanelTitle(rule: GitOpsRuleFormItem) {
  const source = resolveGitOpsSourceLabel(rule.source_param_key)
  const target = formatGitOpsRuleTargetSummary(rule)
  return `${source} -> ${target.path}`
}

function formatGitOpsRulePanelDescription(rule: GitOpsRuleFormItem) {
  const target = formatGitOpsRuleTargetSummary(rule)
  return isHelmGitOps()
    ? `${pathBaseName(target.file)} · ${target.sample}`
    : `${target.title} · ${pathBaseName(target.file)}`
}

function resetScopeState(scope: ReleasePipelineScope) {
  scopeStates[scope].binding_id = ''
  scopeStates[scope].selected_param_def_ids = []
  scopeStates[scope].selectable_params = []
  scopeStates[scope].loading_params = false
  scopeParamConfigs[scope] = {}
}

function resetFormState() {
  formState.id = ''
  formState.name = ''
  formState.application_id = ''
  formState.status = 'active'
  formState.approval_enabled = false
  formState.approval_mode = 'any'
  formState.remark = ''
  approvalApproverIDs.value = []
  scopeStates.ci.enabled = true
  scopeStates.cd.enabled = false
  resetScopeState('ci')
  resetScopeState('cd')
  bindingOptions.value = []
  loadedTemplateBindings.value = []
  scopeBindingWarnings.ci = ''
  scopeBindingWarnings.cd = ''
  scopeComplianceWarnings.ci = ''
  scopeComplianceWarnings.cd = ''
  templateComplianceFindings.value = []
  gitOpsFieldCandidates.value = []
  gitOpsValuesCandidates.value = []
  gitopsRules.value = []
  gitOpsType.value = 'kustomize'
  cdMode.value = 'argocd'
  argocdInfoActiveKeys.value = []
  gitopsRuleActiveKeys.value = []
  templateHooks.value = []
  hookTypePickerVisible.value = false
  pendingHookType.value = 'agent_task'
}

async function refreshScopeBindingWarning(scope: ReleasePipelineScope) {
  scopeBindingWarnings[scope] = ''
  if (!scopeStates[scope].enabled) {
    return
  }
  const bindingID = scopeStates[scope].binding_id.trim()
  if (!bindingID) {
    return
  }
  const existsInOptions = bindingOptionsByScope.value[scope].some((item) => item.value === bindingID)
  if (existsInOptions) {
    bindingLookupCache.value[bindingID] = true
    return
  }
  if (bindingLookupCache.value[bindingID] === false) {
    const templateBinding = loadedTemplateBindings.value.find(
      (item) => item.pipeline_scope === scope && item.enabled && item.binding_id === bindingID,
    )
    const pipelineID = templateBinding?.pipeline_id?.trim()
    scopeBindingWarnings[scope] = pipelineID
      ? `当前模板引用的 ${scope.toUpperCase()} 绑定已失效，将在执行时回退到快照管线 ${pipelineID}；建议尽快重新选择有效绑定`
      : `当前模板引用的 ${scope.toUpperCase()} 绑定已失效，且未保存可回退的管线 ID；发布预检会拦截执行，请尽快重新选择有效绑定`
    return
  }
  try {
    await getPipelineBindingByID(bindingID)
    bindingLookupCache.value[bindingID] = true
    return
  } catch {
    bindingLookupCache.value[bindingID] = false
    const templateBinding = loadedTemplateBindings.value.find(
      (item) => item.pipeline_scope === scope && item.enabled && item.binding_id === bindingID,
    )
    const pipelineID = templateBinding?.pipeline_id?.trim()
    if (pipelineID) {
      scopeBindingWarnings[scope] = `当前模板引用的 ${scope.toUpperCase()} 绑定已失效，将在执行时回退到快照管线 ${pipelineID}；建议尽快重新选择有效绑定`
      return
    }
    scopeBindingWarnings[scope] = `当前模板引用的 ${scope.toUpperCase()} 绑定已失效，且未保存可回退的管线 ID；发布预检会拦截执行，请尽快重新选择有效绑定`
  }
}

async function refreshBindingWarnings() {
  await Promise.all((['ci', 'cd'] as ReleasePipelineScope[]).map((scope) => refreshScopeBindingWarning(scope)))
}

function normalizeGitOpsRulePayload(item: GitOpsRuleFormItem): ReleaseTemplateGitOpsRulePayload {
  let filePathTemplate = normalizeHelmValuesFilePathTemplate(String(item.file_path_template || '').trim())
  let targetPath = String(item.target_path || '').trim()
  let documentKind = String(item.document_kind || '').trim()
  let documentName = String(item.document_name || '').trim()

  // 兼容历史或异常态：如果 Values 路径下拉把组合值直接落进了 target_path，
  // 提交前在前端再兜底解析一次，避免保存时因为候选键不匹配而失败
  if (normalizedGitOpsType(gitOpsType.value) === 'helm' && targetPath.startsWith('{')) {
    try {
      const parsed = JSON.parse(targetPath)
      filePathTemplate = normalizeHelmValuesFilePathTemplate(String(parsed.file_path_template || filePathTemplate).trim())
      targetPath = String(parsed.target_path || '').trim()
      documentKind = 'values'
      documentName = ''
    } catch {
      // noop: 保留原值，由后端继续兜底校验
    }
  }

  return {
    source_param_key: item.source_param_key,
    source_from: item.source_from,
    file_path_template: filePathTemplate,
    document_kind: documentKind,
    document_name: documentName || undefined,
    target_path: targetPath,
    value_template: item.value_template || undefined,
  }
}

function buildPayload(): ReleaseTemplatePayload | UpdateReleaseTemplatePayload {
  return {
    name: formState.name.trim(),
    ...(modalMode.value !== 'edit' ? { application_id: formState.application_id.trim() } : {}),
    ci_binding_id: scopeStates.ci.enabled ? scopeStates.ci.binding_id.trim() || undefined : undefined,
    cd_binding_id: scopeStates.cd.enabled && isCDUsingPipeline() ? scopeStates.cd.binding_id.trim() || undefined : undefined,
    cd_provider: scopeStates.cd.enabled ? (isCDUsingPipeline() ? (selectedBinding('cd')?.provider || 'jenkins') : 'argocd') : undefined,
    gitops_type: scopeStates.cd.enabled && isCDUsingArgoCD() ? normalizedGitOpsType(gitOpsType.value) : undefined,
    status: formState.status,
    remark: formState.remark.trim() || undefined,
    ci_param_def_ids: scopeStates.ci.enabled ? [...scopeStates.ci.selected_param_def_ids] : [],
    cd_param_def_ids: scopeStates.cd.enabled && isCDUsingPipeline() ? [...scopeStates.cd.selected_param_def_ids] : [],
    ci_param_configs: scopeStates.ci.enabled ? buildTemplateParamConfigs('ci') : [],
    cd_param_configs: scopeStates.cd.enabled && isCDUsingPipeline() ? buildTemplateParamConfigs('cd') : [],
    gitops_rules:
      scopeStates.cd.enabled && isCDUsingArgoCD()
        ? [
            ...(gitopsRulesByType.value['helm'] || []).map<ReleaseTemplateGitOpsRulePayload>((item) => normalizeGitOpsRulePayload(item)),
            ...(gitopsRulesByType.value['kustomize'] || []).map<ReleaseTemplateGitOpsRulePayload>((item) => normalizeGitOpsRulePayload(item)),
          ]
        : [],
    hooks: templateHooks.value.map<ReleaseTemplateHookPayload>((item) => ({
      hook_type: item.hook_type,
      name: item.name.trim(),
      execute_stage: normalizeHookExecuteStages(item.execute_stages)[0],
      execute_stages: normalizeHookExecuteStages(item.execute_stages),
      trigger_condition: item.trigger_condition,
      failure_policy: item.failure_policy,
      env_codes: normalizeHookEnvCodes(item.env_codes),
      target_id:
        item.hook_type === 'agent_task' || item.hook_type === 'notification_hook'
          ? item.target_id.trim() || undefined
          : undefined,
      webhook_method: item.hook_type === 'webhook_notification' ? item.webhook_method : undefined,
      webhook_url: item.hook_type === 'webhook_notification' ? item.webhook_url.trim() || undefined : undefined,
      webhook_body: item.hook_type === 'webhook_notification' ? item.webhook_body_template : undefined,
      note: item.note.trim() || undefined,
    })),
  }
}

async function loadPlatformParamMap() {
  if (platformParamDicts.value.length > 0) {
    return
  }
  try {
    const response = await listPlatformParamDicts({ page: 1, page_size: 200 })
    const next: Record<string, string> = {}
    response.data.forEach((item) => {
      next[item.param_key] = item.name
    })
    platformParamNameMap.value = next
    platformParamDicts.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字库加载失败'))
  }
}

async function loadApprovalUserOptions() {
  if (userOptions.value.length > 0) {
    return
  }
  try {
    const response = await listUserOptions()
    userOptions.value = response.data || []
  } catch (error) {
    userOptions.value = []
    message.error(extractHTTPErrorMessage(error, '审批人选项加载失败'))
  }
}

async function loadAgentTaskTemplates(force = false) {
  if (!force && agentTaskTemplates.value.length > 0) {
    return
  }
  loadingAgentTaskTemplates.value = true
  try {
    const response = await listAllAgentTasks({ page: 1, page_size: 200 })
    agentTaskTemplates.value = (response.data || []).filter(isAgentTaskTemplateCandidate)
  } catch (error) {
    agentTaskTemplates.value = []
    message.error(extractHTTPErrorMessage(error, 'Agent 任务模板加载失败'))
  } finally {
    loadingAgentTaskTemplates.value = false
  }
}

async function loadNotificationHooks() {
  if (notificationHooks.value.length > 0) {
    return
  }
  loadingNotificationHooks.value = true
  try {
    const response = await listNotificationHooks({ page: 1, page_size: 200, enabled: true })
    notificationHooks.value = response.data
  } catch (error) {
    notificationHooks.value = []
    message.error(extractHTTPErrorMessage(error, '通知 Hook 加载失败'))
  } finally {
    loadingNotificationHooks.value = false
  }
}

async function loadGitOpsFieldCandidates(applicationID: string, silent = false, force = false) {
  const appID = String(applicationID || '').trim()
  if (!appID) {
    gitOpsFieldCandidates.value = []
    return
  }
  if (!force && gitOpsFieldCandidateCache.value[appID]) {
    gitOpsFieldCandidates.value = [...gitOpsFieldCandidateCache.value[appID]]
    return
  }
  loadingGitOpsFieldCandidates.value = true
  try {
    const response = await listGitOpsFieldCandidates(appID)
    gitOpsFieldCandidateCache.value[appID] = response.data
    gitOpsFieldCandidates.value = [...response.data]
  } catch (error) {
    gitOpsFieldCandidates.value = []
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, 'GitOps YAML 字段加载失败'))
    }
  } finally {
    loadingGitOpsFieldCandidates.value = false
  }
}

async function loadGitOpsValuesCandidates(applicationID: string, silent = false, force = false) {
  const appID = String(applicationID || '').trim()
  if (!appID) {
    gitOpsValuesCandidates.value = []
    return
  }
  if (!force && gitOpsValuesCandidateCache.value[appID]) {
    gitOpsValuesCandidates.value = [...gitOpsValuesCandidateCache.value[appID]]
    return
  }
  loadingGitOpsFieldCandidates.value = true
  try {
    const response = await listGitOpsValuesCandidates(appID)
    gitOpsValuesCandidateCache.value[appID] = response.data
    gitOpsValuesCandidates.value = [...response.data]
  } catch (error) {
    gitOpsValuesCandidates.value = []
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, 'GitOps Values 路径加载失败'))
    }
  } finally {
    loadingGitOpsFieldCandidates.value = false
  }
}

async function loadApplications() {
  try {
    const response = await listApplications({ page: 1, page_size: 200 })
    applicationRecords.value = response.data
    applicationOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用下拉加载失败'))
  }
}

async function loadTemplates() {
  loading.value = true
  try {
    const response = await listReleaseTemplates({
      application_id: filters.application_id || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
    templateBindingWarnings.value = Object.fromEntries(
      response.data
        .map((item) => [item.id, templateBindingWarningCache.value[item.id] || ''] as const)
        .filter(([, warning]) => Boolean(warning)),
    )
    void loadTemplateBindingWarnings(response.data)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布模板加载失败'))
  } finally {
    loading.value = false
  }
}

async function inspectTemplateBindingWarning(templateID: string): Promise<string> {
  try {
    const response = await getReleaseTemplateByID(templateID)
    const bindings = response.data.bindings || []
    for (const binding of bindings) {
      const bindingID = String(binding.binding_id || '').trim()
      if (!bindingID) {
        continue
      }
      if (bindingLookupCache.value[bindingID] === true) {
        continue
      }
      if (bindingLookupCache.value[bindingID] === false) {
        const pipelineID = String(binding.pipeline_id || '').trim()
        if (pipelineID) {
          return `${binding.pipeline_scope.toUpperCase()} 绑定已失效，将回退到快照管线 ${pipelineID}`
        }
        return `${binding.pipeline_scope.toUpperCase()} 绑定已失效，请重新选择有效绑定`
      }
      try {
        await getPipelineBindingByID(bindingID)
        bindingLookupCache.value[bindingID] = true
      } catch {
        bindingLookupCache.value[bindingID] = false
        const pipelineID = String(binding.pipeline_id || '').trim()
        if (pipelineID) {
          return `${binding.pipeline_scope.toUpperCase()} 绑定已失效，将回退到快照管线 ${pipelineID}`
        }
        return `${binding.pipeline_scope.toUpperCase()} 绑定已失效，请重新选择有效绑定`
      }
    }
    return ''
  } catch {
    return ''
  }
}

async function loadTemplateBindingWarnings(items: ReleaseTemplate[]) {
  const missingItems = items.filter((item) => typeof templateBindingWarningCache.value[item.id] === 'undefined')
  if (missingItems.length === 0) {
    templateBindingWarnings.value = Object.fromEntries(
      items
        .map((item) => [item.id, templateBindingWarningCache.value[item.id] || ''] as const)
        .filter(([, warning]) => Boolean(warning)),
    )
    return
  }

  const nextWarnings = { ...templateBindingWarnings.value }
  const concurrency = 4
  for (let index = 0; index < missingItems.length; index += concurrency) {
    const chunk = missingItems.slice(index, index + concurrency)
    const warnings = await Promise.all(
      chunk.map(async (item) => ({
        id: item.id,
        warning: await inspectTemplateBindingWarning(item.id),
      })),
    )
    warnings.forEach(({ id, warning }) => {
      templateBindingWarningCache.value[id] = warning
      if (warning) {
        nextWarnings[id] = warning
      } else {
        delete nextWarnings[id]
      }
    })
    templateBindingWarnings.value = { ...nextWarnings }
  }
}

async function loadBindings(applicationID: string, options?: { force?: boolean; silent?: boolean }) {
  const appID = String(applicationID || '').trim()
  if (!appID) {
    bindingOptions.value = []
    resetScopeState('ci')
    resetScopeState('cd')
    return
  }
  if (!options?.force && bindingOptionsCache.value[appID]) {
    bindingOptions.value = [...bindingOptionsCache.value[appID]]
    return
  }
  loadingBindings.value = true
  try {
    const response = await listPipelineBindings(appID, {
      status: 'active',
      page: 1,
      page_size: 200,
    })
    const nextOptions = response.data.map((item) => ({
      label: `${item.name || item.binding_type} [${item.binding_type}/${item.provider}]`,
      value: item.id,
      binding_type: item.binding_type,
      provider: item.provider,
      pipeline_id: item.pipeline_id,
    }))
    bindingOptionsCache.value[appID] = nextOptions
    bindingOptions.value = [...nextOptions]
  } catch (error) {
    bindingOptions.value = []
    if (!options?.silent) {
      message.error(extractHTTPErrorMessage(error, '绑定下拉加载失败'))
    }
  } finally {
    loadingBindings.value = false
  }
}

async function loadSelectableParams(scope: ReleasePipelineScope, preserveSelection = false) {
  const state = scopeStates[scope]
  const appID = formState.application_id.trim()
  const binding = selectedBinding(scope)

  if (scope === 'cd' && !isCDUsingPipeline()) {
    state.selectable_params = []
    if (!preserveSelection) {
      state.selected_param_def_ids = []
    }
    scopeParamConfigs.cd = {}
    return
  }

  if (!state.enabled || !appID || !binding) {
    state.selectable_params = []
    if (!preserveSelection) {
      state.selected_param_def_ids = []
    }
    return
  }

  if (binding.provider !== 'jenkins') {
    state.selectable_params = []
    state.selected_param_def_ids = []
    return
  }

  state.loading_params = true
  try {
    const cacheKey = `${appID}:${scope}`
    let cached = selectableParamsCache.value[cacheKey]
    if (!cached) {
      const response = await listApplicationExecutorParamDefs(appID, {
        binding_type: scope,
        status: 'active',
        page: 1,
        page_size: 200,
      })
      cached = response.data.filter((item) => String(item.param_key || '').trim().toLowerCase() !== '')
      selectableParamsCache.value[cacheKey] = cached
    }
    state.selectable_params = [...cached]
    const allowed = new Set(state.selectable_params.map((item) => item.id))
    state.selected_param_def_ids = state.selected_param_def_ids.filter((item) => allowed.has(item))
    syncScopeParamConfigs(scope)
  } catch (error) {
    state.selectable_params = []
    state.selected_param_def_ids = []
    scopeParamConfigs[scope] = {}
    message.error(extractHTTPErrorMessage(error, `${scope.toUpperCase()} 模板参数加载失败`))
  } finally {
    state.loading_params = false
  }
}

async function handleApplicationChange(value: string | undefined) {
  formState.application_id = String(value || '')
  resetScopeState('ci')
  resetScopeState('cd')
  loadedTemplateBindings.value = []
  scopeBindingWarnings.ci = ''
  scopeBindingWarnings.cd = ''
  scopeComplianceWarnings.ci = ''
  scopeComplianceWarnings.cd = ''
  gitopsRules.value = []
  gitOpsType.value = 'kustomize'
  const tasks: Array<Promise<unknown>> = [loadBindings(formState.application_id)]
  if (isCDUsingArgoCD()) {
    tasks.push(isHelmGitOps() ? loadGitOpsValuesCandidates(formState.application_id, true) : loadGitOpsFieldCandidates(formState.application_id, true))
  }
  await Promise.all(tasks)
}

async function handleScopeBindingChange(scope: ReleasePipelineScope, value: string | undefined) {
  scopeStates[scope].binding_id = String(value || '')
  scopeStates[scope].selected_param_def_ids = []
  scopeParamConfigs[scope] = {}
  await loadSelectableParams(scope)
  await refreshScopeBindingWarning(scope)
  await refreshScopeComplianceWarning(scope)
}

function handleCDModeChange(value: string | number) {
  const nextMode = String(value || '').trim() === 'pipeline' ? 'pipeline' : 'argocd'
  if (cdMode.value === nextMode) {
    return
  }
  cdMode.value = nextMode
  if (nextMode === 'argocd') {
    scopeStates.cd.binding_id = ''
    scopeStates.cd.selected_param_def_ids = []
    scopeStates.cd.selectable_params = []
    scopeStates.cd.loading_params = false
    scopeParamConfigs.cd = {}
    scopeComplianceWarnings.cd = ''
    void reloadCurrentGitOpsCandidates()
    return
  }
  scopeStates.cd.selected_param_def_ids = []
  scopeParamConfigs.cd = {}
  void loadSelectableParams('cd')
  void refreshScopeComplianceWarning('cd')
}

async function handleScopeEnabledChange(scope: ReleasePipelineScope, checked: boolean) {
  scopeStates[scope].enabled = checked
  if (!checked) {
    resetScopeState(scope)
    scopeBindingWarnings[scope] = ''
    scopeComplianceWarnings[scope] = ''
    if (scope === 'cd') {
      gitopsRules.value = []
      gitOpsType.value = 'kustomize'
      cdMode.value = 'argocd'
    }
    return
  }
  if (scope === 'cd' && isCDUsingArgoCD()) {
    void reloadCurrentGitOpsCandidates()
    await refreshScopeBindingWarning(scope)
    await refreshScopeComplianceWarning(scope)
    return
  }
  await loadSelectableParams(scope)
  await refreshScopeBindingWarning(scope)
  await refreshScopeComplianceWarning(scope)
}

function handleGitOpsTypeChange(value: ReleaseTemplateGitOpsType) {
  const nextType = normalizedGitOpsType(value)
  if (nextType === gitOpsType.value) {
    return
  }
  swapGitOpsTypeRules(nextType)
}

function swapGitOpsTypeRules(nextType: ReleaseTemplateGitOpsType) {
  const prevType = normalizedGitOpsType(gitOpsType.value)
  if (prevType) {
    gitopsRulesByType.value[prevType] = [...gitopsRules.value]
  }
  gitOpsType.value = nextType
  gitopsRules.value = gitopsRulesByType.value[nextType] ? [...gitopsRulesByType.value[nextType]] : []
  void reloadCurrentGitOpsCandidates()
}

watch(gitOpsType, (value, oldValue) => {
  const nextType = normalizedGitOpsType(value)
  const prevType = normalizedGitOpsType(oldValue)
  if (nextType === prevType) {
    return
  }
  if (prevType) {
    gitopsRulesByType.value[prevType] = [...gitopsRules.value]
  }
  gitopsRules.value = gitopsRulesByType.value[nextType] ? [...gitopsRulesByType.value[nextType]] : []
  void reloadCurrentGitOpsCandidates()
})

function getRowSelection(scope: ReleasePipelineScope) {
  return {
    selectedRowKeys: scopeStates[scope].selected_param_def_ids,
    onChange: (keys: Array<string | number>) => {
      scopeStates[scope].selected_param_def_ids = keys.map((item) => String(item))
      syncScopeParamConfigs(scope)
    },
  }
}

function addGitOpsRule() {
  const item = createGitOpsRuleFormItem()
  gitopsRules.value.push(item)
  gitopsRuleActiveKeys.value = []
}

function removeGitOpsRule(localID: string) {
  gitopsRules.value = gitopsRules.value.filter((item) => item.local_id !== localID)
  gitopsRuleActiveKeys.value = gitopsRuleActiveKeys.value.filter((item) => item !== localID)
}

function handleGitOpsRuleSourceChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  const nextKey = String(value || '').trim().toLowerCase()
  const previousTemplate = `{${rule.source_param_key}}`
  rule.source_param_key = nextKey
  rule.source_from = resolveGitOpsRuleSourceFrom(nextKey)
  if (rule.source_from === 'cd_input') {
    if (!rule.value_template || rule.value_template === previousTemplate) {
      rule.value_template = ''
    }
    return
  }
  if (!rule.value_template || rule.value_template === previousTemplate) {
    rule.value_template = nextKey ? `{${nextKey}}` : ''
  }
}

function clearGitOpsRuleTarget(rule: GitOpsRuleFormItem) {
  rule.file_path_template = ''
  rule.document_kind = ''
  rule.document_name = ''
  rule.target_path = ''
}

function handleYamlFileTemplateChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  rule.file_path_template = String(value || '').trim()
  rule.document_kind = ''
  rule.document_name = ''
  rule.target_path = ''
}

function handleYamlDocumentChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  if (!value) {
    rule.document_kind = ''
    rule.document_name = ''
    rule.target_path = ''
    return
  }
  try {
    const parsed = JSON.parse(String(value))
    rule.document_kind = String(parsed.document_kind || '').trim()
    rule.document_name = String(parsed.document_name || '').trim()
    rule.target_path = ''
  } catch {
    message.error('YAML 资源选择解析失败，请重新选择')
  }
}

function handleYamlTargetPathChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  rule.target_path = String(value || '').trim()
}

function handleValuesFileTemplateChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  rule.file_path_template = String(value || '').trim()
  rule.document_kind = 'values'
  rule.document_name = ''
  rule.target_path = ''
}

function handleValuesTargetPathChange(rule: GitOpsRuleFormItem, value: string | undefined) {
  rule.document_kind = 'values'
  rule.document_name = ''
  if (!value) {
    rule.target_path = ''
    return
  }
  try {
    const parsed = JSON.parse(String(value))
    rule.file_path_template = String(parsed.file_path_template || rule.file_path_template || '').trim()
    rule.target_path = String(parsed.target_path || '').trim()
  } catch {
    rule.target_path = String(value || '').trim()
  }
}

function handleSearch() {
  filters.page = 1
  void loadTemplates()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.pageSize = pageSize
  void loadTemplates()
}

function applyTemplateFiltersFromRouteQuery() {
  filters.application_id = String(route.query.application_id || '').trim()
  filters.page = 1
}

function readTemplateEditorViewportInset() {
  if (typeof document === 'undefined') {
    return 0
  }

  const appLayout = document.querySelector('.app-layout')
  if (appLayout) {
    const rawWidth = window.getComputedStyle(appLayout).getPropertyValue('--layout-sider-width').trim()
    const parsedWidth = Number.parseFloat(rawWidth)
    if (Number.isFinite(parsedWidth) && parsedWidth >= 0) {
      return parsedWidth
    }
  }

  const sider = document.querySelector('.app-sider')
  if (!sider) {
    return 0
  }
  return Math.max(sider.getBoundingClientRect().width, 0)
}

function syncTemplateEditorViewportInset() {
  templateEditorViewportInset.value = readTemplateEditorViewportInset()
}

function observeTemplateEditorViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  templateEditorViewportObserver?.disconnect()
  templateEditorViewportObserver = new ResizeObserver(() => {
    syncTemplateEditorViewportInset()
  })

  if (appLayout) {
    templateEditorViewportObserver.observe(appLayout)
  }
  if (sider) {
    templateEditorViewportObserver.observe(sider)
  }
}

function stopObservingTemplateEditorViewportInset() {
  templateEditorViewportObserver?.disconnect()
  templateEditorViewportObserver = null
}

function openCreateModal() {
  modalMode.value = 'create'
  resetFormState()
  void Promise.all([
    loadPlatformParamMap(),
    loadApprovalUserOptions(),
    loadAgentTaskTemplates(true),
    loadNotificationHooks(),
    loadHookEnvOptions([], true),
  ])
  modalVisible.value = true
}

function applyBindingsToForm(bindings: ReleaseTemplateBinding[]) {
  const ciBinding = bindings.find((item) => item.pipeline_scope === 'ci' && item.enabled)
  const cdBinding = bindings.find((item) => item.pipeline_scope === 'cd' && item.enabled)

  scopeStates.ci.enabled = Boolean(ciBinding)
  scopeStates.ci.binding_id = ciBinding?.binding_id || ''
  scopeStates.cd.enabled = Boolean(cdBinding)
  cdMode.value = cdBinding && cdBinding.provider === 'jenkins' ? 'pipeline' : 'argocd'
  scopeStates.cd.binding_id = cdMode.value === 'pipeline' ? (cdBinding?.binding_id || '') : ''
  scopeStates.cd.selected_param_def_ids = []
  scopeStates.cd.selectable_params = []
  scopeStates.cd.loading_params = false
}

async function openEditModal(record: ReleaseTemplate, mode: Extract<FormMode, 'edit' | 'clone'> = 'edit') {
  modalMode.value = mode
  resetFormState()
  modalVisible.value = true
  modalLoading.value = true
  try {
    const detailPromise = getReleaseTemplateByID(record.id)
    const preloadTasks: Array<Promise<unknown>> = []
    if (!platformParamDicts.value.length) {
      preloadTasks.push(loadPlatformParamMap())
    }
    if (!userOptions.value.length) {
      preloadTasks.push(loadApprovalUserOptions())
    }
    preloadTasks.push(loadAgentTaskTemplates(true))
    if (!notificationHooks.value.length) {
      preloadTasks.push(loadNotificationHooks())
    }
    preloadTasks.push(loadHookEnvOptions([], true))
    const [response] = await Promise.all([detailPromise, ...preloadTasks])
    const { template, bindings, params, gitops_rules, hooks } = response.data
    formState.id = mode === 'clone' ? '' : template.id
    formState.name = mode === 'clone' ? buildCloneName(template.name) : template.name
    formState.application_id = template.application_id
    formState.status = template.status
    formState.approval_enabled = Boolean(template.approval_enabled)
    formState.approval_mode = (template.approval_mode || 'any') as ReleaseTemplateApprovalMode
    approvalApproverIDs.value = [...(template.approval_approver_ids || [])]
    formState.remark = template.remark
    templateComplianceFindings.value = [...(template.compliance_findings || [])]
    ;(['ci', 'cd'] as ReleasePipelineScope[]).forEach((scope) => {
      const findings = templateComplianceFindingsByScope(scope)
      scopeComplianceWarnings[scope] = findings.length
        ? `${scope.toUpperCase()} 管线违反发布模板校验：${findings.map(formatComplianceFinding).join('；')}`
        : ''
    })
    gitOpsType.value = normalizedGitOpsType(template.gitops_type)

    loadedTemplateBindings.value = bindings
    const loadTasks: Array<Promise<unknown>> = [loadBindings(formState.application_id, { silent: true })]
    if (bindings.some((item) => item.pipeline_scope === 'cd' && item.enabled && item.provider === 'argocd')) {
      if (gitOpsType.value === 'helm') {
        loadTasks.push(loadGitOpsValuesCandidates(formState.application_id, true))
      } else {
        loadTasks.push(loadGitOpsFieldCandidates(formState.application_id, true))
      }
    }
    await Promise.all(loadTasks)
    applyBindingsToForm(bindings)
    void refreshBindingWarnings()
    void checkCurrentScanPath()

    scopeStates.ci.selected_param_def_ids = params
      .filter((item) => item.pipeline_scope === 'ci')
      .map((item) => item.executor_param_def_id)
    scopeStates.cd.selected_param_def_ids = params
      .filter((item) => item.pipeline_scope === 'cd')
      .map((item) => item.executor_param_def_id)
    scopeParamConfigs.ci = {}
    params
      .filter((item) => item.pipeline_scope === 'ci')
      .forEach((item) => {
        scopeParamConfigs.ci[item.executor_param_def_id] = createTemplateParamConfigState('ci', {
          value_source: item.value_source,
          source_param_key: item.source_param_key,
          fixed_value: item.fixed_value,
        })
      })
    scopeParamConfigs.cd = {}
    params
      .filter((item) => item.pipeline_scope === 'cd')
      .forEach((item) => {
        scopeParamConfigs.cd[item.executor_param_def_id] = createTemplateParamConfigState('cd', {
          value_source: item.value_source,
          source_param_key: item.source_param_key,
          fixed_value: item.fixed_value,
        })
      })

    const paramLoadTasks: Array<Promise<unknown>> = []
    if (scopeStates.ci.enabled) {
      paramLoadTasks.push(loadSelectableParams('ci', true))
    }
    if (scopeStates.cd.enabled && isCDUsingPipeline()) {
      paramLoadTasks.push(loadSelectableParams('cd', true))
    }
    await Promise.all(paramLoadTasks)

    const allRules = (gitops_rules || []).map((item: ReleaseTemplateGitOpsRule) =>
      createGitOpsRuleFormItem({
        source_param_key: item.source_param_key,
        source_from: item.source_from,
        file_path_template: item.file_path_template,
        document_kind: item.document_kind,
        document_name: item.document_name,
        target_path: item.target_path,
        value_template: item.value_template,
      }),
    )
    gitopsRulesByType.value = {
      helm: allRules.filter((r) => r.document_kind === 'values'),
      kustomize: allRules.filter((r) => r.document_kind !== 'values'),
    }
    gitopsRules.value = normalizedGitOpsType(gitOpsType.value) === 'helm'
      ? [...gitopsRulesByType.value.helm]
      : [...gitopsRulesByType.value.kustomize]
    await loadHookEnvOptions((hooks || []).flatMap((item: ReleaseTemplateHook) => item.env_codes || []), true)
    templateHooks.value = (hooks || []).map((item: ReleaseTemplateHook) => createHookFormItemFromResponse(item))
    templateHooks.value.forEach((item) => syncHookTargetName(item))
    gitopsRuleActiveKeys.value = []
    if (gitopsRules.value.some((item) => formatGitOpsRuleTargetSummary(item).stale)) {
      message.warning(
        isHelmGitOps()
          ? '检测到部分 GitOps 规则引用的 Values 路径已变化，请在保存前重新确认'
          : '检测到部分 GitOps 规则引用的 YAML 字段已变化，请在保存前重新确认',
      )
    }
  } catch (error) {
    modalVisible.value = false
    message.error(extractHTTPErrorMessage(error, mode === 'clone' ? '发布模板克隆配置加载失败' : '发布模板详情加载失败'))
  } finally {
    modalLoading.value = false
  }
}

function openCloneModal(record: ReleaseTemplate) {
  void openEditModal(record, 'clone')
}

function closeModal() {
  modalVisible.value = false
  modalLoading.value = false
}

function handleTemplateEditorAfterClose() {
  submitting.value = false
  resetFormState()
  formRef.value?.clearValidate()
}

function validateScopeState() {
  const enabledScopes = (['ci', 'cd'] as ReleasePipelineScope[]).filter((scope) => scopeStates[scope].enabled)
  if (enabledScopes.length === 0) {
    throw new Error('请至少启用一个执行单元')
  }
  for (const scope of enabledScopes) {
    if (scope === 'cd') {
      if (isCDUsingArgoCD()) {
        continue
      }
      if (!scopeStates[scope].binding_id.trim()) {
        throw new Error('请选择 CD 绑定管线')
      }
      validateTemplateParamConfigs('cd')
      continue
    }
    if (!scopeStates[scope].binding_id.trim()) {
      throw new Error(`请选择 ${scope.toUpperCase()} 绑定管线`)
    }
    validateTemplateParamConfigs(scope)
  }
  if (isCDUsingArgoCD()) {
    if (isUnsupportedKustomizeGitOps()) {
      throw new Error('Kustomize 模式暂不支持，请先切换到 Helm')
    }
    for (const item of gitopsRules.value) {
      if (!item.source_param_key.trim()) {
        throw new Error('请为 GitOps 替换规则选择标准字段')
      }
      if (gitOpsRuleUsesCDInput(item) && !item.value_template.trim()) {
        throw new Error(`请为 CD 自填字段 ${resolvePlatformParamName(item.source_param_key)} 填写固定值`)
      }
      if (isHelmGitOps()) {
        if (!item.file_path_template.trim() || !item.target_path.trim()) {
          throw new Error('请为 GitOps 替换规则选择 Values 路径')
        }
      } else if (!item.file_path_template.trim() || !item.document_kind.trim() || !item.target_path.trim()) {
        throw new Error('请为 GitOps 替换规则选择 YAML 字段')
      }
    }
  }
}

function validateTemplateParamConfigs(scope: ReleasePipelineScope) {
  for (const item of selectedScopeParamDefs(scope)) {
    const config = getTemplateParamConfig(scope, item.id)
    const label = `${resolvePlatformParamName(String(item.param_key || '').trim().toLowerCase())} / ${item.executor_param_name}`
    if (scope === 'ci') {
      if (config.value_source === 'fixed' && !String(config.fixed_value || '').trim()) {
        throw new Error(`请为 ${label} 填写模板固定值`)
      }
      continue
    }
    if (config.value_source === 'fixed' && !String(config.fixed_value || '').trim()) {
      throw new Error(`请为 ${label} 填写模板固定值`)
    }
    if ((config.value_source === 'ci_param' || config.value_source === 'builtin') && !String(config.source_param_key || '').trim()) {
      throw new Error(`请为 ${label} 选择来源字段`)
    }
  }
}

async function submitForm() {
  await formRef.value?.validate()
  try {
    validateScopeState()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '模板配置校验失败')
    return
  }

  submitting.value = true
  try {
    if (modalMode.value !== 'edit') {
      await createReleaseTemplate(buildPayload() as ReleaseTemplatePayload)
      message.success(modalMode.value === 'clone' ? '发布模板克隆成功' : '发布模板创建成功')
    } else {
      await updateReleaseTemplate(formState.id, buildPayload() as UpdateReleaseTemplatePayload)
      message.success('发布模板更新成功')
    }
    closeModal()
    await loadTemplates()
  } catch (error) {
    message.error(
      extractHTTPErrorMessage(
        error,
        modalMode.value === 'edit' ? '发布模板更新失败' : modalMode.value === 'clone' ? '发布模板克隆失败' : '发布模板创建失败',
      ),
    )
  } finally {
    submitting.value = false
  }
}

async function handleDelete(record: ReleaseTemplate) {
  deletingID.value = record.id
  try {
    await deleteReleaseTemplate(record.id)
    message.success('发布模板删除成功')
    if (dataSource.value.length === 1 && filters.page > 1) {
      filters.page -= 1
    }
    await loadTemplates()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布模板删除失败'))
  } finally {
    deletingID.value = ''
  }
}

onMounted(async () => {
  syncTemplateEditorViewportInset()
  observeTemplateEditorViewportInset()
  applyTemplateFiltersFromRouteQuery()
  await loadApplications()
  await loadTemplates()
})

onBeforeUnmount(() => {
  stopObservingTemplateEditorViewportInset()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header release-template-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">模板</h2>
      </div>
      <div class="page-header-actions release-template-header-actions">
        <a-select
          v-model:value="applicationFilterValue"
          class="release-template-toolbar-select release-template-toolbar-select-wide"
          show-search
          option-filter-prop="label"
          placeholder="应用"
          :options="templateApplicationFilterOptions"
        />
        <a-select
          v-model:value="statusFilterValue"
          class="release-template-toolbar-select"
          placeholder="状态"
          :options="templateStatusFilterOptions"
        />
        <a-button class="release-template-toolbar-query-btn" @click="handleSearch">查询</a-button>
        <a-button v-if="canManageTemplate" class="application-toolbar-action-btn release-template-create-btn" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增发布模板
        </a-button>
      </div>
    </div>

    <div class="release-template-table-section">
      <a-table
        row-key="id"
        class="release-template-table"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 980 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <div class="template-name-cell">
              <span class="template-name-text">{{ record.name }}</span>
              <a-tag
                v-if="templateBindingWarnings[record.id]"
                class="dashboard-chip dashboard-chip-warning"
              >
                绑定异常
              </a-tag>
              <button
                v-if="isTemplateViolated(record)"
                type="button"
                class="template-compliance-chip-button"
                title="查看违规概览"
                @click.stop="openTemplateComplianceDetail(record)"
              >
                <span class="dashboard-chip dashboard-chip-warning template-compliance-chip-text">管线违规</span>
              </button>
            </div>
            <div v-if="templateBindingWarnings[record.id]" class="template-binding-warning-text">
              {{ templateBindingWarnings[record.id] }}
            </div>
            <div v-if="templateComplianceWarning(record)" class="template-binding-warning-text">
              {{ templateComplianceWarning(record) }}
            </div>
          </template>
          <template v-else-if="column.key === 'binding_name'">
            <div v-if="templateExecutionUnits(record).length" class="template-binding-pill" :aria-label="`执行单元：${templateExecutionSummary(record)}`">
              <span
                v-for="unit in templateExecutionUnits(record)"
                :key="`${record.id}-${unit.key}`"
                class="template-binding-pill-segment"
                :class="`template-binding-pill-segment--${unit.key}`"
              >
                <DeploymentUnitOutlined v-if="unit.key === 'ci'" />
                <RocketOutlined v-else-if="unit.key === 'cd'" />
                <span>{{ unit.label }}</span>
              </span>
            </div>
            <span v-else class="template-binding-text">{{ templateExecutionSummary(record) }}</span>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space v-if="canManageTemplate">
              <a-button type="link" size="small" @click="openEditModal(record)">编辑</a-button>
              <a-button type="link" size="small" @click="openCloneModal(record)">
                <template #icon>
                  <CopyOutlined />
                </template>
                克隆
              </a-button>
              <a-popconfirm
                title="确认删除当前发布模板吗？"
                ok-text="删除"
                cancel-text="取消"
                @confirm="handleDelete(record)"
              >
                <template #icon>
                  <ExclamationCircleOutlined class="danger-icon" />
                </template>
                <a-button type="link" size="small" danger :loading="deletingID === record.id">删除</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>

      <div class="pagination-area">
        <a-pagination
          :current="filters.page"
          :page-size="filters.pageSize"
          :total="total"
          :page-size-options="['10', '20', '50', '100']"
          show-size-changer
          show-quick-jumper
          :show-total="(count: number) => `共 ${count} 条`"
          @change="handlePageChange"
        />
      </div>
    </div>

    <a-modal
      :open="complianceDetailVisible"
      :width="600"
      title="管线违规概览"
      :footer="null"
      @cancel="closeTemplateComplianceDetail"
    >
      <div v-if="complianceDetailTemplate" class="template-compliance-detail">
        <div class="template-compliance-detail-summary">
          <div>
            <div class="template-compliance-detail-title">{{ complianceDetailTemplate.name || '-' }}</div>
            <div class="template-compliance-detail-app">{{ complianceDetailTemplate.application_name || '-' }}</div>
          </div>
          <a-tag class="dashboard-chip dashboard-chip-warning">违规</a-tag>
        </div>
        <div class="template-compliance-detail-message">
          {{ complianceDetailTemplate.compliance_summary || '发布模板违反已启用的管线规范' }}
        </div>

        <a-empty
          v-if="!complianceDetailFindings.length"
          description="暂无详细违规项，请根据列表摘要检查模板绑定的管线规范"
        />
        <div v-else class="template-compliance-detail-scopes">
          <section
            v-for="scope in complianceDetailScopes"
            :key="scope"
            class="template-compliance-detail-scope"
          >
            <div class="template-compliance-detail-scope-title">
              <span>{{ complianceScopeText(scope) }}</span>
              <a-tag>{{ complianceDetailFindingsByScope(scope).length }} 项</a-tag>
            </div>
            <div class="template-compliance-finding-list">
              <div
                v-for="finding in complianceDetailFindingsByScope(scope)"
                :key="`${scope}-${finding.pipeline_id}-${finding.rule_id}-${finding.rule_code}-${finding.line_no}`"
                class="template-compliance-finding"
              >
                <div class="template-compliance-finding-head">
                  <div>
                    <div class="template-compliance-finding-pipeline">
                      {{ finding.pipeline_name || finding.pipeline_id || '-' }}
                    </div>
                    <div class="template-compliance-finding-rule">
                      {{ finding.rule_name || finding.rule_code || '管线规范' }}
                    </div>
                  </div>
                  <div class="template-compliance-finding-tags">
                    <a-tag :color="complianceSeverityColor(finding.severity)">
                      {{ complianceSeverityText(finding.severity) }}
                    </a-tag>
                    <a-tag>{{ complianceFindingLineText(finding) }}</a-tag>
                  </div>
                </div>
                <div v-if="finding.message" class="template-compliance-finding-message">
                  {{ finding.message }}
                </div>
                <div v-if="finding.suggestion" class="template-compliance-finding-suggestion">
                  建议：{{ finding.suggestion }}
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </a-modal>

    <a-modal
      :open="modalVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleTemplateEditorAfterClose"
      :mask-style="templateEditorMaskStyle"
      :wrap-props="templateEditorWrapProps"
      wrap-class-name="template-editor-modal-wrap"
      @cancel="closeModal"
    >
      <template #title>
        <div class="template-editor-modal-titlebar">
          <span class="template-editor-modal-title-text">{{ modalTitle }}</span>
          <a-button class="application-toolbar-action-btn template-editor-modal-save-btn" :loading="submitting" @click="submitForm">
            保存
          </a-button>
        </div>
      </template>
      <a-spin :spinning="modalLoading">
        <a-form ref="formRef" :model="formState" layout="vertical" :required-mark="false">
        <a-card class="scope-card scope-card-base" :bordered="false">
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item label="模板名称" name="name" :rules="[{ required: true, message: '请输入模板名称' }]">
                <a-input v-model:value="formState.name" allow-clear placeholder="例如：标准 CI + CD 发布模板" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="状态" name="status" :rules="[{ required: true, message: '请选择状态' }]">
                <a-select v-model:value="formState.status" :options="statusOptions" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item label="应用" name="application_id" :rules="[{ required: true, message: '请选择应用' }]">
                <a-select
                  v-model:value="formState.application_id"
                  :disabled="modalMode === 'edit'"
                  show-search
                  option-filter-prop="label"
                  placeholder="请选择应用"
                  :options="applicationOptions"
                  @change="handleApplicationChange"
                />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="备注" name="remark">
                <a-input v-model:value="formState.remark" allow-clear placeholder="可选，补充模板用途说明" />
              </a-form-item>
            </a-col>
          </a-row>
        </a-card>

        <a-row :gutter="16">
          <a-col :xs="24" :lg="12">
            <a-card class="scope-card" :bordered="true">
              <template #title>
                {{ scopeTitles.ci }}
                <a-popover
                  trigger="click"
                  placement="rightTop"
                  overlay-class-name="release-tip-popover"
                >
                  <template #content>
                    <div class="release-tip-content">{{ scopeDescriptions.ci }}</div>
                  </template>
                  <button
                    class="release-tip-trigger release-tip-trigger-info"
                    type="button"
                    aria-label="查看 CI 说明"
                  >
                    <ExclamationCircleOutlined />
                  </button>
                </a-popover>
              </template>
              <template #extra>
                <a-switch :checked="scopeStates.ci.enabled" @change="(checked: boolean) => handleScopeEnabledChange('ci', checked)" />
              </template>

              <a-form-item label="CI 绑定管线" required>
                <a-select
                  v-model:value="scopeStates.ci.binding_id"
                  :disabled="!scopeStates.ci.enabled"
                  show-search
                  allow-clear
                  option-filter-prop="label"
                  placeholder="请选择 CI 绑定管线"
                  :loading="loadingBindings"
                  :options="bindingOptionsByScope.ci"
                  @change="(value: string | undefined) => handleScopeBindingChange('ci', value)"
                />
              </a-form-item>

              <a-alert
                v-if="scopeStates.ci.enabled && selectedBinding('ci')"
                class="scope-binding-alert"
                type="success"
                show-icon
                :message="`当前执行器：${selectedBinding('ci')?.provider}`"
              />
              <a-alert
                v-if="scopeBindingWarnings.ci"
                class="scope-binding-alert"
                type="warning"
                show-icon
                :message="scopeBindingWarnings.ci"
              />
              <a-alert
                v-if="scopeComplianceWarnings.ci"
                class="scope-binding-alert"
                type="warning"
                show-icon
                :message="scopeComplianceWarnings.ci"
              />

              <div class="scope-table-wrapper">
                <a-table
                  row-key="id"
                  size="small"
                  :columns="selectableParamColumns"
                  :data-source="scopeStates.ci.selectable_params"
                  :loading="scopeStates.ci.loading_params"
                  :pagination="false"
                  :row-selection="getRowSelection('ci')"
                  :scroll="{ x: 860, y: 300 }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'param_name'">
                      <div class="param-name-cell">
                        <span class="param-name">{{ resolvePlatformParamName(record.param_key) }}</span>
                        <span class="param-executor">{{ record.executor_param_name }}</span>
                      </div>
                    </template>
                    <template v-else-if="column.key === 'required'">
                      <a-tag :class="record.required ? 'dashboard-chip dashboard-chip-warning' : 'dashboard-chip dashboard-chip-neutral'">
                        {{ record.required ? '必填' : '可选' }}
                      </a-tag>
                    </template>
                  </template>
                </a-table>
              </div>

              <div v-if="selectedScopeParamDefs('ci').length" class="template-param-config-panel">
                <div class="template-param-config-header">
                  <div class="template-param-config-title">发布基础字段与 CI 管线字段映射</div>
                  <div class="template-param-config-subtitle">已选择的平台标准字段可在发布时填写、写死固定值，或直接映射发布基础字段</div>
                </div>
                <div
                  v-for="item in selectedScopeParamDefs('ci')"
                  :key="`ci-config-${item.id}`"
                  class="template-param-config-item"
                >
                  <div class="template-param-config-item-header">
                    <div>
                      <div class="template-param-config-item-title">{{ resolvePlatformParamName(String(item.param_key || '').trim().toLowerCase()) }}</div>
                      <div class="template-param-config-item-meta">{{ item.executor_param_name }}</div>
                    </div>
                    <a-tag class="dashboard-chip dashboard-chip-neutral">
                      {{ resolveTemplateParamSourceLabel('ci', getTemplateParamConfig('ci', item.id, item)) }}
                    </a-tag>
                  </div>
                  <a-row :gutter="[12, 12]" class="template-param-config-grid">
                    <a-col v-if="isBuiltinMappedExecutorParam(item)" class="template-param-value-col" :span="24">
                      <a-form-item label="取值方式" class="template-param-inline-item">
                        <div class="template-param-builtin-lock">
                          已映射内置字段 {{ resolveBuiltinMappedExecutorParamLabel(item) }}，发布链路自动赋值
                        </div>
                      </a-form-item>
                    </a-col>
                    <a-col v-else class="template-param-source-col" :span="24">
                      <a-form-item label="取值方式" class="template-param-inline-item">
                        <a-segmented
                          :value="getTemplateParamConfig('ci', item.id, item).value_source"
                          :options="[
                            { label: '发布时填写', value: 'release_input' },
                            { label: '固定值', value: 'fixed' },
                            { label: '发布基础字段', value: 'builtin' },
                          ]"
                          @change="(value: string | number) => handleTemplateParamValueSourceChange('ci', item.id, String(value) as ReleaseTemplateParamValueSource)"
                        />
                      </a-form-item>
                    </a-col>
                    <a-col
                      v-if="!isBuiltinMappedExecutorParam(item) && getTemplateParamConfig('ci', item.id, item).value_source === 'fixed'"
                      class="template-param-value-col"
                      :span="24"
                    >
                      <a-form-item label="固定值" class="template-param-inline-item">
                        <a-input
                          :value="getTemplateParamConfig('ci', item.id, item).fixed_value"
                          placeholder="请输入模板固定值"
                          @update:value="(value: string) => (getTemplateParamConfig('ci', item.id, item).fixed_value = value)"
                        />
                      </a-form-item>
                    </a-col>
                    <a-col
                      v-else-if="!isBuiltinMappedExecutorParam(item) && getTemplateParamConfig('ci', item.id, item).value_source === 'builtin'"
                      class="template-param-value-col"
                      :span="24"
                    >
                      <a-form-item label="发布基础字段" class="template-param-inline-item">
                        <a-select
                          :value="getTemplateParamConfig('ci', item.id, item).source_param_key || undefined"
                          allow-clear
                          show-search
                          option-filter-prop="label"
                          placeholder="请选择发布基础字段"
                          :options="resolveTemplateParamSourceOptions('ci', getTemplateParamConfig('ci', item.id, item))"
                          @change="(value: string | undefined) => (getTemplateParamConfig('ci', item.id, item).source_param_key = String(value || '').trim().toLowerCase())"
                        />
                      </a-form-item>
                    </a-col>
                  </a-row>
                </div>
              </div>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="12">
            <a-card class="scope-card" :bordered="true">
              <template #title>
                {{ scopeTitles.cd }}
                <a-popover
                  trigger="click"
                  placement="rightTop"
                  overlay-class-name="release-tip-popover"
                >
                  <template #content>
                    <div class="release-tip-content">{{ scopeDescriptions.cd }}</div>
                  </template>
                  <button
                    class="release-tip-trigger release-tip-trigger-info"
                    type="button"
                    aria-label="查看 CD 说明"
                  >
                    <ExclamationCircleOutlined />
                  </button>
                </a-popover>
              </template>
              <template #extra>
                <a-switch :checked="scopeStates.cd.enabled" @change="(checked: boolean) => handleScopeEnabledChange('cd', checked)" />
              </template>

              <a-form-item label="CD 类型">
                <a-segmented
                  :value="cdMode"
                  :disabled="!scopeStates.cd.enabled"
                  :options="[
                    { label: '管线', value: 'pipeline' },
                    { label: 'ArgoCD', value: 'argocd' },
                  ]"
                  @change="handleCDModeChange"
                />
              </a-form-item>

              <a-form-item v-if="isCDUsingPipeline()" label="CD 绑定管线">
                <a-select
                  v-model:value="scopeStates.cd.binding_id"
                  :disabled="!scopeStates.cd.enabled"
                  show-search
                  allow-clear
                  option-filter-prop="label"
                  placeholder="请选择 CD 绑定管线"
                  :loading="loadingBindings"
                  :options="bindingOptionsByScope.cd"
                  @change="(value: string | undefined) => handleScopeBindingChange('cd', value)"
                />
              </a-form-item>
              <a-alert
                v-if="isCDUsingPipeline() && scopeBindingWarnings.cd"
                class="scope-binding-alert"
                type="warning"
                show-icon
                :message="scopeBindingWarnings.cd"
              />
              <a-alert
                v-if="isCDUsingPipeline() && scopeComplianceWarnings.cd"
                class="scope-binding-alert"
                type="warning"
                show-icon
                :message="scopeComplianceWarnings.cd"
              />

              <a-collapse v-if="isCDUsingArgoCD()" v-model:activeKey="argocdInfoActiveKeys" class="argocd-info-collapse" ghost>
                <a-collapse-panel key="argocd-info">
                  <template #header>
                    <div class="collapse-header-block">
                      <div class="collapse-header-title">当前模板的 CD 方式为 ArgoCD</div>
                      <div class="collapse-header-subtitle">
                        {{ isHelmGitOps() ? 'GitOps 类型：Helm' : 'GitOps 类型：Kustomize' }}
                      </div>
                    </div>
                  </template>
                  <div class="argocd-info-panel">
                    {{ resolveArgoCDModeDescription() }}
                  </div>
                </a-collapse-panel>
              </a-collapse>
              <a-alert
                v-else-if="isCDUsingPipeline() && selectedBinding('cd')"
                class="scope-binding-alert"
                type="success"
                show-icon
                :message="`当前执行器：${selectedBinding('cd')?.provider}`"
              />

              <div v-if="isCDUsingPipeline() && selectedBinding('cd')" class="scope-table-wrapper">
                <a-table
                  row-key="id"
                  size="small"
                  :columns="selectableParamColumns"
                  :data-source="scopeStates.cd.selectable_params"
                  :loading="scopeStates.cd.loading_params"
                  :pagination="false"
                  :row-selection="getRowSelection('cd')"
                  :scroll="{ x: 860, y: 300 }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'param_name'">
                      <div class="param-name-cell">
                        <span class="param-name">{{ resolvePlatformParamName(record.param_key) }}</span>
                        <span class="param-executor">{{ record.executor_param_name }}</span>
                      </div>
                    </template>
                    <template v-else-if="column.key === 'required'">
                      <a-tag :class="record.required ? 'dashboard-chip dashboard-chip-warning' : 'dashboard-chip dashboard-chip-neutral'">
                        {{ record.required ? '必填' : '可选' }}
                      </a-tag>
                    </template>
                  </template>
                </a-table>
              </div>

              <div v-if="isCDUsingPipeline() && selectedScopeParamDefs('cd').length" class="template-param-config-panel">
                <div class="template-param-config-header">
                  <div class="template-param-config-title">发布基础字段与 CD 管线字段映射</div>
                  <div class="template-param-config-subtitle">
                    CD 为管线时，参数可配置固定值，或沿用 CI 标准字段、发布基础字段CD 自填字段在这里作为普通标准字段使用
                  </div>
                </div>
                <div
                  v-for="item in selectedScopeParamDefs('cd')"
                  :key="`cd-config-${item.id}`"
                  class="template-param-config-item"
                >
                  <div class="template-param-config-item-header">
                    <div>
                      <div class="template-param-config-item-title">{{ resolvePlatformParamName(String(item.param_key || '').trim().toLowerCase()) }}</div>
                      <div class="template-param-config-item-meta">{{ item.executor_param_name }}</div>
                    </div>
                    <a-tag class="dashboard-chip dashboard-chip-neutral">
                      {{ resolveTemplateParamSourceLabel('cd', getTemplateParamConfig('cd', item.id, item)) }}
                    </a-tag>
                  </div>
                  <a-row :gutter="[12, 12]" class="template-param-config-grid">
                    <a-col v-if="isBuiltinMappedExecutorParam(item)" class="template-param-value-col" :span="24">
                      <a-form-item label="取值方式" class="template-param-inline-item">
                        <div class="template-param-builtin-lock">
                          已映射内置字段 {{ resolveBuiltinMappedExecutorParamLabel(item) }}，发布链路自动赋值
                        </div>
                      </a-form-item>
                    </a-col>
                    <a-col v-else class="template-param-source-col" :span="24">
                      <a-form-item label="取值方式" class="template-param-inline-item">
                        <a-segmented
                          :value="getTemplateParamConfig('cd', item.id, item).value_source"
                          :options="[
                            { label: '发布时填写', value: 'release_input' },
                            { label: '固定值', value: 'fixed' },
                            { label: '沿用 CI 字段', value: 'ci_param' },
                            { label: '发布基础字段', value: 'builtin' },
                          ]"
                          @change="(value: string | number) => handleTemplateParamValueSourceChange('cd', item.id, String(value) as ReleaseTemplateParamValueSource)"
                        />
                      </a-form-item>
                    </a-col>
                    <a-col
                      v-if="!isBuiltinMappedExecutorParam(item) && getTemplateParamConfig('cd', item.id, item).value_source === 'fixed'"
                      class="template-param-value-col"
                      :span="24"
                    >
                      <a-form-item label="固定值" class="template-param-inline-item">
                        <a-input
                          :value="getTemplateParamConfig('cd', item.id, item).fixed_value"
                          placeholder="请输入模板固定值"
                          @update:value="(value: string) => (getTemplateParamConfig('cd', item.id, item).fixed_value = value)"
                        />
                      </a-form-item>
                    </a-col>
                    <a-col
                      v-else-if="!isBuiltinMappedExecutorParam(item) && ['ci_param', 'builtin'].includes(getTemplateParamConfig('cd', item.id, item).value_source)"
                      class="template-param-value-col"
                      :span="24"
                    >
                      <a-form-item :label="getTemplateParamConfig('cd', item.id, item).value_source === 'ci_param' ? 'CI 来源字段' : '发布基础字段'" class="template-param-inline-item">
                        <a-select
                          :value="getTemplateParamConfig('cd', item.id, item).source_param_key || undefined"
                          allow-clear
                          show-search
                          option-filter-prop="label"
                          placeholder="请选择来源字段"
                          :options="resolveTemplateParamSourceOptions('cd', getTemplateParamConfig('cd', item.id, item))"
                          @change="(value: string | undefined) => (getTemplateParamConfig('cd', item.id, item).source_param_key = String(value || '').trim().toLowerCase())"
                        />
                      </a-form-item>
                    </a-col>
                  </a-row>
                </div>
              </div>

              <div v-if="isCDUsingArgoCD()" class="gitops-rule-panel">
                <a-form-item label="GitOps 类型" class="gitops-type-item">
                  <a-segmented
                    v-model:value="gitOpsType"
                    :options="[
                      { label: 'Kustomize', value: 'kustomize' },
                      { label: 'Helm', value: 'helm' },
                    ]"
                  />
                </a-form-item>

                <a-result
                  v-if="isUnsupportedKustomizeGitOps()"
                  class="gitops-unsupported-result"
                  status="info"
                  title="Kustomize 模式正在开发中"
                  sub-title="当前请先使用 Helm 模式配置 GitOps 规则，Kustomize 支持即将上线"
                />

                <template v-if="!isUnsupportedKustomizeGitOps()">
                <a-alert
                  v-if="gitopsScanPathExists === false"
                  class="gitops-kustomize-note"
                  type="warning"
                  show-icon
                  message="GitOps 读取目录不存在"
                >
                  <template #description>
                    <div>
                      <p style="margin:0 0 4px"><strong>路径：</strong><code>{{ gitopsScanPathTemplate }}</code></p>
                      <p style="margin:0">请检查系统设置中的 GitOps 读取目录配置，或确认 GitOps 仓库 master 分支中该目录已创建</p>
                    </div>
                  </template>
                </a-alert>

                <div class="gitops-rule-header">
                  <div>
                    <div class="gitops-rule-title">GitOps 替换规则</div>
                    <div class="gitops-rule-subtitle">
                      {{
                        isHelmGitOps()
                          ? '先选可引用的标准字段，再直接下拉选择平台专用 values 文件中的路径；支持 CI 已勾选字段、发布基础字段，以及只在 CD 阶段填写固定值的 CD 自填字段'
                          : '先选可引用的标准字段，再直接下拉选择目标文件、资源和字段；支持 CI 已勾选字段、发布基础字段，以及只在 CD 阶段填写固定值的 CD 自填字段'
                      }}
                    </div>
                    <div class="gitops-rule-note">候选路径统一从 GitOps 仓库 master 分支读取，作为模板配置基线；实际发布仍按应用命中的目标分支执行</div>
                  </div>
                  <a-tag class="dashboard-chip dashboard-chip-running">{{ gitopsRules.length }} 条规则</a-tag>
                </div>

                <div class="gitops-rule-toolbar">
                  <a-space>
                    <a-button size="small" :loading="loadingGitOpsFieldCandidates" @click="reloadCurrentGitOpsCandidates">
                      <template #icon><ReloadOutlined /></template>
                      {{ isHelmGitOps() ? '同步 Values' : '同步字段' }}
                    </a-button>
                    <a-button type="dashed" size="small" @click="addGitOpsRule">新增规则</a-button>
                  </a-space>
                </div>

                <a-empty
                  v-if="!loadingGitOpsFieldCandidates && ((isHelmGitOps() && platformValuesCandidates().length === 0) || (!isHelmGitOps() && gitOpsFieldCandidates.length === 0))"
                  :description="isHelmGitOps()
                    ? '当前应用还没有扫描到平台专用的 Helm values 路径，请先确认 GitOps 目录下已准备好 apps/helm/platform.values-{env}.yaml'
                    : '当前应用还没有扫描到可替换的 YAML 字段，请先确认 GitOps 目录与 YAML 文件是否已准备好'"
                />

                <a-collapse v-model:activeKey="gitopsRuleActiveKeys" class="gitops-rule-collapse" accordion>
                  <a-collapse-panel v-for="rule in gitopsRules" :key="rule.local_id">
                    <template #header>
                      <div class="collapse-header-block">
                        <div class="collapse-header-title">
                          规则 {{ gitopsRules.findIndex((item) => item.local_id === rule.local_id) + 1 }}：{{ formatGitOpsRulePanelTitle(rule) }}
                        </div>
                        <div class="collapse-header-subtitle">
                          {{ formatGitOpsRulePanelDescription(rule) }}
                        </div>
                      </div>
                    </template>
                    <template #extra>
                      <a-button danger type="link" @click.stop="removeGitOpsRule(rule.local_id)">删除</a-button>
                    </template>

                    <div class="gitops-rule-item">
                      <a-row :gutter="12">
                        <a-col :span="24">
                          <a-form-item label="标准字段">
                            <a-select
                              :value="rule.source_param_key || undefined"
                              show-search
                              allow-clear
                              option-filter-prop="label"
                              placeholder="请选择 CI 已勾选字段、发布基础字段或 CD 自填字段"
                              :options="gitOpsSourceOptions"
                              @change="(value: string | undefined) => handleGitOpsRuleSourceChange(rule, value)"
                            />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <div class="gitops-rule-source-tip">
                        当前来源：{{ resolveGitOpsSourceLabel(rule.source_param_key) }}
                      </div>

                      <div class="gitops-target-preview">
                        <div class="gitops-target-preview-header">
                          <div class="gitops-target-preview-title">{{ isHelmGitOps() ? 'Values 目标路径' : 'YAML 目标字段' }}</div>
                          <a-tag :color="formatGitOpsRuleTargetSummary(rule).stale ? 'error' : 'processing'">
                            {{ formatGitOpsRuleTargetSummary(rule).stale ? '字段已变化' : '字段有效' }}
                          </a-tag>
                        </div>
                        <a-row v-if="isHelmGitOps()" :gutter="12" class="gitops-target-select-row">
                          <a-col :span="24">
                            <a-form-item label="目标文件模板" class="gitops-inline-item">
                              <a-input :value="rule.file_path_template || valuesFileOptions()[0]?.value || 'apps/helm/platform.values-{env}.yaml'" readonly />
                            </a-form-item>
                          </a-col>
                          <a-col :span="24">
                            <a-form-item label="Values 路径" class="gitops-inline-item">
                              <a-select
                                :value="rule.target_path ? JSON.stringify({ file_path_template: rule.file_path_template, target_path: rule.target_path }) : undefined"
                                allow-clear
                                show-search
                                option-filter-prop="label"
                                placeholder="请选择 Values 路径"
                                :options="valuesPathOptions(rule)"
                                @change="(value: string | undefined) => handleValuesTargetPathChange(rule, value)"
                              />
                            </a-form-item>
                          </a-col>
                        </a-row>
                        <a-row v-else :gutter="12" class="gitops-target-select-row">
                          <a-col :span="8">
                            <a-form-item label="目标文件" class="gitops-inline-item">
                              <a-select
                                :value="rule.file_path_template || undefined"
                                allow-clear
                                show-search
                                option-filter-prop="label"
                                placeholder="请选择 YAML 文件"
                                :options="yamlFileOptions(rule)"
                                @change="(value: string | undefined) => handleYamlFileTemplateChange(rule, value)"
                              />
                            </a-form-item>
                          </a-col>
                          <a-col :span="8">
                            <a-form-item label="目标资源" class="gitops-inline-item">
                              <a-select
                                :value="selectedYamlDocumentValue(rule)"
                                allow-clear
                                show-search
                                option-filter-prop="label"
                                placeholder="请选择资源"
                                :options="yamlDocumentOptions(rule)"
                                :disabled="!rule.file_path_template"
                                @change="(value: string | undefined) => handleYamlDocumentChange(rule, value)"
                              />
                            </a-form-item>
                          </a-col>
                          <a-col :span="8">
                            <a-form-item label="目标字段" class="gitops-inline-item">
                              <a-select
                                :value="rule.target_path || undefined"
                                allow-clear
                                show-search
                                option-filter-prop="label"
                                placeholder="请选择字段路径"
                                :options="yamlFieldOptions(rule)"
                                :disabled="!rule.file_path_template || !rule.document_kind"
                                @change="(value: string | undefined) => handleYamlTargetPathChange(rule, value)"
                              />
                            </a-form-item>
                          </a-col>
                        </a-row>
                        <a-descriptions :column="1" size="small" bordered>
                          <a-descriptions-item :label="isHelmGitOps() ? '类型' : '资源'">
                            {{ formatGitOpsRuleTargetSummary(rule).title }}
                          </a-descriptions-item>
                          <a-descriptions-item :label="isHelmGitOps() ? 'Values 文件模板' : '文件'">
                            <span class="gitops-code-text">{{ formatGitOpsRuleTargetSummary(rule).file }}</span>
                          </a-descriptions-item>
                          <a-descriptions-item :label="isHelmGitOps() ? 'Values 路径' : '字段路径'">
                            <span class="gitops-code-text">{{ formatGitOpsRuleTargetSummary(rule).path }}</span>
                          </a-descriptions-item>
                          <a-descriptions-item label="当前示例值">
                            <span class="gitops-code-text">{{ formatGitOpsRuleTargetSummary(rule).sample }}</span>
                          </a-descriptions-item>
                        </a-descriptions>
                      </div>

                      <a-form-item label="取值模版">
                        <a-input
                          v-model:value="rule.value_template"
                          allow-clear
                          :placeholder="resolveGitOpsValueTemplatePlaceholder(rule)"
                        />
                      </a-form-item>
                    </div>
                  </a-collapse-panel>
                </a-collapse>
                </template>
              </div>

            </a-card>
          </a-col>
        </a-row>

        <a-card class="scope-card hook-config-card" :bordered="true">
          <template #title>
            发布后 Hook
            <a-popover
              trigger="click"
              placement="rightTop"
              overlay-class-name="release-tip-popover"
            >
              <template #content>
                <div class="release-tip-content">
                  发布模板中的 Hook 可配置在构建完成或发布完成后串行执行，通知 Hook 会自动使用发布过程中的标准平台 Key 和内置字段渲染消息
                </div>
              </template>
              <button
                class="release-tip-trigger release-tip-trigger-info"
                type="button"
                aria-label="查看发布后 Hook 说明"
              >
                <ExclamationCircleOutlined />
              </button>
            </a-popover>
          </template>
          <template #extra>
            <a-space>
              <a-tag class="dashboard-chip dashboard-chip-running">多阶段执行</a-tag>
              <a-button type="dashed" size="small" @click="openHookTypePicker">
                <template #icon><PlusOutlined /></template>
                新增 Hook
              </a-button>
            </a-space>
          </template>

          <div class="hook-template-summary-grid">
            <div v-for="item in hookSummaryItems" :key="item.label" class="hook-template-summary-item">
              <div class="hook-template-summary-label">{{ item.label }}</div>
              <div class="hook-template-summary-value">{{ item.value }}</div>
            </div>
          </div>

          <div class="hook-template-capability-header">
            <div>
              <div class="hook-template-capability-title">Hook 配置适配预览</div>
              <div class="hook-template-capability-subtitle">
                点击新增 Hook，先选择类型，再补充表单字段Agent 类 Hook 只需选择<strong>临时任务名称</strong>；任务在「Agent 任务管理」里绑定 Agent 后，发布时会自动向各 Agent 派发
              </div>
            </div>
            <a-tag class="dashboard-chip dashboard-chip-neutral">详情页直接看进度</a-tag>
          </div>

          <a-empty v-if="!templateHooks.length" description="暂未配置发布后 Hook，可点击右上角新增 Hook" />

          <div v-else class="hook-template-capability-grid">
            <div v-for="(item, index) in templateHooks" :key="item.local_id" class="hook-template-capability-card">
              <div class="hook-template-capability-card-head">
                <div>
                  <div class="hook-template-capability-card-title">
                    Hook {{ index + 1 }}：{{ item.name || '未命名 Hook' }}
                  </div>
                  <div class="hook-template-capability-card-meta">
                    {{ hookTypeLabel(item.hook_type) }} ·
                    {{ item.hook_type === 'agent_task' ? 'Agent 任务 Hook' : item.hook_type === 'notification_hook' ? '通知 Hook' : 'Webhook 通知 Hook' }}
                  </div>
                </div>
                <a-space>
                  <span class="hook-template-order-actions">
                    <a-button size="small" :disabled="index === 0" @click="moveHook(index, -1)">
                      <template #icon><ArrowUpOutlined /></template>
                      上移
                    </a-button>
                    <a-button size="small" :disabled="index === templateHooks.length - 1" @click="moveHook(index, 1)">
                      <template #icon><ArrowDownOutlined /></template>
                      下移
                    </a-button>
                  </span>
                  <a-tag class="dashboard-chip dashboard-chip-neutral">{{ hookTypeLabel(item.hook_type) }}</a-tag>
                  <a-button type="link" danger size="small" @click="removeHook(item.local_id)">删除</a-button>
                </a-space>
              </div>

              <div class="hook-template-form-stack">
                <a-form-item label="Hook 名称" class="template-param-inline-item">
                  <a-input
                    v-model:value="item.name"
                    allow-clear
                    :placeholder="item.hook_type === 'agent_task'
                      ? '例如：构建/发布后 Agent 任务'
                      : item.hook_type === 'notification_hook'
                        ? '例如：构建/发布通知 Hook'
                        : '例如：构建/发布 Webhook 通知'"
                  />
                </a-form-item>

                <a-form-item label="执行阶段" class="template-param-inline-item">
                  <a-checkbox-group
                    :value="item.execute_stages"
                    :options="hookExecuteStageOptions"
                    class="hook-stage-checkbox-group"
                    @update:value="handleHookStageChange(item, $event)"
                  />
                </a-form-item>

                <a-form-item label="触发条件" class="template-param-inline-item">
                  <a-segmented
                    v-model:value="item.trigger_condition"
                    :options="hookTriggerOptions(item)"
                  />
                </a-form-item>

                <a-form-item label="失败策略" class="template-param-inline-item">
                  <a-segmented
                    v-model:value="item.failure_policy"
                    :options="[
                      { label: '阻断发布单', value: 'block_release' },
                      { label: '仅告警', value: 'warn_only' },
                    ]"
                  />
                </a-form-item>

                <a-form-item label="执行环境" class="template-param-inline-item">
                  <a-select
                    v-model:value="item.env_codes"
                    mode="multiple"
                    allow-clear
                    show-search
                    option-filter-prop="label"
                    :loading="loadingHookEnvOptions"
                    :options="mergeHookEnvOptions(item.env_codes)"
                    placeholder="留空则所有环境执行，例如仅选 prod"
                  />
                </a-form-item>

                <a-form-item :label="item.hook_type === 'agent_task' ? '临时任务' : item.hook_type === 'notification_hook' ? '目标通知 Hook' : 'Webhook URL'" class="template-param-inline-item">
                  <a-select
                    v-if="item.hook_type === 'agent_task'"
                    v-model:value="item.target_id"
                    allow-clear
                    show-search
                    option-filter-prop="label"
                    :loading="loadingAgentTaskTemplates"
                    :options="agentTaskTemplateOptions"
                    placeholder="按任务名称选择；发布时按该任务绑定的 Agent 派发"
                    @change="syncHookTargetName(item)"
                  />
                  <a-select
                    v-else-if="item.hook_type === 'notification_hook'"
                    v-model:value="item.target_id"
                    allow-clear
                    show-search
                    option-filter-prop="label"
                    :loading="loadingNotificationHooks"
                    :options="notificationHookOptions"
                    placeholder="请选择通知 Hook"
                    @change="syncHookTargetName(item)"
                  />
                  <a-input
                    v-else
                    v-model:value="item.webhook_url"
                    allow-clear
                    placeholder="例如：https://notify.example.com/release/hook"
                  />
                </a-form-item>

                <div v-if="item.hook_type === 'agent_task' && !agentTaskTemplateOptions.length" class="hook-template-capability-note">
                  请先在「Agent 任务管理」中创建<strong>临时任务</strong>，并在该任务上绑定一台或多台 Agent模板里只需选任务名称；发布触发 Hook 时会按绑定关系向各 Agent 派发，无需为每个 Agent 单独加 Hook
                </div>
                <div v-if="item.hook_type === 'notification_hook' && !notificationHookOptions.length" class="hook-template-capability-note">
                  当前还没有可引用的通知 Hook，请先到系统管理 / 通知模块中创建通知源、Markdown 模板和通知 Hook
                </div>

                <a-descriptions v-if="item.hook_type === 'agent_task' && findAgentTaskTemplate(item.target_id)" :column="1" size="small" bordered class="hook-template-description">
                  <a-descriptions-item label="已选任务">
                    {{ findAgentTaskTemplate(item.target_id)?.name }}
                  </a-descriptions-item>
                  <a-descriptions-item label="任务模式">
                    临时任务（发布时按下方绑定关系派发）
                  </a-descriptions-item>
                  <a-descriptions-item label="绑定 Agent">
                    {{
                      (findAgentTaskTemplate(item.target_id)?.target_agent_ids || []).length
                        ? `已绑定 ${(findAgentTaskTemplate(item.target_id)?.target_agent_ids || []).length} 台 Agent（发布时各 Agent 各执行一次）`
                        : findAgentTaskTemplate(item.target_id)?.agent_code
                          ? `单 Agent：${findAgentTaskTemplate(item.target_id)?.agent_code}`
                          : '未绑定 Agent，请回到任务管理中配置'
                    }}
                  </a-descriptions-item>
                  <a-descriptions-item label="任务类型">
                    {{ agentTaskTypeLabel(String(findAgentTaskTemplate(item.target_id)?.task_type || '')) }}
                  </a-descriptions-item>
                  <a-descriptions-item label="脚本">
                    {{ findAgentTaskTemplate(item.target_id)?.script_name || findAgentTaskTemplate(item.target_id)?.script_path || '未绑定脚本' }}
                  </a-descriptions-item>
                  <a-descriptions-item label="脚本预览">
                    <div class="hook-template-script-preview-wrap">
                      <pre class="hook-template-script-preview">{{ summarizeAgentTaskScript(item.target_id) }}</pre>
                      <a-button size="small" type="link" @click="loadAgentTaskTemplates(true)">
                        <template #icon><ReloadOutlined /></template>
                        刷新任务快照
                      </a-button>
                    </div>
                  </a-descriptions-item>
                </a-descriptions>

                <a-descriptions v-if="item.hook_type === 'notification_hook' && notificationHooks.find((candidate) => candidate.id === item.target_id)" :column="1" size="small" bordered class="hook-template-description">
                  <a-descriptions-item label="已选通知 Hook">
                    {{ notificationHooks.find((candidate) => candidate.id === item.target_id)?.name }}
                  </a-descriptions-item>
                  <a-descriptions-item label="通知源">
                    {{ notificationHooks.find((candidate) => candidate.id === item.target_id)?.source_name }}
                  </a-descriptions-item>
                  <a-descriptions-item label="Markdown 模板">
                    {{ notificationHooks.find((candidate) => candidate.id === item.target_id)?.markdown_template_name }}
                  </a-descriptions-item>
                  <a-descriptions-item label="变量来源">
                    使用发布过程中的标准平台 Key 与内置字段渲染通知内容
                  </a-descriptions-item>
                </a-descriptions>

                <a-form-item v-if="item.hook_type === 'webhook_notification'" label="请求方法" class="template-param-inline-item">
                  <a-select
                    v-model:value="item.webhook_method"
                    :options="[
                      { label: 'POST', value: 'POST' },
                      { label: 'PUT', value: 'PUT' },
                      { label: 'PATCH', value: 'PATCH' },
                    ]"
                  />
                </a-form-item>

                <a-form-item v-if="item.hook_type === 'webhook_notification'" label="Body 模板" class="template-param-inline-item">
                  <a-textarea
                    v-model:value="item.webhook_body_template"
                    :auto-size="{ minRows: 4, maxRows: 8 }"
                    placeholder="请输入 Webhook body 模板，支持 {env}、{order_no} 等变量"
                  />
                </a-form-item>

                <a-form-item label="补充说明" class="template-param-inline-item">
                  <a-textarea
                    v-model:value="item.note"
                    :auto-size="{ minRows: 2, maxRows: 4 }"
                    placeholder="例如：用于发版成功通知、回滚告警或特殊环境后置动作"
                  />
                </a-form-item>

                <div class="hook-template-variable-row">
                  <div class="hook-template-variable-title">变量来源</div>
                  <div class="hook-template-variable-tags">
                    <a-tag v-for="source in hookVariableSourceTags" :key="`${item.local_id}-${source}`" class="dashboard-chip dashboard-chip-neutral">
                      {{ source }}
                    </a-tag>
                  </div>
                </div>

                <a-descriptions :column="1" size="small" bordered class="hook-template-description">
                  <a-descriptions-item label="当前摘要">
                    {{ hookStageLabels(item.execute_stages).join(' / ') }} · {{ hookTriggerLabel(item.trigger_condition, item.execute_stages) }} · {{ hookFailureLabel(item.failure_policy) }} · {{ hookEnvLabel(item.env_codes) }}
                  </a-descriptions-item>
                  <a-descriptions-item label="详情展示">
                    发布单详情直接展示 Hook 执行进度、变量和日志
                  </a-descriptions-item>
                </a-descriptions>
              </div>
            </div>
          </div>
        </a-card>
        <a-modal
          :open="hookTypePickerVisible"
          title="新增 Hook"
          :width="420"
          wrap-class-name="hook-type-picker-modal-wrap"
          ok-text="添加"
          cancel-text="取消"
          @ok="confirmAddHook"
          @cancel="hookTypePickerVisible = false"
        >
          <a-form layout="vertical" class="hook-type-picker-form">
            <a-form-item name="hookType">
              <template #label>
                Hook 类型
                <a-popover
                  trigger="click"
                  placement="rightTop"
                  overlay-class-name="release-tip-popover"
                >
                  <template #content>
                    <div class="release-tip-content">
                      {{ pendingHookType === 'agent_task'
                        ? '新增后会补充 Agent 任务名称、触发条件、失败策略等字段'
                        : pendingHookType === 'notification_hook'
                          ? '新增后会选择通知 Hook，通知 Hook 由通知源和 Markdown 模板组成'
                          : '新增后会补充 Webhook URL、请求方法、Body 模板等字段' }}
                    </div>
                  </template>
                  <button
                    class="release-tip-trigger release-tip-trigger-info"
                    type="button"
                    aria-label="查看 Hook 类型说明"
                  >
                    <ExclamationCircleOutlined />
                  </button>
                </a-popover>
              </template>
              <a-segmented
                v-model:value="pendingHookType"
                :options="[
                  { label: 'Agent 任务', value: 'agent_task' },
                  { label: '通知 Hook', value: 'notification_hook' },
                  { label: 'Webhook 通知', value: 'webhook_notification' },
                ]"
              />
            </a-form-item>
          </a-form>
        </a-modal>
        </a-form>
      </a-spin>
    </a-modal>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.release-template-page-header {
  flex-wrap: wrap;
  padding: 0 !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
}

.release-template-header-actions {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  min-width: min(100%, 520px);
}

:deep(.release-template-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.release-template-toolbar-select-wide.ant-select) {
  min-width: min(280px, 100%);
}

:deep(.release-template-toolbar-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px;
  min-height: 42px;
  border-radius: 16px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.release-template-toolbar-select .ant-select-selection-item),
:deep(.release-template-toolbar-select .ant-select-arrow) {
  color: #1e3a8a;
  font-weight: 650;
}

:deep(.release-template-toolbar-select .ant-select-selection-item) {
  display: flex;
  align-items: center;
  height: 100%;
  line-height: 1 !important;
}

:deep(.release-template-toolbar-select .ant-select-selection-placeholder) {
  display: flex;
  align-items: center;
  height: 100%;
  color: rgba(30, 58, 138, 0.38) !important;
  font-weight: 600;
  line-height: 1 !important;
}

:deep(.release-template-toolbar-select .ant-select-selection-search) {
  inset-block-start: 0 !important;
  inset-block-end: 0 !important;
}

:deep(.release-template-toolbar-select .ant-select-selection-search-input) {
  height: 100% !important;
  color: #1e3a8a;
  font-weight: 650;
  line-height: 42px !important;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.release-template-toolbar-query-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 42px;
  border-radius: 16px;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #0f172a !important;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.release-template-toolbar-query-btn.ant-btn:hover),
:deep(.release-template-toolbar-query-btn.ant-btn:focus),
:deep(.release-template-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
}

.release-template-table-section {
  margin-top: 24px;
}

.release-template-table :deep(.ant-table) {
  background: transparent;
}

.release-template-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.34);
}

.release-template-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.release-template-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.release-template-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: var(--color-text-main);
}

.release-template-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.release-template-table :deep(.ant-table-cell-fix-right) {
  background: rgba(255, 255, 255, 0.96) !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.04);
}

.release-template-table :deep(.ant-table-thead .ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  box-shadow: none;
}

.scope-card {
  border-radius: var(--radius-xl);
}

.template-editor-modal-wrap :deep(.ant-modal-content) {
  position: relative;
  overflow: hidden;
  isolation: isolate;
  border-radius: 24px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.08), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.08), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  box-shadow:
    0 32px 90px rgba(15, 23, 42, 0.18),
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    inset 0 -1px 0 rgba(255, 255, 255, 0.28);
  backdrop-filter: blur(18px) saturate(180%);
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

.template-editor-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.template-editor-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.template-editor-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.template-editor-modal-title-text {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.template-editor-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

.template-editor-modal-wrap :deep(.ant-modal-title) {
  color: #0f172a;
}

.template-editor-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

.hook-type-picker-modal-wrap :deep(.ant-modal) {
  width: min(420px, calc(100vw - 32px)) !important;
}

.hook-type-picker-modal-wrap :deep(.ant-modal-content) {
  border-radius: 18px;
}

.hook-type-picker-modal-wrap :deep(.ant-modal-body) {
  padding-top: 14px;
}

.hook-type-picker-form {
  width: min(100%, 360px);
  margin-right: auto;
}

.scope-card-base {
  margin-bottom: 16px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.99) 0%, rgba(248, 250, 252, 0.97) 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 28px rgba(15, 23, 42, 0.04);
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

.danger-icon {
  color: var(--color-danger);
}

.scope-alert,
.scope-binding-alert {
  margin-bottom: 12px;
}

.hook-stage-checkbox-group {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.scope-card {
  border: 1px solid rgba(148, 163, 184, 0.16);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.99) 0%, rgba(248, 250, 252, 0.97) 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 26px rgba(15, 23, 42, 0.03);
}

.scope-card :deep(.ant-card-head) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
}

.scope-card :deep(.ant-card-head-title) {
  color: var(--color-text-main);
  font-weight: 700;
}

.scope-alert:deep(.ant-alert) {
  border-radius: 14px;
}

.scope-alert:deep(.ant-alert-info) {
  border: 1px solid rgba(2, 132, 199, 0.14);
  background: linear-gradient(180deg, rgba(240, 249, 255, 0.98) 0%, rgba(248, 250, 252, 0.96) 100%);
}

.scope-alert:deep(.ant-alert-info .ant-alert-message),
.scope-alert:deep(.ant-alert-info .anticon) {
  color: #0369a1;
}

.scope-alert:deep(.ant-alert-info .ant-alert-description) {
  color: #475569;
}

.scope-binding-alert:deep(.ant-alert-success) {
  border: 1px solid rgba(22, 163, 74, 0.14);
  background: linear-gradient(180deg, rgba(240, 253, 244, 0.98) 0%, rgba(248, 250, 252, 0.96) 100%);
}

.scope-binding-alert:deep(.ant-alert-success .ant-alert-message),
.scope-binding-alert:deep(.ant-alert-success .anticon) {
  color: #15803d;
}

.scope-binding-alert:deep(.ant-alert-success .ant-alert-description) {
  color: #475569;
}

.collapse-header-block {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.collapse-header-title {
  font-weight: 600;
  color: var(--color-text-main);
}

.collapse-header-subtitle {
  color: var(--color-text-soft);
  font-size: 12px;
  line-height: 1.5;
}

.argocd-info-collapse,
.gitops-rule-collapse {
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98) 0%, rgba(248, 250, 252, 0.96) 100%);
}

.argocd-info-collapse {
  margin-bottom: 12px;
}

.argocd-info-panel {
  padding: 4px 0 0;
}

.argocd-info-collapse :deep(.ant-collapse-header),
.gitops-rule-collapse :deep(.ant-collapse-header) {
  align-items: flex-start !important;
  background: transparent !important;
}

.argocd-info-collapse :deep(.ant-collapse-content-box),
.gitops-rule-collapse :deep(.ant-collapse-content-box) {
  padding-top: 0 !important;
}

.argocd-info-panel {
  padding: 12px 14px;
  border: 1px solid rgba(2, 132, 199, 0.12);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(240, 249, 255, 0.98) 0%, rgba(248, 250, 252, 0.96) 100%);
  font-size: 13px;
  line-height: 1.8;
  color: var(--color-text-soft);
}

.scope-table-wrapper {
  margin-top: 12px;
  min-height: 320px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 14px;
  overflow: hidden;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.97) 100%);
}

.scope-table-wrapper :deep(.ant-table) {
  background: transparent;
}

.scope-table-wrapper :deep(.ant-table-container) {
  border-inline-start: none !important;
}

.scope-table-wrapper :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.98) 0%, rgba(219, 234, 254, 0.78) 100%);
  color: #334155;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
  font-weight: 700;
}

.scope-table-wrapper :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.scope-table-wrapper :deep(.ant-table-tbody > tr > td) {
  background: rgba(255, 255, 255, 0.88);
  border-bottom: 1px solid rgba(226, 232, 240, 0.86);
  color: var(--color-text-main);
}

.scope-table-wrapper :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(239, 246, 255, 0.72) !important;
}

.scope-table-wrapper :deep(.ant-table-row-selected > td) {
  background: rgba(219, 234, 254, 0.58) !important;
}

.scope-table-wrapper :deep(.ant-checkbox-wrapper),
.scope-table-wrapper :deep(.ant-checkbox + span) {
  color: var(--color-text-main);
}

.template-param-config-panel {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.template-param-config-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.template-param-config-title {
  font-weight: 700;
  color: var(--color-text-main);
}

.template-param-config-subtitle {
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-soft);
}

.template-param-config-item {
  padding: 14px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.97) 100%);
}

.template-param-config-item-header {
  margin-bottom: 12px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.template-param-config-grid {
  row-gap: 12px;
}

.template-param-source-col,
.template-param-value-col {
  min-width: 0;
}

.template-param-config-item :deep(.ant-segmented) {
  max-width: 100%;
}

.template-param-config-item :deep(.ant-segmented-group) {
  flex-wrap: wrap;
  row-gap: 4px;
}

.template-param-config-item :deep(.ant-segmented-item) {
  flex: 0 0 auto;
}

.template-param-config-item :deep(.ant-input),
.template-param-config-item :deep(.ant-select) {
  width: 100%;
}

.template-param-config-item-title {
  font-weight: 700;
  color: var(--color-text-main);
}

.template-param-config-item-meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--color-text-soft);
}

.template-param-inline-item {
  margin-bottom: 0;
}

.template-param-builtin-lock {
  min-height: 38px;
  display: flex;
  align-items: center;
  padding: 7px 12px;
  border: 1px solid rgba(59, 130, 246, 0.18);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.94) 0%, rgba(248, 250, 252, 0.96) 100%);
  color: #1e3a8a;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.5;
}

.hook-config-card {
  margin-top: 16px;
}

.hook-template-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.hook-template-summary-item {
  padding: 14px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.97) 100%);
}

.hook-template-summary-label {
  font-size: 12px;
  color: var(--color-text-soft);
}

.hook-template-summary-value {
  margin-top: 4px;
  font-weight: 700;
  color: var(--color-text-main);
}

.hook-template-capability-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.hook-template-capability-title {
  font-weight: 700;
  color: var(--color-text-main);
}

.hook-template-capability-subtitle {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-soft);
}

.hook-template-capability-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
}

.hook-template-capability-card {
  padding: 14px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.97) 100%);
}

.hook-template-form-stack {
  width: min(100%, 640px);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hook-template-capability-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.hook-template-capability-card-title {
  font-weight: 700;
  color: var(--color-text-main);
}

.hook-template-capability-card-meta {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-soft);
}

.hook-template-order-actions {
  display: inline-flex;
  gap: 6px;
}

.hook-template-order-actions :deep(.ant-btn) {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  border-color: rgba(148, 163, 184, 0.24);
  background: rgba(255, 255, 255, 0.7);
  color: #475569;
  font-weight: 600;
}

.hook-template-order-actions :deep(.ant-btn:not(:disabled):hover),
.hook-template-order-actions :deep(.ant-btn:not(:disabled):focus) {
  border-color: rgba(59, 130, 246, 0.34);
  background: rgba(239, 246, 255, 0.9);
  color: #1d4ed8;
}

.hook-template-variable-row {
  margin: 0;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.hook-template-variable-title {
  min-width: 64px;
  font-size: 12px;
  color: var(--color-text-soft);
}

.hook-template-variable-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.hook-template-capability-note {
  margin-bottom: 12px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--color-text-soft);
}

.hook-template-description {
  background: rgba(255, 255, 255, 0.66);
}

.hook-template-script-preview-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.hook-template-script-preview {
  width: 100%;
  margin: 0;
  padding: 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.92);
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  overflow: auto;
}

.gitops-rule-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.gitops-unsupported-result {
  margin-top: 8px;
  border: 1px dashed rgba(217, 119, 6, 0.35);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(255, 247, 237, 0.98) 0%, rgba(255, 251, 235, 0.96) 100%);
}

.gitops-unsupported-result:deep(.ant-result-title) {
  color: #9a3412;
}

.gitops-unsupported-result:deep(.ant-result-subtitle) {
  color: #7c5e10;
}

.gitops-kustomize-note {
  margin-bottom: 16px;
  border-radius: 0;
}

.gitops-rule-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.gitops-rule-toolbar {
  display: flex;
  justify-content: flex-end;
}

.gitops-rule-title {
  font-weight: 600;
  color: var(--color-text-main);
}

.gitops-rule-subtitle {
  font-size: 12px;
  color: var(--color-text-soft);
}

.gitops-rule-note {
  margin-top: 6px;
  font-size: 12px;
  color: var(--color-dashboard-800);
}

.gitops-rule-item {
  padding: 14px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98) 0%, rgba(248, 250, 252, 0.96) 100%);
}

.gitops-rule-item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.gitops-rule-item-title {
  font-weight: 600;
  color: var(--color-text-main);
}

.gitops-rule-source-tip {
  margin: -4px 0 12px;
  color: var(--color-text-soft);
  font-size: 12px;
}

.gitops-target-preview {
  margin-bottom: 12px;
  padding: 12px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.96) 100%);
}

.gitops-target-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.gitops-target-preview-title {
  font-weight: 600;
  color: var(--color-text-main);
}

.gitops-target-select-row {
  margin-bottom: 12px;
}

.gitops-inline-item {
  margin-bottom: 0;
}

.gitops-code-text {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  word-break: break-all;
}

.param-name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.param-name {
  font-weight: 600;
  color: var(--color-text-main);
}

.param-executor {
  font-size: 12px;
  color: var(--color-text-soft);
}

.template-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.template-name-text {
  font-weight: 600;
  color: var(--color-text-main);
}

.template-compliance-chip-button {
  display: inline-flex;
  align-items: center;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.template-compliance-chip-button:focus-visible {
  outline: 2px solid rgba(245, 158, 11, 0.42);
  outline-offset: 2px;
  border-radius: 999px;
}

.template-compliance-chip-text {
  margin-inline-end: 0;
}

.template-compliance-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.template-compliance-detail-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.86);
}

.template-compliance-detail-title {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
}

.template-compliance-detail-app {
  margin-top: 2px;
  color: var(--color-text-soft);
  font-size: 12px;
  line-height: 1.4;
}

.template-compliance-detail-message {
  color: var(--color-warning-strong, #b45309);
  font-size: 12px;
  line-height: 1.5;
}

.template-compliance-detail-scopes {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.template-compliance-detail-scope {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.template-compliance-detail-scope-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 700;
}

.template-compliance-finding-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.template-compliance-finding {
  padding: 12px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 12px;
  background: rgba(248, 250, 252, 0.74);
}

.template-compliance-finding-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.template-compliance-finding-pipeline {
  color: var(--color-text-main);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.5;
}

.template-compliance-finding-rule {
  margin-top: 2px;
  color: var(--color-text-soft);
  font-size: 12px;
  line-height: 1.5;
  word-break: break-all;
}

.template-compliance-finding-tags {
  display: inline-flex;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}

.template-compliance-finding-tags :deep(.ant-tag) {
  margin-inline-end: 0;
}

.template-compliance-finding-message,
.template-compliance-finding-suggestion {
  margin-top: 8px;
  color: #475569;
  font-size: 12px;
  line-height: 1.6;
}

.template-compliance-finding-suggestion {
  color: #64748b;
}

.template-binding-warning-text {
  margin-top: 4px;
  font-size: 12px;
  color: var(--color-warning-strong, #b45309);
  line-height: 1.5;
}

.template-binding-pill {
  display: inline-flex;
  align-items: stretch;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.26);
  border-radius: 999px;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.62),
    0 6px 14px rgba(15, 23, 42, 0.04);
}

.template-binding-pill-segment {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
}

.template-binding-pill-segment + .template-binding-pill-segment {
  border-left: 1px solid rgba(148, 163, 184, 0.18);
}

.template-binding-pill-segment--ci {
  color: #1d4ed8;
  background: linear-gradient(180deg, #eff6ff 0%, #dbeafe 100%);
}

.template-binding-pill-segment--cd {
  color: #c2410c;
  background: linear-gradient(180deg, #fff7ed 0%, #ffedd5 100%);
}

.template-binding-pill-segment :deep(.anticon) {
  font-size: 12px;
}

.template-binding-text {
  color: var(--color-text-main);
  font-weight: 600;
}

@media (max-width: 992px) {
  .scope-card {
    margin-bottom: 16px;
  }

  .hook-template-summary-grid,
  .hook-template-capability-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .hook-template-capability-header,
  .hook-template-capability-card-head,
  .hook-template-variable-row {
    flex-direction: column;
  }

  .hook-template-variable-tags {
    justify-content: flex-start;
  }

  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
