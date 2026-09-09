<script setup lang="ts">
import {
  ArrowLeftOutlined,
  CopyOutlined,
  DeleteOutlined,
  DeploymentUnitOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  EyeOutlined,
  LinkOutlined,
  PlusOutlined,
  RocketOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getApplicationByID } from '../../api/application'
import {
  createPipelineBinding,
  deletePipelineBinding,
  getPipelineBindingByID,
  listPipelineBindings,
  listPipelines,
  updatePipelineBinding,
} from '../../api/pipeline'
import { useAuthStore } from '../../stores/auth'
import type {
  BindingType,
  Pipeline,
  PipelineBinding,
  PipelineProvider,
  PipelineStatus,
  TriggerMode,
} from '../../types/pipeline'
import { extractHTTPErrorMessage } from '../../utils/http-error'

type FormMode = 'create' | 'edit'

interface BindingFormState {
  id: string
  binding_type: BindingType
  provider: PipelineProvider
  pipeline_id: string
  trigger_mode: TriggerMode
  status: PipelineStatus
}

interface BindingModuleItem {
  type: BindingType
  title: string
  description: string
  record: PipelineBinding | null
}

interface ReadonlyFieldItem {
  label: string
  value: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const loading = ref(false)
const dataSource = ref<PipelineBinding[]>([])
const deletingID = ref('')

const detailVisible = ref(false)
const detailLoading = ref(false)
const detailData = ref<PipelineBinding | null>(null)

const formVisible = ref(false)
const formMode = ref<FormMode>('create')
const formSubmitting = ref(false)
const formRef = ref<FormInstance>()

const jenkinsPipelineOptions = ref<Array<{ label: string; value: string }>>([])
const loadingJenkinsPipelines = ref(false)
const jenkinsPipelineKeyword = ref('')

const formState = reactive<BindingFormState>({
  id: '',
  binding_type: 'ci',
  provider: 'jenkins',
  pipeline_id: '',
  trigger_mode: 'manual',
  status: 'active',
})

const applicationID = computed(() => String(route.params.id || ''))
const pageTitle = '管线绑定'
const bindingFormViewportInset = ref(0)
const canManageBinding = computed(() => authStore.hasPermission('pipeline.manage'))
const canViewPipelineParams = computed(() => authStore.hasPermission('pipeline_param.manage'))
const existingBindingTypes = computed(() => new Set(dataSource.value.map((item) => item.binding_type)))
const isUsingJenkins = computed(() => true)
const bindingTypeOptions = computed(() => [
  { label: 'CI 绑定', value: 'ci', disabled: formMode.value === 'create' && existingBindingTypes.value.has('ci') },
  { label: 'CD 绑定', value: 'cd', disabled: formMode.value === 'create' && existingBindingTypes.value.has('cd') },
])
const providerOptions = [{ label: 'Jenkins', value: 'jenkins' }] as const
const triggerModeOptions = [
  { label: '手动触发', value: 'manual' },
  { label: 'Webhook 触发', value: 'webhook' },
] as const
const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
] as const
const bindingModules = computed<BindingModuleItem[]>(() => {
  const ciRecord = dataSource.value.find((item) => item.binding_type === 'ci') || null
  const cdRecord = dataSource.value.find((item) => item.binding_type === 'cd') || null
  return [
    {
      type: 'ci',
      title: 'CI 绑定',
      description: '构建、制品生成与执行参数入口',
      record: ciRecord,
    },
    {
      type: 'cd',
      title: 'CD 绑定',
      description: '交付与部署阶段的执行入口',
      record: cdRecord,
    },
  ]
})

