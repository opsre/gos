<script setup lang="ts">
import {
  AimOutlined,
  ApartmentOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  RobotOutlined,
  SaveOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  watch,
} from "vue";
import {
  getApplicationApprovalFlowBinding,
  listApplications,
  updateApplicationApprovalFlowBinding,
} from "../../api/application";
import {
  createApprovalFlow,
  listApprovalFlows,
  updateApprovalFlow,
} from "../../api/release";
import { getReleaseSettings } from "../../api/system";
import { listUserOptions } from "../../api/user";
import type { Application } from "../../types/application";
import type {
  ApprovalFlowDefinition,
  ApprovalFlowDefinitionPayload,
  ApprovalFlowApproverSource,
  ApprovalFlowGate,
  ApprovalFlowLink,
  ApprovalFlowStatus,
  ReleaseTemplateApprovalMode,
} from "../../types/release";
import type { ReleaseEnvironmentConfig } from "../../types/system";
import type { UserOption } from "../../types/user";
import { extractHTTPErrorMessage } from "../../utils/http-error";

type EditableNode = {
  code: string;
  name: string;
  gate: ApprovalFlowGate;
  applicable_env_codes: string[];
  approval_mode: ReleaseTemplateApprovalMode;
  approver_source: ApprovalFlowApproverSource;
  manager_level: number;
  approver_ids: string[];
  position_x: number;
  position_y: number;
};

type Point = {
  x: number;
  y: number;
};

type CanvasNodeKind = "start" | "approval" | "waiting" | "end";

type CanvasNodeView = {
  code: string;
  name: string;
  subtitle: string;
  detail: string;
  kind: CanvasNodeKind;
  gate?: ApprovalFlowGate;
};

type CanvasViewport = {
  x: number;
  y: number;
  scale: number;
};

type DragState = {
  kind: "node" | "canvas";
  nodeCode?: string;
  startClientX: number;
  startClientY: number;
  startX: number;
  startY: number;
};

const NODE_WIDTH = 212;
const NODE_HEIGHT = 82;
const NODE_VERTICAL_GAP = 62;
const LAYOUT_PADDING = 84;
const LAYOUT_STORAGE_PREFIX = "gos-approval-flow-layout-v2";

const gateMeta: Array<{
  gate: ApprovalFlowGate;
  title: string;
  description: string;
  nextStage: string;
}> = [
  {
    gate: "before_execute",
    title: "整单审批",
    description: "仅完整发布执行；适合发布负责人统一确认。",
    nextStage: "执行发布",
  },
  {
    gate: "before_ci",
    title: "CI 前审批",
    description: "仅构建和完整发布都会执行；通过后才能开始构建。",
    nextStage: "CI 构建",
  },
  {
    gate: "before_cd",
    title: "CD 前审批",
    description: "发起部署和完整发布都会执行；通过后才能开始部署。",
    nextStage: "CD 部署",
  },
];

const loading = ref(false);
const saving = ref(false);
const bindingLoading = ref(false);
const flows = ref<ApprovalFlowDefinition[]>([]);
const applications = ref<Application[]>([]);
const users = ref<UserOption[]>([]);
const environmentConfigs = ref<ReleaseEnvironmentConfig[]>([]);
const selectedFlowID = ref("");
const bindingApplicationID = ref("");
const bindingFlowID = ref("");
const selectedNodeCode = ref("");
const canvasRef = ref<HTMLElement | null>(null);
const runtimePositions = ref<Record<string, Point>>({});
const viewport = reactive<CanvasViewport>({ x: 36, y: 28, scale: 1 });
const worldSize = reactive({ width: 1200, height: 900 });
let dragState: DragState | null = null;

const form = reactive<{
  id: string;
  name: string;
  status: ApprovalFlowStatus;
  nodes: EditableNode[];
}>({
  id: "",
  name: "",
  status: "active",
  nodes: [],
});

const applicationOptions = computed(() =>
  applications.value.map((item) => ({
    label: `${item.name} (${item.key})`,
    value: item.id,
  })),
);
const userOptions = computed(() =>
  users.value.map((item) => ({
    label: item.display_name || item.username,
    value: item.id,
  })),
);
const environmentOptions = computed(() => {
  const options = new Map<string, string>();
  environmentConfigs.value.forEach((item) => {
    const code = String(item.code || "").trim();
    if (!code) return;
    options.set(code, item.description ? `${code} · ${item.description}` : code);
  });
  form.nodes.forEach((node) => {
    node.applicable_env_codes.forEach((code) => {
      const value = String(code || "").trim();
      if (value && !options.has(value)) options.set(value, value);
    });
  });
  return [...options.entries()].map(([value, label]) => ({ value, label }));
});
const flowOptions = computed(() => [
  { label: "不启用审批流程", value: "" },
  ...flows.value
    .filter((item) => item.status === "active")
    .map((item) => ({ label: item.name, value: item.id })),
]);
const selectedNode = computed(
  () => form.nodes.find((node) => node.code === selectedNodeCode.value) || null,
);

function approverSummary(node: EditableNode) {
  if (node.approver_source === "manager") {
    return node.manager_level === 1
      ? "发起人的直属主管"
      : `发起人的第 ${node.manager_level} 级主管`;
  }
  return node.approver_ids.length
    ? `${node.approver_ids.length} 位指定审批人`
    : "待配置审批人";
}

function nodeSummary(node: EditableNode) {
  const parts = [
    approverSummary(node),
    node.approval_mode === "all" ? "会签" : "或签",
    environmentSummary(node),
  ];
  if (node.gate === "before_cd") {
    parts.push("Agent 检查随发布单并入");
  }
  return parts.join(" · ");
}

function environmentSummary(node: EditableNode) {
  return node.applicable_env_codes.length > 0
    ? node.applicable_env_codes.join(" / ")
    : "全部环境";
}

