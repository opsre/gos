<script setup lang="ts">
import { CaretRightOutlined, CopyOutlined, EyeOutlined, KeyOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { Modal, message } from 'ant-design-vue'
import type { TableColumnsType } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  createAgentTask,
  createAgent,
  deleteAgent,
  deleteAgentTask,
  disableAgent,
  executeAgentTask,
  enableAgent,
  getAgent,
  getAgentBootstrapConfig,
  getAgentConfig,
  listAgentScripts,
  listAllAgentTasks,
  listAgentTasks,
  listAgents,
  maintenanceAgent,
  resetAgentBootstrapToken,
  resetAgentToken,
  resumeAgentTask,
  stopAgentTask,
  updateAgentTask,
} from '../../api/agent'
import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useAuthStore } from '../../stores/auth'

echarts.use([BarChart, GridComponent, TooltipComponent, CanvasRenderer])
import type { AgentInstallConfig, AgentInstance, AgentListParams, AgentRuntimeState, AgentStatus, AgentTask, AgentTaskMode, AgentTaskType, UpsertAgentPayload, AgentScript } from '../../types/agent'
import type { CreateAgentTaskPayload } from '../../types/agent'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const authStore = useAuthStore()
const router = useRouter()
const AUTO_REFRESH_INTERVAL = 15000

const loading = ref(false)
const saving = ref(false)
const detailLoading = ref(false)
const configLoading = ref(false)
const bootstrapConfigLoading = ref(false)
const resettingBootstrapToken = ref(false)
const modalVisible = ref(false)
const detailVisible = ref(false)
const dispatchVisible = ref(false)
const dispatchLoading = ref(false)
const deletingAgentID = ref('')
const editingAgentID = ref('')
const dataSource = ref<AgentInstance[]>([])
const total = ref(0)
const detail = ref<AgentInstance | null>(null)
const installConfig = ref<AgentInstallConfig | null>(null)
const bootstrapConfig = ref<AgentInstallConfig | null>(null)
const taskLoading = ref(false)
const taskList = ref<AgentTask[]>([])
const selectedAgentIDs = ref<string[]>([])
const taskTemplateList = ref<AgentTask[]>([])
const selectedDispatchTaskID = ref('')
const boundAgentModalVisible = ref(false)
const currentBoundTask = ref<AgentTask | null>(null)
const scriptOptions = ref<AgentScript[]>([])
let autoRefreshTimer: number | null = null

// ---- ECharts stats (global overview, independent of filters) ----
const overviewChartRef = ref<HTMLElement | null>(null)
let overviewChart: echarts.ECharts | null = null
const overviewStatsLoading = ref(false)
const overviewStats = reactive({
  total: 0,
  online: 0,
  busy: 0,
  offline: 0,
  maintenance: 0,
  disabled: 0,
})

async function loadOverviewStats() {
  overviewStatsLoading.value = true
  try {
    const response = await listAgents({ page: 1, page_size: 1 })
    overviewStats.total = response.total
    const all = await listAgents({ page: 1, page_size: Math.max(response.total, 200) })
    overviewStats.online = all.data.filter((a) => a.runtime_state === 'online').length
    overviewStats.busy = all.data.filter((a) => a.runtime_state === 'busy').length
    overviewStats.offline = all.data.filter((a) => a.runtime_state === 'offline').length
    overviewStats.maintenance = all.data.filter((a) => a.runtime_state === 'maintenance' || a.status === 'maintenance').length
    overviewStats.disabled = all.data.filter((a) => a.status === 'disabled').length
    renderOverviewChart()
  } catch {
    // silent
  } finally {
    overviewStatsLoading.value = false
  }
}

function renderOverviewChart() {
  if (!overviewChartRef.value) return
  if (!overviewChart) {
    overviewChart = echarts.init(overviewChartRef.value)
  }
  const s = overviewStats
  const labels = ['在线', '执行中', '离线', '维护中', '已禁用']
  const values = [s.online, s.busy, s.offline, s.maintenance, s.disabled]
  const colors = ['#34d399', '#60a5fa', 'rgba(148,163,184,0.68)', '#fbbf24', 'rgba(148,163,184,0.42)']

  overviewChart.setOption({
    animationDuration: 420,
    animationEasing: 'cubicOut',
    grid: { top: 16, right: 8, bottom: 0, left: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(2,6,23,0.92)',
      borderColor: 'rgba(148,163,184,0.2)',
      borderWidth: 1,
      padding: [10, 12],
      textStyle: { color: '#e2e8f0', fontSize: 12 },
    },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: { color: 'rgba(226,232,240,0.56)', fontSize: 12, fontWeight: 600 },
      axisLine: { lineStyle: { color: 'rgba(71,85,105,0.32)' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: 'rgba(226,232,240,0.52)', fontSize: 11 },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: 'rgba(71,85,105,0.22)' } },
    },
    series: [{
      type: 'bar',
      data: values.map((v, i) => ({
        value: v,
        itemStyle: {
          color: colors[i],
          borderRadius: [6, 6, 0, 0],
        },
      })),
      barWidth: '34%',
    }],
  })
}

// ---- search overlay ----
interface SearchSuggestion { id: string; title: string; subtitle: string }
const searchVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchDraft = reactive({ keyword: '' })
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
let searchTimer: ReturnType<typeof window.setTimeout> | null = null
let searchRequestSeq = 0

function openSearchDialog() {
  searchDraft.keyword = filters.keyword
  searchVisible.value = true
  void nextTick(() => { searchInputRef.value?.focus() })
}

function closeSearchDialog() { searchVisible.value = false }

function resetSearchSuggestions() {
  if (searchTimer) { clearTimeout(searchTimer); searchTimer = null }
  searchRequestSeq += 1
  searchSuggestions.value = []
  searchSuggestionsLoading.value = false
}

async function loadSearchSuggestions(kw: string) {
  const reqSeq = ++searchRequestSeq
  searchSuggestionsLoading.value = true
  try {
    const response = await listAgents({ keyword: kw, page: 1, page_size: 6 })
    if (reqSeq !== searchRequestSeq) return
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: item.id,
      title: item.name,
      subtitle: `${item.agent_code} · ${item.hostname || item.host_ip || '-'}`,
    }))
  } catch {
    if (reqSeq !== searchRequestSeq) return
    searchSuggestions.value = []
  } finally {
    if (reqSeq === searchRequestSeq) searchSuggestionsLoading.value = false
  }
}

function handleSearchInput() {
  const kw = searchDraft.keyword.trim()
  if (searchTimer) clearTimeout(searchTimer)
  if (!kw) { resetSearchSuggestions(); return }
  searchTimer = setTimeout(() => { searchTimer = null; void loadSearchSuggestions(kw) }, 260)
}

function handleSearchSubmit() {
  filters.keyword = searchDraft.keyword.trim()
  filters.page = 1
  searchVisible.value = false
  resetSearchSuggestions()
  void loadAgents()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.title
  handleSearchSubmit()
}
// ---- modal masking (sidebar-aware) ----
const agentFormViewportInset = ref(0)

const agentFormMaskStyle = computed(() => ({
	left: `${agentFormViewportInset.value}px`,
	width: `calc(100% - ${agentFormViewportInset.value}px)`,
	background: 'rgba(15, 23, 42, 0.08)',
	backdropFilter: 'blur(10px)',
	WebkitBackdropFilter: 'blur(10px)',
	pointerEvents: modalVisible.value ? 'auto' : 'none',
}))

const agentFormWrapProps = computed(() => ({
	style: {
		left: `${agentFormViewportInset.value}px`,
		width: `calc(100% - ${agentFormViewportInset.value}px)`,
		pointerEvents: modalVisible.value ? 'auto' : 'none',
	},
}))

let agentFormViewportObserver: ResizeObserver | null = null

