<script setup lang="ts">
import { ExclamationCircleOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { computed, h, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  createNotificationHook,
  createNotificationMarkdownTemplate,
  createNotificationSource,
  deleteNotificationHook,
  deleteNotificationMarkdownTemplate,
  deleteNotificationSource,
  listNotificationHooks,
  listNotificationMarkdownTemplates,
  listNotificationSources,
  testNotificationSourceWebhook,
  updateNotificationHook,
  updateNotificationMarkdownTemplate,
  updateNotificationSource,
} from '../../api/notification'
import { listPlatformParamDicts } from '../../api/platform-param'
import type {
  NotificationConditionOperator,
  NotificationHook,
  NotificationHookPayload,
  NotificationMarkdownTemplate,
  NotificationMarkdownTemplateConditionPayload,
  NotificationMarkdownTemplatePayload,
  NotificationSource,
  NotificationSourcePayload,
  NotificationSourceWebhookTestPayload,
  NotificationSourceType,
} from '../../types/notification'
import type { PlatformParamDict } from '../../types/platform-param'
import { extractHTTPErrorMessage } from '../../utils/http-error'

type TabKey = 'sources' | 'templates' | 'hooks'

interface ConditionFormItem {
  local_id: string
  param_key: string
  operator: NotificationConditionOperator
  expected_value: string
  markdown_text: string
}

interface SourceFormState {
  name: string
  source_type: NotificationSourceType
  webhook_url: string
  verification_param: string
  keywords: string
  enabled: boolean
  remark: string
}

interface TemplateFormState {
  name: string
  title_template: string
  body_template: string
  conditions: ConditionFormItem[]
  enabled: boolean
  remark: string
}

interface HookFormState {
  name: string
  source_id: string
  markdown_template_id: string
  enabled: boolean
  remark: string
}

interface NotificationSearchSuggestion {
  id: string
  title: string
  subtitle: string
}

const activeTab = ref<TabKey>('sources')
const platformParams = ref<PlatformParamDict[]>([])

const builtinVariableOptions = [
  { label: 'app_key', value: 'app_key', type: '内置字段' },
  { label: 'app_name', value: 'app_name', type: '内置字段' },
  { label: 'release_name', value: 'release_name', type: '内置字段' },
  { label: 'project_name', value: 'project_name', type: '内置字段' },
  { label: 'env', value: 'env', type: '内置字段' },
  { label: 'env_code', value: 'env_code', type: '内置字段' },
  { label: 'branch', value: 'branch', type: '内置字段' },
  { label: 'git_ref', value: 'git_ref', type: '内置字段' },
  { label: 'image_version', value: 'image_version', type: '内置字段' },
  { label: 'image_tag', value: 'image_tag', type: '内置字段' },
  { label: 'order_no', value: 'order_no', type: '内置字段' },
  { label: 'operation_type', value: 'operation_type', type: '内置字段' },
  { label: 'source_order_no', value: 'source_order_no', type: '内置字段' },
  { label: 'executor_user_id', value: 'executor_user_id', type: '内置字段' },
  { label: 'executor_name', value: 'executor_name', type: '内置字段' },
  { label: 'release_status', value: 'release_status', type: '内置字段' },
  { label: 'release_stage', value: 'release_stage', type: '内置字段' },
  { label: 'release_status_rich', value: 'release_status_rich', type: '内置字段' },
  { label: 'release_stage_rich', value: 'release_stage_rich', type: '内置字段' },
] as const

const markdownVariableOptions = computed(() => {
  const platformOptions = platformParams.value.map((item) => ({
    label: `${item.name} (${item.param_key})`,
    value: item.param_key,
    type: '标准平台 Key',
  }))
  // 去重：移除与内置字段重复的平台参数
  const builtinKeys = new Set(builtinVariableOptions.map((item) => item.value))
  const uniquePlatformOptions = platformOptions.filter(
    (item) => !builtinKeys.has(item.value)
  )
  return [...builtinVariableOptions, ...uniquePlatformOptions]
})

// 条件下拉选择专用选项，排除 release_status 等运行时字段
const conditionParamKeyOptions = computed(() => {
  const platformOptions = platformParams.value.map((item) => ({
    label: `${item.name} (${item.param_key})`,
    value: item.param_key,
    type: '标准平台 Key',
  }))
  // 排除 release_status 等不适合作为条件的运行时字段
  const excludeKeys = new Set(['release_status', 'release_stage', 'release_status_rich', 'release_stage_rich'])
  const filteredBuiltin = builtinVariableOptions.filter(
    (item) => !excludeKeys.has(item.value as string)
  )
  // 去重：移除与内置字段重复的平台参数
  const builtinKeys = new Set(filteredBuiltin.map((item) => item.value))
  const uniquePlatformOptions = platformOptions.filter(
    (item) => !builtinKeys.has(item.value)
  )
  return [...filteredBuiltin, ...uniquePlatformOptions]
})

const conditionOperatorOptions = [
  { label: '等于', value: 'equals' },
  { label: '不等于', value: 'not_equals' },
  { label: '包含', value: 'contains' },
  { label: '不包含', value: 'not_contains' },
  { label: '为空', value: 'is_empty' },
  { label: '不为空', value: 'not_empty' },
] as const

function conditionOperatorLabel(operator: string) {
  return conditionOperatorOptions.find((item) => item.value === operator)?.label || operator
}

const sourceLoading = ref(false)
const sourceRows = ref<NotificationSource[]>([])
const sourceCatalog = ref<NotificationSource[]>([])
const sourceTotal = ref(0)
const sourceFilters = reactive({
  keyword: '',
  source_type: '' as NotificationSourceType | '',
  enabled: '' as '' | 'true' | 'false',
  page: 1,
  pageSize: 10,
})

const templateLoading = ref(false)
const templateRows = ref<NotificationMarkdownTemplate[]>([])
const templateCatalog = ref<NotificationMarkdownTemplate[]>([])
const templateTotal = ref(0)
const templateFilters = reactive({
  keyword: '',
  enabled: '' as '' | 'true' | 'false',
  page: 1,
  pageSize: 10,
})

const hookLoading = ref(false)
const hookRows = ref<NotificationHook[]>([])
const hookTotal = ref(0)
const hookFilters = reactive({
  keyword: '',
  enabled: '' as '' | 'true' | 'false',
  page: 1,
  pageSize: 10,
})

const notificationSearchVisible = ref(false)
const notificationSearchInputRef = ref<HTMLInputElement | null>(null)
const notificationSearchDraft = reactive({ keyword: '' })
const notificationSearchSuggestions = ref<NotificationSearchSuggestion[]>([])
const notificationSearchSuggestionsLoading = ref(false)
let notificationSearchTimer: ReturnType<typeof window.setTimeout> | null = null
let notificationSearchRequestSeq = 0

const sourceModalVisible = ref(false)
const sourceSubmitting = ref(false)
const sourceWebhookTestingID = ref('')
const editingSourceID = ref('')
const editingSourceHasVerificationParam = ref(false)
const isEditSource = computed(() => Boolean(editingSourceID.value))
const sourceFormRef = ref<FormInstance>()
const sourceForm = reactive<SourceFormState>({
  name: '',
  source_type: 'dingtalk',
  webhook_url: '',
  verification_param: '',
  keywords: '',
  enabled: true,
  remark: '',
})

const sourceTypeLabels: Record<NotificationSourceType, string> = {
  dingtalk: '钉钉',
  wecom: '企业微信群机器人',
  feishu: '飞书',
}

function sourceTypeLabel(value: NotificationSourceType | string | undefined) {
  return value ? sourceTypeLabels[value as NotificationSourceType] || value : '-'
}

function sourceUsesVerificationParam(value: NotificationSourceType | string | undefined) {
  return value === 'dingtalk' || value === 'feishu'
}

// ---- source modal masking ----
const sourceFormViewportInset = ref(0)

const sourceFormMaskStyle = computed(() => ({
  left: `${sourceFormViewportInset.value}px`,
  width: `calc(100% - ${sourceFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: sourceModalVisible.value ? 'auto' : 'none',
}))

const sourceFormWrapProps = computed(() => ({
  style: {
    left: `${sourceFormViewportInset.value}px`,
    width: `calc(100% - ${sourceFormViewportInset.value}px)`,
    pointerEvents: sourceModalVisible.value ? 'auto' : 'none',
  },
}))

let sourceFormViewportObserver: ResizeObserver | null = null

function readSourceFormViewportInset() {
  if (typeof document === 'undefined') return 0
  const appLayout = document.querySelector('.app-layout')
  if (appLayout) {
    const rawWidth = window.getComputedStyle(appLayout).getPropertyValue('--layout-sider-width').trim()
    const parsedWidth = Number.parseFloat(rawWidth)
    if (Number.isFinite(parsedWidth) && parsedWidth >= 0) return parsedWidth
  }
  const sider = document.querySelector('.app-sider')
  if (!sider) return 0
  return Math.max(sider.getBoundingClientRect().width, 0)
}

function syncSourceFormViewportInset() {
  sourceFormViewportInset.value = readSourceFormViewportInset()
}

function observeSourceFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') return
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) return
  sourceFormViewportObserver?.disconnect()
  sourceFormViewportObserver = new ResizeObserver(() => {
    syncSourceFormViewportInset()
  })
  if (appLayout) sourceFormViewportObserver.observe(appLayout)
  if (sider) sourceFormViewportObserver.observe(sider)
}

function stopObservingSourceFormViewportInset() {
  sourceFormViewportObserver?.disconnect()
  sourceFormViewportObserver = null
}

// ---- template modal masking ----
const templateFormMaskStyle = computed(() => ({
  left: `${sourceFormViewportInset.value}px`,
  width: `calc(100% - ${sourceFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: templateModalVisible.value ? 'auto' : 'none',
}))

const templateFormWrapProps = computed(() => ({
  style: {
    left: `${sourceFormViewportInset.value}px`,
    width: `calc(100% - ${sourceFormViewportInset.value}px)`,
    pointerEvents: templateModalVisible.value ? 'auto' : 'none',
  },
}))

// ---- hook modal masking ----
const hookFormMaskStyle = computed(() => ({
  left: `${sourceFormViewportInset.value}px`,
  width: `calc(100% - ${sourceFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: hookModalVisible.value ? 'auto' : 'none',
}))

const hookFormWrapProps = computed(() => ({
  style: {
    left: `${sourceFormViewportInset.value}px`,
    width: `calc(100% - ${sourceFormViewportInset.value}px)`,
    pointerEvents: hookModalVisible.value ? 'auto' : 'none',
  },
}))

const templateModalVisible = ref(false)
const templateSubmitting = ref(false)
const editingTemplateID = ref('')
const isEditTemplate = computed(() => Boolean(editingTemplateID.value))
const templateFormRef = ref<FormInstance>()
const templateForm = reactive<TemplateFormState>({
  name: '',
  title_template: '',
  body_template: '',
  conditions: [],
  enabled: true,
  remark: '',
})

const hookModalVisible = ref(false)
const hookSubmitting = ref(false)
const editingHookID = ref('')
const isEditHook = computed(() => Boolean(editingHookID.value))
const hookFormRef = ref<FormInstance>()
const hookForm = reactive<HookFormState>({
  name: '',
  source_id: '',
  markdown_template_id: '',
  enabled: true,
  remark: '',
})

const sourceRules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入通知源名称', trigger: 'blur' }],
  source_type: [{ required: true, message: '请选择通知源类型', trigger: 'change' }],
  webhook_url: [{ required: true, message: '请输入 Webhook 地址', trigger: 'blur' }],
  verification_param: [
    {
      validator: async (_rule: Rule, value: string) => {
        if (sourceForm.source_type !== 'feishu') return
        if (String(value || '').trim()) return
        if (isEditSource.value && editingSourceHasVerificationParam.value) return
        throw new Error('请输入飞书放行关键字')
      },
      trigger: 'blur',
    },
  ],
}

const templateRules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入 Markdown 模板名称', trigger: 'blur' }],
}

const hookRules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入通知 Hook 名称', trigger: 'blur' }],
  source_id: [{ required: true, message: '请选择通知源', trigger: 'change' }],
  markdown_template_id: [{ required: true, message: '请选择 Markdown 模板', trigger: 'change' }],
}

const sourceTypeFilterOptions = [
  { label: '钉钉', value: 'dingtalk' },
  { label: '企业微信群机器人', value: 'wecom' },
  { label: '飞书', value: 'feishu' },
] as const

const sourceTypeFormOptions = [
  { label: '钉钉', value: 'dingtalk' },
  { label: '企业微信群机器人', value: 'wecom' },
  { label: '飞书', value: 'feishu' },
] as const

const sourceFormNote = computed(() =>
  isEditSource.value
    ? '编辑态下通知源类型保持只读，如需修改请删除当前通知源后重新创建。'
    : '创建通知源，用于接收钉钉、企业微信群机器人或飞书机器人消息推送。',
)

const sourceTypeHelpText = computed(() =>
  '当前开放钉钉、企业微信群机器人、飞书通知源创建',
)

const sourceNamePlaceholder = computed(() => {
  if (sourceForm.source_type === 'feishu') return '例如：生产发布飞书群'
  if (sourceForm.source_type === 'wecom') return '例如：生产发布企业微信群'
  return '例如：生产发布钉钉群'
})

const sourceWebhookPlaceholder = computed(() => {
  if (sourceForm.source_type === 'feishu') return '请输入飞书机器人的 Webhook 地址'
  if (sourceForm.source_type === 'wecom') return '请输入企业微信群机器人的 Webhook 地址'
  return '请输入钉钉机器人的 Webhook 地址'
})

const sourceVerificationLabel = computed(() => (sourceForm.source_type === 'feishu' ? '放行关键字' : '验证参数（Secret）'))

const sourceVerificationPlaceholder = computed(() => {
  if (sourceForm.source_type === 'feishu') {
    return editingSourceHasVerificationParam.value
      ? '留空则沿用当前放行关键字，输入新值则覆盖'
      : '请输入飞书机器人安全设置里的放行关键字'
  }
  return editingSourceHasVerificationParam.value ? '留空则沿用当前 Secret，输入新值则覆盖' : '选填，钉钉机器人的加签 Secret'
})

const sourceVerificationHelp = computed(() => {
  if (sourceForm.source_type === 'feishu') {
    return editingSourceHasVerificationParam.value
      ? '当前已配置放行关键字，留空可继续沿用'
      : '消息标题会自动包含该关键字，用于通过飞书机器人安全校验'
  }
  return '当前已配置 Secret，留空可继续沿用'
})

const enabledOptions = [
  { label: '启用', value: 'true' },
  { label: '停用', value: 'false' },
] as const

const activeCreateLabel = computed(() => {
  if (activeTab.value === 'templates') return '新增 Markdown 模板'
  if (activeTab.value === 'hooks') return '新增通知 Hook'
  return '新增通知源'
})

const activeSearchPlaceholder = computed(() => {
  if (activeTab.value === 'templates') return '搜索 Markdown 模板'
  if (activeTab.value === 'hooks') return '搜索通知 Hook'
  return '搜索通知源'
})

const activeEnabledFilter = computed<string | undefined>({
  get() {
    const value =
      activeTab.value === 'templates'
        ? templateFilters.enabled
        : activeTab.value === 'hooks'
          ? hookFilters.enabled
          : sourceFilters.enabled
    return value || undefined
  },
  set(value) {
    const next = (value || '') as '' | 'true' | 'false'
    if (activeTab.value === 'templates') {
      templateFilters.enabled = next
      return
    }
    if (activeTab.value === 'hooks') {
      hookFilters.enabled = next
      return
    }
    sourceFilters.enabled = next
  },
})

const activeSourceTypeFilter = computed<NotificationSourceType | undefined>({
  get() {
    return sourceFilters.source_type || undefined
  },
  set(value) {
    sourceFilters.source_type = (value || '') as NotificationSourceType | ''
  },
})

const sourceOptions = computed(() =>
  sourceCatalog.value.map((item) => ({ label: `${item.name} · ${sourceTypeLabel(item.source_type)}`, value: item.id })),
)

const markdownTemplateOptions = computed(() =>
  templateCatalog.value.map((item) => ({ label: item.name, value: item.id })),
)

const sourceColumns: TableColumnsType<NotificationSource> = [
  { title: '通知源名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '类型', dataIndex: 'source_type', key: 'source_type', width: 120 },
  { title: '校验配置', dataIndex: 'has_verification_param', key: 'has_verification_param', width: 100 },
  { title: '关键字', dataIndex: 'keywords', key: 'keywords', width: 100 },
  { title: 'Webhook 地址', dataIndex: 'webhook_url', key: 'webhook_url', ellipsis: true },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
  { title: '操作', key: 'actions', width: 220, fixed: 'right' },
]

const templateColumns: TableColumnsType<NotificationMarkdownTemplate> = [
  { title: '模板名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '条件分支', key: 'conditions', width: 110 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
  { title: '操作', key: 'actions', width: 180, fixed: 'right' },
]

const hookColumns: TableColumnsType<NotificationHook> = [
  { title: '通知 Hook', dataIndex: 'name', key: 'name', width: 220 },
  { title: '通知源', dataIndex: 'source_name', key: 'source_name', width: 180 },
  { title: 'Markdown 模板', dataIndex: 'markdown_template_name', key: 'markdown_template_name', width: 220 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
  { title: '操作', key: 'actions', width: 180, fixed: 'right' },
]

const defaultNotificationTemplatePreset = {
  name: '通用发布通知模板-默认',
  title_template: '[{env}] {app_name} {release_name} {release_status_rich}',
  body_template: [
    '## 发布进展',
    '',
    '> **阶段**：{release_stage_rich}',
    '> **结果**：{release_status_rich}',
    '',
    '### 核心信息',
    '- **应用**：{app_name}（`{app_key}`）',
    '- **发布名称**：`{release_name}`',
    '- **环境**：`{env}`',
    '- **发布单**：`{order_no}`',
    '- **操作**：`{operation_type}`',
    '- **分支**：`{branch}`',
    '- **子服务**：`{project_name}`',
    '- **镜像版本**：`{image_version}`',
    '- **来源单**：`{source_order_no}`',
    '- **执行人**：{executor_name}',
    '',
    '---',
    '请前往 **GOS Release** 查看执行日志、审批记录与 Hook 详情',
  ].join('\n'),
  remark: '推荐用于构建完成通知与发布完成通知的通用模板，已内置彩色进度语义标签',
} as const

type TemplatePreviewScenario = 'build_success' | 'release_success' | 'release_failed'

const templatePreviewScenario = ref<TemplatePreviewScenario>('build_success')

const templatePreviewScenarioOptions = [
  { label: '构建成功', value: 'build_success' },
  { label: '发布成功', value: 'release_success' },
  { label: '发布失败', value: 'release_failed' },
] as const

const templatePreviewMockValues = computed<Record<string, string>>(() => {
  const common = {
    app_key: 'gov-collab-service',
    app_name: '省公协同后端',
    release_name: '省公协同后端正式发布',
    project_name: 'shenggongxie-notarization-management-java-k8s',
    env: 'prod',
    env_code: 'prod',
    branch: 'release_v2026.04.15',
    git_ref: 'release_v2026.04.15',
    image_version: '20260415.15',
    image_tag: '20260415.15',
    order_no: 'RO-20260415094512-8AC7D1E2',
    operation_type: 'deploy',
    source_order_no: 'RO-20260415090100-01AB23CD',
    executor_user_id: 'user-ops-01',
    executor_name: 'yepingfan',
  }
  if (templatePreviewScenario.value === 'build_success') {
    return buildNotificationTemplatePreviewValues({
      ...common,
      release_stage: 'build_complete',
      release_status: 'success',
    })
  }
  if (templatePreviewScenario.value === 'release_failed') {
    return buildNotificationTemplatePreviewValues({
      ...common,
      release_stage: 'post_release',
      release_status: 'failed',
    })
  }
  return buildNotificationTemplatePreviewValues({
    ...common,
    release_stage: 'post_release',
    release_status: 'success',
  })
})

const templatePreviewStageBadgeClass = computed(() =>
  templatePreviewMockValues.value.release_stage === 'build_complete'
    ? 'notification-preview-badge-warm'
    : 'notification-preview-badge-primary',
)

const templatePreviewStatusBadgeClass = computed(() => {
  switch (templatePreviewMockValues.value.release_status) {
    case 'failed':
      return 'notification-preview-badge-danger'
    case 'success':
      return 'notification-preview-badge-success'
    default:
      return 'notification-preview-badge-neutral'
  }
})

function normalizeEnabledFilter(value: '' | 'true' | 'false') {
  if (value === '') return undefined
  return value === 'true'
}

function buildConditionItem(payload?: NotificationMarkdownTemplateConditionPayload): ConditionFormItem {
  return {
    local_id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    param_key: payload?.param_key || '',
    operator: payload?.operator || 'equals',
    expected_value: payload?.expected_value || '',
    markdown_text: payload?.markdown_text || '',
  }
}

function resetSourceForm() {
  editingSourceID.value = ''
  sourceForm.name = ''
  sourceForm.source_type = 'dingtalk'
  sourceForm.webhook_url = ''
  sourceForm.verification_param = ''
  sourceForm.keywords = ''
  sourceForm.enabled = true
  sourceForm.remark = ''
}

function resetTemplateForm() {
  editingTemplateID.value = ''
  templateForm.name = ''
  templateForm.title_template = ''
  templateForm.body_template = ''
  templateForm.conditions = []
  templateForm.enabled = true
  templateForm.remark = ''
}

function applyDefaultNotificationTemplatePreset() {
  if (!templateForm.name.trim() || templateForm.name === defaultNotificationTemplatePreset.name) {
    templateForm.name = defaultNotificationTemplatePreset.name
  }
  templateForm.title_template = defaultNotificationTemplatePreset.title_template
  templateForm.body_template = defaultNotificationTemplatePreset.body_template
  if (!templateForm.remark.trim()) {
    templateForm.remark = defaultNotificationTemplatePreset.remark
  }
}

function buildReleaseStageRichValue(value: string) {
  switch (String(value || '').trim()) {
    case 'build_complete':
      return '🟠 构建完成'
    case 'post_release':
      return '🔵 发布完成'
    default:
      return value ? `🟡 ${value}` : ''
  }
}

function buildReleaseStatusRichValue(value: string) {
  switch (String(value || '').trim()) {
    case 'success':
    case 'deploy_success':
      return '🟢 成功'
    case 'failed':
    case 'deploy_failed':
      return '🔴 失败'
    case 'cancelled':
      return '⚪ 已取消'
    case 'building':
      return '🟠 构建中'
    case 'built_waiting_deploy':
      return '🟠 已构建待部署'
    case 'deploying':
      return '🔵 部署中'
    case 'pending':
      return '🟡 待执行'
    case 'running':
      return '🔵 执行中'
    case 'pending_approval':
      return '🟣 待审批'
    case 'approving':
      return '🟣 审批中'
    case 'approved':
      return '🟢 审批通过'
    case 'rejected':
      return '🔴 审批拒绝'
    case 'queued':
      return '🟡 排队中'
    default:
      return value ? `🟡 ${value}` : ''
  }
}

function buildNotificationTemplatePreviewValues(values: Record<string, string>) {
  const normalizedValues = { ...values }
  normalizedValues.release_stage_rich = buildReleaseStageRichValue(normalizedValues.release_stage || '')
  normalizedValues.release_status_rich = buildReleaseStatusRichValue(normalizedValues.release_status || '')
  return normalizedValues
}

function escapeMarkdownHTML(input: string) {
  return input
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function renderInlineMarkdown(input: string) {
  let text = escapeMarkdownHTML(input)
  text = text.replace(/`([^`]+)`/g, '<code>$1</code>')
  text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  text = text.replace(/\*([^*]+)\*/g, '<em>$1</em>')
  return text
}

function renderMarkdownPreview(input: string) {
  const source = String(input || '').replace(/\r\n/g, '\n').trim()
  if (!source) {
    return '<p>通知正文预览</p>'
  }

  const lines = source.split('\n')
  const blocks: string[] = []
  let paragraph: string[] = []
  let listItems: string[] = []
  let quoteLines: string[] = []

  const flushParagraph = () => {
    if (!paragraph.length) return
    const content = renderInlineMarkdown(paragraph.join('\n')).replace(/\n/g, '<br />')
    blocks.push(`<p>${content}</p>`)
    paragraph = []
  }

  const flushList = () => {
    if (!listItems.length) return
    blocks.push(`<ul>${listItems.map((item) => `<li>${renderInlineMarkdown(item)}</li>`).join('')}</ul>`)
    listItems = []
  }

  const flushQuote = () => {
    if (!quoteLines.length) return
    blocks.push(`<blockquote>${quoteLines.map((line) => `<p>${renderInlineMarkdown(line)}</p>`).join('')}</blockquote>`)
    quoteLines = []
  }

  for (const rawLine of lines) {
    const line = rawLine.trimRight()
    const trimmed = line.trim()

    if (!trimmed) {
      flushParagraph()
      flushList()
      flushQuote()
      continue
    }

    if (trimmed === '---') {
      flushParagraph()
      flushList()
      flushQuote()
      blocks.push('<hr />')
      continue
    }

    const heading = trimmed.match(/^(#{1,3})\s+(.*)$/)
    if (heading) {
      flushParagraph()
      flushList()
      flushQuote()
      const level = heading[1].length
      blocks.push(`<h${level}>${renderInlineMarkdown(heading[2])}</h${level}>`)
      continue
    }

    if (trimmed.startsWith('> ')) {
      flushParagraph()
      flushList()
      quoteLines.push(trimmed.slice(2).trim())
      continue
    }

    if (trimmed.startsWith('- ')) {
      flushParagraph()
      flushQuote()
      listItems.push(trimmed.slice(2).trim())
      continue
    }

    flushList()
    flushQuote()
    paragraph.push(trimmed)
  }

  flushParagraph()
  flushList()
  flushQuote()

  return blocks.join('')
}

function renderTemplatePlaceholders(input: string, values: Record<string, string>) {
  return String(input || '').replace(/\{([a-zA-Z0-9_]+)\}/g, (_, key: string) => values[key] ?? '')
}

function matchesTemplateCondition(condition: ConditionFormItem, values: Record<string, string>) {
  const actualValue = String(values[String(condition.param_key || '').trim()] || '')
  const expectedValue = String(condition.expected_value || '')
  switch (condition.operator) {
    case 'equals':
      return actualValue === expectedValue
    case 'not_equals':
      return actualValue !== expectedValue
    case 'contains':
      return actualValue.includes(expectedValue)
    case 'not_contains':
      return !actualValue.includes(expectedValue)
    case 'is_empty':
      return actualValue.trim() === ''
    case 'not_empty':
      return actualValue.trim() !== ''
    default:
      return false
  }
}

const templatePreviewTitle = computed(() =>
  renderTemplatePlaceholders(templateForm.title_template, templatePreviewMockValues.value).trim() || '通知标题预览',
)

const templatePreviewBody = computed(() => {
  const blocks: string[] = []
  const body = renderTemplatePlaceholders(templateForm.body_template, templatePreviewMockValues.value).trim()
  if (body) {
    blocks.push(body)
  }
  templateForm.conditions.forEach((condition) => {
    if (!condition.param_key.trim() || !condition.markdown_text.trim()) {
      return
    }
    if (matchesTemplateCondition(condition, templatePreviewMockValues.value)) {
      blocks.push(renderTemplatePlaceholders(condition.markdown_text, templatePreviewMockValues.value).trim())
    }
  })
  return blocks.filter(Boolean).join('\n\n').trim() || '通知正文预览'
})

const templatePreviewBodyHTML = computed(() => renderMarkdownPreview(templatePreviewBody.value))

function resetHookForm() {
  editingHookID.value = ''
  hookForm.name = ''
  hookForm.source_id = ''
  hookForm.markdown_template_id = ''
  hookForm.enabled = true
  hookForm.remark = ''
}

async function loadPlatformParams() {
  try {
    const response = await listPlatformParamDicts({ page: 1, page_size: 500 })
    platformParams.value = response.data
  } catch (error) {
    platformParams.value = []
    message.error(extractHTTPErrorMessage(error, '标准平台 Key 加载失败'))
  }
}

async function loadSources() {
  sourceLoading.value = true
  try {
    const response = await listNotificationSources({
      keyword: sourceFilters.keyword.trim() || undefined,
      source_type: sourceFilters.source_type || undefined,
      enabled: normalizeEnabledFilter(sourceFilters.enabled),
      page: sourceFilters.page,
      page_size: sourceFilters.pageSize,
    })
    sourceRows.value = response.data
    sourceTotal.value = response.total
    sourceFilters.page = response.page
    sourceFilters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '通知源加载失败'))
  } finally {
    sourceLoading.value = false
  }
}

async function loadSourceCatalog() {
  try {
    const response = await listNotificationSources({ page: 1, page_size: 200 })
    sourceCatalog.value = response.data
  } catch {
    sourceCatalog.value = []
  }
}

async function loadMarkdownTemplates() {
  templateLoading.value = true
  try {
    const response = await listNotificationMarkdownTemplates({
      keyword: templateFilters.keyword.trim() || undefined,
      enabled: normalizeEnabledFilter(templateFilters.enabled),
      page: templateFilters.page,
      page_size: templateFilters.pageSize,
    })
    templateRows.value = response.data
    templateTotal.value = response.total
    templateFilters.page = response.page
    templateFilters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Markdown 模板加载失败'))
  } finally {
    templateLoading.value = false
  }
}

async function loadMarkdownTemplateCatalog() {
  try {
    const response = await listNotificationMarkdownTemplates({ page: 1, page_size: 200 })
    templateCatalog.value = response.data
  } catch {
    templateCatalog.value = []
  }
}

async function loadHooks() {
  hookLoading.value = true
  try {
    const response = await listNotificationHooks({
      keyword: hookFilters.keyword.trim() || undefined,
      enabled: normalizeEnabledFilter(hookFilters.enabled),
      page: hookFilters.page,
      page_size: hookFilters.pageSize,
    })
    hookRows.value = response.data
    hookTotal.value = response.total
    hookFilters.page = response.page
    hookFilters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '通知 Hook 加载失败'))
  } finally {
    hookLoading.value = false
  }
}

function addCondition() {
  templateForm.conditions.push(buildConditionItem())
}

function removeCondition(localID: string) {
  templateForm.conditions = templateForm.conditions.filter((item) => item.local_id !== localID)
}

function openCreateSourceModal() {
  resetSourceForm()
  editingSourceHasVerificationParam.value = false
  sourceForm.source_type = 'dingtalk'
  syncSourceFormViewportInset()
  sourceModalVisible.value = true
}

function openEditSourceModal(item: NotificationSource) {
  editingSourceID.value = item.id
  sourceForm.name = item.name
  sourceForm.source_type = item.source_type
  sourceForm.webhook_url = item.webhook_url
  sourceForm.verification_param = ''
  sourceForm.keywords = item.keywords || ''
  sourceForm.enabled = item.enabled
  sourceForm.remark = item.remark || ''
  editingSourceHasVerificationParam.value = item.has_verification_param
  syncSourceFormViewportInset()
  sourceModalVisible.value = true
}

function closeSourceModal() {
  sourceModalVisible.value = false
}

function handleSourceFormAfterClose() {
  editingSourceHasVerificationParam.value = false
  resetSourceForm()
  sourceSubmitting.value = false
  void sourceFormRef.value?.clearValidate()
}

function buildSourcePayload(): NotificationSourcePayload {
  const isDingTalk = sourceForm.source_type === 'dingtalk'
  return {
    name: sourceForm.name.trim(),
    source_type: sourceForm.source_type,
    webhook_url: sourceForm.webhook_url.trim(),
    verification_param: sourceUsesVerificationParam(sourceForm.source_type) ? sourceForm.verification_param.trim() || undefined : undefined,
    keywords: isDingTalk ? sourceForm.keywords.trim() || undefined : undefined,
    enabled: sourceForm.enabled,
    remark: sourceForm.remark.trim() || undefined,
  }
}

async function submitSource() {
  try {
    await sourceFormRef.value?.validate()
  } catch {
    return
  }
  sourceSubmitting.value = true
  try {
    const payload = buildSourcePayload()
    if (editingSourceID.value) {
      await updateNotificationSource(editingSourceID.value, payload)
      message.success('通知源更新成功')
    } else {
      await createNotificationSource(payload)
      message.success('通知源创建成功')
    }
    closeSourceModal()
    await Promise.all([loadSources(), loadSourceCatalog()])
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '通知源保存失败'))
  } finally {
    sourceSubmitting.value = false
  }
}

async function testSavedSourceWebhook(item: NotificationSource) {
  sourceWebhookTestingID.value = item.id
  try {
    const result = await testNotificationSourceWebhook({
      source_id: item.id,
      name: item.name,
      source_type: item.source_type,
      webhook_url: item.webhook_url,
      enabled: item.enabled,
      remark: item.remark || undefined,
    })
    message.success(result.message || '测试消息发送成功')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '测试消息发送失败'))
  } finally {
    sourceWebhookTestingID.value = ''
  }
}