const canvasNodes = computed<CanvasNodeView[]>(() => [
  {
    code: "start",
    name: "开始",
    subtitle: "发布单执行入口",
    detail: "根据执行时选择的模式进入对应审批阶段。",
    kind: "start",
  },
  ...nodesFor("before_execute").map((node) => ({
    code: node.code,
    name: node.name || "整单审批",
    subtitle: nodeSummary(node),
    detail: "完整发布执行时进入该审批节点。",
    kind: "approval" as const,
    gate: node.gate,
  })),
  ...nodesFor("before_ci").map((node) => ({
    code: node.code,
    name: node.name || "CI 前审批",
    subtitle: nodeSummary(node),
    detail: "仅构建和完整发布都会进入该审批节点。",
    kind: "approval" as const,
    gate: node.gate,
  })),
  {
    code: "waiting_deploy",
    name: "待部署",
    subtitle: "按 CD / Agent 能力动态停留",
    detail: "无 CD 且无 Agent 任务时自动放行到结束；存在构建完成 Agent 任务时先显示检查状态，存在 CD 时继续等待部署并进入 CD 前审批。",
    kind: "waiting",
  },
  ...nodesFor("before_cd").map((node) => ({
    code: node.code,
    name: node.name || "CD 前审批",
    subtitle: nodeSummary(node),
    detail: "发起部署和完整发布都会进入该审批节点。",
    kind: "approval" as const,
    gate: node.gate,
  })),
  {
    code: "end",
    name: "结束",
    subtitle: "本次执行流程完成",
    detail: "审批通过后由发布单继续完成所选执行模式。",
    kind: "end",
  },
]);

const selectedCanvasNode = computed(
  () =>
    canvasNodes.value.find((node) => node.code === selectedNodeCode.value) ||
    null,
);

const visualEdges = computed(() =>
  canvasNodes.value.slice(0, -1).map((node, index) => {
    const next = canvasNodes.value[index + 1];
    const start = pointFor(node.code);
    const end = pointFor(next.code);
    const startX = start.x + NODE_WIDTH / 2;
    const startY = start.y + NODE_HEIGHT;
    const endX = end.x + NODE_WIDTH / 2;
    const endY = end.y;
    const middleY = startY + Math.max((endY - startY) / 2, 28);
    return {
      key: `${node.code}-${next.code}`,
      path: `M ${startX} ${startY} C ${startX} ${middleY}, ${endX} ${middleY}, ${endX} ${endY}`,
    };
  }),
);

function nodesFor(gate: ApprovalFlowGate) {
  return form.nodes.filter((node) => node.gate === gate);
}

function gateLabel(gate: ApprovalFlowGate) {
  return gateMeta.find((item) => item.gate === gate)?.title || "审批节点";
}

function pointFor(code: string): Point {
  const node = form.nodes.find((item) => item.code === code);
  if (node) {
    return { x: node.position_x || 0, y: node.position_y || 0 };
  }
  return runtimePositions.value[code] || { x: 0, y: 0 };
}

function setPoint(code: string, point: Point) {
  const node = form.nodes.find((item) => item.code === code);
  if (node) {
    node.position_x = Math.round(point.x);
    node.position_y = Math.round(point.y);
  } else {
    runtimePositions.value = {
      ...runtimePositions.value,
      [code]: { x: Math.round(point.x), y: Math.round(point.y) },
    };
  }
}

function refreshWorldSize() {
  const values = canvasNodes.value.map((node) => pointFor(node.code));
  worldSize.width = Math.max(
    1200,
    ...values.map((point) => point.x + NODE_WIDTH + LAYOUT_PADDING),
  );
  worldSize.height = Math.max(
    900,
    ...values.map((point) => point.y + NODE_HEIGHT + LAYOUT_PADDING),
  );
}

function layoutStorageKey() {
  return `${LAYOUT_STORAGE_PREFIX}:${form.id || "draft"}`;
}

