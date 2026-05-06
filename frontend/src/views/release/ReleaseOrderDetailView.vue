<script setup lang="ts">
import {
  ArrowLeftOutlined,
  CheckCircleFilled,
  ClockCircleFilled,
  CloseCircleFilled,
  ExclamationCircleOutlined,
  LoadingOutlined,
  StopFilled,
  SyncOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import type { TableColumnsType } from "ant-design-vue";
import dayjs from "dayjs";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
} from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  approveReleaseOrder,
  buildReleaseOrder,
  buildReleaseOrderLogStreamURL,
  cancelReleaseOrder,
  deployReleaseOrder,
  executeReleaseOrder,
  getReleaseOrderConcurrentBatchProgress,
  getReleaseOrderByID,
  getReleaseOrderPrecheck,
  getReleaseOrderPipelineStageLog,
  listReleaseOrderApprovalRecords,
  listReleaseOrderExecutions,
  listReleaseOrderParams,
  listReleaseOrderValueProgress,
  listReleaseOrderPipelineStages,
  listReleaseOrderSteps,
  rejectReleaseOrder,
  replayReleaseOrderByID,
  rollbackReleaseOrderByID,
  submitReleaseOrderApproval,
} from "../../api/release";
import { useResizableColumns } from "../../composables/useResizableColumns";
import { useAuthStore } from "../../stores/auth";
import type {
  ReleaseOperationType,
  ReleaseOrderApprovalRecord,
  ReleaseOrderDispatchAction,
  ReleaseOrder,
  ReleaseOrderBusinessStatus,
  ReleaseOrderExecution,
  ReleaseOrderConcurrentBatchProgress,
  ReleaseOrderConcurrentBatchQueueState,
  ReleaseOrderLogStreamEvent,
  ReleaseOrderParam,
  ReleaseOrderPrecheck,
  ReleaseOrderPrecheckItem,
  ReleaseOrderValueProgress,
  ReleaseOrderValueProgressStatus,
  ReleaseOrderPipelineStage,
  ReleaseOrderStatus,
  ReleaseOrderStep,
  ReleasePipelineScope,
  ReleasePipelineStageStatus,
  ReleaseTriggerType,
} from "../../types/release";
import { extractHTTPErrorMessage } from "../../utils/http-error";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const AUTO_REFRESH_INTERVAL_MS = 5000;
// Keep pipeline stages in sync with the main release polling cadence so progress feels responsive.
const PIPELINE_STAGE_REFRESH_INTERVAL_MS = 5000;

type ScopeLogState = {
  text: string;
  offset: number;
  connected: boolean;
  connecting: boolean;
  ended: boolean;
  error: string;
  statusText: string;
  panelRef: HTMLElement | null;
  stream: EventSource | null;
  reconnectTimer: number | null;
  closeIntentional: boolean;
  autoFollow: boolean;
};

function createScopeLogState(): ScopeLogState {
  return {
    text: "",
    offset: 0,
    connected: false,
    connecting: false,
    ended: false,
    error: "",
    statusText: "未连接",
    panelRef: null,
    stream: null,
    reconnectTimer: null,
    closeIntentional: false,
    autoFollow: true,
  };
}

const loading = ref(false);
const querying = ref(false);
const cancelling = ref(false);
const executing = ref(false);
const recovering = ref(false);
const approvalActing = ref(false);
const autoRefreshTimer = ref<number | null>(null);
const executeLocked = ref(false);
const currentDispatchAction = ref<ReleaseOrderDispatchAction>("execute");

const order = ref<ReleaseOrder | null>(null);
const approvalRecords = ref<ReleaseOrderApprovalRecord[]>([]);
const approvalRecordsLoading = ref(false);
const params = ref<ReleaseOrderParam[]>([]);
const valueProgress = ref<ReleaseOrderValueProgress[]>([]);
const steps = ref<ReleaseOrderStep[]>([]);
const executions = ref<ReleaseOrderExecution[]>([]);
const pipelineStages = ref<ReleaseOrderPipelineStage[]>([]);
const precheck = ref<ReleaseOrderPrecheck | null>(null);
const precheckLoading = ref(false);
const pipelineStageModuleVisible = ref(false);
const pipelineStageExecutorType = ref("");
const pipelineStageMessage = ref("");
const pipelineStageLoading = ref(false);
const lastPipelineStageRefreshAt = ref(0);

const stageLogDrawerVisible = ref(false);
const stageLogLoading = ref(false);
const stageLogContent = ref("");
const stageLogHasMore = ref(false);
const stageLogFetchedAt = ref("");
const selectedPipelineStage = ref<ReleaseOrderPipelineStage | null>(null);
const stageLogStillStreaming = computed(
  () =>
    stageLogHasMore.value &&
    Boolean(selectedPipelineStage.value) &&
    isRunningStatus(selectedPipelineStage.value!.status),
);
const stageLogSyncMessage = computed(() => {
  if (!stageLogFetchedAt.value) {
    return "";
  }
  return `最近同步时间：${stageLogFetchedAt.value}${
    stageLogStillStreaming.value ? "，当前阶段仍在持续输出日志" : ""
  }`;
});
const concurrentBatchProgress = ref<ReleaseOrderConcurrentBatchProgress | null>(
  null,
);
const concurrentBatchLoading = ref(false);
const expandedHookTaskMap = reactive<Record<string, boolean>>({});
const expandedHookLogMap = reactive<Record<string, boolean>>({});
const approvalActionModalVisible = ref(false);
const approvalActionMode = ref<"submit" | "approve" | "reject">("submit");
const approvalActionComment = ref("");

const scopeLogStates = reactive<Record<ReleasePipelineScope, ScopeLogState>>({
  ci: createScopeLogState(),
  cd: createScopeLogState(),
});

const orderID = computed(() => String(route.params.id || "").trim());
const fastExecuteRequested = computed(() => {
  const value = String(route.query.fast_execute || "").trim().toLowerCase();
  return value === "1" || value === "true" || value === "yes";
});
const fastExecuteTriggered = ref(false);
const currentBusinessStatus = computed<ReleaseOrderBusinessStatus>(() => {
  if (!order.value) {
    return "pending_execution";
  }
  if (order.value.business_status) {
    return order.value.business_status;
  }
  switch (order.value.status) {
    case "draft":
      return "draft";
    case "pending_approval":
      return "pending_approval";
    case "approving":
      return "approving";
    case "approved":
      return "approved";
    case "building":
      return "building";
    case "built_waiting_deploy":
      return "built_waiting_deploy";
    case "rejected":
      return "rejected";
    case "queued":
      return "queued";
    case "deploying":
      return "deploying";
    case "deploy_success":
    case "success":
      return "deploy_success";
    case "deploy_failed":
    case "failed":
      return "deploy_failed";
    case "running":
      return "deploying";
    case "cancelled":
      return "cancelled";
    default:
      return "pending_execution";
  }
});
const canViewParamSnapshot = computed(() =>
  authStore.hasPermission("release.param_snapshot.view"),
);
const currentUserID = computed(() => String(authStore.profile?.id || "").trim());
const isCurrentUserCreator = computed(() =>
  Boolean(order.value) &&
  String(order.value?.creator_user_id || "").trim() === currentUserID.value,
);
const canExecutePermission = computed(() =>
  order.value
    ? authStore.hasApplicationPermission(
        "release.execute",
        order.value.application_id,
        order.value.env_code,
      )
    : false,
);
const canEdit = computed(
  () =>
    Boolean(order.value) &&
    (authStore.isAdmin || isCurrentUserCreator.value) &&
    String(order.value?.status || "").trim() === "pending" &&
    String(order.value?.operation_type || "").trim() === "deploy" &&
    !String(order.value?.source_order_id || "").trim(),
);
const isCurrentUserApprover = computed(() => {
  if (!order.value || !currentUserID.value) {
    return false;
  }
  return (order.value.approval_approver_ids || []).includes(currentUserID.value);
});
const showApprovalCard = computed(
  () => Boolean(order.value?.approval_required) || approvalRecords.value.length > 0,
);
const displayApprovalRecords = computed<ReleaseOrderApprovalRecord[]>(() => {
  if (approvalRecords.value.length > 0) {
    return approvalRecords.value;
  }
  if (!order.value) {
    return [];
  }
  const fallbackRecords: ReleaseOrderApprovalRecord[] = [];
  if (order.value.approved_at) {
    fallbackRecords.push({
      id: `synthetic-approve-${order.value.id}`,
      release_order_id: order.value.id,
      action: "approve",
      operator_user_id: "",
      operator_name: String(order.value.approved_by || "").trim() || "系统",
      comment: "",
      created_at: order.value.approved_at,
    });
  }
  if (order.value.rejected_at) {
    fallbackRecords.push({
      id: `synthetic-reject-${order.value.id}`,
      release_order_id: order.value.id,
      action: "reject",
      operator_user_id: "",
      operator_name: String(order.value.rejected_by || "").trim() || "系统",
      comment: String(order.value.rejected_reason || "").trim(),
      created_at: order.value.rejected_at,
    });
  }
  return fallbackRecords;
});
const canSubmitApproval = computed(
  () =>
    Boolean(order.value?.approval_required) &&
    currentBusinessStatus.value === "pending_approval" &&
    isCurrentUserCreator.value,
);
const canApproveOrder = computed(
  () =>
    Boolean(order.value?.approval_required) &&
    ["pending_approval", "approving"].includes(currentBusinessStatus.value) &&
    (isCurrentUserApprover.value || authStore.isAdmin),
);
const canRejectOrder = computed(
  () =>
    Boolean(order.value?.approval_required) &&
    ["pending_approval", "approving"].includes(currentBusinessStatus.value) &&
    (isCurrentUserApprover.value || authStore.isAdmin),
);
const canCancel = computed(
  () =>
    Boolean(order.value) &&
    (authStore.isAdmin || isCurrentUserCreator.value) &&
    (currentBusinessStatus.value === "pending_execution" ||
      currentBusinessStatus.value === "building" ||
      currentBusinessStatus.value === "built_waiting_deploy" ||
      currentBusinessStatus.value === "pending_approval" ||
      currentBusinessStatus.value === "approving" ||
      currentBusinessStatus.value === "approved" ||
      currentBusinessStatus.value === "queued" ||
      currentBusinessStatus.value === "deploying"),
);
const precheckBlocked = computed(
  () => Boolean(precheck.value) && !precheck.value?.executable,
);
const canExecute = computed(
  () =>
    canExecutePermission.value &&
    (currentBusinessStatus.value === "pending_execution" ||
      currentBusinessStatus.value === "approved") &&
    !executeLocked.value &&
    !precheckBlocked.value,
);
const supportsStagedDispatch = computed(() => {
  if (!order.value) {
    return false;
  }
  const hasCI =
    Boolean(order.value.has_ci_execution) ||
    Boolean(executions.value.find((item) => item.pipeline_scope === "ci"));
  const hasCD =
    Boolean(order.value.has_cd_execution) ||
    Boolean(executions.value.find((item) => item.pipeline_scope === "cd"));
  return (
    String(order.value.operation_type || "").trim() === "deploy" &&
    hasCI &&
    hasCD
  );
});
const canBuild = computed(
  () =>
    supportsStagedDispatch.value &&
    canExecutePermission.value &&
    (currentBusinessStatus.value === "pending_execution" ||
      currentBusinessStatus.value === "approved") &&
    !executeLocked.value,
);
const canDeploy = computed(
  () =>
    supportsStagedDispatch.value &&
    canExecutePermission.value &&
    currentBusinessStatus.value === "built_waiting_deploy" &&
    !executeLocked.value,
);
const canRollback = computed(
  () => canTriggerArgoReplay.value && canReplayPermission.value,
);
const canReplay = computed(
  () => canTriggerStandardReplay.value && canReplayPermission.value,
);
const canReplayPermission = computed(
  () =>
    Boolean(order.value) &&
    authStore.hasApplicationPermission(
      "release.create",
      order.value?.application_id || "",
      order.value?.env_code || "",
    ),
);
const canTriggerArgoReplay = computed(
  () =>
    Boolean(order.value) &&
    ["deploying", "deploy_failed", "deploy_success"].includes(
      currentBusinessStatus.value,
    ) &&
    String(order.value?.cd_provider || "")
      .trim()
      .toLowerCase() === "argocd",
);
const canTriggerStandardReplay = computed(
  () =>
    Boolean(order.value) &&
    currentBusinessStatus.value === "deploy_success" &&
    String(order.value?.cd_provider || "")
      .trim()
      .toLowerCase() !== "argocd",
);
const shouldAutoRefresh = computed(() => {
  if (!order.value) {
    return true;
  }
  return [
    "pending_execution",
    "pending_approval",
    "approving",
    "building",
    "queued",
    "deploying",
    "approved",
  ].includes(currentBusinessStatus.value);
});
const shouldKeepLogStreaming = computed(() => {
  if (!order.value) {
    return true;
  }
  return ["building", "queued", "deploying"].includes(currentBusinessStatus.value);
});
const shouldLoadPrecheck = computed(() => {
  if (!order.value) {
    return false;
  }
  return ["pending_execution", "built_waiting_deploy", "queued", "deploying"].includes(
    currentBusinessStatus.value,
  );
});
const showPrecheckCard = computed(() => {
  if (precheckLoading.value) {
    return true;
  }
  if (
    currentBusinessStatus.value === "pending_execution" ||
    currentBusinessStatus.value === "built_waiting_deploy"
  ) {
    return true;
  }
  return Boolean(
    (currentBusinessStatus.value === "queued" ||
      currentBusinessStatus.value === "deploying") &&
      precheck.value?.waiting_for_lock,
  );
});

const executionMapByScope = computed<
  Record<ReleasePipelineScope, ReleaseOrderExecution | null>
>(() => ({
  ci: executions.value.find((item) => item.pipeline_scope === "ci") || null,
  cd: executions.value.find((item) => item.pipeline_scope === "cd") || null,
}));

const visibleScopes = computed(() => {
  return (["ci", "cd"] as ReleasePipelineScope[]).filter((scope) =>
    Boolean(executionMapByScope.value[scope]),
  );
});

const detailItems = computed(() => {
  if (!order.value) {
    return [];
  }
  const items = [
    { key: "order_no", label: "发布单号", value: order.value.order_no },
    { key: "release_name", label: "发布名称", value: order.value.release_name || "-" },
    {
      key: "created_at",
      label: "创建时间",
      value: formatTime(order.value.created_at),
    },
    {
      key: "operation_type",
      label: "操作类型",
      value: operationTypeText(order.value.operation_type),
    },
    { label: "应用名称", value: order.value.application_name || "-" },
    { label: "模板名称", value: order.value.template_name || "-" },
    { label: "模板 ID", value: order.value.template_id || "-" },
    { label: "触发方式", value: triggerTypeText(order.value.trigger_type) },
    { label: "创建者", value: order.value.triggered_by || "-" },
    { label: "Git 版本", value: order.value.git_ref || "-" },
    { label: "镜像版本", value: order.value.image_tag || "-" },
    { label: "备注", value: order.value.remark || "-" },
    { label: "开始时间", value: formatTime(order.value.started_at) },
    { label: "结束时间", value: formatTime(order.value.finished_at) },
    { label: "更新时间", value: formatTime(order.value.updated_at) },
  ];
  if (order.value.operation_type !== "deploy" && order.value.source_order_no) {
    items.splice(3, 0, {
      key: "source_order_no",
      label: "来源发布单号",
      value: order.value.source_order_no,
    });
  }
  return items;
});

const heroFacts = computed(() => {
  if (!order.value) {
    return [];
  }
  return [
    { label: "应用", value: order.value.application_name || "-" },
    { label: "环境", value: order.value.env_code || "-" },
    { label: "Git 版本", value: order.value.git_ref || "-" },
  ];
});

const showConcurrentBatchCard = computed(() =>
  Boolean(order.value?.is_concurrent && order.value?.concurrent_batch_no),
);

const concurrentBatchSummary = computed(() => {
  const progress = concurrentBatchProgress.value;
  if (!progress) {
    return [];
  }
  return [
    { label: "总单数", value: progress.total },
    { label: "等待中", value: progress.queued },
    { label: "执行中", value: progress.executing },
    { label: "已完成", value: progress.success },
    { label: "失败", value: progress.failed },
  ];
});

const currentConcurrentBatchItem = computed(() => {
  if (!order.value?.id || !concurrentBatchProgress.value?.items?.length) {
    return null;
  }
  return (
    concurrentBatchProgress.value.items.find(
      (item) => item.order_id === order.value?.id,
    ) || null
  );
});

const isQueuedInConcurrentBatch = computed(
  () =>
    currentConcurrentBatchItem.value?.queue_state === "queued" ||
    Boolean(
      order.value?.status === "running" && precheck.value?.waiting_for_lock,
    ),
);

const spotlightStep = computed(() => {
  const failedSteps = [...steps.value]
    .filter((item) => item.status === "failed")
    .sort(sortSteps);
  if (failedSteps.length > 0) {
    return failedSteps[failedSteps.length - 1];
  }
  const runningSteps = [...steps.value]
    .filter((item) => item.status === "running")
    .sort(sortSteps);
  if (runningSteps.length > 0) {
    return runningSteps[runningSteps.length - 1];
  }
  const successSteps = [...steps.value]
    .filter((item) => item.status === "success")
    .sort(sortSteps);
  if (successSteps.length > 0) {
    return successSteps[successSteps.length - 1];
  }
  return null;
});

