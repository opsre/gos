<script setup lang="ts">
import {
  ArrowLeftOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  FullscreenOutlined,
  PlusOutlined,
  ProfileOutlined,
  RocketOutlined,
  ThunderboltFilled,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getApplicationApprovalFlowBinding, getApplicationByID, listApplications } from '../../api/application'
import { getExecutorParamDefByID, listApplicationExecutorParamDefs } from '../../api/pipeline'
import { batchCreateReleaseOrders, createReleaseOrder, buildReleaseOrder, getReleaseOrderByID, getReleaseTemplateByID, listAllReleaseTemplates, listApprovalFlows, listReleaseOrderParams, updateReleaseOrder, syncTemplateExecutorParamDefs } from '../../api/release'
import { getReleaseSettings } from '../../api/system'
import { useAuthStore } from '../../stores/auth'
import type { Application } from '../../types/application'
import type { ExecutorParamDef } from '../../types/pipeline'
import type { ReleaseEnvironmentConfig } from '../../types/system'
import type {
  CreateReleaseOrderParamPayload,
  CreateReleaseOrderPayload,
  ApprovalFlowDefinition,
  ReleaseOrder,
  ReleaseOrderParam,
  ReleasePipelineScope,
  ReleaseTemplate,
  ReleaseTemplateBinding,
  ReleaseTemplateParam,
} from '../../types/release'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SelectOption {
  label: string
  value: string
  description?: string
  disabled?: boolean
}

interface ChoiceMeta {
  options: SelectOption[]
  multiple: boolean
  delimiter: string
}

interface CreateFormState {
  application_id: string
  template_id: string
  release_name: string
  env_code: string
  git_ref: string
  remark: string
}

interface ScopeRuntimeState {
  loading: boolean
  error: string
  param_defs: ExecutorParamDef[]
}

interface BatchReleaseOrderDraft {
  id: string
  payload: CreateReleaseOrderPayload
  applicationName: string
  templateName: string
  envName: string
  displayName: string
  error: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const formRef = ref<FormInstance>()
const loadingApplications = ref(false)
const loadingEnvOptions = ref(false)
const loadingTemplates = ref(false)
const loadingTemplateDetail = ref(false)
const loadingEditOrder = ref(false)
const submitting = ref(false)
const submittingMode = ref<'standard' | 'fast' | 'build' | 'batch' | ''>('')
const batchDrafts = ref<BatchReleaseOrderDraft[]>([])
const syncingTemplateParams = ref(false)
const templateWarning = ref('')
const loadingApprovalFlow = ref(false)
const approvalFlowError = ref('')
const applicationApprovalFlowID = ref('')
const approvalFlowDefinitions = ref<ApprovalFlowDefinition[]>([])

const allApplicationOptions = ref<SelectOption[]>([])
const applicationRecords = ref<Application[]>([])
const envOptions = ref<SelectOption[]>([])
const defaultEnvCode = ref('')
const templateOptions = ref<SelectOption[]>([])
const templateList = ref<ReleaseTemplate[]>([])
const selectedTemplate = ref<ReleaseTemplate | null>(null)
const templateBindings = ref<ReleaseTemplateBinding[]>([])
const templateParams = ref<ReleaseTemplateParam[]>([])
const editingOrder = ref<ReleaseOrder | null>(null)
const editingParamSnapshot = ref<ReleaseOrderParam[]>([])
const pendingEditSnapshotRestore = ref(false)
const paramValues = reactive<Record<string, string>>({})
const expandedChoiceState = reactive({
  open: false,
  scope: 'ci' as ReleasePipelineScope,
  paramID: '',
  keyword: '',
})

const scopeStates = reactive<Record<ReleasePipelineScope, ScopeRuntimeState>>({
  ci: {
    loading: false,
    error: '',
    param_defs: [],
  },
  cd: {
    loading: false,
    error: '',
    param_defs: [],
  },
})

const preferredTemplateID = ref('')
const preferredBindingID = ref('')
const currentUserID = computed(() => String(authStore.profile?.id || '').trim())
const editingOrderID = computed(() => String(route.params.id || '').trim())
const isEditMode = computed(() => Boolean(editingOrderID.value))
const isBatchMode = computed(() => !isEditMode.value && String(route.query.batch || '').trim() === '1')

const formState = reactive<CreateFormState>({
  application_id: '',
  template_id: '',
  release_name: '',
  env_code: '',
  git_ref: '',
  remark: '',
})

const scopeLabels: Record<ReleasePipelineScope, string> = {
  ci: 'CI',
  cd: 'CD',
}

const allowedApplicationIDs = computed(() => {
  if (authStore.isAdmin) {
    return null
  }
  return new Set(
    allApplicationOptions.value
      .filter((item) => authStore.hasApplicationPermission('release.create', item.value))
      .map((item) => item.value),
  )
})

const applicationOptions = computed<SelectOption[]>(() => {
  const allowed = allowedApplicationIDs.value
  const editingApplicationID = String(editingOrder.value?.application_id || '').trim()
  if (!allowed) {
    return allApplicationOptions.value
  }
  return allApplicationOptions.value.filter((item) => allowed.has(item.value) || (isEditMode.value && item.value === editingApplicationID))
})

const authorizedEnvOptions = computed<SelectOption[]>(() => {
  if (authStore.isAdmin || !formState.application_id.trim()) {
    return envOptions.value
  }
  const allowedEnvCodes = new Set(
    authStore.listApplicationPermissionEnvCodes(
      'release.create',
      formState.application_id.trim(),
      envOptions.value.map((item) => item.value),
    ),
  )
  return envOptions.value.filter((item) => allowedEnvCodes.has(item.value))
})

const releaseEnvNotice = computed(() => {
  if (loadingEnvOptions.value) {
    return ''
  }
  if (envOptions.value.length === 0) {
    return '当前系统尚未配置可用环境，请联系管理员检查发布设置'
  }
  if (formState.application_id.trim() && authorizedEnvOptions.value.length === 0) {
    return '当前账号对所选应用没有可用的环境发布权限'
  }
  return ''
})

const selectedEnvOption = computed(() =>
  authorizedEnvOptions.value.find((item) => item.value === formState.env_code.trim()) || null,
)

const currentUserDisplayName = computed(() => {
  const profile = authStore.profile
  if (!profile) {
    return '-'
  }
  return String(profile.display_name || '').trim() || String(profile.username || '').trim() || '-'
})

const selectedApplicationRecord = computed(() =>
  applicationRecords.value.find((item) => item.id === formState.application_id.trim()) || null,
)

const releaseBranchOptions = computed<SelectOption[]>(() =>
  (selectedApplicationRecord.value?.release_branches || []).map((item) => ({
    label: item.name ? `${item.name} · ${item.branch}` : item.branch,
    value: item.branch,
  })),
)

function templateHasBuiltinSource(keys: string[]) {
  const targetKeys = new Set(keys.map((item) => String(item || '').trim().toLowerCase()).filter(Boolean))
  return templateParams.value.some((item) => {
    const source = String(item.value_source || '').trim().toLowerCase()
    const key = String(item.source_param_key || '').trim().toLowerCase()
    return source === 'builtin' && targetKeys.has(key)
  })
}

const showEnvironmentField = computed(() => templateHasBuiltinSource(['env', 'env_code']))
const showReleaseBranchField = computed(() => templateHasBuiltinSource(['branch', 'git_ref']))

const effectiveEnvCode = computed(() => {
  const explicitEnvCode = formState.env_code.trim()
  if (explicitEnvCode) {
    return explicitEnvCode
  }
  if (showEnvironmentField.value) {
    return ''
  }
  return String(authorizedEnvOptions.value[0]?.value || '').trim()
})

const rules = computed<Record<string, Rule[]>>(() => ({
  application_id: [{ required: true, message: '请选择应用', trigger: 'change' }],
  template_id: [{ required: true, message: '请选择发布模板', trigger: 'change' }],
  ...(showEnvironmentField.value
    ? { env_code: [{ required: true, message: '请选择环境', trigger: 'change' }] }
    : {}),
}))

const bindingMapByScope = computed<Record<ReleasePipelineScope, ReleaseTemplateBinding | null>>(() => ({
  ci: templateBindings.value.find((item) => item.pipeline_scope === 'ci' && item.enabled) || null,
  cd: templateBindings.value.find((item) => item.pipeline_scope === 'cd' && item.enabled) || null,
}))
const hasStagedBuildBindings = computed(() => Boolean(bindingMapByScope.value.ci && bindingMapByScope.value.cd))

const selectedApprovalFlow = computed(() =>
  approvalFlowDefinitions.value.find((item) => item.id === applicationApprovalFlowID.value) || null,
)

const templateParamMetaByScope = computed<Record<ReleasePipelineScope, Record<string, ReleaseTemplateParam>>>(() => {
  const map: Record<ReleasePipelineScope, Record<string, ReleaseTemplateParam>> = {
    ci: {},
    cd: {},
  }
  templateParams.value.forEach((item) => {
    map[item.pipeline_scope][item.executor_param_def_id] = item
  })
  return map
})

const visibleScopes = computed(() => {
  return (['ci', 'cd'] as ReleasePipelineScope[]).filter((scope) => bindingMapByScope.value[scope])
})

const enabledScopeSummary = computed(() => visibleScopes.value.map((scope) => scope.toUpperCase()).join(' + ') || '-')
const advancedParamFillHint = '仅展示需要申请人填写的字段'
const advancedParamApprovalHint = computed(() => {
  if (selectedTemplate.value?.approval_enabled && selectedTemplate.value.approval_approver_ids.length > 0) {
    return '当前模板已启用审批，暂不支持极速发布'
  }
  return ''
})
const advancedParamAriaLabel = computed(() => {
  const templateName = selectedTemplate.value?.name ? `当前模板：${selectedTemplate.value.name}` : '当前模板未选择'
  const approvalText = advancedParamApprovalHint.value ? `，${advancedParamApprovalHint.value}` : ''
  return `${templateName}，已启用 ${enabledScopeSummary.value} 流程${approvalText}，${advancedParamFillHint}`
})

const scopeCardList = computed(() =>
  visibleScopes.value
    .map((scope) => ({
      scope,
      title: scopeLabels[scope],
      binding: bindingMapByScope.value[scope],
      params: visibleAdvancedScopeParams(scope),
      loading: scopeStates[scope].loading,
      error: scopeStates[scope].error,
    }))
    .filter((item) => item.loading || item.error || item.params.length > 0),
)

const hasScopeErrors = computed(() => visibleScopes.value.some((scope) => Boolean(scopeStates[scope].error)))
const isParamLoading = computed(() => loadingTemplateDetail.value || visibleScopes.value.some((scope) => scopeStates[scope].loading))
const isPageLoading = computed(() => {
  if (isEditMode.value) {
    return loadingEditOrder.value
  }
  return loadingApplications.value || loadingEnvOptions.value
})
const canSubmitRelease = computed(() => Boolean(formState.application_id && formState.template_id && selectedTemplate.value && effectiveEnvCode.value) && !hasScopeErrors.value && !isParamLoading.value && !loadingEditOrder.value)
const submitDisabledReason = computed(() => {
  if (!formState.application_id.trim()) {
    return '请先选择应用'
  }
  if (!formState.template_id.trim()) {
    return '请先选择发布模板'
  }
  if (loadingTemplateDetail.value || loadingEditOrder.value) {
    return '当前页面数据仍在加载，请稍后再试'
  }
  if (!selectedTemplate.value) {
    return '当前发布模板详情尚未加载完成'
  }
  if (!effectiveEnvCode.value) {
    return '当前账号对所选应用没有可用的环境发布权限'
  }
  if (hasScopeErrors.value) {
    return '当前模板参数加载失败，请检查模板配置'
  }
  if (isParamLoading.value) {
    return '执行参数仍在加载，请稍后再试'
  }
  return ''
})
const fastReleaseDisabledReason = computed(() => {
  if (isEditMode.value) {
    return '编辑模式下不支持极速发布'
  }
  if (!selectedTemplate.value) {
    return ''
  }
  if (
    Boolean(selectedTemplate.value.approval_enabled) &&
    (selectedTemplate.value.approval_approver_ids || []).length > 0
  ) {
    return '当前模板已配置审批人，极速发布不可用'
  }
  return ''
})
const canFastSubmitRelease = computed(() => canSubmitRelease.value && !fastReleaseDisabledReason.value)
const buildOnlyDisabledReason = computed(() => {
  if (isEditMode.value) {
    return '编辑模式下不支持仅构建'
  }
  if (!selectedTemplate.value) {
    return ''
  }
  if (
    Boolean(selectedTemplate.value.approval_enabled) &&
    (selectedTemplate.value.approval_approver_ids || []).length > 0
  ) {
    return '当前模板已配置审批人，仅构建不可用'
  }
  if (!hasStagedBuildBindings.value) {
    return '当前模板未同时配置 CI / CD，无法仅构建'
  }
  return ''
})
const canBuildOnlySubmitRelease = computed(() => canSubmitRelease.value && hasStagedBuildBindings.value && !buildOnlyDisabledReason.value)
const standardSubmitting = computed(() => submitting.value && submittingMode.value === 'standard')
const fastSubmitting = computed(() => submitting.value && submittingMode.value === 'fast')
const buildOnlySubmitting = computed(() => submitting.value && submittingMode.value === 'build')
const batchSubmitting = computed(() => submitting.value && submittingMode.value === 'batch')
const pageTitle = computed(() => {
  if (isEditMode.value) {
    return '编辑发布单'
  }
  return isBatchMode.value ? '批量新建发布单' : '新建发布单'
})
const primaryActionText = computed(() => {
  if (isEditMode.value) {
    return '保存修改'
  }
  return isBatchMode.value ? '加入创建清单' : '创建发布单'
})

const autoGeneratedReleaseName = computed(() => {
  const appName = selectedApplicationRecord.value?.name || ''
  const templateName = selectedTemplate.value?.name || ''
  if (!appName || !templateName) {
    return '若不填写则在发布时自动生成'
  }
  const now = new Date()
  const pad = (n: number) => n.toString().padStart(2, '0')
  const timestamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  return `${appName}-${templateName}-${timestamp}发布`
})

function resetParamValues() {
  Object.keys(paramValues).forEach((key) => {
    delete paramValues[key]
  })
}

function resetTemplateState() {
  formState.template_id = ''
  formState.git_ref = ''
  templateWarning.value = ''
  selectedTemplate.value = null
  templateBindings.value = []
  templateParams.value = []
  templateOptions.value = []
  templateList.value = []
  scopeStates.ci.error = ''
  scopeStates.ci.param_defs = []
  scopeStates.ci.loading = false
  scopeStates.cd.error = ''
  scopeStates.cd.param_defs = []
  scopeStates.cd.loading = false
  resetParamValues()
}

function resetApplicationApprovalFlowState() {
  applicationApprovalFlowID.value = ''
  approvalFlowDefinitions.value = []
  approvalFlowError.value = ''
}

function formatTemplateOptionLabel(item: ReleaseTemplate) {
  const summary = [item.binding_name, item.binding_type].filter(Boolean).join(' / ')
  const violation = item.compliance_status === 'violated'
    ? `违规：${item.compliance_summary || '管线规范不通过'}`
    : ''
  if (!summary) {
    return violation ? `${item.name}（${violation}）` : item.name
  }
  const label = `${item.name} · ${summary}`
  return violation ? `${label}（${violation}）` : label
}

function isReleaseTemplateSelectable(item: ReleaseTemplate) {
  return item.compliance_status !== 'violated'
}

function applyRouteQuery() {
  if (isEditMode.value) {
    preferredTemplateID.value = ''
    preferredBindingID.value = ''
    return
  }
  const applicationID = String(route.query.application_id || '').trim()
  const templateID = String(route.query.template_id || '').trim()
  const bindingID = String(route.query.binding_id || '').trim()
  if (applicationID) {
    formState.application_id = applicationID
  }
  preferredTemplateID.value = templateID
  preferredBindingID.value = bindingID
}

async function loadApplicationOptions() {
  loadingApplications.value = true
  try {
    const response = await listApplications({ page: 1, page_size: 200 })
    applicationRecords.value = response.data
    allApplicationOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))

