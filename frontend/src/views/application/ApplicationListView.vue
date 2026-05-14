<script setup lang="ts">
import {
  AimOutlined,
  ExclamationCircleOutlined,
  MoreOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  RocketOutlined,
  SearchOutlined,
  SoundOutlined,
  SyncOutlined,
  WarningOutlined,
} from '@ant-design/icons-vue'
import { Modal, message } from 'ant-design-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts/core'
import type { ECharts } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, h, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { deleteApplication, listApplications } from '../../api/application'
import { listActiveAnnouncements } from '../../api/announcement'
import type { Announcement } from '../../types/announcement'
import { listProjects } from '../../api/project'
import {
  createApplicationRollbackOrder,
  getApplicationRollbackCapability,
  getApplicationRollbackPrecheck,
  listAppReleaseStateSummaries,
  listReleaseOrders,
  listAllReleaseTemplates,
} from '../../api/release'
import { useApplicationListStore } from '../../stores/application-list'
import { useAuthStore } from '../../stores/auth'
import type { Application, ApplicationStatus } from '../../types/application'
import type { Project } from '../../types/project'
import type {
  ApplicationRollbackCapability,
  ApplicationRollbackPrecheck,
  ApplicationRollbackPrecheckParam,
  AppReleaseStateSummary,
  ReleaseOperationType,
  ReleaseOrder,
  ReleaseOrderBusinessStatus,
  RollbackSupportedAction,
} from '../../types/release'
import { extractHTTPErrorMessage, isHTTPStatus } from '../../utils/http-error'

type MetricTone = 'default' | 'success' | 'running' | 'danger' | 'warning'
type OverviewCardKey = 'pending' | 'running' | 'failed' | 'ready'
const gitOpsEnvOrder = ['dev', 'test', 'prod']

function releaseBusinessStatus(order: Pick<ReleaseOrder, 'status' | 'business_status'>): ReleaseOrderBusinessStatus {
  if (order.business_status) {
    return order.business_status
  }
  switch (order.status) {
    case 'draft':
      return 'draft'
    case 'pending_approval':
      return 'pending_approval'
    case 'approving':
      return 'approving'
    case 'approved':
      return 'approved'
    case 'rejected':
      return 'rejected'
    case 'queued':
      return 'queued'
    case 'deploying':
      return 'deploying'
    case 'deploy_success':
    case 'success':
      return 'deploy_success'
    case 'deploy_failed':
    case 'failed':
      return 'deploy_failed'
    case 'running':
      return 'deploying'
    case 'cancelled':
      return 'cancelled'
    default:
      return 'pending_execution'
  }
}

interface WorkbenchEnvSection {
  envCode: string
  latestOrder: ReleaseOrder | null
  stateSummary: AppReleaseStateSummary | null
}

function normalizedValue(value: string | null | undefined) {
  return String(value || '').trim()
}

function resolvedCurrentLiveOrderID(section: WorkbenchEnvSection) {
  const summaryID = normalizedValue(section.stateSummary?.current_release_order_id)
  if (summaryID) {
    return summaryID
  }
  return normalizedValue(section.latestOrder?.id)
}

function resolvedCurrentLiveOrderNo(section: WorkbenchEnvSection) {
  const summaryNo = normalizedValue(section.stateSummary?.current_release_order_no)
  if (summaryNo) {
    return summaryNo
  }
  return normalizedValue(section.latestOrder?.order_no)
}

interface WorkbenchCard {
  application: Application
  latestOrder: ReleaseOrder | null
  envSections: WorkbenchEnvSection[]
  templateNames: string[]
  releaseReady: boolean
  runningCount: number
}

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

interface OverviewStatCard {
  key: OverviewCardKey
  label: string
  value: string
  hint: string
  tone: MetricTone
  icon: unknown
  clickable: boolean
}

interface OverviewTrendModel {
  labels: string[]
  success: number[]
  running: number[]
  failed: number[]
  maxValue: number
  totalActivity: number
}

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const router = useRouter()
const listStore = useApplicationListStore()
const authStore = useAuthStore()

const loading = ref(false)
const loadingProjects = ref(false)
const deletingId = ref('')
const dataSource = ref<Application[]>([])
const announcements = ref<Announcement[]>([])
const announcementPopoverVisible = ref(false)
const lastSeenAnnouncementIds = ref<string[]>([])

const hasUnreadAnnouncement = computed(() => {
  if (announcements.value.length === 0) return false
  const seen = new Set(lastSeenAnnouncementIds.value)
  return announcements.value.some(a => !seen.has(a.id))
})

const unreadCount = computed(() => {
  const seen = new Set(lastSeenAnnouncementIds.value)
  return announcements.value.filter(a => !seen.has(a.id)).length
})

function markAsRead() {
  lastSeenAnnouncementIds.value = announcements.value.map(a => a.id)
  try {
    localStorage.setItem('gos-seen-announcement-ids', JSON.stringify(lastSeenAnnouncementIds.value))
  } catch { /* ignore */ }
  announcementPopoverVisible.value = false
}
const total = ref(0)
const loadingTemplateAvailability = ref(false)
const loadingRecentReleases = ref(false)
const loadingReleaseStateSummaries = ref(false)
const loadingOverviewMetrics = ref(false)
const templateApplicationIDs = ref<Set<string>>(new Set())
const templateNamesByApplication = ref<Map<string, string[]>>(new Map())
const recentReleaseOrders = ref<ReleaseOrder[]>([])
const overviewApplications = ref<Application[]>([])
const overviewRecentReleaseOrders = ref<ReleaseOrder[]>([])
const releaseStateSummaries = ref<AppReleaseStateSummary[]>([])
const projectOptions = ref<{ label: string; value: string }[]>([])
const introVisible = ref(false)
const applicationInfoDrawerVisible = ref(false)
const selectedApplication = ref<Application | null>(null)
const releaseDetailCardID = ref('')
const selectedReleaseEnvByApplication = ref<Record<string, string>>({})
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const firstWorkbenchLoaded = ref(false)
const searchDialogVisible = ref(false)
const searchInputRef = ref()
const overviewTrendChartRef = ref<HTMLElement | null>(null)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
const searchDraft = reactive<{
  keyword: string
}>({
  keyword: '',
})
const rollbackActionLoadingKey = ref('')
const rollbackCapabilityHints = ref<Map<string, ApplicationRollbackCapability>>(new Map())
const overflowingAppKeys = ref<Record<string, boolean>>({})
const overflowingAppTitles = ref<Record<string, boolean>>({})
const appKeyElements = new Map<string, HTMLElement>()
const appTitleElements = new Map<string, HTMLElement>()
let autoRefreshTimer: ReturnType<typeof window.setInterval> | null = null
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0
let overviewTrendChart: ECharts | null = null

const canManageApplication = computed(() => authStore.hasPermission('application.manage'))
const canViewPipeline = computed(() => authStore.hasPermission('pipeline.view'))
const canOpenWorkbenchConfig = computed(() => canManageApplication.value)
const overviewApplicationIDs = computed(() => new Set(overviewApplications.value.map((item) => String(item.id || '').trim()).filter(Boolean)))
const overviewVisibleOrders = computed(() =>
  overviewRecentReleaseOrders.value.filter((item) =>
    overviewApplicationIDs.value.has(String(item.application_id || '').trim()),
  ),
)
const workbenchLoading = computed(() =>
  loading.value ||
  loadingTemplateAvailability.value ||
  loadingRecentReleases.value ||
  loadingReleaseStateSummaries.value ||
  loadingOverviewMetrics.value,
)
const initialWorkbenchLoading = computed(() => !firstWorkbenchLoaded.value && workbenchLoading.value && dataSource.value.length === 0)
const projectFilterValue = computed<string | undefined>({
  get: () => {
    const value = String(listStore.project_id || '').trim()
    return value || undefined
  },
  set: (value) => {
    listStore.project_id = String(value || '').trim()
  },
})

const filters = computed(() => ({
  keyword: String(listStore.keyword || '').trim() || undefined,
  project_id: String(listStore.project_id || '').trim() || undefined,
  status: (String(listStore.status || '').trim() as ApplicationStatus | '') || undefined,
  page: listStore.page,
  page_size: listStore.pageSize,
}))

const recentOrdersByApplication = computed(() => {
  const grouped = new Map<string, ReleaseOrder[]>()
  recentReleaseOrders.value.forEach((item) => {
    const appID = String(item.application_id || '').trim()
    if (!appID) {
      return
    }
    const list = grouped.get(appID) || []
    list.push(item)
    grouped.set(appID, list)
  })
  return grouped
})

const releaseStateSummaryByAppEnv = computed(() => {
  const grouped = new Map<string, AppReleaseStateSummary>()
  releaseStateSummaries.value.forEach((item) => {
    const key = `${String(item.application_id || '').trim()}::${String(item.env_code || '').trim()}`
    if (!key || grouped.has(key)) {
      return
    }
    grouped.set(key, item)
  })
  return grouped
})

const workbenchCards = computed<WorkbenchCard[]>(() =>
  dataSource.value.map((application) => {
    const appID = String(application.id || '').trim()
    const orders = recentOrdersByApplication.value.get(appID) || []
    const envMap = new Map<string, ReleaseOrder>()
    orders.forEach((item) => {
      const envCode = String(item.env_code || '').trim()
      if (!envCode || envMap.has(envCode)) {
        return
      }
      envMap.set(envCode, item)
    })
    const summaryEnvCodes = releaseStateSummaries.value
      .filter((item) => String(item.application_id || '').trim() === appID)
      .map((item) => String(item.env_code || '').trim())
      .filter(Boolean)
    const envSections = orderedEnvCodes(Array.from(new Set([...envMap.keys(), ...summaryEnvCodes]))).map((envCode) => ({
      envCode,
      latestOrder: envMap.get(envCode) || null,
      stateSummary: releaseStateSummaryByAppEnv.value.get(`${appID}::${envCode}`) || null,
    }))
    const templateNames = templateNamesByApplication.value.get(appID) || []
    return {
      application,
      latestOrder: orders[0] || null,
      envSections,
      templateNames,
      releaseReady: canReleaseApplication(appID),
      runningCount: orders.filter((item) => releaseBusinessStatus(item) === 'deploying').length,
    }
  }),
)

const workbenchColumnCount = computed(() => {
  if (viewportWidth.value <= 980) {
    return 1
  }
  if (viewportWidth.value <= 1680) {
    return 2
  }
  return 3
})

function cardActivityStatusText(runningCount: number) {
  return runningCount > 0 ? '正在发布' : '暂无动作'
}

function cardActivityStatusClass(runningCount: number) {
  return runningCount > 0 ? 'workbench-status-chip-running' : 'workbench-status-chip-idle'
}

function releaseOverviewText(card: WorkbenchCard) {
  return card.latestOrder ? `最近发布：${card.latestOrder.order_no}` : '最近发布：暂无最近发布'
}

function releaseOverviewStatusText(card: WorkbenchCard) {
  return `状态：${cardActivityStatusText(card.runningCount)}`
}

function releaseStatusChipClass(status: ReleaseOrderBusinessStatus) {
  switch (status) {
    case 'deploy_success':
      return 'dashboard-chip dashboard-chip-running'
    case 'deploying':
      return 'dashboard-chip dashboard-chip-warning'
    case 'deploy_failed':
    case 'rejected':
    case 'cancelled':
      return 'dashboard-chip dashboard-chip-danger'
    default:
      return 'dashboard-chip dashboard-chip-neutral'
  }
}

function releaseOrderStatusText(order: ReleaseOrder | null | undefined) {
  if (!order) {
    return '暂无状态'
  }
  return releaseStatusText(releaseBusinessStatus(order))
}

function releaseOrderStatusTagClass(order: ReleaseOrder | null | undefined) {
  if (!order) {
    return 'dashboard-chip dashboard-chip-neutral'
  }
  return releaseStatusChipClass(releaseBusinessStatus(order))
}

function currentLiveImageTag(section: WorkbenchEnvSection) {
  return normalizedValue(section.stateSummary?.current_image_tag) || normalizedValue(section.latestOrder?.image_tag) || '-'
}

function currentLiveConfirmedAt(section: WorkbenchEnvSection) {
  return (
    section.stateSummary?.current_confirmed_at ||
    section.latestOrder?.live_state_confirmed_at ||
    section.latestOrder?.finished_at ||
    section.latestOrder?.updated_at ||
    ''
  )
}

function currentLiveConfirmedBy(section: WorkbenchEnvSection) {
  return normalizedValue(section.stateSummary?.current_confirmed_by) || normalizedValue(section.latestOrder?.live_state_confirmed_by) || '-'
}

function currentLiveOrderCardClass(section: WorkbenchEnvSection) {
  if (!resolvedCurrentLiveOrderNo(section)) {
    return 'env-status-neutral'
  }
  if (!section.latestOrder) {
    return 'env-status-success'
  }
  return releaseStatusClass(releaseBusinessStatus(section.latestOrder))
}

function hasReleaseDetailData(card: WorkbenchCard) {
  return Boolean(card.latestOrder) || card.envSections.some((section) => Boolean(resolvedCurrentLiveOrderNo(section)))
}

function releaseDetailEnvSections(card: WorkbenchCard) {
  const sections = card.envSections.filter((section) => Boolean(resolvedCurrentLiveOrderNo(section)) || Boolean(section.latestOrder))
  return sections.length > 0 ? sections : card.envSections
}

function defaultReleaseDetailSection(card: WorkbenchCard) {
  const sections = releaseDetailEnvSections(card)
  if (sections.length === 0) {
    return null
  }
  const latestOrderID = normalizedValue(card.latestOrder?.id)
  if (latestOrderID) {
    const latestSection = sections.find((section) => normalizedValue(section.latestOrder?.id) === latestOrderID)
    if (latestSection) {
      return latestSection
    }
  }
  return sections[0]
}

function selectedReleaseSection(card: WorkbenchCard) {
  const sections = releaseDetailEnvSections(card)
  if (sections.length === 0) {
    return null
  }
  const applicationID = normalizedValue(card.application.id)
  const selectedEnv = normalizedValue(selectedReleaseEnvByApplication.value[applicationID])
  return sections.find((section) => section.envCode === selectedEnv) || defaultReleaseDetailSection(card)
}

function setSelectedReleaseEnv(applicationID: string, envCode: string) {
  const id = normalizedValue(applicationID)
  const normalizedEnvCode = normalizedValue(envCode)
  if (!id || !normalizedEnvCode) {
    return
  }
  selectedReleaseEnvByApplication.value = {
    ...selectedReleaseEnvByApplication.value,
    [id]: normalizedEnvCode,
  }
}

function ensureSelectedReleaseEnv(card: WorkbenchCard) {
  const section = selectedReleaseSection(card)
  if (!section) {
    return
  }
  setSelectedReleaseEnv(card.application.id, section.envCode)
}

function canSwitchReleaseEnv(card: WorkbenchCard) {
  return releaseDetailEnvSections(card).length > 1
}

function switchReleaseEnv(card: WorkbenchCard) {
  const sections = releaseDetailEnvSections(card)
  if (sections.length <= 1) {
    return
  }
  const currentEnv = selectedReleaseSection(card)?.envCode || sections[0].envCode
  const currentIndex = Math.max(0, sections.findIndex((section) => section.envCode === currentEnv))
  const nextSection = sections[(currentIndex + 1) % sections.length]
  setSelectedReleaseEnv(card.application.id, nextSection.envCode)
}

function releaseDetailEnvText(card: WorkbenchCard) {
  return selectedReleaseSection(card)?.envCode || normalizedValue(card.latestOrder?.env_code) || '-'
}

function releaseDetailCurrentOrderID(card: WorkbenchCard) {
  const section = selectedReleaseSection(card)
  return section ? resolvedCurrentLiveOrderID(section) : ''
}

function releaseDetailCurrentOrderNo(card: WorkbenchCard) {
  const section = selectedReleaseSection(card)
  return section ? resolvedCurrentLiveOrderNo(section) : ''
}

function releaseDetailLatestOrder(card: WorkbenchCard) {
  const section = selectedReleaseSection(card)
  return section ? section.latestOrder : card.latestOrder
}

function releaseDetailLatestOrderID(card: WorkbenchCard) {
  return normalizedValue(releaseDetailLatestOrder(card)?.id)
}

function releaseDetailLatestOrderNo(card: WorkbenchCard) {
  return normalizedValue(releaseDetailLatestOrder(card)?.order_no)
}

