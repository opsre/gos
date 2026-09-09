<script setup lang="ts">
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import type { FormInstance } from 'ant-design-vue'
import { computed, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createProject, deleteProject, listProjects, updateProject } from '../../api/project'
import type { Project, ProjectPayload, ProjectStatus } from '../../types/project'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface ProjectFormState {
  name: string
  key: string
  description: string
  status: ProjectStatus
}

interface ReadonlyFieldItem {
  label: string
  value: string
}

const loading = ref(false)
const router = useRouter()
const saving = ref(false)
const modalOpen = ref(false)
const editingId = ref('')
const dataSource = ref<Project[]>([])
const total = ref(0)
const filters = reactive({ name: '', page: 1, page_size: 10 })
const formRef = ref<FormInstance>()
const form = reactive<ProjectFormState>({ name: '', key: '', description: '', status: 'active' })
const currentProject = ref<Project | null>(null)
const projectFormViewportInset = ref(0)

const isEditMode = computed(() => Boolean(editingId.value))

const modalTitle = computed(() => (isEditMode.value ? '编辑项目' : '新增项目'))

const statusOptions = [
  { label: 'active', value: 'active' },
  { label: 'inactive', value: 'inactive' },
] as const

const projectReadonlyFields = computed<ReadonlyFieldItem[]>(() => {
  if (!currentProject.value) {
    return []
  }
  return [
    { label: '当前项目名称', value: currentProject.value.name || '-' },
    { label: '项目 Key', value: currentProject.value.key || '-' },
  ]
})

