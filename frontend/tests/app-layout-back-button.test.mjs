import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const source = readFileSync(layoutURL, 'utf8')
const appSource = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const ownBackButtonViews = [
  '../src/views/application/ApplicationCreateView.vue',
  '../src/views/application/ApplicationEditView.vue',
  '../src/views/application/ApplicationPipelineBindingView.vue',
  '../src/views/release/ReleaseOrderCreateView.vue',
  '../src/views/release/ReleaseOrderDetailView.vue',
]

test('authenticated top-level pages do not get a history-dependent global back button', () => {
  assert.doesNotMatch(source, /class="layout-page-back-btn"/)
  assert.doesNotMatch(source, /class="layout-page-toolbar"/)
  assert.doesNotMatch(source, /function goBack\(\)\s*\{\s*router\.back\(\)\s*\}/)
  assert.doesNotMatch(source, /ArrowLeftOutlined/)
  assert.doesNotMatch(source, /routesWithOwnBackButton|showLayoutBackButton/)
  assert.match(source, /\.layout-route-view\s*\{[\s\S]*?min-height:\s*calc\(100vh - 60px\);/)
})

test('pages with an existing header back button use route history too', () => {
  ownBackButtonViews.forEach((relativePath) => {
    const viewSource = readFileSync(new URL(relativePath, import.meta.url), 'utf8')
    assert.match(viewSource, /function goBack\(\)\s*\{\s*router\.back\(\);?\s*\}/, `${relativePath} should return through route history`)
    assert.match(viewSource, /class="application-toolbar-action-btn" @click="goBack"/, `${relativePath} should keep its header back button`)
  })
})

test('public pages except login expose the same top-right back action', () => {
  assert.match(appSource, /function goBack\(\)\s*\{\s*router\.back\(\)\s*\}/)
  assert.match(appSource, /v-if="route\.meta\.public && route\.name !== 'login'"/)
  assert.match(appSource, /\.root-page-back-btn\.ant-btn\s*\{[\s\S]*?top:\s*28px;[\s\S]*?right:\s*28px;/)
  assert.match(appSource, /\.root-page-back-btn\.ant-btn\s*\{[\s\S]*?height:\s*42px;[\s\S]*?border-radius:\s*16px;/)
})
