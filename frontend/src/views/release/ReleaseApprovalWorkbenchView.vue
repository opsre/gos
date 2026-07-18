<script setup lang="ts">
import { CheckOutlined, CloseOutlined, SyncOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  approveReleaseOrder,
  approveReleaseOrderApprovalFlowTask,
  listReleaseApprovalWorkbenchRecords,
  listReleaseApprovalWorkbenchTasks,
  rejectReleaseOrder,
  rejectReleaseOrderApprovalFlowTask,
} from '../../api/release'
import type {
  ApprovalFlowExecutionScope,
  ApprovalFlowGate,
  ReleaseApprovalWorkbenchRecord,
  ReleaseApprovalWorkbenchTask,
  ReleaseOperationType,
} from '../../types/release'
import { extractHTTPErrorMessage } from '../../utils/http-error'

const router = useRouter()

type TabKey = 'pending' | 'handled'

interface SummaryCard {
  key: TabKey
  label: string
  hint: string
  emptyText: string
  value: number
}

const activeTab = ref<TabKey>('pending')
const pendingLoading = ref(false)
const pendingTasks = ref<ReleaseApprovalWorkbenchTask[]>([])
const pendingTotal = ref(0)
const pendingPagination = reactive({ page: 1, pageSize: 10 })

const handledLoading = ref(false)
const handledRecords = ref<ReleaseApprovalWorkbenchRecord[]>([])
const handledTotal = ref(0)
const handledPagination = reactive({ page: 1, pageSize: 10 })
const refreshing = ref(false)

const approvalActionModalVisible = ref(false)
const approvalActionMode = ref<'approve' | 'reject'>('approve')
const approvalActionComment = ref('')
const approvalActionTask = ref<ReleaseApprovalWorkbenchTask | null>(null)
const approvalActing = ref(false)
const approvalActionViewportInset = ref(0)

const summaryCards = computed<SummaryCard[]>(() => [
  {
    key: 'pending',
    label: '待我审批',
    value: pendingTotal.value,
    hint: '当前真正需要我处理的审批任务',
    emptyText: '当前没有待处理的审批任务',
  },
  {
    key: 'handled',
    label: '我已处理',
    value: handledTotal.value,
    hint: '我已经通过或拒绝的审批任务',
    emptyText: '当前还没有处理过的审批任务',
  },
])

const activeSummaryCard = computed(
  () => summaryCards.value.find((item) => item.key === activeTab.value) || summaryCards.value[0],
)
const approvalActionModalTitle = computed(() => (approvalActionMode.value === 'approve' ? '审批通过' : '审批拒绝'))
const approvalActionSubmitText = computed(() => (approvalActionMode.value === 'approve' ? '通过' : '拒绝'))
const approvalActionFieldLabel = computed(() => (approvalActionMode.value === 'approve' ? '审批备注' : '拒绝原因'))
const approvalActionPlaceholder = computed(() =>
  approvalActionMode.value === 'approve' ? '可选填写审批备注' : '请填写拒绝原因',
)
const approvalActionMaskStyle = computed(() => ({
  left: `${approvalActionViewportInset.value}px`,
  width: `calc(100% - ${approvalActionViewportInset.value}px)`,
  background: 'rgba(15, 23, 42, 0.08)',
  backdropFilter: 'blur(10px)',
  WebkitBackdropFilter: 'blur(10px)',
  pointerEvents: approvalActionModalVisible.value ? 'auto' : 'none',
}))
const approvalActionWrapProps = computed(() => ({
  style: {
    left: `${approvalActionViewportInset.value}px`,
    width: `calc(100% - ${approvalActionViewportInset.value}px)`,
    pointerEvents: approvalActionModalVisible.value ? 'auto' : 'none',
  },
}))
let approvalActionViewportObserver: ResizeObserver | null = null

function pendingRowKey(record: ReleaseApprovalWorkbenchTask) {
  return `${record.source}:${record.task_id || record.release_order_id}`
}

