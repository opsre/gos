<script setup lang="ts">
import {
  AimOutlined,
  ApartmentOutlined,
  ReloadOutlined,
  SaveOutlined,
  SearchOutlined,
  TeamOutlined,
  UserOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { listUserOrganization, updateUserManager } from '../../api/user'
import type { UserOrganizationNode } from '../../types/user'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const NODE_WIDTH = 212
const NODE_HEIGHT = 82
const HORIZONTAL_GAP = 44
const VERTICAL_GAP = 72
const LAYOUT_PADDING = 84
const ROOT_ROW_MAX_WIDTH = 880
const LAYOUT_STORAGE_KEY = 'gos-user-organization-layout-v2'

interface Point {
  x: number
  y: number
}

interface CanvasViewport {
  x: number
  y: number
  scale: number
}

interface DragState {
  kind: 'node' | 'canvas'
  nodeID?: string
  startClientX: number
  startClientY: number
  startX: number
  startY: number
}

const loading = ref(false)
const saving = ref(false)
const canvasRef = ref<HTMLElement | null>(null)
const nodes = ref<UserOrganizationNode[]>([])
const positions = ref<Record<string, Point>>({})
const selectedNodeID = ref('')
const selectedManagerID = ref<string | undefined>()
const keyword = ref('')
const viewport = reactive<CanvasViewport>({ x: 40, y: 32, scale: 1 })
const worldSize = reactive({ width: 1600, height: 900 })
let dragState: DragState | null = null

const nodeByID = computed(() => new Map(nodes.value.map((node) => [node.id, node])))
const selectedNode = computed(() => nodeByID.value.get(selectedNodeID.value) || null)
const normalizedKeyword = computed(() => keyword.value.trim().toLowerCase())
const matchedNodeIDs = computed(() => {
  if (!normalizedKeyword.value) return new Set(nodes.value.map((node) => node.id))
  return new Set(
    nodes.value
      .filter((node) => `${node.display_name} ${node.username}`.toLowerCase().includes(normalizedKeyword.value))
      .map((node) => node.id),
  )
})

const relationEdges = computed(() =>
  nodes.value
    .filter((node) => node.manager_user_id && nodeByID.value.has(node.manager_user_id))
    .map((node) => {
      const managerPosition = positions.value[node.manager_user_id]
      const nodePosition = positions.value[node.id]
      if (!managerPosition || !nodePosition) return null
      const startX = managerPosition.x + NODE_WIDTH / 2
      const startY = managerPosition.y + NODE_HEIGHT
      const endX = nodePosition.x + NODE_WIDTH / 2
      const endY = nodePosition.y
      const middleY = startY + Math.max((endY - startY) / 2, 24)
      return {
        key: `${node.manager_user_id}-${node.id}`,
        path: `M ${startX} ${startY} C ${startX} ${middleY}, ${endX} ${middleY}, ${endX} ${endY}`,
      }
    })
    .filter((edge): edge is { key: string; path: string } => Boolean(edge)),
)

const managerOptions = computed(() => {
  const blocked = new Set<string>()
  if (selectedNode.value) {
    blocked.add(selectedNode.value.id)
    collectDescendants(selectedNode.value.id).forEach((id) => blocked.add(id))
  }
  return nodes.value
    .filter((node) => !blocked.has(node.id))
    .map((node) => ({
      label: `${node.display_name || node.username} (${node.username})${node.status === 'inactive' ? ' · 已停用' : ''}`,
      value: node.id,
      disabled: node.status === 'inactive',
    }))
})

watch(selectedNode, (node) => {
  selectedManagerID.value = node?.manager_user_id || undefined
})

function collectDescendants(userID: string) {
  const result = new Set<string>()
  const queue = nodes.value.filter((node) => node.manager_user_id === userID).map((node) => node.id)
  while (queue.length) {
    const current = queue.shift()!
    if (result.has(current)) continue
    result.add(current)
    nodes.value.filter((node) => node.manager_user_id === current).forEach((node) => queue.push(node.id))
  }
  return result
}

function directReportCount(userID: string) {
  return nodes.value.filter((node) => node.manager_user_id === userID).length
}

function managerName(node: UserOrganizationNode) {
  if (!node.manager_user_id) return '组织根节点'
  const manager = nodeByID.value.get(node.manager_user_id)
  return manager ? `直属主管：${manager.display_name || manager.username}` : '直属主管未找到'
}

function readSavedPositions() {
  try {
    const value = JSON.parse(localStorage.getItem(LAYOUT_STORAGE_KEY) || '{}') as Record<string, Point>
    return value && typeof value === 'object' ? value : {}
  } catch {
    return {}
  }
}

function persistPositions() {
  try {
    localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(positions.value))
  } catch {
    // 浏览器禁用本地存储时仍保留当前会话内布局。
  }
}