function readSavedPositions() {
  try {
    const value = JSON.parse(
      localStorage.getItem(layoutStorageKey()) || "{}",
    ) as Record<string, Point>;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function persistPositions() {
  try {
    localStorage.setItem(
      layoutStorageKey(),
      JSON.stringify(
        Object.fromEntries(
          canvasNodes.value.map((node) => [node.code, pointFor(node.code)]),
        ),
      ),
    );
  } catch {
    // 浏览器禁用本地存储时，仍保留当前会话内布局。
  }
}

function applyAutoLayout(preserveStoredPositions = true) {
  const centeredX = LAYOUT_PADDING + 260;
  const saved = preserveStoredPositions ? readSavedPositions() : {};
  const nextRuntime: Record<string, Point> = {};
  canvasNodes.value.forEach((node, index) => {
    const automatic = {
      x: centeredX,
      y: LAYOUT_PADDING + index * (NODE_HEIGHT + NODE_VERTICAL_GAP),
    };
    const editable = form.nodes.find((item) => item.code === node.code);
    if (editable) {
      const locallySaved = saved[node.code];
      if (locallySaved) {
        editable.position_x = locallySaved.x;
        editable.position_y = locallySaved.y;
        return;
      }
      const hasStoredPosition =
        preserveStoredPositions &&
        (editable.position_x !== 0 || editable.position_y !== 0);
      if (!hasStoredPosition) {
        editable.position_x = automatic.x;
        editable.position_y = automatic.y;
      }
      return;
    }
    nextRuntime[node.code] = saved[node.code] || automatic;
  });
  runtimePositions.value = nextRuntime;
  refreshWorldSize();
  persistPositions();
}

function nodeStyle(code: string) {
  const point = pointFor(code);
  return { transform: `translate3d(${point.x}px, ${point.y}px, 0)` };
}

function worldStyle() {
  return {
    width: `${worldSize.width}px`,
    height: `${worldSize.height}px`,
    transform: `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.scale})`,
  };
}

async function fitView() {
  await nextTick();
  const container = canvasRef.value;
  const values = canvasNodes.value.map((node) => pointFor(node.code));
  if (!container || values.length === 0) return;
  const minX = Math.min(...values.map((point) => point.x));
  const minY = Math.min(...values.map((point) => point.y));
  const maxX = Math.max(...values.map((point) => point.x + NODE_WIDTH));
  const maxY = Math.max(...values.map((point) => point.y + NODE_HEIGHT));
  const boundsWidth = Math.max(maxX - minX, NODE_WIDTH);
  const boundsHeight = Math.max(maxY - minY, NODE_HEIGHT);
  const scale = Math.min(
    1.12,
    Math.max(
      0.34,
      Math.min(
        (container.clientWidth - 120) / boundsWidth,
        (container.clientHeight - 100) / boundsHeight,
      ),
    ),
  );
  viewport.scale = scale;
  viewport.x = (container.clientWidth - boundsWidth * scale) / 2 - minX * scale;
  viewport.y =
    (container.clientHeight - boundsHeight * scale) / 2 - minY * scale;
}

function zoomBy(factor: number, centerX?: number, centerY?: number) {
  const container = canvasRef.value;
  if (!container) return;
  const nextScale = Math.min(1.8, Math.max(0.3, viewport.scale * factor));
  const screenX = centerX ?? container.clientWidth / 2;
  const screenY = centerY ?? container.clientHeight / 2;
  const worldX = (screenX - viewport.x) / viewport.scale;
  const worldY = (screenY - viewport.y) / viewport.scale;
  viewport.x = screenX - worldX * nextScale;
  viewport.y = screenY - worldY * nextScale;
  viewport.scale = nextScale;
}

function handleWheel(event: WheelEvent) {
  const rect = canvasRef.value?.getBoundingClientRect();
  if (!rect) return;
  zoomBy(
    event.deltaY > 0 ? 0.9 : 1.1,
    event.clientX - rect.left,
    event.clientY - rect.top,
  );
}

function selectCanvasNode(code: string) {
  selectedNodeCode.value = code;
}

function startNodeDrag(event: PointerEvent, code: string) {
  if (event.button !== 0) return;
  event.preventDefault();
  event.stopPropagation();
  selectCanvasNode(code);
  const point = pointFor(code);
  dragState = {
    kind: "node",
    nodeCode: code,
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: point.x,
    startY: point.y,
  };
  beginPointerTracking();
}

function startCanvasPan(event: PointerEvent) {
  if (event.button !== 0) return;
  dragState = {
    kind: "canvas",
    startClientX: event.clientX,
    startClientY: event.clientY,
    startX: viewport.x,
    startY: viewport.y,
  };
  beginPointerTracking();
}

function beginPointerTracking() {
  document.body.style.userSelect = "none";
  window.addEventListener("pointermove", handlePointerMove);
  window.addEventListener("pointerup", stopPointerTracking, { once: true });
}

function handlePointerMove(event: PointerEvent) {
  if (!dragState) return;
  const deltaX = event.clientX - dragState.startClientX;
  const deltaY = event.clientY - dragState.startClientY;
  if (dragState.kind === "canvas") {
    viewport.x = dragState.startX + deltaX;
    viewport.y = dragState.startY + deltaY;
    return;
  }
  if (!dragState.nodeCode) return;
  setPoint(dragState.nodeCode, {
    x: dragState.startX + deltaX / viewport.scale,
    y: dragState.startY + deltaY / viewport.scale,
  });
  refreshWorldSize();
}

function stopPointerTracking() {
  if (dragState?.kind === "node") persistPositions();
  dragState = null;
  document.body.style.userSelect = "";
  window.removeEventListener("pointermove", handlePointerMove);
}

async function resetLayout() {
  try {
    localStorage.removeItem(layoutStorageKey());
  } catch {
    // 忽略本地存储异常。
  }
  applyAutoLayout(false);
  await fitView();
}

async function afterStructureChange() {
  await nextTick();
  applyAutoLayout(false);
  await fitView();
}

function handleAddNodeMenu({ key }: { key: string }) {
  const gate = String(key) as ApprovalFlowGate;
  if (!gateMeta.some((item) => item.gate === gate)) return;
  addNode(gate);
}

function updateSelectedNodeCode(value: string) {
  const node = selectedNode.value;
  if (!node) return;
  node.code = value;
  selectedNodeCode.value = value;
}

function buildAutomaticLinks(): ApprovalFlowLink[] {
  const raw: ApprovalFlowLink[] = [];
  const addPath = (codes: string[], scope: string) => {
    for (let index = 0; index < codes.length - 1; index += 1) {
      raw.push({
        from_code: codes[index],
        to_code: codes[index + 1],
        execution_scopes: [
          scope as "build_only" | "deploy_only" | "full_release",
        ],
        priority: raw.length + 1,
      });
    }
  };
  const beforeExecute = nodesFor("before_execute").map((node) => node.code);
  const beforeCI = nodesFor("before_ci").map((node) => node.code);
  const beforeCD = nodesFor("before_cd").map((node) => node.code);
  addPath(["start", ...beforeCI, "waiting_deploy"], "build_only");
  addPath(["waiting_deploy", ...beforeCD, "end"], "deploy_only");
  addPath(["start", ...beforeCD, "end"], "deploy_only");
  addPath(
    ["start", ...beforeExecute, ...beforeCI, ...beforeCD, "end"],
    "full_release",
  );

  const merged = new Map<string, ApprovalFlowLink>();
  raw.forEach((link) => {
    const key = `${link.from_code}->${link.to_code}`;
    const existing = merged.get(key);
    if (!existing) {
      merged.set(key, {
        ...link,
        execution_scopes: [...link.execution_scopes],
      });
      return;
    }
    link.execution_scopes.forEach((scope) => {
      if (!existing.execution_scopes.includes(scope))
        existing.execution_scopes.push(scope);
    });
  });
  return [...merged.values()].map((link, index) => ({
    ...link,
    priority: index + 1,
  }));
}

function nextNodeCode() {
  const prefix = "approval";
  let sequence = form.nodes.length + 1;
  while (form.nodes.some((node) => node.code === `${prefix}_${sequence}`))
    sequence += 1;
  return `${prefix}_${sequence}`;
}

function addNode(gate: ApprovalFlowGate) {
  const node: EditableNode = {
    code: nextNodeCode(),
    name: gateLabel(gate),
    gate,
    applicable_env_codes: [],
    approval_mode: "any",
    approver_source: "users",
    manager_level: 1,
    approver_ids: [],
    position_x: 0,
    position_y: 0,
  };
  form.nodes.push(node);
  selectedNodeCode.value = node.code;
  void afterStructureChange();
}

function duplicateSelectedNode() {
  if (!selectedNode.value) return;
  const source = selectedNode.value;
  const node: EditableNode = {
    ...source,
    code: nextNodeCode(),
    name: `${source.name}（副本）`,
    applicable_env_codes: [...source.applicable_env_codes],
    approver_ids: [...source.approver_ids],
    position_x: 0,
    position_y: 0,
  };
  const index = form.nodes.findIndex((item) => item.code === source.code);
  form.nodes.splice(index + 1, 0, node);
  selectedNodeCode.value = node.code;
  void afterStructureChange();
}

function removeNode(node: EditableNode) {
  if (form.nodes.length <= 1) {
    message.warning("审批流至少保留一个审批节点");
    return;
  }
  const index = form.nodes.indexOf(node);
  form.nodes.splice(index, 1);
  if (selectedNodeCode.value === node.code)
    selectedNodeCode.value =
      form.nodes[Math.max(0, index - 1)]?.code || "start";
  void afterStructureChange();
}

function moveNode(node: EditableNode, direction: -1 | 1) {
  const group = nodesFor(node.gate);
  const groupIndex = group.indexOf(node);
  const target = group[groupIndex + direction];
  if (!target) return;
  const nodeIndex = form.nodes.indexOf(node);
  const targetIndex = form.nodes.indexOf(target);
  [form.nodes[nodeIndex], form.nodes[targetIndex]] = [
    form.nodes[targetIndex],
    form.nodes[nodeIndex],
  ];
  void afterStructureChange();
}

function resetFlow() {
  form.id = "";
  form.name = "未命名审批流";
  form.status = "active";
  form.nodes = [];
  selectedFlowID.value = "";
  selectedNodeCode.value = "start";
  addNode("before_execute");
}

function selectFlow(flowID: string) {
  const flow = flows.value.find((item) => item.id === flowID);
  if (!flow) return;
  form.id = flow.id;
  form.name = flow.name;
  form.status = flow.status;
  form.nodes = flow.nodes
    .filter((node) => node.node_type !== "agent_task")
    .map((node) => ({
    code: node.code,
    name: node.name,
    gate: node.gate,
    applicable_env_codes: [...(node.applicable_env_codes || [])],
    approval_mode: (node.approval_mode || "any") as ReleaseTemplateApprovalMode,
    approver_source: node.approver_source || "users",
    manager_level: node.manager_level || 1,
    approver_ids: [...(node.approver_ids || [])],
    position_x: node.position_x || 0,
    position_y: node.position_y || 0,
  }));
  if (form.nodes.length === 0) {
    addNode("before_execute");
  }
  selectedNodeCode.value = form.nodes[0]?.code || "start";
  void nextTick(async () => {
    applyAutoLayout(true);
    await fitView();
  });
}

function payload(): ApprovalFlowDefinitionPayload | null {
  const name = form.name.trim();
  if (!name) {
    message.warning("请填写流程名称");
    return null;
  }
  const codes = new Set<string>();
  for (const node of form.nodes) {
    node.code = node.code.trim();
    if (!node.code || !node.name.trim()) {
      message.warning("请为每个流程节点填写编码和名称");
      return null;
    }
    if (node.approver_source === "users" && node.approver_ids.length === 0) {
      message.warning(`请为节点「${node.name}」选择审批人`);
      return null;
    }
    if (node.approver_source === "manager" && (node.manager_level < 1 || node.manager_level > 20)) {
      message.warning("组织上级层级必须在 1 到 20 之间");
      return null;
    }
    if (codes.has(node.code)) {
      message.warning("节点编码不能重复");
      return null;
    }
    codes.add(node.code);
  }
  return {
    name,
    status: form.status,
    nodes: form.nodes.map((node) => ({
      code: node.code,
      name: node.name.trim(),
      gate: node.gate,
      node_type: "approval",
      applicable_env_codes: [...node.applicable_env_codes],
      approval_mode: node.approval_mode,
      approver_source: node.approver_source,
      manager_level: node.approver_source === "manager" ? node.manager_level : 0,
      approver_ids: node.approver_source === "users" ? [...node.approver_ids] : [],
      approver_names: (node.approver_source === "users" ? node.approver_ids : []).map(
        (id) => users.value.find((item) => item.id === id)?.display_name || id,
      ),
      agent_task_id: "",
      agent_task_name: "",
      position_x: node.position_x,
      position_y: node.position_y,
    })),
    links: buildAutomaticLinks(),
  };
}

async function saveFlow() {
  const nextPayload = payload();
  if (!nextPayload) return;
  saving.value = true;
  try {
    const response = form.id
      ? await updateApprovalFlow(form.id, nextPayload)
      : await createApprovalFlow(nextPayload);
    await loadData(response.data.id);
    message.success("审批流已保存");
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "审批流保存失败"));
  } finally {
    saving.value = false;
  }
}

