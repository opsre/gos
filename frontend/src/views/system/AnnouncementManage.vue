<script setup lang="ts">
import {
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
} from "@ant-design/icons-vue";
import { message, Modal } from "ant-design-vue";
import dayjs from "dayjs";
import { onMounted, reactive, ref } from "vue";
import {
  listAnnouncements,
  createAnnouncement,
  updateAnnouncement,
  deleteAnnouncement,
  toggleAnnouncement,
} from "../../api/announcement";
import type { Announcement } from "../../types/announcement";
import { extractHTTPErrorMessage } from "../../utils/http-error";

const loading = ref(false);
const dataSource = ref<Announcement[]>([]);
const total = ref(0);
const filters = reactive({ page: 1, pageSize: 10 });

const modalVisible = ref(false);
const modalTitle = ref("新增公告");
const editingID = ref("");
const form = reactive({
  title: "",
  content: "",
  enabled: true,
  start_time: "",
  end_time: "",
});
const saving = ref(false);
const togglingId = ref("");

async function loadData() {
  loading.value = true;
  try {
    const response = await listAnnouncements({
      page: filters.page,
      page_size: filters.pageSize,
    });
    dataSource.value = response.data;
    total.value = response.total;
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "公告列表加载失败"));
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  modalTitle.value = "新增公告";
  editingID.value = "";
  form.title = "";
  form.content = "";
  form.enabled = true;
  form.start_time = "";
  form.end_time = "";
  modalVisible.value = true;
}

function openEdit(record: Announcement) {
  modalTitle.value = "编辑公告";
  editingID.value = record.id;
  form.title = record.title;
  form.content = record.content;
  form.enabled = record.enabled;
  form.start_time = record.start_time;
  form.end_time = record.end_time;
  modalVisible.value = true;
}

async function handleSave() {
  if (!form.title.trim()) {
    message.warning("标题不能为空");
    return;
  }
  if (!form.start_time || !form.end_time) {
    message.warning("请填写有效期");
    return;
  }
  saving.value = true;
  try {
    if (editingID.value) {
      await updateAnnouncement(editingID.value, {
        title: form.title.trim(),
        content: form.content.trim(),
        enabled: form.enabled,
        start_time: form.start_time,
        end_time: form.end_time,
      });
      message.success("公告已更新");
    } else {
      await createAnnouncement({
        title: form.title.trim(),
        content: form.content.trim(),
        enabled: form.enabled,
        start_time: form.start_time,
        end_time: form.end_time,
      });
      message.success("公告已创建");
    }
    modalVisible.value = false;
    await loadData();
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "保存失败"));
  } finally {
    saving.value = false;
  }
}

function handleDelete(record: Announcement) {
  Modal.confirm({
    title: "确认删除该公告？",
    icon: () => h(ExclamationCircleOutlined, { class: "danger-icon" }),
    okText: "确认删除",
    cancelText: "取消",
    okButtonProps: { danger: true },
    onOk: async () => {
      try {
        await deleteAnnouncement(record.id);
        message.success("公告已删除");
        await loadData();
      } catch (error) {
        message.error(extractHTTPErrorMessage(error, "删除失败"));
      }
    },
  });
}

function handlePageChange(page: number, pageSize: number) {
  filters.page = page;
  filters.pageSize = pageSize;
  loadData();
}

import { h } from "vue";
import type { TableColumnsType } from "ant-design-vue";

async function handleToggleClick(record: Announcement, checked: boolean) {
  if (record.enabled === checked) return;
  togglingId.value = record.id;
  try {
    await toggleAnnouncement(record.id, checked);
    message.success(checked ? "已启用" : "已停用");
    await loadData();
  } catch (error) {
    message.error(extractHTTPErrorMessage(error, "切换失败"));
  } finally {
    togglingId.value = "";
  }
}

