import { message } from 'ant-design-vue'
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const AppLayout = () => import('../layouts/AppLayout.vue')
const ApplicationCreateView = () => import('../views/application/ApplicationCreateView.vue')
const ApplicationEditView = () => import('../views/application/ApplicationEditView.vue')
const ApplicationListView = () => import('../views/application/ApplicationListView.vue')
const ApplicationPipelineBindingView = () => import('../views/application/ApplicationPipelineBindingView.vue')
const ProjectManagementView = () => import('../views/application/ProjectManagementView.vue')
const PlatformParamDictView = () => import('../views/application/PlatformParamDictView.vue')
const ArtifactCenterView = () => import('../views/artifact/ArtifactCenterView.vue')
const ArtifactRepositoryConfigView = () => import('../views/artifact/ArtifactRepositoryConfigView.vue')
const ArgoCDManagementView = () => import('../views/component/ArgoCDManagementView.vue')
const ArgoCDApplicationManagementView = () => import('../views/component/ArgoCDApplicationManagementView.vue')
const GitOpsManagementView = () => import('../views/component/GitOpsManagementView.vue')
const GitOpsTutorialView = () => import('../views/help/GitOpsTutorialView.vue')
const JenkinsManagementView = () => import('../views/component/JenkinsManagementView.vue')
const PipelineRuleManagementView = () => import('../views/component/PipelineRuleManagementView.vue')
const ExecutorParamManagementView = () => import('../views/component/ExecutorParamManagementView.vue')
const AgentManagementView = () => import('../views/component/AgentManagementView.vue')
const AgentTaskManagementView = () => import('../views/component/AgentTaskManagementView.vue')
const AgentScriptManagementView = () => import('../views/component/AgentScriptManagementView.vue')
const ForbiddenView = () => import('../views/exception/ForbiddenView.vue')
const LoginView = () => import('../views/login/LoginView.vue')
const ReleaseOrderCreateView = () => import('../views/release/ReleaseOrderCreateView.vue')
const ReleaseOrderDetailView = () => import('../views/release/ReleaseOrderDetailView.vue')
const ReleaseOrderListView = () => import('../views/release/ReleaseOrderListView.vue')
const ReleaseOrderScheduleView = () => import('../views/release/ReleaseOrderScheduleView.vue')
const ReleaseApprovalWorkbenchView = () => import('../views/release/ReleaseApprovalWorkbenchView.vue')
const ApprovalFlowManagementView = () => import('../views/release/ApprovalFlowManagementView.vue')
const ReleaseTemplateView = () => import('../views/release/ReleaseTemplateView.vue')
const SystemNotificationView = () => import('../views/system/SystemNotificationView.vue')
const SystemPermissionView = () => import('../views/system/SystemPermissionView.vue')
const SystemSettingsView = () => import('../views/system/SystemSettingsView.vue')
const SystemUserView = () => import('../views/system/SystemUserView.vue')

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    public?: boolean
    permission?: string | string[]
  }
}

function normalizePermissions(metaPermission: string | string[] | undefined): string[] {
  if (!metaPermission) {
    return []
  }
  if (Array.isArray(metaPermission)) {
    return metaPermission.map((item) => String(item || '').trim()).filter(Boolean)
  }
  const value = String(metaPermission || '').trim()
  return value ? [value] : []
}

function resolveFirstAccessiblePath(authStore: ReturnType<typeof useAuthStore>) {
  void authStore
  return '/releases'
}

const ROUTE_CHUNK_RELOAD_KEY = 'gos-route-chunk-reload-path'
const ROUTE_CHUNK_RELOAD_QUERY = '__gos_reload'

function isDynamicImportLoadError(error: unknown) {
  const text = String((error as { message?: string })?.message || error || '').toLowerCase()
  return [
    'failed to fetch dynamically imported module',
    'importing a module script failed',
    'chunkloaderror',
    'unable to preload css',
    'loading css chunk',
    'loading chunk',
  ].some((pattern) => text.includes(pattern))
}