const spotlightTone = computed<"error" | "processing" | "success" | "warning">(
  () => {
    if (!order.value) {
      return "warning";
    }
    if (currentBusinessStatus.value === "queued" || isQueuedInConcurrentBatch.value) {
      return "warning";
    }
    switch (currentBusinessStatus.value) {
      case "deploy_failed":
        return "error";
      case "building":
      case "deploying":
        return "processing";
      case "deploy_success":
        return "success";
      default:
        return "warning";
    }
  },
);

const spotlightStatusKey = computed<
  "failed" | "running" | "queued" | "success" | "cancelled" | "pending"
>(() => {
  if (!order.value) {
    return "pending";
  }
  if (currentBusinessStatus.value === "queued" || isQueuedInConcurrentBatch.value) {
    return "queued";
  }
  switch (currentBusinessStatus.value) {
    case "deploy_failed":
      return "failed";
    case "building":
    case "deploying":
      return "running";
    case "deploy_success":
      return "success";
    case "cancelled":
      return "cancelled";
    case "built_waiting_deploy":
      return "queued";
    default:
      return "pending";
  }
});

const spotlightMeta = computed(() => {
  if (isQueuedInConcurrentBatch.value) {
    const queuePosition = currentConcurrentBatchItem.value?.queue_position || 0;
    if (queuePosition > 0) {
      return `并发批次等待队列 · 当前位次 ${queuePosition}`;
    }
    return "并发批次等待队列 · 等待前序发布释放执行位";
  }
  const step = spotlightStep.value;
  if (step) {
    return `${step.step_name} · ${statusText(step.status)}`;
  }
  if (!order.value) {
    return "等待获取发布详情";
  }
  return `发布单状态 · ${statusText(currentBusinessStatus.value)}`;
});

const spotlightTitle = computed(() => {
  if (!order.value) {
    return "等待加载发布状态";
  }
  if (currentBusinessStatus.value === "queued" || isQueuedInConcurrentBatch.value) {
    return "发布排队中，等待前序发布完成";
  }
  if (currentBusinessStatus.value === "deploy_failed") {
    return "发布失败，需要人工介入";
  }
  if (currentBusinessStatus.value === "building") {
    return "构建执行中";
  }
  if (currentBusinessStatus.value === "built_waiting_deploy") {
    return "构建已完成，等待部署";
  }
  if (currentBusinessStatus.value === "deploying") {
    return "发布执行中";
  }
  if (currentBusinessStatus.value === "deploy_success") {
    return "发布已完成";
  }
  if (currentBusinessStatus.value === "cancelled") {
    return "发布已取消";
  }
  return "发布待执行";
});

const spotlightDescription = computed(() => {
  if (isQueuedInConcurrentBatch.value) {
    const queuePosition = currentConcurrentBatchItem.value?.queue_position || 0;
    if (queuePosition > 0) {
      return `当前发布单已通过预检，正在并发批次队列中等待执行，队列位次 ${queuePosition}`;
    }
    return (
      precheck.value?.conflict_message ||
      "当前发布单已通过预检，正在等待同应用同环境的前序发布执行完成"
    );
  }
  const step = spotlightStep.value;
  if (step) {
    const messageText = String(step.message || "").trim();
    if (messageText) {
      return `${step.step_name}：${messageText}`;
    }
    return `${step.step_name}：${statusText(step.status)}`;
  }
  if (!order.value) {
    return "正在加载发布详情";
  }
  return `当前状态：${statusText(currentBusinessStatus.value)}`;
});

const executionSections = computed(() =>
  visibleScopes.value.map((scope) => ({
    scope,
    title: `${scopeLabel(scope)} 执行单元`,
    execution: executionMapByScope.value[scope] as ReleaseOrderExecution,
  })),
);

function executionSortNoForScope(scope: ReleasePipelineScope) {
  const scopedSteps = steps.value
    .filter(
      (item) =>
        String(item.step_scope || "")
          .trim()
          .toLowerCase() === scope,
    )
    .sort(sortSteps);
  if (scopedSteps.length > 0) {
    return scopedSteps[0].sort_no;
  }
  const scopeIndex = visibleScopes.value.indexOf(scope);
  return scopeIndex >= 0 ? (scopeIndex + 1) * 1000 : 9999;
}

const paramGroups = computed(() => {
  const map: Record<ReleasePipelineScope, ReleaseOrderParam[]> = {
    ci: [],
    cd: [],
  };
  params.value.forEach((item) => {
    const scope = normalizeScope(item.pipeline_scope);
    if (!scope) {
      return;
    }
    map[scope].push(item);
  });
  return visibleScopes.value.map((scope) => ({
    scope,
    title: `${scopeLabel(scope)} 参数快照`,
    items: map[scope],
  }));
});

const valueProgressGroups = computed(() => {
  const map: Record<ReleasePipelineScope, ReleaseOrderValueProgress[]> = {
    ci: [],
    cd: [],
  };
  valueProgress.value.forEach((item) => {
    const scope = normalizeScope(item.pipeline_scope);
    if (!scope) {
      return;
    }
    map[scope].push(item);
  });
  return visibleScopes.value
    .map((scope) => ({
      scope,
      items: map[scope].sort((a, b) => a.sort_no - b.sort_no),
    }))
    .filter((group) => group.items.length > 0);
});

const valueProgressTotal = computed(() =>
  valueProgressGroups.value.reduce((total, group) => total + group.items.length, 0),
);

const stepGroups = computed(() => {
  const groups: Array<{
    key: string;
    title: string;
    items: ReleaseOrderStep[];
  }> = [];
  const globalSteps = steps.value
    .filter(
      (item) =>
        String(item.step_scope || "")
          .trim()
          .toLowerCase() === "global",
    )
    .sort(sortSteps);
  if (globalSteps.length > 0) {
    groups.push({ key: "global", title: "全局步骤", items: globalSteps });
  }
  visibleScopes.value.forEach((scope) => {
    const items = steps.value
      .filter(
        (item) =>
          String(item.step_scope || "")
            .trim()
            .toLowerCase() === scope,
      )
      .sort(sortSteps);
    if (items.length > 0) {
      groups.push({ key: scope, title: `${scopeLabel(scope)} 步骤`, items });
    }
  });
  return groups;
});

function sanitizeHookMessage(messageText: string) {
  const sanitized = String(messageText || "")
    .replace(/(?:^|[，,\s])(?:task_id|source_task_id|batch_id)=[A-Za-z0-9-]+/g, "")
    .replace(/[，,\s]{2,}/g, " ")
    .replace(/[，,]\s*[，,]/g, "，")
    .trim()
    .replace(/[，,]\s*$/, "");
  return sanitized;
}

function hookExecutionSummaryText(item: ReleaseOrderStep) {
  const taskSummaryText = String(item.related_task_summary || "").trim();
  if (taskSummaryText) {
    return taskSummaryText;
  }
  const messageText = sanitizeHookMessage(String(item.message || "").trim());
  if (messageText) {
    return messageText;
  }
  if (item.status === "pending") {
    return "等待主发布流程结束后触发当前 Hook";
  }
  return "当前 Hook 暂无补充执行内容";
}

function hookExecutionLogText(item: ReleaseOrderStep) {
  const detailLogText = String(item.detail_log || "").trim();
  if (detailLogText) {
    return detailLogText;
  }
  const taskSummaryText = String(item.related_task_summary || "").trim();
  if (taskSummaryText) {
    return taskSummaryText;
  }
  const messageText = sanitizeHookMessage(String(item.message || "").trim());
  if (messageText) {
    return messageText;
  }
  if (item.status === "pending") {
    return "等待主发布流程结束后触发当前 Hook";
  }
  return "当前 Hook 暂无补充执行内容";
}

const hookProgressItems = computed(() => {
  const allHookSteps = steps.value
    .filter((item) =>
      String(item.step_code || "")
        .trim()
        .toLowerCase()
        .startsWith("hook:"),
    )
    .sort(sortSteps);
  const businessHookSteps = allHookSteps.filter((item) => {
    const code = String(item.step_code || "")
      .trim()
      .toLowerCase();
    return code !== "hook:prepare" && code !== "hook:summary";
  });
  const items = businessHookSteps.length > 0 ? businessHookSteps : allHookSteps;

  // 过滤掉因环境不匹配被跳过的 hook
  return items
    .filter((item) => {
      const message = String(item.message || "").trim();
      const status = String(item.status || "").trim();
      // 如果 message 包含"未命中 Hook 执行环境"或"已按环境条件跳过"，说明是环境跳过
      if (message.includes("未命中 Hook 执行环境") || message.includes("已按环境条件跳过")) {
        return false;
      }
      return true;
    })
    .map(
      (item, index) => ({
        ...item,
        order_index: index + 1,
        summary: hookExecutionLogText(item),
      }),
    );
});

type HookProgressType = "agent_task" | "notification_hook" | "webhook_notification" | "generic";
type HookExecuteStage = "build_complete" | "post_release";

function hookExecuteStageFromStepCode(stepCode: string): HookExecuteStage {
  const parts = String(stepCode || "")
    .trim()
    .toLowerCase()
    .split(":");
  if (parts.length >= 4 && parts[1] === "build_complete") {
    return "build_complete";
  }
  return "post_release";
}

function hookExecuteStageText(stage: HookExecuteStage) {
  return stage === "build_complete" ? "构建完成" : "发布完成";
}

function inferHookProgressType(item: ReleaseOrderStep): HookProgressType {
  const haystack = [
    item.step_code,
    item.step_name,
    item.message,
  ]
    .map((value) =>
      String(value || "")
        .trim()
        .toLowerCase(),
    )
    .join(" ");
  if (haystack.includes("webhook")) {
    return "webhook_notification";
  }
  if (haystack.includes("notification_hook") || haystack.includes("通知 hook") || haystack.includes("通知hook") || haystack.includes("notification")) {
    return "notification_hook";
  }
  if (haystack.includes("agent")) {
    return "agent_task";
  }
  return "generic";
}

function hookProgressTypeText(type: HookProgressType) {
  switch (type) {
    case "agent_task":
      return "Agent 任务";
    case "notification_hook":
      return "通知 Hook";
    case "webhook_notification":
      return "Webhook 通知";
    default:
      return "通用 Hook";
  }
}

function hookExecutionUnitDisplayTitle(
  stage: HookExecuteStage,
  type: HookProgressType,
) {
  return `${hookExecuteStageText(stage)} · ${hookProgressTypeText(type)}`;
}

function hookExecutionContentText(item: ReleaseOrderStep) {
  return hookExecutionSummaryText(item);
}

function hookGroupSummaryText(group: {
  summary: {
    total: number;
    pending: number;
    running: number;
    success: number;
    failed: number;
  };
}) {
  const parts: string[] = [];
  if (group.summary.running > 0) {
    parts.push(`${group.summary.running} 个执行中`);
  }
  if (group.summary.success > 0) {
    parts.push(`${group.summary.success} 个成功`);
  }
  if (group.summary.failed > 0) {
    parts.push(`${group.summary.failed} 个失败`);
  }
  if (group.summary.pending > 0) {
    parts.push(`${group.summary.pending} 个待执行`);
  }
  return `${group.summary.total} 个 Hook${
    parts.length > 0 ? ` · ${parts.join(" · ")}` : ""
  }`;
}

type HookProgressGroup = {
  key: string;
  type: HookProgressType;
  stage: HookExecuteStage;
  sortNo: number;
  title: string;
  items: Array<
    ReleaseOrderStep & {
      order_index: number;
      summary: string;
    }
  >;
  summary: {
    total: number;
    pending: number;
    running: number;
    success: number;
    failed: number;
  };
  overallStatus: "pending" | "running" | "success" | "failed";
};

const hookProgressGroups = computed<HookProgressGroup[]>(() => {
  const grouped = new Map<
    string,
    HookProgressGroup
  >();

  hookProgressItems.value.forEach((item) => {
    const type = inferHookProgressType(item);
    const stage = hookExecuteStageFromStepCode(item.step_code);
    const key = `${stage}:${type}`;
    if (!grouped.has(key)) {
      grouped.set(key, {
        key,
        type,
        stage,
        sortNo: item.sort_no,
        title: hookExecutionUnitDisplayTitle(stage, type),
        items: [],
        summary: {
          total: 0,
          pending: 0,
          running: 0,
          success: 0,
          failed: 0,
        },
        overallStatus: "pending",
      });
    }
    const group = grouped.get(key)!;
    group.items.push(item);
    group.sortNo = Math.min(group.sortNo, item.sort_no);
    group.summary.total += 1;
    switch (item.status) {
      case "running":
        group.summary.running += 1;
        break;
      case "success":
        group.summary.success += 1;
        break;
      case "failed":
        group.summary.failed += 1;
        break;
      default:
        group.summary.pending += 1;
        break;
    }
  });

  return Array.from(grouped.values()).map((group) => {
    if (group.summary.failed > 0) {
      group.overallStatus = "failed";
    } else if (group.summary.running > 0) {
      group.overallStatus = "running";
    } else if (group.summary.success > 0 && group.summary.pending === 0) {
      group.overallStatus = "success";
    } else {
      group.overallStatus = "pending";
    }
    return group;
  }).sort((a, b) => a.sortNo - b.sortNo || a.key.localeCompare(b.key));
});

type ExecutionUnitItem =
  | {
      kind: "pipeline";
      key: string;
      sortNo: number;
      scope: ReleasePipelineScope;
      title: string;
      execution: ReleaseOrderExecution;
    }
  | {
      kind: "hook";
      key: string;
      sortNo: number;
      group: HookProgressGroup;
    };

const executionUnitItems = computed<ExecutionUnitItem[]>(() => {
  const pipelineItems = executionSections.value.map((item) => ({
    kind: "pipeline" as const,
    key: `pipeline:${item.scope}`,
    sortNo: executionSortNoForScope(item.scope),
    scope: item.scope,
    title: item.title,
    execution: item.execution,
  }));
  const hookItems = hookProgressGroups.value.map((group) => ({
    kind: "hook" as const,
    key: `hook:${group.key}`,
    sortNo: group.sortNo,
    group,
  }));
  return [...pipelineItems, ...hookItems].sort(
    (a, b) => a.sortNo - b.sortNo || a.key.localeCompare(b.key),
  );
});

function hookReferenceIDs(item: ReleaseOrderStep) {
  const refs = new Set<string>();
  (item.related_task_ids || []).forEach((value) => {
    const trimmed = String(value || "").trim();
    if (trimmed) {
      refs.add(trimmed);
    }
  });
  const haystack = [
    item.message,
    item.detail_log,
    item.related_task_summary,
  ].join(" ");
  const pattern = /\b(?:task_id|source_task_id|batch_id)=([A-Za-z0-9-]+)/g;
  for (const match of haystack.matchAll(pattern)) {
    if (match[1]) {
      refs.add(match[1]);
    }
  }
  return Array.from(refs);
}

function hookTaskReferenceText(item: ReleaseOrderStep) {
  const refs = hookReferenceIDs(item);
  if (refs.length > 0) {
    return refs.join(" / ");
  }
  if (item.related_task_count > 0) {
    return `${item.related_task_count} 个任务`;
  }
  return "-";
}

function hookTaskStageTypeText(item: ReleaseOrderStep) {
  return `${hookExecuteStageText(
    hookExecuteStageFromStepCode(item.step_code),
  )} / ${hookProgressTypeText(inferHookProgressType(item))}`;
}

const stageGroupsByScope = computed<
  Record<ReleasePipelineScope, ReleaseOrderPipelineStage[]>
>(() => {
  const map: Record<ReleasePipelineScope, ReleaseOrderPipelineStage[]> = {
    ci: [],
    cd: [],
  };
  pipelineStages.value.forEach((item) => {
    const scope = normalizeScope(item.pipeline_scope);
    if (!scope) {
      return;
    }
    map[scope].push(item);
  });
  map.ci.sort((a, b) => a.sort_no - b.sort_no);
  map.cd.sort((a, b) => a.sort_no - b.sort_no);
  return map;
});

const stageSections = computed(() =>
  visibleScopes.value.map((scope) => {
    const execution = executionMapByScope.value[scope];
    return {
      scope,
      title: `${scopeLabel(scope)} 管线进度`,
      execution,
      stages: stageGroupsByScope.value[scope],
      isJenkins: execution?.provider === "jenkins",
      isArgoCD: execution?.provider === "argocd",
    };
  }),
);

const logSections = computed(() =>
  visibleScopes.value.map((scope) => {
    const execution = executionMapByScope.value[scope];
    return {
      scope,
      title: `${scopeLabel(scope)} 日志`,
      execution,
      isJenkins: execution?.provider === "jenkins",
      state: scopeLogStates[scope],
    };
  }),
);

const paramInitialColumns: TableColumnsType<ReleaseOrderParam> = [
  {
    title: "平台标准 Key",
    dataIndex: "param_key",
    key: "param_key",
    width: 180,
  },
  {
    title: "执行器参数名",
    dataIndex: "executor_param_name",
    key: "executor_param_name",
    width: 220,
  },
  {
    title: "参数值",
    dataIndex: "param_value",
    key: "param_value",
    width: 300,
    ellipsis: true,
  },
  { title: "来源", dataIndex: "value_source", key: "value_source", width: 150 },
  { title: "创建时间", dataIndex: "created_at", key: "created_at", width: 190 },
];
const { columns: paramColumns } = useResizableColumns(paramInitialColumns, {
  minWidth: 100,
  maxWidth: 620,
  hitArea: 10,
});

