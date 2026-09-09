<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { message, Modal } from 'ant-design-vue'
import { isAxiosError } from 'axios'
import { abandonOnboarding, applyOnboardingStep, checkOnboarding, createOnboardingSession, getOnboardingSession, getOnboardingStatus, inspectOnboarding, saveOnboardingDraft } from '../../api/onboarding'
import { listProjects } from '../../api/project'
import { listPipelines } from '../../api/pipeline'
import { listUserOptions } from '../../api/user'
import { listApprovalFlows } from '../../api/release'
import { useAuthStore } from '../../stores/auth'
import type { OnboardingDraft, OnboardingInspection, OnboardingParameter, OnboardingRow, OnboardingSession, OnboardingStatus } from '../../types/onboarding'
import { newOnboardingParameter, onboardingSourceLabel, onboardingSteps } from '../../utils/onboarding'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const route = useRoute(), router = useRouter(), auth = useAuthStore()
const session = ref<OnboardingSession>(), draft = ref<OnboardingDraft>(), status = ref<OnboardingStatus>()
const inspection = ref<OnboardingInspection>({rows: [], fields: [], issues: []})
const activeStep = ref(0), loading = ref(true), busy = ref(false), saving = ref(false), error = ref(''), savedJSON = ref('')
const projectOptions = ref<{label: string; value: string}[]>([]), pipelineOptions = ref<{label: string; value: string}[]>([])
const ownerOptions = ref<{label: string; value: string}[]>([]), flowOptions = ref<{label: string; value: string}[]>([])
const optionsError = ref(''), showAll = ref(false)
const conflicted = ref(false)
const headingRef = ref<HTMLElement>()
let timer: ReturnType<typeof setTimeout> | undefined
let pendingSave: Promise<void> | undefined
let applyingKey = '', applyingFingerprint = ''
const step = computed(() => onboardingSteps[activeStep.value]!)
const locked = computed(() => !!session.value?.refs.template_id || session.value?.status === 'abandoned')
const dirty = computed(() => !!draft.value && JSON.stringify(draft.value) !== savedJSON.value)
const fieldOptions = computed(() => inspection.value.fields.map(f => ({label: `${f.name} · ${f.key}`, value: f.key})))
const builtinOptions = computed(() => inspection.value.fields.filter(f => f.builtin).map(f => ({label: `${f.name} · ${f.key}`, value: f.key})))
const ciOptions = computed(() => [...new Set(draft.value?.params.filter(p => p.scope === 'ci' && !p.omit).map(p => p.param_key) || [])].map(key => ({label: key, value: key})))
const rows = computed(() => inspection.value.rows.map(row => ({row, param: draft.value?.params.find(p => p.scope === row.scope && p.name === row.name)})).filter((item): item is {row: OnboardingRow; param: OnboardingParameter} => !!item.param))
const visibleRows = computed(() => rows.value.filter(({row, param}) => showAll.value || row.problem || param.new_field_name || !row.mapped_key))
const sourceNeeds = computed(() => ({
  repo: draft.value?.params.some(p => !p.omit && p.value_source === 'builtin' && p.source_param_key === 'repo_url'),
  branch: draft.value?.params.some(p => !p.omit && p.value_source === 'builtin' && ['branch', 'git_ref'].includes(p.source_param_key)),
}))
const requiredPermissions = ['application.manage', 'pipeline.view', 'pipeline.manage', 'pipeline_param.manage', 'platform_param.manage', 'release.template.manage']
const missingPermissions = computed(() => requiredPermissions.filter(p => !auth.hasPermission(p)))
const preview = computed(() => draft.value?.params.filter(p => !p.omit) || [])
const changedFields = computed(() => draft.value?.params.filter(p => !p.omit && p.new_field_name) || [])

