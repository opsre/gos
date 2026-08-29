<script setup lang="ts">
import { ExclamationCircleOutlined, PlusOutlined, SafetyCertificateOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  createPlatformParamDict,
  deletePlatformParamDict,
  getPlatformParamDictByID,
  listPlatformParamDicts,
  updatePlatformParamDict,
} from '../../api/platform-param'
import { useResizableColumns } from '../../composables/useResizableColumns'
import type {
  PlatformParamDict,
  PlatformParamDictPayload,
  PlatformParamStatus,
} from '../../types/platform-param'
import { extractHTTPErrorMessage } from '../../utils/http-error'

type FormMode = 'create' | 'edit'
type AbilityKind = 'builtin' | 'custom' | 'required' | 'gitops' | 'cd-self-fill'

interface AbilityTag {
  key: string
  label: string
  kind: AbilityKind
}

interface FormState extends PlatformParamDictPayload {
  id: string
}

const loading = ref(false)
const submitting = ref(false)
const deletingID = ref('')
const dataSource = ref<PlatformParamDict[]>([])
const total = ref(0)

const detailVisible = ref(false)
const detailLoading = ref(false)
const detailData = ref<PlatformParamDict | null>(null)

const modalVisible = ref(false)
const modalMode = ref<FormMode>('create')
const formRef = ref<FormInstance>()
const platformParamFormViewportInset = ref(0)

const filters = reactive({
  param_key: '',
  name: '',
  status: '' as '' | PlatformParamStatus,
  page: 1,
  pageSize: 20,
})

const formState = reactive<FormState>({
  id: '',
  param_key: '',
  name: '',
  description: '',
  param_type: 'string',
  required: false,
  gitops_locator: false,
  cd_self_fill: false,
  status: 1,
})

