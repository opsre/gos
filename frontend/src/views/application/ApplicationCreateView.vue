<script setup lang="ts">
import { ApartmentOutlined, ArrowLeftOutlined, BranchesOutlined, LinkOutlined, PlusOutlined, ProfileOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createApplication } from '../../api/application'
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

const router = useRouter()
const submitting = ref(false)
const ownerLoading = ref(false)
const projectLoading = ref(false)
const artifactRepositoryLoading = ref(false)
const ownerOptions = ref<OwnerOption[]>([])
const projectOptions = ref<ProjectOption[]>([])
const artifactRepositoryOptions = ref<ArtifactRepositoryOption[]>([])
const formRef = ref<InstanceType<typeof ApplicationForm> | null>(null)

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
    const response = await listProjects({ page: 1, page_size: 200, status: 'active' })
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

async function handleSubmit(payload: ApplicationPayload) {
  submitting.value = true
  try {
    await createApplication(payload)
    message.success('应用创建成功')
    void router.push('/applications')
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用创建失败'))
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

onMounted(() => {
  void Promise.all([loadOwnerOptions(), loadProjectOptions(), loadArtifactRepositoryOptions()])
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header create-page-header">
      <div class="page-header-main">
        <div class="page-header-copy">
          <h2 class="page-title">新建应用</h2>
        </div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-action-btn" :loading="submitting" @click="handleSubmitFromToolbar">
          <template #icon>
            <PlusOutlined />
          </template>
          完成创建
        </a-button>
        <a-button class="application-toolbar-action-btn" @click="goBack">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          返回
        </a-button>
      </div>
    </div>

    <div class="create-layout">
      <div class="create-main">
        <ApplicationForm
          ref="formRef"
          :owner-options="ownerOptions"
          :project-options="projectOptions"
          :artifact-repository-options="artifactRepositoryOptions"
          :owner-loading="ownerLoading"
          :project-loading="projectLoading"
          :artifact-repository-loading="artifactRepositoryLoading"
          :loading="submitting"
          :show-advanced-config="false"
          :show-actions="false"
          surface="plain"
          submit-text="完成创建"
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
            <span class="create-side-card-kicker">命名规范（推荐）</span>
            <h3 class="create-side-card-title">应用 Key 建议保持稳定</h3>
          </div>
          <ul class="create-tips-list">
            <li>推荐使用中划线分隔，例如 `pay-center`</li>
            <li>避免后续频繁调整 Key，GitOps 默认分支规则会依赖它</li>
            <li>项目、负责人、制品类型建议在创建时一次确认</li>
          </ul>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.create-page-header {
  display: flex;
  align-items: center;
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

.create-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 28px;
  align-items: start;
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

.create-main {
  min-width: 0;
}

.create-sidebar {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.create-side-card {
  padding: 24px 22px;
  border: 1px solid rgba(191, 219, 254, 0.72);
  border-radius: 24px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(243, 247, 255, 0.86));
  box-shadow: 0 14px 28px rgba(148, 163, 184, 0.1);
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

  .create-layout {
    grid-template-columns: 1fr;
  }

  .page-header-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 1200px) {
  .create-layout {
    grid-template-columns: 1fr;
  }
}
</style>
