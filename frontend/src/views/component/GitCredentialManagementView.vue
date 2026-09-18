<script setup lang="ts">
import {
  ApiOutlined,
  DeleteOutlined,
  EditOutlined,
  InfoCircleOutlined,
  LinkOutlined,
  KeyOutlined,
  LeftOutlined,
  PlusOutlined,
  RightOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import {
  createGitCredential,
  deleteGitCredential,
  listGitCredentials,
  testGitCredentialByID,
  updateGitCredential,
} from '../../api/git-credential'
import { useAuthStore } from '../../stores/auth'
import type {
  GitCredential,
  GitCredentialAuthType,
  GitCredentialPayload,
  GitCredentialStatus,
  GitCredentialTestResult,
} from '../../types/git-credential'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface GitCredentialFormState {
  name: string
  base_url: string
  username: string
  secret: string
  auth_type: GitCredentialAuthType
  status: GitCredentialStatus
  remark: string
}

interface CredentialSearchSuggestion {
  id: string
  title: string
  subtitle: string
}

const authStore = useAuthStore()

const loadingCredentials = ref(false)
const savingCredential = ref(false)
const deletingCredentialID = ref('')
const testingCredentialID = ref('')
const credentials = ref<GitCredential[]>([])
const credentialTotal = ref(0)
const credentialFormRef = ref<FormInstance>()
const credentialModalVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const editingCredentialID = ref('')
const credentialTestResults = reactive<Record<string, GitCredentialTestResult>>({})
const credentialModalViewportInset = ref(0)

const searchOverlayVisible = ref(false)
const searchDraftKeyword = ref('')
const searchSuggestions = ref<CredentialSearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
let searchSuggestionTimer: number | undefined

const filters = reactive({
  keyword: '',
  status: '' as GitCredentialStatus | '',
  page: 1,
  pageSize: 10,
})

const canManageCredential = computed(() => authStore.hasPermission('component.credential.manage'))

const authTypeOptions = [
  { label: '访问令牌', value: 'token' },
  { label: '账号密码', value: 'password' },
]

const statusOptions: Array<{ label: string; value: GitCredentialStatus | '' }> = [
  { label: '状态 · 全部', value: '' },
  { label: '状态 · 启用', value: 'active' },
  { label: '状态 · 停用', value: 'disabled' },
]

const credentialStatusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'disabled' },
]

const credentialColumns: TableColumnsType<GitCredential> = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '地址前缀', dataIndex: 'base_url', key: 'base_url', width: 250 },
  { title: '认证方式', dataIndex: 'auth_type', key: 'auth_type', width: 120 },
  { title: '用户名', dataIndex: 'username', key: 'username', width: 150 },
  { title: '密钥', dataIndex: 'secret_configured', key: 'secret_configured', width: 110 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 180 },
  { title: '操作', key: 'actions', width: 240, fixed: 'right' },
]

const credentialForm = reactive<GitCredentialFormState>({
  name: '',
  base_url: '',
  username: '',
  secret: '',
  auth_type: 'token',
  status: 'active',
  remark: '',
})

const creatingCredential = computed(() => editorMode.value === 'create')
const credentialModalTitle = computed(() => (creatingCredential.value ? '新增凭证' : '编辑凭证'))
const credentialSecretLabel = computed(() => (credentialForm.auth_type === 'password' ? '密码' : '访问令牌'))
const credentialPageCount = computed(() =>
  Math.max(1, Math.ceil(credentialTotal.value / Math.max(1, filters.pageSize))),
)
const credentialTableLocale = computed(() => ({ emptyText: '暂无 Git 凭证' }))

// 密钥只写不读：编辑态留空表示沿用已保存的密钥，因此只有新建时必填。
const editingCredential = computed(
  () => credentials.value.find((item) => item.id === editingCredentialID.value) || null,
)
const editingCredentialSecretConfigured = computed(() => editingCredential.value?.secret_configured ?? false)

