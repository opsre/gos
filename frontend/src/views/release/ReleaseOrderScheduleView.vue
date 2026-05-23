<script setup lang="ts">
import {
  CheckCircleFilled,
  CheckCircleOutlined,
  ClockCircleFilled,
  CloseCircleFilled,
  CloseCircleOutlined,
  DownOutlined,
  EditOutlined,
  EnvironmentOutlined,
  EyeOutlined,
  FilterOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  SearchOutlined,
  StopOutlined,
} from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listApplications } from '../../api/application'
import {
  approveReleaseOrderSchedule,
  cancelReleaseOrderSchedule,
  createReleaseOrderSchedule,
  getReleaseOrderByID,
  listReleaseOrderExecutions,
  listReleaseOrderSchedules,
  listSchedulableReleaseOrdersForSchedule,
  rejectReleaseOrderSchedule,
  submitReleaseOrderScheduleApproval,
  updateReleaseOrderSchedule,
} from '../../api/release'
import { getReleaseSettings } from '../../api/system'
import { useResizableColumns } from '../../composables/useResizableColumns'
import { useAuthStore } from '../../stores/auth'
import type {
  ReleaseOrder,
  ReleaseOrderExecution,
  ReleaseOrderSchedule,
  ReleaseOrderScheduleMode,
  ReleaseOrderSchedulePayload,
  ReleaseOrderScheduleStatus,
} from '../../types/release'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SelectOption {
  label: string
  value: string
}

interface ReleaseOrderOption extends SelectOption {
  record: ReleaseOrder
}