function confirmDeleteSource(item: NotificationSource) {
  Modal.confirm({
    title: `删除通知源"${item.name}"`,
    icon: h(ExclamationCircleOutlined),
    content: '删除后将无法继续被通知 Hook 引用，请确认是否继续',
    async onOk() {
      try {
        await deleteNotificationSource(item.id)
        message.success('通知源删除成功')
        await Promise.all([loadSources(), loadSourceCatalog(), loadHooks()])
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '通知源删除失败'))
      }
    },
  })
}

function openCreateTemplateModal() {
  resetTemplateForm()
  applyDefaultNotificationTemplatePreset()
  syncSourceFormViewportInset()
  templateModalVisible.value = true
}

function openEditTemplateModal(item: NotificationMarkdownTemplate) {
  editingTemplateID.value = item.id
  templateForm.name = item.name
  templateForm.title_template = item.title_template || ''
  templateForm.body_template = item.body_template || ''
  templateForm.conditions = (item.conditions || []).map((cond) => buildConditionItem(cond))
  templateForm.enabled = item.enabled
  templateForm.remark = item.remark || ''
  syncSourceFormViewportInset()
  templateModalVisible.value = true
}

function closeTemplateModal() {
  templateModalVisible.value = false
}

function handleTemplateFormAfterClose() {
  resetTemplateForm()
  templateSubmitting.value = false
  void templateFormRef.value?.clearValidate()
}