    if (!authStore.isAdmin && applicationOptions.value.length === 0) {
      message.warning('当前账号未配置应用发布权限，请联系管理员授权')
    }
    if (!formState.git_ref && selectedApplicationRecord.value?.release_branches?.length === 1) {
      formState.git_ref = String(selectedApplicationRecord.value.release_branches[0]?.branch || '').trim()
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用下拉加载失败'))
  } finally {
    loadingApplications.value = false
  }
}

function isEditableReleaseOrder(orderItem: ReleaseOrder) {
  if (!orderItem) {
    return false
  }
  const isPending = String(orderItem.status || '').trim() === 'pending'
  const isOriginalDeploy =
    String(orderItem.operation_type || '').trim() === 'deploy' &&
    !String(orderItem.source_order_id || '').trim()
  const canOperate =
    authStore.isAdmin ||
    (currentUserID.value !== '' && String(orderItem.creator_user_id || '').trim() === currentUserID.value)
  return isPending && isOriginalDeploy && canOperate
}

function envConfigsFromSettings(configs: ReleaseEnvironmentConfig[], fallbackOptions: string[]): SelectOption[] {
  const result: SelectOption[] = []
  const seen = new Set<string>()
  const source = configs.length > 0
    ? configs
    : fallbackOptions.map((item) => ({ code: item, description: '' }))
  source.forEach((item) => {
    const code = String(item?.code || '').trim()
    if (!code || seen.has(code)) {
      return
    }
    seen.add(code)
    result.push({
      label: code,
      value: code,
      description: String(item?.description || '').trim(),
    })
  })
  return result
}

async function ensureEditingApplicationOption() {
  const applicationID = String(editingOrder.value?.application_id || '').trim()
  if (!applicationID) {
    return
  }
  if (applicationRecords.value.some((item) => item.id === applicationID)) {
    return
  }
  try {
    const response = await getApplicationByID(applicationID)
    const record = response.data
    applicationRecords.value = [...applicationRecords.value, record]
    allApplicationOptions.value = [
      ...allApplicationOptions.value,
      {
        label: `${record.name} (${record.key})`,
        value: record.id,
      },
    ]
  } catch {
    // Ignore application detail lookup failures here and let the form surface later issues.
  }
}

async function loadEditingOrderSnapshot() {
  if (!isEditMode.value) {
    return
  }
  loadingEditOrder.value = true
  try {
    const [orderResp, paramsResp] = await Promise.all([
      getReleaseOrderByID(editingOrderID.value),
      listReleaseOrderParams(editingOrderID.value),
    ])
    if (!isEditableReleaseOrder(orderResp.data)) {
      message.warning('当前发布单不是可编辑的待执行普通发布单')
      void router.replace(`/releases/${editingOrderID.value}`)
      return
    }
    editingOrder.value = orderResp.data
    editingParamSnapshot.value = paramsResp.data
    formState.application_id = String(orderResp.data.application_id || '').trim()
    formState.template_id = ''
    formState.release_name = String(orderResp.data.release_name || '').trim()
    formState.env_code = String(orderResp.data.env_code || '').trim()
    formState.git_ref = String(orderResp.data.git_ref || '').trim()
    formState.remark = String(orderResp.data.remark || '').trim()
    preferredTemplateID.value = String(orderResp.data.template_id || '').trim()
    preferredBindingID.value = ''
    pendingEditSnapshotRestore.value = true
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '待编辑发布单加载失败'))
    void router.replace('/releases')
  } finally {
    loadingEditOrder.value = false
  }
}

async function loadEnvOptions() {
  loadingEnvOptions.value = true
  try {
    const response = await getReleaseSettings()
    envOptions.value = envConfigsFromSettings(response.data.env_configs || [], response.data.env_options || [])
    defaultEnvCode.value = response.data.default_env_code || ''
    syncSelectedEnvCode()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '环境选项加载失败'))
  } finally {
    loadingEnvOptions.value = false
  }
}

function syncSelectedEnvCode() {
  const availableEnvCodes = authorizedEnvOptions.value.map((item) => item.value)
  if (formState.env_code && !availableEnvCodes.includes(formState.env_code)) {
    formState.env_code = ''
  }
  if (!formState.env_code && defaultEnvCode.value && availableEnvCodes.includes(defaultEnvCode.value)) {
    formState.env_code = defaultEnvCode.value
  }
}

function resetSelectionIfUnauthorized() {
  if (isEditMode.value) {
    syncSelectedEnvCode()
    return
  }
  const hasCurrentApplication = applicationOptions.value.some((item) => item.value === formState.application_id)
  if (hasCurrentApplication) {
    syncSelectedEnvCode()
    return
  }
  formState.application_id = ''
  formState.env_code = ''
  resetTemplateState()
  resetApplicationApprovalFlowState()
}

async function findTemplateByBinding(templates: ReleaseTemplate[], bindingID: string) {
  const target = String(bindingID || '').trim()
  if (!target) {
    return ''
  }
  for (const item of templates) {
    try {
      const response = await getReleaseTemplateByID(item.id)
      const matched = response.data.bindings.some((binding) => binding.binding_id === target && binding.enabled)
      if (matched) {
        return item.id
      }
    } catch {
      // ignore single item lookup failures and continue searching
    }
  }
  return ''
}

async function loadApplicationApprovalFlow() {
  const applicationID = formState.application_id.trim()
  resetApplicationApprovalFlowState()
  if (!applicationID) {
    return
  }
  loadingApprovalFlow.value = true
  try {
    const [bindingResponse, flowResponse] = await Promise.all([
      getApplicationApprovalFlowBinding(applicationID),
      listApprovalFlows(),
    ])
    if (formState.application_id.trim() !== applicationID) {
      return
    }
    applicationApprovalFlowID.value = String(bindingResponse.data.approval_flow_id || '').trim()
    approvalFlowDefinitions.value = flowResponse.data || []
    if (applicationApprovalFlowID.value && !selectedApprovalFlow.value) {
      approvalFlowError.value = '应用绑定的审批流已停用或不存在，请联系管理员检查审批流配置'
    }
  } catch (error) {
    if (formState.application_id.trim() === applicationID) {
      approvalFlowError.value = extractHTTPErrorMessage(error, '应用审批流加载失败')
    }
  } finally {
    if (formState.application_id.trim() === applicationID) {
      loadingApprovalFlow.value = false
    }
  }
}