function releaseDetailActionText(card: WorkbenchCard) {
  return hasReleaseDetailData(card) ? '详情' : '查单'
}

function projectLabel(application: Application) {
  return application.project_name
    ? `${application.project_name}${application.project_key ? ` (${application.project_key})` : ''}`
    : '-'
}

function baselineInfoRows(application: Application) {
  return [
    { key: 'name', label: '应用名称', value: application.name || '-' },
    { key: 'key', label: '应用 Key', value: application.key || '-' },
    { key: 'project', label: '归属项目', value: projectLabel(application) },
    { key: 'status', label: '状态', value: applicationStatusText(application.status) },
    { key: 'owner', label: '负责人', value: application.owner || '-' },
    { key: 'artifact_type', label: '制品类型', value: application.artifact_type || '-' },
    { key: 'language', label: '语言', value: application.language || '-' },
    { key: 'repo_url', label: '仓库地址', value: application.repo_url || '-' },
    { key: 'description', label: '描述', value: application.description || '-' },
    { key: 'updated_at', label: '更新时间', value: formatDateTime(application.updated_at) },
  ]
}

function gitOpsEnvRank(envCode: string) {
  const index = gitOpsEnvOrder.indexOf(String(envCode || '').trim().toLowerCase())
  return index >= 0 ? index : gitOpsEnvOrder.length
}

function sortedGitOpsMappings(application: Application) {
  return [...(application.gitops_branch_mappings || [])].sort((left, right) => {
    const rankDiff = gitOpsEnvRank(left.env_code) - gitOpsEnvRank(right.env_code)
    if (rankDiff !== 0) {
      return rankDiff
    }
    const envDiff = String(left.env_code || '').localeCompare(String(right.env_code || ''))
    if (envDiff !== 0) {
      return envDiff
    }
    return String(left.branch || '').localeCompare(String(right.branch || ''))
  })
}

function openApplicationInfoDrawer(application: Application) {
  selectedApplication.value = application
  applicationInfoDrawerVisible.value = true
}

function closeApplicationInfoDrawer() {
  applicationInfoDrawerVisible.value = false
}

function toggleReleaseDetailCard(card: WorkbenchCard) {
  const id = normalizedValue(card.application.id)
  if (!id) {
    return
  }
  if (releaseDetailCardID.value === id) {
    releaseDetailCardID.value = ''
    return
  }
  ensureSelectedReleaseEnv(card)
  releaseDetailCardID.value = id
}

function closeReleaseDetailCard() {
  releaseDetailCardID.value = ''
}

function isReleaseDetailCard(applicationID: string) {
  return releaseDetailCardID.value === normalizedValue(applicationID)
}

function handleReleaseDetailAction(card: WorkbenchCard) {
  if (!hasReleaseDetailData(card)) {
    toReleaseRecords(card.application.id)
    return
  }
  toggleReleaseDetailCard(card)
}

function setAppKeyElement(applicationID: string, element: Element | null) {
  const id = String(applicationID || '').trim()
  if (!id) {
    return
  }
  if (element instanceof HTMLElement) {
    appKeyElements.set(id, element)
    return
  }
  appKeyElements.delete(id)
}

function setAppTitleElement(applicationID: string, element: Element | null) {
  const id = String(applicationID || '').trim()
  if (!id) {
    return
  }
  if (element instanceof HTMLElement) {
    appTitleElements.set(id, element)
    return
  }
  appTitleElements.delete(id)
}

function shouldShowAppKeyFade(applicationID: string) {
  const id = String(applicationID || '').trim()
  return overflowingAppKeys.value[id] === true && overflowingAppTitles.value[id] === true
}

function measureAppKeyOverflow() {
  const nextKey: Record<string, boolean> = {}
  const nextTitle: Record<string, boolean> = {}
  appKeyElements.forEach((element, id) => {
    nextKey[id] = element.scrollWidth - element.clientWidth > 1
  })
  appTitleElements.forEach((element, id) => {
    nextTitle[id] = element.scrollWidth - element.clientWidth > 1
  })
  overflowingAppKeys.value = nextKey
  overflowingAppTitles.value = nextTitle
}

function scheduleMeasureAppKeyOverflow() {
  void nextTick(() => {
    if (typeof window !== 'undefined') {
      window.requestAnimationFrame(() => {
        measureAppKeyOverflow()
      })
      return
    }
    measureAppKeyOverflow()
  })
}

const overviewTrendModel = computed<OverviewTrendModel>(() => {
  const end = dayjs().startOf('hour')
  const start = end.subtract(23, 'hour')
  const labels = Array.from({ length: 24 }, (_, index) => start.add(index, 'hour').format('HH:mm'))
  const success = Array.from({ length: 24 }, () => 0)
  const running = Array.from({ length: 24 }, () => 0)
  const failed = Array.from({ length: 24 }, () => 0)

  overviewVisibleOrders.value.forEach((item) => {
    const timestamp = dayjs(item.updated_at || item.finished_at || item.started_at || item.created_at)
    if (!timestamp.isValid()) {
      return
    }
    const hourIndex = timestamp.startOf('hour').diff(start, 'hour')
    if (hourIndex < 0 || hourIndex >= 24) {
      return
    }

    const status = releaseBusinessStatus(item)
    if (status === 'deploy_success') {
      success[hourIndex] += 1
      return
    }
    if (status === 'deploy_failed') {
      failed[hourIndex] += 1
      return
    }
    if (status === 'deploying') {
      running[hourIndex] += 1
    }
  })

  const maxValue = Math.max(1, ...success, ...running, ...failed)
  const totalActivity = [...success, ...running, ...failed].reduce((sum, item) => sum + item, 0)

  return {
    labels,
    success,
    running,
    failed,
    maxValue,
    totalActivity,
  }
})

const overviewSummaryCards = computed<OverviewStatCard[]>(() => {
  const today = dayjs()
  const pendingIDs = new Set<string>()
  const runningIDs = new Set<string>()
  let failedToday = 0

  overviewVisibleOrders.value.forEach((item) => {
    const orderID = String(item.id || '').trim()
    const status = releaseBusinessStatus(item)
    if ((status === 'pending_approval' || status === 'approving') && orderID) {
      pendingIDs.add(orderID)
    }
    if (status === 'deploying' && orderID) {
      runningIDs.add(orderID)
    }
    if (status === 'deploy_failed' && dayjs(item.updated_at || item.created_at).isSame(today, 'day')) {
      failedToday += 1
    }
  })

  return [
    {
      key: 'pending',
      label: '待审批',
      value: String(pendingIDs.size),
      hint: '审批工作台中的待处理单',
      tone: 'warning',
      icon: ExclamationCircleOutlined,
      clickable: true,
    },
    {
      key: 'running',
      label: '发布中',
      value: String(runningIDs.size),
      hint: '当前处于部署执行的发布单',
      tone: 'running',
      icon: SyncOutlined,
      clickable: true,
    },
    {
      key: 'failed',
      label: '今日失败',
      value: String(failedToday),
      hint: '今天发布失败的单子',
      tone: 'danger',
      icon: WarningOutlined,
      clickable: true,
    },
    {
      key: 'ready',
      label: '可发布',
      value: String(
        overviewApplications.value.filter((item) => canReleaseApplication(String(item.id || '').trim())).length,
      ),
      hint: '有模板且具备发布权限',
      tone: 'success',
      icon: RocketOutlined,
      clickable: false,
    },
  ]
})

const overviewHeaderMeta = computed(() => {
  if (overviewApplications.value.length === 0) {
    return '尚无可见应用'
  }
  if (overviewTrendModel.value.totalActivity <= 0) {
    return `全部应用 ${overviewApplications.value.length} · 近 24 小时暂无波动`
  }
  return `全部应用 ${overviewApplications.value.length} · 近 24 小时共 ${overviewTrendModel.value.totalActivity} 次状态变化`
})

async function listAllApplicationsForOverview() {
  const pageSize = 100
  let page = 1
  const items: Application[] = []

  for (;;) {
    const response = await listApplications({
      page,
      page_size: pageSize,
    })
    items.push(...(response.data || []))
    if (items.length >= response.total || (response.data || []).length < pageSize) {
      break
    }
    page += 1
  }

  return items
}

async function mapSettledWithConcurrency<T, R>(
  items: T[],
  limit: number,
  worker: (item: T) => Promise<R>,
) {
  const results: PromiseSettledResult<R>[] = new Array(items.length)
  let cursor = 0

  async function runNext() {
    for (;;) {
      const index = cursor
      cursor += 1
      if (index >= items.length) {
        return
      }
      try {
        results[index] = {
          status: 'fulfilled',
          value: await worker(items[index]),
        }
      } catch (error) {
        results[index] = {
          status: 'rejected',
          reason: error,
        }
      }
    }
  }

  await Promise.all(
    Array.from({ length: Math.max(1, Math.min(limit, items.length || 1)) }, () =>
      runNext(),
    ),
  )
  return results
}

async function loadOverviewMetrics(options: { silent?: boolean } = {}) {
  if (!options.silent) {
    loadingOverviewMetrics.value = true
  }
  try {
    const applications = await listAllApplicationsForOverview()
    overviewApplications.value = applications

    const applicationIDs = applications
      .map((item) => String(item.id || '').trim())
      .filter(Boolean)
    if (applicationIDs.length === 0) {
      overviewRecentReleaseOrders.value = []
      return
    }

    const results = await mapSettledWithConcurrency(
      applicationIDs,
      8,
      (applicationID) =>
        listReleaseOrders(
          {
            application_id: applicationID,
            page: 1,
            page_size: 100,
          },
          {
            timeout: 30_000,
          },
        ),
    )

    const merged: ReleaseOrder[] = []
    let successCount = 0
    results.forEach((result) => {
      if (result.status !== 'fulfilled') {
        return
      }
      successCount += 1
      merged.push(...result.value.data)
    })

    if (successCount === 0) {
      throw new Error('overview recent release requests all failed')
    }

    overviewRecentReleaseOrders.value = merged.sort(
      (left, right) => dayjs(right.created_at).valueOf() - dayjs(left.created_at).valueOf(),
    )
  } catch (error) {
    overviewApplications.value = []
    overviewRecentReleaseOrders.value = []
    if (!options.silent && !isHTTPStatus(error, 403)) {
      message.error(extractHTTPErrorMessage(error, '状态卡片汇总加载失败'))
    }
  } finally {
    if (!options.silent) {
      loadingOverviewMetrics.value = false
    }
  }
}

function overviewStatCardClass(tone: MetricTone) {
  switch (tone) {
    case 'success':
      return 'overview-summary-card-success'
    case 'running':
      return 'overview-summary-card-running'
    case 'warning':
      return 'overview-summary-card-warning'
    case 'danger':
      return 'overview-summary-card-danger'
    default:
      return 'overview-summary-card-default'
  }
}

function renderOverviewTrendChart() {
  if (!overviewTrendChartRef.value) {
    return
  }
  if (!overviewTrendChart) {
    overviewTrendChart = echarts.init(overviewTrendChartRef.value)
  }
  const model = overviewTrendModel.value
  overviewTrendChart.setOption(
    {
      animationDuration: 420,
      animationEasing: 'cubicOut',
      grid: {
        top: 28,
        right: 14,
        bottom: 18,
        left: 14,
        containLabel: true,
      },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(2, 6, 23, 0.92)',
        borderColor: 'rgba(148, 163, 184, 0.2)',
        borderWidth: 1,
        padding: [10, 12],
        textStyle: {
          color: '#e2e8f0',
          fontSize: 12,
        },
        axisPointer: {
          type: 'line',
          lineStyle: {
            color: 'rgba(148, 163, 184, 0.28)',
          },
        },
      },
      legend: {
        top: 0,
        right: 0,
        icon: 'circle',
        itemWidth: 8,
        itemHeight: 8,
        textStyle: {
          color: 'rgba(226, 232, 240, 0.7)',
          fontSize: 12,
          fontWeight: 600,
        },
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: model.labels,
        axisLabel: {
          color: 'rgba(226, 232, 240, 0.56)',
          fontSize: 11,
          interval: 2,
        },
        axisLine: {
          lineStyle: {
            color: 'rgba(71, 85, 105, 0.32)',
          },
        },
        axisTick: {
          show: false,
        },
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        splitNumber: Math.min(4, model.maxValue),
        axisLabel: {
          color: 'rgba(226, 232, 240, 0.52)',
          fontSize: 11,
        },
        axisLine: {
          show: false,
        },
        axisTick: {
          show: false,
        },
        splitLine: {
          lineStyle: {
            color: 'rgba(71, 85, 105, 0.22)',
          },
        },
      },
      series: [
        {
          name: '成功',
          type: 'line',
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2.5, color: '#34d399' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(52, 211, 153, 0.26)' },
              { offset: 1, color: 'rgba(52, 211, 153, 0.02)' },
            ]),
          },
          data: model.success,
        },
        {
          name: '执行',
          type: 'line',
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2.5, color: '#60a5fa' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(96, 165, 250, 0.22)' },
              { offset: 1, color: 'rgba(96, 165, 250, 0.02)' },
            ]),
          },
          data: model.running,
        },
        {
          name: '失败',
          type: 'line',
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2.5, color: '#f87171' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(248, 113, 113, 0.22)' },
              { offset: 1, color: 'rgba(248, 113, 113, 0.02)' },
            ]),
          },
          data: model.failed,
        },
      ],
    },
    true,
  )
  overviewTrendChart.resize()
}

function disposeOverviewTrendChart() {
  overviewTrendChart?.dispose()
  overviewTrendChart = null
}

function handleOverviewCardClick(card: OverviewStatCard) {
  if (!card.clickable) {
    return
  }
  if (card.key === 'pending') {
    toApprovalWorkbench()
    return
  }
  if (card.key === 'running') {
    void router.push({
      path: '/releases',
      query: {
        status: 'deploying',
      },
    })
    return
  }
  if (card.key === 'failed') {
    const dateText = dayjs().format('YYYY-MM-DD')
    void router.push({
      path: '/releases',
      query: {
        status: 'deploy_failed',
        created_at_from: dateText,
        created_at_to: dateText,
      },
    })
  }
}

function canReleaseApplication(applicationID: string) {
  return (
    authStore.hasApplicationPermission('release.create', applicationID) &&
    templateApplicationIDs.value.has(String(applicationID || '').trim()) &&
    !loadingTemplateAvailability.value
  )
}

function rollbackActionKey(applicationID: string, envCode: string) {
  return `${String(applicationID || '').trim()}::${String(envCode || '').trim()}`
}

function isRollbackActionLoading(applicationID: string, envCode: string) {
  return rollbackActionLoadingKey.value === rollbackActionKey(applicationID, envCode)
}

function rollbackActionLabelForEnv(applicationID: string, envCode: string) {
  const key = rollbackActionKey(applicationID, envCode)
  const capability = rollbackCapabilityHints.value.get(key)
  if (!capability) {
    return '标准回滚'
  }
  if (capability.supported_action === 'replay') {
    return '标准重放'
  }
  const currentState = capability.current_state
  const provider = String(currentState?.cd_provider || '').trim().toLowerCase()
  if (currentState?.has_ci_execution && !currentState?.has_cd_execution) {
    return '标准重放'
  }
  if (currentState?.has_cd_execution && provider !== '' && provider !== 'argocd') {
    return '标准重放'
  }
  return '标准回滚'
}

function orderedEnvCodes(items: string[]) {
  const preferred = ['dev', 'test', 'prod']
  return [...items]
    .filter(Boolean)
    .sort((left, right) => {
      const leftIndex = preferred.indexOf(left)
      const rightIndex = preferred.indexOf(right)
      if (leftIndex >= 0 || rightIndex >= 0) {
        if (leftIndex < 0) {
          return 1
        }
        if (rightIndex < 0) {
          return -1
        }
        if (leftIndex !== rightIndex) {
          return leftIndex - rightIndex
        }
      }
      return left.localeCompare(right)
    })
}

async function loadActiveAnnouncements() {
  try {
    const items = await listActiveAnnouncements()
    announcements.value = Array.isArray(items) ? items : []
    console.log('announcements loaded:', announcements.value.length, 'hasUnread:', hasUnreadAnnouncement.value, 'unreadCount:', unreadCount.value)
  } catch (err) {
    console.error('loadActiveAnnouncements failed:', err)
  }
}

