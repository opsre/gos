<script setup lang="ts">
import { ApartmentOutlined, ArrowLeftOutlined, BranchesOutlined, LinkOutlined, ProfileOutlined, SaveOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getApplicationByID, updateApplication } from '../../api/application'
import { listArtifactRepositories } from '../../api/artifact-repository'
import { listProjects } from '../../api/project'
import { listUserOptions } from '../../api/user'
import type { ApplicationPayload } from '../../types/application'
import { extractHTTPErrorMessage } from '../../utils/http-error'
import ApplicationForm from './ApplicationForm.vue'

interface OwnerOption {
  label: string
  value: string
}

interface ProjectOption {
  label: string
  value: string
}

interface ArtifactRepositoryOption {
  label: string
  value: string
}

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const submitting = ref(false)
const ownerLoading = ref(false)
const projectLoading = ref(false)
const artifactRepositoryLoading = ref(false)
const ownerOptions = ref<OwnerOption[]>([])
const projectOptions = ref<ProjectOption[]>([])
const artifactRepositoryOptions = ref<ArtifactRepositoryOption[]>([])
const initialValues = ref<Partial<ApplicationPayload>>({})
const formRef = ref<InstanceType<typeof ApplicationForm> | null>(null)

const applicationId = computed(() => String(route.params.id || ''))