async function loadTemplateOptions() {
  const applicationID = formState.application_id.trim()
  const preservedGitRef = pendingEditSnapshotRestore.value ? formState.git_ref.trim() : ''
  resetTemplateState()
  if (preservedGitRef) {
    formState.git_ref = preservedGitRef
  }
  if (!applicationID) {
    return
  }

  loadingTemplates.value = true
  try {
    const templates = await listAllReleaseTemplates({
      application_id: applicationID,
      status: 'active',
    })
    templateList.value = templates
    templateOptions.value = templates.map((item) => ({
      label: formatTemplateOptionLabel(item),
      value: item.id,
      disabled: !isReleaseTemplateSelectable(item),
    }))

    let nextTemplateID = ''
    if (preferredTemplateID.value && templateOptions.value.some((item) => item.value === preferredTemplateID.value && !item.disabled)) {
      nextTemplateID = preferredTemplateID.value
      preferredTemplateID.value = ''
    } else if (preferredBindingID.value) {
      nextTemplateID = await findTemplateByBinding(templates, preferredBindingID.value)
      preferredBindingID.value = ''
    } else if (templates.length === 1 && templates[0] && isReleaseTemplateSelectable(templates[0])) {
      nextTemplateID = templates[0].id
    }

    if (nextTemplateID) {
      formState.template_id = nextTemplateID
      await loadSelectedTemplateDetail()
    } else if (templates.length === 0) {
      templateWarning.value = '当前应用下还没有启用中的发布模板，请先到“发布模板”页面完成配置'
    } else if (!templates.some(isReleaseTemplateSelectable)) {
      templateWarning.value = '当前应用下的发布模板均违反管线规范，请先调整模板绑定的管线'
    } else {
      templateWarning.value = '请选择一个发布模板后继续填写参数'
    }
  } catch (error) {
    templateWarning.value = ''
    message.error(extractHTTPErrorMessage(error, '发布模板加载失败'))
  } finally {
    loadingTemplates.value = false
  }
}

async function loadScopeParamDefs(scope: ReleasePipelineScope) {
  const binding = bindingMapByScope.value[scope]
  scopeStates[scope].error = ''
  scopeStates[scope].param_defs = []

  if (!binding) {
    return
  }

  if (binding.provider !== 'jenkins') {
    return
  }

  scopeStates[scope].loading = true
  try {
    const response = await listApplicationExecutorParamDefs(formState.application_id, {
      binding_type: scope,
      binding_id: binding.binding_id,
      status: 'active',
      page: 1,
      page_size: 200,
    })
    const scopedTemplateParams = templateParams.value.filter((item) => item.pipeline_scope === scope)
    const allowedIDs = new Set(scopedTemplateParams.map((item) => item.executor_param_def_id))
    let resolvedParamDefs = response.data.filter((item) => allowedIDs.has(item.id))

    // 某些模板在应用级参数列表里会拿不到绑定对应的定义，逐条回退查询避免页面一直空白
    if (resolvedParamDefs.length === 0 && scopedTemplateParams.length > 0) {
      const fallbackDefs = await Promise.all(
        scopedTemplateParams.map(async (item) => {
          try {
            const detail = await getExecutorParamDefByID(item.executor_param_def_id)
            return detail.data
          } catch {
            return null
          }
        }),
      )
      resolvedParamDefs = fallbackDefs.filter((item): item is ExecutorParamDef => Boolean(item))
    }

    scopeStates[scope].param_defs = resolvedParamDefs

    const valueMap = templateParamMetaByScope.value[scope]
    scopeStates[scope].param_defs.forEach((item) => {
      if (paramValues[item.id] !== undefined) {
        return
      }
      const meta = valueMap[item.id]
      if (meta?.required && item.default_value) {
        paramValues[item.id] = item.default_value
        return
      }
      paramValues[item.id] = item.default_value || ''
    })
  } catch (error) {
    scopeStates[scope].error = extractHTTPErrorMessage(error, `${scope.toUpperCase()} 参数加载失败`)
    scopeStates[scope].param_defs = []
  } finally {
    scopeStates[scope].loading = false
  }
}

function restoreEditingParamValues() {
  if (!pendingEditSnapshotRestore.value) {
    return
  }
  const snapshot = editingParamSnapshot.value
  if (snapshot.length === 0) {
    pendingEditSnapshotRestore.value = false
    return
  }
  for (const scope of visibleScopes.value) {
    const items = visibleAdvancedScopeParams(scope)
    for (const item of items) {
      const matched = snapshot.find((param) => {
        if (param.pipeline_scope !== scope) {
          return false
        }
        const executorParamName = String(param.executor_param_name || '').trim().toLowerCase()
        const currentExecutorParamName = String(item.executor_param_name || '').trim().toLowerCase()
        if (executorParamName && currentExecutorParamName) {
          return executorParamName === currentExecutorParamName
        }
        return String(param.param_key || '').trim().toLowerCase() === String(item.param_key || '').trim().toLowerCase()
      })
      if (matched) {
        paramValues[item.id] = String(matched.param_value || '')
      }
    }
  }
  pendingEditSnapshotRestore.value = false
}

async function loadSelectedTemplateDetail() {
  const templateID = formState.template_id.trim()
  if (!templateID) {
    selectedTemplate.value = null
    templateBindings.value = []
    templateParams.value = []
    scopeStates.ci.param_defs = []
    scopeStates.cd.param_defs = []
    resetParamValues()
    templateWarning.value = '请选择一个发布模板后继续填写参数'
    return
  }

  loadingTemplateDetail.value = true
  try {
    const response = await getReleaseTemplateByID(templateID)
    if (response.data.template.compliance_status === 'violated') {
      selectedTemplate.value = null
      templateBindings.value = []
      templateParams.value = []
      formState.template_id = ''
      resetParamValues()
      templateWarning.value = response.data.template.compliance_summary
        ? `发布模板违反管线规范：${response.data.template.compliance_summary}`
        : '发布模板违反管线规范，不能用于新建发布单'
      message.warning(templateWarning.value)
      return
    }
    selectedTemplate.value = response.data.template
    templateBindings.value = response.data.bindings
    templateParams.value = response.data.params
    resetParamValues()
    await Promise.all([
      loadScopeParamDefs('ci'),
      loadScopeParamDefs('cd'),
    ])
    restoreEditingParamValues()
    templateWarning.value = ''
  } catch (error) {
    selectedTemplate.value = null
    templateBindings.value = []
    templateParams.value = []
    formState.template_id = ''
    resetParamValues()
    message.error(extractHTTPErrorMessage(error, '发布模板详情加载失败'))
  } finally {
    loadingTemplateDetail.value = false
  }

  syncingTemplateParams.value = true
  try {
    const result = await syncTemplateExecutorParamDefs(templateID)
    const detail = result.data
    message.success(
      `管线参数同步完成：共 ${detail.total} 个参数，新增 ${detail.created} 个，更新 ${detail.updated} 个`,
    )
    await Promise.all([
      loadScopeParamDefs('ci'),
      loadScopeParamDefs('cd'),
    ])
  } catch (error) {
    console.error('auto sync pipeline params failed:', error)
  } finally {
    syncingTemplateParams.value = false
  }
}

async function handleApplicationChange(value: string | undefined) {
  formState.application_id = String(value || '')
  preferredTemplateID.value = ''
  preferredBindingID.value = ''
  syncSelectedEnvCode()
  formState.git_ref = ''
  if (releaseBranchOptions.value.length === 1 && releaseBranchOptions.value[0]) {
    formState.git_ref = releaseBranchOptions.value[0].value
  }
  await Promise.all([loadTemplateOptions(), loadApplicationApprovalFlow()])
}

async function handleTemplateChange(value: string | undefined) {
  formState.template_id = String(value || '')
  await loadSelectedTemplateDetail()
}

function goBack() {
  if (isEditMode.value && editingOrderID.value) {
    void router.push(`/releases/${editingOrderID.value}`)
    return
  }
  void router.push('/releases')
}

function resolveTemplateParamLabel(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  const meta = templateParamMetaByScope.value[scope][item.id]
  return meta?.param_name || item.param_key || item.executor_param_name || item.id
}

function scopeTemplateParamDefs(scope: ReleasePipelineScope) {
  return scopeStates[scope].param_defs
}

function resolveTemplateParamValueSource(meta?: ReleaseTemplateParam | null) {
  const value = String(meta?.value_source || '').trim().toLowerCase()
  if (value === 'fixed' || value === 'ci_param' || value === 'builtin' || value === 'release_input') {
    return value
  }
  return 'release_input'
}

function isTemplateParamEditable(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  const meta = templateParamMetaByScope.value[scope][item.id]
  return resolveTemplateParamValueSource(meta) === 'release_input'
}

function isTemplateParamMappedFromBaseField(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  const meta = templateParamMetaByScope.value[scope][item.id]
  return resolveTemplateParamValueSource(meta) === 'builtin'
}

function isTemplateParamInheritedFromCiParam(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  const meta = templateParamMetaByScope.value[scope][item.id]
  return scope === 'cd' && resolveTemplateParamValueSource(meta) === 'ci_param'
}

function isTemplateParamVisibleInReleaseForm(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  return (
    isTemplateParamEditable(scope, item) &&
    !isTemplateParamMappedFromBaseField(scope, item) &&
    !isTemplateParamInheritedFromCiParam(scope, item)
  )
}

function visibleAdvancedScopeParams(scope: ReleasePipelineScope) {
  return scopeTemplateParamDefs(scope).filter((item) => isTemplateParamVisibleInReleaseForm(scope, item))
}

const defaultChoiceMeta: ChoiceMeta = {
  options: [],
  multiple: false,
  delimiter: ',',
}

function readChoiceDelimiter(meta: Record<string, unknown>) {
  const raw = [meta.multiSelectDelimiter, meta.multi_select_delimiter, meta.valueDelimiter, meta.delimiter, meta.separator]
  for (const item of raw) {
    const value = String(item || '').trim()
    if (value) {
      return value
    }
  }
  return ','
}

