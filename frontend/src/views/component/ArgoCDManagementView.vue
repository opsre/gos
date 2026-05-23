<script setup lang="ts">
import { LeftOutlined, PlusOutlined, QuestionCircleOutlined, RightOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  checkArgoCDInstance,
  createArgoCDInstance,
  listArgoCDEnvBindings,
  listArgoCDInstances,
  updateArgoCDEnvBindings,
  updateArgoCDInstance,
} from '../../api/argocd'
import { listGitOpsInstances } from '../../api/gitops'
import { getReleaseSettings } from '../../api/system'
import { useAuthStore } from '../../stores/auth'
import type {
  ArgoCDEnvBinding,
  ArgoCDInstance,
  ArgoCDRecordStatus,
  UpsertArgoCDInstancePayload,
} from '../../types/argocd'
import type { GitOpsInstance } from '../../types/gitops'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const authStore = useAuthStore()
const router = useRouter()

const loadingInstances = ref(false)
const loadingBindings = ref(false)
const savingBindings = ref(false)
const savingInstance = ref(false)
const checkingInstanceID = ref('')
const instanceModalVisible = ref(false)
const editingInstanceID = ref('')
const instanceModalViewportInset = ref(0)
let instanceModalViewportObserver: ResizeObserver | null = null
const instanceTotal = ref(0)
const instanceDataSource = ref<ArgoCDInstance[]>([])
const gitopsInstanceOptions = ref<GitOpsInstance[]>([])
const envOptions = ref<string[]>([])
const envBindingMap = ref<Record<string, ArgoCDEnvBinding[]>>({})

const instanceFilters = reactive({
  page: 1,
  pageSize: 20,
})

const instanceForm = reactive<UpsertArgoCDInstancePayload>({
  instance_code: '',
  name: '',
  base_url: '',
  insecure_skip_verify: true,
  auth_mode: 'token',
  token: '',
  username: '',
  password: '',
  gitops_instance_id: '',
  cluster_name: '',
  default_namespace: '',
  status: 'active',
  remark: '',
})

const canViewArgoCD = computed(() =>
  [
    'component.argocd.view',
    'component.argocd.manage',
    'component.argocd.instance.view',
    'component.argocd.instance.manage',
    'component.argocd.binding.view',
    'component.argocd.binding.manage',
    ].some((code) => authStore.hasPermission(code)),
)
const canViewInstances = computed(
  () =>
    authStore.hasPermission('component.argocd.instance.view') ||
    authStore.hasPermission('component.argocd.instance.manage') ||
    authStore.hasPermission('component.argocd.binding.view') ||
    authStore.hasPermission('component.argocd.binding.manage') ||
    authStore.hasPermission('component.argocd.view') ||
    authStore.hasPermission('component.argocd.manage'),
)
const canViewBindings = computed(
  () =>
    authStore.hasPermission('component.argocd.binding.view') ||
    authStore.hasPermission('component.argocd.binding.manage') ||
    authStore.hasPermission('component.argocd.instance.view') ||
    authStore.hasPermission('component.argocd.instance.manage') ||
    authStore.hasPermission('component.argocd.view') ||
    authStore.hasPermission('component.argocd.manage'),
)
const canManageInstances = computed(
  () => authStore.hasPermission('component.argocd.instance.manage') || authStore.hasPermission('component.argocd.manage'),
)
const canManageBindings = computed(
  () =>
    authStore.hasPermission('component.argocd.binding.manage') ||
    authStore.hasPermission('component.argocd.instance.manage') ||
    authStore.hasPermission('component.argocd.manage'),
)

const bindingRows = computed(() =>
  envOptions.value.map((envCode) => ({
    env_code: envCode,
    bindings: envBindingMap.value[envCode] || [],
  })),
)

const activeInstanceOptions = computed(() => instanceDataSource.value.filter((item) => item.status === 'active'))

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

function openTutorial() {
  void router.push('/help/gitops')
}

async function refreshVisibleData() {
  const tasks: Array<Promise<unknown>> = []
  if (canViewInstances.value || canViewBindings.value) {
    tasks.push(loadInstances())
  }
  if (canManageInstances.value) {
    tasks.push(loadGitOpsInstances())
  }
  if (canViewBindings.value) {
    tasks.push(loadEnvOptions(), loadBindings())
  }
  await Promise.all(tasks)
}

async function refreshBindings() {
  await Promise.all([loadEnvOptions(), loadBindings()])
}

function formatTime(value?: string | null) {
  const text = String(value || '').trim()
  if (!text) {
    return '-'
  }
  return dayjs(text).format('YYYY-MM-DD HH:mm:ss')
}

