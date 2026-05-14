<script setup lang="ts">
import { ExportOutlined, LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import dayjs from 'dayjs'
import { computed, onMounted, reactive, ref } from 'vue'
import {
  getArgoCDApplicationByID,
  getArgoCDApplicationOriginalLink,
  listArgoCDApplications,
  listArgoCDInstances,
  syncArgoCDApplications,
} from '../../api/argocd'
import { useAuthStore } from '../../stores/auth'
import type { ArgoCDApplication, ArgoCDInstance } from '../../types/argocd'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const authStore = useAuthStore()

const loadingApps = ref(false)
const syncingApps = ref(false)
const loadingInstances = ref(false)
const detailLoading = ref(false)
const detailVisible = ref(false)
const openingOriginalID = ref('')
const appTotal = ref(0)
const appDataSource = ref<ArgoCDApplication[]>([])
const instanceDataSource = ref<ArgoCDInstance[]>([])
const applicationPickerOptions = ref<ArgoCDApplication[]>([])
const detail = ref<ArgoCDApplication | null>(null)

const appFilters = reactive({
  argocd_instance_id: '',
  app_name: '',
  page: 1,
  pageSize: 20,
})

const canViewApplications = computed(
  () =>
    authStore.hasPermission('component.argocd.view') ||
    authStore.hasPermission('component.argocd.manage') ||
    authStore.hasPermission('component.argocd.instance.view') ||
    authStore.hasPermission('component.argocd.instance.manage'),
)
const canManageArgoCD = computed(() => authStore.hasPermission('component.argocd.manage'))

const applicationSelectOptions = computed(() => {
  const instanceID = appFilters.argocd_instance_id
  if (!instanceID) {
    return []
  }
  const result: ArgoCDApplication[] = []
  const seen = new Set<string>()
  ;[...applicationPickerOptions.value, ...appDataSource.value].forEach((item) => {
    const name = String(item.app_name || '').trim()
    if (!name || item.argocd_instance_id !== instanceID || seen.has(name)) {
      return
    }
    seen.add(name)
    result.push(item)
  })
  return result
})

const appTotalPages = computed(() => Math.max(1, Math.ceil(appTotal.value / Math.max(appFilters.pageSize, 1))))

const detailRawMeta = computed(() => {
  const text = String(detail.value?.raw_meta || '').trim()
  if (!text) {
    return '-'
  }
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
})

function formatTime(value?: string | null) {
  const text = String(value || '').trim()
  if (!text) {
    return '-'
  }
  return dayjs(text).format('YYYY-MM-DD HH:mm:ss')
}

async function loadInstances() {
  loadingInstances.value = true
  try {
    const response = await listArgoCDInstances({
      page: 1,
      page_size: 500,
    })
    instanceDataSource.value = response.data || []
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'ArgoCD 实例列表加载失败'))
  } finally {
    loadingInstances.value = false
  }
}

async function loadApplications() {
  if (!appFilters.argocd_instance_id) {
    appDataSource.value = []
    appTotal.value = 0
    appFilters.page = 1
    return
  }
  loadingApps.value = true
  try {
    const response = await listArgoCDApplications({
      argocd_instance_id: appFilters.argocd_instance_id || undefined,
      app_name: appFilters.app_name.trim() || undefined,
      page: appFilters.page,
      page_size: appFilters.pageSize,
    })
    appDataSource.value = response.data
    appTotal.value = response.total
    appFilters.page = response.page
    appFilters.pageSize = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'ArgoCD 应用列表加载失败'))
  } finally {
    loadingApps.value = false
  }
}

async function loadApplicationPickerOptions() {
  try {
    const response = await listArgoCDApplications({
      page: 1,
      page_size: 500,
    })
    applicationPickerOptions.value = response.data || []
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'ArgoCD 应用选项加载失败'))
  }
}

async function handleSyncApplications() {
  syncingApps.value = true
  try {
    const response = await syncArgoCDApplications()
    message.success(`同步完成：共 ${response.data.total} 条（新增 ${response.data.created} / 更新 ${response.data.updated} / 失效 ${response.data.inactivated}）`)
    await Promise.all([loadApplications(), loadApplicationPickerOptions(), loadInstances()])
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'ArgoCD 手动同步失败'))
  } finally {
    syncingApps.value = false
  }
}

function handleApplicationPickerChange(value?: string) {
  appFilters.app_name = String(value || '').trim()
  appFilters.page = 1
  void loadApplications()
}

function handleApplicationInstanceChange(value?: string) {
  appFilters.argocd_instance_id = String(value || '').trim()
  appFilters.app_name = ''
  appFilters.page = 1
  void loadApplications()
}