function splitChoiceText(value: string): string[] {
  const text = value.trim()
  if (!text) {
    return []
  }
  if (text.includes('\n') || text.includes('\r')) {
    return text
      .replace(/\r\n/g, '\n')
      .replace(/\r/g, '\n')
      .split('\n')
      .map((item) => item.trim())
      .filter(Boolean)
  }
  if (text.includes(',')) {
    return text
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return [text]
}

function dedupe(values: string[]) {
  const result: string[] = []
  const seen = new Set<string>()
  values.forEach((item) => {
    if (!item || seen.has(item)) {
      return
    }
    seen.add(item)
    result.push(item)
  })
  return result
}

function dedupeChoiceOptions(options: SelectOption[]) {
  const result: SelectOption[] = []
  const seen = new Set<string>()
  options.forEach((item) => {
    const value = String(item.value ?? '').trim()
    const label = String(item.label ?? '').trim() || value
    if (!value || seen.has(value)) {
      return
    }
    seen.add(value)
    result.push({ label, value })
  })
  return result
}

function readChoiceOptionText(raw: unknown) {
  return String(raw ?? '').trim()
}

function normalizeChoiceOptions(raw: unknown): SelectOption[] {
  if (Array.isArray(raw)) {
    const options: SelectOption[] = []
    raw.forEach((item) => {
      if (item && typeof item === 'object') {
        const objectRaw = item as Record<string, unknown>
        const value = readChoiceOptionText(
          objectRaw.value ?? objectRaw.id ?? objectRaw.key ?? objectRaw.name ?? objectRaw.label,
        )
        const label = readChoiceOptionText(
          objectRaw.label ?? objectRaw.name ?? objectRaw.text ?? objectRaw.description ?? value,
        )
        if (value) {
          options.push({ label: label || value, value })
        }
        return
      }
      splitChoiceText(readChoiceOptionText(item)).forEach((value) => {
        options.push({ label: value, value })
      })
    })
    return dedupeChoiceOptions(options)
  }
  if (typeof raw === 'string') {
    return dedupeChoiceOptions(splitChoiceText(raw).map((value) => ({ label: value, value })))
  }
  if (raw && typeof raw === 'object') {
    const objectRaw = raw as Record<string, unknown>
    for (const key of ['choiceOptions', 'options', 'choices', 'choiceList', 'values', 'items', 'list', 'value']) {
      const options = normalizeChoiceOptions(objectRaw[key])
      if (options.length > 0) {
        return options
      }
    }
  }
  return []
}

function normalizeChoiceValues(raw: unknown): string[] {
  return dedupe(normalizeChoiceOptions(raw).map((item) => item.value))
}

function resolveChoiceMeta(item: ExecutorParamDef): ChoiceMeta {
  const raw = String(item.raw_meta || '').trim()
  if (!raw) {
    return {
      ...defaultChoiceMeta,
      multiple: false,
    }
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const options = normalizeChoiceOptions(
      parsed.choiceOptions ?? parsed.options ?? parsed.choices ?? parsed.choiceList ?? parsed.values ?? parsed.value ?? parsed.items ?? null,
    )

    const className = String(parsed._class || '').toLowerCase()
    const typeName = String(parsed.type || parsed.choiceType || parsed.ptype || '').toLowerCase()
    const delimiter = readChoiceDelimiter(parsed)
    const inferredMulti =
      Boolean(parsed.multiSelect) ||
      Boolean(parsed.multi_select) ||
      Boolean(parsed.isMulti) ||
      typeName.includes('multi') ||
      typeName.includes('checkbox') ||
      className.includes('multi')
    const multiple =
      item.single_select
        ? false
        : inferredMulti || (item.param_type === 'choice' && Boolean(delimiter && String(item.default_value || '').includes(delimiter) && options.length > 1))

    return {
      options,
      multiple,
      delimiter,
    }
  } catch {
    return defaultChoiceMeta
  }
}

const choiceMetaMap = computed<Record<string, ChoiceMeta>>(() => {
  const map: Record<string, ChoiceMeta> = {}
  ;(['ci', 'cd'] as ReleasePipelineScope[]).forEach((scope) => {
    scopeStates[scope].param_defs.forEach((item) => {
      map[item.id] = resolveChoiceMeta(item)
    })
  })
  return map
})

function getChoiceMeta(item: ExecutorParamDef) {
  return choiceMetaMap.value[item.id] || defaultChoiceMeta
}

function getParamSelectOptions(item: ExecutorParamDef) {
  return getChoiceMeta(item).options
}

const expandedChoiceParam = computed(() => {
  const paramID = expandedChoiceState.paramID.trim()
  if (!paramID) {
    return null
  }
  return scopeStates[expandedChoiceState.scope].param_defs.find((item) => item.id === paramID) || null
})

const expandedChoiceOptions = computed<SelectOption[]>(() => (
  expandedChoiceParam.value ? getParamSelectOptions(expandedChoiceParam.value) : []
))

const filteredExpandedChoiceOptions = computed<SelectOption[]>(() => {
  const keyword = expandedChoiceState.keyword.trim().toLowerCase()
  if (!keyword) {
    return expandedChoiceOptions.value
  }
  return expandedChoiceOptions.value.filter((option) => {
    const label = String(option.label || '').toLowerCase()
    const value = String(option.value || '').toLowerCase()
    return label.includes(keyword) || value.includes(keyword)
  })
})

const expandedChoiceSelectedValues = computed(() => (
  expandedChoiceParam.value ? getChoiceMultiValues(expandedChoiceParam.value) : []
))

const expandedChoiceSelectedValueSet = computed(() => new Set(expandedChoiceSelectedValues.value))
const expandedChoiceSelectedCount = computed(() => expandedChoiceSelectedValues.value.length)
const expandedChoiceTotalCount = computed(() => expandedChoiceOptions.value.length)
const expandedChoiceTitle = computed(() => {
  const param = expandedChoiceParam.value
  if (!param) {
    return '选择参数'
  }
  return param.param_key || param.executor_param_name || '选择参数'
})

function hasChoiceOptionConstraint(item: ExecutorParamDef) {
  return getParamSelectOptions(item).length > 0
}

function isBranchLikeParam(item: ExecutorParamDef) {
  const candidates = [
    String(item.param_key || '').trim().toLowerCase(),
    String(item.executor_param_name || '').trim().toLowerCase(),
  ]
  return candidates.some((item) => item === 'branch' || item === 'git_ref' || item === 'gitref')
}

function shouldSkipChoiceValidation(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  if (!hasChoiceOptionConstraint(item)) {
    return true
  }
  const meta = templateParamMetaByScope.value[scope][item.id]
  const source = resolveTemplateParamValueSource(meta)
  const sourceParamKey = String(meta?.source_param_key || '').trim().toLowerCase()
  if (source === 'builtin' && (sourceParamKey === 'branch' || sourceParamKey === 'git_ref')) {
    return true
  }
  if (isBranchLikeParam(item) && formState.git_ref.trim()) {
    return true
  }
  return false
}

function isMultipleChoice(_scope: ReleasePipelineScope, item: ExecutorParamDef) {
  return getChoiceMeta(item).multiple
}

function useChoiceSelect(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  return isTemplateParamEditable(scope, item) && hasChoiceOptionConstraint(item)
}

function splitByDelimiter(value: string, delimiter: string) {
  const text = String(value || '').trim()
  if (!text) {
    return []
  }
  if (delimiter && text.includes(delimiter)) {
    return text
      .split(delimiter)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return splitChoiceText(text)
}

function getChoiceSingleValue(scope: ReleasePipelineScope, item: ExecutorParamDef): string | undefined {
  const value = String(paramValues[item.id] || '')
  if (!value) {
    return undefined
  }
  if (!isMultipleChoice(scope, item)) {
    return value
  }
  const first = splitByDelimiter(value, getChoiceMeta(item).delimiter)[0]
  return first || undefined
}

function getChoiceMultiValues(item: ExecutorParamDef): string[] {
  const value = String(paramValues[item.id] || '')
  if (!value) {
    return []
  }
  return splitByDelimiter(value, getChoiceMeta(item).delimiter)
}

function handleChoiceSingleChange(item: ExecutorParamDef, value: unknown) {
  paramValues[item.id] = String(value || '')
}

function handleChoiceMultiChange(item: ExecutorParamDef, values: unknown) {
  const list = Array.isArray(values)
    ? values
        .map((value) => String(value || '').trim())
        .filter(Boolean)
    : []
  writeChoiceMultiValues(item, list)
}

function handleChoiceSelectAll(item: ExecutorParamDef) {
  const all = getParamSelectOptions(item).map((option) => option.value)
  writeChoiceMultiValues(item, all)
}

function handleChoiceDeselectAll(item: ExecutorParamDef) {
  paramValues[item.id] = ''
}

function writeChoiceMultiValues(item: ExecutorParamDef, values: string[]) {
  const list = dedupe(values.map((value) => String(value || '').trim()).filter(Boolean))
  paramValues[item.id] = list.join(getChoiceMeta(item).delimiter || ',')
}

function openExpandedChoiceModal(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  expandedChoiceState.scope = scope
  expandedChoiceState.paramID = item.id
  expandedChoiceState.keyword = ''
  expandedChoiceState.open = true
}

function closeExpandedChoiceModal() {
  expandedChoiceState.open = false
  expandedChoiceState.keyword = ''
}

function isExpandedChoiceChecked(value: string) {
  return expandedChoiceSelectedValueSet.value.has(value)
}

function readCheckboxChecked(event: unknown) {
  const target = (event as { target?: { checked?: boolean } })?.target
  return Boolean(target?.checked)
}

function handleExpandedChoiceToggle(value: string, event: unknown) {
  const param = expandedChoiceParam.value
  if (!param) {
    return
  }
  const checked = readCheckboxChecked(event)
  const current = new Set(expandedChoiceSelectedValues.value)
  if (checked) {
    current.add(value)
  } else {
    current.delete(value)
  }
  writeChoiceMultiValues(param, Array.from(current))
}

function handleExpandedChoiceSelectFiltered() {
  const param = expandedChoiceParam.value
  if (!param) {
    return
  }
  writeChoiceMultiValues(param, [
    ...expandedChoiceSelectedValues.value,
    ...filteredExpandedChoiceOptions.value.map((option) => option.value),
  ])
}

function handleExpandedChoiceDeselectFiltered() {
  const param = expandedChoiceParam.value
  if (!param) {
    return
  }
  const removeSet = new Set(filteredExpandedChoiceOptions.value.map((option) => option.value))
  writeChoiceMultiValues(
    param,
    expandedChoiceSelectedValues.value.filter((value) => !removeSet.has(value)),
  )
}

function handleExpandedChoiceSelectAll() {
  const param = expandedChoiceParam.value
  if (!param) {
    return
  }
  writeChoiceMultiValues(param, expandedChoiceOptions.value.map((option) => option.value))
}

function handleExpandedChoiceDeselectAll() {
  const param = expandedChoiceParam.value
  if (!param) {
    return
  }
  handleChoiceDeselectAll(param)
}

function resolveInvalidChoiceValues(scope: ReleasePipelineScope, item: ExecutorParamDef): string[] {
  if (shouldSkipChoiceValidation(scope, item)) {
    return []
  }
  const allowed = new Set(
    getParamSelectOptions(item).map((option) => String(option.value || '').trim()),
  )
  if (allowed.size === 0) {
    return []
  }
  const values = isMultipleChoice(scope, item)
    ? getChoiceMultiValues(item)
    : (() => {
        const single = getChoiceSingleValue(scope, item)
        return single ? [single.trim()] : []
      })()
  return values.filter((value) => value && !allowed.has(value.trim()))
}

function resolveParamChoiceValidationError(scope: ReleasePipelineScope, item: ExecutorParamDef) {
  const invalidValues = resolveInvalidChoiceValues(scope, item)
  if (invalidValues.length === 0) {
    return ''
  }
  if (invalidValues.length === 1) {
    return `输入值“${invalidValues[0]}”不在下拉可选项中，请重新选择`
  }
  return `存在 ${invalidValues.length} 个值不在下拉可选项中，请重新选择`
}

function buildParamsPayload(): CreateReleaseOrderParamPayload[] {
  const payload: CreateReleaseOrderParamPayload[] = []

  for (const scope of visibleScopes.value) {
    const items = visibleAdvancedScopeParams(scope)
    for (const item of items) {
      const value = String(paramValues[item.id] || '').trim()
      const label = resolveTemplateParamLabel(scope, item)
      const choiceError = resolveParamChoiceValidationError(scope, item)
      if (choiceError) {
        throw new Error(`参数 ${label} 校验失败：${choiceError}`)
      }
      if (!value) {
        throw new Error(`参数 ${label} 为必填，请填写发布值`)
      }
      payload.push({
        pipeline_scope: scope,
        param_key: String(item.param_key || '').trim(),
        executor_param_name: item.executor_param_name,
        param_value: value,
        value_source: 'release_input',
      })
    }
  }

  return payload
}

async function buildReleasePayload(): Promise<CreateReleaseOrderPayload | null> {
  try {
    await formRef.value?.validate()
  } catch {
    return null
  }

  if (!canSubmitRelease.value || !selectedTemplate.value) {
    message.warning('请先选择可用发布模板，并等待参数加载完成')
    return null
  }

  const requiresBuiltinBranch = templateParams.value.some((item) => {
    const source = String(item.value_source || '').trim().toLowerCase()
    const key = String(item.source_param_key || '').trim().toLowerCase()
    return source === 'builtin' && (key === 'branch' || key === 'git_ref')
  })
  if (requiresBuiltinBranch && !formState.git_ref.trim()) {
    message.error('当前模板已使用发布基础字段中的分支，请先选择发布分支')
    return null
  }

  let paramsPayload: CreateReleaseOrderParamPayload[]
  try {
    paramsPayload = buildParamsPayload()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '发布参数校验失败')
    return null
  }

  return {
    application_id: formState.application_id.trim(),
    template_id: formState.template_id.trim(),
    release_name: formState.release_name.trim(),
    env_code: effectiveEnvCode.value,
    git_ref: showReleaseBranchField.value ? (formState.git_ref.trim() || undefined) : undefined,
    trigger_type: 'manual',
    triggered_by: currentUserDisplayName.value !== '-' ? currentUserDisplayName.value : undefined,
    remark: formState.remark.trim() || undefined,
    params: paramsPayload.length > 0 ? paramsPayload : undefined,
  }
}

function cloneReleasePayload(payload: CreateReleaseOrderPayload): CreateReleaseOrderPayload {
  return {
    ...payload,
    params: payload.params?.map((item) => ({ ...item })),
    steps: payload.steps?.map((item) => ({ ...item })),
  }
}

async function submitRelease(options?: { fast?: boolean; buildOnly?: boolean }) {
  const payload = await buildReleasePayload()
  if (!payload) {
    return
  }

  const fast = Boolean(options?.fast)
  const buildOnly = Boolean(options?.buildOnly)
  submitting.value = true
  submittingMode.value = buildOnly ? 'build' : fast ? 'fast' : 'standard'
  try {
    const response = isEditMode.value
      ? await updateReleaseOrder(editingOrderID.value, payload)
      : await createReleaseOrder(payload)
    if (buildOnly) {
      try {
        const buildResponse = await buildReleaseOrder(response.data.id)
        if (['pending_approval', 'approving'].includes(buildResponse.data.business_status || buildResponse.data.status)) {
          message.success('发布单创建成功，审批流程已发起')
        } else {
          message.success('发布单创建成功，已提交仅构建任务')
        }
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '发布单已创建，但仅构建提交失败'))
      }
      void router.push(`/releases/${response.data.id}`)
      return
    }
    if (fast) {
      message.success('极速发布单创建成功，正在进入详情并自动开始发布')
      void router.push({
        path: `/releases/${response.data.id}`,
        query: {
          fast_execute: '1',
        },
      })
      return
    }
    message.success(isEditMode.value ? '发布单修改成功' : '发布单创建成功')
    void router.push(`/releases/${response.data.id}`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, isEditMode.value ? '发布单修改失败' : '发布单创建失败'))
  } finally {
    submitting.value = false
    submittingMode.value = ''
  }
}

