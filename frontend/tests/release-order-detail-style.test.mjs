import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

function countMatches(pattern) {
  return Array.from(source.matchAll(pattern)).length
}

function extractHeaderActions() {
  const match = source.match(
    /<div class="page-header-actions release-detail-actions">([\s\S]*?)\n      <\/div>/,
  )
  assert.ok(match, 'expected to find detail header actions block')
  return match[1]
}

test('release detail page uses standardized transparent header actions', () => {
  const headerActions = extractHeaderActions()

  assert.match(
    source,
    /<div class="page-header release-detail-header">/,
    'detail page should use the transparent detail header',
  )
  assert.doesNotMatch(
    source,
    /<div class="header-left">[\s\S]*返回发布单[\s\S]*<\/div>\n      <div class="page-header-actions release-detail-actions">/,
    'back action should no longer live on the left side',
  )
  assert.match(
    headerActions,
    /class="application-toolbar-action-btn"[\s\S]*编辑[\s\S]*发布[\s\S]*返回发布单/,
    'detail actions and back action should live in the right header group',
  )
  assert.doesNotMatch(
    headerActions,
    /type="primary"|\bghost\b|\sdanger(?:\s|>)|release-detail-danger-btn|rollback-trigger-button/,
    'right header buttons should not keep divergent Ant primary, ghost, danger or rollback styles',
  )
  assert.doesNotMatch(
    source,
    /release-detail-danger-btn|rollback-trigger-button/,
    'detail page should not define divergent header button style classes',
  )
  assert.doesNotMatch(
    source,
    /page-header-card page-header/,
    'detail page should not use the old page header card shell',
  )
  assert.doesNotMatch(source, /ReloadOutlined/, 'detail page should not keep the old refresh action')

  const buttonRule = extractStyleRule(':deep(.application-toolbar-action-btn.ant-btn)')
  assert.match(buttonRule, /height:\s*42px/, 'detail toolbar buttons should use the shared height')
  assert.match(buttonRule, /border-radius:\s*16px/, 'detail toolbar buttons should use shared radius')
  assert.match(buttonRule, /color:\s*#0f172a !important/, 'detail toolbar button text should use the shared dark color')
})

test('stage log streaming hint only appears for running stages', () => {
  assert.match(
    source,
    /const stageLogStillStreaming = computed\([\s\S]*stageLogHasMore\.value[\s\S]*isRunningStatus\(selectedPipelineStage\.value!\.status\)/,
    'stage log streaming hint should depend on both has_more and running stage status',
  )
  assert.match(
    source,
    /const stageLogSyncMessage = computed\([\s\S]*stageLogStillStreaming\.value \? "，当前阶段仍在持续输出日志" : ""/,
    'stage log sync message should append streaming copy only through the guarded computed value',
  )
  assert.doesNotMatch(
    source,
    /stageLogHasMore \? '，当前阶段仍在持续输出日志'/,
    'template should not treat has_more alone as actively streaming',
  )
  assert.match(
    source,
    /stageLogSyncMessage[\s\S]*overlay-class-name="release-tip-popover"[\s\S]*\{\{ stageLogSyncMessage \}\}/,
    'stage log sync message should render through the guarded click tip',
  )
})

test('release detail keeps staged build action labeled as only-build', () => {
  const headerActions = extractHeaderActions()

  assert.match(
    source,
    /case "build":\s*return "仅构建";/,
    'detail build dispatch action text should stay explicit as only-build',
  )
  assert.match(
    headerActions,
    /v-if="canBuild"[\s\S]*@click="handleBuild"[\s\S]*>\s*仅构建\s*<\/a-button>/,
    'detail header should render the only-build button for staged orders',
  )
  assert.doesNotMatch(
    headerActions,
    /v-if="canBuild"[\s\S]*@click="handleBuild"[\s\S]*>\s*构建\s*<\/a-button>/,
    'detail header should not regress to the ambiguous build label',
  )
})

test('release detail uses publish label to continue cd after only-build', () => {
  const headerActions = extractHeaderActions()

  assert.match(
    source,
    /case "deploy":\s*return "发布";/,
    'detail CD continuation should use publish wording because it releases the built artifact',
  )
  assert.match(
    headerActions,
    /v-if="canDeploy"[\s\S]*@click="handleDeploy"[\s\S]*>\s*发布\s*<\/a-button>/,
    'detail header should render a publish button that dispatches deploy/CD',
  )
  assert.doesNotMatch(
    headerActions,
    /v-if="canDeploy"[\s\S]*@click="handleDeploy"[\s\S]*>\s*部署\s*<\/a-button>/,
    'detail header should not show deploy wording for CD continuation',
  )
})

test('release detail backgrounds preserve hero and redesign non-hero sections', () => {
  const cardRule = extractStyleRule('.detail-card')
  assert.match(
    cardRule,
    /border:\s*1px solid rgba\(148,\s*163,\s*184,\s*0\.14\)/,
    'detail cards should use a light non-white section border',
  )
  assert.match(
    cardRule,
    /background:\s*rgba\(241,\s*245,\s*249,\s*0\.34\)/,
    'detail cards should use a soft gray-blue section background',
  )
  assert.match(cardRule, /box-shadow:\s*none/, 'detail cards should remove extra card shadows')

  const standardCardRule = extractStyleRule('.detail-card:not(.release-hero-card)')
  assert.match(standardCardRule, /padding:\s*18px/, 'standard detail sections should use stronger inner spacing')
  assert.match(standardCardRule, /border-radius:\s*24px/, 'standard detail sections should use the redesigned larger radius')
  assert.match(
    standardCardRule,
    /rgba\(255,\s*255,\s*255,\s*0\.82\)/,
    'standard detail sections should move away from the flat gray card surface',
  )
  assert.match(
    standardCardRule,
    /0 18px 42px rgba\(15,\s*23,\s*42,\s*0\.045\)/,
    'standard detail sections should use a restrained lifted shell',
  )

  const timelineCollapseMatch = source.match(
    /<a-collapse class="detail-collapse timeline-collapse" ghost>([\s\S]*?)<\/a-collapse>/,
  )
  assert.ok(timelineCollapseMatch, 'execution timeline should render as a collapsible section')
  assert.match(
    timelineCollapseMatch[1],
    /<a-collapse-panel key="execution-timeline" header="执行时间线">/,
    'execution timeline should keep the original section title in the collapse header',
  )
  assert.doesNotMatch(
    timelineCollapseMatch[0],
    /active-key|default-active-key|v-model:activeKey|v-model:active-key/,
    'execution timeline should stay collapsed by default',
  )
  assert.match(
    source,
    /class="detail-card detail-section-card"[\s\S]*title="阶段与日志"/,
    'stage and log area should keep the redesigned section-card treatment',
  )
  assert.match(
    source,
    /class="detail-collapse base-info-collapse"[\s\S]*header="基础信息与参数快照"[\s\S]*class="detail-collapse value-progress-collapse"[\s\S]*class="value-progress-collapse-title">取值进度/,
    'base information and parameter snapshots should render above value progress in the main column',
  )
  const mainStageOrderRule = extractStyleRule('.dashboard-main > .detail-section-card')
  assert.match(mainStageOrderRule, /order:\s*1/, 'stage logs and value progress should render before secondary collapses')

  const baseInfoOrderRule = extractStyleRule('.dashboard-main > .base-info-collapse')
  assert.match(baseInfoOrderRule, /order:\s*2/, 'base information and snapshots should render above value progress')

  const valueProgressOrderRule = extractStyleRule('.dashboard-main > .value-progress-collapse')
  assert.match(valueProgressOrderRule, /order:\s*3/, 'value progress should render below base information and snapshots')

  const timelineOrderRule = extractStyleRule('.dashboard-main > .timeline-collapse')
  assert.match(timelineOrderRule, /order:\s*4/, 'execution timeline should render below value progress')

  const valueProgressCollapseMatch = source.match(
    /<a-collapse[\s\S]*class="detail-collapse value-progress-collapse"[\s\S]*<\/a-collapse>/,
  )
  assert.ok(valueProgressCollapseMatch, 'value progress should render as a collapsible section')
  assert.doesNotMatch(
    valueProgressCollapseMatch[0],
    /active-key|default-active-key|v-model:activeKey|v-model:active-key/,
    'value progress should stay collapsed by default',
  )

  assert.match(
    source,
    /class="detail-card detail-side-card"[\s\S]*执行单元/,
    'side detail areas should keep compact side-card treatment for execution units',
  )
  assert.doesNotMatch(
    source,
    /发布上下文|contextFacts|context-list|context-item|context-label|context-value/,
    'release context should be removed from the detail page',
  )

  const bodyRule = extractStyleRule('.detail-card :deep(.ant-card-body)')
  assert.match(bodyRule, /padding:\s*0/, 'standard detail card bodies should not add extra card padding')
  assert.match(bodyRule, /background:\s*transparent/, 'detail card bodies should stay clean')

  const heroRule = extractStyleRule('.release-hero-card')
  assert.match(heroRule, /var\(--color-primary-glow\)/, 'release order hero should restore its primary glow background')
  assert.match(
    heroRule,
    /linear-gradient\(\s*180deg,\s*var\(--color-bg-card\) 0%,\s*var\(--color-bg-subtle\) 100%/,
    'release order hero should restore the original light card gradient',
  )

  const heroBodyRule = extractStyleRule('.release-hero-card :deep(.ant-card-body)')
  assert.match(heroBodyRule, /padding:\s*22px 24px/, 'release order hero should restore card body spacing')

  const titleRule = extractStyleRule('.detail-card :deep(.ant-card-head-title)')
  assert.match(titleRule, /font-weight:\s*800/, 'detail card titles should keep strong hierarchy')
})

test('release detail content groups use non-nested redesigned surfaces', () => {
  assert.doesNotMatch(
    source,
    /class="nested-card"|\.nested-card/,
    'detail page should not keep visual nested card shells inside outer sections',
  )
  assert.doesNotMatch(
    source,
    /<a-alert/,
    'detail page hints should use click tip icons instead of block alerts',
  )
  assert.match(
    source,
    /header="基础信息与参数快照"[\s\S]*class="detail-inline-section"[\s\S]*基础信息[\s\S]*class="detail-inline-section"[\s\S]*group\.title/,
    'base info and param snapshots should render as flat inline sections in one collapse',
  )
  assert.match(
    source,
    /class="detail-info-descriptions"[\s\S]*class="detail-data-table detail-snapshot-table"/,
    'base info and param snapshots should use paired information-table styles',
  )

  const scopeRule = extractStyleRule('.scope-section')
  assert.match(scopeRule, /border-radius:\s*0/, 'timeline and stage groups should not render as inner cards')
  assert.match(scopeRule, /background:\s*transparent/, 'timeline and stage groups should stay flat inside the outer card')
  assert.match(scopeRule, /border-bottom:\s*1px solid/, 'timeline and stage groups should separate with list dividers')

  const inlineRule = extractStyleRule('.detail-inline-section')
  assert.match(inlineRule, /background:\s*transparent/, 'inline sections should not introduce nested card surfaces')
  assert.match(inlineRule, /border:\s*none/, 'inline sections should not introduce nested card borders')

  const inlineTitleRule = extractStyleRule('.detail-inline-section-title')
  assert.match(inlineTitleRule, /font-size:\s*14px/, 'base info and param snapshot titles should use the same size')

  const snapshotHeaderRule = extractStyleRule('.detail-info-descriptions :deep(.ant-descriptions-item-label),\n.detail-snapshot-table :deep(.ant-table-thead > tr > th),\n.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell),\n.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell-fix-left),\n.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell-fix-right)')
  assert.match(
    snapshotHeaderRule,
    /linear-gradient\(180deg,\s*#f8fafc 0%,\s*#f1f5f9 100%\)/,
    'param snapshot table header should match the light base-info label surface',
  )
  assert.match(snapshotHeaderRule, /color:\s*#475569 !important/, 'param snapshot table header should match base-info label text color')

  const collapseRule = extractStyleRule('.detail-collapse :deep(.ant-collapse-item)')
  assert.match(collapseRule, /border-radius:\s*24px !important/, 'base info collapse should match the redesigned section shell')

  const executionRule = extractStyleRule('.execution-summary-card')
  assert.match(executionRule, /border-radius:\s*0/, 'execution units should render as rows instead of nested cards')
  assert.match(executionRule, /background:\s*transparent/, 'execution units should avoid inner card backgrounds')
  assert.match(executionRule, /box-shadow:\s*none/, 'execution units should avoid nested shadows')
  assert.doesNotMatch(source, /\.execution-summary-card::before/, 'execution units should not use timeline-like vertical markers')

  assert.match(
    source,
    /v-for="\(\s*unit,\s*index\s*\) in executionUnitItems"[\s\S]*class="execution-summary-order"/,
    'execution units should use one sorted list with compact numeric badges instead of duplicating the timeline design',
  )
  assert.match(
    source,
    /const executionUnitItems = computed<ExecutionUnitItem\[\]>/,
    'pipeline and hook units should be merged before rendering',
  )
  assert.match(
    source,
    /return \[\.\.\.pipelineItems, \.\.\.hookItems\]\.sort/,
    'hook units should be sorted together with CI/CD units',
  )
  const orderRule = extractStyleRule('.execution-summary-order')
  assert.match(orderRule, /border-radius:\s*999px/, 'execution order should be shown as a compact pill badge')

  const executionSummaryHeadRule = extractStyleRule('.execution-summary-head')
  assert.match(executionSummaryHeadRule, /align-items:\s*flex-start/, 'execution unit status should stay at the top-right aligned with the title row')

  assert.match(
    source,
    /unit\.kind === 'pipeline'[\s\S]*class="execution-summary-actions"[\s\S]*'execution-summary-status'[\s\S]*statusToneClass\(unit\.execution\.status\)/,
    'pipeline execution status should live in the same right-side top action rail',
  )
  const pipelineExecutionTemplate = source.match(
    /<template v-if="unit\.kind === 'pipeline'">([\s\S]*?)<\/template>\s*<template v-else>/,
  )
  assert.ok(pipelineExecutionTemplate, 'expected to find pipeline execution template branch')
  assert.doesNotMatch(
    pipelineExecutionTemplate[1],
    /class="hook-task-toggle"|unit\.group/,
    'pipeline execution units must not render hook task controls or access unit.group',
  )

  const metaRule = extractStyleRule('.execution-summary-meta')
  assert.match(metaRule, /grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\)/, 'execution metadata should use a compact matrix layout')

  assert.doesNotMatch(source, /hookContextPreviewItems|hookVariablePreviewItems|expandedHookVariableMap|查看上下文|Hook 上下文/, 'hook context panel should be removed from execution units')
  assert.match(
    source,
    /const expandedHookTaskMap = reactive<Record<string, boolean>>\(\{\}\);[\s\S]*function toggleHookTasks\(key: string\)[\s\S]*expandedHookTaskMap\[key\] = !expandedHookTaskMap\[key\]/,
    'hook task groups should keep explicit collapsed-by-default state',
  )
  assert.match(
    source,
    /class="execution-summary-actions"[\s\S]*class="hook-task-toggle"[\s\S]*shape="round"[\s\S]*@click="toggleHookTasks\(unit\.group\.key\)"[\s\S]*展开任务[\s\S]*'execution-summary-status'[\s\S]*v-if="expandedHookTaskMap\[unit\.group\.key\]"[\s\S]*class="hook-progress-rows"/,
    'hook task toggle should sit to the left of the status and rows should render only after expanding the hook unit',
  )
  assert.match(source, /class="hook-progress-task-grid"[\s\S]*阶段\/类型[\s\S]*关联任务[\s\S]*开始[\s\S]*结束/, 'hook task rows should use a structured task information grid')
  assert.match(source, /function hookTaskReferenceText\(item: ReleaseOrderStep\)/, 'hook task rows should derive related task references from the current hook item')
  const hookTaskGridRule = extractStyleRule('.hook-progress-task-grid')
  assert.match(hookTaskGridRule, /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)/, 'hook task metadata should be grouped into a readable two-column grid')

  const executionSummaryActionsRule = extractStyleRule('.execution-summary-actions')
  assert.match(executionSummaryActionsRule, /display:\s*inline-flex/, 'hook execution actions should keep status and task toggle aligned')
  assert.match(executionSummaryActionsRule, /min-height:\s*24px/, 'execution action rail should align with the title line height')

  const executionSummaryTitleRule = extractStyleRule('.execution-summary-title')
  assert.match(executionSummaryTitleRule, /line-height:\s*24px/, 'execution titles should share the status rail baseline')

  const hookTaskToggleRule = extractStyleRule('.hook-task-toggle')
  assert.match(hookTaskToggleRule, /font-size:\s*12px/, 'hook task toggle should stay visually secondary')
  assert.match(hookTaskToggleRule, /border-radius:\s*999px/, 'hook task toggle should render as a compact pill button')
  assert.match(hookTaskToggleRule, /border-color:\s*rgba\(147,\s*197,\s*253,\s*0\.72\)/, 'hook task toggle should use a visible but quiet button border')

  const hookTaskRowRule = extractStyleRule('.hook-progress-row')
  assert.match(hookTaskRowRule, /border-radius:\s*16px/, 'hook task rows should have a clear but lightweight boundary')
  assert.match(hookTaskRowRule, /background:\s*rgba\(255,\s*255,\s*255,\s*0\.46\)/, 'hook task rows should use a subtle local surface')

  const hookTaskIndexRule = extractStyleRule('.hook-progress-item-index')
  assert.match(hookTaskIndexRule, /border-radius:\s*999px/, 'hook task sequence marker should be a compact pill')

  assert.doesNotMatch(source, /pipelineStageColumns|pipelineStageInitialColumns|:columns="pipelineStageColumns"/, 'pipeline progress should no longer render as an Ant table')
  assert.doesNotMatch(source, /pipeline-stage-board|pipeline-stage-card|pipeline-stage-meta-grid|pipeline-stage-list|pipeline-stage-row/, 'pipeline progress should not use oversized card-grid or plain list presentation')
  assert.doesNotMatch(source, /class="stage-toolbar stage-toolbar-lower"/, 'pipeline executor controls should not sit in a separate row')
  assert.match(
    source,
    /class="scope-section-heading"[\s\S]*section\.title[\s\S]*class="pipeline-stage-title-actions"[\s\S]*class="pipeline-executor-chip"[\s\S]*section\.execution\?\.provider[\s\S]*刷新阶段/,
    'pipeline executor type and refresh action should sit next to each pipeline section title',
  )
  assert.match(
    source,
    /class="scope-section-meta"[\s\S]*section\.execution\?\.binding_name[\s\S]*aria-label="查看阶段提示"/,
    'section-specific tips should live beside the right-side executor metadata instead of crowding the title',
  )
  assert.match(
    source,
    /class="pipeline-stage-chain"[\s\S]*class="pipeline-stage-node"[\s\S]*class="pipeline-stage-meta-line"/,
    'pipeline progress should render as a connected one-line chain',
  )
  assert.doesNotMatch(source, /pipeline-stage-actions|EyeOutlined/, 'pipeline stage log access should not use a visible log button')
  assert.match(
    source,
    /'pipeline-stage-node-clickable': section\.isJenkins[\s\S]*@click="section\.isJenkins && openStageLogDrawer\(stage\)"/,
    'Jenkins pipeline stage nodes should be clickable to open logs',
  )
  const pipelineStageChainRule = extractStyleRule('.pipeline-stage-chain')
  assert.match(pipelineStageChainRule, /display:\s*flex/, 'pipeline stage chain should use a horizontal flow')
  assert.match(pipelineStageChainRule, /flex-wrap:\s*wrap/, 'pipeline stage chain should wrap instead of requiring horizontal drag')
  assert.match(pipelineStageChainRule, /overflow:\s*visible/, 'pipeline stage chain should avoid horizontal scrolling by default')

  const scopeHeadingRule = extractStyleRule('.scope-section-heading')
  assert.match(scopeHeadingRule, /display:\s*inline-flex/, 'pipeline section heading should keep title actions on the same row')

  const pipelineTitleActionsRule = extractStyleRule('.pipeline-stage-title-actions')
  assert.match(pipelineTitleActionsRule, /align-items:\s*center/, 'pipeline title actions should align with the section title')

  const pipelineExecutorChipRule = extractStyleRule('.pipeline-executor-chip')
  assert.match(pipelineExecutorChipRule, /background:\s*rgba\(248,\s*250,\s*252,\s*0\.76\)/, 'pipeline executor chip should use a low-contrast neutral surface')
  assert.match(pipelineExecutorChipRule, /color:\s*#475569/, 'pipeline executor chip should avoid high-saturation running blue')

  const scopeMetaRule = extractStyleRule('.scope-section-meta')
  assert.match(scopeMetaRule, /justify-content:\s*flex-end/, 'pipeline section tips should align with the right-side metadata')

  const tipTriggerRule = extractStyleRule('.release-tip-trigger')
  assert.match(tipTriggerRule, /width:\s*18px/, 'tip icon should be small enough to avoid dominating the section header')
  assert.match(tipTriggerRule, /border:\s*none/, 'tip icon should avoid the heavy blue badge treatment')
  assert.match(tipTriggerRule, /background:\s*transparent/, 'tip icon should render as a lightweight inline hint')

  const pipelineStageNodeRule = extractStyleRule('.pipeline-stage-node')
  assert.match(pipelineStageNodeRule, /flex:\s*1 1 132px/, 'pipeline stage nodes should be compact enough to fit multiple items per row')
  assert.match(pipelineStageNodeRule, /max-width:\s*168px/, 'pipeline stage nodes should not become oversized')

  const pipelineStageConnectorRule = extractStyleRule('.pipeline-stage-node:not(:last-child)::after')
  assert.match(pipelineStageConnectorRule, /right:\s*-16px/, 'pipeline stage nodes should keep compact connectors')

  const pipelineStageClickableRule = extractStyleRule('.pipeline-stage-node-clickable')
  assert.match(pipelineStageClickableRule, /cursor:\s*pointer/, 'clickable pipeline stage nodes should expose hover-click affordance')

  const pipelineStageMetaRule = extractStyleRule('.pipeline-stage-meta-line')
  assert.match(pipelineStageMetaRule, /flex-direction:\s*column/, 'pipeline stage metadata should stay compact inside each chain node')
  assert.match(pipelineStageMetaRule, /font-size:\s*10px/, 'pipeline stage metadata should stay visually secondary')

  const pipelineStageMetaTextRule = extractStyleRule('.pipeline-stage-meta-line span')
  assert.match(pipelineStageMetaTextRule, /text-overflow:\s*ellipsis/, 'long pipeline stage metadata should truncate instead of overflowing')
})

test('release detail inner tables follow management table style', () => {
  assert.equal(
    countMatches(/class="[^"]*\bdetail-data-table\b/g),
    2,
    'detail page should only keep the actual data table for param snapshots',
  )

  const containerRule = extractStyleRule('.detail-data-table :deep(.ant-table-container)')
  assert.match(containerRule, /border-radius:\s*18px/, 'detail tables should use the shared rounded table container')
  assert.match(containerRule, /background:\s*transparent/, 'detail tables should avoid heavy white outer cards')

  const headerRule = extractStyleRule('.detail-data-table :deep(.ant-table-thead > tr > th)')
  assert.match(
    headerRule,
    /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/,
    'detail table headers should use the standardized dark slate gradient',
  )
  assert.match(headerRule, /color:\s*#e2e8f0 !important/, 'detail table headers should use light text')

  const fixedHeaderRule = extractStyleRule('.detail-data-table :deep(.ant-table-thead .ant-table-cell),\n.detail-data-table :deep(.ant-table-thead .ant-table-cell-fix-left),\n.detail-data-table :deep(.ant-table-thead .ant-table-cell-fix-right)')
  assert.match(
    fixedHeaderRule,
    /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/,
    'fixed pipeline table header cells should not fall back to white backgrounds',
  )

  assert.doesNotMatch(
    source,
    /\.detail-data-table :deep\(\.ant-table-cell-fix-left\),\n\.detail-data-table :deep\(\.ant-table-cell-fix-right\)/,
    'fixed column white backgrounds should be scoped to table body cells only',
  )

  const bodyRule = extractStyleRule('.detail-data-table :deep(.ant-table-tbody > tr > td)')
  assert.match(
    bodyRule,
    /background:\s*rgba\(255,\s*255,\s*255,\s*0\.64\)/,
    'detail table rows should use the standardized translucent white background',
  )

  assert.match(
    source,
    /class="detail-collapse value-progress-collapse"[\s\S]*key="value-progress"[\s\S]*valueProgressTotal[\s\S]*overlay-class-name="release-tip-popover"[\s\S]*这里展示模板中已映射标准 Key 的实时取值情况[\s\S]*v-for="group in valueProgressGroups"[\s\S]*class="value-progress-item-list"[\s\S]*v-for="item in group\.items"[\s\S]*class="value-progress-row-meta"/,
    'CI and CD value progress should be merged into a collapsed grouped list with a header tip icon',
  )

  assert.doesNotMatch(
    source,
    /valueProgressColumns|valueProgressInitialColumns|class="detail-data-table value-progress-table"|detail-side-card value-progress-card|value-progress-combined-card|value-progress-detail-grid|value-progress-detail-value/,
    'value progress should not use table column config, Ant table rendering, side-card placement, or per-field card grids',
  )

  const valueProgressCollapseHeadingRule = extractStyleRule('.value-progress-collapse-heading')
  assert.match(valueProgressCollapseHeadingRule, /display:\s*inline-flex/, 'value progress collapse header should keep title and summary together')

  const valueProgressCollapseTitleRule = extractStyleRule('.value-progress-collapse-title')
  assert.match(valueProgressCollapseTitleRule, /font-size:\s*inherit/, 'value progress title should inherit the same size as the execution timeline header')
  assert.match(valueProgressCollapseTitleRule, /font-weight:\s*inherit/, 'value progress title should inherit the same weight as the execution timeline header')

  const valueProgressGroupListRule = extractStyleRule('.value-progress-group-list')
  assert.match(valueProgressGroupListRule, /display:\s*flex/, 'merged value progress card should stack grouped sections')

  const valueProgressGroupHeaderRule = extractStyleRule('.value-progress-group-header')
  assert.match(valueProgressGroupHeaderRule, /justify-content:\s*space-between/, 'merged value progress groups should expose a compact header')
  assert.doesNotMatch(
    source,
    /\$\{scopeLabel\(scope\)\} 取值进度|class="value-progress-group-title"[\s\S]{0,220}group\.title/,
    'value progress group headers should not render repeated CI/CD value-progress titles',
  )

  const valueProgressItemListRule = extractStyleRule('.value-progress-item-list')
  assert.match(valueProgressItemListRule, /border-radius:\s*16px/, 'value progress fields should sit inside one integrated list container')
  assert.match(valueProgressItemListRule, /overflow:\s*hidden/, 'integrated value progress list should clip row separators to the outer radius')

  const valueProgressItemRule = extractStyleRule('.value-progress-item')
  assert.match(valueProgressItemRule, /display:\s*grid/, 'value progress entries should render as compact list rows')
  assert.match(valueProgressItemRule, /border-bottom:\s*1px solid/, 'value progress rows should separate with list dividers')
  assert.match(valueProgressItemRule, /background:\s*transparent/, 'value progress rows should not each look like independent cards')
  assert.doesNotMatch(valueProgressItemRule, /border-radius:/, 'value progress rows should not each have independent rounded corners')

  const valueProgressRowMetaRule = extractStyleRule('.value-progress-row-meta')
  assert.match(
    valueProgressRowMetaRule,
    /grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\)/,
    'value progress metadata should use a compact three-column list row',
  )

  const valueProgressRowMetaValueRule = extractStyleRule('.value-progress-row-meta em')
  assert.match(
    valueProgressRowMetaValueRule,
    /text-overflow:\s*ellipsis/,
    'long value progress content should truncate instead of overflowing',
  )
})