async function submitTemplate() {
  try {
    await templateFormRef.value?.validate()
  } catch {
    return
  }
  if (!templateForm.title_template.trim() && !templateForm.body_template.trim()) {
    message.warning('标题模板和正文模板至少填写一项')
    return
  }
  if (templateForm.conditions.some((item) => !item.param_key.trim() || !item.markdown_text.trim())) {
    message.warning('请完整填写条件分支的标准平台 Key 与 Markdown 语句')
    return
  }
  templateSubmitting.value = true
  try {
    const payload: NotificationMarkdownTemplatePayload = {
      name: templateForm.name.trim(),
      title_template: templateForm.title_template.trim() || undefined,
      body_template: templateForm.body_template.trim() || undefined,
      conditions: templateForm.conditions.map((item) => ({
        param_key: item.param_key.trim(),
        operator: item.operator,
        expected_value: item.expected_value.trim() || undefined,
        markdown_text: item.markdown_text.trim(),
      })),
      enabled: templateForm.enabled,
      remark: templateForm.remark.trim() || undefined,
    }
    if (editingTemplateID.value) {
      await updateNotificationMarkdownTemplate(editingTemplateID.value, payload)
      message.success('Markdown 模板更新成功')
    } else {
      await createNotificationMarkdownTemplate(payload)
      message.success('Markdown 模板创建成功')
    }
    closeTemplateModal()
    await Promise.all([loadMarkdownTemplates(), loadMarkdownTemplateCatalog()])
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Markdown 模板保存失败'))
  } finally {
    templateSubmitting.value = false
  }
}