function normalizeEnvOptions(values: string[]) {
  const result: string[] = []
  const seen = new Set<string>()
  values.forEach((item) => {
    const value = String(item || '').trim()
    if (!value || seen.has(value)) {
      return
    }
    seen.add(value)
    result.push(value)
  })
  return result
}

function resetInstanceForm() {
  editingInstanceID.value = ''
  instanceForm.instance_code = ''
  instanceForm.name = ''
  instanceForm.base_url = ''
  instanceForm.insecure_skip_verify = true
  instanceForm.auth_mode = 'token'
  instanceForm.token = ''
  instanceForm.username = ''
  instanceForm.password = ''
  instanceForm.gitops_instance_id = ''
  instanceForm.cluster_name = ''
  instanceForm.default_namespace = ''
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

function openCreateInstance() {
  resetInstanceForm()
  instanceModalVisible.value = true
}

function openEditInstance(record: ArgoCDInstance) {
  editingInstanceID.value = record.id
  instanceForm.instance_code = record.instance_code
  instanceForm.name = record.name
  instanceForm.base_url = record.base_url
  instanceForm.insecure_skip_verify = record.insecure_skip_verify
  instanceForm.auth_mode = record.auth_mode || 'token'
  instanceForm.token = ''
  instanceForm.username = record.username || ''
  instanceForm.password = ''
  instanceForm.gitops_instance_id = record.gitops_instance_id || ''
  instanceForm.cluster_name = record.cluster_name || ''
  instanceForm.default_namespace = record.default_namespace || ''
  instanceForm.status = (record.status || 'active') as ArgoCDRecordStatus
  instanceForm.remark = record.remark || ''
  instanceModalVisible.value = true
}

function closeInstanceModal() {
  instanceModalVisible.value = false
  resetInstanceForm()
}

async function loadEnvOptions() {
  try {
    const response = await getReleaseSettings()
    envOptions.value = normalizeEnvOptions(response.data.env_options || [])
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布环境配置加载失败'))
  }
}

async function loadInstances() {
  loadingInstances.value = true
  try {
    const response = await listArgoCDInstances({
      page: instanceFilters.page,
      page_size: instanceFilters.pageSize,
    })
    instanceDataSource.value = response.data
    instanceTotal.value = response.total
    instanceFilters.page = response.page
    instanceFilters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'ArgoCD 实例列表加载失败'))
  } finally {
    loadingInstances.value = false
  }
}

async function loadGitOpsInstances() {
  try {
    const response = await listGitOpsInstances({
      status: 'active',
      page: 1,
      page_size: 200,
    })
    gitopsInstanceOptions.value = response.data || []
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'GitOps 实例列表加载失败'))
  }
}

async function loadBindings() {
  loadingBindings.value = true
  try {
    const response = await listArgoCDEnvBindings()
    const next: Record<string, ArgoCDEnvBinding[]> = {}
    ;(response.data || []).forEach((item) => {
      if (!next[item.env_code]) {
        next[item.env_code] = []
      }
      next[item.env_code].push(item)
    })
    envBindingMap.value = next
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '环境绑定加载失败'))
  } finally {
    loadingBindings.value = false
  }
}

async function handleSaveInstance() {
  savingInstance.value = true
  try {
    if (editingInstanceID.value) {
      await updateArgoCDInstance(editingInstanceID.value, instanceForm)
      message.success('ArgoCD 实例已更新')
    } else {
      await createArgoCDInstance(instanceForm)
      message.success('ArgoCD 实例已创建')
    }
    closeInstanceModal()
    await loadInstances()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, editingInstanceID.value ? 'ArgoCD 实例更新失败' : 'ArgoCD 实例创建失败'))
  } finally {
    savingInstance.value = false
  }
}

async function handleCheckInstance(record: ArgoCDInstance) {
  checkingInstanceID.value = record.id
  try {
    await checkArgoCDInstance(record.id)
    message.success(`实例 ${record.name} 连接检查成功`)
    await loadInstances()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, `实例 ${record.name} 连接检查失败`))
  } finally {
    checkingInstanceID.value = ''
  }
}

