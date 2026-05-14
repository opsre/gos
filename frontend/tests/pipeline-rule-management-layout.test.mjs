import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/component/PipelineRuleManagementView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')
const typeURL = new URL('../src/types/pipeline-scan.ts', import.meta.url)
const typeSource = readFileSync(typeURL, 'utf8')

test('pipeline rule empty-check switch persists an explicit boolean false', () => {
  assert.match(
    source,
    /<a-switch[\s\S]*v-model:checked="form\.command_format_required"[\s\S]*:checked-value="true"[\s\S]*:un-checked-value="false"/,
    'empty-check switch should emit real booleans so closing it saves require_block=false',
  )
  assert.match(
    source,
    /require_block:\s*requireBlock === true/,
    'command format DSL should only enable require_block for the real boolean true value',
  )
})

test('pipeline rule editor only treats strict true require_block as enabled', () => {
  assert.match(
    source,
    /return parsed\?\.matcher\?\.format\?\.require_block === true/,
    'editor should not turn string-like false values into enabled empty-check state',
  )
})

test('pipeline rule editor keeps backend generated instance codes stable', () => {
  assert.match(
    source,
    /rule_code:\s*''/,
    'editor form should keep the backend generated rule code for display and update stability',
  )
  assert.match(
    source,
    /const displayedRuleCode = computed\(\(\) => \{[\s\S]*editorMode\.value === 'edit'[\s\S]*form\.rule_code[\s\S]*generatedRuleCode\.value[\s\S]*\}\)/,
    'editor should display the existing generated instance code in edit mode',
  )
  assert.match(
    source,
    /if \(record\.rule_type\) \{[\s\S]*applyRuleTypeParts\(record\.rule_type(?: \|\| '')?\)[\s\S]*\} else if \(record\.rule_code\) \{/,
    'editor should prefer backend rule_type instead of parsing a suffixed generated rule_code',
  )
})

test('pipeline rule create modal clears previous content but edit modal keeps selected record data', () => {
  assert.match(
    source,
    /function resetForm\(\) \{[\s\S]*form\.rule_name = ''[\s\S]*form\.command_format_text = ''[\s\S]*form\.message = ''[\s\S]*form\.suggestion = ''/,
    'creating a pipeline rule should clear previous text content instead of keeping old defaults',
  )
  assert.match(
    source,
    /function openCreateModal\(\) \{\s*resetEditorFormState\(\)[\s\S]*editorMode\.value = 'create'/,
    'create should use the reset path before opening the modal',
  )
  assert.doesNotMatch(
    source,
    /function openEditModal\(record: PipelineScanRule\) \{\s*resetEditorFormState\(\)/,
    'edit should not use the create reset path because it should show the selected record data',
  )
  assert.match(
    source,
    /form\.rule_dsl_json = record\.rule_dsl_json \|\| ''[\s\S]*form\.command_format_text = extractCommandFormatSource\(record\.rule_dsl_json \|\| ''\)[\s\S]*form\.message = record\.message \|\| ''[\s\S]*form\.suggestion = record\.suggestion \|\| ''/,
    'edit should explicitly assign optional record fields so no old values leak through',
  )
})

test('pipeline rule editor configures release template validation scopes', () => {
  assert.match(
    source,
    /template_validation_scopes:\s*\[\] as PipelineScanTemplateValidationScope\[\]/,
    'rule form should keep an explicit multi-select model for template validation scopes',
  )
  assert.match(
    source,
    /const templateValidationScopeOptions = \[[\s\S]*label: 'CI', value: 'ci'[\s\S]*label: 'CD', value: 'cd'/,
    'rule editor should expose CI and CD as release template validation choices',
  )
  assert.match(
    source,
    /label="发布模板校验"[\s\S]*v-model:value="form\.template_validation_scopes"[\s\S]*mode="multiple"/,
    'rule editor should render the validation scope as a multi-select control',
  )
  assert.match(
    source,
    /form\.template_validation_scopes = \[\.\.\.\(record\.template_validation_scopes \|\| \[\]\)\]/,
    'editing an existing rule should restore its template validation scopes',
  )
  assert.match(
    source,
    /template_validation_scopes:\s*\[\.\.\.form\.template_validation_scopes\]/,
    'rule submit payload should send template validation scopes to the backend',
  )
})

test('pipeline rule editor locks template profile to standard and supports pipeline parameter checks', () => {
  assert.match(
    source,
    /const ruleProfileOptions = \[\{ label: '标准', value: 'standard' \}\]/,
    'current version should only expose the standard rule profile',
  )
  assert.match(
    source,
    /v-model:value="form\.rule_profile"[\s\S]*:disabled="true"/,
    'rule profile select should be locked because profile templates are not implemented yet',
  )
  assert.match(
    source,
    /const ruleCheckOptions = \[[\s\S]*完整命令排版[\s\S]*管线参数/,
    'rule check options should include the new pipeline parameter check type',
  )
  assert.match(
    source,
    /v-if="form\.rule_check === 'pipeline_params'"[\s\S]*v-model:value="form\.pipeline_parameter_input"[\s\S]*@press-enter="addPipelineParameter"[\s\S]*v-for="item in form\.pipeline_parameters"/,
    'pipeline parameter rules should use a manual input plus removable tags instead of a dropdown select',
  )
  assert.doesNotMatch(
    source,
    /v-if="form\.rule_check === 'pipeline_params'"[\s\S]*<a-select[\s\S]*mode="tags"/,
    'pipeline parameter rules should not render a tags select because it still looks like a dropdown choice',
  )
  assert.match(
    source,
    /form\.pipeline_parameters = \[\]/,
    'creating a pipeline parameter rule should start empty so users explicitly add parameters',
  )
  assert.doesNotMatch(
    source,
    /defaultOSSPipelineParameters|默认加入对象存储内置字段|form\.pipeline_parameters = \[\.\.\.defaultOSSPipelineParameters\]/,
    'the editor should not auto-fill OSS parameters into every new parameter rule',
  )
  assert.match(
    source,
    /function buildPipelineParametersDSL\(values: string\[\]\)/,
    'pipeline parameter rules should build a dedicated DSL matcher',
  )
  assert.match(
    source,
    /type:\s*'pipeline_parameters'[\s\S]*required_parameters:\s*requiredParameters/,
    'pipeline parameter DSL should persist required parameter names',
  )
})

test('pipeline rule editor supports GOS artifact URL output checks', () => {
  assert.match(
    `${source}\n${typeSource}`,
    /artifact_gos_artifact_url_standard/,
    'type definitions should include the GOS artifact URL standard rule type',
  )
  assert.match(
    source,
    /label: 'GOS'[\s\S]*value: 'gos'/,
    'rule target options should expose GOS',
  )
  assert.match(
    source,
    /label: '制品地址输出'[\s\S]*value: 'artifact_url'/,
    'rule check options should expose artifact URL output',
  )
  assert.match(
    source,
    /function buildGOSArtifactURLDSL\(\)[\s\S]*GOS_ARTIFACT_URL\\\\s\*=|function buildGOSArtifactURLDSL\(\)[\s\S]*GOS_ARTIFACT_URL\\s\*=/,
    'GOS artifact URL rules should build a regex DSL requiring GOS_ARTIFACT_URL=',
  )
  assert.match(
    source,
    /form\.rule_check === 'artifact_url'[\s\S]*buildGOSArtifactURLDSL\(\)/,
    'submitting an artifact URL rule should use the GOS artifact URL DSL builder',
  )
})
