<script setup lang="ts">
import {
  CheckCircleOutlined,
  DeleteOutlined,
  DownOutlined,
  EditOutlined,
  FilterOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listApplications } from '../../api/application'
import {
  checkReleaseAutomation,
  createReleaseAutomation,
  deleteReleaseAutomation,
  getReleaseAutomationByID,
  listReleaseAutomations,
  updateReleaseAutomation,
} from '../../api/release-automation'
import { listAllReleaseTemplates } from '../../api/release'
import { getReleaseSettings } from '../../api/system'
import ReleaseTemplateParamFields from '../../components/release/ReleaseTemplateParamFields.vue'
import { useResizableColumns } from '../../composables/useResizableColumns'
import { useAuthStore } from '../../stores/auth'
import type { ReleaseEnvironmentConfig } from '../../types/system'
import type {
  ReleaseAutomation,
  ReleaseAutomationCheckResult,
  ReleaseAutomationDispatchMode,
  ReleaseAutomationParam,
  ReleaseAutomationPayload,
} from '../../types/release-automation'
import type { ReleasePipelineScope } from '../../types/release'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SelectOption {
  label: string
  value: string
  description?: string
}

interface AutomationFormState {
  name: string
  application_id: string
  template_id: string
  env_code: string
  git_ref: string
  dispatch_mode: ReleaseAutomationDispatchMode
  enabled: boolean
  remark: string
}

const ENABLED_FILTER_ALL = ''
const ENABLED_FILTER_ON = 'true'
const ENABLED_FILTER_OFF = 'false'

const dispatchModeOptions: Array<{ label: string; value: ReleaseAutomationDispatchMode }> = [
  { label: '仅构建', value: 'build' },
  { label: '构建并发布', value: 'build_deploy' },
  { label: '仅发布', value: 'execute' },
]

const enabledFilterOptions: Array<{ label: string; value: string }> = [
  { label: '全部', value: ENABLED_FILTER_ALL },
  { label: '已启用', value: ENABLED_FILTER_ON },
  { label: '已停用', value: ENABLED_FILTER_OFF },
]

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()

const loading = ref(false)
const submitting = ref(false)
const checkingGit = ref(false)
const modalOpen = ref(false)
const advancedSearchExpanded = ref(false)
const enabledFilterExpanded = ref(true)
const editingAutomationID = ref('')
const openingAutomationID = ref('')
const checkingAutomationID = ref('')
const deletingAutomationID = ref('')
const dataSource = ref<ReleaseAutomation[]>([])
const total = ref(0)
const applicationOptions = ref<SelectOption[]>([])
const envOptions = ref<SelectOption[]>([])
const templateOptions = ref<SelectOption[]>([])
const loadingApplications = ref(false)
const loadingEnvOptions = ref(false)
const loadingTemplates = ref(false)
const gitCheckError = ref('')
const formParamPayload = ref<ReleaseAutomationParam[]>([])
const formParamError = ref('')
/** 编辑态：落库参数交给模板参数字段按 scope + 参数键回填。 */
const editingParamSeed = ref<ReleaseAutomationParam[]>([])
const paramFieldsRef = ref<{ validate?: () => string; buildParams?: () => ReleaseAutomationParam[] } | null>(null)
const checkResultModalOpen = ref(false)
const checkResultAutomation = ref<ReleaseAutomation | null>(null)
const checkResult = ref<ReleaseAutomationCheckResult | null>(null)

const filters = reactive({
  keyword: '',
  enabled: ENABLED_FILTER_ALL,
  application_id: '',
  page: 1,
  page_size: 10,
})

const form = reactive<AutomationFormState>({
  name: '',
  application_id: '',
  template_id: '',
  env_code: '',
  git_ref: '',
  dispatch_mode: 'build_deploy',
  enabled: true,
  remark: '',
})

const initialColumns: TableColumnsType<ReleaseAutomation> = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '环境', dataIndex: 'env_code', key: 'env_code', width: 100 },
  { title: '分支', dataIndex: 'git_ref', key: 'git_ref', width: 160 },
  { title: '模板', dataIndex: 'template_name', key: 'template_name', width: 200 },
  { title: '派发方式', dataIndex: 'dispatch_mode', key: 'dispatch_mode', width: 130 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '最近提交', dataIndex: 'last_seen_sha', key: 'last_seen_sha', width: 190 },
  { title: '最近结果', dataIndex: 'last_error', key: 'last_error', width: 220 },
  { title: '操作', key: 'actions', width: 220, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, {
  minWidth: 96,
  maxWidth: 460,
  hitArea: 10,
})