function accept(value: OnboardingSession, replaceDraft = false) {
  session.value = value
  if (replaceDraft) { draft.value = structuredClone(value.draft); savedJSON.value = JSON.stringify(value.draft) }
}
async function loadOptions() {
  optionsError.value = ''
  try {
    const [users, flows, current] = await Promise.all([listUserOptions(), auth.hasPermission('release.template.manage') ? listApprovalFlows() : Promise.resolve({data: []}), getOnboardingStatus()])
    ownerOptions.value = users.data.map(u => ({label: u.display_name || u.username, value: u.id}))
    flowOptions.value = flows.data.map(f => ({label: f.name, value: f.id}))
    status.value = current
    const projects: {label: string; value: string}[] = [], pipelines: {label: string; value: string}[] = []
    for (let page = 1; ; page++) {
      const result = await listProjects({status: 'active', page, page_size: 100})
      projects.push(...result.data.map(p => ({label: `${p.name} · ${p.key}`, value: p.id})))
      if (!result.data.length || projects.length >= result.total) break
    }
    for (let page = 1; ; page++) {
      const result = await listPipelines({provider: 'jenkins', status: 'active', page, page_size: 100})
      pipelines.push(...result.data.map(p => ({label: p.job_full_name, value: p.id})))
      if (!result.data.length || pipelines.length >= result.total) break
    }
    projectOptions.value = projects; pipelineOptions.value = pipelines
  } catch (e) { optionsError.value = extractHTTPErrorMessage(e, '选项读取失败，请重试') }
}
async function load() {
  loading.value = true; error.value = ''
  try {
    accept(await getOnboardingSession(String(route.params.sessionId)), true)
    conflicted.value = false
    activeStep.value = Math.max(0, onboardingSteps.findIndex(s => s.key === session.value!.current_step))
    await loadOptions()
    if (activeStep.value >= 3) await readParams(false)
  } catch (e) { error.value = extractHTTPErrorMessage(e, '接入任务加载失败') } finally { loading.value = false }
}
async function save() {
  clearTimeout(timer)
  if (pendingSave) await pendingSave
  if (conflicted.value) throw new Error('配置已变化，请先重新读取任务再编辑')
  if (!session.value || !draft.value || !dirty.value || locked.value) return
  const snapshot = JSON.parse(JSON.stringify(draft.value)) as OnboardingDraft
  saving.value = true
  pendingSave = (async () => {
    try {
      const result = await saveOnboardingDraft(session.value!, snapshot)
      accept(result); savedJSON.value = JSON.stringify(snapshot); error.value = ''
    } catch (e) {
      if (isAxiosError(e) && e.response?.status === 409) conflicted.value = true
      error.value = extractHTTPErrorMessage(e, '草稿保存失败，修改仍保留在本页'); throw e
    }
    finally { saving.value = false }
  })()
  try { await pendingSave } finally { pendingSave = undefined }
}
watch(draft, () => {
  clearTimeout(timer)
  if (!loading.value && !busy.value && !locked.value && dirty.value) timer = setTimeout(() => { void save().catch(() => undefined) }, 800)
}, {deep: true})
watch(activeStep, async () => {
  await nextTick()
  headingRef.value?.scrollIntoView({block: 'start', behavior: 'smooth'})
})
async function readParams(saveFirst = true) {
  if (!session.value || !draft.value) return
  if (saveFirst) await save()
  const data = await inspectOnboarding(session.value.id)
  inspection.value = data
  if (locked.value) return
  // Preserve decisions when refreshing metadata. Removed parameters remain in
  // the draft until the user explicitly discards them; backend detects drift.
  for (const row of data.rows) {
    const existing = draft.value.params.find(p => p.scope === row.scope && p.name === row.name)
    if (!existing) draft.value.params.push(newOnboardingParameter(row, data.fields))
  }
}
async function refreshParams() {
  busy.value = true
  try { await readParams(); await save() } catch (e) { error.value = extractHTTPErrorMessage(e, '管线参数读取失败') } finally { busy.value = false }
}
const removedParams = computed(() => draft.value?.params.filter(p => !inspection.value.rows.some(r => r.scope === p.scope && r.name === p.name)) || [])
function discardRemoved() {
  if (!draft.value) return
  draft.value.params = draft.value.params.filter(p => inspection.value.rows.some(r => r.scope === p.scope && r.name === p.name))
}
function selectField(param: OnboardingParameter, key: string) {
  param.param_key = key; param.new_field_name = ''
}
function createField(param: OnboardingParameter, row: OnboardingRow) {
  param.param_key = row.new_key; param.new_field_name = row.name
}
async function apply() {
  if (!session.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    await save()
    const fingerprint = `${step.value.key}:${JSON.stringify(draft.value)}`
    if (fingerprint !== applyingFingerprint) { applyingFingerprint = fingerprint; applyingKey = `${step.value.key}-${Date.now()}-${Math.random().toString(36).slice(2)}` }
    const result = await applyOnboardingStep(session.value, step.value.key, applyingKey)
    accept(result, true)
    activeStep.value = Math.max(0, onboardingSteps.findIndex(s => s.key === result.current_step))
    if (result.current_step === 'parameters') { await readParams(false); await save() }
  } catch (e) {
    error.value = extractHTTPErrorMessage(e, '当前步骤保存失败，可修复后重试')
    if (isAxiosError(e) && e.response?.status === 409) conflicted.value = true
    // Do not silently adopt a concurrent editor's version and then overwrite
    // it with this tab's stale draft. Explicit reload is required on conflict.
    if (conflicted.value) return
    // Keep local edits while recovering references/version from partially
    // completed work. A lease may still be active after a network timeout.
    try { accept(await getOnboardingSession(session.value!.id)) } catch { /* keep recoverable draft */ }
  } finally { busy.value = false }
}
async function check() {
  if (!session.value) return
  busy.value = true; error.value = ''
  try { await save(); accept(await checkOnboarding(session.value.id)); if (session.value.check.status === 'ready') activeStep.value = 6 }
  catch (e) { error.value = extractHTTPErrorMessage(e, '检查失败，请重试') } finally { busy.value = false }
}
async function nextApp() {
  if (!session.value) return
  busy.value = true
  try {
    const next = await createOnboardingSession({mode: 'create_application', project_id: session.value.refs.project_id})
    busy.value = false
    await router.push(`/onboarding/${next.id}`)
  } catch (e) { error.value = extractHTTPErrorMessage(e, '创建接入任务失败') } finally { busy.value = false }
}
function abandon() {
  Modal.confirm({title: '放弃这个接入任务？', content: '已创建的项目、应用、字段和模板都会保留，不会删除。', okText: '放弃任务', cancelText: '继续配置', async onOk() {
    if (!session.value) return
    await save(); accept(await abandonOnboarding(session.value), true); message.success('任务已放弃，已有资源已保留')
  }})
}
function firstRelease() {
  if (!session.value) return
  if (session.value.first_release_order_id) { void router.push(`/releases/${session.value.first_release_order_id}`); return }
  void router.push({path: '/releases/new', query: {application_id: session.value.refs.application_id, template_id: session.value.refs.template_id, onboarding_session_id: session.value.id}})
}
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value || busy.value) { e.preventDefault(); e.returnValue = '' } }
async function saveBeforeNavigation() {
  if (busy.value) { message.warning('正在保存配置，请稍候'); return false }
  try { await save() } catch { message.warning('草稿尚未保存，请重试或刷新后重新读取'); return false }
  return true
}
onBeforeRouteLeave(saveBeforeNavigation)
onBeforeRouteUpdate(saveBeforeNavigation)
watch(() => route.params.sessionId, () => { applyingFingerprint = ''; void load() })
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void load() })
onBeforeUnmount(() => { clearTimeout(timer); window.removeEventListener('beforeunload', beforeUnload) })
</script>