async function loadApplications(options: { silent?: boolean } = {}) {
  if (!options.silent) {
    loading.value = true
  }
  try {
    const response = await listApplications(filters.value)
    dataSource.value = response.data
    total.value = response.total
    listStore.setPage(response.page, response.page_size)
    scheduleMeasureAppKeyOverflow()
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, '应用列表加载失败'))
    }
  } finally {
    if (!options.silent) {
      loading.value = false
    }
  }
}

async function loadProjectOptions() {
  loadingProjects.value = true
  try {
    const response = await listProjects({ page: 1, page_size: 200, status: 'active' })
    const projects = response.data || []
    projectOptions.value = projects.map((item: Project) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))
    const current = String(listStore.project_id || '').trim()
    const hasCurrent = current && projectOptions.value.some((item) => item.value === current)
    if (current && !hasCurrent) {
      listStore.project_id = ''
      listStore.setPage(1, listStore.pageSize)
    }
  } catch (error) {
    projectOptions.value = []
    message.error(extractHTTPErrorMessage(error, '项目列表加载失败'))
  } finally {
    loadingProjects.value = false
  }
}

async function loadTemplateAvailability(options: { silent?: boolean } = {}) {
  if (!options.silent) {
    loadingTemplateAvailability.value = true
  }
  try {
    const items = await listAllReleaseTemplates({ status: 'active' })
    const grouped = new Map<string, string[]>()
    items.forEach((item) => {
      const appID = String(item.application_id || '').trim()
      if (!appID) {
        return
      }
      const list = grouped.get(appID) || []
      const templateName = String(item.name || '').trim()
      if (templateName && !list.includes(templateName)) {
        list.push(templateName)
      }
      grouped.set(appID, list)
    })
    templateNamesByApplication.value = grouped
    templateApplicationIDs.value = new Set([...grouped.keys()])
  } catch (error) {
    templateNamesByApplication.value = new Map()
    templateApplicationIDs.value = new Set()
    if (!options.silent && !isHTTPStatus(error, 403)) {
      message.error(extractHTTPErrorMessage(error, '发布模板状态加载失败'))
    }
  } finally {
    if (!options.silent) {
      loadingTemplateAvailability.value = false
    }
  }
}

async function loadRecentReleases(options: { silent?: boolean } = {}) {
  if (!options.silent) {
    loadingRecentReleases.value = true
  }
  try {
    const applicationIDs = dataSource.value.map((item) => String(item.id || '').trim()).filter(Boolean)
    if (applicationIDs.length === 0) {
      recentReleaseOrders.value = []
      return
    }

    const results = await Promise.allSettled(
      applicationIDs.map((applicationID) =>
        listReleaseOrders(
          {
            application_id: applicationID,
            page: 1,
            page_size: 12,
          },
          {
            timeout: 30_000,
          },
        ),
      ),
    )

    const merged: ReleaseOrder[] = []
    let successCount = 0
    results.forEach((result) => {
      if (result.status !== 'fulfilled') {
        return
      }
      successCount += 1
      merged.push(...result.value.data)
    })

    if (successCount === 0) {
      throw new Error('recent release requests all failed')
    }

    recentReleaseOrders.value = merged.sort((left, right) =>
      dayjs(right.created_at).valueOf() - dayjs(left.created_at).valueOf(),
    )
  } catch (error) {
    if (!options.silent && !isHTTPStatus(error, 403) && recentReleaseOrders.value.length === 0) {
      message.error(extractHTTPErrorMessage(error, '最近发布动态加载失败'))
    }
  } finally {
    if (!options.silent) {
      loadingRecentReleases.value = false
    }
  }
}

async function loadReleaseStateSummaries(options: { silent?: boolean } = {}) {
  if (!options.silent) {
    loadingReleaseStateSummaries.value = true
  }
  try {
    const applicationIDs = dataSource.value.map((item) => String(item.id || '').trim()).filter(Boolean)
    if (applicationIDs.length === 0) {
      releaseStateSummaries.value = []
      return
    }
    const response = await listAppReleaseStateSummaries(applicationIDs)
    releaseStateSummaries.value = response.data || []
    await loadRollbackCapabilityHints()
  } catch (error) {
    releaseStateSummaries.value = []
    rollbackCapabilityHints.value = new Map()
    if (!options.silent && !isHTTPStatus(error, 403)) {
      message.error(extractHTTPErrorMessage(error, '当前/上次版本加载失败'))
    }
  } finally {
    if (!options.silent) {
      loadingReleaseStateSummaries.value = false
    }
  }
}

async function loadRollbackCapabilityHints() {
  const entryMap = new Map<string, { applicationID: string; envCode: string }>()
  workbenchCards.value.forEach((card) => {
    const applicationID = String(card.application.id || '').trim()
    card.envSections.forEach((section) => {
      const envCode = String(section.envCode || '').trim()
      if (!applicationID || !envCode) {
        return
      }
      entryMap.set(rollbackActionKey(applicationID, envCode), {
        applicationID,
        envCode,
      })
    })
  })
  const entries = [...entryMap.values()]

  if (entries.length === 0) {
    rollbackCapabilityHints.value = new Map()
    return
  }

  const next = new Map<string, ApplicationRollbackCapability>()
  const results = await Promise.allSettled(
    entries.map(async (item) => {
      const response = await getApplicationRollbackCapability(item.applicationID, {
        env_code: item.envCode,
      })
      return {
        key: rollbackActionKey(item.applicationID, item.envCode),
        capability: response.data,
      }
    }),
  )

  results.forEach((result) => {
    if (result.status !== 'fulfilled') {
      return
    }
    next.set(result.value.key, result.value.capability)
  })
  rollbackCapabilityHints.value = next
}

async function applyWorkbenchFilters() {
  await loadApplications()
  await Promise.all([loadOverviewMetrics(), loadRecentReleases(), loadReleaseStateSummaries()])
}

function openSearchDialog() {
  searchDraft.keyword = String(listStore.keyword || '').trim()
  searchDialogVisible.value = true
  void nextTick(() => {
    searchInputRef.value?.focus?.()
  })
}

function closeSearchDialog() {
  searchDialogVisible.value = false
}

function resetSearchDraft() {
  searchDraft.keyword = ''
}

function resetSearchSuggestions() {
  if (searchSuggestionTimer) {
    window.clearTimeout(searchSuggestionTimer)
    searchSuggestionTimer = null
  }
  searchSuggestionRequestSeq += 1
  searchSuggestions.value = []
  searchSuggestionsLoading.value = false
}

async function loadSearchSuggestions(keyword: string) {
  const requestSeq = ++searchSuggestionRequestSeq
  searchSuggestionsLoading.value = true
  try {
    const response = await listApplications({
      keyword,
      project_id: String(listStore.project_id || '').trim() || undefined,
      status: (String(listStore.status || '').trim() as ApplicationStatus | '') || undefined,
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: String(item.id || '').trim(),
      title: String(item.name || '').trim(),
      subtitle: String(item.key || '').trim(),
      query: String(item.key || item.name || '').trim(),
    }))
  } catch {
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = []
  } finally {
    if (requestSeq === searchSuggestionRequestSeq) {
      searchSuggestionsLoading.value = false
    }
  }
}

function handleSearchSubmit() {
  listStore.keyword = String(searchDraft.keyword || '').trim()
  listStore.setPage(1, listStore.pageSize)
  searchDialogVisible.value = false
  void applyWorkbenchFilters()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  handleSearchSubmit()
}

function handleProjectChange() {
  listStore.setPage(1, listStore.pageSize)
  void applyWorkbenchFilters()
}

function handleReset() {
  listStore.resetFilters()
  resetSearchDraft()
  searchDialogVisible.value = false
  void applyWorkbenchFilters()
}

function handlePageChange(page: number, pageSize: number) {
  listStore.setPage(page, pageSize)
  void (async () => {
    await loadApplications()
    await Promise.all([loadRecentReleases(), loadReleaseStateSummaries()])
  })()
}

function openIntroDrawer() {
  introVisible.value = true
}

function closeIntroDrawer() {
  introVisible.value = false
}

function toCreate() {
  void router.push('/applications/new')
}

function toEdit(id: string) {
  void router.push(`/applications/${id}/edit`)
}

function toBindings(id: string) {
  void router.push(`/applications/${id}/pipeline-bindings`)
}

function toRelease(id: string) {
  if (!canReleaseApplication(id)) {
    message.warning('当前应用未绑定发布模板或无发布权限，请先完成应用配置')
    return
  }
  void router.push({
    path: '/releases/new',
    query: { application_id: id },
  })
}

function toReleaseRecords(id: string) {
  void router.push({
    path: '/releases',
    query: { application_id: id },
  })
}

function toTemplates(id: string) {
  void router.push({
    path: '/release-templates',
    query: { application_id: id },
  })
}

function toReleaseOrderDetail(id: string | null | undefined) {
  const releaseOrderID = String(id || '').trim()
  if (!releaseOrderID) {
    return
  }
  void router.push(`/releases/${releaseOrderID}`)
}

function toApprovalWorkbench() {
  void router.push('/release-approvals')
}

async function refreshWorkbenchSilently() {
  await loadApplications({ silent: true })
  await Promise.all([
    loadTemplateAvailability({ silent: true }),
    loadRecentReleases({ silent: true }),
    loadReleaseStateSummaries({ silent: true }),
  ])
}

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshTimer = window.setInterval(() => {
    void refreshWorkbenchSilently()
  }, 30000)
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

watch(
  [
    () => searchDialogVisible.value,
    () => String(searchDraft.keyword || '').trim(),
    () => String(listStore.project_id || '').trim(),
    () => String(listStore.status || '').trim(),
  ],
  ([visible, keyword]) => {
    if (searchSuggestionTimer) {
      window.clearTimeout(searchSuggestionTimer)
      searchSuggestionTimer = null
    }
    if (!visible || !keyword) {
      resetSearchSuggestions()
      return
    }
    searchSuggestionsLoading.value = true
    searchSuggestionTimer = window.setTimeout(() => {
      void loadSearchSuggestions(keyword)
    }, 180)
  },
)

watch(
  [() => initialWorkbenchLoading.value, () => overviewTrendModel.value],
  ([loading]) => {
    if (loading) {
      disposeOverviewTrendChart()
      return
    }
    void nextTick(() => {
      renderOverviewTrendChart()
    })
  },
  { deep: true },
)

function handleResize() {
  viewportWidth.value = window.innerWidth
  scheduleMeasureAppKeyOverflow()
  overviewTrendChart?.resize()
}

async function handleDelete(id: string) {
  deletingId.value = id
  try {
    await deleteApplication(id)
    message.success('应用删除成功')
    if (dataSource.value.length === 1 && listStore.page > 1) {
      listStore.setPage(listStore.page - 1, listStore.pageSize)
    }
    await loadApplications()
    await Promise.all([loadOverviewMetrics(), loadRecentReleases(), loadReleaseStateSummaries()])
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用删除失败'))
  } finally {
    deletingId.value = ''
  }
}

function confirmDeleteApplication(id: string) {
  Modal.confirm({
    title: '确认删除应用',
    icon: h(ExclamationCircleOutlined),
    content: '删除后不可恢复，相关发布记录不会自动清理',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    onOk: () => handleDelete(id),
  })
}

async function createRollbackOrderForCard(
  card: WorkbenchCard,
  envCode: string,
  action: Exclude<RollbackSupportedAction, 'unsupported'>,
) {
  const applicationID = String(card.application.id || '').trim()
  const actionKey = rollbackActionKey(applicationID, envCode)
  rollbackActionLoadingKey.value = actionKey
  try {
    const response = await createApplicationRollbackOrder(applicationID, {
      env_code: envCode,
      action,
    })
    message.success(`${action === 'rollback' ? '标准回滚' : '标准重放'}单已创建：${response.data.order_no}`)
    await Promise.all([
      loadOverviewMetrics({ silent: true }),
      loadRecentReleases({ silent: true }),
      loadReleaseStateSummaries({ silent: true }),
    ])
    void router.push(`/releases/${response.data.id}`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, action === 'rollback' ? '标准回滚创建失败' : '标准重放创建失败'))
  } finally {
    rollbackActionLoadingKey.value = ''
  }
}

function rollbackActionText(action: Exclude<RollbackSupportedAction, 'unsupported'>) {
  return action === 'rollback' ? '标准回滚' : '标准重放'
}

function rollbackPreviewScopeText(scope: string) {
  return String(scope || '').trim().toLowerCase() === 'cd' ? 'CD 参数' : 'CI 参数'
}

function rollbackPrecheckStatusText(status: string) {
  switch (String(status || '').trim().toLowerCase()) {
    case 'pass':
      return '通过'
    case 'warn':
      return '提示'
    case 'blocked':
      return '阻塞'
    default:
      return '未知'
  }
}

function rollbackPrecheckStatusClass(status: string) {
  switch (String(status || '').trim().toLowerCase()) {
    case 'pass':
      return 'rollback-preview-check-tag rollback-preview-check-tag-pass'
    case 'warn':
      return 'rollback-preview-check-tag rollback-preview-check-tag-warn'
    case 'blocked':
      return 'rollback-preview-check-tag rollback-preview-check-tag-blocked'
    default:
      return 'rollback-preview-check-tag'
  }
}

function rollbackPreviewValueText(value: string) {
  const text = String(value || '').trim()
  if (!text) {
    return '空值'
  }
  return text.length > 64 ? `${text.slice(0, 61)}...` : text
}

function buildRollbackPreviewContent(
  precheck: ApplicationRollbackPrecheck,
) {
  const action: Exclude<RollbackSupportedAction, 'unsupported'> =
    precheck.action === 'replay' ? 'replay' : 'rollback'
  const params = precheck.params || []
  const grouped = new Map<string, ApplicationRollbackPrecheckParam[]>()
  params.forEach((item) => {
    const scope = String(item.pipeline_scope || 'ci').trim().toLowerCase() || 'ci'
    const list = grouped.get(scope) || []
    list.push(item)
    grouped.set(scope, list)
  })
  const scopeOrder = ['ci', 'cd']
  const scopeKeys = [...grouped.keys()].sort((left, right) => {
    const leftIndex = scopeOrder.indexOf(left)
    const rightIndex = scopeOrder.indexOf(right)
    if (leftIndex >= 0 || rightIndex >= 0) {
      if (leftIndex < 0) {
        return 1
      }
      if (rightIndex < 0) {
        return -1
      }
      return leftIndex - rightIndex
    }
    return left.localeCompare(right)
  })

  return h('div', { class: 'rollback-preview-modal' }, [
    h('div', { class: 'rollback-preview-hero' }, [
      h('div', { class: 'rollback-preview-hero-copy' }, [
        h('div', { class: 'rollback-preview-hero-title' }, `${rollbackActionText(action)}预审通过后将创建新单`),
        h(
          'div',
          { class: 'rollback-preview-hero-desc' },
          action === 'rollback'
            ? '将按目标历史版本的 Helm 部署快照恢复 GitOps 配置，并继续执行 CD'
            : '当前环境仅支持标准重放，将按目标历史版本的参数快照重新创建执行单',
        ),
      ]),
      h('span', { class: `rollback-preview-action-badge rollback-preview-action-badge-${action}` }, rollbackActionText(action)),
    ]),
    h('div', { class: 'rollback-preview-summary' }, [
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '动作'),
        h('span', { class: 'rollback-preview-summary-value' }, rollbackActionText(action)),
      ]),
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '环境'),
        h('span', { class: 'rollback-preview-summary-value' }, precheck.env_code || '-'),
      ]),
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '当前版本'),
        h(
          'span',
          { class: 'rollback-preview-summary-value' },
          precheck.current_state.release_order_no || '未确认生效',
        ),
      ]),
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '目标版本'),
        h(
          'span',
          { class: 'rollback-preview-summary-value' },
          precheck.target_state.release_order_no || '无可回退目标',
        ),
      ]),
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '模板'),
        h(
          'span',
          { class: 'rollback-preview-summary-value' },
          precheck.template_name || precheck.target_state.template_name || '-',
        ),
      ]),
      h('div', { class: 'rollback-preview-summary-item' }, [
        h('span', { class: 'rollback-preview-summary-label' }, '执行范围'),
        h(
          'span',
          { class: 'rollback-preview-summary-value' },
          precheck.preview_scope ? rollbackPreviewScopeText(precheck.preview_scope) : '-',
        ),
      ]),
    ]),
    precheck.reason
      ? h('div', { class: 'rollback-preview-reason' }, precheck.reason)
      : null,
    h(
      'div',
      { class: 'rollback-preview-checks' },
      (precheck.items || []).map((item) =>
        h('div', { class: 'rollback-preview-check-item', key: item.key }, [
          h('span', { class: rollbackPrecheckStatusClass(item.status) }, rollbackPrecheckStatusText(item.status)),
          h('div', { class: 'rollback-preview-check-copy' }, [
            h('div', { class: 'rollback-preview-check-title' }, item.name || item.key || '预审项'),
            h('div', { class: 'rollback-preview-check-message' }, item.message || '-'),
          ]),
        ]),
      ),
    ),
    ...scopeKeys.map((scope) =>
      h('div', { class: 'rollback-preview-scope', key: scope }, [
        h('div', { class: 'rollback-preview-scope-title' }, rollbackPreviewScopeText(scope)),
        h(
          'div',
          { class: 'rollback-preview-param-list' },
          (grouped.get(scope) || []).map((item, index) =>
            h('div', { class: 'rollback-preview-param-item', key: `${scope}-${item.param_key}-${item.executor_param_name}-${index}` }, [
              h('span', { class: 'rollback-preview-param-key' }, item.param_key || item.executor_param_name || '未命名参数'),
              h('span', { class: 'rollback-preview-param-value' }, rollbackPreviewValueText(item.param_value)),
            ]),
          ),
        ),
      ]),
    ),
    params.length === 0
      ? h('div', { class: 'rollback-preview-empty' }, '来源版本没有可展示的参数快照')
      : null,
  ])
}

