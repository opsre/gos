<script setup lang="ts">
import {
  AppstoreOutlined,
  ClusterOutlined,
  DatabaseOutlined,
  HomeOutlined,
  LeftOutlined,
  LogoutOutlined,
  RightOutlined,
  RocketOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listReleaseApprovalWorkbenchTasks } from '../api/release'
import { useAuthStore } from '../stores/auth'
import { releaseApprovalTasksChangedEvent } from '../utils/release-approval-events'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const siderCollapsed = ref(false)
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const hasPendingApprovalTasks = ref(false)
let pendingApprovalRefreshTimer: ReturnType<typeof window.setInterval> | null = null

const activeMenuKey = computed(() => {
  if (route.path.startsWith('/system/users')) {
    return ['system-users']
  }
  if (route.path.startsWith('/system/permissions')) {
    return ['system-permissions']
  }
  if (route.path.startsWith('/system/settings')) {
    return ['system-settings']
  }
  if (route.path.startsWith('/system/notifications')) {
    return ['system-notifications']
  }
  if (route.path.includes('/pipeline-bindings')) {
    return ['pipeline-bindings']
  }
  if (route.path.startsWith('/projects')) {
    return ['project-management']
  }
  if (route.path.startsWith('/platform-param-dicts')) {
    return ['platform-param-dicts']
  }
  if (route.path.startsWith('/components/executor-params')) {
    return ['executor-param-management']
  }
  if (route.path.startsWith('/components/jenkins')) {
    return ['jenkins-pipeline-list']
  }
  if (route.path.startsWith('/components/pipeline-rules')) {
    return ['pipeline-rule-management']
  }
  if (route.path.startsWith('/artifacts/repositories')) {
    return ['artifact-repository-config']
  }
  if (route.path.startsWith('/artifacts')) {
    return ['artifact-center-list']
  }
  if (route.path.startsWith('/components/argocd/applications')) {
    return ['argocd-application-management']
  }
  if (route.path.startsWith('/components/argocd')) {
    return ['argocd-management']
  }
  if (route.path.startsWith('/components/gitops')) {
    return ['gitops-management']
  }
  if (route.path.startsWith('/components/agents')) {
    return ['agent-overview']
  }
  if (route.path.startsWith('/components/agent-scripts')) {
    return ['agent-script-management']
  }
  if (route.path.startsWith('/components/agent-tasks')) {
    return ['agent-task-management']
  }
  if (route.path.startsWith('/release-search')) {
    return ['release-home']
  }
  if (route.path.startsWith('/releases')) {
    return ['release-orders']
  }
  if (route.path.startsWith('/release-schedules')) {
    return ['release-order-schedules']
  }
  if (route.path.startsWith('/release-approvals')) {
    return ['release-approval-workbench']
  }
  if (route.path.startsWith('/release-templates')) {
    return ['release-templates']
  }
  if (route.path.startsWith('/release-approval-flows')) {
    return ['release-approval-flows']
  }
  if (route.path.startsWith('/applications')) {
    return ['my-applications']
  }
  return []
})

const openMenuKeys = computed(() => {
  if (route.path.startsWith('/components/')) {
    if (
      route.path.startsWith('/components/jenkins') ||
      route.path.startsWith('/components/pipeline-rules') ||
      route.path.startsWith('/components/executor-params')
    ) {
      return ['component-management', 'jenkins-management-group']
    }
    if (
      route.path.startsWith('/components/agents') ||
      route.path.startsWith('/components/agent-scripts') ||
      route.path.startsWith('/components/agent-tasks')
    ) {
      return ['component-management', 'agent-management-group']
    }
    return ['component-management']
  }
  if (route.path.startsWith('/releases')) {
    return ['release-management']
  }
  if (route.path.startsWith('/release-search')) {
    return []
  }
  if (route.path.startsWith('/release-schedules')) {
    return ['release-management']
  }
  if (route.path.startsWith('/release-approvals')) {
    return ['release-management']
  }
  if (route.path.startsWith('/release-templates')) {
    return ['release-management']
  }
  if (route.path.startsWith('/release-approval-flows')) {
    return ['release-management']
  }
  if (route.path.startsWith('/artifacts')) {
    return ['artifact-center']
  }
  if (
    route.path.startsWith('/applications') ||
    route.path.startsWith('/projects') ||
    route.path.startsWith('/platform-param-dicts')
  ) {
    return ['application-management']
  }
  if (route.path.startsWith('/system/')) {
    return ['system-management']
  }
  return []
})
const visibleOpenMenuKeys = computed(() => (siderCollapsed.value ? [] : openMenuKeys.value))

