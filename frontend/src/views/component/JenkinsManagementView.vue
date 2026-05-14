<script setup lang="ts">
import { DeleteOutlined, DownloadOutlined, EditOutlined, FileSearchOutlined, FileTextOutlined, MoreOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  createJenkinsRawPipeline,
  deleteJenkinsRawPipeline,
  getPipelineOriginalLink,
  getPipelineRawScript,
  listPipelines,
  previewJenkinsRawPipelineConfigXML,
  syncJenkinsPipelines,
  syncJenkinsExecutorParamDefs,
  updateJenkinsRawPipeline,
} from '../../api/pipeline'
import { getPipelineScanResult, getPipelineScanRule, scanAllPipelines, scanPipeline } from '../../api/pipeline-scan'
import { useResizableColumns } from '../../composables/useResizableColumns'
import type { Pipeline, PipelineRawScriptData, PipelineStatus } from '../../types/pipeline'
import type { PipelineScanFinding, PipelineScanResult, PipelineScanSeverity } from '../../types/pipeline-scan'
import { useAuthStore } from '../../stores/auth'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const authStore = useAuthStore()

const loading = ref(false)
const dataSource = ref<Pipeline[]>([])
const total = ref(0)
const scriptVisible = ref(false)
const scriptLoading = ref(false)
const scriptData = ref<PipelineRawScriptData | null>(null)
const scriptPipelineName = ref('')
const scriptLocationFindings = ref<PipelineScanFinding[]>([])
const scriptFocusLineNo = ref(0)
const scriptSelectedFindingID = ref('')
const scriptOverrideText = ref('')
const scriptPendingFindingID = ref('')
const overwritingFindingID = ref('')
const submittingOverwrite = ref(false)
const scanPopoverOpenID = ref('')
const editorVisible = ref(false)
const editorLoading = ref(false)
const submitting = ref(false)
const deletingID = ref('')
const downloadingID = ref('')
const configVisible = ref(false)
const configLoading = ref(false)
const configTitle = ref('')
const configXML = ref('')
const previewingConfig = ref(false)
const syncing = ref(false)
const scanningID = ref('')
const scanningAll = ref(false)
const scanResults = ref<Record<string, PipelineScanResult | undefined>>({})
const scanFindings = ref<Record<string, PipelineScanFinding[] | undefined>>({})
const editorMode = ref<'create' | 'edit'>('create')
const formRef = ref<FormInstance>()
const searchDialogVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
const editorModalViewportInset = ref(0)
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0
let editorModalViewportObserver: ResizeObserver | null = null

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

const filters = reactive({
  name: '',
  status: '' as PipelineStatus | '',
  page: 1,
  pageSize: 20,
})

const searchDraft = reactive({
  keyword: '',
})

const statusFilterValue = computed<PipelineStatus | ''>({
  get: () => filters.status,
  set: (value) => {
    filters.status = value === 'active' || value === 'inactive' ? value : ''
  },
})

const editorForm = reactive({
  id: '',
  full_name: '',
  description: '',
  script: '',
  sandbox: true,
})

const canManagePipeline = computed(() => authStore.hasPermission('pipeline.manage'))
const canSyncJenkins = computed(
  () => authStore.hasPermission('pipeline.manage') || authStore.hasPermission('pipeline_param.manage'),
)

const formRules: Record<string, Array<{ required: boolean; message: string; trigger: string }>> = {
  full_name: [{ required: true, message: '请输入 Jenkins 路径', trigger: 'blur' }],
  script: [{ required: true, message: '请输入原始管线脚本', trigger: 'blur' }],
}

const initialColumns: TableColumnsType<Pipeline> = [
  { title: '管线名称', dataIndex: 'job_name', key: 'job_name', width: 220 },
  { title: 'Jenkins路径', dataIndex: 'job_full_name', key: 'job_full_name', width: 280 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 120 },
  { title: '规范状态', dataIndex: 'scan_status', key: 'scan_status', width: 200 },
  { title: '最近同步时间', dataIndex: 'last_synced_at', key: 'last_synced_at', width: 190 },
  { title: '最近校验时间', dataIndex: 'last_verified_at', key: 'last_verified_at', width: 190 },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 190 },
  { title: '操作', key: 'actions', width: 358, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, { minWidth: 120, maxWidth: 560, hitArea: 10 })

const tableLocale = computed(() => ({
  emptyText: filters.name.trim() ? '未找到匹配的 Jenkins 管线' : '暂无 Jenkins 管线',
}))
const scriptModalMaskStyle = computed(() => buildJenkinsModalMaskStyle(scriptVisible.value))
const scriptModalWrapProps = computed(() => buildJenkinsModalWrapProps(scriptVisible.value))
const configModalMaskStyle = computed(() => buildJenkinsModalMaskStyle(configVisible.value))
const configModalWrapProps = computed(() => buildJenkinsModalWrapProps(configVisible.value))
const editorModalMaskStyle = computed(() => buildJenkinsModalMaskStyle(editorVisible.value))
const editorModalWrapProps = computed(() => buildJenkinsModalWrapProps(editorVisible.value))

const ruleCommandTemplateCache = new Map<string, string>()

function buildJenkinsModalMaskStyle(visible: boolean) {
  return {
    left: `${editorModalViewportInset.value}px`,
    width: `calc(100% - ${editorModalViewportInset.value}px)`,
    background: 'rgba(15, 23, 42, 0.08)',
    backdropFilter: 'blur(10px)',
    WebkitBackdropFilter: 'blur(10px)',
    pointerEvents: visible ? 'auto' : 'none',
  }
}

function buildJenkinsModalWrapProps(visible: boolean) {
  return {
    style: {
      left: `${editorModalViewportInset.value}px`,
      width: `calc(100% - ${editorModalViewportInset.value}px)`,
      pointerEvents: visible ? 'auto' : 'none',
    },
  }
}

const displayScript = computed(() => {
  if (scriptOverrideText.value) {
    return scriptOverrideText.value
  }
  if (!scriptData.value) {
    return ''
  }
  const text = String(scriptData.value.script || '')
  if (text.trim()) {
    return text
  }
  if (scriptData.value.from_scm) {
    const scriptPath = String(scriptData.value.script_path || 'Jenkinsfile').trim()
    return `该 Jenkins 管线使用 SCM 脚本模式，脚本路径：${scriptPath}\n请到对应代码仓库查看脚本内容`
  }
  return '未解析到脚本内容'
})

const displayScriptLines = computed(() => {
  return displayScript.value.split('\n').map((text, index) => ({
    lineNo: index + 1,
    text,
  }))
})

const selectedScriptFinding = computed(() => {
  if (!scriptSelectedFindingID.value) {
    return scriptLocationFindings.value[0]
  }
  return scriptLocationFindings.value.find((finding) => finding.id === scriptSelectedFindingID.value) || scriptLocationFindings.value[0]
})

function formatTime(value: string | null) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function statusColor(status: PipelineStatus) {
  if (status === 'active') {
    return 'green'
  }
  return 'default'
}

function pipelineScanSeverityColor(severity: PipelineScanSeverity) {
  if (severity === 'error') {
    return 'red'
  }
  if (severity === 'warning') {
    return 'orange'
  }
  return 'blue'
}

function pipelineScanSeverityText(severity: PipelineScanSeverity) {
  if (severity === 'error') {
    return '问题'
  }
  if (severity === 'warning') {
    return '警告'
  }
  return '提示'
}

function pipelineScanResult(record: Pipeline) {
  return scanResults.value[record.id]
}

function pipelineScanFindings(record: Pipeline) {
  return scanFindings.value[record.id] || []
}

function pipelineScanSummary(record: Pipeline) {
  const result = pipelineScanResult(record)
  if (!result) {
    return '暂无扫描结果'
  }
  if (result.scan_status === 'unknown') {
    return '脚本为空或未解析'
  }
  if (result.total_findings === 0) {
    return '无问题'
  }
  return `E${result.error_count} / W${result.warning_count} / I${result.info_count}`
}