async function openRollbackPreviewModal(
  card: WorkbenchCard,
  envCode: string,
  action: Exclude<RollbackSupportedAction, 'unsupported'>,
) {
  const applicationID = String(card.application.id || '').trim()
  const precheckResponse = await getApplicationRollbackPrecheck(applicationID, {
    env_code: envCode,
    action,
  })
  const precheck = precheckResponse.data
  if (!precheck.executable) {
    message.warning(precheck.conflict_message || precheck.reason || '当前环境暂不支持创建恢复单')
    return
  }
  Modal.confirm({
    title: action === 'rollback' ? '标准回滚预审' : '标准重放预审',
    width: 820,
    okText: `确认创建${rollbackActionText(action)}单`,
    cancelText: '取消',
    wrapClassName: 'rollback-preview-confirm-modal',
    icon: null,
    content: buildRollbackPreviewContent(precheck),
    onOk: () => createRollbackOrderForCard(card, envCode, action),
  })
}

async function handleStandardRollback(card: WorkbenchCard, envCode: string) {
  const applicationID = String(card.application.id || '').trim()
  if (!applicationID || !envCode) {
    message.warning('当前应用缺少可识别的环境，无法验证回滚支持')
    return
  }
  const actionKey = rollbackActionKey(applicationID, envCode)
  rollbackActionLoadingKey.value = actionKey
  try {
    const response = await getApplicationRollbackCapability(applicationID, {
      env_code: envCode,
    })
    const capability = response.data
    if (capability.supported_action === 'unsupported') {
      message.warning(capability.reason || '当前环境暂不支持标准回滚')
      return
    }
    if (capability.supported_action === 'replay') {
      Modal.confirm({
        title: '仅支持标准重放',
        content: capability.reason || '当前环境不支持标准回滚，仅支持标准重放，是否继续？',
        okText: '继续预审',
        cancelText: '取消',
        icon: h(ExclamationCircleOutlined),
        onOk: () => openRollbackPreviewModal(card, envCode, 'replay'),
      })
      return
    }
    await openRollbackPreviewModal(card, envCode, 'rollback')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '正在验证回滚支持失败'))
  } finally {
    if (rollbackActionLoadingKey.value === actionKey) {
      rollbackActionLoadingKey.value = ''
    }
  }
}

function applicationStatusText(status: Application['status']) {
  return status === 'active' ? '启用中' : '已停用'
}

function applicationStatusClass(status: Application['status']) {
  return status === 'active' ? 'app-state-chip-active' : 'app-state-chip-inactive'
}

function formatDateTime(value: string) {
  if (!value) {
    return '-'
  }
  const date = dayjs(value)
  return date.isValid() ? date.format('YYYY-MM-DD HH:mm:ss') : '-'
}

function releaseStatusTone(status: ReleaseOrderBusinessStatus) {
  switch (status) {
    case 'deploy_success':
      return 'success'
    case 'deploying':
      return 'running'
    case 'deploy_failed':
    case 'rejected':
    case 'cancelled':
      return 'failed'
    default:
      return 'pending'
  }
}

function releaseStatusText(status: ReleaseOrderBusinessStatus) {
  switch (status) {
    case 'deploy_success':
      return '发布成功'
    case 'deploying':
      return '发布中'
    case 'deploy_failed':
      return '发布失败'
    case 'queued':
      return '排队中'
    case 'pending_execution':
      return '待执行'
    case 'approved':
      return '已批准'
    case 'pending_approval':
      return '待审批'
    case 'approving':
      return '审批中'
    case 'rejected':
      return '审批拒绝'
    case 'draft':
      return '草稿'
    case 'cancelled':
      return '已取消'
    default:
      return '待执行'
  }
}

function releaseStatusClass(status: ReleaseOrderBusinessStatus) {
  switch (status) {
    case 'deploy_success':
      return 'env-status-success'
    case 'deploying':
      return 'env-status-running'
    case 'deploy_failed':
      return 'env-status-failed'
    case 'pending_execution':
    case 'queued':
    case 'pending_approval':
    case 'approving':
      return 'env-status-pending'
    case 'cancelled':
      return 'env-status-neutral'
    default:
      return 'env-status-pending'
  }
}

function operationTypeText(operationType: ReleaseOperationType | '' | null | undefined) {
  switch (String(operationType || '').trim().toLowerCase()) {
    case 'rollback':
      return '标准回滚'
    case 'replay':
      return '标准重放'
    default:
      return '普通发布'
  }
}

function operationTypeClass(operationType: ReleaseOperationType | '' | null | undefined) {
  switch (String(operationType || '').trim().toLowerCase()) {
    case 'rollback':
      return 'dashboard-chip dashboard-chip-danger'
    case 'replay':
      return 'dashboard-chip dashboard-chip-warning'
    default:
      return 'dashboard-chip dashboard-chip-neutral'
  }
}

function formatTime(value: string | null | undefined) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function templateSummary(names: string[]) {
  if (names.length === 0) {
    return '未配置可用模板'
  }
  if (names.length === 1) {
    return names[0]
  }
  return `${names[0]} 等 ${names.length} 个模板`
}

function handleGlobalKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && searchDialogVisible.value) {
    event.preventDefault()
    closeSearchDialog()
    return
  }
  if (event.key === 'Escape' && applicationInfoDrawerVisible.value) {
    event.preventDefault()
    closeApplicationInfoDrawer()
    return
  }
  if (event.key === 'Escape' && releaseDetailCardID.value) {
    event.preventDefault()
    closeReleaseDetailCard()
  }
}