interface ScheduleFormState {
  release_order_id: string
  schedule_mode: ReleaseOrderScheduleMode
  build_scheduled_at: string
  deploy_scheduled_at: string
  execute_scheduled_at: string
  timezone: string
  remark: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()

const loading = ref(false)
const submitting = ref(false)
const modalOpen = ref(false)
const editingScheduleID = ref('')
const selectedSchedule = ref<ReleaseOrderSchedule | null>(null)
const handlingActionKey = ref('')
const dataSource = ref<ReleaseOrderSchedule[]>([])
const total = ref(0)
const lastLoadedAt = ref('')
const applicationOptions = ref<SelectOption[]>([])
const envOptions = ref<SelectOption[]>([])
const releaseOrderOptions = ref<ReleaseOrderOption[]>([])
const releaseOrderSearchKeyword = ref('')
const selectedReleaseOrderExecutions = ref<ReleaseOrderExecution[]>([])
const loadingApplications = ref(false)
const loadingEnvOptions = ref(false)
const loadingReleaseOrders = ref(false)
const loadingReleaseOrderExecutions = ref(false)
const scheduleFormViewportInset = ref(0)
const advancedSearchExpanded = ref(false)
const statusExpanded = ref(false)
const envExpanded = ref(false)
const detailDrawerOpen = ref(false)
const detailSchedule = ref<ReleaseOrderSchedule | null>(null)

const scheduleModeOptions: Array<{ label: string; value: ReleaseOrderScheduleMode | '' }> = [
  { label: '全部模式', value: '' },
  { label: 'CI', value: 'build' },
  { label: 'CD', value: 'deploy' },
  { label: 'CI + CD', value: 'build_deploy' },
  { label: '全流程', value: 'execute' },
]

const scheduleModeButtonOptions: Array<{ label: string; value: ReleaseOrderScheduleMode }> = [
  { label: 'CI', value: 'build' },
  { label: 'CD', value: 'deploy' },
  { label: 'CI + CD', value: 'build_deploy' },
  { label: '全流程', value: 'execute' },
]

const scheduleStatusOptions: Array<{ label: string; value: ReleaseOrderScheduleStatus | '' }> = [
  { label: '全部状态', value: '' },
  { label: '待审批', value: 'pending_approval' },
  { label: '审批中', value: 'approving' },
  { label: '待触发', value: 'scheduled' },
  { label: '触发中', value: 'dispatching' },
  { label: '已触发', value: 'dispatched' },
  { label: '已失效', value: 'expired' },
  { label: '已阻塞', value: 'blocked' },
  { label: '失败', value: 'failed' },
  { label: '已跳过', value: 'skipped' },
  { label: '已取消', value: 'cancelled' },
  { label: '已拒绝', value: 'rejected' },
]

const timezoneOptions: SelectOption[] = [
  { label: 'Asia/Shanghai', value: 'Asia/Shanghai' },
  { label: 'UTC', value: 'UTC' },
]

const filters = reactive({
  application_id: '',
  keyword: '',
  env_code: '',
  schedule_mode: '' as ReleaseOrderScheduleMode | '',
  status: '' as ReleaseOrderScheduleStatus | '',
  scheduled_at_range: [] as string[],
  page: 1,
  page_size: 10,
})

const form = reactive<ScheduleFormState>({
  release_order_id: '',
  schedule_mode: 'execute',
  build_scheduled_at: '',
  deploy_scheduled_at: '',
  execute_scheduled_at: '',
  timezone: 'Asia/Shanghai',
  remark: '',
})

const initialColumns: TableColumnsType<ReleaseOrderSchedule> = [
  { title: '预约单号', dataIndex: 'schedule_no', key: 'schedule_no', width: 190 },
  { title: '发布单号', dataIndex: 'release_order_no', key: 'release_order_no', width: 180 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 170 },
  { title: '环境', dataIndex: 'env_code', key: 'env_code', width: 100 },
  { title: '预约模式', dataIndex: 'schedule_mode', key: 'schedule_mode', width: 120 },
  { title: 'CI 时间', dataIndex: 'build_scheduled_at', key: 'build_scheduled_at', width: 180 },
  { title: 'CD 时间', dataIndex: 'deploy_scheduled_at', key: 'deploy_scheduled_at', width: 180 },
  { title: '全流程时间', dataIndex: 'execute_scheduled_at', key: 'execute_scheduled_at', width: 180 },
  { title: 'CD 风险时间', dataIndex: 'cd_conflict_at', key: 'cd_conflict_at', width: 180 },
  { title: '审批状态', dataIndex: 'approval_required', key: 'approval_required', width: 120 },
  { title: '预约状态', dataIndex: 'status', key: 'status', width: 120 },
  { title: '创建人', dataIndex: 'creator_name', key: 'creator_name', width: 130 },
  { title: '审批人', dataIndex: 'approval_approver_names', key: 'approval_approver_names', width: 180 },
  { title: '最近错误', dataIndex: 'last_error', key: 'last_error', width: 220 },
  { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
  { title: '操作', key: 'actions', width: 300, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, {
  minWidth: 96,
  maxWidth: 460,
  hitArea: 10,
})

const canCreateSchedule = computed(() => authStore.hasPermission('release.schedule.create') || authStore.hasPermission('release.create'))
const canApproveSchedules = computed(() => authStore.hasPermission('release.schedule.approve') || authStore.hasPermission('release.approve'))
const isEditMode = computed(() => Boolean(editingScheduleID.value))
const modalTitle = computed(() => (isEditMode.value ? '编辑预约' : '创建预约'))
const selectedReleaseOrder = computed(
  () => releaseOrderOptions.value.find((item) => item.value === form.release_order_id)?.record || null,
)
const selectedPendingScopes = computed(() => {
  const scopes = new Set<string>()
  selectedReleaseOrderExecutions.value.forEach((item) => {
    if (item.status === 'pending') {
      scopes.add(item.pipeline_scope)
    }
  })
  return scopes
})
const availableScheduleModes = computed<ReleaseOrderScheduleMode[]>(() => {
  if (isEditMode.value) {
    return scheduleModeButtonOptions.map((item) => item.value)
  }
  if (!selectedReleaseOrder.value || loadingReleaseOrderExecutions.value) {
    return []
  }
  const scopes = selectedPendingScopes.value
  const modes: ReleaseOrderScheduleMode[] = []
  if (scopes.has('ci')) {
    modes.push('build')
  }
  if (scopes.has('cd')) {
    modes.push('deploy')
  }
  if (scopes.has('ci') && scopes.has('cd')) {
    modes.push('build_deploy')
  }
  if (scopes.size > 0) {
    modes.push('execute')
  }
  return modes
})
const showBuildTime = computed(() => form.schedule_mode === 'build' || form.schedule_mode === 'build_deploy')
const showDeployTime = computed(() => form.schedule_mode === 'deploy' || form.schedule_mode === 'build_deploy')
const showExecuteTime = computed(() => form.schedule_mode === 'execute')
const showAdvancedSearch = computed(() => advancedSearchExpanded.value)
const currentEnvFilter = computed(() => filters.env_code)
const envShortcutOptions = computed(() => envOptions.value)
const quickStatusOptions = computed(() =>
  scheduleStatusOptions.filter((item): item is { label: string; value: ReleaseOrderScheduleStatus } => Boolean(item.value)),
)
const refreshText = computed(() => lastLoadedAt.value || '尚未加载')
const scheduleFormMaskStyle = computed(() => ({
  left: `${scheduleFormViewportInset.value}px`,
  width: `calc(100% - ${scheduleFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalOpen.value ? 'auto' : 'none',
}))
const scheduleFormWrapProps = computed(() => ({
  style: {
    left: `${scheduleFormViewportInset.value}px`,
    width: `calc(100% - ${scheduleFormViewportInset.value}px)`,
    pointerEvents: modalOpen.value ? 'auto' : 'none',
  },
}))
const activeFilterTags = computed(() => {
  const tags: Array<{ key: string; label: string; value: string }> = []
  if (filters.application_id) {
    tags.push({ key: 'application_id', label: '应用', value: optionLabel(applicationOptions.value, filters.application_id) })
  }
  if (filters.keyword.trim()) {
    tags.push({ key: 'keyword', label: '关键词', value: filters.keyword.trim() })
  }
  if (filters.env_code) {
    tags.push({ key: 'env_code', label: '环境', value: filters.env_code })
  }
  if (filters.schedule_mode) {
    tags.push({ key: 'schedule_mode', label: '预约模式', value: scheduleModeText(filters.schedule_mode) })
  }
  if (filters.status) {
    tags.push({ key: 'status', label: '预约状态', value: scheduleStatusText(filters.status) })
  }
  if (filters.scheduled_at_range.length > 0) {
    tags.push({ key: 'scheduled_at_range', label: '预约时间', value: formatRangeLabel(filters.scheduled_at_range) })
  }
  return tags
})
const overviewStatusStats = computed(() => {
  const stats = {
    total: total.value,
    pending: 0,
    scheduled: 0,
    running: 0,
    success: 0,
    risk: 0,
  }
  dataSource.value.forEach((item) => {
    if (item.status === 'pending_approval' || item.status === 'approving') {
      stats.pending += 1
    } else if (item.status === 'scheduled') {
      stats.scheduled += 1
    } else if (item.status === 'dispatching') {
      stats.running += 1
    } else if (item.status === 'dispatched') {
      stats.success += 1
    } else if (['blocked', 'failed', 'expired', 'skipped', 'cancelled', 'rejected'].includes(item.status)) {
      stats.risk += 1
    }
  })
  return stats
})
const scheduleOverviewBars = computed(() => {
  const items = [
    { key: 'pending', label: '待审批', count: overviewStatusStats.value.pending, className: 'schedule-overview-fill--pending' },
    { key: 'scheduled', label: '待触发', count: overviewStatusStats.value.scheduled, className: 'schedule-overview-fill--scheduled' },
    { key: 'running', label: '触发中', count: overviewStatusStats.value.running, className: 'schedule-overview-fill--running' },
    { key: 'success', label: '已触发', count: overviewStatusStats.value.success, className: 'schedule-overview-fill--success' },
    { key: 'risk', label: '异常/终止', count: overviewStatusStats.value.risk, className: 'schedule-overview-fill--risk' },
  ]
  const maxCount = Math.max(1, ...items.map((item) => item.count))
  return items.map((item) => ({
    ...item,
    width: item.count > 0 ? `${Math.max(12, Math.round((item.count / maxCount) * 100))}%` : '0%',
  }))
})
const spotlightSchedules = computed(() =>
  [...dataSource.value]
    .sort((left, right) => dayjs(right.created_at).valueOf() - dayjs(left.created_at).valueOf())
    .slice(0, 2)
    .map((item) => ({
      id: item.id,
      scheduleNo: item.schedule_no,
      releaseOrderID: item.release_order_id,
    })),
)
const spotlightStateKey = computed(() => {
  if (filters.status === 'dispatching' || overviewStatusStats.value.running > 0) {
    return 'running'
  }
  if (filters.status && ['blocked', 'failed', 'expired', 'skipped', 'cancelled', 'rejected'].includes(filters.status)) {
    return 'failed'
  }
  if (filters.status === 'dispatched' || overviewStatusStats.value.success > 0) {
    return 'success'
  }
  return 'pending'
})
const spotlightHeadline = computed(() => {
  if (filters.status) {
    return scheduleStatusText(filters.status)
  }
  if (filters.schedule_mode) {
    return scheduleModeText(filters.schedule_mode)
  }
  return '最新预约'
})
const spotlightHint = computed(() => {
  if (filters.status) {
    return '已按预约状态聚焦当前列表'
  }
  if (filters.schedule_mode || filters.application_id || filters.env_code || filters.keyword.trim() || filters.scheduled_at_range.length) {
    return '展示当前筛选条件下最近创建的预约单'
  }
  return '展示当前筛选条件下最近创建的预约单'
})

let scheduleFormViewportObserver: ResizeObserver | null = null

watch(
  () => form.schedule_mode,
  (mode) => {
    if (mode !== 'build' && mode !== 'build_deploy') {
      form.build_scheduled_at = ''
    }
    if (mode !== 'deploy' && mode !== 'build_deploy') {
      form.deploy_scheduled_at = ''
    }
    if (mode !== 'execute') {
      form.execute_scheduled_at = ''
    }
  },
)

function readScheduleFormViewportInset() {
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

function syncScheduleFormViewportInset() {
  scheduleFormViewportInset.value = readScheduleFormViewportInset()
}

function observeScheduleFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  scheduleFormViewportObserver?.disconnect()
  scheduleFormViewportObserver = new ResizeObserver(() => {
    syncScheduleFormViewportInset()
  })

  if (appLayout) {
    scheduleFormViewportObserver.observe(appLayout)
  }
  if (sider) {
    scheduleFormViewportObserver.observe(sider)
  }
}

function stopObservingScheduleFormViewportInset() {
  scheduleFormViewportObserver?.disconnect()
  scheduleFormViewportObserver = null
}

function applyRouteQuery() {
  const releaseOrderID = String(route.query.release_order_id || '').trim()
  if (releaseOrderID) {
    form.release_order_id = releaseOrderID
  }
  const applicationID = String(route.query.application_id || '').trim()
  if (applicationID) {
    filters.application_id = applicationID
  }
}

function optionLabel(options: SelectOption[], value: string) {
  return options.find((item) => item.value === value)?.label || value
}

function releaseOrderOptionLabel(item: ReleaseOrder) {
  const name = item.release_name || item.application_name || '-'
  return `${item.order_no} · ${name} · ${item.env_code || '-'}`
}

function formatTime(value: string | null) {
  if (!value) {
    return '-'
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return value
  }
  return parsed.format('YYYY-MM-DD HH:mm:ss')
}

function formatRangeLabel(range: string[]) {
  const start = range[0] ? dayjs(range[0]).format('YYYY-MM-DD HH:mm') : '开始'
  const end = range[1] ? dayjs(range[1]).format('YYYY-MM-DD HH:mm') : '结束'
  return `${start} ~ ${end}`
}

function scheduleModeText(mode: ReleaseOrderScheduleMode | '' | null | undefined) {
  switch (mode) {
    case 'build':
      return 'CI'
    case 'deploy':
      return 'CD'
    case 'build_deploy':
      return 'CI + CD'
    case 'execute':
      return '全流程'
    default:
      return '-'
  }
}

function scheduleStatusText(status: ReleaseOrderScheduleStatus | '' | null | undefined) {
  switch (status) {
    case 'pending_approval':
      return '待审批'
    case 'approving':
      return '审批中'
    case 'scheduled':
      return '待触发'
    case 'dispatching':
      return '触发中'
    case 'dispatched':
      return '已触发'
    case 'expired':
      return '已失效'
    case 'blocked':
      return '已阻塞'
    case 'failed':
      return '失败'
    case 'skipped':
      return '已跳过'
    case 'cancelled':
      return '已取消'
    case 'rejected':
      return '已拒绝'
    default:
      return status || '-'
  }
}

function scheduleStatusColor(status: ReleaseOrderScheduleStatus) {
  switch (status) {
    case 'scheduled':
      return 'blue'
    case 'dispatching':
      return 'processing'
    case 'dispatched':
      return 'green'
    case 'pending_approval':
    case 'approving':
      return 'gold'
    case 'expired':
    case 'cancelled':
    case 'skipped':
      return 'default'
    case 'blocked':
    case 'failed':
    case 'rejected':
      return 'red'
    default:
      return 'default'
  }
}

function approvalStatusText(record: ReleaseOrderSchedule) {
  if (!record.approval_required) {
    return '无需审批'
  }
  if (record.status === 'rejected') {
    return '已拒绝'
  }
  if (record.approved_at) {
    return '已通过'
  }
  if (record.status === 'approving') {
    return '审批中'
  }
  return '待审批'
}

function approvalStatusColor(record: ReleaseOrderSchedule) {
  if (!record.approval_required || record.approved_at) {
    return 'green'
  }
  if (record.status === 'rejected') {
    return 'red'
  }
  return 'gold'
}

function approverText(record: ReleaseOrderSchedule) {
  return (record.approval_approver_names || []).filter(Boolean).join(' / ') || '-'
}

function isActiveSchedule(record: ReleaseOrderSchedule) {
  return ['pending_approval', 'approving', 'scheduled', 'dispatching'].includes(record.status)
}

function canEditSchedule(record: ReleaseOrderSchedule) {
  return ['pending_approval', 'approving'].includes(record.status) && (canCreateSchedule.value || authStore.isAdmin)
}

function canSubmitApproval(record: ReleaseOrderSchedule) {
  return record.status === 'pending_approval' && isActiveSchedule(record)
}

function canApproveSchedule(record: ReleaseOrderSchedule) {
  return canApproveSchedules.value && ['pending_approval', 'approving'].includes(record.status)
}

function canRejectSchedule(record: ReleaseOrderSchedule) {
  return canApproveSchedules.value && ['pending_approval', 'approving'].includes(record.status)
}

function canCancelSchedule(record: ReleaseOrderSchedule) {
  return ['pending_approval', 'approving', 'scheduled'].includes(record.status) && (canCreateSchedule.value || authStore.isAdmin)
}

function actionKey(record: ReleaseOrderSchedule, action: string) {
  return `${record.id}:${action}`
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

async function loadEnvOptions() {
  loadingEnvOptions.value = true
  try {
    const response = await getReleaseSettings()
    envOptions.value = (response.data.env_options || []).map((item) => ({
      label: item,
      value: item,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '环境选项加载失败'))
  } finally {
    loadingEnvOptions.value = false
  }
}

async function loadReleaseOrderOptions(keyword = releaseOrderSearchKeyword.value) {
  loadingReleaseOrders.value = true
  releaseOrderSearchKeyword.value = keyword
  try {
    const response = await listSchedulableReleaseOrdersForSchedule({
      keyword: keyword.trim() || undefined,
      page: 1,
      page_size: 50,
    })
    const nextOptions = response.data.map((item) => ({
      label: releaseOrderOptionLabel(item),
      value: item.id,
      record: item,
    }))
    const currentOption = releaseOrderOptions.value.find((item) => item.value === form.release_order_id)
    if (currentOption && !nextOptions.some((item) => item.value === currentOption.value)) {
      nextOptions.unshift(currentOption)
    }
    releaseOrderOptions.value = nextOptions
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布单下拉加载失败'))
  } finally {
    loadingReleaseOrders.value = false
  }
}

async function ensureReleaseOrderOption(releaseOrderID: string) {
  const normalizedID = String(releaseOrderID || '').trim()
  if (!normalizedID || releaseOrderOptions.value.some((item) => item.value === normalizedID)) {
    return
  }
  try {
    const response = await getReleaseOrderByID(normalizedID)
    releaseOrderOptions.value = [
      {
        label: releaseOrderOptionLabel(response.data),
        value: response.data.id,
        record: response.data,
      },
      ...releaseOrderOptions.value,
    ]
  } catch {
    // 从详情页携带 release_order_id 进入时，详情补全失败仍保留原值，最终由保存接口校验。
  }
}

async function loadSelectedReleaseOrderExecutions(releaseOrderID: string) {
  const normalizedID = String(releaseOrderID || '').trim()
  selectedReleaseOrderExecutions.value = []
  if (!normalizedID) {
    return
  }
  loadingReleaseOrderExecutions.value = true
  try {
    const response = await listReleaseOrderExecutions(normalizedID)
    if (form.release_order_id === normalizedID) {
      selectedReleaseOrderExecutions.value = response.data || []
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布单阶段加载失败'))
  } finally {
    if (form.release_order_id === normalizedID) {
      loadingReleaseOrderExecutions.value = false
      ensureScheduleModeAvailable()
    }
  }
}

function ensureScheduleModeAvailable() {
  if (isEditMode.value) {
    return
  }
  if (!availableScheduleModes.value.includes(form.schedule_mode)) {
    form.schedule_mode = availableScheduleModes.value[0] || 'execute'
  }
}

function isScheduleModeDisabled(mode: ReleaseOrderScheduleMode) {
  return !availableScheduleModes.value.includes(mode)
}

function handleReleaseOrderChange(value: string | undefined) {
  form.release_order_id = String(value || '').trim()
  form.build_scheduled_at = ''
  form.deploy_scheduled_at = ''
  form.execute_scheduled_at = ''
  void loadSelectedReleaseOrderExecutions(form.release_order_id)
}

async function loadSchedules() {
  loading.value = true
  try {
    const response = await listReleaseOrderSchedules({
      application_id: filters.application_id || undefined,
      keyword: filters.keyword.trim() || undefined,
      env_code: filters.env_code || undefined,
      schedule_mode: filters.schedule_mode || undefined,
      status: filters.status || undefined,
      scheduled_at_from: filters.scheduled_at_range[0] || undefined,
      scheduled_at_to: filters.scheduled_at_range[1] || undefined,
      page: filters.page,
      page_size: filters.page_size,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.page_size = response.page_size
    lastLoadedAt.value = dayjs().format('HH:mm:ss')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '预约发布列表加载失败'))
  } finally {
    loading.value = false
  }
}

function toggleAdvancedSearch() {
  advancedSearchExpanded.value = !advancedSearchExpanded.value
}

function handleSearch() {
  filters.page = 1
  void loadSchedules()
}

function handleReset() {
  filters.application_id = ''
  filters.keyword = ''
  filters.env_code = ''
  filters.schedule_mode = ''
  filters.status = ''
  filters.scheduled_at_range = []
  filters.page = 1
  filters.page_size = 10
  void loadSchedules()
}

function handleQuickStatusChange(status: ReleaseOrderScheduleStatus | '') {
  filters.status = filters.status === status ? '' : status
  filters.page = 1
  void loadSchedules()
}

function handleEnvQuickFilter(envCode: string) {
  filters.env_code = filters.env_code === envCode ? '' : envCode
  filters.page = 1
  void loadSchedules()
}

function clearFilterTag(key: string) {
  if (key === 'application_id') {
    filters.application_id = ''
  } else if (key === 'keyword') {
    filters.keyword = ''
  } else if (key === 'env_code') {
    filters.env_code = ''
  } else if (key === 'schedule_mode') {
    filters.schedule_mode = ''
  } else if (key === 'status') {
    filters.status = ''
  } else if (key === 'scheduled_at_range') {
    filters.scheduled_at_range = []
  }
  handleSearch()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.page_size = pageSize
  void loadSchedules()
}

function resetForm() {
  form.release_order_id = String(route.query.release_order_id || '').trim()
  form.schedule_mode = 'execute'
  form.build_scheduled_at = ''
  form.deploy_scheduled_at = ''
  form.execute_scheduled_at = ''
  form.timezone = 'Asia/Shanghai'
  form.remark = ''
  editingScheduleID.value = ''
  selectedSchedule.value = null
  selectedReleaseOrderExecutions.value = []
  loadingReleaseOrderExecutions.value = false
}

function openCreateModal() {
  if (!canCreateSchedule.value) {
    message.warning('当前账号没有创建预约发布的权限')
    return
  }
  resetForm()
  modalOpen.value = true
  void loadReleaseOrderOptions()
  if (form.release_order_id) {
    void ensureReleaseOrderOption(form.release_order_id)
    void loadSelectedReleaseOrderExecutions(form.release_order_id)
  }
}

function openEditModal(record: ReleaseOrderSchedule) {
  if (!canEditSchedule(record)) {
    message.warning('审批通过后的预约不可编辑，请取消后重新创建')
    return
  }
  selectedSchedule.value = { ...record }
  editingScheduleID.value = record.id
  form.release_order_id = record.release_order_id
  form.schedule_mode = record.schedule_mode
  form.build_scheduled_at = record.build_scheduled_at || ''
  form.deploy_scheduled_at = record.deploy_scheduled_at || ''
  form.execute_scheduled_at = record.execute_scheduled_at || ''
  form.timezone = record.timezone || 'Asia/Shanghai'
  form.remark = record.remark || ''
  modalOpen.value = true
}

function closeFormModal() {
  modalOpen.value = false
}

function handleFormAfterClose() {
  submitting.value = false
  resetForm()
  formRef.value?.clearValidate()
}

function normalizeScheduleTime(value: string) {
  return String(value || '').trim()
}

function requireFutureTime(value: string, label: string) {
  const text = normalizeScheduleTime(value)
  if (!text) {
    message.error(`${label}为必填，请选择预约时间`)
    return false
  }
  const parsed = dayjs(text)
  if (!parsed.isValid() || !parsed.isAfter(dayjs())) {
    message.error(`${label}必须晚于当前时间`)
    return false
  }
  return true
}

function validateScheduleTimes() {
  if (form.schedule_mode === 'build') {
    return requireFutureTime(form.build_scheduled_at, 'CI 预约时间')
  }
  if (form.schedule_mode === 'deploy') {
    return requireFutureTime(form.deploy_scheduled_at, 'CD 预约时间')
  }
  if (form.schedule_mode === 'execute') {
    return requireFutureTime(form.execute_scheduled_at, '全流程开始时间')
  }
  if (!requireFutureTime(form.build_scheduled_at, 'CI 预约时间') || !requireFutureTime(form.deploy_scheduled_at, 'CD 预约时间')) {
    return false
  }
  if (!dayjs(form.deploy_scheduled_at).isAfter(dayjs(form.build_scheduled_at))) {
    message.error('CD 时间必须晚于 CI 时间')
    return false
  }
  return true
}

function buildSchedulePayload(): ReleaseOrderSchedulePayload {
  const payload: ReleaseOrderSchedulePayload = {
    schedule_mode: form.schedule_mode,
    timezone: form.timezone.trim() || 'Asia/Shanghai',
    remark: form.remark.trim() || undefined,
  }
  if (form.schedule_mode === 'build' || form.schedule_mode === 'build_deploy') {
    payload.build_scheduled_at = normalizeScheduleTime(form.build_scheduled_at)
  }
  if (form.schedule_mode === 'deploy' || form.schedule_mode === 'build_deploy') {
    payload.deploy_scheduled_at = normalizeScheduleTime(form.deploy_scheduled_at)
  }
  if (form.schedule_mode === 'execute') {
    payload.execute_scheduled_at = normalizeScheduleTime(form.execute_scheduled_at)
  }
  return payload
}

async function submitForm() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (!validateScheduleTimes()) {
    return
  }
  if (!isEditMode.value && !availableScheduleModes.value.includes(form.schedule_mode)) {
    message.error('当前发布单不支持所选预约模式，请重新选择')
    return
  }
  submitting.value = true
  try {
    if (editingScheduleID.value) {
      await updateReleaseOrderSchedule(editingScheduleID.value, buildSchedulePayload())
      message.success('预约发布已更新')
    } else {
      await createReleaseOrderSchedule(form.release_order_id.trim(), buildSchedulePayload())
      message.success('预约发布已创建')
    }
    modalOpen.value = false
    await loadSchedules()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editingScheduleID.value ? '预约发布更新失败' : '预约发布创建失败'))
  } finally {
    submitting.value = false
  }
}

function goToReleaseOrder(record: ReleaseOrderSchedule) {
  void router.push(`/releases/${record.release_order_id}`)
}

function openScheduleDetail(record: ReleaseOrderSchedule) {
  detailSchedule.value = record
  detailDrawerOpen.value = true
}

function goToReleaseOrderID(releaseOrderID: string) {
  if (!releaseOrderID) {
    return
  }
  void router.push(`/releases/${releaseOrderID}`)
}

async function runScheduleAction(record: ReleaseOrderSchedule, action: 'submit' | 'approve' | 'reject' | 'cancel') {
  handlingActionKey.value = actionKey(record, action)
  try {
    if (action === 'submit') {
      await submitReleaseOrderScheduleApproval(record.id)
      message.success('预约审批已提交')
    } else if (action === 'approve') {
      await approveReleaseOrderSchedule(record.id)
      message.success('预约审批已通过')
    } else if (action === 'reject') {
      await rejectReleaseOrderSchedule(record.id, { comment: '拒绝预约发布' })
      message.success('预约审批已拒绝')
    } else {
      await cancelReleaseOrderSchedule(record.id)
      message.success('预约发布已取消')
    }
    await loadSchedules()
  } catch (error) {
    const actionText = action === 'submit' ? '提交审批' : action === 'approve' ? '审批通过' : action === 'reject' ? '审批拒绝' : '取消预约'
    message.error(extractHTTPErrorMessage(error, `${actionText}失败`))
    throw error
  } finally {
    handlingActionKey.value = ''
  }
}

function confirmScheduleAction(record: ReleaseOrderSchedule, action: 'submit' | 'approve' | 'reject' | 'cancel') {
  const actionText = action === 'submit' ? '提交审批' : action === 'approve' ? '审批通过' : action === 'reject' ? '审批拒绝' : '取消预约'
  Modal.confirm({
    title: `确认${actionText}吗？`,
    content:
      action === 'cancel'
        ? `预约单 ${record.schedule_no} 取消后不会再到点触发`
        : `预约单 ${record.schedule_no} 将执行${actionText}操作`,
    okText: actionText,
    cancelText: '关闭',
    okButtonProps: { danger: action === 'reject' || action === 'cancel' },
    async onOk() {
      await runScheduleAction(record, action)
    },
  })
}

onMounted(async () => {
  syncScheduleFormViewportInset()
  observeScheduleFormViewportInset()
  applyRouteQuery()
  await authStore.loadMe(true)
  await Promise.all([loadApplicationOptions(), loadEnvOptions()])
  await loadSchedules()
  if (form.release_order_id) {
    await ensureReleaseOrderOption(form.release_order_id)
    await loadSelectedReleaseOrderExecutions(form.release_order_id)
  }
})

onBeforeUnmount(() => {
  stopObservingScheduleFormViewportInset()
})
</script>

<template>
  <div class="page-wrapper schedule-page-wrapper">
    <div class="page-header-card page-header schedule-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">预约发布</h2>
      </div>
      <div class="page-header-actions schedule-header-actions">
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
        <a-button v-if="canCreateSchedule" class="release-toolbar-action-btn release-toolbar-action-btn--primary" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          创建预约
        </a-button>
      </div>
    </div>

    <a-card class="release-overview-card" :bordered="true">
      <div class="overview-bar">
        <div class="overview-chart-panel">
          <div class="overview-chart-header">
            <div>
              <div class="overview-chart-label">预约统计</div>
              <div class="overview-chart-title">全部预约状态分布</div>
            </div>
            <div class="overview-chart-meta">共 {{ overviewStatusStats.total }} 条</div>
          </div>
          <div class="schedule-overview-bars">
            <div v-for="item in scheduleOverviewBars" :key="item.key" class="schedule-overview-row">
              <div class="schedule-overview-row-head">
                <span>{{ item.label }}</span>
                <strong>{{ item.count }}</strong>
              </div>
              <div class="schedule-overview-track">
                <div class="schedule-overview-fill" :class="item.className" :style="{ width: item.width }"></div>
              </div>
            </div>
          </div>
          <div class="overview-chart-footnote">统计口径：汇总当前预约发布列表状态数量</div>
        </div>
        <div class="overview-spotlight">
          <div class="overview-spotlight-icon-wrap">
            <div class="overview-spotlight-icon-orb" :class="`overview-spotlight-icon-orb-${spotlightStateKey}`">
              <ClockCircleFilled v-if="spotlightStateKey === 'running'" class="overview-spotlight-icon" />
              <CloseCircleFilled v-else-if="spotlightStateKey === 'failed'" class="overview-spotlight-icon" />
              <CheckCircleFilled v-else-if="spotlightStateKey === 'success'" class="overview-spotlight-icon" />
              <ClockCircleFilled v-else class="overview-spotlight-icon" />
            </div>
          </div>
          <div>
            <div class="overview-spotlight-label">当前关注</div>
            <div class="overview-spotlight-text">{{ spotlightHeadline || '最新预约' }}</div>
            <div class="overview-spotlight-hint">{{ spotlightHint }}</div>
            <div v-if="spotlightSchedules.length" class="overview-spotlight-orders">
              <span class="overview-spotlight-orders-label">最新单号</span>
              <div class="overview-spotlight-order-links">
                <button
                  v-for="item in spotlightSchedules"
                  :key="item.id"
                  type="button"
                  class="overview-spotlight-order-link"
                  @click="goToReleaseOrderID(item.releaseOrderID)"
                >
                  {{ item.scheduleNo || '-' }}
                </button>
              </div>
            </div>
          </div>
          <div class="overview-spotlight-meta">
            <span>最近刷新</span>
            <strong>{{ refreshText }}</strong>
            <span>自动轮询</span>
            <strong>手动</strong>
          </div>
        </div>
      </div>
    </a-card>

    <a-card class="filter-card" :bordered="true">
      <div class="filter-entry-row">
        <div class="quick-filter-row">
          <a-button
            class="release-toolbar-action-btn release-toolbar-action-btn--primary release-quick-filter-trigger-btn"
            :class="{ 'release-quick-filter-trigger-btn--active': statusExpanded || Boolean(filters.status) }"
            @click="statusExpanded = !statusExpanded"
          >
            <template #icon>
              <FilterOutlined />
            </template>
            状态查询
            <DownOutlined :class="{ 'trigger-icon-rotate': statusExpanded }" />
          </a-button>
          <transition-group name="filter-expand">
            <a-button
              v-for="item in quickStatusOptions"
              v-show="statusExpanded"
              :key="String(item.value)"
              class="release-toolbar-action-btn release-quick-filter-chip-btn"
              :class="{ 'release-quick-filter-chip-btn--active': filters.status === item.value }"
              @click="handleQuickStatusChange(item.value)"
            >
              {{ item.label }}
            </a-button>
          </transition-group>

          <div v-if="envShortcutOptions.length > 0" class="quick-filter-divider"></div>

          <template v-if="envShortcutOptions.length > 0">
            <a-button
              class="release-toolbar-action-btn release-toolbar-action-btn--primary release-quick-filter-trigger-btn"
              :class="{ 'release-quick-filter-trigger-btn--active': envExpanded || Boolean(currentEnvFilter) }"
              @click="envExpanded = !envExpanded"
            >
              <template #icon>
                <EnvironmentOutlined />
              </template>
              环境筛选
              <DownOutlined :class="{ 'trigger-icon-rotate': envExpanded }" />
            </a-button>
            <transition-group name="filter-expand">
              <a-button
                v-for="item in envShortcutOptions"
                v-show="envExpanded"
                :key="item.value"
                class="release-toolbar-action-btn release-quick-filter-chip-btn"
                :class="{ 'release-quick-filter-chip-btn--active': currentEnvFilter === item.value }"
                @click="handleEnvQuickFilter(item.value)"
              >
                {{ item.label }}
              </a-button>
            </transition-group>
          </template>
        </div>
      </div>

      <div v-if="showAdvancedSearch" class="filter-advanced-panel">
        <div class="filter-actions-row">
          <div class="filter-actions-hint">高级条件需点击"查询"后生效</div>
          <div class="filter-actions-buttons">
            <a-button class="release-toolbar-action-btn release-toolbar-action-btn--primary" @click="handleSearch">查询</a-button>
            <a-button class="release-toolbar-action-btn release-toolbar-action-btn--ghost" @click="handleReset">重置</a-button>
          </div>
        </div>
        <a-form layout="vertical" class="filter-grid">
          <a-form-item label="检索词" class="filter-grid-item filter-grid-item--keyword">
            <a-input
              v-model:value="filters.keyword"
              class="filter-select"
              allow-clear
              placeholder="预约单号 / 发布单号 / 应用"
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
            />
          </a-form-item>
          <a-form-item label="预约模式" class="filter-grid-item">
            <a-select v-model:value="filters.schedule_mode" class="filter-select" :options="scheduleModeOptions" allow-clear />
          </a-form-item>
          <a-form-item label="预约时间" class="filter-grid-item filter-grid-item--time">
            <a-range-picker
              v-model:value="filters.scheduled_at_range"
              class="filter-select"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :show-time="{ format: 'HH:mm' }"
              format="YYYY-MM-DD HH:mm"
              allow-clear
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
        class="release-order-table schedule-table"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        row-key="id"
        :scroll="{ x: 1820 }"
      >
        <template #emptyText>
          <a-empty description="暂无预约发布，请从发布单详情或右上角创建预约" />
        </template>
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'schedule_no'">
            <button class="schedule-link-btn" type="button" @click="goToReleaseOrder(record)">
              {{ record.schedule_no || '-' }}
            </button>
          </template>
          <template v-else-if="column.key === 'release_order_no'">
            <button class="schedule-link-btn" type="button" @click="goToReleaseOrder(record)">
              {{ record.release_order_no || '-' }}
            </button>
          </template>
          <template v-else-if="column.key === 'schedule_mode'">
            <a-tag color="blue">{{ scheduleModeText(record.schedule_mode) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'build_scheduled_at'">
            {{ formatTime(record.build_scheduled_at) }}
          </template>
          <template v-else-if="column.key === 'deploy_scheduled_at'">
            {{ formatTime(record.deploy_scheduled_at) }}
          </template>
          <template v-else-if="column.key === 'execute_scheduled_at'">
            {{ formatTime(record.execute_scheduled_at) }}
          </template>
          <template v-else-if="column.key === 'cd_conflict_at'">
            {{ formatTime(record.cd_conflict_at) }}
          </template>
          <template v-else-if="column.key === 'approval_required'">
            <a-tag :color="approvalStatusColor(record)">{{ approvalStatusText(record) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="scheduleStatusColor(record.status)">{{ scheduleStatusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'approval_approver_names'">
            <span class="schedule-cell-ellipsis">{{ approverText(record) }}</span>
          </template>
          <template v-else-if="column.key === 'last_error'">
            <span class="schedule-cell-ellipsis schedule-error-text">{{ record.last_error || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'created_at'">
            {{ formatTime(record.created_at) }}
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space :size="4" wrap>
              <a-button type="link" size="small" @click="openScheduleDetail(record)">
                <template #icon>
                  <InfoCircleOutlined />
                </template>
                详情
              </a-button>
              <a-button type="link" size="small" @click="goToReleaseOrder(record)">
                <template #icon>
                  <EyeOutlined />
                </template>
                查看发布单
              </a-button>
              <a-button v-if="canEditSchedule(record)" type="link" size="small" @click="openEditModal(record)">
                <template #icon>
                  <EditOutlined />
                </template>
                编辑预约
              </a-button>
              <a-button
                v-if="canSubmitApproval(record)"
                type="link"
                size="small"
                :loading="handlingActionKey === actionKey(record, 'submit')"
                @click="confirmScheduleAction(record, 'submit')"
              >
                提交审批
              </a-button>
              <a-button
                v-if="canApproveSchedule(record)"
                type="link"
                size="small"
                :loading="handlingActionKey === actionKey(record, 'approve')"
                @click="confirmScheduleAction(record, 'approve')"
              >
                <template #icon>
                  <CheckCircleOutlined />
                </template>
                通过
              </a-button>
              <a-button
                v-if="canRejectSchedule(record)"
                danger
                type="link"
                size="small"
                :loading="handlingActionKey === actionKey(record, 'reject')"
                @click="confirmScheduleAction(record, 'reject')"
              >
                <template #icon>
                  <CloseCircleOutlined />
                </template>
                拒绝
              </a-button>
              <a-button
                v-if="canCancelSchedule(record)"
                danger
                type="link"
                size="small"
                :loading="handlingActionKey === actionKey(record, 'cancel')"
                @click="confirmScheduleAction(record, 'cancel')"
              >
                <template #icon>
                  <StopOutlined />
                </template>
                取消
              </a-button>
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
        :show-total="(count: number) => `共 ${count} 条预约`"
        @change="handlePageChange"
        @showSizeChange="handlePageChange"
      />
    </div>

    <a-modal
      :open="modalOpen"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleFormAfterClose"
      :mask-style="scheduleFormMaskStyle"
      :wrap-props="scheduleFormWrapProps"
      wrap-class-name="schedule-form-modal-wrap"
      @cancel="closeFormModal"
    >
      <template #title>
        <div class="schedule-form-modal-titlebar">
          <span class="schedule-form-modal-title">{{ modalTitle }}</span>
          <a-button class="application-toolbar-action-btn schedule-form-modal-save-btn" :loading="submitting" @click="submitForm">
            保存
          </a-button>
        </div>
      </template>

      <a-form
        ref="formRef"
        class="schedule-form"
        layout="vertical"
        :model="form"
        :required-mark="false"
        autocomplete="off"
      >
        <div class="schedule-form-note">
          预约发布会先进入预约审批，审批通过后按所选时间触发已有发布单
        </div>

        <div v-if="selectedSchedule" class="schedule-form-panel schedule-form-panel--context">
          <div class="schedule-form-panel-title">当前预约</div>
          <div class="schedule-form-context">
            <div class="schedule-form-context-item">
              <div class="schedule-form-context-label">预约单号</div>
              <div class="schedule-form-context-value">{{ selectedSchedule.schedule_no }}</div>
            </div>
            <div class="schedule-form-context-item">
              <div class="schedule-form-context-label">发布单号</div>
              <div class="schedule-form-context-value">{{ selectedSchedule.release_order_no }}</div>
            </div>
          </div>
        </div>

        <div class="schedule-form-panel">
          <div class="schedule-form-panel-title">预约配置</div>

          <a-form-item
            v-if="!isEditMode"
            name="release_order_id"
            :rules="[{ required: true, message: '请选择可预约发布单' }]"
          >
            <template #label>
              <span class="schedule-form-label">
                可预约发布单
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="form.release_order_id"
              show-search
              allow-clear
              :filter-option="false"
              :loading="loadingReleaseOrders"
              :options="releaseOrderOptions"
              not-found-content="暂无可预约发布单"
              placeholder="下拉选择符合条件的发布单"
              @change="handleReleaseOrderChange"
              @focus="loadReleaseOrderOptions()"
              @search="loadReleaseOrderOptions"
            />
          </a-form-item>

          <a-form-item name="schedule_mode" :rules="[{ required: true, message: '请选择预约模式' }]">
            <template #label>
              <span class="schedule-form-label">
                预约模式
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-radio-group
              v-model:value="form.schedule_mode"
              class="schedule-mode-button-group"
              button-style="solid"
              :disabled="!isEditMode && !form.release_order_id"
            >
              <a-radio-button
                v-for="item in scheduleModeButtonOptions"
                :key="item.value"
                :value="item.value"
                :disabled="isScheduleModeDisabled(item.value)"
              >
                {{ item.label }}
              </a-radio-button>
            </a-radio-group>
          </a-form-item>

          <a-form-item v-if="showBuildTime" name="build_scheduled_at">
            <template #label>
              <span class="schedule-form-label">
                CI 预约时间
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-date-picker
              v-model:value="form.build_scheduled_at"
              class="schedule-form-date-picker"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :show-time="{ format: 'HH:mm' }"
              format="YYYY-MM-DD HH:mm"
              placeholder="选择 CI 预约时间"
            />
          </a-form-item>

          <a-form-item v-if="showDeployTime" name="deploy_scheduled_at">
            <template #label>
              <span class="schedule-form-label">
                CD 预约时间
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-date-picker
              v-model:value="form.deploy_scheduled_at"
              class="schedule-form-date-picker"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :show-time="{ format: 'HH:mm' }"
              format="YYYY-MM-DD HH:mm"
              placeholder="选择 CD 预约时间"
            />
          </a-form-item>

          <a-form-item v-if="showExecuteTime" name="execute_scheduled_at">
            <template #label>
              <span class="schedule-form-label">
                全流程开始时间
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-date-picker
              v-model:value="form.execute_scheduled_at"
              class="schedule-form-date-picker"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              :show-time="{ format: 'HH:mm' }"
              format="YYYY-MM-DD HH:mm"
              placeholder="选择全流程开始时间"
            />
          </a-form-item>

          <a-form-item name="timezone" :rules="[{ required: true, message: '请选择时区' }]">
            <template #label>
              <span class="schedule-form-label">
                时区
                <a-tag class="schedule-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="form.timezone" :options="timezoneOptions" />
          </a-form-item>

          <a-form-item name="remark">
            <template #label>
              <span class="schedule-form-label">备注</span>
            </template>
            <a-textarea v-model:value="form.remark" placeholder="填写预约说明" :rows="3" allow-clear />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer
      v-model:open="detailDrawerOpen"
      title="预约详情"
      placement="right"
      :width="480"
      @close="detailSchedule = null"
    >
      <template v-if="detailSchedule">
        <div class="schedule-detail-body">
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">预约单号</span>
            <span class="schedule-detail-value">{{ detailSchedule.schedule_no || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">发布单号</span>
            <span class="schedule-detail-value">{{ detailSchedule.release_order_no || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">应用</span>
            <span class="schedule-detail-value">{{ detailSchedule.application_name || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">环境</span>
            <span class="schedule-detail-value">{{ detailSchedule.env_code || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">模板</span>
            <span class="schedule-detail-value">{{ detailSchedule.template_name || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">预约模式</span>
            <span class="schedule-detail-value"><a-tag color="blue">{{ scheduleModeText(detailSchedule.schedule_mode) }}</a-tag></span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">预约状态</span>
            <span class="schedule-detail-value"><a-tag :color="scheduleStatusColor(detailSchedule.status)">{{ scheduleStatusText(detailSchedule.status) }}</a-tag></span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">审批状态</span>
            <span class="schedule-detail-value"><a-tag :color="approvalStatusColor(detailSchedule)">{{ approvalStatusText(detailSchedule) }}</a-tag></span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">审批人</span>
            <span class="schedule-detail-value">{{ approverText(detailSchedule) }}</span>
          </div>
          <div class="schedule-detail-divider" />
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">CI 预约时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.build_scheduled_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">CD 预约时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.deploy_scheduled_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">全流程预约时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.execute_scheduled_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">CD 风险时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.cd_conflict_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">CI 实际触发</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.build_dispatched_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">CD 实际触发</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.deploy_dispatched_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">全流程实际触发</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.execute_dispatched_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">时区</span>
            <span class="schedule-detail-value">{{ detailSchedule.timezone || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">创建人</span>
            <span class="schedule-detail-value">{{ detailSchedule.creator_name || '-' }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">创建时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.created_at) }}</span>
          </div>
          <div class="schedule-detail-row">
            <span class="schedule-detail-label">更新时间</span>
            <span class="schedule-detail-value">{{ formatTime(detailSchedule.updated_at) }}</span>
          </div>
          <div class="schedule-detail-divider" />
          <div class="schedule-detail-row schedule-detail-row-block">
            <span class="schedule-detail-label">最近错误</span>
            <span
              class="schedule-detail-value"
              :class="{ 'schedule-detail-error': detailSchedule.last_error }"
            >{{ detailSchedule.last_error || '无' }}</span>
          </div>
          <template v-if="detailSchedule.remark">
            <div class="schedule-detail-divider" />
            <div class="schedule-detail-row schedule-detail-row-block">
              <span class="schedule-detail-label">备注</span>
              <span class="schedule-detail-value">{{ detailSchedule.remark }}</span>
            </div>
          </template>
        </div>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.schedule-page-wrapper {
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

.schedule-header-actions {
  display: flex;
  flex-wrap: wrap;
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

.release-quick-filter-chip-btn {
  min-width: 108px;
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

.release-quick-filter-trigger-btn {
  min-width: 126px;
  padding-inline: 16px;
}

.release-quick-filter-chip-btn {
  padding-inline: 14px;
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

.release-overview-card,
.filter-card,
.table-card {
  border-radius: var(--radius-xl);
}

.release-overview-card {
  background: transparent;
  border: none;
  box-shadow: none;
}

.release-overview-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.filter-card {
  background: transparent;
  border: none;
  box-shadow: none;
}

.filter-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.table-card {
  background: transparent;
  border: none;
  box-shadow: none;
}

.table-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.overview-bar {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(240px, 0.85fr);
  gap: 14px;
}

.overview-chart-panel {
  position: relative;
  min-height: 236px;
  border-radius: 20px;
  padding: 18px;
  border: 1px solid rgba(71, 85, 105, 0.4);
  background:
    radial-gradient(circle at top right, rgba(52, 211, 153, 0.14), transparent 24%),
    radial-gradient(circle at top left, rgba(96, 165, 250, 0.16), transparent 30%),
    linear-gradient(180deg, rgba(2, 6, 23, 0.98), rgba(15, 23, 42, 0.96) 48%, rgba(19, 30, 53, 0.96));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 22px 48px rgba(2, 6, 23, 0.16);
  overflow: hidden;
}

.overview-chart-panel::before {
  content: "";
  position: absolute;
  inset: 0 0 auto;
  height: 1px;
  background: linear-gradient(90deg, rgba(56, 189, 248, 0), rgba(56, 189, 248, 0.46), rgba(52, 211, 153, 0.32), rgba(56, 189, 248, 0));
  pointer-events: none;
}

.overview-chart-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 14px;
  margin-bottom: 12px;
}

.overview-chart-label {
  color: rgba(125, 211, 252, 0.92);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.overview-chart-title {
  margin-top: 6px;
  color: #f8fafc;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

.overview-chart-meta {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  min-height: 36px;
  padding: 0 14px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.34);
  background: rgba(15, 23, 42, 0.44);
  color: rgba(226, 232, 240, 0.7);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.schedule-overview-bars {
  display: grid;
  gap: 11px;
  margin-top: 16px;
}

.schedule-overview-row-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: rgba(226, 232, 240, 0.72);
  font-size: 12px;
  font-weight: 700;
}

.schedule-overview-row-head strong {
  color: #f8fafc;
  font-weight: 800;
}

.schedule-overview-track {
  height: 9px;
  margin-top: 7px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
  overflow: hidden;
}

.schedule-overview-fill {
  height: 100%;
  min-width: 0;
  border-radius: inherit;
  transition: width 0.2s ease;
}

.schedule-overview-fill--pending {
  background: linear-gradient(90deg, #facc15, #fde68a);
}

.schedule-overview-fill--scheduled {
  background: linear-gradient(90deg, #60a5fa, #93c5fd);
}

.schedule-overview-fill--running {
  background: linear-gradient(90deg, #22d3ee, #67e8f9);
}

.schedule-overview-fill--success {
  background: linear-gradient(90deg, #22c55e, #86efac);
}

.schedule-overview-fill--risk {
  background: linear-gradient(90deg, #fb7185, #fecdd3);
}

.overview-chart-footnote {
  margin-top: 14px;
  color: rgba(226, 232, 240, 0.54);
  font-size: 12px;
  line-height: 1.6;
}

.overview-spotlight {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 18px;
  min-height: 236px;
  padding: 18px;
  border-radius: 20px;
  border: 1px solid var(--color-dashboard-border);
  background:
    radial-gradient(circle at top right, var(--color-primary-glow-strong), transparent 38%),
    linear-gradient(145deg, var(--color-dashboard-900) 0%, var(--color-primary-600) 100%);
  color: var(--color-dashboard-text);
  position: relative;
  overflow: hidden;
}

.overview-spotlight-icon-wrap {
  position: absolute;
  top: 16px;
  right: 16px;
}

.overview-spotlight-icon-orb {
  width: 64px;
  height: 64px;
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.18);
  background: rgba(255, 255, 255, 0.08);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.2),
    0 10px 24px rgba(15, 23, 42, 0.22);
  backdrop-filter: blur(8px);
}

.overview-spotlight-icon {
  font-size: 28px;
  color: #eff6ff;
}

.overview-spotlight-icon-orb-running {
  color: #bfdbfe;
}

.overview-spotlight-icon-orb-failed {
  color: #fecdd3;
}

.overview-spotlight-icon-orb-success {
  color: #bbf7d0;
}

.overview-spotlight-icon-orb-pending {
  color: #fde68a;
}

.overview-spotlight-label {
  padding-right: 92px;
  font-size: 12px;
  letter-spacing: 0.08em;
  color: var(--color-dashboard-label);
}

.overview-spotlight-text {
  margin-top: 14px;
  padding-right: 92px;
  font-size: 24px;
  line-height: 1.2;
  font-weight: 800;
  letter-spacing: 0;
}

.overview-spotlight-hint {
  margin-top: 8px;
  padding-right: 88px;
  color: var(--color-dashboard-text-soft);
  font-size: 13px;
  line-height: 1.55;
}

.overview-spotlight-orders {
  margin-top: 12px;
  padding-right: 18px;
}

.overview-spotlight-orders-label {
  display: block;
  margin-bottom: 8px;
  color: rgba(226, 232, 240, 0.58);
  font-size: 12px;
  line-height: 1;
}

.overview-spotlight-order-links {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.overview-spotlight-order-link {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  max-width: 100%;
  padding: 0 10px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
  color: #f8fafc;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  transition:
    background 0.18s ease,
    border-color 0.18s ease,
    transform 0.18s ease;
}

.overview-spotlight-order-link:hover,
.overview-spotlight-order-link:focus-visible {
  border-color: rgba(191, 219, 254, 0.46);
  background: rgba(255, 255, 255, 0.18);
  color: #ffffff;
  transform: translateY(-1px);
}

.overview-spotlight-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--color-dashboard-text-soft);
}

.overview-spotlight-meta span {
  color: rgba(226, 232, 240, 0.58);
}

.overview-spotlight-meta strong {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 9px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
  color: #f8fafc;
  font-weight: 700;
}

.quick-filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  flex: 1;
  min-width: 0;
  align-items: center;
}

.quick-filter-divider {
  width: 1px;
  height: 24px;
  background: rgba(148, 163, 184, 0.24);
  flex-shrink: 0;
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

.filter-entry-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.filter-advanced-panel {
  margin-top: 18px;
  padding: 18px;
  border-radius: 18px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: transparent;
  box-shadow: none;
}

.filter-actions-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  padding-bottom: 14px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.12);
}

.filter-actions-hint {
  color: var(--color-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.filter-actions-buttons {
  display: flex;
  align-items: center;
  gap: 10px;
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 14px 16px;
}

.filter-grid :deep(.ant-form-item) {
  margin-bottom: 0;
}

.filter-grid-item :deep(.ant-form-item-label) {
  padding-bottom: 6px;
}

.filter-grid-item :deep(.ant-form-item-label > label) {
  color: var(--color-text-secondary);
  font-size: 12px;
}

.filter-grid-item--keyword,
.filter-grid-item--app {
  grid-column: span 2;
}

.filter-grid-item--time {
  grid-column: span 2;
}

.filter-select {
  width: 100%;
}

.active-filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px dashed var(--color-border);
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

:deep(.release-order-table .ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.36);
}

:deep(.release-order-table .ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(59, 130, 246, 0.24);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #eff6ff !important;
  font-size: 12px;
  font-weight: 700;
}

:deep(.release-order-table .ant-table-thead > tr > th::before) {
  display: none;
}

:deep(.release-order-table .ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: #334155;
}

:deep(.release-order-table .ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

:deep(.release-order-table .ant-table-cell-fix-right) {
  background: #ffffff !important;
}

.schedule-link-btn {
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: #1d4ed8;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
}

.schedule-link-btn:hover,
.schedule-link-btn:focus-visible {
  color: #1e40af;
  text-decoration: underline;
  outline: none;
}

.schedule-cell-ellipsis {
  display: inline-block;
  max-width: 190px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.schedule-error-text {
  color: #dc2626;
}

.pagination-area {
  display: flex;
  justify-content: flex-end;
}

:global(.schedule-form-modal-wrap .ant-modal-content) {
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

:global(.schedule-form-modal-wrap .ant-modal-header) {
  margin-bottom: 0;
  border-bottom: none;
  background: transparent;
}

.schedule-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.schedule-form-modal-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
}

.schedule-form-modal-save-btn {
  flex: none;
}

.schedule-form {
  padding-top: 18px;
}

.schedule-form-note {
  position: relative;
  margin-bottom: 18px;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.schedule-form-note::before {
  content: '';
  position: absolute;
  top: 3px;
  bottom: 3px;
  left: 0;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.schedule-form-panel + .schedule-form-panel,
.schedule-form-note + .schedule-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.schedule-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
}

.schedule-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.schedule-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.schedule-form-context-item {
  min-width: 0;
  padding-bottom: 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.schedule-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.schedule-form-context-value {
  overflow: hidden;
  color: #0f172a;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.6;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.schedule-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.schedule-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.schedule-form-date-picker {
  width: 100%;
}

.schedule-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.schedule-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.schedule-form :deep(.ant-input),
.schedule-form :deep(.ant-input-affix-wrapper),
.schedule-form :deep(.ant-select-selector),
.schedule-form :deep(.ant-picker),
.schedule-form :deep(.ant-input textarea),
.schedule-form :deep(textarea.ant-input) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.schedule-form :deep(.ant-input:hover),
.schedule-form :deep(.ant-input-affix-wrapper:hover),
.schedule-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.schedule-form :deep(.ant-picker:hover) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.schedule-form :deep(.ant-input:focus),
.schedule-form :deep(.ant-input-focused),
.schedule-form :deep(.ant-input-affix-wrapper-focused),
.schedule-form :deep(.ant-select-focused .ant-select-selector),
.schedule-form :deep(.ant-picker-focused) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

@media (max-width: 1024px) {
  .schedule-page-header {
    flex-direction: column;
  }

  .schedule-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .overview-bar {
    grid-template-columns: 1fr;
  }

  .filter-entry-row,
  .filter-actions-row {
    align-items: stretch;
    flex-direction: column;
  }

  .filter-actions-buttons {
    justify-content: flex-start;
  }

  .filter-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .filter-grid-item--keyword,
  .filter-grid-item--app,
  .filter-grid-item--time {
    grid-column: span 1;
  }
}

@media (max-width: 768px) {
  .release-toolbar-action-btn,
  :deep(.application-toolbar-action-btn.ant-btn) {
    width: 100%;
  }

  .quick-filter-row {
    flex-direction: column;
    align-items: stretch;
  }

  .quick-filter-divider {
    display: none;
  }

  .filter-grid {
    grid-template-columns: 1fr;
  }

  .schedule-form-context {
    grid-template-columns: 1fr;
  }
}

.schedule-detail-body {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.schedule-detail-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 12px;
  align-items: flex-start;
  padding: 10px 0;
  border-bottom: 1px solid rgba(226, 232, 240, 0.7);
}

.schedule-detail-row-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.schedule-detail-row-block .schedule-detail-value {
  white-space: pre-wrap;
  word-break: break-all;
}

.schedule-detail-label {
  color: #64748b;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.6;
  flex-shrink: 0;
}

.schedule-detail-value {
  color: #0f172a;
  font-size: 13px;
  font-weight: 500;
  line-height: 1.6;
  word-break: break-word;
  min-width: 0;
}

.schedule-detail-error {
  color: #b91c1c;
  background: rgba(239, 68, 68, 0.06);
  border-radius: 10px;
  padding: 10px 14px;
  border: 1px solid rgba(239, 68, 68, 0.18);
}

.schedule-detail-divider {
  height: 1px;
  background: rgba(226, 232, 240, 0.7);
  margin: 4px 0;
}
</style>
