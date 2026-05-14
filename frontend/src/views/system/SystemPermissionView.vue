<script setup lang="ts">
import { SaveOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, onMounted, ref } from 'vue'
import { listApplicationOptions } from '../../api/application'
import { getReleaseSettings } from '../../api/system'
import {
  grantUserPermissions,
  listUsers,
  listPermissions,
  listUserPermissions,
  revokeUserPermissions,
} from '../../api/user'
import type { PermissionMeta, UserPermission } from '../../types/user'
import { extractHTTPErrorMessage } from '../../utils/http-error'

interface SelectOption {
  label: string
  value: string
}

interface ApplicationPermissionRow {
  application_id: string
  application_name: string
}

const usersLoading = ref(false)
const selectedUserID = ref('')
const userOptions = ref<SelectOption[]>([])

const permissionsLoading = ref(false)
const permissionMetas = ref<PermissionMeta[]>([])
const userPermissions = ref<UserPermission[]>([])
const checkedPermissionCodes = ref<string[]>([])
const savingPermissions = ref(false)
const applicationOptions = ref<SelectOption[]>([])
const releaseEnvOptions = ref<SelectOption[]>([])
const applicationPermissionMap = ref<Record<string, string[]>>({})
const applicationViewScopedCodes = new Set(['application.view'])
const applicationReleaseScopedCodes = new Set(['release.view', 'release.create', 'release.execute', 'release.cancel'])
const applicationScopedCodes = Array.from(new Set([...applicationViewScopedCodes, ...applicationReleaseScopedCodes]))
const hiddenPermissionCodes = new Set([
  'release.param_config.view',
  'application.view',
  'release.view',
  'release.approval.view',
  'release.approval.submit',
  'release.approval.approve',
  'release.approval.reject',
])

const permissionGroupOrder = ['application', 'pipeline', 'platform_param', 'pipeline_param', 'component', 'release', 'system']

const groupedPermissions = computed(() => {
  const groups = new Map<string, PermissionMeta[]>()
  for (const item of permissionMetas.value) {
    if (hiddenPermissionCodes.has(String(item.code || '').trim().toLowerCase())) {
      continue
    }
    if (applicationReleaseScopedCodes.has(String(item.code || '').trim().toLowerCase())) {
      continue
    }
    if (!groups.has(item.module)) {
      groups.set(item.module, [])
    }
    groups.get(item.module)?.push(item)
  }
  const modules = Array.from(groups.keys()).sort((a, b) => {
    const ai = permissionGroupOrder.indexOf(a)
    const bi = permissionGroupOrder.indexOf(b)
    if (ai === -1 && bi === -1) {
      return a.localeCompare(b)
    }
    if (ai === -1) {
      return 1
    }
    if (bi === -1) {
      return -1
    }
    return ai - bi
  })
  return modules.map((module) => ({
    module,
    items: (groups.get(module) || []).slice().sort((a, b) => a.code.localeCompare(b.code)),
  }))
})

const applicationPermissionRows = computed<ApplicationPermissionRow[]>(() =>
  applicationOptions.value.map((item) => ({
    application_id: item.value,
    application_name: item.label,
  })),
)

function isPermissionChecked(code: string) {
  return checkedPermissionCodes.value.includes(code)
}

function normalizeScopeType(value: string) {
  return String(value || '').trim().toLowerCase()
}

function parseApplicationEnvScopeValue(value: string) {
  const raw = String(value || '').trim()
  if (!raw) {
    return null
  }
  const parts = raw.split('::')
  if (parts.length !== 2) {
    return null
  }
  const applicationID = String(parts[0] || '').trim()
  const envCode = String(parts[1] || '').trim()
  if (!applicationID || !envCode) {
    return null
  }
  return { applicationID, envCode }
}

function buildApplicationEnvScopeValue(applicationID: string, envCode: string) {
  return `${String(applicationID || '').trim()}::${String(envCode || '').trim()}`
}

