<script setup lang="ts">
import { ExclamationCircleOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import type { FormInstance, TableColumnsType } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  createUser,
  deleteUser,
  getUserByID,
  listUsers,
  type UserListParams,
  updateUser,
  type UserPayload,
} from '../../api/user'
import { useResizableColumns } from '../../composables/useResizableColumns'
import type { UserProfile, UserRole, UserStatus } from '../../types/user'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface UserFormState {
  username: string
  display_name: string
  email: string
  phone: string
  role: UserRole
  status: UserStatus
  password: string
}

interface SearchSuggestion {
  id: string
  title: string
  subtitle: string
}

const loading = ref(false)
const submitting = ref(false)
const deletingID = ref('')
const userList = ref<UserProfile[]>([])
const total = ref(0)

const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)

const formRef = ref<FormInstance>()
const modalVisible = ref(false)
const editingID = ref('')
const formState = reactive<UserFormState>({
  username: '',
  display_name: '',
  email: '',
  phone: '',
  role: 'normal',
  status: 'active',
  password: '',
})

const isEdit = computed(() => Boolean(editingID.value))

// ---- search overlay ----
const searchVisible = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchDraft = reactive({ keyword: '' })
const searchSuggestions = ref<SearchSuggestion[]>([])
const searchSuggestionsLoading = ref(false)
let searchTimer: ReturnType<typeof window.setTimeout> | null = null
let searchRequestSeq = 0

// ---- detail drawer ----
const drawerVisible = ref(false)
const drawerLoading = ref(false)
const drawerData = ref<UserProfile | null>(null)

// ---- modal masking ----
const userFormViewportInset = ref(0)

