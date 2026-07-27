<script setup lang="ts">
import { ExclamationCircleOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import {
  createAIModelConfig,
  deleteAIModelConfig,
  getReleaseSettings,
  getSystemManagementSettings,
  listAIModelConfigs,
  setDiagnosisAIModelConfig,
  testAIModelConfig,
  unsetDiagnosisAIModelConfig,
  updateAIModelConfig,
  updateReleaseSettings,
  updateSystemManagementSettings,
} from '../../api/system'
import type { AIModelConfig, ReleaseEnvironmentConfig } from '../../types/system'
import { extractHTTPErrorMessage } from '../../utils/http-error'
import AnnouncementManage from './AnnouncementManage.vue'

const activeTab = ref('release')
const announcementRef = ref<InstanceType<typeof AnnouncementManage> | null>(null)
const loading = ref(false)
const saving = ref(false)
const systemManagementLoading = ref(false)
const systemManagementSaving = ref(false)
const aiModelLoading = ref(false)
const aiModelSaving = ref(false)
const aiModelModalVisible = ref(false)
const editingAIModel = ref<AIModelConfig | null>(null)
const testingAIModelID = ref('')
const settingDiagnosisModelID = ref('')
const aiModelConfigs = ref<AIModelConfig[]>([])
const envConfigs = ref<ReleaseEnvironmentConfig[]>([])
const defaultEnvCode = ref('')
const envConfigModalVisible = ref(false)
const editingEnvCode = ref('')
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

const systemManagementForm = reactive({
  current_site_url: '',
})

const envOptions = computed(() => envConfigs.value.map((item) => item.code))

const envConfigForm = reactive({
  code: '',
  description: '',
})

const aiModelForm = reactive({
  name: '',
  provider: 'openai_compatible',
  base_url: '',
  model: '',
  api_key: '',
  temperature: 0.2,
  max_tokens: 2048,
  timeout_sec: 60,
  enabled: true,
})

function normalizeEnvConfigs(configs: ReleaseEnvironmentConfig[] = [], fallbackOptions: string[] = []) {
  const result: ReleaseEnvironmentConfig[] = []
  const seen = new Set<string>()
  const source = configs.length > 0
    ? configs
    : fallbackOptions.map((item) => ({ code: item, description: '' }))
  source.forEach((item) => {
    const code = String(item?.code || '').trim()
    if (!code || seen.has(code)) {
      return
    }
    seen.add(code)
    result.push({
      code,
      description: String(item?.description || '').trim(),
    })
  })
  return result
}

function syncDefaultEnvCode() {
  if (defaultEnvCode.value && !envOptions.value.includes(defaultEnvCode.value)) {
    defaultEnvCode.value = ''
  }
}

async function loadSettings() {
  loading.value = true
  try {
    const response = await getReleaseSettings()
    envConfigs.value = normalizeEnvConfigs(response.data.env_configs || [], response.data.env_options || [])
    defaultEnvCode.value = response.data.default_env_code || ''
    syncDefaultEnvCode()
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
  const normalized = normalizeEnvConfigs(envConfigs.value)
  if (false && normalized.length === 0) {
    message.warning('请至少保留一个发布环境选项')
    return
  }
  syncDefaultEnvCode()
  saving.value = true
  try {
    const response = await updateReleaseSettings({
      env_options: normalized.map((item) => item.code),
      env_configs: normalized,
      default_env_code: defaultEnvCode.value,
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
    envConfigs.value = normalizeEnvConfigs(response.data.env_configs || [], response.data.env_options || [])
    defaultEnvCode.value = response.data.default_env_code || ''
    syncDefaultEnvCode()
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

async function loadSystemManagementSettings() {
  systemManagementLoading.value = true
  try {
    const response = await getSystemManagementSettings()
    systemManagementForm.current_site_url = response.data.current_site_url || ''
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '系统管理设置加载失败'))
  } finally {
    systemManagementLoading.value = false
  }
}

async function saveSystemManagementSettings() {
  const currentSiteURL = systemManagementForm.current_site_url.trim()
  if (currentSiteURL) {
    try {
      const parsed = new URL(currentSiteURL)
      if (!['http:', 'https:'].includes(parsed.protocol)) {
        throw new Error('unsupported protocol')
      }
    } catch {
      message.warning('请输入有效的 HTTP 或 HTTPS 站点地址')
      return
    }
  }
  systemManagementSaving.value = true
  try {
    const response = await updateSystemManagementSettings({
      current_site_url: currentSiteURL,
    })
    systemManagementForm.current_site_url = response.data.current_site_url || ''
    message.success('系统管理设置已保存')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '系统管理设置保存失败'))
  } finally {
    systemManagementSaving.value = false
  }
}

function resetEnvConfigForm() {
  editingEnvCode.value = ''
  envConfigForm.code = ''
  envConfigForm.description = ''
}

function openEnvCreateModal() {
  resetEnvConfigForm()
  envConfigModalVisible.value = true
}

function openEnvEditModal(item: ReleaseEnvironmentConfig) {
  editingEnvCode.value = item.code
  envConfigForm.code = item.code
  envConfigForm.description = item.description || ''
  envConfigModalVisible.value = true
}

function closeEnvConfigModal() {
  envConfigModalVisible.value = false
  resetEnvConfigForm()
}

function saveEnvConfig() {
  const code = envConfigForm.code.trim()
  const description = envConfigForm.description.trim()
  if (!code) {
    message.warning('请填写环境编码')
    return
  }
  if (!description) {
    message.warning('请填写描述文字')
    return
  }
  const duplicated = envConfigs.value.some((item) => item.code === code && item.code !== editingEnvCode.value)
  if (duplicated) {
    message.warning('环境编码已存在')
    return
  }
  if (editingEnvCode.value) {
    envConfigs.value = normalizeEnvConfigs(envConfigs.value.map((item) => (
      item.code === editingEnvCode.value ? { code, description } : item
    )))
    if (defaultEnvCode.value === editingEnvCode.value) {
      defaultEnvCode.value = code
    }
  } else {
    envConfigs.value = normalizeEnvConfigs([...envConfigs.value, { code, description }])
  }
  closeEnvConfigModal()
}

function removeEnvConfig(item: ReleaseEnvironmentConfig) {
  if (!window.confirm(`确认删除发布环境「${item.code}」？`)) {
    return
  }
  envConfigs.value = envConfigs.value.filter((env) => env.code !== item.code)
  if (defaultEnvCode.value === item.code) {
    defaultEnvCode.value = ''
  }
}

function resetAIModelForm() {
  editingAIModel.value = null
  aiModelForm.name = ''
  aiModelForm.provider = 'openai_compatible'
  aiModelForm.base_url = ''
  aiModelForm.model = ''
  aiModelForm.api_key = ''
  aiModelForm.temperature = 0.2
  aiModelForm.max_tokens = 2048
  aiModelForm.timeout_sec = 60
  aiModelForm.enabled = true
}

async function loadAIModelConfigs() {
  aiModelLoading.value = true
  try {
    const response = await listAIModelConfigs()
    aiModelConfigs.value = response.data || []
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'AI 模型配置加载失败'))
  } finally {
    aiModelLoading.value = false
  }
}

