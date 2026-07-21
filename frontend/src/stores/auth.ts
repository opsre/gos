import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getMe, login as loginAPI, logout as logoutAPI } from '../api/auth'
import type { UserParamPermission, UserPermission, UserProfile } from '../types/user'

const TOKEN_KEY = 'gos_access_token'
const APPLICATION_ENV_SCOPE_SEPARATOR = '::'

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string>(localStorage.getItem(TOKEN_KEY) || '')
  const profile = ref<UserProfile | null>(null)
  const permissions = ref<UserPermission[]>([])
  const paramPermissions = ref<UserParamPermission[]>([])
  const meLoaded = ref(false)

  const isAuthenticated = computed(() => Boolean(accessToken.value))
  const isAdmin = computed(() => profile.value?.role === 'admin')
  const releaseScopedPermissionCodes = new Set([
    'release.view',
    'release.create',
    'release.execute',
    'release.cancel',
  ])

  function setToken(token: string) {
    const value = String(token || '').trim()
    accessToken.value = value
    if (value) {
      localStorage.setItem(TOKEN_KEY, value)
    } else {
      localStorage.removeItem(TOKEN_KEY)
    }
  }

  function clearAuthState() {
    setToken('')
    profile.value = null
    permissions.value = []
    paramPermissions.value = []
    meLoaded.value = false
  }

  async function login(username: string, password: string) {
    const response = await loginAPI({ username, password })
    setToken(response.data.access_token)
    try { localStorage.removeItem('gos-seen-announcement-ids') } catch { /* ignore */ }
    try { localStorage.removeItem('gos-seen-workflow-notification-ids') } catch { /* ignore */ }
    await loadMe(true)
  }

  async function logout() {
    try {
      if (accessToken.value) {
        try {
          await logoutAPI()
        } catch (error) {
          console.warn('[auth] logout request failed, clearing local auth state anyway', error)
        }
      }
    } finally {
      clearAuthState()
    }
  }

  async function loadMe(force = false) {
    if (!accessToken.value) {
      clearAuthState()
      return
    }
    if (!force && meLoaded.value && profile.value) {
      return
    }
    const response = await getMe()
    profile.value = response.data.user
    permissions.value = response.data.permissions || []
    paramPermissions.value = response.data.param_permissions || []
    meLoaded.value = true
  }

  function normalizePermissionCode(value: string) {
    return String(value || '').trim().toLowerCase()
  }

  function normalizeScopeType(value: string) {
    return String(value || '').trim().toLowerCase()
  }

  function parseApplicationEnvScopeValue(value: string) {
    const raw = String(value || '').trim()
    if (!raw) {
      return null
    }
    const segments = raw.split(APPLICATION_ENV_SCOPE_SEPARATOR)
    if (segments.length !== 2) {
      return null
    }
    const applicationID = String(segments[0] || '').trim()
    const envCode = String(segments[1] || '').trim()
    if (!applicationID || !envCode) {
      return null
    }
    return { applicationID, envCode }
  }

  function hasPermission(permissionCode: string) {
    const code = String(permissionCode || '').trim()
    if (!code) {
      return false
    }
    const normalizedCode = normalizePermissionCode(code)
    if (isAdmin.value) {
      return true
    }
    if (releaseScopedPermissionCodes.has(normalizedCode)) {
      return permissions.value.some((item) => {
        if (!item.enabled || normalizePermissionCode(item.permission_code) !== normalizedCode) {
          return false
        }
        const scopeType = normalizeScopeType(item.scope_type)
        const scopeValue = String(item.scope_value || '').trim()
        if (scopeType === 'application') {
          return scopeValue !== ''
        }
        if (scopeType === 'application_env') {
          return Boolean(parseApplicationEnvScopeValue(scopeValue))
        }
        return false
      })
    }
    return permissions.value.some(
      (item) => item.enabled && normalizePermissionCode(item.permission_code) === normalizedCode,
    )
  }

  function hasApplicationPermission(permissionCode: string, applicationID: string, envCode = '') {
    const code = normalizePermissionCode(permissionCode)
    const appID = String(applicationID || '').trim()
    const env = String(envCode || '').trim()
    if (!code || !appID) {
      return false
    }
    if (isAdmin.value) {
      return true
    }
    return permissions.value.some((item) => {
      if (!item.enabled || normalizePermissionCode(item.permission_code) !== code) {
        return false
      }
      const scopeType = normalizeScopeType(item.scope_type)
      const scopeValue = String(item.scope_value || '').trim()
      if (scopeType === 'application') {
        return scopeValue === appID
      }
      if (scopeType === 'application_env') {
        const parsed = parseApplicationEnvScopeValue(scopeValue)
        if (!parsed || parsed.applicationID !== appID) {
          return false
        }
        if (!env) {
          return true
        }
        return parsed.envCode === env
      }
      return false
    })
  }

  function listApplicationPermissionEnvCodes(
    permissionCode: string,
    applicationID: string,
    validEnvOptions: string[] = [],
  ) {
    const code = normalizePermissionCode(permissionCode)
    const appID = String(applicationID || '').trim()
    if (!code || !appID) {
      return []
    }
    const normalizedEnvOptions = Array.from(
      new Set(validEnvOptions.map((item) => String(item || '').trim()).filter(Boolean)),
    )
    if (isAdmin.value) {
      return normalizedEnvOptions
    }

    const result = new Set<string>()
    permissions.value.forEach((item) => {
      if (!item.enabled || normalizePermissionCode(item.permission_code) !== code) {
        return
      }
      const scopeType = normalizeScopeType(item.scope_type)
      const scopeValue = String(item.scope_value || '').trim()
      if (scopeType === 'application' && scopeValue === appID) {
        normalizedEnvOptions.forEach((env) => result.add(env))
        return
      }
      if (scopeType === 'application_env') {
        const parsed = parseApplicationEnvScopeValue(scopeValue)
        if (!parsed || parsed.applicationID !== appID) {
          return
        }
        if (normalizedEnvOptions.length === 0 || normalizedEnvOptions.includes(parsed.envCode)) {
          result.add(parsed.envCode)
        }
      }
    })
    return normalizedEnvOptions.filter((env) => result.has(env))
  }

  function resolveParamPermission(paramKey: string, applicationID: string) {
    const key = String(paramKey || '').trim().toLowerCase()
    const appID = String(applicationID || '').trim()
    if (!key) {
      return { canView: false, canEdit: false }
    }
    if (isAdmin.value) {
      return { canView: true, canEdit: true }
    }

    let globalRule: UserParamPermission | null = null
    let appRule: UserParamPermission | null = null
    for (const item of paramPermissions.value) {
      if (String(item.param_key || '').trim().toLowerCase() !== key) {
        continue
      }
      const scopeAppID = String(item.application_id || '').trim()
      if (!scopeAppID) {
        globalRule = item
        continue
      }
      if (scopeAppID === appID) {
        appRule = item
      }
    }
    const target = appRule || globalRule
    if (!target) {
      return { canView: false, canEdit: false }
    }
    return { canView: Boolean(target.can_view), canEdit: Boolean(target.can_edit) }
  }

  function canViewParam(paramKey: string, applicationID: string) {
    return resolveParamPermission(paramKey, applicationID).canView
  }

  function canEditParam(paramKey: string, applicationID: string) {
    return resolveParamPermission(paramKey, applicationID).canEdit
  }

  return {
    accessToken,
    profile,
    permissions,
    paramPermissions,
    meLoaded,
    isAuthenticated,
    isAdmin,
    setToken,
    clearAuthState,
    login,
    logout,
    loadMe,
    hasPermission,
    hasApplicationPermission,
    listApplicationPermissionEnvCodes,
    canViewParam,
    canEditParam,
  }
})