function calculateAutoLayout(items: UserOrganizationNode[]) {
  const itemMap = new Map(items.map((item) => [item.id, item]))
  const children = new Map<string, UserOrganizationNode[]>()
  items.forEach((item) => {
    if (!item.manager_user_id || !itemMap.has(item.manager_user_id) || item.manager_user_id === item.id) return
    const list = children.get(item.manager_user_id) || []
    list.push(item)
    children.set(item.manager_user_id, list)
  })
  children.forEach((list) => list.sort((a, b) => (a.display_name || a.username).localeCompare(b.display_name || b.username, 'zh-CN')))

  const roots = items
    .filter((item) => !item.manager_user_id || !itemMap.has(item.manager_user_id) || item.manager_user_id === item.id)
    .sort((a, b) => (a.display_name || a.username).localeCompare(b.display_name || b.username, 'zh-CN'))
  const widths = new Map<string, number>()
  const heights = new Map<string, number>()
  const measuring = new Set<string>()
  const measuringHeight = new Set<string>()
  const subtreeWidth = (id: string): number => {
    if (widths.has(id)) return widths.get(id)!
    if (measuring.has(id)) return NODE_WIDTH
    measuring.add(id)
    const list = children.get(id) || []
    const width = list.length
      ? Math.max(
          NODE_WIDTH,
          list.reduce((sum, child) => sum + subtreeWidth(child.id), 0) + HORIZONTAL_GAP * (list.length - 1),
        )
      : NODE_WIDTH
    measuring.delete(id)
    widths.set(id, width)
    return width
  }

  const subtreeHeight = (id: string): number => {
    if (heights.has(id)) return heights.get(id)!
    if (measuringHeight.has(id)) return NODE_HEIGHT
    measuringHeight.add(id)
    const list = children.get(id) || []
    const height = list.length
      ? NODE_HEIGHT + VERTICAL_GAP + Math.max(...list.map((child) => subtreeHeight(child.id)))
      : NODE_HEIGHT
    measuringHeight.delete(id)
    heights.set(id, height)
    return height
  }

  const result: Record<string, Point> = {}
  const placed = new Set<string>()
  const placeNode = (node: UserOrganizationNode, left: number, depth: number, top: number) => {
    if (placed.has(node.id)) return
    placed.add(node.id)
    const width = subtreeWidth(node.id)
    result[node.id] = {
      x: left + width / 2 - NODE_WIDTH / 2,
      y: top + depth * (NODE_HEIGHT + VERTICAL_GAP),
    }
    let childLeft = left
    for (const child of children.get(node.id) || []) {
      placeNode(child, childLeft, depth + 1, top)
      childLeft += subtreeWidth(child.id) + HORIZONTAL_GAP
    }
  }

  let rootLeft = LAYOUT_PADDING
  let rootTop = LAYOUT_PADDING
  let rowHeight = 0
  const layoutRoots = [...roots, ...items.filter((item) => !roots.some((root) => root.id === item.id))]
  layoutRoots.forEach((root) => {
    if (placed.has(root.id)) return
    const width = subtreeWidth(root.id)
    const height = subtreeHeight(root.id)
    if (rootLeft > LAYOUT_PADDING && rootLeft + width > LAYOUT_PADDING + ROOT_ROW_MAX_WIDTH) {
      rootLeft = LAYOUT_PADDING
      rootTop += rowHeight + VERTICAL_GAP
      rowHeight = 0
    }
    placeNode(root, rootLeft, 0, rootTop)
    rootLeft += width + HORIZONTAL_GAP
    rowHeight = Math.max(rowHeight, height)
  })
  return result
}

