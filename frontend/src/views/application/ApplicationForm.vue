<script setup lang="ts">
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons-vue'
import type { FormInstance, Rule } from 'ant-design-vue/es/form'
import { computed, reactive, ref, watch } from 'vue'
import type { ApplicationPayload, GitOpsBranchMapping, ReleaseBranchOption } from '../../types/application'

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

interface ApplicationFormModel {
  name: string
  key: string
  project_id: string
  repo_url: string
  description: string
  owner_user_id: string
  status: ApplicationPayload['status']
  artifact_type: string
  artifact_repository_id: string
  artifact_directory: string
  language: string
  gitops_branch_mappings: GitOpsBranchMapping[]
  release_branches: ReleaseBranchOption[]
}

const props = withDefaults(
  defineProps<{
    initialValues?: Partial<ApplicationPayload>
    ownerOptions?: OwnerOption[]
    projectOptions?: ProjectOption[]
    artifactRepositoryOptions?: ArtifactRepositoryOption[]
    ownerLoading?: boolean
    projectLoading?: boolean
    artifactRepositoryLoading?: boolean
    loading?: boolean
    submitText?: string
    cancelText?: string
    showActions?: boolean
    showAdvancedConfig?: boolean
    surface?: 'card' | 'plain'
  }>(),
  {
    initialValues: () => ({}),
    ownerOptions: () => [],
    projectOptions: () => [],
    artifactRepositoryOptions: () => [],
    ownerLoading: false,
    projectLoading: false,
    artifactRepositoryLoading: false,
    loading: false,
    submitText: '保存',
    cancelText: '取消',
    showActions: true,
    showAdvancedConfig: true,
    surface: 'card',
  },
)

const emit = defineEmits<{
  (e: 'submit', payload: ApplicationPayload): void
  (e: 'cancel'): void
}>()

const formRef = ref<FormInstance>()


const submitHovered = ref(false)
const cancelHovered = ref(false)

const glassPrimaryButtonStyle = computed(() => ({
  background: submitHovered.value
    ? 'linear-gradient(180deg, rgba(248, 251, 255, 0.96), rgba(231, 241, 255, 0.88))'
    : 'linear-gradient(180deg, rgba(241, 247, 255, 0.9), rgba(223, 235, 255, 0.8))',
  borderColor: submitHovered.value ? 'rgba(96, 165, 250, 0.66)' : 'rgba(147, 197, 253, 0.74)',
  color: submitHovered.value ? '#1e3a8a' : '#1d4ed8',
  boxShadow: submitHovered.value
    ? 'inset 0 1px 0 rgba(255, 255, 255, 0.96), 0 12px 26px rgba(59, 130, 246, 0.12)'
    : 'inset 0 1px 0 rgba(255, 255, 255, 0.92), 0 10px 22px rgba(15, 23, 42, 0.06)',
  backdropFilter: 'blur(16px) saturate(145%)',
  borderRadius: '14px',
  transform: submitHovered.value ? 'translateY(-1px)' : 'none',
  transition: 'background 0.18s ease, border-color 0.18s ease, color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease',
}))

const glassSecondaryButtonStyle = computed(() => ({
  background: cancelHovered.value
    ? 'linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(241, 245, 249, 0.72))'
    : 'linear-gradient(180deg, rgba(255, 255, 255, 0.74), rgba(248, 250, 252, 0.6))',
  borderColor: cancelHovered.value ? 'rgba(147, 197, 253, 0.46)' : 'rgba(255, 255, 255, 0.58)',
  color: cancelHovered.value ? 'var(--color-dashboard-900)' : 'var(--color-dashboard-800)',
  boxShadow: cancelHovered.value
    ? 'inset 0 1px 0 rgba(255, 255, 255, 0.96), 0 12px 24px rgba(15, 23, 42, 0.06)'
    : 'inset 0 1px 0 rgba(255, 255, 255, 0.92), 0 10px 22px rgba(15, 23, 42, 0.06)',
  backdropFilter: 'blur(16px) saturate(145%)',
  borderRadius: '14px',
  transform: cancelHovered.value ? 'translateY(-1px)' : 'none',
  transition: 'background 0.18s ease, border-color 0.18s ease, color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease',
}))

const artifactTypeOptions = [
  { label: 'docker-image', value: 'docker-image' },
  { label: 'binary', value: 'binary' },
  { label: 'jar', value: 'jar' },
]

const languageOptions = [
  { label: 'golang', value: 'golang' },
  { label: 'java', value: 'java' },
  { label: 'nodejs', value: 'nodejs' },
  { label: 'python', value: 'python' },
]

