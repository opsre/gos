<script setup lang="ts">
import { CopyOutlined, DeleteOutlined, EditOutlined, PlusOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons-vue'
import { Modal, message } from 'ant-design-vue'
import type { TableColumnsType, UploadProps } from 'ant-design-vue'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { createAgentScript, deleteAgentScript, listAgentScripts, updateAgentScript } from '../../api/agent'
import { useAuthStore } from '../../stores/auth'
import type { AgentScript, AgentScriptListParams, AgentScriptTaskType, UpsertAgentScriptPayload } from '../../types/agent'
import { buildCloneName } from '../../utils/clone-name'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
  query: string
}

const authStore = useAuthStore()
const loading = ref(false)
const saving = ref(false)
const modalVisible = ref(false)
const editingID = ref('')
const cloneSourceID = ref('')
const dataSource = ref<AgentScript[]>([])
const total = ref(0)
const searchDialogVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
const modalViewportInset = ref(0)
let searchSuggestionTimer: ReturnType<typeof window.setTimeout> | null = null
let searchSuggestionRequestSeq = 0
let modalViewportObserver: ResizeObserver | null = null
let pageAlive = true

const filters = reactive<Required<AgentScriptListParams>>({
  keyword: '',
  task_type: '',
  page: 1,
  page_size: 10,
})

const form = reactive<UpsertAgentScriptPayload>({
  name: '',
  description: '',
  task_type: 'shell_task',
  shell_type: 'sh',
  script_path: '',
  script_text: '',
})

const canManage = computed(() => authStore.hasPermission('component.agent.manage'))
const canView = computed(() => canManage.value || authStore.hasPermission('component.agent.view'))