function refreshWorldSize() {
  const values = Object.values(positions.value)
  worldSize.width = Math.max(1600, ...values.map((point) => point.x + NODE_WIDTH + LAYOUT_PADDING))
  worldSize.height = Math.max(900, ...values.map((point) => point.y + NODE_HEIGHT + LAYOUT_PADDING))
}

function applyLayout(useSavedPositions: boolean) {
  const automatic = calculateAutoLayout(nodes.value)
  const saved = useSavedPositions ? readSavedPositions() : {}
  positions.value = Object.fromEntries(
    nodes.value.map((node) => [node.id, saved[node.id] || automatic[node.id] || { x: LAYOUT_PADDING, y: LAYOUT_PADDING }]),
  )
  refreshWorldSize()
  persistPositions()
}

async function fitView() {
  await nextTick()
  const container = canvasRef.value
  const values = Object.values(positions.value)
  if (!container || values.length === 0) return
  const minX = Math.min(...values.map((point) => point.x))
  const minY = Math.min(...values.map((point) => point.y))
  const maxX = Math.max(...values.map((point) => point.x + NODE_WIDTH))
  const maxY = Math.max(...values.map((point) => point.y + NODE_HEIGHT))
  const boundsWidth = Math.max(maxX - minX, NODE_WIDTH)
  const boundsHeight = Math.max(maxY - minY, NODE_HEIGHT)
  const scale = Math.min(1.12, Math.max(0.35, Math.min((container.clientWidth - 120) / boundsWidth, (container.clientHeight - 120) / boundsHeight)))
  viewport.scale = scale
  viewport.x = (container.clientWidth - boundsWidth * scale) / 2 - minX * scale
  viewport.y = (container.clientHeight - boundsHeight * scale) / 2 - minY * scale
}

async function loadOrganization(options: { resetLayout?: boolean; fit?: boolean } = {}) {
  loading.value = true
  try {
    const response = await listUserOrganization()
    nodes.value = (response.data || []).filter((node) => node.role !== 'admin')
    if (selectedNodeID.value && !nodes.value.some((node) => node.id === selectedNodeID.value)) {
      selectedNodeID.value = ''
    }
    applyLayout(!options.resetLayout)
    if (options.fit !== false) await fitView()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '组织架构加载失败'))
  } finally {
    loading.value = false
  }
}

async function resetLayout() {
  try {
    localStorage.removeItem(LAYOUT_STORAGE_KEY)
  } catch {
    // 忽略本地存储异常。
  }
  applyLayout(false)
  await fitView()
}

function selectNode(nodeID: string) {
  selectedNodeID.value = nodeID
}

function nodeStyle(nodeID: string) {
  const point = positions.value[nodeID] || { x: 0, y: 0 }
  return { transform: `translate3d(${point.x}px, ${point.y}px, 0)` }
}

function worldStyle() {
  return {
    width: `${worldSize.width}px`,
    height: `${worldSize.height}px`,
    transform: `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.scale})`,
  }
}

function zoomBy(factor: number, centerX?: number, centerY?: number) {
  const container = canvasRef.value
  if (!container) return
  const nextScale = Math.min(1.8, Math.max(0.3, viewport.scale * factor))
  const screenX = centerX ?? container.clientWidth / 2
  const screenY = centerY ?? container.clientHeight / 2
  const worldX = (screenX - viewport.x) / viewport.scale
  const worldY = (screenY - viewport.y) / viewport.scale
  viewport.x = screenX - worldX * nextScale
  viewport.y = screenY - worldY * nextScale
  viewport.scale = nextScale
}

function handleWheel(event: WheelEvent) {
  const rect = canvasRef.value?.getBoundingClientRect()
  if (!rect) return
  zoomBy(event.deltaY > 0 ? 0.9 : 1.1, event.clientX - rect.left, event.clientY - rect.top)
}