const bindingFormMaskStyle = computed(() => ({
  left: `${bindingFormViewportInset.value}px`,
  width: `calc(100% - ${bindingFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: formVisible.value ? 'auto' : 'none',
}))

const bindingFormWrapProps = computed(() => ({
  style: {
    left: `${bindingFormViewportInset.value}px`,
    width: `calc(100% - ${bindingFormViewportInset.value}px)`,
    pointerEvents: formVisible.value ? 'auto' : 'none',
  },
}))

let bindingFormViewportObserver: ResizeObserver | null = null

function readBindingFormViewportInset() {
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

function syncBindingFormViewportInset() {
  bindingFormViewportInset.value = readBindingFormViewportInset()
}

function observeBindingFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  bindingFormViewportObserver?.disconnect()
  bindingFormViewportObserver = new ResizeObserver(() => {
    syncBindingFormViewportInset()
  })

  if (appLayout) {
    bindingFormViewportObserver.observe(appLayout)
  }
  if (sider) {
    bindingFormViewportObserver.observe(sider)
  }
}

function stopObservingBindingFormViewportInset() {
  bindingFormViewportObserver?.disconnect()
  bindingFormViewportObserver = null
}

function formatTime(value: string | null) {
  if (!value) {
    return '-'
  }
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function goBack() {
  router.back()
}

function providerLabel(provider: PipelineProvider) {
  if (provider === 'argocd') {
    return 'ArgoCD'
  }
  return 'Jenkins'
}

function bindingTypeLabel(type: BindingType) {
  return type === 'cd' ? 'CD 绑定' : 'CI 绑定'
}

function statusLabel(status: PipelineStatus) {
  return status === 'active' ? '启用中' : '已停用'
}

function triggerModeLabel(triggerMode: TriggerMode) {
  return triggerMode === 'webhook' ? 'Webhook' : '手动触发'
}

async function copyPipelineID(value: string) {
  const text = String(value || '').trim()
  if (!text) {
    message.warning('暂无 pipeline_id')
    return
  }

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const input = document.createElement('textarea')
      input.value = text
      input.setAttribute('readonly', 'readonly')
      input.style.position = 'fixed'
      input.style.opacity = '0'
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
    }
    message.success('pipeline_id 已复制')
  } catch {
    message.error('复制失败，请手动复制')
  }
}

function toExecutorParams(record: PipelineBinding) {
  if (record.provider !== 'jenkins') {
    message.info('仅 Jenkins 类型绑定支持查看执行器参数')
    return
  }

  const query: Record<string, string> = {
    application_id: record.application_id,
    binding_type: record.binding_type,
    provider: record.provider,
  }
  if (record.id) {
    query.pipeline_binding_id = record.id
  }
  if (record.pipeline_id) {
    query.pipeline_id = record.pipeline_id
  }
  if (record.name) {
    query.pipeline_name = record.name
  }

  void router.push({
    path: '/components/executor-params',
    query,
  })
}

async function validateApplication() {
  if (!applicationID.value) {
    message.error('缺少应用 ID')
    goBack()
    return
  }
  try {
    await getApplicationByID(applicationID.value)
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用信息加载失败'))
    goBack()
  }
}

async function loadBindings() {
  if (!applicationID.value) {
    return
  }
  loading.value = true
  try {
    const pageSize = 100
    let page = 1
    let totalCount = 0
    const allItems: PipelineBinding[] = []

    do {
      const response = await listPipelineBindings(applicationID.value, {
        page,
        page_size: pageSize,
      })
      totalCount = Number(response.total || 0)
      allItems.push(...response.data)
      page += 1
    } while (allItems.length < totalCount)

    dataSource.value = allItems
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '绑定列表加载失败'))
  } finally {
    loading.value = false
  }
}

function formatJenkinsPipelineLabel(pipeline: Pipeline) {
  const jobFullName = String(pipeline.job_full_name || '').trim()
  const jobName = String(pipeline.job_name || '').trim() || jobFullName
  if (!jobFullName || jobName === jobFullName) {
    return jobName
  }
  return `${jobName} / ${jobFullName}`
}

function upsertJenkinsPipelineOption(option: { label: string; value: string }) {
  const value = String(option.value || '').trim()
  const label = String(option.label || '').trim()
  if (!value || !label) {
    return
  }

  const optionMap = new Map(jenkinsPipelineOptions.value.map((item) => [item.value, item]))
  optionMap.set(value, { value, label })
  jenkinsPipelineOptions.value = Array.from(optionMap.values()).sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'))
}

const currentPipelineOption = computed(() => {
  const pipelineID = String(formState.pipeline_id || '').trim()
  if (!pipelineID) {
    return null
  }
  return jenkinsPipelineOptions.value.find((item) => item.value === pipelineID) || null
})

const formReadonlyFields = computed<ReadonlyFieldItem[]>(() => [
  { label: '绑定类型', value: bindingTypeLabel(formState.binding_type) },
  { label: '提供方', value: providerLabel(formState.provider) },
])

const pipelineFieldHint = computed(() => {
  if (currentPipelineOption.value) {
    return `当前已绑定：${currentPipelineOption.value.label}`
  }
  return '支持按 Jenkins 作业名称搜索并选择需要绑定的管线'
})

async function ensureJenkinsPipelines(force = false, keyword = '') {
  const normalizedKeyword = String(keyword || '').trim()
  if (!force && !normalizedKeyword && jenkinsPipelineOptions.value.length > 0) {
    return
  }
  loadingJenkinsPipelines.value = true
  try {
    const pageSize = 100
    let page = 1
    let totalCount = 0
    const allItems: Pipeline[] = []

    do {
      const response = await listPipelines({
        provider: 'jenkins',
        status: 'active',
        name: normalizedKeyword || undefined,
        page,
        page_size: pageSize,
      })
      totalCount = Number(response.total || 0)
      allItems.push(...response.data)
      page += 1
    } while (allItems.length < totalCount)

    const optionMap = new Map<string, { label: string; value: string }>()
    for (const pipeline of allItems) {
      const value = String(pipeline.id || '').trim()
      if (!value) {
        continue
      }
      optionMap.set(value, {
        value,
        label: formatJenkinsPipelineLabel(pipeline),
      })
    }

    jenkinsPipelineOptions.value = Array.from(optionMap.values()).sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'Jenkins管线加载失败'))
  } finally {
    loadingJenkinsPipelines.value = false
  }
}

function handleJenkinsPipelineSearch(value: string) {
  jenkinsPipelineKeyword.value = String(value || '').trim()
  void ensureJenkinsPipelines(true, jenkinsPipelineKeyword.value)
}

function normalizeFormByRule() {
  if (formState.binding_type === 'ci') {
    formState.provider = 'jenkins'
    return
  }
  formState.provider = 'jenkins'
}

watch(
  () => formState.binding_type,
  () => {
    normalizeFormByRule()
    if (isUsingJenkins.value) {
      void ensureJenkinsPipelines(false, jenkinsPipelineKeyword.value)
    }
  },
)

watch(
  () => formState.provider,
  () => {
    normalizeFormByRule()
    if (isUsingJenkins.value) {
      void ensureJenkinsPipelines(false, jenkinsPipelineKeyword.value)
    }
  },
)

function resetFormState() {
  formState.id = ''
  formState.binding_type = 'ci'
  formState.provider = 'jenkins'
  formState.pipeline_id = ''
  formState.trigger_mode = 'manual'
  formState.status = 'active'
  jenkinsPipelineKeyword.value = ''
}

function openCreateModal(type?: BindingType) {
  const hasCI = existingBindingTypes.value.has('ci')
  const hasCD = existingBindingTypes.value.has('cd')
  if (hasCI && hasCD) {
    message.warning('当前应用已存在 CI 与 CD 绑定，无需重复创建')
    return
  }

  formMode.value = 'create'
  resetFormState()

  if (type && !existingBindingTypes.value.has(type)) {
    formState.binding_type = type
  } else if (hasCI && !hasCD) {
    formState.binding_type = 'cd'
  } else if (!hasCI && hasCD) {
    formState.binding_type = 'ci'
  }

  formVisible.value = true
  syncBindingFormViewportInset()
  void ensureJenkinsPipelines(true, jenkinsPipelineKeyword.value)
}

async function openEditModal(record: PipelineBinding) {
  if (record.provider === 'argocd') {
    message.warning('ArgoCD 已迁移到发布模板中配置，请删除该绑定后在发布模板里启用 ArgoCD')
    return
  }
  formMode.value = 'edit'
  formSubmitting.value = false
  try {
    const response = await getPipelineBindingByID(record.id)
    const item = response.data
    formState.id = item.id
    formState.binding_type = item.binding_type
    formState.provider = item.provider
    formState.pipeline_id = item.pipeline_id || ''
    formState.trigger_mode = item.trigger_mode
    formState.status = item.status
    normalizeFormByRule()
    if (item.pipeline_id) {
      upsertJenkinsPipelineOption({
        value: item.pipeline_id,
        label: String(item.name || item.pipeline_id).trim(),
      })
    }
    syncBindingFormViewportInset()
    formVisible.value = true
    if (isUsingJenkins.value) {
      await ensureJenkinsPipelines(true, jenkinsPipelineKeyword.value)
      if (item.pipeline_id) {
        upsertJenkinsPipelineOption({
          value: item.pipeline_id,
          label: String(item.name || item.pipeline_id).trim(),
        })
      }
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '绑定详情加载失败'))
  }
}

function closeFormModal() {
  formVisible.value = false
}

function handleFormAfterClose() {
  formSubmitting.value = false
  resetFormState()
  formRef.value?.clearValidate()
}

async function submitForm() {
  await formRef.value?.validate()
  normalizeFormByRule()

  const payloadBase = {
    provider: formState.binding_type === 'ci' ? 'jenkins' : formState.provider,
    pipeline_id: isUsingJenkins.value ? formState.pipeline_id.trim() || undefined : undefined,
    trigger_mode: formState.trigger_mode,
    status: formState.status,
  } as const

  formSubmitting.value = true
  try {
    if (formMode.value === 'create') {
      await createPipelineBinding(applicationID.value, {
        binding_type: formState.binding_type,
        ...payloadBase,
      })
      message.success('绑定创建成功')
    } else {
      await updatePipelineBinding(formState.id, payloadBase)
      message.success('绑定更新成功')
    }
    formVisible.value = false
    await loadBindings()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, formMode.value === 'create' ? '绑定创建失败' : '绑定更新失败'))
  } finally {
    formSubmitting.value = false
  }
}

async function openDetailDrawer(record: PipelineBinding) {
  detailVisible.value = true
  detailLoading.value = true
  detailData.value = null
  try {
    const response = await getPipelineBindingByID(record.id)
    detailData.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '绑定详情加载失败'))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

function closeDetailDrawer() {
  detailVisible.value = false
  detailData.value = null
}

async function handleDelete(record: PipelineBinding) {
  deletingID.value = record.id
  try {
    await deletePipelineBinding(record.id)
    message.success('绑定删除成功')
    await loadBindings()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '绑定删除失败'))
  } finally {
    deletingID.value = ''
  }
}

const pipelineFieldRules = [
  {
    validator: async (_rule: unknown, value: string) => {
      if (isUsingJenkins.value && !String(value || '').trim()) {
        return Promise.reject(new Error('请选择 Jenkins 管线'))
      }
      return Promise.resolve()
    },
  },
]

onMounted(async () => {
  syncBindingFormViewportInset()
  observeBindingFormViewportInset()
  await Promise.all([validateApplication(), loadBindings()])
})

onBeforeUnmount(() => {
  stopObservingBindingFormViewportInset()
})
</script>

<template>
  <div class="page-wrapper pipeline-binding-page">
    <div class="page-header binding-page-header">
      <div class="page-header-main">
        <div class="page-header-copy">
          <h2 class="page-title">{{ pageTitle }}</h2>
        </div>
      </div>
      <div class="page-header-actions">
        <a-button v-if="canManageBinding" class="application-toolbar-action-btn" @click="openCreateModal()">
          <template #icon>
            <PlusOutlined />
          </template>
          新增绑定
        </a-button>
        <a-button class="application-toolbar-action-btn" @click="goBack">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          返回
        </a-button>
      </div>
    </div>

    <div class="binding-module-grid" :class="{ 'binding-module-grid-loading': loading }">
      <section
        v-for="module in bindingModules"
        :key="module.type"
        class="binding-module-card"
        :class="[
          `binding-module-card--${module.type}`,
          { 'binding-module-card-empty': !module.record },
        ]"
      >
        <div class="binding-module-head">
          <div class="binding-module-heading">
            <span class="binding-module-icon" :class="`binding-module-icon--${module.type}`">
              <DeploymentUnitOutlined v-if="module.type === 'ci'" />
              <RocketOutlined v-else />
            </span>
            <div class="binding-module-heading-copy">
              <div class="binding-module-title-row">
                <h3 class="binding-module-title">{{ module.title }}</h3>
                <div class="binding-module-title-tags">
                  <a-tag :color="module.record ? 'success' : 'default'" class="binding-module-state-tag">
                    {{ module.record ? '已绑定' : '未绑定' }}
                  </a-tag>
                  <a-tag v-if="module.record" class="binding-module-updated-tag">
                    {{ formatTime(module.record.updated_at) }}
                  </a-tag>
                </div>
              </div>
              <p class="binding-module-description">{{ module.description }}</p>
            </div>
          </div>
        </div>

        <a-skeleton v-if="loading" active :paragraph="{ rows: 5 }" />

        <template v-else-if="module.record">
          <div class="binding-module-summary">
            <div class="binding-module-name">{{ module.record.name || '-' }}</div>
            <div class="binding-module-meta">
              {{ providerLabel(module.record.provider) }} · {{ triggerModeLabel(module.record.trigger_mode) }} ·
              {{ statusLabel(module.record.status) }}
            </div>
          </div>

          <dl class="binding-module-facts">
            <div class="binding-module-fact binding-module-fact--stacked">
              <dt>pipeline_id</dt>
              <dd class="binding-module-meta-copy">
                <code class="binding-module-code">{{ module.record.pipeline_id || '-' }}</code>
                <a-button
                  v-if="module.record.pipeline_id"
                  type="text"
                  size="small"
                  class="binding-module-copy-btn"
                  @click="copyPipelineID(module.record.pipeline_id)"
                >
                  <template #icon>
                    <CopyOutlined />
                  </template>
                </a-button>
              </dd>
            </div>
          </dl>

          <div class="binding-module-footer">
            <div class="binding-module-toolbar">
              <a-button class="binding-module-toolbar-btn" @click="openDetailDrawer(module.record)">
                <template #icon>
                  <EyeOutlined />
                </template>
                查看
              </a-button>
              <a-button v-if="canManageBinding" class="binding-module-toolbar-btn" @click="openEditModal(module.record)">
                <template #icon>
                  <EditOutlined />
                </template>
                编辑
              </a-button>
              <a-button
                v-if="canViewPipelineParams"
                class="binding-module-toolbar-btn"
                :disabled="module.record.provider !== 'jenkins'"
                @click="toExecutorParams(module.record)"
              >
                <template #icon>
                  <SettingOutlined />
                </template>
                执行器参数
              </a-button>
              <a-popconfirm
                v-if="canManageBinding"
                title="确认删除当前绑定吗？"
                ok-text="删除"
                cancel-text="取消"
                @confirm="handleDelete(module.record)"
              >
                <template #icon>
                  <ExclamationCircleOutlined class="danger-icon" />
                </template>
                <a-button class="binding-module-toolbar-btn binding-module-toolbar-btn--danger" danger :loading="deletingID === module.record.id">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                  删除
                </a-button>
              </a-popconfirm>
            </div>
          </div>
        </template>

        <template v-else>
          <a-empty class="binding-module-empty">
            <template #description>
              <div class="binding-module-empty-description">
                <div class="binding-module-empty-title">当前未配置 {{ module.type.toUpperCase() }} 绑定</div>
                <div class="binding-module-empty-text">创建后即可复用该阶段的管线能力</div>
              </div>
            </template>
          </a-empty>
        </template>
      </section>
    </div>

    <a-modal
      :open="formVisible"
      :width="760"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleFormAfterClose"
      :mask-style="bindingFormMaskStyle"
      :wrap-props="bindingFormWrapProps"
      wrap-class-name="binding-form-modal-wrap"
      @cancel="closeFormModal"
    >
      <template #title>
        <div class="binding-form-modal-titlebar">
          <span class="binding-form-modal-title">{{ formMode === 'create' ? '新增绑定' : '编辑绑定' }}</span>
          <a-button class="application-toolbar-action-btn binding-form-modal-save-btn" :loading="formSubmitting" @click="submitForm">
            保存
          </a-button>
        </div>
      </template>
      <a-form ref="formRef" :model="formState" layout="vertical" :required-mark="false" class="binding-form">
        <div v-if="formMode === 'edit'" class="binding-form-note">
          绑定类型和提供方在编辑态下保持只读，如需切换请删除当前绑定后重新创建。
        </div>

        <div v-if="formMode === 'edit'" class="binding-form-panel binding-form-panel--context">
          <div class="binding-form-panel-title">当前绑定</div>
          <div class="binding-form-context">
            <div v-for="item in formReadonlyFields" :key="item.label" class="binding-form-context-item">
              <div class="binding-form-context-label">{{ item.label }}</div>
              <div class="binding-form-context-value">{{ item.value }}</div>
            </div>
          </div>
        </div>

        <div class="binding-form-panel">
          <div class="binding-form-panel-title">{{ formMode === 'create' ? '绑定配置' : '可编辑配置' }}</div>

          <a-form-item
            v-if="formMode === 'create'"
            name="binding_type"
            :rules="[{ required: true, message: '请选择绑定类型' }]"
          >
            <template #label>
              <span class="binding-form-label">
                绑定类型
                <a-tag class="binding-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.binding_type" :options="bindingTypeOptions" placeholder="请选择绑定类型" />
          </a-form-item>

          <a-form-item
            v-if="formMode === 'create'"
            name="provider"
            :rules="[{ required: true, message: '请选择提供方' }]"
          >
            <template #label>
              <span class="binding-form-label">
                提供方
                <a-tag class="binding-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.provider" disabled :options="providerOptions" />
          </a-form-item>

          <a-form-item name="pipeline_id" :rules="pipelineFieldRules" :extra="pipelineFieldHint">
            <template #label>
              <span class="binding-form-label">
                Jenkins 管线
                <a-tag class="binding-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select
              v-model:value="formState.pipeline_id"
              :disabled="!isUsingJenkins"
              allow-clear
              show-search
              :filter-option="false"
              :loading="loadingJenkinsPipelines"
              :options="jenkinsPipelineOptions"
              placeholder="请选择 Jenkins 管线"
              not-found-content="未找到匹配的 Jenkins 管线"
              @search="handleJenkinsPipelineSearch"
            />
          </a-form-item>

          <a-form-item name="trigger_mode" :rules="[{ required: true, message: '请选择触发方式' }]">
            <template #label>
              <span class="binding-form-label">
                触发方式
                <a-tag class="binding-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.trigger_mode" :options="triggerModeOptions" placeholder="请选择触发方式" />
          </a-form-item>

          <a-form-item name="status" :rules="[{ required: true, message: '请选择状态' }]">
            <template #label>
              <span class="binding-form-label">
                绑定状态
                <a-tag class="binding-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-select v-model:value="formState.status" :options="statusOptions" placeholder="请选择绑定状态" />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer :open="detailVisible" title="绑定详情" width="640" @close="closeDetailDrawer">
      <a-skeleton v-if="detailLoading" active :paragraph="{ rows: 8 }" />
      <a-descriptions v-else-if="detailData" :column="1" bordered>
        <a-descriptions-item label="绑定ID">{{ detailData.id }}</a-descriptions-item>
        <a-descriptions-item label="管线名称">{{ detailData.name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用名称">{{ detailData.application_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用ID">{{ detailData.application_id }}</a-descriptions-item>
        <a-descriptions-item label="类型">{{ detailData.binding_type }}</a-descriptions-item>
        <a-descriptions-item label="提供方">{{ detailData.provider }}</a-descriptions-item>
        <a-descriptions-item label="执行器参数">
          <a-button
            type="link"
            class="detail-link-button"
            :disabled="detailData.provider !== 'jenkins'"
            @click="toExecutorParams(detailData)"
          >
            <template #icon>
              <LinkOutlined />
            </template>
            查看执行器参数
          </a-button>
        </a-descriptions-item>
        <a-descriptions-item label="pipeline_id">{{ detailData.pipeline_id || '-' }}</a-descriptions-item>
        <a-descriptions-item label="触发方式">{{ detailData.trigger_mode }}</a-descriptions-item>
        <a-descriptions-item label="状态">{{ detailData.status }}</a-descriptions-item>
        <a-descriptions-item label="创建时间">{{ formatTime(detailData.created_at) }}</a-descriptions-item>
        <a-descriptions-item label="更新时间">{{ formatTime(detailData.updated_at) }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </div>
</template>

<style scoped>
.pipeline-binding-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  margin-top: 0;
  padding-top: 0;
}

.binding-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 0;
  border: none;
  background: transparent;
  box-shadow: none;
}

.page-header-main {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

:deep(.application-toolbar-action-btn.ant-btn),
.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding-inline: 14px;
  font-size: 14px;
  font-weight: 700;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active),
.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn:hover),
.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn:focus),
.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn:focus-visible),
.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn:active) {
  border-color: rgba(59, 130, 246, 0.32) !important;
  background: rgba(239, 246, 255, 0.78) !important;
  color: #0f172a !important;
}

.binding-module-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  width: min(100%, 1120px);
  justify-content: flex-start;
  align-items: stretch;
  margin-top: 0;
}

.binding-module-card {
  display: grid;
  grid-template-rows: auto 1fr auto;
  gap: 16px;
  height: auto;
  min-height: 320px;
  border-radius: 24px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.08), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.08), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 14px 32px rgba(15, 23, 42, 0.04);
  padding: 20px 20px 18px;
  position: relative;
  overflow: hidden;
}

.binding-module-card-empty {
  justify-content: normal;
}

.binding-module-head {
  position: relative;
}

.binding-module-heading {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}

.binding-module-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  font-size: 15px;
  flex: none;
  border: 1px solid rgba(255, 255, 255, 0.82);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.9));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 8px 18px rgba(148, 163, 184, 0.08);
}

.binding-module-icon--ci {
  color: #16a34a;
  border-color: rgba(134, 239, 172, 0.42);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 8px 18px rgba(34, 197, 94, 0.12);
}

.binding-module-icon--cd {
  color: #2563eb;
  border-color: rgba(147, 197, 253, 0.46);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 8px 18px rgba(59, 130, 246, 0.12);
}

.binding-module-heading-copy {
  min-width: 0;
  display: grid;
  gap: 6px;
  flex: 1;
}

.binding-module-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.binding-module-title-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.binding-module-state-tag {
  margin-inline-end: 0;
}

.binding-module-updated-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(248, 250, 252, 0.92);
  color: #64748b;
}

.binding-module-title {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  line-height: 1.2;
  font-weight: 800;
}

.binding-module-description {
  margin: 0;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.binding-module-summary {
  display: grid;
  gap: 6px;
  padding: 14px 16px;
  border-radius: 16px;
  border: 1px solid rgba(191, 219, 254, 0.44);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.9)),
    rgba(255, 255, 255, 0.86);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.8),
    0 8px 18px rgba(148, 163, 184, 0.06);
}

.binding-module-name {
  color: #0f172a;
  font-size: 16px;
  line-height: 1.4;
  font-weight: 700;
  word-break: break-word;
}

.binding-module-meta {
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.binding-module-facts {
  display: grid;
  gap: 10px;
  margin: 0;
}

.binding-module-fact {
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid rgba(226, 232, 240, 0.88);
  background: rgba(255, 255, 255, 0.68);
}

.binding-module-fact--stacked {
  display: grid;
  gap: 6px;
}

.binding-module-fact--inline {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  column-gap: 12px;
}

.binding-module-fact dt {
  color: #64748b;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.binding-module-fact dd {
  margin: 0;
  color: #0f172a;
  font-size: 12px;
  line-height: 1.5;
  font-weight: 600;
  word-break: break-word;
}

.binding-module-fact--inline dd {
  text-align: right;
}

.binding-module-meta-copy {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.binding-module-code {
  display: inline-block;
  max-width: 100%;
  padding: 3px 8px;
  border-radius: 8px;
  background: rgba(241, 245, 249, 0.98);
  color: #0f172a;
  font-family: 'SFMono-Regular', 'SF Mono', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: normal;
  word-break: break-all;
}

.binding-module-copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  min-width: 22px;
  height: 22px;
  padding: 0;
  color: #94a3b8;
}

.binding-module-footer {
  margin-top: auto;
  padding-top: 14px;
  border-top: 1px solid rgba(226, 232, 240, 0.9);
}

.binding-module-toolbar {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  align-items: stretch;
  gap: 8px;
  width: 100%;
}

.binding-module-toolbar > * {
  min-width: 0;
  width: 100%;
}

.binding-module-toolbar :deep(.ant-btn) {
  width: 100%;
  height: 36px;
  justify-content: center;
  padding-inline: 10px;
  white-space: nowrap;
  font-size: 13px;
}

.binding-module-toolbar :deep(.binding-module-toolbar-btn.ant-btn) {
  border-radius: 14px;
}

.binding-module-toolbar :deep(.binding-module-toolbar-btn--danger.ant-btn) {
  color: #dc2626 !important;
  border-color: rgba(248, 113, 113, 0.26) !important;
  background: rgba(255, 255, 255, 0.48) !important;
}

.binding-module-toolbar :deep(.binding-module-toolbar-btn--danger.ant-btn:hover),
.binding-module-toolbar :deep(.binding-module-toolbar-btn--danger.ant-btn:focus),
.binding-module-toolbar :deep(.binding-module-toolbar-btn--danger.ant-btn:focus-visible),
.binding-module-toolbar :deep(.binding-module-toolbar-btn--danger.ant-btn:active) {
  color: #b91c1c !important;
  border-color: rgba(248, 113, 113, 0.34) !important;
  background: rgba(255, 241, 242, 0.72) !important;
}

.binding-module-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 100%;
  margin: 0;
  text-align: center;
}

.binding-module-empty :deep(.ant-empty-description) {
  color: inherit;
  margin-bottom: 0;
}

.binding-module-empty :deep(.ant-empty-image) {
  display: none;
}

.binding-module-empty-description {
  display: grid;
  gap: 6px;
  justify-items: center;
}

.binding-module-empty-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.binding-module-empty-text {
  max-width: 240px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.danger-icon {
  color: #ef4444;
}

.detail-link-button {
  padding-inline: 0;
}

.binding-form-modal-wrap :deep(.ant-modal-content) {
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

.binding-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.binding-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.binding-form-modal-wrap :deep(.ant-modal-title) {
  color: #0f172a;
}

.binding-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.binding-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.binding-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

.binding-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

.binding-form-modal-wrap :deep(.ant-modal-footer) {
  position: relative;
  z-index: 1;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.binding-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.binding-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.binding-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.42), rgba(251, 191, 36, 0.16));
}

.binding-form-panel {
  padding: 0;
}

.binding-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.binding-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.binding-form-note + .binding-form-panel,
.binding-form-panel + .binding-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.binding-form-panel--context {
  padding-bottom: 4px;
}

.binding-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.binding-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.binding-form :deep(.ant-input),
.binding-form :deep(.ant-select-selector) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.binding-form :deep(.ant-input:hover),
.binding-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.binding-form :deep(.ant-input:focus),
.binding-form :deep(.ant-input-focused),
.binding-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.binding-form :deep(.ant-select-disabled .ant-select-selector),
.binding-form :deep(.ant-input[disabled]) {
  background: transparent !important;
  color: #94a3b8 !important;
}

.binding-form :deep(.ant-select-selection-placeholder),
.binding-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

.binding-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.binding-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.binding-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.binding-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

@media (max-width: 1200px) {
  .binding-page-header {
    flex-direction: column;
    align-items: stretch;
  }

  .page-header-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 960px) {
  .binding-module-grid {
    grid-template-columns: 1fr;
    max-width: none;
  }

  .binding-module-card {
    min-height: 320px;
  }
}

@media (max-width: 768px) {
  .binding-module-card {
    padding: 18px;
  }

  .binding-form-context {
    grid-template-columns: 1fr;
  }

  .binding-module-toolbar {
    grid-template-columns: 1fr;
  }

  .binding-module-fact--inline {
    grid-template-columns: 1fr;
    row-gap: 6px;
  }

  .binding-module-fact--inline dd {
    text-align: left;
  }

  .binding-module-toolbar :deep(.ant-btn) {
    min-width: 100%;
  }

  .binding-module-title {
    font-size: 15px;
  }
}
</style>
