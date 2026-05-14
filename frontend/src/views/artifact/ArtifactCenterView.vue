<script setup lang="ts">
import {
  DeleteOutlined,
  DownloadOutlined,
  FileOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  HomeOutlined,
  LeftOutlined,
  PlusOutlined,
  ReloadOutlined,
  RightOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listApplications } from '../../api/application'
import { deleteReleaseOrderArtifactMetadata, listReleaseOrderArtifacts, recordReleaseOrderArtifactMetadata } from '../../api/artifact'
import { listArtifactRepositories } from '../../api/artifact-repository'
import { listProjects } from '../../api/project'
import type { Application } from '../../types/application'
import type { ReleaseOrderArtifactMetadataSummary } from '../../types/artifact'
import type { ArtifactRepository } from '../../types/artifact-repository'
import type { Project } from '../../types/project'
import { extractHTTPErrorMessage } from '../../utils/http-error'

type ArtifactTreeNodeType = 'project' | 'application' | 'env' | 'release' | 'scope'

interface ArtifactTreeNode {
  key: string
  title: string
  type: ArtifactTreeNodeType
  path: string[]
  children?: ArtifactTreeNode[]
}

const router = useRouter()
const loading = ref(false)
const repositoryLoading = ref(false)
const projectLoading = ref(false)
const applicationLoading = ref(false)
const selectedTreeKeys = ref<string[]>([])
const selectedArtifact = ref<ReleaseOrderArtifactMetadataSummary | null>(null)
const visibleFileActionID = ref('')
const detailVisible = ref(false)
const manualArtifactVisible = ref(false)
const manualArtifactSubmitting = ref(false)
const manualArtifactViewportInset = ref(0)
const artifacts = ref<ReleaseOrderArtifactMetadataSummary[]>([])
const repositoryOptions = ref<{ label: string; value: string }[]>([])
const projectOptions = ref<{ label: string; value: string }[]>([])
const applicationOptions = ref<{ label: string; value: string }[]>([])
const ALL_FILTER_VALUE = '__all__'

const filters = reactive({
  repository_id: '',
  project_id: '',
  application_id: '',
  pipeline_scope: '',
})

const activeFilters = reactive({
  repository_id: '',
  project_id: '',
  application_id: '',
  pipeline_scope: '',
})

const pagination = reactive({
  page: 1,
  page_size: 50,
  total: 0,
})

const manualArtifactForm = reactive({
  release_order_id: '',
  artifact_name: '',
  artifact_url: '',
  pipeline_scope: 'ci',
  artifact_type: '',
  artifact_version: '',
  build_number: '',
  object_key: '',
  size_bytes: null as number | null,
  checksum_type: '',
  checksum: '',
})

const pipelineScopeOptions = [
  { label: 'CI', value: 'ci' },
  { label: 'CD', value: 'cd' },
]

const scopeOptions = [
  { label: '执行单元 · 全部', value: ALL_FILTER_VALUE },
  { label: '执行单元 · CI', value: pipelineScopeOptions[0].value },
  { label: '执行单元 · CD', value: pipelineScopeOptions[1].value },
]