const userFormMaskStyle = computed(() => ({
  left: `${userFormViewportInset.value}px`,
  width: `calc(100% - ${userFormViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: modalVisible.value ? 'auto' : 'none',
}))

const userFormWrapProps = computed(() => ({
  style: {
    left: `${userFormViewportInset.value}px`,
    width: `calc(100% - ${userFormViewportInset.value}px)`,
    pointerEvents: modalVisible.value ? 'auto' : 'none',
  },
}))

let userFormViewportObserver: ResizeObserver | null = null

function readUserFormViewportInset() {
  if (typeof document === 'undefined') return 0
  const appLayout = document.querySelector('.app-layout')
  if (appLayout) {
    const rawWidth = window.getComputedStyle(appLayout).getPropertyValue('--layout-sider-width').trim()
    const parsedWidth = Number.parseFloat(rawWidth)
    if (Number.isFinite(parsedWidth) && parsedWidth >= 0) return parsedWidth
  }
  const sider = document.querySelector('.app-sider')
  if (!sider) return 0
  return Math.max(sider.getBoundingClientRect().width, 0)
}

function syncUserFormViewportInset() {
  userFormViewportInset.value = readUserFormViewportInset()
}

function observeUserFormViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') return
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) return
  userFormViewportObserver?.disconnect()
  userFormViewportObserver = new ResizeObserver(() => {
    syncUserFormViewportInset()
  })
  if (appLayout) userFormViewportObserver.observe(appLayout)
  if (sider) userFormViewportObserver.observe(sider)
}

function stopObservingUserFormViewportInset() {
  userFormViewportObserver?.disconnect()
  userFormViewportObserver = null
}

const roleOptions = [
  { label: '管理员', value: 'admin' },
  { label: '普通用户', value: 'normal' },
] as const

const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '停用', value: 'inactive' },
] as const

const initialColumns: TableColumnsType<UserProfile> = [
  { title: '用户名', dataIndex: 'username', key: 'username', width: 180 },
  { title: '姓名', dataIndex: 'display_name', key: 'display_name', width: 180 },
  { title: '角色', dataIndex: 'role', key: 'role', width: 120 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 120 },
  { title: '邮箱', dataIndex: 'email', key: 'email', width: 220, ellipsis: true },
  { title: '电话', dataIndex: 'phone', key: 'phone', width: 140 },
  { title: '操作', key: 'actions', width: 220, fixed: 'right' },
]
const { columns } = useResizableColumns(initialColumns, { minWidth: 100, maxWidth: 520, hitArea: 10 })

const rules: Record<string, Rule[]> = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  display_name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
  status: [{ required: true, message: '请选择状态', trigger: 'change' }],
  password: [
    {
      validator: async (_rule: Rule, value: string) => {
        const text = String(value || '').trim()
        if (!isEdit.value && !text) {
          return Promise.reject(new Error('新增用户必须设置密码'))
        }
        if (text && text.length < 6) {
          return Promise.reject(new Error('密码长度不能少于 6 位'))
        }
        return Promise.resolve()
      },
      trigger: 'blur',
    },
  ],
}

function resetFormState() {
  editingID.value = ''
  formState.username = ''
  formState.display_name = ''
  formState.email = ''
  formState.phone = ''
  formState.role = 'normal'
  formState.status = 'active'
  formState.password = ''
}

function fillFormState(item: UserProfile) {
  editingID.value = item.id
  formState.username = item.username
  formState.display_name = item.display_name
  formState.email = item.email || ''
  formState.phone = item.phone || ''
  formState.role = item.role
  formState.status = item.status
  formState.password = ''
}

function toUserPayload(): UserPayload {
  const payload: UserPayload = {
    display_name: formState.display_name.trim(),
    role: formState.role,
    status: formState.status,
    email: formState.email.trim() || undefined,
    phone: formState.phone.trim() || undefined,
  }
  if (formState.password.trim()) {
    payload.password = formState.password.trim()
  }
  if (!isEdit.value) {
    payload.username = formState.username.trim()
  }
  return payload
}

async function loadUsers() {
  loading.value = true
  try {
    const kw = keyword.value.trim() || undefined
    const params: UserListParams = {
      username: kw,
      name: kw,
      page: page.value,
      page_size: pageSize.value,
    }
    const response = await listUsers(params)
    userList.value = response.data
    total.value = response.total
    page.value = response.page
    pageSize.value = response.page_size
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户列表加载失败'))
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number, ps: number) {
  page.value = p
  pageSize.value = ps
  void loadUsers()
}

// ---- search overlay ----
function openSearchDialog() {
  searchDraft.keyword = keyword.value
  searchVisible.value = true
  void nextTick(() => {
    searchInputRef.value?.focus()
  })
}

function closeSearchDialog() {
  searchVisible.value = false
}

function resetSearchSuggestions() {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
  searchRequestSeq += 1
  searchSuggestions.value = []
  searchSuggestionsLoading.value = false
}

async function loadSearchSuggestions(kw: string) {
  const reqSeq = ++searchRequestSeq
  searchSuggestionsLoading.value = true
  try {
    const response = await listUsers({
      name: kw,
      page: 1,
      page_size: 6,
    })
    if (reqSeq !== searchRequestSeq) return
    searchSuggestions.value = (response.data || []).map((item) => ({
      id: item.id,
      title: item.display_name,
      subtitle: item.username,
    }))
  } catch {
    if (reqSeq !== searchRequestSeq) return
    searchSuggestions.value = []
  } finally {
    if (reqSeq === searchRequestSeq) {
      searchSuggestionsLoading.value = false
    }
  }
}

function handleSearchInput() {
  const kw = searchDraft.keyword.trim()
  if (searchTimer) clearTimeout(searchTimer)
  if (!kw) {
    resetSearchSuggestions()
    return
  }
  searchTimer = setTimeout(() => {
    searchTimer = null
    void loadSearchSuggestions(kw)
  }, 260)
}

function handleSearchSubmit() {
  keyword.value = searchDraft.keyword.trim()
  page.value = 1
  searchVisible.value = false
  resetSearchSuggestions()
  void loadUsers()
}

function handleSearchSuggestionSelect(item: SearchSuggestion) {
  searchDraft.keyword = item.subtitle
  handleSearchSubmit()
}

// ---- modal ----
function openCreateModal() {
  resetFormState()
  syncUserFormViewportInset()
  modalVisible.value = true
}

function openEditModal(item: UserProfile) {
  fillFormState(item)
  syncUserFormViewportInset()
  modalVisible.value = true
}

function closeModal() {
  modalVisible.value = false
}

function handleFormAfterClose() {
  resetFormState()
  submitting.value = false
  void formRef.value?.clearValidate()
}

async function handleSubmit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    const payload = toUserPayload()
    if (isEdit.value) {
      await updateUser(editingID.value, payload)
      message.success('用户更新成功')
    } else {
      await createUser(payload)
      message.success('用户创建成功')
    }
    closeModal()
    await loadUsers()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户保存失败'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(id: string) {
  deletingID.value = id
  try {
    await deleteUser(id)
    message.success('用户删除成功')
    if (userList.value.length === 1 && page.value > 1) {
      page.value -= 1
    }
    await loadUsers()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户删除失败'))
  } finally {
    deletingID.value = ''
  }
}

function formatTime(value: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (isNaN(d.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

async function openDetailDrawer(record: UserProfile) {
  drawerVisible.value = true
  drawerLoading.value = true
  drawerData.value = null
  try {
    const response = await getUserByID(record.id)
    drawerData.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户详情加载失败'))
    drawerVisible.value = false
  } finally {
    drawerLoading.value = false
  }
}

function closeDetailDrawer() {
  drawerVisible.value = false
  drawerData.value = null
}

onMounted(() => {
  syncUserFormViewportInset()
  observeUserFormViewportInset()
  void loadUsers()
})

onBeforeUnmount(() => {
  stopObservingUserFormViewportInset()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header">
      <div class="page-header-copy">
        <h2 class="page-title">用户</h2>
      </div>
      <div class="page-header-actions">
        <a-button class="user-toolbar-icon-btn" @click="openSearchDialog">
          <template #icon>
            <SearchOutlined />
          </template>
        </a-button>
        <a-button class="user-toolbar-action-btn" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          新增用户
        </a-button>
      </div>
    </div>

    <transition name="user-search-fade">
      <div v-if="searchVisible" class="user-search-overlay" @click.self="closeSearchDialog">
        <div class="user-search-floating-panel">
          <div class="user-search-floating-input">
            <SearchOutlined class="user-search-floating-icon" />
            <input
              ref="searchInputRef"
              v-model="searchDraft.keyword"
              class="user-search-floating-field"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="搜索用户名或姓名"
              @input="handleSearchInput"
              @keydown.enter="handleSearchSubmit"
              @keydown.esc="closeSearchDialog"
            />
          </div>
          <div v-if="searchSuggestionsLoading || searchSuggestions.length > 0" class="user-search-suggestions">
            <div v-if="searchSuggestionsLoading" class="user-search-suggestion-loading">正在查询</div>
            <template v-else>
              <button
                v-for="item in searchSuggestions"
                :key="item.id"
                type="button"
                class="user-search-suggestion"
                @click="handleSearchSuggestionSelect(item)"
              >
                <span class="user-search-suggestion-title">{{ item.title }}</span>
                <span class="user-search-suggestion-subtitle">{{ item.subtitle }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>

    <a-card class="table-card" :bordered="false">
      <a-table
        row-key="id"
        :columns="columns"
        :data-source="userList"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 1180 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'role'">
            <a-tag :color="record.role === 'admin' ? 'blue' : 'default'">
              {{ record.role === 'admin' ? '管理员' : '普通用户' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.status === 'active' ? 'green' : 'default'">
              {{ record.status === 'active' ? '启用' : '停用' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-button type="link" size="small" @click="openDetailDrawer(record)">查看</a-button>
              <a-button type="link" size="small" @click="openEditModal(record)">编辑</a-button>
              <a-popconfirm title="确认删除该用户吗？" ok-text="删除" cancel-text="取消" @confirm="handleDelete(record.id)">
                <template #icon>
                  <ExclamationCircleOutlined class="danger-icon" />
                </template>
                <a-button type="link" size="small" danger :loading="deletingID === record.id">删除</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>

      <div class="pagination-area">
        <a-pagination
          :current="page"
          :page-size="pageSize"
          :total="total"
          :page-size-options="['10', '20', '50', '100']"
          show-size-changer
          show-quick-jumper
          :show-total="(count: number) => `共 ${count} 条`"
          @change="handlePageChange"
        />
      </div>
    </a-card>

    <a-modal
      :open="modalVisible"
      :width="640"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleFormAfterClose"
      :mask-style="userFormMaskStyle"
      :wrap-props="userFormWrapProps"
      wrap-class-name="user-form-modal-wrap"
      @cancel="closeModal"
    >
      <template #title>
        <div class="user-form-modal-titlebar">
          <span class="user-form-modal-title">{{ isEdit ? '编辑用户' : '新增用户' }}</span>
          <a-button class="application-toolbar-action-btn user-form-modal-save-btn" :loading="submitting" @click="handleSubmit">
            保存
          </a-button>
        </div>
      </template>

      <a-form ref="formRef" :model="formState" :rules="rules" layout="vertical" :required-mark="false" class="user-form">
        <div class="user-form-note">
          {{ isEdit ? '编辑态下用户名保持只读，如需修改请删除当前用户后重新创建。' : '创建用户账号，用户名保存后不可修改。' }}
        </div>

        <div v-if="isEdit" class="user-form-panel user-form-panel--context">
          <div class="user-form-panel-title">当前用户</div>
          <div class="user-form-context">
            <div class="user-form-context-item">
              <div class="user-form-context-label">用户名</div>
              <div class="user-form-context-value">{{ formState.username }}</div>
            </div>
          </div>
        </div>

        <div class="user-form-panel">
          <div class="user-form-panel-title">{{ isEdit ? '可编辑配置' : '基础配置' }}</div>

          <a-form-item v-if="!isEdit" name="username">
            <template #label>
              <span class="user-form-label">
                用户名
                <a-tag class="user-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="formState.username" placeholder="请输入用户名" />
          </a-form-item>

          <a-form-item name="display_name">
            <template #label>
              <span class="user-form-label">
                姓名
                <a-tag class="user-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input v-model:value="formState.display_name" placeholder="请输入姓名" />
          </a-form-item>

          <a-row :gutter="12">
            <a-col :span="12">
              <a-form-item name="role">
                <template #label>
                  <span class="user-form-label">
                    角色
                    <a-tag class="user-form-required-tag">必填</a-tag>
                  </span>
                </template>
                <a-select v-model:value="formState.role" :options="roleOptions" placeholder="请选择角色" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item name="status">
                <template #label>
                  <span class="user-form-label">
                    状态
                    <a-tag class="user-form-required-tag">必填</a-tag>
                  </span>
                </template>
                <a-select v-model:value="formState.status" :options="statusOptions" placeholder="请选择状态" />
              </a-form-item>
            </a-col>
          </a-row>

          <a-form-item name="email">
            <template #label>
              <span class="user-form-label">邮箱</span>
            </template>
            <a-input v-model:value="formState.email" placeholder="请输入邮箱（可选）" />
          </a-form-item>

          <a-form-item name="phone">
            <template #label>
              <span class="user-form-label">电话</span>
            </template>
            <a-input v-model:value="formState.phone" placeholder="请输入电话（可选）" />
          </a-form-item>

          <a-form-item name="password">
            <template #label>
              <span class="user-form-label">
                {{ isEdit ? '重置密码' : '密码' }}
                <a-tag v-if="!isEdit" class="user-form-required-tag">必填</a-tag>
              </span>
            </template>
            <a-input-password
              v-model:value="formState.password"
              :placeholder="isEdit ? '留空表示不修改密码' : '请输入密码（至少 6 位）'"
            />
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer :open="drawerVisible" title="用户详情" width="640" @close="closeDetailDrawer">
      <a-skeleton v-if="drawerLoading" active :paragraph="{ rows: 8 }" />
      <a-descriptions v-else-if="drawerData" :column="1" bordered>
        <a-descriptions-item label="用户ID">{{ drawerData.id }}</a-descriptions-item>
        <a-descriptions-item label="用户名">{{ drawerData.username }}</a-descriptions-item>
        <a-descriptions-item label="姓名">{{ drawerData.display_name }}</a-descriptions-item>
        <a-descriptions-item label="角色">
          <a-tag :color="drawerData.role === 'admin' ? 'blue' : 'default'">
            {{ drawerData.role === 'admin' ? '管理员' : '普通用户' }}
          </a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="状态">
          <a-tag :color="drawerData.status === 'active' ? 'green' : 'default'">
            {{ drawerData.status === 'active' ? '启用' : '停用' }}
          </a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="邮箱">{{ drawerData.email || '-' }}</a-descriptions-item>
        <a-descriptions-item label="电话">{{ drawerData.phone || '-' }}</a-descriptions-item>
        <a-descriptions-item label="创建时间">{{ formatTime(drawerData.created_at) }}</a-descriptions-item>
        <a-descriptions-item label="更新时间">{{ formatTime(drawerData.updated_at) }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </div>
</template>

<style scoped>
/* ---- page header (transparent, no card bg) ---- */
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

/* ---- header glass buttons ---- */
.user-toolbar-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(148, 163, 184, 0.28) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding: 0;
}

.user-toolbar-icon-btn:hover,
.user-toolbar-icon-btn:focus {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.user-toolbar-action-btn {
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

.user-toolbar-action-btn:hover,
.user-toolbar-action-btn:focus,
.user-toolbar-action-btn:focus-visible,
.user-toolbar-action-btn:active {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
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
  font-weight: 600;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.application-toolbar-action-btn.ant-btn):hover,
:deep(.application-toolbar-action-btn.ant-btn):focus,
:deep(.application-toolbar-action-btn.ant-btn):focus-visible,
:deep(.application-toolbar-action-btn.ant-btn):active {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

/* ---- search overlay ---- */
.user-search-fade-enter-active {
  transition: opacity 0.18s ease;
}

.user-search-fade-leave-active {
  transition: opacity 0.12s ease;
}

.user-search-fade-enter-from,
.user-search-fade-leave-to {
  opacity: 0;
}

.user-search-overlay {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  left: var(--layout-sider-width, 220px);
  z-index: 1200;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 84px 24px 24px;
  background: rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(8px) saturate(112%);
}

.user-search-floating-panel {
  width: min(100%, 480px);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.user-search-floating-input {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 0 14px;
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0.6)),
    rgba(255, 255, 255, 0.44);
  border: 1px solid rgba(255, 255, 255, 0.74);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 16px 32px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px) saturate(125%);
}

.user-search-floating-input:focus-within {
  border-color: rgba(255, 255, 255, 0.82);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.9),
    0 18px 38px rgba(15, 23, 42, 0.1);
}

.user-search-floating-icon {
  color: rgba(148, 163, 184, 0.9);
  font-size: 14px;
}

.user-search-floating-field {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0;
  border: none;
  outline: none;
  background: transparent;
  box-shadow: none;
  color: #0f172a;
  font-size: 13px;
  line-height: 34px;
}

.user-search-floating-field::placeholder {
  color: rgba(71, 85, 105, 0.72);
}

.user-search-suggestions {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: 14px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(248, 250, 252, 0.94));
  border: 1px solid rgba(148, 163, 184, 0.14);
  box-shadow:
    0 14px 36px rgba(15, 23, 42, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.84);
}

.user-search-suggestion-loading {
  padding: 10px 12px;
  color: #94a3b8;
  font-size: 12px;
  text-align: center;
}

.user-search-suggestion {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  color: inherit;
  font-family: inherit;
}

.user-search-suggestion:hover,
.user-search-suggestion:focus {
  background: rgba(239, 246, 255, 0.8);
  outline: none;
}

.user-search-suggestion-title {
  color: #0f172a;
  font-size: 13px;
  font-weight: 600;
}

.user-search-suggestion-subtitle {
  color: #94a3b8;
  font-size: 12px;
}

.pagination-area {
  margin-top: var(--space-6);
  display: flex;
  justify-content: flex-end;
}

.danger-icon {
  color: #ff4d4f;
}

/* ---- modal shell ---- */
.user-form-modal-wrap :deep(.ant-modal-content) {
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

.user-form-modal-wrap :deep(.ant-modal-content)::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.62), rgba(255, 255, 255, 0.16) 34%, rgba(255, 255, 255, 0.02) 58%),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.34), transparent 32%);
  pointer-events: none;
  z-index: 0;
}

.user-form-modal-wrap :deep(.ant-modal-header) {
  position: relative;
  z-index: 1;
  margin-bottom: 10px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
  background: transparent;
}

.user-form-modal-wrap :deep(.ant-modal-title) {
  color: #0f172a;
}

.user-form-modal-wrap :deep(.ant-modal-body) {
  position: relative;
  z-index: 1;
  padding-top: 10px;
}

/* ---- titlebar ---- */
.user-form-modal-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
}

.user-form-modal-title {
  min-width: 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.user-form-modal-save-btn.ant-btn {
  flex: none;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: normal;
}

/* ---- form layout ---- */
.user-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ---- note block ---- */
.user-form-note {
  position: relative;
  padding: 0 0 0 14px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.user-form-note::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 4px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(59, 130, 246, 0.42), rgba(96, 165, 250, 0.16));
}

/* ---- form panels ---- */
.user-form-panel {
  padding: 0;
}

.user-form-panel-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.4;
  font-weight: 700;
}

.user-form-panel-title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, rgba(203, 213, 225, 0.78), rgba(226, 232, 240, 0));
  transform: translateY(1px);
}

.user-form-note + .user-form-panel,
.user-form-panel + .user-form-panel {
  padding-top: 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.92);
}

.user-form-panel--context {
  padding-bottom: 4px;
}

/* ---- context block ---- */
.user-form-context {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.user-form-context-item {
  min-width: 0;
  padding: 0 0 10px;
  border-bottom: 1px dashed rgba(226, 232, 240, 0.92);
}

.user-form-context-label {
  margin-bottom: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.user-form-context-value {
  color: #0f172a;
  font-size: 14px;
  line-height: 1.6;
  font-weight: 600;
}

/* ---- form label ---- */
.user-form-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.user-form-required-tag {
  margin-inline-end: 0;
  border: 1px solid rgba(191, 219, 254, 0.72);
  background: rgba(239, 246, 255, 0.96);
  color: #2563eb;
  font-size: 11px;
  line-height: 18px;
}

/* ---- form item spacing & label ---- */
.user-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.user-form :deep(.ant-form-item-label > label) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

/* ---- input/select transparent ---- */
.user-form :deep(.ant-input),
.user-form :deep(.ant-select-selector),
.user-form :deep(.ant-input-affix-wrapper) {
  background: transparent !important;
  border-color: rgba(203, 213, 225, 0.88) !important;
  box-shadow: none !important;
}

.user-form :deep(.ant-input:hover),
.user-form :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector),
.user-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: rgba(96, 165, 250, 0.48) !important;
}

.user-form :deep(.ant-input:focus),
.user-form :deep(.ant-input-focused),
.user-form :deep(.ant-input-affix-wrapper-focused),
.user-form :deep(.ant-select-focused .ant-select-selector) {
  background: transparent !important;
  border-color: rgba(59, 130, 246, 0.56) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12) !important;
}

.user-form :deep(.ant-select-disabled .ant-select-selector),
.user-form :deep(.ant-input[disabled]) {
  background: transparent !important;
  color: #94a3b8 !important;
}

.user-form :deep(.ant-select-selection-placeholder),
.user-form :deep(.ant-input::placeholder) {
  color: #94a3b8;
}

@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 768px) {
  .pagination-area {
    justify-content: center;
  }

  .user-form-context {
    grid-template-columns: 1fr;
  }

  .user-search-overlay {
    padding: 64px 16px 24px;
  }
}
</style>
