import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderScheduleView.vue', import.meta.url)
const routerURL = new URL('../src/router/index.ts', import.meta.url)
const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const apiURL = new URL('../src/api/release.ts', import.meta.url)
const typesURL = new URL('../src/types/release.ts', import.meta.url)

function read(url) {
  return readFileSync(url, 'utf8')
}

test('scheduled release page is routed from release management menu', () => {
  const router = read(routerURL)
  const layout = read(layoutURL)

  assert.match(
    router,
    /const ReleaseOrderScheduleView = \(\) => import\('\.\.\/views\/release\/ReleaseOrderScheduleView\.vue'\)/,
    'router should lazy-load the scheduled release page',
  )
  assert.match(
    router,
    /path: '\/release-schedules'[\s\S]*name: 'release-order-schedules'[\s\S]*component: ReleaseOrderScheduleView[\s\S]*meta: \{ title: '预约发布' \}/,
    'router should expose the /release-schedules page with the expected title',
  )
  assert.match(
    layout,
    /route\.path\.startsWith\('\/release-schedules'\)[\s\S]*return \['release-order-schedules'\]/,
    'layout should mark the scheduled release menu item active',
  )
  assert.match(
    layout,
    /function goToReleaseSchedules\(\) \{[\s\S]*router\.push\('\/release-schedules'\)/,
    'layout should navigate to scheduled releases from the menu',
  )
  assert.match(
    layout,
    /<a-menu-item key="release-order-schedules" @click="goToReleaseSchedules">预约发布<\/a-menu-item>/,
    'release management menu should include the scheduled release entry',
  )
})

test('scheduled release page follows release list layout chrome', () => {
  const source = read(viewURL)

  assert.match(source, /<div class="page-header-card page-header schedule-page-header">/, 'page should use the same transparent header shell as release list')
  assert.match(
    source,
    /<div class="page-header-actions schedule-header-actions">[\s\S]*class="release-toolbar-action-btn"[\s\S]*高级检索[\s\S]*class="release-toolbar-action-btn release-toolbar-action-btn--primary"[\s\S]*创建预约/,
    'header actions should match the release list button style',
  )
  assert.doesNotMatch(
    source,
    /schedule-toolbar-search|schedule-toolbar-select|schedule-toolbar-range/,
    'dense query controls should not be placed in the page header',
  )
  assert.match(
    source,
    /<a-card class="release-overview-card"[\s\S]*预约统计[\s\S]*全部预约状态分布[\s\S]*当前关注[\s\S]*最新预约/,
    'page should keep the same overview section rhythm as the release list',
  )
  assert.match(
    source,
    /<a-card class="filter-card"[\s\S]*class="filter-entry-row"[\s\S]*class="quick-filter-row"[\s\S]*状态查询[\s\S]*环境筛选[\s\S]*class="filter-advanced-panel"[\s\S]*高级条件需点击"查询"后生效/,
    'filters should use quick filters plus an advanced panel like the release list',
  )
  assert.match(
    source,
    /class="active-filter-bar"[\s\S]*当前筛选/,
    'active filters should live inside the filter card',
  )
  assert.match(
    source,
    /<a-card class="table-card"[\s\S]*<a-table[\s\S]*class="release-order-table schedule-table"[\s\S]*row-key="id"[\s\S]*:scroll="\{ x: 1820 \}"/,
    'schedule table should use the same table card shell as release list',
  )
  assert.match(source, /<div class="pagination-area">[\s\S]*show-size-changer[\s\S]*show-quick-jumper/, 'pagination should live in the standardized pagination area')

  assert.match(source, /\.table-card\s*\{[\s\S]*?background:\s*transparent/, 'table card should stay visually aligned with release list')
})

test('scheduled release create edit modal follows button popup form standard', () => {
  const source = read(viewURL)

  assert.match(
    source,
    /<a-modal[\s\S]*:width="760"[\s\S]*:closable="false"[\s\S]*:footer="null"[\s\S]*:destroy-on-close="true"[\s\S]*wrap-class-name="schedule-form-modal-wrap"/,
    'schedule form should use the standardized modal shell',
  )
  assert.match(
    source,
    /<template #title>[\s\S]*class="schedule-form-modal-titlebar"[\s\S]*class="application-toolbar-action-btn schedule-form-modal-save-btn"[\s\S]*保存/,
    'schedule form save action should live in the modal title bar',
  )
  assert.match(
    source,
    /class="schedule-form"[\s\S]*layout="vertical"[\s\S]*:required-mark="false"/,
    'schedule form should be vertical and use custom required tags',
  )
  assert.match(
    source,
    /预约模式[\s\S]*class="schedule-form-required-tag"[\s\S]*CI 预约时间[\s\S]*CD 预约时间[\s\S]*全流程开始时间[\s\S]*时区[\s\S]*备注/,
    'schedule form should include all fields from the design document',
  )
  assert.match(
    source,
    /function validateScheduleTimes\(\)[\s\S]*CD 时间必须晚于 CI 时间/,
    'form should validate build_deploy time ordering on the client',
  )
})

test('scheduled release API and types expose the design contract', () => {
  const api = read(apiURL)
  const types = read(typesURL)

  assert.match(types, /export type ReleaseOrderScheduleMode = "build" \| "deploy" \| "build_deploy" \| "execute"/)
  assert.match(types, /export type ReleaseOrderScheduleStatus =[\s\S]*"pending_approval"[\s\S]*"scheduled"[\s\S]*"dispatching"[\s\S]*"cancelled"/)
  assert.match(types, /export interface ReleaseOrderSchedule \{[\s\S]*schedule_no: string;[\s\S]*release_order_no: string;[\s\S]*cd_conflict_at: string \| null;[\s\S]*last_error: string;/)
  assert.match(api, /export async function listReleaseOrderSchedules\([\s\S]*"\/release-order-schedules"/)
  assert.match(api, /export async function createReleaseOrderSchedule\([\s\S]*`\/release-orders\/\$\{encodeURIComponent\(String\(releaseOrderID \|\| ""\)\.trim\(\)\)\}\/schedule`/)
  assert.match(api, /export async function updateReleaseOrderSchedule\([\s\S]*`\/release-order-schedules\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}`/)
  assert.match(api, /export async function cancelReleaseOrderSchedule\([\s\S]*`\/release-order-schedules\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}\/cancel`/)
  assert.match(api, /export async function approveReleaseOrderSchedule\([\s\S]*`\/release-order-schedules\/\$\{encodeURIComponent\(String\(id \|\| ""\)\.trim\(\)\)\}\/approve`/)
})
