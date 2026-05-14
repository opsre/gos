import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/component/JenkinsManagementView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected to find style rule for ${selector}`)
  return match[1]
}

test('jenkins management uses standardized header search and actions', () => {
  assert.match(source, /<div class="page-header">[\s\S]*<div class="page-header-actions">/, 'page should use transparent header actions')
  assert.match(source, /class="application-toolbar-icon-btn"[\s\S]*SearchOutlined/, 'keyword search should be an icon entry in the header')
  assert.match(source, /class="jenkins-toolbar-select"[\s\S]*状态 · 全部[\s\S]*class="jenkins-toolbar-query-btn"[\s\S]*查询/, 'status filter and query should live in the header action strip')
  assert.match(source, /scanAllPipelines/, 'pipeline list should use the batch scan API')
  assert.match(source, /async function handleScanAllPipelines\(\)[\s\S]*await scanAllPipelines\(\)[\s\S]*await loadPipelines\(\{\s*waitForScanResults:\s*true\s*\}\)/, 'batch scan should refresh the list scan status after completion')
  assert.match(source, /class="application-toolbar-action-btn"[\s\S]*新增管线[\s\S]*class="application-toolbar-action-btn"[\s\S]*扫描[\s\S]*class="application-toolbar-action-btn"[\s\S]*手动同步/, 'create, scan and sync actions should use shared toolbar button styling in the requested order')
  assert.doesNotMatch(source, /page-header-card|filter-card|filter-form|handleReset/, 'page should not keep old header card, filter card or reset action')

  const actionButtonRule = extractStyleRule(':deep(.application-toolbar-action-btn.ant-btn),\n:deep(.application-toolbar-icon-btn.ant-btn),\n:deep(.jenkins-toolbar-query-btn.ant-btn)')
  assert.match(actionButtonRule, /height:\s*42px/, 'toolbar buttons should use standardized height')
  assert.match(actionButtonRule, /border-radius:\s*16px/, 'toolbar buttons should use standardized radius')
  assert.match(actionButtonRule, /color:\s*#0f172a !important/, 'toolbar buttons should keep shared dark text')

  const selectRule = extractStyleRule(':deep(.jenkins-toolbar-select.ant-select .ant-select-selector)')
  assert.match(selectRule, /height:\s*42px !important/, 'toolbar select should align with button height')
  assert.match(selectRule, /border-radius:\s*16px !important/, 'toolbar select should align with button radius')
})

test('jenkins management search overlay follows query UI pattern', () => {
  assert.match(source, /class="jenkins-search-overlay"[\s\S]*placeholder="管线名称 \/ Jenkins 路径"[\s\S]*@keydown\.enter="handleSearchSubmit"/, 'search should open a floating overlay with a single keyword input')
  assert.match(source, /listPipelines\(\{[\s\S]*provider:\s*'jenkins'[\s\S]*page_size:\s*6/, 'search suggestions should use backend pipeline query with a small page size')
  assert.match(source, /window\.setTimeout\(\(\) => \{[\s\S]*fetchSearchSuggestions\(keyword\)[\s\S]*\}, 220\)/, 'search suggestions should be debounced')

  const overlayRule = extractStyleRule('.jenkins-search-overlay')
  assert.match(overlayRule, /left:\s*var\(--layout-sider-width,\s*220px\)/, 'search overlay should not cover the side navigation')
  assert.match(overlayRule, /backdrop-filter:\s*blur\(8px\)/, 'search overlay should use a light blur')

  const suggestionRule = extractStyleRule('.jenkins-search-suggestion')
  assert.match(suggestionRule, /cursor:\s*pointer/, 'suggestions should be clickable buttons')
})

test('jenkins management table uses management table style', () => {
  assert.match(source, /<a-card class="table-card"[\s\S]*<a-table[\s\S]*class="jenkins-table"/, 'table should be directly hosted in the main content area')
  assert.match(source, /column\.key === 'job_name'[\s\S]*class="jenkins-pipeline-name-link"[\s\S]*@click="openOriginalLink\(record\)"/, 'pipeline name should open the Jenkins original link')
  assert.match(source, /const directTarget = String\(record\.job_url \|\| ''\)\.trim\(\)[\s\S]*window\.open\(directTarget, '_blank', 'noopener,noreferrer'\)/, 'original link should prefer the job_url already returned by the list API')
  assert.match(source, /class="jenkins-row-actions"[\s\S]*jenkins-row-action-btn-script[\s\S]*原始脚本[\s\S]*jenkins-row-action-btn[\s\S]*编辑[\s\S]*jenkins-row-more-btn[\s\S]*更多/, 'row actions should show script, edit and a neutral more entry')
  assert.match(source, /title:\s*'操作',\s*key:\s*'actions',\s*width:\s*268/, 'actions column should be wide enough for visible row actions')
  assert.match(source, /class="jenkins-hidden-danger-panel"[\s\S]*危险操作[\s\S]*class="jenkins-hidden-delete-btn"[\s\S]*删除管线/, 'delete should be hidden inside the dangerous action popover')
  assert.doesNotMatch(source, />\s*原始链接\s*<|ExportOutlined|openingOriginalID/, 'original link should not remain as a separate row action')
  assert.doesNotMatch(source, /class="filter-card"|class="advanced-search-panel"/, 'table should not be preceded by a heavy filter card')

  const tableCardRule = extractStyleRule('.table-card')
  assert.match(tableCardRule, /background:\s*transparent/, 'table outer card should stay transparent')
  assert.match(tableCardRule, /box-shadow:\s*none/, 'table outer card should not add a heavy shell')
  assert.match(tableCardRule, /border-radius:\s*0/, 'table outer card should not round the table shell')
  assert.doesNotMatch(tableCardRule, /border-radius:\s*(18|20)px/, 'table outer card should not keep the old rounded shell')

  const tableContainerRule = extractStyleRule('.jenkins-table :deep(.ant-table-container)')
  assert.match(tableContainerRule, /border-radius:\s*0\s*!important/, 'table container should be a proper square table')
  assert.doesNotMatch(tableContainerRule, /border-radius:\s*(18|20)px/, 'table container should not keep the old rounded shell')

  const tableInnerRule = extractStyleRule('.jenkins-table :deep(.ant-table),\n.jenkins-table :deep(.ant-table-content),\n.jenkins-table :deep(.ant-table-body)')
  assert.match(tableInnerRule, /border-radius:\s*0\s*!important/, 'inner ant table surfaces should also reset radius')

  const tableHeaderRule = extractStyleRule('.jenkins-table :deep(.ant-table-thead > tr > th)')
  assert.match(tableHeaderRule, /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/, 'table header should use dark slate gradient')
  assert.match(tableHeaderRule, /color:\s*rgba\(239,\s*246,\s*255,\s*0\.96\)/, 'table header should use light text')
  assert.match(tableHeaderRule, /border-radius:\s*0\s*!important/, 'table header cells should keep square corners')

  const fixedActionRule = extractStyleRule('.jenkins-table :deep(.ant-table-cell-fix-right)')
  assert.match(fixedActionRule, /background:\s*#fff !important/, 'fixed action column should keep an opaque background')

  const nameLinkRule = extractStyleRule('.jenkins-pipeline-name-link')
  assert.match(nameLinkRule, /font-weight:\s*700/, 'pipeline name link should keep table emphasis')
  assert.match(nameLinkRule, /cursor:\s*pointer/, 'pipeline name should clearly be clickable')

  const rowActionsRule = extractStyleRule('.jenkins-row-actions')
  assert.match(rowActionsRule, /gap:\s*8px/, 'row actions should keep compact spacing')
  assert.match(rowActionsRule, /min-width:\s*236px/, 'row actions should not be clipped by the fixed column')

  const rowButtonRule = extractStyleRule(':deep(.jenkins-row-action-btn.ant-btn)')
  assert.match(rowButtonRule, /height:\s*28px/, 'row action buttons should be compact')
  assert.match(rowButtonRule, /border-radius:\s*999px/, 'row action buttons should use a pill shape')
  assert.match(rowButtonRule, /flex:\s*none/, 'row action buttons should not shrink and clip text')

  const moreButtonRule = extractStyleRule('.jenkins-row-more-btn')
  assert.match(moreButtonRule, /min-width:\s*58px/, 'more action should keep a stable width')

  const dangerPanelRule = extractStyleRule('.jenkins-hidden-danger-panel')
  assert.match(dangerPanelRule, /width:\s*220px/, 'hidden danger panel should have a compact controlled width')

  const hiddenDeleteRule = extractStyleRule(':deep(.jenkins-hidden-delete-btn.ant-btn)')
  assert.match(hiddenDeleteRule, /width:\s*100%/, 'hidden delete action should be explicit inside the popover')
})

test('jenkins editor modal follows button modal style', () => {
  assert.match(source, /:closable="false"[\s\S]*:footer="null"[\s\S]*:destroy-on-close="true"[\s\S]*wrap-class-name="jenkins-editor-modal-wrap"/, 'editor modal should disable default close and footer')
  assert.match(source, /class="jenkins-editor-modal-titlebar"[\s\S]*class="application-toolbar-action-btn jenkins-editor-modal-action-btn"[\s\S]*预览配置XML[\s\S]*class="application-toolbar-action-btn jenkins-editor-modal-save-btn"[\s\S]*保存/, 'editor actions should live in the modal titlebar')
  assert.match(source, /class="jenkins-editor-note"[\s\S]*仅支持 Jenkins inline raw pipeline/, 'editor helper copy should use a lightweight note')
  assert.doesNotMatch(source, /<template #footer>|<a-alert/, 'editor should not use default footer buttons or block alerts')

  const modalContentRule = extractStyleRule('.jenkins-editor-modal-wrap :deep(.ant-modal-content)')
  assert.match(modalContentRule, /border-radius:\s*24px/, 'modal shell should use standardized radius')
  assert.match(modalContentRule, /backdrop-filter:\s*blur\(18px\) saturate\(180%\)/, 'modal shell should use glass treatment')

  const titlebarRule = extractStyleRule('.jenkins-editor-modal-titlebar')
  assert.match(titlebarRule, /justify-content:\s*space-between/, 'modal titlebar should separate title and actions')
})
