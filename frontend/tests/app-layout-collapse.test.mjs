import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const source = readFileSync(layoutURL, 'utf8')

test('sider footer displays the current release version', () => {
  assert.match(source, /<span>v1\.3<\/span>/, 'sider footer should display v1.3')
  assert.doesNotMatch(source, /v1\.2\.3/, 'sider footer should not display the previous version')
})

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

test('sider toggle controls stay visually prominent', () => {
  assert.match(source, /\.sider-footer-toggle,\s*\.layout-sider-restore\s*\{[\s\S]*border:\s*1px solid rgba\(96,\s*165,\s*250,\s*0\.34\);/, 'toggle controls should use a visible border')
  assert.match(source, /\.sider-footer-toggle\s*\{[\s\S]*width:\s*26px;[\s\S]*height:\s*26px;/, 'footer collapse control should be large enough to notice')
  assert.match(source, /\.layout-sider-restore\s*\{[\s\S]*width:\s*30px;[\s\S]*height:\s*64px;/, 'restore control should use a tall visible hit target')
  assert.match(source, /\.sider-footer-toggle::before\s*\{[\s\S]*content:\s*'<';/, 'footer collapse control should render a clear left arrow')
  assert.match(source, /\.layout-sider-restore::before\s*\{[\s\S]*content:\s*'>';/, 'restore control should render a clear right arrow')
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
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline::before\)\s*\{[\s\S]*left:\s*14px;[\s\S]*width:\s*2px;[\s\S]*rgba\(34,\s*211,\s*238,\s*0\.8\)/,
    'inline submenu container should render a subtle nesting guide line',
  )
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline > \.ant-menu-item\),[\s\S]*width:\s*calc\(100% - 34px\);[\s\S]*margin-inline-start:\s*auto;/,
    'submenu entries should shrink and shift right to create a nested column effect',
  )
  assert.match(
    source,
    /\.sider-menu\s+:deep\(\.ant-menu-sub\.ant-menu-inline \.ant-menu-submenu-title\),[\s\S]*padding-inline-start:\s*22px\s*!important;/,
    'submenu entry content should keep a deeper left inset inside the shifted column',
  )
})
