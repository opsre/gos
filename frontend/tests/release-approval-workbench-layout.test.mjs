import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseApprovalWorkbenchView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  return match?.[1] || ''
}

test('release approval workbench is reduced to task inbox and handled records', () => {
  assert.match(source, /class="page-header-card page-header release-approval-page-header"/, 'approval workbench should expose a transparent page header shell')
  assert.match(source, /class="application-toolbar-action-btn approval-refresh-btn"/, 'approval workbench should expose the shared glass refresh action in the header')
  assert.match(source, /class="approval-summary-grid"/, 'approval workbench should keep summary cards in a dedicated grid')
  assert.match(source, /class="approval-summary-card"/, 'approval workbench summary items should use dedicated clickable card styling')
  assert.match(source, /:class="\[`approval-summary-card-\$\{item\.key\}`,\s*\{\s*'is-active': activeTab === item\.key\s*\}\]"/, 'approval workbench summary cards should expose per-card tone classes')
  assert.match(source, /@click="activeTab = item\.key"/, 'approval workbench summary cards should switch the active dataset directly')
  assert.match(source, /<h2 class="page-title">审批待办<\/h2>/, 'approval workbench should be named as a focused approval inbox')
  assert.match(source, /type TabKey = 'pending' \| 'handled'/, 'approval inbox should only expose pending and handled views')
  assert.doesNotMatch(source, /全部记录/, 'approval inbox should not duplicate the global audit record list')
  assert.doesNotMatch(source, /<a-tabs/, 'approval workbench should not rely on the default Ant tabs shell any more')
  assert.doesNotMatch(source, /class="page-subtitle"/, 'approval workbench should remove the page header subtitle')

  const headerRule = extractStyleRule('.release-approval-page-header')
  assert.match(headerRule, /background:\s*transparent\s*!important/, 'approval workbench header should remove the old card background')
  assert.match(headerRule, /box-shadow:\s*none\s*!important/, 'approval workbench header should remove the old card shadow')

  const summaryGridRule = extractStyleRule('.approval-summary-grid')
  assert.match(summaryGridRule, /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)/, 'approval workbench summary cards should use a two-column desktop grid')

  assert.match(source, /approval-summary-card-pending\.is-active/, 'pending summary card should expose its own active tone')
  assert.match(source, /approval-summary-card-handled\.is-active/, 'handled summary card should expose its own active tone')
})

test('release approval workbench consolidates lists into a single light panel with management-table styling', () => {
  assert.match(source, /class="approval-workbench-panel"/, 'approval workbench should wrap list content in a dedicated panel')
  assert.match(source, /class="approval-workbench-table"/, 'approval workbench tables should expose a dedicated styling hook')
  assert.equal((source.match(/:pagination="false"/g) || []).length, 2, 'approval workbench tables should disable built-in pagination for both datasets')
  assert.match(source, /class="pagination-area"/, 'approval workbench should move pagination into the shared footer area')
  assert.doesNotMatch(source, /class="approval-workbench-panel-eyebrow"/, 'approval workbench should not keep the extra inner section eyebrow copy')
  assert.doesNotMatch(source, /class="approval-workbench-panel-title"/, 'approval workbench should not keep the repeated panel title above the table body')
  assert.doesNotMatch(source, /class="approval-workbench-panel-subtitle"/, 'approval workbench should remove the redundant table-area subtitle')

  const panelRule = extractStyleRule('.approval-workbench-panel')
  assert.match(panelRule, /background:\s*transparent/, 'approval workbench should remove the extra outer background card around the table')
  assert.match(panelRule, /border:\s*none/, 'approval workbench should remove the extra outer border around the table')
  assert.match(panelRule, /box-shadow:\s*none/, 'approval workbench should remove the extra outer shadow around the table')

  const tableHeadRule = extractStyleRule('.approval-workbench-table :deep(.ant-table-thead > tr > th)')
  assert.match(tableHeadRule, /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/, 'approval workbench table header should use the dark management-table gradient')
})

test('release approval workbench uses aggregated task data and task-specific actions', () => {
  assert.match(source, /listReleaseApprovalWorkbenchTasks/, 'pending approval should load from the aggregated task endpoint')
  assert.match(source, /listReleaseApprovalWorkbenchRecords/, 'handled approval should load from the aggregated record endpoint')
  assert.match(source, /approveReleaseOrderApprovalFlowTask\(task\.release_order_id, task\.task_id/, 'custom flow approval must submit the current task id')
  assert.match(source, /rejectReleaseOrderApprovalFlowTask\(task\.release_order_id, task\.task_id/, 'custom flow rejection must submit the current task id')
  assert.match(source, /task\.source === 'flow'/, 'workbench should keep legacy approval compatibility explicit')
  assert.match(source, /当前审批任务/, 'pending table should expose the current approval node')
  assert.match(source, /执行模式/, 'pending table should expose the execution scope')
})

test('release approval action modal uses the button dialog shell instead of the default footer', () => {
  assert.match(source, /const approvalActionViewportInset = ref\(0\)/, 'approval action modal should track the live content inset')
  assert.match(source, /const approvalActionMaskStyle = computed\(\(\) => \(\{[\s\S]*background: 'rgba\(15,\s*23,\s*42,\s*0\.08\)'[\s\S]*backdropFilter: 'blur\(10px\)'/, 'approval action modal should use the constrained light mask')
  assert.match(source, /const approvalActionWrapProps = computed\(\(\) => \(\{[\s\S]*left: `\$\{approvalActionViewportInset\.value\}px`[\s\S]*width: `calc\(100% - \$\{approvalActionViewportInset\.value\}px\)`/, 'approval action modal should stay inside the content area')
  assert.match(source, /:closable="false"/, 'approval action modal should remove the default close icon')
  assert.match(source, /:footer="null"/, 'approval action modal should remove the default footer')
  assert.match(source, /wrap-class-name="approval-action-modal-wrap"/, 'approval action modal should expose a dedicated shell class')
  assert.match(source, /<template #title>[\s\S]*class="approval-action-modal-titlebar"[\s\S]*class="application-toolbar-action-btn approval-action-modal-submit-btn"/, 'approval action modal should move the primary action into the title bar')
  assert.doesNotMatch(source, /@ok="handleApprovalAction"/, 'approval action modal should not use the default ok handler')
  assert.doesNotMatch(source, /confirm-loading="approvalActing"/, 'approval action modal should not keep the default footer loading state')
})