const columns: TableColumnsType<AgentScript> = [
  { title: '脚本名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '类型', dataIndex: 'task_type', key: 'task_type', width: 140 },
  { title: 'Shell', dataIndex: 'shell_type', key: 'shell_type', width: 90 },
  { title: '脚本文件', dataIndex: 'script_path', key: 'script_path', width: 180 },
  { title: '说明', dataIndex: 'description', key: 'description', width: 260, ellipsis: true },
  { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
  { title: '操作', key: 'actions', width: 230, fixed: 'right' },
]

const taskTypeFilterValue = computed<AgentScriptTaskType | ''>({
  get: () => filters.task_type,
  set: (value) => {
    filters.task_type = value === 'shell_task' || value === 'script_file_task' ? value : ''
  },
})

const searchDraft = reactive({
  keyword: '',
})

const modalMaskStyle = computed(() => ({
  left: `${modalViewportInset.value}px`,
  width: `calc(100% - ${modalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalVisible.value ? 'auto' : 'none',
}))

const modalWrapProps = computed(() => ({
  style: {
    left: `${modalViewportInset.value}px`,
    width: `calc(100% - ${modalViewportInset.value}px)`,
    pointerEvents: modalVisible.value ? 'auto' : 'none',
  },
}))

function resetForm() {
  editingID.value = ''
  cloneSourceID.value = ''
  form.name = ''
  form.description = ''
  form.task_type = 'shell_task'
  form.shell_type = 'sh'
  form.script_path = ''
  form.script_text = ''
}

function taskTypeText(taskType: AgentScriptTaskType) {
  return taskType === 'script_file_task' ? '脚本文件' : 'Shell 脚本'
}

function formatTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
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
  searchDraft.keyword = filters.keyword.trim()
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
    const response = await listAgentScripts({
      keyword: normalizedKeyword,
      task_type: (filters.task_type || undefined) as AgentScriptTaskType | undefined,
      page: 1,
      page_size: 6,
    })
    if (requestSeq !== searchSuggestionRequestSeq) {
      return
    }
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: item.id,
      title: item.name || item.script_path || item.id,
      subtitle: `${taskTypeText(item.task_type)} · ${item.script_path || item.shell_type || '-'}`,
      query: item.name || item.script_path || normalizedKeyword,
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

async function loadScripts() {
  if (!canView.value) return
  loading.value = true
  try {
    const response = await listAgentScripts({
      keyword: filters.keyword.trim() || undefined,
      task_type: (filters.task_type || undefined) as AgentScriptTaskType | undefined,
      page: filters.page,
      page_size: filters.page_size,
    })
    if (!pageAlive) return
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.page_size = response.page_size
  } catch (error) {
    if (pageAlive) {
      message.error(extractHTTPErrorMessage(error, '脚本列表加载失败'))
    }
  } finally {
    if (pageAlive) {
      loading.value = false
    }
  }
}

function queryScripts() {
  filters.page = 1
  void loadScripts()
}

function handleSearchSubmit() {
  filters.keyword = searchDraft.keyword.trim()
  filters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadScripts()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.query
  filters.keyword = item.query
  filters.page = 1
  searchDialogVisible.value = false
  clearSearchSuggestions()
  void loadScripts()
}

function openCreate() {
  resetForm()
  modalVisible.value = true
}

function fillScriptForm(record: AgentScript, name: string) {
  form.name = name
  form.description = record.description || ''
  form.task_type = record.task_type
  form.shell_type = record.shell_type || 'sh'
  form.script_path = record.script_path || ''
  form.script_text = record.script_text || ''
}

function openEdit(record: AgentScript) {
  editingID.value = record.id
  cloneSourceID.value = ''
  fillScriptForm(record, record.name)
  modalVisible.value = true
}

function openClone(record: AgentScript) {
  editingID.value = ''
  cloneSourceID.value = record.id
  fillScriptForm(record, buildCloneName(record.name))
  modalVisible.value = true
}

function closeModal() {
  modalVisible.value = false
  resetForm()
}

const uploadProps: UploadProps = {
  beforeUpload: async (file) => {
    const lowerName = String(file.name || '').toLowerCase()
    if (!(lowerName.endsWith('.sh') || lowerName.endsWith('.bash'))) {
      message.error('脚本管理仅支持上传 .sh 或 .bash 文件')
      return false
    }
    form.script_path = file.name
    form.script_text = await file.text()
    return false
  },
  showUploadList: false,
  accept: '.sh,.bash',
}

async function handleSave() {
  saving.value = true
  try {
    const payload: UpsertAgentScriptPayload = {
      name: form.name,
      description: form.description,
      task_type: form.task_type,
      shell_type: form.shell_type,
      script_path: form.task_type === 'script_file_task' ? form.script_path : '',
      script_text: form.script_text,
    }
    if (editingID.value) {
      await updateAgentScript(editingID.value, payload)
      message.success('脚本已更新')
    } else {
      await createAgentScript(payload)
      message.success(cloneSourceID.value ? '脚本已克隆' : '脚本已创建')
    }
    closeModal()
    await loadScripts()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editingID.value ? '脚本更新失败' : cloneSourceID.value ? '脚本克隆失败' : '脚本创建失败'))
  } finally {
    saving.value = false
  }
}

function handleDelete(record: AgentScript) {
  Modal.confirm({
    title: '删除脚本',
    content: `确认删除脚本“${record.name}”吗？`,
    okText: '删除',
    cancelText: '取消',
    okButtonProps: { danger: true },
    async onOk() {
      try {
        await deleteAgentScript(record.id)
        message.success('脚本已删除')
        await loadScripts()
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '脚本删除失败'))
      }
    },
  })
}

onMounted(() => {
  pageAlive = true
  syncModalViewportInset()
  observeModalViewportInset()
  void loadScripts()
})

onUnmounted(() => {
  pageAlive = false
  clearSearchSuggestions()
  stopObservingModalViewportInset()
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
        <div class="page-title">脚本</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon><SearchOutlined /></template>
        </a-button>
        <a-select
          v-model:value="taskTypeFilterValue"
          class="component-toolbar-select"
          :options="[
            { label: '类型 · 全部', value: '' },
            { label: '类型 · Shell', value: 'shell_task' },
            { label: '类型 · 文件', value: 'script_file_task' },
          ]"
        />
        <a-button class="component-toolbar-query-btn" @click="queryScripts">查询</a-button>
        <a-button v-if="canManage" class="application-toolbar-action-btn" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          新增脚本
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
              placeholder="脚本名称 / 文件名 / 说明"
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

    <a-card :bordered="false" class="table-card">
      <a-table
        row-key="id"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="{
          current: filters.page,
          pageSize: filters.page_size,
          total,
          showSizeChanger: true,
          pageSizeOptions: ['10', '20', '50'],
          onChange: (page: number, pageSize: number) => {
            filters.page = page
            filters.page_size = pageSize
            loadScripts()
          },
          onShowSizeChange: (_current: number, size: number) => {
            filters.page = 1
            filters.page_size = size
            loadScripts()
          },
        }"
        :scroll="{ x: 1180 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <div class="script-name">{{ record.name }}</div>
            <div class="muted-text">{{ record.created_by || '系统维护' }}</div>
          </template>
          <template v-else-if="column.key === 'task_type'">
            <a-tag>{{ taskTypeText(record.task_type) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'shell_type'">
            <span>{{ record.shell_type || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'script_path'">
            <span>{{ record.script_path || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'updated_at'">
            <span>{{ formatTime(record.updated_at) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-button v-if="canManage" class="component-row-action-btn" size="small" @click="openEdit(record)">
                <template #icon><EditOutlined /></template>
                编辑
              </a-button>
              <a-button v-if="canManage" class="component-row-action-btn" size="small" @click="openClone(record)">
                <template #icon><CopyOutlined /></template>
                克隆
              </a-button>
              <a-button v-if="canManage" class="component-row-action-btn component-row-action-btn--danger" size="small" @click="handleDelete(record)">
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      :open="modalVisible"
      :width="720"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="modalMaskStyle"
      :wrap-props="modalWrapProps"
      wrap-class-name="component-instance-modal-wrap agent-script-modal-wrap"
      @cancel="closeModal"
    >
      <template #title>
        <div class="component-instance-modal-titlebar">
          <span class="component-instance-modal-title">{{ editingID ? '编辑脚本' : cloneSourceID ? '克隆脚本' : '新增脚本' }}</span>
          <a-button class="application-toolbar-action-btn component-instance-modal-save-btn" :loading="saving" @click="handleSave">
            保存
          </a-button>
        </div>
      </template>

      <a-form layout="vertical" :required-mark="false" class="component-instance-form">
        <div class="component-instance-form-note">
          Shell 内容直接进入脚本库；上传文件时仅接受 .sh / .bash，并会同步填充脚本文本
        </div>
        <a-form-item label="脚本名称">
          <a-input v-model:value="form.name" placeholder="例如：生产服务重启、校验目录结构" />
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="脚本类型">
              <a-select v-model:value="form.task_type">
                <a-select-option value="shell_task">Shell 脚本</a-select-option>
                <a-select-option value="script_file_task">脚本文件</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Shell 类型">
              <a-select v-model:value="form.shell_type">
                <a-select-option value="sh">sh</a-select-option>
                <a-select-option value="bash">bash</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item v-if="form.task_type === 'script_file_task'" label="上传脚本文件">
          <a-space direction="vertical" style="width: 100%">
            <a-upload v-bind="uploadProps">
              <a-button class="component-upload-btn">
                <template #icon><UploadOutlined /></template>
                选择 .sh/.bash 文件
              </a-button>
            </a-upload>
            <div class="muted-text">脚本文件会保存在平台脚本库中，任务引用时会由 Agent 拉取并执行</div>
            <div v-if="form.script_path" class="muted-text">已选择：{{ form.script_path }}</div>
          </a-space>
        </a-form-item>
        <a-form-item v-else label="脚本内容">
          <a-textarea v-model:value="form.script_text" :rows="14" placeholder='例如：echo "hello {env}"\npwd\nls -la' />
        </a-form-item>
        <a-form-item v-if="form.task_type === 'script_file_task' && form.script_text" label="脚本内容预览">
          <a-textarea :value="form.script_text" :rows="12" readonly />
        </a-form-item>
        <a-form-item label="说明">
          <a-textarea v-model:value="form.description" :rows="3" placeholder="记录脚本用途、适用环境和注意事项" />
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

.table-card :deep(.ant-table-wrapper) {
  overflow: visible;
}

.table-card :deep(.ant-table) {
  overflow: hidden;
  border-radius: 18px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(255, 255, 255, 0.56);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.76),
    0 14px 28px rgba(15, 23, 42, 0.05);
}

.table-card :deep(.ant-table-thead > tr > th) {
  background: rgba(15, 23, 42, 0.9) !important;
  color: #f8fafc !important;
  font-size: 12px;
  font-weight: 800;
}

.table-card :deep(.ant-table-tbody > tr > td) {
  background: rgba(255, 255, 255, 0.66);
  color: #0f172a;
  font-size: 12px;
}

.table-card :deep(.ant-pagination) {
  margin: 14px 0 0;
}

.script-name {
  color: #0f172a;
  font-weight: 800;
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
  min-width: 132px;
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

:deep(.component-row-action-btn--danger.ant-btn) {
  border-color: rgba(252, 165, 165, 0.64) !important;
  color: #dc2626 !important;
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

.component-upload-btn {
  border-radius: 14px;
}

@media (max-width: 900px) {
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

  .component-instance-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