const displayName = computed(() => {
  const name = String(authStore.profile?.display_name || '').trim()
  if (name) {
    return name
  }
  return String(authStore.profile?.username || '')
})
const roleText = computed(() => (authStore.isAdmin ? '管理员' : '普通用户'))

const canViewApplications = computed(() => authStore.hasPermission('application.view'))
const canManageApplications = computed(() => authStore.hasPermission('application.manage'))
const canCreateRelease = computed(() => authStore.hasPermission('release.create'))
const canViewPipeline = computed(() => authStore.hasPermission('pipeline.view'))
const canManagePipeline = computed(() => authStore.hasPermission('pipeline.manage'))
const canManageArtifactRepository = computed(() => authStore.hasPermission('artifact_repo.manage'))
const canViewArtifactCenter = computed(
  () =>
    canManageArtifactRepository.value ||
    ['release.view', 'release.create', 'release.execute', 'release.cancel'].some((code) => authStore.hasPermission(code)),
)
const canManagePlatformParam = computed(() => authStore.hasPermission('platform_param.manage'))
const canViewComponent = computed(() => authStore.hasPermission('component.view'))
const canManagePipelineParam = computed(() => authStore.hasPermission('pipeline_param.manage'))
const canViewArgoCD = computed(
  () =>
    [
      'component.argocd.view',
      'component.argocd.manage',
      'component.argocd.instance.view',
      'component.argocd.instance.manage',
      'component.argocd.binding.view',
      'component.argocd.binding.manage',
    ].some((code) => authStore.hasPermission(code)),
)
const canViewGitOps = computed(
  () => authStore.hasPermission('component.gitops.view') || authStore.hasPermission('component.gitops.manage'),
)
const canViewAgent = computed(
  () => authStore.hasPermission('component.agent.view') || authStore.hasPermission('component.agent.manage'),
)
const canManageReleaseTemplate = computed(() => authStore.hasPermission('release.template.manage'))
const canManageUser = computed(() => authStore.hasPermission('system.user.manage'))
const canManagePermission = computed(() => authStore.hasPermission('system.permission.manage'))
const canManageNotification = computed(() => authStore.hasPermission('system.notification.manage'))

const showApplicationMenu = computed(() => true)
const showComponentMenu = computed(
  () =>
    canViewComponent.value ||
    canManagePipeline.value ||
    canManagePipelineParam.value ||
    canViewArgoCD.value ||
    canViewGitOps.value ||
    canViewAgent.value,
)
const showReleaseMenu = computed(() => true)
const showArtifactMenu = computed(() => canViewArtifactCenter.value || canManageArtifactRepository.value)
const showSystemMenu = computed(() => canManageUser.value || canManagePermission.value || canManageNotification.value)
const siderExpandedWidth = computed(() => {
  if (viewportWidth.value <= 768) {
    return 180
  }
  if (viewportWidth.value <= 1024) {
    return 200
  }
  return 220
})
const currentSiderWidth = computed(() => (siderCollapsed.value ? 0 : siderExpandedWidth.value))

function goToApplications() {
  void router.push('/applications')
}

function goToPipelineBindings() {
  const appID = String(route.params.id || route.query.application_id || '').trim()
  if (appID) {
    void router.push(`/applications/${appID}/pipeline-bindings`)
    return
  }
  message.info('请先进入具体应用，再查看管线绑定')
  void router.push('/applications')
}

function goToPlatformParamDicts() {
  void router.push('/platform-param-dicts')
}

function goToProjectManagement() {
  void router.push('/projects')
}

function goToJenkinsManagement() {
  void router.push('/components/jenkins')
}

function goToPipelineRuleManagement() {
  void router.push('/components/pipeline-rules')
}