function pipelineScanSummaryClass(record: Pipeline) {
  const result = pipelineScanResult(record)
  return {
    'is-empty': !result,
    'is-clean': result?.total_findings === 0,
    'is-problem': !!result && result.total_findings > 0,
    'is-unknown': result?.scan_status === 'unknown',
  }
}

function pipelineScanEmptyDetail(record: Pipeline) {
  const result = pipelineScanResult(record)
  if (!result) {
    return '当前管线还没有扫描结果'
  }
  if (result.scan_status === 'unknown') {
    return '当前管线脚本为空或无法解析，暂时无法判断规范状态'
  }
  return '当前管线未命中规范问题'
}

function findingEndLineNo(finding: PipelineScanFinding) {
  const rawText = String(finding.matched_text || '')
  if (!rawText.trim()) {
    return finding.line_no
  }
  const lineCount = rawText.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n').length
  return finding.line_no + Math.max(lineCount, 1) - 1
}

function isLineInFindingRange(lineNo: number, finding: PipelineScanFinding | undefined) {
  if (!finding || finding.line_no <= 0) {
    return false
  }
  return lineNo >= finding.line_no && lineNo <= findingEndLineNo(finding)
}

function scriptLineRangeFindings(lineNo: number) {
  return scriptLocationFindings.value.filter((finding) => {
    if (finding.line_no <= 0) {
      return false
    }
    return lineNo >= finding.line_no && lineNo <= findingEndLineNo(finding)
  })
}

function strongestScanSeverity(findings: PipelineScanFinding[]) {
  if (findings.some((finding) => finding.severity === 'error')) {
    return 'error'
  }
  if (findings.some((finding) => finding.severity === 'warning')) {
    return 'warning'
  }
  if (findings.some((finding) => finding.severity === 'info')) {
    return 'info'
  }
  return ''
}

function scriptLineClass(lineNo: number) {
  const rangeFindings = scriptLineRangeFindings(lineNo)
  const severity = strongestScanSeverity(rangeFindings)
  return {
    'is-annotated': rangeFindings.length > 0,
    'is-error': severity === 'error',
    'is-warning': severity === 'warning',
    'is-info': severity === 'info',
    'is-selected-range': isLineInFindingRange(lineNo, selectedScriptFinding.value),
  }
}

function scriptFindingBadgeLabel(finding: PipelineScanFinding) {
  const findingIndex = scriptLocationFindings.value.indexOf(finding)
  const scopedFindings = findingIndex >= 0 ? scriptLocationFindings.value.slice(0, findingIndex + 1) : [finding]
  const sameSeverityCount = scopedFindings
    .filter((item) => item.severity === finding.severity).length
  return `${pipelineScanSeverityText(finding.severity)} ${sameSeverityCount}`
}

function scriptFindingBadgeClass(severity: PipelineScanSeverity) {
  return {
    'is-error': severity === 'error',
    'is-warning': severity === 'warning',
    'is-info': severity === 'info',
  }
}

function scriptFindingRangeLabel(finding: PipelineScanFinding) {
  if (finding.line_no <= 0) {
    return '未返回行号'
  }
  const endLine = findingEndLineNo(finding)
  if (endLine > finding.line_no) {
    return `第 ${finding.line_no}-${endLine} 行`
  }
  return `第 ${finding.line_no} 行`
}

function scriptFindingItemClass(finding: PipelineScanFinding) {
  return {
    ...scriptFindingBadgeClass(finding.severity),
    'is-selected': selectedScriptFinding.value?.id === finding.id,
  }
}

async function selectScriptFinding(finding: PipelineScanFinding) {
  scriptSelectedFindingID.value = finding.id
  scriptFocusLineNo.value = finding.line_no > 0 ? finding.line_no : 0
  await nextTick()
  revealScriptFocusLine()
}

function extractRuleCommandTemplate(rawDSL: string) {
  try {
    const parsed = JSON.parse(rawDSL || '{}')
    return String(parsed?.matcher?.format?.source_text || '').trim()
  } catch {
    return ''
  }
}

async function loadRuleCommandTemplate(finding: PipelineScanFinding) {
  const ruleID = String(finding.rule_id || '').trim()
  if (!ruleID) {
    return ''
  }
  if (ruleCommandTemplateCache.has(ruleID)) {
    return ruleCommandTemplateCache.get(ruleID) || ''
  }
  const response = await getPipelineScanRule(ruleID)
  const template = extractRuleCommandTemplate(response.data.rule_dsl_json)
  ruleCommandTemplateCache.set(ruleID, template)
  return template
}

function replaceScriptLineRange(script: string, startLine: number, endLine: number, replacement: string) {
  const normalizedScript = String(script || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const normalizedReplacement = String(replacement || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const lines = normalizedScript.split('\n')
  const replacementLines = normalizedReplacement.split('\n')
  const startIndex = Math.max(startLine - 1, 0)
  const endIndex = Math.max(endLine, startIndex + 1)
  return [...lines.slice(0, startIndex), ...replacementLines, ...lines.slice(endIndex)].join('\n')
}

async function previewScriptOverwrite(finding: PipelineScanFinding) {
  if (!canManagePipeline.value) {
    message.warning('当前账号没有管线管理权限，无法覆盖')
    return
  }
  if (scriptPendingFindingID.value && scriptPendingFindingID.value !== finding.id) {
    message.warning('请先撤回当前覆盖预览，再处理其他问题')
    return
  }
  if (!scriptData.value) {
    message.warning('原始脚本未加载完成，无法覆盖')
    return
  }
  if (scriptData.value.from_scm) {
    message.warning('当前管线为 SCM 脚本模式，无法在平台内直接覆盖')
    return
  }
  if (finding.line_no <= 0) {
    message.warning('当前问题没有明确行号，无法覆盖')
    return
  }
  overwritingFindingID.value = finding.id
  try {
    const replacement = await loadRuleCommandTemplate(finding)
    if (!replacement) {
      message.warning('当前规则没有配置可覆盖的标准命令')
      return
    }
    const updatedScript = replaceScriptLineRange(displayScript.value, finding.line_no, findingEndLineNo(finding), replacement)
    scriptOverrideText.value = updatedScript
    scriptPendingFindingID.value = finding.id
    message.success('已预览覆盖当前批注范围，提交后才会保存管线')
    await nextTick()
    revealScriptFocusLine()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '生成覆盖预览失败'))
  } finally {
    overwritingFindingID.value = ''
  }
}

async function submitScriptOverwrite(finding: PipelineScanFinding) {
  if (!canManagePipeline.value) {
    message.warning('当前账号没有管线管理权限，无法提交')
    return
  }
  if (!scriptData.value) {
    message.warning('原始脚本未加载完成，无法提交')
    return
  }
  if (scriptPendingFindingID.value !== finding.id || !scriptOverrideText.value) {
    message.warning('请先生成覆盖预览')
    return
  }
  submittingOverwrite.value = true
  const pipelineID = scriptData.value.pipeline.id
  const updatedScript = scriptOverrideText.value
  try {
    await updateJenkinsRawPipeline(pipelineID, {
      description: scriptData.value.description || '',
      script: updatedScript,
      sandbox: scriptData.value.sandbox,
    })
    scriptData.value = {
      ...scriptData.value,
      script: updatedScript,
    }
    scriptOverrideText.value = ''
    scriptPendingFindingID.value = ''
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '提交覆盖失败'))
    submittingOverwrite.value = false
    return
  }

  try {
    const response = await scanPipeline(pipelineID)
    scanResults.value = {
      ...scanResults.value,
      [pipelineID]: response.data.result,
    }
    scanFindings.value = {
      ...scanFindings.value,
      [pipelineID]: response.data.findings,
    }
    scriptLocationFindings.value = response.data.findings
    const nextFinding = response.data.findings.find((item) => item.line_no > 0) || response.data.findings[0]
    scriptSelectedFindingID.value = nextFinding?.id || ''
    scriptFocusLineNo.value = nextFinding && nextFinding.line_no > 0 ? nextFinding.line_no : 0
    if (response.data.findings.length > 0) {
      message.success(`已提交并重新扫描，剩余 ${response.data.findings.length} 条问题`)
    } else {
      message.success('已提交并重新扫描，当前管线未发现问题')
    }
    await nextTick()
    revealScriptFocusLine()
  } catch (error) {
    message.warning(extractHTTPErrorMessage(error, '覆盖已提交，重新扫描失败'))
  } finally {
    submittingOverwrite.value = false
  }
}