async function loadBinding() {
  bindingFlowID.value = "";
  if (!bindingApplicationID.value) return;
  bindingLoading.value = true;
  try {
    const response = await getApplicationApprovalFlowBinding(
      bindingApplicationID.value,
    );
    bindingFlowID.value = response.data.approval_flow_id || "";
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "应用审批流加载失败"));
  } finally {
    bindingLoading.value = false;
  }
}

async function saveBinding() {
  if (!bindingApplicationID.value) {
    message.warning("请选择需要绑定的应用");
    return;
  }
  bindingLoading.value = true;
  try {
    await updateApplicationApprovalFlowBinding(
      bindingApplicationID.value,
      bindingFlowID.value,
    );
    message.success("应用默认审批流已保存，新建发布单会自动继承");
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "应用审批流保存失败"));
  } finally {
    bindingLoading.value = false;
  }
}

async function loadData(preferredFlowID = "") {
  loading.value = true;
  try {
    const [flowResponse, appResponse, userResponse, settingsResponse] = await Promise.all([
      listApprovalFlows(),
      listApplications({ page: 1, page_size: 100 }),
      listUserOptions(),
      getReleaseSettings().catch(() => null),
    ]);
    flows.value = flowResponse.data || [];
    applications.value = appResponse.data || [];
    users.value = userResponse.data || [];
    if (settingsResponse) {
      const configs = settingsResponse.data.env_configs || [];
      environmentConfigs.value = configs.length > 0
        ? configs
        : (settingsResponse.data.env_options || []).map((code) => ({ code, description: "" }));
    }
    const targetID =
      preferredFlowID || selectedFlowID.value || flows.value[0]?.id || "";
    if (targetID) {
      selectedFlowID.value = targetID;
      selectFlow(targetID);
    } else {
      resetFlow();
    }
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "审批流数据加载失败"));
    resetFlow();
  } finally {
    loading.value = false;
  }
}