function goToExecutorParamManagement() {
  const appID = String(route.params.id || route.query.application_id || '').trim()
  if (appID) {
    void router.push(`/components/executor-params?application_id=${encodeURIComponent(appID)}&binding_type=ci`)
    return
  }
  void router.push('/components/executor-params')
}

function goToArgoCDManagement() {
  void router.push('/components/argocd')
}

function goToArgoCDApplications() {
  void router.push('/components/argocd/applications')
}

function goToGitOpsManagement() {
  void router.push('/components/gitops')
}

function goToAgentManagement() {
  void router.push('/components/agents')
}

function goToAgentScriptManagement() {
  void router.push('/components/agent-scripts')
}

function goToAgentTaskManagement() {
  void router.push('/components/agent-tasks')
}

function goToReleaseOrders() {
  void router.push('/releases')
}

function goToReleaseSearch() {
  void router.push('/release-search')
}

function goToReleaseSchedules() {
  void router.push('/release-schedules')
}

function goToReleaseTemplates() {
  void router.push('/release-templates')
}

function goToReleaseApprovalFlows() {
  void router.push('/release-approval-flows')
}

function goToReleaseApprovalWorkbench() {
  void router.push('/release-approvals')
}

function goToArtifactRepositoryConfig() {
  void router.push('/artifacts/repositories')
}

function goToArtifactCenter() {
  void router.push('/artifacts')
}

function goToSystemUsers() {
  void router.push('/system/users')
}

function goToSystemPermissions() {
  void router.push('/system/permissions')
}

function goToSystemNotifications() {
  void router.push('/system/notifications')
}

function goToSystemSettings() {
  void router.push('/system/settings')
}

async function handleLogout() {
  await authStore.logout()
  message.success('已退出登录')
  await router.replace('/login')
}

function handleResize() {
  viewportWidth.value = window.innerWidth
  if (viewportWidth.value <= 640) {
    siderCollapsed.value = true
  }
}

function toggleSider() {
  siderCollapsed.value = !siderCollapsed.value
}

async function refreshPendingApprovalIndicator() {
  try {
    const response = await listReleaseApprovalWorkbenchTasks({ page: 1, page_size: 1 })
    hasPendingApprovalTasks.value = response.total > 0
  } catch {
    // Keep the previous state when a background refresh temporarily fails.
  }
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') {
    void refreshPendingApprovalIndicator()
  }
}