function startNodeDrag(event: PointerEvent, nodeID: string) {
  if (event.button !== 0) return
  event.preventDefault()
  event.stopPropagation()
  selectNode(nodeID)
  const point = positions.value[nodeID]
  if (!point) return
  dragState = {
    kind: 'node',
    nodeID,
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: point.x,
    startY: point.y,
  }
  beginPointerTracking()
}

function startCanvasPan(event: PointerEvent) {
  if (event.button !== 0) return
  dragState = {
    kind: 'canvas',
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: viewport.x,
    startY: viewport.y,
  }
  beginPointerTracking()
}

function beginPointerTracking() {
  document.body.style.userSelect = 'none'
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', stopPointerTracking, { once: true })
}

function handlePointerMove(event: PointerEvent) {
  if (!dragState) return
  const deltaX = event.clientX - dragState.startClientX
  const deltaY = event.clientY - dragState.startClientY
  if (dragState.kind === 'canvas') {
    viewport.x = dragState.startX + deltaX
    viewport.y = dragState.startY + deltaY
    return
  }
  if (!dragState.nodeID) return
  positions.value = {
    ...positions.value,
    [dragState.nodeID]: {
      x: dragState.startX + deltaX / viewport.scale,
      y: dragState.startY + deltaY / viewport.scale,
    },
  }
  refreshWorldSize()
}

function stopPointerTracking() {
  if (dragState?.kind === 'node') persistPositions()
  dragState = null
  document.body.style.userSelect = ''
  window.removeEventListener('pointermove', handlePointerMove)
}

async function saveManager() {
  if (!selectedNode.value) return
  saving.value = true
  try {
    await updateUserManager(selectedNode.value.id, selectedManagerID.value || '')
    message.success('直属主管已更新')
    await loadOrganization({ resetLayout: true, fit: true })
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '直属主管更新失败'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadOrganization({ fit: true })
})

onBeforeUnmount(() => {
  stopPointerTracking()
})

defineExpose({
  reload: () => loadOrganization({ fit: false }),
})
</script>

<template>
  <section class="organization-shell">
    <header class="organization-toolbar">
      <div class="organization-search">
        <SearchOutlined />
        <input v-model="keyword" type="search" placeholder="搜索姓名或用户名" aria-label="搜索组织成员" />
      </div>
      <div class="organization-toolbar-hint"><ApartmentOutlined />拖动节点调整布局，关系修改后自动重新排版</div>
      <div class="organization-toolbar-actions">
        <a-button type="text" aria-label="缩小" @click="zoomBy(0.9)"><ZoomOutOutlined /></a-button>
        <span class="organization-zoom-value">{{ Math.round(viewport.scale * 100) }}%</span>
        <a-button type="text" aria-label="放大" @click="zoomBy(1.1)"><ZoomInOutlined /></a-button>
        <a-button type="text" aria-label="适应画布" @click="fitView"><AimOutlined /></a-button>
        <a-button type="text" aria-label="自动排版" @click="resetLayout"><TeamOutlined /></a-button>
        <a-button type="text" aria-label="刷新组织架构" :loading="loading" @click="loadOrganization({ fit: false })">
          <ReloadOutlined />
        </a-button>
      </div>
    </header>

    <div class="organization-content">
      <div
        ref="canvasRef"
        class="organization-canvas"
        :class="{ 'is-loading': loading }"
        @pointerdown="startCanvasPan"
        @wheel.prevent="handleWheel"
      >
        <div class="organization-world" :style="worldStyle()">
          <svg class="organization-edge-layer" :width="worldSize.width" :height="worldSize.height" aria-hidden="true">
            <path v-for="edge in relationEdges" :key="edge.key" :d="edge.path" />
          </svg>

          <article
            v-for="node in nodes"
            :key="node.id"
            class="organization-node"
            :class="{
              selected: selectedNodeID === node.id,
              muted: normalizedKeyword && !matchedNodeIDs.has(node.id),
              inactive: node.status === 'inactive',
              root: !node.manager_user_id,
            }"
            :style="nodeStyle(node.id)"
            @pointerdown="startNodeDrag($event, node.id)"
            @click.stop="selectNode(node.id)"
          >
            <div class="organization-avatar">{{ (node.display_name || node.username).slice(0, 1).toUpperCase() }}</div>
            <div class="organization-node-copy">
              <div class="organization-node-name">{{ node.display_name || node.username }}</div>
              <div class="organization-node-username">{{ node.username }}</div>
              <div class="organization-node-manager">{{ managerName(node) }}</div>
            </div>
            <span v-if="directReportCount(node.id)" class="organization-report-count">{{ directReportCount(node.id) }}</span>
          </article>
        </div>

        <a-spin v-if="loading && nodes.length === 0" class="organization-loading" />
        <div v-else-if="!loading && nodes.length === 0" class="organization-empty">
          <TeamOutlined />
          <span>暂无组织成员</span>
        </div>
      </div>

      <aside class="organization-inspector">
        <template v-if="selectedNode">
          <div class="inspector-avatar"><UserOutlined /></div>
          <h3>{{ selectedNode.display_name || selectedNode.username }}</h3>
          <p>{{ selectedNode.username }}</p>

          <dl class="inspector-facts">
            <div><dt>状态</dt><dd>{{ selectedNode.status === 'active' ? '启用' : '停用' }}</dd></div>
            <div><dt>直属成员</dt><dd>{{ directReportCount(selectedNode.id) }} 人</dd></div>
          </dl>

          <div class="inspector-field">
            <label>直属主管</label>
            <a-select
              v-model:value="selectedManagerID"
              allow-clear
              show-search
              option-filter-prop="label"
              :options="managerOptions"
              placeholder="设为组织根节点"
            />
            <span>不能选择本人或自己的下级。</span>
          </div>

          <a-button class="inspector-save" :loading="saving" @click="saveManager">
            <template #icon><SaveOutlined /></template>
            保存上下级关系
          </a-button>
        </template>
        <div v-else class="inspector-empty">
          <ApartmentOutlined />
          <strong>选择一个成员</strong>
          <span>查看并调整直属主管关系</span>
        </div>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.organization-shell {
  overflow: hidden;
  min-height: 680px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 16px 42px rgba(15, 23, 42, 0.06);
}

