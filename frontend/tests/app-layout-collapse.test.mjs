import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const source = readFileSync(layoutURL, 'utf8')

test('collapsed sider clears controlled submenu open keys', () => {
  assert.match(
    source,
    /const visibleOpenMenuKeys = computed\(\(\) => \(siderCollapsed\.value \? \[\] : openMenuKeys\.value\)\)/,
    'layout should expose empty menu open keys while the sider is collapsed',
  )
  assert.match(
    source,
    /:open-keys="visibleOpenMenuKeys"/,
    'menu should bind to collapse-aware open keys instead of route open keys directly',
  )
  assert.doesNotMatch(
    source,
    /:open-keys="openMenuKeys"/,
    'collapsed sider must not keep route submenu groups open',
  )
})

test('collapsed sider hides menu internals and prevents pointer events', () => {
  assert.match(
    source,
    /:class="\{ 'app-sider-collapsed': siderCollapsed \}"/,
    'sider should expose a collapsed class for hiding internal menu content',
  )
  assert.match(
    source,
    /\.app-sider-collapsed\s+:deep\(\.ant-layout-sider-children\)/,
    'collapsed sider should style its children explicitly',
  )
  assert.match(source, /pointer-events:\s*none;/, 'collapsed sider internals should not receive hover events')
  assert.match(source, /visibility:\s*hidden;/, 'collapsed sider internals should not leave visible submenu content')
})

test('layout background stays clean instead of gray haze', () => {
  assert.match(
    source,
    /linear-gradient\(180deg,\s*#fbfdff 0%,\s*#f8fbff 46%,\s*#fbfcff 100%\)/,
    'layout should use a clean cool-white background gradient',
  )
  assert.doesNotMatch(
    source,
    /linear-gradient\(180deg,\s*#f5f7fa 0%,\s*#f1f4f8 18%,\s*#eceff4 100%\)/,
    'layout should not use the old gray haze background',
  )
  assert.match(
    source,
    /rgba\(59,\s*130,\s*246,\s*0\.055\)/,
    'decorative blue glow should stay very subtle',
  )
})

test('expanded submenu children are visually nested to the right', () => {
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline\)\s*\{[\s\S]*align-items:\s*flex-end;/,
    'inline submenu container should align children toward the right edge',
  )
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline::before\)\s*\{[\s\S]*left:\s*12px;[\s\S]*width:\s*1px;/,
    'inline submenu container should render a subtle nesting guide line',
  )
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline > \.ant-menu-item\),[\s\S]*width:\s*calc\(100% - 24px\);[\s\S]*margin-inline-start:\s*auto;/,
    'submenu entries should shrink and shift right to create a nested column effect',
  )
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline \.ant-menu-submenu-title\),[\s\S]*padding-inline-start:\s*20px\s*!important;/,
    'submenu entry content should keep a deeper left inset inside the shifted column',
  )
})
