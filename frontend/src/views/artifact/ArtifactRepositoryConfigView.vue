<script setup lang="ts">
import { CloudServerOutlined, DeleteOutlined, EditOutlined, EyeOutlined, LinkOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  createArtifactRepository,
  deleteArtifactRepository,
  listArtifactRepositories,
  testArtifactRepositoryConnection,
  updateArtifactRepository,
} from '../../api/artifact-repository'
import type {
  ArtifactRepository,
  ArtifactRepositoryACL,
  ArtifactRepositoryPayload,
  ArtifactRepositoryStatus,
  ArtifactRepositoryType,
} from '../../types/artifact-repository'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface ArtifactRepositoryFormState {
  name: string
  type: ArtifactRepositoryType
  endpoint: string
  bucket: string
  directory: string
  access_key_id: string
  access_key_secret: string
  acl: ArtifactRepositoryACL
  status: ArtifactRepositoryStatus
}

const repositoryModalVisible = ref(false)
const savingRepository = ref(false)
const testingRepositoryConnection = ref(false)
const loadingRepositories = ref(false)
const deletingRepositoryID = ref('')
const repositoryFormRef = ref<FormInstance>()
const artifactRepositories = ref<ArtifactRepository[]>([])
const editorMode = ref<'create' | 'edit'>('create')
const editingRepositoryID = ref('')
const detailVisible = ref(false)
const detailRepository = ref<ArtifactRepository | null>(null)
const repositoryModalViewportInset = ref(0)

const repositoryTypeOptions = [
  { label: 'OSS 对象存储', value: 'oss' },
]

const statusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
]

const repositoryColumns: TableColumnsType<ArtifactRepository> = [
  { title: '制品库', dataIndex: 'name', key: 'name', width: 220 },
  { title: '类型', dataIndex: 'type', key: 'type', width: 150 },
  { title: 'Endpoint', dataIndex: 'endpoint', key: 'endpoint', width: 260, ellipsis: true },
  { title: 'Bucket', dataIndex: 'bucket', key: 'bucket', width: 180 },
  { title: '目录', dataIndex: 'directory', key: 'directory', width: 180 },
  { title: 'ACL', dataIndex: 'acl', key: 'acl', width: 150 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 110 },
  { title: '操作', key: 'actions', width: 230, fixed: 'right' },
]

const repositoryForm = reactive<ArtifactRepositoryFormState>({
  name: '',
  type: 'oss',
  endpoint: '',
  bucket: '',
  directory: '',
  access_key_id: '',
  access_key_secret: '',
  acl: 'private',
  status: 'enabled',
})

const repositoryFormRules = {
  name: [{ required: true, message: '请输入制品库名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择制品库类型', trigger: 'change' }],
  endpoint: [{ required: true, message: '请输入 OSS Endpoint', trigger: 'blur' }],
  bucket: [{ required: true, message: '请输入 Bucket', trigger: 'blur' }],
  access_key_id: [{ required: true, message: '请输入 AccessKey ID', trigger: 'blur' }],
  access_key_secret: [{ required: true, message: '请输入 AccessKey Secret', trigger: 'blur' }],
  acl: [{ required: true, message: '请选择 ACL', trigger: 'change' }],
}