async function handleAddBatchDraft() {
  if (submitting.value) {
    return
  }
  if (batchDrafts.value.length >= 50) {
    message.warning('一次最多创建 50 张发布单')
    return
  }
  const payload = await buildReleasePayload()
  if (!payload) {
    return
  }
  batchDrafts.value.push({
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
    payload: cloneReleasePayload(payload),
    applicationName: selectedApplicationRecord.value?.name || payload.application_id,
    templateName: selectedTemplate.value?.name || payload.template_id,
    envName: selectedEnvOption.value?.label || payload.env_code || '-',
    displayName: payload.release_name || '自动生成发布名称',
    error: '',
  })
  formState.release_name = ''
  message.success(`已加入创建清单，共 ${batchDrafts.value.length} 张`)
}

function removeBatchDraft(id: string) {
  batchDrafts.value = batchDrafts.value.filter((item) => item.id !== id)
}

async function loadBatchDraftForEdit(draft: BatchReleaseOrderDraft) {
  const payload = draft.payload
  await handleApplicationChange(payload.application_id)
  await handleTemplateChange(payload.template_id)
  formState.release_name = payload.release_name || ''
  formState.env_code = payload.env_code || ''
  formState.git_ref = payload.git_ref || ''
  formState.remark = payload.remark || ''

  for (const scope of visibleScopes.value) {
    for (const param of visibleAdvancedScopeParams(scope)) {
      const matched = payload.params?.find((item) => (
        item.pipeline_scope === scope && (
          String(item.executor_param_name || '').trim().toLowerCase() === String(param.executor_param_name || '').trim().toLowerCase()
          || String(item.param_key || '').trim().toLowerCase() === String(param.param_key || '').trim().toLowerCase()
        )
      ))
      if (matched) {
        paramValues[param.id] = matched.param_value
      }
    }
  }
  removeBatchDraft(draft.id)
  message.info('已载入表单，修改后请重新加入创建清单')
}

async function handleBatchCreate() {
  if (submitting.value) {
    return
  }
  if (batchDrafts.value.length === 0) {
    message.warning('请先把至少一张发布单加入创建清单')
    return
  }

  const submittedDrafts = [...batchDrafts.value]
  submitting.value = true
  submittingMode.value = 'batch'
  try {
    const response = await batchCreateReleaseOrders({
      orders: submittedDrafts.map((item) => cloneReleasePayload(item.payload)),
    })
    const createdCount = response.data.orders.length
    const failures = response.data.failures || []
    if (failures.length === 0) {
      message.success(`已成功创建 ${createdCount} 张发布单`)
      void router.push('/releases')
      return
    }

    batchDrafts.value = failures
      .map((failure) => {
        const draft = submittedDrafts[failure.index]
        return draft ? { ...draft, error: failure.error || '创建失败' } : null
      })
      .filter((item): item is BatchReleaseOrderDraft => Boolean(item))
    message.warning(`成功创建 ${createdCount} 张，失败 ${failures.length} 张；失败项已保留，可载入修改后重试`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '批量创建发布单失败'))
  } finally {
    submitting.value = false
    submittingMode.value = ''
  }
}

async function handleSubmit() {
  if (!canSubmitRelease.value) {
    if (submitDisabledReason.value) {
      message.warning(submitDisabledReason.value)
    }
    return
  }
  if (isBatchMode.value) {
    await handleAddBatchDraft()
    return
  }
  await submitRelease()
}

async function handleFastSubmit() {
  if (!canFastSubmitRelease.value) {
    if (fastReleaseDisabledReason.value) {
      message.warning(fastReleaseDisabledReason.value)
    }
    return
  }
  await submitRelease({ fast: true })
}

async function handleBuildOnlySubmit() {
  if (!canBuildOnlySubmitRelease.value) {
    const blockedReason = buildOnlyDisabledReason.value || submitDisabledReason.value
    if (blockedReason) {
      message.warning(blockedReason)
    }
    return
  }
  await submitRelease({ buildOnly: true })
}