function confirmDeleteTemplate(item: NotificationMarkdownTemplate) {
  Modal.confirm({
    title: `删除 Markdown 模板"${item.name}"`,
    icon: h(ExclamationCircleOutlined),
    content: '删除后引用该模板的通知 Hook 将无法继续发送，请确认是否继续',
    async onOk() {
      try {
        await deleteNotificationMarkdownTemplate(item.id)
        message.success('Markdown 模板删除成功')
        await Promise.all([loadMarkdownTemplates(), loadMarkdownTemplateCatalog(), loadHooks()])
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, 'Markdown 模板删除失败'))
      }
    },
  })
}

function openCreateHookModal() {
  resetHookForm()
  syncSourceFormViewportInset()
  hookModalVisible.value = true
}

function openEditHookModal(item: NotificationHook) {
  editingHookID.value = item.id
  hookForm.name = item.name
  hookForm.source_id = item.source_id
  hookForm.markdown_template_id = item.markdown_template_id
  hookForm.enabled = item.enabled
  hookForm.remark = item.remark || ''
  syncSourceFormViewportInset()
  hookModalVisible.value = true
}

function closeHookModal() {
  hookModalVisible.value = false
}

function handleHookFormAfterClose() {
  resetHookForm()
  hookSubmitting.value = false
  void hookFormRef.value?.clearValidate()
}

async function submitHook() {
  try {
    await hookFormRef.value?.validate()
  } catch {
    return
  }
  hookSubmitting.value = true
  try {
    const payload: NotificationHookPayload = {
      name: hookForm.name.trim(),
      source_id: hookForm.source_id,
      markdown_template_id: hookForm.markdown_template_id,
      enabled: hookForm.enabled,
      remark: hookForm.remark.trim() || undefined,
    }
    if (editingHookID.value) {
      await updateNotificationHook(editingHookID.value, payload)
      message.success('通知 Hook 更新成功')
    } else {
      await createNotificationHook(payload)
      message.success('通知 Hook 创建成功')
    }
    closeHookModal()
    await loadHooks()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '通知 Hook 保存失败'))
  } finally {
    hookSubmitting.value = false
  }
}

function confirmDeleteHook(item: NotificationHook) {
  Modal.confirm({
    title: `删除通知 Hook"${item.name}"`,
    icon: h(ExclamationCircleOutlined),
    content: '删除后发布模板将无法继续引用该通知 Hook，请确认是否继续',
    async onOk() {
      try {
        await deleteNotificationHook(item.id)
        message.success('通知 Hook 删除成功')
        await loadHooks()
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '通知 Hook 删除失败'))
      }
    },
  })
}

function handleSourceSearch() {
  sourceFilters.page = 1
  void loadSources()
}

function handleTemplateSearch() {
  templateFilters.page = 1
  void loadMarkdownTemplates()
}

function handleHookSearch() {
  hookFilters.page = 1
  void loadHooks()
}

function getActiveKeyword() {
  if (activeTab.value === 'templates') return templateFilters.keyword
  if (activeTab.value === 'hooks') return hookFilters.keyword
  return sourceFilters.keyword
}

function setActiveKeyword(value: string) {
  const next = value.trim()
  if (activeTab.value === 'templates') {
    templateFilters.keyword = next
    return
  }
  if (activeTab.value === 'hooks') {
    hookFilters.keyword = next
    return
  }
  sourceFilters.keyword = next
}

function handleActiveSearch() {
  if (activeTab.value === 'templates') {
    handleTemplateSearch()
    return
  }
  if (activeTab.value === 'hooks') {
    handleHookSearch()
    return
  }
  handleSourceSearch()
}

function openCreateForActiveTab() {
  if (activeTab.value === 'templates') {
    openCreateTemplateModal()
    return
  }
  if (activeTab.value === 'hooks') {
    openCreateHookModal()
    return
  }
  openCreateSourceModal()
}

function openNotificationSearchDialog() {
  notificationSearchDraft.keyword = getActiveKeyword()
  notificationSearchVisible.value = true
  window.setTimeout(() => {
    notificationSearchInputRef.value?.focus()
  }, 0)
  void fetchNotificationSearchSuggestions(notificationSearchDraft.keyword)
}

function closeNotificationSearchDialog() {
  notificationSearchVisible.value = false
  if (notificationSearchTimer) {
    clearTimeout(notificationSearchTimer)
    notificationSearchTimer = null
  }
  notificationSearchRequestSeq += 1
  notificationSearchSuggestions.value = []
  notificationSearchSuggestionsLoading.value = false
}