const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
]

const model = reactive<ApplicationFormModel>({
  name: '',
  key: '',
  project_id: '',
  repo_url: '',
  description: '',
  owner_user_id: '',
  status: 'active',
  artifact_type: '',
  artifact_repository_id: '',
  artifact_directory: '',
  language: '',
  gitops_branch_mappings: [],
  release_branches: [],
})

const rules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入应用名称', trigger: 'blur' }],
  key: [{ required: true, message: '请输入应用 Key', trigger: 'blur' }],
  project_id: [{ required: true, message: '请选择归属项目', trigger: 'change' }],
  owner_user_id: [{ required: true, message: '请选择负责人', trigger: 'change' }],
  status: [{ required: true, message: '请选择状态', trigger: 'change' }],
  artifact_type: [{ required: true, message: '请选择制品类型', trigger: 'change' }],
  artifact_directory: [{ validator: validateArtifactDirectory, trigger: 'blur' }],
  language: [{ required: true, message: '请选择语言', trigger: 'change' }],
}

function validateArtifactDirectory(_rule: Rule, value: string) {
  const repositoryID = model.artifact_repository_id.trim()
  const directory = String(value || '').trim()
  if (!repositoryID && directory) {
    return Promise.reject('请先选择制品库')
  }
  if (repositoryID && !directory) {
    return Promise.reject('请输入制品路径')
  }
  return Promise.resolve()
}

watch(
  () => props.initialValues,
  (values) => {
    model.name = values.name ?? ''
    model.key = values.key ?? ''
    model.project_id = values.project_id ?? ''
    model.repo_url = values.repo_url ?? ''
    model.description = values.description ?? ''
    model.owner_user_id = values.owner_user_id ?? ''
    model.status = values.status ?? 'active'
    model.artifact_type = values.artifact_type ?? ''
    model.artifact_repository_id = values.artifact_repository_id ?? ''
    model.artifact_directory = values.artifact_directory === '/' ? '' : values.artifact_directory ?? ''
    model.language = values.language ?? ''
    model.gitops_branch_mappings = Array.isArray(values.gitops_branch_mappings)
      ? values.gitops_branch_mappings.map((item) => ({
          env_code: item.env_code ?? '',
          branch: item.branch ?? '',
        }))
      : []
    model.release_branches = Array.isArray(values.release_branches)
      ? values.release_branches.map((item) => ({
          name: item.name ?? '',
          branch: item.branch ?? '',
        }))
      : []
  },
  { immediate: true, deep: true },
)

async function handleSubmit() {
  try {
    await formRef.value?.validate()
    const artifactRepositoryID = model.artifact_repository_id.trim()
    emit('submit', {
      ...model,
      artifact_repository_id: artifactRepositoryID,
      artifact_directory: artifactRepositoryID ? normalizeArtifactDirectory(model.artifact_directory) : '',
    })
  } catch {
    // 校验失败由表单项自身提示
  }
}

function handleCancel() {
  emit('cancel')
}

function normalizeArtifactDirectory(value: string) {
  const raw = String(value || '').trim().replace(/^\/+|\/+$/g, '')
  return raw || '/'
}

function handleArtifactRepositoryChange(value: string | undefined) {
  if (!value) {
    model.artifact_directory = ''
    formRef.value?.clearValidate(['artifact_directory'])
  }
}

defineExpose({
  submit: handleSubmit,
  cancel: handleCancel,
})

function addGitOpsBranchMapping() {
  model.gitops_branch_mappings.push({
    env_code: '',
    branch: '',
  })
}

function removeGitOpsBranchMapping(index: number) {
  model.gitops_branch_mappings.splice(index, 1)
}

function addReleaseBranch() {
  model.release_branches.push({
    name: '',
    branch: '',
  })
}

function removeReleaseBranch(index: number) {
  model.release_branches.splice(index, 1)
}
</script>