onMounted(async () => {
  await authStore.loadMe(true)
  applyRouteQuery()
  await loadEditingOrderSnapshot()
  await Promise.all([loadApplicationOptions(), loadEnvOptions()])
  await ensureEditingApplicationOption()
  resetSelectionIfUnauthorized()
  if (formState.application_id) {
    await Promise.all([loadTemplateOptions(), loadApplicationApprovalFlow()])
  }
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header create-page-header">
      <div class="page-header-main">
        <div class="page-header-copy">
          <h2 class="page-title">{{ pageTitle }}</h2>
        </div>
      </div>
      <div class="page-header-actions">
        <a-button
          class="application-toolbar-action-btn"
          :class="{ 'application-toolbar-action-btn-disabled': !canSubmitRelease }"
          :loading="standardSubmitting"
          :aria-disabled="!canSubmitRelease"
          @click="handleSubmit"
        >
          <template #icon>
            <PlusOutlined v-if="isBatchMode" />
            <RocketOutlined v-else />
          </template>
          {{ primaryActionText }}
        </a-button>
        <a-button
          v-if="isBatchMode"
          class="application-toolbar-action-btn release-build-toolbar-btn"
          disabled
          :loading="batchSubmitting"
        >
          <template #icon>
            <RocketOutlined />
          </template>
          批量创建（{{ batchDrafts.length }}）
        </a-button>
        <a-button
          v-if="!isEditMode && !isBatchMode"
          class="application-toolbar-action-btn release-build-toolbar-btn"
          :class="{ 'release-build-toolbar-btn-disabled': !canBuildOnlySubmitRelease }"
          :loading="buildOnlySubmitting"
          :aria-disabled="!canBuildOnlySubmitRelease"
          @click="handleBuildOnlySubmit"
        >
          <template #icon>
            <BranchesOutlined />
          </template>
          仅构建
        </a-button>
        <a-button class="application-toolbar-action-btn" @click="goBack">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          {{ isEditMode ? '返回详情' : '返回发布单' }}
        </a-button>
      </div>
    </div>

    <a-form
      ref="formRef"
      class="release-create-form application-form-plain"
      layout="vertical"
      :model="formState"
      :rules="rules"
      :required-mark="false"
      autocomplete="off"
    >
      <div class="create-layout">
        <div class="create-main">
          <section class="form-section release-form-section">
            <div class="form-section-heading">
              <span class="form-section-bar"></span>
              <h3 class="form-section-heading-title">发布基础</h3>
            </div>

            <a-skeleton :loading="isPageLoading" active :title="false" :paragraph="{ rows: 6 }">
              <a-row :gutter="12" class="form-row-compact">
          <a-col :xs="24" :md="12">
            <a-form-item class="form-item-compact" name="application_id">
              <template #label>
                <span class="field-label-with-hint application-field-label">
                  应用
                  <span class="field-required-hint">必填</span>
                  <a-tag v-if="loadingApprovalFlow" class="application-approval-flow-tag application-approval-flow-tag-loading">
                    审批流加载中
                  </a-tag>
                  <a-tooltip v-else-if="approvalFlowError" :title="approvalFlowError">
                    <a-tag class="application-approval-flow-tag application-approval-flow-tag-error">审批流异常</a-tag>
                  </a-tooltip>
                  <a-tooltip v-else-if="selectedApprovalFlow" :title="selectedApprovalFlow.name">
                    <a-tag class="application-approval-flow-tag">{{ selectedApprovalFlow.name }}</a-tag>
                  </a-tooltip>
                  <a-tag
                    v-else-if="formState.application_id"
                    class="application-approval-flow-tag application-approval-flow-tag-empty"
                  >
                    无审批流
                  </a-tag>
                </span>
              </template>
              <a-select
                v-model:value="formState.application_id"
                show-search
                :allow-clear="!isEditMode"
                :disabled="isEditMode"
                option-filter-prop="label"
                :placeholder="isEditMode ? '编辑模式下应用已锁定' : '请选择应用'"
                :loading="loadingApplications"
                :options="applicationOptions"
                style="width: 100%"
                @change="handleApplicationChange"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="12">
            <a-form-item class="form-item-compact" name="template_id">
              <template #label>
                <span class="field-label-with-hint">发布模板 <span class="field-required-hint">必填</span></span>
              </template>
              <a-select
                v-model:value="formState.template_id"
                show-search
                allow-clear
                option-filter-prop="label"
                placeholder="请选择发布模板"
                :loading="loadingTemplates || loadingTemplateDetail"
                :options="templateOptions"
                style="width: 100%"
                @change="handleTemplateChange"
              />
            </a-form-item>
          </a-col>
        </a-row>

        <a-alert
          v-if="syncingTemplateParams"
          class="template-sync-alert"
          type="info"
          show-icon
          message="正在从 Jenkins 同步当前模板绑定的管线参数…"
        />

        <a-row :gutter="12" class="form-row-compact">
          <a-col :span="24">
            <a-form-item class="form-item-compact" name="release_name">
              <template #label>
                <span class="field-label-with-hint">发布名称</span>
              </template>
              <a-input
                v-model:value="formState.release_name"
                :maxlength="128"
                :placeholder="autoGeneratedReleaseName"
                allow-clear
              />
            </a-form-item>
          </a-col>
        </a-row>

        <a-row :gutter="12" class="form-row-compact">
          <a-col v-if="showReleaseBranchField" :xs="24" :md="12">
            <a-form-item class="form-item-compact">
              <template #label>
                <span class="field-label-with-hint">发布分支</span>
              </template>
              <a-select
                v-if="releaseBranchOptions.length"
                v-model:value="formState.git_ref"
                :options="releaseBranchOptions"
                show-search
                allow-clear
                option-filter-prop="label"
                placeholder="请选择发布分支"
                style="width: 100%"
              />
              <a-input
                v-else
                v-model:value="formState.git_ref"
                placeholder="请输入发布分支（可选）"
                allow-clear
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="showReleaseBranchField ? 12 : 24">
            <a-form-item class="form-item-compact" name="remark">
              <template #label>
                <span class="field-label-with-hint">备注</span>
              </template>
              <a-input v-model:value="formState.remark" placeholder="本次发布说明" allow-clear />
            </a-form-item>
          </a-col>
        </a-row>

        <a-alert
          v-if="templateWarning && !selectedTemplate"
          class="template-alert template-alert-warning"
          type="warning"
          show-icon
          :message="templateWarning"
        />

        <a-alert
          v-if="releaseEnvNotice"
          class="template-alert template-alert-warning"
          type="warning"
          show-icon
          :message="releaseEnvNotice"
        />
            </a-skeleton>
          </section>

      <section v-if="isParamLoading || scopeCardList.length > 0" class="form-section form-section-divided release-param-section">
        <div class="form-section-heading release-param-heading">
          <div class="release-param-heading-main">
            <span class="form-section-bar"></span>
            <h3 class="form-section-heading-title">高级参数</h3>
            <a-popover
              trigger="click"
              placement="topLeft"
              overlay-class-name="advanced-param-hint-popover"
            >
              <template #content>
                <div class="advanced-param-hint-card">
                  <div class="advanced-param-hint-title">参数说明</div>
                  <div class="advanced-param-hint-row">
                    <span class="advanced-param-hint-label">当前模板</span>
                    <span class="advanced-param-hint-value">{{ selectedTemplate?.name || '-' }}</span>
                  </div>
                  <div class="advanced-param-hint-row">
                    <span class="advanced-param-hint-label">执行流程</span>
                    <span class="advanced-param-hint-value">{{ enabledScopeSummary }}</span>
                  </div>
                  <div v-if="advancedParamApprovalHint" class="advanced-param-hint-row advanced-param-hint-row-warning">
                    <span class="advanced-param-hint-label">审批限制</span>
                    <span class="advanced-param-hint-value">{{ advancedParamApprovalHint }}</span>
                  </div>
                  <div class="advanced-param-hint-row">
                    <span class="advanced-param-hint-label">填写范围</span>
                    <span class="advanced-param-hint-value">{{ advancedParamFillHint }}</span>
                  </div>
                </div>
              </template>
              <button
                class="advanced-param-heading-hint"
                type="button"
                :aria-label="advancedParamAriaLabel"
              >
                <ExclamationCircleOutlined />
              </button>
            </a-popover>
          </div>
        </div>

        <div class="scope-card-body advanced-param-body">
          <a-skeleton v-if="isParamLoading && scopeCardList.length === 0" active :paragraph="{ rows: 6 }" />

          <div
            v-for="item in scopeCardList"
            :key="`${formState.template_id}-${item.scope}-${item.binding?.binding_id || item.binding?.provider || 'none'}`"
            class="advanced-param-scope-group"
          >
            <a-alert v-if="item.error" class="scope-alert scope-alert-error" type="error" show-icon :message="item.error" />

            <a-skeleton :loading="item.loading && item.params.length === 0" active :paragraph="{ rows: 4 }">
              <a-empty
                v-if="!item.loading && item.params.length === 0"
                :description="item.binding?.provider === 'jenkins'
                  ? `当前 ${item.title} 没有需要申请人补充的高级参数`
                  : item.binding?.provider === 'argocd'
                    ? '当前执行单元会沿用 CI 中映射并勾选的发布基础字段自动完成 GitOps 配置更新；其中 image_version 在 Jenkins CI 下默认取 BUILD_NUMBER'
                    : '当前执行单元暂无可填写的高级参数'"
              />
              <div v-else class="scope-param-form">
                <a-row v-for="rowIndex in Math.ceil(item.params.length / 2)" :key="`${item.scope}-row-${rowIndex}`" :gutter="12" class="form-row-compact">
                  <a-col
                    v-for="param in item.params.slice((rowIndex - 1) * 2, (rowIndex - 1) * 2 + 2)"
                    :key="param.id"
                    :xs="24"
                    :md="12"
                  >
                    <a-form-item
                      class="form-item-compact"
                      :required="true"
                      :validate-status="resolveParamChoiceValidationError(item.scope, param) ? 'error' : ''"
                      :help="resolveParamChoiceValidationError(item.scope, param) || undefined"
                    >
                      <template #label>
                        <span class="field-label-with-hint">
                          {{ resolveTemplateParamLabel(item.scope, param) }}
                          <span class="field-required-hint">必填</span>
                        </span>
                      </template>
                      <a-select
                        v-if="useChoiceSelect(item.scope, param) && isMultipleChoice(item.scope, param)"
                        mode="multiple"
                        class="param-value-control"
                        :value="getChoiceMultiValues(param)"
                        :options="getParamSelectOptions(param)"
                        :show-arrow="true"
                        show-search
                        option-filter-prop="label"
                        placeholder="请输入关键词筛选后勾选"
                        allow-clear
                        :max-tag-count="'responsive'"
                        :dropdown-style="{ minWidth: '280px' }"
                        @change="handleChoiceMultiChange(param, $event)"
                      >
                        <template #dropdownRender="{ menuNode: menu }">
                          <div class="multi-choice-dropdown">
                            <div class="multi-choice-dropdown-actions">
                              <a-button size="small" type="link" @click.stop="handleChoiceSelectAll(param)">全选</a-button>
                              <a-button size="small" type="link" @click.stop="handleChoiceDeselectAll(param)">取消全选</a-button>
                              <a-button
                                size="small"
                                type="link"
                                class="multi-choice-expand-btn"
                                @click.stop="openExpandedChoiceModal(item.scope, param)"
                              >
                                <template #icon>
                                  <FullscreenOutlined />
                                </template>
                                展开
                              </a-button>
                            </div>
                            <a-divider style="margin: 4px 0" />
                            <component :is="menu" />
                          </div>
                        </template>
                      </a-select>
                      <a-select
                        v-else-if="useChoiceSelect(item.scope, param)"
                        mode="combobox"
                        class="param-value-control"
                        :value="getChoiceSingleValue(item.scope, param)"
                        :options="getParamSelectOptions(param)"
                        :show-arrow="true"
                        show-search
                        option-filter-prop="label"
                        placeholder="请输入关键词筛选或手动输入"
                        allow-clear
                        @change="handleChoiceSingleChange(param, $event)"
                      />
                      <a-input
                        v-else
                        v-model:value="paramValues[param.id]"
                        class="param-value-control"
                        :placeholder="isMultipleChoice(item.scope, param) ? '必填，请输入发布值，多个值可用逗号分隔' : '必填，请输入发布值'"
                        allow-clear
                      />
                    </a-form-item>
                  </a-col>
                </a-row>
              </div>
            </a-skeleton>
          </div>
        </div>
      </section>

        </div>

        <aside class="create-sidebar">
          <section v-if="isBatchMode" class="create-side-card batch-draft-card">
            <div class="batch-draft-header">
              <div>
                <span class="create-side-card-kicker">创建清单</span>
                <h3 class="create-side-card-title">待创建 {{ batchDrafts.length }} / 50</h3>
              </div>
              <a-button
                size="small"
                type="primary"
                :disabled="batchDrafts.length === 0"
                :loading="batchSubmitting"
                @click="handleBatchCreate"
              >
                创建全部
              </a-button>
            </div>
            <a-empty v-if="batchDrafts.length === 0" description="填写左侧表单后加入清单" />
            <div v-else class="batch-draft-list">
              <article
                v-for="(draft, index) in batchDrafts"
                :key="draft.id"
                class="batch-draft-item"
                :class="{ 'batch-draft-item-error': draft.error }"
              >
                <div class="batch-draft-item-head">
                  <span class="batch-draft-index">{{ index + 1 }}</span>
                  <strong :title="draft.displayName">{{ draft.displayName }}</strong>
                </div>
                <div class="batch-draft-meta">{{ draft.applicationName }} · {{ draft.envName }}</div>
                <div class="batch-draft-meta" :title="draft.templateName">{{ draft.templateName }}</div>
                <div v-if="draft.error" class="batch-draft-error">{{ draft.error }}</div>
                <div class="batch-draft-actions">
                  <a-button size="small" type="link" @click="loadBatchDraftForEdit(draft)">载入修改</a-button>
                  <a-button size="small" type="link" danger @click="removeBatchDraft(draft.id)">
                    <template #icon><DeleteOutlined /></template>
                    移除
                  </a-button>
                </div>
              </article>
            </div>
          </section>

          <section class="create-side-card create-side-process">
            <div class="create-side-card-header">
              <span class="create-side-card-kicker">发布创建流程</span>
            </div>
            <a-skeleton :loading="isPageLoading" active :title="false" :paragraph="{ rows: 6 }">
              <ol class="create-process-list">
              <li class="create-process-item">
                <span class="create-process-index">
                  <ProfileOutlined />
                </span>
                <div class="create-process-copy">
                  <strong>选择应用与模板</strong>
                  <span>先确定发布归属和执行链路</span>
                </div>
              </li>
                <li class="create-process-item">
                  <span class="create-process-index">
                    <BranchesOutlined />
                  </span>
                  <div class="create-process-copy">
                    <strong>确认发布信息</strong>
                    <span>基础字段按模板映射展示</span>
                  </div>
                </li>
              <li class="create-process-item">
                <span class="create-process-index">
                  <CheckCircleOutlined />
                </span>
                <div class="create-process-copy">
                  <strong>填写执行参数</strong>
                  <span>只填写模板开放的发布输入项</span>
                </div>
              </li>
              <li class="create-process-item">
                <span class="create-process-index">
                  <RocketOutlined />
                </span>
                <div class="create-process-copy">
                  <strong>{{ isBatchMode ? '加入清单并统一创建' : '创建发布单' }}</strong>
                  <span>{{ isBatchMode ? '只生成独立发布单，不会自动发起审批或执行' : '提交后进入详情页执行或审批' }}</span>
                </div>
              </li>
            </ol>
            </a-skeleton>
          </section>

          <section v-if="showEnvironmentField" class="create-side-env">
            <div class="create-side-env-header">
              <h3 class="create-side-env-title">选择发布环境</h3>
            </div>
            <a-form-item class="form-item-compact release-env-form-item" name="env_code" style="margin-bottom: 0;">
              <a-radio-group
                v-model:value="formState.env_code"
                class="release-env-target-list"
                :disabled="loadingEnvOptions || authorizedEnvOptions.length === 0"
              >
                <a-radio
                  v-for="option in authorizedEnvOptions"
                  :key="option.value"
                  class="release-env-target-option"
                  :value="option.value"
                >
                  <span class="release-env-target-content">
                    <span class="release-env-target-main">
                      <strong>{{ option.label }}</strong>
                      <span v-if="option.description" class="release-env-target-desc">{{ option.description }}</span>
                    </span>
                    <span v-if="formState.env_code === option.value" class="release-env-target-badge">已选</span>
                  </span>
                </a-radio>
              </a-radio-group>
              <div v-if="releaseEnvNotice" class="release-env-target-notice">{{ releaseEnvNotice }}</div>
            </a-form-item>
          </section>

            <a-button
              v-if="!isEditMode && !isBatchMode"
              type="primary"
              block
              class="create-side-fast-btn"
              :loading="isPageLoading || fastSubmitting"
              :disabled="isPageLoading || !canFastSubmitRelease"
              @click="handleFastSubmit"
            >
              <template #icon v-if="!isPageLoading">
                <ThunderboltFilled />
              </template>
              {{ isPageLoading ? '加载中...' : '极速发布' }}
            </a-button>
        </aside>
      </div>
    </a-form>

    <a-modal
      v-model:open="expandedChoiceState.open"
      class="expanded-choice-modal"
      :width="880"
      :footer="null"
      :destroy-on-close="true"
      @cancel="closeExpandedChoiceModal"
    >
      <template #title>
        <div class="expanded-choice-titlebar">
          <div>
            <div class="expanded-choice-title">选择 {{ expandedChoiceTitle }}</div>
            <div class="expanded-choice-subtitle">
              已选 {{ expandedChoiceSelectedCount }} / {{ expandedChoiceTotalCount }}
            </div>
          </div>
          <a-button size="small" @click="closeExpandedChoiceModal">关闭</a-button>
        </div>
      </template>

      <div class="expanded-choice-panel">
        <div class="expanded-choice-toolbar">
          <a-input
            v-model:value="expandedChoiceState.keyword"
            class="expanded-choice-search"
            placeholder="搜索选项"
            allow-clear
          />
          <div class="expanded-choice-count">
            当前显示 {{ filteredExpandedChoiceOptions.length }} 项
          </div>
        </div>
        <div class="expanded-choice-actions">
          <a-button size="small" @click="handleExpandedChoiceSelectFiltered">选中当前结果</a-button>
          <a-button size="small" @click="handleExpandedChoiceDeselectFiltered">清除当前结果</a-button>
          <a-button size="small" @click="handleExpandedChoiceSelectAll">全选</a-button>
          <a-button size="small" danger @click="handleExpandedChoiceDeselectAll">取消全选</a-button>
        </div>
        <div class="expanded-choice-list">
          <a-empty v-if="filteredExpandedChoiceOptions.length === 0" description="没有匹配的选项" />
          <template v-else>
            <label
              v-for="option in filteredExpandedChoiceOptions"
              :key="option.value"
              class="expanded-choice-option"
            >
              <a-checkbox
                :checked="isExpandedChoiceChecked(option.value)"
                @change="handleExpandedChoiceToggle(option.value, $event)"
              />
              <span class="expanded-choice-option-main">{{ option.label }}</span>
              <span
                v-if="option.value !== option.label"
                class="expanded-choice-option-value"
              >
                {{ option.value }}
              </span>
            </label>
          </template>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.create-page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 0;
  border: none;
  background: transparent;
  box-shadow: none;
}

.page-header-main {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

:deep(.application-toolbar-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.application-toolbar-action-btn.ant-btn[disabled]),
:deep(.application-toolbar-action-btn.ant-btn.ant-btn-disabled) {
  opacity: 0.58;
  color: rgba(15, 23, 42, 0.62) !important;
}

:deep(.application-toolbar-action-btn-disabled.ant-btn) {
  opacity: 0.58;
  cursor: not-allowed;
}

:deep(.release-build-toolbar-btn.ant-btn) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(239, 246, 255, 0.62) !important;
  color: #1d4ed8 !important;
}

:deep(.release-build-toolbar-btn.ant-btn:hover),
:deep(.release-build-toolbar-btn.ant-btn:focus),
:deep(.release-build-toolbar-btn.ant-btn:focus-visible) {
  border-color: rgba(59, 130, 246, 0.42) !important;
  background: rgba(219, 234, 254, 0.76) !important;
  color: #1e40af !important;
}

:deep(.release-build-toolbar-btn-disabled.ant-btn) {
  opacity: 0.54;
  cursor: not-allowed;
}

.release-create-form {
  width: 100%;
}

.create-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 28px;
  align-items: start;
}