function readApprovalActionViewportInset() {
  if (typeof document === 'undefined') return 0
  const appLayout = document.querySelector('.app-layout')
  if (appLayout) {
    const rawWidth = window.getComputedStyle(appLayout).getPropertyValue('--layout-sider-width').trim()
    const parsedWidth = Number.parseFloat(rawWidth)
    if (Number.isFinite(parsedWidth) && parsedWidth >= 0) return parsedWidth
  }
  const sider = document.querySelector('.app-sider')
  return sider ? Math.max(sider.getBoundingClientRect().width, 0) : 0
}

function syncApprovalActionViewportInset() {
  approvalActionViewportInset.value = readApprovalActionViewportInset()
}

function observeApprovalActionViewportInset() {
  if (typeof window === 'undefined' || typeof ResizeObserver === 'undefined') return
  const appLayout = document.querySelector('.app-layout')
  const sider = document.querySelector('.app-sider')
  if (!appLayout && !sider) return
  approvalActionViewportObserver?.disconnect()
  approvalActionViewportObserver = new ResizeObserver(syncApprovalActionViewportInset)
  if (appLayout) approvalActionViewportObserver.observe(appLayout)
  if (sider) approvalActionViewportObserver.observe(sider)
}

function operationTypeText(type: ReleaseOperationType) {
  if (type === 'rollback') return '标准回滚'
  if (type === 'replay') return '标准重放'
  return '普通发布'
}

function gateText(gate: ApprovalFlowGate) {
  if (gate === 'before_ci') return 'CI 构建前'
  if (gate === 'before_cd') return 'CD 部署前'
  return '执行前'
}

function executionScopeText(scope: ApprovalFlowExecutionScope | '') {
  if (scope === 'build_only') return '仅构建'
  if (scope === 'deploy_only') return '仅部署'
  if (scope === 'full_release') return '完整发布'
  return '执行时确定'
}

function approvalActionText(action: ReleaseApprovalWorkbenchRecord['action']) {
  return action === 'approve' ? '审批通过' : '审批拒绝'
}