function normalizeEnvCodes(values: string[]) {
  const allowedEnvCodes = new Set(releaseEnvOptions.value.map((item) => item.value))
  return Array.from(
    new Set(
      values
        .map((item) => String(item || '').trim())
        .filter((item) => item && allowedEnvCodes.has(item)),
    ),
  )
}

function handlePermissionToggle(code: string, checked: boolean) {
  const next = new Set(checkedPermissionCodes.value)
  if (checked) {
    next.add(code)
  } else {
    next.delete(code)
  }
  checkedPermissionCodes.value = Array.from(next)
}

function moduleLabel(module: string) {
  const mapping: Record<string, string> = {
    application: '应用管理',
    pipeline: '管线管理',
    platform_param: '标准字库',
    pipeline_param: '执行器参数',
    component: '组件管理',
    release: '发布管理',
    system: '系统管理',
  }
  return mapping[module] || module
}

function syncApplicationPermissionMap() {
  const nextMap: Record<string, string[]> = {}
  const currentEnvCodes = releaseEnvOptions.value.map((item) => item.value)
  const legacyGrantedScopeValues = new Set(
    userPermissions.value
      .filter(
        (item) =>
          item.enabled &&
          applicationScopedCodes.includes(String(item.permission_code || '').trim().toLowerCase()) &&
          normalizeScopeType(item.scope_type) === 'application' &&
          String(item.scope_value || '').trim() !== '',
      )
      .map((item) => String(item.scope_value || '').trim()),
  )
  const envGrantedMap = new Map<string, Set<string>>()
  userPermissions.value.forEach((item) => {
    if (
      !item.enabled ||
      !applicationReleaseScopedCodes.has(String(item.permission_code || '').trim().toLowerCase()) ||
      normalizeScopeType(item.scope_type) !== 'application_env'
    ) {
      return
    }
    const parsed = parseApplicationEnvScopeValue(item.scope_value)
    if (!parsed || !currentEnvCodes.includes(parsed.envCode)) {
      return
    }
    if (!envGrantedMap.has(parsed.applicationID)) {
      envGrantedMap.set(parsed.applicationID, new Set<string>())
    }
    envGrantedMap.get(parsed.applicationID)?.add(parsed.envCode)
  })
  for (const item of applicationOptions.value) {
    const envCodes = Array.from(envGrantedMap.get(item.value) || [])
    if (envCodes.length === 0 && legacyGrantedScopeValues.has(item.value)) {
      nextMap[item.value] = [...currentEnvCodes]
      continue
    }
    nextMap[item.value] = normalizeEnvCodes(envCodes)
  }
  applicationPermissionMap.value = nextMap
}

function selectedApplicationEnvCodes(applicationID: string) {
  const id = String(applicationID || '').trim()
  return applicationPermissionMap.value[id] || []
}

function handleApplicationReleaseChange(applicationID: string, values: string[]) {
  const id = String(applicationID || '').trim()
  if (!id) {
    return
  }
  applicationPermissionMap.value = {
    ...applicationPermissionMap.value,
    [id]: normalizeEnvCodes(values),
  }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const response = await listUsers({
      role: 'normal',
      status: 'active',
      page: 1,
      page_size: 500,
    })
    userOptions.value = response.data.map((item) => ({
      label: `${item.display_name} (${item.username})`,
      value: item.id,
    }))
    if (selectedUserID.value && !userOptions.value.some((item) => item.value === selectedUserID.value)) {
      selectedUserID.value = ''
    }
    if (!selectedUserID.value && userOptions.value.length > 0) {
      const first = userOptions.value[0]
      if (first) {
        selectedUserID.value = first.value
      }
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户下拉加载失败'))
  } finally {
    usersLoading.value = false
  }
}

async function loadPermissionMeta() {
  permissionsLoading.value = true
  try {
    const response = await listPermissions()
    permissionMetas.value = response.data
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '权限字典加载失败'))
  } finally {
    permissionsLoading.value = false
  }
}