function openAIModelCreateModal() {
  resetAIModelForm()
  aiModelModalVisible.value = true
}

function openAIModelEditModal(item: AIModelConfig) {
  editingAIModel.value = item
  aiModelForm.name = item.name
  aiModelForm.provider = item.provider || 'openai_compatible'
  aiModelForm.base_url = item.base_url
  aiModelForm.model = item.model
  aiModelForm.api_key = ''
  aiModelForm.temperature = Number(item.temperature ?? 0.2)
  aiModelForm.max_tokens = Number(item.max_tokens || 2048)
  aiModelForm.timeout_sec = Number(item.timeout_sec || 60)
  aiModelForm.enabled = Boolean(item.enabled)
  aiModelModalVisible.value = true
}

function closeAIModelModal() {
  aiModelModalVisible.value = false
  resetAIModelForm()
}

async function saveAIModelConfig() {
  if (!aiModelForm.name.trim() || !aiModelForm.base_url.trim() || !aiModelForm.model.trim()) {
    message.warning('请填写模型名称、Base URL 和 Model')
    return
  }
  aiModelSaving.value = true
  try {
    const payload = {
      name: aiModelForm.name.trim(),
      provider: aiModelForm.provider,
      base_url: aiModelForm.base_url.trim(),
      model: aiModelForm.model.trim(),
      api_key: aiModelForm.api_key.trim() || undefined,
      temperature: Number(aiModelForm.temperature ?? 0.2),
      max_tokens: Number(aiModelForm.max_tokens || 2048),
      timeout_sec: Number(aiModelForm.timeout_sec || 60),
      enabled: Boolean(aiModelForm.enabled),
    }
    if (editingAIModel.value) {
      await updateAIModelConfig(editingAIModel.value.id, payload)
    } else {
      await createAIModelConfig(payload)
    }
    message.success('AI 模型配置已保存')
    closeAIModelModal()
    await loadAIModelConfigs()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'AI 模型配置保存失败'))
  } finally {
    aiModelSaving.value = false
  }
}