<template>
  <main class="onboarding-wizard">
    <header class="wizard-header"><div><a-button type="link" @click="router.push('/system/quick-start')">← 应用接入中心</a-button><h1>{{ draft?.identity.name || '接入一个应用' }}</h1><p>已有管线直接绑定 · 标准字段按需补齐 · 后续新增应用可再次使用</p></div><a-tag>{{ saving ? '正在保存草稿…' : dirty ? '有未保存修改' : '草稿已保存' }}</a-tag></header>
    <a-alert v-if="error" type="error" show-icon :message="error" description="本页修改仍保留；保存冲突时可重新读取任务。已完成的资源不会重复创建。"><template #action><a-button :disabled="busy || saving" @click="load">重新读取</a-button></template></a-alert>
    <a-spin :spinning="loading"><template v-if="session && draft">
      <nav class="step-nav" aria-label="接入步骤"><button v-for="(item, index) in onboardingSteps" :key="item.key" :class="{active: activeStep === index}" :aria-current="activeStep === index ? 'step' : undefined" :disabled="busy || index > Math.max(activeStep, onboardingSteps.findIndex(s => s.key === session!.current_step))" @click="activeStep = index"><span>{{ index + 1 }}</span>{{ item.title }}</button></nav>
      <section class="wizard-panel"><div ref="headingRef" class="step-heading"><h2>{{ step.title }}</h2><p>{{ step.help }}</p></div>
        <a-alert v-if="session.status === 'abandoned'" message="任务已放弃。原有资源仍保留，可在应用列表重新发起接入。" type="warning" show-icon />
        <a-alert v-else-if="locked && activeStep < 5" message="模板已经存在，本页只读。需要修改请使用高级配置，完成后回来重新检查。" type="info" show-icon />
        <a-button v-if="locked && session.status === 'blocked' && session.current_step === 'template_flow' && step.key === 'template_flow'" :loading="busy" @click="apply">重试保存应用流程</a-button>
        <a-alert v-if="optionsError" type="error" :message="optionsError"><template #action><a-button @click="loadOptions">重试选项</a-button></template></a-alert>
        <fieldset :disabled="busy || locked" class="step-fields">
          <template v-if="step.key === 'preflight'">
            <div class="check-card"><b>执行端连接配置</b><a-tag :color="status?.jenkins_enabled ? 'green' : 'red'">{{ status?.jenkins_enabled ? '已启用' : '未启用' }}</a-tag><p>真实连接和任务读取权限将在选择管线后验证；这里不会触发构建。</p></div>
            <div class="check-card"><b>可用管线</b><span>{{ status?.pipeline_count || 0 }} 条</span><p>需要先接入已有管线。若列表为空，到<a href="/components/jenkins" target="_blank" rel="noopener">管线列表</a>同步后回来刷新。</p></div>
            <div class="check-card"><b>发布环境</b><p>{{ status?.env_options.join(' / ') || '尚未设置' }}（首单创建时明确选择，不自动选生产）</p></div>
            <a-alert v-if="missingPermissions.length" type="warning" message="当前账号缺少部分配置权限" :description="`请管理员补齐：${missingPermissions.join('、')}。向导不会自动授予权限。`" show-icon />
            <a-button @click="loadOptions">重新检查基础配置</a-button>
          </template>
          <a-form v-else-if="step.key === 'identity'" layout="vertical" :disabled="busy || locked">
            <a-alert v-if="session.refs.application_id" type="info" message="项目和应用已保存，基本信息只读；仍可在这里补充代码仓库地址。" />
            <a-form-item label="所属项目（可选已有项目，或清空后新建）"><a-select v-model:value="draft.identity.project_id" :disabled="!!session.refs.application_id" allow-clear show-search option-filter-prop="label" :options="projectOptions" placeholder="选择已有项目；没有项目则在下方填写" @clear="draft.identity.project_id = ''" /></a-form-item>
            <div v-if="!draft.identity.project_id" class="form-grid"><a-form-item label="新项目名称" required><a-input v-model:value="draft.identity.project_name" :disabled="!!session.refs.application_id" :maxlength="100" placeholder="例如：韦二" /></a-form-item><a-form-item label="项目 Key" required><a-input v-model:value="draft.identity.project_key" :disabled="!!session.refs.application_id" :maxlength="100" placeholder="例如：weier" /></a-form-item></div>
            <div class="form-grid"><a-form-item label="应用名称" required><a-input v-model:value="draft.identity.name" :disabled="!!session.refs.application_id" :maxlength="100" placeholder="例如：韦二管理平台前端" /></a-form-item><a-form-item label="应用 Key" required><a-input v-model:value="draft.identity.key" :disabled="!!session.refs.application_id" :maxlength="100" placeholder="例如：weier-admin-web" /></a-form-item></div>
            <a-form-item label="应用代码仓库地址（可选）"><a-input v-model:value="draft.repo_url" :maxlength="1000" placeholder="例如：https://git.example.com/team/repository.git" /></a-form-item>
            <a-form-item label="应用负责人" required><a-select v-model:value="draft.identity.owner_user_id" :disabled="!!session.refs.application_id" show-search option-filter-prop="label" :options="ownerOptions" /></a-form-item>
            <a-form-item label="应用说明（可选）"><a-textarea v-model:value="draft.identity.description" :disabled="!!session.refs.application_id" :maxlength="1000" /></a-form-item>
            <p class="muted">语言、制品类型等分类信息可以之后补充，不影响绑定已有管线。</p>
          </a-form>
          <a-form v-else-if="step.key === 'pipelines'" layout="vertical" :disabled="busy || locked">
            <a-alert type="info" message="可以只构建、构建后部署，或独立部署。CI_JOB / CI_BUILD 等依赖 CI 的部署参数不能用于无 CI 的独立部署。" show-icon />
            <a-form-item label="CI 构建管线（可选）"><a-select v-model:value="draft.ci_pipeline_id" :disabled="!!session.refs.ci_binding_id" allow-clear show-search option-filter-prop="label" :options="pipelineOptions" placeholder="选择已有构建任务" @clear="draft.ci_pipeline_id = ''" /></a-form-item>
            <a-form-item label="CD 部署管线（可选）"><a-select v-model:value="draft.cd_pipeline_id" :disabled="!!session.refs.cd_binding_id" allow-clear show-search option-filter-prop="label" :options="pipelineOptions" placeholder="选择已有部署任务" @clear="draft.cd_pipeline_id = ''" /></a-form-item>
            <p class="muted">已保存的绑定不会被替换。ArgoCD / GitOps 或绑定变更，请使用下方高级配置。</p>
          </a-form>
          <template v-else-if="step.key === 'parameters'">
            <div class="parameter-toolbar"><a-button @click="refreshParams">重新读取管线参数</a-button><a-checkbox v-model:checked="showAll">展示全部参数（{{ rows.length }}）</a-checkbox></div>
            <a-alert v-if="!showAll && visibleRows.length < rows.length" type="info" show-icon :message="`已折叠 ${rows.length - visibleRows.length} 项有效共享映射。展开可调整当前应用的取值来源。`" description="只复用字段映射，不会复制其他应用的固定值；下方摘要列出发布时需要填写的参数。" />
            <a-alert v-for="issue in inspection.issues" :key="issue.code + issue.field_path" type="error" :message="issue.message" show-icon />
            <a-alert v-if="removedParams.length" type="warning" message="部分草稿参数已不在当前管线中" :description="removedParams.map(p => p.name).join('、')"><template #action><a-button @click="discardRemoved">确认移除这些草稿项</a-button></template></a-alert>
            <a-empty v-if="!rows.length && !inspection.issues.length" description="当前管线没有参数，可继续生成模板" />
            <article v-for="{row, param} in visibleRows" :key="row.scope + row.name" class="parameter-card" :class="{invalid: row.problem}">
              <div class="parameter-title"><a-tag>{{ row.scope.toUpperCase() }}</a-tag><h3>{{ row.name }}</h3><span>{{ row.type }} · {{ row.required || row.runtime ? '必需' : '可选' }}</span><a-tag v-if="row.mapped_key" color="green">复用已有映射</a-tag></div>
              <p v-if="row.description" class="muted">{{ row.description }}</p>
              <a-alert v-if="row.problem" type="error" :message="row.problem" show-icon />
              <template v-else>
                <p v-if="row.default_value" class="muted">管线默认：{{ row.default_value }}</p>
                <a-checkbox v-if="!row.required && !row.runtime" v-model:checked="param.omit">使用管线默认值，不传递此参数</a-checkbox>
                <a-form v-if="!param.omit" layout="vertical" :disabled="busy || locked" class="param-form">
                  <a-form-item label="标准字段（供模板和发布表单复用）">
                    <template v-if="!param.new_field_name"><a-select :value="param.param_key" :disabled="!!row.mapped_key" show-search option-filter-prop="label" :options="fieldOptions" @change="(key: string) => selectField(param, key)" /><a-button v-if="!row.mapped_key && !row.runtime" type="link" @click="createField(param, row)">缺少字段？就在这里新建</a-button></template>
                    <template v-else><div class="form-grid"><a-input v-model:value="param.new_field_name" placeholder="字段显示名称" aria-label="新标准字段名称" /><a-input v-model:value="param.param_key" placeholder="标准字段 Key" aria-label="新标准字段 Key" /></div><p class="muted">新字段类型：{{ row.type }}。保存本步时创建，不覆盖已有同名 Key。</p><a-button type="link" @click="param.new_field_name = ''; param.param_key = ''">改为选择已有字段</a-button></template>
                  </a-form-item>
                  <div class="form-grid"><a-form-item label="参数值从哪里来"><a-select v-model:value="param.value_source" :disabled="row.runtime"><a-select-option value="release_input">发布时填写</a-select-option><a-select-option value="fixed">此应用固定值</a-select-option><a-select-option value="builtin">发布基础字段 / 运行时结果</a-select-option><a-select-option v-if="row.scope === 'cd' && draft.ci_pipeline_id" value="ci_param">沿用 CI 参数</a-select-option></a-select></a-form-item>
                  <a-form-item v-if="param.value_source === 'builtin'" label="来源字段"><a-select v-model:value="param.source_param_key" :disabled="row.runtime" show-search option-filter-prop="label" :options="builtinOptions" /></a-form-item>
                  <a-form-item v-else-if="param.value_source === 'ci_param'" label="CI 来源字段"><a-select v-model:value="param.source_param_key" :options="ciOptions" /></a-form-item>
                  <a-form-item v-else-if="param.value_source === 'fixed'" label="固定值"><a-select v-if="row.type === 'choice'" v-model:value="param.fixed_value" :options="row.choices.map(value => ({label: value, value}))" /><a-select v-else-if="row.type === 'bool'" v-model:value="param.fixed_value" :options="[{label: 'true', value: 'true'}, {label: 'false', value: 'false'}]" /><a-input v-else v-model:value="param.fixed_value" placeholder="明确填写；不自动采用执行端默认值" :maxlength="4000" /></a-form-item></div>
                  <p v-if="row.runtime" class="muted">自动使用本次 CI 的任务名 / 构建号，不允许填入历史构建号。</p>
                </a-form>
              </template>
            </article>
            <a-alert v-if="changedFields.length" type="info" message="即将新增公共标准字段" :description="[...new Set(changedFields.map(p => `${p.new_field_name} (${p.param_key})`))].join('、')" show-icon />
            <a-form v-if="sourceNeeds.repo || sourceNeeds.branch" layout="vertical" :disabled="busy || locked" class="check-card"><a-checkbox v-model:checked="draft.save_app_sources">将以下信息同步到应用配置</a-checkbox><p class="muted">仓库地址供管线自动取值；发布分支只用于提供下拉候选，可留空并在创建发布单时手工填写。已有信息不变时无需勾选。</p><a-form-item v-if="sourceNeeds.repo" label="应用仓库地址"><a-input v-model:value="draft.repo_url" placeholder="管线明确需要的仓库来源" /></a-form-item><a-form-item v-if="sourceNeeds.branch" label="发布分支候选（可选）"><a-input v-model:value="draft.branch" placeholder="例如 main；留空时发布单可手工填写" /></a-form-item></a-form>
          </template>
          <a-form v-else-if="step.key === 'template_flow'" layout="vertical" :disabled="busy || locked">
            <a-form-item label="模板名称（留空自动使用应用名）"><a-input v-model:value="draft.template_name" :placeholder="`${draft.identity.name}-默认发布`" :maxlength="100" /></a-form-item>
            <div class="check-card"><b>当前发布方式</b><p>{{ draft.ci_pipeline_id ? '包含构建阶段' : '不含构建阶段' }} · {{ draft.cd_pipeline_id ? '包含部署阶段' : '不含部署阶段' }}。创建发布单不会执行；构建、部署及审批沿用现有平台规则。</p></div>
            <a-checkbox v-model:checked="draft.change_approval_flow">明确设置此应用的审批流程</a-checkbox>
            <a-form-item v-if="draft.change_approval_flow" label="应用审批流程"><a-select v-model:value="draft.approval_flow_id" allow-clear :options="flowOptions" placeholder="不选择表示无审批流程" @clear="draft.approval_flow_id = ''" /></a-form-item>
            <p v-else class="muted">保留应用现有审批设置；新应用默认无流程。审批不属于模板。</p>
          </a-form>
        </fieldset>
        <template v-if="['parameters', 'template_flow', 'review', 'first_release'].includes(step.key)">
          <details class="preview" :open="activeStep >= 4"><summary>发布表单预览与配置摘要（{{ preview.length }} 项）</summary><div v-for="param in preview" :key="param.scope + param.name" class="preview-row"><span>{{ param.scope.toUpperCase() }} · {{ param.name }} <small>{{ param.param_key }}</small></span><span>{{ onboardingSourceLabel(param, inspection.fields) }}</span></div><p v-if="!preview.length" class="muted">当前模板无额外管线参数。</p></details>
        </template>
        <div v-if="['review', 'first_release'].includes(step.key)" class="completion">
          <a-alert :type="session.check.status === 'ready' ? 'success' : 'warning'" :message="session.status === 'verified' ? '首单实际执行成功' : session.check.status === 'ready' ? '配置完成，可以创建发布单' : '尚未通过配置检查'" :description="session.status === 'verified' ? '可继续新增应用，共用已有项目和标准字段。' : '配置检查不会执行管线，也不代表发布成功。'" show-icon />
          <a-button :loading="busy" :disabled="session.status === 'abandoned'" @click="check">重新检查配置 / 首单状态</a-button>
          <template v-if="step.key === 'first_release' && session.check.status === 'ready'"><a-button type="primary" :disabled="busy || !auth.hasApplicationPermission('release.create', session.refs.application_id)" @click="firstRelease">{{ session.first_release_order_id ? '查看首个发布单' : '创建首个发布单（不执行）' }}</a-button><a-button :loading="busy" @click="nextApp">继续接入下一个应用</a-button></template>
        </div>
        <div v-if="session.check.issues?.length" class="issues"><a-alert v-for="(issue, index) in session.check.issues" :key="index" type="error" :message="issue.message" :description="issue.remedy" show-icon><template #action><a-button @click="activeStep = Math.max(0, onboardingSteps.findIndex(s => s.key === issue.step))">返回修复</a-button></template></a-alert></div>
      </section>
      <footer class="wizard-footer"><div><a-button :disabled="busy || activeStep === 0" @click="activeStep--">上一步</a-button><a-button :loading="saving" :disabled="busy || locked" @click="save().catch(() => undefined)">保存草稿</a-button><a-button v-if="session.status !== 'abandoned'" type="text" :disabled="busy || saving" @click="abandon">放弃任务</a-button></div><div><a-button :disabled="busy" @click="router.push('/system/quick-start')">稍后继续</a-button><a-button v-if="activeStep < 6 && !locked" type="primary" :loading="busy" :disabled="saving || !!optionsError" @click="apply">保存并继续</a-button><a-button v-else-if="activeStep < 5" type="primary" :disabled="busy" @click="activeStep++">查看下一步</a-button></div></footer>
      <aside class="advanced-links"><span>高级配置：</span><router-link v-if="session.refs.application_id" :to="`/applications/${session.refs.application_id}/edit`">应用信息</router-link><router-link v-if="session.refs.application_id" :to="`/applications/${session.refs.application_id}/pipeline-bindings`">管线绑定</router-link><router-link :to="{path: '/release-templates', query: {application_id: session.refs.application_id}}">发布模板</router-link><router-link to="/platform-param-dicts">标准字库</router-link><router-link to="/components/executor-params">参数映射</router-link></aside>
    </template></a-spin>
  </main>