const initialColumns: TableColumnsType<PlatformParamDict> = [
  { title: '标准 Key', dataIndex: 'param_key', key: 'param_key', width: 180 },
  { title: '字段名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '字段说明', dataIndex: 'description', key: 'description', width: 260, ellipsis: true },
  { title: '字段类型', dataIndex: 'param_type', key: 'param_type', width: 120 },
  { title: '字段能力', key: 'abilities', width: 280 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 110 },
  { title: '操作', key: 'actions', width: 220, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, { minWidth: 100, maxWidth: 520, hitArea: 10 })

const typeOptions = [
  { label: 'string', value: 'string' },
  { label: 'choice', value: 'choice' },
  { label: 'bool', value: 'bool' },
  { label: 'number', value: 'number' },
] as const

const statusOptions = [
  { label: '启用', value: 1 },
  { label: '停用', value: 0 },
] as const
const statusFilterOptions = [
  { label: '全部状态', value: '' as const },
  ...statusOptions,
]
const protectedBuiltinParamKeys = new Set(['gos_artifact_path', 'repo_url'])

const modalTitle = computed(() => (modalMode.value === 'create' ? '新增标准字段' : '编辑标准字段'))
const platformParamFormMaskStyle = computed(() => ({
  left: `${platformParamFormViewportInset.value}px`,
  width: `calc(100% - ${platformParamFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalVisible.value ? 'auto' : 'none',
}))
const platformParamFormWrapProps = computed(() => ({
  style: {
    left: `${platformParamFormViewportInset.value}px`,
    width: `calc(100% - ${platformParamFormViewportInset.value}px)`,
    pointerEvents: modalVisible.value ? 'auto' : 'none',
  },
}))
let platformParamFormViewportObserver: ResizeObserver | null = null

function readPlatformParamFormViewportInset() {
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

function syncPlatformParamFormViewportInset() {
  platformParamFormViewportInset.value = readPlatformParamFormViewportInset()
}

function observePlatformParamFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  platformParamFormViewportObserver?.disconnect()
  platformParamFormViewportObserver = new ResizeObserver(() => {
    syncPlatformParamFormViewportInset()
  })

  if (appLayout) {
    platformParamFormViewportObserver.observe(appLayout)
  }
  if (sider) {
    platformParamFormViewportObserver.observe(sider)
  }
}

function stopObservingPlatformParamFormViewportInset() {
  platformParamFormViewportObserver?.disconnect()
  platformParamFormViewportObserver = null
}
function formatTime(value: string) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function statusText(status: PlatformParamStatus) {
  return status === 1 ? '启用' : '停用'
}

function statusColor(status: PlatformParamStatus) {
  return status === 1 ? 'green' : 'default'
}

function boolText(value: boolean) {
  return value ? '是' : '否'
}

function isBuiltinParam(item: Pick<PlatformParamDict, 'param_key' | 'builtin'> | null | undefined) {
  if (!item) {
    return false
  }
  return item.builtin || protectedBuiltinParamKeys.has(normalizeParamKey(item.param_key || ''))
}

function abilityTags(item: Pick<PlatformParamDict, 'param_key' | 'builtin' | 'required' | 'gitops_locator' | 'cd_self_fill'>): AbilityTag[] {
  const tags: AbilityTag[] = []
  tags.push(isBuiltinParam(item) ? { key: 'builtin', label: '内置', kind: 'builtin' } : { key: 'custom', label: '自定义', kind: 'custom' })
  if (item.required) {
    tags.push({ key: 'required', label: '必填', kind: 'required' })
  }
  if (item.gitops_locator) {
    tags.push({ key: 'gitops', label: 'GitOps 定位', kind: 'gitops' })
  }
  if (item.cd_self_fill) {
    tags.push({ key: 'cd-self-fill', label: 'CD 自填', kind: 'cd-self-fill' })
  }
  return tags
}

function normalizeParamKey(value: string) {
  return value.trim().toLowerCase()
}

function resetFormState() {
  formState.id = ''
  formState.param_key = ''
  formState.name = ''
  formState.description = ''
  formState.param_type = 'string'
  formState.required = false
  formState.gitops_locator = false
  formState.cd_self_fill = false
  formState.status = 1
}

function toPayload(): PlatformParamDictPayload {
  return {
    param_key: normalizeParamKey(formState.param_key),
    name: formState.name.trim(),
    description: formState.description.trim(),
    param_type: formState.param_type,
    required: formState.required,
    gitops_locator: formState.gitops_locator,
    cd_self_fill: formState.cd_self_fill,
    status: formState.status,
  }
}

async function loadPlatformParams() {
  loading.value = true
  try {
    const response = await listPlatformParamDicts({
      param_key: filters.param_key.trim() || undefined,
      name: filters.name.trim() || undefined,
      status: filters.status === '' ? undefined : filters.status,
      page: filters.page,
      page_size: filters.pageSize,
    })
    dataSource.value = response.data
    total.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字库加载失败'))
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  filters.page = 1
  void loadPlatformParams()
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page
  filters.pageSize = pageSize
  void loadPlatformParams()
}

function openCreateModal() {
  modalMode.value = 'create'
  submitting.value = false
  resetFormState()
  modalVisible.value = true
}

async function openEditModal(record: PlatformParamDict) {
  modalMode.value = 'edit'
  submitting.value = false
  try {
    const response = await getPlatformParamDictByID(record.id)
    const item = response.data
    formState.id = item.id
    formState.param_key = item.param_key
    formState.name = item.name
    formState.description = item.description
    formState.param_type = item.param_type
    formState.required = item.required
    formState.gitops_locator = item.gitops_locator
    formState.cd_self_fill = item.cd_self_fill
    formState.status = item.status
    modalVisible.value = true
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字段详情加载失败'))
  }
}

function closeModal() {
  modalVisible.value = false
}

function handleFormAfterClose() {
  resetFormState()
  formRef.value?.clearValidate()
  submitting.value = false
}

async function submitForm() {
  await formRef.value?.validate()
  submitting.value = true
  try {
    const payload = toPayload()
    if (modalMode.value === 'create') {
      await createPlatformParamDict(payload)
      message.success('标准字段创建成功')
    } else {
      await updatePlatformParamDict(formState.id, payload)
      message.success('标准字段更新成功')
    }
    closeModal()
    await loadPlatformParams()
  } catch (error) {
    message.error(
      extractHTTPErrorMessage(error, modalMode.value === 'create' ? '标准字段创建失败' : '标准字段更新失败'),
    )
  } finally {
    submitting.value = false
  }
}

async function openDetailDrawer(record: PlatformParamDict) {
  detailVisible.value = true
  detailLoading.value = true
  detailData.value = null
  try {
    const response = await getPlatformParamDictByID(record.id)
    detailData.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字段详情加载失败'))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

function closeDetailDrawer() {
  detailVisible.value = false
  detailData.value = null
}

async function handleDelete(record: PlatformParamDict) {
  deletingID.value = record.id
  try {
    await deletePlatformParamDict(record.id)
    message.success('标准字段删除成功')
    if (dataSource.value.length === 1 && filters.page > 1) {
      filters.page -= 1
    }
    await loadPlatformParams()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '标准字段删除失败'))
  } finally {
    deletingID.value = ''
  }
}

function handleParamKeyInput(value: string) {
  formState.param_key = value.toLowerCase()
}

onMounted(() => {
  syncPlatformParamFormViewportInset()
  observePlatformParamFormViewportInset()
  void loadPlatformParams()
})

onBeforeUnmount(() => {
  stopObservingPlatformParamFormViewportInset()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header platform-param-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">字段</h2>
      </div>
      <div class="page-header-actions platform-param-header-actions">
        <a-input
          v-model:value="filters.param_key"
          class="platform-param-toolbar-search"
          allow-clear
          placeholder="标准 Key"
          @press-enter="handleSearch"
        />
        <a-input
          v-model:value="filters.name"
          class="platform-param-toolbar-search"
          allow-clear
          placeholder="字段名称"
          @press-enter="handleSearch"
        />
        <a-select
          v-model:value="filters.status"
          class="platform-param-toolbar-select"
          placeholder="状态"
          :options="statusFilterOptions"
        />
        <a-button class="platform-param-toolbar-query-btn" @click="handleSearch">查询</a-button>
        <a-button class="application-toolbar-action-btn platform-param-create-btn" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增标准字段
        </a-button>
      </div>
    </div>

    <div class="platform-param-table-section">
      <a-table
        class="platform-param-table"
        row-key="id"
        :columns="columns"
        :data-source="dataSource"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 1360 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'param_key'">
            <span class="param-key-cell">
              <span>{{ record.param_key }}</span>
              <a-tooltip v-if="isBuiltinParam(record)" title="系统内置字段">
                <SafetyCertificateOutlined class="builtin-icon" />
              </a-tooltip>
            </span>
          </template>
          <template v-else-if="column.key === 'required'">
            {{ boolText(record.required) }}
          </template>
          <template v-else-if="column.key === 'gitops_locator'">
            {{ boolText(record.gitops_locator) }}
          </template>
          <template v-else-if="column.key === 'cd_self_fill'">
            {{ boolText(record.cd_self_fill) }}
          </template>
          <template v-else-if="column.key === 'abilities'">
            <div class="ability-tags">
              <a-tag
                v-for="tag in abilityTags(record)"
                :key="tag.key"
                class="ability-tag"
                :class="`ability-tag--${tag.kind}`"
              >
                {{ tag.label }}
              </a-tag>
            </div>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space size="small">
              <a-button type="link" size="small" class="table-action-button" @click="openDetailDrawer(record)">查看</a-button>
              <template v-if="!isBuiltinParam(record)">
                <a-button type="link" size="small" class="table-action-button" @click="openEditModal(record)">编辑</a-button>
                <a-popconfirm
                  title="确认删除当前标准字段吗？"
                  ok-text="删除"
                  cancel-text="取消"
                  @confirm="handleDelete(record)"
                >
                  <template #icon>
                    <ExclamationCircleOutlined class="danger-icon" />
                  </template>
                  <a-button type="link" size="small" class="table-action-button table-action-button-danger" danger :loading="deletingID === record.id">删除</a-button>
                </a-popconfirm>
              </template>
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
    </div>

    <a-modal
      :open="modalVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleFormAfterClose"
      :mask-style="platformParamFormMaskStyle"
      :wrap-props="platformParamFormWrapProps"
      wrap-class-name="platform-param-form-modal-wrap"
      @cancel="closeModal"
    >
      <template #title>
        <div class="platform-param-form-modal-titlebar">
          <span class="platform-param-form-modal-title">{{ modalTitle }}</span>
          <a-button class="application-toolbar-action-btn platform-param-form-modal-save-btn" :loading="submitting" @click="submitForm">
            保存
          </a-button>
        </div>
      </template>

      <a-form ref="formRef" :model="formState" layout="vertical" :required-mark="false" class="platform-param-form">
        <div v-if="modalMode === 'create'" class="platform-param-form-note">
          平台手动新增的标准字段默认都是非内置字段
        </div>

        <div class="platform-param-form-panel">
          <div class="platform-param-form-panel-title">基础信息</div>

          <a-form-item
            name="param_key"
            :rules="[
              { required: true, message: '请输入标准 Key' },
              {
                pattern: /^[a-z][a-z0-9_]*$/,
                message: 'param_key 必须为小写字母、数字或下划线，且以字母开头',
              },
            ]"
          >
            <template #label>
              <span class="platform-param-form-label">
                标准 Key
                <a-tag class="platform-param-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input
              :value="formState.param_key"
              allow-clear
              placeholder="例如：branch_name"
              @update:value="handleParamKeyInput"
            />
          </a-form-item>

          <a-form-item name="name" :rules="[{ required: true, message: '请输入字段名称' }]">
            <template #label>
              <span class="platform-param-form-label">
                字段名称
                <a-tag class="platform-param-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="formState.name" allow-clear placeholder="请输入字段名称" />
          </a-form-item>

          <a-form-item label="字段说明" name="description">
            <a-textarea
              v-model:value="formState.description"
              :rows="3"
              allow-clear
              placeholder="请输入字段说明"
            />
          </a-form-item>

          <a-form-item name="param_type" :rules="[{ required: true, message: '请选择字段类型' }]">
            <template #label>
              <span class="platform-param-form-label">
                字段类型
                <a-tag class="platform-param-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.param_type" :options="typeOptions" />
          </a-form-item>
        </div>

        <div class="platform-param-form-panel">
          <div class="platform-param-form-panel-title">字段能力</div>

          <a-form-item label="默认必填" name="required">
            <a-switch v-model:checked="formState.required" checked-children="是" un-checked-children="否" />
          </a-form-item>

          <a-form-item label="GitOps 定位字段" name="gitops_locator">
            <a-switch
              v-model:checked="formState.gitops_locator"
              checked-children="是"
              un-checked-children="否"
            />
          </a-form-item>

          <a-form-item label="CD 自填字段" name="cd_self_fill">
            <a-switch
              v-model:checked="formState.cd_self_fill"
              checked-children="是"
              un-checked-children="否"
            />
          </a-form-item>

          <a-form-item name="status" :rules="[{ required: true, message: '请选择状态' }]">
            <template #label>
              <span class="platform-param-form-label">
                状态
                <a-tag class="platform-param-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.status" :options="statusOptions" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer :open="detailVisible" title="字段说明" width="680" @close="closeDetailDrawer">
      <a-skeleton v-if="detailLoading" active :paragraph="{ rows: 8 }" />
      <template v-else-if="detailData">
        <div class="detail-hero">
          <div class="detail-hero-topline">
            <div class="detail-hero-label">平台字段</div>
            <a-tag :color="statusColor(detailData.status)">{{ statusText(detailData.status) }}</a-tag>
          </div>
          <div class="detail-hero-title">{{ detailData.name }}</div>
          <div v-if="detailData.description" class="detail-hero-description">{{ detailData.description }}</div>
          <div class="detail-hero-facts">
            <div class="detail-hero-fact detail-hero-fact--key">
              <div class="detail-hero-fact-label">
                标准 Key
                <a-tooltip v-if="isBuiltinParam(detailData)" title="系统内置字段">
                  <SafetyCertificateOutlined class="builtin-icon" />
                </a-tooltip>
              </div>
              <div class="detail-hero-fact-value detail-hero-fact-value--code">
                {{ detailData.param_key }}
              </div>
            </div>

            <div class="detail-hero-fact">
              <div class="detail-hero-fact-label">字段类型</div>
              <div class="detail-hero-fact-value">{{ detailData.param_type }}</div>
            </div>

            <div class="detail-hero-fact">
              <div class="detail-hero-fact-label">字段能力</div>
              <div class="ability-tags detail-hero-ability-tags">
                <a-tag
                  v-for="tag in abilityTags(detailData)"
                  :key="tag.key"
                  class="ability-tag"
                  :class="`ability-tag--${tag.kind}`"
                >
                  {{ tag.label }}
                </a-tag>
              </div>
            </div>
          </div>
        </div>

        <a-descriptions :column="1" bordered class="detail-descriptions">
          <a-descriptions-item label="字段 ID">{{ detailData.id }}</a-descriptions-item>
          <a-descriptions-item label="字段类型">{{ detailData.param_type }}</a-descriptions-item>
          <a-descriptions-item label="默认必填">{{ boolText(detailData.required) }}</a-descriptions-item>
          <a-descriptions-item label="GitOps 定位字段">{{ boolText(detailData.gitops_locator) }}</a-descriptions-item>
          <a-descriptions-item label="CD 自填字段">{{ boolText(detailData.cd_self_fill) }}</a-descriptions-item>
          <a-descriptions-item label="创建时间">{{ formatTime(detailData.created_at) }}</a-descriptions-item>
          <a-descriptions-item label="更新时间">{{ formatTime(detailData.updated_at) }}</a-descriptions-item>
        </a-descriptions>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.platform-param-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 0 !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
}

.platform-param-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex: none;
  flex-wrap: nowrap;
  gap: 12px;
  min-width: 0;
}

:deep(.platform-param-toolbar-search.ant-input-affix-wrapper) {
  flex: none;
  width: 176px;
  min-width: 176px;
  height: 42px;
  border-radius: 16px;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #1e3a8a;
  font-weight: 650;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.platform-param-toolbar-search.ant-input) {
  min-width: 180px;
  height: 42px;
  border-radius: 16px;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #1e3a8a;
  font-weight: 650;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.platform-param-toolbar-search.ant-input-affix-wrapper .ant-input) {
  background: transparent !important;
  color: #1e3a8a;
  font-weight: 650;
}

:deep(.platform-param-toolbar-search.ant-input::placeholder) {
  color: rgba(30, 58, 138, 0.38);
  font-weight: 600;
}

:deep(.platform-param-toolbar-select.ant-select) {
  flex: none;
  width: 144px;
  min-width: 144px;
}

:deep(.platform-param-toolbar-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  border-radius: 16px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding: 0 14px !important;
}

:deep(.platform-param-toolbar-select .ant-select-selection-item),
:deep(.platform-param-toolbar-select .ant-select-arrow) {
  color: #1e3a8a;
  font-weight: 650;
}

:deep(.platform-param-toolbar-select .ant-select-selection-placeholder) {
  display: flex;
  align-items: center;
  height: 100%;
  color: rgba(30, 58, 138, 0.38) !important;
}

:deep(.platform-param-toolbar-select .ant-select-selection-search) {
  inset-inline-start: 14px !important;
  inset-inline-end: 14px !important;
  inset-block-start: 0 !important;
  inset-block-end: 0 !important;
}

:deep(.platform-param-toolbar-select .ant-select-selection-search-input) {
  height: 100% !important;
  color: #1e3a8a;
  font-weight: 650;
  line-height: 42px !important;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.platform-param-toolbar-query-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
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
:deep(.platform-param-toolbar-query-btn.ant-btn:hover),
:deep(.platform-param-toolbar-query-btn.ant-btn:focus),
:deep(.platform-param-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
}

.platform-param-table-section {
  margin-top: 24px;
}

.platform-param-table :deep(.ant-table) {
  background: transparent;
}

.platform-param-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.34);
}

.platform-param-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.platform-param-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.platform-param-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: var(--color-text-main);
}

.platform-param-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.platform-param-table :deep(.ant-table-cell-fix-right) {
  background: rgba(255, 255, 255, 0.96) !important;
  box-shadow: -12px 0 24px rgba(15, 23, 42, 0.04);
}

.platform-param-table :deep(.ant-table-thead .ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  box-shadow: none;
}

.danger-icon {
  color: var(--color-danger);
}

.param-key-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.ability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ability-tag {
  margin-inline-end: 0;
  border-radius: 999px;
  border: none;
  padding-inline: 10px;
  font-weight: 600;
}

.ability-tag--builtin {
  background: var(--color-primary-50);
  color: var(--color-primary-600);
}

.ability-tag--custom {
  background: var(--color-bg-subtle);
  color: var(--color-dashboard-800);
}

.ability-tag--required {
  background: #fff7ed;
  color: var(--color-warning);
}

.ability-tag--gitops {
  background: #eef2ff;
  color: #4338ca;
}

.ability-tag--cd-self-fill {
  background: #f5f3ff;
  color: #7c3aed;
}

.builtin-icon {
  color: var(--color-primary-500);
}

.table-action-button {
  padding: 0 6px;
  color: var(--color-dashboard-800);
  font-weight: 600;
}

.table-action-button:hover,
.table-action-button:focus {
  color: var(--color-dashboard-900);
}

.table-action-button-danger,
.table-action-button-danger:hover,
.table-action-button-danger:focus {
  color: var(--color-danger);
}

.platform-param-form-modal-wrap :deep(.ant-modal-content) {
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

.platform-param-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.platform-param-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.platform-param-form-modal-wrap :deep(.ant-modal-title) {
  color: #0f172a;
}

.platform-param-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.platform-param-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.platform-param-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

.platform-param-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

.platform-param-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.platform-param-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.platform-param-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.42), rgba(251, 191, 36, 0.16));
}

.platform-param-form-panel {
  padding: 0;
}

.platform-param-form-panel + .platform-param-form-panel,
.platform-param-form-note + .platform-param-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.platform-param-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.platform-param-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.platform-param-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.platform-param-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.platform-param-form :deep(.ant-input),
.platform-param-form :deep(.ant-select-selector) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.platform-param-form :deep(.ant-input:hover),
.platform-param-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.platform-param-form :deep(.ant-input:focus),
.platform-param-form :deep(.ant-input-focused),
.platform-param-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.platform-param-form :deep(.ant-select-selection-placeholder),
.platform-param-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

.detail-hero {
  margin-bottom: 16px;
  padding: 18px;
  border-radius: 18px;
  background:
    radial-gradient(circle at top left, var(--color-primary-glow), transparent 34%),
    linear-gradient(180deg, var(--color-bg-card) 0%, var(--color-bg-subtle) 100%);
  border: 1px solid var(--color-panel-border);
}

.detail-hero-label {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-text-soft);
}

.detail-hero-topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.detail-hero-title {
  margin-top: 10px;
  color: var(--color-text-main);
  font-size: 24px;
  font-weight: 800;
  line-height: 1.2;
}

.detail-hero-description {
  margin-top: 12px;
  color: var(--color-text-secondary);
  line-height: 1.7;
}

.detail-hero-facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 18px;
}

.detail-hero-fact {
  padding: 14px 16px;
  border-radius: 16px;
  border: 1px solid rgba(203, 213, 225, 0.78);
  background: rgba(255, 255, 255, 0.62);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 20px rgba(15, 23, 42, 0.04);
}

.detail-hero-fact-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.detail-hero-fact-value {
  margin-top: 10px;
  color: var(--color-text-main);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.5;
}

.detail-hero-fact-value--code {
  font-family: 'SFMono-Regular', 'Roboto Mono', 'Source Code Pro', monospace;
  word-break: break-all;
}

.detail-hero-ability-tags {
  margin-top: 10px;
}

.detail-descriptions {
  margin-top: 16px;
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 1120px) {
  .platform-param-page-header {
    flex-direction: column;
    align-items: stretch;
  }

  .platform-param-header-actions {
    justify-content: flex-start;
    flex-wrap: wrap;
  }
}

@media (max-width: 768px) {
  .platform-param-header-actions {
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .detail-hero-facts {
    grid-template-columns: 1fr;
  }

  :deep(.platform-param-toolbar-search.ant-input-affix-wrapper),
  :deep(.platform-param-toolbar-search.ant-input),
  :deep(.platform-param-toolbar-select.ant-select) {
    width: 100%;
    min-width: 0;
  }
}
</style>
