import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/api/application.ts', import.meta.url), 'utf8')
const typeSource = readFileSync(new URL('../src/types/application.ts', import.meta.url), 'utf8')
const viewSource = readFileSync(
  new URL('../src/views/application/ApplicationListView.vue', import.meta.url),
  'utf8',
)

test('application workbench uses a single aggregate snapshot endpoint', () => {
  assert.match(
    apiSource,
    /getApplicationWorkbench[\s\S]*\/applications\/workbench/,
    'the client should expose the aggregate workbench endpoint',
  )
  assert.match(typeSource, /interface ApplicationWorkbenchResponse[\s\S]*template_names_by_application/)
  assert.match(typeSource, /recent_release_orders[\s\S]*release_state_summaries[\s\S]*overview/)
  assert.match(viewSource, /const response = await getApplicationWorkbench\(filters\.value\)/)
  assert.match(viewSource, /await loadWorkbench\(\{ silent: true \}\)/)
})

test('workbench refresh no longer fans out by application or environment', () => {
  assert.doesNotMatch(viewSource, /applicationIDs\.map\([\s\S]{0,500}listReleaseOrders/)
  assert.doesNotMatch(viewSource, /entries\.map\([\s\S]{0,500}getApplicationRollbackCapability/)
  assert.doesNotMatch(viewSource, /listAllReleaseTemplates\(/)
  assert.doesNotMatch(viewSource, /listAppReleaseStateSummaries\(/)
})