.create-main {
  min-width: 0;
}

.create-sidebar {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.batch-draft-card {
  max-height: min(620px, calc(100vh - 180px));
  overflow: hidden;
}

.batch-draft-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.batch-draft-header > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
}

.batch-draft-list {
  display: flex;
  max-height: 500px;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
  padding-right: 3px;
}

.batch-draft-item {
  padding: 12px;
  border: 1px solid rgba(191, 219, 254, 0.78);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.72);
}

.batch-draft-item-error {
  border-color: rgba(248, 113, 113, 0.48);
  background: rgba(254, 242, 242, 0.76);
}

.batch-draft-item-head {
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
}

.batch-draft-item-head strong {
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-draft-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.12);
  color: #2563eb;
  font-size: 11px;
  font-weight: 800;
}

.batch-draft-meta {
  margin-top: 7px;
  overflow: hidden;
  color: #64748b;
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-draft-error {
  margin-top: 8px;
  color: #dc2626;
  font-size: 12px;
  line-height: 1.55;
  word-break: break-word;
}

.batch-draft-actions {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  margin-top: 6px;
}

.application-form-plain {
  background: transparent;
  border: none;
  padding: 0;
  border-radius: 0;
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 4px 0 0;
}

.form-section-divided {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.88);
}

.form-section-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.form-section-bar {
  width: 4px;
  height: 24px;
  border-radius: 999px;
  background: linear-gradient(180deg, #2563eb, #3b82f6);
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.22);
}

.form-section-heading-title {
  margin: 0;
  color: var(--color-text-main);
  font-size: 16px;
  font-weight: 800;
  line-height: 1.2;
}

.release-param-heading {
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
}

.release-param-heading-main {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.template-sync-alert {
  margin-top: 8px;
  margin-bottom: 16px;
  border-radius: 14px;
  border-color: #93c5fd;
  background: linear-gradient(180deg, #eff6ff 0%, #f8fbff 100%);
}

.advanced-param-heading-hint {
  appearance: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: #64748b;
  font-size: 15px;
  line-height: 1;
  cursor: pointer;
}

.advanced-param-heading-hint:hover,
.advanced-param-heading-hint:focus-visible {
  background: rgba(148, 163, 184, 0.14);
  color: #2563eb;
  outline: none;
}

:global(.advanced-param-hint-popover) {
  max-width: min(320px, calc(100vw - 32px));
}

:global(.advanced-param-hint-popover .ant-popover-inner) {
  border-radius: 14px;
  border: 1px solid rgba(203, 213, 225, 0.78);
  background: rgba(255, 255, 255, 0.98);
  box-shadow:
    0 18px 42px rgba(15, 23, 42, 0.14),
    inset 0 1px 0 rgba(255, 255, 255, 0.82);
}

:global(.advanced-param-hint-popover .ant-popover-inner-content) {
  padding: 0;
}

.advanced-param-hint-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 296px;
  padding: 14px;
}

.advanced-param-hint-title {
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
  line-height: 1.4;
}

.advanced-param-hint-row {
  display: grid;
  grid-template-columns: 66px minmax(0, 1fr);
  gap: 10px;
  align-items: flex-start;
  padding-top: 10px;
  border-top: 1px solid rgba(226, 232, 240, 0.88);
}

.advanced-param-hint-label {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.7;
}

.advanced-param-hint-value {
  min-width: 0;
  color: #334155;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.7;
  word-break: break-word;
}

.advanced-param-hint-row-warning .advanced-param-hint-label,
.advanced-param-hint-row-warning .advanced-param-hint-value {
  color: #b45309;
}

.field-label-with-hint {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  line-height: 1.1;
}

.field-required-hint,
.field-readonly-hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 20px;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  line-height: 20px;
}

.field-required-hint {
  background: rgba(59, 130, 246, 0.1);
  color: #2563eb;
}

.application-field-label {
  max-width: 100%;
}

.application-approval-flow-tag {
  display: inline-block;
  max-width: 180px;
  height: 20px;
  margin: 0;
  overflow: hidden;
  border-color: rgba(96, 165, 250, 0.28);
  border-radius: 999px;
  background: rgba(239, 246, 255, 0.92);
  color: #2563eb;
  font-size: 11px;
  font-weight: 650;
  line-height: 18px;
  text-overflow: ellipsis;
  vertical-align: middle;
  white-space: nowrap;
}

.application-approval-flow-tag-loading {
  color: #3b82f6;
  opacity: 0.72;
}

.application-approval-flow-tag-empty {
  border-color: rgba(148, 163, 184, 0.24);
  background: rgba(248, 250, 252, 0.9);
  color: #64748b;
}

.application-approval-flow-tag-error {
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(255, 251, 235, 0.92);
  color: #b45309;
}

.field-readonly-hint {
  background: rgba(148, 163, 184, 0.12);
  color: #64748b;
}

.form-row-compact {
  margin-bottom: 2px;
}

.application-form-plain :deep(.ant-form-item) {
  margin-bottom: 16px;
}

.application-form-plain :deep(.ant-form-item-label) {
  padding-bottom: 6px;
}

