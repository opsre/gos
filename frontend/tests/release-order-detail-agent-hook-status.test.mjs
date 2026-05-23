import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ReleaseOrderDetailView.vue', import.meta.url)
const viewSource = readFileSync(viewURL, 'utf8')

test('release detail blocks success display until Agent hook reaches terminal success', () => {
  assert.match(
    viewSource,
    /type AgentHookRuntimeStatus\s*=\s*\|?\s*"none"\s*\|\s*"pending"\s*\|\s*"running"\s*\|\s*"success"\s*\|\s*"failed"/,
    'detail page should model Agent hook runtime status separately from the main order status',
  )
  assert.match(
    viewSource,
    /const agentHookRuntimeStatus = computed<AgentHookRuntimeStatus>\(\(\) => \{[\s\S]*isAgentHookStep\(item\)[\s\S]*item\.status === "failed"[\s\S]*item\.status === "running"[\s\S]*item\.status === "pending"[\s\S]*return "success"/,
    'detail page should derive Agent hook status from Agent hook steps',
  )
  assert.match(
    viewSource,
    /function resolveBusinessStatusWithAgentHook[\s\S]*status !== "deploy_success"[\s\S]*agentHookRuntimeStatus\.value[\s\S]*case "failed":[\s\S]*return "deploy_failed"[\s\S]*case "pending":[\s\S]*case "running":[\s\S]*return "deploying"[\s\S]*return status/,
    'deploy success should be gated by Agent hook success and mapped to running or failed while needed',
  )
  assert.match(
    viewSource,
    /order\.value\.business_status[\s\S]*return resolveBusinessStatusWithAgentHook\(order\.value\.business_status\)/,
    'backend business status should still be resolved through Agent hook runtime status',
  )
  assert.match(
    viewSource,
    /case "deploy_success":[\s\S]*case "success":[\s\S]*return resolveBusinessStatusWithAgentHook\("deploy_success"\)/,
    'fallback success status should still wait for Agent hook success',
  )
})

test('release detail keeps polling while Agent hook is unfinished after deploy success', () => {
  assert.match(
    viewSource,
    /const shouldAutoRefresh = computed\(\(\) => \{[\s\S]*agentHookRuntimeStatus\.value === "running"[\s\S]*agentHookRuntimeStatus\.value === "pending"[\s\S]*return \[[\s\S]*\]\.includes\(currentBusinessStatus\.value\)/,
    'detail page should keep auto refresh active until Agent hook leaves pending/running',
  )
})
