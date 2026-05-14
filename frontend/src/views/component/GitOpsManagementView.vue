<script setup lang="ts">
import { LeftOutlined, PlusOutlined, QuestionCircleOutlined, RightOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  createGitOpsInstance,
  getGitOpsInstanceStatus,
  listGitOpsInstances,
  updateGitOpsInstance,
} from '../../api/gitops'
import { useAuthStore } from '../../stores/auth'
import type {
  GitOpsInstance,
  GitOpsRecordStatus,
  GitOpsStatus,
  UpsertGitOpsInstancePayload,
} from '../../types/gitops'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

const authStore = useAuthStore()
const router = useRouter()

const loadingInstances = ref(false)
const loadingStatus = ref(false)
const savingInstance = ref(false)
const instanceTotal = ref(0)
const instanceDataSource = ref<GitOpsInstance[]>([])
const selectedInstanceID = ref('')
const selectedInstance = ref<GitOpsInstance | null>(null)
const detail = ref<GitOpsStatus | null>(null)
const instanceModalVisible = ref(false)
const editingInstanceID = ref('')
const searchDialogVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
const instanceModalViewportInset = ref(0)
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0
let instanceModalViewportObserver: ResizeObserver | null = null
let pageAlive = true

const instanceFilters = reactive({
  keyword: '',
  status: '' as GitOpsRecordStatus | '',
  page: 1,
  pageSize: 20,
})

const instanceForm = reactive<UpsertGitOpsInstancePayload>({
  instance_code: '',
  name: '',
  local_root: '',
  default_branch: 'master',
  username: '',
  password: '',
  token: '',
  author_name: 'gos-bot',
  author_email: 'gos@example.com',
  commit_message_template: 'chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})',
  command_timeout_sec: 30,
  status: 'active',
  remark: '',
})

const canManageGitOps = computed(() => authStore.hasPermission('component.gitops.manage'))

const statusFilterValue = computed<GitOpsRecordStatus | ''>({
  get: () => instanceFilters.status,
  set: (value) => {
    instanceFilters.status = value === 'active' || value === 'inactive' ? value : ''
  },
})

const searchDraft = reactive({
  keyword: '',
})

const instanceTotalPages = computed(() => Math.max(1, Math.ceil(instanceTotal.value / Math.max(instanceFilters.pageSize, 1))))

