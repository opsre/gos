<script setup lang="ts">
import { DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import {
  createPipelineScanRule,
  deletePipelineScanRule,
  listPipelineScanRules,
  setPipelineScanRuleEnabled,
  updatePipelineScanRule,
} from '../../api/pipeline-scan'
import type {
  PipelineScanCategory,
  PipelineScanRule,
  PipelineScanTemplateValidationScope,
  PipelineScanRuleType,
  PipelineScanSeverity,
} from '../../types/pipeline-scan'
import { useResizableColumns } from '../../composables/useResizableColumns'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const loadingRules = ref(false)
const submitting = ref(false)
const editorVisible = ref(false)
const viewerVisible = ref(false)
const searchDialogVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const formRef = ref<FormInstance>()
const searchInputRef = ref<HTMLInputElement>()
const rules = ref<PipelineScanRule[]>([])
const viewingRule = ref<PipelineScanRule | null>(null)
const togglingRuleIDs = ref<Set<string>>(new Set())
const ruleTotal = ref(0)
const ruleFormViewportInset = ref(0)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
let ruleFormViewportObserver: ResizeObserver | null = null
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

const filters = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
})

const searchDraft = reactive({
  keyword: '',
})

const form = reactive({
  id: '',
  rule_code: '',
  rule_domain: 'artifact',
  rule_target: 'oss',
  rule_check: 'command_format',
  rule_profile: 'standard',
  rule_name: '',
  severity: 'warning' as PipelineScanSeverity,
  enabled: true,
  template_validation_scopes: [] as PipelineScanTemplateValidationScope[],
  scope_json: '{}',
  rule_dsl_json: '',
  command_format_text: '',
  command_format_required: false,
  pipeline_parameter_input: '',
  pipeline_parameters: [] as string[],
  message: '',
  suggestion: '',
})

const categoryOptions = [
  { label: '制品', value: 'artifact' },
  { label: '安全', value: 'security' },
  { label: '凭据', value: 'credential' },
  { label: '命名', value: 'naming' },
  { label: '自定义', value: 'custom' },
]

const severityOptions = [
  { label: '提示', value: 'info' },
  { label: '警告', value: 'warning' },
  { label: '错误', value: 'error' },
]

const ruleDomainOptions = [{ label: '制品', value: 'artifact' }]
const ruleTargetOptions = [
  { label: '对象存储', value: 'oss' },
  { label: 'GOS', value: 'gos' },
]
const ruleCheckOptions = [
  { label: '完整命令排版', value: 'command_format', targets: ['oss'] },
  { label: '管线参数', value: 'pipeline_params', targets: ['oss'] },
  { label: '制品地址输出', value: 'artifact_url', targets: ['gos'] },
]
const ruleProfileOptions = [{ label: '标准', value: 'standard' }]
const templateValidationScopeOptions = [
  { label: 'CI', value: 'ci' },
  { label: 'CD', value: 'cd' },
] satisfies Array<{ label: string; value: PipelineScanTemplateValidationScope }>

const availableRuleCheckOptions = computed(() => {
  return ruleCheckOptions
    .filter((item) => item.targets.includes(form.rule_target))
    .map(({ targets: _targets, ...item }) => item)
})

const generatedRuleType = computed(() => {
  return [
    form.rule_domain,
    form.rule_target,
    form.rule_check,
    form.rule_profile,
  ].join('_') as PipelineScanRuleType
})

const generatedRuleCode = computed(() => {
  return [
    form.rule_domain,
    form.rule_target,
    form.rule_check,
    form.rule_profile,
  ].join('.')
})

const displayedRuleCode = computed(() => {
  if (editorMode.value === 'edit' && form.rule_code) {
    return form.rule_code
  }
  return generatedRuleCode.value
})

const initialRuleColumns: TableColumnsType<PipelineScanRule> = [
  { title: '规则名称', dataIndex: 'rule_name', key: 'rule_name', width: 220 },
  { title: '规则编码', dataIndex: 'rule_code', key: 'rule_code', width: 260 },
  { title: '类型', dataIndex: 'builtin', key: 'builtin', width: 90 },
  { title: '分类', dataIndex: 'category', key: 'category', width: 110 },
  { title: '级别', dataIndex: 'severity', key: 'severity', width: 100 },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
  { title: '操作', key: 'actions', width: 300, fixed: 'right' },
]

const { columns: ruleColumns } = useResizableColumns(initialRuleColumns, { minWidth: 90, maxWidth: 560, hitArea: 10 })

const formRules = {
  rule_domain: [{ required: true, message: '请选择规则域', trigger: 'change' }],
  rule_target: [{ required: true, message: '请选择规则对象', trigger: 'change' }],
  rule_check: [{ required: true, message: '请选择检查项', trigger: 'change' }],
  rule_profile: [{ required: true, message: '请选择规则模板', trigger: 'change' }],
  rule_name: [{ required: true, message: '请输入规则名称', trigger: 'blur' }],
  command_format_text: [{ required: true, message: '请输入完整命令', trigger: 'blur' }],
}