function updateBindingValue(envCode: string, instanceIDs?: string[] | string) {
  const next = { ...envBindingMap.value }
  const selectedIDs = Array.isArray(instanceIDs) ? instanceIDs.filter(Boolean) : instanceIDs ? [instanceIDs] : []
  if (selectedIDs.length === 0) {
    delete next[envCode]
  } else {
    const existingByInstance = new Map((next[envCode] || []).map((item) => [item.argocd_instance_id, item]))
    next[envCode] = selectedIDs.map((instanceID, index) => {
      const existing = existingByInstance.get(instanceID)
      const target = instanceDataSource.value.find((item) => item.id === instanceID)
      return {
        id: existing?.id || '',
        env_code: envCode,
        argocd_instance_id: instanceID,
        argocd_instance_code: target?.instance_code || existing?.argocd_instance_code || '',
        argocd_instance_name: target?.name || existing?.argocd_instance_name || '',
        cluster_name: target?.cluster_name || existing?.cluster_name || '',
        priority: index + 1,
        status: 'active',
        created_at: existing?.created_at || '',
        updated_at: existing?.updated_at || '',
      }
    })
  }
  envBindingMap.value = next
}

async function saveBindings() {
  const bindings = bindingRows.value
    .flatMap((item) =>
      item.bindings
        .filter((binding) => binding.argocd_instance_id)
        .map((binding) => ({
          env_code: item.env_code,
          argocd_instance_id: binding.argocd_instance_id,
          status: 'active' as ArgoCDRecordStatus,
        })),
    )
  savingBindings.value = true
  try {
    const response = await updateArgoCDEnvBindings({ bindings })
    const next: Record<string, ArgoCDEnvBinding[]> = {}
    ;(response.data || []).forEach((item) => {
      if (!next[item.env_code]) {
        next[item.env_code] = []
      }
      next[item.env_code].push(item)
    })
    envBindingMap.value = next
    message.success('环境绑定已保存')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '环境绑定保存失败'))
  } finally {
    savingBindings.value = false
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

onMounted(() => {
  if (!canViewArgoCD.value) {
    return
  }
  syncInstanceModalViewportInset()
  observeInstanceModalViewportInset()
  void refreshVisibleData()
})

onUnmounted(() => {
  stopObservingInstanceModalViewportInset()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-header">
      <div class="page-header-copy">
        <div class="page-title">编排</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" title="使用教程" @click="openTutorial">
          <template #icon><QuestionCircleOutlined /></template>
        </a-button>
        <a-button v-if="canManageInstances" class="application-toolbar-action-btn" @click="openCreateInstance">
          <template #icon><PlusOutlined /></template>
          新增实例
        </a-button>
        <a-button v-if="canViewBindings" class="component-toolbar-ghost-btn" @click="refreshBindings">刷新绑定</a-button>
        <a-button v-if="canViewBindings" class="application-toolbar-action-btn" :loading="savingBindings" :disabled="!canManageBindings" @click="saveBindings">
          保存绑定
        </a-button>
      </div>
    </div>

    <div class="argocd-unified-layout">
      <section v-if="canViewInstances" class="argocd-module argocd-module--instances">
        <div class="argocd-module-header">
          <div>
            <div class="argocd-module-kicker">01 · 实例管理</div>
            <h3 class="argocd-module-title">ArgoCD 实例</h3>
          </div>
          <div class="argocd-module-meta">共 {{ instanceTotal }} 个实例</div>
        </div>
        <a-card :bordered="false" class="table-card">
          <a-spin :spinning="loadingInstances">
            <div class="argocd-resource-list argocd-instance-list">
              <article v-for="record in instanceDataSource" :key="record.id" class="argocd-resource-card">
                <div class="argocd-resource-card-head">
                  <div class="argocd-resource-identity">
                    <div class="argocd-resource-title-row">
                      <span class="argocd-resource-title">{{ record.name || '-' }}</span>
                      <a-tag :color="record.status === 'active' ? 'green' : 'default'">{{ record.status }}</a-tag>
                      <a-tag :color="record.health_status === 'healthy' ? 'green' : record.health_status === 'unreachable' ? 'red' : 'default'">
                        {{ record.health_status || 'unknown' }}
                      </a-tag>
                    </div>
                    <div class="argocd-resource-subtitle">{{ record.instance_code || '-' }}</div>
                  </div>
                  <div v-if="canManageInstances" class="argocd-resource-actions">
                    <a-button class="component-row-action-btn" size="small" @click="openEditInstance(record)">编辑</a-button>
                    <a-button
                      class="component-row-action-btn"
                      size="small"
                      :loading="checkingInstanceID === record.id"
                      @click="handleCheckInstance(record)"
                    >
                      连接检查
                    </a-button>
                  </div>
                </div>
                <div class="argocd-resource-grid">
                  <div class="argocd-resource-field">
                    <span>GitOps 实例</span>
                    <strong>{{ record.gitops_instance_name || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>Base URL</span>
                    <strong class="truncate-text" :title="record.base_url">{{ record.base_url || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>集群</span>
                    <strong>{{ record.cluster_name || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>认证方式</span>
                    <strong>{{ record.auth_mode || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>最近检查</span>
                    <strong>{{ formatTime(record.last_check_at) }}</strong>
                  </div>
                </div>
              </article>
              <a-empty v-if="!loadingInstances && instanceDataSource.length === 0" class="argocd-empty" description="暂无 ArgoCD 实例" />
            </div>
          </a-spin>

          <div v-if="instanceTotal > instanceFilters.pageSize" class="argocd-compact-pager">
            <span class="argocd-page-summary">第 {{ instanceFilters.page }} / {{ instanceTotalPages }} 页</span>
            <a-button class="argocd-pager-btn" :disabled="instanceFilters.page <= 1" @click="changeInstancePage(instanceFilters.page - 1)">
              <template #icon><LeftOutlined /></template>
            </a-button>
            <a-button class="argocd-pager-btn" :disabled="instanceFilters.page >= instanceTotalPages" @click="changeInstancePage(instanceFilters.page + 1)">
              <template #icon><RightOutlined /></template>
            </a-button>
          </div>
        </a-card>
      </section>

      <section v-if="canViewBindings" class="argocd-module argocd-module--bindings">
        <div class="argocd-module-header">
          <div>
            <div class="argocd-module-kicker">02 · 环境绑定</div>
            <h3 class="argocd-module-title">发布环境实例</h3>
          </div>
          <div class="argocd-module-meta">共 {{ bindingRows.length }} 个环境</div>
        </div>
        <a-card :bordered="false" :loading="loadingBindings" class="table-card">
          <div class="argocd-binding-list">
            <article v-for="record in bindingRows" :key="record.env_code" class="argocd-binding-row">
              <div class="argocd-binding-env">
                <span>环境</span>
                <strong>{{ record.env_code }}</strong>
              </div>
              <div class="argocd-binding-control">
                <span>ArgoCD 实例</span>
                <a-select
                  mode="multiple"
                  :value="record.bindings.map((item) => item.argocd_instance_id)"
                  allow-clear
                  placeholder="请选择 ArgoCD 实例"
                  :disabled="!canManageBindings"
                  @change="(value) => updateBindingValue(record.env_code, value)"
                >
                  <a-select-option v-for="item in activeInstanceOptions" :key="item.id" :value="item.id">
                    {{ item.name }}<span v-if="item.cluster_name">（{{ item.cluster_name }}）</span>
                  </a-select-option>
                </a-select>
              </div>
              <div class="argocd-binding-current">
                <span>当前绑定</span>
                <span v-if="record.bindings.length" class="argocd-binding-current-list">
                  <a-tag v-for="binding in record.bindings" :key="binding.argocd_instance_id" color="blue">
                    {{ binding.argocd_instance_name || '-' }}<span v-if="binding.cluster_name"> / {{ binding.cluster_name }}</span>
                  </a-tag>
                </span>
                <span v-else>-</span>
              </div>
            </article>
            <a-empty v-if="bindingRows.length === 0" class="argocd-empty" description="暂无发布环境" />
          </div>

          <div class="bottom-actions">
            <span class="muted-text">共 {{ bindingRows.length }} 个环境</span>
          </div>
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
      wrap-class-name="component-instance-modal-wrap argocd-instance-modal-wrap"
      @cancel="closeInstanceModal"
    >
      <template #title>
        <div class="component-instance-modal-titlebar">
          <span class="component-instance-modal-title">{{ editingInstanceID ? '编辑 ArgoCD 实例' : '新增 ArgoCD 实例' }}</span>
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
              <a-input v-model:value="instanceForm.instance_code" :disabled="Boolean(editingInstanceID)" placeholder="例如 prod-cn" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="实例名称">
              <a-input v-model:value="instanceForm.name" placeholder="例如 生产华东集群" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="Base URL">
          <a-input v-model:value="instanceForm.base_url" placeholder="例如 https://argocd.example.com" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="认证方式">
              <a-select v-model:value="instanceForm.auth_mode">
                <a-select-option value="token">token</a-select-option>
                <a-select-option value="password">password</a-select-option>
                <a-select-option value="basic">basic</a-select-option>
                <a-select-option value="session">session</a-select-option>
              </a-select>
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
            <a-form-item label="集群名称">
              <a-input v-model:value="instanceForm.cluster_name" placeholder="例如 华东生产集群" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="默认命名空间">
              <a-input v-model:value="instanceForm.default_namespace" placeholder="选填" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="关联 GitOps 实例">
          <a-select
            v-model:value="instanceForm.gitops_instance_id"
            allow-clear
            placeholder="请选择 GitOps 实例"
          >
            <a-select-option
              v-for="item in gitopsInstanceOptions"
              :key="item.id"
              :value="item.id"
            >
              {{ item.name }}（{{ item.instance_code }}）
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item v-if="instanceForm.auth_mode === 'token'" label="Token">
          <a-input-password v-model:value="instanceForm.token" placeholder="留空则更新时沿用原值" />
        </a-form-item>
        <a-row v-else :gutter="16">
          <a-col :span="12">
            <a-form-item label="用户名">
              <a-input v-model:value="instanceForm.username" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item :label="editingInstanceID ? '密码（留空沿用）' : '密码'">
              <a-input-password v-model:value="instanceForm.password" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item>
          <a-checkbox v-model:checked="instanceForm.insecure_skip_verify">跳过 TLS 证书校验</a-checkbox>
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
  gap: 16px;
  margin-bottom: 16px;
}

.page-header {
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

.argocd-unified-layout {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.argocd-module {
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

.argocd-module-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.argocd-module-kicker {
  color: #2563eb;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
}

.argocd-module-title {
  margin: 4px 0 0;
  color: #0f172a;
  font-size: 18px;
  font-weight: 850;
  line-height: 1.3;
}

.argocd-module-meta {
  flex: none;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(255, 255, 255, 0.54);
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
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

.argocd-resource-list,
.argocd-binding-list {
  display: grid;
  gap: 12px;
}

.argocd-resource-card,
.argocd-binding-row {
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

.argocd-resource-card {
  padding: 16px;
}

.argocd-resource-card-head {
  position: relative;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.argocd-resource-identity {
  min-width: 0;
}

.argocd-resource-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.argocd-resource-title {
  min-width: 0;
  color: #0f172a;
  font-size: 15px;
  font-weight: 850;
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.argocd-resource-subtitle {
  margin-top: 3px;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.argocd-resource-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  flex: none;
}

.argocd-resource-grid {
  position: relative;
  display: grid;
  grid-template-columns: 1.05fr 1.6fr 1fr 0.8fr 1fr;
  gap: 10px;
  margin-top: 14px;
}

.argocd-resource-field {
  min-width: 0;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid rgba(226, 232, 240, 0.76);
  background: rgba(255, 255, 255, 0.5);
}

.argocd-resource-field span,
.argocd-binding-env span,
.argocd-binding-control > span,
.argocd-binding-current > span:first-child {
  display: block;
  margin-bottom: 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.3;
}

.argocd-resource-field strong,
.argocd-binding-env strong,
.argocd-binding-current > span:last-child {
  display: block;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 750;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.argocd-binding-row {
  display: grid;
  grid-template-columns: 0.55fr minmax(260px, 1.4fr) minmax(180px, 0.85fr);
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
}

.argocd-binding-control,
.argocd-binding-current {
  min-width: 0;
}

.argocd-binding-control :deep(.ant-select) {
  width: 100%;
}

.argocd-binding-current-list {
  display: flex !important;
  flex-wrap: wrap;
  gap: 6px;
}

.argocd-empty {
  padding: 24px 0;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.component-toolbar-ghost-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.component-toolbar-ghost-btn.ant-btn) {
  padding-inline: 14px;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.component-toolbar-ghost-btn.ant-btn:hover),
:deep(.component-toolbar-ghost-btn.ant-btn:focus),
:deep(.component-toolbar-ghost-btn.ant-btn:focus-visible) {
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

.muted-text {
  color: var(--ant-color-text-description, #8c8c8c);
}

.argocd-compact-pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.argocd-page-summary {
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

:deep(.argocd-pager-btn.ant-btn) {
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

:deep(.argocd-pager-btn.ant-btn:hover:not(:disabled)),
:deep(.argocd-pager-btn.ant-btn:focus-visible:not(:disabled)) {
  border-color: rgba(37, 99, 235, 0.36) !important;
  color: #1d4ed8 !important;
  box-shadow: 0 10px 20px rgba(37, 99, 235, 0.1);
}

.truncate-text {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bottom-actions {
  margin-top: 16px;
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

  .argocd-module {
    padding: 14px;
  }

  .argocd-module-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .argocd-resource-card-head {
    flex-direction: column;
  }

  .argocd-resource-actions {
    justify-content: flex-start;
  }

  .argocd-resource-grid,
  .argocd-binding-row {
    grid-template-columns: 1fr;
  }

  .component-instance-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
