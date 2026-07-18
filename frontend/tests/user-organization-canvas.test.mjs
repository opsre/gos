import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const canvasSource = readFileSync(new URL('../src/components/system/UserOrganizationCanvas.vue', import.meta.url), 'utf8')
const userViewSource = readFileSync(new URL('../src/views/system/SystemUserView.vue', import.meta.url), 'utf8')
const userAPISource = readFileSync(new URL('../src/api/user.ts', import.meta.url), 'utf8')

test('user management exposes organization canvas and keeps list mode', () => {
  assert.match(userViewSource, /value="organization"[^>]*>.*组织架构/)
  assert.match(userViewSource, /value="list"[^>]*>.*用户列表/)
  assert.match(userViewSource, /<UserOrganizationCanvas v-if="viewMode === 'organization'"/)
})

test('organization canvas uses one aggregate request and supports stable canvas controls', () => {
  assert.match(userAPISource, /http\.get<UserOrganizationResponse>\('\/users\/organization'\)/)
  assert.match(canvasSource, /listUserOrganization\(\)/)
  assert.match(canvasSource, /@pointerdown="startNodeDrag\(\$event, node\.id\)"/)
  assert.match(canvasSource, /@wheel\.prevent="handleWheel"/)
  assert.match(canvasSource, /class="organization-edge-layer"/)
  assert.match(canvasSource, /z-index: 0;/)
})

test('organization relationship changes are explicit and protected from cycles', () => {
  assert.match(canvasSource, /collectDescendants\(selectedNode\.value\.id\)/)
  assert.match(canvasSource, /保存上下级关系/)
  assert.match(canvasSource, /updateUserManager\(selectedNode\.value\.id, selectedManagerID\.value \|\| ''\)/)
  assert.match(canvasSource, /拖动节点调整布局，关系修改后自动重新排版/)
})

test('administrator stays outside the business organization hierarchy', () => {
  assert.match(canvasSource, /filter\(\(node\) => node\.role !== 'admin'\)/)
  assert.match(userViewSource, /item\.id !== editingID\.value && item\.role !== 'admin'/)
  assert.match(userViewSource, /v-if="formState\.role !== 'admin'" name="manager_user_id"/)
  assert.match(userViewSource, /formState\.role === 'admin' \? '' : formState\.manager_user_id/)
})
