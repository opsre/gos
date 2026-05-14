<script setup lang="ts">
import { LinkOutlined, SearchOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getExecutorParamDefByID,
  listApplicationExecutorParamDefs,
  listExecutorParamDefs,
  updateExecutorParamDef,
} from '../../api/pipeline'
import { listPlatformParamDicts } from '../../api/platform-param'
import { useResizableColumns } from '../../composables/useResizableColumns'
import type { BindingType, ExecutorParamDef } from '../../types/pipeline'
import type { PlatformParamDict } from '../../types/platform-param'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface MappingFormState {
  id: string
  application_id: string
  application_name: string
  application_key: string
  binding_type: BindingType | ''
  pipeline_id: string
  pipeline_name: string
  executor_param_name: string
  param_key?: string
}

interface PlatformParamOption {
  label: string
  value: string
}

interface RouteBindingHint {
  id: string
  pipeline_id: string
  pipeline_name: string
}

interface InitialRouteContext {
  application_id: string
  binding_type: BindingType
  binding_hint: RouteBindingHint | null
}

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

interface AutoMappingTarget {
  id: string
  targetParamKey: string
}

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const mappingOptionsLoading = ref(false)
const mappingSubmitting = ref(false)
const autoMappingSubmitting = ref(false)
const dataSource = ref<ExecutorParamDef[]>([])
const total = ref(0)
const routeBindingHint = ref<RouteBindingHint | null>(null)
const initialRouteContext = ref<InitialRouteContext | null>(null)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailData = ref<ExecutorParamDef | null>(null)
const mappingModalViewportInset = ref(0)
const mappingVisible = ref(false)
const mappingFormRef = ref<FormInstance>()
const mappingOptions = ref<PlatformParamOption[]>([])
const searchDialogVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0

const filters = reactive({
  application_id: '',
  keyword: '',
  binding_type: 'ci' as BindingType,
  status: '' as '' | 'active' | 'inactive',
  visible: '' as '' | 'true' | 'false',
  editable: '' as '' | 'true' | 'false',
  page: 1,
  pageSize: 20,
})

const statusFilterValue = computed<string | undefined>({
  get: () => filters.status || undefined,
  set: (value) => {
    filters.status = value === 'active' || value === 'inactive' ? value : ''
  },
})
const visibleFilterValue = computed<string | undefined>({
  get: () => filters.visible || undefined,
  set: (value) => {
    filters.visible = value === 'true' || value === 'false' ? value : ''
  },
})
const editableFilterValue = computed<string | undefined>({
  get: () => filters.editable || undefined,
  set: (value) => {
    filters.editable = value === 'true' || value === 'false' ? value : ''
  },
})

const searchDraft = reactive({
  keyword: '',
})

const mappingForm = reactive<MappingFormState>({
  id: '',
  application_id: '',
  application_name: '',
  application_key: '',
  binding_type: '',
  pipeline_id: '',
  pipeline_name: '',
  executor_param_name: '',
  param_key: undefined,
})

const initialColumns: TableColumnsType<ExecutorParamDef> = [
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 260 },
  { title: '所属执行器', dataIndex: 'pipeline_name', key: 'pipeline_name', width: 220 },
  { title: '真实参数名', dataIndex: 'executor_param_name', key: 'executor_param_name', width: 220 },
  { title: '平台标准 Key', dataIndex: 'param_key', key: 'param_key', width: 180 },
  { title: '参数类型', dataIndex: 'param_type', key: 'param_type', width: 120 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 110 },
  { title: '操作', key: 'actions', width: 180, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, { minWidth: 90, maxWidth: 520, hitArea: 10 })