const canManageAutomation = computed(() => authStore.hasPermission('release.automation.manage'))
// 「立即检查」在服务端只要求 view 权限，只读用户也可以触发一次 git 探测
const canCheckAutomation = computed(
  () =>
    canManageAutomation.value ||
    authStore.hasPermission('release.automation.view'),
)
const isEditMode = computed(() => Boolean(editingAutomationID.value))
const modalTitle = computed(() => (isEditMode.value ? '编辑自动化' : '新建自动化'))
const activeFilterCount = computed(() => {
  let count = 0
  if (filters.keyword.trim()) {
    count += 1
  }
  if (filters.enabled !== ENABLED_FILTER_ALL) {
    count += 1
  }
  if (filters.application_id) {
    count += 1
  }
  return count
})
const activeFilterTags = computed(() => {
  const tags: Array<{ key: string; label: string; value: string }> = []
  if (filters.keyword.trim()) {
    tags.push({ key: 'keyword', label: '关键词', value: filters.keyword.trim() })
  }
  if (filters.enabled !== ENABLED_FILTER_ALL) {
    tags.push({
      key: 'enabled',
      label: '状态',
      value: filters.enabled === ENABLED_FILTER_ON ? '已启用' : '已停用',
    })
  }
  if (filters.application_id) {
    tags.push({
      key: 'application_id',
      label: '应用',
      value: optionLabel(applicationOptions.value, filters.application_id),
    })
  }
  return tags
})
const checkResultModalTitle = computed(() =>
  checkResultAutomation.value ? `检查结果 · ${checkResultAutomation.value.name || '-'}` : '检查结果',
)

function optionLabel(options: SelectOption[], value: string) {
  return options.find((item) => item.value === value)?.label || value
}

function routeQueryText(key: string) {
  const value = route.query[key]
  if (Array.isArray(value)) {
    return String(value[0] || '').trim()
  }
  return String(value || '').trim()
}

function formatTime(value: string | null | undefined) {
  if (!value) {
    return '-'
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return value
  }
  return parsed.format('YYYY-MM-DD HH:mm:ss')
}

function shortSHA(sha: string | null | undefined) {
  const text = String(sha || '').trim()
  if (!text) {
    return '-'
  }
  return text.slice(0, 7)
}

function dispatchModeText(mode: ReleaseAutomationDispatchMode | '' | null | undefined) {
  return dispatchModeOptions.find((item) => item.value === mode)?.label || '-'
}

function dispatchModeColor(mode: ReleaseAutomationDispatchMode | '' | null | undefined) {
  switch (mode) {
    case 'build':
      return 'blue'
    case 'build_deploy':
      return 'geekblue'
    case 'execute':
      return 'cyan'
    default:
      return 'default'
  }
}

function resultText(record: ReleaseAutomation) {
  return String(record.last_error || '').trim() || '正常'
}

function isGitRelatedMessage(text: string) {
  return /git|分支|权限|permission|auth|credential|凭证|denied|forbidden/i.test(text)
}

function showGitCheckFailure(rawMessage: string) {
  const text = String(rawMessage || '').trim() || 'git 校验失败，请确认分支可达与 git 权限'
  gitCheckError.value = text
  message.error(text)
}

function applyRouteQuery() {
  const applicationID = routeQueryText('application_id')
  if (applicationID) {
    filters.application_id = applicationID
  }
  const keyword = routeQueryText('keyword')
  if (keyword) {
    filters.keyword = keyword
  }
  const enabled = routeQueryText('enabled')
  if (enabled === ENABLED_FILTER_ON || enabled === ENABLED_FILTER_OFF) {
    filters.enabled = enabled
  }
}

async function loadApplicationOptions() {
  loadingApplications.value = true
  try {
    const response = await listApplications({ page: 1, page_size: 200 })
    applicationOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用下拉加载失败'))
  } finally {
    loadingApplications.value = false
  }
}

function envConfigsFromSettings(
  configs: ReleaseEnvironmentConfig[],
  fallbackOptions: string[],
): SelectOption[] {
  const result: SelectOption[] = []
  const seen = new Set<string>()
  const source =
    configs.length > 0
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

async function loadEnvOptions() {
  loadingEnvOptions.value = true
  try {
    const response = await getReleaseSettings()
    envOptions.value = envConfigsFromSettings(
      response.data.env_configs || [],
      response.data.env_options || [],
    )
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '环境选项加载失败'))
  } finally {
    loadingEnvOptions.value = false
  }
}

