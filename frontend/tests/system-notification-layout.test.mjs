import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/system/SystemNotificationView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  assert.ok(match, `expected style rule for ${selector}`)
  return match[1]
}

test('notification page moves search and actions into the header toolbar', () => {
  assert.match(source, /<div class="page-header-card page-header notification-page-header">/, 'notification page should use the transparent standard header')
  assert.match(source, /<div class="page-header-actions notification-header-actions">/, 'notification page should expose a right aligned toolbar')
  assert.match(source, /class="application-toolbar-icon-btn notification-search-trigger"[\s\S]*SearchOutlined/, 'keyword search should be an icon entry in the header')
  assert.match(source, /v-if="activeTab === 'sources'"[\s\S]*class="notification-toolbar-select"/, 'source type filter should live in the header toolbar')
  assert.match(source, /v-model:value="activeEnabledFilter"[\s\S]*class="notification-toolbar-select"/, 'status filter should live in the header toolbar')
  assert.match(source, /class="notification-toolbar-query-btn"[\s\S]*查询/, 'query action should use the toolbar button shell')
  assert.match(source, /class="application-toolbar-action-btn notification-create-btn"[\s\S]*\{\{ activeCreateLabel \}\}/, 'create action should be driven by the active tab')
  assert.doesNotMatch(source, /page-subtitle|toolbar-row|filter-form|filter-input|filter-select|type="primary"[\s\S]*新增通知源|type="primary"[\s\S]*新增 Markdown 模板|type="primary"[\s\S]*新增通知 Hook/, 'old subtitle, inline filter rows and primary create buttons should be removed')

  const headerRule = extractStyleRule('.notification-page-header')
  assert.match(headerRule, /display:\s*flex/, 'header should keep title and controls on one row')
  assert.match(headerRule, /align-items:\s*flex-start/, 'header controls should align to the title top edge')
  assert.match(headerRule, /justify-content:\s*space-between/, 'header should push controls to the right')
  assert.match(headerRule, /background:\s*transparent\s*!important/, 'header should be transparent')
  assert.match(headerRule, /box-shadow:\s*none\s*!important/, 'header should not draw a card shadow')

  const actionsRule = extractStyleRule('.notification-header-actions')
  assert.match(actionsRule, /flex-wrap:\s*nowrap/, 'header controls should not wrap on desktop')
  assert.match(actionsRule, /min-width:\s*0/, 'header controls should shrink inside the title row')

  const selectRule = extractStyleRule(':deep(.notification-toolbar-select.ant-select .ant-select-selector)')
  assert.match(selectRule, /height:\s*42px\s*!important/, 'toolbar selects should align with button height')
  assert.match(selectRule, /border-radius:\s*16px\s*!important/, 'toolbar selects should use the standard radius')
  assert.match(selectRule, /align-items:\s*center/, 'toolbar selects should vertically center text')
})