const hasKeyword = computed(() => String(filters.keyword || '').trim() !== '')
const showBindingLink = computed(() => Boolean(filters.application_id) && !hasKeyword.value)
const emptyDescription = computed(() => {
  if (hasKeyword.value) {
    return '未找到匹配的 Jenkins 执行器参数'
  }
  if (filters.application_id) {
    return '当前应用暂无可展示的 Jenkins 执行器参数'
  }
  return '暂无 Jenkins 执行器参数'
})
const tableLocale = computed(() => ({
  emptyText: emptyDescription.value,
}))
const mappingApplicationLabel = computed(() => {
  const name = String(mappingForm.application_name || '').trim()
  const key = String(mappingForm.application_key || '').trim()
  const applicationID = String(mappingForm.application_id || '').trim()
  if (name && key) {
    return `${name} (${key})`
  }
  return name || key || applicationID || '-'
})
const mappingFormReadonlyFields = computed(() => [
  { label: '应用', value: mappingApplicationLabel.value },
  { label: '绑定类型', value: String(mappingForm.binding_type || filters.binding_type || '').trim() || '-' },
  { label: '所属执行器', value: String(mappingForm.pipeline_name || mappingForm.pipeline_id || '').trim() || '-' },
  { label: '真实参数名', value: String(mappingForm.executor_param_name || '').trim() || '-' },
])
const mappingModalMaskStyle = computed(() => ({
  left: `${mappingModalViewportInset.value}px`,
  width: `calc(100% - ${mappingModalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: mappingVisible.value ? 'auto' : 'none',
}))
const mappingModalWrapProps = computed(() => ({
  style: {
    left: `${mappingModalViewportInset.value}px`,
    width: `calc(100% - ${mappingModalViewportInset.value}px)`,
    pointerEvents: mappingVisible.value ? 'auto' : 'none',
  },
}))

let mappingModalViewportObserver: ResizeObserver | null = null

function readMappingModalViewportInset() {
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

function syncMappingModalViewportInset() {
  mappingModalViewportInset.value = readMappingModalViewportInset()
}

function observeMappingModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  mappingModalViewportObserver?.disconnect()
  mappingModalViewportObserver = new ResizeObserver(() => {
    syncMappingModalViewportInset()
  })

  if (appLayout) {
    mappingModalViewportObserver.observe(appLayout)
  }
  if (sider) {
    mappingModalViewportObserver.observe(sider)
  }
}

function stopObservingMappingModalViewportInset() {
  mappingModalViewportObserver?.disconnect()
  mappingModalViewportObserver = null
}

function cloneRouteBindingHint(value: RouteBindingHint | null): RouteBindingHint | null {
  if (!value) {
    return null
  }
  return {
    id: value.id,
    pipeline_id: value.pipeline_id,
    pipeline_name: value.pipeline_name,
  }
}

function captureInitialRouteContext() {
  initialRouteContext.value = {
    application_id: filters.application_id,
    binding_type: filters.binding_type,
    binding_hint: cloneRouteBindingHint(routeBindingHint.value),
  }
}

function restoreInitialRouteContext() {
  filters.application_id = initialRouteContext.value?.application_id || ''
  filters.binding_type = initialRouteContext.value?.binding_type || 'ci'
  routeBindingHint.value = cloneRouteBindingHint(initialRouteContext.value?.binding_hint || null)
}

function formatTime(value: string) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function boolText(value: boolean) {
  return value ? '是' : '否'
}

function statusColor(status: string) {
  return status === 'inactive' ? 'default' : 'green'
}

function parseBooleanFilter(value: '' | 'true' | 'false') {
  if (value === '') {
    return undefined
  }
  return value === 'true'
}

function normalizeAutoMappingName(value: unknown) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
}

function registerAutoMappingCandidate(paramMap: Map<string, string>, value: unknown, paramKey: string) {
  const normalizedName = normalizeAutoMappingName(value)
  const normalizedParamKey = String(paramKey || '').trim().toLowerCase()
  if (!normalizedName || !normalizedParamKey || paramMap.has(normalizedName)) {
    return
  }
  paramMap.set(normalizedName, normalizedParamKey)
}

function formatApplicationLabel(record: Partial<ExecutorParamDef>) {
  const name = String(record.application_name || '').trim()
  const key = String(record.application_key || '').trim()
  const applicationID = String(record.application_id || '').trim()
  if (name && key) {
    return `${name} (${key})`
  }
  return name || key || applicationID || '-'
}

function formatPipelineLabel(record: Partial<ExecutorParamDef>) {
  const pipelineName = String(record.pipeline_name || '').trim()
  const hintedName = String(routeBindingHint.value?.pipeline_name || '').trim()
  const pipelineID = String(record.pipeline_id || '').trim()
  const hintedID = String(routeBindingHint.value?.pipeline_id || '').trim()
  return pipelineName || hintedName || pipelineID || hintedID || '-'
}

function syncRouteQuery() {
  const nextQuery: Record<string, string> = {}
  const keyword = String(filters.keyword || '').trim()
  if (filters.application_id && !keyword) {
    nextQuery.application_id = filters.application_id
  }
  if (filters.binding_type) {
    nextQuery.binding_type = filters.binding_type
  }
  if (keyword) {
    nextQuery.keyword = keyword
  } else if (routeBindingHint.value) {
    if (routeBindingHint.value.id) {
      nextQuery.pipeline_binding_id = routeBindingHint.value.id
    }
    if (routeBindingHint.value.pipeline_id) {
      nextQuery.pipeline_id = routeBindingHint.value.pipeline_id
    }
    if (routeBindingHint.value.pipeline_name) {
      nextQuery.pipeline_name = routeBindingHint.value.pipeline_name
    }
  }
  void router.replace({ path: '/components/executor-params', query: nextQuery })
}

function applyRouteQuery() {
  filters.application_id = String(route.query.application_id || '').trim()
  filters.keyword = String(route.query.keyword || '').trim()

  const bindingType = String(route.query.binding_type || '').trim()
  if (bindingType === 'ci' || bindingType === 'cd') {
    filters.binding_type = bindingType
  }

  const pipelineBindingID = String(route.query.pipeline_binding_id || '').trim()
  const pipelineID = String(route.query.pipeline_id || '').trim()
  const pipelineName = String(route.query.pipeline_name || '').trim()
  routeBindingHint.value =
    pipelineBindingID || pipelineID || pipelineName
      ? {
          id: pipelineBindingID,
          pipeline_id: pipelineID,
          pipeline_name: pipelineName,
        }
      : null
}

async function loadPlatformParamOptions(currentParamKey = '') {
  mappingOptionsLoading.value = true
  try {
    const response = await listPlatformParamDicts({
      status: 1,
      page: 1,
      page_size: 100,
    })
    const options = response.data
      .filter((item: PlatformParamDict) => !item.cd_self_fill)
      .map((item: PlatformParamDict) => ({
        value: item.param_key,
        label: `${item.name} (${item.param_key})`,
      }))
    if (currentParamKey && !options.some((item) => item.value === currentParamKey)) {
      options.unshift({
        value: currentParamKey,
        label: `${currentParamKey} (当前值)`,
      })
    }
    mappingOptions.value = options
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字库下拉加载失败'))
  } finally {
    mappingOptionsLoading.value = false
  }
}

async function loadAutoMappingParamMap() {
  const paramMap = new Map<string, string>()
  let page = 1
  const pageSize = 100

  while (true) {
    const response = await listPlatformParamDicts({
      status: 1,
      page,
      page_size: pageSize,
    })
    const items = response.data || []
    items
      .filter((item: PlatformParamDict) => !item.cd_self_fill)
      .forEach((item: PlatformParamDict) => {
        registerAutoMappingCandidate(paramMap, item.param_key, item.param_key)
        registerAutoMappingCandidate(paramMap, item.name, item.param_key)
      })

    if (items.length === 0 || page * (response.page_size || pageSize) >= response.total) {
      break
    }
    page += 1
  }

  return paramMap
}

async function loadExecutorParams() {
  const keyword = String(filters.keyword || '').trim()
  loading.value = true
  try {
    const commonParams = {
      binding_type: filters.binding_type,
      status: filters.status || undefined,
      visible: parseBooleanFilter(filters.visible),
      editable: parseBooleanFilter(filters.editable),
      page: filters.page,
      page_size: filters.pageSize,
    }
    const response =
      filters.application_id && !keyword
        ? await listApplicationExecutorParamDefs(filters.application_id, commonParams)
        : await listExecutorParamDefs({
            ...commonParams,
            keyword: keyword || undefined,
          })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
  } catch (error) {
    dataSource.value = []
    total.value = 0
    message.error(extractHTTPErrorMessage(error, '执行器参数加载失败'))
  } finally {
    loading.value = false
  }
}

async function listExecutorParamsForAutoMapping() {
  const keyword = String(filters.keyword || '').trim()
  const items: ExecutorParamDef[] = []
  let page = 1
  const pageSize = 100

  while (true) {
    const commonParams = {
      binding_type: filters.binding_type,
      status: filters.status || undefined,
      visible: parseBooleanFilter(filters.visible),
      editable: parseBooleanFilter(filters.editable),
      page,
      page_size: pageSize,
    }
    const response =
      filters.application_id && !keyword
        ? await listApplicationExecutorParamDefs(filters.application_id, commonParams)
        : await listExecutorParamDefs({
            ...commonParams,
            keyword: keyword || undefined,
          })
    const pageItems = response.data || []
    items.push(...pageItems)

    if (pageItems.length === 0 || items.length >= response.total) {
      break
    }
    page += 1
  }

  return items
}

function buildAutoMappingTargets(items: ExecutorParamDef[], paramMap: Map<string, string>) {
  const targets: AutoMappingTarget[] = []
  for (const item of items) {
    if (!item.can_edit) {
      continue
    }
    const normalizedExecutorParamName = normalizeAutoMappingName(item.executor_param_name)
    const targetParamKey = paramMap.get(normalizedExecutorParamName)
    if (!targetParamKey) {
      continue
    }
    const currentParamKey = String(item.param_key || '').trim().toLowerCase()
    if (currentParamKey === targetParamKey) {
      continue
    }
    targets.push({
      id: item.id,
      targetParamKey,
    })
  }
  return targets
}

function handleAutoMapExecutorParams() {
  if (autoMappingSubmitting.value) {
    return
  }

  Modal.confirm({
    title: '确认一键映射参数？',
    content:
      '系统会读取当前筛选范围内的管线参数，将字段名称与管线参数名称一致的字段自动绑定。' +
      '已有映射如果与匹配结果不一致会被覆盖，请确认后再执行。',
    okText: '确认映射',
    cancelText: '取消',
    async onOk() {
      await runAutoMapExecutorParams()
    },
  })
}

async function runAutoMapExecutorParams() {
  if (autoMappingSubmitting.value) {
    return
  }

  autoMappingSubmitting.value = true
  try {
    const [paramMap, items] = await Promise.all([loadAutoMappingParamMap(), listExecutorParamsForAutoMapping()])
    const targets = buildAutoMappingTargets(items, paramMap)
    if (!targets.length) {
      message.info('没有找到可自动映射的参数')
      return
    }

    let successCount = 0
    for (const item of targets) {
      const targetParamKey = item.targetParamKey
      await updateExecutorParamDef(item.id, {
        param_key: targetParamKey,
      })
      successCount += 1
    }

    message.success(`已自动映射 ${successCount} 个参数`)
    await loadExecutorParams()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '一键映射失败'))
  } finally {
    autoMappingSubmitting.value = false
  }
}

function openSearchDialog() {
  searchDraft.keyword = String(filters.keyword || '').trim()
  searchDialogVisible.value = true
  void nextTick(() => {
    searchInputRef.value?.focus()
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

function buildSearchSuggestionQuery(item: ExecutorParamDef, keyword: string) {
  const normalizedKeyword = String(keyword || '').trim().toLowerCase()
  const applicationName = String(item.application_name || '').trim()
  const applicationKey = String(item.application_key || '').trim()
  const paramKey = String(item.param_key || '').trim()

  if (normalizedKeyword) {
    if (paramKey && paramKey.toLowerCase().includes(normalizedKeyword)) {
      return paramKey
    }
    if (applicationKey && applicationKey.toLowerCase().includes(normalizedKeyword)) {
      return applicationKey
    }
    if (applicationName && applicationName.toLowerCase().includes(normalizedKeyword)) {
      return applicationName
    }
  }

  return paramKey || applicationKey || applicationName
}

async function loadSearchSuggestions(keyword: string) {
  const requestSeq = ++searchSuggestionRequestSeq
  searchSuggestionsLoading.value = true
  try {
    const response = await listExecutorParamDefs({
      keyword,
      binding_type: filters.binding_type,
      status: filters.status || undefined,
      visible: parseBooleanFilter(filters.visible),
      editable: parseBooleanFilter(filters.editable),
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => {
      const applicationLabel = formatApplicationLabel(item)
      const pipelineLabel = formatPipelineLabel(item)
      const paramKey = String(item.param_key || '').trim()
      const subtitleParts = [paramKey, pipelineLabel].filter(Boolean)
      return {
        id: String(item.id || '').trim(),
        title: applicationLabel,
        subtitle: subtitleParts.join(' · '),
        query: buildSearchSuggestionQuery(item, keyword),
      }
    })
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
  const keyword = String(searchDraft.keyword || '').trim()
  filters.page = 1
  filters.pageSize = 20
  filters.keyword = keyword
  if (keyword) {
    filters.application_id = ''
    routeBindingHint.value = null
  } else {
    restoreInitialRouteContext()
  }
  searchDialogVisible.value = false
  syncRouteQuery()
  void loadExecutorParams()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  handleSearchSubmit()
}

function handleToolbarFilterChange() {
  filters.page = 1
  syncRouteQuery()
  void loadExecutorParams()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.pageSize = pageSize
  void loadExecutorParams()
}

async function openDetailDrawer(record: ExecutorParamDef) {
  detailVisible.value = true
  detailLoading.value = true
  detailData.value = null
  try {
    const response = await getExecutorParamDefByID(record.id)
    detailData.value = {
      ...response.data,
      application_id: response.data.application_id || record.application_id,
      application_name: response.data.application_name || record.application_name,
      application_key: response.data.application_key || record.application_key,
      binding_type: response.data.binding_type || record.binding_type,
      pipeline_name: response.data.pipeline_name || record.pipeline_name,
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '参数详情加载失败'))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

function closeDetailDrawer() {
  detailVisible.value = false
  detailData.value = null
}

async function openMappingModal(record: ExecutorParamDef) {
  mappingSubmitting.value = false
  try {
    const response = await getExecutorParamDefByID(record.id)
    const item = response.data
    mappingForm.id = item.id
    mappingForm.application_id = item.application_id || record.application_id || ''
    mappingForm.application_name = item.application_name || record.application_name || ''
    mappingForm.application_key = item.application_key || record.application_key || ''
    mappingForm.binding_type = item.binding_type || record.binding_type || filters.binding_type
    mappingForm.pipeline_id = item.pipeline_id || record.pipeline_id || ''
    mappingForm.pipeline_name = item.pipeline_name || record.pipeline_name || ''
    mappingForm.executor_param_name = item.executor_param_name
    mappingForm.param_key = item.param_key || undefined
    await loadPlatformParamOptions(item.param_key)
    mappingVisible.value = true
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '参数详情加载失败'))
  }
}

function closeMappingModal() {
  mappingVisible.value = false
  mappingForm.id = ''
  mappingForm.application_id = ''
  mappingForm.application_name = ''
  mappingForm.application_key = ''
  mappingForm.binding_type = ''
  mappingForm.pipeline_id = ''
  mappingForm.pipeline_name = ''
  mappingForm.executor_param_name = ''
  mappingForm.param_key = undefined
}

async function submitMapping() {
  await mappingFormRef.value?.validate()
  mappingSubmitting.value = true
  try {
    await updateExecutorParamDef(mappingForm.id, {
      param_key: String(mappingForm.param_key || '').trim(),
    })
    message.success('平台标准参数映射更新成功')
    closeMappingModal()
    await loadExecutorParams()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '平台标准参数映射更新失败'))
  } finally {
    mappingSubmitting.value = false
  }
}

onMounted(async () => {
  syncMappingModalViewportInset()
  observeMappingModalViewportInset()
  applyRouteQuery()
  captureInitialRouteContext()
  await loadExecutorParams()
})

watch(
  () => searchDialogVisible.value,
  (visible) => {
    if (!visible) {
      resetSearchSuggestions()
      return
    }
    const keyword = String(searchDraft.keyword || '').trim()
    if (!keyword) {
      resetSearchSuggestions()
      return
    }
    void loadSearchSuggestions(keyword)
  },
)

watch(
  () => String(searchDraft.keyword || '').trim(),
  (keyword) => {
    if (!searchDialogVisible.value) {
      return
    }
    if (!keyword) {
      resetSearchSuggestions()
      return
    }
    if (searchSuggestionTimer) {
      window.clearTimeout(searchSuggestionTimer)
      searchSuggestionTimer = null
    }
    searchSuggestionTimer = window.setTimeout(() => {
      void loadSearchSuggestions(keyword)
    }, 220)
  },
)

onUnmounted(() => {
  resetSearchSuggestions()
  stopObservingMappingModalViewportInset()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header">
      <div class="page-header-copy">
        <h2 class="page-title">参数</h2>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-select
          v-model:value="filters.binding_type"
          class="executor-toolbar-select"
          :options="[
            { label: '绑定类型 · ci', value: 'ci' },
            { label: '绑定类型 · cd', value: 'cd' },
          ]"
        />
        <a-select
          v-model:value="statusFilterValue"
          class="executor-toolbar-select"
          placeholder="状态"
          :options="[
            { label: '状态 · active', value: 'active' },
            { label: '状态 · inactive', value: 'inactive' },
          ]"
        />
        <a-select
          v-model:value="visibleFilterValue"
          class="executor-toolbar-select"
          placeholder="展示"
          :options="[
            { label: '展示 · 是', value: 'true' },
            { label: '展示 · 否', value: 'false' },
          ]"
        />
        <a-select
          v-model:value="editableFilterValue"
          class="executor-toolbar-select"
          placeholder="可编辑"
          :options="[
            { label: '可编辑 · 是', value: 'true' },
            { label: '可编辑 · 否', value: 'false' },
          ]"
        />
        <a-button class="executor-toolbar-query-btn" @click="handleToolbarFilterChange">查询</a-button>
        <a-button
          class="application-toolbar-action-btn executor-auto-map-btn"
          :loading="autoMappingSubmitting"
          :disabled="loading"
          @click="handleAutoMapExecutorParams"
        >
          <template #icon>
            <ThunderboltOutlined />
          </template>
          一键映射
        </a-button>
        <a-button
          v-if="showBindingLink"
          class="page-header-link-btn"
          @click="router.push(`/applications/${filters.application_id}/pipeline-bindings`)"
        >
          <template #icon>
            <LinkOutlined />
          </template>
          查看管线绑定
        </a-button>
      </div>
    </div>

    <transition name="executor-search-fade">
      <div v-if="searchDialogVisible" class="executor-search-overlay" @click.self="closeSearchDialog">
        <div class="executor-search-floating-panel">
          <div class="executor-search-floating-input">
            <SearchOutlined class="executor-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="executor-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="应用 / 应用Key / 平台Key"
              @keydown.enter="handleSearchSubmit"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="executor-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="executor-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="executor-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="executor-search-suggestion-title">{{ item.title }}</span>
                <span v-if="item.subtitle" class="executor-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-card class="table-card" :bordered="true">
      <a-table
        class="executor-param-table"
        row-key="id"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        :locale="tableLocale"
        :scroll="{ x: 1290 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'application_name'">
            {{ formatApplicationLabel(record) }}
          </template>
          <template v-else-if="column.key === 'pipeline_name'">
            {{ formatPipelineLabel(record) }}
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'param_key'">
            {{ record.param_key || '-' }}
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-button type="link" size="small" @click="openDetailDrawer(record)">查看</a-button>
              <a-button v-if="record.can_edit" type="link" size="small" @click="openMappingModal(record)">
                编辑映射
              </a-button>
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
    </a-card>

    <a-modal
      :open="mappingVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="mappingModalMaskStyle"
      :wrap-props="mappingModalWrapProps"
      wrap-class-name="executor-mapping-modal-wrap"
      @cancel="closeMappingModal"
    >
      <template #title>
        <div class="executor-mapping-modal-titlebar">
          <span class="executor-mapping-modal-title">编辑平台标准参数映射</span>
          <a-button
            class="application-toolbar-action-btn executor-mapping-modal-save-btn"
            :loading="mappingSubmitting"
            @click="submitMapping"
          >
            保存
          </a-button>
        </div>
      </template>

      <a-form
        ref="mappingFormRef"
        :model="mappingForm"
        layout="vertical"
        :required-mark="false"
        class="executor-mapping-form"
      >
        <div class="executor-mapping-form-note">
          真实参数名保持只读；平台标准参数 Key 可以重新选择，也可以清空以取消当前映射。
        </div>

        <div class="executor-mapping-form-panel executor-mapping-form-panel--context">
          <div class="executor-mapping-form-panel-title">当前上下文</div>
          <div class="executor-mapping-form-context">
            <div
              v-for="item in mappingFormReadonlyFields"
              :key="item.label"
              class="executor-mapping-form-context-item"
            >
              <div class="executor-mapping-form-context-label">{{ item.label }}</div>
              <div class="executor-mapping-form-context-value">{{ item.value }}</div>
            </div>
          </div>
        </div>

        <div class="executor-mapping-form-panel">
          <div class="executor-mapping-form-panel-title">映射配置</div>

          <a-form-item name="param_key">
            <template #label>
              <span class="executor-mapping-form-label">
                平台标准参数 Key
                <a-tag class="executor-mapping-form-optional-tag">可空</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="mappingForm.param_key"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="请选择平台标准字段，也可清空取消映射"
              :loading="mappingOptionsLoading"
              :options="mappingOptions"
            />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer :open="detailVisible" title="执行器参数详情" width="720" @close="closeDetailDrawer">
      <a-skeleton v-if="detailLoading" active :paragraph="{ rows: 10 }" />
      <a-descriptions v-else-if="detailData" :column="1" bordered>
        <a-descriptions-item label="参数 ID">{{ detailData.id }}</a-descriptions-item>
        <a-descriptions-item label="应用">{{ formatApplicationLabel(detailData) }}</a-descriptions-item>
        <a-descriptions-item label="绑定类型">{{ detailData.binding_type || filters.binding_type }}</a-descriptions-item>
        <a-descriptions-item label="所属执行器">{{ formatPipelineLabel(detailData) }}</a-descriptions-item>
        <a-descriptions-item label="管线 ID">{{ detailData.pipeline_id }}</a-descriptions-item>
        <a-descriptions-item label="执行器类型">{{ detailData.executor_type }}</a-descriptions-item>
        <a-descriptions-item label="真实参数名">{{ detailData.executor_param_name }}</a-descriptions-item>
        <a-descriptions-item label="平台标准 Key">{{ detailData.param_key || '-' }}</a-descriptions-item>
        <a-descriptions-item label="参数类型">{{ detailData.param_type }}</a-descriptions-item>
        <a-descriptions-item label="必填">{{ boolText(detailData.required) }}</a-descriptions-item>
        <a-descriptions-item label="默认值">{{ detailData.default_value || '-' }}</a-descriptions-item>
        <a-descriptions-item label="参数描述">{{ detailData.description || '-' }}</a-descriptions-item>
        <a-descriptions-item label="展示">{{ boolText(detailData.visible) }}</a-descriptions-item>
        <a-descriptions-item label="可编辑">{{ boolText(detailData.editable) }}</a-descriptions-item>
        <a-descriptions-item label="来源">{{ detailData.source_from }}</a-descriptions-item>
        <a-descriptions-item label="排序">{{ detailData.sort_no }}</a-descriptions-item>
        <a-descriptions-item label="创建时间">{{ formatTime(detailData.created_at) }}</a-descriptions-item>
        <a-descriptions-item label="更新时间">{{ formatTime(detailData.updated_at) }}</a-descriptions-item>
        <a-descriptions-item label="原始结构">
          <pre class="raw-meta-block">{{ detailData.raw_meta || '{}' }}</pre>
        </a-descriptions-item>
      </a-descriptions>
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

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
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

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

.executor-param-table :deep(.ant-table),
.executor-param-table :deep(.ant-table-content),
.executor-param-table :deep(.ant-table-body) {
  border-radius: 0 !important;
  background: transparent;
}

.executor-param-table :deep(.ant-table-container) {
  overflow: hidden;
  border-radius: 0 !important;
  border: 1px solid rgba(226, 232, 240, 0.92);
}

.executor-param-table :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, #243247, #1f2a3d);
  color: rgba(239, 246, 255, 0.96);
  border-bottom: none;
  border-radius: 0 !important;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.executor-param-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.76);
  border-radius: 0 !important;
  background: rgba(255, 255, 255, 0.64);
  transition: background 0.18s ease;
}

.executor-param-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.executor-param-table :deep(.ant-table-cell-fix-right) {
  background: #fff !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.05);
}

.executor-param-table :deep(.ant-table-cell-fix-left) {
  background: #fff !important;
}

.executor-param-table :deep(.ant-table-thead > tr > th.ant-table-cell-fix-left),
.executor-param-table :deep(.ant-table-thead > tr > th.ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
}

.executor-param-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right) {
  background: #f8fafc !important;
}

.executor-param-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-left) {
  background: #f8fafc !important;
}

.raw-meta-block {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: 'SFMono-Regular', 'Menlo', monospace;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.application-toolbar-icon-btn.ant-btn),
:deep(.page-header-link-btn.ant-btn),
:deep(.executor-toolbar-query-btn.ant-btn) {
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

:deep(.application-toolbar-action-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  min-width: 42px;
  padding-inline: 0;
}

:deep(.page-header-link-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.executor-toolbar-query-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.executor-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selector) {
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

:deep(.executor-toolbar-select.ant-select .ant-select-selection-wrap) {
  height: 100%;
  display: flex;
  align-items: center;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selection-item),
:deep(.executor-toolbar-select.ant-select .ant-select-selection-placeholder),
:deep(.executor-toolbar-select.ant-select .ant-select-arrow),
:deep(.executor-toolbar-select.ant-select .ant-select-clear) {
  color: #0f172a !important;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selection-item),
:deep(.executor-toolbar-select.ant-select .ant-select-selection-search-input) {
  font-weight: 600;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selection-placeholder) {
  display: flex;
  align-items: center;
  height: 100%;
  line-height: 1 !important;
  color: rgba(15, 23, 42, 0.54) !important;
  font-weight: 600;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selection-search) {
  display: flex;
  align-items: center;
  height: 100%;
}

:deep(.executor-toolbar-select.ant-select .ant-select-selection-search-input) {
  height: 100% !important;
  line-height: 42px !important;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active),
:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.application-toolbar-icon-btn.ant-btn:active),
:deep(.page-header-link-btn.ant-btn:hover),
:deep(.page-header-link-btn.ant-btn:focus),
:deep(.page-header-link-btn.ant-btn:focus-visible),
:deep(.page-header-link-btn.ant-btn:active),
:deep(.executor-toolbar-query-btn.ant-btn:hover),
:deep(.executor-toolbar-query-btn.ant-btn:focus),
:deep(.executor-toolbar-query-btn.ant-btn:focus-visible),
:deep(.executor-toolbar-query-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.executor-mapping-modal-wrap .ant-modal) {
  padding-bottom: 32px;
}

:deep(.executor-mapping-modal-wrap .ant-modal-content) {
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

:deep(.executor-mapping-modal-wrap .ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

:deep(.executor-mapping-modal-wrap .ant-modal-body) {
  padding: 20px 28px 28px;
}

.executor-mapping-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.executor-mapping-modal-title {
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

:deep(.executor-mapping-modal-save-btn.ant-btn) {
  flex: none;
  position: relative;
  overflow: hidden;
  height: 42px;
  padding-inline: 18px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.56), rgba(255, 255, 255, 0.24)),
    rgba(255, 255, 255, 0.18) !important;
  color: #0f172a !important;
  font-size: 14px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.84),
    inset 0 -10px 18px rgba(255, 255, 255, 0.14),
    0 14px 28px rgba(15, 23, 42, 0.08) !important;
  backdrop-filter: blur(18px) saturate(150%);
}

:deep(.executor-mapping-modal-save-btn.ant-btn)::before {
  content: '';
  position: absolute;
  inset: 1px 1px auto;
  height: 48%;
  border-radius: 15px 15px 10px 10px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.54), rgba(255, 255, 255, 0));
  pointer-events: none;
}

:deep(.executor-mapping-modal-save-btn.ant-btn > span) {
  position: relative;
  z-index: 1;
}

:deep(.executor-mapping-modal-save-btn.ant-btn:hover),
:deep(.executor-mapping-modal-save-btn.ant-btn:focus),
:deep(.executor-mapping-modal-save-btn.ant-btn:focus-visible),
:deep(.executor-mapping-modal-save-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.38) !important;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.66), rgba(255, 255, 255, 0.32)),
    rgba(255, 255, 255, 0.24) !important;
  color: #0f172a !important;
}

.executor-mapping-form {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.executor-mapping-form-note {
  position: relative;
  padding-left: 16px;
  color: rgba(51, 65, 85, 0.88);
  font-size: 13px;
  line-height: 1.7;
}

.executor-mapping-form-note::before {
  content: '';
  position: absolute;
  top: 2px;
  left: 0;
  width: 4px;
  height: calc(100% - 4px);
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.82), rgba(96, 165, 250, 0.34));
}

.executor-mapping-form-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.executor-mapping-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.executor-mapping-form-panel-title::after {
  content: '';
  flex: 1;
  min-width: 36px;
  height: 1px;
  background: linear-gradient(90deg, rgba(148, 163, 184, 0.34), rgba(148, 163, 184, 0));
}

.executor-mapping-form-note + .executor-mapping-form-panel,
.executor-mapping-form-panel + .executor-mapping-form-panel {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.88);
}

.executor-mapping-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 18px;
}

.executor-mapping-form-context-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-bottom: 10px;
  border-bottom: 1px dashed rgba(203, 213, 225, 0.92);
}

.executor-mapping-form-context-label {
  color: rgba(100, 116, 139, 0.9);
  font-size: 12px;
  font-weight: 600;
}

.executor-mapping-form-context-value {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.5;
  word-break: break-word;
}

.executor-mapping-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.executor-mapping-form-optional-tag {
  margin-inline-start: 0;
  padding-inline: 8px;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.26);
  background: rgba(241, 245, 249, 0.9);
  color: rgba(71, 85, 105, 0.88);
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
}

.executor-mapping-form :deep(.ant-form-item) {
  margin-bottom: 0;
}

.executor-mapping-form :deep(.ant-form-item-label) {
  padding-bottom: 6px;
}

.executor-mapping-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  min-height: auto;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.executor-mapping-form :deep(.ant-form-item-explain),
.executor-mapping-form :deep(.ant-form-item-explain-error) {
  min-height: 18px;
  line-height: 18px;
}

.executor-mapping-form :deep(.ant-input),
.executor-mapping-form :deep(.ant-input-affix-wrapper),
.executor-mapping-form :deep(.ant-select-selector) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(248, 250, 252, 0.58)) !important;
  border-color: rgba(255, 255, 255, 0.62) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 10px 20px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(16px) saturate(140%);
  border-radius: 14px !important;
  font-size: 13px;
  color: #0f172a !important;
}

.executor-mapping-form :deep(.ant-select-single:not(.ant-select-customize-input) .ant-select-selector) {
  min-height: 44px !important;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
}

.executor-mapping-form :deep(.ant-select-single .ant-select-selector) {
  display: flex;
  align-items: center;
  padding-inline: 14px !important;
}

.executor-mapping-form :deep(.ant-select-single .ant-select-selection-search),
.executor-mapping-form :deep(.ant-select-single .ant-select-selection-item),
.executor-mapping-form :deep(.ant-select-single .ant-select-selection-placeholder) {
  line-height: 42px !important;
}

.executor-mapping-form :deep(.ant-select .ant-select-arrow),
.executor-mapping-form :deep(.ant-select .ant-select-clear),
.executor-mapping-form :deep(.ant-select-selection-placeholder) {
  color: rgba(100, 116, 139, 0.72) !important;
}

.executor-mapping-form :deep(.ant-input:hover),
.executor-mapping-form :deep(.ant-input:focus),
.executor-mapping-form :deep(.ant-input-affix-wrapper:hover),
.executor-mapping-form :deep(.ant-input-affix-wrapper-focused),
.executor-mapping-form :deep(.ant-select:hover .ant-select-selector),
.executor-mapping-form :deep(.ant-select-focused .ant-select-selector) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.82), rgba(241, 245, 249, 0.66)) !important;
  border-color: rgba(147, 197, 253, 0.48) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 12px 24px rgba(59, 130, 246, 0.06) !important;
}

:deep(.executor-toolbar-select.ant-select:hover .ant-select-selector),
:deep(.executor-toolbar-select.ant-select.ant-select-focused .ant-select-selector),
:deep(.executor-toolbar-select.ant-select.ant-select-open .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.executor-search-overlay {
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

.executor-search-floating-panel {
  width: min(100%, 480px);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.executor-search-floating-input {
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

.executor-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.executor-search-floating-field {
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

.executor-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.executor-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.66)),
    rgba(255, 255, 255, 0.5);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 18px 36px rgba(15, 23, 42, 0.1);
}

.executor-search-suggestions {
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

.executor-search-suggestion {
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

.executor-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
  transform: translateY(-1px);
}

.executor-search-suggestion-loading {
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.28);
  color: rgba(51, 65, 85, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.executor-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.executor-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.78);
  font-size: 12px;
  font-weight: 600;
}

.executor-search-fade-enter-active,
.executor-search-fade-leave-active {
  transition: opacity 0.18s ease;
}

.executor-search-fade-enter-from,
.executor-search-fade-leave-to {
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

  :deep(.executor-toolbar-select.ant-select) {
    width: 100%;
  }

  .executor-mapping-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }

  .executor-mapping-form-context {
    grid-template-columns: 1fr;
  }

  :deep(.table-card .ant-card-body) {
    padding: 16px;
  }

  .executor-search-overlay {
    left: 0;
    padding: 72px 16px 16px;
  }
}
</style>