onMounted(() => {
  try {
    const raw = localStorage.getItem('gos-seen-announcement-ids')
    if (raw) lastSeenAnnouncementIds.value = JSON.parse(raw)
  } catch { /* ignore */ }
  loadActiveAnnouncements()
  void (async () => {
    await loadProjectOptions()
    await loadApplications()
    await Promise.all([loadTemplateAvailability(), loadOverviewMetrics(), loadRecentReleases(), loadReleaseStateSummaries()])
    firstWorkbenchLoaded.value = true
    scheduleMeasureAppKeyOverflow()
  })()
  window.addEventListener('resize', handleResize)
  window.addEventListener('keydown', handleGlobalKeydown)
  startAutoRefresh()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('keydown', handleGlobalKeydown)
  stopAutoRefresh()
  resetSearchSuggestions()
  disposeOverviewTrendChart()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header application-page-header">
      <div class="page-header-copy">
        <span class="page-title-wrap">
          <h2 class="page-title">应用</h2>
          <span v-if="hasUnreadAnnouncement" class="announcement-badge" @click.stop="announcementPopoverVisible = true">{{ unreadCount }}</span>
        </span>
        <a-popover
          v-model:open="announcementPopoverVisible"
          placement="bottomLeft"
          trigger="click"
          overlay-class-name="announcement-popover"
        >
          <template #content>
            <div class="announcement-panel">
              <div class="announcement-panel-head">
                <SoundOutlined class="announcement-head-icon" />
                <span class="announcement-head-title">系统公告</span>
              </div>
              <div class="announcement-list">
                <div
                  v-for="item in announcements"
                  :key="item.id"
                  class="announcement-item"
                >
                  <div class="announcement-item-title">{{ item.title }}</div>
                  <div v-if="item.content" class="announcement-item-content">{{ item.content }}</div>
                </div>
                <div v-if="announcements.length === 0" class="announcement-empty">暂无公告</div>
              </div>
              <div v-if="hasUnreadAnnouncement" class="announcement-panel-foot">
                <a-button
                  type="link"
                  size="small"
                  class="announcement-read-btn"
                  @click="markAsRead"
                >标记已读</a-button>
              </div>
            </div>
          </template>
        </a-popover>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-select
          v-model:value="projectFilterValue"
          class="application-toolbar-project-select"
          allow-clear
          option-filter-prop="label"
          placeholder="项目"
          :loading="loadingProjects"
          :options="projectOptions"
          @change="handleProjectChange"
        />
        <a-button class="application-toolbar-action-btn" @click="openIntroDrawer">
          <template #icon>
            <QuestionCircleOutlined />
          </template>
          发布流程介绍
        </a-button>
        <a-button v-if="canManageApplication" class="application-toolbar-action-btn" @click="toCreate">
          <template #icon>
            <PlusOutlined />
          </template>
          新增应用
        </a-button>
      </div>
    </div>

    <transition name="application-search-fade">
      <div v-if="searchDialogVisible" class="application-search-overlay" @click.self="closeSearchDialog">
        <div class="application-search-floating-panel">
          <div class="application-search-floating-input">
            <SearchOutlined class="application-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="application-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="名称或 Key"
              @keydown.enter="handleSearchSubmit"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="application-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="application-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="application-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="application-search-suggestion-title">{{ item.title }}</span>
                <span v-if="item.subtitle" class="application-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-card class="application-overview-card" :bordered="false">
      <div v-if="initialWorkbenchLoading" class="overview-loading-state">
        <div class="overview-loading-header">
          <SyncOutlined spin class="overview-loading-icon" />
          <div>
            <div class="overview-loading-title">正在汇总应用视图</div>
            <div class="overview-loading-text">正在加载应用、模板与最近发布记录，页面准备好后会自动展示工作台</div>
          </div>
        </div>
        <div class="overview-loading-grid">
          <a-skeleton-button v-for="index in 4" :key="`metric-${index}`" active block class="overview-loading-metric" />
        </div>
      </div>
      <div v-else class="overview-layout">
        <section class="overview-chart-panel">
          <div class="overview-chart-header">
            <div>
              <div class="overview-chart-label">发布态势</div>
              <div class="overview-chart-title">近 24 小时执行概览</div>
            </div>
            <div class="overview-chart-meta">{{ overviewHeaderMeta }}</div>
          </div>
          <div ref="overviewTrendChartRef" class="overview-chart-canvas"></div>
          <div class="overview-chart-footnote">统计口径：按发布单最近状态更新时间汇总</div>
        </section>
        <div class="overview-summary-grid">
          <button
            v-for="item in overviewSummaryCards"
            :key="item.key"
            class="overview-summary-card"
            :class="[overviewStatCardClass(item.tone), item.clickable ? 'overview-summary-card-clickable' : '']"
            type="button"
            @click="handleOverviewCardClick(item)"
          >
            <div class="overview-summary-head">
              <span class="overview-summary-icon">
                <component :is="item.icon" />
              </span>
              <span v-if="item.clickable" class="overview-summary-badge">查看</span>
            </div>
            <div class="overview-summary-label">{{ item.label }}</div>
            <div class="overview-summary-value">{{ item.value }}</div>
            <div class="overview-summary-hint">{{ item.hint }}</div>
          </button>
        </div>
      </div>
    </a-card>

    <div v-if="initialWorkbenchLoading" class="workbench-loading-state">
      <div class="workbench-loading-copy">
        <div class="workbench-loading-title">应用工作台加载中</div>
        <div class="workbench-loading-text">我们正在整理环境状态、最近发布和模板可用性，请稍等片刻</div>
      </div>
      <div class="workbench-skeleton-grid">
        <a-skeleton v-for="index in 6" :key="index" active :paragraph="{ rows: 6 }" class="workbench-skeleton-card" />
      </div>
    </div>

    <div
      v-else-if="workbenchCards.length > 0"
      class="application-workbench-columns"
      :class="`application-workbench-columns-${workbenchColumnCount}`"
    >
      <a-card
        v-for="card in workbenchCards"
        :key="card.application.id"
        class="application-workbench-card"
        :class="[
          'application-workbench-card-collapsed',
          card.runningCount > 0 ? 'application-workbench-card-running' : '',
          isReleaseDetailCard(card.application.id) ? 'application-workbench-card-release-active' : '',
        ]"
        :bordered="false"
      >
        <div class="workbench-card-shell" :class="{ 'workbench-card-shell-running': card.runningCount > 0 }">
          <transition name="workbench-card-detail-switch" mode="out-in">
            <div
              v-if="!isReleaseDetailCard(card.application.id)"
              :key="`${card.application.id}-summary`"
              class="workbench-card-view workbench-card-summary-view"
            >
              <div class="workbench-card-header-shell">
                <div class="workbench-card-header">
                  <div class="workbench-card-header-copy">
                    <div class="workbench-app-eyebrow">
                      <span class="workbench-app-project">
                        {{ card.application.project_name || '未配置' }}
                      </span>
                      <span class="workbench-app-owner-inline">{{ card.application.owner || '未配置' }}</span>
                    </div>
                    <div class="workbench-card-title-row">
                      <button
                        class="workbench-app-title"
                        type="button"
                        :ref="(el) => setAppTitleElement(card.application.id, el)"
                        @click="openApplicationInfoDrawer(card.application)"
                      >
                        {{ card.application.name }}
                      </button>
                      <span
                        class="workbench-app-key"
                        :class="{ 'workbench-app-key-overflowing': shouldShowAppKeyFade(card.application.id) }"
                        :title="card.application.key"
                        :ref="(el) => setAppKeyElement(card.application.id, el)"
                      >{{ card.application.key }}</span>
                    </div>
                  </div>
                </div>
              </div>
              <div class="workbench-card-header-actions">
                <span class="workbench-app-state" :class="applicationStatusClass(card.application.status)">
                  {{ applicationStatusText(card.application.status) }}
                </span>
              </div>

              <div class="workbench-card-collapsed-summary workbench-card-release-summary">
                <div class="workbench-collapsed-latest">
                  <button
                    v-if="card.latestOrder"
                    class="workbench-collapsed-chip workbench-collapsed-chip-order"
                    type="button"
                    @click="toReleaseOrderDetail(card.latestOrder.id)"
                  >
                    {{ releaseOverviewText(card) }}
                  </button>
                  <span v-else class="workbench-collapsed-chip workbench-collapsed-chip-order workbench-collapsed-chip-muted">{{ releaseOverviewText(card) }}</span>
                </div>
                <div class="workbench-collapsed-tail">
                  <div class="workbench-collapsed-meta">
                    <span class="workbench-collapsed-chip workbench-status-chip" :class="cardActivityStatusClass(card.runningCount)">
                      <span v-if="card.runningCount > 0" class="workbench-status-chip-dot"></span>
                      {{ releaseOverviewStatusText(card) }}
                    </span>
                  </div>
                  <div class="workbench-collapsed-content">
                    <a-button class="workbench-primary-action" type="primary" @click="toRelease(card.application.id)">发布</a-button>
                    <a-button
                      class="workbench-primary-action workbench-release-detail-trigger"
                      type="primary"
                      :aria-pressed="isReleaseDetailCard(card.application.id)"
                      @click="handleReleaseDetailAction(card)"
                    >
                      <template #icon>
                        <AimOutlined />
                      </template>
                      {{ releaseDetailActionText(card) }}
                    </a-button>
                    <a-popover
                      v-if="canOpenWorkbenchConfig"
                      trigger="click"
                      placement="topRight"
                      overlay-class-name="workbench-manage-popover"
                    >
                      <template #content>
                        <div class="workbench-manage-actions">
                          <a-button class="workbench-secondary-action" @click="toTemplates(card.application.id)">查看模版</a-button>
                          <a-button
                            v-if="canViewPipeline || canManageApplication"
                            class="workbench-secondary-action"
                            @click="toBindings(card.application.id)"
                          >
                            管线绑定
                          </a-button>
                          <a-button
                            v-if="canManageApplication"
                            class="workbench-secondary-action"
                            @click="toEdit(card.application.id)"
                          >
                            编辑
                          </a-button>
                          <a-button
                            v-if="canManageApplication"
                            class="workbench-secondary-action workbench-danger-action"
                            danger
                            :style="{ color: '#ef4444', borderColor: 'rgba(248, 113, 113, 0.28)' }"
                            :loading="deletingId === card.application.id"
                            @click="confirmDeleteApplication(card.application.id)"
                          >
                            删除
                          </a-button>
                        </div>
                      </template>
                      <a-button class="workbench-manage-trigger workbench-config-trigger workbench-primary-action" type="primary">
                        <template #icon>
                          <MoreOutlined />
                        </template>
                        配置
                      </a-button>
                    </a-popover>
                    <a-button
                      v-else
                      class="workbench-manage-trigger workbench-config-trigger workbench-primary-action workbench-config-readonly"
                      type="primary"
                      @click.stop.prevent
                    >
                      <template #icon>
                        <MoreOutlined />
                      </template>
                      配置
                    </a-button>
                  </div>
                </div>
              </div>
            </div>
            <div
              v-else
              :key="`${card.application.id}-release-detail`"
              class="workbench-card-view workbench-card-release-detail"
            >
              <div class="workbench-release-detail-head">
                <span class="workbench-release-detail-env">{{ releaseDetailEnvText(card) }}</span>
                <span class="workbench-app-state" :class="applicationStatusClass(card.application.status)">
                  {{ applicationStatusText(card.application.status) }}
                </span>
              </div>
              <div class="workbench-release-state-stack">
                <button
                  v-if="releaseDetailCurrentOrderID(card)"
                  class="state-bubble state-bubble-current latest-release-order-bubble"
                  type="button"
                  @click="toReleaseOrderDetail(releaseDetailCurrentOrderID(card))"
                >
                  生效：{{ releaseDetailCurrentOrderNo(card) }}
                </button>
                <span v-else class="state-bubble state-bubble-current latest-release-order-bubble">
                  生效：未确认生效
                </span>
                <button
                  v-if="releaseDetailLatestOrderID(card)"
                  class="state-bubble state-bubble-latest latest-release-order-bubble"
                  type="button"
                  @click="toReleaseOrderDetail(releaseDetailLatestOrderID(card))"
                >
                  最近：{{ releaseDetailLatestOrderNo(card) }}
                </button>
                <span v-else class="state-bubble state-bubble-latest latest-release-order-bubble">
                  最近：暂无最近发布
                </span>
              </div>
              <div class="workbench-release-detail-actions">
                <a-button
                  v-if="canSwitchReleaseEnv(card)"
                  class="workbench-secondary-action"
                  @click="switchReleaseEnv(card)"
                >
                  切换
                </a-button>
                <a-button class="workbench-secondary-action" @click="toReleaseRecords(card.application.id)">发布单</a-button>
                <a-button class="workbench-secondary-action" @click="closeReleaseDetailCard">返回</a-button>
              </div>
            </div>
          </transition>
        </div>
      </a-card>
    </div>
    <a-card v-if="!initialWorkbenchLoading && workbenchCards.length === 0" class="table-card" :bordered="true">
      <a-empty description="当前没有符合条件的应用" />
    </a-card>

    <div class="pagination-area">
      <a-pagination
        :current="listStore.page"
        :page-size="listStore.pageSize"
        :total="total"
        :page-size-options="['6', '10', '20', '50']"
        show-size-changer
        show-quick-jumper
        :show-total="(count: number) => `共 ${count} 个应用`"
        @change="handlePageChange"
        @showSizeChange="handlePageChange"
      />
    </div>

    <a-drawer :open="applicationInfoDrawerVisible" title="应用信息" width="640" @close="closeApplicationInfoDrawer">
      <a-descriptions v-if="selectedApplication" :column="1" bordered>
        <a-descriptions-item v-for="item in baselineInfoRows(selectedApplication)" :key="item.key" :label="item.label">
          {{ item.value }}
        </a-descriptions-item>
        <a-descriptions-item label="GitOps 映射">
          <div v-if="selectedApplication.gitops_branch_mappings?.length" class="application-info-mini-table-scroll">
            <table class="application-info-mini-table application-info-mini-table-gitops">
              <thead>
                <tr>
                  <th>环境</th>
                  <th>分支</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(mapping, index) in sortedGitOpsMappings(selectedApplication)"
                  :key="`${mapping.env_code}-${mapping.branch}-${index}`"
                >
                  <td>{{ mapping.env_code || '-' }}</td>
                  <td>{{ mapping.branch || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <span v-else>当前未配置映射</span>
        </a-descriptions-item>
        <a-descriptions-item label="发布分支">
          <div v-if="selectedApplication.release_branches?.length" class="application-info-mini-table-scroll">
            <table class="application-info-mini-table application-info-mini-table-release">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>分支</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(branch, index) in selectedApplication.release_branches"
                  :key="`${branch.name}-${branch.branch}-${index}`"
                >
                  <td>{{ branch.name || '-' }}</td>
                  <td>{{ branch.branch || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <span v-else>当前未配置发布分支</span>
        </a-descriptions-item>
      </a-descriptions>
    </a-drawer>

    <a-drawer class="architecture-drawer" :open="introVisible" title="发布架构关系" width="760" @close="closeIntroDrawer">
      <div class="intro-drawer-content architecture-content">
        <section class="architecture-hero">
          <div class="architecture-kicker">Architecture</div>
          <div class="architecture-heading">GOS 用模板和发布单把应用、执行编排与交付底座连接起来</div>
          <div class="architecture-copy">
            应用定义发布对象，发布模板定义执行单元、字段映射与 Hook，发布单承载一次实际执行上下文。交付阶段既可以直接走 CD 管线，也可以通过 ArgoCD + GitOps 修改声明并同步到目标集群。
          </div>
          <div class="architecture-chip-row">
            <span class="architecture-chip">应用为中心对象</span>
            <span class="architecture-chip">模板定义执行方式</span>
            <span class="architecture-chip accent">ArgoCD 依赖 GitOps 实例</span>
          </div>
        </section>

        <section class="architecture-layer architecture-layer-entry">
          <div class="architecture-layer-head">
            <span class="architecture-layer-index">01</span>
            <div>
              <div class="architecture-layer-title">业务入口</div>
              <div class="architecture-layer-summary">围绕应用建立模板与发布单，确定谁能发、怎么发、发什么</div>
            </div>
          </div>
          <div class="architecture-node-grid architecture-node-grid-three">
            <article class="architecture-node primary">
              <div class="architecture-node-title">应用</div>
              <div class="architecture-node-desc">承载发布对象、项目归属、分支映射与环境视图</div>
              <div class="architecture-node-tags">
                <span>App</span>
                <span>分支</span>
              </div>
            </article>
            <article class="architecture-node primary">
              <div class="architecture-node-title">发布模板</div>
              <div class="architecture-node-desc">定义 CI/CD 执行单元、参数映射、审批人与 Hook 策略</div>
              <div class="architecture-node-tags">
                <span>CI/CD</span>
                <span>Hook</span>
              </div>
            </article>
            <article class="architecture-node primary">
              <div class="architecture-node-title">发布单</div>
              <div class="architecture-node-desc">落地一次执行上下文，记录参数快照、审批状态与执行进度</div>
              <div class="architecture-node-tags">
                <span>快照</span>
                <span>状态</span>
              </div>
            </article>
          </div>
        </section>

        <div class="architecture-link">模板把应用配置转换成执行上下文，并在发布单中固化</div>

        <section class="architecture-layer architecture-layer-control">
          <div class="architecture-layer-head">
            <span class="architecture-layer-index">02</span>
            <div>
              <div class="architecture-layer-title">执行编排</div>
              <div class="architecture-layer-summary">这一层负责构建参数、控制审批并发，并决定通知与 Hook 何时触发</div>
            </div>
          </div>
          <div class="architecture-node-grid architecture-node-grid-four">
            <article class="architecture-node">
              <div class="architecture-node-title">CI 执行单元</div>
              <div class="architecture-node-desc">拉代码、构建、产出镜像版本等运行期动态值</div>
            </article>
            <article class="architecture-node">
              <div class="architecture-node-title">参数映射</div>
              <div class="architecture-node-desc">把基础环境、分支与标准字段映射到 CI/CD 与 Agent 参数</div>
            </article>
            <article class="architecture-node">
              <div class="architecture-node-title">审批与并发</div>
              <div class="architecture-node-desc">控制执行入口，避免同应用同环境并发冲突</div>
            </article>
            <article class="architecture-node">
              <div class="architecture-node-title">通知与 Hook</div>
              <div class="architecture-node-desc">根据构建完成、发布完成或失败结果触发通知与脚本</div>
            </article>
          </div>
        </section>

        <div class="architecture-link accent">交付阶段支持直接 CD 或 GitOps 驱动的 ArgoCD 两条底座</div>

        <section class="architecture-layer architecture-layer-runtime">
          <div class="architecture-layer-head">
            <span class="architecture-layer-index">03</span>
            <div>
              <div class="architecture-layer-title">交付底座</div>
              <div class="architecture-layer-summary">发布上下文在这里真正落地到流水线、仓库声明与集群</div>
            </div>
          </div>
          <div class="architecture-runtime-grid">
            <article class="architecture-node architecture-runtime-direct">
              <div class="architecture-node-title">CD 管线</div>
              <div class="architecture-node-desc">直接调用绑定的 Jenkins/CD 流程，适合已有自定义部署逻辑</div>
              <div class="architecture-node-tags">
                <span>Jenkins</span>
                <span>直接部署</span>
              </div>
            </article>
            <article class="architecture-runtime-cluster">
              <div class="architecture-runtime-cluster-head">ArgoCD / GitOps 路径</div>
              <div class="architecture-runtime-chain">
                <article class="architecture-node accent">
                  <div class="architecture-node-title">ArgoCD 实例</div>
                  <div class="architecture-node-desc">按 env 命中目标实例，决定使用哪套应用入口与集群视图</div>
                </article>
                <div class="architecture-runtime-arrow">→</div>
                <article class="architecture-node accent">
                  <div class="architecture-node-title">GitOps 实例</div>
                  <div class="architecture-node-desc">提供工作目录、Git 凭据、提交身份与本地路径映射</div>
                </article>
                <div class="architecture-runtime-arrow">→</div>
                <article class="architecture-node accent">
                  <div class="architecture-node-title">Git 仓库</div>
                  <div class="architecture-node-desc">修改 values 或 YAML 并提交推送，成为 ArgoCD 的真实输入</div>
                </article>
                <div class="architecture-runtime-arrow">→</div>
                <article class="architecture-node accent">
                  <div class="architecture-node-title">目标集群</div>
                  <div class="architecture-node-desc">ArgoCD Sync 与健康检查完成最终部署与状态回传</div>
                </article>
              </div>
            </article>
          </div>
        </section>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.application-page-header {
  padding: 4px 0 0;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

:deep(.application-toolbar-icon-btn.ant-btn),
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
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  min-width: 42px;
  padding-inline: 0;
}

:deep(.application-toolbar-project-select.ant-select) {
  min-width: 156px;
  width: min(220px, 32vw);
}

:deep(.application-toolbar-project-select.ant-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  padding: 0 14px !important;
  border-radius: 16px !important;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-project-select.ant-select .ant-select-selection-wrap) {
  height: 100%;
  display: flex;
  align-items: center;
}

:deep(.application-toolbar-project-select.ant-select .ant-select-selection-placeholder),
:deep(.application-toolbar-project-select.ant-select .ant-select-arrow),
:deep(.application-toolbar-project-select.ant-select .ant-select-clear) {
  color: rgba(15, 23, 42, 0.62) !important;
}

:deep(.application-toolbar-project-select.ant-select .ant-select-selection-placeholder),
:deep(.application-toolbar-project-select.ant-select .ant-select-selection-item) {
  display: flex;
  align-items: center;
  line-height: 1 !important;
}

:deep(.application-toolbar-project-select.ant-select .ant-select-selection-item),
:deep(.application-toolbar-project-select.ant-select .ant-select-selection-search-input) {
  color: #0f172a !important;
  font-weight: 700;
}

:deep(.application-toolbar-action-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.application-toolbar-icon-btn.ant-btn:active),
:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.application-toolbar-project-select.ant-select:hover .ant-select-selector),
:deep(.application-toolbar-project-select.ant-select.ant-select-focused .ant-select-selector),
:deep(.application-toolbar-project-select.ant-select.ant-select-open .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.application-overview-card {
  border-radius: var(--radius-xl);
}

:deep(.application-overview-card.ant-card) {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
}

:deep(.application-overview-card .ant-card-body) {
  padding: 0 !important;
  background: transparent !important;
}

.application-search-overlay {
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
}

.application-search-floating-panel {
  width: min(100%, 480px);
  padding: 0;
  background: transparent;
  border: none;
  box-shadow: none;
  backdrop-filter: none;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.application-search-floating-input {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 0 14px;
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.6)),
    rgba(255, 255, 255, 0.44);
  border: 1px solid rgba(255, 255, 255, 0.74);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 16px 32px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(125%);
}

.application-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.application-search-floating-field {
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
  line-height: 34px;
}

.application-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.application-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.66)),
    rgba(255, 255, 255, 0.5);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 18px 36px rgba(15, 23, 42, 0.1);
}

.application-search-suggestions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-radius: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.52), rgba(255, 255, 255, 0.36)),
    rgba(255, 255, 255, 0.22);
  border: 1px solid rgba(255, 255, 255, 0.62);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.74),
    0 16px 30px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(124%);
}

.application-search-suggestion {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  padding: 10px 12px;
  border: none;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.34);
  color: #0f172a;
  text-align: left;
  cursor: pointer;
  transition: background 0.18s ease, transform 0.18s ease;
}

.application-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
  transform: translateY(-1px);
}

.application-search-suggestion-loading {
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.28);
  color: rgba(51, 65, 85, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.application-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.application-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.74);
  font-size: 12px;
  line-height: 1.4;
}

.application-search-fade-enter-active,
.application-search-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.application-search-fade-enter-from,
.application-search-fade-leave-to {
  opacity: 0;
}

.application-search-fade-enter-from .application-search-floating-panel,
.application-search-fade-leave-to .application-search-floating-panel {
  transform: translateY(-8px);
  opacity: 0;
}

.application-search-fade-enter-to .application-search-floating-panel,
.application-search-fade-leave-from .application-search-floating-panel {
  transform: translateY(0);
  opacity: 1;
}

.overview-loading-state {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.overview-loading-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 4px 2px;
}

.overview-loading-icon {
  color: var(--color-dashboard-800);
  font-size: 20px;
}

.overview-loading-title {
  color: var(--color-text-main);
  font-size: 18px;
  font-weight: 800;
}

.overview-loading-text {
  margin-top: 4px;
  color: var(--color-text-soft);
  font-size: 13px;
  line-height: 1.7;
}

.overview-loading-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.overview-loading-metric {
  height: 104px;
  border-radius: 18px;
}

.overview-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.9fr) minmax(320px, 1fr);
  gap: 18px;
}

.overview-chart-panel {
  position: relative;
  min-height: 284px;
  border-radius: 24px;
  padding: 22px 22px 18px;
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
  content: '';
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
  gap: 18px;
  margin-bottom: 20px;
}