test('notification page uses a floating keyword search overlay', () => {
  assert.match(source, /notification-search-overlay/, 'notification page should render a search overlay')
  assert.match(source, /notification-search-floating-panel/, 'search overlay should use the floating panel shell')
  assert.match(source, /:placeholder="activeSearchPlaceholder"/, 'search placeholder should follow the active tab')
  assert.match(source, /notification-search-suggestions/, 'search overlay should support quick suggestions')
  assert.match(source, /fetchNotificationSearchSuggestions/, 'search suggestions should query the active notification list')

  const overlayRule = extractStyleRule('.notification-search-overlay')
  assert.match(overlayRule, /left:\s*var\(--layout-sider-width,\s*220px\)/, 'search overlay should stay inside the content area')
  assert.match(overlayRule, /background:\s*rgba\(255,\s*255,\s*255,\s*0\.08\)/, 'search overlay should not create a white mask')
  assert.match(overlayRule, /backdrop-filter:\s*blur\(8px\) saturate\(112%\)/, 'search overlay should use a light glass blur')

  const floatingPanelRule = extractStyleRule('.notification-search-floating-panel')
  assert.match(floatingPanelRule, /background:\s*transparent/, 'floating panel should not draw a solid card')
  assert.match(floatingPanelRule, /box-shadow:\s*none/, 'floating panel should let the input glass carry the depth')
  assert.match(floatingPanelRule, /backdrop-filter:\s*none/, 'floating panel should avoid double glass layers')

  const floatingInputRule = extractStyleRule('.notification-search-floating-input')
  assert.match(floatingInputRule, /linear-gradient\(180deg,\s*rgba\(255,\s*255,\s*255,\s*0\.72\)/, 'search input should use transparent glass gradient')
  assert.match(floatingInputRule, /backdrop-filter:\s*blur\(18px\) saturate\(125%\)/, 'search input should carry the glass blur')

  const suggestionsRule = extractStyleRule('.notification-search-suggestions')
  assert.match(suggestionsRule, /rgba\(255,\s*255,\s*255,\s*0\.22\)/, 'suggestions should use the transparent glass layer')
  assert.match(suggestionsRule, /backdrop-filter:\s*blur\(18px\) saturate\(124%\)/, 'suggestions should use glass blur')
})

test('notification lists use the standard table shell', () => {
  assert.match(source, /<a-card :bordered="false" class="table-card notification-table-card">[\s\S]*<a-tabs/, 'notification tabs should sit in the table card shell')
  assert.equal((source.match(/class="notification-table"/g) || []).length, 3, 'all three notification lists should share the table class')
  assert.doesNotMatch(source, /showQuickJumper:\s*true/, 'notification list pagination should not keep the bulky quick jumper')

  const tableCardRule = extractStyleRule('.notification-table-card')
  assert.match(tableCardRule, /background:\s*transparent/, 'table card should not draw an extra white shell')
  assert.match(tableCardRule, /box-shadow:\s*none/, 'table card should stay flat')

  const tableHeadRule = extractStyleRule('.notification-table :deep(.ant-table-thead > tr > th)')
  assert.match(tableHeadRule, /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/, 'notification tables should use the management-table header')

  const tableContainerRule = extractStyleRule('.notification-table :deep(.ant-table-container)')
  assert.match(tableContainerRule, /border-radius:\s*0\s*!important/, 'notification tables should use square outer corners')
  assert.doesNotMatch(tableContainerRule, /border-radius:\s*18px/, 'notification tables should not keep rounded card corners')

  const firstTableHeadRule = extractStyleRule('.notification-table :deep(.ant-table-thead > tr > th:first-child)')
  assert.match(firstTableHeadRule, /border-top-left-radius:\s*0\s*!important/, 'table header should use a square left corner')

  const lastTableHeadRule = extractStyleRule('.notification-table :deep(.ant-table-thead > tr > th:last-child)')
  assert.match(lastTableHeadRule, /border-top-right-radius:\s*0\s*!important/, 'table header should use a square right corner')

  const fixedColumnRule = extractStyleRule('.notification-table :deep(.ant-table-cell-fix-right)')
  assert.match(fixedColumnRule, /background:\s*#fff\s*!important/, 'fixed action column should stay opaque')
})

test('notification editors use titlebar save instead of default modal footer', () => {
  assert.equal((source.match(/:footer="null"/g) || []).length, 3, 'all notification modals should remove the default footer')
  assert.equal((source.match(/:closable="false"/g) || []).length, 3, 'all notification modals should remove the default close icon')
  assert.equal((source.match(/:destroy-on-close="true"/g) || []).length, 3, 'all notification modals should destroy after close')
  assert.match(source, /class="source-form-modal-titlebar"[\s\S]*class="application-toolbar-action-btn source-form-modal-save-btn"[\s\S]*保存/, 'source modal save action should live in the titlebar')
  assert.match(source, /class="template-form-modal-titlebar"[\s\S]*class="application-toolbar-action-btn template-form-modal-save-btn"[\s\S]*保存/, 'template modal save action should live in the titlebar')
  assert.match(source, /class="hook-form-modal-titlebar"[\s\S]*class="application-toolbar-action-btn hook-form-modal-save-btn"[\s\S]*保存/, 'hook modal save action should live in the titlebar')
  assert.doesNotMatch(source, /@ok="submitSource"|@ok="submitTemplate"|@ok="submitHook"|confirm-loading|ok-text="保存"|cancel-text="取消"/, 'default modal ok/cancel wiring should be removed')

  for (const selector of [
    '.source-form-modal-wrap :deep(.ant-modal-content)',
    '.template-form-modal-wrap :deep(.ant-modal-content)',
    '.hook-form-modal-wrap :deep(.ant-modal-content)',
  ]) {
    const modalRule = extractStyleRule(selector)
    assert.match(modalRule, /border-radius:\s*24px/, `${selector} should use the standard large radius`)
    assert.match(modalRule, /backdrop-filter:\s*blur\(18px\) saturate\(180%\)/, `${selector} should use glass treatment`)
  }
})

test('notification template variables include release name', () => {
  assert.match(source, /value:\s*'release_name'[\s\S]*type:\s*'内置字段'/, 'release_name should be exposed as a built-in notification variable')
  assert.match(source, /title_template:\s*'\[\{env\}\] \{app_name\} \{release_name\} \{release_status_rich\}'/, 'default notification title should include release_name')
  assert.match(source, /'\- \*\*发布名称\*\*：`\{release_name\}`'/, 'default notification body should include release_name')
  assert.match(source, /release_name:\s*'省公协同后端正式发布'/, 'notification preview should provide a release_name value')
})