async function loadTemplateOptions(applicationID: string) {
  templateOptions.value = []
  const targetApplicationID = String(applicationID || '').trim()
  if (!targetApplicationID) {
    return
  }
  loadingTemplates.value = true
  try {
    const templates = await listAllReleaseTemplates({
      application_id: targetApplicationID,
      status: 'active',
    })
    templateOptions.value = templates.map((item) => ({
      label: item.name,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布模板加载失败'))
  } finally {
    loadingTemplates.value = false
  }
}

async function loadAutomations(options?: { silent?: boolean }) {
  const silent = Boolean(options?.silent)
  if (!silent) {
    loading.value = true
  }
  try {
    const response = await listReleaseAutomations({
      keyword: filters.keyword.trim() || undefined,
      enabled:
        filters.enabled === ENABLED_FILTER_ALL ? undefined : filters.enabled === ENABLED_FILTER_ON,
      application_id: filters.application_id || undefined,
      page: filters.page,
      page_size: filters.page_size,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.page_size = response.page_size
  } catch (error) {
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, '自动化列表加载失败'))
    }
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

function syncRouteQuery() {
  const query: Record<string, string> = {}
  if (filters.keyword.trim()) {
    query.keyword = filters.keyword.trim()
  }
  if (filters.enabled !== ENABLED_FILTER_ALL) {
    query.enabled = filters.enabled
  }
  if (filters.application_id) {
    query.application_id = filters.application_id
  }
  if (filters.page > 1) {
    query.page = String(filters.page)
  }
  if (filters.page_size !== 10) {
    query.page_size = String(filters.page_size)
  }
  void router.replace({ path: '/release-automations', query })
}

function handleSearch() {
  filters.page = 1
  syncRouteQuery()
  void loadAutomations()
}

function handleReset() {
  filters.keyword = ''
  filters.enabled = ENABLED_FILTER_ALL
  filters.application_id = ''
  filters.page = 1
  filters.page_size = 10
  advancedSearchExpanded.value = false
  syncRouteQuery()
  void loadAutomations()
}

function handleEnabledFilterChange(value: string) {
  filters.enabled = value
  filters.page = 1
  syncRouteQuery()
  void loadAutomations()
}

function handleApplicationFilterChange(value: string | undefined) {
  filters.application_id = String(value || '')
}

function clearFilterTag(key: string) {
  if (key === 'keyword') {
    filters.keyword = ''
  } else if (key === 'enabled') {
    filters.enabled = ENABLED_FILTER_ALL
  } else if (key === 'application_id') {
    filters.application_id = ''
  }
  handleSearch()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.page_size = pageSize
  syncRouteQuery()
  void loadAutomations()
}

function toggleAdvancedSearch() {
  advancedSearchExpanded.value = !advancedSearchExpanded.value
}

function resetFormState() {
  form.name = ''
  form.application_id = filters.application_id || ''
  form.template_id = ''
  form.env_code = ''
  form.git_ref = ''
  form.dispatch_mode = 'build_deploy'
  form.enabled = true
  form.remark = ''
  formParamPayload.value = []
  formParamError.value = ''
  editingParamSeed.value = []
  templateFixedGitRef.value = ''
  gitCheckError.value = ''
  templateOptions.value = []
}

/** 派发方式决定这次要填哪些管线的参数：仅构建=CI、构建并发布=CI+CD、仅发布=CD。 */
const activeParamScopes = computed<ReleasePipelineScope[]>(() => {
  switch (form.dispatch_mode) {
    case 'build':
      return ['ci']
    case 'execute':
      return ['cd']
    default:
      return ['ci', 'cd']
  }
})

function handleParamPayloadChange(payload: ReleaseAutomationParam[]) {
  formParamPayload.value = payload
}

function handleParamErrorChange(message: string) {
  formParamError.value = message
}

/** 模板把 CI 分支写死时，自动化的监听分支默认取模板值，但仍可改成别的分支。 */
const templateFixedGitRef = ref('')

function handleTemplateFixedGitRef(value: string) {
  const previous = templateFixedGitRef.value
  templateFixedGitRef.value = value
  if (value) {
    form.git_ref = value
    return
  }
  // 换到没有固定分支的模板时，别把上一个模板的分支带过来
  if (previous && form.git_ref.trim() === previous) {
    form.git_ref = ''
  }
}

const gitRefFollowsTemplate = computed(
  () => Boolean(templateFixedGitRef.value) && form.git_ref.trim() === templateFixedGitRef.value,
)

function openCreateModal() {
  editingAutomationID.value = ''
  resetFormState()
  void ensureSelectedApplicationOptions()
  modalOpen.value = true
}

async function ensureSelectedApplicationOptions() {
  const applicationID = String(form.application_id || '').trim()
  if (applicationID) {
    await loadTemplateOptions(applicationID)
  }
}

async function openEditModal(record: ReleaseAutomation) {
  openingAutomationID.value = record.id
  try {
    // 列表接口可能裁剪参数，编辑前先取详情，避免在请求未回来时用户改到一半又被覆盖
    let detail = record
    try {
      const response = await getReleaseAutomationByID(record.id)
      detail = response.data
    } catch (error) {
      message.error(extractHTTPErrorMessage(error, '自动化详情加载失败，已按列表数据回填'))
    }
    editingAutomationID.value = record.id
    resetFormState()
    form.name = detail.name
    form.application_id = detail.application_id
    form.template_id = detail.template_id
    form.env_code = detail.env_code
    form.git_ref = detail.git_ref
    form.dispatch_mode = detail.dispatch_mode || 'build_deploy'
    form.enabled = Boolean(detail.enabled)
    editingParamSeed.value = detail.params || []
    modalOpen.value = true
    await loadTemplateOptions(detail.application_id)
  } finally {
    openingAutomationID.value = ''
  }
}

function closeFormModal() {
  modalOpen.value = false
}

function handleFormAfterClose() {
  editingAutomationID.value = ''
  resetFormState()
}

async function handleApplicationChange(value: string | undefined) {
  form.application_id = String(value || '')
  form.template_id = ''
  await loadTemplateOptions(form.application_id)
}

async function ensureGitReachableBeforeSave() {
  if (!isEditMode.value) {
    // 新增时还没有 id，无法预检；保存接口 check_git 默认为 true，由服务端校验并返回可读原因
    return true
  }
  checkingGit.value = true
  try {
    const response = await checkReleaseAutomation(editingAutomationID.value)
    const result = response.data
    if (!result.reachable) {
      showGitCheckFailure(result.message || '当前分支不可达，请检查 git 凭证与权限')
      return false
    }
    gitCheckError.value = ''
    return true
  } catch (error) {
    showGitCheckFailure(extractHTTPErrorMessage(error, 'git 校验失败，请稍后重试'))
    return false
  } finally {
    checkingGit.value = false
  }
}

async function submitForm() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  const paramError = paramFieldsRef.value?.validate?.() || formParamError.value
  if (paramError) {
    message.warning(paramError)
    return
  }
  gitCheckError.value = ''
  submitting.value = true
  try {
    const reachable = await ensureGitReachableBeforeSave()
    if (!reachable) {
      return
    }
    const payload: ReleaseAutomationPayload = {
      name: form.name.trim(),
      application_id: form.application_id,
      template_id: form.template_id,
      env_code: form.env_code,
      git_ref: form.git_ref.trim(),
      dispatch_mode: form.dispatch_mode,
      enabled: form.enabled,
      check_git: true,
    }
    const params = paramFieldsRef.value?.buildParams?.() || formParamPayload.value
    if (params.length > 0) {
      payload.params = params
    }
    const remark = form.remark.trim()
    if (remark) {
      payload.remark = remark
    }

    if (isEditMode.value) {
      await updateReleaseAutomation(editingAutomationID.value, payload)
      message.success('自动化已更新')
    } else {
      await createReleaseAutomation(payload)
      message.success('自动化已创建')
    }
    modalOpen.value = false
    await loadAutomations({ silent: true })
  } catch (error) {
    const text = extractHTTPErrorMessage(error, isEditMode.value ? '自动化更新失败' : '自动化创建失败')
    if (isGitRelatedMessage(text)) {
      // 服务端 check_git 失败时返回 400 与可读原因，这里原样显示并阻止保存
      showGitCheckFailure(text)
    } else {
      message.error(text)
    }
  } finally {
    submitting.value = false
  }
}

async function handleCheckNow(record: ReleaseAutomation) {
  checkingAutomationID.value = record.id
  try {
    const response = await checkReleaseAutomation(record.id)
    checkResultAutomation.value = record
    checkResult.value = response.data
    checkResultModalOpen.value = true
    if (!response.data.reachable) {
      message.error(response.data.message || '分支不可达')
    }
    await loadAutomations({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '自动化检查失败'))
  } finally {
    checkingAutomationID.value = ''
  }
}

async function handleDelete(record: ReleaseAutomation) {
  deletingAutomationID.value = record.id
  try {
    await deleteReleaseAutomation(record.id)
    message.success('自动化已删除')
    if (dataSource.value.length === 1 && filters.page > 1) {
      filters.page -= 1
    }
    await loadAutomations({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '自动化删除失败'))
  } finally {
    deletingAutomationID.value = ''
  }
}

onMounted(async () => {
  applyRouteQuery()
  advancedSearchExpanded.value = activeFilterCount.value > 0
  await loadEnvOptions()
  await loadApplicationOptions()
  await loadAutomations()
})
</script>

<template>
  <div class="page-wrapper automation-page-wrapper">
    <div class="page-header-card page-header automation-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">自动化</h2>
      </div>
      <div class="page-header-actions automation-header-actions">
        <a-button
          class="release-toolbar-action-btn"
          :class="{ 'release-toolbar-action-btn--primary': advancedSearchExpanded }"
          @click="toggleAdvancedSearch"
        >
          <template #icon>
            <SearchOutlined />
          </template>
          {{ advancedSearchExpanded ? '收起检索' : '高级检索' }}
        </a-button>
        <a-button class="release-toolbar-action-btn" @click="loadAutomations()">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新
        </a-button>
        <a-button
          v-if="canManageAutomation"
          class="release-toolbar-action-btn release-toolbar-action-btn--primary"
          @click="openCreateModal"
        >
          <template #icon>
            <PlusOutlined />
          </template>
          新建自动化
        </a-button>
      </div>
    </div>

    <a-card class="filter-card" :bordered="true">
      <div class="filter-entry-row">
        <div class="quick-filter-row">
          <a-button
            class="release-toolbar-action-btn release-toolbar-action-btn--primary release-quick-filter-trigger-btn"
            :class="{
              'release-quick-filter-trigger-btn--active':
                enabledFilterExpanded || filters.enabled !== ENABLED_FILTER_ALL,
            }"
            @click="enabledFilterExpanded = !enabledFilterExpanded"
          >
            <template #icon>
              <FilterOutlined />
            </template>
            状态查询
            <DownOutlined :class="{ 'trigger-icon-rotate': enabledFilterExpanded }" />
          </a-button>
          <transition-group name="filter-expand">
            <a-button
              v-for="item in enabledFilterOptions"
              v-show="enabledFilterExpanded"
              :key="item.value || 'all'"
              class="release-toolbar-action-btn release-quick-filter-chip-btn"
              :class="{ 'release-quick-filter-chip-btn--active': filters.enabled === item.value }"
              @click="handleEnabledFilterChange(item.value)"
            >
              {{ item.label }}
            </a-button>
          </transition-group>
        </div>
      </div>

      <div v-if="advancedSearchExpanded" class="filter-advanced-panel">
        <div class="filter-actions-row">
          <div class="filter-actions-hint">高级条件需点击「查询」后生效</div>
          <div class="filter-actions-buttons">
            <a-button class="release-toolbar-action-btn release-toolbar-action-btn--primary" @click="handleSearch">
              查询
            </a-button>
            <a-button class="release-toolbar-action-btn release-toolbar-action-btn--ghost" @click="handleReset">
              重置
            </a-button>
          </div>
        </div>
        <a-form layout="vertical" class="filter-grid">
          <a-form-item label="检索词" class="filter-grid-item filter-grid-item--keyword">
            <a-input
              v-model:value="filters.keyword"
              class="filter-select"
              allow-clear
              placeholder="自动化名称 / 应用 / 环境 / 模板 / 分支"
              @keydown.enter.prevent="handleSearch"
            />
          </a-form-item>
          <a-form-item label="应用" class="filter-grid-item filter-grid-item--app">
            <a-select
              v-model:value="filters.application_id"
              class="filter-select"
              :options="applicationOptions"
              :loading="loadingApplications"
              show-search
              allow-clear
              option-filter-prop="label"
              placeholder="全部应用"
              @change="handleApplicationFilterChange"
            />
          </a-form-item>
        </a-form>
      </div>

      <div v-if="activeFilterTags.length > 0" class="active-filter-bar">
        <span class="active-filter-label">当前筛选</span>
        <a-space wrap :size="[8, 8]">
          <a-tag
            v-for="item in activeFilterTags"
            :key="item.key"
            closable
            class="active-filter-tag"
            @close.prevent="clearFilterTag(item.key)"
          >
            {{ item.label }}：{{ item.value }}
          </a-tag>
        </a-space>
      </div>
    </a-card>

    <a-card class="table-card" :bordered="true">
      <a-table
        class="release-order-table automation-table"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        row-key="id"
        :scroll="{ x: 1620 }"
      >
        <template #emptyText>
          <a-empty description="暂无自动化配置，可在右上角新建自动化" />
        </template>
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="automation-name-text">{{ record.name || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'application_name'">
            <span class="automation-cell-ellipsis">{{ record.application_name || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'env_code'">
            <a-tag color="blue">{{ record.env_code || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'git_ref'">
            <span class="automation-cell-ellipsis">{{ record.git_ref || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'template_name'">
            <span class="automation-cell-ellipsis">{{ record.template_name || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'dispatch_mode'">
            <a-tag :color="dispatchModeColor(record.dispatch_mode)">
              {{ dispatchModeText(record.dispatch_mode) }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? '启用' : '停用' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'last_seen_sha'">
            <div class="automation-commit-cell">
              <span class="automation-commit-sha">{{ shortSHA(record.last_seen_sha) }}</span>
              <span class="automation-commit-meta">检查于 {{ formatTime(record.last_checked_at) }}</span>
            </div>
          </template>
          <template v-else-if="column.key === 'last_error'">
            <span
              class="automation-cell-ellipsis"
              :class="{ 'automation-error-text': Boolean(String(record.last_error || '').trim()) }"
            >{{ resultText(record) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space :size="4" wrap>
              <a-button
                v-if="canManageAutomation"
                type="link"
                size="small"
                :loading="openingAutomationID === record.id"
                @click="openEditModal(record)"
              >
                <template #icon>
                  <EditOutlined />
                </template>
                编辑
              </a-button>
              <a-button
                v-if="canCheckAutomation"
                type="link"
                size="small"
                :loading="checkingAutomationID === record.id"
                @click="handleCheckNow(record)"
              >
                <template #icon>
                  <ThunderboltOutlined />
                </template>
                立即检查
              </a-button>
              <a-popconfirm
                v-if="canManageAutomation"
                title="确认删除该自动化配置吗？删除后不可恢复"
                ok-text="确认删除"
                cancel-text="取消"
                @confirm="handleDelete(record)"
              >
                <a-button
                  danger
                  type="link"
                  size="small"
                  :loading="deletingAutomationID === record.id"
                >
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                  删除
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <div class="pagination-area">
      <a-pagination
        :current="filters.page"
        :page-size="filters.page_size"
        :total="total"
        :page-size-options="['10', '20', '50']"
        show-size-changer
        show-quick-jumper
        :show-total="(count: number) => `共 ${count} 条自动化`"
        @change="handlePageChange"
        @showSizeChange="handlePageChange"
      />
    </div>

    <a-modal
      :open="modalOpen"
      :width="820"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleFormAfterClose"
      wrap-class-name="automation-form-modal-wrap"
      @cancel="closeFormModal"
    >
      <template #title>
        <div class="automation-form-modal-titlebar">
          <span class="automation-form-modal-title">{{ modalTitle }}</span>
          <a-button
            class="application-toolbar-action-btn automation-form-modal-save-btn"
            :loading="submitting || checkingGit"
            @click="submitForm"
          >
            保存
          </a-button>
        </div>
      </template>

      <a-form
        ref="formRef"
        class="automation-form"
        layout="vertical"
        :model="form"
        :required-mark="false"
        autocomplete="off"
      >
        <div
          v-if="gitCheckError"
          class="automation-form-alert"
          role="alert"
        >
          <div class="automation-form-alert-title">git 校验未通过，已阻止保存</div>
          <div class="automation-form-alert-message">{{ gitCheckError }}</div>
          <div class="automation-form-alert-hint">
            请确认该应用的分支可达，并检查对应 git 凭证的用户名、访问令牌与仓库读权限。
          </div>
        </div>

        <div class="automation-form-panel">
          <div class="automation-form-panel-title">基础配置</div>

          <a-form-item name="name" :rules="[{ required: true, message: '请输入自动化名称' }]">
            <template #label>
              <span class="automation-form-label">
                名称
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.name" placeholder="例如：主干有新提交自动发布到测试环境" allow-clear />
          </a-form-item>

          <a-form-item
            name="application_id"
            :rules="[{ required: true, message: '请选择应用' }]"
          >
            <template #label>
              <span class="automation-form-label">
                应用
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="form.application_id"
              :options="applicationOptions"
              :loading="loadingApplications"
              show-search
              allow-clear
              option-filter-prop="label"
              placeholder="选择应用"
              @change="handleApplicationChange"
            />
          </a-form-item>

          <a-form-item
            name="template_id"
            :rules="[{ required: true, message: '请选择发布模板' }]"
          >
            <template #label>
              <span class="automation-form-label">
                发布模板
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="form.template_id"
              :options="templateOptions"
              :loading="loadingTemplates"
              :disabled="!form.application_id"
              show-search
              allow-clear
              option-filter-prop="label"
              :placeholder="form.application_id ? '选择该应用下的发布模板' : '请先选择应用'"
              not-found-content="该应用暂无启用中的发布模板"
            />
          </a-form-item>

          <a-form-item name="env_code" :rules="[{ required: true, message: '请选择环境' }]">
            <template #label>
              <span class="automation-form-label">
                环境
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="form.env_code"
              :options="envOptions"
              :loading="loadingEnvOptions"
              allow-clear
              placeholder="选择环境"
            />
          </a-form-item>

          <a-form-item name="git_ref" :rules="[{ required: true, message: '请输入分支' }]">
            <template #label>
              <span class="automation-form-label">
                分支
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.git_ref" placeholder="例如：main / release/1.0" allow-clear />
            <div v-if="gitRefFollowsTemplate" class="automation-form-field-hint">
              已按发布模板的固定分支 {{ templateFixedGitRef }} 填入，也可以改成其他分支
            </div>
            <div v-else-if="templateFixedGitRef" class="automation-form-field-hint automation-form-field-hint--warn">
              这里监听的提交来自 {{ form.git_ref || '未填写' }}，而模板固定构建 {{ templateFixedGitRef }} 分支
            </div>
          </a-form-item>

          <a-form-item name="dispatch_mode" :rules="[{ required: true, message: '请选择派发方式' }]">
            <template #label>
              <span class="automation-form-label">
                派发方式
                <a-tag class="automation-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="form.dispatch_mode" :options="dispatchModeOptions" />
          </a-form-item>

          <a-form-item name="enabled">
            <template #label>
              <span class="automation-form-label">启用状态</span>
            </template>
            <a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="停用" />
          </a-form-item>

          <a-form-item name="remark">
            <template #label>
              <span class="automation-form-label">备注</span>
            </template>
            <a-textarea v-model:value="form.remark" placeholder="填写自动化说明" :rows="3" allow-clear />
          </a-form-item>
        </div>

        <div class="automation-form-panel">
          <div class="automation-form-panel-head">
            <div>
              <div class="automation-form-panel-title">发布参数</div>
              <div class="automation-form-panel-hint">
                按所选发布模板的参数映射列出，只需要填「发布时填写」的参数
              </div>
            </div>
          </div>

          <ReleaseTemplateParamFields
            ref="paramFieldsRef"
            :application-id="form.application_id"
            :template-id="form.template_id"
            :scopes="activeParamScopes"
            :initial-params="editingParamSeed"
            :disabled="submitting"
            @update:params="handleParamPayloadChange"
            @update:error="handleParamErrorChange"
            @update:fixed-git-ref="handleTemplateFixedGitRef"
          />
        </div>

      </a-form>
    </a-modal>

    <a-modal
      :open="checkResultModalOpen"
      :width="560"
      :closable="false"
      :footer="null"
      wrap-class-name="automation-check-modal-wrap"
      @cancel="checkResultModalOpen = false"
    >
      <template #title>
        <div class="automation-form-modal-titlebar">
          <span class="automation-form-modal-title">{{ checkResultModalTitle }}</span>
          <a-button class="application-toolbar-action-btn" @click="checkResultModalOpen = false">
            关闭
          </a-button>
        </div>
      </template>

      <div v-if="checkResult" class="automation-check-body">
        <div class="automation-check-row">
          <span class="automation-check-label">分支可达</span>
          <span class="automation-check-value">
            <a-tag :color="checkResult.reachable ? 'green' : 'red'">
              <CheckCircleOutlined v-if="checkResult.reachable" />
              {{ checkResult.reachable ? '可达' : '不可达' }}
            </a-tag>
          </span>
        </div>
        <div class="automation-check-row">
          <span class="automation-check-label">HEAD</span>
          <span class="automation-check-value automation-check-sha">{{ checkResult.head_sha || '-' }}</span>
        </div>
        <div class="automation-check-row automation-check-row-block">
          <span class="automation-check-label">说明</span>
          <span
            class="automation-check-value"
            :class="{ 'automation-error-text': !checkResult.reachable }"
          >{{ checkResult.message || '-' }}</span>
        </div>
        <div v-if="!checkResult.reachable" class="automation-form-alert">
          <div class="automation-form-alert-hint">
            请检查对应 git 凭证的用户名、访问令牌与仓库读权限后重试。
          </div>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.automation-page-wrapper {
  gap: 18px;
}

.page-header-card {
  background: transparent;
  border: none;
  box-shadow: none;
  padding: 0;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.automation-header-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.release-toolbar-action-btn,
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

.release-toolbar-action-btn:hover,
.release-toolbar-action-btn:focus,
.release-toolbar-action-btn:focus-visible,
.release-toolbar-action-btn:active,
:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.release-toolbar-action-btn--primary {
  background: linear-gradient(180deg, rgba(241, 247, 255, 0.9), rgba(223, 235, 255, 0.8)) !important;
  border-color: rgba(147, 197, 253, 0.74) !important;
  color: #1d4ed8 !important;
}

.release-toolbar-action-btn--primary:hover,
.release-toolbar-action-btn--primary:focus,
.release-toolbar-action-btn--primary:focus-visible,
.release-toolbar-action-btn--primary:active {
  background: linear-gradient(180deg, rgba(248, 251, 255, 0.96), rgba(231, 241, 255, 0.88)) !important;
  border-color: rgba(96, 165, 250, 0.66) !important;
  color: #1e3a8a !important;
  transform: translateY(-1px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 12px 26px rgba(59, 130, 246, 0.12) !important;
}

.release-toolbar-action-btn--ghost {
  background: transparent !important;
  border-color: rgba(30, 41, 59, 0.16) !important;
  color: var(--color-text-secondary) !important;
  box-shadow: none !important;
}

.release-toolbar-action-btn--ghost:hover,
.release-toolbar-action-btn--ghost:focus,
.release-toolbar-action-btn--ghost:focus-visible,
.release-toolbar-action-btn--ghost:active {
  background: rgba(241, 245, 249, 0.8) !important;
  border-color: rgba(30, 41, 59, 0.24) !important;
  color: var(--color-text-main) !important;
}

.release-quick-filter-trigger-btn {
  min-width: 126px;
  padding-inline: 16px;
}

.release-quick-filter-chip-btn {
  min-width: 92px;
  padding-inline: 14px;
  border: 1px solid rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #0f172a !important;
  font-size: 14px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
}

.release-quick-filter-chip-btn:hover,
.release-quick-filter-chip-btn:focus,
.release-quick-filter-chip-btn:focus-visible,
.release-quick-filter-chip-btn:active {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
}

.release-quick-filter-trigger-btn--active {
  transform: translateY(-1px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 12px 26px rgba(59, 130, 246, 0.12) !important;
}

.release-quick-filter-chip-btn--active {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 14px 28px rgba(59, 130, 246, 0.08) !important;
}

.filter-card,
.table-card {
  background: transparent;
  border: none;
  box-shadow: none;
}

.filter-card :deep(.ant-card-body),
.table-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.filter-entry-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.quick-filter-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.trigger-icon-rotate {
  transform: rotate(180deg);
  transition: transform 0.2s ease;
}

.filter-expand-enter-active {
  transition: opacity 0.18s ease;
}

.filter-expand-leave-active {
  transition: opacity 0.12s ease;
}

.filter-expand-enter-from,
.filter-expand-leave-to {
  opacity: 0;
}

.filter-advanced-panel {
  margin-top: 16px;
  padding: 18px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: var(--radius-xl);
  background: rgba(255, 255, 255, 0.5);
}

.filter-actions-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.filter-actions-hint {
  color: var(--color-text-soft);
  font-size: 12px;
}

.filter-actions-buttons {
  display: flex;
  align-items: center;
  gap: 10px;
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0 16px;
}

.filter-grid-item {
  margin-bottom: 8px;
}

.filter-select {
  width: 100%;
}

.active-filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 14px;
}

.active-filter-label {
  color: var(--color-text-secondary);
  font-size: 12px;
}

.active-filter-tag {
  border-radius: 999px;
  padding-inline: 10px;
}

.release-order-table {
  background: transparent;
}

:deep(.automation-table .ant-table-container),
:deep(.automation-table .ant-table-content),
:deep(.automation-table .ant-table-body),
:deep(.automation-table .ant-table-thead > tr > th),
:deep(.automation-table .ant-table-tbody > tr > td) {
  border-radius: 0 !important;
}

:deep(.automation-table .ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(255, 255, 255, 0.36);
}

:deep(.automation-table .ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(59, 130, 246, 0.24);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #eff6ff !important;
  font-size: 12px;
  font-weight: 700;
}

:deep(.automation-table .ant-table-thead > tr > th::before) {
  display: none;
}

:deep(.automation-table .ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: #334155;
}

:deep(.automation-table .ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

:deep(.automation-table .ant-table-cell-fix-right) {
  background: #ffffff !important;
}

.automation-name-text {
  color: #0f172a;
  font-weight: 700;
}

.automation-cell-ellipsis {
  display: inline-block;
  max-width: 190px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.automation-error-text {
  color: #dc2626;
}

.automation-commit-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.automation-commit-sha {
  color: #1d4ed8;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-weight: 700;
}

.automation-commit-meta {
  color: var(--color-text-soft);
  font-size: 12px;
}

.pagination-area {
  display: flex;
  justify-content: flex-end;
}

:global(.automation-form-modal-wrap .ant-modal-content),
:global(.automation-check-modal-wrap .ant-modal-content) {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 24px;
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.08), transparent 34%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.12), transparent 36%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.95));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 32px 90px rgba(15, 23, 42, 0.18);
  backdrop-filter: blur(18px) saturate(180%);
}

:global(.automation-form-modal-wrap .ant-modal-header),
:global(.automation-check-modal-wrap .ant-modal-header) {
  margin-bottom: 0;
  border-bottom: none;
  background: transparent;
}

.automation-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.automation-form-modal-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
}

.automation-form-modal-save-btn {
  flex: none;
}

.automation-form-panel {
  margin-bottom: 18px;
  padding: 18px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: var(--radius-xl);
  background: rgba(255, 255, 255, 0.56);
}

.automation-form-panel:last-child {
  margin-bottom: 0;
}

.automation-form-panel-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.automation-form-field-hint {
  margin-top: 6px;
  color: var(--color-text-soft);
  font-size: 12px;
}

.automation-form-field-hint--warn {
  color: #d97706;
}

.automation-form-panel-title {
  color: #0f172a;
  font-size: 15px;
  font-weight: 800;
}

.automation-form-panel-hint {
  margin-top: 4px;
  color: var(--color-text-soft);
  font-size: 12px;
}

.automation-form-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-secondary);
  font-weight: 700;
}

.automation-form-required-tag {
  margin-inline-end: 0;
  border: none;
  background: rgba(59, 130, 246, 0.12);
  color: #1d4ed8;
  font-size: 11px;
}

.automation-form-alert {
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid rgba(248, 113, 113, 0.42);
  border-radius: var(--radius-lg);
  background: rgba(254, 242, 242, 0.86);
}

.automation-form-alert-title {
  color: #b91c1c;
  font-size: 13px;
  font-weight: 800;
}

.automation-form-alert-message {
  margin-top: 6px;
  color: #7f1d1d;
  font-size: 13px;
  word-break: break-word;
}

.automation-form-alert-hint {
  margin-top: 6px;
  color: #92400e;
  font-size: 12px;
  line-height: 1.7;
}

.automation-param-empty {
  padding: 14px 0;
  color: var(--color-text-soft);
  font-size: 13px;
}

.automation-param-row {
  display: grid;
  grid-template-columns: 110px minmax(0, 1.2fr) minmax(0, 1.2fr) minmax(0, 1.2fr) auto;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.automation-param-scope,
.automation-param-input {
  width: 100%;
}

.automation-param-remove-btn {
  flex: none;
  padding-inline: 12px;
}

.automation-check-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.automation-check-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.automation-check-row-block {
  flex-direction: column;
  gap: 6px;
}

.automation-check-label {
  flex: 0 0 76px;
  color: var(--color-text-soft);
  font-size: 13px;
}

.automation-check-value {
  flex: 1 1 auto;
  color: #0f172a;
  font-size: 13px;
  word-break: break-word;
}

.automation-check-sha {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  word-break: break-all;
}

@media (max-width: 1024px) {
  .automation-param-row {
    grid-template-columns: 1fr;
  }

  .automation-check-row {
    flex-direction: column;
    gap: 4px;
  }
}
</style>
