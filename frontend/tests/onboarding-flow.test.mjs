import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
const read = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const wizard = read('views/onboarding/ApplicationOnboardingView.vue')
const center = read('views/onboarding/OnboardingCenterView.vue')
const release = read('views/release/ReleaseOrderCreateView.vue')

test('onboarding is a permanent route and supports a new application after completion', () => {
  const router = read('router/index.ts')
  assert.ok(router.includes("path: '/onboarding/:sessionId'"))
  assert.ok(router.includes('OnboardingCenterView.vue'))
  assert.ok(center.includes("mode: applicationID ? 'complete_application' : 'create_application'"))
  assert.ok(center.includes('getApplicationSetupStatus'))
  assert.ok(center.includes('一个模板异常不影响其他有效模板'))
  assert.match(wizard, /createOnboardingSession\(\{mode: 'create_application', project_id: session\.value\.refs\.project_id\}\)/)
  assert.ok(wizard.includes('继续接入下一个应用'))
})
test('draft saving is serialized and never disables parameter editing while a save is pending', () => {
  assert.ok(wizard.includes('if (pendingSave) await pendingSave'))
  assert.ok(wizard.includes('}, 800)'))
  assert.ok(wizard.includes('onBeforeRouteLeave'))
  assert.ok(wizard.includes('onBeforeRouteUpdate'))
  assert.ok(wizard.includes('if (conflicted.value) return'))
  assert.ok(!wizard.includes('busy || saving || locked'))
})
test('runtime parameters and inline field creation remain explicit', () => {
  assert.ok(wizard.includes('即将新增公共标准字段'))
  assert.ok(wizard.includes('缺少字段？就在这里新建'))
  assert.ok(wizard.includes(':disabled="row.runtime"'))
  assert.ok(wizard.includes('管线参数已变化') || wizard.includes('部分草稿参数已不在当前管线中'))
  assert.ok(wizard.includes('sourceNeeds.repo || sourceNeeds.branch'))
})
test('application repository is collected up front and release branches remain optional', () => {
  const identityStep = wizard.slice(wizard.indexOf(`step.key === 'identity'`), wizard.indexOf(`step.key === 'pipelines'`))
  assert.match(identityStep, /应用代码仓库地址（可选）[\s\S]*draft\.repo_url/)
  assert.match(identityStep, /基本信息只读；仍可在这里补充代码仓库地址/)
  assert.doesNotMatch(identityStep, /<a-form[^>]*:disabled="!!session\.refs\.application_id/)
  assert.match(wizard, /发布分支候选（可选）/)
  assert.ok(wizard.includes('留空时发布单可手工填写'))
})
test('onboarding first-order branch returns before any normal build or fast-execute action', () => {
  const branch = release.slice(release.indexOf('if (onboardingSessionID.value) {'), release.indexOf('const response = isEditMode.value'))
  assert.ok(branch.includes('createOnboardingFirstRelease'))
  assert.ok(branch.includes('return'))
  assert.ok(!branch.includes('buildReleaseOrder') && !branch.includes('fast_execute'))
  assert.ok(release.includes(':disabled="!!onboardingSessionID"'))
  assert.ok(release.includes('!isBatchMode && !onboardingSessionID'))
})