const projectFormMaskStyle = computed(() => ({
  left: `${projectFormViewportInset.value}px`,
  width: `calc(100% - ${projectFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalOpen.value ? 'auto' : 'none',
}))

const projectFormWrapProps = computed(() => ({
  style: {
    left: `${projectFormViewportInset.value}px`,
    width: `calc(100% - ${projectFormViewportInset.value}px)`,
    pointerEvents: modalOpen.value ? 'auto' : 'none',
  },
}))

let projectFormViewportObserver: ResizeObserver | null = null

function readProjectFormViewportInset() {
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

function syncProjectFormViewportInset() {
  projectFormViewportInset.value = readProjectFormViewportInset()
}

function observeProjectFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  projectFormViewportObserver?.disconnect()
  projectFormViewportObserver = new ResizeObserver(() => {
    syncProjectFormViewportInset()
  })

  if (appLayout) {
    projectFormViewportObserver.observe(appLayout)
  }
  if (sider) {
    projectFormViewportObserver.observe(sider)
  }
}

function stopObservingProjectFormViewportInset() {
  projectFormViewportObserver?.disconnect()
  projectFormViewportObserver = null
}

async function loadProjects() {
  loading.value = true
  try {
    const response = await listProjects({ ...filters })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.page_size = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '项目列表加载失败'))
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.name = ''
  form.key = ''
  form.description = ''
  form.status = 'active'
  editingId.value = ''
  currentProject.value = null
}

function openCreate() {
  resetForm()
  modalOpen.value = true
}

function openEdit(record: Project) {
  editingId.value = record.id
  currentProject.value = { ...record }
  form.name = record.name
  form.key = record.key
  form.description = record.description
  form.status = record.status
  modalOpen.value = true
}

function closeFormModal() {
  modalOpen.value = false
}

function handleFormAfterClose() {
  saving.value = false
  resetForm()
  formRef.value?.clearValidate()
}

async function submitForm() {
  await formRef.value?.validate()

  saving.value = true
  const payload: ProjectPayload = {
    name: form.name.trim(),
    key: form.key.trim(),
    description: form.description.trim(),
    status: form.status,
  }
  try {
    if (editingId.value) {
      await updateProject(editingId.value, payload)
      message.success('项目更新成功')
    } else {
      await createProject(payload)
      message.success('项目创建成功')
    }
    modalOpen.value = false
    await loadProjects()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editingId.value ? '项目更新失败' : '项目创建失败'))
  } finally {
    saving.value = false
  }
}

function confirmDelete(record: Project) {
  Modal.confirm({
    title: '确认删除项目吗？',
    content: h('div', `项目 ${record.name} 删除后将无法恢复，请确认当前没有新的应用继续归属到该项目`),
    okText: '删除',
    cancelText: '取消',
    okButtonProps: { danger: true },
    async onOk() {
      try {
        await deleteProject(record.id)
        message.success('项目删除成功')
        await loadProjects()
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, '项目删除失败'))
        throw error
      }
    },
  })
}

function handleSearch() {
  filters.page = 1
  void loadProjects()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.page_size = pageSize
  void loadProjects()
}

onMounted(() => {
  syncProjectFormViewportInset()
  observeProjectFormViewportInset()
  void loadProjects()
})

onBeforeUnmount(() => {
  stopObservingProjectFormViewportInset()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header project-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">项目</h2>
      </div>
      <div class="page-header-actions project-header-actions">
        <a-input
          v-model:value="filters.name"
          class="project-toolbar-search"
          placeholder="名称"
          @pressEnter="handleSearch"
        />
        <a-button class="project-toolbar-query-btn" @click="handleSearch">查询</a-button>
        <a-button class="application-toolbar-action-btn project-create-btn" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          新增项目
        </a-button>
      </div>
    </div>

    <div class="project-table-section">
      <a-table
        class="project-table"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        row-key="id"
        :scroll="{ x: 920 }"
      >
        <a-table-column title="项目名称" data-index="name" key="name" />
        <a-table-column title="项目 Key" data-index="key" key="key" />
        <a-table-column title="状态" data-index="status" key="status" width="120" />
        <a-table-column title="描述" data-index="description" key="description" />
        <a-table-column title="操作" key="action" width="280">
          <template #default="{ record }">
            <a-space>
              <a-button type="link" :disabled="record.status !== 'active'" @click="router.push({ path: '/system/quick-start', query: { project_id: record.id } })">新增应用</a-button>
              <a-button type="link" @click="openEdit(record)">
                <template #icon><EditOutlined /></template>
                编辑
              </a-button>
              <a-button danger type="link" @click="confirmDelete(record)">
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-space>
          </template>
        </a-table-column>
      </a-table>
    </div>

    <div class="pagination-area">
      <a-pagination
        :current="filters.page"
        :page-size="filters.page_size"
        :total="total"
        :page-size-options="['10', '20', '50']"
        show-size-changer
        :show-total="(count: number) => `共 ${count} 个项目`"
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
      :mask-style="projectFormMaskStyle"
      :wrap-props="projectFormWrapProps"
      wrap-class-name="project-form-modal-wrap"
      @cancel="closeFormModal"
    >
      <template #title>
        <div class="project-form-modal-titlebar">
          <span class="project-form-modal-title">{{ modalTitle }}</span>
          <a-button class="application-toolbar-action-btn project-form-modal-save-btn" :loading="saving" @click="submitForm">
            保存
          </a-button>
        </div>
      </template>

      <a-form ref="formRef" :model="form" layout="vertical" :required-mark="false" class="project-form">
        <div v-if="isEditMode" class="project-form-note">
          编辑态保留项目 Key 为只读标识，避免项目归属标识被误改
        </div>

        <div v-if="isEditMode" class="project-form-panel project-form-panel--context">
          <div class="project-form-panel-title">当前配置</div>
          <div class="project-form-context">
            <div v-for="item in projectReadonlyFields" :key="item.label" class="project-form-context-item">
              <div class="project-form-context-label">{{ item.label }}</div>
              <div class="project-form-context-value">{{ item.value }}</div>
            </div>
          </div>
        </div>

        <div class="project-form-panel">
          <div class="project-form-panel-title">{{ isEditMode ? '可编辑配置' : '项目配置' }}</div>

          <a-form-item name="name" :rules="[{ required: true, message: '请输入项目名称' }]">
            <template #label>
              <span class="project-form-label">
                项目名称
                <a-tag class="project-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.name" placeholder="例如：南通业务线" />
          </a-form-item>

          <a-form-item v-if="!isEditMode" name="key" :rules="[{ required: true, message: '请输入项目 Key' }]">
            <template #label>
              <span class="project-form-label">
                项目 Key
                <a-tag class="project-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="form.key" placeholder="例如：nantong" />
          </a-form-item>

          <a-form-item name="status" :rules="[{ required: true, message: '请选择状态' }]">
            <template #label>
              <span class="project-form-label">
                状态
                <a-tag class="project-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="form.status" :options="statusOptions" placeholder="请选择状态" />
          </a-form-item>

          <a-form-item label="项目描述" name="description">
            <a-textarea v-model:value="form.description" :rows="4" placeholder="请输入项目描述" />
          </a-form-item>
        </div>
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

.project-page-header {
  flex-wrap: wrap;
  padding: 0 !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
}

.project-header-actions {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  min-width: min(100%, 420px);
}

:deep(.project-toolbar-search.ant-input) {
  width: min(240px, 100%);
  height: 42px;
  border-radius: 16px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #1e3a8a;
  font-weight: 650;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.project-toolbar-search.ant-input::placeholder) {
  color: rgba(30, 58, 138, 0.38);
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.project-toolbar-query-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 42px;
  border-radius: 16px;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #0f172a !important;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.project-toolbar-query-btn.ant-btn:hover),
:deep(.project-toolbar-query-btn.ant-btn:focus),
:deep(.project-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
}

.project-table-section {
  margin-top: 24px;
}

.project-table :deep(.ant-table) {
  background: transparent;
}

.project-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.34);
}

.project-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.project-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.project-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: var(--color-text-main);
}

.project-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.project-table :deep(.ant-table-tbody > tr > td:last-child) {
  background: rgba(255, 255, 255, 0.96) !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.04);
}

.project-form-modal-wrap :deep(.ant-modal-content) {
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

.project-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.project-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.project-form-modal-wrap :deep(.ant-modal-title) {
  color: #0f172a;
}

.project-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.project-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.project-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: normal;
}

.project-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

.project-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.project-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.project-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.42), rgba(251, 191, 36, 0.16));
}

.project-form-panel {
  padding: 0;
}

.project-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.project-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.project-form-note + .project-form-panel,
.project-form-panel + .project-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.project-form-panel--context {
  padding-bottom: 4px;
}

.project-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.project-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.project-form :deep(.ant-input),
.project-form :deep(.ant-select-selector),
.project-form :deep(.ant-input-affix-wrapper),
.project-form :deep(.ant-input-textarea) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.project-form :deep(.ant-input:hover),
.project-form :deep(.ant-input-affix-wrapper:hover),
.project-form :deep(.ant-input-textarea:hover),
.project-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.project-form :deep(.ant-input:focus),
.project-form :deep(.ant-input-focused),
.project-form :deep(.ant-input-affix-wrapper-focused),
.project-form :deep(.ant-select-focused .ant-select-selector),
.project-form :deep(.ant-input-textarea-focused) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.project-form :deep(.ant-select-selection-placeholder),
.project-form :deep(.ant-input::placeholder),
.project-form :deep(.ant-input-textarea textarea::placeholder) {
  color: #94a3b8;
}

.project-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.project-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.project-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.project-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .project-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .project-form-context {
    grid-template-columns: 1fr;
  }
}
</style>
