import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseTemplateView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

function extractStyleRule(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\}`))
  return match?.[1] || ''
}

test('release template page moves filters into the header toolbar', () => {
  assert.match(source, /class="page-header-actions release-template-header-actions"/, 'release template filters should live in header actions')
  assert.match(source, /class="release-template-toolbar-select release-template-toolbar-select-wide"/, 'application filter should use toolbar select styling')
  assert.match(source, /class="release-template-toolbar-select"/, 'status filter should use toolbar select styling')
  assert.match(source, /class="release-template-toolbar-query-btn"[\s\S]*>\s*查询\s*<\/a-button>/, 'query should be a header toolbar action')
  assert.match(source, /class="application-toolbar-action-btn release-template-create-btn"/, 'create action should reuse the toolbar button shell')
  assert.doesNotMatch(source, /type="primary"[\s\S]*新增发布模板/, 'create action should not use the blue primary button style')
  assert.doesNotMatch(source, /<a-card class="filter-card"/, 'release template page should not keep the old filter card')
  assert.doesNotMatch(source, /release-page-guide-collapse/, 'release template page should not show the old guide card in the main list')

  const headerRule = extractStyleRule('.release-template-page-header')
  assert.match(headerRule, /background:\s*transparent\s*!important/, 'release template header should remove the card background')
  assert.match(headerRule, /box-shadow:\s*none\s*!important/, 'release template header should remove the card shadow')
  assert.match(headerRule, /border:\s*none\s*!important/, 'release template header should remove the card border')
  assert.match(source, /background:\s*rgba\(255,\s*255,\s*255,\s*0\.62\)\s*!important/, 'toolbar actions should force the non-blue glass button background')
})

test('release template filters default to empty placeholders', () => {
  assert.match(source, /const applicationFilterValue = computed<string \| undefined>/, 'application select should use an undefined value adapter for placeholder display')
  assert.match(source, /const statusFilterValue = computed<ReleaseTemplateStatus \| undefined>/, 'status select should use an undefined value adapter for placeholder display')
  assert.doesNotMatch(source, /\{ label: '全部应用', value: '' \}/, 'application options should not inject an all-applications selected value')
  assert.doesNotMatch(source, /\{ label: '全部状态', value: '' \}/, 'status options should not inject an all-status selected value')
  assert.match(source, /v-model:value="applicationFilterValue"[\s\S]*placeholder="应用"/, 'application select should show an empty placeholder by default')
  assert.match(source, /v-model:value="statusFilterValue"[\s\S]*placeholder="状态"/, 'status select should show an empty placeholder by default')
  assert.doesNotMatch(source, /syncFiltersFromRoute/, 'release template list should not sync query parameters into default filter values')
})

test('release template list applies application query parameters when linked from application cards', () => {
  assert.match(source, /import \{ useRoute \} from 'vue-router'/, 'release template page should read URL query parameters')
  assert.match(source, /const route = useRoute\(\)/, 'release template page should keep access to the current route')
  assert.match(source, /function applyTemplateFiltersFromRouteQuery\(\)/, 'route query handling should be isolated from default placeholder adapters')
  assert.match(source, /route\.query\.application_id/, 'release template page should read application_id from the URL query')
  assert.match(source, /filters\.application_id = String\(route\.query\.application_id \|\| ''\)\.trim\(\)/, 'application query should be copied into the list filter before loading')
  assert.match(source, /applyTemplateFiltersFromRouteQuery\(\)[\s\S]*await loadTemplates\(\)/, 'query filters should be applied before the first template list request')
})

test('release template toolbar placeholder text is vertically centered and muted', () => {
  const selectorRule = extractStyleRule(':deep(.release-template-toolbar-select .ant-select-selector)')
  assert.match(selectorRule, /height:\s*42px/, 'select shell should have a fixed toolbar height for vertical centering')
  assert.match(selectorRule, /align-items:\s*center/, 'select shell should center content vertically')

  const placeholderRule = extractStyleRule(':deep(.release-template-toolbar-select .ant-select-selection-placeholder)')
  assert.match(placeholderRule, /display:\s*flex/, 'select placeholder should use flex alignment')
  assert.match(placeholderRule, /align-items:\s*center/, 'select placeholder should be vertically centered')
  assert.match(placeholderRule, /height:\s*100%/, 'select placeholder should fill the selector height')
  assert.match(placeholderRule, /color:\s*rgba\(30,\s*58,\s*138,\s*0\.38\)\s*!important/, 'select placeholder text should use a muted blue tone')

  const searchInputRule = extractStyleRule(':deep(.release-template-toolbar-select .ant-select-selection-search-input)')
  assert.match(searchInputRule, /height:\s*100%\s*!important/, 'searchable select input should fill the selector height')
  assert.match(searchInputRule, /line-height:\s*42px\s*!important/, 'searchable select input text should align vertically with the toolbar height')
})

test('release template table uses the current management table theme without an outer white card', () => {
  assert.doesNotMatch(source, /<a-card class="table-card"/, 'release template table should not be wrapped in a white card')
  assert.match(source, /class="release-template-table-section"/, 'release template table should use a structural section wrapper')
  assert.match(source, /class="release-template-table"/, 'release template table should expose a dedicated styling hook')
  assert.doesNotMatch(source, /dataIndex: 'updated_at'/, 'low-frequency update time should not stay in the primary template list')
  assert.doesNotMatch(source, /dataIndex: 'param_count'/, 'low-frequency parameter count should not stay in the primary template list')

  const tableHeadRule = extractStyleRule('.release-template-table :deep(.ant-table-thead > tr > th)')
  assert.match(tableHeadRule, /linear-gradient\(180deg,\s*#243247,\s*#1f2a3d\)/, 'table header should use the dark management-table gradient')

  const fixedColumnRule = extractStyleRule('.release-template-table :deep(.ant-table-cell-fix-right)')
  assert.match(fixedColumnRule, /background:\s*rgba\(255,\s*255,\s*255,\s*0\.96\)/, 'fixed operation column should keep an opaque background')
})

test('release template execution unit column uses segmented icon pills instead of plain text', () => {
  assert.match(source, /import \{[\s\S]*DeploymentUnitOutlined[\s\S]*RocketOutlined[\s\S]*\} from '@ant-design\/icons-vue'/, 'execution unit column should import CI/CD icons')
  assert.match(source, /function templateExecutionUnits\(record: ReleaseTemplate\)/, 'execution unit column should normalize scope summaries into explicit units')
  assert.match(source, /<template v-else-if="column\.key === 'binding_name'">[\s\S]*class="template-binding-pill"[\s\S]*v-for="unit in templateExecutionUnits\(record\)"/, 'execution unit column should render a segmented pill for each enabled unit')
  assert.match(source, /<DeploymentUnitOutlined v-if="unit\.key === 'ci'" \/>[\s\S]*<RocketOutlined v-else-if="unit\.key === 'cd'" \/>/, 'execution unit pill should show dedicated icons for CI and CD')
  assert.doesNotMatch(source, /<template v-else-if="column\.key === 'binding_name'">[\s\S]*\{\{\s*record\.binding_name\s*\}\}/, 'execution unit column should not fall back to plain summary text')

  const pillRule = extractStyleRule('.template-binding-pill')
  assert.match(pillRule, /display:\s*inline-flex/, 'segmented execution unit pill should lay out units in one row')
  assert.match(pillRule, /border-radius:\s*999px/, 'segmented execution unit pill should keep a capsule silhouette')
  assert.match(pillRule, /overflow:\s*hidden/, 'segmented execution unit pill should clip segment corners cleanly')

  const ciRule = extractStyleRule('.template-binding-pill-segment--ci')
  assert.match(ciRule, /linear-gradient\(180deg,\s*#eff6ff 0%,\s*#dbeafe 100%\)/, 'CI segment should use the blue pipeline gradient')

  const cdRule = extractStyleRule('.template-binding-pill-segment--cd')
  assert.match(cdRule, /linear-gradient\(180deg,\s*#fff7ed 0%,\s*#ffedd5 100%\)/, 'CD segment should use the warm deployment gradient')
})

test('release template editor hides required stars while keeping required validation', () => {
  assert.match(source, /<a-form ref="formRef" :model="formState" layout="vertical" :required-mark="false">/, 'template editor should hide Ant required markers')
  assert.match(source, /label="模板名称" name="name" :rules="\[\{ required: true, message: '请输入模板名称' \}\]"/, 'template name should stay required')
  assert.match(source, /label="状态" name="status" :rules="\[\{ required: true, message: '请选择状态' \}\]"/, 'status should stay required')
  assert.match(source, /label="应用" name="application_id" :rules="\[\{ required: true, message: '请选择应用' \}\]"/, 'application should stay required')
})

test('release template clone mode keeps the application selector editable and reuses create-style payload editing', () => {
  assert.match(source, /openCloneModal\(record: ReleaseTemplate\)[\s\S]*openEditModal\(record, 'clone'\)/, 'clone action should still open the editor in clone mode')
  assert.match(source, /formState\.id = mode === 'clone' \? '' : template\.id/, 'clone mode should clear the source template id before submit')
  assert.match(source, /formState\.name = mode === 'clone' \? buildCloneName\(template\.name\) : template\.name/, 'clone mode should prefill a cloned template name')
  assert.match(source, /formState\.application_id = template\.application_id/, 'clone mode should prefill the source application before the user changes it')
  assert.match(source, /v-model:value="formState\.application_id"[\s\S]*:disabled="modalMode === 'edit'"/, 'application selector should stay editable in clone mode and only lock during true edit mode')
  assert.doesNotMatch(source, /v-model:value="formState\.application_id"[\s\S]*:disabled="modalMode !== 'create'"/, 'clone mode should not inherit the old create-only application lock')
  assert.match(source, /\.\.\.\(modalMode\.value !== 'edit' \? \{ application_id: formState\.application_id\.trim\(\) \} : \{\}\)/, 'clone submit payload should still send the edited application id like create mode')
  assert.match(source, /async function handleApplicationChange\(value: string \| undefined\)[\s\S]*loadBindings\(formState\.application_id\)[\s\S]*isHelmGitOps\(\) \? loadGitOpsValuesCandidates\(formState\.application_id,\s*true\) : loadGitOpsFieldCandidates\(formState\.application_id,\s*true\)/, 'changing the application in clone mode should reload the matching gitops candidate source so all cloned sections remain editable')
})

test('release template editor moves modal actions into a glass title bar and blurs the mask', () => {
  assert.match(source, /const templateEditorViewportInset = ref\(0\)/, 'template editor should track the sidebar width for content-area overlays')
  assert.match(source, /const templateEditorMaskStyle = computed\(\(\) => \(\{[\s\S]*background: 'rgba\(15,\s*23,\s*42,\s*0\.08\)'[\s\S]*backdropFilter: 'blur\(10px\)'/, 'template editor should use the same light blurred mask as pipeline binding')
  assert.match(source, /:mask-style="templateEditorMaskStyle"/, 'template editor should apply the blurred mask style')
  assert.match(source, /:wrap-props="templateEditorWrapProps"/, 'template editor should offset the modal wrap so the menu is not covered')
  assert.match(source, /left:\s*`\$\{templateEditorViewportInset\.value\}px`/, 'template editor mask should start after the live sider width')
  assert.match(source, /width:\s*`calc\(100% - \$\{templateEditorViewportInset\.value\}px\)`/, 'template editor mask and wrap should only cover the main content area')
  assert.match(source, /pointerEvents:\s*modalVisible\.value\s*\?\s*'auto'\s*:\s*'none'/, 'template editor overlay should release pointer events after close')
  assert.match(source, /templateEditorViewportObserver/, 'template editor should observe layout changes so the sider offset stays current')
  assert.match(source, /:closable="false"/, 'template editor should not show the default close icon beside the title button')
  assert.match(source, /:footer="null"/, 'template editor should remove the default bottom modal footer')
  assert.match(source, /:destroy-on-close="true"/, 'template editor should destroy modal content after close')
  assert.match(source, /:after-close="handleTemplateEditorAfterClose"/, 'template editor should clean up state after the close transition')
  assert.doesNotMatch(source, /ok-text="保存"[\s\S]*cancel-text="取消"[\s\S]*@ok="submitForm"/, 'template editor should not rely on default footer actions')
  assert.match(source, /<template #title>[\s\S]*class="template-editor-modal-titlebar"[\s\S]*{{ modalTitle }}[\s\S]*class="application-toolbar-action-btn template-editor-modal-save-btn"[\s\S]*@click="submitForm"[\s\S]*保存[\s\S]*<\/template>/, 'template editor should render the save action in the top-right title area')
  assert.doesNotMatch(source, /template-editor-modal-action-btn[\s\S]*取消/, 'template editor should not keep a second cancel button in the title bar')

  const titlebarRule = extractStyleRule('.template-editor-modal-titlebar')
  assert.match(titlebarRule, /justify-content:\s*space-between/, 'modal titlebar should place actions on the right')

  const saveButtonRule = extractStyleRule('.template-editor-modal-save-btn.ant-btn')
  assert.match(saveButtonRule, /flex:\s*none/, 'modal save action should only add layout constraints on top of the shared glass button')
  assert.match(saveButtonRule, /font-size:\s*14px/, 'modal save action should reset inherited title typography')
  assert.match(saveButtonRule, /font-weight:\s*700/, 'modal save action should keep the documented 700 weight instead of downgrading the shared button shell')

  const buttonRule = extractStyleRule(':deep(.application-toolbar-action-btn.ant-btn),\n:deep(.release-template-toolbar-query-btn.ant-btn)')
  assert.match(buttonRule, /color:\s*#0f172a\s*!important/, 'release template shared action buttons should use the documented dark text color')

  const buttonHoverRule = extractStyleRule(':deep(.application-toolbar-action-btn.ant-btn:hover),\n:deep(.application-toolbar-action-btn.ant-btn:focus),\n:deep(.application-toolbar-action-btn.ant-btn:focus-visible),\n:deep(.release-template-toolbar-query-btn.ant-btn:hover),\n:deep(.release-template-toolbar-query-btn.ant-btn:focus),\n:deep(.release-template-toolbar-query-btn.ant-btn:focus-visible)')
  assert.match(buttonHoverRule, /color:\s*#0f172a\s*!important/, 'release template shared action buttons should keep the same dark text color on hover and focus')

  const modalContentRule = extractStyleRule('.template-editor-modal-wrap :deep(.ant-modal-content)')
  assert.match(modalContentRule, /border:\s*1px solid rgba\(255,\s*255,\s*255,\s*0\.68\)/, 'template editor should use the documented glass shell border')
  assert.match(modalContentRule, /backdrop-filter:\s*blur\(18px\) saturate\(180%\)/, 'template editor should use the documented modal glass blur')
})

test('release template editor can reorder existing hooks', () => {
  assert.match(source, /function moveHook\(index: number, direction: -1 \| 1\)/, 'hook editor should expose a reorder function')
  assert.match(source, /templateHooks\.value = next/, 'hook reorder should replace the hook array so ordering is reactive')
  assert.match(source, /@click="moveHook\(index, -1\)"[\s\S]*上移/, 'hook cards should offer a move-up action')
  assert.match(source, /@click="moveHook\(index, 1\)"[\s\S]*下移/, 'hook cards should offer a move-down action')
  assert.match(source, /:disabled="index === 0"/, 'first hook should not move up')
  assert.match(source, /:disabled="index === templateHooks\.length - 1"/, 'last hook should not move down')
  assert.match(source, /class="hook-template-order-actions"/, 'reorder actions should have a dedicated styling hook')
})

test('release template agent hook selector only lists reusable manual temporary tasks', () => {
  assert.match(source, /function isAgentTaskTemplateCandidate\(item: AgentTask\)[\s\S]*item\.task_mode === 'temporary'[\s\S]*!String\(item\.source_task_id \|\| ''\)\.trim\(\)/, 'agent task hook selector should define reusable task candidates as manual temporary tasks')
  assert.match(source, /agentTaskTemplates\.value = \(response\.data \|\| \[\]\)\.filter\(isAgentTaskTemplateCandidate\)/, 'agent task templates loaded for release hooks should drop dispatched history tasks')
  assert.match(source, /const agentTaskTemplateOptions = computed<SelectOption\[\]>\(\(\) => \{[\s\S]*const temps = agentTaskTemplates\.value\.filter\(isAgentTaskTemplateCandidate\)/, 'agent task hook options should keep the same reusable-task guard')
  assert.doesNotMatch(source, /const temps = agentTaskTemplates\.value\.filter\(\(item\) => item\.task_mode === 'temporary'\)/, 'agent task hook options should not include all temporary tasks because that includes history children')
})

test('release template param mapping controls avoid segmented overlap in narrow cards', () => {
  assert.match(source, /class="template-param-config-grid"/, 'param mapping rows should use a dedicated wrapping layout')
  assert.match(source, /class="template-param-source-col"[\s\S]*:span="24"/, 'value source segmented control should take a full row')
  assert.match(source, /class="template-param-value-col"[\s\S]*:span="24"/, 'dependent value field should take a full row below the segmented control')
  assert.doesNotMatch(source, /<a-col :span="10">\s*<a-form-item label="取值方式"/, 'segmented control should not remain in the old narrow 10-column layout')
  assert.doesNotMatch(source, /:span="14"[\s\S]*label="发布基础字段"/, 'dependent value field should not remain in the old side-by-side 14-column layout')

  const gridRule = extractStyleRule('.template-param-config-grid')
  assert.match(gridRule, /row-gap:\s*12px/, 'param mapping grid should create vertical spacing between wrapped rows')

  const segmentedGroupRule = extractStyleRule('.template-param-config-item :deep(.ant-segmented-group)')
  assert.match(segmentedGroupRule, /flex-wrap:\s*wrap/, 'segmented buttons should wrap instead of overflowing into the next control')
})

test('release template builtin sources include release name', () => {
  assert.match(source, /const builtinTemplateSourceKeys = new Set\(\[[\s\S]*'release_name'[\s\S]*\]\)/, 'release_name should be selectable as a template built-in source')
})

test('release template surfaces pipeline compliance violations without blocking edit', () => {
  assert.match(
    source,
    /import \{ getPipelineScanResult, listPipelineScanRules \} from '..\/..\/api\/pipeline-scan'/,
    'template editor should be able to preview selected pipeline scan violations',
  )
  assert.match(
    source,
    /function isTemplateViolated\(record: ReleaseTemplate\)/,
    'template list should identify violated templates from backend compliance state',
  )
  assert.match(
    source,
    /v-if="isTemplateViolated\(record\)"[\s\S]*管线违规/,
    'template list should show a pipeline violation tag',
  )
  assert.match(
    source,
    /templateComplianceWarning\(record\)/,
    'template list should render the violation summary returned by the backend',
  )
  assert.match(
    source,
    /scopeComplianceWarnings = reactive<Record<ReleasePipelineScope, string>>/,
    'template editor should track scope-level violation warnings',
  )
  assert.match(
    source,
    /async function refreshScopeComplianceWarning\(scope: ReleasePipelineScope\)/,
    'template editor should refresh compliance warnings when pipeline bindings change',
  )
  assert.match(
    source,
    /await refreshScopeComplianceWarning\(scope\)/,
    'binding changes should trigger compliance preview without blocking save',
  )
  assert.match(
    source,
    /v-if="scopeComplianceWarnings\.ci"[\s\S]*:message="scopeComplianceWarnings\.ci"/,
    'CI card should show compliance warning details',
  )
  assert.match(
    source,
    /v-if="isCDUsingPipeline\(\) && scopeComplianceWarnings\.cd"[\s\S]*:message="scopeComplianceWarnings\.cd"/,
    'CD pipeline card should show compliance warning details',
  )
})

test('release template locks executor params mapped to builtin standard fields', () => {
  assert.match(
    source,
    /const builtinPlatformParamKeySet = computed\(\(\) =>/,
    'release template should derive builtin platform keys from the standard dictionary',
  )
  assert.match(
    source,
    /function isBuiltinMappedExecutorParam\(item\?: ExecutorParamDef \| null\)/,
    'release template should detect executor params already mapped to builtin fields',
  )
  assert.match(
    source,
    /function enforceBuiltinMappedTemplateParamConfig\(config: TemplateParamConfigState, item\?: ExecutorParamDef \| null\)/,
    'release template should force builtin mapped params to builtin source before display and submit',
  )
  assert.match(
    source,
    /config\.value_source = 'builtin'[\s\S]*config\.source_param_key = paramKey[\s\S]*config\.fixed_value = ''/,
    'builtin mapped params should be saved as builtin source with their platform key',
  )
  assert.match(
    source,
    /v-if="isBuiltinMappedExecutorParam\(item\)"[\s\S]*已映射内置字段[\s\S]*发布链路自动赋值[\s\S]*v-else[\s\S]*<a-segmented/,
    'builtin mapped rows should render a readonly auto-fill message instead of the value-source segmented control',
  )
  assert.match(
    source,
    /handleTemplateParamValueSourceChange\([\s\S]*if \(isBuiltinMappedExecutorParam\(paramDef\)\)/,
    'manual value source changes should be ignored for builtin mapped params',
  )
})