function readAgentFormViewportInset() {
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

function syncAgentFormViewportInset() {
	agentFormViewportInset.value = readAgentFormViewportInset()
}

function observeAgentFormViewportInset() {
	if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') return
	const appLayout = document.querySelector('.app-layout')
	const sider = document.querySelector('.app-sider')
	if (!appLayout && !sider) return
	agentFormViewportObserver?.disconnect()
	agentFormViewportObserver = new ResizeObserver(() => {
		syncAgentFormViewportInset()
	})
	if (appLayout) agentFormViewportObserver.observe(appLayout)
	if (sider) agentFormViewportObserver.observe(sider)
}

function stopObservingAgentFormViewportInset() {
	agentFormViewportObserver?.disconnect()
	agentFormViewportObserver = null
}


const filters = reactive<Required<AgentListParams>>({
  keyword: '',
  status: '',
  runtime_state: '',
  page: 1,
  page_size: 20,
})

const form = reactive<UpsertAgentPayload>({
  agent_code: '',
  name: '',
  environment_code: '',
  work_dir: '',
  tags: [],
  status: 'active',
  remark: '',
})
const tagsText = ref('')

const canManageAgent = computed(() => authStore.hasPermission('component.agent.manage'))
const canViewAgent = computed(() => canManageAgent.value || authStore.hasPermission('component.agent.view'))
const residentTasks = computed(() => taskList.value.filter((item) => item.task_mode === 'resident'))
const temporaryTasks = computed(() => taskList.value.filter((item) => item.task_mode !== 'resident'))
const selectedDispatchAgents = computed(() =>
  selectedAgentIDs.value
    .map((id) => dataSource.value.find((item) => item.id === id))
    .filter((item): item is AgentInstance => Boolean(item)),
)
const selectedDispatchTask = computed(() => taskTemplateList.value.find((item) => item.id === selectedDispatchTaskID.value) || null)
const dispatchTaskOptions = computed(() =>
  taskTemplateList.value.map((item) => ({
    value: item.id,
    label: `${item.name} · ${taskModeText(item.task_mode)} · ${item.agent_code || '未分配'}`,
  })),
)
const rowSelection = computed(() =>
  canManageAgent.value
    ? {
        selectedRowKeys: selectedAgentIDs.value,
        onChange: (keys: (string | number)[]) => {
          selectedAgentIDs.value = keys.map(String)
        },
      }
    : undefined,
)

const columns: TableColumnsType<AgentInstance> = [
  { title: 'Agent', dataIndex: 'name', key: 'name', width: 200 },
  { title: '编码', dataIndex: 'agent_code', key: 'agent_code', width: 150 },
  { title: '环境', dataIndex: 'environment_code', key: 'environment_code', width: 100 },
  { title: '主机', dataIndex: 'hostname', key: 'hostname', width: 180 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 140 },
  { title: '运行态', dataIndex: 'runtime_state', key: 'runtime_state', width: 120 },
  { title: '接单态', dataIndex: 'status', key: 'status', width: 120 },
  { title: '最后心跳', dataIndex: 'last_heartbeat_at', key: 'last_heartbeat_at', width: 190 },
  { title: '当前任务', dataIndex: 'current_task_name', key: 'current_task_name', width: 220 },
  { title: '当前常驻任务', dataIndex: 'current_resident_task_name', key: 'current_resident_task_name', width: 240 },
  { title: '最近结果', dataIndex: 'last_task_status', key: 'last_task_status', width: 120 },
  { title: '操作', key: 'actions', width: 260, fixed: 'right' },
]

function resetForm() {
  editingAgentID.value = ''
  form.agent_code = ''
  form.name = ''
  form.environment_code = ''
  form.work_dir = ''
  form.tags = []
  form.status = 'active'
  form.remark = ''
  tagsText.value = ''
}

function normalizeTagsText(raw: string) {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

async function loadAgents(options: { silent?: boolean } = {}) {
  if (!canViewAgent.value) {
    return
  }
  if (!options.silent) {
    loading.value = true
  }
  try {
    const response = await listAgents({
      keyword: filters.keyword.trim() || undefined,
      status: (filters.status || undefined) as AgentStatus | undefined,
      runtime_state: (filters.runtime_state || undefined) as AgentRuntimeState | undefined,
      page: filters.page,
      page_size: filters.page_size,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.page_size = response.page_size
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, 'Agent 列表加载失败'))
    }
  } finally {
    if (!options.silent) {
      loading.value = false
    }
  }
}

async function loadBootstrapConfig(options: { silent?: boolean } = {}) {
  if (!canManageAgent.value) {
    bootstrapConfig.value = null
    return
  }
  if (!options.silent) {
    bootstrapConfigLoading.value = true
  }
  try {
    const response = await getAgentBootstrapConfig()
    bootstrapConfig.value = response.data
  } catch (error) {
    bootstrapConfig.value = null
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, 'Agent 接入配置加载失败'))
    }
  } finally {
    if (!options.silent) {
      bootstrapConfigLoading.value = false
    }
  }
}

async function loadTasks(agentID: string, options: { silent?: boolean } = {}) {
  if (!agentID) {
    taskList.value = []
    return
  }
  if (!options.silent) {
    taskLoading.value = true
  }
  try {
    const response = await listAgentTasks(agentID, { page: 1, page_size: 20 })
    taskList.value = response.data
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, 'Agent 任务加载失败'))
    }
  } finally {
    if (!options.silent) {
      taskLoading.value = false
    }
  }
}

async function loadScriptOptions() {
  try {
    const response = await listAgentScripts({ page: 1, page_size: 200 })
    scriptOptions.value = response.data
  } catch {
    scriptOptions.value = []
  }
}

async function openDetail(record: AgentInstance) {
  detailVisible.value = true
  await loadDetail(record.id, { includeConfig: true })
  await loadTasks(record.id)
}

async function loadDetail(id: string, options: { silent?: boolean; includeConfig?: boolean } = {}) {
  if (!id) {
    return
  }
  if (!options.silent) {
    detailLoading.value = true
  }
  if (options.includeConfig) {
    configLoading.value = true
    installConfig.value = null
  }
  try {
    const requests: Promise<any>[] = [getAgent(id)]
    if (options.includeConfig) {
      requests.push(getAgentConfig(id))
    }
    const [detailResponse, configResponse] = await Promise.all(requests)
    detail.value = detailResponse.data
    if (options.includeConfig && configResponse) {
      installConfig.value = configResponse.data
    }
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, 'Agent 详情加载失败'))
    }
  } finally {
    if (!options.silent) {
      detailLoading.value = false
    }
    if (options.includeConfig) {
      configLoading.value = false
    }
  }
}

async function runAutoRefresh() {
  if (document.hidden || !canViewAgent.value) {
    return
  }
  await loadAgents({ silent: true })
  if (canManageAgent.value) {
    await loadBootstrapConfig({ silent: true })
  }
  if (detailVisible.value && detail.value?.id) {
    await loadDetail(detail.value.id, { silent: true })
    await loadTasks(detail.value.id, { silent: true })
  }
}

function startAutoRefresh() {
  if (autoRefreshTimer !== null) {
    window.clearInterval(autoRefreshTimer)
  }
  autoRefreshTimer = window.setInterval(() => {
    void runAutoRefresh()
  }, AUTO_REFRESH_INTERVAL)
}

function stopAutoRefresh() {
  if (autoRefreshTimer !== null) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function openCreate() {
  resetForm()
  syncAgentFormViewportInset()
  modalVisible.value = true
}

function openEdit(record: AgentInstance) {
  editingAgentID.value = record.id
  form.agent_code = record.agent_code
  form.name = record.name
  form.environment_code = record.environment_code || ''
  form.work_dir = record.work_dir
  form.tags = [...(record.tags || [])]
  form.status = record.status
  form.remark = record.remark || ''
  tagsText.value = (record.tags || []).join(', ')
  syncAgentFormViewportInset()
  modalVisible.value = true
}

function closeModal() {
  modalVisible.value = false
  resetForm()
}

function closeDetail() {
  detailVisible.value = false
  detail.value = null
  installConfig.value = null
  taskList.value = []
}

function goToTaskManagement() {
  void router.push('/components/agent-tasks')
}

async function openDispatchModal() {
  if (!selectedAgentIDs.value.length) {
    message.info('请先勾选要下发的 Agent')
    return
  }
  dispatchVisible.value = true
  dispatchLoading.value = true
  try {
    const response = await listAllAgentTasks({ page: 1, page_size: 300 })
    taskTemplateList.value = response.data.filter((item) => !item.agent_id)
    if (selectedDispatchTaskID.value && !taskTemplateList.value.some((item) => item.id === selectedDispatchTaskID.value)) {
      selectedDispatchTaskID.value = ''
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '任务模板加载失败'))
  } finally {
    dispatchLoading.value = false
  }
}

function closeDispatchModal() {
  dispatchVisible.value = false
  selectedDispatchTaskID.value = ''
  taskTemplateList.value = []
}

async function handleDispatchTask() {
  if (!selectedAgentIDs.value.length) {
    message.warning('请先选择目标 Agent')
    return
  }
  if (!selectedDispatchTask.value) {
    message.warning('请选择要下发的任务')
    return
  }
  dispatchLoading.value = true
  try {
    const source = selectedDispatchTask.value
    const payload = {
      name: source.name,
      task_mode: source.task_mode,
      task_type: source.task_type,
      shell_type: source.shell_type,
      work_dir: source.work_dir,
      script_id: source.script_id,
      script_path: source.script_path,
      script_text: source.script_text,
      variables: source.variables,
      timeout_sec: source.timeout_sec,
    }
    const results = await Promise.allSettled(selectedAgentIDs.value.map((agentID) => createAgentTask(agentID, payload)))
    const failed = results.filter((item) => item.status === 'rejected') as PromiseRejectedResult[]
    if (failed.length) {
      throw new Error(extractHTTPErrorMessage(failed[0].reason, '任务下发失败'))
    }
    message.success(`任务已创建到 ${selectedAgentIDs.value.length} 台 Agent，点击执行后才会开始领取`)
    closeDispatchModal()
    await loadAgents({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, error instanceof Error ? error.message : '任务下发失败'))
  } finally {
    dispatchLoading.value = false
  }
}