onMounted(() => {
  handleResize()
  void refreshPendingApprovalIndicator()
  pendingApprovalRefreshTimer = window.setInterval(() => {
    void refreshPendingApprovalIndicator()
  }, 30_000)
  window.addEventListener('resize', handleResize)
  window.addEventListener('focus', refreshPendingApprovalIndicator)
  window.addEventListener(releaseApprovalTasksChangedEvent, refreshPendingApprovalIndicator)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (pendingApprovalRefreshTimer !== null) {
    window.clearInterval(pendingApprovalRefreshTimer)
    pendingApprovalRefreshTimer = null
  }
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('focus', refreshPendingApprovalIndicator)
  window.removeEventListener(releaseApprovalTasksChangedEvent, refreshPendingApprovalIndicator)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<template>
  <a-layout class="app-layout" :style="{ '--layout-sider-width': `${currentSiderWidth}px` }">
    <a-layout-sider
      v-model:collapsed="siderCollapsed"
      class="app-sider"
      :class="{ 'app-sider-collapsed': siderCollapsed }"
      theme="dark"
      :width="siderExpandedWidth"
      :collapsed-width="0"
      :trigger="null"
      collapsible
    >
      <div class="sider-brand" @click="goToReleaseSearch">
        <img class="brand-mark" src="/logo-mark.svg?v=20260829-b" alt="" />
        <div class="brand-copy">
          <div class="brand-title">GOS Release</div>
        </div>
      </div>
      <a-menu
        mode="inline"
        theme="dark"
        :selected-keys="activeMenuKey"
        :open-keys="visibleOpenMenuKeys"
        class="sider-menu"
      >
        <a-menu-item key="release-home" @click="goToReleaseSearch">
          <template #icon>
            <HomeOutlined />
          </template>
          首页
        </a-menu-item>

        <a-sub-menu v-if="showApplicationMenu" key="application-management">
          <template #icon>
            <AppstoreOutlined />
          </template>
          <template #title>应用管理</template>

          <a-menu-item key="my-applications" @click="goToApplications">
            我的应用
          </a-menu-item>
          <a-menu-item v-if="canViewPipeline" key="pipeline-bindings" @click="goToPipelineBindings">
            管线绑定
          </a-menu-item>
          <a-menu-item v-if="canManageApplications" key="project-management" @click="goToProjectManagement">
            项目管理
          </a-menu-item>
          <a-menu-item v-if="canManagePlatformParam" key="platform-param-dicts" @click="goToPlatformParamDicts">
            标准字库
          </a-menu-item>
        </a-sub-menu>

        <a-sub-menu v-if="showReleaseMenu" key="release-management">
          <template #icon>
            <RocketOutlined />
          </template>
          <template #title>发布管理</template>

          <a-menu-item key="release-orders" @click="goToReleaseOrders">发布单</a-menu-item>
          <a-menu-item key="release-order-schedules" @click="goToReleaseSchedules">预约发布</a-menu-item>
          <a-menu-item key="release-approval-workbench" @click="goToReleaseApprovalWorkbench">
            <span class="sidebar-approval-menu-label">
              <span>审批待办</span>
              <span v-if="hasPendingApprovalTasks" class="sidebar-approval-menu-dot" aria-label="存在待审批任务"></span>
            </span>
          </a-menu-item>
          <a-menu-item v-if="canManageReleaseTemplate" key="release-templates" @click="goToReleaseTemplates">
            发布模板
          </a-menu-item>
          <a-menu-item v-if="canManageReleaseTemplate" key="release-approval-flows" @click="goToReleaseApprovalFlows">
            审批流管理
          </a-menu-item>
        </a-sub-menu>

        <a-sub-menu v-if="showArtifactMenu" key="artifact-center">
          <template #icon>
            <DatabaseOutlined />
          </template>
          <template #title>制品中心</template>

          <a-menu-item v-if="canViewArtifactCenter" key="artifact-center-list" @click="goToArtifactCenter">
            制品目录
          </a-menu-item>
          <a-menu-item
            v-if="canManageArtifactRepository"
            key="artifact-repository-config"
            @click="goToArtifactRepositoryConfig"
          >
            制品库配置
          </a-menu-item>
        </a-sub-menu>

        <a-sub-menu v-if="showComponentMenu" key="component-management">
          <template #icon>
            <ClusterOutlined />
          </template>
          <template #title>组件管理</template>

          <a-sub-menu v-if="canViewComponent || canManagePipeline || canManagePipelineParam" key="jenkins-management-group">
            <template #title>Jenkins管理</template>

            <a-menu-item v-if="canViewComponent" key="jenkins-pipeline-list" @click="goToJenkinsManagement">
              管线列表
            </a-menu-item>
            <a-menu-item v-if="canManagePipeline" key="pipeline-rule-management" @click="goToPipelineRuleManagement">
              管线规范
            </a-menu-item>
            <a-menu-item v-if="canManagePipelineParam" key="executor-param-management" @click="goToExecutorParamManagement">
              执行器参数
            </a-menu-item>
          </a-sub-menu>

          <a-menu-item v-if="canViewArgoCD" key="argocd-management" @click="goToArgoCDManagement">
            ArgoCD管理
          </a-menu-item>
          <a-menu-item v-if="canViewArgoCD" key="argocd-application-management" @click="goToArgoCDApplications">
            ArgoCD应用
          </a-menu-item>
          <a-menu-item v-if="canViewGitOps" key="gitops-management" @click="goToGitOpsManagement">
            GitOps管理
          </a-menu-item>
          <a-sub-menu v-if="canViewAgent" key="agent-management-group">
            <template #title>Agent管理</template>

            <a-menu-item key="agent-overview" @click="goToAgentManagement">
              Agent概览
            </a-menu-item>
            <a-menu-item key="agent-script-management" @click="goToAgentScriptManagement">
              脚本管理
            </a-menu-item>
            <a-menu-item key="agent-task-management" @click="goToAgentTaskManagement">
              任务管理
            </a-menu-item>
          </a-sub-menu>
        </a-sub-menu>

        <a-sub-menu v-if="showSystemMenu" key="system-management">
          <template #icon>
            <SettingOutlined />
          </template>
          <template #title>系统管理</template>

          <a-menu-item v-if="canManageUser" key="system-users" @click="goToSystemUsers">
            用户管理
          </a-menu-item>
          <a-menu-item v-if="canManagePermission" key="system-permissions" @click="goToSystemPermissions">
            权限授权
          </a-menu-item>
          <a-menu-item v-if="canManageNotification" key="system-notifications" @click="goToSystemNotifications">
            通知模块
          </a-menu-item>
          <a-menu-item v-if="canManagePermission" key="system-settings" @click="goToSystemSettings">
            系统设置
          </a-menu-item>
        </a-sub-menu>
      </a-menu>
      <div class="sider-footer">
        <div class="sider-footer-row">
          <div class="sider-footer-version">
            <span>v1.3.2</span>
            <a href="https://github.com/yl1664907302/gos" target="_blank" class="github-link" title="访问 GitHub">
              <svg class="github-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 0C5.374 0 0 5.373 0 12c0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0112 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576C20.566 21.797 24 17.3 24 12c0-6.627-5.373-12-12-12z" fill="currentColor"/>
              </svg>
            </a>
          </div>
          <div class="sider-footer-role-group">
            <div class="sider-footer-role">{{ roleText }}</div>
            <button class="sider-footer-toggle" type="button" title="收起侧栏" @click="toggleSider" aria-label="收起侧栏">
              <LeftOutlined aria-hidden="true" />
            </button>
          </div>
        </div>
        <div class="sider-footer-user">
          <UserOutlined />
          <span>{{ displayName }}</span>
        </div>
        <a-button type="text" class="sider-footer-logout" @click="handleLogout">
          <template #icon>
            <LogoutOutlined />
          </template>
          退出登录
        </a-button>
      </div>
    </a-layout-sider>
    <button v-if="siderCollapsed" class="layout-sider-restore" type="button" title="展开侧栏" @click="toggleSider" aria-label="展开侧栏">
      <RightOutlined aria-hidden="true" />
    </button>

    <a-layout>
      <a-layout-content class="app-content">
        <router-view v-slot="{ Component, route }">
          <Transition name="layout-route-switch" mode="out-in">
            <component :is="Component" :key="route.fullPath" class="layout-route-view" />
          </Transition>
        </router-view>
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.055), transparent 30%),
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.035), transparent 26%),
    linear-gradient(180deg, #fbfdff 0%, #f8fbff 46%, #fbfcff 100%);
}