async function loadApplicationOptions() {
  try {
    const response = await listApplicationOptions()
    applicationOptions.value = response.data.map((item) => ({
      label: `${item.name} (${item.key})`,
      value: item.id,
    }))
    syncApplicationPermissionMap()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '应用下拉加载失败'))
  }
}

async function loadReleaseEnvOptions() {
  try {
    const response = await getReleaseSettings()
    releaseEnvOptions.value = (response.data.env_options || []).map((item) => ({
      label: item,
      value: item,
    }))
    syncApplicationPermissionMap()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '发布环境加载失败'))
  }
}

async function loadUserPermissions() {
  if (!selectedUserID.value) {
    userPermissions.value = []
    checkedPermissionCodes.value = []
    applicationPermissionMap.value = {}
    return
  }
  permissionsLoading.value = true
  try {
    const response = await listUserPermissions(selectedUserID.value)
    userPermissions.value = response.data
    checkedPermissionCodes.value = response.data
      .filter(
        (item) =>
          item.enabled &&
          normalizeScopeType(item.scope_type) === 'global' &&
          !hiddenPermissionCodes.has(String(item.permission_code || '').trim().toLowerCase()) &&
          !applicationReleaseScopedCodes.has(String(item.permission_code || '').trim().toLowerCase()),
      )
      .map((item) => item.permission_code)
    syncApplicationPermissionMap()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '用户权限加载失败'))
  } finally {
    permissionsLoading.value = false
  }
}

async function loadSelectedUserAuthorization() {
  await loadUserPermissions()
}

async function handleUserChange(value: string | undefined) {
  selectedUserID.value = String(value || '').trim()
  await loadSelectedUserAuthorization()
}