.organization-toolbar {
  display: flex;
  min-height: 58px;
  align-items: center;
  gap: 16px;
  padding: 9px 14px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.96);
}

.organization-search {
  display: flex;
  width: 240px;
  height: 38px;
  align-items: center;
  gap: 9px;
  padding: 0 12px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 12px;
  color: #94a3b8;
}

.organization-search:focus-within {
  border-color: rgba(59, 130, 246, 0.45);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.08);
}

.organization-search input {
  min-width: 0;
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: #0f172a;
  font: inherit;
  font-size: 13px;
}

.organization-toolbar-hint {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  color: #64748b;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organization-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.organization-toolbar-actions :deep(.ant-btn) {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: #475569;
}

.organization-toolbar-actions :deep(.ant-btn:hover) {
  background: #eff6ff;
  color: #2563eb;
}

.organization-zoom-value {
  min-width: 42px;
  color: #64748b;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.organization-content {
  display: grid;
  height: 620px;
  grid-template-columns: minmax(0, 1fr) 286px;
}

.organization-canvas {
  position: relative;
  overflow: hidden;
  cursor: grab;
  background-color: #f8fbff;
  background-image: radial-gradient(circle, rgba(96, 165, 250, 0.34) 1.2px, transparent 1.2px);
  background-size: 20px 20px;
  touch-action: none;
}

.organization-canvas:active {
  cursor: grabbing;
}

.organization-canvas.is-loading {
  opacity: 0.78;
}

.organization-world {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
  will-change: transform;
}

.organization-edge-layer {
  position: absolute;
  z-index: 0;
  top: 0;
  left: 0;
  overflow: visible;
  pointer-events: none;
}

.organization-edge-layer path {
  fill: none;
  stroke: #b8c8dd;
  stroke-linecap: round;
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}

.organization-node {
  position: absolute;
  z-index: 1;
  display: flex;
  width: 212px;
  height: 82px;
  align-items: center;
  gap: 11px;
  padding: 12px 13px;
  border: 1px solid #dbe5f1;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 8px 24px rgba(30, 64, 175, 0.08);
  cursor: grab;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    opacity 0.16s ease;
  user-select: none;
  will-change: transform;
}

