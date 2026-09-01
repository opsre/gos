import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const listSource = readFileSync(
  new URL('../src/views/release/ReleaseOrderListView.vue', import.meta.url),
  'utf8',
)
const detailSource = readFileSync(
  new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url),
  'utf8',
)

test('replay orders can continue a completed build from the release list', () => {
  assert.match(
    listSource,
    /function supportsDeployDispatch[\s\S]*\["deploy", "replay"\]\.includes[\s\S]*has_ci_execution[\s\S]*has_cd_execution/,
    'manual deploy dispatch should accept both normal and replay orders with CI and CD',
  )
  assert.match(
    listSource,
    /function canDeploy[\s\S]*supportsDeployDispatch\(record\)[\s\S]*built_waiting_deploy/,
    'the list deploy action should use the replay-aware dispatch capability',
  )
})

test('replay orders can continue a completed build from release details', () => {
  assert.match(
    detailSource,
    /const supportsDeployDispatch = computed[\s\S]*\["deploy", "replay"\]\.includes[\s\S]*pipeline_scope === "ci"[\s\S]*pipeline_scope === "cd"/,
    'detail deploy dispatch should accept both normal and replay orders with CI and CD',
  )
  assert.match(
    detailSource,
    /const canDeploy = computed[\s\S]*supportsDeployDispatch\.value[\s\S]*built_waiting_deploy/,
    'the detail deploy action should use the replay-aware dispatch capability',
  )
})

test('replay orders do not gain the build-only action', () => {
  assert.match(
    listSource,
    /function supportsStagedDispatch[\s\S]*operation_type[\s\S]*=== "deploy"[\s\S]*function canBuild[\s\S]*supportsStagedDispatch\(record\)/,
    'the list build-only action should remain limited to normal deploy orders',
  )
  assert.match(
    detailSource,
    /const supportsStagedDispatch = computed[\s\S]*operation_type[\s\S]*=== "deploy"[\s\S]*const canBuild = computed[\s\S]*supportsStagedDispatch\.value/,
    'the detail build-only action should remain limited to normal deploy orders',
  )
})
