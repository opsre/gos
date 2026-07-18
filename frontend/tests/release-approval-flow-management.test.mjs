import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewURL = new URL('../src/views/release/ApprovalFlowManagementView.vue', import.meta.url)
const source = readFileSync(viewURL, 'utf8')

test('approval flow management hides manual link configuration', () => {
  assert.doesNotMatch(source, /自定义连线|已配置分支/, 'manual link and branch panels should not be rendered')
  assert.doesNotMatch(source, /class="link-editor"|class="canvas-link-summary"/, 'manual link controls should be removed')
  assert.doesNotMatch(source, /flow-link-label/, 'execution scope labels should not clutter the canvas')
  assert.doesNotMatch(source, /<a-alert/, 'release runtime states should not appear in approval configuration')
  assert.doesNotMatch(source, /<div class="library-title">自动动作/, 'read-only automatic actions should not duplicate the canvas')
  assert.doesNotMatch(source, /class="flow-node flow-node-action canvas-action-node"/, 'automatic action placeholders should not be rendered on the canvas')
})

test('approval flow management shares the organization canvas interaction model', () => {
  assert.match(source, /class="approval-canvas"/)
  assert.match(source, /class="approval-edge-layer"/)
  assert.match(source, /@pointerdown="startNodeDrag\(\$event, node\.code\)"/)
  assert.match(source, /@wheel\.prevent="handleWheel"/)
  assert.match(source, /aria-label="适应画布"/)
  assert.match(source, /aria-label="自动排版"/)
  assert.match(source, /localStorage\.setItem\(/)
  assert.match(source, /if \(dragState\?\.kind === "node"\) persistPositions\(\)/)
  assert.match(source, /kind-waiting/)
  assert.match(source, /按 CD \/ Agent 能力动态停留/)
  assert.match(source, /无 CD 且无 Agent 任务时自动放行到结束/, 'waiting deploy should not block orders without CD and Agent work')
  assert.match(source, /存在构建完成 Agent 任务时先显示检查状态[\s\S]*存在 CD 时继续等待部署/, 'waiting deploy should describe the Agent and CD branches')
})

test('approval flow management generates standardized runtime routes', () => {
  assert.match(source, /function buildAutomaticLinks\(\): ApprovalFlowLink\[\]/, 'runtime links should be system generated')
  assert.match(source, /addPath\(\["start", \.\.\.beforeCI, "waiting_deploy"\], "build_only"\)/, 'build-only should stop at waiting deploy')
  assert.match(source, /addPath\(\["waiting_deploy", \.\.\.beforeCD, "end"\], "deploy_only"\)/, 'deploy should resume from waiting deploy')
  assert.match(source, /addPath\([\s\S]*\["start", \.\.\.beforeExecute, \.\.\.beforeCI, \.\.\.beforeCD, "end"\],[\s\S]*"full_release",[\s\S]*\)/, 'full release should bypass the waiting pause')
  assert.match(source, /links: buildAutomaticLinks\(\)/, 'saved definitions should use the generated routes')
})

test('approval nodes support organization hierarchy approvers', () => {
  assert.match(source, /<a-radio-button value="manager">组织上级<\/a-radio-button>/, 'manager approver source should be selectable')
  assert.match(source, /v-model:value="selectedNode\.manager_level"/, 'manager level should be configurable')
  assert.match(source, /以发布单发起人为起点动态解析/, 'manager hierarchy semantics should be explained')
})

test('approval nodes can target release environments', () => {
  assert.match(source, /label="适用环境"/, 'node inspector should expose applicable environments')
  assert.match(source, /v-model:value="selectedNode\.applicable_env_codes"/, 'environment selection should be stored on the node')
  assert.match(source, /placeholder="全部环境（不限制）"/, 'empty selection should clearly mean all environments')
  assert.match(source, /getReleaseSettings/, 'environment options should reuse release settings')
  assert.match(source, /applicable_env_codes: \[\.\.\.node\.applicable_env_codes\]/, 'saved nodes should include applicable environments')
  assert.match(source, /自动跳过环境不匹配的节点/, 'runtime skip behavior should be explained')
})

test('Agent tasks are folded into CD approval instead of becoming approval nodes', () => {
  assert.doesNotMatch(source, /Agent 任务节点|`agent_task:\$\{stage\.gate\}`|选择临时 Agent 任务/, 'approval flow should not create standalone Agent nodes')
  assert.doesNotMatch(source, /listAllAgentTasks|agentTaskOptions/, 'approval configuration should not load or select Agent task templates')
  assert.match(source, /v-if="selectedNode\.gate === 'before_cd'" class="cd-agent-bridge"/, 'CD approval should explain its Agent integration')
  assert.match(source, /Agent 任务随发布单自动并入/, 'CD approval should make dynamic Agent inclusion visible')
  assert.match(source, /构建完成[”"]阶段的 Agent 任务[\s\S]*成功后进入人工审批[\s\S]*失败则按 Hook 策略阻断发布/, 'CD approval should define Agent-before-human ordering')
  assert.match(source, /node_type: "approval"/, 'saved approval nodes should remain human approval nodes')
})
