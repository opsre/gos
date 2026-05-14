import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderCreateView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

test('release order create page uses standardized header actions instead of bottom actions', () => {
  assert.match(
    source,
    /<div class="page-header create-page-header">/,
    'create page should use the standardized transparent create header',
  )
  assert.match(
    source,
    /<div class="page-header-actions">[\s\S]*class="application-toolbar-action-btn"[\s\S]*\{\{ primaryActionText \}\}[\s\S]*release-fast-toolbar-btn[\s\S]*返回发布单/,
    'create actions should live in the page header and use application toolbar button styles',
  )
  assert.doesNotMatch(
    source,
    /<div class="action-area">/,
    'create page should not keep the old bottom action area',
  )
  assert.doesNotMatch(
    source,
    /page-header-card page-header/,
    'create page should not use the old page header card shell',
  )

  const buttonRule = extractStyleRule(':deep(.application-toolbar-action-btn.ant-btn)')
  assert.match(buttonRule, /height:\s*42px/, 'toolbar buttons should keep the shared 42px height')
  assert.match(buttonRule, /border-radius:\s*16px/, 'toolbar buttons should keep shared rounded corners')
  assert.match(buttonRule, /color:\s*#0f172a !important/, 'toolbar button text should use the shared dark color')
})

test('release order create page provides ci-only build submit action', () => {
  assert.match(
    source,
    /createReleaseOrder,\s*buildReleaseOrder/,
    'create page should import the build API next to createReleaseOrder',
  )
  assert.match(
    source,
    /const hasStagedBuildBindings = computed\(\(\) => Boolean\(bindingMapByScope\.value\.ci && bindingMapByScope\.value\.cd\)\)/,
    'only-build submit should require both CI and CD bindings because backend staged build needs a later deploy unit',
  )
  assert.match(
    source,
    /const canBuildOnlySubmitRelease = computed\(\(\) => canSubmitRelease\.value && hasStagedBuildBindings\.value && !buildOnlyDisabledReason\.value\)/,
    'only-build submit should have its own availability gate',
  )
  assert.match(
    source,
    /async function submitRelease\(options\?: \{ fast\?: boolean; buildOnly\?: boolean \}\)/,
    'submitRelease should accept a buildOnly mode',
  )
  assert.match(
    source,
    /if \(buildOnly\) \{[\s\S]*await buildReleaseOrder\(response\.data\.id\)[\s\S]*message\.success\('发布单创建成功，已提交仅构建任务'\)[\s\S]*void router\.push\(`\/releases\/\$\{response\.data\.id\}`\)[\s\S]*return[\s\S]*\}/,
    'buildOnly mode should create the order, dispatch CI build, then enter detail',
  )
  assert.match(
    source,
    /async function handleBuildOnlySubmit\(\) \{[\s\S]*await submitRelease\(\{ buildOnly: true \}\)[\s\S]*\}/,
    'create page should expose a dedicated only-build click handler',
  )
  assert.match(
    source,
    /v-if="!isEditMode"[\s\S]*:loading="buildOnlySubmitting"[\s\S]*:aria-disabled="!canBuildOnlySubmitRelease"[\s\S]*@click="handleBuildOnlySubmit"[\s\S]*仅构建/,
    'header should render the only-build button for new release orders',
  )
})

test('release order create page uses plain form sections and required hints', () => {
  assert.match(
    source,
    /class="release-create-form application-form-plain"[\s\S]*:required-mark="false"/,
    'release form should use plain surface and disable default required stars',
  )
  assert.match(source, /<div class="create-layout">/, 'release create page should use a two-column create layout')
  assert.match(source, /<section class="form-section release-form-section">/, 'base fields should be in a plain form section')
  assert.match(source, /<h3 class="form-section-heading-title">发布基础<\/h3>/, 'base section title should be explicit')
  assert.match(source, /应用 <span class="field-required-hint">必填<\/span>/, 'application field should use required hint tag')
  assert.match(source, /发布模板 <span class="field-required-hint">必填<\/span>/, 'template field should use required hint tag')
  assert.match(source, /const showEnvironmentField = computed\(\(\) => templateHasBuiltinSource\(\['env', 'env_code'\]\)\)/, 'environment should only be shown when the template maps env builtins')
  assert.match(source, /const showReleaseBranchField = computed\(\(\) => templateHasBuiltinSource\(\['branch', 'git_ref'\]\)\)/, 'release branch should only be shown when the template maps branch builtins')
  assert.match(source, /v-if="showEnvironmentField"[\s\S]*环境 <span class="field-required-hint">必填<\/span>/, 'environment field should be conditional on builtin mapping')
  assert.match(source, /v-if="showReleaseBranchField"[\s\S]*发布分支/, 'release branch field should be conditional on builtin mapping')
  assert.match(
    source,
    /<a-row :gutter="12" class="form-row-compact">\s*<a-col v-if="showEnvironmentField" :xs="24" :md="8"[\s\S]*class="form-item-compact release-env-form-item"[\s\S]*环境 <span class="field-required-hint">必填<\/span>[\s\S]*<a-col v-if="showReleaseBranchField" :xs="24" :md="showEnvironmentField \? 16 : 24"[\s\S]*发布分支[\s\S]*<a-col v-else :xs="24" :md="showEnvironmentField \? 16 : 24"[\s\S]*备注/,
    'environment and release branch should share the first row when branch is shown, otherwise remark should share the row with environment',
  )
  assert.match(
    source,
    /\.release-env-form-item :deep\(\.ant-form-item-control\)[\s\S]*padding-top:\s*4px/,
    'environment control should sit slightly lower than the row baseline',
  )
  assert.match(
    source,
    /<a-row v-if="showReleaseBranchField" :gutter="12" class="form-row-compact">\s*<a-col :span="24">[\s\S]*备注/,
    'remark should move to a final full-width row when release branch is shown',
  )
  assert.match(source, /env_code: effectiveEnvCode\.value/, 'hidden environment field should still submit the resolved environment value')
  assert.match(source, /git_ref: showReleaseBranchField\.value \? \(formState\.git_ref\.trim\(\) \|\| undefined\) : undefined/, 'hidden branch field should not submit stale branch values')
  assert.doesNotMatch(source, /创建者[\s\S]*formCreatorDisplayName/, 'base fields should not show the creator readonly field')
  assert.doesNotMatch(source, /<a-card class="form-card"/, 'create form should not use the old heavy card shell')

  const layoutRule = extractStyleRule('.create-layout')
  assert.match(
    layoutRule,
    /grid-template-columns:\s*minmax\(0,\s*1fr\)\s*minmax\(260px,\s*320px\)/,
    'create layout should match the standardized form/sidebar grid',
  )
})

test('release order create page adds standardized sidebar guidance cards', () => {
  assert.match(
    source,
    /<aside class="create-sidebar">[\s\S]*发布创建流程[\s\S]*发布前检查/,
    'release create page should include the standardized right sidebar guidance cards',
  )
  assert.match(
    source,
    /选择应用与模板[\s\S]*确认发布信息[\s\S]*填写执行参数[\s\S]*创建发布单/,
    'release create process should describe the main release creation steps',
  )
  assert.match(
    source,
    /应用会决定当前账号是否有创建权限[\s\S]*模板启用审批人后只能创建发布单，不能极速发布[\s\S]*只有模板映射到发布基础字段时，才会展示对应填写项/,
    'release preflight tips should explain permission, approval and mapped base fields',
  )

  const sideCardRule = extractStyleRule('.create-side-card')
  assert.match(sideCardRule, /border-radius:\s*24px/, 'sidebar cards should use the standardized 24px radius')
  assert.match(sideCardRule, /rgba\(191,\s*219,\s*254,\s*0\.72\)/, 'sidebar cards should use the standardized light blue border')
})

test('release order create page consolidates ci cd fields under advanced params', () => {
  assert.match(
    source,
    /<h3 class="form-section-heading-title">高级参数<\/h3>/,
    'CI/CD parameter sections should be consolidated under a single advanced params title',
  )
  assert.doesNotMatch(
    source,
    /CI 参数|CD 参数|CI 构建参数|CD 发布参数/,
    'CI/CD labels should not remain as primary parameter section titles',
  )
  assert.match(
    source,
    /function visibleAdvancedScopeParams\(scope: ReleasePipelineScope\)/,
    'create page should expose only advanced params that require applicant input',
  )
  assert.match(
    source,
    /function isTemplateParamMappedFromBaseField\(scope: ReleasePipelineScope, item: ExecutorParamDef\)/,
    'create page should explicitly identify params mapped from base fields',
  )
  assert.match(
    source,
    /function isTemplateParamInheritedFromCiParam\(scope: ReleasePipelineScope, item: ExecutorParamDef\)/,
    'create page should explicitly identify CD params inherited from CI params',
  )
  assert.match(
    source,
    /resolveTemplateParamValueSource\(meta\) === 'builtin'/,
    'base-field mapped params should be detected from builtin value source',
  )
  assert.match(
    source,
    /scope === 'cd' && resolveTemplateParamValueSource\(meta\) === 'ci_param'/,
    'CD params that inherit CI params should be hidden from advanced params',
  )
  assert.match(
    source,
    /\.filter\(\(item\) => item\.loading \|\| item\.error \|\| item\.params\.length > 0\)/,
    'advanced params should hide scopes that only contain auto-filled builtin params',
  )
  assert.match(
    source,
    /v-for="param in item\.params\.slice/,
    'primary param rows should render from prefiltered visible advanced params',
  )
  assert.doesNotMatch(
    source,
    /readonlyBuiltinMappedScopeParams|release-auto-param-list|release-auto-param-item|已映射内置字段[\s\S]*发布链路自动赋值/,
    'new release orders should hide builtin auto-filled params instead of showing readonly cards',
  )
  assert.doesNotMatch(
    source,
    /function scopedAdvancedParamDisplayCount\(scope: ReleasePipelineScope\)/,
    'advanced params empty state should not count hidden builtin auto-filled params',
  )
  assert.match(
    source,
    /class="advanced-param-scope-group"/,
    'CI/CD groups should still exist inside advanced params',
  )
  assert.match(
    source,
    /ExclamationCircleOutlined/,
    'advanced params hint should use the standardized exclamation icon',
  )
  assert.match(
    source,
    /class="advanced-param-heading-hint"/,
    'advanced params should show a single heading hint icon',
  )
  assert.match(
    source,
    /<a-popover[\s\S]*trigger="click"[\s\S]*overlay-class-name="advanced-param-hint-popover"[\s\S]*<template #content>[\s\S]*class="advanced-param-hint-card"[\s\S]*<button[\s\S]*class="advanced-param-heading-hint"[\s\S]*type="button"[\s\S]*:aria-label="advancedParamAriaLabel"/,
    'advanced params hint icon should show a structured explanation popover on click and remain accessible',
  )
  assert.match(
    source,
    /class="advanced-param-hint-title"[\s\S]*参数说明[\s\S]*class="advanced-param-hint-row"[\s\S]*当前模板[\s\S]*selectedTemplate\?\.name[\s\S]*执行流程[\s\S]*enabledScopeSummary[\s\S]*填写范围[\s\S]*advancedParamFillHint/,
    'advanced params hint should split template, flow and fill guidance into readable rows',
  )
  assert.match(
    source,
    /\.advanced-param-hint-popover[\s\S]*\.advanced-param-hint-card[\s\S]*\.advanced-param-hint-row/,
    'advanced params popover should include dedicated styling hooks',
  )
  assert.doesNotMatch(source, /class="template-alert template-alert-success"[\s\S]*当前模板：/, 'selected template summary should not render as a separate alert')
  assert.doesNotMatch(source, /当前模板已启用 \$\{scopeText\} 执行字段/, 'advanced params hint should avoid verbose runtime scope wording')
  assert.doesNotMatch(
    source,
    /advanced-param-scope-name|advanced-param-scope-hint|release-tip-trigger|release-tip-content/,
    'advanced params should not show CI/CD icon pills, inline hint text, or right-side info popovers',
  )
  assert.doesNotMatch(
    source,
    /visibleAdvancedParamCount|需填写\s*\{\{[^}]+\}\}\s*个|需要填写\s*\$\{[^}]+\.value\}\s*个高级参数/,
    'advanced params should not show a count of fields to fill',
  )
  assert.match(
    source,
    /\.advanced-param-scope-group \+ \.advanced-param-scope-group/,
    'CI/CD groups inside advanced params should be separated by a dashed divider',
  )
  assert.doesNotMatch(
    source,
    /<template v-else>\s*<a-input[\s\S]*resolveTemplateParamDisplayValue/,
    'hidden params should not stay in the primary form as disabled input fields',
  )
  assert.doesNotMatch(
    source,
    /release-auto-param-control[\s\S]*<a-select|release-auto-param-control[\s\S]*<a-input/,
    'builtin mapped readonly rows should not render selectable or editable controls',
  )
})

test('release order create disables violated release templates', () => {
  assert.match(
    source,
    /disabled:\s*!isReleaseTemplateSelectable\(item\)/,
    'template dropdown options should disable violated templates',
  )
  assert.match(
    source,
    /function isReleaseTemplateSelectable\(item: ReleaseTemplate\)[\s\S]*item\.compliance_status !== 'violated'/,
    'template selection should be based on backend compliance status',
  )
  assert.match(
    source,
    /违规：\$\{item\.compliance_summary \|\| '管线规范不通过'\}/,
    'violated template labels should explain that the template is non-compliant',
  )
  assert.match(
    source,
    /if \(response\.data\.template\.compliance_status === 'violated'\)[\s\S]*formState\.template_id = ''[\s\S]*发布模板违反管线规范/,
    'direct route or stale selections should be rejected after template detail load',
  )
  assert.match(
    source,
    /!templates\.some\(isReleaseTemplateSelectable\)[\s\S]*当前应用下的发布模板均违反管线规范/,
    'empty selectable state should tell users to adjust template pipeline bindings',
  )
})