const columns: TableColumnsType<Announcement> = [
  {
    title: "状态",
    key: "enabled",
    width: 70,
  },
  { title: "标题", dataIndex: "title", key: "title", width: 200 },
  { title: "内容", dataIndex: "content", key: "content", ellipsis: true },
  {
    title: "有效期",
    key: "period",
    width: 260,
    customRender: ({ record }) =>
      `${record.start_time} ~ ${record.end_time}`,
  },
  { title: "创建人", dataIndex: "created_by", key: "created_by", width: 100 },
  {
    title: "创建时间",
    dataIndex: "created_at",
    key: "created_at",
    width: 160,
  },
  { title: "操作", key: "actions", width: 140 },
];

function isActive(record: Announcement) {
  const now = dayjs();
  return now.isAfter(dayjs(record.start_time)) && now.isBefore(dayjs(record.end_time));
}

onMounted(() => {
  loadData();
});

defineExpose({ openCreate, loadData });
</script>

<template>
  <div class="announcement-manage">
    <a-table
      class="announcement-table"
      row-key="id"
      :columns="columns"
      :data-source="dataSource"
      :loading="loading"
      :pagination="{
        current: filters.page,
        pageSize: filters.pageSize,
        total,
        showSizeChanger: true,
        showTotal: (t: number) => `共 ${t} 条`,
        onChange: handlePageChange,
      }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'enabled'">
          <a-switch
            :checked="record.enabled"
            :loading="togglingId === record.id"
            size="small"
            @update:checked="(val: boolean | string | number) => handleToggleClick(record, Boolean(val))"
          />
        </template>
        <template v-if="column.key === 'title'">
          <span>
            <a-tag v-if="isActive(record)" color="green">生效中</a-tag>
            <a-tag v-else color="default">已过期</a-tag>
            {{ record.title }}
          </span>
        </template>
        <template v-if="column.key === 'actions'">
          <a-space>
            <a-button type="link" size="small" @click="openEdit(record)">
              <EditOutlined />
              编辑
            </a-button>
            <a-popconfirm
              title="确认删除该公告？"
              ok-text="确认"
              cancel-text="取消"
              @confirm="handleDelete(record)"
            >
              <a-button type="link" size="small" danger>
                <DeleteOutlined />
                删除
              </a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-modal
      v-model:open="modalVisible"
      :title="modalTitle"
      :confirm-loading="saving"
      @ok="handleSave"
      width="560px"
    >
      <a-form layout="vertical">
        <a-form-item label="标题" required>
          <a-input v-model:value="form.title" placeholder="请输入公告标题" />
        </a-form-item>
        <a-form-item label="内容">
          <a-textarea
            v-model:value="form.content"
            placeholder="请输入公告内容"
            :rows="4"
          />
        </a-form-item>
        <a-form-item label="启用">
          <a-switch v-model:checked="form.enabled" />
        </a-form-item>
        <a-form-item label="有效期" required>
          <a-space>
            <a-date-picker
              v-model:value="form.start_time"
              show-time
              format="YYYY-MM-DD HH:mm"
              value-format="YYYY-MM-DD HH:mm"
              placeholder="开始时间"
              :disabled-date="(d: dayjs.Dayjs) => d.isBefore(dayjs().startOf('day'))"
            />
            <span>~</span>
            <a-date-picker
              v-model:value="form.end_time"
              show-time
              format="YYYY-MM-DD HH:mm"
              value-format="YYYY-MM-DD HH:mm"
              placeholder="结束时间"
              :disabled-date="(d: dayjs.Dayjs) => d.isBefore(dayjs().startOf('day'))"
            />
          </a-space>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.announcement-manage {
  min-width: 0;
}

.announcement-table :deep(.ant-table-thead > tr > th) {
  background: var(--color-dashboard-900) !important;
  color: var(--color-dashboard-text) !important;
  font-size: 12px;
  font-weight: 700;
}

.announcement-table :deep(.ant-table-thead > tr > th::before) {
  display: none;
}
</style>