async function handleSavePermissions() {
  if (!selectedUserID.value) {
    message.warning('请先选择用户')
    return
  }
  savingPermissions.value = true
  try {
    const current = new Set(
      checkedPermissionCodes.value.filter(
        (code) => !applicationReleaseScopedCodes.has(String(code || '').trim().toLowerCase()),
      ),
    )
    const before = new Set(
      userPermissions.value
        .filter(
          (item) =>
            item.enabled &&
            normalizeScopeType(item.scope_type) === 'global' &&
            !hiddenPermissionCodes.has(String(item.permission_code || '').trim().toLowerCase()) &&
            !applicationReleaseScopedCodes.has(String(item.permission_code || '').trim().toLowerCase()),
        )
        .map((item) => item.permission_code),
    )

    const toGrant = Array.from(current).filter((code) => !before.has(code))
    const toRevoke = Array.from(before).filter((code) => !current.has(code))
    const legacyGlobalReleasePermissions = userPermissions.value
      .filter(
        (item) =>
          item.enabled &&
          normalizeScopeType(item.scope_type) === 'global' &&
          (
            applicationScopedCodes.includes(String(item.permission_code || '').trim().toLowerCase())
          ),
      )
      .map((item) => ({
        permission_code: item.permission_code,
        scope_type: 'global',
        scope_value: '',
      }))

    if (toGrant.length > 0) {
      await grantUserPermissions(selectedUserID.value, {
        items: toGrant.map((code) => ({
          permission_code: code,
          scope_type: 'global',
          scope_value: '',
        })),
      })
    }
    if (toRevoke.length > 0) {
      await revokeUserPermissions(selectedUserID.value, {
        items: toRevoke.map((code) => ({
          permission_code: code,
          scope_type: 'global',
          scope_value: '',
        })),
      })
    }
    const currentAppEnvSelections = Object.entries(applicationPermissionMap.value).flatMap(([applicationID, envCodes]) =>
      normalizeEnvCodes(envCodes).map((envCode) => ({
        application_id: String(applicationID || '').trim(),
        env_code: envCode,
      })),
    )

    const beforeManagedItems = userPermissions.value
      .filter(
        (item) =>
          item.enabled &&
          (
            (applicationViewScopedCodes.has(String(item.permission_code || '').trim().toLowerCase()) &&
              normalizeScopeType(item.scope_type) === 'application') ||
            (applicationReleaseScopedCodes.has(String(item.permission_code || '').trim().toLowerCase()) &&
              (normalizeScopeType(item.scope_type) === 'application' ||
                normalizeScopeType(item.scope_type) === 'application_env'))
          ) &&
          String(item.scope_value || '').trim() !== '',
      )
      .map((item) => ({
        permission_code: String(item.permission_code || '').trim().toLowerCase(),
        scope_type: normalizeScopeType(item.scope_type),
        scope_value: String(item.scope_value || '').trim(),
      }))

    const beforeKeySet = new Set(
      beforeManagedItems.map((item) => `${item.permission_code}::${item.scope_type}::${item.scope_value}`),
    )
    const currentAppViewItems = Array.from(
      new Set(currentAppEnvSelections.map((item) => item.application_id).filter(Boolean)),
    ).map((applicationID) => ({
      permission_code: 'application.view',
      scope_type: 'application',
      scope_value: applicationID,
    }))
    const currentReleaseItems = currentAppEnvSelections.flatMap((item) =>
      Array.from(applicationReleaseScopedCodes).map((code) => ({
        permission_code: code,
        scope_type: 'application_env',
        scope_value: buildApplicationEnvScopeValue(item.application_id, item.env_code),
      })),
    )
    const currentItems = [...currentAppViewItems, ...currentReleaseItems]
    const currentKeySet = new Set(
      currentItems.map((item) => `${item.permission_code}::${item.scope_type}::${item.scope_value}`),
    )

    const appToGrant = currentItems.filter(
      (item) => !beforeKeySet.has(`${item.permission_code}::${item.scope_type}::${item.scope_value}`),
    )
    const appToRevoke = beforeManagedItems.filter(
      (item) => !currentKeySet.has(`${item.permission_code}::${item.scope_type}::${item.scope_value}`),
    )

    if (appToGrant.length > 0) {
      await grantUserPermissions(selectedUserID.value, {
        items: appToGrant,
      })
    }
    if (appToRevoke.length > 0) {
      await revokeUserPermissions(selectedUserID.value, {
        items: appToRevoke,
      })
    }
    if (legacyGlobalReleasePermissions.length > 0) {
      await revokeUserPermissions(selectedUserID.value, {
        items: legacyGlobalReleasePermissions,
      })
    }
    const legacyHiddenPermissions = userPermissions.value
      .filter(
        (item) =>
          item.enabled &&
          normalizeScopeType(item.scope_type) === 'global' &&
          hiddenPermissionCodes.has(String(item.permission_code || '').trim().toLowerCase()),
      )
      .map((item) => ({
        permission_code: item.permission_code,
        scope_type: 'global',
        scope_value: '',
      }))
    if (legacyHiddenPermissions.length > 0) {
      await revokeUserPermissions(selectedUserID.value, {
        items: legacyHiddenPermissions,
      })
    }

    message.success('权限授权已保存')
    await loadUserPermissions()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '权限保存失败'))
  } finally {
    savingPermissions.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadUsers(), loadPermissionMeta(), loadApplicationOptions(), loadReleaseEnvOptions()])
  await loadSelectedUserAuthorization()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header">
      <div class="page-header-copy">
        <h2 class="page-title">权限</h2>
      </div>
      <div class="page-header-actions">
        <a-select
          v-model:value="selectedUserID"
          class="permission-toolbar-user-select"
          show-search
          allow-clear
          option-filter-prop="label"
          :loading="usersLoading"
          :options="userOptions"
          placeholder="选择授权用户"
          @change="handleUserChange"
        />
        <a-button class="permission-toolbar-action-btn permission-toolbar-action-btn--primary" :loading="savingPermissions" @click="handleSavePermissions">
          <template #icon>
            <SaveOutlined />
          </template>
          保存权限设置
        </a-button>
      </div>
    </div>

    <section class="permission-content-panel">
      <a-spin :spinning="permissionsLoading">
        <a-empty v-if="!selectedUserID" description="请先选择用户" />
        <div v-else class="permission-content">
          <section class="permission-section permission-section--global">
            <div class="permission-section-head">
              <div>
                <div class="permission-section-eyebrow">GLOBAL</div>
              </div>
              <span class="permission-section-count">{{ checkedPermissionCodes.length }} 项已选</span>
            </div>
            <div class="permission-matrix">
              <article v-for="group in groupedPermissions" :key="group.module" class="permission-module-row">
                <div class="permission-module-cell">
                  <span class="permission-module-name">{{ moduleLabel(group.module) }}</span>
                </div>
                <div class="permission-actions-grid">
                  <a-checkbox
                    v-for="item in group.items"
                    :key="item.code"
                    class="permission-check-pill"
                    :checked="isPermissionChecked(item.code)"
                    @change="handlePermissionToggle(item.code, Boolean($event?.target?.checked))"
                  >
                    <span class="permission-name">{{ item.name }}</span>
                    <span class="permission-code">{{ item.code }}</span>
                  </a-checkbox>
                </div>
              </article>
            </div>
          </section>

          <section class="permission-section permission-section--applications">
            <div class="permission-section-head">
              <div>
                <div class="permission-section-eyebrow">APPLICATION</div>
              </div>
              <span class="permission-section-count">{{ applicationPermissionRows.length }} 个应用</span>
            </div>
            <div class="permission-app-permission-list">
              <article v-for="record in applicationPermissionRows" :key="record.application_id" class="permission-app-row">
                <div class="permission-app-meta">
                  <span class="permission-app-name">{{ record.application_name }}</span>
                </div>
                <a-select
                  mode="multiple"
                  class="app-env-select permission-app-env-select"
                  allow-clear
                  :options="releaseEnvOptions"
                  :value="selectedApplicationEnvCodes(record.application_id)"
                  placeholder="选择允许发布的环境"
                  @change="handleApplicationReleaseChange(record.application_id, $event as string[])"
                />
              </article>
            </div>
          </section>
        </div>
      </a-spin>
    </section>
  </div>
