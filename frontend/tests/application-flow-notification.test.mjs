import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewSource = readFileSync(new URL('../src/views/application/ApplicationListView.vue', import.meta.url), 'utf8')
const authSource = readFileSync(new URL('../src/stores/auth.ts', import.meta.url), 'utf8')

test('application notification popover separates system announcements and workflow notifications', () => {
  assert.match(viewSource, /type NotificationTabKey = 'announcement' \| 'workflow'/)
  assert.match(viewSource, />\s*系统公告\s*[\s\S]*>\s*流程通知\s*/)
  assert.match(viewSource, /notificationTab === 'announcement'/)
  assert.match(viewSource, /notificationTab === 'workflow'/)
})

test('workflow notifications aggregate approval tasks and handled release lifecycle updates', () => {
  assert.match(viewSource, /listReleaseApprovalWorkbenchTasks\(\{ page: 1, page_size: 30 \}\)/)
  assert.match(viewSource, /listReleaseApprovalWorkbenchRecords\(\{ page: 1, page_size: 30 \}\)/)
  assert.match(viewSource, /id: `pending:\$\{item\.source\}:\$\{item\.task_id \|\| item\.release_order_id\}:\$\{item\.release_order_status\}`/)
  assert.match(viewSource, /id: `handled:\$\{item\.source\}:\$\{item\.id \|\| item\.task_id \|\| item\.release_order_id\}:\$\{item\.release_order_status\}`/)
  assert.match(viewSource, /title: `待你审批/)
  assert.match(viewSource, /item\.action === 'approve' \? '你已通过' : '你已拒绝'/)
})

test('workflow notification unread state contributes to the application red badge and opens release detail', () => {
  assert.match(viewSource, /const notificationUnreadCount = computed\(\(\) => unreadCount\.value \+ workflowUnreadCount\.value\)/)
  assert.match(viewSource, /seenWorkflowNotificationStorageKey = 'gos-seen-workflow-notification-ids'/)
  assert.match(viewSource, /if \(unreadCount\.value === 0 && workflowUnreadCount\.value > 0\)[\s\S]*notificationTab\.value = 'workflow'/)
  assert.match(viewSource, /router\.push\(`\/releases\/\$\{item\.releaseOrderID\}`\)/)
  assert.match(authSource, /localStorage\.removeItem\('gos-seen-workflow-notification-ids'\)/)
})

test('application red badge breathes only while workflow notifications are unread', () => {
  assert.match(viewSource, /'announcement-badge-workflow-alert': workflowUnreadCount > 0/)
  assert.match(viewSource, /\.announcement-badge-workflow-alert\s*\{[\s\S]*animation: workflow-notification-breathe/)
  assert.match(viewSource, /@keyframes workflow-notification-breathe/)
  assert.match(viewSource, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.announcement-badge-workflow-alert/)
})