async function handleSave() {
  saving.value = true
  const isEditing = Boolean(editingAgentID.value)
  try {
    const payload: UpsertAgentPayload = {
      agent_code: form.agent_code,
      name: form.name,
      environment_code: form.environment_code,
      work_dir: form.work_dir,
      tags: normalizeTagsText(tagsText.value),
      status: form.status,
      remark: form.remark,
    }
    const response = editingAgentID.value
      ? await updateAgent(editingAgentID.value, payload)
      : await createAgent(payload)
    closeModal()
    await loadAgents()
    detail.value = response.data
    message.success(isEditing ? 'Agent 已更新' : 'Agent 已创建')
    if (!isEditing) {
      message.info('现在统一使用平台接入 Token 自动注册，部署时直接复制接入配置即可')
      await openDetail(response.data)
    } else if (detailVisible.value && detail.value?.id === response.data.id) {
      await openDetail(response.data)
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, isEditing ? 'Agent 更新失败' : 'Agent 创建失败'))
  } finally {
    saving.value = false
  }
}

async function handleChangeStatus(record: AgentInstance, target: AgentStatus) {
  try {
    if (target === 'active') {
      await enableAgent(record.id)
    } else if (target === 'disabled') {
      await disableAgent(record.id)
    } else {
      await maintenanceAgent(record.id)
    }
    message.success('Agent 状态已更新')
    await loadAgents()
    if (detailVisible.value && detail.value?.id === record.id) {
      await openDetail(record)
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Agent 状态更新失败'))
  }
}

function handleDeleteAgent(record: AgentInstance) {
  Modal.confirm({
    title: '删除 Agent',
    content: `删除后会移除 Agent 实例本身，常驻任务会一并清理，历史临时任务会保留日志但不再绑定这台 Agent此操作不可恢复，确认继续吗？`,
    okText: '确认删除',
    cancelText: '取消',
    okButtonProps: { danger: true },
    async onOk() {
      deletingAgentID.value = record.id
      try {
        await deleteAgent(record.id)
        selectedAgentIDs.value = selectedAgentIDs.value.filter((item) => item !== record.id)
        if (detailVisible.value && detail.value?.id === record.id) {
          closeDetail()
        }
        await loadAgents()
        message.success('Agent 已删除')
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, 'Agent 删除失败'))
      } finally {
        deletingAgentID.value = ''
      }
    },
  })
}

function runtimeTagColor(state: AgentRuntimeState) {
  switch (state) {
    case 'online':
      return 'green'
    case 'busy':
      return 'blue'
    case 'maintenance':
      return 'gold'
    case 'disabled':
      return 'default'
    default:
      return 'red'
  }
}

function runtimeText(state: AgentRuntimeState) {
  switch (state) {
    case 'online':
      return '在线'
    case 'busy':
      return '执行中'
    case 'maintenance':
      return '维护中'
    case 'disabled':
      return '已禁用'
    default:
      return '离线'
  }
}

function statusTagColor(status: AgentStatus) {
  switch (status) {
    case 'active':
      return 'green'
    case 'maintenance':
      return 'gold'
    default:
      return 'default'
  }
}

function statusText(status: AgentStatus) {
  switch (status) {
    case 'active':
      return '可接单'
    case 'maintenance':
      return '维护中'
    default:
      return '已禁用'
  }
}

function taskTypeText(taskType: AgentTaskType) {
  switch (taskType) {
    case 'shell_task':
      return 'Shell 脚本'
    case 'script_file_task':
      return '脚本文件任务'
    case 'file_distribution_task':
      return '文件下发任务'
    default:
      return taskType
  }
}

function taskModeText(taskMode?: AgentTaskMode) {
  return taskMode === 'resident' ? '常驻任务' : '临时任务'
}

function getTemporaryTaskAgentText(task: AgentTask) {
  const targetAgentIDs = task.target_agent_ids || []
  if (!targetAgentIDs.length) {
    return '未绑定'
  }
  return `绑定 ${targetAgentIDs.length} 台 Agent`
}

function showBoundAgentsModal(task: AgentTask) {
  currentBoundTask.value = task
  boundAgentModalVisible.value = true
}

function getBoundAgentList(task: AgentTask | null) {
  if (!task) return []
  const targetAgentIDs = task.target_agent_ids || []
  return targetAgentIDs
    .map((id) => dataSource.value.find((item) => item.id === id))
    .filter((item): item is AgentInstance => Boolean(item))
}

function residentRuntimeText(task: AgentTask) {
  if (task.status === 'running') {
    return '执行中'
  }
  if (task.status === 'queued') {
    return '排队中'
  }
  if (task.status === 'claimed') {
    return '准备执行'
  }
  if (task.status === 'cancelled') {
    if (task.started_at && (!task.finished_at || new Date(task.finished_at).getTime() < new Date(task.started_at).getTime())) {
      return '停止中'
    }
    return '已停止'
  }
  if ((task.run_count || 0) > 0) {
    return '待下一轮'
  }
  return '待首次执行'
}

function residentRuntimeColor(task: AgentTask) {
  if (task.status === 'running') {
    return 'blue'
  }
  if (task.status === 'queued') {
    return 'orange'
  }
  if (task.status === 'claimed') {
    return 'gold'
  }
  if (task.status === 'cancelled') {
    return 'default'
  }
  return 'green'
}

function residentSuccessPercent(task: AgentTask) {
  if (!task.run_count) {
    return 0
  }
  return Math.max(0, Math.min(100, Math.round((task.success_count / task.run_count) * 100)))
}

function taskStatusColor(status: AgentTask['status']) {
  switch (status) {
    case 'draft':
      return 'cyan'
    case 'success':
      return 'green'
    case 'failed':
      return 'red'
    case 'running':
      return 'blue'
    case 'queued':
      return 'orange'
    case 'claimed':
      return 'gold'
    case 'cancelled':
      return 'default'
    default:
      return 'default'
  }
}

function taskStatusText(status: AgentTask['status']) {
  switch (status) {
    case 'draft':
      return '待执行'
    case 'pending':
      return '待领取'
    case 'queued':
      return '排队中'
    case 'claimed':
      return '已领取'
    case 'running':
      return '执行中'
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'cancelled':
      return '已取消'
    default:
      return status
  }
}

function lastRunStatusText(status: AgentTask['last_run_status']) {
  return status ? taskStatusText(status) : '尚未执行'
}

function lastTaskTagColor(status: AgentInstance['last_task_status']) {
  switch (status) {
    case 'success':
      return 'green'
    case 'failed':
      return 'red'
    case 'running':
      return 'blue'
    case 'cancelled':
      return 'default'
    default:
      return 'default'
  }
}

function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

async function copyText(value: string, successText: string) {
  if (!value) {
    message.warning('暂无可复制内容')
    return
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      const input = document.createElement('textarea')
      input.value = value
      input.setAttribute('readonly', 'readonly')
      input.style.position = 'fixed'
      input.style.opacity = '0'
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
    }
    message.success(successText)
  } catch {
    message.error('复制失败，请手动复制')
  }
}

function copyConfigYAML(configYAML?: string) {
  void copyText(configYAML || '', '配置文件已复制')
}

function maskConfigYAML(yaml?: string): string {
  if (!yaml) return ''
  return yaml.replace(/(registration_token\s*:\s*).+/g, '$1*****')
}

function copyInstallCommand() {
  if (!bootstrapConfig.value) return
  const cmd = `wget -qO- https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/install_gos_agent.sh | sudo bash -s -- \\
  --server-url ${bootstrapConfig.value.resolved_server_url} \\
  --token ${bootstrapConfig.value.registration_token} \\
  --work-dir /etc/gos-agent \\
  --name prod-xxxx \\
  --tags production,web`
  void copyText(cmd, '一键安装命令已复制（包含 Token）')
}

function canExecuteTemporaryTask(task: AgentTask) {
  return task.task_mode !== 'resident' && ['draft', 'success', 'failed', 'cancelled'].includes(String(task.status || ''))
}

function executeActionText(task: AgentTask) {
  return task.run_count > 0 ? '重新执行' : '执行'
}