async function withdrawScriptOverwrite() {
  if (!scriptPendingFindingID.value) {
    return
  }
  const finding = scriptLocationFindings.value.find((item) => item.id === scriptPendingFindingID.value)
  scriptOverrideText.value = ''
  scriptPendingFindingID.value = ''
  message.info('已撤回覆盖预览')
  if (finding) {
    await nextTick()
    await selectScriptFinding(finding)
  }
}

function clearScriptLocation() {
  scriptLocationFindings.value = []
  scriptFocusLineNo.value = 0
  scriptSelectedFindingID.value = ''
  scriptOverrideText.value = ''
  scriptPendingFindingID.value = ''
  overwritingFindingID.value = ''
  submittingOverwrite.value = false
}

function revealScriptFocusLine() {
  const lineNo = scriptFocusLineNo.value
  if (lineNo <= 0 || typeof window === 'undefined') {
    return
  }
  window.setTimeout(() => {
    const target = document.querySelector(`[data-script-line="${lineNo}"]`)
    target?.scrollIntoView({ block: 'center' })
  }, 80)
}

function handleScanPopoverOpenChange(recordID: string, open: boolean) {
  scanPopoverOpenID.value = open ? recordID : ''
}

function getPipelineName(record: Pipeline) {
  return record.job_name || record.job_full_name || '-'
}

function readEditorModalViewportInset() {
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

function syncEditorModalViewportInset() {
  editorModalViewportInset.value = readEditorModalViewportInset()
}

function observeEditorModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }
  editorModalViewportObserver?.disconnect()
  editorModalViewportObserver = new ResizeObserver(syncEditorModalViewportInset)
  if (appLayout) {
    editorModalViewportObserver.observe(appLayout)
  }
  if (sider) {
    editorModalViewportObserver.observe(sider)
  }
}

function stopObservingEditorModalViewportInset() {
  editorModalViewportObserver?.disconnect()
  editorModalViewportObserver = null
}

function closeScriptModal() {
  scriptVisible.value = false
  scriptLoading.value = false
  scriptData.value = null
  scriptPipelineName.value = ''
  clearScriptLocation()
}

function closeConfigModal() {
  configVisible.value = false
  configLoading.value = false
  configTitle.value = ''
  configXML.value = ''
}

function resetEditorForm() {
  editorForm.id = ''
  editorForm.full_name = ''
  editorForm.description = ''
  editorForm.script = ''
  editorForm.sandbox = true
}

function closeEditorModal() {
  editorVisible.value = false
  editorLoading.value = false
  submitting.value = false
  resetEditorForm()
}

function openCreateModal() {
  editorMode.value = 'create'
  resetEditorForm()
  editorVisible.value = true
}

async function openOriginalLink(record: Pipeline) {
  const directTarget = String(record.job_url || '').trim()
  if (directTarget) {
    window.open(directTarget, '_blank', 'noopener,noreferrer')
    return
  }

  try {
    const response = await getPipelineOriginalLink(record.id)
    const target = String(response.data.original_link || '').trim()
    if (!target) {
      message.warning('当前管线缺少 Jenkins 原始链接')
      return
    }
    window.open(target, '_blank', 'noopener,noreferrer')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '打开 Jenkins 原始链接失败'))
  }
}

async function openScriptModal(record: Pipeline, options: { locate?: boolean } = {}) {
  clearScriptLocation()
  if (options.locate) {
    const findings = pipelineScanFindings(record)
    scriptLocationFindings.value = findings
    const firstFinding = findings.find((finding) => finding.line_no > 0) || findings[0]
    scriptSelectedFindingID.value = firstFinding?.id || ''
    scriptFocusLineNo.value = firstFinding?.line_no && firstFinding.line_no > 0 ? firstFinding.line_no : 0
  }
  scriptVisible.value = true
  scriptLoading.value = true
  scriptData.value = null
  scriptPipelineName.value = record.job_name || record.job_full_name || record.id
  try {
    const response = await getPipelineRawScript(record.id)
    scriptData.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载管线原始脚本失败'))
    closeScriptModal()
    return
  } finally {
    scriptLoading.value = false
  }
  await nextTick()
  revealScriptFocusLine()
}

async function openScriptLocation(record: Pipeline) {
  const findings = pipelineScanFindings(record)
  if (findings.length === 0) {
    message.info('当前管线没有可定位的问题')
    return
  }
  scanPopoverOpenID.value = ''
  await openScriptModal(record, { locate: true })
}

async function openEditModal(record: Pipeline) {
  if (record.status !== 'active') {
    message.info('失效管线暂不支持编辑，请先在 Jenkins 中恢复或重新创建')
    return
  }
  editorMode.value = 'edit'
  editorLoading.value = true
  resetEditorForm()
  try {
    const response = await getPipelineRawScript(record.id)
    if (response.data.from_scm) {
      message.warning('当前管线为 SCM 模式，暂不支持在平台内直接编辑原始脚本')
      return
    }
    editorForm.id = record.id
    editorForm.full_name = response.data.pipeline.job_full_name
    editorForm.description = response.data.description || ''
    editorForm.script = response.data.script || ''
    editorForm.sandbox = response.data.sandbox
    editorVisible.value = true
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载可编辑管线失败'))
  } finally {
    editorLoading.value = false
  }
}