async function fetchNotificationSearchSuggestions(keyword: string) {
  const currentKeyword = keyword.trim()
  if (!currentKeyword) {
    notificationSearchSuggestions.value = []
    notificationSearchSuggestionsLoading.value = false
    return
  }
  const requestSeq = ++notificationSearchRequestSeq
  const requestTab = activeTab.value
  notificationSearchSuggestionsLoading.value = true
  try {
    if (requestTab === 'templates') {
      const response = await listNotificationMarkdownTemplates({
        keyword: currentKeyword,
        enabled: normalizeEnabledFilter(templateFilters.enabled),
        page: 1,
        page_size: 6,
      })
      if (requestSeq !== notificationSearchRequestSeq || requestTab !== activeTab.value) return
      notificationSearchSuggestions.value = response.data.map((item) => ({
        id: item.id,
        title: item.name,
        subtitle: `${item.conditions.length} 条条件分支`,
      }))
      return
    }
    if (requestTab === 'hooks') {
      const response = await listNotificationHooks({
        keyword: currentKeyword,
        enabled: normalizeEnabledFilter(hookFilters.enabled),
        page: 1,
        page_size: 6,
      })
      if (requestSeq !== notificationSearchRequestSeq || requestTab !== activeTab.value) return
      notificationSearchSuggestions.value = response.data.map((item) => ({
        id: item.id,
        title: item.name,
        subtitle: `${item.source_name || '-'} / ${item.markdown_template_name || '-'}`,
      }))
      return
    }
    const response = await listNotificationSources({
      keyword: currentKeyword,
      source_type: sourceFilters.source_type || undefined,
      enabled: normalizeEnabledFilter(sourceFilters.enabled),
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== notificationSearchRequestSeq || requestTab !== activeTab.value) return
    notificationSearchSuggestions.value = response.data.map((item) => ({
      id: item.id,
      title: item.name,
      subtitle: sourceTypeLabel(item.source_type),
    }))
  } catch {
    if (requestSeq === notificationSearchRequestSeq) {
      notificationSearchSuggestions.value = []
    }
  } finally {
    if (requestSeq === notificationSearchRequestSeq) {
      notificationSearchSuggestionsLoading.value = false
    }
  }
}

function handleNotificationSearchInput() {
  const keyword = notificationSearchDraft.keyword.trim()
  if (notificationSearchTimer) clearTimeout(notificationSearchTimer)
  if (!keyword) {
    notificationSearchSuggestions.value = []
    notificationSearchSuggestionsLoading.value = false
    return
  }
  notificationSearchTimer = setTimeout(() => {
    notificationSearchTimer = null
    void fetchNotificationSearchSuggestions(keyword)
  }, 220)
}

function handleNotificationSearchSubmit() {
  setActiveKeyword(notificationSearchDraft.keyword)
  handleActiveSearch()
  closeNotificationSearchDialog()
}

function handleNotificationSearchSuggestionSelect(item: NotificationSearchSuggestion) {
  notificationSearchDraft.keyword = item.title
  setActiveKeyword(item.title)
  handleActiveSearch()
  closeNotificationSearchDialog()
}

function handleSourcePageChange(page: number, pageSize: number) {
  sourceFilters.page = page
  sourceFilters.pageSize = pageSize
  void loadSources()
}

function handleTemplatePageChange(page: number, pageSize: number) {
  templateFilters.page = page
  templateFilters.pageSize = pageSize
  void loadMarkdownTemplates()
}

function handleHookPageChange(page: number, pageSize: number) {
  hookFilters.page = page
  hookFilters.pageSize = pageSize
  void loadHooks()
}

const selectedSource = computed(() => sourceCatalog.value.find((item) => item.id === hookForm.source_id) || null)
const selectedMarkdownTemplate = computed(() => templateCatalog.value.find((item) => item.id === hookForm.markdown_template_id) || null)

onMounted(async () => {
  syncSourceFormViewportInset()
  observeSourceFormViewportInset()
  await Promise.all([
    loadPlatformParams(),
    loadSources(),
    loadSourceCatalog(),
    loadMarkdownTemplates(),
    loadMarkdownTemplateCatalog(),
    loadHooks(),
  ])
})

onBeforeUnmount(() => {
  stopObservingSourceFormViewportInset()
  if (notificationSearchTimer) {
    clearTimeout(notificationSearchTimer)
    notificationSearchTimer = null
  }
  notificationSearchRequestSeq += 1
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header notification-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">通知</h2>
      </div>
      <div class="page-header-actions notification-header-actions">
        <a-button class="application-toolbar-icon-btn notification-search-trigger" @click="openNotificationSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-select
          v-if="activeTab === 'sources'"
          v-model:value="activeSourceTypeFilter"
          allow-clear
          class="notification-toolbar-select"
          :options="sourceTypeFilterOptions"
          placeholder="类型 · 全部"
        />
        <a-select
          v-model:value="activeEnabledFilter"
          allow-clear
          class="notification-toolbar-select"
          :options="enabledOptions"
          placeholder="状态 · 全部"
        />
        <a-button class="notification-toolbar-query-btn" @click="handleActiveSearch">查询</a-button>
        <a-button class="application-toolbar-action-btn notification-create-btn" @click="openCreateForActiveTab">
          <template #icon><PlusOutlined /></template>
          {{ activeCreateLabel }}
        </a-button>
      </div>
    </div>

    <transition name="notification-search-fade">
      <div v-if="notificationSearchVisible" class="notification-search-overlay" @click.self="closeNotificationSearchDialog">
        <div class="notification-search-floating-panel">
          <div class="notification-search-floating-input">
            <SearchOutlined class="notification-search-floating-icon" />
            <input
              ref="notificationSearchInputRef"
              v-model="notificationSearchDraft.keyword"
              class="notification-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              :placeholder="activeSearchPlaceholder"
              @input="handleNotificationSearchInput"
              @keydown.enter="handleNotificationSearchSubmit"
              @keydown.esc="closeNotificationSearchDialog"
            />
          </div>
          <div v-if="notificationSearchSuggestionsLoading || notificationSearchSuggestions.length > 0" class="notification-search-suggestions">
            <div v-if="notificationSearchSuggestionsLoading" class="notification-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in notificationSearchSuggestions"
                :key="item.id"
                type="button"
                class="notification-search-suggestion"
                @click="handleNotificationSearchSuggestionSelect(item)"
              >
                <span class="notification-search-suggestion-title">{{ item.title }}</span>
                <span class="notification-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-card :bordered="false" class="table-card notification-table-card">
      <a-tabs v-model:activeKey="activeTab" class="notification-tabs">
        <a-tab-pane key="sources" tab="通知源">
          <a-table
            class="notification-table"
            row-key="id"
            :loading="sourceLoading"
            :columns="sourceColumns"
            :data-source="sourceRows"
            :pagination="false"
            :scroll="{ x: 980 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'source_type'">
                {{ sourceTypeLabel(record.source_type) }}
              </template>
              <template v-else-if="column.key === 'has_verification_param'">
                <a-tag :color="record.has_verification_param ? 'blue' : 'default'">
                  {{ record.has_verification_param ? '已配置' : '未配置' }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'keywords'">
                <span>{{ record.keywords || '-' }}</span>
              </template>
              <template v-else-if="column.key === 'enabled'">
                <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'actions'">
                <a-space>
                  <a-button type="link" size="small" :loading="sourceWebhookTestingID === record.id" @click="testSavedSourceWebhook(record)">测试</a-button>
                  <a-button type="link" size="small" @click="openEditSourceModal(record)">编辑</a-button>
                  <a-button type="link" danger size="small" @click="confirmDeleteSource(record)">删除</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
          <div class="pagination-area">
            <a-pagination
              :current="sourceFilters.page"
              :page-size="sourceFilters.pageSize"
              :total="sourceTotal"
              show-size-changer
              show-quick-jumper
              :show-total="(total: number) => `共 ${total} 条`"
              @change="handleSourcePageChange"
            />
          </div>
        </a-tab-pane>

        <a-tab-pane key="templates" tab="Markdown 模板">
          <a-table
            class="notification-table"
            row-key="id"
            :loading="templateLoading"
            :columns="templateColumns"
            :data-source="templateRows"
            :pagination="false"
            :scroll="{ x: 900 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'conditions'">
                {{ record.conditions.length }} 条
              </template>
              <template v-else-if="column.key === 'enabled'">
                <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'actions'">
                <a-space>
                  <a-button type="link" size="small" @click="openEditTemplateModal(record)">编辑</a-button>
                  <a-button type="link" danger size="small" @click="confirmDeleteTemplate(record)">删除</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
          <div class="pagination-area">
            <a-pagination
              :current="templateFilters.page"
              :page-size="templateFilters.pageSize"
              :total="templateTotal"
              show-size-changer
              show-quick-jumper
              :show-total="(total: number) => `共 ${total} 条`"
              @change="handleTemplatePageChange"
            />
          </div>
        </a-tab-pane>

        <a-tab-pane key="hooks" tab="通知 Hook">
          <a-table
            class="notification-table"
            row-key="id"
            :loading="hookLoading"
            :columns="hookColumns"
            :data-source="hookRows"
            :pagination="false"
            :scroll="{ x: 980 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'enabled'">
                <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'actions'">
                <a-space>
                  <a-button type="link" size="small" @click="openEditHookModal(record)">编辑</a-button>
                  <a-button type="link" danger size="small" @click="confirmDeleteHook(record)">删除</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
          <div class="pagination-area">
            <a-pagination
              :current="hookFilters.page"
              :page-size="hookFilters.pageSize"
              :total="hookTotal"
              show-size-changer
              show-quick-jumper
              :show-total="(total: number) => `共 ${total} 条`"
              @change="handleHookPageChange"
            />
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-modal
      :open="sourceModalVisible"
      :width="560"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleSourceFormAfterClose"
      :mask-style="sourceFormMaskStyle"
      :wrap-props="sourceFormWrapProps"
      wrap-class-name="source-form-modal-wrap"
      @cancel="closeSourceModal"
    >
      <template #title>
        <div class="source-form-modal-titlebar">
          <span class="source-form-modal-title">{{ isEditSource ? '编辑通知源' : '新增通知源' }}</span>
          <a-button class="application-toolbar-action-btn source-form-modal-save-btn" :loading="sourceSubmitting" @click="submitSource">
            保存
          </a-button>
        </div>
      </template>

      <a-form ref="sourceFormRef" :model="sourceForm" :rules="sourceRules" layout="vertical" :required-mark="false" class="source-form">
        <div class="source-form-note">
          {{ sourceFormNote }}
        </div>

        <div v-if="isEditSource" class="source-form-panel source-form-panel--context">
          <div class="source-form-panel-title">当前通知源</div>
          <div class="source-form-context">
            <div class="source-form-context-item">
              <div class="source-form-context-label">通知源名称</div>
              <div class="source-form-context-value">{{ sourceForm.name }}</div>
            </div>
            <div class="source-form-context-item">
              <div class="source-form-context-label">通知源类型</div>
              <div class="source-form-context-value">{{ sourceTypeLabel(sourceForm.source_type) }}</div>
            </div>
          </div>
        </div>

        <div class="source-form-panel">
          <div class="source-form-panel-title">{{ isEditSource ? '可编辑配置' : '基础配置' }}</div>

          <a-form-item name="name">
            <template #label>
              <span class="source-form-label">
                通知源名称
                <a-tag class="source-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="sourceForm.name" allow-clear :placeholder="sourceNamePlaceholder" />
          </a-form-item>

          <a-form-item name="source_type">
            <template #label>
              <span class="source-form-label">
                通知源类型
                <a-tag class="source-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="sourceForm.source_type" :options="sourceTypeFormOptions" :disabled="isEditSource" />
            <div class="form-help-text">{{ sourceTypeHelpText }}</div>
          </a-form-item>

          <a-form-item name="webhook_url">
            <template #label>
              <span class="source-form-label">
                Webhook 地址
                <a-tag class="source-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="sourceForm.webhook_url" allow-clear :placeholder="sourceWebhookPlaceholder" />
          </a-form-item>

          <a-form-item v-if="sourceUsesVerificationParam(sourceForm.source_type)" name="verification_param">
            <template #label>
              <span class="source-form-label">
                {{ sourceVerificationLabel }}
                <a-tag v-if="sourceForm.source_type === 'feishu'" class="source-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input-password
              v-model:value="sourceForm.verification_param"
              allow-clear
              :placeholder="sourceVerificationPlaceholder"
            />
            <div v-if="editingSourceHasVerificationParam || sourceForm.source_type === 'feishu'" class="form-help-text">{{ sourceVerificationHelp }}</div>
          </a-form-item>

          <a-form-item v-if="sourceForm.source_type === 'dingtalk'" name="keywords">
            <template #label>
              <span class="source-form-label">安全关键字</span>
            </template>
            <a-input
              v-model:value="sourceForm.keywords"
              allow-clear
              placeholder="选填，钉钉机器人安全设置中的自定义关键字"
            />
            <div class="form-help-text">消息正文将自动包含该关键字，用于通过钉钉机器人安全校验</div>
          </a-form-item>

          <a-form-item name="remark">
            <template #label>
              <span class="source-form-label">备注</span>
            </template>
            <a-textarea v-model:value="sourceForm.remark" :auto-size="{ minRows: 2, maxRows: 4 }" placeholder="例如：生产发版群、回滚通知群" />
          </a-form-item>

          <a-form-item name="enabled">
            <template #label>
              <span class="source-form-label">启用状态</span>
            </template>
            <a-switch v-model:checked="sourceForm.enabled" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      :open="templateModalVisible"
      :width="680"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleTemplateFormAfterClose"
      :mask-style="templateFormMaskStyle"
      :wrap-props="templateFormWrapProps"
      wrap-class-name="template-form-modal-wrap"
      @cancel="closeTemplateModal"
    >
      <template #title>
        <div class="template-form-modal-titlebar">
          <span class="template-form-modal-title">{{ isEditTemplate ? '编辑 Markdown 模板' : '新增 Markdown 模板' }}</span>
          <a-button class="application-toolbar-action-btn template-form-modal-save-btn" :loading="templateSubmitting" @click="submitTemplate">
            保存
          </a-button>
        </div>
      </template>
      <a-form
        ref="templateFormRef"
        :model="templateForm"
        :rules="templateRules"
        layout="vertical"
        :required-mark="false"
        class="template-form"
      >
        <div class="template-form-note">
          {{ isEditTemplate ? '编辑态下可修改模板名称、内容与条件分支，修改后引用的通知 Hook 将使用新内容。' : '创建 Markdown 模板，定义通知标题与正文内容，支持条件分支与变量插值。' }}
        </div>

        <div v-if="isEditTemplate" class="template-form-panel template-form-panel--context">
          <div class="template-form-panel-title">当前模板</div>
          <div class="template-form-context">
            <div class="template-form-context-item">
              <div class="template-form-context-label">模板名称</div>
              <div class="template-form-context-value">{{ templateForm.name }}</div>
            </div>
            <div class="template-form-context-item">
              <div class="template-form-context-label">条件分支数</div>
              <div class="template-form-context-value">{{ templateForm.conditions.length }} 条</div>
            </div>
          </div>
        </div>

        <div class="template-form-panel">
          <div class="template-form-panel-title">{{ isEditTemplate ? '可编辑配置' : '基础配置' }}</div>

          <a-form-item name="name">
            <template #label>
              <span class="template-form-label">
                模板名称
                <a-tag class="template-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="templateForm.name" allow-clear placeholder="例如：发布结果通知模板" />
          </a-form-item>
        </div>

        <div class="notification-template-preset-bar">
          <div>
            <div class="section-title">推荐模板</div>
            <div class="section-description">这套默认模板已经适配当前的构建完成 / 发布完成两段进度，适合直接作为通用发布通知模板，默认还会用彩色语义标签把构建 / 发布阶段凸显出来</div>
          </div>
          <a-button @click="applyDefaultNotificationTemplatePreset">套用“通用发布通知模板-默认”</a-button>
        </div>

        <a-form-item label="标题模板">
          <a-input v-model:value="templateForm.title_template" allow-clear placeholder="例如：[{env}] {app_name} 发布结果" />
        </a-form-item>
        <a-form-item label="正文模板" name="body_template">
          <a-textarea
            v-model:value="templateForm.body_template"
            :auto-size="{ minRows: 6, maxRows: 12 }"
            placeholder="支持直接使用 {order_no}、{app_key}、{env}、{image_version} 等标准平台 Key 变量"
          />
        </a-form-item>

        <div class="notification-preview-card notification-preview-warm">
          <div class="notification-preview-head">
            <div>
              <div class="section-title">模板预览</div>
              <div class="section-description">这里会按 Markdown 语法实时渲染预览标准 Markdown 本身不支持字体颜色，正文里建议直接使用 {release_stage_rich} / {release_status_rich}，让阶段和结果随字段值自动带出彩色语义标签</div>
            </div>
            <a-segmented v-model:value="templatePreviewScenario" :options="templatePreviewScenarioOptions" />
          </div>
          <div class="notification-preview-meta">
            <span class="notification-preview-badge" :class="templatePreviewStageBadgeClass">{{ templatePreviewMockValues.release_stage_rich }}</span>
            <span class="notification-preview-badge" :class="templatePreviewStatusBadgeClass">{{ templatePreviewMockValues.release_status_rich }}</span>
            <span class="notification-preview-badge notification-preview-badge-neutral">{{ templatePreviewMockValues.env.toUpperCase() }}</span>
          </div>
          <div class="notification-preview-shell">
            <div class="notification-preview-title">{{ templatePreviewTitle }}</div>
            <div class="notification-preview-body markdown-preview-content" v-html="templatePreviewBodyHTML"></div>
          </div>
        </div>

        <div class="variable-guide-card">
          <div class="section-title">可用变量</div>
          <div class="section-description">推荐优先使用 <code>{release_stage_rich}</code> 与 <code>{release_status_rich}</code>标准 Markdown 不支持直接改字色，这两个变量会按当前阶段 / 结果自动输出带语义色彩的展示文本</div>
          <div class="variable-chip-grid">
            <span v-for="item in markdownVariableOptions" :key="item.value" class="variable-chip">
              {{ item.value }}
              <small>{{ item.type }}</small>
            </span>
          </div>
        </div>

        <div class="condition-section-head notification-template-section-head">
          <div>
            <div class="section-title">条件 Markdown 语句</div>
            <div class="section-description">根据标准平台 Key 的值命中条件后，附加对应 Markdown 语句注意：release_status / release_stage 以及它们的彩色语义标签变量都属于运行时字段，已在预览区展示，但不建议作为条件判断字段</div>
          </div>
          <a-button type="dashed" @click="addCondition">
            <template #icon><PlusOutlined /></template>
            新增条件
          </a-button>
        </div>

        <a-empty v-if="!templateForm.conditions.length" description="暂未配置条件语句" />

        <div v-else class="condition-list">
          <div v-for="(condition, index) in templateForm.conditions" :key="condition.local_id" class="condition-card">
            <div class="condition-card-head">
              <div class="condition-card-title">条件 {{ index + 1 }}</div>
              <a-button type="link" danger size="small" @click="removeCondition(condition.local_id)">删除</a-button>
            </div>
            <div class="condition-form-stack">
              <a-form-item label="标准平台 Key" class="compact-item">
                <a-select
                  v-model:value="condition.param_key"
                  show-search
                  allow-clear
                  option-filter-prop="label"
                  :options="conditionParamKeyOptions"
                  placeholder="选择用于判断的标准平台 Key"
                />
              </a-form-item>
              <a-form-item label="条件运算符" class="compact-item">
                <a-select v-model:value="condition.operator" :options="conditionOperatorOptions" />
              </a-form-item>
              <a-form-item label="期望值" class="compact-item">
                <a-input
                  v-model:value="condition.expected_value"
                  allow-clear
                  :disabled="condition.operator === 'is_empty' || condition.operator === 'not_empty'"
                  placeholder="例如：prod / gateway"
                />
              </a-form-item>
              <a-form-item label="Markdown 语句" class="compact-item">
                <a-textarea
                  v-model:value="condition.markdown_text"
                  :auto-size="{ minRows: 3, maxRows: 8 }"
                  placeholder="命中条件后，将这段 Markdown 追加到正文后面"
                />
              </a-form-item>
            </div>
            <div class="condition-preview">
              规则预览：当 <code>{{ condition.param_key || '标准平台 Key' }}</code> {{ conditionOperatorLabel(condition.operator) }}
              <code>{{ condition.operator === 'is_empty' || condition.operator === 'not_empty' ? '空值规则' : condition.expected_value || '...' }}</code>
              时，追加这一段 Markdown
            </div>
          </div>
        </div>

        <div class="template-form-panel">
          <div class="template-form-panel-title">其他配置</div>

          <a-form-item name="remark">
            <template #label>
              <span class="template-form-label">备注</span>
            </template>
            <a-textarea v-model:value="templateForm.remark" :auto-size="{ minRows: 2, maxRows: 4 }" placeholder="例如：生产成功通知、审批拒绝通知" />
          </a-form-item>

          <a-form-item name="enabled">
            <template #label>
              <span class="template-form-label">启用状态</span>
            </template>
            <a-switch v-model:checked="templateForm.enabled" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      :open="hookModalVisible"
      :width="520"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleHookFormAfterClose"
      :mask-style="hookFormMaskStyle"
      :wrap-props="hookFormWrapProps"
      wrap-class-name="hook-form-modal-wrap"
      @cancel="closeHookModal"
    >
      <template #title>
        <div class="hook-form-modal-titlebar">
          <span class="hook-form-modal-title">{{ isEditHook ? '编辑通知 Hook' : '新增通知 Hook' }}</span>
          <a-button class="application-toolbar-action-btn hook-form-modal-save-btn" :loading="hookSubmitting" @click="submitHook">
            保存
          </a-button>
        </div>
      </template>
      <a-form
        ref="hookFormRef"
        :model="hookForm"
        :rules="hookRules"
        layout="vertical"
        :required-mark="false"
        class="hook-form"
      >
        <div class="hook-form-note">
          {{ isEditHook ? '编辑态下可修改通知 Hook 名称、关联的通知源与模板，修改后引用的发布模板将使用新配置。' : '创建通知 Hook，将通知源与 Markdown 模板绑定，供发布模板引用。' }}
        </div>

        <div v-if="isEditHook" class="hook-form-panel hook-form-panel--context">
          <div class="hook-form-panel-title">当前 Hook</div>
          <div class="hook-form-context">
            <div class="hook-form-context-item">
              <div class="hook-form-context-label">Hook 名称</div>
              <div class="hook-form-context-value">{{ hookForm.name }}</div>
            </div>
            <div class="hook-form-context-item">
              <div class="hook-form-context-label">关联通知源</div>
              <div class="hook-form-context-value">{{ selectedSource ? `${selectedSource.name} · ${sourceTypeLabel(selectedSource.source_type)}` : '-' }}</div>
            </div>
          </div>
        </div>

        <div class="hook-form-panel">
          <div class="hook-form-panel-title">{{ isEditHook ? '可编辑配置' : '基础配置' }}</div>

          <a-form-item name="name">
            <template #label>
              <span class="hook-form-label">
                通知 Hook 名称
                <a-tag class="hook-form-required-tag">必填</a-tag>
              </span>
              <a-popover
                trigger="click"
                placement="rightTop"
                overlay-class-name="release-tip-popover"
              >
                <template #content>
                  <div class="release-tip-content">
                    发布模板引用该通知 Hook 后，会自动使用发布过程中的标准平台 Key 数据渲染通知内容
                  </div>
                </template>
                <button
                  class="release-tip-trigger release-tip-trigger-info"
                  type="button"
                  aria-label="查看通知 Hook 说明"
                >
                  <ExclamationCircleOutlined />
                </button>
              </a-popover>
            </template>
            <a-input v-model:value="hookForm.name" allow-clear placeholder="例如：生产发布结果通知 Hook" />
          </a-form-item>

          <a-form-item name="source_id">
            <template #label>
              <span class="hook-form-label">
                通知源
                <a-tag class="hook-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="hookForm.source_id" show-search option-filter-prop="label" :options="sourceOptions" placeholder="选择通知源" />
          </a-form-item>

          <a-form-item name="markdown_template_id">
            <template #label>
              <span class="hook-form-label">
                Markdown 模板
                <a-tag class="hook-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="hookForm.markdown_template_id"
              show-search
              option-filter-prop="label"
              :options="markdownTemplateOptions"
              placeholder="选择 Markdown 模板"
            />
          </a-form-item>
        </div>

        <div class="hook-form-panel">
          <div class="hook-form-panel-title">关联预览</div>

          <a-form-item>
            <template #label>
              <span class="hook-form-label">已选通知源</span>
            </template>
            <a-input
              :value="selectedSource ? `${selectedSource.name} · ${sourceTypeLabel(selectedSource.source_type)}` : '未选择'"
              readonly
            />
          </a-form-item>
          <a-form-item>
            <template #label>
              <span class="hook-form-label">已选 Markdown 模板</span>
            </template>
            <a-input :value="selectedMarkdownTemplate?.name || '未选择'" readonly />
          </a-form-item>
        </div>

        <div class="hook-form-panel">
          <div class="hook-form-panel-title">其他配置</div>

          <a-form-item name="remark">
            <template #label>
              <span class="hook-form-label">备注</span>
            </template>
            <a-textarea v-model:value="hookForm.remark" :auto-size="{ minRows: 2, maxRows: 4 }" placeholder="例如：用于发布成功通知、回滚失败告警" />
          </a-form-item>

          <a-form-item name="enabled">
            <template #label>
              <span class="hook-form-label">启用状态</span>
            </template>
            <a-switch v-model:checked="hookForm.enabled" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.notification-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: nowrap;
  gap: 20px;
  padding: 0 !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
}

.notification-page-header .page-header-copy {
  flex: 0 0 auto;
  min-width: 0;
}

.notification-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
  flex: 1 1 auto;
  gap: 12px;
  min-width: 0;
}

:deep(.notification-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.notification-toolbar-select.ant-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  min-height: 42px;
  border-radius: 16px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.notification-toolbar-select.ant-select .ant-select-selection-item),
:deep(.notification-toolbar-select.ant-select .ant-select-arrow) {
  color: #1e3a8a;
  font-weight: 650;
}

:deep(.notification-toolbar-select.ant-select .ant-select-selection-item),
:deep(.notification-toolbar-select.ant-select .ant-select-selection-placeholder) {
  display: flex;
  align-items: center;
  height: 100%;
  line-height: 1 !important;
}

:deep(.notification-toolbar-select.ant-select .ant-select-selection-placeholder) {
  color: rgba(30, 58, 138, 0.38) !important;
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.application-toolbar-icon-btn.ant-btn),
:deep(.notification-toolbar-query-btn.ant-btn) {
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

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  padding: 0;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.notification-toolbar-query-btn.ant-btn:hover),
:deep(.notification-toolbar-query-btn.ant-btn:focus) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.76) !important;
  color: #0f172a !important;
}

