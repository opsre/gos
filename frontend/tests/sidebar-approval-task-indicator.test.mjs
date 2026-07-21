import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const layoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const workbenchSource = readFileSync(new URL('../src/views/release/ReleaseApprovalWorkbenchView.vue', import.meta.url), 'utf8')
const detailSource = readFileSync(new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url), 'utf8')
const eventSource = readFileSync(new URL('../src/utils/release-approval-events.ts', import.meta.url), 'utf8')

test('sidebar approval dot reflects real pending approval task total', () => {
  assert.match(layoutSource, /listReleaseApprovalWorkbenchTasks\(\{ page: 1, page_size: 1 \}\)/)
  assert.match(layoutSource, /hasPendingApprovalTasks\.value = response\.total > 0/)
  assert.match(layoutSource, /v-if="hasPendingApprovalTasks" class="sidebar-approval-menu-dot"/)
})

test('sidebar approval dot is static and periodically refreshes', () => {
  assert.match(layoutSource, /window\.setInterval\([\s\S]*30_000/)
  assert.match(layoutSource, /\.sidebar-approval-menu-dot\s*\{[\s\S]*background:\s*#ef4444/)
  assert.doesNotMatch(layoutSource, /\.sidebar-approval-menu-dot\s*\{[^}]*animation:/)
})

test('successful approval actions refresh the sidebar task indicator immediately', () => {
  assert.match(eventSource, /window\.dispatchEvent\(new Event\(releaseApprovalTasksChangedEvent\)\)/)
  assert.match(layoutSource, /window\.addEventListener\(releaseApprovalTasksChangedEvent, refreshPendingApprovalIndicator\)/)
  assert.match(workbenchSource, /message\.success\([\s\S]*notifyReleaseApprovalTasksChanged\(\)/)
  assert.ok((detailSource.match(/notifyReleaseApprovalTasksChanged\(\)/g) || []).length >= 2)
})