async function loadOwnerOptions() {
  ownerLoading.value = true
  try {
    const response = await listUserOptions()
    ownerOptions.value = response.data.map((item) => ({
      label: `${item.display_name} (${item.username})`,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '负责人下拉加载失败'))
  } finally {
    ownerLoading.value = false
  }
}

async function loadArtifactRepositoryOptions() {
  artifactRepositoryLoading.value = true
  try {
    const response = await listArtifactRepositories({ page: 1, page_size: 100 })
    artifactRepositoryOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.bucket})`,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '制品库下拉加载失败'))
  } finally {
    artifactRepositoryLoading.value = false
  }
}

async function loadProjectOptions() {
  projectLoading.value = true
  try {
    const response = await listProjects({ page: 1, page_size: 200 })
    projectOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '项目下拉加载失败'))
  } finally {
    projectLoading.value = false
  }
}

async function loadDetail() {
  if (!applicationId.value) {
    message.error('缺少应用 ID')
    void router.push('/applications')
    return
  }

  loading.value = true
  try {
    const response = await getApplicationByID(applicationId.value)
    const app = response.data
    initialValues.value = {
      name: app.name,
      key: app.key,
      project_id: app.project_id,
      repo_url: app.repo_url,
      description: app.description,
      owner_user_id: app.owner_user_id,
      status: app.status,
      artifact_type: app.artifact_type,
      artifact_repository_id: app.artifact_repository_id,
      artifact_directory: app.artifact_directory,
      language: app.language,
      gitops_branch_mappings: app.gitops_branch_mappings || [],
      release_branches: app.release_branches || [],
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '加载应用失败'))
    void router.push('/applications')
  } finally {
    loading.value = false
  }
}

async function handleSubmit(payload: ApplicationPayload) {
  if (!applicationId.value) {
    message.error('缺少应用 ID')
    return
  }

  submitting.value = true
  try {
    await updateApplication(applicationId.value, payload)
    message.success('应用更新成功')
    void router.push('/applications')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用更新失败'))
  } finally {
    submitting.value = false
  }
}

function goBack() {
  void router.push('/applications')
}

function handleSubmitFromToolbar() {
  void formRef.value?.submit()
}

onMounted(async () => {
  await Promise.all([loadOwnerOptions(), loadProjectOptions(), loadArtifactRepositoryOptions(), loadDetail()])
})
</script>

<template>
  <div class="page-wrapper application-edit-page">
    <div class="page-header create-page-header">
      <div class="page-header-main">
        <div class="page-header-copy">
          <h2 class="page-title">编辑应用</h2>
        </div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-action-btn" :loading="submitting" @click="handleSubmitFromToolbar">
          <template #icon>
            <SaveOutlined />
          </template>
          保存修改
        </a-button>
        <a-button class="application-toolbar-action-btn" @click="goBack">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          返回
        </a-button>
      </div>
    </div>

    <a-skeleton v-if="loading" active :paragraph="{ rows: 10 }" />
    <div v-else class="create-layout">
      <div class="create-main">
        <ApplicationForm
          ref="formRef"
          :initial-values="initialValues"
          :owner-options="ownerOptions"
          :project-options="projectOptions"
          :artifact-repository-options="artifactRepositoryOptions"
          :owner-loading="ownerLoading"
          :project-loading="projectLoading"
          :artifact-repository-loading="artifactRepositoryLoading"
          :loading="submitting"
          :show-actions="false"
          surface="plain"
          submit-text="保存修改"
          cancel-text="返回"
          @submit="handleSubmit"
          @cancel="goBack"
        />
      </div>

      <aside class="create-sidebar">
        <section class="create-side-card create-side-process">
          <div class="create-side-card-header">
            <span class="create-side-card-kicker">后续配置流程</span>
          </div>
          <ol class="create-process-list">
            <li class="create-process-item">
              <span class="create-process-index">
                <BranchesOutlined />
              </span>
              <div class="create-process-copy">
                <strong>发布分支</strong>
                <span>配置应用默认发布分支</span>
              </div>
            </li>
            <li class="create-process-item">
              <span class="create-process-index">
                <ApartmentOutlined />
              </span>
              <div class="create-process-copy">
                <strong>GitOps 分支环境映射</strong>
                <span>将分支映射到目标环境</span>
              </div>
            </li>
            <li class="create-process-item">
              <span class="create-process-index">
                <LinkOutlined />
              </span>
              <div class="create-process-copy">
                <strong>管线绑定</strong>
                <span>关联 CI/CD 流水线</span>
              </div>
            </li>
            <li class="create-process-item">
              <span class="create-process-index">
                <ProfileOutlined />
              </span>
              <div class="create-process-copy">
                <strong>发布模板</strong>
                <span>选择或创建发布模板</span>
              </div>
            </li>
          </ol>
        </section>

        <section class="create-side-card create-side-tips">
          <div class="create-side-card-header">
            <span class="create-side-card-kicker">编辑建议（推荐）</span>
            <h3 class="create-side-card-title">修改后建议复核关键项</h3>
          </div>
          <ul class="create-tips-list">
            <li>应用 Key 建议保持稳定，避免影响 GitOps 默认规则</li>
            <li>归属项目、负责人调整后请确认权限范围</li>
            <li>制品类型与语言变更后建议同步校验模板参数</li>
          </ul>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.application-edit-page {
  --pipeline-binding-surface-border: rgba(148, 163, 184, 0.16);
  --pipeline-binding-surface-background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.08), transparent 30%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.08), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  --pipeline-binding-surface-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.88),
    0 14px 32px rgba(15, 23, 42, 0.04);
}

.create-page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
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
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.create-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 28px;
  align-items: start;
}

.create-main {
  min-width: 0;
}

.create-side-card {
  border: 1px solid var(--pipeline-binding-surface-border);
  background: var(--pipeline-binding-surface-background);
  box-shadow: var(--pipeline-binding-surface-shadow);
}

.create-sidebar {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.create-side-card {
  padding: 24px 22px;
  border-radius: 24px;
}

.create-side-card-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
}

.create-side-card-kicker {
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.create-side-card-title {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
}

.create-process-list {
  position: relative;
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.create-process-list::before {
  content: '';
  position: absolute;
  left: 15px;
  top: 10px;
  bottom: 10px;
  width: 2px;
  background: rgba(191, 219, 254, 0.9);
}

.create-process-item {
  position: relative;
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  gap: 12px;
  align-items: flex-start;
}

.create-process-index {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  background: linear-gradient(180deg, #3b82f6, #2563eb);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
  box-shadow: 0 10px 18px rgba(37, 99, 235, 0.18);
}

.create-process-index :deep(.anticon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
}

.create-process-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-top: 2px;
}

.create-process-copy strong {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.create-process-copy span {
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.create-tips-list {
  margin: 0;
  padding-left: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.create-tips-list li {
  position: relative;
  padding-left: 26px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.7;
}

.create-tips-list li::before {
  content: '✓';
  position: absolute;
  left: 0;
  top: 1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  background: rgba(22, 163, 74, 0.14);
  color: #16a34a;
  font-size: 12px;
  font-weight: 800;
}

@media (max-width: 768px) {
  .create-page-header {
    align-items: stretch;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .create-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1200px) {
  .create-layout {
    grid-template-columns: 1fr;
  }
}
</style>