async function loadPipelines(options: { waitForScanResults?: boolean } = {}) {
  loading.value = true
  try {
    const response = await listPipelines({
      provider: 'jenkins',
      name: filters.name.trim() || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
    const scanResultsPromise = loadPipelineScanResults(response.data)
    if (options.waitForScanResults) {
      await scanResultsPromise
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Jenkins管线加载失败'))
  } finally {
    loading.value = false
  }
}

async function loadPipelineScanResults(items: Pipeline[]) {
  const nextResults: Record<string, PipelineScanResult | undefined> = {}
  const nextFindings: Record<string, PipelineScanFinding[] | undefined> = {}
  await Promise.all(
    items.map(async (item) => {
      try {
        const response = await getPipelineScanResult(item.id)
        nextResults[item.id] = response.data.result
        nextFindings[item.id] = response.data.findings
      } catch {
        nextResults[item.id] = undefined
        nextFindings[item.id] = undefined
      }
    }),
  )
  scanResults.value = nextResults
  scanFindings.value = nextFindings
}

async function handleScanPipeline(record: Pipeline) {
  if (record.status !== 'active') {
    message.info('失效管线暂不支持扫描')
    return
  }
  scanningID.value = record.id
  try {
    const response = await scanPipeline(record.id)
    scanResults.value = {
      ...scanResults.value,
      [record.id]: response.data.result,
    }
    scanFindings.value = {
      ...scanFindings.value,
      [record.id]: response.data.findings,
    }
    message.success(`扫描完成：${response.data.result.total_findings} 个问题`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '管线规范扫描失败'))
  } finally {
    scanningID.value = ''
  }
}

async function handleScanAllPipelines() {
  if (!canManagePipeline.value) {
    message.warning('当前账号没有管线扫描权限')
    return
  }
  scanningAll.value = true
  try {
    const response = await scanAllPipelines()
    await loadPipelines({ waitForScanResults: true })
    const result = response.data
    const summary = `总数 ${result.total} / 已扫描 ${result.scanned} / 跳过 ${result.skipped} / 失败 ${result.failed}`
    if (result.failed > 0) {
      message.warning(`扫描完成，存在失败：${summary}`)
      return
    }
    message.success(`扫描完成：${summary}`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '管线规范扫描失败'))
  } finally {
    scanningAll.value = false
  }
}

async function handleManualSync() {
  syncing.value = true
  const summaries: string[] = []
  try {
    if (authStore.hasPermission('pipeline.manage')) {
      const pipelineResult = await syncJenkinsPipelines()
      await loadPipelines()
      summaries.push(
        `管线 ${pipelineResult.data.total} 条（新增 ${pipelineResult.data.created} / 更新 ${pipelineResult.data.updated} / 失效 ${pipelineResult.data.inactivated} / 跳过 ${pipelineResult.data.skipped}）`,
      )
    }
    if (authStore.hasPermission('pipeline.manage') || authStore.hasPermission('pipeline_param.manage')) {
      const paramResult = await syncJenkinsExecutorParamDefs()
      summaries.push(
        `参数 ${paramResult.data.total} 条（新增 ${paramResult.data.created} / 更新 ${paramResult.data.updated} / 失效 ${paramResult.data.inactivated} / 跳过 ${paramResult.data.skipped}）`,
      )
    }
    if (summaries.length === 0) {
      message.warning('当前账号没有 Jenkins 手动同步权限')
      return
    }
    message.success(`手动同步完成：${summaries.join('；')}`)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Jenkins 手动同步失败'))
  } finally {
    syncing.value = false
  }
}

function handleSearch() {
  filters.page = 1
  void loadPipelines()
}

function clearSearchSuggestions() {
  if (searchSuggestionTimer) {
    window.clearTimeout(searchSuggestionTimer)
    searchSuggestionTimer = null
  }
  searchSuggestionRequestSeq += 1
  searchSuggestions.value = []
  searchSuggestionsLoading.value = false
}

function openSearchDialog() {
  searchDraft.keyword = filters.name.trim()
  searchDialogVisible.value = true
  void nextTick(() => {
    searchInputRef.value?.focus()
  })
}

function closeSearchDialog() {
  searchDialogVisible.value = false
  clearSearchSuggestions()
}

async function fetchSearchSuggestions(keyword: string) {
  const normalizedKeyword = keyword.trim()
  if (!normalizedKeyword) {
    clearSearchSuggestions()
    return
  }
  const requestSeq = ++searchSuggestionRequestSeq
  searchSuggestionsLoading.value = true
  try {
    const response = await listPipelines({
      provider: 'jenkins',
      name: normalizedKeyword,
      status: filters.status || undefined,
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: item.id,
      title: item.job_name || item.job_full_name || item.id,
      subtitle: `${item.job_full_name || '-'} · ${item.status}`,
      query: item.job_name || item.job_full_name || normalizedKeyword,
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
  filters.name = searchDraft.keyword.trim()
  filters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadPipelines()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  filters.name = item.query
  filters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadPipelines()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.pageSize = pageSize
  void loadPipelines()
}

async function submitEditor() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    if (editorMode.value === 'create') {
      await createJenkinsRawPipeline({
        full_name: editorForm.full_name.trim(),
        description: editorForm.description.trim() || undefined,
        script: editorForm.script,
        sandbox: editorForm.sandbox,
      })
      message.success('原始管线创建成功')
    } else {
      await updateJenkinsRawPipeline(editorForm.id, {
        description: editorForm.description.trim() || undefined,
        script: editorForm.script,
        sandbox: editorForm.sandbox,
      })
      message.success('原始管线更新成功')
    }
    closeEditorModal()
    filters.page = 1
    await loadPipelines()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editorMode.value === 'create' ? '原始管线创建失败' : '原始管线更新失败'))
  } finally {
    submitting.value = false
  }
}

async function previewConfigFromForm() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  previewingConfig.value = true
  try {
    const response = await previewJenkinsRawPipelineConfigXML({
      full_name: editorForm.full_name.trim(),
      description: editorForm.description.trim() || undefined,
      script: editorForm.script,
      sandbox: editorForm.sandbox,
    })
    configTitle.value = editorMode.value === 'create' ? `预览配置XML - ${editorForm.full_name.trim()}` : `预览配置XML - ${editorForm.full_name}`
    configXML.value = response.data.config_xml || ''
    configVisible.value = true
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '预览配置XML失败'))
  } finally {
    previewingConfig.value = false
  }
}

async function handleDelete(record: Pipeline) {
  deletingID.value = record.id
  try {
    await deleteJenkinsRawPipeline(record.id)
    message.success('原始管线删除成功')
    await loadPipelines()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除原始管线失败'))
  } finally {
    deletingID.value = ''
  }
}

async function handleDownloadPipeline(record: Pipeline) {
  downloadingID.value = record.id
  try {
    const response = await getPipelineRawScript(record.id)
    const script = response.data.script || ''
    const pipelineName = record.job_name || record.job_full_name || record.id
    const extension = response.data.definition_class === 'org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition' ? '.groovy' : ''
    const filename = `${pipelineName}${extension}`
    const blob = new Blob([script], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
    message.success('管线脚本下载成功')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '下载管线脚本失败'))
  } finally {
    downloadingID.value = ''
  }
}

onMounted(() => {
  syncEditorModalViewportInset()
  observeEditorModalViewportInset()
  void loadPipelines()
})

onUnmounted(() => {
  clearSearchSuggestions()
  stopObservingEditorModalViewportInset()
})

watch(
  () => searchDialogVisible.value,
  (visible) => {
    if (!visible) {
      clearSearchSuggestions()
      return
    }
    const keyword = searchDraft.keyword.trim()
    if (keyword) {
      void fetchSearchSuggestions(keyword)
    }
  },
)