<template>
  <a-form
    ref="formRef"
    layout="vertical"
    :model="model"
    :rules="rules"
    class="application-form"
    :class="`application-form-${surface}`"
    autocomplete="off"
    :required-mark="false"
  >
        <section class="form-section">
      <div class="form-section-heading">
        <span class="form-section-bar"></span>
        <h3 class="form-section-heading-title">基础信息</h3>
      </div>

      <a-row :gutter="12" class="form-row-compact">
        <a-col :xs="24" :md="12">
          <a-form-item class="form-item-compact" name="name">
            <template #label>
              <span class="field-label-with-hint">应用名称 <span class="field-required-hint">必填</span></span>
            </template>
            <a-input v-model:value="model.name" :maxlength="64" />
          </a-form-item>
        </a-col>
        <a-col :xs="24" :md="12">
          <a-form-item class="form-item-compact form-item-key" name="key">
            <template #label>
              <span class="field-label-with-hint">应用 Key <span class="field-required-hint">必填</span></span>
            </template>
            <a-input v-model:value="model.key" :maxlength="64" />
          </a-form-item>
        </a-col>
      </a-row>
    </section>

    <section class="form-section form-section-divided">
      <div class="form-section-heading">
        <span class="form-section-bar"></span>
        <h3 class="form-section-heading-title">归属信息</h3>
      </div>

      <a-row :gutter="12" class="form-row-compact">
        <a-col :xs="24" :md="8">
          <a-form-item class="form-item-compact form-item-project" name="project_id">
            <template #label>
              <span class="field-label-with-hint">归属项目 <span class="field-required-hint">必填</span></span>
            </template>
            <a-select
              v-model:value="model.project_id"
              show-search
              option-filter-prop="label"
              :options="projectOptions"
              :loading="projectLoading"
            />
          </a-form-item>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-form-item class="form-item-compact form-item-owner" name="owner_user_id">
            <template #label>
              <span class="field-label-with-hint">负责人 <span class="field-required-hint">必填</span></span>
            </template>
            <a-select
              v-model:value="model.owner_user_id"
              show-search
              option-filter-prop="label"
              :options="ownerOptions"
              :loading="ownerLoading"
            />
          </a-form-item>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-form-item name="status" class="form-item-compact form-item-status">
            <template #label>
              <span class="field-label-with-hint">状态 <span class="field-required-hint">必填</span></span>
            </template>
            <a-select v-model:value="model.status" :options="statusOptions" />
          </a-form-item>
        </a-col>
      </a-row>
    </section>

    <section class="form-section form-section-divided">
      <div class="form-section-heading">
        <span class="form-section-bar"></span>
        <h3 class="form-section-heading-title">配置属性</h3>
      </div>

      <a-row :gutter="12" class="form-row-compact form-row-pair">
        <a-col :xs="24" :md="12">
          <a-form-item name="artifact_type" class="form-item-compact form-item-artifact">
            <template #label>
              <span class="field-label-with-hint">制品类型 <span class="field-required-hint">必填</span></span>
            </template>
            <a-select
              v-model:value="model.artifact_type"
              :options="artifactTypeOptions"
            />
          </a-form-item>
        </a-col>
        <a-col :xs="24" :md="12">
          <a-form-item name="language" class="form-item-compact form-item-language">
            <template #label>
              <span class="field-label-with-hint">语言 <span class="field-required-hint">必填</span></span>
            </template>
            <a-select v-model:value="model.language" :options="languageOptions" />
          </a-form-item>
        </a-col>
      </a-row>

      <a-row :gutter="12" class="form-row-compact form-row-artifact-binding">
        <a-col :xs="24" :md="12">
          <a-form-item name="artifact_repository_id" class="form-item-compact form-item-artifact-repository">
            <template #label>
              <span class="field-label-with-hint">制品库</span>
            </template>
            <a-select
              v-model:value="model.artifact_repository_id"
              show-search
              allow-clear
              option-filter-prop="label"
              :options="artifactRepositoryOptions"
              :loading="artifactRepositoryLoading"
              placeholder="可选，选择已配置制品库"
              @change="handleArtifactRepositoryChange"
            />
          </a-form-item>
        </a-col>
        <a-col :xs="24" :md="12">
          <a-form-item name="artifact_directory" class="form-item-compact form-item-artifact-directory">
            <template #label>
              <span class="field-label-with-hint">制品路径</span>
            </template>
            <a-input
              v-model:value="model.artifact_directory"
              :disabled="!model.artifact_repository_id"
              placeholder="例如 release/pay-center"
            />
          </a-form-item>
        </a-col>
      </a-row>

      <a-form-item class="form-item-compact form-item-wide" label="代码仓库地址" name="repo_url">
        <a-input v-model:value="model.repo_url" />
      </a-form-item>

      <a-row :gutter="12" class="form-row-compact">
        <a-col :xs="24" :md="24">
          <a-form-item class="form-item-description" label="应用描述" name="description">
            <a-textarea
              v-model:value="model.description"
              :rows="3"
              :maxlength="250"
            />
          </a-form-item>
        </a-col>
      </a-row>
    </section>

    <template v-if="showAdvancedConfig">
      <section class="form-section form-section-advanced">
        <div v-if="surface === 'plain'" class="form-section-header">
          <div class="form-section-copy">
            <h3 class="form-section-title">高级配置</h3>
          </div>
        </div>

        <a-form-item label="发布分支">
          <div class="mapping-panel">
            <div class="mapping-header">
              <div class="mapping-copy">
                <div class="mapping-title">维护应用可选发布分支</div>
                <div class="mapping-help">
                  这里维护的分支会作为发布基础字段中的下拉选项，可直接映射给发布模板中的 CI / CD 管线字段
                </div>
              </div>
              <a-button type="dashed" @click="addReleaseBranch">
                <template #icon>
                  <PlusOutlined />
                </template>
                新增分支
              </a-button>
            </div>
            <div v-if="!model.release_branches.length" class="mapping-empty">
              当前未配置发布分支，发布时将无法从发布基础字段里直接下拉选择分支
            </div>
            <div v-else class="mapping-list">
              <div v-for="(item, index) in model.release_branches" :key="`release-branch-${index}`" class="mapping-row">
                <a-input v-model:value="item.name" />
                <a-input v-model:value="item.branch" />
                <a-button danger type="text" @click="removeReleaseBranch(index)">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                </a-button>
              </div>
            </div>
          </div>
        </a-form-item>

        <a-form-item label="GitOps 分支环境映射">
          <div class="mapping-panel">
            <div class="mapping-header">
              <div class="mapping-copy">
                <div class="mapping-title">按应用配置 GitOps 分支映射</div>
                <div class="mapping-help">
                  未配置映射时，平台默认使用 <code>app_key-env</code>，例如
                  <code>java-nantong-test-prod</code>
                </div>
              </div>
              <a-button type="dashed" @click="addGitOpsBranchMapping">
                <template #icon>
                  <PlusOutlined />
                </template>
                新增映射
              </a-button>
            </div>
            <div v-if="!model.gitops_branch_mappings.length" class="mapping-empty">
              当前未配置映射，将按默认规则使用 `app_key-env` 分支
            </div>
            <div v-else class="mapping-list">
              <div v-for="(item, index) in model.gitops_branch_mappings" :key="index" class="mapping-row">
                <a-input v-model:value="item.env_code" />
                <a-input v-model:value="item.branch" />
                <a-button danger type="text" @click="removeGitOpsBranchMapping(index)">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                </a-button>
              </div>
            </div>
          </div>
        </a-form-item>
      </section>
    </template>

    <a-space v-if="showActions" class="action-area" :class="{ 'action-area-plain': surface === 'plain' }">
      <a-button type="primary" :style="glassPrimaryButtonStyle" :loading="loading" @mouseenter="submitHovered = true" @mouseleave="submitHovered = false" @click="handleSubmit">{{ submitText }}</a-button>
      <a-button :style="glassSecondaryButtonStyle" @mouseenter="cancelHovered = true" @mouseleave="cancelHovered = false" @click="handleCancel">{{ cancelText }}</a-button>
    </a-space>
  </a-form>