.organization-node:active {
  cursor: grabbing;
}

.organization-node:hover,
.organization-node.selected {
  border-color: #60a5fa;
  box-shadow:
    0 10px 28px rgba(37, 99, 235, 0.15),
    0 0 0 3px rgba(59, 130, 246, 0.08);
}

.organization-node.root {
  border-color: #9fd5bf;
}

.organization-node.muted {
  opacity: 0.24;
}

.organization-node.inactive {
  filter: grayscale(0.7);
}

.organization-avatar {
  display: grid;
  width: 38px;
  height: 38px;
  flex: none;
  place-items: center;
  border-radius: 12px;
  background: #eaf2ff;
  color: #2563eb;
  font-size: 15px;
  font-weight: 800;
}

.organization-node.root .organization-avatar {
  background: #e8f7f0;
  color: #059669;
}

.organization-node-copy {
  min-width: 0;
  flex: 1;
}

.organization-node-name,
.organization-node-username,
.organization-node-manager {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organization-node-name {
  color: #172033;
  font-size: 14px;
  font-weight: 750;
  line-height: 20px;
}

.organization-node-username {
  color: #64748b;
  font-size: 11px;
  line-height: 17px;
}

.organization-node-manager {
  color: #94a3b8;
  font-size: 10px;
  line-height: 15px;
}

.organization-report-count {
  display: grid;
  min-width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 999px;
  background: #eff6ff;
  color: #2563eb;
  font-size: 11px;
  font-weight: 800;
}

.organization-loading,
.organization-empty {
  position: absolute;
  z-index: 3;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}

.organization-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  color: #94a3b8;
  font-size: 13px;
}

.organization-empty :deep(.anticon) {
  font-size: 30px;
}

.organization-inspector {
  padding: 24px 20px;
  border-left: 1px solid rgba(148, 163, 184, 0.18);
  background: #fff;
}

.inspector-avatar {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  margin-bottom: 14px;
  border-radius: 14px;
  background: #eaf2ff;
  color: #2563eb;
  font-size: 19px;
}

.organization-inspector h3 {
  margin: 0;
  color: #0f172a;
  font-size: 17px;
  line-height: 25px;
}

.organization-inspector > p {
  margin: 2px 0 20px;
  color: #94a3b8;
  font-size: 12px;
}

.inspector-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 0 0 22px;
}

.inspector-facts div {
  padding: 10px;
  border-radius: 12px;
  background: #f8fafc;
}

.inspector-facts dt {
  color: #94a3b8;
  font-size: 10px;
}

.inspector-facts dd {
  margin: 3px 0 0;
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

.inspector-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.inspector-field label {
  color: #334155;
  font-size: 12px;
  font-weight: 700;
}

.inspector-field span {
  color: #94a3b8;
  font-size: 10px;
  line-height: 16px;
}

.inspector-save {
  width: 100%;
  height: 40px;
  margin-top: 18px;
  border-color: #bfd6ff;
  border-radius: 12px;
  background: #eff6ff;
  color: #2563eb;
  font-weight: 700;
}

.inspector-empty {
  display: flex;
  height: 100%;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 7px;
  color: #94a3b8;
  text-align: center;
}

.inspector-empty :deep(.anticon) {
  margin-bottom: 5px;
  font-size: 30px;
}

.inspector-empty strong {
  color: #475569;
  font-size: 13px;
}

.inspector-empty span {
  font-size: 11px;
}

@media (max-width: 1100px) {
  .organization-content {
    grid-template-columns: minmax(0, 1fr) 248px;
  }

  .organization-toolbar-hint {
    display: none;
  }
}

@media (max-width: 760px) {
  .organization-toolbar {
    flex-wrap: wrap;
  }

  .organization-search {
    width: 100%;
  }

  .organization-content {
    height: 720px;
    grid-template-columns: 1fr;
    grid-template-rows: minmax(0, 1fr) auto;
  }

  .organization-inspector {
    min-height: 180px;
    border-top: 1px solid rgba(148, 163, 184, 0.18);
    border-left: 0;
  }
}
</style>