.application-form-plain :deep(.ant-form-item-label > label) {
  color: var(--color-text-main);
  min-height: auto;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.release-env-exclusive-section {
  padding-bottom: 2px;
}

.release-env-exclusive-section .form-section-heading {
  margin-bottom: 12px;
}

.release-env-exclusive-body .release-env-form-item {
  margin-bottom: 0 !important;
}

.release-env-inline-options {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  width: 100%;
  min-height: 44px;
  gap: 24px;
  padding: 0 2px;
}

.release-env-inline-options :deep(.ant-radio-wrapper) {
  position: relative;
  margin-inline-end: 0;
  padding: 0 0 8px;
  color: #64748b;
  cursor: pointer;
  font-size: 13px;
  font-weight: 700;
  line-height: 36px;
  transition: color 0.2s ease;
}

.release-env-inline-options :deep(.ant-radio-wrapper)::after {
  position: absolute;
  right: 0;
  bottom: 6px;
  left: 0;
  height: 2px;
  border-radius: 999px;
  background: #2563eb;
  content: '';
  transform: scaleX(0);
  transform-origin: center;
  transition: transform 0.2s ease;
}

.release-env-inline-options :deep(.ant-radio-wrapper:hover),
.release-env-inline-options :deep(.ant-radio-wrapper:focus-within),
.release-env-inline-options :deep(.ant-radio-wrapper-checked) {
  color: #1d4ed8;
}

.release-env-inline-options :deep(.ant-radio-wrapper:focus-within)::after,
.release-env-inline-options :deep(.ant-radio-wrapper-checked)::after {
  transform: scaleX(1);
}

.release-env-inline-options :deep(.ant-radio) {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.release-env-inline-options :deep(.ant-radio + span) {
  padding-inline: 0;
}

.release-env-inline-options :deep(.ant-radio-wrapper-disabled) {
  color: #94a3b8;
  cursor: not-allowed;
}

.application-form-plain :deep(.ant-form-item-explain),
.application-form-plain :deep(.ant-form-item-explain-error) {
  min-height: 18px;
  line-height: 18px;
}

.application-form-plain :deep(.ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper),
.application-form-plain :deep(.ant-select-selector) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.7), rgba(248, 250, 252, 0.54)) !important;
  border-color: rgba(255, 255, 255, 0.58) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 10px 20px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(16px) saturate(140%);
  border-radius: 14px !important;
  font-size: 13px;
  color: var(--color-text-main) !important;
}

.application-form-plain :deep(.ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper),
.application-form-plain :deep(.ant-select-single:not(.ant-select-customize-input) .ant-select-selector) {
  min-height: 44px !important;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
}

.application-form-plain :deep(.ant-input) {
  padding-inline: 14px;
}

.application-form-plain :deep(.ant-select-single .ant-select-selector) {
  display: flex;
  align-items: center;
  padding-inline: 14px !important;
}

.application-form-plain :deep(.ant-select-single .ant-select-selection-search),
.application-form-plain :deep(.ant-select-single .ant-select-selection-item),
.application-form-plain :deep(.ant-select-single .ant-select-selection-placeholder) {
  line-height: 42px !important;
}

.application-form-plain :deep(.ant-select .ant-select-arrow),
.application-form-plain :deep(.ant-input::placeholder),
.application-form-plain :deep(.ant-select-selection-placeholder) {
  color: rgba(100, 116, 139, 0.72) !important;
}

.application-form-plain :deep(.ant-input-affix-wrapper .ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper .ant-input:hover),
.application-form-plain :deep(.ant-input-affix-wrapper .ant-input:focus) {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  padding-inline: 0 !important;
}

.application-form-plain :deep(.ant-input:hover),
    .application-form-plain :deep(.ant-input:focus),
    .application-form-plain :deep(.ant-input-affix-wrapper:hover),
    .application-form-plain :deep(.ant-input-affix-wrapper-focused),
    .application-form-plain :deep(.ant-select:hover .ant-select-selector),
    .application-form-plain :deep(.ant-select-focused .ant-select-selector) {
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.82), rgba(241, 245, 249, 0.66)) !important;
      border-color: rgba(147, 197, 253, 0.48) !important;
      box-shadow:
        inset 0 1px 0 rgba(255, 255, 255, 0.96),
        0 12px 24px rgba(59, 130, 246, 0.06) !important;
    }

.release-env-target-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.release-env-target-option {
  display: flex;
  align-items: center;
  width: 100%;
  margin-inline-end: 0;
  padding: 8px 10px;
  overflow: hidden;
  border: 1px solid rgba(203, 213, 225, 0.74);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.86), rgba(248, 250, 252, 0.72));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 6px 14px rgba(15, 23, 42, 0.035);
  backdrop-filter: blur(14px) saturate(140%);
  color: #334155;
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    background 0.18s ease,
    box-shadow 0.18s ease;
}

.release-env-target-option:hover,
.release-env-target-option:focus-within {
  border-color: rgba(96, 165, 250, 0.52);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(241, 245, 249, 0.76));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.94),
    0 8px 18px rgba(37, 99, 235, 0.055);
}

.release-env-target-option.ant-radio-wrapper-checked {
  border-color: #3b82f6;
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.96), rgba(219, 234, 254, 0.54));
  box-shadow:
    inset 0 0 0 1px rgba(59, 130, 246, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 8px 18px rgba(37, 99, 235, 0.07);
}

.release-env-target-option :deep(.ant-radio) {
  flex: none;
  margin-inline-end: 8px;
  opacity: 0.72;
}

.release-env-target-option :deep(.ant-radio-inner) {
  width: 13px;
  height: 13px;
  border-color: #cbd5e1;
  background: rgba(255, 255, 255, 0.72);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.72);
}

.release-env-target-option :deep(.ant-radio-checked .ant-radio-inner) {
  border-color: #2563eb;
  background: #2563eb;
}

.release-env-target-option :deep(.ant-radio-checked .ant-radio-inner::after) {
  background: #fff;
}

.release-env-target-option :deep(.ant-radio + span) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding-inline: 0;
}

.release-env-target-content,
.release-env-target-main {
  display: flex;
  min-width: 0;
}

.release-env-target-content {
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
}

.release-env-target-main {
  flex-direction: column;
  gap: 2px;
}

.release-env-target-main strong {
  min-width: 0;
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.release-env-target-desc {
  color: #64748b;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.35;
}

.release-env-target-notice {
  margin-top: 10px;
  color: #b45309;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.45;
}

.release-env-target-badge {
  display: none;
}

.release-env-target-option.ant-radio-wrapper-disabled {
  cursor: not-allowed;
  filter: saturate(0.72);
  opacity: 0.56;
}

.scope-card-body {
  margin-top: 2px;
}

.advanced-param-body {
  display: flex;
  flex-direction: column;
}

.advanced-param-scope-group {
  padding: 2px 0 18px;
}

.advanced-param-scope-group + .advanced-param-scope-group {
  padding-top: 18px;
  border-top: 1px dashed rgba(148, 163, 184, 0.46);
}

.template-alert,
.scope-alert {
  margin-top: 8px;
  margin-bottom: 16px;
  border-radius: 16px;
  border-width: 1px;
  border-style: solid;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.84),
    0 10px 24px rgba(15, 23, 42, 0.04);
}

.template-alert :deep(.ant-alert-message),
.scope-alert :deep(.ant-alert-message) {
  font-weight: 700;
  font-size: 14px;
  line-height: 1.5;
}

.template-alert :deep(.ant-alert-description),
.scope-alert :deep(.ant-alert-description) {
  color: var(--color-text-secondary);
  line-height: 1.8;
}

.template-alert-warning {
  background: linear-gradient(180deg, #fff7ed 0%, #fffbeb 100%);
  border-color: #fdba74;
}

.template-alert-warning :deep(.ant-alert-message),
.template-alert-warning :deep(.ant-alert-icon) {
  color: #b45309;
}

.scope-alert-info {
  background: linear-gradient(180deg, #eff6ff 0%, #f8fbff 100%);
  border-color: #93c5fd;
}

.scope-alert-info :deep(.ant-alert-message),
.scope-alert-info :deep(.ant-alert-icon) {
  color: #1d4ed8;
}

.scope-alert-error {
  background: linear-gradient(180deg, #fff1f2 0%, #fff5f5 100%);
  border-color: #fda4af;
}

.scope-alert-error :deep(.ant-alert-message),
.scope-alert-error :deep(.ant-alert-icon) {
  color: #b91c1c;
}

.scope-param-form {
  min-height: 40px;
}

.param-value-control {
  width: 100%;
}

.multi-choice-dropdown-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding: 4px 12px;
}

.multi-choice-expand-btn {
  margin-left: auto;
}

.expanded-choice-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-right: 28px;
}

.expanded-choice-title {
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
}

.expanded-choice-subtitle {
  margin-top: 3px;
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
}

.expanded-choice-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.expanded-choice-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
}

.expanded-choice-search {
  min-width: 0;
}

.expanded-choice-count {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.expanded-choice-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.expanded-choice-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 8px;
  max-height: min(56vh, 520px);
  overflow: auto;
  padding: 8px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 14px;
  background: #f8fafc;
}

.expanded-choice-option {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  column-gap: 8px;
  row-gap: 2px;
  align-items: start;
  min-width: 0;
  padding: 9px 10px;
  border: 1px solid rgba(203, 213, 225, 0.84);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.88);
  cursor: pointer;
}

.expanded-choice-option:hover {
  border-color: rgba(96, 165, 250, 0.48);
  background: #fff;
}

.expanded-choice-option-main,
.expanded-choice-option-value {
  min-width: 0;
  overflow-wrap: anywhere;
}

.expanded-choice-option-main {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
  line-height: 1.45;
}

.expanded-choice-option-value {
  grid-column: 2;
  color: #64748b;
  font-size: 12px;
  line-height: 1.35;
}

.application-form-plain :deep(.ant-empty) {
  padding: 12px 0;
}

.application-form-plain :deep(.ant-empty-description) {
  color: var(--color-text-soft);
}

.create-side-card {
      padding: 24px 22px;
      border: 1px solid rgba(191, 219, 254, 0.72);
      border-radius: 24px;
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(243, 247, 255, 0.86));
      box-shadow: 0 14px 28px rgba(148, 163, 184, 0.1);
    }

    .create-side-card-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
}

.create-side-card-kicker {
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.create-side-card-title {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
}

.create-side-env {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 2px 0 0;
}

.create-side-env-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.create-side-env-title {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
}

    .create-process-list {
  position: relative;
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.create-process-list::before {
  content: '';
  position: absolute;
  left: 15px;
  top: 10px;
  bottom: 10px;
  width: 2px;
  background: rgba(191, 219, 254, 0.9);
}

.create-process-item {
  position: relative;
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  gap: 12px;
  align-items: flex-start;
}

.create-process-index {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  background: linear-gradient(180deg, #3b82f6, #2563eb);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
  box-shadow: 0 10px 18px rgba(37, 99, 235, 0.18);
}

.create-process-index :deep(.anticon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
}

.create-process-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-top: 2px;
}

.create-process-copy strong {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.create-process-copy span {
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.create-tips-list {
  margin: 0;
  padding-left: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.create-tips-list li {
  position: relative;
  padding-left: 26px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.7;
}

.create-tips-list li::before {
  content: '✓';
  position: absolute;
  left: 0;
  top: 1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  background: rgba(22, 163, 74, 0.14);
  color: #16a34a;
  font-size: 12px;
  font-weight: 800;
}

.create-side-fast-btn {
  margin-top: 12px;
  height: 46px;
  border-radius: 16px;
  font-weight: 700;
  font-size: 15px;
  box-shadow: 0 10px 22px rgba(139, 92, 246, 0.28);
}

.create-side-fast-btn:hover,
.create-side-fast-btn:focus {
  box-shadow: 0 14px 28px rgba(139, 92, 246, 0.38);
}

@media (max-width: 1200px) {
  .create-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .create-page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .release-param-heading {
    flex-direction: column;
    align-items: flex-start;
  }

  .expanded-choice-toolbar {
    grid-template-columns: 1fr;
  }
}
</style>