.overview-chart-label {
  color: rgba(125, 211, 252, 0.92);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.overview-chart-title {
  margin-top: 8px;
  color: #f8fafc;
  font-size: 28px;
  font-weight: 800;
  line-height: 1.1;
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

.overview-chart-canvas {
  height: 188px;
  width: 100%;
}

.overview-chart-footnote {
  margin-top: 8px;
  color: rgba(226, 232, 240, 0.54);
  font-size: 12px;
  line-height: 1.6;
}

.overview-summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.overview-summary-card {
  display: flex;
  width: 100%;
  min-height: 135px;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  padding: 18px;
  border-radius: 20px;
  border: 1px solid rgba(71, 85, 105, 0.38);
  background:
    radial-gradient(circle at top right, rgba(148, 163, 184, 0.12), transparent 32%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.98), rgba(30, 41, 59, 0.94));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 18px 38px rgba(15, 23, 42, 0.14);
  appearance: none;
  text-align: left;
  transition: transform 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease;
}

.overview-summary-card-clickable {
  cursor: pointer;
}

.overview-summary-card-clickable:hover {
  transform: translateY(-1px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 22px 44px rgba(15, 23, 42, 0.18);
}

.overview-summary-head {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.overview-summary-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(248, 250, 252, 0.9);
  font-size: 15px;
}

.overview-summary-badge {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(226, 232, 240, 0.72);
  font-size: 11px;
  font-weight: 700;
}

.overview-summary-label {
  color: rgba(226, 232, 240, 0.74);
  font-size: 13px;
  font-weight: 700;
}

.overview-summary-value {
  color: #f8fafc;
  font-size: 34px;
  font-weight: 800;
  line-height: 1;
}

.overview-summary-hint {
  margin-top: auto;
  color: rgba(226, 232, 240, 0.58);
  font-size: 12px;
  line-height: 1.6;
}

.overview-summary-card-default {
  border-color: rgba(148, 163, 184, 0.18);
}

.overview-summary-card-success {
  border-color: rgba(74, 222, 128, 0.24);
  background:
    radial-gradient(circle at top right, rgba(74, 222, 128, 0.2), transparent 34%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.98), rgba(22, 101, 52, 0.92));
}

.overview-summary-card-running {
  border-color: rgba(96, 165, 250, 0.24);
  background:
    radial-gradient(circle at top right, rgba(96, 165, 250, 0.2), transparent 34%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.98), rgba(29, 78, 216, 0.9));
}

.overview-summary-card-warning {
  border-color: rgba(251, 191, 36, 0.22);
  background:
    radial-gradient(circle at top right, rgba(251, 191, 36, 0.18), transparent 34%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.98), rgba(146, 64, 14, 0.92));
}

.overview-summary-card-danger {
  border-color: rgba(248, 113, 113, 0.24);
  background:
    radial-gradient(circle at top right, rgba(248, 113, 113, 0.2), transparent 34%),
    linear-gradient(160deg, rgba(15, 23, 42, 0.98), rgba(127, 29, 29, 0.92));
}

.application-workbench-columns {
  display: grid;
  width: 100%;
  gap: 20px;
  align-items: start;
  grid-auto-flow: row;
}

.application-search-card {
  margin-top: 4px;
}

.application-search-shell {
  display: flex;
  justify-content: flex-start;
}

.application-search-bar {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  width: min(100%, 760px);
  max-width: 70%;
  min-height: 72px;
  padding: 14px 16px;
  border-radius: 24px;
  border: 1px solid rgba(71, 85, 105, 0.34);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.1), transparent 26%),
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.14), transparent 32%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.94), rgba(15, 23, 42, 0.88));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.06),
    0 18px 42px rgba(2, 6, 23, 0.12);
  backdrop-filter: blur(18px);
  overflow: hidden;
}

.application-search-bar::before {
  content: '';
  position: absolute;
  inset: 0 0 auto;
  height: 1px;
  background: linear-gradient(90deg, rgba(56, 189, 248, 0), rgba(56, 189, 248, 0.5), rgba(34, 197, 94, 0.36), rgba(56, 189, 248, 0));
  pointer-events: none;
}

.application-search-meta {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
  min-width: 0;
  color: rgba(226, 232, 240, 0.82);
}

.application-search-meta-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #34d399;
  box-shadow: 0 0 0 6px rgba(52, 211, 153, 0.14);
}

.application-search-meta-label {
  color: rgba(226, 232, 240, 0.86);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.application-search-meta-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.32);
  background: rgba(15, 23, 42, 0.56);
  color: #f8fafc;
  font-size: 12px;
  font-weight: 800;
}

.application-search-field {
  position: relative;
  display: flex;
  align-items: center;
  flex: 1 1 320px;
  min-width: 260px;
  padding-left: 40px;
  padding-right: 8px;
  border-radius: 18px;
  border: 1px solid rgba(71, 85, 105, 0.38);
  background: rgba(15, 23, 42, 0.56);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  transition: border-color 0.2s ease, background 0.2s ease, box-shadow 0.2s ease;
}

.application-search-field:hover {
  border-color: rgba(96, 165, 250, 0.34);
  background: rgba(15, 23, 42, 0.64);
}

.application-search-field:focus-within {
  border-color: rgba(56, 189, 248, 0.46);
  background: rgba(15, 23, 42, 0.7);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 0 0 4px rgba(56, 189, 248, 0.12);
}

.application-search-field-icon {
  position: absolute;
  left: 14px;
  color: rgba(125, 211, 252, 0.84);
  font-size: 14px;
  pointer-events: none;
}

.application-search-actions {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
}

:deep(.application-search-select.ant-select) {
  width: 100%;
}

:deep(.application-search-select .ant-select-selector) {
  height: 46px !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
  padding: 0 34px 0 0 !important;
}

:deep(.application-search-select .ant-select-selection-search-input) {
  height: 46px !important;
  color: #f8fafc;
}

:deep(.application-search-select .ant-select-selection-item) {
  display: inline-flex;
  align-items: center;
  color: #f8fafc;
  font-weight: 700;
}

:deep(.application-search-select .ant-select-selection-placeholder) {
  display: inline-flex;
  align-items: center;
  color: rgba(148, 163, 184, 0.92);
}

:deep(.application-search-select .ant-select-arrow),
:deep(.application-search-select .ant-select-clear) {
  color: rgba(148, 163, 184, 0.92);
}

:deep(.application-search-reset.ant-btn) {
  height: 44px;
  padding-inline: 18px;
  border-radius: 16px;
  border-color: rgba(71, 85, 105, 0.34) !important;
  background: rgba(15, 23, 42, 0.42) !important;
  color: rgba(226, 232, 240, 0.86) !important;
  box-shadow: none !important;
}

:deep(.application-search-reset.ant-btn:hover),
:deep(.application-search-reset.ant-btn:focus),
:deep(.application-search-reset.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.28) !important;
  background: rgba(30, 41, 59, 0.72) !important;
  color: #f8fafc !important;
}

:deep(.application-search-reset.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(15, 23, 42, 0.78) !important;
  color: #f8fafc !important;
}

.application-workbench-columns-3 {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.application-workbench-columns-2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.application-workbench-columns-1 {
  grid-template-columns: minmax(0, 1fr);
}

.workbench-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}

.workbench-loading-state {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.workbench-loading-copy {
  padding: 2px 2px 0;
}

.workbench-loading-title {
  color: var(--color-text-main);
  font-size: 18px;
  font-weight: 800;
}

.workbench-loading-text {
  margin-top: 6px;
  color: var(--color-text-soft);
  font-size: 13px;
  line-height: 1.7;
}

.workbench-skeleton-card {
  border-radius: 20px;
  padding: 18px;
  background: rgba(255, 255, 255, 0.86);
}

.application-workbench-card {
  --workbench-card-bg:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.12), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.12), transparent 24%),
    linear-gradient(180deg, rgba(2, 6, 23, 0.98), rgba(15, 23, 42, 0.98) 42%, rgba(19, 30, 53, 0.98));
  position: relative;
  min-width: 0;
  align-self: start;
  height: auto;
  min-height: 0;
  overflow: visible;
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
}

.application-workbench-card::before {
  display: none;
}

:deep(.application-workbench-card .ant-card-body) {
  position: relative;
  display: block;
  height: 100%;
  min-height: 100%;
  padding: 0;
  overflow: visible;
}

.workbench-card-shell {
  --workbench-card-padding: 24px;
  --workbench-action-right-offset: 18px;
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 100%;
  overflow: hidden;
  border-radius: 24px;
  border: 1px solid rgba(71, 85, 105, 0.48);
  background: var(--workbench-card-bg);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 24px 52px rgba(2, 6, 23, 0.28);
  padding: var(--workbench-card-padding);
  isolation: isolate;
}

.workbench-card-shell::before {
  content: '';
  position: absolute;
  inset: 0 0 auto 0;
  height: 1px;
  background: linear-gradient(90deg, rgba(56, 189, 248, 0), rgba(56, 189, 248, 0.55), rgba(34, 197, 94, 0.4), rgba(56, 189, 248, 0));
  pointer-events: none;
  z-index: 0;
}

.workbench-card-shell::after {
  content: '';
  position: absolute;
  inset: -20% -30%;
  background:
    linear-gradient(112deg, transparent 36%, rgba(56, 189, 248, 0.16) 46%, rgba(34, 197, 94, 0.14) 54%, transparent 66%),
    radial-gradient(circle at 18% 50%, rgba(56, 189, 248, 0.18), transparent 22%),
    radial-gradient(circle at 82% 50%, rgba(34, 197, 94, 0.16), transparent 24%);
  opacity: 0;
  transform: translateX(-18%);
  pointer-events: none;
  z-index: 0;
}

.workbench-card-shell > * {
  position: relative;
  z-index: 1;
}

.workbench-card-shell-running {
  border-color: rgba(56, 189, 248, 0.42);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.07),
    0 24px 52px rgba(2, 6, 23, 0.3),
    0 0 0 1px rgba(56, 189, 248, 0.12),
    0 0 28px rgba(56, 189, 248, 0.14),
    0 0 36px rgba(34, 197, 94, 0.12);
  animation: workbench-card-running-breathe 3.1s ease-in-out infinite;
}

.workbench-card-shell-running::before {
  height: 2px;
  background:
    linear-gradient(90deg, rgba(56, 189, 248, 0), rgba(56, 189, 248, 0.72), rgba(34, 197, 94, 0.62), rgba(56, 189, 248, 0.76), rgba(56, 189, 248, 0));
  background-size: 220% 100%;
  box-shadow: 0 0 10px rgba(56, 189, 248, 0.24);
  animation: workbench-card-running-topline 2.5s linear infinite;
}

.workbench-card-shell-running::after {
  opacity: 0.86;
  animation: workbench-card-running-sweep 4.1s cubic-bezier(0.22, 1, 0.36, 1) infinite;
}

@keyframes workbench-card-running-breathe {
  0%,
  100% {
    transform: translateY(0);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.07),
      0 24px 52px rgba(2, 6, 23, 0.3),
      0 0 0 1px rgba(56, 189, 248, 0.12),
      0 0 28px rgba(56, 189, 248, 0.14),
      0 0 36px rgba(34, 197, 94, 0.12);
  }

  50% {
    transform: translateY(-1px) scale(1.002);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.1),
      0 28px 60px rgba(2, 6, 23, 0.34),
      0 0 0 1px rgba(56, 189, 248, 0.18),
      0 0 40px rgba(56, 189, 248, 0.2),
      0 0 52px rgba(34, 197, 94, 0.16);
  }
}

@keyframes workbench-card-running-topline {
  0% {
    background-position: 0% 50%;
    opacity: 0.74;
  }

  50% {
    background-position: 100% 50%;
    opacity: 0.94;
  }

  100% {
    background-position: 200% 50%;
    opacity: 0.76;
  }
}

@keyframes workbench-card-running-sweep {
  0% {
    opacity: 0.44;
    transform: translateX(-22%) scale(1.01);
  }

  50% {
    opacity: 0.82;
    transform: translateX(10%) scale(1.02);
  }

  100% {
    opacity: 0.44;
    transform: translateX(28%) scale(1.01);
  }
}

.application-workbench-card-collapsed {
  height: 250px;
  min-height: 250px;
}

.workbench-card-header-shell {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 96px;
}

.workbench-card-header {
  position: relative;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 18px;
  border-bottom: none;
}

.workbench-card-header-actions {
  position: absolute;
  top: 14px;
  right: var(--workbench-action-right-offset);
  display: flex;
  align-items: center;
  flex-direction: row;
  gap: 10px;
  flex-shrink: 0;
  z-index: 2;
}

.workbench-card-header-copy {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  min-width: 0;
}

.workbench-app-eyebrow {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 10px;
  padding-right: 150px;
  min-width: 0;
  overflow: hidden;
}

.workbench-card-title-row {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.workbench-app-title {
  border: none;
  background: transparent;
  flex: 0 1 auto;
  width: auto;
  max-width: 100%;
  min-width: 0;
  padding: 0;
  color: #f8fafc;
  font-size: 23px;
  font-weight: 800;
  line-height: 1.2;
  cursor: pointer;
  text-align: left;
  transition: color 0.2s ease;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: block;
}

.workbench-app-title:hover {
  color: #4ade80;
}

.workbench-app-key {
  display: inline-flex;
  position: relative;
  align-items: center;
  justify-content: flex-start;
  flex: 0 1 auto;
  min-width: 0;
  width: auto;
  max-width: 100%;
  padding: 6px 14px 6px 10px;
  border-radius: 999px;
  border: 1px solid rgba(100, 116, 139, 0.36);
  background: rgba(15, 23, 42, 0.64);
  color: #cbd5e1;
  font-size: 12px;
  font-weight: 700;
  font-family:
    'SFMono-Regular',
    'Consolas',
    'Liberation Mono',
    monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.workbench-app-key-overflowing::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 22px;
  border-radius: 0 999px 999px 0;
  background: linear-gradient(90deg, rgba(15, 23, 42, 0), rgba(15, 23, 42, 0.96) 82%);
  pointer-events: none;
}


.workbench-app-project,
.workbench-app-owner-inline {
  display: inline-flex;
  align-items: center;
  width: max-content;
  max-width: 100%;
  padding: 5px 10px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.38);
  background: rgba(15, 23, 42, 0.48);
  color: rgba(191, 219, 254, 0.88);
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
  letter-spacing: 0.03em;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.workbench-app-project {
  flex: 0 1 auto;
  max-width: 100%;
}

.workbench-app-owner-inline {
  flex: 0 1 auto;
  max-width: 100%;
}


.workbench-app-state {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.workbench-app-state::before {
  content: '';
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 4px rgba(255, 255, 255, 0.06);
}

.workbench-card-collapse {
  flex-shrink: 0;
}

.workbench-card-collapsed-summary {
  margin-top: auto;
  padding-top: 18px;
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  justify-content: flex-end;
  gap: 12px;
}

.workbench-collapsed-item {
  display: inline-flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.58);
  border: 1px solid rgba(71, 85, 105, 0.36);
  color: rgba(226, 232, 240, 0.72);
  font-size: 12px;
  font-weight: 600;
}

.workbench-collapsed-tail {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.workbench-collapsed-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.workbench-collapsed-content {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: flex-end;
  flex-shrink: 0;
  margin-right: calc(var(--workbench-action-right-offset) - var(--workbench-card-padding));
}

.workbench-collapsed-latest {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.workbench-collapsed-chip {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  width: max-content;
  max-width: 100%;
  min-height: 36px;
  border: 1px solid rgba(71, 85, 105, 0.36);
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.58);
  padding: 8px 14px;
  color: #e2e8f0;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  box-sizing: border-box;
  font-family:
    'SFMono-Regular',
    'Consolas',
    'Liberation Mono',
    monospace;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: border-color 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.workbench-collapsed-chip-order {
  font-family:
    'SFMono-Regular',
    'Consolas',
    'Liberation Mono',
    monospace;
}

button.workbench-collapsed-chip {
  cursor: pointer;
}

button.workbench-collapsed-chip:hover {
  border-color: rgba(125, 211, 252, 0.28);
  color: #bae6fd;
  transform: translateY(-1px);
}

.workbench-collapsed-chip-muted {
  color: rgba(226, 232, 240, 0.66);
}

.app-state-chip-active {
  color: #4ade80;
  background: rgba(20, 83, 45, 0.35);
  border: 1px solid rgba(74, 222, 128, 0.34);
}

.app-state-chip-inactive {
  color: #cbd5e1;
  background: rgba(30, 41, 59, 0.52);
  border: 1px solid rgba(100, 116, 139, 0.34);
}

.workbench-meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 0;
}

.workbench-release-overview {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  border: none;
  box-shadow: none;
}

.workbench-release-overview-chip {
  display: inline-flex;
  align-items: center;
  justify-content: flex-start;
  min-width: 0;
  width: min(var(--workbench-overview-chip-width, max-content), 100%);
  max-width: 100%;
  min-height: 36px;
  padding: 8px 14px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.34);
  background: rgba(15, 23, 42, 0.54);
  color: rgba(226, 232, 240, 0.8);
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  box-sizing: border-box;
}

button.workbench-release-overview-chip {
  cursor: pointer;
  text-align: left;
  transition: border-color 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

button.workbench-release-overview-chip:hover {
  border-color: rgba(125, 211, 252, 0.28);
  color: #bae6fd;
  transform: translateY(-1px);
}

.workbench-release-overview-chip-order {
  font-family:
    'SFMono-Regular',
    'Consolas',
    'Liberation Mono',
    monospace;
}

.workbench-release-overview-chip-muted {
  color: rgba(226, 232, 240, 0.64);
}

.workbench-status-chip {
  gap: 8px;
}

.workbench-status-chip-running {
  border-color: rgba(56, 189, 248, 0.32);
  color: #bae6fd;
  background: rgba(30, 64, 175, 0.26);
}

.workbench-status-chip-idle {
  border-color: rgba(71, 85, 105, 0.34);
  color: rgba(226, 232, 240, 0.74);
  background: rgba(15, 23, 42, 0.52);
}

.workbench-status-chip-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #38bdf8;
  box-shadow: 0 0 0 0 rgba(56, 189, 248, 0.55);
  animation: workbench-status-pulse 1.5s ease-out infinite;
}

@keyframes workbench-status-pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(56, 189, 248, 0.55);
    transform: scale(1);
  }

  70% {
    box-shadow: 0 0 0 8px rgba(56, 189, 248, 0);
    transform: scale(1.06);
  }

  100% {
    box-shadow: 0 0 0 0 rgba(56, 189, 248, 0);
    transform: scale(1);
  }
}