function normalizeScope(scope: string): ReleasePipelineScope | null {
  const value = String(scope || "")
    .trim()
    .toLowerCase();
  if (value === "ci" || value === "cd") {
    return value as ReleasePipelineScope;
  }
  return null;
}

function scopeLabel(scope: ReleasePipelineScope) {
  return scope === "ci" ? "CI" : "CD";
}

function formatTime(value: string | null) {
  if (!value) {
    return "-";
  }
  return dayjs(value).format("YYYY-MM-DD HH:mm:ss");
}

function formatTimeCompact(value: string | null) {
  if (!value) {
    return "";
  }
  return dayjs(value).format("MM-DD HH:mm:ss");
}

function statusText(
  status:
    | ReleaseOrderBusinessStatus
    | ReleaseOrderStatus
    | ReleaseOrderStep["status"]
    | ReleasePipelineStageStatus
    | ReleaseOrderExecution["status"],
) {
  switch (status) {
    case "draft":
      return "草稿";
    case "pending_execution":
      return "待执行";
    case "pending_approval":
      return "待审批";
    case "approving":
      return "审批中";
    case "approved":
      return "已批准";
    case "building":
      return "构建中";
    case "built_waiting_deploy":
      return "已构建待部署";
    case "rejected":
      return "审批拒绝";
    case "queued":
      return "排队中";
    case "deploying":
      return "发布中";
    case "deploy_success":
      return "发布成功";
    case "deploy_failed":
      return "发布失败";
    case "pending":
      return "待执行";
    case "running":
      return "执行中";
    case "success":
      return "成功";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    case "skipped":
      return "已跳过";
    default:
      return status;
  }
}

function statusToneClass(
  status:
    | ReleaseOrderBusinessStatus
    | ReleaseOrderStatus
    | ReleaseOrderStep["status"]
    | ReleasePipelineStageStatus
    | ReleaseOrderExecution["status"],
) {
  switch (status) {
    case "deploy_success":
    case "success":
      return "status-pill-success";
    case "deploy_failed":
    case "rejected":
    case "failed":
      return "status-pill-failed";
    case "building":
      return "status-pill-running";
    case "deploying":
    case "approving":
    case "running":
      return "status-pill-running";
    case "queued":
    case "built_waiting_deploy":
    case "approved":
    case "pending_approval":
      return "status-pill-warning";
    case "cancelled":
      return "status-pill-neutral";
    case "skipped":
      return "status-pill-neutral";
    default:
      return "status-pill-pending";
  }
}

function valueProgressStatusText(status: ReleaseOrderValueProgressStatus) {
  switch (status) {
    case "resolved":
      return "已取值";
    case "running":
      return "取值中";
    case "failed":
      return "取值失败";
    case "skipped":
      return "未取值";
    default:
      return "等待取值";
  }
}

function valueProgressToneClass(status: ReleaseOrderValueProgressStatus) {
  switch (status) {
    case "resolved":
      return "status-pill-success";
    case "running":
      return "status-pill-running";
    case "failed":
      return "status-pill-failed";
    case "skipped":
      return "status-pill-neutral";
    default:
      return "status-pill-pending";
  }
}

function precheckStatusText(status: ReleaseOrderPrecheckItem["status"]) {
  switch (status) {
    case "pass":
      return "通过";
    case "warn":
      return "等待";
    case "blocked":
      return "阻塞";
    default:
      return status;
  }
}

function precheckToneClass(status: ReleaseOrderPrecheckItem["status"]) {
  switch (status) {
    case "pass":
      return "status-pill-success";
    case "warn":
      return "status-pill-running";
    case "blocked":
      return "status-pill-failed";
    default:
      return "status-pill-pending";
  }
}

function concurrentQueueStateText(
  state: ReleaseOrderConcurrentBatchQueueState,
) {
  switch (state) {
    case "queued":
      return "排队中";
    case "executing":
      return "执行中";
    case "success":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return "待调度";
  }
}

function concurrentQueueToneClass(
  state: ReleaseOrderConcurrentBatchQueueState,
) {
  switch (state) {
    case "success":
      return "status-pill-success";
    case "failed":
      return "status-pill-failed";
    case "executing":
      return "status-pill-running";
    case "cancelled":
      return "status-pill-neutral";
    case "queued":
      return "status-pill-warning";
    default:
      return "status-pill-pending";
  }
}

function dispatchActionText(action: ReleaseOrderDispatchAction) {
  switch (action) {
    case "build":
      return "仅构建";
    case "deploy":
      return "发布";
    default:
      return "发布";
  }
}

const currentPrecheckAction = computed<ReleaseOrderDispatchAction>(() => {
  if (currentBusinessStatus.value === "built_waiting_deploy") {
    return "deploy";
  }
  return "execute";
});

const precheckSummaryMessage = computed(() => {
  if (!precheck.value) {
    return "";
  }
  const actionText = dispatchActionText(currentPrecheckAction.value);
  if (!precheck.value.executable && precheck.value.ahead_count > 0) {
    return (
      precheck.value.conflict_message ||
      `当前应用前面还有 ${precheck.value.ahead_count} 单，请等待先前执行单结束后再点击${actionText}`
    );
  }
  if (precheck.value.waiting_for_lock) {
    return (
      precheck.value.conflict_message ||
      `当前目标已被其他发布占用，系统会在锁释放后继续${actionText}`
    );
  }
  if (precheckBlocked.value) {
    return (
      precheck.value.conflict_message ||
      `当前发布单未通过${actionText}前预检，请先处理阻塞项`
    );
  }
  if (precheck.value.lock_enabled) {
    return `并发发布保护已启用，当前按 ${precheck.value.lock_scope || "application_env"} 范围进行调度控制`;
  }
  return "";
});

const precheckSummaryTone = computed<"info" | "warning" | "error">(() => {
  if (precheck.value?.waiting_for_lock) {
    return "warning";
  }
  if (precheckBlocked.value) {
    return "error";
  }
  return "info";
});

const precheckCardTitle = computed(() => `${dispatchActionText(currentPrecheckAction.value)}前预检`);

function triggerTypeText(
  triggerType: ReleaseTriggerType | "" | null | undefined,
) {
  switch (
    String(triggerType || "")
      .trim()
      .toLowerCase()
  ) {
    case "manual":
      return "手动";
    case "webhook":
      return "Webhook";
    case "schedule":
      return "定时";
    default:
      return triggerType || "-";
  }
}

function operationTypeText(
  operationType: ReleaseOperationType | "" | null | undefined,
) {
  switch (
    String(operationType || "")
      .trim()
      .toLowerCase()
  ) {
    case "rollback":
      return "标准回滚";
    case "replay":
      return "标准重放";
    default:
      return "普通发布";
  }
}

function isCiOnlyRecovery(record?: ReleaseOrder | null) {
  return String(record?.cd_provider || "").trim() === "";
}

function replayActionText(record?: ReleaseOrder | null) {
  return "一键重发";
}

function replayConfirmTitle(record?: ReleaseOrder | null) {
  return isCiOnlyRecovery(record)
    ? "确认创建 CI 标准重放单吗？"
    : "确认创建标准重放单吗？";
}

function replaySuccessText(record: ReleaseOrder, orderNo: string) {
  return isCiOnlyRecovery(record)
    ? `已创建 CI 标准重放单：${orderNo}`
    : `已创建标准重放单：${orderNo}`;
}

function replayFailureText(record?: ReleaseOrder | null) {
  return isCiOnlyRecovery(record) ? "CI 标准重放创建失败" : "标准重放创建失败";
}

function isRunningStatus(
  status:
    | ReleaseOrderStatus
    | ReleaseOrderStep["status"]
    | ReleasePipelineStageStatus
    | ReleaseOrderExecution["status"],
) {
  return status === "running";
}