function buildReloadURL(fullPath: string) {
  const url = new URL(fullPath || '/', window.location.origin)
  url.searchParams.set(ROUTE_CHUNK_RELOAD_QUERY, String(Date.now()))
  return `${url.pathname}${url.search}${url.hash}`
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/applications',
    },
    {
      path: '/login',
      name: 'login',
      component: LoginView,
      meta: { title: '登录', public: true },
    },
    {
      path: '/403',
      name: 'forbidden',
      component: ForbiddenView,
      meta: { title: '无权限', public: true },
    },
    {
      path: '/',
      component: AppLayout,
      children: [
        {
          path: '/applications',
          name: 'application-list',
          component: ApplicationListView,
          meta: { title: '我的应用' },
        },
        {
          path: '/applications/new',
          name: 'application-create',
          component: ApplicationCreateView,
          meta: { title: '新增应用', permission: 'application.manage' },
        },
        {
          path: '/applications/:id/edit',
          name: 'application-edit',
          component: ApplicationEditView,
          meta: { title: '编辑应用', permission: 'application.manage' },
        },
        {
          path: '/applications/:id/pipeline-bindings',
          name: 'application-pipeline-bindings',
          component: ApplicationPipelineBindingView,
          meta: { title: '管线绑定', permission: ['pipeline.view', 'pipeline.manage'] },
        },
        {
          path: '/projects',
          name: 'project-management',
          component: ProjectManagementView,
          meta: { title: '项目管理', permission: 'application.manage' },
        },
        {
          path: '/platform-param-dicts',
          name: 'platform-param-dicts',
          component: PlatformParamDictView,
          meta: { title: '标准字库', permission: 'platform_param.manage' },
        },
        {
          path: '/components/jenkins',
          name: 'jenkins-management',
          component: JenkinsManagementView,
          meta: { title: '管线列表', permission: 'component.view' },
        },
        {
          path: '/components/pipeline-rules',
          name: 'pipeline-rule-management',
          component: PipelineRuleManagementView,
          meta: { title: '管线规范', permission: 'pipeline.manage' },
        },
        {
          path: '/components/argocd',
          name: 'argocd-management',
          component: ArgoCDManagementView,
          meta: {
            title: 'ArgoCD管理',
            permission: [
              'component.argocd.view',
              'component.argocd.manage',
              'component.argocd.instance.view',
              'component.argocd.instance.manage',
              'component.argocd.binding.view',
              'component.argocd.binding.manage',
            ],
          },
        },
        {
          path: '/components/argocd/applications',
          name: 'argocd-application-management',
          component: ArgoCDApplicationManagementView,
          meta: {
            title: 'ArgoCD应用',
            permission: [
              'component.argocd.view',
              'component.argocd.manage',
              'component.argocd.instance.view',
              'component.argocd.instance.manage',
            ],
          },
        },
        {
          path: '/components/gitops',
          name: 'gitops-management',
          component: GitOpsManagementView,
          meta: { title: 'GitOps管理', permission: ['component.gitops.view', 'component.gitops.manage'] },
        },
        {
          path: '/help/gitops',
          name: 'gitops-tutorial',
          component: GitOpsTutorialView,
          meta: { title: 'GitOps配置教程' },
        },
        {
          path: '/components/agents',
          name: 'agent-management',
          component: AgentManagementView,
          meta: { title: 'Agent管理', permission: ['component.agent.view', 'component.agent.manage'] },
        },
        {
          path: '/components/agent-scripts',
          name: 'agent-script-management',
          component: AgentScriptManagementView,
          meta: { title: '脚本管理', permission: ['component.agent.view', 'component.agent.manage'] },
        },
        {
          path: '/components/agent-tasks',
          name: 'agent-task-management',
          component: AgentTaskManagementView,
          meta: { title: '任务管理', permission: ['component.agent.view', 'component.agent.manage'] },
        },
        {
          path: '/components/executor-params',
          name: 'executor-param-management',
          component: ExecutorParamManagementView,
          meta: { title: '执行器参数', permission: 'pipeline_param.manage' },
        },
        {
          path: '/artifacts',
          name: 'artifact-center',
          component: ArtifactCenterView,
          meta: {
            title: '制品目录',
            permission: ['release.view', 'release.create', 'release.execute', 'release.cancel', 'artifact_repo.manage'],
          },
        },
        {
          path: '/artifacts/repositories',
          name: 'artifact-repository-config',
          component: ArtifactRepositoryConfigView,
          meta: { title: '制品库配置', permission: 'artifact_repo.manage' },
        },
        {
          path: '/releases',
          name: 'release-order-list',
          component: ReleaseOrderListView,
          meta: { title: '发布单' },
        },
        {
          path: '/release-approvals',
          name: 'release-approval-workbench',
          component: ReleaseApprovalWorkbenchView,
          meta: { title: '审批待办' },
        },
        {
          path: '/release-schedules',
          name: 'release-order-schedules',
          component: ReleaseOrderScheduleView,
          meta: { title: '预约发布' },
        },
        {
          path: '/releases/new',
          name: 'release-order-create',
          component: ReleaseOrderCreateView,
          meta: { title: '新建发布单', permission: 'release.create' },
        },
        {
          path: '/releases/:id/edit',
          name: 'release-order-edit',
          component: ReleaseOrderCreateView,
          meta: { title: '编辑发布单' },
        },
        {
          path: '/releases/:id',
          name: 'release-order-detail',
          component: ReleaseOrderDetailView,
          meta: { title: '发布单详情' },
        },
        {
          path: '/release-templates',
          name: 'release-template-list',
          component: ReleaseTemplateView,
          meta: { title: '发布模板', permission: ['release.template.manage', 'release.create'] },
        },
        {
          path: '/release-approval-flows',
          name: 'release-approval-flow-management',
          component: ApprovalFlowManagementView,
          meta: { title: '审批流管理', permission: 'release.template.manage' },
        },
        {
          path: '/system',
          redirect: '/system/settings',
        },
        {
          path: '/system/quick-start',
          redirect: '/system/settings',
        },
        {
          path: '/system/users',
          name: 'system-users',
          component: SystemUserView,
          meta: { title: '用户管理', permission: 'system.user.manage' },
        },
        {
          path: '/system/permissions',
          name: 'system-permissions',
          component: SystemPermissionView,
          meta: { title: '权限授权', permission: 'system.permission.manage' },
        },
        {
          path: '/system/notifications',
          name: 'system-notifications',
          component: SystemNotificationView,
          meta: { title: '通知模块', permission: 'system.notification.manage' },
        },
        {
          path: '/system/settings',
          name: 'system-settings',
          component: SystemSettingsView,
          meta: { title: '系统设置', permission: 'system.permission.manage' },
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const authStore = useAuthStore()
  const token = String(authStore.accessToken || '').trim()
  const isPublic = Boolean(to.meta.public)

  if (isPublic) {
    // 登录页始终可访问，方便已登录用户切换账号
    return true
  }

  if (!token) {
    return {
      path: '/login',
      query: { redirect: to.fullPath },
    }
  }

  try {
    await authStore.loadMe(true)
  } catch {
    authStore.clearAuthState()
    return {
      path: '/login',
      query: { redirect: to.fullPath },
    }
  }

  const permissions = normalizePermissions(to.meta.permission)
  if (permissions.length === 0) {
    return true
  }

  const allowed = permissions.some((item) => authStore.hasPermission(item))
  if (!allowed) {
    const fallback = resolveFirstAccessiblePath(authStore)
    if (fallback !== '/403' && fallback !== to.path) {
      message.warning('当前账号无该页面权限，已跳转到可访问页面')
      return { path: fallback }
    }
    message.warning('当前账号没有访问该页面的权限')
    return { path: '/403' }
  }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · GOS Release` : 'GOS Release'
  sessionStorage.removeItem(ROUTE_CHUNK_RELOAD_KEY)
})

router.onError((error, to) => {
  if (!isDynamicImportLoadError(error)) {
    console.error('[router] navigation error:', error)
    message.error('页面加载失败，请刷新后重试')
    return
  }

  const targetPath = String(to?.fullPath || window.location.pathname || '/')
  const reloadedPath = sessionStorage.getItem(ROUTE_CHUNK_RELOAD_KEY)
  if (reloadedPath === targetPath) {
    sessionStorage.removeItem(ROUTE_CHUNK_RELOAD_KEY)
    message.error('页面资源已更新，请关闭当前页后重新打开')
    return
  }

  sessionStorage.setItem(ROUTE_CHUNK_RELOAD_KEY, targetPath)
  window.location.replace(buildReloadURL(targetPath))
})
