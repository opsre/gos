import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseAutomationView.vue', import.meta.url)
const apiURL = new URL('../src/api/release-automation.ts', import.meta.url)
const typesURL = new URL('../src/types/release-automation.ts', import.meta.url)
const routerURL = new URL('../src/router/index.ts', import.meta.url)
const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const listURL = new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url)
const releaseApiURL = new URL('../src/api/release.ts', import.meta.url)
const paramFieldsURL = new URL('../src/components/release/ReleaseTemplateParamFields.vue', import.meta.url)
const releaseTypesURL = new URL('../src/types/release.ts', import.meta.url)

function read(url) {
  return readFileSync(url, 'utf8')
}

test('automation page is routed from the release management menu', () => {
  const router = read(routerURL)
  const layout = read(layoutURL)

  assert.match(
    router,
    /const ReleaseAutomationView = \(\) => import\('\.\.\/views\/release\/ReleaseAutomationView\.vue'\)/,
    'router should lazy-load the automation page',
  )
  assert.match(
    router,
    /path: '\/release-automations'[\s\S]*name: 'release-automation-list'[\s\S]*component: ReleaseAutomationView[\s\S]*title: '自动化'[\s\S]*permission: \['release\.automation\.view', 'release\.automation\.manage'\]/,
    'router should expose /release-automations with the frozen permission codes',
  )
  assert.match(
    router,
    /path: '\/release-schedules'[\s\S]*path: '\/release-automations'/,
    'automation route should sit next to the scheduled release route',
  )
  assert.match(
    layout,
    /route\.path\.startsWith\('\/release-automations'\)[\s\S]*return \['release-automations'\]/,
    'layout should mark the automation menu item active',
  )
  assert.match(
    layout,
    /route\.path\.startsWith\('\/release-automations'\)[\s\S]*return \['release-management'\]/,
    'automation route should keep the release management submenu open',
  )
  assert.match(
    layout,
    /function goToReleaseAutomations\(\) \{[\s\S]*router\.push\('\/release-automations'\)/,
    'layout should navigate to the automation page from the menu',
  )
  assert.match(
    layout,
    /release\.automation\.view[\s\S]*release\.automation\.manage/,
    'menu visibility should accept either automation permission code',
  )
  assert.match(
    layout,
    /key="release-order-schedules" @click="goToReleaseSchedules">预约发布<\/a-menu-item>[\s\S]*key="release-automations"[\s\S]*自动化/,
    'release management menu should place the automation entry right after the scheduled release entry',
  )
})

test('automation api follows the frozen backend contract', () => {
  const api = read(apiURL)

  assert.match(api, /http\.get<ReleaseAutomationListResponse>\(\s*"\/release-automations"/, 'list should call GET /release-automations')
  assert.match(
    api,
    /http\.get<ReleaseAutomationDataResponse>\(\s*`\/release-automations\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}`/,
    'detail should call GET /release-automations/:id',
  )
  assert.match(api, /http\.post<ReleaseAutomationDataResponse>\(\s*"\/release-automations"/, 'create should call POST /release-automations')
  assert.match(
    api,
    /http\.put<ReleaseAutomationDataResponse>\(\s*`\/release-automations\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}`/,
    'update should call PUT /release-automations/:id',
  )
  assert.match(
    api,
    /http\.delete\(\s*`\/release-automations\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}`/,
    'delete should call DELETE /release-automations/:id',
  )
  assert.match(
    api,
    /http\.post<ReleaseAutomationCheckResponse>\(\s*`\/release-automations\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}\/check`/,
    'check should call POST /release-automations/:id/check',
  )

  const types = read(typesURL)
  for (const field of [
    'application_name',
    'template_name',
    'dispatch_mode',
    'last_seen_sha',
    'last_triggered_sha',
    'last_checked_at',
    'last_error',
  ]) {
    assert.match(types, new RegExp(`\\b${field}:`), `ReleaseAutomation should expose ${field}`)
  }
  assert.match(types, /"build" \| "build_deploy" \| "execute"/, 'dispatch_mode should match the frozen enum')
  assert.match(types, /check_git\?: boolean/, 'payload should expose the optional check_git flag')
})

test('automation page follows the release list layout chrome with square tables', () => {
  const source = read(viewURL)

  assert.match(source, /<div class="page-header-card page-header automation-page-header">/, 'page should use the same transparent header shell as the release list')
  assert.match(
    source,
    /<div class="page-header-actions automation-header-actions">[\s\S]*release-toolbar-action-btn[\s\S]*新建自动化/,
    'header actions should match the transparent glass action buttons',
  )
  assert.match(source, /class="release-order-table automation-table"/, 'page should reuse the release table style with a page-scoped table class')
  assert.match(
    source,
    /:deep\(\.automation-table \.ant-table-container\)[\s\S]*border-radius: 0 !important/,
    'automation table shells must reset to square corners',
  )
  assert.doesNotMatch(
    source,
    /automation-table[\s\S]{0,400}border-radius:\s*(18|20)px/,
    'page-scoped table styles must not reintroduce 18px/20px radius',
  )
  assert.match(source, /:deep\(\.automation-table \.ant-table-cell-fix-right\)[\s\S]*background: #ffffff !important/, 'fixed action column must stay opaque')
  assert.match(source, /:closable="false"[\s\S]*:footer="null"[\s\S]*automation-form-modal-wrap/, 'create/edit form should use a custom modal shell without the default ant footer')
  assert.match(source, /class="automation-form-modal-titlebar"[\s\S]*保存/, 'modal title bar should carry the right-aligned save action')
})

test('automation form renders the parameters the release template maps to release input', () => {
  const source = read(viewURL)
  const fields = read(paramFieldsURL)

  assert.match(
    source,
    /<ReleaseTemplateParamFields[\s\S]*:application-id="form\.application_id"[\s\S]*:template-id="form\.template_id"[\s\S]*:scopes="activeParamScopes"[\s\S]*:initial-params="editingParamSeed"/,
    'the automation form should delegate its parameter area to the template-driven fields',
  )
  assert.doesNotMatch(
    source,
    /formParamRows|addParamRow|buildParamDraftRow|validateParamRows/,
    'the hand-written parameter override rows must be gone',
  )
  assert.match(
    source,
    /const activeParamScopes = computed<ReleasePipelineScope\[\]>\(\(\) => \{[\s\S]*case 'build':\s*return \['ci'\][\s\S]*case 'execute':\s*return \['cd'\][\s\S]*return \['ci', 'cd'\]/,
    'the visible parameter scope must follow the dispatch mode',
  )
  assert.match(
    source,
    /const params = paramFieldsRef\.value\?\.buildParams\?\.\(\) \|\| formParamPayload\.value[\s\S]*payload\.params = params/,
    'saving should submit the parameters built from the template mapping',
  )
  assert.match(
    fields,
    /function isReleaseInputParam\(param: ReleaseTemplateParam\)[\s\S]*const source = String\(param\.value_source \|\| ''\)\.trim\(\)\.toLowerCase\(\)[\s\S]*source === '' \|\| source === 'release_input'/,
    'only parameters the template opens for release input should be rendered',
  )
  assert.match(
    fields,
    /\.filter\(\(param\) => isReleaseInputParam\(param\)\)[\s\S]*\.filter\(\(param\) => activeScopes\.value\.has/,
    'fields should be limited to the active pipeline scopes',
  )
  assert.match(
    fields,
    /getReleaseTemplateByID\(templateID\)[\s\S]*response\.data\.params/,
    'fields should be derived from the selected release template detail',
  )
  assert.match(
    fields,
    /listApplicationExecutorParamDefs\(props\.applicationId, \{[\s\S]*binding_type: scope[\s\S]*binding_id: bindingID/,
    'choice options should come from the executor param definitions of the template binding',
  )
  assert.match(
    fields,
    /const missing = visibleFields\.value\.find\(\(field\) => field\.required && !fieldValue\(field\)\.trim\(\)\)[\s\S]*为必填，请填写后再保存/,
    'a required release-input parameter must block saving when left empty',
  )
  assert.match(
    fields,
    /v-else-if="!templateId"[\s\S]*选择应用与发布模板后，这里会列出模板要求填写的参数/,
    'an unselected template should ask for one instead of claiming there is nothing to fill',
  )
  assert.match(
    fields,
    /const fixedGitRef = computed\(\(\) => \{[\s\S]*scope === 'ci' && key === 'git_ref' && source === 'fixed'[\s\S]*emit\('update:fixedGitRef', value\)/,
    'a template that pins the CI branch must publish that value to the form',
  )
  assert.match(
    source,
    /function handleTemplateFixedGitRef\(value: string\) \{[\s\S]*templateFixedGitRef\.value = value\s*if \(value\) \{\s*form\.git_ref = value/,
    'the automation branch should default to the branch the template builds',
  )
  assert.doesNotMatch(
    source,
    /:disabled="Boolean\(templateFixedGitRef\)"/,
    'the branch must stay editable so another branch can still be watched',
  )
  assert.match(
    source,
    /const gitRefFollowsTemplate = computed\([\s\S]*form\.git_ref\.trim\(\) === templateFixedGitRef\.value/,
    'the form should know whether the branch still follows the template',
  )
  assert.match(
    source,
    /gitRefFollowsTemplate[\s\S]*已按发布模板的固定分支 \{\{ templateFixedGitRef \}\} 填入[\s\S]*v-else-if="templateFixedGitRef"[\s\S]*模板固定构建 \{\{ templateFixedGitRef \}\} 分支/,
    'a mismatch between the watched branch and the template branch should be called out',
  )
  assert.match(
    source,
    /if \(previous && form\.git_ref\.trim\(\) === previous\) \{\s*form\.git_ref = ''/,
    'switching to a template without a fixed branch must not keep the previous branch',
  )
  assert.match(
    source,
    /@update:params="handleParamPayloadChange"[\s\S]*@update:fixed-git-ref="handleTemplateFixedGitRef"/,
    'the form should listen to the template-driven branch value',
  )
  assert.match(
    fields,
    /value_source: 'release_input'/,
    'submitted parameters should be marked as release input',
  )
})

test('automation page gates saving on the git check result', () => {
  const source = read(viewURL)

  assert.match(
    source,
    /async function ensureGitReachableBeforeSave\(\)[\s\S]*checkReleaseAutomation\(editingAutomationID\.value\)[\s\S]*if \(!result\.reachable\)[\s\S]*showGitCheckFailure/,
    'edit mode should call the check endpoint and block saving when the branch is unreachable',
  )
  assert.match(
    source,
    /function showGitCheckFailure\(rawMessage: string\)[\s\S]*const text = String\(rawMessage \|\| ''\)\.trim\(\)[\s\S]*gitCheckError\.value = text[\s\S]*message\.error\(text\)/,
    'the backend message must be shown verbatim instead of being replaced by a generic text',
  )
  assert.match(
    source,
    /const text = extractHTTPErrorMessage\(error,[\s\S]*isGitRelatedMessage\(text\)[\s\S]*showGitCheckFailure\(text\)/,
    'create failures reported by check_git should surface the git reason and stop the save',
  )
  assert.match(source, /check_git: true/, 'save payload should keep server-side git verification enabled')
  assert.match(
    source,
    /class="automation-form-alert"[\s\S]*git 校验未通过，已阻止保存/,
    'the form should show an inline blocking alert for git check failures',
  )
  assert.match(source, /checkResult\.reachable[\s\S]*checkResult\.head_sha[\s\S]*checkResult\.message/, 'check result panel should render reachable/head_sha/message')
})

test('release order list adds the automatic/manual switch and the automation tag', () => {
  const source = read(listURL)
  const releaseTypes = read(releaseTypesURL)

  assert.match(releaseTypes, /export type ReleaseTriggerType =[\s\S]*"automation"/, 'ReleaseTriggerType should include automation')
  assert.match(
    source,
    /const DEFAULT_TRIGGER_TYPE: ReleaseTriggerType = "manual"/,
    'the switch should default to the manual list',
  )
  assert.match(
    source,
    /<div class="page-header-copy page-header-title-group">[\s\S]*<h2[\s\S]*class="page-title"[\s\S]*\{\{ pageTitleText \}\}[\s\S]*<\/h2>[\s\S]*class="release-trigger-switch"[\s\S]*class="release-trigger-switch-button"[\s\S]*<SwapOutlined/,
    'the ⇄ switch button (same icon as the detail page) should sit next to the page title',
  )
  assert.doesNotMatch(
    source,
    /release-trigger-switch-label/,
    'the switch itself must not render a 手动/自动 text label',
  )
  assert.match(
    source,
    /const pageTitleText = computed\(\(\) => \(triggerToggleIsAutomation\.value \? "自动" : "发布"\)\)/,
    'the page title should read 自动 instead of 发布 in the automation view',
  )
  assert.match(
    source,
    /\.page-title--automation \{\s*color: #1d4ed8;\s*\}/,
    'the automation title should only turn blue',
  )
  assert.doesNotMatch(
    source,
    /page-title-automation-badge|page-title--automation::before/,
    'the title should not get a lightning badge or a backlight glow',
  )
  assert.match(
    source,
    /function handleTriggerTypeToggleChange\(\)[\s\S]*isTriggerToggleValue\(current\) && current === "manual" \? "automation" : "manual"[\s\S]*filters\.trigger_type = next[\s\S]*handleSearch\(\)/,
    'clicking the switch should flip between 手动 and 自动 and reuse the filter apply logic',
  )
  assert.match(
    source,
    /function triggerToggleIconClass\(\) \{[\s\S]*"release-trigger-switch-icon--auto": filters\.trigger_type === "automation"[\s\S]*"release-trigger-switch-icon--spin-to-auto"[\s\S]*triggerToggleDirection\.value === "to-auto"/,
    'the swap icon should flip and animate with the current mode',
  )
  assert.match(
    source,
    /function markTriggerToggleSwitch\(\)[\s\S]*sessionStorage\.getItem\(TRIGGER_TOGGLE_LAST_MODE_KEY\)[\s\S]*previous !== filters\.trigger_type/,
    'the switch direction must survive the fullPath-keyed page remount',
  )
  assert.match(
    source,
    /onMounted\(async \(\) => \{\s*applyRouteQuery\(\);\s*markTriggerToggleSwitch\(\);/,
    'the switch animation state should be derived right after the route query is applied',
  )
  assert.match(
    source,
    /@keyframes release-trigger-switch-spin-to-auto \{[\s\S]*rotate\(0deg\)[\s\S]*rotate\(180deg\)/,
    'the icon should spin between the two modes',
  )
  assert.match(
    source,
    /@media \(prefers-reduced-motion: reduce\)/,
    'the switch animation should respect reduced motion',
  )
  assert.match(
    source,
    /const triggerToggleIsAutomation = computed\(\(\) => filters\.trigger_type === "automation"\)/,
    'the title switch should follow the automation filter state',
  )
  assert.match(
    source,
    /const triggerToggleHint = computed\(\(\) => `当前只看\$\{triggerToggleLabel\.value\}触发的发布单，点击切换`\)/,
    'the button should describe the current mode for hover/assistive tech',
  )
  assert.match(
    source,
    /function syncReleaseListQueryToRoute\(\)[\s\S]*router\.replace\(\{ path: "\/releases", query: nextQuery \}\)/,
    'filter changes should stay in the URL so a refresh keeps the same trigger_type',
  )
  assert.match(
    source,
    /\{ label: "自动化", value: "automation" \}/,
    'advanced trigger type select should offer the automation option',
  )
  assert.match(source, /case "automation":\s*return "自动化";/, 'triggerTypeText should map automation to 自动化')
  assert.match(
    source,
    /<a-tag\s+v-if="record\.trigger_type === 'automation'"\s+class="release-trigger-automation-tag"\s*>[\s\S]*自动/,
    'automation triggered orders should carry a 自动 tag in the order number cell',
  )
})

test('dispatch precheck survives a slow Jenkins parameter read', () => {
  const releaseApi = read(releaseApiURL)
  const source = read(listURL)

  assert.match(
    releaseApi,
    /export async function getReleaseOrderPrecheck\([\s\S]*?\/release-orders\/\$\{id\}\/precheck[\s\S]*?timeout: 120_000,[\s\S]*?\);/,
    'the precheck call must not be killed by the global 10s timeout while Jenkins parameters load',
  )
  assert.match(
    source,
    /v-if="executePreviewLoading" class="execute-preview-loading"[\s\S]*class="execute-preview-loading-hint"[\s\S]*正在读取 Jenkins 真实参数/,
    'the preview modal should explain the slow first precheck instead of showing bare skeletons',
  )
})