const tableLocale = computed(() => ({
  emptyText: filters.keyword.trim() ? '未找到匹配的规则' : '暂无管线规则',
}))

const ruleFormMaskStyle = computed(() => ({
  left: `${ruleFormViewportInset.value}px`,
  width: `calc(100% - ${ruleFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: editorVisible.value || viewerVisible.value ? 'auto' : 'none',
}))

const ruleFormWrapProps = computed(() => ({
  style: {
    left: `${ruleFormViewportInset.value}px`,
    width: `calc(100% - ${ruleFormViewportInset.value}px)`,
    pointerEvents: editorVisible.value || viewerVisible.value ? 'auto' : 'none',
  },
}))

function resetForm() {
  form.id = ''
  form.rule_code = ''
  form.rule_domain = 'artifact'
  form.rule_target = 'oss'
  form.rule_check = 'command_format'
  form.rule_profile = 'standard'
  form.rule_name = ''
  form.severity = 'warning'
  form.enabled = true
  form.template_validation_scopes = []
  form.scope_json = '{}'
  form.rule_dsl_json = ''
  form.command_format_text = ''
  form.command_format_required = false
  form.pipeline_parameter_input = ''
  form.pipeline_parameters = []
  form.message = ''
  form.suggestion = ''
}

function resetEditorFormState() {
  resetForm()
  formRef.value?.clearValidate?.()
}

function isSupportedRuleTarget(value: string) {
  return value === 'oss' || value === 'gos'
}

function defaultRuleCheckForTarget(target: string) {
  return target === 'gos' ? 'artifact_url' : 'command_format'
}

function ensureRuleCheckCompatible() {
  const matched = ruleCheckOptions.some((item) => item.value === form.rule_check && item.targets.includes(form.rule_target))
  if (!matched) {
    form.rule_check = defaultRuleCheckForTarget(form.rule_target)
  }
}

function applyGOSArtifactURLDefaults() {
  if (form.rule_target !== 'gos' || form.rule_check !== 'artifact_url') {
    return
  }
  if (!form.rule_name.trim()) {
    form.rule_name = 'GOS 制品地址输出规范'
  }
  if (!form.message.trim()) {
    form.message = '缺少 GOS_ARTIFACT_URL 制品地址输出'
  }
  if (!form.suggestion.trim()) {
    form.suggestion = 'OSS 上传成功后输出 echo "GOS_ARTIFACT_URL=..."'
  }
}

function applyRuleCodeParts(ruleCode: string) {
  const parts = String(ruleCode || '').trim().split('.')
  const profileIndex = parts.indexOf('standard')
  if (parts.length < 4 || profileIndex < 3 || parts[0] !== 'artifact' || !isSupportedRuleTarget(parts[1])) {
    resetRuleCodeParts()
    return
  }
  form.rule_domain = 'artifact'
  form.rule_target = parts[1] || 'oss'
  form.rule_check = parts.slice(2, profileIndex).join('_') || 'command_format'
  form.rule_profile = 'standard'
  ensureRuleCheckCompatible()
}

function applyRuleTypeParts(ruleType: string) {
  const normalized = String(ruleType || '').trim()
  const parts = normalized.split('_')
  if (parts.length < 4 || parts[0] !== 'artifact' || !isSupportedRuleTarget(parts[1]) || parts[parts.length - 1] !== 'standard') {
    resetRuleCodeParts()
    return
  }
  form.rule_domain = 'artifact'
  form.rule_target = parts[1]
  form.rule_check = parts.slice(2, -1).join('_') || 'command_format'
  form.rule_profile = 'standard'
  ensureRuleCheckCompatible()
}

function resetRuleCodeParts() {
  form.rule_domain = 'artifact'
  form.rule_target = 'oss'
  form.rule_check = 'command_format'
  form.rule_profile = 'standard'
}

function formatTime(value: string) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function severityColor(severity: PipelineScanSeverity) {
  if (severity === 'error') {
    return 'red'
  }
  if (severity === 'warning') {
    return 'orange'
  }
  return 'blue'
}

function categoryText(category: PipelineScanCategory) {
  return categoryOptions.find((item) => item.value === category)?.label || category
}

function severityText(severity: PipelineScanSeverity) {
  return severityOptions.find((item) => item.value === severity)?.label || severity
}

function templateValidationScopesText(scopes: PipelineScanTemplateValidationScope[]) {
  if (!Array.isArray(scopes) || scopes.length === 0) {
    return '-'
  }
  return scopes.map((item) => item.toUpperCase()).join(' / ')
}

function isBuiltinRule(record: PipelineScanRule) {
  return record.builtin === true
}

function ruleSourceText(record: PipelineScanRule) {
  return isBuiltinRule(record) ? '内置' : '自定义'
}

function canEditRule(record: PipelineScanRule) {
  return !isBuiltinRule(record)
}

function canDeleteRule(record: PipelineScanRule) {
  return !isBuiltinRule(record)
}

function isRuleEnableToggling(record: PipelineScanRule) {
  return togglingRuleIDs.value.has(record.id)
}

function setRuleEnableToggling(id: string, loading: boolean) {
  const next = new Set(togglingRuleIDs.value)
  if (loading) {
    next.add(id)
  } else {
    next.delete(id)
  }
  togglingRuleIDs.value = next
}

async function loadRules() {
  loadingRules.value = true
  try {
    const response = await listPipelineScanRules({
      keyword: filters.keyword.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    rules.value = response.data
    ruleTotal.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '管线规则加载失败'))
  } finally {
    loadingRules.value = false
  }
}

function openSearchDialog() {
  searchDraft.keyword = String(filters.keyword || '').trim()
  searchDialogVisible.value = true
  void nextTick(() => {
    searchInputRef.value?.focus?.()
  })
}

function closeSearchDialog() {
  searchDialogVisible.value = false
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
    const response = await listPipelineScanRules({
      keyword,
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: String(item.id || '').trim(),
      title: String(item.rule_name || '').trim(),
      subtitle: String(item.rule_code || item.message || '').trim(),
      query: String(item.rule_code || item.rule_name || '').trim(),
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

function submitSearchDialog() {
  filters.keyword = String(searchDraft.keyword || '').trim()
  filters.page = 1
  searchDialogVisible.value = false
  void loadRules()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  submitSearchDialog()
}

function openCreateModal() {
  resetEditorFormState()
  editorMode.value = 'create'
  editorVisible.value = true
}

function openViewModal(record: PipelineScanRule) {
  viewingRule.value = record
  viewerVisible.value = true
}

function openEditModal(record: PipelineScanRule) {
  if (!canEditRule(record)) {
    message.warning('内置管线规范不可编辑')
    return
  }
  formRef.value?.clearValidate?.()
  editorMode.value = 'edit'
  form.id = record.id
  form.rule_code = record.rule_code || ''
  if (record.rule_type) {
    applyRuleTypeParts(record.rule_type || '')
  } else if (record.rule_code) {
    applyRuleCodeParts(record.rule_code)
  } else {
    resetRuleCodeParts()
  }
  form.rule_name = record.rule_name || ''
  form.severity = record.severity || 'warning'
  form.enabled = record.enabled
  form.template_validation_scopes = [...(record.template_validation_scopes || [])]
  form.scope_json = record.scope_json || '{}'
  form.rule_dsl_json = record.rule_dsl_json || ''
  form.command_format_text = extractCommandFormatSource(record.rule_dsl_json || '')
  form.command_format_required = extractCommandFormatRequireBlock(record.rule_dsl_json || '')
  form.pipeline_parameter_input = ''
  form.pipeline_parameters = extractPipelineParameters(record.rule_dsl_json || '')
  form.message = record.message || ''
  form.suggestion = record.suggestion || ''
  editorVisible.value = true
}

function closeEditor() {
  editorVisible.value = false
  submitting.value = false
}

function closeViewer() {
  viewerVisible.value = false
  viewingRule.value = null
}

function formatRuleDSL(rawDSL: string) {
  const raw = String(rawDSL || '').trim()
  if (!raw) {
    return '-'
  }
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function normalizeCommandFormatLines(raw: string) {
  return String(raw || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && line !== '"""' && line !== "'''" && !/^sh\s+["']{3}\s*$/.test(line))
}

function inferStartPattern(lines: string[]) {
  const ossLine = lines.find((line) => /ossutil\s+cp/.test(line))
  if (ossLine) {
    return 'ossutil\\s+cp'
  }
  const first = lines[0] || ''
  const command = first.match(/^[^\s]+/)?.[0] || ''
  return command ? escapeRegExp(command) : '.+'
}

function buildCommandFormatDSL(raw: string, requireBlock: boolean) {
  const lines = normalizeCommandFormatLines(raw)
  if (lines.length === 0) {
    throw new Error('完整命令不能为空')
  }
  return JSON.stringify(
    {
      matcher: {
        type: 'command_format',
        start_pattern: inferStartPattern(lines),
        format: {
          max_lines: Math.max(lines.length + 2, 8),
          allow_extra_lines: false,
          require_block: requireBlock === true,
          source_text: raw,
          lines: lines.map((line, index) => ({
            name: `第 ${index + 1} 行`,
            pattern: `^\\s*${escapeRegExp(line)}\\s*$`,
          })),
        },
      },
    },
    null,
    2,
  )
}

function normalizePipelineParameter(value: string) {
  return String(value || '').trim().toUpperCase()
}

function normalizePipelineParameters(values: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  values.forEach((item) => {
    const key = normalizePipelineParameter(item)
    if (!key) {
      return
    }
    if (seen.has(key)) {
      return
    }
    seen.add(key)
    result.push(key)
  })
  return result
}

function buildPipelineParametersDSL(values: string[]) {
  const requiredParameters = normalizePipelineParameters(values)
  if (requiredParameters.length === 0) {
    throw new Error('请至少添加一个管线参数')
  }
  return JSON.stringify(
    {
      matcher: {
        type: 'pipeline_parameters',
        required_parameters: requiredParameters,
      },
    },
    null,
    2,
  )
}

function buildGOSArtifactURLDSL() {
  return JSON.stringify(
    {
      matcher: {
        type: 'regex',
        pattern: '(?m)\\bGOS_ARTIFACT_URL\\s*=',
      },
    },
    null,
    2,
  )
}

function extractCommandFormatSource(rawDSL: string) {
  try {
    const parsed = JSON.parse(rawDSL || '{}')
    return String(parsed?.matcher?.format?.source_text || '')
  } catch {
    return ''
  }
}

function extractCommandFormatRequireBlock(rawDSL: string) {
  try {
    const parsed = JSON.parse(rawDSL || '{}')
    return parsed?.matcher?.format?.require_block === true
  } catch {
    return false
  }
}

function extractPipelineParameters(rawDSL: string) {
  try {
    const parsed = JSON.parse(rawDSL || '{}')
    if (parsed?.matcher?.type !== 'pipeline_parameters') {
      return []
    }
    return normalizePipelineParameters(Array.isArray(parsed?.matcher?.required_parameters) ? parsed.matcher.required_parameters : [])
  } catch {
    return []
  }
}

function addPipelineParameter() {
  const value = normalizePipelineParameter(form.pipeline_parameter_input)
  if (!value) {
    return
  }
  form.pipeline_parameters = normalizePipelineParameters([...form.pipeline_parameters, value])
  form.pipeline_parameter_input = ''
}

function removePipelineParameter(value: string) {
  const key = normalizePipelineParameter(value)
  form.pipeline_parameters = form.pipeline_parameters.filter((item) => normalizePipelineParameter(item) !== key)
}

function handleRuleCheckChange(value: string | number) {
  form.rule_check = String(value || '') || 'command_format'
  form.rule_profile = 'standard'
  form.pipeline_parameter_input = ''
  applyGOSArtifactURLDefaults()
}

function handleRuleTargetChange(value: string | number) {
  const target = String(value || '')
  form.rule_target = isSupportedRuleTarget(target) ? target : 'oss'
  form.rule_profile = 'standard'
  form.pipeline_parameter_input = ''
  ensureRuleCheckCompatible()
  applyGOSArtifactURLDefaults()
}

async function submitRule() {
  if (editorMode.value === 'edit') {
    const current = rules.value.find((item) => item.id === form.id)
    if (current && !canEditRule(current)) {
      message.warning('内置管线规范不可编辑')
      return
    }
  }
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  let ruleDSL = ''
  try {
    if (form.rule_check === 'pipeline_params') {
      ruleDSL = buildPipelineParametersDSL(form.pipeline_parameters)
    } else if (form.rule_check === 'artifact_url') {
      ruleDSL = buildGOSArtifactURLDSL()
    } else {
      ruleDSL = buildCommandFormatDSL(form.command_format_text, form.command_format_required)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '规则内容格式不正确')
    return
  }
  submitting.value = true
  try {
    const payload = {
      rule_type: generatedRuleType.value,
      rule_name: form.rule_name.trim(),
      category: form.rule_domain as PipelineScanCategory,
      severity: form.severity,
      enabled: form.enabled,
      template_validation_scopes: [...form.template_validation_scopes],
      scope_json: '{}',
      rule_dsl_json: ruleDSL,
      message: form.message.trim(),
      suggestion: form.suggestion.trim(),
    }
    if (editorMode.value === 'create') {
      await createPipelineScanRule(payload)
      message.success('规则已新增')
    } else {
      await updatePipelineScanRule(form.id, payload)
      message.success('规则已更新')
    }
    closeEditor()
    await loadRules()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editorMode.value === 'create' ? '新增规则失败' : '更新规则失败'))
  } finally {
    submitting.value = false
  }
}

async function removeRule(record: PipelineScanRule) {
  if (!canDeleteRule(record)) {
    message.warning('内置管线规范不可删除')
    return
  }
  try {
    await deletePipelineScanRule(record.id)
    message.success('规则已删除')
    await loadRules()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除规则失败'))
  }
}

async function toggleRuleEnabled(record: PipelineScanRule, checked: boolean | string | number) {
  const id = String(record.id || '').trim()
  if (!id) {
    return
  }
  const enabled = checked === true
  setRuleEnableToggling(id, true)
  try {
    const response = await setPipelineScanRuleEnabled(id, enabled)
    const updated = response.data
    const index = rules.value.findIndex((item) => item.id === id)
    if (index >= 0) {
      rules.value[index] = updated
    }
    if (viewingRule.value?.id === id) {
      viewingRule.value = updated
    }
    message.success(enabled ? '规范已启用' : '规范已停用')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, enabled ? '启用规范失败' : '停用规范失败'))
  } finally {
    setRuleEnableToggling(id, false)
  }
}

function handleRulePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.pageSize = pageSize
  void loadRules()
}

function readRuleFormViewportInset() {
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

function syncRuleFormViewportInset() {
  ruleFormViewportInset.value = readRuleFormViewportInset()
}

function observeRuleFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }
  ruleFormViewportObserver?.disconnect()
  ruleFormViewportObserver = new ResizeObserver(syncRuleFormViewportInset)
  if (appLayout) {
    ruleFormViewportObserver.observe(appLayout)
  }
  if (sider) {
    ruleFormViewportObserver.observe(sider)
  }
}

function stopObservingRuleFormViewportInset() {
  ruleFormViewportObserver?.disconnect()
  ruleFormViewportObserver = null
}

function handleFormAfterClose() {
  submitting.value = false
}

watch(
  [
    () => searchDialogVisible.value,
    () => String(searchDraft.keyword || '').trim(),
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

onMounted(() => {
  syncRuleFormViewportInset()
  observeRuleFormViewportInset()
  void loadRules()
})

onBeforeUnmount(() => {
  resetSearchSuggestions()
  stopObservingRuleFormViewportInset()
})
</script>

<template>
  <div class="pipeline-rule-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">管线规范</h2>
      </div>
      <div class="page-header-actions">
        <a-button class="toolbar-action-btn toolbar-icon-btn" title="搜索" @click="openSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-button class="toolbar-action-btn" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增规则
        </a-button>
      </div>
    </div>

    <transition name="pipeline-rule-search-fade">
      <div v-if="searchDialogVisible" class="pipeline-rule-search-overlay" @click.self="closeSearchDialog">
        <div class="pipeline-rule-search-floating-panel">
          <div class="pipeline-rule-search-floating-input">
            <SearchOutlined class="pipeline-rule-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="pipeline-rule-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="模糊搜索全部"
              @keydown.enter="submitSearchDialog"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="pipeline-rule-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="pipeline-rule-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="pipeline-rule-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="pipeline-rule-search-suggestion-title">{{ item.title }}</span>
                <span v-if="item.subtitle" class="pipeline-rule-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-table
      row-key="id"
      class="pipeline-rule-table"
      :columns="ruleColumns"
      :data-source="rules"
      :loading="loadingRules"
      :pagination="false"
      :locale="tableLocale"
      :scroll="{ x: 1280 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'rule_name'">
          <div class="primary-cell">{{ record.rule_name }}</div>
          <div class="secondary-cell">{{ record.message }}</div>
        </template>
        <template v-else-if="column.key === 'category'">
          <a-tag>{{ categoryText(record.category) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'builtin'">
          <a-tag :color="record.builtin ? 'blue' : 'default'">{{ ruleSourceText(record) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'severity'">
          <a-tag :color="severityColor(record.severity)">{{ severityText(record.severity) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'updated_at'">
          {{ formatTime(record.updated_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <div class="row-actions">
            <a-switch
              size="small"
              :checked="record.enabled"
              :loading="isRuleEnableToggling(record)"
              checked-children="启用"
              un-checked-children="停用"
              @change="(checked) => toggleRuleEnabled(record, checked)"
            />
            <a-button
              type="link"
              size="small"
              class="row-action-btn"
              title="查看"
              @click="openViewModal(record)"
            >
              <template #icon>
                <EyeOutlined />
              </template>
              查看
            </a-button>
            <a-button
              type="link"
              size="small"
              class="row-action-btn"
              :disabled="!canEditRule(record)"
              :title="canEditRule(record) ? '编辑' : '内置管线规范不可编辑'"
              @click="openEditModal(record)"
            >
              <template #icon>
                <EditOutlined />
              </template>
              编辑
            </a-button>
            <a-popconfirm
              v-if="canDeleteRule(record)"
              title="确认删除当前规则吗？"
              ok-text="删除"
              cancel-text="取消"
              @confirm="removeRule(record)"
            >
              <a-button type="link" size="small" class="row-action-btn" danger>
                <template #icon>
                  <DeleteOutlined />
                </template>
                删除
              </a-button>
            </a-popconfirm>
            <a-button
              v-else
              type="link"
              size="small"
              class="row-action-btn"
              danger
              disabled
              title="内置管线规范不可删除"
            >
              <template #icon>
                <DeleteOutlined />
              </template>
              删除
            </a-button>
          </div>
        </template>
      </template>
    </a-table>

    <div class="pagination-area">
      <a-pagination
        :current="filters.page"
        :page-size="filters.pageSize"
        :total="ruleTotal"
        show-size-changer
        show-quick-jumper
        :show-total="(count: number) => `共 ${count} 条`"
        @change="handleRulePageChange"
      />
    </div>

    <a-modal
      :open="viewerVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="ruleFormMaskStyle"
      :wrap-props="ruleFormWrapProps"
      wrap-class-name="pipeline-rule-form-modal-wrap"
      @cancel="closeViewer"
    >
      <template #title>
        <div class="pipeline-rule-form-titlebar">
          <span class="pipeline-rule-form-title">查看管线规范</span>
          <a-button class="toolbar-action-btn pipeline-rule-form-save" @click="closeViewer">
            关闭
          </a-button>
        </div>
      </template>

      <div v-if="viewingRule" class="pipeline-rule-viewer">
        <div class="pipeline-rule-viewer-heading">
          <div>
            <div class="pipeline-rule-viewer-name">{{ viewingRule.rule_name || '-' }}</div>
            <div class="pipeline-rule-viewer-code">{{ viewingRule.rule_code || '-' }}</div>
          </div>
          <div class="pipeline-rule-viewer-tags">
            <a-tag :color="viewingRule.builtin ? 'blue' : 'default'">{{ ruleSourceText(viewingRule) }}</a-tag>
            <a-tag :color="severityColor(viewingRule.severity)">{{ severityText(viewingRule.severity) }}</a-tag>
            <a-tag :color="viewingRule.enabled ? 'green' : 'default'">{{ viewingRule.enabled ? '启用' : '停用' }}</a-tag>
          </div>
        </div>

        <a-descriptions :column="1" bordered size="small" class="pipeline-rule-view-descriptions">
          <a-descriptions-item label="分类">{{ categoryText(viewingRule.category) }}</a-descriptions-item>
          <a-descriptions-item label="发布模板校验">
            {{ templateValidationScopesText(viewingRule.template_validation_scopes || []) }}
          </a-descriptions-item>
          <a-descriptions-item label="提示文案">{{ viewingRule.message || '-' }}</a-descriptions-item>
          <a-descriptions-item label="修复建议">{{ viewingRule.suggestion || '-' }}</a-descriptions-item>
          <a-descriptions-item label="创建时间">{{ formatTime(viewingRule.created_at) }}</a-descriptions-item>
          <a-descriptions-item label="更新时间">{{ formatTime(viewingRule.updated_at) }}</a-descriptions-item>
        </a-descriptions>

        <section class="pipeline-rule-form-panel">
          <div class="pipeline-rule-form-panel-title">规则 DSL</div>
          <pre class="pipeline-rule-view-code-block">{{ formatRuleDSL(viewingRule.rule_dsl_json) }}</pre>
        </section>
      </div>
    </a-modal>

    <a-modal
      :open="editorVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="ruleFormMaskStyle"
      :wrap-props="ruleFormWrapProps"
      wrap-class-name="pipeline-rule-form-modal-wrap"
      :after-close="handleFormAfterClose"
      @cancel="closeEditor"
    >
      <template #title>
        <div class="pipeline-rule-form-titlebar">
          <span class="pipeline-rule-form-title">
            {{ editorMode === 'create' ? '新增管线规则' : '编辑管线规则' }}
          </span>
          <a-button class="toolbar-action-btn pipeline-rule-form-save" :loading="submitting" @click="submitRule">
            保存
          </a-button>
        </div>
      </template>

        <a-form ref="formRef" class="pipeline-rule-form" layout="vertical" :model="form" :rules="formRules" :required-mark="false">
          <div class="pipeline-rule-form-note">
            {{ editorMode === 'create' ? '按规则域、对象、检查项和模板拼接规则类型，规则编码由后台生成' : '编辑态保持已有后台编码不变' }}
          </div>

        <section class="pipeline-rule-form-panel">
          <div class="pipeline-rule-form-panel-title">规则类型</div>
          <div class="form-grid">
            <a-form-item name="rule_domain">
              <template #label>
                <span class="pipeline-rule-form-label">规则域<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
              </template>
              <a-select v-model:value="form.rule_domain" :options="ruleDomainOptions" />
            </a-form-item>
            <a-form-item name="rule_target">
              <template #label>
                <span class="pipeline-rule-form-label">规则对象<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
              </template>
              <a-select v-model:value="form.rule_target" :options="ruleTargetOptions" @change="handleRuleTargetChange" />
            </a-form-item>
            <a-form-item name="rule_check">
              <template #label>
                <span class="pipeline-rule-form-label">检查项<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
              </template>
              <a-select
                v-model:value="form.rule_check"
                :options="availableRuleCheckOptions"
                @change="handleRuleCheckChange"
              />
            </a-form-item>
            <a-form-item name="rule_profile">
              <template #label>
                <span class="pipeline-rule-form-label">规则模板<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
              </template>
              <a-select v-model:value="form.rule_profile" :options="ruleProfileOptions" :disabled="true" />
              <div class="form-help-text is-block">当前版本仅支持标准规则模板</div>
            </a-form-item>
            <a-form-item class="form-grid-full" label="生成编码">
              <a-input :value="displayedRuleCode" disabled />
              <div class="form-help-text is-block">提交时拼接类型：{{ generatedRuleType }}</div>
            </a-form-item>
          </div>
        </section>

        <section class="pipeline-rule-form-panel">
          <div class="pipeline-rule-form-panel-title">规则内容</div>
          <div class="form-grid">
            <a-form-item name="rule_name">
              <template #label>
                <span class="pipeline-rule-form-label">规则名称<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="form.rule_name" placeholder="OSS 上传必须设置 ACL" />
            </a-form-item>
            <a-form-item label="级别">
              <a-select v-model:value="form.severity" :options="severityOptions" />
            </a-form-item>
            <a-form-item label="发布模板校验">
              <a-select
                v-model:value="form.template_validation_scopes"
                mode="multiple"
                allow-clear
                placeholder="请选择 CI 或 CD"
                :options="templateValidationScopeOptions"
              />
              <span class="form-help-text">命中这些范围的模板会在列表标记违规，发布单创建时不可选择违规模板</span>
            </a-form-item>
            <a-form-item v-if="form.rule_check === 'command_format'" label="为空判断">
              <a-switch
                v-model:checked="form.command_format_required"
                :checked-value="true"
                :un-checked-value="false"
                checked-children="开启"
                un-checked-children="关闭"
              />
              <span class="form-help-text">未匹配到完整命令时提示“管线中未找到该完整命令”</span>
            </a-form-item>
          </div>
          <a-form-item label="提示文案">
            <a-input v-model:value="form.message" placeholder="扫描命中后显示的提示" />
          </a-form-item>
          <a-form-item label="修复建议">
            <a-input v-model:value="form.suggestion" placeholder="可选" />
          </a-form-item>
          <a-form-item label="作用域">
            <a-input value="{}" disabled />
            <div class="form-help-text is-block">当前版本暂不支持编辑作用域，默认对全部管线生效</div>
          </a-form-item>
        </section>

        <section v-if="form.rule_check === 'command_format'" class="pipeline-rule-form-panel">
          <div class="pipeline-rule-form-panel-title">完整命令</div>
          <a-form-item name="command_format_text">
            <template #label>
              <span class="pipeline-rule-form-label">命令模板<a-tag class="pipeline-rule-form-required-tag">必填</a-tag></span>
            </template>
            <a-textarea
              v-model:value="form.command_format_text"
              class="dsl-textarea"
              :auto-size="{ minRows: 10, maxRows: 22 }"
              placeholder="粘贴 Jenkinsfile 里的完整上传命令"
            />
          </a-form-item>
        </section>
        <section v-if="form.rule_check === 'pipeline_params'" class="pipeline-rule-form-panel">
          <div class="pipeline-rule-form-panel-title">管线参数</div>
          <a-form-item label="参数名称">
            <div class="pipeline-parameter-entry">
              <a-input
                v-model:value="form.pipeline_parameter_input"
                placeholder="输入 Jenkins 参数名"
                @press-enter="addPipelineParameter"
              />
              <a-button class="pipeline-parameter-add-btn" @click="addPipelineParameter">
                <template #icon><PlusOutlined /></template>
                添加
              </a-button>
            </div>
            <div v-if="form.pipeline_parameters.length" class="pipeline-parameter-tags">
              <a-tag
                v-for="item in form.pipeline_parameters"
                :key="item"
                closable
                @close="removePipelineParameter(item)"
              >
                {{ item }}
              </a-tag>
            </div>
            <div v-else class="pipeline-parameter-empty">暂未添加参数</div>
            <div class="form-help-text is-block">手动输入 Jenkins 参数名后添加，可配置多条管线参数</div>
          </a-form-item>
        </section>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.pipeline-rule-page {
  color: #0f172a;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.page-title {
  margin: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  line-height: 1.35;
}

.page-header-actions,
.row-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

:deep(.toolbar-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  border-radius: 16px;
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

:deep(.toolbar-icon-btn.ant-btn) {
  width: 42px;
  padding-inline: 0;
}

:deep(.toolbar-action-btn.ant-btn:hover),
:deep(.toolbar-action-btn.ant-btn:focus),
:deep(.toolbar-action-btn.ant-btn:focus-visible),
:deep(.toolbar-action-btn.ant-btn:active) {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.pipeline-rule-search-overlay {
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

.pipeline-rule-search-floating-panel {
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

.pipeline-rule-search-floating-input {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 0 14px;
  border: 1px solid rgba(255, 255, 255, 0.74);
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.6)),
    rgba(255, 255, 255, 0.44);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 16px 32px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(125%);
  -webkit-backdrop-filter: blur(18px) saturate(125%);
}

.pipeline-rule-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.66)),
    rgba(255, 255, 255, 0.5);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 18px 36px rgba(15, 23, 42, 0.1);
}

.pipeline-rule-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.pipeline-rule-search-floating-field {
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

.pipeline-rule-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.pipeline-rule-search-suggestions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border: 1px solid rgba(255, 255, 255, 0.62);
  border-radius: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.52), rgba(255, 255, 255, 0.36)),
    rgba(255, 255, 255, 0.22);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.74),
    0 16px 30px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(124%);
  -webkit-backdrop-filter: blur(18px) saturate(124%);
}

.pipeline-rule-search-suggestion {
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

.pipeline-rule-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
  transform: translateY(-1px);
}

.pipeline-rule-search-suggestion-loading {
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.28);
  color: rgba(51, 65, 85, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.pipeline-rule-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.pipeline-rule-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.74);
  font-size: 12px;
  line-height: 1.4;
}

.pipeline-rule-search-fade-enter-active,
.pipeline-rule-search-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.pipeline-rule-search-fade-enter-from,
.pipeline-rule-search-fade-leave-to {
  opacity: 0;
}

.pipeline-rule-search-fade-enter-from .pipeline-rule-search-floating-panel,
.pipeline-rule-search-fade-leave-to .pipeline-rule-search-floating-panel {
  opacity: 0;
  transform: translateY(-8px);
}

.pipeline-rule-search-fade-enter-to .pipeline-rule-search-floating-panel,
.pipeline-rule-search-fade-leave-from .pipeline-rule-search-floating-panel {
  opacity: 1;
  transform: translateY(0);
}

.pipeline-rule-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(226, 232, 240, 0.92);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.36);
}

.pipeline-rule-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #eff6ff !important;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.pipeline-rule-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.pipeline-rule-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: #334155;
  font-size: 13px;
}

.pipeline-rule-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.pipeline-rule-table :deep(.ant-table-cell-fix-right) {
  background: rgba(255, 255, 255, 0.96) !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.04);
}

.pipeline-rule-table :deep(.ant-table-thead .ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  box-shadow: none;
}

.pipeline-rule-table :deep(.resizable-header-cell) {
  position: relative;
  cursor: col-resize;
  user-select: none;
}

.pipeline-rule-table :deep(.resizable-header-cell::after) {
  content: '';
  position: absolute;
  top: 25%;
  right: 0;
  width: 1px;
  height: 50%;
  background: rgba(203, 213, 225, 0.88);
}

.primary-cell {
  color: #0f172a;
  font-weight: 700;
  line-height: 1.5;
}

.secondary-cell {
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

:deep(.row-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 8px;
  font-weight: 600;
  padding-inline: 0;
}

.pagination-area {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}

.form-grid-full {
  grid-column: 1 / -1;
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal) {
  padding-bottom: 32px;
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal-content) {
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
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.34), transparent 36%);
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal-title) {
  position: relative;
  z-index: 1;
}

.pipeline-rule-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding: 18px 28px 28px;
}

.pipeline-rule-form-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.pipeline-rule-form-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
  line-height: 1.4;
}

:deep(.pipeline-rule-form-save.ant-btn) {
  flex: none;
  height: 42px;
  border-radius: 16px;
  padding: 0 14px;
  background: rgba(255, 255, 255, 0.72);
  border-color: rgba(203, 213, 225, 0.76);
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.8);
}

.pipeline-rule-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.pipeline-rule-viewer {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.pipeline-rule-viewer-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.pipeline-rule-viewer-name {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
  line-height: 1.45;
}

.pipeline-rule-viewer-code {
  margin-top: 4px;
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.5;
  word-break: break-all;
}

.pipeline-rule-viewer-tags {
  display: flex;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.pipeline-rule-viewer-tags :deep(.ant-tag) {
  margin-inline-end: 0;
}

.pipeline-rule-view-descriptions :deep(.ant-descriptions-view) {
  border-color: rgba(203, 213, 225, 0.78);
}

.pipeline-rule-view-descriptions :deep(.ant-descriptions-item-label) {
  width: 132px;
  background: rgba(248, 250, 252, 0.78);
  color: #475569;
  font-weight: 700;
}

.pipeline-rule-view-code-block {
  max-height: 360px;
  margin: 0;
  overflow: auto;
  padding: 14px;
  border: 1px solid rgba(203, 213, 225, 0.76);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.04);
  color: #0f172a;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.pipeline-rule-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.pipeline-rule-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.pipeline-rule-form :deep(.ant-input),
.pipeline-rule-form :deep(.ant-select-selector),
.pipeline-rule-form :deep(.ant-input-affix-wrapper) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.pipeline-rule-form :deep(.ant-input:hover),
.pipeline-rule-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.pipeline-rule-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.pipeline-rule-form :deep(.ant-input:focus),
.pipeline-rule-form :deep(.ant-input-focused),
.pipeline-rule-form :deep(.ant-input-affix-wrapper-focused),
.pipeline-rule-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(96, 165, 250, 0.72) !important;
  box-shadow: 0 0 0 3px rgba(96, 165, 250, 0.14) !important;
}

.pipeline-rule-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.pipeline-rule-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

.pipeline-rule-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.pipeline-rule-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.pipeline-rule-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.pipeline-rule-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.pipeline-rule-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.dsl-textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
}

.pipeline-parameter-entry {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pipeline-parameter-entry :deep(.ant-input) {
  flex: 1;
  min-width: 0;
}

:deep(.pipeline-parameter-add-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
  border-radius: 10px;
  font-weight: 700;
}

.pipeline-parameter-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.pipeline-parameter-tags :deep(.ant-tag) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-inline-end: 0;
  padding: 4px 8px;
  border-radius: 8px;
  border-color: rgba(191, 219, 254, 0.82);
  background: rgba(239, 246, 255, 0.9);
  color: #1e3a8a;
  font-weight: 700;
}

.pipeline-parameter-empty {
  margin-top: 10px;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
}

.form-help-text {
  margin-left: 10px;
  color: #64748b;
  font-size: 12px;
}

.form-help-text.is-block {
  margin-left: 0;
  margin-top: 6px;
}

@media (max-width: 768px) {
  .page-header,
  .page-header-actions {
    align-items: stretch;
    flex-direction: column;
  }

  :deep(.toolbar-icon-btn.ant-btn) {
    width: 100%;
  }

    .form-grid {
      grid-template-columns: 1fr;
    }

    .pipeline-parameter-entry {
      align-items: stretch;
      flex-direction: column;
    }
  }
  </style>