const instanceModalMaskStyle = computed(() => ({
  left: `${instanceModalViewportInset.value}px`,
  width: `calc(100% - ${instanceModalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: instanceModalVisible.value ? 'auto' : 'none',
}))

const instanceModalWrapProps = computed(() => ({
  style: {
    left: `${instanceModalViewportInset.value}px`,
    width: `calc(100% - ${instanceModalViewportInset.value}px)`,
    pointerEvents: instanceModalVisible.value ? 'auto' : 'none',
  },
}))

const pathStateTone = computed(() => {
  if (!detail.value?.enabled) return 'muted'
  return detail.value.path_exists ? 'success' : 'warning'
})
const repoStateTone = computed(() => {
  if (!detail.value?.enabled) return 'muted'
  return detail.value.is_git_repo ? 'success' : 'warning'
})
const remoteStateTone = computed(() => {
  if (!detail.value?.enabled || !detail.value.remote_origin) return 'muted'
  return detail.value.remote_reachable ? 'success' : 'danger'
})
const worktreeStateTone = computed(() => {
  if (!detail.value?.enabled || !detail.value.is_git_repo) return 'muted'
  return detail.value.worktree_dirty ? 'warning' : 'success'
})

function resetInstanceForm() {
  editingInstanceID.value = ''
  instanceForm.instance_code = ''
  instanceForm.name = ''
  instanceForm.local_root = ''
  instanceForm.default_branch = 'master'
  instanceForm.username = ''
  instanceForm.password = ''
  instanceForm.token = ''
  instanceForm.author_name = 'gos-bot'
  instanceForm.author_email = 'gos@example.com'
  instanceForm.commit_message_template = 'chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})'
  instanceForm.command_timeout_sec = 30
  instanceForm.status = 'active'
  instanceForm.remark = ''
}

function readInstanceModalViewportInset() {
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

function syncInstanceModalViewportInset() {
  instanceModalViewportInset.value = readInstanceModalViewportInset()
}

function observeInstanceModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }
  instanceModalViewportObserver?.disconnect()
  instanceModalViewportObserver = new ResizeObserver(syncInstanceModalViewportInset)
  if (appLayout) {
    instanceModalViewportObserver.observe(appLayout)
  }
  if (sider) {
    instanceModalViewportObserver.observe(sider)
  }
}

function stopObservingInstanceModalViewportInset() {
  instanceModalViewportObserver?.disconnect()
  instanceModalViewportObserver = null
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
  searchDraft.keyword = instanceFilters.keyword.trim()
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
    const response = await listGitOpsInstances({
      keyword: normalizedKeyword,
      status: instanceFilters.status || undefined,
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: item.id,
      title: item.name || item.instance_code || item.id,
      subtitle: `${item.instance_code || '-'} · ${item.local_root || '-'}`,
      query: item.name || item.instance_code || normalizedKeyword,
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

function queryInstances() {
  instanceFilters.page = 1
  void loadInstances()
}

function handleSearchSubmit() {
  instanceFilters.keyword = searchDraft.keyword.trim()
  instanceFilters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadInstances()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  instanceFilters.keyword = item.query
  instanceFilters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadInstances()
}

function openCreateInstance() {
  resetInstanceForm()
  instanceModalVisible.value = true
}

function openEditInstance(record: GitOpsInstance) {
  editingInstanceID.value = record.id
  instanceForm.instance_code = record.instance_code
  instanceForm.name = record.name
  instanceForm.local_root = record.local_root
  instanceForm.default_branch = record.default_branch || 'master'
  instanceForm.username = record.username || ''
  instanceForm.password = ''
  instanceForm.token = ''
  instanceForm.author_name = record.author_name || 'gos-bot'
  instanceForm.author_email = record.author_email || 'gos@example.com'
  instanceForm.commit_message_template = record.commit_message_template || 'chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})'
  instanceForm.command_timeout_sec = record.command_timeout_sec || 30
  instanceForm.status = (record.status || 'active') as GitOpsRecordStatus
  instanceForm.remark = record.remark || ''
  instanceModalVisible.value = true
}

function closeInstanceModal() {
  instanceModalVisible.value = false
  resetInstanceForm()
}

async function loadInstances() {
  loadingInstances.value = true
  try {
    const response = await listGitOpsInstances({
      keyword: instanceFilters.keyword.trim() || undefined,
      status: instanceFilters.status || undefined,
      page: instanceFilters.page,
      page_size: instanceFilters.pageSize,
    })
    if (!pageAlive) return
    instanceDataSource.value = response.data
    instanceTotal.value = response.total
    instanceFilters.page = response.page
    instanceFilters.pageSize = response.page_size

    const current = selectedInstanceID.value
    const matched = response.data.find((item) => item.id === current)
    selectedInstance.value = matched || null
    selectedInstanceID.value = matched?.id || ''
    if (matched?.id) {
      await loadStatus(matched.id)
    } else {
      detail.value = null
    }
  } catch (error) {
    if (pageAlive) {
      message.error(extractHTTPErrorMessage(error, 'GitOps 实例列表加载失败'))
    }
  } finally {
    if (pageAlive) {
      loadingInstances.value = false
    }
  }
}

async function loadStatus(id?: string) {
  const instanceID = String(id || selectedInstanceID.value || '').trim()
  if (!instanceID) {
    detail.value = null
    return
  }
  loadingStatus.value = true
  try {
    const response = await getGitOpsInstanceStatus(instanceID)
    if (!pageAlive) return
    selectedInstance.value = response.data.instance
    selectedInstanceID.value = response.data.instance.id
    detail.value = response.data.status
  } catch (error) {
    if (pageAlive) {
      message.error(extractHTTPErrorMessage(error, 'GitOps 状态加载失败'))
    }
  } finally {
    if (pageAlive) {
      loadingStatus.value = false
    }
  }
}

async function handleSaveInstance() {
  savingInstance.value = true
  try {
    if (editingInstanceID.value) {
      await updateGitOpsInstance(editingInstanceID.value, instanceForm)
      message.success('GitOps 实例已更新')
    } else {
      await createGitOpsInstance(instanceForm)
      message.success('GitOps 实例已创建')
    }
    closeInstanceModal()
    await loadInstances()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editingInstanceID.value ? 'GitOps 实例更新失败' : 'GitOps 实例创建失败'))
  } finally {
    savingInstance.value = false
  }
}

function handleSelectInstance(record: GitOpsInstance) {
  selectedInstanceID.value = record.id
  selectedInstance.value = record
  void loadStatus(record.id)
}

function openTutorial() {
  void router.push('/help/gitops')
}

function handleStatusInstanceChange(value?: string) {
  const instanceID = String(value || '').trim()
  selectedInstanceID.value = instanceID
  detail.value = null
  const matched = instanceDataSource.value.find((item) => item.id === instanceID) || null
  selectedInstance.value = matched
  if (matched?.id) {
    void loadStatus(matched.id)
  }
}

function changeInstancePage(page: number) {
  const nextPage = Math.min(Math.max(page, 1), instanceTotalPages.value)
  if (nextPage === instanceFilters.page) {
    return
  }
  instanceFilters.page = nextPage
  void loadInstances()
}

async function copyValue(value: string, label: string) {
  const text = String(value || '').trim()
  if (!text) {
    message.warning(`${label}为空，无法复制`)
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    message.success(`${label}已复制`)
  } catch {
    message.error(`${label}复制失败`)
  }
}

onMounted(() => {
  pageAlive = true
  syncInstanceModalViewportInset()
  observeInstanceModalViewportInset()
  void loadInstances()
})

onUnmounted(() => {
  pageAlive = false
  clearSearchSuggestions()
  stopObservingInstanceModalViewportInset()
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
  <div class="page-wrap">
    <div class="page-header">
      <div class="page-header-copy">
        <div class="page-title">仓配</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" title="使用教程" @click="openTutorial">
          <template #icon><QuestionCircleOutlined /></template>
        </a-button>
        <a-button class="application-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon><SearchOutlined /></template>
        </a-button>
        <a-select
          v-model:value="statusFilterValue"
          class="component-toolbar-select"
          :options="[
            { label: '状态 · 全部', value: '' },
            { label: '状态 · active', value: 'active' },
            { label: '状态 · inactive', value: 'inactive' },
          ]"
        />
        <a-button class="component-toolbar-query-btn" @click="queryInstances">查询</a-button>
        <a-button v-if="canManageGitOps" class="application-toolbar-action-btn" @click="openCreateInstance">
          <template #icon><PlusOutlined /></template>
          新增实例
        </a-button>
      </div>
    </div>

    <transition name="component-search-fade">
      <div v-if="searchDialogVisible" class="component-search-overlay" @click.self="closeSearchDialog">
        <div class="component-search-floating-panel">
          <div class="component-search-floating-input">
            <SearchOutlined class="component-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="component-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="实例编码 / 名称 / 工作目录"
              @keydown.enter="handleSearchSubmit"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="component-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="component-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="component-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="component-search-suggestion-title">{{ item.title }}</span>
                <span class="component-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <div class="gitops-unified-layout">
      <section class="gitops-module gitops-module--instances">
        <div class="gitops-module-header">
          <div>
            <div class="gitops-module-kicker">01 · 实例管理</div>
            <h3 class="gitops-module-title">GitOps 实例</h3>
          </div>
          <div class="gitops-module-meta">共 {{ instanceTotal }} 个实例</div>
        </div>
        <a-card :bordered="false" class="table-card gitops-list-card">
          <a-spin :spinning="loadingInstances">
            <div class="gitops-resource-list gitops-instance-list">
              <article
                v-for="record in instanceDataSource"
                :key="record.id"
                class="gitops-resource-card"
                @click="handleSelectInstance(record)"
              >
                <div class="gitops-resource-card-head">
                  <div class="gitops-resource-identity">
                    <div class="gitops-resource-title-row">
                      <span class="gitops-resource-title">{{ record.name || '-' }}</span>
                      <a-tag :color="record.status === 'active' ? 'green' : 'default'">{{ record.status }}</a-tag>
                    </div>
                    <div class="gitops-resource-subtitle">{{ record.instance_code || '-' }}</div>
                  </div>
                  <div v-if="canManageGitOps" class="gitops-resource-actions">
                    <a-button class="component-row-action-btn" size="small" @click.stop="openEditInstance(record)">编辑</a-button>
                  </div>
                </div>
                <div class="gitops-resource-grid">
                  <div class="gitops-resource-field gitops-resource-field--wide">
                    <span>工作目录</span>
                    <strong class="truncate-text" :title="record.local_root">{{ record.local_root || '-' }}</strong>
                  </div>
                  <div class="gitops-resource-field">
                    <span>默认分支</span>
                    <strong>{{ record.default_branch || '-' }}</strong>
                  </div>
                  <div class="gitops-resource-field">
                    <span>提交身份</span>
                    <strong>{{ record.author_name || '-' }}</strong>
                  </div>
                  <div class="gitops-resource-field">
                    <span>提交邮箱</span>
                    <strong class="truncate-text" :title="record.author_email">{{ record.author_email || '-' }}</strong>
                  </div>
                </div>
              </article>
              <a-empty
                v-if="!loadingInstances && instanceDataSource.length === 0"
                class="gitops-empty"
                :description="instanceFilters.keyword.trim() ? '未找到匹配的 GitOps 实例' : '暂无 GitOps 实例'"
              />
            </div>
          </a-spin>

          <div v-if="instanceTotal > instanceFilters.pageSize" class="gitops-compact-pager">
            <span class="gitops-page-summary">第 {{ instanceFilters.page }} / {{ instanceTotalPages }} 页</span>
            <a-button class="gitops-pager-btn" :disabled="instanceFilters.page <= 1" @click="changeInstancePage(instanceFilters.page - 1)">
              <template #icon><LeftOutlined /></template>
            </a-button>
            <a-button class="gitops-pager-btn" :disabled="instanceFilters.page >= instanceTotalPages" @click="changeInstancePage(instanceFilters.page + 1)">
              <template #icon><RightOutlined /></template>
            </a-button>
          </div>
        </a-card>
      </section>

      <section class="gitops-module gitops-module--status">
        <div class="gitops-module-header gitops-module-header--status">
          <div>
            <div class="gitops-module-kicker">02 · 仓库状态</div>
            <h3 class="gitops-module-title">{{ selectedInstance?.name || '选择 GitOps 实例' }}</h3>
          </div>
          <div class="gitops-status-controls">
            <a-select
              class="gitops-status-instance-picker"
              :value="selectedInstanceID || undefined"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="选择 GitOps 实例"
              @change="handleStatusInstanceChange"
            >
              <a-select-option
                v-for="item in instanceDataSource"
                :key="item.id"
                :value="item.id"
                :label="`${item.name} ${item.instance_code} ${item.local_root}`"
              >
                {{ item.name }}<span v-if="item.instance_code"> · {{ item.instance_code }}</span>
              </a-select-option>
            </a-select>
          </div>
        </div>
        <a-card :bordered="false" class="table-card gitops-detail-card-shell">
          <a-empty v-if="!selectedInstance" class="gitops-empty" description="请选择 GitOps 实例后查看仓库状态" />
          <a-spin v-else :spinning="loadingStatus">
            <div class="gitops-detail-panel">
              <article class="gitops-detail-card gitops-detail-card--wide">
                <div class="gitops-detail-card-head">
                  <span>基础信息</span>
                  <a-button class="component-row-action-btn" size="small" @click="copyValue(detail?.local_root || selectedInstance.local_root || '', '工作目录')">复制路径</a-button>
                </div>
                <div class="gitops-detail-grid">
                  <div class="gitops-detail-field">
                    <span>实例编码</span>
                    <strong>{{ selectedInstance.instance_code || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field gitops-detail-field--wide">
                    <span>工作目录</span>
                    <strong class="truncate-text" :title="detail?.local_root || selectedInstance.local_root">{{ detail?.local_root || selectedInstance.local_root || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field">
                    <span>默认分支</span>
                    <strong>{{ detail?.default_branch || selectedInstance.default_branch || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field">
                    <span>提交身份</span>
                    <strong>{{ detail?.author_name || selectedInstance.author_name || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field">
                    <span>提交邮箱</span>
                    <strong class="truncate-text" :title="detail?.author_email || selectedInstance.author_email">{{ detail?.author_email || selectedInstance.author_email || '-' }}</strong>
                  </div>
                </div>
                <div class="template-block">{{ detail?.commit_message_template || selectedInstance.commit_message_template || '-' }}</div>
              </article>

              <article class="gitops-health-panel">
                <div class="gitops-health-heading">
                  <div>
                    <span class="gitops-health-label">仓库状态</span>
                    <strong>{{ detail?.remote_origin ? (detail?.remote_reachable ? '远端可达' : '远端不可达') : '远端未配置' }}</strong>
                  </div>
                  <span class="gitops-state-token" :class="`gitops-state-token--${remoteStateTone}`">
                    {{ detail?.remote_origin ? (detail?.remote_reachable ? '远端可达' : '远端不可达') : '远端未配置' }}
                  </span>
                </div>

                <div class="gitops-health-strip">
                  <div class="gitops-health-pill">
                    <span class="gitops-health-label">路径状态</span>
                    <strong>{{ detail?.path_exists ? '路径可用' : '路径不存在' }}</strong>
                    <span class="gitops-state-token" :class="`gitops-state-token--${pathStateTone}`">{{ detail?.path_exists ? 'ok' : 'missing' }}</span>
                  </div>
                  <div class="gitops-health-pill">
                    <span class="gitops-health-label">仓库状态</span>
                    <strong>{{ detail?.is_git_repo ? '已识别 Git 仓库' : '未识别 Git 仓库' }}</strong>
                    <span class="gitops-state-token" :class="`gitops-state-token--${repoStateTone}`">{{ detail?.mode === 'direct_repo' ? '直接仓库模式' : '工作根目录模式' }}</span>
                  </div>
                  <div class="gitops-health-pill">
                    <span class="gitops-health-label">工作区状态</span>
                    <strong>{{ detail?.worktree_dirty ? '存在未提交变更' : '工作区干净' }}</strong>
                    <span class="gitops-state-token" :class="`gitops-state-token--${worktreeStateTone}`">{{ detail?.worktree_dirty ? 'dirty' : 'clean' }}</span>
                  </div>
                </div>

                <div class="gitops-repo-grid">
                  <div class="gitops-detail-field gitops-detail-field--wide">
                    <span>远端仓库</span>
                    <strong class="truncate-text" :title="detail?.remote_origin">{{ detail?.remote_origin || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field">
                    <span>当前分支</span>
                    <strong>{{ detail?.current_branch || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field">
                    <span>最新提交</span>
                    <strong>{{ detail?.head_commit_short || '-' }}</strong>
                  </div>
                  <div class="gitops-detail-field gitops-detail-field--wide">
                    <span>提交说明</span>
                    <strong>{{ detail?.head_commit_subject || '-' }}</strong>
                  </div>
                </div>
              </article>

              <article class="gitops-detail-card">
                <div class="gitops-detail-card-head">
                  <span>工作区变化</span>
                </div>
                <a-empty v-if="!detail?.status_summary?.length" class="gitops-empty" description="当前没有未提交的工作区变化" />
                <pre v-else class="status-panel">{{ detail.status_summary.join('\n') }}</pre>
              </article>
            </div>
          </a-spin>
        </a-card>
      </section>
    </div>

    <a-modal
      :open="instanceModalVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="instanceModalMaskStyle"
      :wrap-props="instanceModalWrapProps"
      wrap-class-name="component-instance-modal-wrap gitops-instance-modal-wrap"
      @cancel="closeInstanceModal"
    >
      <template #title>
        <div class="component-instance-modal-titlebar">
          <span class="component-instance-modal-title">{{ editingInstanceID ? '编辑 GitOps 实例' : '新增 GitOps 实例' }}</span>
          <a-button class="application-toolbar-action-btn component-instance-modal-save-btn" :loading="savingInstance" @click="handleSaveInstance">
            保存
          </a-button>
        </div>
      </template>
      <a-form layout="vertical" :required-mark="false" class="component-instance-form">
        <div class="component-instance-form-note">
          实例编码用于系统识别，编辑态保持只读；凭据留空时沿用原值
        </div>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="实例编码">
              <a-input v-model:value="instanceForm.instance_code" :disabled="Boolean(editingInstanceID)" placeholder="例如 prod-gitops" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="实例名称">
              <a-input v-model:value="instanceForm.name" placeholder="例如 生产 GitOps 仓库" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="工作目录">
          <a-input v-model:value="instanceForm.local_root" placeholder="例如 /data/gitops/prod" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="默认分支">
              <a-input v-model:value="instanceForm.default_branch" placeholder="master" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="记录状态">
              <a-select v-model:value="instanceForm.status">
                <a-select-option value="active">active</a-select-option>
                <a-select-option value="inactive">inactive</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="Git 用户名">
              <a-input v-model:value="instanceForm.username" placeholder="选填" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item :label="editingInstanceID ? 'Git 密码（留空沿用）' : 'Git 密码'">
              <a-input-password v-model:value="instanceForm.password" placeholder="选填" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item :label="editingInstanceID ? 'Git Token（留空沿用）' : 'Git Token'">
          <a-input-password v-model:value="instanceForm.token" placeholder="选填" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="提交身份">
              <a-input v-model:value="instanceForm.author_name" placeholder="gos-bot" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="提交邮箱">
              <a-input v-model:value="instanceForm.author_email" placeholder="gos@example.com" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="提交模版">
          <a-textarea v-model:value="instanceForm.commit_message_template" :rows="3" />
          <div class="field-help">仅允许使用平台字段字典中的标准字段占位符，例如 `app_key`、`project_name`、`env`、`image_version`、`branch`</div>
        </a-form-item>
        <a-form-item label="命令超时（秒）">
          <a-input-number v-model:value="instanceForm.command_timeout_sec" :min="1" :max="600" style="width: 100%" />
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="instanceForm.remark" :rows="3" />
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
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

.gitops-unified-layout {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.gitops-module {
  padding: 18px;
  border-radius: 24px;
  border: 1px solid rgba(226, 232, 240, 0.8);
  background:
    radial-gradient(circle at right top, rgba(96, 165, 250, 0.09), transparent 30%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.58), rgba(248, 250, 252, 0.36));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 18px 42px rgba(15, 23, 42, 0.05);
}

.gitops-module-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.gitops-module-kicker {
  color: #2563eb;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
}

.gitops-module-title {
  margin: 4px 0 0;
  color: #0f172a;
  font-size: 18px;
  font-weight: 850;
  line-height: 1.3;
}

.gitops-module-meta {
  flex: none;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(255, 255, 255, 0.54);
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.gitops-module-header--status {
  align-items: flex-start;
}

.gitops-status-controls {
  flex: none;
  width: min(360px, 42vw);
}

.gitops-status-instance-picker {
  width: 100%;
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

.gitops-resource-list,
.gitops-detail-panel {
  display: grid;
  gap: 12px;
}

.gitops-resource-card,
.gitops-detail-card,
.gitops-health-panel {
  position: relative;
  overflow: visible;
  border-radius: 18px;
  border: 1px solid rgba(203, 213, 225, 0.74);
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.78), rgba(248, 250, 252, 0.5)),
    radial-gradient(circle at 0 0, rgba(34, 197, 94, 0.08), transparent 32%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.76),
    0 14px 28px rgba(15, 23, 42, 0.05);
}

.gitops-resource-card {
  padding: 16px;
  cursor: pointer;
}

.gitops-resource-card-head,
.gitops-detail-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.gitops-resource-identity {
  min-width: 0;
}

.gitops-resource-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.gitops-resource-title {
  min-width: 0;
  color: #0f172a;
  font-size: 15px;
  font-weight: 850;
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.gitops-resource-subtitle {
  margin-top: 3px;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.gitops-resource-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  flex: none;
}

.gitops-resource-grid,
.gitops-detail-grid {
  display: grid;
  grid-template-columns: minmax(240px, 1.6fr) minmax(120px, 0.7fr) minmax(130px, 0.8fr) minmax(180px, 1fr);
  gap: 10px;
  margin-top: 14px;
}

.gitops-detail-grid {
  grid-template-columns: minmax(120px, 0.75fr) minmax(220px, 1.35fr) minmax(110px, 0.65fr) minmax(120px, 0.75fr) minmax(160px, 1fr);
}

.gitops-detail-grid--repo {
  grid-template-columns: minmax(260px, 1.55fr) minmax(120px, 0.7fr) minmax(120px, 0.7fr);
}

.gitops-resource-field,
.gitops-detail-field {
  min-width: 0;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid rgba(226, 232, 240, 0.76);
  background: rgba(255, 255, 255, 0.5);
}

.gitops-resource-field--wide,
.gitops-detail-field--wide {
  min-width: 0;
}

.gitops-resource-field span,
.gitops-detail-field span,
.gitops-health-label,
.gitops-detail-card-head > span {
  display: block;
  margin-bottom: 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.3;
}

.gitops-detail-card-head > span {
  margin-bottom: 0;
  color: #0f172a;
  font-size: 13px;
}

.gitops-resource-field strong,
.gitops-detail-field strong,
.gitops-health-heading strong,
.gitops-health-pill strong {
  display: block;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 750;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.gitops-detail-card {
  padding: 16px;
}

.gitops-health-panel {
  padding: 16px;
  display: grid;
  grid-template-columns: minmax(260px, 0.82fr) minmax(0, 1.68fr);
  grid-template-areas:
    'heading repo'
    'strip repo';
  gap: 12px 14px;
  align-items: stretch;
}

.gitops-health-heading {
  grid-area: heading;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.gitops-health-heading .gitops-health-label {
  margin-bottom: 0;
  color: #0f172a;
  font-size: 13px;
}

.gitops-health-heading strong {
  margin-top: 4px;
  font-size: 15px;
}

.gitops-health-strip {
  grid-area: strip;
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}

.gitops-health-pill {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 2px 8px;
  min-width: 0;
  padding: 8px 10px;
  border-radius: 14px;
  border: 1px solid rgba(226, 232, 240, 0.76);
  background: rgba(255, 255, 255, 0.5);
}

.gitops-health-pill .gitops-health-label {
  grid-column: 1 / -1;
}

.gitops-health-pill strong {
  font-size: 13px;
}

.gitops-state-token {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  justify-self: start;
  width: fit-content;
  max-width: 100%;
  min-height: 20px;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.82);
  background: rgba(248, 250, 252, 0.82);
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.2;
  white-space: nowrap;
}

.gitops-state-token--success {
  border-color: rgba(74, 222, 128, 0.54);
  background: rgba(220, 252, 231, 0.72);
  color: #15803d;
}

.gitops-state-token--warning {
  border-color: rgba(251, 146, 60, 0.58);
  background: rgba(255, 247, 237, 0.78);
  color: #c2410c;
}

.gitops-state-token--danger {
  border-color: rgba(248, 113, 113, 0.52);
  background: rgba(254, 226, 226, 0.72);
  color: #b91c1c;
}

.gitops-state-token--muted {
  border-color: rgba(203, 213, 225, 0.82);
  background: rgba(248, 250, 252, 0.78);
  color: #64748b;
}

.gitops-repo-grid {
  grid-area: repo;
  display: grid;
  grid-template-columns: minmax(220px, 1.35fr) minmax(120px, 0.7fr) minmax(120px, 0.7fr);
  align-content: stretch;
  gap: 10px;
}

.gitops-repo-grid .gitops-detail-field {
  min-height: 100%;
}

.gitops-repo-grid .gitops-detail-field--wide:last-child {
  grid-column: 1 / -1;
}

.gitops-empty {
  padding: 24px 0;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.application-toolbar-icon-btn.ant-btn),
:deep(.component-toolbar-query-btn.ant-btn) {
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
:deep(.component-toolbar-query-btn.ant-btn) {
  padding-inline: 14px;
  font-weight: 700;
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  width: 42px;
  min-width: 42px;
  padding-inline: 0;
}

:deep(.component-toolbar-select.ant-select) {
  min-width: 138px;
}

:deep(.component-toolbar-select.ant-select .ant-select-selector) {
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

:deep(.component-toolbar-select.ant-select .ant-select-selection-item),
:deep(.component-toolbar-select.ant-select .ant-select-arrow) {
  color: #0f172a !important;
  font-weight: 700;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.component-toolbar-query-btn.ant-btn:hover),
:deep(.component-toolbar-query-btn.ant-btn:focus),
:deep(.component-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.component-row-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
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

.gitops-compact-pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.gitops-page-summary {
  display: inline-flex;
  align-items: center;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.56);
  color: #64748b;
  font-size: 12px;
  font-weight: 750;
  line-height: 1;
}

:deep(.gitops-pager-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.78) !important;
  background: rgba(255, 255, 255, 0.64) !important;
  color: #334155 !important;
  box-shadow: 0 6px 14px rgba(15, 23, 42, 0.04);
}

:deep(.gitops-pager-btn.ant-btn:hover:not(:disabled)),
:deep(.gitops-pager-btn.ant-btn:focus-visible:not(:disabled)) {
  border-color: rgba(37, 99, 235, 0.36) !important;
  color: #1d4ed8 !important;
  box-shadow: 0 10px 20px rgba(37, 99, 235, 0.1);
}

.template-block {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid rgba(226, 232, 240, 0.76);
  background: rgba(255, 255, 255, 0.5);
  color: #0f172a;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  word-break: break-word;
}

.field-help {
  margin-top: 8px;
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.status-panel {
  margin: 0;
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--color-dashboard-900);
  color: var(--color-dashboard-text);
  overflow: auto;
  font-size: 12px;
  line-height: 1.65;
}

.truncate-text {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.component-search-overlay {
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

.component-search-floating-panel {
  width: min(100%, 480px);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.component-search-floating-input {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 0 14px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.74);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.6)),
    rgba(255, 255, 255, 0.44);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 16px 32px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(125%);
}

.component-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.component-search-floating-field {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0;
  border: none;
  outline: none;
  background: transparent;
  color: #0f172a;
  font-size: 13px;
  line-height: 34px;
}

.component-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.component-search-suggestions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-radius: 18px;
  border: 1px solid rgba(255, 255, 255, 0.62);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.52), rgba(255, 255, 255, 0.36)),
    rgba(255, 255, 255, 0.22);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.74),
    0 16px 30px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(124%);
}

.component-search-suggestion,
.component-search-suggestion-loading {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.34);
}

.component-search-suggestion {
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
  transition: background 0.18s ease;
}

.component-search-suggestion:hover {
  background: rgba(255, 255, 255, 0.54);
}

.component-search-suggestion-loading {
  padding: 12px 14px;
  color: rgba(51, 65, 85, 0.76);
  font-size: 12px;
  font-weight: 600;
}

.component-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.component-search-suggestion-subtitle {
  color: rgba(51, 65, 85, 0.78);
  font-size: 12px;
  font-weight: 600;
}

.component-search-fade-enter-active,
.component-search-fade-leave-active {
  transition: opacity 0.18s ease;
}

.component-search-fade-enter-from,
.component-search-fade-leave-to {
  opacity: 0;
}

.component-instance-modal-wrap :deep(.ant-modal) {
  padding-bottom: 32px;
}

.component-instance-modal-wrap :deep(.ant-modal-content) {
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

.component-instance-modal-wrap :deep(.ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

.component-instance-modal-wrap :deep(.ant-modal-body) {
  padding: 20px 28px 28px;
}

.component-instance-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.component-instance-modal-title {
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

:deep(.component-instance-modal-save-btn.ant-btn) {
  flex: none;
  height: 42px;
  padding-inline: 18px;
  border-radius: 16px;
  color: #0f172a !important;
  font-size: 14px;
  font-weight: 700;
}

.component-instance-form-note {
  position: relative;
  margin-bottom: 18px;
  color: rgba(51, 65, 85, 0.88);
  font-size: 13px;
  line-height: 1.7;
}

.component-instance-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.component-instance-form :deep(.ant-input),
.component-instance-form :deep(.ant-input-affix-wrapper),
.component-instance-form :deep(.ant-input-number),
.component-instance-form :deep(.ant-select-selector),
.component-instance-form :deep(.ant-input-textarea textarea) {
  border-color: rgba(203, 213, 225, 0.78);
  background: rgba(255, 255, 255, 0.5);
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

  .component-search-overlay {
    left: 0;
    padding-inline: 16px;
  }

  .gitops-module {
    padding: 14px;
  }

  .gitops-module-header,
  .gitops-resource-card-head,
  .gitops-detail-card-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .gitops-status-controls {
    width: 100%;
  }

  .gitops-resource-actions {
    justify-content: flex-start;
  }

  .gitops-resource-grid,
  .gitops-detail-grid,
  .gitops-detail-grid--repo,
  .gitops-health-strip,
  .gitops-repo-grid {
    grid-template-columns: 1fr;
  }

  .gitops-health-panel {
    grid-template-columns: 1fr;
    grid-template-areas:
      'heading'
      'strip'
      'repo';
  }

  .gitops-repo-grid .gitops-detail-field--wide:last-child {
    grid-column: auto;
  }

  .component-instance-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