.workbench-meta-chip {
  display: inline-flex;
  align-items: center;
  padding: 7px 12px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.52);
  border: 1px solid rgba(71, 85, 105, 0.32);
  color: rgba(226, 232, 240, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.workbench-template-strip {
  margin-top: 18px;
  border-radius: 18px;
  padding: 16px 18px;
  border: 1px solid rgba(71, 85, 105, 0.32);
  background:
    linear-gradient(180deg, rgba(15, 23, 42, 0.64), rgba(15, 23, 42, 0.48)),
    rgba(2, 6, 23, 0.38);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
}

.latest-release-panel {
  margin-top: 18px;
  margin-bottom: 12px;
  padding: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.workbench-template-strip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.workbench-template-strip-muted {
  background:
    linear-gradient(180deg, rgba(15, 23, 42, 0.54), rgba(15, 23, 42, 0.42)),
    rgba(2, 6, 23, 0.34);
}

.workbench-strip-label,
.section-title {
  color: rgba(125, 211, 252, 0.92);
  font-size: 13px;
  font-weight: 700;
}

.section-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.section-header-row.compact {
  justify-content: flex-start;
  flex-wrap: wrap;
}

.section-loading-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.56);
  border: 1px solid rgba(56, 189, 248, 0.24);
  color: rgba(186, 230, 253, 0.9);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.section-loading-chip.compact {
  padding: 4px 8px;
  font-size: 11px;
}

.workbench-strip-value {
  flex: 1;
  min-width: 180px;
  color: #f8fafc;
  font-size: 14px;
  font-weight: 600;
}

.env-status-success {
  border-color: rgba(74, 222, 128, 0.26);
  background:
    radial-gradient(circle at top right, rgba(74, 222, 128, 0.16), transparent 36%),
    linear-gradient(180deg, rgba(6, 78, 59, 0.22), rgba(15, 23, 42, 0.82));
}

.env-status-running {
  border-color: rgba(56, 189, 248, 0.28);
  background:
    radial-gradient(circle at top right, rgba(56, 189, 248, 0.16), transparent 36%),
    linear-gradient(180deg, rgba(30, 64, 175, 0.18), rgba(15, 23, 42, 0.82));
}

.env-status-failed {
  border-color: rgba(248, 113, 113, 0.28);
  background:
    radial-gradient(circle at top right, rgba(248, 113, 113, 0.16), transparent 36%),
    linear-gradient(180deg, rgba(127, 29, 29, 0.2), rgba(15, 23, 42, 0.82));
}

.env-status-pending,
.env-status-neutral {
  border-color: rgba(71, 85, 105, 0.34);
  background:
    radial-gradient(circle at top right, rgba(148, 163, 184, 0.12), transparent 36%),
    linear-gradient(180deg, rgba(30, 41, 59, 0.16), rgba(15, 23, 42, 0.8));
}

.latest-release-state-bubbles {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.env-release-view {
  margin-top: 14px;
  width: 100%;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.env-switch-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.env-switch-btn {
  border: 1px solid rgba(71, 85, 105, 0.36);
  background: rgba(15, 23, 42, 0.44);
  color: rgba(226, 232, 240, 0.76);
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.22s ease;
}

.env-switch-btn:hover {
  border-color: rgba(56, 189, 248, 0.42);
  color: #bae6fd;
}

.env-switch-btn-active {
  border-color: rgba(56, 189, 248, 0.52);
  background: rgba(30, 64, 175, 0.26);
  color: #bae6fd;
}

.env-card-switch-enter-active,
.env-card-switch-leave-active {
  transition: opacity 0.24s ease, transform 0.24s ease;
}

.env-card-switch-enter-from {
  opacity: 0;
  transform: translateX(12px);
}

.env-card-switch-leave-to {
  opacity: 0;
  transform: translateX(-12px);
}

.env-release-card {
  border-radius: 16px;
  padding: 14px 16px;
  border: 1px solid rgba(71, 85, 105, 0.32);
  background: rgba(15, 23, 42, 0.48);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
}

.env-release-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.env-release-env {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52px;
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.34);
  background: rgba(2, 6, 23, 0.48);
  color: #e2e8f0;
  font-size: 12px;
  font-weight: 800;
  text-transform: lowercase;
  letter-spacing: 0.04em;
}

.env-release-state-bubbles {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.env-release-main {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.env-release-empty {
  margin-top: 12px;
  color: rgba(226, 232, 240, 0.68);
  font-size: 12px;
}

.application-workbench-card-release-active {
  height: 250px;
  min-height: 250px;
}

.application-workbench-card-release-active .workbench-card-shell {
  height: 100%;
  min-height: 100%;
  overflow: hidden;
}

.workbench-card-view {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
  height: 100%;
  flex-direction: column;
}

.workbench-card-release-summary {
  width: 100%;
}

.workbench-card-release-detail {
  justify-content: space-between;
  gap: 18px;
  margin: 0;
}

.workbench-release-detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}

.workbench-release-detail-env {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  color: #f8fafc;
  font-size: 23px;
  font-weight: 800;
  line-height: 1.2;
  letter-spacing: 0.02em;
  text-transform: lowercase;
}

.workbench-release-state-stack {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 12px;
}

.workbench-release-detail-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.25fr);
  gap: 14px;
  align-items: stretch;
}

.latest-release-panel.workbench-release-detail-section,
.env-release-view.workbench-release-detail-section {
  margin: 0;
  flex: none;
  min-height: 0;
  border: 1px solid rgba(71, 85, 105, 0.34);
  border-radius: 18px;
  background:
    radial-gradient(circle at top right, rgba(56, 189, 248, 0.1), transparent 38%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.58), rgba(15, 23, 42, 0.42));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
  padding: 16px;
}

.workbench-release-detail-section-empty {
  justify-content: flex-start;
}

.workbench-release-detail-empty-text {
  margin-top: 14px;
  color: rgba(226, 232, 240, 0.68);
  font-size: 13px;
  line-height: 1.7;
}

.workbench-release-env-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
  max-height: 278px;
  overflow: auto;
  padding-right: 4px;
}

.workbench-release-detail-meta {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: rgba(203, 213, 225, 0.82);
  font-size: 12px;
  line-height: 1.6;
}

.workbench-release-detail-meta span {
  display: inline-flex;
  max-width: 100%;
  min-width: 0;
  padding: 5px 9px;
  border-radius: 999px;
  border: 1px solid rgba(71, 85, 105, 0.32);
  background: rgba(2, 6, 23, 0.24);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workbench-release-detail-actions {
  margin-top: auto;
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.workbench-card-release-detail :deep(.workbench-secondary-action.ant-btn) {
  border-color: rgba(71, 85, 105, 0.28) !important;
  background: rgba(15, 23, 42, 0.38) !important;
  color: rgba(226, 232, 240, 0.56) !important;
  box-shadow: none !important;
}

.workbench-card-release-detail :deep(.workbench-secondary-action.ant-btn:hover),
.workbench-card-release-detail :deep(.workbench-secondary-action.ant-btn:focus),
.workbench-card-release-detail :deep(.workbench-secondary-action.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.26) !important;
  background: rgba(30, 41, 59, 0.62) !important;
  color: rgba(248, 250, 252, 0.86) !important;
}

.workbench-release-detail-empty {
  min-height: 120px;
}

.workbench-card-detail-switch-enter-active,
.workbench-card-detail-switch-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.workbench-card-detail-switch-enter-from {
  opacity: 0;
  transform: translateX(16px);
}

.workbench-card-detail-switch-leave-to {
  opacity: 0;
  transform: translateX(-16px);
}

.env-release-main span.state-bubble {
  cursor: default;
}

:deep(.rollback-preview-modal) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

:deep(.rollback-preview-summary) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 16px;
}

:deep(.rollback-preview-summary-item) {
  display: flex;
  gap: 8px;
  align-items: center;
}

:deep(.rollback-preview-summary-label) {
  min-width: 56px;
  color: #7c8597;
}

:deep(.rollback-preview-summary-value) {
  color: #1f2937;
  font-weight: 600;
}

:deep(.rollback-preview-reason) {
  padding: 10px 12px;
  border: 1px solid #ffe7ba;
  border-radius: 12px;
  background: #fff7e8;
  color: #ad6800;
}

:deep(.rollback-preview-checks) {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

:deep(.rollback-preview-check-item) {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid #e6ebf5;
  border-radius: 12px;
  background: #f8fafc;
}

:deep(.rollback-preview-check-tag) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  color: #64748b;
  background: rgba(148, 163, 184, 0.16);
}

:deep(.rollback-preview-check-tag-pass) {
  color: #166534;
  background: rgba(34, 197, 94, 0.14);
}

:deep(.rollback-preview-check-tag-warn) {
  color: #92400e;
  background: rgba(245, 158, 11, 0.16);
}

:deep(.rollback-preview-check-tag-blocked) {
  color: #b91c1c;
  background: rgba(239, 68, 68, 0.16);
}

:deep(.rollback-preview-check-copy) {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

:deep(.rollback-preview-check-title) {
  color: #1f2937;
  font-size: 13px;
  font-weight: 600;
}

:deep(.rollback-preview-check-message) {
  color: #64748b;
  font-size: 12px;
  line-height: 1.7;
}

:deep(.rollback-preview-scope) {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

:deep(.rollback-preview-scope-title) {
  color: #1f2937;
  font-weight: 600;
}

:deep(.rollback-preview-param-list) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

:deep(.rollback-preview-param-item) {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid #e6ebf5;
  border-radius: 12px;
  background: #f8fafc;
}

:deep(.rollback-preview-param-key) {
  font-size: 12px;
  color: #7c8597;
}

:deep(.rollback-preview-param-value) {
  color: #1f2937;
  word-break: break-all;
}

:deep(.rollback-preview-empty) {
  color: #7c8597;
}

.state-bubble {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  width: fit-content;
  max-width: 100%;
  border: none;
  border-radius: 999px;
  min-height: 38px;
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.state-bubble:hover {
  transform: translateY(-1px);
}

.state-bubble-current {
  background: rgba(22, 101, 52, 0.36);
  color: #86efac;
  box-shadow: 0 10px 18px rgba(22, 101, 52, 0.18);
}

.state-bubble-latest {
  background: rgba(180, 83, 9, 0.34);
  color: #fcd34d;
  box-shadow: 0 10px 18px rgba(180, 83, 9, 0.2);
}

.latest-release-main {
  margin-top: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.latest-release-order-bubble {
  font-family:
    'SFMono-Regular',
    'Consolas',
    'Liberation Mono',
    monospace;
}

.workbench-actions {
  margin-top: 20px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.workbench-footer-row {
  margin-top: auto;
  padding-top: 16px;
  border-top: 1px dashed rgba(71, 85, 105, 0.44);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}

.workbench-footer-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.inline-loading-state {
  margin-top: 12px;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: rgba(226, 232, 240, 0.72);
  font-size: 13px;
  font-weight: 600;
}

.inline-loading-state.compact {
  margin-top: 0;
  font-size: 12px;
}

.inline-loading-icon {
  color: #7dd3fc;
}

.workbench-manage-trigger {
  flex-shrink: 0;
}

:deep(.workbench-manage-popover .ant-popover-inner) {
  border-radius: 18px;
  padding: 12px;
}

.workbench-manage-actions {
  min-width: 140px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.workbench-empty-state {
  margin-top: 12px;
  border-radius: 16px;
  border: 1px dashed rgba(71, 85, 105, 0.42);
  background: rgba(15, 23, 42, 0.42);
  min-height: 100px;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  z-index: 1;
}

.workbench-empty-state :deep(.ant-empty-description) {
  color: rgba(226, 232, 240, 0.6);
  font-size: 13px;
}

.application-workbench-card :deep(.dashboard-chip) {
  border-color: rgba(71, 85, 105, 0.36);
  background: rgba(15, 23, 42, 0.56);
  color: rgba(226, 232, 240, 0.78);
}

.application-workbench-card :deep(.dashboard-chip-running) {
  border-color: rgba(74, 222, 128, 0.32);
  background: rgba(21, 128, 61, 0.26);
  color: #86efac;
}

.application-workbench-card :deep(.dashboard-chip-warning) {
  border-color: rgba(251, 191, 36, 0.28);
  background: rgba(146, 64, 14, 0.28);
  color: #fde68a;
}

.application-workbench-card :deep(.dashboard-chip-danger) {
  border-color: rgba(248, 113, 113, 0.3);
  background: rgba(127, 29, 29, 0.3);
  color: #fecaca;
}

.application-workbench-card :deep(.dashboard-chip-neutral) {
  border-color: rgba(71, 85, 105, 0.34);
  background: rgba(15, 23, 42, 0.5);
  color: #cbd5e1;
}

.application-workbench-card :deep(.ant-btn) {
  border-radius: 12px;
}

:deep(.workbench-primary-action.ant-btn),
:deep(.env-release-action.ant-btn),
:deep(.workbench-secondary-action.ant-btn),
:deep(.workbench-manage-trigger.ant-btn) {
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding-inline: 16px;
  font-weight: 700;
}

:deep(.workbench-manage-trigger.ant-btn .ant-btn-icon),
:deep(.workbench-primary-action.ant-btn .ant-btn-icon),
:deep(.workbench-secondary-action.ant-btn .ant-btn-icon),
:deep(.env-release-action.ant-btn .ant-btn-icon) {
  display: inline-flex;
  align-items: center;
}

:deep(.workbench-primary-action.ant-btn) {
  border-color: rgba(30, 64, 175, 0.18) !important;
  background: #1d4ed8 !important;
  color: #eff6ff !important;
  box-shadow: none !important;
}

:deep(.workbench-primary-action.ant-btn:hover),
:deep(.workbench-primary-action.ant-btn:focus),
:deep(.workbench-primary-action.ant-btn:focus-visible),
:deep(.workbench-primary-action.ant-btn:active) {
  border-color: rgba(29, 78, 216, 0.28) !important;
  background: #2563eb !important;
  color: #ffffff !important;
  box-shadow: none !important;
}

:deep(.workbench-primary-action.ant-btn[disabled]),
:deep(.workbench-primary-action.ant-btn.ant-btn-disabled) {
  border-color: rgba(30, 64, 175, 0.28) !important;
  background: #1d4ed8 !important;
  color: #eff6ff !important;
  box-shadow: none !important;
}

:deep(.workbench-config-readonly.ant-btn) {
  cursor: not-allowed !important;
}

:deep(.workbench-secondary-action.ant-btn),
:deep(.workbench-card-collapse.ant-btn),
:deep(.env-release-action.ant-btn) {
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.9) !important;
  color: #1e3a8a !important;
  box-shadow: none !important;
}

:deep(.workbench-secondary-action.ant-btn:hover),
:deep(.workbench-card-collapse.ant-btn:hover),
:deep(.env-release-action.ant-btn:hover),
:deep(.workbench-secondary-action.ant-btn:focus),
:deep(.workbench-card-collapse.ant-btn:focus),
:deep(.env-release-action.ant-btn:focus),
:deep(.workbench-secondary-action.ant-btn:focus-visible),
:deep(.workbench-card-collapse.ant-btn:focus-visible),
:deep(.env-release-action.ant-btn:focus-visible),
:deep(.workbench-secondary-action.ant-btn:active),
:deep(.workbench-card-collapse.ant-btn:active),
:deep(.env-release-action.ant-btn:active) {
  border-color: rgba(59, 130, 246, 0.24) !important;
  color: #1d4ed8 !important;
  background: rgba(239, 246, 255, 0.92) !important;
  box-shadow: none !important;
}

:deep(.workbench-secondary-action.ant-btn[disabled]),
:deep(.workbench-card-collapse.ant-btn[disabled]),
:deep(.env-release-action.ant-btn[disabled]),
:deep(.workbench-secondary-action.ant-btn.ant-btn-disabled),
:deep(.workbench-card-collapse.ant-btn.ant-btn-disabled),
:deep(.env-release-action.ant-btn.ant-btn-disabled) {
  border-color: rgba(148, 163, 184, 0.18) !important;
  background: rgba(241, 245, 249, 0.84) !important;
  color: rgba(100, 116, 139, 0.7) !important;
  box-shadow: none !important;
}

:deep(.workbench-manage-popover .ant-popover-inner) {
  border: 1px solid rgba(148, 163, 184, 0.2);
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.88)),
    rgba(255, 255, 255, 0.9);
  box-shadow: 0 24px 48px rgba(15, 23, 42, 0.1);
  backdrop-filter: blur(18px);
}

:deep(.workbench-manage-popover .ant-popover-inner-content) {
  padding: 0;
}

:deep(.workbench-manage-popover .workbench-manage-actions .ant-btn) {
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.9) !important;
  color: #1e3a8a !important;
  box-shadow: none !important;
}

:deep(.workbench-manage-popover .workbench-manage-actions .ant-btn:hover),
:deep(.workbench-manage-popover .workbench-manage-actions .ant-btn:focus),
:deep(.workbench-manage-popover .workbench-manage-actions .ant-btn:focus-visible),
:deep(.workbench-manage-popover .workbench-manage-actions .ant-btn:active) {
  border-color: rgba(59, 130, 246, 0.24) !important;
  color: #1d4ed8 !important;
  background: rgba(239, 246, 255, 0.92) !important;
}

:deep(.workbench-manage-popover .workbench-danger-action.ant-btn),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous) {
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.9) !important;
  color: #ef4444 !important;
  box-shadow: none !important;
}

