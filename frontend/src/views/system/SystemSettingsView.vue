<script setup lang="ts">
import { ExclamationCircleOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { onMounted, reactive, ref } from 'vue'
import { getReleaseSettings, updateReleaseSettings } from '../../api/system'
import { extractHTTPErrorMessage } from '../../utils/http-error'
import AnnouncementManage from './AnnouncementManage.vue'

const activeTab = ref('release')
const announcementRef = ref<InstanceType<typeof AnnouncementManage> | null>(null)
const loading = ref(false)
const saving = ref(false)
const envOptions = ref<string[]>([])
const concurrency = reactive({
  enabled: false,
  lock_scope: 'application_env',
  conflict_strategy: 'reject',
  lock_timeout_sec: 1800,
})

const gitopsConfig = reactive({
  helm_scan_path: 'apps/helm',
  kustomize_scan_path: 'apps/{app_key}/overlays/{env}',
})

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

async function loadSettings() {
  loading.value = true
  try {
    const response = await getReleaseSettings()
    envOptions.value = normalizeEnvOptions(response.data.env_options || [])
    concurrency.enabled = Boolean(response.data.concurrency?.enabled)
    concurrency.lock_scope = response.data.concurrency?.lock_scope || 'application_env'
    concurrency.conflict_strategy = response.data.concurrency?.conflict_strategy || 'reject'
    concurrency.lock_timeout_sec = Number(response.data.concurrency?.lock_timeout_sec || 1800)
    gitopsConfig.helm_scan_path = response.data.gitops_config?.helm_scan_path || 'apps/helm'
    gitopsConfig.kustomize_scan_path = response.data.gitops_config?.kustomize_scan_path || 'apps/{app_key}/overlays/{env}'
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '系统设置加载失败'))
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  const normalized = normalizeEnvOptions(envOptions.value)
  if (normalized.length === 0) {
    message.warning('请至少保留一个发布环境选项')
    return
  }
  saving.value = true
  try {
    const response = await updateReleaseSettings({
      env_options: normalized,
      concurrency: {
        enabled: concurrency.enabled,
        lock_scope: concurrency.lock_scope as 'application' | 'application_env' | 'gitops_repo_branch',
        conflict_strategy: concurrency.conflict_strategy as 'reject' | 'queue',
        lock_timeout_sec: Number(concurrency.lock_timeout_sec || 1800),
      },
      gitops_config: {
        helm_scan_path: gitopsConfig.helm_scan_path.trim() || 'apps/helm',
        kustomize_scan_path: gitopsConfig.kustomize_scan_path.trim() || 'apps/{app_key}/overlays/{env}',
      },
    })
    envOptions.value = normalizeEnvOptions(response.data.env_options || [])
    concurrency.enabled = Boolean(response.data.concurrency?.enabled)
    concurrency.lock_scope = response.data.concurrency?.lock_scope || 'application_env'
    concurrency.conflict_strategy = response.data.concurrency?.conflict_strategy || 'reject'
    concurrency.lock_timeout_sec = Number(response.data.concurrency?.lock_timeout_sec || 1800)
    gitopsConfig.helm_scan_path = response.data.gitops_config?.helm_scan_path || 'apps/helm'
    gitopsConfig.kustomize_scan_path = response.data.gitops_config?.kustomize_scan_path || 'apps/{app_key}/overlays/{env}'
    message.success('系统设置已保存')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '系统设置保存失败'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header">
      <div class="page-header-copy">
        <h2 class="page-title">设置</h2>
      </div>
      <div class="page-header-actions">
        <a-button
          v-if="activeTab === 'release'"
          class="settings-toolbar-action-btn settings-toolbar-action-btn--primary"
          :loading="saving"
          @click="saveSettings"
        >保存</a-button>
        <a-button
          v-if="activeTab === 'announcement'"
          class="settings-toolbar-action-btn settings-toolbar-action-btn--primary"
          @click="announcementRef?.openCreate()"
        >
          <template #icon><PlusOutlined /></template>
          新增公告
        </a-button>
      </div>
    </div>

    <a-tabs v-model:activeKey="activeTab" class="settings-tabs">
      <a-tab-pane key="release" tab="发布设置">
        <a-card :loading="loading" :bordered="false" class="settings-card">
      <template #title>
        发布环境
        <a-popover
          trigger="click"
          placement="rightTop"
          overlay-class-name="release-tip-popover"
        >
          <template #content>
            <div class="release-tip-content">
              <p style="margin:0 0 8px;font-weight:600;">发布单基础字段"环境"会从这里读取下拉选项</p>
              建议按实际环境维护，例如 dev、test、prod。修改后新建发布单页面会直接使用这里的配置
            </div>
          </template>
          <button
            class="release-tip-trigger release-tip-trigger-info"
            type="button"
            aria-label="查看环境配置说明"
          >
            <ExclamationCircleOutlined />
          </button>
        </a-popover>
      </template>
      <a-form layout="vertical">
        <a-form-item label="环境选项">
          <a-select
            v-model:value="envOptions"
            mode="tags"
            :token-separators="[',', '，', ' ']"
            placeholder="输入环境并回车，例如 dev / test / prod"
            style="width: 100%"
          />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :loading="loading" :bordered="false" class="settings-card">
      <template #title>
        并发发布配置
        <a-popover
          trigger="click"
          placement="rightTop"
          overlay-class-name="release-tip-popover"
        >
          <template #content>
            <div class="release-tip-content">
              <p style="margin:0 0 8px;font-weight:600;">配置同一目标的并发发布行为</p>
              启用后，平台会在发布执行前按应用、应用环境或 GitOps 仓库分支加锁；冲突时可直接拒绝，或进入排队等待
            </div>
          </template>
          <button
            class="release-tip-trigger release-tip-trigger-info"
            type="button"
            aria-label="查看并发发布配置说明"
          >
            <ExclamationCircleOutlined />
          </button>
        </a-popover>
      </template>
      <a-form layout="vertical">
        <a-form-item label="启用并发控制">
          <a-switch v-model:checked="concurrency.enabled" />
        </a-form-item>
        <a-form-item label="锁范围">
          <a-select v-model:value="concurrency.lock_scope">
            <a-select-option value="application">按应用</a-select-option>
            <a-select-option value="application_env">按应用 + 环境</a-select-option>
            <a-select-option value="gitops_repo_branch">按 GitOps 仓库分支</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="冲突策略">
          <a-select v-model:value="concurrency.conflict_strategy">
            <a-select-option value="reject">直接拒绝</a-select-option>
            <a-select-option value="queue">进入排队</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="锁超时（秒）">
          <a-input-number v-model:value="concurrency.lock_timeout_sec" :min="30" :max="86400" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :loading="loading" :bordered="false" class="settings-card">
      <template #title>
        GitOps 读取目录
        <a-popover
          trigger="click"
          placement="rightTop"
          overlay-class-name="release-tip-popover"
        >
          <template #content>
            <div class="release-tip-content">
              <p style="margin:0 0 8px;font-weight:600;">配置 GitOps 扫描候选字段时使用的目录路径</p>
              支持占位符：{app_key} 应用标识、{env} 环境。修改后在发布模板页重新同步字段即可生效
            </div>
          </template>
          <button
            class="release-tip-trigger release-tip-trigger-info"
            type="button"
            aria-label="查看 GitOps 目录配置说明"
          >
            <ExclamationCircleOutlined />
          </button>
        </a-popover>
      </template>
      <a-form layout="vertical">
        <a-form-item label="Helm 模式（扫描 Values 文件）">
          <a-input
            v-model:value="gitopsConfig.helm_scan_path"
            placeholder="apps/helm"
            style="max-width: 480px"
          />
        </a-form-item>
        <a-form-item label="Kustomize 模式（扫描 Overlay 目录）">
          <a-input
            v-model:value="gitopsConfig.kustomize_scan_path"
            placeholder="apps/{app_key}/overlays/{env}"
            style="max-width: 480px"
          />
        </a-form-item>
      </a-form>
    </a-card>
        </a-tab-pane>
        <a-tab-pane key="announcement" tab="公告管理">
          <AnnouncementManage ref="announcementRef" />
        </a-tab-pane>
      </a-tabs>
  </div>
</template>

<style scoped>
/* ---- page header ---- */
.page-header-card {
  background: transparent;
  border: none;
  box-shadow: none;
  padding: 0;
}

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

/* ---- header glass button ---- */
.settings-toolbar-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding-inline: 14px;
  font-size: 14px;
  font-weight: 700;
}

.settings-toolbar-action-btn:hover,
.settings-toolbar-action-btn:focus {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.settings-toolbar-action-btn--primary {
  background: linear-gradient(180deg, rgba(241, 247, 255, 0.9), rgba(223, 235, 255, 0.8)) !important;
  border-color: rgba(147, 197, 253, 0.74) !important;
  color: #1d4ed8 !important;
}

.settings-toolbar-action-btn--primary:hover,
.settings-toolbar-action-btn--primary:focus {
  background: linear-gradient(180deg, rgba(248, 251, 255, 0.96), rgba(231, 241, 255, 0.88)) !important;
  border-color: rgba(96, 165, 250, 0.66) !important;
  color: #1e3a8a !important;
}

/* ---- settings cards ---- */
.settings-card {
  border-radius: var(--radius-xl);
  background: var(--color-bg-card);
  border: 1px solid var(--color-panel-border);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.04);
}

.settings-card :deep(.ant-card-head) {
  border-bottom: 1px solid var(--color-panel-border);
  padding: 18px 24px;
}

.settings-card :deep(.ant-card-head-title) {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-main);
}

.settings-card :deep(.ant-card-body) {
  padding: 24px;
}

/* ---- form ---- */
.settings-card :deep(.ant-form-item) {
  margin-bottom: 20px;
}

.settings-card :deep(.ant-form-item:last-child) {
  margin-bottom: 0;
}

.settings-card :deep(.ant-form-item-label > label) {
  font-weight: 500;
  color: var(--color-text-main);
}

.settings-card :deep(.ant-select),
.settings-card :deep(.ant-input-number) {
  max-width: 480px;
}

@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .settings-card :deep(.ant-card-head) {
    padding: 14px 18px;
  }

  .settings-card :deep(.ant-card-body) {
    padding: 18px;
  }
}

@media (max-width: 768px) {
  .settings-card :deep(.ant-select),
  .settings-card :deep(.ant-input-number) {
    max-width: 100%;
  }
}
</style>