async function testAIModel(item: AIModelConfig) {
  testingAIModelID.value = item.id
  try {
    await testAIModelConfig(item.id)
    message.success('模型连接测试通过')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '模型连接测试失败'))
  } finally {
    testingAIModelID.value = ''
  }
}

async function setDiagnosisModel(item: AIModelConfig) {
  settingDiagnosisModelID.value = item.id
  try {
    await setDiagnosisAIModelConfig(item.id)
    message.success('已设置为诊断模型')
    await loadAIModelConfigs()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '设置诊断模型失败'))
  } finally {
    settingDiagnosisModelID.value = ''
  }
}

async function unsetDiagnosisModel(item: AIModelConfig) {
  settingDiagnosisModelID.value = item.id
  try {
    await unsetDiagnosisAIModelConfig(item.id)
    message.success('已取消诊断模型设置')
    await loadAIModelConfigs()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '取消诊断模型设置失败'))
  } finally {
    settingDiagnosisModelID.value = ''
  }
}

async function removeAIModel(item: AIModelConfig) {
  if (item.is_diagnosis_model) {
    message.warning('请先设置其他诊断模型后再删除')
    return
  }
  if (!window.confirm(`确认删除 AI 模型配置「${item.name}」？`)) {
    return
  }
  try {
    await deleteAIModelConfig(item.id)
    message.success('AI 模型配置已删除')
    await loadAIModelConfigs()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, 'AI 模型配置删除失败'))
  }
}