function formatTime(value: string | null | undefined) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function waitingTime(value: string) {
  const elapsed = Math.max(Date.now() - new Date(value).getTime(), 0)
  const minutes = Math.floor(elapsed / 60_000)
  if (minutes < 60) return `${Math.max(minutes, 1)} 分钟`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时`
  return `${Math.floor(hours / 24)} 天`
}

async function loadPendingTasks() {
  pendingLoading.value = true
  try {
    const response = await listReleaseApprovalWorkbenchTasks({
      page: pendingPagination.page,
      page_size: pendingPagination.pageSize,
    })
    pendingTasks.value = response.data
    pendingTotal.value = response.total
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '待我审批加载失败'))
  } finally {
    pendingLoading.value = false
  }
}

async function loadHandledRecords() {
  handledLoading.value = true
  try {
    const response = await listReleaseApprovalWorkbenchRecords({
      page: handledPagination.page,
      page_size: handledPagination.pageSize,
    })
    handledRecords.value = response.data
    handledTotal.value = response.total
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '已处理审批加载失败'))
  } finally {
    handledLoading.value = false
  }
}

async function reloadAll() {
  await Promise.all([loadPendingTasks(), loadHandledRecords()])
}

async function handleRefresh() {
  refreshing.value = true
  try {
    await reloadAll()
  } finally {
    refreshing.value = false
  }
}

function handlePendingPageChange(page: number, pageSize: number) {
  pendingPagination.page = page
  pendingPagination.pageSize = pageSize
  void loadPendingTasks()
}

function handleHandledPageChange(page: number, pageSize: number) {
  handledPagination.page = page
  handledPagination.pageSize = pageSize
  void loadHandledRecords()
}

function goToDetail(id: string) {
  void router.push(`/releases/${id}`)
}

function openApprovalAction(mode: 'approve' | 'reject', task: ReleaseApprovalWorkbenchTask) {
  approvalActionMode.value = mode
  approvalActionComment.value = ''
  approvalActionTask.value = task
  approvalActionModalVisible.value = true
}

function closeApprovalAction() {
  approvalActionModalVisible.value = false
}

function handleApprovalActionAfterClose() {
  approvalActionComment.value = ''
  approvalActionTask.value = null
  approvalActing.value = false
}

async function handleApprovalAction() {
  const task = approvalActionTask.value
  if (!task) return
  const comment = approvalActionComment.value.trim()
  if (approvalActionMode.value === 'reject' && !comment) {
    message.warning('请先填写拒绝原因')
    return
  }
  approvalActing.value = true
  try {
    if (task.source === 'flow') {
      if (approvalActionMode.value === 'approve') {
        await approveReleaseOrderApprovalFlowTask(task.release_order_id, task.task_id, { comment })
      } else {
        await rejectReleaseOrderApprovalFlowTask(task.release_order_id, task.task_id, { comment })
      }
    } else if (approvalActionMode.value === 'approve') {
      await approveReleaseOrder(task.release_order_id, { comment })
    } else {
      await rejectReleaseOrder(task.release_order_id, { comment })
    }
    message.success(approvalActionMode.value === 'approve' ? '审批已通过' : '审批已拒绝')
    closeApprovalAction()
    await reloadAll()
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, '审批操作失败'))
  } finally {
    approvalActing.value = false
  }
}

onMounted(() => {
  syncApprovalActionViewportInset()
  observeApprovalActionViewportInset()
  void reloadAll()
})

onBeforeUnmount(() => {
  approvalActionViewportObserver?.disconnect()
  approvalActionViewportObserver = null
})
</script>

<template>
  <div class="page-wrapper">
    <div class="page-header-card page-header release-approval-page-header">
      <div class="page-header-copy">
        <h2 class="page-title">审批待办</h2>
      </div>
      <div class="page-header-actions release-approval-page-header-actions">
        <a-button class="application-toolbar-action-btn approval-refresh-btn" :loading="refreshing" @click="handleRefresh">
          <template #icon><SyncOutlined /></template>
          刷新
        </a-button>
      </div>
    </div>

    <div class="approval-summary-grid">
      <button
        v-for="item in summaryCards"
        :key="item.key"
        type="button"
        class="approval-summary-card"
        :class="[`approval-summary-card-${item.key}`, { 'is-active': activeTab === item.key }]"
        @click="activeTab = item.key"
      >
        <span class="approval-summary-card-label">{{ item.label }}</span>
        <strong class="approval-summary-card-value">{{ item.value }}</strong>
        <span class="approval-summary-card-hint">{{ item.hint }}</span>
      </button>
    </div>

    <div class="approval-workbench-panel">
      <template v-if="activeTab === 'pending'">
        <a-table
          class="approval-workbench-table"
          :row-key="pendingRowKey"
          :data-source="pendingTasks"
          :loading="pendingLoading"
          :pagination="false"
          :scroll="{ x: 980 }"
          :locale="{ emptyText: activeSummaryCard.emptyText }"
        >
          <a-table-column title="发布单" key="release_order" width="220">
            <template #default="{ record }">
              <div class="task-primary task-order-no">{{ record.order_no }}</div>
              <div class="task-secondary">{{ record.application_name }} · {{ record.env_code || '-' }}</div>
            </template>
          </a-table-column>
          <a-table-column title="当前审批任务" key="node" width="220">
            <template #default="{ record }">
              <div class="task-primary">{{ record.node_name }}</div>
              <div class="task-secondary">{{ record.flow_name }}</div>
            </template>
          </a-table-column>
          <a-table-column title="阶段 / 执行模式" key="gate" width="155">
            <template #default="{ record }">
              <a-tag color="blue">{{ gateText(record.gate) }}</a-tag>
              <div class="task-secondary">{{ executionScopeText(record.execution_scope) }}</div>
            </template>
          </a-table-column>
          <a-table-column title="审批方式" key="approval_mode" width="90">
            <template #default="{ record }">{{ record.approval_mode === 'all' ? '会签' : '或签' }}</template>
          </a-table-column>
          <a-table-column title="发起 / 等待" key="waiting" width="135">
            <template #default="{ record }">
              <div class="task-primary">{{ record.triggered_by || '-' }}</div>
              <div class="task-secondary">已等待 {{ waitingTime(record.created_at) }}</div>
            </template>
          </a-table-column>
          <a-table-column title="操作" key="actions" width="170" fixed="right">
            <template #default="{ record }">
              <a-space :size="4">
                <a-button type="link" size="small" class="table-action-button" @click="goToDetail(record.release_order_id)">详情</a-button>
                <a-button type="link" size="small" class="table-action-button" @click="openApprovalAction('approve', record)">
                  <template #icon><CheckOutlined /></template>通过
                </a-button>
                <a-button type="link" size="small" danger class="table-action-button" @click="openApprovalAction('reject', record)">
                  <template #icon><CloseOutlined /></template>拒绝
                </a-button>
              </a-space>
            </template>
          </a-table-column>
        </a-table>
        <div class="pagination-area">
          <a-pagination
            :current="pendingPagination.page"
            :page-size="pendingPagination.pageSize"
            :total="pendingTotal"
            :page-size-options="['10', '20', '50']"
            show-size-changer
            show-quick-jumper
            :show-total="(count: number) => `共 ${count} 个任务`"
            @change="handlePendingPageChange"
          />
        </div>
      </template>

      <template v-else>
        <a-table
          class="approval-workbench-table"
          row-key="id"
          :data-source="handledRecords"
          :loading="handledLoading"
          :pagination="false"
          :scroll="{ x: 960 }"
          :locale="{ emptyText: activeSummaryCard.emptyText }"
        >
          <a-table-column title="发布单" key="release_order" width="220">
            <template #default="{ record }">
              <div class="task-primary task-order-no">{{ record.order_no }}</div>
              <div class="task-secondary">{{ record.application_name }} · {{ record.env_code || '-' }}</div>
            </template>
          </a-table-column>
          <a-table-column title="审批任务" key="node" width="210">
            <template #default="{ record }">
              <div class="task-primary">{{ record.node_name }}</div>
              <div class="task-secondary">{{ gateText(record.gate) }} · {{ executionScopeText(record.execution_scope) }}</div>
            </template>
          </a-table-column>
          <a-table-column title="处理结果" key="action" width="120">
            <template #default="{ record }">
              <a-tag :color="record.action === 'approve' ? 'green' : 'red'">{{ approvalActionText(record.action) }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="审批意见" data-index="comment" key="comment" ellipsis />
          <a-table-column title="处理时间" key="created_at" width="180">
            <template #default="{ record }">{{ formatTime(record.created_at) }}</template>
          </a-table-column>
          <a-table-column title="操作" key="actions" width="90" fixed="right">
            <template #default="{ record }">
              <a-button type="link" size="small" class="table-action-button" @click="goToDetail(record.release_order_id)">详情</a-button>
            </template>
          </a-table-column>
        </a-table>
        <div class="pagination-area">
          <a-pagination
            :current="handledPagination.page"
            :page-size="handledPagination.pageSize"
            :total="handledTotal"
            :page-size-options="['10', '20', '50']"
            show-size-changer
            show-quick-jumper
            :show-total="(count: number) => `共 ${count} 条`"
            @change="handleHandledPageChange"
          />
        </div>
      </template>
    </div>

    <a-modal
      :open="approvalActionModalVisible"
      :width="680"
      :closable="false"
      :footer="null"
      :destroy-on-close="true"
      :after-close="handleApprovalActionAfterClose"
      :mask-style="approvalActionMaskStyle"
      :wrap-props="approvalActionWrapProps"
      wrap-class-name="approval-action-modal-wrap"
      @cancel="closeApprovalAction"
    >
      <template #title>
        <div class="approval-action-modal-titlebar">
          <div>
            <div class="approval-action-modal-title">{{ approvalActionModalTitle }}</div>
            <div v-if="approvalActionTask" class="approval-action-modal-context">
              {{ approvalActionTask.order_no }} · {{ approvalActionTask.node_name }}
            </div>
          </div>
          <a-button class="application-toolbar-action-btn approval-action-modal-submit-btn" :loading="approvalActing" @click="handleApprovalAction">
            {{ approvalActionSubmitText }}
          </a-button>
        </div>
      </template>
      <a-form layout="vertical" :required-mark="false" class="approval-action-form">
        <div v-if="approvalActionMode === 'reject'" class="approval-action-note">拒绝操作需要填写原因</div>
        <a-form-item :label="approvalActionFieldLabel" :required="approvalActionMode === 'reject'">
          <a-textarea v-model:value="approvalActionComment" :rows="4" :maxlength="400" :placeholder="approvalActionPlaceholder" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.release-approval-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 0 !important;
  border: none !important;
  background: transparent !important;
  box-shadow: none !important;
}

.release-approval-page-header-actions { display: flex; justify-content: flex-end; }

:deep(.application-toolbar-action-btn.ant-btn) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 42px;
  border-radius: 14px;
  border-color: rgba(148, 163, 184, 0.24) !important;
  background: rgba(255, 255, 255, 0.7) !important;
  color: #0f172a !important;
  font-weight: 700;
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.05) !important;
}

.approval-summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.approval-summary-card {
  appearance: none;
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: center;
  gap: 8px 18px;
  min-height: 118px;
  padding: 20px 22px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 20px;
  background: linear-gradient(145deg, rgba(15, 23, 42, 0.98), rgba(30, 41, 59, 0.94));
  box-shadow: 0 16px 34px rgba(15, 23, 42, 0.12);
  text-align: left;
  cursor: pointer;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.approval-summary-card:hover,
.approval-summary-card.is-active { transform: translateY(-2px); }
.approval-summary-card-pending.is-active { box-shadow: 0 20px 42px rgba(37, 99, 235, 0.22); }
.approval-summary-card-handled.is-active { box-shadow: 0 20px 42px rgba(22, 163, 74, 0.18); }
.approval-summary-card-label { color: #dbeafe; font-size: 14px; font-weight: 700; }
.approval-summary-card-value { grid-row: span 2; color: #f8fafc; font-size: 34px; line-height: 1; }
.approval-summary-card-hint { color: rgba(226, 232, 240, 0.62); font-size: 12px; }

.approval-workbench-panel { background: transparent; border: none; box-shadow: none; }
.approval-workbench-table { margin-top: 0; }
.approval-workbench-table :deep(.ant-table) { background: transparent; }
.approval-workbench-table :deep(.ant-table-container) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.44);
}
.approval-workbench-table :deep(.ant-table-thead > tr > th) {
  border-bottom: 1px solid rgba(15, 23, 42, 0.18);
  background: linear-gradient(180deg, #243247, #1f2a3d) !important;
  color: #dbeafe;
  font-size: 12px;
  font-weight: 700;
}
.approval-workbench-table :deep(.ant-table-thead > tr > th::before) { display: none; }
.approval-workbench-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(226, 232, 240, 0.78);
  background: rgba(255, 255, 255, 0.78);
}
.approval-workbench-table :deep(.ant-table-tbody > tr:hover > td) { background: #f8fafc !important; }
.approval-workbench-table :deep(.ant-table-cell-fix-right) { background: rgba(255, 255, 255, 0.98) !important; }
.approval-workbench-table :deep(.ant-table-thead .ant-table-cell-fix-right) { background: #1f2a3d !important; }
.approval-workbench-table :deep(.ant-empty) { margin: 42px 0; }

.task-primary { color: #0f172a; font-weight: 650; }
.task-order-no { overflow-wrap: anywhere; }
.task-secondary { margin-top: 3px; color: #64748b; font-size: 12px; line-height: 1.4; }
.table-action-button { padding: 0 5px; color: var(--color-dashboard-800); font-weight: 650; }
.pagination-area { display: flex; justify-content: flex-end; margin-top: 22px; }

.approval-action-modal-wrap :deep(.ant-modal-content) {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 22px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 32px 90px rgba(15, 23, 42, 0.18);
}
.approval-action-modal-wrap :deep(.ant-modal-header) { margin-bottom: 18px; background: transparent; }
.approval-action-modal-titlebar { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.approval-action-modal-title { color: #0f172a; font-size: 21px; font-weight: 800; }
.approval-action-modal-context { margin-top: 3px; color: #64748b; font-size: 12px; font-weight: 500; }
.approval-action-form { display: flex; flex-direction: column; gap: 14px; }
.approval-action-note { padding-left: 12px; border-left: 4px solid #f59e0b; color: #64748b; font-size: 13px; }

@media (max-width: 900px) {
  .approval-summary-grid { grid-template-columns: 1fr; }
}

@media (max-width: 768px) {
  .release-approval-page-header { flex-direction: column; align-items: stretch; }
  .release-approval-page-header-actions { justify-content: flex-start; }
  .pagination-area { justify-content: flex-start; overflow-x: auto; }
}
</style>