.app-sider {
  position: sticky;
  top: 0;
  align-self: flex-start;
  height: 100vh;
  min-height: 100vh;
  overflow: hidden;
  z-index: 20;
  isolation: isolate;
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.12), transparent 24%),
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.18), transparent 30%),
    linear-gradient(180deg, rgba(2, 6, 23, 0.99), rgba(15, 23, 42, 0.99) 42%, rgba(19, 30, 53, 0.99)) !important;
  border-inline-end: none;
  box-shadow:
    inset -1px 0 0 rgba(255, 255, 255, 0.04),
    18px 0 44px rgba(2, 6, 23, 0.28);
  transition:
    width 0.22s ease,
    min-width 0.22s ease,
    max-width 0.22s ease,
    flex-basis 0.22s ease,
    opacity 0.18s ease;
}

.sider-brand {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 88px;
  padding: 20px 22px 18px;
  color: #fff;
  border-bottom: 1px solid rgba(71, 85, 105, 0.34);
  cursor: pointer;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.28), rgba(15, 23, 42, 0));
}

.brand-mark {
  display: block;
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  box-shadow:
    0 10px 22px rgba(2, 6, 23, 0.18);
}

.brand-copy {
  min-width: 0;
}

.brand-title {
  color: #f8fafc;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.02em;
}


.sider-menu {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  border-inline-end: none;
  padding: 14px 10px 18px;
  background: transparent !important;
}

.app-content {
  width: 100%;
  max-width: none;
  margin: 0;
  padding: 28px 28px 32px;
  position: relative;
  box-sizing: border-box;
  overflow: hidden;
}

