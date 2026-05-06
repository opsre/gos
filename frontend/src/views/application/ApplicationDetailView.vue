<script setup lang="ts">
import { ArrowLeftOutlined, EditOutlined } from '@ant-design/icons-vue'
import dayjs from 'dayjs'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getApplicationByID } from '../../api/application'
import type { Application } from '../../types/application'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const route = useRoute()
const router = useRouter()

const applicationId = computed(() => String(route.params.id || ''))
const application = ref<Application | null>(null)
const loading = ref(false)
const errorMessage = ref('')

const gitOpsEnvOrder = ['dev', 'test', 'prod']

function applicationStatusText(status: Application['status']) {
  return status === 'active' ? '启用中' : '已停用'
}

function formatDateTime(value: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function projectLabel(app: Application) {
  return app.project_name
    ? `${app.project_name}${app.project_key ? ` (${app.project_key})` : ''}`
    : '-'
}

function baselineInfoRows(app: Application) {
  return [
    { key: 'name', label: '应用名称', value: app.name || '-' },
    { key: 'key', label: '应用 Key', value: app.key || '-' },
    { key: 'project', label: '归属项目', value: projectLabel(app) },
    { key: 'status', label: '状态', value: applicationStatusText(app.status) },
    { key: 'owner', label: '负责人', value: app.owner || '-' },
    { key: 'artifact_type', label: '制品类型', value: app.artifact_type || '-' },
    { key: 'language', label: '语言', value: app.language || '-' },
    { key: 'repo_url', label: '仓库地址', value: app.repo_url || '-' },
    { key: 'description', label: '描述', value: app.description || '-' },
    { key: 'updated_at', label: '更新时间', value: formatDateTime(app.updated_at) },
  ]
}

function gitOpsEnvRank(envCode: string) {
  const index = gitOpsEnvOrder.indexOf(String(envCode || '').trim().toLowerCase())
  return index >= 0 ? index : gitOpsEnvOrder.length
}

function sortedGitOpsMappings(app: Application) {
  return [...(app.gitops_branch_mappings || [])].sort((left, right) => {
    const rankDiff = gitOpsEnvRank(left.env_code) - gitOpsEnvRank(right.env_code)
    if (rankDiff !== 0) return rankDiff
    const envDiff = String(left.env_code || '').localeCompare(String(right.env_code || ''))
    if (envDiff !== 0) return envDiff
    return String(left.branch || '').localeCompare(String(right.branch || ''))
  })
}

function goBack() {
  router.push('/applications')
}

function goEdit() {
  router.push({ name: 'application-edit', params: { id: applicationId.value } })
}

async function loadApplication() {
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await getApplicationByID(applicationId.value)
    application.value = response.data
  } catch (error) {
    errorMessage.value = extractHTTPErrorMessage(error) || '加载应用信息失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadApplication()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header">
      <div class="page-header-main">
        <div class="page-header-copy">
          <h2 class="page-title">应用详情</h2>
        </div>
      </div>
      <div class="page-header-actions">
        <a-button type="primary" @click="goEdit">
          <template #icon><EditOutlined /></template>
          编辑应用
        </a-button>
        <a-button @click="goBack">
          <template #icon><ArrowLeftOutlined /></template>
          返回列表
        </a-button>
      </div>
    </div>

    <a-skeleton v-if="loading" active :paragraph="{ rows: 10 }" />

    <a-result
      v-else-if="errorMessage"
      status="error"
      title="加载失败"
      :sub-title="errorMessage"
    >
      <template #extra>
        <a-button @click="goBack">返回列表</a-button>
        <a-button type="primary" @click="loadApplication">重试</a-button>
      </template>
    </a-result>

    <div v-else-if="application" class="detail-content">
      <a-descriptions :column="1" bordered>
        <a-descriptions-item
          v-for="item in baselineInfoRows(application)"
          :key="item.key"
          :label="item.label"
        >
          {{ item.value }}
        </a-descriptions-item>

        <a-descriptions-item label="GitOps 映射">
          <div
            v-if="application.gitops_branch_mappings?.length"
            class="mini-table-scroll"
          >
            <table class="mini-table mini-table-gitops">
              <thead>
                <tr>
                  <th>环境</th>
                  <th>分支</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(mapping, idx) in sortedGitOpsMappings(application)"
                  :key="`${mapping.env_code}-${mapping.branch}-${idx}`"
                >
                  <td>{{ mapping.env_code || '-' }}</td>
                  <td>{{ mapping.branch || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <span v-else>当前未配置映射</span>
        </a-descriptions-item>

        <a-descriptions-item label="发布分支">
          <div
            v-if="application.release_branches?.length"
            class="mini-table-scroll"
          >
            <table class="mini-table mini-table-release">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>分支</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(branch, idx) in application.release_branches"
                  :key="`${branch.name}-${branch.branch}-${idx}`"
                >
                  <td>{{ branch.name || '-' }}</td>
                  <td>{{ branch.branch || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <span v-else>当前未配置发布分支</span>
        </a-descriptions-item>
      </a-descriptions>
    </div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
}

.page-header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.detail-content {
  max-width: 960px;
}

.detail-content :deep(.ant-descriptions) {
  background: #fff;
  border-radius: 0;
}

.detail-content :deep(.ant-descriptions-item-label),
.detail-content :deep(.ant-descriptions-item-content) {
  background: #fff;
}

.mini-table-scroll {
  max-height: 220px;
  overflow: auto;
  border: 1px solid #e6ebf5;
  border-radius: 12px;
  background: #fff;
}

.mini-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 13px;
}

.mini-table th,
.mini-table td {
  padding: 9px 12px;
  text-align: left;
  vertical-align: top;
  word-break: break-word;
}

.mini-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: #f8fafc;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.mini-table td {
  border-top: 1px solid #edf2f7;
  color: #1f2937;
  line-height: 1.55;
}

.mini-table th:first-child,
.mini-table td:first-child {
  width: 34%;
  color: #334155;
  font-weight: 700;
}
</style>