const tableLocale = computed(() => ({
  emptyText: '暂无制品库配置',
}))
const repositoryModalTitle = computed(() => (editorMode.value === 'create' ? '新增制品库' : '编辑制品库'))
const repositoryModalMaskStyle = computed(() => ({
  left: `${repositoryModalViewportInset.value}px`,
  width: `calc(100% - ${repositoryModalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: repositoryModalVisible.value ? 'auto' : 'none',
}))
const repositoryModalWrapProps = computed(() => ({
  style: {
    left: `${repositoryModalViewportInset.value}px`,
    width: `calc(100% - ${repositoryModalViewportInset.value}px)`,
    pointerEvents: repositoryModalVisible.value ? 'auto' : 'none',
  },
}))

let repositoryModalViewportObserver: ResizeObserver | null = null

async function loadArtifactRepositories() {
  loadingRepositories.value = true
  try {
    const response = await listArtifactRepositories({ page: 1, page_size: 100 })
    artifactRepositories.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载制品库失败'))
  } finally {
    loadingRepositories.value = false
  }
}

function resetRepositoryForm() {
  repositoryForm.name = ''
  repositoryForm.type = 'oss'
  repositoryForm.endpoint = ''
  repositoryForm.bucket = ''
  repositoryForm.directory = ''
  repositoryForm.access_key_id = ''
  repositoryForm.access_key_secret = ''
  repositoryForm.acl = 'private'
  repositoryForm.status = 'enabled'
  repositoryFormRef.value?.clearValidate()
}

function openCreateRepositoryModal() {
  editorMode.value = 'create'
  editingRepositoryID.value = ''
  resetRepositoryForm()
  repositoryForm.type = 'oss'
  repositoryModalVisible.value = true
}

function closeRepositoryModal() {
  repositoryModalVisible.value = false
}

function normalizeDirectory(value: string) {
  const raw = String(value || '').trim().replace(/^\/+|\/+$/g, '')
  return raw || '/'
}

function buildRepositoryPayload(): ArtifactRepositoryPayload {
  return {
    name: repositoryForm.name.trim(),
    type: repositoryForm.type,
    endpoint: repositoryForm.endpoint.trim(),
    bucket: repositoryForm.bucket.trim(),
    directory: normalizeDirectory(repositoryForm.directory),
    access_key_id: repositoryForm.access_key_id.trim(),
    access_key_secret: repositoryForm.access_key_secret.trim(),
    acl: repositoryForm.acl,
    status: repositoryForm.status,
  }
}

function formatRepositoryType(type: ArtifactRepositoryType) {
  if (type === 'oss') {
    return 'OSS 对象存储'
  }
  return type
}

function formatStatus(status: ArtifactRepositoryStatus) {
  return status === 'enabled' ? '启用' : '停用'
}

function formatACL(acl: ArtifactRepositoryACL) {
  return acl === 'public-read' ? '公共读 public-read' : '私有 private'
}

function statusColor(status: ArtifactRepositoryStatus) {
  return status === 'enabled' ? 'green' : 'default'
}

function maskSecret(value: string) {
  if (!value) {
    return '-'
  }
  return '••••••••'
}

function populateRepositoryForm(record: ArtifactRepository) {
  repositoryForm.name = record.name
  repositoryForm.type = record.type
  repositoryForm.endpoint = record.endpoint
  repositoryForm.bucket = record.bucket
  repositoryForm.directory = record.directory === '/' ? '' : record.directory
  repositoryForm.access_key_id = record.access_key_id
  repositoryForm.access_key_secret = record.access_key_secret
  repositoryForm.acl = record.acl
  repositoryForm.status = record.status
  repositoryFormRef.value?.clearValidate()
}

function openRepositoryDetail(record: ArtifactRepository) {
  detailRepository.value = record
  detailVisible.value = true
}

function closeRepositoryDetail() {
  detailVisible.value = false
}

function openEditRepositoryModal(record: ArtifactRepository) {
  editorMode.value = 'edit'
  editingRepositoryID.value = record.id
  populateRepositoryForm(record)
  repositoryModalVisible.value = true
}

async function deleteRepository(record: ArtifactRepository) {
  deletingRepositoryID.value = record.id
  try {
    await deleteArtifactRepository(record.id)
    artifactRepositories.value = artifactRepositories.value.filter((item) => item.id !== record.id)
    if (detailRepository.value?.id === record.id) {
      detailRepository.value = null
      detailVisible.value = false
    }
    message.success('制品库已删除')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除制品库失败'))
  } finally {
    deletingRepositoryID.value = ''
  }
}

async function submitRepository() {
  await repositoryFormRef.value?.validate()
  savingRepository.value = true
  try {
    const payload = buildRepositoryPayload()

    if (editorMode.value === 'edit' && editingRepositoryID.value) {
      const response = await updateArtifactRepository(editingRepositoryID.value, payload)
      artifactRepositories.value = artifactRepositories.value.map((item) =>
        item.id === editingRepositoryID.value
          ? response.data
          : item,
      )
      if (detailRepository.value?.id === editingRepositoryID.value) {
        detailRepository.value = response.data
      }
      message.success('制品库已更新')
      closeRepositoryModal()
      return
    }

    const response = await createArtifactRepository(payload)
    artifactRepositories.value = [response.data, ...artifactRepositories.value]
    message.success('制品库已新增')
    closeRepositoryModal()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editorMode.value === 'edit' ? '更新制品库失败' : '新增制品库失败'))
  } finally {
    savingRepository.value = false
  }
}

function validateRepositoryConnectionInput() {
  const requiredFields = [
    { value: repositoryForm.endpoint, message: '请输入 OSS Endpoint' },
    { value: repositoryForm.bucket, message: '请输入 Bucket' },
    { value: repositoryForm.access_key_id, message: '请输入 AccessKey ID' },
    { value: repositoryForm.access_key_secret, message: '请输入 AccessKey Secret' },
  ]
  const missing = requiredFields.find((field) => !String(field.value || '').trim())
  return missing?.message || ''
}

async function testRepositoryConnection() {
  const warning = validateRepositoryConnectionInput()
  if (warning) {
    message.warning(warning)
    return
  }

  testingRepositoryConnection.value = true
  try {
    const payload = buildRepositoryPayload()
    const response = await testArtifactRepositoryConnection(payload)
    message.success(response.message || '制品库连通性检测通过')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '制品库连通性检测失败'))
  } finally {
    testingRepositoryConnection.value = false
  }
}

function readRepositoryModalViewportInset() {
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

function syncRepositoryModalViewportInset() {
  repositoryModalViewportInset.value = readRepositoryModalViewportInset()
}

function observeRepositoryModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  repositoryModalViewportObserver?.disconnect()
  repositoryModalViewportObserver = new ResizeObserver(() => {
    syncRepositoryModalViewportInset()
  })

  if (appLayout) {
    repositoryModalViewportObserver.observe(appLayout)
  }
  if (sider) {
    repositoryModalViewportObserver.observe(sider)
  }
}

function stopObservingRepositoryModalViewportInset() {
  repositoryModalViewportObserver?.disconnect()
  repositoryModalViewportObserver = null
}

onMounted(() => {
  syncRepositoryModalViewportInset()
  observeRepositoryModalViewportInset()
  void loadArtifactRepositories()
})

onBeforeUnmount(() => {
  stopObservingRepositoryModalViewportInset()
})
</script>

<template>
  <div class="artifact-config-page">
    <div class="page-header">
      <div>
        <div class="page-title">制品库配置</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-action-btn artifact-create-btn" @click="openCreateRepositoryModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增制品库
        </a-button>
      </div>
    </div>

    <a-table
      row-key="id"
      class="artifact-repository-table"
      :columns="repositoryColumns"
      :data-source="artifactRepositories"
      :loading="loadingRepositories"
      :pagination="false"
      :locale="tableLocale"
      :scroll="{ x: 1250 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <div class="repository-name-cell">
            <span class="repository-name-icon"><CloudServerOutlined /></span>
            <span>
              <span class="repository-name">{{ record.name }}</span>
              <span class="repository-secret">AK {{ maskSecret(record.access_key_secret) }}</span>
            </span>
          </div>
        </template>
        <template v-else-if="column.key === 'type'">
          <a-tag>{{ formatRepositoryType(record.type) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'directory'">
          {{ record.directory || '/' }}
        </template>
        <template v-else-if="column.key === 'acl'">
          <a-tag>{{ formatACL(record.acl) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="statusColor(record.status)">{{ formatStatus(record.status) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'actions'">
          <div class="row-actions">
            <a-button type="link" size="small" class="row-action-btn" @click="openRepositoryDetail(record)">
              <template #icon>
                <EyeOutlined />
              </template>
              查看基础信息
            </a-button>
            <a-button type="link" size="small" class="row-action-btn" @click="openEditRepositoryModal(record)">
              <template #icon>
                <EditOutlined />
              </template>
              编辑
            </a-button>
            <a-popconfirm title="确认删除当前制品库吗？" ok-text="删除" cancel-text="取消" @confirm="deleteRepository(record)">
              <a-button type="link" size="small" class="row-action-btn" :loading="deletingRepositoryID === record.id" danger>
                <template #icon>
                  <DeleteOutlined />
                </template>
                删除
              </a-button>
            </a-popconfirm>
          </div>
        </template>
      </template>
    </a-table>

    <a-modal
      :open="repositoryModalVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="repositoryModalMaskStyle"
      :wrap-props="repositoryModalWrapProps"
      wrap-class-name="artifact-repository-modal-wrap"
      @cancel="closeRepositoryModal"
    >
      <template #title>
        <div class="artifact-repository-modal-titlebar">
          <span class="artifact-repository-modal-title">{{ repositoryModalTitle }}</span>
          <div class="artifact-repository-modal-actions">
            <a-button
              class="application-toolbar-action-btn artifact-repository-test-btn"
              :loading="testingRepositoryConnection"
              :disabled="savingRepository"
              @click="testRepositoryConnection"
            >
              <template #icon>
                <LinkOutlined />
              </template>
              测试连通性
            </a-button>
            <a-button
              class="application-toolbar-action-btn artifact-repository-save-btn"
              :loading="savingRepository"
              :disabled="testingRepositoryConnection"
              @click="submitRepository"
            >
              保存
            </a-button>
          </div>
        </div>
      </template>

      <a-form
        ref="repositoryFormRef"
        class="artifact-repository-form"
        layout="vertical"
        :model="repositoryForm"
        :rules="repositoryFormRules"
        :required-mark="false"
      >
        <section class="artifact-form-panel artifact-form-panel-basic">
          <div class="artifact-form-panel-title">基础信息</div>
          <div class="artifact-form-grid">
            <a-form-item name="name">
              <template #label>
                <span class="artifact-form-label">制品库名称<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="repositoryForm.name" placeholder="例如 oa-jar-oss" />
            </a-form-item>
            <a-form-item name="type">
              <template #label>
                <span class="artifact-form-label">制品库类型<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-select v-model:value="repositoryForm.type" :options="repositoryTypeOptions" />
            </a-form-item>
            <a-form-item name="status">
              <template #label>
                <span class="artifact-form-label">状态</span>
              </template>
              <a-select v-model:value="repositoryForm.status" :options="statusOptions" />
            </a-form-item>
            <a-form-item name="acl">
              <template #label>
                <span class="artifact-form-label">默认 ACL<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-radio-group v-model:value="repositoryForm.acl" class="artifact-acl-radio">
                <a-radio-button value="private">私有</a-radio-button>
                <a-radio-button value="public-read">公共读</a-radio-button>
              </a-radio-group>
            </a-form-item>
          </div>
        </section>

        <section class="artifact-form-panel">
          <div class="artifact-form-panel-title">OSS 连接</div>
          <div class="artifact-form-grid">
            <a-form-item name="endpoint">
              <template #label>
                <span class="artifact-form-label">Endpoint<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="repositoryForm.endpoint" placeholder="https://oss-cn-shanghai.aliyuncs.com" />
            </a-form-item>
            <a-form-item name="bucket">
              <template #label>
                <span class="artifact-form-label">Bucket<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="repositoryForm.bucket" placeholder="例如 oa" />
            </a-form-item>
            <a-form-item name="directory">
              <template #label>
                <span class="artifact-form-label">目录前缀</span>
              </template>
              <a-input v-model:value="repositoryForm.directory" placeholder="例如 releases/jar，可为空" />
            </a-form-item>
          </div>
        </section>

        <section class="artifact-form-panel">
          <div class="artifact-form-panel-title">访问凭证</div>
          <div class="artifact-form-grid">
            <a-form-item name="access_key_id">
              <template #label>
                <span class="artifact-form-label">AccessKey ID<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="repositoryForm.access_key_id" placeholder="请输入 AccessKey ID" />
            </a-form-item>
            <a-form-item name="access_key_secret">
              <template #label>
                <span class="artifact-form-label">AccessKey Secret<a-tag class="artifact-required-tag">必填</a-tag></span>
              </template>
              <a-input-password v-model:value="repositoryForm.access_key_secret" autocomplete="new-password" placeholder="请输入 AccessKey Secret" />
            </a-form-item>
          </div>
        </section>
      </a-form>
    </a-modal>

    <a-drawer
      :open="detailVisible"
      width="520"
      title="制品库基础信息"
      class="artifact-detail-drawer"
      @close="closeRepositoryDetail"
    >
      <div v-if="detailRepository" class="artifact-detail">
        <div class="artifact-detail-row">
          <span>制品库名称</span>
          <strong>{{ detailRepository.name }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>制品库类型</span>
          <strong>{{ formatRepositoryType(detailRepository.type) }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>状态</span>
          <strong class="artifact-detail-status" :class="{ 'artifact-detail-status--enabled': detailRepository.status === 'enabled' }">{{ formatStatus(detailRepository.status) }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>默认 ACL</span>
          <strong>{{ formatACL(detailRepository.acl) }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>Endpoint</span>
          <strong>{{ detailRepository.endpoint }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>Bucket</span>
          <strong>{{ detailRepository.bucket }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>目录前缀</span>
          <strong>{{ detailRepository.directory || '/' }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>AccessKey ID</span>
          <strong>{{ detailRepository.access_key_id }}</strong>
        </div>
        <div class="artifact-detail-row">
          <span>AccessKey Secret</span>
          <strong>{{ maskSecret(detailRepository.access_key_secret) }}</strong>
        </div>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.artifact-config-page {
  min-height: 100%;
  padding: 0;
  color: #0f172a;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.page-title {
  color: #0f172a;
  font-size: 24px;
  font-weight: 700;
  line-height: 1.25;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  flex: none;
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

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.artifact-repository-table :deep(.ant-table) {
  background: transparent;
}

.artifact-repository-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: #fff;
}

.artifact-repository-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.artifact-repository-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.artifact-repository-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: #fff;
  color: #0f172a;
}

.repository-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.repository-name-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  border-radius: 12px;
  background: rgba(219, 234, 254, 0.72);
  color: #2563eb;
}

.repository-name,
.repository-secret {
  display: block;
  min-width: 0;
}

.repository-name {
  color: #0f172a;
  font-weight: 700;
}

.repository-secret {
  margin-top: 3px;
  color: #94a3b8;
  font-size: 12px;
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.row-action-btn {
  padding-inline: 4px;
  color: #2563eb;
  font-weight: 650;
}

.row-action-btn:hover,
.row-action-btn:focus {
  color: #1d4ed8;
}

:global(.artifact-repository-modal-wrap .ant-modal) {
  padding-bottom: 32px;
}

:global(.artifact-repository-modal-wrap .ant-modal-content) {
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

:global(.artifact-repository-modal-wrap .ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

:global(.artifact-repository-modal-wrap .ant-modal-body) {
  padding: 10px 28px 28px;
}

.artifact-repository-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.artifact-repository-modal-title {
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

.artifact-repository-modal-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex: none;
}

:global(.artifact-repository-modal-wrap .artifact-repository-test-btn.ant-btn),
:global(.artifact-repository-modal-wrap .artifact-repository-save-btn.ant-btn) {
  flex: none;
  height: 42px;
  border: 1px solid rgba(96, 165, 250, 0.42) !important;
  border-radius: 16px;
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

:global(.artifact-repository-modal-wrap .artifact-repository-test-btn.ant-btn:not(:disabled):hover),
:global(.artifact-repository-modal-wrap .artifact-repository-test-btn.ant-btn:not(:disabled):focus),
:global(.artifact-repository-modal-wrap .artifact-repository-save-btn.ant-btn:not(:disabled):hover),
:global(.artifact-repository-modal-wrap .artifact-repository-save-btn.ant-btn:not(:disabled):focus) {
  border-color: rgba(37, 99, 235, 0.54) !important;
  background: rgba(255, 255, 255, 0.88) !important;
  color: #2563eb !important;
}

:global(.artifact-repository-modal-wrap .artifact-repository-test-btn.ant-btn) {
  padding-inline: 16px;
}

:global(.artifact-repository-modal-wrap .artifact-repository-save-btn.ant-btn) {
  padding-inline: 18px;
}

.artifact-repository-form {
  display: grid;
  gap: 14px;
}

.artifact-form-panel {
  padding: 0;
}

.artifact-form-panel + .artifact-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.artifact-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.artifact-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.artifact-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 14px;
}

.artifact-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

.artifact-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  border-radius: 999px;
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.artifact-form-panel :deep(.ant-form-item) {
  margin-bottom: 0;
}

.artifact-form-panel :deep(.ant-input),
.artifact-form-panel :deep(.ant-input-password),
.artifact-form-panel :deep(.ant-select-selector) {
  border-radius: 14px !important;
}

.artifact-acl-radio {
  display: flex;
}

.artifact-acl-radio :deep(.ant-radio-button-wrapper) {
  flex: 1;
  height: 38px;
  border-color: rgba(148, 163, 184, 0.26);
  background: rgba(255, 255, 255, 0.72);
  text-align: center;
  line-height: 36px;
  color: #334155;
  font-weight: 700;
  box-shadow: none;
}

.artifact-acl-radio :deep(.ant-radio-button-wrapper:hover) {
  color: #2563eb;
}

.artifact-acl-radio :deep(.ant-radio-button-wrapper-checked:not(.ant-radio-button-wrapper-disabled)) {
  border-color: rgba(37, 99, 235, 0.35);
  background: rgba(239, 246, 255, 0.9);
  color: #1d4ed8;
  box-shadow: none;
}

.artifact-acl-radio :deep(.ant-radio-button-wrapper-checked:not(.ant-radio-button-wrapper-disabled)::before) {
  background-color: rgba(37, 99, 235, 0.24);
}

.artifact-detail {
  display: grid;
  gap: 10px;
}

.artifact-detail-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 14px;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid rgba(226, 232, 240, 0.78);
}

.artifact-detail-row span {
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

.artifact-detail-row strong {
  min-width: 0;
  overflow-wrap: anywhere;
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.artifact-detail-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.artifact-detail-status::before {
  content: '';
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
  flex-shrink: 0;
}

.artifact-detail-status--enabled::before {
  background: #22c55e;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
  }

  .page-title {
    font-size: 21px;
  }

  .page-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .artifact-repository-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }

  .artifact-repository-modal-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .artifact-form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
