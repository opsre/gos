import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const apiURL = new URL('../src/api/release.ts', import.meta.url)
const typeURL = new URL('../src/types/release.ts', import.meta.url)

const viewSource = readFileSync(viewURL, 'utf8')
const apiSource = readFileSync(apiURL, 'utf8')
const typeSource = readFileSync(typeURL, 'utf8')

test('release API exposes pipeline stage diagnosis calls', () => {
  assert.match(apiSource, /createReleaseOrderPipelineStageDiagnosis/, 'release API should create stage diagnosis')
  assert.match(apiSource, /getLatestReleaseOrderPipelineStageDiagnosis/, 'release API should fetch latest stage diagnosis')
  assert.match(apiSource, /\/diagnoses\/latest/, 'release API should use the latest diagnosis endpoint')
})

test('release types include structured stage diagnosis result', () => {
  assert.match(typeSource, /ReleaseOrderPipelineStageDiagnosis/, 'release types should include diagnosis response type')
  assert.match(typeSource, /root_causes/, 'diagnosis result should include root causes')
  assert.match(typeSource, /suggested_actions/, 'diagnosis result should include suggested actions')
  assert.match(typeSource, /related_log_lines/, 'diagnosis result should include related log lines')
})

test('Jenkins stage view renders AI diagnosis action and drawer', () => {
  assert.match(viewSource, /openStageDiagnosisDrawer/, 'release detail should have a diagnosis drawer opener')
  assert.match(viewSource, /AI 诊断/, 'release detail should show an AI diagnosis action')
  assert.match(viewSource, /stageDiagnosisDrawerVisible/, 'release detail should render the diagnosis drawer state')
  assert.match(viewSource, /可能原因/, 'diagnosis drawer should render root cause section')
  assert.match(viewSource, /建议动作/, 'diagnosis drawer should render suggested actions section')
})

test('ArgoCD stage nodes can open the stage log drawer', () => {
  assert.match(viewSource, /function canOpenStageLog/, 'release detail should centralize stage log clickability')
  assert.match(
    viewSource,
    /canOpenStageLog\(section\)[\s\S]*openStageLogDrawer\(stage\)/,
    'stage nodes should use the shared log clickability guard before opening logs',
  )
  assert.match(
    viewSource,
    /section\.isJenkins \|\| section\.isArgoCD/,
    'ArgoCD stages should be treated as log-viewable like Jenkins stages',
  )
})

test('stage diagnosis drawer uses an AI assistant conversation layout', () => {
  assert.match(viewSource, /stage-diagnosis-assistant/, 'diagnosis drawer should render an assistant shell')
  assert.match(viewSource, /stage-diagnosis-message--assistant/, 'diagnosis drawer should render assistant message bubbles')
  assert.match(viewSource, /stage-diagnosis-thinking/, 'diagnosis drawer should show analysis progress steps')
  assert.match(viewSource, /stageDiagnosisQuickPrompts/, 'diagnosis drawer should expose quick follow-up prompts')
  assert.match(viewSource, /stage-diagnosis-evidence-item/, 'diagnosis drawer should render evidence references')
})

test('stage diagnosis loading state shows an animated AI assistant', () => {
  assert.match(
    viewSource,
    /v-if="stageDiagnosisLoading"[\s\S]*stage-diagnosis-diagnosing/,
    'diagnosis loading state should render the assistant diagnosing animation',
  )
  assert.match(
    viewSource,
    /@keyframes\s+stageDiagnosisPulse/,
    'diagnosis loading state should define a pulse animation',
  )
  assert.doesNotMatch(
    viewSource,
    /stage-diagnosis-spinner/,
    'diagnosis loading state should not render the large standalone spinner',
  )
  assert.match(
    viewSource,
    /@keyframes\s+stageDiagnosisSpin/,
    'diagnosis loading state should define a spinner rotation animation',
  )
  assert.match(
    viewSource,
    /stage-diagnosis-thinking-step--active \.anticon\s*\{[\s\S]*animation:\s*stageDiagnosisSpin/,
    'active diagnosis progress icons should spin while diagnosis is running',
  )
  assert.doesNotMatch(
    viewSource,
    /<a-skeleton v-if="stageDiagnosisLoading"/,
    'diagnosis loading state should not be a generic skeleton screen',
  )
  assert.doesNotMatch(
    viewSource,
    /AI 助手正在诊断/,
    'diagnosis loading animation should not render redundant title text',
  )
  assert.doesNotMatch(
    viewSource,
    /正在读取 Jenkins 阶段日志，提取错误上下文并生成处理建议/,
    'diagnosis loading animation should not render redundant helper copy',
  )
})

test('stage diagnosis assistant omits duplicate header metadata', () => {
  assert.doesNotMatch(viewSource, /stage-diagnosis-assistant-head/, 'assistant body should not render duplicate title metadata')
  assert.doesNotMatch(viewSource, /stageDiagnosisConfidencePercent/, 'assistant body should not render confidence metadata')
  assert.doesNotMatch(viewSource, /stageDiagnosisSeverityText/, 'assistant body should not render severity metadata')
  assert.doesNotMatch(viewSource, />查看阶段日志<\/a-button>/, 'diagnosis drawer should not offer a direct stage log shortcut')
  assert.doesNotMatch(viewSource, /openStageLogFromDiagnosis/, 'diagnosis evidence should not open the stage log drawer')
  assert.doesNotMatch(
    viewSource,
    /AI 诊断 · \$\{selectedDiagnosisStage\.stage_name\}/,
    'diagnosis drawer title should not duplicate stage scope and stage name',
  )
})

test('stage diagnosis evidence is static and wraps long snippets', () => {
  assert.doesNotMatch(viewSource, /stage-diagnosis-evidence-btn/, 'evidence rows should not be clickable buttons')
  assert.match(
    viewSource,
    /stage-diagnosis-evidence-item[\s\S]*overflow-wrap:\s*anywhere/,
    'evidence rows should force long file names and log snippets to wrap',
  )
  assert.match(
    viewSource,
    /stage-diagnosis-evidence-item[\s\S]*cursor:\s*default/,
    'evidence rows should look like static evidence, not actions',
  )
})

test('stage diagnosis evidence falls back to root cause evidence', () => {
  assert.match(viewSource, /stageDiagnosisEvidenceItems/, 'diagnosis drawer should compute displayable evidence items')
  assert.match(
    viewSource,
    /root_causes[\s\S]*evidence[\s\S]*related_log_lines/,
    'diagnosis evidence should fall back to root cause evidence when related log lines are empty',
  )
})

test('stage diagnosis quick prompts dispatch follow-up requests', () => {
  assert.match(apiSource, /followUpReleaseOrderPipelineStageDiagnosis/, 'release API should expose follow-up calls')
  assert.match(apiSource, /\/follow-up/, 'release API should call the follow-up endpoint')
  assert.match(typeSource, /ReleaseOrderPipelineStageDiagnosisFollowUpResponse/, 'release types should include follow-up response type')
  assert.match(viewSource, /stageDiagnosisFollowUpMessages/, 'diagnosis drawer should render follow-up turns')
  assert.match(
    viewSource,
    /followUpReleaseOrderPipelineStageDiagnosis[\s\S]*question/,
    'quick prompt handler should send the selected prompt to the backend',
  )
})