const credentialFormRules = computed(() => {
  const rules: Record<string, unknown[]> = {
    name: [{ required: true, message: '请输入凭证名称', trigger: 'blur' }],
    base_url: [{ required: true, message: '请输入仓库地址前缀', trigger: 'blur' }],
    auth_type: [{ required: true, message: '请选择认证方式', trigger: 'change' }],
  }
  if (credentialForm.auth_type === 'password') {
    rules.username = [{ required: true, message: '请输入用户名', trigger: 'blur' }]
  }
  if (creatingCredential.value) {
    rules.secret = [
      {
        required: true,
        message: credentialForm.auth_type === 'password' ? '请输入密码' : '请输入访问令牌',
        trigger: 'blur',
      },
    ]
  }
  return rules
})

const credentialModalMaskStyle = computed(() => ({
  left: `${credentialModalViewportInset.value}px`,
  width: `calc(100% - ${credentialModalViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: credentialModalVisible.value ? 'auto' : 'none',
}))
const credentialModalWrapProps = computed(() => ({
  style: {
    left: `${credentialModalViewportInset.value}px`,
    width: `calc(100% - ${credentialModalViewportInset.value}px)`,
    pointerEvents: credentialModalVisible.value ? 'auto' : 'none',
  },
}))

let credentialModalViewportObserver: ResizeObserver | null = null

async function loadGitCredentials(options?: { silent?: boolean }) {
  if (!options?.silent) {
    loadingCredentials.value = true
  }
  try {
    const response = await listGitCredentials({
      keyword: filters.keyword || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    credentials.value = response.data
    credentialTotal.value = response.total
    filters.page = response.page
    filters.pageSize = response.page_size
    Object.keys(credentialTestResults).forEach((id) => {
      if (!response.data.some((item) => item.id === id)) {
        delete credentialTestResults[id]
      }
    })
  } catch (error) {
    if (!options?.silent) {
      message.error(extractHTTPErrorMessage(error, '加载 Git 凭证失败'))
    }
  } finally {
    if (!options?.silent) {
      loadingCredentials.value = false
    }
  }
}

function resetCredentialForm() {
  credentialForm.name = ''
  credentialForm.base_url = ''
  credentialForm.username = ''
  credentialForm.secret = ''
  credentialForm.auth_type = 'token'
  credentialForm.status = 'active'
  credentialForm.remark = ''
  credentialFormRef.value?.clearValidate()
}

function openCreateCredentialModal() {
  editorMode.value = 'create'
  editingCredentialID.value = ''
  resetCredentialForm()
  credentialModalVisible.value = true
}

function populateCredentialForm(record: GitCredential) {
  credentialForm.name = record.name
  credentialForm.base_url = record.base_url
  credentialForm.username = record.username
  // 接口不回传密钥，编辑时始终从空值开始，留空即保留原密钥。
  credentialForm.secret = ''
  credentialForm.auth_type = record.auth_type
  credentialForm.status = record.status
  credentialForm.remark = record.remark
  credentialFormRef.value?.clearValidate()
}

function openEditCredentialModal(record: GitCredential) {
  editorMode.value = 'edit'
  editingCredentialID.value = record.id
  populateCredentialForm(record)
  credentialModalVisible.value = true
}

function closeCredentialModal() {
  credentialModalVisible.value = false
}

// 地址前缀参与最长前缀匹配，尾部斜杠会造成重复层级，写入前统一收敛。
function normalizeBaseURL(value: string) {
  const raw = String(value || '').trim()
  return raw.replace(/\/+$/, '')
}

function buildCredentialPayload(): GitCredentialPayload {
  const payload: GitCredentialPayload = {
    name: credentialForm.name.trim(),
    provider: 'gitlab',
    base_url: normalizeBaseURL(credentialForm.base_url),
    username: credentialForm.username.trim(),
    auth_type: credentialForm.auth_type,
    status: credentialForm.status,
    remark: credentialForm.remark.trim(),
  }
  const secret = credentialForm.secret.trim()
  // 留空表示保留已保存的密钥，此时不提交 secret 字段。
  if (secret) {
    payload.secret = secret
  }
  return payload
}

function formatAuthType(authType: GitCredentialAuthType) {
  return authType === 'password' ? '账号密码' : '访问令牌'
}

function formatCredentialStatus(status: GitCredentialStatus) {
  return status === 'active' ? '启用' : '停用'
}

function credentialStatusColor(status: GitCredentialStatus) {
  return status === 'active' ? 'green' : 'default'
}

function formatCredentialState(configured: boolean) {
  return configured ? '已配置' : '未配置'
}

function credentialTestResult(record: GitCredential) {
  return credentialTestResults[record.id] || null
}

async function submitCredential() {
  await credentialFormRef.value?.validate()
  savingCredential.value = true
  try {
    const payload = buildCredentialPayload()

    if (editorMode.value === 'edit' && editingCredentialID.value) {
      const response = await updateGitCredential(editingCredentialID.value, payload)
      credentials.value = credentials.value.map((item) =>
        item.id === editingCredentialID.value ? response.data : item,
      )
      delete credentialTestResults[response.data.id]
      message.success('凭证已更新')
      closeCredentialModal()
      return
    }

    await createGitCredential(payload)
    message.success('凭证已新增')
    closeCredentialModal()
    filters.page = 1
    await loadGitCredentials({ silent: true })
  } catch (error) {
    message.error(
      extractHTTPErrorMessage(error, editorMode.value === 'edit' ? '更新凭证失败' : '新增凭证失败'),
    )
  } finally {
    savingCredential.value = false
  }
}

async function deleteCredential(record: GitCredential) {
  deletingCredentialID.value = record.id
  try {
    await deleteGitCredential(record.id)
    message.success('凭证已删除')
    delete credentialTestResults[record.id]
    // 删掉当前页最后一条时回退一页，避免落在空页上。
    if (credentials.value.length <= 1 && filters.page > 1) {
      filters.page -= 1
    }
    await loadGitCredentials({ silent: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '删除凭证失败'))
  } finally {
    deletingCredentialID.value = ''
  }
}

async function testCredentialConnection(record: GitCredential) {
  testingCredentialID.value = record.id
  try {
    const response = await testGitCredentialByID(record.id)
    const result = response.data
    credentialTestResults[record.id] = result
    if (result.ok) {
      const account = String(result.gitlab_username || '').trim()
      message.success(account ? `连接正常，GitLab 账号：${account}` : result.message || '连接测试通过')
      return
    }
    message.error(result.message || '连接测试失败')
  } catch (error) {
    delete credentialTestResults[record.id]
    message.error(extractHTTPErrorMessage(error, '连接测试失败'))
  } finally {
    testingCredentialID.value = ''
  }
}

function queryCredentials() {
  filters.page = 1
  void loadGitCredentials()
}

function changeCredentialPage(next: number) {
  if (next < 1 || next > credentialPageCount.value || next === filters.page) {
    return
  }
  filters.page = next
  void loadGitCredentials()
}

function clearKeywordFilter() {
  filters.keyword = ''
  searchDraftKeyword.value = ''
  queryCredentials()
}

function openSearchOverlay() {
  searchOverlayVisible.value = true
  searchDraftKeyword.value = filters.keyword
  void nextTick(() => {
    searchInputRef.value?.focus()
    searchInputRef.value?.select()
  })
}

function closeSearchOverlay() {
  searchOverlayVisible.value = false
  searchSuggestions.value = []
  searchSuggestionsLoading.value = false
}

// 联想结果必须来自后端模糊查询，而不是对当前页做本地过滤。
async function fetchSearchSuggestions(keyword: string) {
  const value = String(keyword || '').trim()
  if (!value) {
    searchSuggestions.value = []
    searchSuggestionsLoading.value = false
    return
  }
  searchSuggestionsLoading.value = true
  try {
    const response = await listGitCredentials({ keyword: value, page: 1, page_size: 8 })
    searchSuggestions.value = response.data.map((item) => ({
      id: item.id,
      title: item.name,
      subtitle: [item.base_url, item.username].filter(Boolean).join(' · '),
    }))
  } catch {
    // 联想结果失败时保持输入框可用，不额外弹窗打断输入。
    searchSuggestions.value = []
  } finally {
    searchSuggestionsLoading.value = false
  }
}

watch(searchDraftKeyword, (value) => {
  if (!searchOverlayVisible.value) {
    return
  }
  if (searchSuggestionTimer !== undefined) {
    window.clearTimeout(searchSuggestionTimer)
  }
  searchSuggestionTimer = window.setTimeout(() => {
    void fetchSearchSuggestions(value)
  }, 220)
})

function submitKeywordSearch() {
  filters.keyword = searchDraftKeyword.value.trim()
  closeSearchOverlay()
  queryCredentials()
}

function handleSearchSuggestionSelect(item: CredentialSearchSuggestion) {
  filters.keyword = item.title
  searchDraftKeyword.value = item.title
  closeSearchOverlay()
  queryCredentials()
}

function readCredentialModalViewportInset() {
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

function syncCredentialModalViewportInset() {
  credentialModalViewportInset.value = readCredentialModalViewportInset()
}

function observeCredentialModalViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') {
    return
  }

  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) {
    return
  }

  credentialModalViewportObserver?.disconnect()
  credentialModalViewportObserver = new ResizeObserver(() => {
    syncCredentialModalViewportInset()
  })

  if (appLayout) {
    credentialModalViewportObserver.observe(appLayout)
  }
  if (sider) {
    credentialModalViewportObserver.observe(sider)
  }
}

function stopObservingCredentialModalViewportInset() {
  credentialModalViewportObserver?.disconnect()
  credentialModalViewportObserver = null
}

onMounted(() => {
  syncCredentialModalViewportInset()
  observeCredentialModalViewportInset()
  void loadGitCredentials()
})

onBeforeUnmount(() => {
  stopObservingCredentialModalViewportInset()
  if (searchSuggestionTimer !== undefined) {
    window.clearTimeout(searchSuggestionTimer)
    searchSuggestionTimer = undefined
  }
})
</script>

<template>
  <div class="credential-page">
    <div class="page-header">
      <div class="page-header-copy">
        <div class="page-title">凭证管理</div>
      </div>
      <div class="page-header-actions">
        <a-button class="application-toolbar-icon-btn" aria-label="搜索凭证" @click="openSearchOverlay">
          <template #icon><SearchOutlined /></template>
        </a-button>
        <a-tag v-if="filters.keyword" class="credential-keyword-tag" closable @close="clearKeywordFilter">
          {{ filters.keyword }}
        </a-tag>
        <a-select
          v-model:value="filters.status"
          class="component-toolbar-select"
          :options="statusOptions"
          @change="queryCredentials"
        />
        <a-button class="component-toolbar-query-btn" @click="queryCredentials">查询</a-button>
        <a-button v-if="canManageCredential" class="application-toolbar-action-btn" @click="openCreateCredentialModal">
          <template #icon><PlusOutlined /></template>
          新增凭证
        </a-button>
      </div>
    </div>

    <div class="credential-hint">
      <InfoCircleOutlined class="credential-hint-icon" aria-hidden="true" />
      <span>
        按仓库地址前缀匹配（最长前缀优先），仓库地址以该前缀开头的应用会使用此凭证。
        GitLab API 需要访问令牌（token），请在 GitLab 生成具备 read_api 权限的个人访问令牌。
      </span>
    </div>

    <transition name="credential-search-fade">
      <div v-if="searchOverlayVisible" class="credential-search-overlay" @click.self="closeSearchOverlay">
        <div class="credential-search-panel">
          <div class="credential-search-input">
            <SearchOutlined class="credential-search-input-icon" aria-hidden="true" />
            <input
              ref="searchInputRef"
              v-model="searchDraftKeyword"
              class="credential-search-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="名称 / 地址前缀 / 用户名"
              @keydown.enter="submitKeywordSearch"
              @keydown.esc="closeSearchOverlay"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="credential-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="credential-search-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="credential-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="credential-search-suggestion-title">{{ item.title }}</span>
                <span class="credential-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-table
      row-key="id"
      class="credential-table"
      :columns="credentialColumns"
      :data-source="credentials"
      :loading="loadingCredentials"
      :pagination="false"
      :locale="credentialTableLocale"
      :scroll="{ x: 1400 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <div class="credential-name-cell">
            <span class="credential-name-icon"><KeyOutlined /></span>
            <span class="credential-name-copy">
              <span class="credential-name">{{ record.name }}</span>
              <span class="credential-provider">{{ record.provider }}</span>
              <span
                v-if="credentialTestResult(record)"
                class="credential-test-state"
                :class="{ 'credential-test-state--failed': !credentialTestResult(record)?.ok }"
              >
                连通性 {{ credentialTestResult(record)?.ok ? '通过' : '失败' }}
                <template v-if="credentialTestResult(record)?.gitlab_username">
                  · {{ credentialTestResult(record)?.gitlab_username }}
                </template>
                <template v-else-if="!credentialTestResult(record)?.ok">
                  · {{ credentialTestResult(record)?.message }}
                </template>
              </span>
            </span>
          </div>
        </template>
        <template v-else-if="column.key === 'base_url'">
          <span class="credential-mono">{{ record.base_url }}</span>
        </template>
        <template v-else-if="column.key === 'auth_type'">
          <a-tag>{{ formatAuthType(record.auth_type) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'username'">
          {{ record.username || '-' }}
        </template>
        <template v-else-if="column.key === 'secret_configured'">
          <a-tag :color="record.secret_configured ? 'blue' : 'default'">
            {{ formatCredentialState(record.secret_configured) }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="credentialStatusColor(record.status)">{{ formatCredentialStatus(record.status) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'remark'">
          <a-tooltip v-if="record.remark" :title="record.remark" placement="topLeft">
            <span class="credential-remark">{{ record.remark }}</span>
          </a-tooltip>
          <span v-else>-</span>
        </template>
        <template v-else-if="column.key === 'actions'">
          <div class="row-actions">
            <a-button
              v-if="canManageCredential"
              type="link"
              size="small"
              class="row-action-btn"
              @click="openEditCredentialModal(record)"
            >
              <template #icon><EditOutlined /></template>
              编辑
            </a-button>
            <a-button
              type="link"
              size="small"
              class="row-action-btn"
              :loading="testingCredentialID === record.id"
              @click="testCredentialConnection(record)"
            >
              <template #icon><LinkOutlined /></template>
              连接测试
            </a-button>
            <a-popconfirm
              v-if="canManageCredential"
              title="确认删除当前凭证吗？"
              ok-text="删除"
              cancel-text="取消"
              @confirm="deleteCredential(record)"
            >
              <a-button
                type="link"
                size="small"
                class="row-action-btn"
                danger
                :loading="deletingCredentialID === record.id"
              >
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-popconfirm>
          </div>
        </template>
      </template>
    </a-table>

    <div v-if="credentialTotal > filters.pageSize" class="credential-compact-pager">
      <span class="credential-page-summary">
        第 {{ filters.page }} / {{ credentialPageCount }} 页 · 共 {{ credentialTotal }} 条
      </span>
      <a-button
        class="credential-pager-btn"
        aria-label="上一页"
        :disabled="filters.page <= 1"
        @click="changeCredentialPage(filters.page - 1)"
      >
        <template #icon><LeftOutlined /></template>
      </a-button>
      <a-button
        class="credential-pager-btn"
        aria-label="下一页"
        :disabled="filters.page >= credentialPageCount"
        @click="changeCredentialPage(filters.page + 1)"
      >
        <template #icon><RightOutlined /></template>
      </a-button>
    </div>

    <a-modal
      :open="credentialModalVisible"
      :width="720"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :mask-style="credentialModalMaskStyle"
      :wrap-props="credentialModalWrapProps"
      wrap-class-name="credential-modal-wrap"
      @cancel="closeCredentialModal"
    >
      <template #title>
        <div class="credential-modal-titlebar">
          <span class="credential-modal-title">{{ credentialModalTitle }}</span>
          <div class="credential-modal-actions">
            <a-button
              class="application-toolbar-action-btn credential-modal-save-btn"
              :loading="savingCredential"
              @click="submitCredential"
            >
              保存
            </a-button>
          </div>
        </div>
      </template>

      <a-form
        ref="credentialFormRef"
        class="credential-form"
        layout="vertical"
        :model="credentialForm"
        :rules="credentialFormRules"
        :required-mark="false"
      >
        <section class="credential-form-panel">
          <div class="credential-form-panel-title">凭证信息</div>
          <div class="credential-form-grid">
            <a-form-item name="name">
              <template #label>
                <span class="credential-form-label">名称<a-tag class="credential-required-tag">必填</a-tag></span>
              </template>
              <a-input v-model:value="credentialForm.name" placeholder="例如 gitlab-内网" />
            </a-form-item>
            <a-form-item name="auth_type">
              <template #label>
                <span class="credential-form-label">认证方式<a-tag class="credential-required-tag">必填</a-tag></span>
              </template>
              <a-select v-model:value="credentialForm.auth_type" :options="authTypeOptions" />
            </a-form-item>
            <a-form-item name="base_url" class="credential-form-item-wide">
              <template #label>
                <span class="credential-form-label">
                  仓库地址前缀<a-tag class="credential-required-tag">必填</a-tag>
                </span>
              </template>
              <a-input v-model:value="credentialForm.base_url" placeholder="http://git.cloud.local:9080" />
              <div class="credential-form-help-text">
                按最长前缀匹配，仓库地址以该前缀开头的应用会使用此凭证。
              </div>
            </a-form-item>
            <a-form-item name="username">
              <template #label>
                <span class="credential-form-label">
                  用户名
                  <a-tag v-if="credentialForm.auth_type === 'password'" class="credential-required-tag">必填</a-tag>
                </span>
              </template>
              <a-input
                v-model:value="credentialForm.username"
                :placeholder="credentialForm.auth_type === 'password' ? '请输入 GitLab 登录用户名' : '访问令牌认证可留空'"
              />
            </a-form-item>
            <a-form-item name="secret">
              <template #label>
                <span class="credential-form-label">
                  {{ credentialSecretLabel }}
                  <a-tag v-if="creatingCredential" class="credential-required-tag">必填</a-tag>
                  <a-tag v-else class="credential-credential-tag">
                    {{ formatCredentialState(editingCredentialSecretConfigured) }}
                  </a-tag>
                </span>
              </template>
              <a-input-password
                v-model:value="credentialForm.secret"
                autocomplete="new-password"
                :placeholder="creatingCredential ? `请输入${credentialSecretLabel}` : '留空表示不修改'"
              />
              <div v-if="!creatingCredential" class="credential-form-help-text">留空表示不修改已保存的密钥。</div>
            </a-form-item>
            <a-form-item name="status">
              <template #label>
                <span class="credential-form-label">状态</span>
              </template>
              <a-select v-model:value="credentialForm.status" :options="credentialStatusOptions" />
            </a-form-item>
            <a-form-item name="remark">
              <template #label>
                <span class="credential-form-label">备注</span>
              </template>
              <a-input v-model:value="credentialForm.remark" placeholder="例如 仅内部 GitLab 使用" />
            </a-form-item>
          </div>
          <div class="credential-form-note">
            <ApiOutlined class="credential-form-note-icon" aria-hidden="true" />
            <span>保存后可在列表中对该凭证执行「连接测试」，校验地址、账号与令牌是否可用。</span>
          </div>
        </section>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.credential-page {
  min-height: 100%;
  padding: 0;
  color: #0f172a;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 14px;
}

.page-header-copy {
  min-width: 0;
}

.page-title {
  color: #0f172a;
  font-size: 24px;
  font-weight: 700;
  line-height: 1.25;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  flex: none;
  flex-wrap: wrap;
}

:deep(.application-toolbar-action-btn.ant-btn),
:deep(.component-toolbar-query-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  font-weight: 600;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-icon-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  min-width: 42px;
  height: 42px;
  padding-inline: 0;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-icon-btn.ant-btn:hover),
:deep(.application-toolbar-icon-btn.ant-btn:focus),
:deep(.application-toolbar-icon-btn.ant-btn:focus-visible),
:deep(.component-toolbar-query-btn.ant-btn:hover),
:deep(.component-toolbar-query-btn.ant-btn:focus),
:deep(.component-toolbar-query-btn.ant-btn:focus-visible) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.component-toolbar-select.ant-select) {
  min-width: 132px;
}

:deep(.component-toolbar-select.ant-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  padding: 0 14px !important;
  border-radius: 16px !important;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.component-toolbar-select.ant-select .ant-select-selection-item),
:deep(.component-toolbar-select.ant-select .ant-select-arrow) {
  color: #0f172a !important;
  font-weight: 700;
}

:deep(.credential-keyword-tag.ant-tag) {
  display: inline-flex;
  align-items: center;
  height: 42px;
  margin-inline-end: 0;
  padding-inline: 12px;
  border-radius: 16px;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.92);
  color: #1d4ed8;
  font-weight: 700;
}

.credential-hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 14px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.credential-hint-icon {
  margin-top: 2px;
  color: #93c5fd;
  flex: none;
}

/* 组件管理族表格统一直角，固定操作列保持不透明。 */
.credential-table :deep(.ant-table),
.credential-table :deep(.ant-table-content),
.credential-table :deep(.ant-table-body) {
  border-radius: 0 !important;
}

.credential-table :deep(.ant-table-container),
.credential-table :deep(.ant-table-thead > tr > th),
.credential-table :deep(.ant-table-tbody > tr > td),
.credential-table :deep(.ant-table-tbody > tr:last-child > td) {
  border-radius: 0 !important;
}

.credential-table :deep(.ant-table) {
  background: transparent;
}

.credential-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  background: #fff;
}

.credential-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.credential-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.credential-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: #fff;
  color: #0f172a;
}

.credential-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-right) {
  background: #fff !important;
}

.credential-table :deep(.ant-table-tbody > tr:hover > td),
.credential-table :deep(.ant-table-tbody > tr > td.ant-table-cell-row-hover) {
  background: rgba(248, 250, 252, 0.96) !important;
}

.credential-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right),
.credential-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-right.ant-table-cell-row-hover) {
  background: #f8fafc !important;
}

.credential-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.credential-name-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  border-radius: 12px;
  background: rgba(219, 234, 254, 0.72);
  color: #2563eb;
}

.credential-name-copy {
  min-width: 0;
}

.credential-name {
  display: block;
  color: #0f172a;
  font-weight: 700;
}

.credential-provider {
  display: block;
  margin-top: 3px;
  color: #94a3b8;
  font-size: 12px;
  text-transform: lowercase;
}

.credential-test-state {
  display: block;
  margin-top: 3px;
  color: #15803d;
  font-size: 12px;
}

.credential-test-state--failed {
  color: #b91c1c;
}

.credential-mono {
  font-family: 'JetBrains Mono', 'SFMono-Regular', Menlo, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
}

.credential-remark {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.row-action-btn {
  padding-inline: 4px;
  color: #2563eb;
  font-weight: 650;
}

.row-action-btn:hover,
.row-action-btn:focus {
  color: #1d4ed8;
}

.credential-compact-pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.credential-page-summary {
  display: inline-flex;
  align-items: center;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.56);
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

:deep(.credential-pager-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  min-width: 30px;
  height: 30px;
  padding-inline: 0;
  border-radius: 999px;
  border: 1px solid rgba(203, 213, 225, 0.72) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #334155 !important;
}

.credential-search-overlay {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 12vh 24px 24px;
  background: rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.credential-search-panel {
  width: min(640px, 100%);
  padding: 18px;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.92),
    0 28px 70px rgba(15, 23, 42, 0.16);
  backdrop-filter: blur(18px) saturate(180%);
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

.credential-search-input {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 48px;
  padding-inline: 14px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.72);
}

.credential-search-input-icon {
  color: #94a3b8;
  flex: none;
}

.credential-search-field {
  width: 100%;
  border: none;
  background: transparent;
  color: #0f172a;
  font-size: 14px;
  font-weight: 600;
  outline: none;
}

.credential-search-suggestions {
  display: grid;
  gap: 4px;
  margin-top: 12px;
  max-height: 320px;
  overflow-y: auto;
}

.credential-search-loading {
  padding: 12px 10px;
  color: #94a3b8;
  font-size: 13px;
}

.credential-search-suggestion {
  display: grid;
  gap: 2px;
  width: 100%;
  padding: 10px 12px;
  border: none;
  border-radius: 12px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.credential-search-suggestion:hover,
.credential-search-suggestion:focus-visible {
  background: rgba(239, 246, 255, 0.92);
  outline: none;
}

.credential-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.credential-search-suggestion-subtitle {
  color: #94a3b8;
  font-size: 12px;
  word-break: break-all;
}

.credential-search-fade-enter-active,
.credential-search-fade-leave-active {
  transition: opacity 0.16s ease;
}

.credential-search-fade-enter-from,
.credential-search-fade-leave-to {
  opacity: 0;
}

:global(.credential-modal-wrap .ant-modal) {
  padding-bottom: 32px;
}

:global(.credential-modal-wrap .ant-modal-content) {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 24px;
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.08), transparent 34%),
    radial-gradient(circle at bottom left, rgba(59, 130, 246, 0.12), transparent 36%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.95));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.96),
    0 32px 90px rgba(15, 23, 42, 0.18);
  backdrop-filter: blur(18px) saturate(180%);
  -webkit-backdrop-filter: blur(18px) saturate(180%);
}

:global(.credential-modal-wrap .ant-modal-header) {
  padding: 24px 28px 0;
  margin-bottom: 0;
  background: transparent;
  border-bottom: none;
}

:global(.credential-modal-wrap .ant-modal-body) {
  padding: 10px 28px 28px;
}

.credential-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.credential-modal-title {
  color: #0f172a;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

.credential-modal-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex: none;
}

:global(.credential-modal-wrap .credential-modal-save-btn.ant-btn) {
  flex: none;
  height: 42px;
  padding-inline: 18px;
  border: 1px solid rgba(96, 165, 250, 0.42) !important;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a !important;
  font-size: 13px;
  font-weight: 700;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    0 10px 22px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
  -webkit-backdrop-filter: blur(14px) saturate(135%);
}

:global(.credential-modal-wrap .credential-modal-save-btn.ant-btn:not(:disabled):hover),
:global(.credential-modal-wrap .credential-modal-save-btn.ant-btn:not(:disabled):focus) {
  border-color: rgba(37, 99, 235, 0.54) !important;
  background: rgba(255, 255, 255, 0.88) !important;
  color: #2563eb !important;
}

.credential-form {
  display: grid;
  gap: 14px;
}

.credential-form-panel {
  padding: 0;
}

.credential-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.credential-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.credential-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 14px;
}

.credential-form-item-wide {
  grid-column: 1 / -1;
}

.credential-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

.credential-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  border-radius: 999px;
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

.credential-credential-tag {
  margin-inline-start: 0;
}

.credential-form-help-text {
  margin-top: 6px;
  color: #7b8798;
  font-size: 12px;
}

.credential-form-note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
  color: #7b8798;
  font-size: 12px;
  line-height: 1.5;
}

.credential-form-note-icon {
  margin-top: 2px;
  color: #93c5fd;
  flex: none;
}

.credential-form-panel :deep(.ant-form-item) {
  margin-bottom: 0;
}

.credential-form-panel :deep(.ant-input),
.credential-form-panel :deep(.ant-input-password),
.credential-form-panel :deep(.ant-select-selector) {
  border-radius: 14px !important;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
  }

  .page-title {
    font-size: 21px;
  }

  .page-header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .credential-form-grid {
    grid-template-columns: 1fr;
  }

  .credential-modal-titlebar {
    align-items: flex-start;
    flex-direction: column;
  }

  .credential-modal-actions {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