watch(
  () => searchDraft.keyword.trim(),
  (keyword) => {
    if (!searchDialogVisible.value) {
      return
    }
    if (searchSuggestionTimer) {
      window.clearTimeout(searchSuggestionTimer)
      searchSuggestionTimer = null
    }
    searchSuggestionTimer = window.setTimeout(() => {
      void fetchSearchSuggestions(keyword)
    }, 220)
  },
)
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header">
      <div class="page-header-copy">
        <h2 class="page-title">管线</h2>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-select
          v-model:value="statusFilterValue"
          class="jenkins-toolbar-select"
          :options="[
            { label: '状态 · 全部', value: '' },
            { label: '状态 · active', value: 'active' },
            { label: '状态 · inactive', value: 'inactive' },
          ]"
        />
        <a-button class="jenkins-toolbar-query-btn" @click="handleSearch">查询</a-button>
        <a-button v-if="canManagePipeline" class="application-toolbar-action-btn" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增管线
        </a-button>
        <a-button v-if="canManagePipeline" class="application-toolbar-action-btn" :loading="scanningAll" @click="handleScanAllPipelines">
          <template #icon>
            <FileSearchOutlined />
          </template>
          扫描
        </a-button>
        <a-button v-if="canSyncJenkins" class="application-toolbar-action-btn" :loading="syncing" @click="handleManualSync">
          <template #icon>
            <ReloadOutlined />
          </template>
          手动同步
        </a-button>
      </div>
    </div>

    <transition name="jenkins-search-fade">
      <div v-if="searchDialogVisible" class="jenkins-search-overlay" @click.self="closeSearchDialog">
        <div class="jenkins-search-floating-panel">
          <div class="jenkins-search-floating-input">
            <SearchOutlined class="jenkins-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="jenkins-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="管线名称 / Jenkins 路径"
              @keydown.enter="handleSearchSubmit"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="jenkins-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="jenkins-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="jenkins-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="jenkins-search-suggestion-title">{{ item.title }}</span>
                <span class="jenkins-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-card class="table-card" :bordered="true">
      <a-table
        class="jenkins-table"
        row-key="id"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        :locale="tableLocale"
        :scroll="{ x: 1560 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'job_name'">
            <button
              type="button"
              class="jenkins-pipeline-name-link"
              :title="`打开 Jenkins 原始链接：${record.job_full_name || getPipelineName(record)}`"
              @click="openOriginalLink(record)"
            >
              {{ getPipelineName(record) }}
            </button>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'scan_status'">
            <a-popover
              trigger="click"
              placement="rightTop"
              overlay-class-name="jenkins-scan-popover"
              :open="scanPopoverOpenID === record.id"
              @open-change="(open: boolean) => handleScanPopoverOpenChange(record.id, open)"
            >
              <template #content>
                <div class="jenkins-scan-popover-panel">
                  <div class="jenkins-scan-popover-head">
                    <div>
                      <div class="jenkins-scan-popover-title">{{ getPipelineName(record) }}</div>
                      <div class="jenkins-scan-popover-subtitle">{{ record.job_full_name || '-' }}</div>
                    </div>
                  </div>
                  <div class="jenkins-scan-popover-meta">
                    <span>最近扫描</span>
                    <strong>{{ formatTime(pipelineScanResult(record)?.last_scanned_at || null) }}</strong>
                  </div>
                  <div v-if="pipelineScanResult(record)" class="jenkins-scan-count-grid">
                    <span class="is-error">E{{ pipelineScanResult(record)?.error_count || 0 }}</span>
                    <span class="is-warning">W{{ pipelineScanResult(record)?.warning_count || 0 }}</span>
                    <span class="is-info">I{{ pipelineScanResult(record)?.info_count || 0 }}</span>
                  </div>
                  <div v-if="pipelineScanFindings(record).length" class="jenkins-scan-finding-list">
                    <div
                      v-for="finding in pipelineScanFindings(record).slice(0, 4)"
                      :key="finding.id"
                      class="jenkins-scan-finding-item"
                    >
                      <div class="jenkins-scan-finding-head">
                        <a-tag :color="pipelineScanSeverityColor(finding.severity)">{{ finding.severity }}</a-tag>
                        <span>{{ finding.rule_name || finding.rule_code }}</span>
                      </div>
                      <div class="jenkins-scan-finding-message">
                        <span v-if="finding.line_no > 0">第 {{ finding.line_no }} 行 · </span>{{ finding.message }}
                      </div>
                      <div v-if="finding.suggestion" class="jenkins-scan-finding-suggestion">{{ finding.suggestion }}</div>
                    </div>
                    <div v-if="pipelineScanFindings(record).length > 4" class="jenkins-scan-finding-more">
                      还有 {{ pipelineScanFindings(record).length - 4 }} 条问题，请到管线规范页查看
                    </div>
                  </div>
                  <div v-else class="jenkins-scan-empty-detail">{{ pipelineScanEmptyDetail(record) }}</div>
                  <div class="jenkins-scan-popover-actions">
                    <a-button
                      class="jenkins-scan-popover-action-btn"
                      size="small"
                      :disabled="record.status !== 'active'"
                      :loading="scanningID === record.id"
                      @click.stop="handleScanPipeline(record)"
                    >
                      重新扫描
                    </a-button>
                    <a-button
                      class="jenkins-scan-popover-action-btn"
                      size="small"
                      :disabled="pipelineScanFindings(record).length === 0"
                      @click.stop="openScriptLocation(record)"
                    >
                      查看定位
                    </a-button>
                  </div>
                </div>
              </template>
              <button type="button" class="jenkins-scan-status-trigger">
                <span class="jenkins-scan-status-cell">
                  <span class="jenkins-scan-summary" :class="pipelineScanSummaryClass(record)">
                    {{ pipelineScanSummary(record) }}
                  </span>
                </span>
              </button>
            </a-popover>
          </template>
          <template v-else-if="column.key === 'last_synced_at'">
            {{ formatTime(record.last_synced_at) }}
          </template>
          <template v-else-if="column.key === 'last_verified_at'">
            {{ formatTime(record.last_verified_at) }}
          </template>
          <template v-else-if="column.key === 'updated_at'">
            {{ formatTime(record.updated_at) }}
          </template>
          <template v-else-if="column.key === 'actions'">
            <div class="jenkins-row-actions">
              <a-button class="jenkins-row-action-btn jenkins-row-action-btn-script" size="small" @click="openScriptModal(record)">
                <template #icon>
                  <FileTextOutlined />
                </template>
                原始脚本
              </a-button>
              <a-button
                v-if="canManagePipeline"
                class="jenkins-row-action-btn"
                size="small"
                :disabled="record.status !== 'active'"
                :loading="scanningID === record.id"
                @click="handleScanPipeline(record)"
              >
                <template #icon>
                  <FileSearchOutlined />
                </template>
                扫描
              </a-button>
              <a-button
                v-if="canManagePipeline"
                class="jenkins-row-action-btn"
                size="small"
                :disabled="record.status !== 'active' || editorLoading"
                @click="openEditModal(record)"
              >
                <template #icon>
                  <EditOutlined />
                </template>
                编辑
              </a-button>
              <a-popover
                v-if="canManagePipeline"
                trigger="click"
                placement="bottomRight"
                overlay-class-name="jenkins-danger-popover"
              >
                <template #content>
                  <div class="jenkins-hidden-danger-panel">
                    <a-button
                      class="jenkins-hidden-download-btn"
                      size="small"
                      :loading="downloadingID === record.id"
                      @click.stop="handleDownloadPipeline(record)"
                    >
                      <template #icon>
                        <DownloadOutlined />
                      </template>
                      下载脚本
                    </a-button>
                    <div class="jenkins-hidden-danger-title">危险操作</div>
                    <div class="jenkins-hidden-danger-copy">删除会同步回平台并置为失效状态，请确认当前管线不再使用</div>
                    <a-popconfirm
                      title="确认删除当前原始管线吗？删除后会同步回平台并置为失效状态"
                      ok-text="删除"
                      cancel-text="取消"
                      @confirm="handleDelete(record)"
                    >
                      <a-button
                        class="jenkins-hidden-delete-btn"
                        size="small"
                        danger
                        :disabled="record.status !== 'active'"
                        :loading="deletingID === record.id"
                      >
                        <template #icon>
                          <DeleteOutlined />
                        </template>
                        删除管线
                      </a-button>
                    </a-popconfirm>
                  </div>
                </template>
                <a-button class="jenkins-row-action-btn jenkins-row-more-btn" size="small">
                  <template #icon>
                    <MoreOutlined />
                  </template>
                  更多
                </a-button>
              </a-popover>
            </div>
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
    </a-card>

    <a-modal
      :open="scriptVisible"
      :footer="null"
      :width="1180"
      :closable="false"
      :destroy-on-close="true"
      :mask-style="scriptModalMaskStyle"
      :wrap-props="scriptModalWrapProps"
      wrap-class-name="jenkins-script-modal-wrap"
      @cancel="closeScriptModal"
    >
      <template #title>
        <div class="script-modal-titlebar">
          <span class="script-modal-title">管线原始脚本</span>
          <a-button class="application-toolbar-action-btn script-modal-close-btn" @click="closeScriptModal">
            关闭
          </a-button>
        </div>
      </template>
      <div class="script-modal-note">
        当前管线：{{ scriptPipelineName || '-' }}
      </div>
      <a-skeleton v-if="scriptLoading" active :paragraph="{ rows: 8 }" />
      <template v-else-if="scriptData">
        <div
          v-if="scriptData.from_scm"
          class="jenkins-inline-note"
        >
          该管线为 SCM 脚本模式，Jenkins 仅记录脚本路径，完整内容请查看代码仓库
        </div>
        <div class="script-review-layout" :class="{ 'has-issues': scriptLocationFindings.length > 0 }">
          <div class="script-panel script-panel-lined">
            <div
              v-for="line in displayScriptLines"
              :key="line.lineNo"
              class="script-line"
              :class="scriptLineClass(line.lineNo)"
              :data-script-line="line.lineNo"
            >
              <span class="script-line-number">{{ line.lineNo }}</span>
              <code class="script-line-code">{{ line.text || ' ' }}</code>
            </div>
          </div>
          <aside v-if="scriptLocationFindings.length" class="script-issue-panel">
            <div class="script-issue-list">
              <div
                v-for="finding in scriptLocationFindings"
                :key="finding.id"
                role="button"
                tabindex="0"
                class="script-issue-item"
                :class="scriptFindingItemClass(finding)"
                @click="selectScriptFinding(finding)"
                @keydown.enter.prevent="selectScriptFinding(finding)"
              >
                <span class="script-issue-item-top">
                  <span class="script-issue-badge" :class="scriptFindingBadgeClass(finding.severity)">
                    {{ scriptFindingBadgeLabel(finding) }}
                  </span>
                  <span class="script-issue-line">{{ scriptFindingRangeLabel(finding) }}</span>
                </span>
                <span class="script-issue-title">{{ finding.rule_name || finding.rule_code }}</span>
                <span v-if="selectedScriptFinding?.id === finding.id" class="script-issue-detail">
                  <span class="script-issue-detail-block">
                    <strong>提示</strong>
                    <span>{{ finding.message || '-' }}</span>
                  </span>
                  <span class="script-issue-detail-block">
                    <strong>建议</strong>
                    <span>{{ finding.suggestion || '-' }}</span>
                  </span>
                  <span class="script-issue-detail-actions">
                    <template v-if="scriptPendingFindingID === finding.id">
                      <a-button size="small" :disabled="submittingOverwrite" @click.stop="withdrawScriptOverwrite">
                        撤回
                      </a-button>
                      <a-button
                        size="small"
                        type="primary"
                        :disabled="!canManagePipeline"
                        :loading="submittingOverwrite"
                        @click.stop="submitScriptOverwrite(finding)"
                      >
                        提交
                      </a-button>
                    </template>
                    <a-button
                      v-else
                      size="small"
                      type="primary"
                      :disabled="!canManagePipeline || !!scriptPendingFindingID"
                      :loading="overwritingFindingID === finding.id"
                      @click.stop="previewScriptOverwrite(finding)"
                    >
                      一键覆盖
                    </a-button>
                  </span>
                </span>
              </div>
            </div>
          </aside>
        </div>
      </template>
    </a-modal>

    <a-modal
      :open="configVisible"
      :footer="null"
      :width="920"
      :closable="false"
      :destroy-on-close="true"
      :mask-style="configModalMaskStyle"
      :wrap-props="configModalWrapProps"
      wrap-class-name="jenkins-config-modal-wrap"
      @cancel="closeConfigModal"
    >
      <template #title>
        <div class="config-modal-titlebar">
          <span class="config-modal-title">配置 XML</span>
          <a-button class="application-toolbar-action-btn config-modal-close-btn" @click="closeConfigModal">
            关闭
          </a-button>
        </div>
      </template>
      <div class="config-modal-note">
        当前管线：{{ configTitle || '-' }}
      </div>
      <a-skeleton v-if="configLoading" active :paragraph="{ rows: 10 }" />
      <template v-else>
        <pre class="script-panel">{{ configXML || '未获取到配置XML' }}</pre>
      </template>
    </a-modal>

    <a-modal
      :open="editorVisible"
      :width="860"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="editorModalMaskStyle"
      :wrap-props="editorModalWrapProps"
      wrap-class-name="jenkins-editor-modal-wrap"
      @cancel="closeEditorModal"
    >
      <template #title>
        <div class="jenkins-editor-modal-titlebar">
          <span class="jenkins-editor-modal-title">{{ editorMode === 'create' ? '新增原始管线' : '编辑原始管线' }}</span>
          <div class="jenkins-editor-modal-actions">
            <a-button class="application-toolbar-action-btn jenkins-editor-modal-action-btn" :loading="previewingConfig" @click="previewConfigFromForm">
              预览配置XML
            </a-button>
            <a-button class="application-toolbar-action-btn jenkins-editor-modal-save-btn" :loading="submitting" @click="submitEditor">
              保存
            </a-button>
          </div>
        </div>
      </template>
      <div class="jenkins-editor-note">
        仅支持 Jenkins inline raw pipeline；Jenkins 路径支持根目录或已有 folder/子路径，不会自动创建文件夹
      </div>
      <a-skeleton v-if="editorLoading" active :paragraph="{ rows: 8 }" />
      <a-form
        v-else
        ref="formRef"
        layout="vertical"
        :model="editorForm"
        :rules="formRules"
        :required-mark="false"
        class="jenkins-editor-form"
      >
        <a-form-item label="Jenkins 路径" name="full_name">
          <a-input
            v-model:value="editorForm.full_name"
            :disabled="editorMode === 'edit'"
            placeholder="例如 folder-a/demo-pipeline 或 demo-pipeline"
          />
        </a-form-item>
        <a-form-item label="描述">
          <a-input
            v-model:value="editorForm.description"
            allow-clear
            placeholder="可选，填写这条管线的用途说明"
          />
        </a-form-item>
        <a-form-item label="Sandbox">
          <a-switch v-model:checked="editorForm.sandbox" />
        </a-form-item>
        <a-form-item label="原始脚本" name="script">
          <a-textarea
            v-model:value="editorForm.script"
            :auto-size="{ minRows: 14, maxRows: 24 }"
            placeholder="请输入 Jenkins Pipeline Script"
          />
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
}