.notification-search-overlay {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  left: var(--layout-sider-width, 220px);
  z-index: 1200;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 84px 24px 24px;
  background: rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(8px) saturate(112%);
  -webkit-backdrop-filter: blur(8px) saturate(112%);
}

.notification-search-floating-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: min(100%, 480px);
  padding: 0;
  border: none;
  background: transparent;
  box-shadow: none;
  backdrop-filter: none;
}

.notification-search-floating-input {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.74);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.6)),
    rgba(255, 255, 255, 0.44);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 16px 32px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(125%);
  -webkit-backdrop-filter: blur(18px) saturate(125%);
  padding: 0 14px;
}

.notification-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.notification-search-floating-field {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0;
  border: none;
  outline: none;
  background: transparent;
  box-shadow: none;
  color: #0f172a;
  font-size: 13px;
  font-weight: 650;
  line-height: 34px;
}

.notification-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.notification-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.66)),
    rgba(255, 255, 255, 0.5);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 18px 36px rgba(15, 23, 42, 0.1);
}

.notification-search-suggestions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-radius: 18px;
  border: 1px solid rgba(255, 255, 255, 0.62);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.52), rgba(255, 255, 255, 0.36)),
    rgba(255, 255, 255, 0.22);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.74),
    0 16px 30px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(124%);
  -webkit-backdrop-filter: blur(18px) saturate(124%);
}

