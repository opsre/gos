import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/api/release.ts', import.meta.url), 'utf8')
const typeSource = readFileSync(new URL('../src/types/release.ts', import.meta.url), 'utf8')
const realtimeSource = readFileSync(
  new URL('../src/composables/useReleaseOrderRealtime.ts', import.meta.url),
  'utf8',
)
const detailSource = readFileSync(
  new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url),
  'utf8',
)

test('release realtime API follows the snapshot and event stream contract', () => {
  assert.match(
    typeSource,
    /interface ReleaseOrderRealtimeSnapshot[\s\S]*version:\s*string[\s\S]*generated_at:\s*string[\s\S]*pipeline_stage_view:\s*ReleaseOrderPipelineStageListResponse[\s\S]*concurrent_batch_progress:\s*ReleaseOrderConcurrentBatchProgress\s*\|\s*null/,
  )
  assert.match(
    apiSource,
    /getReleaseOrderRealtimeSnapshot[\s\S]*\/release-orders\/\$\{orderID\}\/realtime-snapshot/,
  )
  assert.match(
    apiSource,
    /buildReleaseOrderRealtimeEventsURL[\s\S]*\/release-orders\/\$\{orderID\}\/events/,
  )
})

test('release realtime stream authenticates by header and falls back to lightweight polling', () => {
  assert.match(
    realtimeSource,
    /headers\.Authorization\s*=\s*`Bearer \$\{token\}`/,
    'the new state stream must not expose the access token in its URL',
  )
  assert.match(realtimeSource, /response\.body\.getReader\(\)/)
  assert.match(realtimeSource, /eventName !== "snapshot"/)
  assert.match(realtimeSource, /requestSnapshot\("fallback"\)/)
  assert.match(realtimeSource, /DEFAULT_FALLBACK_INTERVAL_MS\s*=\s*5_000/)
  assert.match(realtimeSource, /lastAppliedVersion/)
  assert.match(realtimeSource, /requestGeneration !== generation/)
  assert.match(
    realtimeSource,
    /version === lastAppliedVersion\.value/,
    'SHA-256 snapshot versions should only be deduplicated',
  )
  assert.doesNotMatch(
    realtimeSource,
    /BigInt\(|localeCompare\(|isNewerVersion/,
    'SHA-256 snapshot versions are not sortable sequence numbers',
  )
  assert.match(
    realtimeSource,
    /expectedRevision !== null && expectedRevision !== appliedRevision/,
    'an HTTP fallback started before a newer SSE snapshot must not overwrite it',
  )
  assert.match(realtimeSource, /STREAM_SILENCE_TIMEOUT_MS\s*=\s*40_000/)
  assert.match(
    realtimeSource,
    /const \{ done, value \} = await reader\.read\(\);[\s\S]*onStreamActivity\(\)/,
    'every stream chunk should reset the silence watchdog',
  )
  assert.match(
    realtimeSource,
    /silenceTimedOut = true;[\s\S]*controller\.abort\(\)/,
  )
  assert.match(
    realtimeSource,
    /lastError\.value = silenceTimedOut[\s\S]*startFallbackPolling\(true\)/,
    'watchdog aborts should immediately enter lightweight fallback mode',
  )
})

test('release realtime reconnect state resets only after a heartbeat or stable connection', () => {
  assert.match(
    realtimeSource,
    /eventName === "heartbeat"[\s\S]*reconnectAttempt = 0/,
  )
  assert.match(
    realtimeSource,
    /stableConnectionTimer = window\.setTimeout\([\s\S]*reconnectAttempt = 0[\s\S]*STABLE_CONNECTION_MS/,
  )
  const connectedBlock = realtimeSource.match(
    /connected\.value = true;[\s\S]*?armSilenceWatchdog\(\);/,
  )?.[0] || ''
  assert.doesNotMatch(
    connectedBlock,
    /reconnectAttempt = 0/,
    'an HTTP 200 alone is not proof of a stable event stream',
  )
})

test('manual snapshot refresh fences the old stream and token changes reset representation', () => {
  assert.match(
    realtimeSource,
    /function refreshNow\(\)[\s\S]*closeNetwork\(\);[\s\S]*requestSnapshot\("manual"\)[\s\S]*reconcile\(\)/,
  )
  assert.match(
    realtimeSource,
    /nextAccessToken !== activeAccessToken[\s\S]*closeNetwork\(true\)/,
    'a new token may expose a different permission-filtered representation',
  )
})

test('release detail applies dynamic snapshots without full-detail interval polling', () => {
  assert.match(detailSource, /useReleaseOrderRealtime\(\{/)
  assert.match(detailSource, /onSnapshot:\s*applyRealtimeSnapshot/)
  const applySnapshotSource = detailSource.match(
    /function applyRealtimeSnapshot[\s\S]*?\n}\n\nasync function loadDetail/,
  )?.[0] || ''
  assert.match(applySnapshotSource, /snapshot\.executions/)
  assert.match(applySnapshotSource, /snapshot\.steps/)
  assert.match(applySnapshotSource, /snapshot\.pipeline_stage_view/)
  assert.match(applySnapshotSource, /snapshot\.approval_records/)
  assert.match(
    applySnapshotSource,
    /incomingOrderUpdatedAt < currentOrderUpdatedAt/,
    'an older cached stream frame must not overwrite a newer action response',
  )
  assert.match(
    applySnapshotSource,
    /!snapshot\.value_progress_visible[\s\S]*valueProgress\.value = \[\]/,
    'a permission downgrade must still clear sensitive values even on an older frame',
  )
  assert.doesNotMatch(
    detailSource,
    /setInterval\([\s\S]{0,300}loadDetail\(\{\s*silent:\s*true\s*\}\)/,
    'running details must not reload static parameter snapshots every five seconds',
  )
  assert.match(detailSource, /await loadDetail\(\)/, 'the first full detail load stays in place')
  assert.match(detailSource, /await refreshRealtimeSnapshot\(\)/)
})

test('release detail refreshes approval flow when realtime approval state changes', () => {
  const applySnapshotSource = detailSource.match(
    /function applyRealtimeSnapshot[\s\S]*?\n}\n\nasync function loadDetail/,
  )?.[0] || ''
  assert.match(
    applySnapshotSource,
    /shouldRefreshApprovalFlowFromRealtime\(previousOrder,\s*snapshot\.order\)/,
    'a realtime order update must also refresh the independently loaded approval flow',
  )
  assert.match(applySnapshotSource, /scheduleApprovalFlowRealtimeRefresh\(\)/)
  assert.match(
    detailSource,
    /function shouldRefreshApprovalFlowFromRealtime[\s\S]*pending_approval[\s\S]*current_task_id[\s\S]*currentApprovalFlowTask/,
    'an approval task created after the first detail load must be detected as incomplete state',
  )
  assert.match(
    detailSource,
    /function scheduleApprovalFlowRealtimeRefresh[\s\S]*approvalFlowRealtimeRefreshQueued[\s\S]*loadApprovalFlow\(\{\s*silent:\s*true\s*\}\)/,
    'overlapping realtime frames should be coalesced without losing the latest approval state',
  )
})

test('release precheck keeps its own serial lightweight polling cadence', () => {
  assert.match(detailSource, /PRECHECK_REFRESH_INTERVAL_MS\s*=\s*5_000/)
  const pollingSource = detailSource.match(
    /function canPollPrecheck[\s\S]*?function closeAllLogStreams/,
  )?.[0] || ''
  assert.match(pollingSource, /shouldLoadPrecheck\.value/)
  assert.match(pollingSource, /!document\.hidden/)
  assert.match(pollingSource, /!precheckQuerying\.value/)
  assert.match(pollingSource, /!cancelling\.value/)
  assert.match(pollingSource, /!executing\.value/)
  assert.match(pollingSource, /!approvalActing\.value/)
  assert.match(pollingSource, /window\.setTimeout/)
  assert.match(pollingSource, /loadPrecheck\(\{ silent: true \}\)/)
  assert.doesNotMatch(pollingSource, /loadDetail\(/)
  assert.match(detailSource, /startRealtimeUpdates\(\);\s*startPrecheckRefresh\(\);/)
  assert.match(detailSource, /stopPrecheckRefresh\(\);\s*stopRealtimeUpdates\(\);/)
})