</template>

<style scoped>
/* ---- page header (transparent) ---- */
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

.permission-toolbar-user-select {
  width: 280px;
  flex: none;
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selector) {
  display: flex;
  align-items: center;
  height: 42px !important;
  min-height: 42px;
  border-radius: 16px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.62) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 12px 24px rgba(15, 23, 42, 0.04) !important;
  backdrop-filter: blur(14px) saturate(135%);
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selection-item),
:deep(.permission-toolbar-user-select.ant-select .ant-select-arrow) {
  color: #1e3a8a;
  font-weight: 650;
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selection-item) {
  display: flex;
  align-items: center;
  height: 100%;
  line-height: 1 !important;
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selection-placeholder) {
  display: flex;
  align-items: center;
  height: 100%;
  color: rgba(30, 58, 138, 0.38) !important;
  font-weight: 600;
  line-height: 1 !important;
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selection-search) {
  inset-block-start: 0 !important;
  inset-block-end: 0 !important;
}

:deep(.permission-toolbar-user-select.ant-select .ant-select-selection-search-input) {
  height: 100% !important;
  color: #1e3a8a;
  font-weight: 650;
  line-height: 42px !important;
}

:deep(.permission-toolbar-user-select.ant-select-focused .ant-select-selector),
:deep(.permission-toolbar-user-select.ant-select:hover .ant-select-selector) {
  border-color: rgba(96, 165, 250, 0.46) !important;
  background: rgba(255, 255, 255, 0.74) !important;
}