.notification-search-suggestion,
.notification-search-suggestion-loading {
  width: 100%;
  border: none;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.34);
  color: rgba(51, 65, 85, 0.76);
  padding: 10px 12px;
  text-align: left;
}

.notification-search-suggestion {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  cursor: pointer;
  transition: background 0.18s ease, transform 0.18s ease;
}

.notification-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
  transform: translateY(-1px);
}

.notification-search-suggestion-title,
.notification-search-suggestion-subtitle {
  display: block;
}

.notification-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 750;
}

.notification-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.74);
  font-size: 12px;
  line-height: 1.4;
}

.notification-search-fade-enter-active,
.notification-search-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.notification-search-fade-enter-from,
.notification-search-fade-leave-to {
  opacity: 0;
}

.notification-search-fade-enter-from .notification-search-floating-panel,
.notification-search-fade-leave-to .notification-search-floating-panel {
  opacity: 0;
  transform: translateY(-8px);
}

.notification-search-fade-enter-to .notification-search-floating-panel,
.notification-search-fade-leave-from .notification-search-floating-panel {
  opacity: 1;
  transform: translateY(0);
}

.notification-table-card {
  background: transparent;
  box-shadow: none;
}

.notification-table-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.notification-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 16px;
}

.notification-table :deep(.ant-table-container) {
  overflow: hidden;
  border-radius: 0 !important;
  border: 1px solid rgba(148, 163, 184, 0.22);
  background: rgba(255, 255, 255, 0.96);
}

.notification-table :deep(.ant-table),
.notification-table :deep(.ant-table-content),
.notification-table :deep(.ant-table-body) {
  border-radius: 0 !important;
}

.notification-table :deep(.ant-table-thead > tr > th) {
  border-bottom: none;
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: rgba(239, 246, 255, 0.96);
  font-size: 13px;
  font-weight: 800;
}

.notification-table :deep(.ant-table-thead > tr > th:first-child) {
  border-top-left-radius: 0 !important;
}

.notification-table :deep(.ant-table-thead > tr > th:last-child) {
  border-top-right-radius: 0 !important;
}

.notification-table :deep(.ant-table-tbody > tr > td) {
  border-bottom-color: rgba(226, 232, 240, 0.78);
  color: #0f172a;
}

.notification-table :deep(.ant-table-tbody > tr:last-child > td:first-child) {
  border-bottom-left-radius: 0 !important;
}

.notification-table :deep(.ant-table-tbody > tr:last-child > td:last-child) {
  border-bottom-right-radius: 0 !important;
}

.notification-table :deep(.ant-table-cell-fix-right) {
  background: #fff !important;
}

.notification-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right) {
  background: #f8fafc !important;
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

.notification-template-preset-bar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
  padding: 16px 18px;
  border: 1px solid #f2dfbf;
  border-radius: 18px;
  background:
    radial-gradient(circle at top left, rgba(217, 119, 6, 0.12), transparent 38%),
    linear-gradient(180deg, #fffaf1 0%, #fff4e4 100%);
}

.variable-guide-card {
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid #eef2ff;
  border-radius: 16px;
  background: linear-gradient(180deg, #fafcff 0%, #f6f9ff 100%);
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: #102340;
}

.section-description {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7a90;
}

.notification-preview-card {
  margin-bottom: 16px;
  padding: 18px;
  border-radius: 20px;
  border: 1px solid #f0d9b4;
  background:
    radial-gradient(circle at top right, rgba(217, 119, 6, 0.12), transparent 28%),
    linear-gradient(180deg, #fffbf5 0%, #fff4e5 100%);
}

.notification-preview-warm {
  border-color: #f0d9b4;
}

.notification-preview-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 14px;
}

.notification-preview-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}

.notification-preview-badge {
  display: inline-flex;
  align-items: center;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.82);
  border: 1px solid rgba(148, 163, 184, 0.22);
  color: #1f3352;
  font-size: 12px;
  font-weight: 700;
}