function changeApplicationPage(page: number) {
  const nextPage = Math.min(Math.max(page, 1), appTotalPages.value)
  if (nextPage === appFilters.page) {
    return
  }
  appFilters.page = nextPage
  void loadApplications()
}

async function openDetail(record: ArgoCDApplication) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = record
  try {
    const response = await getArgoCDApplicationByID(record.id)
    detail.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载 ArgoCD 应用详情失败'))
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  detailVisible.value = false
  detailLoading.value = false
  detail.value = null
}

async function openOriginalLink(record: ArgoCDApplication) {
  openingOriginalID.value = record.id
  try {
    const response = await getArgoCDApplicationOriginalLink(record.id)
    const target = String(response.data.original_link || '').trim()
    if (!target) {
      message.warning('当前应用缺少 ArgoCD 原始链接')
      return
    }
    window.open(target, '_blank', 'noopener,noreferrer')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '打开 ArgoCD 原始链接失败'))
  } finally {
    openingOriginalID.value = ''
  }
}

onMounted(() => {
  if (!canViewApplications.value) {
    return
  }
  void Promise.all([loadInstances(), loadApplicationPickerOptions()])
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-header">
      <div class="page-header-copy">
        <div class="page-title">ArgoCD 应用</div>
      </div>
      <div class="page-header-actions">
        <a-button v-if="canManageArgoCD" class="application-toolbar-action-btn" :loading="syncingApps" @click="handleSyncApplications">
          <template #icon><ReloadOutlined /></template>
          手动同步
        </a-button>
      </div>
    </div>

    <div class="argocd-unified-layout">
      <section class="argocd-module argocd-module--applications">
        <a-card :bordered="false" class="table-card">
          <a-spin :spinning="loadingInstances">
            <div class="argocd-application-filter">
              <a-select
                class="argocd-instance-picker"
                :value="appFilters.argocd_instance_id || undefined"
                allow-clear
                show-search
                option-filter-prop="label"
                placeholder="选择 ArgoCD 实例"
                @change="handleApplicationInstanceChange"
              >
                <a-select-option
                  v-for="item in instanceDataSource"
                  :key="item.id"
                  :value="item.id"
                  :label="`${item.name} ${item.instance_code} ${item.cluster_name}`"
                >
                  {{ item.name }}<span v-if="item.cluster_name"> · {{ item.cluster_name }}</span>
                </a-select-option>
              </a-select>
              <a-select
                class="argocd-application-picker"
                :value="appFilters.app_name || undefined"
                allow-clear
                show-search
                :disabled="!appFilters.argocd_instance_id"
                option-filter-prop="label"
                placeholder="先选择实例后选择应用"
                @change="handleApplicationPickerChange"
              >
                <a-select-option
                  v-for="item in applicationSelectOptions"
                  :key="`${item.argocd_instance_id}-${item.app_name}`"
                  :value="item.app_name"
                  :label="item.app_name"
                >
                  {{ item.app_name }}<span v-if="item.instance_name"> · {{ item.instance_name }}</span>
                </a-select-option>
              </a-select>
            </div>
          </a-spin>

          <a-spin :spinning="loadingApps">
            <div class="argocd-resource-list argocd-application-list">
              <article v-for="record in appDataSource" :key="record.id" class="argocd-resource-card argocd-application-card">
                <div class="argocd-resource-card-head">
                  <div class="argocd-resource-identity">
                    <div class="argocd-resource-title-row">
                      <span class="argocd-resource-title">{{ record.app_name || '-' }}</span>
                      <a-tag>{{ record.project || 'default' }}</a-tag>
                      <a-tag :color="String(record.sync_status || '').toLowerCase() === 'synced' ? 'green' : String(record.sync_status || '').toLowerCase() === 'outofsync' ? 'orange' : 'default'">
                        {{ record.sync_status || 'Unknown' }}
                      </a-tag>
                      <a-tag :color="String(record.health_status || '').toLowerCase() === 'healthy' ? 'green' : String(record.health_status || '').toLowerCase() === 'degraded' ? 'red' : String(record.health_status || '').toLowerCase() === 'progressing' ? 'blue' : 'default'">
                        {{ record.health_status || 'Unknown' }}
                      </a-tag>
                    </div>
                    <div class="argocd-resource-subtitle">{{ record.instance_name || '-' }} / {{ record.cluster_name || '-' }}</div>
                  </div>
                  <div class="argocd-resource-actions">
                    <a-button class="component-row-action-btn" size="small" @click="openDetail(record)">详情</a-button>
                    <a-button class="component-row-action-btn" size="small" :loading="openingOriginalID === record.id" @click="openOriginalLink(record)">
                      <template #icon><ExportOutlined /></template>
                      原始链接
                    </a-button>
                  </div>
                </div>
                <div class="argocd-resource-grid argocd-application-grid">
                  <div class="argocd-resource-field">
                    <span>Repo 地址</span>
                    <strong class="truncate-text" :title="record.repo_url">{{ record.repo_url || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>Source Path</span>
                    <strong class="truncate-text" :title="record.source_path">{{ record.source_path || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>目标 Namespace</span>
                    <strong>{{ record.dest_namespace || '-' }}</strong>
                  </div>
                  <div class="argocd-resource-field">
                    <span>最后同步时间</span>
                    <strong>{{ formatTime(record.last_synced_at) }}</strong>
                  </div>
                </div>
              </article>
              <a-empty
                v-if="!loadingApps && appDataSource.length === 0"
                class="argocd-empty"
                :description="appFilters.argocd_instance_id ? '暂无 ArgoCD 应用' : '请选择 ArgoCD 实例后查看应用'"
              />
            </div>
          </a-spin>

          <div v-if="appTotal > appFilters.pageSize" class="argocd-compact-pager">
            <span class="argocd-page-summary">第 {{ appFilters.page }} / {{ appTotalPages }} 页</span>
            <a-button class="argocd-pager-btn" :disabled="appFilters.page <= 1" @click="changeApplicationPage(appFilters.page - 1)">
              <template #icon><LeftOutlined /></template>
            </a-button>
            <a-button class="argocd-pager-btn" :disabled="appFilters.page >= appTotalPages" @click="changeApplicationPage(appFilters.page + 1)">
              <template #icon><RightOutlined /></template>
            </a-button>
          </div>
        </a-card>
      </section>
    </div>

    <a-drawer :open="detailVisible" width="720" title="ArgoCD 应用详情" @close="closeDetail">
      <a-spin :spinning="detailLoading">
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="实例">{{ detail?.instance_name || '-' }}</a-descriptions-item>
          <a-descriptions-item label="集群">{{ detail?.cluster_name || '-' }}</a-descriptions-item>
          <a-descriptions-item label="应用名称">{{ detail?.app_name || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Project">{{ detail?.project || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Repo地址">{{ detail?.repo_url || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Source Path">{{ detail?.source_path || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Target Revision">{{ detail?.target_revision || '-' }}</a-descriptions-item>
          <a-descriptions-item label="目标集群">{{ detail?.dest_server || '-' }}</a-descriptions-item>
          <a-descriptions-item label="目标Namespace">{{ detail?.dest_namespace || '-' }}</a-descriptions-item>
          <a-descriptions-item label="同步状态">{{ detail?.sync_status || '-' }}</a-descriptions-item>
          <a-descriptions-item label="健康状态">{{ detail?.health_status || '-' }}</a-descriptions-item>
          <a-descriptions-item label="操作阶段">{{ detail?.operation_phase || '-' }}</a-descriptions-item>
          <a-descriptions-item label="最后同步时间">{{ formatTime(detail?.last_synced_at) }}</a-descriptions-item>
          <a-descriptions-item label="原始Meta">
            <pre class="raw-meta">{{ detailRawMeta }}</pre>
          </a-descriptions-item>
        </a-descriptions>
      </a-spin>
    </a-drawer>
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

.argocd-application-filter {
  display: grid;
  grid-template-columns: minmax(240px, 320px) minmax(280px, 420px);
  gap: 10px;
  margin-bottom: 12px;
}

.argocd-instance-picker,
.argocd-application-picker {
  width: 100%;
}

.argocd-resource-list {
  display: grid;
  gap: 12px;
}

.argocd-resource-card {
  position: relative;
  overflow: visible;
  padding: 16px;
  border-radius: 18px;
  border: 1px solid rgba(203, 213, 225, 0.74);
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.78), rgba(248, 250, 252, 0.5)),
    radial-gradient(circle at 0 0, rgba(34, 197, 94, 0.08), transparent 32%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.76),
    0 14px 28px rgba(15, 23, 42, 0.05);
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
  grid-template-columns: minmax(220px, 1.4fr) minmax(160px, 1fr) minmax(120px, 0.7fr) minmax(150px, 0.8fr);
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

.argocd-resource-field span {
  display: block;
  margin-bottom: 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.3;
}

.argocd-resource-field strong {
  display: block;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 750;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.argocd-empty {
  padding: 24px 0;
}

:deep(.application-toolbar-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  padding-inline: 14px;
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

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible) {
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

.raw-meta {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
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

  .argocd-application-filter,
  .argocd-resource-grid {
    grid-template-columns: 1fr;
  }

  .argocd-resource-card-head {
    flex-direction: column;
  }

  .argocd-resource-actions {
    justify-content: flex-start;
  }
}
</style>
