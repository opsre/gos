import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewSource = readFileSync(new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url), 'utf8')
const detailSource = readFileSync(new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url), 'utf8')
const createSource = readFileSync(new URL('../src/views/release/ReleaseOrderCreateView.vue', import.meta.url), 'utf8')
const typeSource = readFileSync(new URL('../src/types/release.ts', import.meta.url), 'utf8')

test('release order expansion loads the frozen approval flow before choosing legacy or no-flow rendering', () => {
  assert.match(
    viewSource,
    /getReleaseOrderApprovalFlow[\s\S]*async function loadExpandedApprovalFlow[\s\S]*response = await getReleaseOrderApprovalFlow\(orderID\)/,
    'expanded rows should fetch the release-order approval flow instance',
  )
  assert.match(
    viewSource,
    /@expand="handleApprovalFlowExpand"/,
    'approval flow data should load when a row expands',
  )
  assert.match(
    viewSource,
    /function approvalFlowTrackNodes\(record: ReleaseOrder\)[\s\S]*\(flow\.nodes \|\| \[\]\)[\s\S]*latestApprovalTask\(flow, node\.code\)/,
    'the track should render the frozen graph nodes and their runtime tasks',
  )
})

test('bound approval flows keep configured nodes visible when the current environment will skip them', () => {
  assert.match(
    viewSource,
    /function approvalFlowNodeMatchesEnvironment[\s\S]*function approvalFlowTrackNodes\(record: ReleaseOrder\)[\s\S]*const configuredNodes = \(flow\.nodes \|\| \[\]\)[\s\S]*\.map\(\(node\) =>/,
    'configured snapshot nodes should be mapped without being filtered out by env code',
  )
  assert.match(
    viewSource,
    /仅适用于 \$\{environments\}，当前 \$\{envCode \|\| "环境"\} 执行时自动跳过/,
    'nodes outside the current environment should explain that execution will skip them',
  )
})

test('release order expansion restores the original approval status card and five-column track style', () => {
  assert.match(
    viewSource,
    /class="approval-flow-kicker">审批流程<[\s\S]*class="approval-flow-title">\{\{ record\.order_no \}\}/,
    'the expanded header should use the original approval-flow title treatment',
  )
  assert.match(
    viewSource,
    /\.approval-flow-track\s*\{[\s\S]*display:\s*grid[\s\S]*grid-template-columns:\s*repeat\(5, minmax\(0, 1fr\)\)/,
    'the approval status nodes should use the original five-column grid',
  )
  assert.match(
    viewSource,
    /function expandedApprovalFlowNodes\(record: ReleaseOrder\)[\s\S]*approvalFlowTrackNodes\(record\)[\s\S]*legacyApprovalFlowNodes\(record\)[\s\S]*noApprovalFlowNodes\(record\)/,
    'real, legacy, and unbound orders should share the original card renderer',
  )
})

test('approval flow nodes only spin while work is actually executing', () => {
  assert.match(
    viewSource,
    /interface ApprovalFlowTrackNode[\s\S]*spinning\?: boolean/,
    'track nodes should model animation independently from their active color',
  )
  assert.match(
    viewSource,
    /spinning: task\?\.status === "running"/,
    'approval nodes should spin only for running tasks, not pending human approval',
  )
  assert.match(
    viewSource,
    /spinning: status === "building" \|\| status === "deploying"/,
    'execution nodes should spin during actual build or deploy execution',
  )
  assert.match(
    viewSource,
    /approvalFlowIcon\(node\)[\s\S]*:spin="node\.spinning === true"/,
    'the renderer should use the explicit execution animation flag',
  )
  assert.doesNotMatch(
    viewSource,
    /:spin="node\.tone === 'active'"/,
    'waiting nodes must not spin merely because they use the active tone',
  )
})

test('release order approval flow contract includes the immutable node and link snapshot', () => {
  assert.match(
    typeSource,
    /export interface ReleaseOrderApprovalFlow\s*\{[\s\S]*nodes:\s*ApprovalFlowNode\[\][\s\S]*links:\s*ApprovalFlowLink\[\]/,
    'frontend approval flow type should expose its frozen graph snapshot',
  )
})

test('approval task comments are exposed and rendered in list progress nodes and detail records', () => {
  assert.match(
    typeSource,
    /export interface ReleaseOrderApprovalFlowTaskRecord[\s\S]*comment:\s*string[\s\S]*export interface ReleaseOrderApprovalFlowTask[\s\S]*records:\s*ReleaseOrderApprovalFlowTaskRecord\[\]/,
    'the approval-flow contract should expose persisted task records',
  )
  assert.match(
    viewSource,
    /function approvalTaskCaption[\s\S]*const recordSummary = approvalTaskRecordSummary\(task\)[\s\S]*function approvalTaskRecordSummary[\s\S]*task\.records[\s\S]*审批备注（\$\{operator\}/,
    'expanded release rows should show approval comments with operator and time in their approval node',
  )
  assert.match(
    detailSource,
    /const customFlowRecords[\s\S]*approvalFlow\.value\?\.tasks[\s\S]*task\.records[\s\S]*comment:\s*record\.comment[\s\S]*displayApprovalRecords/,
    'the release detail approval record list should merge custom-flow task comments',
  )
  assert.match(
    detailSource,
    /v-if="record\.comment" class="approval-record-comment"[\s\S]*\{\{ record\.comment \}\}/,
    'the detail record card should render the merged comment text',
  )
})

test('dispatch actions treat approval startup as success on list, detail, and create flows', () => {
  assert.match(
    viewSource,
    /response = await executeReleaseOrder[\s\S]*nextStatus === "pending_approval" \|\| nextStatus === "approving"[\s\S]*审批流程已发起，审批通过后可继续执行发布/,
    'list dispatch should show approval-started success instead of a raw pending-task error',
  )
  assert.match(
    detailSource,
    /approvalStarted = currentBusinessStatus\.value === "pending_approval"[\s\S]*审批流程已发起，审批通过后可继续执行发布/,
    'detail dispatch should use the same approval-started success message',
  )
  assert.match(
    createSource,
    /const buildResponse = await buildReleaseOrder[\s\S]*pending_approval[\s\S]*发布单创建成功，审批流程已发起/,
    'create-and-build should distinguish approval startup from executor dispatch',
  )
})
