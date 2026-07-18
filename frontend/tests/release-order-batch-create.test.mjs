import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const createViewSource = readFileSync(
  new URL('../src/views/release/ReleaseOrderCreateView.vue', import.meta.url),
  'utf8',
)
const listViewSource = readFileSync(
  new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url),
  'utf8',
)
const apiSource = readFileSync(
  new URL('../src/api/release.ts', import.meta.url),
  'utf8',
)

test('release list exposes an independent batch-create entry', () => {
  assert.match(listViewSource, /function toBatchCreate\(\)/)
  assert.match(listViewSource, /query:\s*Record<string, string>\s*=\s*\{ batch: "1" \}/)
  assert.match(listViewSource, /@click="toBatchCreate"[\s\S]*批量创建/)
})

test('batch-create mode snapshots validated single-order payloads into a bounded list', () => {
  assert.match(createViewSource, /const isBatchMode = computed/)
  assert.match(createViewSource, /async function buildReleasePayload\(\)/)
  assert.match(createViewSource, /async function handleAddBatchDraft\(\)/)
  assert.match(createViewSource, /if \(batchDrafts\.value\.length >= 50\)/)
  assert.match(createViewSource, /payload:\s*cloneReleasePayload\(payload\)/)
  assert.match(createViewSource, /载入修改/)
})

test('only the create-list card can submit a populated batch', () => {
  const headerButton = createViewSource.match(
    /<a-button\s+v-if="isBatchMode"\s+class="application-toolbar-action-btn release-build-toolbar-btn"([\s\S]*?)<\/a-button>/,
  )
  assert.ok(headerButton, 'expected the header batch summary button')
  assert.match(headerButton[1], /\sdisabled\s/)
  assert.doesNotMatch(headerButton[1], /@click="handleBatchCreate"/)
  assert.match(
    createViewSource,
    /class="create-side-card batch-draft-card"[\s\S]*@click="handleBatchCreate"[\s\S]*创建全部/,
    'the create-list card should keep the only active batch submit action',
  )
})

test('batch-create submission only creates orders and preserves per-item failures', () => {
  const handler = createViewSource.match(
    /async function handleBatchCreate\(\) \{([\s\S]*?)\n\}\n\nasync function handleSubmit/,
  )
  assert.ok(handler, 'expected batch-create handler')
  assert.match(handler[1], /batchCreateReleaseOrders\(/)
  assert.match(handler[1], /orders:\s*submittedDrafts\.map/)
  assert.match(handler[1], /response\.data\.failures/)
  assert.doesNotMatch(handler[1], /buildReleaseOrder|executeReleaseOrder|batchExecuteReleaseOrders/)
  assert.match(apiSource, /"\/release-orders\/batch-create"/)
})