</template>

<style scoped>
.onboarding-wizard{max-width:1320px;margin:0 auto;padding:24px;min-width:0}.wizard-header{display:flex;align-items:center;justify-content:space-between;gap:20px;flex-wrap:wrap;margin-bottom:24px}.wizard-header h1{font-size:28px;margin:8px 0;overflow-wrap:anywhere}.wizard-header p,.step-heading p,.muted{color:#64748b;line-height:1.75}.wizard-header .ant-btn{padding-left:0}.step-nav{display:flex;flex-wrap:wrap;gap:8px;margin:0 0 20px}.step-nav button{display:flex;align-items:center;gap:7px;background:#fff;border:1px solid #dbe3ef;border-radius:9px;padding:10px 12px;color:#475569;cursor:pointer}.step-nav button span{background:#eef2f8;border-radius:50%;width:22px;height:22px;display:grid;place-items:center}.step-nav button.active{color:#1d4ed8;border-color:#3b82f6;background:#eff6ff}.step-nav button:disabled{opacity:.5;cursor:default}.wizard-panel{background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:28px;min-width:0}.step-heading{margin-bottom:24px}.step-heading h2{margin:0;font-size:22px}.step-fields{border:0;margin:0;padding:0;min-width:0}.step-fields:disabled{opacity:.85}.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.check-card{border:1px solid #e2e8f0;background:#f8fafc;border-radius:10px;padding:18px;margin:16px 0}.check-card>b{margin-right:16px}.parameter-toolbar{display:flex;align-items:center;gap:16px;flex-wrap:wrap;margin:0 0 20px}.parameter-card{border:1px solid #dce4ef;border-radius:12px;padding:20px;margin:16px 0;overflow-wrap:anywhere}.parameter-card.invalid{border-color:#fca5a5}.parameter-title{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.parameter-title h3{font-size:16px;margin:0}.parameter-title>span{font-size:12px;color:#64748b}.param-form{margin-top:16px}.param-form .ant-select{width:100%}.preview{margin:24px 0;background:#f8fafc;border-radius:10px;padding:18px}.preview summary{cursor:pointer;font-weight:600}.preview-row{display:flex;gap:16px;justify-content:space-between;padding:12px 0;border-bottom:1px solid #e2e8f0;overflow-wrap:anywhere}.preview-row>span{min-width:0}.preview-row small{display:block;color:#64748b}.completion,.issues{display:flex;gap:14px;flex-wrap:wrap;margin:20px 0}.completion>.ant-alert,.issues>.ant-alert{width:100%}.wizard-footer{display:flex;justify-content:space-between;gap:20px;flex-wrap:wrap;margin:20px 0}.wizard-footer>div{display:flex;gap:10px;flex-wrap:wrap}.advanced-links{display:flex;gap:16px;flex-wrap:wrap;color:#64748b;font-size:13px}.ant-alert{margin-bottom:14px}.ant-form-item{margin-top:14px}button:focus-visible,summary:focus-visible{outline:2px solid #2563eb;outline-offset:3px}@media(max-width:760px){.onboarding-wizard{padding:12px}.wizard-panel{padding:18px}.form-grid{grid-template-columns:1fr;gap:0}.preview-row{flex-direction:column;gap:6px}.step-nav button{font-size:12px;padding:8px}.wizard-footer{flex-direction:column}.wizard-footer>div{width:100%}}
</style>
