<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { createOnboardingSession, getApplicationSetupStatus, getOnboardingStatus } from '../../api/onboarding'
import { useAuthStore } from '../../stores/auth'
import type { ApplicationSetupStatus, OnboardingStatus } from '../../types/onboarding'
import { extractHTTPErrorMessage } from '../../utils/http-error'
const route = useRoute(), router = useRouter(), auth = useAuthStore()
const status = ref<OnboardingStatus>(), loading = ref(false), starting = ref(false), error = ref('')
const setup = ref<ApplicationSetupStatus>(), checking = ref(false), setupError = ref('')
const canCheck = computed(() => ['application.manage', 'release.template.manage', 'pipeline.view', 'platform_param.manage', 'pipeline_param.manage'].every(code => auth.hasPermission(code)))
const checkLabels = { ready: '配置完成', blocked: '待完善', unknown: '暂时无法确认' }
watch(() => route.query.application_id, () => { setup.value = undefined; setupError.value = '' })
async function inspectApplication() {
  const id = String(route.query.application_id || '')
  if (!id || checking.value) return
  checking.value = true; setupError.value = ''
  try {
    const result = await getApplicationSetupStatus(id)
    if (id === String(route.query.application_id || '')) setup.value = result
  } catch (e) { setupError.value = extractHTTPErrorMessage(e, '无法检查现有配置') } finally { checking.value = false }
}
const labels: Record<string, string> = { draft: '草稿', in_progress: '配置中', blocked: '待完善', configured: '配置完成', verified: '首单已验证', abandoned: '已放弃' }
async function load() {
  loading.value = true; error.value = ''
  try { status.value = await getOnboardingStatus() } catch (e) { error.value = extractHTTPErrorMessage(e, '无法读取接入状态') } finally { loading.value = false }
}
async function start(applicationID = '') {
  if (starting.value) return
  starting.value = true
  try {
    const session = await createOnboardingSession({ mode: applicationID ? 'complete_application' : 'create_application', application_id: applicationID || undefined, project_id: String(route.query.project_id || '') || undefined })
    await router.push(`/onboarding/${session.id}`)
  } catch (e) { message.error(extractHTTPErrorMessage(e, '创建接入任务失败')) } finally { starting.value = false }
}
onMounted(load)
</script>
<template>
  <main class="onboarding-center">
    <header><div><span class="eyebrow">从首次配置到日常新增</span><h1>应用接入向导</h1><p>把项目、已有管线、参数和发布模板一次串起来。已有公共配置可复用，每个应用独立接入。</p></div>
      <a-button type="primary" size="large" :loading="starting" @click="start()">新增应用（引导）</a-button>
    </header>
    <a-alert v-if="error" :message="error" type="error" show-icon><template #action><a-button @click="load">重试</a-button></template></a-alert>
    <a-alert v-if="route.query.application_id" type="info" show-icon message="继续配置已有应用，不会覆盖它的绑定、模板或权限。"><template #action><a-button :loading="starting" @click="start(String(route.query.application_id))">继续配置此应用</a-button></template></a-alert>
    <a-card v-if="route.query.application_id" title="现有配置检查">
      <p>检查真实绑定和模板，不需要先创建接入任务。一个模板异常不影响其他有效模板。</p>
      <a-button v-if="canCheck" :loading="checking" @click="inspectApplication">{{ setup ? '重新检查' : '检查此应用配置' }}</a-button>
      <p v-else>检查需要应用、模板、标准字段、执行器参数管理及管线查看权限，请管理员协助。</p>
      <a-alert v-if="setupError" :message="setupError" type="error" show-icon />
      <div v-if="setup" class="setup-results">
        <a-alert :message="checkLabels[setup.status]" :type="setup.status === 'ready' ? 'success' : 'warning'" show-icon />
        <p v-if="!setup.templates.length">尚无发布模板，点击上方“继续配置此应用”补齐。</p>
        <article v-for="item in setup.templates" :key="item.template_id" class="setup-template">
          <strong>{{ item.template_name }}</strong> <a-tag>{{ checkLabels[item.check.status] }}</a-tag>
          <p v-for="(issue, index) in item.check.issues" :key="index">{{ issue.message }}</p>
          <a-button v-if="item.check.status === 'ready'" @click="router.push({path: '/releases/new', query: {application_id: setup.application_id, template_id: item.template_id}})">使用此模板创建发布单</a-button>
        </article>
      </div>
    </a-card>
    <section class="guide-cards"><article><b>① 选择项目与管线</b><p>已有项目直接选，也可以在引导内新建。只绑定已有管线，不生成脚本。</p></article><article><b>② 就地配置参数</b><p>复用已有标准映射，缺少的字段当前页面补齐，发布表单实时预览。</p></article><article><b>③ 完成并持续复用</b><p>生成独立模板，创建首单或继续新增应用。中途离开也能恢复。</p></article></section>
    <a-spin :spinning="loading"><a-card title="我的接入任务">
      <a-empty v-if="!status?.sessions.length" description="还没有接入任务，点击上方按钮开始" />
      <div v-for="session in status?.sessions" :key="session.id" class="session-row"><div><strong>{{ session.draft.identity.name || '未命名应用' }}</strong><p>{{ session.draft.identity.project_name || session.draft.identity.project_id || '项目待选择' }} · {{ new Date(session.updated_at).toLocaleString() }}</p></div><a-tag>{{ labels[session.status] || session.status }}</a-tag><a-button @click="router.push(`/onboarding/${session.id}`)">{{ ['configured', 'verified', 'abandoned'].includes(session.status) ? '查看' : '继续配置' }}</a-button></div>
    </a-card></a-spin>
    <footer><a-button @click="router.push('/applications')">返回应用列表</a-button><a-button v-if="auth.hasPermission('application.manage')" type="link" @click="router.push('/applications/new')">高级创建</a-button></footer>
  </main>
</template>
<style scoped>
.onboarding-center{max-width:1280px;margin:0 auto;padding:28px;display:grid;gap:24px;min-width:0}.onboarding-center header{display:flex;align-items:center;justify-content:space-between;gap:24px;flex-wrap:wrap}.onboarding-center h1{font-size:30px;margin:8px 0}.onboarding-center p{color:#64748b;margin:8px 0;line-height:1.8}.eyebrow{font-size:13px;color:#2563eb;letter-spacing:1px}.guide-cards{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px}.guide-cards article{background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:24px}.session-row{display:flex;gap:16px;align-items:center;border-bottom:1px solid #eef2f6;padding:16px 0;flex-wrap:wrap}.session-row>div{flex:1;min-width:180px;overflow-wrap:anywhere}.onboarding-center footer{display:flex;gap:12px}@media(max-width:700px){.onboarding-center{padding:16px}.guide-cards{grid-template-columns:1fr}.onboarding-center header{align-items:flex-start}}
</style>
<style scoped>
.setup-results{display:grid;gap:16px;margin-top:16px}.setup-template{padding:16px 0;border-bottom:1px solid #e2e8f0;overflow-wrap:anywhere}
</style>
