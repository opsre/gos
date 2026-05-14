<script setup lang="ts">
import { CaretRightOutlined, CopyOutlined, DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { createAgentTask, createUnassignedAgentTask, deleteResidentAgentTask, deleteTemporaryAgentTask, executeAgentTask, executeStandaloneAgentTask, listAgents, listAgentScripts, listAllAgentTasks, updateResidentAgentTask, updateTemporaryAgentTask } from '../../api/agent'
import { listPlatformParamDicts } from '../../api/platform-param'
import { useAuthStore } from '../../stores/auth'
import type {
  AgentInstance,
  AgentScript,
  AgentTask,
  AgentTaskMode,
  AgentTaskType,
  CreateAgentTaskPayload,
} from '../../types/agent'
import type { PlatformParamDict } from '../../types/platform-param'
import { buildCloneName } from '../../utils/clone-name'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface TaskVariableFormItem {
  id: string
  key_mode: 'platform' | 'custom'
  platform_key: string
  custom_key: string
  value: string
}

interface AgentTaskViewItem extends AgentTask {
  agent_name: string
  agent_code_display: string
  agent_environment_code: string
  resident_assigned_count?: number
  resident_running_count?: number
  resident_queued_count?: number
  resident_claimed_count?: number
  resident_pending_count?: number
  resident_cancelled_count?: number
}

interface TaskSearchOption {
  value: string
  label: string
  subtitle: string
  taskID: string
  taskMode: AgentTaskMode
  searchText: string
}

const authStore = useAuthStore()
const AUTO_REFRESH_INTERVAL = 15000

const loadingAgents = ref(false)
const refreshingTasks = ref(false)
const savingTask = ref(false)
const createTaskVisible = ref(false)
const previewTaskVisible = ref(false)
const boundAgentModalVisible = ref(false)
const currentBoundTask = ref<AgentTaskViewItem | AgentTask | null>(null)
const dataSource = ref<AgentInstance[]>([])
const scriptOptions = ref<AgentScript[]>([])
const platformParamOptions = ref<PlatformParamDict[]>([])
const taskVariables = ref<TaskVariableFormItem[]>([])
const editTaskVariables = ref<TaskVariableFormItem[]>([])
const editResidentTaskVariables = ref<TaskVariableFormItem[]>([])
const selectedScriptID = ref('')
const residentTaskList = ref<AgentTaskViewItem[]>([])
const historyTaskList = ref<AgentTaskViewItem[]>([])
const previewTask = ref<AgentTaskViewItem | null>(null)
const activeTaskTab = ref<'resident' | 'history'>('resident')
const taskSearchDialogVisible = ref(false)
const taskSearchInputRef = ref<HTMLInputElement | null>(null)
const modalViewportInset = ref(0)
let autoRefreshTimer: number | null = null
let modalViewportObserver: ResizeObserver | null = null
const historyFilters = reactive({
  keyword: '',
  page: 1,
  page_size: 10,
})
const taskSearchDraft = reactive({
  keyword: '',
})

const taskForm = reactive<CreateAgentTaskPayload>({
  name: '',
  task_mode: 'temporary',
  task_type: 'shell_task',
  shell_type: 'sh',
  work_dir: '',
  script_id: '',
  script_path: '',
  script_text: '',
  variables: {},
  timeout_sec: 300,
})
const taskTargetAgentIDs = ref<string[]>([])

const canManageAgent = computed(() => authStore.hasPermission('component.agent.manage'))
const canViewAgent = computed(() => canManageAgent.value || authStore.hasPermission('component.agent.view'))
const taskTargetOptions = computed(() =>
  dataSource.value.map((item) => ({
    value: item.id,
    label: `${item.name || item.agent_code} · ${item.environment_code || '未设置环境'} · ${item.work_dir}`,
  })),
)
const selectedTaskAgents = computed(() =>
  taskTargetAgentIDs.value
    .map((id) => dataSource.value.find((item) => item.id === id))
    .filter((item): item is AgentInstance => Boolean(item)),
)
const selectedAgentNames = computed(() => selectedTaskAgents.value.map((item) => item.name || item.agent_code))
const selectedAgentWorkDirs = computed(() => Array.from(new Set(selectedTaskAgents.value.map((item) => item.work_dir).filter(Boolean))))
const selectedScript = computed(() => scriptOptions.value.find((item) => item.id === selectedScriptID.value) || null)
const taskSearchOptions = computed(() => [
  ...residentTaskList.value.map(taskSearchOptionFromTask),
  ...historyTaskList.value.map(taskSearchOptionFromTask),
])
const visibleTaskSearchOptions = computed(() => {
  const keyword = taskSearchDraft.keyword.trim()
  const options = keyword
    ? taskSearchOptions.value.filter((item) => filterTaskSearchOption(keyword, item))
    : taskSearchOptions.value
  return options.slice(0, 8)
})
const filteredResidentTaskList = computed(() => {
  const keyword = historyFilters.keyword.trim().toLowerCase()
  return residentTaskList.value.filter((item) => taskMatchesKeyword(item, keyword))
})
const filteredHistoryTaskList = computed(() => {
  const keyword = historyFilters.keyword.trim().toLowerCase()
  return historyTaskList.value.filter((item) => taskMatchesKeyword(item, keyword))
})
const pagedHistoryTaskList = computed(() => {
  const start = (historyFilters.page - 1) * historyFilters.page_size
  return filteredHistoryTaskList.value.slice(start, start + historyFilters.page_size)
})
const modalMaskStyle = computed(() => ({
  left: `${modalViewportInset.value}px`,
  width: `calc(100% - ${modalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: createTaskVisible.value ? 'auto' : 'none',
}))
const modalWrapProps = computed(() => ({
  style: {
    left: `${modalViewportInset.value}px`,
    width: `calc(100% - ${modalViewportInset.value}px)`,
    pointerEvents: createTaskVisible.value ? 'auto' : 'none',
  },
}))

function createTaskVariableItem(): TaskVariableFormItem {
  return {
    id: `var-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    key_mode: 'platform',
    platform_key: '',
    custom_key: '',
    value: '',
  }
}

function buildTaskVariableItems(variables?: Record<string, string>) {
  const entries = Object.entries(variables || {})
  if (!entries.length) {
    return [createTaskVariableItem()]
  }
  const platformKeys = new Set(
    platformParamOptions.value
      .map((item) => String(item.param_key || '').trim())
      .filter(Boolean),
  )
  return entries
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => {
      const normalizedKey = String(key || '').trim()
      const isPlatformKey = platformKeys.has(normalizedKey)
      return {
        id: `var-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
        key_mode: isPlatformKey ? 'platform' : 'custom',
        platform_key: isPlatformKey ? normalizedKey : '',
        custom_key: isPlatformKey ? '' : normalizedKey,
        value: String(value || ''),
      } satisfies TaskVariableFormItem
    })
}

function resetTaskForm() {
  taskForm.name = ''
  taskForm.task_mode = 'temporary'
  taskForm.task_type = 'shell_task'
  taskForm.shell_type = 'sh'
  taskForm.work_dir = ''
  taskForm.script_id = ''
  taskForm.script_path = ''
  taskForm.script_text = ''
  taskForm.variables = {}
  taskForm.timeout_sec = 300
  taskTargetAgentIDs.value = []
  selectedScriptID.value = ''
  taskVariables.value = [createTaskVariableItem()]
}

function openCreateTaskModal() {
  createTaskVisible.value = true
}

function closeCreateTaskModal() {
  createTaskVisible.value = false
}

function readModalViewportInset() {
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
  return sider ? Math.max(sider.getBoundingClientRect().width, 0) : 0
}

function syncModalViewportInset() {
  modalViewportInset.value = readModalViewportInset()
}

function observeModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }
  modalViewportObserver?.disconnect()
  modalViewportObserver = new ResizeObserver(syncModalViewportInset)
  if (appLayout) {
    modalViewportObserver.observe(appLayout)
  }
  if (sider) {
    modalViewportObserver.observe(sider)
  }
}

function stopObservingModalViewportInset() {
  modalViewportObserver?.disconnect()
  modalViewportObserver = null
}

function openTaskPreview(item: AgentTaskViewItem) {
  previewTask.value = item
  previewTaskVisible.value = true
}

function closeTaskPreview() {
  previewTaskVisible.value = false
}

function normalizeAgents(rows: AgentInstance[]) {
  rows.forEach((item) => {
    // 缓存项无需额外处理，这里预留做展示映射
    void item
  })
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

function taskPreviewTitle(task: AgentTask | null) {
  if (!task) {
    return '任务内容预览'
  }
  return `${task.name} · 任务内容预览`
}

function taskContentLabel(task: AgentTask | null) {
  if (!task) {
    return '脚本内容'
  }
  return task.task_type === 'file_distribution_task' ? '文件内容' : '脚本内容'
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

function normalizeSearchValue(value: unknown) {
  return String(value ?? '').toLowerCase()
}

function taskMatchesKeyword(task: AgentTaskViewItem, keyword: string) {
  if (!keyword) {
    return true
  }
  const variableEntries = Object.entries(task.variables || {}).flatMap(([key, value]) => [key, value])
  const statusForSearch = task.last_run_status || task.status
  const searchableValues = [
    task.name,
    taskModeText(task.task_mode),
    taskTypeText(task.task_type),
    taskStatusText(statusForSearch),
    residentRuntimeText(task),
    task.agent_name,
    task.agent_code,
    task.agent_code_display,
    task.agent_environment_code,
    task.script_name,
    task.script_path,
    task.shell_type,
    task.work_dir,
    task.created_by,
    task.last_run_summary,
    task.failure_reason,
    task.stdout_text,
    task.stderr_text,
    ...task.target_agent_ids,
    ...variableEntries,
  ]
  return searchableValues.some((value) => normalizeSearchValue(value).includes(keyword))
}

function taskSearchOptionFromTask(task: AgentTaskViewItem): TaskSearchOption {
  const modeLabel = taskModeText(task.task_mode)
  const secondary = [task.script_name || task.script_path, task.agent_name || task.agent_code_display]
    .filter(Boolean)
    .join(' · ')
  const label = secondary ? `${task.name} · ${modeLabel} · ${secondary}` : `${task.name} · ${modeLabel}`
  const searchText = [
    label,
    task.name,
    modeLabel,
    taskTypeText(task.task_type),
    task.agent_name,
    task.agent_code,
    task.agent_code_display,
    task.script_name,
    task.script_path,
    task.work_dir,
  ].join(' ')
  return {
    value: task.name,
    label,
    subtitle: secondary ? `${modeLabel} · ${secondary}` : modeLabel,
    taskID: task.id,
    taskMode: task.task_mode,
    searchText,
  }
}

function filterTaskSearchOption(input: string, option: TaskSearchOption) {
  const keyword = normalizeSearchValue(input).trim()
  if (!keyword) {
    return true
  }
  return normalizeSearchValue(option.searchText || option.label || option.value).includes(keyword)
}

function readTaskSearchInput(value: string | Event) {
  if (typeof value === 'string') {
    return value
  }
  const target = value.target as HTMLInputElement | null
  return target?.value || ''
}

function chooseTaskTabForKeyword(keywordRaw: string) {
  const keyword = normalizeSearchValue(keywordRaw).trim()
  if (!keyword) {
    return
  }
  const residentMatched = residentTaskList.value.some((item) => taskMatchesKeyword(item, keyword))
  const historyMatched = historyTaskList.value.some((item) => taskMatchesKeyword(item, keyword))
  if (residentMatched && !historyMatched) {
    activeTaskTab.value = 'resident'
  } else if (historyMatched && !residentMatched) {
    activeTaskTab.value = 'history'
  }
}

function openTaskSearchDialog() {
  taskSearchDraft.keyword = historyFilters.keyword
  taskSearchDialogVisible.value = true
  void nextTick(() => {
    taskSearchInputRef.value?.focus()
  })
}

function closeTaskSearchDialog() {
  taskSearchDialogVisible.value = false
}

function handleTaskSearchInput(value: string | Event) {
  taskSearchDraft.keyword = readTaskSearchInput(value)
  historyFilters.keyword = taskSearchDraft.keyword
  historyFilters.page = 1
  chooseTaskTabForKeyword(historyFilters.keyword)
}

function handleTaskSearchSubmit() {
  historyFilters.keyword = taskSearchDraft.keyword.trim()
  historyFilters.page = 1
  chooseTaskTabForKeyword(historyFilters.keyword)
  closeTaskSearchDialog()
}

function handleTaskSearchSelect(option: TaskSearchOption) {
  taskSearchDraft.keyword = option.value
  historyFilters.keyword = option.value
  historyFilters.page = 1
  activeTaskTab.value = option.taskMode === 'resident' ? 'resident' : 'history'
  closeTaskSearchDialog()
}

function taskAgentBindingText(task: AgentTaskViewItem | AgentTask) {
  const targetAgentIDs = task.target_agent_ids || []
  if (!targetAgentIDs.length) {
    return '未绑定'
  }
  return `绑定 ${targetAgentIDs.length} 台 Agent`
}

function showBoundAgentsModal(task: AgentTaskViewItem | AgentTask) {
  currentBoundTask.value = task
  boundAgentModalVisible.value = true
}

function getBoundAgentList(task: AgentTaskViewItem | AgentTask | null) {
  if (!task) return []
  const targetAgentIDs = task.target_agent_ids || []
  return targetAgentIDs
    .map((id) => dataSource.value.find((item) => item.id === id))
    .filter((item): item is AgentInstance => Boolean(item))
}

function taskVariableSignature(variables?: Record<string, string>) {
  const entries = Object.entries(variables || {}).sort(([a], [b]) => a.localeCompare(b))
  return JSON.stringify(entries)
}

function residentTaskSignature(task: AgentTask) {
  return [
    task.name || '',
    task.task_type || '',
    task.shell_type || '',
    task.script_id || '',
    task.script_name || '',
    task.script_path || '',
    task.script_text || '',
    taskVariableSignature(task.variables),
  ].join('::')
}

function residentRuntimeText(task: AgentTask) {
  const runningCount = Number((task as AgentTaskViewItem).resident_running_count || 0)
  const queuedCount = Number((task as AgentTaskViewItem).resident_queued_count || 0)
  const claimedCount = Number((task as AgentTaskViewItem).resident_claimed_count || 0)
  const pendingCount = Number((task as AgentTaskViewItem).resident_pending_count || 0)
  const assignedCount = Number((task as AgentTaskViewItem).resident_assigned_count || 0)
  const cancelledCount = Number((task as AgentTaskViewItem).resident_cancelled_count || 0)
  if (runningCount > 0) {
    return '执行中'
  }
  if (claimedCount > 0) {
    return '准备执行'
  }
  if (queuedCount > 0) {
    return '排队中'
  }
  if (assignedCount === 0) {
    return '未分发'
  }
  if (cancelledCount === assignedCount) {
    return '已停止'
  }
  if (pendingCount > 0) {
    return '待下一轮'
  }
  if (task.status === 'running') {
    return '执行中'
  }
  if (task.status === 'claimed') {
    return '准备执行'
  }
  if (task.status === 'queued') {
    return '排队中'
  }
  if (task.status === 'cancelled') {
    return '已停止'
  }
  if ((task.run_count || 0) > 0) {
    return '待下一轮'
  }
  return '待首次执行'
}

function residentRuntimeColor(task: AgentTask) {
  const runningCount = Number((task as AgentTaskViewItem).resident_running_count || 0)
  const queuedCount = Number((task as AgentTaskViewItem).resident_queued_count || 0)
  const claimedCount = Number((task as AgentTaskViewItem).resident_claimed_count || 0)
  const assignedCount = Number((task as AgentTaskViewItem).resident_assigned_count || 0)
  const cancelledCount = Number((task as AgentTaskViewItem).resident_cancelled_count || 0)
  const pendingCount = Number((task as AgentTaskViewItem).resident_pending_count || 0)
  if (runningCount > 0) {
    return 'blue'
  }
  if (claimedCount > 0) {
    return 'gold'
  }
  if (queuedCount > 0) {
    return 'orange'
  }
  if (assignedCount === 0 || cancelledCount === assignedCount) {
    return 'default'
  }
  if (pendingCount > 0) {
    return 'green'
  }
  if (task.status === 'running') {
    return 'blue'
  }
  if (task.status === 'claimed') {
    return 'gold'
  }
  if (task.status === 'queued') {
    return 'orange'
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

function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function platformParamLabel(paramKey: string) {
  const matched = platformParamOptions.value.find((item) => item.param_key === paramKey)
  if (!matched) {
    return paramKey
  }
  return `${matched.name} (${matched.param_key})`
}

function resolvedVariableKey(item: TaskVariableFormItem) {
  return item.key_mode === 'platform' ? item.platform_key.trim() : item.custom_key.trim()
}

function addTaskVariable() {
  taskVariables.value.push(createTaskVariableItem())
}

function addTaskVariableTo(list: typeof taskVariables) {
  list.value.push(createTaskVariableItem())
}

function addEditTaskVariable() {
  addTaskVariableTo(editTaskVariables)
}

function addEditResidentTaskVariable() {
  addTaskVariableTo(editResidentTaskVariables)
}

function handleVariableModeChange(item: TaskVariableFormItem, mode: 'platform' | 'custom') {
  item.key_mode = mode
  if (mode === 'platform') {
    item.custom_key = ''
  } else {
    item.platform_key = ''
  }
}

function handleVariableModeChangeFor(
  item: TaskVariableFormItem,
  mode: 'platform' | 'custom',
) {
  handleVariableModeChange(item, mode)
}

function removeTaskVariable(id: string) {
  if (taskVariables.value.length <= 1) {
    taskVariables.value = [createTaskVariableItem()]
    return
  }
  taskVariables.value = taskVariables.value.filter((item) => item.id !== id)
}

function removeTaskVariableFrom(list: typeof taskVariables, id: string) {
  if (list.value.length <= 1) {
    list.value = [createTaskVariableItem()]
    return
  }
  list.value = list.value.filter((item) => item.id !== id)
}

function removeEditTaskVariable(id: string) {
  removeTaskVariableFrom(editTaskVariables, id)
}

function removeEditResidentTaskVariable(id: string) {
  removeTaskVariableFrom(editResidentTaskVariables, id)
}

function serializeTaskVariables(): Record<string, string> {
  return serializeTaskVariableItems(taskVariables.value)
}

function serializeTaskVariableItems(items: TaskVariableFormItem[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const item of items) {
    const key = resolvedVariableKey(item)
    const value = String(item.value || '').trim()
    if (!key && !value) {
      continue
    }
    if (!key) {
      throw new Error('请补全变量 Key')
    }
    if (!value) {
      throw new Error(`请填写变量 ${platformParamLabel(key)} 的值`)
    }
    if (result[key] !== undefined) {
      throw new Error(`变量 ${platformParamLabel(key)} 重复，请检查配置`)
    }
    result[key] = value
  }
  return result
}

function handleSelectManagedScript(scriptID: string) {
  selectedScriptID.value = scriptID
  const matched = scriptOptions.value.find((item) => item.id === scriptID)
  if (!matched) {
    return
  }
  taskForm.script_id = matched.id
  taskForm.task_type = matched.task_type
  taskForm.shell_type = matched.shell_type || 'sh'
  taskForm.script_path = matched.script_path || ''
  taskForm.script_text = matched.script_text || ''
  if (!String(taskForm.name || '').trim()) {
    taskForm.name = matched.name
  }
}

function clearManagedScript() {
  selectedScriptID.value = ''
  taskForm.script_id = ''
  taskForm.task_type = 'shell_task'
  taskForm.shell_type = 'sh'
  taskForm.script_path = ''
  taskForm.script_text = ''
}

function canExecuteTemporaryTask(task: AgentTaskViewItem) {
  return (
    task.task_mode === 'temporary' &&
    (Boolean(task.agent_id) || (task.target_agent_ids || []).length > 0) &&
    ['draft', 'success', 'failed', 'cancelled'].includes(String(task.status || ''))
  )
}

function executeActionText(task: AgentTaskViewItem) {
  return task.run_count > 0 ? '重新执行' : '执行'
}

async function loadAgents(options: { silent?: boolean } = {}) {
  if (!canViewAgent.value) {
    return
  }
  if (!options.silent) {
    loadingAgents.value = true
  }
  try {
    const response = await listAgents({ page: 1, page_size: 200 })
    dataSource.value = response.data
    normalizeAgents(response.data)
    taskTargetAgentIDs.value = taskTargetAgentIDs.value.filter((id) => response.data.some((item) => item.id === id))
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, 'Agent 列表加载失败'))
    }
  } finally {
    if (!options.silent) {
      loadingAgents.value = false
    }
  }
}

async function loadPlatformParamOptions() {
  try {
    const response = await listPlatformParamDicts({ page: 1, page_size: 300, status: 1 })
    platformParamOptions.value = response.data
  } catch {
    platformParamOptions.value = []
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

async function loadTaskViews(options: { silent?: boolean } = {}) {
  if (!canViewAgent.value) {
    return
  }
  if (!options.silent) {
    refreshingTasks.value = true
  }
  try {
    const response = await listAllAgentTasks({ page: 1, page_size: 300 })
    const allTasks = response.data
      .map<AgentTaskViewItem>((task) => {
        const matchedAgent = dataSource.value.find((item) => item.id === task.agent_id)
        return {
          ...task,
          agent_name: matchedAgent?.name || task.agent_code || '未分配',
          agent_code_display: matchedAgent?.agent_code || task.agent_code || '未分配',
          agent_environment_code: matchedAgent?.environment_code || '',
        }
      })
      .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    const residentTemplates = allTasks.filter((item) => item.task_mode === 'resident' && !item.agent_id)
    const residentInstances = allTasks.filter((item) => item.task_mode === 'resident' && !!item.agent_id)
    residentTaskList.value = residentTemplates.map((item) => {
      const signature = residentTaskSignature(item)
      const matchedInstances = residentInstances.filter((task) => residentTaskSignature(task) === signature)
      const latestInstance = [...matchedInstances].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())[0]
      return {
        ...item,
        resident_assigned_count: matchedInstances.length,
        resident_running_count: matchedInstances.filter((task) => task.status === 'running').length,
        resident_queued_count: matchedInstances.filter((task) => task.status === 'queued').length,
        resident_claimed_count: matchedInstances.filter((task) => task.status === 'claimed').length,
        resident_pending_count: matchedInstances.filter((task) => task.status === 'pending').length,
        resident_cancelled_count: matchedInstances.filter((task) => task.status === 'cancelled').length,
        run_count: matchedInstances.length ? matchedInstances.reduce((sum, task) => sum + Number(task.run_count || 0), 0) : item.run_count,
        success_count: matchedInstances.length ? matchedInstances.reduce((sum, task) => sum + Number(task.success_count || 0), 0) : item.success_count,
        failure_count: matchedInstances.length ? matchedInstances.reduce((sum, task) => sum + Number(task.failure_count || 0), 0) : item.failure_count,
        last_run_status: latestInstance?.last_run_status || latestInstance?.status || item.last_run_status,
        last_run_summary: latestInstance?.last_run_summary || latestInstance?.failure_reason || item.last_run_summary,
        failure_reason: latestInstance?.failure_reason || item.failure_reason,
        finished_at: latestInstance?.finished_at || item.finished_at,
        started_at: latestInstance?.started_at || item.started_at,
        claimed_at: latestInstance?.claimed_at || item.claimed_at,
      }
    })
    // 临时任务列表只展示手动创建的临时任务（source_task_id 为空的）
    // 排除发布单触发的任务（source_task_id 有值的）
    historyTaskList.value = allTasks.filter((item) => item.task_mode !== 'resident' && !item.source_task_id)
  } catch (error) {
    if (!options.silent) {
      message.error(extractHTTPErrorMessage(error, '任务加载失败'))
    }
  } finally {
    if (!options.silent) {
      refreshingTasks.value = false
    }
  }
}

async function handleCreateTask() {
  savingTask.value = true
  try {
    if (!selectedScriptID.value) {
      throw new Error('请选择脚本管理中的脚本')
    }
    if (!String(taskForm.script_text || '').trim()) {
      throw new Error('当前脚本内容为空，请重新选择脚本')
    }
    const payload: CreateAgentTaskPayload = {
      name: taskForm.name,
      task_mode: taskForm.task_mode || 'temporary',
      task_type: taskForm.task_type || 'shell_task',
      shell_type: taskForm.shell_type || 'sh',
      work_dir: taskForm.work_dir,
      script_id: selectedScriptID.value,
      script_path: taskForm.script_path,
      script_text: taskForm.script_text,
      variables: serializeTaskVariables(),
      target_agent_ids: taskTargetAgentIDs.value,
      timeout_sec: taskForm.timeout_sec,
    }
    if ((taskForm.task_mode || 'temporary') === 'temporary') {
      await createUnassignedAgentTask(payload)
      historyFilters.keyword = ''
      historyFilters.page = 1
      message.success(
        taskTargetAgentIDs.value.length
          ? `任务已创建，已绑定 ${taskTargetAgentIDs.value.length} 台 Agent；执行时会按绑定关系批量下发`
          : '任务已创建；后续绑定 Agent 后点击执行才会真正开始',
      )
    } else {
      const results = await Promise.allSettled(taskTargetAgentIDs.value.map((agentID) => createAgentTask(agentID, payload)))
      const failed = results.filter((item) => item.status === 'rejected') as PromiseRejectedResult[]
      if (failed.length) {
        const firstMessage = extractHTTPErrorMessage(failed[0].reason, '批量下发任务失败')
        throw new Error(taskTargetAgentIDs.value.length === failed.length ? firstMessage : `部分 Agent 下发失败：${firstMessage}`)
      }
      historyFilters.keyword = ''
      historyFilters.page = 1
      message.success(`任务已创建到 ${taskTargetAgentIDs.value.length} 台 Agent，点击执行后才会开始领取`)
    }
    resetTaskForm()
    createTaskVisible.value = false
    await loadTaskViews({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, error instanceof Error ? error.message : '创建任务失败'))
  } finally {
    savingTask.value = false
  }
}

async function handleExecuteTemporaryTask(task: AgentTaskViewItem) {
  try {
    if (task.agent_id) {
      await executeAgentTask(task.agent_id, task.id)
    } else {
      await executeStandaloneAgentTask(task.id)
    }
    message.success(task.run_count > 0 ? '任务已重新进入执行队列' : '任务已进入执行队列')
    await loadTaskViews({ silent: true })
    await loadAgents({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '任务执行失败'))
  }
}

function canEditTemporaryTask(task: AgentTaskViewItem) {
  // 手动创建的临时任务都可以编辑（除了执行中）
  return !task.source_task_id && task.status !== 'running' && task.status !== 'claimed'
}

function canDeleteTemporaryTask(task: AgentTaskViewItem) {
  // 执行中的任务不能删除
  return task.status !== 'running' && task.status !== 'claimed' && !task.source_task_id
}

const editTaskVisible = ref(false)
const editTaskSaving = ref(false)
const editTaskForm = reactive<CreateAgentTaskPayload>({
  name: '',
  task_mode: 'temporary',
  task_type: 'shell_task',
  shell_type: 'sh',
  work_dir: '',
  script_id: '',
  script_path: '',
  script_text: '',
  variables: {},
  timeout_sec: 300,
})
const editTaskID = ref('')
const editTaskCloneSourceID = ref('')
const editTaskTargetAgentIDs = ref<string[]>([])

function fillTemporaryTaskEditForm(task: AgentTaskViewItem, name: string) {
  editTaskForm.name = name
  editTaskForm.task_mode = task.task_mode
  editTaskForm.task_type = task.task_type
  editTaskForm.shell_type = task.shell_type
  editTaskForm.work_dir = task.work_dir
  editTaskForm.script_id = task.script_id
  editTaskForm.script_path = task.script_path
  editTaskForm.script_text = task.script_text
  editTaskForm.variables = { ...task.variables }
  editTaskVariables.value = buildTaskVariableItems(task.variables)
  editTaskForm.timeout_sec = task.timeout_sec
  editTaskTargetAgentIDs.value = [...(task.target_agent_ids || [])]
}

async function handleEditTemporaryTask(task: AgentTaskViewItem) {
  editTaskID.value = task.id
  editTaskCloneSourceID.value = ''
  fillTemporaryTaskEditForm(task, task.name)
  editTaskVisible.value = true
}

function handleCloneTemporaryTask(task: AgentTaskViewItem) {
  editTaskID.value = ''
  editTaskCloneSourceID.value = task.id
  fillTemporaryTaskEditForm(task, buildCloneName(task.name))
  editTaskVisible.value = true
}

async function handleSaveEditTemporaryTask() {
  editTaskSaving.value = true
  try {
    if (!editTaskForm.script_id) {
      throw new Error('请选择脚本')
    }
    const payload: CreateAgentTaskPayload = {
      name: editTaskForm.name,
      task_mode: editTaskForm.task_mode,
      task_type: editTaskForm.task_type,
      shell_type: editTaskForm.shell_type,
      work_dir: editTaskForm.work_dir,
      script_id: editTaskForm.script_id,
      script_path: editTaskForm.script_path,
      script_text: editTaskForm.script_text,
      variables: serializeTaskVariableItems(editTaskVariables.value),
      target_agent_ids: editTaskTargetAgentIDs.value,
      timeout_sec: editTaskForm.timeout_sec,
    }
    if (editTaskCloneSourceID.value) {
      await createUnassignedAgentTask(payload)
      message.success('任务已克隆')
    } else {
      await updateTemporaryAgentTask(editTaskID.value, payload)
      message.success('任务已更新')
    }
    editTaskVisible.value = false
    await loadTaskViews({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editTaskCloneSourceID.value ? '克隆任务失败' : '更新任务失败'))
  } finally {
    editTaskSaving.value = false
  }
}

async function handleDeleteTemporaryTask(taskID: string) {
  try {
    await deleteTemporaryAgentTask(taskID)
    message.success('任务已删除')
    await loadTaskViews({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除任务失败'))
  }
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
const editResidentTaskCloneSourceID = ref('')

function fillResidentTaskEditForm(task: AgentTaskViewItem, name: string) {
  editResidentTaskForm.name = name
  editResidentTaskForm.task_mode = task.task_mode
  editResidentTaskForm.task_type = task.task_type
  editResidentTaskForm.shell_type = task.shell_type
  editResidentTaskForm.work_dir = task.work_dir
  editResidentTaskForm.script_id = task.script_id
  editResidentTaskForm.script_path = task.script_path
  editResidentTaskForm.script_text = task.script_text
  editResidentTaskForm.variables = { ...task.variables }
  editResidentTaskVariables.value = buildTaskVariableItems(task.variables)
  editResidentTaskForm.timeout_sec = task.timeout_sec
}

function handleEditResidentTask(task: AgentTaskViewItem) {
  editResidentTaskID.value = task.id
  editResidentTaskCloneSourceID.value = ''
  fillResidentTaskEditForm(task, task.name)
  editResidentTaskVisible.value = true
}

function handleCloneResidentTask(task: AgentTaskViewItem) {
  editResidentTaskID.value = ''
  editResidentTaskCloneSourceID.value = task.id
  fillResidentTaskEditForm(task, buildCloneName(task.name))
  editResidentTaskVisible.value = true
}

async function handleSaveEditResidentTask() {
  editResidentTaskSaving.value = true
  try {
    if (!editResidentTaskForm.script_id) {
      throw new Error('请选择脚本')
    }
    const payload: CreateAgentTaskPayload = {
      name: editResidentTaskForm.name,
      task_mode: 'resident',
      task_type: editResidentTaskForm.task_type,
      shell_type: editResidentTaskForm.shell_type,
      work_dir: editResidentTaskForm.work_dir,
      script_id: editResidentTaskForm.script_id,
      script_path: editResidentTaskForm.script_path,
      script_text: editResidentTaskForm.script_text,
      variables: serializeTaskVariableItems(editResidentTaskVariables.value),
      timeout_sec: editResidentTaskForm.timeout_sec,
    }
    if (editResidentTaskCloneSourceID.value) {
      await createUnassignedAgentTask(payload)
      message.success('常驻任务已克隆')
    } else {
      await updateResidentAgentTask(editResidentTaskID.value, payload)
      message.success('常驻任务已更新')
    }
    editResidentTaskVisible.value = false
    await loadTaskViews({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editResidentTaskCloneSourceID.value ? '克隆任务失败' : '更新任务失败'))
  } finally {
    editResidentTaskSaving.value = false
  }
}

async function handleDeleteResidentTask(taskID: string) {
  try {
    await deleteResidentAgentTask(taskID)
    message.success('常驻任务已删除')
    await loadTaskViews({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除任务失败'))
  }
}

function applyHistoryFilter() {
  historyFilters.page = 1
}

function resetHistoryFilter() {
  historyFilters.keyword = ''
  taskSearchDraft.keyword = ''
  historyFilters.page = 1
}

function handleHistoryPageChange(page: number, pageSize: number) {
  historyFilters.page = page
  historyFilters.page_size = pageSize
}

watch(
  () => filteredHistoryTaskList.value.length,
  (total) => {
    const maxPage = Math.max(1, Math.ceil(total / historyFilters.page_size))
    if (historyFilters.page > maxPage) {
      historyFilters.page = maxPage
    }
  },
)

async function runAutoRefresh() {
  if (document.hidden || !canViewAgent.value) {
    return
  }
  await loadAgents({ silent: true })
  await loadTaskViews({ silent: true })
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

onMounted(async () => {
  resetTaskForm()
  syncModalViewportInset()
  observeModalViewportInset()
  await Promise.all([loadAgents(), loadPlatformParamOptions(), loadScriptOptions()])
  await loadTaskViews()
  startAutoRefresh()
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  stopObservingModalViewportInset()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-header">
      <div class="page-header-copy">
        <div class="page-title">任务</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" @click="openTaskSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-button class="application-toolbar-action-btn" :loading="refreshingTasks" @click="loadTaskViews">
          <template #icon><ReloadOutlined /></template>
          刷新任务
        </a-button>
        <a-button v-if="canManageAgent" class="application-toolbar-action-btn" @click="openCreateTaskModal">
          <template #icon><PlusOutlined /></template>
          新增任务
        </a-button>
      </div>
    </div>

    <transition name="application-search-fade">
      <div v-if="taskSearchDialogVisible" class="application-search-overlay task-search-overlay" @click.self="closeTaskSearchDialog">
        <div class="application-search-floating-panel">
          <div class="application-search-floating-input">
            <SearchOutlined class="application-search-floating-icon" />
            <input
              ref="taskSearchInputRef"
              v-model="taskSearchDraft.keyword"
              class="application-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="搜索任务 / Agent / 脚本"
              @input="handleTaskSearchInput"
              @keydown.enter="handleTaskSearchSubmit"
              @keydown.esc="closeTaskSearchDialog"
            />
          </div>
          <div v-if="visibleTaskSearchOptions.length" class="application-search-suggestions">
            <button
              v-for="item in visibleTaskSearchOptions"
              :key="item.taskID"
              type="button"
              class="application-search-suggestion"
              @click="handleTaskSearchSelect(item)"
            >
              <span class="application-search-suggestion-title">{{ item.value }}</span>
              <span class="application-search-suggestion-subtitle">{{ item.subtitle }}</span>
            </button>
          </div>
        </div>
      </div>
    </transition>

    <a-modal
      v-model:open="createTaskVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="resetTaskForm"
      :mask-style="modalMaskStyle"
      :wrap-props="modalWrapProps"
      wrap-class-name="task-form-modal-wrap"
      @cancel="closeCreateTaskModal"
    >
      <template #title>
        <div class="task-form-modal-titlebar">
          <span class="task-form-modal-title">新增任务</span>
          <a-button class="application-toolbar-action-btn task-form-modal-save-btn" :loading="savingTask" @click="handleCreateTask">
            保存
          </a-button>
        </div>
      </template>

      <a-form layout="vertical" :required-mark="false" class="task-create-form">
        <div class="task-form-note">
          临时任务保存绑定的 Agent 集合，后续执行或被发布模板引用时再批量下发
        </div>

        <div class="task-form-panel">
          <div class="task-form-panel-title">任务配置</div>

          <a-form-item>
            <template #label>
              <span class="task-form-label">目标 Agent</span>
            </template>
            <div class="task-agent-selector">
              <a-select
                v-model:value="taskTargetAgentIDs"
                mode="multiple"
                allow-clear
                show-search
                placeholder="请选择要下发任务的 Agent"
                :options="taskTargetOptions"
                :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
              />
              <div v-if="selectedAgentNames.length" class="task-agent-selection">
                <a-tag v-for="name in selectedAgentNames" :key="name">{{ name }}</a-tag>
              </div>
              <div v-if="selectedAgentWorkDirs.length > 1" class="task-variable-tip">
                已选择的 Agent 工作目录不一致，如需覆盖请手动填写下方工作目录
              </div>
              <div class="task-variable-tip">
                不选择 Agent 时只保存任务内容；执行前仍可补充绑定目标
              </div>
            </div>
          </a-form-item>

          <a-row :gutter="12">
            <a-col :span="12">
              <a-form-item>
                <template #label>
                  <span class="task-form-label">
                    任务名称
                    <a-tag class="task-form-required-tag">必填</a-tag>
                  </span>
                </template>
                <a-input v-model:value="taskForm.name" placeholder="例如：版本检查、下载产物" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item>
                <template #label>
                  <span class="task-form-label">
                    任务模式
                    <a-tag class="task-form-required-tag">必填</a-tag>
                  </span>
                </template>
                <a-select v-model:value="taskForm.task_mode">
                  <a-select-option value="temporary">临时任务</a-select-option>
                  <a-select-option value="resident">常驻任务</a-select-option>
                </a-select>
              </a-form-item>
            </a-col>
          </a-row>

          <a-form-item>
            <template #label>
              <span class="task-form-label">
                选择脚本
                <a-tag class="task-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              :value="selectedScriptID || undefined"
              allow-clear
              show-search
              placeholder="请选择脚本管理中的脚本"
              :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
              :options="scriptOptions.map((item) => ({ value: item.id, label: `${item.name} · ${taskTypeText(item.task_type as AgentTaskType)}${item.script_path ? ` · ${item.script_path}` : ''}` }))"
              @update:value="(value) => value ? handleSelectManagedScript(String(value)) : clearManagedScript()"
            />
          </a-form-item>

          <div v-if="selectedScript" class="task-script-summary">
            <div class="task-script-summary-head">
              <div>
                <div class="task-script-summary-title">{{ selectedScript.name }}</div>
                <div class="muted-text">{{ selectedScript.description || '暂无脚本说明' }}</div>
              </div>
              <a-tag>{{ taskTypeText(selectedScript.task_type as AgentTaskType) }}</a-tag>
            </div>
            <div class="task-script-summary-meta">
              <span>Shell：{{ selectedScript.shell_type || '-' }}</span>
              <span v-if="selectedScript.script_path">脚本文件：{{ selectedScript.script_path }}</span>
            </div>
          </div>
        </div>

        <div class="task-form-panel">
          <div class="task-form-panel-title">执行参数</div>

          <a-row :gutter="12">
            <a-col :span="12">
              <a-form-item label="脚本类型">
                <a-input :value="selectedScript ? taskTypeText(selectedScript.task_type as AgentTaskType) : '-'" readonly />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="Shell 类型">
                <a-input :value="selectedScript?.shell_type || '-'" readonly />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="超时时间（秒）">
                <a-input-number v-model:value="taskForm.timeout_sec" :min="10" :max="3600" style="width: 100%" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="工作目录">
                <a-input v-model:value="taskForm.work_dir" placeholder="留空则使用 Agent 工作目录" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-form-item label="脚本预览">
            <a-textarea :value="taskForm.script_text" :rows="8" readonly placeholder="请选择脚本后预览内容" />
          </a-form-item>
        </div>

        <div class="task-form-panel">
          <div class="task-form-panel-title">执行变量</div>
          <div class="task-variable-panel">
            <div class="task-variable-tip">
              选择脚本后，同样支持标准平台 Key 变量；脚本里直接写 <code>{env}</code> 这样的占位符即可
            </div>
            <div class="task-variable-list-editor">
              <div v-for="item in taskVariables" :key="item.id" class="task-variable-row">
                <a-select
                  :value="item.key_mode"
                  class="task-variable-mode"
                  @update:value="(value) => handleVariableModeChange(item, value as 'platform' | 'custom')"
                >
                  <a-select-option value="platform">标准平台 Key</a-select-option>
                  <a-select-option value="custom">自定义变量</a-select-option>
                </a-select>
                <a-select
                  v-if="item.key_mode === 'platform'"
                  v-model:value="item.platform_key"
                  class="task-variable-key"
                  show-search
                  allow-clear
                  placeholder="选择标准平台 Key"
                  :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
                  :options="platformParamOptions.map((option) => ({ value: option.param_key, label: platformParamLabel(option.param_key) }))"
                />
                <a-input v-else v-model:value="item.custom_key" class="task-variable-key" placeholder="例如 artifact_url" />
                <a-input v-model:value="item.value" class="task-variable-value" placeholder="请输入变量值" />
                <a-button type="text" danger class="task-variable-remove" @click="removeTaskVariable(item.id)">
                  移除
                </a-button>
                <div v-if="resolvedVariableKey(item)" class="task-variable-preview">
                  脚本占位符：<code>{{ '{' + resolvedVariableKey(item) + '}' }}</code>
                </div>
              </div>
            </div>
            <a-button type="dashed" block @click="addTaskVariable">
              <template #icon><PlusOutlined /></template>
              新增变量
            </a-button>
          </div>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="editTaskVisible"
      :title="editTaskCloneSourceID ? '克隆临时任务' : '编辑临时任务'"
      :width="860"
      :confirm-loading="editTaskSaving"
      ok-text="保存"
      cancel-text="取消"
      wrap-class-name="task-edit-modal-wrap"
      @ok="handleSaveEditTemporaryTask"
      @cancel="() => { editTaskVisible = false }"
    >
      <a-form layout="vertical" class="task-create-form">
        <a-form-item label="目标 Agent">
          <div class="task-target-card">
            <a-select
              v-model:value="editTaskTargetAgentIDs"
              mode="multiple"
              allow-clear
              show-search
              placeholder="请选择要下发任务的 Agent"
              :options="taskTargetOptions"
              :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
            />
          </div>
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="任务名称" required>
              <a-input v-model:value="editTaskForm.name" placeholder="例如：版本检查、下载产物" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="任务模式">
              <a-select v-model:value="editTaskForm.task_mode">
                <a-select-option value="temporary">临时任务</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="选择脚本" required>
          <a-select
            :value="editTaskForm.script_id || undefined"
            allow-clear
            show-search
            placeholder="请选择脚本管理中的脚本"
            :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
            :options="scriptOptions.map((item) => ({ value: item.id, label: `${item.name} · ${taskTypeText(item.task_type as AgentTaskType)}${item.script_path ? ` · ${item.script_path}` : ''}` }))"
            @update:value="(value) => { editTaskForm.script_id = String(value || ''); const s = scriptOptions.find(x => x.id === value); if (s) { editTaskForm.task_type = s.task_type as any; editTaskForm.shell_type = s.shell_type; editTaskForm.script_text = s.script_text; editTaskForm.script_path = s.script_path; editTaskForm.script_name = s.name } }"
          />
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="脚本类型">
              <a-input :value="taskTypeText(editTaskForm.task_type)" readonly />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Shell 类型">
              <a-input :value="editTaskForm.shell_type || '-'" readonly />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="超时时间（秒）">
              <a-input-number v-model:value="editTaskForm.timeout_sec" :min="10" :max="3600" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="工作目录">
              <a-input v-model:value="editTaskForm.work_dir" placeholder="留空则使用 Agent 工作目录" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="脚本内容">
          <a-textarea v-model:value="editTaskForm.script_text" :rows="8" readonly />
        </a-form-item>

        <a-form-item label="执行变量">
          <div class="task-variable-panel">
            <div class="task-variable-tip">
              编辑任务时同样支持标准平台 Key 和自定义变量；脚本里继续使用 <code>{env}</code> 这样的占位符
            </div>
            <div class="task-variable-list-editor">
              <div v-for="item in editTaskVariables" :key="item.id" class="task-variable-row">
                <a-select
                  :value="item.key_mode"
                  class="task-variable-mode"
                  @update:value="(value) => handleVariableModeChangeFor(item, value as 'platform' | 'custom')"
                >
                  <a-select-option value="platform">标准平台 Key</a-select-option>
                  <a-select-option value="custom">自定义变量</a-select-option>
                </a-select>
                <a-select
                  v-if="item.key_mode === 'platform'"
                  v-model:value="item.platform_key"
                  class="task-variable-key"
                  show-search
                  allow-clear
                  placeholder="选择标准平台 Key"
                  :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
                  :options="platformParamOptions.map((option) => ({ value: option.param_key, label: platformParamLabel(option.param_key) }))"
                />
                <a-input v-else v-model:value="item.custom_key" class="task-variable-key" placeholder="例如 artifact_url" />
                <a-input v-model:value="item.value" class="task-variable-value" placeholder="请输入变量值" />
                <a-button type="text" danger class="task-variable-remove" @click="removeEditTaskVariable(item.id)">
                  移除
                </a-button>
                <div v-if="resolvedVariableKey(item)" class="task-variable-preview">
                  脚本占位符：<code>{{ '{' + resolvedVariableKey(item) + '}' }}</code>
                </div>
              </div>
            </div>
            <a-button type="dashed" block @click="addEditTaskVariable">
              <template #icon><PlusOutlined /></template>
              新增变量
            </a-button>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-tabs v-model:activeKey="activeTaskTab" class="task-view-tabs">
      <a-tab-pane key="resident" tab="常驻任务">
        <a-card class="table-card" :bordered="false">
          <a-spin :spinning="refreshingTasks">
            <div class="task-list" v-if="filteredResidentTaskList.length">
              <div v-for="item in filteredResidentTaskList" :key="item.id" class="task-item resident-task-item">
                <div class="task-item-head">
                  <div>
                    <div class="task-name-row">
                      <div class="task-name">{{ item.name }}</div>
                      <a-tag color="purple">{{ taskModeText(item.task_mode) }}</a-tag>
                      <a-tag>{{ taskTypeText(item.task_type) }}</a-tag>
                    </div>
                    <div class="muted-text">
                      已分发 {{ item.resident_assigned_count || 0 }} 台 Agent
                      <span v-if="item.resident_running_count"> · 执行中 {{ item.resident_running_count }}</span>
                      <span v-if="item.resident_queued_count"> · 排队中 {{ item.resident_queued_count }}</span>
                      <span v-if="item.resident_claimed_count"> · 待启动 {{ item.resident_claimed_count }}</span>
                      <span v-if="item.resident_pending_count"> · 待下一轮 {{ item.resident_pending_count }}</span>
                      <span v-if="item.resident_cancelled_count"> · 已停止 {{ item.resident_cancelled_count }}</span>
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
                    <a-button
                      v-if="canManageAgent"
                      type="link"
                      size="small"
                      @click="handleEditResidentTask(item)"
                    >
                      <template #icon><EditOutlined /></template>
                      编辑
                    </a-button>
                    <a-button
                      v-if="canManageAgent"
                      type="link"
                      size="small"
                      @click="handleCloneResidentTask(item)"
                    >
                      <template #icon><CopyOutlined /></template>
                      克隆
                    </a-button>
                    <a-popconfirm
                      v-if="canManageAgent"
                      title="确认删除此常驻任务？"
                      description="删除后所有关联的运行实例都会被清理，此操作不可恢复"
                      @confirm="handleDeleteResidentTask(item.id)"
                    >
                      <a-button type="link" size="small" danger>
                        <template #icon><DeleteOutlined /></template>
                        删除
                      </a-button>
                    </a-popconfirm>
                    <a-button type="link" size="small" @click="openTaskPreview(item)">
                      <template #icon><EyeOutlined /></template>
                      预览任务
                    </a-button>
                    <a-tag :color="residentRuntimeColor(item)">{{ residentRuntimeText(item) }}</a-tag>
                  </a-space>
                </div>
                <div class="task-meta">
                  <span>目录：{{ item.work_dir }}</span>
                  <span>脚本：{{ item.script_name || item.script_path || '-' }}</span>
                  <span>超时：{{ item.timeout_sec }}s</span>
                  <span>最近结果：{{ taskStatusText(item.last_run_status || 'pending') }}</span>
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
                  <span v-if="!(item.resident_assigned_count || 0)">尚未下发到 Agent</span>
                  <span v-if="item.claimed_at">领取：{{ formatTime(item.claimed_at) }}</span>
                  <span v-if="item.started_at">开始：{{ formatTime(item.started_at) }}</span>
                  <span v-if="item.finished_at">结束：{{ formatTime(item.finished_at) }}</span>
                </div>
                <div v-if="item.last_run_summary" class="task-summary">{{ item.last_run_summary }}</div>
                <div v-if="item.failure_reason" class="task-error">{{ item.failure_reason }}</div>
              </div>
            </div>
            <a-empty v-else description="暂无常驻任务" />
          </a-spin>
        </a-card>
      </a-tab-pane>

      <a-tab-pane key="history" tab="临时任务">
        <a-card class="table-card" :bordered="false">
          <a-spin :spinning="refreshingTasks">
            <div class="task-list" v-if="pagedHistoryTaskList.length">
              <div v-for="item in pagedHistoryTaskList" :key="item.id" class="task-item">
                <div class="task-item-head">
                  <div>
                    <div class="task-name-row">
                      <div class="task-name">{{ item.name }}</div>
                      <a-tag>{{ taskModeText(item.task_mode) }}</a-tag>
                      <a-tag>{{ taskTypeText(item.task_type) }}</a-tag>
                    </div>
                    <div class="muted-text">
                      <a v-if="item.target_agent_ids && item.target_agent_ids.length" class="agent-link" @click="showBoundAgentsModal(item)">
                        {{ taskAgentBindingText(item) }}
                      </a>
                      <span v-else>{{ taskAgentBindingText(item) }}</span>
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
                    <a-button
                      v-if="canManageAgent && canEditTemporaryTask(item)"
                      type="link"
                      size="small"
                      @click="handleEditTemporaryTask(item)"
                    >
                      <template #icon><EditOutlined /></template>
                      编辑
                    </a-button>
                    <a-button
                      v-if="canManageAgent && !item.source_task_id"
                      type="link"
                      size="small"
                      @click="handleCloneTemporaryTask(item)"
                    >
                      <template #icon><CopyOutlined /></template>
                      克隆
                    </a-button>
                    <a-popconfirm
                      v-if="canManageAgent && canDeleteTemporaryTask(item)"
                      title="确认删除此临时任务？"
                      description="删除后无法恢复"
                      @confirm="handleDeleteTemporaryTask(item.id)"
                    >
                      <a-button type="link" size="small" danger>
                        <template #icon><DeleteOutlined /></template>
                        删除
                      </a-button>
                    </a-popconfirm>
                    <a-button type="link" size="small" @click="openTaskPreview(item)">
                      <template #icon><EyeOutlined /></template>
                      预览任务
                    </a-button>
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
            <a-empty v-else description="暂无历史任务" />
          </a-spin>
          <div v-if="filteredHistoryTaskList.length" class="history-pagination">
            <a-pagination
              :current="historyFilters.page"
              :page-size="historyFilters.page_size"
              :total="filteredHistoryTaskList.length"
              :show-size-changer="false"
              @change="handleHistoryPageChange"
            />
          </div>
        </a-card>
      </a-tab-pane>
    </a-tabs>

    <a-modal
      :open="previewTaskVisible"
      :title="taskPreviewTitle(previewTask)"
      :footer="null"
      :width="860"
      @cancel="closeTaskPreview"
    >
      <template v-if="previewTask">
        <div class="task-preview-meta">
          <a-tag>{{ taskModeText(previewTask.task_mode) }}</a-tag>
          <a-tag>{{ taskTypeText(previewTask.task_type) }}</a-tag>
          <span>Agent：{{ taskAgentBindingText(previewTask) }}</span>
          <span>目录：{{ previewTask.work_dir || '-' }}</span>
          <span>脚本：{{ previewTask.script_name || previewTask.script_path || '-' }}</span>
        </div>

        <div v-if="Object.keys(previewTask.variables || {}).length" class="task-preview-section">
          <div class="task-output-title">变量配置</div>
          <div class="task-preview-vars">
            <a-tag v-for="(value, key) in previewTask.variables" :key="key">{{ key }} = {{ value }}</a-tag>
          </div>
        </div>

        <div class="task-preview-section">
          <div class="task-output-title">{{ taskContentLabel(previewTask) }}</div>
          <pre class="task-log">{{ previewTask.script_text || '暂无任务内容' }}</pre>
        </div>
      </template>
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
          { 
            title: '状态', 
            key: 'runtime_state', 
            width: 100,
            customRender: ({ record }: any) => {
              const stateColors: Record<string, string> = {
                online: 'green',
                offline: 'default',
                busy: 'orange',
                disabled: 'red',
                maintenance: 'blue',
              }
              const stateText: Record<string, string> = {
                online: '在线',
                offline: '离线',
                busy: '忙碌',
                disabled: '禁用',
                maintenance: '维护',
              }
              return {
                children: stateText[record.runtime_state] || record.runtime_state,
                tagProps: { color: stateColors[record.runtime_state] || 'default' },
              }
            }
          },
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
      :title="editResidentTaskCloneSourceID ? '克隆常驻任务' : '编辑常驻任务'"
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

        <a-form-item label="执行变量">
          <div class="task-variable-panel">
            <div class="task-variable-tip">
              编辑常驻任务时同样支持标准平台 Key 和自定义变量；脚本里继续使用 <code>{env}</code> 这样的占位符
            </div>
            <div class="task-variable-list-editor">
              <div v-for="item in editResidentTaskVariables" :key="item.id" class="task-variable-row">
                <a-select
                  :value="item.key_mode"
                  class="task-variable-mode"
                  @update:value="(value) => handleVariableModeChangeFor(item, value as 'platform' | 'custom')"
                >
                  <a-select-option value="platform">标准平台 Key</a-select-option>
                  <a-select-option value="custom">自定义变量</a-select-option>
                </a-select>
                <a-select
                  v-if="item.key_mode === 'platform'"
                  v-model:value="item.platform_key"
                  class="task-variable-key"
                  show-search
                  allow-clear
                  placeholder="选择标准平台 Key"
                  :filter-option="(input: string, option: any) => String(option?.label || '').toLowerCase().includes(input.toLowerCase())"
                  :options="platformParamOptions.map((option) => ({ value: option.param_key, label: platformParamLabel(option.param_key) }))"
                />
                <a-input v-else v-model:value="item.custom_key" class="task-variable-key" placeholder="例如 artifact_url" />
                <a-input v-model:value="item.value" class="task-variable-value" placeholder="请输入变量值" />
                <a-button type="text" danger class="task-variable-remove" @click="removeEditResidentTaskVariable(item.id)">
                  移除
                </a-button>
                <div v-if="resolvedVariableKey(item)" class="task-variable-preview">
                  脚本占位符：<code>{{ '{' + resolvedVariableKey(item) + '}' }}</code>
                </div>
              </div>
            </div>
            <a-button type="dashed" block @click="addEditResidentTaskVariable">
              <template #icon><PlusOutlined /></template>
              新增变量
            </a-button>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: var(--space-6);
}

.page-header-actions {
  --task-header-action-bg: rgba(255, 255, 255, 0.42);
  --task-header-action-bg-hover: rgba(255, 255, 255, 0.56);
  --task-header-action-border: rgba(148, 163, 184, 0.28);
  --task-header-action-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.68), 0 10px 22px rgba(15, 23, 42, 0.05);
  --task-header-action-filter: blur(14px) saturate(135%);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
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

.task-form-modal-wrap :deep(.ant-modal-content) {
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

.task-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
}

.task-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.task-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

.task-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.task-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

:deep(.task-form-modal-save-btn.ant-btn) {
  flex: none;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: normal;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
}

.task-edit-modal-wrap :deep(.ant-modal-footer) {
  margin-top: 14px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.task-edit-modal-wrap :deep(.ant-modal-footer .ant-btn) {
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  font-weight: 600;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

.task-edit-modal-wrap :deep(.ant-modal-footer .ant-btn-primary) {
  border-color: rgba(96, 165, 250, 0.42) !important;
  background: rgba(96, 165, 250, 0.12) !important;
  color: #2563eb !important;
}

.task-edit-modal-wrap :deep(.ant-modal-footer .ant-btn:hover),
.task-edit-modal-wrap :deep(.ant-modal-footer .ant-btn:focus) {
  border-color: rgba(96, 165, 250, 0.48) !important;
  background: rgba(255, 255, 255, 0.56) !important;
}

.task-create-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.task-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.task-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.task-form-panel {
  padding: 0;
}

.task-form-note + .task-form-panel,
.task-form-panel + .task-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.task-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.task-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.task-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.task-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.task-create-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.task-create-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.task-create-form :deep(.ant-input),
.task-create-form :deep(.ant-input-affix-wrapper),
.task-create-form :deep(.ant-input-number),
.task-create-form :deep(.ant-select-selector),
.task-create-form :deep(.ant-input-textarea textarea) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.task-create-form :deep(.ant-input:hover),
.task-create-form :deep(.ant-input-number:hover),
.task-create-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.task-create-form :deep(.ant-input:focus),
.task-create-form :deep(.ant-input-focused),
.task-create-form :deep(.ant-input-number-focused),
.task-create-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.task-create-form :deep(.ant-input[readonly]) {
  color: #475569;
}

.task-agent-selector,
.task-script-summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-agent-selection,
.task-script-summary-meta,
.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  color: var(--color-text-secondary);
  font-size: 12px;
}

.task-script-summary {
  margin-top: -2px;
  padding: 0 0 12px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.task-script-summary-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.task-script-summary-title {
  color: var(--color-text-main);
  font-weight: 700;
}

.task-variable-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-variable-tip {
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.6;
  padding: 0;
}

.task-tip-link {
  padding-inline: 6px;
}

.task-variable-list-editor {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-variable-row {
  display: grid;
  grid-template-columns: 132px minmax(0, 1.1fr) minmax(0, 1.3fr) 64px;
  gap: 10px;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.task-variable-mode,
.task-variable-key,
.task-variable-value {
  width: 100%;
}

.task-variable-remove {
  justify-self: end;
  padding-inline: 0;
}

.task-variable-preview {
  grid-column: 1 / -1;
  color: var(--color-text-secondary);
  font-size: 12px;
}

.task-view-tabs {
  margin-top: 0;
}

.table-card {
  overflow: visible;
  border-radius: 20px;
  border: none;
  background: transparent;
  box-shadow: none;
}

.table-card :deep(.ant-card-body) {
  padding: 0;
}

:deep(.application-toolbar-icon-btn.ant-btn),
:deep(.application-toolbar-action-btn.ant-btn),
:deep(.component-toolbar-reset-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  padding-inline: 14px;
  border-radius: 16px;
  border: 1px solid var(--task-header-action-border) !important;
  background: var(--task-header-action-bg) !important;
  color: #0f172a !important;
  font-weight: 600;
  box-shadow: var(--task-header-action-shadow) !important;
  backdrop-filter: var(--task-header-action-filter);
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  min-width: 42px;
  padding-inline: 0;
}

:deep(.application-toolbar-action-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.component-toolbar-reset-btn.ant-btn:hover),
:deep(.component-toolbar-reset-btn.ant-btn:focus),
:deep(.component-toolbar-reset-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: var(--task-header-action-bg-hover) !important;
  color: #0f172a !important;
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

.history-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
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

.resident-task-item {
  border-color: rgba(59, 130, 246, 0.16);
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

.task-meta-secondary,
.muted-text {
  color: var(--color-text-secondary);
  font-size: 12px;
}

.task-summary {
  color: var(--color-text-main);
  font-size: 12px;
}

.task-error {
  color: var(--color-danger);
  background: var(--color-danger-bg);
  border: 1px solid rgba(220, 38, 38, 0.16);
  border-radius: 12px;
  padding: 10px 12px;
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

.task-preview-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 12px;
  margin-bottom: 16px;
  color: var(--color-text-secondary);
  font-size: 13px;
}

.task-preview-section + .task-preview-section {
  margin-top: 16px;
}

.task-preview-vars {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 900px) {
  .page-header,
  .task-form-header,
  .selected-script-head,
  .task-item-head {
    flex-direction: column;
    align-items: flex-start;
  }

  .task-variable-row {
    grid-template-columns: 1fr;
  }

  .page-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .application-search-overlay {
    left: 0;
    padding-inline: 16px;
  }
}
</style>