function formatDuration(durationMillis: number) {
  const value = Number(durationMillis || 0);
  if (!Number.isFinite(value) || value <= 0) {
    return "-";
  }
  if (value < 1000) {
    return `${Math.floor(value)} ms`;
  }
  const totalSeconds = Math.floor(value / 1000);
  if (totalSeconds < 60) {
    return `${totalSeconds} s`;
  }
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}m ${seconds}s`;
}

function sortSteps(a: ReleaseOrderStep, b: ReleaseOrderStep) {
  if (a.sort_no !== b.sort_no) {
    return a.sort_no - b.sort_no;
  }
  return a.step_code.localeCompare(b.step_code);
}

function stepComponentStatus(status: ReleaseOrderStep["status"]) {
  switch (status) {
    case "success":
      return "finish";
    case "running":
      return "process";
    case "failed":
      return "error";
    default:
      return "wait";
  }
}

function describeStep(step: ReleaseOrderStep) {
  const parts: string[] = [];
  if (String(step.message || "").trim()) {
    parts.push(step.message);
  } else if (step.status === "pending") {
    parts.push("等待执行");
  }
  const timeParts = [
    formatTimeCompact(step.started_at),
    formatTimeCompact(step.finished_at),
  ].filter(Boolean);
  if (timeParts.length > 0) {
    parts.push(timeParts.join(" -> "));
  }
  return parts.join(" ｜ ");
}

function hookStatusText(status: ReleaseOrderStep["status"]) {
  switch (status) {
    case "pending":
      return "待执行";
    case "running":
      return "执行中";
    case "success":
      return "成功";
    case "failed":
      return "失败";
    default:
      return status;
  }
}

function approvalModeText(mode: string | null | undefined) {
  return String(mode || "").trim().toLowerCase() === "all" ? "会签" : "或签";
}

function approvalActionText(action: ReleaseOrderApprovalRecord["action"]) {
  switch (action) {
    case "submit":
      return "提交审批";
    case "approve":
      return "审批通过";
    case "reject":
      return "审批拒绝";
    default:
      return action;
  }
}

const approvalStatusSummary = computed(() => {
  if (!order.value?.approval_required) {
    return "当前模板未启用审批流";
  }
  switch (currentBusinessStatus.value) {
    case "pending_approval":
      return "当前发布单待审批，发起人可提交审批，审批人也可以直接处理";
    case "approving":
      return "当前发布单正在审批中，审批通过后才允许触发发布";
    case "approved":
      return order.value.approved_by
        ? `审批已通过，最后确认人：${order.value.approved_by}`
        : "审批已通过，可继续触发发布";
    case "building":
      return "审批已通过，当前处于构建阶段";
    case "built_waiting_deploy":
      return "审批与构建均已完成，等待手动触发部署";
    case "rejected":
      return order.value.rejected_reason
        ? `审批已拒绝：${order.value.rejected_reason}`
        : "审批已拒绝，当前发布单不能继续执行";
    default:
      return "审批流程已完成，可在此查看审批记录与审批人";
  }
});

const approvalActionModalTitle = computed(() => {
  switch (approvalActionMode.value) {
    case "submit":
      return "提交审批";
    case "approve":
      return "审批通过";
    case "reject":
      return "审批拒绝";
    default:
      return "审批操作";
  }
});

const approvalActionModalOkText = computed(() => {
  switch (approvalActionMode.value) {
    case "submit":
      return "提交";
    case "approve":
      return "通过";
    case "reject":
      return "拒绝";
    default:
      return "确认";
  }
});

const approvalActionPlaceholder = computed(() => {
  switch (approvalActionMode.value) {
    case "submit":
      return "可选填写审批备注，帮助审批人理解本次发布背景";
    case "approve":
      return "可选填写审批意见，例如放行原因、观察项";
    case "reject":
      return "请填写拒绝原因，便于发起人修正发布内容";
    default:
      return "请输入审批备注";
  }
});

function hookToneClass(status: ReleaseOrderStep["status"] | "skipped") {
  switch (status) {
    case "success":
      return "status-pill-success";
    case "running":
      return "status-pill-running";
    case "failed":
      return "status-pill-failed";
    case "skipped":
      return "status-pill-neutral";
    default:
      return "status-pill-pending";
  }
}

function toggleHookTasks(key: string) {
  expandedHookTaskMap[key] = !expandedHookTaskMap[key];
}

function toggleHookLogs(id: string) {
  expandedHookLogMap[id] = !expandedHookLogMap[id];
}

function openApprovalActionModal(mode: "submit" | "approve" | "reject") {
  approvalActionMode.value = mode;
  approvalActionComment.value = "";
  approvalActionModalVisible.value = true;
}

function closeApprovalActionModal() {
  approvalActionModalVisible.value = false;
  approvalActionComment.value = "";
}

function latestScopeStepMessage(
  scope: ReleasePipelineScope,
  preferredStatus?: ReleaseOrderStep["status"],
) {
  const scopedSteps = steps.value
    .filter(
      (item) =>
        String(item.step_scope || "")
          .trim()
          .toLowerCase() === scope,
    )
    .sort(sortSteps);

  const candidates = preferredStatus
    ? scopedSteps.filter((item) => item.status === preferredStatus)
    : scopedSteps;

  for (let index = candidates.length - 1; index >= 0; index -= 1) {
    const messageText = String(candidates[index].message || "").trim();
    if (messageText) {
      return messageText;
    }
  }
  return "";
}

function pipelineStageEmptyDescription(section: {
  scope: ReleasePipelineScope;
  execution: ReleaseOrderExecution | null;
  isArgoCD: boolean;
  isJenkins: boolean;
}) {
  if (!section.execution) {
    return "暂无阶段数据";
  }

  if (section.isArgoCD) {
    const failedMessage = latestScopeStepMessage(section.scope, "failed");
    switch (section.execution.status) {
      case "failed":
        return (
          failedMessage ||
          "CD 在启动阶段失败，尚未生成 GitOps / ArgoCD 聚合进度"
        );
      case "pending":
        return "CD 尚未启动，待前置步骤完成后会自动生成 GitOps / ArgoCD 进度";
      case "running":
        return "GitOps 写回 / ArgoCD Sync 进度正在回收中，请稍后自动刷新";
      case "success":
        return "CD 已完成，但当前没有额外的聚合阶段数据";
      case "cancelled":
        return failedMessage || "CD 已取消，未生成聚合阶段数据";
      case "skipped":
        return "CD 已跳过，未生成聚合阶段数据";
      default:
        return "暂无阶段数据";
    }
  }

  if (section.isJenkins) {
    return latestScopeStepMessage(section.scope, "failed") || "暂无阶段数据";
  }

  return latestScopeStepMessage(section.scope, "failed") || "暂无阶段数据";
}

function parseStreamEvent(data: string): ReleaseOrderLogStreamEvent | null {
  const text = String(data || "").trim();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text) as ReleaseOrderLogStreamEvent;
  } catch {
    return {
      type: "status",
      timestamp: new Date().toISOString(),
      message: text,
    };
  }
}

function getLogState(scope: ReleasePipelineScope) {
  return scopeLogStates[scope];
}

function setLogPanelRef(scope: ReleasePipelineScope, element: Element | null) {
  getLogState(scope).panelRef = element instanceof HTMLElement ? element : null;
}

function isLogNearBottom(scope: ReleasePipelineScope) {
  const panel = getLogState(scope).panelRef;
  if (!panel) {
    return true;
  }
  const remain = panel.scrollHeight - panel.scrollTop - panel.clientHeight;
  return remain <= 48;
}

function scrollLogToBottom(scope: ReleasePipelineScope, force = false) {
  const state = getLogState(scope);
  if (!state.panelRef) {
    return;
  }
  if (!force && !state.autoFollow) {
    return;
  }
  state.panelRef.scrollTop = state.panelRef.scrollHeight;
}

function syncLogFollowState(scope: ReleasePipelineScope) {
  getLogState(scope).autoFollow = isLogNearBottom(scope);
}

function handleLogFollowChange(scope: ReleasePipelineScope, checked: boolean) {
  const state = getLogState(scope);
  state.autoFollow = checked;
  if (checked) {
    void nextTick(() => {
      scrollLogToBottom(scope, true);
    });
  }
}

function jumpLogToBottom(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  state.autoFollow = true;
  void nextTick(() => {
    scrollLogToBottom(scope, true);
  });
}

function appendLogContent(scope: ReleasePipelineScope, content: string) {
  const state = getLogState(scope);
  const chunk = String(content || "");
  if (!chunk) {
    return;
  }
  state.text = state.text ? state.text + chunk : chunk;
  void nextTick(() => {
    scrollLogToBottom(scope);
  });
}

function appendStatusLine(scope: ReleasePipelineScope, messageText: string) {
  const text = String(messageText || "").trim();
  if (!text) {
    return;
  }
  appendLogContent(scope, `[${dayjs().format("HH:mm:ss")}] ${text}\n`);
}

function clearReconnectTimer(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  if (state.reconnectTimer !== null) {
    window.clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }
}

function closeLogStream(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  clearReconnectTimer(scope);
  if (state.stream) {
    state.closeIntentional = true;
    state.stream.close();
    state.stream = null;
  }
  state.connected = false;
  state.connecting = false;
}

function resetLogState(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  closeLogStream(scope);
  state.text = "";
  state.offset = 0;
  state.connected = false;
  state.connecting = false;
  state.ended = false;
  state.error = "";
  state.statusText = "未连接";
  state.closeIntentional = false;
  state.autoFollow = true;
}

function scheduleReconnect(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  if (state.closeIntentional || state.ended || !shouldKeepLogStreaming.value) {
    return;
  }
  clearReconnectTimer(scope);
  state.reconnectTimer = window.setTimeout(() => {
    void startLogStream(scope, false);
  }, 2000);
}

async function startLogStream(scope: ReleasePipelineScope, reset: boolean) {
  const execution = executionMapByScope.value[scope];
  if (!orderID.value || !execution || execution.provider !== "jenkins") {
    return;
  }

  const state = getLogState(scope);
  closeLogStream(scope);
  state.closeIntentional = false;
  if (reset) {
    state.text = "";
    state.offset = 0;
    state.error = "";
    state.ended = false;
    state.statusText = "准备连接";
    state.autoFollow = true;
  }

  const streamURL = buildReleaseOrderLogStreamURL(
    orderID.value,
    state.offset,
    authStore.accessToken,
    scope,
  );
  const source = new EventSource(streamURL);
  state.stream = source;
  state.connecting = true;
  state.statusText = "连接中...";

  source.onopen = () => {
    state.connecting = false;
    state.connected = true;
    state.error = "";
    if (!state.ended) {
      state.statusText = "流式同步中";
    }
  };

  const handleEventData = (
    eventType: string,
    payload: MessageEvent<string>,
  ) => {
    const parsed = parseStreamEvent(payload.data);
    if (!parsed) {
      return;
    }
    const eventOffset = Number(parsed.offset ?? Number.NaN);
    if (Number.isFinite(eventOffset) && eventOffset >= 0) {
      state.offset = Math.max(state.offset, Math.floor(eventOffset));
    }

    switch (eventType) {
      case "log":
        appendLogContent(scope, String(parsed.content || ""));
        if (parsed.message) {
          appendStatusLine(scope, parsed.message);
        }
        return;
      case "done":
        if (parsed.message) {
          appendStatusLine(scope, parsed.message);
        }
        state.ended = true;
        state.statusText = "已结束";
        state.closeIntentional = true;
        source.close();
        state.stream = null;
        state.connected = false;
        state.connecting = false;
        return;
      case "error":
        if (parsed.message) {
          appendStatusLine(scope, parsed.message);
          state.error = parsed.message;
        } else {
          state.error = "日志流发生异常";
        }
        return;
      default:
        if (parsed.message) {
          appendStatusLine(scope, parsed.message);
          state.statusText = parsed.message;
        }
    }
  };

  source.addEventListener("log", (event) => {
    handleEventData("log", event as MessageEvent<string>);
  });
  source.addEventListener("status", (event) => {
    handleEventData("status", event as MessageEvent<string>);
  });
  source.addEventListener("done", (event) => {
    handleEventData("done", event as MessageEvent<string>);
  });
  source.addEventListener("error", (event) => {
    handleEventData("error", event as MessageEvent<string>);
  });

  source.onerror = () => {
    state.connecting = false;
    state.connected = false;
    if (state.closeIntentional || state.ended) {
      return;
    }
    state.error = "";
    source.close();
    state.stream = null;
    scheduleReconnect(scope);
  };
}

function reconnectLogStream(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  state.error = "";
  state.statusText = "准备重连";
  const shouldReset = state.ended || !shouldKeepLogStreaming.value;
  if (shouldReset) {
    state.ended = false;
  }
  void startLogStream(scope, shouldReset);
}

function clearLogOutput(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  state.text = "";
  state.offset = 0;
  state.error = "";
  state.ended = false;
  state.autoFollow = true;
}

function logStreamTagColor(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  if (state.ended) {
    return "default";
  }
  if (state.error) {
    return "warning";
  }
  return "processing";
}

function logStreamHintText(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  if (state.error) {
    return "日志异常";
  }
  if (latestScopeStepMessage(scope, "failed")) {
    return "执行失败";
  }
  if (state.ended) {
    return "已结束";
  }
  return "";
}

function logSectionWarningMessage(scope: ReleasePipelineScope) {
  const state = getLogState(scope);
  if (state.error) {
    return state.error;
  }
  return latestScopeStepMessage(scope, "failed");
}

function logSectionEmptyDescription(scope: ReleasePipelineScope) {
  return logSectionWarningMessage(scope) || "暂无日志输出";
}

function syncVisibleLogStreams() {
  (["ci", "cd"] as ReleasePipelineScope[]).forEach((scope) => {
    const execution = executionMapByScope.value[scope];
    const state = getLogState(scope);
    if (!execution || execution.provider !== "jenkins") {
      resetLogState(scope);
      return;
    }

    if (!state.stream && !state.connecting && state.text === "") {
      void startLogStream(scope, true);
      return;
    }

    if (
      shouldKeepLogStreaming.value &&
      !state.stream &&
      !state.connecting &&
      !state.ended
    ) {
      void startLogStream(scope, false);
    }
  });
}

async function loadDetail(options?: { silent?: boolean }) {
  const silent = Boolean(options?.silent);
  if (!orderID.value) {
    if (!silent) {
      message.error("缺少发布单 ID");
      void router.push("/releases");
    }
    return;
  }
  if (querying.value) {
    return;
  }

  querying.value = true;
  if (!silent) {
    loading.value = true;
  }
  try {
    const previousStatus = order.value?.status || "";
    const orderResp = await getReleaseOrderByID(orderID.value);
    order.value = orderResp.data;

    const detailResults = await Promise.allSettled([
      listReleaseOrderExecutions(orderID.value),
      canViewParamSnapshot.value
        ? listReleaseOrderParams(orderID.value)
        : Promise.resolve({ data: [] }),
      canViewParamSnapshot.value
        ? listReleaseOrderValueProgress(orderID.value)
        : Promise.resolve({ data: [] }),
      listReleaseOrderSteps(orderID.value),
    ]);

    const detailErrors: string[] = [];

    const executionsResult = detailResults[0];
    if (executionsResult.status === "fulfilled") {
      executions.value = [...executionsResult.value.data].sort(
        (a, b) => scopeSort(a.pipeline_scope) - scopeSort(b.pipeline_scope),
      );
    } else {
      detailErrors.push(
        `执行单元：${extractHTTPErrorMessage(
          executionsResult.reason,
          "加载失败",
        )}`,
      );
    }

    const paramsResult = detailResults[1];
    if (paramsResult.status === "fulfilled") {
      params.value = paramsResult.value.data;
    } else {
      detailErrors.push(
        `参数快照：${extractHTTPErrorMessage(paramsResult.reason, "加载失败")}`,
      );
    }

    const valueProgressResult = detailResults[2];
    if (valueProgressResult.status === "fulfilled") {
      valueProgress.value = valueProgressResult.value.data;
    } else {
      detailErrors.push(
        `值传递进度：${extractHTTPErrorMessage(
          valueProgressResult.reason,
          "加载失败",
        )}`,
      );
    }

    const stepsResult = detailResults[3];
    if (stepsResult.status === "fulfilled") {
      steps.value = stepsResult.value.data;
    } else {
      detailErrors.push(
        `步骤时间线：${extractHTTPErrorMessage(stepsResult.reason, "加载失败")}`,
      );
    }

    await loadApprovalRecords({ silent: true });
    if (orderResp.data.is_concurrent) {
      await loadConcurrentBatchProgress({ silent });
    } else {
      concurrentBatchProgress.value = null;
    }

    if (shouldLoadPrecheck.value) {
      await loadPrecheck({ silent: true });
    } else {
      precheck.value = null;
    }

    const now = Date.now();
    const shouldRefreshPipelineStages =
      !stageLogDrawerVisible.value &&
      (!silent ||
        pipelineStages.value.length === 0 ||
        previousStatus !== orderResp.data.status ||
        now - lastPipelineStageRefreshAt.value >=
          PIPELINE_STAGE_REFRESH_INTERVAL_MS);
    if (shouldRefreshPipelineStages) {
      await loadPipelineStageView({ silent });
    }

    syncVisibleLogStreams();
    if (!silent && detailErrors.length > 0) {
      message.warning(`部分详情刷新失败：${detailErrors.join("；")}`);
    }
  } catch (error) {
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, "发布单详情加载失败"));
      void router.push("/releases");
    }
  } finally {
    querying.value = false;
    if (!silent) {
      loading.value = false;
    }
  }
}

async function clearFastExecuteQuery() {
  if (!fastExecuteRequested.value) {
    return;
  }
  const nextQuery = { ...route.query };
  delete nextQuery.fast_execute;
  await router.replace({
    path: route.path,
    query: nextQuery,
  });
}

async function loadConcurrentBatchProgress(options?: { silent?: boolean }) {
  if (!orderID.value || !showConcurrentBatchCard.value) {
    concurrentBatchProgress.value = null;
    return;
  }
  const silent = Boolean(options?.silent);
  if (!silent) {
    concurrentBatchLoading.value = true;
  }
  try {
    const response = await getReleaseOrderConcurrentBatchProgress(
      orderID.value,
    );
    concurrentBatchProgress.value = response.data;
  } catch (error) {
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, "并发批次进度加载失败"));
    }
  } finally {
    if (!silent) {
      concurrentBatchLoading.value = false;
    }
  }
}

function scopeSort(scope: string) {
  const normalized = String(scope || "")
    .trim()
    .toLowerCase();
  if (normalized === "ci") {
    return 1;
  }
  if (normalized === "cd") {
    return 2;
  }
  return 99;
}

async function loadPipelineStageView(options?: { silent?: boolean }) {
  if (!orderID.value) {
    return;
  }
  const silent = Boolean(options?.silent);
  if (!silent) {
    pipelineStageLoading.value = true;
  }
  try {
    const response = await listReleaseOrderPipelineStages(orderID.value);
    pipelineStageModuleVisible.value = Boolean(response.show_module);
    pipelineStageExecutorType.value = String(
      response.executor_type || "",
    ).trim();
    pipelineStageMessage.value = String(response.message || "").trim();
    pipelineStages.value = response.data || [];
    lastPipelineStageRefreshAt.value = Date.now();
  } catch (error) {
    if (silent) {
      pipelineStageMessage.value = extractHTTPErrorMessage(
        error,
        "管线阶段暂时同步失败，请稍后手动刷新",
      );
    } else {
      pipelineStageModuleVisible.value = false;
      pipelineStageExecutorType.value = "";
      pipelineStageMessage.value = "";
      pipelineStages.value = [];
      message.error(extractHTTPErrorMessage(error, "管线阶段加载失败"));
    }
  } finally {
    if (!silent) {
      pipelineStageLoading.value = false;
    }
  }
}

async function loadPrecheck(options?: {
  silent?: boolean;
  action?: ReleaseOrderDispatchAction;
}) {
  if (!orderID.value || !shouldLoadPrecheck.value) {
    precheck.value = null;
    return;
  }
  const silent = Boolean(options?.silent);
  if (!silent) {
    precheckLoading.value = true;
  }
  try {
    const response = await getReleaseOrderPrecheck(
      orderID.value,
      options?.action || currentPrecheckAction.value,
    );
    precheck.value = response.data;
  } catch (error) {
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, "执行前预检加载失败"));
    }
  } finally {
    if (!silent) {
      precheckLoading.value = false;
    }
  }
}

async function loadApprovalRecords(options?: { silent?: boolean }) {
  if (!orderID.value) {
    approvalRecords.value = [];
    return;
  }
  const silent = Boolean(options?.silent);
  const shouldLoad =
    Boolean(order.value?.approval_required) ||
    ["pending_approval", "approving", "approved", "rejected"].includes(
      currentBusinessStatus.value,
    );
  if (!shouldLoad) {
    approvalRecords.value = [];
    return;
  }
  if (!silent) {
    approvalRecordsLoading.value = true;
  }
  try {
    const response = await listReleaseOrderApprovalRecords(orderID.value);
    approvalRecords.value = response.data || [];
  } catch (error) {
    if (!silent) {
      message.error(extractHTTPErrorMessage(error, "审批记录加载失败"));
    }
  } finally {
    if (!silent) {
      approvalRecordsLoading.value = false;
    }
  }
}

async function openStageLogDrawer(stage: ReleaseOrderPipelineStage) {
  selectedPipelineStage.value = stage;
  stageLogDrawerVisible.value = true;
  await loadStageLog();
}

function closeStageLogDrawer() {
  stageLogDrawerVisible.value = false;
  selectedPipelineStage.value = null;
  stageLogContent.value = "";
  stageLogHasMore.value = false;
  stageLogFetchedAt.value = "";
}

async function loadStageLog() {
  if (!orderID.value || !selectedPipelineStage.value) {
    return;
  }
  stageLogLoading.value = true;
  try {
    const response = await getReleaseOrderPipelineStageLog(
      orderID.value,
      selectedPipelineStage.value.id,
    );
    selectedPipelineStage.value = response.data.stage;
    stageLogContent.value = response.data.content || "";
    stageLogHasMore.value = Boolean(response.data.has_more);
    stageLogFetchedAt.value = formatTime(response.data.fetched_at);
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "阶段日志加载失败"));
  } finally {
    stageLogLoading.value = false;
  }
}

async function handleCancel() {
  if (!order.value) {
    return;
  }
  cancelling.value = true;
  try {
    const response = await cancelReleaseOrder(order.value.id);
    order.value = response.data;
    message.success("发布单取消成功");
    await loadDetail({ silent: true });
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "发布单取消失败"));
  } finally {
    cancelling.value = false;
  }
}

async function handleExecute() {
  await executeCurrentOrder("execute");
}

async function handleBuild() {
  await executeCurrentOrder("build");
}

async function handleDeploy() {
  await executeCurrentOrder("deploy");
}

async function executeCurrentOrder(
  action: ReleaseOrderDispatchAction = "execute",
  options?: {
  skipPrecheck?: boolean;
  successMessage?: string;
  errorMessage?: string;
},
) {
  if (!order.value || executeLocked.value) {
    return;
  }
  currentDispatchAction.value = action;
  const skipPrecheck = Boolean(options?.skipPrecheck);
  const allowExecuteByStatus =
    action === "build"
      ? canBuild.value
      : action === "deploy"
        ? canDeploy.value
        : canExecutePermission.value &&
          (currentBusinessStatus.value === "pending_execution" ||
            currentBusinessStatus.value === "approved");
  if (!allowExecuteByStatus) {
    message.warning(
      precheckSummaryMessage.value ||
        (action === "build"
          ? "当前发布单不满足仅构建条件，无法再次触发仅构建"
          : action === "deploy"
            ? "当前发布单不满足发布条件，无法再次触发发布"
            : "当前发布单已执行完成、已取消或不处于待执行状态，无法再次触发发布"),
    );
    return;
  }
  executeLocked.value = true;
  executing.value = true;
  try {
    if (!skipPrecheck) {
      await loadPrecheck({ silent: true, action });
    }
    if (!skipPrecheck && precheckBlocked.value) {
      message.warning(
        precheckSummaryMessage.value ||
          (action === "build"
            ? "当前发布单未通过仅构建前预检，请先处理阻塞项"
            : action === "deploy"
              ? "当前发布单未通过发布前预检，请先处理阻塞项"
              : "当前发布单未通过执行前预检，请先处理阻塞项"),
      );
      return;
    }
    const response =
      action === "build"
        ? await buildReleaseOrder(order.value.id)
        : action === "deploy"
          ? await deployReleaseOrder(order.value.id)
          : await executeReleaseOrder(order.value.id);
    order.value = response.data;
    message.success(
      options?.successMessage ||
        (action === "build"
          ? "仅构建已提交，正在调度执行"
          : action === "deploy"
            ? "发布已提交，正在调度执行"
            : "发布已提交，正在调度执行"),
    );
    try {
      await loadDetail({ silent: true });
    } catch (refreshError) {
      message.warning(
        extractHTTPErrorMessage(
          refreshError,
          action === "build"
            ? "仅构建已触发，但详情刷新失败，请稍后手动刷新"
            : action === "deploy"
              ? "发布已触发，但详情刷新失败，请稍后手动刷新"
              : "发布已触发，但详情刷新失败，请稍后手动刷新",
        ),
      );
    }
  } catch (error) {
    message.error(
      extractHTTPErrorMessage(
        error,
        options?.errorMessage ||
          (action === "build"
            ? "仅构建执行失败"
            : action === "deploy"
              ? "发布执行失败"
              : "发布执行失败"),
      ),
    );
  } finally {
    executing.value = false;
    executeLocked.value = false;
  }
}

async function tryAutoExecuteFastRelease() {
  if (!fastExecuteRequested.value || fastExecuteTriggered.value) {
    return;
  }
  fastExecuteTriggered.value = true;
  await clearFastExecuteQuery();
  if (!order.value) {
    return;
  }
  if (order.value.approval_required) {
    message.warning("当前模板启用了审批，极速发布已自动取消");
    return;
  }
  await executeCurrentOrder("execute", {
    skipPrecheck: true,
    successMessage: "极速发布已自动开始执行",
    errorMessage: "极速发布自动执行失败",
  });
}

async function handleApprovalAction() {
  if (!order.value) {
    return;
  }
  const comment = approvalActionComment.value.trim();
  if (approvalActionMode.value === "reject" && !comment) {
    message.warning("请先填写拒绝原因");
    return;
  }
  approvalActing.value = true;
  try {
    let response;
    switch (approvalActionMode.value) {
      case "submit":
        response = await submitReleaseOrderApproval(order.value.id, { comment });
        message.success("审批已提交");
        break;
      case "approve":
        response = await approveReleaseOrder(order.value.id, { comment });
        message.success("审批已通过");
        break;
      case "reject":
        response = await rejectReleaseOrder(order.value.id, { comment });
        message.success("审批已拒绝");
        break;
      default:
        return;
    }
    order.value = response.data;
    closeApprovalActionModal();
    await loadApprovalRecords({ silent: true });
    await loadDetail({ silent: true });
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "审批操作失败"));
  } finally {
    approvalActing.value = false;
  }
}

async function handleRollback() {
  if (!order.value || !canRollback.value) {
    return;
  }
  recovering.value = true;
  try {
    const response = await rollbackReleaseOrderByID(order.value.id);
    message.success(`已创建一键重发单：${response.data.order_no}`);
    void router.push(`/releases/${response.data.id}`);
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "一键重发创建失败"));
  } finally {
    recovering.value = false;
  }
}

async function handleReplay() {
  if (!order.value || !canReplay.value) {
    return;
  }
  recovering.value = true;
  try {
    const response = await replayReleaseOrderByID(order.value.id);
    message.success(replaySuccessText(order.value, response.data.order_no));
    void router.push(`/releases/${response.data.id}`);
  } catch (error) {
    message.error(
      extractHTTPErrorMessage(error, replayFailureText(order.value)),
    );
  } finally {
    recovering.value = false;
  }
}

function goBack() {
  void router.push("/releases");
}

function handleEdit() {
  if (!order.value || !canEdit.value) {
    message.warning("当前发布单不是可编辑的待执行普通发布单");
    return;
  }
  void router.push(`/releases/${order.value.id}/edit`);
}

function stopAutoRefresh() {
  if (autoRefreshTimer.value !== null) {
    window.clearInterval(autoRefreshTimer.value);
    autoRefreshTimer.value = null;
  }
}

function startAutoRefresh() {
  stopAutoRefresh();
  autoRefreshTimer.value = window.setInterval(() => {
    if (document.hidden || cancelling.value || !shouldAutoRefresh.value) {
      return;
    }
    void loadDetail({ silent: true });
  }, AUTO_REFRESH_INTERVAL_MS);
}

function closeAllLogStreams() {
  (["ci", "cd"] as ReleasePipelineScope[]).forEach((scope) => {
    closeLogStream(scope);
  });
}

onMounted(async () => {
  await loadDetail();
  await nextTick();
  await tryAutoExecuteFastRelease();
  startAutoRefresh();
});

onBeforeUnmount(() => {
  stopAutoRefresh();
  closeAllLogStreams();
});
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header release-detail-header">
      <div class="header-left">
        <div class="page-header-copy">
          <h2 class="page-title">详情</h2>
        </div>
      </div>
      <div class="page-header-actions release-detail-actions">
        <a-button
          v-if="canEdit"
          class="application-toolbar-action-btn"
          @click="handleEdit"
        >
          编辑
        </a-button>
        <a-button
          v-if="canSubmitApproval"
          class="application-toolbar-action-btn"
          :loading="approvalActing && approvalActionMode === 'submit'"
          @click="openApprovalActionModal('submit')"
        >
          提交审批
        </a-button>
        <a-button
          v-if="canApproveOrder"
          class="application-toolbar-action-btn"
          :loading="approvalActing && approvalActionMode === 'approve'"
          @click="openApprovalActionModal('approve')"
        >
          审批通过
        </a-button>
        <a-button
          v-if="canRejectOrder"
          class="application-toolbar-action-btn"
          :loading="approvalActing && approvalActionMode === 'reject'"
          @click="openApprovalActionModal('reject')"
        >
          审批拒绝
        </a-button>
        <a-button
          v-if="canBuild"
          class="application-toolbar-action-btn"
          :loading="executing && currentDispatchAction === 'build'"
          :disabled="executeLocked"
          @click="handleBuild"
        >
          仅构建
        </a-button>
        <a-button
          v-if="canDeploy"
          class="application-toolbar-action-btn"
          :loading="executing && currentDispatchAction === 'deploy'"
          :disabled="executeLocked"
          @click="handleDeploy"
        >
          发布
        </a-button>
        <a-popconfirm
          v-if="canExecute"
          title="确认执行当前发布单吗？"
          ok-text="确认"
          cancel-text="取消"
          @confirm="handleExecute"
        >
          <template #icon>
            <ExclamationCircleOutlined />
          </template>
          <a-button
            class="application-toolbar-action-btn"
            :loading="executing && currentDispatchAction === 'execute'"
            :disabled="executeLocked"
          >
            发布
          </a-button>
        </a-popconfirm>
        <a-button
          v-else-if="!canBuild && !canDeploy"
          class="application-toolbar-action-btn"
          disabled
        >
          发布
        </a-button>
        <a-popconfirm
          v-if="canTriggerArgoReplay"
          :disabled="!canRollback"
          title="确认基于当前发布单创建一键重发单吗？"
          ok-text="确认重发"
          cancel-text="取消"
          @confirm="handleRollback"
        >
          <template #icon>
            <ExclamationCircleOutlined class="danger-icon" />
          </template>
          <a-button
            class="application-toolbar-action-btn"
            :disabled="!canRollback"
            :loading="recovering"
          >
            一键重发
          </a-button>
        </a-popconfirm>
        <a-popconfirm
          v-else-if="canTriggerStandardReplay"
          :disabled="!canReplay"
          :title="replayConfirmTitle(order)"
          :ok-text="isCiOnlyRecovery(order) ? '确认重发' : '确认重放'"
          cancel-text="取消"
          @confirm="handleReplay"
        >
          <template #icon>
            <ExclamationCircleOutlined />
          </template>
          <a-button
            class="application-toolbar-action-btn"
            :disabled="!canReplay"
            :loading="recovering"
          >
            {{ replayActionText(order) }}
          </a-button>
        </a-popconfirm>
        <a-popconfirm
          v-if="canCancel"
          title="确认取消当前发布单吗？"
          ok-text="确认"
          cancel-text="取消"
          @confirm="handleCancel"
        >
          <template #icon>
            <ExclamationCircleOutlined class="danger-icon" />
          </template>
          <a-button
            class="application-toolbar-action-btn"
            :loading="cancelling"
          >
            取消发布
          </a-button>
        </a-popconfirm>
        <a-button class="application-toolbar-action-btn" @click="goBack">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          返回发布单
        </a-button>
      </div>
    </div>

    <a-card
      class="detail-card release-hero-card"
      :loading="loading"
      :bordered="true"
    >
      <div class="release-hero">
        <div class="release-hero-main">
          <div class="release-hero-title-row">
            <div>
              <div class="release-hero-label">发布单号</div>
              <div class="release-hero-order">
                <span>{{ order?.order_no || "-" }}</span>
                <a-tag
                  v-if="order?.operation_type === 'rollback'"
                  class="status-chip status-chip-danger"
                >
                  {{ operationTypeText(order?.operation_type) }}
                </a-tag>
                <a-tag
                  v-else-if="order?.operation_type === 'replay'"
                  class="status-chip status-chip-warning"
                >
                  {{ operationTypeText(order?.operation_type) }}
                </a-tag>
              </div>
            </div>
          </div>

          <div class="release-hero-facts">
            <div v-for="item in heroFacts" :key="item.label" class="hero-fact">
              <span class="hero-fact-label">{{ item.label }}</span>
              <span class="hero-fact-value">{{ item.value }}</span>
            </div>
          </div>
        </div>

        <div
          class="release-spotlight"
          :class="`release-spotlight-${spotlightStatusKey}`"
        >
          <div class="release-spotlight-content">
            <div class="release-spotlight-header">
              <div class="release-spotlight-label">整体进度</div>
            </div>
            <div class="release-spotlight-title">{{ spotlightTitle }}</div>
            <div class="release-spotlight-description">
              {{ spotlightDescription }}
            </div>
            <div class="release-spotlight-meta">{{ spotlightMeta }}</div>
          </div>
          <div class="release-spotlight-icon-wrap">
            <div
              class="release-spotlight-icon-orb"
              :class="`release-spotlight-icon-orb-${spotlightStatusKey}`"
            >
              <SyncOutlined
                v-if="spotlightStatusKey === 'running'"
                spin
                class="release-spotlight-icon"
              />
              <ClockCircleFilled
                v-else-if="spotlightStatusKey === 'queued'"
                class="release-spotlight-icon"
              />
              <CheckCircleFilled
                v-else-if="spotlightStatusKey === 'success'"
                class="release-spotlight-icon"
              />
              <CloseCircleFilled
                v-else-if="spotlightStatusKey === 'failed'"
                class="release-spotlight-icon"
              />
              <StopFilled
                v-else-if="spotlightStatusKey === 'cancelled'"
                class="release-spotlight-icon"
              />
              <ClockCircleFilled v-else class="release-spotlight-icon" />
            </div>
          </div>
        </div>
      </div>
    </a-card>

    <div class="detail-dashboard">
      <div class="dashboard-main">
        <a-collapse class="detail-collapse timeline-collapse" ghost>
          <a-collapse-panel key="execution-timeline" header="执行时间线">
            <a-spin :spinning="loading">
              <a-empty v-if="stepGroups.length === 0" description="暂无步骤数据" />
              <div v-else class="step-groups">
                <div
                  v-for="group in stepGroups"
                  :key="group.key"
                  class="scope-section"
                >
                  <div class="scope-section-header scope-section-header-inline">
                    <a-tag class="status-chip status-chip-section">{{
                      group.title
                    }}</a-tag>
                    <span class="scope-section-subtitle"
                      >{{ group.items.length }} 个步骤</span
                    >
                  </div>
                  <a-steps direction="vertical" size="small" class="step-progress">
                    <a-step
                      v-for="step in group.items"
                      :key="step.id"
                      :title="step.step_name"
                      :status="stepComponentStatus(step.status)"
                    >
                      <template #description>
                        <div class="step-description">{{ describeStep(step) }}</div>
                      </template>
                    </a-step>
                  </a-steps>
                </div>
              </div>
            </a-spin>
          </a-collapse-panel>
        </a-collapse>

        <a-collapse class="detail-collapse base-info-collapse" ghost>
          <a-collapse-panel key="base-info" header="基础信息与参数快照">
            <section class="detail-inline-section">
              <div class="detail-inline-section-header">
                <div class="detail-inline-section-title">基础信息</div>
              </div>
              <a-descriptions
                class="detail-info-descriptions"
                :column="{ xs: 1, md: 2 }"
                bordered
              >
                <a-descriptions-item
                  v-for="item in detailItems"
                  :key="item.key || item.label"
                  :label="item.label"
                >
                  <template v-if="item.key === 'order_no'">
                    <a-space :size="6">
                      <span>{{ item.value }}</span>
                      <a-tag
                        v-if="order?.operation_type === 'rollback'"
                        class="status-chip status-chip-danger"
                      >
                        {{ operationTypeText(order?.operation_type) }}
                      </a-tag>
                      <a-tag
                        v-else-if="order?.operation_type === 'replay'"
                        class="status-chip status-chip-warning"
                      >
                        {{ operationTypeText(order?.operation_type) }}
                      </a-tag>
                    </a-space>
                  </template>
                  <template v-else>
                    {{ item.value }}
                  </template>
                </a-descriptions-item>
              </a-descriptions>
            </section>

            <template v-if="canViewParamSnapshot">
              <section
                v-for="group in paramGroups"
                :key="group.scope"
                class="detail-inline-section"
              >
                <div class="detail-inline-section-header">
                  <div class="detail-inline-section-title">
                    {{ group.title }}
                  </div>
                </div>
                <a-empty
                  v-if="group.items.length === 0"
                  description="暂无参数快照"
                />
                <a-table
                  class="detail-data-table detail-snapshot-table"
                  v-else
                  row-key="id"
                  :columns="paramColumns"
                  :data-source="group.items"
                  :pagination="false"
                  :scroll="{ x: 1200 }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'created_at'">
                      {{ formatTime(record.created_at) }}
                    </template>
                    <template v-else-if="column.key === 'param_value'">
                      {{ record.param_value || "-" }}
                    </template>
                  </template>
                </a-table>
              </section>
            </template>
          </a-collapse-panel>
        </a-collapse>

        <a-card
          class="detail-card detail-section-card"
          title="阶段与日志"
          :loading="pipelineStageLoading"
          :bordered="true"
        >
          <a-tabs>
            <a-tab-pane key="stages" tab="管线进度">
              <template #tab>
                <span>管线进度</span>
              </template>
              <div v-if="stageSections.length > 0" class="stage-sections">
                <div
                  v-for="section in stageSections"
                  :key="section.scope"
                  class="scope-section"
                >
                  <div class="scope-section-header scope-section-header-inline">
                    <div class="scope-section-heading">
                      <a-tag class="status-chip status-chip-section">{{
                        section.title
                      }}</a-tag>
                      <a-space class="pipeline-stage-title-actions" size="small">
                        <a-popover
                          v-if="pipelineStageMessage"
                          trigger="click"
                          placement="bottomLeft"
                          overlay-class-name="release-tip-popover"
                        >
                          <template #content>
                            <div class="release-tip-content">
                              {{ pipelineStageMessage }}
                            </div>
                          </template>
                          <button
                            class="release-tip-trigger release-tip-trigger-info"
                            type="button"
                            aria-label="查看管线提示"
                          >
                            <ExclamationCircleOutlined />
                          </button>
                        </a-popover>
                        <a-tag class="pipeline-executor-chip">
                          {{
                            section.execution?.provider ||
                            pipelineStageExecutorType ||
                            "-"
                          }}
                        </a-tag>
                        <a-button size="small" @click="loadPipelineStageView"
                          >刷新阶段</a-button
                        >
                      </a-space>
                    </div>
                    <div class="scope-section-meta">
                      <span class="scope-section-subtitle">{{
                        section.execution?.binding_name || "-"
                      }}</span>
                      <a-popover
                        v-if="section.isArgoCD"
                        trigger="click"
                        placement="bottomRight"
                        overlay-class-name="release-tip-popover"
                      >
                        <template #content>
                          <div class="release-tip-content">
                            当前阶段来自 ArgoCD 执行链路，展示的是 GitOps
                            写回、Sync 与健康检查进度
                          </div>
                        </template>
                        <button
                          class="release-tip-trigger release-tip-trigger-info"
                          type="button"
                          aria-label="查看阶段提示"
                        >
                          <ExclamationCircleOutlined />
                        </button>
                      </a-popover>
                      <a-popover
                        v-else-if="!section.isJenkins"
                        trigger="click"
                        placement="bottomRight"
                        overlay-class-name="release-tip-popover"
                      >
                        <template #content>
                          <div class="release-tip-content">
                            {{ scopeLabel(section.scope) }} 当前使用
                            {{ section.execution?.provider || "未知执行器" }}，部署进度视图待接入
                          </div>
                        </template>
                        <button
                          class="release-tip-trigger release-tip-trigger-info"
                          type="button"
                          aria-label="查看阶段提示"
                        >
                          <ExclamationCircleOutlined />
                        </button>
                      </a-popover>
                    </div>
                  </div>

                  <a-empty
                    v-if="section.stages.length === 0"
                    :description="pipelineStageEmptyDescription(section)"
                  />
                  <div v-else class="pipeline-stage-chain">
                    <div
                      v-for="stage in section.stages"
                      :key="stage.id"
                      class="pipeline-stage-node"
                      :class="[
                        `pipeline-stage-node-${stage.status}`,
                        { 'pipeline-stage-node-clickable': section.isJenkins },
                      ]"
                      :role="section.isJenkins ? 'button' : undefined"
                      :tabindex="section.isJenkins ? 0 : undefined"
                      :title="section.isJenkins ? '点击查看阶段日志' : undefined"
                      @click="section.isJenkins && openStageLogDrawer(stage)"
                      @keydown.enter.prevent="
                        section.isJenkins && openStageLogDrawer(stage)
                      "
                      @keydown.space.prevent="
                        section.isJenkins && openStageLogDrawer(stage)
                      "
                    >
                      <div class="pipeline-stage-order-col">
                        <span class="pipeline-stage-index">{{
                          stage.sort_no
                        }}</span>
                        <a-tag
                          :class="[
                            'status-tag',
                            statusToneClass(stage.status),
                          ]"
                        >
                          <LoadingOutlined
                            v-if="isRunningStatus(stage.status)"
                            spin
                          />
                          <span>{{ statusText(stage.status) }}</span>
                        </a-tag>
                      </div>
                      <div class="pipeline-stage-node-main">
                        <div class="pipeline-stage-node-head">
                          <div class="pipeline-stage-name">
                            {{ stage.stage_name || "-" }}
                          </div>
                        </div>
                        <div class="pipeline-stage-meta-line">
                          <span>
                            <b>耗时</b>{{ formatDuration(stage.duration_millis) }}
                          </span>
                          <span v-if="stage.raw_status" :title="stage.raw_status">
                            <b>原始</b>{{ stage.raw_status }}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <a-empty v-else description="暂无管线进度数据" />
            </a-tab-pane>

            <a-tab-pane key="logs" tab="实时日志">
              <template #tab>
                <span>实时日志</span>
              </template>
              <div class="log-sections">
                <section
                  v-for="section in logSections"
                  :key="section.scope"
                  class="detail-inline-section"
                >
                  <div class="detail-inline-section-header">
                    <div class="detail-inline-section-title-row">
                      <div class="detail-inline-section-title">
                        {{ section.title }}
                      </div>
                      <a-popover
                        v-if="!section.isJenkins"
                        trigger="click"
                        placement="bottomLeft"
                        overlay-class-name="release-tip-popover"
                      >
                        <template #content>
                          <div class="release-tip-content">
                            {{
                              section.execution?.provider === "argocd"
                                ? `${scopeLabel(section.scope)} 当前使用 ArgoCD，当前版本先展示执行进度；事件流/日志视图将在后续版本补齐`
                                : `${scopeLabel(section.scope)} 当前使用 ${section.execution?.provider || "未知执行器"}，独立日志视图待接入`
                            }}
                          </div>
                        </template>
                        <button
                          class="release-tip-trigger release-tip-trigger-info"
                          type="button"
                          aria-label="查看日志提示"
                        >
                          <ExclamationCircleOutlined />
                        </button>
                      </a-popover>
                    </div>
                    <a-space
                      v-if="section.isJenkins"
                      class="detail-inline-section-extra"
                    >
                      <a-popover
                        v-if="logSectionWarningMessage(section.scope)"
                        trigger="click"
                        placement="bottomRight"
                        overlay-class-name="release-tip-popover"
                      >
                        <template #content>
                          <div class="release-tip-content">
                            {{ logSectionWarningMessage(section.scope) }}
                          </div>
                        </template>
                        <button
                          class="release-tip-trigger release-tip-trigger-warning"
                          type="button"
                          aria-label="查看日志警告"
                        >
                          <ExclamationCircleOutlined />
                        </button>
                      </a-popover>
                      <a-tag
                        v-if="logStreamHintText(section.scope)"
                        :color="logStreamTagColor(section.scope)"
                        >{{ logStreamHintText(section.scope) }}</a-tag
                      >
                      <a-switch
                        size="small"
                        :checked="section.state.autoFollow"
                        checked-children="跟随"
                        un-checked-children="暂停"
                        @change="handleLogFollowChange(section.scope, $event)"
                      />
                      <a-button
                        size="small"
                        @click="jumpLogToBottom(section.scope)"
                        >底部</a-button
                      >
                      <a-button
                        size="small"
                        @click="reconnectLogStream(section.scope)"
                        :loading="section.state.connecting"
                        >重连</a-button
                      >
                      <a-button
                        size="small"
                        @click="clearLogOutput(section.scope)"
                        >清空</a-button
                      >
                    </a-space>
                  </div>

                  <template v-if="section.isJenkins">
                    <pre
                      :ref="
                        (el) =>
                          setLogPanelRef(section.scope, el as Element | null)
                      "
                      class="log-panel"
                      @scroll="syncLogFollowState(section.scope)"
                      >{{
                        section.state.text ||
                        logSectionEmptyDescription(section.scope)
                      }}</pre
                    >
                  </template>
                </section>
              </div>
            </a-tab-pane>
          </a-tabs>
        </a-card>

        <a-collapse
          v-if="canViewParamSnapshot && valueProgressGroups.length > 0"
          class="detail-collapse value-progress-collapse"
          ghost
        >
          <a-collapse-panel key="value-progress">
            <template #header>
              <div class="value-progress-collapse-heading">
                <span class="value-progress-collapse-title">取值进度</span>
                <span class="value-progress-collapse-summary">
                  {{ valueProgressGroups.length }} 组 · {{ valueProgressTotal }} 项
                </span>
              </div>
            </template>
            <template #extra>
              <a-popover
                trigger="click"
                placement="bottomRight"
                overlay-class-name="release-tip-popover"
              >
                <template #content>
                  <div class="release-tip-content">
                    这里展示模板中已映射标准 Key 的实时取值情况
                  </div>
                </template>
                <button
                  class="release-tip-trigger release-tip-trigger-info"
                  type="button"
                  aria-label="查看取值说明"
                  @click.stop
                >
                  <ExclamationCircleOutlined />
                </button>
              </a-popover>
            </template>
            <a-spin :spinning="loading">
              <div class="value-progress-group-list">
                <section
                  v-for="group in valueProgressGroups"
                  :key="`value-${group.scope}`"
                  class="value-progress-group"
                >
                  <div class="value-progress-group-header">
                    <div class="value-progress-group-title">
                      <a-tag class="status-chip status-chip-section">
                        {{ scopeLabel(group.scope) }}
                      </a-tag>
                    </div>
                    <span class="value-progress-group-meta">
                      {{ group.items.length }} 项
                    </span>
                  </div>
                  <div class="value-progress-item-list">
                    <div
                      v-for="item in group.items"
                      :key="`${item.pipeline_scope}-${item.param_key}-${item.executor_param_name}`"
                      class="value-progress-item"
                      :class="`value-progress-item-${item.status}`"
                    >
                      <div class="value-progress-keyline">
                        <span class="value-progress-order">{{
                          item.sort_no
                        }}</span>
                        <div class="value-progress-copy">
                          <div class="value-progress-key">
                            {{ item.param_key || "-" }}
                          </div>
                          <div class="value-progress-name">
                            <span>{{ item.param_name || "-" }}</span>
                            <a-tag
                              v-if="item.required"
                              class="required-tag status-chip status-chip-danger"
                              >必需</a-tag
                            >
                          </div>
                        </div>
                      </div>
                      <div class="value-progress-current">
                        <span class="value-progress-label">当前值</span>
                        <span
                          class="value-progress-value"
                          :title="item.value || '-'"
                          >{{ item.value || "-" }}</span
                        >
                      </div>
                      <div class="value-progress-row-meta">
                        <span>
                          <b>执行器参数</b>
                          <em :title="item.executor_param_name || '-'">{{
                            item.executor_param_name || "-"
                          }}</em>
                        </span>
                        <span>
                          <b>来源</b>
                          <em>{{ item.value_source || "-" }}</em>
                        </span>
                        <span>
                          <b>更新时间</b>
                          <em>{{ formatTime(item.updated_at) }}</em>
                        </span>
                      </div>
                      <a-tag
                        :class="[
                          'status-tag',
                          valueProgressToneClass(item.status),
                        ]"
                      >
                        <LoadingOutlined v-if="item.status === 'running'" spin />
                        <span>{{ valueProgressStatusText(item.status) }}</span>
                      </a-tag>
                      <div v-if="item.message" class="value-progress-message">
                        {{ item.message }}
                      </div>
                    </div>
                  </div>
                </section>
              </div>
            </a-spin>
          </a-collapse-panel>
        </a-collapse>

      </div>

      <div class="dashboard-side">
        <a-card
          v-if="showConcurrentBatchCard"
          class="detail-card detail-side-card"
          title="并发批次进度"
          :loading="concurrentBatchLoading"
          :bordered="true"
        >
          <div class="batch-progress-meta">
            <div class="batch-progress-badge">
              <span class="batch-progress-label">批次号</span>
              <span class="batch-progress-value">{{
                concurrentBatchProgress?.batch_no ||
                order?.concurrent_batch_no ||
                "-"
              }}</span>
            </div>
            <div class="batch-progress-stats">
              <div
                v-for="item in concurrentBatchSummary"
                :key="item.label"
                class="batch-progress-stat"
              >
                <span class="batch-progress-stat-label">{{ item.label }}</span>
                <span class="batch-progress-stat-value">{{ item.value }}</span>
              </div>
            </div>
          </div>
          <a-empty
            v-if="!concurrentBatchProgress?.items?.length"
            description="暂无并发批次进度"
          />
          <div v-else class="batch-progress-list">
            <div
              v-for="item in concurrentBatchProgress.items"
              :key="item.order_id"
              class="batch-progress-item"
            >
              <div class="batch-progress-item-main">
                <div class="batch-progress-item-order">
                  <span>#{{ item.concurrent_batch_seq || "-" }}</span>
                  <span>{{ item.order_no }}</span>
                </div>
                <div class="batch-progress-item-meta">
                  <span>{{ item.application_name || "-" }}</span>
                  <span>{{ item.env_code || "-" }}</span>
                  <span>{{ operationTypeText(item.operation_type) }}</span>
                </div>
              </div>
              <div class="batch-progress-item-side">
                <a-tag
                  :class="[
                    'status-tag',
                    concurrentQueueToneClass(item.queue_state),
                  ]"
                >
                  <LoadingOutlined
                    v-if="
                      item.queue_state === 'executing' ||
                      item.queue_state === 'queued'
                    "
                    :spin="item.queue_state === 'executing'"
                  />
                  <span>{{ concurrentQueueStateText(item.queue_state) }}</span>
                </a-tag>
                <div
                  v-if="item.queue_position > 0"
                  class="batch-progress-item-position"
                >
                  队列位次 {{ item.queue_position }}
                </div>
              </div>
            </div>
          </div>
        </a-card>

        <a-card
          v-if="showApprovalCard"
          class="detail-card detail-side-card"
          title="审批进度"
          :loading="loading || approvalRecordsLoading"
          :bordered="true"
        >
          <div class="approval-summary">
            <div class="approval-summary-head">
              <div>
                <div class="approval-summary-title">
                  {{ statusText(currentBusinessStatus) }}
                </div>
                <div class="approval-summary-subtitle">
                  {{ approvalStatusSummary }}
                </div>
              </div>
              <a-space class="approval-summary-actions">
                <a-popover
                  v-if="order?.rejected_reason"
                  trigger="click"
                  placement="bottomRight"
                  overlay-class-name="release-tip-popover"
                >
                  <template #content>
                    <div class="release-tip-content">
                      拒绝原因：{{ order.rejected_reason }}
                    </div>
                  </template>
                  <button
                    class="release-tip-trigger release-tip-trigger-warning"
                    type="button"
                    aria-label="查看拒绝原因"
                  >
                    <ExclamationCircleOutlined />
                  </button>
                </a-popover>
                <a-tag
                  :class="['status-tag', statusToneClass(currentBusinessStatus)]"
                >
                  <LoadingOutlined
                    v-if="
                      currentBusinessStatus === 'approving' ||
                      currentBusinessStatus === 'pending_approval'
                    "
                    :spin="currentBusinessStatus === 'approving'"
                  />
                  <span>{{ statusText(currentBusinessStatus) }}</span>
                </a-tag>
              </a-space>
            </div>
            <div class="approval-meta-grid">
              <div class="approval-meta-item">
                <span class="approval-meta-label">审批方式</span>
                <span class="approval-meta-value">{{
                  approvalModeText(order?.approval_mode)
                }}</span>
              </div>
              <div class="approval-meta-item">
                <span class="approval-meta-label">审批人</span>
                <span class="approval-meta-value">{{
                  (order?.approval_approver_names || []).join(" / ") || "-"
                }}</span>
              </div>
              <div class="approval-meta-item" v-if="order?.approved_at">
                <span class="approval-meta-label">通过时间</span>
                <span class="approval-meta-value">{{
                  formatTime(order?.approved_at || null)
                }}</span>
              </div>
              <div class="approval-meta-item" v-if="order?.rejected_at">
                <span class="approval-meta-label">拒绝时间</span>
                <span class="approval-meta-value">{{
                  formatTime(order?.rejected_at || null)
                }}</span>
              </div>
            </div>
          </div>

          <div class="approval-records">
            <div class="approval-records-header">
              <span>审批记录</span>
              <span>{{ displayApprovalRecords.length }} 条</span>
            </div>
            <a-empty
              v-if="displayApprovalRecords.length === 0"
              description="当前还没有审批动作记录"
            />
            <div v-else class="approval-record-list">
              <div
                v-for="record in displayApprovalRecords"
                :key="record.id"
                class="approval-record-item"
              >
                <div class="approval-record-main">
                  <div class="approval-record-title-row">
                    <span class="approval-record-title">{{
                      approvalActionText(record.action)
                    }}</span>
                    <a-tag class="status-chip status-chip-section">
                      {{ record.operator_name || record.operator_user_id || "-" }}
                    </a-tag>
                  </div>
                  <div class="approval-record-time">
                    {{ formatTime(record.created_at) }}
                  </div>
                  <div v-if="record.comment" class="approval-record-comment">
                    {{ record.comment }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </a-card>

        <a-card
          v-if="showPrecheckCard"
          class="detail-card detail-side-card"
          :title="precheckCardTitle"
          :loading="precheckLoading"
          :bordered="true"
        >
          <template #extra>
            <a-popover
              v-if="precheckSummaryMessage"
              trigger="click"
              placement="bottomRight"
              overlay-class-name="release-tip-popover"
            >
              <template #content>
                <div class="release-tip-content">
                  {{ precheckSummaryMessage }}
                </div>
              </template>
              <button
                class="release-tip-trigger"
                :class="`release-tip-trigger-${precheckSummaryTone}`"
                type="button"
                aria-label="查看预检提示"
              >
                <ExclamationCircleOutlined />
              </button>
            </a-popover>
          </template>
          <div v-if="precheck?.items?.length" class="precheck-list">
            <div
              v-for="item in precheck.items"
              :key="item.key"
              class="precheck-item"
            >
              <div class="precheck-item-copy">
                <div class="precheck-item-name">{{ item.name }}</div>
                <div class="precheck-item-message">{{ item.message }}</div>
              </div>
              <a-tag :class="['status-tag', precheckToneClass(item.status)]">
                <LoadingOutlined v-if="item.status === 'warn'" spin />
                <span>{{ precheckStatusText(item.status) }}</span>
              </a-tag>
            </div>
          </div>
          <a-empty v-else description="暂无预检结果" />
        </a-card>

        <a-card
          class="detail-card detail-side-card"
          title="执行单元"
          :loading="loading"
          :bordered="true"
        >
          <div class="execution-stack">
            <a-empty
              v-if="executionUnitItems.length === 0"
              description="暂无执行单元"
            />
            <template v-else>
              <div
                v-for="(unit, index) in executionUnitItems"
                :key="unit.key"
                class="execution-summary-card"
                :class="{ 'execution-summary-card-hook': unit.kind === 'hook' }"
              >
                <template v-if="unit.kind === 'pipeline'">
                  <div class="execution-summary-head">
                    <div class="execution-summary-main">
                      <span class="execution-summary-order">{{
                        index + 1
                      }}</span>
                      <div class="execution-summary-copy">
                        <div class="execution-summary-title">
                          {{ unit.title }}
                        </div>
                        <div class="execution-summary-subtitle">
                          {{ unit.execution.binding_name || "-" }}
                        </div>
                      </div>
                    </div>
                    <div class="execution-summary-actions">
                      <a-tag
                        :class="[
                          'status-tag',
                          'execution-summary-status',
                          statusToneClass(unit.execution.status),
                        ]"
                      >
                        <LoadingOutlined
                          v-if="isRunningStatus(unit.execution.status)"
                          spin
                        />
                        <span>{{ statusText(unit.execution.status) }}</span>
                      </a-tag>
                    </div>
                  </div>
                  <div class="execution-summary-meta">
                    <span>
                      <span class="execution-summary-meta-label">执行器</span>
                      <span>{{ unit.execution.provider || "-" }}</span>
                    </span>
                    <span>
                      <span class="execution-summary-meta-label">开始</span>
                      <span>{{ formatTime(unit.execution.started_at) }}</span>
                    </span>
                    <span>
                      <span class="execution-summary-meta-label">结束</span>
                      <span>{{ formatTime(unit.execution.finished_at) }}</span>
                    </span>
                  </div>
                </template>
                <template v-else>
                  <div class="execution-summary-head">
                    <div class="execution-summary-main">
                      <span class="execution-summary-order">{{
                        index + 1
                      }}</span>
                      <div class="execution-summary-copy">
                        <div class="execution-summary-title">
                          {{ unit.group.title }}
                        </div>
                        <div class="execution-summary-subtitle">
                          {{ hookGroupSummaryText(unit.group) }}
                        </div>
                      </div>
                    </div>
                    <div class="execution-summary-actions">
                      <a-button
                        class="hook-task-toggle"
                        :class="{
                          'hook-task-toggle-open':
                            expandedHookTaskMap[unit.group.key],
                        }"
                        size="small"
                        shape="round"
                        @click="toggleHookTasks(unit.group.key)"
                      >
                        {{
                          expandedHookTaskMap[unit.group.key]
                            ? "收起任务"
                            : `展开任务 ${unit.group.items.length}`
                        }}
                      </a-button>
                      <a-tag
                        :class="[
                          'status-tag',
                          'execution-summary-status',
                          hookToneClass(unit.group.overallStatus),
                        ]"
                      >
                        <LoadingOutlined
                          v-if="unit.group.overallStatus === 'running'"
                          spin
                        />
                        <span>{{ hookStatusText(unit.group.overallStatus) }}</span>
                      </a-tag>
                    </div>
                  </div>

                  <div
                    v-if="expandedHookTaskMap[unit.group.key]"
                    class="hook-progress-rows"
                  >
                    <div
                      v-for="item in unit.group.items"
                      :key="item.id"
                      class="hook-progress-row"
                    >
                      <div class="hook-progress-row-main">
                        <div class="hook-progress-item-name-row">
                          <span class="hook-progress-item-index"
                            >#{{ item.order_index }}</span
                          >
                          <span class="hook-progress-item-name">{{
                            item.step_name
                          }}</span>
                          <a-tag
                            :class="['status-tag', hookToneClass(item.status)]"
                          >
                            <LoadingOutlined v-if="item.status === 'running'" spin />
                            <span>{{ hookStatusText(item.status) }}</span>
                          </a-tag>
                        </div>
                        <div class="hook-progress-row-detail">
                          {{ hookExecutionContentText(item) }}
                        </div>
                        <div class="hook-progress-task-grid">
                          <div class="hook-progress-task-item">
                            <span class="hook-progress-task-label">阶段/类型</span>
                            <span class="hook-progress-task-value">{{
                              hookTaskStageTypeText(item)
                            }}</span>
                          </div>
                          <div class="hook-progress-task-item">
                            <span class="hook-progress-task-label">关联任务</span>
                            <span class="hook-progress-task-value">{{
                              hookTaskReferenceText(item)
                            }}</span>
                          </div>
                          <div class="hook-progress-task-item">
                            <span class="hook-progress-task-label">开始</span>
                            <span class="hook-progress-task-value">{{
                              formatTime(item.started_at)
                            }}</span>
                          </div>
                          <div class="hook-progress-task-item">
                            <span class="hook-progress-task-label">结束</span>
                            <span class="hook-progress-task-value">{{
                              formatTime(item.finished_at)
                            }}</span>
                          </div>
                        </div>
                      </div>
                      <div class="hook-progress-item-actions">
                        <a-button
                          type="link"
                          size="small"
                          @click="toggleHookLogs(item.id)"
                        >
                          {{ expandedHookLogMap[item.id] ? "收起日志" : "查看日志" }}
                        </a-button>
                      </div>
                      <div
                        v-if="expandedHookLogMap[item.id]"
                        class="hook-progress-panel"
                      >
                        <div class="hook-progress-panel-title">执行日志</div>
                        <pre class="hook-progress-log">{{
                          hookExecutionLogText(item)
                        }}</pre>
                      </div>
                    </div>
                  </div>
                </template>
              </div>
            </template>
          </div>
        </a-card>

      </div>
    </div>

    <a-drawer
      :open="stageLogDrawerVisible"
      :width="760"
      :title="
        selectedPipelineStage
          ? `${selectedPipelineStage.pipeline_scope?.toUpperCase() || ''} 阶段日志 · ${selectedPipelineStage.stage_name}`
          : '阶段日志'
      "
      @close="closeStageLogDrawer"
    >
      <template #extra>
        <a-space>
          <a-tag
            v-if="selectedPipelineStage"
            :class="[
              'status-tag',
              statusToneClass(selectedPipelineStage.status),
            ]"
          >
            {{ statusText(selectedPipelineStage.status) }}
          </a-tag>
          <a-button
            size="small"
            :loading="stageLogLoading"
            @click="loadStageLog"
            >刷新日志</a-button
          >
          <a-popover
            v-if="stageLogSyncMessage"
            trigger="click"
            placement="bottomRight"
            overlay-class-name="release-tip-popover"
          >
            <template #content>
              <div class="release-tip-content">
                {{ stageLogSyncMessage }}
              </div>
            </template>
            <button
              class="release-tip-trigger release-tip-trigger-info"
              type="button"
              aria-label="查看日志同步提示"
            >
              <ExclamationCircleOutlined />
            </button>
          </a-popover>
        </a-space>
      </template>

      <pre class="log-panel stage-log-panel">{{
        stageLogContent || "暂无阶段日志输出"
      }}</pre>
    </a-drawer>

    <a-modal
      :open="approvalActionModalVisible"
      :title="approvalActionModalTitle"
      :confirm-loading="approvalActing"
      :ok-text="approvalActionModalOkText"
      cancel-text="取消"
      @ok="handleApprovalAction"
      @cancel="closeApprovalActionModal"
    >
      <a-form layout="vertical">
        <a-form-item
          :label="approvalActionMode === 'reject' ? '拒绝原因' : '审批备注'"
          :required="approvalActionMode === 'reject'"
        >
          <a-textarea
            v-model:value="approvalActionComment"
            :rows="4"
            :maxlength="400"
            :placeholder="approvalActionPlaceholder"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.release-detail-header {
  padding: 0;
  border: none;
  background: transparent;
  box-shadow: none;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.page-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  min-width: 0;
}

:deep(.application-toolbar-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 42px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.34) !important;
  background: rgba(255, 255, 255, 0.42) !important;
  color: #0f172a !important;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.68),
    0 10px 22px rgba(15, 23, 42, 0.05) !important;
  backdrop-filter: blur(14px) saturate(135%);
  padding-inline: 14px;
  font-weight: 600;
}

:deep(.application-toolbar-action-btn.ant-btn:hover),
:deep(.application-toolbar-action-btn.ant-btn:focus),
:deep(.application-toolbar-action-btn.ant-btn:focus-visible),
:deep(.application-toolbar-action-btn.ant-btn:active) {
  border-color: rgba(96, 165, 250, 0.34) !important;
  background: rgba(255, 255, 255, 0.56) !important;
  color: #0f172a !important;
}

:deep(.application-toolbar-action-btn.ant-btn[disabled]),
:deep(.application-toolbar-action-btn.ant-btn.ant-btn-disabled) {
  opacity: 0.58;
  color: rgba(15, 23, 42, 0.62) !important;
}

.detail-card {
  border-radius: var(--radius-xl);
  border: 1px solid rgba(148, 163, 184, 0.14);
  background: rgba(241, 245, 249, 0.34);
  box-shadow: none;
}

.detail-card:not(.release-hero-card) {
  padding: 18px;
  border-radius: 24px;
  border-color: rgba(191, 219, 254, 0.34);
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.055), transparent 34%),
    linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.82) 0%,
      rgba(248, 250, 252, 0.62) 100%
    );
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    0 18px 42px rgba(15, 23, 42, 0.045);
}

.detail-section-card {
  border-color: rgba(147, 197, 253, 0.34);
  background:
    radial-gradient(circle at top left, rgba(14, 165, 233, 0.055), transparent 30%),
    linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.86) 0%,
      rgba(248, 250, 252, 0.64) 100%
    );
}

.detail-side-card {
  border-color: rgba(203, 213, 225, 0.58);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.05), transparent 34%),
    linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.76) 0%,
      rgba(248, 250, 252, 0.56) 100%
    );
}

.detail-card :deep(.ant-card-head) {
  min-height: auto;
  padding: 0 0 14px;
  border-bottom: none;
  background: transparent;
}

.detail-card :deep(.ant-card-head-title) {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--color-dashboard-900);
  font-size: 15px;
  font-weight: 800;
  letter-spacing: 0.01em;
}

.detail-section-card :deep(.ant-card-head-title)::before,
.detail-side-card :deep(.ant-card-head-title)::before {
  content: "";
  width: 7px;
  height: 18px;
  border-radius: 999px;
  background: linear-gradient(180deg, #60a5fa 0%, #2563eb 100%);
  box-shadow: 0 8px 16px rgba(37, 99, 235, 0.18);
}

.detail-card :deep(.ant-card-body) {
  padding: 0;
  background: transparent;
}

.detail-data-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: transparent;
}

.detail-data-table :deep(.ant-table) {
  background: transparent;
}

.detail-data-table :deep(.ant-table-thead > tr > th) {
  border-bottom: none !important;
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #e2e8f0 !important;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.detail-data-table :deep(.ant-table-thead .ant-table-cell),
.detail-data-table :deep(.ant-table-thead .ant-table-cell-fix-left),
.detail-data-table :deep(.ant-table-thead .ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #e2e8f0 !important;
}

.detail-data-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}

.detail-data-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.64);
  color: var(--color-text-main);
}

.detail-data-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(248, 250, 252, 0.92) !important;
}

.detail-data-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-left),
.detail-data-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-right) {
  background: #ffffff !important;
}

.detail-data-table :deep(.ant-table-thead > tr > th.ant-table-cell-fix-left),
.detail-data-table :deep(.ant-table-thead > tr > th.ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
}

.detail-data-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-left),
.detail-data-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right) {
  background: #ffffff !important;
}

.detail-info-descriptions :deep(.ant-descriptions-view),
.detail-snapshot-table :deep(.ant-table-container) {
  border-color: rgba(203, 213, 225, 0.62);
  border-radius: 16px;
  overflow: hidden;
}

.detail-info-descriptions :deep(.ant-descriptions-item-label),
.detail-snapshot-table :deep(.ant-table-thead > tr > th),
.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell),
.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell-fix-left),
.detail-snapshot-table :deep(.ant-table-thead .ant-table-cell-fix-right) {
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%) !important;
  color: #475569 !important;
  font-size: 13px;
  font-weight: 700;
}

.detail-info-descriptions :deep(.ant-descriptions-item-content),
.detail-snapshot-table :deep(.ant-table-tbody > tr > td),
.detail-snapshot-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-left),
.detail-snapshot-table :deep(.ant-table-tbody > tr > td.ant-table-cell-fix-right) {
  background: rgba(255, 255, 255, 0.68) !important;
  color: #0f172a;
  font-size: 13px;
}

.detail-snapshot-table :deep(.ant-table-tbody > tr:hover > td),
.detail-snapshot-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-left),
.detail-snapshot-table :deep(.ant-table-tbody > tr:hover > td.ant-table-cell-fix-right) {
  background: rgba(248, 250, 252, 0.86) !important;
}

.value-progress-collapse-heading {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.value-progress-collapse-title {
  color: #0f172a;
  font-size: inherit;
  font-weight: inherit;
  line-height: inherit;
}

.value-progress-collapse-summary {
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
}

.value-progress-group-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.value-progress-group {
  min-width: 0;
}

.value-progress-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}

.value-progress-group-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
}

.value-progress-group-meta {
  flex: none;
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
}

.value-progress-item-list {
  overflow: hidden;
  border-radius: 16px;
  border: 1px solid rgba(203, 213, 225, 0.62);
  background: rgba(255, 255, 255, 0.48);
}

.value-progress-item {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(170px, 1.05fr) minmax(180px, 1fr) minmax(220px, 1.15fr) auto;
  gap: 12px;
  align-items: center;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: transparent;
}

.value-progress-item:last-child {
  border-bottom: none;
}

.value-progress-item-running {
  background: rgba(239, 246, 255, 0.42);
}

.value-progress-item-failed {
  background: rgba(255, 241, 242, 0.48);
}

.value-progress-keyline {
  min-width: 0;
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.value-progress-order {
  flex: none;
  min-width: 26px;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  border: 1px solid rgba(147, 197, 253, 0.76);
  background: rgba(239, 246, 255, 0.84);
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 800;
  line-height: 22px;
  text-align: center;
}

.value-progress-copy {
  min-width: 0;
}

.value-progress-key {
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.value-progress-name {
  margin-top: 4px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.45;
}

.value-progress-current {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.value-progress-label,
.value-progress-row-meta b {
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
  font-style: normal;
}

.value-progress-value {
  overflow: hidden;
  color: #0f172a;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.value-progress-row-meta {
  min-width: 0;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.value-progress-row-meta span {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.value-progress-row-meta em {
  overflow: hidden;
  color: #334155;
  font-size: 12px;
  font-style: normal;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.value-progress-message {
  grid-column: 1 / -1;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.release-hero-card {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background:
    radial-gradient(
      circle at top left,
      var(--color-primary-glow),
      transparent 34%
    ),
    linear-gradient(
      180deg,
      var(--color-bg-card) 0%,
      var(--color-bg-subtle) 100%
    );
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.76),
    0 14px 30px rgba(15, 23, 42, 0.05);
}

.release-hero-card :deep(.ant-card-body) {
  padding: 22px 24px;
}

.release-hero {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(280px, 0.9fr);
  gap: 20px;
  align-items: stretch;
}

.release-hero-main {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.release-hero-title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.release-hero-label {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-text-soft);
}

.release-hero-order {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 26px;
  font-weight: 800;
  color: var(--color-dashboard-900);
  word-break: break-all;
}

.release-hero-status {
  align-self: flex-start;
  padding: 7px 14px;
  font-size: 13px;
  box-shadow: 0 10px 26px rgba(37, 99, 235, 0.12);
}

.release-hero-facts {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.hero-fact {
  padding: 14px 16px;
  border-radius: 14px;
  border: 1px solid var(--color-panel-border-strong);
  background: rgba(248, 250, 252, 0.48);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.hero-fact-label {
  font-size: 12px;
  color: var(--color-text-soft);
}

.hero-fact-value {
  font-size: 14px;
  font-weight: 700;
  color: var(--color-dashboard-900);
  word-break: break-word;
}

.release-spotlight {
  border-radius: 22px;
  align-self: stretch;
  border: 1px solid rgba(96, 165, 250, 0.22);
  background:
    radial-gradient(
      circle at top right,
      rgba(96, 165, 250, 0.14),
      transparent 42%
    ),
    linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.98) 0%,
      rgba(248, 250, 252, 0.94) 100%
    );
  box-shadow: 0 18px 38px rgba(15, 23, 42, 0.08);
  padding: 24px 26px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 92px;
  gap: 22px;
  align-items: center;
  position: relative;
  overflow: hidden;
}

.release-spotlight-success {
  border-color: rgba(74, 222, 128, 0.38);
  background:
    radial-gradient(
      circle at top right,
      rgba(74, 222, 128, 0.16),
      transparent 40%
    ),
    linear-gradient(
      180deg,
      rgba(240, 253, 244, 0.98) 0%,
      rgba(248, 250, 252, 0.94) 100%
    );
}

.release-spotlight-running {
  border-color: rgba(96, 165, 250, 0.38);
  background:
    radial-gradient(
      circle at top right,
      rgba(96, 165, 250, 0.16),
      transparent 40%
    ),
    linear-gradient(
      180deg,
      rgba(239, 246, 255, 0.98) 0%,
      rgba(248, 250, 252, 0.94) 100%
    );
}

.release-spotlight-failed {
  border-color: rgba(251, 113, 133, 0.34);
  background:
    radial-gradient(
      circle at top right,
      rgba(251, 113, 133, 0.14),
      transparent 40%
    ),
    linear-gradient(
      180deg,
      rgba(255, 241, 242, 0.98) 0%,
      rgba(255, 250, 250, 0.94) 100%
    );
}

.release-spotlight-queued,
.release-spotlight-cancelled,
.release-spotlight-pending {
  border-color: rgba(251, 191, 36, 0.34);
  background:
    radial-gradient(
      circle at top right,
      rgba(251, 191, 36, 0.14),
      transparent 40%
    ),
    linear-gradient(
      180deg,
      rgba(255, 247, 237, 0.98) 0%,
      rgba(255, 251, 235, 0.94) 100%
    );
}

.release-spotlight-icon-wrap {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

.release-spotlight-icon-orb {
  width: 60px;
  height: 60px;
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.72);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.82),
    0 10px 24px rgba(15, 23, 42, 0.07);
  backdrop-filter: blur(10px);
}

.release-spotlight-icon-orb-success {
  color: #15803d;
  background: linear-gradient(
    180deg,
    rgba(240, 253, 244, 0.9) 0%,
    rgba(255, 255, 255, 0.74) 100%
  );
  border-color: rgba(134, 239, 172, 0.4);
}

.release-spotlight-icon-orb-running {
  color: #1d4ed8;
  background: linear-gradient(
    180deg,
    rgba(239, 246, 255, 0.9) 0%,
    rgba(255, 255, 255, 0.74) 100%
  );
  border-color: rgba(147, 197, 253, 0.4);
}

.release-spotlight-icon-orb-failed {
  color: #b91c1c;
  background: linear-gradient(
    180deg,
    rgba(255, 241, 242, 0.9) 0%,
    rgba(255, 255, 255, 0.74) 100%
  );
  border-color: rgba(253, 164, 175, 0.42);
}

.release-spotlight-icon-orb-queued,
.release-spotlight-icon-orb-cancelled,
.release-spotlight-icon-orb-pending {
  color: #b45309;
  background: linear-gradient(
    180deg,
    rgba(255, 247, 237, 0.92) 0%,
    rgba(255, 255, 255, 0.74) 100%
  );
  border-color: rgba(253, 186, 116, 0.42);
}

.release-spotlight-icon {
  font-size: 24px;
}

.release-spotlight-content {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.release-spotlight-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.release-spotlight-label {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-text-soft);
}

.release-spotlight-title {
  font-size: 24px;
  line-height: 1.2;
  font-weight: 800;
  color: var(--color-dashboard-900);
}

.release-spotlight-description {
  color: var(--color-text-secondary);
  line-height: 1.9;
  max-width: 520px;
}

.release-spotlight-meta {
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.66);
  border: 1px solid rgba(148, 163, 184, 0.2);
}

.detail-dashboard {
  display: grid;
  grid-template-columns: minmax(0, 1.58fr) minmax(340px, 0.92fr);
  gap: 20px;
  align-items: start;
}

.dashboard-main,
.dashboard-side {
  display: flex;
  flex-direction: column;
}

.dashboard-main {
  gap: 20px;
}

.dashboard-main > .detail-section-card {
  order: 1;
}

.dashboard-main > .value-progress-collapse {
  order: 2;
}

.dashboard-main > .timeline-collapse {
  order: 3;
}

.dashboard-main > .base-info-collapse {
  order: 4;
}

.dashboard-side {
  gap: 16px;
}

.execution-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.execution-extra-section {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid rgba(148, 163, 184, 0.18);
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.execution-extra-section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.execution-extra-section-title {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.execution-extra-section-subtitle {
  margin-top: 4px;
  font-size: 12px;
  color: #64748b;
}

.execution-summary-card {
  padding: 14px 0;
  border-radius: 0;
  background: transparent;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  box-shadow: none;
}

.execution-summary-card:last-child {
  padding-bottom: 0;
  border-bottom: none;
}

.execution-summary-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.execution-summary-actions {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 24px;
}

.execution-summary-main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}

.execution-summary-order {
  flex: none;
  min-width: 32px;
  height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(147, 197, 253, 0.58);
  background: rgba(239, 246, 255, 0.72);
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 800;
  line-height: 1;
}

.execution-summary-copy {
  min-width: 0;
}

.execution-summary-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--color-text-main);
  line-height: 24px;
}

.execution-summary-status {
  margin-inline-end: 0;
}

.execution-summary-subtitle {
  margin-top: 4px;
  color: var(--color-text-secondary);
  font-size: 13px;
}

.execution-summary-meta {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px 10px;
  margin-top: 14px;
  margin-left: 44px;
  color: var(--color-text-secondary);
  font-size: 13px;
}

.execution-summary-card-hook .execution-summary-meta {
  margin-left: 0;
}

.execution-summary-meta > span {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.execution-summary-meta-label {
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
}

.execution-summary-caption {
  margin-top: 12px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.execution-substack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.step-groups,
.stage-sections,
.log-sections {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.scope-section {
  padding: 0 0 16px;
  border-radius: 0;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  background: transparent;
  box-shadow: none;
}

.scope-section + .scope-section {
  margin-top: 0;
}

.scope-section:last-child {
  padding-bottom: 0;
  border-bottom: none;
}

.scope-section-header {
  margin-bottom: 12px;
}

.scope-section-header-inline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.scope-section-heading {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.scope-section-subtitle {
  font-size: 12px;
  color: var(--color-text-soft);
}

.step-description {
  color: var(--color-text-secondary);
  line-height: 1.7;
}

.danger-icon {
  color: var(--color-danger);
}

.status-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 999px;
  padding: 5px 10px;
  border: 1px solid transparent;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
}

.status-tag :deep(.anticon),
.status-chip :deep(.anticon) {
  color: currentColor;
}

.status-pill-success {
  color: #15803d;
  background: linear-gradient(180deg, #f0fdf4 0%, #dcfce7 100%);
  border-color: #86efac;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.7);
}

.status-pill-running {
  color: #1d4ed8;
  background: linear-gradient(180deg, #eff6ff 0%, #dbeafe 100%);
  border-color: #93c5fd;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.status-pill-failed {
  color: #b91c1c;
  background: linear-gradient(180deg, #fff1f2 0%, #ffe4e6 100%);
  border-color: #fda4af;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.status-pill-pending {
  color: #b45309;
  background: linear-gradient(180deg, #fff7ed 0%, #ffedd5 100%);
  border-color: #fdba74;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.status-pill-warning {
  color: #c2410c;
  background: linear-gradient(180deg, #fff7ed 0%, #fed7aa 100%);
  border-color: #fdba74;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.status-pill-neutral {
  color: #475569;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  border-color: #cbd5e1;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.72);
}

.status-chip {
  border-radius: 999px;
  padding: 5px 10px;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  border: 1px solid transparent;
}

.status-chip-section {
  color: #0f172a;
  background: linear-gradient(
    180deg,
    rgba(226, 232, 240, 0.92) 0%,
    rgba(203, 213, 225, 0.85) 100%
  );
  border-color: rgba(148, 163, 184, 0.46);
}

.status-chip-running {
  color: #1d4ed8;
  background: linear-gradient(
    180deg,
    rgba(239, 246, 255, 0.96) 0%,
    rgba(219, 234, 254, 0.9) 100%
  );
  border-color: rgba(96, 165, 250, 0.56);
}

.status-chip-danger {
  color: #b91c1c;
  background: linear-gradient(
    180deg,
    rgba(255, 241, 242, 0.98) 0%,
    rgba(255, 228, 230, 0.92) 100%
  );
  border-color: rgba(251, 113, 133, 0.48);
}

.required-tag {
  margin-left: 8px;
}

.precheck-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hook-progress-item-name-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.hook-progress-item-index {
  min-width: 30px;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(248, 250, 252, 0.86);
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
}

.hook-progress-item-name {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 700;
}

.execution-summary-card-hook {
  padding: 14px 0;
}

.hook-task-toggle {
  height: 26px;
  padding: 0 12px;
  border-radius: 999px;
  border-color: rgba(147, 197, 253, 0.72);
  background: rgba(239, 246, 255, 0.74);
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  line-height: 24px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.72);
}

.hook-task-toggle:hover,
.hook-task-toggle:focus {
  border-color: rgba(96, 165, 250, 0.9);
  background: rgba(219, 234, 254, 0.9);
  color: #1d4ed8;
}

.hook-task-toggle-open {
  border-color: rgba(37, 99, 235, 0.72);
  background: rgba(37, 99, 235, 0.1);
  color: #1e40af;
}

.hook-progress-rows {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
}

.hook-progress-row {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 16px;
  border: 1px solid rgba(203, 213, 225, 0.58);
  background: rgba(255, 255, 255, 0.46);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.66);
}

.hook-progress-row:first-child {
  padding-top: 12px;
}

.hook-progress-row-main {
  min-width: 0;
}

.hook-progress-row-detail {
  margin-top: 8px;
  color: var(--color-text-main);
  font-size: 13px;
  line-height: 1.7;
}

.hook-progress-task-grid {
  margin-top: 8px;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
}

.hook-progress-task-item {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.hook-progress-task-label {
  color: var(--color-text-soft);
  font-size: 12px;
  font-weight: 700;
}

.hook-progress-task-value {
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}

.hook-progress-item-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.hook-progress-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 10px 0 0;
  border-radius: 0;
  border: none;
  border-top: 1px solid rgba(203, 213, 225, 0.54);
  background: transparent;
}

.hook-progress-panel-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--color-text-main);
}

.hook-progress-log {
  margin: 0;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 12px;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-x: auto;
}

.precheck-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
  border-radius: 0;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  background: transparent;
}

.precheck-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.precheck-item-copy {
  min-width: 0;
}

.precheck-item-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.precheck-item-message {
  margin-top: 4px;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.approval-summary {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.approval-summary-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.approval-summary-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--color-text-main);
}

.approval-summary-subtitle {
  margin-top: 6px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.approval-meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.approval-meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0 0 10px;
  border-radius: 0;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  background: transparent;
}

.approval-meta-label {
  color: var(--color-text-soft);
  font-size: 12px;
}

.approval-meta-value {
  color: var(--color-text-main);
  font-size: 13px;
  font-weight: 700;
  word-break: break-word;
}

.approval-records {
  margin-top: 18px;
  padding-top: 0;
  border-top: none;
}

.approval-records-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 700;
}

.approval-record-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.approval-record-item {
  padding: 12px 0;
  border-radius: 0;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  background: transparent;
}

.approval-record-item:last-child {
  padding-bottom: 0;
  border-bottom: none;
}

.approval-record-main {
  min-width: 0;
}

.approval-record-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.approval-record-title {
  color: var(--color-text-main);
  font-size: 14px;
  font-weight: 700;
}

.approval-record-time {
  margin-top: 6px;
  color: var(--color-text-soft);
  font-size: 12px;
}

.approval-record-comment {
  margin-top: 8px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.7;
  word-break: break-word;
}

.pipeline-stage-title-actions {
  align-items: center;
}

.pipeline-executor-chip {
  height: 24px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 0 10px;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(248, 250, 252, 0.76);
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.72);
}

.pipeline-stage-title-actions :deep(.ant-btn) {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding-inline: 10px;
  border-radius: 999px;
  border-color: rgba(203, 213, 225, 0.72);
  background: rgba(255, 255, 255, 0.62);
  color: #0f172a;
  font-size: 12px;
  font-weight: 700;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.74);
}

.pipeline-stage-title-actions :deep(.ant-btn .anticon),
.page-header :deep(.ant-btn .anticon) {
  color: currentColor;
}

.scope-section-meta {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  min-width: 0;
}

.release-tip-trigger {
  width: 18px;
  height: 18px;
  padding: 0;
  border-radius: 999px;
  border: none;
  background: transparent;
  color: #64748b;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  box-shadow: none;
}

.release-tip-trigger:hover,
.release-tip-trigger:focus-visible {
  background: rgba(219, 234, 254, 0.72);
  color: #2563eb;
  outline: none;
}

.release-tip-trigger-warning {
  color: #ea580c;
}

.release-tip-trigger-warning:hover,
.release-tip-trigger-warning:focus-visible {
  background: rgba(255, 237, 213, 0.86);
  color: #c2410c;
}

.release-tip-trigger-error {
  color: #dc2626;
}

.release-tip-trigger-error:hover,
.release-tip-trigger-error:focus-visible {
  background: rgba(254, 226, 226, 0.86);
  color: #b91c1c;
}

.release-tip-content {
  max-width: 320px;
  color: #334155;
  font-size: 13px;
  line-height: 1.7;
}

.pipeline-stage-chain {
  display: flex;
  align-items: stretch;
  flex-wrap: wrap;
  gap: 10px 16px;
  overflow: visible;
  padding: 2px 0;
}

.pipeline-stage-node {
  position: relative;
  flex: 1 1 132px;
  max-width: 168px;
  min-height: 88px;
  display: grid;
  grid-template-rows: auto 1fr;
  gap: 5px;
  padding: 8px 9px;
  border-radius: 14px;
  border: 1px solid rgba(203, 213, 225, 0.62);
  background: rgba(255, 255, 255, 0.46);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.68);
}

.pipeline-stage-node-clickable {
  cursor: pointer;
}

.pipeline-stage-node-clickable:hover,
.pipeline-stage-node-clickable:focus-visible {
  border-color: rgba(96, 165, 250, 0.68);
  background: rgba(239, 246, 255, 0.62);
  outline: none;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.74),
    0 10px 22px rgba(37, 99, 235, 0.08);
}

.pipeline-stage-node:not(:last-child)::after {
  content: "";
  position: absolute;
  top: 50%;
  right: -16px;
  width: 16px;
  height: 2px;
  background: linear-gradient(90deg, rgba(148, 163, 184, 0.7), rgba(148, 163, 184, 0.18));
}

.pipeline-stage-node:not(:last-child)::before {
  content: "";
  position: absolute;
  top: calc(50% - 4px);
  right: -16px;
  width: 8px;
  height: 8px;
  border-top: 2px solid rgba(148, 163, 184, 0.7);
  border-right: 2px solid rgba(148, 163, 184, 0.7);
  transform: rotate(45deg);
}

.pipeline-stage-node-running {
  border-color: rgba(147, 197, 253, 0.72);
  background: rgba(239, 246, 255, 0.58);
}

.pipeline-stage-node-failed {
  border-color: rgba(253, 164, 175, 0.72);
  background: rgba(255, 241, 242, 0.58);
}

.pipeline-stage-order-col {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.pipeline-stage-index {
  width: 26px;
  height: 20px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(203, 213, 225, 0.72);
  background: rgba(248, 250, 252, 0.9);
  color: #334155;
  font-size: 12px;
  font-weight: 800;
  line-height: 1;
}

.pipeline-stage-node-main {
  min-width: 0;
}

.pipeline-stage-node-head {
  display: flex;
  flex-direction: column;
  gap: 5px;
  align-items: flex-start;
}

.pipeline-stage-name {
  min-width: 0;
  color: #0f172a;
  font-size: 12px;
  font-weight: 800;
  line-height: 1.35;
  word-break: break-word;
}

.pipeline-stage-meta-line {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 5px;
  color: var(--color-text-secondary);
  font-size: 10px;
  line-height: 1.25;
}

.pipeline-stage-meta-line span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pipeline-stage-meta-line b {
  margin-right: 4px;
  color: var(--color-text-soft);
  font-size: 10px;
  font-weight: 700;
}

:deep(.step-progress .ant-steps-item-icon) {
  border-width: 1px;
  border-style: solid;
  border-color: #cbd5e1;
  background: #ffffff;
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.06);
}

:deep(.step-progress .ant-steps-item-icon > .ant-steps-icon) {
  color: #64748b;
  font-weight: 700;
}

:deep(.step-progress .ant-steps-item-process .ant-steps-item-icon) {
  border-color: #60a5fa;
  background: linear-gradient(180deg, #3b82f6 0%, #2563eb 100%);
  box-shadow: 0 12px 24px rgba(37, 99, 235, 0.26);
}

:deep(
  .step-progress .ant-steps-item-process .ant-steps-item-icon > .ant-steps-icon
) {
  color: #eff6ff;
}

:deep(.step-progress .ant-steps-item-finish .ant-steps-item-icon) {
  border-color: #4ade80;
  background: linear-gradient(180deg, #22c55e 0%, #16a34a 100%);
  box-shadow: 0 12px 24px rgba(22, 163, 74, 0.24);
}

:deep(
  .step-progress .ant-steps-item-finish .ant-steps-item-icon > .ant-steps-icon
) {
  color: #f0fdf4;
}

:deep(.step-progress .ant-steps-item-error .ant-steps-item-icon) {
  border-color: #fb7185;
  background: linear-gradient(180deg, #ef4444 0%, #dc2626 100%);
  box-shadow: 0 12px 24px rgba(220, 38, 38, 0.2);
}

:deep(
  .step-progress .ant-steps-item-error .ant-steps-item-icon > .ant-steps-icon
) {
  color: #fff1f2;
}

:deep(.step-progress .ant-steps-item-wait .ant-steps-item-icon) {
  border-color: #fbbf24;
  background: linear-gradient(180deg, #fff7ed 0%, #ffedd5 100%);
  box-shadow: none;
}

:deep(
  .step-progress .ant-steps-item-wait .ant-steps-item-icon > .ant-steps-icon
) {
  color: #b45309;
}

.detail-inline-section {
  padding: 0;
  background: transparent;
  border: none;
}

.detail-inline-section + .detail-inline-section {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid rgba(203, 213, 225, 0.62);
}

.detail-inline-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.detail-inline-section-title {
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
}

.detail-inline-section-extra {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.detail-collapse :deep(.ant-collapse-item) {
  border-radius: 24px !important;
  background:
    radial-gradient(circle at top left, rgba(14, 165, 233, 0.045), transparent 28%),
    linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.78) 0%,
      rgba(248, 250, 252, 0.58) 100%
    );
  border: 1px solid rgba(191, 219, 254, 0.32);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.76),
    0 16px 36px rgba(15, 23, 42, 0.04);
  overflow: hidden;
}

.detail-collapse :deep(.ant-collapse-header) {
  padding: 16px 18px !important;
  color: #0f172a !important;
  font-weight: 800;
}

.detail-collapse :deep(.ant-collapse-content-box) {
  padding: 0 18px 18px;
}

.log-panel {
  margin: 0;
  min-height: 260px;
  max-height: 480px;
  overflow: auto;
  padding: 14px;
  border-radius: 10px;
  background: #141414;
  color: #f5f5f5;
  font-size: 12px;
  line-height: 1.6;
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  white-space: pre-wrap;
  word-break: break-word;
}

.stage-log-panel {
  min-height: 220px;
}

.batch-progress-meta {
  display: grid;
  gap: 14px;
}

.batch-progress-badge {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0 0 12px;
  border-radius: 0;
  background: transparent;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
}

.batch-progress-label,
.batch-progress-stat-label,
.batch-progress-item-position {
  font-size: 12px;
  color: var(--color-text-secondary);
}

.batch-progress-value,
.batch-progress-stat-value {
  color: var(--color-text-main);
  font-weight: 700;
}

.batch-progress-stats {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}

.batch-progress-stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0;
  border-radius: 0;
  background: transparent;
  border: none;
}

.batch-progress-list {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.batch-progress-item {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
  border-radius: 0;
  border: none;
  border-bottom: 1px solid rgba(203, 213, 225, 0.62);
  background: transparent;
}

.batch-progress-item:last-child {
  padding-bottom: 0;
  border-bottom: none;
}

.batch-progress-item-main,
.batch-progress-item-side {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.batch-progress-item-order {
  display: flex;
  gap: 8px;
  align-items: center;
  color: var(--color-text-main);
  font-weight: 700;
}

.batch-progress-item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--color-text-secondary);
  font-size: 12px;
}

.batch-progress-item-side {
  align-items: flex-end;
}

@media (max-width: 768px) {
  .release-hero,
  .detail-dashboard {
    grid-template-columns: 1fr;
  }

  .release-spotlight {
    grid-template-columns: 1fr;
    padding: 20px 18px;
  }

  .release-spotlight-icon-wrap {
    justify-content: flex-start;
  }

  .release-spotlight-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .release-hero-facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .header-left {
    flex-direction: column;
    align-items: flex-start;
  }

  .release-hero-order {
    font-size: 20px;
  }

  .hook-progress-summary,
  .hook-progress-item-head {
    flex-direction: column;
    align-items: flex-start;
  }

  .hook-progress-task-grid {
    grid-template-columns: 1fr;
  }

  .value-progress-item {
    grid-template-columns: 1fr;
  }

  .value-progress-row-meta {
    grid-template-columns: 1fr;
  }

  .approval-meta-grid {
    grid-template-columns: 1fr;
  }

  .approval-record-title-row {
    flex-direction: column;
    align-items: flex-start;
  }

  .batch-progress-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .batch-progress-item {
    flex-direction: column;
  }

  .batch-progress-item-side {
    align-items: flex-start;
  }
}
</style>
