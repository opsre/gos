import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/component/ExecutorParamManagementView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\n\\}`))
  return match?.[1] || ''
}

test('executor param page moves keyword search into the header actions', () => {
  assert.match(
    source,
    /class="page-header-actions"/,
    'executor param page should expose a header actions area for the search trigger',
  )
  assert.match(
    source,
    /application-toolbar-icon-btn/,
    'executor param page should reuse the application page search trigger shell in the header',
  )
  assert.match(
    source,
    /SearchOutlined/,
    'executor param page should render a search icon trigger in the header',
  )
  assert.match(
    source,
    /executor-search-overlay/,
    'executor param page should render a floating search overlay instead of an inline keyword field',
  )
  assert.match(
    source,
    /executor-search-suggestions/,
    'executor param search overlay should render live suggestion results like the application page',
  )
  assert.doesNotMatch(
    source,
    /page-header-card/,
    'executor param page should remove the old title card shell and use a plain header row',
  )
})

test('executor param page removes the old application and platform key form filters', () => {
  assert.doesNotMatch(
    source,
    /<a-form-item label="应用">/,
    'executor param page should no longer keep the old application selector in the filter form',
  )
  assert.doesNotMatch(
    source,
    /<a-form-item label="平台标准 Key">/,
    'executor param page should no longer keep the old platform key input in the filter form',
  )
  assert.doesNotMatch(
    source,
    /class="filter-card"/,
    'executor param page should remove the old multi-condition filter card',
  )
})

test('executor param table becomes a cross-application result list', () => {
  assert.match(
    source,
    /title:\s*'应用'/,
    'executor param table should add an application column for cross-application search results',
  )
  assert.match(
    source,
    /keyword:/,
    'executor param page should track a unified keyword search state',
  )
  assert.match(
    source,
    /loadSearchSuggestions/,
    'executor param page should fetch live search suggestions while typing',
  )
  assert.doesNotMatch(
    source,
    /title:\s*'必填'|title:\s*'默认值'|title:\s*'参数描述'|title:\s*'排序'|title:\s*'展示'|title:\s*'可编辑'|title:\s*'来源'|title:\s*'更新时间'/,
    'executor param table should hide low-frequency metadata columns from the list view',
  )
  assert.match(
    source,
    /<a-descriptions-item label="必填">[\s\S]*<a-descriptions-item label="默认值">[\s\S]*<a-descriptions-item label="参数描述">[\s\S]*<a-descriptions-item label="展示">[\s\S]*<a-descriptions-item label="可编辑">[\s\S]*<a-descriptions-item label="来源">[\s\S]*<a-descriptions-item label="排序">[\s\S]*<a-descriptions-item label="更新时间">/,
    'executor param detail drawer should still expose the hidden metadata fields',
  )
})

test('executor param page loads the binding-type list without application or keyword filters', () => {
  assert.doesNotMatch(
    source,
    /if \(!filters\.application_id && !keyword\)/,
    'executor param page should not stop loading when only binding_type is present',
  )
  assert.match(
    source,
    /filters\.application_id && !keyword[\s\S]*\? await listApplicationExecutorParamDefs\(filters\.application_id, commonParams\)[\s\S]*: await listExecutorParamDefs\(\{[\s\S]*\.\.\.commonParams[\s\S]*keyword: keyword \|\| undefined/,
    'executor param page should load the global executor-param list when no application filter exists',
  )
  assert.doesNotMatch(
    source,
    /if \(!showStandaloneEmpty\.value\)/,
    'executor param page should load data on mount even without a selected application',
  )
  assert.doesNotMatch(
    source,
    /<a-empty v-if="showStandaloneEmpty"/,
    'executor param page should render the table empty state instead of replacing the table before loading',
  )
})

test('executor param fixed operation column uses an opaque surface', () => {
  const tableCardRule = extractStyleRule('.table-card')
  assert.match(
    tableCardRule,
    /border-radius:\s*0/,
    'executor param table shell should not round the table card',
  )
  assert.doesNotMatch(
    tableCardRule,
    /border-radius:\s*(18|20)px/,
    'executor param table shell should not keep the old rounded radius',
  )

  const tableContainerRule = extractStyleRule('.executor-param-table :deep(.ant-table-container)')
  assert.match(
    tableContainerRule,
    /border-radius:\s*0\s*!important/,
    'executor param table container should be a square table',
  )
  assert.doesNotMatch(
    tableContainerRule,
    /border-radius:\s*(18|20)px/,
    'executor param table container should not keep the old rounded radius',
  )

  const tableInnerRule = extractStyleRule(
    '.executor-param-table :deep(.ant-table),\n.executor-param-table :deep(.ant-table-content),\n.executor-param-table :deep(.ant-table-body)',
  )
  assert.match(
    tableInnerRule,
    /border-radius:\s*0\s*!important/,
    'executor param inner ant table surfaces should also reset radius',
  )

  const tableHeaderRule = extractStyleRule('.executor-param-table :deep(.ant-table-thead > tr > th)')
  assert.match(
    tableHeaderRule,
    /border-radius:\s*0\s*!important/,
    'executor param table header cells should keep square corners',
  )

  const fixedColumnRule = extractStyleRule('.executor-param-table :deep(.ant-table-cell-fix-right)')
  assert.match(
    fixedColumnRule,
    /background:\s*#fff\s*!important/,
    'fixed operation column should use a solid white background instead of translucent glass',
  )
  assert.doesNotMatch(
    fixedColumnRule,
    /background:\s*rgba\(/,
    'fixed operation column background should not be transparent',
  )

  const fixedColumnHoverRule = extractStyleRule(
    '.executor-param-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right)',
  )
  assert.match(
    fixedColumnHoverRule,
    /background:\s*#f8fafc\s*!important/,
    'fixed operation column hover state should also stay opaque',
  )
})

test('executor param secondary filters move into the header toolbar', () => {
  assert.match(
    source,
    /executor-toolbar-select/,
    'executor param page should style secondary filters as toolbar controls in the header',
  )
  assert.match(
    source,
    /executor-toolbar-query-btn/,
    'executor param page should keep a query action in the same toolbar visual language',
  )
  assert.doesNotMatch(
    source,
    /executor-toolbar-reset-btn/,
    'executor param page should remove the reset action from the header toolbar',
  )
  assert.doesNotMatch(
    source,
    /class="executor-toolbar-select"[\s\S]{0,120}allow-clear/,
    'executor param toolbar selects should not expose the clear x icon; reset handles clearing instead',
  )
})

test('executor param toolbar filters show vertically centered placeholder text', () => {
  assert.match(
    source,
    /const statusFilterValue = computed<string \| undefined>/,
    'status filter should use an undefined adapter so the placeholder can render',
  )
  assert.match(
    source,
    /const visibleFilterValue = computed<string \| undefined>/,
    'visible filter should use an undefined adapter so the placeholder can render',
  )
  assert.match(
    source,
    /const editableFilterValue = computed<string \| undefined>/,
    'editable filter should use an undefined adapter so the placeholder can render',
  )
  assert.match(
    source,
    /v-model:value="statusFilterValue"[\s\S]*placeholder="状态"/,
    'status select should show a status placeholder',
  )
  assert.match(
    source,
    /v-model:value="visibleFilterValue"[\s\S]*placeholder="展示"/,
    'visible select should show a display placeholder',
  )
  assert.match(
    source,
    /v-model:value="editableFilterValue"[\s\S]*placeholder="可编辑"/,
    'editable select should show an editable placeholder',
  )

  const selectorRule = extractStyleRule(':deep(.executor-toolbar-select.ant-select .ant-select-selector)')
  assert.match(selectorRule, /height:\s*42px/, 'toolbar select selector should keep a fixed height')
  assert.match(selectorRule, /align-items:\s*center/, 'toolbar select selector should center content vertically')

  const placeholderRule = extractStyleRule(':deep(.executor-toolbar-select.ant-select .ant-select-selection-placeholder)')
  assert.match(placeholderRule, /display:\s*flex/, 'toolbar placeholder should use flex alignment')
  assert.match(placeholderRule, /align-items:\s*center/, 'toolbar placeholder should be vertically centered')
  assert.match(placeholderRule, /height:\s*100%/, 'toolbar placeholder should fill the selector height')
  assert.match(placeholderRule, /line-height:\s*1\s*!important/, 'toolbar placeholder should avoid baseline drift')
})

test('executor param page exposes a one-click auto mapping action in the header', () => {
  assert.match(
    source,
    /ThunderboltOutlined/,
    'executor param page should use an icon for the auto mapping action',
  )
  assert.match(
    source,
    /const autoMappingSubmitting = ref\(false\)/,
    'executor param page should track auto mapping submission state',
  )
  assert.match(
    source,
    /<a-button class="executor-toolbar-query-btn" @click="handleToolbarFilterChange">查询<\/a-button>[\s\S]*class="application-toolbar-action-btn executor-auto-map-btn"[\s\S]*:loading="autoMappingSubmitting"[\s\S]*@click="handleAutoMapExecutorParams"[\s\S]*一键映射/,
    'executor param page should render the one-click mapping button immediately after the query action',
  )
  assert.match(
    source,
    /import \{ message, Modal \} from 'ant-design-vue'/,
    'executor param page should use Ant Design modal confirmation for auto mapping',
  )
  assert.match(
    source,
    /Modal\.confirm\(\{[\s\S]*title: '确认一键映射参数？'[\s\S]*content:[\s\S]*当前筛选范围[\s\S]*已有映射如果与匹配结果不一致会被覆盖[\s\S]*okText: '确认映射'[\s\S]*cancelText: '取消'[\s\S]*await runAutoMapExecutorParams\(\)/,
    'auto mapping click should show a second confirmation that explains scope and overwrite risk before running',
  )
})

test('executor param one-click mapping matches normalized standard fields with executor param names', () => {
  assert.match(
    source,
    /function normalizeAutoMappingName\(value: unknown\)/,
    'auto mapping should normalize names before comparing them',
  )
  assert.match(
    source,
    /registerAutoMappingCandidate\(paramMap, item\.param_key, item\.param_key\)/,
    'auto mapping should match by platform param key',
  )
  assert.match(
    source,
    /registerAutoMappingCandidate\(paramMap, item\.name, item\.param_key\)/,
    'auto mapping should also match by platform param display name',
  )
  assert.match(
    source,
    /normalizeAutoMappingName\(item\.executor_param_name\)/,
    'auto mapping should compare against the executor parameter name',
  )
  assert.match(
    source,
    /if \(!item\.can_edit\)/,
    'auto mapping should skip rows the current user cannot edit',
  )
  assert.match(
    source,
    /currentParamKey === targetParamKey/,
    'auto mapping should skip rows that are already bound to the matching field',
  )
  assert.match(
    source,
    /updateExecutorParamDef\(item\.id,\s*\{\s*param_key: targetParamKey,\s*\}\)/,
    'auto mapping should update matching rows through the existing mapping endpoint',
  )
})

test('executor param mapping form should use the shared modal form shell instead of default ant modal actions', () => {
  assert.match(
    source,
    /wrap-class-name="executor-mapping-modal-wrap"/,
    'executor param mapping form should provide a dedicated modal shell class',
  )
  assert.match(
    source,
    /:closable="false"/,
    'executor param mapping form should hide the default modal close icon',
  )
  assert.match(
    source,
    /:footer="null"/,
    'executor param mapping form should remove the default modal footer actions',
  )
  assert.match(
    source,
    /executor-mapping-modal-titlebar/,
    'executor param mapping form should render a custom title bar with a save action',
  )
  assert.match(
    source,
    /class="application-toolbar-action-btn executor-mapping-modal-save-btn"[\s\S]*保存/,
    'executor param mapping form should render the save action with the shared toolbar shell',
  )
  assert.match(
    source,
    /executor-mapping-form-note/,
    'executor param mapping form should explain the editable scope in a lightweight note block',
  )
  assert.match(
    source,
    /executor-mapping-form-panel/,
    'executor param mapping form should split readonly context and editable config into panels',
  )
  assert.match(
    source,
    /:required-mark="false"/,
    'executor param mapping form should disable the default red asterisk styling',
  )
  assert.doesNotMatch(
    source,
    /ok-text="保存"|cancel-text="取消"/,
    'executor param mapping form should not keep the default Ant footer button copy',
  )

  const titlebarRule = extractStyleRule('.executor-mapping-modal-titlebar')
  assert.match(titlebarRule, /justify-content:\s*space-between/, 'mapping modal titlebar should separate title and save action')
  assert.match(titlebarRule, /width:\s*100%/, 'mapping modal titlebar should give the save action the full right edge')

  const saveButtonRule = extractStyleRule(':deep(.executor-mapping-modal-save-btn.ant-btn)')
  assert.match(saveButtonRule, /height:\s*42px/, 'mapping modal save button should use the standardized modal action height')
  assert.match(saveButtonRule, /border-radius:\s*16px/, 'mapping modal save button should use the standardized modal action radius')
  assert.match(saveButtonRule, /font-size:\s*14px/, 'mapping modal save button should use the standardized modal action font size')
  assert.match(saveButtonRule, /font-weight:\s*700/, 'mapping modal save button should use the standardized modal action weight')
  assert.match(saveButtonRule, /color:\s*#0f172a !important/, 'mapping modal save button should keep the standardized dark text color')
  assert.match(saveButtonRule, /linear-gradient\(180deg,\s*rgba\(255,\s*255,\s*255,\s*0\.56\),\s*rgba\(255,\s*255,\s*255,\s*0\.24\)\)/, 'mapping modal save button should use a translucent glass gradient instead of a flat white fill')
  assert.match(saveButtonRule, /backdrop-filter:\s*blur\(18px\) saturate\(150%\)/, 'mapping modal save button should visibly use a glass blur treatment')

  const saveButtonHighlightRule = extractStyleRule(':deep(.executor-mapping-modal-save-btn.ant-btn)::before')
  assert.match(saveButtonHighlightRule, /background:\s*linear-gradient\(180deg,\s*rgba\(255,\s*255,\s*255,\s*0\.54\),\s*rgba\(255,\s*255,\s*255,\s*0\)\)/, 'mapping modal save button should include a top glass highlight')

  const saveButtonTextRule = extractStyleRule(':deep(.executor-mapping-modal-save-btn.ant-btn > span)')
  assert.match(saveButtonTextRule, /z-index:\s*1/, 'mapping modal save button text should sit above the glass highlight')
})