.table-card {
  overflow: hidden;
  border-radius: 0;
  border: none;
  background: transparent;
  box-shadow: none;
}

:deep(.table-card .ant-card-body) {
  padding: 0;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

.jenkins-table :deep(.ant-table),
.jenkins-table :deep(.ant-table-content),
.jenkins-table :deep(.ant-table-body) {
  border-radius: 0 !important;
  background: transparent;
}

.jenkins-table :deep(.ant-table-container) {
  overflow: hidden;
  border-radius: 0 !important;
  border: 1px solid rgba(226, 232, 240, 0.92);
}

.jenkins-table :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, #243247, #1f2a3d);
  color: rgba(239, 246, 255, 0.96);
  border-bottom: none;
  border-radius: 0 !important;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.jenkins-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.76);
  border-radius: 0 !important;
  background: rgba(255, 255, 255, 0.64);
  transition: background 0.18s ease;
}

.jenkins-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.jenkins-table :deep(.ant-table-cell-fix-right) {
  background: #fff !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.05);
}

.jenkins-table :deep(.ant-table-thead > tr > th.ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
}

.jenkins-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right) {
  background: #f8fafc !important;
}

.jenkins-pipeline-name-link {
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: #0f172a;
  font: inherit;
  font-weight: 700;
  line-height: 1.5;
  text-align: left;
  cursor: pointer;
  transition: color 0.18s ease;
}

.jenkins-pipeline-name-link:hover,
.jenkins-pipeline-name-link:focus-visible {
  color: #2563eb;
  outline: none;
}

.jenkins-row-actions {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 8px;
  min-width: 326px;
}

.jenkins-scan-status-trigger {
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.jenkins-scan-status-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 150px;
}

.jenkins-scan-summary {
  color: #64748b;
  font-size: 12px;
  line-height: 1.3;
  white-space: nowrap;
}

.jenkins-scan-summary.is-clean {
  color: #15803d;
}

.jenkins-scan-summary.is-problem {
  color: #b45309;
  font-weight: 700;
}

.jenkins-scan-summary.is-empty,
.jenkins-scan-summary.is-unknown {
  color: #64748b;
}

:deep(.jenkins-scan-popover .ant-popover-inner) {
  width: 420px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 20px;
  background:
    radial-gradient(circle at top right, rgba(134, 239, 172, 0.16), transparent 34%),
    radial-gradient(circle at left bottom, rgba(96, 165, 250, 0.14), transparent 40%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(248, 250, 252, 0.92));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 24px 70px rgba(15, 23, 42, 0.16);
  backdrop-filter: blur(18px) saturate(180%);
}