</template>

<style scoped>
.application-form {
  background: #fff;
  border: 1px solid var(--border-color);
  padding: var(--space-6);
  border-radius: var(--radius-xl);
}

.application-form-plain {
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: transparent;
  border: none;
  padding: 0;
  border-radius: 0;
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.form-section-divided {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.88);
}

.form-section-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.form-section-bar {
  width: 4px;
  height: 24px;
  border-radius: 999px;
  background: linear-gradient(180deg, #2563eb, #3b82f6);
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.22);
}

.form-section-heading-title {
  margin: 0;
  color: var(--color-text-main);
  font-size: 16px;
  font-weight: 800;
  line-height: 1.2;
}

.application-form-plain .form-section {
  padding: 4px 0 0;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.field-label-with-hint {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  line-height: 1.1;
}

.form-row-compact {
  margin-bottom: 2px;
}

.form-row-pair :deep(.ant-form-item),
.form-row-triple :deep(.ant-form-item),
.form-row-artifact-binding :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.form-row-pair :deep(.ant-select-selector),
.form-row-triple :deep(.ant-select-selector),
.form-row-artifact-binding :deep(.ant-select-selector) {
  min-width: 0;
}

.form-item-key,
.form-item-project,
.form-item-owner,
.form-item-status,
.form-item-artifact,
.form-item-language,
.form-item-artifact-repository,
.form-item-artifact-directory {
  width: 100%;
  min-width: 0;
}

.field-required-hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 20px;
  padding: 0 8px;
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.1);
  color: #2563eb;
  font-size: 11px;
  font-weight: 700;
  line-height: 20px;
}

