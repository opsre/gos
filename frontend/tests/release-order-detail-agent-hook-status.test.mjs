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

test('build-complete Agent tasks are rendered inside the CD approval card', () => {
  assert.match(
    viewSource,
    /const cdApprovalAgentSteps = computed\(\(\) =>[\s\S]*code\.startsWith\("hook:build_complete:"\)[\s\S]*isAgentHookStep\(item\)[\s\S]*!isSkippedHookStep\(item\)/,
    'only active build-complete Agent hooks should be attached to CD approval',
  )
  assert.match(
    viewSource,
    /v-if="cdApprovalAgentSteps\.length > 0"[\s\S]*CD 审核 · Agent 自动检查[\s\S]*v-for="step in cdApprovalAgentSteps"/,
    'the approval card should render the release order Agent tasks and statuses',
  )
  assert.match(
    viewSource,
    /Agent 自动检查执行中，完成后进入人工 CD 审核[\s\S]*Agent 自动检查已通过，可继续人工 CD 审核/,
    'runtime copy should preserve Agent-before-human CD approval ordering',
  )
})

test('approval summary distinguishes auto-pass and Agent-aware waiting deploy', () => {
  assert.match(
    viewSource,
    /approvalFlow\.value\.status === "completed"[\s\S]*没有需要等待的 CD 环节，待部署节点已自动放行/,
    'no-CD flows should explain that waiting deploy was bypassed',
  )
  assert.match(
    viewSource,
    /current_node_code === "waiting_deploy"[\s\S]*cdApprovalAgentSteps\.value\.length > 0[\s\S]*cdApprovalAgentSummary\.value[\s\S]*等待发起 CD 部署/,
    'waiting deploy should include Agent status when the order has Agent work',
  )
})