.notification-preview-badge-primary {
  color: #0f4c81;
  border-color: rgba(59, 130, 246, 0.24);
  background: rgba(239, 246, 255, 0.92);
}

.notification-preview-badge-success {
  color: #166534;
  border-color: rgba(34, 197, 94, 0.22);
  background: rgba(240, 253, 244, 0.96);
}

.notification-preview-badge-warm {
  color: #9a5a00;
  border-color: rgba(217, 119, 6, 0.24);
  background: rgba(255, 247, 237, 0.96);
}

.notification-preview-badge-danger {
  color: #be123c;
  border-color: rgba(244, 63, 94, 0.2);
  background: rgba(255, 241, 242, 0.96);
}

.notification-preview-badge-neutral {
  color: #475569;
  border-color: rgba(148, 163, 184, 0.24);
  background: rgba(248, 250, 252, 0.96);
}

.notification-preview-shell {
  padding: 16px;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.85);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.notification-preview-title {
  font-size: 15px;
  font-weight: 800;
  color: #13233f;
  margin-bottom: 12px;
}

.notification-preview-body {
  margin: 0;
  font-size: 13px;
  line-height: 1.78;
  color: #31435f;
}

.markdown-preview-content :deep(h1),
.markdown-preview-content :deep(h2),
.markdown-preview-content :deep(h3) {
  margin: 0 0 12px;
  color: #13233f;
  font-weight: 800;
}

.markdown-preview-content :deep(h2) {
  font-size: 16px;
}

.markdown-preview-content :deep(h3) {
  font-size: 14px;
}

.markdown-preview-content :deep(p) {
  margin: 0 0 10px;
}

.markdown-preview-content :deep(ul) {
  margin: 0 0 12px;
  padding-left: 18px;
}

.markdown-preview-content :deep(li + li) {
  margin-top: 6px;
}

.markdown-preview-content :deep(blockquote) {
  margin: 0 0 12px;
  padding: 10px 12px;
  border-left: 3px solid #f2b463;
  border-radius: 10px;
  background: rgba(255, 248, 235, 0.92);
}

.markdown-preview-content :deep(blockquote p:last-child) {
  margin-bottom: 0;
}

.markdown-preview-content :deep(code) {
  padding: 1px 6px;
  border-radius: 999px;
  background: #fff2d8;
  color: #8a4b00;
  font-family: 'SFMono-Regular', 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
}

.markdown-preview-content :deep(hr) {
  border: 0;
  border-top: 1px solid #edd6b0;
  margin: 14px 0;
}

.variable-chip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.variable-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 999px;
  background: #ffffff;
  border: 1px solid #d9e3f7;
  color: #17345f;
  font-size: 12px;
  font-weight: 600;
}

.variable-chip small {
  color: #7b8798;
  font-weight: 500;
}

.condition-section-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
}

.notification-template-section-head {
  width: min(100%, 520px);
}

.condition-list {
  display: grid;
  gap: 12px;
  margin-bottom: 16px;
  width: min(100%, 520px);
}

.condition-card {
  padding: 16px;
  border: 1px solid #e8eefb;
  border-radius: 18px;
  background: #fbfcff;
}

.condition-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.condition-card-title {
  font-size: 14px;
  font-weight: 700;
  color: #102340;
}

.compact-item {
  margin-bottom: 12px;
}

.condition-form-stack {
  width: 100%;
  display: flex;
  flex-direction: column;
}

.condition-preview {
  padding: 10px 12px;
  border-radius: 12px;
  background: #fff;
  color: #526175;
  font-size: 12px;
}

.condition-preview code {
  padding: 1px 6px;
  border-radius: 999px;
  background: #eef4ff;
  color: #204c8a;
}

.form-help-text {
  margin-top: 6px;
  font-size: 12px;
  color: #7b8798;
}

/* ---- template form modal shell ---- */
.template-form-modal-wrap :deep(.ant-modal) {
  width: min(680px, calc(100vw - 32px)) !important;
}

.template-form-modal-wrap :deep(.ant-modal-content) {
  position: relative;
  overflow: hidden;
  isolation: isolate;
  border-radius: 24px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  background:
    radial-gradient(circle at top right, rgba(217, 119, 6, 0.08), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.08), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  box-shadow:
    0 32px 90px rgba(15, 23, 42, 0.18),
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    inset 0 -1px 0 rgba(255, 255, 255, 0.28);
  backdrop-filter: blur(18px) saturate(180%);
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

.template-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.template-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.template-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

/* ---- template form titlebar ---- */
.template-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.template-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.template-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

/* ---- template form layout ---- */
.template-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ---- template form note ---- */
.template-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.template-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(217, 119, 6, 0.42), rgba(245, 158, 11, 0.16));
}

/* ---- template form panels ---- */
.template-form-panel {
  padding: 0;
}

.template-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.template-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.template-form-note + .template-form-panel,
.template-form-panel + .template-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.template-form-panel--context {
  padding-bottom: 4px;
}

/* ---- template form context block ---- */
.template-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.template-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.template-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.template-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

/* ---- template form label ---- */
.template-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.template-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(253, 230, 138, 0.72);
  background: rgba(255, 251, 235, 0.96);
  color: #b45309;
  font-size: 11px;
  line-height: 18px;
}

/* ---- template form item spacing & label ---- */
.template-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.template-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

/* ---- template form input/select transparent ---- */
.template-form :deep(.ant-input),
.template-form :deep(.ant-select-selector),
.template-form :deep(.ant-input-affix-wrapper) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.template-form :deep(.ant-input:hover),
.template-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.template-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: rgba(245, 158, 11, 0.48) !important;
}

.template-form :deep(.ant-input:focus),
.template-form :deep(.ant-input-focused),
.template-form :deep(.ant-input-affix-wrapper-focused),
.template-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(217, 119, 6, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(217, 119, 6, 0.12) !important;
}

.template-form :deep(.ant-select-disabled .ant-select-selector),
.template-form :deep(.ant-input[disabled]) {
  background: transparent !important;
  color: #94a3b8 !important;
}

.template-form :deep(.ant-select-selection-placeholder),
.template-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

/* ---- hook form modal shell ---- */
.hook-form-modal-wrap :deep(.ant-modal) {
  width: min(520px, calc(100vw - 32px)) !important;
}

.hook-form-modal-wrap :deep(.ant-modal-content) {
  position: relative;
  overflow: hidden;
  isolation: isolate;
  border-radius: 24px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  background:
    radial-gradient(circle at top right, rgba(16, 185, 129, 0.08), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.08), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  box-shadow:
    0 32px 90px rgba(15, 23, 42, 0.18),
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    inset 0 -1px 0 rgba(255, 255, 255, 0.28);
  backdrop-filter: blur(18px) saturate(180%);
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

.hook-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.hook-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.hook-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

/* ---- hook form titlebar ---- */
.hook-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.hook-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.hook-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

/* ---- hook form layout ---- */
.hook-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ---- hook form note ---- */
.hook-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.hook-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(16, 185, 129, 0.42), rgba(52, 211, 153, 0.16));
}

/* ---- hook form panels ---- */
.hook-form-panel {
  padding: 0;
}

.hook-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.hook-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.hook-form-note + .hook-form-panel,
.hook-form-panel + .hook-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.hook-form-panel--context {
  padding-bottom: 4px;
}

/* ---- hook form context block ---- */
.hook-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.hook-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.hook-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.hook-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

/* ---- hook form label ---- */
.hook-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.hook-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(167, 243, 208, 0.72);
  background: rgba(236, 253, 245, 0.96);
  color: #047857;
  font-size: 11px;
  line-height: 18px;
}

/* ---- hook form item spacing & label ---- */
.hook-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.hook-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

/* ---- hook form input/select transparent ---- */
.hook-form :deep(.ant-input),
.hook-form :deep(.ant-select-selector),
.hook-form :deep(.ant-input-affix-wrapper) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.hook-form :deep(.ant-input:hover),
.hook-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.hook-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: rgba(16, 185, 129, 0.48) !important;
}

.hook-form :deep(.ant-input:focus),
.hook-form :deep(.ant-input-focused),
.hook-form :deep(.ant-input-affix-wrapper-focused),
.hook-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(16, 185, 129, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.12) !important;
}

.hook-form :deep(.ant-select-disabled .ant-select-selector),
.hook-form :deep(.ant-input[disabled]) {
  background: transparent !important;
  color: #94a3b8 !important;
}

.hook-form :deep(.ant-select-selection-placeholder),
.hook-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

/* ---- source form modal shell ---- */
.source-form-modal-wrap :deep(.ant-modal) {
  width: min(560px, calc(100vw - 32px)) !important;
}

.source-form-modal-wrap :deep(.ant-modal-content) {
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

.source-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.source-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.source-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

/* ---- source form titlebar ---- */
.source-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.source-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.source-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

/* ---- source form layout ---- */
.source-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ---- source form note ---- */
.source-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.source-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

/* ---- source form panels ---- */
.source-form-panel {
  padding: 0;
}

.source-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.source-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.source-form-note + .source-form-panel,
.source-form-panel + .source-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.source-form-panel--context {
  padding-bottom: 4px;
}

/* ---- source form context block ---- */
.source-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.source-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.source-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.source-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

/* ---- source form label ---- */
.source-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.source-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

/* ---- source form item spacing & label ---- */
.source-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.source-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

/* ---- source form input/select transparent ---- */
.source-form :deep(.ant-input),
.source-form :deep(.ant-select-selector),
.source-form :deep(.ant-input-affix-wrapper) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.source-form :deep(.ant-input:hover),
.source-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.source-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.source-form :deep(.ant-input:focus),
.source-form :deep(.ant-input-focused),
.source-form :deep(.ant-input-affix-wrapper-focused),
.source-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.source-form :deep(.ant-select-disabled .ant-select-selector),
.source-form :deep(.ant-input[disabled]) {
  background: transparent !important;
  color: #94a3b8 !important;
}

.source-form :deep(.ant-select-selection-placeholder),
.source-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

@media (max-width: 900px) {
  .notification-page-header {
    flex-wrap: wrap;
  }

  .notification-header-actions {
    flex-wrap: wrap;
    justify-content: flex-start;
    width: 100%;
  }

  :deep(.notification-toolbar-select.ant-select) {
    flex: 1 1 140px;
  }

  .condition-section-head,
  .notification-preview-head,
  .notification-template-preset-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