async function handleResetBootstrapToken() {
  Modal.confirm({
    title: '重置接入 Token',
    content: '重置后旧 Token 立即失效，所有使用旧 Token 的节点将无法注册。确认继续吗？',
    okText: '确认重置',
    cancelText: '取消',
    okButtonProps: { danger: true },
    async onOk() {
      resettingBootstrapToken.value = true
      try {
        const response = await resetAgentBootstrapToken()
        bootstrapConfig.value = response.data
        if (detailVisible.value && detail.value?.id) {
          await loadDetail(detail.value.id, { silent: true, includeConfig: true })
        }
        message.success('接入 Token 已重置，后续新部署请使用新的配置文件')
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '接入 Token 重置失败'))
      } finally {
        resettingBootstrapToken.value = false
      }
    },
  })
}

async function handleExecuteTemporaryTask(task: AgentTask) {
  if (!detail.value) {
    return
  }
  try {
    await executeAgentTask(detail.value.id, task.id)
    message.success(task.run_count > 0 ? '任务已重新进入执行队列' : '任务已进入执行队列')
    await loadTasks(detail.value.id)
    await loadDetail(detail.value.id, { silent: true })
    await loadAgents({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '任务执行失败'))
  }
}

async function handleStopResidentTask(task: AgentTask) {
  if (!detail.value) {
    return
  }
  Modal.confirm({
    title: '停止常驻任务',
    content: '停止后该任务将不再被 Agent 循环领取；如果当前这一轮正在执行，会在本轮结束后彻底停止',
    okText: '确认停止',
    cancelText: '取消',
    async onOk() {
      try {
        await stopAgentTask(detail.value!.id, task.id)
        await loadTasks(detail.value!.id)
        await loadDetail(detail.value!.id, { silent: true })
        await loadAgents({ silent: true })
        message.success('常驻任务已停止')
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '常驻任务停止失败'))
      }
    },
  })
}

async function handleResumeResidentTask(task: AgentTask) {
  if (!detail.value) {
    return
  }
  try {
    await resumeAgentTask(detail.value.id, task.id)
    await loadTasks(detail.value.id)
    await loadDetail(detail.value.id, { silent: true })
    await loadAgents({ silent: true })
    message.success('常驻任务已重新启用')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '常驻任务重新启用失败'))
  }
}

async function handleDeleteResidentTask(task: AgentTask) {
  if (!detail.value) {
    return
  }
  Modal.confirm({
    title: '删除常驻任务',
    content: '删除后该常驻任务会从当前 Agent 中移除，无法继续自动执行此操作不可恢复，确认继续吗？',
    okText: '确认删除',
    cancelText: '取消',
    okButtonProps: { danger: true },
    async onOk() {
      try {
        await deleteAgentTask(detail.value!.id, task.id)
        await loadTasks(detail.value!.id)
        await loadDetail(detail.value!.id, { silent: true })
        await loadAgents({ silent: true })
        message.success('常驻任务已删除')
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '常驻任务删除失败'))
      }
    },
  })
}

const editResidentTaskVisible = ref(false)
const editResidentTaskSaving = ref(false)
const editResidentTaskForm = reactive<CreateAgentTaskPayload>({
  name: '',
  task_mode: 'resident',
  task_type: 'shell_task',
  shell_type: 'sh',
  work_dir: '',
  script_id: '',
  script_path: '',
  script_text: '',
  variables: {},
  timeout_sec: 300,
})
const editResidentTaskID = ref('')

function handleEditResidentTask(task: AgentTask) {
  editResidentTaskID.value = task.id
  editResidentTaskForm.name = task.name
  editResidentTaskForm.task_mode = task.task_mode
  editResidentTaskForm.task_type = task.task_type
  editResidentTaskForm.shell_type = task.shell_type
  editResidentTaskForm.work_dir = task.work_dir
  editResidentTaskForm.script_id = task.script_id
  editResidentTaskForm.script_path = task.script_path
  editResidentTaskForm.script_text = task.script_text
  editResidentTaskForm.variables = { ...task.variables }
  editResidentTaskForm.timeout_sec = task.timeout_sec
  editResidentTaskVisible.value = true
}

async function handleSaveEditResidentTask() {
  if (!detail.value) return
  editResidentTaskSaving.value = true
  try {
    if (!editResidentTaskForm.script_id) {
      throw new Error('请选择脚本')
    }
    await updateAgentTask(detail.value.id, editResidentTaskID.value, editResidentTaskForm)
    message.success('常驻任务已更新')
    editResidentTaskVisible.value = false
    await loadTasks(detail.value.id)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '更新任务失败'))
  } finally {
    editResidentTaskSaving.value = false
  }
}