onMounted(() => {
  void loadSettings()
  void loadSystemManagementSettings()
  void loadAIModelConfigs()
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
          v-if="activeTab === 'system-management'"
          class="settings-toolbar-action-btn settings-toolbar-action-btn--primary"
          :loading="systemManagementSaving"
          @click="saveSystemManagementSettings"
        >保存</a-button>
        <a-button
          v-if="activeTab === 'announcement'"
          class="settings-toolbar-action-btn settings-toolbar-action-btn--primary"
          @click="announcementRef?.openCreate()"
        >
          <template #icon><PlusOutlined /></template>
          新增公告
        </a-button>
        <a-button
          v-if="activeTab === 'ai-models'"
          class="settings-toolbar-action-btn settings-toolbar-action-btn--primary"
          @click="openAIModelCreateModal"
        >
          <template #icon><PlusOutlined /></template>
          新增模型
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
          <div class="release-env-config-toolbar">
            <div class="release-env-config-copy">
              <strong>发布环境配置</strong>
              <span>新增环境时需要填写编码和描述文字，发布单环境卡片会优先使用这里的描述</span>
            </div>
            <a-button type="primary" ghost @click="openEnvCreateModal">
              <template #icon><PlusOutlined /></template>
              新增环境
            </a-button>
          </div>
          <div v-if="envConfigs.length > 0" class="release-env-config-list">
            <div v-for="item in envConfigs" :key="item.code" class="release-env-config-row">
              <div class="release-env-config-main">
                <span class="release-env-config-code">{{ item.code }}</span>
                <span class="release-env-config-desc">{{ item.description || '未填写描述文字' }}</span>
              </div>
              <a-space class="release-env-config-actions" wrap>
                <a-button size="small" @click="openEnvEditModal(item)">编辑环境</a-button>
                <a-button size="small" danger @click="removeEnvConfig(item)">删除</a-button>
              </a-space>
            </div>
          </div>
          <a-empty v-else description="暂无发布环境">
            <template #description>
              <span>暂无发布环境，请先新增环境并填写描述文字</span>
            </template>
          </a-empty>
        </a-form-item>
        <a-form-item label="默认环境">
          <a-select
            v-model:value="defaultEnvCode"
            placeholder="请选择默认环境"
            allow-clear
            style="width: 100%"
          >
            <a-select-option v-for="env in envOptions" :key="env" :value="env">{{ env }}</a-select-option>
          </a-select>
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
            disabled
            value="正在开发中"
            style="max-width: 480px"
          />
        </a-form-item>
      </a-form>
    </a-card>
        </a-tab-pane>
        <a-tab-pane key="system-management" tab="系统管理">
          <a-card :loading="systemManagementLoading" :bordered="false" class="settings-card">
            <template #title>站点设置</template>
            <a-form layout="vertical">
              <a-form-item label="当前站点URL">
                <a-input
                  v-model:value="systemManagementForm.current_site_url"
                  placeholder="https://gos.example.com"
                  style="max-width: 640px"
                />
                <div class="settings-field-help">
                  填写用户访问当前平台时使用的完整 HTTP 或 HTTPS 地址。
                </div>
              </a-form-item>
            </a-form>
          </a-card>
        </a-tab-pane>
        <a-tab-pane key="announcement" tab="公告管理">
          <AnnouncementManage ref="announcementRef" />
        </a-tab-pane>
        <a-tab-pane key="ai-models" tab="AI 模型">
          <a-card :loading="aiModelLoading" :bordered="false" class="settings-card">
            <template #title>
              大模型配置
              <a-popover
                trigger="click"
                placement="rightTop"
                overlay-class-name="release-tip-popover"
              >
                <template #content>
                  <div class="release-tip-content">
                    维护可用于发布单阶段日志分析的大模型配置。只有被设置为诊断模型的启用配置会被阶段 AI 诊断调用。
                  </div>
                </template>
                <button
                  class="release-tip-trigger release-tip-trigger-info"
                  type="button"
                  aria-label="查看 AI 模型配置说明"
                >
                  <ExclamationCircleOutlined />
                </button>
              </a-popover>
            </template>
            <div v-if="aiModelConfigs.length > 0" class="ai-model-list">
              <div v-for="item in aiModelConfigs" :key="item.id" class="ai-model-row">
                <div class="ai-model-main">
                  <div class="ai-model-title-row">
                    <span class="ai-model-title">{{ item.name }}</span>
                    <a-tag v-if="item.is_diagnosis_model" color="blue">诊断模型</a-tag>
                    <a-tag :color="item.enabled ? 'green' : 'default'">
                      {{ item.enabled ? '启用' : '停用' }}
                    </a-tag>
                  </div>
                  <div class="ai-model-meta">
                    <span>{{ item.provider }}</span>
                    <span>{{ item.model }}</span>
                    <span>{{ item.base_url }}</span>
                    <span>{{ item.api_key_configured ? 'Key 已配置' : 'Key 未配置' }}</span>
                  </div>
                </div>
                <a-space class="ai-model-actions" wrap>
                  <a-button size="small" @click="openAIModelEditModal(item)">编辑</a-button>
                  <a-button
                    size="small"
                    :loading="testingAIModelID === item.id"
                    @click="testAIModel(item)"
                  >测试连接</a-button>
                  <a-button
                    size="small"
                    type="primary"
                    ghost
                    :disabled="!item.enabled || !item.api_key_configured"
                    :loading="settingDiagnosisModelID === item.id"
                    @click="setDiagnosisModel(item)"
                    v-if="!item.is_diagnosis_model"
                  >设置为诊断模型</a-button>
                  <a-button
                    v-else
                    size="small"
                    danger
                    ghost
                    :loading="settingDiagnosisModelID === item.id"
                    @click="unsetDiagnosisModel(item)"
                  >取消诊断模型设置</a-button>
                  <a-button size="small" danger @click="removeAIModel(item)">删除</a-button>
                </a-space>
              </div>
            </div>
            <a-empty v-else description="暂无 AI 模型配置" />
          </a-card>
        </a-tab-pane>
      </a-tabs>

    <a-modal
      :open="envConfigModalVisible"
      :title="editingEnvCode ? '编辑环境' : '新增环境'"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveEnvConfig"
      @cancel="closeEnvConfigModal"
    >
      <a-form layout="vertical">
        <a-form-item label="环境编码">
          <a-input v-model:value="envConfigForm.code" placeholder="例如：dev / test / prod" />
        </a-form-item>
        <a-form-item label="描述文字">
          <a-textarea
            v-model:value="envConfigForm.description"
            :rows="3"
            placeholder="例如：可直接发布，适合日常联调"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      :open="aiModelModalVisible"
      :title="editingAIModel ? '编辑 AI 模型' : '新增 AI 模型'"
      :confirm-loading="aiModelSaving"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveAIModelConfig"
      @cancel="closeAIModelModal"
    >
      <a-form layout="vertical">
        <a-form-item label="模型名称">
          <a-input v-model:value="aiModelForm.name" placeholder="例如：默认诊断模型" />
        </a-form-item>
        <a-form-item label="Provider">
          <a-select v-model:value="aiModelForm.provider">
            <a-select-option value="openai_compatible">OpenAI Compatible</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Base URL">
          <a-input v-model:value="aiModelForm.base_url" placeholder="https://api.example.com/v1" />
        </a-form-item>
        <a-form-item label="Model">
          <a-input v-model:value="aiModelForm.model" placeholder="chat-model" />
        </a-form-item>
        <a-form-item :label="editingAIModel?.api_key_configured ? 'API Key（留空保留原 Key）' : 'API Key'">
          <a-input-password v-model:value="aiModelForm.api_key" autocomplete="new-password" />
        </a-form-item>
        <a-form-item label="Temperature">
          <a-input-number v-model:value="aiModelForm.temperature" :min="0" :max="2" :step="0.1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="Max Tokens">
          <a-input-number v-model:value="aiModelForm.max_tokens" :min="256" :max="32000" style="width: 100%" />
        </a-form-item>
        <a-form-item label="Timeout（秒）">
          <a-input-number v-model:value="aiModelForm.timeout_sec" :min="5" :max="300" style="width: 100%" />
        </a-form-item>
        <a-form-item label="启用">
          <a-switch v-model:checked="aiModelForm.enabled" />
        </a-form-item>
      </a-form>
    </a-modal>
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
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
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

.settings-field-help {
  margin-top: 8px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.release-env-config-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  max-width: 760px;
  margin-bottom: 14px;
}

.release-env-config-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.release-env-config-copy strong {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 800;
}

.release-env-config-copy span,
.release-env-config-desc {
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.release-env-config-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 760px;
}

.release-env-config-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.72);
}

.release-env-config-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.release-env-config-code {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 800;
}

.release-env-config-actions {
  flex-shrink: 0;
}

.ai-model-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ai-model-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 0;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
}

.ai-model-row:last-child {
  border-bottom: none;
}

.ai-model-main {
  min-width: 0;
}

.ai-model-title-row,
.ai-model-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.ai-model-title {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 800;
}

.ai-model-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  margin-top: 8px;
  color: var(--color-text-secondary);
  font-size: 12px;
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

  .release-env-config-toolbar,
  .release-env-config-row {
    align-items: stretch;
    flex-direction: column;
  }

  .release-env-config-actions {
    width: 100%;
  }
}
</style>