:deep(.jenkins-scan-popover .ant-popover-arrow) {
  display: none;
}

:deep(.jenkins-scan-popover .ant-popover-inner-content) {
  padding: 18px;
}

.jenkins-scan-popover-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.jenkins-scan-popover-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.jenkins-scan-popover-title {
  color: #0f172a;
  font-size: 15px;
  font-weight: 800;
  line-height: 1.4;
}

.jenkins-scan-popover-subtitle {
  max-width: 340px;
  margin-top: 2px;
  overflow: hidden;
  color: #64748b;
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jenkins-scan-popover-meta {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-left: 14px;
  color: #64748b;
  font-size: 12px;
}

.jenkins-scan-popover-meta::before {
  content: '';
  position: absolute;
  left: 0;
  top: 2px;
  bottom: 2px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.jenkins-scan-popover-meta strong {
  color: #334155;
  font-weight: 700;
}

.jenkins-scan-count-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.jenkins-scan-count-grid span {
  padding: 6px 8px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 800;
  text-align: center;
}

.jenkins-scan-count-grid .is-error {
  border: 1px solid rgba(254, 202, 202, 0.72);
  background: rgba(254, 242, 242, 0.86);
  color: #b91c1c;
}

.jenkins-scan-count-grid .is-warning {
  border: 1px solid rgba(253, 230, 138, 0.72);
  background: rgba(255, 251, 235, 0.88);
  color: #b45309;
}

.jenkins-scan-count-grid .is-info {
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.88);
  color: #1d4ed8;
}

.jenkins-scan-finding-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 280px;
  overflow: auto;
}

.jenkins-scan-finding-item {
  padding: 10px 11px;
  border: 1px solid rgba(226, 232, 240, 0.82);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.58);
  box-shadow: 0 10px 22px rgba(15, 23, 42, 0.04);
}

.jenkins-scan-finding-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #0f172a;
  font-size: 12px;
  font-weight: 800;
}

.jenkins-scan-finding-message,
.jenkins-scan-finding-suggestion,
.jenkins-scan-finding-more,
.jenkins-scan-empty-detail {
  margin-top: 6px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.55;
}

.jenkins-scan-finding-suggestion {
  color: #475569;
}

.jenkins-scan-finding-more {
  margin-top: 0;
  text-align: center;
}

.jenkins-scan-empty-detail {
  margin-top: 0;
  position: relative;
  padding: 0 0 0 14px;
  border-radius: 0;
  background: transparent;
}

.jenkins-scan-empty-detail::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(34, 197, 94, 0.42), rgba(134, 239, 172, 0.16));
}

.jenkins-scan-popover-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 4px;
}

:deep(.jenkins-scan-popover-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 34px;
  padding-inline: 12px;
  border-radius: 14px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  font-size: 12px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 8px 18px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.jenkins-scan-popover-action-btn.ant-btn:hover),
:deep(.jenkins-scan-popover-action-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.jenkins-scan-popover-action-btn.ant-btn[disabled]),
:deep(.jenkins-scan-popover-action-btn.ant-btn[disabled]:hover) {
  border-color: rgba(226, 232, 240, 0.62) !important;
  background: rgba(248, 250, 252, 0.42) !important;
  color: rgba(100, 116, 139, 0.46) !important;
  box-shadow: none !important;
}

:deep(.jenkins-row-action-btn.ant-btn) {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 28px;
  padding-inline: 10px;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.82) !important;
  background: rgba(255, 255, 255, 0.72) !important;
  color: #334155 !important;
  font-size: 12px;
  font-weight: 700;
  box-shadow: 0 6px 14px rgba(15, 23, 42, 0.04);
}

:deep(.jenkins-row-action-btn.ant-btn:hover),
:deep(.jenkins-row-action-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.46) !important;
  background: rgba(239, 246, 255, 0.92) !important;
  color: #1d4ed8 !important;
}

.jenkins-row-more-btn {
  min-width: 58px;
}

:deep(.jenkins-row-action-btn.ant-btn[disabled]),
:deep(.jenkins-row-action-btn.ant-btn[disabled]:hover) {
  border-color: rgba(226, 232, 240, 0.82) !important;
  background: rgba(248, 250, 252, 0.62) !important;
  color: rgba(100, 116, 139, 0.44) !important;
  box-shadow: none;
}

:deep(.jenkins-danger-popover .ant-popover-inner) {
  border-radius: 16px;
  border: 1px solid rgba(248, 113, 113, 0.24);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(254, 242, 242, 0.96)),
    #fff;
  box-shadow: 0 18px 38px rgba(127, 29, 29, 0.12);
}

:deep(.jenkins-danger-popover .ant-popover-inner-content) {
  padding: 12px;
}

.jenkins-hidden-danger-panel {
  width: 220px;
}

.jenkins-hidden-danger-title {
  color: #991b1b;
  font-size: 13px;
  font-weight: 800;
  line-height: 1.4;
}

.jenkins-hidden-danger-copy {
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

:deep(.jenkins-hidden-download-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  width: 100%;
  height: 30px;
  margin-bottom: 12px;
  border-radius: 12px;
  font-weight: 700;
}

:deep(.jenkins-hidden-delete-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  width: 100%;
  height: 30px;
  margin-top: 10px;
  border-radius: 12px;
  font-weight: 700;
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.application-toolbar-icon-btn.ant-btn),
:deep(.jenkins-toolbar-query-btn.ant-btn) {
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

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.jenkins-toolbar-query-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  min-width: 42px;
  padding-inline: 0;
}

:deep(.jenkins-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.jenkins-toolbar-select.ant-select .ant-select-selector) {
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

:deep(.jenkins-toolbar-select.ant-select .ant-select-selection-item),
:deep(.jenkins-toolbar-select.ant-select .ant-select-selection-placeholder),
:deep(.jenkins-toolbar-select.ant-select .ant-select-arrow) {
  color: #0f172a !important;
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.jenkins-toolbar-query-btn.ant-btn:hover),
:deep(.jenkins-toolbar-query-btn.ant-btn:focus),
:deep(.jenkins-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.jenkins-inline-note {
  margin-bottom: 12px;
  padding-left: 12px;
  border-left: 3px solid #3b82f6;
  color: #475569;
  font-size: 13px;
  line-height: 1.7;
}

.jenkins-script-modal-wrap :deep(.ant-modal),
.jenkins-config-modal-wrap :deep(.ant-modal) {
  padding-bottom: 32px;
}

.jenkins-script-modal-wrap :deep(.ant-modal-content),
.jenkins-config-modal-wrap :deep(.ant-modal-content) {
  position: relative;
  isolation: isolate;
  overflow: hidden;
  border-radius: 24px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  background:
    radial-gradient(circle at top right, rgba(134, 239, 172, 0.18), transparent 34%),
    radial-gradient(circle at left bottom, rgba(96, 165, 250, 0.16), transparent 40%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(248, 250, 252, 0.92));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 32px 90px rgba(15, 23, 42, 0.18);
  backdrop-filter: blur(18px) saturate(180%);
}

.jenkins-script-modal-wrap :deep(.ant-modal-content)::before,
.jenkins-config-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.34), transparent 36%);
}

.jenkins-script-modal-wrap :deep(.ant-modal-header),
.jenkins-config-modal-wrap :deep(.ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

.jenkins-script-modal-wrap :deep(.ant-modal-title),
.jenkins-config-modal-wrap :deep(.ant-modal-title) {
  width: 100%;
}

.jenkins-script-modal-wrap :deep(.ant-modal-body),
.jenkins-config-modal-wrap :deep(.ant-modal-body) {
  padding: 20px 28px 28px;
}

.script-modal-note,
.config-modal-note {
  position: relative;
  margin-bottom: 18px;
  padding-left: 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.script-modal-note::before,
.config-modal-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.script-panel {
  margin: 0;
  min-height: 520px;
  max-height: 620px;
  overflow: auto;
  padding: 12px 0;
  border: 1px solid rgba(15, 23, 42, 0.18);
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(15, 23, 42, 0.98), rgba(17, 24, 39, 0.98)),
    #111827;
  color: #f8fafc;
  font-size: 12px;
  line-height: 1.6;
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
  white-space: pre-wrap;
  word-break: break-word;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.06),
    0 18px 42px rgba(15, 23, 42, 0.14);
}