watch(selectedFlowID, (value) => {
  if (value) selectFlow(value);
});

onMounted(() => loadData());

onBeforeUnmount(() => {
  stopPointerTracking();
});
</script>

<template>
  <div class="approval-settings">
    <header class="settings-header">
      <div class="settings-heading">
        <h2>审批流管理</h2>
        <span>应用级默认流程</span>
      </div>
      <div class="binding-controls">
        <label
          >应用
          <a-select
            v-model:value="bindingApplicationID"
            show-search
            option-filter-prop="label"
            :options="applicationOptions"
            placeholder="选择应用"
            @change="loadBinding"
        /></label>
        <label
          >流程
          <a-select
            v-model:value="bindingFlowID"
            :options="flowOptions"
            :loading="bindingLoading"
            placeholder="应用默认流程"
        /></label>
        <a-button :loading="bindingLoading" @click="saveBinding"
          >保存应用绑定</a-button
        >
      </div>
      <div class="header-actions">
        <a-dropdown :trigger="['click']">
          <a-button>
            <template #icon><PlusOutlined /></template>添加审批
          </a-button>
          <template #overlay>
            <a-menu @click="handleAddNodeMenu">
              <a-menu-item v-for="stage in gateMeta" :key="stage.gate">
                <UserOutlined />{{ stage.title }}
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
        <a-button @click="resetFlow"
          ><template #icon><PlusOutlined /></template>新建流程</a-button
        >
        <a-button type="primary" :loading="saving" @click="saveFlow"
          ><template #icon><SaveOutlined /></template>保存流程</a-button
        >
      </div>
    </header>

    <div class="settings-layout">
      <main class="stage-editor">
        <div class="flow-toolbar">
          <a-select
            v-if="flows.length"
            v-model:value="selectedFlowID"
            class="flow-selector"
            :options="
              flows.map((item) => ({ label: item.name, value: item.id }))
            "
            placeholder="选择已有流程"
          />
          <div v-else class="flow-selector flow-selector-empty">暂无已保存流程</div>
          <a-input
            v-model:value="form.name"
            class="flow-name"
            placeholder="流程名称"
          />
          <a-select
            v-model:value="form.status"
            class="flow-status"
            :options="[
              { label: '启用', value: 'active' },
              { label: '停用', value: 'disabled' },
            ]"
          />
          <div class="flow-toolbar-hint">
            <ApartmentOutlined />拖动节点调整布局，流程关系由系统自动生成
          </div>
          <div class="flow-toolbar-actions">
            <a-button type="text" aria-label="缩小" @click="zoomBy(0.9)"><ZoomOutOutlined /></a-button>
            <span>{{ Math.round(viewport.scale * 100) }}%</span>
            <a-button type="text" aria-label="放大" @click="zoomBy(1.1)"><ZoomInOutlined /></a-button>
            <a-button type="text" aria-label="适应画布" @click="fitView"><AimOutlined /></a-button>
            <a-button type="text" aria-label="自动排版" @click="resetLayout"><TeamOutlined /></a-button>
            <a-button type="text" aria-label="刷新审批流" :loading="loading" @click="loadData(form.id)"><ReloadOutlined /></a-button>
          </div>
        </div>

        <div
          ref="canvasRef"
          class="approval-canvas"
          :class="{ 'is-loading': loading }"
          @pointerdown="startCanvasPan"
          @wheel.prevent="handleWheel"
        >
          <div class="approval-world" :style="worldStyle()">
            <svg
              class="approval-edge-layer"
              :width="worldSize.width"
              :height="worldSize.height"
              aria-hidden="true"
            >
              <path v-for="edge in visualEdges" :key="edge.key" :d="edge.path" />
            </svg>

            <article
              v-for="node in canvasNodes"
              :key="node.code"
              class="approval-canvas-node"
              :class="[
                `kind-${node.kind}`,
                { selected: selectedNodeCode === node.code },
              ]"
              :style="nodeStyle(node.code)"
              @pointerdown="startNodeDrag($event, node.code)"
              @click.stop="selectCanvasNode(node.code)"
            >
              <span class="canvas-node-icon">
                <PlayCircleOutlined v-if="node.kind === 'start'" />
                <PauseCircleOutlined v-else-if="node.kind === 'waiting'" />
                <CheckCircleOutlined v-else-if="node.kind === 'end'" />
                <UserOutlined v-else />
              </span>
              <span class="canvas-node-copy">
                <strong>{{ node.name }}</strong>
                <small>{{ node.code }}</small>
                <em>{{ node.subtitle }}</em>
              </span>
            </article>
          </div>

          <a-spin v-if="loading" class="approval-canvas-loading" />
        </div>
      </main>

      <aside class="node-inspector">
        <div class="inspector-head">
          <div>
            <h3>节点配置</h3>
            <p>选择画布节点查看或配置</p>
          </div>
          <div v-if="selectedNode" class="inspector-head-actions">
            <a-button
              type="text"
              size="small"
              aria-label="上移节点"
              :disabled="nodesFor(selectedNode.gate).indexOf(selectedNode) === 0"
              @click="moveNode(selectedNode, -1)"
            ><ArrowUpOutlined /></a-button>
            <a-button
              type="text"
              size="small"
              aria-label="下移节点"
              :disabled="nodesFor(selectedNode.gate).indexOf(selectedNode) === nodesFor(selectedNode.gate).length - 1"
              @click="moveNode(selectedNode, 1)"
            ><ArrowDownOutlined /></a-button>
            <a-button type="text" size="small" aria-label="复制节点" @click="duplicateSelectedNode"><CopyOutlined /></a-button>
          </div>
        </div>
        <template v-if="selectedNode">
          <div class="inspector-summary">
            <span class="node-icon"><UserOutlined /></span>
            <div>
              <strong>{{ selectedNode.name || "审批节点" }}</strong
              ><small>人工审批节点 · {{ gateLabel(selectedNode.gate) }}</small>
            </div>
          </div>
          <a-form layout="vertical" class="inspector-form">
            <a-form-item label="节点名称"
              ><a-input v-model:value="selectedNode.name" :maxlength="50"
            /></a-form-item>
            <a-form-item label="节点编码"
              ><a-input :value="selectedNode.code" :maxlength="50" @update:value="updateSelectedNodeCode"
            /></a-form-item>
            <a-form-item label="审批阶段"
              ><a-select
                v-model:value="selectedNode.gate"
                :options="
                  gateMeta.map((item) => ({
                    label: item.title,
                    value: item.gate,
                  }))
                "
            /></a-form-item>
            <a-form-item label="适用环境">
              <a-select
                v-model:value="selectedNode.applicable_env_codes"
                mode="multiple"
                allow-clear
                show-search
                option-filter-prop="label"
                :max-tag-count="2"
                :options="environmentOptions"
                placeholder="全部环境（不限制）"
              />
              <div class="manager-level-help">不选择表示全部环境；发布单执行时会自动跳过环境不匹配的节点。</div>
            </a-form-item>
            <div v-if="selectedNode.gate === 'before_cd'" class="cd-agent-bridge">
              <span class="cd-agent-bridge-icon"><RobotOutlined /></span>
              <div>
                <strong>Agent 任务随发布单自动并入</strong>
                <p>发布模板若配置“构建完成”阶段的 Agent 任务，将先在此 CD 审核节点执行；成功后进入人工审批，失败则按 Hook 策略阻断发布。未配置时直接进入人工审批。</p>
              </div>
            </div>
            <a-form-item label="审批方式"
              ><a-radio-group v-model:value="selectedNode.approval_mode"
                ><a-radio value="any">或签</a-radio
                ><a-radio value="all">会签</a-radio></a-radio-group
              ></a-form-item
            >
            <a-form-item label="审批人来源">
              <a-radio-group v-model:value="selectedNode.approver_source" button-style="solid">
                <a-radio-button value="users">指定用户</a-radio-button>
                <a-radio-button value="manager">组织上级</a-radio-button>
              </a-radio-group>
            </a-form-item>
            <a-form-item v-if="selectedNode.approver_source === 'users'" label="审批人"
              ><a-select
                v-model:value="selectedNode.approver_ids"
                mode="multiple"
                show-search
                option-filter-prop="label"
                :options="userOptions"
                placeholder="选择审批人"
            /></a-form-item>
            <a-form-item v-else label="上级层级">
              <a-input-number v-model:value="selectedNode.manager_level" :min="1" :max="20" style="width: 100%" />
              <div class="manager-level-help">1 表示直属主管，2 表示直属主管的主管，以发布单发起人为起点动态解析。</div>
            </a-form-item>
          </a-form>
          <div class="inspector-note">
            <SettingOutlined />通过后进入「{{
              gateMeta.find((item) => item.gate === selectedNode?.gate)
                ?.nextStage
            }}」阶段。
          </div>
          <a-button danger block @click="removeNode(selectedNode)"
            ><template #icon><DeleteOutlined /></template>删除节点</a-button
          >
        </template>
        <template v-else-if="selectedCanvasNode">
          <div class="inspector-state-icon" :class="`kind-${selectedCanvasNode.kind}`">
            <PlayCircleOutlined v-if="selectedCanvasNode.kind === 'start'" />
            <PauseCircleOutlined v-else-if="selectedCanvasNode.kind === 'waiting'" />
            <CheckCircleOutlined v-else />
          </div>
          <h4 class="inspector-state-title">{{ selectedCanvasNode.name }}</h4>
          <p class="inspector-state-code">{{ selectedCanvasNode.code }}</p>
          <div class="inspector-state-detail">{{ selectedCanvasNode.detail }}</div>
          <div class="inspector-note state-note">
            <SettingOutlined />系统状态节点只用于表达发布单生命周期，不需要配置审批人。
          </div>
        </template>
        <div v-else class="inspector-empty">
          <ApartmentOutlined />
          <strong>选择一个节点</strong>
          <span>查看系统状态或配置审批规则</span>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.approval-settings {
  height: calc(100vh - 118px);
  min-height: 660px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 16px 42px rgba(15, 23, 42, 0.06);
}
.settings-header {
  min-height: 70px;
  padding: 12px 16px 12px 20px;
  display: flex;
  align-items: center;
  gap: 22px;
  border-bottom: 1px solid #e8edf4;
}
.settings-heading {
  display: flex;
  align-items: baseline;
  gap: 10px;
  white-space: nowrap;
}
.settings-heading h2 {
  margin: 0;
  color: #172033;
  font-size: 18px;
}
.settings-heading span {
  color: #8a94a6;
  font-size: 12px;
}
.binding-controls {
  min-width: 0;
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
}
.binding-controls label {
  display: flex;
  align-items: center;
  gap: 7px;
  color: #667085;
  font-size: 12px;
  white-space: nowrap;
}
.binding-controls :deep(.ant-select) {
  width: 180px;
}
.header-actions {
  display: flex;
  gap: 8px;
}
.settings-layout {
  min-height: 0;
  flex: 1;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 306px;
}
.stage-editor {
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0;
  background: #f8fbff;
}
.flow-toolbar {
  display: flex;
  min-height: 58px;
  align-items: center;
  gap: 10px;
  margin: 0;
  padding: 9px 14px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.96);
}
.flow-selector {
  width: 200px;
}
.flow-selector-empty {
  display: flex;
  height: 32px;
  align-items: center;
  padding: 0 11px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 10px;
  background: #f8fafc;
  color: #94a3b8;
  font-size: 12px;
}
.flow-name {
  width: 210px;
}
.flow-status {
  width: 90px;
}
.flow-toolbar :deep(.ant-select-selector),
.flow-toolbar :deep(.ant-input) {
  border-radius: 10px !important;
}
.flow-toolbar-hint {
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
.flow-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}
.flow-toolbar-actions :deep(.ant-btn) {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: #475569;
}
.flow-toolbar-actions :deep(.ant-btn:hover) {
  background: #eff6ff;
  color: #2563eb;
}
.flow-toolbar-actions > span {
  min-width: 42px;
  color: #64748b;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: center;
}
.approval-canvas {
  position: relative;
  min-height: 0;
  flex: 1;
  overflow: hidden;
  cursor: grab;
  background-color: #f8fbff;
  background-image: radial-gradient(circle, rgba(96, 165, 250, 0.34) 1.2px, transparent 1.2px);
  background-size: 20px 20px;
  touch-action: none;
}
.approval-canvas:active {
  cursor: grabbing;
}
.approval-canvas.is-loading {
  opacity: 0.78;
}
.approval-world {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
  will-change: transform;
}
.approval-edge-layer {
  position: absolute;
  z-index: 0;
  top: 0;
  left: 0;
  overflow: visible;
  pointer-events: none;
}
.approval-edge-layer path {
  fill: none;
  stroke: #b8c8dd;
  stroke-linecap: round;
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.approval-canvas-node {
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
    box-shadow 0.16s ease;
  user-select: none;
  will-change: transform;
}
.approval-canvas-node:active {
  cursor: grabbing;
}
.approval-canvas-node:hover,
.approval-canvas-node.selected {
  border-color: #60a5fa;
  box-shadow:
    0 10px 28px rgba(37, 99, 235, 0.15),
    0 0 0 3px rgba(59, 130, 246, 0.08);
}
.approval-canvas-node.kind-start {
  border-color: #9fd5bf;
}
.approval-canvas-node.kind-waiting {
  border-color: #f5c76f;
}
.approval-canvas-node.kind-end {
  border-color: #f0b4b4;
}
.canvas-node-icon {
  display: grid;
  width: 38px;
  height: 38px;
  flex: none;
  place-items: center;
  border-radius: 12px;
  background: #eaf2ff;
  color: #2563eb;
  font-size: 16px;
}
.kind-start .canvas-node-icon {
  background: #e8f7f0;
  color: #059669;
}
.kind-waiting .canvas-node-icon {
  background: #fff6dc;
  color: #d48b05;
}
.kind-end .canvas-node-icon {
  background: #fff0f0;
  color: #dc4c4c;
}
.canvas-node-copy {
  min-width: 0;
  flex: 1;
}
.canvas-node-copy strong,
.canvas-node-copy small,
.canvas-node-copy em {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.canvas-node-copy strong {
  color: #172033;
  font-size: 14px;
  font-style: normal;
  font-weight: 750;
  line-height: 20px;
}
.canvas-node-copy small {
  color: #64748b;
  font-size: 10px;
  line-height: 15px;
}
.canvas-node-copy em {
  color: #94a3b8;
  font-size: 10px;
  font-style: normal;
  line-height: 15px;
}
.approval-canvas-loading {
  position: absolute;
  z-index: 3;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}
.stage-list {
  display: grid;
  gap: 14px;
}
.stage-section {
  padding: 16px;
  border: 1px solid #e1e7ef;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 2px 8px rgba(15, 39, 72, 0.04);
}
.stage-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}
.stage-header h3 {
  margin: 0;
  color: #24334a;
  font-size: 15px;
}
.stage-header p {
  margin: 5px 0 0;
  color: #8490a2;
  font-size: 12px;
  line-height: 1.5;
}
.stage-nodes {
  display: grid;
  gap: 8px;
}
.approval-node-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 58px;
  padding: 9px 10px;
  border: 1px solid #e1e7ef;
  border-radius: 8px;
  background: #fbfcfe;
  cursor: pointer;
  transition: 0.16s;
}
.approval-node-row:hover {
  border-color: #91caff;
  background: #f7fbff;
}
.approval-node-row.selected {
  border-color: #1677ff;
  background: #eef6ff;
  box-shadow: 0 0 0 2px rgba(22, 119, 255, 0.09);
}
.node-order {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  flex: none;
  border-radius: 50%;
  background: #eaf0f7;
  color: #64748b;
  font-size: 11px;
  font-weight: 700;
}
.node-icon {
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  flex: none;
  border-radius: 50%;
  background: #1677ff;
  color: #fff;
}
.node-main {
  min-width: 0;
  flex: 1;
}
.node-main strong {
  display: block;
  color: #2c3b50;
  font-size: 13px;
}
.node-main small {
  display: block;
  margin-top: 3px;
  color: #8190a3;
  font-size: 11px;
}
.node-actions {
  display: flex;
  align-items: center;
}
.empty-stage {
  width: 100%;
  height: 48px;
  border: 1px dashed #ccd6e3;
  border-radius: 8px;
  background: #fafbfd;
  color: #8a97a8;
  font-size: 12px;
  cursor: pointer;
}
.empty-stage:hover {
  border-color: #91caff;
  background: #f5faff;
  color: #1677ff;
}
.node-inspector {
  overflow: auto;
  padding: 22px 20px;
  border-left: 1px solid rgba(148, 163, 184, 0.18);
  background: #fff;
}
.inspector-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.inspector-head h3 {
  margin: 0;
  color: #172033;
  font-size: 15px;
}
.inspector-head p {
  margin: 5px 0 0;
  color: #8995a6;
  font-size: 11px;
}
.inspector-head-actions {
  display: flex;
  align-items: flex-start;
}
.inspector-head-actions :deep(.ant-btn) {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  color: #64748b;
}
.inspector-summary {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 20px 0 18px;
  padding: 12px;
  border-radius: 8px;
  background: #f5f9ff;
}
.inspector-summary strong {
  display: block;
  color: #26364d;
  font-size: 13px;
}
.inspector-summary small {
  display: block;
  margin-top: 3px;
  color: #7e8b9e;
  font-size: 11px;
}
.inspector-form :deep(.ant-form-item) {
  margin-bottom: 15px;
}
.inspector-form :deep(.ant-form-item-label > label) {
  color: #475467;
  font-size: 12px;
  font-weight: 600;
}
.manager-level-help {
  margin-top: 7px;
  color: #8591a3;
  font-size: 11px;
  line-height: 1.55;
}
.cd-agent-bridge {
  display: flex;
  gap: 10px;
  margin: 0 0 16px;
  padding: 12px;
  border: 1px solid rgba(37, 99, 235, 0.14);
  border-radius: 10px;
  background: #f5f9ff;
}
.cd-agent-bridge-icon {
  display: grid;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  place-items: center;
  border-radius: 9px;
  background: #e0ecff;
  color: #2563eb;
}
.cd-agent-bridge strong {
  display: block;
  color: #26364d;
  font-size: 12px;
}
.cd-agent-bridge p {
  margin: 4px 0 0;
  color: #728096;
  font-size: 11px;
  line-height: 1.6;
}
.inspector-note {
  display: flex;
  gap: 7px;
  margin: 2px 0 16px;
  padding: 10px;
  border-radius: 6px;
  background: #f7f9fc;
  color: #778497;
  font-size: 11px;
  line-height: 1.55;
}
.inspector-note :deep(svg) {
  margin-top: 2px;
  color: #7990ad;
}
.inspector-state-icon {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  margin: 34px 0 14px;
  border-radius: 14px;
  background: #eaf2ff;
  color: #2563eb;
  font-size: 19px;
}
.inspector-state-icon.kind-start {
  background: #e8f7f0;
  color: #059669;
}
.inspector-state-icon.kind-waiting {
  background: #fff6dc;
  color: #d48b05;
}
.inspector-state-icon.kind-end {
  background: #fff0f0;
  color: #dc4c4c;
}
.inspector-state-title {
  margin: 0;
  color: #0f172a;
  font-size: 17px;
  line-height: 25px;
}
.inspector-state-code {
  margin: 2px 0 20px;
  color: #94a3b8;
  font-size: 12px;
}
.inspector-state-detail {
  padding: 13px;
  border-radius: 12px;
  background: #f8fafc;
  color: #475569;
  font-size: 12px;
  line-height: 1.7;
}
.state-note {
  margin-top: 14px;
}
.inspector-empty {
  display: flex;
  height: calc(100% - 48px);
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
  .settings-header {
    flex-wrap: wrap;
  }
  .binding-controls {
    order: 3;
    flex-basis: 100%;
  }
  .settings-layout {
    grid-template-columns: minmax(0, 1fr) 300px;
  }
  .flow-toolbar-hint {
    display: none;
  }
  .flow-name {
    width: 170px;
  }
}
@media (max-width: 760px) {
  .approval-settings {
    height: auto;
  }
  .binding-controls {
    flex-wrap: wrap;
  }
  .binding-controls label,
  .binding-controls :deep(.ant-select) {
    width: 100%;
  }
  .settings-layout {
    grid-template-columns: 1fr;
    grid-template-rows: 620px auto;
  }
  .node-inspector {
    border-left: 0;
    border-top: 1px solid #e8edf4;
  }
  .flow-toolbar {
    flex-wrap: wrap;
  }
  .flow-selector,
  .flow-name {
    width: 100%;
  }
  .flow-status {
    flex: 1;
  }
  .flow-toolbar-actions {
    margin-left: auto;
  }
  .approval-canvas {
    min-height: 540px;
  }
}
</style>