.layout-route-view {
  min-height: calc(100vh - 60px);
}

.layout-route-switch-enter-active,
.layout-route-switch-leave-active {
  transition: opacity 0.12s ease;
}

.layout-route-switch-enter-from {
  opacity: 0;
}

.layout-route-switch-leave-to {
  opacity: 0;
}

.sider-footer-toggle,
.layout-sider-restore {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: rgba(191, 219, 254, 0.68);
  line-height: 1;
  cursor: pointer;
  transition:
    color 0.18s ease,
    transform 0.18s ease,
    border-color 0.18s ease,
    background 0.18s ease;
}

.sider-footer-toggle {
  width: 28px;
  min-width: 28px;
  height: 28px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.06);
  font-size: 12px;
}

.layout-sider-restore {
  position: fixed;
  top: 50%;
  left: 10px;
  z-index: 36;
  width: 30px;
  min-width: 30px;
  height: 52px;
  border: 1px solid rgba(96, 165, 250, 0.24);
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.9);
  box-shadow: 0 10px 24px rgba(2, 6, 23, 0.2);
  font-size: 13px;
  transform: translateY(-50%);
}

.sider-footer-toggle:hover,
.sider-footer-toggle:focus,
.sider-footer-toggle:focus-visible {
  color: #dbeafe;
  border-color: rgba(96, 165, 250, 0.16);
  background: rgba(96, 165, 250, 0.12);
  outline: none;
  transform: translateX(-1px);
}

.sider-footer-toggle:focus-visible {
  box-shadow: 0 0 0 2px rgba(96, 165, 250, 0.16);
}

.layout-sider-restore:hover,
.layout-sider-restore:focus,
.layout-sider-restore:focus-visible {
  color: #eff6ff;
  border-color: rgba(96, 165, 250, 0.46);
  background: rgba(30, 41, 59, 0.96);
  outline: none;
}

.layout-sider-restore:hover,
.layout-sider-restore:focus,
.layout-sider-restore:focus-visible {
  transform: translateY(calc(-50% - 1px));
}

.app-content::before {
  content: '';
  position: absolute;
  inset: 0 24px auto;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(96, 165, 250, 0.16), transparent);
  pointer-events: none;
}

.app-sider :deep(.ant-layout-sider-children) {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.app-sider-collapsed {
  box-shadow: none;
}

.app-sider-collapsed :deep(.ant-layout-sider-children) {
  pointer-events: none;
  visibility: hidden;
}

.sider-menu :deep(.ant-menu),
.sider-menu :deep(.ant-menu-dark),
.sider-menu :deep(.ant-menu-inline) {
  background: transparent !important;
}

.sider-menu :deep(.ant-menu-submenu-title),
.sider-menu :deep(.ant-menu-item) {
  height: 46px;
  margin: 6px 0;
  border-radius: 14px;
  color: rgba(226, 232, 240, 0.82) !important;
  border: 1px solid transparent;
  background: rgba(15, 23, 42, 0.18) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
  transition: all 0.2s ease;
}

.sider-menu :deep(.ant-menu-submenu-title:hover),
.sider-menu :deep(.ant-menu-item:hover) {
  color: #f8fafc !important;
  border-color: rgba(71, 85, 105, 0.34);
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 44%),
    rgba(30, 41, 59, 0.72) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 10px 18px rgba(2, 6, 23, 0.16);
}

.sider-menu :deep(.ant-menu-submenu-title .ant-menu-title-content),
.sider-menu :deep(.ant-menu-item .ant-menu-title-content) {
  font-weight: 600;
}

.sider-menu :deep(.ant-menu-submenu-title .anticon),
.sider-menu :deep(.ant-menu-item .anticon) {
  color: rgba(191, 219, 254, 0.76);
}

.sider-menu :deep(.ant-menu-submenu-open > .ant-menu-submenu-title) {
  color: #f8fafc !important;
  border-color: rgba(71, 85, 105, 0.34);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.1), transparent 42%),
    rgba(30, 41, 59, 0.72) !important;
}

.sider-menu :deep(.ant-menu-submenu-selected > .ant-menu-submenu-title) {
  color: #f8fafc !important;
}