.application-form-plain :deep(.ant-form-item) {
  margin-bottom: 16px;
}

.application-form-plain :deep(.ant-form-item-label) {
  padding-bottom: 6px;
}

.application-form-plain :deep(.ant-form-item-label > label) {
  color: var(--color-text-main);
  min-height: auto;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.application-form-plain :deep(.ant-form-item-explain),
.application-form-plain :deep(.ant-form-item-explain-error) {
  min-height: 18px;
  line-height: 18px;
}

.application-form-plain :deep(.ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper),
.application-form-plain :deep(.ant-input-textarea textarea),
.application-form-plain :deep(.ant-select-selector),
.application-form-plain :deep(.ant-input-number),
.application-form-plain :deep(.ant-input-number-input) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.7), rgba(248, 250, 252, 0.54)) !important;
  border-color: rgba(255, 255, 255, 0.58) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.92), 0 10px 20px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(16px) saturate(140%);
  border-radius: 14px !important;
  font-size: 13px;
  color: var(--color-text-main) !important;
}

.application-form-plain :deep(.ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper),
.application-form-plain :deep(.ant-select-single:not(.ant-select-customize-input) .ant-select-selector) {
  min-height: 44px !important;
  padding-top: 0 !important;
  padding-bottom: 0 !important;
}

.application-form-plain :deep(.ant-input) {
  padding-inline: 14px;
}

.application-form-plain :deep(.ant-select-single .ant-select-selector) {
  display: flex;
  align-items: center;
  padding-inline: 14px !important;
}

.application-form-plain :deep(.ant-select-single .ant-select-selection-search),
.application-form-plain :deep(.ant-select-single .ant-select-selection-item),
.application-form-plain :deep(.ant-select-single .ant-select-selection-placeholder) {
  line-height: 42px !important;
}

.application-form-plain :deep(.ant-input-textarea textarea) {
  min-height: 108px;
  padding: 12px 14px;
  line-height: 1.6;
}

.application-form-plain :deep(.ant-select .ant-select-arrow),
.application-form-plain :deep(.ant-input::placeholder),
.application-form-plain :deep(.ant-input-textarea textarea::placeholder),
.application-form-plain :deep(.ant-select-selection-placeholder) {
  color: rgba(100, 116, 139, 0.72) !important;
}

.application-form-plain :deep(.ant-input-affix-wrapper .ant-input),
.application-form-plain :deep(.ant-input-affix-wrapper .ant-input:hover),
.application-form-plain :deep(.ant-input-affix-wrapper .ant-input:focus) {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  padding-inline: 0 !important;
}

.application-form-plain :deep(.ant-input:hover),
.application-form-plain :deep(.ant-input:focus),
.application-form-plain :deep(.ant-input-affix-wrapper:hover),
.application-form-plain :deep(.ant-input-affix-wrapper-focused),
.application-form-plain :deep(.ant-input-textarea textarea:hover),
.application-form-plain :deep(.ant-input-textarea textarea:focus),
.application-form-plain :deep(.ant-select:hover .ant-select-selector),
.application-form-plain :deep(.ant-select-focused .ant-select-selector) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.82), rgba(241, 245, 249, 0.66)) !important;
  border-color: rgba(147, 197, 253, 0.48) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.96), 0 12px 24px rgba(59, 130, 246, 0.06) !important;
}

.form-item-wide {
  margin-top: 4px;
}

.form-item-description {
  margin-bottom: 0;
}

.action-area {
  width: 100%;
  justify-content: center;
}

.action-area-plain {
  order: -1;
  justify-content: flex-end;
  padding-top: 0;
  margin-bottom: 4px;
}

.mapping-help code,
.mapping-empty code {
  padding: 0 6px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.06);
  color: #22304d;
  font-size: 12px;
}

.mapping-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: #fafbfc;
}

.mapping-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.mapping-copy {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.mapping-title {
  font-weight: 600;
  color: var(--heading-color);
}

.mapping-help,
.mapping-empty {
  color: var(--text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.mapping-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mapping-row {
  display: grid;
  grid-template-columns: minmax(0, 180px) minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
}

@media (max-width: 1024px) {
  .application-form {
    padding: 20px;
  }

  .application-form-plain .form-section {
    padding: 18px 18px 2px;
  }
}

@media (max-width: 768px) {
  .application-form {
    padding: 16px;
  }

  .application-form-plain .form-section {
    padding: 16px 16px 0;
  }

  .mapping-header {
    flex-direction: column;
  }

  .mapping-row {
    grid-template-columns: 1fr;
  }
}
</style>