.script-modal-titlebar,
.config-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  width: 100%;
}

.script-modal-title,
.config-modal-title {
  min-width: 0;
  overflow: hidden;
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-modal-close-btn.ant-btn,
.config-modal-close-btn.ant-btn {
  flex: none;
  font-size: 14px;
}

.script-review-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
}

.script-review-layout.has-issues {
  grid-template-columns: minmax(0, 1fr) 344px;
}

.script-panel-lined {
  white-space: normal;
}

.script-line {
  position: relative;
  display: grid;
  grid-template-columns: 52px minmax(0, 1fr);
  column-gap: 10px;
  min-height: 24px;
  padding: 2px 14px;
  border-bottom: none;
}

.script-line.is-annotated.is-error {
  background: rgba(239, 68, 68, 0.18);
}

.script-line.is-annotated.is-warning {
  background: rgba(251, 191, 36, 0.14);
}

.script-line.is-annotated.is-info {
  background: rgba(59, 130, 246, 0.16);
}

.script-line.is-selected-range {
  box-shadow: inset 0 0 0 9999px rgba(255, 255, 255, 0.035);
}

.script-line-number {
  color: rgba(148, 163, 184, 0.86);
  text-align: right;
  user-select: none;
}

.script-line-code {
  min-width: 0;
  color: #f8fafc;
  font: inherit;
  white-space: pre-wrap;
  word-break: break-word;
}

.script-issue-panel {
  display: flex;
  flex-direction: column;
  min-height: 520px;
  max-height: 620px;
  overflow: auto;
  background: transparent;
}

.script-issue-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow: visible;
  padding: 0 2px 2px 0;
}

.script-issue-item {
  display: flex;
  flex-direction: column;
  gap: 7px;
  width: 100%;
  padding: 12px;
  border: 1px solid rgba(226, 232, 240, 0.82);
  border-left: 4px solid transparent;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.62);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 12px 26px rgba(15, 23, 42, 0.06);
  color: #0f172a;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background 0.16s ease,
    border-color 0.16s ease,
    box-shadow 0.16s ease;
}

.script-issue-item:hover,
.script-issue-item:focus-visible {
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.1);
  outline: none;
}

.script-issue-item.is-error {
  border-left-color: #ef4444;
}

.script-issue-item.is-warning {
  border-left-color: #f59e0b;
}

.script-issue-item.is-info {
  border-left-color: #3b82f6;
}

.script-issue-item.is-selected {
  border-color: rgba(15, 23, 42, 0.18);
  background: rgba(255, 255, 255, 0.9);
  box-shadow:
    0 0 0 2px rgba(15, 23, 42, 0.06),
    0 14px 30px rgba(15, 23, 42, 0.1);
}

.script-issue-item-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.script-issue-badge {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  color: #fff;
  font-size: 12px;
  font-weight: 800;
}

.script-issue-badge.is-error {
  background: #b91c1c;
}

.script-issue-badge.is-warning {
  background: #b45309;
}

.script-issue-badge.is-info {
  background: #1d4ed8;
}

.script-issue-line {
  color: #64748b;
  font-size: 12px;
  white-space: nowrap;
}

.script-issue-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
  line-height: 1.5;
}

.script-issue-detail {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 2px;
  padding-top: 8px;
  border-top: 1px solid rgba(203, 213, 225, 0.78);
}

.script-issue-detail-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #334155;
  font-size: 12px;
  line-height: 1.55;
}

.script-issue-detail-block strong {
  color: #64748b;
  font-weight: 800;
}

.script-issue-detail-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 2px;
}

.script-issue-detail-actions :deep(.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 30px;
  padding-inline: 12px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.46) !important;
  color: #0f172a !important;
  font-size: 12px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 8px 18px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

.script-issue-detail-actions :deep(.ant-btn:hover),
.script-issue-detail-actions :deep(.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.6) !important;
  color: #0f172a !important;
}

.script-issue-detail-actions :deep(.ant-btn[disabled]),
.script-issue-detail-actions :deep(.ant-btn[disabled]:hover) {
  border-color: rgba(226, 232, 240, 0.62) !important;
  background: rgba(248, 250, 252, 0.42) !important;
  color: rgba(100, 116, 139, 0.46) !important;
  box-shadow: none !important;
}

.jenkins-editor-modal-wrap :deep(.ant-modal) {
  padding-bottom: 32px;
}

.jenkins-editor-modal-wrap :deep(.ant-modal-content) {
  overflow: hidden;
  border-radius: 24px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  background:
    radial-gradient(circle at top right, rgba(134, 239, 172, 0.18), transparent 34%),
    radial-gradient(circle at left bottom, rgba(96, 165, 250, 0.16), transparent 40%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(248, 250, 252, 0.92));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 32px 90px rgba(15, 23, 42, 0.18);
  backdrop-filter: blur(18px) saturate(180%);
}

.jenkins-editor-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.34), transparent 36%);
}

.jenkins-editor-modal-wrap :deep(.ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

.jenkins-editor-modal-wrap :deep(.ant-modal-title) {
  width: 100%;
}

.jenkins-editor-modal-wrap :deep(.ant-modal-body) {
  padding: 20px 28px 28px;
}

.jenkins-editor-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.jenkins-editor-modal-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
  line-height: 1.4;
}

.jenkins-editor-modal-actions {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.jenkins-editor-modal-action-btn.ant-btn,
.jenkins-editor-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
}

.jenkins-editor-note {
  margin-bottom: 18px;
  padding-left: 12px;
  border-left: 3px solid #3b82f6;
  color: #475569;
  font-size: 13px;
  line-height: 1.7;
}

.jenkins-editor-form :deep(.ant-form-item-label > label) {
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

.jenkins-editor-form :deep(.ant-input),
.jenkins-editor-form :deep(.ant-input-affix-wrapper),
.jenkins-editor-form :deep(.ant-input-textarea textarea) {
  border-color: rgba(203, 213, 225, 0.78);
  background: rgba(255, 255, 255, 0.5);
}

.jenkins-search-overlay {
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

.jenkins-search-floating-panel {
  width: min(100%, 480px);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.jenkins-search-floating-input {
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

.jenkins-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.jenkins-search-floating-field {
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

.jenkins-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.jenkins-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.66)),
    rgba(255, 255, 255, 0.5);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 18px 36px rgba(15, 23, 42, 0.1);
}

.jenkins-search-suggestions {
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

.jenkins-search-suggestion,
.jenkins-search-suggestion-loading {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.34);
}

.jenkins-search-suggestion {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  padding: 10px 12px;
  border: none;
  color: #0f172a;
  text-align: left;
  cursor: pointer;
  transition: background 0.18s ease, transform 0.18s ease;
}

.jenkins-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
  transform: translateY(-1px);
}

.jenkins-search-suggestion-loading {
  padding: 12px 14px;
  color: rgba(51, 65, 85, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.jenkins-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.jenkins-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.78);
  font-size: 12px;
  font-weight: 600;
}

.jenkins-search-fade-enter-active,
.jenkins-search-fade-leave-active {
  transition: opacity 0.18s ease;
}

.jenkins-search-fade-enter-from,
.jenkins-search-fade-leave-to {
  opacity: 0;
}

@media (max-width: 1024px) {
  .page-header {
    flex-wrap: wrap;
  }
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  :deep(.jenkins-toolbar-select.ant-select) {
    min-width: min(100%, 180px);
  }

  .jenkins-editor-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }

  .jenkins-editor-modal-actions {
    flex-wrap: wrap;
  }

  .jenkins-search-overlay {
    left: 0;
    padding-inline: 16px;
  }
}
</style>