onMounted(() => {
  syncAgentFormViewportInset()
  observeAgentFormViewportInset()
  void loadAgents()
  void loadBootstrapConfig()
  void loadScriptOptions()
  void loadOverviewStats()
  startAutoRefresh()
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  stopObservingAgentFormViewportInset()
  overviewChart?.dispose()
  overviewChart = null
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header">
      <div class="page-header-copy">
        <h2 class="page-title">节点</h2>
      </div>
      <div class="page-header-actions">
        <a-button class="agent-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon><SearchOutlined /></template>
        </a-button>
        <a-button v-if="canManageAgent" class="agent-toolbar-action-btn" @click="openDispatchModal">
          下发任务
        </a-button>
        <a-button v-if="canViewAgent" class="agent-toolbar-action-btn" @click="goToTaskManagement">
          <template #icon><EyeOutlined /></template>
          任务管理
        </a-button>
        <a-button v-if="canManageAgent" class="agent-toolbar-action-btn agent-toolbar-action-btn--primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          新增 Agent
        </a-button>
      </div>
    </div>

    <transition name="agent-search-fade">
      <div v-if="searchVisible" class="agent-search-overlay" @click.self="closeSearchDialog">
        <div class="agent-search-floating-panel">
          <div class="agent-search-floating-input">
            <SearchOutlined class="agent-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="agent-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="搜索编码 / 名称 / 主机 / IP"
              @input="handleSearchInput"
              @keydown.enter="handleSearchSubmit"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="agent-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="agent-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="agent-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="agent-search-suggestion-title">{{ item.title }}</span>
                <span class="agent-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <section class="overview-chart-card">
      <div class="overview-chart-head">
        <div class="overview-chart-copy">
          <div class="overview-chart-label">节点统计</div>
          <div class="overview-chart-title">全部节点运行态分布</div>
        </div>
        <div class="overview-chart-pill">共 {{ overviewStats.total }} 台</div>
      </div>
      <div ref="overviewChartRef" class="overview-chart-canvas"></div>
      <div class="overview-chart-footnote">统计口径：汇总全部节点运行态数量，不跟随筛选条件变动</div>
    </section>

    <a-card v-if="canManageAgent" class="filter-card bootstrap-card" :bordered="false">
      <div class="bootstrap-card-head">
        <div>
          <div class="task-panel-title">快速配置</div>
        </div>
        <a-space>
          <a-button v-if="bootstrapConfig" class="agent-toolbar-action-btn" @click="copyInstallCommand">
            <template #icon><CopyOutlined /></template>
            复制安装命令
          </a-button>
          <a-button class="agent-toolbar-action-btn" @click="copyConfigYAML(bootstrapConfig?.config_yaml)">
            <template #icon><CopyOutlined /></template>
            复制配置
          </a-button>
          <a-button class="agent-toolbar-action-btn" :loading="resettingBootstrapToken" @click="handleResetBootstrapToken">
            <template #icon><KeyOutlined /></template>
            重置接入 Token
          </a-button>
        </a-space>
      </div>
      <a-spin :spinning="bootstrapConfigLoading">
        <div class="config-meta">
          <div class="config-col">
            <div class="config-label">一键安装</div>
            <pre v-if="bootstrapConfig" class="config-preview">wget -qO- https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/install_gos_agent.sh | sudo bash -s -- \
  --server-url {{ bootstrapConfig.resolved_server_url }} \
  --token [已隐藏，点击上方复制按钮获取完整命令] \
  --work-dir /etc/gos-agent \
  --name prod-xxxx \
  --tags production,web</pre>
            <pre v-else class="config-preview">接入配置生成中…</pre>
          </div>
          <div class="config-col">
            <div class="config-label">启动命令</div>
            <pre class="config-preview">{{ bootstrapConfig?.launch_command ? 'nohup ' + bootstrapConfig.launch_command + ' > agent.log 2>&1 &' : '-' }}</pre>
          </div>
          <div class="config-col config-col--yaml">
            <div class="config-label">配置文件</div>
            <pre class="config-preview">{{ maskConfigYAML(bootstrapConfig?.config_yaml) || '接入配置生成中…' }}</pre>
          </div>
        </div>
      </a-spin>
    </a-card>

    <a-card class="table-card" :bordered="false">
      <a-table
        row-key="id"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :row-selection="rowSelection"
        :pagination="{
          current: filters.page,
          pageSize: filters.page_size,
          total,
          showSizeChanger: true,
          pageSizeOptions: ['10', '20', '50', '100'],
          onChange: (page: number, pageSize: number) => {
            filters.page = page
            filters.page_size = pageSize
            loadAgents()
          },
          onShowSizeChange: (_current: number, size: number) => {
            filters.page = 1
            filters.page_size = size
            loadAgents()
          },
        }"
        :scroll="{ x: 1450 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <div class="agent-primary">
              <div class="agent-name">{{ record.name }}</div>
              <div class="agent-meta">{{ record.work_dir }}</div>
            </div>
          </template>
          <template v-else-if="column.key === 'runtime_state'">
            <a-tag :color="runtimeTagColor(record.runtime_state)">{{ runtimeText(record.runtime_state) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusTagColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'last_heartbeat_at'">
            <div>{{ formatTime(record.last_heartbeat_at) }}</div>
            <div class="muted-text">{{ record.heartbeat_age_sec ? `${record.heartbeat_age_sec}s 前` : '尚未上报' }}</div>
          </template>
          <template v-else-if="column.key === 'current_task_name'">
            <span v-if="record.current_task_id">{{ record.current_task_name || record.current_task_id }}</span>
            <span v-else class="muted-text">当前空闲</span>
          </template>
          <template v-else-if="column.key === 'current_resident_task_name'">
            <a-space v-if="record.current_resident_task_id" size="small">
              <span>{{ record.current_resident_task_name || record.current_resident_task_id }}</span>
              <a-tag :color="taskStatusColor(record.current_resident_task_status || 'pending')">
                {{ taskStatusText(record.current_resident_task_status || 'pending') }}
              </a-tag>
            </a-space>
            <span v-else class="muted-text">未分配常驻任务</span>
          </template>
          <template v-else-if="column.key === 'last_task_status'">
            <a-tag :color="lastTaskTagColor(record.last_task_status)">{{ record.last_task_status || 'unknown' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-button type="link" @click="openDetail(record)">
                <template #icon><EyeOutlined /></template>
                查看
              </a-button>
              <a-button v-if="canManageAgent" type="link" @click="openEdit(record)">编辑</a-button>
              <a-button
                v-if="canManageAgent"
                type="link"
                danger
                :loading="deletingAgentID === record.id"
                @click="handleDeleteAgent(record)"
              >
                删除
              </a-button>
              <a-dropdown v-if="canManageAgent">
                <a class="ant-dropdown-link" @click.prevent>状态</a>
                <template #overlay>
                  <a-menu>
                    <a-menu-item @click="handleChangeStatus(record, 'active')">设为可接单</a-menu-item>
                    <a-menu-item @click="handleChangeStatus(record, 'maintenance')">设为维护中</a-menu-item>
                    <a-menu-item @click="handleChangeStatus(record, 'disabled')">设为已禁用</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="detailVisible" width="720" title="Agent 详情" @close="closeDetail">
      <a-spin :spinning="detailLoading">
        <template v-if="detail">
          <div class="detail-grid">
            <div class="detail-item">
              <div class="detail-label">Agent</div>
              <div class="detail-value">{{ detail.name }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">编码</div>
              <div class="detail-value">{{ detail.agent_code }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">环境</div>
              <div class="detail-value">{{ detail.environment_code || '-' }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">工作目录</div>
              <div class="detail-value">{{ detail.work_dir }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">主机名</div>
              <div class="detail-value">{{ detail.hostname || '-' }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">主机 IP</div>
              <div class="detail-value">{{ detail.host_ip || '-' }}</div>
            </div>
            <div class="detail-item">
              <div class="detail-label">运行态</div>
              <div class="detail-value"><a-tag :color="runtimeTagColor(detail.runtime_state)">{{ runtimeText(detail.runtime_state) }}</a-tag></div>
            </div>
            <div class="detail-item">
              <div class="detail-label">接单态</div>
              <div class="detail-value"><a-tag :color="statusTagColor(detail.status)">{{ statusText(detail.status) }}</a-tag></div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">当前任务</div>
              <div class="detail-value">{{ detail.current_task_name || detail.current_task_id || '当前空闲' }}</div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">当前常驻任务</div>
              <div class="detail-value">
                <a-space v-if="detail.current_resident_task_id" size="small">
                  <span>{{ detail.current_resident_task_name || detail.current_resident_task_id }}</span>
                  <a-tag :color="taskStatusColor(detail.current_resident_task_status || 'pending')">
                    {{ taskStatusText(detail.current_resident_task_status || 'pending') }}
                  </a-tag>
                </a-space>
                <span v-else class="muted-text">未分配常驻任务</span>
              </div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">最近结果</div>
              <div class="detail-value">{{ detail.last_task_status }} {{ detail.last_task_summary ? `· ${detail.last_task_summary}` : '' }}</div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">标签</div>
              <div class="detail-value">
                <a-space wrap>
                  <a-tag v-for="item in detail.tags" :key="item">{{ item }}</a-tag>
                  <span v-if="!detail.tags.length" class="muted-text">暂无标签</span>
                </a-space>
              </div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">接入方式</div>
              <div class="detail-value token-row">
                <span class="muted-text">当前 Agent 使用平台统一接入 Token 自动注册，运行凭据由系统维护，无需手工配置单机 Token</span>
              </div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label">安装配置</div>
              <div class="config-card">
                <div class="config-meta">
                  <div>
                    <div class="config-label">建议路径</div>
                    <div class="config-value">{{ installConfig?.suggested_path || '-' }}</div>
                  </div>
                  <div>
                    <div class="config-label">启动命令</div>
                    <div class="config-value">{{ installConfig?.launch_command || '-' }}</div>
                  </div>
                </div>
                <a-spin :spinning="configLoading">
                  <pre class="config-preview">{{ maskConfigYAML(installConfig?.config_yaml) || '配置生成中…' }}</pre>
                </a-spin>
                <div class="muted-text">这份配置会带上平台接入 Token，并自动回填当前 Agent 的名称、环境和工作目录</div>
                <div class="config-actions">
                  <a-button size="small" @click="copyConfigYAML(installConfig?.config_yaml)">
                    <template #icon><CopyOutlined /></template>
                    复制配置文件
                  </a-button>
                </div>
              </div>
            </div>
            <div class="detail-item detail-item-full">
              <div class="detail-label tasks-header">
                <span>最近任务</span>
                <a-space>
                  <a-button size="small" @click="detail?.id && loadTasks(detail.id)">
                    <template #icon><ReloadOutlined /></template>
                    刷新任务
                  </a-button>
                  <a-button size="small" @click="goToTaskManagement">
                    前往任务管理
                  </a-button>
                </a-space>
              </div>
              <a-spin :spinning="taskLoading">
                <div class="task-sections" v-if="taskList.length">
                  <div class="task-section">
                    <div class="task-section-title">常驻任务</div>
                    <div class="task-list" v-if="residentTasks.length">
                      <div v-for="item in residentTasks" :key="item.id" class="task-item resident-task-item">
                        <div class="task-item-head">
                          <div>
                            <div class="task-name-row">
                              <div class="task-name">{{ item.name }}</div>
                              <a-tag color="purple">{{ taskModeText(item.task_mode) }}</a-tag>
                              <a-tag>{{ taskTypeText(item.task_type) }}</a-tag>
                            </div>
                            <div class="muted-text">{{ item.script_name || '未命名脚本' }} · {{ formatTime(item.created_at) }}</div>
                          </div>
                          <a-space>
                            <a-tag :color="residentRuntimeColor(item)">{{ residentRuntimeText(item) }}</a-tag>
                            <a-button
                              v-if="canManageAgent && item.status !== 'running' && item.status !== 'claimed'"
                              size="small"
                              @click="handleEditResidentTask(item)"
                            >
                              编辑
                            </a-button>
                            <a-button
                              v-if="canManageAgent && item.status !== 'cancelled'"
                              size="small"
                              danger
                              @click="handleStopResidentTask(item)"
                            >
                              停止
                            </a-button>
                            <a-button
                              v-if="canManageAgent && item.status === 'cancelled'"
                              size="small"
                              type="primary"
                              @click="handleResumeResidentTask(item)"
                            >
                              重新启用
                            </a-button>
                            <a-button
                              v-if="canManageAgent && item.status !== 'running' && item.status !== 'claimed'"
                              size="small"
                              danger
                              @click="handleDeleteResidentTask(item)"
                            >
                              删除
                            </a-button>
                          </a-space>
                        </div>
                        <div class="task-meta">
                          <span>目录：{{ item.work_dir }}</span>
                          <span>脚本：{{ item.script_name || item.script_path || '-' }}</span>
                          <span>超时：{{ item.timeout_sec }}s</span>
                          <span>最近结果：{{ lastRunStatusText(item.last_run_status) }}</span>
                        </div>
                        <div class="resident-progress-card">
                          <div class="resident-progress-head">
                            <span>执行进度</span>
                            <span class="muted-text">{{ item.run_count ? `累计 ${item.run_count} 次` : '尚未执行' }}</span>
                          </div>
                          <a-progress :percent="residentSuccessPercent(item)" :show-info="false" size="small" />
                          <div class="resident-progress-stats">
                            <span>成功 {{ item.success_count }}</span>
                            <span>失败 {{ item.failure_count }}</span>
                            <span v-if="item.finished_at">最近结束 {{ formatTime(item.finished_at) }}</span>
                          </div>
                        </div>
                        <div class="task-meta task-meta-secondary">
                          <span v-if="item.claimed_at">领取：{{ formatTime(item.claimed_at) }}</span>
                          <span v-if="item.started_at">开始：{{ formatTime(item.started_at) }}</span>
                          <span v-if="item.finished_at">结束：{{ formatTime(item.finished_at) }}</span>
                        </div>
                        <div v-if="item.last_run_summary" class="task-summary">{{ item.last_run_summary }}</div>
                        <div v-if="item.failure_reason" class="task-error">{{ item.failure_reason }}</div>
                        <div v-if="item.stdout_text || item.stderr_text" class="task-output-grid">
                          <div v-if="item.stdout_text" class="task-output-card">
                            <div class="task-output-title">标准输出</div>
                            <pre class="task-log">{{ item.stdout_text }}</pre>
                          </div>
                          <div v-if="item.stderr_text" class="task-output-card task-output-card-error">
                            <div class="task-output-title">标准错误</div>
                            <pre class="task-log task-log-error">{{ item.stderr_text }}</pre>
                          </div>
                        </div>
                        <div v-else class="muted-text task-empty-output">暂无执行回显</div>
                      </div>
                    </div>
                    <a-empty v-else description="暂无常驻任务" />
                  </div>

                  <div class="task-section">
                    <div class="task-section-title">临时任务</div>
                    <div class="task-list" v-if="temporaryTasks.length">
                      <div v-for="item in temporaryTasks" :key="item.id" class="task-item">
                        <div class="task-item-head">
                          <div>
                            <div class="task-name-row">
                              <div class="task-name">{{ item.name }}</div>
                              <a-tag>{{ taskModeText(item.task_mode) }}</a-tag>
                              <a-tag>{{ taskTypeText(item.task_type) }}</a-tag>
                            </div>
                            <div class="muted-text">{{ item.script_name || item.script_path || '-' }} · {{ formatTime(item.created_at) }}</div>
                            <div v-if="item.target_agent_ids && item.target_agent_ids.length" class="muted-text">
                              <a class="agent-link" @click="showBoundAgentsModal(item)">
                                {{ getTemporaryTaskAgentText(item) }}
                              </a>
                            </div>
                          </div>
                          <a-space>
                            <a-button
                              v-if="canManageAgent && canExecuteTemporaryTask(item)"
                              type="link"
                              size="small"
                              @click="handleExecuteTemporaryTask(item)"
                            >
                              <template #icon><CaretRightOutlined /></template>
                              {{ executeActionText(item) }}
                            </a-button>
                            <a-tag :color="taskStatusColor(item.status)">{{ taskStatusText(item.status) }}</a-tag>
                          </a-space>
                        </div>
                        <div class="task-meta">
                          <span>目录：{{ item.work_dir }}</span>
                          <span>超时：{{ item.timeout_sec }}s</span>
                          <span>退出码：{{ item.exit_code }}</span>
                        </div>
                        <div class="task-meta task-meta-secondary">
                          <span v-if="item.claimed_at">领取：{{ formatTime(item.claimed_at) }}</span>
                          <span v-if="item.started_at">开始：{{ formatTime(item.started_at) }}</span>
                          <span v-if="item.finished_at">结束：{{ formatTime(item.finished_at) }}</span>
                        </div>
                        <div v-if="item.failure_reason" class="task-error">{{ item.failure_reason }}</div>
                        <div v-if="item.stdout_text || item.stderr_text" class="task-output-grid">
                          <div v-if="item.stdout_text" class="task-output-card">
                            <div class="task-output-title">标准输出</div>
                            <pre class="task-log">{{ item.stdout_text }}</pre>
                          </div>
                          <div v-if="item.stderr_text" class="task-output-card task-output-card-error">
                            <div class="task-output-title">标准错误</div>
                            <pre class="task-log task-log-error">{{ item.stderr_text }}</pre>
                          </div>
                        </div>
                        <div v-else class="muted-text task-empty-output">暂无执行回显</div>
                      </div>
                    </div>
                    <a-empty v-else description="暂无临时任务" />
                  </div>
                </div>
                <a-empty v-else description="暂无任务记录" />
              </a-spin>
            </div>
          </div>
        </template>
      </a-spin>
    </a-drawer>

    <a-modal
      :open="modalVisible"
      :width="640"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="() => { closeModal(); saving = false; }"
      :mask-style="agentFormMaskStyle"
      :wrap-props="agentFormWrapProps"
      wrap-class-name="agent-form-modal-wrap"
      @cancel="closeModal"
    >
      <template #title>
        <div class="agent-form-modal-titlebar">
          <span class="agent-form-modal-title">{{ editingAgentID ? '编辑 Agent' : '新增 Agent' }}</span>
          <a-button class="application-toolbar-action-btn agent-form-modal-save-btn" :loading="saving" @click="handleSave">
            保存
          </a-button>
        </div>
      </template>

      <a-form ref="formRef" layout="vertical" :required-mark="false" class="agent-form">
        <div class="agent-form-note">
          {{ editingAgentID ? '编辑态下 Agent 编码保持只读。' : '创建后使用平台统一 Token 自动注册，部署时在目标主机运行安装命令即可。' }}
        </div>

        <div class="agent-form-panel">
          <div class="agent-form-panel-title">{{ editingAgentID ? '可编辑配置' : '基础配置' }}</div>

          <a-form-item name="agent_code">
            <template #label>
              <span class="agent-form-label">
                Agent 编码
                <a-tag class="agent-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.agent_code" :disabled="Boolean(editingAgentID)" placeholder="例如 prod-agent-01" />
          </a-form-item>

          <a-form-item name="name">
            <template #label>
              <span class="agent-form-label">
                Agent 名称
                <a-tag class="agent-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.name" placeholder="用于页面展示" />
          </a-form-item>

          <a-form-item name="work_dir">
            <template #label>
              <span class="agent-form-label">
                工作目录
                <a-tag class="agent-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.work_dir" placeholder="例如 /opt/gos-agent/work" />
          </a-form-item>

          <a-form-item name="environment_code">
            <template #label>
              <span class="agent-form-label">环境标识</span>
            </template>
            <a-input v-model:value="form.environment_code" placeholder="例如 prod / special-prod" />
          </a-form-item>

          <a-form-item name="status">
            <template #label>
              <span class="agent-form-label">接单状态</span>
            </template>
            <a-select v-model:value="form.status">
              <a-select-option value="active">可接单</a-select-option>
              <a-select-option value="maintenance">维护中</a-select-option>
              <a-select-option value="disabled">已禁用</a-select-option>
            </a-select>
          </a-form-item>

          <a-form-item name="tags">
            <template #label>
              <span class="agent-form-label">标签</span>
            </template>
            <a-input v-model:value="tagsText" placeholder="逗号分隔，例如 prod, java, ecs" />
          </a-form-item>

          <a-form-item name="remark">
            <template #label>
              <span class="agent-form-label">备注</span>
            </template>
            <a-textarea v-model:value="form.remark" :rows="3" placeholder="记录主机用途、职责范围或特殊限制" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="dispatchVisible"
      title="下发任务"
      :confirm-loading="dispatchLoading"
      ok-text="下发"
      cancel-text="取消"
      @ok="handleDispatchTask"
      @cancel="closeDispatchModal"
    >
      <div class="dispatch-copy">
        已选 Agent：{{ selectedDispatchAgents.length }} 台
      </div>
      <a-space wrap class="dispatch-agent-tags">
        <a-tag v-for="item in selectedDispatchAgents" :key="item.id">{{ item.name || item.agent_code }}</a-tag>
      </a-space>
      <a-form layout="vertical">
        <a-form-item label="选择任务" required>
          <a-select
            v-model:value="selectedDispatchTaskID"
            show-search
            allow-clear
            placeholder="请选择要下发的任务"
            :options="dispatchTaskOptions"
            :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
          />
        </a-form-item>
        <div v-if="selectedDispatchTask" class="selected-task-card">
          <div class="selected-task-head">
            <div>
              <div class="selected-task-title">{{ selectedDispatchTask.name }}</div>
              <div class="muted-text">
                {{ taskModeText(selectedDispatchTask.task_mode) }} · {{ taskTypeText(selectedDispatchTask.task_type) }} · 来源 {{ selectedDispatchTask.agent_code || '未分配任务' }}
              </div>
            </div>
            <a-tag :color="taskStatusColor(selectedDispatchTask.status)">{{ taskStatusText(selectedDispatchTask.status) }}</a-tag>
          </div>
          <div class="task-meta">
            <span>目录：{{ selectedDispatchTask.work_dir || '-' }}</span>
            <span>脚本：{{ selectedDispatchTask.script_name || selectedDispatchTask.script_path || '-' }}</span>
            <span>超时：{{ selectedDispatchTask.timeout_sec }}s</span>
          </div>
          <a-textarea :value="selectedDispatchTask.script_text" :rows="10" readonly />
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="boundAgentModalVisible"
      :title="currentBoundTask ? `${currentBoundTask.name} - 绑定 Agent 列表` : '绑定 Agent 列表'"
      :footer="null"
      :width="720"
    >
      <a-table
        :columns="[
          { title: 'Agent 名称', dataIndex: 'name', key: 'name', ellipsis: true },
          { title: 'Agent Code', dataIndex: 'agent_code', key: 'agent_code', width: 180, ellipsis: true },
          { title: '环境', dataIndex: 'environment_code', key: 'environment_code', width: 100 },
          { title: '状态', key: 'runtime_state', width: 100 },
        ]"
        :data-source="getBoundAgentList(currentBoundTask)"
        :pagination="false"
        size="small"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'name'">
            {{ record.name || record.agent_code }}
          </template>
          <template v-if="column.dataIndex === 'environment_code'">
            {{ record.environment_code || '-' }}
          </template>
          <template v-if="column.key === 'runtime_state'">
            <a-tag :color="{ online: 'green', offline: 'default', busy: 'orange', disabled: 'red', maintenance: 'blue' }[record.runtime_state] || 'default'">
              {{ { online: '在线', offline: '离线', busy: '忙碌', disabled: '禁用', maintenance: '维护' }[record.runtime_state] || record.runtime_state }}
            </a-tag>
          </template>
        </template>
      </a-table>
    </a-modal>

    <a-modal
      v-model:open="editResidentTaskVisible"
      title="编辑常驻任务"
      :width="860"
      :confirm-loading="editResidentTaskSaving"
      ok-text="保存"
      cancel-text="取消"
      @ok="handleSaveEditResidentTask"
      @cancel="() => { editResidentTaskVisible = false }"
    >
      <a-form layout="vertical" class="task-create-form">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="任务名称" required>
              <a-input v-model:value="editResidentTaskForm.name" placeholder="例如：版本检查、下载产物" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="任务模式">
              <a-select v-model:value="editResidentTaskForm.task_mode">
                <a-select-option value="resident">常驻任务</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="选择脚本" required>
          <a-select
            :value="editResidentTaskForm.script_id || undefined"
            allow-clear
            show-search
            placeholder="请选择脚本管理中的脚本"
            :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
            :options="scriptOptions.map((item) => ({ value: item.id, label: `${item.name} · ${taskTypeText(item.task_type as AgentTaskType)}${item.script_path ? ` · ${item.script_path}` : ''}` }))"
            @update:value="(value) => { editResidentTaskForm.script_id = String(value || ''); const s = scriptOptions.find(x => x.id === value); if (s) { editResidentTaskForm.task_type = s.task_type as any; editResidentTaskForm.shell_type = s.shell_type; editResidentTaskForm.script_text = s.script_text; editResidentTaskForm.script_path = s.script_path; editResidentTaskForm.script_name = s.name } }"
          />
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="脚本类型">
              <a-input :value="taskTypeText(editResidentTaskForm.task_type)" readonly />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Shell 类型">
              <a-input :value="editResidentTaskForm.shell_type || '-'" readonly />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="超时时间（秒）">
              <a-input-number v-model:value="editResidentTaskForm.timeout_sec" :min="10" :max="3600" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="工作目录">
              <a-input v-model:value="editResidentTaskForm.work_dir" placeholder="留空则使用 Agent 工作目录" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="脚本内容">
          <a-textarea v-model:value="editResidentTaskForm.script_text" :rows="8" readonly />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
/* ---- page header (transparent, no card bg) ---- */
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

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

/* ---- header glass buttons ---- */
.agent-toolbar-action-btn {
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
  font-size: 14px;
  font-weight: 600;
}

.agent-toolbar-action-btn:hover,
.agent-toolbar-action-btn:focus,
.agent-toolbar-action-btn:focus-visible,
.agent-toolbar-action-btn:active {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
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
  font-weight: 600;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn):hover,
:deep(.application-toolbar-action-btn.ant-btn):focus,
:deep(.application-toolbar-action-btn.ant-btn):focus-visible,
:deep(.application-toolbar-action-btn.ant-btn):active {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.agent-toolbar-action-btn--primary {
  background: linear-gradient(180deg, rgba(241, 247, 255, 0.9), rgba(223, 235, 255, 0.8)) !important;
  border-color: rgba(147, 197, 253, 0.74) !important;
  color: #1d4ed8 !important;
}

.agent-toolbar-action-btn--primary:hover,
.agent-toolbar-action-btn--primary:focus,
.agent-toolbar-action-btn--primary:active {
  background: linear-gradient(180deg, rgba(248, 251, 255, 0.96), rgba(231, 241, 255, 0.88)) !important;
  border-color: rgba(96, 165, 250, 0.66) !important;
  color: #1e3a8a !important;
}

/* ---- header icon button ---- */
.agent-toolbar-icon-btn {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 42px;
	height: 42px;
	border-radius: 16px;
	border: 1px solid rgba(148, 163, 184, 0.28) !important;
	background: rgba(255, 255, 255, 0.42) !important;
	color: #0f172a !important;
	box-shadow:
		inset 0 1px 0 rgba(255, 255, 255, 0.68),
		0 10px 22px rgba(15, 23, 42, 0.05) !important;
	backdrop-filter: blur(14px) saturate(135%);
	padding: 0;
}

.agent-toolbar-icon-btn:hover,
.agent-toolbar-icon-btn:focus {
	border-color: rgba(96, 165, 250, 0.34) !important;
	background: rgba(255, 255, 255, 0.56) !important;
	color: #0f172a !important;
}

/* ---- search overlay ---- */
.agent-search-fade-enter-active { transition: opacity 0.18s ease; }
.agent-search-fade-leave-active { transition: opacity 0.12s ease; }
.agent-search-fade-enter-from,
.agent-search-fade-leave-to { opacity: 0; }

.agent-search-overlay {
	position: fixed;
	top: 0; right: 0; bottom: 0;
	left: var(--layout-sider-width, 220px);
	z-index: 1200;
	display: flex;
	align-items: flex-start;
	justify-content: center;
	padding: 84px 24px 24px;
	background: rgba(255, 255, 255, 0.08);
	backdrop-filter: blur(8px) saturate(112%);
}

.agent-search-floating-panel {
	width: min(100%, 480px);
	display: flex;
	flex-direction: column;
	gap: 10px;
}

.agent-search-floating-input {
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

.agent-search-floating-input:focus-within {
	border-color: rgba(255, 255, 255, 0.82);
	box-shadow:
		inset 0 1px 0 rgba(255, 255, 255, 0.9),
		0 18px 38px rgba(15, 23, 42, 0.1);
}

.agent-search-floating-icon { color: rgba(148, 163, 184, 0.9); font-size: 14px; }

.agent-search-floating-field {
	flex: 1; min-width: 0; height: 34px; padding: 0;
	border: none; outline: none; background: transparent; box-shadow: none;
	color: #0f172a; font-size: 13px; line-height: 34px;
}

.agent-search-floating-field::placeholder { color: rgba(71, 85, 105, 0.72); }

.agent-search-suggestions {
	display: flex; flex-direction: column; gap: 2px;
	padding: 8px; border-radius: 14px;
	background: linear-gradient(180deg, rgba(255,255,255,0.96), rgba(248,250,252,0.94));
	border: 1px solid rgba(148,163,184,0.14);
	box-shadow: 0 14px 36px rgba(15,23,42,0.1), inset 0 1px 0 rgba(255,255,255,0.84);
}

.agent-search-suggestion-loading { padding: 10px 12px; color: #94a3b8; font-size: 12px; text-align: center; }

.agent-search-suggestion {
	display: flex; align-items: center; justify-content: space-between; gap: 10px;
	width: 100%; padding: 8px 10px; border: none; border-radius: 10px;
	background: transparent; cursor: pointer; text-align: left;
	color: inherit; font-family: inherit;
}

.agent-search-suggestion:hover,
.agent-search-suggestion:focus { background: rgba(239, 246, 255, 0.8); outline: none; }

.agent-search-suggestion-title { color: #0f172a; font-size: 13px; font-weight: 600; }
.agent-search-suggestion-subtitle { color: #94a3b8; font-size: 12px; }


/* ---- overview chart card (dark dashboard) ---- */
.overview-chart-card {
  position: relative;
  overflow: hidden;
  isolation: isolate;
  min-height: 236px;
  border-radius: 20px;
  border: 1px solid rgba(71, 85, 105, 0.4);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.12), transparent 32%),
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.1), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.98), rgba(30, 41, 59, 0.96));
  box-shadow:
    0 24px 48px rgba(15, 23, 42, 0.14),
    inset 0 1px 0 rgba(255, 255, 255, 0.12);
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.overview-chart-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 1px;
  background: linear-gradient(90deg, rgba(34, 197, 94, 0.32), rgba(59, 130, 246, 0.32), rgba(34, 197, 94, 0.16));
  pointer-events: none;
  z-index: 1;
}

.overview-chart-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.overview-chart-copy {
  min-width: 0;
}

.overview-chart-label {
  color: rgba(148, 163, 184, 0.72);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.overview-chart-title {
  margin-top: 4px;
  color: #eff6ff;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

.overview-chart-pill {
  display: inline-flex;
  align-items: center;
  padding: 5px 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.1);
  color: rgba(226, 232, 240, 0.78);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.overview-chart-canvas {
  width: 100%;
  height: 142px;
}

.overview-chart-footnote {
  color: rgba(148, 163, 184, 0.56);
  font-size: 12px;
  line-height: 1.6;
}

.agent-link {
  color: #1890ff;
  cursor: pointer;
  transition: color 0.2s;
}

.agent-link:hover {
  color: #40a9ff;
  text-decoration: underline;
}

.agent-primary {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.agent-name {
  font-weight: 600;
}

.agent-meta,
.muted-text {
  color: var(--color-text-secondary);
  font-size: 12px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.detail-item {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 16px;
  padding: 14px 16px;
  background: rgba(255, 255, 255, 0.88);
}

.detail-item-full {
  grid-column: 1 / -1;
}

.detail-label {
  color: var(--color-text-secondary);
  font-size: 12px;
  margin-bottom: 8px;
}

.detail-value {
  color: var(--color-text-main);
  font-weight: 600;
  word-break: break-all;
}

.token-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.bootstrap-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.task-panel-title {
  color: var(--color-text-main);
  font-size: 16px;
  font-weight: 700;
}

.task-panel-subtitle {
  margin-top: 6px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.config-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.config-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.config-col {
  min-width: 0;
}

.config-col--yaml {
  grid-column: 1 / -1;
}

.config-label {
  color: var(--color-text-secondary);
  font-size: 12px;
  margin-bottom: 6px;
}

.config-value {
  color: var(--color-text-main);
  font-size: 13px;
  font-weight: 600;
  word-break: break-all;
}

.config-preview {
  margin: 0;
  padding: 16px;
  border-radius: 16px;
  border: 1px solid var(--color-border);
  background: #0f172a;
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.7;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.config-copy-btn {
  margin-top: 10px;
  border-radius: 10px !important;
  font-weight: 600;
}

.config-actions {
  display: flex;
  justify-content: flex-end;
}

.tasks-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-sections {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.task-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-main);
}

.task-item {
  border: 1px solid var(--color-border);
  border-radius: 16px;
  padding: 14px 16px;
  background: var(--color-bg-card);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-item-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.task-name {
  font-weight: 600;
  color: var(--color-text-main);
}

.task-name-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  color: var(--color-text-secondary);
  font-size: 12px;
}

.task-meta-secondary {
  color: var(--color-text-muted);
}

.task-error {
  color: var(--color-danger);
  background: var(--color-danger-bg);
  border: 1px solid rgba(220, 38, 38, 0.16);
  border-radius: 12px;
  padding: 10px 12px;
}

.task-output-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 12px;
}

.task-output-card {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.9);
  padding: 12px;
}

.task-output-card-error {
  border-color: rgba(220, 38, 38, 0.16);
  background: #fff7f7;
}

.task-output-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
}

.task-log {
  margin: 0;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 12px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-x: auto;
}

.task-log-error {
  color: #fecaca;
}

.task-empty-output,
.task-summary {
  font-size: 12px;
}

.resident-task-item {
  border-color: rgba(59, 130, 246, 0.16);
}

.resident-progress-card {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.88);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.resident-progress-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.resident-progress-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .overview-chart-card {
    min-height: 210px;
  }

  .overview-chart-canvas {
    height: 132px;
  }

  .overview-chart-title {
    font-size: 18px;
  }
}

@media (max-width: 900px) {
  .page-header,
  .bootstrap-card-head,
  .task-item-head,
  .tasks-header,
  .token-row {
    flex-direction: column;
    align-items: flex-start;
  }

  .detail-grid,
  .config-meta {
    grid-template-columns: 1fr;
  }

  .config-col--yaml {
    grid-column: 1;
  }
}

/* ---- agent form modal ---- */
.agent-form-modal-wrap :deep(.ant-modal-content) {
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
}

.agent-form-modal-wrap :deep(.ant-modal-content)::before {
	content: '';
	position: absolute;
	inset: 0;
	background:
		linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
		radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
	pointer-events: none;
	z-index: 0;
}

.agent-form-modal-wrap :deep(.ant-modal-header) {
	position: relative; z-index: 1;
	margin-bottom: 10px;
	border-bottom: 1px solid rgba(226, 232, 240, 0.92);
	background: transparent;
}

.agent-form-modal-wrap :deep(.ant-modal-body) {
	position: relative; z-index: 1;
	padding-top: 10px;
}

.agent-form-modal-titlebar {
	display: flex; align-items: center; justify-content: space-between;
	gap: 16px; width: 100%;
}

.agent-form-modal-title {
	min-width: 0;
	color: #0f172a; font-size: 22px; font-weight: 800;
	letter-spacing: -0.02em;
}

.agent-form-modal-save-btn.ant-btn {
	flex: none; font-size: 14px; font-weight: 600; letter-spacing: normal;
}

.agent-form {
	display: flex; flex-direction: column; gap: 20px;
}

.agent-form-note {
	position: relative;
	padding: 0 0 0 14px;
	color: #64748b; font-size: 13px; line-height: 1.6;
}

.agent-form-note::before {
	content: '';
	position: absolute; left: 0; top: 3px; bottom: 3px;
	width: 4px; border-radius: 999px;
	background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.agent-form-panel { padding: 0; }

.agent-form-panel-title {
	display: flex; align-items: center; gap: 12px;
	margin-bottom: 14px;
	color: #0f172a; font-size: 14px; line-height: 1.4; font-weight: 700;
}

.agent-form-panel-title::after {
	content: '';
	flex: 1; height: 1px;
	background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
	transform: translateY(1px);
}

.agent-form-note + .agent-form-panel {
	padding-top: 18px;
	border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.agent-form-label {
	display: inline-flex; align-items: center; gap: 8px;
	color: #0f172a;
}

.agent-form-required-tag {
	margin-inline-end: 0;
	border: 1px solid rgba(191, 219, 254, 0.72);
	background: rgba(239, 246, 255, 0.96);
	color: #2563eb; font-size: 11px; line-height: 18px;
}

.agent-form :deep(.ant-form-item) { margin-bottom: 14px; }

.agent-form :deep(.ant-form-item-label > label) {
	color: #0f172a; font-size: 13px; font-weight: 700;
}

.agent-form :deep(.ant-input),
.agent-form :deep(.ant-select-selector),
.agent-form :deep(.ant-input-affix-wrapper),
.agent-form :deep(.ant-input-textarea textarea) {
	background: transparent !important;
	border-color: rgba(203, 213, 225, 0.88) !important;
	box-shadow: none !important;
}

.agent-form :deep(.ant-input:hover),
.agent-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.agent-form :deep(.ant-input-affix-wrapper:hover) {
	border-color: rgba(96, 165, 250, 0.48) !important;
}

.agent-form :deep(.ant-input:focus),
.agent-form :deep(.ant-input-focused),
.agent-form :deep(.ant-input-affix-wrapper-focused),
.agent-form :deep(.ant-select-focused .ant-select-selector) {
	background: transparent !important;
	border-color: rgba(59, 130, 246, 0.56) !important;
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.agent-form :deep(.ant-select-disabled .ant-select-selector),
.agent-form :deep(.ant-input[disabled]) {
	background: transparent !important; color: #94a3b8 !important;
}

.agent-form :deep(.ant-select-selection-placeholder),
.agent-form :deep(.ant-input::placeholder) { color: #94a3b8; }
</style>