:deep(.workbench-manage-popover .workbench-danger-action.ant-btn span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous span) {
  color: #ef4444 !important;
}

:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:hover),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:focus),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:focus-visible),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:active),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:hover),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:focus),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:focus-visible),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:active) {
  border-color: rgba(248, 113, 113, 0.28) !important;
  color: #dc2626 !important;
  background: rgba(254, 242, 242, 0.98) !important;
  box-shadow: none !important;
}

:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:hover span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:focus span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:focus-visible span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn:active span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:hover span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:focus span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:focus-visible span),
:deep(.workbench-manage-popover .workbench-danger-action.ant-btn.ant-btn-dangerous:active span) {
  color: #dc2626 !important;
}

.danger-icon {
  color: var(--color-danger);
}

.pagination-area {
  display: flex;
  justify-content: flex-end;
  margin-top: 40px;
}

.application-info-mini-table-scroll {
  max-height: 220px;
  overflow: auto;
  border: 1px solid #e6ebf5;
  border-radius: 12px;
  background: #ffffff;
}

.application-info-mini-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 13px;
}

.application-info-mini-table th,
.application-info-mini-table td {
  padding: 9px 12px;
  text-align: left;
  vertical-align: top;
  word-break: break-word;
}

.application-info-mini-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: #f8fafc;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.application-info-mini-table td {
  border-top: 1px solid #edf2f7;
  color: #1f2937;
  line-height: 1.55;
}

.application-info-mini-table th:first-child,
.application-info-mini-table td:first-child {
  width: 34%;
  color: #334155;
  font-weight: 700;
}

.intro-drawer-content {
  width: 100%;
}

:deep(.architecture-drawer .ant-drawer-header) {
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.1), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.98), rgba(15, 23, 42, 0.94));
}

:deep(.architecture-drawer .ant-drawer-title),
:deep(.architecture-drawer .ant-drawer-close) {
  color: #eff6ff;
}

:deep(.architecture-drawer .ant-drawer-body) {
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.1), transparent 30%),
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.12), transparent 34%),
    linear-gradient(180deg, #081120 0%, #0e1728 42%, #13203a 100%);
}

.architecture-content {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.architecture-hero,
.architecture-layer,
.architecture-runtime-cluster {
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 22px;
  background: linear-gradient(180deg, rgba(17, 24, 39, 0.88), rgba(15, 23, 42, 0.72));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.06),
    0 16px 34px rgba(2, 6, 23, 0.2);
}

.architecture-hero {
  padding: 22px 22px 18px;
}

.architecture-kicker {
  color: #38bdf8;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.architecture-heading {
  margin-top: 12px;
  color: #f8fafc;
  font-size: 23px;
  font-weight: 700;
  line-height: 1.35;
}

.architecture-copy {
  margin-top: 12px;
  color: rgba(226, 232, 240, 0.88);
  font-size: 14px;
  line-height: 1.8;
}

.architecture-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 16px;
}

.architecture-chip {
  display: inline-flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 999px;
  border: 1px solid rgba(96, 165, 250, 0.2);
  background: rgba(15, 23, 42, 0.52);
  color: #eff6ff;
  font-size: 12px;
  font-weight: 600;
}

.architecture-chip.accent {
  border-color: rgba(52, 211, 153, 0.24);
  color: #ecfeff;
}

.architecture-layer {
  padding: 20px;
}

.architecture-layer-head {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  margin-bottom: 16px;
}

.architecture-layer-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.18);
  color: #bfdbfe;
  font-size: 13px;
  font-weight: 700;
}

.architecture-layer-title {
  color: #f8fafc;
  font-size: 18px;
  font-weight: 700;
}

.architecture-layer-summary {
  margin-top: 4px;
  color: rgba(203, 213, 225, 0.9);
  font-size: 13px;
  line-height: 1.7;
}

.architecture-node-grid {
  display: grid;
  gap: 12px;
}

.architecture-node-grid-three {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.architecture-node-grid-four {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.architecture-node {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.96), rgba(15, 23, 42, 0.78));
  padding: 18px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.architecture-node.primary {
  border-color: rgba(59, 130, 246, 0.24);
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.98), rgba(15, 23, 42, 0.82));
}

.architecture-node.accent {
  border-color: rgba(52, 211, 153, 0.28);
  background: linear-gradient(180deg, rgba(6, 78, 59, 0.24), rgba(15, 23, 42, 0.82));
}

.architecture-node-title {
  color: #f8fafc;
  font-size: 15px;
  font-weight: 700;
}

.architecture-node-desc {
  margin-top: 8px;
  color: rgba(226, 232, 240, 0.82);
  font-size: 13px;
  line-height: 1.7;
}

.architecture-node-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}

.architecture-node-tags span {
  display: inline-flex;
  align-items: center;
  padding: 5px 10px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.56);
  color: #dbeafe;
  font-size: 11px;
  font-weight: 600;
}

.architecture-link {
  position: relative;
  margin: -4px 14px -2px;
  padding-left: 18px;
  color: #2563eb;
  font-size: 12px;
  font-weight: 600;
}

.architecture-link.accent {
  color: #059669;
}

.architecture-link::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: rgba(96, 165, 250, 0.7);
  transform: translateY(-50%);
  box-shadow: 0 0 18px rgba(96, 165, 250, 0.34);
}

.architecture-link.accent::before {
  background: rgba(52, 211, 153, 0.76);
  box-shadow: 0 0 18px rgba(52, 211, 153, 0.34);
}

.architecture-runtime-grid {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 14px;
}

.architecture-runtime-direct {
  height: 100%;
}

.architecture-runtime-cluster {
  padding: 18px;
}

.architecture-runtime-cluster-head {
  color: #ccfbf1;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
  margin-bottom: 14px;
}

.architecture-runtime-chain {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: stretch;
}

.architecture-runtime-arrow {
  display: none;
  align-items: center;
  justify-content: center;
  color: rgba(94, 234, 212, 0.82);
  font-size: 18px;
  font-weight: 700;
}

@media (max-width: 1200px) {
  .architecture-node-grid-three,
  .architecture-node-grid-four,
  .architecture-runtime-grid,
  .architecture-runtime-chain {
    grid-template-columns: 1fr;
  }

  .architecture-link {
    margin-inline: 4px;
  }

  .overview-loading-grid,
  .overview-layout {
    grid-template-columns: 1fr;
  }

  .overview-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .workbench-skeleton-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .workbench-release-detail-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    width: 100%;
    justify-content: flex-start;
  }
}

@media (max-width: 768px) {
  .overview-summary-grid,
  .application-workbench-columns,
  .workbench-skeleton-grid {
    grid-template-columns: 1fr;
  }

  .overview-chart-header {
    grid-template-columns: 1fr;
  }

  .overview-chart-meta {
    justify-content: flex-start;
    white-space: normal;
  }

  .application-workbench-card,
  .application-workbench-card-collapsed,
  .application-workbench-card-expanded {
    height: auto;
    min-height: 0;
  }

  .workbench-card-shell {
    position: relative;
    top: auto;
    right: auto;
    bottom: auto;
    left: auto;
    min-height: 0;
    height: auto;
  }

  .workbench-card-expanded {
    margin-top: 18px;
  }

  .workbench-card-expanded,
  .latest-release-panel,
  .env-release-view {
    min-height: unset;
  }

  .env-release-view {
    overflow: visible;
  }

  .workbench-release-env-list {
    max-height: none;
    overflow: visible;
  }

  .workbench-card-header,
  .workbench-template-strip {
    flex-direction: column;
    align-items: flex-start;
  }

  .workbench-card-title-row {
    width: 100%;
  }

  .workbench-baseline-table,
  .workbench-baseline-modules {
    grid-template-columns: 1fr;
  }

  .workbench-app-eyebrow {
    padding-right: 0;
  }

  .workbench-card-header-actions {
    position: static;
    z-index: auto;
  }

  .workbench-card-header-actions {
    width: 100%;
    justify-content: space-between;
  }

  .workbench-actions,
  .workbench-footer-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .pagination-area {
    justify-content: center;
  }

  :deep(.application-toolbar-project-select.ant-select) {
    width: 100%;
    min-width: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .workbench-card-shell-running,
  .workbench-card-shell-running::before,
  .workbench-card-shell-running::after {
    animation: none !important;
    transform: none !important;
  }
}

@media (min-width: 640px) {
  .flow-branch-grid {
    grid-template-columns: 1fr 1fr;
  }
}
</style>

<style>
.rollback-preview-confirm-modal .ant-modal-confirm-body {
  align-items: flex-start;
}

.rollback-preview-confirm-modal .ant-modal-confirm-paragraph {
  max-width: 100%;
}

.rollback-preview-confirm-modal .ant-modal-confirm-content {
  margin-top: 14px;
}

.rollback-preview-confirm-modal .ant-modal-body {
  padding: 24px;
}

.rollback-preview-modal {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.rollback-preview-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 16px;
  background:
    radial-gradient(circle at top right, rgba(251, 191, 36, 0.16), transparent 38%),
    linear-gradient(180deg, rgba(248, 250, 252, 0.98), rgba(255, 255, 255, 0.98));
}

.rollback-preview-hero-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 6px;
}

.rollback-preview-hero-title {
  color: #0f172a;
  font-size: 16px;
  font-weight: 700;
  line-height: 1.4;
}

.rollback-preview-hero-desc {
  color: #64748b;
  font-size: 13px;
  line-height: 1.7;
}

.rollback-preview-action-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  min-width: 88px;
  padding: 8px 14px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
}

.rollback-preview-action-badge-rollback {
  color: #9f1239;
  background: rgba(244, 114, 182, 0.14);
}

.rollback-preview-action-badge-replay {
  color: #92400e;
  background: rgba(245, 158, 11, 0.16);
}

.rollback-preview-summary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 16px;
  padding: 16px 18px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 16px;
  background: #f8fafc;
}

.rollback-preview-summary-item {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.rollback-preview-summary-label {
  min-width: 56px;
  color: #7c8597;
  font-size: 12px;
}

.rollback-preview-summary-value {
  color: #1f2937;
  font-weight: 600;
  word-break: break-all;
}

.rollback-preview-reason {
  padding: 12px 14px;
  border: 1px solid #ffe7ba;
  border-radius: 14px;
  background: #fff7e8;
  color: #ad6800;
  line-height: 1.7;
}

.rollback-preview-checks {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.rollback-preview-check-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid #e6ebf5;
  border-radius: 14px;
  background: #f8fafc;
}

.rollback-preview-check-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  color: #64748b;
  background: rgba(148, 163, 184, 0.16);
}

.rollback-preview-check-tag-pass {
  color: #166534;
  background: rgba(34, 197, 94, 0.14);
}

.rollback-preview-check-tag-warn {
  color: #92400e;
  background: rgba(245, 158, 11, 0.16);
}

.rollback-preview-check-tag-blocked {
  color: #b91c1c;
  background: rgba(239, 68, 68, 0.16);
}

.rollback-preview-check-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 4px;
}

.rollback-preview-check-title {
  color: #1f2937;
  font-size: 13px;
  font-weight: 600;
}

.rollback-preview-check-message {
  color: #64748b;
  font-size: 12px;
  line-height: 1.7;
}

.rollback-preview-scope {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.rollback-preview-scope-title {
  color: #1f2937;
  font-weight: 700;
}

.rollback-preview-param-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.rollback-preview-param-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid #e6ebf5;
  border-radius: 14px;
  background: #f8fafc;
}

.rollback-preview-param-key {
  font-size: 12px;
  color: #7c8597;
}

.rollback-preview-param-value {
  color: #1f2937;
  word-break: break-all;
}

.rollback-preview-empty {
  color: #7c8597;
  text-align: center;
}

@media (max-width: 768px) {
  .application-search-bar {
    max-width: 100%;
    width: 100%;
    align-items: stretch;
    flex-wrap: wrap;
    gap: 12px;
    padding: 14px;
  }

  .application-search-meta,
  .application-search-actions {
    width: 100%;
  }

  .application-search-actions {
    justify-content: flex-end;
  }

  .application-search-field {
    min-width: 100%;
  }

  .rollback-preview-hero,
  .rollback-preview-summary {
    grid-template-columns: 1fr;
  }

  .rollback-preview-hero {
    flex-direction: column;
  }

  .rollback-preview-param-list,
  .rollback-preview-summary {
    grid-template-columns: 1fr;
  }
}

.page-title-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.announcement-badge {
  position: absolute;
  top: -4px;
  right: -8px;
  min-width: 15px;
  height: 15px;
  padding: 0 4px;
  border-radius: 8px;
  background: #ef4444;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 15px;
  text-align: center;
  cursor: pointer;
  box-shadow: 0 1px 4px rgba(239, 68, 68, 0.4);
  white-space: nowrap;
}

.announcement-popover {
  z-index: 1060;
}

.announcement-popover .ant-popover-inner {
  width: 360px;
  max-width: 90vw;
  padding: 0;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 16px;
  overflow: hidden;
  background: #fff;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.12);
}

.announcement-popover .ant-popover-inner-content {
  padding: 0;
}

.announcement-popover .ant-popover-arrow {
  display: none;
}

.announcement-panel {
  display: flex;
  flex-direction: column;
}

.announcement-panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.5);
}

.announcement-head-icon {
  color: #64748b;
  font-size: 16px;
}

.announcement-head-title {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.announcement-list {
  display: flex;
  flex-direction: column;
  max-height: 280px;
  overflow-y: auto;
}

.announcement-item {
  padding: 14px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.5);
}

.announcement-item:last-of-type {
  border-bottom: none;
}

.announcement-item-title {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  margin-bottom: 6px;
}

.announcement-item-title::before {
  content: "";
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #ef4444;
  flex-shrink: 0;
}

.announcement-item-content {
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.announcement-empty {
  padding: 32px 18px;
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
}

.announcement-panel-foot {
  display: flex;
  justify-content: center;
  padding: 8px 18px 12px;
  border-top: 1px solid rgba(226, 232, 240, 0.5);
}

.announcement-read-btn {
  color: #94a3b8;
  font-size: 12px;
  padding: 0;
}

.announcement-read-btn:hover {
  color: #1d4ed8;
}
</style>