const manualArtifactMaskStyle = computed(() => ({
  left: `${manualArtifactViewportInset.value}px`,
  width: `calc(100% - ${manualArtifactViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: manualArtifactVisible.value ? 'auto' : 'none',
}))

const manualArtifactWrapProps = computed(() => ({
  style: {
    left: `${manualArtifactViewportInset.value}px`,
    width: `calc(100% - ${manualArtifactViewportInset.value}px)`,
    pointerEvents: manualArtifactVisible.value ? 'auto' : 'none',
  },
}))

let manualArtifactViewportObserver: ResizeObserver | null = null

const artifactTreeData = computed<ArtifactTreeNode[]>(() => {
  const roots: ArtifactTreeNode[] = []
  const nodeMap = new Map<string, ArtifactTreeNode>()

  function ensureNode(parent: ArtifactTreeNode | null, node: ArtifactTreeNode) {
    const existing = nodeMap.get(node.key)
    if (existing) {
      return existing
    }
    nodeMap.set(node.key, node)
    if (parent) {
      parent.children = parent.children || []
      parent.children.push(node)
    } else {
      roots.push(node)
    }
    return node
  }

  for (const item of artifacts.value) {
    const projectTitle = item.project_name || '未归属项目'
    const appTitle = item.application_name || item.application_id || '未命名应用'
    const envTitle = item.env_code || '未配置环境'
    const releaseTitle = item.release_display_name || buildReleaseDisplayName(item)
    const scopeTitle = formatPipelineScope(item.pipeline_scope)

    const projectNode = ensureNode(null, {
      key: projectKey(item),
      title: projectTitle,
      type: 'project',
      path: [projectTitle],
    })
    const appNode = ensureNode(projectNode, {
      key: applicationKey(item),
      title: appTitle,
      type: 'application',
      path: [...projectNode.path, appTitle],
    })
    const envNode = ensureNode(appNode, {
      key: envKey(item),
      title: envTitle,
      type: 'env',
      path: [...appNode.path, envTitle],
    })
    const releaseNode = ensureNode(envNode, {
      key: releaseKey(item),
      title: releaseTitle,
      type: 'release',
      path: [...envNode.path, releaseTitle],
    })
    ensureNode(releaseNode, {
      key: scopeKey(item),
      title: scopeTitle,
      type: 'scope',
      path: [...releaseNode.path, scopeTitle],
    })
  }

  return roots
})

const visibleArtifacts = computed(() => {
  const key = selectedTreeKeys.value[0]
  if (!key) {
    return artifacts.value
  }
  return artifacts.value.filter((item) => artifactMatchesTreeKey(item, key))
})

const currentTreeNode = computed(() => {
  const key = selectedTreeKeys.value[0]
  return key ? findTreeNode(artifactTreeData.value, key) : null
})

const explorerDirectories = computed(() => {
  return currentTreeNode.value ? currentTreeNode.value.children || [] : artifactTreeData.value
})

const explorerFiles = computed(() => {
  return explorerDirectories.value.length > 0 ? [] : visibleArtifacts.value
})

const explorerStatusText = computed(() => {
  if (loading.value) {
    return '加载中'
  }
  return `${explorerDirectories.value.length} 个目录，${explorerFiles.value.length} 个文件`
})

const explorerPanelTitle = computed(() => currentTreeNode.value?.title || '根目录')

const canManualAddArtifact = computed(() => canQueryArtifacts.value && Boolean(currentTreeNode.value) && explorerDirectories.value.length === 0)

const selectedReleaseContext = computed(() => {
  const key = selectedTreeKeys.value[0]
  const matched = key
    ? artifacts.value.find((item) => releaseKey(item) === key || scopeKey(item) === key)
    : null
  if (matched?.release_order_id) {
    return buildReleaseContext(matched, scopeKey(matched) === key ? matched.pipeline_scope : '')
  }
  return inferReleaseContextFromArtifacts(visibleArtifacts.value)
})

const manualArtifactContextText = computed(() => {
  const context = selectedReleaseContext.value
  if (!context) {
    return '当前列表未定位到单个发布单'
  }
  const scope = context.pipeline_scope ? ` / ${formatPipelineScope(context.pipeline_scope)}` : ''
  return `${context.title}${scope}`
})

const projectFilterOptions = computed(() => [
  { label: '项目 · 全部', value: ALL_FILTER_VALUE },
  ...projectOptions.value,
])
const applicationFilterOptions = computed(() => [
  { label: '应用 · 全部', value: ALL_FILTER_VALUE },
  ...applicationOptions.value,
])

const projectFilterValue = computed<string | undefined>({
  get: () => filters.project_id || undefined,
  set: (value) => {
    filters.project_id = normalizeFilterValue(value)
  },
})
const applicationFilterValue = computed<string | undefined>({
  get: () => filters.application_id || undefined,
  set: (value) => {
    filters.application_id = normalizeFilterValue(value)
  },
})
const scopeFilterValue = computed<string | undefined>({
  get: () => filters.pipeline_scope || undefined,
  set: (value) => {
    filters.pipeline_scope = normalizeFilterValue(value)
  },
})

const canQueryArtifacts = computed(() => Boolean(activeFilters.repository_id))
const emptyText = computed(() => {
  if (!canQueryArtifacts.value) {
    return '请先选择制品库'
  }
  return loading.value ? '加载中' : '暂无制品'
})

async function loadRepositories() {
  repositoryLoading.value = true
  try {
    const response = await listArtifactRepositories({ page: 1, page_size: 200, status: 'enabled' })
    repositoryOptions.value = (response.data || []).map((item: ArtifactRepository) => ({
      label: item.bucket ? `${item.name} (${item.bucket})` : item.name,
      value: item.id,
    }))
    const firstRepositoryID = repositoryOptions.value[0]?.value
    if (!filters.repository_id && firstRepositoryID) {
      filters.repository_id = firstRepositoryID
      activeFilters.repository_id = firstRepositoryID
      activeFilters.project_id = ''
      activeFilters.application_id = ''
      activeFilters.pipeline_scope = ''
      void loadArtifacts()
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载制品库失败'))
  } finally {
    repositoryLoading.value = false
  }
}

async function loadProjects() {
  projectLoading.value = true
  try {
    const response = await listProjects({ page: 1, page_size: 200, status: 'active' })
    projectOptions.value = (response.data || []).map((item: Project) => ({
      label: item.key ? `${item.name} (${item.key})` : item.name,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载项目失败'))
  } finally {
    projectLoading.value = false
  }
}

async function loadApplications() {
  applicationLoading.value = true
  try {
    const response = await listApplications({
      page: 1,
      page_size: 200,
      project_id: filters.project_id || undefined,
      status: 'active',
    })
    applicationOptions.value = (response.data || []).map((item: Application) => ({
      label: item.key ? `${item.name} (${item.key})` : item.name,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载应用失败'))
  } finally {
    applicationLoading.value = false
  }
}

async function loadArtifacts() {
  if (!canQueryArtifacts.value) {
    artifacts.value = []
    pagination.total = 0
    return
  }
  loading.value = true
  try {
    const response = await listReleaseOrderArtifacts({
      repository_id: activeFilters.repository_id,
      project_id: activeFilters.project_id || undefined,
      application_id: activeFilters.application_id || undefined,
      pipeline_scope: activeFilters.pipeline_scope || undefined,
      page: pagination.page,
      page_size: pagination.page_size,
    })
    artifacts.value = response.data || []
    pagination.total = response.total || 0
    pagination.page = response.page || pagination.page
    pagination.page_size = response.page_size || pagination.page_size
    if (selectedTreeKeys.value[0] && !findTreeNode(artifactTreeData.value, selectedTreeKeys.value[0])) {
      selectedTreeKeys.value = []
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载制品失败'))
  } finally {
    loading.value = false
  }
}

function searchArtifacts() {
  activeFilters.repository_id = filters.repository_id
  activeFilters.project_id = filters.project_id
  activeFilters.application_id = filters.application_id
  activeFilters.pipeline_scope = filters.pipeline_scope
  pagination.page = 1
  selectedTreeKeys.value = []
  void loadArtifacts()
}

function handleProjectChange() {
  filters.application_id = ''
  void loadApplications()
}

function handleGridPageChange(page: number, pageSize: number) {
  pagination.page = Number(page || 1)
  pagination.page_size = Number(pageSize || 50)
  void loadArtifacts()
}

function showPaginationTotal(total: number) {
  return `共 ${total} 条`
}

function handleTreeSelect(keys: (string | number)[]) {
  hideFileActions()
  selectedTreeKeys.value = keys.map((item) => String(item))
}

function resetExplorerLocation() {
  hideFileActions()
  selectedTreeKeys.value = []
}

function openExplorerDirectory(node: ArtifactTreeNode) {
  hideFileActions()
  selectedTreeKeys.value = [node.key]
}

function showFileActions(record: ReleaseOrderArtifactMetadataSummary) {
  visibleFileActionID.value = record.id || ''
}

function hideFileActions() {
  visibleFileActionID.value = ''
}

function openManualArtifactModal() {
  if (!canManualAddArtifact.value) {
    message.warning('请进入最后一级目录后再手动添加')
    return
  }
  const context = selectedReleaseContext.value
  if (!context?.release_order_id) {
    message.warning('请先筛选到单个发布单后再手动添加')
    return
  }
  resetManualArtifactForm()
  manualArtifactForm.release_order_id = context.release_order_id
  manualArtifactForm.pipeline_scope = context.pipeline_scope || 'ci'
  manualArtifactVisible.value = true
}

function isManualArtifact(record: ReleaseOrderArtifactMetadataSummary) {
  return !String(record.execution_id || '').trim()
}

async function deleteManualArtifact(record: ReleaseOrderArtifactMetadataSummary) {
  hideFileActions()
  if (!isManualArtifact(record)) {
    message.warning('发布过程产出的制品会随发布单删除')
    return
  }
  if (!record.release_order_id || !record.id) {
    message.warning('制品记录信息不完整')
    return
  }
  Modal.confirm({
    title: '删除制品',
    content: `确认删除 ${record.artifact_name || '该制品'}？`,
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    async onOk() {
      try {
        await deleteReleaseOrderArtifactMetadata(record.release_order_id, record.id)
        message.success('制品已删除')
        if (selectedArtifact.value?.id === record.id) {
          detailVisible.value = false
          selectedArtifact.value = null
        }
        void loadArtifacts()
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '删除制品失败'))
      }
    },
  })
}

function closeManualArtifactModal() {
  manualArtifactVisible.value = false
}

async function submitManualArtifact() {
  if (!manualArtifactForm.release_order_id) {
    message.warning('请选择发布单')
    return
  }
  if (!manualArtifactForm.artifact_name.trim()) {
    message.warning('请输入文件名')
    return
  }
  if (!manualArtifactForm.artifact_url.trim()) {
    message.warning('请输入下载地址')
    return
  }

  manualArtifactSubmitting.value = true
  try {
    const repositoryID = selectedReleaseContext.value?.repository_id || activeFilters.repository_id || filters.repository_id
    await recordReleaseOrderArtifactMetadata(manualArtifactForm.release_order_id, {
      artifact_name: manualArtifactForm.artifact_name.trim(),
      artifact_url: manualArtifactForm.artifact_url.trim(),
      pipeline_scope: manualArtifactForm.pipeline_scope,
      artifact_type: manualArtifactForm.artifact_type.trim(),
      artifact_version: manualArtifactForm.artifact_version.trim(),
      build_number: manualArtifactForm.build_number.trim(),
      object_key: manualArtifactForm.object_key.trim(),
      checksum_type: manualArtifactForm.checksum_type.trim(),
      checksum: manualArtifactForm.checksum.trim(),
      size_bytes: Number(manualArtifactForm.size_bytes || 0),
      repository_id: repositoryID,
      metadata: { source: 'manual' },
    })
    message.success('文件已添加')
    manualArtifactVisible.value = false
    pagination.page = 1
    void loadArtifacts()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '添加文件失败'))
  } finally {
    manualArtifactSubmitting.value = false
  }
}

function openDetail(record: ReleaseOrderArtifactMetadataSummary) {
  hideFileActions()
  selectedArtifact.value = record
  detailVisible.value = true
}

function closeDetail() {
  detailVisible.value = false
}

function downloadArtifact(record: ReleaseOrderArtifactMetadataSummary, event?: MouseEvent) {
  event?.preventDefault()
  event?.stopPropagation()
  hideFileActions()
  const url = String(record.artifact_url || '').trim()
  if (!url) {
    message.warning('制品下载地址为空')
    return
  }
  const frame = document.createElement('iframe')
  frame.style.display = 'none'
  frame.style.width = '0'
  frame.style.height = '0'
  frame.setAttribute('aria-hidden', 'true')
  frame.src = url
  document.body.appendChild(frame)
  window.setTimeout(() => {
    frame.remove()
  }, 60_000)
}

function viewReleaseOrder(record: ReleaseOrderArtifactMetadataSummary) {
  if (!record.release_order_id) {
    return
  }
  void router.push(`/releases/${record.release_order_id}`)
}

function projectKey(item: ReleaseOrderArtifactMetadataSummary) {
  return `project:${item.project_id || item.project_name || 'unassigned'}`
}

function applicationKey(item: ReleaseOrderArtifactMetadataSummary) {
  return `${projectKey(item)}/application:${item.application_id || item.application_name || 'unknown'}`
}

function envKey(item: ReleaseOrderArtifactMetadataSummary) {
  return `${applicationKey(item)}/env:${item.env_code || 'unknown'}`
}

function releaseKey(item: ReleaseOrderArtifactMetadataSummary) {
  return `${envKey(item)}/release:${item.release_order_id || item.release_order_no || 'unknown'}`
}

function scopeKey(item: ReleaseOrderArtifactMetadataSummary) {
  return `${releaseKey(item)}/scope:${item.pipeline_scope || 'unknown'}`
}

function artifactMatchesTreeKey(item: ReleaseOrderArtifactMetadataSummary, key: string) {
  return [projectKey(item), applicationKey(item), envKey(item), releaseKey(item), scopeKey(item)].includes(key)
}

function inferReleaseContextFromArtifacts(items: ReleaseOrderArtifactMetadataSummary[]) {
  const validItems = items.filter((item) => item.release_order_id)
  const releaseOrderIDs = new Set(validItems.map((item) => item.release_order_id))
  if (releaseOrderIDs.size !== 1) {
    return null
  }
  const matched = validItems[0]
  const pipelineScopes = new Set(validItems.map((item) => item.pipeline_scope).filter(Boolean))
  return buildReleaseContext(matched, pipelineScopes.size === 1 ? matched.pipeline_scope : '')
}

function buildReleaseContext(item: ReleaseOrderArtifactMetadataSummary, pipelineScope = '') {
  return {
    release_order_id: item.release_order_id,
    title: item.release_display_name || buildReleaseDisplayName(item),
    pipeline_scope: pipelineScope,
    repository_id: item.repository_id || activeFilters.repository_id || filters.repository_id,
  }
}

function findTreeNode(nodes: ArtifactTreeNode[], key: string): ArtifactTreeNode | null {
  for (const node of nodes) {
    if (node.key === key) {
      return node
    }
    const child = findTreeNode(node.children || [], key)
    if (child) {
      return child
    }
  }
  return null
}

function buildReleaseDisplayName(item: ReleaseOrderArtifactMetadataSummary) {
  const name = String(item.release_name || '').trim()
  const no = String(item.release_order_no || '').trim()
  if (name && no) {
    return `${name} - ${no}`
  }
  return name || no || '-'
}

function formatPipelineScope(scope: string) {
  const value = String(scope || '').toLowerCase()
  if (value === 'ci') {
    return 'CI'
  }
  if (value === 'cd') {
    return 'CD'
  }
  return scope || '-'
}

function formatDate(value: string) {
  return value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '-'
}

function formatValue(value: unknown) {
  const text = String(value ?? '').trim()
  return text || '-'
}

function resetManualArtifactForm() {
  manualArtifactForm.release_order_id = ''
  manualArtifactForm.artifact_name = ''
  manualArtifactForm.artifact_url = ''
  manualArtifactForm.pipeline_scope = 'ci'
  manualArtifactForm.artifact_type = ''
  manualArtifactForm.artifact_version = ''
  manualArtifactForm.build_number = ''
  manualArtifactForm.object_key = ''
  manualArtifactForm.size_bytes = null
  manualArtifactForm.checksum_type = ''
  manualArtifactForm.checksum = ''
}

function treeNodeIcon(type: ArtifactTreeNodeType) {
  return type === 'scope' ? FileOutlined : FolderOutlined
}

function normalizeFilterValue(value: string | undefined) {
  if (!value || value === ALL_FILTER_VALUE) {
    return ''
  }
  return value
}

function readManualArtifactViewportInset() {
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

function syncManualArtifactViewportInset() {
  manualArtifactViewportInset.value = readManualArtifactViewportInset()
}

function observeManualArtifactViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  manualArtifactViewportObserver?.disconnect()
  manualArtifactViewportObserver = new ResizeObserver(() => {
    syncManualArtifactViewportInset()
  })

  if (appLayout) {
    manualArtifactViewportObserver.observe(appLayout)
  }
  if (sider) {
    manualArtifactViewportObserver.observe(sider)
  }
}

function stopObservingManualArtifactViewportInset() {
  manualArtifactViewportObserver?.disconnect()
  manualArtifactViewportObserver = null
}

onMounted(() => {
  syncManualArtifactViewportInset()
  observeManualArtifactViewportInset()
  void Promise.all([loadRepositories(), loadProjects(), loadApplications()])
})

onBeforeUnmount(() => {
  stopObservingManualArtifactViewportInset()
})
</script>

<template>
  <div class="artifact-center-page" @click="hideFileActions">
    <div class="page-header">
      <div>
        <div class="page-title">制品目录</div>
      </div>

      <div class="artifact-center-toolbar">
        <a-select
          v-model:value="filters.repository_id"
          class="artifact-toolbar-select artifact-repository-filter"
          show-search
          option-filter-prop="label"
          placeholder="制品库"
          :loading="repositoryLoading"
          :options="repositoryOptions"
        />
        <a-select
          v-model:value="projectFilterValue"
          class="artifact-toolbar-select artifact-filter"
          show-search
          option-filter-prop="label"
          placeholder="项目 · 全部"
          :loading="projectLoading"
          :options="projectFilterOptions"
          @change="handleProjectChange"
        />
        <a-select
          v-model:value="applicationFilterValue"
          class="artifact-toolbar-select artifact-filter"
          show-search
          option-filter-prop="label"
          placeholder="应用 · 全部"
          :loading="applicationLoading"
          :options="applicationFilterOptions"
        />
        <a-select
          v-model:value="scopeFilterValue"
          class="artifact-toolbar-select artifact-scope-filter"
          placeholder="执行单元 · 全部"
          :options="scopeOptions"
        />
        <a-button class="artifact-toolbar-query-btn" :disabled="!filters.repository_id" @click="searchArtifacts">
          <template #icon><SearchOutlined /></template>
          查询
        </a-button>
        <a-button v-if="canManualAddArtifact" class="artifact-toolbar-add-btn" @click="openManualArtifactModal">
          <template #icon><PlusOutlined /></template>
          手动添加
        </a-button>
      </div>
    </div>

    <section class="artifact-window">
      <div class="artifact-window-toolbar">
        <div class="artifact-nav-tools">
          <button class="artifact-tool-icon" type="button" aria-label="后退" disabled>
            <LeftOutlined />
          </button>
          <button class="artifact-tool-icon" type="button" aria-label="前进" disabled>
            <RightOutlined />
          </button>
          <button class="artifact-tool-icon" type="button" aria-label="根目录" @click="resetExplorerLocation">
            <HomeOutlined />
          </button>
          <button class="artifact-tool-icon" type="button" aria-label="刷新" @click="loadArtifacts">
            <ReloadOutlined />
          </button>
        </div>
      </div>

      <div v-if="canQueryArtifacts" class="artifact-browser">
        <aside class="artifact-tree-panel">
          <div class="artifact-sidebar-title">
            <FolderOpenOutlined />
            <span>位置</span>
          </div>
          <a-tree
            v-if="artifactTreeData.length > 0"
            class="artifact-tree"
            block-node
            default-expand-all
            :tree-data="artifactTreeData"
            :selected-keys="selectedTreeKeys"
            @select="handleTreeSelect"
          >
            <template #title="{ title, dataRef }">
              <span class="artifact-tree-title">
                <component :is="treeNodeIcon(dataRef.type)" />
                <span>{{ title }}</span>
              </span>
            </template>
          </a-tree>
          <a-empty v-else class="artifact-empty" description="暂无目录" />
        </aside>

        <main class="artifact-file-panel">
          <div class="artifact-file-panel-header">
            <div class="artifact-file-panel-title">
              <FolderOpenOutlined />
              <span>目录</span>
              <strong>{{ explorerPanelTitle }}</strong>
            </div>
          </div>
          <div class="artifact-file-grid" :class="{ 'artifact-file-grid-loading': loading }">
            <button
              v-for="directory in explorerDirectories"
              :key="directory.key"
              class="artifact-directory-tile"
              type="button"
              @click="openExplorerDirectory(directory)"
            >
              <span class="artifact-folder-glyph" />
              <span class="artifact-tile-name">{{ directory.title }}</span>
            </button>

            <div
              v-for="record in explorerFiles"
              :key="record.id"
              class="artifact-file-tile"
              :class="{ 'artifact-file-tile-actions-visible': visibleFileActionID === record.id }"
              role="button"
              tabindex="0"
              @contextmenu.prevent.stop="showFileActions(record)"
              @click="openDetail(record)"
              @keyup.enter="openDetail(record)"
            >
              <div class="artifact-file-identity">
                <div class="artifact-file-glyph">
                  <FileOutlined />
                </div>
                <div class="artifact-tile-name">{{ record.artifact_name || '-' }}</div>
              </div>
              <div class="artifact-file-actions" @click.stop>
                <a-button class="artifact-file-action-btn" size="small" @click.stop="downloadArtifact(record, $event)">
                  <template #icon><DownloadOutlined /></template>
                  下载
                </a-button>
                <a-button v-if="isManualArtifact(record)" class="artifact-file-action-btn" danger size="small" @click.stop="deleteManualArtifact(record)">
                  <template #icon><DeleteOutlined /></template>
                  删除
                </a-button>
              </div>
            </div>

            <a-empty
              v-if="!loading && explorerDirectories.length === 0 && explorerFiles.length === 0"
              class="artifact-empty artifact-grid-empty"
              :description="emptyText"
            />
          </div>
          <div class="artifact-explorer-status">
            <span>{{ explorerStatusText }}</span>
            <a-pagination
              size="small"
              :current="pagination.page"
              :page-size="pagination.page_size"
              :total="pagination.total"
              show-size-changer
              :page-size-options="['20', '50', '100']"
              :show-total="showPaginationTotal"
              @change="handleGridPageChange"
              @show-size-change="handleGridPageChange"
            />
          </div>
        </main>
      </div>
      <div v-else class="artifact-repository-required">
        <FolderOpenOutlined />
        <div>
          <div class="artifact-required-title">请选择制品库</div>
          <div class="artifact-required-desc">选择制品库后，再按项目、应用和执行单元查看对应制品目录。</div>
        </div>
      </div>
    </section>

    <a-drawer
      title="制品详情"
      width="560"
      :open="detailVisible"
      @close="closeDetail"
    >
      <a-descriptions v-if="selectedArtifact" :column="1" bordered size="small">
        <a-descriptions-item label="文件名">{{ selectedArtifact.artifact_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="发布">
          <button
            class="artifact-release-link"
            type="button"
            :disabled="!selectedArtifact.release_order_id"
            @click="viewReleaseOrder(selectedArtifact)"
          >
            {{ selectedArtifact.release_display_name || buildReleaseDisplayName(selectedArtifact) }}
          </button>
        </a-descriptions-item>
        <a-descriptions-item label="项目">{{ selectedArtifact.project_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用">{{ selectedArtifact.application_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="环境">{{ selectedArtifact.env_code || '-' }}</a-descriptions-item>
        <a-descriptions-item label="执行单元">{{ formatPipelineScope(selectedArtifact.pipeline_scope) }}</a-descriptions-item>
        <a-descriptions-item label="制品库">{{ selectedArtifact.repository_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Bucket">{{ selectedArtifact.bucket || '-' }}</a-descriptions-item>
        <a-descriptions-item label="制品库 ID">{{ selectedArtifact.repository_id || '-' }}</a-descriptions-item>
        <a-descriptions-item label="校验">
          {{ formatValue(selectedArtifact.checksum_type) }} {{ formatValue(selectedArtifact.checksum) }}
        </a-descriptions-item>
        <a-descriptions-item label="记录时间">{{ formatDate(selectedArtifact.created_at) }}</a-descriptions-item>
        <a-descriptions-item label="下载地址">
          <div class="artifact-url">{{ selectedArtifact.artifact_url || '-' }}</div>
        </a-descriptions-item>
      </a-descriptions>
    </a-drawer>

    <a-modal
      :open="manualArtifactVisible"
      :footer="null"
      :width="760"
      :closable="false"
      :destroy-on-close="true"
      :mask-style="manualArtifactMaskStyle"
      :wrap-props="manualArtifactWrapProps"
      wrap-class-name="manual-artifact-modal-wrap"
      @cancel="closeManualArtifactModal"
    >
      <template #title>
        <div class="manual-artifact-modal-titlebar">
          <span class="manual-artifact-modal-title">手动添加文件</span>
          <a-button class="manual-artifact-modal-save-btn" :loading="manualArtifactSubmitting" @click="submitManualArtifact">保存</a-button>
        </div>
      </template>

      <a-form :model="manualArtifactForm" layout="vertical" :required-mark="false" class="manual-artifact-form">
        <div class="manual-artifact-form-note">手动添加的文件会记录到当前发布单目录，不影响发布过程自动产出的制品</div>

        <div class="manual-artifact-context-card">
          <div class="manual-artifact-section-title">当前目录</div>
          <div class="manual-artifact-context-grid">
            <div class="manual-artifact-context-item">
              <div class="manual-artifact-context-label">发布目录</div>
              <div class="manual-artifact-context-value">{{ manualArtifactContextText }}</div>
            </div>
          </div>
        </div>

        <section class="manual-artifact-form-section">
          <div class="manual-artifact-section-title">基础信息</div>
          <div class="manual-artifact-form-grid">
            <a-form-item label="文件名" required>
              <a-input v-model:value="manualArtifactForm.artifact_name" allow-clear placeholder="请输入文件名" />
            </a-form-item>
            <a-form-item label="下载地址" required>
              <a-input v-model:value="manualArtifactForm.artifact_url" allow-clear placeholder="请输入下载地址" />
            </a-form-item>
            <a-form-item label="执行单元">
              <a-select v-model:value="manualArtifactForm.pipeline_scope" :options="pipelineScopeOptions" />
            </a-form-item>
            <a-form-item label="文件类型">
              <a-input v-model:value="manualArtifactForm.artifact_type" allow-clear placeholder="例如 zip / jar" />
            </a-form-item>
          </div>
        </section>

        <section class="manual-artifact-form-section">
          <div class="manual-artifact-section-title">扩展信息</div>
          <div class="manual-artifact-form-grid">
            <a-form-item label="版本">
              <a-input v-model:value="manualArtifactForm.artifact_version" allow-clear placeholder="请输入版本" />
            </a-form-item>
            <a-form-item label="构建号">
              <a-input v-model:value="manualArtifactForm.build_number" allow-clear placeholder="请输入构建号" />
            </a-form-item>
            <a-form-item label="对象路径">
              <a-input v-model:value="manualArtifactForm.object_key" allow-clear placeholder="请输入对象路径" />
            </a-form-item>
            <a-form-item label="文件大小">
              <a-input-number v-model:value="manualArtifactForm.size_bytes" :min="0" style="width: 100%" placeholder="字节数" />
            </a-form-item>
            <a-form-item label="校验类型">
              <a-input v-model:value="manualArtifactForm.checksum_type" allow-clear placeholder="例如 sha256" />
            </a-form-item>
            <a-form-item label="校验值">
              <a-input v-model:value="manualArtifactForm.checksum" allow-clear placeholder="请输入校验值" />
            </a-form-item>
          </div>
        </section>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.artifact-center-page {
  min-height: 100%;
  padding: 0;
  color: #1f2937;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 20px;
  margin-bottom: 18px;
}

.page-title {
  color: #0f172a;
  font-size: 24px;
  font-weight: 700;
  line-height: 1.25;
}

.artifact-window {
  overflow: hidden;
  min-height: calc(100vh - 168px);
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.72);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 18px 44px rgba(15, 23, 42, 0.06);
  backdrop-filter: blur(14px) saturate(135%);
  -webkit-backdrop-filter: blur(14px) saturate(135%);
}

.artifact-window-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 58px;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.86);
  background: rgba(248, 250, 252, 0.72);
}

.artifact-nav-tools {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
}

.artifact-tool-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  padding: 0;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.52);
  color: #475569;
  cursor: pointer;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 8px 18px rgba(15, 23, 42, 0.04);
}

.artifact-tool-icon:not(:disabled):hover {
  border-color: rgba(96, 165, 250, 0.34);
  background: rgba(255, 255, 255, 0.78);
  color: #2563eb;
}

.artifact-tool-icon:disabled {
  background: rgba(241, 245, 249, 0.46);
  color: rgba(100, 116, 139, 0.42);
  cursor: not-allowed;
  box-shadow: none;
}

.artifact-center-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1 1 auto;
  min-width: 0;
}

.artifact-repository-filter {
  width: 220px;
}

.artifact-filter {
  width: 150px;
}

.artifact-scope-filter {
  width: 140px;
}

:deep(.artifact-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.artifact-toolbar-select.ant-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  padding: 0 14px !important;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  border-radius: 16px !important;
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 22px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.artifact-toolbar-select.ant-select .ant-select-selection-search-input) {
  height: 40px !important;
}

:deep(.artifact-toolbar-select.ant-select .ant-select-selection-placeholder),
:deep(.artifact-toolbar-select.ant-select .ant-select-selection-item) {
  color: #334155;
  font-size: 13px;
  font-weight: 600;
}

:deep(.artifact-toolbar-select.ant-select-focused .ant-select-selector),
:deep(.artifact-toolbar-select.ant-select-open .ant-select-selector),
:deep(.artifact-toolbar-select.ant-select:hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.38) !important;
  background: rgba(255, 255, 255, 0.88) !important;
}

:deep(.artifact-toolbar-query-btn.ant-btn) {
  height: 42px;
  min-width: 82px;
  padding: 0 16px;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a !important;
  font-size: 13px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 22px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.artifact-toolbar-query-btn.ant-btn:not(:disabled):hover),
:deep(.artifact-toolbar-query-btn.ant-btn:not(:disabled):focus) {
  border-color: rgba(96, 165, 250, 0.38) !important;
  background: rgba(255, 255, 255, 0.88) !important;
  color: #2563eb !important;
}

:deep(.artifact-toolbar-query-btn.ant-btn:disabled) {
  border-color: rgba(148, 163, 184, 0.24) !important;
  background: rgba(241, 245, 249, 0.5) !important;
  color: rgba(100, 116, 139, 0.72) !important;
  box-shadow: none;
}

:deep(.artifact-toolbar-add-btn.ant-btn) {
  height: 42px;
  padding: 0 16px;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a !important;
  font-size: 13px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 22px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.artifact-toolbar-add-btn.ant-btn:not(:disabled):hover),
:deep(.artifact-toolbar-add-btn.ant-btn:not(:disabled):focus) {
  border-color: rgba(96, 165, 250, 0.38) !important;
  background: rgba(255, 255, 255, 0.88) !important;
  color: #2563eb !important;
}

.artifact-browser {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  min-height: calc(100vh - 212px);
  background: #fff;
}

.artifact-tree-panel {
  min-width: 0;
  max-height: calc(100vh - 212px);
  overflow: auto;
  border-right: 1px solid rgba(226, 232, 240, 0.86);
  background: linear-gradient(180deg, #f8fafc, #fff);
  box-shadow: none;
  color: #0f172a;
}

.artifact-file-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 0;
  background: #fff;
}

.artifact-file-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 44px;
  padding: 0 22px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.86);
  background: rgba(248, 250, 252, 0.72);
}

.artifact-file-panel-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #475569;
  font-size: 14px;
  font-weight: 700;
  line-height: 20px;
}

.artifact-file-panel-title svg {
  flex: 0 0 auto;
  color: #2563eb;
  font-size: 16px;
}

.artifact-file-panel-title strong {
  overflow: hidden;
  max-width: min(560px, 52vw);
  color: #0f172a;
  font-size: 15px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.artifact-sidebar-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 16px 16px 12px;
  color: #0f172a;
  font-size: 15px;
  font-weight: 700;
}

:deep(.artifact-tree.ant-tree) {
  padding: 0 8px 16px;
  background: transparent;
  color: #334155;
}

:deep(.artifact-tree.ant-tree .ant-tree-switcher),
:deep(.artifact-tree.ant-tree .ant-tree-node-content-wrapper) {
  color: #475569;
}

:deep(.artifact-tree.ant-tree .ant-tree-node-content-wrapper:hover),
:deep(.artifact-tree.ant-tree .ant-tree-node-selected) {
  background: rgba(219, 234, 254, 0.72) !important;
  color: #1d4ed8;
}

.artifact-tree-title {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  color: inherit;
}

.artifact-tree-title svg {
  flex: 0 0 auto;
  color: #2563eb;
}

.artifact-empty {
  margin: 80px 0;
}

.artifact-url {
  max-width: 100%;
  overflow-wrap: anywhere;
  color: #2563eb;
}

.artifact-release-link {
  display: inline;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: #2563eb;
  font: inherit;
  font-weight: 700;
  line-height: inherit;
  text-align: left;
  overflow-wrap: anywhere;
  cursor: pointer;
}

.artifact-release-link:hover,
.artifact-release-link:focus-visible {
  color: #1d4ed8;
  text-decoration: underline;
  outline: none;
}

.artifact-release-link:disabled {
  color: #64748b;
  text-decoration: none;
  cursor: default;
}

.artifact-file-grid {
  display: grid;
  align-content: start;
  grid-template-columns: repeat(auto-fill, minmax(132px, 1fr));
  gap: 22px 24px;
  min-height: 480px;
  padding: 26px 28px 24px;
  overflow: auto;
  background: #fff;
}

.artifact-file-grid-loading {
  opacity: 0.62;
  pointer-events: none;
}

.artifact-directory-tile,
.artifact-file-tile {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  align-self: start;
  min-width: 0;
  min-height: 132px;
  padding: 14px 10px 12px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 16px;
  background: #fff;
  color: #334155;
  text-align: center;
  cursor: pointer;
  box-shadow: 0 10px 22px rgba(15, 23, 42, 0.04);
  transition:
    border-color 0.16s ease,
    background 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
}

.artifact-directory-tile:hover,
.artifact-file-tile:hover,
.artifact-file-tile:focus {
  border-color: rgba(96, 165, 250, 0.36);
  background: #f8fbff;
  box-shadow: 0 14px 30px rgba(37, 99, 235, 0.08);
  transform: translateY(-1px);
  outline: none;
}

.artifact-folder-glyph {
  position: relative;
  display: block;
  width: 66px;
  height: 46px;
  margin-bottom: 13px;
  border-radius: 10px 10px 9px 9px;
  background: linear-gradient(180deg, #93c5fd 0%, #3b82f6 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.48),
    0 10px 18px rgba(37, 99, 235, 0.16);
}

.artifact-folder-glyph::before {
  position: absolute;
  top: -7px;
  left: 0;
  width: 32px;
  height: 13px;
  border-radius: 9px 9px 0 0;
  background: #bfdbfe;
  content: '';
}

.artifact-file-glyph {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 54px;
  height: 64px;
  margin-bottom: 12px;
  border: 1px solid rgba(191, 219, 254, 0.9);
  border-radius: 12px;
  background: linear-gradient(180deg, #eff6ff, #dbeafe);
  color: #2563eb;
  font-size: 26px;
  box-shadow: 0 10px 18px rgba(37, 99, 235, 0.1);
}

.artifact-file-glyph::after {
  position: absolute;
  top: 0;
  right: 0;
  width: 0;
  height: 0;
  border-top: 14px solid #bfdbfe;
  border-left: 14px solid rgba(255, 255, 255, 0.96);
  content: '';
}

.artifact-file-identity {
  display: flex;
  align-items: center;
  flex-direction: column;
  width: 100%;
}

.artifact-file-tile-actions-visible .artifact-file-identity {
  display: none;
}

.artifact-tile-name {
  display: -webkit-box;
  overflow: hidden;
  width: 100%;
  color: #0f172a;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  text-overflow: ellipsis;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow-wrap: anywhere;
}

.artifact-file-actions {
  display: none;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  min-height: 96px;
  pointer-events: none;
}

.artifact-file-tile-actions-visible .artifact-file-actions {
  display: flex;
  pointer-events: auto;
}

:deep(.artifact-file-action-btn.ant-btn) {
  min-width: 88px;
  height: 32px;
  border-radius: 10px;
  background: rgba(248, 250, 252, 0.94);
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

:deep(.artifact-file-action-btn.ant-btn-dangerous) {
  border-color: rgba(248, 113, 113, 0.45);
  background: rgba(254, 242, 242, 0.96);
  color: #dc2626 !important;
}

:deep(.artifact-file-action-btn.ant-btn-dangerous:hover),
:deep(.artifact-file-action-btn.ant-btn-dangerous:focus) {
  border-color: rgba(239, 68, 68, 0.5);
  background: rgba(254, 226, 226, 0.98);
  color: #b91c1c !important;
}

.artifact-grid-empty {
  grid-column: 1 / -1;
}

.artifact-explorer-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 52px;
  padding: 8px 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.86);
  background: rgba(248, 250, 252, 0.86);
  color: #475569;
  font-size: 13px;
}

.artifact-repository-required {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 320px;
  padding: 24px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.34);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.62),
    0 14px 32px rgba(15, 23, 42, 0.04);
  color: #475569;
  backdrop-filter: blur(14px) saturate(135%);
  -webkit-backdrop-filter: blur(14px) saturate(135%);
}

.artifact-repository-required svg {
  color: #2563eb;
  font-size: 26px;
}

.artifact-required-title {
  color: #0f172a;
  font-size: 16px;
  font-weight: 700;
  line-height: 24px;
}

.artifact-required-desc {
  margin-top: 4px;
  color: #64748b;
  font-size: 13px;
  line-height: 20px;
}

:global(.manual-artifact-modal-wrap .ant-modal-content) {
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
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

:global(.manual-artifact-modal-wrap .ant-modal-header) {
  margin-bottom: 0;
  border-bottom: none;
  background: transparent;
}

:global(.manual-artifact-modal-wrap .ant-modal-title) {
  color: #0f172a;
}

.manual-artifact-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.manual-artifact-modal-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 800;
}

.manual-artifact-modal-save-btn.ant-btn {
  flex: none;
  height: 34px;
  padding: 0 16px;
  border: 1px solid rgba(96, 165, 250, 0.42) !important;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a !important;
  font-size: 13px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 22px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
  -webkit-backdrop-filter: blur(14px) saturate(135%);
}

.manual-artifact-modal-save-btn.ant-btn:not(:disabled):hover,
.manual-artifact-modal-save-btn.ant-btn:not(:disabled):focus {
  border-color: rgba(37, 99, 235, 0.54) !important;
  background: rgba(255, 255, 255, 0.88) !important;
  color: #2563eb !important;
}

.manual-artifact-form {
  padding-top: 18px;
}

.manual-artifact-form-note {
  position: relative;
  margin-bottom: 18px;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.manual-artifact-form-note::before {
  position: absolute;
  top: 3px;
  bottom: 3px;
  left: 0;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
  content: '';
}

.manual-artifact-context-card + .manual-artifact-form-section,
.manual-artifact-form-section + .manual-artifact-form-section {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.manual-artifact-context-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
  margin-bottom: 16px;
}

.manual-artifact-context-item {
  min-width: 0;
  padding-bottom: 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.manual-artifact-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.manual-artifact-context-value {
  overflow: hidden;
  min-width: 0;
  color: #0f172a;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.6;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.manual-artifact-section-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
}

.manual-artifact-section-title::after {
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  content: '';
  transform: translateY(1px);
}

.manual-artifact-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.manual-artifact-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.manual-artifact-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.manual-artifact-form :deep(.ant-input),
.manual-artifact-form :deep(.ant-input-affix-wrapper),
.manual-artifact-form :deep(.ant-select-selector),
.manual-artifact-form :deep(.ant-input-number),
.manual-artifact-form :deep(.ant-input-number-input) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.manual-artifact-form :deep(.ant-input:hover),
.manual-artifact-form :deep(.ant-input-affix-wrapper:hover),
.manual-artifact-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.manual-artifact-form :deep(.ant-input-number:hover) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.manual-artifact-form :deep(.ant-input:focus),
.manual-artifact-form :deep(.ant-input-focused),
.manual-artifact-form :deep(.ant-input-affix-wrapper-focused),
.manual-artifact-form :deep(.ant-select-focused .ant-select-selector),
.manual-artifact-form :deep(.ant-input-number-focused) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

@media (max-width: 1100px) {
  .page-header {
    align-items: flex-start;
  }

  .artifact-center-toolbar {
    justify-content: flex-start;
    width: 100%;
  }

  .artifact-repository-filter,
  .artifact-filter,
  .artifact-scope-filter,
  :deep(.artifact-toolbar-query-btn.ant-btn),
  :deep(.artifact-toolbar-add-btn.ant-btn) {
    flex: 1 1 180px;
  }

  .artifact-browser {
    grid-template-columns: 1fr;
  }

  .artifact-tree-panel {
    max-height: 280px;
  }

  .artifact-file-grid {
    grid-template-columns: repeat(auto-fill, minmax(112px, 1fr));
    gap: 18px;
    padding: 22px 18px;
  }

  .artifact-explorer-status {
    align-items: flex-start;
    flex-direction: column;
  }

  .manual-artifact-form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