/* ---- header glass button ---- */
.permission-toolbar-action-btn {
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

.permission-toolbar-action-btn:hover,
.permission-toolbar-action-btn:focus {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

.permission-toolbar-action-btn--primary {
  background: linear-gradient(180deg, rgba(241, 247, 255, 0.9), rgba(223, 235, 255, 0.8)) !important;
  border-color: rgba(147, 197, 253, 0.74) !important;
  color: #1d4ed8 !important;
}

.permission-toolbar-action-btn--primary:hover,
.permission-toolbar-action-btn--primary:focus {
  background: linear-gradient(180deg, rgba(248, 251, 255, 0.96), rgba(231, 241, 255, 0.88)) !important;
  border-color: rgba(96, 165, 250, 0.66) !important;
  color: #1e3a8a !important;
}

.permission-content-panel {
  border: none;
  background: transparent;
  box-shadow: none;
  padding: 0;
  overflow: visible;
}

.permission-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.permission-section {
  min-width: 0;
}

.permission-section + .permission-section {
  border-top: 1px solid rgba(148, 163, 184, 0.18);
  padding-top: 20px;
}

.permission-section-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 14px;
  margin-bottom: 14px;
}

.permission-section-eyebrow {
  color: #2563eb;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.permission-section-count {
  flex: none;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  background: rgba(255, 255, 255, 0.68);
  color: #475569;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 700;
}

.permission-matrix,
.permission-app-permission-list {
  border-radius: 18px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  background: rgba(255, 255, 255, 0.48);
  overflow: hidden;
}

.permission-module-row {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  gap: 18px;
  padding: 16px;
}

.permission-module-row + .permission-module-row,
.permission-app-row + .permission-app-row {
  border-top: 1px solid rgba(148, 163, 184, 0.16);
}

.permission-module-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  justify-content: center;
  min-width: 0;
}

.permission-module-name {
  color: #0f172a;
  font-size: 15px;
  font-weight: 800;
}

.permission-actions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
  min-width: 0;
}

:deep(.permission-check-pill.ant-checkbox-wrapper) {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-height: 48px;
  margin-inline-start: 0;
  border-radius: 14px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.62);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.74);
  padding: 10px 12px;
  transition:
    border-color 0.18s ease,
    background 0.18s ease,
    box-shadow 0.18s ease;
}

:deep(.permission-check-pill.ant-checkbox-wrapper:hover) {
  border-color: rgba(37, 99, 235, 0.28);
  background: rgba(248, 251, 255, 0.88);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.84),
    0 10px 22px rgba(15, 23, 42, 0.04);
}

:deep(.permission-check-pill .ant-checkbox) {
  margin-block-start: 2px;
}

:deep(.permission-check-pill .ant-checkbox + span) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
  padding-inline-start: 0;
}

.permission-name {
  color: #0f172a;
  font-size: 13px;
  font-weight: 750;
  line-height: 1.35;
}

.permission-code {
  color: #64748b;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 11px;
  line-height: 1.3;
  word-break: break-all;
}

.permission-app-row {
  display: grid;
  grid-template-columns: minmax(190px, 1fr) minmax(260px, 460px);
  align-items: center;
  gap: 16px;
  padding: 16px;
}

.permission-app-meta {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.permission-app-name {
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-env-select {
  width: 100%;
}

:deep(.permission-app-env-select.ant-select .ant-select-selector) {
  min-height: 40px;
  border-radius: 14px !important;
  border-color: rgba(148, 163, 184, 0.22) !important;
  background: rgba(255, 255, 255, 0.66) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.72);
}

:deep(.permission-app-env-select.ant-select .ant-select-selection-placeholder) {
  color: rgba(30, 58, 138, 0.36) !important;
  font-weight: 600;
}

@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .permission-toolbar-user-select {
    width: 100%;
    flex: 1 1 100%;
  }

  .permission-section-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .permission-module-row,
  .permission-app-row {
    grid-template-columns: minmax(0, 1fr);
  }

  .permission-actions-grid {
    grid-template-columns: minmax(0, 1fr);
  }

}
</style>