.sider-menu :deep(.ant-menu-item-selected) {
  color: #eff6ff !important;
  border: 1px solid rgba(56, 189, 248, 0.24);
  background:
    radial-gradient(circle at top right, rgba(56, 189, 248, 0.22), transparent 46%),
    linear-gradient(135deg, rgba(29, 78, 216, 0.58), rgba(37, 99, 235, 0.28)) !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.08),
    0 10px 20px rgba(2, 6, 23, 0.18);
}

.sider-menu :deep(.ant-menu-item-selected .anticon),
.sider-menu :deep(.ant-menu-item-selected .ant-menu-title-content) {
  color: #eff6ff !important;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline) {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  margin-top: 4px;
  padding: 4px 0 10px !important;
  background: transparent !important;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline::before) {
  content: '';
  position: absolute;
  top: 8px;
  bottom: 14px;
  left: 14px;
  width: 2px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(34, 211, 238, 0.8), rgba(59, 130, 246, 0.28));
  box-shadow: 0 0 12px rgba(34, 211, 238, 0.28);
  pointer-events: none;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline > .ant-menu-item),
.sider-menu :deep(.ant-menu-sub.ant-menu-inline > .ant-menu-submenu) {
  width: calc(100% - 34px);
  margin-inline-start: auto;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline .ant-menu-submenu-title),
.sider-menu :deep(.ant-menu-sub.ant-menu-inline .ant-menu-item) {
  margin-inline: 0;
  padding-inline-start: 22px !important;
  color: rgba(203, 213, 225, 0.84) !important;
  border-radius: 12px;
  background: rgba(2, 6, 23, 0.16) !important;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline .ant-menu-item::before) {
  display: none;
}

.sider-menu :deep(.ant-menu-sub.ant-menu-inline .ant-menu-item-selected::before) {
  display: none;
}

.sidebar-approval-menu-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.sidebar-approval-menu-dot {
  display: inline-block;
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #ef4444;
  box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.16);
}

.sider-footer {
  margin-top: auto;
  flex: 0 0 auto;
  padding: 16px 14px 18px;
  border-top: 1px solid rgba(71, 85, 105, 0.26);
  background: transparent;
}

.sider-footer-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.sider-footer-role-group {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.sider-footer-version {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.42);
  color: rgba(191, 219, 254, 0.6);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.05em;
}

.github-link {
  display: inline-flex;
  align-items: center;
  color: rgba(191, 219, 254, 0.7);
  transition: color 0.2s ease;
}

.github-link:hover {
  color: #fff;
}

.github-icon {
  width: 14px;
  height: 14px;
}

.sider-footer-role {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid rgba(56, 189, 248, 0.16);
  background: rgba(37, 99, 235, 0.16);
  color: #dbeafe;
  font-size: 12px;
  font-weight: 600;
}

.sider-footer-user {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  color: rgba(226, 232, 240, 0.82);
  font-size: 13px;
  font-weight: 500;
}

.sider-menu::-webkit-scrollbar {
  width: 6px;
}

.sider-menu::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(56, 189, 248, 0.7), rgba(34, 197, 94, 0.6));
}

.sider-menu::-webkit-scrollbar-track {
  background: transparent;
}

.sider-footer-logout {
  margin-top: 12px;
  width: 100%;
  justify-content: flex-start;
  border-radius: 12px;
  color: rgba(226, 232, 240, 0.82);
  border: 1px solid transparent;
  background: rgba(15, 23, 42, 0.18);
}

.sider-footer-logout:hover,
.sider-footer-logout:focus {
  color: #eff6ff;
  border-color: rgba(71, 85, 105, 0.34);
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 44%),
    rgba(30, 41, 59, 0.72);
}

@media (max-width: 1024px) {
  .app-content {
    padding: 20px;
  }
}

@media (max-width: 768px) {
  .sider-brand {
    padding: 0 12px;
    min-height: 76px;
    gap: 10px;
  }

  .brand-title {
    font-size: 16px;
  }

  .app-content {
    padding: 16px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .layout-route-switch-enter-active,
  .layout-route-switch-leave-active {
    transition: none;
  }
}
</style>
